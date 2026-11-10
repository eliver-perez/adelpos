# Etapa 9A · Avance: ajustes pendientes, reportes, indicadores y exportaciones

Rama `etapa-9a`, creada desde `main` (`579e845`, versión 0.8.0, esquema 14, con 0010–0014 aplicadas en la instalación
real). La especificación del propietario es `docs/etapas/ETAPA_9A.md`, que no se modifica. Las migraciones nuevas (0015
en adelante) **no se aplican a la instalación real** sin autorización expresa del propietario (§30). Nada de la etapa
9B (identidad visual, logotipo, diseñador de documentos) se implementa aquí.

Las fases son las de la tabla de §0 (0, A, B, C y D); la frase final de la especificación («0 → E») se interpreta como
esa misma tabla.

## Fase 0 · Verificación contra el código

Revisión de solo lectura del repositorio y del sistema anterior (`../legacy-csharp/`, sin compilar, ejecutar ni leer
archivos de configuración). Ningún hallazgo obliga a cambiar una decisión del propietario: lo que el modelo actual no
resuelve se implementa respetando §0-B y queda anotado abajo. La etapa continúa sin detenerse.

### 1. Equivalencia por valor con piso y la opción de §3.1

- **Orden del cálculo** (`catalog/calculator.go`), para cada partida:
  1. base: producto, variante o lista de precios;
  2. extras de ingredientes;
  3. grupos de complementos;
  4. componentes de combo.

  Después, las promociones sobre toda la cotización. Los descuentos manuales de la etapa 6 se aplican en `payments`
  sobre el importe ya promocionado. El orden de §0-B (complementos → promoción → descuento manual) ya lo garantiza la
  estructura.

- **Fórmula del grupo** (`group_calculator.go`): en modo valor, `diferencia = max(0, Σ solicitado × valor de
referencia − Σ incluido × valor de referencia)`, con los extras explícitos aparte. En modo cantidad se cobran
  tramos.
- **La opción** («El precio puede bajar al cambiar complementos», desactivada por omisión):
  - **Dónde vive:** es una columna nueva del producto que el snapshot del pedido congela, y la cotización guarda el
    ajuste con signo.
  - **Desactivada:** el cálculo es exactamente el actual.
  - **Activada:** la diferencia con signo baja el núcleo del precio, nunca por debajo del valor de los complementos que
    permanecen ni de cero. Los extras y cargos independientes se suman después, sin descontarse.
  - **Promociones:** su base pasa a ser el precio ya ajustado, para no acreditar dos veces la misma diferencia.
  - **Combos:** el ajuste negativo de un componente solo aplica si el combo también tiene la opción.
  - **IVA:** está incluido en el precio, así que no cambia su regla.

### 2. Datos para las métricas de §6 y sus fechas

- **Venta comercial:**
  - unidades (`restaurant_item_units`, con su cuenta actual e importes);
  - versiones congeladas (`quote_line`, tasa de IVA);
  - ajustes (`account_adjustments`, con nombre histórico del motivo y asignaciones por unidad).

  El IVA se calcula por cuenta y tasa (`payments/finance.go`). Dividir, transferir o combinar solo cambia la cuenta de
  la unidad, así que cada unidad se cuenta una sola vez.

- **Cierre de una cuenta:** `restaurant_accounts.closed_at`. **Hallazgo:** una devolución reabre la cuenta y borra
  `closed_at`, y al volver a liquidarse cambia de fecha. Una cuenta reabierta cuyas unidades se cancelan queda abierta
  con saldo 0. Calcular la venta desde el estado actual movería o borraría la venta del día original, en contra de
  §0-B.

  **Solución:** un libro inmutable de reconocimiento de ventas que se escribe en la misma transacción en que la cuenta
  queda liquidada:
  - en el primer cierre registra la venta completa;
  - en una reliquidación registra solo las diferencias;
  - la devolución se reporta en su propio día.

  Las cuentas cerradas hoy se rellenan con la fecha de su primer cierre según la auditoría.

- **Cobros:** `payments` (método, caja, turno, usuario, dólares recibidos, tipo de cambio y cambio por moneda),
  `payment_refunds` y `cash_movements` (efecto físico por moneda).
- **Caja:**
  - `cash_shifts.business_date`, que es la fecha de apertura, sin hora de corte;
  - `cash_operations`;
  - cortes (`cash_shift_reports`) y cierres (`cash_shift_closures`, con diferencias por moneda).

  El esperado tiene un solo cálculo (`cash_expected.go`). Los reportes conservan la frontera de Caja y además excluyen
  los turnos en arqueo de todo agregado.

- **Inventario y compras:** movimientos con `occurred_at`, órdenes, recepciones (directas sin orden), devoluciones a
  proveedor y vínculos con gastos (que nunca son una segunda compra).
- **Fechas disponibles:**
  - confirmación (eventos de la cuenta);
  - cierre;
  - cobro y devolución (`created_at`);
  - liberación a cocina (`kitchen_unit_releases.released_at`);
  - movimiento (`occurred_at`);
  - recepción;
  - fecha comercial del turno.
- **Hora de inicio del día:** es `inventory_settings.operating_day_cutoff` (05:00), con la zona horaria del
  establecimiento. No se crea otra.

### 3. Costo de venta (§14)

Las partidas vendidas sí están enlazadas con su consumo: `inventory_movements.restaurant_unit_id`. El costo de una
unidad es −Σ del valor de sus movimientos `consumption` y `consumption_reversal`, sin contar los `cost_adjustment`, que
se reportan como concepto del periodo. Cada partida queda en uno de tres estados:

| Estado    | Cuándo                                                              |
| --------- | ------------------------------------------------------------------- |
| Estimado  | Algún consumo con costo pendiente o valorado al costo de referencia |
| Sin costo | Sin consumo registrado, o con solicitud pendiente o fallida         |
| Firme     | En cualquier otro caso                                              |

### 4. Interfaz, librerías de PDF/Excel y puente

- **Interfaz:** no hay librería de gráficos. Se crearán componentes SVG propios y ligeros, con tabla alternativa, y se
  reutilizan paginación, tarjetas de cifras, controles segmentados, campos de fecha y acordeones. El dashboard no se
  recalculará con cada evento SSE.
- **Librerías Go:**

  | Uso                           | Librería y versión                                                           | Licencia |
  | ----------------------------- | ---------------------------------------------------------------------------- | -------- |
  | Excel                         | `github.com/xuri/excelize/v2` v2.11.0                                        | BSD-3    |
  | PDF                           | `codeberg.org/go-pdf/fpdf` v0.12.0 (el repositorio de GitHub está archivado) | MIT      |
  | Fuentes Go, con acentos y «ñ» | `golang.org/x/image` v0.47.0                                                 | BSD-3    |

  Las tres funcionan sin red, se fijan en `go.sum` y se registran en `docs/DEPENDENCIES.md`.

- **Puente:** hoy solo transporta JSON. La exportación se hará con una operación cerrada nueva y el diálogo nativo de
  guardado de Tauri (`tauri-plugin-dialog` 2.8.3), usado solo desde Rust.
- **Infraestructura de consultas:** cada petición abre hoy una transacción de escritura, y las lecturas de restaurante
  y cobros toman el lock 7820148305 compartido. Los reportes autentican en una transacción corta y consultan en otra de
  solo lectura (`REPEATABLE READ READ ONLY`). Esa segunda transacción:
  - usa un número limitado de conexiones;
  - tiene límite de tiempo;
  - no toma ningún lock operativo.

### 5. Sistema anterior

- **Fecha de la venta:** solo contaba las cuentas liquidadas, pero **las agrupaba por la fecha de apertura**, no por la
  de cierre. Por eso, la afirmación de §0-B de que contaba la venta en el día de cierre no corresponde al código
  disponible.
- **Semana del año:** no era ISO 8601; en algunos años se adelantaba una semana y nunca había semana 53.
- **Totales:** solo mostraba subtotal, IVA y total en el reporte de entradas a inventario y en el detalle de una venta,
  no en el resumen de ventas.
- **Lo que no tenía:** propinas registradas, dólares físicos, devoluciones, costos, cierre ciego, exportaciones,
  gráficos ni dashboard.
- **Funciones que no están en el plan** (se reportan sin implementar):
  - reporte de modificaciones de cantidades autorizadas;
  - lista de cuentas anuladas con quién autorizó;
  - columna «Recibió» en compras;
  - folio del comprobante de tarjeta;
  - conteo de canceladas en el resumen.

Ganan las decisiones de §0-B (cierre como fecha de la venta y semana ISO).

### Decisiones menores tomadas en la verificación

1. Venta reconocida en el primer cierre mediante el libro inmutable de reconocimientos. Una cuenta reabierta que nunca
   vuelve a cerrarse conserva su reconocimiento y aparece en «En curso» como reabierta.
2. Ventas por turno: la venta pertenece al turno de la operación que cerró la cuenta; los cobros, al turno de cada
   pago.
3. Los descuentos se fechan con el reconocimiento de su venta, para que su porcentaje sobre ventas cuadre.
4. Caja: igual frontera que Caja y exclusión total de los turnos en arqueo.
5. No se reporta quién capturó: el borrador se borra al confirmar. Sí quién confirmó, quién es el responsable y quién
   cobró, descontó, autorizó, devolvió o registró movimientos de caja.
6. «Compras de mostrador» será un proveedor especial único, con el comercio concreto como referencia opcional de la
   recepción.
7. En Excel, el texto que podría tomarse como fórmula se escribe con un apóstrofo inicial, y el archivo no contiene
   fórmulas.
