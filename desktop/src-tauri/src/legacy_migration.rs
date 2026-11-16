// Asistente de migración del sistema anterior (etapa 9H, fase D): operaciones cerradas del puente nativo.
//
// Todo lo que toca archivos del propietario lo hace Rust, nunca React:
//
// - Los archivos se eligen solo con los diálogos nativos (tauri-plugin-dialog, usado solo desde Rust: la ventana no
//   tiene permisos dialog:* ni fs:*). Rust guarda las rutas elegidas en memoria y a React solo le dice el nombre del
//   archivo y su tamaño. React nunca envía una ruta.
// - «Preparar paquete» lanza el servidor que viaja dentro de la aplicación (junto a current_exe(): el .pkg de macOS lo
//   instala en Contents/MacOS y el instalador de Windows junto al ejecutable) con una línea de órdenes fija:
//   `ad-el-pos-server legacy-prepare --backup <respaldo> --output <paquete> --watch-stdin`, con argumentos separados
//   y sin intérprete de órdenes. Lo único que llega de React es la frase de paso, que va por la entrada estándar
//   (primera línea; nunca en argumentos, variables ni registros). Cerrar la entrada estándar cancela la preparación
//   (también en Windows, donde no hay SIGTERM); si la aplicación se cierra, el sistema cierra la tubería y el lector
//   limpia lo suyo. Los eventos JSON del lector llegan a React por un Channel (patrón de menu_subscribe), saneados:
//   sin rutas ni detalles técnicos. Al terminar, Rust anota la preparación en el Registro de movimientos con la ruta
//   de la Fase C (POST maintenance/legacy/preparations), solo con fechas, conteos, tiempos y tamaño.
// - «Importar paquete» sube el archivo a Go por partes (rutas de la Fase C): Rust lo lee, calcula su SHA-256, crea la
//   subida y envía cada parte de 1 MiB en base64 con su huella, en orden, con su avance por un Channel. React nunca
//   recibe el contenido: al terminar recibe la subida (su identificador y su estado) para pedir el análisis.
// - «Descargar informe (PDF)» pide el informe de un lote a Go y lo guarda con el diálogo «Guardar como», como
//   export_report de export.rs (tipo y firma comprobados, escritura atómica).
//
// Solo en la computadora principal (Go vuelve a exigir la escucha local y settings.manage en cada ruta).

use crate::{api, backup_folder, export, pairing, HTTPResult, TerminalState};
use reqwest::Method;
use serde_json::{json, Map, Value};
use sha2::{Digest, Sha256};
use std::ffi::OsString;
use std::fs::File;
use std::io::{BufRead, BufReader, Read, Write};
use std::path::{Path, PathBuf};
use std::process::{Child, ChildStdin, Command, Stdio};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;
use std::time::{Duration, Instant};
use tauri::ipc::Channel;
use tauri::{Manager, State};
use tauri_plugin_dialog::DialogExt;

// Nombre del servidor que viaja con la aplicación (server_binary de config/product.json; lo comprueba una prueba).
const SERVER_BINARY: &str = "ad-el-pos-server";
const BACKUP_EXTENSION: &str = "bak";
const PACKAGE_EXTENSION: &str = "adelmig";
// Tope del paquete (el de Go: 1 GiB) y de cada parte (1 MiB, part_size de Go).
const MAX_PACKAGE_BYTES: u64 = 1 << 30;
const MAX_PART_BYTES: u64 = 1 << 20;
// Una respuesta JSON de Go es pequeña; el informe en PDF, como una exportación.
const RESPONSE_MAX_BYTES: usize = 2 * 1024 * 1024;
const REPORT_MAX_BYTES: usize = 32 * 1024 * 1024;
const REPORT_TIMEOUT_SECONDS: u64 = 60;
// Una línea de avance del lector (el resumen final es la más larga).
const MAX_EVENT_LINE: usize = 4 * 1024 * 1024;
// Intentos de una parte ante un corte de la conexión (reenviar la última no cambia nada en Go).
const PART_ATTEMPTS: u32 = 3;
// La frase de paso: el mínimo y el máximo de legacypkg.ValidatePassphrase (el lector vuelve a validarla).
const MIN_PASSPHRASE_CHARS: usize = 8;
const MAX_PASSPHRASE_BYTES: usize = 1024;
// Lo más que se espera a que termine la limpieza de restos de una preparación anterior.
const CLEANUP_TIMEOUT: Duration = Duration::from_secs(90);

const NOT_MAIN_COMPUTER: &str =
    "La migración del sistema anterior solo se hace en la computadora principal del restaurante.";
const READER_UNAVAILABLE: &str = "Esta copia de AD-EL POS no incluye el preparador del paquete. Usa la aplicación instalada en la computadora principal.";
const INVALID_RESPONSE: &str = "El servidor no respondió con un contrato válido.";

// Pasos de legacy-prepare (internal/legacyread/events.go): lista cerrada.
const STEPS: &[&str] = &["image", "start", "restore", "read", "write", "cleanup"];

// ---------------------------------------------------------------------------------------------------------------
// Archivos elegidos (solo en memoria; React solo ve su nombre).

struct Selection {
    backup: Option<PathBuf>,
    output: Option<PathBuf>,
    package: Option<PathBuf>,
}

static SELECTION: Mutex<Selection> = Mutex::new(Selection {
    backup: None,
    output: None,
    package: None,
});

// Un solo diálogo de archivo a la vez.
static CHOOSING: AtomicBool = AtomicBool::new(false);

struct Guard(&'static AtomicBool);

impl Guard {
    fn acquire(flag: &'static AtomicBool, busy: &str) -> Result<Self, String> {
        if flag
            .compare_exchange(false, true, Ordering::AcqRel, Ordering::Acquire)
            .is_err()
        {
            return Err(busy.to_string());
        }
        Ok(Self(flag))
    }
}

impl Drop for Guard {
    fn drop(&mut self) {
        self.0.store(false, Ordering::Release);
    }
}

fn selection() -> std::sync::MutexGuard<'static, Selection> {
    SELECTION
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner())
}

// file_name es el nombre del archivo (sin carpeta) para mostrarlo.
fn file_name(path: &Path) -> String {
    path.file_name()
        .and_then(|name| name.to_str())
        .unwrap_or("")
        .to_string()
}

fn has_extension(path: &Path, extension: &str) -> bool {
    path.extension()
        .and_then(|value| value.to_str())
        .map(|value| value.eq_ignore_ascii_case(extension))
        .unwrap_or(false)
}

// chosen_file describe para React un archivo elegido: su nombre y su tamaño, nunca la ruta.
fn chosen_file(path: &Path) -> Result<Value, String> {
    let metadata = std::fs::metadata(path).map_err(|_| "No se pudo leer el archivo elegido.")?;
    Ok(json!({"file_name": file_name(path), "size": metadata.len()}))
}

// accept_backup comprueba el respaldo elegido: un archivo .bak existente con ruta absoluta y nombre de texto.
pub(crate) fn accept_backup(path: &Path) -> Result<Value, String> {
    if !path.is_absolute() || path.to_str().is_none() {
        return Err("Elige un respaldo guardado en esta computadora.".into());
    }
    if !has_extension(path, BACKUP_EXTENSION) {
        return Err(
            "Elige el respaldo del sistema anterior (un archivo terminado en .bak).".into(),
        );
    }
    if !path.is_file() {
        return Err("El respaldo elegido no existe o no se puede abrir.".into());
    }
    chosen_file(path)
}

// accept_output decide dónde se guarda el paquete: fuerza la extensión .adelmig y nunca reemplaza un archivo (el lector
// tampoco lo hace: OUTPUT_EXISTS).
pub(crate) fn accept_output(path: PathBuf) -> Result<PathBuf, String> {
    if !path.is_absolute() || path.to_str().is_none() {
        return Err("Elige una carpeta de esta computadora o de una USB.".into());
    }
    let path = if has_extension(&path, PACKAGE_EXTENSION) {
        path
    } else {
        let mut text = path.into_os_string();
        text.push(".");
        text.push(PACKAGE_EXTENSION);
        PathBuf::from(text)
    };
    if path.exists() {
        return Err(
            "Ya existe un archivo con ese nombre. Elige otro nombre para el paquete.".into(),
        );
    }
    let folder = path
        .parent()
        .filter(|folder| folder.is_dir())
        .ok_or("La carpeta elegida no existe.")?;
    if folder.as_os_str().is_empty() {
        return Err("La carpeta elegida no existe.".into());
    }
    Ok(path)
}

// accept_package comprueba el paquete elegido para importar: un .adelmig existente que no excede el tope.
pub(crate) fn accept_package(path: &Path) -> Result<Value, String> {
    if !path.is_absolute() || path.to_str().is_none() {
        return Err("Elige un paquete guardado en esta computadora o en una USB.".into());
    }
    if !has_extension(path, PACKAGE_EXTENSION) {
        return Err(
            "Elige el paquete de migración (un archivo terminado en .adelmig) que preparó AD-EL POS."
                .into(),
        );
    }
    let metadata = std::fs::metadata(path).map_err(|_| "No se pudo leer el paquete elegido.")?;
    if !metadata.is_file() || metadata.len() == 0 {
        return Err("El paquete elegido está vacío o no es un archivo.".into());
    }
    if metadata.len() > MAX_PACKAGE_BYTES {
        return Err("El paquete es demasiado grande: el máximo es de 1024 MB.".into());
    }
    chosen_file(path)
}

// local_only exige que la aplicación hable con el servidor de la propia computadora (la principal).
fn local_only(state: &TerminalState) -> Result<(), String> {
    let config = state.config.as_ref().map_err(Clone::clone)?;
    if !backup_folder::is_local_server(&config.server_url) {
        return Err(NOT_MAIN_COMPUTER.into());
    }
    Ok(())
}

// Los diálogos se abren fuera del hilo principal (spawn_blocking), como exigen las funciones blocking_*.
fn pick_backup(app: tauri::AppHandle, window: tauri::WebviewWindow) -> Option<PathBuf> {
    app.dialog()
        .file()
        .set_parent(&window)
        .set_title("Elegir el respaldo del sistema anterior")
        .add_filter("Respaldo del sistema anterior", &[BACKUP_EXTENSION])
        .blocking_pick_file()
        .and_then(|path| path.into_path().ok())
}

fn pick_output(
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
    suggested: String,
) -> Option<PathBuf> {
    app.dialog()
        .file()
        .set_parent(&window)
        .set_title("Guardar el paquete de migración")
        .set_file_name(suggested)
        .add_filter("Paquete de migración", &[PACKAGE_EXTENSION])
        .set_can_create_directories(true)
        .blocking_save_file()
        .and_then(|path| path.into_path().ok())
}

fn pick_package(app: tauri::AppHandle, window: tauri::WebviewWindow) -> Option<PathBuf> {
    app.dialog()
        .file()
        .set_parent(&window)
        .set_title("Elegir el paquete de migración")
        .add_filter("Paquete de migración", &[PACKAGE_EXTENSION])
        .blocking_pick_file()
        .and_then(|path| path.into_path().ok())
}

// suggested_output es el nombre que propone el diálogo: «sistema-anterior-AAAA-MM-DD.adelmig» (fecha en UTC: solo es
// una sugerencia que el propietario puede cambiar).
fn suggested_output() -> String {
    let days = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|value| value.as_secs() / 86_400)
        .unwrap_or(0) as i64;
    let (year, month, day) = civil_from_days(days);
    format!("sistema-anterior-{year:04}-{month:02}-{day:02}.{PACKAGE_EXTENSION}")
}

// civil_from_days convierte días desde 1970-01-01 en año, mes y día (algoritmo de Howard Hinnant).
fn civil_from_days(days: i64) -> (i64, u32, u32) {
    let z = days + 719_468;
    let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
    let doe = z - era * 146_097;
    let yoe = (doe - doe / 1460 + doe / 36_524 - doe / 146_096) / 365;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let day = (doy - (153 * mp + 2) / 5 + 1) as u32;
    let month = if mp < 10 { mp + 3 } else { mp - 9 } as u32;
    let year = yoe + era * 400 + if month <= 2 { 1 } else { 0 };
    (year, month, day)
}

fn busy_preparing() -> bool {
    preparation().running
}

#[tauri::command]
pub async fn legacy_choose_backup(
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
    state: State<'_, TerminalState>,
) -> Result<Value, String> {
    local_only(&state)?;
    if busy_preparing() {
        return Err("Espera a que termine la preparación en curso.".into());
    }
    let _guard = Guard::acquire(
        &CHOOSING,
        "Ya está abierto un diálogo para elegir un archivo.",
    )?;
    let chosen = tauri::async_runtime::spawn_blocking(move || pick_backup(app, window))
        .await
        .map_err(|_| "No se pudo abrir el diálogo para elegir el respaldo.")?;
    let Some(path) = chosen else {
        return Ok(json!({"result": "cancelled"}));
    };
    let file = accept_backup(&path)?;
    selection().backup = Some(path);
    Ok(json!({"result": "chosen", "file": file}))
}

#[tauri::command]
pub async fn legacy_choose_package_output(
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
    state: State<'_, TerminalState>,
) -> Result<Value, String> {
    local_only(&state)?;
    if busy_preparing() {
        return Err("Espera a que termine la preparación en curso.".into());
    }
    let _guard = Guard::acquire(
        &CHOOSING,
        "Ya está abierto un diálogo para elegir un archivo.",
    )?;
    let suggested = suggested_output();
    let chosen = tauri::async_runtime::spawn_blocking(move || pick_output(app, window, suggested))
        .await
        .map_err(|_| "No se pudo abrir el diálogo para guardar el paquete.")?;
    let Some(path) = chosen else {
        return Ok(json!({"result": "cancelled"}));
    };
    let path = accept_output(path)?;
    let name = file_name(&path);
    selection().output = Some(path);
    Ok(json!({"result": "chosen", "file": {"file_name": name}}))
}

#[tauri::command]
pub async fn legacy_choose_package(
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
    state: State<'_, TerminalState>,
) -> Result<Value, String> {
    local_only(&state)?;
    if UPLOADING.load(Ordering::Acquire) {
        return Err("Espera a que termine de subirse el paquete.".into());
    }
    let _guard = Guard::acquire(
        &CHOOSING,
        "Ya está abierto un diálogo para elegir un archivo.",
    )?;
    let chosen = tauri::async_runtime::spawn_blocking(move || pick_package(app, window))
        .await
        .map_err(|_| "No se pudo abrir el diálogo para elegir el paquete.")?;
    let Some(path) = chosen else {
        return Ok(json!({"result": "cancelled"}));
    };
    let file = accept_package(&path)?;
    selection().package = Some(path);
    Ok(json!({"result": "chosen", "file": file}))
}

// legacy_selection dice qué archivos están elegidos (al volver a entrar al asistente), sin sus rutas.
#[tauri::command]
pub fn legacy_selection(state: State<'_, TerminalState>) -> Result<Value, String> {
    local_only(&state)?;
    let current = selection();
    let describe = |path: &Option<PathBuf>| -> Value {
        path.as_deref()
            .and_then(|path| chosen_file(path).ok())
            .unwrap_or(Value::Null)
    };
    Ok(json!({
        "backup": describe(&current.backup),
        "output": current
            .output
            .as_deref()
            .map(|path| json!({"file_name": file_name(path)}))
            .unwrap_or(Value::Null),
        "package": describe(&current.package),
    }))
}

// ---------------------------------------------------------------------------------------------------------------
// Preparar paquete: el lector del sistema anterior (legacy-prepare).

struct Preparation {
    running: bool,
    // Entrada estándar del lector: cerrarla (soltarla) cancela la preparación.
    stdin: Option<ChildStdin>,
    channel: Option<Channel<Value>>,
    // Lo que vio la última preparación (pasos, último avance, última tabla y el final), para volver a entrar.
    events: Vec<Value>,
}

static PREPARATION: Mutex<Preparation> = Mutex::new(Preparation {
    running: false,
    stdin: None,
    channel: None,
    events: Vec::new(),
});

// La limpieza de restos (--cleanup) toma el mismo bloqueo del lector: mientras corre no se empieza a preparar.
static CLEANING: AtomicBool = AtomicBool::new(false);

fn preparation() -> std::sync::MutexGuard<'static, Preparation> {
    PREPARATION
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner())
}

// server_binary es el servidor que viaja con la aplicación. En una compilación de desarrollo (tauri dev) no está junto
// al ejecutable: ADEL_SERVER_BINARY puede indicar el de bin/ (solo en desarrollo; la aplicación instalada usa siempre
// el suyo).
fn server_binary() -> Result<PathBuf, String> {
    #[cfg(debug_assertions)]
    if let Some(path) = std::env::var_os("ADEL_SERVER_BINARY") {
        let path = PathBuf::from(path);
        if path.is_absolute() && path.is_file() {
            return Ok(path);
        }
    }
    let directory = std::env::current_exe()
        .ok()
        .and_then(|path| path.parent().map(Path::to_path_buf))
        .ok_or(READER_UNAVAILABLE)?;
    let candidate = directory.join(binary_file_name());
    if candidate.is_file() {
        Ok(candidate)
    } else {
        Err(READER_UNAVAILABLE.into())
    }
}

fn binary_file_name() -> String {
    if cfg!(target_os = "windows") {
        format!("{SERVER_BINARY}.exe")
    } else {
        SERVER_BINARY.to_string()
    }
}

// prepare_arguments es la línea de órdenes fija del lector: argumentos separados, nunca un intérprete de órdenes ni
// texto de React.
pub(crate) fn prepare_arguments(backup: &Path, output: &Path) -> Vec<OsString> {
    vec![
        OsString::from("legacy-prepare"),
        OsString::from("--backup"),
        backup.as_os_str().to_os_string(),
        OsString::from("--output"),
        output.as_os_str().to_os_string(),
        OsString::from("--watch-stdin"),
    ]
}

// validate_passphrase repite las reglas básicas de legacypkg.ValidatePassphrase (el lector vuelve a validarla con la
// normalización NFC): al menos 8 caracteres, hasta 1024 bytes y una sola línea.
pub(crate) fn validate_passphrase(passphrase: &str) -> Result<(), String> {
    if passphrase.len() > MAX_PASSPHRASE_BYTES {
        return Err("La frase de paso es demasiado larga.".into());
    }
    if passphrase
        .chars()
        .any(|character| character == '\n' || character == '\r' || character == '\0')
    {
        return Err("La frase de paso debe escribirse en una sola línea.".into());
    }
    if passphrase.chars().count() < MIN_PASSPHRASE_CHARS {
        return Err("La frase de paso debe tener al menos 8 caracteres.".into());
    }
    Ok(())
}

fn hidden_process(command: &mut Command) {
    #[cfg(target_os = "windows")]
    {
        use std::os::windows::process::CommandExt;
        // CREATE_NO_WINDOW: el lector no abre una consola.
        command.creation_flags(0x0800_0000);
    }
    #[cfg(not(target_os = "windows"))]
    let _ = command;
}

// start_reader lanza legacy-prepare y le entrega la frase de paso por la entrada estándar, que queda abierta (cerrarla
// cancela).
pub(crate) fn start_reader(
    binary: &Path,
    backup: &Path,
    output: &Path,
    passphrase: &str,
) -> Result<(Child, ChildStdin), String> {
    let mut command = Command::new(binary);
    command
        .args(prepare_arguments(backup, output))
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null());
    hidden_process(&mut command);
    let mut child = command
        .spawn()
        .map_err(|_| "No se pudo iniciar el preparador del paquete.".to_string())?;
    let Some(mut stdin) = child.stdin.take() else {
        let _ = child.kill();
        let _ = child.wait();
        return Err("No se pudo iniciar el preparador del paquete.".into());
    };
    let mut line = String::with_capacity(passphrase.len() + 1);
    line.push_str(passphrase);
    line.push('\n');
    if stdin
        .write_all(line.as_bytes())
        .and_then(|_| stdin.flush())
        .is_err()
    {
        drop(stdin);
        let _ = child.kill();
        let _ = child.wait();
        return Err("No se pudo entregar la frase de paso al preparador.".into());
    }
    Ok((child, stdin))
}

fn number(value: &Value, key: &str) -> u64 {
    value.get(key).and_then(Value::as_u64).unwrap_or(0)
}

fn seconds(value: Option<&Value>) -> Option<f64> {
    value
        .and_then(Value::as_f64)
        .filter(|seconds| seconds.is_finite() && *seconds >= 0.0)
}

fn plain_key(text: &str) -> bool {
    !text.is_empty()
        && text.len() <= 64
        && text
            .chars()
            .all(|character| character.is_ascii_alphanumeric() || character == '_')
}

// safe_text deja pasar solo textos cortos como los que acepta Go en el resumen de una preparación
// (^[A-Za-z0-9_:.+ -]{0,40}$): fechas y códigos, nunca datos.
pub(crate) fn safe_text(value: Option<&Value>) -> String {
    let text = value.and_then(Value::as_str).unwrap_or("");
    let valid = text.len() <= 40
        && text.chars().all(|character| {
            character.is_ascii_alphanumeric()
                || matches!(character, '_' | ':' | '.' | '+' | ' ' | '-')
        });
    if valid {
        text.to_string()
    } else {
        String::new()
    }
}

fn error_code(value: Option<&Value>) -> Option<String> {
    let code = value.and_then(Value::as_str)?;
    let valid = !code.is_empty()
        && code.len() <= 40
        && code
            .chars()
            .all(|character| character.is_ascii_uppercase() || character == '_');
    valid.then(|| code.to_string())
}

fn plain_message(value: Option<&Value>) -> String {
    value
        .and_then(Value::as_str)
        .filter(|text| text.chars().count() <= 600 && !text.chars().any(char::is_control))
        .unwrap_or("")
        .to_string()
}

fn sanitize_summary(summary: &Value) -> Value {
    let backup = summary.get("backup").cloned().unwrap_or(Value::Null);
    let sales = summary.get("sales").filter(|value| value.is_object());
    let tables: Vec<Value> = summary
        .get("tables")
        .and_then(Value::as_array)
        .map(|tables| {
            tables
                .iter()
                .filter_map(|table| {
                    let name = table.get("name").and_then(Value::as_str)?;
                    plain_key(name).then(|| json!({"name": name, "rows": number(table, "rows")}))
                })
                .take(200)
                .collect()
        })
        .unwrap_or_default();
    let mut timings = Map::new();
    if let Some(values) = summary.get("timings").and_then(Value::as_object) {
        for (key, value) in values.iter().take(50) {
            if let (true, Some(value)) = (plain_key(key), seconds(Some(value))) {
                timings.insert(key.clone(), json!(value));
            }
        }
    }
    let fingerprint = summary
        .get("content_fingerprint")
        .and_then(Value::as_str)
        .filter(|text| text.len() == 64 && text.chars().all(|c| c.is_ascii_hexdigit()))
        .unwrap_or("");
    json!({
        "package_bytes": number(summary, "package_bytes"),
        "verified": summary.get("verified").and_then(Value::as_bool).unwrap_or(false),
        "fingerprint": fingerprint.to_ascii_lowercase(),
        "backup": {
            "started_at": safe_text(backup.get("started_at")),
            "finished_at": safe_text(backup.get("finished_at")),
            "sets": number(&backup, "sets"),
        },
        "sales": sales.map(|sales| json!({
            "first": safe_text(sales.get("first")),
            "last": safe_text(sales.get("last")),
        })),
        "tables": tables,
        "timings": timings,
    })
}

// sanitize_event convierte una línea de legacy-prepare en el evento que ve React: solo la lista cerrada de eventos y
// campos, sin rutas (output, backup.file), nombres de la base ni el detalle técnico de un error. Lo demás se ignora.
pub(crate) fn sanitize_event(line: &[u8]) -> Option<Value> {
    let value: Value = serde_json::from_slice(line).ok()?;
    let event = value.get("event")?.as_str()?;
    let step = value
        .get("step")
        .and_then(Value::as_str)
        .filter(|step| STEPS.contains(step));
    match event {
        "step" => {
            let status = value
                .get("status")
                .and_then(Value::as_str)
                .filter(|status| *status == "started" || *status == "finished")?;
            let mut result = json!({"event": "step", "step": step?, "status": status});
            if let Some(seconds) = seconds(value.get("seconds")) {
                result["seconds"] = json!(seconds);
            }
            Some(result)
        }
        "progress" => Some(json!({
            "event": "progress",
            "step": step?,
            "current": number(&value, "current"),
            "total": number(&value, "total"),
        })),
        "table" => {
            let status = value
                .get("status")
                .and_then(Value::as_str)
                .filter(|status| matches!(*status, "reading" | "done" | "skipped"))?;
            Some(json!({
                "event": "table",
                "status": status,
                "done": number(&value, "done"),
                "total": number(&value, "total"),
                "rows": number(&value, "rows"),
            }))
        }
        "result" => {
            let summary = value.get("summary").filter(|summary| summary.is_object())?;
            let mut result = sanitize_summary(summary);
            result["event"] = json!("result");
            Some(result)
        }
        "error" => Some(json!({
            "event": "error",
            "code": error_code(value.get("code"))?,
            "message": plain_message(value.get("message")),
        })),
        _ => None,
    }
}

fn is_final(event: &Value) -> bool {
    matches!(
        event.get("event").and_then(Value::as_str),
        Some("result") | Some("error")
    )
}

// read_line lee una línea con tope; una más larga se descarta completa (devuelve una línea vacía).
fn read_line(reader: &mut impl BufRead, buffer: &mut Vec<u8>) -> std::io::Result<bool> {
    buffer.clear();
    let read = reader
        .by_ref()
        .take(MAX_EVENT_LINE as u64 + 1)
        .read_until(b'\n', buffer)?;
    if read == 0 {
        return Ok(false);
    }
    if buffer.len() > MAX_EVENT_LINE && !buffer.ends_with(b"\n") {
        buffer.clear();
        let mut rest = Vec::new();
        loop {
            rest.clear();
            let more = reader
                .by_ref()
                .take(MAX_EVENT_LINE as u64)
                .read_until(b'\n', &mut rest)?;
            if more == 0 || rest.ends_with(b"\n") {
                break;
            }
        }
    }
    Ok(true)
}

// follow_reader lee los eventos del lector hasta que termina, entrega a `deliver` los de avance y devuelve el final
// (un resultado o un error; si el lector terminó sin decirlo, un error propio).
pub(crate) fn follow_reader(mut child: Child, mut deliver: impl FnMut(Value)) -> Value {
    let mut last: Option<Value> = None;
    if let Some(stdout) = child.stdout.take() {
        let mut reader = BufReader::new(stdout);
        let mut buffer = Vec::new();
        while let Ok(true) = read_line(&mut reader, &mut buffer) {
            let Some(event) = sanitize_event(&buffer) else {
                continue;
            };
            if is_final(&event) {
                last = Some(event);
            } else {
                deliver(event);
            }
        }
    }
    let _ = child.wait();
    last.unwrap_or_else(|| {
        json!({
            "event": "error",
            "code": "INTERNAL",
            "message": "El preparador del paquete terminó sin avisar el resultado. Vuelve a intentarlo.",
        })
    })
}

// preparation_record es el resumen que se anota en el Registro de movimientos (POST maintenance/legacy/preparations):
// exactamente los campos que acepta Go, sin rutas ni datos del restaurante.
pub(crate) fn preparation_record(final_event: &Value) -> Value {
    let succeeded = final_event.get("event").and_then(Value::as_str) == Some("result");
    if !succeeded {
        return json!({
            "succeeded": false,
            "error_code": safe_text(final_event.get("code")),
            "tables": {},
            "timings_ms": {},
        });
    }
    let mut tables = Map::new();
    if let Some(list) = final_event.get("tables").and_then(Value::as_array) {
        for table in list.iter().take(200) {
            if let Some(name) = table
                .get("name")
                .and_then(Value::as_str)
                .filter(|name| plain_key(name))
            {
                tables.insert(name.to_string(), json!(number(table, "rows")));
            }
        }
    }
    let mut timings = Map::new();
    if let Some(values) = final_event.get("timings").and_then(Value::as_object) {
        for (key, value) in values.iter().take(50) {
            if let (true, Some(value)) = (plain_key(key), seconds(Some(value))) {
                timings.insert(key.clone(), json!((value * 1000.0).round() as u64));
            }
        }
    }
    let backup = final_event.get("backup").cloned().unwrap_or(Value::Null);
    let backup_taken_at = {
        let finished = safe_text(backup.get("finished_at"));
        if finished.is_empty() {
            safe_text(backup.get("started_at"))
        } else {
            finished
        }
    };
    let sales = final_event.get("sales").cloned().unwrap_or(Value::Null);
    json!({
        "succeeded": true,
        "error_code": "",
        "backup_taken_at": backup_taken_at,
        "sales_first": safe_text(sales.get("first")),
        "sales_last": safe_text(sales.get("last")),
        "package_bytes": number(final_event, "package_bytes"),
        "format_version": 1,
        "fingerprint": final_event.get("fingerprint").and_then(Value::as_str).unwrap_or(""),
        "tables": tables,
        "timings_ms": timings,
    })
}

// remember guarda en el resumen de la preparación lo que hace falta para volver a dibujarla: los pasos, el último avance,
// la última tabla y el final.
fn remember(events: &mut Vec<Value>, event: &Value) {
    let kind = event.get("event").and_then(Value::as_str).unwrap_or("");
    if kind == "progress" || kind == "table" {
        events.retain(|previous| previous.get("event").and_then(Value::as_str) != Some(kind));
    }
    events.push(event.clone());
}

fn deliver(event: Value) {
    let mut current = preparation();
    remember(&mut current.events, &event);
    if let Some(channel) = current.channel.as_ref() {
        let _ = channel.send(event);
    }
}

#[tauri::command]
pub fn legacy_prepare_start(
    app: tauri::AppHandle,
    state: State<'_, TerminalState>,
    passphrase: String,
    channel: Channel<Value>,
) -> Result<Value, String> {
    local_only(&state)?;
    validate_passphrase(&passphrase)?;
    if CLEANING.load(Ordering::Acquire) {
        return Err(
            "Se están borrando los restos de una preparación anterior. Vuelve a intentarlo en unos segundos."
                .into(),
        );
    }
    let mut current = preparation();
    if current.running {
        return Err("Ya se está preparando un paquete. Espera a que termine.".into());
    }
    let (backup, output) = {
        let chosen = selection();
        (chosen.backup.clone(), chosen.output.clone())
    };
    let backup = backup.ok_or("Elige primero el respaldo del sistema anterior.")?;
    let output = output.ok_or("Elige primero dónde guardar el paquete.")?;
    if output.exists() {
        return Err(
            "Ya existe un archivo con ese nombre. Elige otro nombre para el paquete.".into(),
        );
    }
    let binary = server_binary()?;
    let (child, stdin) = start_reader(&binary, &backup, &output, &passphrase)?;
    drop(passphrase);
    current.running = true;
    current.stdin = Some(stdin);
    current.channel = Some(channel);
    current.events.clear();
    drop(current);
    let package_name = file_name(&output);
    std::thread::spawn(move || {
        let mut final_event = follow_reader(child, deliver);
        let succeeded = final_event.get("event").and_then(Value::as_str) == Some("result");
        if succeeded {
            // El paquete recién preparado queda elegido para importarlo; para preparar otro se elige otro nombre.
            let mut chosen = selection();
            chosen.package = Some(output.clone());
            chosen.output = None;
            final_event["package_file_name"] = json!(package_name);
        }
        let record = preparation_record(&final_event);
        let recorded = tauri::async_runtime::block_on(async {
            let state = app.state::<TerminalState>();
            matches!(
                call(&state, Method::POST, "maintenance/legacy/preparations", Some(&record)).await,
                Ok(ref result) if result.status == 200
            )
        });
        final_event["recorded"] = json!(recorded);
        {
            let mut current = preparation();
            current.running = false;
            current.stdin = None;
        }
        deliver(final_event);
    });
    Ok(json!({"started": true}))
}

// legacy_prepare_watch vuelve a escuchar la preparación (al volver a entrar al asistente): sustituye el canal y
// devuelve lo que ya pasó.
#[tauri::command]
pub fn legacy_prepare_watch(
    state: State<'_, TerminalState>,
    channel: Channel<Value>,
) -> Result<Value, String> {
    local_only(&state)?;
    let mut current = preparation();
    current.channel = Some(channel);
    Ok(json!({"running": current.running, "events": current.events}))
}

// legacy_prepare_cancel cierra la entrada estándar del lector: cancela, limpia y avisa CANCELLED por el canal.
#[tauri::command]
pub fn legacy_prepare_cancel(state: State<'_, TerminalState>) -> Result<Value, String> {
    local_only(&state)?;
    let mut current = preparation();
    let cancelling = current.stdin.take().is_some();
    Ok(json!({"cancelling": cancelling}))
}

// run_cleanup ejecuta `legacy-prepare --cleanup` (sin entrada estándar) y devuelve su evento final.
pub(crate) fn run_cleanup(binary: &Path, timeout: Duration) -> Value {
    let mut command = Command::new(binary);
    command
        .args(["legacy-prepare", "--cleanup"])
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::null());
    hidden_process(&mut command);
    let Ok(mut child) = command.spawn() else {
        return json!({"result": "unavailable"});
    };
    // La salida se lee en otro hilo y se espera con plazo: si un proceso hijo del lector (docker) sigue vivo después de
    // detenerlo, la tubería no se cierra y la lectura no debe bloquear al asistente.
    let stdout = child.stdout.take();
    let (sender, receiver) = std::sync::mpsc::channel();
    std::thread::spawn(move || {
        let mut text = String::new();
        if let Some(mut stdout) = stdout {
            let _ = stdout
                .by_ref()
                .take(MAX_EVENT_LINE as u64)
                .read_to_string(&mut text);
        }
        let _ = sender.send(text);
    });
    let started = Instant::now();
    loop {
        match child.try_wait() {
            Ok(Some(_)) => break,
            Ok(None) if started.elapsed() < timeout => {
                std::thread::sleep(Duration::from_millis(100))
            }
            _ => {
                let _ = child.kill();
                let _ = child.wait();
                break;
            }
        }
    }
    let text = receiver
        .recv_timeout(Duration::from_secs(2))
        .unwrap_or_default();
    for line in text.lines().rev() {
        let Ok(value) = serde_json::from_str::<Value>(line) else {
            continue;
        };
        match value.get("event").and_then(Value::as_str) {
            Some("cleanup") => {
                let removed = value.get("removed").cloned().unwrap_or(Value::Null);
                let docker = if value.get("docker").and_then(Value::as_str) == Some("checked") {
                    "checked"
                } else {
                    "unavailable"
                };
                return json!({
                    "result": "cleaned",
                    "removed": {
                        "folders": number(&removed, "folders"),
                        "partials": number(&removed, "partials"),
                        "containers": number(&removed, "containers"),
                        "volumes": number(&removed, "volumes"),
                    },
                    "docker": docker,
                });
            }
            Some("error") => {
                let busy = value.get("code").and_then(Value::as_str) == Some("ALREADY_RUNNING");
                let result = if busy { "busy" } else { "failed" };
                return json!({ "result": result });
            }
            _ => {}
        }
    }
    json!({"result": "failed"})
}

// legacy_prepare_cleanup borra los restos de una preparación que terminó a la fuerza (React lo pide al abrir el
// asistente, sin esperar su resultado para dibujar la pantalla). Si esta aplicación está preparando, no hace nada.
#[tauri::command]
pub async fn legacy_prepare_cleanup(state: State<'_, TerminalState>) -> Result<Value, String> {
    local_only(&state)?;
    if busy_preparing() {
        return Ok(json!({"result": "skipped"}));
    }
    let Ok(binary) = server_binary() else {
        return Ok(json!({"result": "unavailable"}));
    };
    let Ok(_guard) = Guard::acquire(&CLEANING, "") else {
        return Ok(json!({"result": "skipped"}));
    };
    tauri::async_runtime::spawn_blocking(move || run_cleanup(&binary, CLEANUP_TIMEOUT))
        .await
        .map_err(|_| "No se pudieron revisar los restos de una preparación anterior.".to_string())
}

// ---------------------------------------------------------------------------------------------------------------
// Peticiones a Go con las credenciales que custodia Rust (como api_request).

pub(crate) struct Session {
    pub base: String,
    pub device_id: Option<String>,
    pub device_token: Option<String>,
    pub session_token: Option<String>,
}

#[derive(Debug)]
pub(crate) enum Failure {
    // La petición no tuvo respuesta (se puede repetir: Go descarta una parte repetida).
    Network,
    // Respuesta que no es JSON o que excede el tope.
    Invalid(String),
}

fn request(
    client: &reqwest::Client,
    session: &Session,
    method: Method,
    path: &str,
) -> reqwest::RequestBuilder {
    let mut request =
        api::with_client_headers(client.request(method, format!("{}/api/v1/{path}", session.base)));
    if let (Some(id), Some(token)) = (&session.device_id, &session.device_token) {
        request = request
            .header("X-Device-ID", id)
            .header("X-Device-Token", token);
    }
    if let Some(token) = &session.session_token {
        request = request.bearer_auth(token);
    }
    request
}

async fn read_body(response: &mut reqwest::Response, limit: usize) -> Result<Vec<u8>, Failure> {
    let mut bytes = Vec::new();
    loop {
        match response.chunk().await {
            Ok(Some(chunk)) => {
                if bytes.len() + chunk.len() > limit {
                    return Err(Failure::Invalid(
                        "La respuesta excede el límite admitido.".into(),
                    ));
                }
                bytes.extend_from_slice(&chunk);
            }
            Ok(None) => return Ok(bytes),
            Err(_) => return Err(Failure::Network),
        }
    }
}

// send_json envía una petición JSON y devuelve el estado y el cuerpo.
pub(crate) async fn send_json(
    client: &reqwest::Client,
    session: &Session,
    method: Method,
    path: &str,
    body: Option<&Value>,
) -> Result<(u16, Value), Failure> {
    let mut builder = request(client, session, method, path);
    if let Some(body) = body {
        builder = builder.json(body);
    }
    let mut response = builder.send().await.map_err(|_| Failure::Network)?;
    let status = response.status().as_u16();
    let bytes = read_body(&mut response, RESPONSE_MAX_BYTES).await?;
    let value =
        serde_json::from_slice(&bytes).map_err(|_| Failure::Invalid(INVALID_RESPONSE.into()))?;
    Ok((status, value))
}

fn session_of(state: &TerminalState) -> Result<Session, String> {
    let config = state.config.as_ref().map_err(Clone::clone)?;
    let credentials = state.vault()?.read()?;
    Ok(Session {
        base: config.server_url.trim_end_matches('/').to_string(),
        device_id: credentials.device_id,
        device_token: credentials.device_token,
        session_token: credentials.session_token,
    })
}

// settle aplica a una respuesta de Go la misma invalidación de credenciales que api_request y la sanea.
fn settle(state: &TerminalState, status: u16, mut body: Value) -> Result<HTTPResult, String> {
    if status != 200 && api::invalidates_credentials(status, &body) {
        let vault = state.vault()?;
        let mut credentials = vault.read()?;
        credentials.session_token = None;
        let code = body
            .pointer("/error/code")
            .and_then(Value::as_str)
            .unwrap_or("");
        if code == "DEVICE_REVOKED" || code == "DEVICE_REQUIRED" {
            credentials.device_id = None;
            credentials.device_token = None;
        }
        vault.save(credentials)?;
    }
    api::sanitize_response(&mut body);
    Ok(HTTPResult { status, body })
}

async fn call(
    state: &TerminalState,
    method: Method,
    path: &str,
    body: Option<&Value>,
) -> Result<HTTPResult, String> {
    let client = state.client.as_ref().map_err(Clone::clone)?;
    let session = session_of(state)?;
    match send_json(client, &session, method, path, body).await {
        Ok((status, body)) => settle(state, status, body),
        Err(Failure::Network) => Err(state.connection_error()),
        Err(Failure::Invalid(message)) => Err(message),
    }
}

// ---------------------------------------------------------------------------------------------------------------
// Subida del paquete por partes.

static UPLOADING: AtomicBool = AtomicBool::new(false);
static UPLOAD_CANCEL: AtomicBool = AtomicBool::new(false);

// base64 codifica con el alfabeto estándar y relleno (lo que espera encoding/json de Go para []byte).
pub(crate) fn base64(bytes: &[u8]) -> String {
    const ALPHABET: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut output = String::with_capacity(bytes.len().div_ceil(3) * 4);
    for chunk in bytes.chunks(3) {
        let first = chunk[0] as u32;
        let second = chunk.get(1).copied().unwrap_or(0) as u32;
        let third = chunk.get(2).copied().unwrap_or(0) as u32;
        let triple = (first << 16) | (second << 8) | third;
        output.push(ALPHABET[(triple >> 18) as usize & 63] as char);
        output.push(ALPHABET[(triple >> 12) as usize & 63] as char);
        if chunk.len() > 1 {
            output.push(ALPHABET[(triple >> 6) as usize & 63] as char);
        } else {
            output.push('=');
        }
        if chunk.len() > 2 {
            output.push(ALPHABET[triple as usize & 63] as char);
        } else {
            output.push('=');
        }
    }
    output
}

// file_sha256 calcula la huella del archivo completo (antes de crear la subida).
pub(crate) fn file_sha256(path: &Path, cancel: &AtomicBool) -> Result<String, UploadError> {
    let mut file = File::open(path).map_err(|_| unreadable())?;
    let mut digest = Sha256::new();
    let mut buffer = vec![0u8; 1 << 20];
    loop {
        if cancel.load(Ordering::Acquire) {
            return Err(UploadError::Cancelled);
        }
        let read = file.read(&mut buffer).map_err(|_| unreadable())?;
        if read == 0 {
            break;
        }
        digest.update(&buffer[..read]);
    }
    Ok(pairing::to_hex(&digest.finalize()))
}

#[derive(Debug)]
pub(crate) enum UploadError {
    // Go rechazó la petición: su respuesta llega a React como la de api_request.
    Rejected(u16, Value),
    Cancelled,
    Message(String),
}

fn unreadable() -> UploadError {
    UploadError::Message(
        "No se pudo leer el paquete. Si está en una USB, comprueba que siga conectada y vuelve a elegirlo."
            .into(),
    )
}

fn connection_lost() -> UploadError {
    UploadError::Message(
        "Se perdió la conexión al subir el paquete. Vuelve a intentarlo; el paquete no se modificó.".into(),
    )
}

fn invalid_upload() -> UploadError {
    UploadError::Message(INVALID_RESPONSE.into())
}

// read_part lee la parte `index` del archivo (todas de part_size salvo la última).
fn read_part(file: &mut File, length: usize) -> Result<Vec<u8>, UploadError> {
    let mut data = vec![0u8; length];
    file.read_exact(&mut data).map_err(|_| unreadable())?;
    Ok(data)
}

async fn cancel_upload(client: &reqwest::Client, session: &Session, id: &str) {
    let _ = send_json(
        client,
        session,
        Method::POST,
        &format!("maintenance/legacy/uploads/{id}/cancel"),
        None,
    )
    .await;
}

// upload_file sube el paquete a Go: crea la subida con su tamaño y su huella y envía cada parte en orden con la huella de
// sus bytes. Una parte sin respuesta (o que llegó dañada, salvo la última) se reenvía: Go no cambia nada si recibe otra
// vez la última. Si algo falla o se cancela, la subida se cancela en Go. Devuelve la subida completa de Go.
pub(crate) async fn upload_file(
    client: &reqwest::Client,
    session: &Session,
    path: &Path,
    cancel: &AtomicBool,
    progress: &(dyn Fn(u64, u64) + Sync),
) -> Result<Value, UploadError> {
    let size = std::fs::metadata(path).map_err(|_| unreadable())?.len();
    if size == 0 || size > MAX_PACKAGE_BYTES {
        return Err(UploadError::Message(
            "El paquete elegido está vacío o es demasiado grande.".into(),
        ));
    }
    let digest = file_sha256(path, cancel)?;
    let created = send_json(
        client,
        session,
        Method::POST,
        "maintenance/legacy/uploads",
        Some(&json!({"size": size, "sha256": digest})),
    )
    .await
    .map_err(|failure| match failure {
        Failure::Network => connection_lost(),
        Failure::Invalid(message) => UploadError::Message(message),
    })?;
    if created.0 != 200 {
        return Err(UploadError::Rejected(created.0, created.1));
    }
    let upload = created.1;
    let id = upload
        .get("id")
        .and_then(Value::as_str)
        .filter(|id| api::resource_id(Some(id)).is_ok())
        .ok_or_else(invalid_upload)?
        .to_string();
    let part_size = upload.get("part_size").and_then(Value::as_u64).unwrap_or(0);
    let parts = upload.get("parts").and_then(Value::as_u64).unwrap_or(0);
    if part_size == 0 || part_size > MAX_PART_BYTES || parts != size.div_ceil(part_size) {
        cancel_upload(client, session, &id).await;
        return Err(invalid_upload());
    }
    let result = send_parts(
        client, session, path, &id, size, part_size, parts, cancel, progress,
    )
    .await;
    if result.is_err() {
        cancel_upload(client, session, &id).await;
    }
    result
}

#[allow(clippy::too_many_arguments)]
async fn send_parts(
    client: &reqwest::Client,
    session: &Session,
    path: &Path,
    id: &str,
    size: u64,
    part_size: u64,
    parts: u64,
    cancel: &AtomicBool,
    progress: &(dyn Fn(u64, u64) + Sync),
) -> Result<Value, UploadError> {
    let mut file = File::open(path).map_err(|_| unreadable())?;
    let route = format!("maintenance/legacy/uploads/{id}/parts");
    let mut sent = 0u64;
    let mut last = Value::Null;
    progress(0, size);
    for index in 0..parts {
        if cancel.load(Ordering::Acquire) {
            return Err(UploadError::Cancelled);
        }
        let length = part_size.min(size - sent) as usize;
        let data = read_part(&mut file, length)?;
        let body = json!({
            "index": index,
            "data": base64(&data),
            "sha256": pairing::to_hex(&Sha256::digest(&data)),
        });
        let final_part = index + 1 == parts;
        let mut attempt = 0;
        last = loop {
            attempt += 1;
            match send_json(client, session, Method::POST, &route, Some(&body)).await {
                Ok((200, value)) => break value,
                Ok((status, value)) => {
                    let code = value.pointer("/error/code").and_then(Value::as_str);
                    let retry = status == 400
                        && code == Some("LEGACY_UPLOAD_HASH_MISMATCH")
                        && !final_part
                        && attempt < PART_ATTEMPTS;
                    if !retry {
                        return Err(UploadError::Rejected(status, value));
                    }
                }
                Err(Failure::Network) if attempt < PART_ATTEMPTS => {}
                Err(Failure::Network) => return Err(connection_lost()),
                Err(Failure::Invalid(message)) => return Err(UploadError::Message(message)),
            }
            tokio::time::sleep(Duration::from_millis(300 * attempt as u64)).await;
        };
        sent += length as u64;
        progress(sent, size);
    }
    if last.get("status").and_then(Value::as_str) != Some("complete") {
        return Err(invalid_upload());
    }
    Ok(last)
}

struct UploadGuard;

impl UploadGuard {
    fn acquire() -> Result<Self, String> {
        if UPLOADING
            .compare_exchange(false, true, Ordering::AcqRel, Ordering::Acquire)
            .is_err()
        {
            return Err("Ya se está subiendo un paquete.".into());
        }
        UPLOAD_CANCEL.store(false, Ordering::Release);
        Ok(Self)
    }
}

impl Drop for UploadGuard {
    fn drop(&mut self) {
        UPLOADING.store(false, Ordering::Release);
    }
}

// legacy_upload_package sube a Go el paquete elegido. React recibe {result: "uploaded", upload} o {result:
// "cancelled"}; un rechazo de Go llega como el de api_request ({status, body}).
#[tauri::command]
pub async fn legacy_upload_package(
    state: State<'_, TerminalState>,
    channel: Channel<Value>,
) -> Result<HTTPResult, String> {
    local_only(&state)?;
    let path = selection()
        .package
        .clone()
        .ok_or("Elige primero el paquete de migración.")?;
    let _guard = UploadGuard::acquire()?;
    let client = state.client.as_ref().map_err(Clone::clone)?;
    let session = session_of(&state)?;
    let progress = move |sent: u64, total: u64| {
        let percent = if total == 0 { 0 } else { sent * 100 / total };
        let _ = channel.send(json!({"sent": sent, "total": total, "percent": percent}));
    };
    match upload_file(client, &session, &path, &UPLOAD_CANCEL, &progress).await {
        Ok(upload) => Ok(HTTPResult {
            status: 200,
            body: json!({"result": "uploaded", "upload": upload}),
        }),
        Err(UploadError::Cancelled) => Ok(HTTPResult {
            status: 200,
            body: json!({"result": "cancelled"}),
        }),
        Err(UploadError::Rejected(status, body)) => settle(&state, status, body),
        Err(UploadError::Message(message)) => Err(message),
    }
}

#[tauri::command]
pub fn legacy_upload_cancel(state: State<'_, TerminalState>) -> Result<Value, String> {
    local_only(&state)?;
    let uploading = UPLOADING.load(Ordering::Acquire);
    if uploading {
        UPLOAD_CANCEL.store(true, Ordering::Release);
    }
    Ok(json!({"cancelling": uploading}))
}

// ---------------------------------------------------------------------------------------------------------------
// Informe en PDF de un lote.

static SAVING: AtomicBool = AtomicBool::new(false);

#[tauri::command]
pub async fn legacy_save_report(
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
    state: State<'_, TerminalState>,
    batch_id: String,
) -> Result<HTTPResult, String> {
    local_only(&state)?;
    let id = api::resource_id(Some(&batch_id))?.to_ascii_lowercase();
    let _guard = Guard::acquire(&SAVING, "Ya se está guardando el informe.")?;
    let client = state.client.as_ref().map_err(Clone::clone)?;
    let session = session_of(&state)?;
    let mut response = request(
        client,
        &session,
        Method::GET,
        &format!("maintenance/legacy/batches/{id}/report.pdf"),
    )
    .timeout(Duration::from_secs(REPORT_TIMEOUT_SECONDS))
    .send()
    .await
    .map_err(|_| state.connection_error())?;
    let status = response.status().as_u16();
    let header = |name: reqwest::header::HeaderName| {
        response
            .headers()
            .get(name)
            .and_then(|value| value.to_str().ok())
            .map(str::to_string)
    };
    let content_type = header(reqwest::header::CONTENT_TYPE);
    let disposition = header(reqwest::header::CONTENT_DISPOSITION);
    let limit = if status == 200 {
        REPORT_MAX_BYTES
    } else {
        RESPONSE_MAX_BYTES
    };
    let bytes = read_body(&mut response, limit)
        .await
        .map_err(|failure| match failure {
            Failure::Network => "Se interrumpió la respuesta del servidor.".to_string(),
            Failure::Invalid(message) => message,
        })?;
    if status != 200 {
        let body: Value = serde_json::from_slice(&bytes).map_err(|_| INVALID_RESPONSE)?;
        return settle(&state, status, body);
    }
    export::validate_file(export::ExportFormat::Pdf, content_type.as_deref(), &bytes)?;
    let suggested = export::suggested_name(disposition.as_deref(), export::ExportFormat::Pdf);
    let body = tauri::async_runtime::spawn_blocking(move || {
        let target = export::DialogTarget {
            app,
            window,
            title: "Guardar el informe de la migración",
        };
        export::save_with(&target, &suggested, export::ExportFormat::Pdf, &bytes)
    })
    .await
    .map_err(|_| "No se pudo abrir el diálogo para guardar el informe.")??;
    Ok(HTTPResult { status: 200, body })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::VecDeque;
    use std::net::{TcpListener, TcpStream};
    use std::sync::{Arc, Mutex as StdMutex};

    fn temp_dir(name: &str) -> PathBuf {
        let nanos = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let directory = std::env::temp_dir().join(format!("adel-legacy-{name}-{nanos:x}"));
        std::fs::create_dir_all(&directory).unwrap();
        directory
    }

    #[test]
    fn the_server_binary_is_the_one_of_the_product() {
        let metadata: Value =
            serde_json::from_str(include_str!("../../src/generated/product.json")).unwrap();
        assert_eq!(metadata["server_binary"], SERVER_BINARY);
    }

    #[test]
    fn chosen_files_are_validated_and_never_replace() {
        let directory = temp_dir("choose");
        let backup = directory.join("Respaldo Ñandú.BAK");
        std::fs::write(&backup, b"TAPE").unwrap();
        let described = accept_backup(&backup).unwrap();
        assert_eq!(
            described,
            json!({"file_name": "Respaldo Ñandú.BAK", "size": 4})
        );
        assert!(accept_backup(&directory.join("otro.txt")).is_err());
        assert!(accept_backup(&directory.join("falta.bak")).is_err());
        assert!(accept_backup(Path::new("relativo.bak")).is_err());

        // El paquete siempre termina en .adelmig y nunca reemplaza un archivo existente.
        let output = accept_output(directory.join("paquete")).unwrap();
        assert_eq!(output, directory.join("paquete.adelmig"));
        assert_eq!(
            accept_output(directory.join("Paquete.ADELMIG")).unwrap(),
            directory.join("Paquete.ADELMIG")
        );
        std::fs::write(directory.join("existe.adelmig"), b"x").unwrap();
        assert!(accept_output(directory.join("existe.adelmig")).is_err());
        assert!(accept_output(directory.join("existe")).is_err());
        assert!(accept_output(directory.join("no-existe").join("p.adelmig")).is_err());
        assert!(accept_output(PathBuf::from("p.adelmig")).is_err());

        // El paquete para importar: .adelmig con contenido.
        assert_eq!(
            accept_package(&directory.join("existe.adelmig")).unwrap(),
            json!({"file_name": "existe.adelmig", "size": 1})
        );
        std::fs::write(directory.join("vacio.adelmig"), b"").unwrap();
        assert!(accept_package(&directory.join("vacio.adelmig")).is_err());
        assert!(accept_package(&backup).is_err());
        #[cfg(unix)]
        {
            // Un nombre que no es texto nunca se acepta (macOS ni siquiera permite crearlo).
            use std::ffi::OsStr;
            use std::os::unix::ffi::OsStrExt;
            let invalid = directory.join(OsStr::from_bytes(b"\xff.bak"));
            assert!(accept_backup(&invalid).is_err());
            assert!(accept_output(directory.join(OsStr::from_bytes(b"\xff"))).is_err());
        }
        std::fs::remove_dir_all(directory).unwrap();
    }

    #[test]
    fn the_suggested_name_has_the_date() {
        assert_eq!(civil_from_days(0), (1970, 1, 1));
        assert_eq!(civil_from_days(20_773), (2026, 11, 16));
        assert_eq!(civil_from_days(11_016), (2000, 2, 29));
        let name = suggested_output();
        assert!(
            name.starts_with("sistema-anterior-20") && name.ends_with(".adelmig"),
            "{name}"
        );
    }

    #[test]
    fn the_command_line_is_fixed_and_the_passphrase_is_validated() {
        let arguments = prepare_arguments(Path::new("/tmp/r.bak"), Path::new("/tmp/p.adelmig"));
        assert_eq!(
            arguments,
            [
                "legacy-prepare",
                "--backup",
                "/tmp/r.bak",
                "--output",
                "/tmp/p.adelmig",
                "--watch-stdin"
            ]
            .map(OsString::from)
        );
        assert!(validate_passphrase("frase de paso").is_ok());
        assert!(validate_passphrase("ñandúñañ").is_ok());
        assert!(validate_passphrase("corta").is_err());
        assert!(validate_passphrase("dos\nlíneas largas").is_err());
        assert!(validate_passphrase("con\rretorno largo").is_err());
        assert!(validate_passphrase(&"x".repeat(1025)).is_err());
    }

    #[test]
    fn events_reach_react_without_paths_or_details() {
        let step = sanitize_event(
            br#"{"event":"step","step":"restore","status":"finished","message":"Abriendo","seconds":1.5}"#,
        )
        .unwrap();
        assert_eq!(
            step,
            json!({"event":"step","step":"restore","status":"finished","seconds":1.5})
        );
        assert!(
            sanitize_event(br#"{"event":"step","step":"rm -rf","status":"started"}"#).is_none()
        );
        let table = sanitize_event(
            br#"{"event":"table","table":"ventas","status":"done","done":3,"total":52,"rows":10,"table_rows":10}"#,
        )
        .unwrap();
        assert_eq!(
            table,
            json!({"event":"table","status":"done","done":3,"total":52,"rows":10})
        );
        let error = sanitize_event(
            br#"{"event":"error","code":"DOCKER_MISSING","message":"Instala Docker","detail":"/usr/local/bin/docker"}"#,
        )
        .unwrap();
        assert_eq!(
            error,
            json!({"event":"error","code":"DOCKER_MISSING","message":"Instala Docker"})
        );
        assert!(sanitize_event(br#"{"event":"error","code":"rm","message":"x"}"#).is_none());
        let result = sanitize_event(
            br#"{"event":"result","summary":{"output":"/Users/x/Documents/p.adelmig","package_bytes":32885,
            "content_fingerprint":"ABCDEF0123456789abcdef0123456789ABCDEF0123456789abcdef0123456789","verified":true,
            "backup":{"file":"/Users/x/possys.bak","bytes":6000,"database":"possys","started_at":"2026-11-12T22:05:31.000",
            "finished_at":"2026-11-12T22:05:33.000","server_version":"11","sets":1},
            "sales":{"first":"2018-03-01T12:00:00.000","last":"2026-11-12T21:00:00.000"},
            "tables":[{"name":"ventas","rows":240},{"name":"../x","rows":1}],
            "timings":{"read":14.2,"bad key":1},"previous_leftovers":{"folders":0}}}"#,
        )
        .unwrap();
        let text = result.to_string();
        assert!(
            !text.contains("/Users") && !text.contains("possys"),
            "{text}"
        );
        assert_eq!(result["fingerprint"].as_str().unwrap().len(), 64);
        assert_eq!(result["tables"], json!([{"name":"ventas","rows":240}]));
        assert_eq!(result["timings"], json!({"read":14.2}));
        assert_eq!(result["sales"]["first"], "2018-03-01T12:00:00.000");
        assert!(sanitize_event(br#"{"event":"cleanup"}"#).is_none());
        assert!(sanitize_event(b"no es json").is_none());

        // Lo que se anota en Go: solo los campos que acepta la ruta de la preparación.
        let record = preparation_record(&result);
        assert_eq!(record["succeeded"], true);
        assert_eq!(record["backup_taken_at"], "2026-11-12T22:05:33.000");
        assert_eq!(record["tables"], json!({"ventas": 240}));
        assert_eq!(record["timings_ms"], json!({"read": 14200}));
        assert_eq!(record["package_bytes"], 32885);
        let failed = preparation_record(&error);
        assert_eq!(
            failed,
            json!({"succeeded": false, "error_code": "DOCKER_MISSING", "tables": {}, "timings_ms": {}})
        );
        assert_eq!(
            safe_text(Some(&json!("2026-11-12T22:05:33.000"))),
            "2026-11-12T22:05:33.000"
        );
        assert_eq!(safe_text(Some(&json!("Ñ"))), "");
        assert_eq!(safe_text(Some(&json!("x".repeat(41)))), "");
    }

    #[test]
    fn the_snapshot_keeps_steps_and_only_the_last_progress() {
        let mut events = Vec::new();
        for event in [
            json!({"event":"step","step":"read","status":"started"}),
            json!({"event":"table","done":1,"total":52}),
            json!({"event":"table","done":2,"total":52}),
            json!({"event":"progress","step":"read","current":1,"total":2}),
            json!({"event":"progress","step":"read","current":2,"total":2}),
        ] {
            remember(&mut events, &event);
        }
        assert_eq!(events.len(), 3);
        assert_eq!(events[1]["done"], 2);
        assert_eq!(events[2]["current"], 2);
    }

    #[test]
    fn base64_matches_the_standard() {
        for (input, expected) in [
            ("", ""),
            ("f", "Zg=="),
            ("fo", "Zm8="),
            ("foo", "Zm9v"),
            ("foob", "Zm9vYg=="),
            ("fooba", "Zm9vYmE="),
            ("foobar", "Zm9vYmFy"),
        ] {
            assert_eq!(base64(input.as_bytes()), expected);
        }
        assert_eq!(base64(&[0xfb, 0xff, 0xbf]), "+/+/");
    }

    // Lector simulado: un script que hace lo que legacy-prepare (lee la frase de la primera línea de la entrada
    // estándar, emite eventos y, si se le pide, espera a que se cierre la entrada para cancelar).
    #[cfg(unix)]
    fn fake_reader(directory: &Path, body: &str) -> PathBuf {
        use std::os::unix::fs::PermissionsExt;
        let script = directory.join("ad-el-pos-server");
        let text = format!(
            "#!/bin/sh\nhere=$(dirname \"$0\")\nprintf '%s\\n' \"$@\" > \"$here/args.txt\"\n{body}\n"
        );
        std::fs::write(&script, text).unwrap();
        std::fs::set_permissions(&script, std::fs::Permissions::from_mode(0o755)).unwrap();
        script
    }

    #[cfg(unix)]
    #[test]
    fn the_reader_gets_the_passphrase_by_stdin_and_reports_events() {
        let directory = temp_dir("reader");
        let binary = fake_reader(
            &directory,
            r#"IFS= read -r frase
printf '%s' "$frase" > "$here/stdin.txt"
echo '{"event":"step","step":"read","status":"started","message":"Leyendo"}'
echo 'línea que no es json'
echo '{"event":"table","table":"ventas","status":"done","done":1,"total":2,"rows":5}'
echo '{"event":"result","summary":{"output":"/secreto/p.adelmig","package_bytes":10,"tables":[{"name":"ventas","rows":5}],"timings":{"read":0.5}}}'
exit 0"#,
        );
        let backup = directory.join("r.bak");
        let output = directory.join("p.adelmig");
        let (child, stdin) = start_reader(&binary, &backup, &output, "frase secreta 9H").unwrap();
        let mut delivered = Vec::new();
        let last = follow_reader(child, |event| delivered.push(event));
        drop(stdin);
        assert_eq!(delivered.len(), 2);
        assert_eq!(delivered[0]["step"], "read");
        assert_eq!(delivered[1]["done"], 1);
        assert_eq!(last["event"], "result");
        assert_eq!(last["tables"][0]["rows"], 5);
        assert!(!last.to_string().contains("secreto"));
        // La frase llegó por la entrada estándar y nunca en los argumentos.
        assert_eq!(
            std::fs::read_to_string(directory.join("stdin.txt")).unwrap(),
            "frase secreta 9H"
        );
        let arguments = std::fs::read_to_string(directory.join("args.txt")).unwrap();
        assert!(!arguments.contains("frase"));
        assert_eq!(
            arguments.lines().collect::<Vec<_>>(),
            [
                "legacy-prepare",
                "--backup",
                backup.to_str().unwrap(),
                "--output",
                output.to_str().unwrap(),
                "--watch-stdin"
            ]
        );
        std::fs::remove_dir_all(directory).unwrap();
    }

    #[cfg(unix)]
    #[test]
    fn closing_stdin_cancels_the_preparation() {
        let directory = temp_dir("cancel");
        let binary = fake_reader(
            &directory,
            r#"IFS= read -r frase
echo '{"event":"step","step":"start","status":"started"}'
while IFS= read -r resto; do :; done
echo '{"event":"error","code":"CANCELLED","message":"Se canceló la preparación del paquete. No quedó nada a medias.","detail":"x"}'
exit 1"#,
        );
        let (child, stdin) = start_reader(
            &binary,
            &directory.join("r.bak"),
            &directory.join("p.adelmig"),
            "frase de paso",
        )
        .unwrap();
        let (sender, receiver) = std::sync::mpsc::channel();
        let follower = std::thread::spawn(move || {
            follow_reader(child, |event| {
                let _ = sender.send(event);
            })
        });
        // Al ver el primer paso se cancela cerrando la entrada estándar, como legacy_prepare_cancel.
        let first = receiver.recv_timeout(Duration::from_secs(10)).unwrap();
        assert_eq!(first["step"], "start");
        drop(stdin);
        let last = follower.join().unwrap();
        assert_eq!(last["code"], "CANCELLED");
        assert!(last.get("detail").is_none());
        std::fs::remove_dir_all(directory).unwrap();
    }

    #[cfg(unix)]
    #[test]
    fn a_reader_that_dies_without_result_is_an_error() {
        let directory = temp_dir("dies");
        let binary = fake_reader(&directory, "exit 3");
        let (child, _stdin) = start_reader(
            &binary,
            &directory.join("r.bak"),
            &directory.join("p.adelmig"),
            "frase de paso",
        )
        .unwrap();
        let last = follow_reader(child, |_| {});
        assert_eq!(last["code"], "INTERNAL");
        std::fs::remove_dir_all(directory).unwrap();
    }

    #[cfg(unix)]
    #[test]
    fn cleanup_reports_what_was_removed() {
        let directory = temp_dir("cleanup");
        let binary = fake_reader(
            &directory,
            r#"[ "$2" = "--cleanup" ] || exit 9
echo '{"event":"cleanup","removed":{"folders":1,"partials":0,"containers":1,"volumes":1},"docker":"checked"}'"#,
        );
        assert_eq!(
            run_cleanup(&binary, Duration::from_secs(10)),
            json!({"result":"cleaned","removed":{"folders":1,"partials":0,"containers":1,"volumes":1},"docker":"checked"})
        );
        let busy = fake_reader(
            &directory,
            r#"echo '{"event":"error","code":"ALREADY_RUNNING","message":"Ya se está preparando"}'; exit 1"#,
        );
        assert_eq!(
            run_cleanup(&busy, Duration::from_secs(10)),
            json!({"result":"busy"})
        );
        // Una limpieza que no termina se detiene al vencer el plazo.
        let hung = fake_reader(&directory, "sleep 30");
        let started = Instant::now();
        assert_eq!(
            run_cleanup(&hung, Duration::from_millis(300)),
            json!({"result":"failed"})
        );
        assert!(started.elapsed() < Duration::from_secs(10));
        assert_eq!(
            run_cleanup(&directory.join("no-existe"), Duration::from_secs(1)),
            json!({"result":"unavailable"})
        );
        std::fs::remove_dir_all(directory).unwrap();
    }

    // Servidor de prueba en 127.0.0.1 que imita las rutas de la subida de Go: guarda cada petición y responde con lo que
    // decide `respond` (None = cierra la conexión sin responder, como un corte de red).
    type Responder = dyn Fn(&str, &str, &Value, usize) -> Option<(u16, Value)> + Send + Sync;

    struct FakeGo {
        port: u16,
        requests: Arc<StdMutex<Vec<(String, String, Value)>>>,
    }

    fn read_request(stream: &mut TcpStream) -> Option<(String, String, Value, bool)> {
        let mut reader = BufReader::new(stream.try_clone().ok()?);
        let mut line = String::new();
        reader.read_line(&mut line).ok()?;
        let mut parts = line.split_whitespace();
        let method = parts.next()?.to_string();
        let path = parts.next()?.to_string();
        let mut length = 0usize;
        let mut authorized = false;
        loop {
            let mut header = String::new();
            reader.read_line(&mut header).ok()?;
            let header = header.trim_end();
            if header.is_empty() {
                break;
            }
            let lower = header.to_ascii_lowercase();
            if let Some(value) = lower.strip_prefix("content-length:") {
                length = value.trim().parse().ok()?;
            }
            if lower.starts_with("authorization: bearer sesion-9h") {
                authorized = true;
            }
        }
        let mut body = vec![0u8; length];
        reader.read_exact(&mut body).ok()?;
        let value = if body.is_empty() {
            Value::Null
        } else {
            serde_json::from_slice(&body).ok()?
        };
        Some((method, path, value, authorized))
    }

    fn fake_go(respond: Arc<Responder>) -> FakeGo {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let port = listener.local_addr().unwrap().port();
        let requests = Arc::new(StdMutex::new(Vec::new()));
        let recorded = Arc::clone(&requests);
        std::thread::spawn(move || {
            for stream in listener.incoming() {
                let Ok(mut stream) = stream else {
                    continue;
                };
                let Some((method, path, body, authorized)) = read_request(&mut stream) else {
                    continue;
                };
                assert!(authorized, "la petición debe llevar la sesión");
                let count = {
                    let mut list = recorded.lock().unwrap();
                    list.push((method.clone(), path.clone(), body.clone()));
                    list.len()
                };
                let Some((status, reply)) = respond(&method, &path, &body, count) else {
                    drop(stream);
                    continue;
                };
                let text = reply.to_string();
                let _ = write!(
                    stream,
                    "HTTP/1.1 {status} X\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{text}",
                    text.len()
                );
                let _ = stream.flush();
            }
        });
        FakeGo { port, requests }
    }

    fn session(port: u16) -> Session {
        Session {
            base: format!("http://127.0.0.1:{port}"),
            device_id: Some("dispositivo".into()),
            device_token: Some("token".into()),
            session_token: Some("sesion-9h".into()),
        }
    }

    fn client() -> reqwest::Client {
        reqwest::Client::builder()
            .timeout(Duration::from_secs(10))
            .no_proxy()
            .build()
            .unwrap()
    }

    const UPLOAD_ID: &str = "8f0e2a52-6f3b-4c7a-9a59-0f1e2d3c4b5a";

    // upload_go responde como Go: crea la subida con partes de `part` bytes y acepta cada parte en orden.
    fn upload_go(size: u64, part: u64, drop_request: Option<usize>) -> Arc<Responder> {
        let state = Arc::new(StdMutex::new(VecDeque::<u64>::new()));
        Arc::new(move |method, path, body, count| {
            if Some(count) == drop_request {
                return None;
            }
            if method == "POST" && path == "/api/v1/maintenance/legacy/uploads" {
                let parts = size.div_ceil(part);
                return Some((
                    200,
                    json!({"id": UPLOAD_ID, "status": "receiving", "size": body["size"], "sha256": body["sha256"],
                        "part_size": part, "parts": parts, "received_parts": 0, "received_bytes": 0}),
                ));
            }
            if path.ends_with("/cancel") {
                return Some((200, json!({"cancelled": true})));
            }
            let index = body["index"].as_u64().unwrap();
            let mut received = state.lock().unwrap();
            if !received.contains(&index) {
                received.push_back(index);
            }
            let parts = size.div_ceil(part);
            let status = if received.len() as u64 == parts {
                "complete"
            } else {
                "receiving"
            };
            Some((
                200,
                json!({"id": UPLOAD_ID, "status": status, "received_parts": received.len()}),
            ))
        })
    }

    fn package(directory: &Path, size: usize) -> PathBuf {
        let path = directory.join("paquete.adelmig");
        let bytes: Vec<u8> = (0..size).map(|index| (index * 7 % 251) as u8).collect();
        std::fs::write(&path, bytes).unwrap();
        path
    }

    fn decode_base64(text: &str) -> Vec<u8> {
        let alphabet = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
        let mut bits = 0u32;
        let mut count = 0;
        let mut output = Vec::new();
        for character in text.bytes().filter(|character| *character != b'=') {
            let value = alphabet.iter().position(|item| *item == character).unwrap() as u32;
            bits = (bits << 6) | value;
            count += 6;
            if count >= 8 {
                count -= 8;
                output.push((bits >> count) as u8 & 0xff);
            }
        }
        output
    }

    #[test]
    fn the_package_is_uploaded_in_ordered_parts_with_their_hashes() {
        let directory = temp_dir("upload");
        let path = package(&directory, 2500);
        let go = fake_go(upload_go(2500, 1000, None));
        let seen = StdMutex::new(Vec::new());
        let progress = |sent: u64, total: u64| seen.lock().unwrap().push((sent, total));
        let cancel = AtomicBool::new(false);
        let upload = tauri::async_runtime::block_on(upload_file(
            &client(),
            &session(go.port),
            &path,
            &cancel,
            &progress,
        ))
        .unwrap();
        assert_eq!(upload["status"], "complete");
        let requests = go.requests.lock().unwrap().clone();
        assert_eq!(requests.len(), 4);
        let content = std::fs::read(&path).unwrap();
        assert_eq!(requests[0].1, "/api/v1/maintenance/legacy/uploads");
        assert_eq!(requests[0].2["size"], 2500);
        assert_eq!(
            requests[0].2["sha256"],
            pairing::to_hex(&Sha256::digest(&content))
        );
        let mut rebuilt = Vec::new();
        for (index, (method, route, body)) in requests[1..].iter().enumerate() {
            assert_eq!(method, "POST");
            assert_eq!(
                route,
                &format!("/api/v1/maintenance/legacy/uploads/{UPLOAD_ID}/parts")
            );
            assert_eq!(body["index"], index);
            let data = decode_base64(body["data"].as_str().unwrap());
            assert_eq!(body["sha256"], pairing::to_hex(&Sha256::digest(&data)));
            rebuilt.extend(data);
        }
        assert_eq!(rebuilt, content);
        assert_eq!(
            seen.lock().unwrap().clone(),
            [(0, 2500), (1000, 2500), (2000, 2500), (2500, 2500)]
        );
        std::fs::remove_dir_all(directory).unwrap();
    }

    #[test]
    fn a_part_without_answer_is_sent_again() {
        let directory = temp_dir("retry");
        let path = package(&directory, 1500);
        // La segunda petición (la parte 0) se corta sin respuesta: se reenvía la misma parte.
        let go = fake_go(upload_go(1500, 1000, Some(2)));
        let cancel = AtomicBool::new(false);
        let upload = tauri::async_runtime::block_on(upload_file(
            &client(),
            &session(go.port),
            &path,
            &cancel,
            &|_, _| {},
        ))
        .unwrap();
        assert_eq!(upload["status"], "complete");
        let requests = go.requests.lock().unwrap().clone();
        let indexes: Vec<_> = requests[1..]
            .iter()
            .map(|request| request.2["index"].clone())
            .collect();
        assert_eq!(indexes, [json!(0), json!(0), json!(1)]);
        assert_eq!(requests[1].2, requests[2].2);
        std::fs::remove_dir_all(directory).unwrap();
    }

    #[test]
    fn a_rejection_or_a_cancellation_cancels_the_upload_in_go() {
        let directory = temp_dir("reject");
        let path = package(&directory, 1500);
        let base = upload_go(1500, 1000, None);
        let rejecting: Arc<Responder> = Arc::new(move |method, path, body, count| {
            if path.ends_with("/parts") && body["index"] == 1 {
                return Some((
                    400,
                    json!({"error": {"code": "LEGACY_UPLOAD_HASH_MISMATCH", "message": "El paquete llegó dañado."}}),
                ));
            }
            base(method, path, body, count)
        });
        let go = fake_go(rejecting);
        let cancel = AtomicBool::new(false);
        let failure = tauri::async_runtime::block_on(upload_file(
            &client(),
            &session(go.port),
            &path,
            &cancel,
            &|_, _| {},
        ))
        .unwrap_err();
        match failure {
            UploadError::Rejected(400, body) => {
                assert_eq!(body["error"]["code"], "LEGACY_UPLOAD_HASH_MISMATCH")
            }
            other => panic!("{other:?}"),
        }
        let requests = go.requests.lock().unwrap().clone();
        // La última parte con la huella del paquete completo no se reenvía; la subida se cancela.
        assert_eq!(requests.len(), 4);
        assert!(requests[3].1.ends_with(&format!("{UPLOAD_ID}/cancel")));

        // Cancelar a la mitad: no se envían más partes y se cancela la subida.
        let go = fake_go(upload_go(1500, 1000, None));
        let cancel = AtomicBool::new(false);
        let progress = |sent: u64, _total: u64| {
            if sent >= 1000 {
                cancel.store(true, Ordering::Release);
            }
        };
        let failure = tauri::async_runtime::block_on(upload_file(
            &client(),
            &session(go.port),
            &path,
            &cancel,
            &progress,
        ))
        .unwrap_err();
        assert!(matches!(failure, UploadError::Cancelled));
        let requests = go.requests.lock().unwrap().clone();
        assert_eq!(requests.len(), 3);
        assert!(requests[2].1.ends_with("/cancel"));
        std::fs::remove_dir_all(directory).unwrap();
    }

    #[test]
    fn a_go_rejection_of_the_upload_is_returned_as_is() {
        let directory = temp_dir("too-large");
        let path = package(&directory, 10);
        let go = fake_go(Arc::new(|_, _, _, _| {
            Some((
                403,
                json!({"error": {"code": "LOCAL_REQUIRED", "message": "Solo desde la computadora principal."}}),
            ))
        }));
        let cancel = AtomicBool::new(false);
        let failure = tauri::async_runtime::block_on(upload_file(
            &client(),
            &session(go.port),
            &path,
            &cancel,
            &|_, _| {},
        ))
        .unwrap_err();
        assert!(matches!(failure, UploadError::Rejected(403, _)));
        assert_eq!(go.requests.lock().unwrap().len(), 1);
        std::fs::remove_dir_all(directory).unwrap();
    }

    #[test]
    fn the_webview_gets_the_new_operations_but_no_dialog_or_filesystem_permissions() {
        let permissions = include_str!("../permissions/diagnostics.toml");
        for command in [
            "legacy_choose_backup",
            "legacy_choose_package_output",
            "legacy_choose_package",
            "legacy_selection",
            "legacy_prepare_start",
            "legacy_prepare_watch",
            "legacy_prepare_cancel",
            "legacy_prepare_cleanup",
            "legacy_upload_package",
            "legacy_upload_cancel",
            "legacy_save_report",
        ] {
            assert!(permissions.contains(&format!("\"{command}\"")), "{command}");
        }
        let capability = include_str!("../capabilities/main.json");
        for forbidden in ["dialog:", "fs:", "shell:", "allow-open", "allow-execute"] {
            assert!(!capability.contains(forbidden), "{forbidden}");
            assert!(!permissions.contains(forbidden), "{forbidden}");
        }
    }
}
