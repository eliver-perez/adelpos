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
            "export_report",
            "menu_subscribe",
            "menu_set_state",
            "installation_state",
            "set_installation_mode",
            "restart_application",
        ]));
    tauri_build::try_build(attributes).expect("No se pudo preparar la aplicación Tauri");
}
