use crate::{HTTPResult, TerminalState};
use reqwest::Method;
use serde::Serialize;
use serde_json::Value;
use std::collections::BTreeMap;
use tauri::State;

fn resource_id(id: Option<&str>) -> Result<&str, String> {
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

fn route(operation: &str, id: Option<&str>) -> Result<(Method, String), String> {
    let fixed = match operation {
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
        "registers" => Some((Method::GET, "registers")),
        "create_register" => Some((Method::POST, "registers")),
        "devices" => Some((Method::GET, "devices")),
        "invite_device" => Some((Method::POST, "devices/invitations")),
        "sessions" => Some((Method::GET, "sessions")),
        "audit" => Some((Method::GET, "audit")),
        "preferences" => Some((Method::GET, "preferences")),
        "save_preferences" => Some((Method::PUT, "preferences")),
        "save_terminal_preferences" => Some((Method::PUT, "terminal/preferences")),
        _ => None,
    };
    if let Some((method, path)) = fixed {
        return Ok((method, path.to_string()));
    }
    let identifier = resource_id(id)?;
    let (method, path) = match operation {
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
    let vault = state.vault.as_ref().map_err(Clone::clone)?;
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
    let vault = state.vault.as_ref().map_err(Clone::clone)?;
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
        if operation != "audit"
            || query.keys().any(|key| {
                ![
                    "page",
                    "page_size",
                    "actor",
                    "action",
                    "device_id",
                    "from",
                    "until",
                ]
                .contains(&key.as_str())
            })
        {
            return Err("Parámetros de consulta no autorizados.".into());
        }
        request = request.query(&query);
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

fn sanitize_response(result: &mut Value) {
    // Los secretos duraderos nunca cruzan al WebView ni forman parte del estado React.
    if let Some(object) = result.as_object_mut() {
        object.remove("session_token");
        object.remove("device_token");
    }
}

fn invalidates_credentials(status: u16, result: &Value) -> bool {
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
    fn rejects_arbitrary_routes_and_identifiers() {
        assert!(route("https://attacker.test", None).is_err());
        assert!(route("update_user", Some("../../settings")).is_err());
        assert!(route("me", None).is_ok());
        assert!(route("update_user", Some("00000000-0000-0000-0000-000000000001")).is_ok());
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
