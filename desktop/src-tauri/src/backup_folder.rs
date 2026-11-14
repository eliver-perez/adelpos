// Carpeta de los respaldos automáticos (etapa 9G, fase F): operación cerrada del puente nativo.
//
// La carpeta (otro disco o una USB) solo se elige en la computadora principal y solo con el diálogo nativo de carpeta
// (tauri-plugin-dialog, usado solo desde Rust: la ventana no tiene permisos dialog:* ni fs:*). React no envía ninguna
// ruta: pide abrir el diálogo y Rust envía a Go (PUT maintenance/backups/directory) exactamente la carpeta que eligió
// el administrador, con las credenciales que custodia (como api_request). Go vuelve a validarla (ruta absoluta,
// existente y con permiso de escritura para el servidor) y solo la acepta desde una conexión local. Si el diálogo se
// cancela, no se envía nada. React recibe {result: "cancelled"} o {result: "saved", state} con el estado de los
// respaldos que devuelve Go; un rechazo de Go llega como el de api_request ({status, body}).

use crate::{api, HTTPResult, TerminalState};
use reqwest::Url;
use serde_json::{json, Value};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use tauri::State;
use tauri_plugin_dialog::DialogExt;

// Un rechazo o el estado de los respaldos son JSON pequeños.
const RESPONSE_MAX_BYTES: usize = 2 * 1024 * 1024;

// Un solo diálogo de carpeta a la vez.
static CHOOSING: AtomicBool = AtomicBool::new(false);

// is_local_server indica si la terminal habla con el servidor de la propia computadora (loopback): solo ahí se elige
// la carpeta de los respaldos.
pub(crate) fn is_local_server(server_url: &str) -> bool {
    let Ok(url) = Url::parse(server_url) else {
        return false;
    };
    if url.scheme() != "http" && url.scheme() != "https" {
        return false;
    }
    matches!(
        url.host_str(),
        Some("127.0.0.1") | Some("[::1]") | Some("localhost")
    )
}

// directory_body arma el cuerpo para Go con la carpeta elegida: ruta absoluta en texto válido (una ruta que no se puede
// representar como texto no se envía).
pub(crate) fn directory_body(directory: &Path) -> Result<Value, String> {
    if !directory.is_absolute() {
        return Err("Elige una carpeta de esta computadora.".into());
    }
    let text = directory
        .to_str()
        .ok_or("El nombre de la carpeta no se puede usar; elige otra.")?;
    Ok(json!({ "directory": text }))
}

// ChoosingGuard libera el indicador del diálogo en curso al terminar, también con error.
struct ChoosingGuard;

impl ChoosingGuard {
    fn acquire() -> Result<Self, String> {
        if CHOOSING
            .compare_exchange(false, true, Ordering::AcqRel, Ordering::Acquire)
            .is_err()
        {
            return Err("Ya está abierto el diálogo para elegir la carpeta.".into());
        }
        Ok(Self)
    }
}

impl Drop for ChoosingGuard {
    fn drop(&mut self) {
        CHOOSING.store(false, Ordering::Release);
    }
}

// pick_folder abre el diálogo nativo de carpeta sobre la ventana principal. Se llama fuera del hilo principal
// (spawn_blocking), como exige blocking_pick_folder.
fn pick_folder(
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
) -> Result<Option<PathBuf>, String> {
    let chosen = app
        .dialog()
        .file()
        .set_parent(&window)
        .set_title("Elegir la carpeta de los respaldos")
        .set_can_create_directories(true)
        .blocking_pick_folder();
    match chosen {
        None => Ok(None),
        Some(path) => path
            .into_path()
            .map(Some)
            .map_err(|_| "La carpeta elegida no es válida.".to_string()),
    }
}

#[tauri::command]
pub async fn choose_backup_directory(
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
    state: State<'_, TerminalState>,
) -> Result<HTTPResult, String> {
    let config = state.config.as_ref().map_err(Clone::clone)?;
    if !is_local_server(&config.server_url) {
        return Err(
            "La carpeta de los respaldos solo se puede elegir en la computadora principal.".into(),
        );
    }
    let _guard = ChoosingGuard::acquire()?;
    let chosen = tauri::async_runtime::spawn_blocking(move || pick_folder(app, window))
        .await
        .map_err(|_| "No se pudo abrir el diálogo para elegir la carpeta.")??;
    let Some(directory) = chosen else {
        return Ok(HTTPResult {
            status: 200,
            body: json!({"result": "cancelled"}),
        });
    };
    let body = directory_body(&directory)?;
    let vault = state.vault()?;
    let client = state.client.as_ref().map_err(Clone::clone)?;
    let mut credentials = vault.read()?;
    let mut request = api::with_client_headers(client.put(format!(
        "{}/api/v1/maintenance/backups/directory",
        config.server_url
    )));
    if let (Some(id), Some(token)) = (&credentials.device_id, &credentials.device_token) {
        request = request
            .header("X-Device-ID", id)
            .header("X-Device-Token", token);
    }
    if let Some(token) = &credentials.session_token {
        request = request.bearer_auth(token);
    }
    let mut response = request
        .json(&body)
        .send()
        .await
        .map_err(|_| state.connection_error())?;
    let status = response.status().as_u16();
    let mut bytes = Vec::new();
    while let Some(chunk) = response
        .chunk()
        .await
        .map_err(|_| "Se interrumpió la respuesta del servidor.")?
    {
        if bytes.len() + chunk.len() > RESPONSE_MAX_BYTES {
            return Err("La respuesta excede el límite admitido.".into());
        }
        bytes.extend_from_slice(&chunk);
    }
    let mut result: Value = serde_json::from_slice(&bytes)
        .map_err(|_| "El servidor no respondió con un contrato válido.")?;
    if status != 200 {
        // Mismo contrato y misma invalidación de credenciales que api_request.
        if api::invalidates_credentials(status, &result) {
            credentials.session_token = None;
            let code = result
                .pointer("/error/code")
                .and_then(Value::as_str)
                .unwrap_or("");
            if code == "DEVICE_REVOKED" || code == "DEVICE_REQUIRED" {
                credentials.device_id = None;
                credentials.device_token = None;
            }
            vault.save(credentials)?;
        }
        api::sanitize_response(&mut result);
        return Ok(HTTPResult {
            status,
            body: result,
        });
    }
    api::sanitize_response(&mut result);
    Ok(HTTPResult {
        status: 200,
        body: json!({"result": "saved", "state": result}),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_the_local_server_can_choose_the_folder() {
        for local in [
            "http://127.0.0.1:8181",
            "http://localhost:8181",
            "http://[::1]:8181",
            "https://127.0.0.1:8443",
        ] {
            assert!(is_local_server(local), "{local}");
        }
        for remote in [
            "",
            "https://192.168.1.20:8443",
            "https://servidor.local:8443",
            "http://127.0.0.2:8181",
            "http://127.0.0.1.example.com:8181",
            "file:///tmp",
            "ftp://127.0.0.1",
            "no es una dirección",
        ] {
            assert!(!is_local_server(remote), "{remote}");
        }
    }

    #[test]
    fn only_absolute_text_folders_are_sent() {
        let absolute = std::env::temp_dir().join("Respaldos");
        assert_eq!(
            directory_body(&absolute).unwrap(),
            json!({"directory": absolute.to_str().unwrap()})
        );
        assert!(directory_body(Path::new("Respaldos")).is_err());
        assert!(directory_body(Path::new("./Respaldos")).is_err());
        #[cfg(unix)]
        {
            use std::ffi::OsStr;
            use std::os::unix::ffi::OsStrExt;
            let invalid = Path::new(OsStr::from_bytes(b"/Volumes/USB/\xff"));
            assert!(directory_body(invalid).is_err());
        }
    }

    #[test]
    fn only_one_dialog_at_a_time() {
        let first = ChoosingGuard::acquire().unwrap();
        assert!(ChoosingGuard::acquire().is_err());
        drop(first);
        assert!(ChoosingGuard::acquire().is_ok());
    }

    #[test]
    fn the_webview_still_has_no_dialog_or_filesystem_permissions() {
        let capability = include_str!("../capabilities/main.json");
        let permissions = include_str!("../permissions/diagnostics.toml");
        for forbidden in ["dialog:", "fs:", "allow-open", "allow-save"] {
            assert!(!capability.contains(forbidden), "{forbidden}");
            assert!(!permissions.contains(forbidden), "{forbidden}");
        }
        assert!(permissions.contains("\"choose_backup_directory\""));
    }
}
