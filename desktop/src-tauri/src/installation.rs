//! Modalidad de esta computadora (etapa 9G, fase A; §2.3 y decisión D3).
//!
//! Tres modalidades, en una lista cerrada: «Servidor y terminal» (`server-terminal`), «Solo servidor»
//! (`server-only`) y «Solo terminal» (`terminal-only`). React solo elige una de ellas; Rust la guarda:
//!
//! - del lado de la terminal, en `connection.json` (la carpeta de configuración de la aplicación o
//!   `ADEL_TERMINAL_CONFIG`): con servidor, la dirección local `http://127.0.0.1:8181`; «Solo terminal»
//!   sin servidor emparejado, la dirección vacía (la búsqueda y el emparejamiento son de las fases B y C);
//! - del lado del servidor (solo macOS), en la carpeta de datos del servicio
//!   (`/Library/Application Support/AD-EL POS`): `installation-mode.json` y el archivo de habilitación
//!   `service-enabled`, que mantiene vivo el servicio (KeepAlive → PathState). El formato es el mismo de
//!   `backend/internal/standalone/mode.go` y del instalador. Crear o quitar el archivo no pide la
//!   contraseña de administrador: la carpeta es de la cuenta que instaló.
//!
//! Dejar de ser servidor nunca borra la base: el servidor deja de servir y conserva los datos. En Windows
//! la modalidad se elige en el instalador de la Fase W; hasta entonces aquí no se cambia.

use crate::connection::{parse_config, ConnectionConfig};
use crate::TerminalState;
use serde::Serialize;
use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};
use tauri::State;

pub const MODES: [&str; 3] = ["server-terminal", "server-only", "terminal-only"];
/// Dirección del servidor de la propia computadora (Go solo escucha en loopback).
pub const LOCAL_SERVER_URL: &str = "http://127.0.0.1:8181";
const MODE_FILE: &str = "installation-mode.json";
const ENABLE_FILE: &str = "service-enabled";
const CLUSTER_FOLDER: &str = "postgres";
const PROVISIONING_FILE: &str = "provisioning-token";
#[cfg_attr(not(target_os = "macos"), allow(dead_code))]
const SERVICE_LABEL: &str = "local.adel.pos.server";

/// Indica si la modalidad incluye el servidor.
pub fn includes_server(mode: &str) -> bool {
    mode == "server-terminal" || mode == "server-only"
}

fn check_mode(mode: &str) -> Result<(), String> {
    if MODES.contains(&mode) {
        Ok(())
    } else {
        Err("Modalidad desconocida.".into())
    }
}

/// Rutas del servicio del servidor de esta computadora.
#[derive(Clone)]
pub struct ServicePaths {
    pub data_dir: PathBuf,
    pub launch_daemon: PathBuf,
}

impl ServicePaths {
    /// Servicio de macOS: carpeta de datos del sistema y LaunchDaemon. `None` en otros sistemas (en
    /// Windows el servicio y su carpeta son de la Fase W).
    pub fn system() -> Option<Self> {
        #[cfg(target_os = "macos")]
        {
            let metadata: serde_json::Value =
                serde_json::from_str(include_str!("../../src/generated/product.json")).ok()?;
            let name = metadata.get("name")?.as_str()?;
            Some(Self {
                data_dir: PathBuf::from("/Library/Application Support").join(name),
                launch_daemon: PathBuf::from("/Library/LaunchDaemons")
                    .join(format!("{SERVICE_LABEL}.plist")),
            })
        }
        #[cfg(not(target_os = "macos"))]
        {
            None
        }
    }

    /// El instalador registró el servicio (LaunchDaemon y carpeta de datos).
    pub fn installed(&self) -> bool {
        self.launch_daemon.is_file() && self.data_dir.is_dir()
    }

    /// Existe el archivo de habilitación (un archivo normal, no un enlace).
    pub fn enabled(&self) -> bool {
        fs::symlink_metadata(self.data_dir.join(ENABLE_FILE))
            .map(|metadata| metadata.is_file())
            .unwrap_or(false)
    }

    /// La carpeta conserva la base de un servidor (el clúster de PostgreSQL).
    pub fn has_server_data(&self) -> bool {
        self.data_dir.join(CLUSTER_FOLDER).is_dir()
    }

    /// Token del asistente inicial que el servidor del servicio deja en su carpeta de datos (privado, de
    /// la misma cuenta): sirve para preparar el establecimiento desde esta computadora la primera vez.
    pub fn provisioning_token(&self) -> Option<String> {
        let text = fs::read_to_string(self.data_dir.join(PROVISIONING_FILE)).ok()?;
        let token = text.trim().to_string();
        (token.len() == 43).then_some(token)
    }

    /// Guarda la modalidad del lado del servidor y crea o quita el archivo de habilitación. Nunca toca la
    /// base ni otros archivos de la carpeta.
    pub fn apply(&self, mode: &str) -> Result<(), String> {
        check_mode(mode)?;
        let denied =
            "Solo la cuenta de la computadora que instaló AD-EL POS puede cambiar su modalidad.";
        let metadata = fs::symlink_metadata(&self.data_dir)
            .map_err(|_| "No se encontró la carpeta de datos del servidor de esta computadora.")?;
        if !metadata.is_dir() {
            return Err(
                "La carpeta de datos del servidor no es válida; pide ayuda a soporte.".into(),
            );
        }
        let body = format!("{{\"installation_mode\":\"{mode}\"}}\n");
        write_private(&self.data_dir.join(MODE_FILE), body.as_bytes()).map_err(|_| denied)?;
        let enable = self.data_dir.join(ENABLE_FILE);
        if includes_server(mode) {
            if !self.enabled() {
                write_private(&enable, b"1\n").map_err(|_| denied)?;
            }
        } else {
            match fs::remove_file(&enable) {
                Ok(()) => {}
                Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
                Err(_) => return Err(denied.into()),
            }
        }
        Ok(())
    }
}

/// Archivo de conexión que lee y escribe la aplicación: `ADEL_TERMINAL_CONFIG` o `connection.json` en la
/// carpeta de configuración.
pub fn connection_file(config_dir: Option<PathBuf>) -> Option<PathBuf> {
    if let Some(filename) = std::env::var_os("ADEL_TERMINAL_CONFIG") {
        return Some(PathBuf::from(filename));
    }
    config_dir.map(|directory| directory.join("connection.json"))
}

/// Lo que la aplicación sabe al iniciar: dónde guarda la conexión, si la modalidad ya se eligió y el
/// servicio de la computadora.
#[derive(Clone)]
pub struct Context {
    pub connection_file: Option<PathBuf>,
    pub chosen: bool,
    pub service: Option<ServicePaths>,
}

impl Context {
    pub fn detect(config_dir: Option<PathBuf>) -> Self {
        let connection_file = connection_file(config_dir);
        // En macOS la modalidad se elige en el primer arranque (sin connection.json); en Windows sigue
        // la de la instalación de la 9E hasta la Fase W.
        let chosen = !cfg!(target_os = "macos")
            || connection_file
                .as_ref()
                .map(|filename| filename.is_file())
                .unwrap_or(true);
        Self {
            connection_file,
            chosen,
            service: ServicePaths::system(),
        }
    }
}

impl Context {
    /// Token del asistente inicial del servidor de esta computadora, solo en una modalidad con servidor
    /// y con el servicio instalado.
    pub fn provisioning_token(&self, config: &ConnectionConfig) -> Option<String> {
        if !includes_server(&config.installation_mode) {
            return None;
        }
        let service = self.service.as_ref()?;
        if !service.installed() {
            return None;
        }
        service.provisioning_token()
    }
}

/// Estado de la modalidad que ve React (sin rutas ni datos técnicos).
#[derive(Serialize, Debug, PartialEq)]
pub struct InstallationState {
    pub chosen: bool,
    pub mode: String,
    /// La terminal ya tiene un servidor al que conectarse.
    pub server_configured: bool,
    /// Se puede cambiar la modalidad desde esta computadora (macOS).
    pub can_change: bool,
    pub service_installed: bool,
    pub service_enabled: bool,
    /// Sin servidor en esta modalidad, pero la carpeta conserva la base de cuando lo era.
    pub server_data_kept: bool,
    /// Etapa 9G (fase B): terminal emparejada con la computadora principal y el nombre del restaurante
    /// que informó el servidor al emparejarse.
    pub paired: bool,
    pub server_name: Option<String>,
}

pub fn state_of(context: &Context, config: &ConnectionConfig) -> InstallationState {
    let service = context.service.as_ref();
    let installed = service.map(ServicePaths::installed).unwrap_or(false);
    let enabled = service.map(ServicePaths::enabled).unwrap_or(false);
    let has_data = service.map(ServicePaths::has_server_data).unwrap_or(false);
    InstallationState {
        chosen: context.chosen,
        mode: config.installation_mode.clone(),
        server_configured: !config.awaiting_server(),
        can_change: service.is_some() && context.connection_file.is_some(),
        service_installed: installed,
        service_enabled: enabled,
        server_data_kept: !includes_server(&config.installation_mode) && has_data,
        paired: config.paired(),
        server_name: config.server_name.clone(),
    }
}

/// Conexión de la terminal para una modalidad: con servidor, el de esta computadora; «Solo terminal»
/// conserva el servidor emparejado si ya lo tenía y, si no, queda sin servidor.
pub fn connection_for(mode: &str, current: &ConnectionConfig) -> Result<ConnectionConfig, String> {
    check_mode(mode)?;
    let mut next = current.clone();
    next.installation_mode = mode.to_string();
    if includes_server(mode) {
        next.server_url = LOCAL_SERVER_URL.to_string();
        next.ca_certificate = None;
        clear_pairing(&mut next);
    } else if current.installation_mode != "terminal-only" {
        next.server_url = String::new();
        next.ca_certificate = None;
        clear_pairing(&mut next);
    }
    // Se valida con las mismas reglas que al leer el archivo.
    let text = serde_json::to_string(&next).map_err(|_| "No se pudo preparar la conexión.")?;
    parse_config(&text)
}

/// Una computadora con servidor se conecta a sí misma: deja de usar el servidor emparejado (etapa 9G).
fn clear_pairing(config: &mut ConnectionConfig) {
    config.server_id = None;
    config.server_fingerprint = None;
    config.server_name = None;
}

/// Escribe el archivo de conexión (0600 en macOS, reemplazo atómico).
pub fn save_connection(filename: &Path, config: &ConnectionConfig) -> Result<(), String> {
    let mut body =
        serde_json::to_vec_pretty(config).map_err(|_| "No se pudo preparar la conexión.")?;
    body.push(b'\n');
    if let Some(directory) = filename.parent() {
        fs::create_dir_all(directory)
            .map_err(|_| "No se pudo guardar la modalidad de esta terminal.")?;
    }
    write_private(filename, &body)
        .map_err(|_| "No se pudo guardar la modalidad de esta terminal.".into())
}

/// Escribe un archivo privado con reemplazo atómico: un temporal en la misma carpeta y `rename`.
fn write_private(filename: &Path, body: &[u8]) -> std::io::Result<()> {
    let directory = filename.parent().unwrap_or_else(|| Path::new("."));
    let name = filename
        .file_name()
        .and_then(|value| value.to_str())
        .unwrap_or("archivo");
    let temporary = directory.join(format!(".{name}.{}.tmp", std::process::id()));
    let result = (|| {
        let mut options = fs::OpenOptions::new();
        options.write(true).create_new(true);
        #[cfg(unix)]
        {
            use std::os::unix::fs::OpenOptionsExt;
            options.mode(0o600);
        }
        let mut file = options.open(&temporary)?;
        file.write_all(body)?;
        file.sync_all()?;
        fs::rename(&temporary, filename)
    })();
    if result.is_err() {
        let _ = fs::remove_file(&temporary);
    }
    result
}

/// Cambia la modalidad: primero el lado del servidor (si el servicio está instalado) y después la
/// conexión de la terminal, de modo que un permiso negado no deja la terminal a medias.
pub fn change_mode(
    context: &Context,
    current: &ConnectionConfig,
    mode: &str,
) -> Result<ConnectionConfig, String> {
    check_mode(mode)?;
    let service = context
        .service
        .as_ref()
        .ok_or("En esta computadora la modalidad todavía no se puede cambiar desde aquí.")?;
    let filename = context
        .connection_file
        .as_ref()
        .ok_or("No se encontró dónde guardar la modalidad de esta terminal.")?;
    let next = connection_for(mode, current)?;
    if service.installed() {
        service.apply(mode)?;
    }
    save_connection(filename, &next)?;
    Ok(next)
}

#[tauri::command]
pub fn installation_state(state: State<'_, TerminalState>) -> Result<InstallationState, String> {
    let config = state.config.as_ref().map_err(Clone::clone)?;
    Ok(state_of(&state.installation, config))
}

/// Guarda la modalidad elegida (primer arranque o Mantenimiento). La aplicación se reinicia después
/// (`restart_application`) para conectarse con la configuración nueva.
#[tauri::command]
pub fn set_installation_mode(state: State<'_, TerminalState>, mode: String) -> Result<(), String> {
    let config = state.config.as_ref().map_err(Clone::clone)?;
    change_mode(&state.installation, config, &mode).map(|_| ())
}

/// Reinicia la aplicación después de cambiar la modalidad (operación cerrada, sin argumentos).
#[tauri::command]
pub fn restart_application(app: tauri::AppHandle) {
    app.request_restart();
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temporary(name: &str) -> PathBuf {
        let directory = std::env::temp_dir().join(format!(
            "adel-installation-{name}-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        fs::create_dir_all(&directory).unwrap();
        directory
    }

    fn service(root: &Path) -> ServicePaths {
        let data_dir = root.join("Library/Application Support/AD-EL POS");
        let launch_daemon = root.join("Library/LaunchDaemons/local.adel.pos.server.plist");
        fs::create_dir_all(&data_dir).unwrap();
        fs::create_dir_all(launch_daemon.parent().unwrap()).unwrap();
        fs::write(&launch_daemon, "<plist/>").unwrap();
        ServicePaths {
            data_dir,
            launch_daemon,
        }
    }

    fn config(mode: &str, url: &str) -> ConnectionConfig {
        parse_config(&serde_json::json!({"installation_mode": mode, "server_url": url}).to_string())
            .unwrap()
    }

    #[test]
    fn modes_are_a_closed_list() {
        let current = config("server-terminal", LOCAL_SERVER_URL);
        assert!(connection_for("servidor", &current).is_err());
        assert!(connection_for("", &current).is_err());
        let root = temporary("closed");
        assert!(service(&root).apply("todo").is_err());
        fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn server_modes_connect_to_this_computer_and_terminal_waits_for_a_server() {
        let local = config("server-terminal", LOCAL_SERVER_URL);
        let server_only = connection_for("server-only", &local).unwrap();
        assert_eq!(server_only.installation_mode, "server-only");
        assert_eq!(server_only.server_url, LOCAL_SERVER_URL);
        let terminal = connection_for("terminal-only", &local).unwrap();
        assert!(
            terminal.awaiting_server(),
            "dejar de ser servidor no se conecta a sí mismo"
        );
        assert_eq!(terminal.credential_namespace, "main");
        // Una terminal ya emparejada conserva su servidor.
        let paired = config("terminal-only", "https://192.168.1.20:8443");
        assert_eq!(
            connection_for("terminal-only", &paired).unwrap().server_url,
            "https://192.168.1.20:8443"
        );
        let back = connection_for("server-terminal", &terminal).unwrap();
        assert_eq!(back.server_url, LOCAL_SERVER_URL);
    }

    // Etapa 9G, fase B: al pasar a una modalidad con servidor se deja el servidor emparejado; seguir como
    // terminal lo conserva.
    #[test]
    fn server_modes_forget_the_paired_server() {
        let text = serde_json::json!({
            "installation_mode": "terminal-only",
            "server_url": "https://192.168.1.20:8443",
            "server_id": "11111111-2222-4333-8444-555555555555",
            "server_fingerprint": "ab".repeat(32),
            "server_name": "Fonda",
        })
        .to_string();
        let paired = parse_config(&text).unwrap();
        let kept = connection_for("terminal-only", &paired).unwrap();
        assert!(kept.paired());
        let server = connection_for("server-terminal", &paired).unwrap();
        assert!(!server.paired() && server.server_name.is_none());
        assert_eq!(server.server_url, LOCAL_SERVER_URL);
    }

    #[test]
    fn apply_writes_the_mode_and_the_enable_file_without_touching_data() {
        let root = temporary("apply");
        let paths = service(&root);
        let cluster = paths.data_dir.join(CLUSTER_FOLDER);
        fs::create_dir_all(&cluster).unwrap();
        fs::write(cluster.join("PG_VERSION"), "18").unwrap();
        paths.apply("server-only").unwrap();
        assert!(paths.enabled());
        let stored = fs::read_to_string(paths.data_dir.join(MODE_FILE)).unwrap();
        assert_eq!(stored, "{\"installation_mode\":\"server-only\"}\n");
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            for file in [MODE_FILE, ENABLE_FILE] {
                let mode = fs::metadata(paths.data_dir.join(file))
                    .unwrap()
                    .permissions()
                    .mode();
                assert_eq!(mode & 0o777, 0o600, "{file}");
            }
        }
        paths.apply("server-terminal").unwrap();
        assert!(paths.enabled());
        paths.apply("terminal-only").unwrap();
        assert!(
            !paths.enabled(),
            "dejar de ser servidor quita el archivo de habilitación"
        );
        assert!(
            cluster.join("PG_VERSION").is_file(),
            "la base nunca se borra"
        );
        assert!(paths.has_server_data());
        paths.apply("terminal-only").unwrap();
        fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn change_mode_saves_both_sides_and_reports_the_state() {
        let root = temporary("change");
        let paths = service(&root);
        let filename = root.join("config/connection.json");
        let context = Context {
            connection_file: Some(filename.clone()),
            chosen: false,
            service: Some(paths.clone()),
        };
        let current = config("server-terminal", LOCAL_SERVER_URL);
        let next = change_mode(&context, &current, "terminal-only").unwrap();
        let saved = parse_config(&fs::read_to_string(&filename).unwrap()).unwrap();
        assert_eq!(saved.installation_mode, "terminal-only");
        assert!(saved.awaiting_server());
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mode = fs::metadata(&filename).unwrap().permissions().mode();
            assert_eq!(mode & 0o777, 0o600);
        }
        fs::create_dir_all(paths.data_dir.join(CLUSTER_FOLDER)).unwrap();
        let state = state_of(&context, &next);
        assert_eq!(
            state,
            InstallationState {
                chosen: false,
                mode: "terminal-only".into(),
                server_configured: false,
                can_change: true,
                service_installed: true,
                service_enabled: false,
                server_data_kept: true,
                paired: false,
                server_name: None,
            }
        );
        let server = change_mode(&context, &next, "server-terminal").unwrap();
        assert!(paths.enabled());
        assert!(!state_of(&context, &server).server_data_kept);
        fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn provisioning_token_comes_from_the_service_only_with_a_server_mode() {
        let root = temporary("provisioning");
        let paths = service(&root);
        let context = Context {
            connection_file: None,
            chosen: true,
            service: Some(paths.clone()),
        };
        let token = "a".repeat(43);
        fs::write(paths.data_dir.join(PROVISIONING_FILE), format!("{token}\n")).unwrap();
        let local = config("server-terminal", LOCAL_SERVER_URL);
        assert_eq!(context.provisioning_token(&local), Some(token));
        let terminal = config("terminal-only", "");
        assert_eq!(context.provisioning_token(&terminal), None);
        fs::write(paths.data_dir.join(PROVISIONING_FILE), "corto").unwrap();
        assert_eq!(context.provisioning_token(&local), None);
        fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn without_permission_nothing_changes() {
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let root = temporary("denied");
            let paths = service(&root);
            let filename = root.join("connection.json");
            let context = Context {
                connection_file: Some(filename.clone()),
                chosen: true,
                service: Some(paths.clone()),
            };
            fs::set_permissions(&paths.data_dir, fs::Permissions::from_mode(0o500)).unwrap();
            let current = config("server-terminal", LOCAL_SERVER_URL);
            let error = change_mode(&context, &current, "server-only")
                .err()
                .expect("sin permiso debe fallar");
            assert!(error.contains("cuenta"), "{error}");
            assert!(!filename.exists(), "la terminal no queda a medias");
            fs::set_permissions(&paths.data_dir, fs::Permissions::from_mode(0o700)).unwrap();
            fs::remove_dir_all(root).unwrap();
        }
    }

    #[test]
    fn without_the_service_only_the_terminal_changes() {
        let root = temporary("dev");
        let filename = root.join("connection.json");
        let missing = ServicePaths {
            data_dir: root.join("no-existe"),
            launch_daemon: root.join("no-existe.plist"),
        };
        let context = Context {
            connection_file: Some(filename.clone()),
            chosen: true,
            service: Some(missing.clone()),
        };
        let current = config("server-terminal", LOCAL_SERVER_URL);
        change_mode(&context, &current, "server-only").unwrap();
        assert!(
            !missing.data_dir.exists(),
            "sin servicio instalado no se crea su carpeta"
        );
        let windows = Context {
            connection_file: Some(filename),
            chosen: true,
            service: None,
        };
        assert!(change_mode(&windows, &current, "terminal-only").is_err());
        assert!(!state_of(&windows, &current).can_change);
        fs::remove_dir_all(root).unwrap();
    }
}
