# Verificación · Etapa A1 (Android)

Verificación por fases del **16 y 17 de noviembre de 2026**, en la Mac (Apple Silicon, macOS 26.5.2), en ramas locales
por fase que el coordinador fusionó en `etapa-a1-android` (creada desde `main` = 0.11.0, esquema 24, con la 9H
fusionada en `24065f3`; especificación en `b286c4d`): Fase 0 `6b5109e` (solo documentación; decisiones del propietario
en `1763dfd`); fase B del servidor y el escritorio `ed603ae`, `56bd9ae`, `3aacd0c`, `b11c01f`, `c9c2c14` y `0b0633e`
(fusión en `bc30780`, avance `a139e2e`); fase A, proyecto `android/`, `0a04de4` a `f33aeb7` (fusión en `90e5426`,
avance `aeb9753`); fase E del servidor `cc3e4e5` a `d8dd384` (fusión en `da9c5e2`, avance `762400e`); fases C y B del
dispositivo `3854031` a `bc7ba59` (fusión en `fca254c`, avance `f7c20c7`); fase D `95d2dbb` a `706dc19` (fusión en
`616f812`); revisión independiente R3 y decisiones A1-D16 y A1-D17 (`3ca8b6e`); correcciones de R3 en el servidor
`29b63b3`, `df8d00f` y `1248802` (fusión en `aa315bb`, avance `b4e1cfe`) y en el dispositivo `00f16b0` a `ee85a0f`
(fusión en `c99c66a`, avance `6a1c496`); fase E del dispositivo, 19 commits de `c9731fe` a `735a97a` (fusión en
`f6e6b39`, avance `61515c1`); revisión independiente R6 y decisiones A1-D18 y A1-D19 (`8c6d2a7`); correcciones de R6 en
el servidor `74edb74`, `3019c5b` y `268bc89` (fusión en `1612e00`, avance `bd7beb4`); fase F `30e094d` a `28bd495`
(fusión en `407f61d`, avance `fd44940`); Fase G, guía del propietario `7e0684f` (fusión en `2f9f988`) y aviso al
emparejar con una principal anterior a la 0.12.0 `b49b817` (fusión en `c683c4a`, avance `cd75b5c`); `CLAUDE.md` y
`AGENTS.md` (`819c752`, `3724dfd` y `2a5c1ca`); correcciones de R6 en el dispositivo `990aada` a `75542f2` (fusión en
`bbe84a2`); avance en `e2b8486`; y, después de la regresión final, las pruebas al día en **`238c6f6`** (solo tres
archivos de prueba).

Servidor **0.12.0** (se fija en la Fase B; `min_client_versions`: `desktop` 0.10.0 y `android` 0.1.0;
`min_client_version` y `min_server_version` siguen en 0.10.0), API v1, migración `0025_android_waiters.sql` (esquema
**25**, una sola para la etapa, con la sección **B** —PIN de los meseros, credencial de envío, bloqueo por inactividad
por dispositivo y permiso `waiter_devices.use`, con el bloqueo largo de A1-D16— y la sección **E** —renglones del
borrador que sustituye un pedido sin conexión—). Aplicación Android **0.1.0** (`versionCode` 1, `minSdk` 26,
`targetSdk` 36, `compileSdk` 37), base local `AppDatabase` versión 5 y `catalog.db` versión 1. Especificación:
[etapas/ETAPA_A1_ANDROID.md](etapas/ETAPA_A1_ANDROID.md). Avance con el detalle de cada fase:
[ETAPA_A1_AVANCE.md](ETAPA_A1_AVANCE.md). Guía del propietario: [ANDROID_INSTALAR.md](ANDROID_INSTALAR.md); reglas y
pruebas de la aplicación: [android/CLAUDE.md](../android/CLAUDE.md). Evidencia anterior: [etapa 9H](VERIFICATION_STAGE9H.md)
y las anteriores desde [VERIFICATION.md](VERIFICATION.md). Este documento registra **solo lo que se ejecutó**.

**Alcance de esta evidencia.** Las cifras de las fases son las que cada agente registró sobre su propia copia, según el
avance. Lo que valida el código completo de la etapa es la «Regresión final». **Commit final: `238c6f6`.** La regresión
completa corrió sobre una copia exacta de `e2b8486`; `238c6f6` solo cambia **tres archivos de prueba** respecto a él
(`git diff --stat e2b8486 238c6f6`: `desktop/e2e/workflow-stage9c-navigation.spec.ts`, `FieldOnGoTest.kt` y
`WaiterModeOnGoTest.kt`; ningún archivo de la aplicación, del servidor ni del escritorio), y lo que esos archivos
afectan se repitió sobre una copia exacta de `238c6f6`. En `e2b8486` hubo dos fallas, las dos **de las pruebas**
(`workflow-stage9c-navigation` esperaba el menú del usuario sin «Mi PIN», que agregó la Fase B, y el paso `f6` de
`FieldOnGoTest` no alcanzaba el botón «Enviar a cocina» fuera de la vista en los teléfonos); con las pruebas corregidas
en `238c6f6` **todo aprobó** (detalle en «Regresión final»).

**Las pruebas no tocaron la instalación real**: ni el servicio `local.adel.pos.server`, ni `/Library/Application
Support/AD-EL POS`, ni `/Applications/AD-EL POS.app`, ni `.local` del repositorio, ni el Keychain, ni PostgreSQL en 55432
(ni los procesos `postgres` del `host` del servicio), ni Go en 8181 o en la escucha de red 8443, **salvo el incidente de
la Fase F** (abajo, «Incidente con el servicio real»). Ninguna prueba usó `sudo`, `installer` ni `launchctl` (salvo el
`launchctl` simulado de las pruebas de los scripts del instalador), ni las órdenes `dev.sh` de la instalación (`db-*`,
`server*`, `migrate`, `backup`, `test-integration`, `test-browser`). Las pruebas de la aplicación usan el ejecutor
`AdelTestRunner`, que busca por NSD el tipo de prueba `_adelpostest._tcp`; nunca el anuncio `_adelpos._tcp` del
servicio real.

**Datos.** Todas las pruebas usan **datos sintéticos** (restaurantes, meseros, catálogos y mesas creados por cada prueba
en un Go temporal con base desechable). La etapa no usó el respaldo del sistema anterior ni datos del restaurante.

## Entorno aislado

Un árbol de trabajo (`git worktree`) por fase en el directorio temporal de la sesión, con `.tools`, `node_modules` y las
cachés enlazadas; un clúster PostgreSQL 18.6 temporal en el puerto 55499 (solo loopback) compartido por los agentes, con
una base `adel_pos_test_*` desechable por paquete, por motor de Playwright y por Go temporal; `GOPROXY=off` y cachés de
Go, Rust y npm propias por fase (`ISO_TAG`). Para Android, un **Go temporal en modo de pruebas** (`test_mode: true`) con
su escucha HTTP local en `127.0.0.1` y su escucha HTTPS de red también en loopback (el emulador la alcanza como
`10.0.2.2`), con base nueva y **uno nuevo por suite y emulador**; Gradle 9.9.1 con el JDK de Android Studio (JBR 25) y
el SDK del propietario; y los emuladores del propietario abiertos **sin ventana y de solo lectura** (`-read-only`: los
AVD no se modifican), siempre elegidos con `ANDROID_SERIAL`:

| Emulador               | Android      | Imagen                                 | Para qué                                                        | Corte de red                               |
| ---------------------- | ------------ | -------------------------------------- | --------------------------------------------------------------- | ------------------------------------------ |
| `ADEL_A1_Tablet_API26` | 8.0 (API 26) | `google_apis` arm64, 2 GiB, 1280 × 800 | Tableta económica: el mínimo, TLS 1.2, NSD antiguo, rendimiento | Desde la Mac: `adb root` + `svc wifi/data` |
| `ADEL_A1_Phone_API33`  | 13 (API 33)  | `google_apis` arm64, 2 GiB, pixel_6a   | Teléfono de hace 2–3 años                                       | `cmd connectivity airplane-mode`           |
| `Pixel_8`              | 17 (API 37)  | `google_apis_playstore` arm64, 2 GiB   | Teléfono actual                                                 | `cmd connectivity airplane-mode`           |
| `Pixel_Tablet`         | 17 (API 37)  | `google_apis_playstore` arm64, 2 GiB   | Tableta actual, dos paneles y rotación                          | `cmd connectivity airplane-mode`           |

**Pruebas automatizadas en emuladores, no físicas.** Ninguna prueba corrió en un teléfono o una tableta reales, en la
wifi de un restaurante (con o sin internet) ni contra el servicio real (salvo el incidente de la Fase F); los emuladores de Android 17 ven el anuncio mDNS
de la Mac, los de Android 8 y 13 no. Vitest y Playwright sustituyen el puente nativo del escritorio por la simulación de
`e2e/native-driver.ts`, como en las etapas anteriores.

## Fase 0 · verificación

Un solo agente, sin cambios en el repositorio (proyecto de prueba fuera de él). El entorno compila y prueba en la JVM y
en los emuladores de Android 8, 13 y 17 contra el Go temporal (9/9 en API 33 y 37; 8/9 en API 26, sin corte de red por
`shell`). Hallazgos que cambiaron el plan: la app 0.1.0 recibía 426 (una sola versión mínima para todos los clientes:
A1-D4), el rol Mesero nacía sin permisos (A1-D2), sesiones de 12 h frente a la captura sin conexión sin límite (A1-D1,
credencial de envío), los cambios de preparación no llegaban al mesero (A1-D11) y el permiso de red local de Android 17
con `targetSdk` 37 (A1-D7: `targetSdk` 36). Decisiones A1-D1 a A1-D11 en el avance; A1-D1 y A1-D2 las confirmó el
propietario.

## Resultados ejecutados por fase

«JVM» = `:core:domain:test testDebugUnitTest lintDebug` (JVM, Robolectric SDK 36 y Android Lint, contadas en los XML);
«instrumentadas» = `connectedDebugAndroidTest` con las aplicaciones de prueba desinstaladas antes y los XML contados por
módulo (las de punta a punta salen omitidas sin los argumentos del guion); «de punta a punta» = `scripts/android-e2e.sh`
contra un Go temporal nuevo. En el servidor, integración con `-tags=integration` y `-race`, tiempos de `go test`. En
cada fase se corrieron además `fmt-check` (Prettier, `gofmt`, `cargo fmt` y la comprobación de datos reales), `ktfmtCheck`
en las de Android y `sync-check`, y `docs/etapas` quedó intacto por md5.

| Fase o parte (commit)                                       | Servidor y escritorio                                                                                                                                                                                                                                                    | Android (JVM e instrumentadas)                                                                                                                                                         | Android contra el Go temporal                                                                                                                                                        |
| ----------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| B, servidor y escritorio (`0b0633e`)                        | `go-unit` aprobado (dos veces); `database`, `administration` 178 s (`TestStageA1WaiterPIN`), `operations` 442 s y `apicontract`; Vitest 96 archivos, 914 pruebas; `cargo test` 107 aprobadas, 3 ignoradas; Playwright `workflow-stagea1-pin` 1/1 en Chromium y en WebKit | —                                                                                                                                                                                      | —                                                                                                                                                                                    |
| A, proyecto `android/` (`f33aeb7`)                          | Sin cambios                                                                                                                                                                                                                                                              | JVM 41/41; instrumentadas 19/19 + 2 omitidas en `Pixel_8`, `Pixel_Tablet` y `ADEL_A1_Phone_API33`, 18/18 + 3 en `ADEL_A1_Tablet_API26`; APK de depuración 12.7 MiB, `release` 1.43 MiB | `ServerApiOnDeviceTest` 2/2 en API 26 y 33 (TLS 1.2 con la huella fijada)                                                                                                            |
| E, servidor (`d8dd384`)                                     | `go-unit` aprobado; `operations` 359 s (`TestStageA1WaiterMode`), `apicontract` 14 s, `database` 15 s, `administration` 174 s                                                                                                                                            | —                                                                                                                                                                                      | —                                                                                                                                                                                    |
| C y B, dispositivo (`bc7ba59`)                              | Sin cambios                                                                                                                                                                                                                                                              | JVM 49 + 39 + 15, Lint sin avisos; instrumentadas 33/33 + 9 omitidas (`Pixel_8`, `Pixel_Tablet`), 32/32 + 10 (`ADEL_A1_Tablet_API26`)                                                  | `AccessOnGoTest` 7/7 en `Pixel_8`, `Pixel_Tablet` y `ADEL_A1_Tablet_API26`                                                                                                           |
| D, catálogo (`706dc19`)                                     | Sin cambios                                                                                                                                                                                                                                                              | JVM 154/154; instrumentadas 52 (0 fallas) en `Pixel_Tablet`, `Pixel_8` y `ADEL_A1_Tablet_API26`                                                                                        | `CatalogOnGoTest` 3/3 en los tres (303 productos: 219 ms en API 26, ≈ 0.8 s en API 37)                                                                                               |
| Correcciones de R3, servidor y escritorio (`1248802`)       | `go-unit`; `administration` 217 s (`TestStageA1WaiterPIN`, 13 subpruebas), `operations` 359 s, `apicontract` 24 s, `database` 11 s; Vitest 96 archivos, 916 pruebas; Playwright `workflow-stagea1` 1/1 en los dos motores y 9/9 de las vecinas en Chromium               | —                                                                                                                                                                                      | —                                                                                                                                                                                    |
| Correcciones de R3, dispositivo (`ee85a0f`)                 | Sin cambios                                                                                                                                                                                                                                                              | JVM 176, 0 fallas; instrumentadas 54, 0 fallas, en `Pixel_Tablet` (12 omitidas) y `ADEL_A1_Tablet_API26` (13)                                                                          | `AccessOnGoTest` 7/7 en los dos                                                                                                                                                      |
| E, dispositivo (`735a97a`)                                  | Sin cambios                                                                                                                                                                                                                                                              | JVM 219, 0 fallas; instrumentadas 71, 0 fallas, 22 omitidas, en `Pixel_8` y `Pixel_Tablet`                                                                                             | `--suite waiter` 10/10 en los cuatro emuladores                                                                                                                                      |
| Correcciones de R6, servidor (`268bc89`)                    | `TestStageA1OfflineNeverRepeats` y `TestStageA1WaiterMode` 11/11; `operations` 386 s; `apicontract` 26 s; `go-unit` aprobado en la segunda corrida (la primera falló por tiempo en `catalog`, ver la nota)                                                               | —                                                                                                                                                                                      | —                                                                                                                                                                                    |
| F, de punta a punta (`28bd495`)                             | Sin cambios                                                                                                                                                                                                                                                              | JVM 220, 0 fallas; instrumentadas 84, 0 fallas, en los cuatro (35 omitidas; 37 en Android 8)                                                                                           | `--suite all` en los cuatro: access 7/7, catalog 3/3, waiter 10/10, field 13/13 (con las repeticiones de la nota); APK `release` aprobado en API 26 y 33; **incidente** en `Pixel_8` |
| G, aviso con una principal anterior a la 0.12.0 (`b49b817`) | Sin cambios                                                                                                                                                                                                                                                              | JVM 224, 0 fallas (4 pruebas nuevas)                                                                                                                                                   | —                                                                                                                                                                                    |
| Correcciones de R6, dispositivo (`75542f2`)                 | Sin cambios                                                                                                                                                                                                                                                              | JVM 256, 0 fallas; instrumentadas 94, 0 fallas, en `Pixel_Tablet` (44 omitidas) y `ADEL_A1_Tablet_API26` (46)                                                                          | `--suite waiter` 19/19 y `--suite field` 13/13 en `Pixel_Tablet` y `ADEL_A1_Tablet_API26`; después del último ajuste, en API 26, waiter 4/4 y field 2/2 de los pasos afectados       |
| Punta de la etapa (`e2b8486`, y `238c6f6` para lo afectado) | «Regresión final» (abajo)                                                                                                                                                                                                                                                | «Regresión final»                                                                                                                                                                      | «Regresión final»                                                                                                                                                                    |

**Notas de las fases.**

- **`TestCatalogImageCountsScansAfterStrayByteAfterSOI` (correcciones de R6 en el servidor)**: falló una vez por tiempo
  (573 ms frente al tope de 500 ms) con la Mac muy cargada por los emuladores de la Fase F; sola tres veces y en la
  segunda corrida completa, aprobada. En la regresión final, ver abajo.
- **Fase F, entorno**: con dos emuladores y Gradle a la vez, el lanzador de `Pixel_Tablet` mostró «Pixel Launcher isn't
  responding» y se quedó con el foco (fallas de `w2`, `f4`, `f9`, `f10` y una instrumentada en el carril con otro
  emulador); las suites se repitieron con el emulador recién encendido y solo, aprobadas. Dos pasos sin ningún resultado
  a la misma hora en dos emuladores (probable interrupción de `adb`) se repitieron aparte, aprobados.
- **Fase F, defecto corregido** (`13af08c`): «Cambiar» una partida después de que Android cerró la aplicación volvía con
  la configuración original del producto.
- **Correcciones de R6 en el dispositivo, entorno**: con la aplicación de la suite field instalada, AGP terminó «con
  éxito» sin correr `app` (`INSTALL_FAILED_ALREADY_EXISTS`); `DeviceLayoutTest` perdió el texto al girar una vez con el
  otro emulador cargado (sola, aprobada dos veces); y el emulador de Android 8 terminó de arrancar pero `adb shell` no
  respondió en más de una hora (se apagó, se reinició `adb` y se volvió a encender con tope en cada `adb shell`).

## Incidente con el servicio real (Fase F)

El 17/11/2026 a las 11:00–11:01, la prueba de humo del APK `release` en `Pixel_8` (Android 17, cuyo emulador ve el
anuncio mDNS de la Mac) abrió la aplicación sin `AdelTestRunner`; el guion de la prueba tocó por texto el primer botón
«Pedir código» de la pantalla, que era el del anuncio real («ADEL POS — Eliver’s MacBook Pro»), y la aplicación mandó
**una** petición de emparejamiento al servicio real, que respondió 426 (el servicio está en 0.11.0, cuya versión mínima
única es la del escritorio). En el código de la 0.11.0 la versión del cliente se revisa antes de llegar a cualquier ruta
salvo salud, versión y contrato, así que la petición no llegó al emparejamiento ni escribió en la base real; el
registro del servicio no se consultó (no se toca la instalación real). La aplicación se cerró y se desinstaló. El guion
se corrigió (solo toca el campo de la dirección y pide el código con la acción del teclado; se detiene y desinstala si
la lista muestra cualquier servidor). **Pendiente del propietario**: revisar en Administración › Sistema › Terminales de
la computadora principal que no quedó ninguna solicitud de emparejamiento.

## Revisiones independientes

Dos revisiones de solo lectura que pide la especificación (§0, «Eficiencia»), sobre copias exactas y con pruebas
propias que reproducen cada hallazgo; las correcciones las hicieron los agentes de la etapa en ramas nuevas y las
pruebas del revisor quedaron en el repositorio **al revés** (exigiendo lo correcto).

**R3 · PIN y sesión (§3)**, sobre `f7c20c7`. El servidor estaba bien; los defectos estaban en el dispositivo.

| Hallazgo                                                                                       | Corrección                                                                                                                                                                                                                         |
| ---------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A1 (alto) · una entrada sin conexión reutilizaba la sesión de Go que el mesero dejó en memoria | `enterOfflineLocked` borra esa sesión; la prueba mira la cabecera `Authorization` de una petición real (`AccessSafetyTest`) (`6904e30`)                                                                                            |
| A2 (alto) · el plazo del bloqueo por inactividad se reiniciaba al volver a crear la actividad  | Bloqueo del proceso (`IdleLock.supervise`, `AppContainer.startIdleLock`) que no reinicia el plazo y se recalcula al volver a primer plano con `elapsedRealtime` (`6904e30`)                                                        |
| M1, M2 · el límite sin conexión dependía del reloj de pared y no respetaba el bloqueo de Go    | Fallos acumulados sin reloj (Room v3); al quinto, o con un `PIN_LOCKED` de Go, la entrada sin conexión de ese mesero queda cerrada hasta que entre con conexión                                                                    |
| M3 · una sesión terminada sacaba al mesero con pedidos sin enviar                              | «Vuelve a escribir tu PIN» para el mismo mesero, sin perder captura ni pendientes                                                                                                                                                  |
| M4 · retirar o emparejar de nuevo no borraba lo de las demás fases                             | Registro único de borrado (`DeviceDataWipe`, `00f16b0`) al que se suman el catálogo y, desde la Fase E, la cola y WorkManager                                                                                                      |
| M5 · cabeceras del mesero activo, no del dueño de la operación                                 | `requestHeaders(userId)`                                                                                                                                                                                                           |
| M6 (para decidir) · 5 intentos cada 5 minutos sin escalamiento                                 | Decisiones del propietario **A1-D16** (tras 3 bloqueos en 24 h, bloqueado hasta que lo desbloquee un administrador) y **A1-D17** (la sesión con PIN solo tiene permisos de mesero), en el servidor (`29b63b3`) y en el dispositivo |
| B1, B2, B4, B6, B7, O4                                                                         | Credencial de envío rechazada fuera de su ruta; `CLIENT_KIND_MISMATCH`; toques de los diálogos (`f71c53f`); sin «Asignar PIN» en la fila propia; prueba real de las cabeceras; límite de verificaciones Argon2id a la vez          |

**R6 · cola sin conexión (§6 y §7)**, sobre `61515c1`. El camino principal estaba bien (misma clave, respuesta perdida sin
duplicar, Room tras reiniciar); los defectos, en los bordes entre el borrador de Go y la cola.

| Hallazgo                                                                                                           | Corrección                                                                                                                                                                                                                                               |
| ------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| C1 (crítico) · «Enviar a cocina» con un guardado del borrador sin respuesta dejaba esos renglones por enviar en Go | **A1-D18** en Go (`74edb74`: los renglones del borrador con el id de una partida se sustituyen) y **A1-D19** en el dispositivo (`990aada`: el guardado se resuelve antes de armar el pedido; sin conexión, el trabajador lo repite con su clave)         |
| A1 (alto) · una confirmación aplicada sin respuesta salía otra vez con otra clave                                  | La misma clave en `confirmDraft` y en la cola; Go responde `OFFLINE_ORDER_ALREADY_ACCEPTED` (`confirm_draft`) y lo ya enviado nunca se reenvía                                                                                                           |
| A2 (alto) · «Descartar» solo borraba en el dispositivo                                                             | Quita de Go los renglones propios que siguen igual y descarta la cuenta que nació con la captura; sin conexión queda en descarte pendiente y se dice                                                                                                     |
| M1 · borrar los datos durante un envío revivía el pedido                                                           | Escrituras condicionales al estado leído (nunca `Upsert`) y el borrado espera a que se cancele el trabajo                                                                                                                                                |
| M2 · «Corregir en la captura» sobrescribía el cambio de otra terminal                                              | La captura conserva la base vista y pide «Conservar mis cambios» o «Descartar mis cambios»                                                                                                                                                               |
| M3 · cuenta combinada sin salida útil y con riesgo de duplicado                                                    | «Enviar a la cuenta en que se combinó» / «Seguir en la cuenta en que se combinó»; con una cuenta combinada no se ofrece «Abrir una cuenta nueva» (regla del coordinador, `2456b63`)                                                                      |
| B2–B5, O1–O3                                                                                                       | «Enviando» sin dueño vuelve a «Por enviar»; pedidos de otros meseros con su motivo; hora de captura con la de Go (`ServerClock`); el borrado vuelve a programar el periódico; comentarios y `null` en lugar de cadena vacía. B1, B6 y O4 quedan anotados |

## Cobertura de la etapa A1

Correspondencia con las pruebas de §9 de la especificación. Rutas de Android desde `android/` (JVM en `src/test`,
instrumentadas en `src/androidTest`); del servidor, desde `backend/`.

| Prueba exigida (§9)                                                                      | Dónde y qué                                                                                                                                                                                                                                                                                                                                  |
| ---------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Unitarias: dominio, estados de la cola y correspondencia con la API                      | `core:domain` (`MoneyFormatTest` con la tabla compartida de Go y el escritorio, `WaiterRulesTest`, `OrderQueueRulesTest`, `R6RulesTest`, `CustomizationChoicesTest`, `TableNamesTest`…); `core:data` (`ServerApiTest`, `CustomizationJsonTest`, `OrderQueueTest`, `CaptureSyncTest`, `OfflineNeverRepeatsTest` con las formas exactas de Go) |
| Verificador del PIN                                                                      | `PinVerifierTest` (JVM) y `PinVerifierOnDeviceTest` (Keystore real, PBKDF2 de 120 000 iteraciones); `AccessRulesTest` y `AccessSafetyTest` (límite sin reloj, entrada sin conexión sin la sesión de Go, A1-D16)                                                                                                                              |
| Selección del servidor por `server_id` y huella                                          | `ServerAnnouncementsTest` (TXT, `server_id` + huella corta y cambio de IP), `AccessRepositoryTest` con la sonda HTTPS real contra el servidor simulado, `NsdServerBrowserTest` (anuncio propio dentro del emulador)                                                                                                                          |
| Room: migraciones, cola tras reiniciar el proceso y datos por mesero                     | `AppDatabaseMigrationTest` (1 → 2 → 3 → 4 → 5 con `MigrationTestHelper`), `CatalogDatabaseMigrationTest`, `AppDatabaseTest`; de punta a punta, cada paso es un proceso nuevo (`w3` → `w4`, `w7a` → `w7b`, `f4`)                                                                                                                              |
| Emparejamiento completo con aprobación del código                                        | `AccessOnGoTest` paso 1 (código equivocado rechazado, huella fijada = la del certificado); `FieldOnGoTest` `f1`                                                                                                                                                                                                                              |
| Certificado cambiado (rechazado)                                                         | `AccessOnGoTest` paso 6, con **otro** Go temporal de otra identidad en las mismas direcciones: «necesita emparejarse de nuevo» sin que llegue ninguna petición                                                                                                                                                                               |
| Entrada con PIN, PIN incorrecto y bloqueo                                                | `AccessOnGoTest` pasos 2–4 (bloqueo al quinto, que cierra también la entrada sin conexión; PIN cambiado con «Vuelve a escribir tu PIN») y `FieldOnGoTest` `f7a`–`f7c` (bloqueo largo de A1-D16, desbloqueo por la principal); en Go, `TestStageA1WaiterPIN`                                                                                  |
| Catálogo completo y por cambios, `reset` y producto agotado                              | `CatalogOnGoTest` `a`–`c` (≈ 300 productos, cambios por el canal y por descarga, agotado, 304 de la imagen, baja en `deleted` y `reset: true`); `CatalogSyncTest` y `CatalogImageCacheTest`                                                                                                                                                  |
| Captura en línea y envío a cocina                                                        | `WaiterModeOnGoTest` `w2` (creación diferida de la cuenta, giro sin perder nada, `confirmDraft`) y `FieldOnGoTest` `f2`–`f3` (con cocina: «En preparación» y «Listo» al momento)                                                                                                                                                             |
| Sin conexión, envío al volver, respuesta perdida sin duplicar, precio cambiado o agotado | `WaiterModeOnGoTest` `w3`–`w5` y `w7a`–`w13c` (red cortada por el guion; `w4` primera respuesta perdida, `w9` guardado sin respuesta, `w10` confirmación aplicada sin respuesta, `w12` descarte sin conexión, `w13` cuenta combinada); en Go, `TestStageA1WaiterMode` y `TestStageA1OfflineNeverRepeats`                                     |
| Mesa cobrada mientras tanto                                                              | `WaiterModeOnGoTest` `w6`; transferir y combinar desde la principal con la cuenta abierta, `FieldOnGoTest` `f6` (en los cuatro emuladores con las pruebas de `238c6f6`; en `e2b8486` la prueba no alcanzaba el botón en los teléfonos, ver «Fallas corregidas en la prueba»)                                                                 |
| Revocación del dispositivo                                                               | `AccessOnGoTest` paso 7 (Room vacío, «retirado»); `AccessScreensTest` (lo que se borra y los pendientes)                                                                                                                                                                                                                                     |
| Versión vieja (426)                                                                      | `AccessOnGoTest` paso 5 (`clientVersion` 0.0.9); emparejar con una principal anterior a la 0.12.0: `PairingRepositoryTest` (JVM)                                                                                                                                                                                                             |
| Interfaz: teléfono y tableta, rotación sin perder lo capturado                           | `DeviceLayoutTest` (rotación real con `UiAutomation.setRotation`), `AdaptivePanesTest`, `WaiterScreensTest`; `w2` gira durante la captura                                                                                                                                                                                                    |
| Tema oscuro                                                                              | `AdelThemeTest`, `WaiterScreensTest` (ambos temas) y `FieldOnGoTest` `f10` (`cmd uimode night yes`, capturas de las pantallas principales y zonas táctiles de 48 dp; `TouchTargetsTest`)                                                                                                                                                     |
| Indicador de conexión y de pendientes                                                    | `ConnectionIndicatorTest`, `ConnectionOnDeviceTest` (modo avión real, Android 11+), `w3` («1 pedido por enviar»)                                                                                                                                                                                                                             |
| Bloqueo por inactividad que no ocurre con pendientes                                     | `IdleLockWithPendingOnDeviceTest`, `AccessViewModelsTest` (tiempo virtual) y `FieldOnGoTest` `f9` (plazo de 30 s, Atrás en Mesas, con captura en curso no se bloquea)                                                                                                                                                                        |
| Dispositivo físico                                                                       | **No hay** (ver «No verificado»)                                                                                                                                                                                                                                                                                                             |
| Regresión completa del servidor                                                          | «Regresión final»                                                                                                                                                                                                                                                                                                                            |

## No verificado

No se afirma ninguna de estas:

- **Lo que no se repitió sobre `238c6f6`**: Go, Vitest, Rust, los scripts del instalador, la JVM de Android, el APK
  `release`, las suites `access` y `catalog`, `waiter` en las tabletas y `field` en `Pixel_Tablet` corrieron sobre
  `e2b8486`, y ninguno de sus archivos cambió. Las instrumentadas tampoco se repitieron: su APK de pruebas de `app`
  incluye `FieldOnGoTest` y `WaiterModeOnGoTest`, pero sin los argumentos del guion esas dos salen omitidas.
- **El lector con Docker real** (`TestLegacyPrepareWithDocker`): no se corrió en esta regresión porque Docker Desktop
  estaba apagado; el código del lector no cambió en la etapa (su última corrida aprobada es la de la regresión de la
  9H).
- **Dispositivo físico** (no se conectó ninguno; parada de la especificación): la búsqueda NSD real del servidor de
  punta a punta (emparejar eligiéndolo de la lista y el cambio de IP por `server_id` y huella; solo se vio leer un
  anuncio real, sin conectarse), la wifi del restaurante **sin internet** (WorkManager sin restricción de red y el
  indicador; en el emulador se simuló una red sin validar), Doze largo con la aplicación cerrada (el trabajo periódico
  de 15 minutos y la espera exponencial larga), el costo real de PBKDF2 y de la entrada con PIN, la memoria en una
  tableta de 2 GiB real y el flujo principal completo en un teléfono y una tableta reales. Los tiempos y la memoria de la
  Fase F son de emuladores sobre la Mac.
- **Instalación real** (pendiente del propietario): instalar el `.pkg` 0.12.0 con «Servidor del restaurante» marcada
  (contraseña de administrador), que el `host` respalde y aplique la 0025 sobre la base del servicio, y emparejar un
  dispositivo con la computadora principal real. Hasta entonces el servicio sigue en 0.11.0 y la aplicación 0.1.0 dice
  «Actualiza el servidor del restaurante».
- **Contra el servicio real nunca se probó nada a propósito** (incidente de la Fase F aparte): ni el emparejamiento, ni el
  escritorio 0.12.0 contra un servidor 0.11.0 real (simulado en Vitest), ni la aplicación contra un servidor 0.10/0.11
  real (en la JVM con las formas de Go).
- **La muerte real del proceso**: `am instrument` detiene la aplicación al empezar y al terminar cada paso; `f8` la
  simula recreando la actividad sin sus ViewModel. La personalización en curso de una partida nueva vuelve con la
  configuración original (limitación conocida F2).
- **El bloqueo largo con tres bloqueos reales** de 5 minutos: los dos primeros de la serie se escriben en la base
  temporal (`f7`); el tercero y todo lo que sigue sí es real contra Go. La época del catálogo se cambia con SQL en la
  base temporal (Go no tiene API para eso: una restauración real no se probó).
- **Lo que solo se probó en la JVM**: «Todavía se está guardando…» (necesita una respuesta de más de 26 s),
  `items_in_account` y la apertura de cuenta sin respuesta contra Go, el aviso de B4 con la hora de Go alterada, `503
PIN_LOGIN_BUSY` y `400 CLIENT_KIND_MISMATCH` en el dispositivo, Atrás en la raíz de Android 8–11 con un reposo profundo
  real (tiempo virtual) y el permiso `ACCESS_LOCAL_NETWORK` en uso (hoy no se pide con `targetSdk` 36).
- **Interfaz**: el mapa de Mesas del teléfono con 40 mesas y el teclado de «Buscar el servidor» en Android 8 después de
  sus correcciones (`50ddacf`) no se volvieron a fotografiar; foldables y multiventana; Robolectric en SDK distintos de
  36; las reglas de lint de lifecycle 2.11.0 (las omite el lint de AGP 9.5.1).
- **Firma**: el APK se firmó solo con llaves **desechables** del directorio temporal (borradas al terminar); la llave de
  desarrollo del APK de prueba para el propietario y el archivo `AD-EL-POS-Android-0.1.0.apk` no son parte de esta
  verificación.
- **Windows**: nada de la etapa se ejecutó en Windows (solo `GOOS=windows` para Go, también con `integration`).
- Pendientes de etapas anteriores sin cambio: papel y corte, cajón físico, pantalla táctil física, la ventana nativa y el
  diálogo nativo real de guardado; los de la 9G (red con una segunda computadora, reinicio de la Mac sin sesión) y los de
  la 9H (instalación real de la 0.11.0 e importación del sistema anterior, que decide el propietario).

## Regresión final

Una sola vez (§0, «Eficiencia», y §9 de la especificación), sobre una **copia exacta de `e2b8486`**
(`git archive e2b8486`, punta de `etapa-a1-android`; `git rev-parse` = `e2b8486d138991c2ec809b2afd65130a47a079a2`), sin
`.git`, con el arnés aislado (`ISO_TAG=G`): clúster temporal 55499; Playwright con Go temporal 8183 y vista previa 1421, y
un segundo Go limpio (`ISO_TAG=GC`, 8193) para las cuatro pruebas de «instalación limpia» de la 9H; para Android, Go
temporal `ISO_TAG=G` en 8199 (local) y 8459 (red), **uno nuevo por suite y emulador**; emuladores de solo lectura en las
consolas 5580 (`Pixel_Tablet`), 5582 (`Pixel_8`), 5584 (`ADEL_A1_Tablet_API26`) y 5586 (`ADEL_A1_Phone_API33`), **uno a
la vez** y siempre con `ANDROID_SERIAL`; cada `adb shell` y cada espera con un tope de tiempo (en esta corrida no hubo
ningún cuelgue). Un solo agente, sin otras cargas en la Mac (Android Studio y VS Code del propietario abiertos).
17/11/2026, de 14:27 a 17:08 (hora de Monterrey): servidor y escritorio de 14:27 a 15:50, Android de 15:50 a 17:08. **No
se cambió código.** Lo que afectan las pruebas corregidas se repitió después sobre `238c6f6` (abajo, «Repetición sobre
`238c6f6`»).

**Resultado en `e2b8486`**: dos fallas reproducibles (repetidas una vez cada una, con el mismo resultado), las dos **de
las pruebas**: la especificación de Playwright `workflow-stage9c-navigation` (en los dos motores) y el paso `f6` de
`FieldOnGoTest` en los dos **teléfonos** (en las dos tabletas aprobaba); todo lo demás aprobó. **Resultado con las
pruebas al día (`238c6f6`): todo aprobó** (Playwright 43 de 43 en los dos motores y `f6` en los cuatro emuladores). Las
tablas de abajo son de `e2b8486`; las fallas y su corrección, en «Fallas en `e2b8486`, corregidas en la prueba».

### Servidor y escritorio

| Suite                                                                                | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                        | Tiempo         |
| ------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------- |
| `fmt-check` (comprobación de datos reales, Prettier, `gofmt`, `cargo fmt`)           | Aprobado. La copia no tiene `.git` y la comprobación de datos reales lo dice y termina bien; por eso se corrió además `scripts/check-legacy-data.sh` sobre un índice de git **propio** hecho con `git read-tree e2b8486` (`GIT_INDEX_FILE` en el directorio temporal; el índice del repositorio no se tocó): «el índice de git no tiene respaldos ni paquetes de migración» (1 698 archivos)                                     | 16 s           |
| `sync-check`                                                                         | «Metadatos coherentes.»                                                                                                                                                                                                                                                                                                                                                                                                          | < 1 s          |
| `go-unit`                                                                            | Aprobado: `go vet` (normal e `integration`), `go test -race -count=1 ./...` (35 paquetes con pruebas, todos aprobados; 5 sin pruebas), `GOOS=windows` `go build` y `go vet` (normal e `integration`). `TestCatalogImageCountsScansAfterStrayByteAfterSOI` (la del tope de 500 ms de las correcciones de R6) aprobó a la primera                                                                                                  | 61 s           |
| `go vet -tags 'integration legacydocker'`, en macOS y con `GOOS=windows`             | Aprobado                                                                                                                                                                                                                                                                                                                                                                                                                         | 5 s            |
| Integración PostgreSQL, los **14 paquetes** de la tupla de `scripts/database.py`     | **14 de 14** (`-race`, una base por paquete, tiempos de `go test`): `database` 11,5 s, `administration` 210,7 s, `catalog` 19,8 s, `operations` 360,7 s, `payments` 2,4 s, `reports` 143,1 s, `reports/operational` 43,5 s, `designer` 22,3 s, `identity` 19,2 s, `apicontract` 23,5 s, `backupschedule` 7,0 s, `cmd/ad-el-pos-server` 62,9 s (con PostgreSQL 18 de Homebrew), `discovery` 5,2 s y `legacyimport` 11,9 s         | 977 s de reloj |
| `reports/operational` y la hora; `TestServiceHostOnMacOS`                            | `reports/operational` corrió a las 14:44, fuera de la franja de 00:00 a 05:00 de Monterrey. Para que ninguna saliera omitida sin decirlo, se corrieron otra vez con `-v`: `TestOperationalReports` a las 14:48, la prueba y sus 16 subpruebas aprobadas, ninguna omitida (42,3 s); `TestServiceHostOnMacOS` aprobada, no omitida (63,3 s, clústeres propios 55511–55513)                                                         | —              |
| Docker real del lector (`ADEL_TEST_DOCKER=1`, `TestLegacyPrepareWithDocker`)         | **No se corrió**: Docker Desktop estaba apagado (`docker info` sin servidor) y el encargo dice no encenderlo                                                                                                                                                                                                                                                                                                                     | —              |
| `vitest` (`tsc --noEmit` + Vitest)                                                   | Aprobado: 96 archivos, 916 pruebas                                                                                                                                                                                                                                                                                                                                                                                               | 26 s           |
| `build` (`npm run build`)                                                            | Aprobado (solo el aviso de tamaño de fragmento de siempre)                                                                                                                                                                                                                                                                                                                                                                       | 10 s           |
| `cargo test --locked --offline`                                                      | 107 aprobadas, 0 fallidas, 3 ignoradas                                                                                                                                                                                                                                                                                                                                                                                           | 33 s           |
| **Playwright Chromium**, todas las especificaciones en orden y compartiendo la base  | **Falla** (dos corridas iguales): 13 aprobadas, **26 fallidas** y 4 omitidas de 43. Falla `workflow-stage9c-navigation` (ver «Fallas») y las 25 siguientes caen en cadena («La aceptación debe comenzar con una base sin configurar»); las 4 omitidas son las de «instalación limpia», sin el segundo Go en el arnés                                                                                                             | 5,9 min ×2     |
| Chromium **sin** `workflow-stage9c-navigation` (`--grep-invert 'navegación 9C'`)     | Diagnóstico. 1.ª corrida: 29 aprobadas, falló `workflow-stage9g-two-terminals` «Dos terminales a la vez» al tocar «Reintentar» (`element was detached from the DOM`: la capa «Sin conexión con el servidor» ya se había cerrado sola al volver la red) y 8 en cadena; **repetida: 38 de 38 aprobadas** (4 omitidas). Las 4 de «instalación limpia» con el Go limpio, filtradas: **4 de 4**. En total, **42 de las 43** aprobadas | 9,7 + 10,0 min |
| **Playwright WebKit**, igual (con el Go limpio)                                      | **Falla** (dos corridas iguales): 17 aprobadas y **26 fallidas** de 43, por la misma especificación y la misma cadena                                                                                                                                                                                                                                                                                                            | 6,1 min ×2     |
| WebKit **sin** `workflow-stage9c-navigation`                                         | Diagnóstico: **42 de 42 aprobadas** (también las 4 de «instalación limpia»). La pantalla de la Mac estaba desbloqueada al empezar y al terminar (`CGSSessionScreenIsLocked` ausente)                                                                                                                                                                                                                                             | 10,1 min       |
| `scripts/macos-pkg/test-scripts.sh` (sin root, destino falso y `launchctl` simulado) | «Scripts del instalador: todo correcto»: 31 comprobaciones, ninguna falla; `bash -n` de los 6 scripts del instalador, de `check-legacy-data.sh` y de `scripts/android-e2e.sh` aprobado                                                                                                                                                                                                                                           | 3 s            |

### Android

| Suite                                                                          | Resultado                                                                                                                                                                                                                                                                                                                                    | Tiempo |
| ------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ |
| `android-format-check` (`ktfmtCheck --rerun-tasks`)                            | Aprobado (21 tareas ejecutadas, sin la caché)                                                                                                                                                                                                                                                                                                | 7 s    |
| `android-test` (`:core:domain:test testDebugUnitTest lintDebug --rerun-tasks`) | **256 pruebas, 0 fallas, 0 omitidas** (contadas en los XML): `core:domain` 103, `core:data` 133, `core:ui` 9, `feature:access` 7, `feature:waiter` 4; 224 tareas ejecutadas. Lint sin errores ni avisos (3 sugerencias informativas: `ObsoleteLintCustomCheck` de lifecycle en `core:ui` y `AutoboxingStateCreation` ×2 en `feature:access`) | 39 s   |
| `assembleDebug assembleRelease assembleDebugAndroidTest`                       | Aprobado (y el `release` firmado con una llave **desechable** del directorio temporal, borrada al terminar; `apksigner verify` correcto, `versionName` 0.1.0, `versionCode` 1, `targetSdk` 36, `compileSdk` 37)                                                                                                                              | 53 s   |

Instrumentadas de todos los módulos (`connectedDebugAndroidTest`), con las aplicaciones de prueba desinstaladas antes en
cada emulador y los XML de `*/build/outputs/androidTest-results/connected/` copiados y contados por módulo
(`feature:access` y `feature:waiter` no tienen pruebas instrumentadas). Ninguna instalación falló. De punta a punta,
`scripts/android-e2e.sh --suite all` (que incluye `field`) con `ADELPOS_E2E_NEW_SERVER` = `iso.sh - server-down` +
`iso.sh <copia> server-up`: un Go temporal **nuevo** para access 1–5, otro con **otra identidad** en las mismas
direcciones para access 6–7 (certificado cambiado, sin reinstalar), y uno nuevo para catalog, waiter y field. Pasos
aprobados / pasos, del resumen del guion.

| Prueba                                                | `ADEL_A1_Tablet_API26` (Android 8)                  | `ADEL_A1_Phone_API33` (Android 13)                                                        | `Pixel_Tablet` (Android 17)                         | `Pixel_8` (Android 17)                                                                    |
| ----------------------------------------------------- | --------------------------------------------------- | ----------------------------------------------------------------------------------------- | --------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Instrumentadas (XML): `app` / `core:data` / `core:ui` | **94, 0 fallas**: 62 / 19 / 13; 46 omitidas (140 s) | **94, 0 fallas**: 62 / 19 / 13; 44 omitidas (210 s)                                       | **94, 0 fallas**: 62 / 19 / 13; 44 omitidas (220 s) | **94, 0 fallas**: 62 / 19 / 13; 44 omitidas (220 s)                                       |
| access (`AccessOnGoTest`, 7 pasos, dos servidores)    | **7/7**                                             | **7/7**                                                                                   | **7/7**                                             | **7/7**                                                                                   |
| catalog (`CatalogOnGoTest`, 3 etapas)                 | **3/3**                                             | **3/3**                                                                                   | **3/3**                                             | **3/3**                                                                                   |
| waiter (`WaiterModeOnGoTest`, 19 pasos, `w1`–`w13c`)  | **19/19**                                           | **19/19**                                                                                 | **19/19**                                           | **19/19**                                                                                 |
| field (`FieldOnGoTest`, 13 instrumentaciones)         | **13/13**                                           | **12/13**: falla `f6`; repetida la suite completa con otro Go: 12/13, falla `f6` otra vez | **13/13**                                           | **12/13**: falla `f6`; repetida la suite completa con otro Go: 12/13, falla `f6` otra vez |
| `--suite all` (total)                                 | **42/42** (481 s)                                   | 41/42 (581 s)                                                                             | **42/42** (572 s)                                   | 41/42 (621 s)                                                                             |
| APK `release` (guion corregido de la Fase F)          | **Aprobado** (71 s)                                 | **Aprobado** (83 s)                                                                       | — (no lo pide el encargo)                           | **Aprobado con la wifi del emulador apagada** (83 s; ver abajo)                           |

Las omitidas de las instrumentadas son las de punta a punta sin los argumentos del guion (`AccessOnGoTest` 7,
`CatalogOnGoTest` 3, `WaiterModeOnGoTest` 19, `FieldOnGoTest` 13 y `ServerApiOnDeviceTest` 2) y, en Android 8,
`ConnectionOnDeviceTest.modoAvionCambiaElIndicador` y `WorkNetworkOnDeviceTest.conRedSinInternetSoloCorreElTrabajoSinRestriccion`
(órdenes de shell de Android 11+): las mismas cifras que las correcciones de R6 en el dispositivo. En Android 8 la red se
cortó desde la Mac (`adb root` + `svc wifi/data`, «Active default network: none»); en las demás, con el modo avión.
`f10` copió 11 capturas del tema oscuro por emulador.

**APK `release` en los emuladores** (el guion de la Fase F, sin cambios de lógica: solo se le puso tope de tiempo a cada
`adb`). Instala el APK firmado con la llave desechable, lo abre como el lanzador, espera 10 s a que termine la búsqueda,
**se detiene y desinstala si la lista muestra cualquier servidor**, toca solo el campo de la dirección (`10.0.2.2:8459`),
pide el código con la acción del teclado, la principal (escucha local del Go temporal) lo aprueba, entra Ana con su
PIN, captura sin conexión un pedido «Por enviar» y WorkManager lo envía al volver la red (una sola partida en Go). En los
tres la lista dijo «No se encontró ninguna computadora principal en la red» y el guion no se detuvo. **En `Pixel_8`**
(Android 17 ve el anuncio mDNS de la Mac por la wifi del emulador), para no abrir la aplicación con la búsqueda en la red
real, **se apagó la wifi del emulador** antes de instalar (`cmd wifi set-wifi-enabled disabled`; quedó solo la red
celular simulada, «MOBILE[NR] CONNECTED», validada) y siguió apagada al terminar; la búsqueda no mostró ningún servidor
y el servicio real **no se tocó**. Arranque en frío del `release` (`am start -W`, `TotalTime`): 443 ms en Android 8,
625 ms en Android 13 y 747 ms en `Pixel_8`. Al terminar, el APK se desinstaló de cada emulador.

**Tamaños de los APK** (`e2b8486`):

| APK                                                    | Bytes                              | MiB   |
| ------------------------------------------------------ | ---------------------------------- | ----- |
| `app-debug.apk` (depuración, con `src/debug`)          | 15 162 806                         | 14,46 |
| `app-release-unsigned.apk` (R8 que reduce sin ofuscar) | 2 405 467                          | 2,29  |
| `app-release.apk` firmado con la llave desechable      | 2 413 659                          | 2,30  |
| APK de pruebas de `app` / `core:data` / `core:ui`      | 1 252 096 / 5 513 412 / 13 037 055 | —     |

**Rendimiento** (`FieldOnGoTest.f2` y `f3`, registro `AdelOnGo`, como en la Fase F; un emulador a la vez sobre la Mac):

| Medida                                                 | `ADEL_A1_Tablet_API26` | `ADEL_A1_Phone_API33` | `Pixel_Tablet` | `Pixel_8` |
| ------------------------------------------------------ | ---------------------- | --------------------- | -------------- | --------- |
| Arranque en frío (proceso → lista de meseros)          | 1282 ms                | 1823 ms               | 2823 ms        | 2762 ms   |
| PIN (con conexión) → Mesas con la primera mesa         | 776 ms                 | 824 ms                | 961 ms         | 959 ms    |
| PIN → la mesa 40 a la vista                            | 1342 ms                | 1424 ms               | 1795 ms        | 1709 ms   |
| «Nueva cuenta» → catálogo de ~300 productos a la vista | 222 ms                 | 240 ms                | 371 ms         | 300 ms    |
| «Enviar a cocina» → «Recibido por el restaurante»      | 310 ms                 | 305 ms                | 307 ms         | 308 ms    |
| Cocina «Listo» → visto por el mesero                   | 917 ms                 | 916 ms                | 666 ms         | 680 ms    |
| Memoria (PSS total de `dumpsys meminfo`)               | ≈ 83 MiB               | ≈ 146 MiB             | ≈ 142 MiB      | ≈ 143 MiB |

### Repetición sobre `238c6f6` (pruebas al día)

Copia exacta nueva en otra carpeta (`git archive 238c6f6`; `git rev-parse` =
`238c6f6095206ca012297a9da4185c14b2ec9bc4`), con los mismos enlaces y el mismo arnés. `git diff --stat e2b8486 238c6f6`:
3 archivos, 26 inserciones y 1 borrado, **solo pruebas**, y `diff -r` de las dos copias no encontró otra diferencia
(fuera de lo que generan las compilaciones):

- `desktop/e2e/workflow-stage9c-navigation.spec.ts`: el menú del usuario espera «Mis preferencias», «Cambiar
  contraseña», «Mi PIN» y «Cerrar sesión».
- `android/app/src/androidTest/…/FieldOnGoTest.kt` y `WaiterModeOnGoTest.kt`: `tap()` comprueba si el nodo está a la
  vista (`assertIsDisplayed`); si no, se lo pide a la lista perezosa (`reveal`, con `performScrollToNode`) y después lo
  toca.

El APK de depuración de la aplicación salió del mismo tamaño (15 162 806 bytes) y el de pruebas de `app` pasó de
1 252 096 a 1 252 336 bytes. 17/11/2026, de 17:15 a 19:07 (hora de Monterrey). La pantalla de la Mac estaba
desbloqueada antes y después de Playwright. En `Pixel_8`, la wifi del emulador estuvo **apagada** en las dos suites
(`cmd wifi set-wifi-enabled disabled`, comprobado con tope de tiempo antes de cada una: solo la red celular simulada).

| Suite                                                                                                         | Resultado                                                                                                                                                                                                                                                                                                                                                                                          | Tiempo         |
| ------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------- |
| Playwright **Chromium** completo, sin filtro (sin el segundo Go)                                              | **39 de 39 aprobadas**, también `workflow-stage9c-navigation`; 4 omitidas: las de «instalación limpia», que necesitan el segundo Go limpio                                                                                                                                                                                                                                                         | 10,2 min       |
| Chromium, las 4 de «instalación limpia», **aparte** con el segundo Go limpio (`ISO_TAG=GC`, 8193, base nueva) | **4 de 4**                                                                                                                                                                                                                                                                                                                                                                                         | 26 s           |
| Playwright **WebKit** completo, sin filtro (sin el segundo Go)                                                | 1.ª corrida: 30 aprobadas, 9 fallidas y 4 omitidas: falló `workflow-stage9g-two-terminals` «Dos terminales a la vez» al tocar «Reintentar» (la capa «Sin conexión con el servidor» ya se había cerrado sola: reintenta sola con su cuenta regresiva y a veces gana al toque de la prueba) y las 8 siguientes en cadena. **Repetida: 39 de 39 aprobadas**, 4 omitidas (las de «instalación limpia») | 9,7 + 10,1 min |
| WebKit, las 4 de «instalación limpia», **aparte** con el segundo Go limpio (base nueva)                       | **4 de 4**                                                                                                                                                                                                                                                                                                                                                                                         | 26 s           |
| `ktfmtCheck --rerun-tasks`                                                                                    | Aprobado (21 tareas ejecutadas)                                                                                                                                                                                                                                                                                                                                                                    | 7 s            |

| `scripts/android-e2e.sh`, un Go temporal nuevo por suite | `ADEL_A1_Phone_API33` (Android 13) | `Pixel_8` (Android 17, wifi apagada)                                    | `ADEL_A1_Tablet_API26` (Android 8) |
| -------------------------------------------------------- | ---------------------------------- | ----------------------------------------------------------------------- | ---------------------------------- |
| `--suite field` (13 instrumentaciones)                   | **13/13** (281 s)                  | 1.ª corrida colgada por el entorno (abajo); **repetida: 13/13** (271 s) | **13/13** (250 s)                  |
| `--suite waiter` (19 pasos)                              | **19/19** (180 s)                  | **19/19** (190 s)                                                       | — (no lo pide el encargo)          |

En los tres, `f6` registró «pedido de la mesa: RECEIVED» y «renglones de la malteada que siguen por enviar en la cuenta
destino: 0» (M3). Las capturas de `ADEL_A1_Phone_API33` durante `f6` (cada 6 s) muestran la lista desplazada con
«Enviar a cocina» **a la vista y habilitado** (el botón principal azul, debajo de «Total de lo enviado y lo nuevo:
$275.00»), y el pedido llegó «Recibido».

**Cuelgue en `Pixel_8` (entorno).** La primera corrida de `--suite field` (desde las 17:57) aprobó `f1` a `f8`, también
`f6`, pero se volvió muy lenta desde `f7c` (18 min en un paso que tarda segundos) y `f9` no terminó: el tope de 20 min
sin progreso la detuvo a las 18:53. En ese momento Android llevaba 6 min encendido dentro del emulador (`uptime`) aunque
el proceso del emulador llevaba 57 min (el sistema se reinició solo), y la Mac tenía la memoria de intercambio casi llena
(6 134 de 7 168 MB) y un promedio de carga de 113. Se hizo lo que pide la regla: Go temporal detenido, `adb
kill-server`, emulador apagado y encendido **una vez más** (wifi apagada otra vez) y la suite repetida con un Go nuevo:
13/13, y después `waiter` 19/19. No es de la aplicación.

### Fallas en `e2b8486`, corregidas en la prueba

**1 · Playwright, `workflow-stage9c-navigation.spec.ts:289` (Chromium y WebKit, dos veces en cada uno).** El paso 8
espera que el menú del usuario tenga exactamente «Mis preferencias», «Cambiar contraseña» y «Cerrar sesión», y el
administrador ve además **«Mi PIN»**:

```text
Locator: getByRole('group', { name: 'Menú del usuario' }).getByRole('button')
- Expected  - 0
+ Received  + 1
    "Mis preferencias",
    "Cambiar contraseña",
+   "Mi PIN",
    "Cerrar sesión",
```

«Mi PIN» lo agregó la Fase B (`b11c01f`, `desktop/src/navigation/UserMenu.tsx:66`, solo con `waiter_devices.use`, que la
0025 da al administrador): es el comportamiento pedido, y la especificación de la 9C no se actualizó (la Fase B solo
corrió Playwright filtrado y dejó la regresión completa para la Fase G). Como las especificaciones comparten la base y el
ejecutor guarda la terminal preparada en memoria, al reiniciarse tras la falla **las 25 siguientes fallan en cadena**
(«La aceptación debe comenzar con una base sin configurar», `e2e/native-driver.ts:474`). Sin esa especificación, las 42
restantes aprueban en los dos motores (arriba). **Corregida en la prueba** (`238c6f6`): la lista esperada incluye «Mi
PIN». Con eso la especificación completa aprueba en los dos motores, también lo que en `e2b8486` quedó sin comprobar
después de la línea 289 (el resto del paso 8, «Mis preferencias» sin «Pantalla inicial», y el paso 9, la captura con la
barra reducida), y ya no hay cadena.

**2 · `FieldOnGoTest.f6_transferirYCombinarConLaCuentaAbierta` en los teléfonos** (`ADEL_A1_Phone_API33` y `Pixel_8`;
en `--suite all` y en la repetición de `--suite field` con un Go temporal nuevo, y en API 33 otras dos corridas de
diagnóstico `--steps "f1 f6"` con capturas de pantalla). En las dos tabletas aprueba. Escenario: Ana envía una
hamburguesa en la mesa 8, vuelve a la cuenta y captura una malteada (guardada en el borrador de Go); la principal
transfiere lo enviado a una cuenta de la mesa 9 y después **combina** la cuenta de Ana en ella; la captura muestra
«La cuenta ya no está abierta» y Ana toca «Seguir en la cuenta en que se combinó» (todo eso aprueba); después la prueba
toca «Enviar a cocina» y espera el pedido «Recibido»:

```text
java.lang.IllegalStateException: No llegó: el pedido de la mesa
  at local.adel.pos.android.FieldOnGoTest.waitUntil(FieldOnGoTest.kt:790)
  at local.adel.pos.android.FieldOnGoTest.waitForOrder(FieldOnGoTest.kt:1136)
  at local.adel.pos.android.FieldOnGoTest.f6_transferirYCombinarConLaCuentaAbierta(FieldOnGoTest.kt:435)
```

Lo observado: después del toque, el registro del Go temporal no tiene **ninguna** petición de escritura del dispositivo
(solo lecturas) en los 60 s de espera; las capturas cada 6 s muestran la misma pantalla sin cambios: el panel de la
cuenta en «Mesa F9 · folio N» con «Guardado en la computadora principal.», lo enviado («1 × Hamburguesa F», «Liberado a
preparación»), «Por enviar» con «Tacos F» («Capturado en otra terminal») y «Malteada F», y «Total de lo enviado y lo
nuevo: $275.00» al pie de la pantalla; el botón «Enviar a cocina» queda **debajo**, fuera de la vista (en la tableta
cabe). El toque de la prueba (`tap` = `performScrollTo` dentro de `runCatching` + `performClick`) no llegaba al botón:
la lista perezosa ya lo había compuesto fuera de la vista y `performScrollTo` no desplaza una lista perezosa, así que
el toque caía fuera de la pantalla, como pasó con otras listas en la Fase E (`735a97a`). **Corregida en la prueba**
(`238c6f6`): `tap()` trae el nodo a la vista con la lista antes de tocarlo. Con la misma aplicación, `f6` aprueba en los
dos teléfonos y en la tableta de Android 8, y las capturas muestran el botón a la vista y habilitado (arriba): el camino
de M3/A1-D19 «Seguir en la cuenta en que se combinó» → «Enviar a cocina» queda verificado también en teléfonos. Capturas
y registros en el directorio temporal de la sesión (`g/android/api33-f6-diag/shots/`,
`g/android/pixel8-field-repeat-shots/`, `g/android2/api33-field-shots/` y el `field-server.log` de cada corrida).

**Repetido y por qué**: Chromium completo (falla 1, para descartar el entorno: igual); WebKit completo (igual); la
corrida de diagnóstico de Chromium sin la especificación de la 9C (la primera falló en `workflow-stage9g-two-terminals`
por la capa «Sin conexión» que se cerró sola antes del toque; repetida, aprobó: **intermitente**, de la prueba);
`--suite field` en `ADEL_A1_Phone_API33` y en `Pixel_8` (falla 2: igual), más dos corridas de diagnóstico de `f1` y `f6`
en API 33 (igual; el volcado de `uiautomator` no funciona mientras corre la instrumentación, así que solo hay capturas).
`TestOperationalReports` y `TestServiceHostOnMacOS` se corrieron otra vez con `-v` solo para comprobar que no se
omitieron. Sobre `238c6f6`: WebKit completo (la primera corrida falló en `workflow-stage9g-two-terminals`, la misma
intermitencia de la capa «Sin conexión»; repetida, aprobó) y `--suite field` en `Pixel_8` (la primera se colgó por el
entorno; con `adb` reiniciado y el emulador encendido de nuevo, aprobó). Ese fue el único cuelgue.

**Al terminar** (también después de la repetición sobre `238c6f6`): ningún emulador encendido (`adb devices` vacío; los cuatro con `emu-down`, de solo lectura: los AVD
del propietario no cambiaron); ningún `ad-el-pos-server` de las pruebas en marcha (solo el servicio real, al que no se
le hizo ninguna petición); ninguna base `adel_pos_test_*` en el clúster 55499; los puertos 8183, 1421, 8193, 8199, 8459
y 8463 sin escucha; la llave de firma desechable y su archivo de propiedades borrados (también la caché de configuración
de Gradle de la copia); `docs/etapas/*.md` idéntico por md5; y el repositorio principal sin cambios de este agente (en
`etapa-a1-android`, ahora en `238c6f6`, el commit del coordinador). Los artefactos de Playwright quedaron en la copia (`.local/browser-artifacts`), no en `.local`
del repositorio. No se detuvieron los demonios de Gradle (los comparte Android Studio del propietario). **No se compiló el
`.pkg` 0.12.0 ni el APK de prueba con la llave de desarrollo** (los hace el coordinador).

## Instalación real

Pendiente del propietario. Al escribir este documento, el servicio de la Mac tiene la 0.11.0 (esquema 24). Se actualiza
con el `.pkg` 0.12.0, que compila el coordinador y que instala el propietario con «Servidor del restaurante» marcada
(parada: contraseña de administrador); al arrancar, el `host` respalda la base y aplica la 0025. Después, el propietario
instala el APK de prueba en una tableta o un teléfono y lo empareja con la computadora principal
([ANDROID_INSTALAR.md](ANDROID_INSTALAR.md)). Pendiente también del incidente de la Fase F: revisar en Administración ›
Sistema › Terminales que no quedó ninguna solicitud de emparejamiento.
