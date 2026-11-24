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
            "pairing_start",
            "pairing_status",
            "pairing_rename",
            "pairing_cancel",
            "pairing_forget",
            "choose_backup_directory",
            "pairing_start_found",
            "discovery_search",
            // Etapa 9H: migración del sistema anterior (legacy_migration.rs).
            "legacy_choose_backup",
            "legacy_choose_package_output",
            "legacy_choose_package",
            "legacy_selection",
            "legacy_prepare_start",
            "legacy_prepare_watch",
            "legacy_prepare_cancel",
            "legacy_prepare_cleanup",
            "legacy_upload_package",
            "legacy_upload_cancel",
            "legacy_save_report",
            // Etapa L1: archivos de la licencia (license_files.rs).
            "license_save_request",
            "license_import_file",
            "license_import_unlock_file",
        ]));
    tauri_build::try_build(attributes).expect("No se pudo preparar la aplicación Tauri");
}
