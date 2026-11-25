# Verificación · Etapa K1 (aplicación de cocina para Android y el «Reconectando» del escritorio)

Verificación por fases del **24 y el 25 de noviembre de 2026**, en la Mac (Apple Silicon, macOS 26.5.2). Cada fase se
hizo en una rama local que el coordinador unió en `etapa-k1`. Esa rama se creó desde `main` (`2f715d4`: 0.15.0,
esquema 28, aplicación de meseros 0.4.0, con la L1 fusionada). La especificación está en `eb1fe7b`. Commits y avances de
cada fase:

- **Fase 0** (verificación, estructura del proyecto y la causa del «Reconectando»; sin código del producto): avance en
  `45fa9b2`.
- **Fase A** (servidor: el dispositivo `kitchen`, sus rutas `kitchen-display/*`, el cupo, los eventos, la `0029` y el
  contrato): `aa8c921` a `e6e4c06`; avance en `88b4d8d`.
- **Fase B** (`android-core/` como build propio con lo compartido movido sin cambiar su comportamiento, meseros 0.4.1 y
  el proyecto `cocina/`): `b5ef209` a `fa3c7b7`; avance, con la revisión R1, en `70ee1a1`.
- **Revisión independiente R1** (el emparejamiento como pantalla de cocina y la autorización de sus rutas): la prueba
  `TestStageK1R1KitchenDisplaySecurity`, unida en `f545c4d`; sus correcciones en `f690bce` (K1-D29).
- **Fase C** (la pantalla de pedidos de la tableta): `adb0320` a `104a50e`; avance en `efaaba7`.
- **Fase E** (Terminales, la licencia y el «Reconectando» del escritorio): `f3830bf` a `9d7dd13`, `895efa2`
  (`kitchen-display/unpair`, K1-D32) y los retoques de `cbcb184`; avance en `78bb708`.
- **Fase D** (la conexión tolerante de la tableta, las alertas, la pantalla, los ajustes y
  [COCINA_INSTALAR.md](COCINA_INSTALAR.md)): unión en `cd1f15d`; avance en `e6c03a4`.
- **Documentos de la etapa**: `82fa59c` (`CLAUDE.md`), `7185752` ([ANDROID_INSTALAR.md](ANDROID_INSTALAR.md)) y
  `6c44ff6` ([API_V1.md](API_V1.md)).
- **Fase F** (rama `etapa-k1-f`, desde `e6c03a4`): las versiones en `4debb8c`, la corrección de una prueba de Vitest en
  `dd532f8`, la del texto «Actualiza el servidor del restaurante» de la tableta de cocina en `43b2bd4` y los documentos
  de la entrega (`9b9f472` y el de este documento).

**Servidor 0.16.0.** API v1, migración `0029_pantalla_de_cocina.sql` (esquema **29**), una sola para toda la etapa con
la sección **«Fase A»**: los límites de color generales de las pantallas en `kitchen_settings`, los ajustes y el «cupo
liberado» de cada pantalla en `devices`, el cupo `kitchen_display` sin sesión y el acuse de recepción sin usuario (los
dos solo para un dispositivo `kitchen`, con su disparador), `app.kitchen_display_dismissals` y dos índices.
`min_client_versions.kitchen` es `0.1.0` y la capacidad nueva, `kitchen-display-v1`. **Sigue siendo una versión de
desarrollo**, como la 0.15.0: confía solo en las llaves del entorno `test`
([licencias/PASO_A_PRODUCCION.md](licencias/PASO_A_PRODUCCION.md)).

**Aplicación de cocina 0.1.0** (`com.adelpos.cocina`, `versionCode` 1, `minSdk` 26, proyecto `cocina/`): un
dispositivo `kitchen` sin usuario ni sesión que solo llega a su lista cerrada de rutas. Exige el servidor 0.16.0 con
`kitchen-display-v1` (K1-D27).

**Aplicación de meseros 0.4.1** (`com.adelpos.meseros`, `versionCode` 5): lo compartido se movió a `android-core/`
(incluido con `includeBuild`), sin cambiar su comportamiento; se instala encima de la 0.4.0 sin emparejar de nuevo.

**Referencias.**

- Especificación: [etapas/ETAPA_K1_KDS_ANDROID_v3.md](etapas/ETAPA_K1_KDS_ANDROID_v3.md).
- Avance, con el detalle de cada fase y las decisiones K1-D1 a K1-D37: [ETAPA_K1_AVANCE.md](ETAPA_K1_AVANCE.md).
- Guías del propietario: [COCINA_INSTALAR.md](COCINA_INSTALAR.md) y [ANDROID_INSTALAR.md](ANDROID_INSTALAR.md).
- Las reglas de cada carpeta Android: [cocina/CLAUDE.md](../cocina/CLAUDE.md),
  [android-core/CLAUDE.md](../android-core/CLAUDE.md) y [android/CLAUDE.md](../android/CLAUDE.md).
- Evidencia anterior: [etapa L1](VERIFICATION_STAGEL1.md).

Este documento registra **solo lo que se ejecutó**.

**Alcance de esta evidencia.** Las cifras de las fases son las que cada agente registró sobre su propia copia, según el
avance. Lo que valida el código completo de la etapa es la «Regresión final», sobre copias exactas (`git archive`): la de
`4debb8c` (todas las fases unidas y las versiones de la entrega) para el servidor y el escritorio, y la de `43b2bd4` (con
la corrección del texto compartido de Android) para Android.

**Las pruebas no tocaron la instalación real**: ni el servicio `local.adel.pos.server`, ni `/Library/Application
Support/AD-EL POS`, ni `/Applications/AD-EL POS.app`, ni `.local` del repositorio, ni el Keychain, ni PostgreSQL en
55432, ni Go en 8181 o en la escucha de red 8443. Ninguna prueba usó `sudo`, `installer` ni `launchctl` (salvo el
`launchctl` simulado de las pruebas de los scripts del instalador), ni las órdenes de `dev.sh` de la instalación. **La
hora de la Mac nunca se cambió.** `kill -STOP` y `-CONT` solo se mandaron al PID del Go temporal de cada prueba,
comprobado antes con `lsof` y `ps`.

**Ninguna prueba llamó a `licensing.sicrat.com`**, y los códigos de las licencias de prueba no se usaron ni se
escribieron en ningún lado: las pruebas usan solo `contracts/licencias/v1/fixtures/vectors.json` y el simulador
`license-test`.

**Datos.** Todas las pruebas usan **datos sintéticos**, creados por cada prueba en un Go temporal con base desechable.

## Entorno aislado

El de la L1 ([VERIFICATION_STAGEL1.md](VERIFICATION_STAGEL1.md), «Entorno aislado»), con lo que agregó la etapa:

- **Como mucho dos agentes a la vez** (§0 de la especificación), cada uno con su `ISO_TAG`, sus puertos y sus
  emuladores. La regresión final la hizo un solo agente.
- El clúster PostgreSQL 18.6 temporal en 55499, solo en loopback, con una base `adel_pos_test_*` desechable por paquete,
  por motor de Playwright y por Go temporal.
- `go-int ./cmd/ad-el-pos-server` usa puertos fijos (8192–8196 y 55511–55513): se corrió con un candado, uno a la vez.
- **Los tres proyectos Android** con el arnés (`ISO_GRADLE_DIR=android|cocina|android-core`). Los emuladores, **sin
  ventana y de solo lectura**, siempre con `ANDROID_SERIAL` y tope de tiempo en cada `adb`: durante las fases,
  `Pixel_Tablet` (Android 17) y `ADEL_A1_Tablet_API26` (Android 8.0), en horizontal; en la regresión final, también el
  teléfono `Pixel_8` (Android 17) para la de meseros. `ADEL_A1_Phone_API33` (`emulator-5554`), del propietario, no se
  tocó.
- **Las pruebas instrumentadas** corren con `AdelTestRunner` (meseros) y `CocinaTestRunner` (cocina), que buscan el tipo
  NSD de pruebas `_adelpostest._tcp`: ninguna abrió una aplicación normal con la búsqueda automática en la red real.
- **Los cortes de la computadora** se hacen pausando el Go temporal (`kill -STOP` y `-CONT`, siempre con el `-CONT` en
  una limpieza), y los de red, con el modo avión del emulador (Android 17) o con `adb root` y `svc wifi/data` (Android
  8). La red de la Mac no se toca.

Ninguna prueba corrió en una tableta o un teléfono reales, ni en la wifi de un restaurante. Vitest y Playwright
sustituyen el puente nativo del escritorio por la simulación de `e2e/native-driver.ts`.

## Resultados ejecutados por fase

| Fase o parte  | Lo que se ejecutó y su resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 0             | Lectura del código y una prueba de concepto de Gradle fuera del repositorio (`includeBuild`, catálogo único, bloqueos por build); con un Go temporal sin uso, la medida del canal de eventos: Go lo renueva cada 30 s a propósito y Rust lo trataba como una falla (un hueco sin canal de unos 0,6 s cada 30 s)                                                                                                                                                                                                                                                                |
| A             | `TestStageK1KitchenDisplay` (emparejar como pantalla, la lista cerrada de 33 familias de rutas, el filtrado por estación y por recepción con la impresión de hoy, tarjetas, marcas y el aviso al mesero, recuperar sin aviso, eventos y reconexión sin duplicados, versión instalada, cupo, licencia y hora), `TestStageK1Upgrade` (28 → 29) y la pantalla en `TestContractV1`; `go-int` de `database`, `kitchen`, `licenseseats`, `licensing`, `administration`, `operations` (877 s), `apicontract` y `cmd`; `go-unit`; Playwright `restaurant.spec` y `9f-kitchen-stations` |
| B             | Antes y después del movimiento, **los mismos casos**: JVM 413 (62 XML) e instrumentadas 168 en cada tableta, 0 fallas; e2e `access` 6/6 y `alerts` 11/11 en las dos; la actualización 0.4.0 → 0.4.1 en dos partes en `Pixel_Tablet`; `cocina` JVM 7/7, instrumentadas 3 + 2 omitidas y `kitchen` `k1`–`k2` en las dos                                                                                                                                                                                                                                                          |
| R1 (revisión) | `TestStageK1R1KitchenDisplaySecurity`: **852 combinaciones de ruta y método rechazadas** a la pantalla (también con el `Authorization` del administrador y por la red), el alcance por estación en todo y el cupo atómico. Dos hallazgos de gravedad baja (R1-1 y R1-2), corregidos en la Fase E (K1-D29)                                                                                                                                                                                                                                                                      |
| C             | JVM 439 (65 XML); instrumentadas de cocina 17 (11 corridas) y de meseros y `android-core` 168 en las dos tabletas; `kitchen` 8/8 (`k1`–`k6` y los conteos de `ready_notices`) en las dos; la actualización 0.4.0 → 0.4.1 en dos partes también en Android 8                                                                                                                                                                                                                                                                                                                    |
| E             | `go-int` de `kitchen`, `operations`, `administration`, `apicontract` y `licensing`; `go-unit`; Vitest 1 015; `cargo` 122 y 4 ignoradas; la prueba de Rust de 10 minutos por la escucha local y por la de red (1 `connected`, 0 `reconnecting` en cada una); Playwright `workflow-stagez-k1-reconnect` y `-terminals` y 15 especificaciones afectadas, en Chromium y WebKit                                                                                                                                                                                                     |
| D             | JVM 476 (68 XML); instrumentadas de cocina 15 corridas y de meseros 159/158 + `android-core` 10 en las dos tabletas; `kitchen` `k1`–`k13` y `endtoend` 4/4 en las dos; las medidas del margen de K1-D33 (abajo)                                                                                                                                                                                                                                                                                                                                                                |
| F             | La regresión final (abajo)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |

En cada fase se corrieron además `format`, `fmt-check` y `sync-check`, y los MD5 de `docs/etapas/` quedaron iguales
antes y después.

## Revisión independiente

La §0 de la especificación pide un revisor independiente solo para el emparejamiento como pantalla de cocina y la
autorización de sus rutas. El revisor no escribió el código y no cambió código de producción: solo agregó su prueba.

| Revisión                                                      | Hallazgos                                                                                                                                                                                                                                                                                                                                                                                                                        | Corrección y decisiones                                                                                                                                                                                                                                                                                                                                   |
| ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **R1**: el emparejamiento como pantalla de cocina y sus rutas | Ninguno alto ni medio. R1-1 (baja): `settings-access` respondía con un usuario o una contraseña demasiado largos un 403 con el texto de las autorizaciones de cobros, que el contrato no tenía. R1-2 (baja): una petición que ya había pasado `authenticateDevice` seguía aunque la pantalla se revocara mientras esperaba. Fuera del alcance: al emparejar, `device_kind` y `X-Client-Kind` solo se comparaban para la pantalla | K1-D29, en la Fase E: la longitud se valida antes, como en el inicio de sesión (401 `AUTHORIZATION_INVALID`, los vacíos con 400 documentado); para la pantalla, `touchDevice` solo actualiza con `revoked_at IS NULL` y si no cambia ninguna fila responde 401 `DEVICE_REVOKED`; la coincidencia de tipos vale para los tres. La prueba de la R1 se quedó |

## La causa del «Reconectando» falso y su corrección

- **La causa** (Fase 0, K1-D10): Go cierra a propósito el canal de eventos cada 30 s para renovarlo, y Rust trataba ese
  fin normal como una falla: mandaba «reconnecting», esperaba 500 ms y, al volver, un `connected` que recargaba todas
  las pantallas. De ahí el recuadro «Reconectando» de 1 o 2 segundos cada 30 s, con la computadora en línea.
- **La corrección** (Fase E): en Rust, el fin normal de un canal que respondió 200 se vuelve a abrir al momento desde el
  mismo cursor, sin `reconnecting` ni un `connected` nuevo; solo una falla real cuenta. Un solo estado de conexión en
  React con la tolerancia de la A3 (3 fallas seguidas y 20 s sin éxito, o el sistema sin red; «En línea» con el primer
  éxito), que leen la barra, la capa «Sin conexión con el servidor» y los avisos. **Go no cambió**: Android y la
  aplicación de cocina no cambian de comportamiento (§7.5).
- **La prueba**: la de Rust de 10 minutos sin uso contra un Go temporal, por la escucha local y por la de red a la vez,
  con cero `reconnecting` (en la Fase E y otra vez en la regresión final), y Playwright con pausas del Go de 1, 3, 5 y
  42 s, también como terminal por la red.

## Medidas del margen sin red de la tableta (Fase D, K1-D33)

| Corte | `Pixel_Tablet` (modo avión): sin red para Android | `ADEL_A1_Tablet_API26` (wifi y datos): sin red para Android | Aviso                                                                 |
| ----- | ------------------------------------------------- | ----------------------------------------------------------- | --------------------------------------------------------------------- |
| 1 s   | 2,9 s                                             | 1,3 s                                                       | ninguno en los dos                                                    |
| 3 s   | 4,0 s                                             | 3,3 s                                                       | ninguno en los dos                                                    |
| 5 s   | 6,0 s                                             | 5,3 s                                                       | ninguno en los dos                                                    |
| 35 s  | 36,1 s                                            | 35,3 s                                                      | el aviso aparece al pasar el margen de 10 s y se quita solo al volver |

Con la computadora colgada (el Go pausado: acepta la conexión pero no responde), el aviso sale a los ~41 s (K1-D37).

## Regresión final

> **Resumen.**
>
> - **El servidor y el escritorio** (copia de `4debb8c`):
>   - Go aprobó: `go-unit` y la integración de los 17 paquetes. `reports/operational` cayó en su franja conocida de
>     00:00 a 05:00 y aprobó al repetirse a las 05:02 sobre `2e81ff1`;
>   - `cargo` 122;
>   - Vitest 1 015 de 1 015, ya con la prueba corregida;
>   - **Playwright 53 de 53 en Chromium y 53 de 53 en WebKit**, con las 4 de «instalación limpia», después de repetir
>     por la medianoche (Chromium) y por una carrera de una prueba de la 9D (WebKit);
>   - **la prueba de 10 minutos sin uso, con 0 `reconnecting`** por la escucha local y por la de red.
> - **Android** (copia de `43b2bd4`):
>   - JVM 476;
>   - instrumentadas en los tres emuladores, 0 fallas;
>   - `--suite all` 84/84 en `Pixel_Tablet` y en `Pixel_8` y 83/84 en Android 8 (`takeaway t6`, el pendiente conocido
>     A3-D32);
>   - `kitchen` 14/14 y `endtoend` 4/4 en las dos tabletas;
>   - la actualización encima 0.4.0 → 0.4.1 en sus dos partes.
> - **Dos correcciones**:
>   - una prueba de Vitest con una carrera;
>   - un texto de la tableta de cocina que nombraba a la aplicación de meseros y su versión.
> - **Los entregables**, sobre una copia exacta de `9b9f472`. **No se instaló nada.**

Una sola vez (§0, «Eficiencia»), en la rama `etapa-k1-f` (desde `e6c03a4`, con todas las fases, la R1 y los documentos
de la etapa), 24 y 25/11/2026, de 23:30 a 03:15 (hora de Monterrey). Un solo agente, sin otro trabajando a la vez. Go
1.27.3, Playwright 1.65.0, PostgreSQL 18.6, macOS 26.5.2.

- **El commit de las versiones**, `4debb8c`: `config/product.json` en **0.16.0** con `dev.sh sync` (los metadatos, la
  copia generada, `package.json`, `tauri.conf.json`, `Cargo.toml` y los dos lockfile), `schema_version` 29 y
  `min_client_versions` sin cambio; `KitchenServerRequirements` exige el servidor **0.16.0** con
  `kitchen-display-v1` (K1-D27) y su prueba agrega un servidor 0.15.0 con la capacidad. Meseros sigue en 0.4.1 (5) y
  cocina en 0.1.0 (1).
- **Las copias**: el servidor y el escritorio corrieron sobre una copia exacta de `4debb8c` (`git archive`, 2 085
  archivos, idénticos por md5 a `git ls-files`), con `.tools`, `.cache/go-mod`, `.cache/playwright` y
  `desktop/node_modules` enlazados y el arnés aislado (`ISO_TAG=F`). Android corrió sobre una copia exacta de
  `43b2bd4`, que agrega a `4debb8c` la corrección de un texto compartido (abajo, «Lo corregido»). De `4debb8c` a
  `43b2bd4` cambiaron solo una prueba de Vitest y `android-core`, `android/feature/access` y `cocina/`: nada del
  servidor ni del escritorio fuera de esa prueba.
- **Los puertos**: el clúster temporal 55499; Playwright con su Go temporal en 8231, la vista previa en 1443, el
  servidor de licencias simulado en 8511 con la confianza de `vectors.json` y el Go sin activar en 8233
  (`ISO_LICENSE_SIM=1`), y un Go limpio nuevo por motor (`ISO_TAG=FC`, 8232 con su escucha de red en 8502) para las
  cuatro de «instalación limpia» de la 9H. La prueba de 10 minutos y Android, con un Go temporal 8231/8501.

### Servidor y escritorio

| Suite                                                                                                                                             | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| ------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `fmt-check` (comprobación de datos reales, Prettier, `gofmt`, `cargo fmt`) y `sync-check`                                                         | Aprobados, en la copia y en el árbol de trabajo (con su índice de git: «el índice de git no tiene respaldos ni paquetes de migración»). «Metadatos coherentes.» (0.16.0, esquema 29)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `go-unit`                                                                                                                                         | Aprobado (83 s): `go vet` (normal e `integration`), `go test -race -count=1 ./...` (**39 paquetes `ok`**, 7 sin pruebas de unidad), `GOOS=windows` `go build` y `go vet` (normal e `integration`). Incluye `TestNoAutomatedTestCallsTheRealLicenseServer`, `TestTestToolsNeverShip` y `TestProductionBuildFailsWithoutProductionKeys`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `go vet -tags 'integration legacydocker'`, en macOS y con `GOOS=windows`                                                                          | Aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Integración PostgreSQL, los **16 paquetes** de `scripts/database.py` y `./internal/kitchen`, con `-v`                                             | **16 de 17 a la primera y 17 de 17 con `reports/operational` repetida a las 05:02** (sobre `2e81ff1`, aprobada entera en 42 s; ver «Lo que corrigió o repitió el coordinador»): `database` 18 s, `administration` 325 s, `catalog` 32 s, `operations` 886 s (con `TestStageK1KitchenDisplay`, `TestStageK1R1KitchenDisplaySecurity` y las de la 9F, A1, A2 y L1), `payments` 3 s, `reports` 163 s, `designer` 28 s, `identity` 25 s, `apicontract` 64 s (`TestContractV1` con la pantalla de cocina), `backupschedule` 9 s, `discovery` 6 s, `legacyimport` 15 s, `licensing` 12 s, `licenseseats` 6 s, `kitchen` 6 s y `cmd/ad-el-pos-server` 70 s (con `TestServiceHostOnMacOS`, 63 s, con el candado). Las únicas omitidas son las opcionales de siempre: las mediciones (`TestLegacyImportVolume`, `TestStage9HLegacyReportsVolume`, `TestReportsVolumeMeasurement`, `TestOperationalVolume` e `inventory_b_measure_release`), la impresora física (`TestControlledCUPSPrinter`) y la búsqueda mDNS en vivo (`TestHoldAnnouncementForLiveSearch`). **`reports/operational`**: corrió a las 00:00, dentro de la franja de 00:00 a 05:00 de Monterrey en que `TestOperationalReports` falla por una dependencia de la hora anterior a la 9H (§6 de `CLAUDE.md`): las mismas 4 subpruebas de siempre fallaron (`turno_en_arqueo…`, `cierre_definitivo…`, `esperado_y_diferencias…` y `compras_por_periodo…`), por la fecha del turno abierto, que queda fuera de los días comerciales consultados; el resto aprobó. La K1 no tocó `internal/reports`. No se repitió después de las 05:00 (abajo, «No verificado») |
| Docker real del lector                                                                                                                            | **No se corrió**: Docker Desktop apagado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `vitest` (`tsc --noEmit` + Vitest)                                                                                                                | Sobre `4debb8c`: **107 archivos, 1 014 de 1 015**: falló una vez la prueba de la 9F del primer toque de Preparación, con la Mac cargada (`go-unit` con `-race` a la vez, carga 32); sola aprobó 5 de 5. Se corrigió la prueba (`dd532f8`, abajo). Sobre la copia de `9b9f472` (con la corrección): **107 archivos, 1 015 de 1 015**, con la Mac cargada (carga 20, el `.pkg` compilando a la vez)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `build` (`npm run build`)                                                                                                                         | Aprobado (el aviso de tamaño de fragmento de siempre)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `cargo test --locked --offline`                                                                                                                   | **122 aprobadas, 4 ignoradas** (entre ellas la de 10 minutos, que se corre aparte)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `scripts/macos-pkg/test-scripts.sh` (sin root) y `bash -n`                                                                                        | «Scripts del instalador: todo correcto» (31 comprobaciones); `bash -n` de `common.sh`, `test-scripts.sh`, `app/preinstall`, `server/preinstall` y `postinstall`, `service/postinstall`, `build-macos-pkg.sh`, `check-legacy-data.sh` y `android-e2e.sh`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| **Playwright Chromium** completo (53 pruebas, 41 especificaciones)                                                                                | **47 de 53 en la primera corrida** (22,3 min), con las 4 de «instalación limpia» (ninguna omitida). La primera falla, `workflow-stagel1-notices`, fue por la medianoche: cerró el aviso diario a las 23:59:58 y al volver a entrar, a las 00:00:05, ya era otro día y el aviso volvió a salir, como debe; las 5 siguientes fallaron en cascada («La aceptación debe comenzar con una base sin configurar»). **Las 6 repetidas, 6 de 6** (5,5 min): `workflow-stagel1-notices`, `-seats`, `workflow-stagez-k1-reconnect` (2), `-terminals` y `workflow-zz-test-data`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| **Playwright WebKit** completo                                                                                                                    | **53 de 53** en la segunda corrida (26,9 min), con las 4 de «instalación limpia» y las 3 de la K1. La primera corrida falló en `workflow-stage9d-accounts` y el resto en cascada: la página, recién entrada, perdió su sesión («Tu sesión se cerró porque el usuario inició sesión de nuevo en esta terminal.») porque la prueba vuelve a entrar por la API con la misma terminal justo después de entrar por la página, y lo más probable es que una petición de la página ya fuera en camino con la sesión anterior. Es una carrera de la prueba, no de la aplicación (que no tiene ese segundo inicio de sesión por la API); en la segunda corrida aprobó, y el resto de la suite también                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| La prueba de Rust de 10 minutos sin uso (`idle-fase-f.sh`), por la escucha local y por la de red a la vez, contra un Go temporal recién levantado | **Las dos aprobadas**, de 00:42 a 00:53, con la Mac casi sin carga (de 3 a 4) y sin Gradle compilando: «600 s: 817 mensajes de cambios, 1 connected, 0 reconnecting» por `http://127.0.0.1:8231` y «600 s: 818 mensajes de cambios, 1 connected, 0 reconnecting» por `https://127.0.0.1:8501` (con la huella fijada), cada una con `test result: ok`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |

Pantalla de la Mac **desbloqueada** (`ioreg … CGSSessionScreenIsLocked` sin la clave) antes y después de cada corrida
de Playwright. **Ninguna prueba llamó a `licensing.sicrat.com`**: los Go temporales de Playwright tenían
`license_server_url` = `https://127.0.0.1:8511` (el simulador); su única falla con él es la esperada (`INVALID_PROOF`
de la licencia firmada con `license-test issue`, L1-D51). Los Go limpios arrancaron con `license_server_disabled`.
Ninguna bitácora nombra el servidor real.

### Android

Sobre la copia exacta de `43b2bd4`. Antes de cada corrida de instrumentadas se desinstalaron las aplicaciones y sus
paquetes de prueba (`pm list packages`: ninguno) y se contaron los XML de cada módulo. Cada suite de punta a punta,
con un Go temporal nuevo en modo de pruebas (`ISO_TAG=F`, 8231 y 8501), que confía en las llaves de `vectors.json` y
recibe su licencia de prueba sin internet. Antes de las suites con pausas (`kitchen`), el PID que escucha en 8231 y en
8501 se comprobó con `lsof` y `ps` (el binario de ese Go temporal); el guion vuelve a comprobarlo antes de cada pausa.
Un emulador a la vez durante las pruebas.

**Los tres builds, en la Mac.**

| Prueba                                                                                     | Resultado                                                                                                                                                                                                                                                                                                                                               |
| ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| JVM y Robolectric (`--rerun`), Lint y `ktfmtCheck` de `android-core`, `android` y `cocina` | **68 XML, 476 pruebas, 0 fallas**: `android-core` 93 (`model` 60, `network` 26, `ui` 7), meseros 327 (`core:data` 165, `core:domain` 136, `core:ui` 8, `feature:access` 7, `feature:takeaway` 5 y `feature:waiter` 6) y cocina 56. Lint sin problemas (solo las 2 sugerencias de siempre en `feature:access`); ktfmt aprobado. Lo mismo sobre `4debb8c` |

**Instrumentadas** (`connectedDebugAndroidTest`; las pruebas contra Go se omiten aquí, corren con el guion):

| Emulador                                   | `android-core` | Meseros (`app` / `core:data` / `core:ui`)                  | Cocina                                                         |
| ------------------------------------------ | -------------- | ---------------------------------------------------------- | -------------------------------------------------------------- |
| `Pixel_Tablet` (Android 17, 5580)          | 10, 0 fallas   | **159**: 135 / 13 / 11; 72 corridas, 87 omitidas, 0 fallas | **28**: 15 corridas, 13 omitidas (`KitchenOnGoTest`), 0 fallas |
| `ADEL_A1_Tablet_API26` (Android 8.0, 5582) | 10, 0 fallas   | **159**: 70 corridas, 89 omitidas, 0 fallas                | **28**: 15 corridas, 13 omitidas, 0 fallas                     |
| `Pixel_8` (Android 17, 5586)               | 10, 0 fallas   | **159**: 72 corridas, 87 omitidas, 0 fallas                | — (la de cocina es para tabletas)                              |

Son los 168 casos de la línea base de la Fase B (158 de meseros y 10 de `android-core`) más `e1` de la prueba de punta
a punta. En `Pixel_Tablet` también se corrieron sobre `4debb8c`, con los mismos números.

**De punta a punta contra el Go temporal** (`scripts/android-e2e.sh`):

| Suite                                                                                                                                         | `Pixel_Tablet` (Android 17, 5580) | `ADEL_A1_Tablet_API26` (Android 8.0, 5582)                                                                                     | `Pixel_8` (Android 17, 5586)   |
| --------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ | ------------------------------ |
| `--suite all` de meseros (access 1–5 y 6–7, catalog, waiter, field, takeaway, precheck, alerts, adjustments y license; un Go nuevo por parte) | **84/84** de un tirón (30 min)    | **83/84**, por partes (instalando con `adb install -r -t` y `--no-install`, A3-D33): todo aprobado salvo `takeaway t6` (abajo) | **84/84** de un tirón (29 min) |
| `--suite kitchen` (aplicación de cocina, `k1`–`k13` sin `k12` y los dos conteos de `ready_notices`)                                           | **14/14** (303 s)                 | **14/14** (297 s), instalando con `adb`                                                                                        | —                              |
| `--suite endtoend` (§8: las dos aplicaciones en la misma tableta)                                                                             | **4/4** (48 s)                    | **4/4** (28 s), instalando con `adb`                                                                                           | —                              |

- **`kitchen`**:
  - `k7`, cinco pausas del Go de 1 a 5 s, ningún aviso;
  - `k8`, la marca hecha durante un corte de 18 s se repite y Go la registra una vez;
  - `k9`, la pausa de 45 s muestra el aviso, no deja marcar y se quita sola;
  - `k10`, los cortes de red (modo avión en Android 17; wifi y datos con root en Android 8). Android vio sin red 2,9,
    4,0, 6,1 y 36,1 s en `Pixel_Tablet` y 1,3, 3,3, 5,3 y 35,3 s en Android 8. El aviso salió solo en el largo, a los
    10 s del margen de K1-D33 en los dos;
  - `k11`, los ajustes con la contraseña del administrador;
  - `k13`, «Volver a emparejar» suelta el cupo.
- **`endtoend`**: Ana envía 2 Filete R a la Mesa R4 desde la aplicación de meseros, la pantalla de cocina ve la tarjeta
  «Mesa R4» y la marca lista (`k12`), y a Ana le llega «Mesa R4 · 2 platillos listos / 2 × Filete R» (`e1`).
- **`takeaway t6` en Android 8**: el pendiente conocido A3-D32 (L1-D48). Falló dos veces, en `5582-takeaway-1` y
  `-3`, en el mismo lugar que en la A3 y la L1: `TakeawayOnGoTest.kt:308`, `waitForText("Luisa Martínez")` a los 40 s.
  - La condición anterior sí se cumple: el nombre llegó a Go, sin teléfono, y la cola de ediciones quedó vacía.
  - Hubo 0 rechazos de licencia, cupo u hora en el logcat del paso y en su Go.
  - En los otros dos emuladores aprobó.
  - Una repetición intermedia (`-2`) se descartó: la instrumentación de `t2` se quedó colgada 11 minutos sin
    responder (problema del emulador, con la aplicación ya cerrada). Se detuvieron sus procesos y se repitió la suite.
- **Las líneas «se repite» (A3-D26)**: ninguna, en la salida de las corridas.
- **La carga**: un emulador a la vez. Desde `waiter` de Android 8 se compilaban a la vez los APK y el `.pkg` de entrega,
  con prioridad baja (`taskpolicy -b`); la carga de la Mac llegó a 16 y las partes aprobaron igual.

**La actualización encima de la 0.4.0 a la 0.4.1** (K1-D28), en `Pixel_Tablet`:

- **Parte 1, los datos**, con las compilaciones debug:
  - la 0.4.0 (de `45fa9b2`) empareja con un Go temporal nuevo, entra Ana y deja un pedido «Por enviar» (`waiter w1`
    y `w3`, aprobadas);
  - se instala **encima** la 0.4.1 de `43b2bd4` con `adb install -r`, sin desinstalar: la base local sigue igual (el
    servidor emparejado, Ana y el pedido `queued`), `versionCode` 4 → 5 y la misma `firstInstallTime`;
  - sin volver a entrar, `w4` aprueba: el pedido sale una sola vez (`received`).
- **Parte 2, la firma**:
  - el APK 0.4.0 entregado (`9ed2733f…`) y la release 0.4.1 de `43b2bd4` tienen el mismo certificado
    (`fb6a4bac…7c86`);
  - `adb install -r` de la 0.4.1 encima de la 0.4.0: «Success», `versionCode` 4 → 5, misma `firstInstallTime`;
  - la release no se abre (K1-D28);
  - la release de entrega, compilada después sobre `9b9f472`, tiene **el mismo contenido** archivo por archivo (los
    mismos CRC); cambian solo la firma y los metadatos del zip.

**Rendimiento** (`field f2`, registro `AdelOnGo`), parecido a la L1.

| Medida                                            | `ADEL_A1_Tablet_API26` | `Pixel_Tablet` | `Pixel_8` |
| ------------------------------------------------- | ---------------------- | -------------- | --------- |
| Arranque en frío (proceso → lista de meseros)     | 1 426 ms               | 2 553 ms       | 2 798 ms  |
| PIN → Mesas con la primera mesa                   | 744 ms                 | 919 ms         | 923 ms    |
| PIN → la mesa 40 a la vista                       | 1 244 ms               | 1 594 ms       | 1 545 ms  |
| «Nueva cuenta» → catálogo a la vista              | 266 ms                 | 394 ms         | 337 ms    |
| «Enviar a cocina» → «Recibido por el restaurante» | 316 ms                 | 324 ms         | 326 ms    |
| Memoria (total de `dumpsys meminfo`)              | ≈ 87 MiB               | ≈ 147 MiB      | ≈ 148 MiB |

**Ningún servidor de licencias real.** Los 31 Go temporales de `all` y de las partes dicen `license_server_disabled` al
arrancar, y sus registros no nombran `sicrat`. Los de `kitchen`, `endtoend` y la actualización se levantaron igual (`iso.sh server-up` sin dirección del
servidor de licencias). Las licencias se importaron sin internet. Ninguna lista mostró un servidor real.

**Capturas de lo nuevo** (§0: solo las pantallas nuevas o cambiadas), revisadas una por una:

- **Terminales con una pantalla de cocina** (WebKit, tema claro): `k1-terminales-configuracion`, `-editar` (estaciones
  solo en pantalla, letra, tema, «Sonido» con «Activado» y «Desactivado», volumen y el texto de los cupos de K1-D35),
  `-liberar-cupo` (con el texto de K1-D30) y `k1-cocina-colores` (los límites de color de las pantallas; la prueba
  guarda 12 y 25 minutos).
- **El escritorio**: `k1-reconexion-sin-conexion` y `k1-reconexion-terminal-red` (la capa «Sin conexión con el
  servidor» solo en la pausa larga de 42 s). Que no aparezca «Reconectando» sin uso ni en las pausas de 1 a 5 s lo
  comprueban las especificaciones, que exigen «En línea» en cada lectura, y la prueba de 10 minutos.
- **La aplicación de cocina**, en las dos tabletas (14 capturas de cada una): las tarjetas en los dos temas y las dos
  letras (`c8-dark-normal`, `-dark-large`, `-light-normal`, `-light-large`), los colores por tiempo, lo marcado,
  Recientes, muchos pedidos, la hora incorrecta, sin cupo, cupo liberado, sin activar, «Sin conexión», la tarjeta nueva,
  la contraseña, los ajustes, «Volver a emparejar» y las pantallas de la entrada.
- **Lo que se encontró**: en `b4-actualizar-el-servidor`, la tableta de cocina decía «no sirve para esta versión de la
  aplicación de meseros… (necesita la versión 0.13.0 o posterior)» con un servidor 0.15.0. Se corrigió (abajo, «Lo
  corregido») y la captura nueva dice «necesita la versión 0.16.0 o posterior» sin nombrar a meseros. El resto, sin
  textos provisionales, identificadores ni mensajes técnicos a la vista.

### Lo corregido en la regresión

| Commit    | Qué                                                                                                                                                                                                                                                                                                                                                         | Por qué                                                                                                                                                                                                                                                             | Qué se repitió                                                                                                                                                                                                                          |
| --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `dd532f8` | La prueba de Vitest «con el aviso activado, el primer toque o tecla prepara el sonido…» (revisión 9F, `preparationBoard.test.tsx`) ahora toca hasta que el efecto escucha el primer toque y después cuenta los dos gestos                                                                                                                                   | Con la Mac cargada falló una vez: las tarjetas ya se veían, pero React todavía no había corrido el efecto que escucha el primer toque. Con el registro del efecto retrasado a propósito, la prueba anterior falla igual y la nueva aprueba. No cambia la aplicación | El archivo, 3 veces; Vitest completo en la copia final                                                                                                                                                                                  |
| `43b2bd4` | «Actualiza el servidor del restaurante» no nombra ninguna aplicación y recibe la versión mínima de cada una (`requiredServerVersion` en `UpdateServerScreen`, `FindServerRoute` y `pairingProblemText` de `android-core`): meseros pasa 0.13.0 y cocina 0.16.0. `KitchenScreensTest` (b4) exige «necesita la versión 0.16.0 o posterior» y ningún «meseros» | Defecto de la regla de textos: la tableta de cocina decía «no sirve para esta versión de la aplicación de meseros… (necesita la versión 0.13.0 o posterior)», aunque el servidor fuera 0.15.0. El texto compartido se movió de meseros en la Fase B sin cambiar     | Toca `android-core`, que comparten las dos aplicaciones: toda la regresión de Android corrió sobre `43b2bd4` (JVM, Lint, ktfmt, instrumentadas, capturas, las suites y la actualización encima). El servidor y el escritorio no cambian |

Ningún hallazgo obligó a cambiar una decisión de la especificación ni del avance.

### Lo que corrigió o repitió el coordinador

- **`reports/operational` fuera de su franja** (K1-D40): el coordinador corrió
  `ISO_TAG=F $SP/iso.sh $SP/wt/k1-f go-int ./internal/reports/operational` a las 05:02 del 25/11/2026 sobre
  `2e81ff1`, que tiene el mismo código que el final. La prueba aprobó entera en 42 s, incluidas las 4 subpruebas que
  habían fallado a las 00:00, así que la integración queda en 17 de 17.
- **Entregables**: el coordinador volvió a calcular los tres SHA-256 en el repositorio y en las copias, y comprobó:
  - la firma de los dos APK (`fb6a4bac…7c86`);
  - en los dos APK, `minSdk` 26 y `targetSdk` 36, con sus paquetes, versiones y nombres;
  - que el `.pkg` va sin firma de instalador, como el 0.15.0.

## No verificado

- **Docker real del lector**: Docker Desktop estaba apagado.
- **Dispositivos físicos, wifi real y la aplicación real de Tauri**: Android solo en emuladores y el escritorio con la
  vista previa y el puente simulado. El «Reconectando» real lo cubren la prueba de Rust de 10 minutos contra un Go
  temporal (local y por la red) y Playwright; el ámbar «Reconectando» (canal caído con Go respondiendo) solo en Vitest.
- **El `.pkg` 0.16.0 instalado**: instalarlo es la parada del propietario (§0 de la especificación).
- **`ADEL_A1_Phone_API33`** (`emulator-5554`, del propietario) y la aplicación de cocina en el teléfono: la de cocina es
  para tabletas en horizontal (§1) y se probó en las dos tabletas.
- **La suite `connection`** de meseros (la hora sin uso, fuera de `all`).
- **La parte 2 de la actualización encima no abre la release** (K1-D28): abrirla sin emparejar empezaría la búsqueda en
  la red real; que la release abra con lo emparejado se infiere de la parte 1 (mismo código y mismos datos) y de la
  firma.
- **Windows**: solo `GOOS=windows` para Go.
- **`scripts/browser-tests.py` tal cual** (`dev.sh test-browser`): usa `.local/server.json` y el clúster 55432, que la
  etapa no toca; la regresión usó el arnés aislado, que hace lo mismo.
- **Limitaciones aceptadas**: con la computadora principal colgada, el aviso de la tableta tarda unos 41 s (K1-D37);
  Go manda como mucho 200 tarjetas (K1-D31); «Liberar cupo» no es definitivo (K1-D30); si «Volver a emparejar» no
  llega a Go, la pantalla vieja sigue ocupando su cupo hasta revocarla (K1-D32).

## Entregables

| Archivo                                                                  | Bytes      | SHA-256                                                            | Qué es                                                                                                                      |
| ------------------------------------------------------------------------ | ---------- | ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------- |
| `desktop/src-tauri/target/release/bundle/macos-pkg/AD-EL POS_0.16.0.pkg` | 23 957 122 | `add1e9e7e9fb46cdc6649cfc3edc7e692ecabea989d66339a775faf5307d3f47` | Aplicación y servidor 0.16.0 (esquema 29), **versión de desarrollo** (confía en el entorno `test`); firma ad hoc verificada |
| `android/app/build/outputs/apk/release/AD-EL-POS-Android-0.4.1.apk`      | 2 674 583  | `d783157306448bcde7368285e9fa6121fadea01e1720e3c75464099d1c9e8cfd` | `com.adelpos.meseros` 0.4.1 (`versionCode` 5), firmado con la llave de desarrollo                                           |
| `cocina/app/build/outputs/apk/release/AD-EL-POS-Cocina-0.1.0.apk`        | 1 878 017  | `36bb9822660cda64a2fc37f0941d79d7f68ce4cfae97bcc8eb556b91f44ae039` | `com.adelpos.cocina` 0.1.0 (`versionCode` 1), firmado con la misma llave                                                    |

Los tres se compilaron sobre una **copia exacta de `9b9f472`** (`git archive`, 2 085 archivos, los mismos que
`git ls-tree`), con `CARGO_NET_OFFLINE=true`, `GOFLAGS=-mod=readonly` y `GOPROXY=off`, nunca con `gradle clean` en
el repositorio. De `43b2bd4` (la copia de Android) a `9b9f472` solo cambió Markdown, y ningún binario embebe
documentos. Después cambiaron `CLAUDE.md` y este documento. Copias también en `$SP/` con el mismo SHA-256.

**El `.pkg` 0.16.0** (`./scripts/dev.sh build-macos-pkg`, sin instalar nada: ni `sudo`, ni `installer`, ni `launchctl`):

- **`pkgutil --expand-full`**:
  - los tres paquetes son 0.16.0: `local.adel.pos.app`, `.server` y `.service`;
  - la distribución ofrece «Servidor del restaurante» marcada por omisión (`start_selected="true"`), el servicio
    oculto y la arquitectura `arm64`;
  - la aplicación de dentro es 0.16.0 (`local.adel.pos.dev`), arm64, con `ad-el-pos-desktop` y `ad-el-pos-server` en
    `Contents/MacOS`;
  - el servidor de dentro dice `0.16.0` con `version`, lleva `"schema_version": 29` y la migración
    `0029_pantalla_de_cocina.sql` embebida;
  - los scripts (`preinstall`, `postinstall`, `common.sh` y el `LaunchDaemon`) son idénticos a los de la copia.
- **`codesign --verify --deep --strict --verbose=2`** de la aplicación de dentro: «valid on disk» y «satisfies its
  Designated Requirement», firma `adhoc`. El `.pkg` no tiene firma de desarrollador, como dice el script.
- **Confía en el entorno `test`**: `go version -m` del servidor no muestra `-tags` (no es la de `adelpos_production`),
  y el binario lleva el `kid` `adelpos-test-202611-1` y el de soporte de desarrollo
  (`adelpos-soporte-development-20261123`).
- **No lleva lo que no debe**:
  - ningún `kid` ni semilla de `vectors.json`;
  - ningún archivo `vectors.json`, `.lic`, `.licreq`, `.pem` ni `.key`, ni `license-test` ni `soporte-desbloqueo`;
  - ningún bloque `-----BEGIN … PRIVATE KEY-----`. La cadena `PRIVATE KEY` solo aparece como nombre de tipo PEM de
    las bibliotecas de Go y de Rust.
- **Los anteriores no cambiaron**: los `.pkg` de 0.10.0 a 0.15.0 siguen con el mismo SHA-256 (el de la 0.15.0,
  `2ef89832…669b`).

Instalarlo es la parada del propietario.

**Los APK** (`:app:assembleRelease` con `--no-build-cache`):

- **`apksigner verify`**: esquema v2, un firmante, «CN=AD-EL POS desarrollo - APK de prueba, O=AD-EL POS», SHA-256
  `fb6a4bac93052ca9051342ea122ed90469340a3685de14243bdab288d71c7c86` en los dos (la misma llave de la 0.2.0 a la 0.4.0).
- **`aapt2 dump badging`**:
  - meseros: `com.adelpos.meseros` 0.4.1 (5), «AD-EL POS»;
  - cocina: `com.adelpos.cocina` 0.1.0 (1), «AD-EL POS Cocina», con su propio ícono;
  - los dos con `minSdk` 26, `targetSdk` 36 y `compileSdk` 37.
- **La actualización encima** de la 0.4.0 a la 0.4.1, en `Pixel_Tablet`: sección «Android».
- **Los anteriores no cambiaron** (SHA-256 antes y después): la 0.4.0 (`9ed2733f…`), la 0.3.0 (`87477497…`), la 0.2.0
  (`38d687ce…`) y el `app-release.apk` del 18/11.

## Instalación real

La instalación real de la Mac está en **0.15.0** (la aplicación de `/Applications` dice 0.15.0; el servicio
`local.adel.pos.server` corre desde el 24/11): PostgreSQL en 55432 y Go en 8181 y 8443. Ninguna prueba de la etapa la
tocó; solo se leyó la versión de la aplicación, sin escribir nada.

**Se actualiza con el `.pkg` 0.16.0**, que instala el **propietario** con «Servidor del restaurante» marcada. Es una
parada, porque pide la contraseña de administrador. Al arrancar:

1. **El `host` respalda la base** (`…-antes-de-migrar.dump`) **y aplica la `0029`**, en una sola transacción:
   - `kitchen_settings`: los límites de color de las pantallas de cocina, `display_warning_minutes` (10) y
     `display_late_minutes` (20);
   - `devices`: los ajustes de cada pantalla de cocina (letra normal, tema oscuro, sonido activado y volumen medio por
     omisión) y su «cupo liberado», que solo puede tener una pantalla de cocina;
   - `license_seats`: el cupo `kitchen_display` puede quedar sin sesión, con un disparador que exige que sea de un
     dispositivo `kitchen`;
   - `kitchen_kds_receipts`: el acuse de recepción puede quedar sin usuario, también solo para un dispositivo `kitchen`;
   - la tabla `app.kitchen_display_dismissals` y dos índices de las lecturas de la pantalla.

   **No cambia ninguna fila**: ventas, pagos, cortes, cuentas, pedidos, sesiones, usuarios, terminales, estaciones,
   impresoras y licencia quedan igual. Las columnas nuevas toman su valor por omisión.

2. **La identidad de la red local no cambia**: las terminales de escritorio y los teléfonos y tabletas de meseros que ya
   están emparejados siguen emparejados. La licencia, si ya está activada, sigue igual (la 0.16.0 también confía en el
   entorno `test`).
3. **El escritorio deja de mostrar el «Reconectando» falso** cada 30 s: la corrección está en la aplicación (Rust y
   React), que llega con el mismo `.pkg`. Las terminales de escritorio por la red se actualizan con su propio `.pkg`.
4. **Terminales** muestra la versión instalada de cada terminal y dispositivo, que se actualiza sola la próxima vez que
   se conecta.

**Los teléfonos y tabletas de meseros** con la 0.4.0 se actualizan instalando encima el APK 0.4.1 (misma llave y mismo
identificador): no hace falta desemparejarlos ni borrar sus datos, y los pedidos por enviar salen solos. La 0.4.0
también sigue funcionando con el servidor 0.16.0.

**La tableta de cocina** se instala aparte con el APK de cocina 0.1.0, después de instalar el `.pkg` 0.16.0 (con una
versión anterior la tableta dice «Actualiza el servidor del restaurante… necesita la versión 0.16.0 o posterior»). Se
empareja con su código y se configura en **Administración › Sistema › Terminales**; ocupa un cupo de pantalla de cocina
de la licencia (la Perpetua con el plan elemental trae 3). Los pasos para el dueño:
[COCINA_INSTALAR.md](COCINA_INSTALAR.md).
