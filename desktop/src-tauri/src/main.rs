#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod api;
// Etapa 9G, fase F: carpeta de los respaldos con el diálogo nativo (solo en la computadora principal).
mod backup_folder;
mod connection;
mod export;
mod installation;
mod menu;
mod pairing;
mod realtime;
mod server_host;
mod vault;
mod window_check;
#[cfg(target_os = "windows")]
mod window_state;

use connection::{load_config, ConnectionConfig};
use serde::Serialize;
use std::sync::atomic::{AtomicBool, AtomicU64};
use std::sync::{Arc, Mutex, OnceLock};
use std::time::Duration;
use tauri::{Manager, RunEvent, State};
use vault::Vault;

pub struct TerminalState {
    config: Result<ConnectionConfig, String>,
    client: Result<reqwest::Client, String>,
    vault: Arc<OnceLock<Result<Vault, String>>>,
    provisioning_token: Option<String>,
    enrollment_code: Option<String>,
    events_generation: Arc<AtomicU64>,
    // Servidor local de la instalación de Windows (etapa 9E); siempre None en macOS.
    host: Option<server_host::ServerHost>,
    // Modalidad de la computadora y servicio del servidor (etapa 9G, fase A; installation.rs).
    installation: installation::Context,
    // Etapa 9G (fase B): verificador con la huella fijada de una terminal emparejada (avisa si el servidor
    // cambió de identidad) y el emparejamiento en curso (pairing.rs).
    pin: Option<Arc<pairing::PinnedVerifier>>,
    pairing: Mutex<Option<pairing::PairingSession>>,
    // Una sola consulta de estado del emparejamiento a la vez (pairing_status).
    pairing_busy: AtomicBool,
}

impl TerminalState {
    fn vault(&self) -> Result<&Vault, String> {
        self.vault
            .get()
            .ok_or_else(|| STORAGE_PENDING_MESSAGE.to_string())?
            .as_ref()
            .map_err(Clone::clone)
    }

    /// Token del asistente inicial: el de ADEL_PROVISIONING_FILE (macOS y desarrollo) o, en la
    /// instalación de Windows, el que el servidor local deja en la carpeta de datos (etapa 9E); desde la
    /// 9G también el del servicio de macOS (installation.rs).
    pub fn provisioning_token(&self) -> Option<String> {
        self.provisioning_token
            .clone()
            .or_else(|| {
                self.host
                    .as_ref()
                    .and_then(|host| host.provisioning_token())
            })
            .or_else(|| {
                self.config
                    .as_ref()
                    .ok()
                    .and_then(|config| self.installation.provisioning_token(config))
            })
    }

    /// Explicación de una petición sin respuesta: la general o, mientras el servidor local de Windows
    /// arranca, que el equipo se está preparando.
    pub fn connection_error(&self) -> String {
        // Etapa 9G: el servidor emparejado presentó otro certificado; nunca se acepta.
        if pairing::identity_changed(&self.pin) {
            return pairing::IDENTITY_CHANGED_MESSAGE.to_string();
        }
        self.host
            .as_ref()
            .and_then(|host| host.connection_message())
            .unwrap_or(
                "No se pudo conectar con el servidor. Comprueba su estado y la confianza TLS.",
            )
            .to_string()
    }
}

// El almacén seguro es Keychain en macOS y el Administrador de credenciales en Windows (etapa 9E).
#[cfg(target_os = "macos")]
const STORAGE_PENDING_MESSAGE: &str =
    "Esperando autorización del almacén seguro del sistema. Revisa el diálogo de macOS.";
#[cfg(target_os = "windows")]
const STORAGE_PENDING_MESSAGE: &str =
    "Esperando autorización del almacén seguro del sistema. Revisa el aviso de Windows.";
#[cfg(not(any(target_os = "macos", target_os = "windows")))]
const STORAGE_PENDING_MESSAGE: &str = "Esperando autorización del almacén seguro del sistema.";

/// Una terminal «Solo terminal» que todavía no se conecta a ningún servidor (etapa 9G).
const AWAITING_SERVER_MESSAGE: &str = "Esta terminal todavía no está conectada a un servidor.";

#[derive(Serialize)]
pub struct HTTPResult {
    status: u16,
    body: serde_json::Value,
}

#[derive(Serialize)]
struct Snapshot {
    live: Option<HTTPResult>,
    ready: Option<HTTPResult>,
    version: Option<HTTPResult>,
}

#[tauri::command]
fn connection_config(state: State<'_, TerminalState>) -> Result<ConnectionConfig, String> {
    state.config.clone()
}

#[tauri::command]
fn window_fullscreen_state(window: tauri::WebviewWindow) -> Result<bool, String> {
    window
        .is_fullscreen()
        .map_err(|_| "No se pudo consultar la ventana.".into())
}

#[tauri::command]
fn window_set_fullscreen(window: tauri::WebviewWindow, fullscreen: bool) -> Result<(), String> {
    // La API nativa conserva los Spaces y controles normales de macOS; no es modo quiosco.
    window
        .set_fullscreen(fullscreen)
        .map_err(|_| "No se pudo cambiar la pantalla completa.".to_string())?;
    // Windows: al salir de pantalla completa vuelve el último tamaño y posición en modo ventana.
    #[cfg(target_os = "windows")]
    if !fullscreen {
        window_state::restore_windowed(&window);
    }
    Ok(())
}

async fn get(client: &reqwest::Client, base: &str, route: &str) -> Option<HTTPResult> {
    let mut response = client
        .get(format!("{base}/api/v1/{route}"))
        .send()
        .await
        .ok()?;
    let status = response.status().as_u16();
    let mut bytes = Vec::new();
    while let Some(chunk) = response.chunk().await.ok()? {
        if bytes.len() + chunk.len() > 32768 {
            return None;
        }
        bytes.extend_from_slice(&chunk);
    }
    Some(HTTPResult {
        status,
        body: serde_json::from_slice(&bytes).ok()?,
    })
}

#[tauri::command]
async fn server_snapshot(state: State<'_, TerminalState>) -> Result<Snapshot, String> {
    let config = state.config.as_ref().map_err(Clone::clone)?;
    let client = state.client.as_ref().map_err(Clone::clone)?;
    let live = get(client, &config.server_url, "health/live").await;
    if live.is_none() {
        return Ok(Snapshot {
            live,
            ready: None,
            version: None,
        });
    }
    let ready = get(client, &config.server_url, "health/ready").await;
    let version = get(client, &config.server_url, "version").await;
    Ok(Snapshot {
        live,
        ready,
        version,
    })
}

/// Cliente HTTP de la terminal: con un servidor emparejado, TLS con la huella fijada (etapa 9G, pairing.rs);
/// si no, el de siempre (loopback o una CA local configurada).
fn client(
    config: &ConnectionConfig,
) -> Result<(reqwest::Client, Option<Arc<pairing::PinnedVerifier>>), String> {
    if let Some(fingerprint) = config.server_fingerprint.as_deref() {
        let pinned = pairing::fingerprint_from_hex(fingerprint)
            .ok_or("La conexión con el servidor emparejado no es válida.")?;
        let verifier = Arc::new(pairing::PinnedVerifier::pinned(pinned));
        let client = pairing::pinned_client(Arc::clone(&verifier))?;
        return Ok((client, Some(verifier)));
    }
    unpinned_client(config).map(|client| (client, None))
}

fn unpinned_client(config: &ConnectionConfig) -> Result<reqwest::Client, String> {
    let mut builder = reqwest::Client::builder()
        .timeout(Duration::from_secs(12))
        .redirect(reqwest::redirect::Policy::none())
        .no_proxy();
    if let Some(filename) = &config.ca_certificate {
        let bytes = std::fs::read(filename).map_err(|_| "No se pudo leer la CA configurada.")?;
        let certificate = reqwest::Certificate::from_pem(&bytes)
            .map_err(|_| "El certificado de confianza no es válido.")?;
        builder = builder.add_root_certificate(certificate);
    }
    builder
        .build()
        .map_err(|_| "No se pudo preparar la conexión segura.".into())
}

/// Menú nativo en español (etapa 9C): Rust arma la lista cerrada de opciones y las envía a React por un
/// Channel; solo «Salir» se atiende aquí (en macOS no detiene Go ni PostgreSQL).
#[cfg(not(target_os = "windows"))]
fn with_native_menu(builder: tauri::Builder<tauri::Wry>) -> tauri::Builder<tauri::Wry> {
    builder.menu(menu::build).on_menu_event(menu::handle_event)
}

/// Windows (etapa 9E, §3.6): la barra de menú clásica aparecía también en pantalla completa, no es táctil y
/// repite lo que ofrecen la barra superior, la paleta de comandos (Ctrl+K) y los atajos con Ctrl, que atiende
/// la capa de React. Se omite: `menu_subscribe` y `menu_set_state` siguen respondiendo sin eventos.
#[cfg(target_os = "windows")]
fn with_native_menu(builder: tauri::Builder<tauri::Wry>) -> tauri::Builder<tauri::Wry> {
    builder
}

fn main() {
    let builder = tauri::Builder::default()
        // Diálogo nativo «Guardar como» de la exportación de reportes: solo lo usa Rust (export.rs);
        // la ventana no tiene permisos dialog:* en capabilities.
        .plugin(tauri_plugin_dialog::init())
        .manage(menu::MenuState::default());
    with_native_menu(builder)
        .setup(|app| {
            let config = load_config(app.path().app_config_dir().ok());
            let installation = installation::Context::detect(app.path().app_config_dir().ok());
            let prepared = config.as_ref().map_err(Clone::clone).and_then(client);
            let pin = prepared.as_ref().ok().and_then(|(_, pin)| pin.clone());
            let client = prepared.map(|(client, _)| client);
            let vault = Arc::new(OnceLock::new());
            let pending_vault = Arc::clone(&vault);
            let vault_config = config.clone();
            // macOS puede esperar consentimiento para una compilación nueva. La espera no debe
            // bloquear el hilo de la ventana ni sustituir o borrar la credencial existente.
            std::thread::spawn(move || {
                let opened = vault_config
                    .as_ref()
                    .map_err(Clone::clone)
                    .and_then(|value| {
                        // Etapa 9G: una terminal «Solo terminal» sin servidor todavía no tiene credencial.
                        if value.awaiting_server() {
                            return Err(AWAITING_SERVER_MESSAGE.to_string());
                        }
                        // Etapa 9G: la llave de un servidor emparejado es su identificador.
                        Vault::open(&value.credential_server(), &value.credential_namespace)
                    });
                let _ = pending_vault.set(opened);
            });
            let provisioning_token = std::env::var_os("ADEL_PROVISIONING_FILE")
                .and_then(|filename| std::fs::read_to_string(filename).ok())
                .map(|value| value.trim().to_string())
                .filter(|value| value.len() == 43);
            // Instalación de Windows: la aplicación lanza y vigila el servidor local (server_host.rs).
            let host = config.as_ref().ok().and_then(server_host::start);
            let enrollment_code = std::env::var_os("ADEL_ENROLLMENT_FILE")
                .and_then(|filename| std::fs::read_to_string(filename).ok())
                .map(|value| value.trim().to_string())
                .filter(|value| value.len() == 43);
            app.manage(TerminalState {
                config,
                client,
                vault,
                provisioning_token,
                enrollment_code,
                events_generation: Arc::new(AtomicU64::new(0)),
                host,
                installation,
                pin,
                pairing: Mutex::new(None),
                pairing_busy: AtomicBool::new(false),
            });
            window_check::start(app.handle().clone());
            // Windows: la ventana recuerda tamaño, posición y pantalla completa (etapa 9E, §3.7).
            #[cfg(target_os = "windows")]
            window_state::start(app.handle());
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            connection_config,
            window_fullscreen_state,
            window_set_fullscreen,
            realtime::events_start,
            realtime::events_stop,
            server_snapshot,
            api::authentication_state,
            api::api_request,
            export::export_report,
            menu::menu_subscribe,
            menu::menu_set_state,
            installation::installation_state,
            installation::set_installation_mode,
            installation::restart_application,
            pairing::pairing_start,
            pairing::pairing_status,
            pairing::pairing_rename,
            pairing::pairing_cancel,
            pairing::pairing_forget,
            backup_folder::choose_backup_directory,
        ])
        .build(tauri::generate_context!())
        .expect("No se pudo iniciar la aplicación de escritorio")
        .run(|app, event| {
            // Al salir, el servidor local de Windows se apaga en orden; en macOS no hay ninguno.
            if let RunEvent::Exit = event {
                if let Some(host) = app.state::<TerminalState>().host.as_ref() {
                    host.shutdown();
                }
            }
        });
}

#[cfg(test)]
pub(crate) mod startup_tests {
    use super::*;

    /// Estado de la terminal sin configuración ni almacén seguro (también para las pruebas de pairing.rs).
    pub(crate) fn test_state() -> TerminalState {
        TerminalState {
            config: Err("Prueba sin configuración".into()),
            client: Err("Prueba sin HTTP".into()),
            vault: Arc::new(OnceLock::new()),
            provisioning_token: None,
            enrollment_code: None,
            events_generation: Arc::new(AtomicU64::new(0)),
            host: None,
            installation: installation::Context {
                connection_file: None,
                chosen: true,
                service: None,
            },
            pin: None,
            pairing: Mutex::new(None),
            pairing_busy: AtomicBool::new(false),
        }
    }

    #[test]
    fn pending_secure_storage_does_not_supply_empty_credentials() {
        let state = test_state();
        assert!(state
            .vault()
            .err()
            .unwrap()
            .contains("Esperando autorización"));
        assert!(state
            .vault
            .set(Err("Acceso denegado por el sistema".into()))
            .is_ok());
        assert_eq!(
            state.vault().err().unwrap(),
            "Acceso denegado por el sistema"
        );
    }
}
