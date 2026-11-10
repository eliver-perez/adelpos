# Etapa 8 · Avance: inventarios, recetas, almacenes, compras y costos

Rama `etapa-8`, creada desde `main` (`53ccdb3`, versión 0.7.1, esquema 9). La especificación del propietario es
`docs/etapas/ETAPA_8.md`, que no se modifica. Las migraciones nuevas (0010 en adelante) **no se aplican a la
instalación real** sin autorización expresa del propietario (§28).

## Fase 0 · Verificación contra el código

Revisión de solo lectura del repositorio y del sistema anterior (`../legacy-csharp/`, sin compilar, ejecutar ni leer
archivos de configuración). Ningún hallazgo obliga a cambiar una decisión del propietario, así que la etapa continúa
sin detenerse. Las conclusiones de diseño están al final de esta sección.

### 1. Ingredientes, porciones y complementos del catálogo

- **No hay un modelo «sin / normal / extra»: todo es un número decimal de porciones** (de 0 a 1000, con 3 decimales)
  que se compara con lo incluido.
  - Ingredientes: `customization.ingredients[].quantity` frente a `catalog_product_ingredients.included_quantity`
    (con `removable`, `addable`, `max_quantity` y `extra_price` por porción sobre lo incluido).
  - Opciones de grupo (complementos): `customization.groups[].options[].quantity` frente a
    `catalog_group_defaults.quantity`.
  - «Sin cebolla» es `"0"`. El botón «Extra» de la captura fija `min(incluido + 1, máximo)`, que equivale al doble
    solo cuando lo incluido es 1. «2 porciones de papas» es `"2"`.
  - Go rechaza el mismo ingrediente o la misma opción dos veces (`catalog/calculator.go`,
    `catalog/group_calculator.go`): cada ingrediente tiene un único valor efectivo.
- **La partida confirmada guarda la selección resuelta** en `restaurant_item_versions.quote_line` (`ProductBreakdown`):
  - todos los ingredientes del producto (`ingredient_id`, `included`, `requested`);
  - todas las opciones de cada grupo (`option_id`, `complement_id`, `portion_id`, `included`, `requested`);
  - todos los componentes de combo (`components[].product`, recursivo, con su `quantity` y su personalización).
    El vínculo del complemento con su producto o ingrediente y el `factor` de la porción están en el snapshot del
    pedido (`restaurant_orders.pricing_snapshot`). La variante solo cambia el precio.
- **Precio y consumo son independientes.** La equivalencia por valor con piso en el precio base solo afecta el
  precio. El inventario leerá las cantidades `requested` de cada ingrediente y de cada opción.
- **Cantidades físicas:** no existen.
  - `catalog_product_ingredients.recipe_quantity` («Cantidad para receta futura») tiene un significado indefinido y
    vale 0 por omisión. Nadie la usa, y el inventario **no** la usará.
  - El `factor` de `catalog_portions` tampoco se usa hoy. El inventario lo aplicará como multiplicador de la opción.
- **Editar una partida confirmada** crea una versión nueva (`restaurant_item_versions`, solo inserción). La versión
  vigente de la unidad (`restaurant_item_units.version_id`) cambia; la que se liberó a cocina queda fija en
  `kitchen_unit_releases.version_id`.
- **Panel de complementos de la 6.1 frente a §0-B:**
  - Ya cumple:
    - tarjetas táctiles de 56 px, con los incluidos ya seleccionados;
    - sustitución en dos toques sin listas desplegables;
    - botones − y + de 44 px;
    - mínimos y máximos aplicados en la interfaz y validados en Go;
    - total de la partida en vivo, calculado por Go.
  - Falta, y se ajusta en la fase B:
    - en los grupos de varias porciones, que un toque en la tarjeta **sume** una porción (hoy la quita);
    - el − en la propia tarjeta y la cantidad en grande;
    - el precio en la tarjeta (hoy solo aparece si hay cargo extra);
    - el encabezado «2 de 2»;
    - «+$10.00» o «sin cambio» en el grupo (hoy solo se ve si es positivo);
    - un aviso al intentar pasar del máximo.

### 2. Liberación canónica de cocina (`kitchen_unit_releases`)

- **Estructura.** `kitchen_unit_releases` tiene `unit_id` como clave primaria y referencias a la liberación
  (`release_id`) y a la versión liberada (`version_id`). `released_at` es el inicio de la transacción. Ninguna fila
  se actualiza ni se borra.
- **Dónde se crea.** Solo en `kitchen/releases.go` (`prepareRelease`), una vez por unidad comercial y fuera del
  recorrido de estaciones, dentro de la transacción que libera y con el lock 7820148305 exclusivo. Las transacciones
  que llegan ahí son cuatro:
  - la confirmación inmediata, en la misma transacción comercial;
  - el planificador de pedidos programados;
  - la liberación manual;
  - el reintento de un cambio pendiente.
- **Liberaciones bloqueadas.** Una liberación con advertencias (sin ruta, impresora inactiva) queda bloqueada y no
  crea filas hasta que se resuelve.
- **Productos sin preparación.** Los productos «Sin preparación» reciben fila canónica, aunque no tengan tareas.
- **Comportamiento:**

  | Caso                                                                                   | Efecto en `kitchen_unit_releases`                                                                                       |
  | -------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
  | Varias estaciones                                                                      | Una tarea por estación (`kitchen_tasks`) y una sola fila canónica por unidad                                            |
  | Modificación antes de liberar                                                          | Se libera después con la versión vigente                                                                                |
  | Modificación después de liberar                                                        | No cambia la fila canónica; cocina recibe el aviso de modificación y las unidades añadidas reciben una liberación nueva |
  | Reducción de cantidad                                                                  | Las unidades sobrantes se cancelan (`cancelled_unit_ids` del evento `item.modified`)                                    |
  | Cancelación antes de liberar                                                           | Nunca crea fila                                                                                                         |
  | Cancelación después de liberar                                                         | La fila permanece; no existe clasificación «preparado / no preparado»                                                   |
  | Transferir, dividir, combinar, mover de mesa, reimprimir, cambiar de impresora o pagar | No tocan las filas canónicas                                                                                            |
  | Reintento HTTP                                                                         | Devuelve la respuesta guardada en `restaurant_operations`                                                               |
  | Evento reprocesado                                                                     | Lo detiene `kitchen_event_receipts`                                                                                     |

- **Modalidad.** La de la cuenta (`restaurant_accounts.mode`: `dine_in` o `takeaway`, que incluye ventanilla) no
  cambia en toda la vida de la unidad y se congela en la liberación.

### 3. Indicaciones rápidas de la 7.1

- La tabla `catalog_category_quick_notes` (0009) guarda solo texto, orden, activa y revisión, sin vínculo con
  ingredientes.
- Llegan a la captura como texto y se unen con «, » en `customization.notes`. La partida no guarda su identificador.
- **Cambio que exige §9** (fase B), compatible con lo existente:
  - **Indicación:** efecto (`none|omit|normal|extra`) e ingrediente vinculado; las indicaciones actuales quedan sin
    efecto.
  - **Partida:** `customization.quick_notes` con el identificador y el texto, opcional. Los borradores y las partidas
    existentes no cambian: conservan solo texto y siguen sin efecto en inventario.
  - **Efecto:** fija la misma cantidad que el personalizador; Go valida que la indicación aplique al producto y no
    acumula efectos.
  - **Comanda:** se evita imprimir dos veces lo mismo («SIN Cebolla» y «NOTA: Sin cebolla»).

### 4. Gastos de caja de la etapa 7

- **Estructura.** Un gasto es una fila inmutable de `cash_operations` (`kind='expense'`), con:
  - moneda MXN o USD;
  - importe con 2 decimales;
  - categoría;
  - turno;
  - autorización;
  - `operation_id`.
- **Corrección.** Una corrección es otra fila (`kind='correction'`) enlazada por `corrects_operation_id`, con un
  índice único parcial que impide corregir dos veces. La corrección de un turno cerrado es una regularización en el
  turno vigente, sin cambiar la diferencia del cerrado.
- **Estado.** Se deriva (`recorded`, `corrected`, `correction`, `regularization`); no hay columna de estado. Tampoco
  existe búsqueda de gastos entre turnos.
- **Vinculación con compras (§20).** Va en una tabla propia del inventario, sin tocar `cash_operations` ni el turno.
  - **Al vincular:** el gasto debe existir, ser `expense` en pesos y no estar corregido.
  - **Después de vincular:** una corrección del gasto se detecta y se señala.

### 5. Sistema anterior

- **Tablas.** `articulos` (con tipos fijos, entre ellos mobiliario y equipo), `unidades` (seis fijas, sin
  conversiones), `inventario` (existencia global con 3 decimales), `entradas` y `entradas_detalles` (subtotal, IVA y
  total por renglón y documento), `ajustes_inventario` (cantidad con signo y descripción, sin usuario), `recetas` (por
  producto, sin versiones) y `proveedores`.
- **Lo que no existía:** almacenes, costos, órdenes de compra, devoluciones, transferencias, conteos ni mínimos.
- **Corrección a la especificación:** el §0.5 dice que el sistema anterior descontaba inventario al capturar. En la
  versión del código disponible el descuento ocurría **al registrar el pago completo de la cuenta**: los pagos
  parciales no descontaban y el último descontaba toda la venta. No cambia ninguna decisión, porque AD-EL POS consume
  al liberar a cocina.
- **Otras diferencias** (gana lo decidido en AD-EL POS):
  - ignoraba personalizaciones y complementos;
  - nunca reintegraba cancelaciones ni registraba mermas;
  - permitía existencias negativas por venta pero no por ajuste, y el cobro completo fallaba si un ingrediente no
    tenía registro de existencia;
  - el usuario que recibía era siempre el administrador;
  - no relacionaba compras con caja.
- **Funciones del sistema anterior que no están en el plan** (se reportan sin implementarlas, `CLAUDE.md` §21):
  - imágenes de artículo;
  - reporte de compras por periodo (corresponde a la etapa 9);
  - segundo teléfono, celular, página web y domicilio desglosado del proveedor;
  - RFC único y obligatorio;
  - tasa de IVA de compra libre por renglón;
  - bloqueo de ajustes que dejan existencia negativa;
  - tipo de artículo «Utensilios».

  El mobiliario y el equipo son control de activos y quedan fuera, como indica la especificación.

- **Recepción directa.** En el restaurante todas las compras eran recepciones directas, sin orden. La etapa 8 ofrece
  la recepción directa como flujo rápido y explícito (§18).

### Conclusiones de diseño de la Fase 0

1. **Consumo de ingredientes:** porciones solicitadas × porción habitual del ingrediente (cantidad por porción), como
   indica §0-B para catálogos con cantidades. Sin, normal y extra dan 0, 1 y 2 cuando lo incluido es 1. La porción se
   configura en el ingrediente y cada receta puede sobrescribirla para su producto. Sin porción configurada no se
   inventa consumo.
2. **Consumo de complementos:**
   - fórmula: cantidad elegida × factor de la porción × la receta que aplique;
   - receta que aplica, en este orden de prioridad: la del complemento, la del producto vinculado, la porción del
     ingrediente vinculado;
   - el precio nunca interviene.
3. **Solicitudes de consumo:**
   - **Liberación:** `prepareRelease` registra la solicitud después de crear la fila canónica, solo con la función
     activada, sin lock de inventario y sin leer recetas.
   - **Modificaciones y cancelaciones** de unidades ya liberadas: se registran desde `kitchen.Integrate`
     (`item.modified` e `item.cancelled`), aunque el cambio de cocina quede pendiente.
4. **Fechas que fijan el consumo:**
   - el trabajador resuelve recetas, porciones y almacenes a la fecha de la liberación, con historial de vigencias;
   - la fecha de activación se guarda en `establishment_features.enabled_at`, que hoy no existe;
   - el día operativo usa la zona horaria del establecimiento (`establishment.timezone`) y una hora de corte nueva.
5. **Pedidos confirmados antes de activar el inventario y liberados después:** consumen, porque la regla de §0-B es
   la hora de liberación. Las unidades liberadas antes de activar nunca consumen ni generan ajustes.

El diseño completo, con el modelo de datos y las reglas del motor de costos, se resume por fase en las secciones
siguientes a medida que se implementa.

## Fase A · backend (A1)

Núcleo de inventario en Go y PostgreSQL: migración `0010_inventory_core.sql`, paquete `backend/internal/inventory`
(motor de costos, libro de movimientos, unidades, insumos, presentaciones, almacenes, existencias, existencia inicial,
configuración y resumen), rutas `inventory/*`, permisos, eventos, puente Tauri y `desktop/src/services/inventory.ts`.
La interfaz de la fase A (A2) se construye después sobre este contrato. En esta fase **todavía no hay consumo**: la
función «Control de inventario» se puede activar, pero ninguna liberación a cocina genera movimientos hasta la fase B.
La 0010 no se aplicó a la instalación real.

### Migración 0010

Transaccional (sin `CONCURRENTLY` ni `VACUUM`), sin tocar 0001–0009 ni filas existentes, con restricciones de nombre
explícito y CHECK que exigen `IS NOT NULL` en la rama que valida.

| Estructura                          | Contenido                                                                                                                                                                                                                                                                                    |
| ----------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `establishment_features.enabled_at` | Fecha y hora de la última activación (NULL en las filas existentes). Fila `inventory_control` desactivada.                                                                                                                                                                                   |
| `realtime_events_scope_check`       | Recreado con el mismo nombre: los seis ámbitos anteriores + `inventory`.                                                                                                                                                                                                                     |
| `inventory_settings`                | Fila única con revisión y todas las columnas de las fases A–D: `purchase_tax_in_cost` (true), `operating_day_cutoff` (05:00), `warn_products_without_recipe` (true), `count_frequency` (`none`) y `count_interval_days` (solo con `custom`, 1–365).                                          |
| `inventory_units`                   | `code` único, `name` único sin mayúsculas, `dimension` (`mass`, `volume`, `count`, `portion`), `factor numeric(24,10)` > 0 y ≤ 1,000,000, `system`, `active`. Siembra: g, kg (1000), ml, l (1000), pza, porción.                                                                             |
| `inventory_categories`              | Nombre único sin mayúsculas, orden por posiciones, revisión.                                                                                                                                                                                                                                 |
| `inventory_items`                   | Código y nombre únicos sin mayúsculas, código de barras único opcional, categoría, `base_unit_id`, `kind` (`purchased`, `prepared`), `reference_cost numeric(24,10)` > 0 o NULL, `min_stock`, `reorder_point`, revisión y responsables.                                                      |
| `inventory_presentations`           | `base_quantity numeric(20,6)` en la unidad base, nombre único por insumo, código de barras único, una sola predeterminada de compra (activa) por insumo.                                                                                                                                     |
| `inventory_warehouses`              | Nombre único; `is_default` único y siempre activo. Siembra: «Almacén general».                                                                                                                                                                                                               |
| `inventory_station_warehouses`      | Historial de solo inserción estación → almacén (`warehouse_id` NULL = predeterminado), `effective_from`, actor, `operation_id`.                                                                                                                                                              |
| `inventory_stock`                   | `(item_id, warehouse_id)` → cantidad `numeric(20,6)` (puede ser negativa).                                                                                                                                                                                                                   |
| `inventory_valuations`              | Por insumo: cantidad total, valor total, `last_valid_cost`, `pending_cost_quantity`; CHECK `inventory_valuations_zero_check` (cantidad 0 ⇒ valor 0).                                                                                                                                         |
| `inventory_movements`               | Libro de solo inserción con los 15 tipos, valor con signo, `unit_cost`, `cost_basis`, `cost_pending`, `occurred_at`/`recorded_at`, origen, unidad comercial, `related_movement_id`, motivo, usuario **o** proceso, `operation_id`, `entry_key` único y estado posterior; índices del diseño. |
| Disparadores                        | `app.inventory_reject_mutation()`: rechaza UPDATE, DELETE y TRUNCATE en `inventory_movements` e `inventory_station_warehouses`.                                                                                                                                                              |
| Permisos                            | Los 10 de la fase A, concedidos solo al rol `administrator`; el rol «Inventarios» sigue sin permisos.                                                                                                                                                                                        |

### Motor de costos (`inventory/costing.go`)

Función pura sobre `shopspring/decimal`. Estado por insumo: cantidad total Q, valor total V, último costo válido L y
costo de referencia configurado R; el costo de referencia vigente es L o, si nunca hubo, R. Operaciones: `Issue`
(salidas), `Receive` (entrada con documento y existencia inicial), `Admit` (ajuste positivo y sobrante), `Restore`
(reversión a valor exacto y entrada de elaboración), `Transfer` y `ReturnToSupplier`. Cada una devuelve el valor con
signo del movimiento, su costo unitario y base, el ajuste de costo separado y el residuo de valuación cuando
corresponden, y el estado nuevo. Reglas 1–8 del §5 del diseño, con estas precisiones:

- **Precisión elegida**: cantidades 6 decimales en la unidad base, valores 6 decimales y costos unitarios 10, con
  redondeo mitad alejándose de cero (`Round` y `DivRound`, sin redondeo doble). El redondeo a centavos solo se usa en los
  campos `*_display`.
- **Entrada con documento por valor**: `Receive` recibe el valor total del documento (no el costo unitario). Si cubre
  parte del déficit, el valor real de las unidades que cubren es `valor × d ÷ q` (exacto si d = q); el ajuste de costo es
  provisional − real.
- **Último costo válido**: L solo se actualiza con Q > 0 **y** V > 0. Un promedio de cero solo puede venir de entradas
  sin costo y no es un costo válido (decisión conservadora: conserva la señal de «costo pendiente»).
- **Cantidad pendiente de costo**: nunca excede el déficit vigente; baja en min(pendiente, d) cuando una recepción,
  un ajuste positivo o una reversión cubre d unidades del déficit (aproximación documentada: es un indicador para
  alertas, no interviene en ningún valor).
- **Ajuste positivo con existencia negativa** (regla 3): el déficit cubierto se valora a su provisional exacto y el
  resto a la referencia vigente; sin ningún costo, valor cero y «costo pendiente». Sin ajuste de costo separado.
- **Devolución**: la diferencia contra el costo original la absorbe la existencia restante (si es positiva), sin dejar
  el valor negativo; el resto se devuelve como «diferencia no absorbida» para el documento de la fase C.
- **Invariante** Q = 0 ⇒ V = 0: residuo `cost_adjustment` de −V y, además, el CHECK de la base.

Casos de prueba (`costing_test.go`, cada uno con el cálculo a mano anotado):

| Caso                                      | Datos                                                              | Resultado                                                                                    |
| ----------------------------------------- | ------------------------------------------------------------------ | -------------------------------------------------------------------------------------------- |
| Promedio con existencia positiva (§15)    | 10 u a $20 + 10 u a $30                                            | 20 u, $500, promedio $25                                                                     |
| Salida a cero exacto                      | 3 u por $10: sale 1 u y luego 2 u                                  | −3.333333 y −6.666667 (exacto V); 0 u, $0, sin residuo                                       |
| Salida que cruza a negativo               | 2 u por $50 (L = 25), sale 5 u                                     | −$125 (50 + 3 × 25), base `reference`; −3 u, −$75                                            |
| Salida con existencia negativa a L        | −3 u por −$75 (L = 25, R = 40), sale 2 u                           | −$50 a L; −5 u, −$125                                                                        |
| Salida a R sin L                          | 0 u, R = 18, sale 4 u                                              | −$72, base `configured`; −4 u, −$72, L sin valor                                             |
| Varias salidas negativas con R distinto   | R = 18 sale 4 u; R = 20 sale 2 u; recepción 10 u por $250          | −6 u, −$112; ajuste 112 − 150 = −$38; 4 u, $100 (L = 25)                                     |
| Recepción que cubre parte del déficit     | −6 u, −$112; recepción 4 u por $100; después 2 u por $50           | ajuste −25.333333 (−2 u, −37.333333); ajuste −12.666667 (0 u, $0); total −$38                |
| Recepción que cruza a positivo (§15)      | −5 u, −$100 (L = 20); recepción 8 u por $240                       | ajuste −$50; 3 u, $90, promedio $30                                                          |
| Costo pendiente y su resolución           | sin L ni R sale 5 u; recepción 8 u por $240                        | salida $0 pendiente (5 u); ajuste −$150; 3 u, $90, nada pendiente                            |
| Recepción parcial de un déficit pendiente | −5 u pendientes; recepción 3 u por $30; sale 1 u                   | ajuste −$30; −2 u, $0, pendiente 2; luego pendiente 3                                        |
| Ajuste positivo                           | 20 u, $500 + 4 u; −5 u, −$100 (L = 20) + 8 u; 0 u sin costos + 2 u | +$100 (24 u, $600); +$160 base `provisional` (3 u, $60); $0 pendiente (2 u, $0, L sin valor) |
| Transferencia                             | 20 u, $500 traslada 5 u; −2 u, −$50 (L = 25) traslada 1 u          | $125 y $25; Q, V y L sin cambio                                                              |
| Devolución con diferencia absorbida       | 20 u, $500; devuelve 4 u a $30 (y a $20)                           | salida −$100, documento $120, ajuste −$20 (16 u, $380, L 23.75); a $20: ajuste +$20 ($420)   |
| Devolución con diferencia no absorbida    | 4 u, $100 devuelve 4 a $30; 5 u, $10 devuelve 4 a $30              | sin ajuste, $20 sin absorber; ajuste limitado −$2 y $110 sin absorber (1 u, $0, L = 2)       |
| Sobrante y faltante de conteo             | 10 u, $250: faltante 2 u; sobrante 3 u                             | −$50 (8 u, $200); +$75 (11 u, $275)                                                          |
| Invariante Q = 0 ⇒ V = 0                  | −5 u, −$100; reversión de 5 u registradas en $150                  | residuo −$50; 0 u, $0                                                                        |
| Redondeo fraccionario                     | 1250.5 g por $87.53; salidas de 12.3456 g, 0.000125 g y el resto   | costo 0.0699960016; −0.864143; −0.000009; −86.665848 exacto; 0 g, $0                         |

Conversiones (`units_test.go`): kg ↔ g, l ↔ ml, onza (28.349523125 g) redondeada a 6 decimales, conversión que se
redondea a cero rechazada, caja de 24, paquete de 12, costal de 25 kg en un insumo en gramos y rechazo entre
dimensiones.

### Libro de movimientos (`inventory/ledger.go`)

`Apply(ctx, tx, MovementInput)` es la única vía para cambiar existencias y valuaciones y la usarán todas las fases:
comprueba `entry_key` (si existe devuelve lo registrado con `Replayed`), bloquea con `FOR UPDATE` la valuación y la
existencia (las crea en cero), aplica el motor, inserta el movimiento principal y, si corresponden, el ajuste de costo
(`<clave>:cost`) y el residuo (`<clave>:residual`) ligados por `related_movement_id`, cada uno con el estado posterior,
y actualiza existencia y valuación. Verifica que la suma de valores coincida con el motor. Una existencia negativa se
informa (`NegativeStock`) y nunca bloquea. `ApplyTransfer` registra la pareja `transfer_out`/`transfer_in` con el mismo
valor (claves `<clave>:out`/`<clave>:in`). El llamador toma 7820148308; `Apply` lo vuelve a pedir (los locks consultivos
son reentrantes) como red de seguridad.

### Rutas y permisos

Todas exigen `inventory.read` más el permiso de la ruta; contratos completos en [API_CONTRACTS.md](../API_CONTRACTS.md)
(«Inventario · esquema 10») y tipos en `desktop/src/services/inventory.ts`.

| Ruta                                                                                                                                       | Permiso adicional                                                                                          |
| ------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------- |
| `GET summary`, `GET settings`, `GET units`, `GET categories`, `GET items`, `GET items/{id}`, `GET warehouses`, `GET station-warehouses`    | —                                                                                                          |
| `PUT settings`                                                                                                                             | `inventory.settings.manage`                                                                                |
| `POST units`, `PATCH units/{id}`, `POST/PATCH categories`, `POST/PATCH items`, `POST items/{id}/presentations`, `PATCH presentations/{id}` | `inventory.items.manage` (+ `inventory.costs.manage` para enviar `reference_cost`)                         |
| `POST warehouses`, `PATCH warehouses/{id}`, `PUT station-warehouses/{estación}`                                                            | `inventory.warehouses.manage`                                                                              |
| `GET stock`                                                                                                                                | `inventory.stock.read` (costos con `inventory.costs.read`, valuación total con `inventory.valuation.read`) |
| `GET movements`                                                                                                                            | `inventory.movements.read` (valores con `inventory.costs.read`)                                            |
| `POST initial-balances`                                                                                                                    | `inventory.adjustments.authorize` + `inventory.costs.manage`                                               |

Registro: rama `inventory/` en `administration/routes.go` (traduce `inventory.Error`), `inventory.Permissions` en
`administration/permissions.go`, ámbito `inventory` → `inventory.read` en `events.allowed`, límite de cuerpo de 2 MiB
para `inventory/` (existencia inicial de hasta 500 renglones), `ordering.InventoryCategories`, `features.InventoryControl`
y `enabled_at` en `PUT settings/features`, que además publica `inventory.settings.changed` en el ámbito `inventory`. Lock
7820148308 en `CLAUDE.md` §3. Las pruebas de integración viven en `internal/operations`, así que `scripts/database.py`
no cambia.

### Decisiones y desviaciones menores

1. **Toda escritura de inventario exige `operation_id`** (también las entidades administrables), con la misma huella,
   respuesta guardada y 409 `IDEMPOTENCY_CONFLICT` de `payments`, además de `revision`. Es lo que pide el §2 del diseño
   y deja un solo patrón de escritura para la interfaz (`useRestaurantMutation`).
2. Todas las rutas exigen `inventory.read` además del permiso propio: la sección es la frontera de acceso.
3. Los costos ocultos llegan en `null` y la respuesta trae `costs_visible`/`valuation_visible` (el diseño admitía
   «ausentes o null»). Sin `inventory.costs.read` tampoco se listan los ajustes de costo (solo valor) ni la marca de
   costo pendiente. Como React no redondea, Go entrega `*_display` (valores a centavos y costos a cuatro decimales).
4. Columna adicional `inventory_movements.related_movement_id` (ajuste de costo, residuo y entrada de transferencia
   apuntan al movimiento que los originó). `source_kind` se valida por formato y no por lista cerrada, para que las fases
   B–D no recreen el CHECK; `kind` sí es lista cerrada con los 15 tipos.
5. Existencia inicial: solo si el insumo no tiene movimientos en ese almacén (después, recepción o ajuste); costo por
   unidad capturada (por caja si se captura en cajas) con valor exacto cantidad capturada × costo; `use_reference_cost`
   excluyente con `unit_cost`; costo 0 admitido como costo explícito; sin insumos repetidos; 1–500 renglones; fecha no
   futura (por omisión, la del servidor).
6. Conversiones redondeadas a 6 decimales (mitad alejándose de cero); una conversión que se redondea a cero se rechaza.
   Límites: cantidades hasta 1,000,000,000 en la unidad base, costos capturados hasta 1,000,000.
7. `reference_cost` debe ser mayor que cero (NULL = sin costo de referencia). Unidad base: no cambia con movimientos ni
   con presentaciones, y si el insumo tiene costo de referencia el cambio exige capturarlo de nuevo (es por unidad base).
8. Código de barras único entre insumos **y** presentaciones (índices únicos por tabla + comprobación en Go bajo el lock).
9. Unidades propias: abreviatura y dimensión inmutables; la equivalencia solo cambia si la unidad no es base de ningún
   insumo; no se desactiva la unidad base de un insumo activo. Las del sistema no se modifican.
10. Almacenes: el predeterminado no se desactiva ni se reasigna por API; no se desactiva un almacén con existencias
    distintas de cero ni uno asignado a una estación (activa o inactiva: al reactivarse volvería a consumir de él).
11. Asignación estación → almacén: `revision` = número de asignaciones registradas; repetir el vigente no agrega
    historial; `null` vuelve al predeterminado.
12. Eventos: almacenes y asignaciones usan `inventory.settings.changed` (sin tipo nuevo); la existencia inicial publica
    `inventory.stock.changed` e `inventory.movements.recorded`.
13. Bajo mínimo y punto de reabastecimiento comparan la existencia **total** del insumo (los niveles por almacén llegan
    en la fase D).
14. Filtros de fecha de movimientos: días de negocio `AAAA-MM-DD` en la zona horaria del establecimiento.
15. `enabled_at`: NULL en las funciones existentes; se fija al pasar de desactivada a activada y se renueva al
    reactivar.
16. La auditoría de inventario puede contener costos (solo `audit.read`, de administración).
17. Pantalla de inicio: `administration/preferences.go` no cambió. Si la interfaz quiere «Inventarios» como pantalla de
    inicio, debe agregar `"inventory": "inventory.read"` a `screenPermissions` con el mismo identificador de pantalla.

### Limitaciones

- Sin consumo, recepciones, ajustes, transferencias, conteos ni devoluciones por API: el libro y el motor ya los
  admiten (probados por llamadas directas de Go en la integración), pero sus rutas y documentos llegan en las fases B–D.
- El valor es por insumo; no hay valuación por almacén.
- La «diferencia no absorbida» de una devolución solo la devuelve la primera aplicación (un reintento del libro no la
  recalcula); el documento de la fase C debe guardarla.
- La cantidad pendiente de costo es un indicador agregado (ver motor); no se probó bajo carga ni con concurrencia real
  más allá del lock exclusivo.

### Pruebas de la fase A (backend)

- **Motor** (`inventory/costing_test.go`, 18 pruebas): todos los casos de la tabla anterior, más entradas inválidas.
- **Conversiones y decimales** (`inventory/units_test.go`, 7 pruebas) y **permisos Go ↔ 0010**
  (`inventory/permissions_test.go`: misma clave y descripción, concesión solo al administrador, 10 permisos).
- **Integración PostgreSQL** (`operations/inventory_core_test.go` e `inventory_ledger_test.go`, encadenadas al final de
  `TestRestaurantAcceptance`): siembras (unidades, «Almacén general», función desactivada sin `enabled_at`,
  configuración, permisos iguales en Go y PostgreSQL y solo del administrador; el rol «Inventarios» vacío); unidades
  propias, unidad del sistema inmutable, abreviatura repetida y revisión obsoleta; categorías con posiciones; insumos
  (duplicados de código, nombre y código de barras; revisión; auditoría con antes y después; costo conservado si no se
  envía; cambio de unidad base rechazado con presentaciones o sin recapturar el costo y aceptado al recapturarlo);
  presentaciones (caja de 24, paquete de 12, costal de 25 kg convertido a 25,000 g, una sola predeterminada, edición con
  revisión, rechazo entre dimensiones y de código de barras repetido); permisos (el capturista no fija costos, sin
  `items.manage` no escribe y nada queda registrado, sin `inventory.read` 403); almacenes y asignación de estaciones con
  historial (revisión, misma asignación sin historial, almacén asignado o con existencias no se desactiva, el
  predeterminado tampoco, historial de solo inserción en PostgreSQL); existencia inicial (unidad, presentación, otra
  unidad de la dimensión y costo de referencia confirmado; mismo `operation_id` devuelve la respuesta guardada sin
  duplicar; cuerpo distinto 409; existencia previa, referencia faltante, sin costo, ambos costos, cantidad negativa y
  dimensión distinta rechazados; 403 sin los dos permisos; auditoría y eventos); libro por llamadas directas de Go como
  lo hará el trabajador (consumo que cruza a negativo, idempotencia por `entry_key`, recepción con ajuste de costo de
  −$10, transferencia con el mismo valor e idempotente, tipo no admitido) con consistencia Σ existencias = cantidad de la
  valuación, Σ valores = valor de la valuación, Σ cantidades por almacén = existencia y último estado = valuación;
  libro inmutable (UPDATE, DELETE, TRUNCATE) y CHECK cantidad 0 ⇒ valor 0; consulta de movimientos (por insumo, tipo con
  paginación, almacén, días de negocio, origen; sin costos no ve ajustes de costo ni valores); visibilidad de costos en
  insumos, existencias y resumen (valuación total $985.50 solo con `valuation.read`); configuración con revisión,
  validaciones, 403 y auditoría; función `inventory_control` con `enabled_at` fijado, conservado al desactivar y
  renovado al reactivar; eventos `inventory` recibidos con `inventory.read` y ninguno sin él.
- **Actualización desde el esquema 9** (`database/existing_install_test.go`, `TestStage8UpgradeOfExistingInstallation`):
  instalación con roles (incluido «Inventarios»), administrador con todos los permisos, cajera, estación, `usd_cash`
  activa, turno con un gasto corregido y otro vigente y un evento de caja; la 0010 se aplica (1 migración) y una segunda
  ejecución no aplica nada; los datos anteriores no cambian (`usd_cash` sin `enabled_at`, gastos y corrección, permisos
  de la cajera, evento); siembras y concesión solo al administrador; los siete ámbitos de eventos; las 70 restricciones
  con nombre; CHECK probados con tablas temporales (movimientos, configuración, valuaciones, unidades e insumos) y libro
  de solo inserción.
- **Rust** (`api.rs`): `inventory_routes_are_closed_and_keep_write_methods` (23 operaciones, método y ruta, sin
  identificador o con `../../settings` rechazadas, sin borrado ni edición de existencias) e
  `inventory_query_parameters_are_whitelisted` (lista blanca `inventory_`, `order_by` e `include_costs` rechazados).

### Verificación de la fase A (backend)

En la copia aislada del scratchpad, con el clúster temporal del puerto 55499 (nunca la instalación real):

- `prettier --check`, `gofmt -l`, `cargo fmt --check`, `sync-metadata --check`, `go vet` y `go vet -tags=integration`:
  aprobados.
- `go test -race ./...` (unitarias): todos los paquetes `ok`, incluido `internal/inventory`.
- Integración (`go test -race -tags=integration`): `internal/database` ok (2.9 s), `internal/administration` ok
  (53.4 s), `internal/operations` ok (137.1 s, con la aceptación completa de las etapas 4–7.1 y la fase A), y
  `internal/payments` y `internal/catalog` ok.
- Vitest 39 archivos y 317 pruebas aprobadas; `tsc --noEmit` y `npm run build` aprobados.
- `cargo test --locked --offline`: 17 aprobadas (las 15 anteriores y las 2 de inventario; 1 ignorada, la misma de
  antes).
- Playwright no se ejecutó: la fase A no cambia pantallas (la interfaz es A2).
