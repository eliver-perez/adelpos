#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod connection;

use connection::{load_config, ConnectionConfig};
use serde::Serialize;
use std::time::Duration;
use tauri::{Manager, State};

struct TerminalState {
    config: Result<ConnectionConfig, String>,
    client: reqwest::Client,
}

#[derive(Serialize)]
struct HTTPResult {
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

// El WebView no decide rutas, métodos ni cuerpos. Este puente solo consulta salud.
async fn get(client: &reqwest::Client, base: &str, route: &str) -> Option<HTTPResult> {
    let mut response = client.get(format!("{base}/api/v1/{route}")).send().await.ok()?;
    let status = response.status().as_u16();
    let mut bytes = Vec::new();
    while let Some(chunk) = response.chunk().await.ok()? {
        if bytes.len() + chunk.len() > 32768 { return None; }
        bytes.extend_from_slice(&chunk);
    }
    Some(HTTPResult { status, body: serde_json::from_slice(&bytes).ok()? })
}

#[tauri::command]
async fn server_snapshot(state: State<'_, TerminalState>) -> Result<Snapshot, String> {
    let config = state.config.as_ref().map_err(Clone::clone)?;
    let live = get(&state.client, &config.server_url, "health/live").await;
    if live.is_none() { return Ok(Snapshot { live, ready: None, version: None }); }
    let ready = get(&state.client, &config.server_url, "health/ready").await;
    let version = get(&state.client, &config.server_url, "version").await;
    Ok(Snapshot { live, ready, version })
}

fn main() {
    tauri::Builder::default()
        .setup(|app| {
            let config = load_config(app.path().app_config_dir().ok());
            let client = reqwest::Client::builder()
                .timeout(Duration::from_secs(3))
                .redirect(reqwest::redirect::Policy::none())
                .no_proxy()
                .build()?;
            app.manage(TerminalState { config, client });
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![connection_config, server_snapshot])
        .run(tauri::generate_context!())
        .expect("No se pudo iniciar la aplicación de escritorio");
}
