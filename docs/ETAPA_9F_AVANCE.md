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
