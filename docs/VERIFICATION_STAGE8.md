# Verificación · Etapa 8

Verificación por fases del **10 de noviembre de 2026**, zona America/Monterrey, antes de cada commit local de la rama
`etapa-8`: fase 0 `3d63681`; fase A servidor `6d19dcf` e interfaz `45e373e`; fase B servidor de recetas y consumo
`6e063dc`, indicaciones rápidas con efecto `700ec1a`, interfaz `a1d4e74` y cierre `00a79d4`; fase C servidor `b2bccb1`,
interfaz `ec5d8d7` y cierre `4820308`; fase D servidor `ba2be6e`; correcciones de la revisión independiente (fase F del
avance) `343ad77`; fase D interfaz `3a145e9`. Versión **0.8.0**, API v1, esquema **14**
(migraciones `0010_inventory_core.sql` a `0014_inventory_control.sql`). Evidencia anterior: [etapa 7.1](VERIFICATION_STAGE7_1.md),
[etapa 7](VERIFICATION_STAGE7.md), [etapa 6.2](VERIFICATION_STAGE6_2.md), [etapa 6.1](VERIFICATION_STAGE6_1.md),
[etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md), [etapa 4](VERIFICATION_STAGE4.md),
[etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y [etapa 1](VERIFICATION_STAGE1.md). Avance:
[ETAPA_8_AVANCE.md](ETAPA_8_AVANCE.md). Guía: [INVENTORY_AND_PURCHASING.md](INVENTORY_AND_PURCHASING.md).

**Alcance de esta evidencia.** Las cifras son las que cada parte registró en el avance. Hasta el servidor de la fase D se
obtuvieron con `config/product.json` todavía en 0.7.1 y esquema 9 (las migraciones nuevas ya estaban embebidas); la
versión 0.8.0 y el esquema 14 se fijaron en la fase E (`45b7047`), antes de las correcciones de la fase F y de la interfaz
de la fase D. La regresión completa sobre el árbol final se registra en [VERIFICATION.md](VERIFICATION.md) («Regresión
final»).

**Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432): las migraciones
0010–0014 **no se aplicaron** a la instalación real, que sigue en 0.7.1 y esquema 9 hasta la autorización expresa del
propietario. No se leyeron secretos y ninguna prueba envió trabajos a CUPS, a una impresora, a un cajón ni a una conexión
TCP.

## Entorno aislado

Copias del repositorio en el directorio temporal de la sesión, sincronizadas con el árbol de trabajo en cada parte (sin
`.git`, `.local`, `.tools`, cachés, artefactos ni las especificaciones del propietario), clúster PostgreSQL temporal en el
puerto 55499 (solo loopback), bases `adel_pos_test_*` desechables, `GOPROXY=off`, y Go temporal para Playwright en 8183 o,
cuando dos aceptaciones corrieron a la vez, en 8184 y 8185 (`ADEL_BROWSER_SERVER_URL` y `ADEL_BROWSER_PREVIEW_PORT`, desde
la fase B), según `CLAUDE.md` §6. Las verificaciones de cierre de las fases B y C se ejecutaron sobre una copia exacta del
commit de cierre, sin cambios en curso de otras partes. Algunas copias de partes paralelas incluían el trabajo en curso de
otra parte, como se indica en el avance.

Dobles de prueba: un disparador de prueba que hace fallar el consumo de una unidad (fase B) o de una venta (conteos de la
fase D) para probar errores y recuperación; un gasto en dólares insertado directamente en `cash_operations` en la fase C (el
pago en dólares es una función ajena a la prueba); los demás gastos, correcciones, regularizaciones y cierres se hicieron
por la API de caja.

## Resultados ejecutados por fase

| Parte (commit)                        | Estáticas        | Go unitarias (`-race`) | Integración PostgreSQL                | Vitest             | `tsc` y build     | Rust (`cargo test`)      | Playwright (Chromium y WebKit)         |
| ------------------------------------- | ---------------- | ---------------------- | ------------------------------------- | ------------------ | ----------------- | ------------------------ | -------------------------------------- |
| A servidor (`6d19dcf`)                | Aprobadas        | `ok`                   | 5 paquetes `ok`; `operations` 137.1 s | 317 en 39 archivos | Sin errores       | 17 aprobadas, 1 ignorada | No aplica (sin pantallas)              |
| A interfaz (`45e373e`)                | Aprobadas        | `ok`                   | `administration` `ok` (53.8 s)        | 337 en 41          | Sin errores       | 17 y 1                   | 2 especificaciones aprobadas por motor |
| B recetas y consumo (`6e063dc`)       | Archivos propios | `ok`                   | 5 paquetes `ok`; `operations` 165.4 s | —                  | `tsc` sin errores | 18 y 1                   | No aplica                              |
| B indicaciones con efecto (`700ec1a`) | Aprobadas        | `ok`                   | 3 paquetes `ok`; `operations` 157.3 s | 345 en 42          | Sin errores       | 18 y 1                   | 3 aprobadas por motor                  |
| B interfaz (`a1d4e74`)                | Prettier propio  | —                      | —                                     | 365 en 44          | Sin errores       | —                        | 4 aprobadas por motor                  |
| Cierre B (`00a79d4`, sobre `a1d4e74`) | Aprobadas        | `ok`                   | 5 paquetes `ok`                       | 365 en 44          | Sin errores       | 18 y 1                   | 4 de 4 por motor                       |
| C servidor (`b2bccb1`)                | Archivos propios | `ok` (15 paquetes)     | 5 paquetes `ok`; `operations` 186.5 s | —                  | `tsc` sin errores | 19 y 1                   | No aplica                              |
| C interfaz (`ec5d8d7`)                | Prettier propio  | —                      | —                                     | 386 en 45          | Sin errores       | —                        | 5 aprobadas por motor                  |
| Cierre C (`4820308`, sobre `ec5d8d7`) | Aprobadas        | `ok`                   | 5 paquetes `ok`                       | 386 en 45          | Sin errores       | 19 y 1                   | 5 de 5 por motor                       |
| D servidor (`ba2be6e`)                | Archivos propios | `ok`                   | 5 paquetes `ok`; `operations` 216.0 s | —                  | `tsc` sin errores | 20 y 1                   | No aplica                              |
| F correcciones (`343ad77`)            | Archivos propios | `ok`                   | 3 paquetes `ok`; `operations` 220.7 s | —                  | `tsc` sin errores | —                        | No aplica (sin pantallas)              |
| D interfaz (`3a145e9`)                | Prettier propio  | —                      | —                                     | 404 en 46          | Sin errores       | —                        | 6 aprobadas por motor                  |

«—»: la parte no cambiaba esa capa y no ejecutó la suite (la cubre el cierre de su fase o la parte siguiente).

- **Estáticas**: Prettier, gofmt, `cargo fmt --check` (o `rustfmt --check` de `api.rs`), `sync-metadata --check` y `go vet`
  con y sin la etiqueta `integration`. En las partes de servidor desarrolladas en paralelo se comprobaron los archivos
  propios; los cierres de fase, todo el árbol.
- **Integración PostgreSQL**: `database`, `administration`, `catalog`, `operations` y `payments` (la fase B de indicaciones
  ejecutó `catalog`, `operations` y `database`). `operations` encadena `TestRestaurantAcceptance` con las etapas 4–7.1 y las
  pruebas de inventario de las fases ya implementadas, de modo que cada fase repitió las suites anteriores. En la fase D,
  `TestRestaurantAcceptance` tardó 213.1 s con `-race`; el plazo de su contexto (`newFixture`) se amplió de 300 a 600 s en el
  mismo commit. La fase F ejecutó `database` (4.0 s), `administration` (55.4 s) y `operations` (220.7 s), con sus subpruebas
  nuevas. La prueba física opt-in `TestControlledCUPSPrinter` quedó omitida.
- **Prueba de mutación (fase F)**: en otra copia se reintrodujeron los seis defectos de la revisión (`production_in` como
  reversión, el `Restore` anterior, `Admit` a valor cero, valor por insumo con solo `costs.read` y las tres fugas de
  existencias) y fallaron `inventory_cost_visibility`, `inventory_b_entries_with_negative_stock`,
  `inventory_d_counts_with_concurrent_sales`, `inventory_d_alerts`, `inventory_d_purchase_suggestions` e
  `inventory_d_entries_without_cost` (y en cascada `inventory_d_events_audit_and_ledger`); en el motor fallaron los casos de
  reversión, último costo válido, invariante y secuencias pseudoaleatorias. Con las correcciones, todas aprueban.
- **Rust**: la prueba ignorada es la de Keychain real (opt-in). La etapa suma `inventory_routes_are_closed_and_keep_write_methods`,
  `inventory_query_parameters_are_whitelisted`, `inventory_recipe_and_consumption_routes_are_closed`,
  `inventory_purchasing_routes_are_closed` e `inventory_control_routes_are_closed` (81 operaciones `inventory_*`).
- **Playwright**: `inventory.spec.ts` (A, 20.8 s en Chromium y 20.2 s en WebKit), `takeaway-quick-notes.spec.ts` (B),
  `stock-consumption.spec.ts` (B, 27.2 s y 26.5 s sola), `vendor-purchases.spec.ts` (C) y `warehouse-control.spec.ts` (D,
  55.8 s en Chromium y 55.3 s en WebKit sola), junto con `restaurant.spec.ts` (etapas 4–7.1) en la misma base temporal por
  motor. En la fase C el recorrido completo tardó 2.7 min en Chromium y 2.6 min en WebKit; con la interfaz de la fase D, 6
  aprobadas por motor en 3.6 min cada uno. Incidencias intermedias, todas aprobadas al repetir sin cambios en el código
  probado: en la interfaz de la fase B, Chromium perdía un toque porque `restaurant.spec.ts` deja el teclado virtual en
  «siempre» (la especificación nueva lo desactiva por la API); en la de la fase D, WebKit falló una vez al pulsar «Entrar»
  en el inicio de sesión («html intercepts pointer events») y, en Chromium, `vendor-purchases.spec.ts` falló una vez en su
  comprobación de tema antes de una captura y, al reiniciarse Playwright, `warehouse-control.spec.ts` rechazó la base ya
  configurada (la especificación nueva fija el tema hasta que se conserva antes de cada captura).

## Cobertura de la etapa 8

Correspondencia con las pruebas obligatorias de §29 de la especificación. Archivos de integración en
`backend/internal/operations/` salvo que se indique otro.

| Prueba exigida (§29)                                                                             | Dónde y qué                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| ------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Insumos y unidades: crear y modificar, kg/g y l/ml, paquetes, precisión, incompatibles           | `inventory/units_test.go`; `inventory_core_test.go` (duplicados, revisión, auditoría, cambio de unidad base, caja de 24, paquete de 12, costal de 25 kg = 25,000 g, rechazo entre dimensiones).                                                                                                                                                                                                                                                                                                                                                                                                        |
| Recetas: crear, versionar, subrecetas, rendimientos, ciclos, consumos anteriores                 | `inventory_recipes_test.go` (versiones 2 y 3 con la 1 conservada, renglones de solo inserción, rendimiento efectivo con 4 % de merma, ciclos a través del insumo producido); `inventory_consumption_test.go` (receta modificada entre liberación y procesamiento).                                                                                                                                                                                                                                                                                                                                     |
| Personalizaciones: normal, sin, con, extra, sin porción, observación e indicación sin vínculo    | `inventory/resolution_test.go` y `inventory_consumption_test.go` (las seis personalizaciones con el diagnóstico del ingrediente sin porción).                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Indicaciones incompatibles, cambio posterior, partidas antiguas                                  | `catalog/quick_note_effects_test.go` y `quick_note_effects_test.go` (`QUICK_NOTE_CONFLICT`, indicación editada sin cambiar la partida, partida y borrador sin `quick_notes`, huella estable, comanda de la 7.1).                                                                                                                                                                                                                                                                                                                                                                                       |
| Complementos sustituidos, porciones adicionales, combos personalizados                           | `inventory_consumption_test.go` (filete: arroz por pasta a $140, dos porciones de papas, medias porciones, combo en dos estaciones y dos almacenes sin duplicar).                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| Cocina: borrador, cuenta, liberación, estaciones, reintento, reimpresión, división               | `inventory_consumption_test.go` (borrador y cuenta sin consumo, una solicitud por unidad, reintento HTTP idempotente, reproceso sin duplicar, reimpresión con cambio de impresora, división, cambio de mesa y combinación sin consumo).                                                                                                                                                                                                                                                                                                                                                                |
| Modificación, cancelaciones, clasificación al cierre, devolución financiera                      | `inventory_consumption_test.go` (modificación de partida liberada, «No preparado» con reversión exacta, «Preparado / merma», reconciliación antes y después del corte e idempotente, corrección con `adjustments.authorize`, trabajador que al iniciar reconcilia una vencida, devolución financiera sin reintegración).                                                                                                                                                                                                                                                                               |
| Existencias y costos: saldos positivo, cero y negativo, recepción con negativo, cruce            | `inventory/costing_test.go` (casos con cifras exactas); `inventory_consumption_test.go` (queso con saldo 1, 0 y −1); `inventory_purchasing_test.go` (recepción sobre −5 piezas con ajuste de costo de −$50).                                                                                                                                                                                                                                                                                                                                                                                           |
| Promedio ponderado, diferencias de costo, ajustes, transferencias, subreceta sin doble descuento | `inventory/costing_test.go`; `inventory_ledger_test.go`; `inventory_control_test.go` (ajustes y transferencias sin cambio de costo); `inventory_consumption_test.go` (enchiladas con arroz rojo a $2.391579).                                                                                                                                                                                                                                                                                                                                                                                          |
| Compras: orden, recepción total, parcial y repetida, devolución                                  | `inventory_purchasing_test.go` (12 + 8 de 20 cajas con descuento prorrateado, repetición con el mismo `operation_id`, `IDEMPOTENCY_CONFLICT`, exceso, devoluciones con diferencia absorbida y no absorbida).                                                                                                                                                                                                                                                                                                                                                                                           |
| Compra sin pago, gasto sin compra, vinculación opcional, egresos duplicados                      | `inventory_purchasing_test.go` (sin operaciones de caja al crear, emitir o recibir; gasto sin compra; vinculación total y parcial; `CASH_EXPENSE_OVERLINKED`, `RECEIPT_OVERLINKED`, `CASH_LINK_EXISTS`; gasto corregido o regularizado después; turnos cerrados; gasto en dólares).                                                                                                                                                                                                                                                                                                                    |
| Decisiones de §0-B: inventario desactivado y activación                                          | `inventory_consumption_test.go` (función desactivada sin solicitudes ni movimientos; unidad liberada antes de activar modificada y cancelada sin efecto; programado antes de activar y liberado después que consume).                                                                                                                                                                                                                                                                                                                                                                                  |
| Consumo asíncrono, captura sin espera, una solicitud                                             | `inventory_consumption_test.go` (confirmación con el trabajador detenido que termina con la solicitud pendiente; reproceso; receta cambiada antes de procesar) y la medición `inventory_b_measure_release`.                                                                                                                                                                                                                                                                                                                                                                                            |
| Indicación con efecto y personalizador, dos porciones de papas, «solo para llevar»               | `quick_note_effects_test.go`, `catalog/quick_note_effects_test.go` y `inventory_consumption_test.go` (bolsa en para llevar y ventanilla, no en salón).                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| Cancelación sin clasificar al corte, con Go apagado, idempotente                                 | `inventory/operating_day_test.go` (14 casos con adelanto y atraso de horario) e `inventory_consumption_test.go`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| IVA de compras incluido y excluido; cambio sin efecto anterior                                   | `inventory_purchasing_test.go` (recepción directa a $116.00 y a $100.00 tras cambiar la configuración; la anterior conserva `tax_in_cost` y su valor).                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| Costo uno por insumo: transferencias sin cambio de costo                                         | `inventory/costing_test.go`, `inventory_ledger_test.go` e `inventory_control_test.go` (vino a $120 sin cambio).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| Conteos: por almacén, diferencias, ventas concurrentes, idempotente, autorización                | `inventory_control_test.go` (captura a ciegas, también sin el movimiento de cada renglón; consumo fallido que impide aplicar con `COUNT_CONSUMPTION_PENDING`, consumos anteriores procesados y posterior conservado, `COUNT_ALREADY_APPLIED`, 403 sin `counts.approve`, ajustes autorizados por otra persona y sin autoautorización); `inventory_d_entries_without_cost` (`COST_REQUIRED` al aplicar un sobrante sin costo, sin registrar nada y con el conteo aprobado); `inventoryControl.test.tsx` y `warehouse-control.spec.ts` (conteo a ciegas y con diferencias, enviado, aprobado y aplicado). |
| Entradas sin costo y existencias negativas (§0-B.6, fase F)                                      | `inventory/costing_test.go` (elaboración con déficit, reversión con existencia negativa, entrada sin costo, último costo válido e invariantes en 200 secuencias); `inventory_b_entries_with_negative_stock` (salsa elaborada después de venderse, «No preparado» con la crema en negativo); `inventory_d_entries_without_cost` (ajuste pendiente y directo con `COST_REQUIRED` y aplicación tras configurar el costo de referencia).                                                                                                                                                                   |
| Costos confidenciales (§26: existencias no implica costos)                                       | `inventory_core_test.go` (costos ocultos sin `costs.read`); `inventory_cost_visibility` (con `costs.read` sin `valuation.read`: costo promedio, sin valor por insumo ni valuación posterior); `inventory_d_alerts` e `inventory_d_purchase_suggestions` (sin cantidades sin `stock.read`); Vitest de Existencias y Movimientos sin valuación.                                                                                                                                                                                                                                                          |
| Actualización de una instalación existente                                                       | `database/existing_install_test.go` (`TestStage8UpgradeOfExistingInstallation`), `stage8_purchasing_upgrade_test.go` y `stage8_control_upgrade_test.go`: esquema 9 con datos → 0010–0014 una vez, sin cambios en los datos anteriores, 24 permisos solo del administrador, restricciones con nombre y disparadores.                                                                                                                                                                                                                                                                                    |
| Regresión: permisos, auditoría, SSE                                                              | Cada archivo de integración comprueba 403 sin escribir, auditoría en la misma transacción y eventos `inventory.*` sin contenido recibidos con `inventory.read` y ninguno sin él; `permissions_test.go` compara Go con las migraciones.                                                                                                                                                                                                                                                                                                                                                                 |

Recorridos de Playwright (Chromium y WebKit), sin desbordamiento a 1280 × 720 ni 1920 × 1080 en claro y oscuro en cada
pantalla:

- `inventory.spec.ts`: pestañas y resumen; «Harina de prueba» en kilogramos con la presentación «Costal 25 kg» capturada en
  gramos; existencia inicial de 2 costales a $450 (50 kg y $900.00); Existencias (costo promedio $18.00 / kg) y Movimientos
  (+50 kg), comprobados también en Go.
- `takeaway-quick-notes.spec.ts`: filete de $140 con arroz y ensalada: «2 de 2», «Sin cambio», aviso de máximo, − de 44 px,
  arroz → papas en dos toques («+$5.00» y $145.00 de Go), «Queso extra» que marca «Extra queso» y cobra $12.50, «Sin
  cebolla», resumen sin duplicados y borrador con identificadores.
- `stock-consumption.spec.ts`: carne con 5 kg a $180, receta de 150 g (Go: 0.15 kg y $27.00), porción habitual de 50 g,
  «Control de inventario» activado, pedido para llevar enviado a cocina, consumo de −0.15 kg por el proceso de consumo
  (−$27.00), traza, cancelación, «No preparado» y existencia de vuelta en 5 kg según Go.
- `vendor-purchases.spec.ts`: proveedor con RFC normalizado; orden de 20 cajas a $250 con $50 de descuento ($5,742.00);
  recepciones de 12 y 8 (480 pza, dos «Entrada por compra»); recepción directa de 50 vasos ($116.00) y vínculo total con un
  gasto de $116.00 de Caja 1.
- `warehouse-control.spec.ts`: «Harina E2E D» en kg con mínimo de 100 y 20 kg a $18 (por la API); las 13 pestañas en orden,
  sin desplazamiento a 1280 × 720 y de al menos 44 px; transferencia de 5 kg del almacén general a «Barra E2E D» (15 kg y
  5 kg después, $90.00); merma de 1 kg por rotura (14 kg); conteo de la barra con 4.5 kg contados (teórico 5 kg, faltante
  0.5 kg) enviado, aprobado y aplicado, con el `count_out` de −0.5 kg y origen «Conteo físico» en Movimientos; alerta «Bajo
  mínimo» en el Resumen con acceso a Existencias (18.5 kg); Go confirma 18.5 kg y el conteo aplicado; sin errores de página.

Capturas a 1280 × 720 en claro y oscuro (Chromium y WebKit) en el directorio temporal de la sesión, con el tema comprobado
por un píxel de fondo en la fase B. La interfaz de la fase D se comprobó además con Vitest (`inventoryControl.test.tsx`, 18
pruebas: pestañas por permiso, ajustes, mermas, `COST_REQUIRED`, transferencias, conteos a ciegas y con diferencias,
niveles, alertas, sugerencias con y sin existencias, valor por insumo y valuación posterior ocultos sin `valuation.read`, y
ningún `type="number"`).

## Mediciones del consumo asíncrono

Prueba `inventory_b_measure_release` (`ADEL_INVENTORY_MEASURE=1`): 200 confirmaciones con envío a cocina con la función
activada y 200 desactivada, en bloques alternados de 50 tras 5 de calentamiento; se mide solo el `POST …/confirm`.

| Ejecución                   | Desactivada (media / p95 / máx.) | Activada (media / p95 / máx.) | Trabajador                             |
| --------------------------- | -------------------------------- | ----------------------------- | -------------------------------------- |
| `go test -tags=integration` | 6.79 / 7.83 / 9.73 ms            | 7.01 / 7.74 / 8.11 ms         | 205 solicitudes en 1.23 s (6.0 ms c/u) |
| `go test -race`             | 17.58 / 20.70 / 21.76 ms         | 18.03 / 21.22 / 23.04 ms      | 205 en 1.35 s (6.6 ms c/u)             |

La liberación con la función activada agrega ≈ 0.2 ms de media; calcular el consumo dentro de ella costaría ≈ 6 ms por
unidad con el lock 7820148305 tomado, por lo que se conservó el consumo asíncrono. Medido en este equipo, sin concurrencia
real de terminales.

## No ejecutado o no verificado

- **Instalación real**: 0010–0014 no se aplicaron; no se ensayaron todavía el respaldo, la restauración ni la migración de
  la base real con estas migraciones (solo la prueba de actualización sobre una instalación simulada en esquema 9).
- **Ventana nativa Tauri (WKWebView)**, **pantalla táctil física** y **teclado virtual** en Inventarios y en la captura de
  complementos e indicaciones: solo Vitest (jsdom) y Playwright.
- **Papel**: ninguna comanda con indicaciones con efecto se imprimió; solo la prueba de plantilla con el texto exacto.
- **Hardware**: no hay lectores de código de barras ni básculas en el alcance; los códigos de barras solo se buscan como
  texto.
- **Volumen y concurrencia**: sin medición con catálogos, existencias o libros grandes, con muchas terminales capturando o
  contando a la vez, ni de las alertas calculadas en cada consulta; la medición del consumo es de un solo equipo.
- **Keychain real**, **LAN** con otra máquina y **Windows**: no probados.
- **Regresión final de la fase E** sobre el árbol completo: pendiente al redactar este documento (ver
  [VERIFICATION.md](VERIFICATION.md)).
