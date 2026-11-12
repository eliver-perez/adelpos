# Etapa 9C · Avance

Nueva interfaz principal, navegación, teclado y ajustes. Especificación del propietario:
[docs/etapas/ETAPA_9C_v3.md](etapas/ETAPA_9C_v3.md) (§0-B prevalece). Rama local `etapa-9c`, creada desde `etapa-9b`
(`0c7e488`, versión 0.9.1, esquema 19, ya instalada) **por decisión del propietario**: la especificación dice `main`, pero
`main` sigue en la 9A (0.9.0, esquema 17) y una 0.9.2 sin la 9B sería incompatible con la instalación real. Sin push ni
merge. Límite del propietario: nunca más de 2 agentes trabajando a la vez y sin subagentes de subagentes; Fase 0,
revisión independiente y regresión final, un agente cada una. Autorización previa del propietario (11/11/2026): al
terminar, si todo está bien, aplicar los cambios a la instalación real sin esperar confirmación.

## Fase 0 · Verificación contra el código

Hecha por un solo agente (V0) el 11/11/2026 sobre `etapa-9c` (creada desde `etapa-9b` por decisión del propietario; la
especificación dice `main`), versión 0.9.1, esquema 19. Solo lectura: no se editó código. Inventario detallado por
archivo y línea en el inventario técnico de la fase (directorio temporal de la sesión). `docs/etapas/ETAPA_9C_referencia.png` no existe. §5.4 ya está
cumplido: `docs/etapas/` figura en `.prettierignore:15`.

### Decisiones que pide la Fase 0 (no las cambia, pero hay que fijarlas)

1. **Pantalla de inicio (§1.5 y §4.1).** Hoy solo existe por **usuario** (`app.user_preferences.home_screen`, DEFAULT
   `'status'`, `0002…sql:108-112`; Go cae a `status`, `preferences.go:39-59`); no hay nada por terminal (`app.devices` solo
   tiene `register_id` y `keyboard_mode`, `0002…sql:64-73`, y ninguna migración posterior la altera). Hace falta la columna
   por terminal (Mesas por omisión) en la 0020. Recomendación: la terminal manda (§4.1: «la aplicación abre en la pantalla
   de inicio de la terminal») y el logotipo vuelve a ella; «Pantalla inicial» sale de Mis preferencias (Go conserva el
   campo por compatibilidad y deja de usarlo); si el usuario no puede ver la pantalla de la terminal, la primera sección
   permitida de la barra, nunca «Estado del sistema». Se configura en Administración → Sistema → Terminales
   (`devices.manage`), no en `PUT terminal/preferences`, que hoy no exige permiso (`preferences.go:105-123`).
2. **«Apariencia predeterminada para usuarios nuevos» (§4.4).** Hoy no hace eso: la apariencia de Configuración general
   solo se copia al administrador en el asistente (`setup.go:123`); un usuario nuevo recibe `light` por el DEFAULT
   (`users.go:79`) y `readPreferences` también cae a `light` (`preferences.go:54-56`). Con solo renombrar, la etiqueta
   mentiría. Opciones: (a) Go copia `establishment.theme` al crear el usuario y lo usa como respaldo al leer (cambio pequeño,
   sin migración ni permisos; recomendado); (b) renombrar sin cambiar el comportamiento (no recomendado).
3. **§5.1 · lo que no se guarda.** AD-EL POS **no pide autorización de otra persona** para anular una cuenta ni para
   cancelar o reducir cantidades: lo hace quien tiene `accounts.manage` u `orders.cancel` (`operations/handler.go:202-205`,
   `amendments.go:201-205`); la autorización de supervisor solo existe en cobros y caja. Una columna «Autorizó» no puede
   llenarse sin inventar datos; agregar el paso cambiaría un flujo (prohibido por §0-B/§7). Además, dos de los cuatro ya
   existen desde la 9A: «Recibió» en «Compras recibidas por periodo» (`reports/purchases.go:107, 281-283`) y «Referencia»
   (opcional, oculta por omisión) en «Detalle de cobros» (`reports/payments_details.go:62, 75`). Ver punto 6.
4. **Selección múltiple (§2).** No hay ningún `<select multiple>`; lo múltiple se hace con casillas (`ScopeSelector` de
   promociones con productos/categorías/listas, estaciones de una ruta y de una terminal, editor de permisos) y con botones
   (días). Recomendación: el selector múltiple sustituye a `ScopeSelector` (productos y categorías son muchos); el editor
   de permisos y los días se quedan.
5. **Pantallas sin lugar en §1.1/§1.3.** Las vistas internas de Restaurante «Cuentas» (cuentas sin mesa) y «Áreas y
   distribución» (`OperationalPage.tsx:74-89`) no aparecen en la barra ni en los mosaicos. Recomendación: siguen como
   pestañas dentro de Mesas (§0-B: no se rediseñan las pestañas internas). Además, «Cobros» se ve hoy con
   `cash.shifts.read` (`PreferencesPage.tsx:36-47`) por su acordeón «Turno actual»; un cajero vería el botón Administración
   solo por eso: conviene que el mosaico Cobros exija los permisos de administración de cobros (no cambia permisos, solo la
   visibilidad del mosaico).
6. **«Registro de movimientos» (§4.2).** Existen «Caja → Movimientos» (`cash/CashPage.tsx:21`), «Inventarios → Existencias
   → Movimientos» (`inventory/inventorySections.ts:75`) y el reporte «Movimientos de efectivo» (`reports/cash_details.go:31`):
   no se llaman igual, pero se confunden. Opciones: dejarlos (el contexto los distingue) o llamarlos «Movimientos de caja»
   y «Movimientos de inventario» (rompe nombres de pestañas en Playwright). Recomendación: dejarlos y reportarlo.

### 1. Navegación actual

- Sin rutas: estado `screen` en `App.tsx:56` (inicial `'status'`) y `switch` en `content()` 209-276. Lista y permisos en
  `PreferencesPage.tsx:17-52` (`screens`, 17 entradas) con `screenAuthorized` 62-70 (`permission` exacto + `anyPermission`
  alguno); `App.tsx:67-70` exige además `accounts.read` para Para llevar. Go replica la tabla en `preferences.go:15-32`.
- Menú lateral `App.tsx:280-309` (`nav aria-label="Navegación principal"` 287), cabecera 311-327, navegación angosta con
  `<select aria-label="Pantalla">` bajo 920 px 328-344 (sobra con §0-B). La operación (Mesas/Para llevar) vive montada
  siempre y se oculta (`App.tsx:358-372`), con sus pestañas internas Mesas, Para llevar, Cuentas y Áreas y distribución
  (`OperationalPage.tsx:287-333`); sin permisos de mapa, «Restaurante» abre «Cuentas» (`useRestaurantWorkspace.ts:203`).
- Inicio: `homeScreen(preferences.home_screen)` al iniciar sesión y `'status'` si la pantalla deja de estar permitida
  (`App.tsx:90-104`); la sugerencia «se utilizará Estado del sistema» (`PreferencesPage.tsx:111`). Por terminal: no existe
  (decisión 1).
- Cambios sin guardar: `useLeaveGuard.ts:9-34` (conjunto global + `beforeunload`), solo lo usa el editor de plantillas
  (`Designer.tsx:126`); `App.tsx:171-196` (`confirmLeave` con `ConfirmDialog`, `requestScreen`) y `logout` 198-207. Lo
  saltan `setScreen` directos: Inventarios → Configuración (`App.tsx:244`) e «Ir a Caja» (`App.tsx:369`). La barra, el menú
  nativo, la paleta y los atajos deben pasar todos por un único `requestScreen`.

### 2. Caja, conexión, pantalla completa, usuario y captura

- **Atajo Caja 7.1**: `pages/cash/CashShortcut.tsx` (estados 24-32; `useCashShortcut` 46-114 consulta el turno cada 15 s
  solo con `active`; visible con `cash.shifts.read` **y** `payments.read`; opciones 128-187, incluida «Abrir turno» que
  §1.1 no menciona; botón `aria-label="Caja · {estado}"` 255-262). Una sola instancia en `OperationalPage.tsx:54`, pintada en
  la barra operativa 305, la hoja 344 y la captura 477-478. Para la barra global hay que subir el gancho y la hoja a `App`.
  Colores ya en tokens (`styles.css:4553-4575`: `--cash-entry` verde, `--cash-exit` ámbar, neutro).
- **«Sesión verificada · En tiempo real»**: `App.tsx:318-326`. Lo primero sale de `session.connectionError` (sondeo de `me`
  cada 5 s, `useSession.ts:65-74`); lo segundo de `useRealtime.ts` (solo con `events.read`, que hoy solo tiene el
  administrador): «En tiempo real», «Recuperando eventos · sondeo activo», «Canal interrumpido · sondeo activo»,
  «Sondeo activo · canal no disponible» o «Actualización periódica» (29-55, 118); Rust envía
  `connected|reconnecting|unauthorized` (`realtime.rs:84-119`). Los tres estados de §1.1 se derivan de estas dos fuentes.
- **`WindowControls`**: `components/WindowControls.tsx` (texto «Salir de pantalla completa»/«Pantalla completa», `title`
  con «Control + ⌘ + F»), fijo arriba a la derecha (`styles.css:1541-1556`) y montado fuera de la sesión (`App.tsx:47`):
  también aparece en acceso y asistente, donde la barra no existirá. Rust `main.rs:58-71`.
- **Menú del usuario**: hoy es el pie del menú lateral (`App.tsx:299-308`: nombre, rol, Cambiar contraseña, Cerrar sesión).
  «Mis preferencias» es una entrada del menú (`PreferencesPage.tsx:51`) con Tema, Pantalla inicial y el teclado de la
  terminal (149-198).
- **Barra de la captura**: `capture/CaptureTopBar.tsx:73-196` (regreso «Mesas»/«Para llevar»/«Cuentas», mesa y cuenta,
  detalles, «Datos de la cuenta», otras cuentas, Caja, Actualizar); en captura el menú lateral y la cabecera de `App` siguen
  visibles (más estrechos, `styles.css:962-1023`). Enviar a cocina y Cobrar viven en `BillFooter`.

### 3. Listas desplegables

105 `<select>` en 58 archivos, ninguno múltiple, sin `<datalist>`. Concentración: `ReferenceField` (`EditorFields.tsx:175`,
30 usos), `PositionField` (9), `ItemPicker` (8), `SupplierPicker` (5), `WarehouseSelect` (4), `DimensionUnitSelect` (4),
`MeasureSelect` (3), `RecipeItemPicker` (3) y los filtros de reportes. Con muchas opciones: zona horaria (≈420), productos,
ingredientes, categorías, grupos, mesas, insumos, proveedores, preparaciones, turnos de caja, tipos de movimiento de
inventario (≈16), documentos recientes, reportes de un grupo (hasta ≈19), meses (13); siete componentes ya van con un campo de
búsqueda aparte (zona horaria; insumos, proveedores, preparaciones e ingredientes; productos e insumos de reportes). Hay `optgroup` (unidades, ingredientes del catálogo), una
opción de acción «+ Nueva categoría» (`inventory/ItemEditor.tsx:328`, valor `__new_category__` → `QuickCategoryDialog`) y otra
«más unidades» (`inventory/CountPanel.tsx:141`). Caso especial: `UsersPage.tsx:154`, un select de 3 opciones por cada uno de
≈150 permisos. Los IVA ya son botones cerrados (`SegmentedControl`) y deben seguir así. Lista completa en el inventario técnico de la fase.

### 4. Teclado y menú nativo

- Atajos existentes: Escape y foco atrapado en `ConfirmDialog`/`ChoiceDialog`, hoja y menú de Caja (Escape en `document`
  sin `stopPropagation`), `QuickCategoryDialog`, `ModalPanel`, «Más…», paleta de colores; flechas en el diseñador de mesas
  (`TableMap.tsx:187-204`) y en los gráficos; **Alt/Ctrl/Cmd + ↑/↓ mueve bloques** en el diseñador 9B
  (`StructureList.tsx:91-106`); teclado físico del cobro en `NumericKeypad.tsx:75-102` (ignora modificadores; **Escape borra
  el importe**, `amountInput.ts:82-84`); Intro en indicaciones. No hay atajos globales ni paleta. Teclado virtual en
  `VirtualKeyboard.tsx` (abre con `focusin`, no está en un portal).
- Tauri: `main.rs:130-182` no define menú. Con Tauri 2.12.5 macOS muestra el **menú predeterminado en inglés**
  (`tauri-2.12.5/src/app.rs:2409-2414`, `src/menu/menu.rs:142-240`): AD-EL POS (Acerca de, Ocultar, Salir), Edit
  (deshacer, cortar, copiar, pegar, seleccionar todo), View (pantalla completa, el «Control + ⌘ + F» del botón), Window
  (minimizar, cerrar ventana ⌘W). La capacidad `capabilities/main.json` solo concede `allow-diagnostics` (9 comandos propios,
  `permissions/diagnostics.toml`, `build.rs`), sin `core:*`. Para el menú propio bastan `Builder::menu` + `on_menu_event`
  en Rust (no requieren capacidades); para avisar a React y para habilitar opciones según permisos, lo coherente con el
  puente cerrado es un `Channel` con comandos propios (p. ej. suscripción y estado del menú) en `build.rs`,
  `diagnostics.toml`, `generate_handler!`, `native-driver.ts` y pruebas Rust; la alternativa `listen()` exigiría añadir
  `core:event:allow-listen`/`allow-unlisten`.

### 5. Reportes del sistema anterior (solo lectura de `../legacy-csharp/PoS/`)

- Canceladas: `cancelaciones(venta, autorizo, fecha)` (`database/Base de Datos.sql:926-934`); lista `RegistroCanceladas.cs:33-48`
  con folio, fecha, caja, **quien autorizó** y total; no guardaba motivo ni quién canceló.
- Modificaciones de cantidad: `ventas_modificaciones(producto, cantAnt, cantNueva, autorizacion, fecha)` (`Base de Datos.sql:854-863`,
  escrito en `CapturaOrden.cs:1586-1603` y `appFunctions.cs:1173`); sin quién la hizo ni motivo; no se encontró pantalla que lo
  listara.
- «Recibió»: `entradas.recibio` (`Base de Datos.sql:591-602`), mostrado en `RegistroEgresos.cs:157-171` (columna «Recibio»), pero
  `Entrada_Inventario.cs:121-122` guardaba siempre el usuario 1.
- Folio de tarjeta: `pagos.folio varchar(20)` (`Base de Datos.sql:912-924`), capturado con teclado en `PagarTarjeta.cs:191-242` y
  guardado en `PuntoDeVenta.cs:280`; `RegistroVentas.cs:177-186, 248-249` solo sumaba efectivo y tarjeta (no listaba el folio).

### 6. §5.1 · datos que existen hoy

| Reporte                      | Existe                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | No se guarda                                               |
| ---------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| Cuentas anuladas             | `restaurant_accounts.state='voided'`; evento `account.void` (motivo 5-500, autor, terminal, fecha; `operations/accounts_write.go:305-313, 334-344`) o `account.released` (motivo fijo «Cuenta vacía liberada», `accounts_release.go:77-87`) en `restaurant_events` (`0004…sql:156-170`). Solo se anula una cuenta **sin productos activos ni borradores y con cobros netos en cero**: su «importe» activo es 0; lo útil es la suma de sus unidades canceladas (`restaurant_item_units` con `state='cancelled'`). | Quién autorizó (no hay autorización).                      |
| Modificaciones de cantidades | Eventos `item.cancelled` (unidades antes con importes, cantidad, ids, motivo, autor; `cancellations.go:62-68`) e `item.modified` (unidades antes, petición nueva, `cancelled_unit_ids`, motivo, autor; `amendments.go:282, 306-311`, `amendment_persistence.go:78`); todas son posteriores a confirmar. Cantidad anterior/nueva, diferencia en unidades e importe se calculan sin inventar. Ya existe «Productos cancelados» por unidad (`reports/products.go:200-235`, con motivo y «Canceló»).                 | Quién autorizó (basta `orders.cancel` del propio usuario). |
| «Recibió» en compras         | Ya está: `purchases.received` columna «Recibió» = `inventory_receipts.received_by` (actor que registró, `inventory/receipts.go:385-390`).                                                                                                                                                                                                                                                                                                                                                                        | —                                                          |
| Folio de tarjeta             | `payments.reference` y `payment_refunds.reference` (texto ≤120, `0006…sql:92, 128`), común a todo método no efectivo, opcional por omisión en débito/crédito (`reference_mode`, `0006…sql:46, 54-59`; «Referencia (opcional)» en `PaymentCapture.tsx:353-360`). Ya sale como columna opcional «Referencia» en «Detalle de cobros». Encajaría también en «Devoluciones» (`payments.refunds`) y «Correcciones».                                                                                                    | Un folio propio de tarjeta distinto de la referencia.      |

Índices: `restaurant_events` solo por `(account_id, sequence)`; un reporte por periodo recorrerá la tabla (medir antes de
decidir un índice en la 0020). Los reportes nuevos van al registro cerrado de `internal/reports`, a `EXPORTABLE_REPORTS`
(`export.rs:34`) y, si llevan filtros nuevos, a la lista blanca `reports_*` de `api.rs`.

### 7. §2.1 · horas

Seis lugares: hora de inicio del día (`SettingsPage.tsx:290-309`, texto `HH:MM`; Go `business_day.go:20, 67-69`, 00:00-23:59);
hora inicial y final de las restricciones de horario (`EditorFields.tsx:357-381`: `type="time"` y texto libre; Go
`catalog/availability.go:27-31`, `15:04`, final admite **`24:00`**, sin traslapes) usadas en producto, variante, categoría y
promoción; pedido programado (`AccountDetails.tsx:155-162`) y reprogramación desde cocina (`PreparationBoard.tsx:365-371`),
ambos `datetime-local` con fecha (Go `2006-01-02T15:04` en la zona del establecimiento, `accounts_write.go:100-101`,
`releases.go:236-238`); filtros Desde/Hasta de Auditoría (`AuditPage.tsx:51-64`, `datetime-local` convertido con la zona de
la **terminal**, Go RFC3339 `sessions_audit.go:69`). El selector de hora necesita, por tanto, una variante con fecha
(`OptionalDateField` + hora) y la opción 24:00 solo en la hora final.

### 8. §4.1-§4.4

- **Estado del sistema** (`StatusPage.tsx`): aviso de diagnóstico, tarjetas «Backend Go», «PostgreSQL» y «Esta instalación»,
  «Detalles técnicos» (URL del servidor, versión, API, última comprobación) y «Cerrar esta aplicación no detiene Go ni
  PostgreSQL»; consulta `server_snapshot` cada 5 s (`useConnection.ts`, `diagnostics.ts:34-80`). Todo lo que pide §4.1
  (base de datos, servidor, versión) ya está en `Diagnostics`. También se abre antes de iniciar sesión con «Diagnóstico
  técnico» (`App.tsx:106-115, 141-143`), donde aún no se sabe si es administrador.
- **Auditoría**: menú `PreferencesPage.tsx:50`, título y descripción `AuditPage.tsx:19-20`, permiso «Consultar auditoría»
  (`administration/permissions.go:39`; la interfaz lee las descripciones del registro de Go, `routes.go:154-159`). Muestra
  códigos internos (acción, entidad, UUID, JSON). Confusiones: decisión 6.
- **Términos técnicos**: ≈60 textos visibles (lista en el inventario técnico de la fase). Destacan `StatusPage.tsx`, `SettingsPage.tsx:356`,
  `CatalogPage.tsx:149`, `CashPage.tsx:95`, `InventoryPage.tsx:289`, `PaymentsAdminPage.tsx:77`, «Go» en 12 textos fuera de Estado del sistema, la zona
  IANA en 7 etiquetas, claves de permisos y UUID visibles, «Reintentar con el mismo identificador» (11 paneles) y el aviso por
  omisión de todo formulario «Cambios guardados en el servidor.» (`Forms.tsx:66`). El ejemplo «Identificador IANA del
  establecimiento» no existe tal cual; lo más cercano es el mensaje de Go `setup.go:38`. ESC/POS/CUPS/KDS son vocabulario de la
  configuración de impresoras: conviene decidir si se conservan allí.
- **Apariencia**: Configuración general «Apariencia inicial» (`SettingsPage.tsx:119-124`) y Mis preferencias «Tema»
  (`PreferencesPage.tsx:100-108`). Guardar en Mis preferencias llama `save_preferences` y `refresh()`; `App.tsx:95` aplica
  `data-theme` con la nueva identidad: **funciona de inmediato** (no hay vista previa antes de guardar) y **se conserva** al
  volver a entrar. Comprobado con una prueba temporal en la copia (no versionada) y con la aceptación de Go (`17_preferences`).
  Detalles: tras cerrar sesión la pantalla de acceso conserva el tema del último usuario; al abrir la aplicación es claro.
  La apariencia de Configuración general no llega a usuarios nuevos (decisión 2).

### 9. §5.3 · columnas por impresora

Hoy el ancho sale solo del papel: `documents/render.go:28-33` (58→30, otro→42), `Render` 102-133 (texto grande a la mitad
122-124). `RenderOptions.Columns` (19-21, 118-120) ya permite sustituirlo (lo usan las envolturas de pruebas). Cada camino
pasa el papel de la impresora destino: comandas al encolar (`kitchen/documents.go:176-181`) y al reimprimir o redirigir
(`queue_actions.go:160-177`, con `original.PrinterSnapshot` o la impresora nueva), prueba técnica (`queue_actions.go:187-226`),
plantilla vigente (`kitchen/rerender.go:167`, impresora actual), comerciales (`payments/document_data.go:114`,
`documents.go:155`), caja (`cash_documents.go:172, 191-196`); vista previa del diseñador solo 58/80 (`designer/preview.go:60-91`)
y la de la cola fija en 80 (`kitchen/reads.go:215`). Para reimprimir, cada trabajo guarda `content` (texto), `layout` (IR con
`Columns`, si no es texto plano) y `printer_snapshot` (la impresora completa en JSON, `kitchen/documents.go:219-221`): los
comerciales y de caja copian `content`+`layout` (exactos); las comandas se vuelven a renderizar con el `printer_snapshot`
original, de modo que si el campo de columnas entra en `kitchen.Printer` la copia sale con las columnas originales sin otro
dato (las anteriores a la 9C no lo tienen y caen al predeterminado). El respaldo automático cambia de impresora sin volver a
renderizar (`worker.go:268-295`). La columna nueva requiere migración (CHECK por ancho) y `kitchen/printers.go:24-30, 107-136`
(validación y `printer.saved` en `kitchen_history`).

### 10. §5.2 · imágenes del catálogo

`catalog/images.go:47-61` valida tamaño, `DecodeConfig`, dimensiones y decodifica completo **sin recorrer los marcadores**.
La validación a reutilizar es `logoraster/validate.go` (`MaxJPEGScans=100`, `MaxJPEGSegments=1024` 21-30; `checkJPEGSegments`
161-195 y sus auxiliares 199-239, no exportados y con mensajes «El logotipo…»; llamada en `Validate` 82-86). Hay que
exportar el recorrido con un error neutro (cada módulo pone su mensaje) y llevar a la prueba de catálogo un generador como
`progressiveJPEG` de `validate_scans_test.go` (no exportado). `logoraster` no depende de otros paquetes del proyecto.

### 11. Playwright

Navegan con `getByRole('navigation', { name: 'Navegación principal' })` (14 usos en 11 especificaciones) y con botones por
nombre exacto del menú lateral («Restaurante», «Para llevar» `.first()`, «Caja», «Configuración», «Cobros», «Catálogo»,
«Cocina y preparación», «Inventarios», «Reportes», «Documentos»), «Salir de pantalla completa»/«Pantalla completa»
(`kitchen-flow.ts:5-6`), `.application-header` para la posición de los avisos (`notices.ts:32`) y `.capture-topbar`
(`restaurant.spec.ts:578`). Cambian: ayudantes `cash-flow.ts`, `kitchen-flow.ts`, `payments-flow.ts`, `notices.ts`,
`native-driver.ts` (comandos del menú) y las 12 especificaciones; en la Fase B, además, 34 `selectOption` y 42
`getByRole('combobox')`. «Caja» dejará de ser un enlace directo (abre el menú rápido), así que `cash-flow.ts:72, 218, 235, 281`
tendrán que ir por «Ir a Caja» o por teclado. En Vitest: `App.test.tsx` (inicio en «Estado del sistema», botones del menú,
«Sesión verificada»), `WindowControls.test.tsx` y ≈13 archivos con selects.

### 12. Mapa de mesas

`components/restaurant/TableMap.tsx`: áreas como botones (115-128, ya es el selector de área), mosaico con nombre, indicador
de estado con icono (22-63) y «N cuentas» (208-212), leyenda y modo diseñador. Panel lateral `pages/restaurant/TablePanel.tsx`
con las acciones de la 6.2/7.1 ya con `IconLabel`. Sin buscador de mesas. La cuenta **no guarda comensales**: solo hay
`table.seats` (capacidad). No hay iconos de mesa, cocina, engrane, usuario, lupa, maximizar/reducir ni ayuda en `Icon.tsx`.

### Migración necesaria (una sola, 0020, transaccional)

- `app.devices.home_screen text NOT NULL DEFAULT 'tables'` con CHECK con nombre sobre la lista cerrada de secciones de la
  barra (Mesas, Para llevar, Cocina, Caja).
- `app.kitchen_printers.columns`: 80 mm → 42 o 48; 58 mm → 30 o 32. Dos formas seguras: `NOT NULL` rellenada por ancho
  (`CASE paper_width WHEN 58 THEN 30 ELSE 42 END`) con `CHECK ((paper_width = 80 AND columns IN (42, 48)) OR
(paper_width = 58 AND columns IN (30, 32)))`, o `NULL` = predeterminado con `CHECK (columns IS NULL OR …)`. Cambiar el
  ancho de papel obliga a reajustar las columnas en la misma escritura.
- Opcional, tras medir: índice de `restaurant_events` por tipo y fecha para los reportes de §5.1.
- No hacen falta permisos nuevos (reportes con los permisos de la 9A; prueba de ancho con `kitchen.printers.manage`) ni otro
  tipo de ticket (la prueba de ancho puede usar `test`, ya admitido por `kitchen_tickets_kind_check` de la 0018). La versión
  pasa a esquema 20.

### Riesgos

- **Menú nativo en macOS**: el primer submenú se convierte en el menú de la aplicación, así que «Archivo» necesita un
  submenú «AD-EL POS» antes; sin un submenú Edición con los elementos predefinidos se pierden ⌘C/⌘V/⌘X/⌘Z/⌘A en los campos;
  el predeterminado trae ⌘W (cerrar la única ventana). Un atajo con acelerador en el menú y además en React puede ejecutarse
  dos veces (solo verificable en la ventana nativa).
- **Escape global** choca con el Escape del teclado del cobro (borra el importe) y con el del menú de Caja (escucha en
  `document`); los atajos con dígitos sin modificador chocan con `NumericKeypad`.
- **Selector y teclado virtual**: el teclado virtual no está en un portal y su `pointerdown` no cierra nada; un cierre «al
  tocar fuera» del selector debe excluirlo.
- **48 columnas por CUPS en texto**: `transport.go:154-170` fija `cpi=12`; a 48 columnas el texto podría cortarse en colas
  CUPS que no son raw (ESC/POS: 576 puntos = 48 caracteres de 12 puntos; 384 = 32).
- **Inicio**: todos los usuarios de la instalación real tienen `home_screen='status'` por omisión; la regla de la terminal
  debe ignorarlo. `Estado del sistema` antes de iniciar sesión no puede filtrar por administrador.
- **Lugar de Caja en la barra global**: hoy el turno solo se consulta en operación (`active`); en la barra se consultará en
  todas las pantallas (cada 15 s).
- **Pruebas**: el cambio de navegación toca las 12 especificaciones de Playwright y la Fase B, ≈76 interacciones con selects.

### Ejecutado en la copia la copia aislada del agente

- Vitest: `npx vitest run src/zzV0ThemeCheck.test.tsx src/App.test.tsx src/components/WindowControls.test.tsx` → 3 archivos,
  7 pruebas aprobadas (2.4 s). `zzV0ThemeCheck.test.tsx` es una prueba temporal solo de la copia (no versionada).
- Go con PostgreSQL (clúster temporal 55499, iniciado y detenido por V0): `iter_snap.sh ./internal/administration` →
  `ok … internal/administration 54.111s` (incluye la persistencia del tema, `17_preferences_and_18_terminal_keyboard`).

### Decisiones del coordinador tras la Fase 0 (ninguna cambia §0-B)

1. **Inicio por terminal**: migración única `0020_terminal_home_and_printer_columns.sql` con
   `devices.home_screen` (`tables` por omisión; lista cerrada Mesas, Para llevar, Cocina, Caja), editable en
   Administración → Sistema → Terminales con el permiso existente de administrar terminales. La terminal manda; si el
   usuario no puede ver esa sección, abre la primera que la barra le permite; nunca «Estado del sistema». «Pantalla
   inicial» sale de Mis preferencias; Go conserva `user_preferences.home_screen` por compatibilidad y la aplicación deja
   de usarla.
2. **Apariencia para usuarios nuevos**: Go copia la apariencia de Configuración general al crear un usuario y la usa como
   respaldo al leer sus preferencias, para que la etiqueta nueva sea cierta (sin migración ni permisos).
3. **§5.1**: dos reportes nuevos en el registro cerrado de la 9A —«Cuentas anuladas» y «Modificaciones de cantidades»—
   con los datos que existen (quién, terminal, motivo, fecha, cantidades e importes de las unidades canceladas); **sin
   columna «Autorizó»**, porque AD-EL POS no pide autorización de otra persona para esas acciones (no se inventa).
   «Recibió» ya existe en «Compras recibidas por periodo»; la referencia del pago con tarjeta ya es columna opcional de
   «Detalle de cobros» y se agrega donde falte en los reportes de devoluciones y correcciones.
4. **Selección múltiple**: el selector nuevo la admite y sustituye a las casillas de `ScopeSelector` (promociones); el
   editor de permisos y los días siguen como están.
5. **«Cuentas» y «Áreas y distribución»** siguen como pestañas dentro de Mesas. El mosaico **Cobros** se muestra con los
   permisos de administración de cobros (métodos, configuración e IVA); el turno actual sigue en Caja. No cambia ningún
   permiso, solo qué mosaicos ve cada rol.
6. **Nombres parecidos a «Registro de movimientos»** («Caja → Movimientos», «Inventarios → Movimientos», reporte
   «Movimientos de efectivo») se conservan: el contexto los distingue; se documenta.
7. **Columnas por impresora**: `kitchen_printers.columns` en la misma 0020, con NULL = predeterminado del papel (42 u 30)
   y CHECK de lista cerrada por papel (42/48 en 80 mm, 30/32 en 58 mm). El ancho entra en la fotografía de la impresora, de
   modo que la reimpresión de comandas sale con las columnas originales.
8. **`schema_version` 20** desde el inicio de la fase A (la versión 0.9.2 se fija al final). Verificado: la 0020 se aplica
   sobre una instalación con datos (`internal/database` en el clúster temporal: `ok`).

## Fase A · Barra superior, Administración y navegación

Hecha el 11/11/2026 sobre `etapa-9c` en una copia aislada, por un agente sin subagentes, en paralelo con la parte D1. Sin cambios de reglas de negocio, cálculos, permisos ni rutas nuevas; la única escritura
nueva es el campo opcional `home_screen` del recurso de terminales existente (migración 0020 del
coordinador, sin editarla).

### Qué cambió

- **Registro de secciones** (`desktop/src/navigation/sections.ts`): una sola fuente con id, nombre,
  icono, grupo (operación, administración, personal), pantalla padre (mosaicos y ruta), descripción
  del mosaico, regla de permisos (la de `screens`/`screenAuthorized`, movida aquí; Para llevar exige
  además `accounts.read` en el registro) y la clave de inicio de la terminal. Funciones
  `screenAuthorized`, `barSections`, `tilesOf`, `breadcrumbOf`, `barHighlight` y `resolveHome`. La
  navegación compartida (`NavigationContext`) expone el único `requestScreen` de App.tsx, que respeta
  `useLeaveGuard`; ahora también lo usan Inventarios → Configuración, «Ir a Caja», el logotipo, los
  mosaicos, la ruta y el menú del usuario.
- **Barra superior fija** (`navigation/TopBar.tsx`) de una fila con botones de 52 px: logotipo
  (vuelve al inicio de la terminal), Mesas, Para llevar, Cocina, Caja (indicador verde, ámbar o gris y
  el menú rápido de la 7.1 con sus permisos, «Abrir turno» incluido; el gancho y la hoja subieron a
  App.tsx), Administración (solo con alguna opción de administración), estado del servidor (en línea,
  reconectando, fuera de línea; en 1280 px un punto con descripción accesible y texto visible si no
  está en línea), menú del usuario (nombre y rol: Mis preferencias, Cambiar contraseña, Cerrar
  sesión) y pantalla completa (un botón de icono maximizar/reducir con el comportamiento de
  `WindowControls`; en el acceso y el asistente sigue fijo arriba a la derecha). Sin menú lateral ni
  navegación angosta. Iconos nuevos en `components/Icon.tsx`: mesa, cocina, engrane, usuario,
  usuarios, lupa, maximizar, reducir, mosaicos, etiqueta, documento, tarjeta, escudo, llave,
  historial, flechas, ayuda, teclado y salir.
- **Captura** (§1.2): la barra superior se reduce a la de la captura: «‹ Mesas» o «‹ Para llevar», mesa
  y cuenta (o pedido), Caja, Actualizar, el usuario y, solo si no está en línea, el estado del
  servidor. Enviar a cocina y Cobrar no se movieron.
- **Mesas y Para llevar**: secciones de la barra que comparten la pantalla operativa (montada siempre,
  como antes). Dentro de Mesas siguen las pestañas Mesas, Cuentas y Áreas y distribución (decisión 5);
  Para llevar ya no muestra pestañas.
- **Administración por mosaicos** (`navigation/TileScreen.tsx`): Catálogo, Inventarios, Reportes,
  Documentos, Cobros (solo con `payments.methods.manage`, `payments.settings.manage` o
  `payments.tax.manage`), Personal (Usuarios, Roles, Sesiones) y Sistema (Configuración, Terminales,
  Cajas físicas, Registro de movimientos). Personal y Sistema abren una segunda pantalla de mosaicos.
  **Ruta** «‹ Administración › Sistema › Terminales» con cada pantalla anterior tocable; Catálogo,
  Inventarios y Reportes informan su pestaña o grupo como último tramo.
- **Inicio por terminal**: Go lee `devices.home_screen` en `auth/me` y en la lista de terminales y lo
  edita en `PATCH devices/{id}` con `devices.manage` (campo opcional, lista cerrada `tables`,
  `takeaway`, `kitchen`, `cash`, auditado en `device.updated`). Sistema → Terminales lo elige con un
  control segmentado y muestra la columna «Pantalla inicial». Al iniciar sesión y con el logotipo, la
  pantalla de la terminal; si el usuario no puede verla, la primera de la barra; nunca «Estado del
  sistema». «Pantalla inicial» salió de Mis preferencias (y de las preferencias de otro usuario); Go
  conserva `user_preferences.home_screen` y la aplicación reenvía el valor guardado sin usarlo.
- **Estado del sistema** fuera de la navegación: «Ver estado del sistema» en Sistema → Configuración,
  en lenguaje sencillo (servidor, base de datos, versión y qué hacer) con «Información para soporte»
  plegado solo con `settings.manage`; antes de iniciar sesión, «Ver estado del sistema» en el acceso
  con el mismo lenguaje y el bloque de soporte plegado.
- **«Auditoría» → «Registro de movimientos»**: mosaico, título, descripción visible del permiso
  (`audit.read` = «Consultar el registro de movimientos» en Go; la descripción guardada en la base no
  cambia), mensaje de Go de solo consulta y dos ayudas que la mencionaban. Clave, rutas y datos sin
  cambios.

### Decisiones y desviaciones

- Personal y Sistema como segunda pantalla de mosaicos (no pestañas): cada pantalla interna ya tiene
  título, acciones y, a veces, pestañas propias.
- «Información para soporte» del administrador = quien tiene `settings.manage` (la interfaz no conoce
  el código del rol). Antes de iniciar sesión se muestra plegado: no hay usuario y es la única vía de
  diagnóstico sin sesión.
- En la ruta, la pantalla actual y su pestaña son texto (`aria-current`), no botones.
- Pendiente fuera de mi alcance: «queda registrada en la auditoría» en
  `pages/documents/rerender.tsx` (archivo del alcance de D1).
- `preferences.go` no se tocó (la apariencia la cambió D1); `acceptance_test.go` solo para enlazar
  `testTerminalHome` (D1 agregó después su propia llamada en el mismo lugar).

- Correcciones encontradas por Playwright: con la captura abierta en segundo plano, su botón Caja (que
  comparte el estado del menú con el de la barra) ya no se monta mientras la operación está oculta,
  porque su «tocar fuera» cerraba el menú de la barra; y la recomendación de cambiar la contraseña
  inicial no se muestra durante la captura (dejaba la barra reducida debajo de los avisos).

### Pruebas de la fase A (copia aislada del agente, 11/11/2026)

- Go: `go vet ./...` y `go vet -tags=integration` de `internal/administration` e
  `internal/database` sin hallazgos; `go test -race -count=1 ./internal/administration
./internal/database` → `ok` (8.4 s y 1.6 s; incluye `TestTerminalHomeIsAClosedList`).
- Integración en el clúster temporal 55499: `internal/administration` → `ok` (63.6 s), con
  `TestStageTwoAcceptance/9C_lectura_mesas_por_omision`, `9C_edicion_con_permiso_y_auditoria`,
  `9C_lista_cerrada_en_go_y_en_la_base` y `9C_sin_devices_manage_no_se_edita` aprobadas;
  `internal/database` → `ok` (6.6 s), con `TestStage9CTerminalHomeUpgrade` (la 0020 sobre un esquema
  19 con terminales e impresoras) aprobada.
- Frontend: `npx tsc --noEmit` sin errores; `npm test` → 63 archivos, 575 pruebas aprobadas (incluye
  `navigation/navigation.test.tsx` y los `App.test.tsx` y `WindowControls.test.tsx` actualizados);
  `npm run build` correcto.
- Playwright (Go temporal 8184, vista previa 1422): suite completa, Chromium 13/13 (5.2 min) y WebKit
  13/13 (5.1 min), con `workflow-stage9c-navigation.spec.ts` nuevo y los 12 recorridos anteriores y
  sus ayudantes (`e2e/navigation.ts` nuevo; `cash-flow.ts`, `kitchen-flow.ts`, `payments-flow.ts`,
  `notices.ts`) en la barra superior. Capturas claro/oscuro a 1280 × 720 y 1920 × 1080 de la barra
  con Mesas, Administración, Catálogo con su ruta, Estado del sistema y la captura reducida.
- Sin Rust: no se tocó `src-tauri`.

### Limitaciones

- Sin verificar en la ventana nativa (WKWebView), con pantalla táctil física ni con el teclado
  virtual sobre la barra superior. El turno de Caja se consulta ahora cada 15 s en todas las
  pantallas (antes solo en la operación).
- Debajo de 1280 px (fuera del diseño mínimo) la barra se compacta; no se probó por debajo de 1024 px
  fuera de la captura.
- Los módulos con pestañas propias que no informan su pestaña a la ruta: Documentos (alcance de D1),
  Caja (no es de Administración), Cobros y Configuración (acordeones).

### Cierre de la fase A

Sobre una copia exacta del commit `e575ad0` (`git archive`, clúster temporal 55499), sin los cambios de D1: Prettier y
gofmt limpios; `go vet` con y sin `-tags=integration` sin hallazgos; `go test -race` de `administration` y `database`
`ok`; integración `administration` (59.1 s) y `database` (6.6 s) `ok`; `tsc --noEmit` sin errores; `npm test` (63
archivos, 567 pruebas) y `npm run build` correctos; Playwright completo Chromium 13/13 (5.2 min) y WebKit 13/13
(5.1 min).

## Fase D (parte D1, en paralelo con la fase A) · Pendientes de backend

Hecha por un solo agente (D1) el 11/11/2026, en paralelo con la fase A, sin subagentes. Sin migración nueva: usa la
0020 tal como está (`kitchen_printers.columns`, NULL = predeterminado del papel). Ningún permiso, lock ni regla de
negocio nuevos.

### §5.1 · reportes del sistema anterior

- **«Cuentas anuladas»** (`sales.voided_accounts`, grupo Ventas, `reports.read` + `reports.sales.read`, como
  «Productos cancelados»): una fila por cuenta anulada con motivo (`account.void`) o liberada vacía
  (`account.released`, «Cuenta vacía liberada»): fecha de anulación, folio, tipo, quién anuló, terminal, motivo,
  modalidad, mesa u origen, área, unidades canceladas e importe de esas unidades (opcionales: nombre de la cuenta,
  apertura y quién abrió). Filtros: tipo, modalidad, área y usuario. Fecha: la de la anulación con la hora de inicio
  del día.
- **«Modificaciones de cantidades»** (`products.quantity_changes`, grupo Productos, mismo permiso): una fila por
  cancelación (`item.cancelled`) o modificación que retiró unidades (`item.modified` con `cancelled_unit_ids`) de una
  partida ya confirmada: fecha, folio, producto, cambio («Cantidad reducida» o «Partida cancelada»), cantidad anterior y
  nueva (sin total), diferencia en unidades e importe (con signo negativo y total), quién y motivo (opcionales: acción,
  terminal, modalidad, mesa u origen y unidades ya liberadas a cocina). Filtros: producto, modalidad y usuario. La
  cantidad anterior de una modificación sale de su evento; la de una cancelación, de la respuesta idempotente de esa
  misma operación (la cuenta como quedó) más lo retirado; si faltara, queda vacía (no se inventa).
- **Sin columna «Autorizó»** en ninguno: AD-EL POS no pide autorización de otra persona para esas acciones (Fase 0,
  decisión 3); ambos lo dicen en una nota.
- «Recibió» ya existía en «Compras recibidas por periodo» (comprobado por su etiqueta). La **referencia del
  comprobante** es columna opcional nueva en «Devoluciones» («Referencia» y «Referencia del pago original») y en
  «Correcciones de pagos» («Referencia original» y «Referencia correcta»); «Detalle de cobros» ya la tenía.
- Registro de 92 a **94 reportes**; ambos en `EXPORTABLE_REPORTS` (`export.rs`). Sin filtros nuevos: `api.rs` no cambió
  su lista blanca. Transacción de solo lectura del ejecutor, sin locks.
- **Índice: no hace falta.** Medido con un año sintético (607 214 eventos de restaurante, 667 MB; 1 038 cuentas
  anuladas y 5 201 cancelaciones en el año): «Cuentas anuladas» 21 ms y «Modificaciones de cantidades» 141 ms (mejor de
  3, ejecutor completo); con un índice parcial candidato sobre `restaurant_events(created_at)` para esos tipos, 20 ms y
  141 ms: no lo usan, porque parten de las cuentas anuladas y de las unidades canceladas (índice de la 0017) y leen el
  evento por el índice de la cuenta. La 0020 no se tocó.

### §5.2 · imágenes del catálogo

`logoraster.CheckJPEGMarkers` exporta el recorrido de marcadores (100 barridos y 1024 segmentos) con errores neutros
(`ErrJPEGTooManyScans`, `ErrJPEGTooManySegments`); el logotipo conserva sus mensajes. `catalog/images.go` lo llama antes
de leer el encabezado o decodificar, con mensajes propios («La imagen JPEG tiene demasiados barridos progresivos
(máximo 100)…»). El JPEG progresivo de 1 MiB con unos 61 000 barridos se rechaza al instante (prueba unitaria < 500 ms
y aceptación con PostgreSQL < 2 s); el máximo exacto se acepta y se guarda.

### §5.3 · columnas por impresora

- `kitchen.Printer.columns` (null = predeterminado): lista cerrada en Go (80 mm: 42 o 48; 58 mm: 30 o 32) y en
  PostgreSQL (CHECK de la 0020); el predeterminado se guarda como NULL; cambiar el papel sin elegir columnas válidas se
  rechaza con «Con papel de 58 mm las columnas de texto son 30 (predeterminado) o 32; 48 no es una opción. Si cambiaste
  el papel, elige de nuevo las columnas.» (la interfaz las devuelve al predeterminado al cambiar el papel). Requiere
  `kitchen.printers.manage` y queda en `printer.saved` como cualquier cambio de impresora.
- El renderizador único usa el papel y las columnas de la impresora destino (`Printer.RenderOptions`; texto grande de
  Cocina a la mitad; el logotipo sigue por puntos): comandas, prueba técnica, muestras del diseñador, precuentas,
  tickets, comprobantes y cortes (impresora de tickets de Cobros). La **reimpresión** de comandas usa la fotografía del
  trabajo original (las anteriores a la 9C no tienen el campo: predeterminado); los comerciales y de caja copian su
  texto. **«Imprimir con la plantilla vigente»** usa ahora la configuración actual de la impresora del original.
- **CUPS en texto**: con el predeterminado los argumentos de `lp` no cambian (cpi 12 o 6); con otras columnas, cpi en
  proporción (12 × columnas ÷ predeterminado: 13.71 para 48 y 12.8 para 32; 6.86 y 6.4 con el texto grande). Que la
  impresora respete esa densidad es **prueba física pendiente**. ESC/POS por red y CUPS raw no cambian (fuente A: 48 =
  576 puntos, 32 = 384).
- **«Imprimir prueba de ancho»** junto a la opción en Cocina → Impresoras (`POST kitchen/printers/{id}/width-test`,
  `kitchen.printers.manage`, operación cerrada `kitchen_printer_width_test`): regla numerada con decenas, línea completa
  de guiones y renglón «Producto de ejemplo … $1,234.50», entre marcas «*** PRUEBA DE ANCHO ***», con las columnas
  elegidas aunque no se hayan guardado, en un ticket `test` por la cola de siempre; sin ventas, pagos ni movimientos;
  su copia desde la cola repite el texto y se audita (`print.width_test_requested`).
- **Vista previa del diseñador**: además de 58/80 mm, «Ver con el ancho de una impresora» (impresoras activas de
  `documents/printers`, que ahora informa sus columnas y `default_columns`); `documents/preview` acepta `printer_id` y
  responde `printer_id`, `printer_name` y las columnas usadas.
- **Equivalencia de la 9B intacta con el predeterminado**: `TestDefaultKitchenTemplatesMatchLegacyRenderer`,
  `TestPlainLayoutKeepsTransportBytesAndArguments`, `TestPlainLayoutKeepsESCPosBytesOverTCP`,
  `TestDefaultCommercialTemplatesMatchLegacyRenderer`, `TestDefaultCashTemplatesMatchLegacyRenderer` y la cadena 9B de
  `internal/operations` aprobadas; nuevas: salida a 48 y 32 en los diez tipos, valor fuera de la lista rechazado (Go y
  PostgreSQL), reimpresión exacta tras cambiar las columnas y prueba de ancho con conteos antes y después.

### §4.4 · apariencia de usuarios nuevos (solo Go)

Al crear un usuario, Go copia la apariencia de Configuración general a sus preferencias; cambiarla después no altera a
los existentes; un usuario sin preferencias guardadas la usa como respaldo al leerlas (también `auth/me`). La etiqueta
de la pantalla la cambia la parte D2.

### Pruebas ejecutadas (copia aislada, clúster temporal 55499)

- `go vet ./...` y `go vet -tags=integration ./...`: sin hallazgos. `go test -race` de `reports`, `catalog`,
  `logoraster`, `kitchen`, `documents`, `designer`, `payments` y `administration`: `ok`.
- Integración: `internal/reports` (incluye `TestStage9CCancellationReports` y las exportaciones), `internal/reports/operational`,
  `internal/catalog`, `internal/designer`, `internal/database`, `internal/administration` y `internal/operations`
  (`TestRestaurantAcceptance` con los cuatro subtests 9C): `ok`.
- Rust `cargo test --locked --offline`: 34 aprobadas (una nueva). Vitest completo: 63 archivos, 575 pruebas; `npx tsc
--noEmit` y `npm run build`: sin errores. Playwright (Chromium, puertos 8185/1423):
  `workflow-stage9b-designer.spec.ts` aprobado con la vista previa nueva.

### Limitaciones

- El respaldo automático de impresora no vuelve a renderizar (como ya ocurría con el papel): si el respaldo tiene
  otras columnas, el texto conserva las del original y CUPS usa la densidad del respaldo.
- La vista previa de una comanda en la cola y la precuenta en pantalla siguen a 80 mm y 42 columnas (como antes).
- Las cuentas anuladas muestran la mesa y el área actuales de la cuenta.
- La densidad fraccionaria de CUPS (13.71, 12.8) y 48/32 columnas en papel: prueba física pendiente; la prueba de ancho
  existe para comprobarlo antes de elegir.
- La lista de impresoras de la vista previa es un `<select>` (la fase B lo sustituye por el selector nuevo).

## Fase C · Teclado, menú nativo y paleta de comandos

Hecha el 11/11/2026 sobre `etapa-9c` (fase A confirmada en `e575ad0`), por un agente sin subagentes, en
paralelo con la parte D1 y con la fase B ya en curso. Sin cambios de reglas de negocio, cálculos,
permisos, rutas de Go ni migraciones: el menú, los atajos y la paleta ejecutan las mismas funciones
que los botones de la interfaz.

### Qué cambió

- **Registro de acciones** (`desktop/src/commands/actions.ts`), sobre el registro de secciones de la
  fase A: 23 acciones con identificador, nombre, grupo de menú (Archivo, Edición, Ver, Caja, Ayuda),
  sinónimos para la paleta, atajo por plataforma, visibilidad por permisos (las reglas de la
  interfaz), habilitación por contexto con su explicación y ejecución = la función del botón:
  navegar con el único `requestScreen` de App (que ahora devuelve si se navegó y sigue respetando
  «Salir sin guardar»), abrir la hoja de Caja con su opción (`openSheet` del menú rápido de la 7.1),
  «Capturar»/«Nueva cuenta» de la mesa, «Cuenta sin mesa», «Nuevo pedido», «Precuenta», «‹ Mesas»,
  el tema de Mis preferencias (`services/preferences.ts`, la misma operación que su formulario),
  cerrar sesión y el cambio de pantalla completa del botón de la barra
  (`setWindowFullscreen`/`toggleWindowFullscreen` de `WindowControls`). La paleta agrega las
  pantallas de Administración (con su ruta y sinónimos) y las mesas del mapa.
- **Despachador único** (`commands/dispatcher.ts`): menú, teclado y paleta pasan por él. Una misma
  acción que llega por la otra vía dentro de 700 ms se descarta una sola vez (la gemela del
  acelerador del menú nativo y la pulsación en el WebView); una pulsación que no actuó (mientras se
  escribe o con un diálogo abierto) también consume su gemela del menú; una acción asíncrona en
  curso no se vuelve a ejecutar.
- **Menú nativo en Rust** (`src-tauri/src/menu.rs`, registrado en `main.rs` con `Builder::menu` y
  `on_menu_event`): «AD-EL POS» solo en macOS (Acerca de, Ocultar, Ocultar otras, Mostrar todas,
  Salir ⌘Q), Archivo (Nueva cuenta en mesa, Cuenta sin mesa, Nuevo pedido para llevar, Imprimir
  precuenta, Cerrar sesión, Salir), Edición con los elementos predefinidos (deshacer, rehacer,
  cortar, copiar, pegar, seleccionar todo) y «Buscar en la pantalla», Ver (Mesas, Para llevar,
  Cocina, Caja, Administración, Paleta de comandos, Pantalla completa, Tema claro u oscuro), Caja
  (Ingreso, Retiro, Gasto, Corte parcial, Abrir cajón, Abrir turno, Ir a Caja) y Ayuda (Atajos de
  teclado, Acerca de AD-EL POS). Sin submenú Ventana ni ⌘W. Aceleradores junto a cada opción.
  «Salir» lo atiende Rust (`app.exit(0)`): cierra la aplicación sin detener Go ni PostgreSQL.
- **Dos operaciones cerradas nuevas del puente**: `menu_subscribe(channel)` (eventos
  `{ action }` por un `Channel`, una sola suscripción vigente) y `menu_set_state(enabled)` (React
  informa las opciones habilitadas; un identificador fuera de la lista cerrada `MENU_ACTIONS`
  rechaza el estado). En `build.rs`, `permissions/diagnostics.toml` (`allow-diagnostics`),
  `generate_handler!`, con pruebas Rust y simuladas en `e2e/native-driver.ts`, que lee la misma
  lista `MENU_ACTIONS` de `menu.rs` y ofrece `clickMenu(id)`/`menuState()` a las pruebas. Sin
  permisos `core:*` nuevos.
- **Atajos** (⌘ en macOS, Ctrl en Windows): 1–4 Mesas, Para llevar, Cocina y Caja; 5
  Administración; K paleta; F buscar en la pantalla; N nueva cuenta en mesa; P precuenta; ⌃⌘F / F11
  pantalla completa; Esc cerrar o volver un nivel. No actúan mientras se escribe en un campo (salvo
  Esc) ni con un diálogo, menú o lista abiertos; la pulsación se consume para que el navegador
  (buscar, imprimir, ventana nueva) y el acelerador nativo no actúen. Ninguna acción financiera tiene
  atajo: las de Caja solo abren su hoja (que registra con su confirmación de siempre) y «Imprimir
  precuenta» abre la precuenta, cuyo botón «Imprimir precuenta» sigue siendo el que imprime.
- **Escape**: cierra el panel abierto de la cuenta (como su «Volver»), sale de la captura (como
  «‹ Mesas»), cierra el panel lateral de la mesa y, en Administración, vuelve a la pantalla
  anterior de la ruta. Los diálogos, la hoja y el menú rápido de Caja, «Más…», el menú del usuario,
  el selector y los controles propios (gráficos) conservan su Escape; el teclado del cobro conserva
  «Escape limpia el importe»; el primer Escape en un campo solo sale del campo.
- **⌘F / Ctrl+F**: el buscador de la pantalla visible (productos en la captura, que se marcó con
  `data-screen-search`; cuentas; tablas de Administración); en el mapa de mesas, «Buscar mesa» en
  la paleta; en una pantalla sin buscador, la paleta.
- **Paleta de comandos** (`commands/CommandPalette.tsx`): diálogo modal con el patrón ARIA combobox
  - listbox (`aria-activedescendant`), flechas, Re Pág/Av Pág, Ctrl+Inicio/Fin, Intro, Escape y
    ⌘K/Ctrl+K para cerrar; búsqueda sin acentos por palabras al inicio del nombre o de sus sinónimos
    («productos» → Catálogo, «retiro», «corte», «mesa 5» sin «Mesa 15»); filtrada por permisos y
    contexto (lo que el contexto impide se ve deshabilitado con su motivo); opciones de 48 px.
- **«Atajos de teclado»** y **«Acerca de AD-EL POS»** (`commands/HelpDialogs.tsx`) desde Ayuda y la
  paleta, también antes de iniciar sesión. Los atajos se muestran en el menú, en la paleta y en las
  descripciones (`title` y `aria-keyshortcuts`) de los botones de la barra, de Caja, de
  «Precuenta», del buscador de productos y de pantalla completa (antes fijo «Control + ⌘ + F»).

### Decisiones y desviaciones

- ⌘5 → Administración, ⌘N → Nueva cuenta en mesa y ⌘P → Imprimir precuenta se agregaron a la
  propuesta mínima (no chocan con nada existente). «Cuenta sin mesa» y «Nuevo pedido para llevar»
  crean la cuenta al instante como sus botones, por eso no tienen atajo (solo menú y paleta).
- «Nueva cuenta en mesa» actúa sobre la mesa del panel lateral o de la captura de una mesa; sin
  mesa elegida abre el mapa y lo explica con un aviso.
- «Abrir turno» se agregó al menú Caja (es una opción del menú rápido de la 7.1). «Acerca de»
  está en el menú de la aplicación de macOS y en Ayuda; «Salir», en el menú de la aplicación (⌘Q)
  y en Archivo.
- ⌘1/⌘2 dentro de la captura de esa sección equivalen a «‹ Mesas» / «‹ Para llevar»; en el cobro
  solo navegan (no lo cierran).
- En Administración, Escape no sale de una pantalla con un formulario a la vista, para no
  descartar lo capturado (los formularios no tienen protección de cambios sin guardar); Escape no
  cierra la personalización de la captura (su «Volver» confirma el descarte).
- Archivos de otros alcances con cambios mínimos: `pages/PreferencesPage.tsx` (el guardado usa
  `savePreferences`, sobre el cambio de la fase B), `styles.css` (bloque al final),
  `pages/cash/CashShortcut.tsx`, `pages/restaurant/TablePanel.tsx`,
  `pages/restaurant/capture/BillFooter.tsx` y `ProductGrid.tsx` (descripciones con el atajo) y
  `pages/restaurant/OperationalPage.tsx` (informa su contexto y entrega sus botones).
- La especificación nueva se llama `workflow-stage9c-keyboard.spec.ts` (nombre del encargo): se
  ordena antes de `workflow-stage9c-navigation.spec.ts`, después de todas las demás; usa su propia
  área y mesa («Terraza teclado», «Mesa 5») y la suite completa comprueba que no interfiere.

### Pruebas de la fase C (copia aislada: `HEAD` de `etapa-9c` + solo los archivos de la fase C, 11/11/2026)

La copia aislada se armó con `git archive HEAD` (`e575ad0`) más solo los archivos de la fase C
(en `PreferencesPage.tsx` y `styles.css`, la versión de HEAD con el cambio de C), para no mezclar el
trabajo en curso de B y D1.

- Rust: `cargo fmt --check` sin diferencias; `cargo test --locked --offline` → 40 aprobadas,
  1 ignorada (Keychain real), con las 6 nuevas de `menu::tests`.
- Frontend: `npx tsc --noEmit` sin errores; `npm test` → 65 archivos, 602 pruebas aprobadas
  (incluye `commands/commands.test.ts` con 15, `commands/commandCenter.test.tsx` con 13 y los
  `WindowControls.test.tsx` y `navigation.test.tsx` actualizados); `npm run build` correcto (con el
  aviso de tamaño de bloque que ya existía).
- Playwright (Go temporal 8187, vista previa 1425), suite completa: Chromium 14/14 (5.3 min) y
  WebKit 14/14 (5.2 min), con `workflow-stage9c-keyboard.spec.ts` nuevo y las 13 anteriores sin
  cambios (todas usan el puente con la simulación del menú). Después se agregó a la especificación
  nueva la comprobación sin desbordamiento de la paleta y de «Atajos de teclado» a 1280 × 720 y
  1920 × 1080 en ambos temas, con capturas: Chromium y WebKit aprobadas (6.2 s y 5.8 s).
- En el árbol compartido (con la fase B a medias) Vitest falla en 18 archivos ajenos a esta fase
  (selectores reemplazados por la fase B y vista previa de impresoras de D1); con solo los archivos
  de C, todo aprueba.

### Limitaciones

- Sin verificar en la ventana nativa (WKWebView): el menú real de macOS, si `preventDefault` en el
  WebView evita el acelerador del menú (el despachador evita la doble ejecución en ambos casos),
  los elementos que macOS agrega por su cuenta (Dictado y Emoji en Edición; posiblemente «Entrar en
  pantalla completa» en Ver) y el menú en pantalla completa (macOS lo muestra al acercar el puntero
  arriba; por eso existen los atajos y la paleta). El menú de Windows no se compiló ni probó (solo
  su estructura en las pruebas Rust); en Windows los aceleradores pueden no llegar al menú con el
  foco en el WebView, y entonces los atiende la capa de teclado de React.
- «Salir» no pregunta por cambios sin guardar (como cerrar la ventana antes).
- Un menú nativo no muestra el motivo de una opción deshabilitada; la paleta sí.
- Sin pantalla táctil física ni teclado virtual sobre la paleta.

La fase C se confirmó antes que la B (que seguía en curso) con sus archivos propios; en `PreferencesPage.tsx` y `styles.css`, compartidos con la B, solo entró su parte. Se probó sobre la fase A más sus archivos (sin la parte D1 ni la B), y la integración conjunta se comprueba al cerrar la fase B y en la regresión final.

### Cierre de la fase C

Sobre una copia exacta de `1feaa59` (fases A y C y parte D1 juntas, sin la B): Prettier, gofmt y `cargo fmt --check`
limpios; `go vet` con y sin `-tags=integration` sin hallazgos; `tsc --noEmit` sin errores; `npm test` (65 archivos, 602
pruebas) y `npm run build` correctos; `cargo test` 40 aprobadas y 1 ignorada; Playwright WebKit 14/14 (5.2 min). En
Chromium falló la especificación del diseñador de la 9B al cerrar los avisos antes de sus capturas (un aviso de éxito se
cerró solo durante el clic y el clic se reintentó hasta agotar el tiempo); las tres especificaciones siguientes fallaron en
cascada porque Playwright reinicia el proceso y pierde la terminal preparada. Se corrigió el ayudante de la prueba (el clic
tolera que el aviso desaparezca y el bucle tiene tope) y se repitió Chromium sobre la copia exacta de `a3f6534`: 14/14
(5.8 min).

## Fase B · Selector moderno para listas desplegables

Hecha por un solo agente (B) el 11/11/2026 sobre `etapa-9c` (después de la fase A y de la parte D1), sin dependencias
nuevas, sin migraciones y sin cambios en Go, Rust, permisos ni reglas de negocio. En paralelo trabajó el agente C (teclado,
menú nativo y paleta) sin archivos compartidos salvo `styles.css` (bloques distintos).

### Qué cambió

- **Componente único propio** `components/Select.tsx` (`Select` y `MultiSelect`; partes en `SelectPopup.tsx`,
  `selectHooks.ts` y la lógica pura en `selectModel.ts`), patrón ARIA _combobox_ con lista emergente: disparador
  `<button role="combobox">` de 44 px con el aspecto de los demás campos, nombre tomado del `<label>` de `Forms.Field`
  (la indicación queda como descripción) y lista en un portal con posición fija que nunca se corta (acordeones, áreas con
  desplazamiento, borde de la ventana, teclado virtual). Opciones de 44 px, grupos (`optgroup`), opción vacía, opciones
  deshabilitadas, **opciones de acción** al final y siempre visibles («+ Nueva categoría» de la 9A y «Otra unidad o
  presentación…» del conteo ya no son valores especiales), **búsqueda automática con más de 8 opciones** sin distinguir
  mayúsculas ni acentos (cada palabra en cualquier orden) con anuncio de resultados, y **selección múltiple** con marcas,
  «Listo» y etiquetas para quitar lo elegido con un toque. Temas claro y oscuro con los tokens de `styles.css`.
- **Teclado completo**: flechas, Inicio/Fin, Re Pág/Av Pág, Intro, Espacio, Esc (cierra solo la lista y no llega al diálogo
  ni a los atajos de la fase C), Alt+↓/↑, Tab (cierra sin cambiar y avanza al siguiente campo) y escribir para buscar (con
  búsqueda va al campo; sin ella, a la opción que empieza así).
- **Teclado virtual**: con el dedo la búsqueda no se enfoca sola; al tocarla aparece el teclado virtual, se escribe con él y
  la lista no se cierra al tocarlo; la lista se coloca por encima de él.
- **Formularios**: con `required` o `name`, un campo oculto participa en la validación nativa (el acordeón cerrado se
  revela y el foco vuelve al disparador) y en el envío; `Form` («Guardar y crear otra»), `ModalPanel` y `ConfirmDialog`
  reconocen el selector como campo.
- **Sustituidos los 104 `<select>`** que quedaban (el de la navegación angosta lo retiró la fase A), en 55 archivos,
  incluidos los compartidos `ReferenceField` (30 usos), `PositionField` (9), `ItemPicker`, `SupplierPicker`,
  `WarehouseSelect`, `DimensionUnitSelect`, `MeasureSelect`, `RecipeItemPicker`, `PreparationPicker`,
  `CatalogIngredientPicker`, los filtros de Reportes y el selector de impresora de la vista previa del diseñador (D1).
  No queda ningún `<select>`, `<option>` ni `<optgroup>` en la interfaz. Las tasas de IVA siguen como botones cerrados.
- **Selección múltiple** en lugar de las casillas de `ScopeSelector` de promociones (productos, categorías y listas;
  decisión 4 de la Fase 0). Estaciones de rutas y terminales, el editor de permisos y los días no cambian.
- **Búsquedas**: se retiraron los campos de búsqueda aparte de la zona horaria (ahora con grupos «México» y «Otras zonas»)
  y de los selectores de insumo, proveedor, preparación e ingrediente del catálogo; la búsqueda integrada la sigue
  resolviendo Go con los mismos criterios (nombre, código, código de barras, RFC, contacto…) y conserva el nombre del campo
  anterior. Se conserva «Buscar producto/insumo» de los filtros de Reportes, que consulta a Go solo al pulsar «Buscar»
  (regla de la 9A: nunca por tecla); su selector filtra en pantalla las coincidencias ya recibidas.
- **Pruebas**: ayudante único de Playwright `e2e/select.ts` (elegir por etiqueta, por valor o con búsqueda; comprobar valor,
  texto y opciones) usado por todas las especificaciones y ayudantes que usaban `selectOption`, `locator('option')` o
  `toHaveValue` sobre una lista; ayudante de Vitest `src/testing/select.ts`, con el que se actualizaron las pruebas de las
  pantallas que cambian.

### Decisiones y desviaciones

- Sin librería: el componente se construyó con React y los tokens existentes (ninguna dependencia nueva que registrar en
  `docs/DEPENDENCIES.md`).
- En los selectores obligatorios, «Selecciona…» o «Elige …» es el texto vacío del disparador y ya no una opción elegible;
  en los opcionales y en los filtros («Sin asignar», «Todos», «Sin filtro»…) sigue siendo una opción.
- Tab cierra sin cambiar el valor (como Select2 por omisión) para evitar cambios accidentales; Intro y Espacio eligen.
- Al cerrar la lista se vacía la búsqueda; en las búsquedas de Go se vuelve a pedir la primera página. Con la búsqueda en Go
  en curso, Intro no elige nada (un lector de códigos que termina con Intro no toma un resultado anterior).
- El valor elegido queda en el atributo `value` del disparador y cada opción lleva `data-value`, para que las pruebas
  comprueben y elijan por valor como antes.

### Pruebas de la fase B (copia aislada del agente, 11/11/2026)

- `npx tsc --noEmit` → sin errores.
- `npm test` → 66 archivos, 618 pruebas aprobadas (incluye `components/Select.test.tsx`, 15 pruebas: nombre por etiqueta,
  ratón, teclado completo, búsqueda sin acentos con anuncio, grupos, acciones, validación con acordeón, deshabilitado,
  múltiple con etiquetas, búsqueda en Go, teclado virtual, apertura con el dedo, sin `type="number"` ni `<select>`, posición
  dentro de la ventana y opción con valor vacío; y una prueba nueva de promociones con selección múltiple en
  `catalogFixes.test.tsx`).
- `npm run build` → compilación correcta.
- Playwright con la suite completa en Chromium y WebKit (puertos 8186/1424, clúster temporal 55499): 15/15 en cada motor —las
  14 especificaciones del repositorio más una especificación temporal de capturas que no se versiona—. Capturas del selector
  corto, con búsqueda (zona horaria) y múltiple (promociones), en claro y oscuro a 1280 × 720, en el directorio temporal de
  la sesión (`shots9cB`). Antes de llegar ahí, dos ejecuciones encontraron y permitieron corregir dos defectos propios: la
  opción con valor vacío («Todos») se mostraba como «Selecciona…» y, en WebKit, abrir con el ratón no dejaba el foco en el
  disparador (Esc no cerraba la lista).

### Limitaciones

- No verificado en pantalla táctil física, ventana nativa (WKWebView) ni con un lector de pantalla real (VoiceOver): lo táctil
  y la accesibilidad se comprobaron con Vitest, Testing Library y Playwright.
- La lista no se virtualiza (≈420 zonas se dibujan al abrir); no se midió en hardware de punto de venta.
- El aviso nativo de un selector obligatorio vacío es el de un campo de texto («Completa este campo»).

La fase B se confirmó después de la C (las dos corrieron a la vez); su copia aislada era idéntica al árbol de trabajo, que ya contenía las fases A y C y la parte D1.
