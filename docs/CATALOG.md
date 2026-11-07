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

## Disponibilidad, imágenes y concurrencia

Fechas inclusivas, días domingo 0 a sábado 6 e intervalos `[inicio, fin)` en hora del establecimiento. Vacío significa sin restricciones. `24:00` se admite como fin; para cruzar medianoche crear intervalos separados. No se acepta un reloj enviado por el cliente.

Producto o categoría inactivos, producto archivado o presentación inactiva no se pueden seleccionar, aun con excepción. Ingredientes, opciones, complementos y porciones inactivos con cantidad solicitada positiva impiden el cálculo. Los hijos se verifican recursivamente. La excepción de disponibilidad exige permiso propio y motivo; omite horarios y pausa temporal, queda auditada y no vuelve activos registros desactivados.

Fotografías opcionales PNG/JPEG: máximo 1 MiB y 2048 × 2048. Go verifica base64, cabecera, dimensiones y decodificación completa; guarda bytes y checksum en PostgreSQL, sin rutas locales suministradas por el cliente. La consulta requiere autenticación; el puente entrega una URI `data:`. La auditoría guarda metadatos, nunca la fotografía completa.

Cada edición envía el documento completo y su `revision`. Una revisión obsoleta devuelve HTTP 409 `REVISION_CONFLICT`; el editor conserva el borrador para comparar y recargar deliberadamente. Un lock de catálogo compartido para consultas/cálculos y exclusivo para cambios mantiene coherentes las relaciones y evita carreras al validar ciclos. Escritura y auditoría confirman en la misma transacción. Las denegaciones siguen la auditoría de seguridad de etapa 2. La consulta de auditoría también omite valores monetarios del catálogo si el actor no tiene permiso de precios.

Límites operativos actuales: 20 líneas/1000 unidades por previsualización, ocho niveles y 1000 nodos de expansión, 200 productos cargados por selección; 100 presentaciones/ingredientes y 30 grupos/componentes por producto; 100 opciones por grupo, 30 alternativas por componente, 100 tramos; listas hasta 5000 entradas por edición. Los selectores cargan hasta 5000 productos y las respuestas nativas tienen límite de 2 MiB. No se ha validado carga comercial de gran catálogo o muchas terminales; antes de superar esos límites, implementar selectores remotos y medición de rendimiento.

## Marca

Los originales están en `assets/brand/adel-pos-tema-claro.png` y `adel-pos-tema-oscuro.png`. `BrandLogo` los incorpora localmente en acceso y navegación, cambia por tema, conserva transparencia/proporción y usa el icono compacto en espacios estrechos. No se modificaron los PNG originales ni se confunden con el logotipo futuro del restaurante.
