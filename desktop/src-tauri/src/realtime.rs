use crate::TerminalState;
use serde_json::{json, Value};
use std::future::Future;
use std::sync::atomic::Ordering;
use std::time::{Duration, Instant};
use tauri::{ipc::Channel, State};

// Canal de eventos del escritorio (GET /api/v1/events, SSE).
//
// Etapa K1 (Fase E, §7 de la especificación y decisión K1-D10): Go renueva el canal cada 30 s a propósito (termina el
// cuerpo y el cliente vuelve a abrirlo desde su cursor; así revisa otra vez sesión, permisos y estaciones). Hasta la
// 0.15.0 este bucle trataba ese fin normal como una falla: mandaba «reconnecting», esperaba 500 ms y al volver mandaba
// «connected», y la barra mostraba «Reconectando» 1 o 2 segundos cada 30 s (y todas las pantallas volvían a consultar).
// Ahora:
//   - el fin normal de un canal que respondió 200 (el cuerpo terminó sin error después de un rato) se vuelve a abrir al
//     momento, desde el mismo cursor, sin mandar nada y sin esperar;
//   - solo una falla real cuenta: no conectar, que el servidor no responda a tiempo, un cuerpo cortado, un canal mudo
//     más de STALL_TIMEOUT, un 5xx o un aviso «interrupted» de Go. Entonces se manda «reconnecting» y se espera con
//     espera creciente; al volver a abrir se manda «connected» (una sola vez por cada falla);
//   - la tolerancia (que una falla aislada no se note) la decide React con el estado único de conexión
//     (navigation/serverLink.ts), igual que Android desde la A3.

// Un canal que termina normalmente antes de esto no cuenta como renovación sino como falla: así un servidor que cerrara
// el canal enseguida no produce un bucle sin espera.
const RENEWAL_MINIMUM: Duration = Duration::from_secs(5);
// Tiempo para recibir la respuesta al abrir el canal (con la computadora principal detenida, la conexión TCP puede
// abrirse y la respuesta no llegar nunca).
const HEAD_TIMEOUT: Duration = Duration::from_secs(10);
// Go manda un mensaje cada 750 ms: un canal abierto que no manda nada en este tiempo está caído.
const STALL_TIMEOUT: Duration = Duration::from_secs(10);
// Plazo total de una petición del canal (Go lo cierra a los 30 s).
const STREAM_TIMEOUT: Duration = Duration::from_secs(40);
const FIRST_DELAY: Duration = Duration::from_millis(500);
const MAX_DELAY: Duration = Duration::from_millis(15000);
// Tamaño máximo de un mensaje sin terminar.
const BUFFER_LIMIT: usize = 262144;

fn parse_frame(frame: &[u8]) -> Result<Option<Value>, String> {
    let text = std::str::from_utf8(frame).map_err(|_| "Evento con codificación inválida.")?;
    let data = text
        .lines()
        .filter_map(|line| line.strip_prefix("data: "))
        .collect::<Vec<_>>()
        .join("\n");
    if data.is_empty() {
        return Ok(None);
    }
    serde_json::from_str(&data)
        .map(Some)
        .map_err(|_| "Evento con contrato inválido.".into())
}

// Tiempos del bucle (las pruebas los acortan).
#[derive(Clone, Copy)]
pub(crate) struct StreamTiming {
    pub renewal_minimum: Duration,
    pub head_timeout: Duration,
    pub stall_timeout: Duration,
    pub first_delay: Duration,
    pub max_delay: Duration,
}

impl StreamTiming {
    pub(crate) const PRODUCTION: StreamTiming = StreamTiming {
        renewal_minimum: RENEWAL_MINIMUM,
        head_timeout: HEAD_TIMEOUT,
        stall_timeout: STALL_TIMEOUT,
        first_delay: FIRST_DELAY,
        max_delay: MAX_DELAY,
    };
}

// Lo que pasó al intentar abrir el canal.
pub(crate) enum Attempt {
    // Go respondió (con cualquier estado).
    Response(reqwest::Response),
    // No se llegó a Go (conexión, plazo).
    Failed,
    // No hay credenciales o la vista se cerró: el bucle termina sin avisar nada.
    Stop,
}

// Cómo terminó la lectura de un canal abierto.
enum StreamEnd {
    // El cuerpo terminó sin error. interrupted: Go mandó «interrupted» antes de cerrar (una falla del lado de Go).
    Ended { interrupted: bool },
    // Cuerpo cortado, mensaje demasiado grande o canal mudo.
    Broken,
    // La vista ya no escucha.
    Stopped,
}

// Qué decidir después de cada vuelta del canal.
#[derive(Debug, PartialEq)]
enum Outcome {
    Renewed,
    Failed,
}

// ChannelLink decide qué mensajes de conexión se mandan: «connected» la primera vez y después de cada falla, nunca en la
// renovación normal; «reconnecting» en cada falla, con su espera creciente.
pub(crate) struct ChannelLink {
    announced: bool,
    delay: Duration,
    timing: StreamTiming,
}

impl ChannelLink {
    pub(crate) fn new(timing: StreamTiming) -> Self {
        ChannelLink {
            announced: false,
            delay: timing.first_delay,
            timing,
        }
    }

    // El canal respondió 200: «connected» solo si no se había anunciado (al empezar o después de una falla).
    pub(crate) fn opened(&mut self) -> Option<Value> {
        self.delay = self.timing.first_delay;
        if self.announced {
            return None;
        }
        self.announced = true;
        Some(json!({"connection": "connected"}))
    }

    // Una falla real: «reconnecting» y cuánto esperar antes del siguiente intento.
    pub(crate) fn failed(&mut self) -> (Value, Duration) {
        self.announced = false;
        let delay = self.delay;
        self.delay = (self.delay * 2).min(self.timing.max_delay);
        (json!({"connection": "reconnecting"}), delay)
    }
}

// Lee el canal abierto: entrega cada mensaje y actualiza el cursor.
async fn read_stream(
    mut response: reqwest::Response,
    cursor: &mut i64,
    sink: &impl Fn(Value) -> bool,
    active: &impl Fn() -> bool,
    timing: StreamTiming,
) -> StreamEnd {
    let mut buffer = Vec::new();
    let mut interrupted = false;
    while active() {
        let chunk = match tokio::time::timeout(timing.stall_timeout, response.chunk()).await {
            Err(_) => return StreamEnd::Broken,
            Ok(Err(_)) => return StreamEnd::Broken,
            Ok(Ok(None)) => return StreamEnd::Ended { interrupted },
            Ok(Ok(Some(chunk))) => chunk,
        };
        if buffer.len() + chunk.len() > BUFFER_LIMIT {
            return StreamEnd::Broken;
        }
        buffer.extend_from_slice(&chunk);
        while let Some(end) = buffer.windows(2).position(|pair| pair == b"\n\n") {
            let frame = buffer.drain(..end + 2).collect::<Vec<_>>();
            if let Ok(Some(message)) = parse_frame(&frame) {
                if let Some(next) = message.get("cursor").and_then(Value::as_i64) {
                    *cursor = next;
                }
                // Go avisa «interrupted» (con error y status) cuando la lectura falla y después cierra el canal.
                if message.get("error").is_some() {
                    interrupted = true;
                }
                if !sink(message) {
                    return StreamEnd::Stopped;
                }
            }
        }
    }
    StreamEnd::Stopped
}

// follow mantiene el canal abierto mientras active() lo pida. open(cursor) abre una petición del canal desde el cursor;
// sink entrega cada mensaje a la vista (falso: la vista ya no escucha).
pub(crate) async fn follow<Open, Opening>(
    after: i64,
    mut open: Open,
    sink: impl Fn(Value) -> bool,
    active: impl Fn() -> bool,
    timing: StreamTiming,
) where
    Open: FnMut(i64) -> Opening,
    Opening: Future<Output = Attempt>,
{
    let mut cursor = after;
    let mut link = ChannelLink::new(timing);
    while active() {
        let attempt = match tokio::time::timeout(timing.head_timeout, open(cursor)).await {
            Ok(attempt) => attempt,
            Err(_) => Attempt::Failed,
        };
        let outcome = match attempt {
            Attempt::Stop => break,
            Attempt::Failed => Outcome::Failed,
            Attempt::Response(response) => {
                let status = response.status().as_u16();
                if status == 401 || status == 403 {
                    let _ = sink(json!({"connection": "unauthorized", "status": status}));
                    break;
                }
                if !response.status().is_success() {
                    Outcome::Failed
                } else {
                    if let Some(message) = link.opened() {
                        if !sink(message) {
                            break;
                        }
                    }
                    let started = Instant::now();
                    match read_stream(response, &mut cursor, &sink, &active, timing).await {
                        StreamEnd::Stopped => break,
                        StreamEnd::Broken => Outcome::Failed,
                        StreamEnd::Ended { interrupted } => {
                            if interrupted || started.elapsed() < timing.renewal_minimum {
                                Outcome::Failed
                            } else {
                                Outcome::Renewed
                            }
                        }
                    }
                }
            }
        };
        if !active() {
            break;
        }
        if outcome == Outcome::Renewed {
            // La renovación normal: se vuelve a abrir al momento y en silencio.
            continue;
        }
        let (message, delay) = link.failed();
        if !sink(message) {
            break;
        }
        tokio::time::sleep(delay).await;
    }
}

#[tauri::command]
pub fn events_stop(state: State<'_, TerminalState>, subscription_id: u64) {
    // Una limpieza atrasada de React no puede cerrar la suscripción de la siguiente vista.
    let _ = state.events_generation.compare_exchange(
        subscription_id,
        subscription_id + 1,
        Ordering::SeqCst,
        Ordering::SeqCst,
    );
}

#[tauri::command]
pub fn events_start(
    state: State<'_, TerminalState>,
    channel: Channel<Value>,
    after: Option<i64>,
    station_id: Option<String>,
) -> Result<u64, String> {
    if after.unwrap_or(0) < 0 {
        return Err("Cursor no válido.".into());
    }
    if let Some(identifier) = station_id.as_deref() {
        crate::api::resource_id(Some(identifier))?;
    }
    state.vault()?;
    state.config.as_ref().map_err(Clone::clone)?;
    let client = state.client.as_ref().map_err(Clone::clone)?.clone();
    // Etapa 9G (fase C): la dirección se lee en cada intento; si el servidor emparejado cambió de IP, el canal
    // lo vuelve a encontrar (discovery.rs) y sigue desde el mismo cursor.
    let locator = state.locator.clone();
    let vault = state.vault.clone();
    let generation = state.events_generation.clone();
    let subscription_id = generation.fetch_add(1, Ordering::SeqCst) + 1;
    tauri::async_runtime::spawn(async move {
        let active_generation = generation.clone();
        let active = move || active_generation.load(Ordering::SeqCst) == subscription_id;
        let open = |cursor: i64| {
            let client = client.clone();
            let locator = locator.clone();
            let vault = vault.clone();
            let station_id = station_id.clone();
            async move {
                let Some(Ok(storage)) = vault.get() else {
                    return Attempt::Stop;
                };
                let Ok(credentials) = storage.read() else {
                    return Attempt::Stop;
                };
                let (Some(device), Some(secret), Some(session)) = (
                    credentials.device_id,
                    credentials.device_token,
                    credentials.session_token,
                ) else {
                    return Attempt::Stop;
                };
                // El destino y las credenciales se obtienen únicamente del perfil nativo.
                let base = locator.base();
                let mut request =
                    crate::api::with_client_headers(client.get(format!("{base}/api/v1/events")))
                        .header("X-Device-ID", device)
                        .header("X-Device-Token", secret)
                        .bearer_auth(session)
                        .header("Accept", "text/event-stream")
                        .query(&[("after", cursor.to_string())])
                        .timeout(STREAM_TIMEOUT);
                if let Some(station) = &station_id {
                    request = request.query(&[("station_id", station)]);
                }
                match request.send().await {
                    Ok(response) => Attempt::Response(response),
                    Err(error) => {
                        if error.is_connect() {
                            let _ = locator.relocate(&base).await;
                        }
                        Attempt::Failed
                    }
                }
            }
        };
        let sink = |message: Value| {
            // Etapa 9G (fase E): cualquier cambio puede traer un trabajo para las impresoras de esta terminal; el
            // agente lo revisa sin esperar su consulta de respaldo.
            if message.get("connection").is_none() {
                crate::print_agent::nudge();
            }
            channel.send(message).is_ok()
        };
        follow(
            after.unwrap_or(0),
            open,
            sink,
            active,
            StreamTiming::PRODUCTION,
        )
        .await;
    });
    Ok(subscription_id)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::{Read, Write};
    use std::net::{TcpListener, TcpStream};
    use std::sync::atomic::{AtomicBool, AtomicUsize};
    use std::sync::{Arc, Mutex};
    use std::thread;

    #[test]
    fn parses_utf8_replay_and_ignores_heartbeat() {
        let message = parse_frame("id: 5\nevent: changes\ndata: {\"cursor\":5,\"events\":[{\"kind\":\"preparación\"}]}\n\n".as_bytes()).unwrap().unwrap();
        assert_eq!(message["cursor"], 5);
        assert!(parse_frame(b": keepalive\n\n").unwrap().is_none());
        assert!(parse_frame(b"data: invalid\n\n").is_err());
    }

    #[test]
    fn connected_only_first_and_after_a_failure() {
        let mut link = ChannelLink::new(StreamTiming::PRODUCTION);
        assert_eq!(link.opened(), Some(json!({"connection": "connected"})));
        // Las renovaciones no vuelven a anunciar nada.
        assert_eq!(link.opened(), None);
        assert_eq!(link.opened(), None);
        let (message, delay) = link.failed();
        assert_eq!(message, json!({"connection": "reconnecting"}));
        assert_eq!(delay, FIRST_DELAY);
        let (_, second) = link.failed();
        assert_eq!(second, FIRST_DELAY * 2);
        assert_eq!(link.opened(), Some(json!({"connection": "connected"})));
        // El éxito reinicia la espera.
        assert_eq!(link.failed().1, FIRST_DELAY);
        for _ in 0..10 {
            link.failed();
        }
        assert_eq!(link.failed().1, MAX_DELAY);
    }

    // Servidor de prueba en 127.0.0.1: cada conexión la atiende behave(número de petición, flujo, cursor pedido).
    struct TestServer {
        port: u16,
        requests: Arc<AtomicUsize>,
        cursors: Arc<Mutex<Vec<String>>>,
    }

    fn serve(behave: impl Fn(usize, &mut TcpStream) + Send + Sync + 'static) -> TestServer {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let port = listener.local_addr().unwrap().port();
        let requests = Arc::new(AtomicUsize::new(0));
        let cursors = Arc::new(Mutex::new(Vec::new()));
        let behave = Arc::new(behave);
        let counter = requests.clone();
        let seen = cursors.clone();
        thread::spawn(move || {
            for stream in listener.incoming() {
                let Ok(mut stream) = stream else { continue };
                let number = counter.fetch_add(1, Ordering::SeqCst);
                let behave = behave.clone();
                let seen = seen.clone();
                thread::spawn(move || {
                    let mut head = Vec::new();
                    let mut byte = [0u8; 1];
                    while !head.ends_with(b"\r\n\r\n") {
                        if stream.read(&mut byte).unwrap_or(0) == 0 {
                            return;
                        }
                        head.push(byte[0]);
                    }
                    let text = String::from_utf8_lossy(&head);
                    let after = text
                        .split_whitespace()
                        .nth(1)
                        .and_then(|path| path.split("after=").nth(1))
                        .unwrap_or("")
                        .to_string();
                    seen.lock().unwrap().push(after);
                    behave(number, &mut stream);
                });
            }
        });
        TestServer {
            port,
            requests,
            cursors,
        }
    }

    // Un canal como el de Go: 200 y algunos mensajes cada pocos milisegundos; termina normalmente.
    fn stream_ok(stream: &mut TcpStream, cursor: i64, messages: usize, every: Duration) {
        let _ = stream.write_all(
            b"HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n",
        );
        for index in 0..messages {
            let frame = format!(
                "id: {0}\nevent: changes\ndata: {{\"cursor\":{0},\"reset\":false,\"events\":[]}}\n\n",
                cursor + index as i64
            );
            if stream.write_all(frame.as_bytes()).is_err() {
                return;
            }
            let _ = stream.flush();
            thread::sleep(every);
        }
    }

    fn fast_timing() -> StreamTiming {
        StreamTiming {
            renewal_minimum: Duration::from_millis(100),
            head_timeout: Duration::from_millis(800),
            stall_timeout: Duration::from_millis(800),
            first_delay: Duration::from_millis(50),
            max_delay: Duration::from_millis(200),
        }
    }

    // Corre follow contra el servidor de prueba hasta recibir `limit` mensajes o `deadline`, y devuelve lo recibido.
    fn run(
        server: &TestServer,
        timing: StreamTiming,
        limit: usize,
        deadline: Duration,
    ) -> Vec<Value> {
        let received = Arc::new(Mutex::new(Vec::new()));
        let running = Arc::new(AtomicBool::new(true));
        let client = reqwest::Client::builder().no_proxy().build().unwrap();
        let base = format!("http://127.0.0.1:{}", server.port);
        let started = Instant::now();
        let sink_received = received.clone();
        let sink_running = running.clone();
        let sink = move |message: Value| {
            let mut list = sink_received.lock().unwrap();
            list.push(message);
            if list.len() >= limit {
                sink_running.store(false, Ordering::SeqCst);
            }
            true
        };
        let active_running = running.clone();
        let active = move || active_running.load(Ordering::SeqCst) && started.elapsed() < deadline;
        let open = |cursor: i64| {
            let request = client
                .get(format!("{base}/api/v1/events"))
                .query(&[("after", cursor.to_string())])
                .timeout(Duration::from_secs(5));
            async move {
                match request.send().await {
                    Ok(response) => Attempt::Response(response),
                    Err(_) => Attempt::Failed,
                }
            }
        };
        tauri::async_runtime::block_on(follow(7, open, sink, active, timing));
        let list = received.lock().unwrap().clone();
        list
    }

    fn connections(messages: &[Value], kind: &str) -> usize {
        messages
            .iter()
            .filter(|message| message.get("connection").and_then(Value::as_str) == Some(kind))
            .count()
    }

    // Etapa K1 (K1-D10): el fin normal del canal (Go lo renueva cada 30 s; aquí cada 300 ms) se vuelve a abrir al momento
    // desde el mismo cursor, sin «reconnecting» y sin otro «connected».
    #[test]
    fn the_normal_end_reopens_silently_from_the_same_cursor() {
        let server = serve(|number, stream| {
            stream_ok(
                stream,
                10 + number as i64 * 3,
                3,
                Duration::from_millis(100),
            )
        });
        let messages = run(&server, fast_timing(), 13, Duration::from_secs(10));
        assert_eq!(connections(&messages, "connected"), 1, "{messages:?}");
        assert_eq!(connections(&messages, "reconnecting"), 0, "{messages:?}");
        assert!(server.requests.load(Ordering::SeqCst) >= 4);
        // Cada canal nuevo pide desde el último cursor recibido.
        let cursors = server.cursors.lock().unwrap().clone();
        assert_eq!(cursors[0], "7");
        assert_eq!(cursors[1], "12");
        assert_eq!(cursors[2], "15");
    }

    // Un corte breve (el servidor no responde un rato y vuelve) es una falla: se avisa una vez y al volver se anuncia
    // «connected» otra vez. La tolerancia de 3 fallas y 20 s la decide React, así que un corte de 1 a 5 s nunca se ve.
    #[test]
    fn a_short_outage_is_one_failure_and_recovers() {
        let server = serve(|number, stream| {
            if number == 1 {
                // Segunda conexión: no responde (como Go detenido) hasta que vence el plazo del cliente.
                thread::sleep(Duration::from_millis(1500));
                return;
            }
            stream_ok(
                stream,
                20 + number as i64 * 2,
                2,
                Duration::from_millis(150),
            );
        });
        let messages = run(&server, fast_timing(), 9, Duration::from_secs(10));
        assert_eq!(connections(&messages, "reconnecting"), 1, "{messages:?}");
        assert_eq!(connections(&messages, "connected"), 2, "{messages:?}");
    }

    // Un canal que se abre y se queda mudo cuenta como falla (Go detenido con el canal abierto).
    #[test]
    fn a_stalled_stream_is_a_failure() {
        let server = serve(|number, stream| {
            let _ = stream.write_all(
                b"HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n",
            );
            if number == 0 {
                let _ =
                    stream.write_all(b"data: {\"cursor\":30,\"reset\":false,\"events\":[]}\n\n");
                thread::sleep(Duration::from_secs(3));
                return;
            }
            stream_ok(stream, 31, 3, Duration::from_millis(150));
        });
        let messages = run(&server, fast_timing(), 7, Duration::from_secs(10));
        assert_eq!(connections(&messages, "reconnecting"), 1, "{messages:?}");
        assert_eq!(connections(&messages, "connected"), 2, "{messages:?}");
    }

    // Un corte largo: cada intento falla y se avisa cada vez, con espera creciente.
    #[test]
    fn a_long_outage_keeps_reporting_failures() {
        let server = serve(|_, stream| {
            let _ = stream.write_all(b"HTTP/1.1 503 Service Unavailable\r\nContent-Length: 0\r\nConnection: close\r\n\r\n");
        });
        let messages = run(&server, fast_timing(), 5, Duration::from_secs(10));
        assert_eq!(connections(&messages, "reconnecting"), 5, "{messages:?}");
        assert_eq!(connections(&messages, "connected"), 0, "{messages:?}");
    }

    // «interrupted» de Go (la lectura falló del lado de Go) y un canal que se cierra enseguida son fallas, no
    // renovaciones: no hay bucle sin espera.
    #[test]
    fn interrupted_and_instant_ends_are_failures() {
        let server = serve(|number, stream| {
            let _ = stream.write_all(
                b"HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n",
            );
            if number == 0 {
                let _ = stream.write_all(b"event: interrupted\ndata: {\"error\":\"SERVICE_UNAVAILABLE\",\"status\":503}\n\n");
            }
            // Las demás se cierran al momento.
        });
        let messages = run(&server, fast_timing(), 5, Duration::from_secs(10));
        assert_eq!(connections(&messages, "reconnecting"), 2, "{messages:?}");
    }

    // 401 o 403: la sesión ya no sirve; se avisa y el canal termina.
    #[test]
    fn unauthorized_stops_the_channel() {
        let server = serve(|_, stream| {
            let _ = stream.write_all(
                b"HTTP/1.1 401 Unauthorized\r\nContent-Length: 0\r\nConnection: close\r\n\r\n",
            );
        });
        let messages = run(&server, fast_timing(), 10, Duration::from_secs(3));
        assert_eq!(
            messages,
            vec![json!({"connection": "unauthorized", "status": 401})]
        );
        assert_eq!(server.requests.load(Ordering::SeqCst), 1);
    }

    // Etapa K1 (Fase E): 10 minutos sin uso contra un Go temporal real (local y por la red), con la sesión del
    // administrador. Debe dar cero «reconnecting» y un solo «connected». Ignorada por omisión; se corre con:
    //   ADEL_IDLE_BASE=http://127.0.0.1:8203 ADEL_IDLE_DEVICE=… ADEL_IDLE_TOKEN=… ADEL_IDLE_SESSION=… \
    //   [ADEL_IDLE_FINGERPRINT=<huella SPKI en hexadecimal, para https>] [ADEL_IDLE_SECONDS=600] \
    //   cargo test --release realtime::tests::idle_channel_against_go -- --ignored --nocapture
    #[test]
    #[ignore]
    fn idle_channel_against_go() {
        let base = std::env::var("ADEL_IDLE_BASE").expect("ADEL_IDLE_BASE");
        let device = std::env::var("ADEL_IDLE_DEVICE").expect("ADEL_IDLE_DEVICE");
        let token = std::env::var("ADEL_IDLE_TOKEN").expect("ADEL_IDLE_TOKEN");
        let session = std::env::var("ADEL_IDLE_SESSION").expect("ADEL_IDLE_SESSION");
        let seconds: u64 = std::env::var("ADEL_IDLE_SECONDS")
            .ok()
            .and_then(|text| text.parse().ok())
            .unwrap_or(600);
        assert!(
            !base.contains(":8181") && !base.contains(":8443"),
            "nunca contra el servicio real"
        );
        let client = match std::env::var("ADEL_IDLE_FINGERPRINT") {
            Ok(hex) => {
                let mut fingerprint = [0u8; 32];
                for (index, value) in fingerprint.iter_mut().enumerate() {
                    *value = u8::from_str_radix(&hex[index * 2..index * 2 + 2], 16).unwrap();
                }
                let verifier = crate::pairing::PinnedVerifier::pinned(fingerprint);
                crate::pairing::pinned_client(std::sync::Arc::new(verifier)).unwrap()
            }
            Err(_) => reqwest::Client::builder()
                .no_proxy()
                .pool_idle_timeout(Duration::from_secs(30))
                .build()
                .unwrap(),
        };
        let received = Arc::new(Mutex::new(Vec::new()));
        let started = Instant::now();
        let sink_received = received.clone();
        let sink = move |message: Value| {
            if message.get("connection").is_some() {
                eprintln!("{:>6.1} s {message}", started.elapsed().as_secs_f64());
            }
            sink_received.lock().unwrap().push(message);
            true
        };
        let active = move || started.elapsed() < Duration::from_secs(seconds);
        let open = |cursor: i64| {
            let request =
                crate::api::with_client_headers(client.get(format!("{base}/api/v1/events")))
                    .header("X-Device-ID", &device)
                    .header("X-Device-Token", &token)
                    .bearer_auth(&session)
                    .header("Accept", "text/event-stream")
                    .query(&[("after", cursor.to_string())])
                    .timeout(STREAM_TIMEOUT);
            async move {
                match request.send().await {
                    Ok(response) => Attempt::Response(response),
                    Err(_) => Attempt::Failed,
                }
            }
        };
        tauri::async_runtime::block_on(follow(0, open, sink, active, StreamTiming::PRODUCTION));
        let messages = received.lock().unwrap().clone();
        let changes = messages
            .iter()
            .filter(|message| message.get("cursor").is_some())
            .count();
        eprintln!(
            "{seconds} s: {changes} mensajes de cambios, {} connected, {} reconnecting",
            connections(&messages, "connected"),
            connections(&messages, "reconnecting")
        );
        assert_eq!(connections(&messages, "reconnecting"), 0);
        assert_eq!(connections(&messages, "connected"), 1);
        assert_eq!(connections(&messages, "unauthorized"), 0);
        // Go manda un mensaje cada 750 ms: en 10 minutos son unos 800.
        assert!(
            changes as u64 >= seconds,
            "{changes} mensajes en {seconds} s"
        );
    }
}
