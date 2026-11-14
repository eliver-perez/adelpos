//! Agente de impresión de la terminal (etapa 9G, fase E, §6 y decisión D13).
//!
//! Mientras la aplicación está abierta, la terminal imprime los trabajos de las impresoras USB conectadas a su
//! computadora que el restaurante configuró en Cocina › Impresoras como «Impresora USB de una terminal»:
//!
//! 1. informa a Go las impresoras de su computadora (solo lecturas del sistema de impresión: `lpstat -a` y
//!    `lpoptions -p` en macOS) al abrir y cada pocos minutos;
//! 2. reclama de la cola durable de Go el siguiente trabajo de sus impresoras (Go confirma el intento y la lease de
//!    60 s antes de entregarlo, con los bytes ESC/POS ya preparados: el texto, el formato o el pulso del cajón);
//! 3. lo entrega a la cola de su computadora en modo raw (`lp -d <cola> -o raw`, con argumentos separados y nunca un
//!    intérprete de órdenes) y confirma el resultado: aceptado por la cola (nunca «impreso»), error confirmado sin
//!    enviar nada o incierto (lp empezó y no devolvió un resultado inequívoco), que exige decisión humana.
//!
//! Se despierta con cada evento del canal en tiempo real de la sesión (realtime.rs llama a `nudge`) y, como
//! respaldo, consulta cada pocos segundos aunque nadie haya iniciado sesión. Cada consulta le dice a Go que la
//! terminal sigue conectada; si la aplicación se cierra o pierde la red, sus trabajos esperan en la cola («La Caja 2
//! está desconectada») y los imprime al volver.
//!
//! Reintentos sin duplicar: un reclamo sin respuesta se repite con la misma clave (Go devuelve el mismo intento), un
//! resultado sin respuesta se repite igual (Go responde lo guardado) y un intento ya entregado a la cola nunca se
//! vuelve a imprimir aunque Go lo entregue otra vez.
//!
//! Windows: la cola de impresión de Windows (EnumPrinters y RAW con winspool, como `kitchen/windows_queue.go` de Go)
//! queda para la Fase W; hasta entonces el agente no arranca en Windows (`start`) y la terminal no informa
//! impresoras, así que no se le puede asignar una.

use crate::TerminalState;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::collections::VecDeque;
use std::io::Read;
use std::process::{Command, Stdio};
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::{Duration, Instant};
use tauri::{AppHandle, Manager};

/// Aviso de que puede haber trabajo nuevo (un evento del canal en tiempo real).
static WAKE: AtomicBool = AtomicBool::new(false);

/// Consulta de respaldo sin eventos: cada consulta también mantiene a la terminal «conectada» en Go, que la
/// considera desconectada tras 30 s sin consultas.
const IDLE_POLL: Duration = Duration::from_secs(3);
/// Cada cuánto se vuelve a informar la lista de impresoras de la computadora.
const REPORT_EVERY: Duration = Duration::from_secs(300);
/// Límite de cada consulta del sistema de impresión y de lp (el mismo de Go).
const QUERY_TIMEOUT: Duration = Duration::from_secs(3);
const SUBMIT_TIMEOUT: Duration = Duration::from_secs(12);
/// Tamaño máximo de un trabajo y de las impresoras informadas.
const MAX_JOB_BYTES: usize = 4 * 1024 * 1024;
const MAX_PRINTERS: usize = 64;
/// Intentos de entregas ya hechos que se recuerdan para no imprimirlos dos veces.
const REMEMBERED_ATTEMPTS: usize = 128;

/// Avisa al agente que revise si hay trabajos (lo llama realtime.rs con cada mensaje del servidor).
pub fn nudge() {
    WAKE.store(true, Ordering::SeqCst);
}

/// Inicia el agente en segundo plano. En Windows no arranca hasta la Fase W.
pub fn start(app: AppHandle) {
    if cfg!(target_os = "windows") {
        return;
    }
    tauri::async_runtime::spawn(async move {
        run(app, SystemCommands).await;
    });
}

// ---------------------------------------------------------------------------------------------------------------
// Programas del sistema
// ---------------------------------------------------------------------------------------------------------------

/// Resultado de ejecutar un programa del sistema.
#[derive(Clone, Debug, PartialEq)]
pub enum CommandResult {
    /// No se pudo iniciar (no existe o sin permiso): nada se envió.
    NotStarted,
    /// No terminó a tiempo y se detuvo.
    TimedOut,
    /// Terminó, con su salida estándar y de error juntas.
    Finished { success: bool, output: String },
}

/// Ejecuta un programa con argumentos separados, nunca un intérprete de órdenes construido con datos del servidor.
/// Las pruebas lo sustituyen para comprobar los argumentos sin ejecutar nada ni tocar impresoras.
pub trait CommandRunner: Send + Sync {
    fn run(&self, program: &str, arguments: &[String], timeout: Duration) -> CommandResult;
}

/// Programas reales de la computadora.
pub struct SystemCommands;

impl CommandRunner for SystemCommands {
    fn run(&self, program: &str, arguments: &[String], timeout: Duration) -> CommandResult {
        let child = Command::new(program)
            .args(arguments)
            .env("LC_ALL", "C")
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn();
        let Ok(mut child) = child else {
            return CommandResult::NotStarted;
        };
        // La salida se lee en hilos aparte para que un programa que escribe mucho no se detenga esperando.
        let readers: Vec<_> = [
            child
                .stdout
                .take()
                .map(|pipe| Box::new(pipe) as Box<dyn Read + Send>),
            child
                .stderr
                .take()
                .map(|pipe| Box::new(pipe) as Box<dyn Read + Send>),
        ]
        .into_iter()
        .flatten()
        .map(|mut pipe| {
            std::thread::spawn(move || {
                let mut bytes = Vec::new();
                let _ = pipe.by_ref().take(65536).read_to_end(&mut bytes);
                bytes
            })
        })
        .collect();
        let started = Instant::now();
        let status = loop {
            match child.try_wait() {
                Ok(Some(status)) => break Some(status),
                Ok(None) if started.elapsed() < timeout => {
                    std::thread::sleep(Duration::from_millis(25))
                }
                _ => {
                    let _ = child.kill();
                    let _ = child.wait();
                    break None;
                }
            }
        };
        let mut output = Vec::new();
        for reader in readers {
            output.extend(reader.join().unwrap_or_default());
        }
        match status {
            Some(status) => CommandResult::Finished {
                success: status.success(),
                output: String::from_utf8_lossy(&output).into_owned(),
            },
            None => CommandResult::TimedOut,
        }
    }
}

/// Nombre de una cola: el mismo patrón de Go (`queuePattern`). Nunca una ruta ni otros argumentos.
pub fn valid_queue(queue: &str) -> bool {
    let mut characters = queue.chars();
    let Some(first) = characters.next() else {
        return false;
    };
    queue.len() <= 127
        && (first.is_ascii_alphanumeric() || first == '_')
        && characters.all(|character| {
            character.is_ascii_alphanumeric() || character == '_' || character == '-'
        })
}

// ---------------------------------------------------------------------------------------------------------------
// Impresoras de la computadora
// ---------------------------------------------------------------------------------------------------------------

/// Impresora que se informa a Go: la cola, el modelo, el tipo de conexión (solo el esquema de device-uri: usb,
/// socket, ipp…; nunca la dirección ni el número de serie) y el tipo de impresora de CUPS. Si sirve para tickets lo
/// decide Go.
#[derive(Clone, Debug, PartialEq, Serialize)]
pub struct LocalPrinter {
    pub queue: String,
    pub model: String,
    pub connection: String,
    pub printer_type: Option<i64>,
}

/// Colas de `lpstat -a`: la primera palabra de cada renglón (el resto se traduce según el idioma del sistema).
pub fn parse_lpstat_queues(output: &str) -> Vec<String> {
    let mut queues: Vec<String> = Vec::new();
    for line in output.lines() {
        let Some(queue) = line.split_whitespace().next() else {
            continue;
        };
        if valid_queue(queue) && !queues.iter().any(|known| known == queue) {
            queues.push(queue.to_string());
        }
    }
    queues.truncate(MAX_PRINTERS);
    queues
}

/// Atributos de `lpoptions -p cola` (no se traduce): pares nombre=valor separados por espacios, con valores entre
/// comillas simples o dobles y escapes con barra invertida. Es la misma lectura que `ParseQueueOptions` de Go.
pub fn parse_queue_options(output: &str) -> Vec<(String, String)> {
    let mut options = Vec::new();
    let (mut name, mut value) = (String::new(), String::new());
    let (mut in_value, mut escaped) = (false, false);
    let mut quote: Option<char> = None;
    let flush = |name: &mut String,
                 value: &mut String,
                 in_value: &mut bool,
                 options: &mut Vec<(String, String)>| {
        if !name.is_empty() {
            options.push((std::mem::take(name), std::mem::take(value)));
        }
        name.clear();
        value.clear();
        *in_value = false;
    };
    for character in output.chars() {
        let target = if in_value { &mut value } else { &mut name };
        if escaped {
            target.push(character);
            escaped = false;
        } else if character == '\\' {
            escaped = true;
        } else if let Some(open) = quote {
            if character == open {
                quote = None;
            } else {
                target.push(character);
            }
        } else if character == '\'' || character == '"' {
            quote = Some(character);
        } else if character.is_whitespace() {
            flush(&mut name, &mut value, &mut in_value, &mut options);
        } else if character == '=' && !in_value {
            in_value = true;
        } else {
            target.push(character);
        }
    }
    flush(&mut name, &mut value, &mut in_value, &mut options);
    options
}

/// La impresora informada a partir de sus atributos.
pub fn local_printer(queue: &str, options: &[(String, String)]) -> LocalPrinter {
    let find = |key: &str| {
        options
            .iter()
            .find(|(name, _)| name == key)
            .map(|(_, value)| value.as_str())
            .unwrap_or("")
    };
    let scheme = find("device-uri")
        .split(':')
        .next()
        .unwrap_or("")
        .trim()
        .to_ascii_lowercase();
    let connection = if scheme.len() <= 20
        && scheme
            .chars()
            .all(|character| character.is_ascii_alphanumeric() || "+.-".contains(character))
    {
        scheme
    } else {
        String::new()
    };
    let model: String = find("printer-make-and-model")
        .chars()
        .filter(|character| !character.is_control())
        .take(120)
        .collect();
    LocalPrinter {
        queue: queue.to_string(),
        model: model.trim().to_string(),
        connection,
        printer_type: find("printer-type").trim().parse().ok(),
    }
}

/// Impresoras de la computadora (solo lecturas). `None` si el sistema de impresión no respondió: entonces no se
/// informa nada y Go conserva la lista anterior.
#[cfg(not(target_os = "windows"))]
pub fn local_printers(runner: &dyn CommandRunner) -> Option<Vec<LocalPrinter>> {
    let listed = runner.run("/usr/bin/lpstat", &["-a".to_string()], QUERY_TIMEOUT);
    let queues = match listed {
        CommandResult::Finished {
            success: true,
            output,
        } => parse_lpstat_queues(&output),
        // Sin impresoras, lpstat termina con error y sin colas que listar: la lista queda vacía.
        CommandResult::Finished {
            success: false,
            output,
        } if parse_lpstat_queues(&output).is_empty() => Vec::new(),
        _ => return None,
    };
    let mut printers = Vec::new();
    for queue in queues {
        let arguments = vec!["-p".to_string(), queue.clone()];
        if let CommandResult::Finished {
            success: true,
            output,
        } = runner.run("/usr/bin/lpoptions", &arguments, QUERY_TIMEOUT)
        {
            printers.push(local_printer(&queue, &parse_queue_options(&output)));
        }
    }
    Some(printers)
}

/// Windows: punto de integración de la Fase W (EnumPrinters con su puerto, como `windows_queue.go`).
#[cfg(target_os = "windows")]
pub fn local_printers(_runner: &dyn CommandRunner) -> Option<Vec<LocalPrinter>> {
    None
}

// ---------------------------------------------------------------------------------------------------------------
// Entrega de un trabajo a la cola de la computadora
// ---------------------------------------------------------------------------------------------------------------

/// Trabajo que entrega Go al reclamar (el pulso del cajón llega igual: sus bytes exactos).
#[derive(Clone, Debug, Deserialize)]
pub struct ClaimedJob {
    pub id: String,
    pub attempt_id: String,
    pub queue: String,
    pub title: String,
    pub data: String,
}

/// Resultado que se confirma a Go, con las mismas reglas de la cola.
#[derive(Clone, Debug, PartialEq)]
pub enum Delivery {
    /// La cola de la computadora aceptó el trabajo (su referencia, «cola-123»); no confirma papel.
    Submitted(String),
    /// Nada se envió: not_found (la cola no existe), unreachable (no respondió) o setup (revisar la impresora).
    Failed(&'static str),
    /// lp empezó y no devolvió un resultado inequívoco: nunca se reintenta solo.
    Uncertain,
}

impl Delivery {
    fn body(&self, attempt_id: &str, detail: &str) -> Value {
        match self {
            Delivery::Submitted(reference) => {
                json!({"attempt_id": attempt_id, "outcome": "submitted", "reference": reference, "detail": detail})
            }
            Delivery::Failed(code) => {
                json!({"attempt_id": attempt_id, "outcome": "error", "code": code, "detail": detail})
            }
            Delivery::Uncertain => {
                json!({"attempt_id": attempt_id, "outcome": "uncertain", "detail": detail})
            }
        }
    }
}

/// Argumentos separados de lp: la cola validada, modo raw (la cola no convierte los bytes), una sola copia (Go ya
/// repitió las copias en los bytes) y el título técnico del trabajo.
pub fn lp_arguments(queue: &str, title: &str, file: &str) -> Vec<String> {
    ["-d", queue, "-o", "raw", "-n", "1", "-t", title, file]
        .iter()
        .map(|value| value.to_string())
        .collect()
}

/// Referencia del trabajo en la respuesta de lp («cola-123»), sin depender del idioma del mensaje.
pub fn job_reference(queue: &str, output: &str) -> Option<String> {
    output
        .split(|character: char| character.is_whitespace() || character == '(' || character == ')')
        .find(|word| {
            word.strip_prefix(queue)
                .and_then(|rest| rest.strip_prefix('-'))
                .is_some_and(|number| {
                    !number.is_empty() && number.chars().all(|digit| digit.is_ascii_digit())
                })
        })
        .map(str::to_string)
}

/// Título técnico seguro (lo arma Go con el identificador del trabajo).
fn valid_title(title: &str) -> bool {
    !title.is_empty()
        && title.len() <= 80
        && title
            .chars()
            .all(|character| character.is_ascii_alphanumeric() || character == '-')
}

/// Entrega los bytes a la cola de la computadora y dice qué pasó, con un detalle técnico para soporte.
pub fn deliver(
    runner: &dyn CommandRunner,
    job: &ClaimedJob,
    data: &[u8],
    folder: &std::path::Path,
) -> (Delivery, String) {
    if !valid_queue(&job.queue) || !valid_title(&job.title) {
        return (
            Delivery::Failed("setup"),
            "El trabajo no trae una cola válida.".into(),
        );
    }
    // La cola existe en esta computadora (lpstat -p falla con una cola desconocida); nada se envía si no.
    match runner.run(
        "/usr/bin/lpstat",
        &["-p".to_string(), job.queue.clone()],
        QUERY_TIMEOUT,
    ) {
        CommandResult::Finished { success: true, .. } => {}
        CommandResult::Finished {
            success: false,
            output,
        } => {
            return (Delivery::Failed("not_found"), short(&output));
        }
        CommandResult::NotStarted => {
            return (
                Delivery::Failed("unreachable"),
                "No se pudo consultar el sistema de impresión.".into(),
            );
        }
        CommandResult::TimedOut => {
            return (
                Delivery::Failed("unreachable"),
                "El sistema de impresión no respondió a tiempo.".into(),
            );
        }
    }
    let file = folder.join(format!("adel-agent-{}-{}.bin", job.id, random_hex(4)));
    if write_private(&file, data).is_err() {
        let _ = std::fs::remove_file(&file);
        return (
            Delivery::Failed("setup"),
            "No se pudo preparar el archivo privado.".into(),
        );
    }
    let result = runner.run(
        "/usr/bin/lp",
        &lp_arguments(&job.queue, &job.title, &file.to_string_lossy()),
        SUBMIT_TIMEOUT,
    );
    let _ = std::fs::remove_file(&file);
    match result {
        CommandResult::NotStarted => (
            Delivery::Failed("unreachable"),
            "No se pudo ejecutar lp.".into(),
        ),
        CommandResult::Finished {
            success: true,
            output,
        } => match job_reference(&job.queue, &output) {
            Some(reference) => (Delivery::Submitted(reference), String::new()),
            None => (Delivery::Uncertain, short(&output)),
        },
        // Una vez iniciado lp, un error o un tiempo agotado pueden ocultar que la cola sí lo aceptó.
        CommandResult::Finished {
            success: false,
            output,
        } => (Delivery::Uncertain, short(&output)),
        CommandResult::TimedOut => (Delivery::Uncertain, "lp no terminó a tiempo.".into()),
    }
}

fn short(text: &str) -> String {
    text.chars()
        .filter(|character| !character.is_control() || *character == ' ')
        .take(300)
        .collect::<String>()
        .trim()
        .to_string()
}

/// Escribe un archivo nuevo legible solo por el usuario.
fn write_private(path: &std::path::Path, data: &[u8]) -> std::io::Result<()> {
    use std::io::Write;
    let mut options = std::fs::OpenOptions::new();
    options.write(true).create_new(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        options.mode(0o600);
    }
    let mut file = options.open(path)?;
    file.write_all(data)?;
    file.sync_all()
}

/// Bytes aleatorios del generador seguro, en hexadecimal.
fn random_hex(size: usize) -> String {
    let mut bytes = vec![0u8; size];
    let _ = rustls::crypto::ring::default_provider()
        .secure_random
        .fill(&mut bytes);
    crate::pairing::to_hex(&bytes)
}

/// Identificador nuevo (UUID v4) para la clave de un reclamo.
pub fn new_claim_id() -> String {
    let mut bytes = [0u8; 16];
    let _ = rustls::crypto::ring::default_provider()
        .secure_random
        .fill(&mut bytes);
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    let hex = crate::pairing::to_hex(&bytes);
    format!(
        "{}-{}-{}-{}-{}",
        &hex[0..8],
        &hex[8..12],
        &hex[12..16],
        &hex[16..20],
        &hex[20..32]
    )
}

/// Base64 estándar de los bytes del trabajo (sin dependencias nuevas).
pub fn decode_base64(text: &str) -> Option<Vec<u8>> {
    let value = |character: u8| -> Option<u32> {
        match character {
            b'A'..=b'Z' => Some((character - b'A') as u32),
            b'a'..=b'z' => Some((character - b'a') as u32 + 26),
            b'0'..=b'9' => Some((character - b'0') as u32 + 52),
            b'+' => Some(62),
            b'/' => Some(63),
            _ => None,
        }
    };
    let bytes = text.as_bytes();
    if bytes.len() % 4 != 0 || bytes.len() / 4 * 3 > MAX_JOB_BYTES + 3 {
        return None;
    }
    let mut result = Vec::with_capacity(bytes.len() / 4 * 3);
    for (index, chunk) in bytes.chunks(4).enumerate() {
        let last = index == bytes.len() / 4 - 1;
        let padding = chunk.iter().rev().take_while(|&&byte| byte == b'=').count();
        if padding > 2 || (padding > 0 && !last) {
            return None;
        }
        let mut group = 0u32;
        for (position, &byte) in chunk.iter().enumerate() {
            let digit = if position >= 4 - padding {
                0
            } else {
                value(byte)?
            };
            group = (group << 6) | digit;
        }
        let decoded = [(group >> 16) as u8, (group >> 8) as u8, group as u8];
        result.extend_from_slice(&decoded[..3 - padding]);
    }
    Some(result)
}

// ---------------------------------------------------------------------------------------------------------------
// Conversación con Go
// ---------------------------------------------------------------------------------------------------------------

/// Por qué no se completó una petición al servidor.
#[derive(Debug, PartialEq)]
pub enum Failure {
    /// No hubo respuesta (sin red, servidor apagado): se reintenta igual.
    Network,
    /// La terminal no tiene credencial o fue revocada: el agente espera.
    Unauthorized,
    /// Otra respuesta del servidor (su código y su clave de error).
    Rejected(u16, String),
}

/// Lo que el agente necesita de la terminal: hacer peticiones con su credencial y saber qué imprimió.
pub trait AgentServer: Send + Sync {
    fn call(
        &self,
        method: &'static str,
        path: &str,
        body: Value,
    ) -> impl std::future::Future<Output = Result<Value, Failure>> + Send;
}

/// Lo que pasó en una vuelta del agente.
#[derive(Debug, PartialEq)]
pub enum Cycle {
    /// Entregó (o confirmó) un trabajo: busca el siguiente de inmediato.
    Worked,
    /// No había trabajo.
    Idle,
    Unauthorized,
    Offline,
}

/// Estado del agente entre vueltas: los intentos ya entregados a la cola con su resultado, para nunca imprimir dos
/// veces el mismo intento.
#[derive(Default)]
pub struct AgentMemory {
    delivered: VecDeque<(String, Delivery, String)>,
}

impl AgentMemory {
    fn find(&self, attempt_id: &str) -> Option<(Delivery, String)> {
        self.delivered
            .iter()
            .find(|(attempt, _, _)| attempt == attempt_id)
            .map(|(_, delivery, detail)| (delivery.clone(), detail.clone()))
    }

    fn remember(&mut self, attempt_id: &str, delivery: Delivery, detail: String) {
        self.delivered
            .push_back((attempt_id.to_string(), delivery, detail));
        while self.delivered.len() > REMEMBERED_ATTEMPTS {
            self.delivered.pop_front();
        }
    }
}

/// Repite una petición que no tuvo respuesta, con el mismo cuerpo (Go la trata sin duplicar).
async fn with_retries<S: AgentServer>(
    server: &S,
    method: &'static str,
    path: &str,
    body: Value,
    tries: u32,
) -> Result<Value, Failure> {
    let mut attempt = 0;
    loop {
        match server.call(method, path, body.clone()).await {
            Err(Failure::Network) if attempt + 1 < tries => {
                attempt += 1;
                tokio::time::sleep(Duration::from_millis(500 * u64::from(attempt))).await;
            }
            result => return result,
        }
    }
}

/// Informa las impresoras de la computadora.
pub async fn report_printers<S: AgentServer>(
    server: &S,
    printers: Vec<LocalPrinter>,
) -> Result<(), Failure> {
    with_retries(
        server,
        "PUT",
        "print-agent/printers",
        json!({ "printers": printers }),
        2,
    )
    .await
    .map(|_| ())
}

/// Una vuelta: reclama el siguiente trabajo, lo entrega a la cola (si no se entregó ya) y confirma el resultado.
pub async fn cycle<S: AgentServer, R: CommandRunner + 'static>(
    server: &S,
    runner: std::sync::Arc<R>,
    memory: &mut AgentMemory,
    folder: std::path::PathBuf,
) -> Cycle {
    let claim = json!({ "claim_id": new_claim_id() });
    let claimed = match with_retries(server, "POST", "print-agent/jobs/claim", claim, 3).await {
        Ok(value) => value,
        Err(Failure::Network) => return Cycle::Offline,
        Err(Failure::Unauthorized) => return Cycle::Unauthorized,
        Err(Failure::Rejected(..)) => return Cycle::Offline,
    };
    let Some(job) = claimed
        .get("job")
        .filter(|value| !value.is_null())
        .and_then(|value| serde_json::from_value::<ClaimedJob>(value.clone()).ok())
    else {
        return Cycle::Idle;
    };
    let (delivery, detail) = match memory.find(&job.attempt_id) {
        // Ya se entregó a la cola: solo se vuelve a confirmar.
        Some(known) => known,
        None => {
            let result = match decode_base64(&job.data) {
                None => (
                    Delivery::Failed("setup"),
                    "El trabajo llegó dañado.".to_string(),
                ),
                Some(data) => {
                    let job = job.clone();
                    let runner = runner.clone();
                    tauri::async_runtime::spawn_blocking(move || {
                        deliver(runner.as_ref(), &job, &data, &folder)
                    })
                    .await
                    .unwrap_or((Delivery::Uncertain, "El envío se interrumpió.".to_string()))
                }
            };
            memory.remember(&job.attempt_id, result.0.clone(), result.1.clone());
            result
        }
    };
    let path = format!("print-agent/jobs/{}/result", job.id);
    match with_retries(
        server,
        "POST",
        &path,
        delivery.body(&job.attempt_id, &detail),
        6,
    )
    .await
    {
        Err(Failure::Unauthorized) => Cycle::Unauthorized,
        // Sin respuesta tras varios intentos, Go deja el trabajo incierto al vencer su lease: nunca se reimprime.
        _ => Cycle::Worked,
    }
}

/// Peticiones reales con la credencial de la terminal (sin sesión de usuario).
struct TerminalServer {
    app: AppHandle,
}

impl AgentServer for TerminalServer {
    async fn call(&self, method: &'static str, path: &str, body: Value) -> Result<Value, Failure> {
        let state = self.app.state::<TerminalState>();
        let client = state.client.as_ref().map_err(|_| Failure::Network)?.clone();
        let credentials = state
            .vault()
            .and_then(|vault| vault.read())
            .map_err(|_| Failure::Unauthorized)?;
        let (Some(device), Some(token)) = (credentials.device_id, credentials.device_token) else {
            return Err(Failure::Unauthorized);
        };
        let base = state.locator.base();
        let method =
            reqwest::Method::from_bytes(method.as_bytes()).map_err(|_| Failure::Network)?;
        let request = crate::api::with_client_headers(
            client.request(method, format!("{base}/api/v1/{path}")),
        )
        .header("X-Device-ID", device)
        .header("X-Device-Token", token)
        .timeout(Duration::from_secs(20))
        .json(&body);
        let mut response = match request.send().await {
            Ok(response) => response,
            Err(error) => {
                // Como las demás peticiones (fase C): si el servidor emparejado cambió de dirección, se vuelve a buscar.
                if error.is_connect() {
                    let _ = state.locator.relocate(&base).await;
                }
                return Err(Failure::Network);
            }
        };
        let status = response.status().as_u16();
        let mut bytes = Vec::new();
        while let Some(chunk) = response.chunk().await.map_err(|_| Failure::Network)? {
            if bytes.len() + chunk.len() > MAX_JOB_BYTES * 2 {
                return Err(Failure::Network);
            }
            bytes.extend_from_slice(&chunk);
        }
        let value: Value = serde_json::from_slice(&bytes).map_err(|_| Failure::Network)?;
        if status == 401 {
            return Err(Failure::Unauthorized);
        }
        if status != 200 {
            let code = value
                .pointer("/error/code")
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_string();
            return Err(Failure::Rejected(status, code));
        }
        Ok(value)
    }
}

/// Espera hasta el siguiente aviso del canal en tiempo real o hasta la consulta de respaldo.
async fn wait_for_work(limit: Duration) {
    let started = Instant::now();
    while started.elapsed() < limit {
        if WAKE.swap(false, Ordering::SeqCst) {
            return;
        }
        tokio::time::sleep(Duration::from_millis(200)).await;
    }
}

async fn run<R: CommandRunner + 'static>(app: AppHandle, runner: R) {
    let runner = std::sync::Arc::new(runner);
    let server = TerminalServer { app: app.clone() };
    let mut memory = AgentMemory::default();
    let mut reported: Option<Instant> = None;
    let mut offline_delay = Duration::from_secs(1);
    let folder = std::env::temp_dir();
    loop {
        if reported.is_none_or(|at| at.elapsed() >= REPORT_EVERY) {
            let reader = runner.clone();
            let printers =
                tauri::async_runtime::spawn_blocking(move || local_printers(reader.as_ref()))
                    .await
                    .ok()
                    .flatten();
            match printers {
                Some(printers) => match report_printers(&server, printers).await {
                    Ok(()) => reported = Some(Instant::now()),
                    Err(Failure::Unauthorized) => {
                        // Terminal sin credencial todavía (emparejándose) o revocada: espera sin consultar de más.
                        tokio::time::sleep(Duration::from_secs(10)).await;
                        continue;
                    }
                    Err(_) => {}
                },
                None => reported = Some(Instant::now()),
            }
        }
        match cycle(&server, runner.clone(), &mut memory, folder.clone()).await {
            Cycle::Worked => offline_delay = Duration::from_secs(1),
            Cycle::Idle => {
                offline_delay = Duration::from_secs(1);
                wait_for_work(IDLE_POLL).await;
            }
            Cycle::Unauthorized => tokio::time::sleep(Duration::from_secs(10)).await,
            Cycle::Offline => {
                tokio::time::sleep(offline_delay).await;
                offline_delay = (offline_delay * 2).min(Duration::from_secs(15));
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{Arc, Mutex};

    /// Ejecutor simulado: registra cada programa con sus argumentos y responde lo indicado, sin ejecutar nada.
    #[derive(Default)]
    struct Simulated {
        calls: Mutex<Vec<(String, Vec<String>)>>,
        responses: Mutex<Vec<(String, CommandResult)>>,
    }

    impl Simulated {
        fn respond(&self, program: &str, result: CommandResult) {
            self.responses
                .lock()
                .unwrap()
                .push((program.to_string(), result));
        }
        fn calls(&self) -> Vec<(String, Vec<String>)> {
            self.calls.lock().unwrap().clone()
        }
    }

    impl CommandRunner for Simulated {
        fn run(&self, program: &str, arguments: &[String], _timeout: Duration) -> CommandResult {
            self.calls
                .lock()
                .unwrap()
                .push((program.to_string(), arguments.to_vec()));
            let mut responses = self.responses.lock().unwrap();
            let index = responses.iter().position(|(name, _)| name == program);
            match index {
                Some(index) => responses.remove(index).1,
                None => CommandResult::NotStarted,
            }
        }
    }

    fn finished(output: &str) -> CommandResult {
        CommandResult::Finished {
            success: true,
            output: output.into(),
        }
    }

    fn failed(output: &str) -> CommandResult {
        CommandResult::Finished {
            success: false,
            output: output.into(),
        }
    }

    fn job(queue: &str) -> ClaimedJob {
        ClaimedJob {
            id: "11111111-2222-4333-8444-555555555555".into(),
            attempt_id: "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee".into(),
            queue: queue.into(),
            title: "ADEL-11111111-2222-4333-8444-555555555555".into(),
            data: "G0AbdAJIb2xhCgoK".into(),
        }
    }

    fn folder() -> std::path::PathBuf {
        let path = std::env::temp_dir().join(format!("adel-agent-test-{}", random_hex(6)));
        std::fs::create_dir_all(&path).unwrap();
        path
    }

    #[test]
    fn lpstat_and_lpoptions_are_read_without_translation_dependencies() {
        // Salida de lpstat -a en español (macOS la traduce aunque se pida LC_ALL=C).
        let queues = parse_lpstat_queues(
            "Printer_POS_80 aceptando solicitudes desde jue 12 nov 2026\n\
             HP_Smart_Tank_530_series aceptando solicitudes desde mié 11 nov 2026\n\
             Printer_POS_80 repetida\n\
             lpstat: algo; rm -rf /\n",
        );
        assert_eq!(queues, vec!["Printer_POS_80", "HP_Smart_Tank_530_series"]);
        // Atributos de lpoptions -p como los grabó la etapa 7.1 (kitchen/testdata/cups).
        let options = parse_queue_options(
            "copies=1 device-uri=usb://Printer/POS-80?serial=XYZ finishings=3 printer-make-and-model='POS-80 Series' \
             printer-type=36876 printer-info=\"Caja \\\"2\\\"\"",
        );
        let printer = local_printer("Printer_POS_80", &options);
        assert_eq!(
            printer,
            LocalPrinter {
                queue: "Printer_POS_80".into(),
                model: "POS-80 Series".into(),
                connection: "usb".into(),
                printer_type: Some(36876),
            }
        );
        // La conexión es solo el esquema: nunca el número de serie ni la dirección de la impresora.
        assert!(!serde_json::to_string(&printer).unwrap().contains("XYZ"));
        let airprint = local_printer(
            "HP",
            &parse_queue_options("device-uri=dnssd://HP._ipps._tcp.local/ printer-make-and-model='HP Smart Tank-AirPrint'"),
        );
        assert_eq!(airprint.connection, "dnssd");
        assert_eq!(airprint.printer_type, None);
    }

    #[test]
    fn local_printers_use_separate_arguments_and_tolerate_an_empty_system() {
        let runner = Simulated::default();
        runner.respond(
            "/usr/bin/lpstat",
            finished("POS_80 accepting requests since Thu\nOficina accepting\n"),
        );
        runner.respond(
            "/usr/bin/lpoptions",
            finished("device-uri=usb://POS printer-make-and-model='POS-80'"),
        );
        runner.respond(
            "/usr/bin/lpoptions",
            finished("device-uri=ipp://oficina.local/ipp printer-make-and-model='Generic'"),
        );
        let printers = local_printers(&runner).unwrap();
        assert_eq!(printers.len(), 2);
        assert_eq!(printers[1].connection, "ipp");
        let calls = runner.calls();
        assert_eq!(calls[0], ("/usr/bin/lpstat".into(), vec!["-a".into()]));
        assert_eq!(
            calls[1],
            (
                "/usr/bin/lpoptions".into(),
                vec!["-p".into(), "POS_80".into()]
            )
        );
        // Sin impresoras, lpstat falla sin colas: la lista queda vacía. Sin respuesta, no se informa nada.
        let empty = Simulated::default();
        empty.respond(
            "/usr/bin/lpstat",
            failed("lpstat: No hay destinos agregados."),
        );
        assert_eq!(local_printers(&empty), Some(vec![]));
        let silent = Simulated::default();
        silent.respond("/usr/bin/lpstat", CommandResult::TimedOut);
        assert_eq!(local_printers(&silent), None);
    }

    #[test]
    fn delivery_uses_raw_lp_with_separate_arguments_and_never_a_shell() {
        let runner = Simulated::default();
        runner.respond("/usr/bin/lpstat", finished("printer POS_80 is idle."));
        runner.respond(
            "/usr/bin/lp",
            finished("request id is POS_80-123 (1 file(s))"),
        );
        let place = folder();
        let (delivery, _) = deliver(&runner, &job("POS_80"), b"\x1b@hola", &place);
        assert_eq!(delivery, Delivery::Submitted("POS_80-123".into()));
        let calls = runner.calls();
        assert_eq!(
            calls[0],
            ("/usr/bin/lpstat".into(), vec!["-p".into(), "POS_80".into()])
        );
        let (program, arguments) = &calls[1];
        assert_eq!(program, "/usr/bin/lp");
        assert_eq!(
            &arguments[..8],
            &[
                "-d",
                "POS_80",
                "-o",
                "raw",
                "-n",
                "1",
                "-t",
                "ADEL-11111111-2222-4333-8444-555555555555"
            ]
        );
        assert_eq!(arguments.len(), 9);
        assert!(arguments[8].starts_with(place.to_string_lossy().as_ref()));
        // El archivo privado se borra al terminar.
        assert!(std::fs::read_dir(&place).unwrap().next().is_none());
        // Nunca se ejecuta un intérprete de órdenes.
        assert!(calls
            .iter()
            .all(|(program, _)| program.starts_with("/usr/bin/lp")));
        let _ = std::fs::remove_dir(&place);
    }

    #[test]
    fn delivery_follows_the_queue_result_rules() {
        let place = folder();
        // La cola no existe: error confirmado sin enviar nada (lp ni se ejecuta).
        let missing = Simulated::default();
        missing.respond(
            "/usr/bin/lpstat",
            failed("lpstat: Invalid destination name in list \"POS_80\"."),
        );
        assert_eq!(
            deliver(&missing, &job("POS_80"), b"x", &place).0,
            Delivery::Failed("not_found")
        );
        assert_eq!(missing.calls().len(), 1);
        // El sistema de impresión no responde: error confirmado.
        let slow = Simulated::default();
        slow.respond("/usr/bin/lpstat", CommandResult::TimedOut);
        assert_eq!(
            deliver(&slow, &job("POS_80"), b"x", &place).0,
            Delivery::Failed("unreachable")
        );
        // lp no se pudo iniciar: nada se envió.
        let absent = Simulated::default();
        absent.respond("/usr/bin/lpstat", finished(""));
        assert_eq!(
            deliver(&absent, &job("POS_80"), b"x", &place).0,
            Delivery::Failed("unreachable")
        );
        // lp empezó y no devolvió un resultado inequívoco: incierto (nunca se reintenta solo).
        for result in [
            failed("lp: error"),
            finished("sin referencia"),
            CommandResult::TimedOut,
        ] {
            let runner = Simulated::default();
            runner.respond("/usr/bin/lpstat", finished(""));
            runner.respond("/usr/bin/lp", result);
            assert_eq!(
                deliver(&runner, &job("POS_80"), b"x", &place).0,
                Delivery::Uncertain
            );
        }
        // Una cola o un título con caracteres de argumentos nunca llegan a lp.
        for queue in ["POS 80", "-o", "../POS", ""] {
            let runner = Simulated::default();
            assert_eq!(
                deliver(&runner, &job(queue), b"x", &place).0,
                Delivery::Failed("setup")
            );
            assert!(runner.calls().is_empty());
        }
        let _ = std::fs::remove_dir_all(&place);
    }

    #[test]
    fn helpers_decode_and_identify_safely() {
        assert_eq!(
            decode_base64("G0AbdAJIb2xhCgoK").unwrap(),
            b"\x1b@\x1bt\x02Hola\n\n\n"
        );
        assert_eq!(decode_base64("YQ==").unwrap(), b"a");
        assert_eq!(decode_base64("YWI=").unwrap(), b"ab");
        assert_eq!(decode_base64("").unwrap(), b"");
        assert!(decode_base64("YQ=").is_none());
        assert!(decode_base64("Y===").is_none());
        assert!(decode_base64("YQ==YQ==").is_none());
        assert!(decode_base64("@@@@").is_none());
        let claim = new_claim_id();
        assert_eq!(claim.len(), 36);
        assert_eq!(&claim[14..15], "4");
        assert!(crate::api::resource_id(Some(&claim)).is_ok());
        assert_ne!(claim, new_claim_id());
        assert_eq!(
            job_reference("POS_80", "request id is POS_80-7 (1 file(s))").as_deref(),
            Some("POS_80-7")
        );
        assert_eq!(
            job_reference("POS_80", "la solicitud es POS_80-8 (1 archivo(s))").as_deref(),
            Some("POS_80-8")
        );
        assert!(job_reference("POS_80", "request id is POS_80_X-7").is_none());
        assert!(job_reference("POS", "request id is POS_80-7").is_none());
        assert!(valid_queue("Printer_POS-80") && !valid_queue("-d") && !valid_queue("a b"));
    }

    /// Servidor simulado: guarda cada petición y responde en orden (o «sin respuesta»).
    #[derive(Default)]
    struct FakeServer {
        requests: Mutex<Vec<(String, String, Value)>>,
        responses: Mutex<VecDeque<Result<Value, Failure>>>,
    }

    impl FakeServer {
        fn then(&self, response: Result<Value, Failure>) {
            self.responses.lock().unwrap().push_back(response);
        }
        fn requests(&self) -> Vec<(String, String, Value)> {
            self.requests.lock().unwrap().clone()
        }
    }

    impl AgentServer for FakeServer {
        async fn call(
            &self,
            method: &'static str,
            path: &str,
            body: Value,
        ) -> Result<Value, Failure> {
            self.requests
                .lock()
                .unwrap()
                .push((method.into(), path.into(), body));
            self.responses
                .lock()
                .unwrap()
                .pop_front()
                .unwrap_or(Ok(json!({"job": null})))
        }
    }

    fn claimed(job: &ClaimedJob) -> Value {
        json!({"job": {"id": job.id, "attempt_id": job.attempt_id, "queue": job.queue, "title": job.title,
            "data": job.data, "size": 9, "drawer": false, "lease_until": "2026-11-13T12:00:00Z"}})
    }

    #[test]
    fn retries_never_print_the_same_attempt_twice() {
        tauri::async_runtime::block_on(async {
            let server = FakeServer::default();
            let runner = Arc::new(Simulated::default());
            runner.respond("/usr/bin/lpstat", finished(""));
            runner.respond(
                "/usr/bin/lp",
                finished("request id is POS_80-9 (1 file(s))"),
            );
            let mut memory = AgentMemory::default();
            let place = folder();
            let work = job("POS_80");
            // El reclamo pierde su respuesta y se repite con la misma clave; el resultado también.
            server.then(Err(Failure::Network));
            server.then(Ok(claimed(&work)));
            server.then(Err(Failure::Network));
            server.then(Ok(json!({"state": "submitted", "recorded": true})));
            assert_eq!(
                cycle(&server, runner.clone(), &mut memory, place.clone()).await,
                Cycle::Worked
            );
            let requests = server.requests();
            assert_eq!(requests.len(), 4);
            assert_eq!(requests[0].2["claim_id"], requests[1].2["claim_id"]);
            assert_eq!(requests[2].2, requests[3].2);
            assert_eq!(
                requests[3].1,
                format!("print-agent/jobs/{}/result", work.id)
            );
            assert_eq!(requests[3].2["outcome"], "submitted");
            assert_eq!(requests[3].2["reference"], "POS_80-9");
            assert_eq!(requests[3].2["attempt_id"], work.attempt_id.as_str());
            // Go vuelve a entregar el mismo intento (por ejemplo, el resultado no llegó): se confirma sin reimprimir.
            server.then(Ok(claimed(&work)));
            server.then(Ok(json!({"state": "submitted", "recorded": true})));
            assert_eq!(
                cycle(&server, runner.clone(), &mut memory, place.clone()).await,
                Cycle::Worked
            );
            let printed = runner
                .calls()
                .iter()
                .filter(|(program, _)| program == "/usr/bin/lp")
                .count();
            assert_eq!(printed, 1, "el mismo intento se imprimió dos veces");
            assert_eq!(server.requests().last().unwrap().2["reference"], "POS_80-9");
            // Sin trabajo, la vuelta queda inactiva; sin credencial, espera.
            server.then(Ok(json!({"job": null})));
            assert_eq!(
                cycle(&server, runner.clone(), &mut memory, place.clone()).await,
                Cycle::Idle
            );
            server.then(Err(Failure::Unauthorized));
            assert_eq!(
                cycle(&server, runner.clone(), &mut memory, place.clone()).await,
                Cycle::Unauthorized
            );
            let _ = std::fs::remove_dir_all(&place);
        });
    }

    #[test]
    fn a_damaged_job_is_reported_without_printing() {
        tauri::async_runtime::block_on(async {
            let server = FakeServer::default();
            let runner = Arc::new(Simulated::default());
            let mut work = job("POS_80");
            work.data = "no es base64".into();
            server.then(Ok(claimed(&work)));
            server.then(Ok(json!({"state": "error", "recorded": true})));
            let mut memory = AgentMemory::default();
            assert_eq!(
                cycle(&server, runner.clone(), &mut memory, std::env::temp_dir()).await,
                Cycle::Worked
            );
            assert!(runner.calls().is_empty());
            let result = &server.requests()[1].2;
            assert_eq!(result["outcome"], "error");
            assert_eq!(result["code"], "setup");
        });
    }

    #[test]
    fn printers_are_reported_as_go_expects() {
        tauri::async_runtime::block_on(async {
            let server = FakeServer::default();
            server.then(Ok(json!({"printers": []})));
            let printer = LocalPrinter {
                queue: "POS_80".into(),
                model: "POS-80".into(),
                connection: "usb".into(),
                printer_type: None,
            };
            report_printers(&server, vec![printer]).await.unwrap();
            let (method, path, body) = server.requests()[0].clone();
            assert_eq!(
                (method.as_str(), path.as_str()),
                ("PUT", "print-agent/printers")
            );
            assert_eq!(
                body,
                json!({"printers": [{"queue": "POS_80", "model": "POS-80", "connection": "usb", "printer_type": null}]})
            );
        });
    }
}
