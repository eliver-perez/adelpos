# API v1 · Etapa 9D

Los contratos anteriores se mantienen. El detalle operativo nuevo está al final de este documento y en [RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md).

Etapa 8 (inventarios, recetas, almacenes, compras y costos; guía en [INVENTORY_AND_PURCHASING.md](docs/INVENTORY_AND_PURCHASING.md)): sus contratos están al final, en el orden de sus migraciones: «Inventario · fase A: núcleo (esquema 10)», «Inventario · fase B: recetas y consumo (esquema 11)», «Catálogo · indicaciones rápidas con efecto (esquema 12)», «Inventario · fase C: compras (esquema 13)» e «Inventario · fase D: control de inventario (esquema 14)». Las reglas comunes de inventario (permisos, lock, `operation_id`, cantidades y visibilidad de costos) se describen en la fase A y rigen las demás. Las correcciones de la revisión independiente (fase F del avance: valuación por insumo, elaboraciones y reversiones con existencia negativa, `COST_REQUIRED` y campos ocultos sin `inventory.stock.read`) están integradas en la sección de cada ruta.

Etapa 9A (ajustes pendientes de catálogo e inventarios, reportes, indicadores y exportaciones; guía en [REPORTS.md](docs/REPORTS.md)): sus contratos están al final, en el orden de sus partes: «Etapa 9A · precio de complementos y vigencia de la precuenta (parte A1, esquema 15)», «Etapa 9A · proveedores, compras de mostrador, código sugerido, categorías y navegación de Inventarios (parte A2, esquema 16)», «Etapa 9A · reportes, día comercial y reconocimiento de ventas (parte B1, esquema 17)», «Etapa 9A · reportes de caja, inventarios, costos y márgenes, compras, proveedores y empleados (parte B2, sin migración nueva)», «Etapa 9A · exportación PDF y Excel y operación del puente (parte C1, sin migración nueva)» y «Etapa 9A · interfaz de Reportes (parte C2, sin cambios de contrato)». Las reglas comunes de los reportes (ejecución en una transacción de solo lectura, resultado único, permisos, formato de valores y errores) se describen en la parte B1 y rigen las partes B2, C1 y C2.

Etapa 9B (identidad del establecimiento y diseñador visual de documentos; guía en [DOCUMENT_DESIGNER.md](docs/DOCUMENT_DESIGNER.md)): sus contratos están al final: «Etapa 9B · plantillas de documentos, vista previa, muestras e impresión con la plantilla vigente (fase A, esquema 18)», «Etapa 9B · identidad del establecimiento y logotipo (fase B, esquema 19)» y «Etapa 9B · editor visual (fase C, sin cambios de contrato)». Las correcciones de la revisión independiente de la fase D están integradas en la sección de cada ruta. Las reglas comunes del diseñador (`operation_id`, revisión, errores y validación del layout) se describen en la fase A y rigen la B.

Etapa 9C (nueva interfaz principal, navegación, teclado y ajustes; guía en [NAVIGATION_AND_SHORTCUTS.md](docs/NAVIGATION_AND_SHORTCUTS.md)): no cambia reglas de negocio, cálculos ni permisos. Sus contratos están al final, en «Etapa 9C · inicio por terminal, columnas por impresora, reportes nuevos y puente del menú (esquema 20)»: el campo `home_screen` de las terminales, las columnas y la prueba de ancho de las impresoras, la vista previa del diseñador por impresora, dos reportes y columnas nuevas, la validación de JPEG del catálogo, mensajes que cambiaron de texto y tres operaciones nuevas del puente (`kitchen_printer_width_test`, `menu_subscribe` y `menu_set_state`).

Etapa 9D (ajustes de interfaz, color de marca y cancelar cuenta; avance en [ETAPA_9D_AVANCE.md](docs/ETAPA_9D_AVANCE.md)): no cambia reglas de negocio, cálculos, la clasificación de la etapa 8 ni los flujos de cobro salvo lo que dice su §0-B. Sus contratos están al final, en «Etapa 9D · cancelar cuenta, descartar cuentas vacías, motivos, umbral de cuenta abierta y reportes (esquema 21)»: rutas para cancelar y descartar cuentas, el catálogo de motivos de cancelación y el umbral de cuenta abierta; campos nuevos de la cuenta, la mesa y la configuración; dos eventos de cuenta; «Cuentas canceladas» y «Cancelaciones por motivo»; siete operaciones nuevas del puente y la retirada de `restaurant_void`.

HTTP JSON bajo `/api/v1`. Por defecto `http://127.0.0.1:8181`; LAN requiere HTTPS con identidad de servidor verificada.

## Autenticación de transporte

El puente nativo añade `X-Device-ID`, `X-Device-Token` y `Authorization: Bearer <sesión>`. Los tokens duraderos no se devuelven a React. Todos los endpoints protegidos comprueban dispositivo, sesión, usuario activo y permiso efectivo en PostgreSQL. Cambiar un identificador público no sustituye una credencial válida. Los ejemplos documentan nombres de cabeceras, nunca valores reales.

## Rutas

| Método     | Ruta                      | Requisito                                                                                             |
| ---------- | ------------------------- | ----------------------------------------------------------------------------------------------------- |
| GET        | `/health/live`            | Público; proceso activo.                                                                              |
| GET        | `/health/ready`           | Público; PostgreSQL y esquema compatibles, 200 o 503.                                                 |
| GET        | `/version`                | Público; producto, versión, API y versión del esquema (21 en la 0.9.3).                               |
| GET        | `/events`                 | SSE; sesión/dispositivo, events.read y ámbitos vigentes.                                              |
| GET        | `/setup/status`           | Público; solo configured booleano.                                                                    |
| POST       | `/setup`                  | Loopback y X-Provisioning-Token del propietario local.                                                |
| POST       | `/devices/enroll`         | Código de autorización de un solo uso, no requiere sesión previa.                                     |
| POST       | `/auth/login`             | Dispositivo autorizado; username y password.                                                          |
| POST       | `/auth/logout`            | Sesión; revoca la actual.                                                                             |
| GET        | `/auth/me`                | Sesión; identidad, permisos, preferencias y terminal (con `home_screen` desde la 9C).                 |
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
| PATCH      | `/devices/{id}`           | devices.manage; nombre, caja, teclado y pantalla inicial (9C).                                        |
| POST       | `/devices/{id}/revoke`    | devices.manage; revoca terminal y sesiones.                                                           |
| GET        | `/sessions`               | sessions.read; hasta 200 recientes, sin tokens.                                                       |
| POST       | `/sessions/{id}/revoke`   | sessions.manage; una cuenta ordinaria no revoca directamente sesiones de sa.                          |
| GET / PUT  | `/preferences`            | Sesión propia.                                                                                        |
| PUT        | `/terminal/preferences`   | Sesión; solo teclado de la terminal actual.                                                           |
| GET        | `/audit`                  | audit.read; filtros y paginación.                                                                     |

Desde la etapa 9D (esquema 21): `GET / PUT /settings/long-open-hours` (umbral de cuenta abierta por mucho tiempo; GET con
cualquier sesión autenticada y PUT con settings.features.manage; contrato en «Etapa 9D»).

Delegación: un 403 `PERMISSION_DENIED` con mensaje específico rechaza crear, asignar o editar roles y excepciones cuyo resultado tenga permisos efectivos que el actor no posee, administrar a un usuario con más permisos, y modificar el propio rol o las propias excepciones. Retirar un `deny` (`inherit`) cuenta como concesión. Un cambio que dejaría al sistema sin un administrador completo (usuario ordinario activo con todos los permisos registrados) responde 400 `VALIDATION_ERROR` y no modifica datos.

## Cuerpos principales

Funciones del establecimiento (etapa 6.1): `GET /settings/features` devuelve `[{code, name, description, enabled, revision, updated_at, updated_by_name}]` con todas las funciones conocidas por Go (hoy solo `usd_cash`, desactivada por omisión). `PUT /settings/features` recibe `{code, enabled, revision}` sin `operation_id` (configuración con revisión optimista, como `/settings`); el código viaja en el cuerpo porque el puente nativo solo admite identificadores UUID en la ruta. Responde la función guardada; una revisión obsoleta da 409 `REVISION_CONFLICT`, un código desconocido 404 `FEATURE_NOT_FOUND`. Se audita como `settings.feature_changed` y publica `settings.features.changed` (ámbito `configuration`). Desactivar una función nunca borra ni altera registros. Desde la etapa 8 (esquema 10) cada función incluye `enabled_at`: la fecha y hora en que pasó de desactivada a activada por última vez (null si nunca se activó después de la 0010; desactivarla la conserva). La función `inventory_control` («Control de inventario», desactivada por omisión) publica además `inventory.settings.changed` en el ámbito `inventory`.

Setup/settings: `name`, `timezone` IANA (Go exige una zona existente con `time.LoadLocation` y rechaza `""` y `Local`), `currency` MXN, `phone`, `email`, `address`, `theme` light/dark. Setup añade `terminal_name`. La zona propuesta procede del OS; Go valida y conserva la elección. Setup repetido con autorización local responde configured/already_configured y no vuelve a emitir credenciales ni modifica datos.

Usuarios: `username`, `display_name`, `role_id`, `active`; POST añade password de 8 a 256 caracteres. PATCH no restablece contraseñas. Rol: `name`, `permissions` (claves del registro). Excepciones: `{ "overrides": { "users.read": "allow", "registers.read": "deny", "audit.read": "inherit" } }`. Heredar elimina la excepción; las demás claves enviadas se actualizan.

Cajas: `name`, `description`, `active`. Dispositivo: `name`, `register_id` UUID o null, `keyboard_mode` auto/always/disabled y, desde la etapa 9C, `home_screen` opcional (ver «Etapa 9C»). Enrolamiento: `code`, `name`. Preferencias: `theme` light/dark y `home_screen`; se normaliza a status si no está implementada o permitida. Desde la etapa 6.1 `preview` ya no es una pantalla: un valor guardado o enviado como `preview` se interpreta como `catalog` (la vista previa vive en Catálogo). Desde la etapa 7 (fase B) `cash` («Caja») es una pantalla de inicio válida con `cash.shifts.read`. Desde la etapa 8 (fase A) `inventory` («Inventarios») también lo es, con `inventory.read`, y desde la etapa 9A (parte B1) `reports` («Reportes»), con `reports.read`. Desde la etapa 9C la aplicación **ya no usa** `home_screen` de las preferencias para abrir (lo decide la terminal, `devices.home_screen`): Go lo sigue leyendo, normalizando y guardando, y la interfaz reenvía el valor guardado sin cambiarlo. Contraseña propia: `current_password`, `new_password`.

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

JSON UTF-8, Cache-Control no-store, X-Content-Type-Options nosniff y request ID generado por Go. CORS exacto conserva consultas técnicas GET; la operación autenticada de escritorio usa HTTP nativo, sin ampliar CORS ni CSP. Ninguna credencial se transporta por query string. Timeouts administrativos de diez segundos en Go y doce en el cliente; respuesta nativa limitada a 2 MiB. Desde la etapa 9A, `reports/*` tiene 40 s en Go y 45 s en el puente, y `reports/export/*` 110 s en Go y 120 s en la operación `export_report`, que admite archivos de hasta 32 MiB (ver las secciones «Etapa 9A»).

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

| Método      | Ruta bajo `/restaurant`        | Permiso específico                                                                  |
| ----------- | ------------------------------ | ----------------------------------------------------------------------------------- |
| GET         | `/overview`                    | `restaurant.tables.read` y `restaurant.areas.read`                                  |
| GET / POST  | `/areas`                       | `restaurant.areas.read` / `restaurant.areas.manage`                                 |
| PATCH       | `/areas/{id}`                  | `restaurant.areas.manage`                                                           |
| GET / POST  | `/tables`                      | `restaurant.tables.read` / `restaurant.tables.manage`                               |
| PATCH       | `/tables/{id}`                 | `restaurant.tables.manage`                                                          |
| POST        | `/tables/{id}/service`         | `restaurant.tables.manage` (etapa 7.1)                                              |
| PUT         | `/layout`                      | `restaurant.tables.manage`                                                          |
| GET / PUT   | `/settings`                    | `accounts.read` / `restaurant.settings.manage`                                      |
| GET         | `/staff`                       | `accounts.transfer`                                                                 |
| GET / POST  | `/accounts`                    | `accounts.read` / `accounts.open`                                                   |
| GET / PATCH | `/accounts/{id}`               | `accounts.read` / `accounts.manage`                                                 |
| PUT         | `/accounts/{id}/draft`         | `orders.capture`                                                                    |
| POST        | `/accounts/{id}/draft-preview` | `orders.capture`; solo calcula                                                      |
| GET         | `/accounts/{id}/preview`       | `accounts.read`; calcula borrador guardado                                          |
| POST        | `/accounts/{id}/confirm`       | `orders.confirm`                                                                    |
| POST        | `/accounts/{id}/edit-preview`  | `orders.edit`; solo calcula                                                         |
| POST        | `/accounts/{id}/edit`          | `orders.edit`; reducir unidades exige `orders.cancel` o, desde la 9F, un supervisor |
| POST        | `/accounts/{id}/cancel`        | `orders.cancel` o, desde la 9F, `accounts.open` con un supervisor                   |
| POST        | `/accounts/{id}/split`         | `accounts.split` y `accounts.open`                                                  |
| POST        | `/accounts/{id}/transfer`      | `accounts.transfer_items`                                                           |
| POST        | `/accounts/{id}/merge`         | `accounts.merge`                                                                    |
| POST        | `/accounts/{id}/move`          | `accounts.move`                                                                     |
| POST        | `/accounts/{id}/assign`        | `accounts.transfer`                                                                 |
| POST        | `/accounts/{id}/void`          | retirada en la etapa 9F: `410 ROUTE_RETIRED`                                        |
| POST        | `/accounts/{id}/release`       | `accounts.open` (etapa 7.1)                                                         |
| GET         | `/accounts/{id}/history`       | `orders.history.read`                                                               |

Rutas de la etapa 9D (esquema 21; contratos en «Etapa 9D»):

| Método     | Ruta bajo `/restaurant`         | Permiso específico                                                          |
| ---------- | ------------------------------- | --------------------------------------------------------------------------- |
| GET / POST | `/cancel-reasons`               | `accounts.read` / `payments.settings.manage`                                |
| PATCH      | `/cancel-reasons/{id}`          | `payments.settings.manage`                                                  |
| POST       | `/accounts/{id}/discard`        | `accounts.open`                                                             |
| POST       | `/accounts/{id}/cancel-account` | `accounts.open`; `accounts.cancel` o supervisor, decidido dentro de la ruta |

Desde la 9D el puente ya no tiene la operación de `POST /accounts/{id}/void`, y desde la 9F Go la retiró: responde
`410 ROUTE_RETIRED` sin escribir nada (ver «Etapa 9F · ruta antigua de anulación retirada»).

Guardar/cotizar borrador, confirmar y editar exige además `catalog.read` y `catalog.prices.read`. Seleccionar otra lista requiere `orders.price_list.manage`. El PATCH de cuenta verifica también `accounts.move` o `accounts.transfer` si cambian esos campos. Un mesero con `orders.edit` y `orders.cancel` no necesita otra credencial de supervisor para editar/cancelar; sin `orders.cancel`, desde la etapa 9F cancelar o reducir unidades pide la autorización puntual de un supervisor.

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

Mover recibe `{operation_id,revision,table_id}` con null para retirar mesa. Asignar recibe `{operation_id,revision,assigned_user_id}`. Anular cuenta vacía recibía `{operation_id,revision,reason}`: desde la etapa 9D la aplicación usa «Descartar» y «Cancelar cuenta» (el puente ya no tiene `restaurant_void`) y desde la 9F la ruta responde `410 ROUTE_RETIRED`. Las respuestas son cuentas completas y todas las acciones se auditan transaccionalmente.

No existen rutas de cobro o entrega. `payment_state:not_recorded` y `delivery_state:not_delivered` siguen independientes de preparación. La etapa 5 integra liberaciones, documentos, impresión y eventos conforme a los contratos siguientes.

## Cocina e impresión · etapa 5

Prefijo `/api/v1/kitchen`. Todos los UUID se validan; las escrituras conservan `operation_id` y recuperan el resultado solo para el mismo actor, terminal, ruta y cuerpo. PATCH/PUT y acciones sobre entidades existentes usan revisión. Conflictos responden 409; no se regeneran UUID después de una respuesta incierta. No hay DELETE ni URL o comando arbitrario desde React.

| Método     | Sufijo                                  | Permiso                                                                                                                    |
| ---------- | --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| GET / POST | `/stations`                             | `kitchen.stations.read` / `kitchen.stations.manage`                                                                        |
| PATCH      | `/stations/{id}`                        | `kitchen.stations.manage`; tiempos requieren también `kitchen.timing.manage`                                               |
| GET        | `/outputs`                              | `kitchen.stations.manage`; destinos sin configuración técnica completa                                                     |
| GET / PUT  | `/access`                               | `kitchen.stations.manage`                                                                                                  |
| GET / POST | `/printers`                             | `kitchen.printers.manage`                                                                                                  |
| PATCH      | `/printers/{id}`                        | `kitchen.printers.manage`                                                                                                  |
| GET        | `/discovery`                            | `kitchen.printers.manage`; consulta CUPS real del servidor (en Windows, desde la 9E, las impresoras instaladas en Windows) |
| GET        | `/targets`                              | `kitchen.routing.manage`; categorías/productos sin precios                                                                 |
| GET / POST | `/routes`                               | `kitchen.routing.manage`                                                                                                   |
| PATCH      | `/routes/{id}`                          | `kitchen.routing.manage`                                                                                                   |
| GET        | `/settings`                             | `kitchen.stations.read`, `kitchen.formats.manage` o `kitchen.timing.manage`                                                |
| PUT        | `/settings`                             | `kitchen.formats.manage`; cambios de tiempos/seguimiento exigen `kitchen.timing.manage`                                    |
| PUT        | `/timing`                               | `kitchen.timing.manage`                                                                                                    |
| GET        | `/board`, `/releases`, `/releases/{id}` | `kitchen.states.read` y alcance de estación                                                                                |
| POST       | `/releases/{id}/release`                | `kitchen.release`; adelantada requiere `kitchen.release_early`                                                             |
| POST       | `/releases/{id}/schedule`               | `kitchen.timing.manage`                                                                                                    |
| POST       | `/changes/{id}/retry`                   | `kitchen.release`                                                                                                          |
| POST       | `/states`                               | `kitchen.states.manage` y alcance por cada estación afectada                                                               |
| GET        | `/tickets`, `/tickets/{id}`             | `kitchen.tickets.read` y alcance de estación                                                                               |
| POST       | `/tickets/{id}/ack`                     | `kitchen.kds.ack` y alcance de estación                                                                                    |
| GET        | `/jobs`, `/jobs/{id}`                   | `kitchen.queue.read` y `kitchen.stations.all`                                                                              |
| POST       | `/jobs/{id}/retry`                      | `kitchen.queue.retry` y `kitchen.stations.all`                                                                             |
| POST       | `/jobs/{id}/resolve`                    | `kitchen.queue.resolve` y `kitchen.stations.all`                                                                           |
| POST       | `/reprint`                              | `kitchen.reprint` y `kitchen.stations.all`                                                                                 |
| POST       | `/test-print`                           | `kitchen.printers.test`                                                                                                    |
| GET        | `/history`                              | `kitchen.history.read` y `kitchen.stations.all`                                                                            |

El alcance global `kitchen.stations.all` se comprueba además del permiso funcional. Las consultas y acciones sobre liberaciones/cambios pendientes completos también lo exigen. Sin él, las tareas/documentos operativos y eventos se filtran por las estaciones visibles (desde la etapa 9F, las de la terminal intersectadas con las del usuario; ver «Etapa 9F · cocina por estaciones»); no basta conocer UUID. El listado de configuración de estaciones conserva su permiso específico. Cambiar un destino durante una resolución o reimpresión exige también `kitchen.printers.manage`. Los veinte permisos nuevos solo se asignan inicialmente al administrador.

### Configuración

- Estación: `{name,description,active,sort_order,reception,printer_id,target_minutes,warning_minutes,overdue_minutes,grouping,tracking}` y `position:{after}` opcional (etapa 6.1, misma regla del catálogo; historial `station.reordered`). Recepción `printer|kds|both`; agrupación `inherit|independent|unified`; seguimiento `inherit|item|order`. PATCH lleva `id` y `revision`.
- Impresora: `{name,transport,destination,port,paper_width,copies,supports_cut,cut,cutter_option,active,fallback_id}`. Transporte `cups|escpos_tcp|agent`; papel 58/80 mm, copias 1–3, puerto 9100–9109. CUPS utiliza nombre de cola (desde la etapa 9E, con el servidor en Windows, `cups` significa «cola de impresión del equipo» y el destino es el nombre de la impresora instalada en Windows; ver «Etapa 9E»); TCP una IP privada explícita; agente todavía no implementado. El respaldo es opcional y no puede formar ciclos. PATCH conserva identidad/revisión.
- Ruta: exactamente uno de `category_id|product_id`, `mode:prepare|none`, `station_ids`. Preparación exige estaciones activas no repetidas; exención explícita usa lista vacía. PATCH conserva `id/revision`; producto prevalece sobre categoría.
- Acceso: `{device_id,station_ids,expected_station_ids,operation_id}` y, desde la etapa 9F, `all_stations`, `sound_alert` y `expected_all_stations` opcionales (ver «Etapa 9F · cocina por estaciones»). La lista esperada del último GET permite rechazar una edición concurrente. Solo dispositivos autorizados existentes.
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

Migración `0016_inventory_suppliers_counter.sql`. Todas las rutas siguen bajo `/api/v1/inventory`, exigen `inventory.read`
además de su permiso y conservan idempotencia (`operation_id`), revisión, auditoría y eventos de la etapa 8.

### Proveedores (`suppliers`)

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

### Órdenes de compra y sugerencias

- `POST purchase-orders`, `PATCH purchase-orders/{id}` (cambiar el proveedor de un borrador) y
  `POST purchase-suggestions/draft-order` con el proveedor de mostrador → 409 `COUNTER_SUPPLIER` («… solo se usa en
  recepciones directas: elige un proveedor formal para la orden de compra.»). Sin escrituras.
- Las sugerencias solo consideran proveedores formales (`sp.kind='formal'`).

### Recepciones (`receipts`)

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

### Código interno de insumos (`item-codes`, nueva)

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

### Puente nativo (`desktop/src-tauri/src/api.rs`)

- Operación nueva `inventory_item_code` → `GET inventory/item-codes`.
- Parámetros de consulta nuevos para `inventory_*`: `code` y `supplier_kind` (`kind` ya existía).
- Tipos TypeScript: `SupplierKind`, `Supplier.phone_secondary|mobile_phone|website|kind`, `SupplierReference.kind`,
  `ReceiptSummary.counter_store`, `DirectReceiptInput.counter_store`, `SupplierQuery.kind`, `ReceiptQuery.supplier_kind`
  (`services/inventoryPurchasing.ts`) e `ItemCodeCheck` / `checkInventoryItemCode` (`services/inventory.ts`).

### Esquema (para reportes)

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

## Etapa 9A · reportes de caja, inventarios, costos y márgenes, compras, proveedores y empleados (parte B2, sin migración nueva)

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

## Etapa 9A · exportación PDF y Excel y operación del puente (parte C1, sin migración nueva)

### `GET reports/export/{id}` (exportación a PDF y Excel)

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

### Puente nativo: operación cerrada `export_report` (`desktop/src-tauri/src/export.rs`)

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

### Frontend: `desktop/src/services/reportExport.ts`

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

### Playwright: `desktop/e2e/native-driver.ts`

`installDriver(page)` devuelve `{ request, nextSaveDialog(choice: 'save' | 'cancel'), exports }`. La rama
`export_report` lee la lista cerrada de `export.rs` y la lista blanca de `api.rs`, pide el archivo a Go desde Node,
valida tipo, firma y nombre como Rust y aplica la siguiente respuesta del diálogo de la cola (sin respuesta: guardar).
Los archivos guardados quedan en `exports` (`{report, format, fileName, contentType, bytes: Buffer}`) en Node; el
navegador recibe solo el resultado. Los rechazos del puente llegan al WebView como texto, igual que con Tauri.

## Etapa 9A · interfaz de Reportes (parte C2, sin cambios de contrato)

La parte C2 no agrega ni modifica rutas, operaciones del puente, permisos ni formas JSON. Consume:

- `GET reports/catalog` (`reports_catalog`): navegación, filtros y columnas; `dashboard`, `periods`, `timezone`,
  `day_start`; `export.pdf` / `export.excel` (los botones exigen además `reports.export.pdf` /
  `reports.export.excel` en la sesión).
- `GET reports/run/{id}` (`reports_run`): `period` + `date` | `week=AAAA-Wss` | `month=AAAA-MM` | `year` |
  `from`/`until`; `compare=previous|none` solo si `comparison: true`; `page`, `page_size=50`; `sort`/`direction`
  al ordenar; los filtros que declara el reporte (solo con valor).
- `GET reports/dashboard` (`reports_dashboard`): los mismos parámetros de periodo y siempre `compare`.
- `GET reports/filters/{source}` (`reports_filters`): una vez por fuente; `search` solo en `products` e
  `inventory_items`.
- Operación cerrada `export_report` (parte C1) mediante `exportReport(id, 'pdf'|'xlsx', consulta)` de
  `services/reportExport.ts`, con la misma consulta de la pantalla (sin paginación).

Pantalla de inicio `reports` (`reports.read`) registrada en `PreferencesPage.tsx`, ya admitida por Go desde B1
(`administration/preferences.go`).

## Etapa 9A · correcciones de la revisión en reportes (F1)

Sin rutas, permisos ni migraciones nuevas. Cambian el contenido y los permisos efectivos de rutas existentes:

### `GET reports/run/payments.*` y `GET reports/export/payments.*`

- Todos los reportes del grupo `payments` (`payments.by_method`, `by_register`, `by_shift`, `by_user`, `by_day`, `list`,
  `mixed_partial`, `refunds`, `corrections`, `tips`, `currency`) se limitan, como los `cash.*`, a los turnos que el usuario
  puede consultar en Caja: `cash.shifts.read` (sin él, ningún cobro); otras cajas que la de la terminal con
  `cash.shifts.read_all`; turnos cerrados con `cash.history.read` o si él los cerró. Excepción: `payments.pending` (cuentas
  abiertas, sin turnos), sin cambio.
- `meta.warnings` incluye `{"code":"CASH_SCOPE","message":"Solo se incluyen los turnos que puedes consultar en Caja (…)"}`
  si falta cualquiera de esos tres permisos. `SHIFT_CLOSING` ahora cuenta solo los turnos en arqueo dentro del alcance.
- `meta.warnings` es siempre un arreglo (antes podía omitirse el aviso; el formato JSON no cambia).
- El valor mostrado de un filtro `shift_id` fuera del alcance (`meta.filters[].display`) es «Registro no encontrado».
- `payments.currency`: `mxn_in`/`mxn_out` de un movimiento anterior a la 0007 (sin columnas por moneda) =
  `cash_received` / `cash_received − net_amount`, como Caja (`currencyTotals.add`) y `cash.*`.

### `GET reports/run/employees.activity` (y su exportación)

- `payments`, `collected`, `tips`, `refunds`, `refunded` y `cash_operations` solo cuentan los turnos del alcance de Caja
  (mismas reglas que `payments.*`); `SHIFT_CLOSING` cuenta los turnos en arqueo del alcance y `CASH_SCOPE` («Los cobros,
  las devoluciones y los movimientos de caja solo incluyen…») aparece si el alcance es limitado. Las demás columnas (venta
  como responsable, confirmaciones, cancelaciones, descuentos, autorizaciones) no cambian.

### `GET reports/dashboard`

- Secciones `payments` (cobros), `cash` y `cash_differences` con el mismo alcance de Caja; `warnings` incluye una vez
  `CASH_SCOPE` cuando alguna de ellas se calculó con alcance limitado.

### `GET reports/filters/{source}`

- 404 `NOT_FOUND` si la fuente no existe (igual que antes); **403 `PERMISSION_DENIED`** (auditado como `access.denied`) si ninguno de
  los reportes que el usuario puede consultar declara un filtro con esa fuente.
- `shifts`: solo los turnos del alcance de Caja del usuario.

### `GET reports/run/sales.evolution` (y su exportación)

- Si el periodo anterior tiene más cubetas que el consultado, las sobrantes van al final como renglones
  `{"bucket":"<etiqueta> (solo en el periodo anterior)","bucket_start":null,"net":null,"previous_start":"AAAA-MM-DD",
"previous_net":"…","change":null,"change_percent":null}`, con una nota en `meta.notes`. `totals.previous_net`,
  `totals.change` y `totals.change_percent` usan el periodo anterior completo.

### `GET reports/run/sales.by_shift`

- `summary` (`accounts`, `net`, `tax`, `quantity`, `average_ticket`) excluye los turnos en arqueo como los renglones.

### `GET reports/run/purchases.received` y `purchases.by_item`

- `summary.returned_value` («Devoluciones a proveedor del periodo») aplica los filtros del reporte (proveedor, insumo y
  categoría por renglón de la devolución, almacén de la devolución, modalidad y estado de la orden de la recepción), con la
  fecha de la devolución.
- `summary.cash_linked`: con filtro de insumo o categoría su etiqueta es «Pagado desde caja (recepciones completas)» (el
  importe vinculado completo de cada recepción incluida, porque un vínculo no se reparte por insumo); sin esos filtros sigue
  siendo exacto y con la etiqueta «Pagado desde caja».

### Reconocimiento de ventas (`payments.closeIfSettled` → `app.sales_recognitions`)

- Al cerrar una cuenta también se llevan a cero las fuentes de sus unidades **canceladas que hoy le pertenecen** aunque se
  hayan reconocido en otra cuenta (unidad transferida tras una reapertura y cancelada en la cuenta nueva). Cada unidad se
  revierte una sola vez (suma global), cierre primero la cuenta que cierre.

## Etapa 9A · correcciones de la revisión en precios, captura, inventario y puente (F2)

Sección afectada de `API_CONTRACTS.md`: «Cotizaciones (`catalog/preview`, `restaurant/accounts/{id}/draft-preview`,
`…/preview`, `…/edit-preview`)» (desglose `ProductBreakdown`, también en cada componente de combo y en `quote_line` de
`restaurant_item_versions`). Ninguna ruta, permiso ni migración nueva. Con la opción desactivada **nada cambia**: los
campos nuevos se omiten y la prueba de huellas SHA-256 anteriores a la 9A sigue aprobada byte a byte.

### Campo nuevo `groups[].applied_difference` (solo con la opción aplicada)

```json
{
  "base_price": "20.00",
  "group_extras": "-20.00",
  "unit_price": "0.00",
  "price_can_decrease": true,
  "complement_adjustment": "-20.00",
  "groups": [
    {
      "mode": "value",
      "original_value": "35.00",
      "requested_value": "0.00",
      "difference": "-35.00",
      "applied_difference": "-20.00",
      "extras": "0.00"
    }
  ]
}
```

- `applied_difference` (cadena con signo, `omitempty`): efecto **real** del grupo en el precio por unidad = diferencia
  con la regla con piso + la parte de la reducción efectiva que se le asignó. La reducción efectiva
  (`complement_adjustment`) se reparte entre los grupos y después entre los componentes, en el orden del desglose y sin
  pasar de lo que permite cada uno; la suma de los efectos es exactamente el ajuste (salvo el caso de centavos de abajo).
  Con los límites del precio puede ser menor que `difference` en valor absoluto; nunca promete más de lo aplicado. Una
  diferencia positiva siempre aparece completa.
- `difference` no cambia de significado en modo valor (solicitado − incluido, con signo con la opción).

### Cambio de regla en modo cantidad (`groups[].difference` con la opción)

- Antes: tramos − valor de referencia de las porciones incluidas retiradas.
- Ahora: **tramos + min(0, valor solicitado − valor incluido)**. Solo bajan las diferencias negativas de valor (§0-B):
  sustituir por un complemento más caro nunca baja el precio (ensalada $15 → papas $25 con tramo de $15: $165, igual que
  sin la opción; antes daba $150). Sin valores de referencia no cambia nada.

### Combos (`complement_adjustment` y `unit_price` de un componente)

- La reducción de complementos de cada componente (cuando el combo y el componente tienen la opción) se acumula con
  signo **sin acotarla al precio suelto del componente**; los límites (no bajar de cero, ni del valor de los complementos
  que permanecen, ni subir sobre la regla con piso) se aplican **una sola vez, en el núcleo del combo**. El resultado ya
  no depende del precio suelto del componente (combo de $160 con el filete sin ensalada: $147.50 con el filete suelto a
  $0, $5, $30 o $150; antes $162.50, $162.50, $152.50 y $147.50).
- En el producto de un componente, `complement_adjustment` es ahora la parte que ese componente aportó al precio del
  combo, **por unidad del componente**, y **no** se incluye en su `group_extras` ni en su `unit_price`, que siguen la
  regla con piso (el combo lo cubre; solo su cargo se cobra). En la partida (la raíz) sigue incluida en `group_extras` y
  `unit_price` (`unit_price = base_price + ingredient_extras + group_extras + component_extras`).
- `components[].charge` no cambia (nunca negativo; regla con piso).
- Si la parte de un componente con varias unidades no se divide exacta en centavos, la parte por unidad se trunca hacia
  cero (lo mostrado nunca supera lo aplicado; ejemplo: el combo baja $5.01 con dos filetes y cada uno muestra −$2.50).

### Inventario (sin cambio de forma)

- `GET inventory/item-codes`: la respuesta es la misma (`code`, `available`, `suggestion`, `taken_by`). `suggestion`
  ahora compara con la clave del índice único (`lower()` de PostgreSQL con la configuración regional de la base, C en
  el proyecto): con «Ñ» o acentos ya no sugiere un código ocupado.
- `POST/PATCH inventory/suppliers…`: `phone_secondary`, `mobile_phone` y `website` son opcionales en el cuerpo.
  **Ausentes** al editar se conservan (antes se borraban: un cliente anterior a la 9A los borraba al guardar); al crear,
  ausentes equivalen a vacío. Presentes se validan como antes y `""` los borra. La respuesta no cambia.

### Puente (`export_report`, sin cambio de contrato con React)

- Sigue devolviendo solo `{result: saved|cancelled, file_name, size}` o el rechazo de Go `{status, body}`.
- Si la ruta elegida no tiene la extensión del formato y, con ella, el archivo ya existe, el diálogo nativo se vuelve a
  abrir en la misma carpeta con el nombre completo (hasta 4 aperturas por exportación; después, el error de siempre).
- El temporal de la escritura atómica es `.adel-export-<pid>-<ns>-<seq>.part` (menos de 80 bytes, independiente del
  nombre elegido).

## Etapa 9B · plantillas de documentos, vista previa, muestras e impresión con la plantilla vigente (fase A, esquema 18)

Todas bajo `/api/v1`, autenticadas como el resto (dispositivo + sesión + permisos leídos en cada petición) y con una
transacción por petición. Rutas del paquete `backend/internal/designer`, delegadas desde `administration/routes.go` con el
prefijo `documents/` (salvo `documents/identity*`, fase B). Ninguna ruta acepta HTML, CSS, SQL ni bytes ESC/POS: una
plantilla es una lista de bloques de un catálogo cerrado validado en Go. Operación del puente (`api.rs`) entre corchetes.

### Reglas comunes

- Permisos comprobados antes de leer el cuerpo, de tomar locks y de buscar el resultado idempotente; un 403 revierte con
  el savepoint y audita `access.denied`. Ruta o método inexistente → `404 NOT_FOUND`. Cuerpo ≤ 64 KiB.
- Escrituras: `operation_id` UUID obligatorio, idempotencia en `app.restaurant_operations` (mismo cuerpo → respuesta
  guardada; otro cuerpo → `409 IDEMPOTENCY_CONFLICT`). Las que cambian una plantilla llevan `revision` y bloquean su fila
  (`FOR UPDATE`).
- Errores (`{"error": {"code", "message", "request_id", "details"?}}`, mensaje en español): `400 DOCUMENTS_VALIDATION`
  (identificador, tipo, ancho, fuente o cuerpo inválidos); `422 TEMPLATE_INVALID` con `details.block_id` (vacío si el
  problema es del documento); `409 REVISION_CONFLICT` con `details: {revision, current_version_id}` (otro administrador
  guardó antes; el cliente conserva su borrador); `409 TEMPLATE_READ_ONLY` («La plantilla predeterminada no se modifica:
  duplícala para editarla.»); `409 TEMPLATE_NAME_TAKEN` (nombre repetido en el tipo, sin distinguir mayúsculas); `409
SOURCE_KIND_MISMATCH` (versión de origen o documento real de otro tipo); `403 PERMISSION_DENIED` y los errores de origen
  (`STATION_SCOPE_DENIED`, etc.) en la vista previa real; `404 NOT_FOUND`.
- `kind` (lista cerrada): `receipt`, `precheck`, `kitchen_order`, `kitchen_modification`, `kitchen_cancellation`,
  `cash_income`, `cash_withdrawal`, `cash_expense`, `cash_partial_cut`, `cash_final_cut`.

### Layout (JSON de una versión)

```json
{
  "options": { "text_size": "inherit" },
  "blocks": [
    {
      "id": "b1",
      "type": "field",
      "visible": true,
      "align": "left",
      "size": "normal",
      "bold": false,
      "uppercase": false,
      "spacing_before": 0,
      "spacing_after": 0,
      "options": {
        "items": [{ "key": "date", "label": "Fecha:" }],
        "layout": "inline",
        "omit_empty": true
      }
    }
  ]
}
```

- `id` `^[a-z0-9-]{1,40}$` único; `align` `left|center|right`; `size` `normal|double_height|double_width|double`;
  `spacing_*` 0–3. Propiedades ausentes toman su valor por omisión; claves desconocidas (también en `options`) se
  rechazan; `options` ausentes = las del tipo. `options.text_size`: `inherit|normal|large` en cocina, `normal` en las
  demás familias.
- Límites: 1–60 bloques, 32 KiB; texto fijo ≤ 300 caracteres y ≤ 8 líneas; etiquetas y título ≤ 40; campo de datos 1–4
  datos (`pair`, exactamente 1); hasta 3 logotipos. Los textos se sanean al guardar (controles e invisibles eliminados,
  tabuladores a espacio, `\r\n` → `\n`). Se guarda la forma canónica (todas las propiedades y opciones explícitas).
- Bloques compatibles por tipo, opciones y claves de `field`: [DOCUMENT_DESIGNER.md](docs/DOCUMENT_DESIGNER.md),
  «Catálogo de bloques» y «Campos de datos». En `kitchen_items`, `show_prices` admite `inherit|no` (corrección de la
  revisión: para imprimir importes se activa «Incluir importes comerciales en comandas» en Cocina).

### Plantillas

`TemplateSummary`:

```json
{
  "id": "uuid",
  "kind": "receipt",
  "kind_name": "Ticket de venta",
  "name": "Predeterminada",
  "system": true,
  "active": true,
  "revision": 1,
  "current_version": {
    "id": "uuid",
    "number": 1,
    "note": "…",
    "created_at": "…",
    "created_by_name": null,
    "block_count": 17
  },
  "created_at": "…",
  "created_by_name": null,
  "updated_at": "…",
  "updated_by_name": null
}
```

`TemplateDetail` = `TemplateSummary` + `"layout"`. Plantillas del sistema: `9b0d0c00-0000-4000-8000-0000000000NN`
(versión 1: `9b0d0c01-0000-4000-8000-0000000000NN`), NN = 01 `receipt` … 10 `cash_final_cut` en el orden de `kind`.

| Ruta                                                                     | Permisos                                                                                                   | Cuerpo / consulta                                                                        | Respuesta                                                                               |
| ------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| `GET documents/catalog` [`documents_catalog`]                            | `documents.templates.read`                                                                                 | —                                                                                        | `Catalog`                                                                               |
| `GET documents/templates?kind=` [`documents_templates`]                  | read                                                                                                       | `kind` opcional                                                                          | `TemplateSummary[]` (por tipo; la activa primero)                                       |
| `GET documents/templates/{id}` [`documents_template`]                    | read                                                                                                       | —                                                                                        | `TemplateDetail`                                                                        |
| `POST documents/templates` [`documents_create_template`]                 | read + `documents.templates.manage`                                                                        | `{operation_id, kind, name, source_version_id?}`                                         | `TemplateDetail` inactiva; versión 1 = copia del origen o la predeterminada del tipo    |
| `PUT documents/templates/{id}` [`documents_save_template`]               | read + manage                                                                                              | `{operation_id, revision, layout, note?}` (nota ≤ 200)                                   | `TemplateDetail` con la versión nueva (revisión + 1)                                    |
| `POST documents/templates/{id}/rename` [`documents_rename_template`]     | read + manage                                                                                              | `{operation_id, revision, name}` (1–80)                                                  | `TemplateDetail`                                                                        |
| `POST documents/templates/{id}/activate` [`documents_activate_template`] | read + manage                                                                                              | `{operation_id, revision}`                                                               | `TemplateDetail` activa; la anterior queda inactiva y solo la activada sube de revisión |
| `GET documents/templates/{id}/versions` [`documents_template_versions`]  | read                                                                                                       | —                                                                                        | `VersionSummary[]` (la más reciente primero, hasta 200)                                 |
| `GET documents/versions/{id}` [`documents_version`]                      | read                                                                                                       | —                                                                                        | `VersionDetail` = `VersionSummary` + `template_id`, `template_name`, `kind`, `layout`   |
| `POST documents/restore-default` [`documents_restore_default`]           | read + manage                                                                                              | `{operation_id, kind}`                                                                   | `TemplateDetail` de la plantilla del sistema, activa                                    |
| `POST documents/preview` [`documents_preview`]                           | read; con documento real además `documents.preview.real` y el permiso de lectura de su origen              | `{kind, layout, paper_width: 58\|80, source: {"sample": true} \| {"ticket_id": "uuid"}}` | `Preview`. Sin escrituras                                                               |
| `GET documents/recent?kind=` [`documents_recent`]                        | read + `documents.preview.real` + el permiso de lectura de su origen (comprobantes: `cash.movements.read`) | `kind` obligatorio                                                                       | `[{ticket_id, kind, created_at, label}]` (hasta 20; la etiqueta no lleva importes)      |
| `POST documents/samples` [`documents_print_sample`]                      | `documents.samples.print`                                                                                  | `{operation_id, kind, layout, printer_id}`                                               | `{status, job_id, ticket_id, printer_id, message}`                                      |
| `GET documents/printers` [`documents_printers`]                          | read                                                                                                       | —                                                                                        | `PrinterCompatibility[]`                                                                |

- Restaurar una versión anterior = `PUT` con el `layout` de `GET documents/versions/{id}` (la interfaz agrega la nota
  «Restaurada desde la versión N»).
- La vista previa y los recientes toman el lock 7820148305 compartido; la muestra, exclusivo.
- Permisos de origen de la vista previa real y de los recientes: comandas `kitchen.tickets.read` (ámbito de estaciones;
  importes con `catalog.prices.read`); precuenta y ticket `payments.read` (recientes: `payments.read` + `accounts.read`, y
  para llevar con `takeaway.read`); caja `cash.shifts.read` con el alcance de Caja y, en comprobantes, `cash.movements.read`.

### `Catalog`

```json
{
  "kinds": [
    {
      "kind": "receipt",
      "name": "Ticket de venta",
      "family": "commercial",
      "text_sizes": ["normal"],
      "blocks": [
        {
          "type": "field",
          "name": "Campo de datos",
          "group": "common",
          "description": "…",
          "properties": [
            "visible",
            "align",
            "size",
            "bold",
            "uppercase",
            "spacing_before",
            "spacing_after"
          ],
          "options": [
            {
              "key": "items",
              "label": "Datos",
              "type": "field_items",
              "default": [],
              "max_length": 40,
              "min": 1,
              "max": 4,
              "required": true
            },
            {
              "key": "layout",
              "label": "Disposición",
              "type": "enum",
              "default": "inline",
              "values": [
                { "value": "inline", "label": "…" },
                { "value": "pair", "label": "…" }
              ]
            },
            {
              "key": "omit_empty",
              "label": "Omitir el renglón si no hay datos",
              "type": "boolean",
              "default": true
            }
          ],
          "max": 0,
          "required": false,
          "protected": false
        }
      ],
      "field_keys": [{ "key": "date", "name": "Fecha", "default_label": "Fecha:" }],
      "shortcuts": [
        {
          "name": "Fecha",
          "options": {
            "items": [{ "key": "date", "label": "Fecha:" }],
            "layout": "inline",
            "omit_empty": true
          }
        }
      ]
    }
  ],
  "paper_widths": [
    { "paper_width": 58, "columns": 30 },
    { "paper_width": 80, "columns": 42 }
  ],
  "properties": {
    "align": [{ "value": "left", "label": "…" }],
    "size": [{ "value": "normal", "label": "…" }],
    "spacing": { "min": 0, "max": 3 }
  },
  "limits": {
    "blocks": 60,
    "bytes": 32768,
    "fixed_text_chars": 300,
    "fixed_text_lines": 8,
    "label_chars": 40,
    "field_items": 4,
    "spacing": 3,
    "spacer_lines": 5
  }
}
```

`OptionSpec.type`: `enum` (con `values`), `boolean`, `text` (`max_length`, `max_lines`), `integer` (`min`, `max`),
`field_items` (lista de `{key, label}` con claves de `field_keys`). `max`: apariciones permitidas (0 = sin límite propio);
`required`: exactamente uno y visible (aviso fiscal); `protected`: esperado y diferencias.

### `Preview`

```json
{
  "kind": "receipt",
  "paper_width": 58,
  "columns": 30,
  "text_size": "normal",
  "sample": true,
  "includes_expected": false,
  "lines": [
    {
      "text": "TOTAL         $500.00",
      "bold": true,
      "size": "double_height",
      "block_id": "total",
      "image": null
    },
    {
      "text": "",
      "bold": false,
      "size": "normal",
      "block_id": "logo",
      "image": {
        "raster_id": "uuid",
        "width_dots": 384,
        "height_dots": 96,
        "data_uri": "data:image/png;base64,…"
      }
    }
  ],
  "text": "…proyección de texto (lo que va en kitchen_print_jobs.content)…",
  "warnings": ["No hay logotipo cargado: el bloque no imprime nada."],
  "empty_blocks": [{ "block_id": "usd-total", "reason": "…" }]
}
```

`columns` es el ancho de una línea normal (15/21 con texto grande de cocina); una línea `double_width`/`double` tiene a lo
más `columns/2` caracteres. `includes_expected` indica si el bloque protegido se renderizó: con datos de muestra, según
`cash.expected.read`; con un documento real, además fuera del arqueo de su turno y solo si el original lo incluía. Con un
documento de caja real emitido desde la 9B se usan sus datos guardados; con uno anterior, el corte o la operación
actuales, con un aviso en `warnings` (corrección de la revisión).

### `PrinterCompatibility`

```json
{
  "id": "uuid",
  "name": "Tickets",
  "transport": "cups",
  "paper_width": 80,
  "columns": 42,
  "format": { "status": "supported|unsupported|unknown", "reason": "…" },
  "logo": { "status": "supported|unsupported|unknown", "reason": "…" }
}
```

Solo impresoras activas. ESC/POS por red: `supported`. CUPS (consulta de solo lectura `lpstat`/`lpoptions`, con un límite
total de tiempo por petición): `supported` solo si la impresora tiene activada la opción «Esta impresora entiende ESC/POS
(formato y logotipo)» y la cola admite raw; `unsupported` sin la opción o sin raw («imprime el texto sin negrita, tamaños
ni logotipo»); `unknown` si la cola no se pudo consultar. Agente: `unsupported`. Sin logotipo cargado, `logo.reason`
empieza con «No hay logotipo cargado: el bloque «Logotipo» no imprime nada.».

### Cambios de la revisión independiente (fase D)

Correcciones posteriores a las fases A y B, verificadas con pruebas que fallaban antes de corregir (detalle en [docs/ETAPA_9B_AVANCE.md](docs/ETAPA_9B_AVANCE.md), fase D).

Todos bajo `/api/v1`. No hay rutas nuevas ni cambios en `api.rs`; solo campos, valores y mensajes.

#### Impresoras de Cocina: `accepts_escpos`

`GET kitchen/printers`, `POST kitchen/printers` y `PATCH kitchen/printers/{id}` (permiso `kitchen.printers.manage`, sin
cambios) llevan un campo nuevo en el objeto de impresora:

```json
{
  "id": "…",
  "name": "Tickets USB",
  "transport": "cups",
  "destination": "POS_80",
  "port": 9100,
  "paper_width": 80,
  "copies": 1,
  "supports_cut": false,
  "cut": false,
  "cutter_option": "",
  "accepts_escpos": false,
  "active": true,
  "fallback_id": null,
  "revision": 1
}
```

- `accepts_escpos` (boolean, por omisión `false`; ausente en el cuerpo = `false`): «Esta impresora entiende ESC/POS
  (formato y logotipo)». Solo se admite en `transport = "cups"`. Con `escpos_tcp` o `agent` y `true` → `400
KITCHEN_VALIDATION` («La opción «Esta impresora entiende ESC/POS» solo se configura en colas CUPS…»). Defensa en la
  base: `kitchen_printers_accepts_escpos_check CHECK (NOT accepts_escpos OR transport = 'cups')` (migración 0018).
- Las impresoras existentes quedan en `false`. ESC/POS por red recibe formato y logotipo sin la marca.
- La fotografía de la impresora en cada trabajo (`printer_snapshot`) lleva el campo; los trabajos anteriores sin él se
  tratan como `false`.

Efecto en la cola (un documento **con** negrita, tamaños o logotipo; los planos, incluidas las plantillas
predeterminadas, siguen el camino de siempre sin cambios):

| Transporte   | Marca   | Cola raw         | Envío                             | Aviso añadido a `last_error`/`message`                                                                                                      |
| ------------ | ------- | ---------------- | --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `escpos_tcp` | —       | —                | ESC/POS con atributos             | —                                                                                                                                           |
| `cups`       | `false` | (no se consulta) | texto por el camino anterior      | « La impresora no está marcada como «Esta impresora entiende ESC/POS (formato y logotipo)»: el documento se envió sin formato ni logotipo.» |
| `cups`       | `true`  | no               | texto por el camino anterior      | « La cola no admite datos raw: el documento se envió sin formato ni logotipo.»                                                              |
| `cups`       | `true`  | sí               | `lp -o raw` con los bytes ESC/POS | —                                                                                                                                           |
| `cups`       | `true`  | no responde      | `error` como antes                | «No se pudo consultar la cola CUPS…»                                                                                                        |

Las reglas de `submitted`/`uncertain`/`error` y de respaldo no cambian.

#### `GET documents/printers`

`format` y `logo` llevan un campo nuevo `code`:

```json
{
  "id": "…",
  "name": "…",
  "transport": "cups",
  "paper_width": 80,
  "columns": 42,
  "format": {
    "status": "unsupported",
    "code": "escpos_disabled",
    "reason": "Desactivado en la impresora: …"
  },
  "logo": { "status": "unsupported", "code": "escpos_disabled", "reason": "…" }
}
```

| `code`              | `status`      | Cuándo                                                                                     |
| ------------------- | ------------- | ------------------------------------------------------------------------------------------ |
| `escpos_network`    | `supported`   | ESC/POS por red                                                                            |
| `escpos_disabled`   | `unsupported` | CUPS con `accepts_escpos=false` («Desactivado en la impresora: …»; la cola no se consulta) |
| `queue_raw`         | `supported`   | CUPS marcada y cola raw                                                                    |
| `queue_not_raw`     | `unsupported` | CUPS marcada y cola sin raw («Cola sin raw: …» + motivo de CUPS)                           |
| `queue_unavailable` | `unknown`     | CUPS no respondió por la cola (motivo de `cups_raw.go`)                                    |
| `queue_timeout`     | `unknown`     | se agotó el límite total antes de consultar la cola («CUPS no respondió a tiempo: …»)      |
| `agent`             | `unsupported` | agente futuro                                                                              |

- Límite total de 4 s para todas las consultas a CUPS de la petición (como el cajón; cada consulta conserva su límite de
  3 s), una cola tras otra. La ruta no toma locks consultivos.
- Sin logotipo cargado, `logo.reason` empieza con «No hay logotipo cargado: …» (sin cambios).

#### Catálogo del diseñador: «Importes» de la lista de cocina

`GET documents/catalog`: el bloque `kitchen_items` declara

```json
{
  "key": "show_prices",
  "label": "Importes",
  "type": "enum",
  "default": "inherit",
  "values": [
    { "value": "inherit", "label": "Según la configuración de Cocina" },
    { "value": "no", "label": "Ocultar" }
  ],
  "help": "Para imprimir los importes, activa «Incluir importes comerciales en comandas» en Cocina → Comandas y tiempos; con «Ocultar», esta plantilla no los imprime aunque Cocina los muestre."
}
```

- `OptionSpec.help` (cadena opcional, `omitempty`) es nuevo para cualquier opción; hoy solo lo usa `show_prices`.
- `show_prices: "yes"` ya no es válido: `PUT documents/templates/{id}`, `POST documents/preview` y `POST documents/samples`
  responden `422 TEMPLATE_INVALID` con `details.block_id` del bloque y el mensaje «Los importes se imprimen según la
  configuración de Cocina o se ocultan. Para imprimirlos, activa «Incluir importes comerciales en comandas» en Cocina.»
  `show_notes` conserva `inherit | yes | no`.
- Datos de muestra de cocina (vista previa con `{"sample": true}` y `POST documents/samples`): toman la configuración
  vigente de Cocina (`show_prices`, `show_notes`, `show_establishment`, `text_size`, `line_spacing`, `quantity_style`)
  y, sin `show_prices`, la muestra no lleva importes (la misma regla que la comanda real). Con la configuración por
  omisión de Cocina el resultado es el de antes.

#### `POST documents/preview` con un documento de caja real

- Documento emitido desde la 9B: usa los datos guardados en su ticket (los de «Imprimir con la plantilla vigente») y la
  misma regla del esperado: `includes_expected` solo si el original lo incluía, quien consulta tiene `cash.expected.read`
  y el turno no está en arqueo. Permisos de origen sin cambios (`cash.shifts.read`, reglas del turno y, en
  comprobantes, `cash.movements.read`).
- Documento anterior a la 9B (sin datos guardados): se lee el corte u operación actuales y `warnings[0]` es «Este
  documento se emitió antes de las plantillas de documentos y no guardó sus datos: la vista previa usa los datos actuales
  de su corte u operación y puede no coincidir con lo que se imprimió.» (los avisos de los bloques siguen después).

#### `GET documents/recent?kind=` de caja

- `cash_income`, `cash_withdrawal`, `cash_expense`: exigen además `cash.movements.read` → `403 PERMISSION_DENIED`.
- `cash_partial_cut`, `cash_final_cut`: `cash.shifts.read` (sin cambios).
- Alcance de turnos en una sola consulta con `LIMIT 20` (la caja de la terminal o todas con `cash.shifts.read_all`;
  los cerrados con `cash.history.read` o para quien los cerró). La forma de la respuesta no cambia.

#### `409 PRINT_COPY_PENDING` entre familias

Las rutas `POST kitchen/reprint`, `POST kitchen/print-jobs/rerender`, `POST payments/accounts/{id}/documents/reprint`,
`POST payments/accounts/{id}/documents/rerender`, `POST payments/cash-documents/{id}/reprint` y
`POST payments/cash-documents/{id}/rerender` responden `409 PRINT_COPY_PENDING` mientras el documento de origen tenga
cualquier trabajo sin resolver (`pending`, `processing`, `uncertain`, `error`, `intervention`): sus copias exactas, sus
impresiones con la plantilla vigente o las copias de estas desde la cola. Mensaje único: «Ya existe una copia de este
documento pendiente o sin resolver (reimpresión o impresión con la plantilla vigente). Revisa ese trabajo antes de
solicitar otra.» Si el trabajo original sigue sin resolver se mantiene el `400` anterior.

#### Trabajo enviado a una impresora de otro ancho

Si el layout de un trabajo se renderizó para otro ancho de papel que el de su impresora (respaldo, redirección o copia a
otra impresora), o un raster es más ancho que el área imprimible, la imagen no se envía; el resto se imprime y el
resultado agrega « El logotipo se preparó para otro ancho de papel que el de esta impresora: se omitió.»

#### Logotipo

`POST documents/identity/logo`: el archivo se valida antes del lock de la identidad, así que un archivo inválido
responde `422 VALIDATION_ERROR` aunque la revisión sea vieja (antes se comprobaba primero la revisión). Rechazos nuevos
de JPEG: «El logotipo JPEG tiene demasiados barridos progresivos. Guárdalo de nuevo como PNG o como JPEG estándar.»
(más de 100 SOS) y «El logotipo JPEG tiene demasiados segmentos. …» (más de 1024 segmentos con longitud).

#### Migración 0019

`document_identity` agrega `logo_raster_58_paper integer NOT NULL DEFAULT 58 CHECK (= 58)` y
`logo_raster_80_paper … DEFAULT 80 CHECK (= 80)` (constantes que Go no escribe) y sus llaves compuestas pasan a
`(logo_raster_58_id, logo_id, logo_raster_58_paper)` / `(…80…)` → `document_logo_rasters (id, logo_id, paper_width)`
(la llave única `document_logo_rasters_logo_key` es ahora `(id, logo_id, paper_width)`). La API no cambia.

### Impresión con la plantilla vigente (en sus módulos de origen)

Acción distinta de la reimpresión exacta: renderiza los datos históricos con la plantilla activa del tipo y la identidad
vigente, en la impresora del trabajo original, en un ticket nuevo `document_rerender`. Nunca crea `payment_documents`,
`cash_documents`, ventas ni movimientos. Se rechaza con `400 KITCHEN_VALIDATION` si el original sigue pendiente o incierto
y con `409 PRINT_COPY_PENDING` mientras haya una copia sin resolver del mismo documento, sea una reimpresión exacta, una
impresión con la plantilla vigente o una copia de esta (el bloqueo es mutuo también para `…/reprint`).

| Ruta                                                                             | Permisos                                                                                                                                                     | Cuerpo                                                | Respuesta                                                                                                                                          | Auditoría                                                                     |
| -------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| `POST kitchen/print-jobs/rerender` [`kitchen_rerender`]                          | `kitchen.reprint` + `kitchen.stations.all` + `documents.history.render_current` (otra impresora: `kitchen.printers.manage`)                                  | `{operation_id, job_id, reason (5–500), printer_id?}` | `{job: PrintJob, ticket_id, template_version_id}`                                                                                                  | `print.rerendered` (+ historial de cocina)                                    |
| `POST payments/accounts/{id}/documents/rerender` [`payments_rerender`]           | `payments.tickets.reprint` (+ acceso a la cuenta) + `documents.history.render_current`                                                                       | `{operation_id, ticket_id}`                           | `{status, job_id, ticket_id, source_ticket_id, template_version_id}`                                                                               | `payments.document_rerendered`                                                |
| `POST payments/cash-documents/{id}/rerender` [`payments_rerender_cash_document`] | `cash.reports.reprint` (+ turno) + `documents.history.render_current`; si incluye el esperado, `cash.expected.read` y fuera del arqueo (`409 SHIFT_CLOSING`) | `{operation_id}`                                      | `PrintOutcome` (`requested`, `status`, `document_id` = el documento de caja original, `ticket_id` nuevo, `job_id`, `includes_expected`, `message`) | `cash.document_rerendered`; evento `cash.document.rerendered` (ámbito `cash`) |

`kitchen/print-jobs/rerender` solo acepta comandas y avisos (`normal`, `modification`, `cancellation`). Un documento de
caja anterior a la 9B (sin datos estructurados) responde `409 DOCUMENT_DATA_UNAVAILABLE`.

### Cambios en contratos existentes

- `GET payments/accounts/{id}/precheck` y los documentos comerciales guardados agregan `template_version_id` e `identity`
  (`{name, address?, phone?, email?, …}`). Los importes y el `content` no cambian con las plantillas predeterminadas.
- `GET kitchen/tickets` no lista `document_sample` ni `document_rerender`; `GET kitchen/tickets/{id}` de esos tipos aplica
  las reglas de su origen (caja: `cash.shifts.read` y el esperado con `cash.expected.read` fuera del arqueo; comerciales:
  cobros; comanda con importes: `catalog.prices.read`). `POST kitchen/reprint` de un documento de caja impreso con la
  plantilla vigente responde 400 («se reimprimen desde Caja»).
- Las respuestas de trabajos de la cola no incluyen la columna nueva `layout` (ni `content`).
- Un trabajo que se envía a una impresora de otro ancho (respaldo, redirección o copia a otra impresora) no envía el
  logotipo convertido para el ancho original.

### SSE y auditoría

- `documents.templates.changed` (ámbito nuevo `documents`, `aggregate_id` = plantilla, sin contenido) en la misma
  transacción de cada creación, guardado, renombre, activación y restauración. El ámbito lo reciben quienes tienen
  `documents.templates.read` (`events/store.go`).
- Auditoría (nunca el layout): `documents.template_created` (tipo, nombre, versión, origen, bloques),
  `documents.template_saved` (versión anterior y nueva, número, bloques, bytes, nota), `documents.template_renamed`,
  `documents.template_activated` y `documents.default_restored` (plantilla activa antes y después),
  `documents.sample_printed` (tipo, trabajo, ticket, bloques; entidad: la impresora; historial de la cola
  `print.sample_requested`), `print.rerendered`, `payments.document_rerendered` y `cash.document_rerendered`.

### Puente nativo (`desktop/src-tauri/src/api.rs`)

17 operaciones nuevas en la lista cerrada, en el formato que lee `e2e/native-driver.ts`, con un identificador por ruta y
la prueba Rust de la lista: `documents_catalog`, `documents_templates`, `documents_create_template`,
`documents_restore_default`, `documents_preview`, `documents_recent`, `documents_print_sample`, `documents_printers`,
`kitchen_rerender` (fijas); `documents_template`, `documents_save_template`, `documents_rename_template`,
`documents_activate_template`, `documents_template_versions`, `documents_version`, `payments_rerender` y
`payments_rerender_cash_document` (con identificador). Única clave de consulta admitida: `kind`.

### Permisos nuevos (0018, solo al administrador)

`documents.templates.read`, `documents.templates.manage`, `documents.preview.real`, `documents.samples.print`,
`documents.identity.manage` (la usa la fase B) y `documents.history.render_current`.

## Etapa 9B · identidad del establecimiento y logotipo (fase B, esquema 19)

Paquete `backend/internal/identity`, delegado desde `administration/routes.go` (prefijo `documents/identity`, antes que el
diseñador) con la misma transacción, `Actor` y auditoría. La identidad es única: ninguna ruta lleva identificador. Sin
permisos nuevos.

| Ruta                                   | Operación del puente              | Permisos (Go)                                    | Cuerpo                                                                              | Respuesta                         |
| -------------------------------------- | --------------------------------- | ------------------------------------------------ | ----------------------------------------------------------------------------------- | --------------------------------- |
| `GET documents/identity`               | `documents_identity`              | `documents.templates.read` **o** `settings.read` | —                                                                                   | `IdentityView` con vistas previas |
| `PUT documents/identity`               | `documents_save_identity`         | `documents.identity.manage`                      | `{operation_id, revision, additional_info, header_text, footer_text, presentation}` | `IdentityView` sin vistas previas |
| `POST documents/identity/logo`         | `documents_upload_identity_logo`  | `documents.identity.manage`                      | `{operation_id, revision, content_base64}`                                          | `IdentityView` sin vistas previas |
| `POST documents/identity/logo/remove`  | `documents_remove_identity_logo`  | `documents.identity.manage`                      | `{operation_id, revision}`                                                          | `IdentityView` sin vistas previas |
| `POST documents/identity/logo/preview` | `documents_preview_identity_logo` | `documents.identity.manage`                      | `{presentation}`                                                                    | `LogoPreview` (no escribe)        |

- Permisos antes de leer el cuerpo o escribir; un 403 revierte con el savepoint y audita `access.denied`. Otro método o
  subruta de `documents/identity*` → `404 NOT_FOUND`. El cuerpo de `POST documents/identity/logo` admite 2 MiB; el resto,
  64 KiB.
- Escrituras: `operation_id` UUID obligatorio (`422 VALIDATION_ERROR` si falta). El archivo se valida **antes** de tomar
  el lock de la fila de `app.document_identity` (corrección de la revisión); después se toma el lock y se busca el
  resultado idempotente (mismo actor, terminal y huella → respuesta guardada; otra huella → `409 IDEMPOTENCY_CONFLICT`;
  la huella incluye el archivo). `revision` distinta de la vigente → `409 REVISION_CONFLICT` («Otra persona guardó la
  identidad del establecimiento. Tus cambios siguen en pantalla…»), sin escribir.
- Decodificación estricta (claves desconocidas → `422`). Textos: saltos unificados, tabuladores a espacio, controles e
  invisibles eliminados (ESC, GS, C1, formato Unicode), espacios finales de cada renglón y renglones vacíos al inicio y al
  final recortados; después ≤ 300 caracteres y ≤ 8 renglones (`422` con el campo en el mensaje, p. ej. «El texto de pie
  admite como máximo 300 caracteres.»).
- `presentation`: `width_percent` 25–100, `align` `left|center|right`, `method` `threshold|dither`, `threshold` 1–255 (se
  valida siempre; con difuminado no interviene).
- Logotipo: base64 estándar estricto, ≤ 1 398 104 caracteres antes de decodificar; después PNG o JPEG por firma y
  `DecodeConfig`, ≤ 1 MiB, 1–2048 px por lado antes de decodificar, JPEG con un número acotado de barridos (corrección de
  la revisión) y decodificación completa. Mensajes (`422`): «El archivo del logotipo está vacío.», «El logotipo debe
  ocupar como máximo 1 MiB.», «El logotipo debe ser una imagen PNG o JPEG; no se aceptan SVG, GIF, WebP ni otros
  formatos.», «El logotipo debe medir entre 1 × 1 y 2048 × 2048 píxeles.», «El logotipo está dañado o incompleto.», «El
  archivo del logotipo no se recibió completo. Vuelve a elegirlo.» (base64 inválido).
- Retirar o previsualizar sin logotipo → `409 NO_LOGO` («No hay un logotipo cargado.»). Guardar sin cambios devuelve la
  vista sin crear revisión, auditoría ni evento. Las escrituras devuelven la vista **sin** `data_uri` (así se guarda en
  `restaurant_operations`); la interfaz vuelve a consultar `GET`.

### `IdentityView`

```json
{
  "revision": 4,
  "establishment": {
    "name": "Fonda Doña Rosa",
    "address": "Calle 1",
    "phone": "81 1234 5678",
    "email": ""
  },
  "additional_info": "RFC: XAXX010101000",
  "header_text": "¡Bienvenidos!",
  "footer_text": "Gracias por su visita",
  "presentation": {
    "width_percent": 60,
    "align": "center",
    "method": "threshold",
    "threshold": 128
  },
  "logo": null,
  "updated_at": "RFC 3339",
  "updated_by": null,
  "limits": {
    "text_max_characters": 300,
    "text_max_lines": 8,
    "logo_max_bytes": 1048576,
    "logo_max_dimension": 2048,
    "width_percent_min": 25,
    "width_percent_max": 100,
    "threshold_min": 1,
    "threshold_max": 255,
    "max_height_dots": 320,
    "paper_widths": [
      { "paper_width": 58, "width_dots": 384 },
      { "paper_width": 80, "width_dots": 576 }
    ]
  },
  "warnings": []
}
```

Con logotipo, `logo` es `{id, media_type: image/png|image/jpeg, width, height, size_bytes, sha256, uploaded_at,
uploaded_by: {id, name}, data_uri (solo GET: el original), rasters: [{paper_width: 58|80, raster_id, width_dots: 384|576,
height_dots, data_uri (solo GET)}]}`. `establishment` es de solo lectura (Configuración general) y nunca se duplica.
`rasters[].data_uri` es un PNG de 1 bit con exactamente los puntos que se imprimen. Un GET con un original de 1 MiB pesa
≈ 1,4 MB (límite del puente: 2 MiB).

### `LogoPreview` (`POST documents/identity/logo/preview`)

`{"presentation": {…}, "rasters": [{"paper_width": 58, "raster_id": "uuid (solo si ya estaba guardada)", "width_dots":
384, "height_dots": 154, "data_uri": "…"}, {…80}]}`. Usa la conversión guardada si la combinación existe; si no, convierte
en memoria y no guarda nada.

### SSE, auditoría y puente

- `documents.identity.changed` (ámbito `documents`, sin agregado ni contenido) en la misma transacción de cada escritura
  con cambios; lo reciben quienes tienen `documents.templates.read`.
- Auditoría (sin bytes ni base64): `documents.identity_updated` (entidad `document_identity`; antes y después con
  `additional_info`, `header_text`, `footer_text`, `presentation`, `logo_id` y `revision`), `documents.logo_uploaded`
  (entidad `document_logo`; después `{logo_id, media_type, width, height, size_bytes, sha256, rasters, replaced_logo_id,
revision}`) y `documents.logo_removed` (`{logo_id, revision}`).
- Cinco operaciones fijas nuevas en `api.rs` (`documents_identity`, `documents_save_identity`,
  `documents_upload_identity_logo`, `documents_remove_identity_logo`, `documents_preview_identity_logo`) con la prueba
  `document_identity_routes_are_closed_and_keep_write_methods`; sin parámetros de consulta.

### Exportaciones de reportes (`GET reports/export/{id}`)

Conserva parámetros, permisos, límites, cabeceras, nombre de archivo y auditoría de la 9A; cambia solo el encabezado:

- **PDF**: la primera página empieza con el logotipo (a lo más 50 × 14 mm) y, a su derecha, el nombre, la dirección y
  «Tel. {teléfono} · {correo}» (solo los datos no vacíos de `app.establishment`) y una línea divisoria. Una sola imagen por
  documento: PNG RGB o JPEG (`DCTDecode`) de a lo más 600 × 168 px.
- **XLSX**, hoja «Filtros»: logotipo en A1 (`xl/drawings/drawing1.xml`, `oneCellAnchor`, ≤ 200 × 56 px, sin hipervínculo;
  `xl/media/image1.png` o `image1.jpeg`) y el nombre en B1; renglón 2 vacío; la tabla «Concepto | Valor» desde el renglón
  3 con «Establecimiento» y, si existen, «Dirección», «Teléfono» y «Correo electrónico». La hoja «Datos» es idéntica a la de
  la 9A. Sin fórmulas, hipervínculos, nombres definidos, vínculos externos ni macros.
- Logotipo: el del restaurante si la fuente (`identity.ReportLogo`, un `SELECT` en la transacción de solo lectura del
  reporte, sin locks) devuelve uno válido; si no, el de AD-EL POS (`assets/brand/adel-pos-tema-claro.png`).

## Etapa 9B · editor visual (fase C, sin cambios de contrato)

La fase C no agrega rutas, permisos, migraciones ni operaciones del puente: usa las de las fases A y B y la de cocina
existente `kitchen_job` (seguimiento del trabajo de una muestra, con `kitchen.queue.read` + `kitchen.stations.all`).

- `services/documents.ts` agrega `documentPermissions` (las seis claves `documents.*`) y el evento de la aplicación
  `templatesChangeEvent = 'adel-document-templates-change'` con `detail: { templates: string[] }`; `hooks/useRealtime.ts`
  lo publica al recibir `documents.templates.changed` (un reinicio del cursor lo publica sin identificadores) y publica
  `adel-documents-identity-change` con `documents.identity.changed`. Los eventos solo invalidan.
- Pantalla nueva `documents` en `screens` (`documents.templates.read`, `home: false`).
- Vista previa con el borrador local, antirrebote de 250 ms y una sola petición en vuelo; `422 TEMPLATE_INVALID` con
  `details.block_id` marca el bloque. `409 REVISION_CONFLICT` → `GET documents/templates/{id}` y diálogo de opciones;
  «guardar mi borrador» repite el `PUT` con la revisión leída. El `operation_id` de cada acción se conserva mientras su
  resultado sea incierto (sin conexión o error ≥ 500) y el cuerpo no cambie.

## Etapa 9C · inicio por terminal, columnas por impresora, reportes nuevos y puente del menú (esquema 20)

Migración `0020_terminal_home_and_printer_columns.sql` (aplicada en la instalación real el 11 de noviembre de 2026). Sin rutas de Go nuevas
salvo `POST kitchen/printers/{id}/width-test`, sin permisos nuevos y sin cambios de reglas de negocio: los demás cambios
son campos opcionales, columnas de reportes y mensajes. Detalle en [docs/ETAPA_9C_AVANCE.md](docs/ETAPA_9C_AVANCE.md).

### Inicio por terminal (`auth/me` y `devices`)

- `GET auth/me`: `terminal` agrega `home_screen` (`tables` | `takeaway` | `kitchen` | `cash`; `tables` en todas las
  terminales existentes al aplicar la 0020).
- `GET devices` (`devices.read`): cada terminal agrega `home_screen`.
- `PATCH devices/{id}` (`devices.manage`, sin cambio): campo **opcional** `home_screen` en el cuerpo
  `{name, register_id, keyboard_mode, home_screen?}`. Sin el campo (o con `null`) se conserva el guardado, de modo que un
  cliente anterior sigue editando la terminal sin cambiarlo. Otro valor → `400 VALIDATION_ERROR` «Elige la pantalla
  inicial de la terminal: Mesas, Para llevar, Cocina o Caja.». Defensa en la base: `devices_home_screen_check`. Se audita
  en `device.updated` con el antes y el después (que incluyen `home_screen`).
- `GET/PUT preferences` y `users/{id}/preferences` no cambian de forma (`home_screen` del usuario se conserva, pero la
  aplicación ya no lo usa para abrir; el respaldo de la apariencia se describe abajo).

### Apariencia de usuarios nuevos

`POST users` (sin cambio de cuerpo ni permisos): Go copia la apariencia de Configuración general (el campo `theme` de
`/settings`, `app.establishment.theme`) a las
preferencias del usuario nuevo; cambiarla después no altera a los existentes. Un usuario sin preferencias guardadas recibe
esa apariencia como respaldo en `GET preferences` y `GET auth/me` (antes, `light`).

### Impresoras de Cocina: `columns` y prueba de ancho

`GET kitchen/printers`, `POST kitchen/printers` y `PATCH kitchen/printers/{id}` (`kitchen.printers.manage`, sin cambio)
llevan un campo nuevo:

- `columns: number | null`: `null` = predeterminado del papel (42 en 80 mm, 30 en 58 mm). Lista cerrada: 80 mm → 42 o
  48; 58 mm → 30 o 32. Elegir el predeterminado se guarda como `null`. Otro valor, o uno que no corresponde al papel →
  `400 KITCHEN_VALIDATION` («Con papel de 58 mm las columnas de texto son 30 (predeterminado) o 32; 48 no es una opción.
  Si cambiaste el papel, elige de nuevo las columnas.»). Defensa en la base: `kitchen_printers_columns_check`. Queda en el
  historial `printer.saved` como cualquier cambio de impresora.
- La fotografía de la impresora en cada trabajo (`printer_snapshot`) lleva `columns`; los trabajos anteriores a la 9C no
  lo tienen y usan el predeterminado. La reimpresión de una comanda vuelve a renderizar con esa fotografía; precuentas,
  tickets y documentos de caja copian su contenido. «Imprimir con la plantilla vigente» usa la configuración actual de la
  impresora del trabajo original.
- CUPS en texto: con el predeterminado los argumentos de `lp` no cambian (`cpi` 12, o 6 con el texto grande de Cocina);
  con otras columnas, `cpi` = 12 × columnas ÷ predeterminado, con dos decimales (13.71 para 48, 12.8 para 32; 6.86 y 6.4
  con el texto grande). ESC/POS por red y CUPS raw no cambian.

`POST kitchen/printers/{id}/width-test` (`kitchen.printers.manage`; operación del puente `kitchen_printer_width_test`):

```json
{ "operation_id": "uuid", "columns": 48 }
```

- `columns` opcional: las del formulario aunque aún no se hayan guardado; sin ellas, las de la impresora. Fuera de la
  lista → `400 KITCHEN_VALIDATION` con el mensaje anterior.
- Respuesta: `{"job": PrintJob, "ticket_id": "uuid", "paper_width": 80, "columns": 48}`.
- Encola por la cola de siempre un ticket `test` (con `purpose: "width_test"` en su documento) con la regla numerada del
  ancho elegido, una línea completa de guiones y el renglón «Producto de ejemplo … $1,234.50», entre las marcas
  «*** PRUEBA DE ANCHO ***». El trabajo lleva la fotografía de la impresora con las columnas probadas. **No crea ventas,
  pagos ni movimientos de caja o inventario.**
- Como toda escritura de cocina: lock 7820148305 exclusivo, idempotencia por `operation_id` en `restaurant_operations`,
  historial `print.width_test_requested` (auditoría `kitchen.print.width_test_requested`) y evento
  `kitchen.printers.changed` en el ámbito `printing`. Una copia desde la cola repite el mismo texto; su vista en la cola
  exige `kitchen.stations.all`.

### Diseñador: vista previa por impresora

- `POST documents/preview` acepta `printer_id` (opcional): una impresora **activa**; `paper_width` puede omitirse o debe
  coincidir con su papel (si no, `400 DOCUMENTS_VALIDATION`: «Elige una impresora activa para la vista previa.» o «El
  ancho de papel no corresponde a la impresora elegida.»). Sin `printer_id`, como antes (58 u 80 mm con sus columnas
  predeterminadas). La respuesta `Preview` agrega `printer_id` y `printer_name` (omitidos sin impresora); `columns` son
  las columnas con que se renderizó (la mitad con el texto grande de Cocina).
- `GET documents/printers`: `PrinterCompatibility.columns` son ahora las columnas **efectivas** de la impresora y se
  agrega `default_columns` (las del papel).
- Las muestras (`POST documents/samples`) salen con las columnas de la impresora elegida.

### Reportes nuevos y columnas nuevas (`reports/run`, `reports/export`)

Registro cerrado de 92 a **94 reportes**, ambos en `EXPORTABLE_REPORTS` de `export.rs`; sin filtros nuevos (la lista
blanca `reports_*` de `api.rs` no cambió). Permisos: `reports.read` + `reports.sales.read` (como «Productos cancelados»).
Ninguno tiene columna «Autorizó»: AD-EL POS no pide la autorización de otra persona para anular una cuenta ni para
reducir o cancelar cantidades (desde la 9D «Cuentas canceladas» la tiene, y desde la 9F también «Modificaciones de
cantidades» y «Productos cancelados»).

| Reporte                                                                      | Fecha                                                                   | Filtros                                                                                                                          | Columnas                                                                                                                                                                                                                                                                                                                                              |
| ---------------------------------------------------------------------------- | ----------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `sales.voided_accounts` «Cuentas anuladas» (grupo Ventas)                    | Anulación de la cuenta, con la hora de inicio del día                   | `kind` (`void` «Anuladas con motivo», `released` «Cuentas vacías liberadas»), `mode`, `area_id`, `user_id` («Usuario que anuló») | `voided_at`, `folio`, `kind` (Anulada / Cuenta vacía liberada), `voided_by`, `terminal`, `reason`, `mode`, `place` (mesa u origen), `area`, `cancelled_units` (suma), `cancelled_amount` (suma); opcionales `account`, `opened_at`, `opened_by`. Orden por omisión: `voided_at`.                                                                      |
| `products.quantity_changes` «Modificaciones de cantidades» (grupo Productos) | Cancelación o modificación de la partida, con la hora de inicio del día | `product_id`, `mode`, `user_id` («Usuario que modificó»)                                                                         | `changed_at`, `folio`, `product`, `change` (Cantidad reducida / Partida cancelada), `previous_quantity` y `new_quantity` (sin total), `difference` (unidades, negativa, suma), `amount_difference` (importe, negativo, suma), `changed_by`, `reason`; opcionales `action`, `terminal`, `mode`, `place`, `released_units` (suma). Orden: `changed_at`. |

- Desde la etapa 9D, `sales.voided_accounts` se llama «Cuentas canceladas» y cambió de columnas y filtros (sin `kind`); ver «Etapa 9D · cancelar cuenta…». Lo que sigue describe la versión de la 9C.
- «Cuentas anuladas» parte de las cuentas `voided` y lee su evento `account.void` o `account.released`; el importe es el
  de las unidades canceladas en la cuenta antes de anularla (una cuenta solo se anula sin productos activos y con cobros
  netos en cero); mesa y área son las actuales.
- «Modificaciones de cantidades» parte de las unidades canceladas del periodo y lee el evento `item.cancelled` o
  `item.modified` (con `cancelled_unit_ids`) que las retiró; si falta la cantidad anterior, queda vacía (no se inventa).
- Columnas opcionales nuevas (ocultas por omisión): `payments.refunds` → `reference` («Referencia», de la devolución) y
  `original_reference` («Referencia del pago original»); `payments.corrections` → `original_reference` («Referencia
  original») y `new_reference` («Referencia correcta»). `payments.list` ya tenía `reference`; `purchases.received` ya
  tenía «Recibió».

### Catálogo: imágenes JPEG

`POST catalog/images` (`catalog.images.manage`, sin cambio): un archivo que empieza con `FF D8` se recorre por marcadores
**antes** de leer el encabezado o decodificar. Más de 100 barridos o 1024 segmentos → `400 CATALOG_VALIDATION` («La imagen
JPEG tiene demasiados barridos progresivos (máximo 100)…» o «…demasiados segmentos (máximo 1024)…»). El máximo exacto se
acepta.

### Mensajes que cambiaron (solo texto; códigos sin cambio)

- `409 IDEMPOTENCY_CONFLICT` en `payments`, `operations`, `kitchen`, `inventory`, `designer` e `identity`: «Esta operación
  ya se había enviado con otros datos y no se repitió. Consulta el resultado antes de intentarlo de nuevo.».
- `POST setup` con zona horaria inválida: «Elige la zona horaria del restaurante de la lista.».
- `GET permissions`: la descripción de `audit.read` es «Consultar el registro de movimientos» (la columna
  `app.permissions.description` de la base no cambia); el mensaje de solo consulta de `audit` es «El registro de
  movimientos es de solo consulta.».

### Puente nativo

- `api.rs`: operación nueva `kitchen_printer_width_test` (`POST`, con identificador UUID), con la prueba
  `kitchen_printer_width_test_is_a_closed_post_with_identifier` (rechaza rutas y valores que no son UUID).
- Operaciones nativas nuevas (`src-tauri/src/menu.rs`, permiso `allow-diagnostics`, registradas en `build.rs`,
  `permissions/diagnostics.toml` y `generate_handler!`; sin permisos `core:*`):

| Operación        | Argumentos                                                           | Resultado                                                                  | Errores                                                                                                                                                |
| ---------------- | -------------------------------------------------------------------- | -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `menu_subscribe` | `channel: Channel<{ action: string }>`                               | `u64` (identificador de la suscripción; una nueva sustituye a la anterior) | «No se pudo preparar el menú.»                                                                                                                         |
| `menu_set_state` | `enabled: string[]` (identificadores de `MENU_ACTIONS`, a lo más 23) | `()`; las demás opciones propias se deshabilitan                           | «Opción de menú no reconocida.» (fuera de la lista, incluido `file.quit`), «Estado del menú no válido.» (demasiadas), «No se pudo actualizar el menú.» |

- Rust envía `{ "action": "<id>" }` por el canal al elegir una opción propia. Los elementos predefinidos (deshacer,
  copiar…, ocultar, «Salir de AD-EL POS») los atiende el sistema; «Salir» de Archivo (`file.quit`) lo atiende Rust con
  `app.exit(0)` y nunca llega a React. Antes del primer `menu_set_state` solo están habilitadas «Pantalla completa»,
  «Atajos de teclado» y «Acerca de».
- `e2e/native-driver.ts` simula ambas operaciones leyendo `MENU_ACTIONS` de `menu.rs` y ofrece `clickMenu(id)` y
  `menuState()` a las pruebas.

## Etapa 9D · cancelar cuenta, descartar cuentas vacías, motivos, umbral de cuenta abierta y reportes (esquema 21)

Migración `0021_account_cancellation.sql` (aplicada a la instalación real el 12/11/2026): tablas
`account_cancel_reasons` y `account_cancellations`, columna `restaurant_settings.long_open_hours` y permiso
`accounts.cancel` («Cancelar cuentas», solo al administrador). Las escrituras de `restaurant/*` siguen las reglas del
esquema 4: `operation_id` UUID, `revision`, resultado idempotente, auditoría y eventos en la misma transacción, lock
7820148305 exclusivo. Errores con la forma de siempre (`{"error": {"code", "message", "details?"}}`). Detalle en
[docs/ETAPA_9D_AVANCE.md](docs/ETAPA_9D_AVANCE.md) y en [docs/RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md).

### Cancelar cuenta (`POST restaurant/accounts/{id}/cancel-account`)

Operación del puente `restaurant_cancel_account`. Permiso de la ruta: `accounts.read` + `accounts.open` (para llevar,
además `takeaway.manage`). La ruta no exige `accounts.cancel`: con ese permiso basta el propio; sin él, procede con la
autorización puntual de un supervisor que lo tenga, con el mismo mecanismo de los descuentos (sin sesión, con el límite
de intentos del inicio de sesión y sin autoautorización).

```json
{
  "operation_id": "uuid",
  "revision": 7,
  "reason_id": "uuid del catálogo",
  "description": "obligatoria (5 a 500 caracteres) si el motivo la exige; opcional hasta 500",
  "authorization": { "username": "supervisor", "password": "…" }
}
```

- `authorization` solo si Go la pide (`account.cancel_requires_authorization` o el error `AUTHORIZATION_REQUIRED`); se
  ignora si el usuario tiene `accounts.cancel`. La contraseña **no entra** en la huella idempotente, la respuesta
  guardada, los eventos ni la auditoría: un reintento incierto se reenvía con el mismo `operation_id` y
  `authorization: {"username"}` sin contraseña para recuperar el resultado ya confirmado (si no llegó a confirmarse,
  vuelve `AUTHORIZATION_REQUIRED` y hay que autorizar de nuevo).
- **Orden de comprobación**, todo antes de escribir: permiso de la ruta, cuenta abierta y `revision` vigente, cuenta
  descartable (`ACCOUNT_DISCARDABLE`, antes del motivo y de la contraseña: no cuenta en el límite de intentos), motivo,
  guardas financieras (`ACCOUNT_HAS_PAYMENTS`, `ACCOUNT_HAS_ADJUSTMENTS`) y de unidades, autorización.
- **Efectos** (misma transacción): borrador descartado; todas las unidades activas canceladas con **un solo evento**
  `account.cancelled`; cuenta en `state: "voided"` con su fila en `account_cancellations` (motivo, nombre y marca de
  merma congelados, descripción, quién canceló, quién autorizó —`NULL` si bastó el permiso propio—, terminal, operación,
  unidades e importe activos al cancelar); mesa libre; auditoría `restaurant.account.cancelled` (y
  `restaurant.supervisor_authorized` con `scope: "account_cancel"`, el supervisor y el importe, sin contraseña); evento
  en tiempo real `restaurant.account.cancelled`. Nunca es venta: la cuenta no se cierra ni se reconoce en
  `sales_recognitions`.
- **Respuesta 200**: el `Bill` de siempre (cuenta en `voided`, `draft: []`, renglones en `cancelled`) **leído después de
  integrar cocina**: con una estación inactiva dice `preparation_state: "change_pending"`, igual que una lectura posterior
  y que un reintento con el mismo `operation_id`. Incluye `cancellation`:

```json
{
  "cancellation": {
    "reason_id": "uuid",
    "reason_name": "Cliente se retiró sin pagar",
    "description": "Mesa de la ventana",
    "prepared_is_waste": true,
    "cancelled_by_name": "Administrador",
    "authorized_by_name": null,
    "cancelled_units": 3,
    "cancelled_amount": "115.00",
    "created_at": "2026-11-12T16:00:00Z"
  }
}
```

`GET restaurant/accounts/{id}` devuelve el mismo `cancellation` en una cuenta cancelada y `null` en cualquier otra
(también las anuladas antes de la 9D, las liberadas y las descartadas).

| Estado | Código                        | Cuándo (nada se escribe)                                                                                 |
| ------ | ----------------------------- | -------------------------------------------------------------------------------------------------------- |
| 400    | `OPERATION_VALIDATION`        | Sin `reason_id`, identificador inválido, campos desconocidos o cuenta que ya no está abierta.            |
| 404    | `NOT_FOUND`                   | Motivo inexistente.                                                                                      |
| 409    | `ACCOUNT_CONFLICT`            | `revision` vencida.                                                                                      |
| 409    | `ACCOUNT_DISCARDABLE`         | La cuenta es `discardable`: «Esta cuenta no tiene productos enviados: descártala; no hace falta motivo.» |
| 409    | `CANCEL_REASON_INACTIVE`      | Motivo desactivado.                                                                                      |
| 422    | `CANCEL_DESCRIPTION_REQUIRED` | El motivo («Otro») exige una descripción de 5 a 500 caracteres.                                          |
| 422    | `CANCEL_DESCRIPTION_TOO_LONG` | Descripción de más de 500 caracteres.                                                                    |
| 409    | `ACCOUNT_HAS_PAYMENTS`        | Pagos o propinas netos: primero la devolución (etapa 6).                                                 |
| 409    | `ACCOUNT_HAS_ADJUSTMENTS`     | Descuento, cortesía o cargo vigente: primero quitarlo desde los ajustes.                                 |
| 403    | `AUTHORIZATION_REQUIRED`      | Sin `accounts.cancel` y sin autorización.                                                                |
| 403    | `SELF_AUTHORIZATION`          | El supervisor es el mismo usuario.                                                                       |
| 401    | `AUTHORIZATION_INVALID`       | Usuario o contraseña del supervisor incorrectos (cuenta en el límite de intentos).                       |
| 429    | `AUTHORIZATION_THROTTLED`     | Demasiados intentos.                                                                                     |
| 403    | `AUTHORIZATION_INSUFFICIENT`  | El supervisor no tiene `accounts.cancel`.                                                                |
| 409    | `IDEMPOTENCY_CONFLICT`        | Mismo `operation_id` con otro contenido.                                                                 |

Los cinco códigos de autorización son los de los descuentos (`useSupervisorAuthorization.handleError`).

### Descartar cuenta vacía (`POST restaurant/accounts/{id}/discard`)

Operación del puente `restaurant_discard_account`. Permiso `accounts.open` (para llevar, `takeaway.manage`); cuerpo
`{"operation_id", "revision"}`, **sin motivo ni autorización**. Procede con una cuenta abierta sin unidades en ningún
estado, sin pedidos confirmados en ella, sin haber recibido una combinación y sin registros financieros ni de cocina,
aunque tenga un borrador sin enviar, que se borra. Respuesta 200: `Bill` en `voided`, `draft: []`, `cancellation: null`.
Evento `account.discarded` (auditoría y tiempo real `restaurant.account.discarded`), sin fila en
`account_cancellations`: **no es una cancelación** y no aparece en «Cuentas canceladas» ni en otro reporte. Errores: 409
`ACCOUNT_NOT_EMPTY` (productos enviados, pagos, ajustes, documentos o registros de cocina: usar «Cancelar cuenta»), 409
`ACCOUNT_CONFLICT`, 400 si no está abierta. «Liberar mesa» (`POST …/release`) no cambia de regla; su mensaje de
`ACCOUNT_NOT_EMPTY` ahora sugiere descartar o cancelar.

### Creación con el primer producto también para llevar y sin mesa (sin cambio de contrato)

Desde la fase D de la 9D la aplicación ya no abre cuentas vacías: «Nuevo pedido» (Para llevar), «Cuenta sin mesa» y
«Nuevo pedido para llevar» llaman a `POST restaurant/accounts` (puente `restaurant_open_account`) solo al guardarse el
primer producto, con `{operation_id, mode, table_id: null, seen_account_ids: [], items}`, la apertura idempotente con
`items` de la etapa 7.1 que Go ya admitía en cualquier modalidad (comprobado en `TestStage9DAccountCancellation`,
«creación con el primer producto sin mesa y para llevar»). Sin `table_id` no se comprueba la mesa. El contrato no
cambia; las integraciones pueden seguir abriendo una cuenta sin `items`.

### Campos nuevos de lectura

En cada `Account` (listado `restaurant/accounts`, detalle y respuestas de escritura), calculados por Go en cada lectura:

| Campo                           | Tipo                                    | Significado                                                                                                                                             |
| ------------------------------- | --------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `discardable`                   | boolean                                 | «Descartar» procede (abierta, sin unidades en ningún estado, pedidos, combinaciones ni registros financieros o de cocina, aunque tenga borrador).       |
| `releasable`                    | boolean                                 | (7.1) Desde la 9D, `discardable` y sin borrador.                                                                                                        |
| `cancel_blocker`                | `""` \| `"payments"` \| `"adjustments"` | Solo en abiertas no descartables: qué impide «Cancelar cuenta» (primero la devolución o primero quitar el ajuste); `""` si procede o si es descartable. |
| `cancel_requires_authorization` | boolean                                 | El usuario que lee no tiene `accounts.cancel`: pedir supervisor desde el principio (`false` en las descartables).                                       |
| `long_open`                     | boolean                                 | Abierta hace más de `long_open_hours` horas.                                                                                                            |

En cada `RestaurantTable` (`overview`, `tables`, respuestas de mesa): `long_open` (la cuenta abierta más antigua supera el
umbral). En `RestaurantSettings` (con el mapa, el listado de cuentas y `GET restaurant/settings`): `long_open_hours` (1–72,
12 por omisión). Regla única en Go: `elapsed_seconds > long_open_hours × 3600` (estricto). `PUT restaurant/settings`
ignora `long_open_hours` aunque se envíe. En `Bill`: `cancellation` (arriba).

### Motivos de cancelación (`restaurant/cancel-reasons`)

- `GET restaurant/cancel-reasons` (`accounts.read`; puente `restaurant_cancel_reasons`): todos, activos e inactivos, en su
  orden. «Cancelar cuenta» ofrece solo los activos.
- `POST restaurant/cancel-reasons` (`payments.settings.manage`, el permiso de los motivos de descuento; puente
  `restaurant_create_cancel_reason`) y `PATCH restaurant/cancel-reasons/{id}` (mismo permiso; puente
  `restaurant_save_cancel_reason`).

```json
{
  "id": "uuid",
  "name": "Otro",
  "requires_description": true,
  "prepared_is_waste": false,
  "active": true,
  "sort_order": 40,
  "revision": 1,
  "updated_at": "…"
}
```

Cuerpo de alta o cambio: `{operation_id, revision, name, requires_description, prepared_is_waste, active, position:
{after: uuid|null}}`. `name` de 1 a 60 caracteres sin saltos de línea ni caracteres de control. Errores: 400 nombre
inválido, 409 `DUPLICATE_NAME`, 409 `REVISION_CONFLICT`, 409 `LAST_ACTIVE_REASON` (debe quedar uno activo), 404.
Auditoría `restaurant.cancel_reason_created`, `restaurant.cancel_reason_updated` y `restaurant.cancel_reasons_reordered`.
Renombrar o cambiar las marcas no altera lo registrado; colocar un motivo renumera a sus hermanos (sube su `revision`):
releer la lista después de guardar. Semilla de la 0021: «Cliente se retiró sin pagar» (`prepared_is_waste`), «Cliente
canceló el pedido», «Error de captura» y «Otro» (`requires_description`).

### Umbral de cuenta abierta (`settings/long-open-hours`)

- `GET settings/long-open-hours` (cualquier sesión; puente `settings_long_open_hours`):
  `{"long_open_hours": 12, "minimum_hours": 1, "maximum_hours": 72, "revision": 3}`.
- `PUT settings/long-open-hours` (`settings.features.manage`, el de «Funciones del establecimiento»; puente
  `save_settings_long_open_hours`), sin `operation_id`: `{"long_open_hours": 6, "revision": 3}`. Errores: 422
  `LONG_OPEN_HOURS_INVALID` (fuera de 1–72), 409 `REVISION_CONFLICT`, 400 cuerpo inválido. Repetir el valor no escribe.
  Audita `settings.long_open_hours_changed` y publica `restaurant.settings.changed` en los ámbitos `restaurant` y
  `takeaway`. `revision` es la de `restaurant_settings`, compartida con «Advertencias del tablero para llevar».

### Eventos de cuenta e historial

- `account.cancelled`: `before: {account, lines, draft}`; `after: {account, reason_id, reason, description,
prepared_is_waste, unit_ids, cancelled_units, cancelled_amount, authorized_by, authorized_by_username,
authorized_by_name, discarded_draft_items}`. `after.account` es la cuenta antes de integrar cocina; `unit_ids` tiene la
  misma forma que `item.cancelled`, de modo que cocina, inventario y los reportes de productos cancelados lo tratan igual.
  `authorized_by_name` es el nombre visible del supervisor (`null` si bastó el permiso propio).
- `account.discarded`: `before: {account, draft}`; `after: {account}`.
- Etiquetas del historial: «Cuenta cancelada» y «Cuenta vacía descartada». Acciones de auditoría nuevas:
  `restaurant.account.cancelled`, `restaurant.account.discarded`, `restaurant.supervisor_authorized` (alcance
  `account_cancel`), las tres del catálogo de motivos, `settings.long_open_hours_changed` e
  `inventory.cancellation_preclassified`. El Registro de movimientos muestra su nombre legible; el filtro sigue por código.

### Inventario: preclasificación (`GET inventory/cancellations` y la traza de consumo)

`Cancellation` agrega `preclassified` (boolean): una clasificación «Preparado / merma» que el trabajador creó al cancelar
una cuenta con un motivo que marca merma y que nadie ha corregido (resolución `manual` en su primera revisión, cuya
solicitud viene de un `account.cancelled`; sin migración ni cambio a la restricción de la etapa 8). Con `preclassified`,
`resolution_label` es «Preclasificada al cancelar la cuenta» y `classified_by_name` es quien canceló. En cuanto el
encargado la cambia (aunque vuelva a «Preparado / merma»), `preclassified` es `false` y la etiqueta vuelve a «Clasificada
por el encargado». `GET inventory/consumption/{unit_id}` recibe la misma etiqueta.

### Reportes (`reports/run`, `reports/export`)

Permisos: `reports.read` + `reports.sales.read`. Registro de 94 a **95 reportes**, todos en `EXPORTABLE_REPORTS`.

| Reporte                                                            | Filtros                                                                                                                                       | Columnas                                                                                                                                                                                                                                                                                              |
| ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `sales.voided_accounts` «Cuentas canceladas» (se conserva el id)   | `reason_id` (fuente `cancel_reasons`; `none` = anulaciones anteriores con motivo libre), `mode`, `area_id`, `user_id` («Usuario que canceló») | `cancelled_at` «Cancelación», `folio`, `reason`, `description`, `cancelled_amount` «Importe», `cancelled_units`, `cancelled_by` «Canceló», `authorized_by` «Autorizó», `terminal`, `mode`, `place`, `area`; opcionales `kind` «Registro», `account`, `opened_at`, `opened_by`. Orden: `cancelled_at`. |
| `sales.cancellations_by_reason` «Cancelaciones por motivo» (nuevo) | `mode`, `area_id`, `user_id`                                                                                                                  | `reason`, `accounts`, `cancelled_units`, `cancelled_amount`, `share` «% del importe» (totales y porcentajes de Go sobre todo el conjunto). Orden: importe descendente.                                                                                                                                |

- «Cuentas canceladas» parte de `account_cancellations` (por su fecha) y de las anulaciones anteriores (cuentas `voided`
  sin registro cuyo último evento terminal es `account.void`); las liberadas (`account.released`) y las descartadas
  (`account.discarded`) quedan fuera. El filtro `kind` de la 9C se retiró y responde 400. «Autorizó» está vacía si bastó
  el permiso propio y en las anulaciones anteriores.
- «Cancelaciones por motivo» agrupa por el nombre congelado; las anulaciones anteriores se agrupan como «Cancelaciones
  anteriores con motivo libre». Suma las unidades e importe **activos al cancelar**; lo cancelado antes una por una está
  en «Productos cancelados».
- Fuente nueva de filtros `GET reports/filters/cancel_reasons` (operación `reports_filters` con `cancel_reasons`):
  «Sin motivo del catálogo» (`none`) y los motivos, con «Inactivo» en su detalle.
- `products.cancelled`, `products.quantity_changes` (un renglón por partida con la acción «Cancelación de la cuenta» y el
  motivo «Cuenta cancelada: …») y `employees.activity` (unidades canceladas y autorizaciones de supervisor) incluyen las
  cuentas canceladas.

### Mensajes que cambiaron (solo texto; códigos sin cambio)

«Anular» → «Cancelar» en la descripción de `accounts.manage` («Editar datos de cuentas y cancelar cuentas vacías con
motivo libre», también en la base por la 0021), en los mensajes de la anulación anterior, de «Liberar mesa» y de la
guarda financiera.

### Puente nativo (`desktop/src-tauri/src/api.rs`)

Siete operaciones nuevas en la lista cerrada, con la prueba `stage9d_account_cancellation_routes_are_closed_and_keep_write_methods`:
`restaurant_cancel_account` (POST, con identificador), `restaurant_discard_account` (POST, con identificador),
`restaurant_cancel_reasons` (GET), `restaurant_create_cancel_reason` (POST), `restaurant_save_cancel_reason` (PATCH, con
identificador), `settings_long_open_hours` (GET) y `save_settings_long_open_hours` (PUT). **`restaurant_void` se retiró**
de la lista (la prueba exige que no tenga ruta) y del tipo de operaciones de `services/api.ts`; Go conservó
`POST restaurant/accounts/{id}/void` por compatibilidad hasta la etapa 9F, que la retiró (`410 ROUTE_RETIRED`). `export.rs` agrega `sales.cancellations_by_reason` a
`EXPORTABLE_REPORTS`. Sin operaciones nativas nuevas.

## Etapa 9E · Windows (sin rutas nuevas; esquema 21)

Sin migraciones, rutas, permisos ni operaciones nuevas del puente (`api.rs` solo cambia el mensaje de conexión, que en
la instalación de Windows dice que el equipo se está preparando mientras arranca el servidor local). Lo que cambia
depende del sistema donde corre Go; en macOS los contratos siguen idénticos.

### Cola de impresión del equipo en Windows (transporte `cups`)

El valor guardado `cups` significa «cola de impresión del equipo servidor». Con Go en Windows es la impresora
instalada en Windows y recibe **ESC/POS en modo RAW** por el administrador de impresión (`winspool.drv`:
`OpenPrinter`, `StartDocPrinter` con tipo `RAW`, `WritePrinter`, `EndDocPrinter`; `AbortPrinter` si falla). En macOS
sigue siendo CUPS.

- `GET kitchen/discovery` lista las impresoras instaladas en Windows (`EnumPrinters`) con la misma forma
  `{queue, status, options}`; `status` resume en español el puerto, el controlador y si AD-EL POS puede usarla.
  Si Windows no responde: `503 SPOOLER_UNAVAILABLE` («No se pudieron consultar las impresoras instaladas en Windows en
  el equipo servidor.»). En macOS sigue `503 CUPS_UNAVAILABLE`.
- Antes de cada envío la impresora se consulta (solo lectura). Se aceptan puertos USB, COM, LPT y TCP/IP estándar
  (`connection`: `usb`, `serial`, `parallel`, `socket`). Se rechazan, sin enviar nada y con el motivo, las impresoras
  de otro equipo, de red compartida, de Escritorio remoto o de la nube, las virtuales (PDF, XPS, OneNote, fax),
  `FILE:`, `PORTPROMPT:`, `NUL:`, los puertos WSD y los monitores desconocidos.
- Los bytes son los de ESC/POS por red: texto con el codificador de siempre, documentos con formato o logotipo con
  `documents.EncodeESCPos` y el pulso del cajón con los mismos bytes. La marca `accepts_escpos` no aplica en Windows
  (la interfaz la oculta y no hay camino de texto: una impresora que no recibe los datos sin alterar no recibe
  documentos).
- Resultados de la cola sin cambio de reglas: `error` si no se envió nada; `uncertain` (nunca se reintenta solo) si
  falló después de empezar el documento; `submitted` con «Aceptado por la cola de impresión de Windows; no confirma
  salida física de papel.».
- `GET documents/printers`: con la cola de Windows, `format`/`logo` usan `queue_raw` (`supported`), `queue_not_raw`
  (`unsupported`, «La impresora no recibe los comandos ESC/POS sin alterar: AD-EL POS no le envía documentos. …»),
  `queue_unavailable` y `queue_timeout` («Windows no respondió a tiempo: …»); `escpos_disabled` no se usa.
- `GET payments/cash/drawers`: `printers[].connection` y `reason` salen de la misma consulta; una impresora desactivada
  dice «Impresora desactivada; no se consultó en Windows.». `DRAWER_QUEUE_NOT_RAW` y `DRAWER_QUEUE_UNAVAILABLE` se
  conservan con los motivos de Windows.

### SSE (`GET events`) al apagar el servidor

Desde la 9E los flujos SSE terminan en cuanto el servidor empieza a apagarse (`Service.Stopping`), en lugar de esperar
a que agote los 5 s de `Shutdown`: el cierre con un flujo abierto termina en ≈ 1 s y con código 0. El cliente ve el
fin del flujo como siempre y reconecta con `Last-Event-ID` cuando el servidor vuelve.

### Comandos nuevos del servidor (línea de comandos, no HTTP)

| Comando                                                          | Sistema      | Qué hace                                                                                                                                                          |
| ---------------------------------------------------------------- | ------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `stop --pid N`                                                   | Windows      | Activa el evento de parada con nombre del servidor N tras comprobar que es el mismo programa (equivalente de `SIGTERM`). En macOS no hace nada.                   |
| `host --data-dir D --pg-bin B [--watch-stdin] [--log-file F]`    | solo Windows | Servidor de la instalación: bloqueo de la carpeta, clúster propio, rol, base, respaldo antes de migrar sobre datos, migraciones, `serve` y parada de PostgreSQL.  |
| `backup [--pause]`, `restore-check --file F`, `restore --file F` | solo Windows | Respaldo y ensayo con los `pg_dump`/`pg_restore` incluidos; `restore` exige la aplicación cerrada, un archivo de `backups` y crea antes un respaldo de seguridad. |
| `--log-file F` (cualquier comando)                               | ambos        | Bitácora JSON en archivo (rotación simple a 20 MiB) en lugar de la salida estándar.                                                                               |

## Etapa 9F · modo de pruebas y «Quitar datos de prueba» (fase A, sin migración)

### Modo de pruebas (`test_mode`)

- Configuración del servidor: campo opcional `test_mode` (booleano, sin valor = falso). Con `true`, la configuración
  solo es válida si `database.name` empieza por `adel_pos_test_` (y tiene algo después); si no, `config.Load` la
  rechaza con «el modo de pruebas solo se usa con una base de datos de pruebas…» y **ningún comando arranca**
  (`serve`, `migrate`, `prepare-setup`, `check-config` y los demás). Campos opcionales nuevos para el respaldo de
  mantenimiento: `postgres_bin` (carpeta de `pg_dump`/`pg_restore`) y `backup_directory`, ambos rutas absolutas.
- `GET /version` informa siempre `test_mode` (`false` en un servidor ordinario). `desktop/e2e/native-driver.ts` se
  niega a correr si no es `true` («Las pruebas solo corren contra un servidor en modo de pruebas»), y
  `scripts/browser-tests.py` y `scripts/windows/dev.mjs` (`test-browser`) escriben `test_mode: true` (y
  `backup_directory` dentro de su carpeta temporal) en la configuración temporal.
- Windows (`host`, solo línea de comandos): con la variable `ADEL_TEST_MODE=1` (heredada de la aplicación), `host`,
  `backup`, `restore-check` y `restore` se niegan a usar la carpeta de datos real del usuario (la de la API de
  carpetas conocidas `FOLDERID_LocalAppData` + «AD-EL POS», independiente de la variable `LOCALAPPDATA`), y `host`
  informa `test_mode: true` en `/version`. Cualquier otro valor no vacío de la variable es un error.

### Mantenimiento · «Quitar datos de prueba»

| Método | Ruta                            | Requisito                                                                                         |
| ------ | ------------------------------- | ------------------------------------------------------------------------------------------------- |
| GET    | `/maintenance/test-data`        | `settings.manage`; vista previa con locks compartidos 7820148305, 7820148304 y 7820148308.        |
| POST   | `/maintenance/test-data/remove` | `settings.manage`; `{operation_id, plan_id}`; respaldo previo y transacción con locks exclusivos. |

- Vista previa: `{plan_id, remove: Entry[], deactivate: Entry[], unlink: Entry[], keep: Entry[]}` con
  `Entry = {kind: printer|station|route|product|category|setting, id, name, reason}` (motivo en español, sin nombres
  técnicos). `plan_id` es el SHA-256 hexadecimal de `remove`, `deactivate` y `unlink`.
- Identificación por **firma exacta** de los archivos de prueba (lista cerrada en `backend/internal/maintenance/
signatures.go`, cada una con su archivo): impresoras «Destino controlado E2E» y «Destino controlado 9B» con transporte
  `agent` y destino «Agente de aceptación no disponible»; estaciones «Cocina caliente» y «Freidoras» **solo con una de
  esas impresoras**; producto «Filete de prueba» con código `FILETE` y tipo `product`; categoría «Platillos» con color
  `#d64545` **solo si contiene un producto de prueba**. Rutas: se elimina la ruta cuyas estaciones son todas de prueba
  (o la de un producto o categoría de prueba que se elimina); en las demás solo se quitan las estaciones de prueba. La
  ruta de un producto o una categoría **del restaurante** cuyas estaciones son todas de prueba también se elimina (una
  ruta sin estaciones enviaría a ningún lado), pero no es un dato de prueba: va en `unlink` con el motivo «Solo enviaba
  a estaciones de prueba («…»); se quita: …» (el producto usa la ruta de su categoría o, sin ella, la cocina avisa «Sin
  ruta de preparación»).
- Decisión: un dato sin registros relacionados se elimina; con registros (trabajos de impresión, tareas de cocina,
  acuses, avisos, ventas, cuentas abiertas, recetas, combos, **promociones del restaurante** o cualquier otra llave
  foránea que se agregue después) se **desactiva** (el producto además se archiva); ya inactivo y sin nada que
  desvincular no aparece. Un producto o una categoría de prueba que participa en una promoción se desactiva y la
  promoción no cambia (participantes, estado y revisión); el motivo la nombra («Participa en la promoción «…», que no
  cambia.»). Se **conserva** sin
  cambios la impresora de prueba que usa una estación del restaurante y la categoría con otros productos o con una
  ruta que incluye estaciones del restaurante. Se **desvincula** la configuración que apunta a lo de prueba: impresora
  de tickets de Cobros, cajón de cada caja (queda desactivado), impresora de respaldo de otras impresoras, estaciones
  por terminal y estaciones de prueba en rutas que se conservan. Una terminal cuya selección eran solo estaciones de
  prueba pasa a «Todas las estaciones» (`devices.kitchen_all_stations`, revisión independiente parte 3, B3) y se
  presenta una vez con el motivo «La terminal «…» mostraba solo estaciones de prueba; mostrará todas las estaciones.».
  Nunca toca ventas, pagos, cortes, movimientos de inventario, cuentas, documentos ni auditoría.
- Quitar: la transacción de autenticación solo autoriza, valida y comprueba la idempotencia (la misma operación del
  mismo actor y terminal devuelve la respuesta guardada sin otro respaldo; con otro `plan_id`, 409
  `IDEMPOTENCY_CONFLICT`). Después, una sola eliminación a la vez en el servidor: un reenvío de la misma operación
  mientras la primera sigue en curso espera su turno y, con los locks compartidos y **antes** de comparar el plan y de
  respaldar, vuelve a buscar la respuesta guardada (no crea otro respaldo ni responde `TEST_DATA_CHANGED` si la primera
  sí quitó). Si el plan vigente no es el revisado: 409 `TEST_DATA_CHANGED` sin quitar nada; si no
  hay nada que quitar, responde sin respaldo. Si no: **respaldo previo** (macOS y desarrollo: `pg_dump --format=custom
--no-owner --no-acl` con la conexión de la configuración y la contraseña solo en el entorno del proceso hijo,
  `<base>-<AAAAMMDDTHHMMSSZ>-<6 hex>.dump` con permisos 0600 en `backup_directory` o en `backups/` junto a la carpeta de
  la configuración, comprobado con `pg_restore --list`; instalación de Windows: el respaldo de `internal/standalone` con
  la etiqueta `antes-de-quitar-datos-de-prueba`). Si el respaldo falla: 503 `BACKUP_FAILED` («No se pudo crear el
  respaldo de la base de datos; no se quitó ningún dato de prueba.») y auditoría `maintenance.test_data_removed` con
  `success=false` (con un plazo propio de 10 s, también si el de la petición ya venció). Si quedó bien: una
  transacción con 7820148305, 7820148304 y 7820148308 exclusivos recalcula el plan
  (409 `TEST_DATA_CHANGED` si cambió), lo aplica, audita `maintenance.test_data_removed` con la lista y el nombre del
  respaldo, publica `kitchen.printers.changed`, `kitchen.stations.changed`, `kitchen.routes.changed`,
  `kitchen.access.changed`, `payments.settings.changed` (ámbito `configuration`), `cash.settings.changed` (ámbito
  `cash`, por caja) y `maintenance.test_data.removed`, y guarda la respuesta en `restaurant_operations`.
- Respuesta: `{removed, deactivated, unlinked, kept: Entry[], backup}` (`backup` = solo el nombre del archivo, vacío si
  no había nada que quitar). Tiempo de respuesta de `maintenance/*`: 120 s (el puente espera 130 s).
- Puente nativo (lista cerrada de `api.rs`): `maintenance_test_data` (GET) y `maintenance_remove_test_data` (POST).

## Etapa 9F · ruta antigua de anulación retirada y autorización para cancelar unidades (fase E, sin migración)

Sin migración, permisos ni operaciones nuevas del puente (`restaurant_cancel` y `restaurant_edit` ya existían). Detalle
operativo en [RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md) y de los reportes en [REPORTS.md](docs/REPORTS.md).

### Ruta antigua de anulación retirada (`POST restaurant/accounts/{id}/void`)

Decisión D8 del coordinador (§7.1): la ruta se **retira**, no se redirige («Cancelar cuenta» rechaza como descartables
justo las cuentas que esta ruta aceptaba). Cualquier `POST restaurant/accounts/{id}/void` responde **`410
ROUTE_RETIRED`** («Esta forma de cancelar cuentas ya no existe. Usa «Cancelar cuenta», con su motivo y, si hace falta,
la autorización de un supervisor, o «Descartar cuenta vacía» si la cuenta no tiene productos enviados.») **antes** de
comprobar permisos, tomar el lock o leer la cuenta: no escribe nada (ni operación idempotente, ni evento, ni auditoría,
ni la denegación) y la transacción se revierte. Ningún cliente, tampoco la futura aplicación Android, puede cancelar sin
motivo ni autorización. Los eventos `account.void` y la auditoría `restaurant.account.void` anteriores se conservan y
«Cuentas canceladas» los sigue mostrando como «Cancelación anterior con motivo libre». La descripción de
`accounts.manage` en la base («… y cancelar cuentas vacías con motivo libre», de la 0021) solo la cambia una migración.

### Autorización para cancelar unidades (`cancel` y la reducción con `edit`)

Mismo criterio de «Cancelar cuenta» (9D): con `orders.cancel` basta el permiso propio; sin él, la autorización puntual
de un supervisor que lo tenga, con el autorizador de `administration/supervisor.go` (sin sesión, con el límite de intentos
del inicio de sesión, sin autoautorización) y siempre **antes de la primera escritura**.

| Ruta (`/restaurant`)               | Permiso de la ruta                                                                    | Cuerpo                                                                                                    |
| ---------------------------------- | ------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| POST `/accounts/{id}/cancel`       | `accounts.read` y (`orders.cancel` o `accounts.open`); para llevar, `takeaway.manage` | `{operation_id, revision, version_id, quantity, reason, authorization?: {username, password}}`            |
| POST `/accounts/{id}/edit`         | `orders.edit` (sin cambio)                                                            | `{operation_id, revision, version_id, line, reason, expected_fingerprint, authorization?}`                |
| POST `/accounts/{id}/edit-preview` | `orders.edit` (sin cambio); solo calcula                                              | el de siempre (acepta `authorization`, que ignora); responde además `cancel_units_requires_authorization` |

- **Orden de comprobación** de `cancel`: permiso de la ruta, motivo (5–500), revisión, partida y cantidad, unidades sin
  pago ni descuento (`UNIT_HAS_PAYMENT`/`UNIT_HAS_DISCOUNT`), **autorización**; solo entonces se escribe. En `edit`: lo de
  siempre, la huella (`QUOTE_CHANGED`) y, si la modificación deja menos unidades de las que tenía la partida, la
  autorización. Una modificación que no reduce no pide ni registra autorización.
- **Rechazos** (los mismos códigos de «Cancelar cuenta»): sin permiso ni autorización `403 AUTHORIZATION_REQUIRED`
  («Cancelar o reducir unidades enviadas a cocina requiere la autorización de un supervisor con permiso para cancelar
  partidas.»); el mismo usuario `403 SELF_AUTHORIZATION`; usuario o contraseña incorrectos `401 AUTHORIZATION_INVALID`
  (cuenta en el límite de intentos) o `429 AUTHORIZATION_THROTTLED`; un supervisor sin `orders.cancel` `403
AUTHORIZATION_INSUFFICIENT` («El supervisor indicado no tiene permiso para cancelar partidas o cantidades.»); sin
  `orders.cancel` ni `accounts.open`, `403 PERMISSION_DENIED`. Con el permiso propio, una autorización enviada se ignora.
- **Qué se registra**: el evento `item.cancelled` (o `item.modified` de una reducción) agrega a `after`
  `authorized_by` (UUID del supervisor), `authorized_by_username` y `authorized_by_name` (nombre visible), los tres en
  `null` si bastó el permiso propio; la auditoría `restaurant.item.cancelled`/`restaurant.item.modified` lleva lo mismo y la
  autorización se audita aparte como `restaurant.supervisor_authorized` con `scope` `item_cancel` o `item_modify`,
  `supervisor`, `version_id`, `quantity` y `amount`. Quien hizo la operación es el autor del evento. La contraseña nunca
  entra en la huella idempotente, la respuesta guardada, los eventos ni la auditoría: un reintento con el mismo
  `operation_id` sin la contraseña recupera el resultado ya confirmado; con otros datos, `409 IDEMPOTENCY_CONFLICT`.
  Como Go acepta las claves sin distinguir mayúsculas, la huella de `restaurant/*` quita la contraseña de cualquier
  objeto `authorization` sin distinguirlas (`Authorization`, `Password`, `PASSWORD`…), en cualquier nivel del cuerpo
  (revisión independiente de la 9F, B3).
- **Campos nuevos de lectura** (los calcula Go; React no los deduce):

| Campo                                 | Dónde                        | Significado                                                                                                       |
| ------------------------------------- | ---------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `cancel_units_requires_authorization` | cada cuenta (`account`)      | La cuenta está abierta y quien lee no tiene `orders.cancel`: «Cancelar unidades» pedirá supervisor.               |
| `cancel_units_requires_authorization` | respuesta de cada cotización | Solo en `edit-preview`: la modificación reduce unidades y quien la pide no tiene `orders.cancel`; si no, `false`. |

### Reportes (`reports/run`, `reports/export`)

`products.quantity_changes` «Modificaciones de cantidades» agrega la columna **`authorized_by` «Autorizó»** (después de
«Modificó») y `products.cancelled` «Productos cancelados» también (después de «Canceló»): el nombre visible actual del
supervisor de `after.authorized_by`; en las partidas de una cuenta cancelada, quien autorizó cancelarla; vacía si bastó el
permiso propio y en los registros anteriores a la 9F. Sale igual en PDF y Excel. `employees.activity` cuenta en
«Autorizaciones como supervisor» también las cancelaciones y reducciones de unidades. Sin filtros nuevos.

## Etapa 9F · impresión (fase B, esquema 22)

Migración `0022_printing_and_kitchen_stations.sql` (sección «Fase B»; la fase C agrega la suya al mismo archivo). Sin
permisos nuevos. Detalle funcional en [docs/KITCHEN_AND_PRINTING.md](docs/KITCHEN_AND_PRINTING.md),
[docs/PAYMENTS_AND_REFUNDS.md](docs/PAYMENTS_AND_REFUNDS.md) y [docs/DOCUMENT_DESIGNER.md](docs/DOCUMENT_DESIGNER.md).

### Impresoras: corte de papel (`kitchen/printers`)

- Campos nuevos de `Printer`: `cut_mode` (`full` | `partial`) y `cut_feed_lines` (entero de la lista cerrada 0–8 o
  `null`). CHECK `kitchen_printers_cut_mode_check` y `kitchen_printers_cut_feed_lines_check`; el valor por omisión de la
  columna para las nuevas es 4. Desde la revisión independiente (M1), `cut_feed_lines` son los **renglones en blanco
  entre el último renglón del documento y el corte**: Go cierra antes el último renglón si quedó abierto (precuentas,
  tickets y documentos de caja terminan sin salto), así que `GS V` llega siempre al inicio de un renglón (también con 0)
  y el avance es el mismo en todos los documentos. Las impresoras existentes quedan con `full` y, si ya cortaban,
  `cut_feed_lines: null` = «como hasta la 9E» (tres saltos fijos y `GS V 0`, los bytes de siempre); las que no cortaban,
  con 4. Sin corte, los bytes no cambian.
- Al guardar (`POST`/`PATCH`, `kitchen.printers.manage`, auditado como `kitchen.printer.saved`): `supports_cut` se
  guarda **igual a `cut`** (la casilla «el modelo admite corte» ya no existe en la interfaz; el CHECK
  `NOT cut OR supports_cut` de la 0005 sigue vigente). Compatibilidad: en una impresora **nueva**, si el cuerpo no trae
  `cut`, corta con papel de 80 mm y no corta con 58 mm; sin `cut_mode`, `full`; sin `cut_feed_lines` (o `null`), 4. Al
  editar, un campo que el cliente no envía (o `null`) conserva lo guardado, incluido el `null` de una impresora que ya
  cortaba; una que no cortaba y empieza a cortar sin avance recibe 4. Fuera de la lista: 400 `KITCHEN_VALIDATION`.
- Las fotografías de impresora guardadas en trabajos anteriores (sin los campos) se imprimen y reimprimen como antes:
  corte total con 3 saltos si tenían `cut`.

### Rechazos sin impresora válida (decisión D4)

Una impresora sirve para precuentas, tickets de pago y documentos de caja si está configurada en Cobros, activa y no es
de otra terminal (`agent`). Si no, Go responde **antes de encolar** (no se crea trabajo ni documento):

| Ruta                                                                  | Respuesta                                                                              |
| --------------------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| `POST payments/accounts/{id}/precheck/print`                          | 409 `PRINTER_NOT_CONFIGURED` o `PRINTER_UNAVAILABLE`                                   |
| `POST payments/accounts/{id}/payments` con `print`                    | 200; `printing: {requested, status: "not_configured", problem, printer_name, message}` |
| Operaciones de caja, cortes y cierre con `print`                      | 200; `printing` igual (`PrintOutcome.problem`, `printer_name`)                         |
| `POST payments/cash-reports/{id}/print`, `cash-operations/{id}/print` | 409 `PRINTER_NOT_CONFIGURED` o `PRINTER_UNAVAILABLE`                                   |
| `POST payments/accounts/{id}/documents/receipt`                       | 409 igual                                                                              |
| `POST payments/accounts/{id}/documents/retry` y `…/reprint`           | 409 igual (revisión M2: la impresora de tickets vigente)                               |
| `POST payments/cash-documents/{id}/retry` y `…/reprint`               | 409 igual (revisión M2)                                                                |
| `PUT payments/settings` con otra impresora de tickets                 | 422 `RECEIPT_PRINTER_UNAVAILABLE` (revisión B3)                                        |

`error.details = {document: "precheck" | "receipt" | "cash_document", problem: "missing" | "inactive" |
"unsupported", printer_name?}`. Mensajes: «No hay impresora configurada para precuentas.» (con el nombre visible:
«…para tickets de venta.»), «No hay impresora configurada para tickets de pago.», «…para los documentos de caja.»,
«La impresora X está desactivada.», «Esta impresora todavía no puede usarse para imprimir.». Las comandas no cambian:
una impresora inactiva bloquea la liberación como antes y la de otra terminal deja el trabajo en «Requiere
intervención», ahora con el texto «Esta impresora todavía no puede usarse para imprimir.» (antes «El destino está
inactivo o requiere un agente de impresión aún no implementado»).

### Rutas nuevas

| Método | Ruta                                       | Permiso                  | Cuerpo                               |
| ------ | ------------------------------------------ | ------------------------ | ------------------------------------ |
| POST   | `payments/accounts/{id}/documents/retry`   | `payments.tickets.print` | `{operation_id, ticket_id, job_id?}` |
| POST   | `payments/accounts/{id}/documents/receipt` | `payments.tickets.print` | `{operation_id, payment_id}`         |
| POST   | `payments/cash-documents/{id}/retry`       | `cash.reports.print`     | `{operation_id, job_id?}`            |

- **Reintentar** (decisión D3): vuelve a `pending` el trabajo más reciente del documento (o el indicado) si está en
  `error` o `intervention`, **sin motivo**; no exige `kitchen.stations.all`. Desde la revisión independiente (M2) sale
  en la **impresora de tickets vigente de Cobros con su configuración actual** (la que se corrige con «Configurar
  impresora»): el trabajo cambia `printer_id` y `printer_snapshot` y, si el papel o las columnas cambiaron, su
  contenido se prepara de nuevo con los datos guardados del documento y la misma versión de plantilla. Respuesta
  `{status, job_id, ticket_id}` (caja: `PrintOutcome`). Rechazos: 404 si el documento no es de esa cuenta; 409
  `PRINT_NOT_FAILED` (nada que reintentar), `PRINT_UNCERTAIN` (pudo haber salido: se resuelve en la cola),
  `PRINT_JOB_SUPERSEDED` (hay una impresión más reciente), `PRINTER_NOT_CONFIGURED` / `PRINTER_UNAVAILABLE` (sin una
  impresora de tickets válida, con `details.document`) y `PRINT_RENDER_UNAVAILABLE` (un documento de caja anterior a
  las plantillas, sin datos guardados, en una impresora con otro papel).
  Caja aplica además las reglas de consulta del turno y del efectivo esperado. Auditoría `payments.document_retried` /
  `cash.document_retried`; historial del trabajo `print.retry`.
- **Ticket de un pago después**: imprime el ticket de un pago de la cuenta que no lo tiene (el «Reintentar» del aviso
  de un ticket que no se imprimió por falta de impresora). 409 `DOCUMENT_EXISTS` si ya existe (se reimprime con su
  motivo). Desde la revisión independiente (B1) solo para el pago cuyo ticket quedó sin imprimir por falta de impresora
  válida (la respuesta guardada de su `operation_id` dice `printing.status = "not_configured"`): si no, 409
  `RECEIPT_NOT_PENDING`; un pago con devoluciones o corregido, 409 `PAYMENT_REFUNDED`. Auditoría
  `payments.receipt_printed`.

### Cambios de contrato

- `POST kitchen/jobs/{id}/retry`: `reason` **opcional** (si se envía, 5–500). `resolve` y `POST kitchen/reprint` lo
  siguen exigiendo. Desde la revisión independiente (M2), todo envío de nuevo desde la cola (`retry` y `resolve` con
  `decision: "send"`) usa la configuración **actual** de la impresora del trabajo (o la elegida con `printer_id`) y, si
  su papel o sus columnas cambiaron, prepara el documento de nuevo con sus datos guardados y la misma versión de
  plantilla (comandas, avisos y prueba técnica en cocina; precuentas, tickets y documentos de caja con el renderizador
  de pagos, que administración pasa a cocina); una muestra del diseñador, una impresión con la plantilla vigente o un
  documento de caja sin datos guardados responde 409 `PRINT_RENDER_UNAVAILABLE`, y una prueba de ancho conserva las
  columnas que probaba (con otro papel, el mismo 409).
- `POST payments/accounts/{id}/documents/reprint` y `POST payments/cash-documents/{id}/reprint`: **`reason`
  obligatorio** (5–500, 400 `PAYMENT_VALIDATION` si falta, antes de buscar la impresora); queda en la auditoría
  (`payments.document_reprinted`, `cash.document_reprinted`) y en el historial del trabajo (`print.reprinted`). Desde la
  revisión independiente (M2) la copia sale en la impresora de tickets vigente con su configuración actual (destino,
  transporte, corte y copias; sin una válida, 409 `PRINTER_NOT_CONFIGURED`/`PRINTER_UNAVAILABLE`) y, como reimpresión
  exacta (decisión de la 9C), idéntica al original: conserva su contenido y el papel y las columnas con que se emitió.
- Revisión independiente (B2): `POST payments/accounts/{id}/documents/rerender` y
  `POST payments/cash-documents/{id}/rerender` («Imprimir con la plantilla vigente») piden **`reason` obligatorio**
  (5–500, 400 `PAYMENT_VALIDATION`), como la de cocina: queda en la auditoría (`payments.document_rerendered`,
  `cash.document_rerendered`) y en el historial del trabajo (`print.rerendered`, `{job, reason}`).
- Revisión independiente (B3): `PUT payments/settings` rechaza elegir como impresora de tickets una desactivada o de
  otra terminal: 422 `RECEIPT_PRINTER_UNAVAILABLE` con `details {problem, printer_name}` y el mensaje de D4 más «Elige
  otra impresora de tickets.». La que ya estaba elegida y dejó de servir no impide guardar las demás políticas.
- Resultados del transporte y trabajos: `Outcome.code` (clave del problema: `unreachable`, `not_found`, `inactive`,
  `unsupported`, `setup`, `uncertain`), guardada en `kitchen_print_jobs.result` y en cada intento. `not_found` (revisión
  independiente, B4): la cola CUPS no existe (`lpstat -p` falla) o la impresora de Windows no está instalada
  (`ERROR_INVALID_PRINTER_NAME`); un administrador de impresión o una conexión que no contesta sigue siendo
  `unreachable`. Lecturas:
  `GET kitchen/jobs` y `GET kitchen/jobs/{id}` agregan `ticket_kind`, `folio`, `document_title` y `problem`; los
  documentos de la cuenta (`documents[]` de `GET payments/accounts/{id}`) y de caja agregan `job_problem`.
- `payment_settings.precheck_name` (`precheck` | `sales_ticket` | `bill`, CHECK `payment_settings_precheck_name_check`):
  `GET payments/settings` lo informa. Desde la revisión independiente (M3) se elige en Administración › Documentos: `GET
documents/identity` lo informa como `precheck_name` y `PUT documents/identity` lo acepta opcional junto con la
  identidad (`documents.identity.manage`; sin él se conserva; fuera de la lista, 422 `VALIDATION_ERROR`), con la
  revisión de la identidad, la auditoría `documents.precheck_name_changed` (antes y después) y los eventos
  `documents.identity.changed` y `payments.settings.changed`. `PUT payments/settings` **ya no lo acepta** (400 por campo
  desconocido). `GET auth/me` lo informa en `establishment.precheck_name` para todo usuario. El diseñador llama «Ticket
  de pago» al tipo `receipt` (antes «Ticket de venta»). La precuenta guarda `title` («TICKET DE VENTA» o «CUENTA»; vacío
  con «Precuenta», idéntica a la anterior) y lo imprime como encabezado; una plantilla con título propio lo sigue
  sustituyendo.
- Contexto de la precuenta y del ticket: «Mesa X» sin repetir «Mesa» si el nombre de la mesa ya la incluye.
- Huella idempotente de cobros (`payments/handler.go`): quita la contraseña de toda autorización en cualquier nivel del
  cuerpo comparando las claves sin distinguir mayúsculas (como `operations`); el contrato no cambia.

### Puente nativo

- Lista cerrada de `api.rs`: `payments_retry_document`, `payments_print_receipt` y `payments_retry_cash_document`.
- `menu_set_state` acepta además `precheckName` opcional (lista cerrada `precheck`, `sales_ticket`, `bill`; otro valor
  rechaza el estado) y cambia solo el texto de «Imprimir precuenta». Sin rutas nuevas.

## Etapa 9F · cocina por estaciones y terminal de cocina (fase C, esquema 22)

Decisiones D6 y D7 del coordinador. Migración: sección «Fase C» de la 0022 (columnas `devices.kitchen_all_stations` y
`devices.kitchen_sound_alert`, columna `users.kitchen_all_stations`, tabla `kitchen_user_stations` y la descripción
«Editar datos de cuentas» de `accounts.manage`, encargo de la fase E). Sin permisos nuevos. Detalle en
[KITCHEN_AND_PRINTING.md](docs/KITCHEN_AND_PRINTING.md) y guía en [TERMINAL_DE_COCINA.md](docs/TERMINAL_DE_COCINA.md).

### Estaciones visibles

Las estaciones visibles son las de la terminal (todas o su selección) intersectadas con las del usuario (todas, su
selección o, con `kitchen.stations.all`, siempre todas). Antes de la 9F, `kitchen_device_stations` era el ámbito de quien
no tenía `kitchen.stations.all` (vacío = ninguna) y se ignoraba para quien lo tenía.

| Consulta o acción                                                        | Alcance                                                               |
| ------------------------------------------------------------------------ | --------------------------------------------------------------------- |
| `GET kitchen/board` (tareas y `stations`), `station_id`                  | Estaciones visibles, para todos                                       |
| `POST kitchen/states` (tareas o pedido con `station_id`)                 | Estaciones visibles: una tarea de otra `403 STATION_SCOPE_DENIED`     |
| `POST kitchen/tickets/{id}/ack`                                          | La estación del acuse debe ser visible                                |
| `GET kitchen/tickets`, `GET kitchen/tickets/{id}`, recientes y diseñador | Con `kitchen.stations.all`, todas; sin él, las visibles               |
| `GET events`, `GET events/snapshot` (`station_id`)                       | Con `kitchen.stations.all`, todas; sin él, las visibles (`403` fuera) |
| Liberar, programar, cambios pendientes, cola, reimpresión e historial    | `kitchen.stations.all` (sin cambio), sin depender de la terminal      |

`STATION_SCOPE_DENIED` dice ahora «Esta estación no se muestra en esta terminal o tu usuario no la atiende.».

`GET kitchen/board` agrega:

```json
{
  "station_scope": {
    "common": true,
    "all": false,
    "terminal_all": false,
    "terminal_stations": ["Freidora", "Plancha"],
    "user_all": false,
    "user_all_by_permission": false,
    "user_stations": ["Freidora"]
  },
  "sound_alert": false
}
```

`common: false` = la terminal y el usuario no tienen estaciones en común (tareas y `stations` vacías); los nombres van
en el orden de Cocina y solo los de una selección (con `terminal_all`/`user_all`, la lista va vacía).
`user_all_by_permission` indica `kitchen.stations.all`. `sound_alert` es el aviso sonoro de la terminal que consulta.

### Terminal (`GET`/`PUT kitchen/access`, `kitchen.stations.manage`)

Cada terminal autorizada agrega `home_screen`, `all_stations` y `sound_alert` a `{device_id, device_name, station_ids}`.
`PUT` recibe `{operation_id, device_id, all_stations?, station_ids, sound_alert?, expected_all_stations?,
expected_station_ids}`: sin `all_stations`, una lista con estaciones es la selección (forma anterior) y, desde la
revisión independiente (parte 3, B1), una lista vacía responde `422 KITCHEN_ACCESS_ALL_REQUIRED` (antes de la 9F
significaba «ninguna»; para «todas» hay que enviar `all_stations: true`); con «todas» la lista va vacía y sin ella lleva
de 1 a 100 estaciones existentes sin repetir (`400 KITCHEN_VALIDATION`); sin `sound_alert` se conserva. La selección
esperada (y la marca, si se envía) distinta de la vigente responde `409 REVISION_CONFLICT`. Responde `{device_id,
all_stations, station_ids, sound_alert}` y lo audita en el historial de cocina como `station.access.saved` (antes y
después).

### Usuario (`users/{id}/kitchen-stations`)

| Método | Permiso        | Cuerpo / respuesta                                                                                     |
| ------ | -------------- | ------------------------------------------------------------------------------------------------------ |
| GET    | `users.read`   | `{user_id, all_stations, station_ids, all_by_permission, stations:[{id, name, active}]}`               |
| PUT    | `users.manage` | `{all_stations, station_ids, expected_all_stations, expected_station_ids}` → la misma forma que el GET |

Mismas reglas de la selección que la terminal (`400 VALIDATION_ERROR`; campos desconocidos rechazados). Delegación de
siempre: nadie cambia sus propias estaciones ni las de un usuario con permisos que no tiene (`403 PERMISSION_DENIED`),
el usuario reservado `sa` no se consulta ni se cambia, la selección esperada distinta de la vigente responde `409
REVISION_CONFLICT`, se audita `user.kitchen_stations_changed` (antes y después, solo identificadores) en la misma
transacción y se publica `kitchen.access.changed` (ámbito `configuration`, sin contenido). `all_by_permission` indica
que el usuario atiende todas por `kitchen.stations.all` aunque tenga una selección guardada. La ruta no lleva
`operation_id`: desde la revisión independiente (parte 3, B4), si lo pedido ya es lo guardado (el reenvío de un guardado
cuya respuesta se perdió) responde 200 con lo vigente sin escribir, auditar ni publicar, en lugar del 409.

La descripción del permiso `kitchen.stations.manage` pasa a «Administrar estaciones y las estaciones que muestra cada
terminal» (en Go y con un `UPDATE` en la sección «Fase C» de la 0022), porque las estaciones por terminal ya no son un
ámbito de autorización.

### Puente nativo (`desktop/src-tauri/src/api.rs`)

Operaciones nuevas `user_kitchen_stations` (`GET users/{id}/kitchen-stations`) y `save_user_kitchen_stations`
(`PUT users/{id}/kitchen-stations`), con identificador validado. `kitchen_access` y `kitchen_save_access` no cambian.
