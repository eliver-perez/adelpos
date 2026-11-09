# API v1 · Etapa 5

Los contratos anteriores se mantienen. El detalle operativo nuevo está al final de este documento y en [RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md).

HTTP JSON bajo `/api/v1`. Por defecto `http://127.0.0.1:8181`; LAN requiere HTTPS con identidad de servidor verificada.

## Autenticación de transporte

El puente nativo añade `X-Device-ID`, `X-Device-Token` y `Authorization: Bearer <sesión>`. Los tokens duraderos no se devuelven a React. Todos los endpoints protegidos comprueban dispositivo, sesión, usuario activo y permiso efectivo en PostgreSQL. Cambiar un identificador público no sustituye una credencial válida. Los ejemplos documentan nombres de cabeceras, nunca valores reales.

## Rutas

| Método     | Ruta                      | Requisito                                                                                             |
| ---------- | ------------------------- | ----------------------------------------------------------------------------------------------------- |
| GET        | `/health/live`            | Público; proceso activo.                                                                              |
| GET        | `/health/ready`           | Público; PostgreSQL y esquema compatibles, 200 o 503.                                                 |
| GET        | `/version`                | Público; producto, versión, API y esquema 5.                                                          |
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

Setup/settings: `name`, `timezone` IANA (Go exige una zona existente con `time.LoadLocation` y rechaza `""` y `Local`), `currency` MXN, `phone`, `email`, `address`, `theme` light/dark. Setup añade `terminal_name`. La zona propuesta procede del OS; Go valida y conserva la elección. Setup repetido con autorización local responde configured/already_configured y no vuelve a emitir credenciales ni modifica datos.

Usuarios: `username`, `display_name`, `role_id`, `active`; POST añade password de 8 a 256 caracteres. PATCH no restablece contraseñas. Rol: `name`, `permissions` (claves del registro). Excepciones: `{ "overrides": { "users.read": "allow", "registers.read": "deny", "audit.read": "inherit" } }`. Heredar elimina la excepción; las demás claves enviadas se actualizan.

Cajas: `name`, `description`, `active`. Dispositivo: `name`, `register_id` UUID o null, `keyboard_mode` auto/always/disabled. Enrolamiento: `code`, `name`. Preferencias: `theme` light/dark y `home_screen`; se normaliza a status si no está implementada o permitida. Desde la etapa 6.1 `preview` ya no es una pantalla: un valor guardado o enviado como `preview` se interpreta como `catalog` (la vista previa vive en Catálogo). Contraseña propia: `current_password`, `new_password`.

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

400: validación; 401: dispositivo/sesión/credenciales inválidos; 403: permiso o aprovisionamiento rechazado (la operación completa se revierte y solo se audita la denegación); 404: inexistente; 409: duplicado o caja en uso; 413: cuerpo mayor del límite (64 KiB general; 2 MiB catálogo/restaurante); 429: límite de intentos; 503: indisponibilidad. Los diagnósticos conservan los códigos de la etapa 1. Códigos de sesión: SESSION_REQUIRED, SESSION_INVALID, SESSION_EXPIRED, SESSION_REVOKED, SESSION_REPLACED; terminal: DEVICE_REQUIRED y DEVICE_REVOKED. No exponer detalles PostgreSQL.

JSON UTF-8, Cache-Control no-store, X-Content-Type-Options nosniff y request ID generado por Go. CORS exacto conserva consultas técnicas GET; la operación autenticada de escritorio usa HTTP nativo, sin ampliar CORS ni CSP. Ninguna credencial se transporta por query string. Timeouts administrativos de diez segundos en Go y doce en el cliente; respuesta nativa limitada a 2 MiB.

## Versionado y proyectos futuros

API v1 admite campos adicionales compatibles; cambios incompatibles requieren nueva versión. El diagnóstico verifica identidad/API/esquema. El servidor es único dueño del esquema; ningún cliente migra PostgreSQL.

La etapa 4 implementa contratos de pedidos e idempotencia durable con UUID, hash, resultado y evento en la misma transacción. La etapa 5 añade outbox, cursor/replay, SSE y contratos KDS autorizados de consulta/acuse/estado. Android, offline durable y la aplicación KDS independiente siguen pendientes. No se implementó WebSocket: SSE cubre la notificación unidireccional y los comandos usan HTTP idempotente. El punto comercial de licencias sigue centralizado, sin protocolo definitivo.

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

Orden visual por posición (etapa 6.1): los POST/PATCH de categorías y productos aceptan el campo opcional de entrada `position:{after:""|UUID}`; `""` coloca el registro al inicio y un UUID inmediatamente después de ese hermano (categorías: todas; productos y combos: los de la misma categoría con la que se guarda el producto). Con `position`, el `sort_order` del cuerpo se ignora y Go renumera la lista en múltiplos de 10 dentro de la misma transacción (hermanos bloqueados con `FOR UPDATE`, revisión incrementada solo en los hermanos cuyo número cambió, auditoría `catalog.categories.reordered`/`catalog.products.reordered`); un registro que se agrega al final recibe el mayor número + 10 sin tocar a los demás y guardar sin mover no renumera. Sin `position` se conserva el comportamiento anterior. `after` ausente, inexistente, de otra lista (otra categoría) o igual al propio registro responde 400. `position` no se devuelve en las lecturas. El listado de productos se ordena por el orden de su categoría y después por el del producto.

Color de categoría (etapa 6.1): `""` o un `hex` de la paleta cerrada de Go (16 colores, se normaliza a minúsculas). Al editar se acepta también el color ya guardado aunque no esté en la paleta (colores anteriores a la paleta); cualquier otro valor responde 400. `GET /lookups` añade `palette:[{hex,name}]` y, en cada producto, `category_id` y `sort_order`.

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
| GET         | `/accounts/{id}/history`       | `orders.history.read`                                         |

Guardar/cotizar borrador, confirmar y editar exige además `catalog.read` y `catalog.prices.read`. Seleccionar otra lista requiere `orders.price_list.manage`. El PATCH de cuenta verifica también `accounts.move` o `accounts.transfer` si cambian esos campos. Un mesero autorizado no necesita otra credencial de supervisor para editar/cancelar.

Las escrituras llevan `operation_id` UUID. Actualizar exige `revision`; transferir/combinar exige también `target_revision`. GET y POST de previsualización no llevan UUID ni persisten cambios. Repetir exactamente una escritura del mismo actor/terminal devuelve el resultado original. `REVISION_CONFLICT`, `QUOTE_CHANGED` e `IDEMPOTENCY_CONFLICT` responden 409 y conservan el estado previo. No hay DELETE. Cuerpos hasta 2 MiB y respuestas nativas hasta 2 MiB.

### Documentos y consultas

- Área: `name`, `description`, `sort_order`, `active` y, desde la etapa 6.1, `position:{after}` opcional con la misma regla del catálogo (auditoría `restaurant.areas.reordered`); PATCH conserva `id` y revisión del documento leído.
- Mesa: `area_id`, `name`, `shape` (`rectangle`/`round`), `x`, `y`, `width`, `height`, `seats`, `active`, `out_of_service`. El lienzo lógico es 1000×560. `/layout` recibe `tables:[{id,revision,x,y,width,height,shape}]` y UUID del lote; todas las posiciones se validan antes de escribir.
- Cuenta: `name`, `mode` (`dine_in`/`takeaway`, inmutable), `table_id` opcional, `price_list_id` opcional, `customer_name`, `phone`, `origin`, `notes`, `scheduled_local` (`YYYY-MM-DDTHH:mm`), `schedule_kind` (`preparation`/`delivery` o vacío), `assigned_user_id` opcional. Para llevar rechaza mesa. La zona proviene del establecimiento; el cliente no decide el offset.
- Listado: `mode`, `table_id` (UUID o `unassigned`), `search`, `state` (`open` predeterminado, `merged`, `voided`, `all`), `page`, `page_size` hasta 100. Respuesta `{items,total,page,page_size,server_time,timezone,settings}` con orden ascendente de creación. `search` consulta folio, referencia, cliente y teléfono.
- Cuenta completa: `{account,draft,lines,server_time,timezone}`. `lines` agrupa unidades por versión y estado e incluye `quantity`, solicitud, desglose histórico y totales exactos de las unidades que conserva esa cuenta. Los UUID de unidad se conservan en la base y eventos.
- Historial: `page`, 100 eventos por página, orden descendente; incluye eventos donde la cuenta es origen o relacionada. Autor, terminal, instante, tipo, valores anterior/posterior y estado de integración son persistidos.
- Configuración: `warning_minutes`, `overdue_minutes`, `revision`, `operation_id`; advertencia 1–240 minutos, atraso entre advertencia y 1440.

### Captura y confirmación

Guardar o cotizar una captura envía `revision` e `items`. Cada ítem lleva UUID estable de borrador, `product_id`, `variant_id` nullable, `quantity` entera y `customization` del catálogo. PUT de borrador añade el UUID de operación; POST `draft-preview` no lo requiere. Límite de lote: 20 partidas/1000 unidades totales.

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

## Cobros, pagos y devoluciones · esquema 6

Prefijo `/api/v1/payments`. Todas las rutas requieren sesión y terminal vigentes; las rutas de cuenta exigen además `accounts.read` y, para pedidos para llevar, `takeaway.read`. Las escrituras llevan `operation_id` UUID y son idempotentes por actor, terminal, ruta y cuerpo; la huella excluye `authorization.password`. El dinero viaja como cadena decimal con hasta dos decimales. Las previsualizaciones (`preview`) solo calculan. Reglas completas en [PAYMENTS_AND_REFUNDS.md](docs/PAYMENTS_AND_REFUNDS.md).

| Método     | Ruta bajo `/payments`              | Permiso                                                                                                                                                                                        |
| ---------- | ---------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET / PUT  | `/settings`                        | `payments.read` / `payments.settings.manage`                                                                                                                                                   |
| PUT        | `/tax`                             | `payments.tax.manage`; `tax_rate` exactamente `"16"`, `"8"` o `"0"`                                                                                                                            |
| GET / POST | `/methods`                         | `payments.read` / `payments.methods.manage`                                                                                                                                                    |
| PATCH      | `/methods/{id}`                    | `payments.methods.manage`; conserva identidad y revisión                                                                                                                                       |
| GET        | `/shifts/current`                  | `payments.read`; caja de la terminal, turno abierto y si es utilizable                                                                                                                         |
| POST       | `/shifts`                          | `cash.shifts.open`; `{operation_id, opening_float}`                                                                                                                                            |
| GET        | `/shifts/{id}/movements`           | `cash.shifts.read`; movimientos, totales por método y efectivo esperado                                                                                                                        |
| GET        | `/accounts/{id}`                   | `payments.read`; estado financiero completo                                                                                                                                                    |
| POST       | `/accounts/{id}/preview`           | `payments.read`; plan de cobro                                                                                                                                                                 |
| POST       | `/accounts/{id}/payments`          | `payments.collect`; parcial `payments.collect_partial`; selección `payments.collect_selection`; propina `payments.tips.record`; `print` `payments.tickets.print`; `deliver` `takeaway.deliver` |
| POST       | `/accounts/{id}/discounts/preview` | `payments.read`                                                                                                                                                                                |
| POST       | `/accounts/{id}/discounts`         | `payments.discounts.apply` dentro del límite, `payments.courtesy` para cortesías, o autorización de supervisor                                                                                 |
| POST       | `/accounts/{id}/charges`           | `payments.charges.create`                                                                                                                                                                      |
| POST       | `/accounts/{id}/adjustments/void`  | Cargo: `payments.charges.manage`; descuento: `payments.discounts.apply`; o autorización                                                                                                        |
| POST       | `/accounts/{id}/refunds/preview`   | `payments.read`                                                                                                                                                                                |
| POST       | `/accounts/{id}/refunds`           | `payments.refunds.execute` (y bajo el umbral) o autorización con `payments.refunds.authorize`                                                                                                  |
| POST       | `/accounts/{id}/corrections`       | `payments.corrections` o autorización; solo con el turno original abierto                                                                                                                      |
| GET        | `/accounts/{id}/precheck`          | `payments.read`; no registra venta                                                                                                                                                             |
| POST       | `/accounts/{id}/precheck/print`    | `payments.tickets.print`                                                                                                                                                                       |
| POST       | `/accounts/{id}/documents/reprint` | `payments.tickets.reprint`; `{operation_id, ticket_id}`                                                                                                                                        |
| POST       | `/accounts/{id}/deliver`           | `takeaway.deliver`; solo pedidos para llevar liquidados                                                                                                                                        |

### Cuerpos principales

- Método de pago: `{operation_id, revision, name, icon, active, sort_order, is_cash, reference_mode, allows_tip, allows_refund}` y, desde la etapa 6.1, `position:{after}` opcional con la misma regla del catálogo (auditoría `payments.methods_reordered`).
- Pago: `{operation_id, method_id, mode: "total"|"partial", amount?, received?, tip?, reference?, selections?: [{version_id, quantity}], expected_balance, print?, deliver?}`. Respuesta `{payment, finance, closed, delivered, printing}`.
- Descuento: `{operation_id, scope: "account"|"items", method: "percent"|"amount", value, courtesy, concept?, reason, selections?, expected_balance, authorization?: {username, password}}`. Cargo: `{operation_id, concept, amount, reason?, expected_balance}`. Cancelación: `{operation_id, adjustment_id, reason, authorization?}`.
- Devolución: `{operation_id, mode: "total"|"payment"|"items", payment_id?, amount?, tip?, selections?, method_id, reference?, reason, authorization?}`. Corrección: `{operation_id, payment_id, method_id, reference?, reason, authorization?}`.
- `finance`: totales (`items_total`, `item_discounts`, `account_discounts`, `charges`, `total`, `paid`, `refunded`, `tips`, `balance`), `payment_state`, `delivery_state`, `settled`, `tax[{rate, base, tax, total}]`, `lines[]` por versión con unidades y su estado (`unpaid`, `partial`, `settled`), `payments[]`, `refunds[]`, `adjustments[]`, `documents[]` y `delivery`.

Las cuentas del restaurante incluyen `finance: {total, paid, balance, tips, payment_state, delivery_state}`; `payment_state` es `not_recorded`, `partial` o `paid` y `state` admite `closed`. El listado acepta `state=closed`.

Errores específicos: `BALANCE_CHANGED`, `NOTHING_TO_COLLECT`, `SELECTION_EXCEEDS_BALANCE`, `REGISTER_REQUIRED`, `SHIFT_REQUIRED`, `SHIFT_NOT_SHARED`, `SHIFT_ALREADY_OPEN`, `SHIFT_CLOSED`, `ACCOUNT_NOT_OPEN`, `UNIT_HAS_PAYMENT`, `UNIT_HAS_DISCOUNT`, `ACCOUNT_BELOW_PAYMENTS`, `ACCOUNT_HAS_PAYMENTS`, `NOTHING_TO_REFUND`, `PAYMENT_NOT_CORRECTABLE`, `UNIT_NOT_DISCOUNTABLE`, `ADJUSTMENT_VOIDED`, `ALREADY_DELIVERED`, `BALANCE_PENDING`, `PRINTER_NOT_CONFIGURED`, `PRINT_COPY_PENDING` (409); `PAYMENT_VALIDATION` (400); `AUTHORIZATION_REQUIRED`, `AUTHORIZATION_INSUFFICIENT`, `SELF_AUTHORIZATION` (403); `AUTHORIZATION_INVALID` (401, no invalida la sesión); `AUTHORIZATION_THROTTLED` (429). Los errores de guardas financieras dentro de operaciones del restaurante (`restaurant/*`) y los de cocina conservan su código público en lugar de un 503.

Roles: `POST/PATCH /roles` aceptan `discount_limit` opcional (porcentaje `0`–`100`, cadena vacía para eliminarlo); nadie asigna un límite mayor que el suyo.
