# Verificación · Etapa 9F

Verificación por fases del **13 de noviembre de 2026**, en la Mac (Apple Silicon, macOS 26), en la rama local
`etapa-9f` (creada desde `main` = `145640e`, versión 0.9.4 con la 9E fusionada): fases 0-R y 0 `b8d707b`; fase D
`f56fcd9`; fase A `b91fb7a`; fase E `afcd61c`; correcciones de la parte 1 de la revisión independiente `e20d006`; fase B
`0c06001` (migración 0022, versión 0.9.5 y esquema 22); fase C `731dfaa` (sección «Fase C» de la 0022). Versión
**0.9.5**, API v1, migración `0022_printing_and_kitchen_stations.sql` (esquema **22**). Especificación:
[etapas/ETAPA_9F.md](etapas/ETAPA_9F.md). Avance con el detalle de cada fase: [ETAPA_9F_AVANCE.md](ETAPA_9F_AVANCE.md).
Evidencia anterior: [etapa 9E](VERIFICATION_STAGE9E.md), [etapa 9D](VERIFICATION_STAGE9D.md) y las anteriores desde
[VERIFICATION.md](VERIFICATION.md). Este documento registra **solo lo que se ejecutó**.

**Alcance de esta evidencia.** Las cifras son las que cada fase registró en el avance, más los cierres que el
coordinador ejecutó sobre una copia exacta de algunos commits (fases D, A y E y correcciones de la parte 1). Al escribir
este documento, el avance no registra un cierre del coordinador de las fases B y C sobre los commits fusionados: sus
cifras son las de cada agente sobre su propia copia, y la validación del código fusionado es la «Regresión final».
Tampoco registra todavía las correcciones de las partes 2 y 3 de la revisión independiente (ver «Revisión
independiente»), que la regresión final cubre.

**Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432, `backups/`, el `.app`
instalado), salvo lo que se hizo a propósito con la regla de autonomía de `CLAUDE.md` §0: la instalación de la 0.9.4
de la Fase 0-R (con respaldo y ensayo de restauración) y las consultas de solo lectura de las fases 0-R y 0
(transacción `read_only`, solo conteos y nombres de configuración). No se leyeron secretos y ninguna prueba envió
trabajos a una impresora, a un cajón, a CUPS real ni a una conexión TCP fuera de loopback.

## Entorno aislado

Copias del worktree de cada fase en el directorio temporal de la sesión (sin `.git`, `.local`, `.tools`, cachés ni
artefactos; `node_modules` enlazado), un clúster PostgreSQL temporal en el puerto 55499 (solo loopback) compartido por
los agentes, una base `adel_pos_test_*` desechable por paquete y por motor de Playwright, `GOPROXY=off`, y Go temporal y
vista previa en puertos propios de cada parte: 8183/1421 en la Fase 0-R y en los cierres del coordinador (8189/1427 en
el de la fase E), 8185/1423 en la D, 8184/1422 en la A, 8187/1425 en la E, 8190/1428 en las correcciones de la parte 1,
8186/1424 en la B y 8188/1426 en la C. Desde la fase A el Go temporal de Playwright corre con `test_mode: true`. La
pantalla del equipo estuvo desbloqueada en las corridas de Playwright registradas, así que WebKit es una aprobación
real.

**Pruebas automatizadas, no físicas.** Vitest con jsdom y Playwright con Chromium y WebKit sustituyen el puente nativo
por la simulación de `e2e/native-driver.ts`. **No** sustituyen la ventana nativa (WKWebView), una pantalla táctil, el
sonido de un equipo real, el papel ni el corte de una impresora, dos terminales reales ni Windows. La impresión se
comprobó con bytes (transportes controlados, ESC/POS por loopback, la cola de Windows con el administrador de impresión
simulado) y con impresoras de tickets de la aceptación que nunca imprimen (colas del equipo inexistentes). En las
capturas de Preparación, los colores por tiempo los presenta el puente de la prueba (Go calcula los tiempos con su hora)
y «Sin conexión» se simula rechazando las peticiones en el puente.

## Fase 0-R · regresión de la 0.9.4 en macOS

Sobre una copia exacta de `145640e` (`git archive`), 1684 s:

| Suite                                    | Resultado                                                                                                                                                                                         |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato (Prettier, `gofmt`, `cargo fmt`) | aprobado (finales de línea de `.gitattributes` sin diferencias en macOS)                                                                                                                          |
| Metadatos (`sync-metadata --check`)      | aprobado                                                                                                                                                                                          |
| `go vet` (normal y `-tags=integration`)  | aprobado                                                                                                                                                                                          |
| `GOOS=windows go build ./...` y `go vet` | aprobado                                                                                                                                                                                          |
| Go unitarias con `-race`                 | 22 paquetes `ok`                                                                                                                                                                                  |
| Integración PostgreSQL con `-race`       | 9 de 9: `database` 7.2 s, `administration` 65.6 s, `catalog` 9.1 s, `operations` 253.2 s, `payments` 2.4 s, `reports` 122.5 s, `reports/operational` 41.2 s, `designer` 20.6 s, `identity` 18.0 s |
| `tsc --noEmit`                           | sin errores                                                                                                                                                                                       |
| Vitest (`npm test`)                      | 76 archivos, 757 pruebas aprobadas                                                                                                                                                                |
| `npm run build`                          | correcto (aviso de tamaño de fragmentos ya conocido)                                                                                                                                              |
| `cargo test --locked --offline`          | 41 aprobadas, 1 ignorada                                                                                                                                                                          |
| Playwright Chromium                      | 19 de 19 (7.5 min)                                                                                                                                                                                |
| Playwright WebKit                        | 18 de 19 en la corrida completa; **repetición completa: 19 de 19** (7.3 min, sin errores de página)                                                                                               |
| `.app` (`tauri build --bundles app`)     | 0.9.4, Mach-O arm64, 13.54 MiB                                                                                                                                                                    |

El fallo de WebKit fue un error de página «The provided value is non-finite» al final de
`workflow-stage9d-topbar-tables`, que completó todos sus pasos; la especificación sola (con la traza de errores de
página) y la repetición completa aprobaron sin errores. Se registra como **intermitente, de origen no identificado**
(ver «No verificado»). De lo que la 9E cambió desde Windows (procesos, Keychain, rutas, impresión, finales de línea, foco
y Escape de los diálogos) no hubo nada que corregir.

**Instalación de la 0.9.4 en la Mac** (13/11/2026, sin migraciones): comprobación de solo lectura antes y después
(migraciones 1–21, 134 tablas, 154 permisos, 0 turnos en arqueo y 0 trabajos de impresión sin terminar; idéntica);
respaldo `backups/adel_pos-20261113T144419Z-6e62f4.dump` con «Restauración completa verificada: 134 tablas con conteos
idénticos»; `server-stop` ordenado; `migrate` → `applied:0` dos veces; `server-start` con readiness 200 y
`api/v1/version` 0.9.4 / esquema 21; `build-macos` y `open-app` con el `.app` 0.9.4. Detalle en el avance, «Fase 0-R».

## Fase 0 · verificación contra el código

Un solo agente, de solo lectura. Hallazgos y decisiones D1–D9 del coordinador en el avance («Fase 0»); ninguna cambió la
especificación. Lo que se comprobó contra la instalación real fue de solo lectura: **la Mac no tiene datos de prueba**
(0 impresoras, 0 estaciones, 0 rutas, 0 estaciones por terminal y ningún producto ni categoría con nombre de prueba).

## Resultados ejecutados por fase

«Agente»: corrida del agente de la fase sobre su copia; «cierre»: corrida del coordinador sobre una copia exacta del
commit. Integración con `-tags=integration` y `-race`; los tiempos son los de `go test`.

| Fase o parte (commit)                    | Estáticas y Go                                                                                                                                                                                                                              | Integración PostgreSQL                                                                                                                                                                                                                 | Vitest, compilación y Rust                                           | Playwright                                                                                                                                                                                                     |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| D, agente (`b8d707b` + D)                | Prettier sin diferencias; sin cambios en Go ni en Rust                                                                                                                                                                                      | No aplica                                                                                                                                                                                                                              | `tsc` sin errores; 77 archivos, 766 pruebas; build correcto          | Nueva `workflow-stage9f-topbar-map`: Chromium 1/1 y WebKit 1/1; con `restaurant`, `workflow-stage9c-navigation` y dos `workflow-stage9d-*`: 5/5 y 5/5; `workflow-stage9c-keyboard` con `restaurant`: 2/2 y 2/2 |
| D, cierre (`f56fcd9`)                    | Formato, metadatos, `go vet` (también `GOOS=windows`), Go unitarias con `-race`                                                                                                                                                             | —                                                                                                                                                                                                                                      | `tsc`; 77 archivos, 766 pruebas; build; `cargo test` 41 + 1 ignorada | `restaurant`, `workflow-stage9c-navigation`, `workflow-stage9d-*` y `workflow-stage9f-topbar-map`: Chromium 7/7 y WebKit 7/7                                                                                   |
| A, agente (`b8d707b` + A)                | Prettier, `gofmt` y `cargo fmt` sin diferencias; `go vet` (normal e `integration`); `GOOS=windows` `go build` y `go vet` (también `integration`); Go unitarias con `-race`: 23 paquetes `ok` (nuevo `maintenance`)                          | `administration` `ok` (74.6 s), con 6 subpruebas 9F, incluido el respaldo real con `pg_dump`                                                                                                                                           | `tsc`; 77 archivos, 764 pruebas; build; `cargo test` 42 + 1 ignorada | `workflow-stage9f-zz-test-data` sola: 1/1 y 1/1; corrida completa: Chromium 20/20 y WebKit 20/20                                                                                                               |
| A, cierre (`b91fb7a`, con D)             | Formato, metadatos, `go vet` (también `GOOS=windows`), Go unitarias con `-race`                                                                                                                                                             | `administration`                                                                                                                                                                                                                       | `tsc`; 78 archivos, 773 pruebas; build; `cargo test` 42 + 1 ignorada | **Corrida completa: Chromium 21/21 y WebKit 21/21**, con `workflow-stage9f-zz-test-data` al final                                                                                                              |
| E, agente (`f56fcd9` + E)                | `gofmt`, `go vet` (normal e `integration`), `GOOS=windows`; Go unitarias con `-race` de `operations`, `reports` y `administration` `ok`                                                                                                     | `operations` `ok` (291.4 s, `TestStage9FCancelUnitsAndRetiredVoid` 5/5); `reports` `ok` (130.2 s)                                                                                                                                      | `tsc`; 78 archivos, 770 pruebas; build; `cargo test` 41 + 1 ignorada | Nueva `workflow-stage9f-cancel-units`: 1/1 y 1/1; con `restaurant` y dos `workflow-stage9d-*`: 4/4 y 4/4; tras un ajuste, `restaurant` y la nueva: 2/2 y 2/2                                                   |
| E, cierre (`afcd61c`, con D y A)         | Formato, metadatos, `go vet` (también `GOOS=windows`), Go unitarias con `-race`                                                                                                                                                             | `operations`, `reports` y `reports/operational`                                                                                                                                                                                        | `tsc`; 79 archivos, 777 pruebas; build; `cargo test` 42 + 1 ignorada | `restaurant`, `workflow-stage9d-accounts`, `workflow-stage9d-cancel-account` y `workflow-stage9f-cancel-units`: Chromium 4/4 y WebKit 4/4                                                                      |
| Correcciones parte 1, agente             | `gofmt`; `go vet` (también `integration` de `administration`, `operations` y `maintenance`); `GOOS=windows`; Go con `-race` de `maintenance`, `operations` y `administration` `ok`                                                          | `administration` `ok` (72.7 s, 47 subpruebas; `9F_respaldo_real_con_pg_dump` omitida por el entorno del agente); `operations` `ok` (281 s, 263 subpruebas; 2 opcionales omitidas por diseño)                                           | `tsc`; 80 archivos, 781 pruebas; build                               | Filtro `workflow-stage9f-` (base nueva por motor): Chromium 3/3 y WebKit 3/3                                                                                                                                   |
| Correcciones parte 1, cierre (`e20d006`) | Formato, metadatos, `go vet` (también `GOOS=windows`), Go unitarias con `-race`                                                                                                                                                             | `administration` y `operations`; `9F_respaldo_real_con_pg_dump` aparte con `-v`: **aprobada**                                                                                                                                          | `tsc`; 80 archivos, 781 pruebas; build; `cargo test` 42 + 1 ignorada | **Corrida completa: Chromium 22/22 y WebKit 22/22**                                                                                                                                                            |
| B, agente (`b91fb7a` + B)                | Prettier (`--check .`), `gofmt`, `cargo fmt`, `sync-metadata --check` sin diferencias; `go vet` (normal e `integration`); `GOOS=windows` (también `integration`); Go unitarias con `-race`: 23 paquetes `ok`                                | `database` 7.8 s (`TestStage9FPrintingUpgrade`), `operations` 270.5 s (`testPrintingStage9F`, 5 subpruebas), `administration` 69.3 s, `designer` 21.2 s e `identity` 18.6 s: `ok`                                                      | `tsc`; 79 archivos, 785 pruebas; build; `cargo test` 44 + 1 ignorada | Nueva `workflow-stage9f-printing` sola: 1/1 y 1/1; corrida completa: Chromium 22/22 (8.0 min) y WebKit 22/22 (8.3 min)                                                                                         |
| C, agente (`e20d006` + C)                | `gofmt`, `go vet` (normal e `integration`), `GOOS=windows`; Go unitarias con `-race` de `stationscope` (nuevo), `kitchen`, `administration`, `maintenance`, `operations`, `database`, `designer` e `identity` `ok`; `sync-metadata --check` | `operations` 298.6 s (`TestStage9FKitchenStations` 7/7), `administration` 85.8 s (subpruebas `9F_estaciones_del_usuario_*`), `database` (`TestStage9FKitchenStationsUpgrade`), `designer`, `identity`, `kitchen` y `maintenance`: `ok` | `tsc`; 84 archivos, 801 pruebas; build; `cargo test` 43 + 1 ignorada | Nueva `workflow-stage9f-kitchen-stations`: 1/1 y 1/1; con `restaurant`, `workflow-stage9c-navigation` y `workflow-stage9d-cancel-account`: 4/4 y 4/4                                                           |

«—»: la parte no registró esa suite. En la fase B, la integración de `designer` e `identity` corrió antes de los últimos
cambios de la fase en `payments` (huella) y en las firmas de `maintenance`, que no les afectan (avance, «Fase B»). Dos
corridas de las fases B y C se interrumpieron porque la Mac entró en reposo por batería baja y se repitieron completas.

**Controles negativos** (en copias, sin entregar): sin el `blur` del logotipo, la prueba de la barra falla en Chromium, y
con la leyenda de la 9D falla a 1200 px (fase D); sin el 410 y con la regla de autorización anterior,
`TestStage9FCancelUnitsAndRetiredVoid` falla en 3 de 5 subpruebas (fase E); con el `plan.go` y el `maintenance.go`
anteriores fallan las subpruebas de la ruta del restaurante y del reenvío (correcciones de la parte 1); con la terminal
sin filtrar, `TestStage9FKitchenStations` falla en 3 de 7 (fase C).

## Revisión independiente

Un agente de solo lectura, por partes, a medida que se confirmaban las fases.

**Parte 1** (`b8d707b..afcd61c`: fases D, A y E). Sin hallazgos críticos ni altos. Comprobado: «Quitar datos de prueba»
no puede borrar operación (ninguna llave foránea en cascada; los bloqueos se leen de `pg_constraint`, también las
compuestas), firmas exactas, respaldo antes de borrar con el plan recalculado bajo los locks exclusivos, `pg_dump` con
argumentos fijos y la contraseña solo en el entorno, modo de pruebas rechazado en `Validate` y en Windows contra la
carpeta conocida del sistema, 410 de `void` antes de permisos y lock, y autorización de supervisor antes de escribir y
sin autoautorización.

| Hallazgo                                                                                                               | Corrección (`e20d006`, salvo B4)                                                                                    |
| ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| M1 · medio · quitar un producto o una categoría de prueba los sacaba en silencio de promociones del restaurante        | Lo que participa en una promoción se desactiva (el producto además se archiva) y la promoción no cambia             |
| B1 · una ruta de un producto del restaurante con solo estaciones de prueba aparecía como «Datos de prueba»             | Se quita igual, pero aparece en «Se desvinculará» con su motivo                                                     |
| B2 · un reenvío de la misma operación podía crear otro respaldo o responder 409; el fallo tras el plazo no se auditaba | Una eliminación a la vez; el reenvío recibe la respuesta guardada; el intento fallido se audita con un plazo propio |
| B3 · la huella idempotente solo excluía la clave exacta `authorization.password`                                       | `withoutAuthorizationPasswords` sin distinguir mayúsculas en `operations` (y en `payments` con la fase B)           |
| B4 · la descripción de `accounts.manage` seguía hablando de cancelar con motivo libre                                  | La 0022 (sección de la fase C) la cambia a «Editar datos de cuentas»                                                |
| B5 · limpieza frágil del `finally` de la prueba de humo de Windows y sin prueba del rechazo de `requireTestServer`     | Cada paso de limpieza por separado (sin ejecutar en Windows) y `src/testing/requireTestServer.test.ts`              |

**Partes 2 y 3** (fases B y C). Al escribir este documento, sus informes y correcciones no estaban en el avance. Según el
coordinador, las correcciones dejan este estado, que es el que describe la documentación: el nombre de la precuenta se
elige en **Administración › Documentos** (se sigue guardando en `payment_settings.precheck_name`) y en el diseñador el
tipo del ticket de pago se llama «Ticket de pago» (antes «Ticket de venta»; plantillas y salida sin cambios); con corte,
el renglón abierto se cierra antes del avance; «Reintentar» toma la configuración actual de la impresora; «Imprimir con
la plantilla vigente» de cobros y caja pide motivo; la impresora de tickets ya no admite impresoras de tipo agente ni
inactivas; el sonido de la terminal de cocina se activa con el primer toque y avisa si no puede sonar; y `PUT
kitchen/access` rechaza una lista vacía sin «todas». Sus pruebas se registran en el avance; la regresión final corre
sobre el código con ellas.

## Cobertura de la etapa 9F

Correspondencia con las pruebas de §8 de la especificación. Rutas relativas a `backend/internal/`, `desktop/src/` o
`desktop/e2e/` según la capa.

| Prueba exigida (§8)                                                  | Dónde y qué                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| -------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Fase 0-R: regresión completa de la 0.9.4                             | «Fase 0-R · regresión de la 0.9.4 en macOS».                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| Las pruebas no escriben en la base de producción                     | `config/config_test.go` (`TestTestModeRequiresTestDatabase`, `TestTestModeIsReadFromTheFile`), `httpapi/router_test.go` (`TestVersionReportsTestMode`), `testing/requireTestServer.test.ts` (el puente rechaza un servidor sin `test_mode`) y todas las corridas de Playwright desde la fase A. `standalone/testmode_test.go` solo compiló (Windows).                                                                                                                                                                        |
| «Quitar datos de prueba»: lista, respaldo y datos con operaciones    | `administration` (subpruebas 9F de `test_data_acceptance_test.go`: permisos, sin datos no respalda, datos reales con nombre genérico intactos, desvinculaciones, respaldo fallido, idempotencia, reenvío, promociones, datos con ventas y tareas desactivados sin cambiar 16 tablas de operación, respaldo real con `pg_dump`), `maintenance/signatures_test.go` y `reasons_test.go`, `pages/maintenance.test.tsx` y `workflow-stage9f-zz-test-data.spec.ts` (camino «eliminar» sola y «desactivar» en la corrida completa). |
| Aviso sin impresora con «Configurar impresora»                       | `operations/printing_stage9f_test.go` (`9F_D4_sin_impresora_valida_avisa_sin_encolar`) y `workflow-stage9f-printing.spec.ts` (paso 1: aviso en el momento, reintento y ningún trabajo fallido en la cola).                                                                                                                                                                                                                                                                                                                   |
| Reintento sin motivo y motivo obligatorio al reimprimir              | `9F_D3_reintento_sin_motivo_y_reimpresion_con_motivo`, `pages/kitchen/printingStage9f.test.tsx` («reintenta con un toque, sin motivo») y `workflow-stage9f-printing.spec.ts` (pasos 3 y 4).                                                                                                                                                                                                                                                                                                                                  |
| Corte total, parcial y sin corte, con avance, en los bytes ESC/POS   | `documents/paper_cut_test.go` (`TestPaperCutEnding`), `kitchen/paper_cut_test.go` (texto plano, ESC/POS con formato, red por loopback y cola de Windows simulada), `9F_D5_corte_de_papel_validado_y_auditado` y `printingStage9f.test.tsx` (valores por omisión en 80 y 58 mm); la prueba de ancho corta en `workflow-stage9f-printing.spec.ts` (paso 6).                                                                                                                                                                    |
| Nombre de la precuenta en botones y en el impreso                    | `9F_nombre_visible_de_la_precuenta`, `printingStage9f.test.tsx` (paleta y menú) y `workflow-stage9f-printing.spec.ts` (paso 2: botones, menú nativo simulado, panel y encabezado impreso con «Ticket de venta»).                                                                                                                                                                                                                                                                                                             |
| «Mesa» sin repetir                                                   | `documents/table_names_test.go` (`TestTableContextDoesNotRepeatWord`), `9F_mesa_sin_repetir`, `printingStage9f.test.tsx` y `pages/kitchen/preparationModel.test.ts`.                                                                                                                                                                                                                                                                                                                                                         |
| Estaciones por terminal y por usuario, intersección y aviso          | `stationscope` (4 pruebas de la intersección), `TestStage9FKitchenStations` (tablero, filtro, cambios de estado rechazados, acuses, comandas, tiempo real, sin estaciones en común), subpruebas `9F_estaciones_del_usuario_*`, `terminalStations.test.tsx`, `userKitchenStations.test.tsx` y `workflow-stage9f-kitchen-stations.spec.ts`.                                                                                                                                                                                    |
| Un producto en dos estaciones, cada una con su parte                 | `TestStage9FKitchenStations` (combo en dos estaciones, partida lista solo con sus dos partes y cuenta lista al final; complemento ligado a otro producto que viaja con su producto).                                                                                                                                                                                                                                                                                                                                         |
| Colores por tiempo y sin botón «Actualizar»                          | `preparationModel.test.ts` (umbrales de D7), `preparationBoard.test.tsx` (colores con texto, sin «Actualizar», «Reintentar» solo sin conexión, sonido solo con una comanda nueva) y `workflow-stage9f-kitchen-stations.spec.ts` (ambos temas a 1280 × 720 y 1920 × 1080; colores presentados por el puente).                                                                                                                                                                                                                 |
| Cola de impresión: altura limitada y detalle a la vista (Playwright) | `workflow-stage9f-printing.spec.ts` (paso 5) y `printingStage9f.test.tsx` (agrupación «×N», sin datos internos, detalle a la vista).                                                                                                                                                                                                                                                                                                                                                                                         |
| Barra compacta en los cuatro anchos y leyenda sin desplazamiento     | `workflow-stage9f-topbar-map.spec.ts` (1200, 1280, 1440, 1500 y 1920 px en ambos temas, foco del logotipo, «Sin conexión»), `navigation/navigation.test.tsx` y `stage9fLayout.test.ts`.                                                                                                                                                                                                                                                                                                                                      |
| Cancelar unidades con y sin permiso y la columna «Autorizó»          | `TestStage9FCancelUnitsAndRetiredVoid`, `reports` (pruebas de la 9C y la 9D ajustadas), `pages/restaurant/cancelItem.test.tsx`, `capture/capture.test.tsx` y `workflow-stage9f-cancel-units.spec.ts` (también PDF y Excel).                                                                                                                                                                                                                                                                                                  |
| Ruta antigua de anulación retirada                                   | `TestStage9FCancelUnitsAndRetiredVoid` (410 sin cambiar operaciones, auditoría, eventos ni tiempo real) y `workflow-stage9f-cancel-units.spec.ts`.                                                                                                                                                                                                                                                                                                                                                                           |
| Migración 0022                                                       | `database/stage9f_printing_upgrade_test.go` y `stage9f_kitchen_upgrade_test.go` (de 21 a 22 con datos; nada cambia, CHECK con nombre, repetible).                                                                                                                                                                                                                                                                                                                                                                            |
| Go para Windows desde la Mac                                         | `GOOS=windows GOARCH=amd64 go build ./...` y `go vet` en cada fase con Go y en los cierres (tabla de arriba).                                                                                                                                                                                                                                                                                                                                                                                                                |
| Regresión completa                                                   | «Regresión final».                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |

## No verificado

No se afirma ninguna de estas:

- **Papel y corte**: el corte total y parcial y los renglones de avance en papel, en una impresora con y sin cortador,
  por red, por USB en Windows y por CUPS; que una impresora sin cortador ignore `GS V` es lo habitual, no algo
  comprobado. `submitted` sigue siendo aceptación del transporte, no papel.
- **Terminal de cocina**: una pantalla táctil real; el **aviso sonoro en un equipo real** (WebKit y WebView2 exigen un
  toque del usuario antes de reproducir sonido; se activa con el primer toque y avisa si no puede); **dos terminales de
  cocina a la vez**; una **cocinera sin `kitchen.stations.read`**, que ve los cambios de su alcance con el sondeo de 5 s.
- **Windows**: nada de la 9F se ejecutó en Windows (solo compiló y pasó `go vet` con `GOOS=windows`): el `host` con
  `ADEL_TEST_MODE=1`, `installed-smoke.ps1` (ni su sintaxis), la prueba instalada, «Quitar datos de prueba» sobre la
  instalación de Windows de la 9E, el corte por la cola RAW, la barra compacta y el foco del logotipo en WebView2 al
  125 % y el sonido en WebView2. Lista en [NOTAS_WINDOWS.md](NOTAS_WINDOWS.md), «Etapa 9F: qué verificar en Windows».
- **Ventana nativa**: el menú nativo real con el nombre de la precuenta (Rust cambia el texto con `set_text`; solo se
  probaron la lista cerrada y la simulación del puente) y, en general, la ventana WKWebView.
- **Error intermitente de WebKit** «The provided value is non-finite»: visto una vez en la Fase 0-R (y antes en la 9A),
  no reproducido después; origen no identificado.
- **Complementos repartidos entre estaciones**: no se implementó (solo los combos se reparten); queda para decisión del
  propietario.
- Pendientes de etapas anteriores sin cambio: papel con formato y logotipo (9B), papel a 48/32 columnas (9C), cajón
  físico, pantalla táctil física y diálogo nativo real de guardado.

## Regresión final

Ejecutada el 13/11/2026 por el coordinador con un script, sobre una copia exacta de `2b26439` (`git archive`), con el
clúster PostgreSQL temporal 55499, Go temporal en 8183 y vista previa en 1421; la pantalla del equipo estuvo
desbloqueada antes y después de Playwright. 1802 s.

| Suite                                    | Resultado                                                                                                                                                                                         |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato (Prettier, `gofmt`, `cargo fmt`) | aprobado                                                                                                                                                                                          |
| Metadatos (`sync-metadata --check`)      | aprobado                                                                                                                                                                                          |
| `go vet` (normal y `-tags=integration`)  | aprobado                                                                                                                                                                                          |
| `GOOS=windows go build ./...` y `go vet` | aprobado                                                                                                                                                                                          |
| Go unitarias con `-race`                 | 24 paquetes `ok` (con `maintenance` y `stationscope`, nuevos)                                                                                                                                     |
| Integración PostgreSQL con `-race`       | 9 de 9: `database` 8.2 s, `administration` 84.5 s, `catalog` 8.9 s, `operations` 281.9 s, `payments` 2.4 s, `reports` 121.0 s, `reports/operational` 40.4 s, `designer` 20.2 s, `identity` 17.8 s |
| `tsc --noEmit`                           | sin errores                                                                                                                                                                                       |
| Vitest (`npm test`)                      | 86 archivos, 821 pruebas aprobadas                                                                                                                                                                |
| `npm run build`                          | correcto (aviso de tamaño de fragmentos ya conocido)                                                                                                                                              |
| `cargo test --locked --offline`          | 45 aprobadas, 1 ignorada                                                                                                                                                                          |
| Playwright Chromium                      | 24 de 24 (8.7 min)                                                                                                                                                                                |
| Playwright WebKit                        | 24 de 24 (8.8 min)                                                                                                                                                                                |
| `.app` (`tauri build --bundles app`)     | 0.9.5, Mach-O arm64, 13.56 MiB                                                                                                                                                                    |

Sin fallos ni repeticiones; el error intermitente de WebKit de la Fase 0-R no apareció. **Rust para Windows**: el
destino `x86_64-pc-windows-msvc` no está instalado en la Mac (y `cargo check` para MSVC exige además el compilador de
recursos y un compilador C de Windows), así que se revisó a mano: ningún cambio de Rust de la 9F (`api.rs`, `menu.rs`,
`server_host.rs` y la versión) está dentro de bloques condicionales por plataforma, de modo que el mismo código compila
en ambos sistemas (`menu_set_state` sigue registrado en Windows, donde el menú nativo se omite). Las mediciones de
volumen y la impresión física no corren por omisión (por diseño).

## Instalación real

Sobre `2b26439` con el árbol limpio, después de la regresión final aprobada, según la regla de autonomía de
`CLAUDE.md` §0 (13/11/2026):

1. **Estado previo** (servidor 0.9.4, esquema 21): comprobación de solo lectura (transacción `read_only`, solo conteos
   y nombres de configuración): migraciones 1–21, 134 tablas, 154 permisos, cuentas `closed:2, open:7, voided:1`, 15
   unidades, 2 pagos, turnos `closed:1, open:1`, 5 operaciones de caja, 0 movimientos de inventario, 150 eventos de
   auditoría, 9 reconocimientos de venta, sin impresoras, estaciones ni rutas, **0 turnos en arqueo y 0 trabajos de
   impresión sin terminar**, y ningún rol que opere cocina sin `kitchen.stations.all` (el cambio de significado de las
   estaciones por terminal no afecta a esta instalación). Repetida justo antes de detener el servidor: idéntica.
2. **Respaldo** `backups/adel_pos-20261113T211630Z-a3fceb.dump` (674 917 bytes, catálogo legible).
3. **Ensayo de restauración**: «Restauración completa verificada: 134 tablas con conteos idénticos».
4. **Servidor detenido** de forma ordenada (`server-stop`).
5. **Migración**: `./scripts/dev.sh migrate` → `applied:1`; repetición → `applied:0`.
6. **Servidor 0.9.5 iniciado** (`server-start`): readiness 200; `api/v1/version` → **0.9.5 / esquema 22**,
   `test_mode: false`.
7. **Comprobación posterior** de solo lectura, comparada con la previa: solo cambian las migraciones (1–22), las tablas
   (135: `kitchen_user_stations`), las columnas nuevas (`kitchen_printers.cut_mode` y `cut_feed_lines`,
   `payment_settings.precheck_name` = `precheck`, `devices.kitchen_all_stations` = verdadero y
   `kitchen_sound_alert`, `users.kitchen_all_stations` = verdadero en los 2 usuarios) y las descripciones de
   `accounts.manage` («Editar datos de cuentas») y `kitchen.stations.manage` («Administrar estaciones y las estaciones
   que muestra cada terminal»). Cuentas, unidades, pagos, caja, cocina, inventario, catálogo, plantillas, auditoría y
   ventas: **idénticos**. Ningún dato de operación se borró ni se reescribió. No había datos de prueba que quitar.
8. **Aplicación**: `./scripts/dev.sh build-macos` → `AD-EL POS.app` 0.9.5 (13.56 MiB); `./scripts/dev.sh open-app` → la
   aplicación 0.9.5 quedó abierta y conectada (la 0.9.4 ya no estaba abierta).
9. **Fusión**: `etapa-9f` se fusiona en `main` y se envía a `origin` al cerrar esta sección (resultado en la entrega
   final).
