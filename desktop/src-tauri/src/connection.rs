use serde::{Deserialize, Serialize};
use std::{fs, net::IpAddr, path::PathBuf};

/// Destino y modalidad de esta terminal. Desde la etapa 9G (fase A) la modalidad es una de las tres de
/// §2.3 (`server-terminal`, `server-only` y `terminal-only`) y la escribe solo Rust al elegirla
/// (installation.rs). Una terminal «Solo terminal» que todavía no se conecta a un servidor guarda la
/// dirección vacía: no se hace ninguna petición hasta que se empareje (fases B y C).
#[derive(Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct ConnectionConfig {
    pub installation_mode: String,
    pub server_url: String,
    #[serde(default = "default_namespace")]
    pub credential_namespace: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub ca_certificate: Option<String>,
    /// Etapa 9G (fase B): terminal emparejada con la computadora principal. Solo Rust escribe estos campos
    /// (pairing.rs) cuando el emparejamiento se aprueba: el identificador del servidor, la huella SPKI
    /// fijada (hexadecimal, 64 caracteres) y su nombre; `server_url` es la última dirección conocida.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub server_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub server_fingerprint: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub server_name: Option<String>,
}

impl ConnectionConfig {
    /// Una terminal «Solo terminal» sin servidor todavía.
    pub fn awaiting_server(&self) -> bool {
        self.server_url.is_empty()
    }

    /// Terminal emparejada con un servidor (huella fijada).
    pub fn paired(&self) -> bool {
        self.server_id.is_some()
    }

    /// Servidor de la llave del almacén seguro: el identificador de un servidor emparejado (sirve aunque
    /// cambie su dirección) o, como hasta la etapa 9F, la dirección (`http://127.0.0.1:8181` en la
    /// computadora principal, cuya credencial no cambia).
    pub fn credential_server(&self) -> String {
        match &self.server_id {
            Some(server_id) => crate::pairing::credential_server(server_id),
            None => self.server_url.clone(),
        }
    }
}

fn default_namespace() -> String {
    "main".to_string()
}

pub fn parse_config(text: &str) -> Result<ConnectionConfig, String> {
    let mut config: ConnectionConfig = serde_json::from_str(text)
        .map_err(|_| "El archivo de conexión no tiene un formato válido.".to_string())?;
    if !matches!(
        config.installation_mode.as_str(),
        "server-terminal" | "server-only" | "terminal-only"
    ) {
        return Err("El escritorio requiere server-terminal, server-only o terminal-only.".into());
    }
    if config.credential_namespace.is_empty() || config.credential_namespace.len() > 80 {
        return Err("El perfil de credenciales requiere entre 1 y 80 caracteres.".into());
    }
    let pairing_fields = config.server_id.is_some()
        || config.server_fingerprint.is_some()
        || config.server_name.is_some();
    // Solo una terminal que todavía no se conecta a un servidor puede no tener dirección.
    if config.server_url.is_empty() {
        if config.installation_mode != "terminal-only"
            || config.ca_certificate.is_some()
            || pairing_fields
        {
            return Err("La dirección del servidor es obligatoria.".into());
        }
        return Ok(config);
    }
    if pairing_fields {
        return check_paired(config);
    }
    let url =
        reqwest::Url::parse(&config.server_url).map_err(|_| "Dirección del servidor inválida.")?;
    if !matches!(url.scheme(), "http" | "https")
        || !url.username().is_empty()
        || url.password().is_some()
        || url.query().is_some()
        || url.fragment().is_some()
        || url.path() != "/"
    {
        return Err("Usa una dirección HTTP(S) sin credenciales, rutas ni parámetros.".into());
    }
    let host = url.host_str().ok_or("La dirección requiere un host.")?;
    // Las credenciales solo viajan sin TLS a loopback; la LAN exige confianza verificable.
    if host != "localhost" {
        let ip: IpAddr = host
            .trim_matches(['[', ']'])
            .parse()
            .map_err(|_| "HTTP requiere localhost o una IP privada explícita.")?;
        let private = match ip {
            IpAddr::V4(v) => v.is_private() || v.is_loopback(),
            IpAddr::V6(v) => v.is_loopback() || v.is_unique_local(),
        };
        if !private {
            return Err("El servidor debe estar en loopback o en una IP privada explícita.".into());
        }
        if url.scheme() == "http" && !ip.is_loopback() {
            return Err("Las conexiones LAN requieren HTTPS y un certificado verificado.".into());
        }
    }
    config.server_url = url.as_str().trim_end_matches('/').to_string();
    Ok(config)
}

/// Reglas de una terminal emparejada (etapa 9G, D2 y D6): «Solo terminal», HTTPS a una IP privada o de
/// enlace local o a un nombre `.local`, identificador y huella válidos, sin CA (la confianza es la huella).
fn check_paired(mut config: ConnectionConfig) -> Result<ConnectionConfig, String> {
    let invalid =
        "La conexión con el servidor emparejado no es válida; vuelve a emparejar esta terminal.";
    if config.installation_mode != "terminal-only" || config.ca_certificate.is_some() {
        return Err(invalid.into());
    }
    let server_id = config.server_id.as_deref().unwrap_or("");
    let fingerprint = config.server_fingerprint.as_deref().unwrap_or("");
    if !crate::pairing::valid_identifier(server_id)
        || server_id.chars().any(|value| value.is_ascii_uppercase())
        || crate::pairing::fingerprint_from_hex(fingerprint).is_none()
    {
        return Err(invalid.into());
    }
    if let Some(name) = &config.server_name {
        if name.trim().is_empty()
            || name.chars().count() > 100
            || name.chars().any(char::is_control)
        {
            return Err(invalid.into());
        }
    }
    let url = reqwest::Url::parse(&config.server_url).map_err(|_| invalid.to_string())?;
    let host = url.host_str().unwrap_or("");
    if url.scheme() != "https"
        || !url.username().is_empty()
        || url.password().is_some()
        || url.query().is_some()
        || url.fragment().is_some()
        || url.path() != "/"
        || !crate::pairing::valid_server_host(host)
    {
        return Err(invalid.into());
    }
    config.server_url = url.as_str().trim_end_matches('/').to_string();
    Ok(config)
}

pub fn load_config(config_dir: Option<PathBuf>) -> Result<ConnectionConfig, String> {
    if let Some(filename) = std::env::var_os("ADEL_TERMINAL_CONFIG") {
        let text = fs::read_to_string(filename)
            .map_err(|_| "No se pudo leer ADEL_TERMINAL_CONFIG.".to_string())?;
        return parse_config(&text);
    }
    if let Some(directory) = config_dir {
        let filename = directory.join("connection.json");
        match fs::read_to_string(filename) {
            Ok(text) => return parse_config(&text),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
            Err(_) => return Err("No se pudo leer la configuración de terminal.".into()),
        }
    }
    parse_config(include_str!("../../src/generated/connection.json"))
}

#[cfg(test)]
mod tests {
    use super::*;
    fn config(url: &str) -> String {
        serde_json::json!({"installation_mode": "terminal-only", "server_url": url}).to_string()
    }
    #[test]
    fn validates_destinations() {
        for url in [
            "http://127.0.0.1:8181",
            "https://192.168.1.10:8181/",
            "http://[::1]:8181",
            "https://localhost",
        ] {
            assert!(parse_config(&config(url)).is_ok(), "{url}");
        }
        for url in [
            "http://8.8.8.8",
            "http://192.168.1.10",
            "https://example.test",
            "http://example.test",
            "file:///etc/passwd",
            "http://user:secret@localhost",
            "http://localhost/admin",
            "http://localhost?x=1",
        ] {
            assert!(parse_config(&config(url)).is_err(), "{url}");
        }
    }
    #[test]
    fn rejects_unknown_modes_and_fields() {
        assert!(
            parse_config(r#"{"installation_mode":"server","server_url":"http://localhost"}"#)
                .is_err()
        );
        assert!(parse_config(r#"{"installation_mode":"terminal-only","server_url":"http://localhost","password":"secret"}"#).is_err());
    }

    // Etapa 9G, fase B: la conexión de una terminal emparejada es una lista cerrada (D2 y D6).
    #[test]
    fn paired_terminals_need_https_on_the_local_network_and_a_valid_pin() {
        let fingerprint = "ab".repeat(32);
        let paired = |mode: &str, url: &str, id: &str, pin: &str| {
            serde_json::json!({
                "installation_mode": mode,
                "server_url": url,
                "server_id": id,
                "server_fingerprint": pin,
                "server_name": "Fonda Doña Ñeli",
            })
            .to_string()
        };
        let id = "11111111-2222-4333-8444-555555555555";
        for url in [
            "https://192.168.1.20:8443",
            "https://10.0.0.5:8443/",
            "https://caja-principal.local:8443",
            "https://[fd00::5]:8443",
            "https://169.254.1.2:8443",
        ] {
            let config = parse_config(&paired("terminal-only", url, id, &fingerprint)).unwrap();
            assert!(config.paired(), "{url}");
            assert_eq!(config.credential_server(), format!("adel-server:{id}"));
        }
        for (mode, url, server, pin) in [
            (
                "server-terminal",
                "https://192.168.1.20:8443",
                id,
                fingerprint.as_str(),
            ),
            (
                "terminal-only",
                "http://192.168.1.20:8443",
                id,
                fingerprint.as_str(),
            ),
            (
                "terminal-only",
                "https://127.0.0.1:8443",
                id,
                fingerprint.as_str(),
            ),
            (
                "terminal-only",
                "https://8.8.8.8:8443",
                id,
                fingerprint.as_str(),
            ),
            (
                "terminal-only",
                "https://servidor.example:8443",
                id,
                fingerprint.as_str(),
            ),
            (
                "terminal-only",
                "https://192.168.1.20:8443/admin",
                id,
                fingerprint.as_str(),
            ),
            (
                "terminal-only",
                "https://192.168.1.20:8443",
                "no-es-uuid",
                fingerprint.as_str(),
            ),
            ("terminal-only", "https://192.168.1.20:8443", id, "corta"),
            (
                "terminal-only",
                "https://192.168.1.20:8443",
                id,
                &"AB".repeat(32),
            ),
            ("terminal-only", "", id, fingerprint.as_str()),
        ] {
            assert!(
                parse_config(&paired(mode, url, server, pin)).is_err(),
                "{mode} {url} {server} {pin}"
            );
        }
        let with_ca = serde_json::json!({
            "installation_mode": "terminal-only", "server_url": "https://192.168.1.20:8443",
            "server_id": id, "server_fingerprint": fingerprint, "ca_certificate": "/tmp/ca.pem",
        });
        assert!(parse_config(&with_ca.to_string()).is_err());
        // La credencial local de la computadora principal conserva su llave de siempre.
        let local = parse_config(
            r#"{"installation_mode":"server-terminal","server_url":"http://127.0.0.1:8181"}"#,
        )
        .unwrap();
        assert!(!local.paired());
        assert_eq!(local.credential_server(), "http://127.0.0.1:8181");
    }

    // Etapa 9G, fase A: las tres modalidades; la dirección vacía solo para una terminal sin servidor.
    #[test]
    fn accepts_the_three_modes_and_an_unpaired_terminal() {
        for mode in ["server-terminal", "server-only", "terminal-only"] {
            let text = serde_json::json!({"installation_mode": mode, "server_url": "http://127.0.0.1:8181"});
            let config = parse_config(&text.to_string()).unwrap();
            assert_eq!(config.installation_mode, mode);
            assert!(!config.awaiting_server());
        }
        let unpaired =
            parse_config(r#"{"installation_mode":"terminal-only","server_url":""}"#).unwrap();
        assert!(unpaired.awaiting_server());
        assert_eq!(unpaired.credential_namespace, "main");
        for text in [
            r#"{"installation_mode":"server-terminal","server_url":""}"#,
            r#"{"installation_mode":"server-only","server_url":""}"#,
            r#"{"installation_mode":"terminal-only","server_url":"","ca_certificate":"/tmp/ca.pem"}"#,
            r#"{"installation_mode":"terminal-only","server_url":"","credential_namespace":""}"#,
        ] {
            assert!(parse_config(text).is_err(), "{text}");
        }
    }
}
