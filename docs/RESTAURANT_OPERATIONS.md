# Operación del restaurante · Etapa 4

Versión 0.4.0, esquema 4. Continúa el catálogo y la seguridad existentes. `internal/operations` registra cuentas y pedidos; `internal/catalog` sigue siendo el único motor financiero.

## Recorrido de operación

1. En **Restaurante → Áreas y distribución**, crear áreas y mesas. El orden de área, forma, capacidad, dimensiones y posiciones persisten. Arrastrar o usar flechas en el diseñador y pulsar **Guardar distribución**; Shift mueve una unidad lógica.
2. En **Mesas**, tocar una mesa en el mapa abre su **panel lateral** a la derecha (ver [Flujo rápido](#flujo-rápido-de-mesas-y-para-llevar--etapa-62)). Desde la etapa 7.1, en una mesa libre **Capturar** entra a la captura sin crear todavía la cuenta (Go la crea con el primer producto, ver [Mesas · etapa 7.1](#mesas--etapa-71)); una ocupada muestra su cuenta y **Capturar** entra a la captura táctil. Una mesa admite varias cuentas (**Nueva cuenta**). **Cuentas** permite localizar cuentas de salón, incluidas las que no tienen mesa (**Cuenta sin mesa** abre una sin formulario).
3. Capturar en la pantalla táctil (ver [Captura táctil](#captura-táctil--etapa-61)): categoría a la izquierda, producto al centro (un toque lo agrega) y la cuenta a la derecha con el total calculado por Go. Los productos con elecciones obligatorias abren su personalización en el mismo panel central.
4. Los productos nuevos se guardan solos en el borrador persistido (etiqueta **Nuevo**). **Enviar a cocina** es la acción explícita que confirma: registra un lote con sus precios, promociones y configuración histórica. Agregar productos después crea otro lote al enviarlos.
5. **Modificar** una partida enviada abre la misma personalización en el panel central y exige motivo. **Cancelar unidades** permite retirar una cantidad entera, con motivo e historial. El mesero necesita los permisos correspondientes; no se exige un supervisor universal.

Una personalización abierta en el panel central se conserva al cambiar de cuenta o de vista interna: el aviso **Selección sin guardar conservada** permite retomarla o descartarla expresamente. **Volver** la descarta (pide confirmación si hubo cambios). Una recarga o cierre completo del proceso descarta esa selección aún no agregada; el borrador persistido permanece en PostgreSQL.

## Estados reales

| Dimensión            | Estados implementados                               | Transición                                                                                                                                                                                                                                         |
| -------------------- | --------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Cuenta               | `open`, `closed`, `merged`, `voided`                | Abrir; cerrar al liquidarse (salón sin borrador) o al entregarse liquidada (para llevar); reabrir si una devolución deja saldo; combinar hacia otra cuenta sin pagos; anular solo sin unidades activas, sin borradores y con cobros netos en cero. |
| Captura              | Borrador persistido                                 | Guardar/reemplazar mediante revisión; confirmar vacía el lote pendiente.                                                                                                                                                                           |
| Unidades             | `active`, `cancelled`                               | Cancelación explícita; no se eliminan sus identidades.                                                                                                                                                                                             |
| Preparación derivada | `without_items`, `draft` y estados reales de cocina | Depende de unidades activas y borrador; la etapa 5 informa liberación/tareas reales, sin confundirlas con papel o recepción KDS.                                                                                                                   |
| Pago                 | `not_recorded`, `partial`, `paid`                   | Derivado en Go (`payments`) de pagos, devoluciones, descuentos y cargos reales; ver [PAYMENTS_AND_REFUNDS.md](PAYMENTS_AND_REFUNDS.md).                                                                                                            |
| Entrega              | `not_delivered`, `delivered`                        | Solo pedidos para llevar liquidados, una vez por cuenta (`takeaway_deliveries`); independiente de la preparación.                                                                                                                                  |

La mesa es libre si no tiene cuentas abiertas; ocupada si tiene una o más. Desde la etapa 7.1, Go distingue además «Esperando pago» y «Pago parcial» (ver [Mesas · etapa 7.1](#mesas--etapa-71)). Fuera de servicio impide nuevas asignaciones, pero permite consultar cuentas existentes. No se desactiva una mesa o área con cuentas abiertas, ni (desde la etapa 7.1) se pone fuera de servicio una mesa con cuentas abiertas. Desactivar conserva referencias históricas. Anular una cuenta vacía, combinarla o cerrarla al liquidarse retira su ocupación.

## Precios históricos y cambios

El servidor vuelve a calcular al confirmar y compara la huella revisada por el operador. Si cambió catálogo, disponibilidad, lista o promoción, responde `QUOTE_CHANGED` y conserva el borrador. La huella excluye únicamente la hora de cálculo; incluye el contexto, las revisiones y el resultado comercial.

Cada lote guarda snapshot del catálogo, lista y cotización. Los cambios posteriores del catálogo no actualizan pedidos confirmados.

- Cambiar solo observaciones de preparación conserva los centavos históricos. Reducir cantidad sin cambiar receta conserva los importes de las unidades restantes y exige además permiso de cancelar.
- Cambiar presentación, receta/complementos o aumentar cantidad cotiza expresamente la parte afectada con el catálogo vigente. El operador revisa el resultado antes de **Confirmar modificación**. El resto de la cuenta permanece intacto.
- Las promociones automáticas se calculan por lote nuevo o modificación cotizada. Agregar otro lote no redistribuye descuentos de los anteriores.
- Las listas se seleccionan con `orders.price_list.manage`; sin selección se utiliza la predeterminada existente. No hay reglas automáticas de lista por modalidad todavía.

Los importes usan decimal y `numeric(18,2)`; JSON los transporta como cadenas. Cada unidad comercial tiene UUID, ordinal, subtotal, descuento y total exactos. Los centavos residuales se asignan determinísticamente a los primeros ordinales. Por ejemplo, $40 entre tres unidades mantiene $13.34, $13.33 y $13.33. Transferir conserva esos importes.

## División, combinación y transferencias

**Dividir cuenta** selecciona unidades enteras de una o más partidas y crea otra cuenta heredando mesa, modalidad, lista, cliente, programación y responsable. Se puede repetir para obtener más cuentas. **Transferir productos** mueve unidades a una cuenta existente. **Combinar cuentas** mueve todas las unidades activas y los borradores y marca el origen como combinado. La cuenta de origen y sus cancelaciones conservan historial.

La cuenta destino debe estar abierta y compartir modalidad y lista. Para llevar exige además el mismo cliente, teléfono y programación. Los destinos de mesa deben estar activos y disponibles. Cambiar responsable y cambiar mesa son acciones independientes con permisos propios.

Se mueven las mismas unidades, sin clonar identidades, recalcular descuentos ni crear otro pedido/preparación. Origen, destino, revisión, cantidades e historial se actualizan en una sola transacción. Las fracciones de una unidad comercial no se dividen; las cantidades decimales de ingredientes y porciones permanecen dentro de su personalización.

## Para llevar y ventanilla

**Para llevar** presenta listado izquierdo y detalle/cuenta derecho. Desde la etapa 6.2, **Nuevo pedido** abre el pedido y entra directo a la captura, sin formulario; el cliente, teléfono, hora programada, origen y observaciones se capturan después en **Datos del pedido**. El orden ascendente usa la creación del servidor; muestra hora, tiempo transcurrido, cliente/referencia, origen, importe confirmado y programación. Busca por folio, referencia, cliente o teléfono. La selección permanece mientras se actualiza el listado.

Mostrador, Teléfono y Ventanilla son sugerencias de origen de la misma modalidad `takeaway`. No se crean mesas ni un directorio de clientes. Preparación, pago y entrega se muestran separados. Advertencia y antigüedad alta usan umbrales configurables; por defecto 15 y 30 minutos. El sondeo actualiza la antigüedad cada cinco segundos.

La programación admite hora local de preparación o entrega prevista. Go la interpreta en la zona IANA configurada del establecimiento, conserva el instante UTC y devuelve la representación local. Rechaza horas locales inexistentes. En una hora ambigua por cambio estacional se utiliza la resolución de `time.ParseInLocation`; no hay selector de primera/segunda ocurrencia. La etapa 5 permite liberación manual o automática con política capturada por pedido y trabajador Go; no habilita entrega.

## Concurrencia y recuperación

Cada escritura lleva `operation_id` UUID y revisión del agregado. El resultado se guarda en `restaurant_operations` dentro de la misma transacción. Repetir actor, dispositivo, ruta y cuerpo devuelve el resultado original; reutilizar UUID con otro contenido da `IDEMPOTENCY_CONFLICT`. Una revisión obsoleta da `REVISION_CONFLICT`; nunca sobrescribe silenciosamente.

El cliente guarda el UUID/cuerpo en memoria mientras desconoce el resultado de una interrupción. **Recuperar resultado** reintenta esa misma operación y después consulta el estado actual. Bloquea otras escrituras hasta resolverla. Tras cerrar el proceso, consultar el estado del servidor antes de repetir manualmente una operación incierta; no hay cola offline persistente.

Lecturas y previsualizaciones usan lock compartido de operación; escrituras usan lock exclusivo transaccional. El catálogo tiene su lock separado. Esto prioriza consistencia y simplifica dos terminales; no se ha medido carga comercial elevada. El sondeo es de cinco segundos, se detiene cuando la vista está inactiva/oculta y descarta respuestas fuera de orden. Go revalida permisos, sesión y dispositivo en cada petición.

## Integración de cocina desde etapa 5

`restaurant_events` es un diario durable con secuencia, operación, cuenta relacionada, autor, terminal, instante y valores anterior/posterior. Confirmaciones, modificaciones, cancelaciones y transferencias guardan las identidades necesarias para interpretar sus cambios. Se conserva el estado legado del diario; `kitchen_event_receipts` registra su integración transaccional y `realtime_events` publica invalidaciones.

La etapa 5 implementa estaciones, rutas, liberaciones, documentos, cola persistente CUPS/ESC-POS y SSE autenticado con cursor/replay. La preparación y sus contratos KDS se describen en [KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md); no hay aplicación KDS independiente. Un traslado contable conserva las unidades y no genera otra preparación. Desde la etapa 8 la liberación a cocina es también el origen del consumo de inventario (ver [Inventario · etapa 8](#inventario--etapa-8)).

## Componentes visuales y límites

`Accordion` reemplaza los `details/summary` anteriores en horarios, listas/promociones, hijos de combo, desgloses, excepción de disponibilidad y auditoría. Usa botón de encabezado, resumen, chevron, aria, panel montado, `inert`, transición breve y `prefers-reduced-motion`. Revela errores nativos, errores indicados por prop y errores de formularios asincrónicos. Mantiene valores al cerrar.

`VirtualKeyboard` respeta `auto`, `always` y `disabled` por terminal. Incluye alfanumérico, numérico, acentos, selección, borrar y cierre; conserva teclado físico y excluye contraseñas. Reserva espacio y permite desplazar formularios por encima del teclado. Fecha/hora utiliza el control nativo del sistema.

El mapa usa un lienzo lógico de 1000×560 y desplazamiento interno. La captura limita cada lote a 20 partidas y 1000 unidades totales; una selección de traslado admite hasta 100 partidas. Listados de cuentas paginan hasta 100 por página; historial, 100 eventos por página. El catálogo conserva su límite de 5000 referencias. Las respuestas IPC siguen limitadas a 2 MiB: cuentas e historiales excepcionalmente grandes necesitan paginación de detalle antes de uso a gran escala. Snapshots completos por lote incrementan almacenamiento; falta política de retención y medición de rendimiento.

## Verificación reproducible

```bash
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
./scripts/dev.sh build-macos
```

La primera instalación de navegadores de prueba requiere `cd desktop` y `PLAYWRIGHT_BROWSERS_PATH="$PWD/../.cache/playwright" npx playwright install chromium webkit`. Playwright es dependencia de desarrollo fijada. El ejecutor crea una base exclusiva por navegador, ejecuta Go real, sirve el frontend compilado y sustituye únicamente el transporte IPC en Node; los secretos nunca llegan al navegador. No agrega un acceso web alternativo al producto. La compilación y los procesos nativos se verifican por separado; ver [VERIFICATION.md](VERIFICATION.md).

## Partidas con pago · etapa 6

Una unidad con asignación neta de pago (liquidada o con abono) o con descuento manual vigente no se transfiere, divide, combina, cancela, edita ni anula (`UNIT_HAS_PAYMENT`, `UNIT_HAS_DISCOUNT`); primero se registra la devolución correspondiente. Transferir o cancelar una cantidad toma primero las unidades libres. Las unidades sin pago siguen las reglas anteriores siempre que la cuenta no quede con total menor que lo pagado (`ACCOUNT_BELOW_PAYMENTS`). Agregar productos a una cuenta con abonos aumenta su saldo. La guarda vive en `operations/payment_guards.go` y consulta a `payments` dentro de la misma transacción y lock.

## Captura táctil · etapa 6.1

La pantalla de captura se rediseñó para pantalla táctil (fase D de la etapa 6.1). Es la misma para salón, cuentas sin mesa y para llevar, y usa los contratos existentes: borrador persistido, confirmación explícita con `expected_fingerprint`, `operation_id`, liberación a cocina y SSE. No hay rutas ni flujo de pedidos nuevos.

**Distribución.** Barra superior con **Mesas** (o **Para llevar**/**Cuentas**) para volver al mapa o al listado, mesa y cuenta en grande («Mesa 1 · Cuenta #1») con mesero, capacidad de la mesa («Mesa para 4», desde la etapa 6.2) y hora de apertura, **Datos de la cuenta** o **Datos del pedido** (etapa 6.2), las cuentas de la mesa como botones y **+ Nueva cuenta**; en un pedido para llevar muestra cliente, teléfono, hora programada y origen. Tres columnas fluidas: categorías (botones de 64 px con el color de la categoría, nombre y número de productos; la activa con borde de su color; orden visual; solo las activas, disponibles ahora según Go y con productos), productos (buscador por nombre o código, cuadrícula de botones de unos 110 px con franja del color de la categoría, nombre, precio de Go para la lista de la cuenta, imagen opcional y etiqueta **Opciones**) y la cuenta. **Mostrar imágenes** se recuerda por terminal en el almacenamiento local del WebView.

**Un toque agrega.** Un producto sin elecciones obligatorias se agrega con un toque. Si ya hay una partida **nueva** idéntica y sencilla (mismo producto y presentación, sin personalización ni indicaciones) se suma a su cantidad; si no, se crea otra partida. El botón ✎ de cada producto abre su personalización para crear una partida distinta (por ejemplo, «sin cebolla»). **Diferencia con el sistema anterior:** `CapturaOrden` creaba siempre una línea nueva con cantidad 1 por cada toque.

**Elecciones obligatorias.** Muestran **Opciones** y abren la personalización en el panel central: un grupo con mínimo de al menos una porción y más de una opción activa (guarnición, término…), un componente de combo obligatorio con más de un producto permitido, o un producto cuyo precio base no existe en la lista y tiene presentaciones. Go exige que la configuración original de cada grupo respete sus límites, así que lo incluido llega marcado y la selección de partida siempre es válida.

**Personalización en el panel central.** **Volver**, nombre y explicación breve; presentación, cada grupo y cada componente de combo como bloques con botones de 56 px (lo incluido seleccionado; el cargo extra configurado en el catálogo en el botón, por ejemplo «Aros de cebolla +$10.00»; la diferencia por equivalencia y los extras del grupo según la cotización de Go); **Sin …** para ingredientes removibles y **Extra …** para agregables; **Indicaciones para cocina** con las ya usadas en la cuenta como atajos y **Escribir otra** (texto libre con el teclado virtual), guardadas en las observaciones de la partida separadas por comas; cantidad con − y + grandes y **Agregar a la cuenta · $total**, con el total de la partida que devuelve `restaurant_draft_preview` sobre el borrador local vigente. Las reglas y la estructura de la selección son las de `ProductCustomizer` (`components/catalog/customizationRules.ts`); solo cambia la presentación.

**Cuenta.** Partidas enviadas con **En cocina** (y su estado de preparación), personalización en texto pequeño, total histórico, **Modificar** y **Cancelar unidades** (flujos y permisos existentes, con las reglas de partidas con pago de la etapa 6); partidas nuevas con **Nuevo**, − y + de 44 px y **Personalizar**. Totales: enviado a cocina, nuevo sin enviar y total grande con «Precios con IVA incluido»; con cobros o ajustes también lo pagado y el total enviado con descuentos y cargos. Acciones: **Dividir**, **Descuento**, **Precuenta** y **Más…** (datos de cuenta, historial, preparación y comandas, cambiar mesa, transferir, combinar, cambiar responsable y anular cuenta vacía), que sustituye a la lista desplegable «Acciones de cuenta…». Botones principales: **Enviar a cocina · N** (N = unidades nuevas; desactivado sin productos nuevos) y **Cobrar · $saldo**, que abre la pantalla de cobro de la etapa 6 (incluido el efectivo en dólares cuando la función está activa).

### Sincronización del borrador

- Cada toque (agregar, −, +, personalizar o quitar una partida nueva) modifica primero el estado local de la terminal, sin esperar la red, como una operación pendiente sobre el último borrador confirmado por Go.
- El borrador persistido se guarda con antirrebote: 700 ms después del último toque. Hay **una sola petición en vuelo** en toda la terminal; siempre envía el documento completo más reciente (borrador confirmado + operaciones pendientes) con la revisión en la que se basa (`PUT /accounts/{id}/draft`, `operation_id` nuevo). Los toques que llegan durante la petición se acumulan y se envían al terminar. Ningún toque se pierde ni genera una petición propia. Cambiar de cuenta o de vista envía de inmediato lo pendiente.
- Los importes de las partidas nuevas, «Nuevo sin enviar» y el total se muestran solo cuando la cotización de Go (`GET /accounts/{id}/preview`) corresponde exactamente al borrador visible; mientras tanto se muestra «…» o «Calculando…». React no calcula dinero.
- Un error de comunicación o del servidor deja el guardado **incierto** con el mecanismo existente (`useRestaurantMutation` y el almacén global de pendientes, clave `restaurant:draft`): la captura se pausa y **Recuperar resultado del borrador** reintenta con el mismo `operation_id` y cuerpo; después se envían los toques acumulados.
- Si otra terminal cambia la cuenta mientras hay toques sin guardar (o Go responde `ACCOUNT_CONFLICT`), el conflicto es explícito: los productos nuevos se conservan en la terminal y el operador elige **Conservar mis productos nuevos** (las mismas operaciones se aplican sobre el borrador actual, sin sobrescribir lo que hizo la otra terminal) o **Descartar mis cambios** (con confirmación). Sin toques pendientes, la cuenta nueva se adopta sin aviso.
- **Enviar a cocina** vacía primero la cola pendiente, obtiene la cotización vigente de Go y confirma (`restaurant_confirm`) con su revisión y huella. Si la cotización que se mostraba para esa misma revisión tenía otra huella, no confirma y pide revisar los importes. Ante `QUOTE_CHANGED` o un conflicto de revisión muestra un aviso y conserva los productos nuevos para revisarlos. Los toques hechos durante el envío se aplican después sobre la cuenta confirmada.
- Las acciones que escriben (dividir, transferir, cancelar, modificar, cobrar…) se abren después de sincronizar el borrador, para partir de la revisión vigente; mientras están abiertas, las partidas nuevas no se editan con − y +.
- La actualización en tiempo real (SSE y sondeo de cinco segundos) y el descarte de respuestas fuera de orden siguen como antes.

## Flujo rápido de mesas y para llevar · etapa 6.2

Sin migraciones ni rutas nuevas: reutiliza la apertura idempotente de `restaurant/accounts` (`operation_id`), la edición de cuenta (revisión, auditoría, `restaurant_events`), `useRestaurantMutation` y la captura táctil.

**Mapa con panel lateral.** En **Mesas**, el mapa operativo (el mismo dibujo de **Áreas y distribución**, sin controles de edición) ocupa la izquierda. Tocar una mesa abre a la derecha su panel, sin ventana emergente: nombre, estado (Libre, Ocupada, Fuera de servicio), capacidad («Mesa para N») y tiempo desde la apertura de su cuenta más antigua (calculado en Go); las cuentas abiertas como botones cuando hay más de una; para la cuenta elegida, sus productos (enviados con **En cocina** e importe de Go, nuevos sin enviar con **Nuevo**), observaciones y **Total**, **Pagado** y **Saldo** de `payments`. Botones: **Capturar** (entra a la captura de esa cuenta), **Cobrar · $saldo**, **Precuenta**, **Dividir** y **Más…** (el mismo menú de la captura: datos de cuenta, historial, preparación y comandas, cambiar mesa, transferir, combinar, cambiar responsable y anular cuenta vacía, cada uno con su permiso). Las acciones ocupan el lugar del mapa y al cerrarse vuelven a él; el panel permanece. Tocar otra mesa cambia el panel. Se actualiza con SSE y el sondeo de cinco segundos. **Mesas** en la barra de la captura vuelve al mapa con el panel de la misma mesa.

**Abrir sin formulario.** Mesa libre: **Abrir cuenta y capturar** abre la cuenta y entra directo a la captura (desde la etapa 7.1 el botón es **Capturar** y la cuenta se crea con el primer producto; ver [Mesas · etapa 7.1](#mesas--etapa-71)). Mesa ocupada: **+ Nueva cuenta** (en el panel y en la barra de la captura) abre otra cuenta en la misma mesa y entra a capturar (desde la etapa 7.1 el botón es **Nueva cuenta**, con el «+» como icono, y esa cuenta también se crea con el primer producto). Para llevar: **Nuevo pedido** con origen «Para llevar». Cuentas: **Cuenta sin mesa**. No se piden nombre, lista ni observaciones: Go asigna la lista predeterminada, el responsable (quien abre) y la referencia neutra del nombre. Un doble toque no abre dos cuentas: mientras la apertura está en vuelo o incierta, otro toque se ignora sin generar otro `operation_id`, y un reintento con el mismo `operation_id` devuelve la cuenta original.

**Nombre opcional (regla).** Go guarda como nombre de una cuenta sin nombre el de su mesa (o «Cuenta» sin mesa) y «Pedido para llevar» en para llevar; al editar, vaciar el nombre restaura esa referencia, y al cambiar de mesa la referencia sigue a la mesa nueva (un nombre capturado se conserva). La interfaz considera **sin nombre** una cuenta cuyo nombre es la referencia neutra de su estado actual o uno de los nombres que proponía el formulario anterior («Cuenta», «Para llevar»), y la identifica como «Cuenta #folio» o «Pedido #folio». Limitación: si se renombra una mesa, las cuentas abiertas con su nombre anterior lo muestran como nombre capturado.

**Datos de la cuenta durante la captura.** **Datos de la cuenta** (o tocar el título «Mesa 1 · Cuenta #1») abre en el área de categorías y productos, con la cuenta a la vista, el nombre opcional, las observaciones y la lista de precios (solo con `orders.price_list.manage`). En para llevar se titula **Datos del pedido**: nombre del cliente, teléfono, origen (Para llevar, Mostrador, Teléfono, Ventanilla u otro), pedido programado y observaciones. Guarda con `PATCH /accounts/{id}` (`accounts.manage`), con su revisión y aviso de cambios de otra terminal; la mesa y el responsable se cambian con sus acciones. La hora programada solo cambia si se edita aquí, por lo que guardar otros datos no sustituye una reprogramación de cocina (M3 sigue corregido). No hay número de comensales: la cuenta no tiene dónde guardarlo (pendiente, ver [ETAPA_6_2_AVANCE.md](ETAPA_6_2_AVANCE.md)).

**Pedido sin nombre.** El nombre del cliente se sugiere pero no bloquea. Si un pedido para llevar se envía a cocina sin nombre, la barra de la captura muestra el aviso discreto «Pedido sin nombre · se identifica como Pedido #folio». Go no exige nombre para enviar a cocina ni para programar; la hora sigue siendo obligatoria solo cuando hay tipo de programación.

## Mesas · etapa 7.1

Sin migración: la precuenta impresa ya se persistía en `payment_documents`. Contratos en [API_CONTRACTS.md](../API_CONTRACTS.md) («Restaurante»).

**Cuenta creada con el primer producto.** En una mesa libre, **Capturar** (panel de la mesa) y **Nueva cuenta** (en una mesa ocupada, desde el panel o la barra de la captura) entran a la captura con una cuenta pendiente local («Mesa 3 · Cuenta nueva»), sin llamar al servidor. El primer guardado del borrador (mismo antirrebote de 700 ms y una sola petición en vuelo) crea la cuenta y su borrador en una sola operación idempotente: `POST /restaurant/accounts` con `items` y `seen_account_ids`. Los guardados siguientes usan `PUT …/draft` sobre la cuenta creada; «Enviar a cocina», cobrar y las demás acciones siguen igual. Un resultado incierto de la creación queda en «Recuperar resultado del borrador» y se recupera con el mismo `operation_id` (almacén global de pendientes). Salir sin productos (o con toques que se anulan entre sí) no envía nada: la mesa sigue libre y no queda ningún registro. Mientras la cuenta es pendiente, la personalización cotiza con el motor del catálogo y la lista predeterminada (la que Go asignará); los datos de la cuenta, el descuento y el menú «Más…» aparecen al crearse.

**Dos terminales en la misma mesa.** La terminal envía las cuentas abiertas que veía en la mesa. Si al guardar el primer producto otra terminal ya abrió una cuenta, Go responde `TABLE_HAS_ACCOUNT` sin crear nada y la captura muestra «Esta mesa ya tiene una cuenta» con dos opciones (sin «Cancelar»; los productos se conservan en la terminal hasta decidir): **Agregar a la Cuenta #N** guarda los productos en el borrador de esa cuenta, con su revisión vigente y conservando su borrador; **Abrir una cuenta nueva** reenvía la creación sabiendo de esa cuenta, con otro `operation_id`. Lo mismo aplica a **Nueva cuenta** en una mesa ocupada si apareció una cuenta que la terminal no conocía.

**Para llevar y «Cuenta sin mesa» no cambian:** siguen abriendo la cuenta al instante (la especificación se limita a mesas; posible extensión futura). Desde la fase D de la etapa 9D también nacen con el primer producto (ver [Mesas, cuentas, Para llevar y cancelar cuenta · etapa 9D](#mesas-cuentas-para-llevar-y-cancelar-cuenta--etapa-9d)).

**Liberar mesa.** Para las cuentas vacías que ya existen, el panel de la mesa ofrece **Liberar mesa** (con confirmación estándar) solo si Go marca la cuenta como liberable (`releasable`) y la terminal no tiene productos nuevos sin guardar. Requiere `accounts.open`, sin motivo ni autorización adicional. Go exige que la cuenta esté abierta y vacía en sentido estricto (sin borrador, unidades en ningún estado, pedidos, combinación recibida, registros financieros ni de cocina), la deja anulada con el motivo fijo «Cuenta vacía liberada», la audita (`restaurant.account.released`) y emite su evento. Una cuenta con cualquier contenido responde `ACCOUNT_NOT_EMPTY` y sigue el flujo normal de anulación o cancelación; «Anular cuenta vacía» ya no se ofrece en «Más…» para una cuenta de mesa liberable.

**Panel de mesa libre.** Misma estructura que el de una mesa ocupada: encabezado con la mesa y su estado, área de cuenta vacía («Sin cuenta abierta»), totales en $0.00 y el botón grande **Capturar** en la misma posición; **Cobrar**, **Precuenta** y **Dividir** aparecen desactivados (con su permiso).

**Estados de mesa (Go).** El mapa muestra cinco estados calculados por Go en una sola consulta: **Libre** (sin cuentas; «Libre»), **Ocupada** (tiempo desde la apertura más antigua, «45 min»), **Esperando pago** (precuenta impresa posterior al último cambio de contenido y con saldo; icono de recibo y «Precuenta»), **Pago parcial** (abonos y saldo; «Saldo $X») y **Fuera de servicio** (candado). Una cuenta abierta sin productos cuenta como ocupada (es un registro real; el panel ofrece «Liberar mesa»). Con varias cuentas se muestra el más avanzado (esperando pago > pago parcial > ocupada). Cambiar productos, datos de la cuenta o ajustes después de la precuenta la invalida; un pago posterior no. Los colores son tokens del tema (`--table-*-bg/border/text`) en claro y oscuro, distinguibles también por luminosidad (fuera de servicio con rayas y borde punteado); la leyenda muestra los cinco estados y el panel usa los mismos tokens. Se actualiza por SSE con los eventos existentes (la precuenta impresa ya llegaba al ámbito `restaurant`) y por el sondeo de cinco segundos.

**Fuera de servicio desde la operación.** En el panel de una mesa libre, **Poner fuera de servicio**; en una fuera de servicio, **Habilitar**. Requieren `restaurant.tables.manage` (permiso existente «Administrar mesas y distribución»), se auditan y emiten evento. Go rechaza poner fuera de servicio una mesa con cuentas abiertas (`TABLE_HAS_OPEN_ACCOUNTS`), también desde el PATCH de **Áreas y distribución**.

Limitaciones: sin `orders.capture` ya no se puede ocupar una mesa con una cuenta vacía desde el panel (la cuenta nace con su primer producto); los toques de una cuenta pendiente aún no guardados se pierden si se cierra el proceso (como cualquier toque de la captura); la vigencia de la precuenta comparaba instantes de transacción, por lo que un cambio confirmado en el mismo instante que la impresión podía no invalidarla. **Etapa 9A (§33, migración 0015)**: la precuenta guarda la revisión de la cuenta y el número de ajustes, y solo está vigente si ambos siguen iguales (predicado único `precheck_current` de `payments.AccountStandingSQL`, probado con dos carreras controladas); las precuentas anteriores a la 0015 conservan la regla por marcas de tiempo. «Poner fuera de servicio» ocupa su propia fila del panel en cualquier tamaño de ventana. Ver [PAYMENTS_AND_REFUNDS.md](PAYMENTS_AND_REFUNDS.md).

## Inventario · etapa 8

Sin cambios en el ciclo de los pedidos ni en sus contratos: el inventario se integra a partir de la liberación canónica de cocina. Detalle en [INVENTORY_AND_PURCHASING.md](INVENTORY_AND_PURCHASING.md).

- **El consumo nace de la liberación**, no de la captura: guardar o previsualizar un borrador, abrir una cuenta (también la que se crea con el primer producto), confirmar un pedido que aún no se libera, dividir, transferir, combinar, cambiar de mesa, reimprimir y cobrar no consumen. Con la función «Control de inventario» activada, cada unidad liberada a cocina consume una sola vez (también un pedido programado confirmado antes de activarla y liberado después).
- **La operación no espera al inventario**: la liberación solo registra una solicitud por unidad y el trabajador de Go calcula el consumo después, en su propia transacción. Una falla del cálculo no afecta el pedido ni la cocina: se reintenta sola y aparece en Inventarios.
- **Modificar** una partida liberada que ya consumió registra el ajuste del consumo por la diferencia entre la versión nueva y la liberada; **cancelar** unidades liberadas con consumo (también reducir la cantidad) deja la cancelación «por clasificar» en Inventarios (Ajustes y mermas): «No preparado» revierte el consumo y «Preparado (merma)» lo conserva; sin clasificar, Go aplica «No preparado» al pasar el corte del día operativo. Cancelar antes de liberar no tiene efecto en el inventario.
- **Captura**: se usan las mismas selecciones estructuradas de siempre (el consumo es porciones elegidas × porción habitual); el mesero nunca captura gramos y las observaciones escritas no afectan el inventario. La etapa ajustó solo dos cosas de la captura: las **indicaciones rápidas con efecto** («Sin cebolla», «Queso extra»), que aplican la misma selección que el personalizador y guardan su identificador en `customization.quick_notes` (ver [CATALOG.md](CATALOG.md)), y las **tarjetas de complementos** de los grupos de varias porciones (un toque suma una porción, − en la propia tarjeta, «2 de 2», cambio de precio en vivo de Go y aviso al pasar del máximo). No se agregaron formularios administrativos al flujo de captura.
- **Para llevar y ventanilla**: los componentes de receta «solo para llevar» (bolsas, vasos, envases) se consumen solo en cuentas para llevar, incluida ventanilla; la modalidad se congela al liberar.

## Mesas, cuentas, Para llevar y cancelar cuenta · etapa 9D

Especificación: [etapas/ETAPA_9D_v2.md](etapas/ETAPA_9D_v2.md) (§0-B prevalece); avance en
[ETAPA_9D_AVANCE.md](ETAPA_9D_AVANCE.md); contratos en [API_CONTRACTS.md](../API_CONTRACTS.md) («Etapa 9D»). Migración
`0021_account_cancellation.sql` (esquema 21), **todavía sin aplicar en la instalación real**. No cambian las reglas de
negocio, los cálculos, la clasificación de cancelaciones de la etapa 8 (salvo la preclasificación como merma), los flujos
de cobro ni las acciones del panel de mesa; solo su presentación y lo que dice §0-B.

### Mesas (§3)

- **Panel siempre visible** de ancho fijo (360 px; 420 px desde 1600 px): elegir una mesa ya no cambia el tamaño del mapa.
  Sin mesa elegida muestra el **resumen del área**: cuántas mesas hay libres, ocupadas, esperando pago y con pago parcial
  (React cuenta las mesas por el estado que calcula Go); tocar un número resalta esas mesas con un contorno discontinuo
  (distinto de la selección; se quita al volver a tocarlo, al elegir una mesa o al cambiar de área) y, aparte, «N mesas
  fuera de servicio». Debajo, las acciones rápidas **Cuenta sin mesa** y **Nuevo pedido para llevar** y la ayuda que antes
  estaba sobre el mapa. Con mesa elegida, el panel de siempre; Escape lo cierra y vuelve al resumen.
- **Mapa**: a la altura del panel; al abrir un área o cambiar el tamaño de la ventana ajusta la escala para que se vean
  todas sus mesas, con la misma escala para todas (sin pasar del doble). El lado menor de cada mesa mide al menos 64 px;
  si así no caben, el mapa se desplaza arrastrando (el toque que termina un arrastre no elige la mesa) y con barras de
  desplazamiento siempre visibles. La leyenda va dentro del mapa, en una línea, y cada estado conserva su icono (Libre
  tiene ahora la palomita). El diseñador de **Áreas y distribución** conserva su lienzo proporcional de 1000 × 560.
- **Un solo botón principal**: **Capturar pedidos en esta mesa** (antes «Capturar») va relleno en azul a todo el ancho;
  **Cobrar · $saldo** conserva texto e importe con contorno; Precuenta, Dividir, Liberar mesa y Más… quedan como
  secundarios. En el detalle de Cuentas y Para llevar, **Capturar productos en esta cuenta** es el principal y «Enviar a
  cocina» y «Cobrar» van con contorno; dentro de la captura, «Enviar a cocina» y «Cobrar» siguen como principales (§4.6).
- **Tiempos**: menos de una hora «45 min», menos de un día «5 h 12 min» y desde 24 h «3 d 14 h», en mesas, cuentas, Para
  llevar, «Abierta hace…» y Cocina. **Cuenta abierta por mucho tiempo**: con la marca `long_open` de Go (más de
  `long_open_hours` horas desde la apertura; 12 por omisión, de 1 a 72, en Configuración → «Funciones del
  establecimiento», «Marcar una cuenta abierta después de (horas)», con `settings.features.manage`), la mesa muestra un
  punto ámbar y el tiempo en una etiqueta ámbar con triángulo, el panel «Abierta hace…» en ámbar y la tarjeta una franja
  ámbar. Solo es un aviso: no bloquea nada.
- **Sin «Actualizar»** en Mesas, Cuentas, Para llevar y la captura: los datos llegan por eventos y por el sondeo de
  siempre. **Reintentar** aparece solo cuando la barra indica «Sin conexión».

### Cuentas y Para llevar (§4)

- **Creación diferida** (§4.1 y decisión 2 del coordinador): **Nuevo pedido** (Para llevar) y **Cuenta sin mesa**
  (Mesas › Cuentas y el resumen de Mesas), **Nuevo pedido para llevar** del resumen, el menú nativo y la paleta entran a
  la captura **sin crear nada**, como la mesa desde la 7.1. La captura dice «Para llevar · Pedido nuevo» o «Cuenta nueva
  sin mesa»; el pedido o la cuenta nacen en Go al guardarse el primer producto (la apertura idempotente con `items` que Go
  ya admitía; para llevar con el origen «Para llevar» y sin mesa). Salir sin productos no deja nada guardado. Los datos
  del pedido (cliente, teléfono, programación) se capturan con «Datos del pedido» en cuanto el pedido existe, es decir,
  después del primer producto (la barra de la captura lo indica: «Después podrás capturar los datos del cliente»). Un
  rechazo de Go al crear se explica en la cuenta y los productos se conservan en la terminal. Si se sale de la captura
  mientras se guarda el primer producto y Go lo rechaza de forma definitiva (por ejemplo, un producto desactivado en otra
  terminal), un aviso de error lo explica fuera de la captura («El pedido nuevo para llevar y sus productos no se
  guardaron…»; igual para «Cuenta sin mesa» y la mesa de la 7.1), la cuenta pendiente se olvida y la pantalla queda en el
  listado o en el mapa; un resultado incierto sigue con «Recuperar resultado del borrador» y la misma operación. La
  apertura inmediata (`quickOpen`) se retiró.
- **Descartar cuentas vacías** (§4.2 y decisión 1): una cuenta que Go marca `discardable` (sin productos enviados,
  pagos, documentos ni registros de cocina, aunque tenga productos sin enviar) ofrece **Descartar cuenta vacía** (rojo)
  en lugar de «Cancelar cuenta», en «Más…» (panel de la mesa, captura, Cuentas y Para llevar), a la vista en el detalle
  de Cuentas y Para llevar y en el panel de la mesa cuando no es liberable; si la cuenta no tiene nada, sigue **Liberar
  mesa** de la 7.1 (entonces «Más…» del panel no repite «Descartar cuenta vacía»). Sin motivo ni supervisor (`accounts.open`; para llevar, `takeaway.manage`). La confirmación roja dice
  cuántos productos sin enviar se borran, que **descartar no es una cancelación** (no pide motivo ni aparece en los
  reportes) y si la mesa queda libre; «Conservar la cuenta» tiene el foco. Así se quitan las cuentas vacías que ya
  existían: en la instalación real, cuando reciba la 0.9.3, las #1 y #3 de mesa (solo con un borrador) y las #5, #8, #9
  y #10 para llevar. Antes de descartar se guardan los toques pendientes y se usa la revisión vigente; al terminar,
  «Cuenta #N descartada.» / «Pedido #N descartado.», se vuelve al mapa o al listado y el listado elige la siguiente.
- **Tarjetas compactas** (§4.3) en Para llevar y Mesas › Cuentas: renglón 1 con folio (y cliente), mesa o «Sin mesa»,
  tiempo e importe; renglón 2 con etiquetas de preparación, pago, saldo, entrega, unidades, programación y antigüedad.
  Sin la fecha de apertura, «Confirmado:» ni «unidades confirmadas». La preparación va en una etiqueta corta («Sin
  productos», «Borrador», «Sin confirmar en cocina», «En cocina», «Listo»…; el texto completo en su título) y un borrador
  dice «N sin enviar». Unos 60 px por tarjeta (antes ≈ 157 px): a 1280 × 720 caben cinco o más donde antes cabían dos y
  media. La selección usa el azul de la fase A.
- **Textos del detalle** (§4.4): «Pendiente de liberación · revisar cocina» pasa a **«Enviado · sin confirmar en
  cocina»**, una sola vez por producto y ya no en el encabezado (explicación en
  [KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md), etapa 9D). El **desglose** solo aparece si hay complementos
  elegidos, cargos o descuento, sin «Versión N» ni «Descuento $0.00». **Combos**: «Grupo: elección» solo cuando difieren;
  si lo elegido repite o contiene el nombre del grupo (sin distinguir mayúsculas ni acentos, por palabras completas),
  solo lo elegido («Papas a la francesa · Hamburguesa sencilla»). «Preparación y comandas» muestra cada advertencia
  distinta una vez, con «(×N)». En Para llevar, pago y entrega van en un renglón.
- **Menú por producto** (§4.5): **Modificar** y **Cancelar unidades** (rojo) ya no ocupan un renglón en cada producto
  enviado: se abren tocando el renglón o «⋯», debajo del producto, uno a la vez, y se cierran al elegir, con Escape o
  tocando fuera. Los productos nuevos sin enviar conservan − 1 + y «Personalizar»; «Enviado a cocina» / «Nuevo sin
  enviar» y «Cobrar · $importe enviado» no cambian (§4.6).
- **Pestañas de Mesas**: «Mesas / Cuentas / Áreas y distribución» miden 44 px (antes 38), sin desbordar a 1280 × 720.

### Cancelar cuenta (§5)

- **Dónde**: «Cancelar cuenta», en rojo, en el menú «Más…» de cualquier cuenta abierta no descartable, de mesa o para
  llevar (panel de la mesa, captura, Cuentas y Para llevar). Abre un panel en el lugar del mapa, del listado o del
  catálogo de la captura, con la cuenta a la vista. «Anular cuenta» ya no existe en la interfaz.
- **Qué muestra**: unidades enviadas e importe que se cancela (calculados por Go; React no suma), productos sin enviar que
  se descartan, el motivo con botones táctiles (solo los activos, con sus marcas «Exige descripción» y «Lo preparado
  cuenta como merma»), la descripción obligatoria de 5 a 500 caracteres cuando el motivo la exige («Otro»), qué pasa con
  lo enviado a cocina (lo no liberado se retira sin imprimir y las estaciones reciben el aviso de cancelación; con la
  marca de merma y «Control de inventario» activado, lo preparado queda como «Preparado / merma» corregible hasta el
  corte; sin ella, por clasificar), que **no cuenta como venta** y si la mesa queda libre.
- **Permiso y autorización**: con «Cancelar cuentas» (`accounts.cancel`, solo el administrador en una instalación
  existente) basta el propio permiso; sin él (`cancel_requires_authorization`), el panel pide desde el principio la
  autorización de un supervisor con el mismo componente de los descuentos. La contraseña sale del formulario al enviar y
  el almacén de operaciones pendientes no la guarda. Un supervisor sin «Cancelar cuentas», el mismo usuario o una
  contraseña incorrecta se rechazan en su formulario.
- **Bloqueos que explican**: con pagos o propinas (`cancel_blocker = payments`) la opción sigue habilitada con la
  aclaración «Primero registra la devolución»; con un descuento, cortesía o cargo vigente (`adjustments`), «Primero quita
  el descuento o cargo». El panel explica qué hacer y ofrece «Ir a devoluciones» o «Ver descuentos y cargos» si el
  usuario tiene el permiso (si no, dice qué permiso hace falta). Una cuenta que cambió en otra terminal pide «Revisé los
  cambios de esta cuenta» antes de enviar.
- **Confirmación y resultado**: `ConfirmDialog` rojo («¿Cancelar la cuenta #N?» / «¿Cancelar el pedido #N?», foco en
  «Conservar la cuenta»). Al terminar, aviso «Cuenta #N cancelada.», la mesa queda libre y se vuelve al mapa o al
  listado. Con un resultado incierto, «Reintentar sin duplicar» repite con el mismo `operation_id`; si la cancelación con
  supervisor no llegó a registrarse, el reintento sin contraseña vuelve con un error de autorización y se pide de nuevo.
- **Después**: el detalle de una cuenta cancelada muestra su registro (motivo, descripción, quién canceló, quién autorizó
  —«Con su propio permiso» si nadie—, unidades, importe y fecha); las anuladas antes de la 9D, las liberadas y las
  descartadas dicen «Cuenta cancelada sin productos activos o descartada vacía». El filtro de la lista «Anuladas sin
  productos activos» se llama «Canceladas o descartadas» (mismo estado `voided`). El historial detalla «Cuenta cancelada»
  (motivo, descripción, unidades e importe, quién canceló y «Autorizó: <nombre>», productos sin enviar descartados y la
  marca de merma) y «Cuenta vacía descartada» (no es cancelación). El Registro de movimientos muestra un nombre legible
  de las acciones nuevas. Reportes: «Cuentas canceladas» y «Cancelaciones por motivo» ([REPORTS.md](REPORTS.md)).
- **Reglas de Go** (resumen; detalle en `CLAUDE.md` §15, «Etapa 9D»): cualquier cuenta abierta no descartable sin pagos
  ni propinas netos ni ajustes vigentes, con o sin productos enviados; motivo activo del catálogo; un solo evento
  `account.cancelled` en la misma transacción que la auditoría, el registro `account_cancellations` y la mesa libre; nunca
  se reconoce como venta. Una cuenta descartable responde `409 ACCOUNT_DISCARDABLE` sin pedir motivo ni contraseña.
- **Motivos**: Administración → Cobros → «Motivos de cancelación», junto a «Motivos de descuento», con su mismo estilo
  ([PAYMENTS_AND_REFUNDS.md](PAYMENTS_AND_REFUNDS.md)).

### Limitaciones

- Una cuenta con unidades canceladas antes una por una se cancela con el importe que le queda activo: en «Cancelaciones
  por motivo» esas unidades no cuentan para el motivo de la cuenta (están en «Productos cancelados»).
- Un descuento, cortesía o cargo vigente se quita antes de cancelar (no en la misma operación).
- Sin verificar en hardware: pantalla táctil física (también el arrastre del mapa con el dedo, que se probó con el ratón
  de Playwright), ventana nativa e impresión real del aviso de cancelación.
- Con la creación diferida, «Datos del pedido» solo está disponible después del primer producto, y los toques de una
  cuenta pendiente aún no guardados (≤ 700 ms) se pierden si se cierra el proceso, como en la 7.1.
