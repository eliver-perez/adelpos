# Etapa 9F · Avance

Regresión en Mac, impresión, cocina por estaciones y pendientes. Especificación del propietario:
[etapas/ETAPA_9F.md](etapas/ETAPA_9F.md) (de solo lectura). Rama `etapa-9f`, creada desde `main` (0.9.4, esquema 21,
con la 9E fusionada; `git pull` sin cambios el 13/11/2026). Se trabaja en la Mac (Apple Silicon, macOS 26).

**Forma de trabajo**: un coordinador y como máximo 2 agentes a la vez, sin subagentes de subagentes; la Fase 0 la hace un
solo agente. Las pruebas corren en una copia aislada en el directorio temporal de la sesión, con un clúster PostgreSQL
temporal (puerto 55499) y Go temporal (8183, vista previa 1421); nunca contra la instalación real (`adel_pos`, `.local`,
Keychain, servidor 8181). Regla de autonomía de `CLAUDE.md` §0.

**Memoria de trabajo**: al iniciar se intentó actualizar la memoria de trabajo del asistente para que coincida con la
regla de autonomía de `CLAUDE.md` §0 (quitar «nunca push» y «pedir autorización para cada migración»); **el entorno no lo
permitió** (el clasificador de permisos rechazó la escritura). Se aplica igualmente la regla de `CLAUDE.md` §0 y de la
especificación.

## Fase 0-R · Regresión de la 0.9.4 en macOS e instalación

### Regresión

13/11/2026, sobre una copia exacta de `145640e` (`main` con la 9E, = `etapa-9f` al iniciar) con `git archive`, clúster
temporal 55499, Go temporal en 8183 y vista previa en 1421; pantalla del equipo desbloqueada antes y después de
Playwright. 1684 s.

| Suite                                    | Resultado                                                                                                                                                                                         |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato (Prettier, `gofmt`, `cargo fmt`) | aprobado (finales de línea de `.gitattributes` sin diferencias en macOS)                                                                                                                          |
| Metadatos (`sync-metadata --check`)      | aprobado                                                                                                                                                                                          |
| `go vet` (normal y `-tags=integration`)  | aprobado                                                                                                                                                                                          |
| `GOOS=windows go build ./...` y `go vet` | aprobado                                                                                                                                                                                          |
| Go unitarias con `-race`                 | 22 paquetes `ok`                                                                                                                                                                                  |
| Integración PostgreSQL con `-race`       | 9 de 9: `database` 7.2 s, `administration` 65.6 s, `catalog` 9.1 s, `operations` 253.2 s, `payments` 2.4 s, `reports` 122.5 s, `reports/operational` 41.2 s, `designer` 20.6 s, `identity` 18.0 s |
| `tsc --noEmit`                           | sin errores                                                                                                                                                                                       |
| Vitest (`npm test`)                      | 76 archivos, 757 pruebas aprobadas                                                                                                                                                                |
| `npm run build`                          | correcto (aviso de tamaño de fragmentos ya conocido)                                                                                                                                              |
| `cargo test --locked --offline`          | 41 aprobadas, 1 ignorada                                                                                                                                                                          |
| Playwright Chromium                      | 19 de 19 (7.5 min)                                                                                                                                                                                |
| Playwright WebKit                        | 18 de 19 en la corrida completa; **repetición completa: 19 de 19** (7.3 min, sin errores de página)                                                                                               |
| `.app` (`tauri build --bundles app`)     | 0.9.4, Mach-O arm64, 13.54 MiB                                                                                                                                                                    |

**El fallo de WebKit**: `workflow-stage9d-topbar-tables` completó todos sus pasos, pero al final encontró un error de
página «The provided value is non-finite» (una excepción de JavaScript sin capturar, de origen no identificado). La
especificación sola, con la traza de los errores de página activada, aprobó sin errores, y la repetición completa de
WebKit también (19 de 19, ningún error de página). Se registra como **intermitente, de origen no identificado**; no
apareció en Chromium ni en Windows (9E). Queda como observación para la regresión final.

Lo que la 9E cambió desde Windows y se revisó en macOS: arranque y cierre de procesos (`server-stop` ordenado,
`server-start`, cierre de SSE al apagar), credenciales (la aplicación abrió con la credencial de Keychain existente), rutas
(sin cambios en macOS), impresión (CUPS en macOS por `system_queue_unix.go`, cubierta por las pruebas Go), finales de
línea (`.gitattributes`: el formato y el `git status` quedaron limpios) y el foco y Escape en diálogos (especificaciones
`workflow-stage9c-keyboard` y `workflow-stage9d-*` en ambos motores). No hubo nada que corregir.

### Instalación de la 0.9.4 en la Mac (regla de autonomía de §0)

Sin migraciones (el esquema sigue en 21):

1. Comprobación de solo lectura (transacción `read_only`, solo conteos y nombres de configuración): migraciones 1–21,
   134 tablas, 154 permisos, cuentas `closed:2, open:7, voided:1`, 15 unidades, 2 pagos, turnos `closed:1, open:1`, 5
   operaciones de caja, 0 movimientos de inventario, 149 eventos de auditoría, 9 reconocimientos de venta, **0 turnos en
   arqueo y 0 trabajos de impresión sin terminar**. Repetida justo antes de detener el servidor: idéntica.
2. Respaldo `backups/adel_pos-20261113T144419Z-6e62f4.dump` (674 769 bytes, catálogo legible).
3. Ensayo de restauración: «Restauración completa verificada: 134 tablas con conteos idénticos».
4. `server-stop` ordenado; `migrate` → `applied:0` (y otra vez `applied:0`).
5. `server-start`: readiness 200; `api/v1/version` → **0.9.4 / esquema 21**. Comprobación posterior idéntica a la previa.
6. `build-macos` → `AD-EL POS.app` 0.9.4 (13.54 MiB); `open-app` → la aplicación 0.9.4 quedó abierta.

## Fase 0 · Verificación contra el código

Un solo agente, de solo lectura (13/11/2026). Informe completo con citas `archivo:línea` en el directorio temporal de
la sesión (`phase0_report.md`); aquí, lo que pide §2 y las decisiones del coordinador.

1. **Datos de prueba en instalaciones reales.**
   - Go de integración: bases `adel_pos_test_*` creadas y borradas por el script; cada fixture rechaza cualquier otra
     base (18 archivos). Playwright: base `adel_pos_test_browser_*` con Go temporal en 8183, protegida solo por el
     script (`desktop/e2e/native-driver.ts` acepta cualquier dirección).
   - **La única prueba que escribe en una instalación real es la prueba de humo de Windows**
     (`scripts/windows/installed-smoke.ps1` + `desktop/e2e-installed/installed-smoke.spec.ts`): abre la aplicación
     instalada con su carpeta de datos real (`%LOCALAPPDATA%\AD-EL POS`), sin ninguna protección. Creó toda la instalación
     de Windows: el establecimiento «Fonda Doña Ñeli», la categoría «Platillos» y el producto «Filete de prueba»
     (`FILETE`), la impresora «Destino controlado E2E» (transporte de agente, destino «Agente de aceptación no
     disponible»), las estaciones «Cocina caliente» y «Freidoras» con esa impresora, la ruta de «Platillos» a las dos
     estaciones, la impresora de tickets = ese destino (por eso las precuentas nunca imprimen), «Pago en dólares», el área
     «Salón principal» con «Mesa 1» y «Mesa 2», y operación real (turno, cobros, cancelación, corte y cierre).
   - **No hay un prefijo común** en los datos de prueba: solo «Destino controlado E2E» lleva una marca.
   - **La Mac no tiene datos de prueba**: comprobación de solo lectura de la base real (transacción `read_only`, solo
     conteos y nombres de configuración): 0 impresoras, 0 estaciones, 0 rutas, 0 estaciones por terminal y ningún
     producto ni categoría con nombre de prueba (4 categorías, 6 productos; cuentas `closed:2, open:7, voided:1`; 0
     turnos en arqueo y 0 trabajos de impresión sin terminar; servidor 0.9.3, esquema 21).
2. **A qué impresora va cada documento.** Comandas y avisos: ruta del producto o de su categoría → estaciones →
   impresora de cada estación (sin ruta o con la impresora inactiva, la liberación se bloquea con el motivo; con una
   impresora de tipo «agente» el trabajo entra a la cola como «Requiere intervención»). Precuenta, ticket y documentos
   de caja: la «Impresora de tickets» de Cobros › Políticas de cobro (sin ella, la precuenta responde
   `409 PRINTER_NOT_CONFIGURED` y el ticket y los documentos de caja `not_configured`; con una inactiva o de agente, el
   trabajo entra a la cola en «Requiere intervención» con «El destino está inactivo o requiere un agente de impresión
   aún no implementado», y la interfaz dice «Precuenta: Requiere intervención. Se resuelve en Cocina → Cola de
   impresión»). Cajón: la impresora de cada caja. Prueba técnica, prueba de ancho y muestras: la impresora elegida.
3. **Reintentos.** Hoy Go exige motivo (5–500) **también para reintentar** un trabajo en `error` o `intervention` (que
   nunca se envió) y para resolver un incierto o pedir una copia adicional de cocina; en cambio, reimprimir una
   precuenta o un ticket y los documentos de caja **no** piden motivo. La regla «motivo solo al reimprimir lo que ya
   salió en papel» no está escrita en la 9B. Estados: nunca salió en papel = `error` e `intervention`; pudo salir =
   `uncertain`; aceptado por el transporte = `submitted` (no confirma papel).
4. **Corte de papel.** Ya existen por impresora `supports_cut`, `cut` y `cutter_option` (desactivados por omisión): solo
   corte total `GS V 0` después de 3 saltos fijos, por ESC/POS de red, Windows RAW y CUPS raw con «entiende ESC/POS»;
   por CUPS de texto se usa la opción del controlador y, sin ella, el trabajo queda en «Requiere intervención». La prueba
   de ancho ya corta si la impresora tiene el corte activo. Cambiar una impresora exige `kitchen.printers.manage` y se
   audita (`kitchen.printer.saved`). No hay archivos de referencia byte a byte: la equivalencia se comprueba contra los
   renderizadores anteriores conservados en las pruebas.
5. **Estaciones de cocina.** Un producto se asigna a estaciones por la ruta del producto o de su categoría (1–20
   estaciones). Se crea una tarea por unidad y estación con las partes que llegan a esa estación: un combo reparte sus
   componentes según sus rutas, pero **un producto simple con dos estaciones llega completo a ambas**. «Marcar producto
   listo» actúa por estación y la partida queda lista cuando todas sus tareas lo están. **El filtro por terminal existe
   pero es otra cosa**: `kitchen_device_stations` es el ámbito de autorización de quien no tiene
   `kitchen.stations.all` (vacío = ninguna) y se ignora para quien lo tiene. **No existe autorización por usuario**: «Todas
   las estaciones autorizadas» significa el permiso global `kitchen.stations.all`. La pantalla de inicio por terminal
   ya admite Cocina y abre en Preparación. **Los «4 filetes» en Freidoras y Cocina caliente eran configuración**: la
   prueba de humo creó la ruta de la categoría «Platillos» hacia las dos estaciones (`desktop/e2e/kitchen-flow.ts`).
6. **«Ticket de venta» del sistema anterior = nuestra precuenta.** Lo imprimía el botón «Imprimir Venta», que solo lista
   ventas **sin pagar** (`legacy-csharp/PoS/SeleccionaVentaImprimir.cs:57`, `Pedidos.cs:77`); la impresión después de
   cobrar está comentada (`PuntoDeVenta.cs:401`, `Pedidos.cs:299`) y el documento no llevaba título. Se entregaba antes
   de cobrar.
7. **Textos repetidos.** «Mesa Mesa 1»: `backend/internal/payments/documents.go` antepone «Mesa » al nombre de la mesa,
   que ya se llama «Mesa 1» (dato, no plantilla). «Mesa 2 · Mesa 2»: `desktop/src/pages/kitchen/PreparationBoard.tsx`
   muestra el nombre de la cuenta (la referencia neutra de la 6.2, que es el nombre de la mesa) seguido del nombre de la
   mesa.

### Decisiones del coordinador (ninguna cambia una decisión de la especificación)

- **D1 · Datos de prueba.** Protección nueva: modo de pruebas declarado en la configuración del servidor
  (`test_mode`), que se niega a arrancar si la base no empieza por `adel_pos_test_`; en el `host` de Windows, con
  `ADEL_TEST_MODE=1`, se niega a usar la carpeta de datos real del usuario. La prueba de humo de Windows pasa a usar una
  carpeta de datos temporal y la borra al terminar. «Quitar datos de prueba» identifica lo heredado **por la firma exacta
  de los archivos de prueba** (nombre más atributos que solo la prueba crea, lista cerrada en Go), nunca por un nombre
  genérico suelto. Como la operación de la prueba de humo (ventas, turno) no se toca, la instalación de Windows no queda
  «limpia»: se documenta que lo limpio es reinstalar con otra carpeta de datos.
- **D2 · Respaldo antes de borrar.** Go crea el respaldo (`pg_dump`) también en macOS (en Windows ya existe), con la
  ruta de los binarios de PostgreSQL configurable; si no se puede respaldar, no se borra nada.
- **D3 · Motivos.** Reintentar un trabajo que nunca salió (`error`, `intervention`) no pide motivo; resolver un incierto
  lo sigue pidiendo; **reimprimir** un documento ya impreso pide motivo en cocina, precuentas, tickets y documentos de
  caja (como pide §8 de la especificación).
- **D4 · Impresora válida** para precuentas, tickets y documentos de caja = configurada, activa y con un transporte
  implementado (no «agente»); si no, aviso inmediato con «Configurar impresora» y sin encolar. Las comandas conservan su
  comportamiento (la liberación ya se bloquea con impresora inactiva).
- **D5 · Corte de papel.** Se conserva «Cortar el papel al terminar» (`cut`) y se agregan el tipo (total o parcial) y
  los renglones de avance (0–8; 4 en impresoras nuevas; 3 en las existentes, para no cambiar su papel). Sin corte, los
  bytes no cambian. Por CUPS de texto sin opción del controlador se imprime sin corte en lugar de quedar en
  «Requiere intervención». Activado por omisión solo en impresoras **nuevas** de 80 mm.
- **D6 · Estaciones.** `kitchen_device_stations` pasa a ser «Estaciones que muestra esta terminal» (con una marca
  «todas», predeterminada) y se agrega «Estaciones que puede atender» por usuario (todas por omisión; quien tiene
  `kitchen.stations.all` siempre todas). Se ve la intersección; sin estaciones en común, un aviso.
- **D7 · Colores por tiempo.** Ámbar desde la advertencia de la estación (o el 75 % del objetivo si la advertencia no
  es menor que el objetivo) y rojo al pasar el objetivo.
- **D8 · Ruta `void`.** Se retira (responde `410 ROUTE_RETIRED`); solo la usaban pruebas.
- **D9 · Barra compacta.** Se verifica además a 1200 px (la ventana de unos 1500 px de Windows al 125 %).

## Fase D · Ajustes de interfaz (barra superior compacta y leyenda del mapa)

Un agente, solo interfaz (React, CSS, pruebas y documentación; sin Go, Rust ni migraciones), sobre `b8d707b`.

### Barra superior compacta (§6.1, decisión D9)

- **Isotipo de la marca**: `components/BrandLogo.tsx` muestra en la barra compacta (debajo de 1280 px) el isotipo «AE»
  `desktop/src/assets/adel-pos-isotipo.png` (128 × 128, 13 KB), generado con `sips -z 128 128` desde
  `desktop/src-tauri/icons/icon.png` (el mismo del instalador, con su fondo blanco, su marco azul y las esquinas
  transparentes: se lee igual en tema claro y oscuro), con texto alternativo «AD-EL POS» y 40 × 40 px en la barra. Se
  retiraron la «A·» provisional (`.brand-mark`) y sus tres tokens (`--brand-mark-*`). `assets/brand/` no cambió.
- **Contorno de foco del logotipo**: reproducido en Chromium (el motor de WebView2): tras un clic el botón conservaba el
  foco y la tecla siguiente (Escape, una flecha, ⌘2) o el cierre de la paleta (que devuelve el foco al control anterior)
  le encendía `:focus-visible`; en WebKit no ocurre (los botones no toman el foco con el clic). `navigation/TopBar.tsx`
  (`goHome`) suelta el foco cuando la pulsación viene del dedo o del ratón (`event.detail > 0`); con Intro o Espacio
  (`detail = 0`) el foco y el contorno se quedan. Solo el logotipo (lo que pide §6.1).
- **Caja** (`pages/cash/CashShortcut.tsx`): el estado va en texto completo y en texto corto
  (`shortcutStateShortLabels`: «Abierto», «En arqueo», «Cerrado», «Consultando…»); en la compacta se ve el corto. La
  ayuda emergente (`title`) está siempre, también sin atajo: «Caja · Turno abierto · Ir a Caja: ⌘4».
- **«En línea»** (`navigation/ServerStatus.tsx` y estilos): conserva su texto en la compacta; la ayuda emergente empieza
  por el estado («En línea: El equipo servidor responde…», «Sin conexión: No hay comunicación…»).
- **Usuario**: en la compacta vuelve a verse su nombre y rol; si no caben, solo él se acorta con puntos suspensivos.
- **Debajo de 1150 px** (fuera de los anchos verificados; decisión menor): el turno y «En línea» quedan como punto de
  color con su ayuda emergente y el usuario como icono, como la compacta anterior, para no desbordar a 1024 px. Medido:
  sin desbordes a 1024, 1100, 1149, 1150, 1180, 1200 y 1279 px, en línea y sin conexión, en ambos motores.
- `styles.css`: solo reglas de la barra (`.cash-shortcut-state-short`, `@media (max-width: 1279px)` y la nueva
  `@media (max-width: 1149px)`), los tokens de la «A·» y la regla de la marca a 720 px (`display: block` para la
  imagen).

### Leyenda del mapa (§6.2)

- `styles.css` (`.map-legend*`): sin `overflow-x: auto` ni `white-space: nowrap` en el contenedor; los dos grupos
  (estados y datos de cada mesa) se acomodan en dos renglones con `flex-wrap: wrap` y cada estado conserva junta su
  muestra, su icono y su texto (`white-space: nowrap` en `.map-legend-state`). `TableMap.tsx` no cambió. A 1200 px la
  leyenda ocupa dos renglones; a 1280 px y más, uno. Antes, a 1200 px, se desbordaba 64 px con barra horizontal.

### Error intermitente «The provided value is non-finite» (WebKit)

No se reprodujo en ninguna corrida de esta fase (WebKit sin errores de página en `workflow-stage9d-topbar-tables` ni en
la nueva). En `TableMap.tsx` y la barra todos los valores que llegan a las API del DOM son finitos por construcción
(`fitTables` devuelve `null` sin mesas o sin espacio; `scrollLeft`/`scrollTop` salen de diferencias de coordenadas del
puntero; `scrollTo` con ceros). Ya se había visto en la 9A en `restaurant.spec.ts`, antes del mapa con escala, así que
no parece venir del mapa. Origen sin identificar; no se persiguió más.

### Pruebas

Copia aislada del worktree (`sync_tree.sh`), Go temporal en 8185 y vista previa en 1423 contra el clúster temporal
55499; sin cambios en Go ni en Rust (no aplican `go vet`, `go test` ni `cargo test`).

- `npx tsc --noEmit`: sin errores. `npm test` (`vitest run src`): **77 archivos, 766 pruebas aprobadas**, con
  `navigation/navigation.test.tsx` («barra compacta (etapa 9F)»: isotipo con «AD-EL POS» y sin la «A·»; estado corto
  de Caja en sus tres estados con la ayuda emergente; «En línea» y «Sin conexión» con texto y ayuda que empieza por el
  estado; el logotipo suelta el foco con `detail = 1` y lo conserva con `detail = 0`; la prueba de la 9D de Caja en
  dos renglones, adaptada al texto completo y corto), el nuevo `src/stage9fLayout.test.ts` (5: reglas de la compacta y
  de debajo de 1150 px, sin `.brand-mark`, PNG de 128 × 128, leyenda con `flex-wrap: wrap` sin `overflow-x`) y
  `src/stage9dLayout.test.ts` (la prueba de la leyenda «en una sola línea» pasa a «sin desplazamiento horizontal»).
  `npm run build`: aprobado (el isotipo sale como recurso de 13.25 kB). Prettier: sin diferencias.
- Playwright nuevo `e2e/workflow-stage9f-topbar-map.spec.ts`, con una usuaria de rol administrador y nombre largo:
  barra a 1200, 1280, 1440, 1500 y 1920 px en ambos temas (sin desplazamiento horizontal en la página ni en la barra;
  cada botón dentro de la ventana, de al menos 44 × 44 px y sin nada encima; isotipo de 40 px cargado en la compacta y
  logotipo completo en las demás; «Caja Abierto» o «Caja Turno abierto» visibles; «En línea» visible con su ayuda;
  nombre visible y, si se corta, con puntos suspensivos); foco del logotipo a 1200 px (tras un clic no queda enfocado
  ni con contorno después de Escape, una flecha, ⌘2 y la paleta; con Mayúsculas+Tab —Opción en WebKit— llega con
  `:focus-visible` y contorno sólido, e Intro vuelve al inicio conservándolos); «Sin conexión» a 1200 px en ambos temas;
  leyenda sin desplazamiento horizontal en los cinco anchos y ambos temas, cada estado completo con su icono, dos
  renglones a 1200 px y uno a 1920 px. **Chromium 1/1 y WebKit 1/1**.
- Controles negativos (en la copia): sin el `blur` del logotipo la prueba falla en Chromium («focused: true» tras el
  clic); con la leyenda de la 9D falla a 1200 px («la leyenda se desplaza de lado», 64 px).
- Especificaciones existentes, juntas sobre una base por motor (`restaurant.spec`, `workflow-stage9c-navigation`,
  `workflow-stage9d-colors`, `workflow-stage9d-topbar-tables` y la nueva): **Chromium 5/5 y WebKit 5/5**, sin errores de
  página. `workflow-stage9c-keyboard` sola falló en ambos motores por la limitación conocida del filtro (base nueva sin
  la ruta de cocina: «Enviado · sin confirmar en cocina» en lugar de «En cocina»); junto con `restaurant.spec`:
  **Chromium 2/2 y WebKit 2/2**.
- Exploración (no entregada): sin desbordes a 1024, 1100, 1149, 1150, 1180, 1200 y 1279 px, en línea y sin conexión,
  en ambos motores. La corrida completa queda para el coordinador.
- Sin verificar: Windows/WebView2 real al 125 % (la reproducción del foco se hizo con Chromium de Playwright, el mismo
  motor), la ventana nativa y una pantalla táctil física.

### Documentación

`CLAUDE.md` §11–12 («Marca» y «Barra superior, Mesas y cuentas») y `docs/NAVIGATION_AND_SHORTCUTS.md` (párrafo de la
9F, barra compacta, Caja, estado del servidor y pruebas).

**Cierre de la fase D** (coordinador, copia exacta de `f56fcd9`, clúster temporal 55499, Go 8183, vista previa 1421,
pantalla desbloqueada): formato, metadatos, `go vet` (también `GOOS=windows`), Go unitarias con `-race`, `tsc`, Vitest
(77 archivos, 766 pruebas), `npm run build` y `cargo test` (41 + 1 ignorada) aprobados; Playwright de `restaurant`,
`workflow-stage9c-navigation`, `workflow-stage9d-*` y `workflow-stage9f-topbar-map` juntas sobre una base por motor:
**Chromium 7/7 y WebKit 7/7**.

## Fase A · Datos de prueba fuera de las instalaciones reales

Un agente en su propio worktree (desde `b8d707b`), sin migración (la marca es la firma exacta). Decisiones D1 y D2.

### Protección del modo de pruebas (D1)

- **Configuración** (`backend/internal/config/config.go`): campo opcional `test_mode`. Con `true`, `Validate` (que usan
  `config.Load` y todos los comandos) exige una base `adel_pos_test_*`: `serve`, `migrate`, `prepare-setup` y los demás
  se niegan a arrancar con «el modo de pruebas solo se usa con una base de datos de pruebas…». Sin el campo, nada
  cambia (`omitempty`: el `server.json` que escribe la instalación de Windows queda igual). Campos opcionales nuevos
  para el respaldo: `postgres_bin` y `backup_directory` (rutas absolutas).
- **`GET /api/v1/version`** informa siempre `test_mode` (`httpapi.NewWithOptions`; `serveHTTP` recibe el modo).
- **`desktop/e2e/native-driver.ts`**: antes de cualquier petición exige `test_mode: true` en `/version`; si no, falla
  con «Las pruebas solo corren contra un servidor en modo de pruebas». `scripts/browser-tests.py` y
  `scripts/windows/dev.mjs` (`test-browser`) escriben `test_mode: true` y `backup_directory` dentro de su carpeta
  temporal. El `e2e_port.sh` del coordinador lo detecta por el texto `"test_mode"` de `config.go` (está en el
  comentario del campo, porque la etiqueta lleva `omitempty`).
- **Windows `host`** (`cmd/ad-el-pos-server/host_windows.go`, `internal/standalone/testmode.go`, todo `//go:build
windows`): con `ADEL_TEST_MODE=1`, `host`, `backup`, `restore-check` y `restore` se niegan a usar la carpeta de datos
  real (`windows.KnownFolderPath(FOLDERID_LocalAppData)` + «AD-EL POS», comparada con `samePath`; no depende de la
  variable `LOCALAPPDATA`); otro valor no vacío es un error; `host` informa `test_mode: true`. Rust
  (`server_host.rs`) no limpia el entorno del proceso hijo, así que la variable llega a Go (comentario en `spawn`).
- **Prueba de humo de Windows** (`scripts/windows/installed-smoke.ps1`, UTF-8 con BOM, LF): exige la aplicación real
  cerrada y los puertos 8181, 55432 y 9222 libres; abre la aplicación con `LOCALAPPDATA` temporal,
  `ADEL_TERMINAL_CONFIG` con un `credential_namespace` propio (`humo-<12 hex>`, archivo JSON sin BOM) y
  `ADEL_TEST_MODE=1`; al terminar, pase lo que pase, cierra la aplicación en orden (fuerza los procesos que queden tras 60 s), borra la carpeta temporal, borra
  la credencial (`cmdkey /delete:"http://127.0.0.1:8181|<perfil>.local.adel.pos.dev"`) y avisa si el clúster real quedó
  en marcha. `desktop/e2e-installed/installed-smoke.spec.ts` exige `test_mode: true` (por `server_snapshot`) antes del
  primer paso. **No verificado en Windows** (no hay Windows en esta sesión).

### «Quitar datos de prueba» (D1, D2)

- **Paquete nuevo `backend/internal/maintenance`** (solo pruebas unitarias; no hace falta tocar la tupla de
  `scripts/database.py` ni `dev.mjs`: la integración vive en `internal/administration`):
  - `signatures.go`: lista cerrada con el archivo de prueba de cada firma: impresoras «Destino controlado E2E»
    (`e2e/kitchen-flow.ts`) y «Destino controlado 9B» (`e2e/workflow-stage9b-designer.spec.ts`), ambas `agent` con
    destino «Agente de aceptación no disponible»; estaciones «Cocina caliente» y «Freidoras» solo con una de esas
    impresoras; producto «Filete de prueba» `FILETE` `product`; categoría «Platillos» `#d64545` solo si contiene un
    producto de prueba (el rojo es el primer color de la paleta). `signatures_test.go` lee cada archivo de prueba y
    falla si ya no crea esos datos exactos.
  - `plan.go` y `references.go`: decide eliminar, desactivar (el producto además se archiva), desvincular o conservar.
    Los «registros relacionados» se leen de las llaves foráneas del catálogo de PostgreSQL (`pg_constraint`), no de una
    lista a mano: una tabla nueva que apunte a una estación, impresora, producto o categoría hace que el dato se
    desactive en lugar de borrarse. Los motivos están en palabras del restaurante («Ya tiene registros: ventas y tareas
    de cocina.»). Se conserva la impresora de prueba que usa una estación del restaurante y la categoría con otros
    productos o con una ruta con estaciones del restaurante. Se desvincula la impresora de tickets de Cobros, el cajón
    de cada caja (queda desactivado), la impresora de respaldo de otras impresoras, las estaciones por terminal y las
    estaciones de prueba en rutas que se conservan; la ruta cuyas estaciones son todas de prueba se elimina.
  - `apply.go`: una transacción en el orden de las llaves foráneas, eventos `kitchen.*.changed`,
    `payments.settings.changed`, `cash.settings.changed` (por caja) y `maintenance.test_data.removed`.
  - `backup.go`: `pg_dump --format=custom --no-owner --no-acl` con la conexión de la configuración, la contraseña solo
    en el entorno del hijo (se descartan las `PG*` heredadas), binarios de `postgres_bin`, `ADEL_PG_BIN` o Homebrew,
    carpeta `backup_directory` o `backups/` junto a la carpeta de la configuración, nombre
    `<base>-<AAAAMMDDTHHMMSSZ>-<6 hex>.dump` con 0600 (ACL privada en Windows) y `pg_restore --list`; si algo falla
    borra el archivo. En el `host` de Windows se usa `standalone.Backup` con la etiqueta
    `antes-de-quitar-datos-de-prueba`.
- **Rutas** (`administration/maintenance.go`, `settings.manage`): `GET maintenance/test-data` (vista previa con locks
  compartidos 7820148305, 7820148304 y 7820148308; `plan_id` = SHA-256 de lo que se elimina, desactiva y desvincula) y
  `POST maintenance/test-data/remove {operation_id, plan_id}`. La transacción de autenticación solo autoriza y comprueba
  la idempotencia (`restaurant_operations`; misma operación ⇒ respuesta guardada sin otro respaldo; otro plan ⇒ 409
  `IDEMPOTENCY_CONFLICT`); después (como los reportes, `completeDeferred`) el respaldo y, solo si quedó bien, una
  transacción con los tres locks exclusivos que recalcula el plan (409 `TEST_DATA_CHANGED` si no es el revisado),
  aplica, audita `maintenance.test_data_removed` con la lista y el nombre del respaldo y guarda la respuesta. Respaldo
  fallido: 503 `BACKUP_FAILED` sin quitar nada y auditoría con `success=false`; sin nada que quitar no se respalda.
  Tiempo de respuesta de `maintenance/*`: 120 s; el puente espera 130 s.
- **Puente**: `maintenance_test_data` y `maintenance_remove_test_data` en la lista cerrada de `api.rs`.
- **Interfaz**: `pages/MaintenancePage.tsx` y `services/maintenance.ts`; sección «Mantenimiento» en Sistema
  (`navigation/sections.ts`, `settings.manage`, icono `database`). «Revisar datos de prueba» → lista agrupada («Se
  eliminará», «Se desactivará», «Se desvinculará», «Se conserva») o «No hay datos de prueba.» → «Quitar datos de
  prueba» en rojo con `ConfirmDialog` `tone: 'danger'` («Primero se crea un respaldo de la base de datos…») → aviso
  «Datos de prueba quitados.» con lo eliminado, desactivado y desvinculado y «Respaldo previo: <archivo>» (sin
  carpeta). Conserva el `operation_id` ante una respuesta sin conexión. Estilos en un bloque propio al final de
  `styles.css`. «Registro de movimientos» nombra la acción («Datos de prueba quitados»).
- **Documentación**: `API_CONTRACTS.md` («Etapa 9F · modo de pruebas y Quitar datos de prueba»),
  `docs/KITCHEN_AND_PRINTING.md` («Datos de prueba · etapa 9F»), `docs/NAVIGATION_AND_SHORTCUTS.md` (mosaico),
  `CLAUDE.md` §6 («Modo de pruebas» y la frase de `installed-smoke.ps1`) y `DEVELOPMENT_WINDOWS.md`.

- Se ajustó `desktop/e2e/workflow-stage9c-navigation.spec.ts`, que comprueba los mosaicos exactos de Sistema (ahora
  con «Mantenimiento»), y `desktop/e2e/navigation.ts` (`openAdministration(page, 'Mantenimiento')`).

### Pruebas (copia aislada, clúster 55499, Go 8184 y vista previa 1422)

13/11/2026, sobre una copia del worktree; pantalla del equipo desbloqueada.

| Suite                                                               | Resultado                                                                   |
| ------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| Prettier (todo el worktree), `gofmt -l`, `cargo fmt`                | sin diferencias                                                             |
| `go vet` (normal y `-tags=integration`)                             | aprobado                                                                    |
| `GOOS=windows`: `go build ./...` y `go vet` (y `-tags=integration`) | aprobado (`standalone/testmode_test.go` solo compila; no se ejecutó)        |
| Go unitarias con `-race` (`./...`)                                  | 23 paquetes `ok` (nuevo `internal/maintenance`)                             |
| Integración `internal/administration` con `-race`                   | `ok` (74.6 s), con 6 subpruebas 9F, incluido el respaldo real con `pg_dump` |
| `tsc --noEmit`                                                      | sin errores                                                                 |
| Vitest (`npm test`)                                                 | 77 archivos, 764 pruebas (nuevo `pages/maintenance.test.tsx`, 7 pruebas)    |
| `npm run build`                                                     | correcto (aviso de tamaño de fragmentos ya conocido)                        |
| `cargo test --locked --offline`                                     | 42 aprobadas, 1 ignorada (nueva `maintenance_routes_are_fixed`)             |
| Playwright `workflow-stage9f-zz-test-data` sola, base nueva         | Chromium 1/1 y WebKit 1/1 (camino «eliminar»)                               |
| Playwright corrida completa Chromium                                | 20 de 20 (7.5 min; camino «desactivar», con ventas y trabajos de impresión) |
| Playwright corrida completa WebKit                                  | 20 de 20 (7.3 min)                                                          |

La primera corrida completa de Chromium falló en `workflow-stage9c-navigation` (esperaba exactamente cuatro mosaicos
en Sistema) y arrastró en cascada a las seis siguientes; se corrigió la especificación y la repetición aprobó 20 de 20.
En la corrida completa, la vista previa de Go listó: se elimina la ruta de «Platillos»; se desactivan «Cocina caliente»
y «Freidoras» (tareas y avisos de cocina), «Filete de prueba» (recetas de inventario, productos en cuentas abiertas y
ventas), «Destino controlado 9B» y «Destino controlado E2E» (trabajos de impresión); se desvincula la impresora de
tickets de Cobros; se conserva «Platillos» (tiene «Hamburguesa de prueba»). El respaldo previo quedó como
`adel_pos_test_browser_…-20261113T154512Z-b5c5b7.dump` (789 270 bytes, permisos 0600) en la carpeta temporal.

Subpruebas de integración 9F: permiso `settings.manage` (403 auditados) y cuerpo inválido; sin datos no se respalda;
dato real con nombre genérico intacto («Cocina caliente» con otra impresora, «Platillos» con un producto del
restaurante), impresora de tickets, cajón y respaldo de otra impresora desvinculados, respaldo fallido (503 sin quitar
nada y auditoría con `success=false`), idempotencia (misma operación sin otro respaldo, 409 con otro plan, 409 con un
plan viejo) y eventos; datos con venta y tareas desactivados sin cambiar los conteos de 16 tablas de operación;
categoría de prueba con su ruta «Sin preparación» eliminada; respaldo real con `pg_dump`/`pg_restore --list`.

**No verificado**: Windows (`host` con `ADEL_TEST_MODE=1` y la carpeta conocida, `installed-smoke.ps1` y la prueba
instalada; solo compilan o se revisaron), y «Quitar datos de prueba» sobre la instalación real de Windows (la hará el
propietario). La Mac no tiene datos de prueba (Fase 0): no se ejecutó en la instalación real.

**Riesgo para el coordinador**: `workflow-stage9f-zz-test-data.spec.ts` deja la base compartida sin impresora de tickets,
sin la ruta de «Platillos» y con las estaciones y los destinos de aceptación desactivados. Una especificación que se
ordene después (por ejemplo `workflow-stage9f-topbar-*`) no debe depender de ellos; si alguna lo hace, conviene que esta
corra al final.

## Fase E · Pendientes técnicos (ruta antigua de anulación y autorización para cancelar unidades)

Un agente sobre `f56fcd9`, en su propio worktree (Go, React, documentación y pruebas; **sin migración**). Pruebas en una
copia aislada con el clúster temporal 55499, Go temporal en 8187 y vista previa en 1425.

### Ruta antigua de anulación retirada (§7.1, decisión D8)

- `operations/handler.go`: `POST restaurant/accounts/{id}/void` responde **`410 ROUTE_RETIRED`** («Esta forma de
  cancelar cuentas ya no existe. Usa «Cancelar cuenta», con su motivo y, si hace falta, la autorización de un
  supervisor, o «Descartar cuenta vacía»…») **antes** de comprobar permisos, tomar el lock o leer la cuenta: no escribe
  nada (ni operación idempotente, evento, auditoría, tiempo real ni la denegación) y la transacción se revierte. Se
  quitaron `void` de `routePermission`, del despacho (`accounts_read.go`) y de `accountAction` (`accounts_write.go`;
  mover y asignar siguen aceptando `reason` como antes). No estaba en el puente ni en la interfaz (`api.rs` solo cambia
  sus comentarios; la prueba de Rust que exige que `restaurant_void` no tenga ruta sigue igual).
- Las cuatro pruebas que la usaban: `operations/acceptance_test.go` descarta la cuenta dividida vacía (`discard`,
  comprobando `discardable`); `operations/payments_acceptance_test.go` intenta `cancel-account` y espera
  `409 ACCOUNT_HAS_PAYMENTS`; `reports/cancellations_integration_test.go` y
  `reports/stage9d_cancellations_integration_test.go` crean la anulación heredada directamente en su base con
  `legacyVoid` (`reports/fixture_integration_test.go`: cuenta en `voided`, operación idempotente, evento `account.void`
  con el motivo y acuse de cocina, en una transacción; documentado como dato anterior a la 9F).
- **Pendiente para una migración** (no se escribió): la descripción de `accounts.manage` en `app.permissions` («Editar
  datos de cuentas y cancelar cuentas vacías con motivo libre», de la 0021) sigue mencionando la anulación retirada;
  `ensureFoundations` no reescribe descripciones, así que solo la cambia una migración. Si la 0022 existe, propuesta:
  `UPDATE app.permissions SET description='Editar datos de cuentas' WHERE key='accounts.manage'`, el mismo texto en
  `operations/context.go` y en `administration/long_open_hours_acceptance_test.go` (que hoy fija el texto actual).

### Autorización para cancelar unidades (§7.2)

- **Go** (`operations/supervisor_approval.go`, nuevo): `approveWithSupervisor(regla, autorización)` generaliza el
  `approveCancellation` de «Cancelar cuenta» (permiso propio o autorizador de `administration/supervisor.go`, sin
  autoautorización, `AUTHORIZATION_REQUIRED/INSUFFICIENT`, `SELF_AUTHORIZATION`); `authorizationFields` y
  `auditSupervisor` comunes.
  - `cancellations.go` («Cancelar unidades»): acepta `authorization`; con `orders.cancel` basta el permiso propio; sin
    él, supervisor con `orders.cancel`, después de validar motivo, revisión, cantidad y unidades libres y **antes de la
    primera escritura**. El permiso de la ruta baja: `accounts.read` y (`orders.cancel` **o** `accounts.open`) —decisión
    menor: así nadie que tenía `orders.cancel` sin `accounts.open` pierde el permiso propio—.
  - `amendments.go` («Modificar» que reduce): ya no exige `orders.cancel`; `edit` pide la autorización solo si la
    modificación deja menos unidades (después de la huella, antes de escribir).
  - Se registran quién lo hizo (autor del evento) y quién autorizó: `after.authorized_by`, `authorized_by_username` y
    `authorized_by_name` en `item.cancelled`/`item.modified` (null si bastó el permiso; sin columna nueva), la auditoría
    del evento con lo mismo y `restaurant.supervisor_authorized` con `scope` `item_cancel` o `item_modify`. La
    contraseña queda fuera de la huella (la de siempre) y de todo registro.
  - Go informa `cancel_units_requires_authorization` en cada cuenta abierta y en la respuesta de `edit-preview` (en las
    demás cotizaciones, `false`).
- **Interfaz**: `pages/restaurant/CancelItem.tsx` y la personalización de «Modificar» (`capture/CustomizationPanel.tsx`)
  muestran `components/SupervisorAuthorization.tsx` cuando Go lo indica o lo pide con un error
  (`useSupervisorAuthorization`, contraseña fuera del formulario al enviar); la confirmación roja dice «Autoriza:
  <usuario>». «Cancelar unidades» aparece con `orders.cancel` o `accounts.open` (`capture/BillLines.tsx`). «Recuperar
  resultado» (`OperationalPage.tsx`) pide repetir la autorización si el reintento sin contraseña vuelve con un error de
  autorización. El historial (`components/restaurant/HistoryDetails.tsx`) muestra «Autorizó: <nombre>» o «con su propio
  permiso» en las cancelaciones y reducciones nuevas. Tipos en `services/restaurant.ts` (`CancelUnitsInput`).
- **Reportes**: «Modificaciones de cantidades» (`reports/cancellations.go`) tiene la columna **«Autorizó»** después de
  «Modificó» (nombre visible actual del supervisor; en las partidas de una cuenta cancelada, quien autorizó cancelarla;
  vacía si bastó el permiso y en los registros anteriores), también en PDF y Excel; la nota que decía que no había
  «Autorizó» se reemplazó. Revisados los demás: «Cuentas canceladas» ya la tenía (sin cambio); **«Productos
  cancelados»** (`reports/products.go`) también la recibe, después de «Canceló», y «Actividad por empleado»
  (`reports/employees.go`) cuenta esas autorizaciones en «Autorizaciones como supervisor». Sin filtros nuevos; 95
  reportes.

### Pruebas

Copia aislada (`rsync` del worktree), clúster 55499, Go 8187 y vista previa 1425; pantalla desbloqueada.

- `gofmt`, `go vet ./...` y `go vet -tags=integration ./...`: sin hallazgos. `GOOS=windows GOARCH=amd64 go build ./...`
  y `go vet ./...`: aprobados.
- `go test -race` (unitarias): `operations`, `reports` y `administration` `ok`.
- Integración PostgreSQL con `-race`: `operations` **ok (291.4 s)**, con la nueva `stage9f_cancel_units_test.go`
  (`TestStage9FCancelUnitsAndRetiredVoid`, 5 de 5 subpruebas: ruta retirada con 410 y conteos de operaciones, auditoría,
  eventos y tiempo real sin cambio, con y sin permisos y con una cuenta inexistente; con el permiso propio; sin permiso
  —usuario sin `accounts.open` 403, cantidad imposible 400 sin contar intento, sin autorización, autoautorización,
  contraseña incorrecta con el intento contado y supervisor sin el permiso, sin cambiar la cuenta—, con supervisor,
  evento y auditoría, reintento con la misma operación sin contraseña y `IDEMPOTENCY_CONFLICT` con otros datos, la
  contraseña en ningún registro; reducción desde «Modificar» con la previsualización que lo informa, rechazos,
  supervisor y modificación sin reducir sin autorización; «Autorizó» en ambos reportes y 2 autorizaciones en la
  actividad) y las aceptaciones ajustadas; `reports` **ok (130.2 s)**, con las pruebas de la 9C («Autorizó» vacía con el
  permiso propio) y de la 9D («Autorizó» con el supervisor de la cuenta cancelada en ambos reportes). **Control
  negativo** (en la copia): sin el 410 y con la regla de autorización cambiada, la prueba nueva falla en 3 de 5
  subpruebas.
- `npx tsc --noEmit`: sin errores. `npm test`: **78 archivos, 770 pruebas aprobadas**, con el nuevo
  `pages/restaurant/cancelItem.test.tsx` (3: permiso propio sin autorización; supervisor pedido por Go con el envío
  bloqueado hasta usuario y contraseña, «Autoriza: admin» y contraseña borrada; un `AUTHORIZATION_REQUIRED` y un
  `AUTHORIZATION_INVALID` de Go mostrados en el formulario y el tercer envío con la autorización) y una prueba nueva en
  `capture/capture.test.tsx` (reducir desde «Modificar» muestra el formulario cuando la previsualización lo pide y la
  autorización va solo en la confirmación).
- `cargo fmt --check` y `cargo test --locked --offline`: **41 aprobadas, 1 ignorada**.
- Playwright nuevo `e2e/workflow-stage9f-cancel-units.spec.ts` (cajero sin `orders.cancel` cancela una unidad desde el
  menú del producto con la autorización del administrador; la ruta `void` responde 410 sin tocar la cuenta; el historial
  dice quién canceló y quién autorizó; «Modificaciones de cantidades» y «Productos cancelados» con «Autorizó» en
  pantalla y la primera exportada a PDF y Excel; capturas a 1280 × 720 y 1920 × 1080 en ambos temas sin desbordes):
  **Chromium 1/1 y WebKit 1/1** (cada uno solo, sobre una base nueva).
- Especificaciones existentes que dependen de lo cambiado, juntas sobre una base por motor (`restaurant.spec`, con
  «Modificar» y «Cancelar unidades» del administrador; `workflow-stage9d-accounts`, menú por producto;
  `workflow-stage9d-cancel-account`, «Cancelar cuenta» con supervisor y reportes) y la nueva: **Chromium 4/4 y WebKit
  4/4**, sin errores de página. Tras un último ajuste menor en `CustomizationPanel.tsx` (descartar lo escrito en el
  formulario de autorización oculto) se repitieron `restaurant.spec` y la nueva: **Chromium 2/2 y WebKit 2/2**.
- `npm run build`: correcto (aviso de tamaño de fragmentos ya conocido). Prettier: sin diferencias en los archivos
  tocados.
- Sin verificar: Windows/WebView2, la ventana nativa y una pantalla táctil física. La corrida completa de Playwright
  queda para el coordinador.

### Documentación

`API_CONTRACTS.md` (tabla de rutas, `void` retirada, nota del puente y sección nueva «Etapa 9F · ruta antigua de
anulación retirada y autorización para cancelar unidades»), `docs/RESTAURANT_OPERATIONS.md` (recorrido y sección nueva),
`docs/REPORTS.md` (párrafo 9F, «Autorizó», empleados, pruebas y limitaciones), `CLAUDE.md` (§11–12: campo calculado por
Go; §15: nota de la 9C, la ruta `void` retirada y un apartado «Etapa 9F, fase E») y `AGENTS.md` (las dos reglas que
decían que no había «Autorizó» y que `void` seguía en Go).
