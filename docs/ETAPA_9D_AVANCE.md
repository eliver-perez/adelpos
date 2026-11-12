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
