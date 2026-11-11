# API v1 · Etapa 8

Los contratos anteriores se mantienen. El detalle operativo nuevo está al final de este documento y en [RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md).

Etapa 8 (inventarios, recetas, almacenes, compras y costos; guía en [INVENTORY_AND_PURCHASING.md](docs/INVENTORY_AND_PURCHASING.md)): sus contratos están al final, en el orden de sus migraciones: «Inventario · fase A: núcleo (esquema 10)», «Inventario · fase B: recetas y consumo (esquema 11)», «Catálogo · indicaciones rápidas con efecto (esquema 12)», «Inventario · fase C: compras (esquema 13)» e «Inventario · fase D: control de inventario (esquema 14)». Las reglas comunes de inventario (permisos, lock, `operation_id`, cantidades y visibilidad de costos) se describen en la fase A y rigen las demás. Las correcciones de la revisión independiente (fase F del avance: valuación por insumo, elaboraciones y reversiones con existencia negativa, `COST_REQUIRED` y campos ocultos sin `inventory.stock.read`) están integradas en la sección de cada ruta.

HTTP JSON bajo `/api/v1`. Por defecto `http://127.0.0.1:8181`; LAN requiere HTTPS con identidad de servidor verificada.

## Autenticación de transporte

El puente nativo añade `X-Device-ID`, `X-Device-Token` y `Authorization: Bearer <sesión>`. Los tokens duraderos no se devuelven a React. Todos los endpoints protegidos comprueban dispositivo, sesión, usuario activo y permiso efectivo en PostgreSQL. Cambiar un identificador público no sustituye una credencial válida. Los ejemplos documentan nombres de cabeceras, nunca valores reales.

## Rutas

| Método     | Ruta                      | Requisito                                                                                             |
| ---------- | ------------------------- | ----------------------------------------------------------------------------------------------------- |
| GET        | `/health/live`            | Público; proceso activo.                                                                              |
| GET        | `/health/ready`           | Público; PostgreSQL y esquema compatibles, 200 o 503.                                                 |
| GET        | `/version`                | Público; producto, versión, API y versión del esquema (14 en la 0.8.0).                               |
| GET        | `/events`                 | SSE; sesión/dispositivo, events.read y ámbitos vigentes.                                              |
| GET        | `/setup/status`           | Público; solo configured booleano.                                                                    |
| POST       | `/setup`                  | Loopback y X-Provisioning-Token del propietario local.                                                |
| POST       | `/devices/enroll`         | Código de autorización de un solo uso, no requiere sesión previa.                                     |
| POST       | `/auth/login`             | Dispositivo autorizado; username y password.                                                          |
| POST       | `/auth/logout`            | Sesión; revoca la actual.                                                                             |
| GET        | `/auth/me`                | Sesión; identidad, permisos, preferencias y terminal.                                                 |
| POST       | `/auth/password`          | Sesión; contraseña actual/nueva; rota la sesión.                                                      |
| GET / POST | `/users`                  | users.read / users.manage; el rol asignado no puede superar los permisos del actor.                   |
| PATCH      | `/users/{id}`             | users.manage; no modifica sa, el propio rol ni usuarios con más permisos que el actor.                |
| GET / PUT  | `/users/{id}/permissions` | permissions.read / permissions.manage; no las propias ni concesiones fuera de los permisos del actor. |
| GET / PUT  | `/users/{id}/preferences` | users.manage; no modifica sa.                                                                         |
| GET / POST | `/roles`                  | roles.read / roles.manage + permissions.manage; solo permisos que posee el actor.                     |
| PATCH      | `/roles/{id}`             | roles.manage + permissions.manage; rol reservado, propio o con más permisos rechazado.                |
| GET        | `/permissions`            | permissions.read; registro central.                                                                   |
| GET / PUT  | `/settings`               | settings.read / settings.manage.                                                                      |
| GET / PUT  | `/settings/features`      | GET: cualquier sesión autenticada. PUT: settings.features.manage (etapa 6.1, esquema 7).              |
| GET / POST | `/registers`              | registers.read / registers.manage.                                                                    |
| PATCH      | `/registers/{id}`         | registers.manage; desactivar requiere desasociar terminales activas.                                  |
| GET        | `/devices`                | devices.read.                                                                                         |
| POST       | `/devices/invitations`    | devices.manage; código válido diez minutos.                                                           |
| PATCH      | `/devices/{id}`           | devices.manage; nombre, caja y teclado.                                                               |
| POST       | `/devices/{id}/revoke`    | devices.manage; revoca terminal y sesiones.                                                           |
| GET        | `/sessions`               | sessions.read; hasta 200 recientes, sin tokens.                                                       |
| POST       | `/sessions/{id}/revoke`   | sessions.manage; una cuenta ordinaria no revoca directamente sesiones de sa.                          |
| GET / PUT  | `/preferences`            | Sesión propia.                                                                                        |
| PUT        | `/terminal/preferences`   | Sesión; solo teclado de la terminal actual.                                                           |
| GET        | `/audit`                  | audit.read; filtros y paginación.                                                                     |

Delegación: un 403 `PERMISSION_DENIED` con mensaje específico rechaza crear, asignar o editar roles y excepciones cuyo resultado tenga permisos efectivos que el actor no posee, administrar a un usuario con más permisos, y modificar el propio rol o las propias excepciones. Retirar un `deny` (`inherit`) cuenta como concesión. Un cambio que dejaría al sistema sin un administrador completo (usuario ordinario activo con todos los permisos registrados) responde 400 `VALIDATION_ERROR` y no modifica datos.

## Cuerpos principales

Funciones del establecimiento (etapa 6.1): `GET /settings/features` devuelve `[{code, name, description, enabled, revision, updated_at, updated_by_name}]` con todas las funciones conocidas por Go (hoy solo `usd_cash`, desactivada por omisión). `PUT /settings/features` recibe `{code, enabled, revision}` sin `operation_id` (configuración con revisión optimista, como `/settings`); el código viaja en el cuerpo porque el puente nativo solo admite identificadores UUID en la ruta. Responde la función guardada; una revisión obsoleta da 409 `REVISION_CONFLICT`, un código desconocido 404 `FEATURE_NOT_FOUND`. Se audita como `settings.feature_changed` y publica `settings.features.changed` (ámbito `configuration`). Desactivar una función nunca borra ni altera registros. Desde la etapa 8 (esquema 10) cada función incluye `enabled_at`: la fecha y hora en que pasó de desactivada a activada por última vez (null si nunca se activó después de la 0010; desactivarla la conserva). La función `inventory_control` («Control de inventario», desactivada por omisión) publica además `inventory.settings.changed` en el ámbito `inventory`.

Setup/settings: `name`, `timezone` IANA (Go exige una zona existente con `time.LoadLocation` y rechaza `""` y `Local`), `currency` MXN, `phone`, `email`, `address`, `theme` light/dark. Setup añade `terminal_name`. La zona propuesta procede del OS; Go valida y conserva la elección. Setup repetido con autorización local responde configured/already_configured y no vuelve a emitir credenciales ni modifica datos.

Usuarios: `username`, `display_name`, `role_id`, `active`; POST añade password de 8 a 256 caracteres. PATCH no restablece contraseñas. Rol: `name`, `permissions` (claves del registro). Excepciones: `{ "overrides": { "users.read": "allow", "registers.read": "deny", "audit.read": "inherit" } }`. Heredar elimina la excepción; las demás claves enviadas se actualizan.

Cajas: `name`, `description`, `active`. Dispositivo: `name`, `register_id` UUID o null, `keyboard_mode` auto/always/disabled. Enrolamiento: `code`, `name`. Preferencias: `theme` light/dark y `home_screen`; se normaliza a status si no está implementada o permitida. Desde la etapa 6.1 `preview` ya no es una pantalla: un valor guardado o enviado como `preview` se interpreta como `catalog` (la vista previa vive en Catálogo). Desde la etapa 7 (fase B) `cash` («Caja») es una pantalla de inicio válida con `cash.shifts.read`. Desde la etapa 8 (fase A) `inventory` («Inventarios») también lo es, con `inventory.read`. Contraseña propia: `current_password`, `new_password`.

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

Desde la etapa 7.1, un rechazo puede incluir `error.details` con datos públicos que la terminal necesita para decidir (hoy solo `TABLE_HAS_ACCOUNT`, con las cuentas que otra terminal abrió en la mesa). Nunca lleva secretos ni cifras protegidas.

Auditoría de rechazos (etapa 7.1): además de la denegación de permisos (`access.denied`), un retiro o gasto rechazado por efectivo insuficiente revierte todo lo que la operación hubiera escrito y confirma solo su auditoría `cash.outflow_rejected` con `success: false` (ver «Caja, arqueos y cortes»). El error público no cambia.

400: validación; 401: dispositivo/sesión/credenciales inválidos; 403: permiso o aprovisionamiento rechazado (la operación completa se revierte y solo se audita la denegación); 404: inexistente; 409: duplicado o caja en uso; 413: cuerpo mayor del límite (64 KiB general; 2 MiB catálogo, restaurante, cocina e inventario); 429: límite de intentos; 503: indisponibilidad. Los diagnósticos conservan los códigos de la etapa 1. Códigos de sesión: SESSION_REQUIRED, SESSION_INVALID, SESSION_EXPIRED, SESSION_REVOKED, SESSION_REPLACED; terminal: DEVICE_REQUIRED y DEVICE_REVOKED. No exponer detalles PostgreSQL.

JSON UTF-8, Cache-Control no-store, X-Content-Type-Options nosniff y request ID generado por Go. CORS exacto conserva consultas técnicas GET; la operación autenticada de escritorio usa HTTP nativo, sin ampliar CORS ni CSP. Ninguna credencial se transporta por query string. Timeouts administrativos de diez segundos en Go y doce en el cliente; respuesta nativa limitada a 2 MiB.

## Versionado y proyectos futuros

API v1 admite campos adicionales compatibles; cambios incompatibles requieren nueva versión. El diagnóstico verifica identidad/API/esquema. El servidor es único dueño del esquema; ningún cliente migra PostgreSQL.

La etapa 4 implementa contratos de pedidos e idempotencia durable con UUID, hash, resultado y evento en la misma transacción. La etapa 5 añade outbox, cursor/replay, SSE y contratos KDS autorizados de consulta/acuse/estado. Android, offline durable y la aplicación KDS independiente siguen pendientes. No se implementó WebSocket: SSE cubre la notificación unidireccional y los comandos usan HTTP idempotente. El punto comercial de licencias sigue centralizado, sin protocolo definitivo.

## Catálogo · esquema 3

Todas las rutas siguientes requieren dispositivo/sesión y `catalog.read`. El puente expone operaciones fijas `catalog_*`; no acepta una URL arbitraria. Los cuerpos de catálogo admiten hasta 2 MiB. Dinero como cadena JSON; cantidades de porciones como cadenas hasta tres decimales; unidades comerciales como enteros. Los modelos completos están en `backend/internal/catalog/models.go` y `desktop/src/services/catalog.ts`.

| Método       | Ruta bajo `/api/v1/catalog`               | Permiso adicional                                                                                                                              |
| ------------ | ----------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| GET          | `/{recurso}` y `/{recurso}/{id}`          | Los importes se omiten sin `catalog.prices.read`.                                                                                              |
| POST / PATCH | `/categories` / `/categories/{id}`        | `catalog.categories.manage`                                                                                                                    |
| POST         | `/categories/reorder`                     | `catalog.categories.manage`                                                                                                                    |
| POST / PATCH | `/quick-notes` / `/quick-notes/{id}`      | `catalog.categories.manage`; indicaciones rápidas por categoría (etapa 7.1, migración 0009).                                                   |
| POST / PATCH | `/products` / `/products/{id}`            | `catalog.products.manage`; cambios económicos (incluida `tax_rate`) requieren `catalog.prices.manage`; combos también `catalog.combos.manage`. |
| PATCH        | `/products/{id}/state`                    | `catalog.products.state`                                                                                                                       |
| PATCH        | `/products/{id}/availability`             | `catalog.availability.manage`                                                                                                                  |
| POST         | `/products/{id}/duplicate`                | Administración de productos/precios y permisos de su configuración copiada.                                                                    |
| GET          | `/products/{id}/configuration`            | Configuración real, categoría, grupos, zona y motivo de indisponibilidad.                                                                      |
| POST / PATCH | `/ingredients` / `/ingredients/{id}`      | `catalog.ingredients.manage`                                                                                                                   |
| POST / PATCH | `/complements`, `/portions` y sus `/{id}` | `catalog.complements.manage`                                                                                                                   |
| POST / PATCH | `/groups` / `/groups/{id}`                | `catalog.groups.manage` y `catalog.prices.manage`                                                                                              |
| POST / PATCH | `/price-lists` / `/price-lists/{id}`      | `catalog.price_lists.manage` y `catalog.prices.manage`                                                                                         |
| POST / PATCH | `/promotions` / `/promotions/{id}`        | `catalog.promotions.manage` y `catalog.prices.manage`                                                                                          |
| GET          | `/lookups`                                | Referencias del editor; importes también sujetos a permiso.                                                                                    |
| POST / GET   | `/images` / `/images/{id}`                | Carga: `catalog.images.manage`. Consulta autenticada con `catalog.read`.                                                                       |
| POST         | `/preview`                                | `catalog.prices.read`; excepción requiere `catalog.availability.override`.                                                                     |

Recursos de listado: categories, products, ingredients, complements, portions, groups, price-lists, promotions. Consultas `search`, `active`, `page`, `page_size` (hasta 100). Productos añade `category_id`, `kind=product|combo`, `archived=false|true|all`. Respuesta `{items,total,page,page_size}`. Los productos añaden `available_now` y `availability_reason` al listado. Los originales monetarios nunca se filtran a un actor sin permiso de precios.

Productos y combos llevan desde la etapa 6.1 `tax_rate`: `null` (hereda la tasa general vigente de Cobros) o `16`, `8`, `0`; cualquier otro valor responde 400 (y PostgreSQL lo rechaza con `catalog_products_tax_rate_check`). PATCH debe enviarla siempre (ausente equivale a `null`). Duplicar conserva la tasa.

Los POST de documentos nuevos envían `id:""`, `revision:0`, nombre, descripción, activo y campos específicos. PATCH envía el documento completo de GET y su revisión vigente. Los campos derivados exclusivos del listado no forman parte del cuerpo de edición. Una revisión antigua responde 409 `REVISION_CONFLICT` y no modifica datos. No hay DELETE; `active=false` o `archived=true` conservan identidad y referencias. Cambiar imagen, estado u horarios desde un documento también comprueba sus permisos propios; no permite eludir rutas específicas.

Estado: `{revision,active,archived}`. Disponibilidad: `{revision,temporary_unavailable,availability}`. Duplicar: `{revision,name,code}`. Reordenar: `{items:[{id,revision,sort_order}]}`; todo el lote revierte ante conflicto. Imágenes: POST `{content_base64}` PNG/JPEG, máximo 1 MiB y 2048 × 2048; GET devuelve `{id,data_uri,width,height}`. No se aceptan rutas locales ni formatos activos.

Orden visual por posición (etapa 6.1): los POST/PATCH de categorías y productos aceptan el campo opcional de entrada `position:{after:""|UUID}`; `""` coloca el registro al inicio y un UUID inmediatamente después de ese hermano (categorías: todas; productos y combos: los de la misma categoría con la que se guarda el producto). Con `position`, el `sort_order` del cuerpo se ignora y Go renumera la lista en múltiplos de 10 dentro de la misma transacción (hermanos bloqueados con `FOR UPDATE`, revisión incrementada solo en los hermanos cuyo número cambió, auditoría `catalog.categories.reordered`/`catalog.products.reordered`); un registro que se agrega al final recibe el mayor número + 10 sin tocar a los demás y guardar sin mover no renumera. Sin `position` se conserva el comportamiento anterior. `after` ausente, inexistente, de otra lista (otra categoría) o igual al propio registro responde 400. `position` no se devuelve en las lecturas. El listado de productos se ordena por el orden de su categoría y después por el del producto.

Color de categoría (etapa 6.1): `""` o un `hex` de la paleta cerrada de Go (16 colores, se normaliza a minúsculas). Al editar se acepta también el color ya guardado aunque no esté en la paleta (colores anteriores a la paleta); cualquier otro valor responde 400. `GET /lookups` añade `palette:[{hex,name}]` y, en cada producto, `category_id` y `sort_order`.

Captura táctil (etapa 6.1, fase D; sin rutas nuevas): en el listado de productos, `search` busca por nombre **o código interno** (`ILIKE` en ambos; los demás recursos siguen buscando solo por nombre). `GET /lookups` añade en cada categoría `available_now` (booleano de solo lectura: activa y dentro de su horario en este momento, con el reloj de Go y la zona del establecimiento); la captura solo muestra las categorías disponibles y Go sigue rechazando al cotizar cualquier producto no disponible. `available_now` no forma parte del cuerpo de edición de la categoría.

Indicaciones rápidas por categoría (etapa 7.1, fase D; migración 0009): `POST /quick-notes` con `{category_id, text, active, revision: 0, position?: {after}}` y `PATCH /quick-notes/{id}` con `{revision, text, active, category_id?, position?: {after}}` (`catalog.categories.manage`, comprobado antes de leer con bloqueo o escribir; lock exclusivo 7820148304; auditoría `catalog.quick-notes.created`, `catalog.quick-notes.updated` y `catalog.quick-notes.reordered`, entidad `quick-notes`). El texto se recorta y debe tener de 1 a 80 caracteres, sin comas ni saltos de línea o caracteres de control (400 `CATALOG_VALIDATION`); dos indicaciones de la misma categoría que solo difieren en mayúsculas responden 409 `DUPLICATE_QUICK_NOTE`; una indicación no cambia de categoría (400) y la revisión obsoleta responde 409 `REVISION_CONFLICT`; hasta 40 por categoría. La posición sigue `internal/ordering` dentro de la categoría (sin `position` se crea al final). No hay borrado: se desactivan. Respuesta: `{id, category_id, text, active, sort_order, revision, updated_at}`. `GET /lookups` añade `category_quick_notes` con todas las indicaciones (activas e inactivas) en el orden de sus categorías y su orden visual; la captura usa solo las activas de la categoría del producto (o del producto de cada componente de combo), sin otra petición. Puente: `catalog_create_quick_note` y `catalog_save_quick_note`. Desde la etapa 8 (migración 0012) una indicación admite además un efecto sobre un ingrediente: ver «Catálogo · indicaciones rápidas con efecto (esquema 12)».

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

## Restaurante · esquema 4

Todas las rutas bajo `/api/v1/restaurant` requieren sesión y terminal vigentes. Toda ruta de cuenta exige `accounts.read`; los permisos de la tabla se agregan a este requisito. Para cuentas `takeaway`, consultar requiere `takeaway.read` y escribir requiere `takeaway.manage`. El listado filtra las modalidades que el usuario puede consultar. `overview` exige también `restaurant.areas.read`.

| Método      | Ruta bajo `/restaurant`        | Permiso específico                                            |
| ----------- | ------------------------------ | ------------------------------------------------------------- |
| GET         | `/overview`                    | `restaurant.tables.read` y `restaurant.areas.read`            |
| GET / POST  | `/areas`                       | `restaurant.areas.read` / `restaurant.areas.manage`           |
| PATCH       | `/areas/{id}`                  | `restaurant.areas.manage`                                     |
| GET / POST  | `/tables`                      | `restaurant.tables.read` / `restaurant.tables.manage`         |
| PATCH       | `/tables/{id}`                 | `restaurant.tables.manage`                                    |
| POST        | `/tables/{id}/service`         | `restaurant.tables.manage` (etapa 7.1)                        |
| PUT         | `/layout`                      | `restaurant.tables.manage`                                    |
| GET / PUT   | `/settings`                    | `accounts.read` / `restaurant.settings.manage`                |
| GET         | `/staff`                       | `accounts.transfer`                                           |
| GET / POST  | `/accounts`                    | `accounts.read` / `accounts.open`                             |
| GET / PATCH | `/accounts/{id}`               | `accounts.read` / `accounts.manage`                           |
| PUT         | `/accounts/{id}/draft`         | `orders.capture`                                              |
| POST        | `/accounts/{id}/draft-preview` | `orders.capture`; solo calcula                                |
| GET         | `/accounts/{id}/preview`       | `accounts.read`; calcula borrador guardado                    |
| POST        | `/accounts/{id}/confirm`       | `orders.confirm`                                              |
| POST        | `/accounts/{id}/edit-preview`  | `orders.edit`; solo calcula                                   |
| POST        | `/accounts/{id}/edit`          | `orders.edit`; reducir unidades exige también `orders.cancel` |
| POST        | `/accounts/{id}/cancel`        | `orders.cancel`                                               |
| POST        | `/accounts/{id}/split`         | `accounts.split` y `accounts.open`                            |
| POST        | `/accounts/{id}/transfer`      | `accounts.transfer_items`                                     |
| POST        | `/accounts/{id}/merge`         | `accounts.merge`                                              |
| POST        | `/accounts/{id}/move`          | `accounts.move`                                               |
| POST        | `/accounts/{id}/assign`        | `accounts.transfer`                                           |
| POST        | `/accounts/{id}/void`          | `accounts.manage`                                             |
| POST        | `/accounts/{id}/release`       | `accounts.open` (etapa 7.1)                                   |
| GET         | `/accounts/{id}/history`       | `orders.history.read`                                         |

Guardar/cotizar borrador, confirmar y editar exige además `catalog.read` y `catalog.prices.read`. Seleccionar otra lista requiere `orders.price_list.manage`. El PATCH de cuenta verifica también `accounts.move` o `accounts.transfer` si cambian esos campos. Un mesero autorizado no necesita otra credencial de supervisor para editar/cancelar.

Las escrituras llevan `operation_id` UUID. Actualizar exige `revision`; transferir/combinar exige también `target_revision`. GET y POST de previsualización no llevan UUID ni persisten cambios. Repetir exactamente una escritura del mismo actor/terminal devuelve el resultado original. `REVISION_CONFLICT`, `QUOTE_CHANGED` e `IDEMPOTENCY_CONFLICT` responden 409 y conservan el estado previo. No hay DELETE. Cuerpos hasta 2 MiB y respuestas nativas hasta 2 MiB.

### Documentos y consultas

- Área: `name`, `description`, `sort_order`, `active` y, desde la etapa 6.1, `position:{after}` opcional con la misma regla del catálogo (auditoría `restaurant.areas.reordered`); PATCH conserva `id` y revisión del documento leído.
- Mesa: `area_id`, `name`, `shape` (`rectangle`/`round`), `x`, `y`, `width`, `height`, `seats`, `active`, `out_of_service`. El lienzo lógico es 1000×560. `/layout` recibe `tables:[{id,revision,x,y,width,height,shape}]` y UUID del lote; todas las posiciones se validan antes de escribir.
- Estado de mesa (etapa 7.1), calculado por Go en una sola consulta para todo el mapa (`/overview`, `/tables`, respuestas de mesa): `account_count`, `state` (`free`, `occupied`, `awaiting_payment`, `partial_payment`, `out_of_service`), `occupied_since` (apertura de la cuenta abierta más antigua o `null`), `occupied_seconds` (segundos transcurridos según el servidor) y `balance` (suma de los saldos de las cuentas abiertas, cadena decimal). Reglas: `out_of_service` si la mesa está desactivada o fuera de servicio; `free` sin cuentas abiertas; `awaiting_payment` si alguna cuenta tiene una precuenta impresa (`payment_documents.kind='precheck'`) posterior a su último cambio de contenido (`restaurant_accounts.updated_at`, ajustes y cancelaciones de ajustes) y saldo mayor que cero (los pagos posteriores no la invalidan); `partial_payment` si alguna cuenta tiene pagado neto y saldo mayores que cero; `occupied` en otro caso, también con una cuenta sin productos. Con varias cuentas gana el más avanzado: esperando pago > pago parcial > ocupada. El total, lo pagado y el saldo usan la fórmula única de `payments` (`AccountStandingSQL`, la misma de `finance` en cada cuenta). El mapa se actualiza con los eventos existentes (`restaurant.*`, `payments.recorded`, `payments.adjusted`, `payments.document.printed`, etc., todos en el ámbito `restaurant` para salón).
- Fuera de servicio desde la operación (etapa 7.1): `POST /tables/{id}/service` recibe `{operation_id,revision,out_of_service}` y responde la mesa con su estado. Rechaza una mesa desactivada o ya en ese estado (400), una revisión obsoleta (`ACCOUNT_CONFLICT`, 409) y poner fuera de servicio una mesa con cuentas abiertas (`TABLE_HAS_OPEN_ACCOUNTS`, 409); habilitar siempre procede. Audita `restaurant.table.out_of_service` o `restaurant.table.enabled` (antes y después) y publica `restaurant.tables.changed`. El PATCH de mesa responde también `TABLE_HAS_OPEN_ACCOUNTS` si cambia `out_of_service` a verdadero con cuentas abiertas (antes lo permitía); una mesa que ya estaba fuera de servicio con cuentas puede seguir editándose.
- Cuenta: `name`, `mode` (`dine_in`/`takeaway`, inmutable), `table_id` opcional, `price_list_id` opcional, `customer_name`, `phone`, `origin`, `notes`, `scheduled_local` (`YYYY-MM-DDTHH:mm`), `schedule_kind` (`preparation`/`delivery` o vacío), `assigned_user_id` opcional. Para llevar rechaza mesa. La zona proviene del establecimiento; el cliente no decide el offset.
- Nombre opcional (etapa 6.2): al abrir (`POST /accounts`) o editar (`PATCH /accounts/{id}`), un `name` vacío, solo con espacios o ausente ya no responde 400: Go asigna una referencia neutra, el nombre de la mesa (`table_id`) o `Cuenta` sin mesa en salón, y `Pedido para llevar` en para llevar. Sin `price_list_id` se usa la lista predeterminada activa y sin `assigned_user_id` el usuario que abre, como antes. Una cuenta cuyo nombre es la referencia neutra de su mesa la actualiza al cambiar de mesa (`POST /accounts/{id}/move` o PATCH con otra `table_id`); un nombre capturado se conserva. Las demás validaciones no cambian (100 caracteres, programación con hora obligatoria solo si hay `schedule_kind`, mesa activa); `new_account.name` de la división sigue siendo obligatorio. La interfaz trata como «sin nombre» la referencia neutra (y los nombres `Cuenta`/`Para llevar` del formulario anterior) e identifica la cuenta por su folio.
- Cuenta creada con su primer producto (etapa 7.1): `POST /accounts` admite además `items` (borrador inicial con el mismo formato, límites y validaciones que `PUT /accounts/{id}/draft`) y `seen_account_ids` (UUID de las cuentas abiertas que la terminal veía en la mesa, hasta 100). Con `items` no vacío exige además `orders.capture`, `catalog.read` y `catalog.prices.read` antes de escribir. La cuenta (revisión 1) y su borrador se registran en la misma operación idempotente, con los eventos `account.opened` y `draft.saved`; la respuesta es la cuenta completa. Con `seen_account_ids` presente y `table_id`, si la mesa tiene una cuenta abierta que no está en la lista responde `409 TABLE_HAS_ACCOUNT` sin escribir nada, con `error.details.accounts: [{id, folio, name}]`; la terminal reenvía con la lista ampliada y un `operation_id` nuevo («Abrir una cuenta nueva») o guarda sus productos en el borrador de esa cuenta con `PUT …/draft` («Agregar a esa cuenta»). Sin `seen_account_ids` no se comprueba la mesa (comportamiento anterior, que conservan para llevar, «Cuenta sin mesa» y las integraciones).
- Liberar cuenta vacía (etapa 7.1): `POST /accounts/{id}/release` recibe `{operation_id,revision}`, sin motivo ni autorización adicional, y responde la cuenta completa en `voided`. Procede solo si la cuenta está abierta y vacía en sentido estricto: sin partidas en el borrador, sin unidades en ningún estado, sin pedidos confirmados en ella, sin haber recibido una combinación, sin pagos, devoluciones, ajustes (aunque estén cancelados), autorizaciones, movimientos de caja, documentos comerciales ni entregas, y sin liberaciones ni cambios pendientes de cocina; en otro caso, `409 ACCOUNT_NOT_EMPTY`. Guarda el motivo fijo «Cuenta vacía liberada» en el evento `account.released`, audita `restaurant.account.released` y publica `restaurant.account.released`. Cada cuenta incluye `releasable` (booleano calculado por Go con la misma regla) para mostrar «Liberar mesa».
- Listado: `mode`, `table_id` (UUID o `unassigned`), `search`, `state` (`open` predeterminado, `merged`, `voided`, `all`), `page`, `page_size` hasta 100. Respuesta `{items,total,page,page_size,server_time,timezone,settings}` con orden ascendente de creación. `search` consulta folio, referencia, cliente y teléfono.
- Cuenta completa: `{account,draft,lines,server_time,timezone}`. `lines` agrupa unidades por versión y estado e incluye `quantity`, solicitud, desglose histórico y totales exactos de las unidades que conserva esa cuenta. Los UUID de unidad se conservan en la base y eventos.
- Historial: `page`, 100 eventos por página, orden descendente; incluye eventos donde la cuenta es origen o relacionada. Autor, terminal, instante, tipo, valores anterior/posterior y estado de integración son persistidos.
- Configuración: `warning_minutes`, `overdue_minutes`, `revision`, `operation_id`; advertencia 1–240 minutos, atraso entre advertencia y 1440.

### Captura y confirmación

Guardar o cotizar una captura envía `revision` e `items`. Cada ítem lleva UUID estable de borrador, `product_id`, `variant_id` nullable, `quantity` entera y `customization` del catálogo. PUT de borrador añade el UUID de operación; POST `draft-preview` no lo requiere. Límite de lote: 20 partidas/1000 unidades totales. La captura táctil de la etapa 6.1 usa estos mismos contratos: guarda el borrador completo con antirrebote y una sola petición en vuelo, y «Enviar a cocina» llama a `restaurant_confirm` con la huella de `GET /accounts/{id}/preview` (ver [docs/RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md)).

```json
{
  "operation_id": "00000000-0000-4000-8000-000000000001",
  "revision": 1,
  "items": [
    {
      "id": "00000000-0000-4000-8000-000000000002",
      "product_id": "UUID real del catálogo",
      "variant_id": null,
      "quantity": 2,
      "customization": { "ingredients": [], "groups": [], "components": [], "notes": "" }
    }
  ]
}
```

La previsualización devuelve `{account_id,revision,fingerprint,quote,combined,pricing_policy}`. `quote` corresponde al lote; `combined` suma en Go el importe confirmado con el lote tentativo. Confirmar envía UUID, revisión y `expected_fingerprint` de GET `/preview`. Go vuelve a calcular y persiste solo si coincide. El resultado es la cuenta completa; el borrador se vacía y se crea una liberación durable y se informa el estado real de preparación.

Editar envía `{operation_id,revision,version_id,line,reason,expected_fingerprint}`; `line` usa el contrato de catálogo. Primero POST `/edit-preview` con los mismos datos sin UUID/huella. Motivo de edición o cancelación: 5–500 caracteres. Cancelar envía `{operation_id,revision,version_id,quantity,reason}`. Se conservan versiones, unidades y centavos históricos conforme a [las reglas de modificación](docs/RESTAURANT_OPERATIONS.md).

### Acciones entre cuentas

Dividir recibe `{operation_id,revision,selections:[{version_id,quantity}],new_account:{name}}`. Transferir recibe esas selecciones y `target_account_id`, `target_revision`. Combinar recibe revisiones y cuenta destino sin selecciones. Las tres responden `{source,target}` con cuentas completas. Las cantidades son unidades comerciales enteras; no se recalculan ni se clonan sus importes. La cuenta nueva de división hereda contexto comercial y responsable.

Mover recibe `{operation_id,revision,table_id}` con null para retirar mesa. Asignar recibe `{operation_id,revision,assigned_user_id}`. Anular cuenta vacía recibe `{operation_id,revision,reason}`. Las respuestas son cuentas completas y todas las acciones se auditan transaccionalmente.

No existen rutas de cobro o entrega. `payment_state:not_recorded` y `delivery_state:not_delivered` siguen independientes de preparación. La etapa 5 integra liberaciones, documentos, impresión y eventos conforme a los contratos siguientes.

## Cocina e impresión · etapa 5

Prefijo `/api/v1/kitchen`. Todos los UUID se validan; las escrituras conservan `operation_id` y recuperan el resultado solo para el mismo actor, terminal, ruta y cuerpo. PATCH/PUT y acciones sobre entidades existentes usan revisión. Conflictos responden 409; no se regeneran UUID después de una respuesta incierta. No hay DELETE ni URL o comando arbitrario desde React.

| Método     | Sufijo                                  | Permiso                                                                                 |
| ---------- | --------------------------------------- | --------------------------------------------------------------------------------------- |
| GET / POST | `/stations`                             | `kitchen.stations.read` / `kitchen.stations.manage`                                     |
| PATCH      | `/stations/{id}`                        | `kitchen.stations.manage`; tiempos requieren también `kitchen.timing.manage`            |
| GET        | `/outputs`                              | `kitchen.stations.manage`; destinos sin configuración técnica completa                  |
| GET / PUT  | `/access`                               | `kitchen.stations.manage`                                                               |
| GET / POST | `/printers`                             | `kitchen.printers.manage`                                                               |
| PATCH      | `/printers/{id}`                        | `kitchen.printers.manage`                                                               |
| GET        | `/discovery`                            | `kitchen.printers.manage`; consulta CUPS real del servidor                              |
| GET        | `/targets`                              | `kitchen.routing.manage`; categorías/productos sin precios                              |
| GET / POST | `/routes`                               | `kitchen.routing.manage`                                                                |
| PATCH      | `/routes/{id}`                          | `kitchen.routing.manage`                                                                |
| GET        | `/settings`                             | `kitchen.stations.read`, `kitchen.formats.manage` o `kitchen.timing.manage`             |
| PUT        | `/settings`                             | `kitchen.formats.manage`; cambios de tiempos/seguimiento exigen `kitchen.timing.manage` |
| PUT        | `/timing`                               | `kitchen.timing.manage`                                                                 |
| GET        | `/board`, `/releases`, `/releases/{id}` | `kitchen.states.read` y alcance de estación                                             |
| POST       | `/releases/{id}/release`                | `kitchen.release`; adelantada requiere `kitchen.release_early`                          |
| POST       | `/releases/{id}/schedule`               | `kitchen.timing.manage`                                                                 |
| POST       | `/changes/{id}/retry`                   | `kitchen.release`                                                                       |
| POST       | `/states`                               | `kitchen.states.manage` y alcance por cada estación afectada                            |
| GET        | `/tickets`, `/tickets/{id}`             | `kitchen.tickets.read` y alcance de estación                                            |
| POST       | `/tickets/{id}/ack`                     | `kitchen.kds.ack` y alcance de estación                                                 |
| GET        | `/jobs`, `/jobs/{id}`                   | `kitchen.queue.read` y `kitchen.stations.all`                                           |
| POST       | `/jobs/{id}/retry`                      | `kitchen.queue.retry` y `kitchen.stations.all`                                          |
| POST       | `/jobs/{id}/resolve`                    | `kitchen.queue.resolve` y `kitchen.stations.all`                                        |
| POST       | `/reprint`                              | `kitchen.reprint` y `kitchen.stations.all`                                              |
| POST       | `/test-print`                           | `kitchen.printers.test`                                                                 |
| GET        | `/history`                              | `kitchen.history.read` y `kitchen.stations.all`                                         |

El alcance global `kitchen.stations.all` se comprueba además del permiso funcional. Las consultas y acciones sobre liberaciones/cambios pendientes completos también lo exigen. Sin él, las tareas/documentos operativos y eventos se filtran por `kitchen_device_stations`; no basta conocer UUID. El listado de configuración de estaciones conserva su permiso específico. Cambiar un destino durante una resolución o reimpresión exige también `kitchen.printers.manage`. Los veinte permisos nuevos solo se asignan inicialmente al administrador.

### Configuración

- Estación: `{name,description,active,sort_order,reception,printer_id,target_minutes,warning_minutes,overdue_minutes,grouping,tracking}` y `position:{after}` opcional (etapa 6.1, misma regla del catálogo; historial `station.reordered`). Recepción `printer|kds|both`; agrupación `inherit|independent|unified`; seguimiento `inherit|item|order`. PATCH lleva `id` y `revision`.
- Impresora: `{name,transport,destination,port,paper_width,copies,supports_cut,cut,cutter_option,active,fallback_id}`. Transporte `cups|escpos_tcp|agent`; papel 58/80 mm, copias 1–3, puerto 9100–9109. CUPS utiliza nombre de cola; TCP una IP privada explícita; agente todavía no implementado. El respaldo es opcional y no puede formar ciclos. PATCH conserva identidad/revisión.
- Ruta: exactamente uno de `category_id|product_id`, `mode:prepare|none`, `station_ids`. Preparación exige estaciones activas no repetidas; exención explícita usa lista vacía. PATCH conserva `id/revision`; producto prevalece sobre categoría.
- Acceso: `{device_id,station_ids,expected_station_ids,operation_id}`. La lista esperada del último GET permite rechazar una edición concurrente. Solo dispositivos autorizados existentes.
- Formato/política: `{grouping,tracking,automatic_release,lead_minutes,show_prices,show_notes,show_establishment,text_size,line_spacing,quantity_style,revision,operation_id}`. Agrupación `independent|unified`, seguimiento `item|order`, anticipación 0–1440, texto `normal|large`, separación 1–3, cantidad `prefix|multiplier`. PUT `/timing` recibe únicamente automatismo, anticipación, seguimiento, revisión y UUID.

### Preparación y documentos

`GET /board` acepta `station_id`, `account_id`, `state` y `page`; devuelve tareas con contexto actual de cuenta, antigüedad, tiempos, instrucciones y revisión, además de estaciones y, si tiene alcance global, liberaciones/cambios pendientes. Máximo 250 tareas por página. El estado de cuenta/partida lo calcula Go: `pending_release`, `scheduled`, `released`, `preparing`, `ready`, `change_pending`, `cancelled`; el estado vacío representa ausencia de productos confirmados.

Liberar: `{operation_id,revision}`. Programar: `{operation_id,revision,scheduled_local,automatic,lead_minutes}`. La hora local `YYYY-MM-DDTHH:mm` se interpreta en la zona del establecimiento y se rechaza si no existe. Revisar cambio pendiente: `{operation_id,revision}` sobre UUID del evento conservado.

Actualizar partidas: `{operation_id,state,tasks:[{id,revision}]}`. Actualizar pedido: `{operation_id,state,release_id,revision,station_id?}`; sin estación abarca las autorizadas del pedido y exige autorización para todas. Estado `preparing|ready`; una estación configurada por pedido rechaza avances por partida. No se avanza una versión obsoleta, cancelada ni se retrocede desde listo.

`GET /tickets` acepta estación/cuenta/página; devuelve resúmenes con KDS pendiente. El detalle conserva UUID, tipo, contexto, ajustes, secciones e instrucciones históricas y `preview` generado por Go. Se ocultan precios salvo formato explícito y `catalog.prices.read`. `POST /tickets/{id}/ack` recibe `{operation_id,station_id}`; solo estaciones KDS/both y recepción única por documento/estación/dispositivo. No cambia preparación o impresión.

### Cola y copias

`GET /jobs` acepta `state` y `page` (100 registros). El detalle incluye trabajo, intentos y documento; no expone contenido crudo fuera de la presentación autorizada. Estados `pending|processing|submitted|error|uncertain|intervention|cancelled`. `submitted` es aceptación del transporte, sin confirmación física.

Reintentar: `{operation_id,revision,reason,printer_id?}`; solo error confirmado o intervención. Resolver: mismos campos y `decision:cancel|accepted|send`; `send` tras incertidumbre es una autorización explícita que reconoce posible copia. No se interviene mientras `processing`. Motivo 5–500 caracteres.

Reimprimir: `{operation_id,job_id,reason,printer_id?}`. El trabajo anterior debe estar finalizado/resuelto; otra copia pendiente o incierta devuelve `PRINT_COPY_PENDING` 409. Se usa documento y formato original sin leyenda adicional. Prueba técnica: `{operation_id,printer_id}`; crea solo documento de prueba/trabajo, sin pedido o liberación comercial.

### SSE y recuperación

`GET /api/v1/events?after=0&station_id=UUID` transmite `text/event-stream`. Sesión/dispositivo y `events.read` obligatorios; filtro por permiso de cada ámbito y estación. También admite `Last-Event-ID` si falta `after`. Parámetros desconocidos, cursor negativo y estación ajena se rechazan. No colocar tokens en query string.

Cada frame `event: changes` incluye `id: cursor` y JSON `{cursor,reset,events}`. Un evento contiene `{sequence,id,version:1,kind,scope,station_id,aggregate_id,created_at}`. No contiene datos de precio/cliente. Lotes hasta 200; ventana lógica 5000; `reset:true` obliga a volver a consultar el estado completo. `GET /api/v1/events/snapshot` devuelve ese mismo lote con idéntica autenticación y permisos.

Los tipos notifican cambios de restaurante/para llevar/configuración, liberación, preparación, recepción KDS y estados de impresión. Son invalidaciones: el consumidor consulta la API autoritativa y deduplica por UUID. Las sesiones/dispositivos/permisos se revalidan durante la conexión. Dos conexiones por dispositivo; la tercera responde `EVENT_CONNECTION_LIMIT` 429. Duración 30 segundos y reconexión nativa con cursor; un frame `interrupted` incluye código/status público y cierra el flujo.

SSE se eligió como equivalente a WebSocket porque las acciones utilizan HTTP idempotente y las notificaciones son unidireccionales. Rust conserva tokens en Keychain y entrega frames a React con Channel. No existe WebSocket ni aplicación KDS independiente. La guía [KITCHEN_AND_PRINTING.md](docs/KITCHEN_AND_PRINTING.md) detalla contratos KDS y límites de entrega física.

## Cobros, pagos y devoluciones · esquemas 6 y 7

Prefijo `/api/v1/payments`. Todas las rutas requieren sesión y terminal vigentes; las rutas de cuenta exigen además `accounts.read` y, para pedidos para llevar, `takeaway.read`. Las escrituras llevan `operation_id` UUID y son idempotentes por actor, terminal, ruta y cuerpo; la huella excluye `authorization.password`. El dinero viaja como cadena decimal con hasta dos decimales. Las previsualizaciones (`preview`) solo calculan. Reglas completas en [PAYMENTS_AND_REFUNDS.md](docs/PAYMENTS_AND_REFUNDS.md).

| Método     | Ruta bajo `/payments`              | Permiso                                                                                                                                                                                                         |
| ---------- | ---------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET / PUT  | `/settings`                        | `payments.read` / `payments.settings.manage`                                                                                                                                                                    |
| PUT        | `/settings/usd`                    | `payments.settings.manage`; `{operation_id, revision, usd_rate_min, usd_rate_max, usd_rate_alert_percent}`; 409 `FEATURE_DISABLED` sin `usd_cash`                                                               |
| PUT        | `/tax`                             | `payments.tax.manage`; `tax_rate` exactamente `"16"`, `"8"` o `"0"`                                                                                                                                             |
| GET / POST | `/methods`                         | `payments.read` / `payments.methods.manage`                                                                                                                                                                     |
| PATCH      | `/methods/{id}`                    | `payments.methods.manage`; conserva identidad y revisión                                                                                                                                                        |
| GET / POST | `/discount-reasons`                | `payments.read` / `payments.settings.manage`; motivos de descuento (etapa 7.1, migración 0009)                                                                                                                  |
| PATCH      | `/discount-reasons/{id}`           | `payments.settings.manage`; revisión optimista, sin borrado                                                                                                                                                     |
| GET        | `/shifts/current`                  | `payments.read`; caja de la terminal, turno activo (abierto o en arqueo de cierre) y si es utilizable; nunca importes esperados                                                                                 |
| POST       | `/shifts`                          | `cash.shifts.open`; `{operation_id, opening_float, opening_float_usd?, opening_notes?, usd_rate?, confirm_rate_difference?, count?}` (etapa 7); el tipo de cambio exige `usd_cash` y `payments.usd_rate.manage` |
| POST       | `/shifts/{id}/usd-rate`            | `payments.usd_rate.manage`; `{operation_id, usd_rate, confirm_rate_difference?}`; una sola vez por turno abierto sin tipo de cambio                                                                             |
| GET        | `/shifts/{id}/movements`           | `cash.shifts.read`; movimientos y totales por método; el esperado y `by_currency` solo con `cash.expected.read` y fuera del arqueo (etapa 7)                                                                    |
| GET        | `/accounts/{id}`                   | `payments.read`; estado financiero completo                                                                                                                                                                     |
| POST       | `/accounts/{id}/preview`           | `payments.read`; plan de cobro                                                                                                                                                                                  |
| POST       | `/accounts/{id}/payments`          | `payments.collect`; parcial `payments.collect_partial`; selección `payments.collect_selection`; propina `payments.tips.record`; `print` `payments.tickets.print`; `deliver` `takeaway.deliver`                  |
| POST       | `/accounts/{id}/discounts/preview` | `payments.read`                                                                                                                                                                                                 |
| POST       | `/accounts/{id}/discounts`         | `payments.discounts.apply` dentro del límite, `payments.courtesy` para cortesías, o autorización de supervisor                                                                                                  |
| POST       | `/accounts/{id}/charges`           | `payments.charges.create`                                                                                                                                                                                       |
| POST       | `/accounts/{id}/adjustments/void`  | Cargo: `payments.charges.manage`; descuento: `payments.discounts.apply`; o autorización                                                                                                                         |
| POST       | `/accounts/{id}/refunds/preview`   | `payments.read`                                                                                                                                                                                                 |
| POST       | `/accounts/{id}/refunds`           | `payments.refunds.execute` (y bajo el umbral) o autorización con `payments.refunds.authorize`                                                                                                                   |
| POST       | `/accounts/{id}/corrections`       | `payments.corrections` o autorización; solo con el turno original abierto (409 `SHIFT_CLOSING` en arqueo, `SHIFT_CLOSED` cerrado)                                                                               |
| GET        | `/accounts/{id}/precheck`          | `payments.read`; no registra venta                                                                                                                                                                              |
| POST       | `/accounts/{id}/precheck/print`    | `payments.tickets.print`                                                                                                                                                                                        |
| POST       | `/accounts/{id}/documents/reprint` | `payments.tickets.reprint`; `{operation_id, ticket_id}`                                                                                                                                                         |
| POST       | `/accounts/{id}/deliver`           | `takeaway.deliver`; solo pedidos para llevar liquidados                                                                                                                                                         |

### Cuerpos principales

- Método de pago: `{operation_id, revision, name, icon, active, sort_order, is_cash, reference_mode, allows_tip, allows_refund}` y, desde la etapa 6.1, `position:{after}` opcional con la misma regla del catálogo (auditoría `payments.methods_reordered`).
- Pago: `{operation_id, method_id, mode: "total"|"partial", amount?, received?, received_usd?, change_currency?: "MXN"|"USD", tip?, reference?, selections?: [{version_id, quantity}], expected_balance, print?, deliver?, shift_id?}`. Respuesta `{payment, finance, closed, delivered, printing}`. `shift_id` (etapa 7, opcional) es el turno que mostraba la terminal: forma parte de la huella y, si ese turno está en arqueo o cerrado, el cobro (o su reintento incierto) se rechaza con 409 `SHIFT_CLOSING`/`SHIFT_CLOSED`; si la caja tiene otro turno, 409 `SHIFT_CHANGED`. La devolución acepta el mismo campo.
- Dólares (función `usd_cash`, etapa 6.1): `received_usd` y `change_currency: "USD"` solo se aceptan con la función activa (si no, 409 `FEATURE_DISABLED`, también en la vista previa), con un método de efectivo (si no, 400) y con un turno que tenga tipo de cambio (si no, 409 `USD_RATE_REQUIRED`). Con dólares, `received` son los pesos recibidos (vacío = 0). La vista previa agrega `usd: {rate, balance_usd, charged_usd, received_mxn, received_usd, received_usd_mxn, received_total, change_mxn, change_usd, change_currency, covered, missing_mxn, missing_usd}` cuando la función está activa, el método es efectivo y el turno tiene tipo de cambio; si lo recibido no alcanza, la vista previa devuelve `covered:false` con el faltante y el registro responde 400. El pago (`payments[]`) agrega `received_mxn`, `received_usd`, `usd_rate`, `change_currency` y `change_usd` (null en pagos solo en pesos); `received` es el total recibido en pesos y `change` el cambio en pesos.
- Tipo de cambio: cadena con hasta seis enteros y dos decimales, positiva y dentro de `[usd_rate_min, usd_rate_max]` de Cobros (400 `USD_RATE_INVALID` / `USD_RATE_OUT_OF_RANGE`). Si difiere más de `usd_rate_alert_percent` % del último tipo capturado en otro turno, responde 409 `USD_RATE_CONFIRMATION_REQUIRED` salvo `confirm_rate_difference: true`. Un turno que ya lo tiene responde 409 `USD_RATE_ALREADY_SET`. `shift` agrega `usd_rate`, `usd_rate_set_by_name` y `usd_rate_set_at`; `GET /shifts/current` agrega `usd_cash` (estado de la función) y `usd_rate_required`.
- Movimientos del turno: cada movimiento agrega `mxn_in`, `mxn_out`, `usd_in`, `usd_out` y `usd_rate` (null en movimientos anteriores a la 0007 o no efectivo) y la respuesta agrega `by_currency: {mxn_in, mxn_out, usd_in, usd_out, expected_mxn, expected_usd}`. No es un arqueo.
- Configuración (`GET /settings`) agrega `usd_rate_min`, `usd_rate_max` y `usd_rate_alert_percent` (10, 40 y 10 iniciales).
- Descuento: `{operation_id, scope: "account"|"items", method: "percent"|"amount", value, courtesy, concept?, reason, reason_id?, selections?, expected_balance, authorization?: {username, password}}`. Cargo: `{operation_id, concept, amount, reason?, expected_balance}`. Cancelación: `{operation_id, adjustment_id, reason, authorization?}`.
- Motivos de descuento (etapa 7.1): `GET /discount-reasons` devuelve todos los motivos (activos e inactivos) en su orden visual: `[{id, name, requires_description, active, sort_order, revision, updated_at}]`; la 0009 siembra Atención, Promoción, Demora, Cortesía de la casa y Otro (solo Otro con `requires_description`). Alta y edición: `{operation_id, revision?, name, requires_description, active, position?: {after}}`; nombre de 1 a 60 caracteres sin saltos de línea, único (409 `DUPLICATE_NAME`), revisión obsoleta 409 `REVISION_CONFLICT` y debe quedar al menos un motivo activo (409 `LAST_ACTIVE_REASON`). Auditoría `payments.discount_reason_created`, `payments.discount_reason_updated` y `payments.discount_reasons_reordered` (entidad `discount_reason`) y evento `payments.discount_reasons.changed` (ámbito `configuration`). Desactivar o renombrar un motivo no modifica los descuentos registrados. Puente: `payments_discount_reasons`, `payments_create_discount_reason` y `payments_save_discount_reason`.
- Motivo del descuento o la cortesía (etapa 7.1): `reason_id` es opcional por compatibilidad. Sin él rige la regla de la etapa 6 (`reason` obligatorio de 5 a 500 caracteres) y el ajuste se guarda con `reason_id` nulo, como los anteriores. Con él, el motivo debe existir (404) y estar activo (409 `DISCOUNT_REASON_INACTIVE`); `reason` es su descripción: obligatoria de 5 a 500 si el motivo la exige y opcional (hasta 500) en otro caso (400). Se valida antes de escribir y forma parte de la huella idempotente. Cada ajuste de `finance.adjustments` agrega `reason_id` y `reason_name` (nulos en cargos y descuentos sin motivo; `reason_name` es el nombre del motivo al registrar el descuento, no el vigente); la auditoría `payments.discount_applied` agrega `detail.reason_id` y `detail.reason_name`.
- Precuenta y ticket (etapa 7.1): cada ajuste del documento agrega `reason` (nombre del motivo; se imprime «Concepto · Motivo», por ejemplo «Descuento manual · Atención»). Con la función `usd_cash` activa y tipo de cambio disponible, el documento agrega `usd_total` y `usd_rate` y la línea informativa «Total en dólares (T.C. $Y)» con `US$X` (`mxnToUSD`: total en pesos entre el tipo de cambio, redondeado a centavos); la precuenta usa el turno activo (abierto o en arqueo) de la caja de la terminal que la consulta o imprime y el ticket el turno de su pago. Sin la función, sin caja, sin turno o sin tipo de cambio, los campos se omiten y la línea no aparece. No cambia ningún importe; la reimpresión conserva el contenido original.
- Devolución: `{operation_id, mode: "total"|"payment"|"items", payment_id?, amount?, tip?, selections?, method_id, reference?, reason, authorization?}`. Corrección: `{operation_id, payment_id, method_id, reference?, reason, authorization?}`.
- `finance`: totales (`items_total`, `item_discounts`, `account_discounts`, `charges`, `total`, `paid`, `refunded`, `tips`, `balance`), `payment_state`, `delivery_state`, `settled`, `tax[{rate, base, tax, total}]`, `lines[]` por versión con unidades y su estado (`unpaid`, `partial`, `settled`), `payments[]`, `refunds[]`, `adjustments[]`, `documents[]` y `delivery`.

Las cuentas del restaurante incluyen `finance: {total, paid, balance, tips, payment_state, delivery_state}`; `payment_state` es `not_recorded`, `partial` o `paid` y `state` admite `closed`. El listado acepta `state=closed`.

Errores específicos: `SHIFT_CLOSING`, `SHIFT_CHANGED` (409, etapa 7; ver la sección de caja); `FEATURE_DISABLED`, `USD_RATE_REQUIRED`, `USD_RATE_CONFIRMATION_REQUIRED`, `USD_RATE_ALREADY_SET` (409, etapa 6.1); `USD_RATE_INVALID`, `USD_RATE_OUT_OF_RANGE` (400, etapa 6.1); `BALANCE_CHANGED`, `NOTHING_TO_COLLECT`, `SELECTION_EXCEEDS_BALANCE`, `REGISTER_REQUIRED`, `SHIFT_REQUIRED`, `SHIFT_NOT_SHARED`, `SHIFT_ALREADY_OPEN`, `SHIFT_CLOSED`, `ACCOUNT_NOT_OPEN`, `UNIT_HAS_PAYMENT`, `UNIT_HAS_DISCOUNT`, `ACCOUNT_BELOW_PAYMENTS`, `ACCOUNT_HAS_PAYMENTS`, `NOTHING_TO_REFUND`, `PAYMENT_NOT_CORRECTABLE`, `UNIT_NOT_DISCOUNTABLE`, `ADJUSTMENT_VOIDED`, `ALREADY_DELIVERED`, `BALANCE_PENDING`, `PRINTER_NOT_CONFIGURED`, `PRINT_COPY_PENDING` (409); `PAYMENT_VALIDATION` (400); `AUTHORIZATION_REQUIRED`, `AUTHORIZATION_INSUFFICIENT`, `SELF_AUTHORIZATION` (403); `AUTHORIZATION_INVALID` (401, no invalida la sesión); `AUTHORIZATION_THROTTLED` (429). Los errores de guardas financieras dentro de operaciones del restaurante (`restaurant/*`) y los de cocina conservan su código público en lugar de un 503.

Roles: `POST/PATCH /roles` aceptan `discount_limit` opcional (porcentaje `0`–`100`, cadena vacía para eliminarlo); nadie asigna un límite mayor que el suyo.

## Caja, arqueos y cortes · esquema 8 (etapa 7, fases A, B y C)

Mismo prefijo `/api/v1/payments`, misma transacción autenticada, lock 7820148305 (compartido en lecturas, exclusivo en escrituras), idempotencia por `operation_id` en `restaurant_operations`, auditoría y evento en la misma transacción. Cada ruta lleva como máximo un identificador (requisito del puente nativo); los registros relacionados viajan en el cuerpo. Reglas completas en [CASH_SHIFTS_AND_RECONCILIATION.md](docs/CASH_SHIFTS_AND_RECONCILIATION.md).

| Método | Ruta bajo `/payments`                       | Permiso y contrato                                                                                                                                                                                   |
| ------ | ------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET    | `/cash/catalog`                             | `payments.read`; `{denominations[], expense_categories[], income_types[], withdrawal_types[]}`                                                                                                       |
| POST   | `/cash/denominations`                       | `cash.settings.manage`; `{operation_id, currency: "MXN"\|"USD", value, kind: "bill"\|"coin", active}`; 409 `DENOMINATION_EXISTS`                                                                     |
| PATCH  | `/cash/denominations/{id}`                  | `cash.settings.manage`; `{operation_id, revision, active}` (moneda, valor y tipo son inmutables)                                                                                                     |
| POST   | `/cash/expense-categories`                  | `cash.settings.manage`; `{operation_id, name, active, position?: {after}}`                                                                                                                           |
| PATCH  | `/cash/expense-categories/{id}`             | `cash.settings.manage`; `{operation_id, revision, name, active, position?}`                                                                                                                          |
| GET    | `/cash/registers`                           | `cash.shifts.read`; `[{register, shift, own}]` con el turno activo; otras cajas con `cash.shifts.read_all`                                                                                           |
| GET    | `/shifts?register_id&from&until&state&page` | `cash.history.read`; historial por fecha comercial (`AAAA-MM-DD`), 50 por página, `{items[{shift, closure}], page, page_size, total}`; sin `read_all`, solo la caja propia                           |
| GET    | `/shifts/{id}`                              | `cash.shifts.read`; `{shift, opening_count, reports[], summary, expected_visible, expected?}`                                                                                                        |
| GET    | `/shifts/{id}/operations`                   | `cash.movements.read`; `{shift, operations[]}` (ingresos, retiros, gastos, correcciones y regularizaciones con su estado)                                                                            |
| POST   | `/shifts/{id}/income`                       | `cash.income.record`; sin segunda autorización                                                                                                                                                       |
| POST   | `/shifts/{id}/withdrawals`                  | `cash.withdrawals.record` + autorización (`cash.outflows.authorize` propio o de otra persona)                                                                                                        |
| POST   | `/shifts/{id}/expenses`                     | `cash.expenses.record` + autorización (igual que retiros)                                                                                                                                            |
| POST   | `/shifts/{id}/corrections`                  | `cash.corrections.record` + autorización (`cash.corrections.authorize` propio o de otra persona); `{operation_id, corrects_operation_id, reason, authorization?}`                                    |
| POST   | `/shifts/{id}/partial-cuts`                 | `cash.partial_cuts.create`; `{operation_id}`; respuesta sin bloque sensible                                                                                                                          |
| GET    | `/cash-reports/{id}`                        | `cash.shifts.read`; corte parcial o definitivo; `snapshot.sensitive` solo con `cash.expected.read` y fuera del arqueo                                                                                |
| POST   | `/shifts/{id}/closing`                      | `cash.counts.perform`; `{operation_id}`; inicia el arqueo («Cierre en curso») y crea el borrador del conteo                                                                                          |
| GET    | `/shifts/{id}/closing`                      | `cash.counts.perform`; `{shift, count}`; solo lo declarado                                                                                                                                           |
| PUT    | `/shifts/{id}/closing/count`                | `cash.counts.perform`; `{operation_id, revision, lines[{denomination_id, quantity}], extra_mxn?, extra_usd?, extra_note?, notes?}`; 409 `COUNT_CHANGED`                                              |
| POST   | `/shifts/{id}/closing/cancel`               | `cash.counts.perform`; arqueo ajeno exige `cash.closing.cancel_any`; `{operation_id, reason?}`                                                                                                       |
| POST   | `/shifts/{id}/closing/confirm`              | `cash.shifts.close`; `{operation_id, count_revision}`; respuesta `{shift, count, final_report_id, closed}` sin esperado ni diferencias                                                               |
| GET    | `/shifts/{id}/closure`                      | `cash.shifts.read` (y `cash.history.read` salvo quien cerró); `expected`, `differences` y `comments` solo con `cash.expected.read` (comentarios también con `cash.closures.review`)                  |
| POST   | `/shifts/{id}/closure/comments`             | `cash.closures.review`; `{operation_id, comment}`                                                                                                                                                    |
| POST   | `/cash/count-preview`                       | Solo lectura (fase B). `payments.read` y, sin `shift_id`, `cash.shifts.open`; con `shift_id`, `cash.counts.perform` y acceso al turno; no escribe ni audita                                          |
| GET    | `/shifts/{id}/summary`                      | Solo lectura (fase B). `cash.shifts.read` y reglas de acceso del turno; resumen en vivo con los totales de un corte, sin registrarlo                                                                 |
| GET    | `/cash-reports/{id}/preview`                | Fase C. `cash.shifts.read` y acceso al turno; vista previa del corte renderizada por Go, sin escribir                                                                                                |
| POST   | `/cash-reports/{id}/print`                  | Fase C. `cash.reports.print`; `{operation_id}`; imprime el corte que aún no tiene documento (409 `CASH_DOCUMENT_EXISTS`, `PRINTER_NOT_CONFIGURED`)                                                   |
| GET    | `/cash-operations/{id}/preview`             | Fase C. `cash.movements.read` y acceso al turno; vista previa del comprobante de ingreso, retiro o gasto (409 `CASH_DOCUMENT_UNSUPPORTED` en correcciones)                                           |
| POST   | `/cash-operations/{id}/print`               | Fase C. `cash.reports.print`; `{operation_id}`; imprime el comprobante que aún no tiene documento                                                                                                    |
| POST   | `/cash-documents/{id}/reprint`              | Fase C. `cash.reports.reprint` (y `cash.expected.read` fuera del arqueo si el documento incluye el esperado); `{operation_id}`; copia idéntica (`PRINT_COPY_PENDING`)                                |
| GET    | `/shifts/{id}/documents`                    | Fase C. `cash.shifts.read` y acceso al turno; `{shift_id, documents[]}` con el estado del último trabajo                                                                                             |
| GET    | `/shifts/{id}/rejected-outflows`            | Etapa 7.1. `cash.expected.read` y acceso al turno; `{shift_id, visible, message?, rejections[]}`; retiros y gastos rechazados por efectivo insuficiente; vacío (`visible: false`) durante el arqueo  |
| GET    | `/cash/drawers`                             | Fase C. `cash.settings.manage`; `{drawers[], printers[{id, name, transport, active, compatible, connection?, reason?}]}` (etapa 7.1: colas CUPS consultadas con lpstat y lpoptions)                  |
| PUT    | `/cash/drawers/{caja}`                      | Fase C. `cash.settings.manage`; `{operation_id, revision, printer_id, pulse_pin: 0\|1, enabled}`; 400 `DRAWER_PROTOCOL_UNSUPPORTED` (agente), `DRAWER_QUEUE_NOT_RAW`; 409 `DRAWER_QUEUE_UNAVAILABLE` |
| POST   | `/cash/drawers/{caja}/test`                 | Etapa 7.1. `cash.settings.manage`; `{operation_id}`; «Probar apertura» con la configuración guardada; `DrawerOutcome`; 409 `DRAWER_NOT_CONFIGURED`, `DRAWER_UNAVAILABLE`; 400 `DRAWER_QUEUE_NOT_RAW` |
| POST   | `/cash/open-drawer`                         | Fase C. `cash.drawer.open`; `{operation_id, reason}` (5–500); cajón de la caja de la terminal; 409 `DRAWER_DISABLED`, `DRAWER_UNAVAILABLE`, `REGISTER_REQUIRED`                                      |
| GET    | `/cash/limit`                               | Fase C. `cash.settings.manage`; `{enabled, usd_cash, cash_limit_mxn, cash_limit_usd, revision}`                                                                                                      |
| PUT    | `/cash/limit`                               | Fase C. `cash.settings.manage`; `{operation_id, revision, cash_limit_mxn?, cash_limit_usd?}` (vacío = sin tope); 409 `FEATURE_DISABLED` sin `cash_limit` o, en dólares, sin `usd_cash`               |

Acceso: un turno de otra caja exige `cash.shifts.read_all`; un turno cerrado exige `cash.history.read` salvo para quien lo cerró. Las operaciones de efectivo se registran solo desde una terminal asociada a la caja del turno, con el turno abierto y la política de turno compartido (409 `SHIFT_NOT_IN_REGISTER`, `SHIFT_NOT_SHARED`).

### Cuerpos principales

- Apertura (`POST /shifts`): `opening_float` obligatorio (puede ser `"0"`); `opening_float_usd` solo con `usd_cash`; `opening_notes` hasta 2000; `count: {lines: [{denomination_id, quantity}], extra_mxn?, extra_usd?, extra_note?}` opcional: Go calcula el total por moneda y exige que coincida con los fondos (400 `COUNT_TOTAL_MISMATCH`). `shift` agrega `opening_float_usd`, `opening_notes`, `business_date`, `closing_started_by`, `closing_started_by_name`, `closing_started_at`, `closing_device_name`, `closed_by`, `closed_by_name` y `closed_device_name`.
- Ingreso / retiro / gasto: `{operation_id, currency?: "MXN"|"USD", amount, concept?, type?, category_id?, reference?, notes?, exceptional?, reason?, authorization?: {username, password}}`. `type` es obligatorio en ingresos (`float_contribution`, `change_supply`, `cash_return`, `other`) y retiros (`safekeeping`, `partial_withdrawal`, `admin_delivery`, `transfer`, `other`); `category_id` (activa) es obligatoria en gastos; el concepto predeterminado es el tipo o la categoría. Dólares exigen `usd_cash` y el tipo de cambio del turno. Una salida que excede el efectivo esperado de su moneda responde 409 `CASH_INSUFFICIENT` con un mensaje sin cifras, salvo `exceptional: true` con `reason` (5–500) y autorización de alguien con `cash.outflows.authorize` y `cash.expected.read`. Respuesta: la operación (`CashOperation`): `{id, shift_id, register_name, kind, currency, amount, direction, concept, type, type_label, category_name, reference, notes, reason, corrects_operation_id, corrected_by_operation_id, regularization, exceptional, usd_rate, equivalent_mxn, actor_name, device_name, authorized_by_name, authorization_mode: "self"|"supervisor"|null, status: "recorded"|"corrected"|"correction"|"regularization", created_at}`.
- Corrección: invierte exactamente la original (misma moneda e importe, dirección opuesta) en el turno de la ruta; si la original pertenece a un turno cerrado de la misma caja, es una regularización (`regularization: true`) que no cambia la diferencia del cierre ni el esperado del turno vigente. Respuesta `{correction, original}`. 409 `OPERATION_ALREADY_CORRECTED`, `CORRECTION_SHIFT`, `REGULARIZATION_REGISTER`.
- Corte: `{id, shift_id, kind: "partial"|"final", sequence, previous_report_id, covered_from, covered_to, created_by_name, device_name, created_at, expected_visible, snapshot: {control, accumulated, since_previous, declared?, sensitive?: {expected, differences?}}}`. `accumulated`/`since_previous`: `{payments, refunds, payment_corrections, cash_operations, regularizations, sales, tips, refunded, discounts, discount_count, courtesies, courtesy_count, methods[{method_name, is_cash, sales, tips, refunds, net}], cash[{currency, income, withdrawals, expenses, corrections_in, corrections_out}]}`.
- Esperado (`CashExpectation`): `{mxn, usd, usd_rate, usd_equivalent_mxn, total_equivalent_mxn}`. Diferencias (`CashDifferences`): `{mxn, usd, result_mxn, result_usd: "even"|"over"|"short", equivalent_mxn}` (contado − esperado).

- Vista previa del conteo (fase B): `{shift_id?, lines: [{denomination_id, quantity}], extra_mxn?, extra_usd?, extra_note?}` → `{lines: [{denomination_id, currency, kind, value, quantity, amount}], extra_mxn, extra_usd, extra_note, counted_mxn, counted_usd}`. Usa la misma validación que el borrador del arqueo y el desglose de la apertura (`evaluateCount`): denominación inexistente, inactiva o repetida, cantidades fuera de 0–100000 e importe adicional sin identificar responden 400; dólares sin la función (y sin un turno que ya los maneje) 409 `FEATURE_DISABLED`. Nunca contiene el esperado ni un indicador de cercanía; se puede usar durante el arqueo.
- Resumen del turno (fase B): `{shift, generated_at, blind, control, partial_cuts, last_cut, accumulated?, since_last_cut?, expected_visible, expected?, physical?: {mxn_in, mxn_out, usd_in, usd_out}, closure?}`. `accumulated` y `since_last_cut` tienen la forma de los totales del corte y se calculan con la misma función (desde el último corte parcial = acumulado actual − acumulado de ese corte). `expected` (`CashExpectation`) y `physical` (efectivo recibido y entregado en cobros por moneda) solo con `cash.expected.read`; durante `closing` (`blind: true`) se omiten además los totales. En un turno cerrado agrega `closure` (`ClosureSummary`, con diferencias solo con permiso).

- Impresión (fase C): `print?: boolean` opcional (parte de la huella; exige `cash.reports.print`) en ingresos, retiros, gastos, `POST /shifts/{id}/partial-cuts` y `POST /shifts/{id}/closing/confirm`; sin él no se imprime. Respuestas: la operación agrega `printing` y `drawer`; el corte parcial y la confirmación, `printing`; el cobro (`POST /accounts/{id}/payments`) y la devolución agregan `drawer`. `PrintOutcome`: `{requested, status, document_id, ticket_id, job_id, includes_expected, message}` con `status` del trabajo (`pending`, `intervention`…), `not_configured` o `failed`; nunca significa que el papel salió. `DrawerOutcome`: `{requested, status, job_id, event_id, message}` con `status` del trabajo, `disabled`, `unavailable` o `failed`; nunca significa que el cajón se abrió. Ninguno revierte la operación ni el cierre.
- Vista previa (fase C): `{kind: "partial_cut"|"final_cut"|"income"|"withdrawal"|"expense", title, content, includes_expected, paper_width, document}`. Documento: `{id, kind, shift_id, report_id, cash_operation_id, ticket_id, includes_expected, created_by_name, created_at, job_id, job_state, printer_name, copies}`. El esperado y las diferencias solo se incluyen si quien genera el documento tiene `cash.expected.read` y el turno no está en arqueo.
- Tope de caja (fase C): `GET /shifts/current`, `GET /shifts/{id}` y `GET /shifts/{id}/summary` agregan `cash_limit_warning: {currencies, message}` solo con la función `cash_limit` activa, el esperado visible (`cash.expected.read`, turno abierto) y el esperado por encima del tope de su moneda. Nunca bloquea cobros. `GET /cash/registers` agrega `drawer_enabled` por caja.

Ciego: mientras el turno está en `closing`, ninguna respuesta (turno, movimientos, operaciones, cortes, arqueo, historial, cajas) contiene el efectivo esperado ni diferencias, ni siquiera para el administrador; las escrituras nunca los devuelven (su respuesta se guarda para reintentos) y la auditoría tampoco los registra.

- Cajón por CUPS (etapa 7.1, modo mixto): el cajón admite una impresora `escpos_tcp` o `cups`. Para una cola CUPS, Go consulta con `lpstat -p` y `lpoptions -p` (nunca imprime para averiguarlo) y la acepta solo si admite datos raw: conexión `usb`, `serial`, `parallel` o `socket`, sin ser una clase, una cola remota ni una impresora sin controlador (AirPrint o IPP Everywhere). `printers[].connection` es la conexión que informa CUPS y `reason` explica el rechazo; el PUT lo exige solo si `enabled` es verdadero (desactivar nunca depende de CUPS). El pulso se envía con `/usr/bin/lp -d <cola> -o raw -n 1 -t ADEL-cajon-<trabajo> <archivo>`, con argumentos separados, y la cola se vuelve a consultar antes de cada envío (si ya no admite datos raw, el trabajo queda en `intervention` sin enviar). Mismas reglas de la etapa 7: solo con efectivo físico, sin reintentos automáticos de un error o de un resultado incierto, «Comando de apertura enviado».
- Probar apertura (etapa 7.1): `POST /cash/drawers/{caja}/test` envía un pulso con la impresora y el conector guardados, aunque el cajón aún no esté activado; queda en `cash_drawer_events` con `reason: "test"` (migración 0009), ligado al turno activo de la caja si existe, se audita como `cash.drawer_tested` y publica `cash.drawer.command_queued`. Idempotente por `operation_id`.
- Rechazos por efectivo insuficiente (etapa 7.1): cada retiro o gasto rechazado con `CASH_INSUFFICIENT` revierte la operación y deja solo un registro de auditoría (`cash.outflow_rejected`, entidad `cash_shift`, `success: false`, usuario y terminal de la sesión) con `{register_id, shift_id, device_id, kind, currency, attempted_amount, authorization_mode, authorized_by, operation_id}`; nunca el esperado. La respuesta al cajero no cambia (sin cifras). `GET /shifts/{id}/rejected-outflows` devuelve `rejections[{id, occurred_at, kind: "withdrawal"|"expense", currency, attempted_amount, actor_name, device_name, register_name, authorization_mode, authorized_by_name}]` en orden cronológico.

Errores de caja: `SHIFT_CLOSING`, `SHIFT_CLOSED`, `SHIFT_CHANGED`, `SHIFT_ALREADY_OPEN`, `SHIFT_NOT_CLOSING`, `SHIFT_NOT_CLOSED`, `SHIFT_NOT_IN_REGISTER`, `SHIFT_NOT_SHARED`, `CASH_INSUFFICIENT`, `COUNT_CHANGED`, `CLOSING_MOVEMENTS_CHANGED`, `OPERATION_ALREADY_CORRECTED`, `CORRECTION_SHIFT`, `REGULARIZATION_REGISTER`, `DENOMINATION_EXISTS`, `DUPLICATE_NAME`, `REVISION_CONFLICT`, `USD_RATE_REQUIRED`, `FEATURE_DISABLED`, `CASH_DOCUMENT_EXISTS`, `CASH_DOCUMENT_UNSUPPORTED`, `PRINTER_NOT_CONFIGURED`, `PRINT_COPY_PENDING`, `DRAWER_DISABLED`, `DRAWER_UNAVAILABLE`, `DRAWER_NOT_CONFIGURED`, `DRAWER_QUEUE_UNAVAILABLE` (409); `COUNT_TOTAL_MISMATCH`, `PAYMENT_VALIDATION`, `DRAWER_PROTOCOL_UNSUPPORTED`, `DRAWER_QUEUE_NOT_RAW` (400); autorizaciones como en cobros (403/401/429).

Eventos SSE en el ámbito `cash` (sin importes; los reciben `cash.shifts.read` y `payments.read`): `cash.shift.opened`, `cash.shift.usd_rate_set`, `cash.operation.recorded`, `cash.report.created`, `cash.shift.closing_started`, `cash.closing.count_saved`, `cash.shift.closing_cancelled`, `cash.shift.closed`, `cash.closure.commented`, `cash.settings.changed` y, desde la fase C, `cash.movement.recorded` (cobros, devoluciones y correcciones de pago del turno), `cash.document.printed`, `cash.document.reprinted`, `cash.drawer.command_queued` y `cash.print.updated` (resultado de un trabajo de caja en la cola; identifica el trabajo). `cash.settings.changed` se publica también al configurar el cajón o el tope y al activar o desactivar `cash_limit` o `usd_cash`. Todos se escriben en la transacción de su operación.

## Inventario · fase A: núcleo (esquema 10)

Prefijo `/api/v1/inventory`, misma transacción autenticada y callback de auditoría. Toda ruta exige `inventory.read` además del permiso indicado, y todos los permisos se comprueban antes de escribir (un 403 revierte). Lock 7820148308: compartido en lecturas, exclusivo en escrituras (se toma después de 7820148305 y 7820148304 cuando concurren). **Toda escritura exige `operation_id`** (UUID en el cuerpo) y se guarda en `restaurant_operations` con su huella: un reintento con el mismo cuerpo devuelve la respuesta guardada sin repetir efectos y uno con otro cuerpo responde 409 `IDEMPOTENCY_CONFLICT`. Las entidades administrables usan además `revision` (409 `REVISION_CONFLICT`). Un solo identificador por ruta (requisito del puente nativo). Cantidades y dinero son cadenas decimales: cantidades con hasta 6 decimales en la unidad base del insumo y sin ceros finales (`"20"`, `"0.25"`); valores exactos con al menos dos decimales (`"500.00"`, `"33.3333333333"`) y su versión para mostrar `*_display` (valores a centavos, costos unitarios a cuatro decimales). Las capturas aceptan cantidades sin signo con hasta 6 decimales y costos con hasta 10. Los costos y valores solo se entregan con `inventory.costs.read` (si falta, llegan en `null` y la respuesta trae `costs_visible: false`); la valuación total solo con `inventory.valuation.read` (`valuation_visible`). La valuación por insumo (`value` de una existencia y `valuation_value_after` de un movimiento) exige `inventory.costs.read` **e** `inventory.valuation.read`, porque con existencias o con movimientos consecutivos cualquiera de los dos revelaría el otro. `inventory.stock.read` nunca implica costos.

| Método | Ruta bajo `/inventory`                                                                 | Permiso (además de `inventory.read`)                                            | Contrato                                                                                                                                                                                                                                                                      |
| ------ | -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET    | `/summary`                                                                             | —                                                                               | `Summary`                                                                                                                                                                                                                                                                     |
| GET    | `/settings`                                                                            | —                                                                               | `Settings`                                                                                                                                                                                                                                                                    |
| PUT    | `/settings`                                                                            | `inventory.settings.manage`                                                     | `{operation_id, revision, purchase_tax_in_cost, operating_day_cutoff: "HH:MM", warn_products_without_recipe, count_frequency: "none"\|"daily"\|"weekly"\|"monthly"\|"custom", count_interval_days: 1–365 solo con custom, si no null}` → `Settings`                           |
| GET    | `/units`                                                                               | —                                                                               | `[Unit]` (activas e inactivas, por dimensión y equivalencia)                                                                                                                                                                                                                  |
| POST   | `/units`                                                                               | `inventory.items.manage`                                                        | `{operation_id, code, name, dimension: "mass"\|"volume"\|"count"\|"portion", factor, active}` → `Unit`; 409 `DUPLICATE_CODE`, `DUPLICATE_NAME`                                                                                                                                |
| PATCH  | `/units/{id}`                                                                          | `inventory.items.manage`                                                        | `{operation_id, revision, name, factor, active}` (abreviatura y dimensión no cambian: 409 `UNIT_IMMUTABLE`) → `Unit`; 409 `UNIT_SYSTEM`, `UNIT_IN_USE`                                                                                                                        |
| GET    | `/categories`                                                                          | —                                                                               | `[Category]` en su orden visual                                                                                                                                                                                                                                               |
| POST   | `/categories`                                                                          | `inventory.items.manage`                                                        | `{operation_id, name, active, position?: {after}}` → `Category`; 409 `DUPLICATE_NAME`                                                                                                                                                                                         |
| PATCH  | `/categories/{id}`                                                                     | `inventory.items.manage`                                                        | `{operation_id, revision, name, active, position?}` → `Category`                                                                                                                                                                                                              |
| GET    | `/items?search&active&category_id&kind&page&page_size`                                 | —                                                                               | `{items: [Item], total, page, page_size, costs_visible}`; búsqueda por nombre, código o código de barras (también de presentaciones); `category_id=none` sin categoría; 25 por página (máximo 100)                                                                            |
| POST   | `/items`                                                                               | `inventory.items.manage` (+ `inventory.costs.manage` si envía `reference_cost`) | `ItemInput` → `Item`; 409 `DUPLICATE_CODE`, `DUPLICATE_NAME`, `DUPLICATE_BARCODE`, `CATEGORY_INACTIVE`, `UNIT_INACTIVE`                                                                                                                                                       |
| GET    | `/items/{id}`                                                                          | —                                                                               | `Item` con sus presentaciones                                                                                                                                                                                                                                                 |
| PATCH  | `/items/{id}`                                                                          | `inventory.items.manage` (+ `inventory.costs.manage` si envía `reference_cost`) | `ItemInput` con `revision` → `Item`; 409 `BASE_UNIT_LOCKED` (con movimientos), `BASE_UNIT_HAS_PRESENTATIONS`, `REFERENCE_COST_REQUIRED` (cambio de unidad con costo sin recapturarlo)                                                                                         |
| POST   | `/items/{id}/presentations`                                                            | `inventory.items.manage`                                                        | `{operation_id, name, quantity, unit_id?, barcode?, active, purchase_default}` → `Item` completo; 409 `DUPLICATE_NAME`, `DUPLICATE_BARCODE`                                                                                                                                   |
| PATCH  | `/presentations/{id}`                                                                  | `inventory.items.manage`                                                        | igual con `revision` → `Item` completo                                                                                                                                                                                                                                        |
| GET    | `/warehouses`                                                                          | —                                                                               | `[Warehouse]` (el predeterminado primero)                                                                                                                                                                                                                                     |
| POST   | `/warehouses`                                                                          | `inventory.warehouses.manage`                                                   | `{operation_id, name, description?, active}` → `Warehouse`; 409 `DUPLICATE_NAME`                                                                                                                                                                                              |
| PATCH  | `/warehouses/{id}`                                                                     | `inventory.warehouses.manage`                                                   | `{operation_id, revision, name, description?, active}` → `Warehouse`; 409 `DEFAULT_WAREHOUSE`, `WAREHOUSE_HAS_STOCK`, `WAREHOUSE_ASSIGNED`                                                                                                                                    |
| GET    | `/station-warehouses`                                                                  | —                                                                               | `[StationWarehouse]` (todas las estaciones de cocina)                                                                                                                                                                                                                         |
| PUT    | `/station-warehouses/{estación}`                                                       | `inventory.warehouses.manage`                                                   | `{operation_id, revision, warehouse_id: uuid\|null}` (null = almacén predeterminado; revisión = asignaciones registradas) → `StationWarehouse`; repetir la vigente no agrega historial; 409 `WAREHOUSE_INACTIVE`                                                              |
| GET    | `/stock?warehouse_id&category_id&search&state&active&page&page_size`                   | `inventory.stock.read`                                                          | `StockPage`; `state`: `negative`, `zero`, `positive` (existencia filtrada), `below_minimum`, `reorder` (existencia total)                                                                                                                                                     |
| GET    | `/movements?item_id&warehouse_id&kind&source_kind&source_id&from&until&page&page_size` | `inventory.movements.read`                                                      | `{items: [Movement], total, page, page_size, costs_visible, valuation_visible}`, del más reciente (hora de negocio) al más antiguo; `from`/`until` `AAAA-MM-DD` en la zona del establecimiento (días completos); sin `inventory.costs.read` no se listan los ajustes de costo |
| POST   | `/initial-balances`                                                                    | `inventory.adjustments.authorize` + `inventory.costs.manage`                    | `InitialBalanceInput` → `InitialBalanceResult`; 409 `INITIAL_BALANCE_EXISTS`, `REFERENCE_COST_MISSING`, `ITEM_INACTIVE`, `WAREHOUSE_INACTIVE`, `PRESENTATION_INACTIVE`                                                                                                        |

### Cuerpos y respuestas

- `Unit`: `{id, code, name, dimension, dimension_label, factor, system, active, in_use, revision, updated_at}`. `factor` es la equivalencia con la unidad base de su dimensión (g, ml, pza y porción = 1; kg y l = 1000). Sembradas e inmutables: `g`, `kg`, `ml`, `l`, `pza`, `porcion`. `in_use`: es la unidad base de algún insumo (su equivalencia ya no cambia y no se desactiva si un insumo activo la usa). Paquete y caja no son unidades: son presentaciones.
- `Category`: `{id, name, active, sort_order, item_count, revision, updated_at}`.
- `ItemInput`: `{operation_id, revision?, code (1–40), barcode?, name (1–100), description? (≤ 2000), category_id?, base_unit_id, kind?: "purchased"|"prepared", active, reference_cost?: string|null, min_stock?: string|null, reorder_point?: string|null}`. Omitir `reference_cost` conserva el costo vigente; enviarlo (cadena > 0 o null) exige `inventory.costs.manage`. El costo de referencia, la existencia mínima y el punto de reabastecimiento se expresan por la unidad base.
- `Item`: `{id, code, barcode, name, description, category_id, category_name, base_unit: {id, code, name, dimension}, kind, kind_label, active, costs_visible, reference_cost, reference_cost_display, min_stock, reorder_point, has_movements, presentations: [Presentation], revision, created_at, updated_at, updated_by_name}`.
- `Presentation`: `{id, item_id, name, base_quantity, barcode, active, purchase_default, revision, updated_at}`; `base_quantity` está en la unidad base del insumo (la captura `quantity` + `unit_id` opcional de la misma dimensión se convierte en Go). Una sola presentación activa es la de compra predeterminada: marcar otra quita la marca a la anterior.
- `Warehouse`: `{id, name, description, active, is_default, stations: [{id, name}], revision, updated_at}` (`stations`: estaciones activas que hoy consumen de él; las que no tienen asignación cuentan en el predeterminado). «Almacén general» es el predeterminado sembrado.
- `StationWarehouse`: `{station_id, station_name, station_active, warehouse_id, warehouse_name, uses_default, effective_from, assigned_by_name, revision, history: [{warehouse_id, warehouse_name, effective_from, actor_name}]}` (historial de solo inserción, el más reciente primero, hasta 20).
- `StockPage`: `{items: [StockRow], total, page, page_size, warehouse: {id, name}|null, costs_visible, valuation_visible, total_value, total_value_display}`. `StockRow`: `{item_id, item_code, item_name, category_name, unit, active, quantity (en el almacén filtrado o total), total_quantity, negative, min_stock, reorder_point, below_minimum, at_reorder_point, warehouses: [{warehouse_id, warehouse_name, quantity, negative}], average_cost, average_cost_display, value, value_display, cost_pending, updated_at}`. `value` es el valor total del insumo (el costo es uno solo por insumo) y solo llega con `costs_visible` y `valuation_visible`; `average_cost` y `cost_pending` llegan con `costs_visible`; `total_value` es la valuación de todo el inventario (`valuation_visible`).
- `Movement`: `{id, seq, item_id, item_code, item_name, unit, warehouse_id, warehouse_name, kind, kind_label, quantity (con signo), stock_after, valuation_quantity_after, costs_visible, value, value_display, unit_cost, unit_cost_display, cost_basis, cost_basis_label, cost_pending, valuation_visible, valuation_value_after, occurred_at, recorded_at, source_kind, source_kind_label, source_id, source_line, restaurant_unit_id, related_movement_id, reason, actor_name, process, process_label, operation_id}`. Tipos: `initial`, `receipt`, `consumption`, `consumption_reversal`, `waste`, `adjustment_in`, `adjustment_out`, `transfer_out`, `transfer_in`, `supplier_return`, `production_in`, `production_out`, `count_in`, `count_out`, `cost_adjustment` (solo valor, sin almacén). Bases de costo: `average`, `reference`, `configured`, `pending`, `document`, `original`, `provisional`, `difference`; `production_in` usa `document` (entrada con documento: el valor de lo consumido) y `consumption_reversal` usa `original`. `value`, `unit_cost`, `cost_basis` y `cost_pending` llegan con `costs_visible`; `valuation_value_after`, solo con `valuation_visible` (`inventory.costs.read` e `inventory.valuation.read`). La traza de consumo (`GET /consumption/{unidad}`) entrega los mismos `Movement`. En la fase A solo existen `initial` (por API) y lo que registran los procesos de Go.
- `InitialBalanceInput`: `{operation_id, warehouse_id, occurred_at?: RFC 3339 (no futura; por omisión la hora del servidor), reason?: (≤ 500), lines: [{item_id, quantity, unit_id? | presentation_id?, unit_cost? | use_reference_cost: true}]}` (1–500 renglones, sin insumos repetidos). `unit_cost` es por unidad capturada (por caja si se captura en cajas); `use_reference_cost: true` valora la cantidad base al costo de referencia configurado y es excluyente con `unit_cost`. Solo procede si el insumo no tiene movimientos en ese almacén. `InitialBalanceResult`: `{operation_id, warehouse: {id, name}, occurred_at, reason, costs_visible, lines: [{line, item_id, item_code, item_name, unit, captured_quantity, captured_unit_label, quantity, stock_after, movement_id, use_reference_cost, unit_cost, unit_cost_display, value, value_display}], warnings: [{code, message, item_id}]}`.
- `Settings`: `{purchase_tax_in_cost, operating_day_cutoff, warn_products_without_recipe, count_frequency, count_frequency_label, count_interval_days, revision, updated_at, updated_by_name}`.
- `Summary`: `{feature: {code, name, enabled, enabled_at, revision}, active_items, inactive_items, active_warehouses, negative_stock, below_minimum, at_reorder_point, costs_visible, pending_cost_items, valuation_visible, total_value, total_value_display}`.

Errores de inventario: `INVENTORY_VALIDATION` (400, mensaje en español sin costos), `PERMISSION_DENIED` (403), `NOT_FOUND` (404) y los 409 de cada ruta, más `REVISION_CONFLICT` e `IDEMPOTENCY_CONFLICT`. Una existencia negativa nunca bloquea: el libro la informa como aviso y las operaciones de las fases siguientes que puedan producirla (consumos, ajustes, transferencias) la expondrán en `warnings` (`NEGATIVE_STOCK`); en la fase A la existencia inicial no puede producirla, por lo que `warnings` llega vacío.

Eventos SSE en el ámbito `inventory` (sin costos ni importes; los recibe `inventory.read`): `inventory.items.changed` (unidades, categorías, insumos y presentaciones), `inventory.settings.changed` (configuración, almacenes, asignaciones de estaciones y la función `inventory_control`), `inventory.stock.changed` y `inventory.movements.recorded`. Auditoría en la misma transacción: `inventory.unit_created|updated`, `inventory.category_created|updated`, `inventory.categories_reordered`, `inventory.item_created|updated`, `inventory.presentation_created|updated`, `inventory.warehouse_created|updated`, `inventory.station_warehouse_assigned`, `inventory.initial_balance_recorded` y `inventory.settings_changed`. Los registros de auditoría de inventario pueden contener costos (solo los consulta `audit.read`).

## Inventario · fase B: recetas y consumo (esquema 11)

Mismas reglas que la fase A: prefijo `/api/v1/inventory`, `inventory.read` en toda ruta además del permiso indicado,
permisos comprobados antes de escribir (un 403 revierte), lock 7820148308 (compartido en lecturas, exclusivo en
escrituras), **toda escritura con `operation_id`** guardada en `restaurant_operations` (reintento con el mismo cuerpo →
misma respuesta; otro cuerpo → 409 `IDEMPOTENCY_CONFLICT`), `revision` en recetas, porciones y cancelaciones (409
`REVISION_CONFLICT`), un solo identificador por ruta, cantidades y dinero como cadenas decimales, costos solo con
`inventory.costs.read` (`null` y `costs_visible: false` si falta). Tipos TS: `desktop/src/services/inventoryRecipes.ts`.

| Método | Ruta bajo `/inventory`                                         | Permiso (además de `inventory.read`)                                                             | Contrato                                                                                                                                                                                                      |
| ------ | -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET    | `/recipes?search&owner_kind&active&product_id&page&page_size`  | —                                                                                                | `{items: [RecipeSummary], total, page, page_size}`; activas primero, luego producto, complemento y preparación                                                                                                |
| POST   | `/recipes`                                                     | `inventory.recipes.manage` (producto, complemento) o `inventory.subrecipes.manage` (preparación) | `RecipeCreateInput` → `Recipe`; 404 producto, variante, complemento o insumo inexistente; 409 `RECIPE_EXISTS`, `DUPLICATE_NAME`, `ITEM_NOT_PREPARED`, `ITEM_INACTIVE`, `PREPARATION_INACTIVE`, `RECIPE_CYCLE` |
| GET    | `/recipes/{id}`                                                | — (costos con `inventory.costs.read`)                                                            | `Recipe`                                                                                                                                                                                                      |
| PUT    | `/recipes/{id}`                                                | el de su tipo                                                                                    | `RecipeVersionInput` → `Recipe` con la versión nueva; 409 `REVISION_CONFLICT`, `RECIPE_TARGET_IMMUTABLE`, `YIELD_DIMENSION_LOCKED`, `PREPARATION_IN_USE`, `RECIPE_CYCLE`                                      |
| GET    | `/recipe-versions/{version_id}`                                | —                                                                                                | `RecipeVersionDetail` (historial, sin costos)                                                                                                                                                                 |
| GET    | `/recipe-targets?search&kind&state&active&page&page_size`      | —                                                                                                | `{items: [RecipeTarget], total, page, page_size}`; `kind`: `product`, `variant`, `complement`; `state`: `with_recipe`, `without_recipe`, `no_preparation`                                                     |
| GET    | `/ingredient-portions?search&active&configured&page&page_size` | —                                                                                                | `{items: [IngredientPortion], total, page, page_size}` (ingredientes del catálogo; `configured` true/false)                                                                                                   |
| PUT    | `/ingredient-portions/{ingredient_id}`                         | `inventory.recipes.manage`                                                                       | `{operation_id, revision, item_id: uuid\|null, quantity?, unit_id?}` → `IngredientPortion`; `item_id` null retira la porción; repetir la vigente no agrega historial; 409 `ITEM_INACTIVE`                     |
| GET    | `/cancellations?state&from&until&page&page_size`               | — (valores con `inventory.costs.read`)                                                           | `{items: [Cancellation], total, pending, page, page_size, costs_visible}`; pendientes primero por vencimiento; `from`/`until` = días operativos `AAAA-MM-DD`                                                  |
| POST   | `/cancellations/{id}/classify`                                 | `inventory.waste.record` hasta el corte; `inventory.adjustments.authorize` después               | `{operation_id, revision, state: "not_prepared"\|"prepared_waste", notes?}` → `Cancellation`; repetir el estado vigente no escribe                                                                            |
| GET    | `/productions?item_id&warehouse_id&from&until&page&page_size`  | `inventory.production.record` o `inventory.movements.read`                                       | `{items: [Production], total, page, page_size, costs_visible}`, la más reciente primero                                                                                                                       |
| POST   | `/productions`                                                 | `inventory.production.record`                                                                    | `ProductionInput` → `Production`; 409 `RECIPE_NOT_STORED`, `RECIPE_INACTIVE`, `PREPARATION_INACTIVE`, `RECIPE_EMPTY`, `WAREHOUSE_INACTIVE`                                                                    |
| GET    | `/consumption-requests?state&page&page_size`                   | `inventory.movements.read`                                                                       | `{items: [ConsumptionRequest], total, pending, failed, page, page_size}`; sin `state`, las no procesadas; `state`: `pending`, `processed`, `failed`, `all`                                                    |
| GET    | `/consumption/{restaurant_unit_id}`                            | `inventory.movements.read` (valores con `inventory.costs.read`)                                  | `UnitConsumptionTrace`                                                                                                                                                                                        |

### Cuerpos y respuestas

- `RecipeLineInput`: `{component_kind: "item"|"preparation"|"ingredient", item_id?, preparation_recipe_id?, ingredient_id?,
quantity, unit_id?, takeaway_only?}`. `item`: insumo; `preparation`: receta de preparación; `ingredient` (solo recetas de
  producto): `ingredient_id` del catálogo con `item_id` **o** `preparation_recipe_id`, sobrescribe la porción habitual de
  ese ingrediente para este producto (se multiplica por las porciones pedidas). `unit_id` por omisión es la unidad base
  del destino y debe ser de su misma dimensión. `takeaway_only` (consumo solo en para llevar y ventanilla) no aplica a
  renglones de ingrediente ni a preparaciones. Hasta 200 renglones; una receta activa requiere al menos uno; un
  ingrediente una sola vez.
- `RecipeCreateInput`: `{operation_id, owner_kind: "product"|"complement"|"preparation", product_id?, variant_id?,
complement_id?, preparation_mode?: "immediate"|"stored", preparation_item_id? (solo stored: insumo de tipo prepared),
name? (vacío: el del producto · variante, complemento o insumo preparado; obligatorio en preparaciones inmediatas),
active, yield_quantity? ("1" por omisión), yield_unit_id? (porción en productos y complementos; por omisión la unidad
base del insumo preparado), production_waste_percent? (solo preparaciones, 0 ≤ x < 100, hasta 4 decimales), notes?
(≤ 1000), lines: [RecipeLineInput]}`.
- `RecipeVersionInput`: `{operation_id, revision (= versión vigente), name?, active, yield_quantity?, yield_unit_id?,
production_waste_percent?, notes?, lines}`. El destino no cambia (enviarlo distinto → `RECIPE_TARGET_IMMUTABLE`);
  desactivar es una versión con `active: false`.
- `RecipeSummary`: `{id, owner_kind, owner_kind_label, name, target_name, product_id, variant_id, complement_id,
preparation_mode, preparation_mode_label, preparation_item_id, active, revision, effective_from, line_count,
yield_quantity, yield_unit: {id, code, name, dimension}, updated_at, updated_by_name}`.
- `Recipe`: `RecipeSummary` + `{product_name, variant_name, complement_name, preparation_item: ItemReference|null,
version: RecipeVersionDetail, versions: [{id, version, active, effective_from, line_count, created_by_name}] (las 50
más recientes), ingredients: [RecipeIngredient] (solo recetas de producto), used_by: [{id, name, owner_kind, active}]
(recetas cuya versión vigente usa la preparación), cost: RecipeCost}`.
- `RecipeVersionDetail`: `{id, recipe_id, version, name, active, yield_quantity, yield_unit, yield_base_quantity,
production_waste_percent, effective_yield, effective_from, notes, created_by_name, lines: [RecipeLine]}`.
- `RecipeLine`: `{line_no, component_kind, component_kind_label, item: ItemReference|null, preparation_recipe_id,
preparation_name, ingredient_id, ingredient_name, quantity, unit, base_quantity, base_unit, takeaway_only, cost,
cost_display, cost_pending}` (costo del renglón por rendimiento, solo en la versión vigente y con `costs.read`).
- `RecipeIngredient`: `{ingredient_id, name, included, removable, addable, portion_source: "recipe"|"habitual"|"none",
item, preparation_name, portion_quantity, included_quantity, cost, cost_display, cost_pending}`.
- `RecipeCost`: `{costs_visible, unit_cost, unit_cost_display, takeaway_cost, takeaway_cost_display, batch_cost,
batch_cost_display, yield_unit_cost, yield_unit_cost_display, cost_pending}`. Producto/complemento: `unit_cost` (renglones
  fijos ÷ rendimiento + ingredientes incluidos × porción) y `takeaway_cost` (renglones «solo para llevar»); preparación:
  `batch_cost` y `yield_unit_cost` (por unidad base del rendimiento). `cost_pending`: algún componente sin costo.
- `ItemReference`: `{id, code, name, kind, active, unit: {id, code, name, dimension}}`.
- `RecipeTarget`: `{kind, product_id, variant_id, complement_id, name, variant_name, category_name, product_kind, active,
recipe_id, recipe_name, recipe_active, inherited_recipe_id, status, kitchen_route_mode, ingredients_total,
ingredients_with_portion, linked_product_name, linked_ingredient_name, configured}`.
- `IngredientPortion`: `{ingredient_id, ingredient_name, catalog_unit, active, configured, item, quantity, unit,
base_quantity, effective_from, updated_by_name, revision (filas del historial), history: [{item_name, quantity,
unit_code, base_quantity, effective_from, actor_name}] (hasta 20)}`.
- `Cancellation`: `{id, restaurant_unit_id, product_name, account_folio, account_name, cancelled_at, operating_date
(AAAA-MM-DD), deadline_at, expired, state, state_label, resolution: "manual"|"automatic"|"correction"|null,
resolution_label, classified_by_name, classified_at, notes, revision, required_permission, can_classify, costs_visible,
lines: [ConsumptionLine]}`. `expired`: pasó el corte o ya la resolvió el reconciliador.
- `ConsumptionLine`: `{item, warehouse: {id, name}, consumed_quantity (liberación y modificaciones), net_quantity (tras
clasificaciones), value, value_display}`.
- `ProductionInput`: `{operation_id, recipe_id (almacenable activa), warehouse_id? (por omisión «Almacén general»),
batches, produced_quantity? (por omisión lo esperado, en la unidad base del insumo preparado), occurred_at? (RFC 3339, no
futura), notes? (≤ 500)}`.
- `Production`: `{id, recipe_id, recipe_name, recipe_version, item, warehouse, batches, expected_quantity,
produced_quantity, waste_quantity, occurred_at, notes, actor_name, costs_visible, consumed_value,
consumed_value_display, unit_cost, unit_cost_display, lines: [{item, quantity, stock_after, value, value_display}],
warnings: [{code: "NEGATIVE_STOCK", message, item_id}]}` (`lines` y `warnings` solo en la respuesta del registro).
- `ConsumptionRequest`: `{id, seq, kind: "release"|"amend"|"cancel", kind_label, restaurant_unit_id, product_name,
account_folio, service_mode, released_at, occurred_at, state, state_label, attempts, last_error (genérico),
next_attempt_at, processed_at, created_at, diagnostics: [{code, path, product_id?, ingredient_id?, complement_id?,
recipe_id?}]}`. Códigos: `no_configuration`, `ingredient_without_portion`, `complement_without_configuration`,
  `preparation_unavailable`, `depth_exceeded`, `preparation_cycle`, `unit_not_consumed`, `nothing_consumed`,
  `unit_reversed`, `unit_already_registered`.
- `UnitConsumptionTrace`: `{restaurant_unit_id, consumed, state: "consumed"|"reversed"|null, released_at, service_mode,
costs_visible, requests: [ConsumptionRequest], lines: [ConsumptionLine], movements: [Movement], cancellation:
Cancellation|null}`.

Consumo (sin rutas de escritura): la liberación a cocina registra una solicitud por unidad con la función
`inventory_control` activada; las modificaciones y cancelaciones de unidades ya consumidas registran `amend` y `cancel`;
el trabajador de Go escribe los movimientos `consumption` / `consumption_reversal` (`source_kind` `consumption`,
`amendment` o `cancellation`, `process` `inventory_worker` o el usuario que clasificó) y las elaboraciones
`production_out` / `production_in` (`source_kind` `production`).

- `consumption_reversal` (modificación, cancelación «No preparado», reconciliación) conserva el valor original exacto del
  consumo que compensa (base `original`); si el insumo tiene existencia negativa, el déficit cubierto se realinea con un
  `cost_adjustment` ligado (`<entry_key>:cost`, misma `restaurant_unit_id`). El consumo neto de una unidad
  (Σ `consumption` + `consumption_reversal`) sigue siendo exacto.
- `POST /productions`: el `production_in` es una entrada con documento (base `document`). Si el insumo preparado tenía
  existencia negativa (se vendió antes de elaborarlo), el déficit cubierto se valora al costo real de la elaboración y la
  diferencia contra su valor provisional es un `cost_adjustment` separado (cantidad 0, sin almacén) ligado al
  `production_in` (`related_movement_id`, `entry_key production:<elaboración>:in:cost`). La cantidad pendiente de costo baja
  como en una recepción.

Eventos SSE (ámbito `inventory`, sin contenido): `inventory.recipes.changed` (recetas y porciones),
`inventory.stock.changed` e `inventory.movements.recorded` (consumos del trabajador, clasificaciones con movimientos y
elaboraciones), `inventory.alerts.changed` (cancelaciones por clasificar, clasificadas o reconciliadas y solicitudes que
fallan o se recuperan). Auditoría: `inventory.recipe_created`, `inventory.recipe_versioned`,
`inventory.ingredient_portion_saved`, `inventory.cancellation_classified`, `inventory.cancellation_corrected`,
`inventory.production_recorded` y, con el proceso como responsable (`actor_kind: "process"`, `actor_username:
"inventory_worker"`), `inventory.cancellation_reconciled`.

## Catálogo · indicaciones rápidas con efecto (esquema 12)

Etapa 8, fase B. Amplía las indicaciones rápidas por categoría (etapa 7.1, fase D; migración 0009) y la personalización de las partidas.
Sin rutas, permisos, locks ni operaciones del puente nuevos: `catalog_create_quick_note` y
`catalog_save_quick_note` siguen igual en `api.rs`.

### Catálogo · indicaciones rápidas (migración 0012)

Indicaciones con efecto (etapa 8, fase B; migración 0012): `POST /quick-notes` y
`PATCH /quick-notes/{id}` aceptan además `effect` (`none` | `omit` | `normal` | `extra`) e
`ingredient_id` (UUID de `catalog_ingredients`, `null` sin efecto). Un efecto exige un ingrediente
existente y `none` no admite ingrediente (400 `CATALOG_VALIDATION`); un efecto desconocido también
responde 400. Sin `effect` en el cuerpo, una alta queda como `none` y una edición conserva el
efecto vigente (clientes de la etapa 7.1); enviar `ingredient_id` sin `effect` responde 400. No hay
campos de gramos. Respuesta (y cada elemento de `category_quick_notes` en `GET /lookups`):
`{id, category_id, text, effect, ingredient_id, ingredient_name, active, sort_order, revision,
updated_at}`, con `ingredient_id` e `ingredient_name` nulos sin efecto. La auditoría
(`catalog.quick-notes.created`/`.updated`) guarda el efecto y el ingrediente antes y después.
PostgreSQL aplica `catalog_category_quick_notes_effect_check`,
`catalog_category_quick_notes_ingredient_id_fkey` y
`catalog_category_quick_notes_effect_ingredient_check` (`none` ⇔ ingrediente nulo).

### Personalización · `customization.quick_notes`

En cada nivel de `customization` (producto y cada `components[].customization`), campo opcional:

```json
"customization": {
  "ingredients": [{ "ingredient_id": "<cebolla>", "quantity": "0" }],
  "groups": [],
  "components": [],
  "notes": "Sin cebolla, Bien cocida",
  "quick_notes": [
    { "note_id": "<uuid de «Sin cebolla»>", "text": "Sin cebolla" },
    { "note_id": "<uuid de «Bien cocida»>", "text": "Bien cocida" }
  ]
}
```

- Se omite cuando está vacío (`omitempty` en Go; la captura borra la clave): los borradores, las
  solicitudes (`restaurant_item_versions.request`), los eventos y las huellas de borradores
  existentes no cambian. Las partidas antiguas siguen con solo texto en `notes` y nunca tienen efecto.
- El texto de cada indicación sigue también en `notes`, unido por comas como en la etapa 7.1 (la
  comanda y los documentos imprimen `notes`). `text` en `quick_notes` es informativo: la resolución
  usa el texto vigente del catálogo.
- Forma (borrador `PUT /accounts/{id}/draft`, apertura con productos y motor de cálculo): hasta 40 por
  nivel, `note_id` UUID sin repetir y `text` de 1 a 80 caracteres; si no, 400 `OPERATION_VALIDATION`
  o `CATALOG_VALIDATION` (identificador repetido: 422 `QUICK_NOTE_INVALID`).
- Validación al cotizar (`POST /catalog/preview`, `draft-preview`, `GET /accounts/{id}/preview`),
  confirmar y modificar (`edit-preview`/`edit`), **422** con mensaje en español:
  - `QUICK_NOTE_INVALID`: la indicación no existe o está inactiva; no pertenece a la categoría del
    producto (o del producto del componente); su efecto no aplica al producto (ingrediente ausente de
    sus reglas, o `omit` sin incluido > 0 y removible, `normal` sin incluido > 0 ni agregable con
    máximo ≥ 1, `extra` sin agregable con máximo > incluido); o la cantidad solicitada del ingrediente
    no coincide con la que fija el efecto (`omit` → 0; `normal` → lo incluido, o 1 si no estaba
    incluido; `extra` → `min(incluido + 1, máximo)`).
  - `QUICK_NOTE_CONFLICT`: dos indicaciones con efecto sobre el mismo ingrediente en el mismo nivel.
- El precio sale solo de `ingredients` (una indicación nunca agrega cargos propios).

### Desglose · `ProductBreakdown.quick_notes` (en `quote_line`)

`ProductBreakdown` (raíz y `components[].product`) añade, solo si hay indicaciones:

```json
"quick_notes": [
  { "note_id": "<uuid>", "text": "Sin cebolla", "effect": "omit", "ingredient_id": "<cebolla>" },
  { "note_id": "<uuid>", "text": "Bien cocida", "effect": "none", "ingredient_id": null }
]
```

Es la resolución congelada al cotizar: editar o desactivar la indicación después no cambia las
partidas confirmadas. En el camino «Preserve» (cambios solo de observaciones o reducciones), las ya
resueltas se copian tal como estaban y las nuevas se validan con las reglas del producto del
`pricing_snapshot` del pedido y las cantidades congeladas. Sin indicaciones el campo se omite: las
cotizaciones y huellas anteriores no cambian.

`pricing_snapshot` (`catalog.Snapshot`) añade la clave `QuickNotes` (mapa por id con las indicaciones
usadas por el cálculo, en la forma de `CategoryQuickNote`) solo cuando la cotización usa alguna.

### Cocina

`kitchen_tasks.instructions.parts[]` (y las partes de los documentos) añaden `quick_notes` (la misma
forma del desglose), omitido sin indicaciones. La plantilla omite la línea estructurada de un
ingrediente («SIN Cebolla», «Queso: 1 (incluido 0)») cuando una indicación con efecto sobre ese
ingrediente aparece en la nota impresa de la partida o del componente; sin indicaciones con efecto
la comanda es idéntica a la de la etapa 7.1.

## Inventario · fase C: compras (esquema 13)

Mismas reglas que las fases A y B: prefijo `/api/v1/inventory`, `inventory.read` en toda ruta además del permiso indicado,
permisos comprobados antes de escribir (un 403 revierte), lock 7820148308 (compartido en lecturas, exclusivo en
escrituras), **toda escritura con `operation_id`** guardada en `restaurant_operations` (reintento con el mismo cuerpo →
misma respuesta sin repetir efectos; otro cuerpo → 409 `IDEMPOTENCY_CONFLICT`), `revision` en proveedores, insumos de
proveedor y órdenes (409 `REVISION_CONFLICT`), un solo identificador por ruta, cantidades e importes como cadenas
decimales. Tipos TS: `desktop/src/services/inventoryPurchasing.ts`.

**Lectura de compras** («compras»): cualquiera de `inventory.suppliers.manage`, `inventory.purchases.create`,
`inventory.purchases.authorize`, `inventory.purchases.receive`, `inventory.supplier_returns.record` o
`inventory.purchases.link_cash`. Los **importes de compra** (precio sin IVA, importe, descuento, subtotal, IVA, total, valor
asociado de una devolución) los ve quien lee compras; el **costeo de inventario** (`inventory_value`, `unit_cost`,
`outflow_value`, `cost_adjustment`, `unabsorbed`, `value_exact`, `original_unit_cost`) solo con `inventory.costs.read`
(`null` y `costs_visible: false` si falta).

Las rutas de gastos de caja toman además **7820148305 compartido antes que 7820148308** y exigen `cash.movements.read`; un
gasto de otra caja (la de la terminal, `devices.register_id`) exige `cash.shifts.read_all` y uno de un turno cerrado
`cash.history.read` (sin la excepción «quien cerró el turno» de payments).

| Método | Ruta bajo `/inventory`                                                                         | Permiso (además de `inventory.read`)                                         | Contrato                                                                                                                                                                                                                                                                   |
| ------ | ---------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET    | `/suppliers?search&active&item_id&page&page_size`                                              | compras                                                                      | `{items: [Supplier], total, page, page_size}`; activos primero, por nombre                                                                                                                                                                                                 |
| POST   | `/suppliers`                                                                                   | `inventory.suppliers.manage`                                                 | `SupplierInput` → `SupplierDetail`; 409 `DUPLICATE_NAME`                                                                                                                                                                                                                   |
| GET    | `/suppliers/{id}`                                                                              | compras                                                                      | `SupplierDetail`                                                                                                                                                                                                                                                           |
| PATCH  | `/suppliers/{id}`                                                                              | `inventory.suppliers.manage`                                                 | `SupplierInput` con `revision` → `SupplierDetail`; desactivar = `active: false` (no hay borrado)                                                                                                                                                                           |
| PUT    | `/suppliers/{id}/items`                                                                        | `inventory.suppliers.manage`                                                 | `{operation_id, revision (0 si es nuevo), item_id, presentation_id?, supplier_code?, preferred?}` → `SupplierDetail`; 409 `ITEM_INACTIVE`, `PRESENTATION_INACTIVE`                                                                                                         |
| POST   | `/suppliers/{id}/items/remove`                                                                 | `inventory.suppliers.manage`                                                 | `{operation_id, revision, item_id}` → `SupplierDetail` (configuración, no historia)                                                                                                                                                                                        |
| GET    | `/purchase-orders?search&state&supplier_id&from&until&page&page_size`                          | compras                                                                      | `{items: [PurchaseOrderSummary], total, page, page_size}`; `state` uno de los seis u `open` (emitidas y parciales); `from`/`until` fecha de la orden; `search` folio exacto o proveedor                                                                                    |
| POST   | `/purchase-orders`                                                                             | `inventory.purchases.create`                                                 | `PurchaseOrderInput` → `PurchaseOrder` en `draft`; 409 `SUPPLIER_INACTIVE`, `ITEM_INACTIVE`, `PRESENTATION_INACTIVE`, `WAREHOUSE_INACTIVE`                                                                                                                                 |
| GET    | `/purchase-orders/{id}`                                                                        | compras                                                                      | `PurchaseOrder`                                                                                                                                                                                                                                                            |
| PATCH  | `/purchase-orders/{id}`                                                                        | `inventory.purchases.create`                                                 | `PurchaseOrderInput` con `revision` (reemplaza datos y renglones) → `PurchaseOrder`; 409 `ORDER_NOT_EDITABLE` si no es borrador                                                                                                                                            |
| POST   | `/purchase-orders/{id}/issue`                                                                  | `inventory.purchases.authorize`                                              | `{operation_id, revision}` → `PurchaseOrder` `issued`; 409 `ORDER_NOT_DRAFT`, `ORDER_EMPTY`, `SUPPLIER_INACTIVE`, `ITEM_INACTIVE`                                                                                                                                          |
| POST   | `/purchase-orders/{id}/cancel`                                                                 | `inventory.purchases.authorize`                                              | `{operation_id, revision, reason (5–500)}` → `cancelled` desde borrador, emitida o parcial (conserva lo recibido); 409 `ORDER_NOT_CANCELLABLE`                                                                                                                             |
| POST   | `/purchase-orders/{id}/close`                                                                  | `inventory.purchases.authorize`                                              | `{operation_id, revision, reason (5–500)}` → `closed` desde emitida o parcial; 409 `ORDER_NOT_CLOSABLE`                                                                                                                                                                    |
| GET    | `/receipts?search&supplier_id&purchase_order_id&warehouse_id&direct&from&until&page&page_size` | compras                                                                      | `{items: [ReceiptSummary], total, page, page_size, costs_visible}`; `from`/`until` días de negocio; `search` folio exacto, referencia del proveedor o proveedor                                                                                                            |
| POST   | `/receipts`                                                                                    | `inventory.purchases.receive` (+ `inventory.purchases.create` si es directa) | `OrderReceiptInput` o `DirectReceiptInput` → `Receipt` con `warnings`; 409 `ORDER_NOT_RECEIVABLE`, `RECEIPT_EXCEEDS_PENDING`, `SUPPLIER_INACTIVE`, `WAREHOUSE_INACTIVE`                                                                                                    |
| GET    | `/receipts/{id}`                                                                               | compras                                                                      | `Receipt`                                                                                                                                                                                                                                                                  |
| GET    | `/supplier-returns?receipt_id&supplier_id&from&until&page&page_size`                           | compras                                                                      | `{items: [SupplierReturnSummary], total, page, page_size, costs_visible}`                                                                                                                                                                                                  |
| POST   | `/supplier-returns`                                                                            | `inventory.supplier_returns.record`                                          | `SupplierReturnInput` → `SupplierReturn` con `warnings` (`NEGATIVE_STOCK`); 409 `RETURN_EXCEEDS_RECEIVED`, `WAREHOUSE_INACTIVE`                                                                                                                                            |
| GET    | `/supplier-returns/{id}`                                                                       | compras                                                                      | `SupplierReturn`                                                                                                                                                                                                                                                           |
| GET    | `/cash-expenses?from&until&min_amount&max_amount&register_id&receipt_id&page&page_size`        | `inventory.purchases.link_cash` + `cash.movements.read`                      | `{items: [CashExpense], total, page, page_size}`: gastos (`kind='expense'`) que el actor puede leer, el más reciente primero; `register_id` de otra caja sin `read_all` → 403; sin caja ni `read_all` → lista vacía                                                        |
| POST   | `/receipts/{id}/cash-links`                                                                    | `inventory.purchases.link_cash` + `cash.movements.read`                      | `{operation_id, cash_operation_id, amount (> 0, 2 decimales), notes? (≤ 500)}` → `Receipt`; 403 gasto no legible; 409 `CASH_OPERATION_NOT_EXPENSE`, `CASH_EXPENSE_CURRENCY`, `CASH_EXPENSE_CORRECTED`, `CASH_LINK_EXISTS`, `CASH_EXPENSE_OVERLINKED`, `RECEIPT_OVERLINKED` |
| POST   | `/cash-links/{id}/unlink`                                                                      | `inventory.purchases.link_cash` + `cash.movements.read`                      | `{operation_id, reason (5–500)}` → `Receipt`; 403 gasto no legible; 409 `CASH_LINK_INACTIVE`                                                                                                                                                                               |

No existen rutas de borrado (proveedores, órdenes, recepciones, devoluciones, vínculos) ni de edición de recepciones o
devoluciones; ninguna ruta crea, modifica o corrige operaciones de caja.

### Importes, IVA y costo (Go, `inventory/purchasing.go`)

- Renglón: `gross = redondeo(cantidad capturada × precio unitario sin IVA)`; `discount` en pesos (≤ `gross`);
  `subtotal = gross − discount`; `tax = redondeo(subtotal × tasa ÷ 100)`; `total = subtotal + tax`. Redondeo a centavos
  mitad alejándose de cero. Tasa de lista cerrada **16, 8 o 0** (entero en JSON). Precio hasta 6 decimales (≤ 1,000,000);
  importe del renglón ≤ 9,999,999,999.99.
- Documento (orden o recepción): suma exacta de los renglones (`subtotal`, `discount`, `tax`, `total`); el IVA no se
  recalcula sobre el total. `gross` del documento no se entrega (es `subtotal + discount`).
- Recepción de una orden: precio y tasa por omisión los del renglón de la orden; el descuento se **prorratea de forma
  acumulada** por lo recibido: `redondeo(D × min(antes + ahora, Q) ÷ Q) − redondeo(D × min(antes, Q) ÷ Q)` (12 + 8 de 20
  cajas con $50: $30.00 y $20.00; lo recibido de más no lleva descuento). Se pueden capturar otro precio, otro descuento u
  otra tasa (factura real).
- Valor de inventario de cada renglón = `total` (con IVA) si la recepción tiene `tax_in_cost: true`, si no `subtotal`;
  `tax_in_cost` se copia de `inventory_settings.purchase_tax_in_cost` al recibir. Costo por unidad base =
  valor ÷ `base_quantity` (10 decimales). El movimiento `receipt` lleva ese valor (regla 2 del motor: con existencia
  negativa, el ajuste `cost_adjustment` va aparte, ligado al movimiento de la recepción).
- Devolución: valor asociado = cantidad base × valor del renglón ÷ cantidad base recibida (exacto al devolver todo lo que
  quedaba); salida al promedio vigente; la diferencia se absorbe como `cost_adjustment` solo si queda existencia positiva
  (sin dejar el valor negativo) y el resto se guarda como `unabsorbed` en el documento.

### Cuerpos y respuestas

- `SupplierInput`: `{operation_id, revision? (PATCH), trade_name (1–100, único sin mayúsculas), legal_name? (≤ 200), rfc?
(3–4 letras A–Z, Ñ o &, fecha AAMMDD válida y 3 de homoclave; se guarda en mayúsculas; vacío o null = sin RFC; no
único), phone? (≤ 40), email? (≤ 120, formato nombre@dominio), address? (≤ 500, admite saltos de línea), contact? (≤ 100),
notes? (≤ 2000), active? (ausente: activo al crear, sin cambio al editar)}`.
- `Supplier`: `{id, trade_name, legal_name, rfc, phone, email, address, contact, notes, active, item_count, has_purchases,
last_receipt_at, revision, created_at, updated_at, updated_by_name}`.
- `SupplierDetail`: `Supplier` + `{items: [SupplierItem], history: {order_count, receipt_count, return_count,
receipts_total, returns_value, orders: [PurchaseOrderSummary], receipts: [ReceiptSummary], returns:
[SupplierReturnSummary]}}` (las 20 más recientes de cada tipo).
- `SupplierItem`: `{item: ItemReference, presentation: PresentationReference|null, supplier_code, preferred,
last_unit_price (sin IVA, antes del descuento), last_price_unit_label (presentación o unidad de esa recepción),
last_price_at, last_receipt_folio, revision, updated_at}`. Un preferido por insumo: marcar otro quita la marca del
  anterior (auditado). Las recepciones crean la relación si no existía y actualizan el último precio si son iguales o
  más recientes que la anterior.
- `PresentationReference`: `{id, name, base_quantity, active}`; `SupplierReference`: `{id, trade_name, active}`.
- `PurchaseLineInput`: `{item_id, presentation_id? | unit_id? (por omisión la unidad base; nunca ambos), quantity (> 0, 6
decimales), unit_price, discount? ("0" por omisión), tax_rate: 16|8|0}`; un insumo una sola vez por documento.
- `PurchaseOrderInput`: `{operation_id, revision? (PATCH), supplier_id (activo), order_date? (AAAA-MM-DD, no futura; por
omisión hoy en la zona del establecimiento), expected_warehouse_id? (por omisión «Almacén general»), notes? (≤ 2000),
lines: [PurchaseLineInput] (0–200; emitir exige al menos uno)}`.
- `PurchaseLine` (común): `{line_no, item: ItemReference, presentation, unit (unidad de captura o null), capture_label,
quantity, base_quantity, unit_price, gross, discount, subtotal, tax_rate, tax, total}`.
- `PurchaseOrderSummary`: `{id, folio, supplier, state, state_label, order_date, expected_warehouse: {id, name}, subtotal,
discount, tax, total, line_count, receipt_count, created_by_name, issued_at, revision, created_at, updated_at}`.
- `PurchaseOrder`: `PurchaseOrderSummary` + `{notes, updated_by_name, issued_by_name, cancelled_at, cancelled_by_name,
cancel_reason, closed_at, closed_by_name, close_reason, editable, issuable, cancellable, closable, receivable (según el
estado), lines: [PurchaseLine + {received_quantity, pending_quantity, excess_quantity, received_base_quantity,
pending_base_quantity, fully_received}], receipts: [ReceiptSummary]}`. Cada recepción aumenta la revisión de la orden.
- `OrderReceiptInput`: `{operation_id, purchase_order_id (emitida o parcial), supplier_id? (si se envía, el de la orden),
warehouse_id? (por omisión el previsto), occurred_at? (RFC 3339, no futura), supplier_reference? (≤ 60, una línea),
notes? (≤ 2000), allow_excess? (false), lines: [{order_line_no, quantity (en la presentación o unidad del renglón de la
orden), unit_price?, discount?, tax_rate?}] (1–200, sin item_id/presentation_id/unit_id, cada renglón una vez)}`.
- `DirectReceiptInput`: `{operation_id, supplier_id (activo, obligatorio), warehouse_id?, occurred_at?, supplier_reference?,
notes?, lines: [PurchaseLineInput] (1–200)}`.
- `ReceiptSummary`: `{id, folio, supplier, purchase_order_id, purchase_order_folio, direct, warehouse, occurred_at,
received_by_name, supplier_reference, tax_in_cost, subtotal, discount, tax, total, line_count, excess, linked_amount,
linkable_amount, cash_link_attention (algún gasto vinculado vigente se corrigió o regularizó después), returned_value,
costs_visible, inventory_value, inventory_value_display, created_at}`.
- `Receipt`: `ReceiptSummary` + `{notes, lines: [ReceiptLine], returns: [SupplierReturnSummary], cash_links: [CashLink],
warnings: [{code: "RECEIPT_EXCESS", message, item_id}] (solo en la respuesta del registro)}`.
- `ReceiptLine`: `PurchaseLine` + `{order_line_no, ordered_quantity, received_before_quantity (recepciones anteriores de
ese renglón), pending_after_quantity (pendiente después de esta), returned_quantity, returnable_quantity,
returned_base_quantity, returnable_base_quantity, movement_id, stock_after (existencia del almacén tras el movimiento),
costs_visible, inventory_value, inventory_value_display, unit_cost (por unidad base), unit_cost_display}`.
- `SupplierReturnInput`: `{operation_id, receipt_id, warehouse_id? (por omisión el de la recepción), occurred_at? (no
futura ni anterior a la recepción), reason (5–500), lines: [{receipt_line_no, quantity (en la presentación o unidad del
renglón recibido, ≤ returnable_quantity)}] (1–200, cada renglón una vez)}`.
- `SupplierReturnSummary`: `{id, folio, receipt_id, receipt_folio, supplier, warehouse, occurred_at, reason, actor_name,
value (valor asociado a centavos), line_count, costs_visible, unabsorbed, unabsorbed_display, value_exact, created_at,
receipt_supplier_reference}`.
- `SupplierReturn`: `SupplierReturnSummary` + `{lines: [{line_no, receipt_line_no, item, capture_label, quantity,
base_quantity, value, movement_id, stock_after, costs_visible, original_unit_cost, original_unit_cost_display,
outflow_value, outflow_value_display, cost_adjustment, cost_adjustment_display, unabsorbed, unabsorbed_display,
cost_adjustment_movement_id}], warnings}`.
- `CashExpense`: `{id, shift_id, register_id, register_name, shift_state: open|closing|closed, shift_business_date,
currency: MXN|USD, amount, concept, category_name, reference, created_at, actor_name, status: recorded|corrected|regularized,
status_label, corrected_at, linked_amount (Σ vínculos vigentes), available_amount, linked_to_receipt (con receipt_id),
linkable, unavailable_reason: not_expense|currency|corrected|fully_linked|null, unavailable_label}`.
- `CashLink`: `{id, receipt_id, receipt_folio, cash_operation_id, amount, notes, linked_at, linked_by_name, status:
active|expense_corrected|expense_regularized|unlinked, status_label, active, unlinked_at, unlinked_by_name, unlink_reason,
expense_visible, expense: CashExpense|null (solo si el actor puede leer el gasto)}`. Reglas: gasto existente, `kind`
  `expense`, en pesos y sin corrección; Σ vínculos vigentes del gasto ≤ su importe y de la recepción ≤ su total; un vínculo
  vigente por par recepción–gasto (para cambiar el importe se desvincula y se vincula de nuevo). Un vínculo con un gasto
  corregido después sigue contando hasta desvincularlo.

Eventos SSE (ámbito `inventory`, sin contenido): `inventory.purchases.changed` (proveedores, sus insumos y órdenes; también
al recibir una orden), `inventory.receipts.changed` (recepciones, devoluciones y vínculos con gastos),
`inventory.stock.changed` (almacén de la recepción o devolución) e `inventory.movements.recorded` (recepción o devolución).

Libro: movimientos `receipt` (`source_kind` `receipt`, `source_id` = recepción, `source_line` = renglón, `entry_key`
`receipt:<recepción>:<renglón>`) con su `cost_adjustment` (`…:cost`) y `supplier_return` (`source_kind` `supplier_return`,
`entry_key` `supplier_return:<devolución>:<renglón>`). `GET movements?source_kind=receipt&source_id=…` los consulta.

## Inventario · fase D: control de inventario (esquema 14)

Mismas reglas que las fases A–C: prefijo `/api/v1/inventory`, `inventory.read` en toda ruta además del permiso indicado,
permisos comprobados antes de escribir (un 403 revierte y audita `access.denied`), lock 7820148308 (compartido en
lecturas, exclusivo en escrituras), **toda escritura con `operation_id`** guardada en `restaurant_operations` (reintento
con el mismo cuerpo → misma respuesta sin repetir efectos; otro cuerpo → 409 `IDEMPOTENCY_CONFLICT`), `revision` en
autorizaciones, rechazos, transiciones de conteos y niveles (409 `REVISION_CONFLICT`), un solo identificador por ruta,
cantidades y valores como cadenas decimales. Tipos TS: `desktop/src/services/inventoryControl.ts`.

**Ninguna ruta edita existencias**: cada efecto es un movimiento del libro con el motor de costos (`adjustment_in`,
`adjustment_out`, `waste`, `transfer_out`/`transfer_in`, `count_in`, `count_out`, más los ajustes de costo o residuos que
el motor agregue). Una existencia negativa **nunca bloquea**: llega como aviso `{code: "NEGATIVE_STOCK", message,
item_id}` en `warnings`. Valores (`value`, `value_display`) solo con `inventory.costs.read`; si falta llegan en `null` y
`costs_visible: false`.

Lecturas por documento: **ajustes** con `inventory.adjustments.record`, `inventory.adjustments.authorize`,
`inventory.waste.record` o `inventory.movements.read`; **transferencias** con `inventory.transfers.record` o
`inventory.movements.read`; **conteos** con `inventory.counts.perform` o `inventory.counts.approve`.

| Método | Ruta bajo `/inventory`                                                                 | Permiso (además de `inventory.read`)                                                         | Contrato                                                                                                                                                                                                                            |
| ------ | -------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET    | `/adjustments?kind&state&reason&warehouse_id&item_id&from&until&search&page&page_size` | lectura de ajustes                                                                           | `{items: [AdjustmentSummary], total, page, page_size, costs_visible}`; el más reciente primero; `from`/`until` días de negocio del registro; `search` folio exacto                                                                  |
| POST   | `/adjustments`                                                                         | `inventory.adjustments.record` (discrepancia) o `inventory.waste.record` (merma)             | `AdjustmentInput` → `Adjustment` con `warnings`; discrepancia aplicada si la sesión tiene `inventory.adjustments.authorize`, si no `pending_authorization`; merma siempre aplicada; 409 `COST_REQUIRED` si se aplica al registrarla |
| GET    | `/adjustments/{id}`                                                                    | lectura de ajustes                                                                           | `Adjustment` (`authorizable`: pendiente, con el permiso y registrado por otra persona)                                                                                                                                              |
| POST   | `/adjustments/{id}/authorize`                                                          | `inventory.adjustments.authorize`                                                            | `{operation_id, revision}` → `Adjustment` aplicado (movimientos con la hora de la autorización) con `warnings`; 403 `SELF_AUTHORIZATION_FORBIDDEN`; 409 `ADJUSTMENT_NOT_PENDING`, `COST_REQUIRED` (sigue pendiente)                 |
| POST   | `/adjustments/{id}/reject`                                                             | `inventory.adjustments.authorize`                                                            | `{operation_id, revision, reason (5–500)}` → `Adjustment` `rejected` (sin movimientos); 403 `SELF_AUTHORIZATION_FORBIDDEN`; 409 `ADJUSTMENT_NOT_PENDING`                                                                            |
| GET    | `/transfers?warehouse_id&item_id&from&until&search&page&page_size`                     | lectura de transferencias                                                                    | `{items: [TransferSummary], total, page, page_size, costs_visible}`; `warehouse_id` origen o destino                                                                                                                                |
| POST   | `/transfers`                                                                           | `inventory.transfers.record`                                                                 | `TransferInput` → `Transfer` con `warnings` (`NEGATIVE_STOCK` del origen); 400 mismo almacén; 409 `WAREHOUSE_INACTIVE`                                                                                                              |
| GET    | `/transfers/{id}`                                                                      | lectura de transferencias                                                                    | `Transfer`                                                                                                                                                                                                                          |
| GET    | `/counts?warehouse_id&state&from&until&search&page&page_size`                          | lectura de conteos                                                                           | `{items: [CountSummary], total, page, page_size}`; `state` uno de los cinco o `active` (abiertos, enviados y aprobados); `from`/`until` días de negocio de creación                                                                 |
| POST   | `/counts`                                                                              | `inventory.counts.perform`                                                                   | `CountInput` → `Count` abierto; 409 `COUNT_ITEM_IN_PROGRESS`, `COUNT_EMPTY`, `WAREHOUSE_INACTIVE`                                                                                                                                   |
| GET    | `/counts/{id}`                                                                         | lectura de conteos                                                                           | `Count` (existencia teórica y diferencias solo con `theoretical_visible`; avisos vigentes en `warnings`)                                                                                                                            |
| PATCH  | `/counts/{id}`                                                                         | `inventory.counts.perform`                                                                   | `{operation_id, revision, notes?, add_item_ids?, remove_item_ids?}` → `Count`; solo abierto (409 `COUNT_NOT_OPEN`); 409 `COUNT_ITEM_IN_PROGRESS`                                                                                    |
| PUT    | `/counts/{id}/lines`                                                                   | `inventory.counts.perform`                                                                   | `CountCaptureInput` (1–200 renglones, **sin revisión**) → `Count`; solo abierto (409 `COUNT_NOT_OPEN`)                                                                                                                              |
| POST   | `/counts/{id}/submit`                                                                  | `inventory.counts.perform`                                                                   | `{operation_id, revision}` → `submitted`; 409 `COUNT_NOT_OPEN`, `COUNT_NOTHING_COUNTED`                                                                                                                                             |
| POST   | `/counts/{id}/approve`                                                                 | `inventory.counts.approve`                                                                   | `{operation_id, revision}` → `approved`; 409 `COUNT_NOT_SUBMITTED`                                                                                                                                                                  |
| POST   | `/counts/{id}/apply`                                                                   | `inventory.counts.approve`                                                                   | `{operation_id, revision}` → `applied` con `warnings`; 409 `COUNT_NOT_APPROVED`, `COUNT_ALREADY_APPLIED`, `COUNT_CONSUMPTION_PENDING`, `COST_REQUIRED` (sigue aprobado)                                                             |
| POST   | `/counts/{id}/cancel`                                                                  | `inventory.counts.perform` o `inventory.counts.approve` (enviado o aprobado: solo `approve`) | `{operation_id, revision, reason (5–500)}` → `cancelled`; 409 `COUNT_NOT_CANCELLABLE`                                                                                                                                               |
| GET    | `/stock-levels?warehouse_id&item_id&search&page&page_size`                             | —                                                                                            | `{items: [StockLevel], total, page, page_size}`: niveles configurados (existencia y banderas solo con `inventory.stock.read`)                                                                                                       |
| PUT    | `/stock-levels`                                                                        | `inventory.items.manage`                                                                     | `StockLevelInput` → `StockLevel`; los dos valores `null` quitan el nivel (`configured: false`, `revision: 0`)                                                                                                                       |
| GET    | `/alerts?type&severity&page&page_size`                                                 | —                                                                                            | `AlertPage`; `counts` siempre de todas las alertas visibles (sin filtros)                                                                                                                                                           |
| GET    | `/purchase-suggestions?supplier_id&category_id&search&page&page_size`                  | —                                                                                            | `PurchaseSuggestionPage`; `supplier_id` (o `none`) solo con algún permiso de compras (si no, 403)                                                                                                                                   |
| POST   | `/purchase-suggestions/draft-order`                                                    | `inventory.purchases.create`                                                                 | `{operation_id, supplier_id, item_ids (1–200), expected_warehouse_id?, notes?}` → `PurchaseOrder` en `draft` (nunca emitida); 409 `SUGGESTION_CHANGED`, `SUPPLIER_INACTIVE`                                                         |

`GET /summary` agrega `alerts: AlertCounts` (`{total, critical, warning, info, by_type: {tipo: n}}`), con la misma
visibilidad que `GET /alerts`. `GET /stock` agrega a cada fila `level_scope` (`item` | `warehouse`): con `warehouse_id`, un
insumo con nivel en ese almacén compara su existencia del almacén con ese nivel (`min_stock`, `reorder_point`,
`below_minimum`, `at_reorder_point` y los filtros `state=below_minimum|reorder` usan el nivel del almacén); sin nivel o sin
filtro, los del insumo contra la existencia total (como en la fase A).

No existen rutas de edición o borrado de ajustes, transferencias o conteos (se autoriza, rechaza, cancela o se registra
otro documento), ni de edición directa de existencias, ni de emisión desde sugerencias.

### Ajustes y mermas

```
AdjustmentInput = {operation_id, kind: "discrepancy"|"waste", reason, reason_detail?, warehouse_id, notes? (≤ 500),
                   lines: [{item_id, quantity, presentation_id?, unit_id?}] (1–200, un insumo una vez)}
```

- `quantity` con signo: `"-2.5"` salida, `"3"` entrada (hasta 6 decimales; cero no). Capturada en una presentación del
  insumo, en una unidad de su dimensión o (sin ninguna) en su unidad base; se guarda también convertida
  (`captured_quantity` y `quantity` de la respuesta, ambas con signo).
- `reason` (lista cerrada) y su sentido: `breakage` Rotura, `spill` Derrame, `expiry` Caducidad, `physical_shortage`
  Faltante físico, `extraordinary_use` Consumo extraordinario → solo salidas; `surplus` Sobrante → solo entradas;
  `entry_error` Error de registro y `other` Otro → ambos. `other` exige `reason_detail` (3–200, una línea).
- Merma (`waste`): solo salidas y solo `breakage`, `spill`, `expiry`, `extraordinary_use` u `other`; movimiento `waste`.
  Un ajuste negativo de discrepancia es `adjustment_out` (nunca se convierte en merma).
- Entradas: `adjustment_in` al promedio vigente (o, con existencia negativa, al provisional del faltante y el resto a la
  referencia; **sin ningún costo para lo que queda en existencia positiva, 409 `COST_REQUIRED`**, regla 3 del motor).
  Salidas: al promedio (regla 1 del motor).
- 409 `COST_REQUIRED` en `POST /adjustments` (cuando se aplica al registrarlo), `POST /adjustments/{id}/authorize` y
  `POST /counts/{id}/apply`: una entrada sin documento (`adjustment_in`, `count_in`) que dejaría existencia positiva sin
  último costo válido ni costo de referencia se rechaza y no se registra nada (el ajuste pendiente sigue pendiente, el
  conteo sigue aprobado con la misma revisión). Mensaje: «El insumo «X» no tiene costo: configura su costo de referencia
  antes de registrar una entrada sin documento (ajuste positivo o sobrante de conteo).» La parte que solo cubre un
  déficit se sigue aceptando (a su valor provisional).
- Insumos y presentaciones desactivados se admiten (para ajustar la existencia que les quede); el almacén debe estar
  activo.

```
AdjustmentSummary = {id, folio, kind, kind_label, reason, reason_label, reason_detail, warehouse: {id, name}, state,
                     state_label, notes, line_count, requested_by_name, requested_at, authorized_by_name, authorized_at,
                     applied_at, rejected_by_name, rejected_at, rejection_reason, revision, updated_at, costs_visible,
                     value, value_display}
Adjustment = AdjustmentSummary + {authorizable, warnings, lines: [{line_no, item, presentation, unit, capture_label,
             captured_quantity, quantity, direction: "in"|"out", movement_id, movement_kind, movement_label, stock_after,
             value, value_display}]}
```

`state`: `pending_authorization` | `applied` | `rejected`. `authorized_by_name` solo cuando otra persona autorizó un
pendiente (un ajuste aplicado al registrarse no tiene autorizador). `value` = valor neto de sus movimientos (con costos).

### Transferencias

```
TransferInput = {operation_id, from_warehouse_id, to_warehouse_id, notes? (≤ 500),
                 lines: [{item_id, quantity (> 0), presentation_id?, unit_id?}] (1–200, un insumo una vez)}
Transfer = {id, folio, from_warehouse, to_warehouse, occurred_at, actor_name, notes, line_count, created_at, costs_visible,
            value, value_display, warnings, lines: [{line_no, item, presentation, unit, capture_label, captured_quantity,
            quantity, out_movement_id, in_movement_id, from_stock_after, to_stock_after, value, value_display}]}
```

Salida y entrada vinculadas con el mismo valor (`value` = valor trasladado); el costo del insumo no cambia. Ambos almacenes
activos y distintos.

### Conteos físicos

```
CountInput = {operation_id, warehouse_id, scope: "all"|"category"|"items", category_id? (solo category),
              item_ids? (solo items, 1–2000), notes? (≤ 1000)}
CountCaptureInput = {operation_id, lines: [{item_id, counted_quantity: "12"|"0"|null, presentation_id?, unit_id?}]}
```

- `all`: todos los insumos activos; `category`: los activos de la categoría; `items`: la lista (también desactivados). Orden
  de hoja de conteo: categoría y nombre. Un insumo no puede estar en dos conteos sin terminar del mismo almacén
  (`COUNT_ITEM_IN_PROGRESS`).
- Captura: cantidad física (cero admitido) en presentación, unidad o unidad base; `null` borra la captura. La hora del
  renglón (`captured_at`) es la de la transacción del guardado; capturar otra vez sustituye. Sin revisión (varias terminales
  capturan a la vez); cada guardado es una transacción corta.
- Estados: `open` → `submitted` (≥ 1 renglón contado) → `approved` → `applied`; `cancelled` desde `open`, `submitted` o
  `approved`.
- Diferencia = contado − existencia teórica a `captured_at` (Σ del libro del almacén con `occurred_at ≤ captured_at`). En un
  conteo sin aplicar se calcula al consultar; al aplicar se guarda la definitiva.
- Aplicar: procesa en la misma transacción las solicitudes de consumo sin procesar con `released_at ≤` la hora máxima de
  los renglones contados (con el procesador del trabajador); si alguna falla, 409 `COUNT_CONSUMPTION_PENDING` y nada se
  registra. Después, un `count_in` (sobrante) o `count_out` (faltante) por renglón contado con diferencia, con
  `occurred_at = captured_at`, `entry_key = count:<conteo>:<insumo>`, origen `count`; los renglones sin contar no se
  ajustan. `reference_at` = hora máxima aplicada. Una segunda aplicación responde `COUNT_ALREADY_APPLIED`.

```
CountSummary = {id, folio, warehouse, scope, scope_label, category: {id, name}|null, state, state_label, notes, line_count,
                counted_lines, difference_lines (aplicado y visible), created_by_name, created_at, updated_by_name,
                updated_at, submitted_by_name, submitted_at, approved_by_name, approved_at, applied_by_name, applied_at,
                reference_at, cancelled_by_name, cancelled_at, cancel_reason, revision, theoretical_visible}
Count = CountSummary + {editable, submittable, approvable, applicable, cancellable, costs_visible, value, value_display,
        warnings, lines: [CountLine]}
CountLine = {line_no, item, category_name, counted, counted_quantity, captured_quantity, presentation, unit, capture_label,
             captured_at, captured_by_name, theoretical_quantity, difference, difference_kind: "surplus"|"shortage"|"none",
             current_quantity, movement_id, movement_kind, movement_label, value, value_display}
```

`theoretical_visible` = `inventory.counts.approve` o `inventory.stock.read`: sin él, `theoretical_quantity`, `difference`,
`difference_kind`, `current_quantity`, `movement_id`, `movement_kind`, `movement_label`, `difference_lines` y valores llegan
en `null` (captura a ciegas). Valores además con
`inventory.costs.read`. `warnings` de un conteo sin terminar: `COUNT_UNCOUNTED_LINES`, `COUNT_PENDING_CONSUMPTION`,
`COUNT_PENDING_CANCELLATIONS` (cancelaciones sin clasificar anteriores a la hora del conteo que consumieron en el almacén),
`COUNT_PENDING_ADJUSTMENTS` (ajustes por autorizar del almacén sobre insumos contados); al aplicar, `NEGATIVE_STOCK`.

### Niveles por almacén

```
StockLevelInput = {operation_id, item_id, warehouse_id, revision (0 sin nivel), min_stock: string|null, reorder_point: string|null}
StockLevel = {item, warehouse, min_stock, reorder_point, item_min_stock, item_reorder_point, stock_visible, quantity,
              below_minimum, at_reorder_point, configured, revision, updated_at, updated_by_name}
```

En la unidad base, ≥ 0 y hasta 6 decimales. Sobrescriben los del insumo **para ese almacén** (alertas del almacén y
`GET /stock?warehouse_id`); los del insumo siguen comparándose con la existencia total.

### Alertas

```
AlertPage = {items: [Alert], total, page, page_size, counts: AlertCounts, costs_visible, stock_visible,
             purchasing_visible, generated_at}
Alert = {type, type_label, severity: "critical"|"warning"|"info", severity_label, message, reference: {kind, id, label},
         item, warehouse, quantity, threshold, count, occurred_at}
```

| `type`                        | Gravedad | Cuándo                                                                                                                                                                                                                                               | `reference.kind`                | Visibilidad                                          |
| ----------------------------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------- | ---------------------------------------------------- |
| `negative_stock`              | critical | insumo activo con existencia < 0 en un almacén                                                                                                                                                                                                       | `stock`                         | todos                                                |
| `consumption_failed`          | critical | solicitudes de consumo `failed` (una alerta con `count`, `occurred_at` = la más antigua)                                                                                                                                                             | `consumption_requests` (sin id) | todos                                                |
| `below_minimum`               | warning  | nivel del almacén (existencia del almacén) o del insumo (existencia total, `warehouse: null`)                                                                                                                                                        | `stock`                         | todos                                                |
| `cost_pending`                | warning  | valuación con `pending_cost_quantity > 0`                                                                                                                                                                                                            | `item`                          | `inventory.costs.read`                               |
| `count_difference_pending`    | warning  | conteo enviado o aprobado sin aplicar (`count` = diferencias calculadas ahora); sin `inventory.counts.approve` ni `inventory.stock.read`, «El conteo N de «Almacén» está pendiente de aprobar.» (o «aprobado y pendiente de aplicar.») y `count` = 1 | `count`                         | todos (diferencias: `counts.approve` o `stock.read`) |
| `cancellations_unclassified`  | warning  | cancelaciones `pending` (una alerta con `count`, `occurred_at` = vencimiento más próximo)                                                                                                                                                            | `cancellations` (sin id)        | todos                                                |
| `cash_link_expense_corrected` | warning  | recepción con vínculo vigente a un gasto corregido o regularizado después (`count` = vínculos)                                                                                                                                                       | `receipt`                       | algún permiso de compras                             |
| `reorder_point`               | info     | existencia ≤ punto de reabastecimiento (por almacén o total, como `below_minimum`)                                                                                                                                                                   | `stock`                         | todos                                                |
| `count_due`                   | info     | almacén activo con existencias cuyo último conteo aplicado venció según `count_frequency` (o nunca contado)                                                                                                                                          | `warehouse`                     | todos                                                |
| `purchase_order_partial`      | info     | orden `partially_received`                                                                                                                                                                                                                           | `purchase_order`                | algún permiso de compras                             |
| `item_without_supplier`       | info     | insumo comprado activo sin proveedor activo que lo surta                                                                                                                                                                                             | `item`                          | todos                                                |
| `product_without_recipe`      | info     | producto (no combo) activo, no archivado, de categoría activa, sin receta activa; si `warn_products_without_recipe`                                                                                                                                  | `product`                       | todos                                                |

Orden: gravedad, tipo (el de la tabla) y nombre del destino. `quantity` y `threshold` (unidad base) y las cantidades del
texto solo con `inventory.stock.read`. Los textos no llevan identificadores (sí folios y nombres).

### Sugerencias de compra

```
PurchaseSuggestion = {item, category_name, reorder_point, reorder_source: "item"|"warehouses", min_stock, stock_visible,
                      stock, pending_quantity, draft_quantity, need_quantity, below_minimum, purchasing_visible, supplier,
                      supplier_code, presentation, unit, capture_label, suggested_quantity, suggested_base_quantity,
                      last_unit_price, last_price_label, last_price_at}
PurchaseSuggestionPage = {items, total, page, page_size, stock_visible, purchasing_visible}
```

- Insumos comprados activos con punto de reabastecimiento: el del insumo o, si no tiene, la suma de los de sus almacenes
  (`reorder_source: "warehouses"`). `need = reorder_point − (existencia total + pendiente de órdenes issued/partially_received)`;
  solo si `need > 0`. Los borradores no cuentan como pendiente (se informan en `draft_quantity`).
- Presentación: la habitual del proveedor preferido (activa) → la predeterminada de compra del insumo → la unidad base
  (`unit`). Redondeo hacia arriba a presentaciones completas (`ceil(need ÷ base_quantity)`) o a unidades base enteras.
- `stock`, `pending_quantity`, `draft_quantity`, `need_quantity`, `below_minimum`, `reorder_point`, `suggested_quantity` y
  `suggested_base_quantity` (`string | null`) solo con `stock_visible` (`inventory.stock.read` o
  `inventory.purchases.create`): con el punto y lo sugerido se deduciría la existencia. Sin él la sugerencia sigue
  listándose (con presentación, `capture_label`, mínimo y, con permisos de compras, proveedor y último precio). `supplier`,
  `supplier_code` y último precio con algún permiso de compras. El borrador (`purchases.create`) no cambia.
- Orden: por proveedor preferido (los que no tienen, al final) e insumo.
- Borrador: para el proveedor indicado, renglón por insumo con la cantidad sugerida en su presentación (la habitual de ese
  proveedor), precio = último precio del proveedor si se capturó en la misma presentación o unidad (si no, `0`), tasa de IVA
  = la de su última recepción con ese proveedor (si no, 16). Audita `inventory.purchase_order_drafted_from_suggestions` y
  publica `inventory.purchases.changed`. Se revisa y emite con las rutas de órdenes de la fase C.

### Eventos y auditoría

Eventos del ámbito `inventory` (sin contenido): `inventory.adjustments.changed`, `inventory.transfers.changed`,
`inventory.counts.changed`, `inventory.alerts.changed`, `inventory.stock.changed`, `inventory.movements.recorded`,
`inventory.items.changed` (niveles) e `inventory.purchases.changed` (borrador). Auditoría:
`inventory.adjustment_recorded`, `inventory.adjustment_requested`, `inventory.waste_recorded`,
`inventory.adjustment_authorized`, `inventory.adjustment_rejected`, `inventory.transfer_recorded`,
`inventory.count_created`, `inventory.count_updated`, `inventory.count_lines_captured`, `inventory.count_submitted`,
`inventory.count_approved`, `inventory.count_applied`, `inventory.count_cancelled`, `inventory.stock_level_saved`,
`inventory.purchase_order_drafted_from_suggestions`. Orígenes del libro: `adjustment` «Ajuste de inventario», `waste`
«Merma operativa», `transfer` «Transferencia entre almacenes», `count` «Conteo físico».

### Puente nativo

22 operaciones en `api.rs` (`inventory_adjustments`, `inventory_record_adjustment`, `inventory_adjustment`,
`inventory_authorize_adjustment`, `inventory_reject_adjustment`, `inventory_transfers`, `inventory_record_transfer`,
`inventory_transfer`, `inventory_counts`, `inventory_create_count`, `inventory_count`, `inventory_save_count`,
`inventory_capture_count_lines`, `inventory_submit_count`, `inventory_approve_count`, `inventory_apply_count`,
`inventory_cancel_count`, `inventory_stock_levels`, `inventory_save_stock_level`, `inventory_alerts`,
`inventory_purchase_suggestions`, `inventory_draft_order_from_suggestions`); parámetros de consulta nuevos en la lista
blanca `inventory_`: `type`, `severity`, `reason`.

## Etapa 9A · precio de complementos y vigencia de la precuenta (parte A1, esquema 15)

Sin rutas, permisos ni operaciones del puente nuevos. Esquema 15 con la migración
`0015_catalog_price_and_precheck.sql`.

### Catálogo · productos (`catalog/products`, `catalog/products/{id}`, duplicar, lookups)

- Campo nuevo `price_can_decrease` (boolean) en el documento del producto y del combo: «El precio puede
  bajar al cambiar complementos». Siempre presente en las lecturas; `false` por omisión (todos los
  productos existentes). En `POST`/`PATCH` el documento completo se envía tal como se leyó: omitirlo
  equivale a `false` (desactivarla).
- Cambiarlo (como el precio, las presentaciones, los grupos o la tasa de IVA) exige
  `catalog.prices.manage`; sin él, `403 PERMISSION_DENIED` sin cambios. Queda auditado con el guardado
  (`catalog.products.updated`, `before_state`/`after_state` con `price_can_decrease`). Duplicar lo
  conserva. Incrementa la revisión del producto.
- No es un importe: `RedactPrices` no lo oculta.

### Cotizaciones (`catalog/preview`, `restaurant/accounts/{id}/draft-preview`, `…/preview`, `…/edit-preview`)

Desglose del producto (`ProductBreakdown`, también en cada componente de combo y en `quote_line` de
`restaurant_item_versions`):

```json
{
  "base_price": "150.00",
  "group_extras": "-10.00",
  "unit_price": "140.00",
  "price_can_decrease": true,
  "complement_adjustment": "-10.00",
  "groups": [
    {
      "mode": "value",
      "original_value": "35.00",
      "requested_value": "25.00",
      "difference": "-10.00",
      "extras": "0.00"
    }
  ]
}
```

- `price_can_decrease` y `complement_adjustment` solo aparecen cuando la opción **se aplicó** a ese
  cálculo (producto con la opción; en un componente, además el combo con la opción). Sin la opción se
  omiten (`omitempty`): las cotizaciones, borradores y huellas (`expected_fingerprint`) anteriores son
  idénticos byte a byte.
- `complement_adjustment`: reducción respecto de la regla con piso, cadena con signo `"-10.00"` o
  `"0.00"`; nunca positiva. Ya está incluida en `group_extras` y `unit_price` (se mantiene
  `unit_price = base_price + ingredient_extras + group_extras + component_extras`). `group_extras`
  puede ser negativo con la opción.
- `groups[].difference`: con la opción lleva signo (modo valor: solicitado − incluido; modo cantidad:
  tramos − valor de las porciones incluidas retiradas). Sin la opción, como siempre (≥ 0).
- `components[].charge` nunca es negativo: la reducción de los complementos de un componente se aplica
  en el núcleo del combo, no en su cargo.
- Promociones (`promotions[].discount`, `lines[].discount`): con la opción, su base sin extras es
  `(base_price + complement_adjustment) × quantity` (mínimo 0).
- La cocina no recibe ninguno de estos importes (las partes de preparación no llevan importes).

### Cobros · precuentas (interno; sin cambio de respuesta)

- `POST payments/accounts/{id}/precheck/print` guarda además en `payment_documents`
  `account_revision` (revisión de la cuenta) y `adjustment_count` (descuentos y cargos registrados más
  sus cancelaciones), leídos bajo el lock exclusivo 7820148305. Los tickets de pago y las filas
  anteriores a la 0015 los dejan en `NULL` (CHECK `payment_documents_precheck_version_check`). El
  listado `documents` de la cuenta no cambia.
- `payments.AccountStandingSQL` agrega la columna `precheck_current` (predicado único de vigencia):
  precuenta con la revisión y el número de ajustes actuales; las filas `NULL` siguen la regla por
  marcas de tiempo. `precheck_at` y `adjusted_at` se conservan como informativas.
- `restaurant/overview` y `restaurant/tables`: `state = awaiting_payment` ⇔ saldo > 0 y
  `precheck_current`. Mismo contrato JSON; cambia solo cuándo una precuenta deja de estar vigente
  (también con una modificación que esperaba el lock mientras se imprimía).

## Etapa 9A · proveedores, compras de mostrador, código sugerido, categorías y navegación de Inventarios (parte A2, esquema 16)

### Inventario · etapa 9A (fase A, A2): proveedores, compras de mostrador y código sugerido

Migración `0016_inventory_suppliers_counter.sql`. Todas las rutas siguen bajo `/api/v1/inventory`, exigen `inventory.read`
además de su permiso y conservan idempotencia (`operation_id`), revisión, auditoría y eventos de la etapa 8.

#### Proveedores (`suppliers`)

`Supplier` (listado, detalle, alta y edición) agrega:

```json
{
  "phone_secondary": "(33) 3615-0000 ext. 12",
  "mobile_phone": "+52 1 33 1234 5678",
  "website": "https://www.proveedor.com.mx/catalogo",
  "kind": "formal"
}
```

- `kind`: `formal` (todos los existentes y los que se crean) o `counter` («Compras de mostrador», único).
- Cuerpo de `POST suppliers` y `PATCH suppliers/{id}` (`inventory.suppliers.manage`): admite `phone_secondary`,
  `mobile_phone` y `website` opcionales (ausentes = `""`). `kind` **no** se acepta (400 por campo desconocido). El `PATCH`
  sigue guardando todos los datos: un cuerpo sin los campos nuevos los deja vacíos.
- Validación (400 `INVENTORY_VALIDATION`, solo en los campos nuevos): teléfonos de una línea, hasta 40 caracteres, con
  dígitos, espacios, `+ ( ) . - /`, entre 7 y 20 dígitos sin contar una extensión final opcional (`ext. 12`, `ext 12`,
  `x 12`); página web de hasta 200 caracteres, sin espacios, dominio con TLD de letras, puerto y ruta opcionales, con o sin
  `http://`/`https://` (otros esquemas, `localhost` o IP se rechazan).
- Proveedor de mostrador: `PATCH` con `rfc` o `legal_name` no vacíos → 400; se puede renombrar, describir y desactivar.
- `GET suppliers`: nuevo filtro `kind=formal|counter` (otro valor → 400); la búsqueda incluye `phone_secondary`,
  `mobile_phone` y `website`; el orden es `active DESC, kind='counter' DESC, nombre`.
- `PUT suppliers/{id}/items` con el proveedor de mostrador → 409 `COUNTER_SUPPLIER` (no surte insumos: ni preferido ni
  último precio).

`SupplierReference` (en órdenes, recepciones, devoluciones y sugerencias) agrega `kind`:

```json
{ "id": "…", "trade_name": "Compras de mostrador", "kind": "counter", "active": true }
```

#### Órdenes de compra y sugerencias

- `POST purchase-orders`, `PATCH purchase-orders/{id}` (cambiar el proveedor de un borrador) y
  `POST purchase-suggestions/draft-order` con el proveedor de mostrador → 409 `COUNTER_SUPPLIER` («… solo se usa en
  recepciones directas: elige un proveedor formal para la orden de compra.»). Sin escrituras.
- Las sugerencias solo consideran proveedores formales (`sp.kind='formal'`).

#### Recepciones (`receipts`)

- `POST receipts` directa (sin orden; `inventory.purchases.receive` + `inventory.purchases.create`) admite el proveedor de
  mostrador y el campo opcional `counter_store` (comercio concreto, una línea, hasta 100 caracteres, recortado). Con un
  proveedor formal (o en la recepción de una orden) un `counter_store` no vacío → 400. Proveedor de mostrador desactivado →
  409 `SUPPLIER_INACTIVE`.
- Una recepción del proveedor de mostrador **no** actualiza `inventory_supplier_items` (sin último precio).
- `ReceiptSummary`/`Receipt` agregan `counter_store` (`""` en las demás y en las históricas) y `supplier.kind`.
- `GET receipts`: nuevo filtro `supplier_kind=formal|counter` (otro valor → 400); la búsqueda incluye `counter_store`.
- Auditoría `inventory.receipt_recorded`: `after` agrega `supplier_kind` y `counter_store`. Motivo del movimiento:
  `Recepción N · Compras de mostrador (Central de abastos)`.

Ejemplo de cuerpo:

```json
{
  "operation_id": "…",
  "supplier_id": "<id del proveedor de mostrador>",
  "counter_store": "Central de abastos, bodega 12",
  "supplier_reference": "Nota 55",
  "lines": [{ "item_id": "…", "unit_id": "…", "quantity": "3", "unit_price": "40", "tax_rate": 0 }]
}
```

#### Código interno de insumos (`item-codes`, nueva)

`GET inventory/item-codes?code=<código>[&item_id=<insumo que se edita>]` (`inventory.read`; puente `inventory_item_code`,
parámetros `code` e `item_id` en la lista blanca de `api.rs`). Solo lectura; Go sigue respondiendo `DUPLICATE_CODE` al
guardar.

```json
{
  "code": "harina-de-trigo",
  "available": false,
  "suggestion": "harina-de-trigo-2",
  "taken_by": "Harina de trigo"
}
```

- `available`: ningún otro insumo usa el código sin distinguir mayúsculas (se excluye `item_id`).
- `suggestion`: el mismo código si está libre, o el primero libre `base-2`, `base-3`… recortando la base para no pasar de
  40 caracteres y sin guiones al final (misma regla que `suggestProductCode` de `desktop/src/services/productCode.ts`).
- `taken_by`: nombre del insumo que lo usa, o `null`.
- 400: `code` vacío o de más de 40 caracteres o con saltos de línea, `item_id` inválido o parámetros ajenos.

#### Puente nativo (`desktop/src-tauri/src/api.rs`)

- Operación nueva `inventory_item_code` → `GET inventory/item-codes`.
- Parámetros de consulta nuevos para `inventory_*`: `code` y `supplier_kind` (`kind` ya existía).
- Tipos TypeScript: `SupplierKind`, `Supplier.phone_secondary|mobile_phone|website|kind`, `SupplierReference.kind`,
  `ReceiptSummary.counter_store`, `DirectReceiptInput.counter_store`, `SupplierQuery.kind`, `ReceiptQuery.supplier_kind`
  (`services/inventoryPurchasing.ts`) e `ItemCodeCheck` / `checkInventoryItemCode` (`services/inventory.ts`).

#### Esquema (para reportes)

- `app.inventory_suppliers.kind text NOT NULL DEFAULT 'formal'` (`formal|counter`), índice único parcial
  `inventory_suppliers_counter_key` (un solo `counter`), `phone_secondary`, `mobile_phone`, `website` (`text NOT NULL
DEFAULT ''`).
- `app.inventory_receipts.counter_store text NOT NULL DEFAULT ''`.
- Modalidad de una recepción para el reporte de compras: `orden` (`purchase_order_id IS NOT NULL`), `directa` (sin orden y
  proveedor `formal`) o `mostrador` (proveedor `counter`, siempre sin orden).

## Etapa 9A · reportes, día comercial y reconocimiento de ventas (parte B1, esquema 17)

Todas las rutas bajo `/api/v1`, autenticadas como el resto (dispositivo + sesión + permisos leídos en cada
petición). Ninguna ruta de reportes acepta SQL: los reportes salen de un registro cerrado en Go
(`backend/internal/reports`).

### Ejecución de `reports/*`

- La autenticación, los permisos y la validación de forma de los parámetros ocurren en la transacción corta
  habitual de `administration`, que se confirma (una denegación se audita con `access.denied` como en toda la
  API). La consulta corre **después**, en otra transacción `REPEATABLE READ READ ONLY` con
  `statement_timeout=30s`, `lock_timeout=2s` e `idle_in_transaction_session_timeout=45s`, sobre un semáforo
  propio de **2 conexiones** del pool compartido. Sin locks consultivos ni handlers de otros módulos.
- Tiempo de respuesta HTTP de `reports/*`: **40 s** (las demás rutas, 10 s). El puente nativo usa 45 s para
  las operaciones `reports_*`.
- Errores propios (todos con `{"error":{"code","message","request_id"}}` y mensaje en español):
  - `503 REPORTS_BUSY` «Hay otros reportes en curso. Espera unos segundos y vuelve a intentarlo.» (semáforo
    lleno; no espera);
  - `503 REPORT_TIMEOUT` (la consulta excedió su tiempo);
  - `422 REPORT_TOO_LARGE` (más de 100 000 renglones en el conjunto filtrado);
  - `400 VALIDATION_ERROR` (parámetro no admitido por el reporte, periodo inválido, orden por una columna no
    ordenable o protegida, filtro con valor inválido);
  - `403 PERMISSION_DENIED`, `404 NOT_FOUND` (reporte, ruta o fuente de filtros inexistente).

### `GET reports/catalog` (`reports.read`, sin parámetros)

```json
{
  "groups": [
    {"id": "sales", "title": "Ventas", "reports": [
      {"id": "sales.by_day", "title": "Ventas por día", "description": "…", "date_basis": "Fecha de cierre…",
       "period": "required", "comparison": false,
       "filters": [{"key": "mode", "label": "Modalidad", "type": "choice", "options": [{"value": "dine_in", "label": "Salón"}]},
                   {"key": "category_id", "label": "Categoría", "type": "id", "source": "categories"}],
       "columns": [{"key": "day", "label": "Día", "type": "date", "visibility": "default", "total": "none", "sortable": true}],
       "default_sort": {"key": "day", "direction": "asc"},
       "export": {"pdf": false, "excel": false}}]}],
  "periods": [{"value": "today", "label": "Hoy"}, …],
  "dashboard": true, "timezone": "America/Monterrey", "day_start": "05:00"
}
```

Solo incluye los grupos con reportes que el usuario puede consultar y, en cada reporte, solo las columnas que
puede ver. `export.*` refleja `reports.export.pdf` / `reports.export.excel` (las exportaciones son de la fase C).

### `GET reports/run/{id}` (`reports.read` + los permisos del reporte)

Parámetros (cualquier otro → 400):

| Parámetro           | Valores                                                                                                                                                                                                                                                                                                                           |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `period`            | `today` (por omisión), `yesterday`, `current_week`, `current_month`, `current_year`, `day` (+`date=AAAA-MM-DD`), `iso_week` (+`week=AAAA-Wss`), `month` (+`month=AAAA-MM`), `year` (+`year=AAAA`), `range` (+`from`, `until`, AAAA-MM-DD incluidos, ≤ 400 días). No se admite en reportes de situación actual (`period: "none"`). |
| `compare`           | `previous` (periodo anterior equivalente; solo si `comparison: true`) o `none`                                                                                                                                                                                                                                                    |
| `page`, `page_size` | desde 1; `page_size` 1–200 (50 por omisión)                                                                                                                                                                                                                                                                                       |
| `sort`, `direction` | clave de una columna `sortable` visible; `asc`/`desc`                                                                                                                                                                                                                                                                             |
| filtros             | los que declara el reporte: `mode`, `area_id`, `table_id`, `category_id`, `product_id`, `price_list_id`, `origin`, `register_id`, `shift_id`, `method_id`, `user_id`, `reason_id` (UUID o `none` = «Sin motivo del catálogo»), `kind`, `state`, `group_by`, `scope`                                                               |

Respuesta (modelo único; las exportaciones usan el mismo resultado):

```json
{
  "report": {
    "id": "sales.by_day",
    "group": "sales",
    "title": "Ventas por día",
    "description": "…"
  },
  "meta": {
    "title": "Ventas por día",
    "period": {
      "kind": "current_week",
      "label": "Semana 46 de 2026 (del 9 al 15 de noviembre de 2026)",
      "first_day": "2026-11-09",
      "last_day": "2026-11-15",
      "from": "2026-11-09T11:00:00Z",
      "until": "2026-11-16T11:00:00Z",
      "days": 7,
      "iso_year": 2026,
      "iso_week": 46,
      "in_progress": true
    },
    "comparison": null,
    "filters": [
      { "key": "category_id", "label": "Categoría", "value": "…uuid…", "display": "Bebidas" }
    ],
    "generated_at": "2026-11-10T18:00:00Z",
    "timezone": "America/Monterrey",
    "day_start": "05:00",
    "date_basis": "Fecha de cierre de la cuenta (reconocimiento de la venta), con la hora de inicio del día.",
    "warnings": [{ "code": "SHIFT_CLOSING", "message": "Turnos en arqueo excluidos: 1. …" }],
    "notes": ["…"]
  },
  "columns": [
    {
      "key": "net",
      "label": "Venta neta",
      "type": "money_mxn",
      "visibility": "default",
      "total": "sum",
      "sortable": true,
      "description": "…"
    }
  ],
  "rows": [{ "day": "2026-11-10", "accounts": 12, "net": "4520.00", "average_ticket": "376.67" }],
  "totals": { "day": null, "accounts": 12, "net": "4520.00", "average_ticket": "376.67" },
  "summary": [{ "key": "accounts", "label": "Cuentas cerradas", "type": "integer", "value": 12 }],
  "page": { "page": 1, "page_size": 50, "total_rows": 7, "total_pages": 1 },
  "sort": { "key": "day", "direction": "asc" }
}
```

Formato de valores por tipo de columna: `money_mxn`, `money_usd`, `percent` → cadena con dos decimales;
`quantity` → cadena decimal exacta; `integer` → número; `date` → `AAAA-MM-DD` (día comercial); `datetime` →
RFC 3339 en UTC (mostrar en `meta.timezone`); `text`/`code` → cadena; ausente → `null`. Los **totales** se
calculan en Go sobre **todo** el conjunto filtrado (no la página). Las columnas protegidas por permiso (por
ejemplo, el efectivo físico de `payments.list` sin `cash.expected.read`) no aparecen en `columns`, `rows`,
`totals` ni `summary`, y no se puede ordenar por ellas.

#### Reportes registrados (fase B1)

| Grupo              | id                                                                                                                                                                                                                                                                                                                                                                                 | Permisos                                       | Fecha                                         |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------- | --------------------------------------------- |
| Ventas             | `sales.summary` (por tasa de IVA), `sales.by_day`, `sales.by_week` (ISO), `sales.by_month`, `sales.by_hour`, `sales.by_product`, `sales.by_variant`, `sales.by_mode`, `sales.dine_in` (área y mesa), `sales.takeaway` (origen), `sales.by_price_list`, `sales.by_shift`, `sales.accounts` (cierres), `sales.evolution` (con anterior), `sales.in_progress` (En curso, sin periodo) | `reports.sales.read`                           | cierre de la cuenta (reconocimiento)          |
| Categorías         | `sales.by_category`                                                                                                                                                                                                                                                                                                                                                                | `reports.sales.read`                           | cierre                                        |
| Productos          | `products.top`, `products.bottom` (incluye activos sin ventas), `products.combos`, `products.combo_components` (sin importe), `products.complements`, `products.cancelled` (fecha de cancelación), `products.discounted`, `products.trends` (con anterior)                                                                                                                         | `reports.sales.read`                           | cierre (cancelados: cancelación)              |
| Cocina y operación | `kitchen.demand_by_product`, `kitchen.demand_by_hour`, `kitchen.stations`                                                                                                                                                                                                                                                                                                          | `reports.sales.read`                           | liberación a cocina                           |
| Pagos              | `payments.by_method`, `payments.by_register`, `payments.by_shift`, `payments.by_user`, `payments.by_day`, `payments.list`, `payments.mixed_partial`, `payments.refunds`, `payments.corrections`, `payments.tips`, `payments.pending` (sin periodo)                                                                                                                                 | `reports.payments.read`                        | registro del cobro / devolución               |
| Pagos              | `payments.currency` (pesos y dólares físicos)                                                                                                                                                                                                                                                                                                                                      | `reports.payments.read` + `cash.expected.read` | movimiento de efectivo                        |
| Descuentos         | `discounts.summary`, `discounts.by_reason`, `discounts.by_user`, `discounts.by_product`, `discounts.list`, `discounts.general`, `discounts.courtesies`, `discounts.authorizations`                                                                                                                                                                                                 | `reports.discounts.read`                       | cierre de la venta (autorizaciones: su fecha) |

### `GET reports/dashboard` (`reports.read`)

Parámetros: los de periodo y `compare` (por omisión `previous`). Cada sección solo se calcula y envía con su
permiso (`sales` con `reports.sales.read`, `payments` con `reports.payments.read`, `discounts` con
`reports.discounts.read`); `report_id` solo aparece si el usuario puede abrir ese reporte.

```json
{
  "period": {…}, "comparison": {…}, "generated_at": "…", "timezone": "…", "day_start": "05:00",
  "sections": [{"id": "sales", "title": "Ventas", "indicators": [
    {"key": "net_sales", "label": "Ventas netas", "type": "money_mxn", "value": "4520.00", "previous": "3980.00",
     "change": "540.00", "change_percent": "13.57", "report_id": "sales.by_day", "hint": "…"}]}],
  "series": [{"key": "net_sales_by_day", "label": "Venta neta por día", "type": "money_mxn", "report_id": "sales.by_day",
              "points": [{"date": "2026-11-09", "value": "3980.00"}]}],
  "warnings": []
}
```

Indicadores: ventas `sales`, `net_sales`, `accounts`, `average_ticket`, `units`, `open_accounts` y
`open_balance` (situación actual, sin valor anterior); cobros `collected`, `payments`, `refunds`, `tips`;
descuentos `discounts`, `courtesies`, `discount_percent`. `change_percent` es `null` si el anterior es cero.
El dashboard no se recalcula con SSE (la pantalla lo pide o usa un temporizador).

### `GET reports/filters/{source}` (`reports.read`)

`source`: `registers`, `shifts` (200 más recientes), `users` (sin `sa`), `areas`, `tables`, `products` (con
`search`, hasta 100), `categories`, `payment_methods`, `price_lists`, `discount_reasons` (primero
`{"value":"none","label":"Sin motivo del catálogo"}`), `origins`. Respuesta
`{"source": "…", "options": [{"value", "label", "detail"}]}`. Solo nombres y estados, nunca importes.

### `GET|PUT settings/business-day` («Hora de inicio del día»)

- `GET` (`settings.read`): `{"operating_day_cutoff": "05:00", "timezone": "America/Monterrey", "revision": 3,
"updated_at": "…", "updated_by_name": "…"}`.
- `PUT` (`settings.manage`): `{"operating_day_cutoff": "HH:MM", "revision": n}` → la configuración guardada;
  `400` formato, `409 REVISION_CONFLICT`; auditoría `settings.business_day_changed` (antes/después) y evento
  `inventory.settings.changed`. Es la misma fila y revisión que `inventory/settings`.
- `PUT inventory/settings`: `operating_day_cutoff` pasa a ser **opcional** y solo se acepta sin cambios
  (diferente → `400` «La hora de inicio del día se cambia en Configuración general.»). `GET inventory/settings`
  sigue devolviéndola.

### Puente nativo (`desktop/src-tauri/src/api.rs`)

Operaciones nuevas: `reports_catalog` (GET `reports/catalog`), `reports_dashboard` (GET `reports/dashboard`),
`reports_run` (GET `reports/run/{id}`), `reports_filters` (GET `reports/filters/{source}`),
`settings_business_day` (GET) y `save_settings_business_day` (PUT `settings/business-day`). `reports_run` y
`reports_filters` reciben como identificador la clave del registro (minúsculas, `.` y `_`, ≤ 64, sin `..` ni
barras), no un UUID. Lista blanca de parámetros de `reports_*`: `period date week month year from until compare
page page_size sort direction search mode area_id table_id category_id product_id price_list_id origin
register_id shift_id method_id user_id reason_id kind state group_by scope`. Tiempo de espera 45 s.

Tipos TypeScript: `desktop/src/services/reports.ts`.

### Permisos nuevos (0017, solo al administrador)

`reports.read`, `reports.sales.read`, `reports.payments.read`, `reports.discounts.read`, `reports.cash.read`,
`reports.inventory.read`, `reports.purchases.read`, `reports.employees.read`, `reports.export.pdf`,
`reports.export.excel`. Pantalla de inicio `reports` (`reports.read`) en `administration/preferences.go`.

## Etapa 9A · reportes de caja, inventarios, costos y márgenes, compras, proveedores y empleados (parte B2)

Todos los reportes de la fase B2 usan el contrato de la fase B1, sin rutas nuevas: `GET reports/catalog`,
`GET reports/run/{id}` (resultado único `report`, `meta`, `columns`, `rows`, `totals`, `summary`, `page`, `sort`),
`GET reports/dashboard` y `GET reports/filters/{source}`. Dinero, porcentajes y cantidades como cadenas calculadas
en Go; enteros como número; fechas `AAAA-MM-DD`; instantes RFC 3339 (UTC). Una columna con permiso propio
(efectivo físico, esperado, diferencias, costos, valuación, datos del gasto de caja, venta en caja) **no existe** en
`columns`, `rows`, `totals` ni en el catálogo para quien no tiene el permiso, no se puede usar en `sort` y no se
exporta; una cifra resumida protegida tampoco viaja. Los parámetros no declarados por el reporte se rechazan
(400 `VALIDATION_ERROR`). Las exportaciones de la fase C usan los mismos `id`, parámetros y permisos.

### Reportes nuevos (45)

Columnas: `clave:tipo/total[permisos adicionales]` (`sum` = Go suma todo el conjunto filtrado; `custom` = lo
calcula el reporte; sin total = celda vacía). Además de los permisos de cada fila se exige `reports.read`.

| id                                 | Título                              | Permisos                                          | Periodo     | Filtros                                                          | Columnas protegidas                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| ---------------------------------- | ----------------------------------- | ------------------------------------------------- | ----------- | ---------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cash.shifts`                      | Turnos de caja                      | reports.cash.read                                 | periodo     | register_id, user_id, state                                      | expected_mxn(cash.expected.read), expected_usd(cash.expected.read), mxn_in(cash.expected.read), mxn_out(cash.expected.read), usd_in(cash.expected.read), usd_out(cash.expected.read), difference_mxn(cash.expected.read), difference_usd(cash.expected.read), difference_equivalent_mxn(cash.expected.read), result(cash.expected.read)                                                                                                                                                                                     |
| `cash.sales_collections_by_shift`  | Ventas y cobros por turno           | reports.cash.read                                 | periodo     | register_id, user_id                                             | accounts(reports.sales.read), units(reports.sales.read), recognized_net(reports.sales.read), recognized_tax(reports.sales.read)                                                                                                                                                                                                                                                                                                                                                                                             |
| `cash.by_register`                 | Historial por caja                  | reports.cash.read                                 | periodo     | register_id                                                      | over_mxn(cash.expected.read), short_mxn(cash.expected.read), difference_mxn(cash.expected.read), over_usd(cash.expected.read), short_usd(cash.expected.read), difference_usd(cash.expected.read), difference_equivalent_mxn(cash.expected.read)                                                                                                                                                                                                                                                                             |
| `cash.by_day`                      | Caja por día                        | reports.cash.read                                 | periodo     | register_id, shift_id                                            | mxn_in(cash.expected.read), mxn_out(cash.expected.read), usd_in(cash.expected.read), usd_out(cash.expected.read), cash_flow_mxn(cash.expected.read), cash_flow_usd(cash.expected.read), closing_expected_mxn(cash.expected.read), closing_expected_usd(cash.expected.read), over_mxn(cash.expected.read), short_mxn(cash.expected.read), difference_mxn(cash.expected.read), over_usd(cash.expected.read), short_usd(cash.expected.read), difference_usd(cash.expected.read), difference_equivalent_mxn(cash.expected.read) |
| `cash.operations`                  | Movimientos de efectivo             | reports.cash.read                                 | periodo     | kind, currency, register_id, shift_id, user_id                   |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `cash.cuts`                        | Cortes parciales y cierres          | reports.cash.read                                 | periodo     | register_id, shift_id, kind                                      | expected_mxn(cash.expected.read), expected_usd(cash.expected.read), difference_mxn(cash.expected.read), difference_usd(cash.expected.read)                                                                                                                                                                                                                                                                                                                                                                                  |
| `cash.by_user`                     | Operaciones de caja por responsable | reports.cash.read                                 | periodo     | register_id, user_id                                             |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `inventory.stock`                  | Existencias actuales                | reports.inventory.read + inventory.stock.read     | sin periodo | category_id, item_id, state                                      | average_cost(inventory.costs.read), value(inventory.costs.read+inventory.valuation.read), cost_pending(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                |
| `inventory.stock_by_warehouse`     | Existencias por almacén             | reports.inventory.read + inventory.stock.read     | sin periodo | warehouse_id, category_id, item_id, state                        |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `inventory.negative`               | Insumos con existencia negativa     | reports.inventory.read + inventory.stock.read     | sin periodo | warehouse_id, category_id                                        | cost_pending(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `inventory.below_minimum`          | Insumos bajo mínimo                 | reports.inventory.read + inventory.stock.read     | sin periodo | warehouse_id, category_id                                        |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `inventory.valuation`              | Valuación del inventario            | reports.inventory.read + inventory.valuation.read | sin periodo | category_id                                                      | quantity(inventory.stock.read), average_cost(inventory.costs.read), value(inventory.costs.read), cost_pending(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                         |
| `inventory.average_costs`          | Costos promedio                     | reports.inventory.read + inventory.costs.read     | sin periodo | category_id, item_id                                             | quantity(inventory.stock.read), average_cost(inventory.costs.read), last_valid_cost(inventory.costs.read), reference_cost(inventory.costs.read), pending_cost_quantity(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                |
| `inventory.kardex`                 | Kardex                              | reports.inventory.read + inventory.movements.read | periodo     | item_id, warehouse_id, category_id, kind                         | unit_cost(inventory.costs.read), value(inventory.costs.read), cost_basis(inventory.costs.read), cost_pending(inventory.costs.read), valuation_after(inventory.costs.read+inventory.valuation.read)                                                                                                                                                                                                                                                                                                                          |
| `inventory.entries`                | Entradas de inventario              | reports.inventory.read + inventory.movements.read | periodo     | item_id, warehouse_id, category_id, kind                         | value(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `inventory.outflows`               | Salidas de inventario               | reports.inventory.read + inventory.movements.read | periodo     | item_id, warehouse_id, category_id, kind                         | value(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `inventory.transfers`              | Transferencias                      | reports.inventory.read + inventory.movements.read | periodo     | item_id, warehouse_id, category_id                               | value(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `inventory.adjustments`            | Ajustes de inventario               | reports.inventory.read + inventory.movements.read | periodo     | item_id, warehouse_id, state                                     | value(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `inventory.waste`                  | Mermas                              | reports.inventory.read + inventory.movements.read | periodo     | kind, item_id, warehouse_id                                      | value(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `inventory.counts`                 | Conteos físicos                     | reports.inventory.read + inventory.stock.read     | periodo     | warehouse_id, state                                              | applied_value(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `inventory.count_differences`      | Diferencias de conteo               | reports.inventory.read + inventory.stock.read     | periodo     | warehouse_id, item_id, category_id                               | value(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `inventory.consumption_by_item`    | Consumo por ingrediente             | reports.inventory.read + inventory.movements.read | periodo     | item_id, warehouse_id, category_id                               | value(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `inventory.consumption_by_day`     | Consumo por periodo                 | reports.inventory.read + inventory.movements.read | periodo     | item_id, warehouse_id, category_id                               | value(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `inventory.consumption_by_product` | Consumo por producto                | reports.inventory.read + inventory.movements.read | periodo     | product_id, item_id, warehouse_id                                | value(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `inventory.consumption_by_kitchen` | Consumo asociado a cocina           | reports.inventory.read + inventory.movements.read | periodo     | warehouse_id, mode                                               | value(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `purchases.received`               | Compras recibidas por periodo       | reports.purchases.read                            | periodo     | supplier_id, item_id, category_id, warehouse_id, modality, state | inventory_value(inventory.costs.read), unit_cost(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `purchases.ordered`                | Compras ordenadas                   | reports.purchases.read                            | periodo     | supplier_id, item_id, category_id, warehouse_id, state           |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `purchases.pending`                | Pendiente de recepción              | reports.purchases.read                            | sin periodo | supplier_id, item_id, warehouse_id                               |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `purchases.by_supplier`            | Compras por proveedor               | reports.purchases.read                            | periodo     | supplier_id, modality, warehouse_id                              |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `purchases.by_item`                | Compras por insumo                  | reports.purchases.read                            | periodo     | supplier_id, item_id, category_id, warehouse_id, modality, state |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `purchases.supplier_returns`       | Devoluciones a proveedor            | reports.purchases.read                            | periodo     | supplier_id, item_id, warehouse_id                               | outflow_value(inventory.costs.read), unabsorbed(inventory.costs.read)                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `purchases.cash_links`             | Compras pagadas desde caja          | reports.purchases.read                            | periodo     | supplier_id, state                                               | expense_at(cash.movements.read+cash.shifts.read_all+cash.history.read), register(cash.movements.read+cash.shifts.read_all+cash.history.read), concept(cash.movements.read+cash.shifts.read_all+cash.history.read)                                                                                                                                                                                                                                                                                                           |
| `suppliers.activity`               | Proveedores                         | reports.purchases.read                            | periodo     | state                                                            |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `costs.margin_by_product`          | Margen estimado por producto        | reports.sales.read + inventory.costs.read         | periodo     | category_id, product_id                                          |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `costs.margin_by_category`         | Margen estimado por categoría       | reports.sales.read + inventory.costs.read         | periodo     | category_id                                                      |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `costs.margin_by_period`           | Margen estimado por periodo         | reports.sales.read + inventory.costs.read         | periodo     | category_id, product_id                                          |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `costs.cost_status`                | Estado del costo de las ventas      | reports.sales.read + inventory.costs.read         | periodo     | category_id, product_id, state                                   |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `costs.theoretical_products`       | Costo teórico por producto          | reports.sales.read + inventory.costs.read         | sin periodo | category_id, product_id                                          |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `costs.theoretical_recipes`        | Costo teórico por receta            | reports.sales.read + inventory.costs.read         | sin periodo | kind                                                             |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `costs.consumption_valued`         | Consumo valuado                     | reports.sales.read + inventory.costs.read         | periodo     | category_id, item_id, warehouse_id                               |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `costs.waste`                      | Costo de mermas                     | reports.sales.read + inventory.costs.read         | periodo     | warehouse_id                                                     |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `costs.cost_adjustments`           | Ajustes de costo                    | reports.sales.read + inventory.costs.read         | periodo     | item_id, category_id                                             |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `employees.activity`               | Actividad por empleado              | reports.employees.read                            | periodo     | user_id                                                          |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `employees.responsible_sales`      | Ventas por responsable              | reports.employees.read                            | periodo     | user_id, category_id, product_id                                 |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `employees.confirmations`          | Confirmaciones por empleado         | reports.employees.read                            | periodo     | user_id, product_id                                              |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |

#### Columnas por reporte

- `cash.shifts` (orden por omisión `opened_at`): register:text, business_date:date, state:text, opened_at:datetime, closed_at:datetime, opened_by:text, closed_by:text, opening_float:money_mxn/sum, opening_float_usd:money_usd/sum, usd_rate:quantity, payments:integer/sum, sales:money_mxn/sum, tips:money_mxn/sum, refunds:money_mxn/sum, net:money_mxn/sum, cash_net:money_mxn/sum, income_mxn:money_mxn/sum, withdrawals_mxn:money_mxn/sum, expenses_mxn:money_mxn/sum, corrections_in_mxn:money_mxn/sum, corrections_out_mxn:money_mxn/sum, income_usd:money_usd/sum, withdrawals_usd:money_usd/sum, expenses_usd:money_usd/sum, regularizations:integer/sum, partial_cuts:integer/sum, counted_mxn:money_mxn/sum, counted_usd:money_usd/sum, expected_mxn:money_mxn/sum[cash.expected.read], expected_usd:money_usd/sum[cash.expected.read], mxn_in:money_mxn/sum[cash.expected.read], mxn_out:money_mxn/sum[cash.expected.read], usd_in:money_usd/sum[cash.expected.read], usd_out:money_usd/sum[cash.expected.read], difference_mxn:money_mxn/sum[cash.expected.read], difference_usd:money_usd/sum[cash.expected.read], difference_equivalent_mxn:money_mxn/sum[cash.expected.read], result:text[cash.expected.read]
- `cash.sales_collections_by_shift` (orden por omisión `business_date`): register:text, business_date:date, state:text, opened_by:text, accounts:integer/sum[reports.sales.read], units:integer/sum[reports.sales.read], recognized_net:money_mxn/sum[reports.sales.read], recognized_tax:money_mxn/sum[reports.sales.read], payments:integer/sum, collected:money_mxn/sum, net_tips:money_mxn/sum, refunds:money_mxn/sum
- `cash.by_register` (orden por omisión `register`): register:text, shifts:integer/sum, closed_shifts:integer/sum, closing_shifts:integer/sum, opening_float:money_mxn/sum, payments:integer/sum, sales:money_mxn/sum, tips:money_mxn/sum, refunds:money_mxn/sum, net:money_mxn/sum, cash_net:money_mxn/sum, income_mxn:money_mxn/sum, withdrawals_mxn:money_mxn/sum, expenses_mxn:money_mxn/sum, income_usd:money_usd/sum, withdrawals_usd:money_usd/sum, expenses_usd:money_usd/sum, partial_cuts:integer/sum, counted_mxn:money_mxn/sum, counted_usd:money_usd/sum, over_mxn:money_mxn/sum[cash.expected.read], short_mxn:money_mxn/sum[cash.expected.read], difference_mxn:money_mxn/sum[cash.expected.read], over_usd:money_usd/sum[cash.expected.read], short_usd:money_usd/sum[cash.expected.read], difference_usd:money_usd/sum[cash.expected.read], difference_equivalent_mxn:money_mxn/sum[cash.expected.read]
- `cash.by_day` (orden por omisión `day`): day:date, openings:integer/sum, opening_float:money_mxn/sum, opening_float_usd:money_usd/sum, payments:integer/sum, sales:money_mxn/sum, tips:money_mxn/sum, refunds:money_mxn/sum, net:money_mxn/sum, cash_net:money_mxn/sum, operations:integer/sum, income_mxn:money_mxn/sum, withdrawals_mxn:money_mxn/sum, expenses_mxn:money_mxn/sum, corrections_in_mxn:money_mxn/sum, corrections_out_mxn:money_mxn/sum, income_usd:money_usd/sum, withdrawals_usd:money_usd/sum, expenses_usd:money_usd/sum, partial_cuts:integer/sum, closings:integer/sum, counted_mxn:money_mxn/sum, counted_usd:money_usd/sum, mxn_in:money_mxn/sum[cash.expected.read], mxn_out:money_mxn/sum[cash.expected.read], usd_in:money_usd/sum[cash.expected.read], usd_out:money_usd/sum[cash.expected.read], cash_flow_mxn:money_mxn/sum[cash.expected.read], cash_flow_usd:money_usd/sum[cash.expected.read], closing_expected_mxn:money_mxn/sum[cash.expected.read], closing_expected_usd:money_usd/sum[cash.expected.read], over_mxn:money_mxn/sum[cash.expected.read], short_mxn:money_mxn/sum[cash.expected.read], difference_mxn:money_mxn/sum[cash.expected.read], over_usd:money_usd/sum[cash.expected.read], short_usd:money_usd/sum[cash.expected.read], difference_usd:money_usd/sum[cash.expected.read], difference_equivalent_mxn:money_mxn/sum[cash.expected.read]
- `cash.operations` (orden por omisión `created_at`): created_at:datetime, register:text, business_date:date, kind:text, type:text, concept:text, currency:text, in_mxn:money_mxn/sum, out_mxn:money_mxn/sum, in_usd:money_usd/sum, out_usd:money_usd/sum, regularization_mxn:money_mxn/sum, regularization_usd:money_usd/sum, usd_rate:quantity, user:text, authorized_by:text, authorization:text, reference:text
- `cash.cuts` (orden por omisión `created_at`): created_at:datetime, register:text, business_date:date, cut:text, covered_from:datetime, covered_to:datetime, created_by:text, payments:integer/sum, sales:money_mxn/sum, tips:money_mxn/sum, refunds:money_mxn/sum, income_mxn:money_mxn/sum, withdrawals_mxn:money_mxn/sum, expenses_mxn:money_mxn/sum, expected_mxn:money_mxn/sum[cash.expected.read], expected_usd:money_usd/sum[cash.expected.read], difference_mxn:money_mxn/sum[cash.expected.read], difference_usd:money_usd/sum[cash.expected.read]
- `cash.by_user` (orden por omisión `user`): user:text, openings:integer/sum, closings:integer/sum, partial_cuts:integer/sum, operations:integer/sum, income_mxn:money_mxn/sum, withdrawals_mxn:money_mxn/sum, expenses_mxn:money_mxn/sum, income_usd:money_usd/sum, withdrawals_usd:money_usd/sum, expenses_usd:money_usd/sum, corrections:integer/sum, regularizations:integer/sum, authorizations:integer/sum
- `inventory.stock` (orden por omisión `item`): code:code, item:text, category:text, unit:text, quantity:quantity, min_stock:quantity, reorder_point:quantity, status:text, average_cost:quantity[inventory.costs.read], value:money_mxn/sum[inventory.costs.read+inventory.valuation.read], cost_pending:text[inventory.costs.read]
- `inventory.stock_by_warehouse` (orden por omisión `warehouse`): warehouse:text, code:code, item:text, category:text, unit:text, quantity:quantity, min_stock:quantity, reorder_point:quantity, level_scope:text, status:text, updated_at:datetime
- `inventory.negative` (orden por omisión `quantity`): warehouse:text, code:code, item:text, category:text, unit:text, quantity:quantity, updated_at:datetime, cost_pending:text[inventory.costs.read]
- `inventory.below_minimum` (orden por omisión `item`): scope:text, code:code, item:text, category:text, unit:text, quantity:quantity, min_stock:quantity, reorder_point:quantity, missing:quantity
- `inventory.valuation` (orden por omisión `item`): code:code, item:text, category:text, unit:text, quantity:quantity[inventory.stock.read], average_cost:quantity[inventory.costs.read], value:money_mxn/sum[inventory.costs.read], cost_pending:text[inventory.costs.read]
- `inventory.average_costs` (orden por omisión `item`): code:code, item:text, category:text, unit:text, quantity:quantity[inventory.stock.read], average_cost:quantity[inventory.costs.read], last_valid_cost:quantity[inventory.costs.read], reference_cost:quantity[inventory.costs.read], pending_cost_quantity:quantity[inventory.costs.read]
- `inventory.kardex` (orden por omisión `occurred_at`): occurred_at:datetime, code:code, item:text, warehouse:text, kind:text, source:text, quantity:quantity, unit:text, stock_after:quantity, unit_cost:quantity[inventory.costs.read], value:money_mxn/sum[inventory.costs.read], cost_basis:text[inventory.costs.read], cost_pending:text[inventory.costs.read], valuation_after:money_mxn/custom[inventory.costs.read+inventory.valuation.read], responsible:text, reason:text, recorded_at:datetime
- `inventory.entries` (orden por omisión `item`): code:code, item:text, category:text, kind:text, movements:integer/sum, quantity:quantity, unit:text, value:money_mxn/sum[inventory.costs.read]
- `inventory.outflows` (orden por omisión `item`): code:code, item:text, category:text, kind:text, movements:integer/sum, quantity:quantity, unit:text, value:money_mxn/sum[inventory.costs.read]
- `inventory.transfers` (orden por omisión `occurred_at`): occurred_at:datetime, folio:code, from_warehouse:text, to_warehouse:text, code:code, item:text, quantity:quantity, unit:text, actor:text, value:money_mxn/sum[inventory.costs.read], notes:text
- `inventory.adjustments` (orden por omisión `occurred_at`): occurred_at:datetime, folio:code, state:text, warehouse:text, reason:text, code:code, item:text, quantity:quantity, unit:text, requested_by:text, authorized_by:text, value:money_mxn/sum[inventory.costs.read]
- `inventory.waste` (orden por omisión `occurred_at`): occurred_at:datetime, origin:text, classification:text, folio:code, product:text, warehouse:text, code:code, item:text, quantity:quantity, unit:text, responsible:text, value:money_mxn/sum[inventory.costs.read]
- `inventory.counts` (orden por omisión `created_at`): created_at:datetime, folio:code, warehouse:text, scope:text, state:text, lines:integer/sum, counted_lines:integer/sum, difference_lines:integer/sum, created_by:text, applied_at:datetime, applied_by:text, applied_value:money_mxn/sum[inventory.costs.read]
- `inventory.count_differences` (orden por omisión `captured_at`): captured_at:datetime, folio:code, warehouse:text, state:text, code:code, item:text, unit:text, theoretical:quantity, counted:quantity, difference:quantity, captured_by:text, value:money_mxn/sum[inventory.costs.read]
- `inventory.consumption_by_item` (orden por omisión `item`): code:code, item:text, category:text, unit:text, quantity:quantity, units:integer/sum, value:money_mxn/sum[inventory.costs.read]
- `inventory.consumption_by_day` (orden por omisión `day`): day:date, units:integer/sum, movements:integer/sum, quantity:quantity, value:money_mxn/sum[inventory.costs.read]
- `inventory.consumption_by_product` (orden por omisión `product`): product:text, code:code, item:text, unit:text, quantity:quantity, units:integer/sum, value:money_mxn/sum[inventory.costs.read]
- `inventory.consumption_by_kitchen` (orden por omisión `warehouse`): warehouse:text, mode:text, units:integer/sum, movements:integer/sum, items:integer/sum, value:money_mxn/sum[inventory.costs.read]
- `purchases.received` (orden por omisión `occurred_at`): occurred_at:datetime, receipt_folio:code, order_folio:code, modality:text, supplier:text, counter_store:text, warehouse:text, code:code, item:text, category:text, quantity:quantity, capture_unit:text, base_quantity:quantity, base_unit:text, unit_price:quantity, discount:money_mxn/sum, subtotal:money_mxn/sum, tax_rate:integer, tax:money_mxn/sum, total:money_mxn/sum, received_by:text, supplier_reference:text, inventory_value:money_mxn/sum[inventory.costs.read], unit_cost:quantity[inventory.costs.read]
- `purchases.ordered` (orden por omisión `order_date`): order_date:date, folio:code, state:text, supplier:text, warehouse:text, code:code, item:text, category:text, quantity:quantity, capture_unit:text, unit_price:quantity, discount:money_mxn/sum, subtotal:money_mxn/sum, tax_rate:integer, tax:money_mxn/sum, total:money_mxn/sum, base_quantity:quantity, received_quantity:quantity, pending_quantity:quantity, base_unit:text, receipts:integer/sum, received_total:money_mxn/sum
- `purchases.pending` (orden por omisión `order_date`): order_date:date, folio:code, state:text, supplier:text, warehouse:text, code:code, item:text, base_quantity:quantity, received_quantity:quantity, pending_quantity:quantity, base_unit:text, pending_total:money_mxn/sum
- `purchases.by_supplier` (orden por omisión `total`): supplier:text, kind:text, receipts:integer/sum, subtotal:money_mxn/sum, discount:money_mxn/sum, tax:money_mxn/sum, total:money_mxn/sum, ordered_total:money_mxn/sum, returned_value:money_mxn/sum, cash_linked:money_mxn/sum
- `purchases.by_item` (orden por omisión `total`): code:code, item:text, category:text, base_quantity:quantity, base_unit:text, receipts:integer/sum, discount:money_mxn/sum, subtotal:money_mxn/sum, tax:money_mxn/sum, total:money_mxn/sum, average_price:quantity
- `purchases.supplier_returns` (orden por omisión `occurred_at`): occurred_at:datetime, folio:code, receipt_folio:code, supplier:text, warehouse:text, code:code, item:text, base_quantity:quantity, base_unit:text, value:money_mxn/sum, reason:text, actor:text, outflow_value:money_mxn/sum[inventory.costs.read], unabsorbed:money_mxn/sum[inventory.costs.read]
- `purchases.cash_links` (orden por omisión `occurred_at`): occurred_at:datetime, receipt_folio:code, supplier:text, linked:money_mxn/sum, status:text, linked_at:datetime, linked_by:text, expense_at:datetime[cash.movements.read+cash.shifts.read_all+cash.history.read], register:text[cash.movements.read+cash.shifts.read_all+cash.history.read], concept:text[cash.movements.read+cash.shifts.read_all+cash.history.read]
- `suppliers.activity` (orden por omisión `supplier`): supplier:text, kind:text, active:text, contact:text, phone:text, mobile_phone:text, email:text, receipts:integer/sum, total:money_mxn/sum, last_receipt_at:datetime, open_orders:integer/sum
- `costs.margin_by_product` (orden por omisión `base`): product:text, category:text, units:integer/sum, net:money_mxn/sum, base:money_mxn/sum, costed_base:money_mxn/sum, cost:money_mxn/sum, margin:money_mxn/sum, margin_percent:percent/custom, firm_share:percent/custom, firm_units:integer/sum, estimated_units:integer/sum, uncosted_units:integer/sum, status:text
- `costs.margin_by_category` (orden por omisión `base`): category:text, units:integer/sum, net:money_mxn/sum, base:money_mxn/sum, costed_base:money_mxn/sum, cost:money_mxn/sum, margin:money_mxn/sum, margin_percent:percent/custom, firm_share:percent/custom, firm_units:integer/sum, estimated_units:integer/sum, uncosted_units:integer/sum, status:text
- `costs.margin_by_period` (orden por omisión `day`): day:date, units:integer/sum, net:money_mxn/sum, base:money_mxn/sum, costed_base:money_mxn/sum, cost:money_mxn/sum, margin:money_mxn/sum, margin_percent:percent/custom, firm_share:percent/custom, firm_units:integer/sum, estimated_units:integer/sum, uncosted_units:integer/sum, status:text, cost_adjustments:money_mxn/sum
- `costs.cost_status` (orden por omisión `product`): product:text, status:text, reason:text, units:integer/sum, base:money_mxn/sum, cost:money_mxn/sum
- `costs.theoretical_products` (orden por omisión `product`): product:text, variant:text, recipe:text, version:integer/sum, price:money_mxn/sum, price_base:money_mxn/sum, unit_cost:money_mxn/sum, takeaway_cost:money_mxn/sum, margin:money_mxn/sum, margin_percent:percent/custom, status:text
- `costs.theoretical_recipes` (orden por omisión `recipe`): recipe:text, kind:text, active:text, version:integer/sum, yield:quantity, batch_cost:money_mxn/sum, unit_cost:quantity, status:text
- `costs.consumption_valued` (orden por omisión `value`): category:text, code:code, item:text, unit:text, quantity:quantity, value:money_mxn/sum, share:percent/custom
- `costs.waste` (orden por omisión `value`): origin:text, classification:text, records:integer/sum, value:money_mxn/sum
- `costs.cost_adjustments` (orden por omisión `occurred_at`): occurred_at:datetime, code:code, item:text, source:text, related:text, value:money_mxn/sum, reason:text
- `employees.activity` (orden por omisión `user`): user:text, confirmations:integer/sum, confirmed_units:integer/sum, cancelled_units:integer/sum, accounts:integer/sum, sales:money_mxn/sum, payments:integer/sum, collected:money_mxn/sum, tips:money_mxn/sum, discounts:integer/sum, discount_amount:money_mxn/sum, courtesies:integer/sum, courtesy_amount:money_mxn/sum, authorizations:integer/sum, refunds:integer/sum, refunded:money_mxn/sum, cash_operations:integer/sum
- `employees.responsible_sales` (orden por omisión `user`): user:text, product:text, quantity:integer/sum, net:money_mxn/sum, tax:money_mxn/sum
- `employees.confirmations` (orden por omisión `user`): user:text, product:text, units:integer/sum, cancelled:integer/sum, orders:integer/sum

### Filtros nuevos

- Claves: `warehouse_id` (fuente `warehouses`), `item_id` (fuente `inventory_items`, con `search` por nombre o
  código), `category_id` con fuente `inventory_categories` en los reportes de inventario y compras (en los de
  costos y empleados sigue siendo la categoría del catálogo, fuente `categories`), `supplier_id` (fuente
  `suppliers`; el proveedor de mostrador lleva `detail` «Compras de mostrador»), `modality`
  (`order` | `direct` | `counter`: orden de compra, recepción directa, compras de mostrador por
  `inventory_suppliers.kind='counter'`) y `currency` (`MXN` | `USD`). Reutilizadas con valores propios: `state`
  (estado del turno, de la orden, del ajuste, del conteo, del vínculo, del proveedor, del costo o situación de la
  existencia según el reporte), `kind` (tipo de operación de caja, de corte, de movimiento, de receta u origen de
  la merma), `mode`, `user_id`, `register_id`, `shift_id`, `product_id`. Cada reporte declara sus opciones en el
  catálogo (`filters[].options`).
- Puente (`desktop/src-tauri/src/api.rs`, lista blanca `reports_*`): se agregaron `warehouse_id`, `item_id`,
  `supplier_id`, `modality` y `currency` (con su prueba Rust).

### Avisos (`meta.warnings[].code`)

- `SHIFT_CLOSING`: turnos en arqueo excluidos (solo cuántos, nunca importes); en los reportes de caja y de
  empleados.
- `CASH_SCOPE`: el usuario no tiene `cash.shifts.read_all`, `cash.history.read` o `cash.shifts.read`; solo se
  incluyen los turnos que puede consultar en Caja (de su caja; cerrados solo con historial o si él los cerró).
- `CONSUMPTION_PENDING`: hay solicitudes de consumo pendientes o con error en el periodo (o actuales, en los
  reportes sin periodo); los datos pueden estar incompletos. En inventarios, costos y márgenes.

### Cifras resumidas (`summary[].key`) destacadas

- `cash.shifts`: `shifts`, `open_shifts`, `closing_shifts`, `closed_shifts`; con `cash.expected.read`,
  `over_mxn` y `short_mxn`.
- `inventory.kardex` con `item_id`: `opening`, `entries`, `outflows`, `closing` (cantidades del insumo y, con
  `warehouse_id`, del almacén).
- `inventory.valuation`: `total_value` (todos los insumos; solo exige `inventory.valuation.read`).
- `inventory.waste`: `not_prepared` y `pending_classification` (cancelaciones del periodo).
- `inventory.consumption_*`: `units` (unidades comerciales distintas con consumo, cada una una vez).
- `costs.margin_*`: `cost_adjustments` (ajustes de costo del periodo, concepto aparte) y `account_adjustments`
  (descuentos y cargos generales de cuenta sin IVA, que no pertenecen a un producto).
- `purchases.received` y `purchases.by_item`: `receipts`, `total`, `cash_linked` (pagado desde caja, ya incluido
  en el total) y `returned_value` (devoluciones a proveedor del periodo, aparte).

### Dashboard (`GET reports/dashboard`): secciones nuevas

Cada sección exige sus permisos (además de `reports.read`); sin ellos no se calcula ni se envía. Los
indicadores marcados «actual» solo tienen valor en el periodo consultado (`previous` en `null`).

| Sección (`id`)     | Permisos                                                                       | Indicadores (`key`)                                                                                            |
| ------------------ | ------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------- |
| `cash`             | `reports.cash.read`                                                            | `open_shifts` (actual), `closing_shifts` (actual), `openings`, `cash_net`, `income`, `withdrawals`, `expenses` |
| `cash_differences` | `reports.cash.read` + `cash.expected.read`                                     | `closings`, `over_mxn`, `short_mxn`, `difference_mxn`                                                          |
| `purchases`        | `reports.purchases.read`                                                       | `received`, `receipts`, `ordered`, `pending_orders` (actual), `returned`                                       |
| `inventory`        | `reports.inventory.read` + `inventory.movements.read`                          | `consumed_units`, `waste_records`, `pending_requests` (actual)                                                 |
| `inventory_costs`  | `reports.inventory.read` + `inventory.movements.read` + `inventory.costs.read` | `consumption_value`, `waste_value`, `cost_adjustments`                                                         |
| `margin`           | `reports.sales.read` + `inventory.costs.read`                                  | `cost_of_sales`, `margin`, `margin_percent`, `firm_share`, `uncosted_units`                                    |

`margin`, `margin_percent`, `firm_share` y `cost_of_sales` son `null` sin datos suficientes (ninguna unidad vendida
con costo en el periodo). Los turnos en arqueo se excluyen de `cash` para todos.

### Funciones exportadas para los reportes (solo lectura, sin locks)

- `payments.CashFlowTotals`, `payments.ExpectedCash(totals)` y las expresiones `payments.ExpectedPaymentsMXNSQL` /
  `ExpectedPaymentsUSDSQL` (las mismas que usa `readCashFlows`): el esperado de un turno abierto en los reportes es
  el cálculo único de `cash_expected.go`. `payments.CashOperationTypeLabel(kind, code)`.
- `inventory.TheoreticalCosts(ctx, tx)` (costo teórico vigente de todas las recetas con el motor de
  `recipe_costs.go`, sin permisos ni locks) y los nombres visibles `inventory.MovementKindLabel`, `CostBasisLabel`,
  `SourceKindLabel`, `AdjustmentReasonLabel`, `AdjustmentStateLabel`, `CountStateLabel`, `CountScopeLabel`,
  `PurchaseOrderStateLabel` y `inventory.MovementKinds`.

## Etapa 9A · exportación PDF y Excel y operación del puente (parte C1)

#### Exportación de reportes a PDF y Excel (parte C1)

`GET /api/v1/reports/export/{id}?format=pdf|xlsx&…`

- `{id}`: un reporte del registro cerrado (los mismos que `reports/run/{id}`; el dashboard no se exporta).
- Parámetros: los mismos de `reports/run/{id}` (periodo, `compare`, `sort`, `direction` y los filtros que declara el
  reporte; cualquier otro se rechaza), **sin** `page` ni `page_size` (400: el archivo incluye todos los renglones del
  conjunto filtrado), más `format` (obligatorio, `pdf` o `xlsx`) y `paper` (`letter` por omisión o `a4`; solo con
  `format=pdf`, si no 400).
- Permisos (validados en la transacción de autenticación; un 403 se audita como `access.denied`): `reports.read`, los
  del reporte y `reports.export.pdf` (PDF) o `reports.export.excel` (Excel). Las columnas y cifras con permiso propio
  (efectivo físico y esperado con `cash.expected.read`, costos con `inventory.costs.read`, valuación…) no existen en
  el archivo de quien no tiene ese permiso: es el mismo `Result` de la pantalla.
- Respuesta 200: binaria, `Content-Type` exacto (`application/pdf` o
  `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`), `Content-Disposition: attachment;
filename=<reporte>_<primer-día>[_<último-día>].<ext>` (ASCII, sin acentos, ≤ 100 caracteres; un reporte sin periodo
  lleva la fecha comercial de generación; p. ej. `ventas-por-dia_2026-11-01_2026-11-30.pdf`), `Content-Length`,
  `Cache-Control: no-store` y `X-Content-Type-Options: nosniff`. El archivo se genera en la misma petición: no se
  guarda en el servidor ni hay identificador de descarga.
- Errores (JSON `{"error":{code,message,request_id}}`): 400 `VALIDATION_ERROR` (formato, papel, paginación,
  parámetro no admitido, periodo inválido, orden no permitido), 403 `PERMISSION_DENIED`, 404 `NOT_FOUND`, 422
  `EXPORT_TOO_LARGE` (más de **50 000** renglones: «El reporte tiene 50,001 renglones y la exportación admite hasta
  50,000. Reduce el periodo o agrega filtros.», sin generar nada), 422 `REPORT_TOO_LARGE` (más de 100 000 renglones,
  como la pantalla), 422 `EXPORT_FILE_TOO_LARGE` (archivo > 32 MiB), 503 `REPORTS_BUSY`, 503 `REPORT_TIMEOUT`.
- Ejecución: la consulta corre en la transacción `REPEATABLE READ READ ONLY` del ejecutor de reportes (misma
  fotografía para encabezado, detalle y totales; el nombre del establecimiento se lee en ella) y la exportación
  conserva su lugar del semáforo (2) mientras genera el archivo. Tiempo de respuesta propio de 110 s (la ruta amplía su
  plazo de escritura sobre el `WriteTimeout` de 15 s del servidor; `reports/*` también, a 45 s).
- Auditoría: `report.exported` (entidad `report`, sin `entity_id`) en una transacción corta aparte, después de generar
  el archivo y antes de enviarlo (si no se puede auditar, no se entrega): `{report, format, paper?, orientation?,
period: {kind, first_day, last_day}, comparison, filters: {clave: valor tal como se envió}, sort: {key, direction},
rows, bytes}`. Sin nombres de filtros, importes ni contenido. Un 422/400 no se audita como exportación.

Contenido del PDF: encabezado con establecimiento, reporte, descripción, periodo (etiqueta de Go), comparación,
filtros aplicados, fecha usada, hora de inicio del día y zona, orden, renglones y fecha de generación (hora del
restaurante); cifras resumidas y avisos; tabla con encabezados repetidos en cada página, renglones alternados, totales
de Go y notas; pie «AD-EL POS · Generado el …» y «Página n de m». Carta (o A4) vertical si cabe con 8 pt; si no,
horizontal con letra de 8 a 5.5 pt. Importes con `moneytext` (`$1,234.56`, `US$20.05`), porcentajes `12.50 %`,
fechas `dd/mm/aaaa` e instantes `dd/mm/aaaa hh:mm` en la zona del restaurante. Las columnas de importes, cantidades,
fechas y códigos nunca se parten; solo el texto se ajusta en varias líneas.

Contenido del XLSX: hoja «Datos» (encabezado con estilo propio, congelado en A2, una columna por columna visible del
reporte, anchos según el contenido; renglón final «Total» con los totales de Go como números) y hoja «Filtros»
(Concepto | Valor: establecimiento, reporte, descripción, periodo con primer y último día como fechas, comparación,
fecha usada, hora de inicio, zona, orden, renglones, generado, filtros aplicados, resumen, avisos, notas y, si
aplica, «Precisión»). Importes, cantidades y porcentajes (como fracción con formato `0.00%`) son números exactos
(ida y vuelta comprobada, ≤ 15 dígitos significativos; si no, texto alineado a la derecha con nota en «Filtros»);
pesos `"$"#,##0.00` y dólares `"US$"#,##0.00` en columnas separadas; códigos y textos con formato `@`; fechas
comerciales y horas del restaurante como fechas de Excel. Ningún valor es fórmula; un texto que empieza por `=`, `+`,
`-`, `@`, tabulador, retorno o salto (o `＝ ＋ － ＠`) lleva un apóstrofo inicial visible. Sin hipervínculos, nombres
definidos, vínculos externos, macros ni imágenes.

#### Puente nativo: operación cerrada `export_report` (`desktop/src-tauri/src/export.rs`)

`invoke('export_report', { report, format, query })` → `HTTPResult { status, body }`:

- `report`: uno de `EXPORTABLE_REPORTS` (lista cerrada = registro de Go; la prueba Go
  `TestExportBridgeListsEveryReport` exige que coincidan exactamente). `format`: `pdf` | `xlsx`. `query`: claves de la
  lista blanca `reports_*` de `api.rs` (`allowed_query_keys("reports_run")`) excepto `page`, `page_size`, `search` y
  `format`, más `paper` (`letter`|`a4`, solo PDF); valores de ≤ 120 caracteres sin controles. Lo demás se rechaza con
  un error de texto antes de llamar a Go.
- Credenciales como `api_request` (dispositivo y sesión del llavero; un 401 `SESSION_*`/`DEVICE_*` las descarta igual),
  120 s de espera y tope de 32 MiB (2 MiB para un error JSON). Una exportación a la vez («Ya hay una exportación en
  curso.»).
- Go responde ≠ 200 → `{ status, body }` con el JSON de error de Go (sin tokens).
- 200 → valida `Content-Type` exacto y firma (`%PDF-` / `PK\x03\x04`) **antes** del diálogo; nombre de
  `Content-Disposition` saneado (`[A-Za-z0-9._ -]`, sin carpeta ni punto inicial, extensión forzada, ≤ 100 caracteres,
  «reporte.<ext>» si no hay). Diálogo nativo «Guardar como» (`tauri-plugin-dialog`, ventana principal como padre, filtro
  del formato); escritura atómica (temporal oculto en la misma carpeta, `sync_all`, renombrado; el temporal se borra si
  algo falla). Si Rust tiene que agregar la extensión y ese archivo ya existe, no lo sustituye sin preguntar (error).
- `body` en 200: `{ "result": "saved", "file_name": "ventas-por-dia_2026-11-10.pdf", "size": 12345 }` o
  `{ "result": "cancelled" }`. Nunca bytes, ruta completa ni tokens.
- Registro: `build.rs`, `permissions/diagnostics.toml` (`allow-diagnostics` incluye `export_report`), `main.rs`
  (`.plugin(tauri_plugin_dialog::init())` e `invoke_handler`). `capabilities/main.json` sin cambios.

#### Frontend: `desktop/src/services/reportExport.ts`

```ts
type ExportFormat = 'pdf' | 'xlsx';
type ExportPaper = 'letter' | 'a4';
interface ExportOutcome { result: 'saved' | 'cancelled'; file_name?: string; size?: number }
exportReport(report: string, format: ExportFormat, query?: ReportQuery, options?: { paper?: ExportPaper }): Promise<ExportOutcome>
exportQuery(query, format, paper?) // la consulta de la pantalla sin page/page_size ni valores vacíos (+ paper en PDF)
```

Recibe la misma `ReportQuery` de la pantalla (quita la paginación). Errores: `APIError` con el código y mensaje de Go
(y `session-invalid` en 401 `SESSION_*`/`DEVICE_*`, como `api()`); un rechazo local del puente es
`APIError('EXPORT_FAILED', <mensaje de Rust>)`.

#### Playwright: `desktop/e2e/native-driver.ts`

`installDriver(page)` devuelve `{ request, nextSaveDialog(choice: 'save' | 'cancel'), exports }`. La rama
`export_report` lee la lista cerrada de `export.rs` y la lista blanca de `api.rs`, pide el archivo a Go desde Node,
valida tipo, firma y nombre como Rust y aplica la siguiente respuesta del diálogo de la cola (sin respuesta: guardar).
Los archivos guardados quedan en `exports` (`{report, format, fileName, contentType, bytes: Buffer}`) en Node; el
navegador recibe solo el resultado. Los rechazos del puente llegan al WebView como texto, igual que con Tauri.
