// Archivos de la licencia (etapa L1, Fase E): operaciones cerradas del puente nativo.
//
// Todo lo que toca archivos lo hace Rust, nunca React (regla de la etapa 9H, como legacy_migration.rs):
//
// - «Solicitud sin internet»: React elige la acción (activar, renovar o liberar) y, para activar, el código de la
//   licencia si lo tiene. Rust pide a Go la solicitud (POST license/offline-request) y la guarda con el diálogo «Guardar
//   como» (tauri-plugin-dialog, usado solo desde Rust: la ventana no tiene permisos dialog:* ni fs:*). React recibe solo
//   el nombre del archivo guardado; nunca el contenido ni la ruta.
// - «Importar archivo de licencia»: Rust abre el diálogo para elegir el .lic, lo lee (texto UTF-8 pequeño) y lo envía a
//   Go (POST license/import), que lo verifica como una respuesta en línea (§10 del contrato de licencias). React recibe
//   solo la respuesta de Go.
// - «Importar la respuesta de soporte» (desbloqueo de la fecha y hora, §6.4): igual, con el archivo .desbloqueo (POST
//   license/support-unlock con source «file»).
//
// Ninguna de estas rutas está en route() de api.rs. El código de la licencia nunca se escribe en registros ni en
// mensajes: solo viaja en el cuerpo de la petición a Go.

use crate::{export, legacy_migration, HTTPResult, TerminalState};
use reqwest::Method;
use serde_json::{json, Value};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use tauri::State;
use tauri_plugin_dialog::DialogExt;

const REQUEST_EXTENSION: &str = "licreq";
const LICENSE_EXTENSION: &str = "lic";
const UNLOCK_EXTENSION: &str = "desbloqueo";
// Una licencia firmada mide unos pocos KiB (el contrato limita los cuerpos a 16 KiB) y la respuesta de soporte, unos
// 600 caracteres: los topes dejan margen para los espacios y saltos de línea que agregue quien la copia.
const MAX_LICENSE_FILE_BYTES: u64 = 64 * 1024;
const MAX_UNLOCK_FILE_BYTES: u64 = 16 * 1024;
// El código de la licencia tiene 15 símbolos con sus guiones; Go valida el formato. Aquí solo se descarta un texto que
// no puede ser un código antes de enviarlo.
const MAX_LICENSE_KEY_CHARS: usize = 64;
const DEFAULT_REQUEST_NAME: &str = "solicitud-licencia";
const MAX_FILE_NAME_CHARS: usize = 100;
const INVALID_RESPONSE: &str = "El servidor no respondió con un contrato válido.";
const ACTIONS: &[&str] = &["activate", "renew", "deactivate"];

// Un solo diálogo de archivos de la licencia a la vez.
static CHOOSING: AtomicBool = AtomicBool::new(false);

struct ChoosingGuard;

impl ChoosingGuard {
    fn acquire() -> Result<Self, String> {
        if CHOOSING
            .compare_exchange(false, true, Ordering::AcqRel, Ordering::Acquire)
            .is_err()
        {
            return Err("Ya está abierto un diálogo para elegir un archivo.".into());
        }
        Ok(Self)
    }
}

impl Drop for ChoosingGuard {
    fn drop(&mut self) {
        CHOOSING.store(false, Ordering::Release);
    }
}

// request_body arma el cuerpo de la solicitud sin internet: la acción de la lista cerrada y, solo para activar, el
// código de la licencia (opcional). Go vuelve a validar las dos cosas.
pub(crate) fn request_body(action: &str, license_key: Option<&str>) -> Result<Value, String> {
    if !ACTIONS.contains(&action) {
        return Err("Elige activar, renovar o liberar.".into());
    }
    let key = license_key.map(str::trim).unwrap_or("");
    if key.is_empty() {
        return Ok(json!({ "action": action }));
    }
    if action != "activate" {
        return Err("El código de la licencia solo va en la solicitud de activación.".into());
    }
    if key.chars().count() > MAX_LICENSE_KEY_CHARS {
        return Err("El código de la licencia no tiene el formato correcto.".into());
    }
    Ok(json!({ "action": action, "license_key": key }))
}

// suggested_request_name es el nombre que propone el diálogo: el que sugiere Go («solicitud-activar-…»), solo con
// letras, números, puntos, guiones y guiones bajos, sin punto inicial y con la extensión .licreq.
pub(crate) fn suggested_request_name(filename: &str) -> String {
    let base = filename.rsplit(['/', '\\']).next().unwrap_or("");
    let cleaned: String = base
        .chars()
        .map(|character| {
            if character.is_ascii_alphanumeric() || matches!(character, '.' | '-' | '_') {
                character
            } else {
                '_'
            }
        })
        .collect();
    let suffix = format!(".{REQUEST_EXTENSION}");
    let mut stem = cleaned
        .strip_suffix(suffix.as_str())
        .unwrap_or(&cleaned)
        .trim_matches(|character| character == '.' || character == '_')
        .to_string();
    if stem.is_empty() {
        stem = DEFAULT_REQUEST_NAME.to_string();
    }
    let limit = MAX_FILE_NAME_CHARS - suffix.len();
    if stem.len() > limit {
        stem.truncate(limit);
    }
    format!("{stem}{suffix}")
}

fn has_extension(path: &Path, extension: &str) -> bool {
    path.extension()
        .and_then(|value| value.to_str())
        .map(|value| value.eq_ignore_ascii_case(extension))
        .unwrap_or(false)
}

fn file_name(path: &Path) -> String {
    path.file_name()
        .and_then(|name| name.to_str())
        .unwrap_or("")
        .to_string()
}

// request_destination decide dónde se guarda la solicitud: fuerza la extensión .licreq. El panel de macOS ya confirmó
// reemplazar el nombre que mostró; si Rust agregó la extensión y ese archivo existe, no se reemplaza sin preguntar.
pub(crate) fn request_destination(path: PathBuf) -> Result<PathBuf, String> {
    if !path.is_absolute() || path.to_str().is_none() {
        return Err("Elige una carpeta de esta computadora o de una USB.".into());
    }
    if has_extension(&path, REQUEST_EXTENSION) {
        return Ok(path);
    }
    let mut text = path.into_os_string();
    text.push(".");
    text.push(REQUEST_EXTENSION);
    let path = PathBuf::from(text);
    if path.exists() {
        return Err(
            "Ya existe un archivo con ese nombre. Vuelve a guardar la solicitud y elige otro nombre."
                .into(),
        );
    }
    Ok(path)
}

// FileKind describe un archivo que se importa: su extensión, su tope y el mensaje si no es el archivo esperado.
pub(crate) struct FileKind {
    extension: &'static str,
    limit: u64,
    wrong_file: &'static str,
}

pub(crate) const LICENSE_FILE: FileKind = FileKind {
    extension: LICENSE_EXTENSION,
    limit: MAX_LICENSE_FILE_BYTES,
    wrong_file: "Elige el archivo de licencia (un archivo terminado en .lic) que te enviaron.",
};

pub(crate) const UNLOCK_FILE: FileKind = FileKind {
    extension: UNLOCK_EXTENSION,
    limit: MAX_UNLOCK_FILE_BYTES,
    wrong_file: "Elige la respuesta de soporte (un archivo terminado en .desbloqueo).",
};

// read_text_file lee el archivo elegido: con su extensión, no vacío, dentro del tope y con texto UTF-8. Go lo vuelve a
// verificar por completo (firma, destinatario y revisión).
pub(crate) fn read_text_file(path: &Path, kind: &FileKind) -> Result<String, String> {
    if !path.is_absolute() || !has_extension(path, kind.extension) {
        return Err(kind.wrong_file.into());
    }
    let metadata = std::fs::metadata(path).map_err(|_| "No se pudo leer el archivo elegido.")?;
    if !metadata.is_file() || metadata.len() == 0 || metadata.len() > kind.limit {
        return Err(kind.wrong_file.into());
    }
    let bytes = std::fs::read(path).map_err(|_| "No se pudo leer el archivo elegido.")?;
    let text = String::from_utf8(bytes).map_err(|_| kind.wrong_file.to_string())?;
    if text.trim().is_empty() {
        return Err(kind.wrong_file.into());
    }
    Ok(text)
}

// Los diálogos se abren fuera del hilo principal (spawn_blocking), como exigen las funciones blocking_*.
fn pick_file(
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
    title: &'static str,
    filter: &'static str,
    extension: &'static str,
) -> Option<PathBuf> {
    app.dialog()
        .file()
        .set_parent(&window)
        .set_title(title)
        .add_filter(filter, &[extension])
        .blocking_pick_file()
        .and_then(|path| path.into_path().ok())
}

fn save_request(
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
    suggested: String,
    content: Vec<u8>,
) -> Result<Value, String> {
    let chosen = app
        .dialog()
        .file()
        .set_parent(&window)
        .set_title("Guardar la solicitud de licencia")
        .set_file_name(suggested)
        .add_filter("Solicitud de licencia", &[REQUEST_EXTENSION])
        .set_can_create_directories(true)
        .blocking_save_file()
        .and_then(|path| path.into_path().ok());
    let Some(chosen) = chosen else {
        return Ok(json!({"result": "cancelled"}));
    };
    let destination = request_destination(chosen)?;
    export::write_atomically(&destination, &content)?;
    Ok(json!({"result": "saved", "file_name": file_name(&destination)}))
}

fn cancelled() -> HTTPResult {
    HTTPResult {
        status: 200,
        body: json!({"result": "cancelled"}),
    }
}

// license_save_request es «Solicitud sin internet»: Go arma y firma la solicitud y Rust la guarda donde elija el
// administrador. Un rechazo de Go (por ejemplo, renovar sin una licencia activada) llega a React como en api_request.
#[tauri::command]
pub async fn license_save_request(
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
    state: State<'_, TerminalState>,
    action: String,
    license_key: Option<String>,
) -> Result<HTTPResult, String> {
    let body = request_body(&action, license_key.as_deref())?;
    let _guard = ChoosingGuard::acquire()?;
    let response =
        legacy_migration::call(&state, Method::POST, "license/offline-request", Some(&body))
            .await?;
    if response.status != 200 {
        return Ok(response);
    }
    let content = response
        .body
        .get("content")
        .and_then(Value::as_str)
        .ok_or(INVALID_RESPONSE)?
        .as_bytes()
        .to_vec();
    let suggested = suggested_request_name(
        response
            .body
            .get("filename")
            .and_then(Value::as_str)
            .unwrap_or(""),
    );
    let saved =
        tauri::async_runtime::spawn_blocking(move || save_request(app, window, suggested, content))
            .await
            .map_err(|_| "No se pudo abrir el diálogo para guardar la solicitud.")??;
    Ok(HTTPResult {
        status: 200,
        body: saved,
    })
}

// license_import_file es «Importar archivo de licencia».
#[tauri::command]
pub async fn license_import_file(
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
    state: State<'_, TerminalState>,
) -> Result<HTTPResult, String> {
    let _guard = ChoosingGuard::acquire()?;
    let chosen = tauri::async_runtime::spawn_blocking(move || {
        pick_file(
            app,
            window,
            "Elegir el archivo de licencia",
            "Archivo de licencia",
            LICENSE_EXTENSION,
        )
    })
    .await
    .map_err(|_| "No se pudo abrir el diálogo para elegir el archivo de licencia.")?;
    let Some(path) = chosen else {
        return Ok(cancelled());
    };
    let license = read_text_file(&path, &LICENSE_FILE)?;
    let response = legacy_migration::call(
        &state,
        Method::POST,
        "license/import",
        Some(&json!({ "license": license })),
    )
    .await?;
    if response.status != 200 {
        return Ok(response);
    }
    Ok(HTTPResult {
        status: 200,
        body: json!({"result": "imported", "license": response.body}),
    })
}

// license_import_unlock_file es «Importar la respuesta» del desbloqueo de soporte (solo en la computadora principal:
// Go exige la escucha local).
#[tauri::command]
pub async fn license_import_unlock_file(
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
    state: State<'_, TerminalState>,
) -> Result<HTTPResult, String> {
    let _guard = ChoosingGuard::acquire()?;
    let chosen = tauri::async_runtime::spawn_blocking(move || {
        pick_file(
            app,
            window,
            "Elegir la respuesta de soporte",
            "Respuesta de soporte",
            UNLOCK_EXTENSION,
        )
    })
    .await
    .map_err(|_| "No se pudo abrir el diálogo para elegir la respuesta de soporte.")?;
    let Some(path) = chosen else {
        return Ok(cancelled());
    };
    let response_text = read_text_file(&path, &UNLOCK_FILE)?;
    let response = legacy_migration::call(
        &state,
        Method::POST,
        "license/support-unlock",
        Some(&json!({ "response": response_text, "source": "file" })),
    )
    .await?;
    if response.status != 200 {
        return Ok(response);
    }
    Ok(HTTPResult {
        status: 200,
        body: json!({"result": "unlocked", "unlock": response.body}),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temporary_folder(name: &str) -> PathBuf {
        let nanos = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let folder = std::env::temp_dir().join(format!("adel-license-{name}-{nanos}"));
        std::fs::create_dir_all(&folder).unwrap();
        folder
    }

    #[test]
    fn the_request_body_is_a_closed_list() {
        assert_eq!(
            request_body("renew", None).unwrap(),
            json!({"action": "renew"})
        );
        assert_eq!(
            request_body("activate", Some("  ")).unwrap(),
            json!({"action": "activate"})
        );
        assert_eq!(
            request_body("activate", Some(" abc-2222-3333-4444 ")).unwrap(),
            json!({"action": "activate", "license_key": "abc-2222-3333-4444"})
        );
        assert!(request_body("transfer", None).is_err());
        assert!(request_body("../import", None).is_err());
        // El código solo va en la solicitud de activación.
        assert!(request_body("deactivate", Some("ABC-2222-3333-4444")).is_err());
        assert!(request_body("activate", Some(&"A".repeat(65))).is_err());
    }

    #[test]
    fn the_suggested_name_is_clean_and_keeps_the_extension() {
        assert_eq!(
            suggested_request_name("solicitud-activar-20261123-101500.licreq"),
            "solicitud-activar-20261123-101500.licreq"
        );
        assert_eq!(
            suggested_request_name("../../etc/solicitud renovar.licreq"),
            "solicitud_renovar.licreq"
        );
        assert_eq!(suggested_request_name(""), "solicitud-licencia.licreq");
        assert_eq!(
            suggested_request_name(".licreq"),
            "solicitud-licencia.licreq"
        );
        let long = suggested_request_name(&"x".repeat(300));
        assert!(long.len() <= MAX_FILE_NAME_CHARS);
        assert!(long.ends_with(".licreq"));
    }

    #[test]
    fn the_destination_gets_the_extension_and_never_replaces_silently() {
        let folder = temporary_folder("destination");
        let chosen = folder.join("solicitud");
        assert_eq!(
            request_destination(chosen.clone()).unwrap(),
            folder.join("solicitud.licreq")
        );
        // Con la extensión elegida en el panel, el panel ya confirmó reemplazar.
        assert_eq!(
            request_destination(folder.join("otra.LICREQ")).unwrap(),
            folder.join("otra.LICREQ")
        );
        std::fs::write(folder.join("solicitud.licreq"), b"anterior").unwrap();
        assert!(request_destination(chosen).is_err());
        assert!(request_destination(PathBuf::from("relativa.licreq")).is_err());
        std::fs::remove_dir_all(folder).unwrap();
    }

    #[test]
    fn imported_files_are_checked_before_reaching_go() {
        let folder = temporary_folder("import");
        let license = folder.join("licencia.lic");
        std::fs::write(&license, b"  eyJhbGciOiJFZERTQSJ9.e30.firma\n").unwrap();
        assert_eq!(
            read_text_file(&license, &LICENSE_FILE).unwrap(),
            "  eyJhbGciOiJFZERTQSJ9.e30.firma\n"
        );
        // Otra extensión, vacío, demasiado grande o sin texto: no se envía nada.
        let other = folder.join("licencia.txt");
        std::fs::write(&other, b"texto").unwrap();
        assert!(read_text_file(&other, &LICENSE_FILE).is_err());
        let empty = folder.join("vacia.lic");
        std::fs::write(&empty, b"").unwrap();
        assert!(read_text_file(&empty, &LICENSE_FILE).is_err());
        let blank = folder.join("blanca.lic");
        std::fs::write(&blank, b" \n\t").unwrap();
        assert!(read_text_file(&blank, &LICENSE_FILE).is_err());
        let large = folder.join("grande.lic");
        std::fs::write(&large, vec![b'a'; (MAX_LICENSE_FILE_BYTES + 1) as usize]).unwrap();
        assert!(read_text_file(&large, &LICENSE_FILE).is_err());
        let binary = folder.join("binaria.lic");
        std::fs::write(&binary, [0xff, 0xfe, 0x00, 0x41]).unwrap();
        assert!(read_text_file(&binary, &LICENSE_FILE).is_err());
        // La respuesta de soporte tiene su propia extensión.
        let unlock = folder.join("respuesta.desbloqueo");
        std::fs::write(&unlock, b"eyJ.e30.firma").unwrap();
        assert!(read_text_file(&unlock, &UNLOCK_FILE).is_ok());
        assert!(read_text_file(&unlock, &LICENSE_FILE).is_err());
        assert!(read_text_file(&license, &UNLOCK_FILE).is_err());
        std::fs::remove_dir_all(folder).unwrap();
    }

    #[test]
    fn only_one_dialog_at_a_time() {
        let first = ChoosingGuard::acquire().unwrap();
        assert!(ChoosingGuard::acquire().is_err());
        drop(first);
        assert!(ChoosingGuard::acquire().is_ok());
    }

    #[test]
    fn the_webview_receives_the_commands_but_no_dialog_permissions() {
        let capability = include_str!("../capabilities/main.json");
        let permissions = include_str!("../permissions/diagnostics.toml");
        for forbidden in ["dialog:", "fs:", "allow-open", "allow-save"] {
            assert!(!capability.contains(forbidden), "{forbidden}");
            assert!(!permissions.contains(forbidden), "{forbidden}");
        }
        for command in [
            "license_save_request",
            "license_import_file",
            "license_import_unlock_file",
        ] {
            assert!(permissions.contains(&format!("\"{command}\"")), "{command}");
        }
    }
}
