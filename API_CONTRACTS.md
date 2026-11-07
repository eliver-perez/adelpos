# Contratos API

Base implementada: `http://127.0.0.1:8181/api/v1`. Solo diagnósticos públicos de información no sensible; ninguna función administrativa remota. Las escrituras futuras requerirán identidad y autorización reales.

## Rutas implementadas

| Método | Ruta | Respuesta |
| --- | --- | --- |
| GET | `/api/v1/health/live` | `200 {"status":"alive"}`; no consulta DB. |
| GET | `/api/v1/health/ready` | `200` si PostgreSQL y esquema están disponibles; `503` en caso contrario. |
| GET | `/api/v1/version` | Identidad técnica, versión, API, esquema, capacidades no sensibles. |
| GET | `/api/v1/events` | `501 EVENTS_NOT_AVAILABLE`; no realiza upgrade WebSocket. |

Disponibilidad real:

```json
{"status":"ready","checks":{"postgres":"available","schema":"compatible"}}
```

Sin DB: `status=not_ready`, `postgres=unavailable`, `schema=unknown`. DB conectada pero migraciones incompatibles: `postgres=available`, `schema=incompatible`, también `503`. Timeout del chequeo: 2 segundos. Tauri consulta cada 5 segundos, sin solapar consultas, y permite comprobación manual. Cada GET nativo tiene un timeout de 3 segundos y límite de respuesta de 32 KiB. La consulta de los tres endpoints no es una transacción: representa el estado observado durante ese intervalo.

Versión:

```json
{
  "product": "ad-el-pos",
  "server_version": "0.1.0",
  "api_version": "v1",
  "schema_version": 1,
  "supported_api_versions": ["v1"],
  "capabilities": ["technical-health"],
  "events_available": false
}
```

El cliente valida identidad técnica, API y esquema; no basta que una URL devuelva HTTP 200. Un servidor inaccesible deja PostgreSQL en estado desconocido, nunca en un estado disponible almacenado de una consulta anterior.

## Convenciones actuales

JSON UTF-8. `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, `X-Request-ID` generado por el servidor. Los errores usan:

```json
{"error":{"code":"NOT_FOUND","message":"Ruta no disponible.","request_id":"identificador-opaco"}}
```

Códigos: `NOT_FOUND` 404, `METHOD_NOT_ALLOWED` 405, `ORIGIN_NOT_ALLOWED` 403, `PREFLIGHT_REJECTED` 403, `EVENTS_NOT_AVAILABLE` 501, `INTERNAL_ERROR` 500. La disponibilidad negativa usa el documento `checks`, no un error genérico. Los métodos distintos de GET/OPTIONS se rechazan. OPTIONS permite GET desde un origen exacto y sin headers adicionales solicitados; ampliar CORS con autenticación futura de forma deliberada.

El transporte Tauri realiza GET con rutas fijas desde Rust; no acepta una URL, método o SQL arbitrarios desde React. Un cliente nativo no necesita CORS. El modo navegador sí se somete a la lista de orígenes. No hay comodines ni cookies/sesiones en esta entrega.

Versionado: cambios compatibles pueden añadir campos. Eliminaciones, cambios semánticos o tipos incompatibles requieren nueva versión de API. Los clientes deben ignorar campos nuevos y rechazar contratos esenciales ausentes/inválidos. `schema_version` sirve a diagnóstico; solo el servidor opera el esquema. El cliente no ejecuta migraciones.

## Diseñado para la siguiente etapa; no implementado

**Identidad y permisos.** Enrolamiento/revocación de dispositivos separado de sesión de usuario. Toda operación autoriza en Go el permiso correspondiente y el alcance del restaurante/terminal. Ocultar un botón no es autorización. El proveedor de licencias aporta derechos comerciales al límite de política central; no reemplaza permisos de usuario.

**Android offline e idempotencia.** El cliente asignará `operation_id` UUID antes de guardar un pedido en su cola durable. El servidor deberá persistir en una misma transacción la clave única `(device_id, operation_id)`, hash canónico de la solicitud, pedido y resultado confirmado. Un reintento con el mismo contenido devuelve el mismo ID/resultado; con diferente contenido devuelve conflicto. Nunca confirmar recepción antes del commit. Ante respuesta perdida se reintenta la misma operación, sin crear otra. Definir retención de claves, recuperación, estado consultable y reglas de conflicto antes de implementar sincronización. Esto no promete entrega exactamente una vez por la red: evita duplicar el efecto persistido.

**Eventos/KDS.** `events.Envelope` reserva `id`, `type`, `version`, `occurred_at`, `operation_id?` y `payload`. El futuro endpoint WebSocket requerirá autenticación del dispositivo/usuario, validación de Origin para navegadores, límites, heartbeat, backpressure y revocación. KDS recibirá eventos autorizados por estación y enviará cambios de estado por comandos idempotentes a Go. Evaluar outbox transaccional para publicar solo operaciones confirmadas, cursor de reanudación, deduplicación y resync de snapshot cuando el cursor expire. No se ha implementado ninguno de estos mecanismos.

**Concurrencia.** Reservar revisiones de agregado para optimistic locking, responder conflicto al modificar una revisión obsoleta y mantener invariantes transaccionales. Un pool limita conexiones, pero no equivale a un control de terminales ni de sesiones. No aceptar el reloj cliente como autoridad para ordenar cambios.

**Licencias.** Proyecto independiente. El adaptador central decidirá la política de arranque y derechos, con caché/verificación offline futura. Protocolo, firma, activación, periodos de gracia, licencias perpetuas/temporales y módulos están pendientes. No existe conexión al servicio comercial en desarrollo.
