# Catálogo, personalización y precios · Etapa 3

El catálogo se administra desde **Catálogo**. Las pestañas contienen categorías, productos independientes, combos, ingredientes, complementos, porciones, grupos reutilizables, listas y promociones. Todas las escrituras pasan por Go y PostgreSQL. No existen pedidos, cobros ni movimientos de inventario en esta etapa.

## Primer catálogo

1. Crear una categoría. Su estado y sus horarios condicionan la disponibilidad de sus productos.
2. Crear ingredientes, complementos y porciones si se necesitan. Un complemento puede referenciar un producto o ingrediente existente; esa relación también permite verificar su disponibilidad.
3. Crear grupos reutilizables y sus opciones. Cada opción identifica complemento y porción; el valor de referencia y el extra cobrado se configuran por separado.
4. Crear un producto independiente con código único y precio base. Presentaciones, fotografía, ingredientes y grupos son opcionales. Asignar las cantidades incluidas de cada grupo.
5. Abrir **Probar personalización**, seleccionar el producto y modificar las cantidades. El desglose se actualiza desde el servidor y no registra una venta.

Los controles para activar, archivar y pausar productos tienen permisos propios. Las categorías se reordenan mediante su orden visual o la flecha del listado. **Ver productos** aplica el filtro de categoría. Los listados tienen búsqueda, estado y paginación; productos añade categoría, tipo y archivo. No hay borrado físico de productos, presentaciones u opciones referenciadas.

Duplicar conserva la configuración y las referencias compartidas, crea nuevos UUID para el producto, presentaciones y componentes, exige nombre/código nuevos y vacía códigos de barras. Las presentaciones duplicadas reciben códigos `VAR-<uuid>`, editables después. No se copian productos hijos del combo.

## Dinero y redondeo

PostgreSQL utiliza `numeric`; los contratos transportan importes como cadenas decimales. `internal/catalog` usa `shopspring/decimal`, sin `float32`, `float64` ni precios calculados en JavaScript. Los importes de entrada son no negativos, hasta diez dígitos enteros y dos decimales; las cantidades permiten tres decimales, de 0 a 1000. Se rechaza precisión adicional, signos y notación científica.

Redondeo comercial a dos decimales, mitad alejándose de cero: `0.005 → 0.01`. Se redondean el extra de cada ingrediente, la diferencia y extras de cada grupo, los componentes y el descuento asignado a cada línea por promoción. El precio unitario ya redondeado se multiplica por unidades enteras. No se añaden impuestos, cargos manuales ni descuentos generales de cuenta.

### Equivalencia por valor

`Precio unitario = base + ingredientes adicionales + suma(diferencia de grupo + extras explícitos) + cargos de componentes`.

Cada diferencia de grupo es `máximo(0, valor solicitado − valor originalmente incluido)`, salvo en los productos con la opción «El precio puede bajar al cambiar complementos» (ver [más abajo](#precio-que-puede-bajar-al-cambiar-complementos--etapa-9a)). Los dos valores se calculan con cantidades y valores de referencia vigentes de las opciones. El factor físico de porción queda como metadato para la futura receta: no multiplica automáticamente los importes configurados por porción. Desde la etapa 8, el inventario lo usa como multiplicador del consumo de la opción (sigue sin intervenir en el precio).

| Configuración del filete de $150             | Valor original | Solicitado | Diferencia | Resultado |
| -------------------------------------------- | -------------- | ---------- | ---------- | --------- |
| Arroz $20 + ensalada $15                     | $35            | $35        | $0         | $150      |
| Papas $25 + ensalada $15                     | $35            | $40        | $5         | $155      |
| Retirar todo                                 | $35            | $0         | $0         | $150      |
| Reemplazar todo por papas $25 + frijoles $10 | $35            | $35        | $0         | $150      |
| Dos porciones de papas $25                   | $35            | $50        | $15        | $165      |

El mínimo predeterminado de grupos es cero. El administrador puede establecer límites específicos por grupo y sobreescribirlos por producto. Opciones omitidas en una selección explícita significan cantidad cero; un grupo omitido utiliza su configuración original. Esto permite retirar o sustituir todas las guarniciones sin dejar obligatoria una opción original.

El extra explícito por opción se cobra sobre `máximo(0, cantidad solicitada de esa opción − cantidad incluida de esa opción)`. Es independiente de la equivalencia; usar cero cuando todo el cargo deba resultar de la equivalencia. Nunca se cobra nuevamente el extra de unidades ya incluidas. Los ingredientes usan la misma diferencia positiva multiplicada por su cargo explícito, con sus propias reglas removible/agregable/máximo. Las observaciones no generan cargos.

Los grupos de tipo **cantidad** cobran sus adicionales mediante tramos explícitos en lugar de equivalencia por valor. Requieren unidades adicionales enteras. Sin tramos no se inventa un precio para un adicional; Go rechaza esa selección. El desglose conserva de todas formas los valores original y solicitado.

### Ejemplo reproducible de combo $90 / $100 / $140

Crear hamburguesa de $90 y papas de $40 como productos independientes. Crear un combo de precio base $90 con dos componentes:

- Principal: producto hamburguesa, mínimo/máximo/incluido/original = 1, cargo de sustitución = 0.
- Papas: producto papas existente, mínimo/original/incluido = 0, máximo = 2. Alternativa papas con cargo de sustitución = 0 y tramos: **desde 1, $10; desde 2, $40**.

En previsualización, seleccionar 0, 1 y 2 papas produce $90, $100 y $140. Los tramos son incrementales: no significan precio acumulado ni tarifa uniforme. La tercera unidad, si se aumenta el máximo, utiliza el último tramo vigente.

Las unidades cubiertas por el combo cobran únicamente su cargo explícito de sustitución; las adicionales usan tramos. La personalización de un hijo agrega `precio personalizado del hijo − precio base efectivo del hijo`, por cada unidad, sin sumar otra vez su precio de venta individual. La lista elegida también valida el precio del hijo: con respaldo `reject`, este debe tener entrada explícita aunque su base esté cubierta por el combo.

El motor entrega un árbol de componentes con UUID, revisión y selección, apto para preparar consumo por componentes en otra etapa. El catálogo no descuenta inventario: desde la etapa 8 lo hace el inventario a partir de la partida liberada a cocina (ver «Inventario · etapa 8»). Se rechazan ciclos al guardar, incluso entre alternativas, y se limitan profundidad y expansión del cálculo.

## Listas y promociones

Siempre existe una lista activa predeterminada; inicialmente **Restaurante** con respaldo al precio base. Seleccionar otra predeterminada actualiza la revisión y audita también la lista desplazada. Para retirar la actual, primero elegir otra.

Una entrada corresponde exactamente a producto base o a una presentación. Una entrada del producto base no sustituye implícitamente el precio de una presentación. Si falta la entrada exacta: `base` usa el precio propio del producto/presentación; `reject` rechaza la previsualización. Se muestra el origen del importe. No hay integraciones con plataformas externas.

Promociones iniciales: 2×1, porcentaje, compra/paga por cantidad y precio especial por unidad. Sus productos y categorías forman una unión: basta coincidir con uno. Se evalúan fechas, días, horarios y mínimo de unidades con el reloj de Go y la zona IANA del restaurante.

- Prioridad mayor primero; empate por UUID ascendente.
- Sin listas seleccionadas, la promoción solo participa en la lista predeterminada. Otras listas requieren inclusión explícita, y cada lista puede excluir todas las promociones.
- Por defecto se descuentan bases. **Incluir extras** permite descontar también la personalización del producto raíz. No se aplican promociones independientes a productos hijos ya cubiertos por un combo.
- 2×1 bonifica `floor(unidades / 2)` unidades: dos y tres unidades de $150 descuentan $150. Compra/paga bonifica `floor(unidades / compra) × (compra − paga)`. En mezclas se bonifican las unidades elegibles más baratas; empate por posición de línea.
- Una línea ya bonificada queda excluida de otra promoción incompatible. Solo se combinan cuando ambas promociones lo autorizan. La siguiente consume el importe elegible restante después de descuentos anteriores, sin descontar el mismo importe ni generar totales negativos. Ejemplo: 10% y luego 20% sobre $150 → $108.
- Porcentaje redondea por línea. Precio especial descuenta la diferencia positiva entre el importe elegible restante y precio especial × unidades. Nunca aumenta el importe.

El resultado muestra cada promoción aplicada o descartada, motivo e importe descontado. Cambiar la lista, cantidad o personalización invalida visualmente el resultado anterior hasta recibir el cálculo nuevo. Una respuesta atrasada no reemplaza una selección reciente.

## Tasa de IVA por producto · etapa 6.1

Cada producto y cada combo tienen una **tasa de IVA** elegida con botones en su editor: «Tasa general del establecimiento» (predeterminada), **16 %**, **8 %** o **0 %**. No hay captura libre. Go valida la lista cerrada y PostgreSQL la impone (`catalog_products.tax_rate`, `NULL` = hereda la tasa general configurada en Cobros; restricción `catalog_products_tax_rate_check`).

- Los precios ya incluyen IVA: la tasa solo decide cómo se desglosa el importe. El motor de precios no cambia.
- Cambiar la tasa de un producto exige `catalog.prices.manage` (como el precio) y queda auditado con el guardado del producto (`catalog.products.updated`, estado anterior y nuevo). Sin ese permiso los botones se muestran desactivados y Go rechaza el cambio con 403; los demás cambios del producto siguen permitidos.
- Duplicar conserva la tasa. Cambiar la tasa incrementa la revisión del producto, por lo que un borrador abierto recibe `QUOTE_CHANGED` al confirmarse y pide revisar el desglose.
- La tasa solo afecta lo que se confirme después: cada versión de partida guarda su tasa efectiva al confirmarse (ver [PAYMENTS_AND_REFUNDS.md](PAYMENTS_AND_REFUNDS.md)). Un combo aplica su propia tasa a todo su importe, aunque sus componentes tengan otra.

## Precio que puede bajar al cambiar complementos · etapa 9A

Cada producto y cada combo tienen la opción **«El precio puede bajar al cambiar complementos»** (`catalog_products.price_can_decrease`, migración 0015), **desactivada por omisión**: con ella desactivada rige exactamente la equivalencia con piso de arriba (sustituir por un complemento más barato o quitarlo sin reemplazo nunca baja el precio) y la cotización es idéntica, byte a byte, a la anterior a la 9A. Todos los productos existentes quedaron desactivados.

- **Editor**: interruptor junto al precio y la tasa de IVA, con su explicación. Cambiarlo exige `catalog.prices.manage` (Go responde 403 sin él; el interruptor se muestra desactivado) y queda auditado con el guardado (`catalog.products.updated`). Duplicar conserva la opción. Cambiarla incrementa la revisión del producto: un borrador abierto recibe `QUOTE_CHANGED` al confirmarse.
- **Diferencia con signo por grupo**: en modo valor, `valor solicitado − valor incluido` (quitar sin reemplazo también resta); en modo cantidad, los tramos de las porciones adicionales más `min(0, valor solicitado − valor incluido)` (solo baja por una diferencia negativa de valor: sustituir por un complemento más caro nunca baja el precio; sin valores de referencia no cambia nada). Se redondea por grupo, como siempre.
- **Núcleo del precio**: `base + Σ diferencias con signo` (en un combo, más las reducciones de sus componentes), limitado a: nunca menos de cero; nunca menos que el valor de referencia de los complementos que permanecen seleccionados (los propios y, en un combo, los de sus componentes); y nunca más que con la regla con piso (la opción solo permite bajar). Después se suman los **extras explícitos**, los **ingredientes adicionales** y los **cargos de componentes**, que nunca se descuentan.
- Ejemplo del propietario: filete de $150 con arroz $20 + ensalada $15 incluidos ($35); elegir ensalada + frijoles ($25) da −$10 y **$140**; quitar la ensalada sin reemplazo da **$135**; papas + ensalada sigue en **$155**.
- **Presentaciones y listas**: el ajuste parte del precio efectivo (presentación o lista); el límite protege listas con precios bajos.
- **Combos**: la reducción de los complementos de un componente solo aplica si **el combo también** tiene la opción (si no, el componente se calcula con el piso aunque su producto la tenga). La reducción de cada componente se acumula con signo, sin acotarla a su precio suelto, y los límites se aplican una sola vez en el núcleo del combo; no rebaja el cargo del componente (sustitución, tramos y extras). Las sustituciones de ranura no son complementos y no cambian.
- **Promociones**: con la opción, la base sin extras de la promoción es el precio base ya reducido, `(base + complement_adjustment) × unidades` (nunca negativa); con «Incluir extras», el subtotal ya reducido. Ejemplos con el filete a $140: 2×1 de dos unidades descuenta $140 (no $150); 10 % descuenta $14.00; precio especial de $100 descuenta $40. Orden garantizado: complementos (catálogo) → promociones → descuentos manuales de Cobros, que actúan sobre lo que se debe.
- **IVA**: sin regla nueva; la tasa efectiva del producto (o del combo) se aplica al importe reducido.
- **Cotización** (`quote_line`): solo con la opción aplicada agrega `price_can_decrease: true` y `complement_adjustment` (reducción respecto del piso, con signo: `"-10.00"`, o `"0.00"` si no bajó), ya incluida en `group_extras` y `unit_price`; las diferencias de grupo pueden ser negativas y cada grupo agrega `applied_difference`, su efecto real en el precio después de los límites, que es lo que muestra la captura (encabezado del grupo y «Ajuste por complementos»). Sin la opción esos campos no aparecen (`omitempty`), así que las huellas de los borradores guardados no cambian.
- **Historial**: la opción queda congelada en el snapshot del pedido y en la cotización de cada partida al confirmar; cambiarla después no altera partidas confirmadas. Como con un cambio de precio, una modificación que vuelve a cotizar una partida (más cantidad o cambio de personalización) usa la regla vigente; cambiar solo observaciones conserva los importes.
- **Captura y desglose**: el grupo muestra el cambio que calcula Go con signo («+$5.00», «−$10.00» o «Sin cambio»); el desglose rotula «Diferencia» (ya no «Diferencia positiva») y, con la opción, indica la reducción aplicada. La cocina no recibe ninguno de estos importes. El inventario consume según las selecciones, no según el importe.

## Reportes · etapa 9A

Los reportes de ventas y productos ([REPORTS.md](REPORTS.md)) no vuelven a cotizar nada: leen los importes que cada unidad comercial tenía al cerrarse su cuenta (bruto, promoción y neto, ya con la reducción de la opción anterior si se aplicó), copiados al libro de reconocimiento con el nombre, el código, el tipo, la variante, la categoría y la lista de precios de ese momento. Renombrar un producto o cambiar su precio después no cambia los reportes. Un combo es la unidad vendida; sus componentes solo aparecen como cantidades preparadas, sin importe, y los complementos seleccionados se reportan desde la cotización congelada de la partida. «Productos menos vendidos» agrega con cero los productos activos sin ventas del periodo.

## Indicaciones rápidas por categoría · etapa 7.1

Cada categoría puede tener una lista ordenada de **indicaciones rápidas para cocina** sin precio (por ejemplo, Hamburguesas: «Sin cebolla», «Bien cocida»; Bebidas: «Sin hielo»), en `app.catalog_category_quick_notes` (migración 0009). Completa la propuesta de [ETAPA_6_1_AVANCE.md](ETAPA_6_1_AVANCE.md) por categoría; las indicaciones propias y exclusiones por producto quedan fuera (desde la etapa 8 la partida guarda además el identificador de cada indicación elegida; ver «Indicaciones con efecto»).

- **Administración**: en Catálogo → Categorías, al editar una categoría guardada, sección «Indicaciones rápidas para cocina». Requiere `catalog.categories.manage`; cada indicación se guarda por separado (`POST catalog/quick-notes`, `PATCH catalog/quick-notes/{id}`) con su revisión optimista, orden visual por posiciones dentro de su categoría (`internal/ordering`), activación y auditoría en la misma transacción (`catalog.quick-notes.created`, `.updated`, `.reordered`). No se borran: se desactivan. Hasta 40 por categoría.
- **Texto**: de 1 a 80 caracteres, sin comas (las indicaciones de una partida se guardan unidas por comas), sin saltos de línea ni caracteres de control y sin espacios al inicio o al final; dos indicaciones de la misma categoría no pueden diferir solo en mayúsculas. Go y PostgreSQL (CHECK e índice único con nombre) aplican las mismas reglas.
- **Captura**: `GET catalog/lookups` entrega `category_quick_notes` (la captura ya carga los lookups una vez; no se agrega ninguna petición por toque). Al personalizar un producto, la sección «Indicaciones para cocina» muestra primero las indicaciones activas de su categoría, en su orden, como botones que se marcan y desmarcan en su lugar; después, las demás indicaciones de la partida, los atajos de las ya usadas en la cuenta (sin repetir ninguna) y «Escribir otra». Cada componente de un combo ofrece las de la categoría de su propio producto.
- **Pedido y cocina**: la indicación se guarda como texto en `customization.notes`, igual que las escritas; llega a la comanda como «NOTA:» sin cambios en cocina. Editar o desactivar una indicación no cambia las partidas que ya la usan.

### Indicaciones con efecto · etapa 8 (migración 0012)

Una indicación puede tener un **efecto** sobre un ingrediente del catálogo: `none` (solo instrucción de cocina, el valor de todas las existentes), `omit` (omitir), `normal` (porción habitual) o `extra` (porción extra). El efecto se elige con botones («Solo instrucción», «Omitir», «Porción habitual», «Porción extra») y el ingrediente con el selector de ingredientes del catálogo, en el mismo editor de la categoría; no hay campos de gramos. Nunca se deduce del texto: «Sin cebolla» sin efecto configurado sigue siendo solo una instrucción.

- **Una sola selección**: una indicación con efecto no es un segundo mecanismo. Al tocarla, la captura aplica la **misma selección estructurada** que el personalizador (`customization.ingredients`) y guarda su identificador en `customization.quick_notes` (`[{note_id, text}]`, en cada nivel: producto y componentes de combo); su texto sigue también en `notes`. El precio (y el consumo de inventario) salen solo de los ingredientes: «Queso extra» cobra exactamente lo mismo que el botón «Extra queso», porque fija la misma cantidad, y nunca se acumulan multiplicadores.
- **Cantidad que fija cada efecto** (la misma regla en Go, `catalog.QuickNoteTarget`, y en la captura): `omit` → 0 (aplica si el producto incluye el ingrediente y es removible); `normal` → lo incluido («Con cebolla» en un producto que ya la lleva no cambia nada) o 1 si no estaba incluido (aplica si es agregable con máximo de al menos 1); `extra` → `min(incluido + 1, máximo)`, igual que el botón «Extra» (aplica si es agregable y su máximo supera lo incluido).
- **Validación en Go** al cotizar, confirmar y modificar (`catalog/quick_note_effects.go`), 422 en español: la indicación existe, está activa y pertenece a la categoría del producto (o del componente); con efecto, su ingrediente está en las reglas del producto y el efecto es aplicable; dos indicaciones con efecto sobre el mismo ingrediente se rechazan (`QUICK_NOTE_CONFLICT`); y la cantidad solicitada del ingrediente coincide exactamente con la que fija el efecto (`QUICK_NOTE_INVALID`). El borrador solo revisa la forma (UUID, sin repetir, hasta 40 por nivel, texto de 1 a 80 caracteres).
- **Congeladas en la partida**: el desglose (`quote_line`) guarda las indicaciones resueltas (`note_id`, texto, efecto e ingrediente) y el snapshot del pedido guarda solo las indicaciones usadas. Editar o desactivar la indicación después no cambia las partidas confirmadas; una modificación que conserva receta e importes («Preserve») conserva las ya resueltas y valida las nuevas con las reglas del snapshot de su pedido.
- **Compatibilidad**: `quick_notes` se omite cuando está vacío, así que los borradores, solicitudes, desgloses, snapshots y huellas anteriores no cambian. Las partidas antiguas tienen solo texto y nunca tienen efecto.
- **Captura**: se muestran las indicaciones sin efecto (como en la 7.1) y las con efecto que aplican a los ingredientes del producto; una con efecto sobre la cebolla no aparece en un producto sin cebolla (ni su texto como atajo). Dos indicaciones con efecto sobre el mismo ingrediente son excluyentes (tocar una quita la otra) y cambiar ese ingrediente en el personalizador quita la indicación con efecto y su texto.
- **Lookups**: cada elemento de `category_quick_notes` incluye `effect`, `ingredient_id` e `ingredient_name` (nulos sin efecto).

## Inventario · etapa 8

El catálogo no cambió su modelo ni sus precios para el inventario: el inventario **lee** las selecciones estructuradas que la partida ya guarda. Detalle en [INVENTORY_AND_PURCHASING.md](INVENTORY_AND_PURCHASING.md).

- **Porciones**: cada ingrediente y cada opción de grupo de la partida guarda un número decimal de porciones solicitadas (`quote_line`); el consumo es ese número × la porción habitual del ingrediente («20 g de Cebolla blanca»), que se configura en Inventarios → Recetas → Porciones habituales (o en el renglón de ingrediente de la receta del producto). «Sin», «Con» y «Extra» dan 0, 1 y 2 porciones cuando lo incluido es 1; sin porción configurada no se inventa consumo. No existe un segundo modelo «sin / normal / extra».
- **Complementos**: una opción consume porciones elegidas × factor de la porción × la receta del complemento o, si no tiene, la configuración del producto o la porción del ingrediente al que está ligado. La equivalencia por valor solo afecta el precio: quitar arroz y poner pasta a $140 consume pasta, no arroz.
- **Combos**: su propia receta (si la tiene) y cada componente elegido con su personalización.
- **«Cantidad para receta futura»** (`catalog_product_ingredients.recipe_quantity`): sigue sin uso; el inventario no la lee.
- **Recetas**: se administran en Inventarios → Recetas por producto, variante o complemento; el catálogo no se modifica. La alerta «Producto sin receta» señala los productos activos (no combos ni archivados, de categoría activa) sin receta activa, si el aviso está activado en la configuración de inventario.
- Editar el catálogo después no cambia los consumos registrados: el inventario resuelve con la partida y el snapshot congelados al confirmar y con las recetas vigentes a la hora de la liberación.

## Disponibilidad, imágenes y concurrencia

Fechas inclusivas, días domingo 0 a sábado 6 e intervalos `[inicio, fin)` en hora del establecimiento. Vacío significa sin restricciones. `24:00` se admite como fin; para cruzar medianoche crear intervalos separados. No se acepta un reloj enviado por el cliente.

Producto o categoría inactivos, producto archivado o presentación inactiva no se pueden seleccionar, aun con excepción. Ingredientes, opciones, complementos y porciones inactivos con cantidad solicitada positiva impiden el cálculo. Los hijos se verifican recursivamente. La excepción de disponibilidad exige permiso propio y motivo; omite horarios y pausa temporal, queda auditada y no vuelve activos registros desactivados.

Fotografías opcionales PNG/JPEG: máximo 1 MiB y 2048 × 2048. Go verifica base64, cabecera, dimensiones y decodificación completa; guarda bytes y checksum en PostgreSQL, sin rutas locales suministradas por el cliente. La consulta requiere autenticación; el puente entrega una URI `data:`. La auditoría guarda metadatos, nunca la fotografía completa.

Cada edición envía el documento completo y su `revision`. Una revisión obsoleta devuelve HTTP 409 `REVISION_CONFLICT`; el editor conserva el borrador para comparar y recargar deliberadamente. Un lock de catálogo compartido para consultas/cálculos y exclusivo para cambios mantiene coherentes las relaciones y evita carreras al validar ciclos. Escritura y auditoría confirman en la misma transacción. Las denegaciones siguen la auditoría de seguridad de etapa 2. La consulta de auditoría también omite valores monetarios del catálogo si el actor no tiene permiso de precios.

Límites operativos actuales: 20 líneas/1000 unidades por previsualización, ocho niveles y 1000 nodos de expansión, 200 productos cargados por selección; 100 presentaciones/ingredientes y 30 grupos/componentes por producto; 100 opciones por grupo, 30 alternativas por componente, 100 tramos; listas hasta 5000 entradas por edición. Los selectores cargan hasta 5000 productos y las respuestas nativas tienen límite de 2 MiB. No se ha validado carga comercial de gran catálogo o muchas terminales; antes de superar esos límites, implementar selectores remotos y medición de rendimiento.

## «Agotado» y catálogo descargable · etapa 9G

Fase D de la etapa 9G (decisión D8; contrato en [API_V1.md](API_V1.md) y rutas en
[API_CONTRACTS.md](../API_CONTRACTS.md), «Etapa 9G · API v1 para clientes»). Migración: sección D de la 0023.

- **«Agotado»** es distinto de «Pausar» (`temporary_unavailable`, con `catalog.availability.manage`, que no cambia):
  se marca desde la captura con un toque («Marcar agotados» → «Solo por hoy» o «Hasta nuevo aviso») con el permiso
  nuevo `catalog.sold_out.manage` (solo al administrador; se asigna a los roles desde Personal), por
  `POST catalog/products/{id}/sold-out` con `operation_id` (idempotente con `internal/idempotency`, lock 7820148304
  exclusivo). Se guarda en `catalog_products.sold_out_at`, `sold_out_until` (fin del día comercial, calculado al
  marcarlo con la «Hora de inicio del día»; `NULL` = hasta nuevo aviso) y `sold_out_by`; no cambia la revisión del
  producto. Auditoría `catalog.product.sold_out` y, cuando el corte lo devuelve, `catalog.product.sold_out_reset`
  (autor: proceso; `catalog.SoldOutWorker` revisa cada 30 s, y la lectura ya ignora una marca vencida).
- **Un producto agotado no se puede capturar**: `409 PRODUCT_SOLD_OUT` en la vista previa, el borrador y la apertura
  de una cuenta (lo que se agrega; una partida ya guardada se conserva o se reduce), la confirmación, la edición y el
  pedido sin conexión, también cuando es complemento de otro producto o componente de un combo. Ninguna excepción de
  disponibilidad lo pasa por alto. Los complementos que son ingredientes y las porciones no se agotan desde la
  captura: se desactivan en Catálogo.
- **Tiempo real**: desde la 9G toda escritura correcta del catálogo publica `catalog.changed` (ámbito `catalog`, con
  `catalog.read`; quien vuelva a definir `realtime_events_scope_check` debe conservarlo), así que «Agotado» y cualquier
  cambio llegan al momento a todas las terminales (antes la captura los volvía a consultar cada 15 s).
- **Versión del catálogo**: un número por transacción (`catalog_version_seq`) que anotan disparadores en
  `catalog_change_log` (entidad, id, último número, borrado) para las 19 tablas del catálogo y la tasa general de IVA
  (`payment_settings.tax_rate`); el disparador toma 7820148304 (reentrante). **Época** (`app.catalog_epoch` combinada
  con la base y el clúster): cambia sola al restaurar un respaldo con `restore` o al pasar los datos a otro clúster,
  porque entonces los números se repiten con otro contenido.
- **Catálogo descargable** (`GET catalog/sync`, con `catalog.read` y `catalog.prices.read`): completo (sin archivados) o
  «solo cambios desde X» con `epoch` obligatoria (cambiados, archivados que cambiaron, borrados; una categoría trae sus
  productos; la lista predeterminada o el IVA general traen todos; un producto que cambió trae los combos y grupos que
  lo contienen, con `available_now` y `sold_out` por opción). Con otra época o una versión futura entrega todo con
  `reset: true` y el cliente reemplaza lo guardado. Precios de la lista predeterminada con la misma función del cálculo;
  sin promociones (las aplica el servidor al confirmar). Imágenes aparte: `GET catalog/images/{id}/content` binaria con
  `ETag` y 304.

## Marca

Los originales están en `assets/brand/adel-pos-tema-claro.png` y `adel-pos-tema-oscuro.png`. `BrandLogo` los incorpora localmente en acceso y navegación, cambia por tema, conserva transparencia/proporción y usa el icono compacto en espacios estrechos. No se modificaron los PNG originales ni se confunden con el logotipo futuro del restaurante.
