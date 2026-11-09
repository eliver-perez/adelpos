# Etapa 6.2 · Avance

Especificación: [etapas/ETAPA_6_2.md](etapas/ETAPA_6_2.md). Rama local `etapa-6.2`, creada desde `main` (0.6.1, esquema 7). Sin migraciones ni rutas nuevas. Las pruebas se ejecutaron en una copia aislada del repositorio con un clúster PostgreSQL temporal (puerto 55499) y un Go temporal en 8183; la instalación real no se tocó. Sin commits: el trabajo queda en el árbol de trabajo para revisión del arquitecto (versión, `CLAUDE.md` y `docs/VERIFICATION.md` los actualiza él).

## Revisión del sistema anterior (CLAUDE.md §21)

Lectura de solo lectura de `PuntoDeVenta.cs` y `PuntoDeVenta.Designer.cs` (no se abrió `App.config` ni se copió código). El punto de venta mostraba las mesas como una cuadrícula de botones iguales de 190 × 90 (sin distribución ni colores de ocupación; solo la mesa elegida se resaltaba) y, a la derecha, un panel con la orden de la mesa: encabezados «Cant.», «Producto», «Total», los productos de **todas** las órdenes abiertas de la mesa separados por una franja con el nombre de cada orden, subtotal, IVA, total, descuentos, extra y «Total a pagar», y los botones Capturar orden, Modificar, Pagar, Cancelar, Imprimir y Dividir cuenta. Lo que se adoptó como referencia funcional: panel a la derecha sin abandonar el mapa, productos y totales a la vista y acciones grandes desde la mesa.

**Funciones o reglas del sistema anterior que no están en este plan** (se reportan; no se implementaron):

- El panel sumaba en una sola vista todas las órdenes abiertas de la mesa (con un total conjunto). Aquí cada cuenta se ve por separado, con sus botones.
- El panel desglosaba subtotal e IVA, descuentos y «extra» (cargo con descripción). Aquí el panel muestra total, pagado y saldo de Go; el desglose de IVA, descuentos y cargos vive en la precuenta y el cobro.
- Sin un corte (turno) iniciado no se podía capturar, modificar, pagar, dividir ni cancelar («Corte no iniciado»). Aquí el turno solo se exige para cobrar o devolver (etapa 6).
- «Capturar orden» en una mesa ocupada preguntaba «¿Desea agregar una nueva orden?» y creaba otra orden. Aquí «Capturar» entra a la cuenta existente y **+ Nueva cuenta** es una acción explícita.
- «Cancelar» anulaba la orden completa (con productos) previa autenticación de un administrador. Aquí se cancelan unidades con su permiso y motivo, y solo se anula una cuenta vacía.
- «Imprimir» elegía la orden y reimprimía la comanda de cocina o el ticket desde la mesa. Aquí la precuenta está en el panel y la reimpresión de comandas en **Más… → Preparación y comandas**.
- «Asignar mesa a pedido» convertía un pedido para llevar en orden de mesa. Aquí la modalidad de una cuenta es estable.
- Al terminar la captura, la mesa quedaba sin seleccionar. Aquí, al volver con **Mesas**, el panel de la misma mesa sigue abierto.
- Al iniciar, avisaba si la terminal no tenía impresoras de caja o de cocina asignadas.

## Cambios

### Go: nombre opcional al abrir (cambio mínimo)

`restaurant_accounts.name` sigue siendo `NOT NULL` con 1–100 caracteres; no hubo migración.

- `backend/internal/operations/account_names.go` (nuevo): referencia neutra de una cuenta sin nombre: en salón, el nombre de su mesa (o «Cuenta» sin mesa); en para llevar, «Pedido para llevar». `fillAccountName` la asigna cuando el nombre llega vacío o solo con espacios; `movedAccountName` hace que una cuenta identificada por la referencia neutra de su mesa tome la de la mesa nueva al cambiar de mesa (un nombre capturado se conserva).
- `backend/internal/operations/accounts_write.go`: `openAccount` y `updateAccount` llaman a `fillAccountName` antes de la validación existente; `updateAccount` y la acción `move` aplican `movedAccountName`. `validateAccountInput` no cambió: dividir sigue exigiendo el nombre de la cuenta resultante, la lista y el responsable se resuelven por omisión como antes y la programación conserva su validación (hora obligatoria solo si hay tipo de programación).
- Pruebas: `backend/internal/operations/quick_open_acceptance_test.go` (`testQuickOpen`, registrada al final de `TestRestaurantAcceptance`): apertura sin nombre en mesa («Mesa 7», lista predeterminada y responsable por omisión), sin campo nombre y sin mesa («Cuenta»), para llevar con espacios («Pedido para llevar» con su origen), nombre capturado recortado y conservado, validaciones que siguen rechazando (programación sin hora, mesa inexistente, 101 caracteres) sin dejar cuentas; edición que vacía el nombre (salón restaura «Mesa 8» y conserva observaciones; para llevar restaura «Pedido para llevar» con cliente y teléfono); cambio de mesa por `move` y por edición (la referencia sigue a la mesa; un nombre capturado se conserva); doble envío con el mismo `operation_id` (mismo resultado, una sola cuenta en la mesa) y reutilización con otro contenido (`IDEMPOTENCY_CONFLICT`, sin otra cuenta); división con nombre vacío rechazada sin alterar la cuenta.

### Escritorio

- `pages/restaurant/TablePanel.tsx` (nuevo): panel lateral de la mesa.
- `pages/restaurant/AccountDetails.tsx` (nuevo): «Datos de la cuenta» / «Datos del pedido»; sustituye a `AccountForm.tsx` (eliminado), que solo servía para abrir con formulario y para editar.
- `pages/restaurant/OperationalPage.tsx`: mapa + panel en Mesas, acciones de la cuenta en el lugar del mapa, «Nuevo pedido» y «Cuenta sin mesa» sin formulario, «+ Nueva cuenta» de la captura sin formulario.
- `hooks/useRestaurantWorkspace.ts`: la captura de Mesas se abre expresamente (como en Para llevar y Cuentas); `quickOpen` (apertura rápida con la operación existente y protección de doble toque); **Mesas** vuelve al mapa con el panel de la misma mesa.
- `pages/restaurant/capture/CaptureTopBar.tsx`: **Datos de la cuenta** / **Datos del pedido**, título que abre los datos, «Mesa para N», aviso «Pedido sin nombre».
- `pages/restaurant/capture/BillFooter.tsx`: `accountMoreActions` y `CheckoutButton` exportados para que el panel use el mismo menú «Más…» y el mismo botón de cobro.
- `services/restaurant.ts`: regla de nombres (`accountCustomName`, `takeawayCustomerName`, `accountLabel`) y orígenes de para llevar. `AccountList.tsx` y `AccountActions.tsx` la usan (la división propone «Separada de la cuenta #N» si la cuenta no tiene nombre).
- `styles.css`: panel lateral, totales en una fila y botones de la barra de la captura (tokens de tema existentes).
- Pruebas: `pages/restaurant/tablePanel.test.tsx` (nuevo), `capture/capture.test.tsx`, `e2e/restaurant.spec.ts` y `e2e/payments-flow.ts` (actualizados). `e2e/kitchen-flow.ts` no necesitó cambios: sus comprobaciones se conservan y siguen pasando.

## Cómo se usa

- **Mesas.** Tocar una mesa abre su panel a la derecha. Libre: **Abrir cuenta y capturar** entra directo a la captura. Ocupada: la cuenta (o las cuentas, como botones) con sus productos, total, pagado y saldo; **Capturar**, **Cobrar**, **Precuenta**, **Dividir**, **Más…** y **+ Nueva cuenta**. Tocar otra mesa cambia el panel; **Mesas** en la captura regresa al mapa con el panel abierto.
- **Captura.** **Datos de la cuenta** (o tocar «Mesa 1 · Cuenta #1») permite poner o quitar el nombre, observaciones y, con permiso, la lista de precios, sin salir de la captura.
- **Para llevar.** **Nuevo pedido** entra directo a la captura. **Datos del pedido**: cliente, teléfono, origen (Para llevar, Mostrador, Teléfono, Ventanilla u otro), pedido programado y observaciones. Si se envía a cocina sin nombre aparece «Pedido sin nombre · se identifica como Pedido #N»; no bloquea. El tablero de Para llevar funciona como antes.
- **Cuentas.** **Cuenta sin mesa** abre una cuenta de salón sin mesa y entra a capturar.

## Decisiones

- **Regla de «sin nombre».** La interfaz trata como sin nombre una cuenta cuyo nombre es la referencia neutra de su estado actual (nombre de su mesa, «Cuenta», «Pedido para llevar») o uno de los nombres que proponía el formulario anterior («Para llevar»). Esas cuentas se identifican como «Cuenta #folio» o «Pedido #folio»; un pedido para llevar se identifica por el cliente si lo tiene.
- **Cambio de mesa.** Para que una cuenta sin nombre no quede con el nombre de su mesa anterior, Go actualiza la referencia neutra al cambiar de mesa (acción `move` o edición). No es un cambio de validación; está cubierto por la prueba de aceptación.
- **Doble toque.** La apertura usa `useRestaurantMutation` sin clave: mientras está en vuelo o incierta, un segundo toque se ignora antes de generar otro `operation_id` (lectura sincrónica del almacén). Un resultado incierto se recupera con **Recuperar resultado** (mismo `operation_id`).
- **Origen por omisión** de un pedido nuevo: «Para llevar» (antes el formulario proponía «Mostrador»). Mostrador y Teléfono siguen disponibles y un origen distinto se conserva como «Otro origen».
- **Acciones desde el panel.** Dividir, precuenta, historial, cambiar mesa, transferir, combinar, responsable, anular y datos de la cuenta se abren en el lugar del mapa (el panel permanece); el cobro ocupa la vista completa como antes y al cerrarse vuelve al mapa con el panel.
- **Productos nuevos en el panel.** Se muestran con «Nuevo» y «Sin enviar», sin importe: la cotización de las partidas nuevas vive en la captura. El total, pagado y saldo son los de `payments` (lo enviado a cocina).
- **Lista de precios en «Datos del pedido».** El mismo panel ofrece la lista en para llevar con `orders.price_list.manage`, igual que en salón.
- **Sin «Precios con IVA incluido» en el panel** para dejar alto a los productos a 1280 × 720; la leyenda sigue en la captura, la precuenta y el ticket (el panel la conserva en la etiqueta accesible de los totales).

## Pendiente: número de comensales

No se implementó ni se simuló: `restaurant_accounts` no tiene dónde guardarlo y usar `notes` o `name` sería un abuso del dato. La barra de la captura y el panel muestran lo que existe, rotulado como capacidad: «Mesa para N».

Propuesta: migración `0008` con `ALTER TABLE app.restaurant_accounts ADD COLUMN guests smallint NULL CHECK (guests IS NULL OR guests BETWEEN 1 AND 99)` (NULL = sin capturar; las cuentas existentes quedan en NULL), campo `guests` opcional en `AccountInput` (abrir y editar, validado en Go, con auditoría y `restaurant_events` como el resto de los datos de la cuenta) y en `Account`; en «Datos de la cuenta», botones rápidos 1 a 8 y «Más» (teclado numérico de enteros hasta 99) con opción de quitarlo; la barra mostraría «4 personas» y, sin dato, «Mesa para N». Requiere autorización para aplicar la migración a la instalación real (respaldo y ensayo de restauración previos).

## Limitaciones

- Sin pantalla táctil física, ventana nativa WKWebView ni Keychain: la verificación es Vitest y Playwright (Chromium y WebKit) contra Go y PostgreSQL temporales.
- Si se renombra una mesa, las cuentas abiertas que guardan su nombre anterior como referencia lo muestran como nombre capturado (hasta que se vacíe el nombre o cambien de mesa).
- El tiempo abierta de la mesa es el de su cuenta abierta más antigua según el último sondeo (cada cinco segundos o por SSE).
- Al tocar una mesa, el panel muestra por omisión la primera cuenta abierta del listado de Go. Si la cuenta mostrada se liquida o se combina y la mesa conserva otras cuentas abiertas, el panel muestra sus botones y pide elegir una.
- Las acciones de cuenta abiertas desde el panel se presentan con sus formularios existentes en el área del mapa.
- `prettier --check` del repositorio marca `docs/etapas/ETAPA_7.md` (archivo sin seguimiento, ajeno a esta etapa y que no se modificó).

## Pruebas ejecutadas (copia aislada)

- Formato y metadatos: gofmt, `cargo fmt --check` y `sync-metadata --check` aprobados; Prettier aprobado en todo el árbol salvo `docs/etapas/ETAPA_7.md` (sin seguimiento, no modificado). `go vet` con y sin `integration` aprobado.
- `go test -race ./...`: aprobado en todos los paquetes.
- Integración PostgreSQL (clúster 55499): `database`, `administration`, `catalog`, `operations` (incluye `testQuickOpen`) y `payments` aprobados.
- Vitest: 140 pruebas en 24 archivos. Nuevas en `tablePanel.test.tsx` (8): mesa libre con «Abrir cuenta y capturar» sin formulario (cuerpo sin nombre, lista ni observaciones) y entrada directa a la captura; mesa ocupada con varias cuentas (botones, «En cocina»/«Nuevo», total/pagado/saldo del servidor simulado, acciones y «Más…», acción en el lugar del mapa, cambio de cuenta, «Capturar» y regreso al panel); otra mesa cambia el panel; «+ Nueva cuenta» desde el panel y desde la captura; doble toque con una sola petición y una sola cuenta; «Datos de la cuenta» (nombre opcional que vuelve a la referencia neutra, observaciones, lista, cuerpo con revisión y `operation_id`, título que abre los datos); lista oculta sin permiso; para llevar con «Nuevo pedido», «Datos del pedido», origen y aviso «Pedido sin nombre» que desaparece al poner el nombre. `capture.test.tsx` entra a la captura desde el panel y comprueba «Mesa para 4».
- `tsc`, build y `cargo test` (12 aprobadas, 1 ignorada) correctos.
- Playwright Chromium y WebKit aprobados: mesa libre → panel → «Abrir cuenta y capturar» con dos clics en el mismo instante → una sola cuenta en el servidor (`driver.request`) con nombre «Mesa 1» → productos → enviar a cocina (recorrido de la etapa 6.1 conservado: cocina, modificación, cancelación, historial, división); «Datos de la cuenta» vaciando el nombre y con observaciones; mesa ocupada → panel con dos cuentas, productos, saldo y acciones; «+ Nueva cuenta» (tres cuentas en el servidor) y anulación de la vacía desde «Más…» del panel; «Capturar» desde el panel y cobro de la etapa 6; para llevar con «Nuevo pedido» → captura directa → «Datos del pedido» (Ventanilla y entrega programada) → envío a cocina sin nombre con el aviso → nombre del cliente → entrega con cobro en pesos y dólares; teclado virtual en «Datos del pedido».
- Revisión visual: capturas `table-panel-free-*` (mesa libre) y `table-panel-*` (mesa ocupada con dos cuentas) a 1280 × 720 y 1920 × 1080 en tema claro y oscuro, en Chromium y WebKit, sin desplazamiento horizontal y con la acción principal del panel dentro de la ventana; también las capturas existentes de la captura táctil y del teclado.
