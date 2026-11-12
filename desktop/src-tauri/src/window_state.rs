//! Tamaño y posición de la ventana en Windows (etapa 9E, §3.7).
//!
//! La aplicación sigue abriendo en pantalla completa (`tauri.conf.json`), igual que en macOS. En Windows
//! además recuerda, en `window-state.json` de la carpeta de configuración de la aplicación, si se cerró en
//! modo ventana y con qué tamaño, posición y maximización: al volver a abrir se restaura, y al salir de
//! pantalla completa la ventana regresa a la última geometría en modo ventana en lugar de la de fábrica.
//! Solo se guardan preferencias de la ventana, nunca datos del restaurante.

use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use std::sync::Mutex;
use tauri::{
    AppHandle, Manager, PhysicalPosition, PhysicalSize, Position, Size, WebviewWindow, WindowEvent,
};

/// Medida mínima que se acepta al restaurar (la de `minWidth`/`minHeight` de la ventana).
const MIN_WIDTH: u32 = 640;
const MIN_HEIGHT: u32 = 480;

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
pub struct Geometry {
    pub x: i32,
    pub y: i32,
    pub width: u32,
    pub height: u32,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
pub struct Saved {
    pub fullscreen: bool,
    pub maximized: bool,
    pub windowed: Option<Geometry>,
}

/// Última geometría en modo ventana de esta sesión (se actualiza al mover o cambiar el tamaño).
#[derive(Default)]
struct Tracker {
    windowed: Mutex<Option<Geometry>>,
}

fn state_file(app: &AppHandle) -> Option<PathBuf> {
    Some(app.path().app_config_dir().ok()?.join("window-state.json"))
}

/// Valida lo leído del archivo: medidas razonables o nada.
pub fn parse(text: &str) -> Option<Saved> {
    let saved: Saved = serde_json::from_str(text).ok()?;
    if let Some(geometry) = saved.windowed {
        if geometry.width < MIN_WIDTH
            || geometry.height < MIN_HEIGHT
            || geometry.width > 16384
            || geometry.height > 16384
        {
            return Some(Saved {
                windowed: None,
                ..saved
            });
        }
    }
    Some(saved)
}

/// La esquina superior izquierda debe caer en algún monitor conectado (si se desconectó un monitor, la
/// ventana no debe quedar fuera de la vista).
fn on_some_monitor(window: &WebviewWindow, geometry: &Geometry) -> bool {
    let Ok(monitors) = window.available_monitors() else {
        return false;
    };
    monitors.iter().any(|monitor| {
        let position = monitor.position();
        let size = monitor.size();
        geometry.x >= position.x
            && geometry.y >= position.y
            && geometry.x < position.x + size.width as i32
            && geometry.y < position.y + size.height as i32
    })
}

fn apply(window: &WebviewWindow, geometry: &Geometry) {
    let _ = window.set_size(Size::Physical(PhysicalSize::new(
        geometry.width,
        geometry.height,
    )));
    if on_some_monitor(window, geometry) {
        let _ = window.set_position(Position::Physical(PhysicalPosition::new(
            geometry.x, geometry.y,
        )));
    }
}

fn current_geometry(window: &WebviewWindow) -> Option<Geometry> {
    let position = window.outer_position().ok()?;
    let size = window.inner_size().ok()?;
    Some(Geometry {
        x: position.x,
        y: position.y,
        width: size.width,
        height: size.height,
    })
}

/// Restaura lo guardado y empieza a seguir los cambios de la ventana principal.
pub fn start(app: &AppHandle) {
    let Some(window) = app.get_webview_window("main") else {
        return;
    };
    let saved = state_file(app)
        .and_then(|file| std::fs::read_to_string(file).ok())
        .and_then(|text| parse(&text))
        .unwrap_or(Saved {
            fullscreen: true,
            ..Saved::default()
        });
    app.manage(Tracker {
        windowed: Mutex::new(saved.windowed),
    });
    if !saved.fullscreen {
        let _ = window.set_fullscreen(false);
        if let Some(geometry) = saved.windowed.as_ref() {
            apply(&window, geometry);
        }
        if saved.maximized {
            let _ = window.maximize();
        }
    }
    let handle = app.clone();
    let tracked = window.clone();
    window.on_window_event(move |event| match event {
        WindowEvent::Moved(_) | WindowEvent::Resized(_) => remember(&handle, &tracked),
        WindowEvent::CloseRequested { .. } => save(&handle, &tracked),
        _ => {}
    });
}

/// Guarda en memoria la geometría si la ventana está en modo ventana normal.
fn remember(app: &AppHandle, window: &WebviewWindow) {
    let fullscreen = window.is_fullscreen().unwrap_or(true);
    let maximized = window.is_maximized().unwrap_or(false);
    let minimized = window.is_minimized().unwrap_or(false);
    if fullscreen || maximized || minimized {
        return;
    }
    if let (Some(geometry), Some(tracker)) = (current_geometry(window), app.try_state::<Tracker>())
    {
        if let Ok(mut windowed) = tracker.windowed.lock() {
            *windowed = Some(geometry);
        }
    }
}

/// Escribe el estado al cerrar la ventana.
fn save(app: &AppHandle, window: &WebviewWindow) {
    let windowed = app
        .try_state::<Tracker>()
        .and_then(|tracker| tracker.windowed.lock().ok().map(|value| *value))
        .flatten();
    let saved = Saved {
        fullscreen: window.is_fullscreen().unwrap_or(true),
        maximized: window.is_maximized().unwrap_or(false),
        windowed,
    };
    let Some(file) = state_file(app) else {
        return;
    };
    if let Some(parent) = file.parent() {
        let _ = std::fs::create_dir_all(parent);
    }
    if let Ok(text) = serde_json::to_string_pretty(&saved) {
        let _ = std::fs::write(file, text);
    }
}

/// Al salir de pantalla completa, vuelve a la última geometría en modo ventana.
pub fn restore_windowed(window: &WebviewWindow) {
    let app = window.app_handle();
    let geometry = app
        .try_state::<Tracker>()
        .and_then(|tracker| tracker.windowed.lock().ok().map(|value| *value))
        .flatten();
    if let Some(geometry) = geometry.as_ref() {
        apply(window, geometry);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reads_saved_state_and_discards_unreasonable_sizes() {
        let saved = parse(
            r#"{"fullscreen":false,"maximized":false,"windowed":{"x":10,"y":20,"width":1280,"height":720}}"#,
        )
        .unwrap();
        assert!(!saved.fullscreen);
        assert_eq!(
            saved.windowed,
            Some(Geometry {
                x: 10,
                y: 20,
                width: 1280,
                height: 720
            })
        );
        let tiny = parse(
            r#"{"fullscreen":false,"maximized":false,"windowed":{"x":0,"y":0,"width":10,"height":10}}"#,
        )
        .unwrap();
        assert_eq!(tiny.windowed, None);
        assert!(parse("no es json").is_none());
        assert!(
            parse(r#"{"fullscreen":true,"maximized":false,"windowed":null}"#)
                .unwrap()
                .fullscreen
        );
    }
}
