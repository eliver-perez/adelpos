fn main() {
    let attributes =
        tauri_build::Attributes::new().app_manifest(tauri_build::AppManifest::new().commands(&[
            "connection_config",
            "server_snapshot",
            "authentication_state",
            "api_request",
            "window_fullscreen_state",
            "window_set_fullscreen",
            "events_start",
            "events_stop",
        ]));
    tauri_build::try_build(attributes).expect("No se pudo preparar la aplicación Tauri");
}
