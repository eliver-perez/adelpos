//! Verificación nativa opt-in para desarrollo; no altera el arranque normal ni usa credenciales.
use serde_json::json;
use std::time::Duration;
use tauri::{AppHandle, Manager};

pub fn start(app: AppHandle) {
    let Some(filename) = std::env::var_os("ADEL_WINDOW_VERIFICATION_FILE") else {
        return;
    };
    tauri::async_runtime::spawn(async move {
        let result = async {
            let window = app.get_webview_window("main").ok_or("No existe la ventana principal")?;
            tokio::time::sleep(Duration::from_secs(3)).await;
            let initial = window.is_fullscreen().map_err(|_| "No se pudo consultar pantalla completa")?;
            let first_size = window.inner_size().map_err(|_| "No se pudo consultar tamaño")?;
            window.set_fullscreen(false).map_err(|_| "No se pudo salir")?;
            tokio::time::sleep(Duration::from_secs(3)).await;
            let windowed = !window.is_fullscreen().map_err(|_| "No se pudo consultar modo ventana")?;
            window.set_fullscreen(true).map_err(|_| "No se pudo regresar")?;
            tokio::time::sleep(Duration::from_secs(3)).await;
            let returned = window.is_fullscreen().map_err(|_| "No se pudo consultar regreso")?;
            let last_size = window.inner_size().map_err(|_| "No se pudo consultar tamaño final")?;
            Ok::<_, &str>(json!({"initial_fullscreen":initial,"exited_to_window":windowed,"returned_fullscreen":returned,
                "initial_size":{"width":first_size.width,"height":first_size.height},
                "final_size":{"width":last_size.width,"height":last_size.height},"passed":initial&&windowed&&returned}))
        }.await;
        let report = match result {
            Ok(value) => value,
            Err(message) => json!({"passed":false,"error":message}),
        };
        if let Ok(text) = serde_json::to_string_pretty(&report) {
            let _ = std::fs::write(filename, text);
        }
    });
}
