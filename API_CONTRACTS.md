# API v1 · Etapa 3

HTTP JSON bajo `/api/v1`. Por defecto `http://127.0.0.1:8181`; LAN requiere HTTPS con identidad de servidor verificada.

## Autenticación de transporte

El puente nativo añade `X-Device-ID`, `X-Device-Token` y `Authorization: Bearer <sesión>`. Los tokens duraderos no se devuelven a React. Todos los endpoints protegidos comprueban dispositivo, sesión, usuario activo y permiso efectivo en PostgreSQL. Cambiar un identificador público no sustituye una credencial válida. Los ejemplos documentan nombres de cabeceras, nunca valores reales.

## Rutas

| Método     | Ruta                      | Requisito                                                                    |
| ---------- | ------------------------- | ---------------------------------------------------------------------------- |
| GET        | `/health/live`            | Público; proceso activo.                                                     |
| GET        | `/health/ready`           | Público; PostgreSQL y esquema compatibles, 200 o 503.                        |
| GET        | `/version`                | Público; producto, versión, API y esquema 3.                                 |
| GET        | `/events`                 | 501; transporte de eventos pendiente.                                        |
| GET        | `/setup/status`           | Público; solo configured booleano.                                           |
| POST       | `/setup`                  | Loopback y X-Provisioning-Token del propietario local.                       |
| POST       | `/devices/enroll`         | Código de autorización de un solo uso, no requiere sesión previa.            |
| POST       | `/auth/login`             | Dispositivo autorizado; username y password.                                 |
| POST       | `/auth/logout`            | Sesión; revoca la actual.                                                    |
| GET        | `/auth/me`                | Sesión; identidad, permisos, preferencias y terminal.                        |
| POST       | `/auth/password`          | Sesión; contraseña actual/nueva; rota la sesión.                             |
| GET / POST | `/users`                  | users.read / users.manage.                                                   |
| PATCH      | `/users/{id}`             | users.manage; no modifica sa.                                                |
| GET / PUT  | `/users/{id}/permissions` | permissions.read / permissions.manage.                                       |
| GET / PUT  | `/users/{id}/preferences` | users.manage; no modifica sa.                                                |
| GET / POST | `/roles`                  | roles.read / roles.manage + permissions.manage.                              |
| PATCH      | `/roles/{id}`             | roles.manage + permissions.manage; rol reservado rechazado.                  |
| GET        | `/permissions`            | permissions.read; registro central.                                          |
| GET / PUT  | `/settings`               | settings.read / settings.manage.                                             |
| GET / POST | `/registers`              | registers.read / registers.manage.                                           |
| PATCH      | `/registers/{id}`         | registers.manage; desactivar requiere desasociar terminales activas.         |
| GET        | `/devices`                | devices.read.                                                                |
| POST       | `/devices/invitations`    | devices.manage; código válido diez minutos.                                  |
| PATCH      | `/devices/{id}`           | devices.manage; nombre, caja y teclado.                                      |
| POST       | `/devices/{id}/revoke`    | devices.manage; revoca terminal y sesiones.                                  |
| GET        | `/sessions`               | sessions.read; hasta 200 recientes, sin tokens.                              |
| POST       | `/sessions/{id}/revoke`   | sessions.manage; una cuenta ordinaria no revoca directamente sesiones de sa. |
| GET / PUT  | `/preferences`            | Sesión propia.                                                               |
| PUT        | `/terminal/preferences`   | Sesión; solo teclado de la terminal actual.                                  |
| GET        | `/audit`                  | audit.read; filtros y paginación.                                            |

## Cuerpos principales

Setup/settings: `name`, `timezone` IANA, `currency` MXN, `phone`, `email`, `address`, `theme` light/dark. Setup añade `terminal_name`. La zona propuesta procede del OS; Go valida y conserva la elección. Setup repetido con autorización local responde configured/already_configured y no vuelve a emitir credenciales ni modifica datos.

Usuarios: `username`, `display_name`, `role_id`, `active`; POST añade password de 8 a 256 caracteres. PATCH no restablece contraseñas. Rol: `name`, `permissions` (claves del registro). Excepciones: `{ "overrides": { "users.read": "allow", "registers.read": "deny", "audit.read": "inherit" } }`. Heredar elimina la excepción; las demás claves enviadas se actualizan.

Cajas: `name`, `description`, `active`. Dispositivo: `name`, `register_id` UUID o null, `keyboard_mode` auto/always/disabled. Enrolamiento: `code`, `name`. Preferencias: `theme` light/dark y `home_screen`; se normaliza a status si no está implementada o permitida. Contraseña propia: `current_password`, `new_password`.

Login devuelve session_token al cliente nativo. Setup/enrolamiento devuelven device_id y device_token al cliente nativo. Rust los guarda en Keychain y elimina campos secretos antes de responder al WebView. El UUID público de sesión no autentica peticiones. Caducidad de sesión: doce horas; nuevo acceso reemplaza la anterior. El evento lógico de reemplazo se informa con SESSION_REPLACED en la siguiente consulta, no mediante un WebSocket activo.

## Auditoría y errores

`GET /audit`: parámetros `actor` (usuario exacto), `action` exacta, `device_id`, `from`/`until` RFC3339, `page` (desde 1), `page_size` 1..100. Respuesta `items`, `total`, `page`, `page_size`. Cada evento incluye instante, actor, terminal, operación, entidad, resultado y estados antes/después cuando aplica. Contraseñas, hashes y tokens están excluidos. La identidad sa sigue siendo visible en este historial.

Errores públicos:

```json
{
  "error": {
    "code": "PERMISSION_DENIED",
    "message": "No tienes permiso para esta operación.",
    "request_id": "identificador-opaco"
  }
}
```

400: validación; 401: dispositivo/sesión/credenciales inválidos; 403: permiso o aprovisionamiento rechazado; 404: inexistente; 409: duplicado o caja en uso; 413: cuerpo mayor de 64 KiB; 429: límite de intentos; 503: indisponibilidad. Los diagnósticos conservan los códigos de la etapa 1. Códigos de sesión: SESSION_REQUIRED, SESSION_INVALID, SESSION_EXPIRED, SESSION_REVOKED, SESSION_REPLACED; terminal: DEVICE_REQUIRED y DEVICE_REVOKED. No exponer detalles PostgreSQL.

JSON UTF-8, Cache-Control no-store, X-Content-Type-Options nosniff y request ID generado por Go. CORS exacto conserva consultas técnicas GET; la operación autenticada de escritorio usa HTTP nativo, sin ampliar CORS ni CSP. Ninguna credencial se transporta por query string. Timeouts administrativos de diez segundos en Go y doce en el cliente; respuesta nativa limitada a 2 MiB.

## Versionado y proyectos futuros

API v1 admite campos adicionales compatibles; cambios incompatibles requieren nueva versión. El diagnóstico verifica identidad/API/esquema. El servidor es único dueño del esquema; ningún cliente migra PostgreSQL.

Android offline necesitará una clave única (device_id, operation_id), hash de solicitud, resultado y pedido confirmados en la misma transacción; reintento idéntico devuelve el mismo resultado y contenido distinto da conflicto. KDS necesitará comandos autorizados por estación y eventos publicados tras commit. Outbox, cursor/replay, deduplicación, contratos de pedidos y transporte WebSocket continúan pendientes. El punto comercial de licencias sigue centralizado, sin protocolo definitivo.

## Catálogo · esquema 3

Todas las rutas siguientes requieren dispositivo/sesión y `catalog.read`. El puente expone operaciones fijas `catalog_*`; no acepta una URL arbitraria. Los cuerpos de catálogo admiten hasta 2 MiB. Dinero como cadena JSON; cantidades de porciones como cadenas hasta tres decimales; unidades comerciales como enteros. Los modelos completos están en `backend/internal/catalog/models.go` y `desktop/src/services/catalog.ts`.

| Método       | Ruta bajo `/api/v1/catalog`               | Permiso adicional                                                                                                        |
| ------------ | ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| GET          | `/{recurso}` y `/{recurso}/{id}`          | Los importes se omiten sin `catalog.prices.read`.                                                                        |
| POST / PATCH | `/categories` / `/categories/{id}`        | `catalog.categories.manage`                                                                                              |
| POST         | `/categories/reorder`                     | `catalog.categories.manage`                                                                                              |
| POST / PATCH | `/products` / `/products/{id}`            | `catalog.products.manage`; cambios económicos requieren `catalog.prices.manage`; combos también `catalog.combos.manage`. |
| PATCH        | `/products/{id}/state`                    | `catalog.products.state`                                                                                                 |
| PATCH        | `/products/{id}/availability`             | `catalog.availability.manage`                                                                                            |
| POST         | `/products/{id}/duplicate`                | Administración de productos/precios y permisos de su configuración copiada.                                              |
| GET          | `/products/{id}/configuration`            | Configuración real, categoría, grupos, zona y motivo de indisponibilidad.                                                |
| POST / PATCH | `/ingredients` / `/ingredients/{id}`      | `catalog.ingredients.manage`                                                                                             |
| POST / PATCH | `/complements`, `/portions` y sus `/{id}` | `catalog.complements.manage`                                                                                             |
| POST / PATCH | `/groups` / `/groups/{id}`                | `catalog.groups.manage` y `catalog.prices.manage`                                                                        |
| POST / PATCH | `/price-lists` / `/price-lists/{id}`      | `catalog.price_lists.manage` y `catalog.prices.manage`                                                                   |
| POST / PATCH | `/promotions` / `/promotions/{id}`        | `catalog.promotions.manage` y `catalog.prices.manage`                                                                    |
| GET          | `/lookups`                                | Referencias del editor; importes también sujetos a permiso.                                                              |
| POST / GET   | `/images` / `/images/{id}`                | Carga: `catalog.images.manage`. Consulta autenticada con `catalog.read`.                                                 |
| POST         | `/preview`                                | `catalog.prices.read`; excepción requiere `catalog.availability.override`.                                               |

Recursos de listado: categories, products, ingredients, complements, portions, groups, price-lists, promotions. Consultas `search`, `active`, `page`, `page_size` (hasta 100). Productos añade `category_id`, `kind=product|combo`, `archived=false|true|all`. Respuesta `{items,total,page,page_size}`. Los productos añaden `available_now` y `availability_reason` al listado. Los originales monetarios nunca se filtran a un actor sin permiso de precios.

Los POST de documentos nuevos envían `id:""`, `revision:0`, nombre, descripción, activo y campos específicos. PATCH envía el documento completo de GET y su revisión vigente. Los campos derivados exclusivos del listado no forman parte del cuerpo de edición. Una revisión antigua responde 409 `REVISION_CONFLICT` y no modifica datos. No hay DELETE; `active=false` o `archived=true` conservan identidad y referencias. Cambiar imagen, estado u horarios desde un documento también comprueba sus permisos propios; no permite eludir rutas específicas.

Estado: `{revision,active,archived}`. Disponibilidad: `{revision,temporary_unavailable,availability}`. Duplicar: `{revision,name,code}`. Reordenar: `{items:[{id,revision,sort_order}]}`; todo el lote revierte ante conflicto. Imágenes: POST `{content_base64}` PNG/JPEG, máximo 1 MiB y 2048 × 2048; GET devuelve `{id,data_uri,width,height}`. No se aceptan rutas locales ni formatos activos.

Horarios: `{starts_on?:"AAAA-MM-DD",ends_on?:"AAAA-MM-DD",days:[0..6],windows:[{start:"HH:mm",end:"HH:mm"}]}`. Arrays vacíos: sin restricción. Fin exclusivo; `24:00` solo como final. La zona proviene del establecimiento y el instante de Go, nunca del cliente.

Ejemplo de previsualización (sustituir los UUID por registros reales):

```json
{
  "price_list_id": null,
  "override_availability": false,
  "override_reason": "",
  "lines": [
    {
      "product_id": "00000000-0000-0000-0000-000000000001",
      "variant_id": null,
      "quantity": 1,
      "customization": {
        "ingredients": [],
        "groups": [
          {
            "group_id": "00000000-0000-0000-0000-000000000002",
            "options": []
          }
        ],
        "components": [],
        "notes": "Salsa aparte"
      }
    }
  ]
}
```

Grupo omitido: originales incluidos. Grupo explícito con opciones vacías: retirar todas. Cada opción enviada: `{option_id,quantity:"1.5"}`. Ingrediente: `{ingredient_id,quantity:"0"}`. Componente: `{slot_id,product_id,variant_id,quantity,customization}` recursivo. Las selecciones desconocidas, duplicadas, negativas o fuera de límites se rechazan.

Resultado: instante/zona, lista y revisión, `lines`, promociones, `subtotal`, `discount`, `total`. Cada línea entrega configuración/revisión, origen de base, ingredientes y extras, grupos con `original_value`, `requested_value`, `difference`, `extras` y opciones incluidas/solicitadas, árbol de componentes, notas y precio unitario. Las promociones incluyen `applied`, `reason`, `discount`. El resultado no se persiste como venta.

Errores del catálogo: `CATALOG_VALIDATION` 400, `PERMISSION_DENIED` 403, `REVISION_CONFLICT` 409, `PRODUCT_UNAVAILABLE` 409, además de los errores generales de etapa 2. No exponen SQL ni secretos. Escrituras, cambios de predeterminado y excepciones de disponibilidad se auditan transaccionalmente; una consulta normal de previsualización no crea movimientos comerciales.

Reglas, ejemplos y límites operativos: [CATALOG.md](docs/CATALOG.md).
