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

Tomadas según la regla de autonomía de `CLAUDE.md` §0: ninguna cambia la especificación; todas aplican lo que pide.

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
