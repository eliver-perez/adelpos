# AD-EL POS Android · código compartido (`android-core/`)

Lo que comparten la aplicación de meseros ([android/](../android/CLAUDE.md)) y la de cocina
([cocina/](../cocina/CLAUDE.md)): emparejamiento, TLS con certificado fijado, cliente de la API v1, canal de eventos,
el estado de conexión tolerante de la A3, el tema y las pantallas de la entrada al dispositivo. Nació en la etapa K1
(§1 de [docs/etapas/ETAPA_K1_KDS_ANDROID_v3.md](../docs/etapas/ETAPA_K1_KDS_ANDROID_v3.md), decisión K1-D2 de
[docs/ETAPA_K1_AVANCE.md](../docs/ETAPA_K1_AVANCE.md)): se **movió** desde `android/` con `git mv`, sin cambiar su
comportamiento. Las reglas permanentes de [android/CLAUDE.md](../android/CLAUDE.md) (textos, colores, código claro,
`minSdk` 26, seguridad, conexión tolerante) valen aquí igual; este archivo dice solo lo propio de la carpeta.

## Cómo se comparte

- **Build de Gradle propio** (`settings.gradle.kts`, wrapper, bloqueos y verificación). Se compila y se prueba solo y,
  además, `android/` y `cocina/` lo incluyen con `includeBuild("../android-core")` (_composite build_).
- **Coordenadas**: `group = "com.adelpos.core"`. Las aplicaciones piden `implementation("com.adelpos.core:<módulo>")`
  (sin versión) y Gradle lo sustituye por el proyecto de este build. Nunca se publica.
- **Un solo catálogo de versiones**: [gradle/libs.versions.toml](gradle/libs.versions.toml). Este build lo toma solo;
  `android/` y `cocina/` lo importan con
  `versionCatalogs { create("libs") { from(files("../android-core/gradle/libs.versions.toml")) } }`. Ningún proyecto
  tiene otro catálogo: las versiones no pueden divergir.
- **Nada de las aplicaciones aquí**: ningún archivo de `android-core` importa `com.adelpos.meseros…` ni
  `com.adelpos.cocina…`. Lo que cambia por aplicación entra como parámetro: el tipo de cliente
  (`ClientIdentity(version, kind)`, `ClientKinds.ANDROID` o `KITCHEN`), lo que exige al servidor
  (`ServerRequirements`), dónde guarda el emparejamiento (`PairingStore`) y el nombre que se propone al emparejar.
- **No quedan dos copias del mismo código**: si una aplicación necesita algo que ya existe en la otra y sirve a las
  dos, se mueve aquí (con `git mv` cuando se pueda), no se copia.

## Módulos

| Módulo    | Tipo                              | Paquete                    | Qué contiene                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| --------- | --------------------------------- | -------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `model`   | Kotlin/JVM puro                   | `com.adelpos.core.model`   | `connection/` (el estado único de conexión: `ServerLink`, `ServerAnswers`, `ServerLinkTracker`, `LinkState`, `ConnectionSummary`, `DefaultNetworkTracker`); `pairing/` (código de 6 dígitos, `PairingFlow`); `server/` (dirección escrita, anuncio NSD y sus tipos, cambio de IP, `SemanticVersion`, `ServerCompatibility` con `ServerRequirements`); `realtime/`; `error/` (`AppError`, `LicenseRefusal`, `LicenseMessages`); `license/`; `security/SecretSealer`; `text/` (`Hex`, `GoTime`); `access/` (`RepairReason`, `NetworkPermissions`, `UnsentOrder`).                                                                                                                   |
| `network` | biblioteca Android, sin Room      | `com.adelpos.core.network` | `api/` (`PinnedHttpClient` con el punto único `ServerAnswersInterceptor`, `ClientIdentity` y `ClientKinds`, `ServerApi`/`ApiOutcome`, `ApiErrors`, `CallAwait`, `ApiJson`, DTO del emparejamiento); `security/` (huella fijada, primer certificado, `KeystoreSecretSealer`); `discovery/` (NSD y sonda HTTPS); `pairing/` (`PairingRepository`, `PairingStore`/`PairedDevice`, `PairedConnection`); `realtime/` (`RealtimeClient`); `session/` (`ServerAccess`, `AuthorizedServer`); `connection/` (`ServerLinkMonitor`, `ConnectionMonitor`, `AppForeground`); `wipe/DeviceDataWipe`; `license/LicenseStatusRepository`. Su manifiesto pide `INTERNET` y `ACCESS_NETWORK_STATE`. |
| `ui`      | biblioteca Android (Compose)      | `com.adelpos.core.ui`      | `theme/` (`AdelTheme`, **la paleta única** `AdelPalette.kt`, tipografía, medidas); `icons/AdelIcons`; `components/` (botones, `AdelTopBar` con `ConnectionIndicator`, toques de los diálogos); `access/` (textos de la entrada) con `findserver/` («Buscar el servidor» y el código) y `status/` (cargando, emparejar de nuevo, actualizar la aplicación o el servidor, dispositivo retirado, permiso de red local); el logotipo y los textos de todo eso.                                                                                                                                                                                                                        |
| `testing` | biblioteca Android (solo pruebas) | `com.adelpos.core.testing` | Los ayudantes de las pruebas instrumentadas contra el Go temporal, movidos de las de meseros: `MainComputer` (la computadora principal por la escucha local: aprobar el código, crear meseros, retirar dispositivos), `GoTestServer` (argumentos del guion y llamadas a Go) y `SystemDialogs`. Lo usan las dos aplicaciones con `androidTestImplementation("com.adelpos.core:testing")`; nunca entra en un APK de entrega.                                                                                                                                                                                                                                                        |

Dependencias: `ui` → `model`; `network` → `model`. Las aplicaciones: meseros `core:domain` → `model` (`api`),
`core:data` → `network` (`api`), `core:ui` → `ui` (`api`); cocina `app` → los tres.

## Lo que se guarda en el dispositivo nunca se deriva del paquete (K1-D2)

La aplicación de meseros 0.4.1 se instala encima de la 0.4.0 sin emparejar de nuevo, aunque estas clases cambiaron de
paquete. Por eso son **valores fijos**, escritos como texto, y no se cambian:

- el alias del Keystore `adel-pos-secrets-v1` y el formato del sobre (`KeystoreSecretSealer`);
- los propósitos del cifrado: `device-credential` (`PairedDevice.CREDENTIAL_PURPOSE`, el mismo de las dos
  aplicaciones) y, en meseros, `pin-verifier:<id>` y `offline-credential:<id>`;
- los tipos NSD `_adelpos._tcp` y `_adelpostest._tcp` (`ServerAnnouncements.SERVICE_TYPE` y `TEST_SERVICE_TYPE`);
- el tipo de cliente de cada aplicación (`ClientKinds`);
- de las aplicaciones (no viven aquí): los archivos y las clases de Room (`adel-pos.db`, `catalog.db` y su
  `AppDatabase`/`CatalogDatabase` en meseros; `cocina.db` en cocina), el trabajador de la cola de meseros y sus nombres
  únicos, los componentes del manifiesto y los canales de notificación.

Lo demás (clases de OkHttp, del canal o del estado de conexión) no se guarda y puede cambiar de paquete.

## Comandos

Desde la raíz del repositorio (`./scripts/dev.sh android-*` recorre los tres proyectos; ver
[android/CLAUDE.md](../android/CLAUDE.md), «Comandos»):

| Qué                     | Cómo                                                                                  |
| ----------------------- | ------------------------------------------------------------------------------------- |
| Formato                 | `cd android-core && ./gradlew ktfmtFormat` (o `ktfmtCheck`)                           |
| JVM, Robolectric y Lint | `cd android-core && ./gradlew :model:test testDebugUnitTest lintDebug`                |
| Instrumentadas          | `cd android-core && ANDROID_SERIAL=emulator-5580 ./gradlew connectedDebugAndroidTest` |

Cada build corre sus propias tareas en su carpeta: las pruebas y el Lint de `android-core` no corren desde
`android/` ni desde `cocina/`. Los agentes usan el arnés: `ISO_GRADLE_DIR=android-core $SP/iso.sh <árbol> gradle
<tareas…>`.

## Pruebas

- **JVM** (`model` y `network`): las que se movieron con su código: `ServerLinkTrackerTest`,
  `DefaultNetworkTrackerTest`, `LicenseMessagesTest`, `PairingProtocolTest`, `RealtimeTest`, `ServerAddressTest`,
  `ServerAnnouncementsTest`; `PinnedHttpClientTest`, `CallAwaitTest`, `ServerApiTest`, `AccessErrorCodesTest`,
  `LicenseErrorCodesTest` (MockWebServer con TLS y HTTP/1.1, como Go).
- **`ui`** (JVM): `PaletteContrastTest`, `ColorsOnlyInPaletteTest` y `PlainLanguageTest`. Las dos últimas recorren
  **los tres proyectos** (`android/`, `android-core/` y `cocina/`, propiedad `adelpos.repositoryRoot`): los colores
  solo en `AdelPalette.kt` y ningún texto provisional o de desarrollo, en las dos aplicaciones.
- **Instrumentadas**: `network` (`KeystoreSecretSealerTest`, `NsdServerBrowserTest`, con `testOptions.targetSdk = 36`)
  y `ui` (`AdelThemeTest`, `ConnectionIndicatorTest`, `TouchTargetsTest`). Paquetes de prueba
  `com.adelpos.core.network.test` y `com.adelpos.core.ui.test`: desinstalarlos antes y contar los XML, como en meseros.
- Las pruebas que usan piezas de meseros (por ejemplo `PairingRepositoryTest` con `AccessRepository`, o
  `ServerCompatibilityTest` con los requisitos de meseros) se quedaron en `android/` y siguen probando el código de
  aquí a través de la aplicación.

## Dependencias, bloqueos y verificación

- **Bloqueos por build**: cada proyecto de este build tiene su `gradle.lockfile`, más `buildscript-gradle.lockfile` y
  `settings-gradle.lockfile`. El bloqueo de cada **aplicación** incluye además lo que le llega de aquí.
- **Verificación por build raíz**: en un build compuesto manda el `gradle/verification-metadata.xml` del build raíz
  (`android/` o `cocina/`), que verifica también lo que resuelve `android-core`; el de esta carpeta solo se usa al
  compilar `android-core` solo. Por eso hay tres archivos y cada uno debe estar completo para lo que resuelve.
- **Agregar o cambiar una dependencia** (en el catálogo o en un módulo de aquí): regenerar los tres builds, en este
  orden, con un emulador encendido (las instrumentadas resuelven artefactos propios), y revisar el diff:

  ```sh
  cd android-core && ANDROID_SERIAL=emulator-5580 ./gradlew resolveAllDependencies assembleDebug \
    assembleDebugAndroidTest :model:test testDebugUnitTest lintDebug ktfmtCheck connectedDebugAndroidTest \
    --write-locks --write-verification-metadata sha256
  cd ../android && ANDROID_SERIAL=emulator-5580 ./gradlew resolveAllDependencies assembleDebug assembleRelease \
    assembleDebugAndroidTest :core:domain:test testDebugUnitTest lintDebug ktfmtCheck connectedDebugAndroidTest \
    --write-locks --write-verification-metadata sha256
  cd ../cocina && ANDROID_SERIAL=emulator-5580 ./gradlew resolveAllDependencies assembleDebug assembleRelease \
    assembleDebugAndroidTest testDebugUnitTest lintDebug ktfmtCheck connectedDebugAndroidTest \
    --write-locks --write-verification-metadata sha256
  ```

  Una biblioteca nueva se anota en [docs/DEPENDENCIES.md](../docs/DEPENDENCIES.md) y se comprueba su `minSdk` real
  contra 26.

- Gradle 9.9.1 con el mismo wrapper versionado en los tres proyectos.
