# Inventarios, recetas, compras y costos · Etapa 8

Versión 0.9.0, API v1, esquema 17 (inventario en las migraciones `0010` a `0014` y, desde la etapa 9A, `0016`). Las migraciones 0010–0014 se aplicaron a la instalación real el
10 de noviembre de 2026 con autorización del propietario, tras respaldo y ensayo de restauración; «Control de inventario»
quedó desactivado hasta que el restaurante lo active. Este documento describe solo lo
implementado y probado en las ramas `etapa-8` y `etapa-9a`. Las decisiones del propietario están en [etapas/ETAPA_8.md](etapas/ETAPA_8.md)
§0-B; las decisiones menores, las cifras de cada prueba y las mediciones, en [ETAPA_8_AVANCE.md](ETAPA_8_AVANCE.md); los
contratos HTTP completos, en [API_CONTRACTS.md](../API_CONTRACTS.md) (secciones «Inventario · fase A» a «fase D»); la
evidencia de verificación, en [VERIFICATION_STAGE8.md](VERIFICATION_STAGE8.md).

**Etapa 9A (fase A, ajustes de inventario; rama `etapa-9a`).** La migración `0016_inventory_suppliers_counter.sql` agrega
segundo teléfono, celular y página web a los proveedores, el proveedor especial único «Compras de mostrador» con el
comercio concreto en cada recepción directa, y la interfaz agrega el código interno sugerido de insumos (con comprobación
en Go), la administración completa de categorías con alta rápida desde el insumo y la navegación de Inventarios en cinco
grupos. **La 0016 se aplicó a la instalación real el 10 de noviembre de 2026**, junto con la 0015 y la 0017, con
autorización del propietario, respaldo y ensayo de restauración. Las secciones
de abajo marcan lo que cambió con «(9A)». En la fase B de la 9A, el cálculo del día operativo pasó al paquete neutral
`internal/businessday` sin cambiar su comportamiento y su hora de corte (la «Hora de inicio del día») se edita solo en
Configuración general; los reportes de inventarios, costos y márgenes, compras y proveedores están en
[REPORTS.md](REPORTS.md) (ver «Reportes · etapa 9A»).

## Contenido

- [Principios](#principios)
- [Arquitectura y migraciones](#arquitectura-y-migraciones)
- [Función «Control de inventario»](#función-control-de-inventario)
- [Modelo de inventarios](#modelo-de-inventarios)
- [Costo promedio](#costo-promedio)
- [Existencias negativas](#existencias-negativas)
- [Recetas y versiones](#recetas-y-versiones)
- [Subrecetas y elaboraciones](#subrecetas-y-elaboraciones)
- [Porciones habituales](#porciones-habituales)
- [Indicaciones rápidas con efecto](#indicaciones-rápidas-con-efecto)
- [Complementos, combos y empaques para llevar](#complementos-combos-y-empaques-para-llevar)
- [Consumo por liberación a cocina](#consumo-por-liberación-a-cocina)
- [Idempotencia](#idempotencia)
- [Cancelaciones](#cancelaciones)
- [Ajustes y mermas](#ajustes-y-mermas)
- [Transferencias](#transferencias)
- [Conteos físicos](#conteos-físicos)
- [Niveles por almacén, alertas y sugerencias de compra](#niveles-por-almacén-alertas-y-sugerencias-de-compra)
- [Proveedores](#proveedores)
- [Órdenes de compra](#órdenes-de-compra)
- [Recepciones](#recepciones)
- [Devoluciones a proveedor](#devoluciones-a-proveedor)
- [Vinculación con gastos de caja](#vinculación-con-gastos-de-caja)
- [Relación con los demás módulos](#relación-con-los-demás-módulos)
- [Interfaz](#interfaz)
- [API](#api)
- [Permisos](#permisos)
- [Eventos en tiempo real](#eventos-en-tiempo-real)
- [Auditoría](#auditoría)
- [Pruebas](#pruebas)
- [Reportes · etapa 9A](#reportes--etapa-9a)
- [Mediciones del consumo asíncrono](#mediciones-del-consumo-asíncrono)
- [Limitaciones conocidas](#limitaciones-conocidas)
- [Sistema anterior](#sistema-anterior)

## Principios

Reglas obligatorias de la especificación (§2) y decisiones del propietario (§0-B), tal como quedaron implementadas:

- **Las existencias negativas no bloquean las ventas** ni ninguna otra operación de inventario: se informan como aviso.
- **El consumo teórico ocurre al liberar a cocina** (la liberación canónica `kitchen_unit_releases`), no al capturar,
  confirmar sin liberar, cobrar ni imprimir. Una unidad comercial consume una sola vez aunque llegue a varias estaciones.
- **La captura y la liberación no esperan al inventario**: la liberación solo registra una solicitud y un trabajador de Go
  calcula el consumo después (ver [Mediciones](#mediciones-del-consumo-asíncrono)).
- Una devolución financiera no reintegra ingredientes; un gasto de caja no genera una compra y una compra no genera un
  gasto de caja.
- Las recetas usan cantidades configuradas: el mesero nunca captura gramos ni mililitros.
- Las observaciones escritas libremente no modifican el inventario; una indicación rápida solo lo afecta si tiene un efecto
  estructurado y explícito.
- Los movimientos históricos se conservan: el libro es de solo inserción y toda diferencia se expresa con movimientos nuevos.
- Los costos de inventario son independientes de los precios de venta.
- **Go es la autoridad** de cantidades, existencias y costos. React no suma, convierte ni redondea: envía las cadenas
  capturadas y muestra las que devuelve Go.

## Arquitectura y migraciones

- **Paquete `backend/internal/inventory`.** `administration/routes.go` delega `inventory/*` con la misma transacción, el
  `Actor` con permisos efectivos y el callback de auditoría, y traduce `inventory.Error` a errores públicos. `inventory` no
  importa `operations`, `kitchen`, `payments` ni `administration`: lee por SQL, en solo lectura, las versiones de partidas,
  las tareas de cocina y los gastos de caja. La dirección es `kitchen → inventory` (`RecordReleases`, `RecordChange`).
- **Lock 7820148308** (`CLAUDE.md` §3): exclusivo en las escrituras del libro y de los documentos de inventario, compartido
  en las lecturas consistentes; siempre **después** de 7820148305 y 7820148304 cuando concurren. Las rutas que leen gastos
  de caja toman antes 7820148305 compartido. La inserción de solicitudes en la liberación **no** lo toma.
- **Escrituras**: toda escritura de inventario exige `operation_id` (también las entidades administrables) y se guarda en
  `restaurant_operations` con su huella; las entidades administrables llevan además `revision` (409 `REVISION_CONFLICT`).
- **Auditoría** en la misma transacción. Los registros de auditoría de inventario pueden contener costos; solo los consulta
  `audit.read`, que es de administración.
- **Eventos** en el ámbito nuevo `inventory`, sin contenido (ningún costo viaja por SSE).
- **Precisión** (`numeric` en PostgreSQL, `shopspring/decimal` en Go, cadenas en JSON): cantidades `numeric(20,6)` en la
  unidad base del insumo, valores `numeric(20,6)`, costos unitarios `numeric(24,10)`, importes de documentos de compra
  `numeric(18,2)` y precio unitario de compra `numeric(18,6)`. Redondeo mitad alejándose de cero. Go entrega además los
  campos `*_display` (valores a centavos y costos unitarios a cuatro decimales) para que React no redondee.
- **Puente nativo**: 81 operaciones `inventory_*` en la lista cerrada de `desktop/src-tauri/src/api.rs` (un solo
  identificador por ruta y parámetros de consulta en lista blanca), con una prueba Rust por fase. Cuerpos de hasta 2 MiB.
- **Trabajador** `inventory.Worker`, arrancado en `cmd/ad-el-pos-server/main.go` con el mismo contexto cancelable y apagado
  ordenado que los de cocina.

| Migración                                    | Fase | Contenido                                                                                                                                                                                                                                                                                                                                 |
| -------------------------------------------- | ---- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `0010_inventory_core.sql`                    | A    | `establishment_features.enabled_at`, función `inventory_control` desactivada, ámbito `inventory`, configuración, unidades (seis sembradas), categorías, insumos, presentaciones, almacenes («Almacén general»), asignación estación → almacén, existencias, valuaciones, libro de movimientos, disparador de inmutabilidad y 10 permisos. |
| `0011_inventory_recipes_and_consumption.sql` | B    | Porciones habituales, recetas, versiones y renglones, solicitudes de consumo, consumo por unidad, clasificaciones de cancelaciones y elaboraciones; 4 permisos; sin siembras; 84 restricciones con nombre.                                                                                                                                |
| `0012_quick_note_effects.sql`                | B    | Efecto (`none`, `omit`, `normal`, `extra`) e ingrediente en `catalog_category_quick_notes`; las indicaciones existentes quedan sin efecto.                                                                                                                                                                                                |
| `0013_inventory_purchasing.sql`              | C    | Proveedores y sus insumos, órdenes de compra y renglones, recepciones y renglones, devoluciones a proveedor y renglones, vínculos con gastos de caja; 6 permisos; sin siembras; 138 restricciones con nombre.                                                                                                                             |
| `0014_inventory_control.sql`                 | D    | Ajustes y mermas con renglones, transferencias con renglones, conteos físicos con renglones y niveles por almacén; 4 permisos; sin siembras; 100 restricciones con nombre.                                                                                                                                                                |
| `0016_inventory_suppliers_counter.sql` (9A)  | 9A-A | Segundo teléfono, celular y página web del proveedor; modalidad `kind` (`formal` / `counter`) con índice único parcial (un solo `counter`) y CHECK sin RFC ni razón social; `inventory_receipts.counter_store`; siembra del único proveedor «Compras de mostrador» con el primer nombre libre; 6 restricciones con nombre y 1 índice.     |

Todas son transaccionales, no modifican 0001–0009 ni filas existentes, nombran cada restricción y sus CHECK con columnas
opcionales exigen `IS NOT NULL` en la rama que valida. Solo siembran datos de referencia: unidades del sistema, «Almacén
general», la función desactivada, la fila de configuración y los permisos. Los 24 permisos se conceden solo al rol
`administrator`; el rol «Inventarios», que ya existía, sigue sin permisos (no se le asignan por inferencia). Los folios
de los documentos usan `GENERATED ALWAYS AS IDENTITY` y pueden tener huecos si una transacción se revierte.

## Función «Control de inventario»

- Función activable `inventory_control` («Control de inventario») del patrón de la etapa 6.1, **desactivada por omisión**.
  Se cambia en Configuración → Funciones del establecimiento (`PUT settings/features`, `settings.features.manage`); el
  Resumen de Inventarios muestra su estado y desde cuándo está activa, sin un segundo interruptor.
- `enabled_at` guarda la fecha y hora en que pasó de desactivada a activada; se renueva al reactivarla y se conserva al
  desactivarla.
- **Desactivada**: liberar a cocina no registra solicitudes ni movimientos; la venta es idéntica a la de la etapa 7.1.
- **Activada**: solo las liberaciones posteriores consumen. Un pedido confirmado antes de activar y liberado después (por
  ejemplo, uno programado) consume, porque la regla es la hora de la liberación. Una unidad liberada antes de activar nunca
  consume ni genera ajustes al modificarse o cancelarse.
- La administración (insumos, recetas, almacenes, existencias iniciales, compras) funciona con la función desactivada, para
  preparar todo antes de activar. Desactivarla deja de generar consumos y conserva todo el historial.

## Modelo de inventarios

### Unidades y conversiones

- `inventory_units`: abreviatura única, nombre, dimensión (`mass`, `volume`, `count`, `portion`) y equivalencia con la
  unidad base de su dimensión (g, ml, pza y porción = 1). Sembradas e inmutables: g, kg (1000), ml, l (1000), pza y porción.
- Unidades propias (`inventory.items.manage`), por ejemplo la onza (28.349523125 g): abreviatura y dimensión inmutables; la
  equivalencia solo cambia si la unidad no es la base de ningún insumo; no se desactiva la unidad base de un insumo activo.
- Las conversiones se redondean a 6 decimales (mitad alejándose de cero); una conversión que se redondea a cero y una entre
  dimensiones distintas se rechazan. Límites: cantidades hasta 1,000,000,000 en la unidad base y costos capturados hasta
  1,000,000.
- **Paquete y caja no son unidades**: son presentaciones de un insumo.

### Insumos

- `inventory_items`: código y nombre únicos sin distinguir mayúsculas, código de barras único (también frente a las
  presentaciones), categoría opcional, unidad base, tipo `purchased` («Comprado») o `prepared` («Preparación
  almacenable»), costo de referencia por unidad base (mayor que cero o vacío), existencia mínima y punto de
  reabastecimiento (en la unidad base, comparados con la existencia total), activo y revisión.
- El costo de referencia solo se envía y se ve con permisos de costos (`inventory.costs.manage` para fijarlo,
  `inventory.costs.read` para verlo).
- La unidad base no cambia si el insumo tiene movimientos o presentaciones (`BASE_UNIT_LOCKED`,
  `BASE_UNIT_HAS_PRESENTATIONS`) ni si se usa en recetas o porciones (`BASE_UNIT_IN_RECIPES`); con costo de referencia, el
  cambio exige recapturarlo (`REFERENCE_COST_REQUIRED`). Un insumo que produce una preparación almacenable no deja de ser
  `prepared` (`ITEM_PRODUCED_BY_RECIPE`).
- **Código interno sugerido (9A)**: al dar de alta un insumo, la pantalla forma el código a partir del nombre con la regla
  única de la etapa 6.1 (`services/productCode.ts`: minúsculas, sin acentos, guiones, hasta 40 caracteres para insumos) y
  pregunta a Go si está libre (`GET item-codes?code=…`, porque el listado es paginado). Si ya existe, Go propone el primero
  libre con `-2`, `-3`… (la misma regla, en `inventory/item_codes.go`) y la pantalla lo usa mientras el usuario no haya
  escrito el código a mano. Un código escrito por el usuario nunca se sobrescribe: si está tomado, la pista dice qué insumo
  lo usa y cuál está libre. Al editar un insumo el código no se sugiere ni cambia solo (se comprueba excluyendo el propio
  insumo). Go sigue respondiendo `DUPLICATE_CODE` al guardar; ningún código histórico ni UUID cambia.

### Presentaciones

- `inventory_presentations`: nombre único por insumo, contenido en la unidad base (capturado en cualquier unidad de la
  dimensión y convertido por Go: «Costal 25 kg» de un insumo en gramos = 25,000 g), código de barras y una sola
  presentación activa predeterminada para compra (marcar otra quita la marca a la anterior).
- La conversión caja → piezas vive en la presentación, nunca es universal: «Caja con 24», «Paquete de 12».

### Categorías de insumos

Nombre único sin distinguir mayúsculas y orden visual por posiciones (`internal/ordering`), administradas con
`inventory.items.manage`. Desde la 9A se administran en **Configuración → Categorías** (antes era un acordeón de
Insumos): búsqueda por nombre sin distinguir mayúsculas ni acentos (en pantalla: la lista es completa), número de insumos,
estado, «Ver insumos» (abre Insumos filtrado por la categoría), «Activar»/«Desactivar» con confirmación y «Editar» (nombre,
posición y estado). Desactivar solo deja de ofrecerla: los insumos la conservan. En el editor del insumo, el selector de
categoría ofrece **«+ Nueva categoría»** (con `inventory.items.manage`): abre un diálogo propio, fuera del formulario del
insumo, que crea la categoría, la agrega a la lista, la selecciona y conserva todo lo capturado sin crear el insumo; si el
nombre ya está en la lista lo avisa antes de enviar (Go responde `DUPLICATE_NAME` de todos modos).

### Almacenes y almacén por estación

- `inventory_warehouses`: nombre único, descripción y activo. La 0010 siembra **«Almacén general»** como predeterminado: es
  el destino por omisión, siempre activo, y no se reasigna por API. No se desactiva un almacén con existencias distintas de
  cero ni uno asignado a una estación (activa o inactiva: al reactivarse volvería a consumir de él).
- `inventory_station_warehouses`: historial de **solo inserción** estación de cocina → almacén, con `effective_from`, actor y
  `operation_id`. `null` vuelve al predeterminado; repetir la asignación vigente no agrega historial. El trabajador resuelve
  el almacén vigente **a la fecha de la liberación**.

### Existencias y valuaciones

- `inventory_stock`: existencia por (insumo, almacén), que puede ser negativa.
- `inventory_valuations`: por insumo, cantidad total Q (Σ de sus existencias), valor total V, último costo válido L y
  cantidad pendiente de costo. CHECK `inventory_valuations_zero_check`: cantidad 0 ⇒ valor 0.
- **El costo es uno solo por insumo**, no por almacén (decisión del propietario): las transferencias no lo cambian y no hay
  valuación por almacén. Existencias y valuaciones solo las actualiza el libro.

### Libro de movimientos

`inventory_movements` es de **solo inserción**: el disparador `app.inventory_reject_mutation()` rechaza `UPDATE`,
`DELETE` y `TRUNCATE` (también en el historial estación → almacén, las porciones habituales, las versiones y renglones de
receta y las elaboraciones). Cada movimiento guarda tipo, cantidad y valor con signo, costo unitario, base de costo, marca
de costo pendiente, hora de negocio (`occurred_at`) y de registro (`recorded_at`), origen (`source_kind`, `source_id`,
`source_line`), unidad comercial, movimiento relacionado, motivo, usuario **o** proceso responsable, `operation_id`, una
clave de idempotencia única (`entry_key`) y el estado posterior (existencia del almacén y valuación del insumo).

`ledger.go` (`Apply`) es la **única vía** para cambiar existencias y valuaciones: comprueba la `entry_key` (si existe,
devuelve lo registrado sin repetir efectos), bloquea con `FOR UPDATE` la valuación y la existencia, aplica el motor de
costos, inserta el movimiento principal y, si corresponden, el ajuste de costo (`<clave>:cost`) y el residuo de valuación
(`<clave>:residual`) ligados por `related_movement_id`, actualiza existencia y valuación y verifica que la suma de valores
coincida con el motor. `ApplyTransfer` registra la pareja de salida y entrada con el mismo valor.

| Tipo                   | Etiqueta                | Origen (`source_kind`)                                                 |
| ---------------------- | ----------------------- | ---------------------------------------------------------------------- |
| `initial`              | Existencia inicial      | `initial`                                                              |
| `receipt`              | Entrada por compra      | `receipt`                                                              |
| `consumption`          | Consumo por preparación | `consumption`, `amendment`, `cancellation`                             |
| `consumption_reversal` | Reversión de consumo    | `amendment`, `cancellation`                                            |
| `waste`                | Merma                   | `waste`                                                                |
| `adjustment_in`        | Ajuste positivo         | `adjustment`                                                           |
| `adjustment_out`       | Ajuste negativo         | `adjustment`                                                           |
| `transfer_out`         | Transferencia (salida)  | `transfer`                                                             |
| `transfer_in`          | Transferencia (entrada) | `transfer`                                                             |
| `supplier_return`      | Devolución a proveedor  | `supplier_return`                                                      |
| `production_out`       | Elaboración (consumo)   | `production`                                                           |
| `production_in`        | Elaboración (entrada)   | `production`                                                           |
| `count_in`             | Sobrante de conteo      | `count`                                                                |
| `count_out`            | Faltante de conteo      | `count`                                                                |
| `cost_adjustment`      | Ajuste de costo         | El del movimiento que lo origina (solo valor, sin almacén ni cantidad) |

Bases de costo: `average` («Costo promedio»), `reference` («Último costo válido»), `configured` («Costo de referencia del
insumo»), `pending` («Costo pendiente»), `document` («Costo del documento»), `original` («Valor original»), `provisional`
(«Valor provisional del faltante») y `difference` («Diferencia de costo»). Los procesos de Go se nombran «Proceso de
consumo de inventario» (`inventory_worker`).

### Existencia inicial

`POST inventory/initial-balances` (`inventory.adjustments.authorize` + `inventory.costs.manage`): de 1 a 500 renglones sin
insumos repetidos, solo si el insumo no tiene movimientos en ese almacén (después se usa una recepción o un ajuste). Cada
renglón lleva el costo por unidad capturada (por caja si se captura en cajas), con valor exacto cantidad × costo, o «Usar
el costo de referencia» (`use_reference_cost`, excluyente con el costo; sin costo de referencia, `REFERENCE_COST_MISSING`);
un costo 0 se admite como costo explícito. La fecha no puede ser futura (por omisión, la hora del servidor; la interfaz no
la captura). Registra un movimiento `initial` como entrada con documento (reglas 2 y 4 del motor).

## Costo promedio

El motor (`inventory/costing.go`) es una función pura sobre `shopspring/decimal`. Para que la política con existencias
negativas sea exacta, mantiene por insumo la **cantidad total Q y el valor total V** (no solo un costo unitario), además del
último costo válido L y del costo de referencia configurado R. El **costo de referencia vigente** es L o, si nunca hubo,
R. Operaciones: `Issue` (salidas), `Receive` (entrada con documento: recepción, existencia inicial y entrada de una
elaboración), `Admit` (ajuste positivo y sobrante de conteo), `Restore` (reversión de consumo, aplicada como entrada con
documento), `Transfer` y `ReturnToSupplier`. Las reglas son las de la fase A con las correcciones de la revisión
independiente (fase F del avance), que se aplican a los movimientos nuevos: ningún movimiento histórico se reescribe.

### Reglas

La numeración es la del diseño, que usan también el código (`costing.go`), el avance y `API_CONTRACTS.md`:

1. **Salida** de q unidades:
   - con existencia positiva (`Q ≥ q`): valor = q × V ÷ Q; si sale todo (q = Q), exactamente V. Base `average`;
   - que cruza a negativo (`0 < Q < q`): la parte Q al valor V exacto y el resto a la referencia vigente, en un solo
     movimiento (base `reference` cuando la referencia interviene);
   - con existencia en cero o negativa: q × referencia vigente (base `reference` con L, `configured` con solo R). El valor
     total puede quedar negativo junto con la cantidad;
   - **sin ningún costo disponible**: valor cero con marca de **costo pendiente** (`pending_cost_quantity` aumenta). Esos
     movimientos aparecen en la alerta `cost_pending` y se valoran por diferencia con la entrada siguiente, sin
     reescribirlos.
2. **Entrada con documento** de r unidades: recepción, existencia inicial (regla 4) y entrada de una elaboración
   (`production_in`, base `document`, cuyo valor es el de lo consumido):
   - con Q ≥ 0: promedio ponderado (V += valor del documento, Q += r);
   - con existencia negativa: las unidades que cubren el déficit (d = mínimo entre r y el déficit) tenían un valor
     provisional d × V ÷ Q (exactamente −V si cubre todo); su valor real es valor del documento × d ÷ r. El movimiento
     de entrada lleva el valor del documento y la diferencia provisional − real se registra como `cost_adjustment`
     separado (cantidad cero), ligado a la entrada y al insumo. La cantidad pendiente de costo baja en mínimo(pendiente, d).
     Así, un insumo preparado vendido antes de elaborarlo queda valorado al costo real de la elaboración.
3. **Entradas sin costo propio**:
   - reversión de consumo (`consumption_reversal`, base `original`): el movimiento conserva **exactamente** el valor
     original (proporcional) del consumo que compensa, de modo que el consumo neto de la partida sigue siendo exacto; se
     aplica como una entrada con documento: con existencia negativa, el déficit cubierto d = mínimo(q, −Q) se realinea con
     un `cost_adjustment` separado ligado a la reversión (provisional de d − valor original × d ÷ q) y la cantidad pendiente
     baja como en una recepción;
   - transferencia: salida y entrada con el mismo valor; Q, V y L del insumo no cambian;
   - ajuste positivo y sobrante de conteo (`Admit`): con Q > 0, al promedio vigente; con existencia negativa, el déficit
     cubierto a su provisional exacto y el resto a la referencia vigente (L o R), sin ajuste de costo separado. **Si la parte
     que quedaría en existencia positiva no tiene ningún costo** (ni L ni R), la entrada se rechaza con 409
     `COST_REQUIRED` («nunca a un costo inventado», §0-B.6): se configura el costo de referencia del insumo y se vuelve a
     intentar. La parte que solo cubre un déficit sigue permitida a su provisional. Estas entradas ya no producen «costo
     pendiente».
4. **Existencia inicial**: exige costo unitario o el costo de referencia confirmado explícitamente
   (`use_reference_cost`); se aplica como entrada con documento.
5. **Devolución a proveedor**: sale por la regla 1 y el documento se valora al costo de la recepción original. La diferencia
   entre ambos se absorbe como `cost_adjustment` ligado a la devolución solo si queda existencia positiva y sin dejar V
   negativo; lo que no se absorbe se guarda en la devolución como «diferencia no absorbida», sin movimiento.
6. **Invariante Q = 0 ⇒ V = 0**: si una operación deja la cantidad en cero con valor distinto de cero, se registra un
   residuo `cost_adjustment` de −V; además lo exige el CHECK de la base. Como las entradas que cubren todo el déficit ya
   dejan el valor exactamente en cero, el residuo es una red de seguridad.
7. **Último costo válido**: L solo se actualiza con Q > 0, V > 0 **y** un promedio redondeado a 10 decimales mayor que cero
   (un valor diminuto sobre una cantidad grande redondearía a cero, que no es un costo válido); si no, se conserva el
   anterior.
8. **Redondeo**: ver «Precisión».

Invariantes que mantiene toda operación (comprobados en cada estado de las pruebas): Q = 0 ⇒ V = 0; Q > 0 ⇒ V ≥ 0; L nulo
o mayor que cero; 0 ≤ cantidad pendiente ≤ déficit. **Ningún movimiento histórico se reescribe**: toda diferencia es un
movimiento nuevo. Límite conocido: si un déficit pendiente de costo solo se cubre en parte con recepciones, L sigue sin
valor (solo se fija con existencia positiva) y un sobrante que cruce a positivo pide configurar el costo de referencia.

### Precisión

Cantidades con 6 decimales en la unidad base (recetas de gramos o mililitros fraccionarios), valores con 6 decimales y
costos unitarios con 10, con redondeo mitad alejándose de cero y sin redondeo doble. La resolución de recetas usa factores
intermedios de 20 decimales y redondea el resultado a 6 por (insumo, almacén). Una devolución usa un costo de 20 decimales
para que devolver todo lo que quedaba dé exactamente el valor restante (el documento guarda el costo a 10). Los importes se
redondean a centavos solo para mostrar e imprimir.

### Casos verificados

Pruebas unitarias de `inventory/costing_test.go`, cada una con el cálculo a mano anotado:

| Caso                                      | Datos                                                                                                                                                                                                                                               | Resultado                                                                                                                                                        |
| ----------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Promedio con existencia positiva          | 10 u a $20 + 10 u a $30                                                                                                                                                                                                                             | 20 u, $500, promedio $25                                                                                                                                         |
| Salida a cero exacto                      | 3 u por $10: sale 1 u y luego 2 u                                                                                                                                                                                                                   | −3.333333 y −6.666667 (exacto V); 0 u, $0, sin residuo                                                                                                           |
| Salida que cruza a negativo               | 2 u por $50 (L = 25), sale 5 u                                                                                                                                                                                                                      | −$125 (50 + 3 × 25), base `reference`; −3 u, −$75                                                                                                                |
| Salida con existencia negativa a L        | −3 u por −$75 (L = 25, R = 40), sale 2 u                                                                                                                                                                                                            | −$50 a L; −5 u, −$125                                                                                                                                            |
| Salida a R sin L                          | 0 u, R = 18, sale 4 u                                                                                                                                                                                                                               | −$72, base `configured`; −4 u, −$72, L sin valor                                                                                                                 |
| Varias salidas negativas con R distinto   | R = 18 sale 4 u; R = 20 sale 2 u; recepción 10 u por $250                                                                                                                                                                                           | −6 u, −$112; ajuste 112 − 150 = −$38; 4 u, $100 (L = 25)                                                                                                         |
| Recepción que cubre parte del déficit     | −6 u, −$112; recepción 4 u por $100; después 2 u por $50                                                                                                                                                                                            | ajuste −25.333333 (−2 u, −37.333333); ajuste −12.666667 (0 u, $0); total −$38                                                                                    |
| Recepción que cruza a positivo            | −5 u, −$100 (L = 20); recepción 8 u por $240                                                                                                                                                                                                        | ajuste −$50; 3 u, $90, promedio $30                                                                                                                              |
| Costo pendiente y su resolución           | sin L ni R sale 5 u; recepción 8 u por $240                                                                                                                                                                                                         | salida $0 pendiente (5 u); ajuste −$150; 3 u, $90, nada pendiente                                                                                                |
| Recepción parcial de un déficit pendiente | −5 u pendientes; recepción 3 u por $30; sale 1 u                                                                                                                                                                                                    | ajuste −$30; −2 u, $0, pendiente 2; luego pendiente 3                                                                                                            |
| Ajuste positivo                           | 20 u, $500 + 4 u; −5 u, −$100 (L = 20) + 8 u; 0 u, R = 12 + 2 u                                                                                                                                                                                     | +$100 (24 u, $600); +$160 base `provisional` (3 u, $60); +$24 base `configured` (2 u, $24, L 12)                                                                 |
| Entrada sin ningún costo                  | 0 u sin L ni R + 2 u; −3 u pendientes + 3 u, + 2 u y + 5 u; con L = 4, + 5 u                                                                                                                                                                        | rechazo (`COST_REQUIRED`); $0 (0 u); $0 (−1 u, 1 pendiente); rechazo sin cambiar el estado; $8 (2 u, $8)                                                         |
| Transferencia                             | 20 u, $500 traslada 5 u; −2 u, −$50 (L = 25) traslada 1 u                                                                                                                                                                                           | $125 y $25; Q, V y L sin cambio                                                                                                                                  |
| Devolución con diferencia absorbida       | 20 u, $500; devuelve 4 u a $30 (y a $20)                                                                                                                                                                                                            | salida −$100, documento $120, ajuste −$20 (16 u, $380, L 23.75); a $20: ajuste +$20 ($420)                                                                       |
| Devolución con diferencia no absorbida    | 4 u, $100 devuelve 4 a $30; 5 u, $10 devuelve 4 a $30                                                                                                                                                                                               | sin ajuste, $20 sin absorber; ajuste limitado −$2 y $110 sin absorber (1 u, $0, L = 2)                                                                           |
| Sobrante y faltante de conteo             | 10 u, $250: faltante 2 u; sobrante 3 u                                                                                                                                                                                                              | −$50 (8 u, $200); +$75 (11 u, $275)                                                                                                                              |
| Invariante Q = 0 ⇒ V = 0                  | −5 u, −$100 (L = 20); reversión de 5 u registradas en $150 y en $100                                                                                                                                                                                | ajuste de costo −$50 y ningún residuo (0 u, $0); la exacta, sin ajuste                                                                                           |
| Red de seguridad del residuo              | estado de 0 u con $0.000004                                                                                                                                                                                                                         | residuo −0.000004                                                                                                                                                |
| Elaboración con déficit                   | R = 0.1/g; salen 500 g; entran 1000 g por $300                                                                                                                                                                                                      | −$50 (−500 g); ajuste 50 − 150 = −$100; 500 g, $150, L 0.3                                                                                                       |
| Reversión con existencia negativa         | (a) 10 u a $20, sale 10, 10 u a $30, sale 15, se revierten 10 u de $200; (b) −5 u, −$500, se revierten 6 u de $6; (c) −10 u, −$200, se revierten 4 u de $120; (d) 10 u, $300, se revierten 5 u de $100; (e) −5 u pendientes, se revierten 2 u de $0 | (a) −5 u, −$150 → ajuste +$50, 5 u, $100, L 20; (b) ajuste +$495, 1 u, $1; (c) ajuste −$40, −6 u, −$120; (d) sin ajuste, 15 u, $400; (e) sin ajuste, pendiente 3 |
| Último costo válido                       | 10⁹ u por $0.000001 (sin L y con L = 5); 1 u por $0.000001                                                                                                                                                                                          | L sin valor; L = 5 conservado; L = 0.000001                                                                                                                      |
| Secuencias pseudoaleatorias               | 200 secuencias de 60 operaciones de todos los tipos (semilla fija, con y sin R)                                                                                                                                                                     | invariantes y V después = V antes + Σ movimientos en cada paso; las transferencias no cambian el estado                                                          |
| Redondeo fraccionario                     | 1250.5 g por $87.53; salidas de 12.3456 g, 0.000125 g y el resto                                                                                                                                                                                    | costo 0.0699960016; −0.864143; −0.000009; −86.665848 exacto; 0 g, $0                                                                                             |

Las pruebas de integración repiten estas reglas con el libro real: una recepción de 8 piezas a $30 sobre −5 piezas y
−$100 registra el `receipt` de $240 y un `cost_adjustment` de −$50 (provisional $100 − real $150); un queso con existencia
1, 0 y −1 se valora al promedio, exacto y a la referencia; una salsa verde (referencia 0.1/g) vendida antes de elaborarla
(−500 g, −$50) y elaborada con chile de $300 registra `production_in` de $300 base `document` y un `cost_adjustment` de
−$100 (500 g, $150, L 0.3); el «No preparado» de un postre con la crema en −5 u y −$150 registra la reversión de $200 base
`original` y un ajuste de +$50 (5 u, $100, consumo neto de la unidad en cero); y el hielo sin costo (−2 u pendientes)
rechaza con `COST_REQUIRED` un conteo con sobrante de 6, un ajuste pendiente al autorizarlo y un ajuste directo de +3, sin
registrar nada, mientras acepta +2 (solo el déficit, a $0); con costo de referencia de $1.50 se autoriza el ajuste ($7.50)
y se aplica el conteo (11 u, $16.50).

### Costo teórico de las recetas

Solo con `inventory.costs.read`. El costo vigente de cada insumo es V ÷ Q con existencia y valor positivos; si no, el último
costo válido; si no, el costo de referencia; si no, «pendiente» (no suma y se señala). Las preparaciones inmediatas se
costean de forma recursiva por su rendimiento efectivo y las almacenables al costo vigente de su insumo. En productos:
renglones fijos ÷ rendimiento + ingredientes incluidos del catálogo × su porción; los renglones «solo para llevar» se
informan aparte. Las opciones de grupo y los componentes de combo tienen sus propias recetas y no se suman.

## Existencias negativas

- No bloquean ventas, ajustes negativos, mermas, transferencias, elaboraciones ni devoluciones a proveedor. Las respuestas
  de esas operaciones las informan en `warnings` (`NEGATIVE_STOCK`); el consumo de ventas las registra sin avisar a la
  captura.
- Las salidas se valoran con la referencia vigente o, sin ningún costo, con costo pendiente; la entrada siguiente
  (recepción, existencia inicial, elaboración o reversión de consumo) corrige el valor con un ajuste de costo separado y un
  ajuste positivo o sobrante cubre el déficit a su provisional (reglas 1 a 3 del motor). Una entrada sin documento que
  dejaría existencia positiva sin ningún costo se rechaza con `COST_REQUIRED`.
- La existencia inicial no puede producirlas.
- Se ven en Existencias (fila resaltada, etiqueta y aviso de que no bloquean), en el Resumen y en la alerta crítica
  `negative_stock`.

## Recetas y versiones

- **Destinos** (`inventory_recipes.owner_kind`): `product` (con variante opcional: la receta de la variante prevalece y, si
  falta o está inactiva, se usa la del producto), `complement` y `preparation`. El destino, la modalidad de una preparación
  y su insumo producido se fijan al crear (`RECIPE_TARGET_IMMUTABLE`); producto, variante y complemento deben existir en el
  catálogo. Recetas de producto y complemento con `inventory.recipes.manage`; preparaciones con
  `inventory.subrecipes.manage`.
- **Versionado obligatorio**: cada guardado, también desactivar, crea una versión **inmutable** con `effective_from` (la
  hora del guardado); el `PUT` exige la `revision` igual a la versión vigente. El trabajador usa la versión vigente **a la
  hora de la liberación**: una receta modificada entre la liberación y el procesamiento no altera ese consumo (la siguiente
  liberación usa la nueva) y ningún consumo registrado cambia.
- **Renglones** (hasta 200; una receta activa necesita al menos uno): `item` (insumo), `preparation` (subreceta) e
  `ingredient` (solo en recetas de producto: un ingrediente del catálogo con un insumo **o** una preparación que consume cada
  porción; sobrescribe la porción habitual para ese producto y aparece una sola vez por versión). Cada renglón se captura en
  cualquier unidad de la dimensión de su destino y se guarda también en la unidad base; cambiar después la equivalencia de
  una unidad propia no altera recetas ni consumos.
- **Rendimiento**: en productos y complementos se expresa en porciones (1 por omisión) y cada unidad consume renglones ÷
  rendimiento.
- **Ciclos** rechazados (`RECIPE_CYCLE`) con un recorrido sobre las versiones vigentes, incluidos los que pasan por el insumo
  que produce una almacenable, una preparación que se usa a sí misma y una almacenable que consume su propio insumo. La
  resolución corta además en 8 niveles.
- **Destinos sin receta** (`GET recipe-targets`): productos, variantes y complementos con estado `with_recipe`,
  `without_recipe` o `no_preparation` (sin receta y ruta «Sin preparación») y si consumirían algo por porciones o vínculos
  (`configured`).

## Subrecetas y elaboraciones

- **Elaboración inmediata**: al consumirse se descompone en sus insumos por su rendimiento efectivo
  (`rendimiento × (1 − merma/100)`). Su unidad base es la de equivalencia 1 de la dimensión del rendimiento (g, ml, pza o
  porción).
- **Preparada previamente (almacenable)**: produce un insumo de tipo `prepared` con existencias propias; al venderse se
  consume ese insumo y **nunca** sus ingredientes, de modo que no hay doble descuento (probado: enchiladas con 200 g de arroz
  rojo a 200 × $56.80 ÷ 4750 = $2.391579).
- Merma de elaboración de 0 a menos de 100 % (solo en preparaciones); la dimensión del rendimiento no cambia entre versiones
  (`YIELD_DIMENSION_LOCKED`); una preparación no se desactiva si la usa la versión vigente de una receta activa
  (`PREPARATION_IN_USE`).
- **Elaboraciones** (`POST productions`, `inventory.production.record`): preparación almacenable activa, almacén (por
  omisión el predeterminado), lotes, cantidad producida real (por omisión la esperada) y nota. Consume la versión vigente ×
  lotes (`production_out`) y produce el insumo preparado con todo el valor consumido (`production_in`, entrada con documento
  de base `document`, así que su costo incluye la merma); si el insumo preparado tenía existencia negativa (se vendió antes
  de elaborarlo), el déficit cubierto se valora al costo real de la elaboración y la diferencia contra su valor provisional
  es un `cost_adjustment` ligado (`production:<elaboración>:in:cost`). Guarda esperado, producido y merma (lotes ×
  rendimiento − producido, mínimo 0). Idempotente; las existencias negativas se avisan. Ejemplo probado: 2 lotes consumen 2000 g de arroz, 160 g de tomate y
  20 g de chile por $56.80; esperado 4800 g, producido 4750 g, merma 250 g. Una elaboración no se anula ni se edita.

## Porciones habituales

- La **porción habitual** es la cantidad de un insumo por porción de un ingrediente del catálogo («20 g de Cebolla
  blanca»), en `inventory_ingredient_portions`: historial de solo inserción, la vigente es la última, una fila sin insumo la
  retira y repetir la vigente no agrega historial. Se administra con `inventory.recipes.manage` y solo apunta a insumos.
- **Consumo = porciones solicitadas × porción habitual.** El catálogo guarda en la partida un número decimal de porciones por
  ingrediente (`quote_line.ingredients[].requested`), así que «Sin cebolla», «Con cebolla» y «Cebolla extra» consumen 0, 1
  y 2 porciones cuando lo incluido es 1; «3 porciones» consume 3 y media porción, 0.5. No existe un segundo modelo «sin /
  normal / extra».
- Un renglón de ingrediente en la receta del producto sobrescribe la porción para ese producto.
- **Sin porción configurada no se inventa consumo**: queda el diagnóstico `ingredient_without_portion`.
- La columna del catálogo `catalog_product_ingredients.recipe_quantity` («Cantidad para receta futura») no se usa.

## Indicaciones rápidas con efecto

Detalle en [CATALOG.md](CATALOG.md) («Indicaciones con efecto · etapa 8») y [KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md).

- Una indicación por categoría puede tener efecto `omit`, `normal` o `extra` sobre un ingrediente del catálogo (`none` es
  solo instrucción de cocina, el valor de todas las anteriores). Se elige con botones y un selector de ingrediente, sin
  gramos (`catalog.categories.manage`).
- **No es un segundo mecanismo**: al tocarla, la captura aplica la misma selección estructurada que el personalizador
  (`customization.ingredients`) y guarda su identificador en `customization.quick_notes`; Go valida que la cantidad del
  ingrediente coincida con el efecto. Por eso es imposible acumular dos efectos sobre el mismo ingrediente
  (`QUICK_NOTE_CONFLICT`) y **precio e inventario salen del mismo dato**: «Queso extra» cobra y consume lo mismo que el botón
  «Extra queso».
- Solo se muestran las indicaciones con efecto que aplican a los ingredientes del producto; «Con cebolla» en un producto que
  ya la incluye no cambia nada. La comanda imprime la indicación una sola vez.
- Las partidas anteriores (solo texto) nunca tienen efecto y editar una indicación no cambia partidas ni consumos ya
  registrados (el desglose congela la resolución). Las observaciones libres nunca afectan el inventario.

## Complementos, combos y empaques para llevar

- **Opciones de grupo** (complementos): porciones elegidas × factor de la porción del catálogo (del `pricing_snapshot` del
  pedido; 1 si no tiene) × la primera configuración que exista: receta del complemento; si no, la configuración estándar del
  producto vinculado (renglones de su receta e ingredientes incluidos × porción, sin recorrer sus grupos ni componentes); si
  no, la porción habitual del ingrediente vinculado; si no, nada (diagnóstico `complement_without_configuration`).
- **El precio no interviene**: con el filete de $140 que incluye arroz y ensalada, quitar arroz y poner pasta sigue en $140
  (equivalencia por valor con piso) y consume pasta y ensalada, no arroz; «2 porciones de papas» consume 2 × su porción.
- **Combos**: su propia receta, si la tiene (por ejemplo, un empaque), más cada componente elegido × su cantidad, con su
  propia personalización y de forma recursiva.
- **Empaques «solo para llevar»**: un renglón `takeaway_only` (insumos y preparaciones de recetas de producto y complemento)
  se consume solo si la cuenta es para llevar (`takeaway`, que incluye ventanilla); la modalidad se congela al liberar. En
  salón se ignora.
- **Captura de complementos** (ajuste de la fase B, sin rehacer el panel de la 6.1): en grupos de varias porciones, un toque
  en la tarjeta suma una porción y el botón − de 44 px de la propia tarjeta la resta; cantidad grande; precio en la tarjeta;
  encabezado «2 de 2»; cambio de precio del grupo en vivo calculado por Go («+$10.00», «Sin cambio»); aviso al intentar pasar
  del máximo. Sustituir es de dos toques.

## Consumo por liberación a cocina

### Solicitudes

- **Al liberar**: `kitchen/releases.go` (`prepareRelease`), justo después de insertar las filas de `kitchen_unit_releases`
  y antes de los documentos, llama a `inventory.RecordReleases`. Con la función activada ejecuta **un solo**
  `INSERT … SELECT … ON CONFLICT DO NOTHING` de solicitudes `release` con la versión y `released_at` de la fila canónica y
  la modalidad congelada de la liberación; sin lock 7820148308 ni lectura de recetas. Cubre los cuatro caminos que liberan
  (confirmación inmediata, planificador de programados, liberación manual y reintento de un cambio pendiente). Una
  liberación bloqueada por advertencias no crea filas canónicas ni solicitudes hasta resolverse.
- **Al cambiar**: `kitchen.Integrate` llama a `inventory.RecordChange` para `item.modified` e `item.cancelled` **antes** de
  procesar cocina (también cuando el cambio de cocina queda pendiente): `amend` para las unidades modificadas y `cancel` para
  las canceladas o sobrantes de una reducción, solo si la unidad tiene fila canónica **y** solicitud de liberación.
- `inventory_consumption_requests` guarda tipo, unidad, versión, `released_at`, `occurred_at`, modalidad, estado
  (`pending`, `processed`, `failed`), intentos, último error genérico, próximo intento y diagnósticos, con
  `UNIQUE(kind, restaurant_unit_id, source_id)`: una por liberación.
- **No consumen**: guardar o previsualizar un borrador, crear una cuenta, dividir, transferir, combinar, cambiar de mesa,
  reimprimir, cambiar de impresora ni cobrar (probado).

### Trabajador

- `inventory.Worker`: sondeo de 1 s (sin LISTEN/NOTIFY); cada vuelta procesa lo pendiente en lotes de 50 hasta vaciarlo y
  después reconcilia las cancelaciones vencidas. Al iniciar Go procesa todo lo pendiente.
- Cada solicitud va en su propia transacción con 7820148308 exclusivo, tomada con `FOR UPDATE SKIP LOCKED` en orden de
  `seq` y solo si su unidad no tiene otra anterior sin procesar.
- Un fallo revierte esa solicitud y, en otra transacción, la marca `failed` con el mensaje genérico «No se pudo calcular el
  consumo; se reintentará automáticamente.», suma un intento y la reprograma con espera creciente (2, 4, 8 … 256 s, tope
  300 s). Se reintenta indefinidamente, bloquea solo las solicitudes posteriores de su unidad, no detiene las demás y aparece
  en la alerta crítica `consumption_failed` y en Movimientos → Solicitudes de consumo.
- Publica `inventory.stock.changed`, `inventory.movements.recorded` e `inventory.alerts.changed` en su transacción. No audita
  cada consumo en `audit_events`: los movimientos registran el proceso `inventory_worker`.

### Resolución

`inventory/resolution.go` es una función pura (no lee la base) que resuelve la unidad **a `released_at`** con el desglose
canónico de la versión (`quote_line`), el catálogo congelado del pedido (`pricing_snapshot`), la modalidad congelada, las
recetas y porciones vigentes a esa fecha y el enrutamiento de cocina de la unidad:

1. producto o variante: renglones de insumo × 1/rendimiento; preparaciones inmediatas descompuestas; almacenables (o
   insumos preparados) consumidas como insumo;
2. ingredientes del catálogo: porciones pedidas × porción (renglón de ingrediente de la receta o porción habitual);
3. opciones de grupo: ver [Complementos](#complementos-combos-y-empaques-para-llevar);
4. componentes de combo × su cantidad, recursivos, con su propia personalización (partes `root/0`, `root/1`…);
5. renglones «solo para llevar» solo con la modalidad `takeaway`;
6. almacén por parte (ver abajo);
7. resultado redondeado a 6 decimales por (insumo, almacén): un movimiento `consumption` por par con
   `occurred_at = released_at` (y `recorded_at`, la hora de registro).

Sin configuración no se inventa nada: queda el diagnóstico `no_configuration` y la unidad no consume.

### Almacén por estación

Las estaciones salen de `kitchen_tasks.instructions.parts[].path` de la unidad (también de tareas canceladas). Con varias
estaciones, la primera por nombre que tenga almacén asignado a esa fecha; sin asignación, el predeterminado; una parte sin
estación hereda la de su padre. Los productos «Sin preparación» no tienen tareas: consumen del almacén predeterminado (o del
de la estación del combo, si son componente). El motivo del movimiento dice «Liberación a cocina · Estación: …».

### Modificaciones de partidas liberadas

El objetivo es el consumo de la versión nueva resuelto a la fecha de liberación de la unidad; se registra la diferencia con
el neto ya registrado por (insumo, almacén): `consumption` si falta o `consumption_reversal` al valor proporcional
registrado (exacto si revierte todo; con existencia negativa, con su ajuste de costo ligado, regla 3), con `occurred_at` =
hora del cambio. Una unidad sin consumo previo no tiene efecto
(probado: una modificación que solo agrega 20 g de cebolla y revierte exactamente la lechuga).

### Diagnóstico y traza

`GET consumption-requests` lista las solicitudes con su estado, intentos, próximo intento, mensaje genérico y diagnósticos
(`no_configuration`, `ingredient_without_portion`, `complement_without_configuration`, `preparation_unavailable`,
`depth_exceeded`, `preparation_cycle`, `unit_not_consumed`, `nothing_consumed`, `unit_reversed`,
`unit_already_registered`). `GET consumption/{unidad}` es la traza de una partida: estado, liberación, modalidad, consumo
por insumo y almacén, cancelación, movimientos y solicitudes (valores solo con costos).

## Idempotencia

- **HTTP**: toda escritura lleva `operation_id`; el mismo cuerpo devuelve la respuesta guardada sin repetir efectos y otro
  cuerpo responde 409 `IDEMPOTENCY_CONFLICT`. La interfaz reintenta un resultado incierto con el mismo identificador
  (`useInventoryMutation`, sobre `useRestaurantMutation`, clave `inventory` del almacén global de pendientes); los
  formularios de recepción, devolución y vínculo quedan bloqueados tras un resultado incierto.
- **Liberación**: una solicitud por (tipo, unidad, liberación o evento) con restricción única y `ON CONFLICT DO NOTHING`; un
  reintento HTTP de la confirmación devuelve la respuesta guardada; un evento reprocesado lo detiene `kitchen_event_receipts`.
- **Trabajador**: estado de la solicitud, estado por unidad (`inventory_unit_consumptions`) y `entry_key` por movimiento
  (`consumption:<solicitud>:<insumo>:<almacén>`, `amend:…`, `classification:<id>:<revisión>:…`). Reprocesar (una vuelta
  repetida o una solicitud devuelta a pendiente) no duplica movimientos (probado).
- **Libro**: `entry_key` única; `Apply` devuelve lo registrado (`receipt:<recepción>:<renglón>`,
  `supplier_return:<devolución>:<renglón>`, `adjustment:<ajuste>:<renglón>`, `transfer:<transferencia>:<renglón>:out|in`,
  `count:<conteo>:<insumo>`).
- **Documentos**: órdenes, recepciones, devoluciones, vínculos, elaboraciones, ajustes, transferencias y conteos tienen
  `operation_id` único; repetir la clasificación vigente de una cancelación no escribe; el reconciliador actualiza con
  `WHERE revision`; aplicar un conteo dos veces no genera otro ajuste (`COUNT_ALREADY_APPLIED`).

## Cancelaciones

- **Antes de liberar** no hay consumo ni clasificación.
- **Después de liberar**, una unidad cancelada con consumo crea una clasificación `pending` con la hora de la cancelación, su
  **día operativo** y su vencimiento (el corte del día siguiente), calculados al registrarla con la zona y la hora de corte
  vigentes (cambiarlas después no mueve los vencimientos registrados). Una reducción de cantidad cancela las unidades
  sobrantes. Si la unidad no consumió nada no se crea clasificación (`nothing_consumed`).
- **Clasificar** (`POST cancellations/{id}/classify`): «No preparado» revierte el neto exacto (cantidad y valor netos de la
  unidad en cero; con existencia negativa del insumo, la reversión lleva su ajuste de costo ligado, regla 3); «Preparado / merma» conserva el consumo y, si antes se había revertido, vuelve a consumir lo revertido a costo
  vigente. Hasta el corte, con `inventory.waste.record` (resolución `manual`, también para cambiarla); después del corte o de
  la resolución automática, solo con `inventory.adjustments.authorize` (resolución `correction`). Repetir el estado vigente
  no escribe nada. Los movimientos llevan la hora de la cancelación y el origen `cancellation`, así un conteo posterior los
  ve en su lugar.
- **Día operativo** (`operating_day.go`, que desde la 9A delega en `internal/businessday`): un instante pertenece al día
  cuya fecha local tiene la hora de corte en o antes de él, en la zona horaria del establecimiento; la hora de corte es
  configurable (05:00 por omisión; desde la 9A es la «Hora de inicio del día» de los reportes y se cambia solo en
  Configuración general) y no depende de los turnos de caja. Si la hora de corte no existe por un adelanto de horario, el corte es el primer instante tras el salto; si
  ocurre dos veces por un atraso, la primera (probado con Monterrey y Tijuana).
- **Reconciliador** (mismo trabajador): aplica «No preparado» automático a toda pendiente cuyo vencimiento ya pasó, una por
  transacción con el lock y `UPDATE … WHERE revision`, y lo audita como `inventory.cancellation_reconciled` con el proceso
  como responsable. Si Go estuvo apagado, al iniciar procesa los días pendientes. Es idempotente.
- Interfaz: Ajustes y mermas → «Cancelaciones por clasificar» (tarjeta por partida con cuenta legible, día operativo, límite,
  consumo por insumo y botones según `can_classify` de Go) e «Historial de clasificaciones» (incluidas las resueltas «Por
  cierre del día operativo»).

## Ajustes y mermas

- **Ajuste de discrepancia** (`POST adjustments`, `inventory.adjustments.record`): con `inventory.adjustments.authorize` se
  aplica al registrarlo; sin él queda `pending_authorization`, sin movimientos, hasta que **otra persona** con ese permiso lo
  autorice (los movimientos llevan la hora de la autorización) o lo rechace con motivo (5–500 caracteres). Quien lo registró
  nunca lo autoriza ni lo rechaza, aunque después reciba el permiso (403 `SELF_AUTHORIZATION_FORBIDDEN`; también un CHECK).
- **Merma operativa** (`inventory.waste.record`): se aplica directamente, solo salidas, movimiento `waste`. Un ajuste
  negativo de discrepancia es `adjustment_out` (nunca se convierte en merma).
- **Motivos** (lista cerrada con sentido obligatorio): rotura, derrame, caducidad, faltante físico y consumo extraordinario,
  solo salidas; sobrante, solo entradas; error de registro y otro, ambos. «Otro» exige texto (3–200). Una merma admite solo
  rotura, derrame, caducidad, consumo extraordinario u otro.
- Cantidades con signo capturadas en una presentación, una unidad de la dimensión o la unidad base; de 1 a 200 renglones con
  cada insumo una vez. Entradas con la regla 3 del motor y salidas con la regla 1. Pueden dejar existencia negativa
  (`NEGATIVE_STOCK`). Insumos y presentaciones desactivados se admiten; el almacén debe estar activo.
- **`COST_REQUIRED`** se evalúa al **aplicar** la entrada: un ajuste aplicado al registrarlo se rechaza sin guardar nada; uno
  pendiente se registra como siempre y el rechazo llega al autorizarlo (sigue pendiente y sin cambios). El mensaje nombra el
  insumo («El insumo «X» no tiene costo: configura su costo de referencia antes de registrar una entrada sin documento…»);
  con varios insumos sin costo, el primero.
- Un ajuste no se edita ni se borra: se autoriza, se rechaza o se registra otro. La hora de negocio es la de su aplicación.

## Transferencias

`POST transfers` (`inventory.transfers.record`): origen y destino activos y distintos, de 1 a 200 renglones; cada renglón
registra `transfer_out` y `transfer_in` con el mismo valor, de modo que el costo del insumo no cambia (probado: 20 de 50
botellas a $120 dejan la valuación en 50 y $6,000). No se bloquea por existencia negativa en el origen (aviso), no crea
compras ni ventas y el documento es de solo inserción.

## Conteos físicos

1. **Crear** (`inventory.counts.perform`) en un almacén activo con todos los insumos activos, los de una categoría o una
   lista (también desactivados), en orden de hoja de conteo (categoría y nombre). Un insumo no puede estar en dos conteos sin
   terminar del mismo almacén (`COUNT_ITEM_IN_PROGRESS`). Mientras está abierto se agregan o quitan insumos y se cambian las
   notas (con revisión).
2. **Capturar** (`PUT counts/{id}/lines`, de 1 a 200 por guardado, sin revisión para que varias terminales capturen a la
   vez): cantidad física (cero admitido) en presentación, unidad o unidad base; `null` borra la captura. **La hora del
   renglón la fija Go** (la de la transacción del guardado). Cada guardado es una transacción corta: no hay locks largos y la
   venta continúa.
3. **Diferencia = contado − existencia teórica en la hora del renglón**, calculada con el libro (Σ de los movimientos del
   almacén con `occurred_at` hasta esa hora). Se recalcula al consultar un conteo sin aplicar y se guarda al aplicarlo.
4. **Enviar** (`perform`, al menos un renglón contado) → **aprobar** (`inventory.counts.approve`) → **aplicar** (`approve`).
   Quien tiene `counts.approve` puede aprobar y aplicar su propio conteo.
5. **Aplicar**, en una sola transacción con 7820148308 exclusivo: primero procesa con el procesador del trabajador las
   solicitudes de consumo sin procesar con `released_at` hasta la hora máxima de los renglones contados (si alguna falla, se
   revierte todo y responde 409 `COUNT_CONSUMPTION_PENDING`); después registra por renglón con diferencia un `count_in`
   (sobrante, regla 3) o `count_out` (faltante, regla 1) con `occurred_at` = hora del renglón. **Los movimientos posteriores a
   la hora del renglón se conservan**; los renglones sin contar no se ajustan. Un sobrante sin ningún costo disponible hace
   fallar la aplicación con 409 `COST_REQUIRED` sin registrar nada (tampoco los consumos procesados en esa transacción): el
   conteo sigue aprobado con la misma revisión, se configura el costo de referencia y se vuelve a aplicar.
6. **Aplicar dos veces no genera otro ajuste**: el mismo `operation_id` devuelve la respuesta guardada y uno nuevo responde
   `COUNT_ALREADY_APPLIED`.
7. **Cancelar** con motivo: uno abierto, quien cuenta o quien aprueba; uno enviado o aprobado, solo quien aprueba.
8. **Conteo a ciegas**: la existencia teórica, las diferencias, la existencia actual, el movimiento aplicado de cada renglón
   (`movement_id`, `movement_kind`, `movement_label`, que revelarían el sentido de la diferencia) y los valores solo los ve
   quien tiene `inventory.counts.approve` o `inventory.stock.read` (valores además con `inventory.costs.read`).
9. **Avisos** de un conteo sin terminar (no bloquean): renglones sin contar, consumos anteriores sin calcular, cancelaciones
   sin clasificar anteriores a la hora del conteo y ajustes por autorizar del almacén sobre insumos contados.

Probado con ventas concurrentes: con una venta anterior cuya solicitud falla, aplicar responde `COUNT_CONSUMPTION_PENDING`
sin registrar nada; corregida la falla, aplicar procesa los consumos anteriores (no el posterior a la captura) y registra
limón teórico 94 → faltante 1, azúcar 5,000 g → sobrante 200 g y vino 50 → faltante 2, con el consumo posterior conservado.

## Niveles por almacén, alertas y sugerencias de compra

**Niveles por almacén** (`PUT stock-levels`, `inventory.items.manage`): existencia mínima y punto de reabastecimiento de un
insumo en un almacén; sobrescriben los del insumo para ese almacén (alertas del almacén y `GET stock?warehouse_id`, con
`level_scope`). Los niveles del insumo siguen comparándose con la existencia total.

**Alertas** (`GET alerts`, `inventory.read`): se calculan al consultar y nunca bloquean; `GET summary` agrega sus conteos
por gravedad y tipo.

| Tipo                          | Gravedad    | Cuándo                                                                                                                                                                          | Visibilidad            |
| ----------------------------- | ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------- |
| `negative_stock`              | crítica     | insumo activo con existencia negativa en un almacén                                                                                                                             | todos                  |
| `consumption_failed`          | crítica     | solicitudes de consumo fallidas (una alerta con el número)                                                                                                                      | todos                  |
| `below_minimum`               | advertencia | existencia bajo el mínimo del almacén o, sin nivel de almacén, la total bajo el del insumo                                                                                      | todos                  |
| `cost_pending`                | advertencia | insumo con cantidad pendiente de costo (salidas sin ningún costo)                                                                                                               | `inventory.costs.read` |
| `count_difference_pending`    | advertencia | conteo enviado o aprobado sin aplicar; el número de diferencias, calculadas ahora, solo con `counts.approve` o `stock.read` (si no, «… está pendiente de aprobar.» y `count` 1) | todos                  |
| `cancellations_unclassified`  | advertencia | cancelaciones pendientes (una alerta con el número y el vencimiento más próximo)                                                                                                | todos                  |
| `cash_link_expense_corrected` | advertencia | recepción con un vínculo vigente a un gasto corregido o regularizado después                                                                                                    | permisos de compras    |
| `reorder_point`               | información | existencia en o bajo el punto de reabastecimiento (por almacén o total)                                                                                                         | todos                  |
| `count_due`                   | información | almacén activo con existencias cuyo último conteo aplicado venció según la frecuencia configurada                                                                               | todos                  |
| `purchase_order_partial`      | información | orden parcialmente recibida                                                                                                                                                     | permisos de compras    |
| `item_without_supplier`       | información | insumo comprado activo sin proveedor activo que lo surta                                                                                                                        | todos                  |
| `product_without_recipe`      | información | producto (no combo) activo y vendible sin receta activa, si el aviso está activado                                                                                              | todos                  |

Cada alerta lleva texto sin identificadores crudos y una referencia navegable; las cantidades solo con
`inventory.stock.read`. La frecuencia de conteo (`none`, diaria, semanal, mensual o cada N días de 1 a 365) compara días
operativos con la hora de corte y solo genera la alerta `count_due`.

**Sugerencias de compra** (`GET purchase-suggestions`): insumos comprados activos con punto de reabastecimiento (el propio o
la suma de los de sus almacenes); necesidad = punto − (existencia total + pendiente de órdenes emitidas o parciales; los
borradores se informan aparte); se expresa en la presentación habitual del proveedor preferido, si no en la predeterminada
de compra, si no en la unidad base, redondeando hacia arriba a presentaciones completas. «Crear borrador»
(`POST purchase-suggestions/draft-order`, `inventory.purchases.create`) crea **un borrador** para el proveedor elegido
(precio = su último precio si se capturó en la misma presentación o unidad, si no 0; IVA de su última recepción, si no
16 %) y **nunca lo emite**; si una sugerencia ya no aplica responde 409 `SUGGESTION_CHANGED` sin crear nada. Visibilidad:
existencia, pendiente, borradores, necesidad, punto de reabastecimiento y cantidad sugerida (en presentaciones y en la unidad
base) solo con `inventory.stock.read` o `inventory.purchases.create`, porque con el punto y lo sugerido se deduciría la
existencia; sin ellos la sugerencia sigue listando el insumo (con su presentación y su mínimo) con esos campos en `null`.
Proveedor y último precio, solo con algún permiso de compras.

## Proveedores

- `inventory_suppliers`: nombre comercial único sin distinguir mayúsculas, razón social, RFC opcional con formato del SAT
  (normalizado en mayúsculas; **no único**), teléfono, correo, domicilio de varias líneas, contacto, observaciones, activo y
  revisión (`inventory.suppliers.manage`). **No hay borrado**: un proveedor se desactiva y conserva su historial; un
  proveedor desactivado no admite órdenes nuevas ni recepciones directas, pero la mercancía de una orden ya emitida se
  recibe.
- Insumos que surte (`inventory_supplier_items`): presentación habitual, código del proveedor, preferido (uno por insumo:
  marcar otro quita la marca del anterior, auditado) y último precio recibido (sin IVA y antes del descuento, con su
  presentación o unidad, fecha y recepción). Una recepción crea la relación si no existía.
- Historial: conteos, total recibido, valor devuelto y las 20 órdenes, recepciones y devoluciones más recientes.
- **Datos adicionales (9A)**: segundo teléfono, teléfono celular y página web, opcionales (`''` = sin dato). Go los valida
  solo a ellos para no invalidar proveedores existentes: teléfonos de una línea con dígitos, espacios, `+`, paréntesis,
  punto, guion o diagonal, de 7 a 20 dígitos y extensión opcional («ext. 12»); página web con dominio y ruta opcional, con o
  sin `http://`/`https://` (ningún otro esquema), sin espacios y de hasta 200 caracteres. La pantalla la muestra como texto,
  nunca como enlace. La búsqueda incluye los teléfonos nuevos y la página web. Un `PATCH` sin los campos nuevos los conserva (no
  los borra un cliente anterior); enviarlos vacíos (`""`) los borra.
- **«Compras de mostrador» (9A)**: proveedor especial (`kind = 'counter'`) para mercados, centrales de abastos,
  supermercados y comercios ocasionales. Lo siembra la 0016 una sola vez (índice único parcial: no puede haber otro); si la
  instalación ya tenía un proveedor con ese nombre, no se toca ni se convierte y el especial toma «Compras de mostrador
  (general)» o el siguiente nombre libre. No lleva RFC ni razón social (CHECK y Go), no se crea por la API (`kind` no se
  acepta en el cuerpo) y ninguna edición cambia la modalidad; se puede renombrar, describir y desactivar. **Solo se usa en
  recepciones directas**: las órdenes de compra (nuevas o al cambiar el proveedor de un borrador) y los borradores desde
  sugerencias lo rechazan con 409 `COUNTER_SUPPLIER`; no registra insumos surtidos (409
  `COUNTER_SUPPLIER`), por lo que nunca es preferido, y sus recepciones **no registran el último precio** (precios de
  comercios distintos que no deben alimentar sugerencias). La lista lo muestra primero entre los activos con la etiqueta
  «Compras de mostrador» y admite el filtro `kind`. `SupplierReference.kind` distingue la modalidad en órdenes, recepciones
  y devoluciones, para listas y reportes.
- **Lectura de compras**: cualquiera de los seis permisos de compras (no existe un permiso de solo lectura de compras). Los
  importes de compra los ve quien lee compras; el costeo de inventario, solo con `inventory.costs.read`.

## Órdenes de compra

- Estados: `draft` (borrador; crear y editar con `inventory.purchases.create`, reemplazando datos y renglones) → `issued`
  (emitir con `inventory.purchases.authorize`; exige al menos un renglón, proveedor e insumos activos) →
  `partially_received` / `received` por las recepciones. `cancelled` con motivo desde borrador, emitida o parcial (lo
  recibido se conserva) y `closed` (cierre administrativo con motivo) desde emitida o parcial. Una recibida no se cancela ni
  se cierra y no admite más recepciones.
- Los renglones solo cambian mientras la orden es borrador (disparador). Un insumo aparece una vez por orden. La fecha no es
  futura (zona del establecimiento).
- **Crear o emitir una orden no toca existencias ni caja.**
- Importes: ver [Recepciones](#recepciones). La interfaz muestra los importes que Go calcula al guardar el borrador (no hay
  vista previa antes de guardar).

## Recepciones

- `POST receipts` (`inventory.purchases.receive`): de una orden emitida o parcial, por sus renglones (cada uno una vez, en su
  misma presentación o unidad), o **directa** (sin orden; exige además `inventory.purchases.create` y un proveedor activo,
  **obligatorio**). En el restaurante del sistema anterior todas las compras eran recepciones directas: es el flujo rápido.
- Por renglón: solicitado, recibido antes, pendiente después, devuelto y devolvible. Recibir más de lo pendiente exige
  `allow_excess` (409 `RECEIPT_EXCEEDS_PENDING` sin él; aviso `RECEIPT_EXCESS` con él). Una orden con proveedor, insumo o
  presentación desactivados después de emitirse se recibe igual.
- Registra un movimiento `receipt` por renglón con el libro (regla 2 del motor: con déficit, el ajuste de costo va aparte), actualiza el último precio
  del proveedor y **no crea gastos de caja**. Es de solo inserción e idempotente.
- **Compra de mostrador (9A)**: recepción directa con el proveedor «Compras de mostrador» y el comercio concreto como
  referencia opcional (`counter_store`, una línea de hasta 100 caracteres; con un proveedor formal se rechaza con 400: para
  él está la referencia de factura o remisión). El comercio aparece en el motivo del movimiento («Recepción 8 · Compras de
  mostrador (Central de abastos)»), en la auditoría (`supplier_kind`, `counter_store`), en la búsqueda y en el detalle. El
  listado filtra por modalidad (`supplier_kind=formal|counter`). En la interfaz, «Compra de mostrador» abre la recepción
  directa con la modalidad preseleccionada (sin elegir proveedor y con el campo «Comercio (opcional)»); la recepción directa
  normal ofrece las dos modalidades y, con «Proveedor registrado», solo proveedores formales. Las recepciones históricas
  quedan formales y sin comercio.
- **Importes** (`inventory/purchasing.go`): importe del renglón = redondeo(cantidad × precio sin IVA); subtotal = importe −
  descuento comercial en pesos (nunca mayor que el importe); IVA = redondeo(subtotal × tasa ÷ 100) con tasa de **lista
  cerrada 16, 8 o 0**; total = subtotal + IVA. El documento es la suma exacta de sus renglones (el IVA no se recalcula sobre
  el total). Las compras guardan siempre subtotal, IVA y total por renglón y documento.
- En la recepción de una orden, precio y tasa son los de la orden salvo que se capturen otros (factura real), y el descuento
  del renglón se **prorratea de forma acumulada** por lo recibido: redondeo(D × mínimo(antes + ahora, Q) ÷ Q) −
  redondeo(D × mínimo(antes, Q) ÷ Q). Así las recepciones que completan la cantidad suman exactamente el descuento (12 + 8
  de 20 cajas con $50: $30.00 y $20.00) y lo recibido de más no lleva descuento.
- **IVA de las compras**: con «El IVA de las compras forma parte del costo» (activada por omisión) el valor de inventario de
  cada renglón es el total con IVA; desactivada, el subtotal sin IVA (el IVA queda guardado aparte en la compra). Cada
  recepción copia la configuración (`tax_in_cost`): cambiarla solo afecta recepciones futuras y se audita (probado: una
  recepción directa de $100.00 + IVA entra a $116.00, 0.058 por ml; con la configuración desactivada, el mismo documento
  entra a $100.00, 0.05 por ml, y la recepción anterior conserva su valor). Costo por unidad base = valor ÷ cantidad base
  (10 decimales).
- Ejemplo probado: orden de 20 cajas de 24 a $250 con $50 de descuento ($4,950.00 + IVA $792.00 = $5,742.00) recibida en
  12 + 8 cajas; 480 piezas por $5,742.00.

## Devoluciones a proveedor

`POST supplier-returns` (`inventory.supplier_returns.record`): ligada a una recepción, por renglón y en su misma
presentación o unidad, nunca más de lo recibido menos lo ya devuelto (`RETURN_EXCEEDS_RECEIVED`), con motivo (5–500).
Valor asociado = cantidad base × valor del renglón ÷ cantidad base recibida; salida `supplier_return` con la regla 5 del
motor (diferencia absorbida como ajuste de costo o guardada como no absorbida). **No genera ingresos de caja ni saldo a
favor.** Probado: 100 vasos a $2 y 100 a $3 con IVA (200 por $580); devolver 10 vasos sale en $29.00 con valor $34.80 y
ajuste −$5.80.

Desde la 9A, **Compras → Devoluciones a proveedor** lista todas las devoluciones (`GET supplier-returns`, que ya existía)
con proveedor y fechas de negocio, el valor asociado (y el no absorbido solo con costos) y «Ver recepción» para abrir la
recepción de origen, donde se registran.

## Vinculación con gastos de caja

- Vínculo **opcional** entre una recepción y uno o varios gastos de caja existentes (`POST receipts/{id}/cash-links`,
  `inventory.purchases.link_cash` + `cash.movements.read`), en una tabla propia: **no modifica `cash_operations`, los turnos
  ni el esperado**, y ninguna ruta crea, modifica o corrige operaciones de caja. Una compra sin pago vinculado y un gasto
  sin compra son válidos.
- Reglas: el gasto debe existir, ser un gasto (`kind='expense'`) en pesos (los gastos en dólares se rechazan) y no estar
  corregido. Σ vínculos vigentes de un gasto ≤ su importe y de una recepción ≤ su total (`CASH_EXPENSE_OVERLINKED`,
  `RECEIPT_OVERLINKED`): así se previenen egresos duplicados. Un vínculo vigente por par recepción–gasto; para cambiar el
  importe se desvincula (con motivo; queda registrado, no se borra) y se vincula de nuevo.
- Lectura del gasto con las reglas de caja: `cash.movements.read`, otra caja exige `cash.shifts.read_all` y un turno cerrado
  `cash.history.read` (sin la excepción de quien cerró el turno). Las rutas toman 7820148305 compartido antes de 7820148308.
  Los datos del gasto solo se entregan a quien puede leerlo.
- Un gasto **corregido o regularizado después** de vincularse se detecta: el vínculo pasa a `expense_corrected` o
  `expense_regularized`, la recepción marca `cash_link_attention` y aparece la alerta `cash_link_expense_corrected`; sigue
  contando en los límites hasta desvincularlo.
- Búsqueda de gastos candidatos (`GET cash-expenses`) por días de negocio, importe mínimo y máximo, caja y lo vinculado a una
  recepción, con lo disponible de cada gasto y el motivo si no se puede vincular.

## Relación con los demás módulos

- **Cocina**: el consumo nace de la liberación canónica y de los cambios que `kitchen.Integrate` ya recibía. La cocina no
  espera al inventario: la liberación solo inserta solicitudes, y las tareas por estación, las copias y las reimpresiones
  no consumen. Ver [KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md).
- **Pedidos**: confirmar, modificar y cancelar siguen igual; borradores, división, transferencia, combinación y cambio de
  mesa no consumen. Ver [RESTAURANT_OPERATIONS.md](RESTAURANT_OPERATIONS.md).
- **Cobros**: una devolución financiera no toca solicitudes ni movimientos y no reintegra ingredientes (probado). Ver
  [PAYMENTS_AND_REFUNDS.md](PAYMENTS_AND_REFUNDS.md).
- **Caja**: solo el vínculo opcional de arriba, sin modificar turnos. Ver
  [CASH_SHIFTS_AND_RECONCILIATION.md](CASH_SHIFTS_AND_RECONCILIATION.md).
- **Catálogo**: el inventario lee las selecciones estructuradas que ya guarda la partida; el `factor` de la porción del
  catálogo multiplica el consumo de una opción (no el precio). Ver [CATALOG.md](CATALOG.md).
- **Reportes (9A)**: leen el libro, las existencias, los conteos y las compras por SQL de solo lectura, sin el lock
  7820148308, y usan `inventory.TheoreticalCosts` para el costo teórico. Ver «Reportes · etapa 9A».

## Interfaz

Sección principal **Inventarios** (`desktop/src/pages/inventory/`), visible con `inventory.read` después de Catálogo y
elegible como pantalla de inicio. Pestañas por permiso, sin pestañas vacías, con el panel «Reintentar con el mismo
identificador» si una escritura quedó incierta. Componentes existentes (`Accordion`, `ConfirmDialog`, `notify.ts`,
`Form`), cantidades con `services/numericInput.ts` (`inventoryQuantityShape` de 6 decimales e `inventoryCostShape` de 10),
importes con `money.ts`, objetivos táctiles de 44 px y temas claro y oscuro con los tokens existentes. Sin identificadores
técnicos ni `type="number"`; React no calcula cantidades ni importes. Las consultas se recargan con los eventos
`inventory.*` (`adel-inventory-change`) y después de una escritura propia.

**Navegación (9A, §3.6).** Las trece pestañas planas de la etapa 8 se repartían en dos filas desiguales a 1280 × 720.
Desde la 9A, Inventarios tiene cinco **grupos** en la primera fila — Resumen, Catálogo, Existencias, Compras y
Configuración — y las **secciones** del grupo en una segunda fila (`pages/inventory/inventorySections.ts`), ambas de
columnas iguales en una sola línea (sin filas irregulares; en ventanas angostas las etiquetas pasan a dos líneas dentro de
su columna) y con objetivos de 44 px. Cada sección conserva el permiso de la etapa 8; un grupo sin secciones permitidas no
aparece y la segunda fila se omite solo en Resumen. Cada grupo recuerda su última sección mientras la pantalla sigue
abierta. Los destinos internos (`InventoryTarget`: alertas, documentos relacionados, «Ver insumos») cambian de grupo y de
sección; el de la etapa 8 `purchases` con `subsection: 'suggestions'` abre Sugerencias de compra. El menú principal de la
aplicación no cambió.

| Grupo         | Sección                  | Permiso                                                      | Contenido                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| ------------- | ------------------------ | ------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Resumen       | Resumen                  | `inventory.read`                                             | Estado de «Control de inventario» con «Administrar funciones»; cifras de Go de las fases A–D (insumos, almacenes, negativas, bajo mínimo, reabastecimiento, costo pendiente con costos, valor del inventario con valuación, cancelaciones por clasificar, solicitudes con error, ajustes por autorizar, conteos en curso, órdenes abiertas e insumos por reabastecer), cada una con su permiso y acceso a su pestaña, y «Alertas de inventario» (conteos por gravedad de Go, filtro por gravedad, grupos por tipo y acceso directo al insumo, existencia, receta, conteo, orden, recepción o movimientos según la referencia y los permisos). |
| Catálogo      | Insumos                  | `inventory.read` (edición con `inventory.items.manage`)      | Listado con búsqueda y filtros; alta y edición con tipo, unidad base, costo de referencia y niveles del insumo; presentaciones y «Niveles por almacén» en el mismo panel; código interno sugerido (9A) y «+ Nueva categoría» (9A).                                                                                                                                                                                                                                                                                                                                                                                                            |
| Catálogo      | Recetas                  | `inventory.read` (escritura según el tipo)                   | Apartados «Productos y complementos» (destinos con su estado y filtro de lo pendiente), «Preparaciones» (inmediatas y almacenables), «Porciones habituales» y «Elaboraciones»; editor versionado con costo teórico e historial de versiones.                                                                                                                                                                                                                                                                                                                                                                                                  |
| Catálogo      | Almacenes                | `inventory.read` (edición con `inventory.warehouses.manage`) | Almacenes con el predeterminado marcado y «Almacén de consumo por estación» con su historial.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| Catálogo      | Proveedores              | cualquiera de los permisos de compras                        | Proveedores con búsqueda; alta, edición y desactivación con confirmación; insumos que surte con último precio; historial; segundo teléfono, celular y página web (9A); «Compras de mostrador» con su explicación y sin RFC ni insumos surtidos (9A).                                                                                                                                                                                                                                                                                                                                                                                          |
| Existencias   | Existencias              | `inventory.stock.read`                                       | Existencia por insumo, total y por almacén, mínimos y situación, e indicación del nivel comparado («Nivel de <almacén>» o «Nivel del insumo (existencia total)»); costo promedio con costos y valor del insumo solo con costos y valuación; negativas resaltadas; «Existencia inicial».                                                                                                                                                                                                                                                                                                                                                       |
| Existencias   | Movimientos              | `inventory.movements.read`                                   | Libro con filtros de insumo, almacén, tipo, origen y fechas de negocio; valores con costos y valuación posterior con costos y valuación; «Solicitudes de consumo» con errores y diagnósticos; traza de una partida.                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Existencias   | Transferencias           | `inventory.transfers.record` o `inventory.movements.read`    | Listado con filtros; alta con origen y destino distintos (el destino no ofrece el origen), renglones sin signo y confirmación; detalle con la existencia de Go en cada almacén después (negativa resaltada), valor trasladado y avisos.                                                                                                                                                                                                                                                                                                                                                                                                       |
| Existencias   | Ajustes y mermas         | `inventory.read`                                             | «Registrar ajuste» (`adjustments.record`) y «Registrar merma» (`waste.record`), «Cancelaciones por clasificar», «Ajustes y mermas registrados» (con sus permisos de lectura) e «Historial de clasificaciones» en una sola vista.                                                                                                                                                                                                                                                                                                                                                                                                              |
| Existencias   | Conteos                  | `inventory.counts.perform` o `inventory.counts.approve`      | Listado («En curso» por omisión), alta por almacén y alcance, hoja de conteo para tableta y pasos del conteo (ver abajo).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Compras       | Órdenes de compra        | cualquiera de los permisos de compras                        | «Órdenes de compra»: órdenes con búsqueda y filtros, editor del borrador con renglones, tasa por botones y totales de Go, emitir, cancelar y cerrar con su permiso y detalle con lo recibido. Solo proveedores formales (9A).                                                                                                                                                                                                                                                                                                                                                                                                                 |
| Compras       | Sugerencias de compra    | cualquiera de los permisos de compras                        | Sección propia desde la 9A (antes, apartado de «Compras»; ver abajo).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Compras       | Recepciones              | cualquiera de los permisos de compras                        | Recepciones de una orden (proponen lo pendiente de Go; exceso con confirmación) y directas; detalle; devolución a proveedor; vínculo con gastos de caja. «Compra de mostrador» y filtro por modalidad (9A).                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| Compras       | Devoluciones a proveedor | cualquiera de los permisos de compras                        | Lista nueva (9A): todas las devoluciones con proveedor, fechas, valor y acceso a la recepción.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| Configuración | Unidades                 | `inventory.read` (edición con `inventory.items.manage`)      | Unidades de medida por dimensión (antes, acordeón de Insumos).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| Configuración | Categorías               | `inventory.read` (edición con `inventory.items.manage`)      | Categorías de insumos con búsqueda, «Ver insumos», activar o desactivar y editar (9A; antes, acordeón de Insumos).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| Configuración | Parámetros               | `inventory.settings.manage`                                  | IVA de las compras en el costo, hora de corte del día operativo (desde la 9A, solo lectura: se cambia en Configuración general), aviso de productos sin receta y frecuencia de conteo.                                                                                                                                                                                                                                                                                                                                                                                                                                                        |

Pantallas de la fase D:

- **Ajustes y mermas**: motivo de lista cerrada con botones (la merma solo ofrece Rotura, Derrame, Caducidad, Consumo
  extraordinario y Otro), almacén, detalle del motivo (obligatorio con «Otro»), renglones con cantidad **sin signo** y
  «Faltante (salida)» / «Sobrante (entrada)» (deshabilitado el sentido que el motivo no admite; la merma solo registra
  salidas), observaciones y confirmación que resume cada renglón y si se aplicará o quedará pendiente; la pantalla antepone
  «-» al texto de una salida (operación de texto, sin aritmética) y Go valida el sentido. Listado con filtros (folio, tipo,
  estado, motivo, almacén, fechas) y detalle con quién registró, autorizó, aplicó o rechazó, renglones con lo capturado, la
  cantidad base con signo, el movimiento, la existencia después y el valor, y los avisos de Go. «Autorizar y aplicar» y
  «Rechazar» (con motivo) solo si Go entrega `authorizable`, ambos con confirmación; si no, se explica que quien registró el
  ajuste no puede autorizarlo. Un rechazo `COST_REQUIRED` ofrece «Abrir insumo» o «Ir a Insumos».
- **Hoja de conteo**: aviso «La venta sigue durante el conteo…», búsqueda rápida sin acentos, filtro Todos / Sin contar /
  Contados, renglones grandes con «Cantidad contada» (`numericInput`), la unidad base y la capturada («Otra unidad o
  presentación…» consulta solo ese insumo), «Borrar captura», «Quitar», estado «Contado · hora · quién» y «Sin guardar».
  «Guardar avance» envía los renglones cambiados en bloques de 200 (cada bloque una operación). Con `theoretical_visible`,
  el teórico a la hora del renglón, la diferencia con su etiqueta (Sobrante, Faltante, Sin diferencia), la existencia actual
  y, al aplicar, «Teórico aplicado» y el movimiento; sin él, el aviso «Conteo a ciegas» y ninguna de esas columnas. Pasos
  con quién y cuándo; «Enviar a revisión» (`perform`, bloqueado con capturas sin guardar), «Aprobar» y «Aplicar ajustes»
  (`approve`) y «Cancelar conteo» (con motivo), todos con confirmación; avisos vigentes de Go.
- **Niveles por almacén** (editor del insumo): cada almacén activo (y los inactivos con nivel) con su nivel o «Usa los
  niveles del insumo», la existencia y «Bajo mínimo» / «Reabastecer» (con `stock.read`); «Definir nivel» / «Editar nivel» y
  «Quitar nivel» con `items.manage`.
- **Sugerencias de compra** (Compras): búsqueda y categoría; grupos por proveedor preferido en el orden de Go; por insumo la
  existencia, lo pendiente de recibir y en borradores, el punto de reabastecimiento y su origen, la necesidad, lo sugerido en
  la presentación de compra y el último precio, cada dato solo si Go lo entrega («—» si llega en `null`). Con
  `purchases.create`: «Incluir» por insumo, proveedor del borrador para los que no tienen preferido y «Crear orden en
  borrador» con confirmación, que abre el borrador en Compras. Nunca se emite.

En la captura de pedidos la etapa solo cambió lo indispensable: las indicaciones con efecto y las tarjetas de complementos
(ver arriba). No se agregaron formularios administrativos al flujo de captura.

## API

Prefijo `/api/v1/inventory`. Toda ruta exige `inventory.read` además de su permiso; todos los permisos se comprueban antes
de escribir (un 403 revierte y solo audita la denegación). Contratos, cuerpos, respuestas y errores en
[API_CONTRACTS.md](../API_CONTRACTS.md); tipos TypeScript en `desktop/src/services/inventory.ts`, `inventoryRecipes.ts`,
`inventoryPurchasing.ts` e `inventoryControl.ts`.

| Fase | Rutas bajo `/inventory`                                                                                                                                                                                                   |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A    | `summary`, `settings`, `units`, `categories`, `items`, `items/{id}/presentations`, `presentations/{id}`, `warehouses`, `station-warehouses`, `stock`, `movements`, `initial-balances`                                     |
| B    | `recipes`, `recipe-versions/{id}`, `recipe-targets`, `ingredient-portions`, `cancellations`, `cancellations/{id}/classify`, `productions`, `consumption-requests`, `consumption/{unidad}`                                 |
| C    | `suppliers`, `suppliers/{id}/items`, `suppliers/{id}/items/remove`, `purchase-orders` (`issue`, `cancel`, `close`), `receipts`, `supplier-returns`, `cash-expenses`, `receipts/{id}/cash-links`, `cash-links/{id}/unlink` |
| D    | `adjustments` (`authorize`, `reject`), `transfers`, `counts` (`lines`, `submit`, `approve`, `apply`, `cancel`), `stock-levels`, `alerts`, `purchase-suggestions`, `purchase-suggestions/draft-order`                      |
| 9A   | `item-codes` (nueva, lectura con `inventory.read`); filtros `suppliers?kind=` y `receipts?supplier_kind=`; campos nuevos en `suppliers` y `receipts`                                                                      |

La fase B (indicaciones con efecto) amplía además `catalog/quick-notes`, `customization.quick_notes` y el desglose de la
partida, sin rutas nuevas. No existen rutas de borrado de documentos ni de edición directa de existencias: cada efecto es un
movimiento del libro. Errores propios: `INVENTORY_VALIDATION` (400), `PERMISSION_DENIED` (403), `NOT_FOUND` (404), los 409
de cada ruta (entre ellos `COST_REQUIRED` en `POST adjustments`, `POST adjustments/{id}/authorize` y
`POST counts/{id}/apply`), `REVISION_CONFLICT` e `IDEMPOTENCY_CONFLICT`; los mensajes no contienen costos.

## Permisos

Los 24 permisos de `inventory.Permissions` (`inventory/context.go`), insertados por la migración de su fase con la misma
descripción (una prueba lo compara) y concedidos inicialmente solo al rol `administrator`. **Consultar existencias nunca
implica consultar costos**: sin `inventory.costs.read` los costos y valores llegan en `null` con `costs_visible: false`, no se
listan los ajustes de costo ni la marca de costo pendiente. La valuación **por insumo** (el valor de una existencia,
`StockRow.value`, y la valuación posterior de cada movimiento, `valuation_value_after`, con la bandera `valuation_visible`)
exige `inventory.costs.read` **e** `inventory.valuation.read`, porque con existencias o con movimientos consecutivos
cualquiera de los dos revelaría el otro; la valuación total del inventario exige `inventory.valuation.read`. Con solo
`inventory.costs.read` se ven el costo promedio, la marca de costo pendiente y el valor de cada movimiento. Sin
`inventory.stock.read` tampoco se filtran existencias por otras vías: las sugerencias de compra ocultan punto de
reabastecimiento y cantidad sugerida, la alerta de un conteo pendiente no dice cuántas diferencias tiene y un conteo a
ciegas oculta el movimiento de cada renglón.

| Permiso                             | Descripción                                                             | Migración |
| ----------------------------------- | ----------------------------------------------------------------------- | --------- |
| `inventory.read`                    | Consultar inventario (sección y resumen)                                | 0010      |
| `inventory.items.manage`            | Administrar insumos, unidades y presentaciones                          | 0010      |
| `inventory.costs.read`              | Consultar costos (unitarios, valores, costo teórico)                    | 0010      |
| `inventory.costs.manage`            | Administrar costos de referencia y costos de existencias iniciales      | 0010      |
| `inventory.stock.read`              | Consultar existencias (cantidades, sin costos)                          | 0010      |
| `inventory.warehouses.manage`       | Administrar almacenes y su asignación a estaciones                      | 0010      |
| `inventory.movements.read`          | Consultar movimientos (cantidades; valores solo con consulta de costos) | 0010      |
| `inventory.adjustments.authorize`   | Autorizar ajustes y corregir clasificaciones vencidas                   | 0010      |
| `inventory.valuation.read`          | Consultar valuaciones (valor total del inventario por insumo/almacén)   | 0010      |
| `inventory.settings.manage`         | Administrar parámetros de inventario                                    | 0010      |
| `inventory.recipes.manage`          | Administrar recetas y porciones habituales                              | 0011      |
| `inventory.subrecipes.manage`       | Administrar subrecetas (preparaciones)                                  | 0011      |
| `inventory.production.record`       | Registrar elaboraciones de subrecetas almacenables                      | 0011      |
| `inventory.waste.record`            | Registrar mermas y clasificar cancelaciones                             | 0011      |
| `inventory.suppliers.manage`        | Administrar proveedores                                                 | 0013      |
| `inventory.purchases.create`        | Crear órdenes de compra                                                 | 0013      |
| `inventory.purchases.authorize`     | Emitir (autorizar), cancelar y cerrar órdenes de compra                 | 0013      |
| `inventory.purchases.receive`       | Recibir mercancía                                                       | 0013      |
| `inventory.supplier_returns.record` | Registrar devoluciones a proveedores                                    | 0013      |
| `inventory.purchases.link_cash`     | Vincular compras con gastos de caja                                     | 0013      |
| `inventory.adjustments.record`      | Registrar ajustes de inventario                                         | 0014      |
| `inventory.counts.perform`          | Realizar conteos físicos                                                | 0014      |
| `inventory.counts.approve`          | Aprobar y aplicar conteos                                               | 0014      |
| `inventory.transfers.record`        | Transferir mercancía entre almacenes                                    | 0014      |

Combinaciones relevantes: la existencia inicial exige `inventory.adjustments.authorize` + `inventory.costs.manage`; enviar
un costo de referencia, `inventory.items.manage` + `inventory.costs.manage`; vincular gastos, `inventory.purchases.link_cash`

- `cash.movements.read`; la recepción directa, `inventory.purchases.receive` + `inventory.purchases.create`. El permiso
  «Consultar existencias» y el de conteos no muestran costos; un conteo es a ciegas para quien solo cuenta. Como en todas las
  etapas, un permiso nuevo se concede al administrador en la misma migración para que siga siendo administrador completo.

## Eventos en tiempo real

Ámbito `inventory` de `realtime_events` (agregado por la 0010 al CHECK de ámbitos), recibido con `inventory.read`. Los
eventos solo identifican qué cambió: **no llevan costos ni importes** y la terminal vuelve a consultar con sus permisos. La
recuperación ante desconexiones es la existente (instantánea, cursor y reinicio).

| Evento                          | Se publica al                                                                                                                                               |
| ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `inventory.items.changed`       | Crear o editar unidades, categorías, insumos, presentaciones y niveles por almacén                                                                          |
| `inventory.settings.changed`    | Cambiar la configuración, los almacenes, las asignaciones de estaciones o la función `inventory_control`                                                    |
| `inventory.stock.changed`       | Cualquier movimiento que cambie existencias (existencia inicial, consumo, clasificación, elaboración, recepción, devolución, ajuste, transferencia, conteo) |
| `inventory.movements.recorded`  | Registrar movimientos en el libro                                                                                                                           |
| `inventory.recipes.changed`     | Guardar recetas y porciones habituales                                                                                                                      |
| `inventory.purchases.changed`   | Proveedores, sus insumos y órdenes (también al recibir una orden y al crear un borrador desde sugerencias)                                                  |
| `inventory.receipts.changed`    | Recepciones, devoluciones y vínculos con gastos                                                                                                             |
| `inventory.adjustments.changed` | Registrar, autorizar o rechazar ajustes y mermas                                                                                                            |
| `inventory.transfers.changed`   | Registrar transferencias                                                                                                                                    |
| `inventory.counts.changed`      | Crear, editar, capturar, enviar, aprobar, aplicar o cancelar conteos                                                                                        |
| `inventory.alerts.changed`      | Cancelaciones por clasificar o resueltas, solicitudes que fallan o se recuperan y operaciones de la fase D que cambian existencias o conteos                |

## Auditoría

En la misma transacción de cada operación: `inventory.unit_created|updated`, `inventory.category_created|updated`,
`inventory.categories_reordered`, `inventory.item_created|updated`, `inventory.presentation_created|updated`,
`inventory.warehouse_created|updated`, `inventory.station_warehouse_assigned`, `inventory.initial_balance_recorded`,
`inventory.settings_changed`; `inventory.recipe_created`, `inventory.recipe_versioned`, `inventory.ingredient_portion_saved`,
`inventory.cancellation_classified`, `inventory.cancellation_corrected`, `inventory.production_recorded` y, con el proceso
como responsable, `inventory.cancellation_reconciled`; `inventory.supplier_created|updated|deactivated`,
`inventory.supplier_item_saved|removed`, `inventory.purchase_order_created|updated|issued|cancelled|closed`,
`inventory.receipt_recorded`, `inventory.supplier_return_recorded`, `inventory.cash_link_created|removed`;
`inventory.adjustment_recorded|requested|authorized|rejected`, `inventory.waste_recorded`, `inventory.transfer_recorded`,
`inventory.count_created|updated|lines_captured|submitted|approved|applied|cancelled`, `inventory.stock_level_saved` e
`inventory.purchase_order_drafted_from_suggestions`. Los movimientos del trabajador no se auditan uno por uno: el libro los
atribuye al proceso `inventory_worker`. Los registros pueden contener costos (solo `audit.read`).

## Pruebas

Todas se ejecutaron en una copia aislada con un clúster PostgreSQL temporal (puerto 55499), nunca contra la instalación
real; resultados y tiempos por fase en [VERIFICATION_STAGE8.md](VERIFICATION_STAGE8.md).

- **Go unitarias** (`backend/internal/inventory`):
  - `costing_test.go`: el motor con todos los casos de la tabla de arriba, entradas inválidas y 200 secuencias
    pseudoaleatorias; cada estado comprueba los invariantes. Con el `Restore` y el `settle` anteriores a la fase F fallan los
    casos de reversión, último costo válido, invariante y secuencias.
  - `units_test.go`: kg ↔ g, l ↔ ml, onza redondeada a 6 decimales, conversión que se redondea a cero, caja de 24, paquete de
    12, costal de 25 kg en un insumo en gramos y rechazo entre dimensiones.
  - `resolution_test.go`: producto normal, «Sin cebolla» → 0, «Con cebolla» → 1, «Cebolla extra» → 2, 3 porciones y media
    porción; observación libre ignorada; ingrediente sin porción sin consumo inventado; producto sin configuración;
    complementos sustituidos (filete: pasta y ensalada, no arroz), dos porciones de papas, factor de porción; complemento
    ligado a producto; «solo para llevar» en salón y para llevar; preparaciones inmediatas y almacenables; recetas de
    variante; combo en dos estaciones con almacenes distintos; elección de almacén; límite de profundidad; ciclos.
  - `operating_day_test.go`: corte de las 05:00 en Monterrey y en Tijuana con adelanto y atraso de horario.
  - `purchasing_test.go`: importes de renglones y documentos con IVA 16/8/0 y redondeo; prorrateo acumulado del descuento;
    RFC; devoluciones exactas.
  - `control_test.go`: redondeo de sugerencias, vencimiento por frecuencia de conteo, diferencias, coherencia de motivos y
    mermas, resolución de rutas y tipos de alerta.
  - `permissions_test.go`: los 24 permisos de Go están en la 0010, 0011, 0013 o 0014 con la misma descripción, solo para el
    administrador.
- **Go unitarias de otros paquetes**: `catalog/quick_note_effects_test.go` (efectos, cantidades, conflicto, precio igual al
  del botón «Extra», desglose congelado y camino «Preserve»), `kitchen/quick_notes_template_test.go` (comanda sin efecto
  idéntica a la de la 7.1 e indicación impresa una sola vez) y `operations/quick_notes_fingerprint_test.go` (huellas de
  borradores sin indicaciones sin cambios).
- **Integración PostgreSQL** (`//go:build integration`, encadenadas al final de `TestRestaurantAcceptance` en
  `internal/operations`, así que cada fase repitió también la aceptación de las etapas 4 a 7.1):
  - `inventory_core_test.go` e `inventory_ledger_test.go` (fase A): siembras, unidades, categorías, insumos, presentaciones,
    almacenes, asignaciones con historial, existencia inicial idempotente, libro por llamadas directas (cruce a negativo,
    recepción con ajuste de costo, transferencia), inmutabilidad, consultas, visibilidad de costos, configuración, función
    con `enabled_at` y eventos.
  - `inventory_recipes_test.go` e `inventory_consumption_test.go` (fase B): porciones, recetas y versiones, subrecetas,
    elaboraciones, destinos; función desactivada sin consumo, activación, programado liberado después de activar; borrador
    y cuenta sin consumo, una solicitud por unidad, reintento HTTP, reproceso; las personalizaciones exigidas, complementos,
    combos y empaques; reimpresión, división, cambio de mesa y combinación sin consumo; modificación de partidas liberadas,
    receta cambiada antes de procesar; cancelaciones, clasificaciones, reconciliación y corrección; subreceta sin doble
    descuento; devolución financiera sin reintegración; falla controlada y recuperación; traza y eventos; y la medición
    opcional.
  - `quick_note_effects_test.go` (fase B): administración con permiso, precio igual al botón «Extra», 422 de cantidad y de
    conflicto, partida congelada, partidas antiguas sin `quick_notes`, «Preserve» y componentes de combo; y
    `catalog/quick_note_effects_migration_test.go` (0012 sobre el esquema 11).
  - `inventory_purchasing_test.go` (fase C, 10 subpruebas): proveedores, órdenes, recepción parcial, total, repetida y con
    exceso, cierre y cancelación, recepción directa con IVA incluido y excluido, recepción con existencia negativa,
    devoluciones con diferencia absorbida y no absorbida, vínculos con gastos (gasto sin compra, compra sin pago,
    vinculación total y parcial, prevención de egresos duplicados, gasto corregido o regularizado después, turnos cerrados,
    dólares, desvinculación) y desactivación de un proveedor.
  - `inventory_control_test.go` (fase D, 9 subpruebas): ajustes y mermas con autorización y sin autoautorización,
    transferencias sin cambio de costo, conteo con ventas concurrentes y aplicación idempotente, flujo de conteos, niveles
    por almacén, cada tipo de alerta, sugerencias y borrador, eventos, auditoría y consistencia del libro.
  - Correcciones de la fase F: `inventory_b_entries_with_negative_stock` (salsa elaborada después de venderse y reversión
    de un postre con la crema en negativo, con sus ajustes de costo y el consumo neto de la unidad en cero),
    `inventory_d_entries_without_cost` (`COST_REQUIRED` en el conteo, el ajuste pendiente y el directo, sin registrar nada,
    y aplicación después de configurar el costo de referencia), `inventory_cost_visibility` (costo promedio sin valor por
    insumo ni valuación posterior con solo `costs.read`; con `valuation.read`, ambos) y las ampliaciones de conteos a ciegas,
    alertas y sugerencias sin `stock.read`. En una prueba de mutación que reintrodujo los seis defectos fallaron esas
    subpruebas.
  - En cada fase, consistencia del libro: Σ existencias = cantidad de la valuación, Σ valores = valor de la valuación y
    último estado = valuación.
- **Actualización desde el esquema 9** (`database/existing_install_test.go`, `TestStage8UpgradeOfExistingInstallation`, con
  `stage8_purchasing_upgrade_test.go` y `stage8_control_upgrade_test.go`): una instalación simulada en 0.7.1 con roles,
  turnos, gastos corregidos y eventos recibe 0010–0014, que no se repiten; los datos anteriores no cambian; 24 permisos solo
  del administrador; siembras, restricciones con nombre, CHECK con tablas temporales y disparadores reales.
- **Rust** (`api.rs`): `inventory_routes_are_closed_and_keep_write_methods`, `inventory_query_parameters_are_whitelisted`,
  `inventory_recipe_and_consumption_routes_are_closed`, `inventory_purchasing_routes_are_closed` e
  `inventory_control_routes_are_closed`: lista cerrada, métodos, rutas sin identificador o con `../../settings` rechazadas,
  sin rutas de borrado ni de edición de existencias y parámetros de consulta en lista blanca.
- **Vitest**: `pages/inventory/inventory.test.tsx` e `inventoryStock.test.tsx` (fase A), `inventoryRecipes.test.tsx` e
  `inventoryConsumption.test.tsx` (fase B), `inventoryPurchasing.test.tsx` (fase C), `inventoryControl.test.tsx` (fase D:
  13 pestañas por permiso, ajustes con sentido por botones y sin autoautorización, mermas, `COST_REQUIRED`, transferencias,
  conteo a ciegas y con diferencias, niveles por almacén, alertas agrupadas, sugerencias con y sin existencias, valor por
  insumo y valuación posterior ocultos sin `valuation.read`);
  `pages/restaurant/capture/quickNotesCapture.test.tsx` y `capture.test.tsx` (indicaciones y complementos en la captura);
  `pages/catalog/quickNotes.test.tsx` (efecto en el catálogo); `hooks/useRealtime.test.tsx` (`adel-inventory-change`).
  Comprueban cuerpos exactos enviados a Go, permisos, costos ocultos, confirmaciones y que no haya `type="number"`.
- **Playwright** (Chromium y WebKit, con Go y PostgreSQL temporales reales): `inventory.spec.ts` (insumo, presentación y
  existencia inicial), `takeaway-quick-notes.spec.ts` (complementos e indicaciones con efecto en la captura),
  `stock-consumption.spec.ts` (receta, porción, activación, pedido enviado a cocina, consumo en Movimientos, traza,
  cancelación y «No preparado»), `vendor-purchases.spec.ts` (proveedor, orden, recepción parcial y total, recepción
  directa y vínculo con un gasto) y `warehouse-control.spec.ts` (13 pestañas en orden sin desplazamiento a 1280 × 720,
  transferencia de 5 kg entre almacenes, merma de 1 kg por rotura, conteo de la barra con faltante de 0.5 kg enviado,
  aprobado y aplicado, el `count_out` en Movimientos y la alerta «Bajo mínimo» con acceso a Existencias, confirmados en Go).
  Cada pantalla se comprueba sin desbordamiento a 1280 × 720 y 1920 × 1080 en ambos temas.

- **Etapa 9A (A2)**, también en la copia aislada:
  - Go unitarias `inventory/suppliers_test.go`: formatos aceptados y rechazados del segundo teléfono, el celular y la
    página web, y la regla de sufijos del código sugerido (los mismos casos que `services/productCode.test.ts`).
  - Integración `operations/inventory_stage9a_test.go` (al final de las compras de la etapa 8): siembra única del proveedor
    de mostrador y su índice y CHECK, campos nuevos (opcionales, validados, búsqueda, edición y proveedores existentes sin
    ellos), rechazo en órdenes (nuevas y al cambiar el proveedor de un borrador), en sugerencias y en insumos surtidos,
    recepciones directas con y sin comercio, sin último precio, filtro por modalidad, búsqueda por comercio, auditoría,
    historial, desactivación y recepciones históricas intactas; comprobación del código interno (tomado, sugerencia con
    `-2`/`-3`, el propio insumo, comodines, permisos y validación). `database/stage9a_inventory_upgrade_test.go`: la
    actualización desde el esquema 14 con compras de la etapa 8 y proveedores «Compras de mostrador» y «Compras de
    mostrador (general)» ya existentes (no se tocan; el especial queda «Compras de mostrador (general 2)»), restricciones con
    nombre, CHECK con copias temporales e índice único.
  - Vitest `pages/inventory/inventoryStage9A.test.tsx` (código sugerido sin sobrescribir lo escrito, alta rápida de
    categoría que conserva el formulario, categorías en Configuración, compra de mostrador, filtro por modalidad, proveedores
    formales en órdenes y devoluciones a proveedor), `services/productCode.test.ts`, `pages/catalog/codeSuggestions.test.tsx`
    (presentaciones nuevas y copia de un producto) y las pruebas de navegación agrupada por permisos en
    `inventoryControl.test.tsx`, `inventory.test.tsx` e `inventoryPurchasing.test.tsx`.
  - Playwright `e2e/workflow-inventory-9a.spec.ts` (navegación agrupada sin desbordamiento a 1280 × 720 y 1920 × 1080 en
    ambos temas, código sugerido por Go, categoría creada desde el editor, «Ver insumos», compra de mostrador con comercio y
    devoluciones) y las especificaciones de inventario de la etapa 8 adaptadas a los grupos (`e2e/inventory-navigation.ts`).

## Reportes · etapa 9A

Los reportes de inventarios, costos y márgenes, compras y proveedores del módulo Reportes ([REPORTS.md](REPORTS.md))
leen las tablas de este módulo en una transacción de solo lectura propia, **sin el lock 7820148308** y sin escribir nada;
el inventario no cambió sus reglas para ellos.

- **Permisos**: además de `reports.inventory.read` (o `reports.purchases.read` para compras y proveedores, y
  `reports.sales.read` + `inventory.costs.read` para costos y márgenes), cada reporte exige los permisos de la etapa 8:
  existencias, negativos, bajo mínimo y conteos con `inventory.stock.read`; kardex, entradas, salidas, transferencias,
  ajustes, mermas y consumos con `inventory.movements.read`; valuación con `inventory.valuation.read` (el valor por insumo
  además con `inventory.costs.read`) y costos promedio con `inventory.costs.read`. Sin `inventory.costs.read` no viaja
  ningún costo, valor, base de costo ni ajuste de costo, tampoco en los archivos PDF y Excel.
- **Consumo**: el neto de `consumption` + `consumption_reversal` de las unidades, fechado con la liberación canónica; una
  unidad comercial se cuenta una sola vez aunque consuma en varios almacenes. Con solicitudes pendientes o con error en el
  periodo, el aviso `CONSUMPTION_PENDING` advierte que los datos pueden estar incompletos.
- **Mermas**: la merma operativa con su motivo y las cancelaciones «Preparado / merma» con su consumo conservado; las
  cancelaciones «No preparado» y por clasificar se cuentan aparte. Una devolución de dinero nunca es merma.
- **Costo de lo vendido y margen**: el costo neto de los consumos de cada unidad reconocida como venta (sin los
  `cost_adjustment`, que se reportan como concepto del periodo), con los estados «Sin costo», «Estimado» y firme.
- **Compras recibidas por periodo** (el reporte que tenía el sistema anterior): recepciones de una orden, directas y
  «Compras de mostrador» con el comercio, por proveedor, insumo, categoría, almacén, modalidad y estado de la orden; lo
  pagado desde caja se muestra como parte del total, nunca como otra compra, y las devoluciones a proveedor van aparte.
- **Límites**: no hay valuación por almacén (un costo por insumo) y el costo teórico es el vigente, sin historial; el
  kardex de un año con mucho volumen puede exceder los 100 000 renglones de un reporte (se consulta por mes o con
  filtros).

## Mediciones del consumo asíncrono

Prueba `inventory_b_measure_release` (opcional con `ADEL_INVENTORY_MEASURE=1`): 200 confirmaciones con envío a cocina con la
función activada y 200 desactivada, en bloques alternados de 50 tras 5 de calentamiento, midiendo solo el
`POST …/confirm`, en la copia aislada y el clúster temporal de este equipo:

| Ejecución                   | Desactivada (media / p95 / máx.) | Activada (media / p95 / máx.) | Trabajador                             |
| --------------------------- | -------------------------------- | ----------------------------- | -------------------------------------- |
| `go test -tags=integration` | 6.79 / 7.83 / 9.73 ms            | 7.01 / 7.74 / 8.11 ms         | 205 solicitudes en 1.23 s (6.0 ms c/u) |
| `go test -race`             | 17.58 / 20.70 / 21.76 ms         | 18.03 / 21.22 / 23.04 ms      | 205 en 1.35 s (6.6 ms c/u)             |

La liberación con la función activada solo agrega una lectura por clave primaria y un `INSERT … SELECT` (≈ 0.2 ms de media
sin el detector de carreras). Resolver y escribir el consumo de una unidad cuesta ≈ 6 ms: hacerlo dentro de la liberación
alargaría el lock 7820148305 unas 30 veces más por unidad, así que se conservó el consumo asíncrono. Sin concurrencia real de
terminales.

## Limitaciones conocidas

Reunidas de todas las fases del avance (las que se resolvieron en una fase posterior no se repiten):

- **Instalación real**: 0010–0014 y, después, la 0016 de la etapa 9A se aplicaron el 10 de noviembre de 2026 con
  autorización del propietario; «Compras de mostrador» quedó único y activo, sin recepciones.
- **Sin verificar en hardware**: ventana nativa (WKWebView), pantalla táctil física y teclado virtual en Inventarios; ninguna
  comanda con indicaciones con efecto se imprimió en papel.
- **Volumen y concurrencia**: las mediciones son de este equipo, sin varias terminales reales; las alertas (también las de
  `GET summary`) se calculan en cada consulta, sin historial ni medición con volumen real; la cantidad pendiente de costo es
  un indicador agregado no probado bajo carga. La aceptación encadenada de `internal/operations` dura unos 221 s con
  `-race` (fase F; su contexto se amplió a 600 s en la fase D).
- **Costos**: un solo costo por insumo, sin valuación por almacén; los ajustes de conteo usan el promedio vigente al aplicar
  (nada se revalúa a la hora del renglón); el costo teórico de un producto no incluye complementos ni componentes de combo
  y el de un complemento ligado a un producto no recorre los grupos ni componentes de ese producto; «Adicional para llevar»
  aparece aunque sea $0.00. Si un déficit pendiente de costo solo se cubre en parte con recepciones, el último costo válido
  sigue sin valor y un sobrante que cruce a positivo pide configurar el costo de referencia (`COST_REQUIRED`, que nombra
  solo el primer insumo sin costo).
- **Recetas y consumo**: la clasificación de una cancelación guarda solo su estado vigente (el historial está en la
  auditoría y en los movimientos compensatorios); las elaboraciones no se anulan ni se editan (una corrección es un
  ajuste); el trabajador no audita cada consumo; el historial de una porción muestra los últimos 20 cambios y el de una
  receta las últimas 50 versiones; en una receta nueva el selector de ingrediente no distingue los del producto hasta el
  primer guardado.
- **Indicaciones con efecto**: actúan solo sobre los ingredientes del producto, no sobre opciones de grupo cuyo complemento
  esté ligado a un ingrediente; un borrador con una indicación que después se desactiva o cambia de efecto se rechaza al
  cotizar (422) hasta quitarla; la pantalla de preparación no lista ingredientes.
- **Cancelaciones y conteos**: una cancelación clasificada «No preparado» o un ajuste autorizado **después** de aplicar un
  conteo, con hora anterior a la del conteo, cuentan la diferencia dos veces (se avisa al revisar el conteo; no se bloquea);
  una liberación confirmada en los milisegundos entre la comprobación de consumos pendientes y la aplicación de un conteo no
  se incluiría en esa aplicación; la cuenta de una cancelación para llevar con nombre del cliente se muestra como «Cuenta #N
  · Nombre»; la frecuencia mensual al final de mes sigue la normalización de fechas de Go (31 de marzo − 1 mes = 3 de
  marzo).
- **Etapa 9A**: la comprobación del código interno es informativa: dos
  terminales que den de alta a la vez el mismo código reciben la misma sugerencia y la segunda obtiene `DUPLICATE_CODE` al
  guardar. La sugerencia de insumos sigue el nombre solo en el alta y deja de seguirlo en cuanto se teclea en el código
  (como en productos). La sugerencia de presentaciones de producto solo conoce los códigos de productos y de las
  presentaciones del mismo producto (Go rechaza al guardar los de presentaciones de otros productos). La sección recordada
  de cada grupo se pierde al salir de Inventarios. Las compras de mostrador no tienen último precio ni proveedor preferido,
  y desactivar el proveedor de mostrador deshabilita la modalidad en la recepción directa.
- **Compras**: sin cuentas por pagar, notas de crédito ni devoluciones sin recepción (el reporte de compras por periodo
  llegó en la etapa 9A: ver [REPORTS.md](REPORTS.md)); una devolución no genera ingreso de caja ni saldo a favor; la recepción de una orden usa la equivalencia vigente
  de la presentación del renglón; no hay vista previa de totales antes de guardar un borrador; un borrador o un proveedor
  cuyo resultado quedó incierto y se recuperó podría volver a crearse desde el mismo formulario (los de recepción,
  devolución y vínculo sí se bloquean); recepciones, devoluciones, elaboraciones y existencias iniciales usan la hora del
  servidor (la interfaz no captura otra fecha); la búsqueda de gastos de la interfaz no filtra por caja (Go lo admite); el
  borrador desde sugerencias no convierte precios entre presentaciones (precio 0) y usa 16 % sin recepción previa.
- **Interfaz**: «Existencias negativas» del Resumen cuenta pares insumo-almacén, mientras que el filtro «Negativas» sin
  almacén usa la existencia total; la búsqueda de insumos consulta a Go en cada tecla. La hoja de conteo muestra 50 renglones
  a la vez y su búsqueda filtra en pantalla lo que devolvió Go (hasta 2000); «Otra unidad o presentación…» consulta un
  insumo por renglón; si un bloque del guardado del conteo falla, los anteriores quedan guardados (el aviso dice cuántos);
  las alertas se piden de 100 en 100 y se agrupan por página; «Abrir insumo» del aviso `COST_REQUIRED` abre el insumo solo si
  Go incluye su identificador en el error (si no, lleva a Insumos).

## Sistema anterior

El sistema anterior (C# / SQL Server, solo como referencia funcional) tenía artículos, unidades fijas sin conversiones,
existencia global, entradas con subtotal, IVA y total, ajustes con signo y recetas por producto sin versiones; no tenía
almacenes, costos, órdenes de compra, devoluciones, transferencias, conteos ni mínimos. **Corrección a la especificación**:
en el código disponible el descuento de inventario ocurría al registrar el **pago completo** de la cuenta (no al capturar);
AD-EL POS consume al liberar a cocina. Ignoraba personalizaciones y complementos, nunca reintegraba cancelaciones ni
registraba mermas y no relacionaba compras con caja.

Funciones del sistema anterior que **no se incluyeron** (reportadas sin implementarlas, `CLAUDE.md` §21): imágenes de
artículo; domicilio desglosado del proveedor (el reporte de compras por periodo, el segundo teléfono, el celular y la
página web llegaron en la 9A); RFC único y obligatorio; tasa de IVA de compra libre por renglón; bloqueo de ajustes que dejan existencia
negativa; tipo de artículo «Utensilios». El mobiliario y el equipo son control de activos y quedan fuera.
