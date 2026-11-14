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
}

impl ConnectionConfig {
    /// Una terminal «Solo terminal» sin servidor todavía.
    pub fn awaiting_server(&self) -> bool {
        self.server_url.is_empty()
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
    // Solo una terminal que todavía no se conecta a un servidor puede no tener dirección.
    if config.server_url.is_empty() {
        if config.installation_mode != "terminal-only" || config.ca_certificate.is_some() {
            return Err("La dirección del servidor es obligatoria.".into());
        }
        return Ok(config);
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
