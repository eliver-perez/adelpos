// Exportación de reportes a PDF y Excel (etapa 9A): operación cerrada del puente nativo.
//
// React pide un reporte de la lista cerrada con un formato (pdf o xlsx) y los mismos parámetros que
// la pantalla (lista blanca). Rust pide el archivo a Go con las credenciales que custodia (igual que
// api_request), con 120 s de espera y un tope de 32 MiB; valida el tipo de contenido exacto y la
// firma del archivo (%PDF- o PK\x03\x04) antes de abrir el diálogo nativo «Guardar como»
// (tauri-plugin-dialog, usado solo desde Rust: la ventana no tiene permisos dialog:* ni fs:*), y lo
// escribe de forma atómica (temporal en la misma carpeta y renombrado). React nunca recibe los
// bytes, la ruta completa ni los tokens: solo {result: saved|cancelled, file_name, size}. Un
// rechazo de Go llega como el de api_request ({status, body}) para mostrar su mensaje y aplicar la
// invalidación de sesión.

use crate::{api, HTTPResult, TerminalState};
use serde_json::{json, Value};
use std::collections::BTreeMap;
use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use tauri::State;
use tauri_plugin_dialog::DialogExt;

// Tiempo máximo de una exportación (Go responde en hasta 110 s) y tamaño máximo del archivo.
const EXPORT_TIMEOUT_SECONDS: u64 = 120;
const EXPORT_MAX_BYTES: usize = 32 * 1024 * 1024;
// Un rechazo de Go es JSON pequeño.
const ERROR_MAX_BYTES: usize = 2 * 1024 * 1024;
const MAX_FILE_NAME_CHARS: usize = 100;

// Lista cerrada de reportes exportables: exactamente el registro de Go. La prueba
// TestExportBridgeListsEveryReport (backend/internal/reports/export_test.go) falla si un reporte
// registrado en Go falta aquí o si aquí sobra uno.
const EXPORTABLE_REPORTS: &[&str] = &[
    "sales.summary",
    "sales.by_day",
    "sales.by_week",
    "sales.by_month",
    "sales.by_hour",
    "sales.by_product",
    "sales.by_variant",
    "sales.by_mode",
    "sales.dine_in",
    "sales.takeaway",
    "sales.by_price_list",
    "sales.by_shift",
    "sales.accounts",
    "sales.voided_accounts",
    "sales.cancellations_by_reason",
    "sales.evolution",
    "sales.in_progress",
    "products.top",
    "products.bottom",
    "products.combos",
    "products.combo_components",
    "products.complements",
    "products.cancelled",
    "products.quantity_changes",
    "products.discounted",
    "products.trends",
    "sales.by_category",
    "payments.by_method",
    "payments.by_register",
    "payments.by_shift",
    "payments.by_user",
    "payments.by_day",
    "payments.list",
    "payments.mixed_partial",
    "payments.refunds",
    "payments.corrections",
    "payments.tips",
    "payments.currency",
    "payments.pending",
    "discounts.summary",
    "discounts.by_reason",
    "discounts.by_user",
    "discounts.by_product",
    "discounts.list",
    "discounts.general",
    "discounts.courtesies",
    "discounts.authorizations",
    "cash.shifts",
    "cash.sales_collections_by_shift",
    "cash.by_register",
    "cash.by_day",
    "cash.operations",
    "cash.cuts",
    "cash.by_user",
    "kitchen.demand_by_product",
    "kitchen.demand_by_hour",
    "kitchen.stations",
    "inventory.stock",
    "inventory.stock_by_warehouse",
    "inventory.negative",
    "inventory.below_minimum",
    "inventory.valuation",
    "inventory.average_costs",
    "inventory.kardex",
    "inventory.entries",
    "inventory.outflows",
    "inventory.transfers",
    "inventory.adjustments",
    "inventory.waste",
    "inventory.counts",
    "inventory.count_differences",
    "inventory.consumption_by_item",
    "inventory.consumption_by_day",
    "inventory.consumption_by_product",
    "inventory.consumption_by_kitchen",
    "purchases.received",
    "purchases.ordered",
    "purchases.pending",
    "purchases.by_supplier",
    "purchases.by_item",
    "purchases.supplier_returns",
    "purchases.cash_links",
    "suppliers.activity",
    "costs.margin_by_product",
    "costs.margin_by_category",
    "costs.margin_by_period",
    "costs.cost_status",
    "costs.theoretical_products",
    "costs.theoretical_recipes",
    "costs.consumption_valued",
    "costs.waste",
    "costs.cost_adjustments",
    "employees.activity",
    "employees.responsible_sales",
    "employees.confirmations",
];

// Una sola exportación a la vez en esta terminal, para no apilar diálogos ni descargas.
static EXPORTING: AtomicBool = AtomicBool::new(false);
static TEMPORARY_SEQUENCE: AtomicU64 = AtomicU64::new(0);

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum ExportFormat {
    Pdf,
    Xlsx,
}

impl ExportFormat {
    fn parse(value: &str) -> Result<Self, String> {
        match value {
            "pdf" => Ok(Self::Pdf),
            "xlsx" => Ok(Self::Xlsx),
            _ => Err("El formato de exportación no es válido.".into()),
        }
    }

    fn code(self) -> &'static str {
        match self {
            Self::Pdf => "pdf",
            Self::Xlsx => "xlsx",
        }
    }

    fn content_type(self) -> &'static str {
        match self {
            Self::Pdf => "application/pdf",
            Self::Xlsx => "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        }
    }

    fn signature(self) -> &'static [u8] {
        match self {
            Self::Pdf => b"%PDF-",
            Self::Xlsx => b"PK\x03\x04",
        }
    }

    fn filter_name(self) -> &'static str {
        match self {
            Self::Pdf => "Documento PDF",
            Self::Xlsx => "Libro de Excel",
        }
    }
}

// export_route valida el reporte contra la lista cerrada y el formato, y devuelve la ruta de Go.
// Nunca se compone una ruta con texto de React fuera de la lista.
pub(crate) fn export_route(report: &str, format: &str) -> Result<(ExportFormat, String), String> {
    let format = ExportFormat::parse(format)?;
    if !EXPORTABLE_REPORTS.contains(&report) {
        return Err("El reporte no se puede exportar desde el puente nativo.".into());
    }
    Ok((format, format!("reports/export/{report}")))
}

// validate_query admite los parámetros de reports_run (lista blanca de api.rs) sin la paginación ni
// la búsqueda de opciones, más paper (letter o a4) solo para el PDF. El formato lo agrega Rust.
pub(crate) fn validate_query(
    query: &BTreeMap<String, String>,
    format: ExportFormat,
) -> Result<(), String> {
    let allowed = api::allowed_query_keys("reports_run");
    for (key, value) in query {
        let accepted = match key.as_str() {
            "page" | "page_size" | "search" | "format" => false,
            "paper" => format == ExportFormat::Pdf && (value == "letter" || value == "a4"),
            other => allowed.contains(&other),
        };
        if !accepted || value.len() > 120 || value.chars().any(char::is_control) {
            return Err("Parámetros de exportación no autorizados.".into());
        }
    }
    Ok(())
}

// validate_file comprueba, antes de abrir el diálogo, que Go respondió exactamente el tipo pedido
// con la firma de ese formato y dentro del tope.
pub(crate) fn validate_file(
    format: ExportFormat,
    content_type: Option<&str>,
    bytes: &[u8],
) -> Result<(), String> {
    let declared = content_type
        .map(|value| value.trim().to_ascii_lowercase())
        .unwrap_or_default();
    if declared != format.content_type() {
        return Err("El servidor no respondió con el tipo de archivo esperado.".into());
    }
    if bytes.len() > EXPORT_MAX_BYTES {
        return Err("El archivo excede el tamaño admitido.".into());
    }
    if !bytes.starts_with(format.signature()) {
        return Err("El archivo recibido no es válido.".into());
    }
    Ok(())
}

// sanitize_file_name deja solo el nombre base con letras y dígitos ASCII, punto, guion, guion bajo
// y espacio (lo demás se vuelve «_»), sin punto inicial, con la extensión del formato forzada y a lo
// más 100 caracteres. Sin un nombre útil queda «reporte.<ext>».
pub(crate) fn sanitize_file_name(name: &str, format: ExportFormat) -> String {
    let base = name
        .rsplit(|character| character == '/' || character == '\\')
        .next()
        .unwrap_or("");
    let mut cleaned: String = base
        .chars()
        .map(|character| {
            if character.is_ascii_alphanumeric() || matches!(character, '.' | '-' | '_' | ' ') {
                character
            } else {
                '_'
            }
        })
        .collect();
    let extension = format!(".{}", format.code());
    if cleaned.to_ascii_lowercase().ends_with(&extension) {
        cleaned.truncate(cleaned.len() - extension.len());
    }
    let mut stem = cleaned
        .trim_matches(|character: char| character == '.' || character == ' ')
        .to_string();
    if stem
        .chars()
        .all(|character| character == '_' || character == '-')
    {
        stem = "reporte".into();
    }
    stem.truncate(MAX_FILE_NAME_CHARS - extension.len());
    let stem = stem.trim_end_matches(['.', ' ']);
    format!("{stem}{extension}")
}

// suggested_name toma el nombre que sugiere Go en Content-Disposition (filename="…" o sin
// comillas) y lo sanea.
pub(crate) fn suggested_name(disposition: Option<&str>, format: ExportFormat) -> String {
    let name = disposition
        .and_then(|value| {
            value.split(';').find_map(|part| {
                let (key, value) = part.split_once('=')?;
                if key.trim().eq_ignore_ascii_case("filename") {
                    Some(value.trim().trim_matches('"').to_string())
                } else {
                    None
                }
            })
        })
        .unwrap_or_default();
    sanitize_file_name(&name, format)
}

// with_extension fuerza la extensión del formato en la ruta elegida; indica si tuvo que cambiarla.
fn with_extension(path: PathBuf, format: ExportFormat) -> (PathBuf, bool) {
    let matches = path
        .extension()
        .and_then(|value| value.to_str())
        .map(|value| value.eq_ignore_ascii_case(format.code()))
        .unwrap_or(false);
    if matches {
        return (path, false);
    }
    let mut text = path.into_os_string();
    text.push(".");
    text.push(format.code());
    (PathBuf::from(text), true)
}

// temporary_name forma el nombre del temporal: corto (menos de 80 bytes, muy por debajo del
// límite de 255 bytes por nombre de macOS) e independiente del nombre elegido, para que un nombre
// largo nunca impida crear el temporal. Oculto (punto inicial) y único por proceso, instante y
// secuencia.
pub(crate) fn temporary_name(process: u32, nanos: u128, sequence: u64) -> String {
    format!(".adel-export-{process:x}-{nanos:x}-{sequence:x}.part")
}

// write_atomically escribe en un temporal de la misma carpeta, lo sincroniza y lo renombra al
// destino (el panel de macOS ya confirmó la sustitución si existía). Si algo falla, borra el
// temporal y el destino queda como estaba.
pub(crate) fn write_atomically(destination: &Path, bytes: &[u8]) -> Result<u64, String> {
    let folder = destination
        .parent()
        .filter(|folder| folder.is_dir())
        .ok_or("La carpeta elegida no existe.")?;
    destination
        .file_name()
        .and_then(|value| value.to_str())
        .ok_or("El nombre del archivo no es válido.")?;
    let temporary = folder.join(temporary_name(
        std::process::id(),
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|value| value.as_nanos())
            .unwrap_or(0),
        TEMPORARY_SEQUENCE.fetch_add(1, Ordering::Relaxed),
    ));
    let written = (|| -> std::io::Result<()> {
        let mut file = OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&temporary)?;
        file.write_all(bytes)?;
        file.sync_all()?;
        drop(file);
        fs::rename(&temporary, destination)
    })();
    if written.is_err() {
        let _ = fs::remove_file(&temporary);
        return Err("No se pudo guardar el archivo en la carpeta elegida.".into());
    }
    Ok(bytes.len() as u64)
}

// SaveTarget separa el diálogo nativo para poder probar la escritura sin ventana. directory es
// la carpeta en la que se vuelve a abrir el diálogo (ninguna la primera vez).
pub(crate) trait SaveTarget {
    fn choose(
        &self,
        suggested: &str,
        directory: Option<&Path>,
        format: ExportFormat,
    ) -> Result<Option<PathBuf>, String>;
}

// Decisión sobre la ruta que devolvió el diálogo.
#[derive(Debug, PartialEq)]
pub(crate) enum Destination {
    // Escribir en esta ruta (ya con la extensión del formato).
    Write(PathBuf),
    // Volver a abrir el diálogo en la carpeta con el nombre completo sugerido.
    AskAgain { name: String, folder: PathBuf },
}

// MAX_DIALOG_ATTEMPTS acota cuántas veces se abre el diálogo en una misma exportación (la primera y
// las que se vuelven a abrir porque el nombre con extensión ya existía).
const MAX_DIALOG_ATTEMPTS: usize = 4;

// resolve_destination fuerza la extensión del formato en la ruta elegida. El panel solo confirmó
// sustituir el nombre que mostró: si con la extensión agregada por Rust el archivo ya existe, no
// se reemplaza sin preguntar; se vuelve a abrir el diálogo con el nombre completo, de modo que el
// propio panel pida confirmar la sustitución o se elija otro nombre.
pub(crate) fn resolve_destination(
    chosen: PathBuf,
    format: ExportFormat,
) -> Result<Destination, String> {
    if !chosen.is_absolute() {
        return Err("La ruta elegida no es válida.".into());
    }
    let (destination, changed) = with_extension(chosen, format);
    if changed && destination.exists() {
        let name = destination
            .file_name()
            .and_then(|value| value.to_str())
            .ok_or("El nombre del archivo no es válido.")?
            .to_string();
        let folder = destination
            .parent()
            .ok_or("La ruta elegida no es válida.")?
            .to_path_buf();
        return Ok(Destination::AskAgain { name, folder });
    }
    Ok(Destination::Write(destination))
}

// save_with pide el destino y escribe el archivo; devuelve solo lo que React puede ver.
pub(crate) fn save_with(
    target: &dyn SaveTarget,
    suggested: &str,
    format: ExportFormat,
    bytes: &[u8],
) -> Result<Value, String> {
    let mut name = suggested.to_string();
    let mut folder: Option<PathBuf> = None;
    let mut attempt = 0;
    let destination = loop {
        attempt += 1;
        let Some(chosen) = target.choose(&name, folder.as_deref(), format)? else {
            return Ok(json!({"result": "cancelled"}));
        };
        match resolve_destination(chosen, format)? {
            Destination::Write(destination) => break destination,
            Destination::AskAgain {
                name: complete,
                folder: chosen_folder,
            } => {
                if attempt >= MAX_DIALOG_ATTEMPTS {
                    return Err(
                        "Ya existe un archivo con ese nombre. Vuelve a exportar y elige otro nombre."
                            .into(),
                    );
                }
                name = complete;
                folder = Some(chosen_folder);
            }
        }
    };
    let size = write_atomically(&destination, bytes)?;
    let file_name = destination
        .file_name()
        .and_then(|value| value.to_str())
        .unwrap_or("")
        .to_string();
    Ok(json!({"result": "saved", "file_name": file_name, "size": size}))
}

// DialogTarget abre «Guardar como» con tauri-plugin-dialog sobre la ventana principal. Se llama
// fuera del hilo principal (spawn_blocking), como exige blocking_save_file.
struct DialogTarget {
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
}

impl SaveTarget for DialogTarget {
    fn choose(
        &self,
        suggested: &str,
        directory: Option<&Path>,
        format: ExportFormat,
    ) -> Result<Option<PathBuf>, String> {
        let mut dialog = self
            .app
            .dialog()
            .file()
            .set_parent(&self.window)
            .set_title("Guardar reporte")
            .set_file_name(suggested)
            .add_filter(format.filter_name(), &[format.code()])
            .set_can_create_directories(true);
        if let Some(directory) = directory {
            dialog = dialog.set_directory(directory);
        }
        let chosen = dialog.blocking_save_file();
        match chosen {
            None => Ok(None),
            Some(path) => path
                .into_path()
                .map(Some)
                .map_err(|_| "La ruta elegida no es válida.".to_string()),
        }
    }
}

// ExportGuard libera el indicador de exportación en curso al terminar, también con error.
struct ExportGuard;

impl ExportGuard {
    fn acquire() -> Result<Self, String> {
        if EXPORTING
            .compare_exchange(false, true, Ordering::AcqRel, Ordering::Acquire)
            .is_err()
        {
            return Err("Ya hay una exportación en curso.".into());
        }
        Ok(Self)
    }
}

impl Drop for ExportGuard {
    fn drop(&mut self) {
        EXPORTING.store(false, Ordering::Release);
    }
}

#[tauri::command]
pub async fn export_report(
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
    state: State<'_, TerminalState>,
    report: String,
    format: String,
    query: Option<BTreeMap<String, String>>,
) -> Result<HTTPResult, String> {
    let (format, path) = export_route(&report, &format)?;
    let query = query.unwrap_or_default();
    validate_query(&query, format)?;
    let _guard = ExportGuard::acquire()?;
    let config = state.config.as_ref().map_err(Clone::clone)?;
    let vault = state.vault()?;
    let client = state.client.as_ref().map_err(Clone::clone)?;
    let mut credentials = vault.read()?;
    let mut request = client
        .get(format!("{}/api/v1/{path}", config.server_url))
        .timeout(Duration::from_secs(EXPORT_TIMEOUT_SECONDS))
        .query(&query)
        .query(&[("format", format.code())]);
    if let (Some(id), Some(token)) = (&credentials.device_id, &credentials.device_token) {
        request = request
            .header("X-Device-ID", id)
            .header("X-Device-Token", token);
    }
    if let Some(token) = &credentials.session_token {
        request = request.bearer_auth(token);
    }
    let mut response = request.send().await.map_err(|_| {
        "No se pudo conectar con el servidor. Comprueba su estado y la confianza TLS."
    })?;
    let status = response.status().as_u16();
    let header = |name: reqwest::header::HeaderName| {
        response
            .headers()
            .get(name)
            .and_then(|value| value.to_str().ok())
            .map(str::to_string)
    };
    let content_type = header(reqwest::header::CONTENT_TYPE);
    let disposition = header(reqwest::header::CONTENT_DISPOSITION);
    let limit = if status == 200 {
        EXPORT_MAX_BYTES
    } else {
        ERROR_MAX_BYTES
    };
    let mut bytes = Vec::new();
    while let Some(chunk) = response
        .chunk()
        .await
        .map_err(|_| "Se interrumpió la respuesta del servidor.")?
    {
        if bytes.len() + chunk.len() > limit {
            return Err("El archivo excede el tamaño admitido.".into());
        }
        bytes.extend_from_slice(&chunk);
    }
    if status != 200 {
        // Rechazo de Go (permisos, filtros, demasiados renglones, sesión): mismo contrato y misma
        // invalidación de credenciales que api_request.
        let mut body: Value = serde_json::from_slice(&bytes)
            .map_err(|_| "El servidor no respondió con un contrato válido.")?;
        if api::invalidates_credentials(status, &body) {
            credentials.session_token = None;
            let code = body
                .pointer("/error/code")
                .and_then(Value::as_str)
                .unwrap_or("");
            if code == "DEVICE_REVOKED" || code == "DEVICE_REQUIRED" {
                credentials.device_id = None;
                credentials.device_token = None;
            }
            vault.save(credentials)?;
        }
        api::sanitize_response(&mut body);
        return Ok(HTTPResult { status, body });
    }
    validate_file(format, content_type.as_deref(), &bytes)?;
    let suggested = suggested_name(disposition.as_deref(), format);
    let body = tauri::async_runtime::spawn_blocking(move || {
        let target = DialogTarget { app, window };
        save_with(&target, &suggested, format, &bytes)
    })
    .await
    .map_err(|_| "No se pudo abrir el diálogo para guardar el archivo.")??;
    Ok(HTTPResult { status: 200, body })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::cell::RefCell;

    fn query(pairs: &[(&str, &str)]) -> BTreeMap<String, String> {
        pairs
            .iter()
            .map(|(key, value)| (key.to_string(), value.to_string()))
            .collect()
    }

    #[test]
    fn exportable_reports_are_a_closed_list() {
        assert_eq!(
            export_route("sales.by_day", "pdf").unwrap(),
            (ExportFormat::Pdf, "reports/export/sales.by_day".to_string())
        );
        assert_eq!(
            export_route("payments.list", "xlsx").unwrap().1,
            "reports/export/payments.list"
        );
        for rejected in [
            "",
            "sales",
            "sales.unknown",
            "../sales.by_day",
            "sales.by_day/../../auth/me",
            "SALES.BY_DAY",
            "sales.by_day?format=pdf",
            "reports_run",
            "catalog_products",
        ] {
            assert!(export_route(rejected, "pdf").is_err(), "{rejected}");
        }
        for format in ["", "PDF", "csv", "xlsm", "xls", "pdf "] {
            assert!(export_route("sales.by_day", format).is_err(), "{format}");
        }
        let mut sorted = EXPORTABLE_REPORTS.to_vec();
        sorted.sort();
        sorted.dedup();
        assert_eq!(sorted.len(), EXPORTABLE_REPORTS.len());
        for report in EXPORTABLE_REPORTS {
            assert!(api::report_key(Some(report)).is_ok(), "{report}");
        }
    }

    #[test]
    fn export_parameters_are_whitelisted() {
        let accepted = query(&[
            ("period", "month"),
            ("month", "2026-11"),
            ("compare", "previous"),
            ("sort", "net"),
            ("direction", "desc"),
            ("register_id", "0f8e3c2a-0000-4000-8000-000000000001"),
            ("paper", "a4"),
        ]);
        assert!(validate_query(&accepted, ExportFormat::Pdf).is_ok());
        assert!(validate_query(&query(&[("period", "today")]), ExportFormat::Xlsx).is_ok());
        for rejected in [
            query(&[("page", "2")]),
            query(&[("page_size", "200")]),
            query(&[("search", "x")]),
            query(&[("format", "xlsx")]),
            query(&[("paper", "legal")]),
            query(&[("token", "x")]),
            query(&[("period", "today\n")]),
            query(&[("period", &"x".repeat(121))]),
        ] {
            assert!(
                validate_query(&rejected, ExportFormat::Pdf).is_err(),
                "{rejected:?}"
            );
        }
        assert!(validate_query(&query(&[("paper", "letter")]), ExportFormat::Xlsx).is_err());
    }

    #[test]
    fn files_are_validated_by_type_and_signature_before_the_dialog() {
        let pdf = b"%PDF-1.3\n...";
        let xlsx = b"PK\x03\x04rest";
        let pdf_type = Some("application/pdf");
        let xlsx_type = Some("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet");
        assert!(validate_file(ExportFormat::Pdf, pdf_type, pdf).is_ok());
        assert!(validate_file(ExportFormat::Xlsx, xlsx_type, xlsx).is_ok());
        assert!(validate_file(ExportFormat::Pdf, xlsx_type, pdf).is_err());
        assert!(validate_file(ExportFormat::Pdf, Some("application/json"), pdf).is_err());
        assert!(validate_file(ExportFormat::Pdf, None, pdf).is_err());
        assert!(validate_file(ExportFormat::Pdf, Some("application/pdf; x=1"), pdf).is_err());
        assert!(validate_file(ExportFormat::Pdf, pdf_type, b"<html>").is_err());
        assert!(validate_file(ExportFormat::Pdf, pdf_type, b"").is_err());
        assert!(validate_file(ExportFormat::Xlsx, xlsx_type, pdf).is_err());
        let oversized = vec![b'%'; EXPORT_MAX_BYTES + 1];
        assert!(validate_file(ExportFormat::Pdf, pdf_type, &oversized).is_err());
    }

    #[test]
    fn file_names_are_sanitized_with_forced_extension() {
        let pdf = ExportFormat::Pdf;
        let xlsx = ExportFormat::Xlsx;
        assert_eq!(
            suggested_name(
                Some("attachment; filename=ventas-por-dia_2026-11-01_2026-11-30.pdf"),
                pdf
            ),
            "ventas-por-dia_2026-11-01_2026-11-30.pdf"
        );
        assert_eq!(
            suggested_name(Some("attachment; filename=\"cobros.xlsx\""), xlsx),
            "cobros.xlsx"
        );
        assert_eq!(suggested_name(None, pdf), "reporte.pdf");
        assert_eq!(sanitize_file_name("../../etc/passwd", pdf), "passwd.pdf");
        assert_eq!(sanitize_file_name("C:\\temp\\a.xlsx", xlsx), "a.xlsx");
        assert_eq!(sanitize_file_name(".oculto", pdf), "oculto.pdf");
        assert_eq!(sanitize_file_name("nul\u{0}byte", pdf), "nul_byte.pdf");
        assert_eq!(sanitize_file_name("año ñ.pdf", pdf), "a_o _.pdf");
        assert_eq!(sanitize_file_name("reporte.xlsx", pdf), "reporte.xlsx.pdf");
        assert_eq!(sanitize_file_name("REPORTE.PDF", pdf), "REPORTE.pdf");
        assert_eq!(sanitize_file_name("...", xlsx), "reporte.xlsx");
        assert_eq!(sanitize_file_name("///", xlsx), "reporte.xlsx");
        let long = sanitize_file_name(&"a".repeat(300), pdf);
        assert_eq!(long.len(), MAX_FILE_NAME_CHARS);
        assert!(long.ends_with(".pdf"));
    }

    // FakeTarget devuelve, en orden, las rutas que «elegiría» la persona en cada apertura del
    // diálogo (None es cancelar) y registra el nombre sugerido y la carpeta de cada apertura.
    struct FakeTarget {
        choices: RefCell<Vec<Option<PathBuf>>>,
        asked: RefCell<Vec<(String, Option<PathBuf>)>>,
    }

    impl FakeTarget {
        fn new(choices: Vec<Option<PathBuf>>) -> Self {
            Self {
                choices: RefCell::new(choices),
                asked: RefCell::new(vec![]),
            }
        }

        fn suggested(&self) -> Vec<String> {
            self.asked
                .borrow()
                .iter()
                .map(|(name, _)| name.clone())
                .collect()
        }
    }

    impl SaveTarget for FakeTarget {
        fn choose(
            &self,
            suggested: &str,
            directory: Option<&Path>,
            _: ExportFormat,
        ) -> Result<Option<PathBuf>, String> {
            self.asked
                .borrow_mut()
                .push((suggested.to_string(), directory.map(Path::to_path_buf)));
            let mut choices = self.choices.borrow_mut();
            if choices.is_empty() {
                return Ok(None);
            }
            Ok(choices.remove(0))
        }
    }

    fn temporary_folder(name: &str) -> PathBuf {
        let folder = std::env::temp_dir().join(format!(
            "adel-export-test-{name}-{}-{}",
            std::process::id(),
            TEMPORARY_SEQUENCE.fetch_add(1, Ordering::Relaxed)
        ));
        fs::create_dir_all(&folder).unwrap();
        folder
    }

    fn leftovers(folder: &Path) -> Vec<String> {
        fs::read_dir(folder)
            .unwrap()
            .filter_map(|entry| entry.ok())
            .map(|entry| entry.file_name().to_string_lossy().to_string())
            .filter(|name| name.starts_with(".adel-export-"))
            .collect()
    }

    #[test]
    fn saving_is_atomic_and_returns_only_name_and_size() {
        let folder = temporary_folder("save");
        let target = FakeTarget::new(vec![Some(folder.join("ventas.pdf"))]);
        let bytes = b"%PDF-1.3 contenido";
        let result = save_with(&target, "ventas.pdf", ExportFormat::Pdf, bytes).unwrap();
        assert_eq!(
            result,
            json!({"result": "saved", "file_name": "ventas.pdf", "size": bytes.len()})
        );
        assert_eq!(
            target.asked.borrow().as_slice(),
            [("ventas.pdf".to_string(), None)]
        );
        assert_eq!(fs::read(folder.join("ventas.pdf")).unwrap(), bytes);
        assert!(!result.to_string().contains(folder.to_str().unwrap()));
        // Sustituir un archivo confirmado por el panel reemplaza su contenido completo.
        let target = FakeTarget::new(vec![Some(folder.join("ventas.pdf"))]);
        save_with(&target, "ventas.pdf", ExportFormat::Pdf, b"%PDF-nuevo").unwrap();
        assert_eq!(fs::read(folder.join("ventas.pdf")).unwrap(), b"%PDF-nuevo");
        assert!(leftovers(&folder).is_empty());
        // La extensión se fuerza cuando falta y el archivo no existe.
        let target = FakeTarget::new(vec![Some(folder.join("libro"))]);
        let result = save_with(&target, "libro.xlsx", ExportFormat::Xlsx, b"PK\x03\x04").unwrap();
        assert_eq!(result["file_name"], "libro.xlsx");
        assert!(folder.join("libro.xlsx").exists());
        fs::remove_dir_all(&folder).unwrap();
    }

    // Revisión independiente de la 9A: el temporal es corto e independiente del nombre elegido.
    #[test]
    fn temporary_names_are_short_and_independent_of_the_destination() {
        let longest = temporary_name(u32::MAX, u128::MAX, u64::MAX);
        assert!(longest.len() < 80, "{longest}");
        assert!(longest.starts_with(".adel-export-") && longest.ends_with(".part"));
        assert_ne!(temporary_name(1, 2, 3), temporary_name(1, 2, 4));
        // Un nombre de 254 bytes (cerca del límite de 255 de macOS) se guarda sin dejar temporales.
        let folder = temporary_folder("long");
        let name = format!("{}.pdf", "a".repeat(250));
        let target = FakeTarget::new(vec![Some(folder.join(&name))]);
        let result = save_with(&target, "x.pdf", ExportFormat::Pdf, b"%PDF-largo").unwrap();
        assert_eq!(result["file_name"], name.as_str());
        assert_eq!(fs::read(folder.join(&name)).unwrap(), b"%PDF-largo");
        assert!(leftovers(&folder).is_empty());
        // También con letras de varios bytes: 120 «ñ» son 240 bytes más la extensión.
        let name = format!("{}.pdf", "ñ".repeat(120));
        let target = FakeTarget::new(vec![Some(folder.join(&name))]);
        assert!(save_with(&target, "x.pdf", ExportFormat::Pdf, b"%PDF-").is_ok());
        assert!(leftovers(&folder).is_empty());
        fs::remove_dir_all(&folder).unwrap();
    }

    // Revisión independiente de la 9A: si con la extensión agregada el archivo ya existe, el
    // diálogo se vuelve a abrir en la misma carpeta con el nombre completo, en lugar de fallar.
    #[test]
    fn an_existing_name_with_forced_extension_reopens_the_dialog() {
        let folder = temporary_folder("again");
        fs::write(folder.join("ventas.pdf"), b"%PDF-viejo").unwrap();
        assert_eq!(
            resolve_destination(folder.join("ventas"), ExportFormat::Pdf).unwrap(),
            Destination::AskAgain {
                name: "ventas.pdf".into(),
                folder: folder.clone()
            }
        );
        assert_eq!(
            resolve_destination(folder.join("otro"), ExportFormat::Pdf).unwrap(),
            Destination::Write(folder.join("otro.pdf"))
        );
        assert_eq!(
            resolve_destination(folder.join("ventas.PDF"), ExportFormat::Pdf).unwrap(),
            Destination::Write(folder.join("ventas.PDF"))
        );
        assert!(resolve_destination(PathBuf::from("ventas"), ExportFormat::Pdf).is_err());
        // Segunda apertura con «ventas.pdf» en la carpeta elegida: el panel confirmó sustituirlo.
        let target = FakeTarget::new(vec![
            Some(folder.join("ventas")),
            Some(folder.join("ventas.pdf")),
        ]);
        let result = save_with(&target, "reporte.pdf", ExportFormat::Pdf, b"%PDF-nuevo").unwrap();
        assert_eq!(result["file_name"], "ventas.pdf");
        assert_eq!(
            target.asked.borrow().as_slice(),
            [
                ("reporte.pdf".to_string(), None),
                ("ventas.pdf".to_string(), Some(folder.clone()))
            ]
        );
        assert_eq!(fs::read(folder.join("ventas.pdf")).unwrap(), b"%PDF-nuevo");
        // Cancelar la segunda apertura no escribe nada.
        let target = FakeTarget::new(vec![Some(folder.join("ventas")), None]);
        assert_eq!(
            save_with(&target, "reporte.pdf", ExportFormat::Pdf, b"%PDF-otro").unwrap(),
            json!({"result": "cancelled"})
        );
        assert_eq!(fs::read(folder.join("ventas.pdf")).unwrap(), b"%PDF-nuevo");
        // Elegir otro nombre en la segunda apertura guarda ese archivo.
        let target = FakeTarget::new(vec![
            Some(folder.join("ventas")),
            Some(folder.join("abril")),
        ]);
        let result = save_with(&target, "reporte.pdf", ExportFormat::Pdf, b"%PDF-abril").unwrap();
        assert_eq!(result["file_name"], "abril.pdf");
        assert_eq!(fs::read(folder.join("ventas.pdf")).unwrap(), b"%PDF-nuevo");
        // Insistir sin extensión tiene un tope de aperturas y nunca sustituye sin preguntar.
        let target = FakeTarget::new(vec![Some(folder.join("ventas")); MAX_DIALOG_ATTEMPTS + 2]);
        assert!(save_with(&target, "reporte.pdf", ExportFormat::Pdf, b"%PDF-no").is_err());
        assert_eq!(target.suggested().len(), MAX_DIALOG_ATTEMPTS);
        assert_eq!(fs::read(folder.join("ventas.pdf")).unwrap(), b"%PDF-nuevo");
        assert!(leftovers(&folder).is_empty());
        fs::remove_dir_all(&folder).unwrap();
    }

    #[test]
    fn cancelling_writes_nothing_and_failures_leave_no_temporary_file() {
        let folder = temporary_folder("cancel");
        let target = FakeTarget::new(vec![None]);
        assert_eq!(
            save_with(&target, "ventas.pdf", ExportFormat::Pdf, b"%PDF-").unwrap(),
            json!({"result": "cancelled"})
        );
        assert_eq!(fs::read_dir(&folder).unwrap().count(), 0);
        // Carpeta inexistente o ruta relativa: error claro, sin archivos.
        let missing = FakeTarget::new(vec![Some(folder.join("no-existe").join("ventas.pdf"))]);
        assert!(save_with(&missing, "ventas.pdf", ExportFormat::Pdf, b"%PDF-").is_err());
        let relative = FakeTarget::new(vec![Some(PathBuf::from("ventas.pdf"))]);
        assert!(save_with(&relative, "ventas.pdf", ExportFormat::Pdf, b"%PDF-").is_err());
        // El destino es una carpeta: el renombrado falla y el temporal se borra.
        fs::create_dir(folder.join("ocupado.pdf")).unwrap();
        let occupied = FakeTarget::new(vec![Some(folder.join("ocupado.pdf"))]);
        assert!(save_with(&occupied, "ocupado.pdf", ExportFormat::Pdf, b"%PDF-").is_err());
        assert!(leftovers(&folder).is_empty());
        fs::remove_dir_all(&folder).unwrap();
    }

    #[test]
    fn only_one_export_runs_at_a_time() {
        let first = ExportGuard::acquire().unwrap();
        assert!(ExportGuard::acquire().is_err());
        drop(first);
        assert!(ExportGuard::acquire().is_ok());
    }

    // La ventana no recibe permisos del diálogo ni del sistema de archivos: el plugin solo se usa
    // desde Rust y el WebView no puede abrir rutas.
    #[test]
    fn webview_has_no_dialog_or_filesystem_permissions() {
        let capability = include_str!("../capabilities/main.json");
        let permissions = include_str!("../permissions/diagnostics.toml");
        for forbidden in ["dialog:", "fs:", "dialog", "allow-save", "allow-open"] {
            assert!(!capability.contains(forbidden), "{forbidden}");
        }
        assert!(permissions.contains("\"export_report\""));
        assert!(!permissions.contains("dialog"));
    }
}
