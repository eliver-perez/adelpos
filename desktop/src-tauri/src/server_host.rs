//! Servidor local de la instalación de Windows (etapa 9E, §3.1).
//!
//! En macOS la aplicación nunca inicia ni detiene Go ni PostgreSQL (`CLAUDE.md` §3): `start` devuelve
//! `None` y nada de este módulo se ejecuta. En Windows, y solo cuando la aplicación está instalada con el
//! servidor y los binarios de PostgreSQL a su lado (el instalador NSIS los coloca así), la aplicación lanza
//! `ad-el-pos-server.exe host` sin ventana. Go crea y arranca PostgreSQL en la carpeta de datos del
//! usuario y sirve en `127.0.0.1:8181`; la entrada estándar de Go queda conectada a una tubería que solo
//! esta aplicación tiene abierta: al salir (también si la aplicación termina mal) Windows cierra la tubería
//! y Go se apaga en orden, con `pg_ctl stop`. En desarrollo (`tauri dev`) no hay servidor junto al
//! ejecutable y la aplicación usa el que se haya iniciado aparte, igual que en macOS.
//!
//! Un supervisor revisa cada medio segundo: si no responde ningún servidor en el puerto y no hay uno
//! propio en marcha, lanza otro (con espera creciente si los anteriores terminaron enseguida). Así, si se
//! abren dos ventanas, la segunda usa el servidor de la primera y lo sustituye cuando la primera se
//! cierra. El supervisor no ejecuta acciones de negocio ni toca la base: solo administra el proceso.

use crate::connection::ConnectionConfig;

/// Mensaje mientras el servidor propio arranca (el primer arranque crea la base y tarda más).
#[cfg_attr(not(target_os = "windows"), allow(dead_code))]
const STARTING_MESSAGE: &str = "AD-EL POS está preparando el servidor y la base de datos de este \
     equipo. La primera vez puede tardar hasta un minuto; la pantalla se actualiza sola.";

/// Mensaje si el servidor propio no logra quedar en marcha después de varios intentos.
#[cfg_attr(not(target_os = "windows"), allow(dead_code))]
const FAILED_MESSAGE: &str =
    "No se pudo iniciar el servidor de AD-EL POS en este equipo. Cierra y \
     vuelve a abrir la aplicación; si continúa, pide ayuda a soporte.";

pub use platform::ServerHost;

/// Prepara el servidor local si la aplicación es la instalación de Windows; en otro caso, `None`.
pub fn start(config: &ConnectionConfig) -> Option<ServerHost> {
    platform::start(config)
}

#[cfg(target_os = "windows")]
mod platform {
    use super::{ConnectionConfig, FAILED_MESSAGE, STARTING_MESSAGE};
    use std::net::{SocketAddr, TcpStream};
    use std::os::windows::process::CommandExt;
    use std::path::{Path, PathBuf};
    use std::process::{Child, Command, Stdio};
    use std::sync::atomic::{AtomicBool, Ordering};
    use std::sync::{Arc, Mutex};
    use std::thread;
    use std::time::{Duration, Instant};

    /// Proceso de consola sin ventana (CREATE_NO_WINDOW): ni Go ni PostgreSQL muestran consolas.
    const CREATE_NO_WINDOW: u32 = 0x0800_0000;
    /// Puerto fijo del servidor de la instalación (el mismo de macOS).
    const SERVER_PORT: u16 = 8181;
    /// Un servidor que termina antes de este tiempo cuenta como intento fallido.
    const QUICK_EXIT: Duration = Duration::from_secs(20);
    /// Intentos fallidos seguidos tras los cuales se informa la falla (el supervisor sigue intentando).
    const FAILURES_TO_REPORT: u32 = 3;
    /// Tiempo máximo que la aplicación espera, al salir, a que Go y PostgreSQL terminen en orden.
    const SHUTDOWN_WAIT: Duration = Duration::from_secs(25);

    #[derive(Default)]
    struct State {
        child: Option<Child>,
        started_at: Option<Instant>,
        failures: u32,
        next_attempt: Option<Instant>,
        ready: bool,
    }

    #[derive(Clone)]
    pub struct ServerHost {
        executable: PathBuf,
        data_dir: PathBuf,
        pg_bin: PathBuf,
        address: SocketAddr,
        state: Arc<Mutex<State>>,
        stopping: Arc<AtomicBool>,
    }

    /// Carpeta de datos: %LOCALAPPDATA%\AD-EL POS (la misma que usa Go por omisión), nunca la del programa.
    fn data_dir() -> Option<PathBuf> {
        let base = std::env::var_os("LOCALAPPDATA")?;
        let metadata: serde_json::Value =
            serde_json::from_str(include_str!("../../src/generated/product.json")).ok()?;
        let name = metadata.get("name")?.as_str()?;
        Some(PathBuf::from(base).join(name))
    }

    fn server_binary(directory: &Path) -> Option<PathBuf> {
        let metadata: serde_json::Value =
            serde_json::from_str(include_str!("../../src/generated/product.json")).ok()?;
        let name = metadata.get("server_binary")?.as_str()?;
        Some(directory.join(format!("{name}.exe")))
    }

    /// Dirección del servidor si la configuración apunta al servidor local de la instalación.
    fn local_address(config: &ConnectionConfig) -> Option<SocketAddr> {
        if config.installation_mode != "server-terminal" {
            return None;
        }
        let url = reqwest::Url::parse(&config.server_url).ok()?;
        if url.scheme() != "http" || url.port_or_known_default() != Some(SERVER_PORT) {
            return None;
        }
        match url.host_str()? {
            "127.0.0.1" | "localhost" => Some(SocketAddr::from(([127, 0, 0, 1], SERVER_PORT))),
            _ => None,
        }
    }

    pub fn start(config: &ConnectionConfig) -> Option<ServerHost> {
        let address = local_address(config)?;
        let directory = std::env::current_exe().ok()?.parent()?.to_path_buf();
        let executable = server_binary(&directory)?;
        let pg_bin = directory.join("postgresql").join("bin");
        // Solo la instalación empaquetada trae el servidor y PostgreSQL junto a la aplicación.
        if !executable.is_file() || !pg_bin.join("pg_ctl.exe").is_file() {
            return None;
        }
        let host = ServerHost {
            executable,
            data_dir: data_dir()?,
            pg_bin,
            address,
            state: Arc::new(Mutex::new(State::default())),
            stopping: Arc::new(AtomicBool::new(false)),
        };
        host.supervise();
        Some(host)
    }

    fn reachable(address: &SocketAddr) -> bool {
        TcpStream::connect_timeout(address, Duration::from_millis(250)).is_ok()
    }

    /// Espera antes de otro intento tras `failures` fallas seguidas: 1, 2, 4, 8, 16 y luego 30 s.
    pub(super) fn backoff(failures: u32) -> Duration {
        if failures == 0 {
            return Duration::ZERO;
        }
        Duration::from_secs((1u64 << (failures - 1).min(5)).min(30))
    }

    impl ServerHost {
        /// El proceso hijo hereda el entorno de la aplicación (no se limpia con `env_clear`): así
        /// `ADEL_TEST_MODE=1` de la prueba de humo llega a `host`, que entonces se niega a usar la carpeta
        /// de datos real del usuario (etapa 9F). La carpeta sale de `LOCALAPPDATA` (`data_dir`).
        fn spawn(&self) -> std::io::Result<Child> {
            Command::new(&self.executable)
                .arg("host")
                .arg("--data-dir")
                .arg(&self.data_dir)
                .arg("--pg-bin")
                .arg(&self.pg_bin)
                .arg("--watch-stdin")
                .current_dir(self.executable.parent().unwrap_or(Path::new(".")))
                .stdin(Stdio::piped())
                .stdout(Stdio::null())
                .stderr(Stdio::null())
                .creation_flags(CREATE_NO_WINDOW)
                .spawn()
        }

        fn supervise(&self) {
            // El clon comparte el estado y la marca de parada (Arc) con el ServerHost de la aplicación.
            let host = self.clone();
            thread::spawn(move || {
                while !host.stopping.load(Ordering::SeqCst) {
                    host.tick();
                    thread::sleep(Duration::from_millis(500));
                }
            });
        }

        fn tick(&self) {
            let ready = reachable(&self.address);
            let Ok(mut current) = self.state.lock() else {
                return;
            };
            current.ready = ready;
            if self.stopping.load(Ordering::SeqCst) {
                return;
            }
            // Un servidor propio que terminó: falla si duró poco; la espera crece con las fallas.
            let exited = match current.child.as_mut() {
                Some(child) => !matches!(child.try_wait(), Ok(None)),
                None => false,
            };
            if exited {
                current.child = None;
                let lived = current
                    .started_at
                    .map(|at| at.elapsed())
                    .unwrap_or_default();
                current.failures = if lived < QUICK_EXIT {
                    current.failures + 1
                } else {
                    0
                };
                current.next_attempt = Some(Instant::now() + backoff(current.failures));
            }
            if ready && current.child.is_some() {
                current.failures = 0;
            }
            let waiting = current
                .next_attempt
                .map(|at| Instant::now() < at)
                .unwrap_or(false);
            if current.child.is_none() && !ready && !waiting {
                match self.spawn() {
                    Ok(child) => {
                        current.child = Some(child);
                        current.started_at = Some(Instant::now());
                    }
                    Err(_) => {
                        current.failures += 1;
                        current.next_attempt = Some(Instant::now() + backoff(current.failures));
                    }
                }
            }
        }

        /// Mensaje para la pantalla de conexión cuando el servidor propio no responde todavía.
        pub fn connection_message(&self) -> Option<&'static str> {
            let current = self.state.lock().ok()?;
            if current.ready {
                return None;
            }
            if current.failures >= FAILURES_TO_REPORT {
                return Some(FAILED_MESSAGE);
            }
            Some(STARTING_MESSAGE)
        }

        /// Token del asistente inicial que Go deja en la carpeta de datos (en macOS llega por
        /// ADEL_PROVISIONING_FILE). Se lee cada vez: aparece cuando el servidor termina de prepararse.
        pub fn provisioning_token(&self) -> Option<String> {
            std::fs::read_to_string(self.data_dir.join("provisioning-token"))
                .ok()
                .map(|value| value.trim().to_string())
                .filter(|value| value.len() == 43)
        }

        /// Cierra la tubería de Go y espera a que Go y PostgreSQL terminen en orden. No fuerza la
        /// terminación: si algo tarda más, Go termina por su cuenta y el siguiente arranque lo adopta.
        pub fn shutdown(&self) {
            self.stopping.store(true, Ordering::SeqCst);
            let Some(mut child) = self
                .state
                .lock()
                .ok()
                .and_then(|mut current| current.child.take())
            else {
                return;
            };
            drop(child.stdin.take());
            let deadline = Instant::now() + SHUTDOWN_WAIT;
            while Instant::now() < deadline {
                if !matches!(child.try_wait(), Ok(None)) {
                    return;
                }
                thread::sleep(Duration::from_millis(100));
            }
        }
    }

    #[cfg(test)]
    mod tests {
        use super::*;

        fn config(mode: &str, url: &str) -> ConnectionConfig {
            crate::connection::parse_config(
                &serde_json::json!({"installation_mode": mode, "server_url": url}).to_string(),
            )
            .unwrap()
        }

        #[test]
        fn manages_only_the_local_installation_server() {
            assert!(local_address(&config("server-terminal", "http://127.0.0.1:8181")).is_some());
            assert!(local_address(&config("server-terminal", "http://localhost:8181")).is_some());
            assert!(local_address(&config("server-terminal", "http://127.0.0.1:8183")).is_none());
            assert!(local_address(&config("terminal-only", "http://127.0.0.1:8181")).is_none());
        }

        #[test]
        fn development_build_without_bundled_server_does_not_start_anything() {
            // `cargo test` corre junto a target\debug, donde no está el servidor empaquetado.
            assert!(start(&config("server-terminal", "http://127.0.0.1:8181")).is_none());
        }

        #[test]
        fn data_folder_is_local_app_data_with_the_product_name() {
            let folder = data_dir().unwrap();
            assert!(folder.ends_with("AD-EL POS"));
            assert!(folder.starts_with(std::env::var_os("LOCALAPPDATA").unwrap()));
        }

        #[test]
        fn retries_wait_longer_after_each_quick_failure() {
            let waits: Vec<u64> = (0..8).map(|n| backoff(n).as_secs()).collect();
            assert_eq!(waits, vec![0, 1, 2, 4, 8, 16, 30, 30]);
        }
    }
}

#[cfg(not(target_os = "windows"))]
mod platform {
    use super::ConnectionConfig;

    /// Fuera de Windows no existe un servidor administrado por la aplicación.
    pub struct ServerHost;

    pub fn start(_config: &ConnectionConfig) -> Option<ServerHost> {
        None
    }

    impl ServerHost {
        pub fn connection_message(&self) -> Option<&'static str> {
            None
        }

        pub fn provisioning_token(&self) -> Option<String> {
            None
        }

        pub fn shutdown(&self) {}
    }
}
