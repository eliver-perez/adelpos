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

## Estructura

| Módulo           | Tipo                                     | Qué contiene                                                                                                                                                                                                                                                   |
| ---------------- | ---------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `app`            | aplicación (`local.adel.pos.android`)    | `AdelPosApplication` + `AppContainer`, `MainActivity` (de borde a borde), `ShellViewModel` (indicador), navegación (`navigation/AdelPosApp.kt`, rutas con tipo), manifiesto, ícono, `src/debug` con `network_security_config`.                                 |
| `core:domain`    | Kotlin/JVM puro (sin Android)            | `Money` y `MoneyFormat`, `LinkState`/`ConnectionSummary` y `DefaultNetworkTracker`, `PendingOrdersSource`, `AppError`/`AppResult`, `SecretSealer`, `TableState`, `TablesView`. Pruebas JVM en segundos.                                                        |
| `core:data`      | biblioteca Android                       | `api/` (OkHttp con huella fijada, cabeceras del cliente, JSON, `MoneySerializer`, errores de Go → `AppError`, `ServerApi.getVersion`), `security/` (huella, Keystore), `db/` (Room, esquemas en `core/data/schemas/`), `network/`, `pending/`, `preferences/`. |
| `core:ui`        | biblioteca Android (Compose, Material 3) | Tema claro/oscuro (`theme/`), íconos propios (`icons/AdelIcons.kt`, los trazos de `desktop/src/components/Icon.tsx`), botones, barra superior e indicador, estado de mesa, andamio de uno o dos paneles (`layout/`). Compartido con la A2.                     |
| `feature:access` | biblioteca Android                       | Entrada al dispositivo: emparejamiento, búsqueda del servidor y PIN (fases B y C). En la Fase A, la pantalla de muestra «Entrar».                                                                                                                              |
| `feature:waiter` | biblioteca Android                       | Modo Mesero (Fase E). En la Fase A, «Mesas» de muestra con la búsqueda que sobrevive a la rotación y la cuenta a un toque en el teléfono.                                                                                                                      |

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
- **Contra el Go temporal** (`ServerApiOnDeviceTest`, se omite sin argumentos): `iso.sh server-up` y los argumentos de
  instrumentación `pin` (huella SPKI en hexadecimal de `run-<TAG>/identity/server-cert.pem`) y `net`
  (`https://10.0.2.2:<ISO_NET_PORT>`). **Nunca** contra el servicio real de la Mac (8181, 8443).
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

`AppDatabase` (versión 1: `device_preferences`). Cada cambio de esquema: subir `version`, agregar la `Migration` a
`AppDatabase.MIGRATIONS`, versionar el nuevo `core/data/schemas/…/<versión>.json` y un caso en
`AppDatabaseMigrationTest`. **Nunca** `fallbackToDestructiveMigration`: la base guardará pedidos sin enviar.
