# AD-EL POS Android · aplicación de cocina (`cocina/`)

Aplicación Android que funciona **solo como pantalla de cocina** (KDS), para tabletas en horizontal, conectada a la
computadora principal del restaurante (etapa K1:
[docs/etapas/ETAPA_K1_KDS_ANDROID_v3.md](../docs/etapas/ETAPA_K1_KDS_ANDROID_v3.md); decisiones K1-Dn en
[docs/ETAPA_K1_AVANCE.md](../docs/ETAPA_K1_AVANCE.md)). Es otro cliente de la API v1 ([docs/API_V1.md](../docs/API_V1.md)):
un dispositivo de tipo `kitchen`, sin usuario ni sesión, que solo ve y marca los pedidos de sus estaciones (K1-D3 y
K1-D4). Sus rutas (`kitchen-display/*`), su canal de eventos y el orden de arranque están en `docs/API_V1.md`,
«Pantalla de cocina». Cómo la instala y la empareja el dueño: [docs/COCINA_INSTALAR.md](../docs/COCINA_INSTALAR.md).

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
7. **Go decide la tarjeta** (K1-D5, K1-D25 y K1-D31): la tableta pinta lo que manda `kitchen-display/orders`, en su
   orden. Con cualquier evento vuelve a leer `orders` (una lectura en curso y como mucho una pendiente), y el error de
   esa lectura decide la pantalla (sin cupo, cupo liberado, sin activar, hora incorrecta). La espera en vivo es
   `elapsed_seconds` más el reloj monotónico de la tableta, nunca su hora.
8. **Conexión tolerante** (§5.1, K1-D33 y K1-D36): el mismo estado único de la A3 que la barra (`serverLink.state`),
   con un **margen de 10 s** cuando Android dice «sin red» (`CocinaContainer.NETWORK_LOSS_GRACE_MILLIS`; en meseros
   es 0). Dentro de la tolerancia no se ve nada y una marca sin respuesta se repite sola cada 2 s, con su palomita a la
   vista. Con «Sin conexión», el aviso grande arriba («Sin conexión con la computadora principal. Reconectando…») y nada
   se puede marcar; al volver, lectura completa. Con la computadora colgada (acepta la conexión pero no responde) el
   aviso tarda unos 41 s (K1-D37).
9. **Alertas** (§5.2, K1-D15): un sonido para pedido nuevo y otro para cancelación o cambio, generados en la aplicación
   (`alerts/Tones.kt`) y por el canal de notificaciones, con «Sonido» y «Volumen» de Terminales; nunca en la primera
   lectura y a lo más uno de cada tipo por lectura. La tarjeta nueva o cambiada late tres veces en azul.
10. **Pantalla siempre encendida** (`FLAG_KEEP_SCREEN_ON`) y arranque directo en Pedidos con lo guardado en Room.
11. **Ajustes** (§5.4, K1-D13 y K1-D32): pulsación larga en el logotipo y la contraseña de un administrador
    (`settings-access`), que vive **solo en memoria** y se borra al cerrar, al volver a emparejar, si la pantalla deja de
    estar lista y a los 2 minutos sin tocar nada. Muestran la dirección, la conexión y la versión; «Volver a emparejar»
    llama a `kitchen-display/unpair` (suelta el cupo) y, si Go no responde o ya no la reconoce, borra lo suyo de todos
    modos y dice que hay que revocarla en Terminales.

## Estructura (módulo `app`)

| Paquete              | Qué contiene                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `com.adelpos.cocina` | `CocinaApplication` + `CocinaContainer` (inyección manual: identidad `kitchen`, estado único de conexión, Keystore, NSD, `KitchenAccess`, emparejamiento con `PairingRepository` de android-core) y `MainActivity` (horizontal, pantalla completa).                                                                                                                                                                                                                                |
| `access/`            | `KitchenStatus` (qué pantalla se ve), `KitchenServerRequirements` (servidor 0.16.0 o posterior con la capacidad `kitchen-display-v1`, K1-D27) y `KitchenAccess` (el servidor emparejado, su conexión con la huella fijada, la comprobación de versiones y lo que hace cada error de Go; implementa `PairingStore`).                                                                                                                                                                |
| `board/`             | `KitchenDisplayDtos` (el contrato de `kitchen-display/*`), `Board`/`BoardMapper` (el modelo de la pantalla), `BoardRules` (reglas puras: orden, espera, color, qué se toca, pedidos fuera de la vista, qué pantalla toca a cada error), `KitchenDisplayService` (las rutas), `KitchenBoard` (el ciclo `connect` → `orders` → canal, lecturas agrupadas, acuse de comandas, marcas con su reintento, Recientes, cupo y «Sin conexión») y `BoardAlerts` (qué suena en cada lectura). |
| `alerts/`            | `Tones` (los dos tonos en PCM) y `ToneAlertPlayer` (`AudioTrack` con `USAGE_NOTIFICATION_EVENT` y el volumen de Terminales).                                                                                                                                                                                                                                                                                                                                                       |
| `settings/`          | `KitchenSettings`: la contraseña del administrador solo en memoria, `settings-access` y «Volver a emparejar» con `unpair`.                                                                                                                                                                                                                                                                                                                                                         |
| `db/`                | `KitchenDatabase` (`cocina.db`, versión 1: `paired_server`).                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `ui/`                | `CocinaApp`/`CocinaScreen` (la barra con el indicador, el logotipo de los ajustes y la pantalla de cada estado: las de la entrada son las compartidas), `OrdersScreen` (la pantalla de pedidos, el aviso grande de «Sin conexión»), `OrderCardView` (la tarjeta y su animación), `RecentDialog` y `SettingsDialog`.                                                                                                                                                                |

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

- **JVM** (56 pruebas): `KitchenAccessTest` (sin red, con un almacén en memoria: sin servidor, emparejada, credencial
  que ya no se abre, revocada, cada error de Go, emparejar de nuevo y los requisitos del servidor), `BoardRulesTest`,
  `KitchenBoardTest` (el ciclo, la tolerancia con el `ServerLinkMonitor` real y un reloj simulado, el reintento, «Sin
  conexión», los sonidos y lo recuperado), `BoardAlertsTest`, `TonesTest` y `KitchenSettingsTest`.
- **Instrumentadas** (`connectedDebugAndroidTest`, en `Pixel_Tablet` y `ADEL_A1_Tablet_API26`, en horizontal; 15
  corridas, las de Go se omiten): `KitchenScreensTest` (la pantalla vacía, el cupo y la licencia, y las pantallas
  compartidas de la entrada), `KitchenAppStartTest` (la aplicación real sin emparejar abre en horizontal, a pantalla
  completa, siempre encendida y en «Buscar el servidor»), `OrdersScreenTest` (los casos de la tarjeta, los colores
  con el reloj simulado, marcar, la hora incorrecta, Recientes, muchos pedidos y las dos letras por los dos temas) y
  `ConnectionAndSettingsScreensTest` («Sin conexión», la animación, la pulsación larga, la contraseña y «Volver a
  emparejar»); capturas en `files/screens`. Antes, desinstalar `com.adelpos.cocina` y `com.adelpos.cocina.test` y
  contar los XML.
- **Contra el Go temporal**: `KitchenOnGoTest` con **`scripts/android-e2e.sh --suite kitchen`**: `k1` emparejar como
  pantalla de cocina, `k2` conectar (el cupo) y la pantalla vacía, `k3` pedidos reales que aparecen solos, `k4` marcar
  platillo, el cierre automático y el pedido con el aviso al mesero (`ready_notices` en la base temporal, con
  `ADELPOS_E2E_DATABASE_URL`), `k5` Recientes y Recuperar, `k6` letra y tema desde Terminales al momento, `k7` cortes
  de 1 a 5 s del Go (`kill -STOP`/`-CONT` solo a su PID, comprobado con `lsof`) sin ningún aviso, `k8` una marca durante
  un corte de 18 s que se repite y Go registra una vez, `k9` un corte de 45 s con el aviso, sin marcar y que se recupera
  solo, `k10` cortes de red de 1, 3, 5 y 35 s (modo avión en Android 11+; wifi y datos con root en Android 8) con el
  aviso solo pasado el margen de 10 s (`NetworkEpisodes` anota lo que ve Android), `k11` los ajustes con la contraseña
  y `k13` «Volver a emparejar» suelta el cupo. **`--suite endtoend`** (§8): las dos aplicaciones en la misma tableta;
  Ana envía a cocina desde la de meseros (`AlertsOnGoTest` `e1`), esta la marca lista (`k12`) y a Ana le llega el aviso.
  Necesitan un servidor 0.16.0 con la capacidad `kitchen-display-v1`. Nunca contra el servicio real de la Mac.
- **Matriz**: los dos emuladores de la etapa, en horizontal; los cuatro solo si el propietario lo pide.

## Dependencias

Las del catálogo único de `android-core` (`android-core/gradle/libs.versions.toml`); bloqueos propios de este build y
su `gradle/verification-metadata.xml`, que verifica también lo que resuelve `android-core`. Cómo regenerarlos:
[android-core/CLAUDE.md](../android-core/CLAUDE.md), «Dependencias, bloqueos y verificación».
