# Etapa 9H · Avance

Migración del sistema anterior (C#/WinForms con SQL Server 2012) con todo su historial, y pendientes (9F, precio unitario
por renglón y sesiones en varios dispositivos). Especificación: [docs/etapas/ETAPA_9H.md](etapas/ETAPA_9H.md). Rama
`etapa-9h`, creada desde `main` (0.10.0, esquema 23, con la 9G fusionada). Coordinador y como máximo dos agentes a la vez,
cada fase en su árbol de trabajo, probada en copias aisladas (clúster temporal 55499).

**Datos reales**: el respaldo del sistema anterior está fuera del repositorio (`ADEL POS/possys.bak`) y nada extraído de
él entra al repositorio; en `docs/` solo hay conteos y nombres de tablas o columnas, y en este avance, además, cifras
agregadas cuando la especificación las pide (§6).

---

## Fase 0 · Verificación y análisis

Un solo agente, de solo lectura salvo `docs/MIGRACION_ANALISIS.md`. El propietario instaló Docker Desktop durante la fase
(15/11/2026); el coordinador restauró el respaldo en un SQL Server 2022 temporal (edición Express, `--network none`,
respaldo montado de solo lectura; base de compatibilidad 110 = SQL Server 2012, 52 tablas) y el agente lo consultó solo
con consultas agregadas. La memoria de trabajo de Claude Code (§1.1, punto 4) la corrigió el coordinador: tenía las
reglas viejas («nunca push» y «autorizar cada migración») y ahora repite la regla de autonomía de `CLAUDE.md` §0 y la
actualización del servicio con el `.pkg` como parada con el propietario.

### Estado de la 9F

Verificado contra el código de `fd11d89` (= `main` con la 9G) y `docs/ETAPA_9F_AVANCE.md` (fases D y E).

| Punto (§1.1)                                       | Estado                                                | Evidencia                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| -------------------------------------------------- | ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1a. Isotipo de la marca en lugar de la «A»         | Hecho                                                 | `desktop/src/components/BrandLogo.tsx:3`, `:15` (isotipo `desktop/src/assets/adel-pos-isotipo.png`, 128 × 128); `desktop/src/styles.css:8407-8418` (debajo de 1280 px: 40 px, oculta el logotipo completo y muestra el isotipo); sin `.brand-mark` (`desktop/src/stage9fLayout.test.ts:55-58`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| 1b. Contorno de foco del logotipo solo con teclado | Hecho                                                 | `desktop/src/navigation/TopBar.tsx:85-96` (`goHome` suelta el foco si `event.detail > 0`) y `:104`; pruebas en `desktop/src/navigation/navigation.test.tsx` («barra compacta (etapa 9F)», desde `:389`) y `desktop/e2e/workflow-stage9f-topbar-map.spec.ts:333-380`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| 1c. Caja con el estado del turno                   | Hecho                                                 | `desktop/src/pages/cash/CashShortcut.tsx:36-43` (estado corto «Abierto», «En arqueo», «Cerrado», «Consultando…»), `:290-291` (nombre accesible y ayuda emergente), `:304-317` (texto completo y corto); `desktop/src/styles.css:8189-8191`, `:8421-8426`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| 1d. «En línea» con texto o ayuda emergente         | Hecho                                                 | `desktop/src/navigation/ServerStatus.tsx:10-26` (ayuda «<estado>: …» y texto); `desktop/src/styles.css:8224-8233` (texto visible en la barra superior) y `:8431-8438` (debajo de 1150 px solo el punto, con su ayuda)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| 2. Leyenda del mapa sin desplazamiento horizontal  | Hecho                                                 | `desktop/src/styles.css:1714-1755` (`.map-legend` y `.map-legend-states` con `flex-wrap: wrap`, sin `overflow-x`; cada estado con `white-space: nowrap`); `desktop/src/stage9fLayout.test.ts:71-79`; `desktop/e2e/workflow-stage9f-topbar-map.spec.ts:406-433`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| 3. Ruta antigua de anulación                       | Hecho (retirada, no redirigida; decisión D8 de la 9F) | Go: `backend/internal/operations/handler.go:15-17`, `:19-26`, `:31-34` (`410 ROUTE_RETIRED` antes de permisos, lock y lectura; sin escritura); fuera de `routePermission` (`handler.go:168-268`) y de `accountAction` (`accounts_write.go:277-281`); descripción de `accounts.manage` «Editar datos de cuentas» (`backend/migrations/0022_printing_and_kitchen_stations.sql:65-67`, `backend/internal/operations/context.go:63-66`). Rust: sin operación (`desktop/src-tauri/src/api.rs:412-414`) y la prueba `api.rs:2272-2273`. React: ninguna llamada a `restaurant/accounts/{id}/void` (solo existe `payments/accounts/{id}/adjustments/void`, que es otra ruta). Menú nativo (`desktop/src-tauri/src/menu.rs`) y paleta (`desktop/src/commands/actions.ts`): sin la opción. Contrato: `API_CONTRACTS.md:211`, `:224-225`, `:274` y «Etapa 9F · ruta antigua…» (`:3002` en adelante); la ruta nunca estuvo en el contrato de clientes (`backend/internal/apicontract/openapi.json` solo tiene el estado `voided` del filtro, `:755`; `docs/API_V1.md` no la menciona) |
| 4. Memoria de trabajo de Claude Code               | La revisa el coordinador                              | —                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |

**Solo visual**: la 9F verificó la barra y la leyenda con Playwright a 1200, 1280, 1440, 1500 y 1920 px en ambos temas
(Chromium y WebKit 1/1, `docs/ETAPA_9F_AVANCE.md`, fase D). En esta fase no se volvió a ejecutar (no cambió código).
Siguen sin verificar en equipo real: Windows/WebView2 al 125 %, la ventana nativa y una pantalla táctil. Nota: en la Mac,
una ventana de unos 1500 px está por encima de 1280 px y muestra la barra completa, no la compacta; la compacta es la
de Windows al 125 % (≈ 1200 px de página), como decidió D9.

**Nada que hacer en la Fase A para los puntos 1 a 3.**

### Autorización de supervisor en «Cancelar unidades» (la 9F sí la agregó: la Fase A la quita, §2.2)

- **Antes de la 9F** (commit `145640e`, 0.9.4): bastaba el permiso propio `orders.cancel` («Cancelar partidas o cantidades
  confirmadas», `backend/internal/operations/context.go:78`). La ruta `POST restaurant/accounts/{id}/cancel` exigía
  `orders.cancel` (`git show 145640e:backend/internal/operations/handler.go`, línea 213); reducir desde «Modificar»
  (`edit`, ruta con `orders.edit`) exigía además `orders.cancel` si dejaba menos unidades
  (`git show 145640e:backend/internal/operations/amendments.go`, líneas 201-205); la interfaz mostraba «Cancelar unidades»
  solo con `orders.cancel` (`git show 145640e:desktop/src/pages/restaurant/capture/BillLines.tsx`, línea 240). Nadie más
  autorizaba.
- **Hoy**: con `orders.cancel` basta igual; **sin él**, la ruta `cancel` deja entrar con `accounts.read` y `accounts.open`
  («Abrir cuentas») (`backend/internal/operations/handler.go:150-151`, `:157-166`, `:255-257`) y exige la autorización
  puntual de un supervisor con `orders.cancel`, antes de escribir; lo mismo al reducir desde «Modificar».
- **Qué agregó la 9F** (todo lo que la Fase A debe quitar o dejar sin efecto):
  - Go: `backend/internal/operations/supervisor_approval.go:32-38` (`unitsCancelRule`), `:40-64` (`approveWithSupervisor`,
    compartido con «Cancelar cuenta»: se conserva), `:71-88` (`authorizationFields`) y `:90-101` (`auditSupervisor`,
    también compartido); `cancellations.go:9-19`, `:61-66`, `:72-75`, `:90-100` (supervisor, auditoría
    `restaurant.supervisor_authorized` con `scope` `item_cancel` y `authorized_by*` en `item.cancelled`);
    `amendments.go:260` (`cancel_units_requires_authorization` en `edit-preview`), `:300-312` y `:324-334` (supervisor y
    `item_modify`); `handler.go:150-166` y `:255-257` (permiso de la ruta); `models.go:145-148`, `:194-197`, `:219-221`;
    `accounts_read.go:99`. La huella sin contraseña (`handler.go:280-305`) es común a «Cancelar cuenta» y se conserva.
  - React: `desktop/src/pages/restaurant/CancelItem.tsx:20-26`, `:45-47`, `:65`, `:126`;
    `desktop/src/pages/restaurant/capture/CustomizationPanel.tsx:62`, `:70-76`, `:123-125`, `:269`;
    `desktop/src/pages/restaurant/capture/BillLines.tsx:242` (`orders.cancel` o `accounts.open`);
    `desktop/src/services/restaurant.ts:143`, `:244-250`, `:307`, `:453`; `desktop/src/components/restaurant/HistoryDetails.tsx:64-73`
    (`UnitsAuthorization`: conviene conservarlo para los eventos 9F que ya guardaron `authorized_by`); el repetir la
    autorización de «Recuperar resultado» (`desktop/src/pages/restaurant/OperationalPage.tsx:479-486`) es común a las
    rutas con autorización y se conserva.
  - Reportes: «Modificaciones de cantidades» (`products.quantity_changes`, `backend/internal/reports/cancellations.go:183-205`,
    columna «Autorizó» en `:198`, consulta `:224-225`, `:243`, `:254`, `:285`, `:292`, nota `:32-35` y `:302`, descripción
    `:37-40`) y «Productos cancelados» (`products.cancelled`, `backend/internal/reports/products.go:201-236`, columna en
    `:207-210`); «Actividad por empleado» cuenta esas autorizaciones (`backend/internal/reports/employees.go:18`, `:48-50`,
    `:160-162`). Ojo: en «Modificaciones de cantidades», la misma columna muestra también el supervisor de una cuenta
    cancelada (renglones «Cancelación de la cuenta», de la 9D), que sigue siendo verdadero; §2.2 deja elegir entre dejarla
    vacía o quitarla.
  - Contrato: `API_CONTRACTS.md:3002-3060` (rutas, rechazos, eventos y el campo `cancel_units_requires_authorization`). **No**
    está en el contrato de clientes: `openapi.json` no tiene la ruta `cancel` ni el campo, y `docs/API_V1.md` no los
    menciona (la API v1 no incluye cancelar unidades), así que quitarlo no rompe Android.
  - Documentación: `docs/RESTAURANT_OPERATIONS.md:290-312`, `docs/REPORTS.md:37-39`, `:438-444`, `:852-857`, `:953`,
    `CLAUDE.md` §11–12 (último punto de «Reglas permanentes desde la etapa 9F»), §15 («Autorización para cancelar
    unidades (§7.2)») y §21 (Etapa 9F), `AGENTS.md:151`.
  - Pruebas: `backend/internal/operations/stage9f_cancel_units_test.go`, `backend/internal/reports/cancellations_integration_test.go`
    y `stage9d_cancellations_integration_test.go`, `desktop/src/pages/restaurant/cancelItem.test.tsx`,
    `desktop/src/pages/restaurant/capture/capture.test.tsx` (reducción) y `desktop/e2e/workflow-stage9f-cancel-units.spec.ts`.
- **Para decidir en la Fase A** (no cambia §0-B): «basta con el permiso propio» devuelve la regla anterior a la 9F
  (`orders.cancel`); quien solo tiene `accounts.open` deja de poder cancelar unidades (antes de la 9F tampoco podía). Lo
  que se conserva: quién canceló (autor del evento `item.cancelled`/`item.modified` y la auditoría) en el Registro de
  movimientos y en «Modificó» del reporte.

---

### Análisis del sistema anterior

Resultado completo, solo con estructura, correspondencia, conteos y rangos de mes y año, en
[MIGRACION_ANALISIS.md](MIGRACION_ANALISIS.md) (§1 estructura con las diferencias contra `Base de Datos.sql`, §2 conceptos,
§3 IVA, §4 cómo se registraba cada cosa, §5 comprobaciones de los datos, §6 correspondencia y §7 preguntas).

#### Conclusiones para el diseño

- **IVA**: guardado por precio, renglón, complemento y venta, pero el catálogo se capturó al 0 % (365 de 367 precios); sin
  8 %. La tasa del catálogo importado y si se estima el IVA del historial son decisiones del propietario (preguntas 1 y 2).
- **Dólares**: el sistema anterior no los guardaba (efectivo en pesos, sin moneda ni tipo de cambio): «pagos, incluidos
  los dólares con su tipo de cambio» (§4.1) no se puede cumplir con estos datos.
- **Tarjeta**: casi solo desde 2024; antes quedó como efectivo.
- **Cancelación**: `ventas.cancelado` existe en el respaldo pero nunca se escribió; la cancelación es la fila de
  `cancelaciones` (con `pagado = 1`).
- **Corte ↔ venta/pago**: `pagos.corte` (columna del respaldo que el código disponible no escribe) y `ventas.corte` (el
  corte del último pago); 4 254 pagos sin corte entre 02/2022 y 04/2025.
- **Volumen**: 226 170 ventas, 722 400 renglones (1 121 690 unidades), 328 489 complementos y 225 810 pagos.

---

### Notas técnicas para las fases B–D

#### 1. Catálogo de AD-EL POS y cómo encaja el anterior

- **Modelo**: `catalog_products` (`backend/migrations/0003_catalog_and_pricing.sql:45-62`: `kind` producto o combo, `code`
  único obligatorio, `base_price numeric(14,2)` ≥ 0, `image_id`, activo/archivado/pausa) con `tax_rate` 0, 8, 16 o `NULL` =
  tasa general de `payment_settings.tax_rate` (`0007_features_product_tax_and_usd_cash.sql:18-21`; `0006…:6-15`); **los
  precios siempre incluyen IVA** y cada versión de partida guarda su tasa (`docs/CATALOG.md:73-80`;
  `0006…:18-22`). Presentaciones: `catalog_variants` (`0003…:66-77`, precio propio). Ingredientes removibles:
  `catalog_product_ingredients` (`0003…:79-89`). Complementos: `catalog_complements` (`0003…:91-101`, nombre único) dentro
  de grupos `catalog_option_groups` (`0003…:103-113`, `pricing_mode` valor o cantidad, nombre único) con opciones
  `catalog_group_options` (`0003…:115`, `reference_value` y `extra_price` ≥ 0), asignados a productos en
  `catalog_product_groups` y con la **cantidad incluida** en `catalog_group_defaults` (`0003…:144-152`). Combos:
  `catalog_combo_slots`/`options`/`tiers`. Listas de precios `catalog_price_lists` y `catalog_price_entries`
  (`0003…:197-204`) **sin vigencias**: no hay historial de precios (solo auditoría y `catalog_change_log`). «El precio puede
  bajar al cambiar complementos» (`price_can_decrease`, 0015; `docs/CATALOG.md:84-96`). Imágenes PNG/JPEG ≤ 1 MiB en base64
  (`backend/internal/catalog/images.go:104`).
- **Disparadores**: desde la 0023 todo cambio del catálogo registra `catalog_change_log` y sube la versión del catálogo
  (`0023_network_terminals_and_android.sql:161-199`): importar o reemplazar el catálogo es un cambio de catálogo normal
  para las terminales y Android (lo esperado). No hay borrado físico de productos referenciados (`docs/CATALOG.md:13`):
  reemplazar el lote exige borrar antes el historial que los referencia, en la misma transacción.
- **Encaje del anterior**: `categorias` → categorías; `productos` → productos (código generado, p. ej. `SA-<id>`; 3 nombres
  repetidos, que AD-EL POS admite); precio más reciente de `productos_precios.total` → `base_price` (el historial de
  precios con fechas no tiene lugar en el catálogo: va al historial importado); `complementos` + `complementos_precios` +
  `productos_complementos` (13 productos, 6 complementos, 26 relaciones con 1 incluido) → un grupo «Complementos» de modo
  valor con una opción por complemento y la cantidad incluida en `catalog_group_defaults`. Para cobrar igual que el
  anterior: `reference_value` = precio y `extra_price` = 0 en cada opción, y `price_can_decrease` en los productos con
  incluidos si el propietario quiere conservar la bonificación (pregunta 6); sin ella, `extra_price` = precio reproduce
  «(pedido − incluido) × precio» con piso en 0. `promociones`, `servicios`, `recetas`, imágenes, presentaciones y combos:
  sin datos o sin concepto en el anterior. `categorias.cocina` → ruta de cocina solo si ya hay una estación (las
  impresoras no se importan).

#### 2. Historial: ¿tablas operativas o tablas propias?

**Recomendación: tablas propias de historial importado** (p. ej. `legacy_import_batches` y `legacy_accounts`,
`legacy_account_lines`, `legacy_line_complements`, `legacy_quantity_changes`, `legacy_cancellations`, `legacy_discounts`,
`legacy_payments`, `legacy_cash_cuts`, `legacy_cash_withdrawals`, `legacy_price_history`), con `batch_id`, ids y folios
originales, importes `numeric(19,4)` exactos, marcas de incoherencia y llaves foráneas solo hacia lo importado al
catálogo, mesas y usuarios (con los nombres copiados), y **ninguna** llave desde las tablas operativas hacia ellas. Los
reportes las leen junto con las de AD-EL POS. Motivos, con lo que habría que tocar si se guardara en las operativas:

| Dónde guarda AD-EL POS                                                                                       | Invariantes y caminos de escritura que chocan con el historial anterior                                                                                                                                                                                                                                                                                                                                                                                                                                |
| ------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Cuentas `restaurant_accounts` (`0004_restaurant_operations.sql:39-63`)                                       | `folio bigint GENERATED ALWAYS AS IDENTITY` (`:41`): los folios anteriores (1…226 143, 27 repetidos) exigen `OVERRIDING SYSTEM VALUE` y chocarían con la numeración nueva o la adelantarían; `device_id`, `price_list_id`, `opened_by`, `assigned_user_id` obligatorios; estados `open/merged/voided/closed`                                                                                                                                                                                           |
| Pedidos y partidas `restaurant_orders`, `restaurant_item_versions`, `restaurant_item_units` (`0004…:93-143`) | `operation_id` único hacia `restaurant_operations` (idempotencia); `pricing_snapshot`, `quote`, `request` y `quote_line` en el formato del motor de precios (habría que inventarlos); `tax_rate IN (0, 8, 16)` obligatorio por versión (`0006…:20-22`); **una fila por unidad** (1 121 690) con `subtotal ≥ 0`, `total = subtotal − discount` y 2 decimales (`0004…:128-143`): repartir el importe de un renglón entre unidades es recalcular, y los complementos negativos y los 4 decimales no caben |
| Pagos `payments` (`0006_payments_and_refunds.sql:79-102`), `payment_allocations`, `cash_movements`           | `amount > 0` (`:88`; hay 539 pagos en 0 y 9 negativos), `shift_id` y `register_id` obligatorios (`:82-83`; 4 254 pagos sin corte), `operation_id` único, asignación por unidad; método con copia del nombre                                                                                                                                                                                                                                                                                            |
| Turnos `cash_shifts` (`0006…:62-76`, `0008…:11-72`)                                                          | Un turno activo por caja (`cash_shifts_one_active_idx`, `0008…:72`; el anterior tiene 148 pares traslapados en la misma caja); el esperado y el arqueo se calculan con las reglas de AD-EL POS (la «venta» del anterior incluye tarjeta y canceladas)                                                                                                                                                                                                                                                  |
| Retiros `cash_operations` (`0008_cash_shifts_and_reconciliation.sql:166-230`)                                | `amount > 0` (8 retiros en 0 o negativos), autorización obligatoria con modo `self`/`supervisor`, turno obligatorio                                                                                                                                                                                                                                                                                                                                                                                    |
| Cancelaciones `account_cancellations` (`0021_account_cancellation.sql:33-69`)                                | Motivo del catálogo y `cancelled_by` obligatorios (el anterior no guardaba quién canceló ni motivo); **solo inserción** por disparador (`:60-69`)                                                                                                                                                                                                                                                                                                                                                      |
| Descuentos y cargos `account_adjustments` (`0006…:153-175`)                                                  | `value` y `amount` > 0, `operation_id` único, el cargo exige tasa de IVA                                                                                                                                                                                                                                                                                                                                                                                                                               |
| Ventas reconocidas `sales_recognitions` (`0017_reports.sql:29-127`)                                          | **Solo inserción, sin `TRUNCATE`** (`:116-127`): un lote importado ahí no se podría reemplazar (§4.3) sin quitar los disparadores; `net = base + tax` y `tax_rate IN (0, 8, 16)` con 2 decimales; llaves a unidades y versiones                                                                                                                                                                                                                                                                        |
| Caminos que escribirían sobre cuentas importadas                                                             | `operations.Handle` (mover, modificar, cancelar, dividir, combinar), `payments.Handle` (cobrar, devolver; `payments/collect.go:666-679` **reabre** una cuenta cerrada si una devolución deja saldo), cocina, `restaurant/state`, pedidos sin conexión, reportes en curso y Caja: cada uno necesitaría una guarda «origen anterior → 409» y pruebas                                                                                                                                                     |

Con tablas propias, el historial es de solo lectura por construcción (no hay ruta que lo escriba), el lote se reemplaza
con `DELETE … WHERE batch_id` en una transacción, los importes se conservan exactos y la operación, la caja, el
inventario, la cocina, el tiempo real y Android no lo ven. El costo es que los reportes deben leerlo (punto 3) y que, si
se quiere ver el detalle de una venta anterior, hace falta una vista de solo lectura propia.

Folios: los del anterior se guardan como texto y se muestran con su marca («Sistema anterior · 012345»); no consumen
`restaurant_accounts.folio` ni las identidades de inventario (`0013…:97`, `:197`, `:288`; `0014…:24`, `:185`, `:248`).

Formas de pago: `payment_methods` es un catálogo propio (`0006…:38-59`: `cash`, `debit`, `credit`, `transfer`,
`platforms`). Efectivo → `cash`; la «Tarjeta Credito/Debito» del anterior no tiene uno solo (pregunta 7). Dólares: AD-EL
POS los guarda por pago (`payments.received_usd`, `usd_rate`, `0007…:42-60`) y por turno (`cash_shifts.usd_rate`), pero
el anterior no tiene ese dato.

#### 3. Reportes que deberían incluir el historial importado

Registro en `backend/internal/reports` (cada `register(Definition{ID: …})`). Los «de ventas, productos, formas de pago,
por día y por turno»:

| Clave                                                                                                                                                                                                                                                                                                                                                           | Archivo                              | Fuente hoy                                                                                                                                          |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `sales.summary`, `sales.by_day`, `sales.by_week`, `sales.by_month`, `sales.by_hour`, `sales.by_category`, `sales.by_product`, `sales.by_mode`, `sales.dine_in`, `sales.takeaway`, `sales.accounts`                                                                                                                                                              | `sales.go`                           | `app.sales_recognitions r` con `ledgerMeasures` y `ledgerConditions` (`sales.go:21-48`, `:95-109`)                                                  |
| `sales.by_shift`                                                                                                                                                                                                                                                                                                                                                | `sales.go:444-476`                   | El libro con `cash_shifts` y `registers` («Sin turno» si no hay turno)                                                                              |
| `products.top`, `products.bottom`, `products.trends`, `products.complements`                                                                                                                                                                                                                                                                                    | `products.go`                        | El libro (y `restaurant_item_versions.quote_line` para complementos)                                                                                |
| `payments.by_method`, `payments.by_day`, `payments.by_shift`, `payments.by_user`, `payments.by_register`, `payments.list`                                                                                                                                                                                                                                       | `payments.go`, `payments_details.go` | CTE `paymentMovements` sobre `payments` ∪ `payment_refunds` (`payments.go:24-35`) con `paymentScope` (turnos en arqueo y alcance de Caja, `:57-62`) |
| Recomendables: `sales.voided_accounts`, `sales.cancellations_by_reason`, `products.quantity_changes` (`cancellations.go`), `products.cancelled` (`products.go`), `cash.shifts`, `cash.sales_collections_by_shift` (`cash.go`), `discounts.general`/`summary` (`discounts.go`), `sales.evolution` (`sales_progress.go`) y el dashboard (`dashboard_sections.go`) | —                                    | Cancelaciones, eventos de partidas, turnos, ajustes                                                                                                 |

Fuera: variantes, listas de precios, costos, inventario, cocina, compras y el detalle de caja (sin equivalente).

Cómo se construyen: los de ventas, desde el libro `sales_recognitions` filtrado por instantes `[desde, hasta)` del
periodo con la hora de inicio del día comercial (`internal/businessday`, `inPeriod("r.recognized_at")`) y agrupado por
`businessDate`; periodos `range` de hasta 400 días y clase `year` (`backend/internal/businessday/period.go:17-25`), así que
comparar años es posible; hasta 100 000 renglones por reporte (`backend/internal/reports/params.go:24-25`); el ejecutor
tiene 2 conexiones y 30 s por consulta (CLAUDE.md §3). **Trabajo estimado**: una vista o una tabla «libro del historial»
con las mismas columnas de medida del libro (`quantity`, `gross`, `discount`, `charge`, `net`, `base`, `tax`, fecha de
reconocimiento, modalidad, mesa, categoría, producto, turno) más `source`, llenada al importar (un renglón por
renglón del anterior, uno por descuento, uno por cargo extra y, según la pregunta 12, uno por diferencia), y en cada
reporte cambiar `FROM app.sales_recognitions r` por la unión con `source` y agregar el filtro; en cobros, una rama más en
`paymentMovements` (con el nombre del método copiado) y la exclusión de `paymentScope` para lo importado; en turnos, la
rama de los cortes importados. **Choque de nombres**: ya existe el filtro `origin` «Origen (para llevar)»
(`backend/internal/reports/filters.go:72-74`), así que el nuevo necesita otra clave (p. ej. `data_source`) y otro texto
(p. ej. «Datos: AD-EL POS / Sistema anterior»). Medir el rendimiento con el volumen real (hay `volume_integration_test.go`).

#### 4. Usuarios, roles y permisos

- `users.password_hash` admite `NULL` (`0002_identity_and_administration.sql:21-33`); con `NULL`, el inicio de sesión
  responde como credenciales incorrectas (`backend/internal/administration/auth.go:42-56`) y el autorizador de supervisor
  tampoco lo acepta (`administration/supervisor.go:42`). **No existe un estado «sin acceso» explícito** (solo `active` e
  `initial_password`) **ni PIN**. Crear un usuario por la ruta exige contraseña (`administration/users.go:75-79`) y **el
  administrador no puede asignar la contraseña de otro** (`users.go:127-129`: «El cambio de contraseña se realiza desde el
  formulario personal»): un usuario importado sin contraseña no podría entrar nunca. Hace falta una función nueva
  («Asignar contraseña» para usuarios sin contraseña, con `initial_password = true`), pregunta 9.
- Nombre de usuario: `^[a-z][a-z0-9_.-]{1,39}$` (`administration/service.go:151`); `sa` reservado (`0002…:32`); los 7 del
  anterior son compatibles en minúsculas.
- Roles predeterminados (`administration/permissions.go:83-95`): `administrator`, `supervisor`, `cashier`, `waiter`,
  `kitchen`, `inventory`, `finance` y `support` (reservado). Correspondencia propuesta en `MIGRACION_ANALISIS.md` §6.4.

#### 5. Áreas y mesas, clientes, inventario, proveedores y compras

- `restaurant_areas` (`0004…:2-10`) y `restaurant_tables` (`0004…:12-31`: nombre ≤ 60 único por área, `seats` 1–40,
  posición en una cuadrícula de 1000 × 560 con mesas de 64–240 × 56–180): las 24 mesas caben en una cuadrícula de 100 × 72.
- **Clientes**: AD-EL POS no tiene tabla de clientes (la cuenta guarda `customer_name` y `phone`, `0004…:39-63`); el
  anterior tampoco (`ventas.nombre` es el nombre de la cuenta).
- **Inventario**: insumos `inventory_items`, unidades `inventory_units` (semillas `0010_inventory_core.sql:80`),
  almacenes (`:176`, uno predeterminado), existencias y el libro `inventory_movements` **de solo inserción** (`:303-319`).
  Sí hay existencia inicial (`backend/internal/inventory/initial.go:10-19`, hasta 500 renglones, autorizada como ajuste,
  `inventory/handler.go:95`), pero un movimiento importado no se podría reemplazar al repetir el lote. El anterior tiene 1
  artículo con 1 existencia y nada más: recomendación, no importar (pregunta 11).
- **Proveedores y compras**: `inventory_suppliers`, órdenes y recepciones (`0013_inventory_purchasing.sql`, recepciones de
  solo inserción `:419-428`); el anterior no tiene datos.

#### 6. «Instalación sin operación propia» (§4.3)

- **Tablas que indican operación** (cualquier fila que no sea de un lote importado): `restaurant_accounts`,
  `restaurant_orders`, `restaurant_item_units`, `restaurant_draft_items`, `payments`, `payment_refunds`,
  `account_adjustments`, `account_cancellations`, `sales_recognitions`, `takeaway_deliveries`, `cash_shifts`,
  `cash_operations`, `cash_movements`, `cash_counts`, `kitchen_unit_releases`, `inventory_movements`,
  `inventory_receipts`, `inventory_adjustments`, `inventory_counts`, `inventory_transfers`, `inventory_purchase_orders`,
  `inventory_productions` e `inventory_supplier_returns`.
- **Lo que una instalación nueva crea sola y no cuenta**: semillas de las migraciones (lista de precios «Restaurante»
  `0003…:241`; `restaurant_settings` `0004…:37`; `kitchen_settings` `0005…:16`; `payment_settings` y `payment_methods`
  `0006…:15`, `:54-59`; `usd_cash` `0007…:16`; denominaciones y categorías de gasto `0008…:89`, `:154`; motivos de
  descuento `0009…:41`; inventario `0010…:56`, `:80`, `:176`; plantillas `0018…:105`; motivos de cancelación `0021…:22`);
  permisos y roles de `ensureFoundations` (`administration/permissions.go:76-100`); el asistente inicial
  (`administration/setup.go:100-150`: establecimiento, límites de descuento, el usuario `admin`, sus preferencias, la caja
  «Caja 1», la terminal y la auditoría `setup.completed`); y registros técnicos (sesiones, terminales, emparejamiento,
  `audit_events`, `realtime_events`, `backup_runs`, `login_attempts`, trabajos de impresión de prueba). Los datos de las
  pruebas solo existen en bases `adel_pos_test_*` (CLAUDE.md §6; firmas en `backend/internal/maintenance/signatures.go`).
- Si el destino ya tiene catálogo, mesas o usuarios propios (sin operación), los nombres únicos (categorías,
  complementos, grupos, mesas por área, usuarios) pueden chocar: pregunta 13.

#### 7. Trabajos en segundo plano y subida de archivos

- **No hay un trabajo largo genérico con avance**. Patrones existentes: (a) **diferido tras la autenticación** dentro de
  la misma petición con plazo ampliado (`administration/http.go:84-107`; `completeDeferred` en
  `administration/reports.go:63-90`): «Quitar datos de prueba» (120 s, `administration/maintenance.go:29`), «Respaldar
  ahora» (espera hasta 2 min, `administration/backups.go:297`), reportes (40 s) y exportaciones (110 s,
  `administration/reports.go:24-28`); (b) **trabajadores** arrancados en `backend/cmd/ad-el-pos-server/main.go:293-347`
  (cocina, inventario, «Agotado», respaldos programados, limpieza de emparejamientos) con su estado en una tabla (p. ej.
  `backup_runs`, estados `running/succeeded/failed`, `0023…`) que la interfaz consulta. Ninguno informa porcentaje.
  `ReadTimeout`/`WriteTimeout` de 15 s (`main.go:255-256`; `lanlistener/lanlistener.go:235-236`).
- **Subida**: todo es JSON por la lista cerrada de `api.rs`; cuerpo máximo 64 KiB, 2 MiB en `catalog/`, `restaurant/`,
  `kitchen/`, `inventory/` y el logotipo (`administration/http.go:48-62`; logotipo e imágenes en base64, ≤ 1 MiB). No hay
  subida de archivos grandes ni multipart. Los diálogos de archivo solo existen en Rust (`tauri-plugin-dialog`, para
  exportaciones y la carpeta de respaldos). En macOS el servicio no puede leer `~/Documents`, `~/Downloads` ni
  `~/Desktop` sin «Acceso total al disco» (TCC, `docs/ETAPA_9G_AVANCE.md:123`), así que pasarle a Go una ruta elegida por
  el usuario no es confiable.
- **Propuesta**: tabla de corridas de importación (estado, fase, contadores, error, lote) y un trabajador como el de
  respaldos; rutas solo locales (`Request.Local`) y solo en la computadora principal para subir el paquete por partes
  (p. ej. 1 MiB por petición, con SHA-256 por parte y del total, a una carpeta de trabajo dentro de la carpeta de datos),
  iniciar y consultar el avance; la importación en su propia conexión y transacción (con `COPY` de pgx), tomando
  7820148305 → 7820148304 exclusivos solo para el reemplazo final y un lock nuevo (p. ej. 7820148310) para que no corran
  dos importaciones. Antes de importar, el respaldo verificado ya existe (`maintenance` y `backupschedule`:
  `DumpBackup`/`standalone.DumpFile` + verificación). El lector («Preparar paquete», con Docker) es un comando aparte: hay
  que decidir quién lo lanza (Rust lanzaría un proceso, algo que hoy solo hace en Windows con el servidor; o el
  propietario desde la línea de comandos).

#### 8. Contradicciones de la especificación con el código o los datos

1. §4.1 «pagos, incluidos los dólares con su tipo de cambio»: el anterior no guardaba dólares ni tipo de cambio (§4.5 del
   análisis).
2. §4.1 «Usuarios … sin acceso hasta que el administrador les asigne una contraseña o un PIN»: AD-EL POS no tiene PIN y el
   administrador no puede asignar la contraseña de otro usuario (punto 4).
3. §4.1 «Turnos, cortes, egresos y retiros» y §4.4 «Total de egresos»: no hay egresos (`salidas` y `entradas` vacías);
   solo retiros (`cortes_parciales`).
4. §4.1 «Cada producto conserva su tasa de IVA según lo que se encuentre» y §4.2 «IVA estimado»: lo que se encuentra es
   0 % en casi todo y el IVA sí está guardado (0); aplicar §4.1 al pie de la letra deja el catálogo nuevo al 0 %.
5. §4.1 «presentaciones … y combos», «Clientes, si existían», «Inventario, si existía»: el anterior no tiene
   presentaciones, combos ni clientes, y su inventario está prácticamente vacío.
6. §4.2 «Origen: Sistema anterior / AD-EL POS»: el nombre «Origen» ya lo usa un filtro (punto 3).
7. §4.3 «el lote anterior se reemplaza completo»: imposible si el historial va a `sales_recognitions`,
   `account_cancellations` o `inventory_movements` (solo inserción): refuerza las tablas propias.
8. §5 «menú Archivo › Migrar del sistema anterior…»: en Windows no hay menú nativo (CLAUDE.md §3), así que allí solo
   queda Administración › Mantenimiento (y la paleta).
9. §1.2 punto 5 «productos borrados con ventas»: imposible en el anterior (llave foránea sin cascada); en su lugar hay 9
   renglones sin un precio anterior a su venta.

#### 9. Preguntas para el propietario

Las 13 de `MIGRACION_ANALISIS.md` §7 (tasa del catálogo; IVA del historial; fecha de reconocimiento; canceladas; retiros;
bonificación de complementos; tarjeta; pagos repetidos y en cero; asignar contraseña; rol de los administradores;
inventario; ventas que no cuadran; instalación de destino). Las que afectan dinero: 1, 2, 3, 4, 5, 8 y 12.

### Decisiones del propietario (15/11/2026, sobre las preguntas de la Fase 0)

Preguntas 1, 6, 12 y 13 de [MIGRACION_ANALISIS.md](MIGRACION_ANALISIS.md) §7:

1. **Tasa de IVA del catálogo importado**: cada producto entra con **la tasa que tenía en el sistema anterior** (la de
   su precio más reciente: 0 % en casi todos y 16 % en uno), como dice §4.1. El historial conserva su IVA tal como se
   guardó.
2. **Ventas cuyo total no cuadra con sus renglones**: los reportes por venta, día, turno y forma de pago usan **el total
   que guardó el sistema anterior**; los de productos usan los renglones, y **la diferencia aparece como un concepto
   aparte**, «Diferencia del sistema anterior», marcada en el informe.
3. **Complementos incluidos que se quitan**: en los productos importados que incluyen complementos se **activa «El
   precio puede bajar al cambiar complementos»**, para cobrar igual que el sistema anterior.
4. **Instalación de destino**: además de no tener operación propia, debe estar **limpia de catálogo, mesas y usuarios
   propios** (salvo lo que crea el asistente inicial); si los hay, el asistente lo explica y no importa. Cada repetición
   reemplaza completo lo importado.

### Decisiones del coordinador (reglas vigentes del propietario; ninguna cambia la especificación)

- **Fecha de reconocimiento** de una venta importada (pregunta 3): la de su **último pago**, como AD-EL POS reconoce la
  venta al cerrar la cuenta (decisión del propietario de la 9A, CLAUDE.md §21); la fecha de apertura se conserva.
- **Ventas canceladas** (pregunta 4): entran como **canceladas que no son venta** (regla de la 9D: una cuenta cancelada
  nunca cuenta como venta), aunque el «Registro de ventas» y los cortes del anterior las contaran; el informe muestra
  aparte el total al estilo del anterior para poder compararlo.
- **IVA del historial** (pregunta 2): se conserva tal como se guardó (§4.2: los importes nunca se recalculan). El «IVA
  estimado» solo se aplicaría a un IVA que no esté guardado; en el respaldo real las columnas de IVA son obligatorias,
  así que no ocurre.
- **Retiros, pagos repetidos y pagos en cero o negativos** (preguntas 5 y 8): se importan **tal como estaban, con su
  marca y su nota**, cuentan donde los contaba el sistema anterior y se listan en el informe (§4.2, «Datos
  incoherentes»). Los retiros son retiros de efectivo del turno, no gastos.
- **Tarjeta** (pregunta 7): forma de pago propia del historial, «Tarjeta (sistema anterior)», sin crear un método nuevo
  en Cobros.
- **Dólares** (§4.1): el sistema anterior no los guardaba (quedaban como efectivo en pesos): no hay nada que importar y el
  informe lo dice.
- **Usuarios** (preguntas 9 y 10): entran sin contraseña y sin acceso, con el rol de la correspondencia del análisis
  (los de puesto «Administrador», como Administrador); se agrega **«Asignar contraseña»** para los usuarios sin
  contraseña (contraseña inicial que el usuario cambia al entrar), porque AD-EL POS no tiene PIN y hoy el administrador
  no puede asignar la contraseña de otro (§4.1 exige que el administrador pueda darles acceso).
- **Inventario** (pregunta 11): **no se importa** (un solo artículo, sin recetas ni compras; su libro de movimientos es de
  solo inserción y no se podría reemplazar al repetir la migración).
- **Historial en tablas propias** del historial importado (recomendación de la Fase 0, nota 2), de solo lectura por
  construcción y con un disparador que impide modificarlas fuera de la importación; el filtro nuevo de los reportes no se
  llama «Origen» (ya existe «Origen (para llevar)»).

### Qué cambió en la fase

- Nuevo `docs/MIGRACION_ANALISIS.md` (estructura, diferencias con el script, conceptos, IVA, cómo se registraba,
  comprobaciones con conteos, correspondencia y preguntas). Sin cambios de código.

### Decisiones

- Ninguna decisión del propietario. Recomendación técnica (para el coordinador): historial en tablas propias.

### Pruebas ejecutadas

- `ISO_TAG=Z $SP/iso.sh $SP/wt/p0 fmt-check` (Prettier de todo el árbol y `gofmt`): aprobado («All matched files use
  Prettier code style!»), después de formatear `docs/MIGRACION_ANALISIS.md` con Prettier (solo tablas).
- Consultas agregadas al respaldo restaurado (`$SP/legacy-sql.sh sql`): catálogo de SQL Server (tablas, columnas,
  llaves, índices, disparadores, vistas y procedimientos), conteos por tabla y las comprobaciones de §5 del análisis.
- `docs/etapas/*.md` sin cambios (md5 antes y después).

### No verificado

- La barra y la leyenda en equipo real (Windows/WebView2 al 125 %, ventana nativa, pantalla táctil); no se repitió
  Playwright.
- El tamaño del paquete y los tiempos (Fase B/E); el rendimiento de los reportes con el volumen importado.

### Para CLAUDE.md

- §21: el respaldo real tiene 52 tablas (4 más que `Base de Datos.sql` y columnas nuevas en `ventas`, `pagos`, `cortes` e
  `impresoras`); el catálogo se capturó con IVA 0 %; los dólares, las propinas y (antes de 2024) la tarjeta no quedaban
  registrados; `ventas.cancelado` nunca se usó; el «Registro de ventas» y los cortes contaban las canceladas.

---

## Fase A · Pendientes y ajustes

Un agente, en su propio árbol de trabajo (`etapa-9h-a`, desde `etapa-9h` en `fd11d89`), en paralelo con la Fase 0
(solo documentación). 15/11/2026. Migración `0024_legacy_migration.sql` con su sección A (índice de sesiones) y
**esquema 24** (`config/product.json` y `node scripts/sync-metadata.mjs`); la versión del producto sigue en 0.10.0 (la
0.11.0 es de la Fase F). Pruebas en copias aisladas: clúster temporal 55499 (una base `adel_pos_test_a_*` por paquete),
Go temporal de Playwright en 8184 y vista previa en 1422. **No se tocó la instalación real.**

Commits en `etapa-9h-a`:

- `de66512` — «Cancelar unidades» y reducir cantidades sin autorización de supervisor (§2.2).
- `94fc263` — opción «Mostrar precio unitario» del diseñador (§2.3) y referencias de la 9F en `AGENTS.md` y
  `docs/REPORTS.md`.
- `f6aab2d` — una sesión activa por usuario y por terminal; 0024 sección A; esquema 24 (§2.4).
- `8afef7a` — Playwright `workflow-stage9h-adjustments.spec.ts`.

### §2.1 · Lo que faltaba de la 9F

Nada: la Fase 0 encontró hechas la barra compacta, la leyenda del mapa y la ruta antigua de anulación, y el coordinador
corrigió la memoria de trabajo. Sin commit propio.

### Qué cambió

**§2.2 · «Cancelar unidades» sin autorización de supervisor** (decisión del propietario)

- **Qué permiso basta ahora**: el propio, «Cancelar partidas o cantidades confirmadas» (`orders.cancel`), para «Cancelar
  unidades» (`POST restaurant/accounts/{id}/cancel`) y para reducir la cantidad de una partida enviada desde
  «Modificar» (`edit-preview` y `edit`). Sin él, la interfaz no ofrece la acción («Cancelar unidades» no aparece en el
  menú del producto; en «Modificar» el «−» no baja de la cantidad enviada) y Go la rechaza: la ruta `cancel` vuelve a
  exigir `orders.cancel` (`403 PERMISSION_DENIED` antes de leer el cuerpo) y `calculateEdit` exige `orders.cancel` si la
  cantidad baja (`403 PERMISSION_DENIED`, «Reducir la cantidad de un producto enviado a cocina requiere el permiso
  «Cancelar partidas o cantidades confirmadas».», ya en la previsualización). No hay camino de supervisor.
- **Qué pasaba antes de la 9F**: exactamente eso (la ruta exigía `orders.cancel` y reducir también; la interfaz lo
  mostraba solo con el permiso). La 9F agregó que, sin él, procediera con la autorización de un supervisor (la ruta
  dejaba entrar con `accounts.read` + `accounts.open`); la 9H lo retira.
- **Go** (`operations`): fuera `unitsCancelRule`, `authorizationFields` y `auditSupervisor` de `supervisor_approval.go`
  (que queda solo para «Cancelar cuenta», con `approveWithSupervisor` y `approveCancellation`), `requireUnitsCancelRoute`
  de `handler.go`, `Authorization` de `cancel` y de `EditInput`, `Reduces` de `editCalculation` y
  `cancel_units_requires_authorization` de `Account` y `QuoteResult`. Los eventos `item.cancelled`/`item.modified` nuevos
  ya no llevan `authorized_by*` ni se audita `restaurant.supervisor_authorized` (`item_cancel`/`item_modify`); siguen
  registrando quién lo hizo (autor del evento y de la auditoría `restaurant.item.cancelled`/`modified`) y el motivo. La
  huella sin contraseña (`withoutAuthorizationPasswords`) no cambió.
- **React**: `CancelItem.tsx` sin formulario de supervisor; `BillLines.tsx` ofrece «Cancelar unidades» solo con
  `orders.cancel` (la 9F también con `accounts.open`); `CustomizationPanel.tsx` sin autorización y con mínimo = cantidad
  enviada sin `orders.cancel` (`canReduce` desde `CaptureScreen.tsx`); tipos de `services/restaurant.ts` sin el campo ni
  `authorization`. Se conservan `UnitsAuthorization` de `HistoryDetails.tsx` (muestra los eventos de la 9F que guardaron
  quién autorizó) y la repetición de la autorización de «Recuperar resultado» (`OperationalPage.tsx`, común a «Cancelar
  cuenta»).
- **Reportes** (decisión del coordinador): «Autorizó» **se conserva** en «Modificaciones de cantidades» y «Productos
  cancelados»: vacía en lo nuevo, con el supervisor en los registros de la 9F y con quien autorizó cancelar la cuenta en
  las partidas de una cuenta cancelada (9D). «Actividad por empleado» sigue contando las autorizaciones de la 9F (ninguna
  nueva). Nota y descripciones de la columna actualizadas. Sin datos ni columnas borrados de la base.
- **`cancel_units_requires_authorization`** no estaba en el contrato v1 (`openapi.json`, `docs/API_V1.md`): se retiró.
  `API_CONTRACTS.md` marca la sección de la 9F como retirada y agrega «Etapa 9H · “Cancelar unidades” sin autorización de
  supervisor».
- **«Cancelar cuenta» (9D) no cambia**: conserva su autorización de supervisor.

**§2.3 · Precio unitario por renglón** (opción del bloque, decisión del propietario)

- Opción `show_unit_price` «Mostrar precio unitario» de la lista de productos (`ProductsOptions`, sustituye a
  `NotesOptions`), **desactivada por omisión**, solo en la precuenta y el ticket de pago (los documentos con renglones
  con importe; la comanda y los avisos de cocina no la tienen y la lista de cocina rechaza la clave). El catálogo de Go la
  entrega con su ayuda; el editor la muestra sin cambios (control genérico) y la estructura dice «Lista de productos ·
  Con precio unitario» (`blockLabels.ts`).
- **Precio comercial de la unidad**: el que ya calcula el motor del catálogo (base o variante más complementos e
  ingredientes extra), antes de promociones y descuentos, tal como Go lo guardó en las unidades
  (`restaurant_item_units.subtotal`): `payments.FinanceLine.UnitPrice` (`json:"-"`, fuera de la respuesta de cobros) →
  `DocumentLine.unit_price` (guardado con el documento, omitido si falta) → `CommercialLine.UnitPrice`. Si las unidades de
  una partida no comparten un precio, no se muestra. El importe del renglón no cambia (con una promoción, «3 × $150.00»
  puede ir con un importe menor). Se expresa con `moneytext`; React no calcula nada.
- **Impresión** (`renderProductLine`): «3 × $150.00 Tacos … $450.00» en el mismo renglón si cabe completo junto al
  importe en el ancho del bloque; si no, el renglón de siempre («3 x Tacos … $450.00», cortado como antes) y debajo
  « 3 × $150.00» (sangría de las observaciones), antes de la nota. Vale para 42/48 y 30/32 columnas y para el doble
  ancho. Misma IR en vista previa, muestra, impresión, reimpresión y «Imprimir con la plantilla vigente».
- **Predeterminadas idénticas byte a byte**: la clave lleva `omitempty`, así que mientras está apagada no aparece en la
  forma canónica: el JSON de las diez predeterminadas (el que siembra la 0018) y lo que imprimen no cambian, y las
  plantillas guardadas sin la opción siguen válidas con los mismos bytes. Los datos de muestra traen el precio unitario
  (sin efecto con la opción apagada).

**§2.4 · Sesiones en varios dispositivos** (decisión del propietario)

- `issueSession` recibe el alcance: **iniciar sesión** cierra solo la sesión vigente del usuario **en la misma
  terminal** (`replaced`); **cambiar la contraseña** sigue cerrando las de todas las terminales (`password_changed`).
  Cerrar sesión, revocar la terminal, desactivar al usuario y el restablecimiento de soporte no cambian.
- **Base**: 0024 sección A sustituye `sessions_one_active_user_idx` (único por usuario, de la 0002) por
  `sessions_one_active_user_device_idx` (único por `user_id, device_id` con `revoked_at IS NULL`); no toca filas.
- `SESSION_REPLACED` dice «Tu sesión se cerró porque el usuario inició sesión de nuevo en esta terminal.».
- `devices/sessions-summary` ya contaba sesiones (sin cambio de código, comentario aclarado) y el «usuario actual» de
  Sistema › Terminales ya era por terminal.
- **Revisado, sin cambios**: turnos de caja y arqueo son de la caja física de la terminal (`payments` usa
  `devices.register_id`; el único índice por usuario era el de sesiones; `cash_shifts_one_active_idx` es por caja),
  permisos leídos en cada petición de cada sesión, SSE limitado a 2 conexiones por terminal, `restaurant/state` con la
  terminal y el usuario de la sesión, estaciones visibles por terminal ∩ usuario.
- Contrato: descripción de `login` en `openapi.json`, `docs/API_V1.md` (§4) y `API_CONTRACTS.md` («Etapa 9H ·
  sesiones en varios dispositivos»). Sin operaciones ni esquemas nuevos.

### Decisiones tomadas en la fase

1. **`authorization` deja de ser un campo** de `cancel`, `edit` y `edit-preview` (decodificación estricta: enviarlo es
   `400 OPERATION_VALIDATION`). Un cliente 0.10.0 con el permiso propio nunca lo envía (el formulario solo aparecía sin
   él) y sin el permiso el `403` llega antes; un 0.10.0 sin el permiso que toque «Cancelar unidades» recibe `403
PERMISSION_DENIED` con su mensaje. Por eso **no** se subieron `min_client_version` ni `min_server_version`.
2. **`auditSupervisor` y `authorizationFields` se quitaron**: solo los usaban las unidades («Cancelar cuenta» audita su
   autorización directamente en `account_cancellation.go` y guarda quién autorizó en `account_cancellations`). La nota de
   la Fase 0 los daba como compartidos; el compilador lo confirma.
3. **La reducción se rechaza ya en `edit-preview`** (como antes de la 9F), con un mensaje propio que nombra el permiso;
   la interfaz no la ofrece.
4. **Precio unitario = subtotal de la unidad** (no el `unit_price` del JSON de la versión): es lo que Go guardó y cobró,
   también después de una modificación que conserva precios; coincide con el `unit_price` del motor en los demás casos.
5. **Formato**: «N × $P Nombre» en un renglón (el «×» del ejemplo de la especificación; CP850 lo tiene, como «Producto × 2»
   de cocina) y, si no cabe, el renglón de siempre con «N x» más « N × $P» debajo.
6. **Cambiar la contraseña sigue cerrando todas las sesiones** del usuario (la especificación solo cambia el inicio de
   sesión; es lo seguro).

### Pruebas ejecutadas (aisladas)

- `go-unit`: aprobado (`go vet` normal e `integration`, `go test -race ./...`, `GOOS=windows` build y vet normal e
  `integration`).
- Integración (`iso.sh go-int`):
  - después de §2.2: `./internal/operations` aprobado (312 s, incluye `TestStage9HCancelUnitsWithoutAuthorization` y
    `TestStage9FRetiredVoid`) y `./internal/reports` aprobado (124 s);
  - después de §2.3: `./internal/designer` aprobado (22 s); `./internal/operations` con `TestRestaurantAcceptance` (la
    cadena con los documentos de la 9B, reimpresión idéntica incluida) y `TestStage9HUnitPriceInCommercialDocuments`
    aprobado (245 s);
  - después de §2.4: `TestStage9HSameUserOnTwoTerminals` aprobado; `./internal/database` (10 s, con
    `TestStage9HUpgrade` 23 → 24 y la de la 9G ajustada), `./internal/apicontract` (8 s, `TestContractV1`) y
    `./internal/administration` (117 s, `TestStageTwoAcceptance` con `14_new_login_replaces_only_the_session_of_the_same_terminal`)
    aprobados;
  - al final: `./internal/payments` (3 s), `./internal/operations` completo (330 s) y `./internal/reports` (125 s), aprobados.
- Unitarias nuevas: `documents/unit_price_test.go` (predeterminadas idénticas por SHA-256 del JSON canónico y de texto
  y ESC/POS a 42/48/30/32 calculados antes del cambio; plantillas sin la opción; catálogo; renglones con y sin espacio a
  42, 48, 30 y 32 columnas; doble ancho).
- Vitest: `tsc --noEmit` y 94 archivos, 884 pruebas aprobadas (`cancelItem.test.tsx` y el caso de reducción de
  `capture.test.tsx` reescritos; caso nuevo sin `orders.cancel`; `designer.test.tsx` con «Mostrar precio unitario»).
- Playwright Chromium: `workflow-stage9h-adjustments` 3 de 3; junto con `workflow-stage9b-designer`,
  `workflow-stage9f-cancel-units` (reescrita: sin el permiso no se ofrece, «Modificar» no baja y Go responde 403),
  `workflow-stage9g-terminals` y `workflow-stage9g-two-terminals`: 9 de 9. WebKit (pantalla sin bloquear):
  `workflow-stage9f-cancel-units` y `workflow-stage9h-adjustments`, 4 de 4.
- `fmt-check` y `sync-check` aprobados; `docs/etapas/*.md` sin cambios (md5). Sin cambios de Rust (no se corrió
  `cargo`).

### No verificado

- El «×» y los renglones del precio unitario en papel real (CP850 0x9E en una impresora física), y 48/32 columnas en
  papel (siguen como prueba física pendiente desde la 9C).
- Dos dispositivos reales a la vez por la red local (las dos terminales de Playwright son dos credenciales en la misma
  Mac) y un teléfono Android; Windows.
- La regresión completa (Fase F).

### Para CLAUDE.md

- Encabezado y §13–14: esquema **24**, migración `0024_legacy_migration.sql` (una para la etapa, sección A: índice de
  sesiones por usuario y terminal).
- §3, «Composición del backend»: `restaurant/*` recibe el autorizador de supervisor solo para «Cancelar cuenta»; §4, la
  línea de `internal/operations/`: `supervisor_approval.go` es la autorización de «Cancelar cuenta» (la 9F la compartía
  con «Cancelar unidades»).
- §11–12, «Reglas permanentes desde la etapa 9D»: quitar `cancel_units_requires_authorization` de la lista de lo que
  calcula Go. «Reglas permanentes desde la etapa 9F»: sustituir el último punto por «**«Cancelar unidades» y reducir
  una cantidad enviada solo con el permiso propio `orders.cancel`, sin autorización de supervisor** (etapa 9H): sin él no
  se ofrecen y Go las rechaza; queda registrado quién lo hizo. «Cancelar cuenta» conserva su autorización».
- §15, «Etapa 9F» › «Autorización para cancelar unidades»: anotar «retirada en la etapa 9H (decisión del propietario)»;
  nueva entrada **Etapa 9H** con: (1) cancelar o reducir unidades sin supervisor, «Autorizó» conservada (vacía en lo
  nuevo); (2) **precio unitario**: opción `show_unit_price` de la lista de productos (precuenta y ticket), desactivada
  por omisión y omitida de la forma canónica (predeterminadas idénticas byte a byte), precio de Go antes de promociones y
  descuentos (subtotal de la unidad), en el mismo renglón o debajo del nombre; (3) **sesiones por dispositivo**: una por
  usuario y terminal, iniciar sesión solo cierra la de esa terminal, cambiar la contraseña cierra todas, el conteo por
  tipo cuenta sesiones.
- §19, «Una sesión activa por usuario (D9)»: resuelto en la 9H (una por usuario y por terminal); las licencias por
  sesiones simultáneas siguen para la etapa 10.
- §20, punto 3: quitar «o, desde la 9F, `orders.cancel`» de los permisos que puede sustituir un supervisor; punto 6: quitar
  `cancel_units_requires_authorization` de la lista.
- §6, «Pruebas seguras»: integración nueva `TestStage9HCancelUnitsWithoutAuthorization`,
  `TestStage9HUnitPriceInCommercialDocuments` y `TestStage9HSameUserOnTwoTerminals` (`operations`), `TestStage9HUpgrade`
  (`database`, 23 → 24; las pruebas de subida anteriores, también la de la 9G, migran hasta la última); la de la 9F se
  llama ahora `TestStage9FRetiredVoid`; Playwright `workflow-stage9h-adjustments.spec.ts` (sus datos: «Antojitos 9H-A»,
  «Sopes 9H-A», «Patio 9H-A», «Mesa H1», la terminal «Comedor 9H-A» y la plantilla «Ticket con precio unitario 9H», sin
  firmas: no son impresoras, estaciones ni productos de las listas de `maintenance/signatures.go`).
- Regla permanente propuesta: «Una opción nueva de un bloque de documento que no deba cambiar las plantillas existentes
  lleva `omitempty` y su valor por omisión en falso, y una prueba con el SHA-256 de las predeterminadas».

---

## Fase B · Lectura del respaldo y paquete de migración

Un agente (15–16/11/2026), en su árbol de trabajo `etapa-9h-b` (desde `etapa-9h`, `2d214f1`), en paralelo con la Fase A.
Pruebas de Go con `iso.sh` (`ISO_TAG=B`, `TMPDIR` dentro del directorio temporal de la sesión) y, para Docker, contenedores
propios con datos sintéticos (etiquetas `local.adel.pos.legacy-reader=1` del lector y `local.adel.pos.legacy-test=1` de
la base sintética de la prueba). **Nunca se usó el respaldo real** (la prueba con él es de la Fase E).

Commits: `7c9ead6` (formato `legacypkg` y comprobación de datos reales en el índice), `133db77` (base y paquete
sintéticos `legacysynth`) y `9762ba8` (lector `legacyread`, subcomandos `legacy-prepare` y `legacy-synthetic` y
`docs/MIGRACION_FORMATO.md`).

### Qué cambió

**Go**

- `internal/legacypkg` (paquete nuevo, neutral: sin Docker ni SQL Server; lo usará también el importador de la Fase C):
  - Formato `.adelmig` v1 (estructura completa en [MIGRACION_FORMATO.md](MIGRACION_FORMATO.md)): encabezado en claro con
    la firma `ADELMIG\0`, la versión (uint16), un JSON corto de parámetros (Argon2id con sal de 32 bytes: 3 pasadas,
    64 MiB, 4 hilos; AES-256-GCM; bloque de 1 MiB; prefijo de nonce; valor de comprobación de la llave =
    HMAC-SHA-256 con una subllave HKDF) y su SHA-256; cuerpo cifrado por bloques (nonce = prefijo ‖ número de bloque ‖
    bandera de último bloque; AAD = SHA-256 del encabezado); texto claro = gzip de un tar con `manifest.json` primero y
    `tables/<tabla>.json` (arreglo de objetos).
  - `manifest.json`: formato y versión, fecha de creación, versión de AD-EL POS y herramienta, datos del respaldo
    (archivo, tamaño, SHA-256, base, fechas del respaldo, versión de SQL Server de origen, compatibilidad,
    intercalación), digest e identificador de la imagen y versión del lector, rango de fechas de las ventas, por tabla
    (nombre, archivo, filas, bytes, SHA-256, llave primaria, columnas con su tipo, columnas omitidas con sus filas con
    valor), tablas omitidas con su conteo y la **huella de contenido** (SHA-256 sobre nombre, filas y SHA-256 de cada
    tabla en orden; solo depende de los datos).
  - `Writer` en flujo: cada tabla se prepara en un archivo temporal de una carpeta privada **cifrado con una llave
    aleatoria que solo vive en memoria** (el manifiesto va primero pero sus huellas se conocen al final; los datos reales
    nunca quedan en claro en el disco) y `Finish` arma el paquete. `CreateOutput`: temporal privado junto al destino con
    `fileacl.CreatePrivate` y nombre definitivo **sin reemplazar nunca** un archivo existente.
  - `Open`/`Reader`/`TableReader` en flujo con iterador de tablas y filas (valores `nil`, `bool`, `string` y
    `json.Number`), `Verify` e `Inspect` (solo el encabezado). Errores tipificados con mensaje en español:
    `not_package`, `unsupported_version`, `wrong_passphrase`, `damaged` y `limit`. Límites: Argon2 (≤ 256 MiB, ≤ 10
    pasadas, ≤ 16 hilos), bloque (4 KiB–8 MiB), total descomprimido (16 GiB), manifiesto (16 MiB), tabla (8 GiB), 1000
    tablas, 1024 columnas y 4 MiB por fila; nombres del tar validados y en el orden del manifiesto.
  - `policy.go`: las reglas de lo que no viaja, compartidas por el lector, los paquetes sintéticos y el importador
    (`IsPasswordColumn`, `TreatmentFor`, `ExcludedTableReason`: `sysdiagrams` y `personal_detalles`).
- `internal/legacyread` (paquete nuevo, el **lector**; solo lo importa `cmd/ad-el-pos-server/legacy.go`, lo comprueba
  `imports_test.go`): busca `docker` (PATH, `~/.docker/bin`, `/usr/local/bin`, `/opt/homebrew/bin`, la aplicación de
  Docker Desktop y, en Windows, `%ProgramFiles%\Docker\Docker\resources\bin\docker.exe`), comprueba que esté en marcha
  y con contenedores de Linux, descarga la imagen si falta (con avance), limpia los restos con su etiqueta, crea el
  volumen y el contenedor (`--network none`, `--pull never`, imagen por su identificador, contraseña de `sa` solo por
  el entorno, respaldo `readonly`; copia privada si Docker no puede montar la carpeta), espera a SQL Server, lee
  `RESTORE HEADERONLY` y `FILELISTONLY`, restaura con `MOVE` y `STATS` (avance), deja la base de solo lectura, enumera
  todas las tablas del catálogo y las extrae por páginas `FOR JSON`, y escribe el paquete. Siempre elimina el
  contenedor, el volumen, la copia, las tablas en preparación y el paquete a medias (también con error, Ctrl-C/SIGTERM,
  cierre de la entrada estándar con `--watch-stdin`, salida cerrada o pánico).
- `internal/legacysynth` (paquete nuevo): la base sintética con **las 52 tablas** del respaldo (mismas columnas, tipos y
  llaves que `MIGRACION_ANALISIS.md`) y datos inventados con sus casos: IVA por producto (casi todo al 0 %, un producto
  al 16 % desde 11/2025, complementos al 16 % hasta 07/2023), complementos con importe negativo y filas idénticas,
  descuentos, cargos extra (también sin descripción o sin importe), cancelaciones, divisas, 8 cortes (abiertos,
  traslapado, sin ventas, con «venta» que no cuadra), retiros (en cero, negativo y de $1 500 000), salidas, pagos
  partidos, repetidos, de menos, en cero y negativos, tarjeta con folio (y uno repetido), pagos sin corte, ventas
  abiertas, sin renglones y con totales que no cuadran, folio repetido, renglón sin precio anterior, renglones en
  cantidad 0 y agregados después, contraseñas e imágenes inventadas y textos difíciles (acentos, comillas, saltos de
  línea, `|`, caracteres de control, barra invertida, espacios al final). `TSQL` crea la base en un SQL Server;
  `WritePackage`/`Build` escriben, sin Docker, el mismo paquete que `legacy-prepare` produce de su respaldo (mismo
  orden de filas que SQL Server: `order.go`). 240 ventas, 503 renglones, 364 complementos, 242 pagos.
- `cmd/ad-el-pos-server/legacy.go` y `main.go`: `legacy-prepare` y `legacy-synthetic` se atienden antes que los demás
  comandos (tienen su propia salida en líneas JSON, sin la bitácora del servidor).

**Subcomandos**

- `ad-el-pos-server legacy-prepare --backup <respaldo.bak> --output <paquete.adelmig> [--watch-stdin]`: frase de paso
  **solo por la entrada estándar** (primera línea; en una terminal, dos veces sin mostrarla; mínimo 8 caracteres,
  normalizada a NFC). Avance en líneas JSON: `step` (`image`, `start`, `restore`, `read`, `write`, `cleanup`;
  `started`/`finished` con segundos), `progress` (capas de la imagen o porcentaje de la restauración), `table`
  (`reading`/`done`/`skipped`, `done`/`total` de tablas y filas), `result` (resumen: paquete, tamaño, huella, respaldo,
  rango de ventas, conteos por tabla, omitidas y tiempos por paso) y `error` (`code`, `message` sin tecnicismos,
  `detail` técnico sin datos). Códigos: `USAGE`, `PASSPHRASE_INVALID`, `BACKUP_UNREADABLE`, `BACKUP_INVALID`,
  `OUTPUT_EXISTS`, `OUTPUT_INVALID`, `DOCKER_MISSING` («Instálalo desde docker.com (es gratuito), ábrelo y vuelve a
  intentarlo»), `DOCKER_NOT_RUNNING`, `DOCKER_UNSUPPORTED`, `IMAGE_UNAVAILABLE`, `READER_START_FAILED`,
  `READER_MEMORY`, `RESTORE_FAILED`, `READ_FAILED`, `WRITE_FAILED`, `CANCELLED` e `INTERNAL`. Sale con 0 o 1.
- `ADEL_TEST_MODE=1 ad-el-pos-server legacy-synthetic --output <paquete.adelmig>` (solo pruebas; frase por la entrada
  estándar): el paquete sintético, sin Docker, para el asistente en Playwright (Fase D).

**Datos reales en el repositorio**

- `.gitignore`: `*.bak`, `*.adelmig` y `legacy-data/`.
- `scripts/check-legacy-data.sh` (y `checkLegacyData` en `scripts/windows/dev.mjs`, también como comando
  `check-legacy-data`): falla si el índice de git tiene un archivo con esos nombres o un archivo **binario** cuya primera
  línea empiece con la firma del paquete (`ADELMIG`) o la de un respaldo de SQL Server (`TAPE`, MTF), aunque tenga otro
  nombre; sin `.git` lo dice y termina bien. Lo ejecutan `dev.sh format-check` (y por tanto `dev.sh test`) y el
  `formatCheck` de Windows. Tarda 0,2 s sobre el repositorio.

**Documentación**: `docs/MIGRACION_FORMATO.md` (solo estructura: archivo, llaves, contenido, manifiesto, lo que no viaja,
límites, escritura y el subcomando con sus eventos y códigos).

### Decisiones

1. **`-h -1` y `-y 0` no se pueden combinar** (sqlcmd responde «The -h and the -y 0 options are mutually exclusive»).
   Con `-y 0` sqlcmd ya no imprime encabezados, así que las páginas JSON se leen solo con `-y 0` (sin `-W`). Lo
   comprobé en el contenedor: una página de 3 MB sale entera en una línea, sin cortes ni ajuste de ancho. `RESTORE
HEADERONLY`/`FILELISTONLY` se leen con encabezados, `-W -k2` y el separador ASCII `\x1f` (un nombre de respaldo con
   `|` no rompe la lectura).
2. **Lo que entrega `FOR JSON`** (comprobado en SQL Server 2022 con compatibilidad 110): `money` con 4 decimales
   (`150.0000`, `-0.0001`, `922337203685477.5807`), `numeric(p,s)` con sus `s` decimales, texto con `\n`, `\t` y
   `\u0001`, y `datetime` **sin la fracción cuando los milisegundos son cero** (`…T12:34:56`). El lector deja siempre
   `AAAA-MM-DDTHH:MM:SS.mmm` y valida el tipo de cada valor; los números se leen con `UseNumber` y nunca pasan por
   `float`.
3. **Técnica de extracción** (mejor que páginas por rangos de la llave, sin dependencias): una sesión de sqlcmd por tabla
   copia la tabla a `#adel_rows` con `ROW_NUMBER()` sobre el orden determinista y un índice agrupado, entrega una línea
   con el total y los conteos de las columnas omitidas y después una línea `FOR JSON` por página (5000 filas; un solo
   `nvarchar(max)` por página). Así se ordena una sola vez, también en las tablas sin llave primaria
   (`ventas_complementos`), sin `OFFSET` que reordene en cada página. Las columnas van como `c0`, `c1`… en el JSON (un
   nombre con puntos cambiaría la forma de `FOR JSON PATH`).
4. **Orden determinista**: por la llave primaria; sin llave (o con una columna de la llave que no viaja), por todas las
   columnas, las de texto con `COLLATE Latin1_General_BIN2` y su `DATALENGTH` (con la intercalación del sistema
   anterior, que no distingue mayúsculas, y con los espacios del final, que SQL Server ignora al comparar, dos filas
   distintas podían empatar y salir en otro orden entre páginas). Las filas que empatan son idénticas, así que la huella
   de contenido es la misma en cada preparación del mismo respaldo.
5. **Contraseñas**: `<columna>_present` = `CAST(CASE WHEN DATALENGTH(columna) > 0 THEN 1 ELSE 0 END AS bit)` en lugar de
   `<> ''` (sirve también para columnas binarias o `text`); una contraseña de solo espacios cuenta como presente. La
   regla de contraseñas va antes que la de binarios (una columna binaria de contraseña también se sustituye).
6. **`sysdiagrams`** se enumera aunque SQL Server la marque como de la herramienta (`is_ms_shipped`), para registrarla
   como omitida con su conteo.
7. **Destino**: el temporal se renombra sin reemplazar (`renamex_np` con `RENAME_EXCL` en macOS, `renameat2` con
   `RENAME_NOREPLACE` en Linux, `MoveFileEx` sin `MOVEFILE_REPLACE_EXISTING` en Windows; en una USB sin esas operaciones,
   enlace duro o comprobación previa), porque `os.Rename` reemplaza un archivo que haya aparecido mientras tanto.
8. **Imagen**: se ejecuta por su identificador con `--pull never`, así el digest del manifiesto es el de la imagen usada.
   Al montar, Docker Desktop responde «bind source path does not exist» para una carpeta fuera de sus carpetas
   compartidas: con ese error (o si el tamaño del respaldo dentro del contenedor no coincide) se usa la copia privada.
   Comprobé que un respaldo 0600 en una carpeta 0700 se lee dentro del contenedor (Docker Desktop asigna el dueño al
   usuario `mssql`) y que el montaje es de solo lectura (escribir falla con «Read-only file system»).
9. **Respaldo con varios juegos**: se restaura el respaldo completo (`BackupType = 1`) más reciente del archivo; la base
   restaurada queda de solo lectura. Antes de arrancar Docker se comprueba que el archivo empiece con `TAPE` (si no,
   `BACKUP_INVALID` en segundos). El SHA-256 del `.bak` queda en el manifiesto (`backup_sha256`).
10. **Mensajes sin datos**: el detalle de un error de SQL Server lleva solo su número («error 3241 de SQL Server»), nunca
    su texto (puede citar valores del respaldo). La frase de paso nunca aparece en argumentos, eventos ni mensajes (lo
    comprueban las pruebas).
11. **Cancelación desde la aplicación** (para la Fase D): además de SIGTERM, `--watch-stdin` cancela al cerrarse la
    entrada estándar (en Windows no hay SIGTERM; matar el proceso deja el contenedor, que se borra al empezar la
    siguiente preparación). Si la salida estándar se cierra, la preparación se cancela y limpia (en macOS SIGPIPE ya no
    mata el proceso).
12. **Pasos**: la comprobación de Docker va antes del paso `image` (sin evento de paso; sus errores son
    `DOCKER_MISSING`, `DOCKER_NOT_RUNNING` y `DOCKER_UNSUPPORTED`). Agregué el evento `progress` y los códigos
    `DOCKER_UNSUPPORTED`, `READER_MEMORY` (SQL Server pide 2 GB) y `BACKUP_INVALID`.
13. **Paquete sintético = paquete de Docker**: `legacysynth` reproduce el orden de SQL Server (BIN2 en la página 1252,
    espacios del final, `DATALENGTH`, nulos primero). La prueba con Docker comprueba que el paquete de `legacy-prepare`
    tiene **la misma huella de contenido** que el de `legacysynth.Build`: la Fase C puede probar el importador con
    paquetes sintéticos sin Docker sabiendo que tienen la forma exacta de uno real.
14. **`legacy-synthetic` viaja en el binario** (como sugiere el encargo), pero solo corre con `ADEL_TEST_MODE=1`; no toca
    ninguna carpeta de datos, solo escribe `--output`.
15. **Sin dependencias nuevas**: `golang.org/x/crypto/argon2`, `golang.org/x/text/unicode/norm` y
    `encoding/charmap`, `golang.org/x/sys` y `golang.org/x/term` ya estaban en `go.mod`; HKDF es `crypto/hkdf` de la
    biblioteca estándar.

### Pruebas ejecutadas

| Prueba                                                                                                                                                                                                 | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ISO_TAG=B iso.sh … go-unit` (vet normal e `integration`, `go test -race ./...`, `GOOS=windows` build y vet)                                                                                           | Aprobado (dos veces; la última con el código final)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `go vet -tags 'integration legacydocker'` de los tres paquetes, en macOS y con `GOOS=windows`                                                                                                          | Aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `legacypkg` (unitarias)                                                                                                                                                                                | Aprobadas: ida y vuelta con 5000 filas y textos difíciles, huella independiente del cifrado, NFC, frase incorrecta, 13 daños (byte del JSON y del SHA-256 del encabezado, de un bloque, de la etiqueta final, truncados, último bloque quitado o duplicado, bloques intercambiados o quitados, datos de más), versión más nueva y 0, no es un paquete (también un respaldo MTF), límites de encabezado (Argon2 de 4 GiB, tiempo, hilos, bloque de 1 GiB), tar manipulado (`../`, ruta absoluta, enlace, orden, entradas de más o de menos, conteo, SHA-256, columnas, valor anidado, JSON inválido, huella, manifiesto inválido), bomba de descompresión (3 MiB de espacios en menos de 64 KiB), tabla, fila, número de tablas y manifiesto enormes, texto claro de más después del gzip, temporal cifrado y borrado, destino que nunca se reemplaza y la comprobación de datos reales |
| Comprobación de datos reales (`repocheck_test.go`, con un índice de git temporal)                                                                                                                      | Aprobada con `check-legacy-data.sh` y con `dev.mjs`: falla con un paquete real escrito por `Writer` llamado `.bin`, con un respaldo MTF llamado `.dat`, con `Notas.BAK`, con `*.adelmig` y con `legacy-data/`; pasa con textos que solo se parecen y sin `.git`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `legacysynth` (unitarias)                                                                                                                                                                              | Aprobadas: 52 tablas, cada valor cabe en su columna (página 1252, longitudes, decimales, milisegundos 0/3/7, nulos), casos presentes, paquete sintético sin contraseñas ni datos personales, orden como SQL Server, script T-SQL                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `legacyread` (unitarias, docker simulado servido con el paquete sintético)                                                                                                                             | Aprobadas: paquete con la huella del sintético; argumentos exactos de `docker run` (`--network none`, etiqueta, montaje `readonly`, `-e MSSQL_SA_PASSWORD` sin valor; la contraseña solo en el entorno y en ninguna otra orden); limpieza de restos solo por la etiqueta; limpieza en éxito, error de restauración, página dañada, cancelación a media lectura y pánico; respaldo montado como copia privada; Docker detenido, con contenedores de Windows o sin memoria; arranque lento; imagen descargada con avance; contraseñas nunca seleccionadas; binarios y tablas excluidas; análisis de la salida de sqlcmd; CLI (frase por la entrada estándar, frase corta o ausente, argumentos inválidos y `--passphrase` rechazado, terminal con dos frases distintas, `--watch-stdin`, salida cerrada); búsqueda de docker; solo el subcomando importa el lector                       |
| **Docker real con el respaldo sintético** (`ADEL_TEST_DOCKER=1 go test -tags 'integration legacydocker' -run TestLegacyPrepareWithDocker ./internal/legacyread`)                                       | Aprobada dos veces (una corrida anterior falló solo porque mi contenedor de experimentos llevaba la etiqueta de prueba; lo eliminé), 57,5 s: base sintética creada y respaldada (`.bak` de 6,1 MB) en 8,4 s; **`legacy-prepare` (binario compilado) en 24,1 s** (arranque 7,0 s, restauración 1,3 s, lectura de 52 tablas 14,2 s, escritura 0,09 s, limpieza 0,17 s), paquete de 32 885 bytes con **la misma huella de contenido** que el sintético y sin contraseñas ni datos personales; con páginas de 7 filas, 23,9 s y la misma huella; al terminar, ningún contenedor ni volumen con las dos etiquetas                                                                                                                                                                                                                                                                           |
| Medición de volumen (sintética, fuera de la suite): 4 tablas con la forma y los conteos de las grandes del respaldo (722 400 renglones, 328 489 complementos sin llave, 226 170 ventas, 225 810 pagos) | `.bak` de 114 MB; `legacy-prepare` en 22–23 s (arranque 7,0 s, restauración 1,6 s, **lectura de 1 502 869 filas 11,0 s**, escritura 1,0 s); paquete de 15,6 MB (241 MB de JSON); memoria máxima 225 MB; `legacypkg.Verify` del paquete en 1,8 s                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `iso.sh … fmt-check` y `sync-check`                                                                                                                                                                    | Aprobados («All matched files use Prettier code style!», `gofmt` sin diferencias, «Metadatos coherentes»)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |

### No verificado

- El respaldo real (Fase E): la restauración de un respaldo de SQL Server 2012 en 2022 (actualiza la base) y sus tiempos
  reales; la medición de volumen sugiere menos de un minuto de lectura.
- Windows: Docker Desktop en Windows (ruta de `docker.exe`, montaje de `C:\…`, cancelación cuando la aplicación termina
  el proceso, `MoveFileEx`), solo compilado y analizado con `GOOS=windows`. `check-legacy-data` de `dev.mjs` sí corrió
  (con Node en macOS).
- La descarga real de la imagen (ya estaba en la Mac; la descarga y su avance solo con el docker simulado), Docker
  Desktop con menos de 2 GB de memoria (simulado), la copia del respaldo desde una USB real fuera de las carpetas
  compartidas (simulada; el error real de Docker sí lo vi) y el renombrado sin reemplazo en exFAT (camino de reserva sin
  probar en un disco así).
- La frase de paso pedida en una terminal (`term.ReadPassword`), solo con una función simulada.
- El contenedor del coordinador `adel-legacy-analysis` y su volumen ya no estaban la primera vez que listé los
  contenedores después de empezar (16/11, 00:05; existían a las 23:25). Mis órdenes solo eliminan por nombre lo que
  crean o por la etiqueta del lector, que ese contenedor no tenía; lo comprobé con dos contenedores y dos volúmenes de
  prueba (con y sin etiqueta: el filtro solo selecciona los etiquetados). Supongo que el coordinador lo eliminó
  (`legacy-sql.sh down`).

### Para la Fase C (API pública de `legacypkg`)

```go
// Lectura en flujo
func Inspect(source io.Reader, options OpenOptions) (Header, error)
func Open(source io.Reader, passphrase string, options OpenOptions) (*Reader, error)
func (reader *Reader) Header() Header
func (reader *Reader) Manifest() Manifest
func (reader *Reader) NextTable() (*TableReader, error) // io.EOF solo cuando todo cuadró
func (table *TableReader) Info() TableManifest
func (table *TableReader) Columns() []Column
func (table *TableReader) ColumnIndex(name string) int
func (table *TableReader) Next() ([]any, error) // nil, bool, string o json.Number; io.EOF al final de la tabla
func Verify(source io.Reader, passphrase string, options OpenOptions) (Manifest, error)
func (manifest *Manifest) Table(name string) (TableManifest, bool)
func ContentFingerprint(tables []TableManifest) string
func DefaultLimits() Limits
func KindOf(err error) ErrorKind // not_package, unsupported_version, wrong_passphrase, damaged, limit, invalid
// errors.Is(err, legacypkg.ErrWrongPassphrase), ErrDamaged, ErrNotPackage, ErrUnsupportedVersion, ErrLimit

// Escritura en flujo
func NewWriter(options WriterOptions) (*Writer, error) // WriterOptions{SpoolDir, KDF, ChunkSize}
func (writer *Writer) BeginTable(spec TableSpec) (*TableWriter, error)
func (tableWriter *TableWriter) WriteRow(values []any) error // nil, bool, string, json.Number, int, int32, int64
func (tableWriter *TableWriter) SetOmittedRows(name string, rows int64) error
func (tableWriter *TableWriter) Close() error
func (writer *Writer) SkipTable(skipped SkippedTable) error
func (writer *Writer) Finish(output io.Writer, passphrase string, info PackageInfo) (Result, error)
func (writer *Writer) Close() error // siempre: borra las tablas en preparación
func CreateOutput(path string) (*Output, error) // Commit() / Abort() / Path(); ErrOutputExists, ErrOutputExtension
func ValidatePassphrase(passphrase string) error

// Reglas (lo que no viaja) y constantes
func IsPasswordColumn(name string) bool
func TreatmentFor(name, baseType string) ColumnTreatment
func ExcludedTableReason(name string) string
func PresentColumn(name string) Column
func BaseType(sqlType string) string
const Signature, FormatVersion, Extension, PresentSuffix, SalesTable ("ventas"), SalesDateColumn ("fecha")
```

Paquetes sintéticos: `legacysynth.Build(output, frase, legacypkg.WriterOptions{KDF: …})` (para ir rápido en las
pruebas, `KDF: legacypkg.KDFParams{Time: 1, MemoryKiB: 8 << 10, Threads: 1}`), `legacysynth.Dataset()` (las tablas con
sus filas, para armar variantes) con `legacysynth.WritePackage(writer, tablas)`, y `legacysynth.SyntheticPasswords`.
Notas: el importador no debe confirmar su transacción antes de que `NextTable` devuelva `io.EOF`; las fechas del
paquete son hora local del equipo de origen sin zona (convertirlas con la zona del establecimiento, §6.1 del análisis);
los importes llegan como `json.Number` con 4 decimales (`decimal.NewFromString`).

### Para CLAUDE.md

- Estructura: `backend/internal/legacypkg` (formato `.adelmig`, neutral), `backend/internal/legacyread` (lector con
  Docker; **solo** lo importa `cmd/ad-el-pos-server/legacy.go`, lo comprueba `imports_test.go`) y
  `backend/internal/legacysynth` (base y paquete sintéticos); `scripts/check-legacy-data.sh`; `docs/MIGRACION_FORMATO.md`.
- Subcomandos `legacy-prepare --backup F.bak --output F.adelmig [--watch-stdin]` (frase de paso solo por la entrada
  estándar; avance en líneas JSON) y `legacy-synthetic --output F` (solo con `ADEL_TEST_MODE=1`), atendidos antes que
  los demás comandos.
- Reglas de datos reales: `.gitignore` con `*.bak`, `*.adelmig` y `legacy-data/`; `dev.sh format-check` (y `test`) y el
  `format-check` de Windows ejecutan `check-legacy-data`, que también reconoce un paquete o un respaldo renombrado por
  su firma; el lector nunca lee contraseñas (`<columna>_present`), omite binarios, `sysdiagrams` y
  `personal_detalles`, y nunca deja datos en claro en el disco.
- Pruebas: la de Docker real es opcional y nunca va en la tupla de `scripts/database.py`
  (`ADEL_TEST_DOCKER=1 go test -tags 'integration legacydocker' -run TestLegacyPrepareWithDocker ./internal/legacyread`,
  ~1 minuto, necesita la imagen `mcr.microsoft.com/mssql/server:2022-latest` descargada); usa la etiqueta
  `local.adel.pos.legacy-test=1` para su SQL Server de prueba, que el lector no toca. Las demás pruebas de la
  migración usan el docker simulado o `legacysynth`.
- sqlcmd: `-y 0` y `-h` son incompatibles (con `-y 0` no hay encabezados); `FOR JSON` omite los milisegundos en cero de
  `datetime`; con `--network none` hay que usar `-S 127.0.0.1`.
