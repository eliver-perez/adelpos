use keyring::Entry;
use serde::{Deserialize, Serialize};
use std::sync::Mutex;

#[derive(Clone, Default, Deserialize, Serialize)]
pub struct Credentials {
    pub device_id: Option<String>,
    pub device_token: Option<String>,
    pub session_token: Option<String>,
}

pub struct Vault {
    entry: Entry,
    credentials: Mutex<Credentials>,
}

impl Vault {
    pub fn open(server: &str, namespace: &str) -> Result<Self, String> {
        let account = format!("{server}|{namespace}");
        let metadata: serde_json::Value =
            serde_json::from_str(include_str!("../../src/generated/product.json"))
                .map_err(|_| "Los metadatos de la aplicación no son válidos.")?;
        let service = metadata
            .get("bundle_id")
            .and_then(serde_json::Value::as_str)
            .ok_or("Falta la identidad del almacén seguro.")?;
        let entry = Entry::new(service, &account)
            .map_err(|_| "No se pudo abrir el almacén seguro del sistema.")?;
        let credentials = match entry.get_password() {
            Ok(value) => serde_json::from_str(&value).map_err(|_| {
                "La credencial local necesita revisión; no se reemplazará silenciosamente."
            })?,
            Err(keyring::Error::NoEntry) => Credentials::default(),
            Err(_) => {
                return Err("El almacén seguro no está disponible o se denegó su acceso.".into())
            }
        };
        Ok(Self {
            entry,
            credentials: Mutex::new(credentials),
        })
    }

    pub fn read(&self) -> Result<Credentials, String> {
        self.credentials
            .lock()
            .map(|value| value.clone())
            .map_err(|_| "No se pudo consultar la credencial local.".into())
    }

    pub fn save(&self, value: Credentials) -> Result<(), String> {
        let encoded = serde_json::to_string(&value)
            .map_err(|_| "No se pudo preparar la credencial local.")?;
        // Nunca recurrir a archivos o localStorage si el almacén seguro falla.
        self.entry
            .set_password(&encoded)
            .map_err(|_| "No se pudo guardar la credencial en el almacén seguro del sistema.")?;
        *self
            .credentials
            .lock()
            .map_err(|_| "No se pudo actualizar la credencial local.")? = value;
        Ok(())
    }
}

#[cfg(all(test, target_os = "macos"))]
mod tests {
    use super::*;
    use std::time::{SystemTime, UNIX_EPOCH};

    #[test]
    #[ignore = "Escribe y elimina una credencial temporal en el Keychain real; ejecutar explícitamente."]
    fn real_system_vault_roundtrip() {
        let nonce = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let namespace = format!("acceptance-test-{nonce}");
        let vault = Vault::open("http://127.0.0.1:8181", &namespace).unwrap();
        struct Cleanup<'a>(&'a Entry);
        impl Drop for Cleanup<'_> {
            fn drop(&mut self) {
                let _ = self.0.delete_credential();
            }
        }
        let _cleanup = Cleanup(&vault.entry);
        let value = Credentials {
            device_id: Some("temporary-test-device".into()),
            device_token: Some(format!("temporary-device-{nonce}")),
            session_token: Some(format!("temporary-session-{nonce}")),
        };
        vault.save(value.clone()).unwrap();
        let reopened = Vault::open("http://127.0.0.1:8181", &namespace).unwrap();
        let restored = reopened.read().unwrap();
        assert!(restored.device_id == value.device_id);
        assert!(restored.device_token == value.device_token);
        assert!(restored.session_token == value.session_token);
    }
}
