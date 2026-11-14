//! Emparejamiento con código y fijación del certificado del servidor (etapa 9G, fase B; §3.1, §3.2 y
//! decisiones D2, D5 y D6).
//!
//! - **Fijación** (`PinnedVerifier`): la terminal no confía en ninguna autoridad ni en el nombre o la IP del
//!   certificado del servidor. Durante el emparejamiento fija la huella SPKI (SHA-256 de la llave pública)
//!   del primer certificado que ve en la sesión; desde entonces solo acepta un certificado con esa misma
//!   llave, sirva en la dirección que sirva (cambio de IP por DHCP), y comprueba igualmente la firma del
//!   intercambio TLS con esa llave (quien no tiene la llave privada no puede presentarse con ese
//!   certificado). Si el certificado cambia, la terminal no se conecta y avisa en lenguaje sencillo; nunca
//!   acepta cualquier certificado.
//! - **Destino** (D2): React solo propone la dirección escrita a mano; Rust la valida (HTTPS a una IP
//!   privada o de enlace local, o a un nombre `.local`), hace el emparejamiento y guarda el destino, el
//!   `server_id`, la huella y el nombre del servidor solo cuando el emparejamiento se aprueba.
//! - **Código** (D5): el protocolo y la explicación del compromiso están en
//!   `backend/internal/administration/pairing.go`. Aquí se genera el número aleatorio de la terminal, se
//!   envía su compromiso, se revela después de recibir el del servidor y se deriva el mismo código con la
//!   huella fijada: SHA-256(huella ‖ nT ‖ nS ‖ id) → 8 primeros bytes big-endian módulo 10⁶.
//! - **Credencial**: aprobada la solicitud, la terminal la reclama una sola vez y la guarda en el almacén
//!   seguro con una llave por `server_id` (`adel-server:{server_id}|{perfil}`); la credencial local de la
//!   computadora principal (`http://127.0.0.1:8181|main`) no cambia.

use crate::connection::{parse_config, ConnectionConfig};
use crate::installation::save_connection;
use crate::vault::{Credentials, Vault};
use crate::TerminalState;
use rustls::client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier};
use rustls::crypto::CryptoProvider;
use rustls::pki_types::{CertificateDer, ServerName, UnixTime};
use rustls::{CertificateError, DigitallySignedStruct, SignatureScheme};
use serde::Serialize;
use serde_json::Value;
use sha2::{Digest, Sha256};
use std::net::IpAddr;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;
use tauri::State;

/// Puerto de la escucha HTTPS del servidor en la red local (D1).
pub const DEFAULT_PORT: u16 = 8443;

/// Aviso cuando el servidor presenta otro certificado que el fijado al emparejar.
pub const IDENTITY_CHANGED_MESSAGE: &str = "El servidor cambió su identidad y esta terminal ya no puede comprobar que es el del restaurante: vuelve a emparejar esta terminal.";

const UNREACHABLE_MESSAGE: &str = "No se pudo conectar con la computadora principal en esa dirección. Revisa que esté encendida, en la misma red y que la dirección sea correcta.";
const CLIENT_KIND: &str = "desktop";

fn provider() -> Arc<CryptoProvider> {
    Arc::new(rustls::crypto::ring::default_provider())
}

/// Huella SPKI de un certificado en DER: SHA-256 de su SubjectPublicKeyInfo (lo mismo que calcula Go con
/// `RawSubjectPublicKeyInfo`).
pub fn spki_fingerprint(certificate: &CertificateDer<'_>) -> Result<[u8; 32], rustls::Error> {
    let parsed = rustls::server::ParsedCertificate::try_from(certificate)
        .map_err(|_| rustls::Error::InvalidCertificate(CertificateError::BadEncoding))?;
    let spki = parsed.subject_public_key_info();
    Ok(Sha256::digest(spki.as_ref()).into())
}

pub fn to_hex(bytes: &[u8]) -> String {
    bytes.iter().map(|byte| format!("{byte:02x}")).collect()
}

pub fn from_hex(text: &str) -> Option<Vec<u8>> {
    if text.len() % 2 != 0
        || !text
            .chars()
            .all(|character| character.is_ascii_digit() || ('a'..='f').contains(&character))
    {
        return None;
    }
    (0..text.len())
        .step_by(2)
        .map(|index| u8::from_str_radix(&text[index..index + 2], 16).ok())
        .collect()
}

pub fn fingerprint_from_hex(text: &str) -> Option<[u8; 32]> {
    let bytes = from_hex(text)?;
    bytes.try_into().ok()
}

/// Verificador TLS que compara la huella SPKI del certificado del servidor con la fijada e ignora el nombre
/// y la IP. Con `pinned = None` y `trust_first` (solo durante el emparejamiento) fija el primer certificado
/// de la sesión y exige el mismo en las conexiones siguientes.
#[derive(Debug)]
pub struct PinnedVerifier {
    pinned: Mutex<Option<[u8; 32]>>,
    trust_first: bool,
    mismatch: AtomicBool,
    provider: Arc<CryptoProvider>,
}

impl PinnedVerifier {
    /// Verificador de una terminal emparejada: solo la huella guardada.
    pub fn pinned(fingerprint: [u8; 32]) -> Self {
        Self {
            pinned: Mutex::new(Some(fingerprint)),
            trust_first: false,
            mismatch: AtomicBool::new(false),
            provider: provider(),
        }
    }

    /// Verificador de una sesión de emparejamiento: fija el primer certificado que ve.
    pub fn first_seen() -> Self {
        Self {
            pinned: Mutex::new(None),
            trust_first: true,
            mismatch: AtomicBool::new(false),
            provider: provider(),
        }
    }

    /// Huella fijada (la del primer certificado en una sesión de emparejamiento).
    pub fn fingerprint(&self) -> Option<[u8; 32]> {
        self.pinned.lock().ok().and_then(|value| *value)
    }

    /// La última conexión encontró un certificado distinto del fijado.
    pub fn identity_changed(&self) -> bool {
        self.mismatch.load(Ordering::SeqCst)
    }
}

impl ServerCertVerifier for PinnedVerifier {
    fn verify_server_cert(
        &self,
        end_entity: &CertificateDer<'_>,
        _intermediates: &[CertificateDer<'_>],
        _server_name: &ServerName<'_>,
        _ocsp_response: &[u8],
        _now: UnixTime,
    ) -> Result<ServerCertVerified, rustls::Error> {
        // El nombre, la IP, la cadena y la vigencia no cuentan: la confianza es la llave fijada.
        let seen = spki_fingerprint(end_entity)?;
        let mut pinned = self
            .pinned
            .lock()
            .map_err(|_| rustls::Error::General("verificador no disponible".into()))?;
        match *pinned {
            Some(expected) if expected == seen => {
                self.mismatch.store(false, Ordering::SeqCst);
                Ok(ServerCertVerified::assertion())
            }
            Some(_) => {
                self.mismatch.store(true, Ordering::SeqCst);
                Err(rustls::Error::InvalidCertificate(
                    CertificateError::ApplicationVerificationFailure,
                ))
            }
            None if self.trust_first => {
                *pinned = Some(seen);
                Ok(ServerCertVerified::assertion())
            }
            None => Err(rustls::Error::InvalidCertificate(
                CertificateError::ApplicationVerificationFailure,
            )),
        }
    }

    // La firma del intercambio se verifica siempre con la llave del certificado ya comparado con la huella.
    fn verify_tls12_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, rustls::Error> {
        rustls::crypto::verify_tls12_signature(
            message,
            cert,
            dss,
            &self.provider.signature_verification_algorithms,
        )
    }

    fn verify_tls13_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, rustls::Error> {
        rustls::crypto::verify_tls13_signature(
            message,
            cert,
            dss,
            &self.provider.signature_verification_algorithms,
        )
    }

    fn supported_verify_schemes(&self) -> Vec<SignatureScheme> {
        self.provider
            .signature_verification_algorithms
            .supported_schemes()
    }
}

/// Configuración TLS de la terminal con el verificador de huella: TLS 1.2 o 1.3 y sin reanudación de
/// sesiones, para que cada conexión vuelva a comprobar la llave.
pub fn client_config(verifier: Arc<PinnedVerifier>) -> Result<rustls::ClientConfig, String> {
    let mut config = rustls::ClientConfig::builder_with_provider(provider())
        .with_protocol_versions(&[&rustls::version::TLS13, &rustls::version::TLS12])
        .map_err(|_| "No se pudo preparar la conexión segura.")?
        .dangerous()
        .with_custom_certificate_verifier(verifier)
        .with_no_client_auth();
    config.resumption = rustls::client::Resumption::disabled();
    Ok(config)
}

/// Cliente HTTP con la huella fijada (terminal emparejada o sesión de emparejamiento).
pub fn pinned_client(verifier: Arc<PinnedVerifier>) -> Result<reqwest::Client, String> {
    reqwest::Client::builder()
        .timeout(Duration::from_secs(12))
        .redirect(reqwest::redirect::Policy::none())
        .no_proxy()
        .use_preconfigured_tls(client_config(verifier)?)
        .build()
        .map_err(|_| "No se pudo preparar la conexión segura.".into())
}

/// Host aceptable para una terminal emparejada (D2): IPv4 privada o de enlace local, IPv6 única local o de
/// enlace local, o un nombre `.local` (mDNS). Nunca loopback, una IP pública ni otro nombre.
pub fn valid_server_host(host: &str) -> bool {
    let bare = host.trim_start_matches('[').trim_end_matches(']');
    if let Ok(ip) = bare.parse::<IpAddr>() {
        return match ip {
            IpAddr::V4(value) => value.is_private() || value.is_link_local(),
            IpAddr::V6(value) => value.is_unique_local() || value.is_unicast_link_local(),
        };
    }
    let name = host.to_ascii_lowercase();
    let Some(stem) = name.strip_suffix(".local") else {
        return false;
    };
    !stem.is_empty()
        && name.len() <= 253
        && stem.split('.').all(|label| {
            !label.is_empty()
                && label.len() <= 63
                && !label.starts_with('-')
                && !label.ends_with('-')
                && label
                    .chars()
                    .all(|character| character.is_ascii_alphanumeric() || character == '-')
        })
}

/// Valida la dirección que propone React y devuelve la base `https://host:puerto` (puerto 8443 si no se
/// indicó). Acepta «192.168.1.20», «192.168.1.20:8443», «caja.local» o con «https://».
pub fn normalize_address(input: &str) -> Result<String, String> {
    let invalid =
        "Escribe la dirección de la computadora principal, por ejemplo 192.168.1.20 o caja.local.";
    let text = input.trim();
    if text.is_empty() || text.len() > 270 || text.chars().any(char::is_whitespace) {
        return Err(invalid.into());
    }
    let candidate = if text.contains("://") {
        text.to_string()
    } else {
        format!("https://{text}")
    };
    let url = reqwest::Url::parse(&candidate).map_err(|_| invalid.to_string())?;
    if url.scheme() != "https"
        || !url.username().is_empty()
        || url.password().is_some()
        || url.query().is_some()
        || url.fragment().is_some()
        || url.path() != "/"
    {
        return Err(invalid.into());
    }
    let host = url.host_str().ok_or_else(|| invalid.to_string())?;
    if !valid_server_host(host) {
        return Err(
            "La dirección debe ser de la red local del restaurante (por ejemplo 192.168.1.20 o caja.local)."
                .into(),
        );
    }
    let port = url.port().unwrap_or(DEFAULT_PORT);
    Ok(format!("https://{host}:{port}"))
}

/// Código de 6 dígitos: SHA-256(huella ‖ nT ‖ nS ‖ id en binario), 8 primeros bytes big-endian módulo 10⁶.
pub fn pairing_code(
    fingerprint: &[u8; 32],
    terminal_nonce: &[u8; 32],
    server_nonce: &[u8; 32],
    request_id: &str,
) -> Result<String, String> {
    let compact: String = request_id.chars().filter(|value| *value != '-').collect();
    let identifier = from_hex(&compact.to_ascii_lowercase())
        .filter(|bytes| bytes.len() == 16 && request_id.len() == 36)
        .ok_or("La solicitud de emparejamiento no es válida.")?;
    let mut digest = Sha256::new();
    digest.update(fingerprint);
    digest.update(terminal_nonce);
    digest.update(server_nonce);
    digest.update(&identifier);
    let sum = digest.finalize();
    let mut first = [0u8; 8];
    first.copy_from_slice(&sum[..8]);
    Ok(format!("{:06}", u64::from_be_bytes(first) % 1_000_000))
}

/// Compromiso del número de la terminal: SHA-256(nT) en hexadecimal.
pub fn commitment(terminal_nonce: &[u8; 32]) -> String {
    to_hex(&Sha256::digest(terminal_nonce))
}

fn random_nonce() -> Result<[u8; 32], String> {
    let mut value = [0u8; 32];
    rustls::crypto::ring::default_provider()
        .secure_random
        .fill(&mut value)
        .map_err(|_| "No se pudo generar el código seguro.")?;
    Ok(value)
}

/// Nombre del sistema que la terminal informa al servidor.
pub fn os_label() -> &'static str {
    if cfg!(target_os = "macos") {
        "macOS"
    } else if cfg!(target_os = "windows") {
        "Windows"
    } else {
        "Linux"
    }
}

fn valid_name(name: &str) -> Result<String, String> {
    let trimmed = name.trim();
    if trimmed.is_empty() || trimmed.chars().count() > 80 || trimmed.chars().any(char::is_control) {
        return Err("Escribe un nombre para esta terminal (máximo 80 caracteres).".into());
    }
    Ok(trimmed.to_string())
}

/// Sesión de emparejamiento en curso (solo en Rust: el secreto de la solicitud nunca llega a React).
#[derive(Clone)]
pub struct PairingSession {
    base: String,
    client: reqwest::Client,
    fingerprint: [u8; 32],
    request_id: String,
    secret: String,
    server_id: String,
    server_name: String,
    code: String,
    name: String,
    expires_at: String,
}

/// Lo que ve React de la sesión: el código, el nombre, el estado y el nombre del restaurante.
#[derive(Serialize, Debug, PartialEq)]
pub struct PairingView {
    pub state: String,
    pub code: String,
    pub name: String,
    pub server_name: String,
    pub expires_at: String,
}

impl PairingSession {
    fn view(&self, state: &str) -> PairingView {
        PairingView {
            state: state.to_string(),
            code: self.code.clone(),
            name: self.name.clone(),
            server_name: self.server_name.clone(),
            expires_at: self.expires_at.clone(),
        }
    }
}

/// Petición JSON al servidor durante el emparejamiento. Devuelve el cuerpo de una respuesta 200 o el
/// mensaje de Go (en español) de un rechazo.
async fn call(
    client: &reqwest::Client,
    method: reqwest::Method,
    url: String,
    secret: Option<&str>,
    body: Option<Value>,
) -> Result<(u16, Value), String> {
    let mut request = client
        .request(method, url)
        .header("X-Client-Version", env!("CARGO_PKG_VERSION"))
        .header("X-Client-Kind", CLIENT_KIND)
        .timeout(Duration::from_secs(10));
    if let Some(secret) = secret {
        request = request.header("X-Pairing-Secret", secret);
    }
    if let Some(body) = body {
        request = request.json(&body);
    }
    let response = request
        .send()
        .await
        .map_err(|_| UNREACHABLE_MESSAGE.to_string())?;
    let status = response.status().as_u16();
    let bytes = response
        .bytes()
        .await
        .map_err(|_| "Se interrumpió la respuesta del servidor.".to_string())?;
    if bytes.len() > 64 * 1024 {
        return Err("La respuesta excede el límite admitido.".into());
    }
    let value: Value = serde_json::from_slice(&bytes)
        .map_err(|_| "El servidor no respondió como la computadora principal de AD-EL POS.")?;
    Ok((status, value))
}

fn server_message(value: &Value) -> String {
    value
        .pointer("/error/message")
        .and_then(Value::as_str)
        .unwrap_or("El servidor rechazó la solicitud.")
        .to_string()
}

fn text(value: &Value, key: &str) -> String {
    value
        .get(key)
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_string()
}

/// Pide el código: solicitud con el compromiso, número del servidor y revelación del número propio.
pub async fn request_pairing(base: &str, name: &str) -> Result<PairingSession, String> {
    let verifier = Arc::new(PinnedVerifier::first_seen());
    let client = pinned_client(Arc::clone(&verifier))?;
    let nonce = random_nonce()?;
    let (status, created) = call(
        &client,
        reqwest::Method::POST,
        format!("{base}/api/v1/pairing/requests"),
        None,
        Some(serde_json::json!({
            "name": name,
            "device_kind": CLIENT_KIND,
            "os": os_label(),
            "app_version": env!("CARGO_PKG_VERSION"),
            "commitment": commitment(&nonce),
        })),
    )
    .await?;
    if status != 200 {
        return Err(server_message(&created));
    }
    let fingerprint = verifier
        .fingerprint()
        .ok_or("No se pudo comprobar el certificado del servidor.")?;
    let request_id = text(&created, "request_id");
    let secret = text(&created, "pairing_secret");
    let server_nonce: [u8; 32] = from_hex(&text(&created, "server_nonce"))
        .and_then(|bytes| bytes.try_into().ok())
        .ok_or("El servidor no respondió como la computadora principal de AD-EL POS.")?;
    let server_id = text(&created, "server_id");
    if secret.len() != 43 || !valid_identifier(&server_id) || !valid_identifier(&request_id) {
        return Err("El servidor no respondió como la computadora principal de AD-EL POS.".into());
    }
    // El número propio se revela solo después de recibir el del servidor (compromiso previo).
    let (status, revealed) = call(
        &client,
        reqwest::Method::POST,
        format!("{base}/api/v1/pairing/requests/{request_id}/reveal"),
        Some(&secret),
        Some(serde_json::json!({"terminal_nonce": to_hex(&nonce)})),
    )
    .await?;
    if status != 200 {
        return Err(server_message(&revealed));
    }
    let code = pairing_code(&fingerprint, &nonce, &server_nonce, &request_id)?;
    Ok(PairingSession {
        base: base.to_string(),
        client,
        fingerprint,
        request_id,
        secret,
        server_id,
        server_name: text(&created, "server_name"),
        code,
        name: text(&created, "name"),
        expires_at: text(&created, "expires_at"),
    })
}

pub fn valid_identifier(text: &str) -> bool {
    text.len() == 36
        && text.chars().enumerate().all(|(index, character)| {
            if matches!(index, 8 | 13 | 18 | 23) {
                character == '-'
            } else {
                character.is_ascii_hexdigit()
            }
        })
}

/// Clave del almacén seguro de un servidor emparejado.
pub fn credential_server(server_id: &str) -> String {
    format!("adel-server:{server_id}")
}

/// Conexión de una terminal emparejada (lo que se guarda en `connection.json`).
pub fn paired_connection(
    current: &ConnectionConfig,
    session: &PairingSession,
) -> Result<ConnectionConfig, String> {
    let mut next = current.clone();
    next.installation_mode = "terminal-only".into();
    next.server_url = session.base.clone();
    next.ca_certificate = None;
    next.server_id = Some(session.server_id.clone());
    next.server_fingerprint = Some(to_hex(&session.fingerprint));
    let name: String = session.server_name.chars().take(100).collect();
    next.server_name =
        (!name.trim().is_empty() && !name.chars().any(char::is_control)).then_some(name);
    let text = serde_json::to_string(&next).map_err(|_| "No se pudo preparar la conexión.")?;
    parse_config(&text)
}

/// Conexión de una terminal que vuelve a buscar su servidor (para emparejarla de nuevo).
pub fn unpaired_connection(current: &ConnectionConfig) -> Result<ConnectionConfig, String> {
    let mut next = current.clone();
    next.installation_mode = "terminal-only".into();
    next.server_url = String::new();
    next.ca_certificate = None;
    next.server_id = None;
    next.server_fingerprint = None;
    next.server_name = None;
    let text = serde_json::to_string(&next).map_err(|_| "No se pudo preparar la conexión.")?;
    parse_config(&text)
}

fn session_of(state: &TerminalState) -> Result<PairingSession, String> {
    state
        .pairing
        .lock()
        .map_err(|_| "No se pudo consultar el emparejamiento.".to_string())?
        .clone()
        .ok_or_else(|| "No hay un emparejamiento en curso. Pide un código nuevo.".to_string())
}

fn store_session(state: &TerminalState, session: Option<PairingSession>) -> Result<(), String> {
    *state
        .pairing
        .lock()
        .map_err(|_| "No se pudo guardar el emparejamiento.".to_string())? = session;
    Ok(())
}

fn terminal_config(state: &TerminalState) -> Result<ConnectionConfig, String> {
    let config = state.config.as_ref().map_err(Clone::clone)?.clone();
    if config.installation_mode != "terminal-only" {
        return Err("Solo una computadora «Solo terminal» se empareja con un servidor.".into());
    }
    Ok(config)
}

/// Pide el código de emparejamiento a la computadora principal en la dirección escrita a mano.
#[tauri::command]
pub async fn pairing_start(
    state: State<'_, TerminalState>,
    address: String,
    name: String,
) -> Result<PairingView, String> {
    let config = terminal_config(&state)?;
    if config.server_id.is_some() {
        return Err("Esta terminal ya está emparejada con un servidor.".into());
    }
    let base = normalize_address(&address)?;
    let name = valid_name(&name)?;
    let session = request_pairing(&base, &name).await?;
    let view = session.view("pending");
    store_session(&state, Some(session))?;
    Ok(view)
}

/// Consulta el estado; aprobada la solicitud, reclama la credencial (una sola vez), la guarda en el
/// almacén seguro y escribe la conexión. Después React reinicia la aplicación.
#[tauri::command]
pub async fn pairing_status(state: State<'_, TerminalState>) -> Result<PairingView, String> {
    let session = session_of(&state)?;
    let (status, value) = call(
        &session.client,
        reqwest::Method::GET,
        format!(
            "{}/api/v1/pairing/requests/{}",
            session.base, session.request_id
        ),
        Some(&session.secret),
        None,
    )
    .await?;
    if status != 200 {
        return Err(server_message(&value));
    }
    let current = text(&value, "state");
    if current != "approved" {
        return Ok(session.view(&current));
    }
    let (status, claimed) = call(
        &session.client,
        reqwest::Method::POST,
        format!(
            "{}/api/v1/pairing/requests/{}/claim",
            session.base, session.request_id
        ),
        Some(&session.secret),
        Some(serde_json::json!({})),
    )
    .await?;
    if status != 200 {
        store_session(&state, None)?;
        return Err(server_message(&claimed));
    }
    let device_id = text(&claimed, "device_id");
    let device_token = text(&claimed, "device_token");
    if !valid_identifier(&device_id)
        || device_token.len() != 43
        || text(&claimed, "server_id") != session.server_id
    {
        store_session(&state, None)?;
        return Err(
            "El servidor no entregó una autorización válida. Vuelve a emparejar esta terminal."
                .into(),
        );
    }
    let config = terminal_config(&state)?;
    let filename = state
        .installation
        .connection_file
        .clone()
        .ok_or("No se encontró dónde guardar la conexión de esta terminal.")?;
    let mut paired = session.clone();
    paired.name = text(&claimed, "name");
    let next = paired_connection(&config, &paired)?;
    // Primero la credencial (almacén seguro) y después la conexión: sin credencial guardada la terminal
    // sigue buscando servidor en lugar de quedar emparejada sin poder entrar.
    let vault = Vault::open(
        &credential_server(&session.server_id),
        &config.credential_namespace,
    )?;
    vault.save(Credentials {
        device_id: Some(device_id),
        device_token: Some(device_token),
        session_token: None,
    })?;
    save_connection(&filename, &next)?;
    store_session(&state, None)?;
    Ok(paired.view("paired"))
}

/// Cambia el nombre propuesto mientras la solicitud está pendiente.
#[tauri::command]
pub async fn pairing_rename(
    state: State<'_, TerminalState>,
    name: String,
) -> Result<PairingView, String> {
    let mut session = session_of(&state)?;
    let name = valid_name(&name)?;
    let (status, value) = call(
        &session.client,
        reqwest::Method::POST,
        format!(
            "{}/api/v1/pairing/requests/{}/name",
            session.base, session.request_id
        ),
        Some(&session.secret),
        Some(serde_json::json!({ "name": name })),
    )
    .await?;
    if status != 200 {
        return Err(server_message(&value));
    }
    session.name = text(&value, "name");
    let view = session.view("pending");
    store_session(&state, Some(session))?;
    Ok(view)
}

/// Cancela la solicitud en curso (si el servidor no responde, la solicitud vence sola).
#[tauri::command]
pub async fn pairing_cancel(state: State<'_, TerminalState>) -> Result<(), String> {
    let Ok(session) = session_of(&state) else {
        return Ok(());
    };
    store_session(&state, None)?;
    let _ = call(
        &session.client,
        reqwest::Method::POST,
        format!(
            "{}/api/v1/pairing/requests/{}/cancel",
            session.base, session.request_id
        ),
        Some(&session.secret),
        Some(serde_json::json!({})),
    )
    .await;
    Ok(())
}

/// Deja de usar el servidor emparejado para volver a emparejar la terminal (revocada o con el servidor
/// cambiado). Solo cambia la conexión; la credencial anterior queda sin uso en el almacén seguro. Después
/// React reinicia la aplicación.
#[tauri::command]
pub fn pairing_forget(state: State<'_, TerminalState>) -> Result<(), String> {
    let config = terminal_config(&state)?;
    let filename = state
        .installation
        .connection_file
        .clone()
        .ok_or("No se encontró dónde guardar la conexión de esta terminal.")?;
    save_connection(&filename, &unpaired_connection(&config)?)
}

/// Verificador de la conexión vigente, para avisar que el servidor cambió de identidad.
pub fn identity_changed(verifier: &Option<Arc<PinnedVerifier>>) -> bool {
    verifier
        .as_ref()
        .map(|value| value.identity_changed())
        .unwrap_or(false)
}

#[cfg(test)]
mod tests {
    use super::*;
    use rustls::pki_types::PrivateKeyDer;
    use std::io::{Read, Write};

    // Dos certificados ECDSA P-256 autofirmados de prueba (openssl), con nombres y direcciones distintos:
    // A dice ser «servidor-a» en 192.168.1.20; B, «servidor-b» en 10.0.0.9.
    const CERT_A: &str = include_str!("testdata/server-a.cert.hex");
    const KEY_A: &str = include_str!("testdata/server-a.key.hex");
    const CERT_B: &str = include_str!("testdata/server-b.cert.hex");
    const KEY_B: &str = include_str!("testdata/server-b.key.hex");

    fn der(text: &str) -> Vec<u8> {
        from_hex(text.trim()).expect("hexadecimal de prueba")
    }

    fn certificate(text: &str) -> CertificateDer<'static> {
        CertificateDer::from(der(text))
    }

    /// Intercambio TLS completo en memoria entre el cliente con el verificador y un servidor rustls.
    fn handshake(
        verifier: Arc<PinnedVerifier>,
        cert: &str,
        key: &str,
        name: &str,
    ) -> Result<(), rustls::Error> {
        let server_config = rustls::ServerConfig::builder_with_provider(provider())
            .with_protocol_versions(&[&rustls::version::TLS13, &rustls::version::TLS12])
            .unwrap()
            .with_no_client_auth()
            .with_single_cert(
                vec![certificate(cert)],
                PrivateKeyDer::try_from(der(key)).unwrap(),
            )
            .unwrap();
        let client_config = client_config(verifier).unwrap();
        let mut client = rustls::ClientConnection::new(
            Arc::new(client_config),
            ServerName::try_from(name.to_string()).unwrap(),
        )?;
        let mut server = rustls::ServerConnection::new(Arc::new(server_config))?;
        for _ in 0..20 {
            let mut buffer = Vec::new();
            while client.wants_write() {
                client.write_tls(&mut buffer).unwrap();
            }
            if !buffer.is_empty() {
                server.read_tls(&mut buffer.as_slice()).unwrap();
                server.process_new_packets()?;
            }
            let mut back = Vec::new();
            while server.wants_write() {
                server.write_tls(&mut back).unwrap();
            }
            if !back.is_empty() {
                client.read_tls(&mut back.as_slice()).unwrap();
                client.process_new_packets()?;
            }
            if !client.is_handshaking() && !server.is_handshaking() {
                // Los datos de la aplicación también pasan con la conexión ya verificada.
                client.writer().write_all(b"hola").unwrap();
                let mut buffer = Vec::new();
                while client.wants_write() {
                    client.write_tls(&mut buffer).unwrap();
                }
                server.read_tls(&mut buffer.as_slice()).unwrap();
                server.process_new_packets()?;
                let mut received = [0u8; 4];
                server.reader().read_exact(&mut received).unwrap();
                assert_eq!(&received, b"hola");
                return Ok(());
            }
        }
        panic!("el intercambio TLS no terminó");
    }

    fn fingerprint_of(cert: &str) -> [u8; 32] {
        spki_fingerprint(&certificate(cert)).unwrap()
    }

    #[test]
    fn the_pinned_fingerprint_connects_even_with_another_name_or_ip() {
        let pinned = fingerprint_of(CERT_A);
        // Nombre e IP distintos de los del certificado: se aceptan porque la llave es la fijada.
        for name in [
            "192.168.1.77",
            "10.0.0.5",
            "caja-principal.local",
            "servidor-a",
        ] {
            let verifier = Arc::new(PinnedVerifier::pinned(pinned));
            handshake(Arc::clone(&verifier), CERT_A, KEY_A, name).unwrap();
            assert!(!verifier.identity_changed(), "{name}");
        }
    }

    #[test]
    fn a_changed_certificate_is_rejected_and_reported() {
        let verifier = Arc::new(PinnedVerifier::pinned(fingerprint_of(CERT_A)));
        // B presenta un certificado válido para su propio nombre, pero no es la llave fijada.
        let error = handshake(Arc::clone(&verifier), CERT_B, KEY_B, "servidor-b").unwrap_err();
        assert!(
            matches!(error, rustls::Error::InvalidCertificate(_)),
            "{error:?}"
        );
        assert!(verifier.identity_changed());
        assert!(identity_changed(&Some(Arc::clone(&verifier))));
        // De vuelta con el servidor correcto, el aviso desaparece.
        handshake(Arc::clone(&verifier), CERT_A, KEY_A, "192.168.1.20").unwrap();
        assert!(!verifier.identity_changed());
    }

    #[test]
    fn the_certificate_of_the_pin_with_another_private_key_fails_the_signature() {
        // Quien copia el certificado A sin su llave privada (aquí, firma con la de B) no completa el
        // intercambio: la firma se verifica con la llave fijada.
        let verifier = Arc::new(PinnedVerifier::pinned(fingerprint_of(CERT_A)));
        let server_config = rustls::ServerConfig::builder_with_provider(provider())
            .with_protocol_versions(&[&rustls::version::TLS13, &rustls::version::TLS12])
            .unwrap()
            .with_no_client_auth()
            .with_single_cert(
                vec![certificate(CERT_A)],
                PrivateKeyDer::try_from(der(KEY_B)).unwrap(),
            );
        // rustls ya rechaza al configurar un certificado con una llave que no le corresponde; si algún día
        // lo permitiera, el intercambio debe fallar en la firma.
        if let Ok(config) = server_config {
            let mut client = rustls::ClientConnection::new(
                Arc::new(client_config(verifier).unwrap()),
                ServerName::try_from("192.168.1.20").unwrap(),
            )
            .unwrap();
            let mut server = rustls::ServerConnection::new(Arc::new(config)).unwrap();
            let mut failed = false;
            for _ in 0..20 {
                let mut buffer = Vec::new();
                while client.wants_write() {
                    client.write_tls(&mut buffer).unwrap();
                }
                server.read_tls(&mut buffer.as_slice()).unwrap();
                if server.process_new_packets().is_err() {
                    failed = true;
                    break;
                }
                let mut back = Vec::new();
                while server.wants_write() {
                    server.write_tls(&mut back).unwrap();
                }
                client.read_tls(&mut back.as_slice()).unwrap();
                if client.process_new_packets().is_err() {
                    failed = true;
                    break;
                }
            }
            assert!(failed, "una firma con otra llave no debe aceptarse");
        }
    }

    #[test]
    fn a_pairing_session_pins_the_first_certificate() {
        let verifier = Arc::new(PinnedVerifier::first_seen());
        handshake(Arc::clone(&verifier), CERT_A, KEY_A, "192.168.1.20").unwrap();
        assert_eq!(verifier.fingerprint(), Some(fingerprint_of(CERT_A)));
        // Un intermediario que aparece a mitad de la sesión con otro certificado se rechaza.
        assert!(handshake(Arc::clone(&verifier), CERT_B, KEY_B, "192.168.1.20").is_err());
        assert!(verifier.identity_changed());
    }

    #[test]
    fn pairing_code_matches_the_go_reference_vector() {
        let mut fingerprint = [0u8; 32];
        let mut terminal = [0u8; 32];
        let mut server = [0u8; 32];
        for index in 0..32 {
            fingerprint[index] = index as u8;
            terminal[index] = 32 + index as u8;
            server[index] = 64 + index as u8;
        }
        let request = "00000000-0000-4000-8000-000000000001";
        assert_eq!(
            pairing_code(&fingerprint, &terminal, &server, request).unwrap(),
            "166101"
        );
        assert_eq!(
            commitment(&terminal),
            "72dbb7336c76780023f83da4c355f2eeea85733b13d3477697917790c1229084"
        );
        fingerprint[0] = 0xff;
        assert_eq!(
            pairing_code(&fingerprint, &terminal, &server, request).unwrap(),
            "089301"
        );
        assert!(pairing_code(&fingerprint, &terminal, &server, "no-es-un-uuid").is_err());
        let nonce = random_nonce().unwrap();
        assert_ne!(nonce, random_nonce().unwrap());
    }

    #[test]
    fn only_local_network_addresses_are_accepted() {
        for (input, expected) in [
            ("192.168.1.20", "https://192.168.1.20:8443"),
            ("192.168.1.20:9443", "https://192.168.1.20:9443"),
            ("https://10.0.0.5", "https://10.0.0.5:8443"),
            ("172.20.1.1", "https://172.20.1.1:8443"),
            ("169.254.3.4", "https://169.254.3.4:8443"),
            ("caja-principal.local", "https://caja-principal.local:8443"),
            ("Caja.Local:8443", "https://caja.local:8443"),
            ("[fd00::5]", "https://[fd00::5]:8443"),
            (" 192.168.1.20 ", "https://192.168.1.20:8443"),
        ] {
            assert_eq!(normalize_address(input).unwrap(), expected, "{input}");
        }
        for input in [
            "",
            "8.8.8.8",
            "127.0.0.1",
            "localhost",
            "[::1]",
            "example.com",
            "servidor",
            "http://192.168.1.20",
            "https://192.168.1.20/admin",
            "https://user:clave@192.168.1.20",
            "192.168.1.20?x=1",
            "100.64.0.1",
            "[2001:db8::1]",
            ".local",
            "-caja.local",
            "caja .local",
            "file:///etc/passwd",
        ] {
            assert!(normalize_address(input).is_err(), "{input}");
        }
    }

    fn sample_config() -> ConnectionConfig {
        parse_config(r#"{"installation_mode":"terminal-only","server_url":""}"#).unwrap()
    }

    fn sample_session() -> PairingSession {
        PairingSession {
            base: "https://192.168.1.20:8443".into(),
            client: reqwest::Client::new(),
            fingerprint: [7u8; 32],
            request_id: "00000000-0000-4000-8000-000000000001".into(),
            secret: "s".repeat(43),
            server_id: "11111111-2222-4333-8444-555555555555".into(),
            server_name: "Fonda Doña Ñeli".into(),
            code: "123456".into(),
            name: "Caja 2".into(),
            expires_at: "2026-11-13T20:00:00Z".into(),
        }
    }

    #[test]
    fn the_paired_connection_keeps_only_the_closed_list_and_round_trips() {
        let paired = paired_connection(&sample_config(), &sample_session()).unwrap();
        assert_eq!(paired.installation_mode, "terminal-only");
        assert_eq!(paired.server_url, "https://192.168.1.20:8443");
        assert_eq!(
            paired.server_fingerprint.as_deref(),
            Some(to_hex(&[7u8; 32]).as_str())
        );
        assert_eq!(paired.server_name.as_deref(), Some("Fonda Doña Ñeli"));
        assert_eq!(
            paired.credential_server(),
            "adel-server:11111111-2222-4333-8444-555555555555"
        );
        let text = serde_json::to_string(&paired).unwrap();
        assert!(!text.contains("secret") && !text.contains("123456"));
        let reread = parse_config(&text).unwrap();
        assert_eq!(reread.server_id, paired.server_id);
        let forgotten = unpaired_connection(&paired).unwrap();
        assert!(forgotten.awaiting_server() && forgotten.server_id.is_none());
        assert_eq!(forgotten.credential_namespace, "main");
        // La vista para React no lleva el secreto ni datos técnicos.
        let view = sample_session().view("pending");
        assert_eq!(view.code, "123456");
        let shown = serde_json::to_string(&view).unwrap();
        assert!(!shown.contains(&"s".repeat(43)) && !shown.contains("192.168"));
    }

    /// Prueba contra la escucha de red real de un servidor Go temporal (no la instalación real): la corre a
    /// mano con ADEL_PAIRING_TEST_BASE (https://127.0.0.1:puerto) y
    /// ADEL_PAIRING_TEST_FINGERPRINT (huella del servidor). Pide un código por la conexión cifrada real,
    /// comprueba la huella fijada y que otra huella no conecta; imprime la solicitud y el código para que
    /// el script compare el código con los datos que guardó Go.
    #[test]
    #[ignore = "Necesita un servidor Go temporal con la escucha de red; ejecutar explícitamente."]
    fn live_pairing_against_a_temporary_go_server() {
        let base = std::env::var("ADEL_PAIRING_TEST_BASE").unwrap();
        let expected =
            fingerprint_from_hex(&std::env::var("ADEL_PAIRING_TEST_FINGERPRINT").unwrap()).unwrap();
        tauri::async_runtime::block_on(async move {
            let session = request_pairing(&base, "Caja Rust").await.unwrap();
            assert_eq!(
                session.fingerprint, expected,
                "la huella fijada es la del servidor"
            );
            println!(
                "PAIRING_REQUEST={} PAIRING_CODE={}",
                session.request_id, session.code
            );
            let (status, value) = call(
                &session.client,
                reqwest::Method::GET,
                format!("{base}/api/v1/pairing/requests/{}", session.request_id),
                Some(&session.secret),
                None,
            )
            .await
            .unwrap();
            assert_eq!(status, 200);
            assert_eq!(text(&value, "state"), "pending");
            let good = Arc::new(PinnedVerifier::pinned(expected));
            let response = pinned_client(Arc::clone(&good))
                .unwrap()
                .get(format!("{base}/api/v1/health/live"))
                .send()
                .await
                .unwrap();
            assert_eq!(response.status().as_u16(), 200);
            let mut other = expected;
            other[0] ^= 0xff;
            let wrong = Arc::new(PinnedVerifier::pinned(other));
            let refused = pinned_client(Arc::clone(&wrong))
                .unwrap()
                .get(format!("{base}/api/v1/health/live"))
                .send()
                .await;
            assert!(refused.is_err(), "otra huella no conecta");
            assert!(wrong.identity_changed());
        });
    }

    #[test]
    fn names_are_checked_before_asking_for_a_code() {
        assert_eq!(valid_name("  Caja 2 ").unwrap(), "Caja 2");
        assert!(valid_name("").is_err());
        assert!(valid_name("Caja\n2").is_err());
        assert!(valid_name(&"ñ".repeat(81)).is_err());
        assert!(valid_name(&"ñ".repeat(80)).is_ok());
    }
}
