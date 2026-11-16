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
