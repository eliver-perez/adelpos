# API v1 para clientes · Etapa 9G

Contrato de la API que usan la aplicación de escritorio y las terminales y que usará la **aplicación Android de
meseros** (proyecto aparte, etapa posterior). Fase D de la etapa 9G (§5 de
[etapas/ETAPA_9G.md](etapas/ETAPA_9G.md)).

- **Contrato OpenAPI 3.1**: [`backend/internal/apicontract/openapi.json`](../backend/internal/apicontract/openapi.json)
  es la fuente. El servidor lo publica en `GET /api/v1/openapi.json` (sin sesión) y está incrustado en el binario.
- **Pruebas de contrato**: `internal/apicontract` comprueba la estructura del documento y que este archivo explique
  cada operación (pruebas unitarias) y, contra PostgreSQL, recorre cada operación documentada con el servidor real y
  valida cada cuerpo enviado y cada respuesta contra su esquema (`TestContractV1`, integración). Un cambio que rompa
  la v1 —una ruta que desaparece o cambia de estado, una propiedad obligatoria que se quita o cambia de tipo— hace
  fallar las pruebas.
- Para ampliar el contrato: documentar la operación en `openapi.json`, explicarla aquí (con su `operationId` y su
  ruta) y agregar su paso en el recorrido de `contract_integration_test.go`. Agregar propiedades a una respuesta no
  rompe la v1 (los esquemas no prohíben propiedades nuevas); quitar o cambiar una obligatoria sí.

## Reglas para cualquier cliente

1. **Go es la única autoridad** de precios, impuestos, permisos y estados. El cliente muestra lo que devuelve el
   servidor y nunca decide una regla de negocio. Los importes viajan como cadenas decimales con dos decimales
   (`"130.00"`, IVA incluido); nunca como números de punto flotante.
2. **Identificación del cliente**: cada petición lleva `X-Client-Version` (la versión de la aplicación,
   `MAJOR.MINOR.PATCH`) y `X-Client-Kind` (`desktop` o `android`). Si la versión es anterior a
   `min_client_version`, las rutas de la aplicación responden **426 `CLIENT_UPDATE_REQUIRED`** («Actualiza esta
   terminal») con `details.min_client_version`. **Por la escucha de red** (HTTPS, la única que usan las terminales y
   el teléfono) `X-Client-Version` es **obligatoria**: sin ella la respuesta también es 426 (fase C; todo cliente que
   llega por la red es de la 9G o posterior). Por la escucha local de la computadora principal una petición sin
   `X-Client-Version` se acepta como «cliente sin identificar» (herramientas, pruebas automáticas y aplicaciones
   anteriores a la 9G, que solo existen en esa computadora). Una versión o un tipo que el servidor no reconoce es 400
   (`CLIENT_VERSION_INVALID`, `CLIENT_KIND_INVALID`).
3. **Servidor más viejo que el cliente**: el cliente lee `GET /api/v1/version` (siempre responde, sin sesión ni
   cabeceras) y compara `server_version` con la versión mínima de servidor que necesita (en el escritorio,
   `min_server_version` de `config/product.json`); si es menor, avisa «Actualiza el servidor» en lugar de fallar en
   cada operación. `capabilities` dice qué funciones existen (`catalog-sync-v1`, `catalog-sold-out`,
   `offline-orders-v1`, `state-recovery-v1`, `client-version-check`, `openapi-v1`…).
4. **Credenciales**: la terminal (o el teléfono) emparejada envía `X-Device-ID` y `X-Device-Token`; con sesión,
   además `Authorization: Bearer <token>`. Los tokens se guardan en el almacén seguro del sistema. Go valida terminal,
   sesión y permisos en cada petición. **Una sola sesión activa por usuario** en todo el sistema (decisión D9): iniciar
   sesión en otro dispositivo cierra la anterior (`SESSION_REPLACED`).
5. **Errores**: `{"error": {"code", "message", "request_id", "details"?}}`. `message` está en español para mostrarse
   tal cual; el cliente decide por `code`. 401 `DEVICE_*` / `SESSION_*` borra la sesión (y con `DEVICE_REVOKED` o
   `DEVICE_REQUIRED`, la credencial de la terminal: hay que emparejar otra vez).
6. **Idempotencia** (§5.2): ver abajo. Toda escritura de pedidos, cuentas, cocina, cobros y «Agotado» lleva
   `operation_id`.

## Operaciones

| `operationId`            | Método y ruta (bajo `/api/v1`)           | Para qué                                                                       |
| ------------------------ | ---------------------------------------- | ------------------------------------------------------------------------------ |
| `getVersion`             | `GET /version`                           | Versión del servidor, `min_client_version`, tipos de cliente y funciones.      |
| `getReadiness`           | `GET /health/ready`                      | Servidor y base de datos listos (200) o no (503).                              |
| `getOpenAPI`             | `GET /openapi.json`                      | Este contrato.                                                                 |
| `login`                  | `POST /auth/login`                       | Iniciar sesión en una terminal emparejada.                                     |
| `logout`                 | `POST /auth/logout`                      | Cerrar la sesión.                                                              |
| `getIdentity`            | `GET /auth/me`                           | Usuario, permisos vigentes, preferencias, terminal y establecimiento.          |
| `streamEvents`           | `GET /events`                            | Eventos en tiempo real (SSE).                                                  |
| `getCatalogSync`         | `GET /catalog/sync`                      | Catálogo descargable completo o «solo cambios desde la versión X».             |
| `listCatalogProducts`    | `GET /catalog/products`                  | Productos para la captura con disponibilidad, «Agotado» y precio de una lista. |
| `getCatalogImage`        | `GET /catalog/images/{id}`               | Imagen en JSON base64 (la usa el escritorio).                                  |
| `getCatalogImageContent` | `GET /catalog/images/{id}/content`       | Imagen binaria con `ETag` (304 con `If-None-Match`).                           |
| `setProductSoldOut`      | `POST /catalog/products/{id}/sold-out`   | «Agotado» y «Disponible otra vez».                                             |
| `getRecoveryState`       | `GET /restaurant/state`                  | Estado al reconectar: permisos, mesas, cuentas y preparación.                  |
| `listAccounts`           | `GET /restaurant/accounts`               | Cuentas con sus estados.                                                       |
| `openAccount`            | `POST /restaurant/accounts`              | Abrir una cuenta con su primer producto.                                       |
| `getAccount`             | `GET /restaurant/accounts/{id}`          | Cuenta con su borrador y sus partidas confirmadas.                             |
| `saveDraft`              | `PUT /restaurant/accounts/{id}/draft`    | Guardar el borrador.                                                           |
| `previewDraft`           | `GET /restaurant/accounts/{id}/preview`  | Calcular el borrador y obtener su huella.                                      |
| `confirmDraft`           | `POST /restaurant/accounts/{id}/confirm` | Confirmar (enviar a cocina) el borrador.                                       |
| `submitOfflineOrder`     | `POST /restaurant/offline-orders`        | Pedido capturado sin conexión (Android).                                       |
| `createPairingRequest`   | `POST /pairing/requests`                 | Pedir un código de emparejamiento (sin credencial).                            |
| `revealPairingNonce`     | `POST /pairing/requests/{id}/reveal`     | Revelar el número de la terminal después de recibir el del servidor.           |
| `getPairingRequest`      | `GET /pairing/requests/{id}`             | Consultar si la computadora principal ya aprobó el código.                     |
| `renamePairingRequest`   | `POST /pairing/requests/{id}/name`       | Corregir el nombre propuesto mientras espera.                                  |
| `cancelPairingRequest`   | `POST /pairing/requests/{id}/cancel`     | Dejar de esperar la aprobación.                                                |
| `claimPairingCredential` | `POST /pairing/requests/{id}/claim`      | Recibir una sola vez la credencial de la terminal.                             |

## Encontrar el servidor y emparejarse (fases B y C)

1. **Buscar** (fase C): el servidor se anuncia en la red local por mDNS/DNS-SD como `_adelpos._tcp` (dominio
   `local`). El nombre de la instancia es legible («Fonda Doña Ñeli — Mac de caja», hasta 63 bytes UTF-8) y el TXT
   lleva `txtvers=1`, `server_id` (UUID), `api=v1`, `port` (el de la escucha HTTPS, 8443 por omisión) y `fp` (los
   primeros 8 bytes de la huella SPKI en hexadecimal). El cliente hace una consulta PTR de una sola vez desde un puerto
   efímero a `224.0.0.251:5353` y lee las respuestas unicast (PTR, y en los adicionales SRV, TXT y A); elige por
   `server_id`, nunca por el nombre. Si la red bloquea el descubrimiento, el usuario escribe la dirección (IP privada o
   nombre `.local`; el puerto por omisión es 8443). El anuncio no es una prueba de identidad: solo da la dirección.
2. **Emparejar** (fase B): HTTPS a esa dirección. El cliente fija la huella SPKI (SHA-256 de la llave pública del
   certificado) del primer certificado que ve, comprueba que su prefijo coincide con `fp` si vino de la búsqueda e
   ignora nombre, IP, cadena y vigencia del certificado. `createPairingRequest` con el compromiso
   `SHA-256(nT)` de su número aleatorio de 32 bytes → `server_nonce` (nS), `pairing_secret` (para las demás llamadas,
   en `X-Pairing-Secret`) y vencimiento a 5 minutos. Después `revealPairingNonce` con nT. El código de 6 dígitos que
   muestra el teléfono es `SHA-256(huella ‖ nT ‖ nS ‖ id de la solicitud en 16 bytes)`, primeros 8 bytes big-endian
   módulo 10⁶ (vector de prueba en `administration/pairing_test.go`: `166101`).
3. Un administrador escribe ese código en la computadora principal (Sistema › Terminales; solo ahí se aprueba). El
   cliente consulta `getPairingRequest` cada 2 s; con `approved`, `claimPairingCredential` entrega una sola vez
   `device_id` y `device_token`, que se guardan en el almacén seguro junto con `server_id` y la huella.
4. **Desde entonces** el cliente solo se conecta a un servidor que presente esa huella. Si no logra conectar con la
   última dirección (cambio de IP por DHCP), vuelve a buscar por `server_id`, se conecta a la dirección nueva
   comprobando la huella completa (sin emparejarse de nuevo) y la guarda como última dirección. Un servidor con el
   mismo nombre o `server_id` pero otra huella nunca se acepta. Errores del emparejamiento: `PAIRING_*` (por ejemplo
   `PAIRING_EXPIRED`, `PAIRING_REJECTED`, `PAIRING_ALREADY_CLAIMED`, 429 `PAIRING_RATE_LIMITED` y `PAIRING_LIMIT_REACHED`); detalle en
   [API_CONTRACTS.md](../API_CONTRACTS.md), «Etapa 9G · emparejamiento de terminales».

## Sesión y recuperación del estado

1. `getVersion` → si `server_version` es menor que lo que necesita la aplicación, avisar «Actualiza el servidor».
2. `login` con usuario y contraseña → token de sesión (12 h). `getIdentity` da los permisos vigentes.
3. Al reconectar después de perder la red, **`getRecoveryState`** (`GET /restaurant/state`) es la garantía: no se
   depende de haber recibido todos los eventos. Responde a cualquier sesión válida, aunque el usuario ya no pueda
   consultar cuentas, para que el cliente sepa qué permisos perdió:
   - `user.permissions`: permisos vigentes (leídos de PostgreSQL en esa petición);
   - `terminal`: la terminal (una terminal revocada no llega aquí: 401 `DEVICE_REVOKED`);
   - `catalog_version` y `catalog_epoch`: si la época es otra, descargar todo; si no y la versión es mayor que la del
     catálogo guardado, descargar los cambios;
   - `tables` (con `restaurant.tables.read`) con su estado (`free`, `occupied`, `awaiting_payment`,
     `partial_payment`, `out_of_service`) y `accounts` (con `accounts.read`; las de para llevar con `takeaway.read`):
     cuentas abiertas con su **revisión** y sus estados de preparación, pago y entrega. Sin el permiso, la lista es
     `null`.
4. `streamEvents` (`GET /events`, con `events.read`): mensajes «changes» cada 750 ms con `cursor`, `reset` y
   `events` (tipo, ámbito y agregado, nunca importes). El flujo dura 30 s; el cliente se reconecta con
   `?after=<cursor>` (o `Last-Event-ID`). Fuera de la ventana de 5000 eventos llega `reset: true` y el cliente vuelve a
   consultar todo. Desde la 9G existe el ámbito **`catalog`**: `catalog.changed` (cualquier cambio del catálogo),
   `catalog.product.sold_out` y `catalog.product.available` (con el producto como agregado).

## Catálogo descargable por versión (§5.3)

- **Versión**: un número que aumenta con cada transacción que cambia el catálogo (categorías, productos, presentaciones,
  ingredientes, complementos, porciones, grupos y sus opciones, combos, listas y precios, indicaciones rápidas,
  imágenes, «Agotado» y la tasa general de IVA). Sale de la bitácora `app.catalog_change_log` (migración 0023), que
  llenan disparadores en todas las tablas del catálogo.
- **`getCatalogSync`** (`GET /catalog/sync`, con `catalog.read` y `catalog.prices.read`):
  - sin `since` (o `since=0`): **catálogo completo** (`full: true`): el cliente reemplaza lo guardado. No incluye
    productos archivados;
  - `?since=X&epoch=E`: **solo los cambios** posteriores a la versión X (`full: false`): los registros cambiados de
    cada lista, los productos archivados con `archived: true` y los registros borrados en `deleted` (`entity`, `id`).
    Si cambió una categoría, llegan también sus productos; si cambió la lista predeterminada o la tasa general de
    IVA, llegan todos los productos vigentes **y los archivados que cambiaron**; un producto que cambió (por ejemplo,
    agotado) trae también **los combos que lo ofrecen y los grupos cuyos complementos lo usan**, con la
    disponibilidad de cada opción (`available_now`, `sold_out`) y el combo no disponible si una ranura obligatoria
    se quedó sin opciones;
  - **época** (`catalog_epoch`, en cada descarga y en `getRecoveryState`): cambia cuando la base se restaura de un
    respaldo o pasa a otro clúster, porque entonces los números de versión se repiten con otro contenido. La descarga
    de cambios exige `epoch` (sin ella, 400); con otra época, o con una `since` mayor que la versión vigente, se
    entrega todo con `reset: true`. **El cliente guarda la versión y la época de cada descarga; con `reset: true`
    borra lo guardado y lo reemplaza por la respuesta** (también sus imágenes en caché, que se vuelven a pedir con su
    `ETag`).
- **Precios con IVA**: `price` de cada producto y `effective_price` de cada presentación son los de la lista
  predeterminada, calculados por Go con las mismas reglas de la cotización (`catalog.Calculate`); `effective_tax_rate`
  es la tasa propia o la general. Si la lista no le da precio, `price` es `null` y `pricing_error` lo explica. Los
  complementos y sus valores (`reference_value`, `extra_price`, tramos) sirven para mostrar; el precio que cuenta es el
  que calcula el servidor al recibir el pedido.
- **Disponibilidad**: `available_now`, `availability_reason`, `sold_out`, `sold_out_indefinite` y `sold_out_until` se
  calculan al momento de la descarga. Los horarios (`availability`) cambian con la hora sin cambiar la versión: el
  cliente puede usarlos para mostrar, y el servidor decide al recibir el pedido.
- **Imágenes aparte**: la descarga solo lleva su referencia (`images`: `id`, `checksum`, tipo y tamaño).
  **`getCatalogImageContent`** (`GET /catalog/images/{id}/content`) entrega el PNG o JPEG con `ETag` = checksum y
  caché larga (el contenido de una imagen nunca cambia: una imagen nueva tiene otro `id`); con `If-None-Match` igual a
  la etiqueta responde **304** sin contenido. `getCatalogImage` (`GET /catalog/images/{id}`, JSON base64) sigue para
  el escritorio.
- `listCatalogProducts` (`GET /catalog/products`) es el listado paginado que usa la captura del escritorio, con
  disponibilidad, «Agotado» y, con `price_list_id`, el precio efectivo de esa lista.

## «Agotado» (§5.4, decisión D8)

- **`setProductSoldOut`** (`POST /catalog/products/{id}/sold-out`, permiso propio `catalog.sold_out.manage`, distinto
  de «Pausar» y de los horarios): `{"operation_id", "sold_out": true}` lo marca agotado **hasta el inicio del día
  comercial siguiente** («Hora de inicio del día», 05:00 por omisión, en la zona del establecimiento); con
  `"indefinite": true`, **hasta nuevo aviso**. `{"sold_out": false}` es «Disponible otra vez». Responde el estado
  vigente (`SoldOutState`) con la versión nueva del catálogo; es idempotente, se registra en el Registro de movimientos
  y se avisa en tiempo real.
- Un producto agotado **no se puede capturar ni confirmar**: 409 **`PRODUCT_SOLD_OUT`** («Hamburguesa está agotado…»),
  tampoco como complemento de otro producto ni como componente de un combo, ni al guardar el borrador o abrir una
  cuenta con él (una partida que ya estaba guardada cuando se agotó se puede conservar o reducir, pero no aumentar; la
  confirmación la rechaza). Ninguna excepción de disponibilidad lo pasa
  por alto. Marcarlo no cambia la revisión del producto.
- Aplica a **productos** (también a los que se usan como complemento o componente). Los complementos que son
  ingredientes y las porciones no se agotan desde la captura: se desactivan en Catálogo.
- Un trabajador de Go limpia las marcas vencidas cada 30 s (sube la versión del catálogo y avisa a las terminales);
  la lectura ya ignora una marca vencida aunque el trabajador no haya pasado.

## Cuentas y pedidos

- `listAccounts` (`GET /restaurant/accounts`), `getAccount` (`GET /restaurant/accounts/{id}`): cuentas con su
  **revisión** y sus estados (`preparation_state`, `payment_state`, `delivery_state`); las partidas confirmadas traen
  su `preparation_state`.
- `openAccount` (`POST /restaurant/accounts`): abre la cuenta con su primer producto (ninguna cuenta se guarda sin
  productos). Con `seen_account_ids` (las cuentas que el cliente veía en la mesa) responde 409 `TABLE_HAS_ACCOUNT` si
  otra terminal ya abrió una.
- `saveDraft` (`PUT /restaurant/accounts/{id}/draft`) guarda el borrador con la revisión vigente (409
  `ACCOUNT_CONFLICT` si otra terminal cambió la cuenta: recargar y comparar). `previewDraft`
  (`GET /restaurant/accounts/{id}/preview`) lo cotiza y devuelve la huella; `confirmDraft`
  (`POST /restaurant/accounts/{id}/confirm`) lo confirma solo si la huella sigue igual (409 `QUOTE_CHANGED` si cambió un
  precio o el borrador). Cocina recibe el pedido confirmado.

## Idempotencia (§5.2)

- Toda escritura de pedidos, cuentas, cocina, cobros y «Agotado» exige `operation_id`: un **UUID que genera el
  cliente** una vez por operación y **conserva en cada reintento** (nunca se genera otro después de una respuesta
  incierta).
- Si la misma clave llega otra vez con el mismo usuario, la misma terminal y el mismo contenido, el servidor
  **devuelve la respuesta guardada sin repetir la operación** (también si los reintentos llegan a la vez: las
  escrituras toman un lock exclusivo y el segundo encuentra la respuesta del primero). Con otro contenido, otro usuario
  u otra terminal responde **409 `IDEMPOTENCY_CONFLICT`** sin hacer nada.
- La huella es SHA-256 del método, la ruta y el cuerpo en JSON canónico (sin la contraseña de una autorización de
  supervisor). Un error no guarda la clave: reintentar después de un error vuelve a ejecutar.
- **Conservación**: las claves y sus respuestas (`app.restaurant_operations`) **se conservan para siempre** (la
  especificación pide al menos 7 días).

## Pedido capturado sin conexión (§5.5)

La aplicación Android podrá capturar sin señal con el catálogo guardado y una cola de pedidos. Cada pedido de la cola
se envía con **`submitOfflineOrder`** (`POST /restaurant/offline-orders`; permisos `orders.confirm`, `orders.capture`,
`accounts.read`, `catalog.read`, `catalog.prices.read` y, para abrir una cuenta en una mesa, `accounts.open`):

```json
{
  "operation_id": "una por pedido de la cola, la misma en todos sus reenvíos",
  "catalog_version": 41,
  "captured_at": "2026-11-13T20:15:00Z",
  "destination": { "table_id": "…", "seen_account_ids": [] },
  "items": [
    {
      "id": "…",
      "product_id": "…",
      "variant_id": null,
      "quantity": 2,
      "customization": { "ingredients": [], "groups": [], "components": [], "notes": "" },
      "seen_unit_price": "120.00"
    }
  ]
}
```

- `destination`: una cuenta abierta (`account_id`) o una mesa donde abrir una cuenta nueva (`table_id`, con las
  cuentas que el mesero veía abiertas en ella y un `name` opcional).
- `seen_unit_price`: el precio por unidad (con su personalización, antes de promociones) que vio el mesero; para un
  producto sin personalizar es el `price` de la descarga.
- **Sin cambios** → **200** `status: "accepted"`: el pedido se confirma en una sola operación idempotente con los
  precios del servidor, como «Enviar» en la captura (no toca el borrador de la cuenta) y cocina lo recibe. La respuesta
  trae la cuenta completa (`bill`).
- **Con cambios** → **409 `ORDER_REQUIRES_CONFIRMATION`**, sin guardar nada, con `details.changes`: por cada cambio,
  el producto (`product_id`, `product_name`), `before`, `now`, el motivo (`kind`, `message`, p. ej. «Hamburguesa:
  $120.00 → $130.00», «Malteada de fresa: agotado. Quítalo del pedido.») y si el mesero puede aceptarlo
  (`acceptable`), además de la `catalog_version` vigente. Motivos:
  - aceptables con la confirmación del mesero: `price_changed`, `table_has_account` (la mesa ya tiene una cuenta que
    el mesero no veía: confirmar abre otra);
  - que exigen corregir el pedido (`requires_changes: true`): `product_sold_out`, `product_unavailable`,
    `product_removed`, `complement_unavailable`, `item_unavailable`, `item_invalid`, `account_closed` (la cuenta ya se
    cobró o se cerró), `table_unavailable`. El cliente quita o cambia la partida (o el destino) y reenvía.
- **Confirmación**: el cliente reenvía el pedido con **la misma `operation_id`** y
  `"confirmation": {"catalog_version": <la del rechazo>, "changes": <los cambios que vio el mesero>}`. Se acepta solo
  si esa es la versión vigente del catálogo y los cambios cubren todos los de ese momento; entra **siempre con los
  precios del servidor**. Una confirmación con un catálogo que ya cambió otra vez se rechaza de nuevo con los cambios
  actuales.
- **Doble envío**: un pedido ya aceptado reenviado con la misma clave devuelve la misma respuesta. Un envío con la
  misma clave y otro contenido después de aceptado (por ejemplo, el original sin confirmación que llegó tarde y entró
  porque el precio volvió, y después el reenvío con la confirmación) responde **409 `OFFLINE_ORDER_ALREADY_ACCEPTED`**
  con `details.result`, el resultado que sí entró (`order_id`, `account_id`, cuenta completa): nunca entra dos veces y
  **el teléfono no debe capturarlo de nuevo**; marca el pedido de su cola como aceptado con ese resultado. Desde otro
  usuario u otra terminal, la misma clave es `IDEMPOTENCY_CONFLICT` sin datos.
- **Registro de movimientos**: `restaurant.offline_order.accepted` (hora de captura, hora de llegada, versiones del
  catálogo y cambios aceptados) y, en cada rechazo, `restaurant.offline_order.requires_confirmation` (con los cambios);
  ambos con la clave de operación como identificador. La historia de la cuenta marca el pedido (`order.confirmed` con
  `offline`).

## Códigos de error más frecuentes

| Código                                                   | Estado | Qué hacer                                                                              |
| -------------------------------------------------------- | ------ | -------------------------------------------------------------------------------------- |
| `CLIENT_UPDATE_REQUIRED`                                 | 426    | Actualizar la aplicación de este equipo.                                               |
| `DEVICE_REVOKED`, `DEVICE_REQUIRED`                      | 401    | Emparejar la terminal otra vez.                                                        |
| `PAIRING_EXPIRED`, `PAIRING_REJECTED`                    | 409    | Pedir otro código de emparejamiento.                                                   |
| `PAIRING_RATE_LIMITED`, `PAIRING_LIMIT_REACHED`          | 429    | Esperar unos minutos: límite por origen o tope global de solicitudes (30 cada 10 min). |
| `SESSION_REPLACED`, `SESSION_EXPIRED`, `SESSION_REVOKED` | 401    | Iniciar sesión otra vez.                                                               |
| `PERMISSION_DENIED`                                      | 403    | El usuario no tiene el permiso.                                                        |
| `ACCOUNT_CONFLICT`                                       | 409    | Otra terminal cambió la cuenta: recargarla.                                            |
| `QUOTE_CHANGED`                                          | 409    | El cálculo cambió: volver a calcular y confirmar.                                      |
| `TABLE_HAS_ACCOUNT`                                      | 409    | La mesa ya tiene una cuenta abierta: elegir.                                           |
| `PRODUCT_SOLD_OUT`                                       | 409    | El producto está agotado.                                                              |
| `PRODUCT_UNAVAILABLE`                                    | 409    | El producto no está disponible (pausado, fuera de horario o inactivo).                 |
| `ORDER_REQUIRES_CONFIRMATION`                            | 409    | Mostrar los cambios al mesero y reenviar con su confirmación.                          |
| `IDEMPOTENCY_CONFLICT`                                   | 409    | La clave ya se usó con otro contenido: consultar el resultado.                         |
| `OFFLINE_ORDER_ALREADY_ACCEPTED`                         | 409    | El pedido sin conexión ya entró: usar `details.result`, no capturarlo otra vez.        |
| `LOGIN_THROTTLED`                                        | 429    | Esperar 15 minutos.                                                                    |
