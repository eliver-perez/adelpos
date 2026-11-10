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
