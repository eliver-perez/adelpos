#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod api;
mod connection;
mod vault;

use connection::{load_config, ConnectionConfig};
use serde::Serialize;
use std::time::Duration;
use tauri::{Manager, State};
use vault::Vault;

pub struct TerminalState {
    config: Result<ConnectionConfig, String>,
    client: Result<reqwest::Client, String>,
    vault: Result<Vault, String>,
    provisioning_token: Option<String>,
    enrollment_code: Option<String>,
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
            let vault = config
                .as_ref()
                .map_err(Clone::clone)
                .and_then(|value| Vault::open(&value.server_url, &value.credential_namespace));
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
