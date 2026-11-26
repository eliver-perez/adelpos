# Verificación · Etapa K1.1 (pedidos mixtos, productos sin ruta y configuración guiada de cocina)

Verificación por fases del **26 de noviembre de 2026**, en la Mac (Apple Silicon, macOS 26.5.2). Cada fase se hizo en
una rama local que el coordinador unió en `etapa-k1-1`. Esa rama se creó desde `main` (`310d7d7`: 0.16.0, esquema 29,
meseros 0.4.1 y cocina 0.1.0, con la K1 fusionada). La especificación está en `9139759`. Commits de cada fase:

- **Fase 0** (diagnóstico con los casos del propietario; sin código del producto): avance en `e73b921`.
- **Fase A** (servidor: un envío nunca se bloquea, «Productos sin ruta», lo pendiente de cuentas que ya no están
  abiertas, la cobertura, los dos 409, la `0030` y el contrato): `219c20c` a `e4184da`.
- **Fase B** (escritorio: el asistente y el diagnóstico de Terminales, «Productos sin ruta» y «Sin ruta» en
  Enrutamiento, Estaciones, el panel «Por enviar a cocina», el detalle de la cuenta y el historial): `01a4da7` a
  `c82c2be`, unida en `ab9771d`; avance de las fases A y B en `d7c3734`.
- **Fase C** (aplicación de cocina): sin trabajo (K11-D10).
- **Fase D** (rama `etapa-k1-1-d`, desde `d7c3734`): la versión en `f26eda3`, la prueba con fecha fija en `f3530cb`
  (K11-D23), los documentos de la etapa en `07d4778` y este documento.

**Servidor 0.17.0.** API v1, migración `0030_productos_sin_ruta.sql` (esquema **30**), una sola para toda la etapa con
la sección **«Fase A»**: el ajuste «Productos sin ruta» (`kitchen_settings.unrouted_station_id`), `dismissed_at` y
`dismissed_reason` en `kitchen_pending_changes` y, con su historial, los pendientes de cocina de cuentas que ya no están
abiertas dados por terminados. `min_client_versions` sin cambio y la capacidad nueva `kitchen-coverage-v1`. **Sigue
siendo una versión de desarrollo**, como la 0.16.0: confía solo en las llaves del entorno `test`
([licencias/PASO_A_PRODUCCION.md](licencias/PASO_A_PRODUCCION.md)).

**Aplicaciones Android sin cambio**: meseros 0.4.1 y cocina 0.1.0. La etapa no toca `android/`, `android-core/` ni
`cocina/` (K11-D10): `git diff --stat 310d7d7 -- android cocina android-core` está vacío en el commit final, así que no
hay APK nuevo ni se usaron emuladores (§0 y §6 de la especificación).

**Referencias.**

- Especificación: [etapas/ETAPA_K1_1_COCINA_AJUSTES.md](etapas/ETAPA_K1_1_COCINA_AJUSTES.md).
- Avance, con el detalle de cada fase y las decisiones K11-D1 a K11-D23: [ETAPA_K1_1_AVANCE.md](ETAPA_K1_1_AVANCE.md).
- La regla y lo que cambió en cocina: [KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md), «Pedidos mixtos y productos
  sin ruta · etapa K1.1». Guía del propietario: [COCINA_INSTALAR.md](COCINA_INSTALAR.md), sección 4. Contrato:
  [API_V1.md](API_V1.md), «Configuración guiada de cocina (etapa K1.1)».
- Evidencia anterior: [etapa K1](VERIFICATION_STAGEK1.md).

Este documento registra **solo lo que se ejecutó**.

**Las pruebas no tocaron la instalación real**: ni el servicio `local.adel.pos.server`, ni `/Library/Application
Support/AD-EL POS`, ni `/Applications/AD-EL POS.app`, ni `.local` del repositorio, ni el Keychain, ni PostgreSQL en
55432, ni Go en 8181 o en la escucha de red 8443. No se leyó nada de la instalación real (tampoco sus cifras). Ninguna
prueba usó `sudo`, `installer` ni `launchctl` (salvo el `launchctl` simulado de las pruebas de los scripts del
instalador), ni las órdenes de `dev.sh` de la instalación. **La hora de la Mac nunca se cambió.**

**Ninguna prueba llamó a `licensing.sicrat.com`**, y los códigos de las licencias de prueba no se usaron ni se
escribieron en ningún lado: las pruebas usan solo `contracts/licencias/v1/fixtures/vectors.json` y el simulador
`license-test`.

**Datos.** Todas las pruebas usan **datos sintéticos**, creados por cada prueba en una base desechable.

## Entorno aislado

El de la K1 ([VERIFICATION_STAGEK1.md](VERIFICATION_STAGEK1.md), «Entorno aislado»), sin emuladores:

- **Como mucho dos agentes a la vez** (§0 de la especificación), cada uno con su `ISO_TAG` y sus puertos: las fases A y
  B en paralelo; la Fase 0 y la regresión final, un solo agente.
- El clúster PostgreSQL 18.6 temporal en 55499, solo en loopback, con una base `adel_pos_test_*` desechable por paquete,
  por motor de Playwright y por Go temporal.
- `go-int ./cmd/ad-el-pos-server` usa puertos fijos (8192–8196 y 55511–55513): se corrió con un candado, uno a la vez.
- Vitest y Playwright sustituyen el puente nativo del escritorio por la simulación de `e2e/native-driver.ts`.

## Resultados ejecutados por fase

| Fase | Lo que se ejecutó y su resultado                                                                                                                                                                                                                                                                                                                                                                            |
| ---- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 0    | Reproducción por la API con un Go temporal y una pantalla de cocina emparejada: machacado + Coca Cola y machacado + Coca Cola + papas quedaban `blocked` (0 unidades liberadas, sin tarjeta); `TestStageK11UnroutedProducts` fallaba en esos casos y pasaba al quitar, solo para el experimento, la advertencia «Sin ruta de preparación» (revertido)                                                       |
| A    | `go-unit`; `go-int` de `operations` (con `TestStageK11UnroutedProducts` y las pruebas que esperaban el bloqueo, corregidas en `e93f7d2`), `database` (`TestStageK11Upgrade`), `apicontract` (`contractKitchenCoverage`), `administration`, `licensing` y `./cmd/ad-el-pos-server`; `fmt-check` y `sync-check`. `TestDatabaseTrustedTimeCopy` fallaba igual en la base `e73b921` por su fecha fija (K11-D23) |
| B    | `cargo` 122 y 4 ignoradas; `tsc`; Vitest 110 archivos y 1 049 pruebas; `build`; Playwright `workflow-stagez-k11-assistant` y `-diagnostics` solas y con las de cocina que tocan (`workflow-stage9d-accounts`, `-stage9d-cancel-account`, `-stage9f-kitchen-stations` y `-stagez-k1-terminals`): 6 de 6 en Chromium y en WebKit, contra la Fase A hasta `0766f71`                                            |
| D    | La regresión final (abajo)                                                                                                                                                                                                                                                                                                                                                                                  |

En cada fase se corrieron además `format` y `fmt-check`, y el MD5 de la especificación (`9363961a…`) quedó igual antes y
después.

## Regresión final

> **Resumen** (copia exacta de `07d4778`: la versión, la prueba corregida y los documentos de la etapa):
>
> - Go aprobó a la primera: `go-unit` y la integración de los 17 paquetes (los 16 de `scripts/database.py` y
>   `./internal/kitchen`), con `TestDatabaseTrustedTimeCopy` ya corregida (K11-D23);
> - `cargo` 122, Vitest 1 049 de 1 049, `build`, `fmt-check` y `sync-check`;
> - **Playwright 55 de 55 en Chromium** (en la segunda corrida: la primera cayó en la carrera conocida de la 9D) **y
>   55 de 55 en WebKit a la primera**, con las 4 de «instalación limpia» y las 2 de la K1.1, que llegan por primera vez a Playwright
>   con todo lo de la Fase A;
> - **ninguna corrección del producto**: la única corrección de la fase es la prueba con fecha fija (`f3530cb`);
> - **el `.pkg` 0.17.0**, sobre una copia exacta de `07d4778` (después solo cambió Markdown). **No se instaló nada.**

Una sola vez (§0, «Eficiencia»), en la rama `etapa-k1-1-d`, el 26/11/2026 de 11:33 a 12:55 (hora de Monterrey). Un solo
agente, sin otro trabajando a la vez. Go 1.27.3, Playwright 1.65.0, PostgreSQL 18.6, macOS 26.5.2.

- **El commit de la versión**, `f26eda3`: `config/product.json` en **0.17.0** con `dev.sh sync` (los metadatos, la copia
  generada, `package.json`, `tauri.conf.json`, `Cargo.toml` y los dos lockfile), `schema_version` 30 (de la Fase A) y
  `min_client_versions` sin cambio. Meseros sigue en 0.4.1 (5) y cocina en 0.1.0 (1).
- **La copia**: el servidor y el escritorio corrieron sobre una copia exacta de `07d4778` (`git archive`, 2 107 archivos,
  los mismos que `git ls-files`; idénticos por contenido salvo los tres `gradlew.bat`, que `git archive` entrega con
  CRLF por `.gitattributes`), con `.tools`, `.cache/go-mod`, `.cache/playwright` y `desktop/node_modules` enlazados y el
  arnés aislado (`ISO_TAG=D`). Después de `07d4778` solo cambió Markdown (este documento y `CLAUDE.md`).
- **Los puertos**: el clúster temporal 55499; Playwright con su Go temporal en 8231, la vista previa en 1461, el servidor
  de licencias simulado en 8511 con la confianza de `vectors.json` y el Go sin activar en 8233 (`ISO_LICENSE_SIM=1`), y
  un Go limpio nuevo por motor (8232, con su escucha de red en 8502) para las cuatro de «instalación limpia» de la 9H.
- **Las órdenes**: `iso.sh <copia> go-unit`, `go-int <paquete>` (uno por uno, cada uno con su base), `cargo`, `vitest`,
  `build`, `fmt-check`, `sync-check` y `browser <motor>`, con los guiones de la sesión (fuera del repositorio).

### Servidor y escritorio

| Suite                                                                                       | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| ------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `fmt-check` (comprobación de datos reales, Prettier, `gofmt`, `cargo fmt`) y `sync-check`   | Aprobados, en la copia y en el árbol de trabajo (con su índice de git: «el índice de git no tiene respaldos ni paquetes de migración»). «Metadatos coherentes.» (0.17.0, esquema 30)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `go-unit`                                                                                   | Aprobado (55 s): `go vet` (normal e `integration`), `go test -race -count=1 ./...` (**39 paquetes `ok`**, 7 sin pruebas de unidad), `GOOS=windows` `go build` y `go vet` (normal e `integration`). Incluye `TestNoAutomatedTestCallsTheRealLicenseServer`, `TestTestToolsNeverShip` y `TestProductionBuildFailsWithoutProductionKeys`                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `go vet -tags 'integration legacydocker'`, en macOS y con `GOOS=windows`                    | Aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| Integración PostgreSQL, los **16 paquetes** de `scripts/database.py` y `./internal/kitchen` | **17 de 17 a la primera**: `database` 30 s (con `TestStageK11Upgrade`), `administration` 296 s (con la subprueba de «Quitar datos de prueba» de la K1.1), `catalog` 26 s, `operations` 809 s (con `TestStageK11UnroutedProducts`, `TestStageK1KitchenDisplay`, `TestStageK1R1KitchenDisplaySecurity` y las de la 9D, 9F, A1, A2 y L1), `payments` 3 s, `reports` 149 s, `reports/operational` 46 s (fuera de su franja de 00:00 a 05:00), `designer` 25 s, `identity` 22 s, `apicontract` 56 s (`TestContractV1` con `contractKitchenCoverage`), `backupschedule` 9 s, `discovery` 7 s, `legacyimport` 14 s, `licensing` 12 s (con `TestDatabaseTrustedTimeCopy` corregida), `licenseseats` 5 s, `kitchen` 5 s y `./cmd/ad-el-pos-server` 67 s (con el candado) |
| Docker real del lector                                                                      | **No se corrió**: la etapa no lo toca                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `vitest` (`tsc --noEmit` + Vitest)                                                          | **110 archivos, 1 049 de 1 049** (28 s)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `build` (`npm run build`)                                                                   | Aprobado (el aviso de tamaño de fragmento de siempre)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `cargo test --locked --offline`                                                             | **122 aprobadas, 4 ignoradas** (entre ellas la de 10 minutos del canal, que no se corrió: la etapa no toca Rust ni el canal)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `scripts/macos-pkg/test-scripts.sh` (sin root) y `bash -n`                                  | «Scripts del instalador: todo correcto» (31 comprobaciones); `bash -n` de `common.sh`, `test-scripts.sh`, `app/preinstall`, `server/preinstall` y `postinstall`, `service/postinstall`, `build-macos-pkg.sh`, `check-legacy-data.sh` y `android-e2e.sh`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| **Playwright Chromium** completo (55 pruebas, 43 especificaciones)                          | **21 de 55 en la primera corrida** (6,9 min): `workflow-stage9d-accounts` cayó en la carrera conocida de la K1 (abajo) y las 33 siguientes en cascada («La aceptación debe comenzar con una base sin configurar»). **La segunda corrida, completa: 55 de 55** (26,9 min), con las 4 de «instalación limpia» y las 2 de la K1.1                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| **Playwright WebKit** completo                                                              | **55 de 55 a la primera** (26,8 min), con las 4 de «instalación limpia», las 3 de la K1 y las 2 de la K1.1                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |

**La carrera de `workflow-stage9d-accounts`** (la misma que la K1 vio en WebKit, K1-D39): la página entra con `admin` y,
justo después, la prueba vuelve a entrar por la API con la misma terminal; si una petición de la página ya iba en camino
con la sesión anterior, Go responde `SESSION_REPLACED` y la página vuelve a «Inicia sesión» con «Tu sesión se cerró
porque el usuario inició sesión de nuevo en esta terminal.» (así lo muestra el contexto del error). Es de la prueba, no
de la aplicación, y esa parte de la especificación es de la 9D: la Fase B solo cambió cómo se provoca «Enviado · sin
confirmar en cocina» (K11-D13). Se repitió la corrida completa sin cambiar nada y aprobó.

Pantalla de la Mac **desbloqueada** (`ioreg … CGSSessionScreenIsLocked` sin la clave) antes y después de cada corrida de
Playwright. **Ninguna prueba llamó a `licensing.sicrat.com`**: los Go temporales de Playwright tenían
`license_server_url` = `https://127.0.0.1:8511` (el simulador) y el Go limpio arrancó sin servidor de licencias; ninguna
bitácora nombra el servidor real.

### Pruebas con fechas fijas (K11-D23)

`TestDatabaseTrustedTimeCopy` (`internal/licensing`) fijaba el 21 y el 25/11/2026, y la migración deja su registro de
auditoría con la hora real, que el gestor de la licencia toma como la última actividad de la base: desde el 26/11 la
copia de la hora confiable ya no subía con el reloj simulado («cada minuto la copia sube con el reloj: {… Source:audit}»).
La prueba usa ahora horas relativas a la hora actual (el reloj simulado dos días adelante y la hora anterior cuatro días
antes) y comprueba lo mismo; el código de la licencia no cambió (`f3530cb`). Antes de corregirla falló así en el árbol
de la fase; después, `go-int ./internal/licensing` aprobó en el árbol y en la regresión.

Se revisaron con `grep` las demás pruebas con fechas fijas de noviembre de 2026. Las de integración que podrían
compararse con la hora real (`backupschedule_integration_test.go`, con un reloj simulado del 14 al 22/11 y las filas de
`backup_runs`; `stagel1_upgrade_test.go`, `stage9a_*_upgrade_test.go` y `stagek11_upgrade_test.go`, con datos
históricos literales; `cash_history_test.go`, que fija sus propias fechas de turno; `legacyimport` y
`legacy_acceptance_test.go`, con fechas del sistema anterior) no dependen de que la hora real sea anterior a ellas y
aprobaron; las de la licencia que usan `licensingtest.NewClock` en integración ya parten de `time.Now()`, y las
unitarias usan bases simuladas. Ninguna otra falló, así que no se cambió ninguna más.

### Lo corregido en la regresión

| Commit    | Qué                                                                                                                              | Por qué                                                                                                                        | Qué se repitió                                                                            |
| --------- | -------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------- |
| `f3530cb` | `TestDatabaseTrustedTimeCopy` con horas relativas a la hora actual (K11-D23). Solo la prueba; el código de la licencia no cambia | Caducó el 26/11/2026: fijaba el 21 y el 25/11 y la base ya tenía actividad del día en curso (falla igual en `e73b921`, Fase A) | `go-int ./internal/licensing` en el árbol; después, la regresión completa sobre `07d4778` |

La Fase D no corrigió nada del producto: Playwright probó por primera vez lo último de la Fase A (los ajustes de la §6
del contrato, «Quitar datos de prueba» y sus pruebas) y aprobó sin cambios. Ningún hallazgo obligó a cambiar una decisión
de la especificación ni del avance.

**Revisión de las capturas** (tema claro, las de la Fase B y las de esta regresión): los textos nuevos de la K1.1 están
en lenguaje sencillo y sin identificadores internos. La captura de Enrutamiento de la Fase B todavía decía «por ahora»
(el servidor de `0766f71`); la de esta regresión ya dice «Los productos sin ruta iban a Barra …, que está desactivada.
Mientras siga así, no necesitan preparación.». Fuera del alcance de la etapa quedan dos textos anteriores de
Preparación que se ven en las capturas: la etiqueta de estado «Liberado a preparación» de cada tarjeta y «No hay
unidades de preparación en este filtro.» (de la 9F); no se cambiaron.

## No verificado

- **Android**: la etapa no cambia `android/`, `android-core/` ni `cocina/` (K11-D10; `git diff --stat 310d7d7 -- android
cocina android-core` vacío), así que no se corrió ninguna suite de Android ni se usaron emuladores. Que un pedido
  mixto llega a la tableta lo prueba Go con la ruta que usa la tableta (`kitchen-display/orders`) en
  `TestStageK11UnroutedProducts`.
- **Una tableta real, la comanda en papel y la wifi del restaurante**: no hay dispositivo físico ni impresora; la
  impresión, con el transporte controlado de la cola.
- **La ventana nativa (Tauri) y Windows**: el escritorio, con la vista previa y el puente simulado; Windows, solo
  `GOOS=windows` para Go.
- **Un aviso pendiente real para una estación desactivada en el panel** («No se pudo avisar a Barra: está
  desactivada.»): lo prueban Go (`TestStageK11UnroutedProducts`, con `inactive_stations`) y Vitest; Playwright no lo
  provoca.
- **Docker real del lector** y **la prueba de Rust de 10 minutos del canal**: la etapa no los toca.
- **El `.pkg` 0.17.0 instalado**: instalarlo es la parada del propietario (§0 de la especificación).
- **Las cifras de la Mac del propietario**: no se leyó la instalación real; lo que hará la `0030` sale del código y de
  `TestStageK11Upgrade` (abajo, «Instalación real»).
- **Limitaciones aceptadas**: quitar la hora a un pedido programado no lo envía (K11-D9); lo que quedó bloqueado nunca
  consumió existencias y darlo por terminado no registra ese consumo (K11-D11); los bloqueados viejos de cuentas
  todavía abiertas siguen en «Por enviar a cocina» hasta enviarlos o cerrar la cuenta.

## Entregables

| Archivo                                                                  | Bytes      | SHA-256                                                            | Qué es                                                                                                                      |
| ------------------------------------------------------------------------ | ---------- | ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------- |
| `desktop/src-tauri/target/release/bundle/macos-pkg/AD-EL POS_0.17.0.pkg` | 24 019 520 | `14c23297624464066b8784e5bdb03249b141247cca182d74dee3b63737b66d7f` | Aplicación y servidor 0.17.0 (esquema 30), **versión de desarrollo** (confía en el entorno `test`); firma ad hoc verificada |

Sin APK nuevo: la etapa no cambia las aplicaciones Android (K11-D10). Siguen valiendo el de meseros 0.4.1 y el de cocina
0.1.0 de la K1.

**El `.pkg` 0.17.0** (`./scripts/dev.sh build-macos-pkg` sobre una **copia exacta de `07d4778`**, `git archive`, con
`CARGO_NET_OFFLINE=true`, `GOFLAGS=-mod=readonly` y `GOPROXY=off`; sin instalar nada: ni `sudo`, ni `installer`, ni
`launchctl`). De `07d4778` al commit final solo cambió Markdown, y ningún binario embebe documentos. Copia también en el
directorio temporal de la sesión, con el mismo SHA-256.

- **`pkgutil --expand-full`**:
  - los tres paquetes son 0.17.0: `local.adel.pos.app`, `.server` y `.service`;
  - la distribución ofrece «Servidor del restaurante» marcada por omisión (`start_selected="true"`), el servicio
    oculto y la arquitectura `arm64`;
  - la aplicación de dentro es 0.17.0 (`local.adel.pos.dev`), arm64, con `ad-el-pos-desktop` y `ad-el-pos-server` en
    `Contents/MacOS`;
  - el servidor de dentro dice `0.17.0` con `version`, lleva `"schema_version": 30` y la migración
    `0030_productos_sin_ruta.sql` embebida;
  - los scripts (`preinstall`, `postinstall`, `common.sh` y el `LaunchDaemon`) son idénticos a los de la copia.
- **`codesign --verify --deep --strict --verbose=2`** de la aplicación de dentro: «valid on disk» y «satisfies its
  Designated Requirement», firma `adhoc`. El `.pkg` no tiene firma de instalador («no signature»), como dice el script.
- **Confía en el entorno `test`**: `go version -m` del servidor no muestra `-tags` (no es la de `adelpos_production`),
  y el binario lleva el `kid` `adelpos-test-202611-1` y el de soporte de desarrollo
  (`adelpos-soporte-development-20261123`).
- **No lleva lo que no debe**:
  - ninguno de los identificadores ni semillas de `vectors.json` (los 8 buscados en todos los archivos expandidos);
  - ningún archivo `vectors.json`, `.lic`, `.licreq`, `.pem` ni `.key`, ni `license-test` ni `soporte-desbloqueo`;
  - ningún bloque `-----BEGIN … PRIVATE KEY-----`. La cadena `PRIVATE KEY` solo aparece como nombre de tipo PEM de
    las bibliotecas de Go y de Rust.
- **Los anteriores no cambiaron**: los `.pkg` de 0.10.0 a 0.16.0 siguen con el mismo SHA-256 antes y después (el de la
  0.16.0, `add1e9e7…3f47`).

Instalarlo es la parada del propietario.

## Instalación real

Según el avance de la etapa, la instalación real de la Mac está en **0.16.0** (la aplicación de `/Applications` dice
0.16.0): PostgreSQL en 55432 y Go en 8181 y 8443. Ninguna prueba de la etapa la tocó, y esta fase no leyó nada de ella
(tampoco sus cifras: cuántos pendientes tiene).

**Se actualiza con el `.pkg` 0.17.0**, que instala el **propietario** con «Servidor del restaurante» marcada. Es una
parada, porque pide la contraseña de administrador. Al arrancar:

1. **El `host` respalda la base** (`…-antes-de-migrar.dump`) **y aplica la `0030`**, en una sola transacción. Según el
   código de la migración y `TestStageK11Upgrade` (que la aplica sobre una base en el esquema 29 con lo que deja la
   0.16.0):
   - **el ajuste nuevo** «Productos sin ruta» queda en **«No necesitan preparación»** (`unrouted_station_id` vacío): lo
     que no tiene ruta deja de detener los pedidos sin que haya que configurar nada;
   - **los pendientes de cuentas que ya no están abiertas** (cobradas y cerradas, canceladas o combinadas en otra que ya
     se cerró, por la pertenencia actual de sus unidades): sus envíos pendientes, programados o bloqueados pasan a
     «cancelado», cada uno con su renglón «Pedido sin enviar dado por terminado» en Cocina › Historial (motivo: la cuenta
     ya no estaba abierta al actualizar), y sus avisos pendientes quedan terminados con «Aviso sin entregar dado por
     terminado». Así, **los pedidos «Bloqueado por configuración» de las cuentas ya cobradas desaparecen del panel**
     (los 13 de la captura del propietario, si sus cuentas ya se cerraron);
   - **lo de cuentas todavía abiertas se queda**: un bloqueado viejo de una cuenta abierta se ve en «Por enviar a
     cocina» como «La cocina no recibió este pedido», y «Enviar a cocina ahora» lo manda con la regla nueva; un pedido
     para llevar cobrado y sin entregar sigue abierto y su programado llega a su hora;
   - **nada de ventas, pagos, cuentas, partidas, cortes ni inventario cambia**: la prueba compara cada tabla del esquema
     29 antes y después, y solo cambian `kitchen_releases` (estado y revisión de esos pendientes; sus advertencias quedan
     igual), `kitchen_pending_changes` (la hora y el motivo) y `kitchen_history` (los renglones nuevos). Lo que quedó
     bloqueado nunca consumió existencias, y darlo por terminado no registra ese consumo (K11-D11).
2. **La identidad de la red local no cambia**: las terminales de escritorio, los teléfonos y tabletas de meseros y la
   tableta de cocina que ya están emparejados siguen emparejados, sin instalar otro APK. La licencia, si ya está
   activada, sigue igual (la 0.17.0 también confía en el entorno `test`).
3. **Desde ese momento** un pedido con un producto sin ruta llega a la pantalla de cocina con lo que sí tiene ruta; al
   cobrar y cerrar o al cancelar una cuenta, lo que le quedaba pendiente de cocina se da por terminado; y al aprobar una
   pantalla de cocina en Terminales se abre el asistente «Configurar qué recibe»
   ([COCINA_INSTALAR.md](COCINA_INSTALAR.md), sección 4). Las terminales de escritorio por la red se actualizan con su
   propio `.pkg`; mientras tanto, una con la 0.16.0 sigue funcionando y muestra el panel de siempre.
