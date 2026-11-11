# AD-EL POS — ETAPA 8: INVENTARIOS, RECETAS, ALMACENES, COMPRAS Y COSTOS

## OBJETIVO

Continúa el desarrollo de AD-EL POS, sistema comercial de punto de venta para restaurantes.

Tecnologías aprobadas:

- Backend: Go.
- Base de datos: PostgreSQL.
- Frontend: React, TypeScript y Tailwind CSS.
- Aplicación de escritorio: Tauri 2.
- Eventos en tiempo real: SSE autenticado.

Las etapas 1 a 7 están implementadas, incluyendo las ampliaciones 6.1, 6.2 y 7.1.

La versión actual de referencia es **0.7.1, esquema PostgreSQL 9**.

Tu tarea es desarrollar la etapa 8 completa: **inventarios, insumos, recetas, subrecetas, almacenes, consumo por preparación, movimientos, mermas, conteos físicos, proveedores, compras, recepciones y costos**.

La prioridad es mantener un sistema de inventario preciso y auditable, pero sencillo de utilizar en un restaurante real.

**La administración de inventarios no debe complicar ni ralentizar la captura de pedidos.**

No reconstruyas módulos anteriores ni introduzcas otra arquitectura. Debes integrar el inventario con los sistemas existentes de catálogo, pedidos, cocina y caja.

Al terminar, la versión será **0.8.0**, con el esquema que resulte de las migraciones nuevas.

---

## 0. FORMA DE TRABAJO

### Rama

Trabaja en una rama nueva **`etapa-8`** creada desde `main` (que contiene hasta la 7.1). No hagas push ni merge.

### Fase 0: verificación contra el código (obligatoria, antes de diseñar)

Esta especificación se escribió sin ver el código. Antes de implementar, comprueba en el repositorio y registra en `docs/ETAPA_8_AVANCE.md`:

1. **Modelo actual de ingredientes, porciones y complementos del catálogo (etapa 3).** El catálogo ya tiene ingredientes, porciones, grupos y cantidades de porciones, equivalencia por valor y tramos incrementales. Describe cómo se guarda en la partida confirmada lo que el cliente quitó, agregó o sustituyó. **El inventario debe leer exactamente esas selecciones estructuradas; no crear un segundo modelo de «sin / normal / extra» paralelo al del catálogo.**
2. **La liberación canónica de cocina** (`kitchen_unit_releases`): qué datos tiene, cuándo se crea, cómo se comporta con modificaciones, cancelaciones, transferencias y reimpresiones, y en qué transacción y con qué lock.
3. **Las indicaciones rápidas de la 7.1**: hoy la partida guarda solo el texto, no el identificador de la indicación (limitación registrada en `docs/ETAPA_7_1_AVANCE.md`). Evalúa qué cambio exige §9.
4. **Gastos de caja de la etapa 7**: estructura y reglas de corrección, para la vinculación de §20.
5. **Sistema anterior** (`CLAUDE.md` §21): tablas `articulos`, `unidades`, `inventario`, `entradas`, `entradas_detalles`, `ajustes_inventario`, `recetas` y `proveedores`, y los formularios de inventario. Nota: el sistema anterior descontaba inventario **al capturar** (`CapturaOrden.cs`, `Pedidos.cs`); AD-EL POS lo hace **al liberar a cocina**. Sus tipos de artículo incluían mobiliario y equipo: eso es control de activos y **queda fuera** de esta etapa.

Si la verificación encuentra algo que obligue a cambiar una decisión del propietario, **detente y pregunta**. Si no, continúa.

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación contra el código (arriba). |
| **A** | Activación del inventario, insumos, unidades y presentaciones, almacenes, libro de movimientos, existencias y motor de costos (§0-B), con pruebas de integración. |
| **B** | Recetas, versiones, subrecetas, lectura de personalizaciones, combos, indicaciones rápidas con efecto y consumo desde la liberación a cocina, incluidas modificaciones y cancelaciones. |
| **C** | Proveedores, órdenes de compra, recepciones, devoluciones a proveedor y vinculación con gastos de caja. |
| **D** | Ajustes, mermas, transferencias, conteos físicos, alertas y sugerencias de compra. |
| **E** | Interfaz completa revisada, regresiones, documentación y entrega. |

Cada fase incluye su interfaz y sus pruebas; la fase E es de integración y revisión, no de construir la interfaz desde cero.

**Al terminar cada fase:** ejecuta sus pruebas y las suites afectadas, aplica `./scripts/dev.sh format` y `format-check`, registra un resumen en `docs/ETAPA_8_AVANCE.md`, haz un commit local y **continúa con la siguiente fase sin esperar confirmación** si todo pasa.

**Detente y pregunta únicamente si:** una prueba falla y arreglarla exige cambiar una decisión de este documento; encuentras una contradicción que obligue a una decisión funcional importante; necesitas aplicar la migración a la instalación real; o una acción podría afectar datos o servicios fuera del alcance. Ante dudas menores, toma la opción más conservadora, anótala en el avance y continúa.

La conversación será larga. Si se compacta, vuelve a leer este documento y `docs/ETAPA_8_AVANCE.md` antes de continuar.

---

## 0-B. DECISIONES DEL PROPIETARIO Y DISEÑO TÉCNICO

Estas decisiones resuelven ambigüedades de las secciones siguientes. **Si una sección parece decir otra cosa, prevalece esta.**

### El inventario es una función activable

- Usa el patrón de funciones activables de la etapa 6.1: **«Control de inventario»**, desactivada por defecto.
- Mientras esté desactivada, la venta funciona exactamente como hoy y **no se genera ningún consumo**.
- Al activarla se guarda la **fecha y hora de activación**. Solo las liberaciones a cocina **posteriores** generan consumo. Nunca se generan consumos de pedidos anteriores.
- La administración (insumos, recetas, almacenes, existencias iniciales) puede configurarse antes de activar, para que el restaurante prepare todo y active cuando esté listo.
- Desactivarla deja de generar consumos nuevos y conserva todo el historial.

### Porciones: «Sin», «Normal» y «Extra»

- La regla de multiplicadores (0, 1 y 2) del §8 aplica a ingredientes con **porción habitual** configurada y opción simple de extra.
- **Si el catálogo ya permite elegir cantidades de porciones** (grupos, tramos incrementales, «2 porciones de papas»), el consumo es **número de porciones seleccionadas × porción habitual**, no un 2 fijo. El 2 es el caso de «Extra» cuando el catálogo solo ofrece esa opción.
- El inventario **lee** las selecciones estructuradas que ya guarda la partida (Fase 0, punto 1). No agrega controles nuevos a la captura salvo lo indispensable para las indicaciones con efecto, y ninguno pide gramos.

### Indicaciones rápidas con efecto de inventario

- Una indicación con efecto (omitir, normal o extra) **no es un segundo mecanismo**: al tocarla en la captura, aplica la **misma selección estructurada** que el personalizador (por ejemplo, «Sin cebolla» marca la cebolla como retirada). Así es imposible acumular dos efectos sobre el mismo ingrediente.
- La comanda imprime la indicación una sola vez.
- Para esto, la partida debe guardar el **identificador** de las indicaciones elegidas además del texto. Si exige cambiar el formato del borrador, hazlo de forma compatible con las partidas existentes, que siguen con solo texto y sin efecto.
- Una indicación sin efecto configurado sigue siendo solo instrucción de cocina, como hoy.
- **Precio e inventario salen del mismo dato.** Si el catálogo cobra el extra de un ingrediente, la indicación «Cebolla extra» cobra exactamente lo mismo que elegir el extra en el personalizador, porque fija la misma cantidad. Nunca puede cobrarse un extra sin consumirlo, ni consumirlo sin cobrarlo.
- **Solo se muestran las indicaciones que aplican al producto.** Una indicación con efecto sobre la cebolla no aparece en un producto que no tiene cebolla en sus opciones. Las indicaciones sin efecto (por ejemplo, «Bien cocido») se muestran como hoy.
- «Con cebolla» en un producto que ya la incluye no cambia nada: deja la porción normal y llega a cocina como confirmación.

### Complementos: consumo y captura rápida

Ejemplo del propietario: un filete empanizado de $140 incluye arroz ($30) y ensalada ($25). Otros complementos disponibles: papas ($35), frijoles ($30), coditos ($25) y pasta ($20).

- **Precio** (ya implementado en la etapa 3, no cambia): equivalencia por valor con piso en el precio base. Quitar ensalada ($25) y poner papas ($35) → $150. Quitar arroz ($30) y poner pasta ($20) → sigue en $140, no baja del precio establecido.
- **Inventario**: consume los complementos **realmente seleccionados** con su receta o porción, sin importar su precio. En el segundo caso se consume pasta y ensalada, no arroz, aunque el precio no cambie.
- **Captura** (ajuste de interfaz en esta etapa, fase B): la selección de complementos debe ser rápida para el mesero en hora pico. Revisa cómo la muestra hoy el panel de personalización de la 6.1 y ajústala para que:
  - Cada complemento del grupo sea una tarjeta táctil con nombre y precio, y los incluidos aparezcan ya en 1.
  - **Un toque en la tarjeta agregue una porción**, y un botón **−** en la propia tarjeta la reste, visible cuando la cantidad es mayor que cero. Botones de al menos 44 px; la cantidad, grande y visible.
  - **Sustituir sea de dos toques**: − en arroz y toque en pasta. Sin listas desplegables ni pasos intermedios.
  - El encabezado del grupo muestre cuántas porciones lleva respecto a las incluidas («2 de 2») y el panel muestre en vivo cómo cambia el precio («+$10.00» o «sin cambio»), calculado por Go como hoy.
  - Se respeten los mínimos y máximos del grupo ya configurados, con un aviso claro si se intenta pasar del máximo.
  - Si al revisar encuentras que el panel ya cumple esto, no lo rehagas: solo ajusta lo que falte y repórtalo en el avance.

### Consumo asíncrono e idempotente

La liberación a cocina ocurre dentro de la transacción de restaurante y cocina (lock 7820148305). Calcular recetas, subrecetas, conversiones y costos ahí alargaría ese lock y frenaría la captura en hora pico, en contra de la regla de no ralentizarla.

- En la transacción de la liberación solo se registra una **solicitud de consumo** persistente ligada a la liberación canónica, con restricción única: una por liberación.
- Un **trabajador de Go** procesa las solicitudes pendientes en transacciones propias, con su propio lock de inventario, y escribe los movimientos. Es idempotente: reprocesar una solicitud no duplica movimientos (restricción única por solicitud e insumo, o equivalente).
- **La versión de la receta y las selecciones se fijan al liberar**, no al procesar: la solicitud guarda la versión vigente en ese momento, o se resuelve por fecha de vigencia. Un cambio de receta entre la liberación y el procesamiento no altera el consumo.
- El movimiento lleva como fecha la de la liberación, además de la de registro.
- Si al medir encuentras que el cálculo síncrono es despreciable y más simple, puedes proponerlo, con mediciones, en el avance. **No lo cambies sin justificarlo.**
- Modificaciones y cancelaciones de partidas liberadas generan solicitudes de ajuste por el mismo mecanismo.
- Agrega a la tabla de locks de `CLAUDE.md` §3 un lock nuevo de inventario, tomado siempre **después** de 7820148305 y 7820148304 cuando concurran.

### Motor de costos (§15)

**El costo promedio es uno solo por insumo**, no por almacén (decisión del propietario). Las cantidades se llevan por insumo y almacén; las transferencias no cambian el costo.

Para que la política con existencias negativas sea exacta y comprobable, el motor mantiene por insumo la **cantidad total** y el **valor total**, y el costo promedio es valor ÷ cantidad cuando la cantidad es positiva. No guardes solo el costo unitario, porque el redondeo acumulado lo desvía.

1. **Salida con existencia positiva:** valor de salida = cantidad × costo promedio vigente.
2. **Salida con existencia en cero o negativa:** se valora con el **costo de referencia vigente**, que es el último costo promedio válido o, si nunca hubo, el costo de referencia configurado en el insumo. El valor total puede quedar negativo junto con la cantidad.
3. **Sin ningún costo disponible:** la salida se registra con valor cero y marca **«costo pendiente»**. Esos movimientos aparecen en una alerta y se valoran por diferencia cuando llegue la primera recepción (punto 4), sin reescribirlos.
4. **Entrada con existencia negativa:** las unidades que cubren el déficit (d = mínimo entre lo recibido y el déficit) se valoran a costo real de la recepción. Su valor provisional era la parte proporcional del valor negativo acumulado (d × valor negativo ÷ cantidad negativa). La diferencia entre ambos se registra como **movimiento de ajuste de costo** separado (solo valor, cantidad cero), ligado a la recepción y al insumo. Las unidades restantes entran al costo de la recepción.
5. **Entrada con existencia positiva:** promedio ponderado normal.
6. **Ajustes positivos y sobrantes de conteo:** al costo promedio vigente o, sin existencia positiva, al costo de referencia. Nunca a un costo inventado.
7. **Existencia inicial:** exige costo unitario, o usa el de referencia del insumo si el usuario lo confirma explícitamente.
8. **Devolución a proveedor:** sale al costo de la recepción original. Si difiere del promedio vigente, la diferencia se registra como ajuste de costo ligado a la devolución.
9. **Ningún movimiento histórico se reescribe.** Todas las diferencias se expresan con movimientos nuevos.

**Precisión:** cantidades con al menos 4 decimales en la unidad base (para recetas de gramos o mililitros fraccionarios), y costos unitarios y valores con al menos 6 decimales internamente. Los importes se redondean a centavos solo para mostrar e imprimir. Documenta las precisiones elegidas.

**Pruebas obligatorias del motor**, todas con resultados exactos verificados a mano: positivo, cero, negativo, recepción que cubre parte del déficit, recepción que cruza a positivo, varias salidas negativas a costos de referencia distintos antes de la recepción, costo pendiente, ajustes, transferencias (sin cambio de costo), devolución a proveedor y conteo con sobrante y faltante.

### IVA de las compras

- Configuración del establecimiento **«El IVA de las compras forma parte del costo»**, **activada por defecto** (lo normal cuando no se acredita el IVA).
- Activada: el costo del insumo es el total con IVA.
- Desactivada: el costo es el subtotal sin IVA, y el IVA se guarda aparte en la compra.
- Las compras guardan siempre subtotal, IVA y total por renglón y por documento, igual que el sistema anterior, para que el cambio de configuración no pierda información.
- Cambiar la configuración solo afecta recepciones futuras. Auditado.

### Cancelaciones sin clasificar

- Si una partida cancelada después de liberarse no se clasifica, al **cierre del día operativo** se considera **«No preparado»** y se revierte su consumo (decisión del propietario).
- El **día operativo** se define con una **hora de corte** configurable del establecimiento (predeterminada a las 05:00), en su zona horaria. No depende de los turnos de caja.
- La reconciliación la ejecuta un proceso de Go, persistente e idempotente: si Go estuvo apagado, al iniciar procesa los días pendientes.
- Hasta el corte, el encargado puede clasificar «No preparado» o «Preparado / merma» desde una lista de cancelaciones pendientes. Después solo con corrección autorizada.

### Empaques para llevar

Los insumos de ejemplo incluyen bolsas, vasos y envases, que solo se consumen en pedidos para llevar. Permite en la receta componentes marcados **«solo para llevar»**, que se consumen únicamente cuando la partida pertenece a una cuenta para llevar (incluida ventanilla). En salón se ignoran.

### Conteos físicos (§23)

- El **momento de referencia es por renglón**: cada cantidad contada guarda la hora en que se capturó.
- Diferencia = contado − existencia teórica **en esa hora**, calculada con el libro de movimientos.
- El ajuste aplica esa diferencia; los movimientos posteriores a la hora del renglón se conservan.
- Aplicar el mismo conteo dos veces no genera otro ajuste.

---

## 1. REVISIÓN DEL PROYECTO

Antes de modificar el código, revisa:

- `CLAUDE.md`.
- `AGENTS.md`.
- `ARCHITECTURE.md`.
- `API_CONTRACTS.md`.
- `README.md`.
- `NEXT_STEPS_FOR_SOL.md`.
- `docs/CATALOG.md`.
- `docs/RESTAURANT_OPERATIONS.md`.
- `docs/KITCHEN_AND_PRINTING.md`.
- `docs/PAYMENTS_AND_REFUNDS.md`.
- `docs/CASH_SHIFTS_AND_RECONCILIATION.md`.
- `docs/VERIFICATION.md`.
- La documentación de las etapas 6.1, 6.2, 7 y 7.1.
- Las migraciones existentes, hasta la 0009 inclusive.
- Los servicios y modelos reales de catálogo, cocina, pedidos, pagos y caja.

Identifica cómo se implementaron:

- Los ingredientes del catálogo.
- Las personalizaciones.
- Los complementos.
- Las variantes.
- Los combos.
- Las indicaciones rápidas por categoría.
- La liberación canónica de partidas a cocina.
- Las cancelaciones y modificaciones de pedidos.
- Los eventos persistentes.
- La idempotencia.
- Los gastos de caja.
- Los permisos y la auditoría.

### Referencia del sistema anterior

En `CLAUDE.md`, sección 21, existen referencias al sistema anterior desarrollado en C# y SQL Server.

Revisa, cuando estén disponibles:

- `Inventario`.
- `Entrada_Inventario`.
- `AjusteInventario`.
- `Control_Inventario`.
- `Composicion`.
- `Proveedores`.

Estudia las reglas de negocio que realmente utilizaba el restaurante.

El sistema antiguo es una referencia funcional, no un diseño que debas copiar ni una arquitectura que debas reproducir.

No traslades directamente sus tablas de SQL Server.

---

## 2. REGLAS OBLIGATORIAS

Las siguientes decisiones ya están aprobadas y no deben cambiarse.

1. **Las existencias negativas no bloquean las ventas.**
2. **El consumo teórico ocurre cuando una partida se libera a cocina**, no cuando se captura, cobra o imprime.
3. Una misma unidad comercial no puede consumir inventario dos veces por llegar a varias estaciones.
4. Una devolución financiera no reintegra automáticamente ingredientes.
5. Un gasto de caja no genera automáticamente una compra.
6. Una compra no genera automáticamente un gasto de caja.
7. Las recetas utilizan cantidades configuradas, sin exigir que el mesero capture gramos o mililitros.
8. Las observaciones escritas libremente no modifican el inventario.
9. Las indicaciones rápidas solo afectan inventario si cuentan con una relación estructurada y explícita.
10. Los movimientos históricos deben conservarse.
11. Los costos de inventario son independientes de los precios de venta.
12. El backend Go es la autoridad para cantidades, existencias y costos.

La interfaz existente de captura de pedidos, incluida la creación de cuentas al guardar el primer producto, debe conservar su comportamiento.

No introducir formularios administrativos adicionales en ese flujo.

---

## 3. INSUMOS

Implementa un catálogo de insumos inventariables.

Ejemplos:

- Carne.
- Pan.
- Queso.
- Cebolla.
- Lechuga.
- Aceite.
- Arroz.
- Pollo.
- Refrescos.
- Vasos.
- Envases.
- Bolsas para llevar.

Cada insumo debe poder configurarse con:

- Nombre.
- Código interno.
- Código de barras opcional.
- Descripción.
- Categoría.
- Unidad base de inventario.
- Estado activo/inactivo.
- Costo de referencia.
- Existencia mínima.
- Punto de reabastecimiento.
- Presentaciones de compra.
- Proveedores asociados.

### Integración con productos existentes

La etapa 3 ya permite administrar ingredientes, productos y complementos.

No crear registros duplicados innecesariamente.

Define relaciones claras entre:

- Producto comercial.
- Ingrediente.
- Insumo inventariable.
- Receta.
- Presentación de compra.

Una hamburguesa comercial puede consumir múltiples insumos.

Una botella de refresco puede ser simultáneamente producto de venta e insumo inventariable, consumiendo una unidad por cada botella vendida.

No obligar a todos los productos a tener receta.

Los productos sin receta continúan operando y pueden señalarse como pendientes de configuración de inventario, sin bloquear la venta.

---

## 4. UNIDADES Y CONVERSIONES

Implementar unidades de medida.

Ejemplos:

- Pieza.
- Gramo.
- Kilogramo.
- Mililitro.
- Litro.
- Paquete.
- Caja.
- Porción.

Permitir equivalencias de compra configurables.

Ejemplos:

- 1 kg = 1,000 g.
- 1 litro = 1,000 ml.
- 1 caja de refrescos = 24 botellas.
- 1 paquete de pan = 12 piezas.

Las conversiones de paquetes y cajas deben depender de la presentación del insumo, no de una equivalencia universal.

No permitir conversiones entre dimensiones incompatibles.

Utilizar cantidades decimales exactas y una precisión suficiente para recetas pequeñas.

No utilizar números de punto flotante como autoridad.

El backend debe validar todas las conversiones.

---

## 5. ALMACENES

Implementar múltiples almacenes.

Ejemplos:

- Almacén general.
- Cocina.
- Barra.
- Refrigeración.
- Congelación.
- Bodega.

Cada almacén tendrá identidad, nombre, descripción y estado.

Las existencias deben mantenerse por combinación de insumo y almacén.

### Consumo por estación

La etapa 5 ya permite administrar estaciones de preparación.

Agregar la posibilidad de asignar un almacén de consumo a cada estación.

Cuando no exista una asignación específica, usar **Almacén general**.

Varias estaciones pueden compartir un almacén.

Un producto puede pasar por varias estaciones, pero su consumo total debe calcularse una sola vez.

Si distintos componentes de un combo utilizan almacenes diferentes, registrar las salidas correspondientes sin duplicar ningún ingrediente.

La asignación de almacenes debe ser explícita y verificable.

No crear automáticamente almacenes ficticios.

---

## 6. EXISTENCIAS Y MOVIMIENTOS

Construir un libro de movimientos de inventario persistente y auditable.

Operaciones:

- Existencia inicial.
- Entrada por compra.
- Consumo por preparación.
- Reversión de consumo.
- Merma.
- Ajuste positivo.
- Ajuste negativo.
- Transferencia entre almacenes.
- Devolución a proveedor.
- Elaboración de subrecetas almacenables, cuando corresponda.

Cada movimiento debe registrar:

- Insumo.
- Almacén.
- Cantidad.
- Unidad.
- Tipo de movimiento.
- Fecha y hora.
- Origen.
- Usuario o proceso responsable.
- Costo correspondiente.
- Referencia a la operación que lo generó.
- Motivo.
- Información de auditoría.

Los saldos deben mantenerse consistentes con ese historial.

No permitir editar manualmente una existencia sin registrar un movimiento.

### Existencias negativas

Una salida que lleve la existencia por debajo de cero debe registrarse normalmente.

Ejemplo:

Existencia: 2 kg.

Consumo: 3 kg.

Resultado: −1 kg.

Mostrar advertencias de faltante, pero no bloquear la operación.

No truncar existencias negativas a cero ni eliminar movimientos para corregirlas.

---

## 7. RECETAS

Implementar un editor completo de recetas.

Ejemplo:

**Hamburguesa sencilla**

- 1 pan.
- 150 g de carne.
- 1 rebanada de queso.
- 20 g de cebolla.
- 15 g de lechuga.
- 20 ml de salsa.

Cada receta tendrá:

- Producto comercial relacionado.
- Versión.
- Componentes.
- Cantidades.
- Unidades.
- Rendimiento.
- Estado.
- Costo teórico calculado.

Permitir recetas diferentes para variantes cuando corresponda.

### Versionado obligatorio

Los cambios en una receta no modifican consumos anteriores.

Ejemplo:

Versión 1: 150 g de carne.

Versión 2: 180 g de carne.

Un pedido liberado usando la versión 1 debe conservar su consumo original.

No recalcular movimientos históricos con versiones posteriores.

Conservar versiones suficientes para reconciliar, revertir o auditar consumos.

---

## 8. PERSONALIZACIONES E INGREDIENTES

Integrar el motor de inventario con el personalizador existente.

El consumo debe reflejar los ingredientes realmente seleccionados.

### Ingredientes retirados

Producto normal:

- 20 g de cebolla.

Cliente solicita «Sin cebolla»:

- Consumo: 0 g de cebolla.

### Ingredientes normales

Cliente solicita «Con cebolla»:

- Consumo: la porción normal definida en la receta.

No consumir dos porciones si la receta ya incluye cebolla.

### Ingredientes adicionales

Cliente solicita «Cebolla extra»:

- Consumo: dos porciones habituales en total.

Si la porción normal de la receta contiene 20 g, el consumo será de 40 g.

**No solicitar al mesero que indique cuántos gramos adicionales quiere.**

La interfaz debe ofrecer una opción simple y rápida.

### Regla general de porciones

Para ingredientes con porción habitual configurada:

- Sin ingrediente: multiplicador 0.
- Normal: multiplicador 1.
- Extra: multiplicador 2.

Esto es una aproximación operativa deliberada, no una medición exacta de lo que cocina prepara.

Si un restaurante necesita reglas más específicas, podrá configurarlas desde administración, sin añadir complejidad a la captura cotidiana.

No permitir que un ingrediente se duplique varias veces por combinar una indicación rápida con una personalización estructurada equivalente.

### Ingredientes que no forman parte de la receta

No inventar consumos.

Si se permite agregar un ingrediente que no forma parte de la receta original, debe existir una porción de referencia configurada para ese producto o ingrediente.

Cuando no exista esa configuración, la opción no debe generar una cantidad arbitraria de inventario.

---

## 9. INDICACIONES RÁPIDAS POR CATEGORÍA

La etapa 7.1 incorporó indicaciones rápidas configurables por categoría.

Ejemplos:

- Sin cebolla.
- Con cebolla.
- Cebolla extra.
- Sin salsa.
- Salsa aparte.
- Bien cocido.
- Poco picante.

Las indicaciones son útiles para cocina, pero **no deben interpretarse como instrucciones financieras o de inventario por el contenido de su texto**.

### Relación estructurada opcional

Extender la configuración de indicaciones rápidas para permitir una relación opcional con ingredientes.

Cada indicación podrá ser:

**A. Solo instrucción de cocina**

No altera inventario.

Ejemplos:

- Bien cocido.
- Salsa aparte.
- Poco picante.

**B. Omitir ingrediente**

Ejemplo:

- Sin cebolla → multiplicador 0 para la cebolla vinculada.

**C. Porción habitual**

Ejemplo:

- Con cebolla → multiplicador 1 de la porción de referencia.

**D. Porción extra**

Ejemplo:

- Cebolla extra → multiplicador 2 de la porción habitual.

### Configuración sencilla

En administración podrá seleccionarse:

- Indicación rápida.
- Tipo de efecto.
- Ingrediente vinculado.

No añadir campos obligatorios para capturar gramos adicionales en cada indicación.

Utilizar las cantidades de las recetas o porciones de referencia.

### Reglas importantes

- El vínculo debe utilizar identificadores estables.
- Debe comprobarse que el ingrediente sea aplicable al producto.
- La indicación rápida debe conservar su identidad en la personalización y en los datos confirmados.
- No inferir el efecto leyendo el texto visible.
- Si una indicación no tiene efecto de inventario configurado, será solo una instrucción de cocina.
- Las indicaciones antiguas deben conservar su comportamiento actual mientras no se configuren sus relaciones.
- No modificar retroactivamente consumos al editar una indicación.
- Si existen opciones incompatibles para el mismo ingrediente, mantener un único efecto efectivo de manera explícita, sin acumular multiplicadores accidentalmente.
- Reutilizar las personalizaciones estructuradas existentes.

### Observaciones libres

**El texto libre se ignora por completo para calcular inventario.**

Ejemplo:

Un empleado escribe «Sin cebolla» en observaciones, pero no selecciona la opción estructurada.

El sistema imprime la observación para cocina, pero calcula inventario según la receta y las opciones estructuradas realmente seleccionadas.

No implementar reconocimiento de palabras, reglas lingüísticas, IA ni procesamiento de lenguaje natural para deducir consumos.

---

## 10. COMPLEMENTOS Y GUARNICIONES

El POS permite sustituir completamente los complementos de un platillo.

Ejemplo:

Producto: filete empanizado.

Complementos originales:

- Arroz.
- Ensalada.

El cliente los sustituye por:

- Dos porciones de papas.

El consumo debe registrar las papas realmente seleccionadas.

No consumir arroz ni ensalada si fueron retirados.

Respetar:

- Cantidades.
- Sustituciones.
- Porciones adicionales.
- Componentes incluidos.
- Componentes opcionales.
- Reglas estructuradas de personalización.

No calcular cantidades de inventario a partir del valor monetario de los complementos.

El precio y el consumo son cálculos diferentes.

---

## 11. COMBOS

Los combos reutilizan productos del catálogo.

El inventario debe descomponer sus componentes y aplicar las recetas correspondientes.

Ejemplo:

Combo hamburguesa:

- Hamburguesa.
- Papas.
- Refresco.

Debe consumir cada componente una sola vez, aunque el combo se envíe a varias estaciones.

### Requisitos

- Conservar identidad del combo comercial.
- Conservar componentes efectivos.
- Respetar cantidades.
- Respetar sustituciones.
- Respetar personalizaciones individuales.
- Evitar referencias circulares.
- Evitar duplicación por varias estaciones.
- Evitar movimientos repetidos durante reimpresiones o transferencias.

No registrar una segunda venta ni un segundo consumo porque un componente pertenezca a un combo.

---

## 12. SUBRECETAS

Permitir preparaciones reutilizables.

Ejemplos:

- Salsa de la casa.
- Aderezo.
- Arroz preparado.
- Carne marinada.
- Mezcla de empanizado.

Distinguir dos modalidades:

### Subreceta de elaboración inmediata

La receta del platillo utiliza una subreceta cuyos ingredientes se descomponen directamente.

El inventario descuenta los insumos de origen una sola vez.

### Subreceta preparada previamente

El restaurante elabora una cantidad de salsa, arroz u otra preparación y la registra como existencia de un producto preparado.

Al elaborar:

- Consume ingredientes de origen.
- Registra la cantidad producida.

Cuando se utiliza posteriormente:

- Consume el producto preparado.
- No vuelve a descontar sus ingredientes originales.

### Reglas

- No permitir ciclos.
- Configurar rendimientos.
- Registrar mermas de elaboración cuando corresponda.
- Preservar costos y cantidades.
- Evitar doble consumo.
- Conservar historial.

No exigir elaborar lotes a todos los restaurantes ni a todas las subrecetas.

---

## 13. LIBERACIÓN A COCINA Y CONSUMO CANÓNICO

Este es el punto principal de integración.

La etapa 5 ya dejó un mecanismo de liberación canónica por unidad comercial.

Debes localizarlo y reutilizarlo.

### Reglas

- Borrador: no consume.
- Cuenta creada: no consume.
- Pedido confirmado pero no liberado: no consume.
- Partida liberada: consume.
- Nueva partida liberada posteriormente: consume.
- Reimpresión: no consume.
- Modificación de impresora: no consume.
- Cambio de mesa: no consume.
- División de cuenta: no consume.
- Pago: no consume.
- Devolución financiera: no consume ni reintegra.
- Reintento de liberación: no consume dos veces.

La liberación debe tener una relación inequívoca con los movimientos resultantes.

### Idempotencia

No permitir consumos duplicados por:

- Doble clic.
- Reconexión.
- Reinicio de Go.
- Reprocesamiento de eventos.
- Varias estaciones.
- Reimpresión.
- Reintento HTTP.

Utilizar transacciones, identificadores persistentes y restricciones adecuadas.

### Activación inicial

Aplica la función activable «Control de inventario» y el consumo asíncrono de §0-B.

No generar automáticamente consumos para pedidos históricos anteriores a la activación del inventario.

La instalación deberá permitir comenzar con saldos iniciales y recetas configuradas.

Los productos sin receta deben continuar vendiéndose.

Registrar advertencias configurables sin impedir la operación.

---

## 14. MODIFICACIONES Y CANCELACIONES

El inventario debe manejar correctamente cambios posteriores a una liberación.

### Modificación

Si una partida liberada cambia sus ingredientes o cantidades, registrar únicamente la diferencia de consumo que corresponda.

Conservar el movimiento original y los movimientos complementarios.

No volver a consumir toda la receta.

### Cancelación antes de liberar

No genera reversión porque todavía no existía consumo.

### Cancelación después de liberar

El encargado puede clasificarla como:

**No preparado**

Se revierte el consumo teórico mediante movimientos compensatorios.

**Preparado / merma**

El consumo permanece y se registra su clasificación como merma.

### Clasificación pendiente

Regla confirmada por el propietario:

Si una cancelación no se clasifica al cierre del día operativo, se considera «No preparado» por defecto. El día operativo se define con la hora de corte de §0-B.

La reconciliación debe:

- Ejecutarse desde Go.
- Ser persistente.
- Ser idempotente.
- Respetar la fecha operativa del restaurante.
- Mantener auditoría.
- Permitir correcciones autorizadas mediante movimientos compensatorios.

No depender del cierre del turno de caja.

### Devoluciones financieras

No reintegrar ingredientes automáticamente.

Una devolución de dinero no demuestra que el producto no fue preparado.

---

## 15. COSTO PROMEDIO PONDERADO

Implementar costo promedio ponderado móvil.

Ejemplo:

Existencia:

- 10 unidades a $20.
- Valor: $200.

Recepción:

- 10 unidades a $30.
- Valor: $300.

Resultado:

- 20 unidades.
- Valor: $500.
- Promedio: $25.

### Precisión

Utilizar decimales exactos.

Mantener cantidades, costos unitarios, valores y redondeos con precisión apropiada.

No alterar registros históricos por recalcular el costo actual.

### Existencias negativas

**No aplicar ingenuamente la fórmula tradicional cuando existen saldos negativos.**

**La política definitiva es la del motor de costos de §0-B**, que precisa el criterio de abajo: un solo costo por insumo, cantidad y valor totales, diferencia calculada sobre el valor provisional acumulado y movimientos de ajuste de costo separados. El criterio original se conserva como contexto:

1. Mantener el último costo válido como costo de referencia cuando la existencia llegue a cero o sea negativa.
2. Registrar consumos con ese costo de referencia.
3. Al recibir mercancía con existencia negativa, cubrir primero el déficit de unidades.
4. Registrar de manera separada la diferencia entre el costo provisional utilizado para el consumo negativo y el costo real de la recepción.
5. No reescribir consumos históricos.
6. Las unidades que queden positivas después de cubrir el déficit se valoran según el costo de la recepción y las reglas de promedio aplicables.
7. Si no existe ningún costo de referencia válido, marcar el costo como pendiente o provisional y resolverlo con una política explícita, sin inventar valores de adquisición.

Ejemplo:

- Existencia: −5 unidades.
- Último costo válido: $20.
- Recepción: 8 unidades a $30.

Resultado:

- Nueva existencia: 3 unidades.
- Las 5 unidades que cubren el déficit se reconcilian conservando la historia.
- La diferencia de costo correspondiente se registra separadamente.
- Las 3 unidades positivas restantes tienen una valoración coherente con su costo real.

Documentar y probar cuidadosamente los casos de saldos negativos, cero, cruce a positivo, ajustes y transferencias.

### Costos e impuestos

No asumir que la tasa de IVA de venta de un producto determina automáticamente el tratamiento fiscal de su compra.

Separar costo de adquisición, impuestos de compra y costos recuperables o no recuperables cuando corresponda, según la configuración «El IVA de las compras forma parte del costo» de §0-B.

No desarrollar contabilidad fiscal completa en esta etapa.

---

## 16. PROVEEDORES

Crear un catálogo de proveedores.

Campos sugeridos:

- Nombre comercial.
- Razón social opcional.
- RFC opcional.
- Teléfono.
- Correo electrónico.
- Domicilio.
- Contacto.
- Observaciones.
- Estado.
- Insumos suministrados.

Permitir búsqueda, filtros, edición, activación/desactivación y consulta del historial de compras.

No eliminar proveedores utilizados por compras históricas.

No implementar todavía cuentas por pagar completas.

---

## 17. ÓRDENES DE COMPRA

Implementar órdenes de compra reales.

Datos:

- Proveedor.
- Fecha.
- Insumos.
- Cantidades.
- Presentaciones.
- Precios de compra.
- Descuentos comerciales.
- Impuestos cuando corresponda.
- Totales.
- Almacén de destino previsto.
- Observaciones.
- Responsable.

Estados:

- Borrador.
- Emitida.
- Parcialmente recibida.
- Recibida.
- Cancelada.
- Cerrada administrativamente.

Crear una orden no debe incrementar existencias.

Las existencias aumentan al confirmar la recepción física.

No crear movimientos de caja automáticamente al emitirla.

---

## 18. RECEPCIONES DE MERCANCÍA

Permitir recepciones totales y parciales.

Ejemplo:

Orden de 20 cajas.

Primera recepción: 12 cajas.

Segunda recepción: 8 cajas.

Mostrar:

- Solicitado.
- Recibido.
- Pendiente.
- Costo recibido.
- Responsable.
- Fecha.
- Almacén.

### Reglas

- Una recepción debe ser idempotente.
- No recibir dos veces la misma operación.
- La recepción aumenta existencias.
- La recepción actualiza costos según la política de valuación.
- No modificar movimientos anteriores.
- Una recepción parcial no cierra obligatoriamente la compra.
- Cancelar una orden con recepciones no borra esas entradas.

Permitir una recepción directa sin orden previa cuando exista un flujo explícito y autorizado.

---

## 19. DEVOLUCIONES A PROVEEDORES

Permitir devolver mercancía recibida.

Registrar:

- Proveedor.
- Compra o recepción relacionada.
- Insumo.
- Cantidad.
- Almacén.
- Motivo.
- Responsable.
- Fecha.
- Valor asociado.

La devolución disminuye existencias mediante movimientos.

No borrar la recepción original.

No crear automáticamente un ingreso de efectivo ni una devolución bancaria.

No permitir devoluciones duplicadas.

---

## 20. COMPRAS Y GASTOS DE CAJA

La etapa 7 ya registra gastos de caja con categoría.

Permitir vincular opcionalmente una compra o documento de abastecimiento con un gasto existente.

### Ejemplo

Compra:

- Proveedor ABC.
- Importe: $850.
- Pagada en efectivo.

Gasto de caja:

- $850.
- Concepto: Compra de suministros.

El usuario puede relacionarlos.

### Reglas

- Una compra no genera automáticamente un gasto.
- Un gasto no genera automáticamente una compra.
- Una recepción no representa necesariamente un pago.
- No duplicar egresos.
- Validar importes.
- Validar moneda.
- Validar existencia y estado del gasto.
- Respetar permisos.
- Auditar la vinculación.
- Conservar el vínculo aunque el turno histórico ya esté cerrado, sin modificarlo.
- Detectar correcciones o regularizaciones del gasto vinculado.

Si se admiten vínculos parciales, no permitir asignar más del importe disponible.

No mezclar existencias con movimientos financieros.

---

## 21. TRANSFERENCIAS ENTRE ALMACENES

Permitir transferir mercancía.

Ejemplo:

Almacén general: 50 botellas.

Traslado a Barra: 20 botellas.

Resultado:

- General: 30.
- Barra: 20.

La transferencia debe:

- Tener origen y destino.
- Registrar insumos y cantidades.
- Conservar costos y trazabilidad.
- Crear movimientos vinculados de salida y entrada.
- Ser transaccional.
- Ser idempotente.
- Respetar permisos.

No crear compras ni ventas por transferencias.

No bloquear automáticamente una transferencia por existencia negativa, salvo una regla administrativa explícita diferente de la regla de ventas, que deberá justificarse.

---

## 22. AJUSTES Y MERMAS

Permitir ajustes de inventario.

Ejemplos:

- Rotura.
- Derrame.
- Caducidad.
- Faltante físico.
- Sobrante.
- Error de registro.
- Consumo extraordinario.

Los ajustes tendrán:

- Insumo.
- Almacén.
- Cantidad.
- Motivo.
- Tipo.
- Usuario.
- Autorización cuando corresponda.
- Fecha.
- Movimiento compensatorio.

Diferenciar ajustes de discrepancia y mermas operativas.

No convertir automáticamente cualquier ajuste negativo en merma.

No permitir editar directamente la existencia actual.

---

## 23. CONTEOS FÍSICOS

Implementar conteos por almacén.

Funciones:

- Crear conteo.
- Elegir almacén.
- Seleccionar insumos.
- Capturar cantidades físicas.
- Guardar avance.
- Consultar diferencias.
- Revisar.
- Autorizar ajustes.
- Aplicar resultados.

### Frecuencia

Configurable:

- Diaria.
- Semanal.
- Mensual.
- Personalizada.
- Sin obligación de programación.

### Concurrencia

El restaurante puede seguir vendiendo durante el conteo.

No mantener bloqueos de base de datos durante todo el proceso.

Registrar un momento de referencia y considerar los movimientos posteriores antes de aplicar ajustes.

No sobrescribir movimientos ocurridos mientras se contaba.

Una aplicación repetida del mismo conteo no debe generar otro ajuste.

---

## 24. ALERTAS DE INVENTARIO

Implementar advertencias para:

- Existencia negativa.
- Existencia bajo mínimo.
- Insumo sin proveedor.
- Producto sin receta.
- Diferencia de conteo pendiente.
- Orden parcialmente recibida.
- Necesidad de reabastecimiento.

Las alertas no bloquean ventas.

Permitir niveles mínimos por almacén e insumo.

Preparar sugerencias de compra basadas en existencias, pero no emitir órdenes automáticamente sin intervención autorizada.

---

## 25. INTERFAZ

Incorporar una sección principal **Inventarios**.

Organizar mediante navegación práctica:

- Resumen.
- Insumos.
- Recetas.
- Almacenes.
- Existencias.
- Movimientos.
- Transferencias.
- Ajustes y mermas.
- Conteos.
- Compras.
- Recepciones.
- Proveedores.
- Configuración.

### Diseño

Conservar:

- Apariencia profesional.
- Temas claro y oscuro.
- Pantalla completa nativa.
- Diseño adaptable.
- Acordeones elegantes.
- Iconos consistentes.
- Notificaciones unificadas.
- Diálogos de confirmación.
- Formatos monetarios existentes.
- Controles táctiles adecuados.
- Búsquedas y filtros.
- Paginación.
- Componentes reutilizables.

No saturar formularios con campos innecesarios.

### Facilidad de uso

La captura de pedidos no debe mostrar formularios nuevos de gramos, costos, almacenes o recetas.

El mesero utiliza opciones sencillas:

- Sin ingrediente.
- Normal.
- Extra.

Administración configura las cantidades reales.

Los detalles técnicos de inventario permanecen dentro del módulo de administración.

---

## 26. SEGURIDAD, PERMISOS Y AUDITORÍA

Reutilizar el sistema existente.

Incorporar permisos granulares para:

- Consultar inventario.
- Administrar insumos.
- Administrar recetas.
- Administrar subrecetas.
- Consultar costos.
- Administrar costos.
- Consultar existencias.
- Administrar almacenes.
- Consultar movimientos.
- Registrar ajustes.
- Autorizar ajustes.
- Registrar mermas.
- Realizar conteos.
- Aprobar conteos.
- Transferir mercancía.
- Administrar proveedores.
- Crear compras.
- Autorizar compras.
- Recibir compras.
- Registrar devoluciones a proveedores.
- Vincular compras con gastos de caja.
- Consultar valuaciones.
- Administrar parámetros de inventario.

No permitir que el permiso de consultar existencias implique automáticamente permiso para consultar costos confidenciales.

Validar autorizaciones en Go.

Conservar auditoría de las operaciones relevantes.

No mostrar identificadores técnicos crudos al usuario.

---

## 27. EVENTOS EN TIEMPO REAL

Reutilizar SSE autenticado.

Publicar los cambios relevantes de:

- Existencias.
- Movimientos.
- Recetas.
- Compras.
- Recepciones.
- Transferencias.
- Ajustes.
- Conteos.
- Alertas.

Respetar permisos y ámbitos.

No exponer información de costos a usuarios sin autorización.

Implementar recuperación ante desconexiones.

No crear infraestructura paralela innecesaria.

PostgreSQL continúa siendo la fuente de verdad.

---

## 28. MIGRACIONES

La versión actual utiliza esquema 9.

La siguiente migración disponible es **0010**.

No modificar las migraciones 0001–0009.

Crear las estructuras necesarias después de inspeccionar el modelo existente.

Evitar tablas redundantes.

Priorizar:

- Integridad referencial.
- Movimientos auditables.
- Idempotencia.
- Restricciones.
- Control de concurrencia.
- Precisión decimal.
- Versionado de recetas.
- Historial de costos.
- Compatibilidad con cocina.
- Compatibilidad con caja.

### Seguridad antes de migrar

Seguir el procedimiento vigente de respaldo, restauración de prueba, verificación de datos y actualización controlada.

No suponer que la autorización dada para migraciones anteriores se extiende automáticamente a nuevas modificaciones de la instalación real.

Probar primero sobre una base aislada.

---

## 29. PRUEBAS OBLIGATORIAS

Ejecutar pruebas reales para:

### Insumos y unidades

- Crear y modificar insumos.
- Conversiones kg/g y litro/ml.
- Paquetes con equivalencia específica.
- Precisión decimal.
- Rechazo de conversiones incompatibles.

### Recetas

- Crear receta.
- Modificar y versionar.
- Subrecetas.
- Rendimientos.
- Evitar ciclos.
- Preservar consumos anteriores.

### Personalizaciones

- Producto normal.
- Sin cebolla → 0 porciones.
- Con cebolla → 1 porción habitual.
- Cebolla extra → 2 porciones habituales.
- Opción extra sin porción de referencia → no inventar cantidad.
- Observación libre «Sin cebolla» → no altera consumo.
- Indicación rápida sin vínculo → no altera consumo.
- Indicaciones estructuradas incompatibles → resolución determinista.
- Cambio posterior de indicación → no altera consumos históricos.
- Complementos sustituidos.
- Porciones adicionales.
- Combos personalizados.

### Cocina

- Borrador sin consumo.
- Creación de cuenta sin consumo.
- Liberación con consumo.
- Varias estaciones sin duplicados.
- Reintento idempotente.
- Reimpresión sin consumo.
- División y transferencia sin consumo.
- Modificación de partida liberada.
- Cancelación no preparada.
- Cancelación con merma.
- Clasificación pendiente al cierre operativo.
- Devolución financiera sin reintegración.

### Existencias y costos

- Saldo positivo.
- Saldo cero.
- Saldo negativo.
- Recepción con saldo negativo.
- Cruce de negativo a positivo.
- Costo promedio ponderado.
- Diferencias de costos.
- Ajustes y transferencias.
- Subreceta elaborada previamente sin doble descuento.

### Compras

- Orden de compra.
- Recepción total.
- Recepción parcial.
- Recepción repetida.
- Devolución a proveedor.
- Compra sin pago vinculado.
- Gasto sin compra.
- Vinculación opcional.
- Prevención de egresos duplicados.

### Decisiones de §0-B

- Inventario desactivado: liberar a cocina no genera solicitudes ni movimientos.
- Activación: solo las liberaciones posteriores consumen; ningún consumo de pedidos anteriores.
- Consumo asíncrono: la liberación registra una sola solicitud; el trabajador la procesa una vez aunque se reinicie Go o se reprocese; la receta modificada entre liberación y procesamiento no altera el consumo.
- La captura y la liberación no esperan al cálculo de inventario.
- Indicación rápida con efecto y la misma opción en el personalizador sobre el mismo ingrediente: un solo efecto.
- Partidas antiguas con indicaciones solo de texto: sin efecto en inventario.
- Dos porciones de papas elegidas en el catálogo: consumo de 2 × porción habitual.
- Componente «solo para llevar»: se consume en para llevar y ventanilla, no en salón.
- Cancelación sin clasificar: revertida al pasar la hora de corte; procesada al iniciar Go si estaba apagado; idempotente.
- IVA de compras incluido y excluido del costo; cambio de configuración sin efecto en recepciones anteriores.
- Costo uno por insumo: transferencias sin cambio de costo.

### Conteos

- Conteo por almacén.
- Diferencias positivas.
- Diferencias negativas.
- Ventas concurrentes.
- Aplicación idempotente.
- Autorización de ajustes.

### Regresión

- Go race detector.
- Go vet.
- PostgreSQL.
- Vitest.
- TypeScript estricto.
- Rust/Tauri.
- Chromium.
- WebKit.
- Compilación macOS.
- Sesiones.
- Permisos.
- Auditoría.
- SSE.
- Etapas 1–7.1.

No declarar ejecutadas pruebas que solamente se hayan diseñado.

---

## 30. METODOLOGÍA DE DESARROLLO

Sigue las fases de §0. El motor de costos va en la fase A, junto con el libro de movimientos, porque todos los movimientos posteriores dependen de él.

Ajustar la secuencia según las dependencias reales del repositorio, explicándolo en el avance.

La meta es completar la etapa completa, no entregar únicamente interfaces o modelos parciales.

Evitar cambios innecesarios en los módulos anteriores.

---

## 31. FUNCIONES EXCLUIDAS

No implementar todavía:

- Dashboard ejecutivo completo.
- Diseñador visual avanzado de reportes.
- Contabilidad general.
- Conciliación bancaria.
- Facturación electrónica.
- Cuentas por pagar completas.
- Nómina.
- Distribución de propinas.
- Aplicación Android.
- Aplicación KDS independiente.
- Servidor comercial de licencias.
- Instaladores Windows.
- Migrador SQL Server.

Estas funciones corresponden a otras etapas o proyectos.

---

## 32. DOCUMENTACIÓN Y ENTREGA

Actualizar los documentos técnicos relevantes.

Crear:

`docs/INVENTORY_AND_PURCHASING.md`

Documentar especialmente:

- Modelo de inventarios.
- Recetas.
- Subrecetas.
- Porciones habituales.
- Indicaciones rápidas.
- Consumo por liberación.
- Idempotencia.
- Cancelaciones.
- Mermas.
- Existencias negativas.
- Costo promedio.
- Compras.
- Recepciones.
- Proveedores.
- Conteos.
- Transferencias.
- Vinculación con caja.
- API.
- Permisos.
- Eventos.
- Pruebas.
- Limitaciones conocidas.

Actualizar `CLAUDE.md`, `AGENTS.md`, `ARCHITECTURE.md`, `API_CONTRACTS.md`, `README.md`, `NEXT_STEPS_FOR_SOL.md` y `docs/VERIFICATION.md` cuando corresponda.

### Entrega final

Informar:

1. Funcionalidades implementadas.
2. Migraciones nuevas.
3. Rutas API.
4. Pantallas.
5. Modelo de recetas.
6. Consumo por cocina.
7. Integración de indicaciones rápidas.
8. Política de costos negativos.
9. Compras y proveedores.
10. Ajustes y conteos.
11. Integración con caja.
12. Pruebas ejecutadas.
13. Limitaciones pendientes.
14. Comandos para iniciar.
15. Confirmación de continuidad de las etapas anteriores.
16. Resultado de la Fase 0: modelo de porciones encontrado, liberación canónica, cambios al formato de la partida y funciones del sistema anterior no incluidas.
17. Mediciones del consumo asíncrono (tiempo de la liberación con y sin inventario activo).
18. Procedimiento para aplicar las migraciones a la instalación real, **en espera de autorización** (§28: la autorización de migraciones anteriores no se extiende a esta).

---

# CRITERIO FINAL DE ACEPTACIÓN

Debo poder abrir AD-EL POS en mi Mac, iniciar sesión y administrar inventarios reales.

Debo poder:

- Registrar insumos.
- Crear recetas y subrecetas.
- Configurar almacenes.
- Consultar existencias.
- Registrar compras y recepciones.
- Administrar proveedores.
- Registrar mermas y ajustes.
- Realizar transferencias.
- Realizar conteos físicos.
- Consultar costos.
- Consultar movimientos históricos.
- Consultar alertas de reabastecimiento.
- Vincular opcionalmente compras con gastos existentes de caja.

Al liberar un pedido real a cocina, Go deberá descontar una sola vez los insumos correspondientes a las recetas y personalizaciones confirmadas.

Si el cliente pide «Sin cebolla», se omite la porción de cebolla.

Si pide «Con cebolla», se utiliza la porción habitual.

Si pide «Cebolla extra», se utiliza el doble de la porción habitual, sin solicitar gramos adicionales al mesero.

Las observaciones libres no modifican inventarios.

Las existencias negativas no bloquean ventas.

Una devolución financiera no reintegra automáticamente ingredientes.

Una compra no genera automáticamente un gasto de caja.

Todo debe persistirse realmente en PostgreSQL, con historial, permisos, auditoría e idempotencia.

**No quiero maquetas, consumos ficticios ni cálculos autoritativos exclusivamente en React.**

Implementa, integra, prueba y documenta la etapa 8 completa por fases (0 → E) en la rama `etapa-8`, respetando las decisiones de §0-B y conservando el funcionamiento de todas las etapas anteriores.