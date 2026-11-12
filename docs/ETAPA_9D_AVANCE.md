# Etapa 9D · Avance

Ajustes de interfaz, color de marca y cancelar cuenta. Especificación del propietario:
[docs/etapas/ETAPA_9D_v2.md](etapas/ETAPA_9D_v2.md) (§0-B prevalece). Rama local `etapa-9d`, creada desde `main`, que
contiene la 9C (`main` = `etapa-9c` = `origin/main` = `723ca7a`, versión 0.9.2, esquema 20, instalada). Límite del
propietario: nunca más de 2 agentes trabajando a la vez y sin subagentes de subagentes; Fase 0, revisión independiente y
regresión final, un agente cada una. **Regla de autonomía** del propietario (§0 de la especificación): si todo es correcto
y las pruebas pasan, se continúa de fase en fase, se aplican las migraciones a la instalación real con el procedimiento de
respaldo, se instala la nueva versión en la Mac de pruebas y, al terminar, se fusiona `etapa-9d` en `main` y se hace
push.

## Fase 0 · Verificación contra el código

Hecha por un solo agente (V0) el 12/11/2026 sobre `etapa-9d`, creada desde `main` (`main` = `etapa-9d` = `723ca7a`,
el último commit de la 9C: la 9C está en `main`). Versión 0.9.2 y esquema 20 (`config/product.json`). Solo lectura: no
se editó el repositorio. La especificación `docs/etapas/ETAPA_9D_v2.md` estaba sin seguimiento en git; se confirma con esta fase (Prettier no
la toca: `.prettierignore:15`).
Inventario por archivo y línea en el inventario técnico de la fase (directorio temporal de la sesión); los puntos 5 y 6
están además en [docs/NOTAS_WINDOWS.md](NOTAS_WINDOWS.md). Consultas de la instalación real: solo lectura (`transaction_read_only = on`), resultados guardados
junto al inventario técnico.

### Decisiones que deja la Fase 0 (ninguna obliga a cambiar §0-B; fijarlas en el diseño)

1. **Cuentas vacías #1 y #3 no son vacías para Go.** Cada una tiene **una partida de borrador sin enviar** (eventos
   `account.opened` y `draft.saved`, sin pedidos): «Liberar mesa» responde `ACCOUNT_NOT_EMPTY` y «Anular cuenta vacía»
   rechaza los borradores (reproducido en base temporal). Para cumplir §4.2 el descarte debe admitir cuentas con
   borrador pero sin unidades, pedidos, combinaciones, registros financieros ni de cocina, y borrar el borrador.
   Recomendación: ampliar `release` (o una ruta `discard` gemela) con esa definición, sin motivo, `accounts.open`.
2. **«Descartar» no debe aparecer en reportes (§4.2), pero el reporte «Cuentas anuladas» de la 9C incluye «Cuenta
   vacía liberada»** (`reports/cancellations.go:7-9, 29-33, 66, 75`). Recomendación: «Cuentas canceladas» excluye
   `account.released` (y el descarte nuevo); conserva las anulaciones antiguas (`account.void`, motivo libre, «Autorizó»
   vacío). El filtro de la lista «Anuladas sin productos activos» (`AccountList.tsx:89`) también mezcla ambas.
3. **«Cuenta sin mesa» también crea la cuenta al instante** (`OperationalPage.tsx:213-222, 526-535`). §4.1 solo nombra
   Para llevar, pero §0-B dice «ninguna cuenta se guarda sin productos». Recomendación: la misma creación diferida para
   «Cuenta sin mesa» (mismo camino de código). Opción B: dejarla y anotarlo como excepción.
4. **«Cancelar cuenta» con descuentos o cargos.** `payments.RequireNoNetFinance` rechaza con `ACCOUNT_HAS_PAYMENTS`
   cualquier ajuste vigente, además de pagos y propinas (`payments/guards.go:93-107`), y las guardas H3 impiden
   cancelar unidades con descuento manual. Recomendación: igual que con los pagos parciales, el botón explica «quita
   primero el descuento o cargo» (no cambia reglas). Opción B: cancelar los ajustes en la misma operación (toca cobros;
   no recomendada).
5. **«Lo ya preparado queda como merma» (§5.4) frente a la clasificación de la etapa 8.** Hoy una unidad liberada y
   consumida que se cancela queda «Pendiente de clasificar»; la clasifica el encargado (`inventory.waste.record`) y el
   reconciliador aplica «No preparado» al corte (`inventory/cancellations.go:13-24`, `inventory/worker.go:363-398`).
   Sin pantalla de cocina las tareas nunca pasan a «Listo», así que «preparado» no se puede saber por la cocina. Con
   «Control de inventario» desactivado (instalación real) no hay consumo ni clasificación. Opciones: (a) sin cambio:
   pendiente de clasificar y el motivo visible en la lista; (b) Go preclasifica «Preparado / merma» las unidades ya
   liberadas cuando el motivo lo indica (columna del catálogo de motivos), corregible por el encargado hasta el corte.
   Recomendación: (b), porque §5.4 lo pide expresamente y las categorías y montos de la etapa 8 no cambian.
6. **Permiso propio.** `void` exige hoy `accounts.manage` («Editar referencias y anular cuentas sin partidas
   activas»). Recomendación: permiso nuevo `accounts.cancel` en Go y en la 0021, solo al administrador (en la instalación
   real solo el administrador tiene `accounts.manage` y `orders.cancel`: nadie pierde nada). La ruta no puede exigirlo
   (si no, no habría autorización de supervisor): debe exigir `accounts.read`/`accounts.open` y decidir con `approve`.
7. **Verde en paletas de datos.** La paleta de colores de categorías del catálogo (Verde, Lima, Esmeralda;
   `catalog/palette.go:20-22`) es dato del usuario y `--chart-3` (#1baf7a) es una serie de los gráficos de Reportes.
   Recomendación: conservarlos (codifican datos, no estado de la interfaz) y decirlo; opción B: sustituir `--chart-3`
   revalidando la paleta.
8. **Botón principal del panel (§3.3).** En el panel de la mesa el botón se llama «Capturar» y **ya es relleno** con el
   acento (`TablePanel.tsx:258-265, 344-351`); el que va sin color es «Capturar productos en esta cuenta» de Cuentas y
   Para llevar (`CaptureBill.tsx:200-207`, `secondary-button`). «Un solo botón principal» choca con «Cobrar · $…» y
   «Enviar a cocina» (`BillFooter.tsx:260-279`, ambos `primary-button`), que §4.6 conserva. Recomendación: fuera de la
   captura, «Capturar…» es el único relleno y «Cobrar» conserva lugar, texto e importe con el estilo de contorno que la
   9C ya usa en el panel de la mesa (`styles.css:4132-4136`); dentro de la captura «Enviar a cocina» sigue principal.
9. **Colores sin lugar en la paleta.** `--cash-entry` (verde: tarjetas de entradas de Caja y «Turno abierto»),
   `--cash-closing` (violeta, se confundiría con Uva = Pago parcial), `#bf7057` (antigüedad alta de Para llevar) y los
   avisos de cocina `#ba8b32`/`#b66443`. Recomendación: entradas y turno abierto en azul, cierre en gris neutro,
   antigüedad y avisos de cocina en ámbar/rojo con tokens.
10. **Textos.** La barra dice «Fuera de línea» (`serverState.ts:34`) y la paleta del propietario «Sin conexión»:
    recomendación, «Sin conexión». El mapa no tiene icono para «Libre» (`TableMap.tsx:31`) aunque §1.2 pide icono en
    cada estado: agregar uno.

### 1. Colores

- **Definición**: Tailwind 4 solo por `@import 'tailwindcss'` (`styles.css:1`; `@tailwindcss/vite` en
  `vite.config.ts:4,7`), sin `tailwind.config`, sin `@theme` ni variantes `dark:`. Tokens propios en `styles.css`:
  `:root` 94-163 (52 propiedades) y `:root[data-theme='dark']` 164-217 (51). 841 usos de `var(--…)`.
- **Acento verde**: `--accent #1b7568` / oscuro `#79ceba`, `--accent-soft #eaf5f1` / `#203b36`, `--banner #edf4f4` /
  `#203130`. Usos: `var(--accent)` 129, `--accent-soft` 46, `--banner` 8. Botón principal `.primary-button` 352-360
  (fondo acento, texto `--panel`); 60 reglas `color: var(--accent)` (lista en `inventory.md`).
- **Escritos directamente**: en CSS, **34 apariciones en 24 reglas** fuera de los tokens; verdes en 5 reglas:
  `.brand-mark` 246-258 (marca compacta de la barra bajo 1280 px), `.status-dot.good` 327-328, `.paper-segment-*`
  7152-7160 (`rgb(27 117 104)` y `#1b7568`). El resto: 11 sombras/fondos de diálogo neutros, `#bf7057` 1698, `#ba8b32`
  1934, `#b66443` 1938, rótulos de gráficos `#fff`/`#0b0b0b` 6395/6402 y el papel simulado del diseñador 7130-7204. En
  TSX solo **2 componentes**: `documents/identity/LogoSection.tsx:46,54` y `PaperPreview.tsx:39,51` (`bg-white`,
  `text-[#5b6675]`, `border-[#d7dce3]`: papel simulado). Ningún estilo en línea con color salvo el color de categoría
  (dato del catálogo: `CategoryColumn.tsx:34-38`, `ProductGrid.tsx:117`, `ColorField.tsx:20`). PDF y Excel de Reportes
  usan grises propios (`export_pdf.go:49-55`, `export_xlsx.go:119-140`), sin verde.
- **Estados de mesa**: `--table-*` 117-133 / 181-197: libre blanco/`#c5ced9`; **ocupada verde** `#d3ece2`/`#1b7568`;
  esperando pago ámbar `#f3c76f`/`#a86a12`; **pago parcial azul** `#8fb9e9`/`#245a99`; fuera de servicio gris rayado
  punteado. Aplicados en el mapa 1395-1424, la leyenda 1588-1611 y el chip del panel 4151-4176; selección
  `outline 3px var(--accent)` 1536-1539.
- **Avisos (Sonner)**: `AppToast.tsx` (iconos `success`/`info`/`alert`/`error` = palomita, «i», triángulo, equis, 7-12)
  y `styles.css` 4685-4712: éxito = `--accent` (verde), **información = `--info #2f6db3` (azul)**, advertencia
  `--warning`, error `--danger`. Hace falta un token turquesa para información.
- **Importes en verde**: `.quote-total` 918, `.checkout-selection-total` 2111, `.checkout-summary-balance dd` 2160,
  `.checkout-figure-emphasis dd` 2196, `.amount-field-value` 2366, `.capture-product-price` 3440,
  `.capture-grand-total dd` 3913, `.table-panel-balance dd` 4281.
- **Destructivos**: `.danger-button` 699-712 (texto rojo, contorno), `.danger-solid-button` 4805-4811 y
  `ConfirmDialog` con `tone: 'danger'` (29 usos). No usan rojo: «Cancelar unidades» (`BillLines.tsx:69-78`), su envío
  «Confirmar cancelación de unidades» en verde (`CancelItem.tsx:101`), «Anular cuenta vacía» (menú «Más…» sin variante
  de peligro, `MoreActions.tsx:62-65`, y envío de `Form` siempre `primary-button`, `Forms.tsx:75-77`), «Cancelar
  ajuste» (`AdjustmentList.tsx:183`), «Descartar selección/mis cambios/posiciones» y varios «Quitar»/«Borrar» (lista
  completa en `inventory.md`).
- **Cursor deshabilitado**: `button:disabled { cursor: wait; opacity: .55 }` en `styles.css:226-229` (por eso macOS
  muestra «cargando»). Excepciones ya con `not-allowed` (2241, 2445, 2492, 2779, 3602, 3666, 6725) o `default` (453,
  546, 3418, 7804). No hay `cursor: progress` en ningún lado.

### 2. Creación de cuentas

- **Mesa (creación diferida, 7.1)**: «Capturar»/«Nueva cuenta» → `startPendingCapture` (`useRestaurantWorkspace.ts:383-397`)
  sin escribir; el primer guardado del borrador llama `restaurant_open_account` con `items` y `seen_account_ids`
  (`useDraftSync.ts:30-46`, `draftSync.ts:425-445`) y Go crea cuenta y borrador en una operación
  (`accounts_write.go:154-191`; `TABLE_HAS_ACCOUNT` en 196-224). La cuenta pendiente solo existe para mesas
  (`pendingAccountBill` exige mesa, `restaurant.ts:267-307`; `PendingCreation.tableID`, `draftSync.ts:48-51`).
- **Para llevar y «Cuenta sin mesa»**: `openAccount` → `quickOpen` (`OperationalPage.tsx:216-222`,
  `useRestaurantWorkspace.ts:430-457`): `POST restaurant/accounts` sin partidas, cuenta abierta al instante. Go ya
  acepta `items` en cualquier modalidad (reproducido): la creación diferida de Para llevar es de React (variante sin
  mesa de la cuenta pendiente y del transporte). Riesgo: con la cuenta pendiente no hay «Datos de cuenta» (cliente,
  teléfono, programación) hasta guardar el primer producto (`BillFooter.tsx:56-59`).
- **Cuentas vacías en la instalación real** (8 abiertas: 4 de salón, 4 para llevar): #1 y #3 salón con mesa, 1 partida
  de borrador cada una, 3 días; #5 (3 días), #8, #9 y #10 (de hoy, posteriores a instalar la 0.9.2) para llevar,
  **vacías en sentido estricto**. Con productos: #2 (5 unidades) y #7 (1), ninguna liberada, sin pagos.
- **Operaciones para quitarlas hoy**: «Liberar mesa» (`POST …/release`, `accounts.open`, `accounts_release.go:48-87`)
  solo con vacía estricta (`accountEmpty` 15-33; si no, `ACCOUNT_NOT_EMPTY`); Go la admite también para Para llevar
  (reproducido), pero React solo la muestra en el panel de la mesa (`TablePanel.tsx:189-190, 385-394`). «Anular cuenta
  vacía» (`POST …/void`, `accounts.manage`, motivo libre 5-500, `accounts_write.go:307-316`) aparece en «Más…» si no hay
  unidades ni borrador (`BillFooter.tsx:46, 67-73`): sirve hoy para #5, #8, #9 y #10, no para #1 y #3.

### 3. «Anular cuenta» hoy

- **Ruta**: `POST restaurant/accounts/{id}/void` (`api.rs:364-367` `restaurant_void`), permiso `accounts.manage`
  (`operations/handler.go:200-206`; descripción en `context.go:50` y 0004:172). Condiciones: abierta y revisión vigente,
  **sin unidades activas ni borrador**, motivo de 5 a 500 caracteres, `RequireNoNetFinance` (sin pagos netos, propinas
  ni ajustes vigentes). Deja `state='voided'` (CHECK con nombre `restaurant_accounts_state_check`), evento
  `account.void` con el motivo en `payload.after.reason`, auditoría `restaurant.account.void` y evento en tiempo real
  (`queries.go:66-95`). La mesa queda libre (solo cuenta las abiertas).
- **Cocina e inventario**: sin efecto (no hay unidades activas). Las unidades se cancelan antes, una partida a la vez,
  con `POST …/cancel` (`orders.cancel`, motivo libre, `operations/cancellations.go:5-72`), que solo cancela unidades sin
  pago ni descuento (H3) y emite `item.cancelled`: cocina cancela una liberación aún no liberada sin papel o manda la
  comanda de CANCELACIÓN (`kitchen/changes.go:26-127`), e inventario registra la solicitud `cancel` si estaba consumida
  (`inventory/consumption.go:66-105`) → clasificación de la etapa 8.
- **Reportes (9C)**: «Cuentas anuladas» `sales.voided_accounts` (`reports/cancellations.go:40-99`, `reports.sales.read`,
  exportable `export.rs:48`): anuladas con motivo y vacías liberadas, «Anuló», terminal, unidades e importe cancelados.
  **Sin «Autorizó»** porque nadie autoriza: basta `accounts.manage` (nota en 16-18 y 25-27). Las unidades canceladas
  salen además en «Productos cancelados» (`reports/products.go:200-235`) y «Modificaciones de cantidades».
- **Registro de movimientos** muestra la clave cruda de la acción (`AuditPage.tsx:113`): «Cancelar cuenta» se verá ahí
  solo con una acción nueva (p. ej. `restaurant.account.cancelled`); las anteriores conservan `…void`.
- **Autorización de supervisor (descuentos)**: Go — `administration/supervisor.go:16-67` (usuario y contraseña en la
  misma transacción, límite de intentos del inicio de sesión, sin autoautorización, permisos efectivos actuales) se pasa
  solo a `payments.Request` (`routes.go:78-83`); `payments/authorization.go:46-88` (`approve` antes de escribir y
  `recordApproval` en `payment_authorizations`, 0006:197-209, CHECK `payment_authorizations_scope_check` con nombre);
  el plan del descuento devuelve `requires_authorization` (`adjustments.go:155-170, 232-251`). `operations.Request` no
  tiene autorizador (`operations/context.go:85-91`): hay que agregarlo. React — `useSupervisorAuthorization.ts:27-90`
  (estado, `take()` borra la contraseña, `handleError` con los cinco códigos) y `SupervisorAuthorization.tsx:6-58`; usos
  en `DiscountForm.tsx:215, 377` y `AdjustmentList.tsx:53, 94`.
- **Motivos de descuento**: tabla `discount_reasons` (0009:29-46), Go `payments/discount_reasons.go`
  (`payments.settings.manage`, orden por posiciones, `reason_name` congelado), UI Administración → Cobros →
  «Motivos de descuento» (`PaymentsAdminPage.tsx:115-130`, `DiscountReasonsAdmin.tsx`). Es el único catálogo de motivos.

### 4. «Pendiente de liberación · revisar cocina»

- **Origen**: `kitchen.VersionPreparation` devuelve `pending_release` si hay unidades activas sin fila en
  `kitchen_unit_releases` y ninguna liberación programada (`kitchen/states.go:184-195`); React lo traduce en
  `services/kitchen.ts:207` y lo muestra en el encabezado de la cuenta (`CaptureBill.tsx:187-191`), en **cada** renglón
  (`BillLines.tsx:37, 54`), en la tarjeta (`AccountList.tsx:124-127`) y en `AccountKitchen.tsx:33`.
- **Causa en la instalación real**: **0 estaciones, 0 rutas, 0 impresoras**, liberación automática desactivada. Al
  confirmar, `prepareRelease` encuentra «Sin ruta de preparación: producto» (`kitchen/routing.go:137-139`) y deja la
  liberación `blocked` sin unidades liberadas (`kitchen/releases.go:138-142`). Real: 4 liberaciones bloqueadas
  (#2 y #7 abiertas; #4 y #6 ya cerradas y pagadas), 23 advertencias, todas «Sin ruta de preparación», 15 unidades
  activas sin liberar, 0 tareas.
- **Reproducido** en base temporal (prueba solo en la copia): sin configuración → `blocked` y `pending_release` en
  cuenta y renglones; **configurar después estación y ruta no la reintenta** (sigue bloqueada); «Liberar a preparación»
  manual (Cocina → «Pendientes de liberación y avisos», `kitchen.release` + `kitchen.stations.all`,
  `PreparationBoard.tsx:104-150`) la libera; un pedido nuevo con la configuración completa se libera solo.
- **Conclusión**: no es un defecto del cálculo; es la regla documentada de la etapa 5 (`docs/KITCHEN_AND_PRINTING.md:19`:
  sin ruta, liberación bloqueada visible, nunca un envío aparente). Defectos menores de presentación: la advertencia se
  repite por unidad (23 para 15 unidades); una liberación bloqueada no se reintenta al configurar la cocina (solo el
  planificador reintenta las programadas, `kitchen/worker.go:64-92`); las bloqueadas de cuentas cerradas siguen en la
  lista de Cocina (`kitchen/reads.go:139-145`). Corrección propuesta para §4.4: etiqueta «Enviado · sin confirmar en
  cocina» una vez por producto (sin repetirla en el encabezado) y, opcional, deduplicar advertencias; la ruta «Sin
  preparación» (`mode='none'`) ya permite no usar cocina, pero deja los productos como «Listo».

### 5. Empaquetado en macOS

`build-macos` = sync + `tauri build --bundles app` (`dev.sh:77-81`) → `desktop/src-tauri/target/release/bundle/macos/AD-EL
POS.app` (arm64, mínimo macOS 14.0, `local.adel.pos.dev`, sin firma, notarización, DMG ni instalador). **No incluye Go
ni PostgreSQL** (`tauri.conf.json` sin `externalBin` ni recursos). Go se compila a `bin/ad-el-pos-server`
(`build-server`, `dev.sh:8-12`) y corre como proceso independiente con `server-start` (`scripts/server-process.py`:
`Popen` con sesión propia, PID en `.local/server.pid`, log `.local/server.log`, `SIGTERM` al detener); PostgreSQL 18.6
de Homebrew (`/opt/homebrew/opt/postgresql@18/bin`) con el clúster exclusivo `.local/postgres` (55432, socket
`.local/socket`, log `.local/postgres.log`) por `db-start`. Nada arranca solo tras reiniciar; la plantilla launchd
(`ops/launchd/local.adel.pos.server.plist.example`) no está instalada. Configuración y secretos del servidor en
archivos 0600 de `.local/` (`server.json` con la contraseña de la base, `admin-password`, `app-password`,
`provisioning-token`, `enrollment-code`, `tls/`); respaldos en `backups/`. Terminal: `connection.json` embebido
(`http://127.0.0.1:8181`) o el de `app_config_dir` o `ADEL_TERMINAL_CONFIG` (`connection.rs:64-79`); tokens de
terminal y sesión en Keychain (`vault.rs:18-63`, servicio = `bundle_id`, cuenta = `url|perfil`). «Instalar» hoy =
compilar en el repositorio y ejecutar el `.app` desde `target/` con `open-app` (`dev.sh:63-73`); no hay copia en
`/Applications`. Detalle en `NOTAS_WINDOWS.md`.

### 6. Dependencias de macOS

CUPS con rutas fijas `/usr/bin/lp`, `lpstat`, `lpoptions` (`kitchen/transport.go:55,67,100,110,139,319`,
`transport_formatted.go:138`, `cups_raw.go:86,89`) detrás de `Transport` y `QueueInspector`; cajón USB solo por cola CUPS
raw; Keychain por `keyring` (con `windows-native` ya en `Cargo.toml`); menú nativo con submenú «AD-EL POS» solo en macOS
(`menu.rs:197`, `layout` 248-…); atajos ⌘/Ctrl y ⌃⌘F/F11 por plataforma (`commands/platform.ts:5-38`); `/dev/tty` y
`x/term` para `provision-sa` (`cmd/…/local.go:38-58`); `SIGTERM` (`main.go:82`); scripts bash y Python con rutas
Homebrew y `ps`/`osascript` (`database.py:17-26`, `server-process.py`, `tests/runtime_smoke.py:17,88`); `.tools/` con Go,
Node (`darwin-arm64`), Rust y Python de macOS. Textos con «macOS»/«Llavero» (`main.rs:33`, `App.tsx:281`,
`PrintersAdmin.tsx:68-69`). Detalle y riesgos en `NOTAS_WINDOWS.md`.

### 7-10. Para el diseño de las fases

- **Mesas**: `OperationalPage.tsx:695-760` (mapa + `TablePanel` solo con mesa elegida: `operation-map-split`,
  panel `clamp(320px, 28vw, 460px)`, `styles.css:4048-4052`). Mapa `TableMap.tsx` (lienzo lógico 1000 × 560 con
  `aspect-ratio` y `width: 100%`, posiciones y tamaños en %; mesas de 64-240 × 56-180 unidades, `tables.go:54-58`;
  mínimo 44 px; `styles.css:1358-1390, 3990-4014`): la escala depende solo del ancho de la tarjeta, no del contenido, y
  cambia al abrir el panel. Texto de ayuda `OperationalPage.tsx:718-722`; leyenda debajo del mapa en dos listas
  (`TableMap.tsx:389-413`, `styles.css:1551-1611`); **no existe** resumen de estados. «Actualizar»: un solo botón para
  Mesas, Cuentas y Para llevar (`OperationalPage.tsx:522-524`) y el icono «Actualizar» de la captura
  (`CaptureTopBar.tsx:191-198`). Tiempos: `elapsedTime` (`services/restaurant.ts:337-341`) en `TableMap.tsx:48,88`,
  `AccountList.tsx:122`, `TablePanel.tsx:233` («Abierta hace…») y también Cocina (`PreparationBoard.tsx:238`).
- **Cuentas y Para llevar**: tarjeta en `AccountList.tsx:109-144` (5-7 renglones); detalle `CaptureBill.tsx`
  (`WorkspaceBill`, `CaptureScreen.tsx:207-…`); renglones `BillLines.tsx:17-92` con «Modificar»/«Cancelar unidades»
  57-80 y «Desglose · Versión N · Descuento $…» 81-86; nombres de combo «Grupo: elección» en `captureRules.ts:176-182`
  (línea 179).
- **Funciones del establecimiento**: `establishment_features` solo tiene `code, enabled, revision, updated_by,
updated_at, enabled_at` (0007:8-14, 0010:19) y `internal/features` solo activa/desactiva; no admite un número. Ya existe
  `restaurant_settings` (singleton: `warning_minutes` 15, `overdue_minutes` 30, 0004:31-37) que Go entrega con el mapa
  y la lista de cuentas y usa para la urgencia de Para llevar (`accounts_read.go:47-53`). Recomendación: columna nueva
  `restaurant_settings.long_open_hours` (12) mostrada en la sección «Funciones del establecimiento», y Go calcula la
  marca de cuenta abierta por mucho tiempo como hoy la urgencia.
- **Barra superior** (`navigation/TopBar.tsx:72-121`): logotipo · Mesas · Para llevar · Cocina · **Caja con «● Turno
  abierto»** · Administración | estado del servidor · usuario · pantalla completa. A 1280 px (captura 9C
  `shots9cD2/chromium-mapa-panel-light-1280x720.png`) cabe sin cortarse, pero el estado del turno (texto 12 px junto al
  punto, `styles.css:7385-7436`) separa Caja de Administración; bajo 1280 px se oculta (`styles.css:7614-7631`).

### Migración 0021 propuesta (una sola)

Tablas `account_cancel_reasons` (catálogo como `discount_reasons`, con los cuatro motivos; «Otro» exige descripción;
opcional `prepared_is_waste`) y `account_cancellations` (una por cuenta cancelada: motivo y su nombre congelado,
descripción, canceló, **autorizó** opcional distinto, terminal, operación, unidades e importe); columna
`restaurant_settings.long_open_hours` (12, CHECK con nombre); permiso `accounts.cancel` solo al administrador y nueva
descripción de `accounts.manage`. Sin cambios a `restaurant_accounts` (se reutiliza `voided`) ni a
`payment_authorizations`. SQL ensayado (sintaxis, sobre 0001-0020 en una base temporal) en
`0021_propuesta.sql`; la prueba de diagnóstico en `zz_v0_stage9d_test.go.txt`.

### Decisiones del coordinador tras la Fase 0 (ninguna cambia §0-B)

1. **Descartar cuentas vacías (§4.2)**: el descarte sin motivo acepta cuentas sin unidades enviadas, pedidos,
   combinaciones ni registros financieros o de cocina, aunque tengan un borrador sin enviar (#1 y #3); borra el borrador,
   no es una cancelación y **no aparece en reportes**. «Cuentas canceladas» excluye también las liberaciones de la 7.1
   («Cuenta vacía liberada») y conserva las anulaciones anteriores con «Autorizó» vacío.
2. **Creación diferida** (§4.1 y «ninguna cuenta se guarda sin productos» de §0-B): Para llevar **y** «Cuenta sin mesa»
   entran a la captura sin crear nada; la cuenta nace con el primer producto (Go ya lo admite).
3. **Cancelar con descuentos o cargos vigentes**: como con los pagos parciales, el botón explica que primero hay que quitar
   el descuento o el cargo (la guarda de la etapa 6 no cambia).
4. **«Cliente se retiró sin pagar» y lo ya preparado (§5.4)**: cada motivo lleva la marca «lo enviado a cocina cuenta como
   merma»; con ella, las unidades ya liberadas quedan preclasificadas como «Preparado / merma» y el encargado puede
   corregirlas hasta el corte del día, como en la etapa 8; con los demás motivos siguen «por clasificar».
5. **Permiso propio `accounts.cancel`** («Cancelar cuentas»), solo al administrador en la 0021; sin él, autorización de
   supervisor con el mismo mecanismo de los descuentos. La comprobación va dentro de la ruta.
6. **Verde que se conserva**: la paleta de colores de categorías (dato elegido por el restaurante) y la serie verde de los
   gráficos de Reportes (para distinguir series); el resto de la interfaz deja el verde.
7. **Un solo botón principal**: fuera de la captura, «Capturar…» es el único relleno y «Cobrar» queda con contorno; dentro
   de la captura, «Enviar a cocina» sigue siendo el principal (§4.6).
8. **Colores sin lugar en la paleta**: la entrada de caja pasa a azul; la caja en arqueo a ámbar (no violeta, para no
   confundirse con Pago parcial); los colores escritos a mano pasan a tokens.
9. **Textos**: «Sin conexión» en lugar de «Fuera de línea»; «Libre» recibe su icono.
10. **«Pendiente de liberación · revisar cocina» no es un defecto del cálculo**: la instalación real tiene 0 estaciones,
    0 rutas y 0 impresoras, de modo que cada liberación queda bloqueada por «Sin ruta de preparación». Se cambia la
    etiqueta por «Enviado · sin confirmar en cocina», una sola vez por producto; las reglas de liberación de la etapa 5
    (una liberación bloqueada se libera a mano en Cocina) no cambian y se documentan.
11. **Umbral de cuenta abierta (§3.4)**: `restaurant_settings.long_open_hours` (12 por omisión) en la 0021, editable en
    «Funciones del establecimiento».

## Fase A · Color de marca y estilos generales (§1)

Agente A, 12/11/2026, sobre `etapa-9d` (`ecae96d`), solo presentación (sin Go, migraciones, `api.rs` ni
`services/*.ts`). Tabla completa de tokens, contraste por par y colores eliminados: notas de la fase
(`docs9dA/tokens.md` en el directorio temporal de la sesión).

- **Tokens en un solo lugar** (`desktop/src/styles.css`, `:root` y `:root[data-theme='dark']`): marca constante
  (`--brand-blue #1f5fa8`, `--brand-blue-pressed #173d6e`, `--brand-blue-light #4090f0`), paleta de estado de §0-B con
  sus suaves (`--blue`/`-strong`/`-soft #e7eef7`/`-border #a9c2e2`, `--amber`, `--grape`, `--gray`, `--teal`, `--red`)
  y usos (`--accent`, `--accent-strong`, `--on-accent`, `--warning`, `--warning-mark`, `--danger`, `--info`, `--table-*`,
  `--cash-*`, `--status-online`, sombras, velo, papel y marca compacta). Tema oscuro: azul `#529bf2` (el del logotipo
  aclarado lo necesario: 5.55:1 sobre el panel y 4.93:1 sobre el fondo suave) con texto oscuro `#0b1626` encima
  (6.33:1). Todos los pares principales cumplen AA en ambos temas (texto ≥ 4.5:1; bordes y puntos de estado ≥ 3:1).
- **El verde sale de la interfaz**: acento, selección, ocupada, «Turno abierto», «En línea», tarjeta de entradas de Caja,
  marca compacta (`.brand-mark`), punto de «Estado del sistema» y selección del papel simulado pasan al azul. Se
  conservan la paleta de colores de categorías (dato de Go) y la serie verde de los gráficos (`--chart-3`), decisión 6.
- **Estados de mesa** (§1.2): libre blanco con borde gris, ocupada azul claro con borde azul medio, esperando pago ámbar,
  pago parcial uva (violeta, a más de 45° de tono del azul) y fuera de servicio gris rayado con candado. Cada estado
  lleva su icono en el mosaico, en la leyenda y en el panel; «Libre» recibe la palomita (`tableStateIcons`, compartido por
  `TableMap` y `TablePanel`). Selección como antes (contorno azul de 3 px); la ocupada seleccionada pasa su borde al azul
  principal con contorno de 4 px. «Pago: Abono» de las tarjetas de cuenta también en uva.
- **Avisos** (§1.3): éxito azul con palomita, información turquesa con «i», advertencia ámbar con triángulo y error rojo
  con equis (iconos ya existentes; cambian los tokens; `Icon` expone `data-icon` para las pruebas).
- **Importes** (§1.4): total, saldo, cambio, cortesía, importe del cobro y precio del producto en el color del texto y en
  negritas (antes en el acento).
- **Acciones destructivas en rojo** (§1.5), en toda la aplicación: contorno rojo (`danger-button`, borde rojo suave) en
  «Cancelar unidades», «Anular cuenta vacía» del menú «Más…» (`MoreAction.tone`), «Liberar mesa», «Cancelar ajuste»,
  «Descartar selección / posiciones locales / mis cambios / cambios», «Cancelar el arqueo y volver a contar», «Borrar
  captura», «Cancelar conteo», todo «Quitar …» (renglones, niveles, porción, imagen, color, fecha, hora y los
  `RemoveButton` del catálogo), «Retirar de prueba», el icono «Quitar el dato» del diseñador y la opción «Descartar mi
  borrador» de `ChoiceDialog` (`DialogChoice.tone`); relleno rojo (`danger-solid-button`) en el confirmar de
  `ConfirmDialog` con `tone: 'danger'` (se agregó a «Liberar mesa»), en «Confirmar cancelación de unidades» y en la
  variante destructiva nueva de `Form` (`tone="danger"`, usada por «Anular cuenta vacía»). No se pintan de rojo, por no
  destruir nada: «Registrar devolución» (flujo de cobro; su confirmación ya es roja), «Retirar efectivo», «Retirar todos
  los complementos» (elección del cliente), «Desactivar» (reversible), «Liberar a preparación» y «Cancelar» a secas.
  Una prueba recorre el código y falla si un botón con verbo destructivo pierde la clase roja.
- **Cursor** (§1.6): todo control deshabilitado (`button`, `input`, `textarea`, `select`, opciones y botones con
  `aria-disabled`, selector, producto de la captura, paleta) con `cursor: not-allowed`; ya no queda `cursor: wait` ni
  `default`. El de progreso solo con `aria-busy` (formularios de `Form` mientras Go responde, `ChoiceDialog` ocupado,
  reportes y cotizaciones que ya lo marcaban).
- **Indicadores** (solo color; la posición la cambia la fase B): «Turno abierto» y «En línea» azules, turno cerrado gris,
  arqueo ámbar (punto con `--warning-mark`), reconectando ámbar, sin conexión rojo. Caja: entradas en azul y cierre en
  ámbar (decisión 8). Sección activa de la barra y pestañas de áreas, Inventarios y Reportes con fondo `#e7eef7` y
  borde `#a9c2e2`; la tarjeta seleccionada conserva el borde azul principal (3:1 frente al panel).
- **Sin colores escritos directamente**: se retiraron los 28 de `styles.css` fuera de los tokens y los 5 de
  `LogoSection`/`PaperPreview` (lista en las notas). `src/styles.test.ts` recorre `src/**/*.{ts,tsx,css}` y falla con
  cualquier hexadecimal, `rgb()`/`hsl()`…, nombre de color de CSS o clase de color fijo de Tailwind fuera de los
  bloques de tokens; única excepción: la máscara alfa del papel simulado (2 líneas, exigidas por la prueba). No se
  recorren pruebas ni datos de prueba (colores de categorías como dato).
- **Pruebas**: Vitest `src/styles.test.ts` (16: colores escritos, valores de la marca, contraste AA de 29 pares y 3:1 de
  9 en ambos temas, sin verde, uva frente a azul, asignación de la paleta, reglas de avisos, importes, destructivos,
  sección activa, cursores y selección) y `src/components/stage9dStyles.test.tsx` (13: avisos por tipo con su icono,
  estados de mesa con icono en mosaico, leyenda y panel, destructivos de `ConfirmDialog`, `ChoiceDialog`, `Form` con
  `aria-busy`, menú «Más…» y recorrido del código). Playwright `e2e/workflow-stage9d-colors.spec.ts` (Chromium y
  WebKit): colores calculados contra los tokens en ambos temas, iconos de los cinco estados, cursor `not-allowed` en
  «Cobrar» deshabilitado y `progress` con `aria-busy`, ocupada seleccionada con contorno de 4 px, saldo en el color del
  texto, «Liberar mesa» y su confirmación en rojo, los cuatro avisos, la barra con turno abierto, en arqueo, sin conexión
  y sin turno, y 48 capturas (Mesas, Mesas › Cuentas, Para llevar, confirmación destructiva, barra en cuatro estados y
  cuatro avisos, en claro y oscuro a 1280 × 720 y 1920 × 1080). «Esperando pago» exige una precuenta impresa (impresora
  de tickets): en la captura el puente de la prueba presenta así una mesa con pedido confirmado; «Sin conexión» se
  simula rechazando las peticiones en el puente, como lo vería la terminal.
- **Resultados** (copia aislada = `HEAD ecae96d` + archivos de la fase A, sin los cambios en curso del agente BE):
  `npx tsc --noEmit` sin errores; `npm test` 70 archivos y 666 pruebas aprobadas; `npm run build` correcto; Playwright
  completo 16/16 en Chromium y 16/16 en WebKit (en una primera corrida WebKit falló `workflow-stage9c-keyboard` al
  cerrar «Acerca de» con Escape —foco del diálogo, sin relación con los colores— y arrastró a las tres siguientes por
  el reinicio del trabajador; la segunda corrida completa, con la versión final, aprobó todo en ambos motores).

## Fase B · Barra superior (§2)

Agente BC, 12/11/2026, sobre `etapa-9d` (`4abe78d`; validada al final sobre `2c00064`), solo interfaz (sin Go, migraciones ni `api.rs`), en paralelo con
la parte E. Usa los tokens de la fase A; no cambian acciones, permisos ni el menú rápido de Caja.

- **Nuevo orden** (`navigation/TopBar.tsx`): a la izquierda el logotipo y **Mesas, Para llevar, Cocina y
  Administración** seguidas y **del mismo ancho** (cuadrícula de columnas iguales: cada una mide lo que la más larga,
  «Administración»); a la derecha **Caja**, el estado del servidor, el menú del usuario y pantalla completa. El grupo
  `nav` «Navegación principal» contiene solo las cuatro secciones; Caja sale del grupo (§0-B).
- **Caja en dos renglones** (`pages/cash/CashShortcut.tsx`, `placement="bar"`): «Caja» y debajo el estado del turno con
  su indicador (azul abierto, ámbar en arqueo, gris sin turno; «Consultando…» mientras llega el turno). Mismo botón,
  mismo menú rápido de la 7.1/9C, mismos permisos y mismo nombre accesible («Caja · Turno abierto»); el menú se alinea
  ahora con el borde derecho. Sin el turno visible (solo `cash.shifts.read`) el botón «Caja» también va a la derecha y
  abre la pantalla. La barra reducida de la captura conserva su Caja (en línea) y el usuario.
- **Estado del servidor** (`navigation/serverState.ts`, decisión 9): «En línea» (texto y punto en azul, ahora visible
  también a 1280 px), «Reconectando» (ámbar) y **«Sin conexión»** (rojo) en lugar de «Fuera de línea».
- **Atajos sin cambio**: ⌘/Ctrl + 1–4 siguen siendo Mesas, Para llevar, Cocina y Caja (⌘5 Administración); el registro
  de secciones (`sections.ts`) no cambió (la barra separa Caja al dibujarse).
- **Sin cortes a 1280 px**: botones de 52 px (rango 52–56); de 1280 a 1599 px menos relleno (secciones 8 px, logotipo
  116 px, servidor 8 px) para que quepan las cuatro secciones iguales, Caja en dos renglones y «Sin conexión»; si aun
  así faltara espacio (un nombre de usuario largo), solo se acorta el nombre del usuario con puntos suspensivos. Debajo
  de 1280 px sigue la barra compacta de la 9C (marca abreviada; servidor en línea y turno como punto).
- **Corrección del foco de «Acerca de» y «Atajos de teclado»** (`commands/CommandDialog.tsx`): se reprodujo en WebKit la
  falla intermitente de `workflow-stage9c-keyboard` («Acerca de» no se cerraba con Escape; la captura de la falla
  muestra el foco en «Cerrar» cuando ya era tarde). Causa: el foco entraba al diálogo en un `useEffect`, que corre
  después de dibujar; un Escape pulsado en cuanto el diálogo aparece caía fuera de él, y la capa de teclado deja Escape a
  los diálogos abiertos, así que nadie lo atendía. Ahora el foco entra antes de dibujar (`useLayoutEffect`) y, si el foco
  queda fuera del diálogo, un oyente de Escape en el documento lo cierra igual. Prueba nueva en
  `commands/commandCenter.test.tsx`.
- **Pruebas**: Vitest `navigation/navigation.test.tsx` (orden de las cuatro secciones, Caja a la derecha antes del
  servidor, el usuario y pantalla completa, Caja en dos renglones con su indicador y estados abierto/arqueo/sin turno,
  Caja sin turno visible a la derecha, «Sin conexión»), `App.test.tsx` («Sin conexión» y recuperación) y
  `src/stage9dLayout.test.ts` (columnas iguales, 52 px, colores de los indicadores por token, menú de Caja a la
  derecha, solo el nombre del usuario se acorta). Playwright `e2e/workflow-stage9d-topbar-tables.spec.ts` (Chromium y
  WebKit): orden, mismo ancho (±1 px), Caja a la derecha en dos renglones, alturas 52–56 px, ningún texto cortado ni
  desbordamiento a 1280 × 720 y 1920 × 1080 en ambos temas, «En línea» e indicador del turno en azul, menú de Caja
  dentro de la ventana y «Sin conexión» en rojo sin cortes. Se actualizaron `e2e/navigation.ts` (`appBar`,
  `cashButton` y `openCash` buscan Caja en toda la barra) y `workflow-stage9c-navigation.spec.ts` (nuevo orden).
  Capturas de la barra (en línea y sin conexión, claro y oscuro, 1280 × 720 y 1920 × 1080) en las notas de la fase
  (directorio temporal, `shots9dBC/`).
- **Resultados** (12/11/2026): la fase B sola (copia = `HEAD 2c00064` + archivos de la fase B) aprobó `npx tsc --noEmit`,
  `npm test` (73 archivos, 693 pruebas), `npm run build` y Playwright `workflow-stage9c-keyboard`,
  `workflow-stage9c-navigation`, `workflow-stage9d-cancel-account` y `workflow-stage9d-colors` (4/4 en Chromium y en
  WebKit). La suite completa de Playwright se corrió al final de ambas fases (ver la fase C).

## Fase C · Mesas (§3)

Agente BC, 12/11/2026, sobre `etapa-9d` (`4abe78d`; validada al final sobre `2c00064`), solo interfaz; usa `long_open` y `settings.long_open_hours` que
ya entrega Go (parte BE, `b84e3be`). No cambian acciones, estados, reglas ni cálculos.

- **Panel lateral siempre visible** (§3.1, `OperationalPage.tsx`, nuevo `TablesSummaryPanel.tsx`): ancho fijo de 360 px
  (420 px desde 1600 px); elegir una mesa ya no cambia el tamaño del mapa. Sin mesa elegida muestra el **resumen del
  área** (libres, ocupadas, esperando pago y pago parcial; React solo cuenta las mesas por el estado que calcula Go):
  cada número se toca para **resaltar esas mesas** en el mapa (contorno azul discontinuo, distinto de la selección; se
  vuelve a tocar para quitarlo; sin mesas en ese estado, el número no se toca), y aparte «N mesas fuera de servicio».
  Debajo, las **acciones rápidas** «Cuenta sin mesa» y «Nuevo pedido para llevar» (las mismas funciones de hoy:
  `openAccount`; el pedido para llevar además lleva a la sección Para llevar, como la paleta; la fase D las pasará a la
  creación diferida) y el **texto de ayuda** que antes estaba sobre el mapa. Con mesa elegida, el panel de siempre.
  Escape cierra el panel de la mesa y vuelve al resumen.
- **Mapa** (§3.2, `components/restaurant/TableMap.tsx`): a la misma altura que el panel; **escala automática** al abrir
  un área o cambiar el tamaño de la ventana (`fitTables`: una sola escala para todas las mesas del área, recortando el
  espacio vacío del lienzo de 1000 × 560 y centrando el conjunto, sin pasar del doble); **lado menor de cada mesa de al
  menos 64 px**: si así no caben, el mapa se desplaza **arrastrando** (dedo o puntero; el toque que termina un arrastre
  no elige la mesa) y con **barras de desplazamiento siempre visibles**. La **leyenda va dentro del marco del mapa, en
  una sola línea**. El diseñador de «Áreas y distribución» conserva el lienzo proporcional de siempre (las posiciones se
  editan sobre el espacio completo; prop `autoFit` solo en Mesas): arrastrar y mover con flechas sigue igual.
- **Un solo botón principal por panel** (§3.3, decisión 7): en el panel de la mesa, **«Capturar pedidos en esta mesa»**
  (antes «Capturar») relleno en azul a todo el ancho; «Cobrar · $saldo» conserva su texto e importe como primera acción
  secundaria con contorno azul (sin relleno) y Precuenta, Dividir, Liberar mesa y Más… en tres columnas. En el detalle
  de la cuenta de Cuentas y Para llevar, **«Capturar productos en esta cuenta»** pasa a relleno azul y «Enviar a cocina»
  y «Cobrar» quedan con contorno (`BillFooter` `emphasis="overview"`, `CheckoutButton` `tone="secondary"`); dentro de la
  captura «Enviar a cocina» y «Cobrar» siguen como principales (§4.6). En el panel no existe «Ver cuenta»: no se agregó
  ninguna acción.
- **Tiempos** (§3.4, `services/restaurant.ts` `elapsedTime`): desde 24 h, «3 d 14 h» (antes «87 h 33 min»); menos de
  un día sigue «5 h 12 min» y menos de una hora «45 min». Un solo formato para mesas, cuentas, Para llevar, «Abierta
  hace…» y Cocina. **Advertencia ámbar discreta** con la marca `long_open` de Go (React no la calcula): en la mesa, un
  punto ámbar en la esquina y el tiempo en una etiqueta ámbar con el triángulo (nombre accesible «…, abierta hace más de
  N h»); en el panel, «Abierta hace…» en ámbar con el triángulo; en la tarjeta de Cuentas y Para llevar, la franja y el
  tiempo en ámbar con el triángulo. N es `settings.long_open_hours`.
- **Sin «Actualizar»** (§3.5) en Mesas, Cuentas, Para llevar y la barra de la captura: los datos llegan solos (eventos y
  sondeo; `workspace.refresh` se conserva para uso interno). **«Reintentar»** aparece solo cuando la barra indica «Sin
  conexión» (App pasa `connectionLost`): vuelve a consultar la sesión y los datos de la operación; en la captura va en
  su barra reducida.
- **Pruebas**: Vitest `pages/restaurant/stage9dTables.test.tsx` (16: formato «d h», escala que ve todas las mesas,
  mínimo de 64 px con desplazamiento, tope del doble y centrado, mapa en píxeles con la misma escala y la leyenda dentro
  del marco, diseñador proporcional, advertencia por `long_open` en mosaico y nombre accesible, resumen por estado con
  resaltado y acciones rápidas, panel siempre visible y resaltado en el mapa, «Cuenta sin mesa» con la apertura de
  siempre, un solo principal en el panel de la mesa y en el detalle de Cuentas, «Abierta hace 3 d 14 h» en ámbar, sin
  «Actualizar» y «Reintentar» solo sin conexión en Mesas y en la captura) y `src/stage9dLayout.test.ts` (reglas de
  Mesas: ancho fijo, leyenda en una línea, contorno de «Cobrar», ámbar con tokens, resaltado distinto de la selección).
  Se ajustaron `tablePanel.test.tsx` y `capture/capture.test.tsx` («Capturar pedidos en esta mesa»; la prueba que usaba
  «Actualizar» ahora recibe el aviso de cambio del servidor). Playwright `workflow-stage9d-topbar-tables.spec.ts`
  (Chromium y WebKit): panel siempre visible de 360 px con y sin mesa, mapa y panel a la misma altura, resumen y
  resaltado, escala automática con todas las mesas dentro y la misma escala (mesa redonda redonda), 35 mesas que a
  1280 × 720 no caben con 64 px y se desplazan arrastrando sin elegir mesa (y caben a 1920 × 1080), «3 d 14 h» y ámbar
  en la mesa, el panel y la tarjeta, botones principales en el panel y en el detalle, sin «Actualizar» en Mesas, Cuentas,
  Para llevar ni la captura, «Reintentar» sin conexión, diseñador que mueve y guarda una mesa y «Nuevo pedido para
  llevar» del resumen. «3 d 14 h» y `long_open` se presentan con el puente de la prueba (Go los calcula con su hora; la
  prueba no puede envejecer una cuenta). Se actualizaron las especificaciones que usaban «Capturar»
  (`restaurant.spec.ts`, `navigation.ts`, `workflow-stage9c-navigation`, `workflow-stage9c-tables-time`,
  `workflow-stage9d-colors`). Capturas de Mesas sin y con mesa elegida, desplazamiento y detalle de Cuentas (claro y
  oscuro, 1280 × 720 y 1920 × 1080) en `shots9dBC/`.
- **Resultados** (12/11/2026; copia aislada `repo9dBC` = `HEAD 2c00064` —fase E de la interfaz— + fases B y C; clúster
  temporal 55499, Go temporal 8186, vista previa 1424): `npx tsc --noEmit` sin errores; `npm test` 74 archivos y 714
  pruebas aprobadas; `npm run build` correcto; Playwright completo, al final de ambas fases (no de cada una), **18/18 en
  Chromium y 18/18 en WebKit** (registro `docs9dBC/e2e_full_final.log`). Antes, con la fase E aún sin commit, una
  primera corrida completa reprodujo en WebKit la falla de `workflow-stage9c-keyboard` («Acerca de» con Escape; por el
  reinicio del trabajador, las cuatro siguientes fallaron al no encontrar la base preparada); con el foco del diálogo
  corregido (fase B) la segunda corrida y la final aprobaron todo en ambos motores.

## Fase D · Cuentas y Para llevar (interfaz, §4)

Agente D, 12/11/2026, sobre `etapa-9d` (empezó en `ffcaed7`; rebasada y validada sobre `d723e92`, correcciones de la
revisión, parte 1), sin subagentes. Solo interfaz: sin Go, migraciones ni `api.rs` (el backend de D ya estaba en
`b84e3be`). Copia aislada `repo9dD`; clúster temporal 55499, Go temporal 8188, vista previa 1426.

- **Creación diferida** (§4.1, decisión 2): «Nuevo pedido» (Para llevar), «Cuenta sin mesa» (barra de Mesas › Cuentas
  y resumen de Mesas), «Nuevo pedido para llevar» del resumen, el menú nativo y la paleta entran a la captura **sin
  crear nada**, con la cuenta pendiente de la 7.1 generalizada (modalidad y mesa opcional en el motor del borrador
  `draftSync.ts`, `useDraftSync.ts`, `pendingAccountBill` y `useRestaurantWorkspace.startPendingCapture`). El pedido o
  la cuenta nacen en Go al guardarse el primer producto (la apertura idempotente con `items` que Go ya admitía; para
  llevar con el origen «Para llevar»); salir sin productos no deja nada. Títulos «Para llevar · Pedido nuevo» y «Cuenta
  nueva sin mesa». La apertura rápida inmediata (`quickOpen`) se retiró. La paleta y el menú nativo piden la sección y
  la apertura espera a que la pantalla la reciba (la vuelta a la vista principal de la sección ya no puede dejar la
  captura detrás); «Nuevo pedido para llevar» del resumen espera el resultado de `requestScreen` como la paleta
  (observación O1 de la revisión: respeta la protección de cambios sin guardar; `NavigationValue.requestScreen`
  devuelve ahora `Promise<boolean>`, como ya hacía App). La selección automática del primer elemento del listado ya no pisa una cuenta elegida en el mismo
  instante (carrera reproducida en Vitest).
- **Descartar cuentas vacías** (§4.2, decisión 1 y cambio del coordinador): en una cuenta `discardable` la interfaz
  ofrece **«Descartar cuenta vacía»** (rojo) en lugar de «Cancelar cuenta» en «Más…» (panel de la mesa, captura,
  Cuentas y Para llevar); a la vista en el detalle de Cuentas y Para llevar y en el panel de la mesa cuando no es
  liberable (si no tiene nada, «Liberar mesa» de la 7.1 no cambia). «Cancelar cuenta» solo en las no descartables (Go
  responde `ACCOUNT_DISCARDABLE`). Sin motivo ni supervisor (`restaurant_discard_account`, `accounts.open`):
  `ConfirmDialog` de tono peligroso que dice cuántos productos sin enviar se borran, que descartar no es una
  cancelación ni aparece en reportes y si la mesa queda libre. Antes se guardan los toques pendientes y se usa la
  revisión vigente. Después (también al cancelar o liberar) se vuelve al mapa o al listado sin pasar la captura a otra
  cuenta y el listado elige la siguiente, no la recién cerrada.
- **Tarjetas compactas** (§4.3, `AccountList.tsx`): dos renglones (folio y cliente, mesa u origen no habitual, tiempo e
  importe; etiquetas de preparación corta, pago, saldo, entrega, unidades o «N sin enviar», programación y antigüedad),
  sin fecha de apertura, «Confirmado:» ni «unidades confirmadas». ~60 px por tarjeta (antes ~157 px); a 1280 × 720
  caben cinco o más donde antes cabían dos y media. Selección azul de la fase A.
- **Textos del detalle** (§4.4): «Pendiente de liberación · revisar cocina» → **«Enviado · sin confirmar en cocina»**
  (etiqueta ámbar) una vez por producto, ya no en el encabezado; en la tarjeta «Sin confirmar en cocina». «Preparación y
  comandas» muestra cada advertencia distinta una vez («(×N)»). Desglose solo con complementos elegidos, cargos y
  descuento de Go (sin «Versión N» ni «Descuento $0.00»; sin nada que desglosar no aparece). Combos sin nombres
  repetidos (`choiceLabel`: «Grupo: elección» solo si difieren). Pago y entrega de Para llevar en un renglón.
- **Menú por producto** (§4.5, `BillLines.tsx`): tocar el renglón o «⋯» abre «Modificar» y «Cancelar unidades» (rojo)
  debajo del producto, uno a la vez; Escape o tocar fuera lo cierran (la capa global de teclado lo cuenta como capa
  abierta). Los productos nuevos conservan − 1 + y «Personalizar»; «Enviado a cocina» / «Nuevo sin enviar» y «Cobrar ·
  $importe» no cambian.
- **Pestañas de Mesas** (observación O2 de la revisión): «Mesas / Cuentas / Áreas y distribución» pasan de 38 a 44 px
  (13 px de texto), sin desbordar a 1280 × 720.
- **Archivos de otros alcances con cambio mínimo**: `services/restaurant.ts` (`pendingAccountBill` con modalidad y
  mesa opcional; «Pedido nuevo»), `services/kitchen.ts` (texto de `pending_release`), `commands/actions.ts`
  (descripciones), `commands/keyboard.ts` (`.capture-line-menu` como capa), `navigation/NavigationContext.tsx` (tipo
  de `requestScreen`), `TablePanel.tsx` (botón y `onDiscard`
  opcional), `CaptureTopBar.tsx`, `AccountKitchen.tsx`, `captureRules.ts` (además, `describeSelection` ya no falla con
  un borrador sin personalización completa) y las pruebas `capture.test.tsx`, `tablePanel.test.tsx`,
  `stage9dTables.test.tsx` y `draftSync.test.ts`.

Pruebas (copia aislada `repo9dD` = `HEAD d723e92` + fase D; clúster temporal 55499, Go temporal 8188, vista previa
1426; 12/11/2026):

- **Vitest nuevo** `pages/restaurant/stage9dAccounts.test.tsx` (18): «Nuevo pedido» sin crear nada y salir sin dejar
  nada; el pedido nace con el primer producto (para llevar, sin mesa, origen «Para llevar») y el siguiente toque guarda
  el borrador del pedido real; «Cuenta sin mesa» desde el resumen y desde Cuentas; rechazo de Go al crear explicado en
  la cuenta sin código interno; «Nuevo pedido para llevar» del resumen que respeta `requestScreen`; paleta y menú
  nativo (sección y apertura juntas o en orden); descartar en Mesas (botón y «Más…», sin «Liberar mesa» ni «Cancelar
  cuenta», confirmación roja, cuerpo solo con revisión y `operation_id`, mesa libre), cuenta con productos enviados que
  conserva «Cancelar cuenta», Para llevar (a la vista, un solo principal, el listado pasa al siguiente), dentro de la
  captura (vuelve al listado) y con toques sin guardar (los guarda y usa la revisión vigente); tarjetas de dos renglones
  y reglas de estilo (incluidas las pestañas de 44 px); textos de cocina una vez por producto y no en el encabezado;
  desglose sin versión ni descuento en cero y combos sin nombres repetidos; `choiceLabel`, `chosenComplements` y
  `distinctWarnings`; menú por producto (renglón, «⋯», Escape, tocar fuera, uno a la vez, «Cancelar unidades» rojo que
  abre su panel) y productos nuevos con − 1 + y «Personalizar». `draftSync.test.ts` (+1: pedido y cuenta sin mesa en el
  motor). Ajustadas: `capture.test.tsx` (menú por producto), `tablePanel.test.tsx` («Nuevo pedido» diferido; «Datos del
  pedido» sobre un pedido existente) y `stage9dTables.test.tsx` («Cuenta sin mesa» diferida).
- `npx tsc --noEmit` sin errores; `npm test` **75 archivos, 738 pruebas aprobadas**; `npm run build` correcto;
  Prettier sin diferencias en los 29 archivos.
- **Playwright** `e2e/workflow-stage9d-accounts.spec.ts` (Chromium y WebKit, datos reales de Go): «Nuevo pedido» y
  «Cuenta sin mesa» sin productos no crean nada (consultado en Go antes y después); con el primero nacen (sin mesa); se
  descarta desde «Más…» de la captura, desde el detalle de Para llevar y de Cuentas y desde el panel de la mesa (Mesa
  con solo borrador), sin motivo, cuenta en `voided` y **ninguna en «Cuentas canceladas»**; la mesa con producto
  enviado conserva «Cancelar cuenta»; tarjetas de 44–64 px, cinco o más a la vista a 1280 × 720 y las ocho a 1920 ×
  1080; «Enviado · sin confirmar en cocina» (producto sin ruta de preparación, liberación bloqueada) una vez y no en el
  encabezado; menú por producto con «Cancelar unidades» rojo y en una línea; pestañas de 44 px. Se actualizaron
  `restaurant.spec.ts`, `stock-consumption.spec.ts`, `table-price-decrease.spec.ts`, `takeaway-quick-notes.spec.ts` y
  `workflow-stage9d-topbar-tables.spec.ts`. La especificación nueva se ordena antes de `workflow-stage9d-cancel-account`
  (nombre fijado por el encargo; usa nombres propios —Salón D, Mesa D1–D3, Cliente D1–D7— para no cruzarse con las
  demás).
- **Suite completa de Playwright: 19/19 en Chromium (7.4 min) y 19/19 en WebKit (7.3 min)** sobre el código final
  (registro `docs9dD/e2e_full_final.log`). Antes, en una primera corrida (código previo a O1/O2), WebKit falló al
  inicio de `stock-consumption` (el clic en «Administración» no navegó, antes de cualquier paso de la fase D) y por el
  reinicio del trabajador las 16 siguientes no encontraron la base preparada; la repetición de WebKit aprobó 19/19 y
  la corrida final también. Después se repitió solo la especificación nueva (capturas con prefijo propio): 1/1 en ambos
  motores.
- **Capturas** (32: Chromium y WebKit, claro y oscuro, 1280 × 720 y 1920 × 1080) en el directorio de la fase
  (`shots9dD`): Para llevar con tarjetas compactas, Mesas › Cuentas con el detalle y el texto de cocina, el detalle con
  el menú por producto abierto y la confirmación de «Descartar cuenta vacía»; sin desbordamiento horizontal.

## Fases D y E · backend (parte BE)

Hecha por un solo agente (BE) el 12/11/2026 en `etapa-9d` sobre la Fase 0 (`ecae96d`), sin subagentes, en paralelo
con la parte A (colores). Copia aislada y clúster temporal 55499; sin tocar la instalación real. Contratos JSON para
la fase DE en las notas de la parte (directorio temporal de la sesión, `docs9dBE/contratos.md`).

- **Migración única `0021_account_cancellation.sql`** (esquema 21; `config/product.json` + `sync`): catálogo
  `account_cancel_reasons` (como `discount_reasons`, con «Cliente se retiró sin pagar» —marca de merma—, «Cliente
  canceló el pedido», «Error de captura» y «Otro» —exige descripción—); registro `account_cancellations` (una fila
  por cuenta, de solo inserción por disparador: motivo, nombre y marca de merma congelados, descripción, quién
  canceló, **quién autorizó** —nunca el mismo—, terminal, operación, unidades e importe); `restaurant_settings.
long_open_hours` (12, CHECK 1–72); permiso `accounts.cancel` («Cancelar cuentas») solo al administrador y nueva
  descripción de `accounts.manage`. Restricciones con nombre y CHECK con `IS NOT NULL` explícito. No modifica
  cuentas ni eventos existentes (prueba sobre un esquema 20 con datos).
- **Cancelar cuenta** (`POST restaurant/accounts/{id}/cancel-account`): cualquier cuenta abierta sin pagos netos
  (409 `ACCOUNT_HAS_PAYMENTS`: primero la devolución) ni ajustes vigentes (409 `ACCOUNT_HAS_ADJUSTMENTS`: primero
  quitar el descuento o cargo), de mesa o para llevar, con o sin productos enviados, con motivo del catálogo («Otro»
  sin descripción: 422). Con `accounts.cancel` basta el permiso propio; sin él, la autorización puntual de un
  supervisor con el mismo mecanismo de los descuentos (`operations.Request` recibe ahora el autorizador, como
  `payments`; sin sesión, límite de intentos, sin autoautorización; la contraseña no entra en la huella ni en la
  auditoría). En la misma transacción (lock 7820148305): borrador descartado, unidades activas canceladas con un
  solo evento nuevo `account.cancelled`, cuenta en `voided` con su registro, mesa libre, auditoría
  `restaurant.account.cancelled` (y `restaurant.supervisor_authorized` si hubo supervisor) y evento en tiempo real.
  Nunca es venta (no se cierra; sin reconocimientos). Las guardas de la etapa 6 se reutilizan (`RequireCancellable`
  separa los dos casos y termina en `RequireNoNetFinance`; `RequireUnitsFree`, `RequireAccountCovered`).
- **Cocina e inventario**: `kitchen.Integrate` trata `account.cancelled` como `item.cancelled` (cancela sin papel lo
  no liberado y manda el aviso de cancelación a las estaciones de lo liberado); `inventory.RecordChange` registra
  las solicitudes `cancel` de lo consumido y el trabajador, si el motivo marca merma (decisión 4), crea la
  clasificación como «Preparado / merma» a nombre de quien canceló (resolución manual, nota con el motivo,
  auditoría `inventory.cancellation_preclassified`), corregible por el encargado hasta el corte como en la etapa 8;
  con los demás motivos queda «por clasificar». Sin columnas nuevas en inventario.
- **Catálogo de motivos** (`restaurant/cancel-reasons`): listar con `accounts.read`; crear, renombrar, activar o
  desactivar, ordenar, «exige descripción» y «lo enviado cuenta como merma» con `payments.settings.manage`, el mismo
  permiso de los motivos de descuento (para que ambos catálogos vivan juntos en Administración → Cobros); al menos
  uno activo; renombrar no altera lo registrado.
- **Descartar cuenta vacía** (`POST restaurant/accounts/{id}/discard`, `accounts.open`, sin motivo): cuentas sin
  unidades, pedidos, combinaciones ni registros financieros o de cocina, aunque tengan borrador (#1 y #3 de la
  instalación real); borra el borrador, evento `account.discarded`, no es cancelación ni aparece en reportes. Go
  informa `discardable` en cada cuenta (`releasable` = `discardable` sin borrador; «Liberar mesa» no cambia).
  Creación diferida (decisión 2): la API existente ya crea una cuenta para llevar o sin mesa con su primer producto
  (comprobado); la interfaz es de la fase DE.
- **Reportes**: «Cuentas anuladas» pasa a **«Cuentas canceladas»** conservando el identificador
  `sales.voided_accounts` (exportaciones y filtros guardados no se rompen) desde el registro nuevo, con motivo,
  descripción, importe, quién canceló y **quién autorizó**; las anulaciones anteriores aparecen con «Autorizó» vacío
  y las liberaciones y descartes ya no aparecen (el filtro «Tipo» se retiró; nuevo filtro por motivo con «Sin motivo
  del catálogo» para las anteriores). Reporte nuevo **«Cancelaciones por motivo»** (`sales.cancellations_by_reason`,
  cuentas, unidades, importe y porcentaje, PDF y Excel, en `EXPORTABLE_REPORTS`): 95 reportes. «Productos
  cancelados», «Modificaciones de cantidades» (un renglón por partida, «Cancelación de la cuenta») y «Actividad por
  empleado» (unidades canceladas y autorizaciones) incluyen las cuentas canceladas.
- **Umbral de cuenta abierta (§3.4)**: `GET/PUT settings/long-open-hours` (lectura con cualquier sesión; cambio con
  `settings.features.manage`, el de «Funciones del establecimiento»; Go valida 1–72, revisión, auditoría y aviso).
  Go entrega `settings.long_open_hours` con el mapa y las cuentas y calcula `long_open` en cada cuenta y mesa
  (más de N horas desde la apertura de la cuenta más antigua).
- **Textos de Go**: «Anular» → «Cancelar» en la descripción de `accounts.manage` y en los mensajes de la anulación
  anterior, de «Liberar mesa» y de la guarda financiera. La ruta `void` (cuentas vacías con motivo libre,
  `accounts.manage`) se conserva por compatibilidad; la interfaz ya no la necesita.
- **Puente y servicios**: siete operaciones nuevas en la lista cerrada de `api.rs` con su prueba Rust; tipos y
  llamadas en `services/restaurant.ts` y `services/api.ts`.

Pruebas (copia aislada `repo9dBE`, clúster temporal 55499, 12/11/2026): `gofmt` sin diferencias; `go vet ./...` con y
sin `-tags=integration`, aprobado; `go test -race` de `operations`, `administration`, `payments`, `kitchen`,
`inventory`, `reports`, `ordering` y `database`, aprobado; integración `internal/database` (con la nueva
`TestStage9DAccountCancellationUpgrade`: 0021 sobre un esquema 20 con datos), `internal/administration` (umbral y
permiso), `internal/reports` (con `TestStage9DCancelledAccountsReports` y la de la 9C ajustada), `internal/reports/
operational` e `internal/operations` (aceptación encadenada completa más `TestStage9DAccountCancellation`), todas
aprobadas; las de `operations` y `reports` se repitieron sobre el árbol final. `cargo test --locked --offline`: 41
aprobadas (incluida la prueba de las rutas nuevas), `rustfmt --check` limpio; `npx tsc --noEmit` sin errores y
Prettier limpio en los servicios. Vitest de `src/services` y `tablePanel.test.tsx` (110 pruebas) aprobado como
comprobación adicional; la suite completa de Vitest y Playwright quedan para la fase de interfaz y la regresión final.

## Fase E · Cancelar cuenta (interfaz, §5)

Agente E, 12/11/2026, sobre `etapa-9d` (`4abe78d`, fase A, sobre `b84e3be`, backend de D y E), sin subagentes, en
paralelo con BC (barra y Mesas). Solo interfaz: sin Go, migraciones, `api.rs` ni servicios (los tipos y llamadas de BE
ya estaban). Copia aislada = `HEAD` + los cambios de E (sin los cambios en curso de BC); los cambios se llevaron al
árbol con parches que no tocan las partes de BC.

- **«Cancelar cuenta»** sustituye a «Anular cuenta vacía» en el menú «Más…» de cualquier cuenta abierta de mesa o para
  llevar (panel de la mesa, captura, Cuentas y Para llevar), en rojo, con el permiso de la ruta (`accounts.open`; para
  llevar, `takeaway.manage`). Abre un panel en el lugar del mapa, del listado o del catálogo de la captura, con la
  cuenta a la vista: unidades enviadas e importe que se cancela (de Go, sin sumar en React), productos sin enviar que se
  descartan, motivo del catálogo con botones táctiles (solo los activos, con «Exige descripción» y «Lo preparado cuenta
  como merma»), descripción obligatoria de 5 a 500 caracteres cuando el motivo la exige («Otro»), qué pasa con lo
  enviado a cocina (lo no liberado se retira sin imprimir, las estaciones reciben el aviso; con la marca de merma, lo
  preparado cuenta como merma —«Preparado / merma» corregible hasta el corte si hay control de inventario—; sin ella,
  por clasificar), «no cuenta como venta» y si la mesa queda libre. Con `cancel_requires_authorization`, la autorización
  de supervisor con el componente de los descuentos (`SupervisorAuthorization` + `useSupervisorAuthorization`): la
  contraseña sale del formulario al enviar y el almacén de pendientes no la guarda. Confirmación final con
  `ConfirmDialog` en tono peligroso («¿Cancelar la cuenta #N?» / «¿Cancelar el pedido #N?», foco en «Conservar la
  cuenta»). Al terminar, aviso «Cuenta #N cancelada.», la mesa queda libre y se vuelve al mapa o al listado (el
  espacio de trabajo deja de seleccionar la cuenta cerrada).
- **Bloqueos que explican**: con `cancel_blocker = payments` o `adjustments` la opción sigue habilitada en «Más…» con la
  aclaración «Primero registra la devolución» / «Primero quita el descuento o cargo» (descripción accesible del botón)
  y el panel explica qué hacer, con «Ir a devoluciones» o «Ver descuentos y cargos» si el usuario tiene el permiso (si
  no, qué permiso hace falta). Una cuenta que cambió en otra terminal pide «Revisé los cambios de esta cuenta» antes de
  enviar.
- **Resultado incierto**: el panel ofrece «Reintentar sin duplicar» con el mismo `operation_id` (la mutación del
  espacio de trabajo; «Recuperar resultado» de la página también sirve). Si la cancelación era con supervisor y no llegó
  a registrarse, el reintento sin contraseña vuelve con un error de autorización y el formulario pide repetirla
  (`requireRepeat`). Los rechazos del motivo (`CANCEL_DESCRIPTION_REQUIRED`, `…_TOO_LONG`, `CANCEL_REASON_INACTIVE`)
  se explican junto al campo; los de autorización, en el formulario del supervisor.
- **Nombres**: ya no queda «Anular cuenta» en la interfaz. El filtro «Anuladas sin productos activos» es «Canceladas o
  descartadas» y la tarjeta dice «Cuenta cancelada o descartada»; «Liberar mesa» confirma «Se descartará»; el detalle
  de una cuenta cancelada muestra su registro (`Bill.cancellation`: motivo, descripción, canceló, autorizó, unidades,
  importe y fecha) y las anteriores, liberadas o descartadas, «Cuenta cancelada sin productos activos o descartada
  vacía». Historial: «Cuenta cancelada» detalla motivo, descripción, unidades e importe, quién canceló y quién
  autorizó, productos sin enviar descartados y la merma; «Cuenta vacía descartada», que no es cancelación. Registro de
  movimientos: nombre legible sobre el código de las operaciones de la 9D. Cocina: «Aviso de cancelación pendiente»
  también para `account.cancelled`. Los permisos visibles y los títulos de los reportes vienen de Go (sin cambios en la
  interfaz); el menú nativo y la paleta no mencionaban la anulación.
- **Catálogo de motivos de cancelación** en Administración → Cobros, acordeón «Motivos de cancelación» junto a
  «Motivos de descuento» y con su mismo estilo: lista con sus marcas, crear, renombrar, activar o desactivar, ordenar
  por posición, «Exige descripción» y «Lo enviado a cocina cuenta como merma»; lectura con `accounts.read`, cambios con
  `payments.settings.manage` (el permiso de los motivos de descuento), por la mutación de la página con `operation_id`.
- **Umbral de cuenta abierta** al final de «Funciones del establecimiento»: «Marcar una cuenta abierta después de
  (horas)», 1–72 con la entrada numérica de la 7.1 (sin `type="number"`), explicación «Las cuentas abiertas más de N
  horas se marcan en ámbar en Mesas y en las tarjetas…» y «Guardar umbral» con revisión (`settings.features.manage`);
  un conflicto carga el valor vigente y al guardar las pantallas abiertas vuelven a consultar el mapa y las cuentas.
- **Reportes**: «Cuentas canceladas» (con «Autorizó») y «Cancelaciones por motivo» se ven y exportan bien sin cambios
  en la interfaz (títulos, columnas y filtros de Go).
- **Archivos de otros alcances con cambio mínimo** (aplicados con parche, sin tocar lo de BC): `OperationalPage.tsx`
  (BC: importar y mostrar el panel `cancelAccount`, quitar `'void'` de las acciones y «Se anulará» → «Se descartará»),
  `BillFooter.tsx` (la opción del menú; BC cambia «Cobrar» en el mismo archivo), `CaptureBill.tsx` y `AccountList.tsx`
  (BC y D: solo el aviso de la cuenta cancelada, el filtro y el texto de la tarjeta), `styles.css` (un bloque propio),
  `useRestaurantWorkspace.ts` (D: un tipo de panel), `MoreActions.tsx` (aclaración opcional) y las pruebas
  `capture.test.tsx`, `tablePanel.test.tsx` y `stage9dStyles.test.tsx` (de «Anular cuenta vacía» a «Cancelar cuenta»;
  el recorrido de destructivos exige también «Cancelar cuenta» en rojo).

Pruebas (copia aislada = `HEAD 4abe78d` + cambios de E, sin los de BC; clúster temporal 55499; puertos 8187/1425;
12/11/2026):

- **Vitest nuevo**: `pages/restaurant/cancelAccount.test.tsx` (10: permiso propio con importe de Go, confirmación roja y
  mesa libre; supervisor desde el principio con la contraseña retirada y el rechazo en su formulario; «Otro» sin
  descripción y el 422 junto al campo; bloqueo por pagos con su aclaración, «Ir a devoluciones» y sin permiso de
  devoluciones; bloqueo por ajustes con «Ver descuentos y cargos»; para llevar sin productos enviados; reintento
  incierto con el mismo `operation_id`; reintento incierto con supervisor sin contraseña que pide repetirla; cuenta que
  cambió en otra terminal) y `pages/stage9dCancelAdmin.test.tsx` (12: catálogo —lista con marcas, crear con posición,
  renombrar, desactivar y reordenar con revisión, solo lectura sin permiso—; umbral —entrada de texto numérica, fuera de
  intervalo, guardado con revisión y aviso, conflicto que recarga, sin permiso—; registro de la cuenta cancelada con
  «Autorizó» y sin supervisor; historial de `account.cancelled` y `account.discarded`; efecto en cocina; Cocina con
  «cancelación»; filtro «Canceladas o descartadas»; recorrido del código sin textos de «anular» cuentas). Ajustadas:
  `capture.test.tsx`, `tablePanel.test.tsx` y `stage9dStyles.test.tsx` (el recorrido de destructivos exige también
  «Cancelar cuenta» en rojo).
- `npx tsc --noEmit` sin errores; `npm test` **72 archivos, 688 pruebas aprobadas**; `npm run build` correcto;
  Prettier sin diferencias en los archivos de E.
- **Playwright** `e2e/workflow-stage9d-cancel-account.spec.ts` (Chromium y WebKit, datos reales de Go): bloqueo por pago
  con abono real (la opción de «Más…» habilitada con su descripción accesible), cancelación con permiso propio
  («Cliente se retiró sin pagar», 3 unidades enviadas, confirmación roja, Mesa E1 libre), cancelación por un usuario
  creado sin `accounts.cancel` con la autorización del administrador («Otro» con descripción, Mesa E2 libre), registro de
  Go con quién canceló y quién autorizó, «Cuentas canceladas» con «Autorizó» y «Cancelaciones por motivo» en pantalla y
  exportados a PDF y Excel, motivo nuevo con la marca de merma en Cobros y umbral de 6 h guardado (la prueba lo deja en
  12 h). **Suite completa: 17/17 en Chromium (6.8 min) y 17/17 en WebKit (6.7 min)**; después de dos ajustes menores
  (texto repetido en la confirmación de una cuenta sin productos enviados y una espera en Vitest) se repitió la
  especificación nueva: 1/1 en ambos motores. La especificación nueva se ordena antes de `workflow-stage9d-colors`
  (nombre fijado por el encargo); esa corrió después y aprobó en ambos motores.
- **Capturas** (claro y oscuro, 1280 × 720 y 1920 × 1080, Chromium y WebKit, 56): panel de Cancelar cuenta sin y con
  supervisor, bloqueado por pago, menú «Más…» con la aclaración, confirmación, catálogo de motivos y umbral, en el
  directorio de capturas de la fase (`shots9dE`), sin desbordamiento horizontal.
- Comprobación adicional sobre el árbol completo (E + cambios en curso de BC, copia sincronizada): `tsc` sin errores y
  `npm test` 74 archivos, 714 pruebas aprobadas.

## Correcciones de la revisión independiente (parte 1)

Agente Fix1, 12/11/2026, sobre `etapa-9d` (`ffcaed7`), sin subagentes y en paralelo con la fase D (interfaz). Atiende
el informe de la revisión independiente de `ecae96d..2c00064` (sin defectos críticos ni altos; uno medio y siete bajos)
con las decisiones del coordinador. Sin migraciones, permisos nuevos ni cambios de cálculo. Copia aislada = `HEAD` +
solo los archivos de Fix1 (sin los cambios en curso de D); parche desde `HEAD` en las notas de la parte.

- **M1 · Una cuenta vacía no se «cancela»**: `cancel-account` sobre una cuenta `discardable` (sin productos enviados ni
  registros, aunque tenga borrador, como las #1, #3, #5, #8, #9 y #10 de la instalación real) responde **409
  `ACCOUNT_DISCARDABLE`** «Esta cuenta no tiene productos enviados: descártala; no hace falta motivo.», después del
  permiso de la ruta y de la revisión y **antes** del motivo y de la contraseña del supervisor (no cuenta en el límite de
  intentos), sin escribir nada. Así ninguna cuenta vacía queda como cancelación de $0 en los reportes (§4.2). En esas
  cuentas `cancel_requires_authorization` es `false` y `cancel_blocker` vacío; no se agregó `"discardable"` a
  `cancel_blocker` porque el tipo es cerrado y la interfaz de la fase E trata cualquier valor como bloqueo financiero (la
  interfaz decide por `discardable`; ofrecer «Descartar» en su lugar es de la fase D). Ninguna prueba de Go del repositorio
  cancelaba cuentas vacías (solo la reproducción del revisor, en su copia).
- **B2 · Respuesta después de cocina**: la respuesta de `cancel-account` y su resultado idempotente se leen después de
  integrar cocina: con una estación inactiva dicen `change_pending`, igual que una lectura posterior y que un reintento con
  el mismo `operation_id`; el evento conserva la cuenta previa a la integración.
- **B5 · Nombre del supervisor en el historial**: el evento `account.cancelled` guarda `authorized_by_name` (nombre
  visible, sin datos sensibles) y el historial dice «Autorizó: <nombre>», como el detalle y «Cuentas canceladas» (un
  evento anterior sin el campo muestra el usuario).
- **B1 · Preclasificación en Inventarios**: Go marca `preclassified` y entrega la etiqueta «Preclasificada al cancelar la
  cuenta» cuando la clasificación sigue en su primera revisión, es manual y su solicitud viene de un `account.cancelled`
  (sin migración; la restricción de forma de la etapa 8 no cambia). Ajustes y mermas ya no la atribuye al encargado:
  «Clasificó: Al cancelar la cuenta» y «Canceló la cuenta: <nombre>». En cuanto el encargado la cambia vuelve a
  «Clasificada por el encargado».
- **B4 · `restaurant_void` fuera del puente**: se retiró de la lista cerrada de `api.rs` (su prueba Rust exige ahora que
  no tenga ruta) y del tipo de operaciones de `services/api.ts`; ningún servicio la llamaba. Go conserva
  `POST restaurant/accounts/{id}/void` por compatibilidad (sus pruebas de integración la usan para crear anulaciones
  anteriores), pero ningún cliente de la aplicación la alcanza.
- **B3 · Rojo solo para lo que no se deshace (§1.5)**: vuelven al estilo secundario los «Quitar» que solo vacían un campo o
  quitan un renglón de un formulario sin guardar (hora, fecha, color, imagen del catálogo, `RemoveButton` del catálogo,
  renglones de ajustes, saldo inicial, compras y recetas, selección de un conteo nuevo, «Borrar captura» de un renglón de
  conteo, bloque del diseñador con «Deshacer» y el icono «Quitar el dato»). Siguen en rojo Eliminar, Descartar, Cancelar
  cuenta/unidades/ajuste/orden/conteo/arqueo, Liberar mesa, Revocar, Retirar logotipo y de prueba y lo que se aplica al
  instante («Quitar nivel», «Quitar porción», insumos surtidos de un proveedor, renglón de un conteo abierto).
- **B7 · Pruebas de la fase A**: `styles.test.ts` detecta los 148 nombres de color de CSS y los colores del sistema
  (también en `fillStyle`/`strokeStyle`), con una prueba del detector; `stage9dStyles.test.tsx` separa los destructivos
  de la lista explícita y justificada de reversibles (falla si uno se pinta de rojo o si la lista queda obsoleta) y revisa
  las etiquetas por propiedad (`confirmLabel` y `label` de «Más…» y `ChoiceDialog` exigen `tone: 'danger'`).
- **B6 · Sin cambio de cálculo**: «Cancelaciones por motivo» suma lo activo al cancelar; lo cancelado antes por unidades
  está en «Productos cancelados». Queda como nota para `docs/REPORTS.md` en las notas de la parte.

Pruebas (copia aislada `repo9dFix1` = `HEAD ffcaed7` + archivos de Fix1; clúster temporal 55499; puertos 8186/1424):

- **Go**: `gofmt -l` sin salida; `go vet ./...` con y sin `-tags=integration`, aprobado; `go test -race` de `operations`,
  `inventory`, `reports`, `kitchen` y `payments`, aprobado. Integración (`iter_snap.sh`): `internal/database` ok (7.0 s),
  `internal/reports` ok (125.1 s) e `internal/operations` ok (251.4 s; repetida sobre el código final: 254.8 s), con
  `TestStage9DAccountCancellation` ampliada: rechazo `ACCOUNT_DISCARDABLE` de cuentas para llevar vacías y de mesa solo
  con borrador (con permiso propio, sin permiso, con contraseña incorrecta que no cuenta en el límite de intentos y con
  «Otro» sin descripción; nada escrito), subprueba nueva con la estación inactiva (respuesta, lectura y reintento
  idempotente en `change_pending`; tras reactivar y reintentar cocina, un solo aviso y todo cancelado), nombre del
  supervisor en el evento y la preclasificación en `GET inventory/cancellations` (y su paso a «Clasificada por el
  encargado» al corregirla). Sin las correcciones, la subprueba de cocina y la de descarte fallan (comprobado).
- **Rust**: `cargo test --locked --offline` 41 aprobadas, 1 ignorada; `rustfmt --check` limpio.
- **Interfaz**: `npx tsc --noEmit` sin errores; `npm test` 74 archivos y **719 pruebas** aprobadas (HEAD: 714); `npm run
build` correcto; Prettier limpio en los archivos de Fix1. Pruebas nuevas: detector de los 148 nombres de color,
  etiquetas destructivas por propiedad, detector de propiedades, `RemoveButton` secundario y la preclasificación en
  Ajustes y mermas; ajustadas: recorrido de destructivos (lista de reversibles) e historial de «Cuenta cancelada» con el
  nombre del supervisor.
- **Playwright** (puertos 8186/1424) `workflow-stage9d-cancel-account` y `workflow-stage9d-colors`: 2/2 en Chromium y 2/2
  en WebKit. La suite completa queda para la regresión final.

## Correcciones de la revisión independiente (parte 3)

Agente Fix2, 12/11/2026, sobre `etapa-9d` en `04e6a78`, sin subagentes. Solo interfaz: sin Go, migraciones, contratos,
`api.rs` ni cambios en las reglas de negocio. Copia aislada `repo9dFix2` (HEAD exacto + estos cambios); clúster temporal
55499, Go temporal 8187, vista previa 1425.

- **P1 · «Liberar mesa» sin repetir «Descartar cuenta vacía»** (`TablePanel.tsx`): cuando el panel de la mesa muestra
  «Liberar mesa» (Go marca la cuenta `releasable` y no hay productos nuevos sin guardar), «Más…» ya no ofrece además
  «Descartar cuenta vacía» (mismo efecto, dos acciones rojas). En Cuentas, Para llevar, la captura y una mesa no liberable
  no cambia nada (`accountMoreActions` sigue igual; el filtro es del panel).
- **P2 · Rechazo definitivo de una cuenta nueva con la captura ya cerrada** (`draftSync.ts`, `useRestaurantWorkspace.ts`):
  si se sale de la captura con el primer guardado en curso (Para llevar, «Cuenta sin mesa» y la cuenta pendiente de mesa
  de la 7.1) y Go lo rechaza de forma definitiva (por ejemplo, un producto desactivado en otra terminal), el motor del
  borrador olvida la cuenta pendiente (sus toques no se reenvían: nadie puede corregirlos fuera de la captura) y lo
  informa (`onCreationRejected`). La pantalla avisa con `notifyError`: el mensaje es el texto de Go (sin código interno) y
  la descripción dice qué no se guardó y cómo volver a capturarlo («El pedido nuevo para llevar y sus productos no se
  guardaron. Vuelve a capturarlo con «Nuevo pedido».», «La cuenta nueva sin mesa…», «La cuenta nueva de Mesa A…»). Quita
  el identificador local de la selección y cierra su captura si seguía abierta en su vista: se vuelve al listado (con el
  primer pedido o cuenta elegido) o al mapa (la mesa libre), sin detalle vacío; también se descarta la selección de
  personalización de esa cuenta pendiente si la había. Si el rechazo llegó con la captura a la vista, el error sigue en
  la cuenta con sus productos como antes y el aviso aparece al salir. `leavePending` usa `abandonPending` (sustituye a
  `discardPendingIfIdle` en ese punto; lo pendiente de la cuenta se envía de inmediato al salir, como `flushCurrent`).
  **Sin cambios** en un resultado incierto (sigue con «Recuperar resultado del borrador» fuera de la captura y el mismo
  `operation_id`; si al recuperarlo Go lo rechaza, se avisa igual) ni en `TABLE_HAS_ACCOUNT` (su decisión ya se muestra
  fuera de la captura). `OperationalPage.recoverDraft` ya no muestra «Productos nuevos guardados» si la recuperación
  terminó en ese rechazo (la cuenta pendiente ya no existe).
- **Extensión de P2 (mismo mecanismo)**: una cuenta pendiente **vacía** que se deja desde otra vista (por ejemplo,
  «Nuevo pedido» → ⌘1/Mesas → tocar una mesa) cerraba su selección pero no su captura, y al volver a Para llevar se abría
  la captura del primer pedido del listado (reproducido en Vitest antes de corregir). Ahora `leavePending` también
  cierra esa captura cuando la cuenta pendiente es de otra vista.
- **P3 · Nota** (`docs/NAVIGATION_AND_SHORTCUTS.md`, «La barra durante la captura»): la captura se conserva por sección;
  si se va a otra sección sin «‹ Mesas»/«‹ Para llevar», al volver se retoma la captura de la mesa o del pedido; para el
  mapa o el listado se toca «‹ Mesas»/«‹ Para llevar». Sin cambio de comportamiento.

Pruebas (copia `repo9dFix2` = `04e6a78` + estos cambios; 12/11/2026):

- **Vitest nuevo**: `draftSync.test.ts` (+6, «creación con la captura ya cerrada»): rechazo definitivo después de salir
  (se olvida, se informa una vez, no se reenvían los toques acumulados), rechazo con la captura a la vista (el error
  sigue en la cuenta; al salir se informa), salir sin productos y creación correcta tras salir, envío inmediato al salir
  y toques que se anulan, resultado incierto (no se informa ni se olvida; otra recuperación incierta tampoco; la misma
  operación pasa a la cuenta real; un rechazo al recuperar se informa) y `TABLE_HAS_ACCOUNT` con la captura cerrada
  (sigue esperando la decisión; «Abrir una cuenta nueva» rechazada se informa). `stage9dAccounts.test.tsx` (+7,
  «correcciones de la revisión (parte 3)»): P1 (panel con «Liberar mesa» como única acción roja y sin «Descartar» en
  «Más…»; en la captura de la misma cuenta «Más…» sí lo ofrece); P2 en los tres caminos con la apertura retenida en el
  puente y rechazada después de «‹ Para llevar» / «‹ Cuentas» / «‹ Mesas» (aviso con el texto de Go y la descripción, sin
  código interno, una sola petición, nada creado, listado con su primer pedido o cuenta, mapa con la mesa libre y una
  captura nueva vacía); rechazo con la captura a la vista y salida; pendiente vacío dejado desde Mesas; incierto con la
  captura cerrada (panel de recuperación, sin aviso de error, mismo `operation_id`, una sola apertura en «Go», el pedido
  queda elegido). Las seis pruebas que dependen de la corrección fallan sobre `04e6a78` (comprobado).
- `npx tsc --noEmit` sin errores; `npm test` **75 archivos, 751 pruebas aprobadas** (738 + 13); `npm run build`
  correcto; Prettier sin diferencias en los 7 archivos.
- **Playwright** (Go temporal 8187, vista previa 1425, base nueva por motor) con `restaurant.spec.ts`,
  `workflow-stage9d-accounts.spec.ts` y `workflow-stage9d-topbar-tables.spec.ts` sobre el código final: **3/3 en
  Chromium (2.0 min) y 3/3 en WebKit (1.9 min)** (registro `docs9dFix2/e2e_final.log`; una corrida anterior, con parte
  del código previo al envío inmediato de `abandonPending`, también aprobó 3/3 y 3/3). La suite completa queda para la
  regresión final.
- **Parche** `docs9dFix2/fix2_desde_HEAD.patch` (desde `04e6a78`, 7 archivos): `git apply --check` limpio sobre el árbol
  real sin cambios y aplicado (sin commit); los 7 archivos del árbol real quedan idénticos a la copia probada.

Archivos: `desktop/src/pages/restaurant/capture/draftSync.ts`, `desktop/src/hooks/useRestaurantWorkspace.ts`,
`desktop/src/pages/restaurant/TablePanel.tsx`, `desktop/src/pages/restaurant/OperationalPage.tsx` (condición del aviso
de `recoverDraft`), `desktop/src/pages/restaurant/capture/draftSync.test.ts`,
`desktop/src/pages/restaurant/stage9dAccounts.test.tsx` y `docs/NAVIGATION_AND_SHORTCUTS.md`.
