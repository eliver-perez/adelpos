# Verificación · Etapa M1 (unir mesas, mesas divisibles y ajustes pendientes)

Verificación por fases del **27 de noviembre de 2026**, en la Mac (Apple Silicon, macOS 26.5.2). Cada fase se hizo en
una rama local que el coordinador fusionó en `etapa-m1`. Esa rama se creó desde `main` (`a33ada0`: 0.17.0, esquema 30,
meseros 0.4.1 y cocina 0.1.0, con la K1.1 fusionada). La especificación está en `72ecadd`. Fusiones y avances de cada
fase:

- **Fase 0** (verificación, reproducción de los defectos de la §5, el importador con el respaldo real, las decisiones
  M1-D1 a M1-D19 y el contrato de la etapa): `3614024`.
- **Fase E** (las pruebas frágiles de Playwright, §5.8): fusión en `6fa6476`; avance en `8166b86`.
- **Fase A** (servidor: uniones, mesas divisibles, §5.2 y §5.3, la `0031` y el contrato): fusión en `c5b333a`; avance
  en `531773d`.
- **Fase R** (revisor independiente de la concurrencia de las uniones, §2.3): sus pruebas que aprueban, fusionadas en
  `f79faa3`; avance en `48f9a84`.
- **Fase C** (meseros 0.5.0: la tarjeta única, los ajustes de Android y unir mesas): fusión en `83b7720`; avance en
  `f925b89`.
- **Correcciones de la revisión** (Fase A, M1-D29 a M1-D33): fusión en `1a27885`; avance en `7c15d38`.
- **Fase C, segunda parte** (el título del teléfono, el teclado en Android 8, la 0.4.1 frente a una unión): fusión en
  `688e4ca`; avance en `28f8e86`.
- **Fase B** (escritorio: unir mesas, mesas divisibles, §5.1, §5.2 y §5.4): fusión en `cb38924`; avance en `17acd9a`.
- **Fase F** (rama `etapa-m1-f`, desde `17acd9a`): la versión en `d4963a7`, los documentos en `3d24aab` y `b90b8fd`, las
  tres pruebas corregidas en `0ad519b` y `1223606`, los resultados en `2d807c8`; fusión en `996ecc3`. Después, el
  avance y este documento.

**Servidor 0.18.0.** API v1, migración `0031_uniones_de_mesas.sql` (esquema **31**), una sola para toda la etapa y que
solo agrega:

- las mesas divisibles (`app.table_groups` y `app.table_group_sections`);
- las uniones (`app.table_unions`, `app.table_union_members`, con el índice que deja una mesa en una sola unión abierta,
  y el historial `app.table_union_events`);
- la unión en lo cerrado: tres columnas en `app.sales_recognitions` y dos en `app.account_cancellations` (M1-D30), en
  NULL en lo que ya existía;
- el permiso `restaurant.tables.join` («Unir mesas»), concedido a los roles Administrador, Mesero, Cajero y Supervisor
  que ya podían abrir cuentas y ver las mesas (M1-D8).

`min_client_versions` sin cambio, la capacidad nueva `table-unions-v1` y **ningún lock nuevo**. **Sigue siendo una
versión de desarrollo**, como la 0.17.0: confía solo en las llaves del entorno `test`
([licencias/PASO_A_PRODUCCION.md](licencias/PASO_A_PRODUCCION.md)).

**Meseros 0.5.0** (`versionCode` 6, Room 7): se instala encima de la 0.4.1 sin volver a emparejar, con la misma llave de
desarrollo. **Cocina sigue en 0.1.0**: `git diff --stat a33ada0 -- android-core cocina` está vacío en el commit final
(M1-D14 y M1-D40), así que no lleva APK nueva.

**Referencias.**

- Especificación: [etapas/ETAPA_M1_MESAS.md](etapas/ETAPA_M1_MESAS.md).
- Avance, con el detalle de cada fase y las decisiones M1-D1 a M1-D46: [ETAPA_M1_AVANCE.md](ETAPA_M1_AVANCE.md).
- Guía del propietario: [UNIR_MESAS.md](UNIR_MESAS.md). El importador: [MIGRAR_SISTEMA_ANTERIOR.md](MIGRAR_SISTEMA_ANTERIOR.md),
  «Mesas divisibles del sistema anterior». Cocina: [KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md), «Uniones de
  mesas, etiquetas por renglón y textos de Preparación · etapa M1». Contrato: [API_V1.md](API_V1.md). Android:
  [ANDROID_INSTALAR.md](ANDROID_INSTALAR.md) y [android/CLAUDE.md](../android/CLAUDE.md).
- Evidencia anterior: [etapa K1.1](VERIFICATION_STAGEK1_1.md).

Este documento registra **solo lo que se ejecutó**.

**Las pruebas no tocaron la instalación real**: ni el servicio `local.adel.pos.server`, ni `/Library/Application
Support/AD-EL POS`, ni `/Applications/AD-EL POS.app`, ni `.local` del repositorio, ni el Keychain, ni PostgreSQL en
55432, ni Go en 8181 o en la escucha de red 8443. Ninguna prueba usó `sudo`, `installer` ni `launchctl`, ni las órdenes
de `dev.sh` de la instalación. **La hora de la Mac nunca se cambió.** Los emuladores del propietario (consolas 5554 y 5556) no se tocaron.

**Ninguna prueba llamó a `licensing.sicrat.com`**, y los códigos de las licencias de prueba no se usaron ni se
escribieron en ningún lado: las pruebas usan solo `contracts/licencias/v1/fixtures/vectors.json` y el simulador
`license-test`. Los registros de los Go temporales de la regresión no nombran `sicrat`.

**Datos.** Las pruebas usan **datos sintéticos**, creados por cada prueba en una base desechable. La única lectura del
respaldo real fue la de la Fase 0, con `legacy-prepare` en una copia aislada y solo para contar las mesas que el
importador vuelve grupos; en el repositorio solo quedan esos conteos y los nombres de las cuatro secciones de la «Mesa 3».

## Entorno aislado

El de la K1 ([VERIFICATION_STAGEK1.md](VERIFICATION_STAGEK1.md), «Entorno aislado»), con estos cambios:

- **Como mucho dos agentes a la vez**, cada uno con su `ISO_TAG`, su árbol de trabajo y sus puertos (de 8211 a 8269
  para los Go temporales y Playwright, y de 8461 a 8511 para sus escuchas de red y los simuladores de licencias): A y C
  en paralelo, B y la segunda parte de C en paralelo; la Fase 0, el revisor y la regresión final, un solo agente.
- El clúster PostgreSQL 18 temporal en 55499, solo en loopback, con una base `adel_pos_test_*` desechable por paquete,
  por motor de Playwright y por Go temporal.
- `go-int ./cmd/ad-el-pos-server` usa puertos fijos: se corrió con un candado, uno a la vez.
- Vitest y Playwright sustituyen el puente nativo del escritorio por la simulación de `e2e/native-driver.ts`.
- **Emuladores de la etapa (M1-D37)**: mientras el propietario tenía abiertos sus `Pixel_Tablet` y `Pixel_8`, sus copias
  de solo lectura no arrancaban. Se crearon AVD propios con el mismo perfil y la misma imagen (Android 17,
  `google_apis_playstore`): `ADEL_M1_Tablet_API37` (consola 5570) y `ADEL_M1_Phone_API37` (5574), más
  `ADEL_A1_Tablet_API26` (Android 8.0, 5572) de la A1. Sin ventana, de solo lectura y como mucho dos a la vez.
- **Las corridas largas** fueron dentro de `caffeinate -i -s`, y antes de dar una falla por buena se revisaron
  `pmset -g log` y el bloqueo de la pantalla (la primera corrida de WebKit de la Fase B no valió: la Mac se durmió).

## Resultados ejecutados por fase

| Fase             | Lo que se ejecutó y su resultado                                                                                                                                                                                                                                                             |
| ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 0                | Reproducción de cada defecto de la §5 contra un Go temporal y en los emuladores, con su causa; el importador con el respaldo real (solo conteos): un grupo «Mesa 3» de 4 secciones y 0 dudosos                                                                                               |
| E                | Las tres fragilidades de §5.8 (la cascada, la sesión reemplazada y los avisos de la licencia cerca de la medianoche; M1-D20 a M1-D23), con las especificaciones afectadas en Chromium y WebKit                                                                                               |
| A                | `go-unit`; `go-int` de `operations` (con `TestStageM1*` y `TestStageM1Concurrency`), `payments`, `kitchen`, `reports`, `database` (`TestStageM1Upgrade`, 30 → 31), `apicontract` (`TestContractV1`), `administration`, `legacyimport` y `./cmd/ad-el-pos-server`; `fmt-check` y `sync-check` |
| R                | Revisión independiente de la concurrencia: `TestStageM1ReviewConcurrency` y `TestStageM1ReviewDeferred` aprueban y se fusionaron; los hallazgos R1 a R3 y D-R1 y D-R2 pasaron a M1-D29 a M1-D33                                                                                              |
| A (correcciones) | Las dos del revisor con `-count=10` y `-race`, `TestStageM1ReviewClockOrder`, `TestStageM1Concurrency` con `-count=5`, el experimento de 40 rondas sin forzar (**0 tickets distintos de la venta**, no confirmado) y la integración completa de los paquetes que tocó                        |
| C                | JVM, `ktfmtCheck` y lint; las instrumentadas con `StageM1ScreensTest`; las suites de punta a punta `waiter`, `takeaway` (con `t6` en Android 8), `adjustments` y la nueva `unions` en las dos tabletas                                                                                       |
| C, segunda parte | `unions` con el caso de M1-D32 de punta a punta en las dos tabletas, `field` y `precheck`, el título del teléfono en vertical, horizontal y a 360 dp, el teclado de los diálogos en Android 8, y la 0.4.1 frente a una unión con la 0.5.0 encima (suite `legacy`)                            |
| B                | `cargo` 123; Vitest 1 075; `build`; Playwright completo 58 de 58 en Chromium y 58 de 58 en WebKit (segunda corrida)                                                                                                                                                                          |
| F                | La regresión final (abajo)                                                                                                                                                                                                                                                                   |

En cada fase se corrieron además `format` y `fmt-check`, y el MD5 de la especificación (`4750ae46…`) quedó igual antes y
después.

## Regresión final

> **Resumen** (rama `etapa-m1-f`, con el código de `d4963a7`; después solo cambiaron Markdown y tres pruebas):
>
> - Go aprobó a la primera: `go-unit` y la integración de los 16 paquetes de `scripts/database.py`, con
>   `./cmd/ad-el-pos-server` con el candado;
> - `cargo` 123, Vitest 1 075, `build`, `fmt-check` y `sync-check`;
> - **Playwright 58 de 58 en Chromium a la primera y 57 de 58 en WebKit**: la que falló era de la prueba (abajo) y
>   aprobó corregida;
> - Android: JVM 93, 350 y 56, `ktfmtCheck` y lint; las instrumentadas y **todas las suites de punta a punta** en
>   las dos tabletas y las del teléfono en el teléfono, con la actualización 0.4.1 → 0.5.0 firmada;
> - **ninguna corrección del producto**: las tres correcciones son de pruebas.

Una sola vez (§7), en la rama `etapa-m1-f`, el 27/11/2026 de 18:28 a 21:05 (hora de Monterrey). Un solo agente, sin
otro trabajando a la vez. Go 1.27.3, Playwright 1.65.0, PostgreSQL 18, macOS 26.5.2. Las corridas largas fueron
dentro de `caffeinate -i -s`; la Mac no se durmió ni tuvo la pantalla bloqueada (`pmset -g log` y `ioreg`).

- **El commit de la versión**, `d4963a7`: `config/product.json` en **0.18.0** con la sincronía (los metadatos, la copia
  generada, `package.json`, `tauri.conf.json`, `Cargo.toml` y los dos lockfile), `schema_version` 31 (de la Fase A) y
  `min_client_versions` sin cambio. Meseros sigue en 0.5.0 (6) y cocina en 0.1.0 (1).
- **Los puertos**: Playwright con su Go temporal en 8261, la vista previa en 1451, la base limpia en 8262, el Go sin
  activar en 8264 y el simulador de licencias en 8494; el Go temporal de Android, uno nuevo por suite, en 8263 y 8495.

### Servidor y escritorio

| Prueba                                                                                                                                                                                                             | Resultado                                           | Tiempo   |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------- | -------- |
| `go-unit` (vet normal e `integration`, `test -race` de 40 paquetes, `GOOS=windows` build y vet)                                                                                                                    | aprobado                                            | 69 s     |
| `go-int` de los 16 paquetes de la tupla de `scripts/database.py`, uno por uno (`operations` con `TestStageM1*`, `TestStageM1Concurrency` y las del revisor; `reports/operational` a las 19:00, lejos de las 05:00) | **16 de 16** a la primera                           | ≈ 31 min |
| `go-int ./cmd/ad-el-pos-server` con el candado                                                                                                                                                                     | aprobado                                            | 79 s     |
| `fmt-check` (con la comprobación de datos reales) y `sync-check`                                                                                                                                                   | aprobados                                           | —        |
| Vitest (con `tsc --noEmit`)                                                                                                                                                                                        | **112 archivos, 1 075 pruebas**, 0 fallas           | 30 s     |
| `npm run build`                                                                                                                                                                                                    | aprobado                                            | 11 s     |
| `cargo test` (con `stage_m1_table_union_routes_are_closed`)                                                                                                                                                        | **123** aprobadas, 4 ignoradas                      | 37 s     |
| **Playwright Chromium**, suite completa con los cuatro servidores                                                                                                                                                  | **58 de 58** a la primera                           | 27,5 min |
| **Playwright WebKit**, suite completa con los cuatro servidores                                                                                                                                                    | **57 de 58**: falló `workflow-stagez-k11-assistant` | 27,5 min |
| Las dos de la K1.1 solas después de `0ad519b`, en WebKit y en Chromium; la del asistente tres veces seguidas en WebKit                                                                                             | 2 de 2, 2 de 2 y 3 de 3                             | —        |

### Android

En la Mac: `ktfmtCheck` de los tres proyectos; JVM de `android-core` **93**, de meseros **350** y de cocina **56**, con
0 fallas y lint aprobado. Las APK de entrega de prueba (la 0.4.1 de una copia `git archive` de `main` y la 0.5.0 de la
rama) con la llave de desarrollo: las dos con la huella `fb6a4bac…7c86`.

**Instrumentadas** (`connectedDebugAndroidTest`; las pruebas contra Go se omiten aquí porque corren con el guion):

| Proyecto       | 5570 · tableta Android 17                                | 5572 · tableta Android 8    | 5574 · teléfono Android 17                                                     |
| -------------- | -------------------------------------------------------- | --------------------------- | ------------------------------------------------------------------------------ |
| `android-core` | 10, 0 fallas                                             | 10, 0 fallas                | 10, 0 fallas                                                                   |
| Meseros        | 178 (77 corridas), 0 fallas; `StageM1ScreensTest` 4 de 4 | 178 (75 corridas), 0 fallas | 178 (77 corridas), 1 falla de la prueba; corregida, `AccessScreensTest` 5 de 5 |
| Cocina         | 28 (15 corridas), 0 fallas                               | 28 (15 corridas), 0 fallas  | — (es de tableta)                                                              |

**De punta a punta** (`scripts/android-e2e.sh`, cada parte de `all` con un Go temporal nuevo e instalando con adb, más
las suites que no entran en `all`):

| Suite                                                                 | 5570 · tableta Android 17 | 5572 · tableta Android 8 | 5574 · teléfono Android 17                           |
| --------------------------------------------------------------------- | ------------------------- | ------------------------ | ---------------------------------------------------- |
| `access`                                                              | 5 de 5 y 2 de 2           | 5 de 5 y 2 de 2          | 5 de 5 y 2 de 2                                      |
| `catalog`                                                             | 3 de 3                    | 3 de 3                   | 3 de 3                                               |
| `waiter`                                                              | 19 de 19                  | 19 de 19                 | 19 de 19                                             |
| `field` (con f7)                                                      | 13 de 13                  | 13 de 13                 | 13 de 13                                             |
| `takeaway`                                                            | 9 de 9                    | **9 de 9 con `t6`**      | 9 de 9                                               |
| `precheck`                                                            | 7 de 7                    | 7 de 7                   | 7 de 7                                               |
| `alerts`                                                              | 11 de 11                  | 11 de 11                 | 11 de 11                                             |
| `adjustments`                                                         | 7 de 7                    | 7 de 7                   | 7 de 7                                               |
| `license`                                                             | 8 de 8                    | 8 de 8                   | 8 de 8                                               |
| **`unions`**                                                          | 12 de 12                  | 12 de 12                 | 11 de 12 (u2, de la prueba); corregida, **12 de 12** |
| **`legacy`** (v1 con la 0.4.1, v2 con la 0.5.0 encima)                | 1 de 1 y 1 de 1           | 1 de 1 y 1 de 1          | —                                                    |
| `kitchen` (aplicación de cocina)                                      | 14 de 14                  | 14 de 14                 | — (es de tableta)                                    |
| `endtoend`                                                            | 4 de 4                    | 4 de 4                   | —                                                    |
| `connection` (10 minutos sin uso)                                     | 1 de 1: 0 avisos falsos   | 1 de 1: 0 avisos falsos  | —                                                    |
| `AccessScreensTest` y `unions` otra vez, con las pruebas de `1223606` | 5 de 5 y 12 de 12         | 12 de 12                 | 5 de 5 y 12 de 12                                    |
| El título del teléfono en vertical, horizontal y a 360 dp             | —                         | —                        | sin cortar                                           |
| **Actualización 0.4.1 → 0.5.0** (datos y firma)                       | aprobada                  | aprobada                 | —                                                    |

- **La actualización**: `versionCode` 5 → 6 con la misma `firstInstallTime`, sin volver a emparejar; el pedido que
  estaba en la cola salió una sola vez. Con las APK de entrega, «Success» y la misma huella.
- **La 0.4.1 frente a una unión** (`legacy` v1): Go le responde que ya hay una cuenta en la mesa y, con «Abrir otra
  cuenta», el pedido entra en una cuenta nueva **dentro** de la unión «Mesa 21 + 22». Con la 0.5.0 encima, la unión es
  una sola entrada con sus dos cuentas.
- **M1-D41**: «En línea» con el modo avión puesto al abrir la aplicación es el arranque que decidió la A3 (el estado
  duda hasta 20 s si el proceso nace sin red); con la aplicación abierta, el modo avión dice «Sin conexión» al momento.
  No se corrige en esta etapa (M1-D45). El diálogo de cancelar se cerró entre 175 y 531 ms.

### Lo corregido en la regresión

1. **Playwright WebKit, `workflow-stagez-k11-assistant`**: la prueba revisaba el selector de impresora antes de que
   llegara la lista de impresoras de cocina. Era de la prueba; `0ad519b` espera el selector o el botón habilitado.
2. **El teléfono**, dos pruebas que no se habían corrido ahí: `AccessScreensTest` buscaba el nombre del mesero, que en
   el teléfono vertical va solo con su inicial desde la §5.6 (M1-D34), y `unions` u2 tocaba una pestaña de área tapada
   por el panel de la mesa. `1223606` corrige las dos pruebas; la aplicación no cambia.
3. **Del emulador, no de la aplicación**: `ADEL_M1_Tablet_API37` se cerró solo a mitad de `kitchen` con la Mac muy
   cargada, y en Android 8 dos instalaciones con adb no respondieron. Se repitió lo que faltaba (instalando sin
   «streamed» en Android 8) y aprobó todo.

## No verificado

- **Windows**: nada, salvo `GOOS=windows` build y vet de Go.
- **La aplicación real de Tauri** y **dos escritorios a la vez sobre la misma unión**: Playwright usa la vista previa con
  el puente; la carrera la cubren `TestStageM1Concurrency` y las pruebas del revisor.
- **Dispositivos físicos, wifi real, papel** (la comanda con «Salón · Mesa 5 + 6») y una tableta de cocina real.
- **`ADEL_A1_Phone_API33`**: no se usó; el teléfono de la regresión es `ADEL_M1_Phone_API37`.
- **El `.pkg` 0.18.0 instalado**: instalarlo es la parada del propietario.
- **Limitaciones aceptadas**:
  - una cuenta reabierta conserva el nombre de la unión con el que se cerró aunque la unión ya se haya deshecho
    (M1-D27);
  - «Modificaciones de cantidades» deduce la unión por la hora, así que en una carrera de milisegundos puede decir la
    mesa sola (M1-D39);
  - «Quitar datos de prueba» no quita uniones ni grupos (M1-D26);
  - los textos de Cocina fuera de Preparación quedan para una etapa siguiente (M1-D43);
  - «En línea» al abrir la aplicación sin red (M1-D45).

## Entregables

| Archivo                                                                  | Bytes      | SHA-256                                                            | Qué es                                                                                                                                            |
| ------------------------------------------------------------------------ | ---------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `desktop/src-tauri/target/release/bundle/macos-pkg/AD-EL POS_0.18.0.pkg` | 24 144 578 | `f87c6eeded0404e248ed2e86cbca46378cd083112d35e5f14bafeb81a0a0a446` | Aplicación y servidor 0.18.0 (esquema 31), **versión de desarrollo** (confía en el entorno `test`); firma ad hoc verificada                       |
| `android/app/build/outputs/apk/release/AD-EL-POS-Android-0.5.0.apk`      | 2 734 647  | `ed650050ed68ff6a17056862ec596dbafe98318e2dc45a4acf3d4ad3948bd5f4` | `com.adelpos.meseros` 0.5.0 (`versionCode` 6, `minSdk` 26), firmado con la llave de desarrollo (`fb6a4bac…7c86`, la misma de la 0.2.0 a la 0.4.1) |

Sin APK nueva de cocina: sigue la 0.1.0 de la K1 (`36bb9822…e039`).

Los dos se compilaron sobre una **copia exacta de `996ecc3`** (`git archive`, 2 157 archivos, los mismos que
`git ls-tree`), con `.tools`, `.cache/go-mod` y `desktop/node_modules` enlazados, `CARGO_NET_OFFLINE=true`,
`GOFLAGS=-mod=readonly`, `GOPROXY=off` y Gradle `--offline`, nunca con `gradle clean` en el repositorio. Después de
`996ecc3` solo cambió Markdown (el avance y este documento), y ningún binario embebe documentos. Copias también en el
directorio temporal de la sesión, con el mismo SHA-256.

**El `.pkg` 0.18.0** (`./scripts/dev.sh build-macos-pkg`, en 142 s, sin instalar nada: ni `sudo`, ni `installer`, ni
`launchctl`):

- **`pkgutil --expand-full`**:
  - los tres paquetes son 0.18.0: `local.adel.pos.app`, `.server` y `.service`;
  - la distribución ofrece «Servidor del restaurante» marcada por omisión (`start_selected="true"`), el servicio
    oculto y la arquitectura `arm64`;
  - la aplicación de dentro es 0.18.0 (`local.adel.pos.dev`), arm64, con `ad-el-pos-desktop` y `ad-el-pos-server` en
    `Contents/MacOS`;
  - el servidor de dentro dice `0.18.0` con `version`, lleva `"schema_version": 31` y la migración
    `0031_uniones_de_mesas.sql` embebida;
  - los scripts (`preinstall`, `postinstall` y `common.sh` de los tres paquetes) son idénticos a los de la copia.
- **`codesign --verify --deep --strict --verbose=2`** de la aplicación de dentro: «valid on disk» y «satisfies its
  Designated Requirement», firma `adhoc`. El `.pkg` no tiene firma de instalador («no signature»), como dice el script.
- **Confía en el entorno `test`**: `go version -m` del servidor no muestra `-tags` (no es la de `adelpos_production`),
  y el binario lleva el `kid` `adelpos-test-202611-1` y el de soporte de desarrollo
  (`adelpos-soporte-development-20261123`).
- **No lleva lo que no debe**:
  - ninguno de los 20 identificadores, semillas y llaves de `vectors.json` buscados en todos los archivos expandidos;
  - ningún archivo `vectors.json`, `.lic`, `.licreq`, `.pem` ni `.key`, ni `license-test` ni `soporte-desbloqueo`;
  - ningún bloque `-----BEGIN … PRIVATE KEY-----`.
- **Los anteriores no cambiaron**: los 14 archivos de entrega de las etapas anteriores (los `.pkg` de 0.10.0 a 0.17.0,
  las APK de meseros de 0.2.0 a 0.4.1, el `app-release.apk` del 18/11 y la de cocina 0.1.0) siguen con el mismo
  SHA-256 antes y después (el de la 0.17.0, `14c23297…6d7f`).

Instalarlo es la parada del propietario.

**La APK 0.5.0** (`:app:assembleRelease` con `--offline` y `--no-build-cache`, en 97 s, firmada con las propiedades de
`~/.android/adel-pos-dev/`):

- **`apksigner verify`**: esquema v2, un firmante, «CN=AD-EL POS desarrollo - APK de prueba, O=AD-EL POS», SHA-256
  `fb6a4bac93052ca9051342ea122ed90469340a3685de14243bdab288d71c7c86`.
- **`aapt2 dump badging`**: `com.adelpos.meseros` 0.5.0 (6), «AD-EL POS», `minSdk` 26, `targetSdk` 36 y `compileSdk` 37.
- **La actualización encima de la 0.4.1** la probó la Fase F en las dos tabletas con una APK de entrega de su rama
  (sección «Android»). Del código de la aplicación de esa rama a `996ecc3` solo cambiaron pruebas instrumentadas, que no
  entran en la APK.

## Instalación real

La instalación real de la Mac está en **0.17.0** (la aplicación de `/Applications` dice 0.17.0): PostgreSQL en 55432 y
Go en 8181 y 8443. Ninguna prueba de la etapa la tocó.

**Se actualiza con el `.pkg` 0.18.0**, que instala el **propietario** con «Servidor del restaurante» marcada. Es una
parada, porque pide la contraseña de administrador. Al arrancar:

1. **El `host` respalda la base** (`…-antes-de-migrar.dump`) **y aplica la `0031`**, en una sola transacción. Según el
   código de la migración y `TestStageM1Upgrade` (que la aplica sobre una base en el esquema 30):
   - crea las tablas de uniones y de mesas divisibles, vacías: ninguna mesa queda unida ni agrupada;
   - agrega las columnas de la unión a las ventas y a las cancelaciones, en NULL en todo lo que ya existía, así que los
     reportes y los documentos de lo ya cerrado siguen diciendo la mesa de siempre;
   - da el permiso «Unir mesas» a los roles Administrador, Mesero, Cajero y Supervisor que ya podían abrir cuentas y ver
     las mesas; los roles creados por el propietario no cambian;
   - **nada de ventas, pagos, cuentas, partidas, cortes ni inventario cambia**.
2. **La identidad de la red local no cambia**: las terminales de escritorio, los teléfonos y tabletas de meseros y la
   tableta de cocina que ya están emparejados siguen emparejados. La licencia, si ya está activada, sigue igual (la
   0.18.0 también confía en el entorno `test`).
3. **Desde ese momento** se pueden unir mesas en la Mac ([UNIR_MESAS.md](UNIR_MESAS.md)). Los teléfonos y tabletas con
   la 0.4.1 siguen funcionando: ven cada mesa por separado y, si capturan en una mesa unida, el pedido queda dentro de la
   unión sin perder nada. Se actualizan instalando encima la APK 0.5.0 (misma llave y mismo identificador), sin
   desemparejarlos ni borrar sus datos. La tableta de cocina no cambia.
4. **Si se importa otra vez el sistema anterior**, la importación arma la «Mesa 3» con cuatro secciones; si la cuarta no
   es parte de la mesa en U, se quita del grupo en «Editar» ([MIGRAR_SISTEMA_ANTERIOR.md](MIGRAR_SISTEMA_ANTERIOR.md)).
