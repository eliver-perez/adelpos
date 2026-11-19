# AD-EL POS Android · aplicación de meseros

Aplicación Android de meseros de AD-EL POS (etapa A1; la A2 agrega Para llevar, precuenta desde el dispositivo y
aviso de platillos listos; la A3, la conexión tolerante con un solo estado de conexión, la regla de textos y los
ajustes de pantallas de su §4). Es **un
cliente más del servidor del restaurante** y usa la API v1 de la 9G
([docs/API_V1.md](../docs/API_V1.md), `backend/internal/apicontract/openapi.json`). Especificación de la etapa:
[docs/etapas/ETAPA_A1_ANDROID.md](../docs/etapas/ETAPA_A1_ANDROID.md); decisiones y avance:
[docs/ETAPA_A1_AVANCE.md](../docs/ETAPA_A1_AVANCE.md). Los de la A2:
[docs/etapas/ETAPA_A2_ANDROID.md](../docs/etapas/ETAPA_A2_ANDROID.md) y
[docs/ETAPA_A2_AVANCE.md](../docs/ETAPA_A2_AVANCE.md) (decisiones A2-Dn). Los de la A3:
[docs/etapas/ETAPA_A3_AJUSTES.md](../docs/etapas/ETAPA_A3_AJUSTES.md) y
[docs/ETAPA_A3_AVANCE.md](../docs/ETAPA_A3_AVANCE.md) (decisiones A3-Dn). Las reglas generales del repositorio
siguen en [CLAUDE.md](../CLAUDE.md) y [AGENTS.md](../AGENTS.md).

## Identificador y nombres (A2-D5)

Una sola convención, la misma para el identificador de la aplicación, los `namespace` de Gradle y los paquetes de
Kotlin, desde la 0.2.0:

| Pieza                                     | Nombre                                                                                                                                                                                       |
| ----------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `applicationId` (definitivo, §0-B)        | `com.adelpos.meseros`                                                                                                                                                                        |
| `namespace` de `app` (`R`, `BuildConfig`) | `com.adelpos.meseros`                                                                                                                                                                        |
| `namespace` de cada biblioteca            | `com.adelpos.meseros.core.data`, `….core.ui`, `….feature.access`, `….feature.waiter` (los módulos nuevos de la A2: `….core.capture`, `….feature.takeaway`)                                   |
| Paquetes de Kotlin                        | El `namespace` del módulo y sus subpaquetes; `core:domain` (JVM, sin `namespace`) usa `com.adelpos.meseros.core.domain…`                                                                     |
| Carpetas                                  | `<módulo>/src/<main\|test\|androidTest>/kotlin/com/adelpos/meseros/…` (el paquete dice la carpeta)                                                                                           |
| Pruebas instrumentadas                    | Paquete `com.adelpos.meseros.test` con el ejecutor `com.adelpos.meseros.AdelTestRunner`; las de las bibliotecas, `<namespace>.test` (`com.adelpos.meseros.core.data.test`, `….core.ui.test`) |
| Esquemas de Room                          | `core/data/schemas/com.adelpos.meseros.core.data.db.AppDatabase/` y `….catalog.db.CatalogDatabase/` (el nombre completo de la clase, como lo busca `MigrationTestHelper`)                    |

La 0.1.0 usaba el identificador provisional `local.adel.pos.android` (A1-D9). Para Android es **otra aplicación**: la
0.2.0 no la actualiza, se instala aparte, empieza con su propia base, sus propias llaves del Keystore y se empareja de
nuevo (la guía lo explica: [docs/ANDROID_INSTALAR.md](../docs/ANDROID_INSTALAR.md), «Pasar de la 0.1.0 a la 0.2.0»).
El nombre viejo solo aparece donde se nombra a propósito para desinstalarlo (`scripts/android-e2e.sh` y la lista de
paquetes que se desinstalan antes de las instrumentadas, en «Pruebas») y en esta explicación. El nombre visible
(«AD-EL POS»), `X-Client-Kind` (`android`) y la forma de identificar el dispositivo en Go (`device_id`) no cambiaron.

## Reglas permanentes

1. **La aplicación nunca calcula ni decide reglas de negocio.** Precios, impuestos, permisos y estados los decide Go;
   la aplicación muestra lo que Go entrega. Los precios del catálogo local son **referencia**: el importe final siempre
   lo calcula el servidor. Los importes son cadenas decimales (`Money`, `MoneySerializer`): nunca `Double` ni
   aritmética con ellos; se muestran con `MoneyFormat` (el formato único del escritorio y de Go).
2. **Textos en español** del restaurante, en `res/values/strings.xml` de cada módulo, sin tecnicismos (nada de IP,
   puertos, «servidor HTTP» ni códigos crudos; los datos técnicos solo en una sección de soporte), con los criterios del
   escritorio. **Ningún texto provisional o de desarrollo** (etapa A3, §1, A3-D11): nada de «todavía no…», «aún
   no…», notas de etapas o fases, «próximamente», «provisional», «por ahora», «más adelante», «en desarrollo»,
   «versión de prueba», «(muestra)» ni identificadores internos. Un estado real se dice sin ellas («No hay mesas
   guardadas», no «Todavía no hay…»). Lo vigila `PlainLanguageTest` (`core:ui`, JVM): lee los recursos de las carpetas
   `values` de **todos** los módulos (cada `<string>`, `<plurals>` y `<string-array>`, sin comentarios) y las
   cadenas literales de Kotlin de `src/main` (sin comentarios ni las líneas de `require`, `check`, `error(`, `Log.` y
   `…Exception(`). Una excepción solo se agrega a su lista `allowed` con el archivo, el patrón y su porqué; una que ya
   no encuentra nada hace fallar la prueba. Los mensajes de Go se muestran tal cual (los vigila la prueba de textos de
   Go).
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
    `onBackPressed` de la actividad. Los módulos sin `activity-compose` (`core:capture`) usan `SystemBackHandler` de
    `core:ui`. **Una sola flecha** (etapa A3, §4.1): la de `AdelTopBar(onBack)` llama a
    `onBackPressedDispatcher.onBackPressed()`, así pasa por los mismos `BackHandler` que la tecla Atrás (el último
    registrado gana: primero cierra el panel a un toque, después avisa lo que quedó sin enviar). Un panel a un toque
    de `AdaptivePanes` se registra en `LocalTopBarBack` (`TopBarBack`) y entonces no dibuja su propio «Volver a …».
12. **Botones táctiles**: 48 dp como mínimo y **56 dp** las acciones principales (`PrimaryButton`, `SecondaryButton`,
    `AdelDimens`); un solo botón principal por panel.
13. **El indicador de conexión siempre visible** en la barra superior común (`AdelTopBar` + `ConnectionIndicator`):
    «En línea», «Sin conexión» y «N pedidos por enviar». **Un solo estado de conexión** (etapa A3, A3-D1):
    `AppContainer.serverLink` (`ServerLinkMonitor` de `core:data` con las reglas puras de `ServerLinkTracker` en
    `core:domain`, `ServerLink` para quien lo lee). Lo alimentan la red de Android y **cada respuesta de Go** del
    cliente del servidor emparejado (`ServerAnswersInterceptor` en `PinnedHttpClient`, el punto único; cualquier
    respuesta HTTP es un éxito, una cancelación propia no cuenta). Lo leen la barra, «Selecciona tu usuario» y la
    entrada con PIN, Mesas, Para llevar, la precuenta, «Entregado» y la notificación del canal de avisos; **nadie más
    decide «sin conexión» por su cuenta** (una lectura que falló no es «Sin conexión»: se repite sola). Tolerancia: «Sin
    conexión» al instante sin red de Android, o con **3 fallas seguidas y 20 s sin ningún éxito**; «En línea» con el
    primer éxito. Revisión según el uso: sonda `GET health/live` cada 5 s mientras duda y cada 10 s con «Sin conexión»;
    ninguna con el canal de eventos abierto; cada 60 s sin él (se salta si hubo otro éxito, como la lista de «Selecciona
    tu usuario», que también es cada 60 s); nada en segundo plano; versiones y lista cada 5 minutos. Al tocar algo que
    necesita a Go con «Sin conexión» y red de Android, se pregunta en ese momento (`ServerLink.confirm`) antes de decir
    que falta conexión. Las constantes llevan su porqué en `ServerLinkTracker` y `PinnedHttpClient`. La causa de las
    desconexiones de la A2 (Fase 0 de la A3): Go cierra una conexión libre a los 60 s y OkHttp la guardaba 5 minutos sin
    revisarla antes de un `GET`; por eso el pool guarda las conexiones libres **30 s** y una lectura sin respuesta se
    repite **una vez** con una conexión nueva (nunca una escritura: llevan su `operation_id`; ni un canal ni un plazo
    vencido; `retryOnConnectionFailure` sigue en `false`).
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
20. **Lo que comparten Mesas y Para llevar vive en `core:capture`** (etapa A2): la captura, «Pendientes» y
    «Requiere tu revisión». `feature:waiter` y `feature:takeaway` solo aportan su entrada (la mesa o «Nuevo pedido») y
    su lista; ningún `feature` depende de otro y no se copia código de uno a otro.
21. **Un dato que se edita después de que el pedido salió a Go nunca cambia el pedido de la cola** (A2-D1): el cuerpo
    de un pedido o de un guardado sin respuesta es el mismo en cada reintento con su clave. El nombre y el teléfono de
    un pedido para llevar se guardan en la captura solo mientras nada ha salido (`Capture.customerEditable`); después,
    como una edición aparte (`takeaway_customer_edits`, con su propia `operation_id`, la última gana) que envía el
    mismo trabajador de la cola con la sesión del mesero que la hizo.
22. **Un solo canal de eventos y un solo canal de avisos por proceso** (etapa A2, Fase D): el de eventos
    (`RealtimeClient`, solo en primer plano y con sesión, Go admite dos por dispositivo) y el de platillos listos
    (`ReadyAlertsChannel`, con **solo la credencial del dispositivo**; Go mantiene uno por dispositivo: el nuevo
    reemplaza al anterior). El servicio en primer plano del aviso **no** abre el canal de eventos y **no envía
    pedidos**: al reconectar solo pide un envío de la cola
    (`retryNow`), como los demás disparadores. Un servicio en primer plano solo se arranca desde primer plano o con
    una exención (`BOOT_COMPLETED`, `MY_PACKAGE_REPLACED`, reinicio pegajoso); si Android lo rechaza
    (`ForegroundServiceStartNotAllowedException`), se espera al siguiente disparador. Un aviso nunca se muestra dos
    veces (las `key` avisadas en Room y en los extras de la notificación) y nunca se pierde (cursor en Room, guardado
    **después** de mostrar).

## Estructura

| Módulo             | Tipo                                     | Qué contiene                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| ------------------ | ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `app`              | aplicación (`com.adelpos.meseros`)       | `AdelPosApplication` + `AppContainer` (con el bloqueo por inactividad del proceso y el registro de borrado), `MainActivity` (de borde a borde), `ShellViewModel` (indicador), navegación (`navigation/AdelPosApp.kt`, rutas con tipo), `CatalogRealtimeContainer` (catálogo, imágenes y el único canal de tiempo real) y `AppForeground` (primer plano del proceso), `ready/` (aviso de platillos listos, A2: `ReadyAlertsController`, `ReadyAlertsService`, `ReadyAlertsBootReceiver`, notificaciones, permisos y ayuda), manifiesto, ícono, `src/debug` con `network_security_config`.                                                                                                                                                                                                                                                                                                                                                                     |
| `core:domain`      | Kotlin/JVM puro (sin Android)            | `Money` y `MoneyFormat`, `waiter/` (Mesas, cuentas, cola, captura; A3: `TableMapFit`, `CaptureTitle`, `DeviceHome`), `LinkState`/`ConnectionSummary` y `DefaultNetworkTracker`, `ServerLink`/`ServerAnswers` y `ServerLinkTracker` (el estado único de conexión, A3), `PendingOrdersSource`, `AppError`/`AppResult`, `SecretSealer`, `TableState`, `TablesView`; `server/` (dirección escrita, anuncio NSD, cambio de IP, versiones), `pairing/` (código de 6 dígitos, `PairingFlow`), `access/` (`DeviceAccess`, verificador del PIN, límite local, bloqueo por inactividad, `WaiterWork`), `catalog/` (modelos del catálogo, búsqueda sin acentos, `Customization`/`LineSelection`, reglas del personalizador y revisión de lo que declara el catálogo), `realtime/` (`RealtimeFeed`, señales, interrupciones de acceso, espera creciente), `ready/` (A2: avisos, texto, agrupación, sin duplicados, arranque y parada, canales). Pruebas JVM en segundos. |
| `core:data`        | biblioteca Android                       | `api/` (OkHttp con huella fijada, cabeceras del cliente, el punto único `ServerAnswersInterceptor` con el pool de 30 s y la repetición de una lectura, JSON, `MoneySerializer`, errores de Go → `AppError`, `ServerApi` con `ApiOutcome`), `access/` (`AccessRepository`, `PairingRepository`, `PairedConnection`), `discovery/` (NSD, sonda HTTPS), `security/` (huella fijada, primer certificado del emparejamiento, Keystore), `db/` (Room, esquemas en `core/data/schemas/`), `network/` (red de Android y `ServerLinkMonitor`), `pending/`, `preferences/`; `catalog/` (`catalog.db`, descarga completa y por cambios, caché de imágenes, actualizaciones), `realtime/` (cliente SSE), `session/` (`ServerAccess`, `DeviceServerAccess`), `ready/` (A2: cliente y canal de `ready-alerts` con la credencial del dispositivo y su registro en Room).                                                                                                    |
| `core:ui`          | biblioteca Android (Compose, Material 3) | Tema claro/oscuro (`theme/`), íconos propios (`icons/AdelIcons.kt`, los trazos de `desktop/src/components/Icon.tsx`), botones, barra superior e indicador, estado de mesa, toques de los diálogos para el bloqueo por inactividad (`LocalUserActivity`), andamio de uno o dos paneles (`layout/`), `catalog/` (búsqueda, categorías, cuadrícula con «Agotado», imagen con marcador y el personalizador táctil). Compartido con la A2.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `feature:access`   | biblioteca Android                       | Entrada al dispositivo (fases B y C): «Buscar el servidor» y código de emparejamiento, entrada con PIN, «Vuelve a escribir tu PIN», cambio de mesero (`AccessViewModel`), emparejar de nuevo, actualizar la aplicación o el servidor, dispositivo retirado y su bitácora, permiso de red local. Solo usa interfaces de `core:domain`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `core:capture`     | biblioteca Android (A2)                  | La captura compartida de Mesas y Para llevar (`com.adelpos.meseros.core.capture`, movida con `git mv` de `feature:waiter` en la Fase B de la A2): `CaptureScreen`/`CaptureViewModel` (catálogo y personalizador de `core:ui` con la cuenta al lado o a un toque: lo enviado con su preparación, modificar o cancelar unidades, lo por enviar y «Enviar a cocina»; en un pedido para llevar, su cliente y «Nombre y teléfono»), `OrderPanel`, `SentLineDialogs`, `Takeaway.kt` (`CustomerDialog` y cómo se nombra un pedido para llevar) y `pending/` («Pendientes» con los cinco estados y «Requiere tu revisión»); `CaptureDependencies` (solo interfaces de `core:domain`).                                                                                                                                                                                                                                                                                |
| `feature:waiter`   | biblioteca Android                       | Modo Mesero (Fase E): `tables/` (Mesas en lista o mapa con su tiempo, las áreas como pestañas y el panel de la mesa con sus cuentas, su precuenta, «Descartar cuenta vacía» y «Nueva cuenta»); `WaiterDependencies` (la `CaptureDependencies` de `core:capture` más la vista de Mesas y el reloj). La pantalla de muestra «Catálogo» de la Fase D se quitó en la A3 (A3-D10).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `feature:takeaway` | biblioteca Android (A2)                  | Para llevar (Fase B de la A2): `TakeawayScreen`/`TakeawayViewModel` (la lista de `TakeawayBoard` con folio, nombre, tiempo, preparación y pago de Go, lo que falta enviar y, sin conexión, lo último con su hora; «Nuevo pedido», «Nombre y teléfono» y «Entregado» o «Cobrar en caja antes de entregar»); `TakeawayDependencies`. La captura es la de `core:capture`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |

Dependencias entre módulos: `app` → todos; `feature:*` → `core:ui` (→ `core:domain`) y, `feature:waiter` y
`feature:takeaway`, `core:capture` (→ `core:ui`); `core:data` → `core:domain`. Un módulo `feature` nunca depende de otro
`feature`.

**Versión de la aplicación**: un solo lugar, `android/gradle.properties` (`adelpos.versionName`, `adelpos.versionCode`;
0.2.0 y 2 en la A2; desde la A3, **0.3.0 y 3**). La usan el APK y la cabecera `X-Client-Version`
(`BuildConfig.VERSION_NAME` → `ClientIdentity`).

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
  paneles, colores solo en la paleta y contraste AA de los tokens, `SavedStateHandle` de Mesas. Etapa A3:
  `ServerLinkTrackerTest` (tiempo virtual: una falla aislada no cambia, varias en 20 s sí, el primer éxito vuelve,
  sin red al instante, la revisión según el uso), `PinnedHttpClientTest` (MockWebServer con TLS y HTTP/1.1 como Go:
  una conexión libre más que el pool no se reutiliza, un `GET` sobre una conexión cerrada se repite una vez y un
  `POST` no), `AccessRepositoryTest` con una falla aislada (la entrada sigue con sesión de Go) y `PlainLanguageTest`
  (regla 2). Los servidores falsos (`FakeGoServer`, `PinnedHttpClientTest`) hablan HTTP/1.1 sin ALPN, como la
  escucha de red de Go: con HTTP/2 una conexión cerrada se ve distinto. Ajustes de pantallas (A3, §4):
  `StageA3PanelRulesTest` (los casos del mapa de `desktop/src/components/restaurant/TableMap.test.tsx`, el título de
  la captura, la pantalla inicial), `TablePanelAccountsTest` (`discardable` y «Descartar cuenta vacía» con
  MockWebServer), `PrecheckRepositoryTest` (`home_screen`) y `WaiterViewModelsTest` (precuenta y descarte desde el
  panel, el área elegida).
- **Robolectric** (SDK 36, `src/test/resources/robolectric.properties`): Room. Cada SDK adicional descarga ≈ 224 MB.
- **Catálogo y tiempo real (Fase D)**: JVM del dominio (búsqueda, personalizador, revisión, descripción, espera
  creciente); `CatalogSyncTest` y `CatalogImageCacheTest` (Robolectric + MockWebServer con los JSON del contrato de
  `core/data/src/test/resources/catalog/`: completa, cambios, `deleted`, archivados, `reset`, agotados, búsqueda,
  transacción, 304); `CustomizationJsonTest` (la forma exacta de `customization`/`LineRequest`); `RealtimeClientTest`
  (cursor de `changes`, `interrupted` con `id: 0`, `reset`, espera creciente, conexión muerta, sesión nueva, segundo
  plano); `CatalogUpdateTriggersTest`; `DeviceServerAccessTest` (con el Go simulado de la entrada). Instrumentadas:
  `CatalogDatabaseMigrationTest` (`MigrationTestHelper` y reemplazo en una transacción) y `CatalogComponentsTest`
  (`core:ui`).
- **Instrumentadas** (`connectedDebugAndroidTest`): `MigrationTestHelper`, Keystore, Compose (tema, indicador,
  estados de mesa, botones, paneles) y la aplicación completa (navegación, uno o dos paneles según el dispositivo,
  rotación real con `UiAutomation.setRotation` sin perder lo capturado, indicador con el modo avión real). Etapa A3:
  `ServerLinkScreensTest` (la barra y los avisos de «Selecciona tu usuario», Mesas y Para llevar siempre iguales con un
  estado forzado; `FakeAccess.link`). Con `TestDevice` (servidor emparejado que no responde) «Sin conexión» llega con la
  tolerancia (unos 20–30 s), no con la primera falla: las esperas de esas pruebas son de 60 s. Ajustes de §4:
  `StageA3ScreensTest` (la tarjeta de cuenta en dos renglones a 320, 360, 400, 600 y 840 dp, la flecha y el logotipo de
  la barra, la búsqueda en un renglón, la tarjeta sin imagen, los textos de la entrada, la fila de Mesas y el mapa que
  llena el espacio) y `DeviceLayoutTest` (la flecha de Pendientes y Lista | Mapa con rotación real).
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
  Resultados en `android/app/build/outputs/e2e/<serial>/`. **Nunca** contra el servicio real de la Mac. El guion
  nombra la aplicación con `APP_ID` (`com.adelpos.meseros`), `TEST_PACKAGE` y `TEST_RUNNER` (clase de prueba,
  componente de `am instrument` y carpeta de capturas) y, antes de instalar, desinstala también la 0.1.0
  (`LEGACY_APP_ID`, `local.adel.pos.android`, y su `.test`).
- **`CatalogOnGoTest`** (`app`, contra el Go temporal; se omite sin argumentos) se corre con
  **`scripts/android-e2e.sh --suite catalog`** (etapas `a`, `b` y `c`, una instrumentación cada una con `-e stage`):
  `a` empareja y entra por la API, crea el catálogo (≈ 300 productos, con sus tiempos en el registro
  `CatalogOnGoTest`), cambia precio, crea, archiva y agota desde la principal (por el canal y por cambios) y revalida la
  imagen (304); `b` quita el producto de prueba («Filete de prueba», `FILETE`) con «Quitar datos de prueba» de la
  principal (`maintenance/test-data`, la API de Go) y lo recibe en `deleted`; antes de `c` el guion cambia la época del
  catálogo en la base temporal (`UPDATE app.catalog_epoch SET epoch = gen_random_uuid()`, como una restauración: Go no
  tiene API para eso) y `c` recibe `reset: true`. Ese SQL solo corre con `ADELPOS_E2E_DATABASE_URL` y sus guardas
  (127.0.0.1, nunca el puerto 55432, base `adel_pos_test_…` con la terminal principal del servidor que se prueba); sin
  ella `c` sale como omitida.
- **`FieldOnGoTest`** (Fase F, `--suite field`, pasos `f1`–`f10`): cocina en pantalla configurada por la API de la
  principal (estación y ruta) con «En preparación» y «Listo» al momento; rendimiento (arranque en frío, Mesas con 40
  mesas, captura con ~300 productos, «Enviar a cocina» y `dumpsys meminfo`, en el registro `AdelOnGo` con «Fase F ·»);
  dos meseros en el mismo dispositivo (captura en curso que el otro no ve, cambio impedido con pedidos «Por enviar» y
  cada pedido con la credencial de su mesero); transferir y combinar desde la principal con la cuenta abierta; bloqueo
  largo del PIN (A1-D16: el guion deja dos bloqueos previos de la serie de `beto.f` en la base temporal, con las mismas
  guardas) que cierra también la entrada sin conexión; la actividad recreada sin sus ViewModel (como tras la muerte del
  proceso) con la captura y el personalizador abiertos; Atrás en Mesas y el bloqueo por inactividad al volver; tema
  oscuro con capturas (`<salida>/screens`) y zonas táctiles de 48 dp. `-e tableOffset N` usa otras mesas de `f1` para
  repetir un paso en el mismo servidor.
- **`ConnectionOnGoTest`** (etapa A3, §2 y §6, `--suite connection`, **fuera de `all`** porque dura; paso `c1`):
  empareja, entra un mesero y deja el dispositivo sin uso `ADELPOS_E2E_CONNECTION_MINUTES` minutos (10 por omisión;
  la Fase E corre 60, «una hora sin uso sin avisos falsos») con el bloqueo por inactividad de fábrica: Mesas, a los 2
  minutos «Selecciona tu usuario» y el mesero vuelve a entrar 45 s después, una y otra vez; con el aviso de platillos
  listos encendido. Cada segundo registra (`AdelConexion`) cada cambio de la barra, de los avisos de «Selecciona tu
  usuario» y de Mesas, de la notificación y del estado único (que registra sus cambios y fallas en `AdelLink`), y exige
  cero: ningún segundo con «Sin conexión», ninguna petición sin respuesta (`ServerLinkMonitor.noAnswers`), ninguna
  lectura repetida (`repeats`) y cada vuelta con sesión de Go. El paso tiene tiempo límite (sus minutos más 15) y el
  guion agranda el registro del emulador. Con la Mac sin carga y sin Gradle compilando.
- **`AdjustmentsOnGoTest`** (etapa A3, §4, `--suite adjustments`, pasos `a1`–`a6`, con rotación real y capturas de
  pantalla en `<salida>/screens`): `a1` arma el restaurante por la API de la principal (dos áreas con mesas, catálogo
  sin imágenes, una cuenta vacía), empareja y entra; `a2` Mesas en una fila, el mapa, el panel con la flecha y las
  cuentas en dos renglones; `a3` «Imprimir precuenta» desde el panel (llega a Go: sin impresora, Go lo dice) y
  «Descartar cuenta vacía» con confirmación (Go la descarta); `a4` la captura (título, «Buscar producto», tarjetas
  sin imagen, «Ver la cuenta · 1 por enviar», el aviso al salir con la flecha, Pendientes); `a5` la entrada; `a6` la
  pantalla inicial Para llevar cambiada en Terminales (`PATCH` del dispositivo desde la principal). El
  `ACCOUNT_NOT_EMPTY` del descarte se prueba en la JVM (`TablePanelAccountsTest`).
- **Ejecutor de pruebas de `app`**: `com.adelpos.meseros.AdelTestRunner` (`testInstrumentationRunner` de
  `app/build.gradle.kts`; paquete de pruebas `com.adelpos.meseros.test`). Antes de crear el contenedor, busca por NSD
  el tipo `_adelpostest._tcp` y toma `clientVersion` de los argumentos. Ayudantes: `TestDevice` (un servidor
  emparejado que no responde y un mesero conocido para entrar sin conexión, con el Keystore real), `FakeAccess`
  (entrada falsa para las pantallas con estados forzados) y `MainComputer`.
- **`testOptions.targetSdk = 36` en las bibliotecas que tocan la red** (`core:data`): el APK de pruebas de una
  biblioteca toma `compileSdk` (37) si no se indica, y en Android 17 un `targetSdk` 37 exige `ACCESS_LOCAL_NETWORK`
  (visto: `SecurityException: Missing local network permission` al anunciar por NSD). Se prueba con el nivel de la
  aplicación (A1-D7).
- **Matriz** (avance §1.7 y Fase F): `Pixel_8` y `Pixel_Tablet` con Android 17, `ADEL_A1_Phone_API33` y
  `ADEL_A1_Tablet_API26`. Emuladores sin ventana y de solo lectura con `iso.sh - emu-up <avd> <consola>`; como mucho
  dos a la vez (con dos corriendo de punta a punta y Gradle compilando, la entrada con PIN llegó a tardar 12 s y hubo
  fallas por tiempo: mejor no compilar mientras corren); `emu-down` al terminar.
  En cada uno: `connectedDebugAndroidTest` con `ANDROID_SERIAL` (contar los XML de cada módulo) y **`ANDROID_SERIAL=…
ADELPOS_E2E_NEW_SERVER=<orden> scripts/android-e2e.sh --suite all`**, que levanta un servidor temporal nuevo para
  cada parte (access 1–5; access 6–7 con otra identidad sin reinstalar; catalog; waiter; field; takeaway; precheck;
  alerts) con la orden
  de `ADELPOS_E2E_NEW_SERVER` (detiene el anterior, levanta otro en las mismas direcciones e imprime líneas
  `ADELPOS_E2E_*=…`: provisioning y base temporal). Los agentes usan como orden `iso.sh - server-down` seguido de
  `iso.sh <árbol> server-up`. Resultados en `<salida>/<suite>/` y `<salida>/summary.txt`.
- **Corte de red**: `cmd connectivity airplane-mode enable|disable` en Android 11+ (desde la prueba con
  `UiAutomation.executeShellCommand`); en Android 8 solo desde la Mac con `adb root` + `svc wifi disable` +
  `svc data disable` (imágenes `google_apis`; las de Play no permiten root).
- **Ojo**: con AGP 9.5.1, si el APK no se puede instalar (por ejemplo, otra firma de la misma aplicación ya instalada),
  `connectedDebugAndroidTest` lo escribe en el registro pero **termina con BUILD SUCCESSFUL sin correr las pruebas** de
  ese módulo. Comprobar siempre los resultados en `*/build/outputs/androidTest-results/connected/` (contar los XML
  de cada módulo) y desinstalar antes, con `adb -s <serie> uninstall`, la aplicación y sus pruebas
  (`com.adelpos.meseros`, `com.adelpos.meseros.test`, `com.adelpos.meseros.core.data.test`,
  `com.adelpos.meseros.core.ui.test`; `core:capture` y `feature:takeaway` no tienen pruebas instrumentadas propias) y
  las de la 0.1.0 (`local.adel.pos.android`, `local.adel.pos.android.test` y sus bibliotecas `….core.data.test` y
  `….core.ui.test`): un APK de entrega firmado instalado a mano es «otra firma»
  de la misma aplicación para el `debug`.
- **Ojo (emulador cargado)**: con dos emuladores y Gradle a la vez, el lanzador del emulador puede mostrar «Pixel
  Launcher isn't responding»; ese diálogo se queda con el foco, la tecla Atrás que inyecta la instrumentación ya no
  llega a la aplicación y Espresso falla con «has-window-focus=false». Las pruebas de punta a punta lo cierran con
  «Wait» antes de Atrás y de cada captura (`SystemDialogs`); si aparece en las instrumentadas, repetirlas con el
  emulador recién encendido y solo.

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
- **Versiones** (`ServerCompatibility.check`: servidor 0.13.0 o posterior con `waiter-pin-v1` y, desde la A2,
  `takeaway-v1`, `device-precheck-printer-v1` y `ready-alerts-v1` —A2-D6: cada fase agrega la capacidad que usa—, y
  la mínima de Android de `min_client_versions`): se
  comprueba con `GET version` **antes de pedir el código de emparejamiento** (también con la dirección escrita; `PairingRepository.request`) y en cada comprobación ya emparejado. Un servidor viejo da
  «Actualiza el servidor del restaurante» con «Volver a comprobar» (`PairingProblem.UpdateServer` al emparejar,
  `DeviceStatus.UpdateServer` después). Un 426 o un tipo de cliente desconocido durante el emparejamiento de un
  servidor que no informa la mínima de Android también es un servidor viejo, nunca «Actualiza esta aplicación».
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
- **Tiempo real** (`catalogRealtime.realtime`, `RealtimeFeed`): **un solo canal de eventos por proceso**, abierto solo en
  primer plano y con sesión; nunca abrir otro (Go admite dos por dispositivo). El canal de avisos de platillos listos
  es otro, aparte (regla 22). La Fase E se suscribe con
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
- **Contador del indicador**: todos los pedidos sin enviar del dispositivo; «Pendientes» muestra solo los del
  mesero activo y, de los demás, solo cuántos son.
- Pruebas: `OrderQueueRulesTest`, `WaiterRulesTest` (dominio); `OrderQueueTest`, `CaptureSyncTest` (Robolectric +
  MockWebServer); `WaiterViewModelsTest`; instrumentadas `WaiterScreensTest`, `WorkNetworkOnDeviceTest` y, contra el
  Go temporal, `WaiterModeOnGoTest` con **`scripts/android-e2e.sh --suite waiter`** (pasos `w1`–`w13c`; el guion corta
  la red entre pasos y en `w4` la prueba pierde una respuesta con `lostOfflineResponses`, que lee `AdelTestRunner`;
  desde la revisión R6, `w9`–`w13c` prueban A1-D19: `w9` pierde la respuesta del guardado con `lostDraftResponses`,
  `w10` las de «Enviar a cocina», `w11`–`w12c` el descarte con y sin conexión y `w13a`–`w13c` la cuenta combinada
  mientras el pedido espera). En la JVM, la cola de A1-D19: `OfflineNeverRepeatsTest`, `R6RulesTest` y
  `OrderSchedulerTest`.

## Para llevar (etapa A2, Fase B, para los demás módulos)

Un pedido para llevar es una cuenta de Go con `mode: "takeaway"` y sin mesa (no hay mesa ficticia). Es la misma
captura y la misma cola de la A1, con la modalidad en la captura, el pedido y el guardado sin respuesta
(`OrderMode`, `Capture.tableId` opcional) y el cliente (`TakeawayCustomer`, A2-D1).

- **Acceso** (§3.1): la navegación principal (`navigation/AdelPosApp.kt`: `MainNavigationBar` abajo en vertical,
  `MainNavigationRail` a un lado con poca altura o en horizontal; ver «Diseño») muestra «Mesas» y «Para llevar» solo
  si `WaiterPermissions.grants(VIEW_TAKEAWAY)` (`takeaway.read` en los permisos de Go del mesero
  activo; con permisos desconocidos no aparece). «Nuevo pedido» y capturar exigen `takeaway.manage` (`OPEN_TAKEAWAY`,
  `MANAGE_TAKEAWAY`); sin él, solo la lista.
- **Captura** (`CaptureService.openTakeaway`, `setCustomer`): «Nuevo pedido» entra directo al catálogo; el pedido nace
  con el primer producto (`openAccount` con `mode: takeaway`, `origin` «Para llevar», `customer_name` y `phone`,
  `WaiterMapper.openBody`) o, sin conexión, al enviarse (`submitOfflineOrder` con `destination.takeaway`). Salir sin
  productos no deja nada (`closeIfIdle`; el cliente solo no es trabajo pendiente). `items_in_account` se reenvía con la
  misma clave a `destination.account_id`, como en una mesa. Las capturas para llevar cuentan para `WaiterWork`.
- **Nombre y teléfono** (A2-D1, regla 21): en la captura mientras `customerEditable`; después, `takeaway_customer_edits`
  (`CustomerEditEntity`, una fila por pedido con la última edición), que el trabajador de la cola envía con
  `updateTakeawayCustomer` y la sesión de ese mesero (`CustomerEditSender`, después de los pedidos). Una edición de un
  pedido que todavía no nace en Go espera a que la captura o la cola sepan su cuenta (`bindCustomerEdit`). Se borra solo
  si Go confirmó esa clave o la rechazó para siempre; nunca se pierde por falta de conexión. Límites de Go en bytes del
  texto recortado (100 y 40, `TakeawayCustomer.fitsName`/`fitsPhone`).
- **Lista** (`TakeawayService`, `TakeawayRepository`; `TakeawayBoard.entries`): los pedidos para llevar abiertos de
  `restaurant/state` (la misma lectura de Mesas, guardada en `restaurant_snapshot`, con su hora en `TakeawayRead`), al
  momento con los eventos del ámbito `takeaway` del único canal, y encima lo del mesero activo que todavía no nace en Go
  (la cola y las capturas sin enviar; de los demás meseros nunca el detalle). Preparación, pago y entrega como los da
  Go (`TakeawayStates`).
- **Entregado** (A2-D2): `TakeawayStates.deliveryOffer`: solo con `takeaway.deliver` y `payment_state == "paid"`; sin
  pagar, «Cobrar en caja antes de entregar»; sin el permiso, solo el estado. `deliverTakeaway` con conexión y la sesión
  del mesero (nunca va a la cola; sin respuesta se repite una vez con la misma clave); `BALANCE_PENDING` → el mismo
  texto, `ALREADY_DELIVERED` → volver a leer. El botón y «necesitas conexión» siguen al estado único (A3); sin
  respuesta con «En línea», «No se pudo marcar entregado».
- Pruebas: `TakeawayRulesTest` (dominio), `TakeawayQueueTest` (Robolectric + MockWebServer con las formas del
  contrato), `TakeawayViewModelTest` (`feature:takeaway`), `WaiterViewModelsTest` (el cliente en la captura);
  instrumentadas `TakeawayScreensTest`, `AppNavigationTest` (Mesas ↔ Para llevar) y el caso 5 → 6 de
  `AppDatabaseMigrationTest`; contra el Go temporal, `TakeawayOnGoTest` con **`scripts/android-e2e.sh --suite
takeaway`** (pasos `t1`–`t8`; el guion corta la red en `t4`; la prueba hace de cocina y de caja por la escucha
  local).

## Precuenta desde el dispositivo (etapa A2, Fase C, para los demás módulos)

«Imprimir precuenta» en la cuenta de una mesa y en un pedido para llevar, la misma pieza de `core:capture`
(`PrecheckPrintBar`, al final de `OrderPanel`), solo con la cuenta ya en Go. Desde la A3 (§4.9) también en cada
cuenta del panel de la mesa (`PrecheckPrintBar(framed = false)` con un `PrecheckPrinting` en `TablesViewModel`: una
impresión siguiéndose a la vez). Sin `payments.tickets.print` en los permisos de Go del mesero
(`WaiterAction.PRINT_PRECHECK`, con `grants`) el botón se ve deshabilitado con «Tu usuario no tiene permiso para
imprimir la precuenta…» (`PrecheckSection(allowed = false)`); antes de la A3 no aparecía. Go decide todo
(impresora, contenido con «Atendió: <mesero>», permiso y estado); el dispositivo pide, sigue y lo dice.

- **Nunca entra en la cola sin conexión** ni usa la credencial de envío: `PrecheckRepository` (`core:data/waiter`)
  pide con la **sesión** del mesero; sin sesión o con «Sin conexión» (`connected`, el estado único de la barra, A3;
  con red de Android se pregunta antes en el momento) responde `PrintAnswer.NoConnection` y el botón dice «Para
  imprimir necesitas conexión con la computadora principal». Una petición sin respuesta se repite una vez con la
  **misma** `operation_id` (Go no imprime dos); si tampoco responde y el estado sigue «En línea»,
  `PrintAnswer.NoAnswer` («No se pudo imprimir. Intenta de nuevo.»: nunca «necesitas conexión» con la barra en «En
  línea»).
- **Nombre visible e impresora** (`PrecheckSetup`): `establishment.precheck_name` y `terminal.precheck_printer` de
  `getIdentity`, leídos al entrar un mesero con conexión, al abrir una cuenta y después de imprimir; guardados en
  `restaurant_snapshot` con el tipo `precheck` (sin cambiar el esquema de Room; los borra `WaiterDao.eraseAll`, ya en
  `deviceDataWipe`) para verse sin conexión. El botón dice «Imprimir precuenta», «Imprimir ticket de venta» o
  «Imprimir cuenta» (`PrecheckName`); sin impresora asignada se avisa antes de tocar, pero la respuesta de Go manda.
- **Seguimiento** (`PrecheckPrinting`, `core:domain`, en el `viewModelScope` de la captura): `getDocumentStatus` cada
  2,5 s durante 15 s como máximo, sin bloquear la captura, y termina al salir de la cuenta o al cambiar de cuenta.
  `PrecheckFeedback` traduce las claves de Go (`kitchen/print_problems.go`) a `PrintMessage`: «Enviada a Caja 1»,
  «La Caja 2 está desconectada» (`agent_offline`), «La impresora no responde» (`unreachable`), «La impresora necesita
  revisión», revisar el papel (`uncertain`), «Este dispositivo no tiene impresora asignada» (409
  `PRINTER_NOT_CONFIGURED` con `scope: device`, A2-D3), `PRINTER_UNAVAILABLE` con el mensaje de Go, sin permiso.
- **Reintentar** (`retryDocument`, sin motivo) solo si el trabajo **nunca salió en papel** (`error`, `intervention`;
  regla de la 9F), nunca con `uncertain` ni con la terminal desconectada; `PRINT_NOT_FAILED`, `PRINT_UNCERTAIN` y
  `PRINT_JOB_SUPERSEDED` vuelven a leer el estado. Volver a imprimir algo que ya salió es tocar el botón otra vez:
  otra precuenta con otra clave, como en la caja (no hay copia con motivo en el dispositivo).
- Pruebas: `PrecheckRulesTest` (dominio: cada clave y código → su aviso, «Reintentar», nombre visible, el
  seguimiento con tiempo virtual), `PrecheckRepositoryTest` (Robolectric + MockWebServer con los JSON del contrato),
  `WaiterViewModelsTest.capturaImprimeLaPrecuentaYSigueElTrabajo`; instrumentada `PrecheckScreensTest` (cada aviso
  con su texto en ambos temas); contra el Go temporal, `PrecheckOnGoTest` con **`scripts/android-e2e.sh --suite
precheck`** (pasos `p1`–`p7`; la prueba hace de principal y del agente de impresión de una terminal de prueba «Caja 2»
  y revisa los bytes ESC/POS; el guion corta la red en `p6`; la impresora de red que «no responde» usa el puerto 9106
  de la Mac).

## Ajustes de pantallas (etapa A3, §4, para los demás módulos)

- **Barra** (`AdelTopBar`): `onBack` dibuja la flecha (regla 11; en Captura, Pendientes, Revisión, la cuenta a un
  toque y Para llevar con algo detrás); `logo` muestra el logotipo de `assets/brand/` (PNG reducidos en
  `core/ui/src/main/res/drawable-nodpi/`, claro u oscuro según `AdelTheme.colors.isDark`) solo en la entrada.
- **Título de la captura** (`CaptureTitle.of`, `core:domain`): «Mesa 4», «Para llevar #12» (el folio de Go solo si es
  de esa cuenta), «Para llevar · Nombre» o «Nuevo pedido para llevar». `CaptureRoute(onTitleChange)` lo da a la raíz.
- **Salir con productos sin enviar**: `CaptureRoute(onLeaveUnsent)` con «Lo capturado en Mesa 4 quedó guardado sin
  enviar a cocina…» (regla A1: la captura se queda con su mesero); la raíz lo muestra en su `SnackbarHost` al llegar
  a la pantalla a la que se vuelve y lo quita al pasar a otra (en el teléfono tapaba «Ver la cuenta»).
- **Mesas** (`TablesScreen`): una fila con «Lista | Mapa» (`ViewSwitch` propio: el `SegmentedButton` de Material
  reserva 26 dp para un ícono en cada opción y en el teléfono vertical no dejaba lugar a las áreas), las áreas
  (`FilterChip` desplazables, la elegida siempre completa a la vista, el área elegida en `SavedStateHandle`, la lista
  también por área) y «Pendientes»; sin búsqueda de mesas. El mapa (`TablesMap`) usa `TableMapFit.of` (puerto de
  `fitTables` del escritorio: escala por el rectángulo de las mesas del área, lado menor ≥ 64, escala ≤ 2, margen 16,
  desplazamiento en ambos ejes si no caben) y llena su espacio.
- **Cuentas del panel** (`AccountCard`): dos renglones, «Mesa 6 · folio 12 · $755.00» en una línea con elipsis y
  abajo la etiqueta (todas las etiquetas de estado con `maxLines = 1`), en cualquier ancho. Con «Imprimir precuenta»
  (arriba) y, si Go dice `discardable`, «Descartar cuenta vacía» (`RestaurantSource.discardEmptyAccount`:
  `POST restaurant/accounts/{id}/discard` con `operation_id` y `revision`, con conexión y la sesión del mesero, nunca
  en la cola; sin respuesta se repite una vez con la misma clave; `ACCOUNT_NOT_EMPTY` o `ACCOUNT_CONFLICT` → volver a
  leer; sin `accounts.open` + `accounts.read`, el botón deshabilitado con su porqué). Go decide si se puede.
- **Pantalla inicial** (A3-D7): `terminal.home_screen` de `getIdentity`, guardado con la precuenta
  (`PrecheckSetup.home`); `DeviceHome.fromWire`: solo `takeaway` abre Para llevar, y solo con `takeaway.read`
  (`DeviceHome.startsInTakeaway`); cualquier otro valor o ninguno, Mesas. La raíz lo aplica una vez por entrada, si el
  mesero no eligió otra sección.
- **Catálogo**: «Buscar producto» en un renglón siempre; un producto sin imagen usa la tarjeta compacta (nombre y
  precio, `TAG_PRODUCT_WITHOUT_IMAGE`).

## Aviso de platillos listos (etapa A2, Fase D, para los demás módulos)

Cuando cocina marca «Listo» en el KDS, Go registra el aviso (`app.ready_notices`) y decide a quién llega (A2-D4,
A2-D9); el dispositivo lo recibe con **solo su credencial** (`GET ready-alerts?after=` y `ready-alerts/stream`, SSE con
`Last-Event-ID`), aunque esté en «Selecciona tu usuario», con la pantalla apagada o la aplicación cerrada. El texto lo
arma la aplicación (A2-D10) con las plantillas de `app/src/main/res/values/strings.xml`: «Mesa 4 · 2 platillos listos» /
«2 × Filete de res».

- **Dónde vive**: la lógica pura en `core:domain/ready` (`ReadyAlertsProcessor`: quitar lo ya avisado, mostrar,
  guardar; `ReadyNotice`: una notificación por cuenta con el acumulado; `ReadyAlertWords`: el texto con singular y
  plural; `ReadyAlertsLifecycle`: cuándo arrancar, el fin del día, qué hacer con cada error y la espera creciente de 2
  a 30 s; `ReadyChannels`, A2-D7). La red y Room en `core:data/ready` (`ReadyAlertsClient`, `ReadyAlertsChannel` —el
  único canal de avisos del proceso—, `RoomReadyAlertsLedger`, `DeviceReadyAlertsServer`). El servicio, las
  notificaciones, los permisos y la ayuda en `app/ready` (`AppContainer.readyAlerts`, `ReadyAlertsController`).
- **Servicio en primer plano** (`ReadyAlertsService`, A2-D8): tipo `specialUse` (con su subtipo en el manifiesto) y
  la notificación permanente «AD-EL POS conectado» (canal `conectado`, baja importancia; al tocarla, la ayuda). Solo
  sostiene el proceso: el canal y las decisiones son de `ReadyAlertsController`. `startForeground` es lo primero de
  `onStartCommand` (también con `intent == null`, el reinicio pegajoso); el tipo se pasa desde la API 34 (en 29–33,
  ninguno; antes no existe). `START_STICKY`. Quitar la aplicación de recientes no lo detiene.
- **Arranca** (`ReadyAlertsController.request`, decidido por `ReadyAlertsLifecycle.decide` con lo guardado): al entrar
  un mesero con conexión, al abrir la aplicación con el dispositivo listo (si lo último de Go fue `active: true` y no
  terminó el día; si no, pregunta antes con `GET ready-alerts` sin mover el cursor) y desde `BOOT_COMPLETED` /
  `MY_PACKAGE_REPLACED` (`ReadyAlertsBootReceiver`, solo con esa condición guardada). Sin destinatarios en el día no
  arranca: no hay notificación permanente.
- **Se detiene** cuando el canal termina: `active: false` (nadie entró hoy o «Avisar platillos listos» apagado; al
  pasar `day_ends_at` vuelve a preguntar con `GET` y, mientras Go diga `active: true`, sigue); **A2-D12**: pasados
  10 minutos de `day_ends_at` (`DAY_END_STOP_MILLIS`) sin alcanzar a Go desde entonces y sin nadie usando el
  dispositivo (aplicación a la vista con un mesero), sin mover el cursor (`ReadyStopReason.DAY_ENDED`; también tras el
  reinicio pegajoso; lo vuelve a arrancar la siguiente entrada con conexión o abrir la aplicación). En Go (A2-D12) una
  sesión de antes del inicio del día solo cuenta si se usó en los últimos 30 minutos: **en «Selecciona tu usuario» nada
  usa la sesión del último mesero** (canal de eventos, catálogo y mesas van con `ServerAccess.current`, null sin mesero
  activo; la cola solo envía lo pendiente de cada dueño; el aviso va con la credencial del dispositivo), así que de
  noche Go responde `active: false` solo (`r10` lo comprueba); el dispositivo retirado (solo `DEVICE_REVOKED`, por
  `interrupted` o un 401), otra huella o 426 (se avisa a `deviceAccess.report`) o un rechazo permanente al abrir
  (403/400); un 401 con otro `DEVICE_*` al abrir se avisa como en las demás rutas (emparejar de nuevo); cualquier otro
  código de `interrupted` (`TEMPORARY`) o un 5xx es temporal (espera creciente). También cuando el dispositivo deja de
  estar listo y con «borrar los datos de este dispositivo» (registrado en `deviceDataWipe` como «avisos de platillos
  listos»: detiene canal y servicio, borra Room y quita los avisos a la vista).
- **Sin duplicados ni pérdidas** (§4.1): Room guarda el cursor y lo último de Go (`ready_alerts_state`) y los avisos ya
  mostrados del día por la **`key`** de Go, única y nunca reutilizada (`ready_alerts_seen`; el `id` solo es el cursor,
  que Go puede repetir tras restaurar un respaldo; al cambiar de día se borran los anteriores). Cada lote:
  quitar lo ya mostrado → mostrar → guardar ids y cursor en una transacción. Si el proceso muere entre mostrar y
  guardar, el aviso vuelve a llegar y la notificación de su cuenta lo reconoce en sus extras (por su `key`: no vuelve a
  sonar). Sin cambios no escribe en Room.
- **Reconexión**: Go manda solo lo que cambia, `: ping` cada 15 s y cierra cada flujo a los 5 minutos: un cierre normal
  (duró o trajo mensajes) se reabre **enseguida** desde el último cursor (`after` y `Last-Event-ID`); sin red, 5xx,
  `interrupted` temporal o conexión muerta (45 s sin un byte), espera creciente; 429 (no debería pasar: un canal nuevo
  reemplaza al anterior) lee con `GET` mientras espera; tras tres fallas pide buscar el servidor en otra dirección
  (`deviceAccess.recheck`, cambio de IP); al volver la red, reconecta enseguida. Un lote `alerts` que no se entiende
  o algo inesperado al procesarlo (Room, el sistema de notificaciones) es una falla: se reconecta desde el último
  cursor guardado, sin saltarlo; el controlador corre el canal con `CoroutineExceptionHandler` y lo reintenta con
  espera creciente (nunca tumba el proceso ni la captura). La notificación permanente dice «Conectando…» mientras el
  canal no está abierto, «Recibiendo avisos…» solo con el canal abierto y el texto de sin conexión **solo con el
  estado único en «Sin conexión»** (A3: `ReadyOngoing.of(canal, estado)`; una falla del canal que se reconecta sola no
  basta; se vuelve a publicar solo cuando cambia el texto). El protocolo del canal no cambió en la A3. `ReadyAlertsBootReceiver` decide solo con lo
  guardado en Room, sin preguntar a Go (el servicio comprueba el dispositivo al arrancar).
- **Notificaciones**: un solo canal visible «Platillos listos» cuyo id depende del sonido y la vibración de Terminales
  (A2-D7); una notificación por cuenta (etiqueta = la cuenta) con el acumulado, y el resumen del grupo con dos o más.
  Al tocarla, `MainActivity` recibe la cuenta (`ReadyAlertsController.pendingOpen`): en «Selecciona tu usuario» la
  entrada con PIN deja elegido a su mesero y, al entrar (él u otro que pueda ver la cuenta), se abre su captura
  (`ReadyAlertsNavigationEffects`); si ya no está abierta, se pregunta a Go: combinada en otra, abre la vigente; cobrada
  o cancelada, «Esta cuenta ya no está abierta». Con esa cuenta a la vista, el aviso va dentro de la aplicación
  (`inAppNotices`).
- **Permisos** (`ReadyAlertsOverlay`): la primera vez que el servicio corre, una explicación antes del diálogo de
  Android: notificaciones (API 33+) y la exclusión de la optimización de batería (`tools:ignore="BatteryLife"`, con su
  porqué en el manifiesto; esta, una sola vez). Mientras falten las notificaciones, se vuelven a ofrecer en cada
  entrada con conexión (`offerPermissionsAgain`; si Android ya no muestra su diálogo, «Continuar» abre sus Ajustes) y
  Mesas muestra un aviso discreto que lleva a la ayuda (`ReadyAlertsOffNotice`). Si se niegan, la ayuda «Avisos de platillos listos»
  (menú del mesero o la notificación permanente) dice qué falta, con los botones a Ajustes, y si la salida anterior
  fue «Forzar detención» o «Detener» (`ApplicationExitInfo.REASON_USER_REQUESTED`, API 30+).
- **Pruebas**: `ReadyAlertsRulesTest` (dominio), `ReadyAlertsChannelTest` (MockWebServer como Go),
  `RoomReadyAlertsLedgerTest` (Robolectric); instrumentadas `ReadyAlertsOnDeviceTest` (servicio, canales,
  notificaciones, ayuda) y el caso 5 → 6 de `AppDatabaseMigrationTest`; contra el Go temporal, `AlertsOnGoTest` con
  **`scripts/android-e2e.sh --suite alerts`** (`r1`–`r10`, `r10` antes de `r9`: en «Selecciona tu usuario» no se usa la sesión del último mesero; el guion corta la red en `r4`, mata el proceso en `r8b` y
  marca «Listo» desde la Mac). En las pruebas instrumentadas el aviso va **apagado** salvo con `readyAlerts=on`
  (`AdelTestRunner`): su servicio y sus notificaciones no deben cambiar las pruebas de la A1; sus permisos se piden
  solo con `alertPermissions=ask`.

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

Sin ellas, `assembleRelease` produce `app-release-unsigned.apk`. La llave de desarrollo del APK de prueba (nunca una
llave de publicación) la creó la Fase G de la A1 y vive fuera del repositorio, en `~/.android/adel-pos-dev/`
(`signing.properties` y su almacén). **Es la misma para la 0.1.0 y la 0.2.0**: el cambio de identificador (A2-D5) no
pide otra llave, porque Android solo exige la misma firma para actualizar el mismo paquete. Huella SHA-256 del
certificado (`apksigner verify --print-certs`, «CN=AD-EL POS desarrollo - APK de prueba, O=AD-EL POS»):
`fb6a4bac93052ca9051342ea122ed90469340a3685de14243bdab288d71c7c86`; un APK de entrega con otra huella no es el de
siempre.

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
- **Navegación principal «Mesas | Para llevar»** (§3.1 de la A2, siempre a la vista, también en la captura;
  `navigationPlacementFor` y `currentNavigationPlacement()` en `core:ui/layout/PaneLayout.kt`): **barra inferior**
  (`NavigationBar`) en vertical y **riel lateral** (`NavigationRail`, los mismos dos destinos, íconos y textos) con
  poca altura (< 480 dp, el mismo límite de los paneles) o en horizontal. En el teléfono horizontal la barra de 80 dp
  dejaba la cuadrícula de productos sin altura (hallazgo de la Fase E); en la tableta horizontal lo escaso también es
  la altura, y en la tableta vertical la barra solo quita 80 de 1280 dp de alto (el riel le quitaría ancho a los dos
  paneles). El riel va dentro del contenido, que ya respeta los bordes del sistema (sin márgenes propios). Con poca
  altura (`isHeightCompact`, el mismo límite) el catálogo de la captura usa además su **forma compacta**
  (`CatalogBrowser(compactHeight)`): la búsqueda y las categorías en una fila y tarjetas bajas (imagen pequeña a la
  izquierda, nombre y precio a la derecha); con la forma normal, en el teléfono horizontal ninguna tarjeta cabía
  completa (ni sin la barra). Pruebas: `PaneLayoutTest` (JVM), `MainNavigationLayoutTest` (las dos orientaciones en
  el dispositivo), `CatalogComponentsTest.conPocaAlturaLasTarjetasCabenConSuNombre` y `takeaway t9` (productos a la
  vista en horizontal, con capturas de pantalla).
- **De borde a borde** en todas las versiones (`enableEdgeToEdge()` y `WindowInsets.safeDrawing` en el `Scaffold`).

## Diferencias de plataforma (avance §1.6)

TLS 1.3 desde Android 10 (en 8–9, TLS 1.2 con las suites restringidas, verificado contra Go); NSD con
`registerServiceInfoCallback` desde la API 34 y `resolveService` antes, de uno en uno; `POST_NOTIFICATIONS` desde la 33;
`setExpedited` de WorkManager es servicio en primer plano en 26–30 (preferir trabajo normal); WorkManager y el
indicador **no exigen red validada** (la wifi del restaurante puede no tener internet); StrongBox opcional desde la 28;
`dataExtractionRules` desde la 31 (`fullBackupContent` antes); al pasar a `targetSdk` 37, permiso de red local
(`ACCESS_LOCAL_NETWORK`). `java.time` existe desde la 26 (sin «desugaring»).

## Base local (Room)

`AppDatabase` versión 6: `device_preferences` (v1); `paired_server` (una fila: `server_id`, nombre, última dirección,
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
`WAITER_COLUMNS_V5`). Versión 6 (etapa A2, **una sola para toda la etapa**: las fases C y D agregan sus tablas a esta
misma versión antes de la entrega): `captures` admite capturas sin mesa (`table_id` opcional; Android 8 no cambia una
columna a opcional, así que la migración 5 → 6 copia la tabla) con `mode`, `customer_name` y `customer_phone`, y la
tabla nueva `takeaway_customer_edits` (ediciones del cliente por enviar); `WAITER_CHANGES_V6`. El destino guardado de
un pedido (`pending_orders.destination`, JSON) lleva la modalidad y el cliente con valores por omisión para lo guardado
antes. Todo registrado en `deviceDataWipe` (`WaiterDao.eraseAll`) y en la bitácora (`unsentOrders`). La Fase C no
cambia el esquema: el nombre visible y la impresora de precuentas van en `restaurant_snapshot` (tipo `precheck`). Fase
D: el aviso de platillos listos agrega a la misma v6 `ready_alerts_state` (una fila: cursor, si está activo, fin del
día y configuración de Terminales) y `ready_alerts_seen` (avisos ya mostrados con su `key` y su día), vacías al migrar;
`READY_ALERTS_TABLES_V6` al final de la migración 5 → 6 y `ReadyAlertsDao.eraseAll` en `deviceDataWipe`.

Cada cambio de esquema: subir `version`, agregar la `Migration` a `AppDatabase.MIGRATIONS`, versionar el nuevo
`core/data/schemas/…/<versión>.json` y un caso en `AppDatabaseMigrationTest`. **Nunca**
`fallbackToDestructiveMigration`: la base guardará pedidos sin enviar.

`CatalogDatabase` (`catalog.db`, versión 1, Fase D) es **otra base**: la copia del catálogo de Go, que se reemplaza
completa con `reset` o época distinta sin tocar `AppDatabase`. Cada registro guarda el documento JSON de Go
(`document`) y las columnas que se consultan (orden, categoría, `search_key` normalizada); `catalog_state` (versión,
época, lista y tasa), `images` (referencias) e `image_files` (la caché de `cacheDir/catalog-images/`, con su `ETag`).
Cada descarga se guarda en una transacción. Mismas reglas de versiones, con `CatalogDatabase.MIGRATIONS` y
`CatalogDatabaseMigrationTest`.
