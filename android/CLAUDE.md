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
14. **El trabajo pendiente de un mesero manda** (§0-B, §3.2 y §7; `WaiterWork` en `core:domain`): el bloqueo por
    inactividad no ocurre con una captura en curso ni con pedidos sin enviar (`blocksIdleLock`); cambiar de mesero lo
    impiden solo los pedidos sin enviar del mesero actual («Por enviar», «Enviando», «Requiere tu revisión»;
    `blocksSwitch`): una captura en curso se queda guardada con su mesero, su sesión no se cierra y el siguiente no la
    ve; un «Cerrar sesión» explícito lo impiden ambas cosas (`blocksLogout`). Quien muestre esas acciones pregunta a
    `WaiterWorkGate` antes de hacerlas.
15. **Ninguna prueba abre la aplicación normal con la búsqueda automática en la red real**: las pruebas
    instrumentadas de `app` corren con `AdelTestRunner`, que busca el tipo NSD `_adelpostest._tcp` (el emulador de
    Android 17 sí ve los anuncios de la red de la Mac). Para capturas o revisiones a mano, usar el mismo ejecutor o una
    prueba; si alguna vez aparece el servidor real del restaurante, no se toca.

## Estructura

| Módulo           | Tipo                                     | Qué contiene                                                                                                                                                                                                                                                                                                                                                                                                                        |
| ---------------- | ---------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `app`            | aplicación (`local.adel.pos.android`)    | `AdelPosApplication` + `AppContainer`, `MainActivity` (de borde a borde), `ShellViewModel` (indicador), navegación (`navigation/AdelPosApp.kt`, rutas con tipo), manifiesto, ícono, `src/debug` con `network_security_config`.                                                                                                                                                                                                      |
| `core:domain`    | Kotlin/JVM puro (sin Android)            | `Money` y `MoneyFormat`, `LinkState`/`ConnectionSummary` y `DefaultNetworkTracker`, `PendingOrdersSource`, `AppError`/`AppResult`, `SecretSealer`, `TableState`, `TablesView`; `server/` (dirección escrita, anuncio NSD, cambio de IP, versiones), `pairing/` (código de 6 dígitos, `PairingFlow`), `access/` (`DeviceAccess`, verificador del PIN, límite local, bloqueo por inactividad, `WaiterWork`). Pruebas JVM en segundos. |
| `core:data`      | biblioteca Android                       | `api/` (OkHttp con huella fijada, cabeceras del cliente, JSON, `MoneySerializer`, errores de Go → `AppError`, `ServerApi` con `ApiOutcome`), `access/` (`AccessRepository`, `PairingRepository`, `PairedConnection`), `discovery/` (NSD, sonda HTTPS), `security/` (huella fijada, primer certificado del emparejamiento, Keystore), `db/` (Room, esquemas en `core/data/schemas/`), `network/`, `pending/`, `preferences/`.        |
| `core:ui`        | biblioteca Android (Compose, Material 3) | Tema claro/oscuro (`theme/`), íconos propios (`icons/AdelIcons.kt`, los trazos de `desktop/src/components/Icon.tsx`), botones, barra superior e indicador, estado de mesa, andamio de uno o dos paneles (`layout/`). Compartido con la A2.                                                                                                                                                                                          |
| `feature:access` | biblioteca Android                       | Entrada al dispositivo (fases B y C): «Buscar el servidor» y código de emparejamiento, entrada con PIN, cambio de mesero, bloqueo por inactividad (`AccessViewModel`), emparejar de nuevo, actualizar la aplicación o el servidor, dispositivo retirado y su bitácora, permiso de red local. Solo usa interfaces de `core:domain`.                                                                                                  |
| `feature:waiter` | biblioteca Android                       | Modo Mesero (Fase E). En la Fase A, «Mesas» de muestra con la búsqueda que sobrevive a la rotación y la cuenta a un toque en el teléfono.                                                                                                                                                                                                                                                                                           |

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
- **Instrumentadas** (`connectedDebugAndroidTest`): `MigrationTestHelper`, Keystore, Compose (tema, indicador,
  estados de mesa, botones, paneles) y la aplicación completa (navegación, uno o dos paneles según el dispositivo,
  rotación real con `UiAutomation.setRotation` sin perder lo capturado, indicador con el modo avión real).
- **Contra el Go temporal**: `ServerApiOnDeviceTest` (`core:data`, argumentos `pin` y `net`) y, de punta a punta,
  `AccessOnGoTest` (`app`, 7 pasos: emparejar con el código, PIN incorrecto y bloqueo al quinto, entrada sin conexión,
  PIN cambiado, 426, certificado cambiado y dispositivo retirado). Ambas se omiten sin argumentos (en
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
- `requestHeaders(): Headers?` — `X-Device-ID`/`X-Device-Token` y, si hay mesero activo **con sesión de Go**,
  `Authorization: Bearer`. `deviceCredentials()` da solo las del dispositivo; `sessionToken(userId)`, la sesión de un
  mesero (solo en memoria: se pierde al cerrar la aplicación); `offlineCredential(userId)`, su credencial de envío
  (A1-D1, cifrada con el Keystore) para `submitOfflineOrder` sin sesión. `activeWaiter` (`StateFlow`) dice quién usa
  el dispositivo y si tiene sesión (`hasServerSession`; sin conexión no la tiene).
- `report(error: AppError, userId: String?)` — **punto único** para los errores de Go de cualquier petición o del
  canal de eventos (`event: interrupted`): `DEVICE_REVOKED` retira el dispositivo (bitácora y borrado), `SESSION_*`
  regresa a ese mesero a la entrada, `DEVICE_*` y una huella distinta piden emparejar de nuevo, 426 pide actualizar.
- La Fase E implementa en `AppContainer` **`WaiterWorkGate`** (captura en curso y pedidos sin enviar de cada mesero:
  decide el bloqueo por inactividad, el cambio de mesero y el «Cerrar sesión»; ver la regla «El trabajo pendiente de
  un mesero manda») y **`UnsentOrdersSource`** (pedidos que no llegaron al servidor: al retirar el dispositivo o
  emparejarlo de nuevo pasan a la bitácora y se muestran con «Este dispositivo fue retirado del restaurante; estos
  pedidos no se enviaron»). Hoy son `NoWaiterWork` y `NoUnsentOrders`.

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

`AppDatabase` versión 2: `device_preferences` (v1); `paired_server` (una fila: `server_id`, nombre, última dirección,
huella fijada, `device_id`, **token del dispositivo cifrado**, `idle_lock_seconds`), `known_waiters` (meseros que ya
entraron con conexión aquí: nombre, inicial, color, `pin_version`, **verificador del PIN y credencial de envío
cifrados**, intentos locales sin conexión) y `device_log` (bitácora: pedidos que no se enviaron, hasta que se borra).
Las llaves solo en el Android Keystore; lo cifrado lleva su propósito (`device-credential`, `pin-verifier:<id>`,
`offline-credential:<id>`). Migración 1 → 2 con el SQL de `schemas/…/2.json`.

Cada cambio de esquema: subir `version`, agregar la `Migration` a `AppDatabase.MIGRATIONS`, versionar el nuevo
`core/data/schemas/…/<versión>.json` y un caso en `AppDatabaseMigrationTest`. **Nunca**
`fallbackToDestructiveMigration`: la base guardará pedidos sin enviar.
