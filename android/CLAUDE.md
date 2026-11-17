# AD-EL POS Android · aplicación de meseros

Aplicación Android de meseros de AD-EL POS (etapa A1; la A2 agrega Para llevar, precuenta desde el dispositivo y
aviso de platillos listos). Es **un cliente más del servidor del restaurante** y usa la API v1 de la 9G
([docs/API_V1.md](../docs/API_V1.md), `backend/internal/apicontract/openapi.json`). Especificación de la etapa:
[docs/etapas/ETAPA_A1_ANDROID.md](../docs/etapas/ETAPA_A1_ANDROID.md); decisiones y avance:
[docs/ETAPA_A1_AVANCE.md](../docs/ETAPA_A1_AVANCE.md). Las reglas generales del repositorio siguen en
[CLAUDE.md](../CLAUDE.md) y [AGENTS.md](../AGENTS.md).

## Reglas permanentes

1. **La aplicación nunca calcula ni decide reglas de negocio.** Precios, impuestos, permisos y estados los decide Go;
   la aplicación muestra lo que Go entrega. Los precios del catálogo local son **referencia**: el importe final siempre
   lo calcula el servidor. Los importes son cadenas decimales (`Money`, `MoneySerializer`): nunca `Double` ni
   aritmética con ellos; se muestran con `MoneyFormat` (el formato único del escritorio y de Go).
2. **Textos en español** del restaurante, en `res/values/strings.xml` de cada módulo, sin tecnicismos (nada de IP,
   puertos, «servidor HTTP» ni códigos crudos; los datos técnicos solo en una sección de soporte), con los criterios del
   escritorio.
3. **Texto claro solo en `debug` y solo hacia `10.0.2.2`** (`app/src/debug`, la Mac vista desde el emulador, para el
   Go temporal de las pruebas). El APK de entrega es solo HTTPS con la **huella SPKI fijada** en el emparejamiento
   (`PinnedTrustManager`): nunca se acepta cualquier certificado ni se confía en las autoridades del sistema.
4. **Credenciales solo cifradas con el Android Keystore** (`KeystoreSecretSealer`, AES-256-GCM, llave no exportable).
   Nada sensible en Room sin cifrar, en registros ni en preferencias.
5. **Nada de llaves de firma en el repositorio.** El APK de prueba se firma con una llave de desarrollo que vive fuera
   (ver «Firma»); nunca con una llave de publicación. `.gitignore` bloquea `*.jks`, `*.keystore`, `*.p12` y las
   propiedades de firma, y la compilación rechaza archivos de firma dentro del repositorio.
6. **`minSdk` 26** (Android 8.0, tabletas económicas), `targetSdk` 36 y `compileSdk` 37 (A1-D7). Toda API más nueva va
   detrás de `Build.VERSION.SDK_INT` (o en una función con `@RequiresApi`) y se prueba en `ADEL_A1_Tablet_API26`. Las
   diferencias conocidas están en el avance, §1.6 (resumen abajo).
7. **Colores solo en el tema**: `core/ui/.../theme/AdelPalette.kt` es el único archivo con colores escritos (los mismos
   tokens de `desktop/src/styles.css`, en ambos temas, con el mismo significado). Las pantallas usan
   `MaterialTheme.colorScheme` o `AdelTheme.colors`. `ColorsOnlyInPaletteTest` falla con cualquier `Color(0x…)`, color
   con nombre o hexadecimal en recursos fuera de la paleta.
8. **Copias de seguridad desactivadas** (`allowBackup="false"`, `data_extraction_rules.xml` y `backup_rules.xml`
   excluyen todo): la base local y las credenciales no viajan.
9. **Inyección manual**: `AdelPosApplication` crea un `AppContainer`; cada `ViewModel` recibe lo suyo con una fábrica
   `viewModelFactory { initializer { … } }`. Sin Hilt ni búsquedas globales.
10. **Lo capturado sobrevive a la rotación y al cierre del proceso**: el estado de pantalla que escribe el mesero vive
    en el `ViewModel` con `SavedStateHandle` (ver `TablesViewModel`); lo que no se puede perder (pedidos) va a Room.
11. **Atrás con `BackHandler`** de Compose (atrás predictivo de Android 16, `enableOnBackInvokedCallback`); nunca
    `onBackPressed`.
12. **Botones táctiles**: 48 dp como mínimo y **56 dp** las acciones principales (`PrimaryButton`, `SecondaryButton`,
    `AdelDimens`); un solo botón principal por panel.
13. **El indicador de conexión siempre visible** en la barra superior común (`AdelTopBar` + `ConnectionIndicator`):
    «En línea», «Sin conexión» y «N pedidos por enviar».
14. **Código fuente claro, nunca ofuscado ni compactado** (regla del propietario, la misma de `AGENTS.md` para todo el
    repositorio): saltos de línea, indentación de 4 espacios y una declaración por línea; funciones, `when`, `if` y
    llamadas con muchos argumentos en varias líneas; nombres descriptivos y comentarios en español donde haya reglas,
    validaciones, seguridad o lógica compleja. La meta es que cualquiera lo entienda y lo pueda modificar. ktfmt
    (`android-format`) da el formato; no se compacta a mano para ahorrar líneas. R8 del APK de entrega solo reduce el
    archivo compilado y **no ofusca** (`-dontobfuscate`, A1-D8): no toca el código fuente.
15. **El trabajo pendiente de un mesero manda** (§0-B, §3.2 y §7; `WaiterWork` en `core:domain`): el bloqueo por
    inactividad no ocurre con una captura en curso ni con pedidos sin enviar (`blocksIdleLock`); cambiar de mesero lo
    impiden solo los pedidos sin enviar del mesero actual («Por enviar», «Enviando», «Requiere tu revisión»;
    `blocksSwitch`): una captura en curso se queda guardada con su mesero, su sesión no se cierra y el siguiente no la
    ve; un «Cerrar sesión» explícito lo impiden ambas cosas (`blocksLogout`). Quien muestre esas acciones pregunta a
    `WaiterWorkGate` antes de hacerlas. Un acceso que deja de valer (`SESSION_*`, PIN cambiado,
    `OFFLINE_CREDENTIAL_INVALID`) **nunca** saca al mesero ni pierde su trabajo: queda activo con `mustReenterPin` y la
    raíz muestra «Vuelve a escribir tu PIN» encima de su pantalla (`ReenterPinRoute`).
16. **Ninguna prueba abre la aplicación normal con la búsqueda automática en la red real**: las pruebas
    instrumentadas de `app` corren con `AdelTestRunner`, que busca el tipo NSD `_adelpostest._tcp` (el emulador de
    Android 17 sí ve los anuncios de la red de la Mac). Para capturas o revisiones a mano, usar el mismo ejecutor o una
    prueba; si alguna vez aparece el servidor real del restaurante, no se toca.
17. **El bloqueo por inactividad es del proceso**, no de una pantalla (`AppContainer.startIdleLock` con
    `IdleLock.supervise`): volver a crear la actividad nunca reinicia el plazo y, al volver a primer plano, bloquea en
    ese momento si ya venció. Todo `AlertDialog`, `DropdownMenu`, `Dialog` u hoja nueva lleva
    `Modifier.reportsUserActivity(LocalUserActivity.current)` (`core:ui`): sus toques no pasan por
    `Activity.onUserInteraction`.
18. **La cola sin conexión nunca pierde ni duplica un pedido** (§7, Fase E): cada pedido se guarda en Room con su
    clave de operación (la misma en todos sus reenvíos; Go nunca lo duplica) y cada cambio de estado pasa por
    `OrderTransitions` (`core:domain`): un borrador nunca se envía solo (solo con «Enviar a cocina»), «Recibido por el
    restaurante» llega **solo** con la respuesta de Go y «Requiere tu revisión» solo sale cuando el mesero acepta o
    corrige. Nada borra un pedido salvo el mesero en «Requiere tu revisión» o el borrado del dispositivo (después de
    pasarlo a la bitácora). Los envíos usan las credenciales **del mesero que capturó** (`WaiterServer`). Revisión
    R6 (A1-D19): «Enviar a cocina» usa **la misma clave** en `confirmDraft` y en su pedido sin conexión; con conexión
    no arma el pedido con un guardado del borrador sin resolver; toda escritura de la cola es condicional al estado
    leído (`WaiterDao.transition`, nunca `@Upsert` de la fila) y el borrado espera a que se cancele el envío; descartar
    también quita de Go lo guardado.
19. **WorkManager sin restricción de red** (`WorkManagerOrderScheduler.CONSTRAINTS`, `NetworkType.NOT_REQUIRED`):
    `NetworkType.CONNECTED` espera una red validada con internet y la wifi del restaurante puede no tenerla
    (comprobado en `WorkNetworkOnDeviceTest`). El trabajador prueba el servidor él mismo y reintenta con espera
    creciente; nunca trabajo urgente (`setExpedited`), que en Android 8–11 exige `getForegroundInfo()`.

## Estructura

| Módulo           | Tipo                                     | Qué contiene                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| ---------------- | ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `app`            | aplicación (`local.adel.pos.android`)    | `AdelPosApplication` + `AppContainer` (con el bloqueo por inactividad del proceso y el registro de borrado), `MainActivity` (de borde a borde), `ShellViewModel` (indicador), navegación (`navigation/AdelPosApp.kt`, rutas con tipo), `CatalogRealtimeContainer` (catálogo, imágenes y el único canal de tiempo real) y `AppForeground` (primer plano del proceso), manifiesto, ícono, `src/debug` con `network_security_config`.                                                                                                                                                                                                                                                   |
| `core:domain`    | Kotlin/JVM puro (sin Android)            | `Money` y `MoneyFormat`, `LinkState`/`ConnectionSummary` y `DefaultNetworkTracker`, `PendingOrdersSource`, `AppError`/`AppResult`, `SecretSealer`, `TableState`, `TablesView`; `server/` (dirección escrita, anuncio NSD, cambio de IP, versiones), `pairing/` (código de 6 dígitos, `PairingFlow`), `access/` (`DeviceAccess`, verificador del PIN, límite local, bloqueo por inactividad, `WaiterWork`), `catalog/` (modelos del catálogo, búsqueda sin acentos, `Customization`/`LineSelection`, reglas del personalizador y revisión de lo que declara el catálogo), `realtime/` (`RealtimeFeed`, señales, interrupciones de acceso, espera creciente). Pruebas JVM en segundos. |
| `core:data`      | biblioteca Android                       | `api/` (OkHttp con huella fijada, cabeceras del cliente, JSON, `MoneySerializer`, errores de Go → `AppError`, `ServerApi` con `ApiOutcome`), `access/` (`AccessRepository`, `PairingRepository`, `PairedConnection`), `discovery/` (NSD, sonda HTTPS), `security/` (huella fijada, primer certificado del emparejamiento, Keystore), `db/` (Room, esquemas en `core/data/schemas/`), `network/`, `pending/`, `preferences/`; `catalog/` (`catalog.db`, descarga completa y por cambios, caché de imágenes, actualizaciones), `realtime/` (cliente SSE), `session/` (`ServerAccess`, `DeviceServerAccess`).                                                                           |
| `core:ui`        | biblioteca Android (Compose, Material 3) | Tema claro/oscuro (`theme/`), íconos propios (`icons/AdelIcons.kt`, los trazos de `desktop/src/components/Icon.tsx`), botones, barra superior e indicador, estado de mesa, toques de los diálogos para el bloqueo por inactividad (`LocalUserActivity`), andamio de uno o dos paneles (`layout/`), `catalog/` (búsqueda, categorías, cuadrícula con «Agotado», imagen con marcador y el personalizador táctil). Compartido con la A2.                                                                                                                                                                                                                                                |
| `feature:access` | biblioteca Android                       | Entrada al dispositivo (fases B y C): «Buscar el servidor» y código de emparejamiento, entrada con PIN, «Vuelve a escribir tu PIN», cambio de mesero (`AccessViewModel`), emparejar de nuevo, actualizar la aplicación o el servidor, dispositivo retirado y su bitácora, permiso de red local. Solo usa interfaces de `core:domain`.                                                                                                                                                                                                                                                                                                                                                |
| `feature:waiter` | biblioteca Android                       | Modo Mesero (Fase E): `tables/` (Mesas en lista o mapa con su tiempo, búsqueda y el panel de la mesa con sus cuentas y «Nueva cuenta»), `capture/` (catálogo y personalizador de `core:ui` con la cuenta al lado o a un toque: lo enviado con su preparación, modificar o cancelar unidades, lo por enviar y «Enviar a cocina»), `pending/` («Pedidos pendientes» con los cinco estados y «Requiere tu revisión»); `WaiterDependencies` (solo interfaces de `core:domain`). Queda la pantalla de muestra «Catálogo» de la Fase D (sin ruta en la navegación).                                                                                                                        |

Dependencias entre módulos: `app` → todos; `feature:*` → `core:ui` (→ `core:domain`); `core:data` → `core:domain`. Un
módulo `feature` nunca depende de otro `feature`; la A2 agrega `feature:takeaway` sobre `core:*`.

**Versión de la aplicación**: un solo lugar, `android/gradle.properties` (`adelpos.versionName`, `adelpos.versionCode`).
La usan el APK y la cabecera `X-Client-Version` (`BuildConfig.VERSION_NAME` → `ClientIdentity`).

## Comandos

Desde la raíz del repositorio (usan el JDK de Android Studio y `~/Library/Android/sdk` si `JAVA_HOME` o `ANDROID_HOME`
no están definidos; necesitan Android Studio instalado, por eso **no** forman parte de `format-check` ni de `test`):

| Comando                                 | Qué hace                                                                                           |
| --------------------------------------- | -------------------------------------------------------------------------------------------------- |
| `./scripts/dev.sh android-format`       | ktfmt (estilo oficial de Kotlin, 4 espacios, 100 columnas) en fuentes y scripts `.kts`.            |
| `./scripts/dev.sh android-format-check` | Comprueba el formato sin cambiar nada.                                                             |
| `./scripts/dev.sh android-build`        | `assembleDebug` y `assembleRelease` (R8, sin ofuscar) y el tamaño de cada APK.                     |
| `./scripts/dev.sh android-test`         | Pruebas de la JVM (`core:domain`), JVM + Robolectric SDK 36 de los módulos Android y Android Lint. |
| `./scripts/dev.sh android-connected`    | Pruebas instrumentadas; con más de un dispositivo, elegir con `ANDROID_SERIAL=emulator-5580`.      |
| `scripts/android-e2e.sh`                | Extremo a extremo contra un servidor Go **temporal** (ver «Pruebas»); nunca el servicio real.      |

Los agentes de la etapa usan el arnés aislado (`$SP/iso.sh <árbol> gradle <tareas…>`, `emu-up`, `server-up`), que da el
mismo entorno. Los comentarios de línea (`//`) se escriben de 100 columnas como máximo: ktfmt corta los más largos.

## Pruebas

- **JVM** (`core:domain:test`, `testDebugUnitTest`): dinero con la tabla de casos que comparten Go y el escritorio
  (`backend/internal/moneytext/testdata/cases.json`), monitor de conexión (lógica pura), cliente HTTP contra
  MockWebServer con TLS (huella aceptada y rechazada, cabeceras, 426 y demás errores), `MoneySerializer`, regla de los
  paneles, colores solo en la paleta y contraste AA de los tokens, `SavedStateHandle` de Mesas.
- **Robolectric** (SDK 36, `src/test/resources/robolectric.properties`): Room. Cada SDK adicional descarga ≈ 224 MB.
- **Catálogo y tiempo real (Fase D)**: JVM del dominio (búsqueda, personalizador, revisión, descripción, espera
  creciente); `CatalogSyncTest` y `CatalogImageCacheTest` (Robolectric + MockWebServer con los JSON del contrato de
  `core/data/src/test/resources/catalog/`: completa, cambios, `deleted`, archivados, `reset`, agotados, búsqueda,
  transacción, 304); `CustomizationJsonTest` (la forma exacta de `customization`/`LineRequest`); `RealtimeClientTest`
  (cursor de `changes`, `interrupted` con `id: 0`, `reset`, espera creciente, conexión muerta, sesión nueva, segundo
  plano); `CatalogUpdateTriggersTest`; `DeviceServerAccessTest` (con el Go simulado de la entrada). Instrumentadas:
  `CatalogDatabaseMigrationTest` (`MigrationTestHelper` y reemplazo en una transacción), `CatalogComponentsTest`
  (`core:ui`) y `CatalogSampleScreenTest` (`app`).
- **Instrumentadas** (`connectedDebugAndroidTest`): `MigrationTestHelper`, Keystore, Compose (tema, indicador,
  estados de mesa, botones, paneles) y la aplicación completa (navegación, uno o dos paneles según el dispositivo,
  rotación real con `UiAutomation.setRotation` sin perder lo capturado, indicador con el modo avión real).
- **Contra el Go temporal**: `ServerApiOnDeviceTest` (`core:data`, argumentos `pin` y `net`) y, de punta a punta,
  `AccessOnGoTest` (`app`, 7 pasos: emparejar con el código, PIN incorrecto y bloqueo al quinto que cierra la entrada
  sin conexión, cabeceras de cada mesero, entrada sin conexión, PIN cambiado con «Vuelve a escribir tu PIN» sin salir
  de Mesas, 426, certificado cambiado y dispositivo retirado). Ambas se omiten sin argumentos (en
  `connectedDebugAndroidTest` salen como omitidas). `AccessOnGoTest` se corre con **`scripts/android-e2e.sh`**: un paso
  por instrumentación (`am instrument`) con la aplicación instalada una sola vez, la prueba hace de computadora
  principal por la escucha local (`MainComputer`) y la aplicación usa la escucha de red. Recibe por variables
  `ANDROID_SERIAL`, `ADELPOS_E2E_LOCAL_URL` (`http://127.0.0.1:<puerto>`), `ADELPOS_E2E_NETWORK_URL`
  (`https://127.0.0.1:<puerto>`), `ADELPOS_E2E_PIN` (opcional; si falta, la del certificado servido),
  `ADELPOS_E2E_SERVER_ID` y `ADELPOS_E2E_PROVISIONING_FILE` (o `ADELPOS_E2E_PROVISIONING`, o la terminal principal en
  `ADELPOS_E2E_MAIN_DEVICE`/`ADELPOS_E2E_MAIN_TOKEN`); se niega con los puertos 8181/8443, con otro host o si
  `GET version` no dice `test_mode: true`. Pasos 1–5 y 7 con un servidor; el 6 necesita **otro** servidor temporal
  (otra identidad) en las mismas direcciones: `--steps "1 2 3 4 5"`, reemplazar el servidor y `--no-install --steps
"6 7"`. En Android 8 a 10 el paso 3 corta la red desde la Mac con `adb root` (solo imágenes `google_apis`).
  Resultados en `android/app/build/outputs/e2e/<serial>/`. **Nunca** contra el servicio real de la Mac.
- **`CatalogOnGoTest`** (`app`, contra el Go temporal; se omite sin argumentos) corre en tres etapas con
  `am instrument -e class local.adel.pos.android.CatalogOnGoTest -e stage <a|b|c>` y los argumentos `pin`, `net`,
  `local` y `provisioning`, con la aplicación instalada una sola vez (`:app:installDebug :app:installDebugAndroidTest`):
  `a` empareja y entra por la API, crea el catálogo (≈ 300 productos, con sus tiempos en el registro `CatalogOnGoTest`),
  cambia precio, crea, archiva y agota desde la principal (por el canal y por cambios) y revalida la imagen (304);
  entre `a` y `b` se borra un producto en la base temporal (`DELETE FROM app.catalog_products WHERE code='TEMP-D'`) y
  `b` lo recibe en `deleted`; entre `b` y `c` se cambia la época (`UPDATE app.catalog_epoch SET epoch =
gen_random_uuid()`, como una restauración) y `c` recibe `reset: true`. Solo con la base **temporal** del arnés.
- **Ejecutor de pruebas de `app`**: `AdelTestRunner` (antes de crear el contenedor) busca por NSD el tipo
  `_adelpostest._tcp` y toma `clientVersion` de los argumentos. Ayudantes: `TestDevice` (un servidor emparejado que no
  responde y un mesero conocido para entrar sin conexión, con el Keystore real), `FakeAccess` (entrada falsa para las
  pantallas con estados forzados) y `MainComputer`.
- **`testOptions.targetSdk = 36` en las bibliotecas que tocan la red** (`core:data`): el APK de pruebas de una
  biblioteca toma `compileSdk` (37) si no se indica, y en Android 17 un `targetSdk` 37 exige `ACCESS_LOCAL_NETWORK`
  (visto: `SecurityException: Missing local network permission` al anunciar por NSD). Se prueba con el nivel de la
  aplicación (A1-D7).
- **Matriz** (avance §1.7): `Pixel_8` (5580) y `Pixel_Tablet` (5582) con Android 17; `ADEL_A1_Phone_API33` (5592) y
  `ADEL_A1_Tablet_API26` (5590). Emuladores sin ventana y de solo lectura con `iso.sh - emu-up <avd> <consola>`; como
  mucho dos a la vez; `emu-down` al terminar.
- **Corte de red**: `cmd connectivity airplane-mode enable|disable` en Android 11+ (desde la prueba con
  `UiAutomation.executeShellCommand`); en Android 8 solo desde la Mac con `adb root` + `svc wifi disable` +
  `svc data disable` (imágenes `google_apis`; las de Play no permiten root).
- **Ojo**: con AGP 9.5.1, si el APK no se puede instalar (por ejemplo, otra firma de la misma aplicación ya instalada),
  `connectedDebugAndroidTest` lo escribe en el registro pero **termina con BUILD SUCCESSFUL sin correr las pruebas** de
  ese módulo. Comprobar siempre los resultados en `*/build/outputs/androidTest-results/connected/` y desinstalar antes
  (`adb uninstall local.adel.pos.android`).

## Entrada al dispositivo (para los demás módulos)

`AppContainer.deviceAccess` (`AccessRepository`, que implementa `DeviceAccess`) es la única fuente del servidor
emparejado, el mesero activo y sus credenciales. Otro módulo (catálogo, modo Mesero, cola, canal de eventos):

- `serverApi(): ServerApi?` — la API del servidor emparejado en la dirección vigente, con la huella fijada. Pedirla en
  cada uso (la dirección cambia con el cambio de IP); null sin servidor emparejado.
- `requestHeaders(userId): Headers?` — las cabeceras **del mesero dueño de la operación** (guárdalo al crearla; nunca
  «el mesero activo» para una operación de otro): `X-Device-ID`/`X-Device-Token` y, si ese mesero tiene **sesión de
  Go**, `Authorization: Bearer`. Solo una entrada con conexión da sesión; una entrada sin conexión nunca usa la de una
  entrada anterior. `deviceCredentials()` da solo las del dispositivo; `sessionToken(userId)`, la sesión de un mesero
  (solo en memoria: se pierde al cerrar la aplicación); `offlineCredential(userId)`, su credencial de envío (A1-D1,
  cifrada con el Keystore) para `submitOfflineOrder` sin sesión. `activeWaiter` (`StateFlow`) dice quién usa el
  dispositivo, si tiene sesión (`hasServerSession`; sin conexión no la tiene) y si debe volver a escribir su PIN
  (`mustReenterPin`).
- `report(error: AppError, userId: String?)` — **punto único** para los errores de Go de cualquier petición o del
  canal de eventos (`event: interrupted`), con el mismo `userId` de `requestHeaders`: `DEVICE_REVOKED` retira el
  dispositivo (bitácora y borrado), `SESSION_*` y `OFFLINE_CREDENTIAL_INVALID` olvidan la sesión de ese mesero (la
  segunda también su verificador) y, si es el activo, le piden «Vuelve a escribir tu PIN» sin sacarlo; `DEVICE_*` y
  una huella distinta piden emparejar de nuevo, 426 pide actualizar.
- **Borrar los datos de este dispositivo** (`AppContainer.deviceDataWipe`, `DeviceDataWipe`): registro único que usan
  retirar el dispositivo y «Emparejar de nuevo», después de pasar los pedidos sin enviar a la bitácora y antes de
  olvidar el emparejamiento y destruir la llave del Keystore. Hoy registra el catálogo con sus imágenes; **la Fase E
  registra ahí** la cola de pedidos y sus borradores y cancela su trabajo (`WorkManager.cancelAllWork()`). Todo módulo
  nuevo que guarde datos del restaurante se registra ahí.
- Límite de la entrada sin conexión (`OfflineAttemptPolicy`, sin reloj): los fallos (con o sin conexión) se acumulan y
  solo vuelven a cero con una entrada correcta; al quinto, o con el PIN bloqueado en Go (`PIN_LOCKED`, `locked_until`
  o `requires_unlock` de `auth/pin-users`, A1-D16), la entrada sin conexión de ese mesero queda cerrada hasta que
  entre con conexión.
- `AppContainer` da **`WaiterWorkGate`** y **`UnsentOrdersSource`** desde la cola real (`OrderQueueRepository`, Fase
  E): capturas en curso y pedidos «Por enviar», «Enviando» o «Requiere tu revisión» de cada mesero; al retirar el
  dispositivo o emparejarlo de nuevo, eso (también las capturas sin enviar) pasa a la bitácora y se muestra antes. La
  cola y las capturas están registradas en `deviceDataWipe` («cola de pedidos y capturas», con
  `WorkManager.cancelAllWork()`).

### Contrato con la cola y el modo Mesero (revisión R3: M3, M4 y M5)

Estas formas son estables: la Fase E las usa tal cual y cambiarlas se avisa en el informe de la etapa.

- **Dueño de cada operación (M5).** Cada pedido, reintento o actualización guarda al crearse el `user_id` del mesero
  que la hizo y, al salir, pide `deviceAccess.requestHeaders(userId)` con **ese** `user_id` (nunca
  `activeWaiter.value`): si Ana cambió a Beto o se bloqueó mientras tanto, la operación sale con la sesión de Ana o,
  si Ana ya no tiene sesión, sin `Authorization` (y entonces, si es un pedido sin conexión, con
  `offlineCredential(userId)` en `submitOfflineOrder`). Un error de esa respuesta se avisa con
  `deviceAccess.report(error, userId)` y el mismo `user_id`.
- **Acceso que deja de valer (M3).** `report(AppError.SessionEnded(…), userId)` (`SESSION_*`) olvida la sesión de ese
  mesero; `report(AppError.Rejected(401, AccessRepository.OFFLINE_CREDENTIAL_INVALID, …), userId)` olvida además su
  verificador y su credencial de envío (`offlineCredential(userId)` pasa a null: hace falta entrar **con
  conexión**). Ninguno saca al mesero ni toca la cola: si es el activo, `activeWaiter.value` sigue siendo él con
  `hasServerSession = false` y `mustReenterPin = true`, y la raíz muestra «Vuelve a escribir tu PIN» encima de su
  pantalla. La cola **conserva** el pedido (no lo descarta ni lo pasa a otro mesero) y lo reintenta cuando
  `requestHeaders(userId)` vuelva a traer `Authorization` (otra entrada con conexión) u `offlineCredential(userId)`
  vuelva a existir. Observar `activeWaiter` para saber cuándo volvió a entrar (`mustReenterPin` en false). Errores
  que no son de acceso (`VALIDATION_ERROR`, `ORDER_REQUIRES_CONFIRMATION`…) los decide la pantalla: no pasan por
  `report`. 400 `CLIENT_KIND_MISMATCH` es un problema de soporte: `report` no cambia nada con él.
- **Borrar los datos de este dispositivo (M4).** La cola se registra una vez, al crearse en `AppContainer`:
  `deviceDataWipe.register("cola de pedidos") { /* borrar cola y borradores; WorkManager.cancelAllWork() */ }`
  (`DeviceDataPart.erase()` es `suspend`; se llama fuera del hilo principal). Al retirar el dispositivo o con «Emparejar
  de nuevo», `AccessRepository` hace, en este orden: lee `UnsentOrdersSource.unsentOrders()` y lo pasa a la bitácora;
  llama a cada parte registrada (una que falla no detiene a las demás); olvida el emparejamiento, los meseros, sus
  verificadores y credenciales; destruye la llave del Keystore. Antes de confirmar «Emparejar de nuevo», la pantalla
  muestra `DeviceAccess.pendingOrders()` (lo mismo que `unsentOrders()`).

## Catálogo y tiempo real (para los demás módulos)

`AppContainer.catalogRealtime` (`CatalogRealtimeContainer`) arranca con la aplicación y habla con Go a través de
`DeviceServerAccess` (la interfaz estrecha `ServerAccess` sobre `deviceAccess`: hay conexión solo con el dispositivo
listo y un mesero **con sesión de Go**; la dirección se pide en cada uso por el cambio de IP). Sus rechazos y el
`event: interrupted` del canal van a `deviceAccess.report(error, userId)` con el mesero dueño de la sesión.

- **Catálogo** (`catalogRealtime.catalog`, `CatalogSource`): `catalog` (`StateFlow<Catalog>` de `catalog.db`),
  `search(CatalogQuery)` (sin acentos ni mayúsculas, sobre la base local), `status` y `requestRefresh()`. Se actualiza
  al abrir la aplicación, con sesión nueva, al recuperar la red, con los eventos del ámbito `catalog`, con «reconstruir»
  y cada 5 minutos en primer plano; una descarga a la vez (si llega otra petición, una más al terminar). La versión y la
  época guardadas (`catalog.value.version`) son las que la cola sin conexión envía con cada pedido. **Los precios son
  referencia**: el importe final lo calcula Go.
- **Imágenes** (`catalogRealtime.images`, `CatalogImages`): `cached`/`obtain` y `ProductImage` de `core:ui`, que
  muestra un marcador mientras no hay imagen.
- **Personalizador**: `ProductCustomizer` (`core:ui`) produce un `LineSelection` (= `LineRequest`); `toLineRequest()`
  y `Customization.toDto()` (`core:data`) dan el JSON exacto que esperan `openAccount`, `saveDraft`,
  `previewDraft` y `submitOfflineOrder` (con `CustomizationJson`); `CustomizationDto.toDomain()` lee un borrador de Go.
  `CustomizationCheck` revisa solo lo que declara el catálogo; `LineDescription` describe la partida.
- **Tiempo real** (`catalogRealtime.realtime`, `RealtimeFeed`): **un solo canal SSE por proceso**, abierto solo en primer
  plano y con sesión; nunca abrir otro (Go admite dos por dispositivo). La Fase E se suscribe con
  `realtime.events(RealtimeScopes.RESTAURANT)` (o `TAKEAWAY`, con tipos como
  `RealtimeKinds.ACCOUNT_PREPARATION_CHANGED`) y `realtime.rebuilds()` (reset del servidor o canal abierto de nuevo:
  volver a leer `restaurant/state`). El cursor sale del `id` de los mensajes `changes`; se abre desde el
  `event_cursor` de `restaurant/state` (`RecoveryStateCursorSeed`; la Fase E puede pasar su propia semilla para no
  leer el estado dos veces) y se reabre al cambiar de mesero.

## Modo Mesero y cola sin conexión (Fase E, para los demás módulos)

`AppContainer.waiterMode` (`WaiterModeContainer`) y `AppContainer.orderQueue`; las pantallas reciben
`AppContainer.waiterDependencies` (solo interfaces de `core:domain`: `RestaurantSource`, `CaptureService`,
`OrderQueueService`, `SentLineService`).

- **Mesas y cuentas** (`RestaurantRepository`): `getRestaurantOverview` y `getRecoveryState` con la sesión del mesero
  activo, guardados en Room (`restaurant_snapshot`, `waiter_permissions`) para verse sin conexión; los eventos del
  ámbito `restaurant` del único canal avisan qué cuenta cambió (`accountChanges`) y vuelven a leer las mesas.
- **Captura** (`CaptureRepository`, puerto de `draftSync.ts`): cada toque es una `DraftOperation` guardada en Room por
  mesero; con sesión, el borrador se guarda en Go con 700 ms de espera, la revisión vigente y una petición a la vez
  (creación diferida con `openAccount` y `seen_account_ids`; `TABLE_HAS_ACCOUNT`, `ACCOUNT_CONFLICT` y
  `account_not_open` los decide el mesero). Un guardado sin respuesta se repite **igual** (misma clave y mismas
  operaciones, `pendingOperationId` e `inFlightCount` en Room) antes de mandar nada más. Con la base sin leer
  (`baseRevision` 0) primero se lee el borrador de Go: un guardado lo sustituye completo.
- **«Enviar a cocina»** (`sendToKitchen`): la captura pasa a la cola en una transacción (`queueFromCapture`) con la
  clave del pedido = el id de la captura. Con sesión, el borrador guardado y su cotización vigente, primero
  `confirmDraft` con la huella que vio el mesero; si no se puede (sin respuesta, `QUOTE_CHANGED`, agotado, conflicto),
  sigue como pedido sin conexión y Go lo revisa todo; `account_not_open` y 403 van a «Requiere tu revisión».
- **Pedido sin conexión** (`OrderFactory` + `OfflineOrderPlan`, A1-D6): partidas = lo de este dispositivo y lo que
  tocó aquí (con su id de renglón); `replaces_draft_items` = lo que vio de Go de esas, más lo que quitó (solo con
  cuenta). `seen_unit_price`: el de la cotización de Go si la hubo; si no, el de referencia del catálogo (nunca se
  calcula uno).
- **Envío** (`OrderSender` en `OrderSendWorker`): `submitOfflineOrder` con `X-Offline-Credential` del mesero que
  capturó, del más viejo al más nuevo; sin respuesta se detiene y WorkManager reintenta (exponencial desde 10 s); además
  se pide un envío al recuperar la red, al volver a primer plano, cuando un mesero entra con conexión (sus pedidos
  detenidos por `OFFLINE_CREDENTIAL_INVALID` se reintentan) y cada 15 minutos (respaldo periódico). Al arrancar, lo
  que quedó «Enviando» vuelve a «Por enviar».
- **Contador del indicador**: todos los pedidos sin enviar del dispositivo; «Pedidos pendientes» muestra solo los del
  mesero activo y, de los demás, solo cuántos son.
- Pruebas: `OrderQueueRulesTest`, `WaiterRulesTest` (dominio); `OrderQueueTest`, `CaptureSyncTest` (Robolectric +
  MockWebServer); `WaiterViewModelsTest`; instrumentadas `WaiterScreensTest`, `WorkNetworkOnDeviceTest` y, contra el
  Go temporal, `WaiterModeOnGoTest` con **`scripts/android-e2e.sh --suite waiter`** (pasos `w1`–`w7c`; el guion corta
  la red entre pasos y en `w4` la prueba pierde una respuesta con `lostOfflineResponses`, que lee `AdelTestRunner`).

## Dependencias

- Versiones en `gradle/libs.versions.toml` (las de la Fase 0, avance §1.2; fuentes y para qué en
  [docs/DEPENDENCIES.md](../docs/DEPENDENCIES.md), «Etapa A1 · Android»).
- **Bloqueo**: un `gradle.lockfile` por proyecto con todas sus configuraciones, `buildscript-gradle.lockfile` (los
  complementos) y `settings-gradle.lockfile`.
- **Verificación**: `gradle/verification-metadata.xml` con el SHA-256 de cada artefacto (módulos, POM y complementos).
  Una dependencia que no está ahí hace fallar la compilación. `aapt2` es un artefacto por sistema operativo: solo está el
  de macOS (`-osx`).
- **Agregar o cambiar una dependencia**: editar el catálogo y regenerar ambos con un emulador encendido (las pruebas
  instrumentadas resuelven artefactos propios):

  ```sh
  ANDROID_SERIAL=emulator-5592 ./gradlew resolveAllDependencies assembleDebug assembleRelease assembleDebugAndroidTest \
    :core:domain:test testDebugUnitTest lintDebug ktfmtCheck connectedDebugAndroidTest \
    --write-locks --write-verification-metadata sha256
  ```

  Revisar el diff de los bloqueos y de las sumas (que solo cambie lo esperado) y anotar la dependencia en
  `docs/DEPENDENCIES.md`.

- Gradle 9.9.1 con su wrapper versionado (`gradle-wrapper.jar` idéntico al publicado y `distributionSha256Sum`).
- Avisos conocidos e inofensivos: `Configuration.setVisible` en desuso (lo usa el complemento de ktfmt 0.28.0; se
  quitará en Gradle 11), `Unable to strip … libandroidx.graphics.path.so` (no hay NDK), `sun.misc.Unsafe` (protobuf de
  AGP con el JDK 25) y el `lint.jar` de lifecycle 2.11.0 que el lint de AGP 9.5.1 omite (`ObsoleteLintCustomCheck`,
  informativo).

## Firma

El tipo `release` (R8 que reduce sin ofuscar, A1-D8) se firma solo si se indica un archivo de propiedades **fuera del
repositorio**, con la propiedad de Gradle `adelpos.signingProperties` o la variable `ADELPOS_SIGNING_PROPERTIES`:

```properties
storeFile=/ruta/fuera/del/repositorio/adel-pos-desarrollo.jks
storePassword=…
keyAlias=…
keyPassword=…
```

Sin ellas, `assembleRelease` produce `app-release-unsigned.apk`. La llave de desarrollo del APK de prueba la crea la
Fase G (nunca una llave de publicación) y se guarda fuera del repositorio.

## Diseño

- **Tema** (`AdelTheme`): claro u oscuro según el sistema; los roles de Material 3 se llenan todos con los tokens del
  escritorio (ningún componente usa la paleta de muestra de Material). `AdelTheme.colors` da lo que Material no tiene:
  estados de mesa, ámbar, uva, indicador de conexión.
- **Estados de mesa** (`TableStateBadge`): color, texto e ícono, como el escritorio (Libre palomita, Ocupada reloj,
  Esperando pago recibo, Pago parcial efectivo, Fuera de servicio candado). El estado lo calcula Go.
- **Íconos**: `AdelIcons`, `ImageVector` propios con los trazos SVG del escritorio (24 × 24, trazo 1.6); sin la
  biblioteca de íconos extendida. Para agregar uno, copiar su `d` de `Icon.tsx` (círculos y rectángulos con `circle` y
  `roundedRect`).
- **Uno o dos paneles** (`AdaptivePanes`, `currentPaneLayout()` con `currentWindowAdaptiveInfoV2()`): dos paneles con
  ancho ≥ 600 dp **y** alto ≥ 480 dp (tableta en ambas orientaciones); uno en el teléfono, también horizontal (≈ 914 ×
  411 dp), con el otro panel «a un toque» y «Volver a …» o Atrás para regresar.
- **De borde a borde** en todas las versiones (`enableEdgeToEdge()` y `WindowInsets.safeDrawing` en el `Scaffold`).

## Diferencias de plataforma (avance §1.6)

TLS 1.3 desde Android 10 (en 8–9, TLS 1.2 con las suites restringidas, verificado contra Go); NSD con
`registerServiceInfoCallback` desde la API 34 y `resolveService` antes, de uno en uno; `POST_NOTIFICATIONS` desde la 33;
`setExpedited` de WorkManager es servicio en primer plano en 26–30 (preferir trabajo normal); WorkManager y el
indicador **no exigen red validada** (la wifi del restaurante puede no tener internet); StrongBox opcional desde la 28;
`dataExtractionRules` desde la 31 (`fullBackupContent` antes); al pasar a `targetSdk` 37, permiso de red local
(`ACCESS_LOCAL_NETWORK`). `java.time` existe desde la 26 (sin «desugaring»).

## Base local (Room)

`AppDatabase` versión 5: `device_preferences` (v1); `paired_server` (una fila: `server_id`, nombre, última dirección,
huella fijada, `device_id`, **token del dispositivo cifrado**, `idle_lock_seconds`), `known_waiters` (meseros que ya
entraron con conexión aquí: nombre, inicial, color, `pin_version`, **verificador del PIN y credencial de envío
cifrados**, fallos sin conexión y si su entrada sin conexión está cerrada, sin ningún reloj) y `device_log` (bitácora: pedidos que no se enviaron, hasta que se borra).
Las llaves solo en el Android Keystore; lo cifrado lleva su propósito (`device-credential`, `pin-verifier:<id>`,
`offline-credential:<id>`). Migraciones 1 → 2 y 2 → 3 con el SQL de `schemas/…/2.json` y `3.json` (la 2 → 3 copia
`known_waiters` a una tabla nueva: los fallos se conservan y un bloqueo por reloj pasa a entrada cerrada). Versión 4
(Fase E): `captures` (capturas por mesero), `pending_orders` (la cola, con la clave de operación como llave),
`restaurant_snapshot` y `waiter_permissions` (lo último de Go para trabajar sin conexión); migración 3 → 4 con el SQL
de `schemas/…/4.json` (`WAITER_TABLES_V4`). Versión 5 (revisión R6): columnas nuevas `captures.created_account`,
`pending_orders.unresolved_save` y `pending_orders.clock_corrected` (migración 4 → 5 con `ALTER TABLE`,
`WAITER_COLUMNS_V5`).

Cada cambio de esquema: subir `version`, agregar la `Migration` a `AppDatabase.MIGRATIONS`, versionar el nuevo
`core/data/schemas/…/<versión>.json` y un caso en `AppDatabaseMigrationTest`. **Nunca**
`fallbackToDestructiveMigration`: la base guardará pedidos sin enviar.

`CatalogDatabase` (`catalog.db`, versión 1, Fase D) es **otra base**: la copia del catálogo de Go, que se reemplaza
completa con `reset` o época distinta sin tocar `AppDatabase`. Cada registro guarda el documento JSON de Go
(`document`) y las columnas que se consultan (orden, categoría, `search_key` normalizada); `catalog_state` (versión,
época, lista y tasa), `images` (referencias) e `image_files` (la caché de `cacheDir/catalog-images/`, con su `ETag`).
Cada descarga se guarda en una transacción. Mismas reglas de versiones, con `CatalogDatabase.MIGRATIONS` y
`CatalogDatabaseMigrationTest`.
