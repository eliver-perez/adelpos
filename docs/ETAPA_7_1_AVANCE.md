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
