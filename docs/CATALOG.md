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

Cada diferencia de grupo es `máximo(0, valor solicitado − valor originalmente incluido)`. Los dos valores se calculan con cantidades y valores de referencia vigentes de las opciones. El factor físico de porción queda como metadato para la futura receta: no multiplica automáticamente los importes configurados por porción.

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

El motor entrega un árbol de componentes con UUID, revisión y selección, apto para preparar consumo por componentes en otra etapa. No descuenta inventario. Se rechazan ciclos al guardar, incluso entre alternativas, y se limitan profundidad y expansión del cálculo.

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

## Disponibilidad, imágenes y concurrencia

Fechas inclusivas, días domingo 0 a sábado 6 e intervalos `[inicio, fin)` en hora del establecimiento. Vacío significa sin restricciones. `24:00` se admite como fin; para cruzar medianoche crear intervalos separados. No se acepta un reloj enviado por el cliente.

Producto o categoría inactivos, producto archivado o presentación inactiva no se pueden seleccionar, aun con excepción. Ingredientes, opciones, complementos y porciones inactivos con cantidad solicitada positiva impiden el cálculo. Los hijos se verifican recursivamente. La excepción de disponibilidad exige permiso propio y motivo; omite horarios y pausa temporal, queda auditada y no vuelve activos registros desactivados.

Fotografías opcionales PNG/JPEG: máximo 1 MiB y 2048 × 2048. Go verifica base64, cabecera, dimensiones y decodificación completa; guarda bytes y checksum en PostgreSQL, sin rutas locales suministradas por el cliente. La consulta requiere autenticación; el puente entrega una URI `data:`. La auditoría guarda metadatos, nunca la fotografía completa.

Cada edición envía el documento completo y su `revision`. Una revisión obsoleta devuelve HTTP 409 `REVISION_CONFLICT`; el editor conserva el borrador para comparar y recargar deliberadamente. Un lock de catálogo compartido para consultas/cálculos y exclusivo para cambios mantiene coherentes las relaciones y evita carreras al validar ciclos. Escritura y auditoría confirman en la misma transacción. Las denegaciones siguen la auditoría de seguridad de etapa 2. La consulta de auditoría también omite valores monetarios del catálogo si el actor no tiene permiso de precios.

Límites operativos actuales: 20 líneas/1000 unidades por previsualización, ocho niveles y 1000 nodos de expansión, 200 productos cargados por selección; 100 presentaciones/ingredientes y 30 grupos/componentes por producto; 100 opciones por grupo, 30 alternativas por componente, 100 tramos; listas hasta 5000 entradas por edición. Los selectores cargan hasta 5000 productos y las respuestas nativas tienen límite de 2 MiB. No se ha validado carga comercial de gran catálogo o muchas terminales; antes de superar esos límites, implementar selectores remotos y medición de rendimiento.

## Marca

Los originales están en `assets/brand/adel-pos-tema-claro.png` y `adel-pos-tema-oscuro.png`. `BrandLogo` los incorpora localmente en acceso y navegación, cambia por tema, conserva transparencia/proporción y usa el icono compacto en espacios estrechos. No se modificaron los PNG originales ni se confunden con el logotipo futuro del restaurante.
