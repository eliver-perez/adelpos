# Verificación · Etapa 9A

Verificación por partes del **10 de noviembre de 2026**, zona America/Monterrey, antes de cada commit local de la rama
`etapa-9a`: fase 0 `da182ee`; fase A, parte A1 `592fa2a` y parte A2 `5a96f64`; fase B, parte B1 `a2a472c`, verificación
conjunta de las fases A y B1 `1796f31` y parte B2 `f697b02`; fase C, parte C1 `e81aeb0` y parte C2 `793c099`. Versión
**0.9.0**, API v1, esquema **17** (migraciones `0015_catalog_price_and_precheck.sql`, `0016_inventory_suppliers_counter.sql`
y `0017_reports.sql`). Evidencia anterior: [etapa 8](VERIFICATION_STAGE8.md), [etapa 7.1](VERIFICATION_STAGE7_1.md),
[etapa 7](VERIFICATION_STAGE7.md), [etapa 6.2](VERIFICATION_STAGE6_2.md), [etapa 6.1](VERIFICATION_STAGE6_1.md),
[etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md), [etapa 4](VERIFICATION_STAGE4.md),
[etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y [etapa 1](VERIFICATION_STAGE1.md). Avance:
[ETAPA_9A_AVANCE.md](ETAPA_9A_AVANCE.md). Guías: [REPORTS.md](REPORTS.md), [CATALOG.md](CATALOG.md) e
[INVENTORY_AND_PURCHASING.md](INVENTORY_AND_PURCHASING.md).

**Alcance de esta evidencia.** Las cifras son las que cada parte registró en el avance (y, donde el avance no las trae,
en los registros de la parte, como se indica). Hasta la parte C2 se obtuvieron con `config/product.json` todavía en 0.8.0
y esquema 14 (las migraciones nuevas ya estaban embebidas); la versión 0.9.0 y el esquema 17 se fijaron en la fase D, con
las comprobaciones de «Versión 0.9.0» de abajo. **La regresión final de la fase D está pendiente** y se registrará en
[VERIFICATION.md](VERIFICATION.md) («Regresión final»).

**Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432): las migraciones
0015–0017 no se han aplicado a la instalación real. No se leyeron secretos y ninguna prueba envió trabajos a CUPS, a una
impresora, a un cajón ni a una conexión TCP.

## Entorno aislado

Copias del repositorio en el directorio temporal de la sesión, una por parte (sin `.git`, `.local`, `.tools`, cachés,
artefactos ni las especificaciones del propietario), clúster PostgreSQL temporal en el puerto 55499 (solo loopback), una
base `adel_pos_test_*` desechable por paquete, `GOPROXY=off` y Go temporal para Playwright en puertos propios de cada
parte (8184/1424 en A1, 8188/1428 en B1, 8187/1427 en C2), según `CLAUDE.md` §6. Las partes se desarrollaron en paralelo:
algunas copias incluían el trabajo en curso de otra parte, como se indica en el avance. La verificación conjunta de las
fases A y B1 se ejecutó sobre una copia exacta del commit `a2a472c`.

## Resultados ejecutados por parte

| Parte (commit)                         | Estáticas                                     | Go unitarias (`-race`) | Integración PostgreSQL (`-race`)                                                                                                                     | Vitest             | `tsc` y build     | Rust (`cargo test`)      | Playwright (Chromium y WebKit)                           |
| -------------------------------------- | --------------------------------------------- | ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------ | ----------------- | ------------------------ | -------------------------------------------------------- |
| A1 (`592fa2a`)                         | `go vet` con y sin `integration`              | `ok`                   | `catalog` 9.4 s, `database` 5.8 s y `payments` `ok`; `operations` 226.6 s con las 7 subpruebas 9A aprobadas y un fallo ajeno (ver abajo)             | 430 en 50 archivos | Sin errores       | 20 aprobadas, 1 ignorada | 8 de 8 por motor                                         |
| A2 (`5a96f64`)                         | —                                             | —                      | —                                                                                                                                                    | —                  | —                 | —                        | —                                                        |
| B1 (`a2a472c`)                         | `go vet` con y sin `integration`              | `ok`                   | `database` 5.8 s, `administration` 58.7 s, `catalog` 7.1 s, `operations` 220.9 s, `payments` 1.5 s y `reports` 25.2 s: `ok`                          | 434 en 51          | Sin errores       | 21 y 1                   | 8 de 8 por motor                                         |
| Conjunta A + B1 (`1796f31`, `a2a472c`) | Prettier, gofmt, `cargo fmt`, `sync-metadata` | `ok`                   | `database`, `administration`, `catalog`, `operations` (219.0 s), `payments`, `kitchen`, `inventory` y `reports`: `ok`                                | 434 en 51          | Sin errores       | 21 y 1                   | WebKit 8 de 8; Chromium con fallo por tiempo (abajo)     |
| B2 (`f697b02`)                         | `gofmt -l`, `go vet`, `rustfmt --check`       | `ok`                   | `database` 5.3 s, `administration` 55.6 s, `catalog` 7.4 s, `payments` 1.3 s, `operations` 221.7 s, `reports` 91.8 s y `reports/operational` 145.9 s | No aplica          | `tsc` sin errores | 29 y 1                   | No aplica (sin cambios de interfaz)                      |
| C1 (`e81aeb0`)                         | —                                             | `ok` (ver abajo)       | `reports` 97.6 s, `administration` 56.6 s y `operations` 223.1 s: `ok` (registros de la parte)                                                       | —                  | —                 | —                        | Especificación de exportación: 1 de 1 por motor          |
| C2 (`793c099`)                         | —                                             | —                      | —                                                                                                                                                    | 467 en 53          | Sin errores       | —                        | Especificación nueva 1 de 1; completo 10 de 10 por motor |

«—»: la parte no registró esa suite en el avance (no cambiaba esa capa o la cubre otra parte).

- **A1**: la integración de `internal/operations` aprobó las siete subpruebas 9A (`9A_31_*` y `9A_33_*`) y todo lo
  anterior salvo `inventory_settings_feature_and_events` («La hora de inicio del día se cambia en Configuración
  general.»), un cambio en curso de la parte B1 (la hora de inicio se movió a Configuración general), ajeno a A1; una
  ejecución completa previa, antes de ese cambio, aprobó todo `internal/operations` (222.6 s). Con la prueba adaptada en
  B1, la aceptación encadenada aprobó completa en B1, en la verificación conjunta y en B2. Controles negativos: con el
  predicado anterior de la precuenta en `tables.go`, las dos carreras controladas fallan («awaiting_payment»); sin la
  clase nueva de «Poner fuera de servicio», el texto ocupa 3 líneas a 1280 × 720. Una corrida intermedia interrumpida de
  Playwright tuvo en WebKit un error de página «The provided value is non-finite» en `restaurant.spec.ts` que no se
  reprodujo en las dos corridas siguientes.
- **A2**: el avance de la parte no registra la salida de sus suites. Sus pruebas (Go unitarias de proveedores y del código
  sugerido, integración `operations/inventory_stage9a_test.go` y `database/stage9a_inventory_upgrade_test.go`, Vitest
  `inventoryStage9A.test.tsx` y Playwright `workflow-inventory-9a.spec.ts`, descritas en
  [INVENTORY_AND_PURCHASING.md](INVENTORY_AND_PURCHASING.md)) forman parte de las corridas de B1, de la verificación
  conjunta y de B2 (integración) y de C2 (Vitest y Playwright completo). La verificación de la fase A sola (`5a96f64`) no
  fue significativa para la interfaz porque dos pruebas de la fase A ya esperaban la «Hora de inicio del día» de B1; Go,
  la integración y Rust sí aprobaron sobre `5a96f64`.
- **Verificación conjunta**: en Chromium, `restaurant.spec.ts` agotó el límite de 60 s por prueba (dura unos 55 s en un
  equipo libre y el equipo estaba cargado por tres agentes) y las especificaciones siguientes fallaron en cascada porque
  la base ya estaba configurada; el límite pasó a 120 s (`desktop/playwright.config.ts`). Chromium aprobó después 10 de
  10 en la corrida completa de C2.
- **B1**: tras el último cambio de la consulta de diferencias del reconocimiento se repitió `restaurant.spec.ts`
  (Chromium 1/1 y WebKit 1/1). `./internal/reports` se agregó a la tupla de `scripts/database.py`.
- **B2**: `internal/reports` incluyó la aceptación de B1 (que consulta todos los reportes registrados y sus permisos por
  API directa) y las exportaciones de C1 ya presentes en el árbol (25 reportes con columnas protegidas revisados en PDF
  y Excel); `internal/reports/operational` (nuevo, agregado a `scripts/database.py`) incluyó la medición de volumen.
- **C1**: el avance de la parte no registra la salida de sus suites. Sus registros (en el directorio temporal de la
  sesión) muestran las unitarias de Go `ok` (incluido `internal/reports` con las pruebas de `export_test.go`), la
  integración `-race` de `internal/reports` en 97.6 s (`TestReportExports` 70.3 s, `TestReportsAcceptance` 22.6 s,
  `TestReportRunnerIsReadOnlyAndBounded`), `administration` 56.6 s y `operations` 223.1 s, y Playwright
  `workflow-reports-export-9a.spec.ts` 1 de 1 por motor (y 2 de 2 por motor junto con la especificación de C2, tras una
  primera corrida con 2 fallos durante el desarrollo). Las pruebas Rust de `export.rs` corrieron en B2 (29 aprobadas) y
  la interfaz que la usa, en C2.
- **C2**: `pages/reports/reports.test.tsx` con 27 pruebas; Playwright completo con las 10 especificaciones (`inventory`,
  `restaurant`, `stock-consumption`, `table-price-decrease`, `takeaway-quick-notes`, `vendor-purchases`,
  `warehouse-control`, `workflow-inventory-9a`, `workflow-reports-9a` y `workflow-reports-export-9a`); validador de la
  paleta de gráficos (skill dataviz) aprobado en claro y oscuro, con contraste menor de 3:1 en tres tonos del tema claro
  compensado con rótulos visibles y la tabla alternativa; capturas a 1280 × 720 y 1920 × 1080 en ambos temas y motores.
- **Versión 0.9.0 (fase D, documentación)**, en una copia aislada del árbol con la versión nueva: `sync-metadata --check`
  («Metadatos coherentes»), `go vet ./...` sin hallazgos, `go test ./...` con todos los paquetes `ok`, `tsc --noEmit` sin
  errores y `cargo test --locked --offline` con 29 aprobadas y 1 ignorada (la de Keychain real).

## Cobertura de la etapa 9A

Correspondencia con las pruebas obligatorias de §31 de la especificación. Rutas relativas a `backend/internal/` salvo que
se indique otra.

| Prueba exigida (§31)                                                                                      | Dónde y qué                                                                                                                                                                                                                                                                                                                                                                                                                              |
| --------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Precio con piso, opción desactivada y activada, diferencias, límites, extras, promociones, IVA            | `catalog/price_decrease_test.go` (JSON idéntico al anterior en diez escenarios con huellas de `da182ee`, $150/$35/$25 → $140, quitar sin reemplazo, límites de cero y del valor que permanece, la opción nunca sube, extras e ingredientes, modo cantidad, presentaciones, listas, combos, 2×1, porcentaje y precio especial); `operations/stage9a_test.go` (IVA 8 % sobre el importe reducido, promoción y descuento manual posterior). |
| Persistencia histórica de precios                                                                         | `operations/stage9a_test.go` (cambiar la opción no altera la partida; observaciones conservan importes; partida nueva sin la opción); `catalog/acceptance_test.go` (permiso, auditoría, duplicado).                                                                                                                                                                                                                                      |
| Vigencia de la precuenta (§33)                                                                            | `operations/stage9a_test.go` (revisión y ajustes guardados, dos carreras controladas, precuentas históricas con la regla anterior); `database/stage9a_catalog_precheck_upgrade_test.go`.                                                                                                                                                                                                                                                 |
| Compras de mostrador, campos de proveedor, código sugerido, categoría desde el formulario, navegación     | `inventory/suppliers_test.go`; `operations/inventory_stage9a_test.go`; `database/stage9a_inventory_upgrade_test.go`; Vitest `inventoryStage9A.test.tsx`, `productCode.test.ts` y `codeSuggestions.test.tsx`; Playwright `workflow-inventory-9a.spec.ts`.                                                                                                                                                                                 |
| Ventas por periodo, producto y categoría; combos; cuentas divididas y combinadas                          | `reports/reports_integration_test.go` (`TestReportsAcceptance`: combos sin duplicar componentes y cuentas divididas y combinadas sin doble conteo, totales del detalle iguales a los resumidos).                                                                                                                                                                                                                                         |
| Pagos parciales y mixtos, devoluciones, propinas, importes pendientes                                     | `TestReportsAcceptance` (pagos parciales, mixtos, propinas y correcciones; devolución en su propio día; para llevar pagado sin entregar).                                                                                                                                                                                                                                                                                                |
| Descuentos, cortesías y motivos históricos                                                                | `TestReportsAcceptance` (motivo renombrado sin cambiar el reporte, «Sin motivo del catálogo»); `payments/recognition_targets_test.go` (cortesía con venta 0).                                                                                                                                                                                                                                                                            |
| Fechas comerciales y semana ISO                                                                           | `businessday/day_test.go` y `period_test.go` (01:30 con 00:00 y 05:00, cambios de horario, semana 53 y cambio de año); `TestReportsAcceptance` (hora de inicio cambiada por la API sin reescribir registros); Vitest `reports.test.tsx` y `businessDay.test.tsx`.                                                                                                                                                                        |
| Caja: turnos, cortes, cierre, diferencias por moneda, tipo de cambio histórico                            | `reports/operational/operational_test.go` (`TestOperationalReports`: esperado igual al de Caja en pesos y dólares, turno de varios días por turno y por día, cierre con sobrante y cortes, −2.50 MXN y +1 USD con equivalencia 15.50 a 18.00).                                                                                                                                                                                           |
| Esperado protegido, arqueo ciego, reportes denegados                                                      | `TestOperationalReports` (esperado, efectivo físico, diferencias y resultado ausentes de todo el JSON sin `cash.expected.read`; turno en arqueo sin importes y excluido de todo agregado; alcance de Caja; denegaciones por API directa); `TestReportsAcceptance` (permisos de cada reporte por API directa).                                                                                                                            |
| Inventarios: existencias por almacén, kardex, costos, valuación, consumos, mermas, conteos, negativos     | `TestOperationalReports` (existencias negativas y bajo mínimo, por almacén tras transferir y contar, kardex con inicial y final, mermas clasificadas, conteo con diferencia −10, consumo sin contar una unidad dos veces, costos y valuación por permiso).                                                                                                                                                                               |
| Costos provisionales y márgenes                                                                           | `TestOperationalReports` («Sin costo», «Estimado» y firme, paso a firme al procesar el consumo, costo teórico igual al de `GET inventory/recipes/{id}`, ajustes de costo como concepto del periodo).                                                                                                                                                                                                                                     |
| Compras por periodo, recepciones parciales, gasto vinculado sin duplicar                                  | `TestOperationalReports` (orden parcial, directa y mostrador; total 321.44 igual a las recepciones y pagado desde caja 97.44 incluido; ordenado contra recibido y pendiente; devoluciones aparte).                                                                                                                                                                                                                                       |
| Exportación PDF: filtros, totales, paginación, encabezados, monedas, permisos, archivos grandes, acentos  | `reports/export_test.go` (texto extraíble, encabezados repetidos, «Página n de m», totales, acentos y «ñ», carta, A4 y horizontal, 3 000 renglones) y `export_integration_test.go` (`TestReportExports`: igual a la pantalla, filtros, orden y comparación, monedas separadas, datos históricos, 50 000 renglones y el 50 001 rechazado).                                                                                                |
| Exportación Excel: `.xlsx` válido, tipos, fechas, importes, ceros iniciales, fórmulas, totales, precisión | `export_test.go` (sin fórmulas, hipervínculos ni nombres definidos; apóstrofo en los textos que parecen fórmulas; códigos `007` y `000123` como texto; números exactos y formatos por moneda; porcentajes como fracción; precisión de más de 15 dígitos como texto) y `TestReportExports`.                                                                                                                                               |
| Columnas protegidas fuera de PDF y Excel aunque se llame a la API                                         | `TestReportExports` (efectivo físico sin `cash.expected.read` y, en los 25 reportes con columnas protegidas, ninguna columna sin su permiso en ambos formatos); `export_test.go`; Rust `export.rs` (lista cerrada y parámetros).                                                                                                                                                                                                         |
| Decisiones de §0-B: venta al cerrar, «En curso», devolución en su día, reliquidación, sin turnos          | `TestReportsAcceptance` (venta contada al cerrar y abiertas en «En curso», devolución sin alterar el día original y cuenta reabierta con saldo 0, reliquidación con solo diferencias, todos los reportes sin ningún turno).                                                                                                                                                                                                              |
| Reporte de un año con volumen sin bloquear un cobro                                                       | `TestReportsVolumeMeasurement` y `TestOperationalVolume` (opcionales, `ADEL_REPORTS_MEASURE=1`): cobro de 95 ms con el margen de un año en curso; `TestReportsAcceptance` (`REPORTS_BUSY` y cobro sin esperar a un reporte); `runner_integration_test.go`.                                                                                                                                                                               |
| Relleno inicial de la 0017                                                                                | `TestReportsAcceptance` (el bloque de la migración sobre cuentas cerradas por la API, igual a Go por fuente) y `database/stage9a_reports_upgrade_test.go` (desde el esquema 14).                                                                                                                                                                                                                                                         |
| Interfaz de Reportes                                                                                      | Vitest `reports.test.tsx` (27 pruebas) y `reportExport.test.ts`; Playwright `workflow-reports-9a.spec.ts` y `workflow-reports-export-9a.spec.ts`.                                                                                                                                                                                                                                                                                        |
| Regresiones generales                                                                                     | En cada parte, la aceptación encadenada de `internal/operations` (etapas 4–8 y 9A) y las suites de `administration`, `catalog`, `payments` y `database`; Playwright completo en A1, B1 y C2. Compilación del `.app` 0.9.0 e independencia del servidor: pendientes de la regresión final.                                                                                                                                                |

## Mediciones

Detalle en [REPORTS.md](REPORTS.md) («Mediciones») y en el avance (fases B1 y B2). Resumen: con 200 000 renglones de
reconocimiento y 100 000 cobros en un año, los reportes de ventas, cobros y descuentos del año tardan entre 27 y 323 ms y
el resumen 236 ms; con 120 000 unidades, 200 000 movimientos de consumo y 60 000 cobros, los márgenes de un año tardan
alrededor de 0.8 s (p95 de hasta 1 002 ms), el consumo por producto 778 ms (1 065 ms) y el resumen completo 1 049 ms
(1 173 ms); un cobro con el reporte de margen de un año en curso tardó 95 ms (54 ms sin reporte). Ningún reporte se acercó
al límite de 30 s, por lo que no se agregó la 0018. La exportación de 50 000 renglones de `payments.list` tardó 11.1 s en
PDF y 7.7 s en Excel (con `-race`). Mediciones de un solo equipo (Apple M1 Max), sin varias terminales.

## No ejecutado o no verificado

- **Regresión final de la fase D**: pendiente (Prettier, gofmt, `cargo fmt`, Go con `-race`, integración PostgreSQL de
  todos los paquetes, Vitest, build, Rust, Playwright completo en Chromium y WebKit y compilación del `.app` 0.9.0 sobre
  el árbol final).
- **Instalación real**: 0015–0017 no se aplicaron; no se ensayaron todavía el respaldo, la restauración ni la migración
  de la base real con estas migraciones (solo las pruebas de actualización desde el esquema 14 sobre bases simuladas).
- **Ventana nativa Tauri (WKWebView)**, **diálogo nativo real de guardado** y escritura en disco desde la ventana,
  **pantalla táctil física** y **teclado virtual** en Reportes, en la captura con el precio que baja y en Inventarios:
  solo Vitest (jsdom), Playwright con el puente simulado y las pruebas Rust de la escritura atómica con un destino
  simulado.
- **Papel**: ninguna precuenta ni comanda de la 9A se imprimió; los PDF exportados no se imprimieron.
- **Volumen y concurrencia reales**: mediciones de un solo equipo con datos generados; sin varias terminales consultando
  reportes y cobrando a la vez.
- **Keychain real**, **LAN** con otra máquina y **Windows**: no probados.
