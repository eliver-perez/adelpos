use crate::TerminalState;
use serde_json::{json, Value};
use std::sync::atomic::Ordering;
use std::time::Duration;
use tauri::{ipc::Channel, State};

fn parse_frame(frame: &[u8]) -> Result<Option<Value>, String> {
    let text = std::str::from_utf8(frame).map_err(|_| "Evento con codificación inválida.")?;
    let data = text
        .lines()
        .filter_map(|line| line.strip_prefix("data: "))
        .collect::<Vec<_>>()
        .join("\n");
    if data.is_empty() {
        return Ok(None);
    }
    serde_json::from_str(&data)
        .map(Some)
        .map_err(|_| "Evento con contrato inválido.".into())
}

#[tauri::command]
pub fn events_stop(state: State<'_, TerminalState>, subscription_id: u64) {
    // Una limpieza atrasada de React no puede cerrar la suscripción de la siguiente vista.
    let _ = state.events_generation.compare_exchange(
        subscription_id,
        subscription_id + 1,
        Ordering::SeqCst,
        Ordering::SeqCst,
    );
}

#[tauri::command]
pub fn events_start(
    state: State<'_, TerminalState>,
    channel: Channel<Value>,
    after: Option<i64>,
    station_id: Option<String>,
) -> Result<u64, String> {
    if after.unwrap_or(0) < 0 {
        return Err("Cursor no válido.".into());
    }
    if let Some(identifier) = station_id.as_deref() {
        crate::api::resource_id(Some(identifier))?;
    }
    state.vault()?;
    state.config.as_ref().map_err(Clone::clone)?;
    let client = state.client.as_ref().map_err(Clone::clone)?.clone();
    // Etapa 9G (fase C): la dirección se lee en cada intento; si el servidor emparejado cambió de IP, el canal
    // lo vuelve a encontrar (discovery.rs) y sigue desde el mismo cursor.
    let locator = state.locator.clone();
    let vault = state.vault.clone();
    let generation = state.events_generation.clone();
    let subscription_id = generation.fetch_add(1, Ordering::SeqCst) + 1;
    tauri::async_runtime::spawn(async move {
        let mut cursor = after.unwrap_or(0);
        let mut delay = 500;
        while generation.load(Ordering::SeqCst) == subscription_id {
            let Some(Ok(storage)) = vault.get() else {
                break;
            };
            let Ok(credentials) = storage.read() else {
                break;
            };
            let (Some(device), Some(secret), Some(session)) = (
                credentials.device_id,
                credentials.device_token,
                credentials.session_token,
            ) else {
                break;
            };
            // El destino y las credenciales se obtienen únicamente del perfil nativo.
            let base = locator.base();
            let mut request =
                crate::api::with_client_headers(client.get(format!("{base}/api/v1/events")))
                    .header("X-Device-ID", device)
                    .header("X-Device-Token", secret)
                    .bearer_auth(session)
                    .header("Accept", "text/event-stream")
                    .query(&[("after", cursor.to_string())])
                    .timeout(Duration::from_secs(40));
            if let Some(station) = &station_id {
                request = request.query(&[("station_id", station)]);
            }
            let sent = request.send().await;
            if let Err(error) = &sent {
                if error.is_connect() {
                    let _ = locator.relocate(&base).await;
                }
            }
            if let Ok(mut response) = sent {
                if response.status().as_u16() == 401 || response.status().as_u16() == 403 {
                    let _ = channel.send(
                        json!({"connection":"unauthorized","status":response.status().as_u16()}),
                    );
                    break;
                }
                if response.status().is_success() {
                    delay = 500;
                    if channel.send(json!({"connection":"connected"})).is_err() {
                        break;
                    }
                    let mut buffer = Vec::new();
                    while generation.load(Ordering::SeqCst) == subscription_id {
                        let Ok(Some(chunk)) = response.chunk().await else {
                            break;
                        };
                        if buffer.len() + chunk.len() > 262144 {
                            break;
                        }
                        buffer.extend_from_slice(&chunk);
                        while let Some(end) = buffer.windows(2).position(|pair| pair == b"\n\n") {
                            let frame = buffer.drain(..end + 2).collect::<Vec<_>>();
                            if let Ok(Some(message)) = parse_frame(&frame) {
                                if let Some(next) = message.get("cursor").and_then(Value::as_i64) {
                                    cursor = next;
                                }
                                if channel.send(message).is_err() {
                                    return;
                                }
                            }
                        }
                    }
                }
            }
            if generation.load(Ordering::SeqCst) != subscription_id {
                break;
            }
            if channel.send(json!({"connection":"reconnecting"})).is_err() {
                break;
            }
            tokio::time::sleep(Duration::from_millis(delay)).await;
            delay = (delay * 2).min(15000);
        }
    });
    Ok(subscription_id)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_utf8_replay_and_ignores_heartbeat() {
        let message = parse_frame("id: 5\nevent: changes\ndata: {\"cursor\":5,\"events\":[{\"kind\":\"preparación\"}]}\n\n".as_bytes()).unwrap().unwrap();
        assert_eq!(message["cursor"], 5);
        assert!(parse_frame(b": keepalive\n\n").unwrap().is_none());
        assert!(parse_frame(b"data: invalid\n\n").is_err());
    }
}
