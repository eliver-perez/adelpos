# API v1 para clientes · Etapa 9G

Contrato de la API que usan la aplicación de escritorio y las terminales y la **aplicación Android de meseros**
(carpeta `android/`, etapa A1). Fase D de la etapa 9G (§5 de [etapas/ETAPA_9G.md](etapas/ETAPA_9G.md)); la etapa A1
agregó la versión mínima por tipo de cliente, la entrada con PIN y la credencial de envío
([etapas/ETAPA_A1_ANDROID.md](etapas/ETAPA_A1_ANDROID.md), §3) y, para el modo Mesero (§6–§8), las mesas, cancelar y
modificar unidades, el evento de preparación, el motivo de una cuenta que ya no está abierta, el borrador en el pedido
sin conexión y el cursor de eventos de la recuperación (ver «Modo Mesero»). La etapa A2
([etapas/ETAPA_A2_ANDROID.md](etapas/ETAPA_A2_ANDROID.md)) agregó **Para llevar** (servidor 0.13.0, ver «Para llevar») e
**imprimir la precuenta** desde el dispositivo en su impresora (esquema 26, ver «Imprimir la precuenta») y los
**avisos de platillos listos** (ver «Avisos de platillos listos»).

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
   `waiter-mode-v1` (lo del modo Mesero de la sección «Modo Mesero»); desde la etapa A2, `takeaway-v1` (lo de la
   sección «Para llevar»), `device-precheck-printer-v1` (lo de «Imprimir la precuenta») y `ready-alerts-v1` (lo de
   «Avisos de platillos listos»); desde la etapa A3, `permission-groups-v1`, que solo usa el escritorio (permisos
   agrupados y roles de fábrica, `API_CONTRACTS.md`); desde la etapa L1, `license-v1` (lo de la sección «Licencia del
   servidor principal»)). La aplicación Android 0.1.0 exige un servidor 0.12.0 o posterior; el escritorio 0.12.0 sigue
   funcionando con un servidor 0.11.0 y solo muestra lo del PIN si el servidor anuncia `waiter-pin-v1`.
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

| `operationId`            | Método y ruta (bajo `/api/v1`)                 | Para qué                                                                       |
| ------------------------ | ---------------------------------------------- | ------------------------------------------------------------------------------ |
| `getVersion`             | `GET /version`                                 | Versión del servidor, `min_client_version`, tipos de cliente y funciones.      |
| `getReadiness`           | `GET /health/ready`                            | Servidor y base de datos listos (200) o no (503).                              |
| `getOpenAPI`             | `GET /openapi.json`                            | Este contrato.                                                                 |
| `login`                  | `POST /auth/login`                             | Iniciar sesión en una terminal emparejada.                                     |
| `logout`                 | `POST /auth/logout`                            | Cerrar la sesión.                                                              |
| `getIdentity`            | `GET /auth/me`                                 | Usuario, permisos vigentes, preferencias, terminal y establecimiento.          |
| `listPinUsers`           | `GET /auth/pin-users`                          | Meseros que pueden entrar con PIN en el dispositivo (sin sesión, etapa A1).    |
| `pinLogin`               | `POST /auth/pin-login`                         | Entrar con PIN: sesión y credencial de envío (etapa A1).                       |
| `streamEvents`           | `GET /events`                                  | Eventos en tiempo real (SSE).                                                  |
| `getReadyAlerts`         | `GET /ready-alerts`                            | Avisos de platillos listos desde un cursor (credencial del dispositivo, A2).   |
| `streamReadyAlerts`      | `GET /ready-alerts/stream`                     | Canal SSE de los avisos de platillos listos (credencial del dispositivo, A2).  |
| `getCatalogSync`         | `GET /catalog/sync`                            | Catálogo descargable completo o «solo cambios desde la versión X».             |
| `listCatalogProducts`    | `GET /catalog/products`                        | Productos para la captura con disponibilidad, «Agotado» y precio de una lista. |
| `getCatalogImage`        | `GET /catalog/images/{id}`                     | Imagen en JSON base64 (la usa el escritorio).                                  |
| `getCatalogImageContent` | `GET /catalog/images/{id}/content`             | Imagen binaria con `ETag` (304 con `If-None-Match`).                           |
| `setProductSoldOut`      | `POST /catalog/products/{id}/sold-out`         | «Agotado» y «Disponible otra vez».                                             |
| `getRecoveryState`       | `GET /restaurant/state`                        | Estado al reconectar: permisos, mesas, cuentas, preparación y cursor.          |
| `getRestaurantOverview`  | `GET /restaurant/overview`                     | Áreas, mesas (lista y mapa) y umbrales de tiempo (etapa A1).                   |
| `listAccounts`           | `GET /restaurant/accounts`                     | Cuentas con sus estados.                                                       |
| `openAccount`            | `POST /restaurant/accounts`                    | Abrir una cuenta con su primer producto.                                       |
| `getAccount`             | `GET /restaurant/accounts/{id}`                | Cuenta con su borrador y sus partidas confirmadas.                             |
| `saveDraft`              | `PUT /restaurant/accounts/{id}/draft`          | Guardar el borrador.                                                           |
| `previewDraft`           | `GET /restaurant/accounts/{id}/preview`        | Calcular el borrador y obtener su huella.                                      |
| `confirmDraft`           | `POST /restaurant/accounts/{id}/confirm`       | Confirmar (enviar a cocina) el borrador.                                       |
| `cancelUnits`            | `POST /restaurant/accounts/{id}/cancel`        | Cancelar unidades enviadas, con motivo (etapa A1 en el contrato).              |
| `previewEdit`            | `POST /restaurant/accounts/{id}/edit-preview`  | Calcular la modificación de una partida enviada y su huella.                   |
| `editUnits`              | `POST /restaurant/accounts/{id}/edit`          | Modificar una partida enviada, con motivo.                                     |
| `updateTakeawayCustomer` | `POST /restaurant/accounts/{id}/customer`      | Nombre del cliente y teléfono de un pedido para llevar (etapa A2).             |
| `discardAccount`         | `POST /restaurant/accounts/{id}/discard`       | Descartar sin motivo una cuenta vacía (etapa A3 en el contrato).               |
| `submitOfflineOrder`     | `POST /restaurant/offline-orders`              | Pedido capturado sin conexión (Android; con sesión o credencial de envío).     |
| `printPrecheck`          | `POST /payments/accounts/{id}/precheck/print`  | Imprimir la precuenta (etapa A2 en el contrato).                               |
| `getDocumentStatus`      | `GET /payments/accounts/{id}/documents/status` | Estado de impresión de una precuenta, sin importes (etapa A2).                 |
| `retryDocument`          | `POST /payments/accounts/{id}/documents/retry` | Reintentar sin motivo una precuenta que no salió (etapa A2 en el contrato).    |
| `deliverTakeaway`        | `POST /payments/accounts/{id}/deliver`         | Marcar entregado un pedido para llevar ya pagado (etapa A2 en el contrato).    |
| `createPairingRequest`   | `POST /pairing/requests`                       | Pedir un código de emparejamiento (sin credencial).                            |
| `revealPairingNonce`     | `POST /pairing/requests/{id}/reveal`           | Revelar el número de la terminal después de recibir el del servidor.           |
| `getPairingRequest`      | `GET /pairing/requests/{id}`                   | Consultar si la computadora principal ya aprobó el código.                     |
| `renamePairingRequest`   | `POST /pairing/requests/{id}/name`             | Corregir el nombre propuesto mientras espera.                                  |
| `cancelPairingRequest`   | `POST /pairing/requests/{id}/cancel`           | Dejar de esperar la aprobación.                                                |
| `claimPairingCredential` | `POST /pairing/requests/{id}/claim`            | Recibir una sola vez la credencial de la terminal.                             |
| `getLicenseStatus`       | `GET /license/status`                          | Estado de la licencia del servidor principal (etapa L1; también sin sesión).   |
| `setKitchenDisplayMode`  | `POST /kitchen/display-mode`                   | Entrar o salir de Preparación: ocupa o libera el cupo de cocina (etapa L1).    |

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
   - `terminal`: la terminal (una terminal revocada no llega aquí: 401 `DEVICE_REVOKED`); desde la etapa A2, con
     `precheck_printer_id` y `precheck_printer` (`{id, name, terminal_name}` o `null`), la impresora de precuentas del
     dispositivo (ver «Imprimir la precuenta»), y `ready_alerts_enabled`, `ready_alerts_sound` y
     `ready_alerts_vibration` (ver «Avisos de platillos listos»);
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
     `orders.confirm`, `orders.edit`, `orders.cancel`, `catalog.read`, `catalog.prices.read`, `events.read` y, desde la
     etapa A2, `takeaway.read`, `takeaway.manage` y `takeaway.deliver` (Para llevar) y `payments.tickets.print`
     (imprimir la precuenta). `getIdentity` y
     `getRecoveryState` informan ya los permisos acotados; cualquier otra ruta responde 403 como sin el permiso. La
     lista vive en un solo lugar del servidor (`administration/waiter_scope.go`) y solo crece con las funciones de la
     aplicación (en la A2: Para llevar e impresión de la precuenta). Cambiar la contraseña desde esa sesión no la
     amplía.
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
  su `preparation_state`. Desde la etapa A3 el contrato documenta además `discardable` y `releasable` (Go ya los
  enviaba): ver «Descartar una cuenta vacía».
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

### Descartar una cuenta vacía (etapa A3 en el contrato, A3-D8)

Existe desde la etapa 9D y el escritorio y la aplicación la usaban; la etapa A3 la agrega al contrato sin cambiar su
comportamiento.

- **`discardable`** y **`releasable`** de cada cuenta los calcula Go en cada lectura: `discardable` es `true` si la
  cuenta está abierta y **sin productos enviados, pagos, ajustes, documentos ni registros de cocina**, aunque tenga un
  borrador sin enviar; `releasable` es lo mismo y además sin borrador (la cuenta vacía en sentido estricto, «Liberar
  mesa» del escritorio). El cliente no los deduce.
- **`discardAccount`** (`POST /restaurant/accounts/{id}/discard`, permisos **`accounts.open`** y `accounts.read`; en
  un pedido para llevar, además `takeaway.manage`): `{"operation_id", "revision"}` → la cuenta (`Bill`) anulada
  (`state` `voided`) y sin borrador, que se borra en la misma transacción. **No es una cancelación**: no pide motivo ni
  autorización y no aparece en «Cuentas canceladas» ni en otros reportes. Se publica `restaurant.account.discarded`.
- Errores: 409 `ACCOUNT_NOT_EMPTY` si la cuenta ya tiene productos enviados, pagos, ajustes, documentos o registros
  de cocina (se cancela con su motivo desde el escritorio); 409 `ACCOUNT_CONFLICT` si la revisión cambió; 400
  `OPERATION_VALIDATION` con `details.reason` `account_not_open` si la cuenta ya no está abierta (§6.4, abajo).
  Idempotente con `operation_id`.

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
`accounts.read`, `catalog.read`, `catalog.prices.read`, para abrir una cuenta en una mesa `accounts.open` y, para un
pedido para llevar nuevo o una cuenta para llevar, además `takeaway.manage`):

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

- `destination`: **uno solo de** una cuenta abierta (`account_id`), una mesa donde abrir una cuenta nueva (`table_id`,
  con las cuentas que el mesero veía abiertas en ella y un `name` opcional) o, desde la etapa A2, un pedido para
  llevar nuevo (`takeaway`, con `customer_name` y `phone` opcionales; ver «Para llevar»). Dos destinos, ninguno, o
  `seen_account_ids`/`name` sin `table_id`: 400.
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
    borrador que el pedido sustituye ya no está como lo vio el dispositivo, o una partida ya se envió a cocina; ver
    abajo);
  - que exigen corregir el pedido (`requires_changes: true`): `product_sold_out`, `product_unavailable`,
    `product_removed`, `complement_unavailable`, `item_unavailable`, `item_invalid`, `account_closed` (la cuenta ya se
    cobró, se canceló o se combinó; desde la etapa A1, `now` dice cuál: `closed`, `voided` o `merged`),
    `table_unavailable` y, desde A1-D18, `items_in_account` (ver abajo). El cliente quita o cambia la partida (o el
    destino) y reenvía.
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
  usuario u otra terminal, la misma clave es `IDEMPOTENCY_CONFLICT` sin datos. Desde la etapa A1, `details.origin`
  dice qué entró con esa clave (`offline_order` o `confirm_draft`; ver «El servidor nunca repite lo que ya entró»).
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
  - `draft_item_sent`: **ya se envió a cocina** («Tacos al pastor: ya se envió a cocina desde otra terminal y no se
    vuelve a enviar. Si confirmas, se envía lo demás del pedido.»; si lo envió el mismo dispositivo, «ya se había
    enviado a cocina desde este dispositivo…»). Go lo sabe porque cada partida confirmada guarda el id del renglón con que se capturó
    (`restaurant_items.draft_item_id`, migración 0025, sección E).
- Los tres son **aceptables**: el mesero los confirma como un cambio de precio (misma `operation_id`, `confirmation`
  con los cambios del rechazo) y su pedido entra como lo capturó; el renglón de la otra terminal se queda como está.
  O corrige su pedido (quita la partida, y el renglón de `replaces_draft_items`) y lo reenvía. Desde A1-D18,
  confirmar `draft_item_sent` **envía lo demás**: lo ya enviado nunca se vuelve a enviar (ver abajo).
- Idempotencia y doble envío como cualquier pedido sin conexión (un reintento después de aceptado devuelve la misma
  respuesta y no quita nada más; `OFFLINE_ORDER_ALREADY_ACCEPTED` con otro contenido). Funciona igual con la sesión o
  con la credencial de envío.
- Recomendación para la aplicación: guardar con cada renglón el contenido con que el servidor lo aceptó en el último
  `saveDraft` correcto (eso es lo que «vio») y enviar en `replaces_draft_items` solo esos; los renglones nuevos
  capturados sin conexión van solo en `items`. **Cada partida del pedido usa el id de su renglón** (el que generó el
  dispositivo al capturarlo): es lo que permite reconocerlo en el borrador o entre lo ya enviado.

### El servidor nunca repite lo que ya entró (etapa A1, decisión A1-D18)

Después de la revisión independiente de la cola sin conexión (R6), `submitOfflineOrder` reconoce lo que ya entró aunque
la respuesta de la operación anterior se haya perdido:

1. **La misma clave que un «Enviar a cocina» ya aplicado**: si `confirmDraft` entró con la `operation_id` K y su
   respuesta se perdió, el dispositivo manda el pedido de la cola con **la misma K** (A1-D19). Si K entró como
   `confirmDraft` **en la misma cuenta** (`destination.account_id`), **del mismo usuario y el mismo dispositivo**
   (sesión o credencial de envío), la respuesta es **409 `OFFLINE_ORDER_ALREADY_ACCEPTED`** sin mandar nada:

   ```json
   {
     "error": {
       "code": "OFFLINE_ORDER_ALREADY_ACCEPTED",
       "message": "Este pedido ya había entrado al enviarlo a cocina y no se volvió a registrar.",
       "details": {
         "status": "already_accepted",
         "origin": "confirm_draft",
         "result": {
           "status": "accepted",
           "operation_id": "K",
           "order_id": "pedido de la confirmación",
           "account_id": "la cuenta",
           "catalog_version": 52,
           "captured_catalog_version": 51,
           "captured_at": "la del pedido",
           "received_at": "cuando entró la confirmación",
           "accepted_changes": [],
           "removed_draft_item_ids": [],
           "already_sent_item_ids": ["partidas del pedido que estaban en esa confirmación"],
           "bill": { "…": "la cuenta vigente" }
         }
       }
     }
   }
   ```

   El dispositivo marca el pedido «Recibido por el restaurante» con ese resultado. La misma K **hacia otra cuenta o a
   una mesa, desde otro usuario u otro dispositivo, o guardada por otra operación** (guardar el borrador, abrir la
   cuenta…) sigue siendo **409 `IDEMPOTENCY_CONFLICT`** sin datos de nadie.

2. **Renglones del borrador con el id de una partida**: se tratan como sustituidos aunque no vengan en
   `replaces_draft_items` (por ejemplo, un guardado que entró en Go sin que el dispositivo recibiera la respuesta). Si
   el renglón está igual que la partida, o lo guardó por última vez **el mismo mesero desde el mismo dispositivo**
   (entonces lo que hay en Go es suyo, aunque lo cambiara después sin conexión), se quita en la misma transacción y
   aparece en `removed_draft_item_ids`; si otra terminal lo dejó distinto, `draft_item_changed` (aceptable, como antes).
   La misma regla del «último guardado propio» aplica a los renglones de `replaces_draft_items`.
3. **Partidas ya enviadas** (`draft_item_sent`): una partida del pedido —o un renglón de `replaces_draft_items`— cuyo
   id ya está en una partida confirmada (`restaurant_items.draft_item_id`) es `draft_item_sent`, aceptable, con el
   mensaje «Papas: ya se envió a cocina desde otra terminal y no se vuelve a enviar. Si confirmas, se envía lo demás del
   pedido.» (o «ya se había enviado a cocina desde este dispositivo…»). **Confirmarlo envía lo demás**: esas partidas
   no se cotizan ni se vuelven a enviar y la respuesta 200 las lista en **`already_sent_item_ids`**. Si **no queda
   nada**, la respuesta sigue siendo 200 `status: "accepted"`, sin pedido nuevo: `already_sent_item_ids` trae todas las
   partidas y **`order_id` es el pedido en que ya habían entrado**; la cuenta no cambia (ni su revisión).
4. **Pedido a una mesa cuyas partidas ya están en una cuenta abierta de esa mesa** (en su borrador o ya enviadas; por
   ejemplo, la apertura con el primer producto entró y su respuesta se perdió): en lugar del 400 genérico, 409
   `ORDER_REQUIRES_CONFIRMATION` (`requires_changes: true`) con el cambio **`items_in_account`** (`acceptable:
false`, **`account_id`** = esa cuenta, `now` = su folio, `item_id` y `product_id` en `null`), sin
   `table_has_account`. El dispositivo reenvía **con la misma `operation_id`** a `destination.account_id` = esa
   cuenta; ahí aplican los puntos 2 y 3. Desde la etapa A2, lo mismo con un **pedido para llevar nuevo**
   (`destination.takeaway`) cuyas partidas ya están en un pedido para llevar abierto (ver «Para llevar»).

## Para llevar (etapa A2, parte B)

Capacidad **`takeaway-v1`** (servidor 0.13.0). Un pedido para llevar es **una cuenta con `mode: "takeaway"` y sin
mesa** (`table_id: null`; no hay mesa ficticia): las mismas rutas de cuentas, borrador, «Enviar a cocina», cancelar y
modificar unidades valen para él. En una cuenta para llevar toda lectura exige además **`takeaway.read`** y toda
escritura **`takeaway.manage`** (403 sin ellos); la lista cerrada de permisos de mesero los incluye desde esta etapa
(ver «Entrada con PIN y credencial de envío»). El rol Mesero de una instalación no los trae: los marca el
administrador (`docs/ANDROID_INSTALAR.md`).

1. **«Nuevo pedido» con conexión** (como la caja): `openAccount` con `mode: "takeaway"`, `origin: "Para llevar"`,
   `customer_name` y `phone` (opcionales) e `items` con el primer producto (sin `table_id` ni `seen_account_ids`; sin
   `name`, Go pone «Pedido para llevar»). Exige `accounts.open`, `takeaway.manage`, `orders.capture`, `catalog.read` y
   `catalog.prices.read`. El responsable (`assigned_user_id`) es quien lo abre. Después, `saveDraft`, `previewDraft` y
   `confirmDraft` como cualquier cuenta.
2. **Sin conexión**: `submitOfflineOrder` con **`destination.takeaway`**:

   ```json
   {
     "operation_id": "una por pedido de la cola",
     "catalog_version": 41,
     "captured_at": "2026-11-18T20:15:00Z",
     "destination": { "takeaway": { "customer_name": "Juan", "phone": "5551234567" } },
     "items": [
       {
         "id": "…",
         "product_id": "…",
         "variant_id": null,
         "quantity": 1,
         "customization": {},
         "seen_unit_price": "95.00"
       }
     ]
   }
   ```

   - `customer_name` (hasta 100) y `phone` (hasta 40) son opcionales y se recortan; más largos, 400. `takeaway` excluye
     `account_id`, `table_id`, `seen_account_ids`, `name` y `replaces_draft_items` (400).
   - Exige además **`accounts.open`** y **`takeaway.manage`** (403 sin ellos, antes de revisar el pedido). Con la
     credencial de envío valen los permisos de mesero vigentes del usuario.
   - Aceptado (200, `OfflineOrderResult`): abre la cuenta para llevar (`mode: "takeaway"`, `origin: "Para llevar"`,
     el cliente y el teléfono del destino, `name: "Pedido para llevar"`, responsable = el mesero de la sesión o de la
     credencial) y la confirma en la misma operación; cocina la recibe. `bill.account` trae su `folio`.
   - **La misma clave nunca abre dos pedidos**: un reintento con la misma `operation_id` y el mismo contenido (con la
     sesión o con la credencial) devuelve la misma respuesta; con otro contenido, `409 OFFLINE_ORDER_ALREADY_ACCEPTED`
     (`details.origin: "offline_order"`, `details.result` con el pedido que entró); desde otro usuario o dispositivo,
     `409 IDEMPOTENCY_CONFLICT`. Los cambios de precio o de catálogo se confirman igual que en una mesa.
   - **Apertura en línea sin respuesta** (A1-D18): si las partidas ya están en un pedido para llevar abierto (en su
     borrador o ya enviadas), `409 ORDER_REQUIRES_CONFIRMATION` con `requires_changes: true` y el cambio
     **`items_in_account`** (`acceptable: false`, `account_id`, `now: "folio 12"`, `message: "Lo que capturaste ya está
en el pedido para llevar de Juan (folio 12). Envía el pedido a esa cuenta."`; sin nombre del cliente, «…en el pedido
     para llevar (folio 12)…»). El dispositivo lo reenvía **con la misma `operation_id`** a
     `destination.account_id` = esa cuenta (ahí aplican `replaces_draft_items`, el último guardado propio y
     `draft_item_sent`).

3. **Nombre y teléfono en cualquier momento** (decisión A2-D1): **`updateTakeawayCustomer`**
   (`POST /restaurant/accounts/{id}/customer`, `takeaway.manage`) `{"operation_id", "customer_name", "phone"}` → la
   cuenta (`Bill`).
   - **Sin revisión**: no la pide ni la sube (solo cambia esos dos datos), así que no choca con el borrador que se
     guarda al mismo tiempo; si se editan en dos lugares, gana la última edición.
   - Los dos campos son obligatorios (vacío **quita** el dato; sin uno de los dos, 400); se recortan y caben 100 y 40
     caracteres (400 si no). Un campo de más (por ejemplo `revision`): 400.
   - Solo pedidos para llevar abiertos: una cuenta de salón, `400 OPERATION_VALIDATION`; un pedido cobrado y
     entregado, cancelado o combinado, `400 OPERATION_VALIDATION` con `details.reason: "account_not_open"`
     (`AccountNotOpenDetails`).
   - Idempotente con `operation_id` (otro contenido con la misma clave: `409 IDEMPOTENCY_CONFLICT`). Publica
     `restaurant.account.updated` en el ámbito `takeaway` y queda en la historia de la cuenta y en el Registro de
     movimientos (`restaurant.account.updated`); si los datos ya eran esos, no registra nada.
   - Editados sin conexión en un pedido que ya está en Go, el dispositivo los guarda y los manda al volver la conexión
     con su propia `operation_id`.
4. **Pendientes**: `listAccounts?mode=takeaway&state=open` (con `takeaway.read`; sin él, 403) o las cuentas de
   `getRecoveryState`, que incluyen los pedidos para llevar abiertos solo con `takeaway.read`. Cada cuenta trae
   `folio`, `customer_name`, `phone`, `origin`, `elapsed_seconds` y `urgency` (con la hora del servidor), y los tres
   estados, independientes y calculados por Go:
   - `preparation_state`: «Pendiente» = `pending_release`, `scheduled` o `released`; «En preparación» = `preparing`;
     «Listo» = `ready` (más `without_items`, `draft`, `change_pending` y `cancelled`, como en «Estados de
     preparación»);
   - `payment_state`: `not_recorded` (sin pagar), `partial` (con abonos) o `paid` (pagado);
   - `delivery_state`: `not_delivered` o `delivered`. Un pedido pagado y entregado (sin nada por enviar) se cierra y
     sale de la lista.

   Los cambios llegan por `streamEvents` en el ámbito **`takeaway`** (que exige `takeaway.read`), con la cuenta como
   agregado: `restaurant.account.opened`, `restaurant.order.confirmed`, `restaurant.account.updated`,
   `restaurant.draft.saved`, `restaurant.account.preparation_changed`, `takeaway.delivered`, `restaurant.account.closed`,
   etc. El cliente vuelve a leer la cuenta (o la lista).

5. **«Entregado»** (decisión A2-D2): **`deliverTakeaway`** (`POST /payments/accounts/{id}/deliver`,
   `{"operation_id"}`; permisos `takeaway.deliver`, `accounts.read` y `takeaway.read`). **Go no entrega un pedido sin
   pagar**:
   - con saldo pendiente: **`409 BALANCE_PENDING`** («El pedido tiene saldo pendiente. Cóbralo antes de marcarlo como
     entregado.») y no registra nada;
   - ya entregado: **`409 ALREADY_DELIVERED`**; una cuenta de salón: 400;
   - entregado: `200 {"delivered": true, "closed": <bool>, "finance": <estado financiero completo o null>}`. Pagado,
     entregado y sin nada por enviar en su borrador, la cuenta se cierra (`closed: true`; con borrador queda abierta
     y entregada). **`finance` solo llega con `payments.read`**: a la sesión con PIN
     de un mesero (que nunca lo tiene) llega `null`, para no exponer los pagos;
   - idempotente con `operation_id` (el reintento devuelve lo mismo). Publica `takeaway.delivered` (y, al cerrarse,
     `restaurant.account.closed`) en el ámbito `takeaway`.

   El dispositivo ofrece «Entregado» solo con el permiso y `payment_state == "paid"`; sin pagar muestra «Cobrar en caja
   antes de entregar» (también si llega `BALANCE_PENDING` porque otra caja dejó saldo). El mesero no cobra: el cobro
   sigue en la caja.

## Imprimir la precuenta (etapa A2, parte C)

Capacidad **`device-precheck-printer-v1`** (servidor 0.13.0, esquema 26; decisión A2-D3). El mesero imprime la
precuenta de una mesa o de un pedido para llevar **en la impresora asignada a su dispositivo** en Administración ›
Sistema › Terminales › Editar («Impresora de precuentas»: una de red o la USB de una terminal con agente de impresión).
Permiso **`payments.tickets.print`** («Imprimir tickets y precuentas»; está en la lista de permisos de mesero), además
de `accounts.read` (y `takeaway.read` en un pedido para llevar). **Imprimir requiere conexión**: nunca entra en la cola
sin conexión.

1. **La impresora del dispositivo**: `terminal.precheck_printer` de `getIdentity` y `getRecoveryState` (`null` si no
   tiene). Un dispositivo `android` imprime **solo** en ella: sin ella **no** pasa a la impresora de tickets de Cobros ni
   a la de una caja. Las terminales de escritorio siguen como hasta ahora (la de su caja o la general).
2. **El nombre visible**: el botón dice «Imprimir precuenta», «Imprimir ticket de venta» o «Imprimir cuenta» según
   `establishment.precheck_name` de `getIdentity` (`precheck`, `sales_ticket` o `bill`); el impreso lo pone Go con el
   mismo nombre (PRECUENTA, TICKET DE VENTA o CUENTA), con la misma plantilla y el mismo renderizador de la caja, y
   «Atendió: <quien la imprime>».
3. **`printPrecheck`** (`POST /payments/accounts/{id}/precheck/print`, `{"operation_id"}`) → **200**
   `{"requested": true, "status", "job_id", "ticket_id", "message", "problem", "printer_name", "terminal_name"}`.
   Cada toque arma **una precuenta nueva** con el estado vigente de la cuenta (como en la caja): no hay «copia» ni
   motivo. `problem` es lo que Go ya sabe al encolar: **`agent_offline`** si la impresora es la USB de una terminal cuyo
   agente no consulta desde hace más de 30 s (el trabajo espera y sale cuando vuelve) o `null`. `printer_name` es la
   impresora y `terminal_name` su terminal (si es la USB de una terminal; `null` en una de red). Rechazos (no se encola
   nada, esquema `PrinterRejection`):
   - **409 `PRINTER_NOT_CONFIGURED`** con `details: {"document": "precheck", "problem": "missing", "scope": "device"}` y
     el mensaje «Este dispositivo no tiene impresora asignada. Pide al administrador que la elija en Terminales.»;
   - **409 `PRINTER_UNAVAILABLE`** con `details: {"document", "problem": "inactive" | "unsupported", "printer_name",
"scope": "device"}` si la impresora asignada dejó de servir;
   - 403 sin `payments.tickets.print`; 409 `IDEMPOTENCY_CONFLICT` con la misma clave y otro contenido.
4. **`getDocumentStatus`** (`GET /payments/accounts/{id}/documents/status?ticket_id=<de printPrecheck>`, permiso
   `payments.tickets.print`; **sin importes ni pagos**) → `{"ticket_id", "kind", "payment_id", "created_at", "job_id",
"job_state", "job_problem", "printer_name", "copies", "terminal_name"}`. Para seguir la precuenta unos segundos
   después de mandarla (el trabajador de la cola intenta cada segundo y tarda unos 4 s en saber que una impresora de red
   no responde): `job_problem` `unreachable` («La impresora no responde»), `agent_offline` («La Caja 2 está
   desconectada», con `terminal_name`), `uncertain` (revisar el papel antes de pedir otra), `not_found`, `inactive`,
   `unsupported` o `setup`; `job_state` `submitted` es que la impresora o su cola lo aceptó. 404 si el documento no es de
   esa cuenta; 400 si `ticket_id` no es un identificador.
5. **`retryDocument`** (`POST /payments/accounts/{id}/documents/retry`, `{"operation_id", "ticket_id", "job_id"?}`, permiso
   `payments.tickets.print`): reintenta **sin motivo** una precuenta cuyo último trabajo nunca salió en papel (`error` o
   `intervention`; regla de la 9F), en la impresora del dispositivo (la misma resolución: sin ella, el mismo 409
   `PRINTER_NOT_CONFIGURED` con `scope: "device"`). Responde como `printPrecheck`. Sin un trabajo fallido, 409
   `PRINT_NOT_FAILED`. Con `uncertain` no se reintenta: se revisa el papel y, si hace falta, se pide otra precuenta.

## Avisos de platillos listos (etapa A2, parte D)

Capacidad **`ready-alerts-v1`** (servidor 0.13.0, esquema 26; decisiones A2-D4 y A2-D9, con las correcciones de la
revisión independiente). Cuando cocina marca «Listo» en el KDS (una unidad, el pedido de una estación o la cuenta
completa), o cuando una unidad queda con todas sus partes listas porque en esa misma operación se canceló la que
faltaba, Go registra cada unidad que pasó a «Listo» en la misma transacción que el evento
`restaurant.account.preparation_changed` (tabla `app.ready_notices`, una vez por unidad y versión) con el **responsable
de la cuenta en ese momento**. El dispositivo los lee con **solo su credencial** (`X-Device-ID` y `X-Device-Token`, sin
sesión: llegan aunque esté en «¿Quién eres?» o con la aplicación cerrada, desde su servicio en primer plano). Si la
cocina no usa el KDS, no hay avisos.

1. **A quién llega** (lo decide Go). El mesero M **está en** el dispositivo D si tiene en D una sesión con PIN
   **creada desde el inicio del día comercial en curso** (la «Hora de inicio del día» de Administración), o una de
   antes que **sigue vigente y se usó (alguna petición con esa sesión) en los últimos 30 minutos** (decisión A2-D12: cubre al mesero que trabaja justo en
   el cambio de día sin que la última entrada de la noche, que dura 12 h, mantenga el servicio activo hasta la mañana);
   es la misma regla que `active`. Un aviso de M llega a D si:
   - M está en D y el aviso es **posterior a la entrada de M en D** (su primera sesión con PIN en D que cuente): un
     dispositivo nuevo, o una lectura sin cursor, no recibe lo que se marcó «Listo» antes de que M entrara en él;
   - y el aviso quedó **para D o para todos**: al registrar el «Listo», Go lo fija al dispositivo de la sesión con PIN
     vigente de actividad más reciente de M que cuente (con la misma regla), pero solo entre dispositivos `android` no
     retirados, con «Avisar platillos listos» activado y, si Go conoce su versión, 0.2.0 o posterior. Si ninguno sirve (por ejemplo, M cerró su sesión
     con `logout` o su dispositivo tiene el aviso apagado), el aviso es para todos los dispositivos donde M está.

   Cerrar la sesión con `logout` hace que los «Listo» siguientes ya no se fijen a ese dispositivo; como M entró en él
   hoy, sigue recibiendo los que quedan para todos hasta el fin del día comercial. Una entrada con PIN **sin conexión**
   no le dice nada a Go: ese mesero no recibe avisos en ese dispositivo hasta su primera entrada con conexión.
   Transferir la cuenta cambia el destinatario de lo que todavía no estaba listo.

2. **`getReadyAlerts`** (`GET /ready-alerts?after=<cursor>`) → **`ReadyAlertsBatch`**:

   ```json
   {
     "cursor": 812,
     "active": true,
     "day_ends_at": "2026-11-19T11:00:00Z",
     "settings": { "enabled": true, "sound": true, "vibration": false },
     "alerts": [
       {
         "id": 812,
         "key": "5f0c2d6e-3b1a-4c8e-9f27-6d4b1a0e7c35",
         "account_id": "…",
         "mode": "dine_in",
         "table_name": "Mesa 4",
         "folio": 31,
         "customer_name": "",
         "waiter_user_id": "…",
         "waiter_short_name": "Ana",
         "items": [{ "product_name": "Filete · Grande", "quantity": 2 }],
         "ready_at": "2026-11-18T20:41:07.12Z"
       }
     ]
   }
   ```

   - Un aviso es lo que cocina marcó «Listo» en una cuenta **en una misma operación**, agrupado por platillo
     (`product_name` es el nombre visible del producto con su presentación, `quantity` las unidades). En un pedido para
     llevar `table_name` es `null` y `customer_name` el nombre capturado. **Nunca trae precios ni importes.**
   - **`key`** es la clave del aviso: única y **nunca se reutiliza**, ni siquiera tras restaurar un respaldo. El
     dispositivo descarta por `key` los que ya avisó (guarda las del día).
   - **`id`** es solo el cursor (crece con él; tras restaurar un respaldo un número puede repetirse con otro aviso).
     **`cursor`** es el último aviso que Go revisó para el dispositivo, le corresponda o no: el dispositivo lo guarda y
     vuelve con `after=<cursor>`; lo que se registró mientras no estuvo llega entonces. Hasta 50 avisos por respuesta
     (con más, `cursor` es el del último entregado y el resto llega en la siguiente). Un cursor mayor que el último
     aviso registrado (la base volvió atrás) se lee desde el inicio del día comercial; `key` evita los repetidos.
   - **`account_id`** es la cuenta vigente: si la cuenta se combinó en otra, la de destino. Las unidades que se
     cancelaron después del «Listo» ya no aparecen en `items`, y un aviso cuyas unidades se cancelaron todas no se
     manda (el cursor avanza igual). Al tocar un aviso de una cuenta que ya se cerró o se canceló, la aplicación muestra
     «Esta cuenta ya no está abierta».
   - **`active`**: `true` si «Avisar platillos listos» está activado y algún mesero está en el dispositivo (sesión con
     PIN creada desde el inicio del día comercial, o de antes pero vigente y usada en los últimos 30 minutos); si no, `false`, sin avisos (no es un error) y el cursor
     avanza (al volver a activarse no llegan avisos viejos). El servicio en primer plano se detiene con
     `active: false`.
   - **`day_ends_at`**: fin del día comercial en curso. Los avisos cuentan en el día comercial de su `ready_at`: al
     llegar la hora de inicio del día siguiente dejan de contar los meseros que entraron en el día anterior (salvo que
     su sesión siga vigente y la usen en los últimos 30 minutos; después, `active` pasa a `false` sola) y **los avisos del día anterior que no se entregaron ya no llegan**. Un corte de red que
     cruza esa hora pierde lo que se marcó «Listo» en los últimos minutos del día anterior (con la hora de inicio de
     madrugada casi nunca pasa); es la regla del día comercial.
   - **`settings`**: «Avisar platillos listos», «Sonido» y «Vibración» de Terminales › Editar (también en
     `terminal.ready_alerts_*` de `getIdentity` y `getRecoveryState`).
   - Errores: **401 `DEVICE_REVOKED`** (retirado; dejar de escuchar), 401 `DEVICE_REQUIRED`, **403
     `READY_ALERTS_DEVICE_NOT_ALLOWED`** (no es un teléfono o tableta de meseros), 400 con un cursor inválido o un
     parámetro desconocido, 401 `OFFLINE_CREDENTIAL_NOT_ALLOWED` con la credencial de envío.

3. **`streamReadyAlerts`** (`GET /ready-alerts/stream?after=<cursor>` o con `Last-Event-ID`): SSE con la misma lectura.
   Go revisa la base cada 750 ms, pero **solo manda** `event: alerts` (con `id: <cursor>` y `data: <ReadyAlertsBatch>`)
   al abrir y cuando hay avisos nuevos o cambian `active`, `settings` o `day_ends_at`; en silencio manda el comentario
   SSE `: ping` cada **15 s** (para saber que la conexión sigue viva). El canal dura **5 minutos** y el dispositivo lo
   vuelve a abrir de inmediato con `Last-Event-ID`. **Un canal nuevo del mismo dispositivo reemplaza al anterior** (el
   viejo se cierra): tras un cambio de red no hay que esperar. Cada lectura revalida el dispositivo: si falla, manda
   `event: interrupted` **sin `id`** (el cursor no cambia) con `data: {"code": "…"}` y termina. Solo
   **`DEVICE_REVOKED`** (o un 401 `DEVICE_REVOKED` al abrir) significa que el dispositivo se retiró; con cualquier otro
   código (`TEMPORARY` si la base no respondió a tiempo, o el código del rechazo) se reconecta con espera creciente.
   Al abrir, los errores son los de `getReadyAlerts`.

## Cupos de la licencia (etapa L1, Fase D)

El servidor Go aplica los cupos de sesiones simultáneas de la licencia (§7 del contrato de licencias, §5 de
[etapas/ETAPA_L1_LICENCIAS_v2.md](etapas/ETAPA_L1_LICENCIAS_v2.md)): **cajas** (`cash_registers`), **comanderos**
(`order_takers`) y **pantallas de cocina** (`kitchen_displays`). Cuentan **lo que hace cada terminal**, no su tipo. Go
decide; el cliente solo muestra el mensaje o arma el suyo con `details`.

| Cupo               | Lo ocupa                                                                                                                                                                                                                                                                                                                                                                                                                         | Se libera                                                                                                                                                                                                                                                                |
| ------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `cash_registers`   | La terminal que abre un turno de caja, y la terminal que crea o modifica un pedido con un turno activo en su caja aunque lo abriera otra (dos terminales con la misma caja física ocupan dos cupos, L1-D3). Cambiar de cajero en la misma terminal no ocupa otro.                                                                                                                                                                | Al cerrar ese turno (confirmar el arqueo), también desde otra computadora: se liberan todas las terminales que lo usaban. Si la terminal se cae o se revoca, el cupo sigue hasta cerrar el turno.                                                                        |
| `order_takers`     | La **terminal** (no el usuario) que crea o modifica un pedido sin turno activo en su caja: varios meseros con su PIN en una tableta ocupan uno, y volver a «Selecciona tu usuario» no lo libera. El pedido que entrega la cola de una tableta sin sesión (con la credencial de envío) también lo ocupa. Cuenta por actividad. Consultar no ocupa nada; las previas (`previewEdit` y la del borrador) solo renuevan su actividad. | A los **15 minutos** sin crear ni modificar pedidos (valor del servidor), cuando se cierra la última sesión de esa terminal (cerrar sesión, revocarla, vencer), al revocar la terminal, al abrir turno en su caja o desde Terminales («Liberar cupo», `devices.manage`). |
| `kitchen_displays` | La terminal de escritorio en Cocina › Preparación: `setKitchenDisplayMode` con `active: true`, ligado a la sesión que entra.                                                                                                                                                                                                                                                                                                     | Con `active: false`, al cerrar esa sesión o desde Terminales.                                                                                                                                                                                                            |

- **Qué ocupa cupo**: `openAccount`, `saveDraft`, `confirmDraft`, `editUnits` y `submitOfflineOrder` (crear o modificar
  un pedido); abrir turno; y `setKitchenDisplayMode`. **Cobrar y las operaciones de efectivo nunca piden cupo**, ni
  cancelar, reorganizar cuentas, la precuenta, «Entregado», la cocina ya enviada, las lecturas o la administración.
- **Sin cupo**: **409 `LICENSE_SEAT_UNAVAILABLE`** (`LicenseSeatUnavailable`) con `details.seat` (el cupo que falta),
  `details.in_use` (los que usan otras terminales) y `details.limit` (el de la licencia). `message` ya lo dice en
  español, por ejemplo «No hay cupo de comandero libre (3 de 3 en uso). Pide que se cierre la sesión de otra terminal.»;
  si la licencia no trae ese cupo (límite 0), «La licencia no incluye comanderos (0 de 0): solo se capturan pedidos en
  una terminal con el turno de caja abierto.». La operación **no se aplica**: no se guarda el pedido ni su
  `operation_id`.
- **El pedido sin conexión** (`submitOfflineOrder`) rechazado por cupo: el mismo 409 con `details.seat:
"order_takers"`. Es **definitivo para ese envío**: el dispositivo lo marca rechazado con el mensaje y no lo reintenta
  solo (la aplicación 0.3.0 ya trata un 409 con código propio como rechazo). Como la clave no quedó registrada, si el
  mesero lo vuelve a enviar a propósito con la misma `operation_id`, Go lo evalúa otra vez.
- **Lo que ya entró no vuelve a pedir cupo**: un reintento con una `operation_id` ya registrada (la respuesta se perdió)
  recibe su resultado guardado aunque el cupo de esa terminal haya vencido mientras tanto.
- **Licencia nueva con menos cupos**: nadie se interrumpe (quien tiene su cupo lo conserva y sigue capturando); no se
  ocupan cupos nuevos hasta quedar dentro del límite.
- **Sin licencia válida**: 409 `LICENSE_NOT_ACTIVATED` (la puerta de la licencia rechaza antes esas operaciones).
- **`setKitchenDisplayMode`** (`POST /kitchen/display-mode`, `{active: boolean}` → `{active}`): lo llama la terminal al
  entrar a Preparación y al salir. Requiere `kitchen.states.read`; repetirlo no ocupa otro cupo; salir nunca falla por
  la licencia. En un teléfono o tableta de meseros, 400. Si la sesión se cerró mientras llegaba la petición, 401
  `SESSION_REVOKED`.
- **Terminales y `sessions-summary`** (administración, en `API_CONTRACTS.md`): `GET devices` dice en `seats` qué cupo
  ocupa cada terminal; `POST devices/{id}/seat/release` libera su comandero o su cocina y cierra sus sesiones;
  `devices/sessions-summary` conserva `by_kind` y agrega `seats` con `in_use` y `limit` por cupo (`limited: true`).

## Licencia del servidor principal (etapa L1, fase B)

Capacidad `license-v1` (servidor 0.15.0). La licencia es de la computadora principal: solo Go habla con el servidor de
licencias (contrato de licencias §1). El escritorio y Android **solo muestran** el estado y los códigos de error.

- **`getLicenseStatus`** (`GET /license/status`):
  - **Con sesión**, el estado completo (`LicenseStatus`). Ver el estado no pide permiso.
    - `state`: `active` (vigente), `grace` (tolerancia), `expired` (vencida), `revoked` (revocada; el motivo va en
      `revocation_reason`: `deactivated`, `transferred` o `revoked`), `not_activated` o `not_activated_with_data`
      (sin activar, con datos).
    - `license`: plan, cliente, cupos (`limits`), vencimiento (`expires_at`, o `perpetual`), `grace_days` y
      `grace_ends_at`; `null` sin licencia válida.
    - `copied`: la instalación se copió de otra computadora. `machine_unverified` e `identity_unavailable`: no se pudo
      comprobar la computadora o falta su identidad.
    - `environment`: `test` en las versiones de desarrollo (servidor de licencias de pruebas).
  - **Con solo la credencial del dispositivo** (sin `Authorization`), el resumen `LicenseDeviceSummary`: `state`,
    `environment` y `copied`. Sirve al teléfono o la tableta en «¿Quién eres?».
- **La gestión** (activar, transferir, buscar actualización, liberar, la solicitud y el archivo sin internet, y la
  identidad nueva de una instalación copiada) es del escritorio, con el permiso «Administrar la licencia»
  (`license.manage`, solo el rol Administrador de fábrica). No es de este contrato: va en `API_CONTRACTS.md`.
- **Códigos de las rutas de la licencia** (los da Go, con su texto en español; nunca el mensaje del servidor de
  licencias):

  | Código                        | Estado | Caso                                                                                         |
  | ----------------------------- | ------ | -------------------------------------------------------------------------------------------- |
  | `LICENSE_KEY_INVALID_FORMAT`  | 400    | El código no tiene el formato: 15 símbolos en grupos de 3, 4, 4 y 4.                         |
  | `LICENSE_KEY_NOT_FOUND`       | 404    | No existe una licencia con ese código.                                                       |
  | `LICENSE_SERVER_REVOKED`      | 409    | La licencia fue revocada.                                                                    |
  | `LICENSE_ACTIVATION_LIMIT`    | 409    | Activa en otra computadora; `details.transfer_available` dice si se puede transferir.        |
  | `LICENSE_TRANSFER_LIMIT`      | 409    | Tiene una transferencia pendiente: «Comunícate con soporte».                                 |
  | `LICENSE_INSTALLATION_COPIED` | 409    | La instalación se copió de otra computadora: antes hay que crear una identidad nueva.        |
  | `LICENSE_NOT_ACTIVATED`       | 409    | Buscar actualización, liberar o la solicitud de renovar o liberar sin una licencia activada. |
  | `LICENSE_SERVER_BUSY`         | 429    | Demasiados intentos; `details.retry_after_seconds`.                                          |
  | `LICENSE_SERVER_UNAVAILABLE`  | 503    | Sin conexión, falla temporal o TLS; `details.reason`. La licencia instalada no cambia.       |
  | `LICENSE_SERVER_ERROR`        | 502    | El servidor de licencias rechazó la operación o respondió algo inesperado.                   |
  | `LICENSE_RESPONSE_INVALID`    | 502    | La licencia recibida no pasó la verificación y no se instaló; `details.reason`.              |
  | `LICENSE_FILE_INVALID`        | 400    | El archivo `.lic` no es válido para esta computadora; `details.reason`.                      |
  | `LICENSE_STATE_NOT_SAVED`     | 503    | No se pudo guardar la licencia; no cambió nada.                                              |
  | `IDENTITY_UNAVAILABLE`        | 503    | Falta la identidad o no se pudo comprobar la computadora (`details.reason`).                 |

## Códigos de error más frecuentes

| Código                                                   | Estado | Qué hacer                                                                                                                  |
| -------------------------------------------------------- | ------ | -------------------------------------------------------------------------------------------------------------------------- |
| `CLIENT_UPDATE_REQUIRED`                                 | 426    | Actualizar la aplicación de este equipo.                                                                                   |
| `DEVICE_REVOKED`, `DEVICE_REQUIRED`                      | 401    | Emparejar la terminal otra vez.                                                                                            |
| `PAIRING_EXPIRED`, `PAIRING_REJECTED`                    | 409    | Pedir otro código de emparejamiento.                                                                                       |
| `PAIRING_RATE_LIMITED`, `PAIRING_LIMIT_REACHED`          | 429    | Esperar unos minutos: límite por origen o tope global de solicitudes (30 cada 10 min).                                     |
| `SESSION_REPLACED`, `SESSION_EXPIRED`, `SESSION_REVOKED` | 401    | Iniciar sesión otra vez.                                                                                                   |
| `PERMISSION_DENIED`                                      | 403    | El usuario no tiene el permiso.                                                                                            |
| `OPERATION_VALIDATION`                                   | 400    | Datos inválidos; con `details.reason: account_not_open`, la cuenta ya no está abierta.                                     |
| `ACCOUNT_CONFLICT`                                       | 409    | Otra terminal cambió la cuenta: recargarla.                                                                                |
| `QUOTE_CHANGED`                                          | 409    | El cálculo cambió: volver a calcular y confirmar.                                                                          |
| `UNIT_HAS_PAYMENT`, `UNIT_HAS_DISCOUNT`                  | 409    | Las unidades tienen pago o descuento: no se cancelan ni modifican desde el dispositivo.                                    |
| `TABLE_HAS_ACCOUNT`                                      | 409    | La mesa ya tiene una cuenta abierta: elegir.                                                                               |
| `PRODUCT_SOLD_OUT`                                       | 409    | El producto está agotado.                                                                                                  |
| `PRODUCT_UNAVAILABLE`                                    | 409    | El producto no está disponible (pausado, fuera de horario o inactivo).                                                     |
| `ORDER_REQUIRES_CONFIRMATION`                            | 409    | Mostrar los cambios al mesero y reenviar con su confirmación.                                                              |
| `IDEMPOTENCY_CONFLICT`                                   | 409    | La clave ya se usó con otro contenido: consultar el resultado.                                                             |
| `OFFLINE_ORDER_ALREADY_ACCEPTED`                         | 409    | El pedido sin conexión (o su «Enviar a cocina», `details.origin`) ya entró: usar `details.result`, no capturarlo otra vez. |
| `PRINTER_NOT_CONFIGURED`, `PRINTER_UNAVAILABLE`          | 409    | Sin impresora válida para la precuenta; con `details.scope: "device"`, la del dispositivo (se elige en Terminales).        |
| `READY_ALERTS_DEVICE_NOT_ALLOWED`                        | 403    | Los avisos de platillos listos solo llegan a teléfonos y tabletas de meseros.                                              |
| `PRINT_NOT_FAILED`                                       | 409    | La precuenta no tiene una impresión fallida que reintentar.                                                                |
| `BALANCE_PENDING`                                        | 409    | El pedido para llevar tiene saldo: cobrarlo en la caja antes de marcarlo como entregado.                                   |
| `ALREADY_DELIVERED`                                      | 409    | El pedido para llevar ya estaba entregado: volver a leerlo.                                                                |
| `LOGIN_THROTTLED`                                        | 429    | Esperar 15 minutos.                                                                                                        |
| `PIN_INVALID`                                            | 401    | PIN incorrecto: volver a escribirlo.                                                                                       |
| `PIN_LOCKED`                                             | 429    | PIN bloqueado hasta `details.locked_until`, o hasta que un administrador lo desbloquee (`details.requires_unlock`).        |
| `PIN_LOGIN_BUSY`                                         | 503    | Muchas entradas con PIN a la vez: reintentar en unos segundos.                                                             |
| `CLIENT_KIND_MISMATCH`                                   | 400    | `X-Client-Kind` no es el tipo con que se emparejó el dispositivo.                                                          |
| `PIN_DEVICE_NOT_ALLOWED`                                 | 403    | La entrada con PIN solo existe en dispositivos `android` emparejados.                                                      |
| `OFFLINE_CREDENTIAL_INVALID`                             | 401    | Conservar el pedido y pedir al mesero que entre con su PIN con conexión.                                                   |
| `OFFLINE_CREDENTIAL_NOT_ALLOWED`                         | 401    | La credencial de envío solo se usa en `submitOfflineOrder`.                                                                |
| `LICENSE_SEAT_UNAVAILABLE`                               | 409    | Sin cupo de la licencia (`details.seat`, `in_use`, `limit`): mostrar el mensaje; en la cola, marcar el pedido rechazado.   |
