# Etapa A1 (Android) · Avance

Primera etapa de la aplicación Android de meseros: proyecto base, emparejamiento, entrada con PIN, catálogo en el
dispositivo y modo Mesero con captura sin conexión. Especificación:
[docs/etapas/ETAPA_A1_ANDROID.md](etapas/ETAPA_A1_ANDROID.md). Rama `etapa-a1-android`, creada desde `main` (0.11.0,
esquema 24, con la 9H fusionada; el servicio de la Mac ya está en 0.11.0). Coordinador y como máximo dos agentes a la
vez, cada fase en su árbol de trabajo, probada en copias aisladas (clúster temporal 55499, Go temporal en modo de
pruebas con su escucha de red en loopback y emuladores sin ventana en modo de solo lectura). **Ninguna prueba usa el
servicio real de la Mac** (8181, 8443, 55432 ni su carpeta).

**Dispositivos**: emuladores `Pixel_8` (teléfono) y `Pixel_Tablet` (tableta) con Android 17 (API 37, arm64) del
propietario, abiertos siempre en modo de solo lectura (no se modifican). Sin dispositivo físico por ahora.

**Dónde vive la aplicación**: carpeta `android/` de este repositorio, por la especificación (§0, «Repositorio y
estructura»). `CLAUDE.md` §16 decía que Android sería un repositorio independiente; como en la 9H con el lector del
sistema anterior, prevalece la especificación de la etapa y §16 se actualiza en la Fase G.

---

## Fase 0 · Verificación

Rama `etapa-a1-0` (desde `etapa-a1-android` = `main` + la especificación), un solo agente, 16 de noviembre de 2026.
**Sin cambios en el repositorio** (árbol limpio; `docs/etapas/ETAPA_A1_ANDROID.md` con el mismo md5). El proyecto de
prueba vive fuera del repositorio, en `$SP/spike-android/` (desechable). Todas las pruebas usaron el Go temporal del
arnés (`ISO_TAG=P0`, HTTP local 8191 y HTTPS de red 8451, base desechable en el clúster 55499), **nunca** el servicio
real. Ruta de `$SP`: el scratchpad de la sesión del coordinador.

**Resultado: no hay parada.** El entorno compila, prueba en JVM y prueba en emuladores sin ventana contra el Go
temporal en Android 8 (API 26), 13 (API 33) y 17 (API 37). Hallazgos que cambian el plan (detalle en cada sección y
en «Decisiones para el coordinador»):

1. **La versión 0.1.0 de la app recibe 426.** El servidor compara `X-Client-Version` con un único
   `min_client_version` (0.10.0) para todos los tipos de cliente: la app Android 0.1.0 queda rechazada (comprobado:
   `426 CLIENT_UPDATE_REQUIRED` con `client_version: 0.1.0`). Hace falta una versión mínima **por tipo de cliente**
   (Fase B).
2. **El rol Mesero nace sin permisos** en una instalación nueva (solo el administrador recibe todos); no existe un
   permiso «usar dispositivos de meseros» ni ruta alguna de PIN. El servidor **no restringe mesas por mesero**.
3. **Sesiones de 12 h frente a captura sin conexión «sin límite de tiempo»**: la cola necesita una sesión válida del
   mesero que capturó para enviar con WorkManager con la app cerrada. Hay que decidir el mecanismo (Fase B/E).
4. **Los cambios de preparación de cocina no llegan al mesero en tiempo real**: solo se publican en el ámbito
   `kitchen` con estación; un mesero sin permisos de cocina no los recibe (Fase E).
5. **Android 17 (API 37) exige un permiso de red local** (`ACCESS_LOCAL_NETWORK`) a las apps con `targetSdk` 37; con
   `targetSdk` 36 (lo que exige hoy Google Play) no aplica.

### 1. Entorno Android

#### 1.1 Inventario final

| Componente                                   | Estado                                                                                                                                                                         | Nota                                                                                                                                                                                                                                                  |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Android Studio                               | 2026.2 (`AI-262…`), JBR **25.0.4**                                                                                                                                             | El JBR es el JDK de toda la etapa (`iso.sh gradle`). Gradle 9.9.1 corre sobre él sin avisos propios.                                                                                                                                                  |
| SDK `~/Library/Android/sdk`                  | plataformas `android-31`, `android-33`, `android-37.0`; build-tools 28.0.3, 30.0.2, 33.0.0, **36.0.0**                                                                         | AGP 9.5.1 usa build-tools 36.0.0 (su mínimo y predeterminado) y `android-37.0`: **AGP no descargó nada** al SDK.                                                                                                                                      |
| `platform-tools`                             | **37.0.1** (adb 1.0.41, 37.0.1-15733141)                                                                                                                                       | **Lo actualizó el propietario durante la fase** (antes 33.0.3). Reinicié el servidor adb (`adb kill-server`) para usar el nuevo.                                                                                                                      |
| `cmdline-tools`                              | **latest 23.0** (`sdkmanager`, `avdmanager`)                                                                                                                                   | **Las instaló el propietario durante la fase.** Necesitan `JAVA_HOME` del JBR. `sdkmanager --version` avisa que está en desuso en favor de la «Android CLI» (`android sdk`, en la misma carpeta). Queda también el antiguo `tools/` 26.1.1 (sin uso). |
| Emulador                                     | 37.2.13, con **netsim** WiFi                                                                                                                                                   |                                                                                                                                                                                                                                                       |
| Imágenes                                     | `android-37.0` `google_apis` y `google_apis_playstore` (arm64), `android-33` `google_apis`, **`android-26` `google_apis`** (apareció durante la fase, con los AVD de la etapa) | `sdkmanager` ofrece actualizar `android-33` (2.0.0 → 3.0.0) y su imagen (6 → 17): no hace falta.                                                                                                                                                      |
| AVD del propietario                          | `Pixel_8` (1080×2400, 420 dpi), `Pixel_Tablet` (2560×1600, 320 dpi), ambos API 37 Play; `8_Fold-out` (API 37 `google_apis`); `Pixel_4_XL_API_33`                               | Abiertos solo con `iso.sh emu-up` (`-read-only`). Las imágenes Play **no permiten `adb root`**.                                                                                                                                                       |
| AVD de la etapa (creados por el coordinador) | `ADEL_A1_Tablet_API26` (Android 8.0, 1280×800 a 160 dpi, consola 5590) y `ADEL_A1_Phone_API33` (Android 13, pixel_6a, consola 5592)                                            | `google_apis` (sin Play): permiten `adb root`. Arrancan en frío en ≈ 10 s.                                                                                                                                                                            |
| Gradle                                       | caché `~/.gradle` con 5.4.1, 6.7.1, 9.6.0 y ahora **9.9.1** (descargada por el wrapper del proyecto de prueba)                                                                 | `~/.gradle` pasó de 2.4 a 4.0 GiB (distribución 9.9.1 y dependencias de la pila elegida).                                                                                                                                                             |
| Robolectric                                  | `~/.m2/repository/org/robolectric/android-all-instrumented/16-robolectric-13921718-2-i7` (**224 MB**)                                                                          | Lo descarga Robolectric la primera vez (SDK 36). Cada SDK adicional en Robolectric cuesta otro tanto: limitar los SDK de las pruebas JVM.                                                                                                             |
| Disco                                        | ≈ 38 GiB libres al final                                                                                                                                                       | Del proyecto de prueba quedan solo las fuentes (las carpetas `build`, unos 130 MB, se borraron); también los AAR y las páginas descargadas para la verificación.                                                                                      |

**Nada obliga a detenerse.** No hace falta instalar Android Studio ni paquetes del SDK. Sin `cmdline-tools` tampoco
habría sido parada (AGP descarga lo que necesita), y el adb viejo funcionaba; ambos ya están al día.

#### 1.2 Versiones elegidas (todas estables al 16/11/2026)

Fuente: `maven-metadata.xml` de Google Maven (`dl.google.com/dl/android/maven2/…`) y Maven Central
(`repo1.maven.org/maven2/…`), `services.gradle.org/versions/current`, notas de AGP 9.5
(`developer.android.com/build/releases/gradle-plugin`) y la tabla del BOM
(`developer.android.com/develop/ui/compose/bom/bom-mapping`). Se descartaron alfa, beta y RC. Todas compilaron
juntas en el proyecto de prueba (las marcadas con † no se usaron en él, pero su AAR se revisó para el `minSdk`).

| Pieza                                              | Versión                                                                                                                                                                                                                                     | minSdk real (manifiesto del AAR)  | Nota                                                                                                                                                                                 |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Gradle (wrapper)                                   | **9.9.1** (actual)                                                                                                                                                                                                                          | —                                 | AGP 9.5 pide ≥ 9.6.0. Sin avisos de obsolescencia con AGP 9.5.1.                                                                                                                     |
| AGP                                                | **9.5.1**                                                                                                                                                                                                                                   | —                                 | Máximo API 37, build-tools 36.0.0, JDK ≥ 17. Kotlin integrado («built-in Kotlin»): **no** se aplica `org.jetbrains.kotlin.android`. 9.6.0 está en alfa.                              |
| Kotlin (KGP, compilador de Compose, serialización) | **2.4.21**                                                                                                                                                                                                                                  | —                                 | AGP 9.5.1 trae KGP 2.2.10 como dependencia; declarar `org.jetbrains.kotlin.plugin.compose` 2.4.21 lo sube (comprobado con `buildEnvironment`: `2.2.10 -> 2.4.21`). 2.5.0 está en RC. |
| KSP                                                | **2.3.14**                                                                                                                                                                                                                                  | —                                 | KSP2 (versión independiente de Kotlin).                                                                                                                                              |
| Compose BOM                                        | **2026.09.00**                                                                                                                                                                                                                              | —                                 | Compose UI/foundation **1.12.1** (minSdk 23), Material 3 **1.4.0** (21), `material3-adaptive-navigation-suite` 1.4.0 (21)†.                                                          |
| material3-adaptive                                 | **1.3.0** (`adaptive`, `adaptive-layout` 23, `adaptive-navigation` 23†)                                                                                                                                                                     | 23                                | Para los dos paneles (`ListDetailPaneScaffold`) y `currentWindowAdaptiveInfoV2()` (la V1 está en desuso: aviso real del compilador).                                                 |
| androidx.core                                      | 1.19.1                                                                                                                                                                                                                                      | 23                                |                                                                                                                                                                                      |
| activity-compose                                   | 1.13.0                                                                                                                                                                                                                                      | 23                                | `enableEdgeToEdge()`.                                                                                                                                                                |
| lifecycle (runtime/viewmodel-compose)              | 2.11.0                                                                                                                                                                                                                                      | 23                                |                                                                                                                                                                                      |
| navigation-compose                                 | 2.10.2                                                                                                                                                                                                                                      | 24                                | Alternativa: navigation3 1.2.1 (24)†; propongo navigation-compose (estable y conocida); decidir en la Fase A.                                                                        |
| window                                             | 1.5.1†                                                                                                                                                                                                                                      | 23                                | Transitiva de adaptive.                                                                                                                                                              |
| Room                                               | **2.8.5** (+ plugin `androidx.room` para exportar el esquema)                                                                                                                                                                               | 23 (`sqlite-framework` 2.7.1: 23) | Con KSP; `room-testing` para migraciones.                                                                                                                                            |
| WorkManager                                        | **2.12.0**                                                                                                                                                                                                                                  | 24                                | `work-testing` 2.12.0 (24).                                                                                                                                                          |
| OkHttp / okhttp-sse / okhttp-tls / mockwebserver3  | **5.5.1**                                                                                                                                                                                                                                   | 21 (`okhttp-android`)             | Sin Retrofit (no hace falta para ~30 rutas); `okhttp-tls` y `mockwebserver3` solo en pruebas.                                                                                        |
| kotlinx.serialization-json                         | **1.13.0**                                                                                                                                                                                                                                  | JVM                               | Cadenas decimales para el dinero (nunca `Double`).                                                                                                                                   |
| kotlinx.coroutines                                 | **1.11.0** (android y test)                                                                                                                                                                                                                 | JVM                               |                                                                                                                                                                                      |
| Imágenes                                           | **caché propia** (OkHttp + `ETag`/`If-None-Match` del servidor)                                                                                                                                                                             | —                                 | La especificación pide «caché propia» y el servidor ya da `getCatalogImageContent` con 304. Alternativa: Coil 3.6.3† (minSdk 23).                                                    |
| DataStore                                          | 1.3.0†                                                                                                                                                                                                                                      | 24                                | Para preferencias del dispositivo (si hace falta; Room puede guardarlas).                                                                                                            |
| security-crypto                                    | 1.1.0†                                                                                                                                                                                                                                      | 21                                | **En desuso** (sus clases llevan `@Deprecated`): usar directamente Android Keystore con AES-GCM.                                                                                     |
| Pruebas                                            | JUnit **4.13.2**, androidx.test runner/core/rules **1.7.0**, ext-junit **1.3.0**, Robolectric **4.18**, Compose UI test (BOM), Room testing 2.8.5, WorkManager testing 2.12.0, MockWebServer 5.5.1, uiautomator 2.4.1† (23), Turbine 1.2.1† | ≤ 24                              | Espresso 3.7.0 si hace falta.                                                                                                                                                        |

**Todas las bibliotecas admiten `minSdk` 26** (la que más pide es 24: navigation, WorkManager y DataStore); la
compilación con `minSdk = 26` lo confirma (la fusión de manifiestos falla si una pide más). Ninguna exige alternativa.

#### 1.3 `minSdk`, `targetSdk` y `compileSdk`

- **`minSdk = 26`** (especificación). `java.time` existe de forma nativa desde la API 26: sin «desugaring».
- **`targetSdk = 36`**: Google Play exige **API 36** a apps nuevas y actualizaciones **desde el 31 de agosto de 2026**
  (prórroga posible al 1 de noviembre de 2026); las existentes, API 35 para seguir visibles
  (`developer.android.com/google/play/requirements/target-sdk`, actualizada el 28/10/2026).
- **`compileSdk` 37** (`android-37.0`, lo instalado; AGP 9.5 admite hasta 37). Declaración con AGP 9.5.1 (probada):

  ```kotlin
  android {
      compileSdk {
          version = release(37)
      }
      defaultConfig {
          minSdk = 26
          targetSdk = 36
      }
  }
  ```

  (`compileSdk = 37` y `compileSdkMinor` también existen en la DSL de AGP 9.5.1.)

- **Al pasar a `targetSdk` 37** (cuando Play lo exija, previsiblemente en agosto de 2027): las protecciones de red
  local de Android 17 bloquean **todo** el tráfico a la red local (TCP con OkHttp incluido, mDNS y `NsdManager`) sin el
  permiso de ejecución `ACCESS_LOCAL_NETWORK` (grupo «Dispositivos cercanos»). Con `targetSdk` ≤ 36 el permiso no se
  declara ni se pide (`developer.android.com/privacy-and-security/local-network-permission`). La Fase C debe dejar la
  capa de permisos preparada (pantalla de explicación en español) aunque hoy no pida nada.

#### 1.4 Prueba de humo: proyecto de prueba `$SP/spike-android/`

Pila elegida con inyección manual (`SpikeApplication` + `AppContainer`): Compose M3 con `currentWindowAdaptiveInfoV2`,
Room con KSP (esquema exportado en `app/schemas/`), WorkManager (`SendPendingWorker`), OkHttp con huella fijada
(`PinnedTrustManager.kt`, `ServerClient.kt`), derivación del código de emparejamiento (`Pairing.kt`) y una
`network_security_config` **solo en `src/debug`** que permite texto claro únicamente hacia `10.0.2.2`.

Comandos: `$SP/p0/gradle-spike.sh` (mismo entorno que `iso.sh gradle`) y `$SP/p0/connected.sh` (lee la huella SPKI de
`run-P0/identity/server-cert.pem`, el `server_id` y el token de aprovisionamiento y los pasa como argumentos de la
instrumentación `pin`, `serverId`, `provisioning`, `net` y `local`).

**Huella fijada.** `PinnedTrustManager` compara en tiempo constante SHA-256 de `certificate.publicKey.encoded` (la
SubjectPublicKeyInfo en DER, lo mismo que `RawSubjectPublicKeyInfo` de Go y `spki_fingerprint` de
`desktop/src-tauri/src/pairing.rs:60`) y no mira nombre, IP, cadena ni vigencia; el cliente usa
`hostnameVerifier { _, _ -> true }` porque **el certificado de Go no trae nombres alternativos** (comprobado con
`openssl x509`: sin extensiones; sujeto `O=AD-EL POS, CN=AD-EL POS <server_id>`), así que la identidad la da la huella,
como `PinnedVerifier` (`pairing.rs:94`). Rust desactiva la reanudación TLS (`pairing.rs:214`); en Android cada cliente
crea su propio `SSLContext`, cuya caché solo contiene sesiones validadas con esa huella (equivalente).

Pruebas instrumentadas (`GoTemporalTest`, nueve, en orden):

| Prueba | Qué demuestra                                                                                                                                                 | Resultado (API 37, Pixel_8)                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| t01    | `GET /version` por `https://10.0.2.2:8451` con la huella fijada                                                                                               | 200; **TLS 1.3** `TLS_AES_128_GCM_SHA256` (API 33 igual; **API 26: TLS 1.2** `TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256`)                                                                                                                                                                                                                                                                                                                                                                    |
| t02    | Otra huella contra Go → rechazo; otro certificado (MockWebServer TLS dentro del emulador) con la huella de Go → rechazo; ese mismo con su propia huella → 200 | `SSLHandshakeException` en los dos rechazos                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| t03    | Cabeceras por la escucha de red                                                                                                                               | Sin cabeceras **426** `CLIENT_UPDATE_REQUIRED` (`details`: `min_client_version`, `server_version`); con `0.12.0`/`android` 200; `0.9.0` 426; tipo `ios` 400 `CLIENT_KIND_INVALID`                                                                                                                                                                                                                                                                                                          |
| t04    | ¿La escucha local HTTP vista desde el emulador es `Request.Local`?                                                                                            | **Sí**: `POST devices/enroll {}` por `http://10.0.2.2:8191` pasa la comprobación local y responde 400 `VALIDATION_ERROR` (no escribe nada); por la red, 403 `ENROLLMENT_LOCAL_ONLY`                                                                                                                                                                                                                                                                                                        |
| t05    | Emparejamiento completo como `android`                                                                                                                        | Asistente por la escucha local con el token de aprovisionamiento, sesión `admin`; por la red `createPairingRequest` (200, `server_id` correcto: el filtro de orígenes lo acepta), `revealPairingNonce`, código calculado en Kotlin, **aprobación por la escucha local** (`devices/pairing-requests/{id}/approve`), estado `approved`, `claimPairingCredential` (credencial); segundo reclamo 409 `PAIRING_ALREADY_CLAIMED`; `login` y `GET restaurant/state` 200 con la credencial Android |
| t06    | SSE con okhttp-sse                                                                                                                                            | Primer mensaje `id: 0`, `event: changes`, `{"cursor":0,"reset":false,"events":[]}`; un mensaje cada ≈ 760 ms; categoría creada desde la principal → evento `catalog.changed` (ámbito `catalog`) en ≈ 0.2–0.7 s; reanudación con `Last-Event-ID` anterior repite el evento; `?after=999999999` → `reset: true`; el servidor cierra el flujo a los **30 s** (41 mensajes)                                                                                                                    |
| t07    | Cortar y devolver la red del emulador                                                                                                                         | Ver «Corte de red» abajo                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| t08    | Revocación desde la principal                                                                                                                                 | El flujo SSE abierto recibe `event: interrupted` `{"error":"DEVICE_REVOKED","status":401}` en < 1 s; `restaurant/state` → 401 `DEVICE_REVOKED`                                                                                                                                                                                                                                                                                                                                             |
| t09    | NSD en el emulador                                                                                                                                            | Ver §1.5                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |

**`POST pairing/requests` desde el emulador lo acepta el filtro de orígenes**: el emulador sale por la red de usuario
del emulador y llega a `127.0.0.1:8451` como loopback (`lanlistener` acepta loopback). Consecuencia para las pruebas:
**todo el tráfico de todos los emuladores cuenta como un solo origen** (`v4:127.0.0.1`), con el límite de **5
solicitudes por origen y 30 en total cada 10 minutos** (`backend/internal/administration/pairing.go:70`, `:72`): cada
suite debe emparejar pocas veces (una por servidor temporal) o usar un Go temporal nuevo.

**La escucha local sirve como «computadora principal» en las pruebas** (`backend/internal/administration/http.go:40`:
`local` = loopback y no `FromNetwork`; el emulador llega a 8191 como 127.0.0.1). Desde la prueba se puede: configurar
con el token de aprovisionamiento, iniciar sesión como `admin`, crear catálogo, aprobar el código, cambiar precios,
marcar agotados, cobrar o cerrar cuentas y revocar el dispositivo (t05, t06 y t08 lo hicieron). Requiere texto claro
hacia `10.0.2.2` **solo** en la variante de pruebas: `app/src/debug/AndroidManifest.xml` +
`res/xml/network_security_config.xml` (base sin texto claro; `domain-config` solo `10.0.2.2`). La app de entrega no la
lleva: el APK de prueba para el propietario debe salir del tipo `release` firmado con la llave de desarrollo (o de un
tipo de compilación propio), nunca del `debug`.

**Corte de red** (órdenes probadas; «falla» = la petición HTTPS a Go lanza `ConnectException`):

| Orden                                                                                                                                    | API 37 (Pixel_8, Play)                       | API 33 (Phone, google_apis)       | API 26 (Tablet, google_apis)                                                                                                                           |
| ---------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------- | --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `cmd connectivity airplane-mode enable/disable` (desde la prueba con `UiAutomation.executeShellCommand` o desde el host con `adb shell`) | Falla en 0.3–0.44 s; vuelve en **2.9–3.1 s** | Falla en 0.7 s; vuelve en 3.4 s   | **No existe** (la orden no hace nada)                                                                                                                  |
| `svc wifi disable` solo                                                                                                                  | No corta (sigue la red celular)              | No corta                          | No hace nada como `shell`                                                                                                                              |
| `svc data disable` solo                                                                                                                  | Corta en 0.27 s; vuelve en 0.28 s            | No corta en 15 s (sigue la wifi)  | No hace nada como `shell`                                                                                                                              |
| `svc wifi disable` + `svc data disable`                                                                                                  | Corta en 0.28 s; vuelve en 0.28 s            | Corta en 0.29 s; vuelve en 0.30 s | Como `shell` no hace nada; **con `adb root` desde el host sí** (`Active default network: none`, `ping` falla; vuelve en ≈ 0.3 s); después `adb unroot` |

Orden recomendada: `cmd connectivity airplane-mode` en API ≥ 30 (desde la propia prueba), y en API 26 un paso del
host con `adb root` + `svc wifi/data` (solo imágenes `google_apis`; las Play no permiten root).

**Tiempos y tamaños** (Mac de 10 núcleos):

| Paso                                                                   | Tiempo                                                                                     |
| ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| Primer `assembleDebug` (en frío: descarga Gradle 9.9.1 y dependencias) | 1 min 45 s                                                                                 |
| `assembleDebug testDebugUnitTest` (incluye la descarga de Robolectric) | 1 min 03 s; la prueba de Room + WorkManager en Robolectric tarda 22.4 s la primera vez     |
| `connectedDebugAndroidTest` completo en Pixel_8 (9 pruebas)            | 1 min 41 s (93.2 s de pruebas: 30 s esperan el cierre del flujo SSE, 20 s la búsqueda NSD) |
| `assembleRelease` con R8 (`isMinifyEnabled`, `isShrinkResources`)      | 34 s, sin avisos de R8                                                                     |

APK de depuración **13.1 MiB** (13 737 232 bytes); APK de pruebas 1.3 MB; `release` reducido sin firmar **0.97 MiB**
(1 017 279 bytes; con poco código: el real crecerá).

**Avisos observados**: `Unable to strip … libandroidx.graphics.path.so` (no hay NDK; inofensivo);
`currentWindowAdaptiveInfo` en desuso → `currentWindowAdaptiveInfoV2` (corregido); con JDK 25, avisos de
`sun.misc.Unsafe` (protobuf de AGP) y de acceso nativo de Robolectric (inofensivos hoy); `adb logcat -c` falla en API 26
(«failed to clear the 'main' log»: ignorarlo); **MockWebServer 5.5.1 se cae como servidor TLS dentro de Android 8**
(`getRequestedServerNames(...) must not be null`): esa parte de t02 se omite en API < 29 (es la herramienta de prueba,
no la app). Gradle 9.9.1 + AGP 9.5.1 + JBR 25 no dieron avisos de compatibilidad.

#### 1.5 NSD/mDNS en el emulador

Prueba t09: un servicio `_adelspike._tcp` anunciado **dentro del emulador** con `NsdManager.registerService` (TXT con
`server_id` y `fp`) y, a la vez, un anuncio corto en la Mac con `dns-sd -R "Spike Mac" _adelspike._tcp local 9`
(tipo propio, nunca `_adelpos._tcp`; vivo solo mientras corría cada prueba, ≈ 1–2 min, y detenido después; el Go
temporal nunca se anuncia).

|                                                         | API 37 (Pixel_8, netsim)                                                            | API 33 | API 26                                                          |
| ------------------------------------------------------- | ----------------------------------------------------------------------------------- | ------ | --------------------------------------------------------------- |
| Anuncio propio: registro, búsqueda y resolución con TXT | Sí                                                                                  | Sí     | Sí (con `NsdServiceInfo.host`; `hostAddresses` es de la API 34) |
| Anuncio de la Mac visto por el emulador                 | **Sí el PTR** (aparece en la búsqueda), pero **la resolución no respondió en 10 s** | No     | No                                                              |

Conclusión: el emulador no sirve para encontrar un servidor real (y el Go temporal escucha solo en 127.0.0.1 y no se
anuncia). Se podrá probar: **en JVM**, la lectura del TXT y la elección del servidor por `server_id` + huella corta;
**en el emulador**, el flujo de `NsdManager` contra un anuncio simulado dentro del propio dispositivo (verificado en
las tres versiones); **la búsqueda real**, solo en un dispositivo físico en la red del restaurante (parada: conectar un
teléfono o tableta).

#### 1.6 Diferencias de plataforma entre API 26 y 37 que el código debe manejar

| Tema                              | Desde                                                   | Qué hacer en esta etapa                                                                                                                                                                                                                                                                                                                                                  |
| --------------------------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| TLS 1.3                           | API 29                                                  | En 26–28 se negocia TLS 1.2 (`ECDHE-ECDSA-AES128-GCM`, verificado contra Go, que acepta ≥ 1.2). `ConnectionSpec` `RESTRICTED_TLS` + `MODERN_TLS`.                                                                                                                                                                                                                        |
| NSD                               | API 34                                                  | `resolveService` y `NsdServiceInfo.host` en desuso desde 34 → `registerServiceInfoCallback` y `hostAddresses` en 34+, los anteriores en 26–33 (resolver de uno en uno: antes de 34 una resolución simultánea falla con `FAILURE_ALREADY_ACTIVE`).                                                                                                                        |
| Permiso de red local              | Android 16 (opt-in), **obligatorio con `targetSdk` 37** | Nada con `targetSdk` 36. Al subir a 37: `ACCESS_LOCAL_NETWORK` en tiempo de ejecución o el selector del sistema de `NsdManager` (`DiscoveryRequest.FLAG_SHOW_PICKER`).                                                                                                                                                                                                   |
| `NEARBY_WIFI_DEVICES`             | API 33                                                  | No lo necesita `NsdManager` (sí las API de Wi-Fi). Con `targetSdk` 36 no se pide; anotar para la Fase C.                                                                                                                                                                                                                                                                 |
| `POST_NOTIFICATIONS`              | API 33                                                  | Solo si la app avisa en segundo plano (p. ej. «pedido requiere revisión» o, en la A2, platillos listos). Sin permiso, el trabajo sigue corriendo.                                                                                                                                                                                                                        |
| Trabajo urgente de WorkManager    | < API 31                                                | `setExpedited` corre como servicio en primer plano en 26–30: el `CoroutineWorker` debe implementar `getForegroundInfo()` (si no, falla en Android 8–11). Preferir trabajo normal con espera creciente.                                                                                                                                                                   |
| Tipos de servicio en primer plano | API 34                                                  | Si alguna vez se usa `setForeground`: `foregroundServiceType="dataSync"` + `FOREGROUND_SERVICE_DATA_SYNC` (y el límite de 6 h de Android 15). Evitarlo en la A1.                                                                                                                                                                                                         |
| Restricción de red de WorkManager | todas                                                   | **Riesgo por verificar en la Fase E**: `NetworkType.CONNECTED` puede exigir una red «validada» (con salida a internet); la wifi del restaurante sin internet no se valida y la cola no saldría. Mitigación: `setRequiredNetworkRequest` sin `NET_CAPABILITY_INTERNET` (API 28+) o sin restricción de red y comprobar el servidor en el trabajador, con espera creciente. |
| Estado de la red                  | API 24+                                                 | `registerDefaultNetworkCallback` para el indicador; no exigir `NET_CAPABILITY_VALIDATED` (mismo motivo).                                                                                                                                                                                                                                                                 |
| Android Keystore                  | API 23                                                  | AES-256-GCM (`AES/GCM/NoPadding`, IV generado por el Keystore) en 26+; StrongBox opcional en 28+ con respaldo sin él. Si el Keystore falla o se pierde la llave (datos borrados), se empareja de nuevo.                                                                                                                                                                  |
| Copias de seguridad               | API 31 (`dataExtractionRules`)                          | `allowBackup="false"` y reglas que excluyan Room y la credencial (cifradas con una llave que no viaja).                                                                                                                                                                                                                                                                  |
| Edge-to-edge                      | obligatorio con `targetSdk` 35+ en Android 15+          | `enableEdgeToEdge()` en todas las versiones e `insets` en Compose, para que 26 y 37 se vean igual.                                                                                                                                                                                                                                                                       |
| Pantallas grandes                 | Android 16 con `targetSdk` 36                           | En pantallas ≥ 600 dp se ignoran orientación fija y no redimensionable: la app debe rotar y redimensionarse (ya lo pide §2).                                                                                                                                                                                                                                             |
| Atrás predictivo                  | Android 16 con `targetSdk` 36                           | No usar `onBackPressed`; `BackHandler` de Compose.                                                                                                                                                                                                                                                                                                                       |
| Tamaños de ventana                | —                                                       | `currentWindowAdaptiveInfoV2()`: Pixel_8 ≈ 411×914 dp (compacto), Pixel_Tablet y Tablet API 26 ≈ 1280×800 dp (expandido); teléfono horizontal = ancho expandido con alto compacto (considerar el alto). Multiventana desde la API 24.                                                                                                                                    |
| Páginas de 16 KB                  | Android 15+                                             | AGP 9 alinea las bibliotecas nativas; la app no tiene código nativo propio.                                                                                                                                                                                                                                                                                              |

#### 1.7 Matriz de pruebas por versión (propuesta)

| Dispositivo (consola)                             | Android      | Para qué                                                                                                     | Corte de red                       |
| ------------------------------------------------- | ------------ | ------------------------------------------------------------------------------------------------------------ | ---------------------------------- |
| `ADEL_A1_Tablet_API26` (5590), 1280×800 a 160 dpi | 8.0 (mínimo) | Tableta económica: TLS 1.2, NSD antiguo, Keystore, WorkManager sin trabajo urgente, dos paneles, rendimiento | Host: `adb root` + `svc wifi/data` |
| `ADEL_A1_Phone_API33` (5592), pixel_6a            | 13           | Teléfono «de hace 2–3 años»: TLS 1.3, permisos de notificaciones, un panel                                   | `cmd connectivity airplane-mode`   |
| `Pixel_8` (5580)                                  | 17           | Teléfono actual, comportamiento de `targetSdk` 36 en el último sistema, edge-to-edge obligatorio             | `cmd connectivity airplane-mode`   |
| `Pixel_Tablet` (5582)                             | 17           | Tableta actual, dos paneles, rotación                                                                        | `cmd connectivity airplane-mode`   |
| Dispositivo físico (si el propietario lo conecta) | el que haya  | Búsqueda NSD real y el flujo principal                                                                       | real                               |

Reparto sugerido: JVM y Robolectric (SDK 36; añadir 26 solo para código que dependa de la versión) en cada fase;
Compose en teléfono y tableta (API 33 y 26 o 37); extremo a extremo completo en API 37 y API 26 en la Fase F, y en la G
la suite completa en las cuatro.

Resultados reales en esta fase:

| AVD                   | Pruebas                          | Resultado                                                                                                                                                                                                                                                                                               |
| --------------------- | -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Pixel_8 (API 37)      | 9 instrumentadas                 | **9/9** (93.2 s) con la versión anterior de t02/t07/t09; con la final, t01–t02 2/2                                                                                                                                                                                                                      |
| Pixel_Tablet (API 37) | t01–t04                          | **4/4**                                                                                                                                                                                                                                                                                                 |
| ADEL_A1_Phone_API33   | 9 instrumentadas (versión final) | **9/9** (92.5 s)                                                                                                                                                                                                                                                                                        |
| ADEL_A1_Tablet_API26  | 9 instrumentadas                 | **8/9**: HTTPS con huella fijada y TLS 1.2, rechazo de otra huella, 426, escucha local, emparejamiento, SSE con reanudación, revocación y NSD propio bien; **t07 falla** porque ninguna orden de corte de red funciona como `shell` en Android 8 (corte verificado aparte desde el host con `adb root`) |

### 2. La API v1 desde el punto de vista del mesero

Contrato: `backend/internal/apicontract/openapi.json` (26 operaciones) y `docs/API_V1.md`. «Servidor, no contrato» =
ruta que ya existe y usa el escritorio, pero que no está documentada para clientes (hay que agregarla a
`openapi.json`, `docs/API_V1.md` y `TestContractV1`). «No existe» = hay que construirla.

| Necesidad                                                              | Qué existe (evidencia)                                                                                                                                                                                                                                                                                                                                        | Qué falta                                                                                                                                                                                                                     | Cambio propuesto (compatible, v1)                                                                                                                                                                      | Fase |
| ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---- |
| Emparejamiento                                                         | Contrato completo: `createPairingRequest`… `claimPairingCredential` (`openapi.json:1156–1524`); tipo `android` aceptado (`administration/pairing.go:92`, migración 0023:219); aprobación solo local (`pairing.go:629`). **Verificado de punta a punta (t05).**                                                                                                | —                                                                                                                                                                                                                             | Ninguno                                                                                                                                                                                                | C    |
| Versión mínima de cliente                                              | `getVersion`, 426 y `X-Client-Kind: android` (`httpapi/clientversion.go:35`, `:134`, `:158`)                                                                                                                                                                                                                                                                  | **Una sola mínima (0.10.0) para todos los tipos**: la app 0.1.0 recibe 426 (comprobado)                                                                                                                                       | `min_client_versions` por tipo (`desktop` 0.10.0, `android` 0.1.0) en `config/product.json`, en `checkClient` y en `getVersion` (propiedad nueva; `min_client_version` se conserva para el escritorio) | B    |
| Entrada con PIN                                                        | No existe (solo `login` con contraseña, `administration/auth.go:14`)                                                                                                                                                                                                                                                                                          | Todo                                                                                                                                                                                                                          | Ver «Diseño del PIN»                                                                                                                                                                                   | B    |
| Lista de usuarios para el dispositivo (§3.1)                           | No existe (`GET users` pide `users.read`)                                                                                                                                                                                                                                                                                                                     | Ruta con credencial de dispositivo, sin sesión                                                                                                                                                                                | `GET auth/pin-users`                                                                                                                                                                                   | B    |
| Permiso «usar dispositivos de meseros»                                 | No existe nada equivalente                                                                                                                                                                                                                                                                                                                                    | Permiso nuevo                                                                                                                                                                                                                 | `waiter_devices.use`                                                                                                                                                                                   | B    |
| Bloqueo por inactividad configurable por dispositivo (§3.2)            | `devices` guarda `home_screen`, `keyboard_mode` y caja (`PATCH devices/{id}`, `administration/devices.go:93`)                                                                                                                                                                                                                                                 | Columna y campo                                                                                                                                                                                                               | `devices.idle_lock_seconds` (120 por omisión), en el PATCH de Terminales y en `Terminal` (`restaurant/state` y `auth/me`)                                                                              | B    |
| Detección de dispositivo revocado (§3.2)                               | 401 `DEVICE_REVOKED` en toda ruta (`service.go:335`) y `event: interrupted` en el SSE (`events.go:130`). **Verificado (t08).**                                                                                                                                                                                                                                | —                                                                                                                                                                                                                             | Ninguno                                                                                                                                                                                                | C/E  |
| Invalidar el verificador local del PIN (§3.2)                          | No existe                                                                                                                                                                                                                                                                                                                                                     | Señal de «el PIN cambió / el usuario ya no puede»                                                                                                                                                                             | `pin_version` en `auth/pin-users`; cambiar o quitar el PIN cierra sus sesiones Android                                                                                                                 | B    |
| Catálogo completo y por cambios con época                              | `getCatalogSync` (`openapi.json:330`; `backend/internal/catalog/sync.go:104`): `since`+`epoch`, `reset`, `deleted`, categorías, productos con presentaciones, ingredientes, complementos, **porciones**, grupos y opciones (con `portion_name`/`portion_factor`), combos (`slots`), notas rápidas, imágenes aparte con `ETag` (`openapi.json:546`), «Agotado» | —                                                                                                                                                                                                                             | Ninguno (permisos `catalog.read` + `catalog.prices.read`)                                                                                                                                              | D    |
| Personalizador (complementos, Sin/Normal/Extra, combos, notas rápidas) | En la descarga (arriba) y `Customization` (`ingredients`, `groups`, `components`, `notes`, `quick_notes`)                                                                                                                                                                                                                                                     | —                                                                                                                                                                                                                             | Ninguno                                                                                                                                                                                                | D/E  |
| Búsqueda de productos                                                  | Se hace sobre Room (no necesita servidor); `listCatalogProducts` existe                                                                                                                                                                                                                                                                                       | —                                                                                                                                                                                                                             | Ninguno                                                                                                                                                                                                | E    |
| Áreas y mesas con su estado                                            | `restaurant/state` trae `tables` con `state`, `occupied_since` y `area_id` (`openapi.json:674`); **servidor, no contrato**: `GET restaurant/overview` (áreas + mesas + umbrales; `operations/areas.go:113`, permisos `restaurant.tables.read` y `restaurant.areas.read`)                                                                                      | Nombres de áreas y posiciones del mapa en el contrato                                                                                                                                                                         | Documentar `getRestaurantOverview` y las propiedades que Go ya envía en `Table` (`shape`, `x`, `y`, `width`, `height`, `seats`, `occupied_seconds`, `long_open`; `operations/models.go:23`)            | E    |
| Mesas que el mesero no puede atender (§6.1)                            | **El servidor no restringe mesas ni cuentas por mesero** (solo por permisos; `operations/queries.go:105`)                                                                                                                                                                                                                                                     | —                                                                                                                                                                                                                             | Ninguno: se muestran todas                                                                                                                                                                             | E    |
| Cuentas de una mesa, abrir con creación diferida y borrador            | `listAccounts`, `openAccount` (con el primer producto y `seen_account_ids` → 409 `TABLE_HAS_ACCOUNT`), `getAccount`, `saveDraft` (409 `ACCOUNT_CONFLICT`), `previewDraft`                                                                                                                                                                                     | —                                                                                                                                                                                                                             | Ninguno                                                                                                                                                                                                | E    |
| Enviar a cocina con clave de operación                                 | `confirmDraft` con `operation_id` y huella (409 `QUOTE_CHANGED`)                                                                                                                                                                                                                                                                                              | —                                                                                                                                                                                                                             | Ninguno                                                                                                                                                                                                | E    |
| Pedido sin conexión con validación                                     | `submitOfflineOrder` (`openapi.json:1095`; `operations/offline_orders.go:131`): 409 `ORDER_REQUIRES_CONFIRMATION` con cambios (`price_changed`, `product_sold_out`, `account_closed`…), confirmación con la misma clave, 409 `OFFLINE_ORDER_ALREADY_ACCEPTED` (`operations/handler.go:319`)                                                                   | Convivencia con el borrador guardado en el servidor (ver decisión E-1)                                                                                                                                                        | Según la decisión                                                                                                                                                                                      | E    |
| Modificar o cancelar unidades según permisos                           | **Servidor, no contrato**: `POST restaurant/accounts/{id}/cancel` (`orders.cancel`, motivo de 5 a 500 caracteres; `operations/cancellations.go:19–21`), `edit-preview` y `edit` (`orders.edit`, motivo y huella; `amendments.go:267`, `:283`)                                                                                                                 | Contrato                                                                                                                                                                                                                      | Documentar `cancelUnits`, `previewEdit` y `editUnits` con sus pruebas de contrato                                                                                                                      | E    |
| Estados de preparación                                                 | `preparation_state` de la cuenta y de cada partida confirmada en `getAccount`/`restaurant/state` (`without_items`, `draft`, `active`, `preparing`, `ready`, `change_pending`, `cancelled`; `operations/accounts_read.go:62–71`)                                                                                                                               | **Aviso en tiempo real al mesero**: el cambio de estado solo publica `kitchen.state.changed` en el ámbito `kitchen` con estación (`kitchen/states.go:128`); un mesero sin `kitchen.states.read` y sin estaciones no lo recibe | Evento nuevo `restaurant.account.preparation_changed` (ámbito `restaurant`, agregado = cuenta) en la misma transacción. También sirve a la A2 («platillos listos»)                                     | E    |
| Mesa cobrada mientras tanto (§6.4)                                     | Sin conexión: `account_closed` en `submitOfflineOrder`. En línea: `ensureOpen` responde **400 `OPERATION_VALIDATION`** genérico (`operations/queries.go:95`)                                                                                                                                                                                                  | Distinguirlo por código                                                                                                                                                                                                       | Sin cambiar estado ni código (compatibilidad): agregar `details.reason: "account_not_open"`; o enviar siempre por `submitOfflineOrder` (E-1)                                                           | E    |
| Cambios de otras terminales (§6.3)                                     | SSE ámbito `restaurant` (`restaurant.account.*`, `restaurant.*.changed`) con la cuenta como agregado; revisión de la cuenta y 409 `ACCOUNT_CONFLICT`                                                                                                                                                                                                          | —                                                                                                                                                                                                                             | Ninguno                                                                                                                                                                                                | E    |
| Recuperación del estado                                                | `getRecoveryState` (`administration/recovery.go:19`): permisos vigentes, terminal, `catalog_version`/`catalog_epoch`, mesas y cuentas con revisión y estados. **Verificado (t05).**                                                                                                                                                                           | El cursor de eventos del momento                                                                                                                                                                                              | Opcional: `event_cursor` en la respuesta (propiedad nueva)                                                                                                                                             | E    |
| Eventos en tiempo real                                                 | `streamEvents` (ver §3). **Verificado (t06).**                                                                                                                                                                                                                                                                                                                | —                                                                                                                                                                                                                             | Ninguno                                                                                                                                                                                                | E    |
| «Agotado» desde el dispositivo                                         | `setProductSoldOut` existe, pero **la especificación excluye «Agotado»** para el mesero                                                                                                                                                                                                                                                                       | —                                                                                                                                                                                                                             | No usar en la A1                                                                                                                                                                                       | —    |
| Impresora de precuentas por dispositivo (§8)                           | Solo **por caja**: `register_receipt_printers` (migración 0023:440, D10); un dispositivo hereda la de su `register_id`                                                                                                                                                                                                                                        | Asignación por dispositivo sin caja                                                                                                                                                                                           | Anotado para la A2 (no hacer nada ahora)                                                                                                                                                               | A2   |

#### Diseño propuesto del PIN en el servidor (§3.1; sin implementar)

**Migración `backend/migrations/0025_…sql`, sección B:**

```sql
-- B. PIN de los meseros (etapa A1, §3).
ALTER TABLE app.users
    ADD COLUMN pin_hash text,               -- Argon2id, mismo formato que password_hash
    ADD COLUMN pin_set_at timestamptz,
    ADD COLUMN pin_version integer NOT NULL DEFAULT 0,   -- sube al definir, cambiar o quitar el PIN
    ADD COLUMN short_name text CONSTRAINT users_short_name_check CHECK (short_name IS NULL OR length(short_name) BETWEEN 1 AND 20),
    ADD COLUMN badge_color text CONSTRAINT users_badge_color_check CHECK (badge_color IS NULL OR badge_color ~ '^#[0-9a-f]{6}$');
CREATE TABLE app.pin_attempts (                       -- por usuario (no por dispositivo): §3.1 bloquea «el PIN de ese usuario»
    user_id uuid PRIMARY KEY CONSTRAINT pin_attempts_user_id_fkey REFERENCES app.users(id),
    failures integer NOT NULL DEFAULT 0,
    window_start timestamptz NOT NULL DEFAULT now(),
    locked_until timestamptz,
    last_device_id uuid CONSTRAINT pin_attempts_last_device_id_fkey REFERENCES app.devices(id)
);
ALTER TABLE app.devices
    ADD COLUMN idle_lock_seconds integer NOT NULL DEFAULT 120
        CONSTRAINT devices_idle_lock_seconds_check CHECK (idle_lock_seconds BETWEEN 30 AND 3600);
INSERT INTO app.permissions(key, description) VALUES
    ('waiter_devices.use', 'Entrar con PIN en dispositivos de meseros');
INSERT INTO app.role_permissions(role_id, permission_key)
SELECT id, 'waiter_devices.use' FROM app.roles WHERE code = 'administrator';   -- y ¿Mesero? (decisión B-2)
```

- **Hash**: `HashPassword`/`VerifyPassword` de `backend/internal/administration/security.go:18` (Argon2id, m=64 MiB,
  t=3, p=1, sal de 16 bytes). Al no existir el usuario o el PIN, se verifica contra un hash ficticio (como `dummyHash`,
  `auth.go:12`) para no revelar por el tiempo quién tiene PIN.
- **PIN triviales** (400 con mensaje claro): 4 a 6 dígitos; se rechazan todos los dígitos iguales (`0000`, `111111`),
  secuencias ascendentes o descendentes de paso 1 (`1234`, `123456`, `4321`, `0123`, `9876`), patrones repetidos de
  periodo 2 o 3 (`1212`, `121212`, `123123`) y una lista corta de muy usados (`2580`, `0852`, `1122`, `1004`, `2000`,
  `6969`, `1313`, `4444`…). Prueba unitaria con la lista.
- **Bloqueo**: 5 fallos dentro de 15 minutos (desde cualquier dispositivo) bloquean el PIN de ese usuario **5
  minutos** (`429 PIN_LOCKED` con `details.locked_until`, como `LOGIN_THROTTLED`); se registra una vez por bloqueo en el Registro de
  movimientos (`user.pin_locked`, con el dispositivo). La contraseña no se bloquea (el bloqueo de contraseña sigue
  siendo por usuario y terminal, `auth.go:75`). **Desbloqueo** por un administrador (`users.manage`):
  `POST users/{id}/pin/unlock` (`user.pin_unlocked`).
- **Registro de movimientos**: `user.pin_set` (quién lo definió: el propio usuario o un administrador),
  `user.pin_removed`, `session.pin_login`, `session.pin_login_failed` (como `session.login_failed`, `auth.go:85`),
  `user.pin_locked`, `user.pin_unlocked`. Nunca el PIN ni su hash.
- **Quién lo define y dónde**: un administrador con `users.manage` en **Personal › Usuarios** (`PUT users/{id}/pin`
  con `{pin}` y `DELETE users/{id}/pin`; también nombre corto y color), y el propio usuario en su menú de cuenta del
  escritorio («Mi PIN»: `PUT auth/pin` con `{current_password, pin}`). Mismas validaciones en ambos.
- **Entrada con PIN**: `POST auth/pin-login` `{user_id, pin}`, despachada como `auth/login` **después de
  `authenticateDevice` y antes de la sesión** (`service.go:322`); exige `devices.kind = 'android'` (dispositivo
  emparejado; propuesta: **solo `android`**, decisión B-3), usuario activo, no soporte, con PIN y con
  `waiter_devices.use` efectivo (con las excepciones individuales). Responde como `login` (`session_token`) más
  `pin_version`. Emite la sesión con `issueSession(…, sameDevice)` (`auth.go:108`).
- **Lista para el dispositivo**: `GET auth/pin-users` con la credencial del dispositivo `android`, sin sesión:
  `[{id, short_name (o display_name), initial, color, pin_version, locked_until}]` de usuarios activos, no soporte, con
  PIN y con el permiso. Solo eso (nada de roles ni permisos).
- **Invalidar el verificador local**: el dispositivo guarda cada verificador con su `(user_id, pin_version)`; al
  reconectarse (y al abrir la app) pide la lista y borra los de usuarios que ya no aparecen (desactivado, sin PIN, sin
  permiso) o con otro `pin_version`. Cambiar o quitar el PIN y desactivar al usuario cierran además sus sesiones
  (`revoke_reason='pin_changed'`, como `password_changed`).
- **Sesiones**: encaja con «una sesión activa por usuario y por dispositivo» (9H): el PIN de Ana en la Tableta 1
  reemplaza solo la sesión anterior de Ana en esa tableta; las de otros meseros en la misma tableta siguen vigentes,
  lo que permite el **cambio rápido de usuario** sin cerrar las demás. Consecuencia: `devices/sessions-summary`
  (`devices.go:196`) contará **una sesión Android por mesero** en cada tableta compartida (las licencias todavía no
  limitan; anotarlo para cuando lo hagan). El bloqueo por inactividad no cierra la sesión en el servidor: solo vuelve
  a la pantalla de PIN en el dispositivo.
- **Pruebas**: unitarias (lista de triviales, bloqueo), de integración en `administration` (bloqueo a los 5,
  desbloqueo, solo `android`, sin permiso, auditoría, `pin_version`) y `TestContractV1` para las rutas nuevas.

### 3. Eventos en tiempo real con Android

Formato verificado en el emulador (t06) y en el código (`backend/internal/administration/events.go`,
`backend/internal/events/store.go`):

- **Autenticación**: las mismas cabeceras que el resto (`X-Device-ID`, `X-Device-Token`, `Authorization: Bearer`,
  `X-Client-Version`/`X-Client-Kind`), permiso `events.read`; un error inicial responde JSON con su estado HTTP.
- **Mensajes**: `id: <cursor>`, `event: changes`, `data: {"cursor", "reset", "events": [{sequence, id, version, kind,
scope, station_id, aggregate_id, created_at}]}`. Llega **un mensaje cada 750 ms aunque no haya cambios**
  (`events.go:111`): hace de latido. Cada lectura vuelve a validar sesión, dispositivo, permisos y estaciones; si algo
  falla llega `event: interrupted` con `{"error": "<código>", "status": <http>}` y **`id: 0`** (`events.go:130`).
- **Reanudación**: `?after=<cursor>` o la cabecera `Last-Event-ID` (`events.go:59`). Ventana de 5000 eventos y hasta
  200 por lectura (`store.go:62`, `:67`); un cursor fuera de la ventana (o mayor que el último) responde
  `reset: true` con el cursor vigente y sin eventos → reconstruir con `restaurant/state` y el catálogo.
- **Duración**: el servidor cierra cada flujo a los **30 s** (`events.go:113`); máximo **2 conexiones por dispositivo**
  (429 `EVENT_CONNECTION_LIMIT`, `events.go:82`).
- **Ámbitos útiles para el mesero**: `restaurant` (`accounts.read`; cuentas con su agregado), `catalog`
  (`catalog.read`). `kitchen` exige `kitchen.states.read` y estaciones visibles; por eso hace falta el evento nuevo de
  preparación (§2).

**okhttp-sse sirve** (5.5.1, mismo OkHttp): lee bien el formato, pero **no reconecta ni guarda `Last-Event-ID`**. La
app necesita su propio bucle (un solo flujo por proceso, compartido como `Flow`): reconectar al cerrarse (30 s) o al
fallar, con espera creciente; guardar como cursor solo el `id` de los mensajes `changes` (nunca el `0` de
`interrupted`); `readTimeout` de unos 10 s (detecta conexiones muertas, porque llega un mensaje cada 750 ms); con
`interrupted` `DEVICE_REVOKED` → borrado (§3.2), con `SESSION_*` → pantalla de PIN; al cambiar de mesero, reabrir con
la sesión del nuevo (otros permisos). Un lector propio no aporta nada.

**Volver de segundo plano sin depender de los eventos** (con lo que ya existe):

1. Cerrar el flujo SSE al pasar a segundo plano (batería y Doze); la cola la lleva WorkManager.
2. Al volver: `GET version` (compatibilidad) → abrir el flujo con el cursor guardado (o sin él) y tomar el `cursor`
   del primer mensaje → `GET restaurant/state` (permisos, terminal, mesas, cuentas con revisión, versión y época del
   catálogo) → si la época cambió, catálogo completo; si la versión es mayor, `catalog/sync?since&epoch` → desde ahí,
   cada evento solo indica qué volver a pedir (cuenta, mesas o catálogo). Con `reset: true`, repetir el paso de
   `restaurant/state`. Abrir el flujo antes de leer el estado evita perder lo ocurrido entre ambos (un evento repetido
   solo provoca una lectura de más). Con `event_cursor` en `restaurant/state` (opcional, §2) el orden se simplifica.
3. Al recuperar la red (callback de conectividad): lo mismo, y avisar a WorkManager.

### 4. Permisos del rol Mesero

- **Hoy el rol Mesero (`waiter`) no tiene ningún permiso en una instalación nueva**: el asistente da todos al
  administrador y solo unos pocos al supervisor, cajero y finanzas (`backend/internal/administration/setup.go:96–115`;
  roles en `permissions.go:90`); las migraciones conceden los permisos nuevos solo al administrador («a los demás
  roles se les asigna desde Personal», migración 0023). En el Go temporal: `waiter` 0 permisos (comprobado con
  `GET roles`). En la instalación real del propietario dependerá de lo que haya asignado (no se consultó).
- **Permisos que necesita el mesero en esta etapa** (todos existen salvo el del PIN):

| Acción                                | Permiso(s)                                                                                                                     |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| Entrar con PIN                        | `waiter_devices.use` (**nuevo**)                                                                                               |
| Ver mesas y áreas                     | `restaurant.tables.read`, `restaurant.areas.read`                                                                              |
| Ver cuentas y estados                 | `accounts.read`                                                                                                                |
| Abrir cuentas (creación diferida)     | `accounts.open`                                                                                                                |
| Capturar y modificar lo no enviado    | `orders.capture`                                                                                                               |
| Enviar a cocina / pedido sin conexión | `orders.confirm` (+ `orders.capture`, `accounts.read`, `catalog.read`, `catalog.prices.read`; `accounts.open` para mesa nueva) |
| Modificar unidades enviadas           | `orders.edit` (opcional según el restaurante)                                                                                  |
| Cancelar unidades                     | `orders.cancel` (desde la 9H sin supervisor)                                                                                   |
| Catálogo y precios de referencia      | `catalog.read`, `catalog.prices.read`                                                                                          |
| Tiempo real                           | `events.read`                                                                                                                  |
| «Agotado»                             | **no** (`catalog.sold_out.manage` fuera del mesero, como pide la especificación)                                               |

- **Alcanzan** (no hace falta ningún permiso nuevo salvo el del PIN). El servidor **no restringe mesas por mesero**:
  todo mesero con `restaurant.tables.read` ve todas las mesas y con `accounts.open` abre en cualquiera; `assigned_user_id`
  de la cuenta es informativo. En la app: mostrar todas; deshabilitar acciones según `user.permissions` de
  `restaurant/state`.
- Pendiente de decidir (B-2): si la migración 0025 concede al rol Mesero el conjunto anterior (cambiaría los permisos
  de un rol existente en la instalación real) o si se deja a Personal › Roles con instrucciones en
  `docs/ANDROID_INSTALAR.md`.

### 5. Otros puntos

#### 5.1 Impresora de precuentas por dispositivo (§8, para la A2)

El modelo de la 9G **no la admite por dispositivo**: es por **caja** (`app.register_receipt_printers`, migración
0023:436–446, D10) y un dispositivo usa la de su `register_id`. Asignar una caja a una tableta trae consigo el turno de
caja; para la A2 haría falta una asignación propia por dispositivo. Solo se anota.

#### 5.2 Cambios del servidor por fase e impacto en versiones

| Fase  | Cambio                                                                                                                                                                                | Contrato y pruebas                                                                                                                          |
| ----- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| B     | `min_client_versions` por tipo de cliente (Android 0.1.0)                                                                                                                             | `getVersion`, `clientversion_test.go`, `TestContractV1`                                                                                     |
| B     | Migración 0025 §B: PIN, intentos, nombre corto y color, `waiter_devices.use`, `devices.idle_lock_seconds`                                                                             | Integración en `administration`                                                                                                             |
| B     | `GET auth/pin-users`, `POST auth/pin-login`, `PUT auth/pin`, `PUT/DELETE users/{id}/pin`, `POST users/{id}/pin/unlock`; `idle_lock_seconds` en el PATCH de terminales y en `Terminal` | `openapi.json`, `docs/API_V1.md`, `TestContractV1` (las de administración pueden quedar fuera del contrato para clientes, como `devices/*`) |
| B     | Escritorio: PIN, nombre corto y color en Personal › Usuarios; «Mi PIN»; bloqueo por inactividad en Administración › Terminales; etiquetas de los movimientos nuevos                   | Vitest y, si aplica, Playwright                                                                                                             |
| B o E | Credencial de envío sin conexión (según la decisión B-1)                                                                                                                              | Integración + contrato                                                                                                                      |
| C     | **Ninguno** (emparejamiento `android` verificado de punta a punta)                                                                                                                    | —                                                                                                                                           |
| D     | **Ninguno**                                                                                                                                                                           | —                                                                                                                                           |
| E     | Documentar `GET restaurant/overview`, `cancel`, `edit-preview` y `edit` de cuentas y las propiedades de `Table` del mapa                                                              | `TestContractV1`                                                                                                                            |
| E     | Evento `restaurant.account.preparation_changed` (ámbito `restaurant`) al cambiar la preparación                                                                                       | Integración en `kitchen`                                                                                                                    |
| E     | `details.reason: "account_not_open"` en el rechazo de cuenta cerrada (sin cambiar estado ni código) y, opcional, `event_cursor` en `restaurant/state`                                 | `TestContractV1`                                                                                                                            |
| E     | Lo que resulte de la decisión E-1 (borrador del servidor y pedido sin conexión)                                                                                                       | Integración + contrato                                                                                                                      |

Todo es aditivo: el escritorio 0.10.x/0.11.x sigue funcionando, así que **`min_client_version` del escritorio puede
quedarse en 0.10.0**; el servidor sube a **0.12.0**, la app Android exige `min_server_version` **0.12.0** (necesita
las rutas del PIN) y declara su propia mínima `android` **0.1.0**. Esquema 25.

#### 5.3 Estructura propuesta del proyecto `android/`

```
android/
  settings.gradle.kts · gradle/libs.versions.toml · gradle/wrapper (9.9.1) · CLAUDE.md
  app/                 entrada: Application con AppContainer (inyección manual), MainActivity, navegación, WorkManager,
                       manifiesto, network_security_config solo en src/debug
  core/domain/         Kotlin/JVM puro: modelos (cola de pedidos y sus 5 estados, mesas, cuentas), reglas de la app
                       (nunca de negocio), casos de uso; sin Android → pruebas JVM rápidas
  core/data/           biblioteca Android: api/ (OkHttp, DTO con kotlinx.serialization, errores por code, SSE),
                       security/ (huella fijada, Keystore, verificador del PIN), db/ (Room, migraciones, esquemas),
                       sync/ (catálogo, recuperación, cola y trabajadores), repositorios
  core/ui/             tema AD-EL POS (azul #1F5FA8/#173D6E, estados 9D, claro/oscuro), componentes: indicador de
                       conexión y pendientes, botones de 48/56 dp, teclado de PIN, chips de estado de mesa, andamio de
                       uno o dos paneles — compartidos con la A2
  feature/access/      emparejamiento, búsqueda del servidor y entrada con PIN
  feature/waiter/      mesas, cuentas, captura, personalizador, pendientes y «Requiere tu revisión»
```

La A2 agrega `feature/takeaway` sobre `core/*` sin duplicar. Inyección: `AppContainer` creado en `Application`
(como en la prueba) y `ViewModelProvider.Factory` con `initializer {}`; sin Hilt. Si los módulos pesan en la Fase A,
empezar con `app` + `core/domain` + `core/data` + `core/ui` y separar las `feature/` después.

#### 5.4 Estrategia de pruebas para las fases B–F

- **JVM** (en cada fase, segundos): dominio y estados de la cola; correspondencia con la API con JSON de ejemplo;
  cliente con MockWebServer + okhttp-tls (huella fijada, 426, 401 `DEVICE_*`, reintentos con la misma
  `operation_id`); vector del código de emparejamiento (`166101`, `administration/pairing_test.go:29`; ya pasa en la
  prueba); lectura del TXT y elección por `server_id` + huella; verificador del PIN (con un Keystore falso).
- **Robolectric** (SDK 36; 26 solo si el código depende de la versión): Room (persistencia de la cola tras reabrir la
  base, datos por mesero) y WorkManager con `WorkManagerTestInitHelper` + `SynchronousExecutor` (ya pasa en la
  prueba). Migraciones de Room con `MigrationTestHelper` en el emulador.
- **Compose** (emulador): teléfono y tableta, rotación sin perder lo capturado (`StateRestorationTester` o
  `ActivityScenario.recreate`), tema oscuro, indicador de conexión y pendientes, bloqueo por inactividad que no ocurre
  con pendientes (reloj inyectable).
- **Extremo a extremo** (instrumentadas, Fase F, contra `iso.sh server-up`), con el patrón de la prueba:
  argumentos de instrumentación con la huella, el `server_id`, el token de aprovisionamiento y las dos URL; un
  ayudante «computadora principal» por la escucha local (`http://10.0.2.2:<ISO_GO_PORT>`) que configura, crea el
  catálogo, **aprueba el código**, cambia precios, marca agotados, cobra o cierra cuentas y **revoca**; la app bajo
  prueba por la escucha de red con la huella fijada en el emparejamiento. Reglas: un Go temporal nuevo por suite y
  por emulador (el asistente es de una sola vez y el límite de emparejamiento cuenta a todos los emuladores como un
  origen); corte de red con `cmd connectivity airplane-mode` en API ≥ 30 y desde el host con `adb root` en API 26;
  **respuesta perdida** con un interceptor de fallas solo de pruebas que descarta la respuesta después de que Go la
  procesó (el reintento con la misma clave debe dar `OFFLINE_ORDER_ALREADY_ACCEPTED` o la misma respuesta, sin
  duplicar); 426 con una versión de cliente por argumento; la mesa cobrada se cobra desde la principal.
- **Teléfono y tableta a la vez**: posible (dos emuladores con su propio Go temporal y su `ISO_TAG`), pero el disco y la
  memoria aconsejan alternarlos salvo en la regresión de la Fase G.

#### 5.5 Contradicción conocida

`CLAUDE.md` §16, punto 2, dice que Android es un «repositorio independiente»; la especificación (§0) pide la carpeta
`android/` en este repositorio. El coordinador ya la resolvió siguiendo la especificación (como en la 9H); `CLAUDE.md`
se actualizará en la Fase G.

### Decisiones para el coordinador

- **B-1 · Envío de la cola cuando la sesión del mesero ya no sirve.** Las sesiones duran 12 h
  (`administration/auth.go:136`) y la entrada sin conexión con el verificador local no crea sesión en el servidor; sin
  sesión válida WorkManager no puede enviar los pedidos de ese mesero con la app cerrada. Opciones: (a) una
  **credencial de envío por mesero y dispositivo** que emite `pin-login` (larga duración, solo vale para
  `submitOfflineOrder` de pedidos de ese mesero en ese dispositivo, se invalida al cambiar el PIN, desactivar al
  usuario o revocar el dispositivo; el servidor sigue validando los permisos vigentes al recibir) y que el verificador
  local desbloquea —recomendada—; (b) los pedidos esperan en «Por enviar» hasta que ese mesero vuelva a entrar con
  conexión; (c) aceptar sesiones vencidas para `submitOfflineOrder`. Afecta la seguridad: decisión del propietario.
- **B-2 · Permisos del rol Mesero**: ¿la migración 0025 concede a `waiter` el conjunto de §4 (y `waiter_devices.use`),
  o solo al administrador y se documenta cómo asignarlos en Personal › Roles?
- **B-3 · Entrada con PIN solo para dispositivos `android`** (propuesto) o también para terminales de escritorio
  emparejadas.
- **B-4 · Versión mínima por tipo de cliente** (`android` 0.1.0) frente a la alternativa de que la app envíe en
  `X-Client-Version` una versión «de producto» distinta de su `versionName` (no recomendada).
- **B-5 · Duración del bloqueo del PIN**: 5 minutos propuestos (la especificación dice «unos minutos»); opcional
  escalar (tras tres bloqueos en 24 h, hasta que lo desbloquee un administrador).
- **E-1 · Borrador del servidor y pedido sin conexión**: en línea, la especificación pide «como en el escritorio»
  (`openAccount` con el primer producto, `saveDraft`, `confirmDraft`); `submitOfflineOrder` con `account_id` **no
  toca el borrador** guardado en el servidor. Si la red se corta a mitad de una captura que ya se guardó como borrador,
  enviarla después como pedido sin conexión dejaría ese borrador duplicado en la cuenta. Opciones: vaciar el borrador
  del servidor al reconectar (con conflicto posible), o un campo nuevo en `submitOfflineOrder` que reemplace o
  limpie el borrador de esa cuenta en la misma operación (cambio de servidor en la Fase E), o enviar siempre por
  `submitOfflineOrder` y no usar el borrador del servidor salvo para la creación diferida.
- **Nivel objetivo**: `targetSdk` 36 (propuesto; lo exige Play hoy y evita el permiso de red local de Android 17) o 37
  desde ya (pediría `ACCESS_LOCAL_NETWORK` en tiempo de ejecución).
- **R8 en el APK de prueba**: reducir sí (0.97 MiB frente a 13 MiB), pero sin ofuscar (`-dontobfuscate`) para que los
  errores sean legibles; la regla de `AGENTS.md` sobre ofuscar se refiere al código fuente, pero conviene confirmarlo.
- **Identificador de la app**: propuesta `local.adel.pos.android` (en la línea de `local.adel.pos.dev` del escritorio y
  `local.adel.pos.server` del servicio).

### Pruebas ejecutadas

| Qué                                                         | Dónde                                                                                                           | Resultado                                                                                                                                                              |
| ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Versiones estables                                          | `maven-metadata.xml` (Google Maven, Maven Central, portal de plugins), `services.gradle.org`, páginas oficiales | Tabla §1.2                                                                                                                                                             |
| `minSdk` de cada biblioteca                                 | Manifiesto de cada AAR (caché de Gradle o descargado y borrado)                                                 | Todas ≤ 24                                                                                                                                                             |
| `assembleDebug`                                             | `spike-android`                                                                                                 | Correcto (1 min 45 s en frío); APK 13.1 MiB                                                                                                                            |
| `testDebugUnitTest`                                         | JVM + Robolectric (SDK 36)                                                                                      | **4/4**: `PairingTest` (vector `166101`), `PinnedTrustManagerTest` ×2 (acepta su huella sin nombres, rechaza otra), `RoomWorkManagerTest` (Room con KSP + WorkManager) |
| `assembleRelease` con R8                                    | `spike-android`                                                                                                 | Correcto, sin avisos; 0.97 MiB sin firmar                                                                                                                              |
| `connectedDebugAndroidTest` (9) contra el Go temporal       | Pixel_8 API 37 (5580)                                                                                           | **9/9** (con la versión final, t01–t02 repetidas: 2/2)                                                                                                                 |
| Lo mismo                                                    | ADEL_A1_Phone_API33 (5592)                                                                                      | **9/9**                                                                                                                                                                |
| Lo mismo                                                    | ADEL_A1_Tablet_API26 (5590)                                                                                     | **8/9** (t07: sin orden de corte de red utilizable como `shell` en Android 8)                                                                                          |
| t01–t04                                                     | Pixel_Tablet API 37 (5582)                                                                                      | **4/4**                                                                                                                                                                |
| Corte de red desde el host                                  | Pixel_8 (`airplane-mode`: vuelve en ≈ 3 s; `svc data`: ≈ 0.4 s) y API 26 (`adb root` + `svc wifi/data`)         | Funciona; en API 26 solo como root                                                                                                                                     |
| `X-Client-Version: 0.1.0` / `android` por la escucha de red | `curl` en la Mac al Go temporal                                                                                 | 426 `CLIENT_UPDATE_REQUIRED`                                                                                                                                           |
| Permisos de los roles en una base nueva                     | `GET roles` en el Go temporal                                                                                   | `waiter` sin permisos                                                                                                                                                  |

No verificado: la búsqueda NSD de un servidor real (solo dispositivo físico), WorkManager con la restricción de red en
una wifi sin internet (Fase E), Robolectric en SDK distintos de 36 y la pila en un dispositivo físico. Al terminar se
apagaron los emuladores (5580, 5582, 5590 y 5592), el Go temporal de `ISO_TAG=P0` y el demonio de Gradle; no quedó
ningún `dns-sd` de las pruebas (el único que corre es el anuncio `_adelpos._tcp` del servicio real, hijo del servidor
real, que no se tocó). Del proyecto de prueba quedan solo las fuentes (`$SP/spike-android/`, sin carpetas `build`: se
regeneran con `$SP/p0/gradle-spike.sh assembleDebug`); los guiones de la fase están en `$SP/p0/` (`gradle-spike.sh`,
`connected.sh`, `netcut.sh`, `netcut26.sh`, `minsdk.sh`, `meta.sh`) junto con los registros de cada corrida.

### Decisiones del coordinador al cerrar la Fase 0

Tomadas según la regla de autonomía de `CLAUDE.md` §0: ninguna cambia la especificación; todas aplican lo que pide. **El propietario confirmó A1-D1 (credencial de envío) y A1-D2 (permisos del rol Mesero) el 16/11/2026**, después de que el coordinador se las explicara.

- **A1-D1 · Credencial de envío por mesero y dispositivo (B-1).** §0-B pide captura sin conexión «sin límite de
  tiempo» con el servidor como único juez al recibir, y §7 que WorkManager envíe los pendientes «aunque la aplicación
  esté cerrada»; con sesiones de 12 h y entradas sin conexión que no crean sesión, eso exige una credencial propia. La
  emite `auth/pin-login` (y la renueva en cada entrada con conexión; la nueva sustituye a la anterior de ese mesero en
  ese dispositivo), se guarda cifrada con el Android Keystore y **solo sirve para entregar pedidos capturados sin
  conexión** (`submitOfflineOrder`) de ese mesero desde ese dispositivo, junto con la credencial del dispositivo. Al
  recibir, Go valida como siempre: dispositivo no revocado, usuario activo, `pin_version` vigente y **permisos
  vigentes**. Se invalida al cambiar o quitar el PIN, desactivar al usuario o revocar el dispositivo; no vence por
  tiempo. En la base solo su SHA-256, como las demás credenciales. No cuenta como sesión. «Requiere tu revisión»
  siempre necesita al mesero con su PIN y conexión. Entra en la revisión independiente de §3 (PIN y sesión).
- **A1-D2 · Permiso `waiter_devices.use` (B-2).** La migración 0025 lo da al administrador y al rol Mesero (`waiter`),
  sin efecto hasta que alguien tenga PIN. Los demás permisos del rol Mesero **no se tocan** en la instalación real (como
  en la 0023: los permisos de los roles se asignan en Personal › Roles); `docs/ANDROID_INSTALAR.md` dice cuáles darle
  (tabla de §4).
- **A1-D3 · PIN solo desde dispositivos `android` emparejados (B-3).** Las terminales de escritorio siguen con
  contraseña.
- **A1-D4 · Versión mínima por tipo de cliente (B-4).** `min_client_versions` en `config/product.json`
  (`desktop` 0.10.0, `android` 0.1.0), en la comprobación de `X-Client-Version` y en `GET version`;
  `min_client_version` se conserva con el valor del escritorio. El servidor sube a **0.12.0** desde la Fase B (esquema 25) y la app exige servidor ≥ 0.12.0.
- **A1-D5 · Bloqueo del PIN (B-5).** 5 fallos en 15 minutos, desde cualquier dispositivo, bloquean el PIN de ese
  usuario 5 minutos (`429 PIN_LOCKED`), con su registro; un administrador con `users.manage` lo desbloquea. Sin
  escalamiento.
- **A1-D6 · Borrador del servidor y pedido sin conexión (E-1).** En línea, el flujo del escritorio (cuenta que nace con
  el primer producto, borrador guardado en Go, «Enviar a cocina» con su clave). El borrador de Go es uno por cuenta y
  cada renglón guarda su id, autor y dispositivo (`operations/drafts.go`): si la red se corta a mitad de una captura,
  el pedido sin conexión lleva además los ids de los renglones del borrador que **ese dispositivo** había guardado, y
  Go los quita del borrador en la misma transacción en que acepta el pedido (campo opcional nuevo de
  `submitOfflineOrder`, compatible). Lo que otra terminal haya capturado en la misma cuenta no se toca. Detalle en la
  Fase E.
- **A1-D7 · Niveles de Android.** `minSdk` 26, `targetSdk` 36 (lo que exige hoy Google Play) y `compileSdk` 37. La
  capa de permisos de la Fase C queda preparada para el permiso de red local de Android 17 (`targetSdk` 37).
- **A1-D8 · APK de prueba.** Tipo `release` con R8 (reducción) **sin ofuscar** (`-dontobfuscate`: errores legibles;
  la regla de `AGENTS.md` sobre ofuscar es del código fuente), firmado con una llave de desarrollo fuera del
  repositorio. El texto claro hacia `10.0.2.2` existe solo en `src/debug` (pruebas).
- **A1-D9 · Identificador provisional** `local.adel.pos.android` (como `local.adel.pos.dev` del escritorio; cambiarlo
  después obliga a reinstalar y emparejar de nuevo).
- **A1-D10 · Proyecto y dependencias.** Módulos `app`, `core:domain` (Kotlin puro), `core:data`, `core:ui`,
  `feature:access` y `feature:waiter`; inyección manual (`AppContainer`); navigation-compose; versiones de §1.2 en
  `gradle/libs.versions.toml`, con bloqueo de dependencias de Gradle (`gradle.lockfile`) y el wrapper 9.9.1 con su
  SHA-256, como los demás lockfiles del repositorio.
- **A1-D11 · Evento de preparación.** `restaurant.account.preparation_changed` (ámbito `restaurant`) en la misma
  transacción del cambio de estado, para que el mesero vea los estados sin permisos de cocina (Fase E; también lo usará
  el aviso de platillos listos de la A2).

### Plan de las fases siguientes

Como máximo dos agentes a la vez; servidor y Android avanzan en paralelo cuando no dependen uno del otro.

| Tanda | Agente 1                                                                                                                              | Agente 2                                                                                                                           |
| ----- | ------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| 1     | **A** · proyecto `android/` base (§2)                                                                                                 | **B (servidor)** · PIN, credencial de envío, versión mínima por tipo, bloqueo por inactividad por dispositivo, escritorio y 0.12.0 |
| 2     | **C y B (dispositivo)** · búsqueda, emparejamiento, conexión con huella fijada, entrada con PIN, bloqueo por inactividad y revocación | **E (servidor)** · contrato de mesas, cancelar y modificar unidades, evento de preparación, cuenta cerrada, A1-D6                  |
| 3     | **D** · catálogo en el dispositivo                                                                                                    | Revisión independiente de §3 (PIN y sesión)                                                                                        |
| 4     | **E (dispositivo)** · modo Mesero y cola sin conexión                                                                                 | Correcciones de la revisión de §3                                                                                                  |
| 5     | **F** · extremo a extremo en los emuladores                                                                                           | Revisión independiente de §6–§7 (cola sin conexión)                                                                                |
| 6     | **G** · regresión completa, documentación, APK, `.pkg`, fusión y push                                                                 | —                                                                                                                                  |

---

## Fase A · Proyecto base

Rama `etapa-a1-a` (desde `etapa-a1-android`, `6b5109e`), un agente, 16 de noviembre de 2026. Árbol `$SP/wt/A`,
`ISO_TAG=A`, Go temporal propio en 8193/8453 (solo para una prueba del cliente; apagado al terminar). **Sin cambios en
el servidor, el escritorio, `config/` ni `docs/API_V1.md`**; `docs/etapas/*.md` con el mismo md5 antes y después.
Ninguna prueba usó el servicio real de la Mac.

Commits:

| Commit    | Tema                                                                                                                     |
| --------- | ------------------------------------------------------------------------------------------------------------------------ |
| `0a04de4` | Proyecto Gradle de `android/`: wrapper 9.9.1, catálogo, módulos, tipos de compilación, bloqueos y verificación SHA-256   |
| `4e368d0` | `core:domain`: importes como texto, monitor de red, errores de la API, cifrado (interfaz), estados de mesa               |
| `a66f3d4` | `core:data`: cliente con huella fijada, errores de Go, `GET version`, Room v1, monitor de conexión, Keystore             |
| `d78a6a5` | `core:ui`: tema claro/oscuro, íconos propios, estado de mesa, botones, barra con indicador, uno o dos paneles            |
| `f738f7f` | `app`, `feature:access`, `feature:waiter`: inyección manual, navegación, Entrar y Mesas de muestra, rotación sin pérdida |
| `f33aeb7` | `scripts/dev.sh` (`android-*`), `android/CLAUDE.md` y `docs/DEPENDENCIES.md` («Etapa A1 · Android»)                      |

### Qué se hizo

**Proyecto** (`android/`, estructura A1-D10): `app`, `core:domain` (Kotlin/JVM puro), `core:data`, `core:ui`,
`feature:access`, `feature:waiter`; espacios de nombres `local.adel.pos.android[.core.data|…]`. Versiones de §1.2 del
avance, exactas, en `android/gradle/libs.versions.toml` (WorkManager y `okhttp-sse` declarados pero sin usar hasta las
fases D/E). Gradle 9.9.1: el `gradle-wrapper.jar` se regeneró con el propio 9.9.1 y es **idéntico al publicado**
(SHA-256 `621008af…cdaf8`, el de `gradle-9.9.1-wrapper.jar.sha256`; el del proyecto de prueba de la Fase 0 era de otra
versión) y `gradle-wrapper.properties` lleva `distributionSha256Sum`. `minSdk` 26, `targetSdk` 36, `compileSdk` 37,
`applicationId` `local.adel.pos.android`, `versionName` 0.1.0 y `versionCode` 1 (`android/app/build.gradle.kts:62-65`).
**La versión se escribe en un solo lugar**, `android/gradle.properties` (`adelpos.versionName`/`versionCode`); la usan
el APK y `X-Client-Version` (`BuildConfig.VERSION_NAME` → `ClientIdentity` en `AppContainer`).

**Bloqueo y verificación de dependencias**: `dependencyLocking { lockAllConfigurations() }` en todos los proyectos
(`android/build.gradle.kts:33`) y en el classpath de los complementos: un `gradle.lockfile` por proyecto (todas las
configuraciones, también Lint, KSP, ktfmt y la plataforma de pruebas instrumentadas), `buildscript-gradle.lockfile` y
`settings-gradle.lockfile`. **La verificación de Gradle funciona sin dolores**: `android/gradle/verification-metadata.xml`
(555 componentes con SHA-256, metadatos verificados, firmas no) se generó con la tarea `resolveAllDependencies`
(`android/build.gradle.kts:38`, resuelve grafo y archivos de todas las configuraciones) más la compilación, las pruebas,
Lint, ktfmt y `connectedDebugAndroidTest` en un emulador; después pasó todo con la verificación activa y
`--rerun-tasks`, y desde una exportación limpia del commit (`git archive`). Detalles: `aapt2` es por sistema operativo
(solo el de macOS); fuentes y javadoc de Android Studio quedan como confiables (no entran a la compilación); agregar una
dependencia obliga a regenerar ambos con un emulador encendido (orden exacta en `android/CLAUDE.md`, «Dependencias»).

**Tipos de compilación** (A1-D8): `debug` con `app/src/debug/AndroidManifest.xml` y `network_security_config.xml`
(base sin texto claro, `domain-config` solo `10.0.2.2`); `release` con `isMinifyEnabled`, `isShrinkResources` y
`-dontobfuscate` (`app/proguard-rules.pro`), firmado **solo** si la propiedad `adelpos.signingProperties` o la variable
`ADELPOS_SIGNING_PROPERTIES` apuntan a un archivo de propiedades fuera del repositorio
(`app/build.gradle.kts:25`, `loadReleaseSigning`: rechaza archivo o llave dentro del repositorio y faltantes). Manifiesto
con `allowBackup="false"`, `dataExtractionRules` (Android 12+) y `fullBackupContent` (8–11) que excluyen todos los
dominios, `usesCleartextTraffic="false"`, `enableOnBackInvokedCallback="true"` e ícono reducido con `sips` desde
`desktop/src-tauri/icons/icon.png` (mipmaps clásicos de 48 a 192 px).

**Inyección manual**: `AdelPosApplication` crea `AppContainer` (identidad del cliente, monitor de conexión, contador de
pendientes, Room, preferencias y el sellador del Keystore, perezosos); `ShellViewModel` y `TablesViewModel` con
`viewModelFactory { initializer { … } }`. Sin Hilt.

**`core:domain`**: `Money` (cadena decimal validada; nunca número) y `MoneyFormat`, puerto exacto de
`desktop/src/services/money.ts` (formato, con signo, deducción y denominación), probado con **la misma tabla de casos**
que Go y Vitest (`backend/internal/moneytext/testdata/cases.json`, leída por la prueba); `DefaultNetworkTracker` (red
predeterminada, bloqueos, cambio de red; **no exige red validada**); `ConnectionSummary`/`LinkState`;
`PendingOrdersSource`; `AppError`/`AppResult`; `SecretSealer` + `SecretUnavailableException` (KEY_LOST, DAMAGED,
KEYSTORE_FAILED); `TableState` con los nombres de la API; `TablesView`.

**`core:data`**:

- `PinnedHttpClient` (`core/data/.../api/PinnedHttpClient.kt:36-40`): `PinnedTrustManager` (SHA-256 de la SPKI,
  `MessageDigest.isEqual` en tiempo constante, `PinMismatchException`), `hostnameVerifier` sustituido por la huella con
  el comentario del porqué, **solo `ConnectionSpec.RESTRICTED_TLS`** (TLS 1.3/1.2), un `SSLContext` por cliente, sin
  reintentos automáticos, `ClientHeadersInterceptor` (`X-Client-Version`, `X-Client-Kind: android`).
- JSON: `ApiJson` (ignora campos desconocidos), `MoneySerializer` (solo cadenas; un número JSON se rechaza).
- Errores (`ApiProblem.kt:44-116`): la forma de Go (`backend/internal/httpapi/router.go:40`, `fail`/`failWithDetails`)
  → `ApiProblem` (con `details`) → `AppError`: 426 y `CLIENT_VERSION_INVALID`/`CLIENT_KIND_INVALID` → `UpdateRequired`
  (con `min_client_version` y `server_version`); `DEVICE_REVOKED`; otros `DEVICE_*`; `SESSION_*`;
  `PERMISSION_DENIED`; 429 → `TryLater`; 5xx → `ServerProblem`; cuerpo sin la forma de Go → `InvalidResponse`; lo
  demás → `Rejected` con su `code`; sin respuesta → `Unreachable`; huella distinta → `ServerIdentityChanged`.
- `ServerApi.getVersion()` (`VersionInfo` del contrato; `min_client_versions` opcional para el servidor 0.12.0, A1-D4).
- Room `AppDatabase` v1 con `device_preferences` (clave/valor) y `DevicePreferencesRepository` (vista de mesas, lista
  por omisión); esquema exportado en `core/data/schemas/` (complemento `androidx.room`), `MIGRATIONS` vacía y la regla
  de nunca destruir la base.
- `ConnectionMonitor` (`registerDefaultNetworkCallback`, estado inicial síncrono con `current()`), `NoPendingOrders`.
- `KeystoreSecretSealer`: AES-256-GCM, llave del Android Keystore no exportable, StrongBox si el dispositivo lo anuncia
  (función `@RequiresApi(28)` con respaldo sin él), sin autenticación de usuario ni exigencia de desbloqueo (dispositivos
  compartidos y cola con la pantalla bloqueada), vector del Keystore, propósito como dato autenticado; formato versión ·
  largo del vector · vector · cifrado; `destroyKey()` para la revocación.

**`core:ui`**: `AdelPalette.kt`, **único archivo con colores**, con los tokens de `desktop/src/styles.css` (`:root` y
`:root[data-theme='dark']`, resueltos: `--table-free-bg` = panel de cada tema, etc.); `AdelTheme` llena **todos** los
roles de Material 3 (también `surfaceContainer*`) para que nada use la paleta de muestra; `AdelTheme.colors` para lo
propio (mesas, ámbar, uva, indicador). Tipografía algo mayor para tableta (cuerpo 16–18 sp). `AdelIcons`: palomita,
reloj, recibo, efectivo, candado, enviar y volver, con los trazos de `Icon.tsx` (círculos y rectángulos convertidos a
trazos) como `ImageVector`. `TableStateBadge` (color, texto, ícono). `PrimaryButton` 56 dp y `SecondaryButton` 48 dp.
`AdelTopBar` + `ConnectionIndicator` («En línea» punto azul, «Sin conexión» punto y texto rojos, «N pedidos por enviar»
en ámbar con plural; región viva para lectores de pantalla). `AdaptivePanes` + `paneLayoutFor`/`currentPaneLayout()`
con `currentWindowAdaptiveInfoV2()`. Textos en `strings.xml` con `tools:locale="es"`.

**`app` y `feature:*`**: `MainActivity` con `enableEdgeToEdge()` y `WindowInsets.safeDrawing` en el `Scaffold`;
navigation-compose con rutas con tipo («Entrar» → «Mesas»); barra superior común con el indicador en todas las
pantallas; Mesas con «Buscar mesa» y el panel de cuenta abierto en `SavedStateHandle` (`TablesViewModel.kt:20-33`);
en el teléfono la cuenta va «a un toque» («Ver cuenta») y vuelve con «Volver a Mesas» o Atrás (`BackHandler`,
`AdaptivePanes.kt:69`).

**Formato y comprobación**: ktfmt con `kotlinLangStyle()` (complemento `com.ncorti.ktfmt.gradle` 0.28.0 →
`org.jetbrains.kotlinx:ktfmt` 0.65, bloqueado) en fuentes y `.kts` de todos los proyectos; Android Lint con
`warningsAsErrors` en los cinco módulos Android. `scripts/dev.sh` (`:17`, `:123-142`): `android-format`,
`android-format-check`, `android-build` (con el tamaño de cada APK), `android-test` (JVM, Robolectric y Lint) y
`android-connected` (`ANDROID_SERIAL`), con el JBR de Android Studio y `~/Library/Android/sdk` por omisión; **fuera** de
`format-check` y `test`. `.gitignore` (`android/.gradle`, `android/**/build`, `android/local.properties`,
`android/.kotlin`, `*.jks`, `*.keystore`, `*.p12`, propiedades de firma), `.prettierignore` (compilaciones y esquemas de
Room), `.gitattributes` (`*.kt`, `*.kts`, `*.xml`, `*.properties`, `*.lockfile`, `*.pro`, `gradlew` con LF; `*.jar`,
`*.apk`, `*.webp` binarios) y `.editorconfig` (Kotlin con 4 espacios y 100 columnas, XML de Android con 4).

### Decisiones tomadas en la fase

- **Regla de dos paneles: ancho ≥ 600 dp y alto ≥ 480 dp** (`core/ui/.../layout/PaneLayout.kt:22`). El encargo dice «en
  ancho expandido» (≥ 840 dp), pero la Pixel Tablet vertical mide 800 dp de ancho y §0-B pide catálogo y cuenta lado a
  lado en la tableta; con ≥ 600 dp la tableta tiene dos paneles en ambas orientaciones y el teléfono horizontal (ancho
  expandido, alto compacto ≈ 411 dp) se queda con uno. Es una constante: el coordinador puede subirla a 840.
- **Andamio propio** (`AdaptivePanes`) en lugar de `ListDetailPaneScaffold`: regla propia (la de Material da un panel
  en ancho medio), estado guardable por el `ViewModel` y pruebas deterministas con la disposición forzada;
  `material3-adaptive` solo aporta `currentWindowAdaptiveInfoV2()`.
- **Solo `RESTRICTED_TLS`** (el proyecto de prueba agregaba `MODERN_TLS` de respaldo): verificado contra el Go temporal
  en Android 8 y 13 (abajo).
- **Huella distinta detectada en causas y suprimidas**: con `localhost` OkHttp prueba `::1` y `127.0.0.1` y lanza la
  primera falla (`ConnectException`) con el `SSLHandshakeException` de la huella como **suprimida** (comprobado);
  `ApiErrors.fromException` busca `PinMismatchException` en ambas, así nunca se confunde con «sin red».
- **`CLIENT_VERSION_INVALID` y `CLIENT_KIND_INVALID` → `UpdateRequired`**: el remedio es el mismo («Actualiza esta
  terminal»); el `code` se conserva.
- **Room sin cifrado de base completa**: no se agregó SQLCipher (no está en §1.2). Las credenciales y el verificador del
  PIN se guardarán cifrados campo a campo con el Keystore (fases B/C); la base queda en el almacenamiento privado de la
  aplicación, fuera de las copias. Ver «Para el coordinador».
- **Espresso 3.7.0 fijado** en `core:ui` y `app` (pruebas): Compose UI test trae Espresso 3.5.0, que en Android 17 falla
  en todas las pruebas de Compose con `NoSuchMethodException: InputManager.getInstance` (visto en Pixel_8).
- **API v2 de las reglas de prueba de Compose** (`androidx.compose.ui.test.junit4.v2`): las de v1 están en desuso en el
  BOM 2026.09.00.
- **Lint**: `warningsAsErrors`, con `GradleDependency`, `AndroidGradlePluginVersion`, `NewerVersionAvailable` y
  `OldTargetApi` desactivadas (versiones del avance y A1-D7) y `ObsoleteLintCustomCheck` informativa (el `lint.jar` de
  lifecycle 2.11.0 usa una API que el lint de AGP 9.5.1 no tiene: se omiten `UnsafeLifecycleWhenUsage` y
  `RepeatOnLifecycleWrongUsage`). `CustomX509TrustManager` se suprime en `PinnedTrustManager` con la justificación
  (rechaza todo salvo la llave fijada).
- **Rotación real en las pruebas** (`UiAutomation.setRotation`, la actividad se recrea), no `recreate()`: vale también
  en tabletas (Android 16 ignora la orientación pedida por la aplicación en pantallas ≥ 600 dp).
- **Tema** según el sistema (`isSystemInDarkTheme()`); el escritorio guarda la preferencia por usuario en el servidor.
- **Comentarios de línea de 100 columnas como máximo**: ktfmt corta los `//` más largos en renglones sueltos; se
  reacomodaron todos.

### Pruebas ejecutadas (aisladas)

| Qué                                                                                                     | Dónde                                                         | Resultado                                                                                                                                                                                                                                                                                            |
| ------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| JVM (`:core:domain:test`, `testDebugUnitTest`)                                                          | Mac, JBR 25                                                   | **41/41**: `MoneyFormatTest` 7 (tabla compartida: 34 de formato, 10 con signo, 8 de denominación), `TableStateTest` 2, `DefaultNetworkTrackerTest` 5, `ServerApiTest` 11, `MoneySerializerTest` 4, `PaneLayoutTest` 4, `ColorsOnlyInPaletteTest` 2, `PaletteContrastTest` 2, `TablesViewModelTest` 2 |
| Robolectric SDK 36                                                                                      | `AppDatabaseTest`                                             | **2/2** (incluido en las 41)                                                                                                                                                                                                                                                                         |
| Android Lint (`lintDebug`, 5 módulos)                                                                   | Mac                                                           | Sin problemas (solo el aviso informativo de lifecycle)                                                                                                                                                                                                                                               |
| ktfmt (`ktfmtCheck`) y `./scripts/dev.sh android-format-check`                                          | Mac                                                           | Correcto                                                                                                                                                                                                                                                                                             |
| Instrumentadas (`connectedDebugAndroidTest`, 21: core:data 9, core:ui 6, app 6)                         | `Pixel_8` API 37 (5580), con `dev.sh android-connected`       | **19/19** + 2 omitidas (las del Go temporal, sin argumentos)                                                                                                                                                                                                                                         |
| Lo mismo                                                                                                | `Pixel_Tablet` API 37 (5582)                                  | **19/19** + 2 omitidas; dos paneles en ambas orientaciones                                                                                                                                                                                                                                           |
| Lo mismo, con la verificación de dependencias activa y `--rerun-tasks`                                  | `ADEL_A1_Phone_API33` (5592)                                  | **19/19** + 2 omitidas                                                                                                                                                                                                                                                                               |
| Lo mismo                                                                                                | `ADEL_A1_Tablet_API26` (5590), con `dev.sh android-connected` | **18/18** + 3 omitidas (las 2 del Go y el modo avión, que no existe como `shell` en Android 8)                                                                                                                                                                                                       |
| `ServerApiOnDeviceTest` contra el Go temporal A (`https://10.0.2.2:8453`, huella del `server-cert.pem`) | API 26 y API 33                                               | **2/2** en cada uno: `getVersion` 200 con solo `RESTRICTED_TLS` (el registro de Go muestra cada petición) y otra huella → `ServerIdentityChanged`                                                                                                                                                    |
| Indicador con corte de red real en Android 8 (`adb root` + `svc wifi/data disable` desde la Mac)        | API 26                                                        | «Sin conexión» con `Active default network: none` y «En línea» al volver (capturas en `$SP/tmp-A/shots/`)                                                                                                                                                                                            |
| Capturas de Entrar y Mesas, tema claro y oscuro (`cmd uimode night`)                                    | Pixel_8 y Pixel_Tablet                                        | Revisadas: un panel con «Ver cuenta» en el teléfono, dos paneles en la tableta, estados con color, texto e ícono, de borde a borde                                                                                                                                                                   |
| `assembleDebug` / `assembleRelease` (`dev.sh android-build`)                                            | Mac                                                           | Depuración **13 362 399 bytes (12.7 MiB)**; release sin firmar **1 504 318 bytes (1.43 MiB)**; APK de pruebas 1 093 152 bytes                                                                                                                                                                        |
| Release firmado con una llave **desechable** del scratchpad (borrada después)                           | Mac; instalado en API 26 y API 33                             | 1 512 514 bytes; abre, navega a Mesas (R8 no rompe la navegación con rutas serializadas); un archivo de firma dentro del repositorio se rechaza                                                                                                                                                      |
| Exportación limpia del commit (`git archive`) con verificación activa                                   | Mac                                                           | Compila, pruebas JVM, Lint y ktfmt correctos                                                                                                                                                                                                                                                         |
| `iso.sh format`, `fmt-check` (Prettier, gofmt, cargo fmt, datos del sistema anterior) y `sync-check`    | Árbol A                                                       | Correctos; md5 de `docs/etapas/*.md` igual antes y después                                                                                                                                                                                                                                           |

### No verificado

- Dispositivo físico; foldables y multiventana.
- Restauración tras la muerte del proceso **en el dispositivo** (solo la prueba JVM de `SavedStateHandle`; la rotación
  real sí se probó).
- La versión de TLS negociada con el cliente final (no se mide en la prueba; la Fase 0 midió TLS 1.2 en Android 8 y 1.3
  desde el 10 con la misma pila).
- El release en Android 17 (solo en 8 y 13) y el release firmado con la llave de desarrollo real (Fase G).
- La verificación de dependencias con la caché de Gradle vacía (se generó y comprobó con la caché llena y desde una
  exportación limpia).
- Robolectric en SDK distintos de 36; tema oscuro del **sistema** en Android 8 (no hay modo oscuro global: la prueba
  fuerza el parámetro del tema).
- Las verificaciones de lint de lifecycle 2.11.0 (las omite el lint de AGP 9.5.1).

### Para el coordinador

1. **Dos paneles desde 600 dp** (y alto ≥ 480 dp) en lugar de «ancho expandido» (840 dp): ver Decisiones; cambiarlo es
   una constante en `PaneLayout.kt`.
2. **Cifrado de la base local**: el encargo menciona «la base y las credenciales van cifradas con una llave que no
   viaja». Se cifran las credenciales (Keystore); la base de Room **no** (haría falta SQLCipher, que no está en las
   versiones de la Fase 0). Si se quiere la base cifrada, es una dependencia nueva (decidir antes de la Fase E, que
   guarda pedidos).
3. **AGP 9.5.1: una instalación fallida no hace fallar `connectedDebugAndroidTest`** (registra
   `INSTALL_FAILED_UPDATE_INCOMPATIBLE` y termina con BUILD SUCCESSFUL sin correr ese módulo; lo vi al dejar instalado un
   release con otra firma). Las fases siguientes deben contar los resultados en
   `*/build/outputs/androidTest-results/connected/` y desinstalar antes; anotado en `android/CLAUDE.md`.
4. Los resultados de las pruebas instrumentadas de cada módulo se borran en cada corrida: para conservar los de varios
   emuladores, copiarlos después de cada uno.
5. El complemento de ktfmt 0.28.0 usa `Configuration.setVisible` (en desuso, se quita en Gradle 11): aviso en cada
   compilación, inofensivo hasta entonces.
6. Tema por usuario como en el escritorio (preferencia en el servidor) o según el sistema: hoy según el sistema.

### Para CLAUDE.md y AGENTS.md (no los edité)

- `CLAUDE.md` §16, punto 2: la aplicación Android vive en `android/` de este repositorio (ya decidido en la Fase 0);
  §4 (estructura) con `android/` y un enlace a `android/CLAUDE.md`; §5 (comandos) con `./scripts/dev.sh android-format`,
  `android-format-check`, `android-build`, `android-test` y `android-connected` (necesitan Android Studio; fuera de
  `format-check` y `test`); §6 con la matriz de emuladores y la regla de no usar nunca el servicio real.
- `AGENTS.md`, reglas de la etapa A1: Kotlin con ktfmt (`kotlinLangStyle`, 4 espacios, 100 columnas, comentarios `//` de
  100 columnas como máximo) y Android Lint sin advertencias; la aplicación nunca calcula reglas de negocio; colores solo
  en `android/core/ui/.../theme/AdelPalette.kt`; credenciales solo con el Keystore; nada de llaves de firma ni
  `local.properties` en el repositorio; toda dependencia nueva actualiza `libs.versions.toml`, los `gradle.lockfile` y
  `gradle/verification-metadata.xml` (con un emulador encendido) y `docs/DEPENDENCIES.md`; cada cambio de esquema de
  Room sube la versión con su migración y su prueba con `MigrationTestHelper`.
- `README.md` y `DEVELOPMENT_MACOS.md`: los comandos `android-*` y que requieren Android Studio (JBR) y el SDK en
  `~/Library/Android/sdk`.

### Estado al terminar

Emuladores 5580, 5582, 5590 y 5592 apagados; Go temporal de `ISO_TAG=A` detenido y su base borrada (`server-down`);
demonio de Gradle detenido; árbol sin cambios por confirmar. La llave de firma desechable de la prueba del release se
borró. En `$SP/tmp-A/` quedan los registros (`connected-*.log`, `devsh-connected-*.log`, `write-meta.log`,
`verify-run.log`), las capturas (`shots/`), los XML de resultados (`results/`) y los guiones de apoyo.

**Decisiones del coordinador al cerrar la Fase A** (rama `etapa-a1-a` fusionada en `etapa-a1-android`, `90e5426`;
`fmt-check` y `sync-check` aprobados sobre la fusión):

- **A1-D12 · Dos paneles desde 600 dp de ancho y 480 dp de alto** (lo propuesto por la fase): la tableta en vertical
  también muestra catálogo y cuenta lado a lado (§0-B) y el teléfono horizontal conserva un panel.
- **A1-D13 · La base local de Room no se cifra entera.** La especificación pide cifrar con el Android Keystore la
  credencial del dispositivo (§4) y el verificador del PIN (§3.2); eso, la credencial de envío (A1-D1) y cualquier otro
  secreto van cifrados con el Keystore. El resto de la base (catálogo, mesas y pedidos) queda en el almacenamiento
  privado de la app, sin copias de seguridad (`allowBackup="false"` y reglas de extracción que excluyen todo) y con el
  cifrado del propio dispositivo. Cifrar la base completa exigiría SQLCipher (biblioteca nativa nueva, con costo en las
  tabletas económicas) sin que lo pida la especificación.
- **A1-D14 · Tema claro u oscuro según el sistema** del dispositivo (compartido por varios meseros); el escritorio lo
  guarda por usuario, pero aquí no hay una preferencia por mesero.
- **Pruebas instrumentadas**: AGP 9.5.1 puede terminar con «BUILD SUCCESSFUL» sin correr las pruebas de un módulo si
  falla la instalación; cada fase cuenta los XML de resultados (anotado en `android/CLAUDE.md`).

---

## Fase B · Servidor y escritorio

Un agente, en su propio árbol de trabajo (`etapa-a1-bs`, desde `etapa-a1-android` en `6b5109e`), en paralelo con la
Fase A (proyecto `android/`, otro agente). 16/11/2026. **Versión 0.12.0 y esquema 25** (`config/product.json` y
`node scripts/sync-metadata.mjs`, más la versión del paquete en `desktop/package-lock.json` y `Cargo.lock`, como en la
0.11.0). Migración `0025_android_waiters.sql`, una sola para la etapa, con su **sección B**. Pruebas en copias aisladas
(`ISO_TAG=B`): clúster temporal 55499 con una base `adel_pos_test_b_*` por paquete, Go temporal de Playwright en 8186 y
vista previa en 1424. **No se tocó la instalación real** ni `android/`, `scripts/dev.sh`, `.gitignore`,
`docs/DEPENDENCIES.md`, `CLAUDE.md` ni `AGENTS.md`; `docs/etapas/*.md` con el mismo md5 antes y después.

Commits en `etapa-a1-bs`:

- `ed603ae` — versión 0.12.0 (esquema 25) y versión mínima por tipo de cliente (A1-D4).
- `56bd9ae` — migración 0025, sección B, con su prueba de subida 24 → 25.
- `3aacd0c` — entrada con PIN, credencial de envío, administración del PIN, «Mi PIN» y bloqueo por inactividad en Go;
  contrato v1, `docs/API_V1.md` y `API_CONTRACTS.md`.
- `b11c01f` — escritorio: Personal › Usuarios, «Mi PIN», Terminales y Registro de movimientos.
- `c9c2c14` — Playwright `workflow-stagea1-pin.spec.ts`.
- `0b0633e` — el canal de eventos también rechaza la credencial de envío.

### Qué cambió

**Versiones (A1-D4).** `config/product.json` lleva `min_client_versions` (`desktop` 0.10.0, `android` 0.1.0) y conserva
`min_client_version` (0.10.0, la del escritorio) y `min_server_version` (0.10.0). `product.Metadata.MinimumFor(kind)`
(`backend/internal/product/product.go`) da la mínima de un tipo; sin tipo o con uno sin mínima propia, la general.
`checkClient` (`backend/internal/httpapi/clientversion.go`) compara `X-Client-Version` con la mínima del tipo de
`X-Client-Kind` y el 426 informa en `details.min_client_version` la de ese tipo. `GET version` agrega
`min_client_versions` (todos los tipos de `ClientKinds`) y las capacidades **`client-versions-by-kind`** y
**`waiter-pin-v1`** (`router.go`). Ningún cliente 0.10.x/0.11.x deja de funcionar: el escritorio sigue con su mínima
0.10.0 y la omisión del tipo nunca rebaja la exigencia (se compara con la general).

**Migración `backend/migrations/0025_android_waiters.sql`, sección B** (el final del archivo dice dónde van las
secciones de las fases siguientes):

- `app.users`: `pin_hash` (Argon2id, el formato de las contraseñas), `pin_set_at`, `pin_version` (0; sube al definir,
  cambiar o quitar), `short_name` (1–20, sin espacios a los lados) y `badge_color` (`^#[0-9a-f]{6}$`; Go solo acepta los
  de la paleta); `users_pin_fields_check` (hash y fecha juntos).
- `app.pin_attempts` (por usuario): `failures` (0–5), `window_start`, `locked_until`, `last_device_id`.
- `app.offline_order_credentials`: usuario, dispositivo, `secret_hash` (SHA-256, único), `pin_version` con que se
  emitió, `issued_at`, `last_used_at`, `revoked_at` y `revoke_reason` (`replaced`, `pin_changed`, `pin_removed`,
  `user_disabled`, `device_revoked`); índice único parcial **una vigente por usuario y dispositivo**.
- `app.devices.idle_lock_seconds` (120; CHECK 30–3600).
- Permiso **`waiter_devices.use`** («Entrar con PIN en teléfonos y tabletas de meseros», igual en
  `administration.Permissions`) para `administrator` y `waiter` (A1-D2). En una instalación nueva los roles nacen en el
  asistente después de las migraciones: `setup.go` hace la misma concesión al rol Mesero. Ningún otro permiso cambia.

**Entrada con PIN** (`backend/internal/administration/pin_login.go`; despachadas en `service.go` después de
`authenticateDevice` y antes de la sesión):

- `GET auth/pin-users` (`pinUsers`, `pin_login.go:67`): solo dispositivo `android` (si no, `403
PIN_DEVICE_NOT_ALLOWED`); `{device: {id, name, idle_lock_seconds}, users: [{id, name, initial, color, pin_version,
locked_until}]}` de usuarios activos, no de soporte, con PIN y con `waiter_devices.use` efectivo (rol más excepciones
  individuales, la misma regla de `effectivePermissions`, en SQL: `waiterEligibility`).
- `POST auth/pin-login` (`pinLogin`, `pin_login.go:109`) `{user_id, pin}` → `{session_token, pin_version,
offline_credential}`. Orden: solo `android`; forma del PIN (400, no cuenta); bloqueo vigente → `429 PIN_LOCKED` con
  `details.locked_until` sin verificar; siempre se verifica un hash (el ficticio `pinDummyHash` si el usuario no existe,
  está inactivo, es de soporte o no tiene PIN); fallo → `401 PIN_INVALID`, cuenta para el usuario si existe y el quinto
  en 15 minutos (desde cualquier dispositivo) bloquea 5 minutos y responde ya `429 PIN_LOCKED` (`recordPINFailure`,
  `pin_login.go:206`, registra `user.pin_locked` una sola vez con el dispositivo y reinicia la cuenta); acierto → borra
  los intentos; sin `waiter_devices.use` → `403 PERMISSION_DENIED`; sesión con `issueSession(…, "replaced",
sameDevice)` (reemplaza solo la de ese usuario en ese dispositivo: el cambio rápido de mesero no cierra las de los
  demás) y credencial de envío nueva (`issueOfflineCredential`, revoca la anterior de ese usuario y dispositivo).
- **Credencial de envío** (A1-D1): cabecera **`X-Offline-Credential`** junto a `X-Device-ID`/`X-Device-Token`, sin
  `Authorization`. `dispatch` la acepta **solo** en `POST restaurant/offline-orders` (en cualquier otra ruta o método,
  incluido el canal de eventos, `401 OFFLINE_CREDENTIAL_NOT_ALLOWED`; con sesión y credencial juntas, 400).
  `authenticateOfflineCredential` (`pin_login.go:275`) revisa en cada petición: misma credencial y mismo dispositivo,
  no revocada, usuario activo y no de soporte, PIN vigente con la **misma `pin_version`**, dispositivo `android` y
  `waiter_devices.use`; si algo falla, `401 OFFLINE_CREDENTIAL_INVALID`. Devuelve al mesero con sus **permisos
  vigentes** y la operación pasa por `runProtected` y `operations.Handle` como con una sesión (sin `orders.confirm`,
  403). Registra `last_used_at`. No es una sesión: no cuenta en `devices/sessions-summary`.

**Administración del PIN** (`backend/internal/administration/pin.go`; rutas en `API_CONTRACTS.md`, «Etapa A1 · PIN de
los meseros», no en el contrato para clientes):

- `GET users/badge-colors` (`users.read`): `BadgePalette` = `catalog.Palette` (16 colores con contraste ≥ 3:1 en ambos
  temas; la interfaz nunca escribe un color).
- `PUT users/{id}/pin {pin?, short_name, badge_color}` (`users.manage`, lock de administración, nunca soporte,
  delegación segura): con `pin` asigna o cambia (sube `pin_version`, borra intentos, cierra las sesiones del usuario en
  dispositivos `android` y revoca su credencial: `revokeWaiterAccess`); sin `pin` solo cambia el distintivo de quien ya
  tiene PIN (`user.pin_badge_changed`, sin cerrar nada). Un administrador no se da su propio PIN por aquí (403): lo
  hace en «Mi PIN» con su contraseña.
- `POST users/{id}/pin/remove` (409 `PIN_NOT_SET` si no tiene) y `POST users/{id}/pin/unlock` (registra solo si había
  bloqueo).
- `PUT auth/pin {current_password, pin}` («Mi PIN», `ownPIN`): exige `waiter_devices.use` y la contraseña actual (401
  `INVALID_CREDENTIALS`); cierra las sesiones `android` del usuario salvo la actual.
- **Reglas del PIN** (`validatePIN`, `pin.go`): 4 a 6 dígitos ASCII (400 `VALIDATION_ERROR`) y nada trivial (400
  **`PIN_TOO_SIMPLE`**, mensaje en español): todos iguales, secuencia ascendente o descendente de paso 1 módulo 10
  (también 7890 y 0987), bloque de 2 o 3 dígitos repetido completo (1212, 121212, 123123) y la lista `commonPINs` (50:
  fechas y patrones del teclado más usados de 4, 5 y 6 dígitos).
- Desactivar al usuario revoca su credencial (`user_disabled`, `users.go`); revocar un dispositivo, las de ese
  dispositivo (`device_revoked`, `devices.go`).
- `PATCH devices/{id}` acepta `idle_lock_seconds` opcional (30–3600; sin él se conserva). `GET devices`, la
  configuración, `auth/me` (`terminal`) y `restaurant/state` (`terminal`) lo informan; `auth/me` informa además
  `user.has_pin`. Los usuarios (`GET users` y las respuestas de usuario) llevan `has_pin`, `short_name`, `badge_color` y
  `pin_locked_until`.
- **Registro de movimientos** (nunca el PIN, su hash ni la credencial): `user.pin_set` (`set_by` `administrator` o
  `self`, `replaced`, `pin_version`, distintivo y sesiones y credenciales cerradas), `user.pin_badge_changed`,
  `user.pin_removed`, `user.pin_unlocked`, `session.pin_login`, `session.pin_login_failed` (`reason`) y `user.pin_locked`.

**Contrato v1** (`backend/internal/apicontract/openapi.json`, `docs/API_V1.md` sección «Entrada con PIN y credencial de
envío (etapa A1, fase B)», `TestContractV1`): operaciones nuevas `listPinUsers` (`GET /auth/pin-users`) y `pinLogin`
(`POST /auth/pin-login`); esquemas `PinUser`, `PinUsers` y `PinLoginResult`; esquema de seguridad `offlineCredential`
como alternativa de `session` en `submitOfflineOrder`; `VersionInfo.min_client_versions`, `Terminal.idle_lock_seconds`
y `Principal.has_pin`; códigos nuevos en la tabla de errores. El recorrido (`contractWaiterPIN`) usa el teléfono que
empareja `contractPairing`: lista, PIN incorrecto (401), PIN correcto, `getIdentity` con `idle_lock_seconds` y un
pedido sin conexión **con la credencial y sin sesión** (200).

**Escritorio** (React; Rust solo agrega operaciones a la lista cerrada):

- `desktop/src/services/serverCapabilities.ts`: `useSession` guarda las `capabilities` de `version`; lo del PIN y el
  bloqueo por inactividad solo aparecen con `waiter-pin-v1`.
- **Personal › Usuarios** (`pages/UsersPage.tsx` y `pages/UserPin.tsx`): columna «PIN» («Sin PIN», «Con PIN · Ana» con
  la muestra del color, «PIN bloqueado hasta las 14:05»); «Asignar PIN» / «Cambiar PIN» (panel con el PIN dos veces en
  campos ocultos de solo dígitos, «Nombre corto» y «Color del distintivo» con `ColorField` y la paleta de Go; con PIN,
  dejarlo vacío conserva el PIN y guarda solo el distintivo); «Quitar PIN» secundario con `ConfirmDialog` normal
  (anotado en la lista de reversibles de `components/stage9dStyles.test.tsx`); «Desbloquear». Los mensajes son los de
  Go (`notify.ts`).
- **«Mi PIN»** (sección `pin` de `navigation/sections.ts`, permiso `waiter_devices.use`; opción del menú del usuario solo
  con el permiso y la capacidad; `MyPinPage`): contraseña actual y el PIN dos veces.
- **Sistema › Terminales** (`pages/DevicesPage.tsx`): el tipo `android` se llama **«Teléfono o tableta»** (antes
  «Comandero Android»; cambiadas las expectativas de `stage9gPairing.test.tsx` y `workflow-stage9g-terminals.spec.ts`);
  en «Editar» de un teléfono o tableta, «Bloqueo por inactividad» con `Select` en minutos (1, 2 —«recomendado», por
  omisión—, 3, 5, 10, 15, 30 y 60; un valor guardado fuera de la lista se conserva como opción); «Ver configuración»
  dice «2 minutos sin uso vuelve a la pantalla de entrada con PIN».
- **Registro de movimientos** (`pages/AuditPage.tsx`): etiquetas de las siete acciones nuevas.
- `desktop/src-tauri/src/api.rs`: `user_badge_colors`, `set_user_pin`, `remove_user_pin`, `unlock_user_pin` y
  `set_own_pin` (con su prueba `stage_a1_pin_routes_are_closed`; la entrada con PIN no está en la lista del
  escritorio). `native-driver.ts` las descubre con su expresión regular: no necesitó cambios.

### Decisiones (con su porqué)

1. **Escritorio 0.12.0 con un servidor 0.11.0: se esconde lo nuevo por capacidad; `min_server_version` sigue en
   0.10.0.** El servidor y la aplicación de la computadora principal se instalan juntos (`.pkg`), así que el caso real
   es una terminal Mac actualizada antes que la principal: con la mínima en 0.12.0 esa terminal quedaría entera en
   «Actualiza el servidor»; con la capacidad sigue vendiendo y solo no ve el PIN (que ese servidor no tiene). Además,
   enviar `idle_lock_seconds` a un 0.11.0 rompería el guardado de la terminal (`DisallowUnknownFields`): por eso el
   campo solo se envía con la capacidad. Mismo criterio que la 9H (rutas nuevas sin subir la mínima). La aplicación
   Android sí exige servidor ≥ 0.12.0 (A1-D4).
2. **Paleta del distintivo = `catalog.Palette`.** Ya cumple el contraste en ambos temas y evita una segunda lista; se
   guarda el hexadecimal canónico (Android lo pinta sin traducir), validado por Go contra la paleta. Ruta propia
   `GET users/badge-colors` (`users.read`).
3. **Al bloquearse, la cuenta de fallos vuelve a cero** (`window_start = now()`): al vencer los 5 minutos hay otros 5
   intentos. Con la cuenta de `login_attempts` (sin reinicio) el sexto fallo dentro de la misma ventana volvería a
   bloquear al instante, que sería un escalamiento que A1-D5 descarta. **El quinto fallo ya responde 429** (no el
   sexto, como el inicio de sesión con contraseña), para que el dispositivo diga el bloqueo en ese momento.
4. **Los fallos cuentan para cualquier usuario existente** (también inactivo o sin PIN): si no, el 429 al quinto
   intento delataría quién tiene PIN. El dispositivo nunca ve el PIN de nadie y la lista ya es pública para un
   dispositivo emparejado, pero se siguió la regla del encargo (hash ficticio).
5. **PIN correcto sin `waiter_devices.use` → 403 `PERMISSION_DENIED`** (con el PIN verificado se puede decir el motivo
   sin revelar nada); el usuario tampoco aparece en la lista.
6. **Un administrador no se asigna su propio PIN por la ruta de administración** (403): «Mi PIN» pide la contraseña
   actual, para que una sesión de administrador desatendida no baste; quitar el propio PIN sí se permite (reduce
   acceso). «Mi PIN» exige `waiter_devices.use`: solo lo define quien puede usarlo.
7. **Desviaciones del diseño de la Fase 0**: `DELETE users/{id}/pin` pasó a **`POST users/{id}/pin/remove`** (el puente
   y las rutas del proyecto usan POST para acciones, como `devices/{id}/revoke` y `documents/identity/logo/remove`);
   `pin_attempts.last_device_id` en lugar de un dispositivo del bloqueo aparte (es el del quinto fallo); la paleta es
   la del catálogo (la Fase 0 solo pedía un hexadecimal); y se agregó `PUT users/{id}/pin` sin `pin` para cambiar solo
   el nombre corto y el color (no sube `pin_version` ni cierra sesiones).
8. **La credencial de envío también se revisa en el canal de eventos** (`events.go` la pasa a `dispatch`): así «solo en
   esa ruta» vale sin excepciones. Sesión y credencial juntas en `offline-orders` es 400, para que nunca haya duda de
   quién actúa.
9. **Las sesiones `android` se cierran con `pin_changed`/`pin_removed`** (el dispositivo recibe `SESSION_REVOKED`, como
   con `password_changed`); las de escritorio no se tocan porque el PIN no sirve en ellas. «Mi PIN» conserva la sesión
   desde la que se hace.
10. **«Bloqueo por inactividad» se edita en «Editar»** del teléfono o tableta y se muestra en «Ver configuración»: el
    `PATCH` de terminales reemplaza todos sus campos (caja incluida), y editarlo desde la vista de solo lectura obligaría
    a reenviar la terminal completa desde ahí. El encargo lo pedía en «Ver configuración»; queda a decisión del
    coordinador moverlo.

### Forma final para el agente de Android

- Cabeceras de siempre: `X-Client-Version: 0.1.0`, `X-Client-Kind: android`, `X-Device-ID`, `X-Device-Token`.
  `GET /api/v1/version` → `min_client_versions.android` (0.1.0) y capacidad `waiter-pin-v1` (exigir servidor ≥ 0.12.0).
- `GET /api/v1/auth/pin-users` (sin sesión) → `{"device": {"id", "name", "idle_lock_seconds"}, "users": [{"id",
"name", "initial", "color" (hex o null), "pin_version", "locked_until" (RFC 3339 o null)}]}`. En otro tipo de
  dispositivo: 403 `PIN_DEVICE_NOT_ALLOWED`.
- `POST /api/v1/auth/pin-login` (sin sesión) `{"user_id", "pin"}` → 200 `{"session_token" (43), "pin_version",
"offline_credential" (43)}`; 400 `VALIDATION_ERROR` (forma), 401 `PIN_INVALID`, 429 `PIN_LOCKED` con
  `error.details.locked_until`, 403 `PERMISSION_DENIED` (sin `waiter_devices.use`) o `PIN_DEVICE_NOT_ALLOWED`.
- `POST /api/v1/restaurant/offline-orders` con `X-Offline-Credential: <offline_credential>` **en lugar de**
  `Authorization` → como con sesión (200, 409 `ORDER_REQUIRES_CONFIRMATION`, 409 `OFFLINE_ORDER_ALREADY_ACCEPTED`…);
  401 `OFFLINE_CREDENTIAL_INVALID` = conservar el pedido y pedir al mesero que entre con PIN con conexión; 401
  `OFFLINE_CREDENTIAL_NOT_ALLOWED` en cualquier otra ruta; 400 con sesión y credencial juntas; 401 `DEVICE_REVOKED`
  como siempre.
- `terminal.idle_lock_seconds` también en `GET auth/me` y `GET restaurant/state`. Tras cambiar o quitar el PIN, sus
  sesiones `android` responden 401 `SESSION_REVOKED` y `pin_version` sube en la lista.

### Pruebas ejecutadas

| Qué                                                                                               | Dónde                                                                 | Resultado                                                                                                                      |
| ------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `go-unit` (vet normal e integración, `test -race`, `GOOS=windows` build y vet)                    | `iso.sh go-unit`, dos veces (después de los commits de Go y al final) | **Aprobado** las dos veces                                                                                                     |
| `TestPINRules`, `TestBadgeValidation` (lista de muy usados y cada regla)                          | `internal/administration` (unitarias)                                 | Aprobadas                                                                                                                      |
| `TestClientMinimumVersionByKind` y las de 9G ajustadas                                            | `internal/httpapi` (unitarias)                                        | Aprobadas: 426 a `android` 0.0.9 y 0.1.0-beta, a `desktop` 0.9.9 y 0.1.0 y sin tipo 0.1.0; pasa `android` 0.1.0                |
| `go-int ./internal/database`                                                                      | base `adel_pos_test_b_*`                                              | **ok** (`TestStageA1Upgrade` nueva; `TestStage9HUpgrade` y `existing_install_test` excluyen lo de la 0025)                     |
| `go-int ./internal/administration`                                                                | idem                                                                  | **ok** (178 s), con `TestStageA1WaiterPIN` (9 subpruebas); después del último cambio, `TestStageA1WaiterPIN` sola otra vez: ok |
| `go-int ./internal/operations`                                                                    | idem                                                                  | **ok** (442 s)                                                                                                                 |
| `go-int ./internal/apicontract` (`TestContractV1` con `listPinUsers`, `pinLogin` y la credencial) | idem                                                                  | **ok** (dos corridas)                                                                                                          |
| Vitest (`tsc --noEmit` + `vitest run src`)                                                        | `iso.sh vitest`                                                       | **96 archivos, 914 pruebas aprobadas** (12 nuevas en `stageA1Pin.test.tsx`)                                                    |
| `npm run build`                                                                                   | `iso.sh build`                                                        | Correcto                                                                                                                       |
| `cargo test --locked --offline`                                                                   | `iso.sh cargo`                                                        | **107 aprobadas**, 3 ignoradas (con `stage_a1_pin_routes_are_closed`)                                                          |
| Playwright `workflow-stagea1` Chromium                                                            | Go temporal 8186, vista previa 1424, base nueva                       | **1/1** (4.7 s)                                                                                                                |
| Playwright `workflow-stagea1` WebKit                                                              | idem                                                                  | **1/1** (4.6 s)                                                                                                                |
| Playwright `workflow-stage9g-terminals` Chromium (por el cambio de etiqueta)                      | idem                                                                  | **2/2**                                                                                                                        |
| `format`, `fmt-check` (Prettier, gofmt, `cargo fmt --check`), `sync-check`                        | árbol `Bs`                                                            | Correctos                                                                                                                      |
| md5 de `docs/etapas/*.md`                                                                         | antes y al final                                                      | Idénticos                                                                                                                      |

`TestStageA1WaiterPIN` (`backend/internal/administration/pin_acceptance_test.go:107`) cubre: PIN inválido y trivial
(sin guardar nada), sin `users.manage` 403; lista solo en `android` y sin sesión con exactamente las 6 propiedades; PIN
correcto e incorrecto, usuario inexistente, cuatro fallos y el quinto desde **otro** dispositivo → 429 con
`locked_until` ≈ 5 min, también con el PIN correcto; `user.pin_locked` una sola vez con el dispositivo; desbloqueo
(registrado una vez); sin permiso 403; cambio rápido de usuario (Ana y Beto en la misma tableta, la nueva entrada de Ana
reemplaza solo la suya y su credencial); `sessions-summary` cuenta 2 sesiones `android` (la credencial no); pedido con la
credencial sin sesión a nombre del mesero y del dispositivo; credencial en otra ruta, en el canal de eventos y con otro
método (401), con sesión (400), desde otro dispositivo o inventada (401); permisos vigentes (sin `orders.confirm` 403;
sin `waiter_devices.use` 401 y fuera de la lista); cambiar el PIN cierra la sesión `android` y la credencial, conserva
la de escritorio y sube `pin_version`; cambiar solo el distintivo no cierra nada; «Mi PIN» con contraseña incorrecta,
trivial y correcto (cierra lo `android`, conserva la actual; `set_by: self`); «Mi PIN» sin permiso 403; el
administrador no se asigna su PIN por la ruta de administración; quitar el PIN (y 409 la segunda vez); desactivar y
reactivar; revocar el dispositivo (`DEVICE_REVOKED` y credenciales `device_revoked`); bloqueo por inactividad (600 s,
rechazos 29, 3601, 0 y texto, conservación sin el campo, en la lista y en `restaurant/state`, CHECK de la base); y que
ningún registro de usuario, PIN, sesión o pedido sin conexión contiene los PIN usados, `argon2`, el hash, la sesión ni la
credencial.

### No verificado

- La aplicación Android real contra estas rutas (es de las fases C y E del dispositivo); el escritorio en la ventana
  nativa (solo el puente de Playwright).
- La regresión completa de Playwright (Fase G): la especificación nueva solo corrió filtrada, sobre base nueva. En la
  corrida completa empareja **un** teléfono más: cuenta para el límite de 5 solicitudes por origen cada 10 minutos
  junto con las de `workflow-stage9g-*`, que corren mucho antes.
- `go-int` de los demás paquetes de la tupla (catalog, payments, reports, designer, identity, backupschedule,
  `cmd/ad-el-pos-server`, discovery, legacyimport): no se tocaron; quedan para la Fase G.
- El escritorio 0.12.0 contra un servidor 0.11.0 real (cubierto con Vitest simulando un servidor sin `waiter-pin-v1`).

### Para el coordinador

- **Decidir** si «Bloqueo por inactividad» se queda en «Editar» (decisión 10) o se mueve a «Ver configuración».
- La etiqueta «Comandero Android» pasó a «Teléfono o tableta» en todo el escritorio (Terminales, solicitudes y sesiones
  por tipo).
- Para `CLAUDE.md` (no lo edité): §4 versión 0.12.0 y esquema 25 con la 0025 (sección B); §6, pruebas nuevas
  `TestStageA1WaiterPIN` (`administration`), `TestStageA1Upgrade` (`database`; las de subida anteriores excluyen las
  tablas y el permiso de la 0025), `TestClientMinimumVersionByKind` (`httpapi`) y la especificación
  `workflow-stagea1-pin.spec.ts` (sus datos «Andrea Meseros A1» y «Tableta de meseros A1» no son impresoras, estaciones
  ni productos: sin firma en `maintenance/signatures.go`); §7, la credencial de envío (solo SHA-256, solo en
  `offline-orders`, revisada en cada petición); §11–12, regla permanente: **una pantalla que use rutas nuevas del
  servidor se esconde por capacidad (`services/serverCapabilities.ts`) cuando `min_server_version` no sube**, y el
  distintivo del mesero sale de la paleta de Go; §15, las decisiones 1, 3, 6 y 8 de arriba.
- `AGENTS.md`: la `0025_android_waiters.sql` es la única migración de la etapa A1, editable mientras no se aplique a
  ninguna instalación.
- `docs/ANDROID_INSTALAR.md` (Fase G) debe decir que el rol Mesero ya trae «Entrar con PIN en teléfonos y tabletas de
  meseros» y qué permisos darle en Personal › Roles (tabla de §4 de la Fase 0), y cómo asignar el PIN en Personal ›
  Usuarios.

**Nota del coordinador**: el «Bloqueo por inactividad» quedó en «Editar» del teléfono o tableta (Administración › Terminales) y «Ver configuración» lo muestra; cumple §3.2 («configurable por dispositivo en Administración › Terminales»), así que se conserva así. Rama `etapa-a1-bs` fusionada en `etapa-a1-android` (`bc30780`) sin conflictos; las suites de la fase corrieron sobre su rama, que solo difiere de la fusión en este avance.

---

## Fases C y B (dispositivo) · Servidor, emparejamiento y entrada con PIN

Rama `etapa-a1-cb` (desde `etapa-a1-android` en `aeb9753`), un agente, 16/11/2026. Árbol `$SP/wt/CB`, `ISO_TAG=C`,
Go temporal en 8194 (local) y 8454 (red), **un Go temporal nuevo por corrida** de la suite del dispositivo. **Sin
cambios en el servidor, el escritorio, `config/`, `docs/API_V1.md`, `CLAUDE.md` ni `AGENTS.md`** (`android/CLAUDE.md` sí, a pedido del coordinador);
`docs/etapas/*.md` con el mismo md5 antes y después. Ninguna prueba usó el servicio real de la Mac (8181, 8443, 55432 ni
su carpeta) y ninguna búsqueda NSD de las pruebas buscó `_adelpos._tcp` (las pruebas usan `_adelpostest._tcp`; una
captura de pantalla hecha a mano sí lo buscó, sin contactar al servicio: ver el hallazgo en «Pruebas ejecutadas»).
Emuladores usados: `Pixel_8` (5580), `Pixel_Tablet` (5582) y `ADEL_A1_Tablet_API26` (5590), como mucho dos a la vez y
siempre con `ANDROID_SERIAL` fijo.

Commits:

| Commit    | Tema                                                                                                                                                                                                                                 |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `3854031` | `core:domain`: código de emparejamiento, dirección del servidor, anuncio NSD y cambio de IP, versiones, verificador local del PIN, límite local, bloqueo por inactividad e interfaces de entrada                                     |
| `0502f1e` | `core:data`: emparejamiento con el primer certificado fijado, NSD por versión, cambio de IP con la huella completa, entrada con PIN con y sin conexión, credenciales cifradas con el Keystore, revocación y Room v2 con su migración |
| `9b0543f` | `core:ui`: teclado grande del PIN, distintivo del mesero con el color de Go, menú «Cambiar de mesero» en la barra superior, íconos                                                                                                   |
| `a50d48d` | `core:data`: legibilidad (pasos con nombre en la consulta del emparejamiento), la entrada relee Room al abrir y sin red no pregunta a Go                                                                                             |
| `ce4d9aa` | `feature:access` y `app`: pantallas de entrada, puerta de la aplicación, contenedor, ejecutor de pruebas y pruebas instrumentadas (también contra el Go temporal)                                                                    |
| `4e20859` | Pruebas sin carreras con el teclado en pantalla                                                                                                                                                                                      |
| `39f586b` | `WaiterWork.blocksLogout`: «Cerrar sesión» lo impiden la captura en curso y los pedidos sin enviar (decisión del coordinador)                                                                                                        |
| `d7ee88b` | `scripts/android-e2e.sh`: extremo a extremo contra un Go temporal, reutilizable en las fases F y G                                                                                                                                   |
| `bc7ba59` | `android/CLAUDE.md`: Room v2, la entrada para los demás módulos, el ejecutor y el tipo NSD de prueba, el `targetSdk` de prueba de las bibliotecas, el guion y dos reglas permanentes nuevas                                          |

Cada commit compila por sí solo (comprobado con `git archive` de `a50d48d` + `assembleDebug`, el único que quedaba en
duda al reacomodar los commits).

### Fase C · Servidor y emparejamiento

**Búsqueda automática** (`core/data/.../discovery/NsdServerBrowser.kt:55`): NSD por `_adelpos._tcp` durante 4 s,
descubre, resuelve **de una en una** (desde la API 34 con `registerServiceInfoCallback` y `hostAddresses`, `:135`;
antes con `resolveService` y `host`, en desuso pero única opción) y lee el TXT con las reglas de `collect` del
escritorio (`core/domain/.../server/FoundServer.kt`, `ServerAnnouncements.parse`): `server_id` UUID en minúsculas,
`fp` de 16 hexadecimales, puerto y al menos una dirección de la red local; un mismo `server_id` junta sus direcciones;
el nombre de instancia sin los escapes `\ddd` de DNS-SD. El anuncio **nunca da confianza**: solo la dirección. Si el
sistema no deja buscar (NSD no disponible o, con `targetSdk` 37, sin permiso de red local), `Browse.Unavailable` y la
pantalla lo explica y deja escribir la dirección.

**Dirección a mano** (`core/domain/.../server/ServerAddress.kt:44`): las mismas entradas aceptadas y rechazadas que
`normalize_address`/`valid_server_host` de Rust (prueba con la tabla de `pairing.rs`): IPv4 privada o de enlace local,
IPv6 única local o de enlace local, nombre `.local`, puerto 8443 por omisión. Lectura propia de IPv4/IPv6 sin DNS
(`InetAddress.getByName` consultaría un nombre). Más estricta en tres casos (decisión 2).

**Pantalla «Buscar el servidor»** (`feature/access/.../findserver/FindServerScreen.kt`), como `FindServerPage` del
escritorio: nombre editable del dispositivo («Tableta 1» o «Teléfono 1»), «Computadoras principales en la red» con
«Buscar otra vez» (solo nombres, nunca direcciones), «Escribir la dirección» y, si quedó algo en la bitácora, los
pedidos que no se enviaron con «Borrar la bitácora». Nombre y dirección escritos en `SavedStateHandle`.

**Emparejamiento con el protocolo exacto de la 9G** (`core/data/.../access/PairingRepository.kt`):
`FirstSeenTrustManager` fija el **primer certificado** de la sesión y rechaza otro a mitad de ella (como
`PinnedVerifier::first_seen`); con un servidor de la búsqueda, primero `GET version` y su huella debe empezar como el
`fp` anunciado y el `server_id` ser el anunciado (`AnnouncementMismatch`); compromiso SHA-256(nT), número del servidor,
revelación de nT (`request`, `:294`) y código con `PairingProtocol.code` (`core/domain/.../pairing/PairingProtocol.kt:43`, vector `166101`
de Go y Rust). Pantalla del código (`PairingCodePanel`): código en grande en dos grupos («166 101», con descripción para
lectores de pantalla dígito por dígito), nombre del restaurante, cuenta regresiva, «Cambiar nombre», estado y
«Cancelar»; terminado (rechazada, vencida, cancelada) «Pedir otro código». Consulta cada 2 s, una a la vez
(`FindServerViewModel`). Aprobada: **reclamo único** (`claimCredential`, `:190`), validación de la credencial
(`device_id` UUID, token de 43, mismo `server_id`) y guardado (`saveClaimed`, `:227`): token **cifrado con el Keystore**
(propósito `device-credential`), `server_id`, huella fijada, dirección, nombres e `idle_lock_seconds`. Si el Keystore
falla, lo reclamado se conserva en la sesión y la siguiente consulta reintenta sin reclamar otra vez (hallazgo 3 de la
9G). Tipo `android`, sistema «Android <versión>».

**Conexión con la huella fijada** (`core/data/.../access/PairedConnection.kt`): siempre `PinnedTrustManager` con la
huella guardada, solo `RESTRICTED_TLS`, sin redirecciones (nuevo en `PinnedHttpClient`). Una huella distinta es
`ServerIdentityChanged` → «Esta tableta necesita emparejarse de nuevo» (`RepairScreen`, como `RepairTerminal.tsx`, con
confirmación). **Nunca acepta cualquier certificado.**

**Cambio de IP** (`core/domain/.../server/ServerRelocator.kt` + `PairedConnection.call`, `:56`): si una petición
**no llegó a salir** (no se pudo conectar o respondió otro certificado; lo sabe `RequestSent`, un `EventListener` de
OkHttp que marca `requestHeadersStart`), busca por NSD, prueba solo los anuncios con el mismo `server_id` y la misma
huella corta, acepta la dirección nueva solo si responde `health/live` con la **huella completa**
(`HttpsServerProbe`), la guarda en Room y repite la petición una vez. Una sola búsqueda a la vez y como máximo una cada
10 s, como `ServerLocator::relocate`.

**Versiones** (`core/domain/.../server/ServerCompatibility.kt`): en cada comprobación `GET version`; servidor anterior
a 0.12.0 o sin `waiter-pin-v1` → «Actualiza el servidor del restaurante»; `min_client_versions.android` mayor que la
aplicación, o un 426 en cualquier ruta → «Actualiza esta aplicación» (con «Datos para soporte»: versiones). Ambas con
«Volver a comprobar».

**Permisos de Android** (`feature/access/.../status/LocalNetworkPermissionGate.kt`): la política
`NetworkPermissions.required(sdk, targetSdk)` (`core:domain`) devuelve hoy **ninguno** (`targetSdk` 36: `NsdManager` y
las conexiones a la red local no piden permiso; `NEARBY_WIFI_DEVICES` no lo usa NSD). Con `targetSdk` 37 en Android 17
devolverá `ACCESS_LOCAL_NETWORK` y la puerta ya muestra la explicación en español antes de pedirlo y, si se niega,
cómo darlo en los ajustes (sin él no hay ninguna conexión a la red local, ni escribiendo la dirección). Confirmado en el
emulador: el APK de pruebas de `core:data`, que tomaba `targetSdk` 37, recibió `SecurityException: Missing local
network permission` al anunciar por NSD en Android 17 (se fijó `testOptions.targetSdk = 36`).

### Fase B · Entrada con PIN en el dispositivo

**Pantalla de entrada** (`feature/access/.../pin/PinEntryScreen.kt`): «¿Quién eres?» con los meseros de
`auth/pin-users` en **botones grandes** (72 dp) con su distintivo (inicial en un círculo del color de Go) y «Bloqueado
hasta las 14:35» si aplica; al tocar el propio, el **teclado numérico grande** (`core/ui/.../components/PinPad.kt`:
teclas de 72 dp, puntos que dicen cuántos dígitos —nunca cuáles—, «Borrar» y «Entrar» desde 4 dígitos, máximo 6). En la
tableta lista y teclado lado a lado; en el teléfono uno a la vez (Atrás vuelve a la lista). Mensajes: «PIN incorrecto»,
«Demasiados intentos. Intenta de nuevo a las 14:35» (429 `PIN_LOCKED` con `details.locked_until`, o el límite local),
sin permiso (403), «solo entran los meseros que ya entraron antes en esta tableta», actualización requerida, sin
respuesta y el mensaje de Go para lo demás. La lista se pide al mostrarse y cada 30 s. **El PIN solo vive en memoria**
(`PinEntryViewModel`): nunca en `SavedStateHandle`; cada intento lo borra.

**Color del distintivo** (`core/ui/.../theme/AdelPalette.kt:227`, `waiterBadgeColors`): el único lugar donde un color
se construye desde un dato; la inicial va en blanco o en el texto oscuro, el que más contraste; probado con los 16
colores de `catalog.Palette` leídos de `backend/internal/catalog/palette.go` (≥ 3:1, texto grande en negritas) en ambos
temas.

**Cambio rápido de mesero**: `ActiveWaiterMenu` (`core/ui/.../components/WaiterBadge.kt`) en la barra superior,
siempre visible con el mesero activo: su distintivo, su nombre y «Cambiar de mesero» (y «Entraste sin conexión» si es
el caso). Vuelve a la lista sin cerrar la sesión de Go del mesero (`DeviceAccess.leave`); cada mesero tiene su propia
navegación (`key(userId)` en `AdelPosApp.kt`): lo que capture uno no lo ve el siguiente. Con pedidos sin enviar de ese
mesero no se permite (§7) y se avisa.

**Bloqueo por inactividad** (`core/domain/.../access/IdleLock.kt:38`): `idle_lock_seconds` de Go (120 por omisión; 30
como mínimo, como Go), reloj monótono inyectable, cada toque lo reinicia (`MainActivity.onUserInteraction`). **No
bloquea** con captura en curso o pedidos sin enviar: lo pregunta a `WaiterWorkGate`
(`core/domain/.../access/WaiterWork.kt:39`, «¿este mesero tiene trabajo que impide bloquear o cambiar de mesero?»), que
la Fase E implementará; hoy `NoWaiterWork` responde «no». Lo vigila `AccessViewModel` por mesero activo.

**Entrada sin conexión** (`AccessRepository.enterOfflineLocked`, `:484`): solo quien ya entró **con conexión en este
dispositivo**; verificador PBKDF2-HMAC-SHA256 con sal de 16 bytes (`core/domain/.../access/PinVerifier.kt`,
**120 000 iteraciones**, medidas en `ADEL_A1_Tablet_API26`: 0.27 a 0.32 s), cifrado con el Keystore y ligado al
`user_id` (propósito `pin-verifier:<id>`). Límite local `OfflineAttemptPolicy`: 5 fallos en 15 min bloquean 5 min (la
regla de Go, A1-D5), en Room para que reiniciar no lo borre. Se renueva (sal nueva) en **cada entrada con conexión**;
con la lista de Go se invalida (verificador y credencial de envío) si `pin_version` cambió o el mesero ya no aparece
(`invalidateLocked`, `:348`). Una sesión sin conexión **no tiene sesión de Go** (`ActiveWaiter.hasServerSession =
false`).

**Credencial de envío** (A1-D1): la `offline_credential` de cada `pin-login` se guarda cifrada (propósito
`offline-credential:<id>`) y reemplaza la anterior; `AccessRepository.offlineCredential(userId)` la da a la cola de la
Fase E.

**Sesión revocada o vencida**: `DeviceAccess.report(error, userId)` con `SESSION_*` quita la sesión de ese mesero y, si
es el activo, vuelve a la entrada. **Dispositivo revocado** (`DEVICE_REVOKED` de cualquier petición; `retireLocked`,
`:567`): los pedidos sin enviar (`UnsentOrdersSource`, interfaz para la Fase E; hoy vacía) pasan a la **bitácora del
dispositivo**, se borra todo lo demás en una transacción (servidor, meseros, verificadores, credenciales), se olvidan las
sesiones y se **destruye la llave del Keystore**; pantalla «Este dispositivo fue retirado del restaurante» con «Este
dispositivo fue retirado del restaurante; estos pedidos no se enviaron» y la lista si había. La bitácora se conserva
(también al reabrir la aplicación, en «Buscar el servidor») hasta «Borrar la bitácora» (con confirmación). **No se
construyó cliente SSE** (lo hace la Fase D): `event: interrupted` deberá llamar a `report()`.

**Room v2** (`core/data/.../db/AccessEntities.kt`, `AppDatabase.kt:46`): `paired_server` (una fila; token cifrado),
`known_waiters` (verificador y credencial cifrados; nombre, color y los intentos locales sin cifrar), `device_log`;
migración 1 → 2 con el SQL de `schemas/…/2.json`, probada con `MigrationTestHelper`. Las llaves solo en el Keystore.

**Para otros módulos (Fases D y E)** — `AppContainer.deviceAccess` (`AccessRepository`):

- `serverApi(): ServerApi?` — la API del servidor emparejado en la dirección vigente, con la huella fijada (pedirla en
  cada uso: la dirección cambia con el cambio de IP).
- `requestHeaders(): Headers?` — `X-Device-ID`/`X-Device-Token` y, si hay mesero activo **con sesión**, `Authorization:
Bearer`; `deviceCredentials()` solo las del dispositivo; `sessionToken(userId)` y `offlineCredential(userId)` por
  mesero; `activeWaiter` (`StateFlow`) dice quién y si tiene sesión.
- `report(error: AppError, userId: String?)` — **punto único** para «el dispositivo fue revocado» (`DEVICE_REVOKED`),
  «la sesión del mesero ya no vale» (`SESSION_*`), `DEVICE_*` y huella distinta, desde cualquier petición o el SSE.
- Implementar `WaiterWorkGate` y `UnsentOrdersSource` (hoy `NoWaiterWork`, `NoUnsentOrders` en `AppContainer`).

### Decisiones (con su porqué)

1. **La sesión del emparejamiento confía en el primer certificado** (como el escritorio): un servidor de red local no
   tiene certificado de autoridad; el código de 6 dígitos ata esa huella y el administrador lo compara.
2. **Dirección más estricta que Rust** sin ser más permisiva: IPv4 con ceros a la izquierda o abreviada se rechaza (la
   biblioteca de URL de Rust la interpreta como octal o la completa); `:443` escrito se respeta (Rust lo pierde por ser
   el puerto de HTTPS y usa 8443); puerto 0 rechazado.
3. **Cambio de IP también ante otra huella** en la dirección guardada (Rust lo hace igual: su error de conexión incluye
   el TLS): otro equipo pudo tomar la IP; solo se acepta la dirección nueva con la huella completa. Solo se repite una
   petición que no salió.
4. **Sesiones de Go solo en memoria**: no son necesarias después de cerrar la aplicación (se vuelve a entrar con el PIN;
   la cola usa la credencial de envío) y así no hay un secreto más en disco.
5. **Sin respuesta en la última comprobación se entra directo sin conexión** (y se vuelve a comprobar aparte) para que
   el mesero no espere a que venza la conexión; con conexión y sin respuesta en `pin-login`, se cae al verificador.
6. **Trabajo pendiente** (confirmado por el coordinador): el bloqueo por inactividad lo impiden la captura en curso y
   los pedidos sin enviar; cambiar de mesero, solo los pedidos sin enviar del mesero actual (la captura se queda con
   su mesero, su sesión no se cierra y el siguiente no la ve); un «Cerrar sesión» explícito, ambas cosas. En estas
   fases no hay «Cerrar sesión» (el cambio de mesero y el bloqueo no cierran la sesión de Go): la regla queda en
   `WaiterWork.blocksLogout`, con su prueba, para quien lo ofrezca.
7. **«Emparejar de nuevo» también pasa los pedidos sin enviar a la bitácora y destruye la llave**, como la revocación
   (los secretos eran de otro servidor). Ante una huella distinta se ofrece además «Volver a comprobar».
8. **«Tableta» o «teléfono» en los textos** según `smallestScreenWidthDp ≥ 600`; nombre propuesto «Tableta 1» o
   «Teléfono 1».
9. **La puerta relee Room al abrir la pantalla principal** (`AccessRepository.start`, `:95`) y comprueba el servidor al
   recuperar la red y cada minuto en primer plano (`AccessViewModel`): así se detectan revocación, PIN cambiado y
   bloqueo nuevo sin el canal de eventos.
10. **`testOptions.targetSdk = 36` en `core:data`**: el APK de pruebas de una biblioteca toma `compileSdk` (37) y en
    Android 17 eso ya exige el permiso de red local (visto en Pixel_8).
11. **Ejecutor de pruebas propio** (`AdelTestRunner`): NSD con `_adelpostest._tcp` en todas las pruebas de la app (el
    emulador de Android 17 sí ve los PTR de la red de la Mac, avance §1.5: así nunca se prueba la huella contra el
    servidor real) y `clientVersion` por argumento.
12. Dependencias: ninguna nueva en el catálogo; `feature:access` usa `core-ktx`, `activity-compose`, `lifecycle-*` y
    pruebas JVM ya verificadas (solo cambió su `gradle.lockfile`; `verification-metadata.xml` sin cambios).

### Pruebas ejecutadas

| Qué                                                                                                                          | Dónde                                                         | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| ---------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `core:domain:test` (JVM)                                                                                                     | Mac, JBR 25                                                   | **49/49** (también después de `blocksLogout`) (35 nuevas: `PairingProtocolTest` 5 con el vector `166101`, `ServerAddressTest` 5 con la tabla de Rust, `ServerAnnouncementsTest` 8 con TXT, `server_id` + huella y cambio de IP, `ServerCompatibilityTest` 4, `PinVerifierTest` 5 con Keystore falso, `AccessRulesTest` 8: límite local, inactividad con reloj virtual y con trabajo pendiente, `pin_version`, permisos)                                                                                                                                                                                                                                                                                                                                                                 |
| `testDebugUnitTest` `core:data` (JVM + Robolectric SDK 36, Go simulado con TLS)                                              | Mac                                                           | **39/39** (22 nuevas: `AccessRepositoryTest` 13 —con conexión, sin conexión, límite, `PIN_LOCKED`, `pin_version`, revocación con bitácora, versiones, otra huella, credencial ilegible, cambio de IP con la sonda HTTPS real—, `PairingRepositoryTest` 5, `AccessErrorCodesTest` 4: códigos nuevos, primer certificado, «la petición salió»)                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `testDebugUnitTest` `core:ui` / `feature:access` / `feature:waiter`                                                          | Mac                                                           | **9/9** (distintivo con los 16 colores de Go en ambos temas) / **4/4** (ViewModels: PIN solo en memoria, mensajes, inactividad, cambio de mesero, red, espera del código) / **2/2**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `./scripts/dev.sh android-test` (las anteriores + `lintDebug` de los 5 módulos) y `android-format-check`                     | Mac                                                           | Correctos, Lint sin avisos                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Instrumentadas de los módulos (`app`, `core:ui`, `core:data`), con `ANDROID_SERIAL`                                          | `Pixel_8` API 37                                              | **33/33** + 9 omitidas (las 7 de `AccessOnGoTest` y las 2 de `ServerApiOnDeviceTest`, que piden argumentos)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| Lo mismo                                                                                                                     | `Pixel_Tablet` API 37                                         | **33/33** + 9 omitidas                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| Lo mismo                                                                                                                     | `ADEL_A1_Tablet_API26`                                        | **32/32** + 10 omitidas (también el modo avión, que no existe como shell en Android 8)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| Contra el Go temporal (`AccessOnGoTest`, 7 pasos; primero con el guion del scratchpad, después con `scripts/android-e2e.sh`) | `Pixel_8`, `Pixel_Tablet`, `ADEL_A1_Tablet_API26`             | **7/7** en cada uno: emparejamiento con código (uno equivocado se rechaza) y huella fijada = la del certificado; PIN incorrecto ×4 y **bloqueo al quinto** («Intenta de nuevo a las…», también con el PIN correcto); entrada, credencial y verificador; cambio de mesero; **sin conexión** (modo avión en API 37; en API 26 `adb root` + `svc wifi/data` desde la Mac, «Active default network: none») solo Ana, Beto rechazado; **PIN cambiado** → verificador y credencial invalidados al reconectar y entrada con el nuevo; **426** con `clientVersion=0.0.9`; **otro Go temporal con otra identidad** en la misma dirección → «necesita emparejarse de nuevo» sin que llegue ninguna petición HTTP (registro de Go) y nuevo emparejamiento; **revocación** → «retirado», Room vacío |
| Revisión final (`4e20859`)                                                                                                   | `Pixel_8`: módulos y Go temporal; `Pixel_Tablet`: Go temporal | Las mismas cifras (33/33 + 9 y 7/7). Los módulos de `Pixel_Tablet` y todo `ADEL_A1_Tablet_API26` corrieron sobre `ce4d9aa`: `4e20859` solo cambia dos pruebas                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| PBKDF2 de 120 000 iteraciones (`PinVerifierOnDeviceTest`)                                                                    | API 26 / API 37                                               | 0.27–0.32 s / 0.37–0.54 s por derivación                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `iso.sh format`, `fmt-check`, `sync-check`; md5 de `docs/etapas/*.md`                                                        | Árbol CB                                                      | Correctos; md5 iguales antes y después                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |

**Hallazgo**: una captura de pantalla de la aplicación de entrega (abierta a mano con `am start`, sin el ejecutor de
pruebas) en `Pixel_Tablet` mostró en «Computadoras principales en la red» el anuncio **real** del servicio de la Mac
(«ADEL POS — Eliver’s MacBook Pro»): en Android 17 la resolución NSD ya respondió (en la Fase 0 no). Solo fue la
búsqueda mDNS: no se tocó nada, no hubo ninguna petición ni conexión al servicio real, y la aplicación se cerró y
desinstaló enseguida. Confirma que la lectura del TXT de Go funciona con un anuncio real, y que las pruebas deben usar
siempre `AdelTestRunner` (tipo `_adelpostest._tcp`), como ya lo hacen.

### No verificado

- El emparejamiento con un servidor encontrado por NSD de punta a punta y el cambio de IP en el dispositivo (en la JVM
  sí, con la sonda HTTPS real contra el servidor simulado; la lectura de un anuncio real solo se vio en el hallazgo de
  arriba, sin conectarse); requieren un dispositivo físico y un servidor de prueba anunciado en la red.
- El permiso `ACCESS_LOCAL_NETWORK` en uso (hoy no se pide; solo la política y la pantalla).
- El costo de PBKDF2 en una tableta económica real (medido solo en emuladores sobre la Mac).
- La lista de pedidos sin enviar al retirar el dispositivo con pedidos reales (la cola es de la Fase E; probado en la
  JVM con una fuente falsa).
- El bloqueo por inactividad en el dispositivo (probado en la JVM con tiempo virtual).

### Guion de extremo a extremo (`scripts/android-e2e.sh`)

El guion de la suite contra el Go temporal pasó del scratchpad al repositorio, sin rutas del scratchpad ni de
`iso.sh`. Recibe por variables `ANDROID_SERIAL` (obligatoria: nunca corre en todos los dispositivos conectados),
`ADELPOS_E2E_LOCAL_URL` y `ADELPOS_E2E_NETWORK_URL` (las dos escuchas vistas desde la Mac), `ADELPOS_E2E_PIN`
(opcional; se compara con la huella del certificado servido), `ADELPOS_E2E_SERVER_ID` (el paso 1 comprueba el
emparejado), el token de aprovisionamiento (`ADELPOS_E2E_PROVISIONING[_FILE]`) o la terminal principal
(`ADELPOS_E2E_MAIN_DEVICE`/`_TOKEN`), y `ADELPOS_E2E_DEVICE_HOST` (10.0.2.2 por omisión). **Se niega** (salida 2, antes
de cualquier petición) con los puertos 8181/8443, un host que no sea 127.0.0.1/localhost, una URL sin puerto o el
paso 6 junto con los anteriores, y antes de instalar exige `test_mode: true` en `GET version` de las dos escuchas y
la huella esperada. Pasos 1–5 y 7 con un servidor; el 6 con otro servidor temporal de otra identidad en las mismas
direcciones (`--no-install --steps "6 7"`). Resultados en `android/app/build/outputs/e2e/<serial>/`.

Prueba de punta a punta (una vez, `ADEL_A1_Tablet_API26`, Go temporal `ISO_TAG=C` en 8194/8454): `--steps "1 2 3 4
5"` **5/5** (paso 3 con el corte de red desde la Mac: «Active default network: none» y de vuelta 102); después de
reemplazar el servidor con `iso.sh server-down`/`server-up` (otra identidad), una corrida con la huella vieja se
**negó** («la huella … no es ADELPOS_E2E_PIN») y `--no-install --steps "6 7"` dio **2/2**. Guardas comprobadas sin
servidor: 8181, 8443, `192.168.1.20`, URL sin puerto, el paso 6 mezclado y sin `ANDROID_SERIAL` se rechazan; con el
Go temporal apagado, «no responde test_mode: true». La guarda de `test_mode: false` no se probó con un servidor real
(a propósito).

### Legibilidad (regla del propietario)

Revisado todo lo escrito en la fase con la regla 14 de `android/CLAUDE.md` (no solo ktfmt): la consulta del
emparejamiento se partió en pasos con nombre (`checkApproval`, `claimCredential`, `saveClaimed`), el perfil del mesero
que entra y el `locked_until` de Go salieron a funciones propias, los `if` en una línea y las llamadas posicionales
largas se escribieron en varias líneas con argumentos con nombre, y el ayudante de pruebas `MainComputer` quedó en
funciones pequeñas. Comentarios en español en reglas, seguridad y protocolo.

### Para el coordinador

1. Resueltos con sus decisiones: el trabajo pendiente (decisión 6 y regla nueva en `android/CLAUDE.md`), la
   actualización de `android/CLAUDE.md` y el guion en el repositorio.
2. `android/CLAUDE.md`: las dos reglas nuevas van después de la 13; Prettier renumera las listas, así que en esta rama
   quedan como 14 y 15 y al empalmar con tu regla 14 pasarán a 15 y 16 (ninguna referencia depende del número).
3. Fase E: con trabajo pendiente, un «Emparejar de nuevo», «Actualizar» o la revocación sacan hoy al mesero de la
   pantalla (§0-B pide no cerrar la sesión con pedidos sin enviar); la cola decidirá cómo se muestran.
4. Regla del incidente (ya en `android/CLAUDE.md`): en pruebas nunca abrir la aplicación normal con la búsqueda
   automática en la red real; si se ve el servidor real, no tocarlo.

**Decisiones del coordinador al cerrar las fases C y B del dispositivo** (rama `etapa-a1-cb` fusionada en
`etapa-a1-android`, `fca254c`; único empalme en la lista de reglas de `android/CLAUDE.md`: la regla 14 del propietario
sobre código claro se conserva y las del agente quedan como 15 y 16):

- **A1-D15 · Qué impide cerrar, cambiar o bloquear** (§0-B, §3.2 y §7): cambiar de mesero lo impiden solo los
  pedidos sin enviar del mesero actual; una captura en curso se queda con su mesero sin cerrarle la sesión. El bloqueo
  por inactividad y un «Cerrar sesión» explícito los impiden la captura en curso y los pedidos sin enviar.
- **Pendiente para la Fase E**: «Emparejar de nuevo», «Actualiza esta aplicación» y la revocación con pedidos sin
  enviar deben mostrarlos antes con un aviso claro y su confirmación (la revocación ya lo hace con
  `UnsentOrdersSource`); nunca pasarlos en silencio a la bitácora.
- **Regla para todas las fases**: ninguna prueba abre la app normal con la búsqueda en la red real (en la Fase C, una
  revisión a mano la abrió y mostró el anuncio del servicio real; no se tocó ni hubo ninguna petición).

---

## Fase D · Catálogo en el dispositivo

Rama `etapa-a1-d` (desde `etapa-a1-android` en `762400e`), un agente, 16 de noviembre de 2026, en paralelo con las
fases C y B del dispositivo (otro agente). Árbol `$SP/wt/D`, `ISO_TAG=D`, Go temporal en 8195/8455 (un Go nuevo por
corrida contra el servidor; apagado y su base borrada al terminar), emuladores propios de solo lectura en las consolas
5600 (`Pixel_Tablet`), 5602 (`Pixel_8`) y 5604 (`ADEL_A1_Tablet_API26`), uno a la vez y siempre con `ANDROID_SERIAL`.
**Sin cambios en `backend/`, `desktop/`, `config/`, `CLAUDE.md` ni `AGENTS.md`**; `docs/etapas/*.md` con el mismo md5
antes y después. Ninguna prueba usó el servicio real de la Mac. Al final se fusionó `etapa-a1-android` (fases C y B,
`f7c20c7`) en la rama, a pedido del coordinador, y se conectó la interfaz estrecha de la fase con la entrada al
dispositivo.

Commits (en orden):

| Commit    | Tema                                                                                                                 |
| --------- | -------------------------------------------------------------------------------------------------------------------- |
| `95d2dbb` | `core:domain`: modelos del catálogo, búsqueda, personalización, reglas y revisión del personalizador, tiempo real    |
| `b0aafc6` | `core:data`: `catalog.db`, descarga completa y por cambios, imágenes, actualizaciones, cliente SSE, `ServerAccess`   |
| `24f6a08` | `core:ui` (componentes del catálogo y personalizador), «Catálogo» de muestra en `feature:waiter`, conexión en `app`  |
| `68557b2` | `CatalogOnGoTest` y `GoTestServer`: el catálogo contra el Go temporal en tres etapas                                 |
| `945f39a` | Fusión de `etapa-a1-android` (fases C y B) con la integración: `DeviceServerAccess`, `report`, navegación del mesero |
| `b8db561` | `DeviceServerAccessTest`                                                                                             |
| `ad6c428` | Revisión con la regla del propietario: condiciones que dan un valor en varias líneas, con su comentario              |
| `706dc19` | `android/CLAUDE.md` (catálogo y tiempo real para los demás módulos, `catalog.db`, pruebas) y `docs/DEPENDENCIES.md`  |

### Qué se hizo

**Dominio** (`android/core/domain/.../catalog/`, `realtime/`; Kotlin puro):

- `CatalogModels.kt`: categorías, productos (presentaciones, reglas de ingredientes, grupos con sus límites y lo
  incluido, ranuras de combo con sus alternativas), grupos y opciones (con `portion_name`/`portion_factor`,
  `available_now` y `sold_out` de la descarga), indicaciones rápidas con efecto, imágenes y `Catalog` (fotografía
  inmutable con búsquedas por id, orden de la captura y las indicaciones de la categoría de un producto). Los precios son
  `Money` (cadena decimal de Go) y **solo referencia**; la disponibilidad es la que calculó Go en la descarga:
  `AvailabilityStatus` da «Agotado», «Agotado hasta nuevo aviso» o el motivo de Go. `selectable` y `requiresVariant` son
  las mismas reglas de presentación del escritorio (`productSelectable`, presentación obligatoria sin precio base).
- `CatalogSearch.kt`: búsqueda sin acentos ni mayúsculas (NFD sin marcas combinantes, minúsculas, espacios
  normalizados), cada palabra en cualquier orden; la clave lleva nombre, código y código de barras. Con texto, la
  búsqueda recorre todo el catálogo (como el escritorio: la categoría elegida se ignora).
- `Customization.kt`: `Customization` y `LineSelection` con la forma exacta de `Customization`/`LineRequest` del contrato
  (`backend/internal/catalog/quote_models.go`).
- Reglas del personalizador, puerto de `customizationRules.ts`, `quickNoteRules.ts`, `CustomizationBlocks.tsx` y
  `KitchenNotes.tsx` del escritorio: `IngredientChoices` (Sin/Normal/Extra con −/+, los mismos niveles que
  `catalog.QuickNoteTarget`), `GroupChoices` (elección única y varias porciones con avisos de máximo del grupo y de la
  opción), `ComboChoices` (producto, cantidad, presentación y personalización de cada ranura, hasta 8 niveles),
  `QuickNoteChoices` (efectos excluyentes por ingrediente, reconciliación al cambiar el ingrediente) y `KitchenNotes`
  (observaciones separadas por comas con el **límite de Go en bytes UTF-8**, 500).
- `CustomizationCheck.kt`: revisa en el dispositivo **solo lo que declara el catálogo** (presentación obligatoria,
  mínimos y máximos de cada grupo y opción, límites de cada ranura, observaciones, lo que Go marcó como no disponible),
  con la ranura del combo donde está el problema. Nada de precios.
- `LineDescription.kt`: «Sin cebolla», «Extra tocino», «2 × Catsup», componentes del combo y observaciones (como
  `describeSelection` del escritorio), para la cuenta de la Fase E.
- `Realtime.kt`: `RealtimeEvent`, `RealtimeSignal` (`Changes`, `Rebuild` con `SERVER_RESET`/`FRESH_START`,
  `Interrupted`), `AccessInterruption` tipada (`DEVICE_REVOKED`, otros `DEVICE_*`, `SESSION_*`, permiso perdido, 426),
  `AccessInterruptions` (punto único, con el mesero dueño de la sesión), `RealtimeFeed` con `events(ámbito, tipos)` y
  `rebuilds()`. `ReconnectBackoff.kt`: 1 s, doble en cada falla, hasta 30 s.

**Datos** (`android/core/data/.../catalog/`, `realtime/`, `session/`):

- `catalog.db` (`CatalogDatabase`, **base propia**, versión 1, esquema exportado en
  `core/data/schemas/local.adel.pos.android.core.data.catalog.db.CatalogDatabase/1.json`): cada registro guarda el
  documento JSON de Go y las columnas que se consultan (`CatalogEntities.kt`); `catalog_state` (versión, época, lista,
  tasa), `images` e `image_files` (la caché). Reemplazable completa sin tocar `AppDatabase`.
- `CatalogPayload.parse` interpreta la descarga **antes** de la transacción; `CatalogStore.save` la guarda en **una
  transacción**: `full` reemplaza todo (inserción estricta: un id repetido deshace todo), si no aplica los cambios con
  upsert, quita archivados y `deleted` (en bloques de 500 por el límite de variables de SQLite en Android 8) y, con
  `reset`, marca las imágenes para revalidarlas con su `ETag`.
- `CatalogSynchronizer`: completa la primera vez; después `since` + `epoch` guardados; con otra época o versión, Go
  responde `reset: true` y se reemplaza. Rechazos de dispositivo, sesión y versión se avisan al punto único; sin permiso
  del catálogo solo se informa en el estado. `CatalogRepository` (`CatalogSource`): canal «conflated» de capacidad uno
  → **una descarga a la vez y, si llegan peticiones mientras corre, exactamente una más**; recarga la fotografía solo
  si cambió algo; poda y descarga anticipada de imágenes.
- `CatalogUpdateTriggers`: al pasar a primer plano, con sesión nueva, al pasar de «Sin conexión» a «En línea» (y pide
  al canal reconectar ya), con eventos del ámbito `catalog` o `Rebuild`, y cada 5 minutos en primer plano.
- `CatalogImageCache` (`CatalogImages`): `cacheDir/catalog-images/<id>` (id validado como UUID), de una en una, con
  `If-None-Match` y 304, comprobación del SHA-256 contra el `checksum` de Go, tope de 2 MiB por imagen, límite de 48 MiB
  con poda de las que el catálogo ya no usa y de las menos usadas.
- `CatalogApi`: `getCatalogSync` y `getCatalogImageContent`. `CustomizationDtos.kt`: `CustomizationDto`,
  `LineRequestDto`, `toDto()`/`toDomain()`/`toLineRequest()` y `CustomizationJson` (escribe `"variant_id": null`, listas
  vacías siempre y `quick_notes` solo si hay, como el escritorio y Go).
- `RealtimeClient` (`RealtimeFeed`, okhttp-sse 5.5.1): **un solo flujo por proceso**, abierto solo en primer plano y
  con sesión; reconexión enseguida tras el cierre de 30 s y con espera creciente tras fallas; `readTimeout` de 10 s;
  cursor solo del `id` de los mensajes `changes` (nunca el `0` de `interrupted`); `reset: true` → `Rebuild`; abrir
  desde la semilla (o sin cursor) → el primer mensaje es `Rebuild(FRESH_START)`; `interrupted` o el rechazo al abrir
  por dispositivo, sesión, permiso o versión → `Interrupted` + aviso, sin reconectar hasta que cambie la sesión; 429
  `EVENT_CONNECTION_LIMIT` y 5xx → espera creciente. `RecoveryStateCursorSeed`: el `event_cursor` de
  `restaurant/state`.
- `ServerAccess` (interfaz estrecha) y `AuthorizedServer` (dirección y cliente con la huella fijada **pedidos en cada
  uso**, cabeceras del dispositivo y del mesero, mesero dueño). `DeviceServerAccess` la implementa sobre
  `AccessRepository` (fases C y B): conexión solo con el dispositivo listo y un mesero con sesión de Go; sus avisos van a
  `AccessRepository.report(error, userId)`. Para la dirección y el cliente se agregó
  `AccessRepository.serverTransport()` (`PairedConnection.transport()`), porque las imágenes con `ETag` y el SSE no
  caben en `ServerApi`.

**Interfaz** (`android/core/ui/.../catalog/`, compartida con la A2): `CatalogBrowser` (búsqueda con el teclado del
sistema y «Borrar», `CategoryBar` siempre a la mano, `ProductGrid` adaptable), `ProductCard` (precio de referencia,
«Elige presentación», «Agotado»/«Agotado hasta nuevo aviso»/motivo de Go, atenuada y **sin poder elegirse**),
`ProductImage` (marcador con la inicial mientras no hay imagen; decodificación reducida y caché en memoria),
`ProductCustomizer` + `CustomizerBlocks` (presentaciones, grupos de elección única o con −/+, ingredientes
Sin/Normal/Extra, ranuras de combo con «Personalizar …», indicaciones rápidas, observaciones con el teclado del sistema,
cantidad y «Agregar», con lo que falta en rojo), `Stepper` (−/+ de 48 dp). Textos en `strings_catalog.xml` (archivo
propio para no empalmar con la otra fase); íconos `CatalogIcons` (buscar, cerrar, más, menos) con el ayudante de
`AdelIcons` hecho `internal`.

**Muestra y aplicación**: «Catálogo» de muestra (`feature/waiter/.../catalog/`, `CatalogSampleViewModel` con la
búsqueda, la categoría y el producto en `SavedStateHandle`) desde un botón «Ver el catálogo» en Mesas: en la tableta,
cuadrícula y personalizador lado a lado; en el teléfono, el personalizador a un toque; lo agregado se describe con
`LineDescription`. `CatalogRealtimeContainer` y `AppForeground` (`ProcessLifecycleOwner`) en `app`;
`AdelPosApplication.onCreate` los arranca (después de que `AdelTestRunner` deja sus ajustes). En la navegación del
mesero (de las fases C y B) se agregó `CatalogDestination` con su título.

### Decisiones (con su porqué)

1. **Documento JSON por registro en `catalog.db`** en lugar de reproducir el modelo del catálogo en tablas: el catálogo
   de Go es muy anidado (presentaciones, reglas, grupos, ranuras, tramos) y el dispositivo solo lo lee; así la base se
   reemplaza en una transacción, un campo nuevo del contrato queda guardado y solo se indexa lo que se consulta.
2. **La interpretación va fuera de la transacción y la inserción de la descarga completa es estricta (ABORT)**: un
   documento inesperado o un id repetido deja el catálogo anterior intacto (probado en la JVM y en el dispositivo).
3. **«Agotado» como lo dijo Go en la descarga**, sin vencer localmente con `sold_out_until`: el trabajador de Go limpia
   las marcas vencidas cada 30 s y avisa por el canal; sin conexión manda lo último descargado y Go decide al recibir.
4. **Niveles Sin/Normal/Extra** iguales a los botones del escritorio y a `QuickNoteTarget` (Extra = mín(incluido + 1,
   máximo)): así una indicación con efecto y el personalizador nunca difieren.
5. **Límite de las observaciones en bytes UTF-8** (`len(customization.Notes) > 500` en Go): con acentos caben menos de
   500 letras; el escritorio cuenta letras.
6. **Componente de combo**: su disponibilidad la da la opción de la ranura de la descarga (Go la calcula por ranura) y,
   si no la trae, el producto; el original de la ranura se ofrece siempre (como el escritorio).
7. **El canal se cierra en segundo plano y cada periodo en primer plano (o sesión nueva) empieza desde la semilla**
   (`event_cursor` de `restaurant/state`) y avisa `Rebuild(FRESH_START)` en su primer mensaje: lo ocurrido mientras
   estuvo cerrado se recupera leyendo el estado, no evento por evento. Dentro de un periodo, cada reconexión sigue
   desde el último cursor.
8. **Espera de reconexión**: una conexión que recibió mensajes y se cerró reconecta enseguida (cierre de 30 s); una
   que se cayó después de recibir mensajes espera lo mínimo; las fallas seguidas sin un mensaje esperan 1, 2, 4… 30 s.
9. **Permiso de eventos perdido no se avisa a `report`** (que lo trataría como «sin conexión»): el canal se detiene
   hasta que cambie la sesión y el mesero sigue dentro. Una huella distinta en el catálogo tampoco se avisa: la entrada
   hace su propia comprobación y busca el servidor por `server_id` y huella antes de pedir emparejar de nuevo.
10. **`AuthorizedServer` compara por credenciales** y pide la dirección en cada uso: el cambio de IP no reabre nada (la
    siguiente conexión usa la dirección nueva) y otra sesión siempre reabre el canal.
11. **Componentes en `core:ui`** (no en `feature:waiter`), porque un `feature` no depende de otro y la A2 los usará en
    Para llevar; la pantalla de muestra sí está en `feature:waiter`.
12. **`lifecycle-process` declarado en `app`** (ya llegaba como transitiva) para el primer y segundo plano del proceso,
    que no se confunde con una rotación.

### Pruebas ejecutadas

| Qué                                                                                                                               | Dónde                                                                                       | Resultado                                                                                                                                                                                                                                                                                                                                                                 |
| --------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| JVM y Robolectric SDK 36 de todos los módulos (`:core:domain:test testDebugUnitTest`) y `lintDebug`, tras la fusión               | Mac, JBR 25 (`iso.sh gradle` y `./scripts/dev.sh android-test`)                             | **154/154** (51 de la fase: `CatalogSearchTest` 5, `CustomizationChoicesTest` 7, `CustomizationCheckTest` 5, `LineDescriptionTest` 2, `RealtimeTest` 2, `CatalogSyncTest` 8, `CatalogImageCacheTest` 6, `CustomizationJsonTest` 4, `RealtimeClientTest` 8, `CatalogUpdateTriggersTest` 1, `DeviceServerAccessTest` 2, `CatalogSampleViewModelTest` 1); Lint sin problemas |
| Instrumentadas (`connectedDebugAndroidTest`), XML contados por módulo                                                             | `Pixel_Tablet` API 37 (5600), tras la fusión                                                | **52 pruebas**: app 23 (13 correctas + 10 omitidas: las del Go temporal), core:data 16 (14 + 2 omitidas), core:ui 13; 0 fallas                                                                                                                                                                                                                                            |
| Lo mismo                                                                                                                          | `Pixel_8` API 37 (5602)                                                                     | **52**: app 23 (10 omitidas), core:data 16 (2 omitidas), core:ui 13; 0 fallas                                                                                                                                                                                                                                                                                             |
| Lo mismo                                                                                                                          | `ADEL_A1_Tablet_API26` (5604), dos veces (la segunda tras el último commit de código)       | **52**: app 23 (11 omitidas: además el modo avión, que no existe en Android 8), core:data 16 (2 omitidas), core:ui 13; 0 fallas                                                                                                                                                                                                                                           |
| De la fase en cada corrida: `CatalogDatabaseMigrationTest` 2, `CatalogComponentsTest` 4, `CatalogSampleScreenTest` 1              | los tres emuladores                                                                         | Correctas                                                                                                                                                                                                                                                                                                                                                                 |
| `CatalogOnGoTest` contra el Go temporal (8195/8455), etapas `a`, `b` y `c` con los pasos de la base entre ellas                   | `Pixel_Tablet` (dos veces: antes y después de la fusión), `Pixel_8`, `ADEL_A1_Tablet_API26` | **3/3 en cada corrida**: completa (303 productos), cambios por `catalog.changed` y por descarga, agotado («Agotado» y no se puede elegir), 304, baja en `deleted` y `reset`                                                                                                                                                                                               |
| Tiempos (registro `CatalogOnGoTest`)                                                                                              | descarga completa de 303 productos / búsqueda sin acentos (promedio de 20)                  | API 26: **219 ms / 0.61 ms**; Pixel_Tablet: 776 ms / 0.54 ms; Pixel_8: 792 ms / 0.55 ms                                                                                                                                                                                                                                                                                   |
| `iso.sh format`, `fmt-check`, `sync-check`; `./scripts/dev.sh android-format-check`                                               | árbol D                                                                                     | Correctos                                                                                                                                                                                                                                                                                                                                                                 |
| Bloqueos y verificación (`resolveAllDependencies … connectedDebugAndroidTest --write-locks --write-verification-metadata sha256`) | `Pixel_Tablet`, antes y después de la fusión                                                | Correcto; solo `okhttp-sse` 5.5.1 (SHA-1 igual al de Maven Central) y el POM de `androidx.collection` 1.0.0 (SHA-256 igual al de Google Maven); bloqueos de `core:data`, `app` y `feature:waiter`                                                                                                                                                                         |
| md5 de `docs/etapas/*.md`                                                                                                         | antes y al final                                                                            | Idénticos                                                                                                                                                                                                                                                                                                                                                                 |

`CatalogOnGoTest` (en `app/src/androidTest`) y su guion: `$SP/tmp-D/scripts/catalog-on-go.sh <serie> <etiqueta>`
(instala una vez, corre las tres etapas con `am instrument` y `AdelTestRunner`, y entre ellas borra el producto
`TEMP-D` y cambia `app.catalog_epoch` en la base **temporal** de `run-D`); salidas en `$SP/tmp-D/results/go/`. Los XML
de las instrumentadas de cada emulador están en `$SP/tmp-D/results/{tablet37,pixel8,api26,api26-final}/`.

### No verificado

- Un dispositivo físico, `ADEL_A1_Phone_API33` (no lo pedía el encargo) y la rotación real del personalizador
  (`SavedStateHandle` guarda búsqueda, categoría y producto; la personalización en curso vive en el ViewModel: sobrevive
  a la rotación, no a la muerte del proceso, que vuelve al producto con su configuración original).
- La caché de imágenes con un catálogo de cientos de fotos reales (se probó con imágenes chicas y límites forzados).
- La navegación Mesas → «Catálogo» con un mesero dentro en la aplicación completa (necesita emparejar; la pantalla se
  probó directamente en uno y dos paneles). Con la aplicación real sin emparejar no hay sesión y el catálogo no se
  descarga (lo esperado).
- El canal de tiempo real de la aplicación completa con la entrada real (lo cubren `DeviceServerAccessTest` y
  `CatalogOnGoTest` con su propio cliente) y el corte de red con el canal abierto.
- Robolectric en SDK distintos de 36.

### Para el coordinador

1. **Cambio en código de las fases C y B**: `AccessRepository.serverTransport()` y `PairedConnection.transport()`
   (dirección vigente y cliente con la huella fijada), porque las imágenes con `ETag`/304 y el SSE no caben en
   `ServerApi`. Es aditivo.
2. La **Fase E** usa `AppContainer.catalogRealtime`: `catalog`, `images`, `realtime.events(RealtimeScopes.RESTAURANT)`
   y `realtime.rebuilds()`; los componentes `CatalogBrowser` y `ProductCustomizer` (`core:ui`); `toLineRequest()` /
   `Customization.toDto()` para `saveDraft` y `submitOfflineOrder`, y la versión y época de `catalog.value.version`.
   Puede pasar su propia `RealtimeCursorSeed` a `CatalogRealtimeContainer` para no leer `restaurant/state` dos veces.
3. `CatalogOnGoTest` necesita pasos en la base temporal entre etapas; el guion vive en el scratchpad. Si la Fase F lo
   quiere en `scripts/`, hace falta acceso a la base del Go temporal (como el arnés).
4. Para `CLAUDE.md` (no lo edité): §4 y §6 con `catalog.db`, las pruebas nuevas y `CatalogOnGoTest`; §15 las decisiones
   1, 3, 5, 7 y 9. Para `AGENTS.md`: nunca abrir otro canal de eventos (usar `catalogRealtime.realtime`) y los precios
   del catálogo local solo como referencia.

**Al cerrar la Fase D** (rama `etapa-a1-d`, que ya traía fusionadas las fases C y B, fusionada en `etapa-a1-android`,
`616f812`): se aceptan los métodos aditivos `AccessRepository.serverTransport()` y `PairedConnection.transport()` (las
imágenes con `ETag` y el SSE no caben en `ServerApi`). El guion de `CatalogOnGoTest` escribe en la base temporal entre
etapas y queda en la carpeta temporal de la sesión; la Fase F decide si lo lleva a `scripts/`.

---

## Fase E · Servidor

Un agente, en su propio árbol de trabajo (`etapa-a1-es`, desde `etapa-a1-android` en `a139e2e`), en paralelo con la
fase C/B del dispositivo (otro agente en `android/`). 16/11/2026. **Sin cambio de versión** (sigue 0.12.0, esquema 25:
la 0025 todavía no está en ninguna instalación) y sin cambios en el escritorio. Migración `0025_android_waiters.sql`,
**sección E**. Pruebas en copias aisladas (`ISO_TAG=E`): clúster temporal 55499 con una base `adel_pos_test_e_*` por
paquete. **No se tocó la instalación real** ni `android/`, `scripts/dev.sh`, `.gitignore`, `docs/DEPENDENCIES.md`,
`CLAUDE.md` ni `AGENTS.md`; `docs/etapas/*.md` con el mismo md5 antes y después. No se usó Go temporal, Playwright ni
emuladores (no hicieron falta).

Commits en `etapa-a1-es`:

- `cc3e4e5` — evento `restaurant.account.preparation_changed` (A1-D11), `kitchen/preparation.go`.
- `3e34759` — `error.details.reason: account_not_open` con el estado de la cuenta (§6.4).
- `3363894` — `replaces_draft_items` en el pedido sin conexión y `restaurant_items.draft_item_id` (0025, sección E) con
  su comprobación en `TestStageA1Upgrade` (A1-D6).
- `685c81b` — `event_cursor` en `restaurant/state` y capacidad `waiter-mode-v1`.
- `70b31c3` — contrato v1 (`openapi.json`, `TestContractV1`), `docs/API_V1.md`, `API_CONTRACTS.md` y
  `docs/RESTAURANT_OPERATIONS.md`.
- `d8dd384` — prueba `TestStageA1WaiterMode` (`operations`).

Cada commit compila y pasa `go vet -tags=integration` por sí solo (comprobado con `git archive` de cada uno).

### Qué cambió

**1. Rutas que ya existían, ahora en el contrato para clientes** (`openapi.json`, `docs/API_V1.md` «Modo Mesero»,
`TestContractV1`). Comprobado en el código antes de documentar (método, ruta, cuerpo, permisos y errores):

- `getRestaurantOverview` — `GET /restaurant/overview` (`operations/areas.go:113`): permisos `restaurant.tables.read`
  (en la ruta) y `restaurant.areas.read` (en la función); `{areas, tables, settings, timezone}` con los esquemas nuevos
  `Area`, `RestaurantSettings` y `RestaurantOverview`.
- `Table` con lo que Go ya enviaba (`operations/models.go:23`): `shape` (`rectangle`/`round`), `x`, `y`, `width`,
  `height` (lienzo lógico de 1000 × 560, restricciones de la 0004), `seats`, `occupied_seconds`; ahora obligatorias en
  el esquema junto con `occupied_since` y `long_open`.
- `cancelUnits` — `POST /restaurant/accounts/{id}/cancel` (`operations/cancellations.go`): `orders.cancel` +
  `accounts.read`, `{operation_id, revision, version_id, quantity, reason (5–500)}` → `Bill`.
- `previewEdit` — `POST …/edit-preview` y `editUnits` — `POST …/edit` (`operations/amendments.go`): `orders.edit` +
  `accounts.read` + `catalog.read` + `catalog.prices.read`; reducir la cantidad exige además `orders.cancel` (403);
  vista previa `{revision, version_id, line}` → `QuoteResult`; modificar con `operation_id`, `expected_fingerprint`
  (409 `QUOTE_CHANGED`) y `reason`. Errores documentados: `UNIT_HAS_PAYMENT`, `UNIT_HAS_DISCOUNT`,
  `ACCOUNT_BELOW_PAYMENTS`, `ACCOUNT_CONFLICT` y los del catálogo al recalcular.
- `Account` documenta además `assigned_name`, `elapsed_seconds`, `urgency` y `long_open` (opcionales) y los valores de
  `preparation_state`; `ConfirmedLine.preparation_state` y `state`, también.
- Capacidad nueva **`waiter-mode-v1`** en `GET version` (`httpapi/router.go`), para que la app compruebe la función en
  lugar de suponerla por la versión.

**2. Evento de preparación (A1-D11)** — `backend/internal/kitchen/preparation.go` (nuevo):

- `restaurant.account.preparation_changed` con la cuenta como agregado, en el ámbito **`restaurant`** (o **`takeaway`**
  para una cuenta para llevar, la misma regla de `operations/queries.go` para los eventos de la cuenta, de modo que
  quien no tiene `takeaway.read` no se entere de los pedidos para llevar). Sin contenido, como todos.
- Se anota la cuenta (por la pertenencia **actual** de las unidades, que cambia al transferir o dividir) en cada punto
  que modifica los registros de los que sale `kitchen.VersionPreparation` (liberaciones, unidades liberadas, tareas y
  avisos pendientes), y se publica **una vez por cuenta** al final de la operación:
  - API de cocina (`kitchen/handler.go`, después de `write`): `saveStates` (`states.go`, cuentas de las unidades de las
    tareas), `releaseAction` liberar o programar (`releases.go`), `retryChange` (`changes.go`);
  - liberación automática (`worker.go`, `ReleaseDue`, una vez por cuenta del lote);
  - `kitchen.Integrate` (la integración de las operaciones de la cuenta, en su misma transacción): `order.confirmed`,
    `item.modified`, `item.cancelled`, `account.cancelled` y `account.updated` solo si reprogramó alguna liberación.
- Todos los escritores de esas tablas están en `kitchen` (comprobado con `grep` de `kitchen_tasks`, `kitchen_releases`,
  `kitchen_unit_releases` y `kitchen_pending_changes`), así que el punto de publicación es uno por entrada.
- **Restricciones de `realtime_events`**: no hizo falta cambiarlas (el ámbito `restaurant`/`takeaway` ya existe y
  `kind` no tiene CHECK); `realtime_events_scope_check` no se tocó.
- **Escritorio**: `desktop/src/hooks/useRealtime.ts` marca `changed` por mensaje del canal (no por evento) y lanza un
  solo `adel-server-change`; Rust (`realtime.rs`) pasa el JSON sin filtrar tipos. Un tipo nuevo no rompe nada: en una
  confirmación el evento llega en el mismo mensaje que `restaurant.order.confirmed` (la misma transacción) y no provoca
  consultas de más; con un avance de cocina, quien tiene `accounts.read` y no ve cocina (caja, mesero de escritorio)
  ahora recarga lo visible una vez, que es justo lo que pide A1-D11. No se cambió el escritorio.

**3. Cuenta cerrada mientras tanto (§6.4)** — `ensureOpen` → `accountNotOpen` (`operations/queries.go:112`): el mismo
`400 OPERATION_VALIDATION` (compatibilidad), con mensaje por estado («La cuenta «Mesa 4» ya se cobró. No admite nuevas
operaciones.», «…ya se canceló o se liberó…», «…se combinó con otra cuenta…») y `error.details`
`{reason: "account_not_open", account_id, account_state: closed|voided|merged, merged_into_id}` (esquema
`AccountNotOpenDetails`). `ensureOpen` es común, así que aplica a guardar el borrador, la vista previa, la vista previa
del borrador sin guardar, enviar a cocina, cancelar y modificar unidades (y también a transferir, combinar, liberar,
descartar y cancelar la cuenta). En `submitOfflineOrder`, `account_closed` usa el mismo mensaje y lleva el estado en
`now`.

**4. Borrador del servidor y pedido sin conexión (A1-D6)** — `operations/offline_orders.go`:

- `replaces_draft_items` (opcional, **solo con `account_id`**, hasta 20 `DraftItem` únicos con cantidad 1–1000 y la
  forma de personalización del borrador; si no, 400): los renglones del borrador que **ese dispositivo** había guardado
  y que el pedido sustituye, tal como los vio (`validateReplacedDraft`, `:253`).
- Revisión (`diagnoseReplacedDraft`, `:430`), con la cuenta abierta: un renglón que sigue **exactamente** igual
  (producto, presentación, cantidad y personalización con `sameDraftLine`) se quitará; si no, un cambio **aceptable**
  (`replacedDraftChange`, `:461`) con `item_id` = el id del renglón y `product_id`/`product_name` del que vio:
  - `draft_item_changed` — sigue en el borrador con otro contenido: «Papas: lo cambiaron en otra terminal. Si confirmas,
    tu pedido entra como lo capturaste y el cambio de la otra terminal se queda sin enviar en la cuenta.»;
  - `draft_item_sent` — ya no está y una partida confirmada de la cuenta conserva su id: «Tacos al pastor: ya se envió
    desde otra terminal. Si confirmas, tu pedido entra completo y puede repetirlo.» (con el mismo dispositivo, «ya se
    había enviado desde este dispositivo»);
  - `draft_item_removed` — ya no está y nunca se envió: «…: lo quitaron de la cuenta en otra terminal. Si confirmas, tu
    pedido entra como lo capturaste.»
- Al aceptar (`removeReplacedDraft`, `:728`), en la misma transacción y después de `touch`: borra solo esos renglones
  y publica **lo mismo que un guardado del borrador** (`handler.event("draft.saved", antes, después)`: historia de la
  cuenta, `restaurant.draft.saved` en tiempo real y auditoría `restaurant.draft.saved`). Los demás renglones (los de
  otras terminales) conservan su lugar. La respuesta y la auditoría `restaurant.offline_order.accepted` llevan
  `removed_draft_item_ids`.
- **Cómo se sabe «ya enviado»**: la partida confirmada no conservaba el id del renglón (`confirmLines` creaba
  `restaurant_items` con id nuevo). Ahora `confirmLines` (`confirmations.go`) guarda en
  **`restaurant_items.draft_item_id`** el id que el cliente dio al renglón (del borrador al «Enviar a cocina», o de la
  partida del pedido sin conexión). Migración **0025, sección E**: la columna (uuid opcional, NULL en lo existente) y el
  índice parcial `restaurant_items_draft_item_idx`; ninguna fila cambia.
- Idempotencia: el rechazo no guarda la clave; la confirmación usa la misma `operation_id`; un reintento después de
  aceptado devuelve la misma respuesta sin quitar nada más; otro contenido con la misma clave,
  `OFFLINE_ORDER_ALREADY_ACCEPTED`. El mensaje del 409 aceptable ahora menciona «lo que se capturó en otra terminal».
  Funciona igual con la credencial de envío (es el mismo `operations.Handle`).

**5. Cursor de eventos** — `GET restaurant/state` entrega `event_cursor` (`events.Head`, `events/store.go:54`), leído en
`operations.Recovery` **con el lock 7820148305 compartido ya tomado y antes** de mesas y cuentas
(`operations/recovery.go:37`): las escrituras de cuentas, cocina y cobros toman ese lock en exclusiva hasta su commit,
así que ninguna queda entre el cursor y la lectura; el resto (catálogo) confirma su evento con secuencia mayor (los
productores toman el lock de eventos 7820148306 hasta el commit) y a lo más provoca una lectura de más. Documentado el
orden recomendado al volver de segundo plano (`docs/API_V1.md`, «Sesión y recuperación del estado», punto 5).

**Documentación**: `openapi.json`, `docs/API_V1.md` (operaciones, capacidad, cursor, «Modo Mesero», borrador y pedido
sin conexión, códigos), `API_CONTRACTS.md` («Etapa A1 · modo Mesero en el servidor (fase E, esquema 25)») y
`docs/RESTAURANT_OPERATIONS.md`.

### Decisiones (con su porqué)

1. **Coincidencia por contenido, no por `device_id` del renglón.** `saveDraft` reescribe el borrador completo y pone
   en **todas** las filas el autor y el dispositivo de quien guarda (`insertDraftItems`): si otra terminal guarda sin
   tocar el renglón del mesero, el renglón ya «es» de la otra terminal. Exigir el mismo dispositivo daría un falso
   «cambiado». Se quita solo lo que el pedido nombra **y** sigue exactamente como lo vio: nunca se borra en silencio lo
   de otra terminal, y un cliente no gana ningún poder nuevo (con `orders.capture` ya podía reemplazar el borrador).
2. **Los tres cambios del borrador son aceptables** (como el precio, según el encargo). Al aceptar `draft_item_changed`
   el renglón de la otra terminal **se queda** en el borrador (no se borra lo ajeno); el mensaje lo dice. `before` y
   `now` van en `null` en estos tres: el mensaje lo explica y la confirmación se compara por tipo, renglón y producto.
3. **`draft_item_id` también en las partidas del pedido sin conexión**: si la app reutiliza el id del renglón como id
   de la partida (lo natural), una segunda cola que sustituya ese renglón lo verá como ya enviado. No es único: un
   renglón repetido a propósito deja dos partidas.
4. **Evento de preparación también en las operaciones de la cuenta** (confirmar, modificar, cancelar, reprogramar),
   aunque esas ya publican su `restaurant.*`: así la regla es una sola («cambiaron los registros de cocina de la
   cuenta») y la A2 («platillos listos») puede escuchar solo este tipo. Cuesta un evento más por confirmación, en el
   mismo mensaje del canal. **No** se publica al guardar el borrador ni al transferir o combinar (no cambian registros
   de cocina; publican su propio evento).
5. **Ámbito `takeaway` para cuentas para llevar** (el encargo decía «ámbito restaurant»): es la regla de todos los
   eventos de una cuenta; publicarlo en `restaurant` filtraría la actividad de para llevar a quien no tiene
   `takeaway.read`. Para el mesero de salón no cambia nada; la A2 lo recibirá con `takeaway.read`.
6. **Mensajes por estado en `ensureOpen`** (antes uno genérico): el código y el estado no cambian y ningún cliente ni
   prueba dependía del texto (comprobado con `grep` en Go, escritorio y Playwright).
7. **Capacidad `waiter-mode-v1`**: la app exige servidor ≥ 0.12.0, pero la convención del proyecto es comprobar
   capacidades; no cuesta nada y evita depender del número.

### Forma final para el agente de Android

- `GET /api/v1/version` → `capabilities` incluye `waiter-mode-v1`.
- `GET /api/v1/restaurant/overview` → `{"areas": [{"id","name","description","sort_order","active","revision",
"table_count"}], "tables": [{"id","area_id","name","shape","x","y","width","height","seats","active",
"out_of_service","revision","account_count","state","occupied_since","occupied_seconds","balance","long_open"}],
"settings": {"warning_minutes","overdue_minutes","long_open_hours","revision"}, "timezone"}`. Mapa: lienzo de
  1000 × 560 por área.
- `POST /api/v1/restaurant/accounts/{id}/cancel` `{"operation_id","revision","version_id","quantity","reason"}` → `Bill`.
- `POST …/edit-preview` `{"revision","version_id","line": LineRequest}` → `QuoteResult` (`fingerprint`,
  `pricing_policy`); `POST …/edit` `{"operation_id","revision","version_id","line","expected_fingerprint","reason"}` →
  `Bill`. Mismo producto; reducir cantidad exige `orders.cancel`.
- Tiempo real: `restaurant.account.preparation_changed` (scope `restaurant`, `aggregate_id` = cuenta) → volver a leer
  `GET restaurant/accounts/{id}`. Uno por cuenta y transacción. `preparation_state` de cuenta: `without_items`,
  `draft`, `pending_release`, `scheduled`, `released`, `preparing`, `ready`, `change_pending`, `cancelled`; por línea,
  desde `pending_release`. Transferir/dividir/combinar: el agregado es la cuenta de origen (la de destino, por su
  revisión en `restaurant/state`).
- Cuenta que ya no está abierta (en `saveDraft`, `previewDraft`, `confirmDraft`, `cancelUnits`, `previewEdit`,
  `editUnits`): 400 `OPERATION_VALIDATION` con `error.details = {"reason":"account_not_open","account_id",
"account_state":"closed|voided|merged","merged_into_id"}` → «Requiere tu revisión» con `error.message`.
- `POST /api/v1/restaurant/offline-orders` con `destination.account_id` y `"replaces_draft_items": [DraftItem…]` (los
  renglones que el dispositivo guardó en línea, con el contenido del último `saveDraft` aceptado) → 200 con
  `removed_draft_item_ids` y `bill.draft` restante; o 409 `ORDER_REQUIRES_CONFIRMATION` con cambios
  `draft_item_changed` / `draft_item_removed` / `draft_item_sent` (`acceptable: true`, `item_id` = id del renglón,
  `before`/`now` en `null`, `message` para mostrar), que se confirman como el precio (misma `operation_id`,
  `confirmation: {catalog_version, changes}`). `account_closed` trae `now` = `closed|voided|merged`.
- `GET /api/v1/restaurant/state` → `event_cursor`: abrir `GET /api/v1/events?after=<event_cursor>`.

### Pruebas ejecutadas

| Qué                                                                                     | Dónde                                        | Resultado                                |
| --------------------------------------------------------------------------------------- | -------------------------------------------- | ---------------------------------------- |
| `go-unit` (vet normal e integración, `test -race`, `GOOS=windows` build y vet)          | `iso.sh go-unit`, al final                   | **Aprobado**                             |
| `TestStageA1WaiterMode` (6 subpruebas), sola y con `-v`                                 | `go-int ./internal/operations`, base propia  | **ok** (18 s; las 6 subpruebas PASS)     |
| `go-int ./internal/apicontract` (`TestContractV1` con `contractWaiterMode` y unitarias) | base `adel_pos_test_e_*`                     | **ok** (14 s)                            |
| `go-int ./internal/database` (`TestStageA1Upgrade` con `checkStageA1DraftItems`)        | idem                                         | **ok** (15 s)                            |
| `go-int ./internal/kitchen`                                                             | idem (el paquete no tiene pruebas con base)  | **ok** (4 s)                             |
| `go-int ./internal/administration` (por `recovery.go`)                                  | idem                                         | **ok** (174 s)                           |
| `go-int ./internal/operations` completo (con las de cocina, programación y 9G)          | idem                                         | **ok** (359 s)                           |
| `TestClientMinimumVersionByKind` con `waiter-mode-v1`                                   | `internal/httpapi` (unitarias, en `go-unit`) | Aprobada                                 |
| `format`, `fmt-check` (Prettier, gofmt, `cargo fmt --check`), `sync-check`              | árbol `Es`                                   | Correctos                                |
| md5 de `docs/etapas/*.md`                                                               | contra `$SP/etapas.md5`, al final            | Idénticos                                |
| Vitest, `build`, Playwright                                                             | —                                            | No se corrieron: el escritorio no cambió |

`TestStageA1WaiterMode` (`backend/internal/operations/stagea1_waiter_test.go`, base propia) cubre: el mesero (sin
permisos de cocina ni de para llevar; `kitchen/board` 403) confirma dos partidas → **un** evento de preparación, en la
misma transacción que `restaurant.order.confirmed` (misma `created_at`); cocina avanza 4 tareas de 2 cuentas en una
operación → **un evento por cuenta**; el mesero lo recibe **por el SSE** y en su lectura del canal no hay ningún evento
de cocina; la cuenta y la línea dicen `preparing`; «Listo» de una cuenta avisa solo a esa; una cuenta para llevar avisa
en `takeaway` y el mesero no la ve. Cuenta descartada (`voided`: guardar y vista previa), combinada (`merged` con
`merged_into_id`: confirmar, cancelar, vista previa de modificar y modificar) y cobrada (`closed`, simulada en la base:
guardar y `account_closed` con `now` en el pedido sin conexión), siempre 400 `OPERATION_VALIDATION`. Pedido sin conexión
que sustituye dos renglones propios (uno cambiado sin conexión y otro quitado): quita esos dos, deja intacto el de la
otra terminal, conserva `draft_item_id`, publica historia, evento y auditoría del borrador una vez; reintento sin
duplicar ni tocar el borrador; misma clave con otro contenido → `OFFLINE_ORDER_ALREADY_ACCEPTED`. Renglón cambiado,
quitado y ya enviado desde otra terminal → 409 con los tres cambios aceptables y sin escribir nada; confirmación con la
misma clave → entra completo y el renglón de la otra terminal se queda; ya enviado desde el mismo dispositivo → su
mensaje. Con la **credencial de envío** (tableta `android`, PIN, sin sesión): quita el renglón y el reintento devuelve lo
mismo; un solo pedido a nombre del mesero y de la tableta. Forma inválida (mesa, repetidos, 21 renglones, cantidad 0)
→ 400. `event_cursor` = último evento al leer; desde él no queda nada pendiente y llega lo posterior.

`TestContractV1` agrega `contractWaiterMode`: `getRestaurantOverview`, `previewEdit` + `editUnits` (solo observaciones),
`cancelUnits`, `saveDraft` sobre una cuenta descartada (400 con `details` validado contra `AccountNotOpenDetails`) y un
pedido sin conexión con `replaces_draft_items` (409 `draft_item_changed`, confirmación y 200 con
`removed_draft_item_ids`); además que el primer mensaje del canal trae el evento de preparación de la cuenta confirmada y
que `restaurant/state` trae `event_cursor`.

### No verificado

- La aplicación Android real contra estas rutas (fase E del dispositivo).
- La liberación automática (`ReleaseDue`) y `retryChange` publicando el evento: el código está, pero la prueba nueva solo
  ejercita el avance de estados, la confirmación y la cuenta para llevar (las pruebas de programación y de avisos
  pendientes existentes siguen pasando).
- El cierre real por cobro antes del rechazo (`closed` se simuló en la base; el rechazo es el mismo `ensureOpen`).
- Playwright y Vitest: el escritorio no cambió (análisis de `useRealtime.ts` en lugar de prueba).
- `go-int` de los demás paquetes de la tupla (catalog, payments, reports, designer, identity, backupschedule,
  `cmd/ad-el-pos-server`, discovery, legacyimport): no se tocaron; quedan para la Fase G.

### Para el coordinador

- **Sin decisiones de negocio cambiadas.** Dos lecturas del encargo que conviene confirmar: el ámbito `takeaway` para
  el evento de una cuenta para llevar (decisión 5) y que aceptar `draft_item_changed` deja el renglón de la otra
  terminal en el borrador (decisión 2).
- Anotado, sin hacerlo (no indispensable para §6–§7):
  - en transferir, dividir o combinar el evento solo lleva la cuenta de **origen**; la app debe reconocer la de destino
    por su revisión en `restaurant/state` (documentado). Un evento también para la de destino sería un cambio pequeño
    en `operations/queries.go` si el agente de Android lo pide;
  - exponer `draft_item_id` en `ConfirmedLine` (ayudaría a la app a reconciliar sus renglones con lo enviado tras una
    respuesta perdida; hoy basta reintentar con la misma `operation_id`);
  - `POST …/draft-preview` (cotizar el borrador sin guardarlo) existe y no está en el contrato; la app puede guardar y
    usar `previewDraft`.
- Para `CLAUDE.md` (no lo edité): §6, prueba nueva `TestStageA1WaiterMode` (`operations`), `contractWaiterMode` en
  `TestContractV1` y `checkStageA1DraftItems` en `TestStageA1Upgrade`; §15, A1-D11 implementada (regla: todo cambio de
  los registros de cocina de una cuenta publica `restaurant.account.preparation_changed` una vez por transacción desde
  `kitchen/preparation.go`) y A1-D6 (`replaces_draft_items` y `restaurant_items.draft_item_id`); la capacidad
  `waiter-mode-v1`.
- `AGENTS.md`: quien agregue un escritor nuevo de `kitchen_tasks`, `kitchen_releases`, `kitchen_unit_releases` o
  `kitchen_pending_changes` debe anotar la cuenta (`notePreparationAccount`/`notePreparationUnits`) y publicar al final.

**Decisiones del coordinador al cerrar la Fase E del servidor** (rama `etapa-a1-es` fusionada en `etapa-a1-android`, `da9c5e2`, sin conflictos: no comparte archivos con la Fase A):

- Se acepta publicar el evento de preparación de una cuenta para llevar en el ámbito `takeaway` (regla de todos los eventos de una cuenta; no cambia nada para el mesero de salón).
- Se acepta que confirmar `draft_item_changed` conserve en el borrador el renglón que cambió otra terminal (nunca se borra en silencio lo ajeno) y que el mensaje lo diga.
- Lo anotado sin hacer (evento de la cuenta de destino al transferir, `draft_item_id` en `ConfirmedLine`, `draft-preview` en el contrato) se hace solo si el agente de Android de la Fase E lo necesita.

---

## Fase E · Modo Mesero en el dispositivo

Rama `etapa-a1-e` (desde `etapa-a1-android` en `b4e1cfe`), un agente, 16 y 17 de noviembre de 2026, en paralelo con las
correcciones de la revisión R3 en el dispositivo (otro agente). Árbol `$SP/wt/E`, `ISO_TAG=E2`, Go temporal en
8197/8457 (uno nuevo por corrida completa contra el servidor; apagado y su base borrada al terminar), emuladores propios
de solo lectura en las consolas 5600 (`Pixel_Tablet`), 5602 (`Pixel_8`), 5604 (`ADEL_A1_Tablet_API26`) y 5606
(`ADEL_A1_Phone_API33`), como mucho dos a la vez y siempre con `ANDROID_SERIAL`. A mitad de la fase se fusionó
`etapa-a1-android` con las correcciones R3 del dispositivo (fusión `9758899`, sin rebase) y la fase se conectó con ellas
(cabeceras por mesero, «Vuelve a escribir tu PIN», registro único de borrado, Room v3 → la cola va en la **v4**).
**Sin cambios en `backend/`, `desktop/`, `config/`, `CLAUDE.md` ni `AGENTS.md`**; `docs/etapas/*.md` con el mismo md5
antes y después. Ninguna prueba usó el servicio real de la Mac (8181, 8443, 55432 ni su carpeta) y ninguna abrió la
aplicación normal con la búsqueda en la red real (todas con `AdelTestRunner`).

### Commits (`etapa-a1-e`, 19 sobre `etapa-a1-android` en `6a1c496`)

| Commit    | Qué                                                                                                                           |
| --------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `c9731fe` | Dominio del modo Mesero (mesas, tiempo, cuentas, permisos, borrador, captura, A1-D6, cola con cinco estados)                  |
| `9758899` | Fusión de `etapa-a1-android` con las correcciones R3 del dispositivo (sin rebase)                                             |
| `5cd1d89` | Cola en `core:data`: Room v4 y migración 3 → 4, API con cabeceras del mesero dueño, envío, revisión, WorkManager 2.12.0       |
| `64a9d46` | Captura en `core:data` (borrador en Room y en Go), mesas y cuentas sin conexión, modificar o cancelar lo enviado              |
| `7704357` | La aplicación conecta la cola (`WaiterWorkGate`, `UnsentOrdersSource`, contador, registro de borrado) y WorkManager           |
| `b0bdacc` | Pantallas: Mesas (lista y mapa), cuentas, captura, lo enviado, «Enviar a cocina», pendientes y revisión                       |
| `0b12d8a` | Comentario de WorkManager en una línea                                                                                        |
| `a0aafa6` | Pruebas de la aplicación con las etiquetas de Mesas en su paquete nuevo                                                       |
| `13a4115` | Prueba del bloqueo por inactividad con la cola real                                                                           |
| `5f562be` | El trabajador lee el servidor emparejado sin pantallas; reloj de Mesas solo a la vista; WorkManager en `docs/DEPENDENCIES.md` |
| `f70e65e` | Pruebas: ViewModels (JVM), pantallas en el dispositivo, `WaiterModeOnGoTest` y `android-e2e.sh --suite waiter`                |
| `ffc0b42` | `android/CLAUDE.md`, prueba de la red sin internet con WorkManager, giro y Atrás robustos en la prueba de punta a punta       |
| `9b7d33c` | «Actualiza esta aplicación» muestra los pedidos sin enviar                                                                    |
| `fc8bdf2` | Más pruebas (mesa cobrada, cancelar/modificar lo enviado, cuenta cobrada sin conexión, bloqueo con pedido, teléfono)          |
| `1efc7d3` | Una cuenta cobrada que llega por el canal no es un conflicto: se ofrece una cuenta nueva en la mesa                           |
| `4797131` | Formato de las tablas de `android/CLAUDE.md` y `docs/DEPENDENCIES.md`                                                         |
| `02fc45d` | `WaiterModeOnGoTest` reconoce el panel por sus botones (teléfono)                                                             |
| `cc5ae19` | `WaiterModeOnGoTest` acepta Mesas con la mesa abierta (teléfono)                                                              |
| `735a97a` | `WaiterModeOnGoTest` desplaza las listas perezosas hasta lo que busca (pantalla chica de API 26)                              |

### Qué se hizo

**Dominio** (`android/core/domain/.../waiter/`, Kotlin puro):

- `Restaurant.kt`: áreas, mesas (forma y posición en el lienzo lógico de 1000 × 560, `TableCanvas.scaleToFit`), umbrales,
  `RestaurantView` (áreas activas en su orden; mesas activas o con cuentas abiertas, como `operationalTables` del
  escritorio) y `TableSearch` (sin acentos ni mayúsculas). `ElapsedTime`: el formato exacto de `elapsedTime` del
  escritorio («45 min», «5 h 12 min», «3 d 14 h») y los segundos de Go más lo que pasó desde la lectura con el reloj
  monótono.
- `Accounts.kt`: `AccountSummary`, `DraftLine`, `SentLine`, `AccountBill`, `DraftQuote`, `PreparationState` (los nombres
  de Go; un valor desconocido no se inventa).
- `WaiterPermissions.kt`: cada acción con los permisos que exige Go (abrir cuenta, capturar, enviar, modificar, cancelar);
  solo deshabilita lo que Go rechazaría y, si nunca se leyeron (entrada sin conexión), no deshabilita nada.
- `DraftOperations.kt`: puerto de `draftOperations.ts` (agregar, sumar con respaldo, sustituir, quitar; lo sencillo
  idéntico se suma, lo personalizado nunca).
- `Capture.kt`: la captura por mesero (base de Go + toques locales, renglones propios, precios vistos, estado del
  guardado, clave del guardado en curso y cuántos toques incluye) y `OfflineOrderPlan` (A1-D6).
- `OrderQueue.kt`, `OrderSendRules.kt`, `OrderReviewRules.kt`, `OrderFactory.kt`: los **cinco estados** (`DRAFT`,
  `QUEUED`, `SENDING`, `RECEIVED`, `NEEDS_REVIEW`) y `OrderTransitions` (las permitidas y nada más), el pedido de la
  cola, la interpretación de cada respuesta, «Requiere tu revisión» (aceptar, quitar, reenviar, cuenta nueva en la
  mesa) y `WaiterWorkSummary` (lo que decide bloquear, cambiar de mesero y cerrar sesión).
- `WaiterServices.kt`: interfaces para las pantallas (`RestaurantSource`, `CaptureService`, `OrderQueueService`,
  `SentLineService`), para que la A2 las reutilice.

**Datos** (`android/core/data/.../waiter/`):

- `WaiterDtos.kt` y `WaiterMapper.kt`: las formas del contrato (overview, state, cuentas, `Bill`, `QuoteResult`, abrir,
  guardar, confirmar, cancelar, modificar, pedido sin conexión, rechazos con `details`). Al escribir, `CustomizationJson`
  (`"variant_id": null`).
- `WaiterServer` / `DeviceWaiterServer`: interfaz estrecha sobre `AccessRepository` con las cabeceras **del mesero
  dueño** (`requestHeaders(userId)`, solo si trae `Authorization`), la credencial de envío (`X-Offline-Credential`) y
  `report(error, userId)`; `prepare()` lee el servidor emparejado si WorkManager despertó el proceso sin pantallas.
- `WaiterApi`: las rutas del modo Mesero con `ApiOutcome` (rechazo con `details`, «la petición salió»); con
  `LostResponses` (solo pruebas) descarta la respuesta **después** de que Go la procesó.
- Room **`AppDatabase` v4** (`waiter/db/`): `captures`, `pending_orders`, `restaurant_snapshot` y `waiter_permissions`;
  migración 3 → 4 con el SQL del esquema exportado (`WAITER_TABLES_V4`); `queueFromCapture` y `captureFromOrder` en una
  transacción. `WaiterStorage` guarda los renglones con la forma `DraftItem` del contrato.
- `OrderQueueRepository`: la cola (servicio de las pantallas), `PendingOrdersSource` (contador), `WaiterWorkGate`,
  `UnsentOrdersSource` (también las capturas sin enviar), `recoverAfterRestart`, `credentialRenewed` y `eraseAll`.
- `OrderSender` + `OrderSendWorker`/`OrderWorkerFactory`/`WorkManagerOrderScheduler`: envío del más viejo al más nuevo
  con la credencial del mesero que capturó, la misma clave siempre; WorkManager **sin restricción de red**, espera
  exponencial desde 10 s, trabajo normal (no urgente), periódico de respaldo cada 15 min, «otra vuelta» si llega un
  pedido mientras se envía.
- `CaptureRepository`: el guardado del borrador como `draftSync.ts` (700 ms, una petición a la vez, creación diferida,
  `TABLE_HAS_ACCOUNT`, guardado incierto repetido igual también tras reiniciar, `ACCOUNT_CONFLICT` explícito, cuenta
  que ya no está abierta), la cotización de Go (huella y precio por renglón) y «Enviar a cocina» (en línea con
  `confirmDraft` y la huella vista; si no se puede, pedido sin conexión).
- `RestaurantRepository`: mesas, cuentas abiertas y permisos con la sesión del mesero activo, guardados para verse sin
  conexión; el canal de tiempo real (ámbito `restaurant`) avisa qué cuenta cambió y vuelve a leer las mesas.
- `SentLineRepository`: `previewEdit`, `editUnits` y `cancelUnits` con motivo y clave propia.

**Interfaz** (`core:ui/.../waiter/` y `feature:waiter`):

- `OrderStateBadge` (los cinco estados con color, texto e ícono distintos), `PreparationBadge` (los textos del escritorio
  y de cocina) y `ElapsedLabel` (el tiempo, en ámbar con el triángulo si Go marcó `long_open`), compartidos con la A2.
- **Mesas** (`tables/`): lista por área (predeterminada) o **mapa** a escala por área, recordada en el dispositivo;
  búsqueda; «Sin conexión: se ven las mesas de la última vez»; el panel de la mesa con lo del mesero en ella (captura y
  pedidos con su estado), las cuentas abiertas y «Nueva cuenta» (deshabilitada con su motivo sin `accounts.open` o si la
  mesa está fuera de servicio).
- **Captura** (`capture/`): catálogo y personalizador de la Fase D con la cuenta **al lado en la tableta y a un toque en
  el teléfono**; lo enviado con su preparación y «Modificar»/«Cancelar unidades» (con motivo, solo con conexión y los
  permisos); lo por enviar con −/+, «Cambiar» y «Quitar», «Capturado en otra terminal», precios de Go o «Precio de
  referencia»; el estado del guardado con «Conservar mis cambios»/«Descartar mis cambios», «Agregar a esa cuenta»/«Abrir
  otra cuenta» y «Abrir una cuenta nueva en la mesa»; «Enviar a cocina» y «Descartar la captura» (con confirmación).
- **Pedidos pendientes** y **Requiere tu revisión** (`pending/`): lo del mesero con su estado, «Reintentar ya», el aviso
  de la credencial, el número (nunca el detalle) de los pedidos de otros meseros; la revisión con los mensajes de Go,
  «Quitar del pedido», «Aceptar los cambios y enviar» (solo con cambios aceptables), «Abrir una cuenta nueva en la
  mesa», «Corregir en la captura» y «Descartar el pedido».
- `feature:access`: «Actualiza esta aplicación» muestra los pedidos sin enviar con su aviso (salen al instalar encima;
  desinstalar los perdería) — lo pendiente de las fases C y B; «Emparejar de nuevo» y la revocación ya los mostraban con
  `pendingOrders()`, ahora con la cola real.

**Aplicación**: `WaiterModeContainer` (arranque: recuperar la cola, envío al volver la red, a primer plano y cuando un
mesero entra con conexión), `AppContainer` (cola, contador, `WaiterWorkGate`, `UnsentOrdersSource`, registro de borrado
«cola de pedidos y capturas» con `WorkManager.cancelAllWork()`, `waiterDependencies`), `AdelPosApplication` como
`Configuration.Provider` (el inicializador automático de WorkManager quitado en el manifiesto) y la navegación (Mesas →
Captura, Pedidos pendientes, Requiere tu revisión). La pantalla de muestra «Catálogo» de la Fase D queda sin ruta.

**Dónde está lo principal** (rutas desde `android/`; `…/domain` = `core/domain/src/main/kotlin/local/adel/pos/android/core/domain`,
`…/data` = `core/data/src/main/kotlin/local/adel/pos/android/core/data`):

| Qué                                                                       | Ruta:línea                                                                                         |
| ------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Transiciones permitidas de los cinco estados                              | `…/domain/waiter/OrderQueue.kt:47`                                                                 |
| Lo que se manda sin conexión (A1-D6, `replaces_draft_items`)              | `…/domain/waiter/Capture.kt:115`                                                                   |
| Room v4 y la migración 3 → 4                                              | `…/data/db/AppDatabase.kt:45`, `:145`; SQL en `…/data/waiter/db/WaiterEntities.kt:98`              |
| Envío de la cola e interpretación de cada respuesta                       | `…/data/waiter/OrderSender.kt:53`, `:115` (`OFFLINE_CREDENTIAL_INVALID` en `:153`)                 |
| WorkManager sin restricción de red, espera exponencial y periódico        | `…/data/waiter/OrderSendWork.kt:126`, `:129`, `:139`                                               |
| Cola, `WaiterWorkGate`, `UnsentOrdersSource`, recuperación tras reiniciar | `…/data/waiter/OrderQueueRepository.kt:151`, `:157`, `:120`                                        |
| Guardado del borrador (700 ms), cuenta cerrada, «Enviar a cocina»         | `…/data/waiter/CaptureRepository.kt:404`, `:270`, `:308`                                           |
| Cabeceras del mesero dueño y credencial de envío                          | `…/data/waiter/WaiterServer.kt:67`, `:77`                                                          |
| Registro de borrado de la cola                                            | `app/src/main/kotlin/local/adel/pos/android/AppContainer.kt:141`                                   |
| «Actualiza esta aplicación» con los pedidos sin enviar                    | `feature/access/src/main/kotlin/local/adel/pos/android/feature/access/status/StatusScreens.kt:191` |
| Prueba de punta a punta contra Go                                         | `app/src/androidTest/kotlin/local/adel/pos/android/WaiterModeOnGoTest.kt`                          |

### Decisiones (con su porqué)

1. **La cola y las capturas en `AppDatabase` v4** (instrucción del coordinador al fusionar R3, que ya usó la v3): una
   sola base para el acceso, la bitácora y la cola; migración 3 → 4 que solo crea tablas, probada con
   `MigrationTestHelper`.
2. **«Enviar a cocina» siempre deja un pedido en la cola** (`queueFromCapture`, en una transacción con el borrado de la
   captura): con conexión nace «Enviando» e intenta `confirmDraft` con la huella de la cotización que vio el mesero (el
   flujo del escritorio); cualquier cosa que no sea «confirmado», «la cuenta ya no está abierta» o «sin permiso» (sin
   respuesta, `QUOTE_CHANGED`, agotado, conflicto, sesión terminada) lo deja como **pedido sin conexión** y Go lo revisa
   todo al recibirlo con sus propios mensajes (precio, agotado, `draft_item_*` de A1-D6). Así el trabajador de
   WorkManager, que solo tiene la credencial de envío (las sesiones viven en memoria), puede terminar cualquier envío, y
   un reinicio a media confirmación nunca deja un pedido perdido («Enviando» vuelve a «Por enviar»). Si la confirmación
   en línea sí había entrado y la respuesta se perdió dos veces, Go lo dice como `draft_item_sent` («ya se había enviado
   desde este dispositivo») y el mesero lo descarta: nunca entra dos veces sin que el mesero lo acepte.
3. **La clave del pedido es el id de la captura**: «Corregir en la captura» y volver a enviar usa la misma clave (Go no
   guarda la clave de un rechazo); la confirmación en línea tiene su propia clave.
4. **`seen_unit_price`**: el precio por unidad de la cotización de Go si el renglón se guardó en línea; si no, el de
   referencia del catálogo (presentación o producto). **Nunca se calcula uno**: un renglón con complementos con cargo
   capturado sin conexión llega con el precio base y Go lo avisa como cambio de precio que el mesero acepta.
5. **WorkManager sin restricción de red** (riesgo de la Fase 0 **comprobado**: con la red sin validar,
   `NetworkType.CONNECTED` no corre y el trabajo sin restricción sí, `WorkNetworkOnDeviceTest`): el trabajador prueba
   el servidor y reintenta con espera exponencial desde 10 s; además se pide un envío al recuperar la red, al volver a
   primer plano y cuando un mesero entra con conexión, y un trabajo periódico de 15 minutos cubre la aplicación cerrada.
   Nunca trabajo urgente.
6. **Una base de Go sin leer nunca se sobrescribe**: `saveDraft` sustituye el borrador completo, así que una captura que
   empezó sin conexión en una cuenta existente (`baseRevision` 0) primero lee el borrador vigente y guarda encima (sus
   toques nunca vieron otro); con base leída, una revisión nueva con toques pendientes es un conflicto explícito, como
   el escritorio.
7. **Una cuenta cobrada, cancelada o combinada no es un conflicto**: si llega por el canal (o Go rechaza el guardado con
   `account_not_open`), la captura dice que la cuenta ya no está abierta y ofrece «Abrir una cuenta nueva en la mesa»
   (con renglones nuevos: los anteriores pertenecían a la otra cuenta). Observado en el Go temporal: **Go no cierra una
   cuenta de salón liquidada mientras tiene borrador**, así que «la mesa se cobró mientras tanto» ocurre cuando lo del
   mesero aún no estaba guardado en Go (sin conexión o recién tocado); en línea lo cubre la captura y sin conexión la
   revisión (`account_closed` con «Abrir una cuenta nueva en la mesa»).
8. **El contador del indicador es de todo el dispositivo** (lo que importa antes de apagarlo); «Pedidos pendientes»
   muestra solo los del mesero activo y, de los demás, solo cuántos son.
9. **Las capturas sin enviar también pasan a la bitácora** al retirar el dispositivo o emparejarlo de nuevo («(sin enviar
   a cocina)»): nunca se pierden en silencio.
10. **Modificar lo enviado ofrece cantidad y observaciones** (con `previewEdit` primero y motivo); reducir exige además
    cancelar (el diálogo no deja bajar sin ese permiso). Cambiar la receta o los complementos de algo ya enviado no se
    ofrece en esta fase (Go lo admite con `previewEdit`; queda para después si el propietario lo pide).
11. **Mesas**: todas las mesas se muestran (Go no restringe por mesero); el tiempo es el de Go más el reloj monótono
    desde la lectura; la última lectura queda en Room para trabajar sin conexión (también tras reiniciar).
12. **Sin cambios en el servidor**: lo opcional de la Fase E del servidor no hizo falta (la cuenta de destino de una
    transferencia se ve porque cada evento del ámbito `restaurant` vuelve a leer mesas y cuentas; `draft_item_id` en
    `ConfirmedLine` no se necesitó porque el reintento usa la misma clave; `previewDraft` basta).

### Pruebas que se corrieron (todas en `735a97a`, la cabeza de la rama)

| Prueba                                                                                           | Dónde                                                       | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| ------------------------------------------------------------------------------------------------ | ----------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `./scripts/dev.sh android-test` (JVM, Robolectric con Room y WorkManager, lint)                  | Mac                                                         | **219 pruebas, 0 fallas**, lint sin hallazgos: `core:domain` 94 (nuevas `WaiterRulesTest` 10 y `OrderQueueRulesTest` 10), `core:data` 107 (nuevas `OrderQueueTest` 13 y `CaptureSyncTest` 10; `AppDatabaseTest` en la v4), `core:ui` 9, `feature:access` 6, `feature:waiter` 3 (`WaiterViewModelsTest`)                                                                                                                                                                                                                                                                                                                                                                          |
| `./scripts/dev.sh android-format-check`, `iso.sh format` y `fmt-check` antes de cada commit      | Mac                                                         | correcto; `docs/etapas/*.md` con el mismo md5 antes y después                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| Instrumentadas completas (`connectedDebugAndroidTest`), XML contados                             | `Pixel_8` (API 37)                                          | **71 pruebas, 0 fallas**, 22 omitidas a propósito (las de punta a punta sin los argumentos del guion: `AccessOnGoTest` 7, `CatalogOnGoTest` 3, `WaiterModeOnGoTest` 10, `ServerApiOnDeviceTest` 2). Incluye `AppDatabaseMigrationTest` 3 → 4 con `MigrationTestHelper`, `WaiterScreensTest` 4 (los cinco estados distintos en ambos temas, «Pedidos pendientes» con «Reintentar ya» y solo el número de los de otros meseros, la revisión que solo se acepta con cambios aceptables, Mesas en uno o dos paneles y sin permiso para abrir), `WorkNetworkOnDeviceTest` (riesgo de la Fase 0) e `IdleLockWithPendingOnDeviceTest` (no bloquea con un pedido por enviar y sí sin él) |
| Ídem                                                                                             | `Pixel_Tablet` (API 37)                                     | **71 pruebas, 0 fallas**, las mismas 22 omitidas                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `scripts/android-e2e.sh --suite waiter`, 10 pasos contra el Go temporal (base nueva por corrida) | `ADEL_A1_Tablet_API26` (red cortada con `adb root` + `svc`) | **10/10 aprobados**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Ídem                                                                                             | `ADEL_A1_Phone_API33` (modo avión)                          | **10/10 aprobados**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Ídem                                                                                             | `Pixel_8` (API 37)                                          | **10/10 aprobados**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Ídem                                                                                             | `Pixel_Tablet` (API 37)                                     | **10/10 aprobados**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |

Los pasos de `WaiterModeOnGoTest`: `w1` emparejar y entrar con PIN; `w2` captura en línea con creación diferida de la
cuenta, giro sin perder nada, «Enviar a cocina» (`confirmDraft` con la huella) y los cambios de otra terminal al momento
por el canal; `w3` captura **sin conexión** (red cortada por el guion) que queda «Por enviar», con «1 pedido por enviar» en el indicador de la barra (la cola real); `w4` al volver la red el
trabajador lo envía **sin entrar con el PIN**, con la primera respuesta **perdida** (`lostOfflineResponses`): Go lo tiene
una sola vez; `w5` precio cambiado y producto agotado: «Requiere tu revisión», quitar el agotado y aceptar; `w6` la mesa
se cobró mientras se capturaba: «Abrir una cuenta nueva en la mesa»; `w7a`–`w7c` A1-D6 (borrador guardado en Go, sin
conexión se agrega y se cambia, el pedido sustituye el borrador sin duplicar); `w8` cancelar y modificar unidades
enviadas con motivo y permisos. En la primera corrida de API 26, `w2` no encontró «Enviar a cocina» (la lista del pedido
no lo había compuesto en esa pantalla) y `w6` falló por arrastre; la prueba ahora desplaza las listas (`735a97a`) y las
cuatro corridas de arriba son posteriores a ese cambio.

Dependencias: WorkManager 2.12.0 (`work-runtime-ktx` y `work-testing`) y lo que trae (`lifecycle-livedata` y
`lifecycle-service`, que resuelven a 2.10.0 en `core:data` y a 2.11.0 —la del catálogo— en la aplicación;
`concurrent-futures-ktx` 1.1.0, `tracing-ktx` 1.2.0, `startup-runtime` 1.1.1 en la compilación de `core:data`), con el
procedimiento documentado (`--write-locks --write-verification-metadata sha256`; se descargaron de Google Maven y sus
SHA-1 se compararon con los publicados); la corrida completa siguiente no cambió nada. No se instaló ningún paquete del
SDK.

### No verificado

- Los estados de cocina «En preparación» y «Listo» de punta a punta: el Go temporal de las pruebas no tenía estaciones
  ni rutas de cocina, así que lo enviado quedó «Enviado · sin confirmar en cocina» (el estado que da Go). La lectura de
  `preparation_state` y la nueva lectura de la cuenta con cualquier evento de su agregado (incluido
  `restaurant.account.preparation_changed`) sí se ejercitaron (el borrador y la confirmación de otra terminal llegaron al
  momento en `w2`).
- Un dispositivo físico y la wifi real de un restaurante sin internet (se simuló una red sin validar haciendo fallar la
  comprobación de internet del sistema en el emulador).
- La aplicación cerrada mucho tiempo con Doze (el trabajo periódico de 15 minutos y la espera exponencial larga de
  WorkManager no se observaron; sí el envío con el proceso nuevo y sin entrar con el PIN, `w4`).
- Varios meseros a la vez en el mismo dispositivo contra el Go temporal (la separación por mesero se probó en la JVM y en
  `WaiterScreensTest`; el cambio de mesero y su bloqueo con pedidos ya los prueban las fases C y B con
  `WaiterWorkGate`).
- Transferir, dividir o combinar cuentas desde otra terminal mientras se captura.
- La personalización en curso después de que Android cierra el proceso (vuelve el producto con su configuración
  original; la rotación sí la conserva).
- Lint con reglas de lifecycle 2.11.0 (las omite el lint de AGP 9.5.1, como en las fases anteriores).

### Para el coordinador

1. **Sin cambios en el servidor** ni pedidos para él (decisión 12).
2. **Cambio pequeño en `feature:access`** (del agente de las fases C y B, ya fusionado): «Actualiza esta aplicación» ahora
   muestra los pedidos sin enviar con su aviso (lo pendiente de las fases C y B); nada más de su código cambió. La forma
   del contrato R3 (`requestHeaders(userId)`, `report`, `deviceDataWipe`, `pendingOrders()`) sirvió tal cual.
3. **`scripts/android-e2e.sh --suite waiter`**: el guion de las fases C y B ahora corre también `WaiterModeOnGoTest`
   (pasos `w1`–`w8`, la red la corta el guion); `--suite access` (por omisión) no cambia.
4. **Riesgo de la Fase 0 confirmado** (decisión 5): conviene que la Fase F lo repita en un dispositivo físico con una
   wifi sin internet.
5. Para `CLAUDE.md`/`AGENTS.md` (no los edité): §4 y §6 con la cola (`AppDatabase` v4), las pruebas nuevas y
   `--suite waiter`; reglas permanentes nuevas 18 y 19 de `android/CLAUDE.md` (la cola nunca pierde ni duplica; WorkManager
   sin restricción de red).
6. La pantalla de muestra «Catálogo» de la Fase D quedó sin ruta (su prueba sigue): se puede borrar en la Fase G.

**Al cerrar la Fase E del dispositivo** (rama `etapa-a1-e`, que ya traía fusionadas las correcciones R3, fusionada en
`etapa-a1-android`, `f6e6b39`): se aceptan las decisiones de la fase (cola en `AppDatabase` v4; «Enviar a cocina»
siempre pasa por la cola; WorkManager sin restricción de red, porque se comprobó que con `CONNECTED` y una red sin
validar el trabajo no corre; el contador del indicador cuenta los pedidos de todo el dispositivo; «Abrir una cuenta
nueva en la mesa» cuando la cuenta ya se cobró). La revisión independiente de §6–§7 examina en especial que la misma
clave de operación pase de `confirmDraft` a `submitOfflineOrder` sin duplicar ni perder pedidos. La Fase F completa lo
no verificado (estados de preparación con cocina de punta a punta, varios meseros a la vez, transferir o combinar desde
otra terminal y la muerte del proceso a mitad de un personalizador) y la Fase G borra la pantalla de muestra «Catálogo».

---

## Revisión independiente R3 · PIN y sesión (§3)

Revisor independiente que pide la especificación (§0, «Eficiencia»), 16/11/2026. Árbol `$SP/wt/R3` (rama `etapa-a1-r3`,
igual a `etapa-a1-android` en `f7c20c7`), `ISO_TAG=R`. **Solo lectura del código**: no se corrigió nada y el árbol quedó
sin cambios. Las pruebas propias viven en una copia exportada con `git archive` en `$SP/r3/tree` (nunca en el
repositorio): `R3ReviewTest.kt` y `R3IdleLockTest.kt` (Android, JVM) y `pin_r3_review_test.go` (Go, integración). Sus
resultados están en `$SP/r3/results/`, `$SP/r3/gradle-r3test.log`, `$SP/r3/gradle-r3idle.log`, `$SP/r3/go-r3test.log` y
`$SP/r3/go-r3-out.txt`. No se abrió ningún emulador ni Go temporal (`server-up`): la prueba de Go usó el clúster temporal
55499 con una base `adel_pos_test_r_*` que `iso.sh` borró al terminar. No se tocó la instalación real, `docs/etapas/` ni
los emuladores 5600–5606 de la Fase D. El demonio de Gradle no se detuvo porque lo comparte el agente de la Fase D.

Marco: §0-B, §3, §4 y §7 de la especificación; A1-D1 a A1-D5, A1-D13 y A1-D15 del avance (usados para entender la
intención, no como prueba).

### Resumen

El servidor está bien: hash Argon2id con sal igual al de las contraseñas, PIN triviales rechazados en las dos rutas que
definen un PIN, bloqueo por usuario que no se esquiva con peticiones simultáneas (comprobado en vivo), credencial de
envío atada a usuario y dispositivo, solo en `POST restaurant/offline-orders` (también en el SSE) y revisada en cada uso,
y la migración 0025 sección B correcta. Los defectos importantes están en el **dispositivo**: una entrada sin conexión
reutiliza la sesión de Go que el mesero dejó en memoria (A1), el bloqueo por inactividad se reinicia al volver a abrir
la actividad (A2) y el límite local de intentos se esquiva con el reloj o con el modo avión (M1, M2). Las dos primeras
y las dos siguientes se reprodujeron con pruebas.

### Hallazgos

| ID     | Gravedad             | Archivo y línea                                                                                                                                                                                                                                             | Qué está mal                                                                                                                                                                                                                                                                                                                                                                                                         | Escenario (entrada → resultado incorrecto)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Cómo se comprobó                                                                                                                                                                                                                                         | Corrección sugerida                                                                                                                                                                                                                                                                                                                                                       |
| ------ | -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **A1** | **alto**             | `android/core/data/.../access/AccessRepository.kt:215-219` (`requestHeaders`) y `:528-529` (`enterOfflineLocked`); contradice `android/CLAUDE.md:147`                                                                                                       | Una entrada **sin conexión** (solo con el verificador local) deja activo al mesero con `hasServerSession = false`, pero `requestHeaders()` y `sessionToken()` siguen entregando la sesión de Go que ese mesero dejó en memoria en una entrada anterior con conexión                                                                                                                                                  | Ana entra con conexión (sesión S en memoria) y cambia de mesero o se bloquea; otro mesero pone el modo avión, elige a Ana en la lista sin conexión y adivina su PIN contra el verificador local (M1, M2); quita el modo avión → la barra dice «Entraste sin conexión», pero toda petición de las fases D/E con `requestHeaders()` lleva `Authorization: Bearer S`: sesión completa de Ana en Go (todos sus permisos, no solo enviar pedidos) sin que Go haya verificado el PIN; el bloqueo de Go (A1-D5) ya no protege su sesión | Prueba `R3ReviewTest.entradaSinConexionReutilizaLaSesionDeGoEnMemoria` (pasa: `Authorization = Bearer aaaa…1` tras la entrada sin conexión). La prueba del repositorio `sinConexionSoloEntraQuienYaEntroAqui` solo mira la bandera                       | En `enterOfflineLocked` quitar `sessions[userId]` (o que `requestHeaders`/`sessionToken` solo den la sesión si `activeWaiter.hasServerSession`); al volver la red, el mesero que entró sin conexión obtiene sesión solo entrando con su PIN con conexión. Prueba que exija «sin `Authorization`» tras una entrada sin conexión                                            |
| **A2** | **alto**             | `android/core/domain/.../access/IdleLock.kt:38-39` (`watch` empieza con `touch()`), `feature/access/.../AccessViewModel.kt:73-84` (la vigilancia vive en un ViewModel de la actividad) y `app/.../AppContainer.kt:86` (el mesero activo vive en el proceso) | El plazo del bloqueo por inactividad se **reinicia** cuando se vuelve a crear la actividad con el proceso vivo, sin importar cuánto tiempo pasó                                                                                                                                                                                                                                                                      | Tableta con Android 8–11 (minSdk 26, las económicas de §2): Ana está en Mesas y toca Atrás → `MainActivity` termina (comportamiento de la plataforma hasta la API 30), se limpia su ViewModel y se cancela la vigilancia; el proceso queda en caché con Ana activa. Una hora después Beto abre la aplicación → el nuevo `AccessViewModel` llama a `watch()` → `touch()` → Beto ve el modo Mesero de Ana, con su sesión de Go, 2 minutos más y cada toque los alarga: trabaja como Ana sin PIN (§3.2)                             | Prueba `R3IdleLockTest.reabrirLaActividadReiniciaElPlazoDelMeseroAnterior` (pasa: Ana sigue activa 1 h 52 s después de su último toque). Lo de Atrás en Android 8–11 es el comportamiento documentado de la plataforma; **no** se repitió en el emulador | No reiniciar el plazo en `watch()`: usar el `lastActivity` del `IdleLock` de proceso; al empezar a vigilar y en cada `ON_START`/`onResume` (y al encender la pantalla), si ya venció y no hay trabajo pendiente, bloquear en ese momento. O mover la vigilancia al ámbito del proceso (`AccessRepository`/`appScope`). Prueba con la actividad recreada después del plazo |
| M1     | medio                | `android/core/domain/.../access/OfflineAttemptPolicy.kt:22-44`; `AccessRepository.kt:487` (`wallClock` = `System.currentTimeMillis`)                                                                                                                        | El límite local sin conexión (5 fallos → 5 min) usa el **reloj de pared**, que cualquiera cambia en una tableta compartida (Ajustes › Fecha y hora; sin red no hay hora automática). El comentario de `:22-24` dice que no importa, pero el único freno que queda es la derivación de 0.3 s                                                                                                                          | Sin conexión: 5 PIN incorrectos → «bloqueado»; se adelanta la hora 5 min → otros 5; se repite sin fin. Con el verificador de un compañero se prueban todos los PIN de 4 dígitos en horas, y al acertar se entra como él (y con A1, con su sesión de Go)                                                                                                                                                                                                                                                                          | `R3ReviewTest.adelantarElRelojDaIntentosSinFin` (pasa: 100 fallos y después entra)                                                                                                                                                                       | No reiniciar la cuenta por tiempo de pared: p. ej., tras 10 fallos locales desde la última entrada con conexión, desactivar la entrada sin conexión de ese mesero hasta que entre con conexión; guardar la última hora vista y tratar los saltos como manipulación; dentro de un mismo arranque, medir con `elapsedRealtime`                                              |
| M2     | medio                | `AccessRepository.kt:430` (429 `PIN_LOCKED` no se guarda), `:494` y `:612-623` (sin conexión solo cuenta el bloqueo local)                                                                                                                                  | El bloqueo de Go y el límite local son **contadores separados**: el `PIN_LOCKED` de Go (o `locked_until` de la lista) no pasa al dispositivo, y los fallos sin conexión no cuentan en Go                                                                                                                                                                                                                             | 5 fallos con conexión → Go bloquea el PIN de Ana 5 min; se pone el modo avión → 5 intentos más contra el verificador al instante (y con el PIN correcto entra aunque Go lo tenga bloqueado). El presupuesto real es el doble del de A1-D5, o ilimitado con M1                                                                                                                                                                                                                                                                    | `R3ReviewTest.elBloqueoDeGoNoSeRespetaSinConexion` (pasa: `Entered` sin conexión con el PIN bloqueado en Go)                                                                                                                                             | Al recibir `PIN_LOCKED` o una lista con `locked_until`, guardar `local_locked_until = max(local, Go)`; contar también los `PIN_INVALID` con conexión en el límite local                                                                                                                                                                                                   |
| M3     | medio                | `AccessRepository.kt:178-195` (`report` con `SESSION_*`) frente a `:373-376` (`invalidateLocked`); `android/CLAUDE.md:152`                                                                                                                                  | Dos reglas distintas cuando el acceso del mesero activo termina: `report(SESSION_*)` lo saca **sin consultar** `WaiterWork`; `invalidateLocked` lo deja si `blocksSwitch`, pero le borra sesión y credencial. No hay forma de volver a escribir el PIN del **mismo** mesero conservando su trabajo                                                                                                                   | (a) Ana con 2 pedidos sin enviar y su sesión vence (12 h) → vuelve a la lista de meseros (§0-B, A1-D15). (b) Le cambian el PIN con 1 pedido sin enviar → queda activa sin sesión ni credencial, no puede cambiar de mesero (sus pedidos lo impiden) ni ver la pantalla del PIN (solo aparece sin mesero activo): atascada hasta que llegue un `SESSION_*` (y entonces sale igual) o se cierre la aplicación                                                                                                                      | `R3ReviewTest.sesionTerminadaSacaAlMeseroConPedidosSinEnviar` y `pinCambiadoConPedidosDejaAlMeseroSinSesionNiCredencial` (pasan)                                                                                                                         | Una sola regla: si el acceso del mesero activo terminó (sesión vencida o revocada, PIN cambiado, fuera de la lista), pedir «Vuelve a escribir tu PIN» para ese mismo mesero, conservando captura y cola, permitido aunque tenga pedidos sin enviar; nunca pasar a otro mesero en silencio                                                                                 |
| M4     | medio                | `core/data/.../db/AccessEntities.kt:140-147` (`forgetServer`), `AccessRepository.kt:575-599`, `AccessViewModel.kt:132-134`, `feature/access/src/main/res/values/strings.xml:115`                                                                            | Retirar o «Emparejar de nuevo» solo borra las tablas de acceso. No hay punto para que las fases D (catálogo, imágenes) y E (cola, borradores, WorkManager) borren lo suyo: `UnsentOrdersSource` solo lee. Además la confirmación de «Emparejar de nuevo» dice «Los datos del restaurante no cambian» y los pedidos sin enviar pasan a la bitácora sin mostrarse antes (ya anotado por el coordinador para la Fase E) | Con la Fase E: el dispositivo se retira → la cola sigue en Room y WorkManager programado; se empareja con otro servidor → esos pedidos pueden salir hacia él. El catálogo del restaurante anterior queda en el dispositivo (§3.2: «borra sus datos locales»)                                                                                                                                                                                                                                                                     | Lectura                                                                                                                                                                                                                                                  | Interfaz de borrado (p. ej. `LocalDataWiper`) que `forgetLocked` llame dentro del mismo cambio, con `WorkManager.cancelAllWork`; la confirmación de «Emparejar de nuevo» lista los pedidos sin enviar como la revocación; corregir el texto                                                                                                                               |
| M5     | medio                | `AccessRepository.kt:215-219`; `android/CLAUDE.md:147`                                                                                                                                                                                                      | `requestHeaders()` no recibe el mesero: da las cabeceras del mesero **activo al momento de la llamada**, no del dueño de la operación                                                                                                                                                                                                                                                                                | Fase E: una actualización o un reintento que lanzó la pantalla de Ana termina después del cambio de mesero o del bloqueo → sale con la sesión de Beto y Go lo registra como de Beto (Registro de movimientos y autoría equivocados)                                                                                                                                                                                                                                                                                              | Lectura                                                                                                                                                                                                                                                  | `requestHeaders(userId)` obligatorio (la operación guarda su mesero al crearse) y rechazo si ya no es el activo; o que la cola use solo `sessionToken(userId)`/`offlineCredential(userId)`                                                                                                                                                                                |
| M6     | medio (para decidir) | `backend/internal/administration/pin.go:25-31` y `pin_login.go:223` (A1-D5); `migrations/0025_android_waiters.sql:98-99` (A1-D2)                                                                                                                            | Con 5 intentos cada 5 min sin escalamiento hay 1 440 intentos al día por usuario: un PIN de 4 dígitos (≈ 9 950 permitidos) cae en ≈ 3.5 días en promedio desde cualquier tableta emparejada. El rol Administrador trae `waiter_devices.use` y la sesión con PIN es una sesión completa (todos los permisos del usuario por la API con la credencial del dispositivo)                                                 | Un administrador define su PIN en «Mi PIN» → su cuenta de administración queda a un PIN de 4 dígitos desde cualquier teléfono o tableta emparejado. No es un defecto frente a A1-D2/A1-D5, sí un riesgo que conviene decidir                                                                                                                                                                                                                                                                                                     | Lectura; el ritmo del bloqueo se comprobó en vivo (`TestR3ReviewPIN`)                                                                                                                                                                                    | Escalamiento (la opción de B-5: tras 3 bloqueos en 24 h, solo lo desbloquea un administrador) y/o que el administrador no reciba `waiter_devices.use` por omisión; decisión del coordinador (A1-D5) y del propietario (A1-D2)                                                                                                                                             |
| B1     | bajo                 | `backend/internal/administration/service.go:280-316`; `apicontract/openapi.json:1978`                                                                                                                                                                       | La credencial de envío se **ignora** (no se rechaza) en `setup/status`, `setup`, `devices/enroll`, `pairing/*`, `auth/login`, `auth/pin-users` y `auth/pin-login`, que se atienden antes de mirarla. El contrato dice «en cualquier otra ruta, 401 OFFLINE_CREDENTIAL_NOT_ALLOWED». No da ningún acceso (son públicas o tienen su propia autenticación)                                                              | `GET auth/pin-users` con `X-Offline-Credential` → 200                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | En vivo (`TestR3ReviewPIN`: `auth/pin-users` y `setup/status` → 200; las otras 6 rutas probadas → 401 `OFFLINE_CREDENTIAL_NOT_ALLOWED`)                                                                                                                  | Revisar la credencial al principio de `dispatch`, o ajustar el texto del contrato                                                                                                                                                                                                                                                                                         |
| B2     | bajo                 | `backend/internal/httpapi/clientversion.go:130-138`                                                                                                                                                                                                         | `X-Client-Kind` lo declara el cliente y no se compara con `devices.kind`: un escritorio que diga `android` recibe la mínima de Android (0.1.0)                                                                                                                                                                                                                                                                       | Hoy sin efecto (todo escritorio que envía las cabeceras es ≥ 0.10.0); importará cuando suba la mínima del escritorio                                                                                                                                                                                                                                                                                                                                                                                                             | Lectura                                                                                                                                                                                                                                                  | En las rutas con credencial de dispositivo, rechazar un tipo distinto del registrado o tomar la mínima de `devices.kind`                                                                                                                                                                                                                                                  |
| B3     | bajo                 | `AccessRepository.kt:396-403`                                                                                                                                                                                                                               | `pin-login` que salió pero no recibió respuesta (tiempo agotado) cae al verificador con la credencial de envío **anterior**, que Go ya revocó (`replaced`) si procesó la entrada                                                                                                                                                                                                                                     | Wi-Fi inestable: Ana entra «sin conexión» y lo que capture recibe 401 `OFFLINE_CREDENTIAL_INVALID` hasta que vuelva a entrar con conexión (el pedido se conserva)                                                                                                                                                                                                                                                                                                                                                                | Lectura                                                                                                                                                                                                                                                  | Con `requestSent = true` no caer al verificador (pedir reintentar), o borrar el verificador al recibir `OFFLINE_CREDENTIAL_INVALID` para forzar una entrada con conexión                                                                                                                                                                                                  |
| B4     | bajo                 | `app/.../MainActivity.kt:22-25`                                                                                                                                                                                                                             | `onUserInteraction` no recibe los toques dentro de las ventanas propias de Compose (`Dialog`, `ModalBottomSheet`, `DropdownMenu`); además la espera usa `delay` (tiempo despierto, se detiene en reposo profundo) hasta 60 s por vuelta                                                                                                                                                                              | Un mesero que lee o usa un diálogo más de 2 min (sin captura en curso) se bloquea en medio; al encender la pantalla tras un reposo, el mesero anterior puede seguir a la vista hasta ≈ 60 s                                                                                                                                                                                                                                                                                                                                      | Lectura (comportamiento de la plataforma)                                                                                                                                                                                                                | Reiniciar el plazo también desde los diálogos (envoltura de `Window.Callback` o `pointerInput` en la raíz de cada diálogo); comprobar al encender la pantalla (ver A2)                                                                                                                                                                                                    |
| B5     | bajo                 | `backend/internal/administration/pin.go:371-386` (`ownPIN`)                                                                                                                                                                                                 | La contraseña actual de «Mi PIN» se verifica sin límite de intentos, desde cualquier sesión, también una sesión con PIN de Android (igual que `auth/password`, anterior a la etapa)                                                                                                                                                                                                                                  | Con una sesión obtenida con un PIN de 4 dígitos se prueba la contraseña de esa cuenta sin límite                                                                                                                                                                                                                                                                                                                                                                                                                                 | Lectura                                                                                                                                                                                                                                                  | Aplicar el límite de `login_attempts` a la verificación de la contraseña actual                                                                                                                                                                                                                                                                                           |
| B6     | bajo                 | `desktop/src/pages/UsersPage.tsx:440`                                                                                                                                                                                                                       | «Asignar PIN»/«Cambiar PIN» aparece también en la fila del propio administrador; Go responde 403 «Define tu propio PIN en «Mi PIN»»                                                                                                                                                                                                                                                                                  | El administrador lo toca y recibe un rechazo                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | Lectura                                                                                                                                                                                                                                                  | Ocultarlo en la fila propia o llevar a «Mi PIN»                                                                                                                                                                                                                                                                                                                           |
| B7     | bajo                 | `android/core/data/src/test/.../AccessRepositoryTest.kt` (`sinConexionSoloEntraQuienYaEntroAqui`) y avance «Una sesión sin conexión no tiene sesión de Go»                                                                                                  | La prueba dice cubrir «sin conexión no hay sesión de Go» pero solo mira `hasServerSession`; por eso no detectó A1. Igual con el comentario de `OfflineAttemptPolicy.kt:22-24` (M1)                                                                                                                                                                                                                                   | —                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | Lectura y A1                                                                                                                                                                                                                                             | Probar las cabeceras reales y corregir el comentario                                                                                                                                                                                                                                                                                                                      |
| O1     | observación          | `pin_login.go:306-308`                                                                                                                                                                                                                                      | Quitar `waiter_devices.use` no revoca la credencial de envío: se rechaza mientras falta el permiso y **vuelve a valer** si se devuelve. Coherente con A1-D1 (no está en su lista) y el dispositivo la borra al dejar de ver al mesero en la lista                                                                                                                                                                    | Rol sin el permiso → 401 `OFFLINE_CREDENTIAL_INVALID`; permiso devuelto → la misma credencial llega otra vez a la operación                                                                                                                                                                                                                                                                                                                                                                                                      | En vivo (`TestR3ReviewPIN`)                                                                                                                                                                                                                              | Decidir si quitar el permiso debe revocarla (`revoke_reason` nuevo)                                                                                                                                                                                                                                                                                                       |
| O2     | observación          | `users.go:150-159`                                                                                                                                                                                                                                          | Desactivar y reactivar no sube `pin_version`: un dispositivo que no vio la desactivación conserva el verificador y la credencial ya revocada (`user_disabled`); lo que se capture sin conexión recibirá 401                                                                                                                                                                                                          | Tableta apagada de noche mientras el mesero se desactiva y reactiva                                                                                                                                                                                                                                                                                                                                                                                                                                                              | Lectura                                                                                                                                                                                                                                                  | Subir `pin_version` al desactivar, o borrar el verificador ante `OFFLINE_CREDENTIAL_INVALID` (B3)                                                                                                                                                                                                                                                                         |
| O3     | observación          | `pin_login.go:75`                                                                                                                                                                                                                                           | Sin nombre corto la lista usa el nombre visible (puede ser el nombre completo); §3.1 dice «nombre corto»                                                                                                                                                                                                                                                                                                             | —                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | Lectura                                                                                                                                                                                                                                                  | Documentarlo en «Asignar PIN» (ya lo dice la pista del campo)                                                                                                                                                                                                                                                                                                             |
| O4     | observación          | `security.go:16-36`                                                                                                                                                                                                                                         | Argon2id de 64 MiB por intento sin límite de concurrencia; los `pin-login` con usuarios inexistentes no se limitan                                                                                                                                                                                                                                                                                                   | Un dispositivo emparejado con muchas peticiones en paralelo sube la memoria del servidor (igual que `auth/login`, anterior)                                                                                                                                                                                                                                                                                                                                                                                                      | Lectura                                                                                                                                                                                                                                                  | Semáforo global para Argon2                                                                                                                                                                                                                                                                                                                                               |

### Detalle de los hallazgos altos

**A1 · La entrada sin conexión reutiliza la sesión de Go en memoria.** `AccessRepository` guarda la sesión de cada
mesero en `sessions` (solo en memoria, decisión 4 de las fases C y B) y no la borra al salir de él (cambio de mesero y
bloqueo por inactividad no cierran la sesión, A1-D15). `enterOfflineLocked` (`:484-531`) solo verifica el PIN contra el
verificador local y pone `ActiveWaiter(…, hasServerSession = false)`, pero `requestHeaders()` (`:215-219`) toma
`sessions[activo]` sin mirar esa bandera. `android/CLAUDE.md:147` promete lo contrario («si hay mesero activo **con
sesión de Go**») y el encargo pide que una entrada sin conexión nunca se presente como sesión de Go. Consecuencia: la
única verificación del PIN que protege la sesión de Go pasa a ser el verificador local, cuyo límite se esquiva (M1, M2);
el bloqueo de 5 fallos de Go (A1-D5) deja de proteger la sesión. Reproducción: `R3ReviewTest` en
`$SP/r3/tree/android/core/data/src/test/.../access/R3ReviewTest.kt` (`Authorization tras entrada sin conexión = Bearer
aaaa…1`).

**A2 · El bloqueo por inactividad se reinicia al recrear la actividad.** El mesero activo es del proceso
(`AccessRepository`, en `AppContainer`), pero quien lo vigila es `AccessViewModel`, que vive con la actividad. Cuando la
actividad se destruye con el proceso vivo, la vigilancia se cancela; al volver a abrir la aplicación, el ViewModel nuevo
llama a `IdleLock.watch()`, que empieza con `touch()` y concede un plazo completo al mesero anterior. En Android 8–11
(la tableta mínima de la etapa) tocar Atrás en la pantalla raíz termina la actividad; desde Android 12 la mueve al
fondo (ahí no ocurre, salvo destrucción por el sistema). Reproducción con tiempo virtual: `R3IdleLockTest` en
`$SP/r3/tree/android/feature/access/src/test/.../R3IdleLockTest.kt` (`mesero activo 1 h 52 s después de su último toque
= Ana`). No se repitió en el emulador de Android 8 (`ADEL_A1_Tablet_API26`).

### Lo que se revisó sin encontrar defectos

- **Hash del PIN** (`pin.go:275`, `:391`): `HashPassword` (Argon2id m=64 MiB, t=3, p=1, sal de 16 bytes), el mismo de las
  contraseñas; nunca el PIN. Hash ficticio `pinDummyHash` con los mismos parámetros para inexistentes, inactivos, de
  soporte o sin PIN; tiempos iguales en vivo (1.47 s / 1.45 s / 1.46 s por intento con `-race`, con PIN / sin PIN /
  inexistente).
- **PIN triviales**: `validatePIN` en las dos únicas rutas que escriben `pin_hash` (`assignUserPIN` y `ownPIN`;
  comprobado con `grep`); `setup` no define PIN.
- **Bloqueo**: por usuario desde cualquier dispositivo; `SELECT … FOR UPDATE` de la fila del usuario serializa los
  intentos: **12 intentos simultáneos → exactamente 4 × 401 y 8 × 429 y un solo `user.pin_locked`** (en vivo); el id en
  mayúsculas cuenta para la misma fila y con espacios es 400 sin contar (en vivo); inexistentes nunca llegan a 429 (no
  delata quién tiene PIN; la existencia de un UUID no es secreta). El quinto responde 429 y el contador vuelve a cero
  (A1-D5). Desbloqueo con `users.manage`, delegación segura y registro solo si había bloqueo.
- **Registro de movimientos**: `user.pin_set` (quién), `user.pin_removed`, `user.pin_locked` (con el dispositivo, una
  vez), `user.pin_unlocked`, `session.pin_login`, `session.pin_login_failed`; los estados antes y después usan
  `publicUserColumns`, sin hashes ni credenciales; el registro HTTP no guarda cabeceras ni cuerpos (`router.go`).
- **Solo dispositivos emparejados**: `pin-users` y `pin-login` van después de `authenticateDevice` (credencial y no
  revocado) en las dos escuchas y exigen `kind = 'android'`. La lista solo trae usuarios activos, no de soporte, con PIN y
  con el permiso efectivo (misma regla que `effectivePermissions`), con 6 propiedades.
- **Credencial de envío**: igualdad exacta de ruta y método (`restaurant/offline-orders/`, `GET`, `PUT`,
  `restaurant/state`, `auth/logout` y `auth/me` → 401 `OFFLINE_CREDENTIAL_NOT_ALLOWED`, en vivo); SSE (`events.go`
  revalida en cada lectura); atada a usuario y dispositivo (`c.device_id = $2`); `pin_version` igual a la de emisión;
  revocada al cambiar o quitar el PIN, desactivar al usuario, revocar el dispositivo y al reemplazarla (índice único
  parcial); solo su SHA-256; se entrega una vez; permisos vigentes en cada uso (`runProtected` y `operations.Handle`);
  sesión y credencial juntas → 400; idempotencia por usuario y dispositivo.
- **Sesiones**: `issueSession(…, sameDevice)` (una por usuario y dispositivo, 9H); cambiar o quitar el PIN revoca las
  sesiones `android` del usuario (`pin_changed`/`pin_removed`) y conserva las de escritorio; «Mi PIN» conserva la actual.
  El bloqueo por inactividad no cierra la sesión en Go, coherente con §0-B, §7 y A1-D15 (salvo A2).
- **Versión mínima por tipo**: sin tipo se compara con la general (no rebaja); tipo desconocido → 400; escritorio
  0.10.x/0.11.x sin cambios (mínima 0.10.0; campos nuevos solo se agregan; `PATCH devices` conserva
  `idle_lock_seconds` sin el campo).
- **Migración 0025 sección B**: restricciones e índices con nombre, transaccional (el migrador), no cambia filas salvo la
  concesión de A1-D2; índices para el bloqueo (PK de `pin_attempts`), la credencial (`secret_hash` único) y su
  revocación (índice parcial por usuario y dispositivo, índice por dispositivo).
- **Emparejamiento y huella** (§4): `FirstSeenTrustManager` solo en `PairingRepository`; el código de 6 dígitos usa la
  huella fijada en la sesión y es la que se guarda; con servidor encontrado se exige además `fp` y `server_id`
  anunciados. Después, siempre `PinnedTrustManager` (SPKI, comparación en tiempo constante, sin CA del sistema ni
  nombre), `RESTRICTED_TLS` sin `CLEARTEXT` (OkHttp rechaza `http://`), sin redirecciones. Cambio de IP: solo anuncios del
  mismo `server_id` y huella corta, aceptados con `health/live` y la huella completa; el anuncio NSD nunca da confianza
  y la sonda no lleva credenciales. Texto claro hacia `10.0.2.2` solo en `src/debug`.
- **Secretos en el dispositivo**: credencial del dispositivo, verificador y credencial de envío cifrados con AES-256-GCM
  del Keystore con el propósito como dato autenticado; sesiones solo en memoria; el PIN solo en memoria
  (`PinEntryViewModel`, nunca en `SavedStateHandle`); ningún `Log`, `SharedPreferences` ni volcado; copias de
  seguridad excluidas; `DeviceCredentials.toString()` oculta el token. Llave perdida: credencial ilegible →
  «necesita emparejarse de nuevo»; verificador ilegible → se olvida y se pide entrar con conexión.
- **Verificador local**: PBKDF2-HMAC-SHA256 de 120 000 iteraciones, sal de 16 bytes nueva en cada entrada con
  conexión, comparación en tiempo constante, ligado al `user_id`, invalidado por `pin_version` o si el mesero sale de la
  lista; solo entran sin conexión quienes ya entraron con conexión en ese dispositivo (salvo M1/M2 sobre el límite).
- **Room v2**: solo blobs cifrados para secretos; `forgetServer` en una transacción; migración 1 → 2 con el SQL del
  esquema exportado (no se volvió a correr `MigrationTestHelper`).
- **Escritorio**: `UserPin.tsx` (PIN en campos ocultos de solo dígitos, nunca muestra uno guardado, los mensajes son de
  Go), «Mi PIN» solo con la capacidad y el permiso, Terminales envía `idle_lock_seconds` solo con `waiter-pin-v1`,
  `api.rs` con las cinco operaciones en la lista cerrada y sin la entrada con PIN.
- **Calidad**: código claro, sin compactar, comentarios en español en reglas, seguridad y protocolo (regla 14 de
  `android/CLAUDE.md` y `AGENTS.md`), salvo B7.

### No revisado o no verificado

- A2 en un emulador o dispositivo con Android 8–11 (solo la JVM con tiempo virtual y el comportamiento documentado de
  Atrás); B4 en el dispositivo.
- Las suites del repositorio (no se volvieron a correr: se usaron las del avance como contexto), las pruebas
  instrumentadas, Playwright y Vitest del escritorio, y `MigrationTestHelper`.
- `docs/API_V1.md` completo (solo las secciones del PIN y la credencial) y `scripts/android-e2e.sh`.
- `NsdServerBrowser` en detalle (solo que no da confianza) y la búsqueda NSD real.
- La integración con las fases D y E (no existen todavía; M4 y M5 son riesgos de la interfaz).

### Pruebas ejecutadas (solo en la copia `$SP/r3/tree`)

| Qué                                                       | Dónde                                                                                                                   | Resultado                                                                                                                                                                                                                                          |
| --------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `R3ReviewTest` (5 pruebas que reproducen A1, M1, M2 y M3) | `iso.sh … gradle --offline :core:data:testDebugUnitTest` (Robolectric SDK 36, `FakeGoServer` con TLS)                   | **5/5 pasan = los 5 defectos se reproducen** (XML en `$SP/r3/results/`)                                                                                                                                                                            |
| `R3IdleLockTest` (1 prueba que reproduce A2)              | `:feature:access:testDebugUnitTest` (tiempo virtual)                                                                    | **1/1 pasa = se reproduce**                                                                                                                                                                                                                        |
| `TestR3ReviewPIN` (4 subpruebas)                          | `ISO_TAG=R ISO_RUN=TestR3ReviewPIN iso.sh … go-int ./internal/administration` (PostgreSQL 55499, base temporal borrada) | `ok` (51 s): concurrencia 4 × 401 + 8 × 429 y un solo bloqueo; mayúsculas y espacios; tiempos iguales; credencial rechazada en 6 rutas, ignorada en `auth/pin-users` y `setup/status` (B1); permiso quitado → 401 y devuelto → vuelve a valer (O1) |

### Decisiones sobre la revisión R3

**Del propietario** (16/11/2026, preguntadas por el coordinador porque cambian la política de seguridad del PIN):

- **A1-D16 · Bloqueo largo del PIN** (completa A1-D5): los primeros bloqueos siguen siendo de 5 minutos tras 5 fallos
  en 15 minutos, pero **tras 3 bloqueos seguidos en 24 horas sin una entrada correcta, el PIN queda bloqueado hasta que
  un administrador lo desbloquee** en Personal › Usuarios, con su registro. Responde a M6 (unos 1 440 intentos al día
  por usuario con la regla anterior). Se acepta el inconveniente de que alguien bloquee a propósito el PIN de otro.
- **A1-D17 · La sesión abierta con PIN solo tiene permisos de mesero**: los permisos efectivos de una sesión de
  `auth/pin-login` (y de la credencial de envío) son los del usuario **dentro de una lista cerrada** de los que usa la
  app de meseros; aunque la persona sea administrador o supervisor, lo administrativo sigue con contraseña en la
  computadora. La lista solo crece con las funciones de la app (A2: Para llevar e impresión de la precuenta) y vive en un
  solo lugar del servidor, documentada.

**Del coordinador**:

- Se corrigen todos los hallazgos altos y medios y los bajos B1, B2 (si no rompe nada), B4, B6, B7 y la observación O4
  (límite de verificaciones Argon2id a la vez). Las del dispositivo (A1, A2, M1–M5, B4, B7) las hace el agente de las
  fases C y B; las del servidor (A1-D16, A1-D17, B1, B2, B6, O4) el de la Fase B del servidor.
- **M1 y M2 (límite sin conexión)**: el límite local deja de depender del reloj de pared; los fallos sin conexión se
  acumulan y solo vuelven a cero con una entrada correcta; tras el umbral, la entrada sin conexión de ese mesero queda
  cerrada hasta que entre con conexión; y un bloqueo de Go (`PIN_LOCKED` o `locked_until` de `auth/pin-users`) también
  cierra la entrada sin conexión.
- **M3**: una sesión que ya no vale (`SESSION_*`, PIN cambiado, `OFFLINE_CREDENTIAL_INVALID`) nunca saca al mesero ni
  pierde su trabajo: le pide «Vuelve a escribir tu PIN» y conserva su captura y sus pendientes.
- **M4**: un único registro de «borrar los datos de este dispositivo» al que se suman el catálogo, sus imágenes y, en la
  Fase E, la cola y WorkManager; retirar el dispositivo y «Emparejar de nuevo» lo usan (con los pendientes mostrados
  antes) y sus textos dicen qué se borra.
- **M5**: las cabeceras se piden **para un mesero concreto** (el dueño de la operación), nunca «las del mesero activo»
  para una operación de otro.
- Se conservan: O1 (quitar el permiso deja inservible la credencial mientras falte; devolverlo la vuelve a habilitar, y
  así no se pierden pedidos), O2, O3 y B5 (igual que `auth/password`); B3 queda cubierto por M3.

## Correcciones de la revisión R3 · Servidor y escritorio

Un agente, en su propio árbol de trabajo (`etapa-a1-b-fix`, desde `etapa-a1-android` en `3ca8b6e`), en paralelo con el
agente de las fases C y B del dispositivo (que corrige `android/`). 16/11/2026. Sin cambio de versión (0.12.0, esquema
25: la 0025 todavía no está en ninguna instalación, así que su **sección B** se editó; una base de desarrollo o de
prueba que ya la tenía se recrea). Pruebas en copias aisladas (`ISO_TAG=B`): clúster temporal 55499 con una base
`adel_pos_test_b_*` por paquete y Go temporal de Playwright en 8186 con la vista previa en 1424. **No se tocó la
instalación real** ni `android/`, `CLAUDE.md` ni `AGENTS.md`; `docs/etapas/*.md` con el mismo md5 antes y después. No se
usaron emuladores ni `server-up`.

Commits en `etapa-a1-b-fix`:

- `29b63b3` — servidor: A1-D16, A1-D17, B1, B2 y O4; 0025 sección B; contrato v1 y sus pruebas.
- `df8d00f` — `docs/API_V1.md` y `API_CONTRACTS.md`, con la prueba que los compara con la lista de permisos de mesero.
- `1248802` — escritorio: bloqueo largo en Personal › Usuarios, sin «Asignar PIN» en la fila propia (B6) y etiqueta del
  Registro de movimientos.

### Qué cambió

**A1-D16 · Bloqueo largo del PIN** (`backend/internal/administration/pin_login.go:228`, `recordPINFailure`):

- Los fallos siguen igual (5 en 15 minutos → 5 minutos; la cuenta vuelve a cero al bloquearse). Cada bloqueo se cuenta
  en una **serie** que empieza con el primero (`pin_attempts.lock_series_started_at`) y dura 24 horas: el primero y el
  segundo son de 5 minutos (`user.pin_locked`, ahora con `lock_in_series`); **el tercero deja el PIN bloqueado hasta que
  un administrador lo desbloquee** (`requires_unlock = true`, sin `locked_until`; registro propio
  **`user.pin_locked_until_unlock`**, una vez, con el dispositivo). Una serie que empezó hace más de 24 horas se olvida
  y el bloqueo nuevo es el primero de otra. **Una entrada correcta borra la fila** (fallos y serie).
- Respuesta acordada con el dispositivo: `429 PIN_LOCKED` con `details.locked_until` (RFC 3339) y
  `details.requires_unlock: false` en un bloqueo por tiempo; `details.locked_until: null` y `details.requires_unlock:
true` en el largo (mensaje: «Este PIN quedó bloqueado por demasiados intentos. Pide a un administrador que lo
  desbloquee en Personal › Usuarios.»). Las dos propiedades van siempre (`failedPINLogin`, `pin_login.go:281`).
- `GET auth/pin-users`: cada usuario conserva `locked_until` y agrega `requires_unlock` (booleano, siempre presente).
  `GET users`: `pin_requires_unlock`. `POST users/{id}/pin/unlock` quita los dos bloqueos y la serie; `user.pin_unlocked`
  dice cuál quitó.
- **«Mi PIN» no quita el bloqueo largo** (`ownPIN` solo borra los intentos si no espera a un administrador): así «solo
  lo desbloquea un administrador» vale también para el propio usuario. Asignar o quitar el PIN desde Personal ›
  Usuarios (que es una acción de administrador) sí empieza de cero, como antes.
- 0025, sección B: `pin_attempts.locks` (0–3), `lock_series_started_at`, `requires_unlock` y dos CHECK
  (`pin_attempts_series_check`: serie con inicio; `pin_attempts_requires_unlock_check`: el largo sin hora y con 3
  bloqueos).

**A1-D17 · La sesión con PIN solo tiene permisos de mesero**:

- Lista cerrada en un solo lugar: `administration.WaiterPermissions` (`backend/internal/administration/waiter_scope.go:14`).
  **Lista final**: `waiter_devices.use`, `restaurant.tables.read`, `restaurant.areas.read`, `accounts.read`,
  `accounts.open`, `orders.capture`, `orders.confirm`, `orders.edit`, `orders.cancel`, `catalog.read`,
  `catalog.prices.read`, `events.read`. Sale de la tabla de §4 de la Fase 0 y de los permisos que exigen las rutas del
  contrato que llama la app (`operations/handler.go`, `routePermission`): `restaurant/state` (cualquier sesión),
  `restaurant/overview` (`restaurant.tables.read` y `restaurant.areas.read`), cuentas (`accounts.read`), abrir
  (`accounts.open`), borrador (`orders.capture`), enviar a cocina y pedido sin conexión (`orders.confirm` y los demás de
  `submitOfflineOrder`), modificar y cancelar unidades (`orders.edit`, `orders.cancel`), catálogo e imágenes
  (`catalog.read`, `catalog.prices.read`), eventos (`events.read`) y entrar con PIN o «Mi PIN»
  (`waiter_devices.use`). Nada de Para llevar, cocina, cobros, caja ni administración.
- 0025, sección B: `sessions.opened_with_pin` (falso por omisión; las sesiones existentes quedan de contraseña).
  `issueSession` recibe `withPIN` (`auth.go`); `pinLogin` lo pide en `true`; `authenticateSession` lo lee y deja
  `principal.Permissions = waiterScope(efectivos)` (`service.go:459`), así que **toda** ruta, `auth/me` y
  `restaurant/state` ven ya los permisos acotados. La credencial de envío también (`pin_login.go:385`).
- «Cambiar contraseña» desde una sesión con PIN abre la sesión nueva igual de acotada (`principal.PINSession`).
- Las sesiones con **contraseña** no cambian, también en un teléfono: la decisión habla de la sesión de `pin-login`.
- `waiter_scope_test.go` comprueba que la lista solo tiene permisos registrados, nada administrativo ni de Para llevar,
  y que el contrato (descripción de `pinLogin`), `docs/API_V1.md` y `API_CONTRACTS.md` la repiten completa.

**B1 · La credencial de envío se rechaza en toda otra ruta**: `httpapi/router.go:164` (`offlineCredentialMisplaced`,
`clientversion.go`) responde `401 OFFLINE_CREDENTIAL_NOT_ALLOWED` a toda petición con `X-Offline-Credential` que no sea
`POST /api/v1/restaurant/offline-orders` **antes** de la comprobación de método y de versión: versión, salud, contrato,
canal de eventos y rutas inexistentes incluidas. Además `dispatch` lo revisa **al principio** (`service.go:300`), antes
del asistente, `devices/enroll`, `pairing/*`, `auth/login`, `auth/pin-users` y `auth/pin-login` (defensa para quien use
`Service` sin `httpapi`). La constante vive en `httpapi.OfflineCredentialHeader` (administration la reutiliza).

**B2 · `X-Client-Kind` igual al tipo del dispositivo** (`matchClientKind`, `service.go:395`): después de
`authenticateDevice`, si la petición trae `X-Client-Kind` debe ser `android` en un dispositivo `android` y `desktop` en
uno `desktop` o `kitchen`; si no, `400 CLIENT_KIND_MISMATCH`. Sin la cabecera no se compara. **No rompió nada real**: el
escritorio (Rust, `api.rs` y `pairing.rs`) envía `desktop` desde dispositivos de escritorio; la app Android envía
`android` desde su teléfono; los ayudantes de «computadora principal» de las pruebas de Android (`MainComputer.kt`,
`GoTestServer.kt`) no mandan cabeceras de cliente. Lo que sí hubo que ajustar fueron **dos pruebas de Go que mandaban
`android` con la credencial del escritorio del asistente**: `TestContractV1` (el cliente ahora lleva el tipo de su
dispositivo: `desktop` en la terminal del asistente, `android` en el teléfono emparejado y en las rutas `pairing/*` que
llama el teléfono) y `TestStageA1WaiterPIN` (cada petición va con el tipo y la versión de su dispositivo). Lo dejo dicho
por si el coordinador lo considera un cambio de prueba que no debía hacerse; con ese ajuste, las especificaciones de
Playwright que mandan cabeceras (`workflow-stage9g-print-agent`, `-sold-out`, `-terminals`, `-two-terminals` y
`workflow-stage9h-adjustments`) pasan sin cambios.

**O4 · Pocas verificaciones Argon2id a la vez** (`pin_login.go:301`): como máximo **4** `auth/pin-login` verifican a la
vez; las demás esperan su turno hasta 5 s **en `Execute`, antes de abrir la transacción** (`service.go:217`), así que no
ocupan conexiones de la base ni frenan el resto del servidor. Sin turno: `503 PIN_LOGIN_BUSY` («Hay muchas entradas con
PIN al mismo tiempo. Intenta de nuevo en unos segundos.»). El inicio de sesión con contraseña no cambia (fuera del
encargo).

**B6 · Escritorio**: `UsersPage` recibe `currentUserID` (de `App.tsx`) y no ofrece «Asignar PIN»/«Cambiar PIN» en la
fila propia (el PIN propio va en «Mi PIN»); «Desbloquear» sí aparece.

**Escritorio, A1-D16**: `PinStatus` muestra **«PIN bloqueado: pide desbloqueo»** (`pin_requires_unlock`) con
«Desbloquear»; el Registro de movimientos nombra `user.pin_locked_until_unlock` «PIN bloqueado hasta que lo desbloquee
un administrador».

**Contrato v1** (`openapi.json`): `pinLogin` documenta el bloqueo largo, `PIN_LOGIN_BUSY` y la lista de permisos de
mesero, y tiene una respuesta `429` explícita con el esquema nuevo **`PinLockedError`** (`details.locked_until`
date-time o `null` y `details.requires_unlock`, obligatorias); `PinUser.requires_unlock` (obligatoria);
`ClientKind` y el esquema `offlineCredential` dicen las reglas nuevas; `getIdentity` y `getRecoveryState` dicen que con
PIN los permisos llegan acotados; la respuesta de error lista `CLIENT_KIND_MISMATCH` y `PIN_LOGIN_BUSY`. `TestContractV1`
bloquea a un segundo mesero con 5 fallos y valida el 429 contra `PinLockedError` y la lista con `locked_until`.

### Decisiones (con su porqué)

1. **Serie de 24 horas desde el primer bloqueo** (no una ventana deslizante): «3 bloqueos seguidos en 24 horas» con dos
   columnas en lugar de guardar cada bloqueo; con bloqueos de 5 minutos un ataque llega al tercero en unos 20 minutos,
   así que la diferencia en el borde no importa.
2. **«Mi PIN» no quita el bloqueo largo** (sí lo hace un administrador al desbloquear, asignar o quitar el PIN). Así se
   cumple «solo lo desbloquea un administrador» al pie de la letra y el administrador se entera.
3. **Las sesiones con contraseña en un teléfono no se acotan**: A1-D17 habla de la sesión de `pin-login` y de la
   credencial; quien sabe la contraseña ya podía entrar con ella en cualquier terminal. Si el propietario quiere acotar
   también esas, basta marcar las sesiones por tipo de dispositivo.
4. **`CLIENT_KIND_MISMATCH` es 400**, no 401: no es una credencial que haya que borrar (Rust solo borra con
   `DEVICE_*`/`SESSION_*`); la app o la terminal mal configurada lo ve como un error claro.
5. **4 turnos y 5 s de espera** para el PIN: ≈ 256 MiB de Argon2id como máximo; la espera va fuera de la transacción
   para no agotar el grupo de conexiones (lo que de verdad frenaría al resto del servidor).
6. **Defensa doble de B1** (router y `dispatch`): el router cubre las rutas técnicas, que nunca llegan a `dispatch`.

### Para el agente de Android

- `429 PIN_LOCKED`: leer `error.details.requires_unlock`; si es `true` (`locked_until: null`), el PIN espera a un
  administrador: cerrar también la entrada sin conexión de ese mesero y decir «Pide a un administrador que lo
  desbloquee». `auth/pin-users` trae `requires_unlock` en cada usuario.
- `503 PIN_LOGIN_BUSY`: reintentar la entrada en unos segundos (no cuenta como fallo).
- La sesión con PIN y la credencial solo tienen la lista de mesero: no esperar permisos fuera de ella en
  `restaurant/state` (por ejemplo `takeaway.read`); la A2 la ampliará.
- `X-Client-Kind: android` solo con la credencial del teléfono (con otra, `400 CLIENT_KIND_MISMATCH`), y nunca
  `X-Offline-Credential` fuera de `POST restaurant/offline-orders` (401 `OFFLINE_CREDENTIAL_NOT_ALLOWED`, también en
  `version` o `auth/pin-users`).

### Pruebas ejecutadas

| Qué                                                                                                                                      | Dónde                                 | Resultado                                                            |
| ---------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- | -------------------------------------------------------------------- |
| `go-unit` (vet normal e integración, `test -race`, `GOOS=windows` build y vet)                                                           | `iso.sh go-unit`                      | **Aprobado**                                                         |
| `TestWaiterPermissionsAreClosedAndDocumented`, `TestWaiterScopeKeepsOnlyWaiterPermissions`, `TestPINLoginSlotsAreLimited`                | `internal/administration` (unitarias) | Aprobadas                                                            |
| `TestOfflineCredentialOnlyOnItsRoute` (B1 en el router)                                                                                  | `internal/httpapi` (unitarias)        | Aprobada                                                             |
| `TestStageA1WaiterPIN` sola (13 subpruebas; nuevas 09–12)                                                                                | `go-int ./internal/administration`    | **ok** (91 s)                                                        |
| `go-int ./internal/administration` completo                                                                                              | base `adel_pos_test_b_*`              | **ok** (217 s)                                                       |
| `go-int ./internal/operations` completo (con `TestStageA1WaiterMode`)                                                                    | idem                                  | **ok** (359 s)                                                       |
| `go-int ./internal/apicontract` (`TestContractV1` con el 429 y el tipo por dispositivo)                                                  | idem                                  | **ok** (24 s)                                                        |
| `go-int ./internal/database` (`TestStageA1Upgrade` con las columnas y CHECK nuevos)                                                      | idem                                  | **ok** (11 s)                                                        |
| Vitest (`tsc --noEmit` + `vitest run src`)                                                                                               | `iso.sh vitest`                       | **96 archivos, 916 pruebas** (2 nuevas: bloqueo largo y fila propia) |
| `npm run build`                                                                                                                          | `iso.sh build`                        | Correcto                                                             |
| Playwright `workflow-stagea1` Chromium / WebKit                                                                                          | Go temporal 8186, vista previa 1424   | **1/1 y 1/1**                                                        |
| Playwright Chromium `workflow-stage9g-print-agent`, `-sold-out`, `-terminals`, `-two-terminals`, `workflow-stage9h-adjustments` (por B2) | idem                                  | **9/9**                                                              |
| `format`, `fmt-check`, `sync-check`; md5 de `docs/etapas/*.md`                                                                           | árbol `Bfix`                          | Correctos; idénticos                                                 |
| `cargo test`                                                                                                                             | —                                     | No se corrió: `api.rs` no cambió                                     |

`TestStageA1WaiterPIN` (`backend/internal/administration/pin_acceptance_test.go`) agrega:

- **09** dos bloqueos de 5 minutos (se dan por vencidos en la base), el tercero largo (`requires_unlock`, sin hora,
  mensaje con «administrador», `user.pin_locked` ×2 y `user.pin_locked_until_unlock` ×1 con el dispositivo); ni el PIN
  correcto, ni 25 horas, ni «Mi PIN» lo quitan; la lista y Personal › Usuarios lo informan; el administrador lo
  desbloquea (`user.pin_unlocked` con `requires_unlock`) y el mesero entra; una entrada correcta borra una serie de 2;
  una serie de más de 24 horas vuelve a empezar (el bloqueo es de 5 minutos).
- **10** el administrador con PIN: `auth/me` informa exactamente la lista de mesero, `restaurant/state` también; `GET
users`, `PUT users/{id}/pin`, `GET devices` y cuentas para llevar → 403 con su `access.denied`; cuentas y catálogo
  → 200; con su credencial el pedido sin conexión entra y `GET users` → 401 `OFFLINE_CREDENTIAL_NOT_ALLOWED`; «Cambiar
  contraseña» desde esa sesión sigue acotada; con contraseña en la computadora tiene todo.
- **11** la credencial en `setup/status`, `setup`, `devices/enroll`, `pairing/requests`, `auth/login`, `auth/pin-users`,
  `auth/pin-login`, `version`, `health/live`, `openapi.json`, `events` y `restaurant/offline-orders/` → 401; una
  inventada, también.
- **12** escritorio con `android` (con 0.1.0 y con la versión del producto) y teléfono con `desktop` → 400
  `CLIENT_KIND_MISMATCH`; sin cabeceras, 200.
- **13** (antes 09) ningún registro guarda los PIN, la contraseña nueva, la sesión ni la credencial; ahora quita fechas y
  UUID antes de buscar los PIN (podían contener por azar los mismos dígitos).

### No verificado

- La aplicación Android contra estas formas (la corrige el otro agente).
- La regresión completa de Playwright (Fase G); aquí solo las especificaciones de la tabla.
- `go-int` de los demás paquetes de la tupla (no se tocaron).
- El límite de turnos bajo carga real (solo la prueba unitaria del semáforo).

### Para el coordinador

- **Decidir** si acepta el ajuste de cabeceras en `TestContractV1` y `TestStageA1WaiterPIN` por B2 (eran pruebas que
  mandaban `android` con la credencial del escritorio; el escritorio, la app y Playwright no cambian).
- Observación sin cambio: con `accounts.open` (en la lista, como en el rol) una sesión con PIN puede, como hoy cualquier
  mesero con ese permiso, descartar una cuenta vacía, liberar una mesa y pedir «Cancelar cuenta» con la contraseña de un
  supervisor (`supervisor_approval.go`). No es administración y la app no lo ofrece en la A1; si el propietario quiere
  cerrar «Cancelar cuenta» con autorización desde un teléfono, es otra decisión.
- Para `CLAUDE.md` (no lo edité): §6, las subpruebas nuevas de `TestStageA1WaiterPIN`, `TestOfflineCredentialOnlyOnItsRoute`,
  `TestWaiterPermissionsAreClosedAndDocumented` y `TestPINLoginSlotsAreLimited`; §7, A1-D16, A1-D17 (la lista en
  `waiter_scope.go`, regla: solo crece con funciones de la app y se documenta en el contrato, `API_V1.md` y
  `API_CONTRACTS.md`; la prueba lo exige), B1, B2 (`CLIENT_KIND_MISMATCH`) y O4; §15, las decisiones 1–5.

**Al cerrar las correcciones del servidor** (rama `etapa-a1-b-fix` fusionada en `etapa-a1-android`): se acepta el ajuste de `TestContractV1` y `TestStageA1WaiterPIN`, que enviaban `X-Client-Kind: android` con la credencial de una terminal de escritorio (B2 lo rechaza con `400 CLIENT_KIND_MISMATCH`); el escritorio, la app y Playwright no cambian. Se conserva lo observado sobre `accounts.open` (descartar una cuenta vacía, liberar una mesa o pedir «Cancelar cuenta» con un supervisor, como cualquier mesero con ese permiso).

## Correcciones de la revisión R3 · Dispositivo

Rama `etapa-a1-cb-fix` (desde `etapa-a1-android` en `3ca8b6e`, con la fusión de `etapa-a1-android` en `b4e1cfe`:
servidor corregido), un agente, 16 y 17/11/2026. Árbol `$SP/wt/CBfix`, `ISO_TAG=C`, Go temporal en 8194 (local) y 8454
(red), uno nuevo por corrida. **Sin cambios en `backend/`, `desktop/`, `docs/API_V1.md`, `CLAUDE.md` ni `AGENTS.md`**
(`android/CLAUDE.md` y `scripts/android-e2e.sh` sí); `docs/etapas/*.md` con el mismo md5 antes y después. Ninguna
prueba usó el servicio real (8181, 8443, 55432) ni buscó `_adelpos._tcp`; no se abrió la aplicación normal fuera de
`AdelTestRunner`. Emuladores: `Pixel_Tablet` (5582) y `ADEL_A1_Tablet_API26` (5590), siempre con `ANDROID_SERIAL`,
apagados al terminar; nunca 5600–5606. Go temporal detenido y su base borrada.

Commits (rutas explícitas):

| Commit    | Tema                                                                                                                                                                        |
| --------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `00f16b0` | M4: `DeviceDataWipe` (registro único de borrado) y `eraseAll()` de `catalog.db`, la caché de imágenes y el catálogo en memoria                                              |
| `f71c53f` | B4: `LocalUserActivity` y `Modifier.reportsUserActivity` (`core:ui`); el menú del mesero los usa                                                                            |
| `6904e30` | A1, A2, M1, M2, M3, M5, A1-D16 y el cableado de M4/B4: dominio, datos (Room v3), `DeviceServerAccess`, «Vuelve a escribir tu PIN», textos de «Emparejar de nuevo» y pruebas |
| `9fa7cd9` | `android/CLAUDE.md`                                                                                                                                                         |
| `055df3a` | Fusión de `etapa-a1-android` (servidor corregido), sin conflictos                                                                                                           |
| `47b3347` | 503 `PIN_LOGIN_BUSY` se reintenta; 400 `CLIENT_KIND_MISMATCH` es un error de soporte                                                                                        |
| `21aeb3f` | Contrato M3/M4/M5 para la Fase E en `android/CLAUDE.md`, borrado fuera del hilo principal, pruebas instrumentadas nuevas                                                    |
| `260a77e` | `AccessOnGoTest`: bloqueo de Go que cierra la entrada sin conexión, cabeceras por mesero y «Vuelve a escribir tu PIN» contra el Go temporal                                 |
| `ee85a0f` | `scripts/android-e2e.sh`: sin `ADELPOS_E2E_SERVER_ID` ya no pasa un `serverId` vacío (am fallaba con «Invalid userId -2» y el guion salía sin decir nada)                   |

`00f16b0` y `f71c53f` son independientes; `6904e30` reúne los cambios que se necesitan entre sí (dominio, datos y
pantallas cambian juntos). No se comprobó con `git archive` que cada commit compile solo.

### Qué se corrigió

- **A1 y B7** (`core/data/.../access/AccessRepository.kt:574`, `enterOfflineLocked`): una entrada sin conexión borra
  la sesión de Go que el mesero tuviera en memoria (`:601`); solo una entrada con conexión pone sesión. La prueba que
  decía cubrirlo ahora mira la cabecera `Authorization` de una **petición real** al servidor simulado
  (`AccessSafetyTest.unaEntradaSinConexionNuncaLlevaLaSesionDeGo`) y `DeviceServerAccess` no da conexión autorizada a
  ese mesero (`DeviceServerAccessTest.unMeseroQueEntroSinConexionNoTieneConexionAutorizada`). El comentario de
  `OfflineAttemptPolicy` se reescribió.
- **A2 y B4**: el bloqueo por inactividad es del proceso. `IdleLock.supervise` (`core/domain/.../IdleLock.kt:74`)
  combina mesero activo, plazo de Go y primer plano (`AppForeground` de la Fase D) con `collectLatest`; lo arranca
  `AppContainer.startIdleLock()` (`app/.../AppContainer.kt:181`) desde `AdelPosApplication.onCreate`.
  `AccessViewModel` ya no vigila. Empezar a vigilar **no** reinicia el plazo (solo un toque o la entrada de otro
  mesero); al volver a primer plano se recalcula en ese momento con `elapsedRealtime`, así que tras un reposo
  profundo se bloquea sin esperar; la espera es exacta (sin vueltas de 60 s). Los diálogos y menús reportan sus
  toques (`Modifier.reportsUserActivity(LocalUserActivity.current)`, provisto en `AdelPosApp`). Las reglas del
  coordinador siguen: el bloqueo no ocurre con captura en curso ni pedidos sin enviar.
- **M1 y M2** (`OfflineAttemptPolicy.kt:33`, Room v3 con `offline_failures` y `offline_closed`, `MIGRATION_2_3` en
  `AppDatabase.kt:91`, SQL igual a `schemas/…/3.json`): sin ningún reloj. Los fallos con o sin conexión
  (`PIN_INVALID` también cuenta, `:486`) se acumulan y solo vuelven a cero con una entrada correcta; al quinto, la
  entrada sin conexión de ese mesero queda cerrada (`EntryResult.OfflineClosed`) hasta que entre con conexión. Un
  `PIN_LOCKED` de Go o un `locked_until`/`requires_unlock` en `auth/pin-users` (`:404`) también la cierra. La
  migración conserva los fallos y pasa un bloqueo por reloj vigente a «cerrada».
- **A1-D16**: `details.requires_unlock` de 429 `PIN_LOCKED` y `requires_unlock` de `auth/pin-users`
  (`EntryResult.Locked(untilEpochMillis, requiresUnlock)`, `WaiterProfile.requiresUnlock`). Texto «Tu PIN está
  bloqueado. Pide a un administrador que lo desbloquee.»; en la lista, «PIN bloqueado: pide a un administrador que lo
  desbloquee»; sin conexión, «Sin conexión no puede entrar: entra con conexión».
- **M3** (`report`, `:188`, y `accessEndedLocked`, `:212`; `invalidateLocked`, `:388`): `SESSION_*`, PIN cambiado
  (otra `pin_version` o fuera de la lista) y `OFFLINE_CREDENTIAL_INVALID` (además borra verificador y credencial: B3
  queda cubierto) nunca sacan al mesero: queda activo con `hasServerSession = false` y `mustReenterPin = true`, y
  `AdelPosApp` muestra `ReenterPinRoute` («Vuelve a escribir tu PIN, Ana») encima de su pantalla, sin cerrarse con
  Atrás ni fuera, con «Cambiar de mesero» bajo las reglas de siempre. Su navegación y lo capturado siguen debajo.
- **M4** (`forgetLocked`, `:661`): pedidos sin enviar → bitácora; `wipe.eraseAll()` (catálogo e imágenes hoy; la cola
  y WorkManager en la Fase E); olvidar emparejamiento y meseros; destruir la llave. «Emparejar de nuevo» muestra antes
  los pedidos sin enviar (`DeviceAccess.pendingOrders()`) y dice qué se borra; el texto de «retirado» también. El
  borrado corre en `Dispatchers.IO` y una parte que falla no detiene a las demás.
- **M5** (`requestHeaders(userId)`, `:255`): cabeceras del mesero dueño de la operación; `DeviceServerAccess`
  (`:30`) las pide para el mesero activo y no da conexión con `mustReenterPin`.
- **Del aviso del coordinador**: 503 `PIN_LOGIN_BUSY` se reintenta hasta 3 veces cada 2 s (`:434`) sin contar fallo,
  sin marcar sin conexión ni usar el verificador; después `EntryResult.Busy` («…muchos PIN a la vez. Inténtalo de
  nuevo en unos segundos.»). 400 `CLIENT_KIND_MISMATCH` no cambia nada del estado (ni sesión ni red) y la entrada
  muestra «…problema de configuración que debe revisar soporte (código CLIENT_KIND_MISMATCH)». La sesión acotada de
  A1-D17 no rompió nada (`restaurant/state` responde con las cabeceras de cada mesero en el paso 2).

### Formas que consume la Fase E (estables; descritas en `android/CLAUDE.md`, «Contrato con la cola y el modo Mesero»)

- `AccessRepository.requestHeaders(userId: String): Headers?` (antes `requestHeaders()`, sin argumentos).
- `report(error, userId)`: `SessionEnded` y `Rejected(401, AccessRepository.OFFLINE_CREDENTIAL_INVALID, …)` →
  `ActiveWaiter.mustReenterPin`; `offlineCredential(userId)` pasa a null con la segunda.
- `AppContainer.deviceDataWipe.register(name) { … }` (`DeviceDataPart.erase()` suspendida) y
  `DeviceAccess.pendingOrders()`.
- Otros cambios de forma: `ActiveWaiter(profile, hasServerSession, mustReenterPin = false)`; `EntryResult` suma
  `OfflineClosed` y `Busy`, y `Locked` suma `requiresUnlock`; `WaiterProfile` suma `requiresUnlock` y
  `offlineEntryClosed`; el constructor de `AccessRepository` ya no recibe `work` (no lo usaba) y recibe `wipe` y
  `busyRetryMillis`.

### Pruebas ejecutadas (todas con el código final, `ee85a0f`)

| Qué                                                                                             | Dónde                           | Resultado                                                                                                        |
| ----------------------------------------------------------------------------------------------- | ------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| JVM + Robolectric + Lint (`:core:domain:test testDebugUnitTest lintDebug`, con `--rerun-tasks`) | Mac                             | **176 pruebas, 0 fallas** (contadas en los XML: dominio 74, datos 84, ui 9, access 6, waiter 3); Lint sin avisos |
| Instrumentadas (`connectedDebugAndroidTest`)                                                    | `Pixel_Tablet` (API 37)         | **54 pruebas, 0 fallas**, 12 omitidas (las de Go sin argumentos)                                                 |
| Instrumentadas                                                                                  | `ADEL_A1_Tablet_API26` (API 26) | **54 pruebas, 0 fallas**, 13 omitidas (además, el modo avión por shell)                                          |
| `scripts/android-e2e.sh` pasos 1–5, Go nuevo, pasos 6–7                                         | `Pixel_Tablet`                  | 7/7 aprobados                                                                                                    |
| `scripts/android-e2e.sh` pasos 1–5 (red cortada desde la Mac en el 3), Go nuevo, pasos 6–7      | `ADEL_A1_Tablet_API26`          | 7/7 aprobados                                                                                                    |
| `iso.sh format`, `fmt-check`, `ktfmtCheck`                                                      | Mac                             | sin cambios pendientes                                                                                           |

Las pruebas de la revisión (`$SP/r3/.../R3ReviewTest.kt`, `R3IdleLockTest.kt`) quedaron en el repositorio al revés, en
`core/data/src/test/.../access/AccessSafetyTest.kt` (A1, M1, M2, A1-D16, M3 ×3, M4, M5, 503 y 400) y
`AccessViewModelsTest.volverAAbrirLaPantallaNoReiniciaElPlazoDelMeseroAnterior` (A2 con reposo profundo simulado), más
`AccessRulesTest` (dominio), `AppDatabaseMigrationTest.de2a3…`, `CatalogSyncTest`/`CatalogImageCacheTest` (borrado),
`AppNavigationTest.unaSesionTerminadaPideVolverAEscribirElPinSinSalirDeMesas` (en el dispositivo: el diálogo sobre
Mesas y que un toque en él reinicia el plazo) y `AccessScreensTest` (confirmación con lo que se borra y los
pendientes). Contra el Go temporal: un `PIN_LOCKED` real cierra la entrada sin conexión de Beto (paso 2) y sin red no
entra ni con el PIN correcto (paso 3); `restaurant/state` responde a nombre de Ana y de Beto con sus cabeceras con Ana
activa (paso 2); el PIN cambiado mientras Ana trabaja pide volver a escribirlo sin salir de Mesas (paso 4).

### No verificado

- El bloqueo largo de A1-D16 contra Go (pide 3 bloqueos de 5 minutos en 24 horas): solo en la JVM con el servidor
  simulado en la forma del contrato. Igual 503 `PIN_LOGIN_BUSY` y 400 `CLIENT_KIND_MISMATCH`.
- Atrás en la raíz de Android 8–11 con el proceso vivo y la vuelta tras un reposo profundo real: probados con tiempo
  virtual y un reloj que avanza dormido; no en el emulador.
- B4 se probó en el dispositivo con el diálogo de volver a escribir el PIN; el menú del mesero y los demás
  `AlertDialog` usan el mismo modificador sin prueba propia.

### Para el coordinador

- La Fase E debe guardar el `user_id` dueño de cada operación y registrar la cola en `deviceDataWipe`; el resto está
  en `android/CLAUDE.md`.
- El guion de extremo a extremo tenía un error que no se vio en la corrida anterior porque entonces se pasaba
  `ADELPOS_E2E_SERVER_ID`; corregido en `ee85a0f`.

**Al cerrar las correcciones del dispositivo** (rama `etapa-a1-cb-fix`, que ya traía fusionado el servidor corregido, fusionada en `etapa-a1-android`): se acepta que la versión 3 de `AppDatabase` sea de estas correcciones (fallos acumulados y entrada sin conexión cerrada por mesero); la cola de la Fase E pasa a la versión 4. Quedan sin verificar en el dispositivo el bloqueo largo contra Go, `503 PIN_LOGIN_BUSY` y `400 CLIENT_KIND_MISMATCH` (solo JVM) y Atrás con el reposo real en Android 8–11 (tiempo virtual); la Fase F los incluye.

---

## Revisión independiente R6 · Cola sin conexión (§6 y §7)

Revisor independiente de §6–§7 (especificación §0, «Eficiencia»), 17/11/2026, en paralelo con la Fase F. Árbol
`$SP/wt/R6` (rama `etapa-a1-r6` = `etapa-a1-android` en `61515c1`), **solo lectura**: no se cambió nada del árbol. Las
pruebas de comprobación se escribieron y corrieron en una copia (`git archive` → `$SP/r6/tree`), con `ISO_TAG=R6` y el
clúster temporal 55499. No se usaron emuladores ni Go temporal (no hicieron falta); nunca el servicio real de la Mac.

Marco: §0-B, §6, §7 y §9 de la especificación; A1-D1, A1-D6, A1-D11, A1-D13, A1-D15 a A1-D17 y las secciones de la
Fase E del avance (usados para entender la intención, no como prueba). Rutas de Android desde `android/`:
`…/data` = `core/data/src/main/kotlin/local/adel/pos/android/core/data`, `…/domain` =
`core/domain/src/main/kotlin/local/adel/pos/android/core/domain`.

### Resumen

El camino principal de la cola está bien resuelto: cada pedido usa su clave de siempre con la credencial del mesero que
lo capturó, la respuesta perdida no duplica (Go serializa y repite la respuesta), los estados no se confunden y Room
conserva todo tras reiniciar. Los defectos aparecen en los **bordes entre el borrador guardado en Go y la cola**:

- **C1 (crítico)**: «Enviar a cocina» con un guardado del borrador sin respuesta (red lenta: la espera es de 6 s y la
  lectura de 15 s) deja en el borrador de Go lo que ese guardado escribió; el pedido entra y esos renglones siguen «por
  enviar» en la cuenta: el siguiente «Enviar a cocina» de cualquier terminal los manda **otra vez**. Reproducido en el
  dispositivo y en Go.
- **A1 (alto)**: si `confirmDraft` entró en Go y su respuesta se pierde (o se cierra la app a media confirmación), el
  pedido sale después como pedido sin conexión **con otra clave**; Go lo marca `draft_item_sent` (aceptable) y la
  pantalla ofrece «Aceptar los cambios y enviar» como botón principal: un toque lo manda dos veces. Reproducido.
- **A2 (alto)**: «Descartar la captura» (y «Descartar el pedido») solo borran en el dispositivo; lo ya guardado en el
  borrador de Go se queda en la cuenta aunque el diálogo diga «Se quitan los productos que no se han enviado a cocina».
  Reproducido.
- Medios: borrar los datos del dispositivo durante un envío **resucita** el pedido (M1, reproducido); «Corregir en la
  captura» sobrescribe sin aviso el cambio de otra terminal (M2, reproducido); cuenta combinada sin salida útil y con
  riesgo de duplicado (M3, leído).

### Hallazgos

| ID  | Gravedad    | Archivo y línea                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | Qué está mal                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | Escenario (entrada → resultado incorrecto)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | Cómo se comprobó                                                                                                                                                                                                                                                                                                                              | Corrección sugerida                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| --- | ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| C1  | **crítico** | `…/data/waiter/CaptureRepository.kt:308-341` (`sendToKitchen`), `:397-402` (`settle`, 6 s, `SETTLE_TIMEOUT_MILLIS` `:860`) frente a `…/data/api/PinnedHttpClient.kt:15` (lectura 15 s); `…/domain/waiter/Capture.kt:115-141` (`OfflineOrderPlan`: `replaces` solo sale de `base`); servidor `backend/internal/operations/offline_orders.go:661,670` (acepta y solo quita `replacedDraft`) y `drafts.go:56-63` (no rechaza un id que ya está en el borrador de **la misma** cuenta) | Con un guardado del borrador en curso o incierto (`pendingOperationId`), «Enviar a cocina» no espera a resolverlo: el pedido lleva los renglones nuevos como partidas, pero `replaces_draft_items` se calcula contra la última base **confirmada**. Si ese guardado sí entró en Go, Go acepta el pedido y deja esos renglones en el borrador                                                                                                                                                                                                                                                                                               | Wifi lenta: Ana agrega «Papas ×2» en una cuenta abierta; el `PUT …/draft` sale y su respuesta tarda más de 6 s (o se pierde); toca «Enviar a cocina» → pedido «Por enviar» sin `replaces`; Go había guardado el renglón → el pedido entra (papas a cocina) **y** el renglón sigue «por enviar» en la cuenta (Go no deja cerrarla así); el cajero, otra tableta o la propia Ana en línea (`confirmDraft` confirma el borrador completo) lo envía → **papas dos veces en cocina**. Variante con la cuenta todavía pendiente (`openAccount` incierto): la cuenta nace en Go con el borrador; el pedido a la mesa recibe `table_has_account` (su propia cuenta) y, al aceptarlo, 400 «la partida pertenece a otra cuenta» → «Requiere tu revisión» sin explicación útil, y la cuenta fantasma queda abierta | `R6ReviewTest.guardadoInciertoAlEnviarDejaLoGuardadoFueraDeReplaces` (pasa: el `PUT` llevaba `r1`, el pedido lleva `r1` y `replaces` vacío) y `TestR6Review/A1` en Go (pasa: tras aceptar el pedido el renglón sigue en el borrador y, al confirmar el borrador, hay **2** partidas con ese `draft_item_id`). Variante `openAccount`: lectura | Con red: no armar el pedido mientras haya un guardado sin resolver (repetirlo antes, como `draftSync`; si no se resuelve, avisar «Guardando…» en lugar de pasar a la cola). Sin red: hace falta que Go reconozca el renglón por su id. Lo más sólido (cambio compatible en Go, a decidir): al aceptar un pedido con `account_id`, quitar también del borrador los renglones cuyo id coincide con una partida del pedido (el id lo generó el dispositivo; es el mismo renglón) y avisar `draft_item_changed` si su contenido difiere; para la apertura incierta, que el pedido pueda llevar la clave de esa apertura o que Go acepte como destino la cuenta que abrió el mismo dispositivo con esos ids             |
| A1  | **alto**    | `…/data/waiter/CaptureRepository.kt:712` (la confirmación en línea usa una clave nueva, distinta de la del pedido, `:325`), `:724-741` (dos intentos y luego `ContinueOffline`); `…/domain/waiter/OrderSendRules.kt:81-87` y `:111-120` (`recoverInterrupted` tira `onlineConfirm`); servidor `offline_orders.go:475-495` (`draft_item_sent` aceptable); `feature/waiter/…/pending/PendingScreens.kt:389-396`                                                                      | Si `confirmDraft` entró en Go y no llegó la respuesta (dos veces) o el proceso murió a media petición, el pedido pasa a la cola **con otra clave** (la de la captura). Go no puede reconocerlo como repetición: responde 409 con `draft_item_sent` «ya se había enviado desde este dispositivo… puede repetirlo», **aceptable**. La regla de §7 («el reintento usa la misma clave y nunca duplica») no se cumple en este camino: evitar el duplicado depende de que el mesero lea el mensaje y elija «Descartar el pedido»; además, si lo descarta, el pedido nunca se ve «Recibido» aunque entró                                          | Con conexión, «Enviar a cocina» → `confirmDraft` entra; la respuesta se pierde y la repetición de 1.5 s ya no sale (wifi caída) → «Por enviar»; al volver la red → «Requiere tu revisión» con el botón principal «Aceptar los cambios y enviar» → un toque y cocina recibe todo dos veces                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | `R6ReviewTest.yaEnviadoDesdeEsteDispositivoSeOfreceAceptarYSeReenvia` (pasa: `canAccept` verdadero y el segundo envío llega a «Recibido») y `TestR6Review/A3` en Go (pasa: tras `confirmDraft`, el pedido con otra clave, aceptado, deja **2** partidas con el mismo `draft_item_id`)                                                         | Mientras la confirmación en línea esté incierta, conservar `onlineConfirm` y repetir `confirmDraft` con **su** clave cuando el mesero tenga sesión (después de un reinicio no la tiene: ahí hace falta Go). En Go (a decidir): si **todos** los renglones que el pedido sustituye están `draft_item_sent` desde el mismo dispositivo y el mismo usuario, no ofrecerlo como aceptable (un `kind` propio o `acceptable:false`) o responder como «ya entró», para que la app lo marque recibido sin decidir nada por su cuenta. Nota: si se reutilizara la misma clave, Go respondería `IDEMPOTENCY_CONFLICT` (`handler.go:324-330`) y la app hoy lo convertiría en «Requiere tu revisión» (`OrderSender.kt:161-183`) |
| A2  | **alto**    | `…/data/waiter/CaptureRepository.kt:344-350` (`discard`); `feature/waiter/…/capture/CaptureViewModel.kt:284-289`; texto `feature/waiter/src/main/res/values/strings.xml:68`; `…/data/waiter/OrderQueueRepository.kt:103-114` (descartar un pedido en revisión con `replaces`), texto `:131`                                                                                                                                                                                        | Descartar borra la captura o el pedido **solo en el dispositivo**: no guarda el borrador sin los renglones propios ni descarta la cuenta que nació con esa captura. El diálogo promete que se quitan                                                                                                                                                                                                                                                                                                                                                                                                                                       | Ana agrega «Hamburguesa» en línea (queda guardada en Go), se arrepiente y toca «Descartar la captura» → la cuenta sigue con «Hamburguesa por enviar» (si la cuenta nació con esa captura, la mesa queda ocupada); el escritorio o el siguiente «Enviar a cocina» la manda a cocina. Igual con «Descartar el pedido» de un pedido cuyos renglones ya estaban guardados                                                                                                                                                                                                                                                                                                                                                                                                                                   | `R6ReviewTest.descartarLaCapturaNoQuitaDeGoLoGuardado` (pasa: ninguna petición a Go al descartar; el único `PUT` es el que agregó el renglón)                                                                                                                                                                                                 | Con sesión: `saveDraft` sin los renglones propios (conflicto explícito si la cuenta cambió) y, si la cuenta nació de la captura y queda vacía, descartarla; sin conexión: guardar el descarte pendiente (o no permitirlo y explicarlo). Mismo arreglo para «Descartar el pedido» con sus `replaces`                                                                                                                                                                                                                                                                                                                                                                                                                |
| M1  | medio       | `app/src/main/kotlin/local/adel/pos/android/AppContainer.kt:141-144` (`eraseAll` y después `cancelAll`, sin esperar al trabajador); `…/data/waiter/OrderSender.kt:78-91` (al cancelarse guarda con `NonCancellable`), `:110-112`; `…/data/waiter/db/WaiterEntities.kt:183` (`@Upsert`)                                                                                                                                                                                             | El trabajador escribe la fila completa con `Upsert` desde su copia en memoria: si el borrado ocurre mientras espera la respuesta (o lo cancela `cancelAllWork`), al terminar **vuelve a insertar** el pedido borrado                                                                                                                                                                                                                                                                                                                                                                                                                       | «Emparejar de nuevo» (o el retiro) con el trabajador esperando al servidor → el pedido pasa a la bitácora («no se envió») y se borra… y reaparece «Por enviar»: el indicador lo cuenta para siempre y, si se vuelve a emparejar con el mismo servidor y el mesero entra con conexión, se envía (posible duplicado de lo que se recapturó a mano después de verlo en la bitácora)                                                                                                                                                                                                                                                                                                                                                                                                                        | `R6ReviewTest.borrarDuranteUnEnvioDejaVivoElPedido` y `cancelarElTrabajadorDespuesDeBorrarDejaVivoElPedido` (pasan: la fila vuelve en `queued`)                                                                                                                                                                                               | Cancelar y **esperar** el trabajo antes de borrar (`cancelUniqueWork(...).result` y el mismo candado que `OrderSender`), y que las escrituras del envío sean `UPDATE … WHERE operation_id = ? AND state = ?` (nunca `Upsert`): una fila borrada no vuelve y un salto prohibido no se escribe (ver O1)                                                                                                                                                                                                                                                                                                                                                                                                              |
| M2  | medio       | `…/data/waiter/OrderQueueRepository.kt:203-249` (`captureFrom`, `baseRevision = 0` en `:238`); `…/data/waiter/CaptureRepository.kt:425-436` (`syncOnce` lee y guarda encima), `:620` y `:293`                                                                                                                                                                                                                                                                                      | «Corregir en la captura» arma una captura con la base que el pedido vio (`replaces`) pero con revisión 0, la marca de «base nunca leída». Al guardar, adopta el borrador **vigente** de Go y le aplica `Replace`/`Remove` sin conflicto. Responde a la pregunta de `doc-E` («una base de Go que no se ha leído nunca se sobrescribe»): significa que una captura con revisión 0 lee primero el borrador de Go y aplica sus toques encima en lugar de reemplazarlo. Es correcto para una captura que **empezó sin conexión** (sus toques solo tocan renglones nuevos), pero no para la que nace de un pedido: sus toques sí vieron una base | El pedido de Ana sustituye `r1` (2 → 3); otra terminal lo dejó en 5 → 409 `draft_item_changed` («el cambio de la otra terminal se queda sin enviar») → Ana toca «Corregir en la captura» → `PUT` con `r1 = 3` sobre la revisión 7: el 5 de la otra terminal se pierde sin aviso. Con `draft_item_removed`/`draft_item_sent`, `Replace` vuelve a agregar el renglón al borrador (y el siguiente envío lo repite)                                                                                                                                                                                                                                                                                                                                                                                         | `R6ReviewTest.corregirEnLaCapturaSobrescribeElCambioDeOtraTerminal` (pasa: un solo `PUT`, revisión 7, cantidad 3, nunca `CONFLICT`)                                                                                                                                                                                                           | En `captureFrom`, distinguir «base vista sin revisión» de «base nunca leída» (p. ej. guardar la revisión del rechazo o una marca): con una revisión nueva y toques, `CONFLICT` con «Conservar mis cambios / Descartarlos», como cualquier captura                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| M3  | medio       | `…/domain/waiter/OrderReviewRules.kt:63-77` (`moveToNewAccount` conserva los ids); `…/data/waiter/CaptureRepository.kt:831-855` (`movedToNewAccount` sí los renombra); servidor `merges.go:66` (la combinación mueve el borrador a la cuenta destino con los mismos ids) y `drafts.go:56-63`                                                                                                                                                                                       | Con una cuenta **combinada**, la revisión solo ofrece «Abrir una cuenta nueva en la mesa». Desde la cola, el pedido conserva los ids y Go lo rechaza con 400 (los renglones son ahora del borrador de la cuenta destino) → «Requiere tu revisión» otra vez, sin salida útil. Desde la captura, los ids nuevos entran, pero los renglones movidos siguen «por enviar» en la cuenta destino → duplicado cuando alguien la envíe. No se ofrece seguir la combinación (`merged_into_id` llega en `details`)                                                                                                                                    | Ana guardó «Tacos» en la cuenta A; en caja combinan A en B; Ana (sin conexión) envía → `account_closed` (merged) → «Abrir una cuenta nueva» → 400 «la partida pertenece a otra cuenta»                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | Lectura (no ejecutado; transferir o combinar desde otra terminal quedó «no verificado» en la Fase E)                                                                                                                                                                                                                                          | Para `merged`, ofrecer «Enviar a la cuenta en que se combinó» (destino `merged_into_id` con los mismos `replaces`, que Go encontrará allí); para una cuenta nueva, renombrar ids en los dos caminos y avisar que lo guardado se quedó en la otra cuenta                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| B1  | bajo        | `…/data/waiter/OrderSendWork.kt:43-53` y `:97-104`                                                                                                                                                                                                                                                                                                                                                                                                                                 | Despertar perdido: si `requestSend` llega entre el último `again.getAndSet(false)` del trabajador y su `running.decrementAndGet()`, deja `again = true` y no programa nada                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | Un pedido nuevo se queda «Por enviar» con conexión hasta el periódico (≥ 15 min) o el siguiente evento (red, primer plano, entrada)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | Lectura                                                                                                                                                                                                                                                                                                                                       | Al salir del trabajador, si `again` quedó en `true`, encolar; o `enqueueUniqueWork` siempre con `APPEND_OR_REPLACE`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| B2  | bajo        | `…/data/waiter/OrderSender.kt:75`, `:78` y `:90-91` (fuera del `try`); `app/…/WaiterModeContainer.kt:95-99` (`recoverAfterRestart` solo al arrancar)                                                                                                                                                                                                                                                                                                                               | Si WorkManager detiene el trabajador (límite de ejecución, `cancelAllWork`, `REPLACE` en la carrera de `running`) justo después de guardar «Enviando» o antes de guardar el resultado, el pedido queda `SENDING`; el trabajador solo toma `QUEUED`                                                                                                                                                                                                                                                                                                                                                                                         | «Enviando» durante todo el turno con la app abierta: cuenta en el indicador e impide cambiar de mesero hasta reiniciar el proceso (no se pierde)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | Lectura                                                                                                                                                                                                                                                                                                                                       | Abarcar de `start` a `save(next)` con la recuperación, o recuperar al inicio de cada `sendPending` los `SENDING` sin `onlineConfirm` (son del propio trabajador, bajo su candado)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| B3  | bajo        | `…/data/waiter/OrderSender.kt:62-65` y `:71-76`; `OrderQueueRepository.kt:109` (solo se descarta en revisión)                                                                                                                                                                                                                                                                                                                                                                      | Los pedidos de un mesero desactivado o al que le quitaron el permiso quedan «Por enviar» con la credencial rechazada **para siempre**: nadie puede verlos ni descartarlos (los demás solo ven el número)                                                                                                                                                                                                                                                                                                                                                                                                                                   | Se desactiva a Ana con 2 pedidos en la cola → el indicador muestra 2 pendientes indefinidamente; solo «Emparejar de nuevo» los pasa a la bitácora                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Lectura                                                                                                                                                                                                                                                                                                                                       | Decidir: mostrarlos a un administrador o pasarlos a la bitácora con su aviso cuando Go diga que el usuario ya no existe o no tiene permiso                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| B4  | bajo        | `…/domain/waiter/OrderFactory.kt:81` (`capturedAtMillis` = reloj del dispositivo); servidor `offline_orders.go:187-189`                                                                                                                                                                                                                                                                                                                                                            | Un dispositivo con el reloj adelantado más de 10 minutos recibe 400 («La hora de captura… es posterior») → «Requiere tu revisión» sin salida: «Corregir en la captura» vuelve a usar el mismo reloj                                                                                                                                                                                                                                                                                                                                                                                                                                        | Tableta con la hora mal → ningún pedido sin conexión entra                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | Lectura                                                                                                                                                                                                                                                                                                                                       | Aviso claro de soporte («Revisa la hora de este dispositivo») o usar la diferencia con la hora del servidor conocida                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| B5  | bajo        | `AppContainer.kt:143` (`cancelAllWork`) y `app/…/AdelPosApplication.kt:32`                                                                                                                                                                                                                                                                                                                                                                                                         | El borrado cancela también el envío periódico, que solo se vuelve a programar al arrancar el proceso                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Tras «Emparejar de nuevo» sin cerrar la app, la red de respaldo de 15 min no existe hasta el siguiente arranque                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         | Lectura                                                                                                                                                                                                                                                                                                                                       | Volver a programar el periódico al emparejar (o cancelar solo `SEND_WORK`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| B6  | bajo        | `PendingScreens.kt:389-428`; `OrderReviewRules.kt:80-82`                                                                                                                                                                                                                                                                                                                                                                                                                           | Con `table_unavailable` (mesa fuera de servicio o borrada), Go dice «Elige otra mesa», pero la revisión solo ofrece corregir en la **misma** mesa o descartar                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | El pedido no puede ir a otra mesa; hay que descartarlo y recapturar                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | Lectura                                                                                                                                                                                                                                                                                                                                       | Ofrecer «Enviar a otra mesa» (destino mesa con su `seen_account_ids`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| O1  | observación | `WaiterEntities.kt:183`; `OrderSender.kt:104-112`; `OrderQueueRepository.kt:120-149`; `CaptureRepository.kt:733-740`                                                                                                                                                                                                                                                                                                                                                               | Las transiciones se comprueban sobre la copia en memoria y se escriben con `Upsert` de la fila completa: la base **no** impide un salto prohibido ni una escritura perdida. Ejemplo: `recoverAfterRestart` en paralelo con el primer envío del trabajador al arrancar puede devolver un `RECEIVED` a `QUEUED`; se reenvía con la misma clave y Go devuelve lo mismo (sin duplicar)                                                                                                                                                                                                                                                         | —                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Lectura                                                                                                                                                                                                                                                                                                                                       | `UPDATE … SET state = :to WHERE operation_id = :id AND state = :from` y comprobar filas (arregla también M1)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| O2  | observación | `…/domain/waiter/OrderQueue.kt:100-103`                                                                                                                                                                                                                                                                                                                                                                                                                                            | El comentario de `ChangeKind.UNKNOWN` dice «se trata como bloqueo», pero el código (correctamente) usa el `acceptable` de Go y devuelve el `kind` original en la confirmación                                                                                                                                                                                                                                                                                                                                                                                                                                                              | —                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Lectura                                                                                                                                                                                                                                                                                                                                       | Corregir el comentario                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| O3  | observación | `OrderSender.kt:147-152`                                                                                                                                                                                                                                                                                                                                                                                                                                                           | `OFFLINE_ORDER_ALREADY_ACCEPTED` sin `details` deja `receivedAccountId = ""` para un pedido a una mesa                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | Inocuo hoy                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | Lectura                                                                                                                                                                                                                                                                                                                                       | `null` en lugar de cadena vacía                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| O4  | observación | `…/domain/waiter/Capture.kt:95-96`                                                                                                                                                                                                                                                                                                                                                                                                                                                 | Una captura sin conexión cuyos toques se anulan (agregar y quitar) en una cuenta existente sigue «en curso» hasta reconectar e impide el bloqueo por inactividad                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | Menor                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | Lectura                                                                                                                                                                                                                                                                                                                                       | Considerar `visible` contra `base` además de `operations`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |

### Respuestas a las preguntas guía

- **Nunca duplicar ni perder.** La respuesta perdida de `submitOfflineOrder`, «Reintentar ya» junto con WorkManager, dos
  trabajadores (periódico y único: el mismo `OrderSender` con su candado) y el proceso muerto entre «Enviando» y la
  respuesta (`recoverAfterRestart` → misma clave) **no** duplican: Go toma el candado exclusivo 7820148305 en toda
  escritura y repite la respuesta guardada (`handler.go:80-83`, `:298-332`). Sí duplican C1 y A1; A2, M2 y M3 dejan
  renglones «por enviar» que otro puede mandar. La **misma clave no pasa** de `confirmDraft` a `submitOfflineOrder`: son
  dos claves (A1). Pérdida: ningún camino pierde un pedido confirmado (cerrar, reiniciar, batería: `queueFromCapture` es
  una transacción de Room y cada cambio es una escritura; migración 3 → 4 sin `fallbackToDestructiveMigration`;
  revocación, «Emparejar de nuevo» y «Actualiza esta aplicación» los muestran y los pasan a la bitácora). El problema del
  borrado es el contrario: el pedido **vuelve** (M1).
- **Estados (§7).** Cinco estados distintos y visibles (`OrderStateBadge`); «Recibido» solo con 200 o
  `OFFLINE_ORDER_ALREADY_ACCEPTED` de Go (`OrderSendRules.kt:54-55`); un borrador nunca sale solo (solo `sendToKitchen`
  crea pedidos y solo `accept`/`resend`/`moveToNewAccount` los reenvían). Las transiciones prohibidas **no** están
  impedidas por lo que escribe en Room (O1, M1).
- **Requiere tu revisión (§6.4).** Precio, agotado, cuenta cobrada, mesa y `draft_item_*` se muestran con el mensaje de
  Go; solo se reenvía tras la acción del mesero y con la misma clave; un `kind` desconocido se muestra y se confirma tal
  cual (O2 es solo el comentario); si cambia algo otra vez entre aceptar y reenviar, Go vuelve a rechazar con los cambios
  vigentes y la app los muestra (la confirmación anterior se borra, `OrderSendRules.kt:134-142`). Defectos: A1 (aceptar
  lo «ya enviado desde este dispositivo»), M3 (combinada), B6 (mesa).
- **Credencial de envío y dueño.** Cada envío usa las cabeceras y la credencial del mesero que capturó
  (`OrderSender.kt:71`, `CaptureRepository` con `capture.waiterUserId`/`order.waiterUserId`); `OFFLINE_CREDENTIAL_INVALID`
  conserva el pedido y pide «Vuelve a escribir tu PIN» sin sacar al mesero; `SESSION_*` igual; `DEVICE_REVOKED` retira
  con bitácora; 426 muestra «Actualiza esta aplicación» con los pendientes. Los permisos los decide Go
  (`WaiterPermissions` solo deshabilita botones).
- **WorkManager.** Trabajo normal (no urgente), sin restricción de red, espera exponencial desde 10 s, periódico de 15
  min; sin servidor no hay bucle (cada intento sin respuesta termina en `retry()` con espera creciente; el primer plano y
  la red reinician la espera con `REPLACE`). Un pedido a la vez, del más viejo al más nuevo, todos los meseros en la
  misma vuelta; se detiene con la primera falta de respuesta. Defectos menores B1, B2, B5. Doze no se observó.
- **Bloqueos y compuertas.** «Cambiar de mesero» lo impiden los pedidos sin enviar del mesero (`AccessViewModel.kt:100`);
  el bloqueo por inactividad, la captura en curso y los pedidos (`IdleLock.kt:56`); no existe «Cerrar sesión». El
  contador del indicador cuenta `QUEUED`, `SENDING` y `NEEDS_REVIEW` de todo el dispositivo (decisión 8); con M1 contaría
  pedidos borrados.
- **Datos por mesero.** Capturas, pedidos y navegación por `userId` (`AdelPosApp.kt:142`, `key(waiter.userId)`); de los
  demás solo el número. Sin defectos.
- **Borrador del servidor (A1-D6).** `replaces_draft_items` se calcula con la última base que Go **confirmó**, no con lo
  que Go pudo guardar después (C1). Lo de otra terminal no se borra en silencio por el pedido, pero sí por «Corregir en la
  captura» (M2). El significado de «una base de Go que no se ha leído nunca se sobrescribe» está en M2.
- **La app no decide reglas de negocio.** No hay aritmética de precios ni totales en el modo Mesero (búsqueda de
  `times`/`plus`/`multiply`/`sumOf` sin resultados de importes); el precio visto es el de la cotización de Go o el de
  referencia del catálogo; Go recalcula todo al recibir.
- **Calidad.** Código claro, sin compactar, con comentarios en español donde hay reglas. Las pruebas cubren lo que dicen,
  pero ninguna cubre un guardado en curso al enviar, el descarte con renglones guardados, el borrado durante un envío, la
  captura desde un pedido con cambios de otra terminal ni la confirmación en línea aplicada sin respuesta (las
  reproducciones de arriba son esas pruebas faltantes).

### Lo que se revisó sin encontrar defectos

- Idempotencia del servidor para `submitOfflineOrder`: candado exclusivo, `replay` por clave, rechazo sin guardar la
  clave, `OFFLINE_ORDER_ALREADY_ACCEPTED` solo al mismo usuario y dispositivo; `confirmLines` con `draft_item_id`.
- `OrderSender`: orden por `captured_at`, credencial del dueño, interpretación de 200, 409 (con `details` mal formados →
  reintento), 401 de credencial, retiro, sesión, 426, `CLIENT_KIND_MISMATCH`, permisos y validación.
- Cancelación durante la petición (`await` cancelable; el pedido vuelve a «Por enviar» con su clave).
- `OrderReviewRules` (aceptar solo con cambios aceptables; quitar partidas; reenviar sin confirmación cuando ya no hay
  cambios) y `OrderTransitions` en el dominio.
- `queueFromCapture`/`captureFromOrder` en una transacción; Room v4 y su migración 3 → 4 (solo crea tablas).
- El registro de borrado pasa lo no enviado (pedidos y capturas) a la bitácora antes de borrar.
- Ninguna escritura usa la sesión o la credencial del mesero activo en lugar de la del dueño.
- `seen_unit_price` sin cálculo local; cotización por renglón en el orden del borrador.

### No revisado o no verificado

- Nada se ejecutó en emulador ni en dispositivo (no hizo falta para reproducir); Doze, el límite de ejecución de
  WorkManager y la aplicación cerrada mucho tiempo, solo por lectura.
- La variante `openAccount` de C1, M3 (combinada) y B1–B6: solo lectura.
- El escritorio frente a los renglones que quedan en el borrador (C1, A2): se dedujo de `confirmations.go` (confirma el
  borrador completo) y de que Go no cierra una cuenta con borrador (avance, Fase E, decisión 7).
- Las pantallas Compose (textos, temas) solo por lectura; el cliente de tiempo real solo en lo que usa
  `RestaurantRepository` (`aggregateId` → nueva lectura).

### Pruebas ejecutadas (solo en la copia `$SP/r6/tree`)

| Prueba                                                                                                    | Dónde                                                                                               | Resultado                                                                                            |
| --------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `R6ReviewTest` (6 pruebas nuevas, Robolectric con Room y MockWebServer): C1, A1, A2, M1 (2 variantes), M2 | `ISO_TAG=R6 iso.sh $SP/r6/tree gradle :core:data:testDebugUnitTest --tests …R6ReviewTest --offline` | **6/6 pasan** (cada una reproduce su hallazgo; XML en `$SP/r6/results/`, log `$SP/r6/gradle-r6.log`) |
| `TestR6Review` (Go, 2 subpruebas): C1 y A1 contra PostgreSQL                                              | `ISO_TAG=R6 ISO_RUN=TestR6Review iso.sh $SP/r6/tree go-int ./internal/operations` (clúster 55499)   | **ok** (12 s; las dos encuentran 2 partidas con el mismo `draft_item_id`); log `$SP/r6/go-r6.log`    |

Archivos de las pruebas: `$SP/r6/tree/android/core/data/src/test/kotlin/local/adel/pos/android/core/data/waiter/R6ReviewTest.kt`
y `$SP/r6/tree/backend/internal/operations/r6_review_test.go`. El árbol `$SP/wt/R6` quedó sin cambios.

### Decisiones sobre la revisión R6

Del coordinador, porque aplican reglas que ya están en la especificación (§7: «nunca duplica el pedido»; §0-B: nada se
muestra como recibido sin Go) y en A1-D6 (nunca se borra en silencio lo de otra terminal). Se corrigen C1, A1, A2, M1,
M2, M3 y los bajos B2, B3, B4 y B5 (y O1 con M1); B1, B6, O2–O4 quedan anotados.

- **A1-D18 · El servidor nunca repite lo que ya entró** (cambio compatible de `submitOfflineOrder`, en la misma línea de
  A1-D6):
  - Un pedido sin conexión con la **misma clave de operación** que un «Enviar a cocina» (`confirmDraft`) ya aplicado
    en esa cuenta se reconoce como el mismo pedido: responde como ya aceptado (`OFFLINE_ORDER_ALREADY_ACCEPTED`) y no
    vuelve a mandar nada.
  - Los renglones del borrador **con el mismo id** que una partida del pedido se tratan como sustituidos aunque no
    vengan en `replaces_draft_items`: si siguen idénticos, se quitan en la misma transacción; si cambiaron en otra
    terminal, es `draft_item_changed` y el mesero lo confirma.
  - Confirmar un `draft_item_sent` significa «enviar lo demás»: lo que ya se envió **nunca** se reenvía.
  - Un pedido para una mesa cuyas partidas ya están en una cuenta abierta de esa mesa responde un motivo tipado con la
    cuenta, para que el dispositivo lo dirija ahí, en lugar del 400 genérico.
- **A1-D19 · El dispositivo resuelve antes de enviar**: «Enviar a cocina» usa la misma clave en `confirmDraft` y en su
  paso a la cola; antes de mandar un pedido de la cola, el trabajador resuelve las operaciones que quedaron sin respuesta
  (abrir la cuenta o guardar el borrador, repitiéndolas con su clave) para dirigirlo a la cuenta real; con conexión no se
  arma el pedido mientras haya un guardado sin resolver; «Descartar» también quita de Go los renglones propios (sin
  conexión, el descarte queda pendiente y se dice); las transiciones de la cola se escriben con condición sobre el estado
  vigente (nunca reescribiendo la fila completa) y el borrado espera a que se cancele el trabajo; la captura que nace de
  «Corregir» conserva la base que vio; una cuenta combinada ofrece mandar el pedido a la cuenta en que se combinó; un
  pedido que quedó «Enviando» se recupera al arrancar; los pedidos de un mesero desactivado se ven como tales; la hora de
  captura usa la diferencia con la hora de Go; el borrado vuelve a programar el envío periódico.
- Las correcciones del servidor las hace el agente de la Fase E del servidor y, después, las del dispositivo el de la Fase
  E del dispositivo; la Fase F sigue en paralelo y no corrige estos hallazgos.

## Correcciones de la revisión R6 · Servidor

Un agente, en su propio árbol de trabajo (`etapa-a1-es-fix`, desde `etapa-a1-android` en `8c6d2a7`), en paralelo con la
Fase F (otro agente en `android/` y `scripts/`). 17/11/2026. Encargo: **A1-D18** («el servidor nunca repite lo que ya
entró»). **Sin migración** (la columna `restaurant_items.draft_item_id` de la sección E ya basta), sin cambio de versión
(0.12.0, esquema 25) y sin cambios en el escritorio. Pruebas en copias aisladas (`ISO_TAG=E`): clúster temporal 55499
con una base `adel_pos_test_e_*` por paquete; sin Go temporal, Playwright ni emuladores. **No se tocó la instalación
real** ni `android/`, `scripts/`, `CLAUDE.md` ni `AGENTS.md`; `docs/etapas/*.md` con el mismo md5 antes y después.

Commits en `etapa-a1-es-fix`:

- `74edb74` — servidor: A1-D18 en `operations/offline_orders.go` y `operations/handler.go`, con los mensajes nuevos en
  `TestStageA1WaiterMode`.
- `3019c5b` — prueba `TestStageA1OfflineNeverRepeats` (`operations`).
- `268bc89` — contrato v1 (`openapi.json`), `TestContractV1` (`contractOfflineNeverRepeats`), `docs/API_V1.md`,
  `API_CONTRACTS.md` y `docs/RESTAURANT_OPERATIONS.md`.

Los tres compilan y pasan `go vet -tags=integration` por sí solos (`git archive` de cada uno). `TestContractV1` queda
al día en el tercero: en `74edb74` su paso del pedido sin conexión todavía esperaba el comportamiento anterior.

### Qué cambió

Todo en `POST restaurant/offline-orders` (`submitOfflineOrder`), compatible y en el contrato v1.

**1. La misma clave que un «Enviar a cocina» ya aplicado** — `operations/handler.go:328` (`replay`) y
`operations/offline_orders.go:922` (`confirmedAccountOf`) y `:938` (`offlineAlreadyConfirmed`):

- Cómo se guardan las claves: `app.restaurant_operations` (id = `operation_id`, `actor_id`, `device_id`, `route` =
  «método ruta», huella y respuesta) es **común** a `operations`, `kitchen` y `payments`; `replay` corre dentro del
  candado exclusivo 7820148305, después de comprobar los permisos de la ruta.
- Regla nueva, solo si **todo** se cumple: la ruta actual es `POST restaurant/offline-orders`; la guardada es
  exactamente `POST restaurant/accounts/{uuid}/confirm`; el mismo usuario **y** el mismo dispositivo (con sesión o con
  la credencial de envío); `destination.account_id` es esa cuenta; y existe el pedido de la confirmación
  (`restaurant_orders.operation_id` = la clave, en esa cuenta). Entonces **409 `OFFLINE_ORDER_ALREADY_ACCEPTED`** con
  `details.origin: "confirm_draft"` y `details.result` en la forma de `OfflineOrderResult` (el pedido de la
  confirmación, la cuenta vigente, `already_sent_item_ids` con las partidas del pedido que estaban en él), **sin
  escribir nada** (se revierte). Lee la cuenta con `authorizeAccount` (para llevar exige `takeaway.read`).
- Sin agujero: otra cuenta, una mesa, otro usuario, otro dispositivo u otra ruta guardada (guardar el borrador, abrir la
  cuenta, cocina, cobros) siguen en el `IDEMPOTENCY_CONFLICT` de siempre, sin datos. El 409 de una clave que ya entró
  como pedido sin conexión agrega `details.origin: "offline_order"` (aditivo).

**2. Renglones del borrador con el id de una partida** — `diagnoseDraftOverlap` (`offline_orders.go:562`), que sustituye
a `diagnoseReplacedDraft` de la Fase E:

- Candidatos: primero los de `replaces_draft_items` (como los vio el dispositivo), después cada partida del pedido que
  no nombra (el mismo id es el mismo renglón: lo genera el dispositivo).
- En el borrador: se **quita** si está igual (producto, presentación, cantidad y personalización) **o si el último
  guardado del borrador fue del mismo usuario y dispositivo** (`restaurant_draft_items.author_id`/`device_id`, que cada
  guardado reescribe en todas las filas: entonces lo que hay es suyo). Si lo dejó distinto otra terminal,
  `draft_item_changed` (aceptable; al confirmar, el renglón ajeno se queda, como en la Fase E).
- Ya enviado: ver 3. Nombrado, fuera del borrador y nunca enviado: `draft_item_removed`. Una partida nueva no es cambio.

**3. Confirmar `draft_item_sent` es «enviar lo demás»** — `sentDraftItem` (`:631`), `pendingOfflineItems` (`:418`),
`diagnoseOffline` y `acceptOffline` (`:814`):

- Una partida del pedido (o un renglón de `replaces_draft_items`) cuyo id ya está en una partida confirmada
  (`restaurant_items.draft_item_id`, en cualquier cuenta: así también se reconoce tras transferir o combinar) es
  `draft_item_sent`, aceptable. Mensaje nuevo: «Papas: ya se envió a cocina desde otra terminal y no se vuelve a
  enviar. Si confirmas, se envía lo demás del pedido.» (o «ya se había enviado a cocina desde este dispositivo…»).
- Esas partidas **no se cotizan** (ni sus cambios de precio) **ni se confirman**; la respuesta 200 las lista en
  **`already_sent_item_ids`** (propiedad nueva de `OfflineOrderResult`).
- **Si no queda nada**: 200 `status: "accepted"` sin pedido nuevo; `order_id` = el último pedido en que entraron; sin
  `touch` (la revisión de la cuenta no sube), sin evento `order.confirmed`; la clave se guarda con esa respuesta (el
  reintento devuelve lo mismo). Auditoría `restaurant.offline_order.accepted` con `new_order: false` y
  `already_sent_item_ids`.

**4. Pedido a una mesa con las partidas en una cuenta abierta de esa mesa** — `tableAccountsWithItems` (`:498`) en
`diagnoseOfflineTable`:

- Si alguna partida está en el borrador de una cuenta abierta de la mesa, o ya se envió en una (por la cuenta original
  del pedido o por sus unidades), el cambio bloqueante **`items_in_account`** (`acceptable: false`, `account_id` = la
  cuenta, `now` = «folio N», mensaje «Lo que capturaste ya está en la cuenta «Mesa 3» (folio 12) de la mesa «Mesa 3».
  Envía el pedido a esa cuenta.»), uno por cuenta, en lugar de `table_has_account` y del 400 genérico «la partida
  pertenece a otra cuenta». `OfflineChange` agrega **`account_id`** (opcional, `omitempty`, solo en este cambio; está
  en la estructura para que el eco de la confirmación siga pasando la decodificación estricta).
- El dispositivo reenvía con **la misma clave** a `destination.account_id`; ahí aplican 2 y 3.

**Contrato y documentación**: `openapi.json` (`OfflineChange.kind` + `items_in_account` y `account_id`;
`OfflineOrderResult.already_sent_item_ids` y la descripción de `order_id`; `OfflineOrderAlreadyAccepted.details.origin`;
descripciones de `submitOfflineOrder`, su 409 y `replaces_draft_items`), `docs/API_V1.md` (sección nueva «El servidor
nunca repite lo que ya entró», con el JSON exacto del 409 de la clave de una confirmación), `API_CONTRACTS.md` («Etapa
A1 · el servidor nunca repite lo que ya entró») y `docs/RESTAURANT_OPERATIONS.md`.

### Decisiones (con su porqué)

1. **«Último guardado propio» = se quita** (también para los renglones de `replaces_draft_items`). Es lo que corrige C1
   aunque el mesero haya cambiado el renglón sin conexión después del guardado incierto: si el último que guardó el
   borrador fue el mismo mesero desde el mismo dispositivo, el contenido es suyo y el pedido lo sustituye. Con otro
   mesero en la misma tableta (autor distinto) o desde otra terminal, sigue siendo `draft_item_changed`: nunca se borra
   en silencio lo de otro.
2. **Lo ya enviado se reconoce en cualquier cuenta**: el id es del renglón que generó el dispositivo (UUID aleatorio);
   una transferencia o una combinación no debe convertir un envío repetido en uno nuevo. Con la confirmación, la
   cantidad de esa partida no se vuelve a enviar aunque el mesero la hubiera subido sin conexión: lo dice el mensaje y
   puede capturar otra partida.
3. **Todo ya enviado = 200 sin pedido nuevo** (no un 409): la cola necesita un «Recibido» y A1-D18 pide «responde como
   aceptado». `order_id` sigue siendo un UUID (el pedido en que entraron) para no romper el esquema.
4. **`draft_item_sent` sigue pidiendo confirmación** (el encargo dice «confirmar un `draft_item_sent` significa…»);
   ahora confirmarlo nunca duplica.
5. **`items_in_account` es bloqueante** (no se confirma): el pedido debe ir a otra cuenta; con su `account_id` el
   dispositivo lo hace sin preguntar.

### Forma final para el agente de Android

- **Misma clave que `confirmDraft` (A1-D19)**, mismo usuario y dispositivo, `destination.account_id` = esa cuenta:
  `409 {"error": {"code": "OFFLINE_ORDER_ALREADY_ACCEPTED", "message": "Este pedido ya había entrado al enviarlo a
cocina y no se volvió a registrar.", "details": {"status": "already_accepted", "origin": "confirm_draft", "result":
{"status": "accepted", "operation_id", "order_id" (el de la confirmación), "account_id", "catalog_version",
"captured_catalog_version", "captured_at" (la del pedido), "received_at" (cuando entró la confirmación),
"accepted_changes": [], "removed_draft_item_ids": [], "already_sent_item_ids": [ids del pedido que estaban en la
confirmación], "bill"}}}}` → «Recibido por el restaurante». Otra cuenta, una mesa, otro usuario/dispositivo u otra
  operación: `409 IDEMPOTENCY_CONFLICT` (sin `details`). El 409 por otro contenido de un pedido ya aceptado trae
  `details.origin: "offline_order"`.
- **Renglones guardados sin respuesta**: no hace falta nombrarlos en `replaces_draft_items` si la partida lleva el mismo
  id; vuelven en `removed_draft_item_ids`. Si otra terminal los cambió: `draft_item_changed` (aceptable).
- **Ya enviado**: cambio `{"kind": "draft_item_sent", "item_id": <id>, "product_id", "product_name", "before": null,
"now": null, "acceptable": true, "message": "<producto>: ya se envió a cocina desde otra terminal y no se vuelve a
enviar. Si confirmas, se envía lo demás del pedido."}` (o «…ya se había enviado a cocina desde este dispositivo…»). Al
  confirmar (misma clave), 200 con `"already_sent_item_ids": [...]`; si son todas las partidas, no hubo pedido nuevo y
  `order_id` es el pedido en que ya habían entrado.
- **Mesa con las partidas en una cuenta suya**: `409 ORDER_REQUIRES_CONFIRMATION`, `requires_changes: true`, cambio
  `{"kind": "items_in_account", "item_id": null, "product_id": null, "product_name": "", "before": null, "now": "folio
12", "message": "Lo que capturaste ya está en la cuenta «…» (folio 12) de la mesa «…». Envía el pedido a esa
cuenta.", "acceptable": false, "account_id": "<cuenta>"}` → reenviar **con la misma `operation_id`** a
  `destination: {"account_id": "<cuenta>"}`.

### Pruebas ejecutadas

| Qué                                                                                     | Dónde                                                   | Resultado                                                                                                                                                                                                                                           |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TestStageA1OfflineNeverRepeats` (5 subpruebas) y `TestStageA1WaiterMode` (6), con `-v` | `internal/operations`, base `adel_pos_test_e_*`         | **PASS** las 11 (28 s)                                                                                                                                                                                                                              |
| `TestR6Review` original del revisor contra este código                                  | copia desechable en `$SP/tmp-E`                         | **FALLA donde antes reproducía** (ya no hay duplicado); copia borrada                                                                                                                                                                               |
| `go-int ./internal/apicontract` (`TestContractV1` con `contractOfflineNeverRepeats`)    | base `adel_pos_test_e_*`                                | **ok** (26 s), dos corridas                                                                                                                                                                                                                         |
| `go-int ./internal/operations` completo                                                 | idem                                                    | **ok** (386 s)                                                                                                                                                                                                                                      |
| `go-unit` (vet normal e integración, `test -race`, `GOOS=windows` build y vet)          | `iso.sh go-unit`                                        | 1.ª corrida: falla `TestCatalogImageCountsScansAfterStrayByteAfterSOI` (`catalog`, no tocado) por tiempo (573 ms > 500 ms) con la Mac cargada (promedio de carga ≈ 20–32 por la Fase F); esa prueba sola ×3: ok; **2.ª corrida completa: aprobado** |
| `format`, `fmt-check`, `sync-check`                                                     | árbol `Esfix`                                           | Correctos                                                                                                                                                                                                                                           |
| md5 de `docs/etapas/*.md`                                                               | antes y después de formatear, y contra `$SP/etapas.md5` | Idénticos                                                                                                                                                                                                                                           |
| `go-int` de `administration` y `database`; Vitest, Playwright                           | —                                                       | No se corrieron: no se tocaron (sin migración ni escritorio)                                                                                                                                                                                        |

`TestStageA1OfflineNeverRepeats` (`backend/internal/operations/stagea1_r6_test.go`, base propia) lleva las dos
reproducciones del revisor (`TestR6Review` A1 y A3, que ya no se copian: se convirtieron en comprobaciones de lo
correcto) y agrega cada punto:

- **R6 A1**: apertura con el renglón que entró sin respuesta y pedido sin `replaces_draft_items` → 200, el renglón se
  quita, entra una vez y el «Enviar a cocina» siguiente de otra terminal no tiene nada (400 «No hay productos
  pendientes»); el mismo mesero lo cambió sin conexión (2 → 3) → se quita y entran 3; otra terminal lo cambió →
  `draft_item_changed`, confirmado, el suyo se queda.
- **R6 A3**: `confirmDraft` aplicado y pedido con otra clave → `draft_item_sent` con el mensaje nuevo; confirmado, entra
  solo la partida nueva (`already_sent_item_ids`, una sola partida con ese id); todo ya enviado (sin nombrarlo) → 200
  sin pedido nuevo, `order_id` = el de la confirmación, misma revisión; el reintento devuelve lo mismo.
- **Misma clave**: 409 `OFFLINE_ORDER_ALREADY_ACCEPTED` `confirm_draft` con el pedido, la cuenta y la partida, sin
  pedidos nuevos; la misma clave hacia otra cuenta, a una mesa, desde otro usuario y la de un guardado del borrador →
  `IDEMPOTENCY_CONFLICT` sin el id del pedido.
- **Mesa**: `items_in_account` con la cuenta y su folio, no confirmable; redirigido con la misma clave entra una vez;
  ya enviadas también se reconocen.
- **Credencial de envío**: misma clave de una confirmación de la tableta → ya aceptado; otra clave → `draft_item_sent`
  sin repetir; renglón guardado desde la tableta y no nombrado → se quita.

Además, el archivo original del revisor (`$SP/r6/tree/.../r6_review_test.go`), corrido contra este código en una copia
desechable, **falla en sus dos subpruebas justo donde antes encontraba el defecto** («se esperaba el renglón todavía en
el borrador: []» y «partidas confirmadas con el id del renglón: 1»): ya no se reproduce.

### No verificado

- La aplicación Android contra estos cambios (A1-D19, agente del dispositivo).
- La liberación de cocina de un pedido sin partidas nuevas: no hay (no se crea pedido ni evento).
- `go-int` de `administration` y `database`: no se tocaron (sin migración); quedan para la Fase G.

### Para el coordinador

- Sin cambios de reglas de negocio. Dos lecturas que conviene confirmar: el «último guardado propio» también se quita
  cuando el renglón difiere (decisión 1) y lo ya enviado se reconoce en cualquier cuenta (decisión 2).
- Quedan anotados (no son de A1-D18): una partida con el id de un renglón del borrador de **otra** cuenta (no de la
  mesa) sigue siendo el 400 genérico; y un renglón ya enviado que alguien vuelve a guardar en el borrador con el mismo
  id se trata como renglón del borrador (lo evita el dispositivo con A1-D19).
- Para `CLAUDE.md` (no lo edité): §6, prueba nueva `TestStageA1OfflineNeverRepeats` (`operations`) y
  `contractOfflineNeverRepeats` en `TestContractV1`; §15, A1-D18 implementada en `operations/offline_orders.go`.

**Al cerrar las correcciones del servidor de R6** (rama `etapa-a1-es-fix` fusionada en `etapa-a1-android`): se aceptan las dos decisiones del agente: el último guardado propio (mismo mesero y mismo dispositivo) se quita aunque difiera del pedido, porque la versión sin conexión es la más reciente de ese mesero; y lo ya enviado se reconoce en cualquier cuenta, sin mandar la diferencia de una cantidad subida sin conexión (el mensaje lo dice). La falla de tiempo de `TestCatalogImageCountsScansAfterStrayByteAfterSOI` (tope de 500 ms en `catalog`, con la Mac muy cargada) no se repitió sola ni en la segunda corrida; la regresión de la Fase G se corre sin otras cargas.
