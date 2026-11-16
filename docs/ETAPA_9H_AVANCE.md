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

- **Fecha de reconocimiento** de una venta importada (pregunta 3): la del pago que la **liquida**, como AD-EL POS
  reconoce la venta al cerrar la cuenta (decisión del propietario de la 9A, CLAUDE.md §21); la fecha de apertura se
  conserva. Precisada tras la revisión independiente de la Fase C (hallazgo I-1: un pago repetido, en cero o negativo
  registrado después movía la venta de día, turno o año): es el pago con el que lo acumulado, contando solo pagos
  mayores que cero en orden de fecha, alcanza lo que se debía; si nunca lo alcanza, el último pago mayor que cero; sin
  pagos mayores que cero, la apertura. Los pagos en cero o negativos se importan y cuentan en Cobros con su marca, pero
  no mueven la fecha.
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

---

## Fase C · Correspondencia e importación

Un agente, en su propio árbol de trabajo (`etapa-9h-c`, desde `etapa-9h` con la Fase A; la Fase B se fusionó a mitad de la
fase con `git merge etapa-9h`). 15–16/11/2026. Migración `0024_legacy_migration.sql` con su **sección C** (al final de la
misma migración, sin migración nueva; el esquema sigue en 24). Pruebas en copias aisladas: clúster temporal 55499 (una
base `adel_pos_test_c_*` por paquete). **No se tocó la instalación real ni se usó el respaldo real**: todas las pruebas
usan datos sintéticos (los paquetes de `internal/legacysynth` de la Fase B y un paquete en memoria propio de las pruebas
del importador con todos los casos de §7).

Commits en `etapa-9h-c`:

- `3cb716b` — historial en tablas propias (0024 sección C, solo lectura por disparador) y la importación en una sola
  transacción (catálogo, mesas y usuarios en su lugar, libro del historial, reemplazo del lote y validación §4.4).
- `ec827ab` — fusión de `etapa-9h` (Fase B: `legacypkg`, `legacyread`, `legacysynth`).
- `98470b0` — rutas de la migración, trabajador con el lock 7820148310, salidas de efectivo en el historial y «Asignar
  contraseña».
- `e26637d` — medición de volumen opcional y prueba de subida 23 → 24 con la sección C.
- `c222e2b` — `API_CONTRACTS.md`, `MIGRACION_ANALISIS.md` §6.8 y prueba de rutas ampliada.
- `2440f7d` — notas del revisor de la Fase B (tablas y columnas obligatorias, una apertura a la vez, sin la causa en la
  bitácora).

### Qué cambió

- **Paquete Go nuevo `internal/legacyimport`** (correspondencia e importación; no importa `internal/legacyread`, así que
  `serve` y el `host` siguen sin depender de Docker):
  - `package.go`: lectura del `.adelmig` con `legacypkg.Open` (errores del formato → `LEGACY_PASSPHRASE_INVALID`,
    `LEGACY_PACKAGE_DAMAGED`, `LEGACY_PACKAGE_VERSION`, `LEGACY_NOT_PACKAGE`, `LEGACY_PACKAGE_LIMIT`), detrás de una
    interfaz propia `Source`/`Table` (`source.go`; `MemorySource` para las pruebas unitarias).
  - `load.go`/`model.go`/`values.go`: lectura completa del paquete (también la comprobación de huellas y conteos, antes
    de escribir nada) a filas tipadas; importes `MONEY` como enteros de diezmilésimos (`Money`) y cantidades como
    milésimos (`Quantity`): nada pasa por float ni se redondea. Fechas `DATETIME` interpretadas con
    `establishment.timezone`. Falta de una tabla o columna del anterior, o un valor con otra forma →
    `LEGACY_PACKAGE_INCOMPATIBLE` con la tabla y la columna, nunca el valor.
  - `catalog_plan.go`/`names.go`: correspondencia del catálogo, las mesas y los usuarios (`MIGRACION_ANALISIS.md`
    §6.8). `history_plan.go`/`plan.go`: estados, fechas de reconocimiento, sumas, IVA estimado, marcas y el libro del
    historial. `flags.go`: la lista cerrada de marcas.
  - `expected.go`: la pasada independiente de la validación (conteos del manifiesto y totales calculados directamente
    de las filas del paquete, sin la correspondencia ni lo escrito). `validate.go`: la comparación con la base dentro de
    la transacción.
  - `write.go`, `write_history.go`, `write_live.go`, `ids.go`: la importación en una sola transacción (abajo).
  - `clean.go`: las condiciones de instalación limpia con mensajes en español sencillo.
  - `manager.go`, `worker.go`, `runs.go`, `uploads.go`, `status.go`: subidas por partes, corridas en segundo plano, el
    trabajador y el lock 7820148310. `pdf.go`: el informe en PDF. `summary.go`: resumen previo e informe final.
- **Administración** (`administration/legacy_migration.go`, rutas `maintenance/legacy/*`; `user_password.go`, «Asignar
  contraseña»; `users.go` con `has_password`; `http.go` con el límite de cuerpo de una parte y el envío del PDF;
  `routes.go`, `reports.go` —`completeDeferred` entrega la corrida al trabajador después de confirmar la petición— y
  `service.go` con `Service.Legacy`). Contrato completo en `API_CONTRACTS.md`, «Etapa 9H · migración del sistema
  anterior».
- **`cmd/ad-el-pos-server/main.go`**: `newApplication` arma la migración con la carpeta de trabajo
  `<carpeta de la configuración>/legacy-import` (en el servicio, dentro de la carpeta de datos; `.local/legacy-import` en
  desarrollo) y `serveHTTP` arranca y espera su trabajador como los de la 9G (`serve` y el `host`).
- **`scripts/database.py`**: `./internal/legacyimport` en la tupla de integración (14 paquetes).
- **Documentación**: `API_CONTRACTS.md` (rutas, cuerpos, respuestas, errores y auditoría), `docs/MIGRACION_ANALISIS.md`
  §6.8 (correspondencia final, lista de marcas y validación).

### Esquema del historial (0024, sección C): contrato para los reportes (Fase C, parte 2)

Todas las tablas en `app`, con `batch_id` → `legacy_import_batches(id)`; un solo lote `completed` a la vez (índice
único parcial) y, al reemplazarlo, sus filas del historial se borran en la misma transacción (el lote queda
`replaced`). Para los reportes basta filtrar por el lote vigente: `batch_id = (SELECT id FROM app.legacy_import_batches
WHERE status = 'completed')` (o, como solo hay filas del vigente, sin filtro). Importes `numeric(19,4)` exactos (MONEY);
cantidades `numeric(19,3)`; instantes `timestamptz` (la hora local del anterior interpretada con
`legacy_import_batches.timezone`). Ninguna llave hacia lo vivo: los identificadores nuevos (`product_id`, `category_id`,
`table_id`, `area_id`, `*_user_id`) son copias de `legacy_id_map.new_id` y los nombres se copian.

**`legacy_sales_recognitions`** (el libro, con las medidas de `sales_recognitions`):

| Columna                                                             | Significado                                                                                                                                                                                 |
| ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `sequence`                                                          | Identidad (orden de escritura; para `latest()`).                                                                                                                                            |
| `entry_kind`                                                        | `line` (un renglón del anterior con sus complementos sumados), `discount`, `charge` (cargo extra) o `difference` («Diferencia del sistema anterior»).                                       |
| `legacy_sale_id`, `sale_folio`, `legacy_line_id`                    | Venta y folio originales (texto); renglón original solo en `line`.                                                                                                                          |
| `recognized_at`                                                     | Instante de reconocimiento = último pago de la venta (sin pagos: su apertura). Día comercial igual que en los reportes: `((recognized_at AT TIME ZONE zona) - hora_de_inicio)::date`.       |
| `opened_at`                                                         | Apertura de la venta.                                                                                                                                                                       |
| `mode`                                                              | `dine_in` / `takeaway` (mesa 1 del anterior = para llevar).                                                                                                                                 |
| `legacy_table_id`, `table_id`, `table_name`, `area_id`, `area_name` | Mesa (nulos para llevar).                                                                                                                                                                   |
| `legacy_register_id`, `register_name`                               | Caja del anterior (solo su nombre; no hay `registers`).                                                                                                                                     |
| `legacy_shift_id`                                                   | Turno = corte del anterior (`ventas.corte`, el del último pago); `legacy_cash_shifts.legacy_id`. Nulo = «Sin turno».                                                                        |
| `legacy_cashier_id`, `responsible_user_id`, `responsible_name`      | Cajero que abrió la venta.                                                                                                                                                                  |
| `legacy_product_id`, `product_id`, `product_code`, `product_name`   | Producto (solo `line`; `product_code` = `SA-<id>`).                                                                                                                                         |
| `legacy_category_id`, `category_id`, `category_name`                | Categoría del producto (solo `line`).                                                                                                                                                       |
| `concept`                                                           | `discount`: «Descuento: <tipo> · <descripción>»; `charge`: «Cargo: <descripción>»; `difference`: «Diferencia del sistema anterior».                                                         |
| `tax_rate`, `tax_estimated`                                         | Tasa deducida del renglón (0, 8 o 16; nula fuera de `line`) e IVA estimado (no estaba guardado).                                                                                            |
| `quantity`, `gross`, `discount`, `charge`, `net`, `base`, `tax`     | Medidas. `net = gross − discount + charge` y `net = base + tax` (restricciones). `line`: `gross` = total del renglón + Σ total de sus complementos; `tax` = Σ IVA; `discount` y `charge` 0. |

Solo las ventas pagadas no canceladas tienen renglones en el libro. Por venta, **Σ `net` = `legacy_sales.amount_due`**
(total + montoExtra − descuentos: lo que se debía según el anterior) y **Σ `tax` = `legacy_sales.tax`**, exactos; la
`difference` aparece cuando el total o el IVA guardados no cuadran con los renglones (decisión 2 del propietario). No hay
promociones ni cortesías: para unir con `sales_recognitions`, `promotion = 0` y `courtesy = 0`. Índices:
`(recognized_at, sequence)`, `(batch_id, legacy_sale_id)`, `(legacy_shift_id, recognized_at)`, `(product_id,
recognized_at)`.

**Demás tablas** (columnas completas en la migración):

- `legacy_sales`: una por venta (`legacy_id`, `folio`, `status` `paid`/`cancelled`/`open`, `opened_at`, `recognized_at`
  solo en `paid`, `last_payment_at`, `cancelled_at`, `mode`, mesa, área, caja, cajero, `legacy_shift_id`,
  `account_name`, `subtotal`/`tax`/`total` guardados, `amount_paid` (`montoPagado`), los valores originales de
  `pagado`/`enmesa`/`facturado`/`cancelado`, `charge_concept`/`charge_amount` (el cargo extra), `discount_amount`,
  `amount_due`, `lines_amount`, `lines_count`, `payments_amount`, `payments_count`, `tax_estimated`, `flags`, `note`).
  Índice parcial `(recognized_at) WHERE status = 'paid'`. **Canceladas al estilo del anterior** (lo que el anterior
  contaba): Σ `amount_due` de `status = 'cancelled'`.
- `legacy_sale_lines` (renglones: producto, código y categoría copiados, cantidad y original, importes guardados,
  `tax_rate`, sumas de sus complementos, `notes`, `flags`), `legacy_line_complements` (por renglón: complemento,
  `included`, `requested` —por unidad del renglón—, importes con signo; para «Complementos seleccionados»:
  `units` = Σ cantidad del renglón con `requested > 0`, porciones = Σ cantidad × `requested`, adicionales = Σ cantidad ×
  max(`requested` − `included`, 0)), `legacy_quantity_changes` («Modificaciones de cantidades»: `original_quantity`,
  `new_quantity`, quién autorizó; el anterior no guardaba quién modificó), `legacy_discounts`, `legacy_cancellations`
  (quién autorizó y cuándo; sin motivo ni quién canceló).
- `legacy_payments`: `method` `cash`/`card`/`other`, `method_name` («Efectivo», «Tarjeta (sistema anterior)»),
  `reference` (comprobante), `amount` exacto (también 0 o negativo, con su marca), quién recibió, `paid_at`,
  `legacy_shift_id` (`pagos.corte`), `sale_status` (para excluir los pagos de canceladas o abiertas si se quiere) y
  `flags`. Índices `(paid_at)`, `(batch_id, legacy_sale_id)`, `(legacy_shift_id, paid_at)`.
- `legacy_cash_shifts` (cortes tal como se guardaron: caja, cajero, `status` `open`/`closed`/`unknown`, fondo, conteo,
  «venta» —que en el anterior incluía tarjeta y canceladas—, retiros, diferencia, apertura y cierre),
  `legacy_cash_withdrawals` (retiros), `legacy_cash_expenses` (salidas de efectivo; vacía en el respaldo real),
  `legacy_price_history` (historial de precios con vigencias y tasa deducida) y `legacy_id_map`.
- **Filtro de los reportes**: el existente `origin` («Origen (para llevar)») choca; la Fase 0 propuso otra clave (p. ej.
  `data_source`, «Datos: AD-EL POS / Sistema anterior»).

### Importación (transacción, locks y tiempos)

1. **Antes de la transacción** (trabajador): lock 7820148310 (sesión, conexión propia, sin esperar); lectura completa del
   paquete; comprobación de la instalación y del lote confirmado; **respaldo verificado** por el camino de «Respaldar
   ahora» (`backupschedule.Runner.StartManual`, 7820148309 solo en su conexión, sin transacción abierta; si hay otro
   respaldo en curso espera hasta 5 min; si falla, no se importa).
2. **La transacción**: `set_config('adel.legacy_import', <lote>, true)`; lote nuevo `importing`; si hay lote vigente,
   borra su historial; **COPY** del historial completo y del libro; validación del historial (sumas por año y forma de
   pago, ventas cuyo libro no suma); **después**, 7820148305 → 7820148304 → 7820148308 exclusivos (espera acotada a 30 s:
   `LEGACY_LOCKS_BUSY`); vuelve a comprobar la instalación con los locks tomados; escribe catálogo, mesas, usuarios y la
   correspondencia; conteos; marca el lote anterior `replaced` y el nuevo `completed` con su validación e informe;
   confirma. Un error, la cancelación del contexto o el apagado del servidor deshacen todo.
3. **Tiempo con los locks tomados** (medido, ≈ 230 mil ventas sintéticas): **257 ms** (el COPY del historial, 15.4 s, va
   antes de tomarlos). Mientras están tomados esperan las pantallas que leen o escriben la operación del restaurante y
   cobros (7820148305: Mesas y cuentas, captura, cocina, Caja), el catálogo (7820148304: Catálogo, cotizaciones) y el
   inventario (7820148308); en una instalación limpia no hay operación en curso. Las demás rutas (reportes, estado del
   sistema, `maintenance/legacy`) siguen respondiendo: la importación usa su conexión del pool (2 conexiones en total
   con la del lock 7820148310; 3 durante el respaldo).

### Decisiones

1. **Tablas propias del historial** (decisión del coordinador), con el disparador **por sentencia** (no por fila: el
   COPY de cientos de miles de filas no paga un disparador por fila). Crear el lote exige una marca con forma de
   identificador; cualquier otra escritura (también `UPDATE` de lotes y `TRUNCATE`) exige que la marca sea un lote
   `importing` existente. Es una protección contra escrituras accidentales; no contra un dueño de la base.
2. **Repetir la migración actualiza en su lugar** lo vivo de un lote anterior (mismo identificador por clave anterior)
   en vez de borrar e insertar: los usuarios importados pueden tener sesiones, auditoría o una contraseña asignada, y un
   producto importado una receta, y borrarlos fallaría por llave foránea o perdería esa información. Lo que ya no viene
   se borra si nada lo usa; si no, se desactiva («<nombre> (retirado 1a2b3c4d)») y queda como `retired` en la
   correspondencia (sigue siendo «del lote» para la condición de instalación limpia).
3. **Un grupo por conjunto distinto de complementos** (no uno solo): AD-EL POS ofrece todas las opciones del grupo a
   cada producto que lo tiene, y el anterior admitía complementos por producto.
4. **Producto sin precio** → $0.00 y desactivado (no se puede vender gratis por error); categoría inexistente → «Sin
   categoría (sistema anterior)»; `categorias.cocina` no se importa (las estaciones y rutas de cocina no existen todavía
   en una instalación limpia).
5. **`salidas` se importa** (`legacy_cash_expenses`, tabla agregada a la sección C) y forma el «Total de egresos» de la
   validación: el paquete sintético de la Fase B trae salidas para probar los egresos y, sin importarlas, la validación
   habría marcado una diferencia que no es tal. En el respaldo real está vacía (0). `entradas` (compras) no se importa; si
   un paquete trae alguna, la validación la muestra como diferencia.
6. **Analizar también es un trabajo en segundo plano** (la lectura completa del paquete real tarda segundos, más que el
   plazo de una petición) y la importación exige un análisis correcto del mismo paquete (`analysis_run_id`) y la
   confirmación del lote que reemplaza (`replace_batch_id`). La frase de paso se pide en los dos pasos (nunca se guarda).
7. **El estado de las subidas vive en memoria** (el archivo, en la carpeta de trabajo): al arrancar se borra lo que haya
   quedado. Solo el mismo usuario en la misma terminal ve su subida. Tope 1 GiB; vencimiento a 2 horas sin actividad;
   el archivo se borra al importar correctamente (no al fallar: se puede reintentar con la frase correcta).
8. **Permiso `settings.manage` y solo la escucha local** para la migración (como Mantenimiento y los respaldos).
   **«Asignar contraseña» no exige la escucha local**: las demás rutas de usuarios no la exigen (crear un usuario con su
   contraseña funciona desde cualquier terminal con `users.manage`). `GET users` agrega `has_password`.
9. **Validación**: los conteos esperados salen del **manifiesto** del paquete (el lector ya comprobó que cada tabla trae
   exactamente esas filas); los totales, de una pasada propia sobre las filas. El año es el del **día comercial** del
   reconocimiento (como los reportes). Una venta pagada sin pagos se reconoce en su apertura (marca `no_payments`).
   Pagos con `tipo` nulo → efectivo (el predeterminado del anterior era 1).
10. **El respaldo previo aparece como un «Respaldar ahora»** en la lista de respaldos (su `operation_id` es el id de la
    corrida) y con su `maintenance.backup_created`.
11. **El IVA de una venta cuyo IVA guardado no cuadra con sus renglones** también produce un renglón `difference` (con
    `net = 0` si el total sí cuadra): así Σ `tax` por venta es el IVA guardado de la venta.
12. **Notas del revisor independiente de la Fase B**: (1) la importación lee el paquete completo con `Load` —hasta que
    `NextTable` devuelve `io.EOF`, que es cuando el lector confirma bloques, huellas y conteos— antes de abrir la
    transacción, así que nunca se confirma nada de un paquete sin comprobar; (2) **todas las tablas y columnas que el
    importador lee son obligatorias** (24 tablas del anterior, con todas sus columnas usadas; solo `divisas` y
    `articulos`, que solo se cuentan, pueden faltar): un paquete coherente consigo mismo pero sin una de ellas se rechaza
    con `LEGACY_PACKAGE_INCOMPATIBLE` nombrando la tabla, nunca se importa a medias (prueba con un paquete real sin
    `cortes`); (3) `legacypkg.Open` recibe `OpenOptions{}` (los límites predeterminados completos, nunca parciales);
    (4) **una apertura de paquete a la vez** en el servidor (mutex alrededor de `legacypkg.Open`, además de que el
    trabajador corre una corrida a la vez); (5) en la bitácora solo van el código de la migración y el detalle técnico del
    formato (`Error()` de `legacypkg`), nunca la causa interna (`Unwrap`); un error de lectura que no es del formato se
    registra como «error de lectura del paquete».

### Pruebas ejecutadas (aisladas)

- `go-unit`: aprobado (vet normal e `integration`, `go test -race ./...`, `GOOS=windows` build y vet).
- Unitarias nuevas (`internal/legacyimport`): `TestParseFixedIsExact`, `TestDeduceRateAndEstimate`,
  `TestDeriveUsername`, `TestLoadRejectsIncompatiblePackages`, `TestCatalogCorrespondence`,
  `TestHistoryStatesFlagsAndLedger` (Σ del libro = lo que se debía y Σ IVA = IVA de la venta en cada venta),
  `TestExpectedIsIndependent`, `TestLegacyUploadsInParts` (orden, reenvío, huella mala de parte y del total, otro
  usuario, tope, vencimiento y limpieza).
- Integración (`iso.sh go-int`):
  - `./internal/legacyimport` (≈ 7 s): `TestLegacyImportMapsHistoryExactly` (importes exactos en cada tabla, el libro, lo
    vivo, la operación, la caja, los turnos y el inventario sin cambios, la numeración de folios intacta, el disparador
    rechaza `INSERT`/`UPDATE`/`DELETE`/`TRUNCATE` también con una marca que no es de un lote en curso, y el motor real
    `catalog.QuoteForOrder` cobra el producto importado como el anterior: incluidos $150, quitar el arroz $130 —la
    bonificación—, quitar el arroz y agregar papas $155, agregar papas $175, dos ensaladas $165),
    `TestLegacyImportReplacesTheBatchCompletely` (lote anterior `replaced` sin filas, mismos identificadores, contraseña
    asignada conservada, mesa y usuario que ya no vienen borrados, usuario con sesión retirado y sin sesión, confirmación
    del lote), `TestLegacyImportRefusesOwnData` (turno propio; categoría, área y usuario propios con sus mensajes; nada
    escrito), `TestLegacyImportInterruptedLeavesNothing` (error después del historial y contexto cancelado después de lo
    vivo: el lote anterior intacto), `TestLegacyImportShowsMoneyDifferences` (un peso de más provocado en un pago: la
    validación lo muestra con su importe y el lote queda `completed` con diferencias) y
    `TestLegacyManagerAnalyzesAndImportsPackages` (paquetes reales de `legacysynth`: frase incorrecta, dañado, versión
    desconocida, no es un paquete, resumen previo, dos importaciones a la vez —una rechazada—, corrida en cola marcada
    `interrupted`, importación con validación sin diferencias, subida borrada, PDF, auditoría sin la frase de paso,
    reemplazo y respaldo fallido sin importar nada).
  - `./internal/administration` con `TestStage9HLegacyMigrationRoutes` (≈ 13 s): por HTTP, permisos (403), escucha no
    local (`LOCAL_REQUIRED`), tope de tamaño, huella mala, fuera de orden, cancelar, frase corta e incorrecta, paquete
    dañado, versión y no paquete, importar sin confirmar, importación con el respaldo previo real de `backupschedule`
    (destino simulado), estado, lote, PDF, rutas de cuentas y `restaurant/state` sin ventas importadas, «Asignar
    contraseña» (403 sin `users.manage`, 409 a quien ya tiene, contraseña corta, inicio de sesión con
    `initial_password`, auditada), auditoría completa sin la frase de paso, preparación válida y con una ruta rechazada,
    reemplazo sin confirmar el lote (409) y confirmado, y el servidor respondiendo (`maintenance/legacy`,
    `restaurant/tables`, `catalog/products`) con la importación detenida a propósito a mitad.
  - `./internal/database` (≈ 10 s): `TestStage9HUpgrade` ampliada (23 → 24 con la sección C: cada tabla, el disparador
    en cada tabla del historial, ninguna llave de tablas operativas hacia el historial, rechazos, escritura dentro de una
    importación con importe exacto, dos lotes en curso imposibles y **ninguna fila existente cambiada** en ninguna tabla
    del esquema 23).
  - `./internal/administration` completo (122 s) y `./cmd/ad-el-pos-server` (41 s): aprobados (el cambio de
    `publicUserColumns` y de `main.go` no rompe nada de lo anterior). Después de las notas del revisor se repitieron
    `./internal/legacyimport` (7 s) y `TestStage9HLegacyMigrationRoutes` (13 s): aprobados.
- **Medición de volumen** (`ADEL_LEGACY_MEASURE=1`, `TestLegacyImportVolume`, sin `-race`, `$SP/measure-c.sh`): 230 240
  ventas, 719 253 renglones, 323 806 complementos y 230 242 pagos sintéticos. Paquete de 8.8 MB (generarlo, 2.1 s; los
  datos sintéticos se comprimen más que los reales). Análisis: lectura completa 2.6 s, con la correspondencia 3.0 s;
  memoria del modelo en el montón ≈ 367 MB (el proceso de la prueba completo, con la generación del paquete, llegó a
  907 MB de memoria residente). Importación: 17.1 s (COPY del historial 15.4 s, validación del historial 1.3 s), **locks de
  la operación tomados 257 ms**, espera de los locks 4 ms; base de 774 MB después de importar.
- `fmt-check` y `sync-check` aprobados; `docs/etapas/*.md` sin cambios (md5).

### No verificado

- El respaldo real (Fase E): tiempos, tamaño del paquete, memoria y la validación con los datos reales.
- La interfaz (Fase D): la subida desde Rust, el asistente, el PDF visto en pantalla e impreso.
- Windows (solo `GOOS=windows` build y vet): la carpeta de trabajo con ACL, el `host` de Windows con la migración.
- Una hora local ambigua del cambio de horario (no ocurre en el respaldo real: horas de 06 a 21 y sin horario de verano
  desde 2022); la regla es la de `time.Date` (primera ocurrencia).
- Los reportes con el historial importado (Fase C, parte 2).

### Para CLAUDE.md

- Encabezado y §13–14: 0024 con la **sección C** (historial importado del sistema anterior); esquema 24.
- §3, tabla de locks: **7820148310** — migración del sistema anterior (`internal/legacyimport`): `pg_try_advisory_lock`
  de sesión en una conexión propia mientras dura un análisis o una importación (una a la vez aunque dos servidores
  compartan la base; ocupado → la corrida falla con `LEGACY_BUSY`). Se toma primero; con él tomado se hace el respaldo
  previo (7820148309 sigue tomándose solo en su conexión, sin transacción abierta) y después la transacción de la
  importación toma 7820148305 → 7820148304 → 7820148308 exclusivos, solo para lo vivo y después del COPY del historial.
  Nadie más lo pide. Orden: 7820148310 → (7820148309 aparte) → 7820148305 → 7820148304 → 7820148308.
- §3, «Composición del backend»: `maintenance/legacy*` → `administration/legacy_migration.go` con
  `internal/legacyimport` (subidas, corridas, trabajador `legacyimport.Manager.Run` arrancado en `serveHTTP`, rutas solo
  locales con `settings.manage`); `users/{id}/password` → `administration/user_password.go`. El historial vive en
  `app.legacy_*` (solo lectura por disparador, sin llaves desde tablas operativas).
- §4, estructura: `backend/internal/legacyimport/` (correspondencia e importación del sistema anterior).
- §6, pruebas: integración nueva `./internal/legacyimport` (14 paquetes en la tupla), `TestStage9HLegacyMigrationRoutes`
  (`administration`), `TestStage9HUpgrade` con la sección C; medición opcional `ADEL_LEGACY_MEASURE=1`
  (`internal/legacyimport`, sin `-race` para tiempos representativos).
- §15, Etapa 9H: historial en tablas propias con su libro; la importación en una transacción con respaldo previo;
  repetir actualiza en su lugar; «Asignar contraseña» (solo a usuarios sin contraseña, `initial_password`); `has_password`
  en Usuarios.
- Regla permanente propuesta: «El historial importado (`app.legacy_*`) solo lo escribe `internal/legacyimport` dentro de
  su transacción (marca `adel.legacy_import`); ninguna ruta ni módulo lo modifica y ninguna tabla operativa tiene llaves
  hacia él».

---

### Fase B · Correcciones de la revisión

El mismo agente (16/11/2026), en el árbol `etapa-9h-b2` (desde `etapa-9h`, `0fe4a7b`), con `ISO_TAG=B`, en paralelo con
la Fase C (sin tocar `internal/legacyimport`). La prueba con Docker corrió **una sola vez, sin contenedores de otros
agentes** (comprobado antes). Nunca se usó el respaldo real.

Commits: `49535d5` (M-2: comprobación de datos reales), `abe392f` (`legacypkg` y `legacysynth`: lectura, límites, FAT y
orden de textos) y `57a95c0` (`legacyread`: bloqueo, restos, `--cleanup`, Docker local, respaldo y verificación; y
`docs/MIGRACION_FORMATO.md`).

#### Qué cambió

- **I-1 · Una sola preparación a la vez y limpieza que no destruye lectores vivos.**
  - Bloqueo del sistema operativo (`flock` en macOS y Linux, `LockFileEx` en Windows) sobre
    `<carpeta temporal del usuario>/adel-legacy-prepare/lock`. El sistema lo suelta aunque el proceso termine a la
    fuerza. Con otra preparación en curso: `ALREADY_RUNNING` («Ya se está preparando un paquete en esta computadora…»),
    sin tocar Docker ni archivos.
  - El contenedor y el volumen llevan además las etiquetas `local.adel.pos.legacy-reader.pid` y
    `local.adel.pos.legacy-reader.started`.
  - La limpieza de restos solo borra los lectores de un proceso que ya no existe (`kill(pid, 0)` u `OpenProcess`), los
    que no tienen PID (de la versión anterior) o los de hace más de 12 horas (por si el PID se reutilizó). Uno de otra
    preparación viva (por ejemplo, con otra carpeta temporal) nunca se toca.
  - `requireNoLeftovers` de la prueba con Docker solo mira los lectores con el PID de sus propios procesos.
- **M-1 · Restos de una preparación terminada a la fuerza.**
  - Cada preparación trabaja en `adel-legacy-prepare/run-<aleatorio>`: tablas en preparación cifradas, copia del
    respaldo y un archivo `partial` con la ruta del paquete a medias.
  - Al empezar, con el bloqueo tomado, borra lo que dejó otra: carpetas `run-*` (también la copia **en claro** del
    respaldo), el paquete a medias registrado (`legacypkg.RemoveStalePartial`, solo si el nombre es exactamente el de
    un temporal de `CreateOutput` y es un archivo regular) y los contenedores y volúmenes cuyo proceso terminó. También
    borra las carpetas sueltas del formato anterior (`adel-migracion-*` y `adel-respaldo-*` en la carpeta temporal) con
    más de 6 horas.
  - Nuevo `legacy-prepare --cleanup`, para que la Fase D lo llame al abrir el asistente: hace la misma limpieza y
    responde con `{"event":"cleanup","removed":{"folders","partials","containers","volumes"},"docker":"checked"|"unavailable"}`.
    Sin Docker borra igual las carpetas. No lee la entrada estándar. Con una preparación en curso responde
    `ALREADY_RUNNING`.
  - El resumen final lleva `previous_leftovers`.
  - `prepare.go` y `MIGRACION_FORMATO.md` ya no dicen «pase lo que pase»: dicen qué queda tras SIGKILL y quién lo borra.
- **M-2 · Más archivos detectados.**
  - `.gitignore`, `check-legacy-data.sh` y `checkLegacyData` (`dev.mjs`) incluyen `*.bak.gz`, `*.bak.zip`, `*.bak.7z`,
    `*.mdf`, `*.ndf`, `*.ldf` y `*.trn`.
  - Además detectan un gzip con otro nombre cuyo contenido descomprimido empieza con `TAPE`; solo se descomprime el
    principio. En `dev.mjs` los candidatos se buscan con `^\x1f`, porque Node pasaría el byte `8b` como UTF-8, y se
    confirman al descomprimir.
  - Sin ganchos de git.
- **M-3 · Orden de textos como SQL Server (`legacysynth/order.go`).** Al comparar, el texto más corto se rellena con
  espacios; si empatan, decide `DATALENGTH`. `TestOrderingFollowsSQLServer` comprueba el orden completo que midió el
  revisor (`nil`, `""`, `"A"`, `"a\x01"`, `"a\t"`, `"a\x1f"`, `"a"`, `"a "`, `"a!"`) en `varchar` y `nvarchar`. La
  huella del paquete sintético no cambió (`38b7125f…`): sus datos no tenían esos empates.
- **M-4 · Respaldos con varias bases.** Si el archivo tiene respaldos completos de varias bases, `BACKUP_INVALID` con
  el mensaje «El archivo tiene respaldos de varias bases de datos…». El número de juegos del archivo va en el
  manifiesto (`source.backup_sets`) y en el resumen (`backup.sets`).
- **M-5 · Docker local.** Antes de todo se ejecuta `docker context inspect --format {{.Endpoints.docker.Host}}`, que
  respeta `DOCKER_HOST` y `DOCKER_CONTEXT` (comprobado en esta Mac con `tcp://` y `ssh://`). Si el resultado no
  empieza con `unix://` ni `npipe://`, `DOCKER_REMOTE` sin contactar ese Docker.
- **M-6 · Huella del respaldo.** El SHA-256 del respaldo se calcula antes de arrancar el contenedor. Al escribir se
  comprueba que el tamaño y la fecha del archivo no cambiaron; si cambiaron, `BACKUP_CHANGED`. El mismo error sale si
  el contenedor ve el respaldo con otro tamaño.
- **M-7 · Windows con USB FAT o exFAT.** Si el disco no guarda listas de control de acceso (`GetVolumeInformation` sin
  `FILE_PERSISTENT_ACLS`), el temporal del paquete se crea sin ACL propia, sin reemplazar nada. El paquete va cifrado.
- **Sugerencias aplicadas.**
  - Plazo de 2 horas por tabla en `readTable`, con `READ_FAILED` «se agotó el plazo de lectura de <tabla>».
  - `resolveLimits` completa campo por campo los límites en cero.
  - Nuevo tipo `unreadable` en `legacypkg`: un error de entrada y salida de la fuente (en el encabezado o en un bloque)
    ya no se reporta como «dañado», sino «No se pudo leer el archivo del paquete. Si está en una USB…». `KindOf` y
    `errors.Is(err, legacypkg.ErrUnreadable)` lo reconocen.
  - `legacy-prepare` recorre con `legacypkg.Verify` el paquete escrito antes de darle su nombre. Informa
    `verified: true` y el tiempo `timings.verify`.

#### Decisiones

- El bloqueo va en la carpeta temporal del usuario y no en la de configuración, para que las pruebas lo aíslen con su
  `TMPDIR`. En macOS la aplicación y la terminal comparten el mismo `TMPDIR` de la cuenta. Dos preparaciones con
  carpetas temporales distintas sí pueden correr a la vez; las etiquetas con PID impiden que una destruya a la otra, y
  la prueba con Docker lo comprueba.
- En el bloqueo no se borra el archivo `lock`: borrarlo crearía una carrera entre procesos.
- `BACKUP_CHANGED` se decide por tamaño y fecha, no con un segundo SHA-256, para no volver a leer el respaldo completo.
- Para la Fase C: `legacyimport.packageError` hoy traduce un tipo desconocido a `CodeDamaged`, conservando el mensaje
  del formato. Puede agregar `legacypkg.KindUnreadable` con un código propio. No lo toqué porque es de esa fase.

#### Pruebas ejecutadas

| Prueba                                                                              | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| ----------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ISO_TAG=B iso.sh … go-unit` (vet normal e `integration`, `-race`, `GOOS=windows`)  | Aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `go vet -tags 'integration legacydocker'` en macOS y con `GOOS=windows`             | Aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `legacypkg` (nuevas)                                                                | Aprobadas: error de entrada y salida en el encabezado, en el JSON, en el primer bloque y a media lectura → `unreadable`; límites parciales; `RemoveStalePartial` (borra el temporal; no borra un enlace, un archivo ajeno, una ruta relativa ni otro nombre).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| Comprobación de datos reales (índice temporal, `.sh` y `dev.mjs`)                   | Aprobada: falla con `.MDF`, `.ldf`, `.ndf`, `.trn`, `.bak.gz`, `.bak.zip`, `.BAK.7z` y con un respaldo MTF dentro de un gzip llamado `semana.gz`; pasa con un gzip de texto y con un binario que empieza con `1f 8b` y no es gzip. 0,2 s sobre el repositorio.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `legacysynth`                                                                       | Aprobado, con el orden completo de SQL Server en `varchar` y `nvarchar`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `legacyread` (docker simulado)                                                      | Aprobadas. Bloqueo: `ALREADY_RUNNING` sin llamar a Docker ni borrar la carpeta en curso, también con `--cleanup` y desde la línea de órdenes. Restos: solo se borran los lectores muertos, sin PID o de más de 12 h, nunca el vivo. Carpetas y paquete a medias de una preparación matada, borrados al preparar, con `--cleanup` y con `--cleanup` sin Docker. `DOCKER_REMOTE` con `tcp://`, `ssh://` y vacío (sin contactar), y `npipe://` aceptado. `BACKUP_CHANGED` en los dos caminos. Juegos de respaldo: se restaura el completo más reciente de la misma base y se rechazan varias bases. Plazo por tabla. Resumen con `verified`, `verify`, `sets` y `previous_leftovers`. Argumentos de `docker run` y `volume create` con las etiquetas de PID y hora. |
| **Docker real con el respaldo sintético** (una sola vez, sin contenedores de otros) | Aprobada en 64,7 s. Con dos preparaciones en la misma carpeta temporal, la segunda responde `ALREADY_RUNNING` y no escribe nada. Otra preparación con su propia carpeta corre a la vez que la primera y **las dos terminan bien**, con la huella del sintético. Paso de la primera: 25,8 s (lectura 15,1 s, verificación 0,06 s); paquete de 32 890 bytes. Con páginas de 7 filas, 24,8 s y la misma huella. Al terminar, ningún contenedor ni volumen.                                                                                                                                                                                                                                                                                                          |
| Experimento manual de SIGKILL (respaldo sintético del revisor)                      | `kill -9` a los 18 s, durante la lectura: quedaron `run-*`, el contenedor y su volumen. `legacy-prepare --cleanup` respondió `{"folders":1,"partials":0,"containers":1,"volumes":1}`, `docker: checked`, y no quedó nada (en la carpeta de trabajo solo `lock`).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `iso.sh … fmt-check` y `sync-check`                                                 | Aprobados                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |

#### No verificado

- Windows (solo compilado y analizado con `GOOS=windows`): `LockFileEx`, `OpenProcess` y `GetExitCodeProcess`, el
  temporal sin ACL en una USB FAT o exFAT (M-7) y el extremo `npipe://` real.
- El paquete a medias borrado tras un SIGKILL en la fase de escritura (probado con el docker simulado; el experimento
  real mató la preparación durante la lectura, antes de crear el paquete).

#### Para CLAUDE.md

- `legacy-prepare`: una sola preparación a la vez (`ALREADY_RUNNING`, bloqueo en
  `<temporal>/adel-legacy-prepare/lock`). `legacy-prepare --cleanup` borra los restos de una preparación terminada a la
  fuerza; la aplicación lo llama al abrir el asistente. Docker debe ser local (`DOCKER_REMOTE`). `BACKUP_CHANGED` si el
  respaldo cambia durante la preparación. El paquete se verifica antes de darle su nombre.
- Los lectores llevan su PID y su hora en etiquetas, y la limpieza nunca borra el de un proceso vivo. La prueba con
  Docker no debe correr a la vez que la de otro agente.
- `legacypkg`: tipo de error `unreadable` (error de lectura de la fuente, distinto de `damaged`). Los límites en cero de
  `OpenOptions` toman el valor predeterminado.
- La comprobación de datos reales también reconoce `*.bak.gz`, `*.bak.zip`, `*.bak.7z`, `*.mdf`, `*.ndf`, `*.ldf`,
  `*.trn` y un respaldo dentro de un gzip.

---

## Fase C · El historial en los reportes

Un agente, en su propio árbol de trabajo (`etapa-9h-c2`, desde `etapa-9h` con las fases A, B y C parte 1). 16/11/2026.
Sin migración nueva ni cambios en la sección C de la 0024 (ni tablas, ni índices, ni vistas: la medición no los pidió).
Pruebas en copias aisladas: clúster temporal 55499 (bases `adel_pos_test_d2_*`). **No se tocó la instalación real ni se
usó el respaldo real**: el historial de las pruebas se carga con el importador real (`legacyimport`: `OpenPackage`,
`Load`, `Analyze` e `Import`) desde paquetes sintéticos de `internal/legacysynth` (`Dataset()` + `WritePackage`), con
ventas agregadas en fechas límite; ninguna prueba escribe en `app.legacy_*` a mano y no hizo falta ningún ayudante
exportado nuevo en `legacyimport`.

Commit en `etapa-9h-c2`:

- `edd558d` — el historial importado del sistema anterior en los reportes (filtro «Datos», unión del libro, diferencia
  del sistema anterior, cobros fuera del alcance de Caja, cortes y exportaciones).

### Qué cambió

- **`backend/internal/reports/legacy.go`** (nuevo): filtro «Datos» (`data_source`, `choice`: «AD-EL POS y sistema
  anterior» `all` por omisión, «Solo AD-EL POS» `native`, «Solo sistema anterior» `legacy`), alcance por reporte
  (`legacyScope`: hay lote vigente, incluye AD-EL POS, incluye el historial), la unión del libro (`ledgerFrom`: el de
  AD-EL POS tal como antes, el del historial o los dos con las mismas columnas) y sus medidas (`ledgerExtras`), las
  medidas y condiciones del historial para las consultas propias, las columnas «Datos» y «Diferencia del sistema
  anterior», la marca de lo de AD-EL POS (`markNative`) y la unión de renglones por clave (`mergeRows`).
- **Motor** (`engine.go`, `handler.go`, `filters.go`, `model.go`, `query.go`): `Column.Legacy` y `Filter.Legacy`
  (`json:"-"`); una consulta corta por reporte y por catálogo averigua si hay un lote `completed`; sin lote, el catálogo
  no ofrece el filtro ni las columnas del historial, `meta.filters` no lo describe y el valor recibido se ignora.
- **Reportes** (27): ventas y productos por la unión del libro (`sales.summary`, `by_day`, `by_week`, `by_month`,
  `by_hour`, `by_category`, `by_product`, `by_mode`, `dine_in`, `takeaway`, `evolution`, `products.top`, `bottom`,
  `trends`); con renglones propios del historial (`sales.accounts` —folio `SA-<folio>`, «Venta del sistema anterior»—,
  `sales.by_shift` —un renglón por corte, «Sin turno» para las pagadas sin corte—, `products.complements`
  —«Complementos (sistema anterior)»—, `payments.by_method`, `by_shift`, `by_register`, `list`, `by_day` y `by_user`
  —estos dos se juntan con los de AD-EL POS del mismo día o del mismo usuario importado—, `sales.voided_accounts`,
  `sales.cancellations_by_reason`, `products.quantity_changes` y `cash.shifts`). Cada renglón lleva «Datos»: «AD-EL
  POS», «Sistema anterior» o «AD-EL POS y sistema anterior».
- **Puente** (`desktop/src-tauri/src/api.rs`): `data_source` en la lista blanca de `reports_*` (y en su prueba). La
  interfaz no cambió: el filtro y las columnas salen del catálogo de Go (el selector `choice` con valor por omisión ya
  existía); prueba de Vitest nueva.
- **Documentación**: `docs/REPORTS.md` (sección «El historial del sistema anterior», filtros, pruebas, mediciones,
  limitaciones y cómo agregar un reporte) y `reports/doc.go`.

### Decisiones

1. **Sin lote, la misma SQL de antes**: cada consulta arma la unión solo si el reporte incluye el historial; sin lote la
   fuente es `app.sales_recognitions r` y las medidas, las de antes (la prueba compara el JSON de los 27 reportes con y
   sin el parámetro). Con lote y «Solo AD-EL POS», las columnas del historial siguen (para que no cambien al cambiar el
   filtro) con «AD-EL POS» y diferencia 0.
2. **Juntar o separar**: un renglón de una dimensión que existe igual en los dos sistemas (día, semana, mes, hora, tasa,
   modalidad, área y mesa, categoría, producto y, en cobros, día y usuario importado) suma los dos y se marca «AD-EL POS
   y sistema anterior» (comparar años y ver un producto importado vendido en los dos como un solo renglón); las ventas,
   los cortes, las cajas, las formas de pago, las cancelaciones, los cambios de cantidad, los complementos y los cortes
   de caja del anterior son entidades distintas y van en renglones propios. «Efectivo» del anterior no se junta con el
   método de AD-EL POS (son registros de sistemas distintos; la columna «Datos» los distingue).
3. **Diferencia del sistema anterior** (decisión 2 del propietario): medida propia `legacy_difference`, fuera del importe
   bruto e incluida en la venta neta (venta neta = bruto − promociones − descuentos − cortesías + cargos + diferencia);
   columna en los reportes por venta, día, semana, mes, hora, tasa, modalidad, mesa, origen y turno, y cifra resumida en
   todos los de ventas y productos. Los de productos solo cuentan renglones (con sus complementos) y concilian con las
   cifras de descuentos, cargos y diferencia. En «Ventas del periodo» los descuentos, cargos y diferencias del anterior
   (sin IVA propio) forman el renglón «Sin tasa (sistema anterior)».
4. **Cuentas**: cada venta del anterior cuenta una vez; en la unión, un UUID sintético de la versión 0 por venta (los de
   AD-EL POS son de la versión 4) y la secuencia del historial por debajo de la de AD-EL POS, para que en un renglón
   combinado los nombres sean los más recientes de AD-EL POS.
5. **Filtros**: los de algo que solo existe en AD-EL POS (caja, turno, método de pago, lista de precios, motivo del
   catálogo, quién canceló o modificó, origen distinto de «Sistema anterior») excluyen el historial; los de lo importado
   (producto, categoría, área, mesa, usuario) lo filtran por el identificador nuevo; «Sin motivo del catálogo» incluye
   las cancelaciones del anterior.
6. **Cobros**: todos los pagos del anterior en su fecha, también los de importe cero o negativo y los de ventas
   canceladas o abiertas (los cuenta así la validación §4.4 por forma de pago), sin devoluciones, correcciones ni
   propinas; fuera del alcance de Caja y de los turnos en arqueo (como pide el encargo). En «Detalle de cobros» se rotulan
   «Cobro de una venta cancelada» o «abierta».
7. **Cortes del anterior en «Turnos de caja»**: tal como se guardaron, sin esperado (no se recalcula); su diferencia
   guardada exige `cash.expected.read` y **solo se muestran a quien consulta en Caja todas las cajas y el historial**
   (decisión propia: son turnos cerrados de otras cajas; los cobros sí quedan fuera del alcance, como pide el encargo).
   No se agregaron a «Ventas y cobros por turno», «Historial por caja» ni «Caja por día».
8. **Cancelaciones y cambios de cantidad**: motivo «Sin motivo (sistema anterior)», importe = lo que se debía (lo que el
   anterior contaba como vendido), quién autorizó; en «Modificaciones de cantidades», los cambios frente a la cantidad
   original del renglón (también aumentos), sin los registros sin cambio de cantidad ni el importe (no se guardaba).
9. **Resumen ejecutivo sin historial**: es la operación de AD-EL POS, no tiene filtros y mezcla caja, inventario y costos
   sin equivalente; la comparación con años anteriores se hace en «Evolución de ventas» y «Tendencia de demanda», que sí
   lo incluyen. «Evolución de ventas» tiene el filtro pero no la columna «Datos» (compara cubetas de tiempo).
10. **Sin índices nuevos**: la unión empuja el periodo a cada rama y usa `legacy_sales_recognitions_recognized_idx`; el
    reporte más lento con el volumen real fue de 450 ms. Las canceladas del anterior recorren `legacy_sales` (10 ms con
    230 mil ventas): un índice parcial no se justificó.

### Pruebas ejecutadas (aisladas)

- `go-unit` (`ISO_TAG=D2`): aprobado (vet normal e `integration`, `go test -race ./...`, `GOOS=windows` build y vet).
  Unitarias nuevas `reports/legacy_test.go`: `TestLegacyReportsDeclareDataSource`, `TestResolveLegacy`,
  `TestMergeRows`.
- `go-int ./internal/reports` completo: **aprobado** (137 s), con las pruebas nuevas
  `TestStage9HReportsWithoutLegacyBatch` (sin lote: catálogo sin «Datos» ni columnas del historial; los 27 reportes dan
  el mismo JSON sin el parámetro y con `all`, `native` y `legacy`) y `TestStage9HLegacyHistoryInReports` (catálogo y
  metadatos; cada uno de los 27 reportes con los tres valores en 2025 —solo historial— y en un periodo con los dos
  sistemas —cada total igual a AD-EL POS + anterior—; totales de 2025 contra las tablas del lote; día comercial a las
  04:59 y 05:00, hora de inicio cambiada a las 04:00 y de vuelta, los dos cambios de horario de 2021 en Monterrey;
  cuatro decimales redondeados solo al mostrar; diferencia fuera de los productos; cancelada fuera de la venta;
  «Sin turno»; alcance de Caja limitado; producto importado vendido en los dos sistemas en un solo renglón; resumen
  ejecutivo sin historial; PDF y Excel con «Datos: Solo sistema anterior» y la marca; filtros de AD-EL POS que excluyen el
  historial). Las pruebas anteriores del paquete (aceptación, exportaciones, 9C, 9D) siguen aprobadas sin cambios.
- `go-int ./internal/reports/operational`: **4 subpruebas fallan igual con y sin este cambio**
  (`turno en arqueo excluido…`, `cierre definitivo con sobrante y cortes`, `esperado y diferencias protegidos…`,
  `compras por periodo…`): la prueba depende de la hora; entre las 00:00 y las 05:00 de Monterrey (se corrió a la 01:45)
  la fecha calendario de un turno abierto ahora queda fuera del rango de días comerciales que consulta. Se comprobó
  corriendo `TestOperationalReports` sobre una copia del backend con el paquete `reports` original de `etapa-9h`: mismas
  4 fallas. Las demás subpruebas aprobadas.
- `vitest` (`tsc --noEmit` + 94 archivos, 885 pruebas): aprobado, con la nueva «“Datos” (historial del sistema anterior)
  sale del catálogo de Go con su valor por omisión».
- `cargo` (`cargo test --locked --offline`): aprobado (89 pruebas, 3 ignoradas), con `data_source` en
  `report_routes_are_closed_read_only_and_keyed`.
- `fmt-check` y `sync-check`: aprobados.
- **Medición** (`ADEL_REPORTS_MEASURE=1`, `TestStage9HLegacyReportsVolume`, sin `-race`, `$SP/measure-d2.sh`):
  historial sintético importado con el importador real: 230 240 ventas (10/2019–09/2026), 719 258 renglones del libro,
  230 242 pagos; generar e importar, 23 s; base de 784 MB. `reports.Run` completo en el ejecutor real:

  | Reporte (año 2025, ≈ 33 mil ventas)             | Los dos | Solo anterior | Solo AD-EL POS |
  | ----------------------------------------------- | ------- | ------------- | -------------- |
  | `sales.summary`                                 | 159 ms  | 155 ms        | 1 ms           |
  | `sales.by_day` (365 renglones)                  | 157 ms  | 180 ms        | 2 ms           |
  | `sales.by_week`                                 | 242 ms  | 309 ms        | 1 ms           |
  | `sales.by_month`                                | 184 ms  | 205 ms        | 1 ms           |
  | `sales.by_product`                              | 210 ms  | 238 ms        | 1 ms           |
  | `sales.accounts` (32 883 renglones)             | 400 ms  | 392 ms        | 1 ms           |
  | `sales.by_shift`                                | 213 ms  | 189 ms        | 2 ms           |
  | `sales.evolution` (contra 2024)                 | 58 ms   | 101 ms        | 1 ms           |
  | `products.trends` (contra 2024)                 | 315 ms  | 346 ms        | 1 ms           |
  | `products.complements`                          | 47 ms   | 42 ms         | 6 ms           |
  | `payments.by_method`                            | 15 ms   | 8 ms          | 1 ms           |
  | `payments.list` (32 885 renglones)              | 123 ms  | 118 ms        | 2 ms           |
  | `sales.voided_accounts`, `cash.shifts`, cambios | ≤ 14 ms | ≤ 12 ms       | ≤ 2 ms         |

  Con el periodo más largo que se admite (400 días, 2024–2025): el más lento, `sales.accounts` (36 036 renglones),
  450 ms; `products.trends` 425 ms; los demás de ventas y productos entre 116 y 343 ms y los de cobros entre 8 y 125 ms.
  Planes (`EXPLAIN ANALYZE`): la unión empuja el periodo a las dos ramas (índice `legacy_sales_recognitions_recognized_idx`
  en el historial; la consulta de ventas por día de 2025, 29 ms); las canceladas recorren `legacy_sales` (10 ms).

- `docs/etapas/*.md` sin cambios.

### No verificado

- El respaldo real (Fase E): los reportes del historial con los datos reales y su comparación con el sistema anterior.
- La interfaz con el filtro y las columnas en la ventana nativa y en Playwright (no se agregó especificación: la pantalla
  no cambió y el filtro sale del catálogo; cubierto por Vitest y por las rutas de Go). El PDF y el Excel con el
  historial no se abrieron ni imprimieron (solo su texto y sus hojas).
- `TestOperationalReports` completo después de las 05:00 de Monterrey (ver «Pruebas ejecutadas»); conviene repetirlo
  en la regresión de la Fase F.
- Comparar «todos los años» en un solo reporte: el periodo sigue limitado a 400 días (se compara año por año).

### Para CLAUDE.md

- §15 (Etapa 9H) y §6: con un lote importado vigente, 27 reportes (`docs/REPORTS.md`, «El historial del sistema
  anterior») incluyen `app.legacy_*` con el filtro «Datos» (`data_source`: `all` por omisión, `native`, `legacy`) y las
  columnas «Datos» y «Diferencia del sistema anterior»; el resumen ejecutivo no. Sin lote, cada reporte es exactamente el
  de antes (catálogo, columnas, metadatos y SQL).
- Regla permanente propuesta: «Un reporte que incluya el historial del sistema anterior lo declara con `withDataSource`
  y `withLegacyColumns` (`reports/legacy.go`), usa `query.ledgerFrom()`/`ledgerExtras()` o una consulta propia cuando
  `query.legacy.History`, y sin lote vigente ejecuta la misma consulta de antes; los importes del historial se suman
  exactos (4 decimales) y se redondean solo al mostrar; la «Diferencia del sistema anterior» nunca se reparte entre
  productos; los cobros del historial quedan fuera del alcance de Caja».
- §6, pruebas: `TestStage9HReportsWithoutLegacyBatch` y `TestStage9HLegacyHistoryInReports` (`internal/reports`, cargan
  el historial con el importador real); medición opcional `TestStage9HLegacyReportsVolume` con
  `ADEL_REPORTS_MEASURE=1` (sin `-race`). `TestOperationalReports` falla entre las 00:00 y las 05:00 de Monterrey
  (dependencia de la hora anterior a esta fase).
- Puente: `data_source` en `allowed_query_keys` de `reports_*` (`api.rs`).
