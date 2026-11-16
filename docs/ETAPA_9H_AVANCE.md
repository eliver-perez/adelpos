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
