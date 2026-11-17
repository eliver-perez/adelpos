# API v1 para clientes · Etapa 9G

Contrato de la API que usan la aplicación de escritorio y las terminales y la **aplicación Android de meseros**
(carpeta `android/`, etapa A1). Fase D de la etapa 9G (§5 de [etapas/ETAPA_9G.md](etapas/ETAPA_9G.md)); la etapa A1
agregó la versión mínima por tipo de cliente, la entrada con PIN y la credencial de envío
([etapas/ETAPA_A1_ANDROID.md](etapas/ETAPA_A1_ANDROID.md), §3) y, para el modo Mesero (§6–§8), las mesas, cancelar y
modificar unidades, el evento de preparación, el motivo de una cuenta que ya no está abierta, el borrador en el pedido
sin conexión y el cursor de eventos de la recuperación (ver «Modo Mesero»).

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
   `MAJOR.MINOR.PATCH`) y `X-Client-Kind` (`desktop` o `android`). Si la versión es anterior a la mínima **de su
   tipo de cliente** (`min_client_versions` de `getVersion`, desde la etapa A1: `desktop` 0.10.0 y `android` 0.1.0,
   porque la aplicación Android numera sus versiones aparte), las rutas de la aplicación responden **426
   `CLIENT_UPDATE_REQUIRED`** («Actualiza esta terminal») con `details.min_client_version` (la mínima de ese tipo).
   Sin `X-Client-Kind` se exige la general, `min_client_version`, que es la del escritorio y se conserva para los
   clientes que solo leen esa propiedad. **Por la escucha de red** (HTTPS, la única que usan las terminales y
   el teléfono) `X-Client-Version` es **obligatoria**: sin ella la respuesta también es 426 (fase C; todo cliente que
   llega por la red es de la 9G o posterior). Por la escucha local de la computadora principal una petición sin
   `X-Client-Version` se acepta como «cliente sin identificar» (herramientas, pruebas automáticas y aplicaciones
   anteriores a la 9G, que solo existen en esa computadora). Con la credencial de un dispositivo, `X-Client-Kind` debe
   coincidir con el tipo con que se emparejó (`android` en un teléfono o tableta de meseros, `desktop` en una terminal
   de escritorio o de cocina; etapa A1, revisión R3): si no, **400 `CLIENT_KIND_MISMATCH`**, para que nadie tome la
   mínima de otro tipo de cliente. Una versión o un tipo que el servidor no reconoce es 400
   (`CLIENT_VERSION_INVALID`, `CLIENT_KIND_INVALID`).
3. **Servidor más viejo que el cliente**: el cliente lee `GET /api/v1/version` (siempre responde, sin sesión ni
   cabeceras) y compara `server_version` con la versión mínima de servidor que necesita (en el escritorio,
   `min_server_version` de `config/product.json`); si es menor, avisa «Actualiza el servidor» en lugar de fallar en
   cada operación. `capabilities` dice qué funciones existen (`catalog-sync-v1`, `catalog-sold-out`,
   `offline-orders-v1`, `state-recovery-v1`, `client-version-check`, `openapi-v1`…; desde la etapa A1,
   `client-versions-by-kind`, `waiter-pin-v1` (entrada con PIN, credencial de envío y su administración) y
   `waiter-mode-v1` (lo del modo Mesero de la sección «Modo Mesero»)). La
   aplicación Android exige un servidor 0.12.0 o posterior; el escritorio 0.12.0 sigue funcionando con un servidor
   0.11.0 y solo muestra lo del PIN si el servidor anuncia `waiter-pin-v1`.
4. **Credenciales**: la terminal (o el teléfono) emparejada envía `X-Device-ID` y `X-Device-Token`; con sesión,
   además `Authorization: Bearer <token>` (desde la etapa A1, en `submitOfflineOrder` el teléfono puede enviar en su
   lugar la credencial de envío del mesero, `X-Offline-Credential`; ver «Entrada con PIN y credencial de envío»). Los tokens se guardan en el almacén seguro del sistema. Go valida terminal,
   sesión y permisos en cada petición. **Una sesión activa por usuario y por dispositivo** (desde la etapa 9H, decisión
   del propietario; en la 9G era una sola por usuario en todo el sistema, decisión D9): el mismo usuario puede tener
   sesión a la vez en varios dispositivos (por ejemplo, una caja y un teléfono). Iniciar sesión **de nuevo en el mismo
   dispositivo** cierra la sesión anterior de ese dispositivo (`SESSION_REPLACED`); en otro dispositivo, no. Cerrar
   sesión cierra solo la de ese dispositivo; cambiar la contraseña, revocar el dispositivo, desactivar al usuario o el
   restablecimiento de soporte siguen cerrando las suyas como antes (`SESSION_REVOKED`; revocar el dispositivo,
   `DEVICE_REVOKED`). Lo que depende del dispositivo sigue siendo de cada uno: su caja física y el turno de caja, la
   pantalla de inicio, las estaciones de cocina que ve, el canal de eventos (hasta 2 conexiones por dispositivo) y
   `restaurant/state`. «Sesiones activas por tipo de terminal» (`devices/sessions-summary`, de administración, fuera de
   este contrato) cuenta sesiones, no usuarios.
5. **Errores**: `{"error": {"code", "message", "request_id", "details"?}}`. `message` está en español para mostrarse
   tal cual; el cliente decide por `code`. 401 `DEVICE_*` / `SESSION_*` borra la sesión (y con `DEVICE_REVOKED` o
   `DEVICE_REQUIRED`, la credencial de la terminal: hay que emparejar otra vez).
6. **Idempotencia** (§5.2): ver abajo. Toda escritura de pedidos, cuentas, cocina, cobros y «Agotado» lleva
   `operation_id`.

## Operaciones

| `operationId`            | Método y ruta (bajo `/api/v1`)                | Para qué                                                                       |
| ------------------------ | --------------------------------------------- | ------------------------------------------------------------------------------ |
| `getVersion`             | `GET /version`                                | Versión del servidor, `min_client_version`, tipos de cliente y funciones.      |
| `getReadiness`           | `GET /health/ready`                           | Servidor y base de datos listos (200) o no (503).                              |
| `getOpenAPI`             | `GET /openapi.json`                           | Este contrato.                                                                 |
| `login`                  | `POST /auth/login`                            | Iniciar sesión en una terminal emparejada.                                     |
| `logout`                 | `POST /auth/logout`                           | Cerrar la sesión.                                                              |
| `getIdentity`            | `GET /auth/me`                                | Usuario, permisos vigentes, preferencias, terminal y establecimiento.          |
| `listPinUsers`           | `GET /auth/pin-users`                         | Meseros que pueden entrar con PIN en el dispositivo (sin sesión, etapa A1).    |
| `pinLogin`               | `POST /auth/pin-login`                        | Entrar con PIN: sesión y credencial de envío (etapa A1).                       |
| `streamEvents`           | `GET /events`                                 | Eventos en tiempo real (SSE).                                                  |
| `getCatalogSync`         | `GET /catalog/sync`                           | Catálogo descargable completo o «solo cambios desde la versión X».             |
| `listCatalogProducts`    | `GET /catalog/products`                       | Productos para la captura con disponibilidad, «Agotado» y precio de una lista. |
| `getCatalogImage`        | `GET /catalog/images/{id}`                    | Imagen en JSON base64 (la usa el escritorio).                                  |
| `getCatalogImageContent` | `GET /catalog/images/{id}/content`            | Imagen binaria con `ETag` (304 con `If-None-Match`).                           |
| `setProductSoldOut`      | `POST /catalog/products/{id}/sold-out`        | «Agotado» y «Disponible otra vez».                                             |
| `getRecoveryState`       | `GET /restaurant/state`                       | Estado al reconectar: permisos, mesas, cuentas, preparación y cursor.          |
| `getRestaurantOverview`  | `GET /restaurant/overview`                    | Áreas, mesas (lista y mapa) y umbrales de tiempo (etapa A1).                   |
| `listAccounts`           | `GET /restaurant/accounts`                    | Cuentas con sus estados.                                                       |
| `openAccount`            | `POST /restaurant/accounts`                   | Abrir una cuenta con su primer producto.                                       |
| `getAccount`             | `GET /restaurant/accounts/{id}`               | Cuenta con su borrador y sus partidas confirmadas.                             |
| `saveDraft`              | `PUT /restaurant/accounts/{id}/draft`         | Guardar el borrador.                                                           |
| `previewDraft`           | `GET /restaurant/accounts/{id}/preview`       | Calcular el borrador y obtener su huella.                                      |
| `confirmDraft`           | `POST /restaurant/accounts/{id}/confirm`      | Confirmar (enviar a cocina) el borrador.                                       |
| `cancelUnits`            | `POST /restaurant/accounts/{id}/cancel`       | Cancelar unidades enviadas, con motivo (etapa A1 en el contrato).              |
| `previewEdit`            | `POST /restaurant/accounts/{id}/edit-preview` | Calcular la modificación de una partida enviada y su huella.                   |
| `editUnits`              | `POST /restaurant/accounts/{id}/edit`         | Modificar una partida enviada, con motivo.                                     |
| `submitOfflineOrder`     | `POST /restaurant/offline-orders`             | Pedido capturado sin conexión (Android; con sesión o credencial de envío).     |
| `createPairingRequest`   | `POST /pairing/requests`                      | Pedir un código de emparejamiento (sin credencial).                            |
| `revealPairingNonce`     | `POST /pairing/requests/{id}/reveal`          | Revelar el número de la terminal después de recibir el del servidor.           |
| `getPairingRequest`      | `GET /pairing/requests/{id}`                  | Consultar si la computadora principal ya aprobó el código.                     |
| `renamePairingRequest`   | `POST /pairing/requests/{id}/name`            | Corregir el nombre propuesto mientras espera.                                  |
| `cancelPairingRequest`   | `POST /pairing/requests/{id}/cancel`          | Dejar de esperar la aprobación.                                                |
| `claimPairingCredential` | `POST /pairing/requests/{id}/claim`           | Recibir una sola vez la credencial de la terminal.                             |

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
     `null`;
   - `event_cursor` (desde la etapa A1): el cursor del canal de eventos **en el momento de la lectura**. Go lo lee con
     el lock de la operación ya tomado y antes que las mesas y las cuentas, así que ningún cambio de cuentas, cocina o
     cobros queda entre el cursor y el estado. El cliente abre `streamEvents` con `?after=<event_cursor>` y no pierde
     nada; a lo más recibe un evento de algo que ya ve (y vuelve a leer de más).
4. `streamEvents` (`GET /events`, con `events.read`): mensajes «changes» cada 750 ms con `cursor`, `reset` y
   `events` (tipo, ámbito y agregado, nunca importes). El flujo dura 30 s; el cliente se reconecta con
   `?after=<cursor>` (o `Last-Event-ID`). Fuera de la ventana de 5000 eventos llega `reset: true` y el cliente vuelve a
   consultar todo. Desde la 9G existe el ámbito **`catalog`**: `catalog.changed` (cualquier cambio del catálogo),
   `catalog.product.sold_out` y `catalog.product.available` (con el producto como agregado). Desde la etapa A1,
   **`restaurant.account.preparation_changed`** (ver «Estados de preparación»). Un cliente ignora los tipos que no
   conoce.
5. **Orden recomendado al volver de segundo plano o recuperar la red** (etapa A1): `getVersion` → `getRecoveryState`
   (permisos, terminal, mesas, cuentas con su revisión, versión y época del catálogo y `event_cursor`) → catálogo
   completo si cambió la época o por cambios si subió la versión → `streamEvents?after=<event_cursor>`. Con
   `reset: true` en el canal, repetir desde `getRecoveryState`. Cada evento solo dice qué volver a pedir: con la cuenta
   como agregado, esa cuenta; `restaurant.*.changed` sin agregado (mesas, áreas…), `getRestaurantOverview`; en
   transferir, dividir o combinar (`restaurant.items.transferred`, `restaurant.account.split`,
   `restaurant.account.merged`) el agregado es la cuenta de **origen**: la de destino se reconoce por su revisión en
   `getRecoveryState`.

## Entrada con PIN y credencial de envío (etapa A1, fase B)

Los teléfonos y tabletas de meseros se comparten: cada mesero entra con su **PIN** de 4 a 6 dígitos. Solo un
dispositivo emparejado de tipo `android` usa estas rutas (en otro tipo, **403 `PIN_DEVICE_NOT_ALLOWED`**; las
terminales de escritorio siguen con usuario y contraseña). Llevan la credencial del dispositivo y **no** una sesión.

1. **`listPinUsers`** (`GET /auth/pin-users`): `{"device": {"id", "name", "idle_lock_seconds"}, "users": [...]}`.
   Cada usuario: `id`, `name` (nombre corto o, si no tiene, el visible), `initial`, `color` (hexadecimal de la paleta
   que elige el administrador, o `null`), `pin_version`, `locked_until` (hasta cuándo dura un bloqueo por tiempo, o
   `null`) y `requires_unlock` (siempre presente: `true` si el PIN espera a que un administrador lo desbloquee; entonces
   `locked_until` es `null`). Solo usuarios
   activos, no de soporte, con PIN y con el permiso **`waiter_devices.use`** («Entrar con PIN en teléfonos y tabletas
   de meseros»; lo tienen el administrador y el rol Mesero). `idle_lock_seconds` es el bloqueo por inactividad del
   dispositivo (120 por omisión; se cambia en Administración › Terminales y también llega en `terminal` de
   `getIdentity` y `getRecoveryState`).
2. **`pinLogin`** (`POST /auth/pin-login`, `{"user_id", "pin"}`) → `{"session_token", "pin_version",
"offline_credential"}`:
   - PIN incorrecto (o un usuario que no puede entrar): **401 `PIN_INVALID`**. Un PIN sin forma de PIN: 400.
   - **5 fallos en 15 minutos**, desde cualquier dispositivo, bloquean el PIN de ese usuario **5 minutos**: **429
     `PIN_LOCKED`** con `details.locked_until` (RFC 3339) y `details.requires_unlock: false` (también si se intenta
     durante el bloqueo, aunque el PIN sea correcto). Un administrador lo desbloquea en Personal › Usuarios. Queda en
     el Registro de movimientos.
   - **Bloqueo largo** (A1-D16, decisión del propietario): el **tercer bloqueo seguido en 24 horas** (contadas desde el
     primero de la serie) **sin una entrada correcta** deja el PIN bloqueado **hasta que un administrador lo
     desbloquee**: **429 `PIN_LOCKED`** con `details.locked_until: null` y `details.requires_unlock: true`. Ni el tiempo
     ni que el propio usuario cambie su PIN en «Mi PIN» lo quitan. Una entrada correcta borra la serie. Esquema
     `PinLockedError` del contrato.
   - PIN correcto sin `waiter_devices.use`: **403 `PERMISSION_DENIED`**.
   - Pocas entradas con PIN se verifican a la vez en el servidor (revisión R3): si no hay turno en unos segundos,
     **503 `PIN_LOGIN_BUSY`**; reintentar.
   - La sesión es como la de `login` (12 h) y **reemplaza solo la sesión anterior de ese usuario en ese
     dispositivo**: el cambio rápido de mesero no cierra las de los demás meseros del mismo dispositivo.
   - `offline_credential` es la **credencial de envío** del mesero en ese dispositivo: se entrega **una sola vez**,
     se guarda cifrada (Android Keystore) y la nueva sustituye a la anterior de ese mesero en ese dispositivo.
   - **Solo permisos de mesero** (A1-D17, decisión del propietario): la sesión abierta con PIN y la credencial de
     envío tienen los permisos del usuario **que están en la lista cerrada de permisos de mesero**, aunque la persona
     sea administrador o supervisor (lo administrativo sigue con contraseña en la computadora): `waiter_devices.use`,
     `restaurant.tables.read`, `restaurant.areas.read`, `accounts.read`, `accounts.open`, `orders.capture`,
     `orders.confirm`, `orders.edit`, `orders.cancel`, `catalog.read`, `catalog.prices.read` y `events.read` (nada de
     Para llevar todavía). `getIdentity` y `getRecoveryState` informan ya los permisos acotados; cualquier otra ruta
     responde 403 como sin el permiso. La lista vive en un solo lugar del servidor
     (`administration/waiter_scope.go`) y solo crece con las funciones de la aplicación (en la A2: Para llevar e
     impresión de la precuenta). Cambiar la contraseña desde esa sesión no la amplía.
3. **Verificador local y `pin_version`**: el dispositivo guarda el verificador del PIN de cada mesero con su
   `pin_version`. Al reconectarse (y al abrir la aplicación) pide `listPinUsers` y borra el verificador de quien ya no
   aparece (desactivado, sin PIN o sin permiso) o tiene otra `pin_version`. Cambiar o quitar el PIN (o que el propio
   usuario lo cambie en «Mi PIN») sube `pin_version`, **cierra sus sesiones en dispositivos android**
   (`SESSION_REVOKED`) y revoca su credencial de envío; desactivar al usuario también.
4. **Credencial de envío en `submitOfflineOrder`**: WorkManager entrega la cola **sin sesión** con las cabeceras del
   dispositivo y **`X-Offline-Credential: <offline_credential>`** (sin `Authorization`; las dos juntas son 400). Go
   valida al recibir, sin caché: dispositivo no revocado (si no, `DEVICE_REVOKED`), usuario activo, su PIN vigente con
   la misma `pin_version` con que se emitió, `waiter_devices.use` y **los permisos vigentes** de la operación (sin
   `orders.confirm`, 403 como con una sesión). El pedido queda a nombre del mesero y del dispositivo, igual que con su
   sesión, y la misma `operation_id` sirve en los reintentos aunque uno haya ido con sesión y otro con la credencial.
   - Credencial que ya no vale (PIN cambiado o quitado, usuario desactivado o sin `waiter_devices.use`, credencial
     reemplazada por una entrada posterior): **401 `OFFLINE_CREDENTIAL_INVALID`**. El dispositivo **conserva el
     pedido** en «Por enviar» y pide al mesero que entre con su PIN con conexión (eso emite una credencial nueva).
   - **Solo sirve en esa ruta**: en cualquier otra (o con otro método), **también las que no piden credencial**
     (versión, salud, contrato, asistente, emparejamiento, `login`, `listPinUsers`, `pinLogin`) y el canal de eventos,
     la petición se rechaza con **401 `OFFLINE_CREDENTIAL_NOT_ALLOWED`** antes de atenderla, aunque traiga sesión
     (revisión R3). «Requiere tu revisión» siempre necesita al mesero con su PIN y conexión.
   - No es una sesión: no cuenta en «Sesiones activas por tipo de terminal» y no vence por tiempo.

La administración del PIN (asignar, cambiar, quitar y desbloquear, nombre corto y color del distintivo, «Mi PIN» y el
bloqueo por inactividad de cada dispositivo) es de la computadora principal y está en
[API_CONTRACTS.md](../API_CONTRACTS.md), «Etapa A1 · PIN de los meseros».

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

## Modo Mesero (etapa A1, fase E)

Lo que usa el modo Mesero además de las cuentas y pedidos de arriba (todo ya existía en el servidor y la etapa A1 lo
agregó al contrato; el escritorio no cambió). Capacidad `waiter-mode-v1`.

### Mesas

- **`getRestaurantOverview`** (`GET /restaurant/overview`, permisos `restaurant.tables.read` y
  `restaurant.areas.read`) → `{"areas", "tables", "settings", "timezone"}`:
  - `areas`: `id`, `name`, `description`, `sort_order` (orden visual), `active`, `revision`, `table_count`;
  - `tables`: además del estado, `shape` (`rectangle` o `round`), `x`, `y`, `width` y `height` en un **lienzo lógico
    de 1000 × 560 por área** (el cliente escala), `seats`, `occupied_since`, `occupied_seconds` (según la hora del
    servidor), `account_count`, `balance` y `long_open`;
  - `settings`: `warning_minutes`, `overdue_minutes` (la `urgency` de cada cuenta: `normal`, `warning`, `overdue`) y
    `long_open_hours`.
- El servidor **no restringe mesas ni cuentas por mesero**: con `restaurant.tables.read` se ven todas y con
  `accounts.open` se abre en cualquiera (`assigned_user_id` es informativo). El cliente deshabilita acciones según
  `user.permissions` de `getRecoveryState`.

### Cancelar y modificar unidades enviadas

Las partidas confirmadas (`lines` de la cuenta) se identifican por `version_id`; todas las escrituras llevan
`operation_id` y la `revision` vigente de la cuenta (409 `ACCOUNT_CONFLICT` si otra terminal la cambió), quedan
registradas con quién las hizo y cocina recibe su aviso (cancelación o modificación) como en el escritorio.

- **`cancelUnits`** (`POST /restaurant/accounts/{id}/cancel`, permiso **`orders.cancel`**; desde la 9H sin
  autorización de supervisor): `{"operation_id", "revision", "version_id", "quantity", "reason"}` (motivo de 5 a 500
  caracteres) → la cuenta (`Bill`).
- **`previewEdit`** (`POST /restaurant/accounts/{id}/edit-preview`, permiso **`orders.edit`**, sin escribir nada):
  `{"revision", "version_id", "line"}` con la solicitud nueva (`LineRequest`: **mismo producto**, otra cantidad o
  personalización) → `QuoteResult` con `fingerprint` y `pricing_policy`. Cambiar solo observaciones o reducir la
  cantidad conserva los importes históricos; aumentar o cambiar la receta recalcula esa partida con el catálogo
  vigente. **Reducir la cantidad exige además `orders.cancel`** (403).
- **`editUnits`** (`POST /restaurant/accounts/{id}/edit`): lo mismo con `"operation_id"`, `"expected_fingerprint"`
  (la de `previewEdit`; 409 `QUOTE_CHANGED` si cambió) y `"reason"` (5 a 500) → la cuenta.
- Errores propios: 409 `UNIT_HAS_PAYMENT` o `UNIT_HAS_DISCOUNT` (unidades con pago o descuento manual: primero la
  devolución o quitar el descuento) y 409 `ACCOUNT_BELOW_PAYMENTS`; con un producto agotado o no disponible al
  recalcular, los del catálogo (`PRODUCT_SOLD_OUT`…).

### Estados de preparación

- `preparation_state` de la cuenta: `without_items`, `draft` (solo borrador), `pending_release`, `scheduled`,
  `released`, `preparing`, `ready`, `change_pending` o `cancelled`; de cada partida confirmada (por versión y estado de
  sus unidades), los mismos desde `pending_release`. Son los del escritorio y la pantalla de cocina, calculados por Go.
- **`restaurant.account.preparation_changed`** (decisión A1-D11): en la misma transacción en que cambian los registros
  de cocina de una cuenta (avance a En preparación o Listo, liberar o programar un pedido, la liberación automática,
  un aviso de cambio pendiente o resuelto, y la integración con cocina de confirmar, modificar, cancelar o reprogramar)
  se publica **un solo evento por cuenta y por transacción** (aunque cambien muchas unidades o estaciones) en el
  ámbito **`restaurant`** (o `takeaway` si la cuenta es para llevar), con la cuenta como agregado. Lo recibe quien
  tiene `accounts.read` (o `takeaway.read`), **sin permisos de cocina**: el mesero vuelve a leer la cuenta y ve
  «Preparando» o «Listo» al momento. Las operaciones de la cuenta (confirmar, cancelar…) publican además su propio
  evento `restaurant.*`.

### Cuenta que ya no está abierta (§6.4)

Si la cuenta se cobró, se canceló o liberó, o se combinó con otra mientras se capturaba, `saveDraft`, `previewDraft`,
`confirmDraft`, `cancelUnits`, `previewEdit` y `editUnits` responden como siempre **400 `OPERATION_VALIDATION`**
(compatible con el escritorio), con el mensaje en español («La cuenta «Mesa 4» ya se cobró. No admite nuevas
operaciones.») y, desde la etapa A1, `error.details`:

```json
{
  "reason": "account_not_open",
  "account_id": "…",
  "account_state": "closed",
  "merged_into_id": null
}
```

`account_state` es `closed` (cobrada), `voided` (cancelada o liberada) o `merged` (con `merged_into_id`, la cuenta en
que se combinó). El cliente lleva el pedido a «Requiere tu revisión» por `details.reason`, no por el texto. En
`submitOfflineOrder` el mismo caso es el cambio `account_closed` (con `now` = el estado de la cuenta).

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
- `replaces_draft_items` (desde la etapa A1, opcional y **solo con `account_id`**; ver «Borrador del servidor y pedido
  sin conexión»): los renglones del borrador que el pedido sustituye, como `DraftItem`.
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
  - aceptables también (etapa A1): `draft_item_changed`, `draft_item_removed` y `draft_item_sent` (un renglón del
    borrador que el pedido sustituye ya no está como lo vio el dispositivo; ver abajo);
  - que exigen corregir el pedido (`requires_changes: true`): `product_sold_out`, `product_unavailable`,
    `product_removed`, `complement_unavailable`, `item_unavailable`, `item_invalid`, `account_closed` (la cuenta ya se
    cobró, se canceló o se combinó; desde la etapa A1, `now` dice cuál: `closed`, `voided` o `merged`),
    `table_unavailable`. El cliente quita o cambia la partida (o el destino) y reenvía.
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

### Borrador del servidor y pedido sin conexión (etapa A1, decisión A1-D6)

En línea, el teléfono captura como el escritorio: la cuenta nace con el primer producto (`openAccount`), el borrador se
guarda en Go (`saveDraft`, uno por cuenta, que cada guardado reemplaza completo) y «Enviar a cocina» es `confirmDraft`.
Si la red se corta después de guardar, el pedido que el mesero termina sin conexión ya contiene esos renglones:
enviarlo tal cual los duplicaría. Por eso, con `destination.account_id`, el pedido lleva en **`replaces_draft_items`**
los renglones del borrador que **ese dispositivo** había guardado y que el pedido sustituye, **tal como los vio** (`id`,
`product_id`, `variant_id`, `quantity`, `customization`; hasta 20, únicos; sin `account_id`, 400):

- **Al aceptar el pedido**, en la misma transacción, Go quita del borrador los que siguen exactamente así (producto,
  presentación, cantidad y personalización) y lo publica como un guardado del borrador (`restaurant.draft.saved`, con
  su historia y su Registro de movimientos), para que las demás terminales se enteren. **Lo demás del borrador** (lo
  que capturaron otras terminales) **no se toca**. La respuesta trae `removed_draft_item_ids` (los quitados) y la cuenta
  con su borrador restante.
- **Un renglón que ya no está como el dispositivo lo vio** no se quita ni se acepta en silencio: es un cambio más de
  `details.changes` (con `item_id` = el id del renglón, `product_id` y `product_name` del renglón que vio):
  - `draft_item_changed`: otra terminal lo cambió («Tacos al pastor: lo cambiaron en otra terminal. Si confirmas, tu
    pedido entra como lo capturaste y el cambio de la otra terminal se queda sin enviar en la cuenta.»);
  - `draft_item_removed`: lo quitaron del borrador («…: lo quitaron de la cuenta en otra terminal…»);
  - `draft_item_sent`: **ya se envió a cocina** («Tacos al pastor: ya se envió desde otra terminal. Si confirmas, tu
    pedido entra completo y puede repetirlo.»; si lo envió el mismo dispositivo, «ya se había enviado desde este
    dispositivo»). Go lo sabe porque cada partida confirmada guarda el id del renglón con que se capturó
    (`restaurant_items.draft_item_id`, migración 0025, sección E).
- Los tres son **aceptables**: el mesero los confirma como un cambio de precio (misma `operation_id`, `confirmation`
  con los cambios del rechazo) y su pedido entra como lo capturó; el renglón de la otra terminal se queda como está.
  O corrige su pedido (quita la partida, y el renglón de `replaces_draft_items`) y lo reenvía.
- Idempotencia y doble envío como cualquier pedido sin conexión (un reintento después de aceptado devuelve la misma
  respuesta y no quita nada más; `OFFLINE_ORDER_ALREADY_ACCEPTED` con otro contenido). Funciona igual con la sesión o
  con la credencial de envío.
- Recomendación para la aplicación: guardar con cada renglón el contenido con que el servidor lo aceptó en el último
  `saveDraft` correcto (eso es lo que «vio») y enviar en `replaces_draft_items` solo esos; los renglones nuevos
  capturados sin conexión van solo en `items`.

## Códigos de error más frecuentes

| Código                                                   | Estado | Qué hacer                                                                                                           |
| -------------------------------------------------------- | ------ | ------------------------------------------------------------------------------------------------------------------- |
| `CLIENT_UPDATE_REQUIRED`                                 | 426    | Actualizar la aplicación de este equipo.                                                                            |
| `DEVICE_REVOKED`, `DEVICE_REQUIRED`                      | 401    | Emparejar la terminal otra vez.                                                                                     |
| `PAIRING_EXPIRED`, `PAIRING_REJECTED`                    | 409    | Pedir otro código de emparejamiento.                                                                                |
| `PAIRING_RATE_LIMITED`, `PAIRING_LIMIT_REACHED`          | 429    | Esperar unos minutos: límite por origen o tope global de solicitudes (30 cada 10 min).                              |
| `SESSION_REPLACED`, `SESSION_EXPIRED`, `SESSION_REVOKED` | 401    | Iniciar sesión otra vez.                                                                                            |
| `PERMISSION_DENIED`                                      | 403    | El usuario no tiene el permiso.                                                                                     |
| `OPERATION_VALIDATION`                                   | 400    | Datos inválidos; con `details.reason: account_not_open`, la cuenta ya no está abierta.                              |
| `ACCOUNT_CONFLICT`                                       | 409    | Otra terminal cambió la cuenta: recargarla.                                                                         |
| `QUOTE_CHANGED`                                          | 409    | El cálculo cambió: volver a calcular y confirmar.                                                                   |
| `UNIT_HAS_PAYMENT`, `UNIT_HAS_DISCOUNT`                  | 409    | Las unidades tienen pago o descuento: no se cancelan ni modifican desde el dispositivo.                             |
| `TABLE_HAS_ACCOUNT`                                      | 409    | La mesa ya tiene una cuenta abierta: elegir.                                                                        |
| `PRODUCT_SOLD_OUT`                                       | 409    | El producto está agotado.                                                                                           |
| `PRODUCT_UNAVAILABLE`                                    | 409    | El producto no está disponible (pausado, fuera de horario o inactivo).                                              |
| `ORDER_REQUIRES_CONFIRMATION`                            | 409    | Mostrar los cambios al mesero y reenviar con su confirmación.                                                       |
| `IDEMPOTENCY_CONFLICT`                                   | 409    | La clave ya se usó con otro contenido: consultar el resultado.                                                      |
| `OFFLINE_ORDER_ALREADY_ACCEPTED`                         | 409    | El pedido sin conexión ya entró: usar `details.result`, no capturarlo otra vez.                                     |
| `LOGIN_THROTTLED`                                        | 429    | Esperar 15 minutos.                                                                                                 |
| `PIN_INVALID`                                            | 401    | PIN incorrecto: volver a escribirlo.                                                                                |
| `PIN_LOCKED`                                             | 429    | PIN bloqueado hasta `details.locked_until`, o hasta que un administrador lo desbloquee (`details.requires_unlock`). |
| `PIN_LOGIN_BUSY`                                         | 503    | Muchas entradas con PIN a la vez: reintentar en unos segundos.                                                      |
| `CLIENT_KIND_MISMATCH`                                   | 400    | `X-Client-Kind` no es el tipo con que se emparejó el dispositivo.                                                   |
| `PIN_DEVICE_NOT_ALLOWED`                                 | 403    | La entrada con PIN solo existe en dispositivos `android` emparejados.                                               |
| `OFFLINE_CREDENTIAL_INVALID`                             | 401    | Conservar el pedido y pedir al mesero que entre con su PIN con conexión.                                            |
| `OFFLINE_CREDENTIAL_NOT_ALLOWED`                         | 401    | La credencial de envío solo se usa en `submitOfflineOrder`.                                                         |
