# Etapa 9A · Avance: ajustes pendientes, reportes, indicadores y exportaciones

Rama `etapa-9a`, creada desde `main` (`579e845`, versión 0.8.0, esquema 14, con 0010–0014 aplicadas en la instalación
real). La especificación del propietario es `docs/etapas/ETAPA_9A.md`, que no se modifica. Las migraciones nuevas (0015
en adelante) **no se aplican a la instalación real** sin autorización expresa del propietario (§30). Nada de la etapa
9B (identidad visual, logotipo, diseñador de documentos) se implementa aquí.

Las fases son las de la tabla de §0 (0, A, B, C y D); la frase final de la especificación («0 → E») se interpreta como
esa misma tabla.

## Fase 0 · Verificación contra el código

Revisión de solo lectura del repositorio y del sistema anterior (`../legacy-csharp/`, sin compilar, ejecutar ni leer
archivos de configuración). Ningún hallazgo obliga a cambiar una decisión del propietario: lo que el modelo actual no
resuelve se implementa respetando §0-B y queda anotado abajo. La etapa continúa sin detenerse.

### 1. Equivalencia por valor con piso y la opción de §3.1

- **Orden del cálculo** (`catalog/calculator.go`), para cada partida:
  1. base: producto, variante o lista de precios;
  2. extras de ingredientes;
  3. grupos de complementos;
  4. componentes de combo.

  Después, las promociones sobre toda la cotización. Los descuentos manuales de la etapa 6 se aplican en `payments`
  sobre el importe ya promocionado. El orden de §0-B (complementos → promoción → descuento manual) ya lo garantiza la
  estructura.

- **Fórmula del grupo** (`group_calculator.go`): en modo valor, `diferencia = max(0, Σ solicitado × valor de
referencia − Σ incluido × valor de referencia)`, con los extras explícitos aparte. En modo cantidad se cobran
  tramos.
- **La opción** («El precio puede bajar al cambiar complementos», desactivada por omisión):
  - **Dónde vive:** es una columna nueva del producto que el snapshot del pedido congela, y la cotización guarda el
    ajuste con signo.
  - **Desactivada:** el cálculo es exactamente el actual.
  - **Activada:** la diferencia con signo baja el núcleo del precio, nunca por debajo del valor de los complementos que
    permanecen ni de cero. Los extras y cargos independientes se suman después, sin descontarse.
  - **Promociones:** su base pasa a ser el precio ya ajustado, para no acreditar dos veces la misma diferencia.
  - **Combos:** el ajuste negativo de un componente solo aplica si el combo también tiene la opción.
  - **IVA:** está incluido en el precio, así que no cambia su regla.

### 2. Datos para las métricas de §6 y sus fechas

- **Venta comercial:**
  - unidades (`restaurant_item_units`, con su cuenta actual e importes);
  - versiones congeladas (`quote_line`, tasa de IVA);
  - ajustes (`account_adjustments`, con nombre histórico del motivo y asignaciones por unidad).

  El IVA se calcula por cuenta y tasa (`payments/finance.go`). Dividir, transferir o combinar solo cambia la cuenta de
  la unidad, así que cada unidad se cuenta una sola vez.

- **Cierre de una cuenta:** `restaurant_accounts.closed_at`. **Hallazgo:** una devolución reabre la cuenta y borra
  `closed_at`, y al volver a liquidarse cambia de fecha. Una cuenta reabierta cuyas unidades se cancelan queda abierta
  con saldo 0. Calcular la venta desde el estado actual movería o borraría la venta del día original, en contra de
  §0-B.

  **Solución:** un libro inmutable de reconocimiento de ventas que se escribe en la misma transacción en que la cuenta
  queda liquidada:
  - en el primer cierre registra la venta completa;
  - en una reliquidación registra solo las diferencias;
  - la devolución se reporta en su propio día.

  Las cuentas cerradas hoy se rellenan con la fecha de su primer cierre según la auditoría.

- **Cobros:** `payments` (método, caja, turno, usuario, dólares recibidos, tipo de cambio y cambio por moneda),
  `payment_refunds` y `cash_movements` (efecto físico por moneda).
- **Caja:**
  - `cash_shifts.business_date`, que es la fecha de apertura, sin hora de corte;
  - `cash_operations`;
  - cortes (`cash_shift_reports`) y cierres (`cash_shift_closures`, con diferencias por moneda).

  El esperado tiene un solo cálculo (`cash_expected.go`). Los reportes conservan la frontera de Caja y además excluyen
  los turnos en arqueo de todo agregado.

- **Inventario y compras:** movimientos con `occurred_at`, órdenes, recepciones (directas sin orden), devoluciones a
  proveedor y vínculos con gastos (que nunca son una segunda compra).
- **Fechas disponibles:**
  - confirmación (eventos de la cuenta);
  - cierre;
  - cobro y devolución (`created_at`);
  - liberación a cocina (`kitchen_unit_releases.released_at`);
  - movimiento (`occurred_at`);
  - recepción;
  - fecha comercial del turno.
- **Hora de inicio del día:** es `inventory_settings.operating_day_cutoff` (05:00), con la zona horaria del
  establecimiento. No se crea otra.

### 3. Costo de venta (§14)

Las partidas vendidas sí están enlazadas con su consumo: `inventory_movements.restaurant_unit_id`. El costo de una
unidad es −Σ del valor de sus movimientos `consumption` y `consumption_reversal`, sin contar los `cost_adjustment`, que
se reportan como concepto del periodo. Cada partida queda en uno de tres estados:

| Estado    | Cuándo                                                              |
| --------- | ------------------------------------------------------------------- |
| Estimado  | Algún consumo con costo pendiente o valorado al costo de referencia |
| Sin costo | Sin consumo registrado, o con solicitud pendiente o fallida         |
| Firme     | En cualquier otro caso                                              |

### 4. Interfaz, librerías de PDF/Excel y puente

- **Interfaz:** no hay librería de gráficos. Se crearán componentes SVG propios y ligeros, con tabla alternativa, y se
  reutilizan paginación, tarjetas de cifras, controles segmentados, campos de fecha y acordeones. El dashboard no se
  recalculará con cada evento SSE.
- **Librerías Go:**

  | Uso                           | Librería y versión                                                           | Licencia |
  | ----------------------------- | ---------------------------------------------------------------------------- | -------- |
  | Excel                         | `github.com/xuri/excelize/v2` v2.11.0                                        | BSD-3    |
  | PDF                           | `codeberg.org/go-pdf/fpdf` v0.12.0 (el repositorio de GitHub está archivado) | MIT      |
  | Fuentes Go, con acentos y «ñ» | `golang.org/x/image` v0.47.0                                                 | BSD-3    |

  Las tres funcionan sin red, se fijan en `go.sum` y se registran en `docs/DEPENDENCIES.md`.

- **Puente:** hoy solo transporta JSON. La exportación se hará con una operación cerrada nueva y el diálogo nativo de
  guardado de Tauri (`tauri-plugin-dialog` 2.8.3), usado solo desde Rust.
- **Infraestructura de consultas:** cada petición abre hoy una transacción de escritura, y las lecturas de restaurante
  y cobros toman el lock 7820148305 compartido. Los reportes autentican en una transacción corta y consultan en otra de
  solo lectura (`REPEATABLE READ READ ONLY`). Esa segunda transacción:
  - usa un número limitado de conexiones;
  - tiene límite de tiempo;
  - no toma ningún lock operativo.

### 5. Sistema anterior

- **Fecha de la venta:** solo contaba las cuentas liquidadas, pero **las agrupaba por la fecha de apertura**, no por la
  de cierre. Por eso, la afirmación de §0-B de que contaba la venta en el día de cierre no corresponde al código
  disponible.
- **Semana del año:** no era ISO 8601; en algunos años se adelantaba una semana y nunca había semana 53.
- **Totales:** solo mostraba subtotal, IVA y total en el reporte de entradas a inventario y en el detalle de una venta,
  no en el resumen de ventas.
- **Lo que no tenía:** propinas registradas, dólares físicos, devoluciones, costos, cierre ciego, exportaciones,
  gráficos ni dashboard.
- **Funciones que no están en el plan** (se reportan sin implementar):
  - reporte de modificaciones de cantidades autorizadas;
  - lista de cuentas anuladas con quién autorizó;
  - columna «Recibió» en compras;
  - folio del comprobante de tarjeta;
  - conteo de canceladas en el resumen.

Ganan las decisiones de §0-B (cierre como fecha de la venta y semana ISO).

### Decisiones menores tomadas en la verificación

1. Venta reconocida en el primer cierre mediante el libro inmutable de reconocimientos. Una cuenta reabierta que nunca
   vuelve a cerrarse conserva su reconocimiento y aparece en «En curso» como reabierta.
2. Ventas por turno: la venta pertenece al turno de la operación que cerró la cuenta; los cobros, al turno de cada
   pago.
3. Los descuentos se fechan con el reconocimiento de su venta, para que su porcentaje sobre ventas cuadre.
4. Caja: igual frontera que Caja y exclusión total de los turnos en arqueo.
5. No se reporta quién capturó: el borrador se borra al confirmar. Sí quién confirmó, quién es el responsable y quién
   cobró, descontó, autorizó, devolvió o registró movimientos de caja.
6. «Compras de mostrador» será un proveedor especial único, con el comercio concreto como referencia opcional de la
   recepción.
7. En Excel, el texto que podría tomarse como fórmula se escribe con un apóstrofo inicial, y el archivo no contiene
   fórmulas.

## Fase A · parte A1: precio de complementos configurable y ajustes de §33

Migración `0015_catalog_price_and_precheck.sql` (transaccional, sin datos, no toca 0001–0014 ni filas
existentes): `catalog_products.price_can_decrease boolean NOT NULL DEFAULT false` y
`payment_documents.account_revision` / `adjustment_count` (`integer`, NULL en históricos) con el CHECK
con nombre `payment_documents_precheck_version_check` (solo una precuenta los lleva, siempre juntos,
revisión > 0 y ajustes ≥ 0; NULL-seguro). Sin permisos, rutas ni locks nuevos.

### §3.1 «El precio puede bajar al cambiar complementos»

- **Go** (`catalog/`): `Product.PriceCanDecrease` (persistido en `saveProduct`, leído por `to_jsonb`,
  copiado al duplicar, congelado en `pricing_snapshot`); cambiarlo exige `catalog.prices.manage`
  (`product_permissions.go`) y se audita con el guardado. Motor: con la opción desactivada el camino es
  el de siempre (prueba de no regresión que compara el JSON de diez cotizaciones con huellas SHA-256
  calculadas con el código anterior, `da182ee`). Con la opción: diferencia con signo por grupo (valor:
  solicitado − incluido, incluido quitar sin reemplazo; cantidad: tramos − valor de referencia de las
  porciones incluidas retiradas; sin valores de referencia no cambia), núcleo
  `min(núcleo con piso, max(base + Σ diferencias con signo [+ reducciones de componentes], valor de lo
que permanece, 0))`, y después extras explícitos, ingredientes y cargos de componentes.
  `ProductBreakdown` agrega `price_can_decrease` y `complement_adjustment` con `omitempty` (la
  reducción efectiva, ≤ 0, ya incluida en `group_extras` y `unit_price`). Promociones: base sin extras
  `max(0, base + complement_adjustment) × unidades`. Cocina: las partes nunca llevan importes del
  producto y la diferencia del grupo ya se vaciaba; se añadió comentario y prueba
  (`kitchen/preparation_data.go`, `preparation_data_test.go`).
- **Interfaz**: interruptor «El precio puede bajar al cambiar complementos» junto a precio y tasa de IVA
  (`ProductEditor.tsx`, `ToggleButton`, explicación ligada con `aria-describedby`, desactivado sin
  `catalog.prices.manage`); `price_can_decrease: false` en el producto nuevo (`catalogState.ts`).
  Captura: el grupo muestra «−$10.00» / «+$5.00» / «Sin cambio» con `formatSignedMoney` sobre lo que
  entrega Go (`CustomizationBlocks.tsx`, `nonZeroAmount` en `captureRules.ts`). Desglose
  (`QuoteBreakdown.tsx`): «Diferencia» (ya no «Diferencia positiva») con signo y, con la opción, la
  reducción aplicada.

### §33a «Poner fuera de servicio»

`TablePanel.tsx` + `styles.css`: el botón va en su propia fila de las acciones secundarias, a todo el
ancho del pie del panel (`.table-service-action`, `flex-wrap` solo en `.table-panel-footer`), en
cualquier tamaño de ventana. Sin cambio de reglas ni de texto. Playwright: una línea a 1280 × 720 y
1920 × 1080 en claro y oscuro (con el estilo anterior, tres líneas: control negativo ejecutado).

### §33b Vigencia de la precuenta

`payments/documents.go` guarda en la precuenta la revisión de la cuenta (`Finance.Revision`, leída bajo
el lock exclusivo) y el número de ajustes (`adjustmentCountSQL`, la misma expresión que la vigencia).
`payments/standing.go`: columna `precheck_current` en `AccountStandingSQL`, predicado único (precuenta
con la revisión y el número de ajustes actuales; filas NULL con la regla por marcas de tiempo).
`operations/tables.go` la usa para `awaiting_payment`. Los ajustes **no** cambian la revisión de la
cuenta.

### Decisiones menores

1. `complement_adjustment` es la reducción respecto de la regla con piso (≤ 0, «0.00» si no bajó), no
   la suma con signo de las diferencias; así la fórmula de promociones del diseño
   (`(BasePrice + complement_adjustment) × qty`) nunca acredita lo ya descontado ni cambia el trato de
   las diferencias positivas.
2. Además de los límites del diseño, el núcleo nunca supera el de la regla con piso: en un producto
   cuya base es menor que el valor de sus complementos incluidos, el límite del «valor que permanece»
   habría subido el precio al activar la opción. La opción solo permite bajar.
3. Con la opción, `group_extras` lleva el efecto neto de los complementos (puede ser negativo) para
   conservar `unit_price = base + ingredientes + complementos + componentes`.
4. Combos: el componente se calcula con su opción solo si el combo también la tiene (el desglose del
   componente lleva entonces `price_can_decrease`); su reducción no rebaja su cargo (sustitución,
   tramos y extras siguen completos) y se aplica en el núcleo del combo, cuyo límite incluye el valor de
   los complementos que permanecen en sus componentes.
5. Precio en ediciones: como con un cambio de precio, una modificación que vuelve a cotizar una
   partida usa la opción vigente; cambiar solo observaciones conserva los importes.
6. Precuenta: solo las precuentas guardan revisión y ajustes (los tickets quedan en NULL). Se usa la
   revisión con que se armó el documento (`Finance.Revision`), leída con el lock exclusivo ya tomado.
7. §33a: fila propia (no rótulo corto), para no cambiar el texto ni su nombre accesible.

### Pruebas (ejecutadas en `scratchpad/repo9aA1`, última sincronización con el árbol de las tres partes)

- Go: `go vet ./...` y `go vet -tags=integration ./...` sin hallazgos; `go test -race -count=1 ./...`
  todo `ok`. Nuevas: `catalog/price_decrease_test.go` (piso intacto sin la opción, JSON idéntico al
  anterior en diez escenarios con huellas calculadas sobre `da182ee`, diferencia positiva y negativa,
  ejemplo $150/$35/$25 → $140, quitar sin reemplazo, límite de cero, límite por valor que permanece,
  la opción nunca sube, extras e ingredientes no descontados, modo cantidad con y sin valores de
  referencia, presentación, listas, combo con y sin la opción en combo y componente, 2×1, porcentaje
  con y sin extras y precio especial sin doble descuento, base de promoción nunca negativa, permiso) y
  `kitchen/preparation_data_test.go`. La huella heredada de
  `operations/quick_notes_fingerprint_test.go` sigue igual.
- Integración PostgreSQL (clúster 55499, ejecución final): `internal/catalog` ok (9.4 s),
  `internal/database` ok (5.8 s, con `TestStage9ACatalogAndPrecheckUpgrade`), `internal/payments` ok;
  `internal/operations` (226.6 s): pasan las siete subpruebas 9A (`9A_31_*` y `9A_33_*`) y todo lo
  anterior salvo `inventory_settings_feature_and_events` (`inventory_ledger_test.go:427`: «La hora de
  inicio del día se cambia en Configuración general.»), cambio en curso de otra parte (hora de inicio
  del día), ajeno a A1. Una ejecución completa previa (`collect.go` de `HEAD` en la copia, antes de
  ese cambio) aprobó todo `internal/operations` (222.6 s).
  - `operations/stage9a_test.go`: cotización y confirmación con la opción, IVA 8 % sobre el importe
    reducido (129.63 + 10.37), historial (cambiar la opción no altera la partida; observaciones
    conservan importes; partida nueva sin la opción a $150), promoción 10 % sobre $140 y descuento
    manual posterior (−14.00 y −12.60, unidad 140/14/126, IVA 105.00 + 8.40), precuenta con revisión y
    ajustes, cargo y cancelación la invalidan sin cambiar la revisión, **dos carreras controladas**
    (borrador y cargo que inician su transacción antes que la impresión, detenidos al autenticarse por
    un bloqueo de la fila de su terminal y luego en espera del lock 7820148305 detrás de la impresión;
    la prueba verifica que la regla anterior por marcas de tiempo los habría mostrado vigentes),
    precuentas históricas con la regla anterior. Control negativo ejecutado: con el predicado anterior
    en `tables.go` las dos carreras fallan («awaiting_payment»).
  - `catalog/acceptance_test.go`: opción desactivada por omisión, 403 sin `catalog.prices.manage` sin
    cambios, guardado auditado, cotización $140, duplicado la conserva, desactivarla vuelve a $150.
  - `database/stage9a_catalog_precheck_upgrade_test.go`: esquema 14 con productos, combo, precuenta y
    ticket → la 0015 no altera nada (comparación de `to_jsonb` sin las columnas nuevas), opción `false`
    `NOT NULL`, documentos históricos en NULL, CHECK con nombre probado con una copia temporal.
- Vitest: 50 archivos, 430 pruebas ok (nuevas: `pages/catalog/priceDecrease.test.tsx` y una prueba en
  `capture/quickNotesCapture.test.tsx`). `tsc --noEmit` sin errores; `npm run build` ok;
  `cargo test --locked --offline` 20 ok, 1 ignorada (sin cambios en Rust).
- Playwright (puertos 8184/1424), recorrido completo: Chromium 8/8 y WebKit 8/8, con el nuevo
  `e2e/table-price-decrease.spec.ts`: producto con la opción, quitar el arroz (−$20.00, $130.00) y
  sustituirlo por frijoles (−$10.00, $140.00), partida a $140.00; «Poner fuera de servicio» en una línea
  a 1280 × 720 y 1920 × 1080 en claro y oscuro y la regla intacta (fuera de servicio y habilitar).
  Control negativo: sin la clase nueva, el texto ocupa 3 líneas a 1280 × 720. Una corrida anterior
  (interrumpida) tuvo en WebKit un error de página «The provided value is non-finite» en
  `restaurant.spec.ts` que no se reprodujo en las dos corridas siguientes.

### Limitaciones

- Si el límite actúa (producto cuya base es menor que el valor de sus complementos incluidos), la
  diferencia del grupo que ve el mesero puede ser mayor que la reducción real del producto; el precio
  del botón y el desglose («El precio bajó …») muestran el efecto real.
- No verificado en la ventana nativa (WKWebView) ni en pantalla táctil física.

### Para fusionar (archivos que A1 no edita)

- `CLAUDE.md` §4 y la lista de especificaciones de Playwright: agregar `e2e/table-price-decrease.spec.ts`.
- `API_CONTRACTS.md`: ver `api_contracts.md` de esta carpeta. Sin dependencias nuevas.
- Edición mínima fuera del área estricta: una línea en `backend/internal/operations/acceptance_test.go`
  (llamada a `testStage9A` al final de `TestRestaurantAcceptance`) y dos bloques en
  `desktop/src/styles.css` (campo del interruptor y fila propia de «Poner fuera de servicio»).

### Fase A · A2: proveedores, compras de mostrador, código sugerido, categorías y navegación de Inventarios

Migración `0016_inventory_suppliers_counter.sql` (transaccional, sin `CONCURRENTLY`, restricciones con nombre, CHECK
seguros ante NULL). **No se aplicó a la instalación real.** Guía actualizada: `docs/INVENTORY_AND_PURCHASING.md`
(secciones marcadas «(9A)»).

#### §3.3 Datos adicionales de proveedores

- `phone_secondary`, `mobile_phone` y `website` (`text NOT NULL DEFAULT ''`, CHECK de largo y sin espacios en los
  extremos; la web sin espacios). Go valida solo estos campos: teléfonos con 7–20 dígitos, caracteres de teléfono y
  extensión opcional; web con dominio y ruta opcional, solo `http(s)://` o sin esquema. Los proveedores existentes se
  siguen guardando (el teléfono principal conserva la validación de la etapa 8).
- Búsqueda en Go por los teléfonos nuevos y la web; formulario con «Segundo teléfono», «Teléfono celular» y «Página web»
  (mostrada como texto, nunca como enlace); la lista muestra el celular en el contacto.

#### §3.2 «Compras de mostrador»

- Proveedor especial único (`inventory_suppliers.kind = 'counter'`, índice único parcial
  `inventory_suppliers_counter_key`, CHECK `inventory_suppliers_counter_check` sin RFC ni razón social). La 0016 lo siembra
  una vez; si ya existe un proveedor «Compras de mostrador» no se toca ni se convierte y el especial toma el primer nombre
  libre («… (general)», «… (general 2)»…). No crea compras, recepciones, movimientos, insumos surtidos ni precios.
- `inventory_receipts.counter_store` (comercio concreto, opcional, ≤ 100). Go lo rechaza con proveedor formal.
- Reglas en Go: rechazado (409 `COUNTER_SUPPLIER`) en órdenes nuevas, al cambiar el proveedor de un borrador, en el
  borrador desde sugerencias y en insumos surtidos; nunca preferido; sus recepciones no registran el último precio; la
  modalidad no se crea ni cambia por la API. `SupplierReference.kind` y `ReceiptSummary.counter_store` distinguen la
  modalidad en listas y reportes; filtros `suppliers?kind=` y `receipts?supplier_kind=`.
- Interfaz: botón «Compra de mostrador» en Recepciones (recepción directa con la modalidad preseleccionada, sin elegir
  proveedor y con «Comercio (opcional)»); la recepción directa ofrece «Proveedor registrado» / «Compras de mostrador»; el
  selector de proveedor de órdenes, sugerencias y recepción formal solo ofrece formales; etiquetas «Compra de mostrador»
  y «Mostrador · comercio» en listas y detalle; filtro «Compras de mostrador» en Recepciones; el editor del proveedor de
  mostrador oculta RFC, razón social e insumos surtidos y explica su uso.

#### §3.4 Código interno sugerido

- `services/productCode.ts` generalizado (`maxLength`, `freeCode`, `suggestVariantCode`, `followsSuggestion`) sin
  cambiar el comportamiento de productos (60 caracteres).
- Insumos: el alta sugiere el código desde el nombre (40 caracteres) y pregunta a Go por la ruta nueva
  `GET inventory/item-codes` (puente `inventory_item_code`), que dice si está libre, quién lo usa y el primero libre con
  `-2`, `-3`… (misma regla en `inventory/item_codes.go`). La sugerencia sigue al nombre solo mientras el usuario no
  escribe el código; un código escrito nunca se sobrescribe (la pista avisa si está tomado y cuál está libre). Al editar
  un insumo no se sugiere; se comprueba excluyendo el propio insumo. `DUPLICATE_CODE` sigue al guardar.
- Mismo patrón (sin ruta nueva) en la copia de un producto («Código interno de la copia» sugerido desde «nombre (copia)»
  con los códigos de productos conocidos) y en el código de una presentación nueva de un producto
  (`<código del producto>-<nombre de la presentación>`). Nunca cambian códigos guardados ni UUID.

#### §3.5 Categorías de insumos

- Configuración → Categorías: búsqueda (en pantalla, sin distinguir mayúsculas ni acentos), número de insumos, estado,
  «Ver insumos» (Insumos filtrado por la categoría; `InventoryTarget.items.categoryID`), «Activar»/«Desactivar» con
  confirmación y «Editar» (nombre, posición, estado). Duplicados: Go (`DUPLICATE_NAME`).
- «+ Nueva categoría» en el selector del editor del insumo (con `inventory.items.manage`): diálogo propio fuera del
  `<form>` del insumo que crea la categoría, la agrega a la lista local, la selecciona y conserva lo capturado sin crear el
  insumo; avisa antes si el nombre ya está en la lista.

#### §3.6 Navegación de Inventarios

- Cinco grupos (Resumen · Catálogo · Existencias · Compras · Configuración) y una segunda fila con las secciones del grupo
  (`pages/inventory/inventorySections.ts`), ambas en una sola línea de columnas iguales. Secciones nuevas: Sugerencias de
  compra (sacada de Compras), Devoluciones a proveedor (lista nueva con `GET supplier-returns`), Unidades y Categorías
  (sacadas de los acordeones de Insumos); «Compras» pasa a «Órdenes de compra» y «Configuración» a «Parámetros». Permisos
  por sección iguales a la etapa 8; un grupo sin secciones no aparece; la segunda fila se omite solo en Resumen. Cada
  grupo recuerda su última sección mientras la pantalla está montada. `InventoryTarget` se conserva (y `purchases` con
  `subsection: 'suggestions'` abre Sugerencias). El menú principal no cambió.

#### Decisiones menores

1. Nombre reservado del proveedor especial: «Compras de mostrador»; con conflicto, «Compras de mostrador (general)» o
   «(general N)». Se puede renombrar y desactivar; desactivado, la modalidad queda deshabilitada en la recepción directa.
2. Las compras de mostrador no registran último precio ni insumos surtidos (no contaminan preferidos ni sugerencias).
3. `counter_store` con proveedor formal se rechaza (400) en lugar de ignorarse.
4. Las sugerencias de compra conservan la visibilidad de la etapa 8 (permisos de compras), aunque Go las lee con
   `inventory.read`.
5. La regla de sufijos del código existe en TypeScript (productos y presentaciones, con la lista conocida) y en Go
   (insumos, con todos los códigos); ambas con los mismos casos de prueba.
6. `PATCH suppliers/{id}` sigue guardando todos los datos: un cuerpo sin los campos nuevos los deja vacíos (como el resto
   de campos de la etapa 8).
7. Etiquetas de sección: «Órdenes de compra», «Sugerencias de compra», «Devoluciones a proveedor», «Unidades»,
   «Categorías», «Parámetros»; el grupo y la sección «Existencias» comparten nombre (la sección es la primera del grupo).

#### Limitaciones

- La comprobación del código es informativa (dos altas simultáneas con el mismo código: la segunda recibe
  `DUPLICATE_CODE`). La sugerencia de presentaciones solo conoce los códigos de productos y del mismo producto.
- La sección recordada por grupo se pierde al salir de Inventarios.
- La página web del proveedor no se abre desde la aplicación (texto).
- Sin verificar en la ventana nativa (WKWebView) ni en pantalla táctil física.
