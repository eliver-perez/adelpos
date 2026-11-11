use crate::{HTTPResult, TerminalState};
use reqwest::Method;
use serde::Serialize;
use serde_json::Value;
use std::collections::BTreeMap;
use std::time::Duration;
use tauri::State;

// Tiempo de respuesta de las consultas de reportes (Go responde en hasta 40 s).
const REPORT_TIMEOUT_SECONDS: u64 = 45;

pub(crate) fn resource_id(id: Option<&str>) -> Result<&str, String> {
    let value = id.ok_or("Falta el identificador del registro.")?;
    if value.len() != 36
        || value
            .chars()
            .any(|character| !character.is_ascii_hexdigit() && character != '-')
    {
        return Err("El identificador del registro no es válido.".into());
    }
    Ok(value)
}

// report_key valida la clave de un reporte o de una fuente de filtros: minúsculas, puntos y guiones
// bajos, sin barras ni caracteres que permitan salir de la ruta.
pub(crate) fn report_key(id: Option<&str>) -> Result<&str, String> {
    let value = id.ok_or("Falta el identificador del reporte.")?;
    let valid = !value.is_empty()
        && value.len() <= 64
        && value.starts_with(|character: char| character.is_ascii_lowercase())
        && value.chars().all(|character| {
            character.is_ascii_lowercase() || character == '.' || character == '_'
        })
        && !value.contains("..");
    if !valid {
        return Err("El identificador del reporte no es válido.".into());
    }
    Ok(value)
}

fn route(operation: &str, id: Option<&str>) -> Result<(Method, String), String> {
    let fixed = match operation {
        "kitchen_outputs" => Some((Method::GET, "kitchen/outputs")),
        "kitchen_stations" => Some((Method::GET, "kitchen/stations")),
        "kitchen_create_station" => Some((Method::POST, "kitchen/stations")),
        "kitchen_printers" => Some((Method::GET, "kitchen/printers")),
        "kitchen_create_printer" => Some((Method::POST, "kitchen/printers")),
        "kitchen_discovery" => Some((Method::GET, "kitchen/discovery")),
        "kitchen_routes" => Some((Method::GET, "kitchen/routes")),
        "kitchen_targets" => Some((Method::GET, "kitchen/targets")),
        "kitchen_create_route" => Some((Method::POST, "kitchen/routes")),
        "kitchen_settings" => Some((Method::GET, "kitchen/settings")),
        "kitchen_save_settings" => Some((Method::PUT, "kitchen/settings")),
        "kitchen_save_timing" => Some((Method::PUT, "kitchen/timing")),
        "kitchen_access" => Some((Method::GET, "kitchen/access")),
        "kitchen_save_access" => Some((Method::PUT, "kitchen/access")),
        "kitchen_board" => Some((Method::GET, "kitchen/board")),
        "kitchen_releases" => Some((Method::GET, "kitchen/releases")),
        "kitchen_tickets" => Some((Method::GET, "kitchen/tickets")),
        "kitchen_jobs" => Some((Method::GET, "kitchen/jobs")),
        "kitchen_history" => Some((Method::GET, "kitchen/history")),
        "kitchen_save_states" => Some((Method::POST, "kitchen/states")),
        "kitchen_test_print" => Some((Method::POST, "kitchen/test-print")),
        "kitchen_reprint" => Some((Method::POST, "kitchen/reprint")),
        // Etapa 9B: imprimir una comanda histórica con la plantilla vigente (acción distinta de la
        // reimpresión exacta, con su propio permiso).
        "kitchen_rerender" => Some((Method::POST, "kitchen/print-jobs/rerender")),

        "setup_status" => Some((Method::GET, "setup/status")),
        "setup" => Some((Method::POST, "setup")),
        "enroll" => Some((Method::POST, "devices/enroll")),
        "enroll_local" => Some((Method::POST, "devices/enroll")),
        "login" => Some((Method::POST, "auth/login")),
        "logout" => Some((Method::POST, "auth/logout")),
        "me" => Some((Method::GET, "auth/me")),
        "change_password" => Some((Method::POST, "auth/password")),
        "users" => Some((Method::GET, "users")),
        "create_user" => Some((Method::POST, "users")),
        "roles" => Some((Method::GET, "roles")),
        "create_role" => Some((Method::POST, "roles")),
        "permissions" => Some((Method::GET, "permissions")),
        "settings" => Some((Method::GET, "settings")),
        "save_settings" => Some((Method::PUT, "settings")),
        "settings_features" => Some((Method::GET, "settings/features")),
        "save_settings_feature" => Some((Method::PUT, "settings/features")),
        // Etapa 9A: «Hora de inicio del día» (una sola configuración, settings.manage).
        "settings_business_day" => Some((Method::GET, "settings/business-day")),
        "save_settings_business_day" => Some((Method::PUT, "settings/business-day")),
        // Etapa 9A: reportes (todas de solo lectura).
        "reports_catalog" => Some((Method::GET, "reports/catalog")),
        "reports_dashboard" => Some((Method::GET, "reports/dashboard")),
        // Etapa 9B (fase B2): identidad del establecimiento y logotipo (sin identificador: es única).
        "documents_identity" => Some((Method::GET, "documents/identity")),
        "documents_save_identity" => Some((Method::PUT, "documents/identity")),
        "documents_upload_identity_logo" => Some((Method::POST, "documents/identity/logo")),
        "documents_remove_identity_logo" => Some((Method::POST, "documents/identity/logo/remove")),
        "documents_preview_identity_logo" => {
            Some((Method::POST, "documents/identity/logo/preview"))
        }
        "registers" => Some((Method::GET, "registers")),
        "create_register" => Some((Method::POST, "registers")),
        "devices" => Some((Method::GET, "devices")),
        "invite_device" => Some((Method::POST, "devices/invitations")),
        "sessions" => Some((Method::GET, "sessions")),
        "audit" => Some((Method::GET, "audit")),
        "preferences" => Some((Method::GET, "preferences")),
        "save_preferences" => Some((Method::PUT, "preferences")),
        "save_terminal_preferences" => Some((Method::PUT, "terminal/preferences")),
        "catalog_categories" => Some((Method::GET, "catalog/categories")),
        "catalog_create_categories" => Some((Method::POST, "catalog/categories")),
        "catalog_products" => Some((Method::GET, "catalog/products")),
        "catalog_create_products" => Some((Method::POST, "catalog/products")),
        "catalog_ingredients" => Some((Method::GET, "catalog/ingredients")),
        "catalog_create_ingredients" => Some((Method::POST, "catalog/ingredients")),
        "catalog_complements" => Some((Method::GET, "catalog/complements")),
        "catalog_create_complements" => Some((Method::POST, "catalog/complements")),
        "catalog_portions" => Some((Method::GET, "catalog/portions")),
        "catalog_create_portions" => Some((Method::POST, "catalog/portions")),
        "catalog_groups" => Some((Method::GET, "catalog/groups")),
        "catalog_create_groups" => Some((Method::POST, "catalog/groups")),
        "catalog_price_lists" => Some((Method::GET, "catalog/price-lists")),
        "catalog_create_price_lists" => Some((Method::POST, "catalog/price-lists")),
        "catalog_promotions" => Some((Method::GET, "catalog/promotions")),
        "catalog_create_promotions" => Some((Method::POST, "catalog/promotions")),
        "catalog_lookups" => Some((Method::GET, "catalog/lookups")),
        "catalog_preview" => Some((Method::POST, "catalog/preview")),
        "catalog_upload_image" => Some((Method::POST, "catalog/images")),
        "catalog_reorder_categories" => Some((Method::POST, "catalog/categories/reorder")),
        // Etapa 7.1 (fase D): indicaciones rápidas por categoría; se leen en los lookups.
        "catalog_create_quick_note" => Some((Method::POST, "catalog/quick-notes")),
        "payments_settings" => Some((Method::GET, "payments/settings")),
        "payments_save_settings" => Some((Method::PUT, "payments/settings")),
        "payments_save_tax" => Some((Method::PUT, "payments/tax")),
        "payments_methods" => Some((Method::GET, "payments/methods")),
        "payments_create_method" => Some((Method::POST, "payments/methods")),
        "payments_current_shift" => Some((Method::GET, "payments/shifts/current")),
        "payments_open_shift" => Some((Method::POST, "payments/shifts")),
        "payments_save_usd_settings" => Some((Method::PUT, "payments/settings/usd")),
        // Etapa 7.1 (fase D): catálogo de motivos de descuento.
        "payments_discount_reasons" => Some((Method::GET, "payments/discount-reasons")),
        "payments_create_discount_reason" => Some((Method::POST, "payments/discount-reasons")),
        // Administración de caja (etapa 7): historial, catálogo y cajas con su turno activo.
        "payments_shifts" => Some((Method::GET, "payments/shifts")),
        "payments_cash_catalog" => Some((Method::GET, "payments/cash/catalog")),
        "payments_cash_registers" => Some((Method::GET, "payments/cash/registers")),
        "payments_create_denomination" => Some((Method::POST, "payments/cash/denominations")),
        "payments_create_category" => Some((Method::POST, "payments/cash/expense-categories")),
        // Vista previa del conteo por denominaciones (fase B): solo calcula, no escribe.
        "payments_cash_count_preview" => Some((Method::POST, "payments/cash/count-preview")),
        // Fase C: tope de caja, cajones por caja y apertura manual del cajón de la terminal.
        "payments_cash_limit" => Some((Method::GET, "payments/cash/limit")),
        "payments_save_cash_limit" => Some((Method::PUT, "payments/cash/limit")),
        "payments_cash_drawers" => Some((Method::GET, "payments/cash/drawers")),
        "payments_open_drawer" => Some((Method::POST, "payments/cash/open-drawer")),
        "restaurant_overview" => Some((Method::GET, "restaurant/overview")),
        "restaurant_areas" => Some((Method::GET, "restaurant/areas")),
        "restaurant_create_area" => Some((Method::POST, "restaurant/areas")),
        "restaurant_tables" => Some((Method::GET, "restaurant/tables")),
        "restaurant_create_table" => Some((Method::POST, "restaurant/tables")),
        "restaurant_save_layout" => Some((Method::PUT, "restaurant/layout")),
        "restaurant_settings" => Some((Method::GET, "restaurant/settings")),
        "restaurant_save_settings" => Some((Method::PUT, "restaurant/settings")),
        "restaurant_staff" => Some((Method::GET, "restaurant/staff")),
        "restaurant_accounts" => Some((Method::GET, "restaurant/accounts")),
        "restaurant_open_account" => Some((Method::POST, "restaurant/accounts")),
        // Etapa 8 (fase A): inventario.
        "inventory_summary" => Some((Method::GET, "inventory/summary")),
        "inventory_settings" => Some((Method::GET, "inventory/settings")),
        "inventory_save_settings" => Some((Method::PUT, "inventory/settings")),
        "inventory_units" => Some((Method::GET, "inventory/units")),
        "inventory_create_unit" => Some((Method::POST, "inventory/units")),
        "inventory_categories" => Some((Method::GET, "inventory/categories")),
        "inventory_create_category" => Some((Method::POST, "inventory/categories")),
        "inventory_items" => Some((Method::GET, "inventory/items")),
        "inventory_create_item" => Some((Method::POST, "inventory/items")),
        // Etapa 9A: comprobación y sugerencia del código interno de un insumo.
        "inventory_item_code" => Some((Method::GET, "inventory/item-codes")),
        "inventory_warehouses" => Some((Method::GET, "inventory/warehouses")),
        "inventory_create_warehouse" => Some((Method::POST, "inventory/warehouses")),
        "inventory_station_warehouses" => Some((Method::GET, "inventory/station-warehouses")),
        "inventory_stock" => Some((Method::GET, "inventory/stock")),
        "inventory_movements" => Some((Method::GET, "inventory/movements")),
        "inventory_record_initial_balances" => Some((Method::POST, "inventory/initial-balances")),
        // Etapa 8 (fase B): recetas, porciones, cancelaciones, elaboraciones y diagnóstico del consumo.
        "inventory_recipes" => Some((Method::GET, "inventory/recipes")),
        "inventory_create_recipe" => Some((Method::POST, "inventory/recipes")),
        "inventory_recipe_targets" => Some((Method::GET, "inventory/recipe-targets")),
        "inventory_ingredient_portions" => Some((Method::GET, "inventory/ingredient-portions")),
        "inventory_cancellations" => Some((Method::GET, "inventory/cancellations")),
        "inventory_productions" => Some((Method::GET, "inventory/productions")),
        "inventory_record_production" => Some((Method::POST, "inventory/productions")),
        "inventory_consumption_requests" => Some((Method::GET, "inventory/consumption-requests")),
        // Etapa 8 (fase C): proveedores, órdenes de compra, recepciones, devoluciones y gastos de caja.
        "inventory_suppliers" => Some((Method::GET, "inventory/suppliers")),
        "inventory_create_supplier" => Some((Method::POST, "inventory/suppliers")),
        "inventory_purchase_orders" => Some((Method::GET, "inventory/purchase-orders")),
        "inventory_create_purchase_order" => Some((Method::POST, "inventory/purchase-orders")),
        "inventory_receipts" => Some((Method::GET, "inventory/receipts")),
        "inventory_record_receipt" => Some((Method::POST, "inventory/receipts")),
        "inventory_supplier_returns" => Some((Method::GET, "inventory/supplier-returns")),
        "inventory_record_supplier_return" => Some((Method::POST, "inventory/supplier-returns")),
        "inventory_cash_expenses" => Some((Method::GET, "inventory/cash-expenses")),
        // Etapa 8 (fase D): ajustes, mermas, transferencias, conteos, niveles por almacén, alertas y
        // sugerencias de compra (el borrador desde sugerencias es una acción explícita; nunca emite).
        "inventory_adjustments" => Some((Method::GET, "inventory/adjustments")),
        "inventory_record_adjustment" => Some((Method::POST, "inventory/adjustments")),
        "inventory_transfers" => Some((Method::GET, "inventory/transfers")),
        "inventory_record_transfer" => Some((Method::POST, "inventory/transfers")),
        "inventory_counts" => Some((Method::GET, "inventory/counts")),
        "inventory_create_count" => Some((Method::POST, "inventory/counts")),
        "inventory_stock_levels" => Some((Method::GET, "inventory/stock-levels")),
        "inventory_save_stock_level" => Some((Method::PUT, "inventory/stock-levels")),
        "inventory_alerts" => Some((Method::GET, "inventory/alerts")),
        "inventory_purchase_suggestions" => Some((Method::GET, "inventory/purchase-suggestions")),
        "inventory_draft_order_from_suggestions" => {
            Some((Method::POST, "inventory/purchase-suggestions/draft-order"))
        }
        // Etapa 9B (fase A): diseñador de documentos. Plantillas por bloques, vista previa con la IR
        // de Go, documentos recientes, muestras impresas y compatibilidad de impresoras.
        "documents_catalog" => Some((Method::GET, "documents/catalog")),
        "documents_templates" => Some((Method::GET, "documents/templates")),
        "documents_create_template" => Some((Method::POST, "documents/templates")),
        "documents_restore_default" => Some((Method::POST, "documents/restore-default")),
        "documents_preview" => Some((Method::POST, "documents/preview")),
        "documents_recent" => Some((Method::GET, "documents/recent")),
        "documents_print_sample" => Some((Method::POST, "documents/samples")),
        "documents_printers" => Some((Method::GET, "documents/printers")),
        _ => None,
    };
    if let Some((method, path)) = fixed {
        return Ok((method, path.to_string()));
    }
    // Los reportes y las fuentes de filtros se identifican con una clave estable del registro
    // cerrado de Go (por ejemplo «sales.by_day»), no con un UUID.
    if operation == "reports_run" || operation == "reports_filters" {
        let identifier = report_key(id)?;
        let (method, path) = match operation {
            "reports_run" => (Method::GET, format!("reports/run/{identifier}")),
            "reports_filters" => (Method::GET, format!("reports/filters/{identifier}")),
            _ => return Err("Operación no autorizada por el puente nativo.".into()),
        };
        return Ok((method, path));
    }
    let identifier = resource_id(id)?;
    let (method, path) = match operation {
        "kitchen_save_station" => (Method::PATCH, format!("kitchen/stations/{identifier}")),
        "kitchen_save_printer" => (Method::PATCH, format!("kitchen/printers/{identifier}")),
        "kitchen_save_route" => (Method::PATCH, format!("kitchen/routes/{identifier}")),
        "kitchen_release" => (
            Method::POST,
            format!("kitchen/releases/{identifier}/release"),
        ),
        "kitchen_schedule" => (
            Method::POST,
            format!("kitchen/releases/{identifier}/schedule"),
        ),
        "kitchen_retry_change" => (Method::POST, format!("kitchen/changes/{identifier}/retry")),
        "kitchen_ticket" => (Method::GET, format!("kitchen/tickets/{identifier}")),
        "kitchen_ack_ticket" => (Method::POST, format!("kitchen/tickets/{identifier}/ack")),
        "kitchen_job" => (Method::GET, format!("kitchen/jobs/{identifier}")),
        "kitchen_retry_job" => (Method::POST, format!("kitchen/jobs/{identifier}/retry")),
        "kitchen_resolve_job" => (Method::POST, format!("kitchen/jobs/{identifier}/resolve")),

        "restaurant_draft_preview" => (
            Method::POST,
            format!("restaurant/accounts/{identifier}/draft-preview"),
        ),
        "catalog_get_categories" => (Method::GET, format!("catalog/categories/{identifier}")),
        "catalog_save_categories" => (Method::PATCH, format!("catalog/categories/{identifier}")),
        "catalog_get_products" => (Method::GET, format!("catalog/products/{identifier}")),
        "catalog_save_products" => (Method::PATCH, format!("catalog/products/{identifier}")),
        "catalog_get_ingredients" => (Method::GET, format!("catalog/ingredients/{identifier}")),
        "catalog_save_ingredients" => (Method::PATCH, format!("catalog/ingredients/{identifier}")),
        "catalog_get_complements" => (Method::GET, format!("catalog/complements/{identifier}")),
        "catalog_save_complements" => (Method::PATCH, format!("catalog/complements/{identifier}")),
        "catalog_get_portions" => (Method::GET, format!("catalog/portions/{identifier}")),
        "catalog_save_portions" => (Method::PATCH, format!("catalog/portions/{identifier}")),
        "catalog_get_groups" => (Method::GET, format!("catalog/groups/{identifier}")),
        "catalog_save_groups" => (Method::PATCH, format!("catalog/groups/{identifier}")),
        "catalog_get_price_lists" => (Method::GET, format!("catalog/price-lists/{identifier}")),
        "catalog_save_price_lists" => (Method::PATCH, format!("catalog/price-lists/{identifier}")),
        "catalog_get_promotions" => (Method::GET, format!("catalog/promotions/{identifier}")),
        "catalog_save_promotions" => (Method::PATCH, format!("catalog/promotions/{identifier}")),
        "catalog_image" => (Method::GET, format!("catalog/images/{identifier}")),
        "catalog_save_quick_note" => (Method::PATCH, format!("catalog/quick-notes/{identifier}")),
        "catalog_configuration" => (
            Method::GET,
            format!("catalog/products/{identifier}/configuration"),
        ),
        "catalog_product_state" => (
            Method::PATCH,
            format!("catalog/products/{identifier}/state"),
        ),
        "catalog_product_availability" => (
            Method::PATCH,
            format!("catalog/products/{identifier}/availability"),
        ),
        "catalog_duplicate_product" => (
            Method::POST,
            format!("catalog/products/{identifier}/duplicate"),
        ),
        "restaurant_save_area" => (Method::PATCH, format!("restaurant/areas/{identifier}")),
        "restaurant_save_table" => (Method::PATCH, format!("restaurant/tables/{identifier}")),
        // Etapa 7.1: fuera de servicio o habilitar una mesa desde la operación.
        "restaurant_table_service" => (
            Method::POST,
            format!("restaurant/tables/{identifier}/service"),
        ),
        "restaurant_account" => (Method::GET, format!("restaurant/accounts/{identifier}")),
        "restaurant_save_account" => (Method::PATCH, format!("restaurant/accounts/{identifier}")),
        "restaurant_save_draft" => (
            Method::PUT,
            format!("restaurant/accounts/{identifier}/draft"),
        ),
        "restaurant_preview" => (
            Method::GET,
            format!("restaurant/accounts/{identifier}/preview"),
        ),
        "restaurant_history" => (
            Method::GET,
            format!("restaurant/accounts/{identifier}/history"),
        ),
        "restaurant_confirm" => (
            Method::POST,
            format!("restaurant/accounts/{identifier}/confirm"),
        ),
        "restaurant_edit_preview" => (
            Method::POST,
            format!("restaurant/accounts/{identifier}/edit-preview"),
        ),
        "restaurant_edit" => (
            Method::POST,
            format!("restaurant/accounts/{identifier}/edit"),
        ),
        "restaurant_cancel" => (
            Method::POST,
            format!("restaurant/accounts/{identifier}/cancel"),
        ),
        "restaurant_split" => (
            Method::POST,
            format!("restaurant/accounts/{identifier}/split"),
        ),
        "restaurant_transfer" => (
            Method::POST,
            format!("restaurant/accounts/{identifier}/transfer"),
        ),
        "restaurant_merge" => (
            Method::POST,
            format!("restaurant/accounts/{identifier}/merge"),
        ),
        "restaurant_move" => (
            Method::POST,
            format!("restaurant/accounts/{identifier}/move"),
        ),
        "restaurant_assign" => (
            Method::POST,
            format!("restaurant/accounts/{identifier}/assign"),
        ),
        "restaurant_void" => (
            Method::POST,
            format!("restaurant/accounts/{identifier}/void"),
        ),
        // Etapa 7.1: «Liberar mesa» anula sin motivo una cuenta vacía.
        "restaurant_release_account" => (
            Method::POST,
            format!("restaurant/accounts/{identifier}/release"),
        ),
        "payments_save_method" => (Method::PATCH, format!("payments/methods/{identifier}")),
        "payments_save_discount_reason" => (
            Method::PATCH,
            format!("payments/discount-reasons/{identifier}"),
        ),
        "payments_shift_movements" => (
            Method::GET,
            format!("payments/shifts/{identifier}/movements"),
        ),
        "payments_shift_usd_rate" => (
            Method::POST,
            format!("payments/shifts/{identifier}/usd-rate"),
        ),
        // Turno de caja (etapa 7): un solo identificador por ruta; los registros relacionados
        // (la operación que se corrige) viajan en el cuerpo.
        "payments_shift" => (Method::GET, format!("payments/shifts/{identifier}")),
        "payments_shift_operations" => (
            Method::GET,
            format!("payments/shifts/{identifier}/operations"),
        ),
        "payments_shift_income" => (Method::POST, format!("payments/shifts/{identifier}/income")),
        "payments_shift_withdrawal" => (
            Method::POST,
            format!("payments/shifts/{identifier}/withdrawals"),
        ),
        "payments_shift_expense" => (
            Method::POST,
            format!("payments/shifts/{identifier}/expenses"),
        ),
        "payments_shift_correction" => (
            Method::POST,
            format!("payments/shifts/{identifier}/corrections"),
        ),
        "payments_shift_partial_cut" => (
            Method::POST,
            format!("payments/shifts/{identifier}/partial-cuts"),
        ),
        "payments_shift_start_closing" => (
            Method::POST,
            format!("payments/shifts/{identifier}/closing"),
        ),
        "payments_shift_closing" => (Method::GET, format!("payments/shifts/{identifier}/closing")),
        "payments_shift_save_count" => (
            Method::PUT,
            format!("payments/shifts/{identifier}/closing/count"),
        ),
        "payments_shift_cancel_closing" => (
            Method::POST,
            format!("payments/shifts/{identifier}/closing/cancel"),
        ),
        "payments_shift_confirm_closing" => (
            Method::POST,
            format!("payments/shifts/{identifier}/closing/confirm"),
        ),
        "payments_shift_closure" => (Method::GET, format!("payments/shifts/{identifier}/closure")),
        // Resumen en vivo del turno (fase B): los totales de un corte sin registrarlo.
        "payments_shift_summary" => (Method::GET, format!("payments/shifts/{identifier}/summary")),
        "payments_shift_comment_closure" => (
            Method::POST,
            format!("payments/shifts/{identifier}/closure/comments"),
        ),
        "payments_cash_report" => (Method::GET, format!("payments/cash-reports/{identifier}")),
        // Fase C: vista previa, impresión y reimpresión de documentos de caja y cajón por caja.
        "payments_cash_report_preview" => (
            Method::GET,
            format!("payments/cash-reports/{identifier}/preview"),
        ),
        "payments_print_cash_report" => (
            Method::POST,
            format!("payments/cash-reports/{identifier}/print"),
        ),
        "payments_cash_operation_preview" => (
            Method::GET,
            format!("payments/cash-operations/{identifier}/preview"),
        ),
        "payments_print_cash_operation" => (
            Method::POST,
            format!("payments/cash-operations/{identifier}/print"),
        ),
        "payments_reprint_cash_document" => (
            Method::POST,
            format!("payments/cash-documents/{identifier}/reprint"),
        ),
        "payments_shift_documents" => (
            Method::GET,
            format!("payments/shifts/{identifier}/documents"),
        ),
        "payments_save_cash_drawer" => (Method::PUT, format!("payments/cash/drawers/{identifier}")),
        // Etapa 7.1 (fase C): «Probar apertura» del cajón de una caja y retiros o gastos rechazados
        // por efectivo insuficiente de un turno (solo con el permiso de consultar el esperado).
        "payments_test_cash_drawer" => (
            Method::POST,
            format!("payments/cash/drawers/{identifier}/test"),
        ),
        "payments_shift_rejected_outflows" => (
            Method::GET,
            format!("payments/shifts/{identifier}/rejected-outflows"),
        ),
        "payments_save_denomination" => (
            Method::PATCH,
            format!("payments/cash/denominations/{identifier}"),
        ),
        "payments_save_category" => (
            Method::PATCH,
            format!("payments/cash/expense-categories/{identifier}"),
        ),
        "payments_account" => (Method::GET, format!("payments/accounts/{identifier}")),
        "payments_preview" => (
            Method::POST,
            format!("payments/accounts/{identifier}/preview"),
        ),
        "payments_pay" => (
            Method::POST,
            format!("payments/accounts/{identifier}/payments"),
        ),
        "payments_deliver" => (
            Method::POST,
            format!("payments/accounts/{identifier}/deliver"),
        ),
        "payments_discount_preview" => (
            Method::POST,
            format!("payments/accounts/{identifier}/discounts/preview"),
        ),
        "payments_discount" => (
            Method::POST,
            format!("payments/accounts/{identifier}/discounts"),
        ),
        "payments_charge" => (
            Method::POST,
            format!("payments/accounts/{identifier}/charges"),
        ),
        "payments_void_adjustment" => (
            Method::POST,
            format!("payments/accounts/{identifier}/adjustments/void"),
        ),
        "payments_refund_preview" => (
            Method::POST,
            format!("payments/accounts/{identifier}/refunds/preview"),
        ),
        "payments_refund" => (
            Method::POST,
            format!("payments/accounts/{identifier}/refunds"),
        ),
        "payments_correct" => (
            Method::POST,
            format!("payments/accounts/{identifier}/corrections"),
        ),
        "payments_precheck" => (
            Method::GET,
            format!("payments/accounts/{identifier}/precheck"),
        ),
        "payments_print_precheck" => (
            Method::POST,
            format!("payments/accounts/{identifier}/precheck/print"),
        ),
        "payments_reprint" => (
            Method::POST,
            format!("payments/accounts/{identifier}/documents/reprint"),
        ),
        // Etapa 9B: ticket, precuenta o documento de caja histórico con la plantilla vigente.
        "payments_rerender" => (
            Method::POST,
            format!("payments/accounts/{identifier}/documents/rerender"),
        ),
        "payments_rerender_cash_document" => (
            Method::POST,
            format!("payments/cash-documents/{identifier}/rerender"),
        ),
        // Etapa 9B (fase A): una plantilla, sus acciones y sus versiones (un identificador por ruta).
        "documents_template" => (Method::GET, format!("documents/templates/{identifier}")),
        "documents_save_template" => (Method::PUT, format!("documents/templates/{identifier}")),
        "documents_rename_template" => (
            Method::POST,
            format!("documents/templates/{identifier}/rename"),
        ),
        "documents_activate_template" => (
            Method::POST,
            format!("documents/templates/{identifier}/activate"),
        ),
        "documents_template_versions" => (
            Method::GET,
            format!("documents/templates/{identifier}/versions"),
        ),
        "documents_version" => (Method::GET, format!("documents/versions/{identifier}")),
        "update_user" => (Method::PATCH, format!("users/{identifier}")),
        "user_permissions" => (Method::GET, format!("users/{identifier}/permissions")),
        "save_user_permissions" => (Method::PUT, format!("users/{identifier}/permissions")),
        "user_preferences" => (Method::GET, format!("users/{identifier}/preferences")),
        "save_user_preferences" => (Method::PUT, format!("users/{identifier}/preferences")),
        "update_role" => (Method::PATCH, format!("roles/{identifier}")),
        "update_register" => (Method::PATCH, format!("registers/{identifier}")),
        "update_device" => (Method::PATCH, format!("devices/{identifier}")),
        "revoke_device" => (Method::POST, format!("devices/{identifier}/revoke")),
        "revoke_session" => (Method::POST, format!("sessions/{identifier}/revoke")),
        // Etapa 8 (fase A): inventario, un solo identificador por ruta.
        "inventory_save_unit" => (Method::PATCH, format!("inventory/units/{identifier}")),
        "inventory_save_category" => (Method::PATCH, format!("inventory/categories/{identifier}")),
        "inventory_item" => (Method::GET, format!("inventory/items/{identifier}")),
        "inventory_save_item" => (Method::PATCH, format!("inventory/items/{identifier}")),
        "inventory_create_presentation" => (
            Method::POST,
            format!("inventory/items/{identifier}/presentations"),
        ),
        "inventory_save_presentation" => (
            Method::PATCH,
            format!("inventory/presentations/{identifier}"),
        ),
        "inventory_save_warehouse" => (Method::PATCH, format!("inventory/warehouses/{identifier}")),
        "inventory_assign_station_warehouse" => (
            Method::PUT,
            format!("inventory/station-warehouses/{identifier}"),
        ),
        // Etapa 8 (fase B): un solo identificador por ruta (la versión de receta tiene ruta propia).
        "inventory_recipe" => (Method::GET, format!("inventory/recipes/{identifier}")),
        "inventory_save_recipe" => (Method::PUT, format!("inventory/recipes/{identifier}")),
        "inventory_recipe_version" => (
            Method::GET,
            format!("inventory/recipe-versions/{identifier}"),
        ),
        "inventory_save_ingredient_portion" => (
            Method::PUT,
            format!("inventory/ingredient-portions/{identifier}"),
        ),
        "inventory_classify_cancellation" => (
            Method::POST,
            format!("inventory/cancellations/{identifier}/classify"),
        ),
        "inventory_unit_consumption" => {
            (Method::GET, format!("inventory/consumption/{identifier}"))
        }
        // Etapa 8 (fase C): el insumo de un proveedor viaja en el cuerpo (un solo identificador).
        "inventory_supplier" => (Method::GET, format!("inventory/suppliers/{identifier}")),
        "inventory_save_supplier" => (Method::PATCH, format!("inventory/suppliers/{identifier}")),
        "inventory_save_supplier_item" => (
            Method::PUT,
            format!("inventory/suppliers/{identifier}/items"),
        ),
        "inventory_remove_supplier_item" => (
            Method::POST,
            format!("inventory/suppliers/{identifier}/items/remove"),
        ),
        "inventory_purchase_order" => (
            Method::GET,
            format!("inventory/purchase-orders/{identifier}"),
        ),
        "inventory_save_purchase_order" => (
            Method::PATCH,
            format!("inventory/purchase-orders/{identifier}"),
        ),
        "inventory_issue_purchase_order" => (
            Method::POST,
            format!("inventory/purchase-orders/{identifier}/issue"),
        ),
        "inventory_cancel_purchase_order" => (
            Method::POST,
            format!("inventory/purchase-orders/{identifier}/cancel"),
        ),
        "inventory_close_purchase_order" => (
            Method::POST,
            format!("inventory/purchase-orders/{identifier}/close"),
        ),
        "inventory_receipt" => (Method::GET, format!("inventory/receipts/{identifier}")),
        "inventory_link_cash_expense" => (
            Method::POST,
            format!("inventory/receipts/{identifier}/cash-links"),
        ),
        "inventory_unlink_cash_expense" => (
            Method::POST,
            format!("inventory/cash-links/{identifier}/unlink"),
        ),
        "inventory_supplier_return" => (
            Method::GET,
            format!("inventory/supplier-returns/{identifier}"),
        ),
        // Etapa 8 (fase D): un solo identificador por ruta.
        "inventory_adjustment" => (Method::GET, format!("inventory/adjustments/{identifier}")),
        "inventory_authorize_adjustment" => (
            Method::POST,
            format!("inventory/adjustments/{identifier}/authorize"),
        ),
        "inventory_reject_adjustment" => (
            Method::POST,
            format!("inventory/adjustments/{identifier}/reject"),
        ),
        "inventory_transfer" => (Method::GET, format!("inventory/transfers/{identifier}")),
        "inventory_count" => (Method::GET, format!("inventory/counts/{identifier}")),
        "inventory_save_count" => (Method::PATCH, format!("inventory/counts/{identifier}")),
        "inventory_capture_count_lines" => {
            (Method::PUT, format!("inventory/counts/{identifier}/lines"))
        }
        "inventory_submit_count" => (
            Method::POST,
            format!("inventory/counts/{identifier}/submit"),
        ),
        "inventory_approve_count" => (
            Method::POST,
            format!("inventory/counts/{identifier}/approve"),
        ),
        "inventory_apply_count" => (Method::POST, format!("inventory/counts/{identifier}/apply")),
        "inventory_cancel_count" => (
            Method::POST,
            format!("inventory/counts/{identifier}/cancel"),
        ),
        _ => return Err("Operación no autorizada por el puente nativo.".into()),
    };
    Ok((method, path))
}

#[derive(Serialize)]
pub struct LocalState {
    enrolled: bool,
    has_session: bool,
    local_setup_available: bool,
    local_enrollment_available: bool,
}

#[tauri::command]
pub fn authentication_state(state: State<'_, TerminalState>) -> Result<LocalState, String> {
    let vault = state.vault()?;
    let credentials = vault.read()?;
    Ok(LocalState {
        enrolled: credentials.device_id.is_some() && credentials.device_token.is_some(),
        has_session: credentials.session_token.is_some(),
        local_setup_available: state.provisioning_token.is_some(),
        local_enrollment_available: state.enrollment_code.is_some(),
    })
}

#[tauri::command]
pub async fn api_request(
    state: State<'_, TerminalState>,
    operation: String,
    resource_id: Option<String>,
    mut body: Option<Value>,
    query: Option<BTreeMap<String, String>>,
) -> Result<HTTPResult, String> {
    let config = state.config.as_ref().map_err(Clone::clone)?;
    let vault = state.vault()?;
    let client = state.client.as_ref().map_err(Clone::clone)?;
    let mut credentials = vault.read()?;
    let (method, path) = route(&operation, resource_id.as_deref())?;
    let mut request = client.request(method, format!("{}/api/v1/{path}", config.server_url));
    if let (Some(id), Some(token)) = (&credentials.device_id, &credentials.device_token) {
        request = request
            .header("X-Device-ID", id)
            .header("X-Device-Token", token);
    }
    if let Some(token) = &credentials.session_token {
        request = request.bearer_auth(token);
    }
    if operation == "setup" {
        let token = state
            .provisioning_token
            .as_ref()
            .ok_or("Prepara primero el aprovisionamiento local desde Terminal.")?;
        request = request.header("X-Provisioning-Token", token);
    }
    if operation == "enroll_local" {
        let code = state
            .enrollment_code
            .as_ref()
            .ok_or("No existe una autorización local disponible.")?;
        let name = body
            .as_ref()
            .and_then(|value| value.get("name"))
            .and_then(Value::as_str)
            .unwrap_or("");
        body = Some(serde_json::json!({"name": name, "code": code}));
    }
    if let Some(query) = query {
        if !query_allowed(&operation, &query) {
            return Err("Parámetros de consulta no autorizados.".into());
        }
        request = request.query(&query);
    }
    // Los reportes tienen su propio tiempo de respuesta en Go (40 s): el puente espera un poco más.
    if operation.starts_with("reports_") {
        request = request.timeout(Duration::from_secs(REPORT_TIMEOUT_SECONDS));
    }
    if let Some(body) = body {
        request = request.json(&body);
    }
    let mut response = request.send().await.map_err(|_| {
        "No se pudo conectar con el servidor. Comprueba su estado y la confianza TLS."
    })?;
    let status = response.status().as_u16();
    let mut bytes = Vec::new();
    while let Some(chunk) = response
        .chunk()
        .await
        .map_err(|_| "Se interrumpió la respuesta del servidor.")?
    {
        if bytes.len() + chunk.len() > 2 * 1024 * 1024 {
            return Err("La respuesta excede el límite admitido.".into());
        }
        bytes.extend_from_slice(&chunk);
    }
    let mut result: Value = serde_json::from_slice(&bytes)
        .map_err(|_| "El servidor no respondió con un contrato válido.")?;
    if status == 200 {
        if let Some(token) = result.get("device_token").and_then(Value::as_str) {
            credentials.device_token = Some(token.to_string());
            credentials.device_id = result
                .get("device_id")
                .and_then(Value::as_str)
                .map(str::to_string);
            credentials.session_token = None;
            vault.save(credentials.clone())?;
        }
        if let Some(token) = result.get("session_token").and_then(Value::as_str) {
            credentials.session_token = Some(token.to_string());
            vault.save(credentials.clone())?;
        }
        if operation == "logout" {
            credentials.session_token = None;
            vault.save(credentials)?;
        }
    } else if invalidates_credentials(status, &result) {
        credentials.session_token = None;
        let code = result
            .pointer("/error/code")
            .and_then(Value::as_str)
            .unwrap_or("");
        if code == "DEVICE_REVOKED" || code == "DEVICE_REQUIRED" {
            credentials.device_id = None;
            credentials.device_token = None;
        }
        vault.save(credentials)?;
    }
    sanitize_response(&mut result);
    Ok(HTTPResult {
        status,
        body: result,
    })
}

// Lista blanca de parámetros de consulta por operación: cualquier otro parámetro se rechaza antes
// de llegar a Go.
pub(crate) fn allowed_query_keys(operation: &str) -> &'static [&'static str] {
    if operation == "audit" {
        &[
            "page",
            "page_size",
            "actor",
            "action",
            "device_id",
            "from",
            "until",
        ]
    } else if operation.starts_with("documents_") {
        // Diseñador de documentos (etapa 9B): solo el tipo de documento en las listas.
        &["kind"]
    } else if operation.starts_with("kitchen_") {
        &["page", "station_id", "state", "account_id"]
    } else if operation == "payments_shifts" {
        // Historial de turnos: caja, intervalo de fechas comerciales, estado y página.
        &["page", "register_id", "from", "until", "state"]
    } else if operation.starts_with("restaurant_") {
        &["page", "page_size", "search", "mode", "table_id", "state"]
    } else if operation.starts_with("catalog_") {
        &[
            "page",
            "page_size",
            "search",
            "active",
            "category_id",
            "kind",
            "archived",
            "price_list_id",
        ]
    } else if operation.starts_with("reports_") {
        // Reportes (etapa 9A): periodo, comparación, página, orden, búsqueda de opciones y los
        // filtros que declara cada reporte (Go rechaza los que el reporte no admite).
        &[
            "period",
            "date",
            "week",
            "month",
            "year",
            "from",
            "until",
            "compare",
            "page",
            "page_size",
            "sort",
            "direction",
            "search",
            "mode",
            "area_id",
            "table_id",
            "category_id",
            "product_id",
            "price_list_id",
            "origin",
            "register_id",
            "shift_id",
            "method_id",
            "user_id",
            "reason_id",
            "kind",
            "state",
            "group_by",
            "scope",
            // Fase B2: almacén, insumo, proveedor, modalidad de compra y moneda de caja.
            "warehouse_id",
            "item_id",
            "supplier_id",
            "modality",
            "currency",
        ]
    } else if operation.starts_with("inventory_") {
        // Inventario (etapa 8): búsqueda, filtros de listados y paginación.
        &[
            "page",
            "page_size",
            "search",
            "active",
            "category_id",
            "kind",
            "warehouse_id",
            "item_id",
            "state",
            "source_kind",
            "source_id",
            "from",
            "until",
            // Fase B: tipo de receta, producto, porción configurada.
            "owner_kind",
            "product_id",
            "configured",
            // Fase C: proveedor, orden, recepción, caja, recepción directa e importes de gastos.
            "supplier_id",
            "purchase_order_id",
            "receipt_id",
            "register_id",
            "direct",
            "min_amount",
            "max_amount",
            // Fase D: tipo y gravedad de alertas y motivo de ajustes.
            "type",
            "severity",
            "reason",
            // Etapa 9A: código interno que se comprueba y modalidad del proveedor de una recepción.
            "code",
            "supplier_kind",
        ]
    } else {
        &[]
    }
}

fn query_allowed(operation: &str, query: &BTreeMap<String, String>) -> bool {
    let allowed = allowed_query_keys(operation);
    query.keys().all(|key| allowed.contains(&key.as_str()))
}

pub(crate) fn sanitize_response(result: &mut Value) {
    // Los secretos duraderos nunca cruzan al WebView ni forman parte del estado React.
    if let Some(object) = result.as_object_mut() {
        object.remove("session_token");
        object.remove("device_token");
    }
}

pub(crate) fn invalidates_credentials(status: u16, result: &Value) -> bool {
    let code = result
        .pointer("/error/code")
        .and_then(Value::as_str)
        .unwrap_or("");
    // Una contraseña actual incorrecta no invalida una sesión que Go sigue considerando válida.
    status == 401 && (code.starts_with("SESSION_") || code.starts_with("DEVICE_"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn payment_routes_are_closed_and_keep_write_methods() {
        let identifier = "00000000-0000-4000-8000-000000000001";
        assert_eq!(
            route("payments_pay", Some(identifier)).unwrap(),
            (
                Method::POST,
                format!("payments/accounts/{identifier}/payments")
            )
        );
        assert_eq!(
            route("payments_account", Some(identifier)).unwrap(),
            (Method::GET, format!("payments/accounts/{identifier}"))
        );
        assert_eq!(
            route("payments_save_tax", None).unwrap(),
            (Method::PUT, "payments/tax".to_string())
        );
        assert_eq!(
            route("payments_refund", Some(identifier)).unwrap().0,
            Method::POST
        );
        assert!(route("payments_pay", Some("../../users")).is_err());
        assert!(route("payments_pay", None).is_err());
        assert!(route("payments_delete", Some(identifier)).is_err());
    }

    #[test]
    fn feature_and_usd_routes_are_closed_and_keep_write_methods() {
        let identifier = "00000000-0000-4000-8000-000000000001";
        assert_eq!(
            route("settings_features", None).unwrap(),
            (Method::GET, "settings/features".to_string())
        );
        assert_eq!(
            route("save_settings_feature", None).unwrap(),
            (Method::PUT, "settings/features".to_string())
        );
        assert_eq!(
            route("payments_save_usd_settings", None).unwrap(),
            (Method::PUT, "payments/settings/usd".to_string())
        );
        assert_eq!(
            route("payments_shift_usd_rate", Some(identifier)).unwrap(),
            (
                Method::POST,
                format!("payments/shifts/{identifier}/usd-rate")
            )
        );
        assert!(route("payments_shift_usd_rate", Some("../../settings")).is_err());
        assert!(route("payments_shift_usd_rate", None).is_err());
        assert!(route("save_settings_feature_usd_cash", None).is_err());
    }

    #[test]
    fn cash_routes_are_closed_and_keep_write_methods() {
        let identifier = "00000000-0000-4000-8000-000000000001";
        for (operation, method, path) in [
            (
                "payments_shift",
                Method::GET,
                format!("payments/shifts/{identifier}"),
            ),
            (
                "payments_shift_income",
                Method::POST,
                format!("payments/shifts/{identifier}/income"),
            ),
            (
                "payments_shift_withdrawal",
                Method::POST,
                format!("payments/shifts/{identifier}/withdrawals"),
            ),
            (
                "payments_shift_expense",
                Method::POST,
                format!("payments/shifts/{identifier}/expenses"),
            ),
            (
                "payments_shift_correction",
                Method::POST,
                format!("payments/shifts/{identifier}/corrections"),
            ),
            (
                "payments_shift_partial_cut",
                Method::POST,
                format!("payments/shifts/{identifier}/partial-cuts"),
            ),
            (
                "payments_shift_start_closing",
                Method::POST,
                format!("payments/shifts/{identifier}/closing"),
            ),
            (
                "payments_shift_closing",
                Method::GET,
                format!("payments/shifts/{identifier}/closing"),
            ),
            (
                "payments_shift_save_count",
                Method::PUT,
                format!("payments/shifts/{identifier}/closing/count"),
            ),
            (
                "payments_shift_cancel_closing",
                Method::POST,
                format!("payments/shifts/{identifier}/closing/cancel"),
            ),
            (
                "payments_shift_confirm_closing",
                Method::POST,
                format!("payments/shifts/{identifier}/closing/confirm"),
            ),
            (
                "payments_shift_closure",
                Method::GET,
                format!("payments/shifts/{identifier}/closure"),
            ),
            (
                "payments_shift_comment_closure",
                Method::POST,
                format!("payments/shifts/{identifier}/closure/comments"),
            ),
            (
                "payments_cash_report",
                Method::GET,
                format!("payments/cash-reports/{identifier}"),
            ),
            (
                "payments_shift_summary",
                Method::GET,
                format!("payments/shifts/{identifier}/summary"),
            ),
            (
                "payments_save_denomination",
                Method::PATCH,
                format!("payments/cash/denominations/{identifier}"),
            ),
            (
                "payments_save_category",
                Method::PATCH,
                format!("payments/cash/expense-categories/{identifier}"),
            ),
        ] {
            assert_eq!(route(operation, Some(identifier)).unwrap(), (method, path));
            assert!(route(operation, None).is_err());
            assert!(route(operation, Some("../../settings")).is_err());
        }
        for (operation, method, path) in [
            ("payments_shifts", Method::GET, "payments/shifts"),
            (
                "payments_cash_catalog",
                Method::GET,
                "payments/cash/catalog",
            ),
            (
                "payments_cash_registers",
                Method::GET,
                "payments/cash/registers",
            ),
            (
                "payments_create_denomination",
                Method::POST,
                "payments/cash/denominations",
            ),
            (
                "payments_create_category",
                Method::POST,
                "payments/cash/expense-categories",
            ),
            (
                "payments_cash_count_preview",
                Method::POST,
                "payments/cash/count-preview",
            ),
        ] {
            assert_eq!(route(operation, None).unwrap(), (method, path.to_string()));
        }
        assert!(route("payments_shift_delete", Some(identifier)).is_err());
        assert!(route("payments_shift_reopen", Some(identifier)).is_err());
    }

    #[test]
    fn cash_printing_drawer_and_limit_routes_are_closed() {
        let identifier = "00000000-0000-4000-8000-000000000001";
        for (operation, method, path) in [
            (
                "payments_cash_report_preview",
                Method::GET,
                format!("payments/cash-reports/{identifier}/preview"),
            ),
            (
                "payments_print_cash_report",
                Method::POST,
                format!("payments/cash-reports/{identifier}/print"),
            ),
            (
                "payments_cash_operation_preview",
                Method::GET,
                format!("payments/cash-operations/{identifier}/preview"),
            ),
            (
                "payments_print_cash_operation",
                Method::POST,
                format!("payments/cash-operations/{identifier}/print"),
            ),
            (
                "payments_reprint_cash_document",
                Method::POST,
                format!("payments/cash-documents/{identifier}/reprint"),
            ),
            (
                "payments_shift_documents",
                Method::GET,
                format!("payments/shifts/{identifier}/documents"),
            ),
            (
                "payments_save_cash_drawer",
                Method::PUT,
                format!("payments/cash/drawers/{identifier}"),
            ),
            (
                "payments_test_cash_drawer",
                Method::POST,
                format!("payments/cash/drawers/{identifier}/test"),
            ),
            (
                "payments_shift_rejected_outflows",
                Method::GET,
                format!("payments/shifts/{identifier}/rejected-outflows"),
            ),
        ] {
            assert_eq!(route(operation, Some(identifier)).unwrap(), (method, path));
            assert!(route(operation, None).is_err());
            assert!(route(operation, Some("../../settings")).is_err());
        }
        for (operation, method, path) in [
            ("payments_cash_limit", Method::GET, "payments/cash/limit"),
            (
                "payments_save_cash_limit",
                Method::PUT,
                "payments/cash/limit",
            ),
            (
                "payments_cash_drawers",
                Method::GET,
                "payments/cash/drawers",
            ),
            (
                "payments_open_drawer",
                Method::POST,
                "payments/cash/open-drawer",
            ),
        ] {
            assert_eq!(route(operation, None).unwrap(), (method, path.to_string()));
        }
        // Ninguna operación permite abrir el cajón de otra caja ni reimprimir por ruta libre; la
        // prueba de apertura exige el identificador de su caja y no admite rutas arbitrarias.
        assert!(route("payments_open_drawer_register", Some(identifier)).is_err());
        assert!(route("payments_test_cash_drawer", Some("../open-drawer")).is_err());
        assert!(route("payments_shift_rejected_outflows", None).is_err());
        assert!(route("payments_cash_document_delete", Some(identifier)).is_err());
    }

    #[test]
    fn kitchen_routes_are_closed_and_keep_write_methods() {
        let identifier = "00000000-0000-4000-8000-000000000001";
        for (operation, method, path) in [
            (
                "kitchen_save_station",
                Method::PATCH,
                format!("kitchen/stations/{identifier}"),
            ),
            (
                "kitchen_retry_job",
                Method::POST,
                format!("kitchen/jobs/{identifier}/retry"),
            ),
            (
                "kitchen_ack_ticket",
                Method::POST,
                format!("kitchen/tickets/{identifier}/ack"),
            ),
        ] {
            assert_eq!(route(operation, Some(identifier)).unwrap(), (method, path));
        }
        assert!(route("kitchen_execute", Some(identifier)).is_err());
        assert!(route("kitchen_job", Some("../../settings")).is_err());
        assert_eq!(route("kitchen_save_settings", None).unwrap().0, Method::PUT);
    }

    #[test]
    fn rejects_arbitrary_routes_and_identifiers() {
        assert!(route("https://attacker.test", None).is_err());
        assert!(route("update_user", Some("../../settings")).is_err());
        assert!(route("me", None).is_ok());
        assert!(route("update_user", Some("00000000-0000-0000-0000-000000000001")).is_ok());
    }

    #[test]
    fn catalog_routes_are_fixed_and_preserve_methods() {
        let identifier = "00000000-0000-0000-0000-000000000001";
        let (method, path) = route("catalog_preview", None).unwrap();
        assert_eq!(method, Method::POST);
        assert_eq!(path, "catalog/preview");
        let (method, path) = route("catalog_save_groups", Some(identifier)).unwrap();
        assert_eq!(method, Method::PATCH);
        assert_eq!(path, format!("catalog/groups/{identifier}"));
        let (_, path) = route("catalog_configuration", Some(identifier)).unwrap();
        assert_eq!(path, format!("catalog/products/{identifier}/configuration"));
        assert!(route("catalog_save_groups", None).is_err());
        assert!(route("catalog_delete_everything", Some(identifier)).is_err());
        assert!(route("catalog_image", Some("../../../../private/secrets")).is_err());
    }

    #[test]
    fn restaurant_routes_preserve_methods_and_reject_unimplemented_actions() {
        let identifier = "00000000-0000-0000-0000-000000000001";
        for (operation, method, suffix) in [
            ("restaurant_save_draft", Method::PUT, "draft"),
            ("restaurant_draft_preview", Method::POST, "draft-preview"),
            ("restaurant_confirm", Method::POST, "confirm"),
            ("restaurant_edit", Method::POST, "edit"),
            ("restaurant_split", Method::POST, "split"),
            ("restaurant_transfer", Method::POST, "transfer"),
            ("restaurant_merge", Method::POST, "merge"),
        ] {
            let (actual_method, path) = route(operation, Some(identifier)).unwrap();
            assert_eq!(actual_method, method);
            assert_eq!(path, format!("restaurant/accounts/{identifier}/{suffix}"));
            assert!(route(operation, None).is_err());
        }
        let (method, path) = route("restaurant_release_account", Some(identifier)).unwrap();
        assert_eq!(method, Method::POST);
        assert_eq!(path, format!("restaurant/accounts/{identifier}/release"));
        assert!(route("restaurant_release_account", None).is_err());
        assert!(route("restaurant_release_account", Some("../../settings")).is_err());
        let (method, path) = route("restaurant_table_service", Some(identifier)).unwrap();
        assert_eq!(method, Method::POST);
        assert_eq!(path, format!("restaurant/tables/{identifier}/service"));
        assert!(route("restaurant_table_service", None).is_err());
        assert!(route("restaurant_table_service", Some("../../settings")).is_err());
        assert!(route("restaurant_deliver", Some(identifier)).is_err());
        assert!(route("restaurant_pay", Some(identifier)).is_err());
        assert!(route("restaurant_confirm", Some("../../settings")).is_err());
    }

    #[test]
    fn discount_reasons_and_quick_notes_routes_are_closed() {
        let identifier = "00000000-0000-4000-8000-000000000001";
        for (operation, method, path) in [
            (
                "payments_discount_reasons",
                Method::GET,
                "payments/discount-reasons",
            ),
            (
                "payments_create_discount_reason",
                Method::POST,
                "payments/discount-reasons",
            ),
            (
                "catalog_create_quick_note",
                Method::POST,
                "catalog/quick-notes",
            ),
        ] {
            assert_eq!(route(operation, None).unwrap(), (method, path.to_string()));
        }
        for (operation, method, path) in [
            (
                "payments_save_discount_reason",
                Method::PATCH,
                format!("payments/discount-reasons/{identifier}"),
            ),
            (
                "catalog_save_quick_note",
                Method::PATCH,
                format!("catalog/quick-notes/{identifier}"),
            ),
        ] {
            assert_eq!(route(operation, Some(identifier)).unwrap(), (method, path));
            assert!(route(operation, None).is_err());
            assert!(route(operation, Some("../../settings")).is_err());
        }
        // No hay borrado de motivos ni de indicaciones: se desactivan.
        assert!(route("payments_delete_discount_reason", Some(identifier)).is_err());
        assert!(route("catalog_delete_quick_note", Some(identifier)).is_err());
    }

    #[test]
    fn report_routes_are_closed_read_only_and_keyed() {
        for (operation, method, path) in [
            ("reports_catalog", Method::GET, "reports/catalog"),
            ("reports_dashboard", Method::GET, "reports/dashboard"),
            (
                "settings_business_day",
                Method::GET,
                "settings/business-day",
            ),
            (
                "save_settings_business_day",
                Method::PUT,
                "settings/business-day",
            ),
        ] {
            assert_eq!(route(operation, None).unwrap(), (method, path.to_string()));
        }
        assert_eq!(
            route("reports_run", Some("sales.by_day")).unwrap(),
            (Method::GET, "reports/run/sales.by_day".to_string())
        );
        assert_eq!(
            route("reports_filters", Some("payment_methods")).unwrap(),
            (Method::GET, "reports/filters/payment_methods".to_string())
        );
        for invalid in [
            "../settings",
            "sales/by_day",
            "Sales.by_day",
            "sales..by_day",
            "",
            ".sales",
            "sales.by-day",
            "sales.by_day?x=1",
        ] {
            assert!(route("reports_run", Some(invalid)).is_err());
            assert!(route("reports_filters", Some(invalid)).is_err());
        }
        assert!(route("reports_run", None).is_err());
        assert!(route("reports_export", Some("sales.by_day")).is_err());
        assert!(route("reports_run", Some(&"a".repeat(65))).is_err());
        let mut query = BTreeMap::new();
        for key in [
            "period",
            "week",
            "compare",
            "page",
            "page_size",
            "sort",
            "direction",
            "category_id",
            "reason_id",
            "group_by",
            "search",
            "warehouse_id",
            "item_id",
            "supplier_id",
            "modality",
            "currency",
        ] {
            query.insert(key.to_string(), "valor".to_string());
        }
        assert!(query_allowed("reports_run", &query));
        assert!(query_allowed("reports_dashboard", &query));
        let mut rejected = BTreeMap::new();
        rejected.insert("sql".to_string(), "SELECT 1".to_string());
        assert!(!query_allowed("reports_run", &rejected));
        let mut foreign = BTreeMap::new();
        foreign.insert("period".to_string(), "today".to_string());
        assert!(!query_allowed("payments_shifts", &foreign));
    }

    #[test]
    fn document_designer_routes_are_closed_and_keep_write_methods() {
        let identifier = "00000000-0000-4000-8000-000000000009";
        for (operation, method, path) in [
            ("documents_catalog", Method::GET, "documents/catalog"),
            ("documents_templates", Method::GET, "documents/templates"),
            (
                "documents_create_template",
                Method::POST,
                "documents/templates",
            ),
            (
                "documents_restore_default",
                Method::POST,
                "documents/restore-default",
            ),
            ("documents_preview", Method::POST, "documents/preview"),
            ("documents_recent", Method::GET, "documents/recent"),
            ("documents_print_sample", Method::POST, "documents/samples"),
            ("documents_printers", Method::GET, "documents/printers"),
            (
                "kitchen_rerender",
                Method::POST,
                "kitchen/print-jobs/rerender",
            ),
        ] {
            assert_eq!(route(operation, None).unwrap(), (method, path.to_string()));
        }
        for (operation, method, path) in [
            (
                "documents_template",
                Method::GET,
                format!("documents/templates/{identifier}"),
            ),
            (
                "documents_save_template",
                Method::PUT,
                format!("documents/templates/{identifier}"),
            ),
            (
                "documents_rename_template",
                Method::POST,
                format!("documents/templates/{identifier}/rename"),
            ),
            (
                "documents_activate_template",
                Method::POST,
                format!("documents/templates/{identifier}/activate"),
            ),
            (
                "documents_template_versions",
                Method::GET,
                format!("documents/templates/{identifier}/versions"),
            ),
            (
                "documents_version",
                Method::GET,
                format!("documents/versions/{identifier}"),
            ),
            (
                "payments_rerender",
                Method::POST,
                format!("payments/accounts/{identifier}/documents/rerender"),
            ),
            (
                "payments_rerender_cash_document",
                Method::POST,
                format!("payments/cash-documents/{identifier}/rerender"),
            ),
        ] {
            assert_eq!(route(operation, Some(identifier)).unwrap(), (method, path));
            assert!(route(operation, Some("../templates")).is_err());
            assert!(route(operation, None).is_err());
        }
        assert!(route("documents_delete_template", Some(identifier)).is_err());
        let mut kind = BTreeMap::new();
        kind.insert("kind".to_string(), "receipt".to_string());
        assert!(query_allowed("documents_templates", &kind));
        assert!(query_allowed("documents_recent", &kind));
        let mut rejected = BTreeMap::new();
        rejected.insert("layout".to_string(), "{}".to_string());
        assert!(!query_allowed("documents_templates", &rejected));
        assert!(!query_allowed("documents_recent", &rejected));
    }

    #[test]
    fn inventory_routes_are_closed_and_keep_write_methods() {
        let identifier = "00000000-0000-4000-8000-000000000001";
        for (operation, method, path) in [
            ("inventory_summary", Method::GET, "inventory/summary"),
            ("inventory_settings", Method::GET, "inventory/settings"),
            ("inventory_save_settings", Method::PUT, "inventory/settings"),
            ("inventory_units", Method::GET, "inventory/units"),
            ("inventory_create_unit", Method::POST, "inventory/units"),
            ("inventory_categories", Method::GET, "inventory/categories"),
            (
                "inventory_create_category",
                Method::POST,
                "inventory/categories",
            ),
            ("inventory_items", Method::GET, "inventory/items"),
            ("inventory_create_item", Method::POST, "inventory/items"),
            ("inventory_item_code", Method::GET, "inventory/item-codes"),
            ("inventory_warehouses", Method::GET, "inventory/warehouses"),
            (
                "inventory_create_warehouse",
                Method::POST,
                "inventory/warehouses",
            ),
            (
                "inventory_station_warehouses",
                Method::GET,
                "inventory/station-warehouses",
            ),
            ("inventory_stock", Method::GET, "inventory/stock"),
            ("inventory_movements", Method::GET, "inventory/movements"),
            (
                "inventory_record_initial_balances",
                Method::POST,
                "inventory/initial-balances",
            ),
        ] {
            assert_eq!(route(operation, None).unwrap(), (method, path.to_string()));
        }
        for (operation, method, path) in [
            (
                "inventory_save_unit",
                Method::PATCH,
                format!("inventory/units/{identifier}"),
            ),
            (
                "inventory_save_category",
                Method::PATCH,
                format!("inventory/categories/{identifier}"),
            ),
            (
                "inventory_item",
                Method::GET,
                format!("inventory/items/{identifier}"),
            ),
            (
                "inventory_save_item",
                Method::PATCH,
                format!("inventory/items/{identifier}"),
            ),
            (
                "inventory_create_presentation",
                Method::POST,
                format!("inventory/items/{identifier}/presentations"),
            ),
            (
                "inventory_save_presentation",
                Method::PATCH,
                format!("inventory/presentations/{identifier}"),
            ),
            (
                "inventory_save_warehouse",
                Method::PATCH,
                format!("inventory/warehouses/{identifier}"),
            ),
            (
                "inventory_assign_station_warehouse",
                Method::PUT,
                format!("inventory/station-warehouses/{identifier}"),
            ),
        ] {
            assert_eq!(route(operation, Some(identifier)).unwrap(), (method, path));
            assert!(route(operation, None).is_err());
            assert!(route(operation, Some("../../settings")).is_err());
        }
        // No hay borrado ni edición directa de existencias o movimientos: solo el libro las cambia.
        assert!(route("inventory_delete_item", Some(identifier)).is_err());
        assert!(route("inventory_save_stock", Some(identifier)).is_err());
        assert!(route("inventory_save_movement", Some(identifier)).is_err());
    }

    #[test]
    fn inventory_recipe_and_consumption_routes_are_closed() {
        let identifier = "00000000-0000-4000-8000-000000000001";
        for (operation, method, path) in [
            ("inventory_recipes", Method::GET, "inventory/recipes"),
            ("inventory_create_recipe", Method::POST, "inventory/recipes"),
            (
                "inventory_recipe_targets",
                Method::GET,
                "inventory/recipe-targets",
            ),
            (
                "inventory_ingredient_portions",
                Method::GET,
                "inventory/ingredient-portions",
            ),
            (
                "inventory_cancellations",
                Method::GET,
                "inventory/cancellations",
            ),
            (
                "inventory_productions",
                Method::GET,
                "inventory/productions",
            ),
            (
                "inventory_record_production",
                Method::POST,
                "inventory/productions",
            ),
            (
                "inventory_consumption_requests",
                Method::GET,
                "inventory/consumption-requests",
            ),
        ] {
            assert_eq!(route(operation, None).unwrap(), (method, path.to_string()));
        }
        for (operation, method, path) in [
            (
                "inventory_recipe",
                Method::GET,
                format!("inventory/recipes/{identifier}"),
            ),
            (
                "inventory_save_recipe",
                Method::PUT,
                format!("inventory/recipes/{identifier}"),
            ),
            (
                "inventory_recipe_version",
                Method::GET,
                format!("inventory/recipe-versions/{identifier}"),
            ),
            (
                "inventory_save_ingredient_portion",
                Method::PUT,
                format!("inventory/ingredient-portions/{identifier}"),
            ),
            (
                "inventory_classify_cancellation",
                Method::POST,
                format!("inventory/cancellations/{identifier}/classify"),
            ),
            (
                "inventory_unit_consumption",
                Method::GET,
                format!("inventory/consumption/{identifier}"),
            ),
        ] {
            assert_eq!(route(operation, Some(identifier)).unwrap(), (method, path));
            assert!(route(operation, None).is_err());
            assert!(route(operation, Some("../../settings")).is_err());
        }
        // Las recetas no se borran (se desactivan con una versión nueva) y el consumo no se escribe
        // desde la interfaz: lo calcula el trabajador de Go.
        assert!(route("inventory_delete_recipe", Some(identifier)).is_err());
        assert!(route("inventory_record_consumption", None).is_err());
        assert!(route("inventory_process_consumption", None).is_err());
        let mut query = BTreeMap::new();
        query.insert("owner_kind".to_string(), "preparation".to_string());
        query.insert("configured".to_string(), "true".to_string());
        query.insert("state".to_string(), "pending".to_string());
        assert!(query_allowed("inventory_recipes", &query));
        let mut rejected = BTreeMap::new();
        rejected.insert("include_costs".to_string(), "true".to_string());
        assert!(!query_allowed("inventory_recipes", &rejected));
    }

    #[test]
    fn inventory_purchasing_routes_are_closed() {
        let identifier = "00000000-0000-4000-8000-000000000001";
        for (operation, method, path) in [
            ("inventory_suppliers", Method::GET, "inventory/suppliers"),
            (
                "inventory_create_supplier",
                Method::POST,
                "inventory/suppliers",
            ),
            (
                "inventory_purchase_orders",
                Method::GET,
                "inventory/purchase-orders",
            ),
            (
                "inventory_create_purchase_order",
                Method::POST,
                "inventory/purchase-orders",
            ),
            ("inventory_receipts", Method::GET, "inventory/receipts"),
            (
                "inventory_record_receipt",
                Method::POST,
                "inventory/receipts",
            ),
            (
                "inventory_supplier_returns",
                Method::GET,
                "inventory/supplier-returns",
            ),
            (
                "inventory_record_supplier_return",
                Method::POST,
                "inventory/supplier-returns",
            ),
            (
                "inventory_cash_expenses",
                Method::GET,
                "inventory/cash-expenses",
            ),
        ] {
            assert_eq!(route(operation, None).unwrap(), (method, path.to_string()));
        }
        for (operation, method, path) in [
            (
                "inventory_supplier",
                Method::GET,
                format!("inventory/suppliers/{identifier}"),
            ),
            (
                "inventory_save_supplier",
                Method::PATCH,
                format!("inventory/suppliers/{identifier}"),
            ),
            (
                "inventory_save_supplier_item",
                Method::PUT,
                format!("inventory/suppliers/{identifier}/items"),
            ),
            (
                "inventory_remove_supplier_item",
                Method::POST,
                format!("inventory/suppliers/{identifier}/items/remove"),
            ),
            (
                "inventory_purchase_order",
                Method::GET,
                format!("inventory/purchase-orders/{identifier}"),
            ),
            (
                "inventory_save_purchase_order",
                Method::PATCH,
                format!("inventory/purchase-orders/{identifier}"),
            ),
            (
                "inventory_issue_purchase_order",
                Method::POST,
                format!("inventory/purchase-orders/{identifier}/issue"),
            ),
            (
                "inventory_cancel_purchase_order",
                Method::POST,
                format!("inventory/purchase-orders/{identifier}/cancel"),
            ),
            (
                "inventory_close_purchase_order",
                Method::POST,
                format!("inventory/purchase-orders/{identifier}/close"),
            ),
            (
                "inventory_receipt",
                Method::GET,
                format!("inventory/receipts/{identifier}"),
            ),
            (
                "inventory_link_cash_expense",
                Method::POST,
                format!("inventory/receipts/{identifier}/cash-links"),
            ),
            (
                "inventory_unlink_cash_expense",
                Method::POST,
                format!("inventory/cash-links/{identifier}/unlink"),
            ),
            (
                "inventory_supplier_return",
                Method::GET,
                format!("inventory/supplier-returns/{identifier}"),
            ),
        ] {
            assert_eq!(route(operation, Some(identifier)).unwrap(), (method, path));
            assert!(route(operation, None).is_err());
            assert!(route(operation, Some("../../settings")).is_err());
        }
        // Los proveedores no se borran (se desactivan) y las recepciones, devoluciones y vínculos no
        // se editan ni se borran: se registran documentos nuevos o se desvincula con motivo. Ninguna
        // ruta crea gastos de caja.
        assert!(route("inventory_delete_supplier", Some(identifier)).is_err());
        assert!(route("inventory_save_receipt", Some(identifier)).is_err());
        assert!(route("inventory_delete_receipt", Some(identifier)).is_err());
        assert!(route("inventory_save_supplier_return", Some(identifier)).is_err());
        assert!(route("inventory_delete_cash_link", Some(identifier)).is_err());
        assert!(route("inventory_record_cash_expense", None).is_err());
        let mut query = BTreeMap::new();
        query.insert("supplier_id".to_string(), "x".to_string());
        query.insert("purchase_order_id".to_string(), "x".to_string());
        query.insert("direct".to_string(), "true".to_string());
        // Etapa 9A: modalidad del proveedor (compras de mostrador).
        query.insert("supplier_kind".to_string(), "counter".to_string());
        assert!(query_allowed("inventory_receipts", &query));
        // Etapa 9A: comprobación del código interno de un insumo, sin parámetros ajenos.
        let mut code = BTreeMap::new();
        code.insert("code".to_string(), "harina".to_string());
        code.insert("item_id".to_string(), "x".to_string());
        assert!(query_allowed("inventory_item_code", &code));
        assert!(!query_allowed("catalog_products", &code));
        let mut expenses = BTreeMap::new();
        expenses.insert("min_amount".to_string(), "850".to_string());
        expenses.insert("max_amount".to_string(), "850".to_string());
        expenses.insert("register_id".to_string(), "x".to_string());
        expenses.insert("receipt_id".to_string(), "x".to_string());
        assert!(query_allowed("inventory_cash_expenses", &expenses));
        let mut rejected = BTreeMap::new();
        rejected.insert("shift_state".to_string(), "closed".to_string());
        assert!(!query_allowed("inventory_cash_expenses", &rejected));
        // Los parámetros de compras no se admiten fuera de inventario.
        assert!(!query_allowed("payments_shifts", &expenses));
    }

    #[test]
    fn inventory_control_routes_are_closed() {
        let identifier = "00000000-0000-4000-8000-000000000001";
        for (operation, method, path) in [
            (
                "inventory_adjustments",
                Method::GET,
                "inventory/adjustments",
            ),
            (
                "inventory_record_adjustment",
                Method::POST,
                "inventory/adjustments",
            ),
            ("inventory_transfers", Method::GET, "inventory/transfers"),
            (
                "inventory_record_transfer",
                Method::POST,
                "inventory/transfers",
            ),
            ("inventory_counts", Method::GET, "inventory/counts"),
            ("inventory_create_count", Method::POST, "inventory/counts"),
            (
                "inventory_stock_levels",
                Method::GET,
                "inventory/stock-levels",
            ),
            (
                "inventory_save_stock_level",
                Method::PUT,
                "inventory/stock-levels",
            ),
            ("inventory_alerts", Method::GET, "inventory/alerts"),
            (
                "inventory_purchase_suggestions",
                Method::GET,
                "inventory/purchase-suggestions",
            ),
            (
                "inventory_draft_order_from_suggestions",
                Method::POST,
                "inventory/purchase-suggestions/draft-order",
            ),
        ] {
            assert_eq!(route(operation, None).unwrap(), (method, path.to_string()));
        }
        for (operation, method, path) in [
            (
                "inventory_adjustment",
                Method::GET,
                format!("inventory/adjustments/{identifier}"),
            ),
            (
                "inventory_authorize_adjustment",
                Method::POST,
                format!("inventory/adjustments/{identifier}/authorize"),
            ),
            (
                "inventory_reject_adjustment",
                Method::POST,
                format!("inventory/adjustments/{identifier}/reject"),
            ),
            (
                "inventory_transfer",
                Method::GET,
                format!("inventory/transfers/{identifier}"),
            ),
            (
                "inventory_count",
                Method::GET,
                format!("inventory/counts/{identifier}"),
            ),
            (
                "inventory_save_count",
                Method::PATCH,
                format!("inventory/counts/{identifier}"),
            ),
            (
                "inventory_capture_count_lines",
                Method::PUT,
                format!("inventory/counts/{identifier}/lines"),
            ),
            (
                "inventory_submit_count",
                Method::POST,
                format!("inventory/counts/{identifier}/submit"),
            ),
            (
                "inventory_approve_count",
                Method::POST,
                format!("inventory/counts/{identifier}/approve"),
            ),
            (
                "inventory_apply_count",
                Method::POST,
                format!("inventory/counts/{identifier}/apply"),
            ),
            (
                "inventory_cancel_count",
                Method::POST,
                format!("inventory/counts/{identifier}/cancel"),
            ),
        ] {
            assert_eq!(route(operation, Some(identifier)).unwrap(), (method, path));
            assert!(route(operation, None).is_err());
            assert!(route(operation, Some("../../settings")).is_err());
        }
        // Los ajustes, transferencias y conteos no se editan ni se borran (se autorizan, rechazan,
        // cancelan o corrigen con documentos nuevos), ninguna ruta edita existencias ni emite órdenes
        // desde las sugerencias.
        assert!(route("inventory_save_adjustment", Some(identifier)).is_err());
        assert!(route("inventory_delete_adjustment", Some(identifier)).is_err());
        assert!(route("inventory_save_transfer", Some(identifier)).is_err());
        assert!(route("inventory_delete_count", Some(identifier)).is_err());
        assert!(route("inventory_save_stock", None).is_err());
        assert!(route("inventory_issue_order_from_suggestions", None).is_err());
        let mut alerts = BTreeMap::new();
        alerts.insert("type".to_string(), "negative_stock".to_string());
        alerts.insert("severity".to_string(), "critical".to_string());
        assert!(query_allowed("inventory_alerts", &alerts));
        let mut adjustments = BTreeMap::new();
        adjustments.insert("reason".to_string(), "breakage".to_string());
        adjustments.insert("kind".to_string(), "waste".to_string());
        adjustments.insert("state".to_string(), "pending_authorization".to_string());
        assert!(query_allowed("inventory_adjustments", &adjustments));
        let mut rejected = BTreeMap::new();
        rejected.insert("auto_issue".to_string(), "true".to_string());
        assert!(!query_allowed("inventory_purchase_suggestions", &rejected));
        // Los parámetros de alertas no se admiten fuera de inventario.
        assert!(!query_allowed("payments_shifts", &alerts));
    }

    #[test]
    fn inventory_query_parameters_are_whitelisted() {
        let mut query = BTreeMap::new();
        query.insert("warehouse_id".to_string(), "x".to_string());
        query.insert("search".to_string(), "arroz".to_string());
        query.insert("page".to_string(), "2".to_string());
        assert!(query_allowed("inventory_stock", &query));
        query.insert("from".to_string(), "2026-11-01".to_string());
        assert!(query_allowed("inventory_movements", &query));
        let mut rejected = BTreeMap::new();
        rejected.insert("order_by".to_string(), "value".to_string());
        assert!(!query_allowed("inventory_movements", &rejected));
        let mut costs = BTreeMap::new();
        costs.insert("include_costs".to_string(), "true".to_string());
        assert!(!query_allowed("inventory_stock", &costs));
        // Los parámetros de inventario no se admiten en otras operaciones.
        let mut foreign = BTreeMap::new();
        foreign.insert("warehouse_id".to_string(), "x".to_string());
        assert!(!query_allowed("payments_shifts", &foreign));
    }

    #[test]
    fn document_identity_routes_are_closed_and_keep_write_methods() {
        for (operation, method, path) in [
            ("documents_identity", Method::GET, "documents/identity"),
            ("documents_save_identity", Method::PUT, "documents/identity"),
            (
                "documents_upload_identity_logo",
                Method::POST,
                "documents/identity/logo",
            ),
            (
                "documents_remove_identity_logo",
                Method::POST,
                "documents/identity/logo/remove",
            ),
            (
                "documents_preview_identity_logo",
                Method::POST,
                "documents/identity/logo/preview",
            ),
        ] {
            assert_eq!(route(operation, None).unwrap(), (method, path.to_string()));
            // La identidad es única: un identificador no cambia la ruta.
            assert_eq!(
                route(operation, Some("../../etc/passwd")).unwrap().1,
                path.to_string()
            );
        }
        // Solo esas cinco operaciones: los rasters y los originales no tienen ruta propia.
        assert!(route("documents_identity_logo", None).is_err());
        assert!(route("documents_identity_raster", None).is_err());
        assert!(route("documents_delete_identity_logo", None).is_err());
    }

    #[test]
    fn durable_credentials_never_reach_the_webview() {
        let mut response = serde_json::json!({
            "device_id": "public-id", "device_token": "test-device-secret", "session_token": "test-session-secret",
        });
        sanitize_response(&mut response);
        assert!(response.get("device_token").is_none());
        assert!(response.get("session_token").is_none());
        assert!(response.get("device_id").is_some());
    }

    #[test]
    fn only_session_or_device_errors_discard_credentials() {
        let password_error = serde_json::json!({"error": {"code": "INVALID_CREDENTIALS"}});
        let session_error = serde_json::json!({"error": {"code": "SESSION_REPLACED"}});
        let device_error = serde_json::json!({"error": {"code": "DEVICE_REVOKED"}});
        assert!(!invalidates_credentials(401, &password_error));
        assert!(invalidates_credentials(401, &session_error));
        assert!(invalidates_credentials(401, &device_error));
    }
}
