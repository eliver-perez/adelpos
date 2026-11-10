# Etapa 7.1 · Avance por fases

Especificación: [etapas/ETAPA_7_1.md](etapas/ETAPA_7_1.md) (del propietario; no se modifica). Rama local `etapa-7.1`, creada desde `main` (`d9228b5`, versión 0.7.0, esquema 8). Al terminar la etapa la versión será 0.7.1; la única migración prevista es `0009_…sql` (fases B a D) y no se aplica a la instalación real sin autorización. Las pruebas se ejecutan en una copia aislada con un clúster PostgreSQL temporal (55499) y Go temporal (8183); la instalación real (`.local`, Keychain, servidor 8181, clúster 55432) no se toca.

## Fase A · Componentes compartidos

Sin migraciones ni cambios de contrato de la API. Los importes siguen llegando de Go como cadenas; el formato es solo de presentación.

### 1. Sistema único de mensajes

**Implementación.**

- Dependencia nueva **Sonner 2.0.8** (MIT, sin dependencias propias, peer React ^19), fijada de forma exacta y registrada en [DEPENDENCIES.md](DEPENDENCIES.md). No hubo conflicto con React 19 ni con Tauri: la CSP de la ventana (`style-src 'self' 'unsafe-inline'`) admite la hoja de posicionamiento que Sonner inserta al cargar, y Tauri 2.12.5 solo añade nonces a `style-src` cuando el HTML trae elementos `<style>` (el `index.html` compilado no tiene ninguno; comprobado en el código de Tauri y en `dist/index.html`, no en la ventana nativa).
- `services/notify.ts` es el único punto de entrada: `notifySuccess`, `notifyInfo`, `notifyWarning`, `notifyError(error, respaldo)`, `errorMessage` y `dismissNotice`. `components/AppToaster.tsx` se monta una sola vez en `App.tsx` (también en las pantallas de acceso); cada aviso es un `components/AppToast.tsx` propio, con Sonner en modo sin estilos.
- Presentación: arriba al centro, debajo de la cabecera (`--toast-offset-top`: 92 px, 64 px en modo operación), ancho de hasta 480 px, texto de 16 px, icono por tono, botón de cierre de 44 px («Cerrar aviso») y acción opcional de 44 px. Tonos con tokens nuevos en claro y oscuro (`--danger`, `--danger-soft`, `--info`, `--info-soft`, `--warning-soft`, `--toast-shadow`).
- Duraciones: éxito 4 s, información 5 s, advertencia 8 s; el **error no se cierra solo**. Un aviso con el mismo tono y texto (o el mismo `id`) sustituye al anterior en lugar de apilarse.
- Errores en español sin códigos internos: el texto es el de Go (o el respaldo en español si falta o tiene forma de código); el código queda solo como atributo oculto `data-error-code` para depuración.
- La pérdida de conexión con sesión abierta pasó del banner superior a un aviso persistente con `id` fijo que se retira solo cuando el servidor vuelve a responder; la cabecera sigue indicando «Sin conexión».
- En Vitest, `src/testing/toasterSetup.tsx` (`setupFiles` de `vite.config.ts`) monta la pila en cada prueba con DOM, como `App.tsx`, y la vacía al terminar.

**Criterio: qué es mensaje y qué queda como estado en pantalla.** El resultado de una acción del usuario (guardar, crear, enviar, cobrar, imprimir, cancelar, transferir…) es un aviso: éxito que se cierra solo o error persistente. Desaparecieron `notice-success`, `Notice success`, los banners superiores de `App.tsx` y los mensajes de acción en línea (`checkout-inline-note` con el error de la mutación, `retryNotice` de caja, avisos de guardado de `Form`, etc.). No conviven dos sistemas: `Notice` queda solo para estado en pantalla. Siguen en su lugar, porque describen la situación vigente y no deben desaparecer solos:

- validaciones de campo en línea antes de enviar;
- paneles persistentes de flujo: operaciones inciertas y su recuperación (`operation-recovery`, `UncertainOperation`, `PendingCashOperation`), conflicto del borrador (`capture-conflict`), selección conservada (`pending-selection`), conflicto de saldo del cobro (`checkout-conflict`), cuenta cambiada en otra terminal, movimientos posteriores al arqueo;
- errores al **cargar** un panel (`resource.error`, `list.error`, `polled.error`…);
- el resultado del cobro con el cambio a entregar y el de la devolución con el importe a entregar (además lanzan su aviso de éxito);
- avisos de estado que entrega Go: tope de caja, «Cierre en curso», mesa fuera de servicio, cuenta combinada, cerrada o anulada, advertencias de liberación a cocina, pedido para llevar sin nombre;
- el diagnóstico de `StatusPage` y el estado de las pantallas de acceso;
- en caja, el panel de resultado `RecordedOperation` con `PrintNotice`/`DrawerNotice` (lanza además un aviso con tipo, importe y concepto). La fase C rehará ese flujo (cerrar el panel y volver a Operación).

Un error con resultado incierto (sin conexión o ≥ 500) no lanza aviso: lo explica su panel de recuperación. Los errores que disparan un flujo propio (abrir turno, recargar la cuenta) conservan ese flujo.

**Avisos convertidos, por área.**

| Área                            | Avisos que ahora son toast                                                                                                                                                                                                                                                                                                                                                                                                                       |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Global (`App.tsx`, `Form`)      | Error al cerrar sesión; pérdida de conexión (persistente); éxito y error de todos los formularios `Form` («Cambios guardados en el servidor.» o el `successMessage` de cada pantalla; el alta continua del catálogo conserva ««X» quedó guardado. Captura el siguiente registro.»).                                                                                                                                                              |
| Mesas, captura y para llevar    | «Pedido enviado a cocina · N productos», cancelación de unidades, modificación confirmada, dividir, transferir, combinar, mover, cambiar responsable, anular, datos de la cuenta, distribución de mesas, áreas y mesas; errores de abrir cuenta, guardar, cotizar y enviar; recuperación de resultados.                                                                                                                                          |
| Cobro, ajustes y documentos     | Cobro («Cobro de $270.00 con Efectivo registrado.»), devolución, descuento («Descuento registrado: −$15.00.»), cortesía, cargo, cancelación de ajuste, corrección, entrega, apertura de turno (sin importes), tipo de cambio, precuenta enviada a la cola, reimpresión; todos los rechazos de Go y el rechazo de un reintento.                                                                                                                   |
| Caja                            | Ingreso, retiro, gasto y corrección («Retiro registrado: $200.00 · Resguardo»), corte parcial («Corte parcial #N generado.», sin cifras), arqueo iniciado y cancelado, turno cerrado (sin cifras), comentario del cierre, impresión y cajón manuales («enviado», nunca «impreso» ni «cajón abierto»), configuración; `CASH_INSUFFICIENT` como error persistente sin cifras; el reintento rechazado conserva su texto completo en la descripción. |
| Cobros (administración) y cajas | Métodos, políticas, IVA, rango del dólar y cajas físicas.                                                                                                                                                                                                                                                                                                                                                                                        |
| Catálogo                        | Guardar, alta continua, archivar/restaurar, activar/desactivar, disponibilidad, duplicar, orden de categorías, imagen cargada; errores al abrir o actuar.                                                                                                                                                                                                                                                                                        |
| Cocina                          | Avance de preparación, liberación, aviso de cambio, programación, copias, reintentos y decisiones de la cola («a la cola», nunca «impreso»), impresora guardada, impresión de prueba, estaciones, rutas y ajustes.                                                                                                                                                                                                                               |
| Usuarios, roles y configuración | Usuarios, excepciones, roles, funciones activables («Tope de efectivo en caja: activada.»), establecimiento, terminales (código, revocación, edición), sesiones, preferencias, teclado, contraseña, inicio de sesión, enrolamiento y asistente.                                                                                                                                                                                                  |

Ningún aviso de éxito incluye el efectivo esperado, diferencias ni otros datos protegidos: el cierre, el corte parcial y el arqueo se confirman sin cifras, y las pruebas de caja lo comprueban.

### 2. Diálogo de confirmación estándar

`components/ConfirmDialog.tsx` (`useConfirmDialog`) admite `tone: 'danger'` (borde e icono de advertencia, botón de confirmación rojo con token `--danger`), `message` como `ReactNode`, foco inicial en la opción segura, foco atrapado con Tab y Mayús+Tab, devolución del foco al cerrar, Escape como cancelar y botones de 56 px. Usos:

- **Nuevos:** cancelar unidades (`CancelItem`), devolución antes de ejecutarla (importe, propina, método, motivo y saldo resultante que calculó Go; la contraseña del supervisor se pide después de confirmar), confirmación de entrega de `TakeawayDelivery` (sustituye la confirmación en línea propia), descartar la selección conservada de la captura, descartar posiciones locales de la distribución de mesas y revocar una terminal (antes una confirmación en línea).
- **Existentes conservados**, con tono de peligro donde son destructivos o irreversibles: descartar cambios del catálogo («Volver al listado»), descartar la selección de la personalización, descartar mis cambios en el conflicto del borrador, cerrar el turno y cancelar el arqueo. «Iniciar arqueo» queda con tono normal porque se puede cancelar.
- No se añadieron en: cancelar un ajuste y corregir un pago (ya son un segundo paso explícito con motivo obligatorio), anular una cuenta vacía (pide motivo; «Liberar mesa» llega en la fase B con este componente) y revocar una sesión (no tenía confirmación).

### 3. Formato de importes

Una función por lenguaje con reglas idénticas y **la misma tabla de casos**: `backend/internal/moneytext/testdata/cases.json`, que leen las pruebas de Go y `desktop/src/services/money.test.ts`.

- Reglas: separador de miles «,» y punto decimal; al menos dos decimales, conservando sin redondear los que Go entregue de más (solo se quitan ceros finales tras el segundo); ceros a la izquierda normalizados; `$` en pesos y `US$` en dólares; el signo antes del símbolo («-$1,234.56» impreso con guion ASCII, «−$1,234.56» en pantalla con el menos tipográfico que ya se usaba); cero negativo sin signo; vacío o inválido → «—». Se opera sobre el texto, nunca con `number` (la tabla incluye cifras mayores que `Number.MAX_SAFE_INTEGER`).
- **TypeScript** (`services/money.ts`): `formatMoney(valor, moneda)`, `formatDollars`, `formatSignedMoney` (ajustes y diferencias: «+$50.00»), `formatDeduction` (importe positivo que se resta: «−$50.00») y `formatDenomination` (billetes y monedas: «$1,000», «$0.50»). `services/payments.ts` solo reexporta `formatMoney`/`formatDollars`.
- Se reemplazaron todos los «$» armados a mano: `QuoteBreakdown`, `ProductCustomizer`, `CatalogRecord`, captura (`BillLines`, `BillFooter`, `CaptureBill`, `ProductGrid`, `CustomizationPanel`, `CustomizationBlocks`, `TouchCustomizer`), `TablePanel`, `AccountList`, `AccountActions`, `HistoryDetails`, `CheckoutAccount`, `AdjustmentList`, `DiscountForm`, los botones rápidos de efectivo de `useCollectFlow` («$1,000», «US$5») y las etiquetas de denominaciones (`denominationLabel`: «Billetes de $1,000»). Solo quedan los símbolos fijos junto a los campos que se teclean (`AmountField`).
- **Go** (`internal/moneytext`, sin dependencias de módulos de la aplicación): `Format`, `FormatDecimal`, `FormatSigned`, `FormatDenomination`. Lo usan los tickets y precuentas (`payments/documents.go`), comprobantes y cortes (`payments/cash_documents.go`), `formatPesos`/`formatDollars` de los mensajes (`payments/money.go`; se quitó la copia de `shifts.go`), los mensajes de `collect.go` y `refunds.go`, y el importe de la comanda (`kitchen/template.go`). Los valores JSON no cambian.
- Cambios visibles en los impresos: miles en todos los importes («$1,250.00»); los descuentos pasan de «$-50.00» a «-$50.00»; el mensaje de devolución que superaba lo devolvible lleva ahora símbolo («($1,500.00)»).

### 4. Plurales

`services/plural.ts` (`countLabel`, `pluralWord`) y `backend/internal/plural` (`Count`, `Word`), con la tabla compartida `backend/internal/plural/testdata/cases.json`. Corregidos: cuentas del mapa de mesas (texto y nombre accesible), unidades y cuentas de `AccountList`, unidades disponibles de `AccountActions`, pedido enviado a cocina, registros del turno de caja («1 cobro · 0 devoluciones · 1 operación de efectivo · 1 corte parcial»), totales, cortes parciales, envíos de documentos, comentarios y regularizaciones del cierre, métodos activos, registros del catálogo, eventos de auditoría, pedidos y cambios del tablero de preparación, seleccionados en los editores comerciales, unidades incluidas del personalizador, pagadas del cobro y copias. En Go, la línea de regularizaciones del corte («1 regularización de un turno cerrado; no cambia este turno.»); los demás contadores impresos son etiqueta más número y no lo necesitan.

### 5. Iconos

Se continúa con el conjunto propio `components/Icon.tsx` (sin librería nueva): `IconName` exportado, 43 iconos SVG de 24 × 24 con trazo uniforme de 1.6, tamaño por prop, `aria-hidden` y `focusable="false"`. `IconLabel` coloca el icono de 18 px a la izquierda del texto sin cambiar la distribución del botón ni su nombre accesible. `PrinterIcon` y `DeliveryIcon` usan ahora el mismo conjunto; los métodos de pago siguen con `PaymentMethodIcon`.

- Mesas y para llevar: Abrir cuenta y capturar, Nueva cuenta, Capturar, Cobrar, Precuenta, Dividir, Más…, Nuevo pedido, Cuenta sin mesa, Actualizar, Volver y Recuperar resultado.
- Captura: Volver, Nueva cuenta, Actualizar, personalizar producto, Cobrar, Consultar pagos, Dividir, Descuento, Precuenta, Enviar a cocina, Más… (cada opción con icono), Modificar, Cancelar unidades y Agregar/Guardar/Confirmar.
- Cobro y documentos: Registrar pago, Volver, Precuenta, Descuentos y cargos, Dividir, Transferir, Corregir método, Devolver, Imprimir precuenta, Reimprimir, Marcar como entregado, Abrir turno, tipo de cambio, Aplicar descuento, Registrar cargo, Cancelar ajuste y Reintentar.
- Caja: Ingreso, Retiro, Gasto, Corte parcial, Arqueo y cierre, Corrección, Registrar, Volver, Imprimir/Reimprimir, Abrir cajón, Generar corte, Retirar efectivo, Confirmar cierre, Reintentar y Cerrar detalle.
- «+ Nueva cuenta» se llama ahora «Nueva cuenta»: el «+» es el icono.

### Decisiones menores

- La pila de avisos queda **apilada** (solo el más reciente se ve completo) en lugar de desplegada: desplegada cubría la barra de herramientas del cobro con tres avisos.
- **Sin pausa por «hover»**: en pantalla táctil el puntero se queda en el último toque; si un aviso aparecía ahí, Sonner desplegaba la pila y detenía el cierre automático (reproducido en WebKit). `AppToaster` detiene esos eventos del puntero antes de que lleguen a Sonner; cerrar y la acción siguen funcionando. Playwright lo comprueba en ambos motores.
- `formatMoney` completa siempre a dos decimales, también en las etiquetas que muestran lo tecleado: «Registrar ingreso $100.00», «Abrir turno con fondo de $500.00».
- `Form` admite `successMessage` y que `onSaveAndNew` devuelva el texto del aviso.
- `CancelItem` dejó de usar `Form` para pedir la confirmación de peligro antes de enviar; conserva la validación nativa del motivo.

### Limitaciones

- No verificado en la ventana nativa (WKWebView) ni en pantalla táctil física: los avisos y diálogos se comprobaron con Vitest (jsdom) y Playwright en Chromium y WebKit.
- La supresión de la pausa por «hover» depende de los eventos que hoy usa Sonner 2.0.8 (`mouseover`/`mouseout`/`mousemove` y `pointer*`); al actualizar Sonner hay que repetir la prueba de Playwright correspondiente.
- En los paneles basados en `Form`, un resultado incierto muestra además del panel de recuperación el aviso de error de `Form`, cuyo texto remite a «Recuperar resultado».
- El símbolo «—» de un importe inválido no existe en CP850; Go nunca lo genera en documentos (solo aparecería con un importe inválido).
- Los resultados de impresión y cajón dentro del panel de caja siguen en pantalla hasta la fase C.

### Verificación de la fase A

Repetida de forma independiente sobre la copia aislada, sincronizada con el repositorio:

- Estáticas: Prettier, gofmt, `cargo fmt`, `sync-metadata` y `go vet` (también con la etiqueta de integración) aprobados.
- Go unitarias: `ok` en todos los paquetes, incluidos `moneytext` y `plural`.
- Vitest: 36 archivos y 276 pruebas aprobadas; `tsc` y `build` sin errores; cargo: 14 aprobadas, 1 ignorada.
- Integración PostgreSQL en el clúster temporal: `database`, `administration`, `catalog`, `operations`, `payments` y `kitchen` en `ok`. La prueba de impresora física quedó omitida porque es opcional.
- Playwright: 1 aprobada en Chromium y 1 en WebKit.

## Fase B · Mesas

Sin migración (el esquema sigue en 8): la precuenta impresa ya se persistía en `payment_documents`, `restaurant_tables` ya tenía `out_of_service` y el permiso `restaurant.tables.manage` ya existía. Detalle funcional en [RESTAURANT_OPERATIONS.md](RESTAURANT_OPERATIONS.md) («Mesas · etapa 7.1») y contratos en [API_CONTRACTS.md](../API_CONTRACTS.md).

### Contratos nuevos o cambiados

| Contrato                                | Cambio                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `POST restaurant/accounts`              | Admite `items` (borrador inicial, mismas validaciones y límites que `PUT …/draft`) y `seen_account_ids`. Con `items` no vacío exige además `orders.capture`, `catalog.read` y `catalog.prices.read` (en `authorizeRoute`, antes de escribir). Cuenta (revisión 1) y borrador en la misma operación idempotente, con eventos `account.opened` y `draft.saved`. Con `seen_account_ids` y mesa: `409 TABLE_HAS_ACCOUNT` sin escribir nada si hay una cuenta abierta no vista, con `error.details.accounts: [{id, folio, name}]`. Sin el campo, comportamiento anterior. |
| `POST restaurant/accounts/{id}/release` | Nueva (`accounts.open`), `{operation_id, revision}`. Cuenta abierta y vacía en sentido estricto → `voided`, motivo fijo «Cuenta vacía liberada» en el evento `account.released`, auditoría `restaurant.account.released` y evento `restaurant.account.released`. Con contenido: `409 ACCOUNT_NOT_EMPTY`. Puente: `restaurant_release_account`.                                                                                                                                                                                                                       |
| Cuenta (`Account`)                      | Nuevo `releasable`, calculado por Go en cada lectura (solo se consulta la regla completa para cuentas abiertas sin unidades activas ni borrador).                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `POST restaurant/tables/{id}/service`   | Nueva (`restaurant.tables.manage`), `{operation_id, revision, out_of_service}`. Rechaza mesa desactivada o estado repetido (400), revisión obsoleta (409 `ACCOUNT_CONFLICT`) y fuera de servicio con cuentas abiertas (`409 TABLE_HAS_OPEN_ACCOUNTS`). Audita `restaurant.table.out_of_service` / `restaurant.table.enabled` y publica `restaurant.tables.changed`. Puente: `restaurant_table_service`.                                                                                                                                                              |
| `PATCH restaurant/tables/{id}`          | Ahora rechaza con `TABLE_HAS_OPEN_ACCOUNTS` pasar a fuera de servicio con cuentas abiertas (antes lo permitía).                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| Mesa (`/overview`, `/tables`)           | `state` con cinco valores (`free`, `occupied`, `awaiting_payment`, `partial_payment`, `out_of_service`), `occupied_since`, `occupied_seconds` y `balance`, en una sola consulta para todo el mapa (`tableQuery` con `CROSS JOIN LATERAL`, sin consultas por mesa ni por cuenta).                                                                                                                                                                                                                                                                                     |
| Errores                                 | `Fault` y `operations.Error` admiten `Details`; `writeFault` lo publica como `error.details` solo cuando existe. `APIError` de TypeScript expone `details`.                                                                                                                                                                                                                                                                                                                                                                                                          |

### Implementación

- **Go.** `operations/accounts_write.go` (apertura con borrador y `requireNoUnseenAccounts`), `operations/drafts.go` (`insertDraftItems` compartido), `operations/accounts_release.go` (regla estricta de cuenta vacía, `releasable` y liberación), `operations/table_service.go`, `operations/tables.go` (`tableQuery` con estados y guarda del PATCH), `operations/handler.go` (permisos de las rutas nuevas y de la apertura con productos), `payments/standing.go` (`AccountStandingSQL`, fórmula única de total, pagado neto, saldo, última precuenta y último ajuste; `HasAccountRecords`) y `kitchen/states.go` (`HasAccountRecords`). `payments.AccountBrief` usa ahora `AccountStandingSQL`, de modo que el saldo del mapa y el de cada cuenta salen de la misma fórmula. La regla de cuenta vacía consulta cada módulo por sus tablas: operaciones (borrador, unidades en cualquier estado, pedidos con `original_account_id`, combinación recibida), `payments` (pagos, devoluciones, ajustes aunque estén cancelados, autorizaciones, movimientos de caja, documentos, entregas) y `kitchen` (liberaciones y cambios pendientes).
- **Puente nativo.** `restaurant_release_account` y `restaurant_table_service` en la lista cerrada de `api.rs`, con el formato `format!` que lee `desktop/e2e/native-driver.ts`, y prueba Rust (`restaurant_routes_preserve_methods_and_reject_unimplemented_actions`: método, ruta, identificador obligatorio y rechazo de rutas arbitrarias).
- **Cuenta pendiente en la interfaz.** `DraftSyncEngine` (`capture/draftSync.ts`) conoce las cuentas pendientes (`preparePending`): el primer envío llama a `transport.create` (`POST restaurant/accounts` con `items` y `seen_account_ids`, desde `useDraftSync` con la misma clave global de pendientes `restaurant:draft`), y al confirmarse la entrada pasa al identificador real (`resolve`, `onAccountCreated`); los toques acumulados durante la creación se guardan después con `PUT …/draft`. Antirrebote, petición única en vuelo, reintento incierto con el mismo `operation_id` y conflicto de revisión siguen iguales. Toques que se anulan entre sí no crean nada. `TABLE_HAS_ACCOUNT` detiene la cuenta pendiente (`tableConflict`) hasta que el operador elige `addToAccount` (los toques se aplican sobre el borrador vigente de esa cuenta, con su revisión) u `openNewAccount` (reenvío con las cuentas vistas ampliadas y otro `operation_id`). `useRestaurantWorkspace` muestra una cuenta sintética (`pendingAccountBill`) mientras la mesa no tiene cuenta, sin consultar el servidor con su identificador local, y al crearse pasa la selección, la personalización abierta y el mapa a la cuenta real. La personalización de una cuenta pendiente cotiza con `catalog_preview` y la lista predeterminada.
- **Diálogo «Esta mesa ya tiene una cuenta».** `ChoiceDialog` nuevo en `components/ConfirmDialog.tsx`: mismo aspecto, foco atrapado y botones de 56 px que `ConfirmDialog`, una opción por cuenta informada («Agregar a la Cuenta #N») más «Abrir una cuenta nueva», sin «Cancelar» y con Escape sin efecto, porque ninguna opción descarta productos y el operador debe decidir. Se muestra en cualquier vista de la operación mientras exista la decisión pendiente.
- **Panel de la mesa.** Mesa libre con la misma estructura que una ocupada (encabezado con estado, «Sin cuenta abierta», totales en $0.00, «Capturar» como primera acción principal, Cobrar/Precuenta/Dividir desactivados). «Liberar mesa» (confirmación estándar y aviso) solo con `releasable`, `accounts.open` y sin productos nuevos sin guardar; «Anular cuenta vacía» deja de ofrecerse en «Más…» para una cuenta de mesa liberable. «Poner fuera de servicio» (mesa libre) y «Habilitar» (fuera de servicio) con `restaurant.tables.manage` y aviso.
- **Mapa.** Tokens `--table-{free,occupied,awaiting,partial,out}-*` en claro y oscuro con luminosidad decreciente de libre a pago parcial y fuera de servicio con rayas y borde punteado; indicador por estado («Libre», reloj + tiempo con `elapsedTime(occupied_seconds)`, recibo + «Precuenta», «Saldo $X» con `formatMoney`, candado + «Fuera de servicio»), número de cuentas cuando hay más de una, nombre accesible con estado, cuentas y detalle, y leyenda con los cinco estados. El panel usa los mismos tokens en su etiqueta de estado.

### Decisiones menores

- En el panel de la mesa libre, el botón grande se llama **Capturar** (§2.3: el mismo botón y lugar que en una mesa ocupada); sustituye a «Abrir cuenta y capturar», que ya no abre la cuenta.
- Sin `seen_account_ids` en el cuerpo, `POST restaurant/accounts` no comprueba la mesa: conserva el contrato anterior para para llevar, «Cuenta sin mesa», integraciones y la API de prueba.
- La creación con borrador valida las partidas con las mismas reglas que `PUT …/draft` (límites, productos, presentaciones y partidas de otra cuenta); como ese guardado, no cotiza: la cotización sigue en `preview` y la confirmación.
- La liberación se permite en cualquier modalidad en Go (la regla de cuenta vacía es la misma), pero la interfaz solo la ofrece en el panel de la mesa.
- `releasable` no depende de permisos; la interfaz exige además `accounts.open`.
- Fuera de servicio tiene prioridad sobre las cuentas abiertas al mostrar el estado (como antes): una mesa que quedó fuera de servicio con cuentas (registros anteriores a esta regla) se muestra «Fuera de servicio» y su panel conserva las cuentas y ofrece «Habilitar».
- `POST …/service` rechaza el estado repetido (400) en lugar de volver a escribirlo, y una mesa desactivada (se activa en Áreas y distribución).
- La mesa informa `occupied_seconds` calculado por el servidor además de `occupied_since`, para que el tiempo no dependa del reloj de la terminal.
- Liberar una cuenta en una mesa con otras cuentas confirma «las demás cuentas de la mesa no cambian» y avisa «Cuenta #N liberada» en lugar de «Mesa N quedó libre».
- La precuenta impresa ya llegaba al mapa: `payments.document.printed` se publica en el ámbito `restaurant` (salón) que recibe quien tiene `accounts.read`. No se cambió.
- El plazo de la aceptación encadenada de `operations` (`fixture_test.go`) pasó de 120 a 300 s: con las pruebas de la fase B la cadena completa con `-race` rozaba los dos minutos.

### Limitaciones

- Sin `orders.capture` ya no se puede ocupar una mesa con una cuenta vacía desde el panel: la cuenta nace con su primer producto.
- Los toques de una cuenta pendiente aún no guardados (≤ 700 ms) se pierden si se cierra el proceso, como cualquier toque de la captura; una creación incierta sí se conserva para recuperarla mientras la aplicación siga abierta.
- La vigencia de la precuenta compara instantes de inicio de transacción (`payment_documents.created_at` frente a `restaurant_accounts.updated_at` y los ajustes): un cambio que esperó el lock en el mismo instante en que se imprimía la precuenta podría no invalidarla. No se agregó migración para registrar la revisión de la cuenta en la precuenta.
- La regla de cuenta vacía consulta tablas sin índice por cuenta (`restaurant_orders.original_account_id`, `payment_authorizations`, `cash_movements`, `kitchen_pending_changes`, `merged_into_id`); solo se evalúa para cuentas abiertas sin unidades activas ni borrador, que son pocas. No medido con volumen real.
- «Poner fuera de servicio» ocupa tres líneas en el panel a 1280 × 720 (texto largo en la fila de acciones secundarias).
- No verificado en la ventana nativa (WKWebView) ni en pantalla táctil física.

### Pruebas de la fase B

- **Integración PostgreSQL** (`operations/tables_stage71_test.go`, encadenada al final de `TestRestaurantAcceptance`): salir sin capturar no deja registro (conteo de cuentas, borradores, operaciones, eventos, outbox, auditoría y pedidos tras las consultas de la captura); cuenta creada con el primer producto en una sola operación (eventos `account.opened` y `draft.saved` con el mismo `operation_id`), reintento idempotente sin otra cuenta, `IDEMPOTENCY_CONFLICT` con otro contenido, `PUT …/draft` y envío a cocina posteriores, y validaciones del borrador inicial sin escribir; dos terminales en la misma mesa libre (`TABLE_HAS_ACCOUNT` con la cuenta de la otra terminal y sin registros, «Agregar a esa cuenta» conservando su borrador y «Abrir una cuenta nueva»), dos aperturas simultáneas (una cuenta y un 409) y «Nueva cuenta» en mesa ocupada; liberar una cuenta vacía (mesero con `accounts.open`, idempotente, auditada una vez, motivo fijo, evento para el mapa y mesa libre) y rechazo `ACCOUNT_NOT_EMPTY` sin registros con borrador, unidades canceladas, pago, ajuste cancelado, precuenta, cambio de cocina pendiente y combinación recibida (y liberable de nuevo al vaciar el borrador); estados calculados (libre, ocupada con tiempo de la cuenta más antigua, esperando pago, invalidación por un cambio posterior y por un ajuste, pago parcial posterior que no la invalida, pago parcial, prioridad y saldo sumado con varias cuentas, igual al de cada cuenta, y mesa libre al liquidar); fuera de servicio por la operación (permiso, idempotencia, auditoría, evento, sin cuentas nuevas, revisión obsoleta, estado repetido, habilitar) y rechazo con cuenta abierta por la operación y por el PATCH; permisos antes de escribir (apertura con productos sin `orders.capture` denegada, auditada y sin registros; liberar sin `accounts.open`; servicio sin `restaurant.tables.manage`).
- **Vitest**: motor del borrador (creación con el primer guardado y paso a la cuenta real, toques que se anulan, `TABLE_HAS_ACCOUNT` con «abrir nueva» y «agregar», recuperación incierta de la creación); captura de una mesa sin cuenta (creación al primer guardado y `PUT …/draft` después, salir sin productos sin llamadas, cotización de la personalización con el catálogo, diálogo con «Agregar a la Cuenta #5» y con «Abrir una cuenta nueva»); panel de mesa libre con la estructura de una ocupada y «Capturar»; «Nueva cuenta» y doble toque sin crear cuentas; «Liberar mesa» solo con `releasable` y `accounts.open`, con confirmación; fuera de servicio y habilitar con su permiso; mapa y leyenda con los cinco estados (texto o icono, no solo color).
- **Playwright** (`e2e/restaurant.spec.ts`, Chromium y WebKit): leyenda con los cinco estados; panel de mesa libre en claro y oscuro a 1280 × 720 y 1920 × 1080; mesa libre → capturar → salir sin productos → mesa libre (sin cuentas en Go); doble toque en «Capturar»; mesa libre → producto → la cuenta se crea con el primer guardado → enviar a cocina (el recorrido existente continúa); «Nueva cuenta» sin productos no crea cuenta; «Liberar mesa» sobre una cuenta vacía creada con la API de prueba; precuenta impresa → la mesa muestra «Esperando pago» con el recibo y «Precuenta» (capturas en ambos temas).

### Verificación de la fase B

Ejecutada sobre la copia aislada sincronizada con el repositorio (clúster temporal 55499, Go temporal 8183):

- Estáticas (`tools/suite.sh`): Prettier, gofmt, `cargo fmt`, `sync-metadata --check`, `go vet` y `go vet -tags=integration` aprobados.
- Go unitarias con `-race`: `ok` en todos los paquetes.
- Vitest: 36 archivos y 290 pruebas aprobadas; `tsc` y `npm run build` sin errores; cargo: 14 aprobadas, 1 ignorada.
- Integración PostgreSQL (`tools/iter_snap.sh`): `database`, `administration`, `catalog`, `operations` (120,9 s), `payments` y `kitchen` en `ok`; la prueba de impresora física quedó omitida porque es opcional.
- Playwright: 1 aprobada en Chromium (46,2 s) y 1 en WebKit (45,1 s), repetida tras el último cambio de la interfaz.
- Repetida de forma independiente antes del commit, con los mismos resultados: estáticas aprobadas, Go unitarias en `ok`, Vitest con 290 pruebas aprobadas, `tsc`, `build` y cargo (14 aprobadas, 1 ignorada) sin errores, los seis paquetes de integración en `ok` (`operations` en 123,0 s) y Playwright con 1 aprobada en Chromium y 1 en WebKit.

## Fase C · Caja

Detalle funcional en [CASH_SHIFTS_AND_RECONCILIATION.md](CASH_SHIFTS_AND_RECONCILIATION.md) («Etapa 7.1 · fase C») y [KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md) («Cajón por CUPS en modo raw · etapa 7.1»); contratos en [API_CONTRACTS.md](../API_CONTRACTS.md).

### Migración 0009

Se creó `backend/migrations/0009_operation_tables_cash_and_reasons.sql`, la única de la etapa (la fase D agregará sus tablas al final). Es transaccional: el migrador la aplica dentro de la transacción que la registra y no contiene sentencias que no admitan transacción. **No se aplicó a la instalación real**; `config/product.json` sigue en esquema 8 hasta la fase E. 0001–0008 no cambiaron.

```sql
ALTER TABLE app.cash_drawer_events DROP CONSTRAINT cash_drawer_events_reason_check;
ALTER TABLE app.cash_drawer_events
    ADD CONSTRAINT cash_drawer_events_reason_check
        CHECK (reason IN ('payment', 'refund', 'operation', 'manual', 'test'));

CREATE INDEX audit_events_cash_outflow_rejected_idx
    ON app.audit_events (entity_id, occurred_at)
    WHERE action = 'cash.outflow_rejected';
```

Sin permisos nuevos: «Probar apertura» usa `cash.settings.manage` y la consulta de rechazos `cash.expected.read`. Prueba de actualización desde el esquema 8 en `database/existing_install_test.go` (`TestStage71UpgradeOfExistingInstallation`): turno abierto, cajón configurado y un comando manual existentes se conservan, el CHECK conserva su nombre y admite `test`, sigue rechazando motivos desconocidos y la apertura manual sin explicación, el índice es parcial y la migración es repetible.

### Contratos nuevos o cambiados

| Contrato                                     | Cambio                                                                                                                                                                                                                                                                    |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET payments/cash/drawers`                  | Cada impresora informa `connection` y `reason`; las colas CUPS activas se consultan con `lpstat -p` y `lpoptions -p` (solo lecturas) y son compatibles si admiten datos raw.                                                                                              |
| `PUT payments/cash/drawers/{caja}`           | Acepta una cola CUPS con datos raw. `DRAWER_QUEUE_NOT_RAW` (400, con el motivo) o `DRAWER_QUEUE_UNAVAILABLE` (409). Desactivar el cajón no consulta la cola. El agente sigue con `DRAWER_PROTOCOL_UNSUPPORTED`.                                                           |
| `POST payments/cash/drawers/{caja}/test`     | Nueva («Probar apertura», `cash.settings.manage`): pulso con la configuración guardada, `cash_drawer_events.reason='test'` ligado al turno activo, auditoría `cash.drawer_tested`, evento `cash.drawer.command_queued`, idempotente. Puente: `payments_test_cash_drawer`. |
| `GET payments/shifts/{id}/rejected-outflows` | Nueva (`cash.expected.read`): retiros y gastos rechazados por efectivo insuficiente, sin el esperado; vacía y `visible: false` durante el arqueo. Puente: `payments_shift_rejected_outflows`.                                                                             |
| Retiros y gastos (`CASH_INSUFFICIENT`)       | La respuesta no cambia (sin cifras). La operación se revierte y se confirma solo la auditoría `cash.outflow_rejected` (`success=false`).                                                                                                                                  |

### Implementación

- **Go · cajón por CUPS.** `kitchen/cups_raw.go`: `QueueInspector` y `HostQueueInspector` (ejecutor `commandRunner` sustituible en pruebas), `ParseQueueOptions` y `EvaluateRawQueue`. `kitchen/transport.go`: `HostTransport` con ejecutor opcional; `submitDrawerPulse` elige ESC/POS TCP o CUPS; `submitDrawerPulseCUPS` vuelve a consultar la cola y envía `/usr/bin/lp -d <cola> -o raw -n 1 -t ADEL-cajon-<trabajo> <archivo>`; `cupsJobReference` comparte la lectura del trabajo con los documentos. `CompatibleDrawerTransport` admite `escpos_tcp` y `cups`. `payments/cash_drawer.go`: compatibilidad por impresora con presupuesto de tiempo, `requireRawQueue`, `testDrawer` y el motivo `test`. `payments.Request.Queues` y `administration.Service.Queues` llevan el inspector (`main.go` usa el del equipo; la aceptación inyecta uno simulado).
- **Go · rechazos auditados.** `payments.Error` admite `Rejection`; `cash_operations.go` adjunta al `CASH_INSUFFICIENT` de retiros y gastos la auditoría (caja, turno, terminal, tipo, moneda, importe intentado, modo y autor de la autorización, `operation_id`). `administration/routes.go` la traduce en `Fault.Rejection` (`paymentFault`) y `runProtected` revierte el savepoint, escribe la auditoría con `success=false` en la transacción externa y la confirma, como el 403. `payments/cash_rejections.go` atiende la consulta con el índice parcial de la 0009.
- **Puente nativo.** `payments_test_cash_drawer` y `payments_shift_rejected_outflows` en la lista cerrada de `api.rs`, con el formato `format!` que lee `e2e/native-driver.ts` y prueba Rust (`cash_printing_drawer_and_limit_routes_are_closed`: método, ruta, identificador obligatorio y rechazo de rutas arbitrarias).
- **Atajo «Caja».** `pages/cash/CashShortcut.tsx`: `useCashShortcut` (una sola consulta de `payments_current_shift` y, con `cash.drawer.open`, de `payments_cash_registers`, más los eventos `cash`), `CashShortcut` (botón con estado y menú rápido), `CashShortcutSheet` (hoja modal con foco atrapado y Escape) y `NoShiftNotice`. `OperationalPage` lo crea una vez y lo coloca en la barra de mesas, para llevar, cuentas y cobro; `CaptureScreen` lo pasa a `CaptureTopBar` y el aviso sin turno a la columna de la cuenta. «Ir a Caja» usa la prop nueva `onOpenCash` de `App.tsx` (`setScreen('cash')`).
- **Tarjetas y flujo tras registrar.** `CashActions` dibuja `CashActionCard` (icono de 30 px en un círculo con el acento, texto debajo, `data-tone`, `aria-pressed`); tokens `--cash-entry`, `--cash-exit`, `--cash-report`, `--cash-closing` y sus versiones suaves en claro y oscuro. `pages/cash/recordingFlow.ts` (`useCashRecording`, `announceRecordedOperation`, `announcePartialCut`) anuncia el resultado desde la escritura del turno (también tras un reintento incierto) y cierra el panel o la hoja; `CashOperationForm` y `PartialCutPanel` ya no muestran ni anuncian el resultado y admiten `backLabel`; `DrawerControl` admite `initiallyOpen` y `onSent`; `useShiftOpening`/`ShiftOpening` admiten `onOpened` y confirman con el fondo (`openedMessage`). `trapFocus` de `ConfirmDialog` se exporta para la hoja.
- **Configuración del cajón.** `CashHardwareSettings`: conexión de cada impresora (`drawerConnectionLabel`), lista «Impresoras que no sirven para el cajón» con el motivo de Go y «Probar apertura» con confirmación estándar, deshabilitado sin impresora guardada o con cambios sin guardar.
- **Intentos rechazados.** `MovementsView`: opción «Intentos rechazados» del filtro de tipo solo con `cash.expected.read`; tabla con fecha, hora, tipo, importe intentado, usuario, terminal, caja y autorización (consulta solo al elegir el filtro).

### Decisiones menores

- **Criterio de datos raw conservador**: solo `usb`, `serial`, `parallel` y `socket`; IPP, IPPS, Bonjour, LPD y HTTP se rechazan aunque tengan controlador, porque reciben documentos. Una impresora de red debe usar ESC/POS por red.
- **La cola se consulta otra vez antes de cada pulso** (en el trabajador, fuera de la transacción) y no al encolar dentro de la transacción del cobro: así un cobro nunca espera a CUPS. Las consultas al configurar sí ocurren dentro de la transacción con el lock operativo, con un límite de 3 s por consulta y 4 s por petición.
- **Desactivar el cajón no consulta la cola**, para poder desactivarlo aunque CUPS no responda.
- **«Probar apertura» se permite desde cualquier terminal** con `cash.settings.manage` y aunque el cajón no esté activado (para comprobar la conexión antes de activarlo); la interfaz pide confirmación nombrando la caja y la impresora («Hazlo con alguien frente a la caja») y exige guardar antes los cambios.
- **Corte parcial desde el atajo** con `cash.partial_cuts.create` (en Caja la tarjeta sigue con `cash.shifts.read`, porque el panel también consulta cortes).
- **Abrir cajón en el atajo** solo si la caja de la terminal tiene el cajón activado (como en Caja); no depende del turno.
- **Un solo aviso por operación**: tipo, importe y concepto como mensaje; autorización, cola de impresión y cajón en la descripción. Lo que exige atención (sin impresora, falla al encolar, cajón no disponible o incierto) queda además como aviso persistente. En el corte la acción se llama «Imprimir corte».
- **Apertura de turno**: el aviso incluye el fondo declarado tal como lo devuelve Go («Turno de Caja 1 abierto con fondo de $500.00.»); antes no llevaba importes. No es el esperado.
- **Auditoría del rechazo** incluye además el modo de autorización y quién autorizó (útil para detectar el tanteo de un supervisor) y el `operation_id`; el usuario y la terminal son los del registro de auditoría.
- **Consulta de rechazos ciega durante el arqueo** (un importe rechazado acota el esperado), como el esperado mismo.
- **Aviso sin turno en la columna de la cuenta**, junto a «Cobrar», y no como franja sobre la captura: una franja reducía la altura de la pantalla y el menú «Más…» de la cuenta ya no cabía a 1280 × 720 (lo detectó Playwright).
- `e2e/notices.ts` (`expectNoticeLayout`) espera a que termine la animación de entrada antes de medir el aviso: medido al cerrarse la hoja, lo encontraba a medio camino.

### Limitaciones

- **Hardware pendiente**: el pulso por CUPS solo se verificó con el adaptador controlado de la cola y con un ejecutor simulado de `lp` (argumentos y contenido del archivo); no se ejecutó `lp` ni se envió nada a una impresora o cajón real. La prueba física queda pendiente hasta que el propietario tenga una impresora USB con cajón. La detección se comprobó en este Mac solo con lecturas (`lpstat`, `lpoptions`) sobre sus cuatro colas.
- Un controlador CUPS que se anuncie con un modelo genérico pero convierta los datos (filtros propios del fabricante con `-o raw` ignorado) no se detecta: el criterio se basa en la conexión y el modelo que informa CUPS.
- Las consultas a CUPS de la configuración retienen el lock operativo hasta 4 s si CUPS no responde.
- El tanteo por importes se detecta después (auditoría y filtro), no se impide.
- A 1280 × 720 el atajo ocupa espacio en la barra de la captura: el título y los detalles de la cuenta se recortan con puntos suspensivos antes. En el cobro va junto a las pestañas de vista para no agregar otra fila.
- Observado, anterior a esta fase: a 1280 × 720 el pie de la cuenta de la captura recorta a la derecha los importes y «Más…» (se comprobó igual con la rama sin la fase C, `58c461e`, en Chromium). No se corrigió aquí.
- Observado en las capturas de Playwright: un aviso de éxito anterior puede seguir visible detrás de los nuevos más de sus 4 s mientras la pila cambia. No se investigó (componente de la fase A).
- Sin verificar en la ventana nativa (WKWebView) ni en pantalla táctil física.

### Pruebas de la fase C

- **Integración PostgreSQL** (`operations/cash_stage71_test.go`, encadenada al final de `testCashPrinting`, con la consulta de colas simulada `controlledQueues` —evalúa atributos simulados con `kitchen.EvaluateRawQueue`, nunca ejecuta `lpstat`, `lpoptions` ni `lp`— y el adaptador controlado `recordingTransport`): C48 cola USB compatible, AirPrint incompatible con su motivo, cola sin respuesta, configuraciones rechazadas sin cambios ni auditoría, desactivar sin consultar la cola y cajón guardado con la cola USB; C49 pulso por CUPS con un cobro en efectivo (trabajo en la impresora CUPS, adaptador con el pulso del conector 5, una copia) y ninguno con tarjeta; C50 apertura manual por CUPS con permiso y rechazada sin él; C51 «Probar apertura» rechazada sin `cash.settings.manage` (403 auditado, sin comando), aceptada e idempotente con motivo `test`, turno, auditoría y evento, rechazada en una caja sin impresora y con una cola que dejó de admitir datos raw; C52 resultado incierto por CUPS sin reintento del trabajador ni reenvío desde la cola; C53 retiro rechazado sin cifras para el cajero, sin operación, sin resultado idempotente, sin comandos del cajón ni eventos, con su auditoría `success=false` (usuario, terminal, turno, caja, tipo, moneda, importe intentado, autorización) y sin el esperado, y gasto rechazado con su autorizador; C54 consulta rechazada sin `cash.expected.read` (cajero y observador), completa con el permiso y sin el esperado, y ciega durante el arqueo. C37 se actualizó: el agente sigue rechazado y una cola AirPrint simulada responde `DRAWER_QUEUE_NOT_RAW`. Se inyecta la consulta simulada en `fixture_test.go` para que ninguna prueba consulte CUPS.
- **Actualización desde el esquema 8** (`database/existing_install_test.go`, `TestStage71UpgradeOfExistingInstallation`).
- **Go unitarias** (`kitchen/cups_raw_test.go`): detección a partir de las salidas grabadas de `lpoptions -p` de las cuatro colas de este Mac (USB y AppSocket compatibles; IPP y AirPrint no) y de la salida de una cola desconocida; clases, colas remotas, IPP Everywhere, LPD e IPPS rechazadas y puerto serie aceptado; consultas de solo lectura con argumentos separados (`lpstat -p`, `lpoptions -p`) y nombre de cola inválido sin ejecutar nada; argumentos de `lp` (`-d cola -o raw -n 1 -t ADEL-cajon-<trabajo> <archivo>`) con un ejecutor simulado, contenido del archivo (bytes ESC p del conector) y archivo eliminado; resultados incierto, error y cola que ya no admite datos raw sin llamar a `lp`. `template_test.go` actualizado (CUPS compatible; el agente queda en intervención sin ejecutar programas).
- **Vitest** (`pages/cash/stage71.test.tsx`, `restaurant/capture/capture.test.tsx` y pruebas de caja actualizadas): tarjetas de Caja (icono grande, acento por tipo, selección con `aria-pressed` y sin casillas); cierre del panel tras registrar con un solo aviso, recarga del turno e «Imprimir comprobante» que llama a la impresión posterior (y sin el permiso no se ofrece); corte parcial con «Imprimir corte» y sin cifras; atajo con los tres estados, opciones según permisos, operaciones desactivadas en el arqueo, «Abrir turno» sin turno, «Ir a Caja», ingreso desde la hoja con su aviso y cierre, Escape sin registrar; aviso sin turno con y sin `cash.shifts.open` y ausente con turno; aviso en la captura dentro de la cuenta con la captura disponible; filtro «Intentos rechazados» solo con permiso y sin consultar sin él; configuración del cajón con la conexión de cada cola, el motivo de las incompatibles y «Probar apertura» confirmada (y deshabilitada con cambios sin guardar).
- **Prueba Rust** del puente: rutas nuevas con método, ruta, identificador obligatorio y rechazo de rutas arbitrarias.
- **Playwright** (`e2e/cash-flow.ts`, Chromium y WebKit): aviso sin turno en la captura con «Abrir turno» y el atajo «Caja · Sin turno» de 44 px o más; apertura con su aviso del fondo; ingreso de $100 desde el atajo de la captura (menú con sus opciones de 44 px o más, hoja con el formulario de Caja, capturas en claro y oscuro a 1280 × 720 y 1920 × 1080, hoja cerrada, aviso con «Imprimir comprobante» —no se pulsa: el destino no tiene hardware—, posición del aviso y cierre); «Ir a Caja»; tarjetas en ambos temas; retiro desde Caja con el panel cerrado y el aviso con la autorización; corte parcial con su aviso y consulta al reabrir; atajo «Caja · En arqueo» durante el arqueo. El resto del recorrido (cierre, cobro, precuenta, para llevar) sigue igual.

### Verificación de la fase C

Ejecutada sobre la copia aislada sincronizada con el repositorio (clúster temporal 55499, Go temporal 8183), después de `tools/format71.sh`:

- Estáticas (`tools/suite.sh`): Prettier, gofmt, `cargo fmt`, `sync-metadata --check`, `go vet` y `go vet -tags=integration` aprobados.
- Go unitarias con `-race`: `ok` en todos los paquetes, incluido `kitchen` con las pruebas de CUPS.
- Vitest: 37 archivos y 306 pruebas aprobadas; `tsc` y `npm run build` sin errores; cargo: 14 aprobadas, 1 ignorada.
- Integración PostgreSQL (`tools/iter_snap.sh`): `database`, `administration`, `catalog`, `operations` (121,0 s), `payments` y `kitchen` en `ok`; la prueba de impresora física quedó omitida porque es opcional. Una segunda ejecución detallada confirmó `TestStage71UpgradeOfExistingInstallation` y C37–C54 aprobadas.
- Playwright: 1 aprobada en Chromium y 1 en WebKit.
- Ningún trabajo ni pulso se envió a una impresora o cajón real: solo consultas de lectura `lpstat`/`lpoptions` en este Mac para investigar el criterio y grabar las salidas de prueba. La 0009 no se aplicó a la instalación real.
- Repetida de forma independiente antes del commit, con los mismos resultados: estáticas aprobadas, Go unitarias en `ok`, Vitest con 306 pruebas aprobadas, `tsc`, `build` y cargo (14 aprobadas, 1 ignorada) sin errores, los seis paquetes de integración en `ok` (`operations` en 125,8 s) y Playwright con 1 aprobada en Chromium y 1 en WebKit.

## Fase D · Pendientes acordados

Detalle funcional en [PAYMENTS_AND_REFUNDS.md](PAYMENTS_AND_REFUNDS.md) («Motivos de descuento · etapa 7.1» y «Precuentas y tickets»), [CATALOG.md](CATALOG.md) («Indicaciones rápidas por categoría · etapa 7.1») y [KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md); contratos en [API_CONTRACTS.md](../API_CONTRACTS.md). Sin permisos nuevos: los motivos usan `payments.read` y `payments.settings.manage`, y las indicaciones `catalog.read` y `catalog.categories.manage`.

### Migración 0009 (ampliada)

La fase D agregó tres secciones al final de `0009_operation_tables_cash_and_reasons.sql`, sin tocar las dos de la fase C. Sigue siendo una sola migración, transaccional y repetible (el migrador la registra en la misma transacción; una segunda ejecución aplica 0). **No se aplicó a la instalación real**; `config/product.json` sigue en esquema 8 hasta la fase E. 0001–0008 no cambiaron.

```sql
-- 3. Motivos de descuento: catálogo sembrado por decisión del propietario.
CREATE TABLE app.discount_reasons (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CONSTRAINT discount_reasons_name_check
        CHECK (length(name) BETWEEN 1 AND 60 AND name = btrim(name)),
    requires_description boolean NOT NULL DEFAULT false,
    active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0 CONSTRAINT discount_reasons_sort_order_check CHECK (sort_order >= 0),
    revision integer NOT NULL DEFAULT 1 CONSTRAINT discount_reasons_revision_check CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT discount_reasons_name_key UNIQUE (name)
);
INSERT INTO app.discount_reasons(name, requires_description, sort_order) VALUES
    ('Atención', false, 10), ('Promoción', false, 20), ('Demora', false, 30),
    ('Cortesía de la casa', false, 40), ('Otro', true, 50);

-- 4. Motivo de cada descuento o cortesía y su nombre al registrar; las filas existentes quedan en NULL.
ALTER TABLE app.account_adjustments
    ADD COLUMN reason_id uuid CONSTRAINT account_adjustments_reason_id_fkey REFERENCES app.discount_reasons(id),
    ADD COLUMN reason_name text;
ALTER TABLE app.account_adjustments
    ADD CONSTRAINT account_adjustments_reason_kind_check CHECK (reason_id IS NULL OR kind = 'discount');
ALTER TABLE app.account_adjustments
    ADD CONSTRAINT account_adjustments_reason_name_check CHECK (
        (reason_id IS NULL AND reason_name IS NULL)
        OR (reason_id IS NOT NULL AND reason_name IS NOT NULL AND length(reason_name) BETWEEN 1 AND 60)
    );
CREATE INDEX account_adjustments_reason_idx ON app.account_adjustments (reason_id) WHERE reason_id IS NOT NULL;

-- 5. Indicaciones rápidas por categoría: texto sin comas, orden, activa y revisión.
CREATE TABLE app.catalog_category_quick_notes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id uuid NOT NULL
        CONSTRAINT catalog_category_quick_notes_category_id_fkey REFERENCES app.catalog_categories(id),
    text text NOT NULL CONSTRAINT catalog_category_quick_notes_text_check
        CHECK (length(text) BETWEEN 1 AND 80 AND text = btrim(text) AND strpos(text, ',') = 0),
    active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0
        CONSTRAINT catalog_category_quick_notes_sort_order_check CHECK (sort_order >= 0),
    revision integer NOT NULL DEFAULT 1 CONSTRAINT catalog_category_quick_notes_revision_check CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX catalog_category_quick_notes_text_key
    ON app.catalog_category_quick_notes (category_id, lower(text));
```

Todas las restricciones nuevas tienen nombre explícito; las de columnas opcionales (`account_adjustments_reason_kind_check` y `account_adjustments_reason_name_check`) exigen `IS NULL` en la rama que las omite y nunca evalúan a NULL (las demás columnas que validan son `NOT NULL`). La prueba de actualización desde el esquema 8 (`TestStage71UpgradeOfExistingInstallation`, ampliada con `checkStage71PhaseD`) parte de una instalación con un descuento de la etapa 6 y comprueba: el descuento conserva motivo escrito e importe con `reason_id` nulo, la siembra (nombres, orden, solo «Otro» con descripción, todos activos), las once restricciones con nombre y el índice único, los CHECK con tablas temporales (nombres vacíos, con espacios o de más de 60, orden negativo, textos con coma, vacíos, con espacio final o de más de 81, revisión 0, un cargo con motivo, un motivo sin nombre guardado, un nombre sin motivo y un nombre vacío, rechazados; un descuento con y sin motivo y un cargo sin motivo, aceptados), la llave foránea de la categoría y del motivo y el índice que rechaza «SIN CEBOLLA» si ya existe «Sin cebolla». La migración sigue siendo repetible.

### Contratos nuevos o cambiados

| Contrato                                          | Cambio                                                                                                                                                                                                                                                                                                                                                                                                               |
| ------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET payments/discount-reasons`                   | Nueva (`payments.read`): todos los motivos en su orden visual, con `requires_description` y `active`. Puente: `payments_discount_reasons`.                                                                                                                                                                                                                                                                           |
| `POST` / `PATCH payments/discount-reasons[/{id}]` | Nuevas (`payments.settings.manage`): `{operation_id, revision?, name, requires_description, active, position?}`; idempotentes, auditadas (`payments.discount_reason_created`/`updated`/`discount_reasons_reordered`) con evento `payments.discount_reasons.changed`; `DUPLICATE_NAME`, `REVISION_CONFLICT` y `LAST_ACTIVE_REASON` (409). Puente: `payments_create_discount_reason`, `payments_save_discount_reason`. |
| `POST payments/accounts/{id}/discounts`           | `reason_id` opcional. Con él: motivo existente y activo (`DISCOUNT_REASON_INACTIVE`), descripción obligatoria de 5 a 500 si el motivo la exige y opcional (≤ 500) si no. Sin él: regla de la etapa 6 y `reason_id` nulo.                                                                                                                                                                                             |
| `finance.adjustments[]`                           | Agrega `reason_id` y `reason_name`.                                                                                                                                                                                                                                                                                                                                                                                  |
| Precuenta y ticket (`CommercialDocument`)         | Cada ajuste agrega `reason` («Concepto · Motivo» en el texto); con `usd_cash` y tipo de cambio, `usd_total`, `usd_rate` y la línea «Total en dólares (T.C. $Y)» con `US$X`.                                                                                                                                                                                                                                          |
| `POST` / `PATCH catalog/quick-notes[/{id}]`       | Nuevas (`catalog.categories.manage`): `{category_id, text, active, revision, position?}`; revisión optimista, orden por categoría, auditoría `catalog.quick-notes.created`/`updated`/`reordered`; `CATALOG_VALIDATION` (400), `DUPLICATE_QUICK_NOTE` y `REVISION_CONFLICT` (409). Puente: `catalog_create_quick_note`, `catalog_save_quick_note`.                                                                    |
| `GET catalog/lookups`                             | Agrega `category_quick_notes` (todas, con `active`, en el orden de categorías e indicaciones).                                                                                                                                                                                                                                                                                                                       |

### Implementación

- **Go · motivos.** `payments/discount_reasons.go`: lista, alta y edición con `internal/ordering` (alcance nuevo `DiscountReasons`), revisión, idempotencia en `restaurant_operations` (la de todas las escrituras de `payments`), lock 7820148305, auditoría y evento en la misma transacción, y `resolveDiscountReason`, que aplica las reglas de la decisión 15 antes de la primera escritura. `payments/adjustments.go` guarda `reason_id` y audita el motivo junto al plan (`discountAudit`); `payments/finance.go` lee `reason_id` y el nombre guardado con el descuento (`reason_name`).
- **Go · total en dólares.** `payments/documents.go`: `addDollarTotal` (función `usd_cash`, turno activo de la caja de la terminal para la precuenta y turno del pago para el ticket, `mxnToUSD` sobre el total que ya calculó Go) y `renderDocument` con «Concepto · Motivo» y la línea «Total en dólares (T.C. $Y)» con `US$X` en el formato único de `moneytext`, ajustada al ancho del papel.
- **Go · indicaciones.** `catalog/quick_notes.go`: alta y edición con permiso antes de leer con bloqueo, validación (1–80, sin comas ni caracteres de control), revisión, orden por categoría con `internal/ordering` (alcance `CategoryQuickNotes`; `ordering` admite ahora una columna de desempate distinta de `name`, aquí `text`) y auditoría en la transacción del catálogo (lock 7820148304, el único que toman las escrituras de catálogo). `catalog/actions.go` agrega las indicaciones a los lookups.
- **Puente nativo.** Cinco operaciones nuevas en la lista cerrada de `api.rs`, con el formato que lee `e2e/native-driver.ts`, y la prueba Rust `discount_reasons_and_quick_notes_routes_are_closed` (método, ruta, identificador obligatorio, rechazo de rutas arbitrarias y de borrados inexistentes).
- **Interfaz · descuentos.** `DiscountForm` muestra los motivos activos como botones (`capture-choice`, 56 px, `aria-pressed`) en descuentos y cortesías; al elegir uno aparece su descripción («Descripción del motivo «Otro»», obligatoria de 5 a 500, o «Descripción (opcional)»), y el envío lleva `reason_id`. `AdjustmentList` muestra «Motivo: Atención · descripción». Cobros (administración) tiene la sección «Motivos de descuento» (`DiscountReasonsAdmin`: botones por motivo, alta, nombre, «Exige descripción», «Motivo activo» y posición; solo consulta sin `payments.settings.manage`).
- **Interfaz · indicaciones.** `CategoryQuickNotes`, en el editor de una categoría guardada del Catálogo: lista en orden, alta y edición por indicación (texto sin comas con aviso inmediato, activa, posición) fuera del formulario de la categoría. En la captura, `KitchenNotes` recibe las indicaciones activas de la categoría del producto (`quickNotesFor` sobre los lookups que la captura ya consulta cada 15 s) y las muestra primero, en su orden, como botones que se marcan en su lugar; después las demás de la partida, los atajos de la cuenta sin repetir y «Escribir otra». `CustomizationPanel` las pasa para el producto y `TouchCustomizer` para el producto de cada componente de combo.

### Correcciones de interfaz detectadas en la fase C

- **Pie de la cuenta en la captura.** A 1280 × 720 recortaba a la derecha los importes y «Más…». No ocurría en `main` (`d9228b5`): medido con Playwright en Chromium sobre una copia de esa versión en el scratchpad (`git archive`, sin cambiar de rama), las filas del pie medían 328 px y todo quedaba dentro de la cuenta; con la fase C crecían a 373 px y se perdían 29 px. Lo introdujo la fase A: con los iconos, el ancho mínimo de los botones de una sola palabra («Descuento», «Precuenta») superaba su cuarta parte del pie, y la cuadrícula del pie, sin columnas definidas, crecía dentro de una cuenta de 356 px que oculta lo que sobresale. El pie usa ahora una sola columna `minmax(0, 1fr)` y es contenedor de consulta: con 420 px o menos, el icono va sobre el texto en las acciones secundarias y principales, sin cambiar alturas (44 y 56 px) ni orden; las principales usan `repeat(2, minmax(0, 1fr))`. Playwright (`expectBillFooterFits` en `e2e/restaurant.spec.ts`) comprueba a 1280 × 720 y 1920 × 1080, en claro y oscuro, que el pie, sus filas, botones e importes no se desbordan y quedan dentro de la cuenta y de la ventana, con «Más…» y el total completos (falla con el CSS de la fase C). El título de la captura no se recorta en el recorrido; con nombres largos conserva sus puntos suspensivos y no se cambió.
- **Avisos de éxito «visibles más de 4 s».** El registro de la vida de cada aviso en el recorrido completo (cada 100 ms, Chromium y WebKit) mostró que todos los éxitos empiezan a retirarse a sus 4 s (máximo 4.3 s), también los tapados por otros más recientes: ni la pila apilada, ni la supresión de «hover» de `AppToaster` ni los temporizadores de los avisos tapados los retenían. La causa era visual: las reglas que ocultan el contenido de los avisos tapados usaban `.app-toast-frame > .app-toast`, pero Sonner 2.0.8 envuelve cada aviso propio en `[data-content] > [data-title]`, así que el texto y los botones de los avisos anteriores asomaban bajo el más reciente durante toda su vida. Los selectores son ahora descendientes, la tarjeta tapada ocupa la altura del aviso del frente y su contenido queda con `visibility: hidden`, de modo que tampoco se pueden tocar sin verlos su acción o su cierre. El error sigue sin cerrarse solo, el toque lo cierra y la sustitución por mismo texto o `id` no cambia (`AppToaster`, `AppToast` y `notify` no cambiaron). Lo cubren Vitest (`components/messages.test.tsx`: tres éxitos y un error apilados con temporizadores falsos; cada éxito se cierra a sus 4 s y el error tapado permanece hasta tocar «Cerrar aviso») y Playwright (`watchNotices`/`expectNoticesClosedOnTime` en `e2e/notices.ts`: en todo el recorrido cada éxito se retira antes de 5 s y ningún aviso tapado muestra su contenido; falla sin la corrección).

### Decisiones menores

- **Sin `reason_id` el descuento se guarda sin motivo del catálogo** (`reason_id` nulo, como las filas históricas) en lugar de asignarle «Otro»: así la API anterior produce exactamente lo mismo que en la etapa 6 aunque «Otro» se desactive o se renombre. La interfaz siempre envía el motivo; los reportes de la etapa 9 tratarán el nulo como «sin motivo del catálogo».
- **El descuento guarda el nombre de su motivo** (`account_adjustments.reason_name`, añadido en la revisión de la fase D): el diseño solo preveía `reason_id`, y unir con el catálogo hacía que renombrar un motivo cambiara cómo se veían los descuentos anteriores. Ahora el historial, la precuenta, el ticket y los reportes de la etapa 9 muestran el nombre tal como era al registrar; renombrar o desactivar un motivo no altera nada de lo registrado. Un CHECK con nombre (`account_adjustments_reason_name_check`) exige que el nombre acompañe siempre al motivo y solo a él.
- **Debe quedar al menos un motivo activo** (`LAST_ACTIVE_REASON`), porque el formulario de descuento exige elegir uno. Nombre de 1 a 60 caracteres, único, sin saltos de línea (se imprime).
- **Ningún motivo viene preseleccionado**, tampoco en la cortesía: se elige con un toque. La descripción aparece al elegir el motivo; en los que no la exigen es opcional.
- **Texto impreso**: «Concepto · Motivo» con los conceptos existentes, por lo que se imprime «Descuento manual · Atención» (o el concepto capturado) en lugar del ejemplo «Descuento · Atención». La línea de dólares va alineada como las demás: «Total en dólares (T.C. $18.50)» a la izquierda y «US$X» a la derecha, en lugar de «Total en dólares: US$X (T.C. $Y)» en una sola cadena.
- **La vista previa de la precuenta en pantalla** (`GET …/precheck`) muestra la misma línea con el turno de la terminal que la consulta.
- **Indicaciones rápidas con las convenciones del catálogo**: revisión optimista sin `operation_id` (las escrituras del catálogo nunca usaron `restaurant_operations`); un alta repetida tras un resultado incierto responde `DUPLICATE_QUICK_NOTE` en lugar de duplicarse. Solo toman el lock del catálogo (7820148304), como todas sus escrituras.
- **Unicidad sin distinguir mayúsculas** por categoría, hasta 40 por categoría, una indicación no cambia de categoría y no se borra (se desactiva). Solo se administran en categorías ya guardadas.
- Los lookups entregan también las inactivas (con `active`) para la administración; la captura usa solo las activas y las recibe con el sondeo existente de los lookups (15 s), sin peticiones nuevas.
- **En la captura**, las indicaciones de la categoría se marcan y desmarcan en su lugar (no se mueven al principio como las demás elegidas), para que la paleta de la categoría no cambie de orden con cada toque.

### Limitaciones

- No verificado en la ventana nativa (WKWebView) ni en pantalla táctil física; ninguna precuenta ni ticket se imprimió en papel (la impresora de las pruebas es un agente no disponible y los trabajos quedan en intervención).
- Las indicaciones solo existen por categoría: no hay indicaciones propias ni exclusiones por producto, ni se guardan sus identificadores en la partida (la propuesta de la etapa 6.1 las dejaba como opción); los reportes solo verán el texto.
- Playwright cubre el descuento con motivo y el total en dólares de la precuenta en pantalla; las indicaciones rápidas en la captura y la administración de motivos e indicaciones solo con Vitest y la integración de Go.

### Pruebas de la fase D

- **Integración PostgreSQL** (`operations/stage71_phase_d_test.go`, encadenada al final de `TestRestaurantAcceptance` con caja, cajero con límite de 100 % y turno propios, e impresora de tickets sin hardware): siembra y orden de los motivos y consulta rechazada sin `payments.read`; descuento con «Atención» (`reason_id` guardado, `reason_name`, auditoría con el motivo) y cortesía con «Cortesía de la casa» y descripción; «Otro» sin descripción, con menos de 5 o más de 500 caracteres, motivo inexistente o inválido y descripción opcional de más de 500, rechazados sin registros, y «Otro» con descripción aceptado; sin `reason_id` como en la etapa 6 (sin motivo rechazado, con motivo escrito guardado con `reason_id` nulo); motivo desactivado rechazado (`DISCOUNT_REASON_INACTIVE`) sin registros y desactivar sin alterar el descuento ya registrado; renombrar un motivo sin alterar el nombre guardado en el descuento anterior, y un descuento nuevo con el nombre vigente; precuenta impresa y ticket impreso con «Descuento manual · Atención» e historial con el motivo; administración sin permiso (403 sin registros) y con permiso, idempotente con el mismo `operation_id`, auditada (alta, orden, edición), con nombre duplicado, revisión obsoleta y nombre vacío rechazados, posición al inicio y después de «Otro», y `LAST_ACTIVE_REASON`; total en dólares ausente con la función desactivada y con un turno sin tipo de cambio, presente en la precuenta y el ticket con el tipo del turno (`usd_rate` del documento, importes en pesos sin cambios), ausente en la precuenta de una terminal cuya caja no tiene turno y otra vez ausente al desactivar la función; indicaciones rápidas sin permiso (403 sin registros), creadas con permiso y auditadas, con comas, vacías, de más de 80, con salto de línea, repetidas en mayúsculas (`DUPLICATE_QUICK_NOTE`) y de una categoría inexistente rechazadas, edición con revisión obsoleta, con coma, sin permiso o cambiando de categoría rechazada, orden al inicio auditado, exposición en los lookups (orden y estado) y llegada a la comanda: una partida con dos indicaciones rápidas y una escrita conserva el texto y su comanda imprime «NOTA: Bien cocida, Sin cebolla, Sin sal».
- **Actualización desde el esquema 8**: `database/existing_install_test.go` (ver «Migración 0009 (ampliada)»).
- **Go unitarias**: `payments/documents_test.go` (`TestCommercialDocumentShowsReasonAndDollarTotal`: «Descuento · Atención», «Cortesía · Cortesía de la casa», cargo sin separador, «Total en dólares (T.C. $18.50)» con «US$88.92» después del TOTAL, ancho de 58 mm sin perder el importe y sin la línea cuando faltan los datos) y `ordering/ordering_test.go` (desempate por texto de las indicaciones y por nombre en los demás alcances).
- **Vitest**: `adjustments.test.tsx` (motivos activos como botones en su orden, sin el inactivo; aplicar exige elegir; «Atención» sin descripción envía `reason_id`; «Otro» exige de 5 a 500; la cortesía elige su motivo; el historial muestra «Motivo: Promoción»; las pruebas anteriores de autorización y reintento ahora eligen «Otro» con su descripción); `capture.test.tsx` (indicaciones de la categoría antes de «Escribir otra», sin la inactiva ni las de otra categoría, sin duplicar el atajo de la cuenta, marcadas en su lugar y guardadas como «Sin cebolla, Bien cocida» en `customization.notes`); `pages/catalog/quickNotes.test.tsx` (regla de texto, orden, alta con posición y aviso, edición con revisión, sin permiso solo consulta); `pages/payments/discountReasons.test.tsx` (orden, alta con posición y «Exige descripción», desactivar con revisión, sin permiso solo consulta).
- **Rust**: `discount_reasons_and_quick_notes_routes_are_closed`.
- **Playwright** (`e2e/payments-flow.ts`, Chromium y WebKit): la precuenta en pantalla muestra «Total en dólares (T.C. $…)» con la función de dólares activa del recorrido; el descuento del 10 % elige «Atención» (botones del catálogo sembrado en su orden, de 44 px o más, `aria-pressed`) con descripción opcional y el historial de ajustes muestra «Motivo: Atención · Cliente frecuente». Además, las comprobaciones del pie y de los avisos de la sección anterior.

### Verificación de la fase D

Ejecutada sobre la copia aislada sincronizada con el repositorio (clúster temporal 55499, Go temporal 8183) después de `tools/format71.sh`:

- Estáticas (`tools/suite.sh`): Prettier, gofmt, `cargo fmt`, `sync-metadata --check`, `go vet` y `go vet -tags=integration` aprobados.
- Go unitarias con `-race`: `ok` en todos los paquetes, incluidos `payments` (plantilla con motivo y total en dólares) y `ordering`.
- Vitest: 39 archivos y 317 pruebas aprobadas; `tsc` y `npm run build` sin errores; cargo: 15 aprobadas, 1 ignorada.
- Integración PostgreSQL (`tools/iter_snap.sh`): `database`, `administration`, `catalog`, `operations` (126,4 s), `payments` y `kitchen` en `ok`; la prueba de impresora física quedó omitida porque es opcional. Una segunda ejecución detallada confirmó `TestStage71UpgradeOfExistingInstallation` y las doce subpruebas `71D_` aprobadas.
- Playwright: 1 aprobada en Chromium (55,1 s) y 1 en WebKit (53,5 s), con el descuento con motivo, el total en dólares de la precuenta, el pie de la cuenta sin desbordamiento y los avisos que se cierran a tiempo.
- Ningún documento se envió a una impresora ni a un cajón real. La 0009 no se aplicó a la instalación real.
- Revisión antes del commit: se agregó `account_adjustments.reason_name` para que renombrar un motivo no altere los descuentos registrados (ver decisiones), con su CHECK con nombre, la prueba de actualización ampliada y la subprueba `71D_renombrar_un_motivo_no_altera_lo_registrado`. Repetidas después todas las suites: estáticas aprobadas, Go unitarias en `ok`, Vitest con 317 pruebas aprobadas, `tsc`, `build` y cargo (15 aprobadas, 1 ignorada) sin errores, los seis paquetes de integración en `ok` (`operations` en 126,7 s) y Playwright con 1 aprobada en Chromium y 1 en WebKit.

## Fase E · Cierre

### Versión y documentación

- Versión **0.7.1** y `schema_version: 9` en `config/product.json`, con los derivados actualizados: `backend/internal/product/metadata.json`, `desktop/src/generated/product.json`, `desktop/package.json`, la entrada raíz de `desktop/package-lock.json`, `desktop/src-tauri/Cargo.toml`, la entrada raíz de `desktop/src-tauri/Cargo.lock` y `desktop/src-tauri/tauri.conf.json`. La 0009 se aplicó después a la instalación real (ver «Instalación real»).
- Verificación: la de la etapa 7 quedó archivada sin cambios como `docs/VERIFICATION_STAGE7.md` (`git mv`) y la nueva [VERIFICATION.md](VERIFICATION.md) registra, por fase, solo lo ejecutado en las fases A–D y lo que no se verificó.
- `CLAUDE.md`: encabezado (0.7.1 y esquema 9), tecnologías, estructura (migraciones 0001–0009, con 0001–0008 aplicadas), patrones de avisos, confirmación e iconos, fila 7.1 del plan, decisiones de la etapa 7.1 (y corrección de la frase de la etapa 7 que limitaba el cajón a ESC/POS TCP), archivos de referencia, riesgos y limitaciones, y criterios de esquema, frontend y documentación.
- `README.md` (lista de migraciones y estructura), `ARCHITECTURE.md` (migración 0009) y `DEVELOPMENT_MACOS.md` (versión, esquema y procedimiento de la 0009).
- Revisión de las guías actualizadas por las fases: `docs/KITCHEN_AND_PRINTING.md`, `docs/PAYMENTS_AND_REFUNDS.md` y `docs/CASH_SHIFTS_AND_RECONCILIATION.md` pasan a 0.7.1 y esquema 9 (la guía de caja enlaza ahora la regresión de la etapa 7 en `VERIFICATION_STAGE7.md`); `API_CONTRACTS.md` corrige la fila de `/version`, que seguía diciendo «esquema 5»; `docs/RESTAURANT_OPERATIONS.md` aclara en la sección de la etapa 6.2 que «Nueva cuenta» también crea la cuenta con el primer producto. `docs/CATALOG.md` y `docs/DEPENDENCIES.md` ya cubrían la 7.1.
- `AGENTS.md`, `README.md`, `ARCHITECTURE.md` y `DEVELOPMENT_MACOS.md` describen el cajón por ESC/POS TCP o por una cola CUPS en modo raw. El estado de las migraciones en `AGENTS.md` y `NEXT_STEPS_FOR_SOL.md` se actualiza al aplicar la 0009.

### Regresión final

Ejecutada el 9 de noviembre de 2026 con la versión 0.7.1 y `schema_version: 9`, sobre la copia aislada sincronizada con el repositorio (clúster temporal 127.0.0.1:55499, Go temporal en 8183):

- **Estáticas** (`tools/suite.sh`): Prettier («All matched files use Prettier code style!», sin las especificaciones del propietario), gofmt sin archivos, `cargo fmt --check`, `sync-metadata --check`, `go vet` y `go vet -tags=integration`, todos con salida 0.
- **Go unitarias** con `-race`: `ok` en todos los paquetes, incluidos `moneytext` y `plural`.
- **Vitest**: 39 archivos y 317 pruebas aprobadas; `tsc` y `npm run build` sin errores.
- **Rust**: `cargo test`, 15 aprobadas, 0 fallidas y 1 ignorada (Keychain real).
- **Integración PostgreSQL** (`tools/iter_snap.sh`): `database` (2,4 s), `administration` (53,7 s), `catalog` (6,2 s), `operations` (126,3 s), `payments` (1,4 s) y `kitchen` (1,4 s) en `ok`. La prueba de impresora física quedó omitida porque es opcional. Los paquetes con pruebas de integración son los mismos que lista `scripts/database.py`, más `kitchen`.
- **Playwright**: 1 aprobada en Chromium (1,0 min) y 1 en WebKit (1,0 min).
- **Paquete de escritorio**: `tauri build --bundles app` en la copia aislada, con `CARGO_TARGET_DIR` en el scratchpad: `AD-EL POS.app` 0.7.1, ejecutable Mach-O arm64, 9,5 MB.
- Ninguna suite envió trabajos a una impresora ni a un cajón real.
- `./scripts/dev.sh format-check` sobre el repositorio completo no aprueba solo por las especificaciones del propietario `docs/etapas/ETAPA_7.md` y `ETAPA_7_1.md`, que no se formatean.

Detalle en [VERIFICATION.md](VERIFICATION.md).

### Procedimiento para aplicar la 0009 a la instalación real

Propuesto al cierre, con la instalación real en 0.7.0 y esquema 8 sin modificar. Se ejecutó el 9 de noviembre de 2026 desde la rama `etapa-7.1` (ver «Instalación real»):

1. **Preparar.** Cerrar AD-EL POS en todas las terminales, sin cobros ni arqueos en curso. `./scripts/dev.sh status` confirma el servidor 0.7.0 en 8181.
2. **Respaldar.** `./scripts/dev.sh backup` genera `backups/adel_pos-<fecha>.dump` y lo comprueba con `pg_restore --list`.
3. **Ensayar la restauración.** `python3 scripts/restore-check.py backups/NOMBRE.dump` restaura en una base temporal y compara los conteos de todas las tablas (79 en el ensayo de la etapa 7, ya en esquema 8). Si no coinciden, se detiene el procedimiento.
4. **Detener.** `./scripts/dev.sh server-stop`, de forma ordenada.
5. **Migrar.** `./scripts/dev.sh migrate` compila el servidor 0.7.1 y aplica la 0009 en una sola transacción. Resultado esperado `applied:1`; una segunda ejecución debe dar `applied:0`. Si falla, la transacción no deja cambios.
6. **Arrancar.** `./scripts/dev.sh server-start`.
   - `GET http://127.0.0.1:8181/api/v1/health/ready` debe responder 200, con PostgreSQL disponible y esquema compatible.
   - `GET /api/v1/version` debe indicar 0.7.1 y esquema 9.
7. **Comprobar.** Comprobaciones de solo lectura:
   - Migraciones 1–9 y los mismos conteos de cuentas, pagos, ajustes, turnos y comandos del cajón.
   - Cinco motivos de descuento activos en su orden (Atención, Promoción, Demora, Cortesía de la casa y Otro), solo «Otro» con descripción obligatoria.
   - Los descuentos anteriores conservan su motivo escrito con `reason_id` y `reason_name` nulos.
   - Ninguna indicación rápida y ningún permiso nuevo (el total de permisos no cambia).
8. **Instalar la aplicación.** `./scripts/dev.sh build-macos` y `./scripts/dev.sh open-app`.

Efectos de la 0009 sobre los datos existentes: no modifica 0001–0008 ni filas existentes; reemplaza con el mismo nombre el CHECK de motivos del cajón (las filas anteriores siguen siendo válidas), agrega un índice parcial de auditoría, dos tablas y dos columnas nulas; su única siembra son los cinco motivos de descuento. Las cuentas vacías que ya existen en la instalación real no cambian con la migración: se liberan desde el panel de la mesa con «Liberar mesa».

Vuelta atrás:

- **Antes del paso 5:** basta `server-start` desde `main` (0.7.0).
- **Después de aplicarla:** solo con indicación del propietario, restaurar el respaldo del paso 2 con la aplicación cerrada y volver a la 0.7.0.

Mientras no se autorice, **no** ejecutar `migrate`, `server-start` ni `build-macos` sobre la instalación real desde `etapa-7.1`: `migrate` aplicaría la 0009 sin el respaldo previo, y un servidor 0.7.1 vería el esquema 8 como incompatible (readiness 503), porque el migrador rechaza un historial con migraciones pendientes (según su diseño en [ARCHITECTURE.md](../ARCHITECTURE.md); no se ejecutó).

### Instalación real

El propietario autorizó por adelantado aplicar los cambios al terminar si todo aprobaba. Tras la regresión final y el commit de la fase E: respaldo `backups/adel_pos-20261110T022252Z-c7e1e4.dump` con restauración verificada (90 tablas idénticas, `.local/stage9-restore-verification.json`), `server-stop` ordenado, `migrate` `applied:1` y repetición `applied:0`, servidor 0.7.1 iniciado con readiness 200 (PostgreSQL disponible, esquema compatible) y `/api/v1/version` con 0.7.1 y esquema 9, y `.app` 0.7.1 compilado con `build-macos` y abierto con `open-app`. Comprobaciones de solo lectura antes y después: migraciones 1–9; 90 → 92 tablas en `app`; los mismos conteos de cuentas (4 abiertas y 2 cerradas), pagos (2), ajustes (0), turnos (1 cerrado), comandos del cajón (0), categorías (4), auditoría (107) y permisos (113, ninguno nuevo); los cinco motivos de descuento activos en su orden, solo «Otro» con descripción; ningún ajuste con motivo ni indicación rápida; el CHECK del cajón admite `test`, existe el índice de rechazos y las once restricciones con nombre de la 0009; `usd_cash` desactivada. El recorrido en la ventana nativa no se verificó. La 0009 es ahora inmutable; la siguiente ampliación requiere 0010.
