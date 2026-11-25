# AD-EL POS Android · aplicación de cocina (`cocina/`)

Aplicación Android que funciona **solo como pantalla de cocina** (KDS), para tabletas en horizontal, conectada a la
computadora principal del restaurante (etapa K1:
[docs/etapas/ETAPA_K1_KDS_ANDROID_v3.md](../docs/etapas/ETAPA_K1_KDS_ANDROID_v3.md); decisiones K1-Dn en
[docs/ETAPA_K1_AVANCE.md](../docs/ETAPA_K1_AVANCE.md)). Es otro cliente de la API v1 ([docs/API_V1.md](../docs/API_V1.md)):
un dispositivo de tipo `kitchen`, sin usuario ni sesión, que solo ve y marca los pedidos de sus estaciones (K1-D3 y
K1-D4). Sus rutas (`kitchen-display/*`), su canal de eventos y el orden de arranque los documenta la Fase A en
`docs/API_V1.md`.

Proyecto Gradle propio en la raíz del repositorio, **sin nada de la aplicación de meseros**. Lo que comparte con ella
(emparejamiento, TLS con la huella fijada, API v1, canal de eventos, estado de conexión tolerante, tema y pantallas de
la entrada) viene de [android-core/](../android-core/CLAUDE.md), incluido con `includeBuild`. Las reglas permanentes
de [android/CLAUDE.md](../android/CLAUDE.md) valen aquí igual (textos en español sin tecnicismos ni textos
provisionales, colores solo en la paleta de `android-core`, código claro con ktfmt, `minSdk` 26, credenciales solo
cifradas con el Keystore, sin copias de seguridad, inyección manual, un solo estado de conexión, Go decide todo).

## Identificador y nombres

| Pieza                         | Valor                                                                                            |
| ----------------------------- | ------------------------------------------------------------------------------------------------ |
| `applicationId` y `namespace` | `com.adelpos.cocina` (paquetes de Kotlin `com.adelpos.cocina…`)                                  |
| Nombre visible                | «AD-EL POS Cocina», con su propio ícono (fondo oscuro y naranja; el de meseros es blanco y azul) |
| Tipo de cliente               | `X-Client-Kind: kitchen` y `device_kind: kitchen` al emparejar (`ClientKinds.KITCHEN`, fijo)     |
| Versión                       | Un solo lugar: `cocina/gradle.properties` (`adelpos.versionName` 0.1.0, `adelpos.versionCode` 1) |
| SDK                           | `minSdk` 26, `targetSdk` 36, `compileSdk` 37 (los de meseros, A1-D7)                             |
| Base local                    | `cocina.db` (`KitchenDatabase`, Room, esquemas en `app/schemas/`): solo el servidor emparejado   |
| Pruebas instrumentadas        | Paquete `com.adelpos.cocina.test` con el ejecutor `com.adelpos.cocina.CocinaTestRunner`          |

Las dos aplicaciones se pueden instalar en la misma tableta: son paquetes distintos, con su propia base, sus propias
llaves del Keystore y su propio emparejamiento.

## Reglas de la aplicación de cocina

1. **Solo pantalla de cocina**: nada de PIN, mesas, captura, cobros ni administración. Go rechaza cualquier otra ruta
   con `403 KITCHEN_DISPLAY_ONLY`; la aplicación nunca las llama.
2. **Horizontal y pantalla completa**: la actividad es `sensorLandscape` (con
   `PROPERTY_COMPAT_ALLOW_RESTRICTED_RESIZABILITY`, porque Android 16 ignora la orientación fija en pantallas grandes
   con `targetSdk` 36) y esconde las barras del sistema (vuelven un momento al deslizar desde el borde).
3. **Tema oscuro por omisión** (§4): el claro se elige por pantalla en Terminales.
4. **Revocada en Terminales** (`DEVICE_REVOKED`): borra la credencial y todo lo de este dispositivo y vuelve a «Buscar
   el servidor» (§2), sin pasar por la pantalla de «dispositivo retirado» de meseros.
5. **Lo compartido vive en `android-core`**: si algo de aquí también sirve a la de meseros (o al revés), se mueve allá;
   no se copia.
6. **Ninguna prueba abre la aplicación normal con la búsqueda automática en la red real**: `CocinaTestRunner` busca el
   tipo NSD de pruebas `_adelpostest._tcp` (`ServerAnnouncements.TEST_SERVICE_TYPE`).

## Estructura (módulo `app`)

| Paquete              | Qué contiene                                                                                                                                                                                                                                                             |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `com.adelpos.cocina` | `CocinaApplication` + `CocinaContainer` (inyección manual: identidad `kitchen`, estado único de conexión, Keystore, NSD, `KitchenAccess`, emparejamiento con `PairingRepository` de android-core) y `MainActivity` (horizontal, pantalla completa).                      |
| `access/`            | `KitchenStatus` (qué pantalla se ve), `KitchenServerRequirements` (capacidad `kitchen-display-v1`) y `KitchenAccess` (el servidor emparejado, su conexión con la huella fijada, la comprobación de versiones y lo que hace cada error de Go; implementa `PairingStore`). |
| `db/`                | `KitchenDatabase` (`cocina.db`, versión 1: `paired_server`).                                                                                                                                                                                                             |
| `ui/`                | `CocinaApp`/`CocinaScreen` (la barra con el indicador y la pantalla de cada estado: las de la entrada son las compartidas) y `OrdersScreen` (la pantalla de pedidos; sin pedidos, «No hay pedidos»).                                                                     |

## Comandos

Desde la raíz (`./scripts/dev.sh android-format`, `android-format-check`, `android-build`, `android-test` y
`android-connected` recorren `android-core`, `android` y `cocina`) o en la carpeta:

| Qué                     | Cómo                                                                            |
| ----------------------- | ------------------------------------------------------------------------------- |
| APK                     | `cd cocina && ./gradlew assembleDebug assembleRelease`                          |
| JVM, Robolectric y Lint | `cd cocina && ./gradlew testDebugUnitTest lintDebug`                            |
| Instrumentadas          | `cd cocina && ANDROID_SERIAL=emulator-5580 ./gradlew connectedDebugAndroidTest` |
| Contra el Go temporal   | `ANDROID_SERIAL=… scripts/android-e2e.sh --suite kitchen` (ver «Pruebas»)       |

Los agentes usan el arnés: `ISO_GRADLE_DIR=cocina $SP/iso.sh <árbol> gradle <tareas…>`.

## Firma

La **misma llave de desarrollo** de la aplicación de meseros (§1 de la etapa K1), fuera del repositorio
(`~/.android/adel-pos-dev/`), con la misma regla de `android/app/build.gradle.kts`: la propiedad
`adelpos.signingProperties` o la variable `ADELPOS_SIGNING_PROPERTIES` con un archivo de propiedades que debe vivir
fuera del repositorio, igual que su almacén; sin ellas, `assembleRelease` produce el APK sin firmar. Huella esperada del
certificado: la de [android/CLAUDE.md](../android/CLAUDE.md), «Firma». R8 reduce sin ofuscar (`-dontobfuscate`).

## Pruebas

- **JVM**: `KitchenAccessTest` (sin red, con un almacén en memoria: sin servidor, emparejada, credencial que ya no se
  abre, revocada, cada error de Go, emparejar de nuevo y los requisitos del servidor).
- **Instrumentadas** (`connectedDebugAndroidTest`, en `Pixel_Tablet` y `ADEL_A1_Tablet_API26`, en horizontal):
  `KitchenScreensTest` (la pantalla de pedidos vacía con la barra y su indicador, y las pantallas compartidas de la
  entrada, con capturas en `files/screens`) y `KitchenAppStartTest` (la aplicación real sin emparejar abre en
  horizontal, a pantalla completa y en «Buscar el servidor»). Antes, desinstalar `com.adelpos.cocina` y
  `com.adelpos.cocina.test` y contar los XML.
- **Contra el Go temporal**: `KitchenOnGoTest` con **`scripts/android-e2e.sh --suite kitchen`** (pasos `k1` emparejar
  como pantalla de cocina y `k2` conectar con la pantalla de pedidos vacía; las fases C y D agregan los suyos). Necesita
  un servidor con la capacidad `kitchen-display-v1`. Nunca contra el servicio real de la Mac.
- **Matriz**: los dos emuladores de la etapa, en horizontal; los cuatro solo si el propietario lo pide.

## Dependencias

Las del catálogo único de `android-core` (`android-core/gradle/libs.versions.toml`); bloqueos propios de este build y
su `gradle/verification-metadata.xml`, que verifica también lo que resuelve `android-core`. Cómo regenerarlos:
[android-core/CLAUDE.md](../android-core/CLAUDE.md), «Dependencias, bloqueos y verificación».
