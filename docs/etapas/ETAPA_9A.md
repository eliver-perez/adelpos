# AD-EL POS — ETAPA 9A: AJUSTES PENDIENTES, REPORTES, INDICADORES Y EXPORTACIONES

## CONTEXTO

Continúa el desarrollo de AD-EL POS, sistema comercial de punto de venta para restaurantes.

Stack aprobado:

- Backend: Go.
- Base de datos: PostgreSQL.
- Frontend: React, TypeScript y Tailwind CSS.
- Escritorio: Tauri 2.
- Comunicación: API HTTP versionada y SSE autenticado.

Las etapas 1 a 8 están desarrolladas, incluidas las ampliaciones intermedias.

La última versión reportada es **0.8.0, esquema PostgreSQL 14**, con las migraciones `0010` a `0014` correspondientes al inventario.

La etapa 9 se divide en dos entregas. **Esta, la 9A**, implementa los ajustes pendientes de catálogo e inventarios y un sistema integral de reportes, indicadores y exportación PDF/Excel. **La 9B**, en otra entrega, hará la identidad visual del establecimiento y el diseñador de tickets y documentos. **No implementes nada del diseñador ni del logotipo en esta entrega.**

También incorporará un conjunto de mejoras pendientes en catálogo e inventarios, acordadas después de la etapa 8.

**Esta especificación debe contrastarse con el código real antes de implementarse.**

No reconstruyas módulos terminados, no dupliques motores financieros ni de inventario y no modifiques migraciones aplicadas.

El resultado debe ser funcional, mantenible, comprobable y apto para operación comercial.

Al terminar, la versión será **0.9.0**, con el esquema que resulte de las migraciones nuevas.

---

# 0. FORMA DE TRABAJO

### Rama

Trabaja en una rama nueva **`etapa-9a`** creada desde `main` (que contiene hasta la 8). No hagas push ni merge.

Las especificaciones en `docs/etapas/` son de solo lectura. Lee únicamente este documento.

### Fase 0: verificación contra el código (obligatoria, antes de diseñar)

Esta especificación se escribió sin ver el código. Antes de implementar, comprueba en el repositorio y registra en `docs/ETAPA_9A_AVANCE.md`:

1. Cómo calcula hoy el motor de precios la equivalencia por valor con piso, y dónde entra la opción de §3.1 sin romper promociones, listas de precios, variantes ni combos.
2. Qué datos guardan las cuentas, partidas, pagos, ajustes, turnos y movimientos de inventario para cada métrica de §6, y con qué fechas.
3. Si las partidas vendidas están enlazadas con sus movimientos de consumo de la etapa 8 (para el costo de venta de §14).
4. Qué componentes de tablas, filtros y gráficos existen ya en el frontend, y qué librerías de PDF y Excel serían adecuadas en Go.
5. Los reportes del sistema anterior (`CLAUDE.md` §21): `RegistroVentas.cs`, `RegistroEgresos.cs`, `RegistroCanceladas.cs`, `DetallesVenta.cs` y `Cortes.cs`.

Si encuentras algo que obligue a cambiar una decisión del propietario, **detente y pregunta**. Si no, continúa.

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación contra el código. |
| **A** | Ajustes pendientes de catálogo e inventarios (§3) y ajustes menores (§33). |
| **B** | Definiciones (§6), filtros y periodos (§7), y consultas de ventas, productos, pagos, descuentos, caja, inventarios, compras, costos y empleados (§8–§16), con permisos en Go y pruebas de consistencia. |
| **C** | Módulo Reportes en la interfaz, dashboard (§5), y exportación PDF y Excel (§17–§18). |
| **D** | Regresiones completas, rendimiento, documentación y entrega final, con el procedimiento para aplicar las migraciones a la instalación real **en espera de autorización** (§30). |

**Al terminar cada fase:** ejecuta sus pruebas y las suites afectadas, aplica `./scripts/dev.sh format` y `format-check`, registra un resumen en el avance, haz un commit local y **continúa sin esperar confirmación** si todo pasa.

**Detente y pregunta únicamente si:** una prueba falla y arreglarla exige cambiar una decisión de este documento; encuentras una contradicción que obligue a una decisión funcional importante; necesitas aplicar migraciones a la instalación real; o una acción podría afectar datos o servicios fuera del alcance. Ante dudas menores, toma la opción más conservadora, anótala en el avance y continúa.

Las menciones a plantillas, diseñador, logotipo o impresión de muestras que aparecen en las secciones generales (permisos, API, migraciones) corresponden a la **9B**: no las implementes aquí.

---

# 0-B. DECISIONES DEL PROPIETARIO

**Si una sección parece decir otra cosa, prevalece esta.**

### Cuándo cuenta una venta

- **Una venta cuenta cuando su cuenta se cierra (queda liquidada)**, en el día en que se cerró. Es el criterio del sistema anterior y cuadra con lo cobrado.
- Las cuentas abiertas aparecen aparte como **«En curso»**, con su total y su saldo, sin sumarse a las ventas.
- Las cortesías al 100 % que cierran una cuenta cuentan como cuenta cerrada, con venta cero y su importe en «Cortesías».
- Los reportes de **demanda de productos** y de **cocina** pueden usar además la fecha de liberación a cocina, siempre indicando qué fecha usan (§6).
- Una devolución posterior se refleja en el día de la devolución, sin modificar la venta del día original.

### Hora de inicio del día

- Configuración global **«Hora de inicio del día»**. Es **la misma hora de corte** que usa la etapa 8 para el día operativo del inventario; no crees una segunda.
- 00:00 equivale a la fecha del calendario. Con 05:00, una venta de la 1:30 pertenece al día anterior.
- Se conserva el valor actual de la instalación (05:00 por defecto). El propietario puede cambiarlo; el cambio afecta cómo se agrupan los días en las consultas, sin reescribir registros.
- Explica en la pantalla de configuración qué significa, con un ejemplo.

### Los reportes no dependen de los turnos

Algunos restaurantes abren un turno y trabajan con él indefinidamente.

- Los reportes de **ventas, productos, pagos, descuentos, inventarios y compras** se filtran por **día, semana del año, mes o rango**, con la hora de inicio del día. Nunca exigen un turno.
- Los reportes de **caja y de cobros** ofrecen **las dos vistas**: **por turno** (con su fecha comercial, para quien cierra turnos) y **por día** (para quien trabaja con un turno permanente).
- También debe existir **ventas y cobros por turno**: lo vendido y cobrado en cada turno, para los restaurantes que sí los cierran.

### Filtros del sistema anterior

El sistema anterior filtraba por **Día, Semana del año, Mes y Lapso**, y mostraba subtotal, IVA y total. Incluye **«Semana del año»** (ISO 8601) además de los periodos de §7.

### Precio de complementos (§3.1)

- **Solo baja si el producto tiene activada** la opción «El precio puede bajar al cambiar complementos». Con la opción desactivada, **ningún** cambio de complementos baja el precio: ni sustituir por uno más barato ni quitar sin reemplazar.
- Con la opción activada, quitar un complemento sin reemplazarlo también baja el precio por su valor, con los límites de §3.1.
- **Orden del cálculo:** primero el ajuste por complementos (dentro del motor del catálogo), después las promociones y al final los descuentos manuales de la etapa 6. Así una misma diferencia nunca se descuenta dos veces.
- La opción se configura al registrar o modificar el producto.

### Márgenes y costos (§14)

- **El costo de una venta sale de los movimientos de consumo reales** que generó la etapa 8 al enviar a cocina, no de la receta actual multiplicada por el costo actual.
- Una partida sin consumo registrado (vendida antes de activar el inventario, sin receta o con consumo pendiente de procesar) aparece como **«Sin costo»**, no con costo cero.
- Una partida con consumo a **costo pendiente** o con existencias negativas sin reconciliar aparece como **«Estimado»**.
- El margen solo se presenta como definitivo cuando todos sus consumos tienen costo firme. El reporte indica qué porcentaje de las ventas tiene costo firme.
- Los **ajustes de costo** de la etapa 8 (diferencias por existencias negativas) se reportan como concepto del periodo, no se reparten entre productos.

### Exportaciones (§17–§18)

- El PDF y el Excel se **generan en Go**, a partir de la misma consulta que la pantalla y con los mismos permisos. El frontend nunca recibe datos que el usuario no pueda ver.
- Usa librerías maduras de Go que funcionen sin red, con fuentes incluidas que soporten acentos y «ñ». Fíjalas en `go.sum` y regístralas en `docs/DEPENDENCIES.md`.
- La aplicación de escritorio guarda el archivo con el diálogo nativo de Tauri, a través de una operación cerrada del puente.

### Rendimiento

- Las consultas de reportes usan transacciones de **solo lectura** con una fotografía consistente, límite de tiempo por consulta y un número limitado de conexiones reservadas, para que un reporte de un año no frene cobros, captura ni cocina.
- No toman los locks operativos.
- El dashboard no se vuelve a calcular con cada evento SSE: se actualiza con una frecuencia razonable o cuando el usuario lo pide.
- Mide un reporte de un año con volumen simulado en la base aislada y reporta los tiempos en el avance.

---

# 1. REVISIÓN OBLIGATORIA DEL REPOSITORIO

Antes de escribir código, examina:

- `CLAUDE.md`.
- `AGENTS.md`.
- `ARCHITECTURE.md`.
- `API_CONTRACTS.md`.
- `README.md`.
- `NEXT_STEPS_FOR_SOL.md`.
- `docs/VERIFICATION.md`.
- `docs/CATALOG.md`.
- `docs/RESTAURANT_OPERATIONS.md`.
- `docs/PAYMENTS_AND_REFUNDS.md`.
- `docs/KITCHEN_AND_PRINTING.md`.
- `docs/CASH_SHIFTS_AND_RECONCILIATION.md`.
- `docs/INVENTORY_AND_PURCHASING.md`.
- La documentación técnica de las etapas 7.1 y 8.
- Las migraciones existentes.
- Los módulos de catálogo, restaurante, cocina, pagos, caja e inventarios.
- El sistema actual de impresión.
- La seguridad, permisos, auditoría y eventos SSE.
- Los componentes compartidos de tablas, formularios, filtros, notificaciones y gráficos, si existen.

Verifica las estructuras y los contratos reales antes de determinar el diseño de los reportes.

### Referencias del sistema anterior

Consulta la sección 21 de `CLAUDE.md` y, cuando estén disponibles, los archivos del POS original desarrollado en C#.

Presta especial atención a:

- Reportes de ventas.
- Reportes por producto.
- Reportes de caja.
- Reportes por empleado.
- Reportes de proveedores.
- Reporte de compras por periodo.
- Consultas históricas de inventario.
- Tickets y formatos impresos.

El sistema anterior sirve como referencia funcional.

No copiar su arquitectura Windows Forms ni trasladar consultas SQL Server sin adaptarlas correctamente.

---

# 2. PRINCIPIOS DE LA ETAPA

Las siguientes reglas son obligatorias:

1. Todos los importes y cantidades deben provenir de información real del backend Go y PostgreSQL.
2. No recalcular precios históricos utilizando el catálogo actual.
3. No sumar ventas y cobros como si fueran el mismo ingreso.
4. No contabilizar una compra y su gasto de caja vinculado como dos compras o dos salidas físicas adicionales.
5. No tratar una devolución financiera como una devolución de ingredientes.
6. No confundir inventario teórico con inventario físico.
7. No exponer costos a usuarios sin permisos de costos.
8. No exponer efectivo esperado o diferencias de caja sin los permisos correspondientes.
9. Los archivos PDF y Excel deben aplicar las mismas autorizaciones que las consultas de pantalla.
10. Los reportes deben respetar fechas comerciales, zonas horarias, monedas y registros históricos.
11. La impresión no debe crear nuevas ventas ni nuevos movimientos.
12. La reimpresión de documentos históricos debe conservar la información comercial originalmente registrada.
13. Los diseños personalizados no deben permitir ejecutar código arbitrario.
14. Los módulos anteriores deben continuar funcionando normalmente.
15. La aplicación debe mantener la interfaz moderna, adaptable y compatible con pantalla completa.

---

# 3. FASE INICIAL: AJUSTES PENDIENTES DE CATÁLOGO E INVENTARIOS

Antes de desarrollar los reportes definitivos, incorpora los ajustes funcionales pendientes que influyen en el catálogo y en la administración de compras.

Estos ajustes forman parte del alcance de la etapa 9, pero deben mantenerse separados del motor de reportes para que no introduzcan regresiones financieras.

## 3.1. Precio de complementos configurable por producto

Actualmente, el motor de precios mantiene un piso que impide disminuir el precio base cuando se retiran o sustituyen complementos.

Debe conservarse como comportamiento predeterminado.

Agregar una opción en la configuración del producto:

**«El precio puede bajar al cambiar complementos»**

Estado inicial: desactivada.

### Opción desactivada

Conservar exactamente la regla comercial existente.

Una sustitución por complementos de menor valor no reduce el precio base.

### Opción activada

Permitir que las diferencias negativas del valor de los complementos disminuyan el precio comercial del producto, incluido quitar un complemento sin reemplazarlo. El orden del cálculo está en §0-B.

Ejemplo:

- Producto base: $150.
- Complementos incluidos originalmente: valor de $35.
- Complementos seleccionados: valor de $25.
- Diferencia: −$10.
- Precio resultante: $140.

El precio debe reflejar la diferencia real según las reglas de porciones del motor existente.

### Límites

El resultado nunca podrá:

- Ser negativo.
- Quedar por debajo del valor de los complementos que permanecen seleccionados.
- Descontar indebidamente cargos adicionales independientes.
- Duplicar descuentos por la misma sustitución.

Debe conservarse el tratamiento correcto de:

- Extras.
- Cantidades.
- Complementos incluidos.
- Promociones.
- Listas de precios.
- Variantes.
- Combos.
- IVA configurado.

El backend Go será la autoridad de cálculo.

No reimplementar las fórmulas de manera independiente en React.

### Interfaz

Mostrar el efecto de la sustitución mientras el usuario personaliza.

Ejemplos:

- `−$10.00`.
- `+$5.00`.
- `Sin cambio`.

Respetar los formatos monetarios existentes.

### Historial

La opción deberá conservarse como parte del contexto comercial aplicable al producto y a su cotización.

Una vez confirmada una partida, el precio queda registrado históricamente.

Modificar posteriormente esta opción no altera cuentas anteriores.

El consumo de ingredientes y complementos continúa dependiendo de las recetas y personalizaciones estructuradas, no del importe cobrado.

---

## 3.2. Proveedor «Compras de mostrador»

Actualmente, la recepción directa exige seleccionar un proveedor.

Esto resulta incómodo para compras realizadas en:

- Mercados.
- Centrales de abastos.
- Supermercados.
- Autoservicios.
- Comercios ocasionales.

Agregar una opción genérica identificable como:

**Compras de mostrador**

Puede implementarse como proveedor especial o como modalidad explícita de proveedor no formal, según la arquitectura existente.

La opción debe aparecer de manera práctica al registrar recepciones directas.

### Reglas

- No exigir razón social ni RFC para esta modalidad.
- Conservar la identificación de la operación.
- Permitir registrar el comercio concreto como referencia opcional.
- No crear un proveedor nuevo por cada compra ocasional.
- No crear duplicados del proveedor especial.
- No alterar compras históricas.
- No registrar compras o recepciones ficticias al crear la opción.

Debe existir una distinción clara entre un proveedor formal y una compra de mostrador para los reportes.

---

## 3.3. Datos adicionales de proveedores

Ampliar el formulario y el modelo de proveedores con:

- Segundo teléfono.
- Teléfono celular.
- Página web.

Conservar:

- Teléfono principal.
- Correo.
- Dirección.
- Contacto.
- RFC opcional.
- Razón social opcional.

Los nuevos campos deben ser opcionales y validarse apropiadamente.

No convertirlos en requisitos para guardar proveedores ya existentes.

Actualizar consultas, formularios, API y persistencia.

No modificar registros históricos de compras.

---

## 3.4. Código interno sugerido

La etapa 6.1 ya incorporó sugerencias de código interno en algunos formularios.

Reutilizar esa implementación.

### En inventarios

Al registrar un insumo, sugerir automáticamente un código interno a partir de su nombre.

Permitir modificarlo manualmente.

No sobrescribir un código escrito por el usuario.

Comprobar unicidad y conflictos desde Go.

### Regla general

Aplicar el mismo patrón a otros formularios existentes que utilicen un campo de código interno y todavía no dispongan de sugerencia.

No inventar códigos diferentes en distintas partes del sistema.

No cambiar códigos de registros históricos automáticamente.

No modificar identificadores internos UUID.

---

## 3.5. Categorías de insumos

Actualmente existe un selector de categoría de insumo, pero no una administración completa de dichas categorías.

Implementarla.

Funciones:

- Crear categoría.
- Editarla.
- Activarla o desactivarla.
- Ordenarla.
- Buscarla.
- Consultar sus insumos.
- Evitar duplicados.
- Conservar referencias históricas.

### Alta rápida

En el selector de categoría de un insumo, agregar una opción:

**+ Nueva categoría**

Debe permitir crearla sin abandonar innecesariamente el formulario actual.

Después de guardarla:

- Actualizar el catálogo.
- Seleccionar la nueva categoría.
- Conservar los demás datos capturados del insumo.

No exigir guardar primero un insumo incompleto para poder crear su categoría.

---

## 3.6. Reorganización de navegación de Inventarios

La etapa 8 incorporó aproximadamente trece pestañas que actualmente pueden distribuirse en dos filas de manera irregular.

Necesitamos una navegación más ordenada.

Propuesta de agrupación:

**Resumen**

- Indicadores principales.
- Alertas.
- Accesos operativos.

**Catálogo**

- Insumos.
- Recetas.
- Almacenes.
- Proveedores.

**Existencias**

- Existencias.
- Movimientos.
- Transferencias.
- Ajustes y mermas.
- Conteos.

**Compras**

- Órdenes de compra.
- Recepciones.
- Devoluciones a proveedores, cuando corresponda.

**Configuración**

- Unidades.
- Categorías.
- Parámetros.
- Otras configuraciones del inventario.

### Requisitos

- Evitar filas irregulares de pestañas.
- No perder secciones existentes.
- Mantener navegación sencilla.
- Respetar permisos.
- Conservar el contexto cuando sea posible.
- Mantener formularios y acciones accesibles.
- Aprovechar monitores grandes sin desperdiciar espacio.
- Adaptarse correctamente al modo ventana.

Puede utilizarse navegación secundaria, controles segmentados o pestañas agrupadas.

No es necesario copiar literalmente estos nombres si la organización actual permite una solución mejor.

**No rediseñar el menú principal global de la aplicación.** Su posible disposición horizontal todavía no está aprobada.

---

# 4. SISTEMA DE REPORTES

Crear un módulo principal **Reportes**.

Debe servir para consultar y analizar información histórica y operativa.

No limitarlo a tablas con botones de exportación.

La aplicación debe ofrecer reportes con filtros, resúmenes, desgloses y visualizaciones apropiadas.

### Estructura sugerida

- Resumen ejecutivo.
- Ventas.
- Productos.
- Categorías.
- Pagos.
- Descuentos y cortesías.
- Caja.
- Cocina y operación.
- Inventarios.
- Compras.
- Proveedores.
- Costos y márgenes.
- Reportes personalizados, dentro del alcance que se defina.

Conservar las responsabilidades de los módulos existentes.

La administración de una caja continúa en Caja; los reportes consolidados de caja se consultan desde Reportes.

La administración de compras permanece en Inventarios.

---

# 5. DASHBOARD EJECUTIVO

Implementar un dashboard profesional para consultar el estado comercial del establecimiento.

### Indicadores sugeridos

- Ventas del periodo.
- Ventas netas.
- Importe cobrado.
- Saldo pendiente de cuentas.
- Número de operaciones comerciales.
- Número de cuentas atendidas.
- Ticket promedio.
- Productos vendidos.
- Descuentos.
- Cortesías.
- Devoluciones.
- Propinas.
- Compras.
- Consumo teórico.
- Mermas.
- Costos disponibles.
- Margen estimado, únicamente cuando existan datos suficientes.

### Comparaciones

Permitir comparar:

- Periodo actual contra anterior.
- Hoy contra ayer.
- Semana actual contra anterior.
- Mes actual contra anterior.
- Rangos personalizados.

Los periodos deben tener criterios claros y consistentes.

### Visualizaciones

Utilizar gráficos apropiados:

- Líneas para tendencias.
- Barras para comparaciones.
- Gráficos de distribución cuando aporten información.
- Tablas para desgloses detallados.

Reutilizar componentes o bibliotecas gráficas que ya existan en el proyecto, cuando sean adecuadas.

No introducir una biblioteca de visualización grande sin justificación.

### Reglas

- No utilizar datos ficticios.
- Mostrar estados vacíos comprensibles.
- Evitar consultas excesivas al cambiar filtros.
- Permitir navegar desde un indicador hacia su reporte detallado.
- Respetar permisos.
- Ocultar métricas financieras sensibles cuando corresponda.
- Presentar datos actualizados sin consultar constantemente toda la base.

El dashboard no debe ralentizar la captura de pedidos o los cobros.

---

# 6. DEFINICIONES CONTABLES Y OPERATIVAS DE LOS REPORTES

Antes de crear consultas definitivas, documentar el significado de cada métrica.

Distinguir expresamente:

**Venta comercial**

Importe asociado a los productos y cargos comerciales, considerando los descuentos y ajustes que correspondan.

**Cobro**

Movimiento financiero efectivamente registrado mediante un método de pago.

**Cuenta pendiente**

Obligación comercial todavía no liquidada.

**Devolución**

Movimiento compensatorio financiero vinculado a una operación anterior.

**Compra**

Documento comercial de abastecimiento.

**Recepción**

Entrada física de mercancía.

**Gasto de caja**

Salida de efectivo registrada en un turno.

**Consumo de inventario**

Salida teórica o física según el tipo de movimiento y su causa.

**Costo**

Valuación de insumos o consumos.

No mezclar estas entidades en un único campo denominado «Total» sin explicar qué representa.

### Fechas

Distinguir:

- Fecha del pedido.
- Fecha de confirmación.
- Fecha de cobro.
- Fecha de devolución.
- Fecha de liberación a cocina.
- Fecha de movimiento de inventario.
- Fecha de recepción.
- Fecha comercial del turno.

Los reportes deben utilizar la fecha apropiada al concepto mostrado.

---

# 7. FILTROS Y PERIODOS

Implementar un sistema de filtros reutilizable.

Según el reporte, permitir:

- Hoy.
- Ayer.
- Semana actual.
- Mes actual.
- Año actual.
- Semana del año (ISO 8601), como el sistema anterior.
- Periodo personalizado.
- Caja.
- Turno.
- Usuario.
- Mesero.
- Cajero.
- Área.
- Mesa.
- Producto.
- Categoría.
- Método de pago.
- Proveedor.
- Almacén.
- Insumo.
- Estado.

No mostrar filtros irrelevantes en todos los reportes.

### Zona horaria

Usar la zona horaria configurada del restaurante.

No asumir que la fecha local de una terminal coincide necesariamente con la fecha comercial del servidor.

### Fecha comercial

La etapa 7 incorporó fecha comercial del turno.

Los reportes de caja deben utilizarla cuando corresponda.

Un turno que comienza por la noche y termina después de medianoche debe conservar su identidad comercial.

Los reportes históricos no deben reescribirse por cambios posteriores de configuración horaria.

---

# 8. REPORTES DE VENTAS

Implementar reportes de:

- Ventas por día.
- Ventas por periodo.
- Ventas por hora.
- Ventas por categoría.
- Ventas por producto.
- Ventas por variante.
- Ventas por modalidad.
- Ventas de salón.
- Ventas para llevar/ventanilla.
- Ventas por lista de precios.
- Descuentos.
- Cortesías.
- Devoluciones.
- Cuentas con saldo pendiente.
- Evolución de ventas.

### Información

Cuando corresponda, mostrar:

- Cantidad.
- Importe bruto.
- Descuentos.
- Cargos.
- Importe neto.
- IVA incluido o desglosado según los registros.
- Importe cobrado.
- Saldo pendiente.

No sumar el IVA dos veces.

Conservar las tasas y condiciones históricas de los productos.

No aplicar automáticamente tasas fiscales actuales a ventas antiguas.

### Cuentas divididas

La división y combinación de cuentas no deben generar ventas adicionales.

Un producto comercial vendido debe contabilizarse una sola vez, independientemente de cuántas cuentas lo hayan contenido.

### Modificaciones

Los reportes deben reflejar anulaciones, cancelaciones y devoluciones mediante las reglas históricas del sistema.

No eliminar registros anteriores para mostrar únicamente un total neto sin trazabilidad.

---

# 9. REPORTES DE PRODUCTOS Y CATEGORÍAS

Implementar:

- Productos más vendidos.
- Productos menos vendidos.
- Cantidades por categoría.
- Ingresos por producto.
- Ingresos por categoría.
- Variantes vendidas.
- Combos vendidos.
- Complementos seleccionados.
- Productos cancelados.
- Productos con descuentos.
- Tendencias de demanda.

### Combos

Definir claramente cuándo se cuenta:

- Un combo comercial vendido.
- Los componentes utilizados para preparación.
- Los insumos consumidos.

No convertir automáticamente los componentes del combo en ventas individuales.

La suma de los ingresos de los componentes no puede duplicar el precio del combo.

### Historial

Si un producto fue renombrado, los reportes históricos deben conservar su identidad y los datos registrados cuando se realizó la operación.

No perder ventas por desactivar o archivar un producto.

---

# 10. REPORTES DE PAGOS Y DEVOLUCIONES

Implementar:

- Cobros por método.
- Cobros por caja.
- Cobros por turno.
- Cobros por usuario autorizado.
- Pagos mixtos.
- Pagos parciales.
- Devoluciones.
- Correcciones de pagos.
- Importes pendientes.
- Propinas recibidas.

### Monedas

La etapa 6.1 incorporó recepción de pesos y dólares.

Los reportes deben distinguir:

- Importe comercial en MXN.
- Efectivo físico recibido en MXN.
- Efectivo físico recibido en USD.
- Cambio entregado en cada moneda.
- Equivalencias con el tipo de cambio histórico.

No sumar directamente dólares y pesos como cantidades físicas.

No recalcular pagos históricos con el tipo de cambio actual.

### Propinas

**Las propinas se reportan separadas de las ventas.**

Pueden tener su propia distribución por método de pago, caja, turno y periodo.

No inventar un módulo de reparto de propinas.

---

# 11. REPORTES DE DESCUENTOS Y CORTESÍAS

La etapa 7.1 incorporó un catálogo de motivos de descuento.

Cada descuento conserva el nombre que tenía su motivo cuando fue registrado.

**Los reportes deben utilizar ese nombre histórico guardado.**

No depender únicamente del nombre actual del catálogo.

Permitir consultar:

- Descuentos por periodo.
- Descuentos por motivo.
- Descuentos por usuario.
- Descuentos por producto.
- Descuentos generales de cuenta.
- Cortesías.
- Porcentaje de descuentos sobre ventas.
- Importe total descontado.
- Autorizaciones de supervisor, cuando corresponda.

### Motivos históricos

Los descuentos anteriores a la incorporación del catálogo pueden no tener un motivo estructurado.

Deben clasificarse de manera comprensible, por ejemplo:

«Sin motivo del catálogo».

No atribuirles artificialmente un motivo actual.

### Permisos

No mostrar detalles de autorización a usuarios sin permiso suficiente.

---

# 12. REPORTES DE CAJA

Reutilizar las operaciones reales de las etapas 6 y 7.

Incluir:

- Aperturas.
- Fondos iniciales.
- Movimientos.
- Ventas cobradas.
- Ingresos adicionales.
- Retiros.
- Gastos.
- Propinas.
- Devoluciones.
- Cortes parciales.
- Cierres definitivos.
- Sobrantes.
- Faltantes.
- Historial por caja.
- Historial por turno.
- Operaciones por responsable.
- Ventas y cobros por turno.
- **Vista por día** de cobros y movimientos de efectivo, con la hora de inicio del día, para restaurantes que trabajan con un turno permanente (§0-B). En esta vista, el efectivo esperado y las diferencias siguen sujetos a los mismos permisos y nunca se muestran para un turno en arqueo.

### Cierre ciego y permisos

La etapa 7 implementó cierre ciego y permisos separados para consultar importes esperados y diferencias.

**El módulo de Reportes debe respetar exactamente esa protección.**

No basta con ocultar una columna.

El backend debe impedir que una consulta no autorizada devuelva:

- Efectivo esperado.
- Diferencias protegidas.
- Equivalencias que permitan deducir el esperado.
- Agregados o desgloses que permitan reconstruirlo durante un arqueo ciego.

### Exportación

Los archivos PDF y Excel tampoco deben contener datos protegidos si el usuario no tiene autorización.

No permitir que un reporte consolidado revele datos que el usuario no podría consultar en Caja.

---

# 13. REPORTES DE INVENTARIOS

Reutilizar las entidades implementadas en la etapa 8.

Reportes:

- Existencias actuales.
- Existencias por almacén.
- Insumos negativos.
- Insumos bajo mínimo.
- Kardex.
- Entradas.
- Salidas.
- Transferencias.
- Ajustes.
- Mermas.
- Conteos.
- Diferencias de conteo.
- Consumo por periodo.
- Consumo por producto.
- Consumo por ingrediente.
- Consumo asociado a cocina.
- Valuación.
- Costos promedio.

### Consumo

El consumo se genera desde la liberación canónica a cocina, mediante el proceso de inventario existente.

No contabilizar dos veces una unidad porque fue enviada a varias estaciones.

### Consumo pendiente

Si existen eventos de consumo pendientes o fallidos, el reporte debe mostrar claramente que los datos pueden estar incompletos.

No presentar un costo definitivo cuando todavía falta procesar movimientos relevantes.

### Mermas

Conservar las clasificaciones existentes:

- Preparado / merma.
- No preparado.
- Otras categorías registradas por el módulo de inventario.

No confundir una devolución financiera con merma.

### Permisos

Separar:

- Consulta de cantidades.
- Consulta de costos.
- Consulta de valuaciones.

Respetar los permisos existentes de inventario.

No revelar costos mediante alertas, exportaciones, gráficos o agregados a usuarios sin autorización.

---

# 14. COSTOS Y MÁRGENES

Implementar reportes de:

- Costo teórico por producto.
- Costo teórico por receta.
- Consumo valuado.
- Costo por categoría.
- Costo de mermas.
- Costos históricos.
- Valuación de existencias.
- Margen estimado por producto.
- Margen estimado por categoría.
- Margen estimado por periodo.

### Reglas

Utilizar los costos históricos y movimientos reales del inventario.

No calcular el costo de una venta antigua usando únicamente el costo promedio actual.

No equiparar precio de compra, precio de venta y costo de ingredientes.

Separar adecuadamente descuentos, impuestos y valores netos.

No incluir gastos generales del restaurante como si fueran ingredientes.

### Costo provisional

La etapa 8 contempla costos provisionales y conciliaciones cuando existen saldos negativos.

Los reportes deben respetar esa condición.

Cuando no exista información suficiente para calcular un margen confiable:

- Mostrar un estado pendiente o estimado.
- Informar qué dato falta.
- Evitar presentar una utilidad definitiva inventada.

Distinguir margen bruto teórico de utilidad contable.

No desarrollar contabilidad general.

---

# 15. REPORTE DE COMPRAS POR PERIODO

**Este reporte es obligatorio y debe revisar el funcionamiento del sistema anterior.**

El sistema C# tenía reportes de compras o entradas de mercancía por periodo.

Consultar las referencias correspondientes en `CLAUDE.md` y el código original disponible.

Implementar un reporte útil para conocer cuánto compró el restaurante durante determinado periodo.

### Filtros

- Fecha inicial.
- Fecha final.
- Proveedor.
- Insumo.
- Categoría.
- Almacén.
- Estado de compra.
- Modalidad de recepción.

### Información

- Fecha.
- Folio.
- Proveedor.
- Insumo.
- Cantidad.
- Unidad.
- Costo unitario.
- Descuento de compra.
- Impuestos registrados.
- Importe.
- Recepciones.
- Total por proveedor.
- Total del periodo.

### Distinción importante

Una orden emitida no significa mercancía recibida.

Permitir consultar claramente:

- Compras ordenadas.
- Mercancía recibida.
- Compras pendientes de recepción.

No sumar una misma recepción dos veces.

No contabilizar el gasto de caja vinculado como una segunda compra.

### Compras de mostrador

Permitir filtrar y distinguir las compras realizadas mediante el proveedor genérico o modalidad de mostrador.

### Exportaciones

Este reporte debe poder exportarse a PDF y Excel.

No limitarlo a una consulta visual en la aplicación.

---

# 16. REPORTES POR EMPLEADO — PROPUESTA COMPLEMENTARIA

Existe interés en consultar reportes por empleado, como hacía el sistema anterior.

Este punto se considera una ampliación útil de la etapa 9, pero no debe introducir atribuciones comerciales incorrectas.

Revisar qué relaciones de usuarios existen actualmente.

Distinguir, cuando los datos permitan hacerlo:

- Mesero que capturó.
- Usuario que confirmó.
- Cajero que cobró.
- Usuario que aplicó descuentos.
- Usuario que realizó devoluciones.
- Responsable de movimientos de caja.

No atribuir automáticamente una venta al cajero que recibió el pago si el reporte pretende analizar productos capturados por el mesero.

No contar dos veces la venta cuando participaron varios usuarios.

Durante la revisión técnica, determinar qué reportes por empleado pueden implementarse de manera confiable.

No inventar estadísticas de productividad que no estén respaldadas por eventos existentes.

---

# 17. EXPORTACIÓN A PDF

Implementar exportación PDF desde los reportes.

### Requisitos

- Encabezado con establecimiento.
- Nombre del reporte.
- Periodo.
- Filtros aplicados.
- Fecha de generación.
- Tablas y totales.
- Paginación.
- Encabezados repetidos cuando corresponda.
- Formato monetario correcto.
- Separación de monedas.
- Presentación profesional.

Permitir formatos de página razonables según el contenido, como carta o A4.

Los reportes anchos deben adaptarse sin cortar columnas o importes.

### Seguridad

El PDF debe generarse a partir de datos autorizados.

No enviar todos los datos al frontend y confiar en que React oculte columnas protegidas.

No incluir datos personales innecesarios.

No permitir acceso a exportaciones mediante identificadores predecibles sin autorización.

### Consistencia

El contenido exportado debe corresponder al mismo conjunto de filtros, definiciones y totales que el reporte de pantalla.

No recalcular importes con reglas distintas.

---

# 18. EXPORTACIÓN A EXCEL

Implementar exportación real a formato `.xlsx`.

No limitarse a archivos CSV con extensión de Excel.

### Requisitos

- Encabezados claros.
- Datos tabulares.
- Formatos numéricos correctos.
- Monedas.
- Fechas.
- Totales.
- Hoja de filtros o metadatos cuando resulte conveniente.
- Nombre del establecimiento.
- Periodo del reporte.
- Encabezados distinguibles.
- Anchos de columna adecuados.
- Congelación de encabezados cuando corresponda.

Utilizar valores numéricos exactos o representaciones apropiadas para preservar la precisión financiera.

No convertir identificadores, teléfonos o códigos con ceros iniciales en números que pierdan información.

### Seguridad de archivos

Escapar correctamente cualquier texto procedente de usuarios que pudiera interpretarse como fórmula de Excel.

No permitir inyección de fórmulas mediante nombres de productos, proveedores, observaciones u otros campos.

No incluir fórmulas externas, macros ni contenido ejecutable.

### Filtros y permisos

El archivo debe contener solamente la información autorizada.

No permitir exportar costos, valuaciones o datos protegidos de caja sin los permisos correspondientes.

---

# 26. INTERFAZ DE REPORTES

Mantener una interfaz comercial elegante y práctica.

### Requisitos

- Pantalla completa nativa.
- Diseño adaptable.
- Temas claro y oscuro.
- Logotipos correspondientes.
- Tablas con filtros.
- Búsquedas.
- Paginación.
- Columnas legibles.
- Gráficos claros.
- Indicadores con contexto.
- Estados de carga.
- Estados vacíos.
- Mensajes de error.
- Exportación accesible según permisos.

El usuario debe poder elegir filtros y actualizar un reporte sin tener que navegar por múltiples formularios.

### Rendimiento

No cargar miles de registros en React para calcular todos los totales.

Utilizar agregaciones en Go/PostgreSQL y paginación del detalle.

Optimizar índices y consultas.

Evitar consultas costosas que bloqueen cobros, pedidos o cocina.

Considerar rangos de fechas extensos y reportes con gran volumen de movimientos.

---

# 27. PERMISOS Y AUDITORÍA

Reutilizar los permisos existentes.

Extenderlos cuando sea necesario para:

- Consultar reportes.
- Consultar ventas.
- Consultar pagos.
- Consultar descuentos.
- Consultar devoluciones.
- Consultar reportes de caja.
- Consultar costos.
- Consultar valuaciones.
- Consultar inventarios.
- Consultar compras.
- Consultar proveedores.
- Exportar PDF.
- Exportar Excel.
- Administrar plantillas.
- Previsualizar documentos.
- Imprimir muestras.
- Modificar identidad documental.
- Consultar reportes de empleados.

Respetar siempre los permisos más específicos.

Por ejemplo:

Tener permiso para consultar ventas no implica permiso para consultar costos ni diferencias de caja.

Tener permiso para exportar Excel no implica permiso para exportar todos los datos existentes.

Los endpoints deben verificar permisos en Go.

No confiar solamente en la interfaz.

Registrar auditoría de:

- Exportaciones sensibles.
- Cambios de plantillas.
- Cambios de logotipo.
- Reimpresiones.
- Impresiones de muestra.
- Modificaciones de configuración.
- Consultas sensibles cuando el sistema ya tenga esa política.

No registrar innecesariamente datos personales o secretos.

---

# 28. CONSISTENCIA DE REPORTES Y EXPORTACIONES

Cada reporte debe definir una fuente de datos y una semántica única.

La pantalla, el PDF y el Excel deben utilizar la misma lógica de consulta.

### Totales

Verificar que:

- Total de filas y total resumido coincidan.
- Los filtros se apliquen de manera idéntica.
- La paginación no altere totales.
- Las divisiones de cuenta no dupliquen ventas.
- Los pagos mixtos no dupliquen ingresos.
- Los descuentos no se sumen dos veces.
- Las compras vinculadas a caja no dupliquen egresos.
- Las devoluciones se reflejen correctamente.
- Los costos provisionales se distingan de los definitivos.
- Las monedas no se mezclen.

### Fotografía consistente

Para exportaciones grandes, evitar que el encabezado y el detalle correspondan a distintos momentos debido a operaciones concurrentes.

Utilizar una estrategia consistente de lectura o generación.

No mantener bloqueos operativos largos de PostgreSQL durante la creación de archivos.

---

# 29. API Y EVENTOS

Mantener la API `/api/v1`.

Agregar rutas versionadas para:

- Catálogo de reportes disponibles.
- Filtros.
- Consultas agregadas.
- Desgloses.
- Dashboard.
- Exportaciones.
- Configuración documental.
- Plantillas.
- Vista previa.
- Impresión de prueba.
- Identidad visual del establecimiento.

Utilizar contratos claros y tipados.

No crear endpoints genéricos donde el cliente envíe SQL arbitrario.

### SSE

Reutilizar la infraestructura SSE cuando resulte útil para:

- Actualización de indicadores.
- Cambios de configuración.
- Plantillas.
- Estados de exportación prolongada, si corresponde.

No utilizar SSE como sustituto de consultas históricas.

La base de datos sigue siendo la autoridad.

No introducir servicios externos innecesarios.

---

# 30. MIGRACIONES

La versión de referencia tiene esquema 14.

La siguiente migración disponible, de no existir otra posterior, será **0015**.

Verifica la estructura real antes de crear nuevas migraciones.

No modificar las ya aplicadas.

Crear únicamente las tablas o columnas necesarias para:

- Ajustes de precio por producto.
- Campos adicionales de proveedores.
- Proveedor o modalidad de compras de mostrador.
- Categorías de insumos, si no existen.
- Configuración de identidad documental.
- Plantillas.
- Versiones de plantillas.
- Referencias a documentos generados.
- Configuración de reportes.
- Otros elementos estrictamente necesarios.

No duplicar datos transaccionales históricos en tablas de reportes sin una justificación de rendimiento y una política de actualización consistente.

### Seguridad

Antes de modificar una instalación real:

- Realizar respaldo.
- Ensayar restauración.
- Verificar conteos e integridad.
- Aplicar migraciones en el procedimiento controlado existente.
- Comprobar compatibilidad.
- Ejecutar regresiones.
- Conservar una vía de recuperación.

No asumir que una autorización para migraciones anteriores se extiende automáticamente a las de esta etapa.

---

# 31. PRUEBAS OBLIGATORIAS

Implementar pruebas reales para todas las áreas principales.

### Ajustes de catálogo e inventarios

- Precio con piso actual.
- Opción de reducción desactivada.
- Opción activada.
- Diferencia positiva.
- Diferencia negativa.
- Límite de cero.
- Límite por valor de complementos restantes.
- Extras y promociones.
- IVA.
- Persistencia histórica de precios.
- Recepción directa con Compras de mostrador.
- Campos nuevos de proveedor.
- Código interno sugerido.
- Categoría de insumo creada desde el formulario.
- Navegación agrupada de inventarios.

### Reportes comerciales

- Ventas por periodo.
- Ventas por producto.
- Ventas por categoría.
- Ventas de combos sin duplicar componentes.
- Cuentas divididas sin doble conteo.
- Pagos parciales.
- Pagos mixtos.
- Devoluciones.
- Descuentos y cortesías.
- Motivos históricos.
- Propinas separadas.
- Importes pendientes.
- Fechas comerciales.

### Reportes de caja

- Turnos.
- Cortes parciales.
- Cierre definitivo.
- Diferencias por moneda.
- Tipo de cambio histórico.
- Efectivo esperado protegido.
- Reporte denegado por permisos.
- PDF y Excel sin exposición de importes prohibidos.
- Arqueo ciego sin filtración de información.

### Inventarios y compras

- Existencias por almacén.
- Kardex.
- Costos.
- Valuaciones.
- Consumos.
- Mermas clasificadas.
- Conteos.
- Inventario negativo.
- Costos provisionales.
- Compras por periodo.
- Recepciones parciales.
- Compra con gasto vinculado sin duplicación.
- Restricción de costos por permisos.

### Exportación PDF

- Filtros correctos.
- Totales consistentes.
- Paginación.
- Encabezados.
- Separación de monedas.
- Restricciones por permisos.
- Archivos grandes.
- Datos históricos.
- Caracteres y acentos.

### Exportación Excel

- Archivo `.xlsx` válido.
- Tipos de datos.
- Fechas.
- Importes.
- Códigos con ceros iniciales.
- Textos que parecen fórmulas.
- Totales.
- Filtros.
- Seguridad de columnas.
- Precisión monetaria.

### Decisiones de §0-B

- Venta contada al cerrar la cuenta, en su día de cierre; cuentas abiertas en «En curso».
- Devolución reflejada en su propio día sin alterar el día de la venta.
- Hora de inicio del día en 00:00 y en 05:00: venta de la 1:30 en el día correcto; cambiar la hora no reescribe registros.
- Reportes de ventas, inventarios y compras sin ningún turno abierto ni cerrado.
- Caja por turno y por día con un turno permanente de varios días.
- Semana del año ISO, incluido el cambio de año.
- Precio: opción desactivada, quitar o sustituir nunca baja; activada, baja con sus límites; orden complementos → promoción → descuento manual sin doble descuento.
- Margen con partidas sin consumo («Sin costo»), con costo pendiente («Estimado») y con costo firme; ajustes de costo como concepto del periodo.
- PDF y Excel generados en Go sin columnas protegidas para un usuario sin permiso, aunque llame a la API directamente.
- Reporte de un año con volumen simulado sin bloquear un cobro concurrente.

### Regresiones generales

- Catálogo.
- Personalizaciones.
- Combos.
- Mesas.
- Captura de pedidos.
- Cocina.
- Comandas.
- Cobros.
- Caja.
- Inventarios.
- Compras.
- Usuarios.
- Permisos.
- SSE.
- Go race detector y vet.
- PostgreSQL.
- Vitest.
- TypeScript estricto.
- Rust/Tauri.
- Chromium.
- WebKit.
- Compilación macOS.
- Independencia del servidor.

No afirmar que una prueba se ejecutó si únicamente se diseñó.

---

# 32. PRUEBAS FÍSICAS PENDIENTES

Conservar identificadas las pruebas todavía no realizadas:

- Impresión real de tickets.
- Impresión real de precuentas.
- Impresión de comandas.
- Impresión de cortes.
- Cajón conectado a impresora de red.
- Cajón conectado a impresora USB mediante CUPS.
- Pantalla táctil física.
- Dos terminales reales.
- Comportamiento de la ventana nativa en distintas condiciones.

Estas pruebas deberán realizarse cuando el hardware y el entorno estén disponibles.

No bloquear automáticamente el desarrollo de la etapa 9 por no disponer del hardware.

Tampoco afirmar que esas pruebas se completaron mediante adaptadores simulados.

Registrar claramente las limitaciones.

---

# 33. AJUSTES MENORES CONOCIDOS

Existen dos problemas documentados de la etapa 7.1:

### Texto «Poner fuera de servicio»

A 1280 × 720 el texto del botón puede ocupar tres líneas.

Ajustar su presentación sin cambiar las reglas de negocio ni regresar a un diseño limitado a esa resolución.

### Vigencia de precuenta

Existe una posible condición de carrera si una cuenta se modifica en el mismo instante en que se registra la precuenta impresa.

Revisar el uso de marcas de tiempo para determinar su vigencia.

Preferir una referencia de revisión o versión de cuenta cuando resulte apropiado.

No introducir una segunda fuente de verdad.

Incluir pruebas concurrentes para evitar que una precuenta obsoleta se presente como vigente.

Estos ajustes son secundarios frente al desarrollo principal, pero pueden incorporarse si su solución resulta acotada y segura.

---

# 34. EXCLUSIONES

No implementar todavía:

- Identidad visual del establecimiento, logotipo y diseñador de documentos (etapa 9B).
- Rediseño global horizontal del menú principal.
- Aplicación Android.
- Aplicación KDS independiente.
- Servidor comercial de licencias.
- Instaladores Windows.
- Migrador del antiguo SQL Server.
- Contabilidad general.
- Conciliación bancaria externa.
- Facturación electrónica completa.
- Nómina.
- Distribución de propinas.
- Integraciones bancarias automáticas.
- Sistema completo de cuentas por pagar.

No introducir funcionalidades ficticias para anticipar etapas posteriores.

---

# 36. DOCUMENTACIÓN

Crear una guía específica, por ejemplo:

`docs/REPORTS.md`

Documentar:

- Arquitectura de reportes.
- Definiciones de métricas.
- Fechas comerciales.
- Filtros.
- Ventas.
- Pagos.
- Descuentos.
- Propinas.
- Caja.
- Inventarios.
- Compras.
- Costos.
- Permisos.
- Exportaciones.
- Seguridad.
- Pruebas.
- Limitaciones.

Actualizar los documentos técnicos existentes según corresponda:

- `CLAUDE.md`.
- `AGENTS.md`.
- `README.md`.
- `ARCHITECTURE.md`.
- `API_CONTRACTS.md`.
- `NEXT_STEPS_FOR_SOL.md`.
- `docs/VERIFICATION.md`.

No documentar como implementado algo que todavía no funcione.

---

# CRITERIO FINAL DE ACEPTACIÓN

Debo poder abrir AD-EL POS en mi Mac y utilizar el sistema de reportes con información real.

Debo poder:

- Consultar un dashboard ejecutivo.
- Filtrar ventas por periodo.
- Consultar productos y categorías.
- Analizar cobros y devoluciones.
- Consultar descuentos por motivo histórico.
- Consultar propinas separadas de ventas.
- Consultar caja por turno y fecha comercial.
- Ver diferencias de caja por moneda, con los permisos correspondientes.
- Consultar inventarios.
- Consultar costos autorizados.
- Consultar mermas.
- Consultar compras por periodo y proveedor.
- Exportar reportes a PDF.
- Exportar reportes a Excel.
- Utilizar todas estas funciones respetando los permisos.

Adicionalmente, debo poder:

- Configurar por producto si su precio puede disminuir al cambiar complementos.
- Registrar compras de mostrador sin un proveedor formal.
- Capturar los nuevos datos de proveedores.
- Utilizar códigos internos sugeridos.
- Administrar categorías de insumos.
- Crear categorías directamente desde el formulario.
- Navegar por Inventarios mediante secciones agrupadas y ordenadas.

Las operaciones comerciales, financieras y de inventario anteriores deben permanecer intactas.

**No quiero maquetas, reportes con datos inventados, exportaciones que omitan permisos ni un editor que solamente permita modificar código de plantillas.**

Implementa, integra, prueba y documenta la etapa 9A completa por fases (0 → E) en la rama `etapa-9a`, respetando las decisiones de §0-B, reutilizando la arquitectura existente y preservando la integridad histórica de AD-EL POS.