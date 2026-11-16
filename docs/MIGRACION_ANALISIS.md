# Migración del sistema anterior · Análisis

Etapa 9H, Fase 0 (§1.2 de [etapas/ETAPA_9H.md](etapas/ETAPA_9H.md)). Estructura, correspondencia, conteos y rangos de
fechas del sistema anterior (C# / Windows Forms / SQL Server 2012) para planear su importación en AD-EL POS. **Este
documento solo contiene nombres de tablas y columnas, conteos y rangos de mes y año**: ningún dato de clientes, ventas,
usuarios ni contraseñas (§0 de la especificación). Las cifras de dinero agregadas, cuando hacen falta, están en el
avance ([ETAPA_9H_AVANCE.md](ETAPA_9H_AVANCE.md)), nunca aquí.

**Fuentes.**

- El **código** de `../legacy-csharp/PoS/` (fuera del repositorio, de solo lectura; CLAUDE.md §21): el script
  `database/Base de Datos.sql` y las consultas SQL de los formularios (`*.cs`). Es una **versión antigua**: el respaldo
  tiene objetos que el código no usa. Las citas `archivo:línea` de esta sección son de esa carpeta.
- El **respaldo real** (`possys.bak`, base de origen `possys`, SQL Server 2012, versión mayor 11, respaldo de 11/2026),
  restaurado en un SQL Server 2022 temporal (Express, sin red, respaldo montado de solo lectura, compatibilidad 110,
  intercalación `SQL_Latin1_General_CP1_CI_AS`), consultado **solo con consultas agregadas** (conteos, mínimos y
  máximos de fechas, agrupaciones por año, tasa, tipo o bandera). Nada extraído del respaldo se guardó fuera del
  contenedor.

**Resumen.**

- 52 tablas: las 48 del script y 4 que el script no tiene (`categorias_impresora`, `impresoras_poscommands`,
  `pagos_productos` y `sysdiagrams`). Sin vistas, sin disparadores y sin procedimientos propios: los 7 procedimientos y la
  función del respaldo son los de los diagramas de SQL Server Management Studio (`sp_*diagram*`, `fn_diagramobjects`).
- Ventas del **10/2019 al 11/2026** (226 170 ventas en 85 meses; sin ventas en 07/2020), 722 400 renglones, 328 489
  complementos, 225 810 pagos, 256 cortes, 269 retiros (cortes parciales), 1 119 cancelaciones y 31 563 modificaciones de
  cantidades. El catálogo: 5 categorías, 107 productos, 367 precios con vigencia, 6 complementos y 77 relaciones
  producto–complemento. 25 mesas (una es «para llevar»), 3 cajas, 7 usuarios y 9 personas.
- Sin uso (0 filas): inventario real (1 artículo y 1 existencia, sin recetas, entradas ni ajustes), proveedores,
  promociones, servicios, pedido especial (`ventas_especial`), salidas de efectivo (`salidas`), tope de caja, imágenes y
  `pagos_productos`.
- **IVA**: el sistema guardaba el IVA calculado en cada precio, renglón, complemento y venta, pero **casi todo el
  catálogo tenía tasa 0 %** (365 de 367 precios de producto); el IVA distinto de cero viene de complementos al 16 % (hasta
  07/2023) y de un producto al 16 % (desde 11/2025). No aparece el 8 %. Ver §3.
- **Dólares**: no quedaban registrados. Un pago en dólares se guardaba como «Efectivo» por su equivalente en pesos, sin
  moneda ni tipo de cambio (§4.5).

## 1. Estructura

### 1.1 Tipos y convenciones del sistema anterior

- Llaves enteras con `IDENTITY` (sin UUID). Dinero en `MONEY` (4 decimales); cantidades en `NUMERIC(18,3)`; banderas en
  `TINYINT` (0/1); fechas en `DATETIME` **sin zona horaria**, con la hora local del equipo servidor (`GETDATE()`).
- Ninguna llave foránea tiene acciones en cascada (todas `NO ACTION`) y ninguna está deshabilitada ni sin verificar: no
  hay renglones huérfanos posibles en las relaciones declaradas.
- Varias tablas no tienen llave primaria (`ventas_complementos`, `ventas_modificaciones`, `descuentos`, los precios, las
  relaciones producto–complemento, `recetas`, `inventario`, `salidas` y otras): la importación las identifica por su
  posición o por la combinación de columnas.
- Los nombres de dos llaves foráneas están invertidos en el script (`FK_descuentos_venta` es la de `tipo` y
  `FK_descuentos_servicio` la de `venta`); no afecta a los datos.

### 1.2 Diferencias entre el respaldo y `Base de Datos.sql`

| Objeto                         | En el respaldo                                                               | En `Base de Datos.sql` | ¿Lo usa el código?                                                                            |
| ------------------------------ | ---------------------------------------------------------------------------- | ---------------------- | --------------------------------------------------------------------------------------------- |
| `ventas.corte`                 | `INT` nulo, FK → `cortes`                                                    | no existe              | sí: se escribe en cada pago (`CapturaOrden.cs:2375`, `Pedidos.cs:139`, `PuntoDeVenta.cs:241`) |
| `ventas.cancelado`             | `TINYINT NOT NULL` pred. 0                                                   | no existe              | solo se lee (`RegistroVentas.cs:128-140`); **ningún camino lo escribe** y vale 0 en todas     |
| `pagos.corte`                  | `INT` nulo, FK → `cortes`                                                    | no existe              | **no** (el `INSERT INTO pagos` del código no la lleva): señal de una versión más nueva        |
| `cortes.fecha_inicio`          | `DATETIME` nulo                                                              | no existe              | sí (`IniciarCorte.cs:233`, `RegistroVentas.cs:337`)                                           |
| `cortes.fecha_fin`             | `DATETIME` nulo                                                              | no existe              | sí (`CerrarCorte.cs:256`)                                                                     |
| `cortes.retiros`               | `MONEY` nulo                                                                 | no existe              | sí (`CerrarCorte.cs:256`)                                                                     |
| `cortes.diferencia`            | `MONEY` nulo                                                                 | no existe              | sí (`CerrarCorte.cs:256`)                                                                     |
| `cortes.status`                | `VARCHAR(1)` nulo, sin predeterminado; valores `A` (abierto) y `C` (cerrado) | `TINYINT` pred. 1      | sí, como texto (`PuntoDeVenta.cs:825`, `IniciarCorte.cs:254`, `CerrarCorte.cs:266`)           |
| `impresoras.poscommand`        | `TINYINT` nulo, FK → `impresoras_poscommands`                                | no existe              | no                                                                                            |
| `categorias_impresora`         | tabla (`categoria` → `categorias`, `impresora` → `impresoras`), 0 filas      | no existe              | no                                                                                            |
| `impresoras_poscommands`       | tabla (`id`, `com`), 2 filas                                                 | no existe              | no                                                                                            |
| `pagos_productos`              | tabla (`pago` → `pagos`, `producto` → `ventas_productos`, importes), 0 filas | no existe              | no                                                                                            |
| `sysdiagrams` y procedimientos | diagramas de SSMS                                                            | no existen             | no (herramienta, no del sistema)                                                              |
| `info`                         | 14 claves (`conf_*`)                                                         | 12 claves              | sí (configuración del establecimiento; las dos nuevas son `conf_paginaweb` y `conf_colonia`)  |

Del lado contrario, todo lo que el script define existe en el respaldo, y las consultas del código no usan ninguna tabla
ni columna que falte en el respaldo. Comportamientos del respaldo que el código disponible no explica (versión más
nueva): renglones con cantidad 0 que se conservan (el código los borraba, `CapturaOrden.cs:1643-1675`), complementos con
importe negativo (el código los dejaba en 0, `ArticuloLista.cs:184`), pagos con su corte y pagos con tarjeta casi solo
desde 2024 (ver §4).

### 1.3 Tablas, columnas, claves, relaciones y filas

Las columnas sin «nulo» son `NOT NULL`; «pred.» es el valor predeterminado. **[nueva]** marca lo que el respaldo tiene y
`Base de Datos.sql` no.

#### Ventas e historial

- **`ventas`** · filas **226 170** · PK `id`, única (folio, fecha) · `caja` → `cajas`; `cajero` → `usuarios`; `corte` → `cortes` **[nueva]**; `mesa` → `mesas`.
  Columnas: `id` INT IDENTITY, `caja` SMALLINT, `mesa` SMALLINT, `nombre` VARCHAR(50) nulo, `folio` VARCHAR(12), `fecha` DATETIME, `cajero` SMALLINT, `facturado` TINYINT nulo (pred. 0), `subtotal` MONEY, `iva` MONEY, `total` MONEY, `montoPagado` MONEY nulo (pred. 0.00), `pagado` TINYINT nulo (pred. 0), `extra` VARCHAR(250) nulo, `montoExtra` MONEY nulo (pred. 0), `enmesa` TINYINT nulo (pred. 0), `corte` INT nulo **[no está en Base de Datos.sql]**, `cancelado` TINYINT (pred. 0) **[no está en Base de Datos.sql]**.
- **`ventas_productos`** · filas **722 400** · PK `id` · `producto` → `productos`; `venta` → `ventas`.
  Columnas: `id` INT IDENTITY, `venta` INT, `producto` INT, `cantidad` NUMERIC(18,3), `cantidadOriginal` NUMERIC(18,3), `subtotal` MONEY, `iva` MONEY, `total` MONEY, `observaciones` VARCHAR(80) nulo (pred. '').
- **`ventas_complementos`** · filas **328 489** · sin PK · `complemento` → `complementos`; `producto` → `ventas_productos`.
  Columnas: `producto` INT, `complemento` INT, `incluido` NUMERIC(18,3), `pedido` NUMERIC(18,3), `subtotal` MONEY, `iva` MONEY, `total` MONEY.
- **`ventas_modificaciones`** · filas **31 563** · sin PK · `autorizacion` → `usuarios`; `producto` → `ventas_productos`.
  Columnas: `producto` INT, `cantAnt` NUMERIC(18,3), `cantNueva` NUMERIC(18,3), `autorizacion` SMALLINT, `fecha` DATETIME nulo (pred. GETDATE()).
- **`ventas_especial`** · filas **0** · sin PK · `ingrediente` → `articulos`; `producto` → `productos`; `venta` → `ventas`.
  Columnas: `venta` INT, `producto` INT, `ingrediente` INT.
- **`ventas_servicios`** · filas **0** · sin PK · `servicio` → `servicios`; `venta` → `ventas`.
  Columnas: `venta` INT, `servicio` INT, `cantidad` NUMERIC(18,3) nulo, `subtotal` MONEY, `iva` MONEY, `total` MONEY.
- **`descuentos`** · filas **50** · sin PK · `venta` → `ventas`; `tipo` → `tipos_descuentos`.
  Columnas: `tipo` SMALLINT, `venta` INT, `descripcion` VARCHAR(255) nulo (pred. ''), `descuento` MONEY.
- **`tipos_descuentos`** · filas **3** · PK `id` · sin llaves foráneas.
  Columnas: `id` SMALLINT IDENTITY, `tipo` VARCHAR(30).
- **`cancelaciones`** · filas **1 119** · PK `id` · `autorizo` → `usuarios`; `venta` → `ventas`.
  Columnas: `id` INT IDENTITY, `venta` INT, `autorizo` SMALLINT, `fecha` DATETIME.

#### Pagos

- **`pagos`** · filas **225 810** · PK `id` · `corte` → `cortes` **[nueva]**; `recibio` → `usuarios`; `tipo` → `tipos_pagos`; `venta` → `ventas`.
  Columnas: `id` INT IDENTITY, `venta` INT, `folio` VARCHAR(20) nulo (pred. ''), `tipo` TINYINT nulo (pred. 1), `monto` MONEY nulo (pred. 0.00), `recibio` SMALLINT, `fecha` DATETIME, `corte` INT nulo **[no está en Base de Datos.sql]**.
- **`pagos_productos`** **[tabla que no está en Base de Datos.sql]** · filas **0** · sin PK · `pago` → `pagos`; `producto` → `ventas_productos`.
  Columnas: `pago` INT, `producto` INT, `cantidad` NUMERIC(18,3), `subtotal` MONEY, `iva` MONEY, `total` MONEY.
- **`tipos_pagos`** · filas **2** · PK `id` · sin llaves foráneas.
  Columnas: `id` TINYINT IDENTITY, `tipo` VARCHAR(25).
- **`divisas`** · filas **2** · PK `id` · sin llaves foráneas.
  Columnas: `id` SMALLINT IDENTITY, `divisa` VARCHAR(30), `simbolo` VARCHAR(5), `cambio` MONEY.

#### Caja y cortes

- **`cortes`** · filas **256** · PK `id` · `autorizacion` → `usuarios`; `caja` → `cajas`; `cajero` → `usuarios`.
  Columnas: `id` INT IDENTITY, `cajero` SMALLINT, `inicio_caja` MONEY, `fin_caja` MONEY nulo (pred. 0), `venta` MONEY nulo (pred. 0), `caja` SMALLINT, `status` VARCHAR(1) nulo **[en Base de Datos.sql: TINYINT pred. 1]**, `autorizacion` SMALLINT nulo, `fecha_inicio` DATETIME nulo **[no está en Base de Datos.sql]**, `fecha_fin` DATETIME nulo **[no está en Base de Datos.sql]**, `retiros` MONEY nulo **[no está en Base de Datos.sql]**, `diferencia` MONEY nulo **[no está en Base de Datos.sql]**.
- **`cortes_parciales`** · filas **269** · PK `id` · `autorizacion` → `usuarios`; `corte` → `cortes`.
  Columnas: `id` INT IDENTITY, `corte` INT, `monto` MONEY, `razon` VARCHAR(255), `fecha` DATETIME, `autorizacion` SMALLINT.
- **`salidas`** · filas **0** · sin PK · `autorizacion` → `usuarios`; `corte` → `cortes`.
  Columnas: `corte` INT, `monto` MONEY, `fecha` DATETIME, `autorizacion` SMALLINT.
- **`topes_caja`** · filas **0** · PK `id` · sin llaves foráneas.
  Columnas: `id` SMALLINT IDENTITY, `tope` MONEY, `fecha` DATETIME.
- **`cajas`** · filas **3** · PK `id`, única machine_id · `impresora_caja` → `impresoras`; `impresora_cocina` → `impresoras`.
  Columnas: `id` SMALLINT IDENTITY, `machine_id` VARCHAR(30), `caja` VARCHAR(30), `fecha_alta` DATETIME, `impresora_caja` TINYINT nulo, `impresora_cocina` TINYINT nulo.

#### Catálogo

- **`categorias`** · filas **5** · PK `id` · sin llaves foráneas.
  Columnas: `id` SMALLINT IDENTITY, `categoria` VARCHAR(50), `cocina` TINYINT nulo (pred. 0).
- **`productos`** · filas **107** · PK `id` · `categoria` → `categorias`; `unidad` → `unidades`.
  Columnas: `id` INT IDENTITY, `categoria` SMALLINT, `producto` VARCHAR(50), `nombre_corto` VARCHAR(25) nulo, `unidad` VARCHAR(4), `portada` VARBINARY(MAX) nulo.
- **`productos_precios`** · filas **367** · sin PK · `producto` → `productos`.
  Columnas: `producto` INT, `fecha_inicio` DATETIME, `vigencia` DATETIME nulo, `subtotal` MONEY, `iva` MONEY, `total` MONEY.
- **`complementos`** · filas **6** · PK `id` · sin llaves foráneas.
  Columnas: `id` INT IDENTITY, `complemento` VARCHAR(30).
- **`complementos_precios`** · filas **28** · sin PK · `id` → `complementos`.
  Columnas: `id` INT, `fecha_inicio` DATETIME, `vigencia` DATETIME nulo, `subtotal` MONEY, `iva` MONEY, `total` MONEY.
- **`productos_complementos`** · filas **77** · sin PK · `complemento` → `complementos`; `producto` → `productos`.
  Columnas: `producto` INT, `complemento` INT, `cantidad` NUMERIC(18,2) nulo (pred. 0.00).
- **`productos_imagenes`** · filas **0** · sin PK · `producto` → `productos`.
  Columnas: `producto` INT, `imagen` VARBINARY(MAX) nulo, `posicion` TINYINT nulo (pred. 0).
- **`promociones`** · filas **0** · PK `id` · `producto` → `productos`.
  Columnas: `id` INT IDENTITY, `producto` INT, `promocion` MONEY, `inicio` DATETIME nulo, `vigencia` DATETIME nulo.
- **`servicios`** · filas **0** · PK `id` · sin llaves foráneas.
  Columnas: `id` INT IDENTITY, `servicio` VARCHAR(50), `descripcion` VARCHAR(255) nulo, `fechaInicio` DATETIME, `vigencia` DATETIME, `precio` MONEY nulo.
- **`unidades`** · filas **6** · PK `id` · sin llaves foráneas.
  Columnas: `id` VARCHAR(4), `unidad` VARCHAR(20) nulo.

#### Mesas

- **`mesas`** · filas **25** · PK `id` · sin llaves foráneas.
  Columnas: `id` SMALLINT IDENTITY, `mesa` VARCHAR(30) nulo, `capacidad` TINYINT nulo (pred. 4).

#### Personal y usuarios

- **`puestos`** · filas **5** · PK `id`, única puesto · sin llaves foráneas.
  Columnas: `id` TINYINT IDENTITY, `puesto` VARCHAR(50), `funciones` VARCHAR(255) nulo.
- **`personal`** · filas **9** · PK `id` · `puesto` → `puestos`.
  Columnas: `id` INT IDENTITY, `nombre` VARCHAR(50), `paterno` VARCHAR(30) nulo, `materno` VARCHAR(30) nulo, `fechaAlta` DATE nulo, `puesto` TINYINT, `activo` TINYINT nulo (pred. 1).
- **`personal_detalles`** · filas **9** · sin PK · `ciudad` → `ciudades`; `id` → `personal`.
  Columnas: `id` INT, `calle` VARCHAR(40) nulo, `num_ext` VARCHAR(6) nulo, `num_int` VARCHAR(6) nulo, `colonia` VARCHAR(50) nulo, `ciudad` INT, `cp` CHAR(5) nulo, `telefono` VARCHAR(13) nulo, `celular` VARCHAR(13) nulo, `email` VARCHAR(100) nulo.
- **`usuarios`** · filas **7** · PK `id`, única usuario · `personal` → `personal`.
  Columnas: `id` SMALLINT IDENTITY, `usuario` VARCHAR(30), `password` VARCHAR(128), `personal` INT, `activo` TINYINT nulo (pred. 1).

#### Inventario y compras

- **`articulos`** · filas **1** · PK `id`, única articulo · `tipo` → `articulos_tipos`; `unidad` → `unidades`.
  Columnas: `id` INT IDENTITY, `articulo` VARCHAR(50), `tipo` SMALLINT, `unidad` VARCHAR(4), `registrado` DATETIME nulo (pred. GETDATE()).
- **`articulos_tipos`** · filas **6** · PK `id` · sin llaves foráneas.
  Columnas: `id` SMALLINT IDENTITY, `tipo` VARCHAR(30), `descripcion` VARCHAR(255) nulo.
- **`articulos_imagenes`** · filas **0** · sin PK · `articulo` → `articulos`.
  Columnas: `articulo` INT, `imagen` VARBINARY(MAX) nulo.
- **`inventario`** · filas **1** · sin PK · `articulo` → `articulos`.
  Columnas: `articulo` INT, `stock` NUMERIC(18,3) nulo, `ultimaModificacion` DATETIME nulo (pred. GETDATE()).
- **`ajustes_inventario`** · filas **0** · PK `id` · sin llaves foráneas.
  Columnas: `id` INT IDENTITY, `articulo` INT, `cantidad` NUMERIC(18,3), `fecha` DATETIME nulo (pred. GETDATE()), `descripcion` VARCHAR(255) nulo.
- **`recetas`** · filas **0** · sin PK · `articulo` → `articulos`; `producto` → `productos`.
  Columnas: `producto` INT, `articulo` INT, `cantidad` NUMERIC(18,3), `ultimaModificacion` DATETIME nulo (pred. GETDATE()).
- **`entradas`** · filas **0** · PK `id` · `proveedor` → `proveedores`; `recibio` → `usuarios`.
  Columnas: `id` INT IDENTITY, `recibio` SMALLINT, `proveedor` INT, `subtotal` MONEY, `iva` MONEY, `total` MONEY, `fecha` DATETIME.
- **`entradas_detalles`** · filas **0** · sin PK · `articulo` → `articulos`; `id` → `entradas`.
  Columnas: `id` INT, `articulo` INT, `cantidad` NUMERIC(18,3), `precioUnitario` MONEY, `subtotal` MONEY, `iva` MONEY, `total` MONEY.
- **`proveedores`** · filas **0** · PK `id`, única rfc · sin llaves foráneas.
  Columnas: `id` INT IDENTITY, `rfc` VARCHAR(14), `proveedor` VARCHAR(50), `fechaRegistrado` DATE nulo (pred. GETDATE()).
- **`proveedores_detalles`** · filas **0** · sin PK · `id` → `proveedores`; `ciudad` → `ciudades`.
  Columnas: `id` INT, `representante` VARCHAR(30) nulo, `calle` VARCHAR(40) nulo, `num_ext` VARCHAR(6) nulo, `num_int` VARCHAR(6) nulo, `colonia` VARCHAR(50) nulo, `ciudad` INT, `cp` CHAR(5) nulo, `telefono1` VARCHAR(13) nulo, `telefono2` VARCHAR(13) nulo, `celular` VARCHAR(13) nulo, `email` VARCHAR(100) nulo, `paginaweb` VARCHAR(80) nulo.

#### Impresión y equipos

- **`impresoras`** · filas **3** · PK `id` · `poscommand` → `impresoras_poscommands` **[nueva]**.
  Columnas: `id` TINYINT IDENTITY, `impresora` VARCHAR(30), `ubicacion` VARCHAR(30), `ip` VARCHAR(15), `cajaRegistradora` TINYINT nulo (pred. 0), `poscommand` TINYINT nulo **[no está en Base de Datos.sql]**.
- **`impresoras_poscommands`** **[tabla que no está en Base de Datos.sql]** · filas **2** · PK `id` · sin llaves foráneas.
  Columnas: `id` TINYINT, `com` VARCHAR(20).
- **`categorias_impresora`** **[tabla que no está en Base de Datos.sql]** · filas **0** · sin PK · `categoria` → `categorias`; `impresora` → `impresoras`.
  Columnas: `categoria` SMALLINT, `impresora` TINYINT.

#### Configuración y catálogos geográficos

- **`info`** · filas **14** · sin PK, única info · sin llaves foráneas.
  Columnas: `info` VARCHAR(30), `valor` VARCHAR(255).
- **`paises`** · filas **1** · PK `id` · sin llaves foráneas.
  Columnas: `id` SMALLINT IDENTITY, `pais` VARCHAR(30), `abbr` VARCHAR(3) nulo.
- **`estados`** · filas **1** · PK `id` · `pais` → `paises`.
  Columnas: `id` INT IDENTITY, `estado` VARCHAR(30), `pais` SMALLINT.
- **`ciudades`** · filas **1** · PK `id` · `estado` → `estados`.
  Columnas: `id` INT IDENTITY, `ciudad` VARCHAR(30), `estado` INT.
- **`colonias`** · filas **0** · PK `id` · `ciudad` → `ciudades`.
  Columnas: `id` INT IDENTITY, `colonias` VARCHAR(30), `ciudad` INT.

#### Herramienta (SQL Server Management Studio)

- **`sysdiagrams`** **[tabla que no está en Base de Datos.sql]** · filas **1** · PK `diagram_id`, única (principal_id, name) · sin llaves foráneas.
  Columnas: `name` SYSNAME, `principal_id` INT, `diagram_id` INT IDENTITY, `version` INT nulo, `definition` VARBINARY(MAX) nulo.

## 2. Qué representa cada tabla y su concepto en AD-EL POS

Tablas de AD-EL POS: las de `backend/migrations/` (esquema `app`). La columna «Destino propuesto» resume §6.

| Tabla anterior                                                 | Qué representa                                                                           | Concepto y tablas de AD-EL POS                                                                                                         | Destino propuesto                               |
| -------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------- |
| `ventas`                                                       | Cuenta (mesa o para llevar) con sus totales guardados, estado de pago y corte            | Cuenta: `restaurant_accounts`; venta reconocida al cerrar: `sales_recognitions`                                                        | Historial importado                             |
| `ventas_productos`                                             | Renglón: producto, cantidad actual y original, importes y observaciones                  | Partida: `restaurant_items`, `restaurant_item_versions` (JSON `request`/`quote_line`) y una fila por unidad en `restaurant_item_units` | Historial importado                             |
| `ventas_complementos`                                          | Complementos de un renglón: cantidad incluida, pedida e importe del extra                | Selección de un grupo de opciones dentro de `restaurant_item_versions.request`/`quote_line` (sin tabla propia)                         | Historial importado                             |
| `ventas_modificaciones`                                        | Cambio de cantidad de un renglón con quién lo autorizó                                   | Eventos `item.modified`/`item.cancelled` de `restaurant_events`; reporte «Modificaciones de cantidades»                                | Historial importado                             |
| `ventas_especial`                                              | Ingredientes quitados (pedido especial)                                                  | Ingredientes removibles: `catalog_product_ingredients` y la personalización de la partida                                              | No se importa (0 filas)                         |
| `ventas_servicios`, `servicios`                                | Servicios cobrados con vigencia                                                          | Sin equivalente directo (lo más cercano: cargo de cuenta en `account_adjustments`)                                                     | No se importa (0 filas)                         |
| `descuentos`, `tipos_descuentos`                               | Un descuento por importe a la cuenta, con tipo y descripción                             | Descuento de cuenta: `account_adjustments` (`kind='discount'`) y motivos `discount_reasons`                                            | Historial importado                             |
| `ventas.extra`, `ventas.montoExtra`                            | Un cargo extra a la cuenta, con descripción                                              | Cargo de cuenta: `account_adjustments` (`kind='charge'`, siempre con tasa de IVA)                                                      | Historial importado                             |
| `cancelaciones`                                                | Cuenta cancelada y quién autorizó                                                        | Cancelación de cuenta: `account_cancellations` con motivo de `account_cancel_reasons`                                                  | Historial importado                             |
| `pagos`                                                        | Pago (efectivo o tarjeta) con importe, quién lo recibió, fecha, corte y folio de tarjeta | Pago: `payments` (+ `payment_allocations` por unidad y `cash_movements`)                                                               | Historial importado                             |
| `tipos_pagos`                                                  | Forma de pago (2: efectivo y tarjeta)                                                    | Métodos de pago: `payment_methods` (efectivo, débito, crédito, transferencia, plataformas)                                             | Correspondencia (§6.5)                          |
| `divisas`                                                      | Un tipo de cambio global, sin historial                                                  | Tipo de cambio por turno: `cash_shifts.usd_rate`; por pago: `payments.usd_rate`                                                        | No se importa                                   |
| `pagos_productos`                                              | Pago por renglón                                                                         | `payment_allocations`                                                                                                                  | No se importa (0 filas)                         |
| `cortes`                                                       | Turno de caja de un cajero en una caja: fondo, venta, retiros, conteo y diferencia       | Turno: `cash_shifts` (+ `cash_shift_closures`, `cash_counts`, `cash_shift_reports`)                                                    | Historial importado                             |
| `cortes_parciales`                                             | Retiro de efectivo durante el turno, con razón y autorización                            | Operación de caja: `cash_operations` (`kind='withdrawal'`)                                                                             | Historial importado                             |
| `salidas`                                                      | Salida de efectivo                                                                       | `cash_operations` (`withdrawal`/`expense`)                                                                                             | No se importa (0 filas)                         |
| `topes_caja`                                                   | Tope de efectivo                                                                         | Función activable `cash_limit`                                                                                                         | No se importa (0 filas)                         |
| `cajas`                                                        | Equipo que vende (una es la aplicación móvil, `MOBILE`) con sus impresoras               | Caja `registers` y terminal `devices`                                                                                                  | Solo como nombre en el historial                |
| `categorias`                                                   | Categoría y si se envía a cocina                                                         | `catalog_categories`; envío a cocina: rutas `kitchen_routes`                                                                           | Catálogo                                        |
| `productos`                                                    | Producto con nombre corto, unidad e imagen                                               | `catalog_products`                                                                                                                     | Catálogo                                        |
| `productos_precios`                                            | Precio con fecha de inicio (historial de precios con subtotal, IVA y total)              | `catalog_products.base_price` y `catalog_price_entries` (sin vigencias ni historial)                                                   | Precio vigente al catálogo; historial importado |
| `complementos`, `complementos_precios`                         | Complemento y su precio con fecha de inicio                                              | `catalog_complements` y su valor en `catalog_group_options`                                                                            | Catálogo                                        |
| `productos_complementos`                                       | Complementos que admite un producto y cuántos incluye                                    | `catalog_option_groups`, `catalog_group_options`, `catalog_product_groups` y la cantidad incluida en `catalog_group_defaults`          | Catálogo                                        |
| `productos_imagenes`, `productos.portada`                      | Imágenes del producto                                                                    | `catalog_images`                                                                                                                       | No se importa (sin imágenes)                    |
| `promociones`                                                  | Precio promocional con vigencia                                                          | `catalog_promotions`                                                                                                                   | No se importa (0 filas)                         |
| `unidades`                                                     | Unidad de venta del producto y del artículo (`pza`, `g`, `ml`…)                          | Solo inventario: `inventory_units` (el producto no tiene unidad)                                                                       | No se importa                                   |
| `mesas`                                                        | Mesa con capacidad; la `id` 1 es «para llevar»                                           | `restaurant_tables` en un área `restaurant_areas`; para llevar es la modalidad `takeaway`                                              | Mesas                                           |
| `puestos`                                                      | Puesto (decide quién autoriza)                                                           | Rol: `roles`                                                                                                                           | Correspondencia de roles (§6.4)                 |
| `usuarios`                                                     | Usuario con contraseña, persona y estado                                                 | `users`                                                                                                                                | Usuarios sin contraseña y sin acceso            |
| `personal`, `personal_detalles`                                | Persona (nombre, alta, puesto) y su domicilio y contacto                                 | Sin equivalente (solo `users.display_name`)                                                                                            | Solo el nombre visible del usuario              |
| `articulos`, `articulos_tipos`, `inventario`                   | Insumo, su tipo y su existencia                                                          | `inventory_items`, `inventory_categories`, `inventory_stock`                                                                           | No se importa (1 artículo; ver §7)              |
| `ajustes_inventario`, `recetas`                                | Ajuste de existencia; receta por producto                                                | `inventory_adjustments`; `inventory_recipes` y versiones                                                                               | No se importa (0 filas)                         |
| `entradas`, `entradas_detalles`                                | Compra recibida (en la interfaz anterior, «Egresos»)                                     | `inventory_receipts`, `inventory_receipt_lines`                                                                                        | No se importa (0 filas)                         |
| `proveedores`, `proveedores_detalles`                          | Proveedor y su domicilio y contacto                                                      | `inventory_suppliers`                                                                                                                  | No se importa (0 filas)                         |
| `impresoras`, `impresoras_poscommands`, `categorias_impresora` | Impresoras de red, juego de comandos y categorías por impresora                          | `kitchen_printers`, `kitchen_stations`, `kitchen_routes`                                                                               | No se importa (configuración del equipo)        |
| `info`                                                         | Datos del establecimiento (`conf_*`)                                                     | `establishment` y `document_identity`                                                                                                  | No se importa (se capturan en AD-EL POS)        |
| `paises`, `estados`, `ciudades`, `colonias`                    | Catálogo geográfico de domicilios                                                        | Sin equivalente                                                                                                                        | No se importa                                   |
| `sysdiagrams`                                                  | Diagrama de SQL Server Management Studio                                                 | —                                                                                                                                      | No se importa                                   |

## 3. IVA

### 3.1 Según el código

- **Captura del precio** (`Agregar_Producto.cs`, `ModificarProducto.cs`; los complementos igual en
  `Agregar_Complemento.cs` y `ModificarComplemento.cs`): subtotal, «IVA %» (predeterminado 16, editable, admite 0) y
  total. Se captura el subtotal o el total y el otro se calcula: `total = subtotal × (1 + tasa/100)`
  (`Agregar_Producto.cs:465-470`) o `subtotal = total / (1 + tasa/100)` mostrado con dos decimales
  (`Agregar_Producto.cs:526`). Se guardan `subtotal`, `iva` y `total` del precio, con `iva = subtotal × tasa / 100` sin
  redondear (`Agregar_Producto.cs:325-333`; `MONEY` lo deja en 4 decimales). **La tasa no se guarda**: se deduce como
  `iva / subtotal × 100` (`ModificarProducto.cs:43`, `ModificarComplemento.cs:43`).
- **El precio que paga el cliente es el total** (IVA incluido cuando la tasa no es 0): los botones y los renglones usan el
  total (`CapturaOrden.cs:474-486`).
- **Cada venta guardaba el IVA calculado** en tres niveles: el renglón, `subtotal`, `iva` y `total` unitarios por la
  cantidad (`CapturaOrden.cs:921-923`); el complemento, `(pedido − incluido) × precio × cantidad del renglón`
  (`ArticuloLista.cs:165-200`); y la venta, la suma en `double` de renglones y complementos (`CapturaOrden.cs:483-500`,
  `527-535`), recalculada al modificar (`CapturaOrden.cs:1556`). No hay redondeo explícito: los `double` se convierten a
  `MONEY` (4 decimales) al guardar.
- El descuento y el cargo extra no tienen IVA ni cambian `ventas.iva` (`CapturaOrden.cs:2798`, `2849-2873`).
- Se usa el precio **más reciente** (`TOP 1 … ORDER BY fecha_inicio DESC`, `Seleccionar_Producto.cs:116`, `169`), no el
  vigente por fecha; la columna `vigencia` nunca se usa.

### 3.2 En los datos

- **Catálogo**: 365 de los 367 precios de producto tienen IVA 0 (subtotal = total) y 2 el 16 %; con el precio más
  reciente, 106 productos quedan al 0 % y 1 al 16 %. De los 28 precios de complemento, 13 tienen 0 % y 15 el 16 % (con
  fechas de 03/2020 a 07/2023); los 6 complementos quedan al 0 % con su precio más reciente.
- **Renglones vendidos**: 722 365 con IVA 0 y 35 con IVA (1 en 2019, 6 en 2025 y 28 en 2026, todos del mismo producto);
  150 renglones tienen subtotal 0.
- **Complementos vendidos**: 15 438 con IVA al 16 % (de 2020 a 2023); desde 2024, ninguno.
- **Ventas**: 6 360 ventas con IVA distinto de cero (todas por complementos o por el producto al 16 %): la tasa
  efectiva sobre el subtotal es menor que 0.5 % en 3 366, de 0.5 % a 5.5 % en 2 924 y mayor en 2; 68 tienen IVA negativo
  (de 2020 a 2023, por complementos con importe negativo). Desde 2024 el IVA de las ventas es 0 salvo en 33.
- **No aparece la tasa de 8 %** de la región fronteriza norte en ningún precio, renglón, complemento ni venta.
- Hay importes con más de dos decimales por el `MONEY`: 3 286 IVA y 1 904 subtotales de venta, 7 024 IVA y 1 842
  subtotales de complemento, y 19 totales de venta.

### 3.3 Conclusión

El IVA era **por producto** (y por complemento) en el sentido de que cada precio guardaba su propio desglose, pero en la
práctica **el catálogo se capturó con tasa 0 %**: los precios se cobraban tal cual (total = subtotal) y el IVA registrado
es casi siempre 0. El IVA distinto de cero proviene de los complementos capturados con el 16 % predeterminado (hasta
07/2023) y de un producto al 16 % (desde 11/2025). **Sí hay IVA guardado en cada venta** (aunque sea 0), así que la regla
de «IVA estimado» de §4.2 («si el sistema anterior no guardaba el IVA de cada venta») no aplica de forma literal, y la
tasa «vigente» de cada producto en el sistema anterior es 0 % para casi todo. Qué tasa recibe el catálogo importado y si
se estima algún IVA son decisiones del propietario (§7, preguntas 1 y 2).

## 4. Cómo se registraban

Según el código (versión antigua) y, entre paréntesis, lo que confirman los datos.

### 4.1 Ventas y renglones

- Una venta se crea al enviar la primera orden (`CapturaOrden.cs:770-1040`, y la aplicación móvil en
  `appFunctions.cs:556-910`), en una transacción: folio, venta, renglones, complementos y descuento. `fecha` es la de
  apertura; no hay fecha de cierre (la del último pago es la mejor aproximación).
- `subtotal`, `iva` y `total` de la venta son la suma de renglones y complementos, **sin** descuento ni cargo extra; lo que
  se cobra es `total − descuento + montoExtra` (`PagarEfectivo.cs:205`). `montoPagado` acumula los pagos y `pagado` pasa a
  1 con el pago completo (`CapturaOrden.cs:2375`). (`montoPagado` coincide con la suma de los pagos en todas las ventas.)
- Agregar productos a una venta abierta crea renglones con `cantidadOriginal = 0` (`CapturaOrden.cs:1907`; 14 896
  renglones); cambiar la cantidad actualiza `cantidad` e importes y registra `ventas_modificaciones` con la cantidad
  original, la nueva y quién autorizó (`CapturaOrden.cs:1593-1714`). En esta versión, bajar a 0 borraba el renglón; en
  los datos hay 31 renglones con cantidad 0 que se conservan.
- Observaciones de texto por renglón (`ventas_productos.observaciones`, 27 052 con texto); nombre de la cuenta en
  `ventas.nombre` (148 434 con texto).

### 4.2 Complementos y pedido especial

- Un producto admite complementos (`productos_complementos`) con una cantidad incluida (51 relaciones con 0 incluidos y
  26 con 1). En la captura, cada complemento lleva `incluido` y `pedido`; se cobra `(pedido − incluido) × precio`. En
  esta versión del código, si la suma resultaba negativa todos quedaban en 0 (`ArticuloLista.cs:184`); **en los datos
  hay 14 587 complementos con importe negativo** (se quitó uno incluido y se bonificó), 18 238 con cargo positivo y
  295 664 en 0.
- El pedido especial (`ventas_especial`, quitar ingredientes) no se usó (0 filas).

### 4.3 Descuentos, cargos extra, servicios, propinas y facturación

- **Descuento**: uno por venta, por importe, con tipo (`Atencion`, `Promocion`, `Demora`) y descripción; se reemplaza al
  cambiarlo y no puede exceder el total (`CapturaOrden.cs:2716-2759`, `2834-2853`). (50 descuentos; ninguno mayor que su
  venta.)
- **Cargo extra**: uno por venta, descripción e importe en la propia venta (`CapturaOrden.cs:2798`). (2 623 ventas; 1 sin
  descripción y 2 con descripción sin importe.)
- **Servicios**: tablas sin uso (0 filas).
- **Propinas**: la pantalla de cobro las calcula y muestra, pero **no se guardan** (`PagarEfectivo.cs:235-248`; el pago
  registra solo lo que se debe, `PagarEfectivo.cs:678`).
- **Facturado**: `ventas.facturado` se escribe siempre en 0 (`CapturaOrden.cs:810`) y vale 0 en todas.

### 4.4 Cancelaciones

- **Cuenta**: con la autorización de un usuario de puesto «Administrador» (`AutPuntoDeVenta.cs:145-161`), la cancelación
  pone `pagado = 1` y agrega `cancelaciones(venta, autorizo, fecha)` (`PuntoDeVenta.cs:1020-1060`, `Pedidos.cs:716-742`,
  `CapturaOrden.cs:2602-2628`). No guarda motivo ni quién canceló. **`ventas.cancelado` nunca se escribe** (vale 0 en
  las 226 170), así que el «Registro de ventas» anterior (`pagado = 1 AND cancelado = 0`, `RegistroVentas.cs:128-140`)
  **contaba las canceladas como vendidas**. (1 119 canceladas, de 10/2019 a 07/2026, ninguna con pagos; 680 tienen corte.)
- **Unidades**: no hay cancelación de unidades propiamente; reducir la cantidad de un renglón exige autorización y deja
  `ventas_modificaciones` (31 563: 477 reducciones, 7 188 aumentos y 23 898 sin cambio de cantidad —cambios de
  complementos u observaciones—; ninguna a 0). `cantAnt` es la cantidad original del renglón, no la anterior al cambio.

### 4.5 Formas de pago, tarjeta y dólares

- `tipos_pagos`: 1 «Efectivo» y 2 «Tarjeta Credito/Debito». La tarjeta guarda el folio del comprobante en
  `pagos.folio` (los 4 519 pagos con tarjeta lo tienen; 150 folios se repiten).
- **Pagos con tarjeta casi solo desde 2024** (1 en 06/2023 y después 1 446, 1 711 y 1 361 por año). En esta versión
  del código, tres de los cuatro caminos de `PagarTarjeta.cs` registran el pago como efectivo
  (`PagarTarjeta.cs:283-289`): antes de 2024, los cobros con tarjeta quedaron como «Efectivo» y no se pueden distinguir.
- **Dólares**: `PagoDivisa.cs` y `PagarEfectivo.cs:640-690` convierten con el tipo de cambio global de `divisas` (una sola
  fila para el dólar, sin historial) solo para mostrar el total y el cambio; el pago se registra como **efectivo en
  pesos** por lo que se debía (`PagarEfectivo.cs:678`; en el pago parcial, `monto × tipo de cambio`,
  `PagarEfectivo.cs:725`). No se guarda la moneda, los dólares recibidos ni el tipo de cambio aplicado.
- **Pago parcial y dividir cuenta**: varios pagos por venta (700 ventas con 2 pagos, 20 con 3 y 8 con 4). «Dividir
  cuenta» solo reparte en pantalla y registra un único pago en efectivo por el total (`DividirCuenta.cs:250-310`).

### 4.6 Cortes, turnos, retiros y egresos

- **Corte = turno de un cajero en una caja**: se abre con el fondo (`IniciarCorte.cs:233-254`, `status = 'A'`) y se
  cierra con el conteo (`CerrarCorte.cs:256-272`, `status = 'C'`). La venta del corte es
  `Σ(total + montoExtra) − Σ descuentos` de las ventas con ese `corte` (`CerrarCorte.cs:325-370`) —**incluye tarjeta y
  canceladas**—, los retiros son la suma de sus cortes parciales y la diferencia, `fin_caja − (inicio_caja + venta −
retiros)` (`CerrarCorte.cs:222`, `408-410`). (En 249 de los 251 cortes cerrados la venta guardada coincide con esa
  suma.)
- **Relación con ventas y pagos**: el corte abierto del equipo se guarda en `ventas.corte` en cada pago
  (`CapturaOrden.cs:2375`; queda el del último pago) y, en la versión del respaldo, también en `pagos.corte`. Los pagos de
  una venta capturada en la aplicación móvil quedan en el corte de la caja que cobra (111 467 pagos en un corte de otra
  caja que la de su venta).
- Un corte por cajero y caja a la vez (`PuntoDeVenta.cs:825`); varios cajeros podían tener corte abierto en la misma
  caja (148 pares de cortes traslapados en la misma caja, ninguno del mismo cajero). El cierre lo autoriza el propio
  cajero (en los 251).
- **Retiros**: `cortes_parciales` con importe, razón (texto libre) y quién autorizó (`CorteParcial.cs:232`). (269, de
  02/2022 a 03/2026.)
- **Egresos**: `salidas` no se usa (0 filas). Lo que la interfaz anterior llamaba «Egresos» son las compras recibidas
  (`Egresos.cs:33`, `RegistroEgresos.cs:157`: `entradas`), también con 0 filas.
- Cortes por año (por fecha de inicio): 1 inicial (12/2018, cerrado en 02/2022, con todos los pagos hasta entonces), 111
  en 2022, 114 en 2023, 20 en 2024, 8 en 2025 y 2 en 2026; 5 siguen abiertos (abiertos entre 02/2023 y 10/2026).

### 4.7 Mesas, para llevar y aplicación móvil

- `mesas` (25): la `id` 1 es «para llevar» y no aparece en el mapa (`PuntoDeVenta.cs:182`, `id > 1`); `enmesa` indica si
  la cuenta sigue en la mesa. (132 138 ventas para llevar y 94 032 de mesa; las 412 con `enmesa = 1` son canceladas.)
- Mover una cuenta de mesa actualiza `ventas.mesa` (`PuntoDeVenta.cs:1980`), sin historial.
- La aplicación móvil del sistema anterior (`ServerSocket.cs`, `appFunctions.cs`) usa la caja `MOBILE` (84 156 ventas,
  sin cortes propios).

### 4.8 Usuarios, puestos, personal y clientes

- `usuarios` (7, todos activos y con contraseña) → `personal` (9; 2 sin usuario) → `puestos` (5: Administrador, Cajero,
  Mesero, Cocinero, Limpieza; en los datos, 6 personas con usuario de puesto Administrador y 3 Meseros, uno con
  usuario). No hay permisos: solo el puesto 1 (Administrador) autoriza (`AutPuntoDeVenta.cs:161`).
- `personal_detalles` guarda domicilio y contacto (sin teléfonos ni correos en los datos).
- **No hay clientes**: `ventas.nombre` es el nombre de la cuenta.

### 4.9 Inventario, recetas, compras y proveedores

- El código descontaba existencias por receta al registrar el pago completo (`CapturaOrden.cs:2454-2502`). En los datos
  el inventario prácticamente no se usó: 1 artículo con 1 existencia, sin recetas, entradas, ajustes ni proveedores.

### 4.10 Folios

- Folio de venta = `MAX(folio) + 1` sobre todas las ventas, sin bloqueo (`CapturaOrden.cs:782`, `appFunctions.cs:579`),
  guardado como texto; la única restricción es `(folio, fecha)`. (Folios del 1 al 226 143 sin huecos y 27 repetidos.)
- Los pagos no tienen folio propio (el `folio` del pago es el comprobante de la tarjeta).

## 5. Comprobaciones de los datos

Resultados de las consultas agregadas sobre el respaldo restaurado (conteos). Las sumas de dinero de cada caso están en el
avance. Para la Fase E, cada comprobación se repite sobre el paquete y se compara con lo importado.

| #   | Comprobación                                                 | Resultado                                                                                                                                                                                        |
| --- | ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1   | Rango de fechas de las ventas                                | 10/2019 a 11/2026; 85 meses con ventas (sin ventas en 07/2020)                                                                                                                                   |
| 2   | Ventas por año                                               | 2019: 2 630 · 2020: 12 182 · 2021: 36 144 · 2022: 35 606 · 2023: 38 607 · 2024: 36 824 · 2025: 34 377 · 2026: 29 800                                                                             |
| 3   | Fechas imposibles                                            | Ninguna posterior al respaldo ni anterior a 2019; horas de 06 a 21; ningún pago, cancelación ni modificación anterior a su venta                                                                 |
| 4   | Pagos más de 2 días después de su venta                      | 230                                                                                                                                                                                              |
| 5   | Ventas sin renglones                                         | 22 (ninguna cancelada; total 0)                                                                                                                                                                  |
| 6   | `total` ≠ Σ renglones + Σ complementos                       | 423 ventas (411 no canceladas y 12 canceladas), todas por más de $1; de 2020 a 2026                                                                                                              |
| 7   | `subtotal` ≠ Σ; `iva` ≠ Σ; `subtotal + iva` ≠ `total`        | 428; 192; 121 ventas. En renglones, `subtotal + iva ≠ total` en 151; en complementos, en 87                                                                                                      |
| 8   | Pagos que no suman lo que se debía (pagadas y no canceladas) | Cuadran 224 812. **De más: 219** (215 con un pago repetido por el mismo importe y 204 con exactamente el doble; 107 con pagos en cortes distintos). **De menos: 15** (un solo pago)              |
| 9   | Ventas abiertas al hacer el respaldo                         | 5 (sin pagos)                                                                                                                                                                                    |
| 10  | Ventas canceladas con pagos                                  | 0 (1 119 canceladas; 46 con cargo extra, 99 con modificaciones, ninguna con descuento)                                                                                                           |
| 11  | Canceladas marcadas en `ventas.cancelado`                    | 0: la bandera nunca se usó; la cancelación es la fila de `cancelaciones`                                                                                                                         |
| 12  | Productos borrados con ventas                                | Imposible (llave foránea); 1 producto sin ventas; 9 renglones sin un precio anterior a su venta; 1 107 renglones cuyo precio unitario difiere del vigente por fecha                              |
| 13  | Cantidades                                                   | 31 renglones con cantidad 0 (importe 0); 14 896 agregados después (`cantidadOriginal = 0`); ninguna fraccionaria ni negativa; 1 121 690 unidades                                                 |
| 14  | Importes negativos                                           | 14 587 complementos con total negativo; 68 ventas con IVA negativo; ningún renglón ni total de venta negativo                                                                                    |
| 15  | Pagos en cero o negativos                                    | 539 en cero y 9 negativos (en 536 ventas)                                                                                                                                                        |
| 16  | Importes con más de 2 decimales                              | 19 totales de venta, 1 904 subtotales, 3 286 IVA de venta; 18 pagos; 7 024 IVA y 1 842 subtotales de complemento                                                                                 |
| 17  | Folios repetidos                                             | 27 (226 143 folios distintos para 226 170 ventas)                                                                                                                                                |
| 18  | Pagos y ventas sin corte                                     | 4 254 pagos sin corte (de 02/2022 a 04/2025; 3 668 caen dentro del horario de algún corte); 4 237 ventas pagadas sin corte                                                                       |
| 19  | Pago en un corte distinto del de su venta                    | 563                                                                                                                                                                                              |
| 20  | Cortes                                                       | 251 cerrados y 5 abiertos; 148 pares traslapados en la misma caja; 6 sin ventas ni pagos; 2 con «venta» guardada distinta de la suma; diferencia: 81 con sobrante, 51 con faltante y 119 en cero |
| 21  | Retiros (`cortes_parciales`)                                 | 269; 8 con importe en cero o negativo; 45 de $100 000 o más y 4 de $1 000 000 o más (posibles errores de captura)                                                                                |
| 22  | Usuarios                                                     | 7 activos con contraseña; nombres de usuario de 4 a 13 caracteres sin espacios ni símbolos y sin choque con `admin` o `sa`; 2 personas sin usuario                                               |
| 23  | Nombres repetidos en el catálogo                             | 3 nombres de producto repetidos; ninguna categoría, complemento ni mesa repetida                                                                                                                 |

Consultas de referencia (todas agregadas; nombres de columna del respaldo):

- Totales: `ventas.total` contra `Σ ventas_productos.total + Σ ventas_complementos.total` por venta (con
  `ventas_complementos.producto → ventas_productos.id`).
- Lo que se debía: `ventas.total + ventas.montoExtra − Σ descuentos.descuento` contra `Σ pagos.monto`, separando las
  canceladas (`EXISTS cancelaciones`) y las abiertas (`pagado = 0`).
- Cortes: `cortes.venta` contra `Σ(total + montoExtra − descuento)` de `ventas.corte`; `cortes.retiros` contra
  `Σ cortes_parciales.monto` (coinciden en los 251); traslapes por `caja` entre `fecha_inicio` y `fecha_fin`.
- IVA: `round(iva / subtotal × 100)` agrupado por año en precios, renglones, complementos y ventas.

## 6. Propuesta de correspondencia

### 6.1 Principios (§0-B y §4 de la especificación)

- Se migra todo el historial con **sus importes exactos**: nada se recalcula con las reglas de AD-EL POS. Los importes del
  anterior tienen 4 decimales (`MONEY`): el historial los conserva con 4 decimales y los muestra redondeados a 2.
- El historial importado es de **solo lectura** y queda marcado «Sistema anterior» con su folio y su fecha originales;
  nunca se reabre, se cobra, se cancela ni se modifica, y no altera caja, turnos ni inventario vigentes.
- **Destino propuesto del historial**: tablas propias de historial importado (nombre provisional `legacy_*`), con un
  identificador de lote, que los reportes leen junto con los de AD-EL POS; no las tablas operativas (motivos en el
  avance). Catálogo, mesas y usuarios sí van a las tablas reales, marcados con el lote para poder reemplazarlos.
- Las fechas del anterior son hora local sin zona: se convierten con la zona del establecimiento
  (`app.establishment.timezone`). Solo 1 venta ocurre antes de las 07:00, así que el cambio de horario no causa
  ambigüedad práctica.
- Los datos incoherentes (§5) se importan **tal como están**, con una marca y una nota, y se listan en el informe.

### 6.2 Catálogo

| Anterior                                                 | AD-EL POS                                                                                                                                                                                                        | Notas                                                                                                                                                                                       |
| -------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `categorias.categoria`                                   | `catalog_categories.name`                                                                                                                                                                                        | Únicos en ambos                                                                                                                                                                             |
| `categorias.cocina`                                      | Ruta de cocina de la categoría (`kitchen_routes`), si hay una estación                                                                                                                                           | 4 de 5 van a cocina; impresoras y estaciones no se importan                                                                                                                                 |
| `productos.producto`                                     | `catalog_products.name` (1–100)                                                                                                                                                                                  | 3 nombres repetidos: AD-EL POS los admite (el código es el único)                                                                                                                           |
| `productos.id`                                           | `catalog_products.code` = código generado (p. ej. `SA-<id>`)                                                                                                                                                     | El anterior no tenía código                                                                                                                                                                 |
| `productos.nombre_corto`                                 | Sin columna propia (la comanda usa el nombre)                                                                                                                                                                    | Se puede conservar en la descripción                                                                                                                                                        |
| `productos.categoria`                                    | `catalog_products.category_id`                                                                                                                                                                                   |                                                                                                                                                                                             |
| `productos.unidad`                                       | Sin equivalente en el producto                                                                                                                                                                                   | `pza`, `g`, `ml`: no cambia el precio                                                                                                                                                       |
| Precio más reciente (`productos_precios.total`)          | `catalog_products.base_price` (y la lista predeterminada «Restaurante»)                                                                                                                                          | El total es lo que se cobraba; AD-EL POS siempre trata el precio con IVA incluido                                                                                                           |
| Tasa deducida (`iva / subtotal`)                         | `catalog_products.tax_rate` (0, 8, 16 o `NULL` = tasa general)                                                                                                                                                   | Decisión del propietario (§7, pregunta 1)                                                                                                                                                   |
| Historial de `productos_precios`                         | Sin equivalente (AD-EL POS no guarda vigencias de precio)                                                                                                                                                        | Se conserva en el historial importado para consulta                                                                                                                                         |
| `complementos.complemento`                               | `catalog_complements.name`                                                                                                                                                                                       | Únicos en ambos                                                                                                                                                                             |
| `productos_complementos` (13 productos, 6 complementos)  | Un grupo de opciones `catalog_option_groups` («Complementos») con una opción por complemento (`catalog_group_options`), `catalog_product_groups` por producto y la cantidad incluida en `catalog_group_defaults` | Precio del complemento: valor de referencia u «extra» de la opción; si se reproduce la bonificación al quitar un incluido, «El precio puede bajar al cambiar complementos» (§7, pregunta 6) |
| `productos.portada`, `productos_imagenes`, `promociones` | —                                                                                                                                                                                                                | Sin datos                                                                                                                                                                                   |
| Presentaciones, combos, ingredientes removibles          | —                                                                                                                                                                                                                | El anterior no los tenía                                                                                                                                                                    |

### 6.3 Mesas

| Anterior                       | AD-EL POS                                                                           | Notas                                    |
| ------------------------------ | ----------------------------------------------------------------------------------- | ---------------------------------------- |
| `mesas` con `id > 1` (24)      | `restaurant_tables` en un área nueva (p. ej. «Salón»), con posiciones en cuadrícula | El anterior no tenía áreas ni posiciones |
| `mesas.mesa`                   | `restaurant_tables.name` (1–60, único por área)                                     | Sin repetidos                            |
| `mesas.capacidad` (2 a 8)      | `restaurant_tables.seats` (1–40)                                                    |                                          |
| `mesas.id = 1` («para llevar») | Modalidad `takeaway` de la cuenta (no es mesa)                                      | 132 138 ventas                           |

### 6.4 Usuarios y roles

| Anterior                                | AD-EL POS                                                                                                   |
| --------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| `usuarios.usuario`                      | `users.username` en minúsculas (cumple `^[a-z][a-z0-9_.-]{1,39}$`; si alguno empieza con número, se ajusta) |
| `personal.nombre`, `paterno`, `materno` | `users.display_name` (1–100)                                                                                |
| `usuarios.password`                     | **No se lee ni se migra**: `password_hash` `NULL` (sin acceso)                                              |
| `usuarios.activo`, `personal.activo`    | El usuario queda **sin acceso** hasta que el administrador le asigne contraseña (§7, pregunta 9)            |
| Personas sin usuario (2)                | No se importan (no tienen operación)                                                                        |

Correspondencia de puestos con los roles predeterminados de AD-EL POS (`administration/permissions.go:83-95`):

| Puesto anterior (personas con usuario) | Rol de AD-EL POS (`roles.code`) | Motivo                                                                 |
| -------------------------------------- | ------------------------------- | ---------------------------------------------------------------------- |
| Administrador (6)                      | `administrator` «Administrador» | Autorizaba cancelaciones y modificaciones; decisión en §7, pregunta 10 |
| Cajero (0)                             | `cashier` «Cajero»              |                                                                        |
| Mesero (1)                             | `waiter` «Mesero»               |                                                                        |
| Cocinero (0)                           | `kitchen` «Cocina»              |                                                                        |
| Limpieza (0)                           | `waiter` «Mesero», sin acceso   | No hay rol sin permisos; nadie tiene este puesto con usuario           |

### 6.5 Historial

Cada tabla del historial importado guarda el identificador de lote y el identificador original.

| Anterior                                  | Historial importado                                                                                                                           | Notas                                                                                               |
| ----------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| `ventas.id`, `folio`                      | Identificador y folio originales (texto); se muestran como «Sistema anterior · <folio>»                                                       | No consumen ni chocan con `restaurant_accounts.folio` (identidad de AD-EL POS)                      |
| `ventas.fecha`                            | Apertura                                                                                                                                      | Hora local → zona del establecimiento                                                               |
| Fecha del último pago                     | Cierre (fecha de reconocimiento de la venta)                                                                                                  | §7, pregunta 3                                                                                      |
| `pagado`, `cancelaciones`                 | Estado: pagada, cancelada o abierta                                                                                                           | `cancelado` se ignora (siempre 0); las 5 abiertas quedan «abierta en el sistema anterior»           |
| `mesa`                                    | Modalidad (`takeaway` si `mesa = 1`) y la mesa importada, con su nombre copiado                                                               |                                                                                                     |
| `nombre`                                  | Nombre de la cuenta                                                                                                                           |                                                                                                     |
| `caja`                                    | Nombre de la caja anterior (incluida «aplicación móvil»)                                                                                      | No se crean `registers` ni `devices`                                                                |
| `cajero`                                  | Usuario importado y su nombre copiado                                                                                                         | Quien abrió la cuenta                                                                               |
| `subtotal`, `iva`, `total`                | Exactos (4 decimales)                                                                                                                         | Sin recalcular; IVA tal como se guardó                                                              |
| `extra`, `montoExtra`                     | Cargo de cuenta con su concepto                                                                                                               | Sin IVA propio                                                                                      |
| `descuentos` (tipo, descripción, importe) | Descuento de cuenta con su tipo como motivo                                                                                                   |                                                                                                     |
| `corte`                                   | Turno importado                                                                                                                               | 4 237 sin corte: «Sin turno»                                                                        |
| `facturado`, `enmesa`, `montoPagado`      | No se importan como dato propio (facturado y enmesa no aportan; montoPagado = Σ pagos)                                                        |                                                                                                     |
| `ventas_productos`                        | Renglón: producto importado (y nombre y categoría copiados), cantidad, cantidad original, subtotal, IVA, total, tasa deducida y observaciones | 1 renglón, no una fila por unidad                                                                   |
| `ventas_complementos`                     | Complementos del renglón: complemento, incluido, pedido, subtotal, IVA y total (con signo)                                                    | Los negativos se conservan                                                                          |
| `ventas_modificaciones`                   | Cambio de cantidad: renglón, cantidad original, nueva, quién autorizó y fecha                                                                 | En «Modificaciones de cantidades» con origen «Sistema anterior»; «Modificó» vacío (no se guardaba)  |
| `cancelaciones`                           | Cancelación: quién autorizó y fecha; motivo «Sistema anterior (sin motivo)»                                                                   | En «Cuentas canceladas»; «Canceló» vacío (no se guardaba)                                           |
| `pagos`                                   | Pago: forma de pago, importe exacto (también 0 o negativo), referencia (`folio`), quién recibió, fecha y turno                                | «Efectivo» → método `cash`; «Tarjeta Credito/Debito» → §7, pregunta 7. Sin moneda ni tipo de cambio |
| `cortes`                                  | Turno: caja, cajero, apertura y cierre, fondo, venta, retiros, conteo final, diferencia, estado y quién autorizó, tal como se guardaron       | No se convierten en `cash_shifts` ni se recalcula el esperado de AD-EL POS                          |
| `cortes_parciales`                        | Retiro del turno: importe, razón, fecha y quién autorizó                                                                                      | §7, pregunta 5                                                                                      |

### 6.6 Lo que no tiene equivalente o no se importa

- **Dólares y tipo de cambio**: no existen en los datos (§4.5): los pagos en dólares están dentro de «Efectivo» en pesos.
- **Propinas**: no se guardaban.
- **Tarjeta antes de 2024**: quedó registrada como efectivo; no se puede separar.
- **Egresos**: no hay (`salidas` y `entradas` vacías). Solo hay retiros de caja.
- Configuración del equipo (`cajas`, `impresoras`, `impresoras_poscommands`, `categorias_impresora`, `divisas`, `info`,
  `topes_caja`): se configura de nuevo en AD-EL POS.
- `personal_detalles`, catálogos geográficos, `sysdiagrams`, `unidades`, tablas vacías (`ventas_especial`,
  `ventas_servicios`, `servicios`, `promociones`, `pagos_productos`, `salidas`, `topes_caja`, `entradas`,
  `entradas_detalles`, `ajustes_inventario`, `recetas`, `proveedores`, `proveedores_detalles`, imágenes).
- Inventario (1 artículo y su existencia): §7, pregunta 11.

### 6.7 Validación posterior (§4.4)

Contra los conteos y totales del paquete: productos (107), mesas (24 + para llevar), usuarios (7), ventas (226 170; de
ellas 1 119 canceladas y 5 abiertas), renglones (722 400), complementos (328 489), pagos (225 810), cortes (256) y
retiros (269); **total vendido por año** (`Σ(total + montoExtra − descuento)` de las ventas pagadas no canceladas, por el
año de la fecha que se decida en §7, pregunta 3) y **total por forma de pago** (`Σ pagos.monto` por tipo y año); total
de egresos (0) y de retiros. Las diferencias de §5 (filas 6, 8, 14 y 15) se muestran como tales, nunca se ocultan.

### 6.8 Correspondencia final (Fase C)

Lo que implementa `backend/internal/legacyimport` con las decisiones del propietario y del coordinador
([ETAPA_9H_AVANCE.md](ETAPA_9H_AVANCE.md), Fase 0). Donde difiere de §6.2–§6.7, manda esta sección.

**Catálogo, mesas y usuarios (a lo vivo, identificados por su id anterior en `legacy_id_map`).**

| Anterior                                  | AD-EL POS                                                                                                                                                                                                                                |
| ----------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `categorias`                              | `catalog_categories` (nombre sin espacios de más; repetido → «(2)»), en el orden de su id. `cocina` no se importa (las estaciones y rutas de cocina se configuran en AD-EL POS).                                                         |
| `productos`                               | `catalog_products` con código `SA-<id>`, su categoría (sin categoría → «Sin categoría (sistema anterior)») y `nombre_corto` en la descripción («Nombre corto en el sistema anterior: …») si es distinto del nombre.                      |
| Precio más reciente (`productos_precios`) | `base_price` = `total` redondeado a centavos (aviso si tenía más decimales); `tax_rate` = la tasa de ese precio (`iva / subtotal`, la más cercana de 0, 8 y 16; aviso si no era exacta). Sin precio: $0.00 y **desactivado**, con aviso. |
| `productos_complementos` con incluidos    | `price_can_decrease = true` (decisión 3).                                                                                                                                                                                                |
| `complementos` + `complementos_precios`   | `catalog_complements`; su precio más reciente es el valor de referencia de su opción. Aviso si su tasa no era 0 % (en AD-EL POS el complemento se desglosa con la tasa del producto).                                                    |
| Conjuntos de `productos_complementos`     | Un grupo de modo valor por cada conjunto distinto de complementos (el más usado se llama «Complementos»; los demás, «Complementos: A, B…»), opciones con extra 0 y la cantidad incluida (> 0) en `catalog_group_defaults`.               |
| `mesas` (sin la id 1)                     | `restaurant_tables` en el área «Salón» (y «Salón 2»… cada 48), en una cuadrícula de 8 por renglón de 100 × 72; capacidad 4 si falta, entre 1 y 40.                                                                                       |
| `usuarios` + `personal` + `puestos`       | `users` sin contraseña (`password_hash` nulo), con su preferencia de tema; nombre visible = nombre y apellidos; activo si el usuario y la persona lo están; rol según §6.4 (puesto desconocido → Mesero, con aviso).                     |
| `usuarios.usuario`                        | En minúsculas; si no cumple la regla o choca con `admin`, `sa` u otro: sin acentos, lo demás → «.», «u» al inicio si empieza con número y «-2», «-3»… (aviso).                                                                           |

**Repetir la migración.** Lo que ya existía del lote vigente conserva su identificador y se actualiza en su lugar (por
eso se conservan una contraseña asignada o una receta ligada a un producto importado); lo que ya no viene en el paquete
se borra si nada lo usa o, si algo lo usa, se desactiva con el nombre «<nombre> (retirado …)» y queda en la
correspondencia como `retired`. Un cambio de nombres entre respaldos no choca (renombre en dos fases).

**Historial (tablas `legacy_*`, de solo lectura).** Cada tabla del anterior con historial va a su tabla propia con el id
(o la posición, si no tiene llave) y los importes exactos: `ventas` → `legacy_sales`, `ventas_productos` →
`legacy_sale_lines`, `ventas_complementos` → `legacy_line_complements`, `ventas_modificaciones` →
`legacy_quantity_changes`, `descuentos` + `tipos_descuentos` → `legacy_discounts`, `cancelaciones` →
`legacy_cancellations`, `pagos` → `legacy_payments`, `cortes` → `legacy_cash_shifts`, `cortes_parciales` →
`legacy_cash_withdrawals`, `salidas` → `legacy_cash_expenses` (vacía en el respaldo real) y `productos_precios` +
`complementos_precios` → `legacy_price_history`. `entradas` (compras) no se importa: si un paquete trae alguna, la
validación la muestra como diferencia. Estado de la venta: cancelada si tiene fila en `cancelaciones`; si no, pagada con
`pagado = 1`; si no, abierta. Reconocimiento = último pago (sin pagos, la apertura). Forma de pago: `tipo` 1 o nulo →
«Efectivo» (`cash`), 2 → «Tarjeta (sistema anterior)» (`card`), otro → su nombre (`other`, marcado). Fechas
interpretadas con `establishment.timezone`. Un IVA nulo se estima con la tasa del precio vigente en la fecha de la venta
(marca `tax_estimated`). El libro `legacy_sales_recognitions` se describe en el avance (Fase C).

**Marcas de incoherencia** (lista cerrada, `internal/legacyimport/flags.go`; «lista» = va renglón por renglón en el PDF).

| Entidad             | Marca                                                                                                                                                                                                                                                                                                                                                                                                               |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Venta (lista)       | `total_mismatch`, `subtotal_mismatch`, `tax_mismatch`, `amounts_inconsistent` (subtotal + IVA ≠ total), `no_lines`, `overpaid`, `underpaid`, `no_payments`, `cancelled_with_payments`, `open_with_payments`, `paid_amount_mismatch`, `duplicate_folio`, `tax_estimated`, `negative_tax`, `charge_without_amount`, `charge_without_concept`, `unknown_table`, `unknown_cashier`, `unknown_register`, `unknown_shift` |
| Venta (informativa) | `no_shift`, `late_payment` (último pago más de 2 días después de abrir)                                                                                                                                                                                                                                                                                                                                             |
| Renglón             | `unknown_product`, `unknown_sale` (lista); `amounts_inconsistent`, `zero_quantity`, `rate_not_standard`, `tax_estimated` (informativas)                                                                                                                                                                                                                                                                             |
| Complemento         | `unknown_complement`, `unknown_line` (lista); `negative_amount` (bonificación), `amounts_inconsistent`, `tax_estimated` (informativas)                                                                                                                                                                                                                                                                              |
| Pago                | `zero_amount`, `negative_amount`, `possible_duplicate` (mismo importe que otro de una venta cobrada de más), `unknown_method`, `before_sale`, `unknown_sale` (lista); `no_shift`, `shift_differs` (informativas)                                                                                                                                                                                                    |
| Descuento           | `exceeds_total`, `zero_or_negative`, `unknown_sale` (lista); `unknown_type` (informativa)                                                                                                                                                                                                                                                                                                                           |
| Corte               | `sales_mismatch` (su «venta» guardada ≠ Σ(total + montoExtra − descuentos) de sus ventas, incluidas canceladas), `withdrawals_mismatch`, `unknown_status` (lista); `no_activity` (informativa)                                                                                                                                                                                                                      |
| Retiro              | `zero_or_negative`, `doubtful_amount` ($100 000 o más), `unknown_shift` (lista)                                                                                                                                                                                                                                                                                                                                     |
| Precio              | `rate_not_standard` (informativa)                                                                                                                                                                                                                                                                                                                                                                                   |

**Validación (§4.4).** Conteos del manifiesto del paquete (categorías, productos, complementos, mesas sin «para
llevar», usuarios, ventas por estado, renglones, complementos vendidos, cambios de cantidad, descuentos, cancelaciones,
pagos, cortes, retiros, salidas e historial de precios) contra lo escrito; total vendido por año del día comercial del
reconocimiento (Σ del libro contra Σ(total + montoExtra − descuentos) de las pagadas no canceladas calculado aparte);
pagos por forma de pago y año; total de retiros; total de egresos (salidas; 0 en el respaldo real); y cuántas ventas no
suman en el libro lo que se debía (0). Las canceladas al estilo del anterior y la ausencia de dólares quedan como nota.

## 7. Decisiones que necesita el propietario

Preguntas cerradas, con la opción recomendada; el coordinador las presenta al propietario. Ninguna se decidió aquí.

1. **Tasa de IVA del catálogo importado**: casi todos los productos tenían 0 % en el sistema anterior. ¿(a) Se importan
   con la tasa encontrada (0 % y un producto al 16 %), como dice §4.1, o (b) con «Tasa general del establecimiento»,
   para que las ventas nuevas en AD-EL POS desglosen el IVA con la tasa que se configure en Cobros? Recomendación: (b),
   confirmando la tasa general con el contador; el historial conserva su IVA tal como se guardó.
2. **IVA del historial**: ¿se conserva tal como se guardó (0 en casi todas las ventas) sin estimar nada? Recomendación:
   sí; estimarlo sería recalcular importes, y el sistema anterior sí guardaba el IVA de cada venta.
3. **Fecha de las ventas importadas en los reportes**: AD-EL POS reconoce la venta al cerrar la cuenta; el anterior la
   contaba en la fecha de apertura. ¿Se usa la fecha del último pago (recomendado, igual que AD-EL POS; 230 pagos llegaron
   más de 2 días después) o la de apertura (como los reportes anteriores)?
4. **Ventas canceladas**: el reporte anterior las contaba como vendidas (y el corte las incluía). ¿Se importan como
   cuentas canceladas que no son venta (recomendado, regla de AD-EL POS), aceptando que los totales por año difieran de
   los reportes del sistema anterior por ese importe?
5. **Retiros (`cortes_parciales`)**: ¿se importan como retiros de efectivo del turno (recomendado; la razón es texto
   libre) y no como gastos? ¿Y los 4 retiros de $1 000 000 o más, tal cual con la marca «importe dudoso» (recomendado)?
6. **Complementos incluidos que se quitan**: el anterior bonificaba el complemento incluido que se quitaba (14 587
   casos). ¿Se activa «El precio puede bajar al cambiar complementos» en los productos importados que incluyen
   complementos, para cobrar igual que antes (recomendado), o se deja el comportamiento de AD-EL POS (nunca baja)?
7. **Pagos con tarjeta**: el anterior tenía una sola «Tarjeta Credito/Debito». ¿Se importan con un método propio
   inactivo «Tarjeta (sistema anterior)» (recomendado) o como «Tarjeta de débito» o «de crédito»?
8. **Pagos repetidos** (219 ventas cobradas de más, 204 con exactamente el doble) y **pagos en 0 o negativos** (548):
   ¿se importan tal como están, con la marca «pago posiblemente repetido» o «importe en cero/negativo», y cuentan en los
   reportes de cobros (recomendado), o se excluyen de los reportes?
9. **Usuarios sin acceso**: AD-EL POS no tiene PIN y el administrador no puede asignar la contraseña de otro usuario (solo
   cada quien cambia la suya). ¿Se agrega en la migración «Asignar contraseña» para los usuarios importados (recomendado:
   el administrador asigna una contraseña inicial que el usuario cambia al entrar) o los usuarios importados quedan solo
   como nombres del historial, sin poder entrar nunca?
10. **Rol de los 6 usuarios con puesto «Administrador»**: ¿se importan como Administrador (recomendado: quedan sin acceso
    hasta que les asignes contraseña, y en ese momento puedes cambiarles el rol) o como Cajero?
11. **Inventario**: solo hay 1 artículo con su existencia, sin recetas ni compras. ¿Se omite (recomendado: el libro de
    movimientos de AD-EL POS es de solo inserción y una existencia inicial importada ya no se podría reemplazar al
    repetir la migración) o se importa?
12. **Ventas cuyo total no cuadra con sus renglones** (423, §5 fila 6): ¿(a) los reportes por venta, día, turno y forma
    de pago usan el total que guardó el sistema anterior, los reportes por producto usan los renglones y la diferencia
    aparece como un concepto aparte «Diferencia del sistema anterior», marcada en el informe (recomendado: no se pierde
    ni se inventa dinero); (b) se usan los renglones en todo; o (c) se usa el total en todo y se reparte entre los
    renglones?
13. **Instalación de destino**: la importación exige que no haya operación propia (§4.3). ¿Debe estar vacía también de
    catálogo, mesas y usuarios propios (recomendado: sí, se importa en una instalación nueva o limpia y el catálogo,
    las mesas y los usuarios importados se reemplazan completos en cada repetición), o se combinan con los que ya
    existan?
