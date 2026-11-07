#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod api;
mod connection;
mod vault;

use connection::{load_config, ConnectionConfig};
use serde::Serialize;
use std::sync::{Arc, OnceLock};
use std::time::Duration;
use tauri::{Manager, State};
use vault::Vault;

pub struct TerminalState {
    config: Result<ConnectionConfig, String>,
    client: Result<reqwest::Client, String>,
    vault: Arc<OnceLock<Result<Vault, String>>>,
    provisioning_token: Option<String>,
    enrollment_code: Option<String>,
}

impl TerminalState {
    fn vault(&self) -> Result<&Vault, String> {
        self.vault
            .get()
            .ok_or_else(|| {
                "Esperando autorización del almacén seguro del sistema. Revisa el diálogo de macOS."
                    .to_string()
            })?
            .as_ref()
            .map_err(Clone::clone)
    }
}

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

fn client(config: &ConnectionConfig) -> Result<reqwest::Client, String> {
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

fn main() {
    tauri::Builder::default()
        .setup(|app| {
            let config = load_config(app.path().app_config_dir().ok());
            let client = config.as_ref().map_err(Clone::clone).and_then(client);
            let vault = Arc::new(OnceLock::new());
            let pending_vault = Arc::clone(&vault);
            let vault_config = config.clone();
            // macOS puede esperar consentimiento para una compilación nueva. La espera no debe
            // bloquear el hilo de la ventana ni sustituir o borrar la credencial existente.
            std::thread::spawn(move || {
                let opened = vault_config
                    .as_ref()
                    .map_err(Clone::clone)
                    .and_then(|value| Vault::open(&value.server_url, &value.credential_namespace));
                let _ = pending_vault.set(opened);
            });
            let provisioning_token = std::env::var_os("ADEL_PROVISIONING_FILE")
                .and_then(|filename| std::fs::read_to_string(filename).ok())
                .map(|value| value.trim().to_string())
                .filter(|value| value.len() == 43);
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
            });
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            connection_config,
            server_snapshot,
            api::authentication_state,
            api::api_request,
        ])
        .run(tauri::generate_context!())
        .expect("No se pudo iniciar la aplicación de escritorio");
}

#[cfg(test)]
mod startup_tests {
    use super::*;

    #[test]
    fn pending_secure_storage_does_not_supply_empty_credentials() {
        let state = TerminalState {
            config: Err("Prueba sin configuración".into()),
            client: Err("Prueba sin HTTP".into()),
            vault: Arc::new(OnceLock::new()),
            provisioning_token: None,
            enrollment_code: None,
        };
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
