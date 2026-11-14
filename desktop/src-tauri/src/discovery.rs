//! Búsqueda de la computadora principal en la red local y cambio de dirección (etapa 9G, fase C; §4.1 y
//! decisiones D2 y D6).
//!
//! - **Búsqueda** (`search`): una consulta mDNS de una sola vez (RFC 6762 §6.7) desde un puerto efímero a
//!   `224.0.0.251:5353` por el servicio `_adelpos._tcp.local`; el respondedor del sistema de la computadora
//!   principal contesta por unicast con el PTR y, en los adicionales, el SRV (puerto), el TXT (`server_id`,
//!   versión de la API, puerto y huella corta) y las direcciones IPv4. Sin crates nuevos: `std::net::UdpSocket` y un
//!   lector propio de mensajes DNS (con nombres comprimidos y límites contra paquetes malformados).
//! - **React nunca manda un destino**: recibe la lista (nombre, `server_id` y si ya está emparejado) y elige por
//!   identificador; Rust conserva las direcciones (`ServerLocator::discovered`). La dirección escrita a mano sigue
//!   por `pairing::normalize_address`.
//! - **Cambio de IP** (`ServerLocator::relocate`): si una terminal emparejada no logra conectar con la última
//!   dirección, vuelve a buscar por su `server_id`, prueba cada dirección nueva con la huella fijada completa
//!   (`pairing::PinnedVerifier`: sin volver a emparejar) y, si responde, la usa y la guarda en `connection.json`. Un
//!   servidor con el mismo nombre o el mismo `server_id` pero otra huella nunca se acepta: la huella corta del TXT
//!   solo descarta pronto, la comprobación de verdad es el intercambio TLS.

use crate::connection::{parse_config, ConnectionConfig};
use crate::pairing;
use serde::Serialize;
use std::collections::HashMap;
use std::net::{Ipv4Addr, SocketAddrV4, UdpSocket};
use std::path::PathBuf;
use std::sync::{Arc, Mutex, RwLock};
use std::time::{Duration, Instant};

/// Tipo de servicio DNS-SD del servidor (también en NSBonjourServices del Info.plist y en internal/discovery).
pub const SERVICE_LABELS: [&str; 3] = ["_adelpos", "_tcp", "local"];
const MDNS_GROUP: SocketAddrV4 = SocketAddrV4::new(Ipv4Addr::new(224, 0, 0, 251), 5353);
const TYPE_A: u16 = 1;
const TYPE_PTR: u16 = 12;
const TYPE_TXT: u16 = 16;
const TYPE_SRV: u16 = 33;
const CLASS_IN: u16 = 1;
/// Cuánto se escucha cada búsqueda (la respuesta de la red local llega en milisegundos).
pub const SEARCH_WINDOW: Duration = Duration::from_millis(1500);
/// Una terminal emparejada no vuelve a buscar más seguido que esto aunque fallen muchas peticiones.
const RELOCATION_INTERVAL: Duration = Duration::from_secs(10);

/// Mensaje cuando la red no permite buscar (sin red o sin el permiso «Red local» de macOS).
pub const SEARCH_FAILED_MESSAGE: &str = "No se pudo buscar en la red local. Revisa que esta computadora esté conectada a la red del restaurante y que AD-EL POS tenga permiso de usar la red local; también puedes escribir la dirección.";

/// Nombre DNS como lista de etiquetas (una etiqueta puede tener puntos o acentos: el nombre de la instancia).
pub type Name = Vec<String>;

fn same_name(left: &[String], right: &[String]) -> bool {
    left.len() == right.len()
        && left
            .iter()
            .zip(right)
            .all(|(a, b)| a.eq_ignore_ascii_case(b))
}

fn service_name() -> Name {
    SERVICE_LABELS
        .iter()
        .map(|label| label.to_string())
        .collect()
}

/// Un registro de la respuesta con los tipos que importan.
#[derive(Clone, Debug, PartialEq)]
pub enum RecordData {
    Ptr(Name),
    Srv { port: u16, target: Name },
    Txt(Vec<Vec<u8>>),
    A(Ipv4Addr),
    Other,
}

#[derive(Clone, Debug, PartialEq)]
pub struct Record {
    pub name: Name,
    pub data: RecordData,
}

/// Arma una consulta con el identificador y las preguntas (nombre y tipo, clase IN).
pub fn build_query(id: u16, questions: &[(Name, u16)]) -> Vec<u8> {
    let mut packet = Vec::with_capacity(64);
    packet.extend_from_slice(&id.to_be_bytes());
    packet.extend_from_slice(&0u16.to_be_bytes());
    packet.extend_from_slice(&(questions.len() as u16).to_be_bytes());
    packet.extend_from_slice(&[0, 0, 0, 0, 0, 0]);
    for (name, kind) in questions {
        for label in name {
            let bytes = label.as_bytes();
            let length = bytes.len().min(63);
            packet.push(length as u8);
            packet.extend_from_slice(&bytes[..length]);
        }
        packet.push(0);
        packet.extend_from_slice(&kind.to_be_bytes());
        packet.extend_from_slice(&CLASS_IN.to_be_bytes());
    }
    packet
}

fn read_u16(packet: &[u8], offset: usize) -> Result<u16, ()> {
    let bytes = packet.get(offset..offset + 2).ok_or(())?;
    Ok(u16::from_be_bytes([bytes[0], bytes[1]]))
}

/// Lee un nombre (con compresión) y devuelve las etiquetas y la posición siguiente. Los saltos solo van hacia
/// atrás y son pocos: un paquete malformado no provoca un ciclo.
fn read_name(packet: &[u8], start: usize) -> Result<(Name, usize), ()> {
    let mut labels = Vec::new();
    let mut offset = start;
    let mut next = None;
    let mut jumps = 0;
    let mut total = 0usize;
    loop {
        let length = *packet.get(offset).ok_or(())? as usize;
        match length & 0xC0 {
            0x00 => {
                if length == 0 {
                    return Ok((labels, next.unwrap_or(offset + 1)));
                }
                let bytes = packet.get(offset + 1..offset + 1 + length).ok_or(())?;
                total += length + 1;
                if total > 255 {
                    return Err(());
                }
                labels.push(String::from_utf8_lossy(bytes).into_owned());
                offset += 1 + length;
            }
            0xC0 => {
                let pointer = (read_u16(packet, offset)? & 0x3FFF) as usize;
                if pointer >= offset || jumps >= 16 {
                    return Err(());
                }
                if next.is_none() {
                    next = Some(offset + 2);
                }
                jumps += 1;
                offset = pointer;
            }
            _ => return Err(()),
        }
    }
}

/// Lee una respuesta: comprueba que responde a la consulta (identificador y bit de respuesta) y devuelve todos
/// sus registros (respuestas, autoridad y adicionales).
pub fn parse_response(packet: &[u8], id: u16) -> Result<Vec<Record>, ()> {
    if packet.len() < 12 || read_u16(packet, 0)? != id || packet[2] & 0x80 == 0 {
        return Err(());
    }
    let questions = read_u16(packet, 4)?;
    let records = read_u16(packet, 6)? as usize
        + read_u16(packet, 8)? as usize
        + read_u16(packet, 10)? as usize;
    let mut offset = 12;
    for _ in 0..questions {
        let (_, next) = read_name(packet, offset)?;
        offset = next + 4;
    }
    let mut result = Vec::with_capacity(records);
    for _ in 0..records {
        let (name, next) = read_name(packet, offset)?;
        let kind = read_u16(packet, next)?;
        let length = read_u16(packet, next + 8)? as usize;
        let start = next + 10;
        let data = packet.get(start..start + length).ok_or(())?;
        let parsed = match kind {
            TYPE_PTR => RecordData::Ptr(read_name(packet, start)?.0),
            TYPE_SRV if length >= 7 => RecordData::Srv {
                port: read_u16(packet, start + 4)?,
                target: read_name(packet, start + 6)?.0,
            },
            TYPE_TXT => {
                let mut strings = Vec::new();
                let mut position = 0;
                while position < data.len() {
                    let size = data[position] as usize;
                    let value = data.get(position + 1..position + 1 + size).ok_or(())?;
                    strings.push(value.to_vec());
                    position += 1 + size;
                }
                RecordData::Txt(strings)
            }
            TYPE_A if length == 4 => {
                RecordData::A(Ipv4Addr::new(data[0], data[1], data[2], data[3]))
            }
            _ => RecordData::Other,
        };
        result.push(Record { name, data: parsed });
        offset = start + length;
    }
    Ok(result)
}

/// Un servidor encontrado y completo.
#[derive(Clone, Debug, PartialEq)]
pub struct FoundServer {
    pub server_id: String,
    pub name: String,
    pub fingerprint_prefix: String,
    pub port: u16,
    pub addresses: Vec<Ipv4Addr>,
}

impl FoundServer {
    /// Direcciones HTTPS que se pueden probar: solo las de la red local que acepta una terminal emparejada.
    pub fn bases(&self) -> Vec<String> {
        self.addresses
            .iter()
            .map(|address| address.to_string())
            .filter(|host| pairing::valid_server_host(host))
            .map(|host| format!("https://{host}:{}", self.port))
            .collect()
    }
}

/// Lo que se sabe de cada instancia mientras llegan las respuestas.
#[derive(Default, Debug)]
struct Partial {
    target: Option<Name>,
    port: u16,
    txt: Option<HashMap<String, String>>,
}

fn parse_txt(strings: &[Vec<u8>]) -> HashMap<String, String> {
    let mut values = HashMap::new();
    for entry in strings {
        let text = String::from_utf8_lossy(entry);
        if let Some((key, value)) = text.split_once('=') {
            if !key.is_empty() {
                values.insert(key.to_ascii_lowercase(), value.to_string());
            }
        }
    }
    values
}

fn display_name(instance: &[String]) -> String {
    let label = instance.first().map(String::as_str).unwrap_or("");
    let clean: String = label.chars().filter(|value| !value.is_control()).collect();
    clean.trim().chars().take(80).collect()
}

/// Junta los registros: instancias del servicio (PTR) con su SRV, TXT y direcciones IPv4. Solo devuelve las
/// completas y válidas (identificador UUID en minúsculas, huella corta de 16 hexadecimales, puerto y al menos
/// una dirección de la red local); una instancia repetida por `server_id` junta sus direcciones.
pub fn collect(records: &[Record]) -> Vec<FoundServer> {
    let service = service_name();
    let mut instances: Vec<Name> = Vec::new();
    for record in records {
        if let RecordData::Ptr(instance) = &record.data {
            if same_name(&record.name, &service)
                && instance.len() == 4
                && same_name(&instance[1..], &service)
                && !instances.iter().any(|known| same_name(known, instance))
            {
                instances.push(instance.clone());
            }
        }
    }
    let mut found: Vec<FoundServer> = Vec::new();
    for instance in &instances {
        let mut partial = Partial::default();
        for record in records
            .iter()
            .filter(|record| same_name(&record.name, instance))
        {
            match &record.data {
                RecordData::Srv { port, target } => {
                    partial.port = *port;
                    partial.target = Some(target.clone());
                }
                RecordData::Txt(strings) => partial.txt = Some(parse_txt(strings)),
                _ => {}
            }
        }
        let (Some(target), Some(txt)) = (partial.target, partial.txt) else {
            continue;
        };
        let server_id = txt.get("server_id").cloned().unwrap_or_default();
        let prefix = txt.get("fp").cloned().unwrap_or_default();
        let valid_prefix = prefix.len() == 16 && pairing::from_hex(&prefix).is_some();
        if !pairing::valid_identifier(&server_id)
            || server_id.chars().any(|value| value.is_ascii_uppercase())
            || !valid_prefix
            || partial.port == 0
        {
            continue;
        }
        let addresses: Vec<Ipv4Addr> = records
            .iter()
            .filter(|record| same_name(&record.name, &target))
            .filter_map(|record| match record.data {
                RecordData::A(address) if pairing::valid_server_host(&address.to_string()) => {
                    Some(address)
                }
                _ => None,
            })
            .collect();
        if addresses.is_empty() {
            continue;
        }
        if let Some(existing) = found
            .iter_mut()
            .find(|server| server.server_id == server_id)
        {
            for address in addresses {
                if !existing.addresses.contains(&address) {
                    existing.addresses.push(address);
                }
            }
            continue;
        }
        found.push(FoundServer {
            server_id,
            name: display_name(instance),
            fingerprint_prefix: prefix,
            port: partial.port,
            addresses,
        });
    }
    found
}

/// Preguntas de seguimiento para las instancias a las que les falta el SRV, el TXT o la dirección (por si la
/// respuesta no trajo todos los adicionales).
fn follow_up_questions(records: &[Record]) -> Vec<(Name, u16)> {
    let service = service_name();
    let mut questions = Vec::new();
    for record in records {
        let RecordData::Ptr(instance) = &record.data else {
            continue;
        };
        if !same_name(&record.name, &service) {
            continue;
        }
        let mine = || {
            records
                .iter()
                .filter(|other| same_name(&other.name, instance))
        };
        let target = mine().find_map(|other| match &other.data {
            RecordData::Srv { target, .. } => Some(target.clone()),
            _ => None,
        });
        if target.is_none() {
            questions.push((instance.clone(), TYPE_SRV));
        }
        if !mine().any(|other| matches!(other.data, RecordData::Txt(_))) {
            questions.push((instance.clone(), TYPE_TXT));
        }
        if let Some(target) = target {
            let resolved = records.iter().any(|other| {
                same_name(&other.name, &target) && matches!(other.data, RecordData::A(_))
            });
            if !resolved {
                questions.push((target, TYPE_A));
            }
        }
    }
    questions.truncate(16);
    questions
}

fn random_id() -> u16 {
    let mut value = [0u8; 2];
    let _ = rustls::crypto::ring::default_provider()
        .secure_random
        .fill(&mut value);
    u16::from_be_bytes(value)
}

/// Envía una consulta y junta las respuestas hasta el plazo.
fn exchange(
    socket: &UdpSocket,
    questions: &[(Name, u16)],
    window: Duration,
    records: &mut Vec<Record>,
) -> Result<(), String> {
    let id = random_id();
    let query = build_query(id, questions);
    let started = Instant::now();
    let mut sent = 0;
    let mut buffer = [0u8; 9000];
    while started.elapsed() < window {
        // Se repite a la mitad del plazo por si se perdió el primer envío.
        if sent == 0 || (sent == 1 && started.elapsed() >= window / 2) {
            socket
                .send_to(&query, MDNS_GROUP)
                .map_err(|_| SEARCH_FAILED_MESSAGE.to_string())?;
            sent += 1;
        }
        match socket.recv_from(&mut buffer) {
            Ok((length, _)) => {
                if let Ok(parsed) = parse_response(&buffer[..length], id) {
                    records.extend(parsed);
                }
            }
            Err(error)
                if matches!(
                    error.kind(),
                    std::io::ErrorKind::WouldBlock | std::io::ErrorKind::TimedOut
                ) => {}
            Err(_) => return Err(SEARCH_FAILED_MESSAGE.to_string()),
        }
    }
    Ok(())
}

/// Busca servidores en la red local durante `window` (bloqueante: se llama desde `spawn_blocking`).
pub fn search(window: Duration) -> Result<Vec<FoundServer>, String> {
    let socket = UdpSocket::bind((Ipv4Addr::UNSPECIFIED, 0))
        .map_err(|_| SEARCH_FAILED_MESSAGE.to_string())?;
    socket
        .set_read_timeout(Some(Duration::from_millis(100)))
        .map_err(|_| SEARCH_FAILED_MESSAGE.to_string())?;
    let _ = socket.set_multicast_ttl_v4(255);
    let mut records = Vec::new();
    exchange(&socket, &[(service_name(), TYPE_PTR)], window, &mut records)?;
    let follow_up = follow_up_questions(&records);
    if !follow_up.is_empty() {
        exchange(&socket, &follow_up, window / 2, &mut records)?;
    }
    Ok(collect(&records))
}

/// Lo que ve React de cada servidor encontrado: nombre, identificador y si es el servidor emparejado.
#[derive(Serialize, Debug, PartialEq)]
pub struct ServerView {
    pub id: String,
    pub name: String,
    pub paired: bool,
}

/// Servidor emparejado: identificador, huella fijada y la conexión que se reescribe al cambiar de dirección.
pub struct PairedServer {
    pub server_id: String,
    pub fingerprint: [u8; 32],
    pub config: ConnectionConfig,
    pub connection_file: Option<PathBuf>,
}

/// Dirección vigente del servidor (cambia sola si el servidor emparejado aparece en otra dirección) y los
/// servidores de la última búsqueda.
pub struct ServerLocator {
    base: RwLock<String>,
    paired: Option<PairedServer>,
    attempt: tauri::async_runtime::Mutex<Option<Instant>>,
    discovered: Mutex<Vec<FoundServer>>,
}

impl ServerLocator {
    pub fn new(
        config: &Result<ConnectionConfig, String>,
        connection_file: Option<PathBuf>,
    ) -> Self {
        let base = config
            .as_ref()
            .map(|value| value.server_url.clone())
            .unwrap_or_default();
        let paired = config.as_ref().ok().and_then(|value| {
            let server_id = value.server_id.clone()?;
            let fingerprint = pairing::fingerprint_from_hex(value.server_fingerprint.as_deref()?)?;
            Some(PairedServer {
                server_id,
                fingerprint,
                config: value.clone(),
                connection_file,
            })
        });
        Self {
            base: RwLock::new(base),
            paired,
            attempt: tauri::async_runtime::Mutex::new(None),
            discovered: Mutex::new(Vec::new()),
        }
    }

    /// Dirección base vigente (`https://ip:puerto` o la de siempre).
    pub fn base(&self) -> String {
        self.base
            .read()
            .map(|value| value.clone())
            .unwrap_or_default()
    }

    /// `server_id` del servidor emparejado.
    pub fn paired_server(&self) -> Option<&str> {
        self.paired.as_ref().map(|value| value.server_id.as_str())
    }

    /// Guarda la última búsqueda y devuelve lo que ve React.
    pub fn remember(&self, found: Vec<FoundServer>) -> Vec<ServerView> {
        let views = found
            .iter()
            .map(|server| ServerView {
                id: server.server_id.clone(),
                name: server.name.clone(),
                paired: self.paired_server() == Some(server.server_id.as_str()),
            })
            .collect();
        if let Ok(mut stored) = self.discovered.lock() {
            *stored = found;
        }
        views
    }

    /// Un servidor de la última búsqueda, por identificador.
    pub fn found(&self, server_id: &str) -> Option<FoundServer> {
        self.discovered
            .lock()
            .ok()?
            .iter()
            .find(|server| server.server_id == server_id)
            .cloned()
    }

    /// Vuelve a encontrar el servidor emparejado si no responde en `failed_base` (cambio de IP). Devuelve la
    /// dirección nueva si la encontró con la huella correcta (o si otra petición ya la encontró). Una sola
    /// búsqueda a la vez y como máximo una cada 10 s.
    pub async fn relocate(&self, failed_base: &str) -> Option<String> {
        let paired = self.paired.as_ref()?;
        let mut last = self.attempt.lock().await;
        let current = self.base();
        if current != failed_base {
            return Some(current);
        }
        if last.is_some_and(|time| time.elapsed() < RELOCATION_INTERVAL) {
            return None;
        }
        *last = Some(Instant::now());
        let found = tauri::async_runtime::spawn_blocking(|| search(SEARCH_WINDOW))
            .await
            .ok()?
            .ok()?;
        for base in candidates(&found, &paired.server_id, &paired.fingerprint, &current) {
            if probe(&base, paired.fingerprint).await {
                self.move_to(paired, &base).ok()?;
                return Some(base);
            }
        }
        None
    }

    /// Usa la dirección nueva y la guarda como última dirección en `connection.json`.
    fn move_to(&self, paired: &PairedServer, base: &str) -> Result<(), String> {
        let next = relocated_connection(&paired.config, base)?;
        if let Some(filename) = &paired.connection_file {
            crate::installation::save_connection(filename, &next)?;
        }
        if let Ok(mut value) = self.base.write() {
            *value = next.server_url;
        }
        Ok(())
    }
}

/// Direcciones donde probar el servidor emparejado: solo las del mismo `server_id` con la misma huella corta
/// (las demás ni se intentan) y distintas de la que acaba de fallar.
pub fn candidates(
    found: &[FoundServer],
    server_id: &str,
    fingerprint: &[u8; 32],
    failed_base: &str,
) -> Vec<String> {
    let prefix = pairing::to_hex(&fingerprint[..8]);
    found
        .iter()
        .filter(|server| server.server_id == server_id && server.fingerprint_prefix == prefix)
        .flat_map(FoundServer::bases)
        .filter(|base| base != failed_base)
        .collect()
}

/// Prueba una dirección con la huella fijada completa: solo responde quien tiene la llave del servidor.
pub async fn probe(base: &str, fingerprint: [u8; 32]) -> bool {
    let verifier = Arc::new(pairing::PinnedVerifier::pinned(fingerprint));
    let Ok(client) = pairing::pinned_client(verifier) else {
        return false;
    };
    client
        .get(format!("{base}/api/v1/health/live"))
        .timeout(Duration::from_secs(4))
        .send()
        .await
        .map(|response| response.status().as_u16() == 200)
        .unwrap_or(false)
}

/// La conexión con la dirección nueva (misma terminal, mismo servidor y misma huella).
pub fn relocated_connection(
    current: &ConnectionConfig,
    base: &str,
) -> Result<ConnectionConfig, String> {
    let mut next = current.clone();
    next.server_url = base.to_string();
    let text = serde_json::to_string(&next).map_err(|_| "No se pudo preparar la conexión.")?;
    parse_config(&text)
}

/// Cambia el destino de una petición ya armada a la dirección nueva (esquema, host y puerto).
pub fn rebase(url: &mut reqwest::Url, base: &str) -> Result<(), String> {
    let next = reqwest::Url::parse(base).map_err(|_| "Dirección del servidor inválida.")?;
    url.set_scheme(next.scheme())
        .map_err(|_| "Dirección del servidor inválida.")?;
    url.set_host(next.host_str())
        .map_err(|_| "Dirección del servidor inválida.")?;
    url.set_port(next.port())
        .map_err(|_| "Dirección del servidor inválida.")?;
    Ok(())
}

/// Busca la computadora principal en la red local y entrega la lista a React (sin direcciones).
#[tauri::command]
pub async fn discovery_search(
    state: tauri::State<'_, crate::TerminalState>,
) -> Result<Vec<ServerView>, String> {
    let found = tauri::async_runtime::spawn_blocking(|| search(SEARCH_WINDOW))
        .await
        .map_err(|_| SEARCH_FAILED_MESSAGE.to_string())??;
    Ok(state.locator.remember(found))
}

#[cfg(test)]
mod tests {
    use super::*;
    use rustls::pki_types::{CertificateDer, PrivateKeyDer};
    use std::io::{Read, Write};
    use std::net::TcpListener;

    const CERT_A: &str = include_str!("testdata/server-a.cert.hex");
    const KEY_A: &str = include_str!("testdata/server-a.key.hex");
    const CERT_B: &str = include_str!("testdata/server-b.cert.hex");
    const KEY_B: &str = include_str!("testdata/server-b.key.hex");
    const SERVER_ID: &str = "11111111-2222-4333-8444-555555555555";

    fn labels(text: &[&str]) -> Name {
        text.iter().map(|label| label.to_string()).collect()
    }

    /// Escritor de respuestas de prueba con compresión de nombres, como las de mDNSResponder.
    struct Writer {
        packet: Vec<u8>,
        names: Vec<(Name, usize)>,
    }

    impl Writer {
        fn new(id: u16, questions: u16, answers: u16, additionals: u16) -> Self {
            let mut packet = Vec::new();
            packet.extend_from_slice(&id.to_be_bytes());
            packet.extend_from_slice(&[0x84, 0x00]);
            for count in [questions, answers, 0, additionals] {
                packet.extend_from_slice(&count.to_be_bytes());
            }
            Self {
                packet,
                names: Vec::new(),
            }
        }

        fn name(&mut self, name: &[String]) {
            for index in 0..name.len() {
                let suffix = name[index..].to_vec();
                if let Some((_, offset)) = self
                    .names
                    .iter()
                    .find(|(known, _)| same_name(known, &suffix))
                {
                    let pointer = 0xC000 | *offset as u16;
                    self.packet.extend_from_slice(&pointer.to_be_bytes());
                    return;
                }
                self.names.push((suffix, self.packet.len()));
                let bytes = name[index].as_bytes();
                self.packet.push(bytes.len() as u8);
                self.packet.extend_from_slice(bytes);
            }
            self.packet.push(0);
        }

        fn question(&mut self, name: &[String], kind: u16) {
            self.name(name);
            self.packet.extend_from_slice(&kind.to_be_bytes());
            self.packet.extend_from_slice(&CLASS_IN.to_be_bytes());
        }

        fn record(&mut self, name: &[String], kind: u16, data: impl FnOnce(&mut Self)) {
            self.name(name);
            self.packet.extend_from_slice(&kind.to_be_bytes());
            self.packet.extend_from_slice(&CLASS_IN.to_be_bytes());
            self.packet.extend_from_slice(&10u32.to_be_bytes());
            let length_at = self.packet.len();
            self.packet.extend_from_slice(&[0, 0]);
            data(self);
            let length = (self.packet.len() - length_at - 2) as u16;
            self.packet[length_at..length_at + 2].copy_from_slice(&length.to_be_bytes());
        }
    }

    fn txt(writer: &mut Writer, entries: &[String]) {
        for entry in entries {
            writer.packet.push(entry.len() as u8);
            writer.packet.extend_from_slice(entry.as_bytes());
        }
    }

    /// Respuesta con dos instancias: la del restaurante (con acentos y un punto dentro del nombre) y otra con un
    /// identificador inválido, más una dirección pública que se descarta.
    fn sample_response(id: u16) -> Vec<u8> {
        let service = service_name();
        let fonda = labels(&[
            "Fonda Doña Ñeli S.A. — Mac de caja",
            "_adelpos",
            "_tcp",
            "local",
        ]);
        let other = labels(&["Otra computadora", "_adelpos", "_tcp", "local"]);
        let host = labels(&["Mac-de-caja", "local"]);
        let mut writer = Writer::new(id, 1, 2, 5);
        writer.question(&service, TYPE_PTR);
        writer.record(&service, TYPE_PTR, |w| w.name(&fonda));
        writer.record(&service, TYPE_PTR, |w| w.name(&other));
        writer.record(&fonda, TYPE_SRV, |w| {
            w.packet.extend_from_slice(&[0, 0, 0, 0]);
            w.packet.extend_from_slice(&8443u16.to_be_bytes());
            w.name(&host);
        });
        writer.record(&fonda, TYPE_TXT, |w| {
            txt(
                w,
                &[
                    "txtvers=1".into(),
                    format!("server_id={SERVER_ID}"),
                    "api=v1".into(),
                    "port=8443".into(),
                    "fp=00112233aabbccdd".into(),
                ],
            )
        });
        writer.record(&other, TYPE_TXT, |w| {
            txt(
                w,
                &["server_id=no-es-uuid".into(), "fp=00112233aabbccdd".into()],
            )
        });
        writer.record(&host, TYPE_A, |w| {
            w.packet.extend_from_slice(&[192, 168, 1, 20])
        });
        writer.record(&host, TYPE_A, |w| w.packet.extend_from_slice(&[8, 8, 8, 8]));
        writer.packet
    }

    #[test]
    fn the_query_asks_for_the_service_once() {
        let query = build_query(0x4a21, &[(service_name(), TYPE_PTR)]);
        let mut expected = vec![0x4a, 0x21, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0];
        expected.extend_from_slice(b"\x08_adelpos\x04_tcp\x05local\x00\x00\x0c\x00\x01");
        assert_eq!(query, expected);
    }

    #[test]
    fn a_response_gives_the_named_server_with_its_identity_and_local_address() {
        let packet = sample_response(0x1234);
        let records = parse_response(&packet, 0x1234).unwrap();
        let found = collect(&records);
        assert_eq!(found.len(), 1, "{found:?}");
        let server = &found[0];
        assert_eq!(server.name, "Fonda Doña Ñeli S.A. — Mac de caja");
        assert_eq!(server.server_id, SERVER_ID);
        assert_eq!(server.fingerprint_prefix, "00112233aabbccdd");
        assert_eq!(server.port, 8443);
        assert_eq!(server.addresses, vec![Ipv4Addr::new(192, 168, 1, 20)]);
        assert_eq!(
            server.bases(),
            vec!["https://192.168.1.20:8443".to_string()]
        );
        // Nada que pedir de nuevo: la respuesta trajo SRV, TXT y dirección del restaurante.
        assert!(follow_up_questions(&records)
            .iter()
            .all(|(name, _)| name[0] == "Otra computadora"));
    }

    #[test]
    fn foreign_or_broken_packets_are_rejected_without_panicking() {
        let packet = sample_response(0x1234);
        assert!(parse_response(&packet, 0x9999).is_err(), "otra consulta");
        let query = build_query(0x1234, &[(service_name(), TYPE_PTR)]);
        assert!(
            parse_response(&query, 0x1234).is_err(),
            "una consulta no es respuesta"
        );
        for cut in 0..packet.len() {
            let _ = parse_response(&packet[..cut], 0x1234);
        }
        // Un puntero que apunta a sí mismo (ciclo) se rechaza.
        let mut looped = Writer::new(1, 1, 0, 0).packet;
        looped.extend_from_slice(&[0xC0, 12, 0, 12, 0, 1]);
        assert!(parse_response(&looped, 1).is_err());
    }

    #[test]
    fn missing_records_are_asked_again() {
        let service = service_name();
        let fonda = labels(&["Fonda", "_adelpos", "_tcp", "local"]);
        let mut writer = Writer::new(7, 0, 1, 0);
        writer.record(&service, TYPE_PTR, |w| w.name(&fonda));
        let records = parse_response(&writer.packet, 7).unwrap();
        assert!(collect(&records).is_empty());
        let questions = follow_up_questions(&records);
        assert_eq!(
            questions,
            vec![(fonda.clone(), TYPE_SRV), (fonda, TYPE_TXT)]
        );
    }

    fn found(server_id: &str, prefix: &str, address: [u8; 4]) -> FoundServer {
        FoundServer {
            server_id: server_id.into(),
            name: "Fonda".into(),
            fingerprint_prefix: prefix.into(),
            port: 8443,
            addresses: vec![Ipv4Addr::from(address)],
        }
    }

    #[test]
    fn only_the_paired_server_with_its_fingerprint_is_tried() {
        let fingerprint = [0xabu8; 32];
        let prefix = pairing::to_hex(&fingerprint[..8]);
        let list = vec![
            // El servidor emparejado en su dirección nueva.
            found(SERVER_ID, &prefix, [192, 168, 1, 77]),
            // Mismo identificador anunciado con otra huella: ni se intenta.
            found(SERVER_ID, "0000000000000000", [192, 168, 1, 78]),
            // Otro servidor con el mismo nombre.
            found(
                "99999999-2222-4333-8444-555555555555",
                &prefix,
                [192, 168, 1, 79],
            ),
            // La dirección que acaba de fallar.
            found(SERVER_ID, &prefix, [192, 168, 1, 20]),
        ];
        assert_eq!(
            candidates(&list, SERVER_ID, &fingerprint, "https://192.168.1.20:8443"),
            vec!["https://192.168.1.77:8443".to_string()]
        );
    }

    /// Servidor HTTPS de prueba en 127.0.0.1 con el certificado dado: atiende una conexión con 200.
    fn tls_server(cert: &str, key: &str) -> u16 {
        let der = |text: &str| pairing::from_hex(text.trim()).unwrap();
        let config = rustls::ServerConfig::builder_with_provider(Arc::new(
            rustls::crypto::ring::default_provider(),
        ))
        .with_protocol_versions(&[&rustls::version::TLS13, &rustls::version::TLS12])
        .unwrap()
        .with_no_client_auth()
        .with_single_cert(
            vec![CertificateDer::from(der(cert))],
            PrivateKeyDer::try_from(der(key)).unwrap(),
        )
        .unwrap();
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let port = listener.local_addr().unwrap().port();
        let config = Arc::new(config);
        std::thread::spawn(move || {
            if let Ok((socket, _)) = listener.accept() {
                let connection = rustls::ServerConnection::new(config).unwrap();
                let mut stream = rustls::StreamOwned::new(connection, socket);
                let mut request = Vec::new();
                let mut buffer = [0u8; 1024];
                while !request.windows(4).any(|part| part == b"\r\n\r\n") {
                    match stream.read(&mut buffer) {
                        Ok(0) | Err(_) => return,
                        Ok(read) => request.extend_from_slice(&buffer[..read]),
                    }
                }
                let _ = stream.write_all(
                    b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 15\r\nConnection: close\r\n\r\n{\"status\":\"ok\"}",
                );
                let _ = stream.flush();
                stream.conn.send_close_notify();
                let _ = stream.flush();
            }
        });
        port
    }

    #[test]
    fn a_new_address_is_accepted_only_with_the_pinned_fingerprint() {
        let pinned = pairing::spki_fingerprint(&CertificateDer::from(
            pairing::from_hex(CERT_A.trim()).unwrap(),
        ))
        .unwrap();
        let right = tls_server(CERT_A, KEY_A);
        let wrong = tls_server(CERT_B, KEY_B);
        tauri::async_runtime::block_on(async move {
            // El servidor en su dirección nueva responde con la llave fijada: se acepta sin emparejar de nuevo.
            assert!(probe(&format!("https://127.0.0.1:{right}"), pinned).await);
            // Otro servidor (aunque se anuncie igual) presenta otra llave: nunca se acepta.
            assert!(!probe(&format!("https://127.0.0.1:{wrong}"), pinned).await);
        });
    }

    fn paired_config(url: &str) -> ConnectionConfig {
        parse_config(
            &serde_json::json!({
                "installation_mode": "terminal-only",
                "server_url": url,
                "server_id": SERVER_ID,
                "server_fingerprint": "ab".repeat(32),
                "server_name": "Fonda Doña Ñeli",
            })
            .to_string(),
        )
        .unwrap()
    }

    #[test]
    fn moving_to_the_new_address_rewrites_the_last_address_only() {
        let directory = std::env::temp_dir().join(format!("adel-discovery-{}", random_id()));
        std::fs::create_dir_all(&directory).unwrap();
        let filename = directory.join("connection.json");
        let config = paired_config("https://192.168.1.20:8443");
        crate::installation::save_connection(&filename, &config).unwrap();
        let locator = ServerLocator::new(&Ok(config), Some(filename.clone()));
        assert_eq!(locator.base(), "https://192.168.1.20:8443");
        assert_eq!(locator.paired_server(), Some(SERVER_ID));
        let paired = locator.paired.as_ref().unwrap();
        locator
            .move_to(paired, "https://192.168.1.77:8443")
            .unwrap();
        assert_eq!(locator.base(), "https://192.168.1.77:8443");
        let saved = parse_config(&std::fs::read_to_string(&filename).unwrap()).unwrap();
        assert_eq!(saved.server_url, "https://192.168.1.77:8443");
        assert_eq!(saved.server_id.as_deref(), Some(SERVER_ID));
        assert_eq!(saved.server_fingerprint, Some("ab".repeat(32)));
        // Una dirección fuera de la red local no se guarda ni se usa.
        assert!(locator.move_to(paired, "https://8.8.8.8:8443").is_err());
        assert_eq!(locator.base(), "https://192.168.1.77:8443");
        std::fs::remove_dir_all(&directory).unwrap();
        // La petición ya armada cambia de destino conservando la ruta y los parámetros.
        let mut url = reqwest::Url::parse("https://192.168.1.20:8443/api/v1/auth/me?x=1").unwrap();
        rebase(&mut url, "https://192.168.1.77:9443").unwrap();
        assert_eq!(url.as_str(), "https://192.168.1.77:9443/api/v1/auth/me?x=1");
    }

    #[test]
    fn react_sees_names_identifiers_and_the_paired_mark_but_no_addresses() {
        let locator = ServerLocator::new(&Ok(paired_config("https://192.168.1.20:8443")), None);
        let views = locator.remember(vec![
            found(SERVER_ID, "00112233aabbccdd", [192, 168, 1, 20]),
            found(
                "99999999-2222-4333-8444-555555555555",
                "00112233aabbccdd",
                [10, 0, 0, 3],
            ),
        ]);
        assert_eq!(
            views,
            vec![
                ServerView {
                    id: SERVER_ID.into(),
                    name: "Fonda".into(),
                    paired: true
                },
                ServerView {
                    id: "99999999-2222-4333-8444-555555555555".into(),
                    name: "Fonda".into(),
                    paired: false
                },
            ]
        );
        let shown = serde_json::to_string(&views).unwrap();
        assert!(
            !shown.contains("192.168") && !shown.contains("10.0.0.3") && !shown.contains("8443")
        );
        assert_eq!(locator.found(SERVER_ID).unwrap().port, 8443);
        assert!(locator.found("otro").is_none());
        // Sin emparejar no hay a quién volver a buscar.
        let unpaired = ServerLocator::new(
            &parse_config(r#"{"installation_mode":"terminal-only","server_url":""}"#),
            None,
        );
        assert!(unpaired.paired_server().is_none());
        assert!(tauri::async_runtime::block_on(unpaired.relocate("")).is_none());
    }

    /// Búsqueda real en la red local contra un anuncio de prueba (la corre a mano dtools/live-discovery-C.sh
    /// mientras la prueba acotada de Go mantiene el anuncio): ADEL_DISCOVERY_TEST_SERVER_ID es su identificador.
    #[test]
    #[ignore = "Necesita un anuncio de prueba en la red local; ejecutar explícitamente."]
    fn live_search_finds_a_test_announcement() {
        let expected = std::env::var("ADEL_DISCOVERY_TEST_SERVER_ID").unwrap();
        let found = search(SEARCH_WINDOW).unwrap();
        let server = found
            .iter()
            .find(|server| server.server_id == expected)
            .unwrap_or_else(|| panic!("no se encontró el anuncio de prueba: {found:?}"));
        println!(
            "DISCOVERY_FOUND name={:?} port={} addresses={}",
            server.name,
            server.port,
            server.addresses.len()
        );
        assert!(!server.bases().is_empty());
    }
}
