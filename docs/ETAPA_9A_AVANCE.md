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

## Fase B · B1 · Infraestructura de reportes, día comercial, reconocimiento de ventas y reportes de ventas, productos, pagos y descuentos

Migración `0017_reports.sql` (sin aplicar a la instalación real). Paquetes nuevos `internal/reports` y
`internal/businessday`; reconocimiento de ventas en `internal/payments` (`recognition*.go`).

### Infraestructura

- **Rutas** `reports/catalog`, `reports/run/{id}`, `reports/dashboard` y `reports/filters/{source}` (todas GET,
  `reports.read`), con su rama propia en `administration/routes.go`. La autenticación, los permisos y la forma de
  los parámetros se validan en la transacción corta habitual (las denegaciones se auditan) y se confirman; la
  consulta corre después en otra transacción **`REPEATABLE READ READ ONLY`** con `statement_timeout` 30 s,
  `lock_timeout` 2 s e `idle_in_transaction_session_timeout` 45 s, sobre un **semáforo propio de 2 conexiones**
  (lleno: `503 REPORTS_BUSY`, «Hay otros reportes en curso…», sin esperar). Sin locks consultivos ni handlers de
  otros módulos (solo SQL propio, `payments.AccountStandingSQL` y `businessday`). Tiempo de respuesta HTTP de
  `reports/*`: 40 s (`http.go`); el puente usa 45 s.
- **Registro cerrado** de reportes (`Definition`: id estable, grupo, título, permisos, periodo, comparación,
  fecha usada, filtros admitidos, columnas tipadas, orden y función de consulta). Ninguna ruta acepta SQL; un
  parámetro no declarado por el reporte se rechaza.
- **Modelo de resultado único** (`reports.Result`) para pantalla y exportaciones: metadatos (título, periodo
  resuelto con sus instantes, comparación, filtros legibles, fecha de generación = `now()` de la fotografía, zona,
  hora de inicio, fecha usada, avisos, notas), columnas con tipo (texto, código, entero, cantidad, MXN, USD,
  porcentaje, fecha, fecha y hora) y visibilidad, página del detalle, **totales calculados en Go sobre todo el
  conjunto filtrado** y cifras resumidas. Las columnas con permiso propio (efectivo físico) no se envían, no se
  totalizan y no se pueden usar para ordenar sin ese permiso. Valores tipados en Go; dinero como cadena.
- **Puente**: `reports_catalog`, `reports_run`, `reports_dashboard`, `reports_filters`,
  `settings_business_day`, `save_settings_business_day` en `api.rs` (la clave del reporte se valida en Rust:
  minúsculas, `.` y `_`), lista blanca de parámetros `reports_*` y prueba Rust. Tipos en
  `desktop/src/services/reports.ts`.

### Día comercial y periodos

- `internal/businessday`: el cálculo del día operativo de la etapa 8 se movió a un paquete neutral (`DayOf`,
  `CutoffInstant`, `Config.Bounds`, `Load`); inventario lo usa sin cambiar su comportamiento (sus pruebas de
  día operativo, incluidos los cambios de horario, siguen pasando sin cambios).
- Periodos en Go: hoy, ayer, semana actual (ISO), mes actual, año actual, día, **semana ISO 8601** (`AAAA-Wss`,
  con su rango; semana 53 y cambio de año), mes, año y rango (≤ 400 días), con el periodo anterior equivalente
  (día, semana ISO, mes o año anterior; en un rango, los mismos días inmediatamente antes). Instantes
  `[desde, hasta)` con zona y hora de inicio; agrupación en SQL con `((ts AT TIME ZONE zona) - inicio)::date`
  (difiere de Go solo en la hora repetida de un atraso de horario junto a la hora de inicio).
- **«Hora de inicio del día»**: un solo valor (`inventory_settings.operating_day_cutoff`), editable **solo** en
  Configuración general (`PUT settings/business-day`, `settings.manage`, con revisión optimista, auditoría
  `settings.business_day_changed` y evento), con explicación y ejemplo en pantalla («con 05:00 una venta de la
  01:30 pertenece al día anterior»). Inventarios → Parámetros la muestra en solo lectura y su `PUT` la rechaza
  si cambia. Cambiarla no reescribe registros.

### Reconocimiento de ventas (0017)

- Libro inmutable **`app.sales_recognitions`** (disparadores de solo inserción), escrito por
  `payments.closeIfSettled` en la misma transacción del cierre (cobro, ajuste o entrega). Fuentes: `unit`
  (unidad activa: cantidad 1, bruto, promoción, neto), `allocation` (descuento o cortesía por partida asignado a
  la unidad) y `adjustment` (cargo general con su tasa; descuento o cortesía general repartido entre tasas en
  proporción, con `allocate`, y el último registrado recibe lo que falta en cada tasa). Clave de la fuente:
  (clase, unidad, ajuste, tasa).
- Primer cierre = todo; **reliquidación** tras una reapertura = solo diferencias (una unidad cancelada después
  de reabrir resta en el día del nuevo cierre); nada si no cambió. La suma es global por fuente: una unidad
  transferida se reconoce una sola vez, en la cuenta donde estaba al cerrarse.
- IVA por renglón con la regla de `finance.go`: provisional por renglón y residuo de centavos de cada cuenta y
  tasa repartido uno a uno entre los renglones de mayor neto; la suma por cuenta y tasa es exactamente el
  desglose de la cuenta.
- Datos históricos copiados: folio, modalidad, origen, mesa y área de la cuenta al cerrar, responsable, turno y
  caja de la operación que cerró (pago de la operación; sin pago, el turno activo de la caja de la terminal),
  actor, terminal, operación, producto (nombre, código, tipo), variante, categoría, lista de precios, motivo
  histórico, concepto, quién aplicó y autorizó.
- **Relleno inicial** en la 0017 para las cuentas hoy cerradas: fecha = primer `payments.account_closed` de la
  auditoría (o `closed_at`), turno y caja del pago de ese cierre (sin pago, sin turno), importes y nombres del
  estado actual. **Limitaciones**: el historial de reaperturas anterior a la 9A no se reconstruye (una cuenta
  reabierta y vuelta a cerrar queda con sus importes actuales en la fecha de su primer cierre); las cuentas
  reabiertas que hoy siguen abiertas no se rellenan (aparecen en «En curso» como reabiertas y se reconocerán al
  cerrarse); la mesa y el área son las actuales. El bloque está delimitado en la migración y la prueba de
  integración lo ejecuta sobre cuentas cerradas por la API y lo compara con lo que escribió Go (iguales por
  fuente).

### Reportes (43) y dashboard

Ventas (resumen por tasa de IVA, día, semana ISO, mes, hora, producto, variante, modalidad, salón por área y
mesa, para llevar por origen, lista de precios, turno, cuentas cerradas, evolución con el anterior, «En curso»
con pagadas sin entregar y reabiertas), categorías, productos (más y menos vendidos —incluye activos sin
ventas—, combos, componentes preparados sin importe, complementos, cancelados con su fecha y motivo, con
descuentos, tendencia), cocina (demanda por producto y por hora con la fecha de liberación, preparación por
estación), pagos (por método, caja, turno, usuario y día; detalle; mixtos y parciales derivados; devoluciones
en su día; correcciones; propinas; efectivo por moneda; importes pendientes) y descuentos (por día con % sobre
ventas, por motivo histórico, usuario y producto; detalle, generales, cortesías y autorizaciones de
supervisor). Lista completa con permisos en `docs9aB1/api_contracts.md`. Dashboard con secciones de ventas,
cobros y descuentos, cada una con su permiso, comparación con el periodo anterior equivalente y la serie diaria
de venta neta.

### Permisos (0017, Go y `app.permissions`, solo al administrador)

`reports.read`, `reports.sales.read`, `reports.payments.read`, `reports.discounts.read`, `reports.cash.read`,
`reports.inventory.read`, `reports.purchases.read`, `reports.employees.read`, `reports.export.pdf`,
`reports.export.excel`. Pantalla de inicio `reports` en `preferences.go`. El efectivo físico por moneda (y las
columnas «Recibido», «Cambio», «Dólares recibidos» y «Tipo de cambio» del detalle de cobros) exige además
`cash.expected.read`, igual que en Caja; los cobros de turnos en arqueo se excluyen de todo agregado para todos
(con un aviso que solo dice cuántos turnos).

### Índices de fecha (0017)

`sales_recognitions(recognized_at, sequence)`, `(account_id, sequence)`, `(unit_id)`, `(adjustment_id)`,
`(shift_id, recognized_at)`; `payments(created_at)`, `payment_refunds(created_at)`,
`account_adjustments(created_at)`, `cash_movements(created_at)`, `cash_operations(created_at)`,
`kitchen_unit_releases(released_at)`, `kitchen_tasks(released_at)`, `restaurant_item_units(cancelled_at)`
parcial, `inventory_supplier_returns(occurred_at)`, `payment_authorizations(created_at)` y
`audit_events(entity_id, occurred_at)` parcial para `payments.account_closed`. Medición: ver «Mediciones».

### Definiciones de métricas (§6), tal como quedaron

- Venta (reconocimiento): la venta cuenta cuando la cuenta se cierra liquidada, en el día del
  cierre (libro app.sales_recognitions, escrito por payments.closeIfSettled en la misma
  transacción). Una reliquidación tras una reapertura reconoce solo las diferencias en su día;
  una devolución nunca resta de la venta.
- Importe bruto: precio de las unidades antes de promociones (IVA incluido).
- Promociones: descuento de las promociones del catálogo (restaurant_item_units.discount).
- Descuentos: descuentos manuales vigentes, por partida (asignados a unidades) y generales de
  cuenta, sin cortesías. Cortesías: importe de las cortesías al 100 %.
- Cargos: cargos generales de cuenta (con su tasa de IVA).
- Venta neta = bruto − promociones − descuentos − cortesías + cargos (IVA incluido, sin propinas).
- IVA: incluido en la venta neta, por cuenta y tasa con la regla de payments/finance.go (base =
  total / (1 + tasa) redondeada; IVA = total − base); la suma del periodo es la suma de los
  desgloses por cuenta, nunca un recálculo sobre el agregado. Subtotal sin IVA = neta − IVA.
- Cuentas cerradas: cuentas con su primer reconocimiento en el periodo (una reliquidación no
  vuelve a contarlas). Ticket promedio = venta neta / cuentas cerradas.
- Venta antes de descuentos = bruto − promociones + cargos; % de descuentos sobre ventas =
  (descuentos + cortesías) / venta antes de descuentos.
- En curso: cuentas abiertas con total, pagado y saldo de payments.AccountStandingSQL; no son
  venta. «Pagada sin entregar»: para llevar liquidado sin entrega. «Reabierta»: ya se cerró antes.
- Cobro: pago registrado con un método, en su fecha (payments.created_at). Cobrado neto = pagos −
  devoluciones − reversos de corrección; el pago nuevo de una corrección es un pago. Propinas
  netas = propinas cobradas − propinas devueltas (nunca son venta).
- Devolución: payment_refunds kind=refund en su propia fecha; Corrección: kind=correction más el
  pago que la sustituye.
- Pago mixto: cuenta con pagos de más de un método en el periodo; parcial: más de un pago del
  mismo método (derivados; no existe marca).
- Efectivo por moneda: pesos y dólares físicos de cash_movements (recibido y entregado), con la
  equivalencia al tipo de cambio de cada cobro; nunca se suman pesos y dólares. Exige
  cash.expected.read (paridad con Caja) y excluye los turnos en arqueo.
- Turnos en arqueo (closing): sus cobros se excluyen de todo agregado para todos, con un aviso.
- Descuento por motivo: nombre histórico guardado (reason_name); sin motivo del catálogo,
  «Sin motivo del catálogo». Los descuentos se fechan con el reconocimiento de su venta.
- Combos: el combo es la unidad vendida; sus componentes solo se reportan como cantidades
  preparadas, sin importe.
- Demanda y cocina: fecha de liberación a cocina (kitchen_unit_releases), indicada en el reporte.

### Cómo agregar un reporte (para B2 y C1)

La misma guía está en el comentario del paquete (`backend/internal/reports/doc.go`).

1. Crear (o ampliar) un archivo del paquete con un init() que llame a register(Definition{...}):
   ID estable «grupo.nombre» (minúsculas, puntos y guiones bajos), Group de la lista Groups,
   Title, Description, Permissions (todas se exigen, además de reports.read: p. ej.
   reports.cash.read), DateBasis (qué fecha usa y con qué hora de inicio), Filters (solo los que
   aplican: filterRegister(), filterShift(), filterChoice(...)…), Columns tipadas (textColumn,
   moneyColumn, countColumn, dollarColumn, percentColumn, dateColumn…) y DefaultSort.
2. Las columnas con datos protegidos llevan requiring(columna, permiso): sin el permiso, la
   columna, sus valores, sus totales y el orden por ella no existen en la respuesta ni en las
   exportaciones. Una cifra resumida protegida lleva Figure.Permissions. Lo que permita deducir
   un dato protegido (p. ej. el esperado de caja) no debe calcularse para quien no lo puede ver.
3. Query recibe *Query: usar query.Args() (argumentos con nombre @from, @until, @tz, @cutoff y
   uno por filtro), inPeriod(columna), businessDate(columna) y localHour(columna) para que los
   límites coincidan con los de Go, conditions.when(...) para los filtros y query.collect(...)
   con alias SQL iguales a las claves de las columnas (columnas auxiliares no declaradas se
   descartan antes de responder). Devolver todos los renglones del conjunto filtrado: el motor
   ordena, pagina y suma en Go las columnas con Total «sum»; Totals completa las «custom».
4. Avisos y notas en Dataset.Warnings/Notes; nunca importes protegidos en ellos.
5. Indicadores del resumen ejecutivo: registerDashboard(DashboardSection{...}) con su permiso y
   Compute(periodo); cada indicador apunta a su reporte detallado (ReportID).
6. Exportaciones (C1): llamar a Run(ctx, runner, actor, id, parámetros) con el mismo actor y los
   mismos parámetros que la pantalla y usar Result.AllRows, Result.Columns, Result.Totals y
   Result.Meta (valores tipados: decimal.Decimal, int64, time.Time, string); tomar un lugar del
   semáforo con Runner.Acquire si la generación del archivo es larga.
7. Pruebas: reports_test.go valida el registro; las de integración (reports_integration_test.go)
   comprueban totales contra los registros, permisos por API directa y paginación. El puente
   nativo ya admite cualquier id del registro (reports_run) y los parámetros de su lista blanca:
   un filtro nuevo debe agregarse a allowed_query_keys en desktop/src-tauri/src/api.rs.

### Decisiones menores

1. Fuente `allocation` además de `unit` y `adjustment`: el descuento por partida se reconoce por unidad y por
   ajuste para conservar su motivo histórico y su producto sin repartir descuentos generales.
2. Las propinas no se reconocen como venta (se reportan desde cobros, en su fecha).
3. Turno de una venta cerrada sin pago (cortesía o entrega): el turno activo de la caja de la terminal que cerró
   (en el relleno: sin turno).
4. «Ventas por turno» excluye también los turnos en arqueo (como cobros); los reportes de ventas no ofrecen filtro
   de turno (solo `sales.by_shift`), para no aislar la venta de un turno en arqueo.
5. Efectivo por moneda y columnas de efectivo físico con `cash.expected.read` (paridad con Caja: sumadas por
   turno permitirían deducir el esperado).
6. Catálogo y opciones de filtros se resuelven en la transacción de autenticación (lecturas pequeñas sin locks);
   solo los reportes y el dashboard usan el semáforo.
7. Con filtro de producto o categoría, los descuentos generales y cargos de cuenta no se incluyen (no
   pertenecen a un producto); los reportes por producto muestran esos ajustes como cifras de conciliación.
8. «Menos vendidos» agrega con cero los productos activos sin ventas del periodo (solo sin filtros de
   modalidad, área, mesa, origen o lista).
9. El periodo «actual» (hoy, semana, mes, año) se compara con el anterior completo; `in_progress` lo indica.

### Mediciones

Medición opt-in `TestReportsVolumeMeasurement` (`ADEL_REPORTS_MEASURE=1`, Apple M1 Max, clúster temporal 55499,
base aislada): sobre los datos de la aceptación se generaron 200 000 renglones de reconocimiento repartidos en
un año (cada uno con su unidad) y 100 000 cobros (12.7 s). Tiempos de `reports.Run` completo (consulta en la
transacción de solo lectura, totales, orden y página) para el año actual:

| Reporte                                   | Renglones | Tiempo |
| ----------------------------------------- | --------- | ------ |
| `sales.by_day`                            | 365       | 245 ms |
| `sales.by_product`                        | 5         | 323 ms |
| `sales.by_month`                          | 11        | 281 ms |
| `payments.by_method`                      | 2         | 107 ms |
| `payments.by_day`                         | 315       | 27 ms  |
| `discounts.summary`                       | 365       | 127 ms |
| `sales.evolution` (con el año anterior)   | 365       | 116 ms |
| Dashboard (año actual contra el anterior) | —         | 236 ms |

`EXPLAIN (ANALYZE, BUFFERS)`: las consultas de un día usan `sales_recognitions_recognized_idx` (0.6 ms) y el
índice de pagos por fecha (0.2 ms); las de un año recorren la tabla completa porque el año es todo el
volumen (ventas por día 145 ms, por producto 241 ms, cobros por método 26 ms); la consulta de diferencias
que ejecuta cada cierre usa `sales_recognitions_account_idx` y `sales_recognitions_unit_idx` (0.14 ms con
200 000 renglones). La aceptación comprueba además que un cobro se registra mientras un reporte mantiene
abierta su transacción de solo lectura (no espera) y que ese reporte no ve el cobro (fotografía).
El cobro concurrente con un reporte de un año con volumen se mide en la fase D.

### Pruebas ejecutadas (copia aislada `repo9aB1`, clúster temporal 55499)

Salida resumida real (logs en `docs9aB1/*.log`):

- `go vet ./...` y `go vet -tags=integration ./...`: sin hallazgos.
- `go test -race -count=1 ./...`: todos `ok` (incluidos `internal/businessday`, `internal/reports`,
  `internal/payments` con `recognition_targets_test.go`, `internal/inventory` con sus pruebas de día operativo
  sin cambios y `internal/administration` con la pantalla `reports`).
- Integración `-race` (una base temporal por paquete): `internal/database` ok 5.8 s (incluye
  `TestStage9AReportsUpgradeBackfillsClosedAccounts` desde el esquema 14), `internal/administration` ok 58.7 s,
  `internal/catalog` ok 7.1 s, `internal/operations` ok 220.9 s (aceptación encadenada completa, con la prueba
  de parámetros de inventario adaptada), `internal/payments` ok 1.5 s, `internal/reports` ok 25.2 s
  (`TestReportsAcceptance` con 15 subpruebas y `TestReportRunnerIsReadOnlyAndBounded`). `./internal/reports` se
  agregó a la tupla de `scripts/database.py`.
- `npx tsc --noEmit`: sin errores. `npx vitest run src`: 51 archivos, 434 pruebas aprobadas (incluye
  `pages/businessDay.test.tsx`). `npm run build`: compilado.
- `cargo test --locked --offline`: 21 aprobadas, 1 ignorada (Keychain real), incluida
  `report_routes_are_closed_read_only_and_keyed`.
- Playwright completo (sin `SPEC`, puertos 8188/1428): Chromium 8/8 y WebKit 8/8 aprobados; tras el último
  cambio de la consulta de diferencias se repitió `restaurant.spec.ts`: Chromium 1/1 y WebKit 1/1.
- Medición de volumen: ver «Mediciones».

Lo que cubren las pruebas de reportes: día comercial con 00:00 y 05:00 y una venta de la 01:30 (unitaria y por
API cambiando la hora, sin reescribir registros), semana ISO 53 y cambio de año, zona horaria y cambios de
horario; venta contada al cerrar en su día y cuentas abiertas en «En curso»; devolución en su propio día sin
alterar la venta del día original y cuenta reabierta con saldo 0 («Reabierta»); reliquidación que reconoce
solo diferencias; cortesía al 100 % = venta 0 con su importe en cortesías; ventas por periodo, producto y
categoría; combos sin duplicar componentes; cuentas divididas y combinadas sin doble conteo; pagos parciales,
mixtos y correcciones; propinas separadas; dólares físicos y cambio por moneda con el tipo de cambio histórico;
descuentos con motivo histórico (renombrar el motivo no cambia el reporte) y «Sin motivo del catálogo»;
reportes sin ningún turno; permisos de cada reporte por API directa (y columnas protegidas sin
`cash.expected.read`); totales del detalle = resumidos y paginación sin alterar totales; IVA reconocido =
desglose de las cuentas; transacción de solo lectura (una escritura falla con 25006), sin locks consultivos,
`REPORTS_BUSY` y `REPORT_TIMEOUT`; relleno de la 0017 (base en esquema 14 y equivalencia con Go por fuente).

### Verificación conjunta de las fases A y B1

Sobre una copia exacta del commit `a2a472c` (fase A y la parte B1 de la fase B): Prettier, gofmt, `cargo fmt`,
`sync-metadata`, `go vet` (con y sin `integration`) y Go unitarias con `-race` aprobados; integración PostgreSQL
`database`, `administration`, `catalog`, `operations` (219.0 s), `payments`, `kitchen`, `inventory` y `reports` `ok`;
Vitest 51 archivos y 434 pruebas; `tsc` y build sin errores; `cargo test` 21 aprobadas y 1 ignorada; Playwright WebKit 8
de 8. En Chromium, `restaurant.spec.ts` agotó el límite de 60 s por prueba (dura unos 55 s en un equipo libre y el equipo
estaba cargado por tres agentes en paralelo) y las especificaciones siguientes fallaron en cascada porque la base ya
estaba configurada; el límite pasa a 120 s (`desktop/playwright.config.ts`) y Chromium se repite en la regresión final.
La verificación de la fase A sola no era significativa para la interfaz: dos pruebas de la fase A ya esperaban la «Hora
de inicio del día» de B1 (Go, integración y Rust sí aprobaron sobre `5a96f64`).

## Fase B · B2 · Reportes de caja, inventarios, costos y márgenes, compras, proveedores y empleados

Sobre la infraestructura de la fase B1 (registro cerrado, resultado único, transacción `REPEATABLE READ READ ONLY`
con semáforo propio, día comercial, permisos `reports.*`). **Sin migración nueva**: no hizo falta la 0018 (ver
«Mediciones»). 45 reportes nuevos y 6 secciones del resumen ejecutivo, en `backend/internal/reports`
(`cash.go`, `cash_day.go`, `cash_details.go`, `inventory.go`, `inventory_movements.go`,
`inventory_consumption.go`, `costs.go`, `purchases.go`, `employees.go`, `dashboard_operational.go`,
`filters_operational.go`); aceptación propia en `backend/internal/reports/operational` (paquete solo de pruebas,
agregado a la tupla de `scripts/database.py`). Lista completa con columnas y permisos en
`docs9aB2/api_contracts.md`.

### Caja (`reports.cash.read`; 7 reportes y 2 secciones del dashboard)

- **Por turno** (`cash.shifts`, `cash.by_register`, `cash.sales_collections_by_shift`): turnos cuya fecha
  comercial (fecha de apertura en la zona, como el historial de Caja) cae en el periodo, con fondos, cobros
  (ventas cobradas, propinas, devoluciones, neto y neto en efectivo, como el corte), ingresos, retiros, gastos y
  correcciones por moneda sin regularizaciones, cortes parciales, lo contado al cerrar y, con
  `cash.expected.read`, esperado, efectivo físico por moneda, diferencias, sobrantes y faltantes por moneda, su
  equivalencia con el tipo de cambio histórico del turno y el resultado. Ventas y cobros por turno: venta =
  reconocimientos cuyo cierre ocurrió en el turno (columnas con `reports.sales.read`), cobro = pagos del turno;
  nunca se suman entre sí.
- **Por día** (`cash.by_day`): cada registro en el día comercial en que ocurrió (hora de inicio del día):
  aperturas y fondos, cobros, operaciones de efectivo, cortes, cierres y lo contado; con permiso, efectivo físico,
  flujo neto de efectivo y esperado y diferencias de los turnos que se cerraron ese día. Un turno permanente
  reparte sus movimientos entre los días; en la vista por turno aparece en la fecha de su apertura con todo.
- **Detalle**: `cash.operations` (entradas y salidas por moneda; las regularizaciones aparte, con signo, porque no
  cuentan en el esperado), `cash.cuts` (lo guardado en cada corte desde el anterior; el bloque `sensitive` solo
  con permiso) y `cash.by_user` (aperturas, cierres, cortes, operaciones registradas y autorizaciones).
- **Esperado**: el de un turno abierto es `payments.ExpectedCash` (función pura exportada que aplica
  `cashFlows.expectation`, el cálculo único) sobre componentes sumados con las mismas expresiones SQL que
  `readCashFlows` (`payments.ExpectedPaymentsMXNSQL` / `USDSQL`, que ahora usa también Caja); el de un turno
  cerrado, el que guardó su cierre. La prueba lo compara con `GET payments/shifts/{id}/summary` (pesos y dólares)
  antes y después de más actividad. **No se replica `readAdjustmentTotals`**: los descuentos no se atribuyen a
  turnos ni cajas en ningún reporte de caja (están en Descuentos con la fecha del reconocimiento).
- **Paridad exacta con Caja**: sin `cash.expected.read` no se calculan ni se envían el esperado, el efectivo
  físico por moneda, las diferencias, sobrantes, faltantes, su equivalencia ni el resultado (el SQL ni siquiera
  los selecciona); sí lo que Caja muestra (fondo, cobros por método, operaciones por moneda, lo contado). **Turnos
  en arqueo**: excluidos de todo agregado de caja, cobros y empleados para todos (también en la vista por día y en
  el dashboard), con el aviso `SHIFT_CLOSING`; en la lista de turnos aparecen sin ningún importe.
- **Alcance de Caja** (§12 «No permitir que un reporte consolidado revele datos que el usuario no podría
  consultar en Caja»): solo se incluyen los turnos que el usuario puede consultar en Caja (`cash.shifts.read`;
  otra caja con `cash.shifts.read_all`; cerrados con `cash.history.read` o si él los cerró), con el aviso
  `CASH_SCOPE` cuando el alcance está limitado.

### Inventarios (`reports.inventory.read` y los permisos de la etapa 8; 18 reportes y 2 secciones)

- Existencias actuales (total por insumo con niveles y situación), por almacén (niveles del almacén o del
  insumo), negativos y bajo mínimo (`inventory.stock.read`); kardex con existencia después de cada movimiento y
  cifras de existencia inicial, entradas, salidas y final al filtrar un insumo; entradas y salidas por insumo y
  tipo; transferencias; ajustes de discrepancia con motivo, estado y autorización; mermas (merma operativa con su
  motivo y cancelaciones «Preparado / merma» con el consumo conservado de la unidad, más las cuentas de
  cancelaciones «No preparado» y por clasificar; una devolución de dinero nunca es merma) y consumos por
  ingrediente, por día, por producto e ingrediente y asociado a cocina por almacén y modalidad
  (`inventory.movements.read`); conteos y diferencias de conteo (`inventory.stock.read`, porque el teórico revela
  existencias); valuación (`inventory.valuation.read`; el valor por insumo exige además `inventory.costs.read` y el
  valor total solo la valuación, como en la etapa 8) y costos promedio (`inventory.costs.read`).
- Sin `inventory.costs.read` no viajan costos unitarios, valores, bases de costo, «costo pendiente» ni los ajustes
  de costo del kardex; la valuación después de cada movimiento exige costos y valuación.
- Consumo: el neto de `consumption` + `consumption_reversal` de las unidades, fechado con la liberación canónica;
  las unidades se cuentan distintas (una unidad enviada a varias estaciones consume y cuenta una vez; el resumen
  da el total de unidades distintas aunque aparezcan en dos almacenes). Aviso `CONSUMPTION_PENDING` con
  solicitudes pendientes o con error en el periodo.

### Costos y márgenes (`reports.sales.read` + `inventory.costs.read`; 9 reportes y una sección)

- Margen estimado por producto, categoría y día (`costs.margin_by_*`) con el costo real de los consumos de las
  unidades reconocidas en el periodo (renglón × costo neto de su unidad; una unidad retirada en una reliquidación
  resta su costo), estado del costo por unidad: «Sin costo» (sin consumo registrado, sin receta, solicitud
  pendiente o con error), «Estimado» (algún consumo con costo pendiente o valuado con el último costo válido o el
  de referencia, o con configuración incompleta) y firme; margen = venta sin IVA de las unidades con costo − su
  costo; % de la venta (sin IVA) con costo firme; cifras del periodo con los ajustes de costo (concepto aparte) y
  los descuentos y cargos generales de cuenta (no pertenecen a un producto). Nota «no es utilidad contable».
- `costs.cost_status`: unidades sin costo o estimadas por producto con **qué dato falta**.
- Costo teórico por producto (contra el precio vigente sin IVA, margen teórico) y por receta con el motor de la
  etapa 8 (`inventory.TheoreticalCosts`, exportado de `recipe_costs.go`, solo SELECT); consumo valuado por
  categoría de insumo; costo de mermas; ajustes de costo del periodo. La valuación de existencias está en
  `inventory.valuation`.

### Compras y proveedores (`reports.purchases.read`; 8 reportes y una sección)

- **Compras recibidas por periodo** (`purchases.received`, obligatorio): fecha, folio de la recepción y de la
  orden, modalidad (orden, directa, **mostrador** por `inventory_suppliers.kind='counter'` con el comercio),
  proveedor, almacén, insumo, cantidad y unidad capturada y base, precio unitario sin IVA, descuento, importe sin
  IVA, tasa e IVA, importe y quién recibió; filtros proveedor, insumo, categoría de insumo, almacén, modalidad y
  estado de la orden; cifras de recepciones, total, **pagado desde caja** (ya incluido, nunca otra compra) y
  devoluciones del periodo (aparte). Valor de inventario y costo por unidad base solo con `inventory.costs.read`.
- Compras ordenadas (con lo recibido y lo pendiente por renglón), pendiente de recepción (situación actual, con
  importe proporcional de referencia), compras por proveedor (recibido, ordenado, devoluciones y pagado desde
  caja, sin sumarlos entre sí) y por insumo, devoluciones a proveedor a su valor asociado, compras pagadas desde
  caja (los datos del gasto exigen los permisos de consulta de Caja) y proveedores (contacto, compras del periodo,
  última recepción y órdenes abiertas).

### Empleados (`reports.employees.read`; 3 reportes)

- Solo relaciones registradas: confirmó (las unidades que nacieron con su confirmación), canceló, responsable de
  la cuenta al cerrar (la venta se atribuye solo a él: nunca dos veces), cobró (sin turnos en arqueo), aplicó
  descuentos y cortesías, autorizó como supervisor, devolvió y registró movimientos de caja. Sin «capturó» (no
  se conserva) ni estadísticas de productividad. Ningún reporte de empleados incluye costos ni esperado.

### Permisos y puente

- Sin permisos nuevos: los de la 0017 y los de origen (`cash.expected.read`, `cash.shifts.read*`,
  `cash.history.read`, `cash.movements.read`, `inventory.stock.read`, `inventory.movements.read`,
  `inventory.costs.read`, `inventory.valuation.read`).
- Fuentes de filtros nuevas: `warehouses`, `inventory_items`, `inventory_categories`, `suppliers`. Claves nuevas
  en la lista blanca `reports_*` de `api.rs`: `warehouse_id`, `item_id`, `supplier_id`, `modality`, `currency`.

### Decisiones menores

1. Alcance de Caja en los reportes de caja (otras cajas con `cash.shifts.read_all`, cerrados con
   `cash.history.read` o el propio cierre), por §12 «Exportación».
2. Vista por turno = fecha comercial del turno en el periodo (como el historial de Caja) con los importes de todo
   el turno; la vista por día no depende de los turnos.
3. Lo contado al cerrar se muestra sin `cash.expected.read` (Caja también lo muestra); sin la diferencia no permite
   reconstruir el esperado.
4. Venta por turno en Caja con columnas de venta exigiendo `reports.sales.read`; el cobro, solo
   `reports.cash.read` (Caja muestra los totales por método).
5. Inventarios: kardex, entradas, salidas, transferencias, ajustes, mermas y consumos exigen
   `inventory.movements.read` (como la consulta de movimientos de la etapa 8); existencias, negativos, bajo mínimo
   y conteos `inventory.stock.read`.
6. Costos unitarios por unidad base como cantidad decimal con 4 decimales (como la etapa 8), no como importe de
   dos decimales.
7. Configuración incompleta del consumo (diagnósticos del trabajador) ⇒ «Estimado»; procesada sin movimientos ⇒
   «Sin costo» (sin receta).
8. Costo de una unidad reconocida = renglón × costo neto actual de la unidad (consumos − reversiones): una unidad
   cancelada después de vender y clasificada «No preparado» queda en cero; «Preparado / merma» conserva su costo,
   que pasa a mermas cuando la reliquidación la retira.
9. Compras ordenadas sin borradores; las canceladas solo con su filtro. Importe pendiente proporcional al renglón
   de la orden, rotulado como referencia.
10. Unidades confirmadas = las creadas en la misma transacción que la confirmación (`created_at` igual); las
    agregadas después por una modificación no cuentan para quien confirmó.
11. El paquete de aceptación vive aparte (`internal/reports/operational`) para tener su propia base: los
    escenarios de B2 (turnos de varios días, arqueos, inventario activado) no alteran los totales exactos de la
    aceptación de B1. Las fechas de días anteriores se simulan actualizando `created_at` de turnos, pagos,
    movimientos y operaciones de caja en la base temporal (no tienen disparadores de solo inserción).

### Limitaciones

- Valuación por almacén no existe (un costo por insumo, como la etapa 8): las existencias por almacén solo dan
  cantidades.
- El costo teórico es el de hoy (sin historial) y no incluye complementos elegidos ni componentes de combo
  (limitación de la etapa 8).
- El esperado de un turno cerrado antes de la etapa 7 (sin cierre registrado) no existe.
- Las vistas por día y por turno usan calendarios distintos (hora de inicio vs fecha de apertura): se rotulan.
- El kardex de un año completo con el volumen medido excede el máximo de 100 000 renglones en memoria
  (`REPORT_TOO_LARGE`): se consulta por mes o con filtros.

### Mediciones

Medición opt-in `TestOperationalVolume` (`ADEL_REPORTS_MEASURE=1`, después de la aceptación en la misma base;
Apple M1 Max, clúster temporal 55499, `-race`): sobre los datos de la aceptación se generaron en 26.8 s 120 000
unidades vendidas y reconocidas en un año (100 000 con su solicitud de consumo procesada y su estado de consumo,
20 000 «Sin costo»), 200 000 movimientos de consumo y 60 000 cobros con su movimiento de caja. Tiempos de
`reports.Run` completo (consulta en la transacción de solo lectura, totales, orden y página), 12 ejecuciones
cada uno (p95 = la más lenta de las 12):

| Reporte (365 días con el volumen)                                   | Renglones | Media    | p95      |
| ------------------------------------------------------------------- | --------- | -------- | -------- |
| `cash.shifts`                                                       | 2         | 92 ms    | 97 ms    |
| `cash.by_day`                                                       | 364       | 77 ms    | 83 ms    |
| `cash.sales_collections_by_shift`                                   | 2         | 88 ms    | 92 ms    |
| `inventory.consumption_by_item`                                     | 3         | 489 ms   | 554 ms   |
| `inventory.consumption_by_day`                                      | 364       | 252 ms   | 278 ms   |
| `inventory.consumption_by_product`                                  | 3         | 778 ms   | 1 065 ms |
| `inventory.consumption_by_kitchen`                                  | 1         | 526 ms   | 575 ms   |
| `costs.margin_by_product`                                           | 3         | 815 ms   | 1 002 ms |
| `costs.margin_by_category`                                          | 1         | 838 ms   | 911 ms   |
| `costs.margin_by_period`                                            | 364       | 812 ms   | 916 ms   |
| `costs.cost_status`                                                 | 6         | 777 ms   | 870 ms   |
| `purchases.received`                                                | 3         | 4 ms     | 18 ms    |
| `purchases.by_supplier`                                             | 2         | 1 ms     | 3 ms     |
| `employees.activity`                                                | 2         | 86 ms    | 96 ms    |
| `employees.responsible_sales`                                       | 3         | 278 ms   | 285 ms   |
| `inventory.kardex` (un mes, un insumo)                              | 3 020     | 119 ms   | 138 ms   |
| `costs.margin_by_product` (hoy)                                     | 3         | 13 ms    | 16 ms    |
| `inventory.consumption_by_item` (hoy)                               | 3         | 3 ms     | 3 ms     |
| `cash.by_day` (hoy)                                                 | 1         | 2 ms     | 7 ms     |
| Dashboard (365 días contra los 365 anteriores, todas las secciones) | —         | 1 049 ms | 1 173 ms |

**Cobro concurrente**: con el reporte de margen de un año en curso (933 ms) y su transacción de solo lectura aún
abierta, la consulta del saldo y el pago por la API tardaron 95 ms (el mismo cobro sin reporte: 54 ms); la prueba
exige que el cobro termine con la transacción del reporte abierta y en menos de 3 s. `EXPLAIN (ANALYZE, BUFFERS)`:
con un año que es todo el volumen, la base del margen (renglones del libro y sus consumos) es un hash join con
recorrido completo de ambas tablas (84 ms) y el consumo por día un recorrido de los 200 000 movimientos con
ordenamiento (160 ms); las consultas de un día usan `sales_recognitions_recognized_idx`,
`inventory_movements_occurred_idx`, `inventory_movements_unit_idx` e `inventory_consumption_requests_unit_idx`
(margen de un día: 13 ms). Ningún reporte se acerca al límite de 30 s, por lo que **no se agregó la 0018**.
Medición de un solo equipo, sin varias terminales.

### Pruebas ejecutadas (copia aislada `repo9aB2`, clúster temporal 55499)

Salida resumida real (logs en `docs9aB2/*.log`):

- `gofmt -l` sin archivos; `go vet ./...` y `go vet -tags=integration ./...`: sin hallazgos.
- `go test -race -count=1 ./...`: todos `ok` (incluidos `internal/payments` con
  `TestExpectedCashIsTheShiftExpectation`, `internal/inventory` e `internal/reports` con
  `TestReportRegistryIsClosedAndConsistent` sobre los 45 reportes nuevos).
- Integración `-race` (una base temporal por paquete): `internal/database` ok 5.3 s, `internal/administration`
  ok 55.6 s, `internal/catalog` ok 7.4 s, `internal/payments` ok 1.3 s, `internal/operations` ok 221.7 s
  (aceptación encadenada), `internal/reports` ok 91.8 s (aceptación de B1 —que consulta todos los reportes
  registrados sin turno y los permisos de cada uno por API directa— y las exportaciones de C1 en el árbol, que
  revisan 25 reportes con columnas protegidas en PDF y Excel), `internal/reports/operational` ok 145.9 s con la
  medición (`TestOperationalReports` con 14 subpruebas y `TestOperationalVolume`).
- `npx tsc --noEmit`: sin errores. `rustfmt --check src/api.rs`: sin cambios. `cargo test --locked --offline`:
  29 aprobadas, 1 ignorada (Keychain real).
- No se ejecutaron Vitest, `npm run build` ni Playwright (sin cambios de interfaz en esta parte).

Lo que cubre `TestOperationalReports`: turno permanente de tres días comerciales (vista por turno en la fecha de
apertura y vista por día con aperturas, cobros, ingresos, retiros, gastos y corte parcial en su día); esperado del
reporte igual al de Caja en pesos y dólares (fondo distintivo, cobro con cambio, cobro en dólares, operaciones) y
otra vez antes del cierre; ventas y cobros por turno iguales a los cobros por turno; turno en arqueo sin importes
en la lista y excluido del historial por caja, la vista por día, los cobros, el dashboard y empleados (con aviso);
cierre con sobrante (contado 230, esperado 225) y cortes con su esperado; esperado, efectivo físico, diferencias,
resultado y sus valores (`482.34`, `225.00`, `-2.50`, `15.50`) ausentes de todo el JSON de los reportes de caja,
del catálogo y del dashboard para quien no tiene `cash.expected.read`, que sí recibe lo contado y el fondo;
alcance de Caja (`CASH_SCOPE`) y reportes denegados por API directa sin su permiso; diferencias por moneda con el
tipo de cambio histórico (−2.50 MXN, +1 USD, equivalente 15.50 a 18.00); márgenes con unidades «Sin costo»
(vendidas antes de activar el inventario, sin receta y con consumo pendiente, con `CONSUMPTION_PENDING`),
«Estimado» (lata con existencia en cero) y firme, margen = venta sin IVA con costo − costo, y paso a firme al
procesar el consumo; costo teórico igual al de `GET inventory/recipes/{id}`; existencias negativas y bajo mínimo,
existencias por almacén tras transferir y contar, kardex con existencia inicial y final, mermas clasificadas
(rotura y «Preparado / merma»), conteo con diferencia −10, consumo sin contar una unidad dos veces; sin
`inventory.costs.read` ningún costo, valor ni ajuste de costo, sin `inventory.valuation.read` la valuación
denegada y con ella solo el valor total; compras recibidas (orden parcial, directa y mostrador) con el gasto
vinculado sin duplicar (total 321.44 = recepciones; pagado desde caja 97.44), ordenado contra recibido y
pendiente, por proveedor, devoluciones aparte y ajuste de costo de la recepción que cubrió el déficit; empleados:
venta por responsable = venta del periodo, cobrado por usuario = cobros por usuario, confirmaciones y unidades
confirmadas; dashboard con sus seis secciones y margen nulo sin datos suficientes, sin margen ni costos para quien
no tiene `inventory.costs.read`; y en **todos** los reportes registrados, la suma del detalle = el total y el total
de una página = el del conjunto.

## Fase C · C1 · Exportación PDF y Excel en Go y operación cerrada del puente

Sin migraciones. Dependencias nuevas (descarga única con red, después sin red): excelize 2.11.0, go-pdf/fpdf 0.12.0 y
x/image 0.47.0 (fuentes Go) en Go; tauri-plugin-dialog 2.8.3 en Rust (detalle en DEPENDENCIES).

### Exportación en Go (`backend/internal/reports/export*.go`)

- Ruta `GET reports/export/{id}?format=pdf|xlsx&…` con los mismos parámetros que `reports/run` salvo la paginación,
  más `paper=letter|a4` para el PDF. Permisos del reporte **y** `reports.export.pdf` o `reports.export.excel`, en la
  transacción de autenticación (403 auditado como `access.denied`).
- **Misma consulta y mismo resultado que la pantalla**: `Prepare` + `Execute` del motor de B1 en la transacción de solo
  lectura (el nombre del establecimiento se lee en la misma fotografía); el archivo usa `Result.Columns`,
  `Result.AllRows`, `Result.Totals`, `Result.Summary` y `Result.Meta`. Nada se recalcula: las columnas y cifras
  protegidas no existen en el `Result` de quien no tiene su permiso y, por tanto, tampoco en el archivo.
- La exportación toma un lugar del semáforo de reportes y lo conserva hasta terminar el archivo (con 2 lugares, nunca
  hay más de dos consultas o archivos a la vez). Límites: 50 000 renglones (422 `EXPORT_TOO_LARGE` con el número de
  renglones, sin generar) y 32 MiB (422 `EXPORT_FILE_TOO_LARGE`); tiempo propio de 110 s; la generación se interrumpe
  si la petición se cancela.
- Respuesta binaria con `Content-Type` exacto, `Content-Disposition: attachment; filename=…` (reporte y periodo en
  ASCII, p. ej. `ventas-por-dia_2026-11-01_2026-11-30.pdf`) y `Content-Length`. El archivo no se guarda en el
  servidor: no hay identificador de descarga.
- Auditoría `report.exported` en una transacción corta aparte, después de generar el archivo y antes de enviarlo
  (reporte, formato, papel, orientación, periodo, comparación, filtros con sus valores tal como se enviaron, orden,
  renglones y bytes; sin nombres de personas, importes ni contenido).
- **PDF** (fpdf + Go Regular/Bold UTF-8): primera página con establecimiento, reporte, descripción, periodo,
  comparación, filtros, fecha usada, hora de inicio del día y zona, orden, renglones y fecha de generación; cifras
  resumidas y avisos; tabla con encabezados repetidos en cada página, renglones alternados, totales de Go y notas;
  encabezado breve en las páginas siguientes y pie «Página n de m». Carta (o A4) vertical si la tabla cabe con 8 pt; si
  no, horizontal con la letra necesaria (8 a 5.5 pt). Importes, cantidades, fechas y códigos nunca se parten (ancho
  del valor más largo); el texto se ajusta en varias líneas (ancho preferido: percentil 90, hasta 70 mm). Importes con
  `moneytext`, cada moneda en su columna.
- **XLSX** (excelize por flujo): hoja «Datos» con encabezado distinguible y congelado, anchos según el contenido,
  números exactos con formato por columna (pesos y dólares separados), porcentajes como fracción, fechas comerciales y
  horas del restaurante como fechas, códigos y textos con formato «@», totales de Go como números; hoja «Filtros» con
  los metadatos. Sin fórmulas, hipervínculos, nombres definidos, vínculos externos, macros ni imágenes; un texto que
  empieza por `= + - @`, tabulador, retorno o salto (o sus formas de ancho completo) lleva un apóstrofo inicial.
- `administration`: `reports.go` conserva quién pidió el reporte para auditar la exportación y envía el archivo
  binario; `http.go` da 110 s a `reports/export/*` y **amplía el plazo de escritura** de las rutas largas.

### Puente (`desktop/src-tauri/src/export.rs`)

Operación cerrada `export_report(report, format, query)`: lista cerrada de 92 reportes (igual al registro de Go,
comprobado por una prueba Go que lee `export.rs`), parámetros de la lista blanca `reports_*` de `api.rs` sin
paginación ni búsqueda (más `paper` en PDF), credenciales e invalidación como `api_request`, 120 s y 32 MiB, una
exportación a la vez, validación de `Content-Type` y firma antes del diálogo, nombre saneado con extensión forzada,
diálogo nativo `tauri-plugin-dialog` solo desde Rust (sin `dialog:*` en `capabilities`), escritura atómica (temporal
en la misma carpeta y renombrado). React recibe solo `{result: saved|cancelled, file_name, size}`. Registro en
`build.rs`, `permissions/diagnostics.toml` y `main.rs`; en `api.rs` solo se hicieron `pub(crate)`
`allowed_query_keys`, `sanitize_response` e `invalidates_credentials`. Servicio `desktop/src/services/reportExport.ts`
(C2 lo usa en `components/reports/ExportButtons.tsx`).

### Playwright

`native-driver.ts` simula `export_report` (misma lista cerrada leída de `export.rs`, misma lista blanca leída de
`api.rs`, `fetch` desde Node con las credenciales de Node, validación de tipo, firma y nombre como Rust, cola
`nextSaveDialog('save' | 'cancel')`; los bytes quedan en `exports` en Node y los rechazos llegan como texto, como con
Tauri). Especificación nueva `e2e/workflow-reports-export-9a.spec.ts` (después de las existentes; solo lee): exportar
PDF guardado, Excel cancelado y Excel guardado desde los botones de C2, y rechazos de la operación cerrada.

### Decisiones menores

1. El archivo incluye **todas** las columnas que el usuario puede ver (también las «opcionales» que la pantalla oculta
   de inicio), en el orden del registro: el PDF y el Excel tienen las mismas columnas.
2. `page` y `page_size` se rechazan en la exportación (400) en lugar de ignorarse; `paper` solo en PDF.
3. Orientación automática (sin parámetro): vertical si cabe con 8 pt; A4 solo si se pide.
4. Porcentajes en Excel como fracción exacta (12.50 → 0.125 con formato `0.00%`).
5. Un importe con más de 15 dígitos significativos se escribe como texto exacto (Excel lo truncaría) y la hoja
   «Filtros» lo indica.
6. Los nombres de archivo son ASCII (sin acentos) para que el nombre sugerido sea idéntico en Go, Rust y macOS.
7. Si Rust tiene que agregar la extensión al nombre elegido y ese archivo existe, no lo sustituye (el panel no lo
   confirmó): pide volver a exportar con otro nombre.
8. La auditoría guarda los valores de los filtros tal como se enviaron (identificadores o claves), no sus nombres.
9. Un texto muy largo de una celda del PDF se recorta con «…» solo si excede una página entera (el Excel lo conserva
   completo).

### Hallazgo corregido (fuera de C1, mínimo)

El servidor HTTP tiene `WriteTimeout` de 15 s (`cmd/ad-el-pos-server/main.go`): un reporte de B1 que tardara más de
15 s (su plazo es 40 s) se habría cortado al escribir la respuesta. `http.go` ahora amplía el plazo de escritura de
`reports/*` a 45 s y de `reports/export/*` a 115 s con `http.ResponseController` (como ya hace SSE).

### Limitaciones

- El diálogo nativo y la escritura en disco desde la ventana Tauri real no se ejecutaron (no hay ventana en este
  entorno): la escritura atómica se probó como función pura con un destino simulado y el flujo completo con la
  simulación de Playwright.
- Las fuentes Go no tienen ideogramas CJK ni emojis (en el PDF se ven como un recuadro; el Excel los conserva).
- Los filtros nuevos de B2 se exportan cuando se agreguen a la lista blanca `reports_*` de `api.rs` (la exportación
  reutiliza esa lista); un reporte nuevo debe agregarse a `EXPORTABLE_REPORTS` (la prueba Go lo exige).
- La prueba de integración de exportaciones borra y vuelve a crear el esquema de su base temporal al empezar y al
  terminar (como las pruebas de `internal/database`), para no alterar las demás pruebas del paquete.

## Fase C · C2 · Interfaz de Reportes: navegación por catálogo, resumen ejecutivo, filtro de periodo, tablas, gráficos SVG propios y exportación

Sin migraciones ni cambios de API, de Go, de Rust ni del puente. La interfaz es **genérica sobre el catálogo de Go**
(`reports/catalog`): grupos, reportes, filtros, columnas tipadas, periodo y comparación salen de la declaración de
cada reporte, de modo que los reportes de la parte B2 (caja, inventarios, compras, proveedores, costos y márgenes,
empleados) aparecieron sin escribir pantallas nuevas. React no suma, resta, redondea ni ordena importes: muestra
los valores tipados de Go con `services/money.ts` y sus reglas de texto.

### Pantalla «Reportes»

- `reports` en `PreferencesPage.tsx` (`reports.read`, admitida como pantalla de inicio) y en `App.tsx`
  (`ReportsPage` con `can`, la zona del establecimiento y los permisos efectivos; si cambian, el catálogo se vuelve
  a pedir). El menú principal global no cambió (solo una entrada más, después de Inventarios).
- Navegación en dos niveles: grupos del catálogo en una pestaña cada uno («Resumen ejecutivo» primero si
  `dashboard: true`), en columnas iguales de una o dos filas completas (hasta 7 por fila; con 13 grupos, 7 y 6).
  Dentro del grupo, los reportes se listan a la izquierda en pantallas de 1440 px o más; por debajo, un selector
  «Reporte» sobre el contenido deja todo el ancho a la tabla. Cada grupo recuerda su último reporte. Un grupo de un
  solo reporte no lleva lista. Sin reportes visibles se explica el permiso que falta.
- Periodo y filtros son de la pantalla (no de cada reporte): se conservan al cambiar de reporte y cada reporte envía
  solo los que declara. Los valores se guardan por **parámetro y fuente** (`category_id:categories` ≠
  `category_id:inventory_categories`), para que un identificador de categoría del catálogo nunca llegue a un
  reporte de insumos con el mismo nombre de parámetro.

### Filtro de periodo (`components/reports/PeriodFilter.tsx`, `period.ts`)

- Dos controles segmentados: rápidos (Hoy, Ayer, Semana actual, Mes actual, Año actual: se aplican con un toque) y
  con dato (Día, Semana del año, Mes, Año, Personalizado). Solo se ofrecen los periodos que Go lista en el catálogo.
- Los campos se completan con el **último periodo que resolvió Go** (su día comercial, su año y semana ISO), no con
  la fecha local de la terminal, y se aplican con «Aplicar periodo» o Intro: teclear nunca consulta. Las flechas de
  semana anterior y siguiente aplican al momento (una consulta por toque) y cruzan el cambio de año ISO.
- Semana del año ISO 8601 con su rango («Semana 53 de 2026: del 28 de diciembre de 2026 al 3 de enero de 2027, de
  lunes a domingo»): el rango se calcula en el cliente solo como descripción del calendario antes de consultar; el
  periodo que vale es el que devuelve Go (`meta.period.label`), que se muestra como «Periodo aplicado» con «En
  curso», junto con «Comparado con» y «Inicio del día · zona».
- Rango personalizado con dos `OptionalDateField` (dd/mm/aaaa, días incluidos). Comparación con el periodo anterior
  como interruptor (`ToggleButton`), solo en reportes con `comparison: true` y en el resumen.
- Parámetros enviados: `period` + `date` | `week=AAAA-Wss` | `month=AAAA-MM` | `year` | `from`/`until`, y
  `compare=previous|none` cuando aplica.

### Reporte (`pages/reports/ReportView.tsx`, `ReportFilterFields.tsx`, `components/reports/ReportTable.tsx`)

- Un solo panel «Filtros del reporte» con el periodo y los filtros que declara el reporte: `choice` (con su valor
  por omisión si lo trae), `id` con opciones de `reports/filters/{fuente}` (una consulta por fuente mientras la
  pantalla está abierta), productos e insumos con búsqueda en Go al pulsar «Buscar» (hasta 100), texto libre con
  «Aplicar». Cada elección consulta una vez y vuelve a la página 1.
- Metadatos de Go: fecha usada, filtros aplicados (texto legible de Go) y fecha de generación; avisos de Go
  (`meta.warnings`, p. ej. «Turnos en arqueo excluidos», datos de consumo pendientes) como avisos de estado; notas
  al pie; cifras resumidas (`summary`) en tarjetas.
- Tabla: columnas tal como llegan (las protegidas no llegan y no se dibujan); las `optional` ocultas por omisión con
  «Mostrar columnas adicionales (n)». Cifras a la derecha con `tabular-nums`; códigos y fechas sin partir; textos
  «Sin costo», «Estimado», «Firme», «En arqueo»… como etiquetas con texto. `<tfoot>` con los **totales de Go** de
  todo el conjunto filtrado (no de la página; vacíos en columnas sin total). Orden en Go con botones en el
  encabezado y `aria-sort`; paginación de Go (50 renglones) con el `Pagination` existente. Estados: «Cargando
  reporte…», mensaje vacío con el periodo aplicado, error de Go con «Reintentar»; al volver a consultar se conserva
  el resultado anterior atenuado (sin parpadeo).
- «Actualizar» vuelve a consultar. Los reportes, el catálogo y el resumen **no** escuchan los eventos SSE
  (`useReportData`): se consultan al abrir, al cambiar periodo, filtros, orden o página, y a pedido.

### Gráficos SVG propios (`components/reports/`, sin dependencias)

`TrendChart` (líneas), `ComparisonBars` (barras horizontales, una serie o actual frente a anterior),
`SegmentedBar` (distribución), `Sparkline` (tarjetas) y `ChartFrame` (título, leyenda con dos o más series y
«Ver como tabla» con la tabla alternativa accesible). Paleta validada (claro `#2a78d6 #eb6834 #1baf7a #eda100
#e87ba4`, oscuro `#3987e5 #d95926 #199e70 #c98500 #d55181`) como tokens `--chart-1..5`, más `--chart-previous`,
`--chart-grid` y `--chart-axis`, en ambos temas; validador de la skill dataviz: todas las comprobaciones aprobadas en
ambos temas, con contraste < 3:1 en tres tonos del tema claro, compensado con rótulos visibles y la tabla
alternativa (`palette.log`). Líneas de 2 px, punto final con anillo de la superficie, rejilla fina y continua,
barras ≤ 20 px con extremo de datos redondeado y base recta en cero, separación de 2 px en la barra segmentada,
cruceta y recuadro con todas las series y navegación con flechas en la tendencia; el texto nunca usa el color de la
serie. Los números de la geometría salen de las cadenas de Go solo para posicionar marcas; todos los rótulos son
los textos de Go formateados.

Elección del gráfico por la forma del resultado (`pages/reports/ReportChart.tsx`), para que los reportes nuevos lo
tengan sin pantalla propia: secuencia (día, semana, mes, hora o periodo de la evolución) → tendencia, con la serie
`previous_*` en gris si el reporte la trae (Evolución de ventas); hasta 5 categorías no negativas con todo el
conjunto en la página → barra segmentada; más categorías → barras con los primeros 10 renglones en el orden de la
tabla; listados de detalle (fecha y hora o más de dos columnas de texto) → sin gráfico.

### Resumen ejecutivo (`pages/reports/DashboardView.tsx`, `components/reports/MetricCard.tsx`)

- Secciones e indicadores tal como los envía Go (cada uno con su permiso; lo que no llega no se muestra). Cada
  tarjeta: valor, variación con flecha y texto («+13.57 % frente al periodo anterior»; con anterior en cero, la
  variación absoluta), «Anterior: …», la explicación de Go y minigráfico si hay serie del indicador. Tono por
  polaridad: subir es favorable en ventas y cobros, desfavorable en descuentos, cortesías, devoluciones y mermas,
  neutral en lo demás (compras, consumo, caja). Con `report_id` la tarjeta es un botón que abre su reporte
  detallado con el mismo periodo. Estados: carga, error con reintento, sin permisos, «No hay ventas cerradas en
  este periodo» (las cuentas abiertas están en «En curso»).
- Barras del periodo actual frente al anterior para los importes en pesos de cada sección (se omiten si todo es
  cero) y tendencias de las series de Go (venta neta por día) con enlace a su reporte.
- **No se recalcula con cada evento SSE**: «Actualizar», «Actualizado a las HH:MM» y un temporizador de 5 minutos que
  solo consulta con la ventana visible (y solo existe mientras el resumen está en pantalla).

### Exportación (`components/reports/ExportButtons.tsx`)

Botones «Exportar PDF» y «Exportar Excel» solo con `export.pdf`/`export.excel` del catálogo **y** el permiso
(`reports.export.pdf` / `reports.export.excel`); llaman a `exportReport` de la parte C1
(`services/reportExport.ts`, operación cerrada `export_report`) con la misma consulta de la pantalla (periodo,
comparación, orden y filtros; C1 quita la paginación). Resultado con `notify.ts`: «Cobros por método: PDF
guardado.» con nombre y tamaño, «Exportación a PDF cancelada: no se guardó ningún archivo.» o el error de Go/puente.

### «Hora de inicio del día»

Revisada en Configuración general (`SettingsPage.tsx`, parte B1): un solo lugar, explicación de que define el día
comercial de reportes e inventario, «Con 00:00 el día es la fecha del calendario», el ejemplo «con 05:00 una venta
de la 01:30 pertenece al día anterior» y que cambiarla no modifica registros. Cumple §0-B; sin cambios.

### Decisiones menores

1. Lista lateral de reportes desde 1440 px; por debajo, selector «Reporte» (a 1280 × 720 la tabla recibe el ancho
   completo). Ambos eligen lo mismo; solo uno es visible.
2. Periodo y filtros compartidos entre reportes; valores por parámetro + fuente.
3. La tendencia solo se dibuja con todo el periodo en una página y la tabla en orden temporal (si no, una nota lo
   explica); en un periodo en curso el rótulo directo va en el valor más alto (los días siguientes aún valen 0).
4. En la barra segmentada el color sigue al nombre de la parte (orden alfabético), no a su posición en la tabla.
5. Polaridad de los indicadores por lista de claves conocidas; las claves nuevas se muestran sin tono.
6. Tamaño de página fijo de 50 renglones (Go admite hasta 200); sin selector.
7. `Pagination` se reutiliza desde `pages/inventory/InventoryFields.tsx` sin moverlo (no se tocó Inventarios).
8. Búsqueda de productos e insumos solo con «Buscar» (nunca por tecla).

### Limitaciones

- Un periodo que no cabe en una página (p. ej. un año por día: 365 renglones) no dibuja tendencia; se sugiere el
  reporte por semana o mes. Las barras de categorías muestran solo los primeros 10 renglones de la página.
- Las opciones de filtros se consultan una vez por fuente mientras la pantalla está abierta (una categoría nueva
  aparece al volver a entrar).
- La interfaz no ofrece elegir papel A4 (C1 lo admite con `paper`); el PDF sale en carta.
- A 1280 × 720 los reportes con muchas columnas se desplazan a lo ancho dentro de su tabla (la página no).
- Sin verificación en la ventana nativa (WKWebView), con el diálogo nativo real ni en pantalla táctil física; la
  exportación se comprobó con el diálogo simulado de `native-driver.ts`.

### Pruebas ejecutadas (copia aislada `repo9aC2`, clúster temporal 55499, puertos 8187/1427)

Salida resumida real (registros en `docs9aC2/*.log`):

- `npx tsc --noEmit`: sin errores. `npx vitest run src`: 53 archivos, 467 pruebas aprobadas (incluye
  `pages/reports/reports.test.tsx` con 27: navegación por catálogo y permisos, cada periodo, semana ISO 53 y cambio
  de año, rango personalizado, totales de Go en el pie, paginación sin cambiar totales, columna que Go no entrega,
  orden con `aria-sort`, filtros en un solo panel y por parámetro + fuente, estados de carga, vacío y error,
  resumen sin recarga por SSE con «Actualizar» y temporizador solo visible, exportación por permiso con guardado,
  cancelación y error, elección de gráficos y ningún `type="number"`). `npm run build`: compilado.
- Playwright de la especificación nueva `e2e/workflow-reports-9a.spec.ts`: Chromium 1/1 y WebKit 1/1 (dos cuentas
  cerradas por la API; resumen con la cifra de Go; tarjeta → «Ventas por día» con tendencia y su tabla; ventas
  por producto en semana actual y semana ISO con el rango de Go; cobros por método con barra segmentada;
  exportación PDF y Excel guardadas y una cancelada con el diálogo simulado de C1; sin desbordamiento a
  1280 × 720 y 1920 × 1080 en ambos temas).
- Playwright completo (sin `SPEC`): Chromium 10/10 y WebKit 10/10 (incluye la especificación de exportación de C1).
- Validador de paleta (skill dataviz): claro y oscuro con todas las comprobaciones aprobadas (`palette.log`).
- Capturas en `scratchpad/shots9aC2/` (Chromium y WebKit; claro y oscuro; 1280 × 720 y 1920 × 1080): resumen,
  ventas por día (tendencia), ventas por producto, filtro de periodo (semana del año) y cobros por método.
