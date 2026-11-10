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

## Fase A · interfaz (A2)

Sección principal **Inventarios** (`desktop/src/pages/inventory/`) sobre el contrato de A1. En Go solo cambió la
pantalla de inicio (`"inventory": "inventory.read"` en `administration/preferences.go`, con su prueba en
`security_test.go` y la nota en `API_CONTRACTS.md`); el contrato de inventario no cambió. La 0010 no se aplicó a la
instalación real.

### Pantallas

La pantalla `inventory` «Inventarios» está en la lista `screens` después de Catálogo, visible con `inventory.read`, y
se puede elegir como pantalla de inicio. El menú principal no usa iconos; el icono nuevo `inventory` de `Icon.tsx`
(caja, mismo trazo) acompaña al botón «Existencia inicial». `InventoryPage.tsx` sigue el patrón de `CashPage`:
pestañas por permiso, sin pestañas vacías, y el panel «Reintentar con el mismo identificador» si una escritura quedó
incierta.

| Pestaña       | Permiso                                                      | Contenido                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| ------------- | ------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Resumen       | `inventory.read`                                             | «Control de inventario» activado (desde cuándo) o desactivado, con la explicación de que solo consume lo liberado a cocina después de activarlo; «Administrar funciones» abre Configuración → Funciones del establecimiento (con `settings.features.manage` y `settings.read`), sin un segundo interruptor. Cifras de Go: insumos activos e inactivos, almacenes, existencias negativas, bajo mínimo, punto de reabastecimiento, costo pendiente (solo con costos) y valor del inventario (solo con valuación). Las tres de existencia abren Existencias filtrada.          |
| Insumos       | `inventory.read` (edición con `inventory.items.manage`)      | Listado con búsqueda, categoría (incluida «Sin categoría»), estado, tipo y paginación de 25. Panel de alta y edición (nombre, código, código de barras, descripción, categoría, unidad base, activo, costo de referencia, existencia mínima y punto de reabastecimiento) y, en el mismo panel, presentaciones de compra (nombre, contenido en la unidad base o en otra unidad de la dimensión, código de barras, predeterminada para compra, activa). Acordeones «Categorías de insumos» (con `PositionField`) y «Unidades de medida» (alta y edición de unidades propias). |
| Almacenes     | `inventory.read` (edición con `inventory.warehouses.manage`) | Almacenes con su descripción, estado, estaciones que consumen de cada uno y «Predeterminado» marcado (su interruptor de activo está deshabilitado). «Almacén de consumo por estación»: almacén vigente, «Usar el predeterminado», desde cuándo y quién, y el historial en un acordeón por estación.                                                                                                                                                                                                                                                                         |
| Existencias   | `inventory.stock.read`                                       | Por insumo: existencia (total o en el almacén filtrado), total, desglose por almacén, mínimo y reabastecimiento, situación y categoría; costo promedio y valor del insumo solo con `costs_visible`, valor total solo con `valuation_visible`. Filtros de búsqueda, almacén, categoría, situación (negativas, en cero, positivas, bajo mínimo, reabastecimiento) e insumos activos o inactivos. Las negativas se resaltan con borde y fondo de advertencia, etiqueta y un aviso de estado que aclara que no bloquean.                                                        |
| Movimientos   | `inventory.movements.read`                                   | Libro del más reciente al más antiguo: fecha de negocio y de registro, insumo, almacén, tipo, cantidad con signo, existencia después, origen legible y motivo, responsable (persona o proceso) y, con costos, valor con signo, costo unitario y base de costo. Filtros de insumo, almacén, tipo, origen y fechas de negocio con `OptionalDateField`; paginación de 50. Sin identificadores técnicos.                                                                                                                                                                        |
| Configuración | `inventory.settings.manage`                                  | «El IVA de las compras forma parte del costo», hora de corte del día operativo (HH:MM), aviso de productos sin receta y frecuencia de conteo con intervalo personalizado (1–365 días), con revisión.                                                                                                                                                                                                                                                                                                                                                                        |

«Existencia inicial» (en Existencias, con `inventory.adjustments.authorize` e `inventory.costs.manage`): almacén
(el predeterminado propuesto), motivo opcional y renglones con insumo (búsqueda por nombre, código o código de barras),
cantidad, unidad base, otra unidad de la dimensión o presentación, y costo unitario por lo capturado o «Usar el costo
de referencia». El resultado muestra cada renglón convertido por Go, la existencia después, el valor (con costos) y
las advertencias que devuelva Go (además de un aviso por cada una).

### Decisiones y desviaciones menores

1. **Configuración solo con `inventory.settings.manage`.** Go deja leer los parámetros con `inventory.read`, pero la
   pestaña de solo lectura no aporta; las categorías y unidades, que administra `inventory.items.manage`, viven en
   Insumos por la misma razón.
2. **Presentaciones después del alta.** Go necesita el identificador del insumo: «Crear insumo» deja el panel abierto
   con el insumo guardado y su sección de presentaciones, sin volver al listado.
3. **Costo de referencia.** Sin `costs_visible` no aparece en el listado ni en el panel y nunca se envía. Con
   `costs_visible` y sin `inventory.costs.manage` se muestra de solo lectura. Al editar se envía solo si cambió o si
   cambió la unidad base (el costo es por unidad base; Go exige recapturarlo). El tipo (`kind`) se envía siempre: Go
   pondría `purchased` si faltara.
4. **Cantidades.** `inventoryFormat.ts` solo da formato al texto de Go: miles, menos tipográfico, signo explícito en
   movimientos y la abreviatura de la unidad unida por un espacio de no separación («25,000 g», «+50 kg»). La captura
   usa campos de texto con dos formas nuevas en `services/numericInput.ts`: `inventoryQuantityShape` (6 decimales,
   hasta 10 enteros) e `inventoryCostShape` (10 decimales, hasta 7 enteros, también para equivalencias), iguales a los
   límites de Go. La unidad del sistema «porción» se muestra con su abreviatura (`porcion`).
5. **Escrituras.** `useInventoryMutation` envuelve `useRestaurantMutation` con la clave `inventory` del almacén global
   de pendientes. Ante `REVISION_CONFLICT` o `IDEMPOTENCY_CONFLICT` la pestaña recarga y el aviso dice «Se cargaron los
   datos vigentes: revísalos y vuelve a guardar»; los editores llevan la clave `id:revisión`, así que se vuelven a
   montar con la versión vigente (lo capturado y no guardado se pierde, igual que en Cobros).
6. **Desactivar se confirma al pulsar el interruptor** (`ActiveToggle`, tono de peligro) en insumos, presentaciones,
   categorías, unidades y almacenes; el cambio se aplica al guardar. `Form` no permite cancelar un guardado desde
   `onSave` sin mostrar un error, por eso la confirmación va antes.
7. **Existencia inicial.** Siempre se confirma (el libro no se edita ni se borra) y la confirmación nombra los insumos
   que usarán el costo de referencia y su costo si es visible. Las cadenas viajan tal como se capturaron; la unidad
   base no envía `unit_id`. La fecha de negocio no se captura (Go usa la hora del servidor; el contrato la admite como
   opcional). Sin costos visibles no se sabe si hay costo de referencia: el interruptor se ofrece y Go responde
   `REFERENCE_COST_MISSING` si falta.
8. **Estación → almacén.** El almacén predeterminado no se ofrece como opción explícita: «Usar el predeterminado» envía
   `null`, para que una estación sin asignación siga siempre al predeterminado.
9. **Eventos.** `useRealtime` publica además `adel-inventory-change` con los tipos `inventory.*` (y ante un reinicio
   del cursor), como ya hacía con `adel-cash-change`; `useResource` acepta `reloadEvent` y las consultas de la sección
   se recargan solo con ese evento y después de una escritura propia, no con los cambios de cocina o caja ni por
   sondeo. Una estación creada en Cocina aparece en Almacenes al volver a abrir la pestaña.
10. **Existencias** filtra por omisión los insumos activos, como cuentan las cifras del resumen.
11. **Listas de interfaz.** Los tipos de movimiento del filtro repiten las etiquetas de Go; el filtro de origen solo
    ofrece «Existencia inicial», el único origen de la fase A (las fases B–D agregan los suyos).
12. **`<select>` en WebKit.** El WebView de macOS dibuja los `<select>` con su apariencia nativa e ignora la altura de
    44 px; en Inventarios se dibujan como los demás campos (`appearance: none` y una flecha con los tokens del tema).
    El resto de la aplicación no se tocó: allí los `<select>` siguen con la apariencia nativa en WebKit.
13. **Playwright con varias especificaciones.** `e2e.sh` sin `SPEC` corre todas las especificaciones en una sola base
    temporal. `native-driver.ts` recuerda, en memoria del proceso de Playwright (un solo worker), la terminal que
    configuró y la reutiliza en las especificaciones siguientes (sin volver a sembrar el catálogo); una base
    configurada por otro proceso se sigue rechazando.

### CSS añadido (`styles.css`, bloque «Etapa 8: Inventarios»)

Solo tokens existentes de ambos temas: tarjetas de cifras (`inventory-figures`, `inventory-figure`, variante de
advertencia), estado de la función, listas de presentaciones, almacenes y estaciones, etiquetas (`tag-warning`),
celdas numéricas alineadas a la derecha, fila negativa resaltada (`inventory-row-negative`, `inventory-negative-text`
y `inventory-negative-notice`), total del inventario, selector de insumo, renglones de la existencia inicial,
parámetros de configuración, la apariencia de los `<select>` de la sección y espaciado sin márgenes dobles dentro de
`form-stack`. Las tablas se desplazan dentro de su contenedor; la página no se desborda a 1280 × 720 ni a
1920 × 1080.

### Limitaciones

- La explicación del resumen describe el comportamiento final del control de inventario; en la fase A todavía no
  hay consumo (llega en la fase B).
- La fecha de negocio de la existencia inicial no se captura; la búsqueda de insumos consulta a Go en cada tecla,
  como el Catálogo.
- «Existencias negativas» del resumen cuenta pares insumo-almacén (Go), mientras que el filtro «Negativas» sin almacén
  usa la existencia total del insumo: con existencias de signo distinto en dos almacenes las cifras pueden diferir.
- Sin verificar en la ventana nativa (WKWebView), con teclado virtual ni en pantalla táctil física.

### Pruebas de la fase A (interfaz)

- **Vitest** (`pages/inventory/inventory.test.tsx` e `inventoryStock.test.tsx`, con `inventoryTestData.ts` y el
  transporte nativo simulado): la pantalla solo aparece con `inventory.read` (lista `screens` y el menú principal de
  `App`); pestañas según permisos; resumen con la explicación, «Administrar funciones» sin segundo interruptor, cifras
  sin costos ni valuación y la de negativas abriendo Existencias filtrada; alta de un insumo (cuerpo exacto, costo y
  existencia mínima como cadenas, carácter inválido descartado) seguida de una presentación capturada en kilogramos
  para un insumo en gramos (Go devuelve 2,500 g); costo de referencia oculto y no enviado sin `costs_visible`
  (aunque haya `inventory.costs.manage`) y `kind` conservado; confirmación al desactivar un insumo y al salir con
  cambios; almacén predeterminado marcado y sin desactivar, asignación de estación con su revisión y confirmación de
  peligro al desactivar un almacén; existencias con la negativa resaltada, costos y valuación con permiso y ocultos
  sin él, filtros de almacén y situación en la consulta, y recarga con eventos de inventario pero no con los de otras
  áreas; existencia inicial con dos renglones (presentación con costo de referencia y otra unidad con costo capturado)
  enviados tal como se capturaron, la confirmación que nombra el costo de referencia, cancelarla sin enviar nada y el
  resultado con advertencias; movimientos sin identificadores, con y sin costos, y todos los filtros en la consulta;
  configuración con `REVISION_CONFLICT` que recarga la versión vigente y lo explica; formato de cantidades; ningún
  `type="number"` en el DOM de las pestañas ni en el código de la sección. En `hooks/useRealtime.test.tsx`, el evento
  `adel-inventory-change` solo con eventos `inventory.*` y ante un reinicio del cursor.
- **Go**: `TestHomeScreenFallsBackWhenPermissionIsMissing` admite `inventory` con `inventory.read` y lo rechaza sin él.
- **Playwright** (`desktop/e2e/inventory.spec.ts`, mismo transporte que `restaurant.spec.ts`): abre Inventarios,
  comprueba las pestañas y el resumen, crea «Harina de prueba» en kilogramos con costo de referencia y la presentación
  «Costal 25 kg» capturada en gramos (Go la muestra como 25 kg), registra la existencia inicial de 2 costales a $450
  (50 kg y $900.00), la comprueba en Existencias (desglose, costo promedio $18.00 / kg, valor y total) y en Movimientos
  (+50 kg, origen, motivo, responsable y valor), vuelve al resumen y consulta a Go la existencia y el movimiento. En
  cada pestaña comprueba que la página no se desborde a 1280 × 720 ni a 1920 × 1080 en claro y oscuro, y guarda
  capturas a 1280 × 720.

### Verificación de la fase A (interfaz)

En la copia aislada del scratchpad, con el clúster temporal del puerto 55499 y Go temporal en 8183 (nunca la
instalación real). La copia incluía el trabajo en curso del servidor de la fase B que había en el árbol al
sincronizar (la 0011 se aplicó en las bases temporales del navegador).

- `prettier --check`, `gofmt -l`, `cargo fmt --check`, `sync-metadata --check`, `go vet` y
  `go vet -tags=integration`: aprobados en la última sincronización (en una anterior, `go vet -tags=integration`
  falló solo en una prueba en curso de la fase B, `internal/operations/inventory_recipes_test.go`, ajena a esta
  interfaz).
- `go test -race ./...` (unitarias): todos los paquetes `ok`. Integración de `internal/administration`
  (`go test -race -tags=integration`): `ok` (53.8 s).
- Vitest: 41 archivos y 337 pruebas aprobadas (19 nuevas de Inventarios y 1 de `useRealtime`); `tsc --noEmit` y
  `npm run build` aprobados.
- `cargo test --locked --offline`: 17 aprobadas, 1 ignorada (la misma de antes).
- Playwright `inventory.spec.ts`: Chromium aprobado (20.8 s) y WebKit aprobado (20.2 s). Recorrido completo sin
  `SPEC` (`inventory.spec.ts` y `restaurant.spec.ts` en la misma base temporal): Chromium 2 aprobadas (20.7 s y
  53.6 s) y WebKit 2 aprobadas (20.2 s y 53.7 s). `restaurant.spec.ts` solo, en una base nueva: Chromium aprobado
  (54.4 s).

## Fase B · backend de recetas y consumo (B1)

Recetas versionadas, subrecetas, porciones habituales, solicitudes de consumo por liberación a cocina, trabajador de
consumo de Go, ajustes por modificación, clasificación de cancelaciones con reconciliador, elaboraciones de subrecetas
almacenables, diagnóstico del consumo, rutas `inventory/*`, puente Tauri y `desktop/src/services/inventoryRecipes.ts`.
Las indicaciones rápidas con efecto (cambio de `catalog_category_quick_notes`) y la interfaz de Recetas y cancelaciones
son de otras partes de la fase B. La 0011 no se aplicó a la instalación real.

### Migración 0011

Transaccional, sin tocar 0001–0010 (no hizo falta ningún ALTER de la fase A) ni filas existentes, **sin siembras** y con
84 restricciones de nombre explícito; los CHECK con columnas opcionales exigen `IS NOT NULL` en la rama que valida (la
prueba de actualización encontró uno que aceptaba una preparación sin modalidad y se corrigió antes de cualquier
aplicación).

| Estructura                               | Contenido                                                                                                                                                                                                                                                                                                                             |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `inventory_ingredient_portions`          | Porción habitual por ingrediente del catálogo, historial de **solo inserción** (`seq`, `effective_from`): insumo, cantidad y unidad capturadas y `base_quantity` convertida al guardar. Una fila sin insumo retira la porción.                                                                                                        |
| `inventory_recipes`                      | Identidad: `owner_kind` `product` (con `variant_id` opcional, llave compuesta a la variante del producto), `complement` o `preparation` (`preparation_mode` `immediate`/`stored` + `preparation_item_id`). Únicos parciales por destino; nombre único entre preparaciones. `name`, `active` y `revision` reflejan la versión vigente. |
| `inventory_recipe_versions`              | Versiones **inmutables** con `active` (desactivar crea una versión), `yield_quantity`/`yield_unit_id` y `yield_base_quantity`, `production_waste_percent` (0 ≤ x < 100), `effective_from` (hora del guardado), notas, autor y `operation_id`.                                                                                         |
| `inventory_recipe_lines`                 | Renglones inmutables `item`, `preparation` o `ingredient` (este último con insumo **o** preparación), cantidad y unidad capturadas, `base_quantity`, `takeaway_only` (no en renglones de ingrediente). Un renglón por ingrediente y versión.                                                                                          |
| `inventory_consumption_requests`         | Solicitudes `release`/`amend`/`cancel` por unidad comercial: versión, `released_at` de la liberación canónica, `occurred_at`, `service_mode` congelado, `state` `pending`/`processed`/`failed`, intentos, `last_error` genérico, `next_attempt_at`, `diagnostics` (arreglo JSON). `UNIQUE(kind, restaurant_unit_id, source_id)`.      |
| `inventory_unit_consumptions`            | Estado por unidad consumida (`consumed`/`reversed`, última versión y solicitud, `released_at`, modalidad).                                                                                                                                                                                                                            |
| `inventory_cancellation_classifications` | Una por unidad cancelada con consumo: `cancelled_at`, `operating_date`, `deadline_at`, `state` `pending`/`not_prepared`/`prepared_waste`, `resolution` `manual`/`automatic`/`correction`, `classified_by`, notas, revisión. CHECK: la automática solo como «No preparado» y sin usuario.                                              |
| `inventory_productions`                  | Elaboraciones **inmutables**: versión, insumo, almacén, lotes, esperado, producido, merma, valor consumido (confidencial), actor, `operation_id` único y movimiento de entrada.                                                                                                                                                       |
| Disparadores                             | `app.inventory_reject_mutation()` (de la 0010) en porciones, versiones, renglones y elaboraciones (UPDATE, DELETE, TRUNCATE).                                                                                                                                                                                                         |
| Permisos                                 | `inventory.recipes.manage`, `inventory.subrecipes.manage`, `inventory.production.record`, `inventory.waste.record`, solo al administrador (14 permisos de inventario en total).                                                                                                                                                       |

### Recetas, subrecetas y porciones

- **Tipos**: producto (la receta de la variante prevalece; si falta o está inactiva, la del producto), complemento y
  preparación inmediata (se descompone en sus insumos) o almacenable (produce un insumo de tipo `prepared`; al venderse se
  consume ese insumo y nunca sus ingredientes). El destino, la modalidad y el insumo producido se fijan al crear
  (`RECIPE_TARGET_IMMUTABLE`); producto, variante y complemento deben existir en el catálogo.
- **Versionado**: cada guardado (también desactivar) crea una versión inmutable con `effective_from`; el `PUT` exige la
  `revision` (= versión vigente). El trabajador resuelve con la versión vigente **a la fecha de la liberación**.
- **Rendimiento**: en productos y complementos se expresa en porciones (1 por omisión) y cada unidad consume
  renglones ÷ rendimiento. En preparaciones, la cantidad de un lote; el rendimiento efectivo es
  `rendimiento × (1 − merma/100)`. La unidad base de una preparación es la de su insumo preparado (almacenable) o la de
  equivalencia 1 de su dimensión (inmediata: g, ml, pza o porción); la dimensión del rendimiento no cambia entre
  versiones (`YIELD_DIMENSION_LOCKED`).
- **Unidades**: cada renglón se captura en cualquier unidad de la dimensión de su destino y se guarda también en la
  unidad base; cambiar después la equivalencia de una unidad propia no altera recetas ni consumos. Un insumo usado en
  recetas o porciones ya no cambia de unidad base (`BASE_UNIT_IN_RECIPES`) y el insumo que produce una almacenable no
  deja de ser «Preparación almacenable» (`ITEM_PRODUCED_BY_RECIPE`).
- **Ciclos**: se rechazan (`RECIPE_CYCLE`) con un recorrido sobre las versiones vigentes de las preparaciones, cuyas
  aristas son las preparaciones que usan y la almacenable que produce cada insumo que consumen; también una preparación
  que se usa a sí misma o una almacenable que consume su propio insumo. La resolución corta además en 8 niveles.
- **Preparación en uso**: no se desactiva si la usa la versión vigente de una receta activa (`PREPARATION_IN_USE`).
- **Porción habitual** (`inventory.recipes.manage`): cantidad de un insumo por porción del ingrediente del catálogo;
  un renglón de ingrediente de la receta del producto la sobrescribe para ese producto. Repetir la vigente no agrega
  historial.
- **Costo teórico** (solo `inventory.costs.read`): costo vigente de cada insumo (V/Q con existencia y valor positivos;
  si no, último costo válido; si no, costo de referencia; si no, «pendiente» y no suma); preparaciones inmediatas
  recursivas por rendimiento efectivo; almacenables al costo vigente de su insumo. En productos: renglones fijos ÷
  rendimiento + ingredientes incluidos del catálogo × su porción; los «solo para llevar» aparte. Opciones de grupo y
  componentes de combo tienen sus propias recetas y no se suman.
- **Destinos de receta**: productos, variantes y complementos con estado `with_recipe`, `without_recipe` o
  `no_preparation` (sin receta y ruta «Sin preparación») y `configured` (consumiría algo por porciones o vínculos).

### Consumo por liberación

- **Al liberar**: `kitchen/releases.go prepareRelease`, justo después del bucle que inserta `kitchen_unit_releases` y
  antes de los tickets, llama a `inventory.RecordReleases(ctx, tx, releaseID, units, contextSnapshot.Mode)`: con
  `inventory_control` activada, **un solo** `INSERT … SELECT … FROM kitchen_unit_releases … ON CONFLICT DO NOTHING`
  (versión y `released_at` de la fila canónica). Sin lock 7820148308 ni lectura de recetas. Cubre los cuatro caminos
  (confirmación, planificador, liberación manual y reintento de un cambio pendiente).
- **Al cambiar**: `kitchen.Integrate` llama a `inventory.RecordChange(ctx, tx, eventID, kind, payload)` para
  `item.modified`/`item.cancelled` **antes** de procesar cocina: `amend` para `modified_unit_ids` (versión vigente de la
  unidad) y `cancel` para `cancelled_unit_ids` (reducciones) o `unit_ids` (cancelación), solo para unidades con fila
  canónica **y** solicitud de liberación (decisión 2).
- **Trabajador** `inventory.Worker` (en `main.go`, mismo contexto cancelable y apagado ordenado que cocina): sondeo de
  1 s sin LISTEN/NOTIFY; cada vuelta procesa lo pendiente en lotes de 50 hasta vaciarlo y luego reconcilia. Cada
  solicitud en su propia transacción con 7820148308 exclusivo, tomada con `FOR UPDATE SKIP LOCKED` en orden de `seq` y
  solo si su unidad no tiene otra anterior sin procesar. Idempotente por estado, por `inventory_unit_consumptions` y por
  `entry_key` (`consumption:<solicitud>:<insumo>:<almacén>`, `amend:…`, `classification:<id>:<revisión>:…`). Un fallo
  revierte esa solicitud y, en otra transacción, la marca `failed` con el mensaje genérico «No se pudo calcular el
  consumo; se reintentará automáticamente.», `attempts+1` y espera 2, 4, 8 … 256 s (tope 300 s); no detiene las demás
  unidades. `ProcessPending(ctx, pool, limit)` y `ReconcileCancellations(ctx, pool, now)` son las funciones que llaman
  las pruebas. Eventos `inventory.stock.changed`, `inventory.movements.recorded` y `inventory.alerts.changed` en la
  transacción del trabajador.
- **Resolución** (`inventory/resolution.go`, función pura con 13 pruebas unitarias), a `released_at`:
  1. producto o variante: renglones `item` × 1/rendimiento; `preparation` inmediata descompuesta por rendimiento
     efectivo (recursiva); almacenable o insumo preparado consumido como insumo;
  2. ingredientes de `quote_line.ingredients[]`: `requested ×` porción (renglón de ingrediente de la receta o porción
     habitual vigente); sin porción, nada (diagnóstico `ingredient_without_portion`);
  3. opciones de `groups[].options[]`: `requested × factor de la porción del pricing_snapshot (1 si no tiene) ×` receta
     del complemento; si no, configuración estándar del producto vinculado (renglones de su receta + ingredientes
     incluidos × porción); si no, porción habitual del ingrediente vinculado; si no, nada
     (`complement_without_configuration`). El precio no interviene;
  4. componentes de combo × su cantidad, recursivos, con su propia personalización (`root/0`, `root/1`…);
  5. `takeaway_only` solo con modalidad `takeaway` (incluye ventanilla);
  6. almacén por parte: estaciones de `kitchen_tasks.instructions.parts[].path` de la unidad (también tareas
     canceladas); con varias, la primera por nombre con almacén asignado a esa fecha; sin asignación, «Almacén general»;
     una parte sin estación hereda la de su padre. El motivo del movimiento lleva «Liberación a cocina · Estación: …»;
  7. resultado redondeado a 6 decimales por (insumo, almacén); un movimiento `consumption` por par con
     `occurred_at = released_at`. Sin configuración no se inventa nada: queda `no_configuration` en `diagnostics`.
- **Modificación** (`amend`): objetivo = consumo de la versión nueva a la fecha de liberación de la unidad; se escribe la
  diferencia con el neto registrado por (insumo, almacén): `consumption` (falta) o `consumption_reversal` al valor
  proporcional registrado (exacto si revierte todo); `occurred_at` = hora del cambio. Sin consumo previo: sin efecto.
- **Cancelación** (`cancel`): clasificación `pending` con el día operativo de la hora de cancelación; si la unidad no
  consumió nada, no se crea (`nothing_consumed`).
- **Clasificación**: «No preparado» revierte el neto exacto; «Preparado / merma» conserva el consumo (si antes se revirtió,
  vuelve a consumir lo revertido, a costo vigente). Hasta el corte, `inventory.waste.record` (resolución `manual`, también
  para cambiarla); después del corte o tras la automática, solo `inventory.adjustments.authorize` (resolución
  `correction`). Repetir el estado vigente no escribe nada. Los movimientos llevan la hora de la cancelación y origen
  `cancellation`.
- **Reconciliador**: «No preparado» automático a toda pendiente con `deadline_at ≤ now`, una por transacción con el
  lock, `UPDATE … WHERE revision` (idempotente), auditoría `inventory.cancellation_reconciled` con `actor_kind='process'`
  y `actor_username='inventory_worker'`; al iniciar Go procesa los días que estuvo apagado.
- **Día operativo** (`operating_day.go`): un instante pertenece al día cuya fecha local tiene la hora de corte en o antes
  de él (zona del establecimiento); vence en el corte del día siguiente. Si la hora de corte no existe (adelanto de
  horario), el corte es el primer instante tras el salto; si ocurre dos veces (atraso), la primera.
- **Elaboraciones** (`inventory.production.record`): consumen la versión vigente × lotes (`production_out`), producen el
  insumo preparado con el valor total consumido (`production_in`, entrada a valor exacto) y guardan esperado, producido
  (por omisión, lo esperado) y merma (`lotes × rendimiento − producido`, mínimo 0). Idempotentes; existencias negativas
  con aviso `NEGATIVE_STOCK`.
- **Devoluciones financieras**: `payments` no toca solicitudes ni movimientos (probado).

### Rutas y permisos

Todas exigen `inventory.read`; contrato completo en la sección «Inventario · fase B (esquema 11)» de `API_CONTRACTS.md`
(texto en `docs8B1/api_contracts.md`) y tipos en `desktop/src/services/inventoryRecipes.ts`.

| Ruta                                                                                                                                | Permiso adicional                                                                                |
| ----------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| `GET recipes`, `GET recipes/{id}`, `GET recipe-versions/{id}`, `GET recipe-targets`, `GET ingredient-portions`, `GET cancellations` | — (costos solo con `inventory.costs.read`)                                                       |
| `POST recipes`, `PUT recipes/{id}`                                                                                                  | `inventory.recipes.manage` (producto, complemento) o `inventory.subrecipes.manage` (preparación) |
| `PUT ingredient-portions/{ingrediente}`                                                                                             | `inventory.recipes.manage`                                                                       |
| `POST cancellations/{id}/classify`                                                                                                  | `inventory.waste.record` hasta el corte; `inventory.adjustments.authorize` después               |
| `POST productions`                                                                                                                  | `inventory.production.record`                                                                    |
| `GET productions`                                                                                                                   | `inventory.production.record` o `inventory.movements.read`                                       |
| `GET consumption-requests`, `GET consumption/{unidad}`                                                                              | `inventory.movements.read`                                                                       |

Puente Tauri: 14 operaciones nuevas en `api.rs` (8 fijas y 6 con un identificador) con la prueba
`inventory_recipe_and_consumption_routes_are_closed`; lista blanca de consulta ampliada con `owner_kind`, `product_id`
y `configured`. `Operation` de `desktop/src/services/api.ts` ampliado con esas 14 operaciones (cambio mínimo al final).

### Decisiones y desviaciones (fase B, backend)

1. **Versión de receta por su identificador**: `GET inventory/recipe-versions/{version_id}` en lugar de
   `recipes/{id}/versions/{versión}`, porque el puente nativo admite un solo identificador por ruta.
2. `RecordChange` registra `amend`/`cancel` solo si la unidad tiene fila canónica **y** solicitud de liberación: una
   unidad liberada antes de activar (o con la función apagada al liberarse) no deja solicitudes vacías. El trabajador,
   además, ignora una solicitud sin consumo previo de la unidad.
3. Destino, modalidad e insumo de una preparación inmutables; dimensión del rendimiento de una preparación inmutable;
   unidad base de una preparación = la de su insumo (almacenable) o la de equivalencia 1 de su dimensión (inmediata).
4. Rendimiento en recetas de producto y complemento (en porciones; 1 por omisión). La merma de elaboración solo existe
   en preparaciones y define el rendimiento efectivo (descomposición inmediata y esperado de una elaboración); la merma
   registrada de una elaboración es lotes × rendimiento − producido y el costo del insumo preparado incluye esa merma
   (entra con todo el valor consumido).
5. Un renglón de ingrediente puede apuntar a un insumo o a una preparación (extensión natural: «Salsa de la casa» como
   ingrediente del catálogo); la porción habitual solo apunta a insumos.
6. Complemento ligado a un producto sin receta propia: se consume la configuración **estándar** del producto (renglones
   de su receta e ingredientes incluidos × porción); sus grupos y componentes no se recorren.
7. Una receta de variante inactiva cede a la del producto; una receta inactiva cuenta como «sin receta».
8. Horas de negocio: liberación → `released_at`; modificación → hora del cambio; clasificación (manual, automática o
   corrección) → hora de la cancelación (es cuando el insumo dejó o no de usarse, y así un conteo posterior la ve).
9. El vencimiento de una cancelación se calcula y guarda al registrarla (zona y corte vigentes); cambiar después la hora
   de corte no mueve los vencimientos ya registrados. La resuelta automáticamente cuenta como vencida.
10. «Preparado / merma» después de un «No preparado» vuelve a consumir a costo vigente (el valor puede diferir del
    original: nada histórico se reescribe).
11. Una solicitud fallida se reintenta indefinidamente con espera creciente (tope 300 s) y bloquea solo las posteriores
    de su unidad; queda visible en `GET consumption-requests` y publica `inventory.alerts.changed` la primera vez.
12. Una unidad sin consumo neto al cancelarse no genera clasificación (`nothing_consumed`).
13. Los productos «Sin preparación» no tienen tareas: consumen del almacén predeterminado (o del de la estación del
    combo, si son componente).
14. La consulta del libro se amplió con los orígenes `consumption`, `amendment`, `cancellation` y `production`.
15. Prueba de la fase A `checkLedgerConsistency`: el conteo fijo de 4 valuaciones se generalizó a «una valuación por
    insumo con movimientos» porque la fase B agrega insumos; el resto de la verificación no cambió.

### Mediciones (§32.17)

Prueba `inventory_b_measure_release` (opt-in con `ADEL_INVENTORY_MEASURE=1`): 200 confirmaciones con envío a cocina con
la función activada y 200 desactivada, en bloques alternados de 50 tras 5 de calentamiento por caso; se mide solo el
`POST …/confirm` (una hamburguesa por cuenta nueva) en la copia aislada contra el clúster temporal 55499.

| Ejecución                   | Desactivada (media / p95 / máx.) | Activada (media / p95 / máx.) | Trabajador                             |
| --------------------------- | -------------------------------- | ----------------------------- | -------------------------------------- |
| `go test -tags=integration` | 6.79 / 7.83 / 9.73 ms            | 7.01 / 7.74 / 8.11 ms         | 205 solicitudes en 1.23 s (6.0 ms c/u) |
| `go test -race` (final)     | 17.58 / 20.70 / 21.76 ms         | 18.03 / 21.22 / 23.04 ms      | 205 en 1.35 s (6.6 ms c/u)             |

La liberación con la función activada solo agrega una lectura por clave primaria y un `INSERT … SELECT` (≈ 0.2 ms de
media sin el detector de carreras; el p95 queda dentro del ruido). Resolver y escribir el consumo de una unidad cuesta
≈ 6 ms en el trabajador: hacerlo síncrono alargaría el lock 7820148305 unas 30 veces más que el registro de la
solicitud por cada unidad liberada, así que **se conserva el consumo asíncrono**.

### Limitaciones

- Sin interfaz (Recetas, cancelaciones pendientes y diagnóstico se construyen sobre este contrato) y sin alertas (fase D:
  los datos ya existen en `GET cancellations?state=pending`, `GET consumption-requests`, `diagnostics` y
  `GET recipe-targets?state=without_recipe`).
- El costo teórico de un producto no incluye complementos ni componentes de combo; el de un complemento vinculado a un
  producto no recorre los grupos ni componentes de ese producto.
- La clasificación guarda solo su estado vigente; el historial está en la auditoría y en los movimientos compensatorios.
- Las elaboraciones no se anulan por API (una corrección será un ajuste de la fase D).
- El trabajador no audita cada consumo en `audit_events` (los movimientos registran `process='inventory_worker'`); sí la
  reconciliación automática.
- Las mediciones son de este equipo y del clúster temporal, sin concurrencia real de terminales.
- Las indicaciones rápidas con efecto no forman parte de este trabajo.

### Pruebas de la fase B (backend)

- **Unitarias** (`internal/inventory`, 41 pruebas en total con las de la fase A):
  - `resolution_test.go` (13): producto normal, «Sin cebolla» → 0, «Con cebolla» → 1, «Cebolla extra» → 2, 3 porciones
    y media porción; observación libre ignorada; ingrediente sin porción sin consumo inventado; producto sin
    configuración; renglón de ingrediente que sobrescribe la porción; complementos sustituidos (filete: pasta y ensalada,
    no arroz), «2 porciones de papas», factor de porción, complemento sin configuración; complemento ligado a producto y
    receta propia que prevalece; «solo para llevar» en salón y para llevar; preparación inmediata con y sin merma,
    almacenable consumida como insumo, rendimiento de 3 porciones redondeado, preparación inactiva y ciclo defensivo;
    receta de variante y su retirada; combo en dos estaciones con almacenes distintos y componente heredado; elección
    de almacén (primera por nombre con asignación, sin asignación, sin estación); límite de profundidad; lotes de una
    elaboración; detección de ciclos (también a través del insumo producido).
  - `operating_day_test.go` (2 pruebas, 14 casos): corte 05:00 en Monterrey (antes, en el corte, un minuto antes, noche,
    medianoche, 04:30 y fin de año) y en Tijuana con el adelanto (8/3/2026: corte a las 05:00 y corte inexistente a las
    02:30) y el atraso (1/11/2026: primera y segunda pasada de la 01:30 y corte a las 05:00).
  - `permissions_test.go`: los 14 permisos de Go están en la 0010 o la 0011 con la misma descripción, solo para el
    administrador.
- **Integración** (`operations/inventory_recipes_test.go` e `inventory_consumption_test.go`, encadenadas al final del
  núcleo de inventario con su propio catálogo, estaciones, almacenes e insumos; 21 subpruebas + la medición):
  - porciones con conversión, revisión, historial, retirada, 403 y solo inserción;
  - recetas: alta con conversión kg → g, permisos (403 sin escribir nada), rechazos (duplicada, producto o variante
    ajenos, dimensión, ingrediente en complemento, «solo para llevar» en ingrediente, sin renglones), versiones 2 y 3,
    revisión obsoleta, destino inmutable, versión 1 conservada, renglones de solo inserción, costo teórico $39.35 +
    $2.00 para llevar con `costs.read` y oculto sin él, eventos y auditoría;
  - subrecetas: salsa inmediata (lote $17.00, $0.034/ml), almacenable que exige insumo preparado, rendimiento efectivo
    con 4 % de merma, ciclos a través del insumo producido, autouso y propio insumo, dimensión bloqueada, preparación en
    uso, costo pendiente; elaboración (2 lotes: 2000 g de arroz, 160 g de tomate, 20 g de chile = $56.80; esperado 4800,
    producido 4750, merma 250), 403, idempotencia y consistencia del libro;
  - destinos de receta (estado, porciones y complementos);
  - función desactivada sin solicitudes ni movimientos; activación; unidad liberada antes de activar modificada y
    cancelada sin efecto; pedido programado antes de activar y liberado después que consume (con bolsa para llevar);
  - borrador y cuenta sin consumo; confirmación que termina con la solicitud pendiente (trabajador detenido); una
    solicitud por unidad; reintento HTTP idempotente; reproceso (vuelta repetida y solicitud devuelta a pendiente) sin
    duplicar; estación en el motivo; queso con saldo 1, 0 y −1 en el libro real (−$5 promedio, exacto y a referencia);
  - personalizaciones (las seis de arriba con diagnóstico del jalapeño), filete en dos estaciones con una solicitud,
    arroz por pasta a $140, dos porciones de papas, medias porciones; combo personalizado en dos estaciones y dos
    almacenes sin duplicar; bolsa en para llevar y ventanilla y no en salón;
  - reimpresión con cambio de impresora, división, cambio de mesa y combinación sin consumo;
  - modificación de partida liberada (solo +20 g de cebolla y la reversión exacta de la lechuga); receta modificada
    entre liberación y procesamiento (consume la anterior; la siguiente, la nueva);
  - reducción de cantidad → cancelación pendiente con su día operativo y vencimiento; «No preparado» con reversión exacta
    (cantidad y valor netos en cero); «Preparado / merma» sin movimientos, repetición sin efecto y cambio a «No
    preparado»; reconciliación antes del corte (nada), después (automática) e idempotente, con auditoría del proceso;
    corrección después del corte (403 con `waste.record`, 200 con `adjustments.authorize` y nuevo consumo); trabajador
    que al iniciar procesa lo pendiente y reconcilia una vencida;
  - subreceta elaborada sin doble descuento (enchiladas: arroz rojo a 200 × 56.80 / 4750 = $2.391579);
  - devolución financiera sin reintegración;
  - falla controlada de una unidad (disparador de prueba): `failed`, mensaje genérico, sin reintento inmediato, la otra
    unidad se procesa, diagnóstico de solicitudes, recuperación sin duplicar; traza de una unidad con y sin costos;
    eventos del trabajador;
  - eventos `inventory.*` sin contenido y consistencia del libro al final.
- **Actualización desde el esquema 9** (`TestStage8UpgradeOfExistingInstallation`, ampliada): la 0010 y la 0011 se aplican
  y no se repiten; 14 permisos nuevos solo del administrador; la 0011 no siembra filas; sus 84 restricciones con nombre;
  CHECK con tablas temporales (recetas por tipo, renglones, porciones, solicitudes, clasificaciones y versiones); versiones,
  renglones y porciones de solo inserción; llave compuesta de la variante.
- **Rust**: `inventory_recipe_and_consumption_routes_are_closed` (14 operaciones, sin identificador o con `../../settings`
  rechazadas, sin borrado de recetas ni escritura de consumo, lista blanca de consulta).

### Verificación de la fase B (backend)

En la copia aislada `scratchpad/repoB1`, con el clúster temporal del puerto 55499 (nunca la instalación real):

- `gofmt -l` (archivos Go propios), `rustfmt --check` de `api.rs` y `prettier --check` de los servicios TS: sin
  diferencias.
- `go vet ./...` y `go vet -tags=integration ./...`: aprobados.
- `go test -race -count=1 ./...` (unitarias): todos los paquetes `ok`, incluido `internal/inventory`.
- Integración (`go test -race -tags=integration`): `internal/database` ok (3.2 s), `internal/administration` ok
  (56.8 s), `internal/operations` ok (165.4 s, con la aceptación completa de las etapas 4–7.1, la fase A, la fase B y la
  medición), `internal/payments` ok (1.3 s) e `internal/catalog` ok (6.5 s).
- `cargo test --locked --offline`: 18 aprobadas (1 ignorada, la misma de antes).
- `tsc --noEmit`: sin errores.

## Fase B · indicaciones rápidas con efecto y captura de complementos (Bq)

Parte de la fase B desarrollada en paralelo con recetas y consumo (B1): migración
`0012_quick_note_effects.sql`, efecto de las indicaciones rápidas en el catálogo, la captura, la
partida y la comanda, y el ajuste de la captura de complementos (§8 del diseño). Detalle funcional en
[CATALOG.md](CATALOG.md) («Indicaciones con efecto · etapa 8») y
[KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md); contratos en [API_CONTRACTS.md](../API_CONTRACTS.md).
Sin permisos, locks, rutas ni operaciones del puente nuevos (las indicaciones siguen con
`catalog.read` y `catalog.categories.manage`). La 0012 no se aplicó a la instalación real.

### Migración 0012

Transaccional (sin `CONCURRENTLY` ni `VACUUM`), sin tocar 0001–0011 ni sembrar datos. Agregar
columnas con un valor predeterminado constante no reescribe la tabla y las indicaciones existentes
quedan como solo instrucción.

```sql
ALTER TABLE app.catalog_category_quick_notes
    ADD COLUMN effect text NOT NULL DEFAULT 'none'
        CONSTRAINT catalog_category_quick_notes_effect_check CHECK (effect IN ('none', 'omit', 'normal', 'extra')),
    ADD COLUMN ingredient_id uuid
        CONSTRAINT catalog_category_quick_notes_ingredient_id_fkey REFERENCES app.catalog_ingredients(id);

ALTER TABLE app.catalog_category_quick_notes
    ADD CONSTRAINT catalog_category_quick_notes_effect_ingredient_check CHECK (
        (effect = 'none' AND ingredient_id IS NULL)
        OR (effect IN ('omit', 'normal', 'extra') AND ingredient_id IS NOT NULL)
    );

CREATE INDEX catalog_category_quick_notes_ingredient_idx
    ON app.catalog_category_quick_notes (ingredient_id)
    WHERE ingredient_id IS NOT NULL;
```

El CHECK de coherencia compara con `IS NULL`/`IS NOT NULL` explícitos en ambas ramas y nunca evalúa
a NULL. La prueba `catalog/quick_note_effects_migration_test.go` parte de un esquema 11 con dos
indicaciones de la 7.1 (una inactiva), aplica la 0012 y comprueba que texto, estado, orden, revisión
y fechas no cambian, que quedan `none` sin ingrediente, las tres restricciones con nombre y el
índice, que PostgreSQL rechaza efecto sin ingrediente, instrucción con ingrediente, efecto
desconocido e ingrediente inexistente y acepta las combinaciones válidas, y que es repetible.

### Contratos

- **Administración** (`POST`/`PATCH catalog/quick-notes`): `effect` (`none|omit|normal|extra`) e
  `ingredient_id`. Un efecto exige un ingrediente existente; `none` no admite ingrediente (400). Sin
  `effect` en el cuerpo, una alta queda `none` y una edición conserva el efecto vigente (clientes de
  la 7.1). Respuesta y `category_quick_notes` de `catalog/lookups`: `effect`, `ingredient_id` e
  `ingredient_name` (nulos sin efecto). Auditoría con el efecto antes y después.
- **Partida**: `customization.quick_notes: [{note_id, text}]` en cada nivel (raíz y componentes de
  combo), `omitempty`. El texto sigue también en `notes`. Forma validada al guardar el borrador
  (hasta 40 por nivel, UUID sin repetir, texto de 1 a 80 caracteres).
- **Desglose** (`quote_line`): `ProductBreakdown.quick_notes: [{note_id, text, effect,
ingredient_id}]`, `omitempty`: la resolución congelada. `pricing_snapshot` añade `QuickNotes` (solo
  las usadas) únicamente si la cotización usa alguna.
- **Cocina**: `instructions.parts[].quick_notes`, `omitempty`.
- Errores 422 en español: `QUICK_NOTE_INVALID` y `QUICK_NOTE_CONFLICT` (detalle abajo).

### Reglas de validación (Go, `catalog/quick_note_effects.go`)

Se aplican al cotizar (catálogo, borrador, cuenta pendiente), al confirmar y al modificar, en cada
nivel, después de calcular los ingredientes:

1. La indicación existe, está activa y pertenece a la categoría del producto (o del producto del
   componente). Si no: 422 `QUICK_NOTE_INVALID`.
2. Con efecto, su ingrediente está en las reglas del producto y el efecto es aplicable (`QuickNoteTarget`):
   `omit` exige incluido > 0 y removible; `normal`, incluido > 0 o agregable con máximo ≥ 1; `extra`,
   agregable con máximo > incluido. Si no: 422 `QUICK_NOTE_INVALID` («no aplica a …»).
3. Dos indicaciones con efecto sobre el mismo ingrediente: 422 `QUICK_NOTE_CONFLICT` («cambian el
   mismo ingrediente (…); elige solo una»). Se comprueba antes de las cantidades, para que el
   conflicto se informe como tal.
4. La cantidad solicitada efectiva del ingrediente (explícita o la incluida) coincide exactamente con
   el efecto: `omit` → 0; `normal` → lo incluido, o 1 si no estaba incluido; `extra` →
   `min(incluido + 1, máximo)`, igual que el botón «Extra». Si no: 422 `QUICK_NOTE_INVALID`.

Las indicaciones no tienen cargo propio: el precio (y el consumo de inventario de B1) sale solo de
`ingredients`, así que «Queso extra» cobra lo mismo que el botón «Extra queso» y nunca se acumulan
multiplicadores. El desglose guarda la indicación con el texto vigente del catálogo; el `text` de la
solicitud es informativo.

**Camino «Preserve»** (cambios solo de observaciones o reducciones de cantidad): `normalizeRecipe`
trata `quick_notes` como `notes` (preparación, no receta). El desglose se copia completo
(`cloneQuoteLine`) y `catalog.PreserveQuickNotes` conserva las indicaciones ya resueltas en el mismo
nivel tal como se congelaron (aunque la indicación cambie después) y valida las nuevas con el
catálogo vigente, las reglas del producto del `pricing_snapshot` del pedido anterior y las cantidades
congeladas del desglose (que no cambian, porque la receta es la misma).

### Captura

- `quickNotesFor` devuelve las indicaciones completas; `quickNoteRules.ts` (nuevo) decide cuáles se
  muestran (`applicableQuickNotes`: todas las sin efecto y las con efecto aplicables a las reglas del
  producto, con la misma regla de Go) y qué selección enviar.
- Al tocar una con efecto se aplica **la misma selección estructurada** que el personalizador
  (`withIngredient` con la cantidad del efecto, o la configuración original para «Con cebolla» en un
  producto que ya la lleva) y se guarda `{note_id, text}`; su texto se agrega a `notes` sin repetirse.
  Desmarcarla quita su identificador, su texto y su selección del ingrediente.
- Dos indicaciones con efecto sobre el mismo ingrediente son excluyentes: tocar una quita la otra.
- Cambiar ese ingrediente en el personalizador (botones «Sin …» / «Extra …») quita la indicación con
  efecto y su texto (`reconcileQuickNotes`); las demás se conservan.
- Las con efecto que no aplican no se muestran, y su texto tampoco aparece como atajo de la cuenta.
  Los atajos de indicaciones ya usadas en la cuenta siguen sin duplicarse. Las indicaciones sin efecto
  se marcan por texto como en la 7.1 (también en partidas anteriores sin identificador) y ahora
  guardan además su identificador.
- Las indicaciones para cocina del producto principal se dibujan dentro de `TouchCustomizer` (antes
  en `CustomizationPanel`), en el mismo lugar, porque necesitan la configuración del producto; cada
  componente de combo sigue con las suyas.
- El resumen de la partida en la cuenta (`describeBreakdown`/`describeSelection`) muestra la
  indicación con efecto una sola vez, como la comanda.
- `normalizeSelection` (`services/restaurant.ts`) conserva `quick_notes` al abrir una partida para
  editarla; sin indicaciones se omite la clave (la partida queda idéntica).

### Comanda

`routing.go` copia las indicaciones resueltas a cada `Part`. `template.go` omite la línea
estructurada de un ingrediente («SIN Cebolla», «Queso: 1 (incluido 0)») cuando una indicación con
efecto sobre ese ingrediente está entre las indicaciones de la nota impresa: la de la partida para la
parte principal y la del componente para cada componente de combo, también en ANTES/AHORA. Si la
comanda no muestra notas o la nota ya no contiene el texto, la línea estructurada se conserva. Sin
indicaciones con efecto la comanda es idéntica a la de la 7.1 (prueba con el texto exacto obtenido de
la plantilla anterior).

### Complementos en la captura (§8 del diseño)

Se ajustó `GroupBlock` sin rehacer el panel:

- Grupos de varias porciones: la opción es una tarjeta con su botón principal («Agregar una porción
  de …»: **un toque suma una porción**), la cantidad grande dentro de la tarjeta (24 px) y un botón
  **−** de 44 × 44 px en la propia tarjeta, visible con cantidad > 0. Se quitó el `− cantidad +` que
  aparecía debajo y el toque ya no quita la opción completa. Sustituir es de dos toques (− en arroz,
  toque en pasta).
- Los grupos de elección única (máximo 1) siguen reemplazando con un toque, como en la 6.1.
- Precio en la tarjeta: valor de referencia en grupos por equivalencia («$30.00», solo si es mayor que
  cero) más el extra explícito si existe («+$10.00»); en grupos por cantidad, el extra explícito o, sin
  él, el precio del primer tramo. Con `money.ts`, sin aritmética.
- Encabezado «2 de 2» (porciones elegidas de las incluidas; «1 elegida» si el producto no incluye
  ninguna) junto a la regla de mínimo y máximo de siempre.
- Cambio de precio del grupo en vivo con el desglose de Go: «+$10.00», «Sin cambio» o, con
  equivalencia y extras a la vez, los dos importes; «Calculando…» mientras Go recalcula.
- Intentar pasar del máximo del grupo o de la opción muestra un aviso en el propio grupo
  («Guarniciones admite hasta 2 porciones: toca «−» en otra opción para cambiarla.») en lugar de
  desactivar la tarjeta sin explicación; el aviso se retira con el siguiente cambio. Mínimos y máximos
  los sigue validando Go.

### Decisiones y desviaciones menores

1. **Errores 422**: los rechazos de indicaciones usan 422 (`QUICK_NOTE_INVALID`, `QUICK_NOTE_CONFLICT`),
   distintos del 400 `CATALOG_VALIDATION` de forma, para que la captura los distinga.
2. **Texto de la solicitud informativo**: la resolución usa el texto vigente del catálogo, no el de
   la solicitud; si una indicación se renombra con un borrador abierto, la comanda imprime la nota del
   borrador y, como su texto ya no coincide, conserva también la línea estructurada (nunca se pierde
   el cambio). No se exige que el texto coincida para no bloquear borradores.
3. **Indicaciones sin efecto con identificador**: también se guardan en `quick_notes` (la
   especificación pide conservar la identidad de la indicación elegida). Se siguen marcando por texto,
   así que una partida antigua con «Bien cocida» escrita aparece marcada.
4. **«Con cebolla» incluida** se aplica como la configuración original (sin cantidad explícita): la
   solicitud no cambia de receta y una modificación que solo agrega esa indicación sigue el camino
   «Preserve».
5. **Preserve** valida las indicaciones nuevas con el snapshot del pedido anterior y el catálogo
   vigente de la indicación; una nueva con efecto que no coincide con la receta conservada se rechaza
   con 422 (no se recotiza en silencio). Al recotizar (otros cambios) todas se validan con el catálogo
   vigente.
6. **Copia del desglose en Preserve**: `cloneQuoteLine` evita que copiar observaciones modificara
   también los componentes de la versión anterior (compartían punteros), de modo que el evento
   `item.modified` conserva en `quote_line` las notas anteriores de los componentes de combo.
7. **Sin aviso emergente al pasar del máximo**: el aviso es texto en el grupo (`role="status"`), no
   `notifyWarning`, porque describe el estado del grupo junto a sus tarjetas.
8. **Precio de los grupos por cantidad**: sin extra explícito se muestra el precio del primer tramo
   (lo que cuesta la primera porción adicional).

### Limitaciones

- Las indicaciones con efecto actúan solo sobre los ingredientes del producto
  (`catalog_product_ingredients`), no sobre opciones de grupo cuyo complemento esté ligado a un
  ingrediente.
- La pantalla de preparación (KDS) no lista ingredientes, así que no necesitó cambios; los documentos
  históricos conservan su texto.
- Un borrador con una indicación que después se desactiva o se cambia de efecto se rechaza al cotizar
  (422) hasta quitarla; la captura la muestra como indicación escrita para poder quitarla.
- Sin prueba en la ventana nativa ni en pantalla táctil física; ninguna comanda se imprimió en papel.

### Pruebas de la fase Bq

- **Go unitarias**: `catalog/quick_note_effects_test.go` (cantidades de `QuickNoteTarget` por efecto y
  regla; aceptadas y rechazadas con cantidades exactas de `omit`/`normal`/`extra`, efectos no
  aplicables, conflicto de dos efectos, inactiva, otra categoría, inexistente y repetida; «Cebolla
  extra», «Queso extra» y «Chile extra» cobran lo mismo que el botón; desglose congelado en la raíz y
  en un componente de combo; Preserve conserva la resolución congelada y rechaza una nueva
  incompatible); `kitchen/quick_notes_template_test.go` (sin efecto idéntica a la plantilla anterior
  con el texto exacto, una sola impresión en la raíz y en un componente, línea estructurada conservada
  sin notas impresas o sin el texto); `operations/quick_notes_fingerprint_test.go` (huella de un
  borrador sin indicaciones igual a la calculada con el código anterior, sin `quick_notes` en la
  serialización; las indicaciones son preparación en `sameRecipe`; la copia del desglose no comparte
  componentes).
- **Integración PostgreSQL**: `catalog/quick_note_effects_migration_test.go` (0012 sobre esquema 11) y
  `operations/quick_note_effects_test.go`, encadenada al final de la aceptación
  (`8Bq_indicaciones_con_efecto_administradas_con_permiso`: 403 sin permiso, efecto sin ingrediente,
  efecto desconocido, instrucción con ingrediente, ingrediente inexistente y texto con coma
  rechazados sin registros, auditoría, edición sin efecto que lo conserva y lookups con efecto e
  ingrediente; `8Bq_confirmacion_con_indicaciones_con_efecto`: precio igual al del botón Extra, 422 de
  cantidad y de conflicto, `quote_line` congelado y comanda sin la línea duplicada;
  `8Bq_editar_la_indicacion_no_cambia_la_partida_confirmada`; `8Bq_partida_antigua_y_borrador_sin_quick_notes`:
  borrador, solicitud, desglose y snapshot sin `quick_notes`, huella estable y comanda de la 7.1;
  `8Bq_modificacion_preserve_conserva_las_indicaciones`: reducción con observaciones e indicación
  nueva por el camino Preserve con las indicaciones congeladas y 422 de una incompatible;
  `8Bq_componente_de_combo_con_indicacion`).
- **Vitest**: `pages/restaurant/capture/quickNotesCapture.test.tsx` (cantidades por efecto; selección
  estructurada, identificador y texto; exclusión mutua; «Con cebolla» sin cambio; desmarcar; quitar al
  cambiar el ingrediente; ocultas en un producto sin el ingrediente y sin atajo; resumen sin duplicar;
  complementos: toque suma, − en la tarjeta, «2 de 2», «3 de 2», aviso de máximo, sustitución en dos
  toques, «Calculando…», «+$10.00» y «Sin cambio»); `capture.test.tsx` (las indicaciones sin efecto
  se guardan también con su identificador); `pages/catalog/quickNotes.test.tsx` (efecto con botones,
  ingrediente exigido, sin campos de gramos, resumen del efecto en el listado, edición que vuelve a
  solo instrucción).
- **Playwright** (`e2e/takeaway-quick-notes.spec.ts`, nuevo; su catálogo va en una categoría propia,
  «Platos fuertes», creada por la API, y el nombre lo ordena después de `restaurant.spec.ts`, que
  espera la cuenta #1 y dos productos en «Platillos»): pedido para llevar con un filete de $140 (arroz
  y ensalada incluidos): «2 de 2», «Sin cambio», precio en la tarjeta, aviso de máximo, − de 44 px,
  sustitución en dos toques (arroz → papas, «+$5.00» y $145.00 de Go), «Queso extra» marca «Extra
  queso» del personalizador y cobra $12.50, «Sin cebolla» marca el ingrediente, resumen sin
  duplicados y borrador con identificadores; sin desbordamiento del panel ni de la página a 1280 × 720
  y 1920 × 1080 en ambos temas.

### Verificación de la fase Bq

Ejecutada sobre una copia aislada con el `HEAD` de `etapa-8` (`6e063dc`, con la 0011 de B1) más solo
los cambios de Bq (los cambios en curso de otros agentes, como la 0013, se excluyeron de la copia),
con el clúster temporal 55499 y Go temporal en 8185 (vista previa 1425):

- Estáticas: Prettier (`--check`), gofmt, `cargo fmt --check`, `sync-metadata --check`, `go vet` y
  `go vet -tags=integration` aprobados.
- Go unitarias con `-race`: `ok` en todos los paquetes (`catalog`, `kitchen` y `operations` incluidos).
- Vitest: 42 archivos y 345 pruebas aprobadas; `tsc --noEmit` y `npm run build` sin errores; cargo: 18
  aprobadas, 1 ignorada.
- Integración PostgreSQL: `catalog` (7,0 s, con la prueba de la 0012 sobre esquema 11), `operations`
  (157,3 s, con las seis subpruebas `8Bq_`) y `database` (3,0 s) en `ok`; la prueba de impresora física
  quedó omitida porque es opcional.
- Playwright: `inventory.spec.ts`, `restaurant.spec.ts` y `takeaway-quick-notes.spec.ts` aprobadas en
  Chromium (3 en 1,5 min) y en WebKit (3 en 1,5 min).
- Ninguna comanda se envió a una impresora real. La 0012 no se aplicó a la instalación real.

## Fase B · interfaz de recetas, consumo y cancelaciones (B2)

Interfaz de la fase B sobre el contrato de B1 (`desktop/src/services/inventoryRecipes.ts`, sin cambios): pestaña
**Recetas** (destinos del catálogo, editor versionado, preparaciones, porciones habituales y elaboraciones), pestaña
**Ajustes y mermas** con las cancelaciones por clasificar y su historial, diagnóstico de las solicitudes de consumo y
traza de una partida en **Movimientos**, y dos cifras nuevas en el **Resumen**. Sin cambios en Go, `api.rs`,
`services/api.ts`, la captura ni el catálogo. Ninguna migración se aplicó a la instalación real.

### Pantallas

| Pestaña / apartado                              | Permiso                                                                                                                          | Contenido                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| ----------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Recetas → Productos y complementos              | `inventory.read` (crear y editar con `inventory.recipes.manage`)                                                                 | `GET recipe-targets`: productos, variantes y complementos con su estado («Con receta», «Sin receta», «Sin preparación»), tipo (con «Combo»), qué consumen sin receta («1 de 2 ingredientes con porción», vínculo del complemento, «Usa la receta del producto», «No consume inventario todavía») y la receta con su estado. Búsqueda, tipo, estado de receta (el filtro «Sin receta» deja lo pendiente) y disponibilidad (activos por omisión); paginación de 25. «Configurar receta», «Editar receta» / «Ver receta» y, en variantes que heredan, «Ver receta del producto».                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Editor de receta                                | `inventory.read`; escritura con `inventory.recipes.manage` (producto, complemento) o `inventory.subrecipes.manage` (preparación) | Destino fijo, nombre, rendimiento (porciones en productos y complementos; por lote con unidad y merma de elaboración en preparaciones, con el rendimiento efectivo de Go), renglones de tipo insumo, preparación o ingrediente del catálogo (con «Un insumo» / «Una preparación» que consume cada porción), cantidad con `numericInput`, unidad filtrada por la dimensión que entrega Go (la de la unidad base del insumo o la del rendimiento de la preparación), «Solo para llevar» (insumos y preparaciones de productos y complementos), notas e interruptor «Receta activa». El botón dice «Crear receta (versión 1)» o «Guardar como versión N» y un aviso explica que cada guardado crea una versión vigente desde ese momento sin alterar lo ya consumido. Cada renglón guardado muestra la conversión de Go a la unidad base y su costo (solo si Go lo entrega). Debajo: costo teórico de la versión vigente (por porción y adicional para llevar, o lote y costo por unidad base), ingredientes del catálogo con la porción que les aplica, recetas que usan una preparación e historial de versiones (acordeón) con la consulta de una versión anterior. Los rechazos de Go (`RECIPE_CYCLE`, unidades incompatibles, `PREPARATION_IN_USE`, `YIELD_DIMENSION_LOCKED`…) se avisan con su mensaje. |
| Recetas → Preparaciones                         | `inventory.read` (edición con `inventory.subrecipes.manage`)                                                                     | Dos secciones separadas: «Elaboración inmediata» y «Preparadas previamente (almacenables)», cada una con su alta y su tabla (rendimiento por lote, componentes, versión vigente, estado). Búsqueda y estado. Una almacenable nueva elige su insumo preparado (solo insumos de tipo «Preparación almacenable»).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| Recetas → Porciones habituales                  | `inventory.read` (edición con `inventory.recipes.manage`)                                                                        | Ingredientes del catálogo con su porción vigente («20 g de Cebolla blanca»), su equivalencia en la unidad base y el último cambio; filtros de búsqueda, con o sin porción y estado. Panel por ingrediente con insumo, cantidad y unidad de la dimensión del insumo, «Quitar porción» (confirmación de peligro) e historial (acordeón).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Recetas → Elaboraciones                         | `inventory.production.record` o `inventory.movements.read` (registrar con `inventory.production.record`)                         | Listado (fecha, preparación y versión, almacén, lotes, esperado, producido, merma, valor con costos, responsable). «Registrar elaboración»: preparación almacenable activa, almacén (el predeterminado propuesto), lotes, cantidad producida real opcional (con el rendimiento efectivo por lote de Go en la ayuda) y nota; se confirma antes de enviar y muestra el resultado de Go (esperado, producido, merma, valor consumido, insumos consumidos con su existencia después) y un aviso por cada advertencia.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Ajustes y mermas → Cancelaciones por clasificar | `inventory.read` (clasificar según `can_classify` de Go)                                                                         | Tarjeta por partida cancelada después de liberarse: producto, cuenta legible («Cuenta #12 · Mesa 4», «Pedido #7»), hora, día operativo, límite para clasificar (o «Corte del día operativo» si ya pasó), consumo por insumo y almacén (valor solo con costos), «No preparado» y «Preparado (merma)» con `inventory.waste.record`; después del corte, «Corregir: …» con `inventory.adjustments.authorize` y confirmación de peligro. Sin permiso se explica cuál falta. «Ver consumo de la partida» con `inventory.movements.read`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Ajustes y mermas → Historial de clasificaciones | `inventory.read`                                                                                                                 | Todas las cancelaciones (o por clasificación) y días operativos (`OptionalDateField`): estado, resolución de Go, quién clasificó («Por cierre del día operativo» en las automáticas), hora y nota. «Cambiar» antes del corte y «Corregir» después (panel con motivo opcional y confirmación; de peligro en las correcciones).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Movimientos                                     | `inventory.movements.read`                                                                                                       | Acordeón «Solicitudes de consumo» con «N pendientes · M con error» (se abre solo si hay errores), filtro (sin procesar, pendientes, con error, procesadas, todas), partida y cuenta, tipo, liberación, estado con intentos y próximo intento, mensaje genérico de Go y diagnósticos traducidos (sin códigos ni identificadores). «Ver consumo» en cada solicitud y «Ver consumo de la partida» en cada movimiento ligado a una partida. El filtro de origen agrega Liberación a cocina, Modificación de partida, Cancelación de partida y Elaboración.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Traza de la partida                             | `inventory.movements.read` (valores con `inventory.costs.read`)                                                                  | `GET consumption/{unidad}`: estado (consumo registrado, revertido o sin consumo), cuenta, hora de liberación y modalidad, consumo por insumo y almacén (consumido y neto), cancelación si la hubo, movimientos del libro y solicitudes con sus diagnósticos.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| Resumen                                         | `inventory.read` / `inventory.movements.read`                                                                                    | «Cancelaciones por clasificar» (`pending` de `GET cancellations`) abre Ajustes y mermas; «Solicitudes de consumo con error» (`failed` y `pending` de `GET consumption-requests`, solo con `inventory.movements.read`) abre Movimientos. Si la consulta falla, la cifra no aparece.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |

Insumos: el panel del insumo agrega «Tipo de insumo» (Comprado / Preparación almacenable), necesario para crear la
preparación almacenable que lo produce.

### Decisiones y desviaciones menores

1. **Apartados de Recetas** con `SegmentedControl` («Productos y complementos», «Preparaciones», «Porciones
   habituales», «Elaboraciones»); el editor reemplaza al listado como en Insumos y al cerrarlo se vuelve al mismo
   apartado. «Elaboraciones» solo aparece con `inventory.production.record` o `inventory.movements.read` (el permiso de
   `GET productions`).
2. **Tipo de insumo en Insumos.** La interfaz de la fase A siempre creaba insumos «Comprado», así que no había forma de
   crear el insumo de una preparación almacenable. Se agregó el selector; Go sigue rechazando cambiar el tipo de un
   insumo que produce una preparación (`ITEM_PRODUCED_BY_RECIPE`).
3. **Unidad de cada renglón**: se envía siempre la unidad elegida (`unit_id`), propuesta con la unidad base del insumo
   o la del rendimiento de la preparación; las opciones son las unidades activas de la dimensión que entrega Go (más
   la guardada si se desactivó). La conversión y el costo del renglón guardado se muestran mientras no cambien.
4. **Ingrediente del catálogo** de un renglón: los del producto (de `Recipe.ingredients`, cuando la receta ya existe)
   y después los demás del catálogo que coincidan, consultados con `GET ingredient-portions` (no exige permisos de
   catálogo). En una receta nueva todavía no se conocen los del producto.
5. **Activar y desactivar** se confirman al pulsar el interruptor (desactivar con tono de peligro) y se aplican al
   guardar como una versión nueva, igual que el interruptor de activo de la fase A.
6. **Preparaciones**: una página de hasta 100 (el máximo de Go) repartida entre las dos modalidades, porque
   `GET recipes` no filtra por modalidad; con más, un aviso pide usar la búsqueda.
7. **Clasificar** una cancelación pendiente no pide confirmación (es la acción cotidiana del encargado); una pendiente
   vencida y toda corrección posterior al corte sí, con tono de peligro. Cambiar una clasificación antes del corte se
   ofrece («Cambiar», confirmación normal) porque Go lo admite con `inventory.waste.record`. La nota solo se captura
   en el cambio o la corrección.
8. **Historial**: el filtro de Go admite un solo estado, así que «Todas» incluye las pendientes (con su etiqueta).
9. **Cuenta legible** de una cancelación: `Cancellation` no trae la modalidad; con el nombre neutro de Go se muestra
   «Pedido #N» (para llevar) y, si no, «Cuenta #N» más el nombre de la mesa o el capturado.
10. **Cifras del Resumen** tomadas de los listados de la fase B (`pending` y `failed`), porque `GET summary` aún no las
    trae; no se calcula nada en React.
11. **Elaboraciones**: la fecha no se captura (Go usa la hora del servidor, como la existencia inicial) y siempre se
    confirma antes de enviar porque una elaboración no se anula.
12. **Traza** de una partida dentro de la pestaña (Movimientos o Ajustes y mermas), con «Volver a …», sin otra ventana.
13. **Playwright**: la especificación nueva se llama `stock-consumption.spec.ts` para correr después de
    `inventory.spec.ts` y `restaurant.spec.ts` (orden alfabético) y no alterar lo que comprueban; prepara por la API lo
    que no se prueba (ruta «Sin preparación» del filete si no hay ruta, un ingrediente del catálogo y el teclado
    virtual desactivado, que `restaurant.spec.ts` deja en «siempre»). `inventory.spec.ts` solo cambió en la lista de
    pestañas.
14. **Puente de prueba**: `native-driver.ts` descubre las rutas de `api.rs` con expresiones regulares y no reconoce la
    forma con llaves de `inventory_unit_consumption` (`=> {\n (Method::GET, format!(…)) }`). La especificación nueva
    atiende solo esa operación con la misma sesión del puente y la misma ruta de Go (`bridgeUnitConsumption`). En la
    aplicación nativa no hay problema (la ruta está en `api.rs`); conviene ajustar la expresión del puente o el formato
    de `api.rs` en la integración.

### CSS añadido (`styles.css`, al final del bloque «Etapa 8: Inventarios»)

Solo tokens existentes de ambos temas: títulos de tercer nivel de la sección (sin tocar los de los acordeones),
apartados de Recetas que se acomodan en varias líneas, aviso de versión nueva (`recipe-version-note`), renglones de
receta, detalle de una versión del historial, acciones de las filas de destinos, botón de traza en el libro, tarjeta de
cancelación con acento de advertencia (`inventory-cancellation`), lista de datos (`inventory-facts`), acciones de
clasificación, panel de cambio o corrección, filtros del historial y el filtro de solicitudes. A 720 px o menos los
botones de clasificación ocupan el ancho. La página no se desborda a 1280 × 720 ni a 1920 × 1080 (comprobado en
Playwright en claro y oscuro en Recetas, el editor, Porciones, la traza y Cancelaciones).

### Limitaciones

- `inventory_unit_consumption` no funciona en el puente de prueba de Playwright sin el ajuste local de la
  especificación (ver la decisión 14).
- La cuenta de una cancelación para llevar con nombre del cliente se muestra como «Cuenta #N · Nombre» (Go no entrega la
  modalidad en `Cancellation`).
- En una receta nueva el selector de ingrediente no distingue los del producto hasta el primer guardado.
- El costo teórico es el que entrega Go (sin complementos ni componentes de combo); «Adicional para llevar» aparece
  aunque sea $0.00.
- Las elaboraciones no se anulan ni se editan (contrato de B1); el historial de una porción muestra los últimos 20
  cambios y el de una receta las últimas 50 versiones (límites de Go).
- Sin verificar en la ventana nativa (WKWebView), con pantalla táctil física ni con teclado virtual activo en estas
  pantallas.

### Pruebas (B2)

- **Vitest** (`pages/inventory/inventoryRecipes.test.tsx`, `inventoryConsumption.test.tsx` con
  `inventoryRecipesTestData.ts`; 20 pruebas nuevas): destinos con el filtro «Sin receta» en la consulta, variantes que
  heredan, vínculo de complementos y consulta sin permiso (botón de guardar deshabilitado y aviso del permiso); alta de
  la versión 1 con renglones de los tres tipos, unidades filtradas por dimensión (masa: g y kg; volumen: l y ml; piezas:
  pza), carácter no válido descartado, «solo para llevar» (ausente en el renglón de ingrediente) y el cuerpo exacto
  enviado a Go; versión nueva con la revisión vigente, conversión y costo del renglón mientras no cambie, costo teórico
  e ingredientes del catálogo con permiso y ocultos sin `inventory.costs.read`, historial con la versión 1 (150 g frente
  a 180 g vigentes); desactivar con confirmación de peligro (`active: false`); error `RECIPE_CYCLE` de Go mostrado con su
  mensaje (sin el código) en una preparación, que no ofrece «solo para llevar» y conserva la dimensión del rendimiento;
  porciones (filtro «Sin porción», guardar con revisión, historial y retirar con confirmación de peligro); elaboración
  (solo almacenables, almacén predeterminado, confirmación, cuerpo, resultado de Go con merma, existencia negativa y su
  advertencia) y visibilidad del apartado según permisos; cancelaciones (datos legibles sin identificadores, «No
  preparado» con revisión, botones según `can_classify` con el permiso que falta, corrección vencida con confirmación
  de peligro que se puede cancelar sin escribir, historial con «Por cierre del día operativo», corrección con motivo y
  filtro); solicitudes con error (acordeón abierto, intentos, mensaje genérico, diagnósticos traducidos sin códigos ni
  identificadores) y traza desde la solicitud (sin valores sin costos) y desde un movimiento de consumo (con valores);
  cifras del Resumen y su ausencia sin `inventory.movements.read`; ningún `type="number"` en Recetas, el editor ni
  Ajustes y mermas. En `inventory.test.tsx` se actualizaron las listas de pestañas y en `inventoryTestData.ts` las
  respuestas vacías de la fase B y los permisos nuevos.
- **Playwright** (`desktop/e2e/stock-consumption.spec.ts`): insumo «Carne de prueba E2E» en kilogramos con existencia
  inicial de 5 kg a $180, receta de «Filete de prueba» encontrada con el filtro de lo pendiente (150 g capturados en
  gramos; solo unidades de masa; Go muestra 0.15 kg y costo teórico $27.00), porción habitual de 50 g de un
  ingrediente del catálogo, «Control de inventario» activado en Configuración → Funciones del establecimiento, pedido
  para llevar con el filete enviado a cocina, consumo esperado con `expect.poll` sobre Go y comprobado en Movimientos
  (−0.15 kg, «Liberación a cocina», «Proceso de consumo de inventario», −$27.00), traza de la partida, cancelación de la
  partida en la captura, tarjeta en «Cancelaciones por clasificar», «No preparado», historial y reversión (+0.15 kg y
  existencia de vuelta en 5 kg según Go). Sin desbordamiento a 1280 × 720 ni 1920 × 1080 en claro y oscuro en cada
  pantalla, con capturas a 1280 × 720.

### Verificación (B2)

En la copia aislada `scratchpad/repoB2` (sincronizada con el árbol de trabajo, incluido el trabajo en curso de otras
partes de la fase), con el clúster temporal del puerto 55499, Go temporal en 8184 y la vista previa en 1424 (nunca la
instalación real):

- `prettier --check` de los archivos de B2 y de `styles.css`: sin diferencias.
- `tsc --noEmit`: sin errores. `npm run build`: aprobado (el aviso de tamaño del paquete ya existía).
- Vitest (`vitest run src`, como `npm test`): 44 archivos y 365 pruebas aprobadas; `src/pages/inventory`: 4 archivos y
  39 pruebas (20 nuevas de la fase B). Sin avisos de React.
- Playwright `stock-consumption.spec.ts` sola (base nueva, ruta «Sin preparación» preparada por la API): Chromium
  aprobada (27.2 s) y WebKit aprobada (26.5 s).
- Recorrido completo sin `SPEC` (una base por motor; `inventory.spec.ts`, `restaurant.spec.ts`,
  `stock-consumption.spec.ts` y `takeaway-quick-notes.spec.ts` de Bq): Chromium 4 aprobadas (20.8 s, 54.3 s, 26.0 s y
  8.1 s) y WebKit 4 aprobadas (20.2 s, 52.2 s, 26.1 s y 8.0 s). Una corrida anterior falló en Chromium porque
  `restaurant.spec.ts` deja el teclado virtual en «siempre» y el primer toque de «Confirmar cancelación de unidades» se
  perdía; la especificación ahora lo desactiva por la API y reintenta ese toque si el diálogo no aparece.
- Capturas a 1280 × 720 en claro y oscuro (Chromium; también las de WebKit en subcarpeta): `scratchpad/shots8B2/`
  (`b2-recetas-lista-*`, `b2-recetas-editor-*`, `b2-porciones-*`, `b2-cancelaciones-*`, `b2-traza-consumo-*`), con el
  tema comprobado por un píxel de fondo (`docs8B2/check_dark.py`).

### Integración de la fase B (agente principal)

- El puente de prueba de Playwright (`desktop/e2e/native-driver.ts`) ahora reconoce también las entradas de `api.rs`
  que `rustfmt` escribe con llaves (`"op" => { (Method::GET, format!(…)) }`), como `inventory_unit_consumption`; se
  quitó el atajo provisional de `stock-consumption.spec.ts`.
- `native-driver.ts` y `playwright.config.ts` leen `ADEL_BROWSER_SERVER_URL` y `ADEL_BROWSER_PREVIEW_PORT` (por omisión
  `http://127.0.0.1:8183` y `1421`, como antes) para ejecutar dos aceptaciones aisladas a la vez.
- La fase B se desarrolló en tres partes paralelas (B1 backend de recetas y consumo, Bq indicaciones con efecto y
  complementos, B2 interfaz) y por eso tiene dos migraciones: 0011 (recetas y consumo) y 0012 (indicaciones con
  efecto). Las suites completas se ejecutaron sobre una copia exacta del commit de cierre de la fase.

### Verificación de cierre de la fase B

Sobre una copia exacta del commit `a1d4e74` (sin cambios en curso de otras fases), clúster temporal 55499:

- Prettier, gofmt, `cargo fmt`, `sync-metadata`, `go vet` y `go vet -tags=integration`: aprobados.
- Go unitarias con `-race`: todos los paquetes `ok`.
- Integración PostgreSQL con `-race`: `database`, `administration`, `catalog`, `operations` y `payments` `ok`.
- Vitest: 44 archivos y 365 pruebas aprobadas; `tsc --noEmit` y build sin errores.
- `cargo test`: 18 aprobadas y 1 ignorada (la misma de antes).
- Playwright en Chromium y en WebKit: 4 de 4 aprobadas cada uno (`inventory`, `restaurant`, `stock-consumption`,
  que activa el control, envía un producto con receta a cocina y comprueba el consumo y la clasificación de una
  cancelación, y `takeaway-quick-notes`).

## Fase C · backend de compras (C1)

Proveedores con sus insumos, órdenes de compra, recepciones de mercancía (de una orden o directas), devoluciones a
proveedor y vínculo opcional de una recepción con gastos de caja: migración `0013_inventory_purchasing.sql`, archivos
nuevos en `backend/internal/inventory` (`purchasing.go`, `purchase_lines.go`, `suppliers.go`, `purchase_orders.go`,
`receipts.go`, `supplier_returns.go`, `cash_links.go`), rutas `inventory/*`, puente Tauri y
`desktop/src/services/inventoryPurchasing.ts`. La interfaz de Compras, Recepciones y Proveedores es la otra parte de la
fase C. La 0013 no se aplicó a la instalación real.

### Migración 0013

Transaccional, sin tocar 0001–0012 ni filas existentes (el único `ALTER` es sobre su propia tabla
`inventory_supplier_items`, para la llave al último recibo), **sin siembras** y con 138 restricciones de nombre explícito
(todos los nombres caben en los 63 caracteres de PostgreSQL); los CHECK con columnas opcionales exigen `IS NOT NULL` en la
rama que valida. Folios propios por documento con `GENERATED ALWAYS AS IDENTITY` (como el folio de las cuentas: puede
haber huecos si una transacción se revierte).

| Estructura                        | Contenido                                                                                                                                                                                                                                                                                                                                                                                                         |
| --------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `inventory_suppliers`             | Nombre comercial único sin mayúsculas, razón social, RFC opcional con formato del SAT (CHECK; no único), teléfono, correo, domicilio, contacto, observaciones, `active`, revisión y responsables. Sin borrado.                                                                                                                                                                                                    |
| `inventory_supplier_items`        | `(supplier_id, item_id)`: presentación habitual, código del proveedor, `preferred` (índice único parcial por insumo) y el último precio recibido completo o ausente (precio sin IVA antes del descuento, presentación **o** unidad de captura, fecha y recepción).                                                                                                                                                |
| `inventory_purchase_orders`       | Folio, proveedor, estado (`draft`, `issued`, `partially_received`, `received`, `cancelled`, `closed`), fecha, almacén previsto, observaciones, subtotal/descuento/IVA/total (`numeric(18,2)`, `total = subtotal + IVA`), responsable, emisión, cancelación y cierre con quién, cuándo y motivo (CHECK por estado), revisión y `operation_id`.                                                                     |
| `inventory_purchase_order_lines`  | Insumo, presentación **o** unidad, cantidad capturada y base, precio sin IVA `numeric(18,6)`, descuento, tasa 16/8/0, subtotal, IVA y total; un insumo por orden. **Disparador**: solo se insertan, cambian o quitan mientras la orden es borrador.                                                                                                                                                               |
| `inventory_receipts`              | Folio, proveedor obligatorio, orden opcional (NULL = directa), almacén, hora, quién recibió, referencia del proveedor, `tax_in_cost` copiado, importes, `inventory_value` (CHECK: total con IVA o subtotal según `tax_in_cost`), `excess` (solo con orden), `operation_id` único. Solo inserción.                                                                                                                 |
| `inventory_receipt_lines`         | Renglón de la orden (llaves compuestas: la orden debe ser la de su recepción), insumo, captura, cantidades, importes, valor de inventario, costo por unidad base y su movimiento `receipt`. Solo inserción.                                                                                                                                                                                                       |
| `inventory_supplier_returns`      | Folio, recepción, proveedor, almacén, hora, motivo (5–500), responsable, valor asociado, diferencia no absorbida, `operation_id` único. Solo inserción.                                                                                                                                                                                                                                                           |
| `inventory_supplier_return_lines` | Renglón de la recepción (un renglón por devolución, de la misma recepción), cantidad capturada y base, costo original, valor, valor de salida, ajuste de costo y no absorbida (CHECK `valor − salida = no absorbida − ajuste`; ajuste ⇔ su movimiento). Solo inserción.                                                                                                                                           |
| `inventory_receipt_cash_links`    | Recepción, `cash_operation_id` (FK a `cash_operations`, que no se modifica), importe > 0, notas, quién y cuándo vinculó, `operation_id` único, y la desvinculación completa o ausente (quién, cuándo, motivo 5–500 y su operación). Un vínculo vigente por par recepción–gasto (índice único parcial). **Disparador**: no se borra ni se modifica; la única actualización es registrar la desvinculación una vez. |
| Permisos                          | `inventory.suppliers.manage`, `inventory.purchases.create`, `inventory.purchases.authorize`, `inventory.purchases.receive`, `inventory.supplier_returns.record`, `inventory.purchases.link_cash`, solo al administrador (20 permisos de inventario en total).                                                                                                                                                     |

### Proveedores, órdenes, recepciones y devoluciones

- **Proveedores**: alta y edición con revisión, búsqueda (nombre, razón social, RFC, contacto, teléfono, correo), filtros de
  estado e insumo surtido, desactivación (`active: false`; nunca hay borrado y un proveedor con compras se conserva con su
  historial), insumos que surte (código, presentación, preferido único: marcar otro quita la marca del anterior y queda
  auditado) e historial (conteos, total recibido, valor devuelto y las 20 órdenes, recepciones y devoluciones más
  recientes). Un proveedor desactivado no admite órdenes nuevas ni recepciones directas; la mercancía de una orden ya
  emitida se puede recibir.
- **Órdenes**: borrador (crear y editar con `purchases.create`; reemplaza datos y renglones), emisión (`purchases.authorize`,
  exige al menos un renglón, proveedor e insumos activos), cancelación (con motivo, desde borrador, emitida o parcial: lo
  recibido se conserva) y cierre administrativo (con motivo, desde emitida o parcial). Crear o emitir no toca existencias
  ni caja. Cada recepción cambia el estado (parcial o recibida) y la revisión.
- **Recepciones**: `purchases.receive`; la directa además `purchases.create` y proveedor activo. Por renglón: solicitado,
  recibido antes, pendiente después, devuelto y devolvible. Exceso solo con `allow_excess` (aviso `RECEIPT_EXCESS`). Copia
  `tax_in_cost`, registra un `receipt` por renglón con el libro (regla 2: el ajuste de costo de un déficit va aparte),
  actualiza el último precio del proveedor (y crea la relación si no existía) y no crea gastos.
- **Devoluciones**: `supplier_returns.record`, ligadas a una recepción, por renglón y en su misma presentación o unidad,
  nunca más de lo recibido menos lo devuelto; salida `supplier_return` con la regla 5 del motor; sin ingresos de caja.
- **Vínculos con gastos**: ver «Decisiones» y el contrato.

### Reglas de importes, IVA y costo

- Renglón: importe = redondeo(cantidad × precio sin IVA); subtotal = importe − descuento comercial (en pesos, nunca mayor
  que el importe); IVA = redondeo(subtotal × tasa ÷ 100) con tasa de lista cerrada 16/8/0; total = subtotal + IVA. Redondeo
  a centavos mitad alejándose de cero. El documento es la suma exacta de sus renglones (el IVA no se recalcula sobre el
  total), igual que el sistema anterior, así que siempre cuadra.
- Recepción de una orden: precio y tasa los de la orden salvo que se capturen otros (factura real); el descuento del
  renglón de la orden se **prorratea de forma acumulada** por lo recibido (redondeo(D × min(antes + ahora, Q) ÷ Q) −
  redondeo(D × min(antes, Q) ÷ Q)), de modo que las recepciones que completan la cantidad suman exactamente el descuento
  (12 + 8 de 20 cajas con $50: $30.00 y $20.00) y lo recibido de más no lleva descuento. No se usa `payments.allocate`
  porque no hay descuento de documento que repartir entre renglones.
- Valor de inventario de cada renglón = total con IVA si `tax_in_cost`, si no subtotal; costo por unidad base = valor ÷
  cantidad base (10 decimales, lo calcula el libro). Cambiar la configuración solo afecta recepciones futuras.
- Devolución: valor asociado = cantidad base × valor del renglón ÷ cantidad base recibida, con un costo de 20 decimales
  para que devolver todo lo que quedaba dé exactamente el valor restante; el documento guarda el costo a 10 decimales.

### Rutas y permisos

Todas exigen `inventory.read`; contrato completo en la sección «Inventario · fase C: compras (esquema 13)» de
`API_CONTRACTS.md` (texto en `docs8C1/api_contracts.md`).

| Ruta                                                                                                                                                                               | Permiso adicional                                                                           |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `GET suppliers`, `GET suppliers/{id}`, `GET purchase-orders`, `GET purchase-orders/{id}`, `GET receipts`, `GET receipts/{id}`, `GET supplier-returns`, `GET supplier-returns/{id}` | cualquiera de los seis permisos de compras (costeo solo con `inventory.costs.read`)         |
| `POST suppliers`, `PATCH suppliers/{id}`, `PUT suppliers/{id}/items`, `POST suppliers/{id}/items/remove`                                                                           | `inventory.suppliers.manage`                                                                |
| `POST purchase-orders`, `PATCH purchase-orders/{id}`                                                                                                                               | `inventory.purchases.create`                                                                |
| `POST purchase-orders/{id}/issue`, `…/cancel`, `…/close`                                                                                                                           | `inventory.purchases.authorize`                                                             |
| `POST receipts`                                                                                                                                                                    | `inventory.purchases.receive` (+ `inventory.purchases.create` en la directa)                |
| `POST supplier-returns`                                                                                                                                                            | `inventory.supplier_returns.record`                                                         |
| `GET cash-expenses`, `POST receipts/{id}/cash-links`, `POST cash-links/{id}/unlink`                                                                                                | `inventory.purchases.link_cash` + `cash.movements.read` (+ reglas de caja; locks 305 → 308) |

Puente Tauri: 22 operaciones nuevas en `api.rs` (9 fijas y 13 con un identificador; las 22 las descubre el driver de
Playwright) con la prueba `inventory_purchasing_routes_are_closed`; lista blanca de consulta ampliada con `supplier_id`,
`purchase_order_id`, `receipt_id`, `register_id`, `direct`, `min_amount` y `max_amount`. `Operation` de
`desktop/src/services/api.ts` ampliado con esas 22 operaciones (cambio mínimo al final). Los orígenes `receipt` y
`supplier_return` tienen etiqueta en la consulta de movimientos.

### Decisiones y desviaciones (fase C, backend)

1. **Lectura de compras** con cualquiera de los seis permisos de compras (no existe `purchases.read`); los importes de
   compra son visibles para ellos y el costeo de inventario solo con `inventory.costs.read` (lo conservador del brief). El
   valor asociado de una devolución se trata como importe de compra; su valor exacto, el de salida, el ajuste y la
   diferencia no absorbida, como costeo.
2. **Insumo de un proveedor en el cuerpo** (`PUT suppliers/{id}/items` y `POST suppliers/{id}/items/remove`) en lugar de
   `suppliers/{id}/items/{item_id}`: el puente admite un solo identificador por ruta y no hay método DELETE. Quitar un
   insumo del proveedor borra la fila de configuración (no historia; auditado con el antes).
3. **Último precio**: el de la recepción más reciente (por hora de la recepción), sin IVA y antes del descuento, en la
   presentación o unidad de esa recepción (columnas adicionales `last_price_presentation_id`, `last_price_unit_id`,
   `last_price_at`, `last_receipt_id`). Una recepción crea la relación proveedor–insumo si no existía.
4. **Un insumo una sola vez** por orden o recepción directa; la recepción de una orden recibe solo sus renglones (cada uno
   una vez) en su misma presentación o unidad; los artículos no ordenados van en una recepción directa.
5. **Recepción de una orden**: con orden emitida o parcial; con un proveedor, insumo o presentación desactivados después de
   emitirla se puede recibir (la mercancía ya estaba comprometida). Una orden recibida no admite más recepciones (lo que
   llegue después es una recepción directa); recibir más de lo pendiente exige `allow_excess`.
6. **Cancelación y cierre**: cancelar procede desde borrador, emitida o parcial (conserva las entradas); cerrar
   administrativamente desde emitida o parcial. Una orden recibida no se cancela ni se cierra.
7. **Fecha de la orden** AAAA-MM-DD no futura (zona del establecimiento); la hora de la recepción y de la devolución no es
   futura y la devolución no es anterior a su recepción.
8. **Vínculos con gastos**: se toma 7820148305 **compartido** antes de 7820148308 en `GET cash-expenses` y en vincular y
   desvincular (orden de `CLAUDE.md` §3: inventario después de 305); el resto de las rutas de inventario no toman 305 y la
   lectura del estado del vínculo en una recepción es una sola sentencia. Leer un gasto exige `cash.movements.read` y las
   reglas de `readableShift`, **sin** la excepción de payments para quien cerró el turno (decisión conservadora); desvincular
   exige poder leer el gasto, como vincularlo. Un vínculo vigente por par recepción–gasto (para cambiar el importe se
   desvincula y se vincula de nuevo). Un vínculo con un gasto corregido o regularizado **después** sigue contando en los
   límites hasta desvincularlo; su estado (`expense_corrected` / `expense_regularized`) y `cash_link_attention` de la
   recepción son la señal para la alerta de la fase D. Los datos del gasto de un vínculo solo se entregan a quien puede
   leerlo (`expense_visible`); el estado, siempre.
9. **Gastos candidatos**: solo `kind='expense'`, con `linkable`, lo disponible y el motivo si no se puede vincular
   (`currency`, `corrected`, `fully_linked`); filtros por días de negocio del registro, importe mínimo y máximo, caja y
   lo vinculado con una recepción. Sin caja en la terminal ni `cash.shifts.read_all`, la lista es vacía.
10. **Eventos**: proveedores e insumos de proveedor publican `inventory.purchases.changed` (no hay un tipo propio de
    proveedores en el diseño); devoluciones y vínculos publican `inventory.receipts.changed`.
11. Prueba de la fase A/B no modificada salvo el encadenamiento: `testInventoryCore` llama a `testInventoryPurchasing` al
    final, y la prueba de actualización cuenta 20 permisos de inventario y llama a `checkStage8PhaseC`.

### Limitaciones

- Sin interfaz (otra parte de la fase C) ni alertas (fase D: los datos existen en `cash_link_attention`, `status` de los
  vínculos, `GET purchase-orders?state=partially_received` y `GET suppliers?item_id=`).
- Sin cuentas por pagar, reporte de compras por periodo (etapa 9), devoluciones de una recepción directa sin recepción ni
  notas de crédito: una devolución no genera ingreso de caja ni saldo a favor.
- La recepción de una orden usa la equivalencia vigente de la presentación o unidad del renglón (si cambió después de
  emitir la orden, la cantidad base recibida usa la nueva); lo pendiente se cuenta en la presentación o unidad capturada.
- Los folios usan identidad de PostgreSQL: un registro revertido deja un hueco en la serie.
- El gasto en dólares de la prueba se insertó directamente en `cash_operations` (el pago en dólares es una función
  activable ajena a la prueba); los demás gastos, correcciones, regularizaciones y el cierre se hicieron por la API de caja.

### Pruebas de la fase C (backend)

- **Unitarias** (`internal/inventory/purchasing_test.go`; 45 pruebas del paquete en total):
  - `TestPurchaseLineAmounts`: 20 cajas × $250 − $50 = $4,950.00 + IVA $792.00 = $5,742.00; 3 × 33.333333 = $100.00 + 8 %;
    7 × 12.345 = $86.42 exento (mitad alejándose de cero); IVA de $10.03 al 16 % = $1.60 y al 8 % = $0.80; de $10.05 al
    16 % = $1.61; medio centavo; descuento igual al importe; documento = suma de renglones ($5,782.08); rechazos (descuento
    mayor que el importe, tasa 15 %, cantidad cero, precio negativo, importe excesivo).
  - `TestProratedDiscount`: 12 + 8 de 20 cajas con $50 → $30.00 y $20.00; $10 en tercios → 3.33, 3.34, 3.33; excedente
    parcial (18 + 5 de 20 → $5.00) y total (sin descuento); recibir todo de una vez.
  - `TestNormalizeRFC`: persona moral y física, público en general, Ñ y 29 de febrero bisiesto, `&`; rechazos de mes 13,
    30 de febrero, 29 de febrero no bisiesto, longitudes y guiones.
  - `TestPrepareReturnLines`: 4 de 10 cajas = 96 piezas por $464.000000 y las 6 restantes = 144 piezas por $696.000000
    exactos; una caja de más → `RETURN_EXCEEDS_RECEIVED`; renglón inexistente o repetido.
  - `TestInventoryPermissionsAreMigratedAndGrantedToAdministrator`: los 20 permisos de Go en la 0010, 0011 o 0013 con la
    misma descripción, solo para el administrador.
- **Integración** (`operations/inventory_purchasing_test.go`, encadenada al final de la fase B con insumos, proveedores,
  usuarios y una caja propios; 10 subpruebas):
  - proveedores: alta con RFC normalizado, domicilio de varias líneas, 403 sin `suppliers.manage` y sin permisos de compras
    (nada registrado), nombre comercial repetido sin mayúsculas, RFC y correo inválidos, RFC compartido, edición con revisión
    y revisión obsoleta, insumos surtidos (preferido único: el segundo quita la marca al primero), presentación ajena,
    quitar un insumo, búsqueda por RFC, filtros por insumo y estado, parámetro desconocido 400, 7 auditorías;
  - órdenes: borrador ($6,797.40) y edición a 5 l (subtotal $6,036.00, descuento $50.00, IVA $810.00, total $6,846.00;
    480 piezas, 50,000 g y 5,000 ml), rechazos (tasa 15 %, descuento mayor, insumo repetido, presentación ajena, fecha
    futura), 403 al crear sin `purchases.create` y al emitir sin `purchases.authorize`, emisión, segunda emisión
    `ORDER_NOT_DRAFT`, edición de emitida `ORDER_NOT_EDITABLE`, renglones de emitida inmutables en PostgreSQL, ningún
    movimiento ni operación de caja al crear o emitir, orden vacía `ORDER_EMPTY`, cancelación con motivo corto (400) y
    válida, cierre de un borrador `ORDER_NOT_CLOSABLE`, recibir una cancelada `ORDER_NOT_RECEIVABLE`, filtro `open` y
    búsqueda por folio;
  - recepción parcial y total (12 + 8 de 20 cajas): la primera por quien solo recibe ($3,875.70: 3,445.20 del refresco con
    $30.00 de descuento prorrateado y $430.50 del costal), solicitado 20 / recibido antes 0 / pendiente después 8, sin
    costos para quien no tiene `costs.read` y con ellos 11.9625 por pieza y 0.01722 por gramo; orden parcial (12/8, 1/1,
    0/5, pendiente base 192); repetición con el mismo `operation_id` (misma recepción, sin movimientos nuevos) y otro cuerpo
    409 `IDEMPOTENCY_CONFLICT`; 9 cajas sin `allow_excess` 409 `RECEIPT_EXCEEDS_PENDING`; insumo propio en un renglón de
    orden 400; segunda recepción ($2,970.30: $20.00 de descuento, recibido antes 12, pendiente 0); orden recibida (las dos
    suman $6,846.00 = la orden); 480 piezas por $5,742.00; tres movimientos `receipt`; último precio $250.00 «Caja con 24»;
    historial del proveedor; ninguna operación de caja;
  - exceso con `allow_excess` (3 de 2 costales, aviso `RECEIPT_EXCESS`, excedente 1), cierre administrativo con 4 de 10
    cajas recibidas (403 sin `authorize`, las entradas se conservan, recibir después `ORDER_NOT_RECEIVABLE`) y cancelación
    con recepciones (revisión anterior 409, las entradas y su movimiento se conservan);
  - recepción directa: 403 para quien solo recibe sin dejar registros, proveedor obligatorio (400), IVA incluido ($116.00 →
    0.058 por ml) y excluido tras cambiar la configuración (mismo documento → $100.00, 0.05 por ml, movimiento de
    100.000000), la recepción anterior conserva `tax_in_cost` y su valor, filtro `direct`;
  - recepción con existencia negativa: 10 piezas a $20 y un consumo de 15 del trabajador → −5 y −$100; 8 piezas a $30 →
    movimiento `receipt` de 240 con existencia 3 y `cost_adjustment` ligado de −50 (100 provisional − 150 real); 3 piezas
    por $90; consistencia del libro;
  - devoluciones: 100 vasos a $2 y 100 a $3 con IVA (200 por $580); 403 sin `supplier_returns.record`; diferencia
    absorbida (10 vasos: salida $29.00, valor $34.80, ajuste −$5.80, 190 por $545.20); duplicada (mismo documento, sin
    movimientos) y otro cuerpo 409; más de lo recibido (101 de 100 y 91 de 90) `RETURN_EXCEEDS_RECEIVED`; motivo corto;
    consumo de 185 (→ 5 vasos por $14.347368) y diferencia no absorbida (salida $14.347368, valor $17.40, sin ajuste,
    3.052632 en el documento; 0 vasos, $0); devuelto 15 / devolvible 85; ninguna operación de caja; recepciones y
    devoluciones de solo inserción en PostgreSQL;
  - vínculos con gastos (caja «Caja de compras C» operada por la API de caja: apertura, 5 gastos, correcciones,
    regularización y cierre ciego por conteo): gasto sin compra (no crea compras), 403 sin `cash.movements.read`, sin
    `link_cash` y desde una terminal sin caja (nada registrado; su lista de candidatos vacía y `register_id` ajeno 403);
    vinculación total ($850 ↔ $850) y parcial ($500 de $3,875.70); egresos duplicados (`CASH_EXPENSE_OVERLINKED` para los
    gastos de $850 y $500 ya vinculados, `RECEIPT_OVERLINKED` para la recepción de $850 con un gasto disponible,
    `CASH_LINK_EXISTS`); gasto corregido `CASH_EXPENSE_CORRECTED` y corrección `CASH_OPERATION_NOT_EXPENSE`; corrección
    posterior detectada (`expense_corrected`, `cash_link_attention`, el gasto sin cambios); candidatos por importe y con lo
    vinculado a la recepción; turno cerrado (403 sin `cash.history.read`, ausente de sus candidatos, 200 con el permiso,
    datos del gasto ocultos sin él, turno, cierre y operaciones idénticos); regularización posterior detectada
    (`expense_regularized`) sin cambiar el cierre; gasto en dólares `CASH_EXPENSE_CURRENCY`; desvinculación (motivo corto
    400, 403 sin leer el gasto, registro con motivo, segunda 409 `CASH_LINK_INACTIVE`, nuevo vínculo); vínculos sin
    modificación ni borrado en PostgreSQL; solo 9 operaciones de caja nuevas (las de la caja); 5 + 1 auditorías;
  - desactivación de un proveedor con compras (historial conservado, orden nueva y recepción directa `SUPPLIER_INACTIVE`,
    la orden ya emitida se recibe, `DELETE` 404), eventos `inventory.purchases.changed`, `inventory.receipts.changed`,
    `inventory.stock.changed` e `inventory.movements.recorded`, auditoría de órdenes, recepciones, devoluciones y
    desactivación, consistencia del libro.
- **Actualización desde el esquema 9** (`TestStage8UpgradeOfExistingInstallation`, ampliada con
  `database/stage8_purchasing_upgrade_test.go`): todas las migraciones de la etapa 8 del árbol (0010–0013) se aplican y no
  se repiten; 20 permisos de inventario solo del administrador; la 0013 no siembra filas; sus 138 restricciones con nombre;
  CHECK con tablas temporales (RFC, último precio, estados de la orden con emisión, cancelación y cierre, total, renglones
  con captura y tasa, valor de inventario según `tax_in_cost`, exceso sin orden, ajuste y diferencia de una devolución,
  desvinculación completa); disparadores reales (renglones de una orden emitida, recepción de solo inserción, vínculo que
  solo se desvincula una vez) y gastos de caja sin cambios.
- **Rust**: `inventory_purchasing_routes_are_closed` (22 operaciones, sin identificador o con `../../settings` rechazadas,
  sin borrado de proveedores ni edición o borrado de recepciones, devoluciones y vínculos, ni creación de gastos; lista
  blanca de consulta).

### Verificación de la fase C (backend)

En la copia aislada `scratchpad/repoC1` (sincronizada desde el árbol de trabajo, con la 0012 ya presente), con el clúster
temporal del puerto 55499 (nunca la instalación real):

- `gofmt -l` (archivos Go propios), `rustfmt --check` de `api.rs` y `prettier --check` de `inventoryPurchasing.ts` y
  `api.ts`: sin diferencias.
- `go vet ./...` y `go vet -tags=integration ./...`: aprobados.
- `go test -race -count=1 ./...` (unitarias): 15 paquetes `ok`, incluido `internal/inventory` (45 pruebas).
- Integración (`go test -race -tags=integration`): `internal/database` ok (3.4 s, actualización desde el esquema 9 hasta la
  0013), `internal/administration` ok (54.7 s), `internal/operations` ok (186.5 s, con la aceptación completa de las
  etapas 4–7.1 y las fases A, B y C; las 10 subpruebas `inventory_c_*` aprobadas), `internal/payments` ok (1.3 s) e
  `internal/catalog` ok (6.8 s).
- `cargo test --locked --offline`: 19 aprobadas (1 ignorada, la misma de antes), incluida
  `inventory_purchasing_routes_are_closed`.
- `tsc --noEmit`: sin errores.
- Playwright no se ejecutó: esta parte no cambia pantallas.

## Fase C · interfaz de compras (C2)

Pestañas **Compras**, **Recepciones** y **Proveedores** dentro de Inventarios, sobre el backend de compras (C1, migración
0013, commit `b2bccb1`) y sus tipos de `desktop/src/services/inventoryPurchasing.ts` (sin cambios: el contrato de Go se
usa tal cual). Sin cambios en Go, `api.rs`, `services/api.ts`, el puente de pruebas, la captura ni el catálogo.

### Archivos

- Nuevos en `desktop/src/pages/inventory/`: `purchasingFormat.ts` (permisos de lectura, folios legibles, estados, tasas
  16/8/0 y armado de cuerpos copiando las cadenas capturadas), `purchasingNavigation.ts`, `PurchaseFields.tsx` (selector de
  proveedor, unidad o presentación, renglón de compra, tasa de IVA con botones, importes y totales de Go, acción con
  motivo), `SuppliersView.tsx` y `SupplierSections.tsx` (proveedores, insumos que surte e historial),
  `PurchaseOrdersView.tsx` y `PurchaseOrderDetail.tsx` (órdenes), `ReceiptForms.tsx` (recepción de una orden y directa),
  `ReceiptsView.tsx`, `ReceiptDetail.tsx`, `SupplierReturnForm.tsx`, `CashLinksSection.tsx`, y las pruebas
  `inventoryPurchasing.test.tsx` con `inventoryPurchasingTestData.ts`.
- Modificados: `InventoryPage.tsx` (tres pestañas y navegación entre documentos), `inventoryFormat.ts` (orígenes
  «Recepción de mercancía» y «Devolución a proveedor» en el filtro de Movimientos, con las etiquetas de Go),
  `styles.css` (reglas nuevas al final del bloque «Etapa 8: Inventarios»).
- Playwright: `desktop/e2e/vendor-purchases.spec.ts` (nuevo) y la lista de pestañas esperada en `inventory.spec.ts` y
  `stock-consumption.spec.ts` (tres renglones cada una: el administrador ahora ve Compras, Recepciones y Proveedores).

### Pantallas

- **Pestañas**: Compras, Recepciones y Proveedores, entre «Ajustes y mermas» y «Configuración» (orden de §25), visibles
  con **cualquiera** de los seis permisos de compras (como la lectura en Go). Desde el historial de un proveedor se abre
  una orden o una recepción; desde una recepción, su orden (la pestaña cambia y abre el documento).
- **Proveedores**: listado (activos por omisión; búsqueda por nombre, razón social, RFC, contacto, teléfono o correo;
  estado; insumo surtido; paginación) con RFC, contacto, insumos y última recepción. Alta y edición con
  `inventory.suppliers.manage` (nombre comercial, razón social, RFC opcional que valida y normaliza Go, teléfono, correo,
  domicilio de varias líneas, contacto, observaciones); desactivar con el interruptor «Proveedor activo» y confirmación
  de peligro que explica que se conservan sus compras (no hay borrado). Insumos que surte: presentación habitual, código
  del proveedor, preferido (uno por insumo) y último precio de Go (sin IVA, antes del descuento, con su presentación,
  fecha y recepción); agregar, editar y quitar (con confirmación). Historial en un acordeón con los conteos y totales de
  Go y las órdenes, recepciones y devoluciones recientes.
- **Compras**: listado con búsqueda (folio o proveedor), estado (incluye «Abiertas»), proveedor y fechas, paginado.
  Editor del borrador con `inventory.purchases.create`: proveedor, fecha (vacía = hoy), almacén previsto, renglones
  (insumo, unidad base, otra unidad de su dimensión o presentación, cantidad, precio sin IVA hasta 6 decimales,
  descuento en pesos y tasa con los botones «IVA 16 %», «IVA 8 %» e «IVA 0 %») y observaciones. Al elegir el insumo se
  propone la presentación habitual con el proveedor (o la predeterminada de compra) y se muestra su último precio.
  **Los importes y totales son los que devuelve Go al guardar**: antes de guardar se indica que los calcula el servidor,
  y con cambios sin guardar se aclara que los totales son los del borrador guardado. Acciones según estado y permiso
  (`inventory.purchases.authorize`): Emitir (confirmación con el total de Go; exige guardar antes), Cancelar y Cerrar
  administrativamente (motivo de 5 a 500 caracteres y confirmación de peligro). Detalle con solicitado, recibido,
  pendiente y excedente por renglón, totales, observaciones y sus recepciones.
- **Recepciones**: listado con búsqueda (folio, referencia o proveedor), tipo (de orden o directa), almacén y fechas de
  negocio; columnas de origen, almacén, responsable, total (con «IVA en el costo» o «fuera del costo»), valor de
  inventario solo con `inventory.costs.read` y estado del vínculo con gastos.
  - _Recibir una orden_ (desde la orden en Compras o eligiendo una orden abierta en Recepciones): por renglón muestra
    solicitado, recibido antes y pendiente y **propone lo pendiente tal como lo entrega Go**; se capturan la cantidad,
    el precio real (por omisión el de la orden), un descuento propio (vacío: el prorrateo de Go) y la tasa; almacén
    (por omisión el previsto), referencia del proveedor y observaciones. Si Go responde `RECEIPT_EXCEEDS_PENDING`, un
    diálogo de peligro muestra su mensaje y solo con «Registrar con excedente» se reenvía con `allow_excess: true`; el
    aviso `RECEIPT_EXCESS` queda en el detalle.
  - _Recepción directa_: una sola pantalla con proveedor obligatorio, almacén, referencia, observaciones y los mismos
    renglones que la orden (exige además `inventory.purchases.create`).
  - _Detalle_: proveedor, fecha, almacén, responsable, referencia, origen; cantidades (solicitado, recibido antes,
    recibido con su equivalente en unidad base, pendiente después, devuelto y existencia después); importes por renglón
    y costo recibido por unidad base solo con `inventory.costs.read`; totales; valor de inventario, devuelto y vinculado.
  - _Devolución a proveedor_ (`inventory.supplier_returns.record`): renglones con lo devolvible de Go, cantidad por
    devolver, almacén de salida y motivo; confirmación de peligro que aclara que no crea ingresos de efectivo.
  - _Vínculo con gastos de caja_ (`inventory.purchases.link_cash` + `cash.movements.read`): total, vinculado y por
    vincular de Go; vínculos con su estado (Vigente, «Gasto corregido después del vínculo», «Gasto regularizado después
    del vínculo», Desvinculado) y aviso cuando `cash_link_attention`; búsqueda de gastos candidatos por fechas e importe
    mínimo y máximo (con `receipt_id`), donde Go indica cuáles se pueden vincular y por qué no los demás (en dólares,
    corregido, vinculado por completo); vínculo total o parcial con el importe capturado o copiado de «Disponible del
    gasto» o «Por vincular de la recepción»; los rechazos de Go (`CASH_EXPENSE_OVERLINKED`, `RECEIPT_OVERLINKED`,
    `CASH_EXPENSE_CURRENCY`, `CASH_EXPENSE_CORRECTED`…) se muestran con su mensaje; desvincular con motivo y
    confirmación. Sin los permisos, el vínculo se consulta y se explica qué permiso falta.

### Decisiones y desviaciones menores

1. **Totales sin vista previa**: Go no tiene una vista previa de órdenes; el editor muestra los importes del borrador
   guardado (cada renglón y el documento) y no calcula nada mientras se edita. Para recalcular se guarda el borrador.
2. **Exceso**: no se compara en React lo capturado con lo pendiente; se envía sin `allow_excess` y, ante
   `RECEIPT_EXCEEDS_PENDING`, se pide la confirmación explícita y se reenvía con `allow_excess: true` (otro
   `operation_id`, porque el primer intento fue rechazado sin efectos).
3. **Recepción de una orden**: el precio y la tasa solo viajan si se cambiaron respecto de la orden; el descuento solo
   si se capturó (vacío = prorrateo acumulado de Go); los renglones con cantidad vacía o cero no se envían.
4. **Resultado incierto**: el reintento usa el mecanismo existente (`useInventoryMutation`, mismo `operation_id`, aviso
   «Reintentar con el mismo identificador» de la sección). Además, los formularios de recepción, devolución y vínculo
   quedan bloqueados tras un resultado incierto, para que no se vuelvan a enviar con otro identificador aun después de
   recuperarlo: hay que volver al listado y consultar el documento.
5. **Importe del vínculo**: sin aritmética; botones que copian «Disponible del gasto» o «Por vincular de la recepción»
   tal como los entrega Go; Go valida ambos límites.
6. **Tasa por omisión 16 %** en renglones nuevos (como el sistema anterior); la de la orden en la recepción.
7. **Folios legibles** «Orden 12», «Recepción 5», «Devolución 3» (Go nombra igual las recepciones en los movimientos).
8. **Fecha de recepción y devolución**: se registran con la hora del servidor (no se expone `occurred_at`).
9. **Filtro de proveedores**: activos por omisión (se pueden ver inactivos o todos).
10. **Selector de proveedor**: búsqueda + lista de hasta 50 coincidencias (como el selector de insumos).
11. **Pruebas de Playwright existentes**: se agregaron Compras, Recepciones y Proveedores a la lista de pestañas que
    esperan `inventory.spec.ts` y `stock-consumption.spec.ts` (el administrador tiene todos los permisos); nada más cambió
    en ellas.

### CSS añadido (`styles.css`, al final del bloque «Etapa 8: Inventarios»)

`tag-info` y `tag-muted` (tonos de estado), `purchase-line-grid` (rejilla de renglón que se reacomoda), `purchase-tax-field`
(botones de IVA sin desbordar), `purchase-line-amounts` y `purchase-totals`/`purchase-total-amount` (importes de Go),
`purchase-actions` y `danger-solid-button` en Inventarios (≥ 44 px), botón dentro de `inventory-facts` (≥ 44 px) y
`min-width: 0` en los elementos de las rejillas de Inventarios (`.inventory-page .form-stack > *`, `.inventory-page .panel`)
para que las tablas anchas (recepciones, renglones de órdenes) se desplacen en su contenedor en lugar de ensanchar la
página a 1280 × 720. Solo tokens de ambos temas.

### Limitaciones

- Sin vista previa de totales antes de guardar el borrador (Go no la ofrece).
- Un borrador o un proveedor cuyo resultado quedó incierto y se recuperó podría volver a crearse si se envía de nuevo
  desde el mismo formulario (los de recepción, devolución y vínculo sí se bloquean); un borrador repetido se cancela.
- La recepción y la devolución usan la hora del servidor; no se captura una fecha anterior.
- La búsqueda de gastos no filtra por caja (Go lo admite con `register_id`); muestra las cajas que la sesión puede leer.
- Sugerencias de compra, alertas y el resumen completo son de la fase D.
- Sin recorrido en la ventana nativa (WKWebView) ni en pantalla táctil física; las capturas de página completa muestran
  la barra fija de acciones y el botón de pantalla completa a media página (artefacto de la captura).

### Pruebas (C2)

- **Vitest** (`inventoryPurchasing.test.tsx`, 21 pruebas): pestañas con cualquiera de los permisos de compras; alta de
  proveedor sin RFC (cuerpo exacto) e insumo surtido (revisión 0, presentación, código, preferido; último precio
  mostrado); desactivación solo tras confirmar (`active: false` con revisión; sin botón de borrar); consulta sin
  `suppliers.manage`; historial que abre la orden en Compras; borrador con renglón, presentación habitual propuesta,
  último precio, IVA 8 % por botón y cuerpo exacto, **totales y renglón mostrados tal como llegan de Go** (cifras sin
  relación aritmética con lo capturado) y emisión con confirmación; sin `purchases.authorize` no hay emitir, cancelar ni
  cerrar; cierre y cancelación con motivo y confirmación de peligro; recepción parcial 12 de 20 (lo pendiente propuesto,
  cuerpo sin precio ni tasa, pendiente 8, costo recibido) y total (8 pendientes propuestos, sin `allow_excess`); exceso
  con confirmación que reenvía con `allow_excess`; resultado incierto que bloquea el formulario y se recupera con el
  mismo `operation_id`; recepción directa (proveedor obligatorio, cuerpo exacto) y sin `purchases.create` no se ofrece;
  devolución (cantidad, motivo, confirmación de peligro, cuerpo exacto); vínculo con gasto (candidatos con
  `receipt_id`, gasto en dólares no elegible con la explicación de Go, filtro de importe, rechazo de Go mostrado,
  vínculo parcial con cuerpo exacto); gasto corregido después del vínculo señalado y desvinculación con motivo; sin
  `cash.movements.read` no se ofrece vincular; costos ocultos sin `inventory.costs.read`; ningún `type="number"`.
- **Playwright** (`vendor-purchases.spec.ts`, Chromium y WebKit, con su propio proveedor e insumos preparados por la
  API): pestañas; proveedor con RFC normalizado por Go e insumo surtido; orden de 20 cajas a $250 con $50 de descuento
  ($4,950.00 + $792.00 = $5,742.00 de Go), emisión; recepción de 12 (pendiente 8, «Parcialmente recibida», $3,445.20,
  $11.9625 por pieza) y de 8 (pendiente 0, «Recibida», $2,296.80, dos recepciones); existencia 480 pza y dos movimientos
  «Entrada por compra» (+288 y +192), confirmados por la API; recepción directa de 50 vasos ($116.00); vínculo total con
  un gasto de $116.00 registrado por la API en un turno abierto de Caja 1 (vigente, por vincular $0.00; Go lo confirma
  como no vinculable después); sin desbordamiento a 1920 × 1080 y 1280 × 720 en ambos temas en cada pantalla.

### Verificación (C2)

En la copia aislada `scratchpad/repoC2` (sincronizada desde el árbol de trabajo, con el trabajo en curso de la fase D
del otro agente, incluida su `0014`), con el clúster temporal 55499, Go temporal en 8184 y vista previa en 1424:

- `npx vitest run src`: 45 archivos, 386 pruebas aprobadas (Inventarios: 5 archivos, 60 pruebas).
- `npx tsc --noEmit`: sin errores. `npm run build`: correcto.
- `prettier --check` de los archivos propios: sin diferencias.
- Playwright `vendor-purchases.spec.ts`: Chromium 1 aprobada, WebKit 1 aprobada.
- Playwright recorrido completo (sin `SPEC`): Chromium 5 aprobadas (2.7 min) y WebKit 5 aprobadas (2.6 min).
- Capturas (claro y oscuro, 1280 × 720) en `scratchpad/shots8C2/` (Chromium) y `scratchpad/shots8C2/webkit/`.
