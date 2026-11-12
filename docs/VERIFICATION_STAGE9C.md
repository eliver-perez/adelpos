# Verificación · Etapa 9C

Verificación por fases del **11 de noviembre de 2026**, zona America/Monterrey, en la rama local `etapa-9c` (creada desde
`etapa-9b` por decisión del propietario): fase 0 `4a0a60f`; fase A `e575ad0` (cierre registrado en `e4b4e97`); parte D1
`e73b6b4`; fase C `1feaa59` (cierre en `a3f6534` y `eb782f8`); fase B `88d54f0` (cierre en `0036c65`); corrección de la
revisión en el catálogo `0dc0cfc`; parte D2 `ab755eb`; correcciones de interfaz de la revisión `41e31b7`; fase E
`d268600` (versión 0.9.2 y esquema 20). API v1, migración `0020_terminal_home_and_printer_columns.sql`. Evidencia
anterior: [etapa 9B](VERIFICATION_STAGE9B.md), [etapa 9A](VERIFICATION_STAGE9A.md), [etapa 8](VERIFICATION_STAGE8.md) y
las anteriores desde [VERIFICATION.md](VERIFICATION.md). Avance: [ETAPA_9C_AVANCE.md](ETAPA_9C_AVANCE.md). Guía:
[NAVIGATION_AND_SHORTCUTS.md](NAVIGATION_AND_SHORTCUTS.md).

**Alcance de esta evidencia.** Las cifras son las que cada fase registró en el avance y en sus registros de cierre (en el
directorio temporal de la sesión). De la fase A a la D se obtuvieron con `config/product.json` en 0.9.1 y esquema 20 (la
0020 embebida y el migrador aplicando 20 migraciones en cada base temporal); la versión 0.9.2 se fijó en la fase E. La
regresión completa sobre el commit final está en «Regresión final».

**Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432, `backups/`, el `.app`
instalado); la 0020 se aplicó después, con la autorización previa del propietario (ver «Instalación real»). No se leyeron secretos y
ninguna prueba envió trabajos a CUPS real, a una impresora, a un cajón ni a una conexión TCP fuera de loopback.

## Entorno aislado

Copias del repositorio en el directorio temporal de la sesión, una por fase o parte (sin `.git`, `.local`, `.tools`,
cachés ni artefactos), clúster PostgreSQL temporal en el puerto 55499 (solo loopback), una base `adel_pos_test_*`
desechable por paquete, `GOPROXY=off` y Go temporal para Playwright en puertos propios de cada parte (8184/1422 en la
fase A, 8185/1423 en D1, 8187/1425 en C, 8186/1424 en B, 8188/1426 en D2), según `CLAUDE.md` §6. Las fases A, B y C y la
parte D1 se desarrollaron en parte en paralelo; cada cierre de fase se ejecutó sobre una copia exacta del commit.

**Pruebas automatizadas, no físicas.** Vitest con jsdom y Playwright con Chromium y WebKit sustituyen el puente nativo
por la simulación de `e2e/native-driver.ts` (que ahora también simula `menu_subscribe` y `menu_set_state` y permite
«elegir» una opción del menú con `clickMenu`). **No** sustituyen la ventana nativa (WKWebView), el menú real de macOS ni
de Windows, una pantalla táctil física ni el teclado virtual sobre un dispositivo real. La impresión se comprobó, como en
la 9B, con el transporte controlado de la cola y ejecutores simulados: **bytes y argumentos, no papel**.

## Resultados ejecutados por fase

| Fase (commit)                         | Estáticas                                                              | Go unitarias (`-race`)                                                                     | Integración PostgreSQL (`-race`)                                                                                                                     | Vitest (`npm test`)                  | `tsc` y build           | Rust (`cargo test`)      | Playwright                                                                           |
| ------------------------------------- | ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------ | ----------------------- | ------------------------ | ------------------------------------------------------------------------------------ |
| 0 (`4a0a60f`), solo lectura           | —                                                                      | —                                                                                          | `administration` 54.1 s `ok`                                                                                                                         | 3 archivos, 7 pruebas (una temporal) | —                       | —                        | —                                                                                    |
| A, agente                             | `go vet` con y sin `integration`                                       | `administration` 8.4 s, `database` 1.6 s                                                   | `administration` 63.6 s, `database` 6.6 s                                                                                                            | 63 archivos, 575 pruebas             | Sin errores; build `ok` | No aplica (sin Rust)     | Chromium 13/13 (5.2 min) y WebKit 13/13 (5.1 min)                                    |
| A, cierre (`e575ad0`)                 | Prettier, gofmt, `go vet` con y sin `integration`                      | `administration` 8.7 s, `database` 1.3 s                                                   | `administration` 59.1 s, `database` 6.6 s                                                                                                            | 63 archivos, 567 pruebas             | Sin errores; build `ok` | —                        | Chromium 13/13 (5.2 min) y WebKit 13/13 (5.1 min)                                    |
| D1, agente (`e73b6b4`)                | `go vet` con y sin `integration`                                       | `ok`: reports, catalog, logoraster, kitchen, documents, designer, payments, administration | `reports` 106.2 s, `reports/operational` 43.0 s, `catalog` 8.4 s, `designer` 20.4 s, `database` 6.4 s, `administration` 64.8 s, `operations` 242.3 s | 63 archivos, 575 pruebas             | Sin errores; build `ok` | 34 aprobadas (una nueva) | Chromium: `workflow-stage9b-designer` aprobada                                       |
| C, agente                             | `cargo fmt --check`                                                    | No aplica (sin cambios de Go)                                                              | No aplica                                                                                                                                            | 65 archivos, 602 pruebas             | Sin errores; build `ok` | 40 aprobadas, 1 ignorada | Chromium 14/14 (5.3 min) y WebKit 14/14 (5.2 min)                                    |
| C, cierre (`1feaa59`, A + C + D1)     | Prettier, gofmt, `cargo fmt --check`, `go vet` con y sin `integration` | —                                                                                          | —                                                                                                                                                    | 65 archivos, 602 pruebas             | Sin errores; build `ok` | 40 y 1                   | WebKit 14/14 (5.2 min); Chromium 10/14 (ver abajo) y, tras corregir, 14/14 (5.8 min) |
| B, agente                             | —                                                                      | No aplica (sin cambios de Go ni Rust)                                                      | No aplica                                                                                                                                            | 66 archivos, 618 pruebas             | Sin errores; build `ok` | —                        | 15/15 por motor (las 14 del repositorio y una temporal de capturas)                  |
| B, cierre (`88d54f0`, A + B + C + D1) | Prettier, gofmt, `cargo fmt --check`, `go vet` con y sin `integration` | —                                                                                          | —                                                                                                                                                    | 66 archivos, 618 pruebas             | Sin errores; build `ok` | 40 y 1                   | Chromium 14/14 (5.4 min) y WebKit 14/14 (5.2 min)                                    |
| Corrección JPEG (`0dc0cfc`)           | —                                                                      | `ok`: logoraster, catalog, identity                                                        | `catalog` 11.4 s, `identity` 20.6 s                                                                                                                  | —                                    | —                       | —                        | —                                                                                    |
| D2, agente (`ab755eb`)                | `go vet`, `gofmt -l` vacío, Prettier de sus archivos                   | `ok`: administration, designer, identity, inventory, kitchen, operations, payments         | `administration` 64.8 s, `designer` 20.9 s, `identity` 18.1 s, `kitchen` 5.5 s, `payments` 3.6 s, `inventory` 1.8 s, `operations` 241.3 s            | 68 archivos, 636 pruebas             | Sin errores; build `ok` | —                        | Chromium 15/15 (5.7 min); **WebKit no ejecutado** (pantalla bloqueada, ver abajo)    |
| Correcciones de interfaz (`41e31b7`)  | —                                                                      | —                                                                                          | —                                                                                                                                                    | 68 archivos, 637 pruebas             | `tsc` sin errores       | —                        | —                                                                                    |
| E (`d268600`)                         | Solo versión 0.9.2 (`product.json` + `sync`)                           | Regresión final                                                                            | Regresión final                                                                                                                                      | Regresión final                      | Regresión final         | Regresión final          | Regresión final                                                                      |

«—»: la fase no registró esa suite (no cambiaba esa capa o la cubrió otra fase). Las pruebas de integración se
ejecutaron con `-tags=integration`; los tiempos son los de `go test`.

- **0**: verificación de solo lectura contra el código, con una prueba temporal de Vitest (no versionada) que comprobó que
  cambiar la apariencia en Mis preferencias aplica el tema de inmediato y se conserva, y la aceptación de
  `internal/administration` (persistencia del tema). Después, el coordinador comprobó que la 0020 se aplica sobre una
  instalación con datos (`internal/database` en el clúster temporal, `ok`).
- **A**: integración de `administration` con `TestStageTwoAcceptance/9C_lectura_mesas_por_omision`,
  `9C_edicion_con_permiso_y_auditoria`, `9C_lista_cerrada_en_go_y_en_la_base` y `9C_sin_devices_manage_no_se_edita`;
  `internal/database` con `TestStage9CTerminalHomeUpgrade` (la 0020 sobre un esquema 19 con terminales e impresoras:
  terminales en Mesas, impresoras en NULL, CHECK con nombre). Vitest con `navigation/navigation.test.tsx` y los
  `App.test.tsx` y `WindowControls.test.tsx` actualizados. Playwright: los 12 recorridos anteriores y sus ayudantes en la
  barra superior, más `workflow-stage9c-navigation.spec.ts`, con capturas claro y oscuro a 1280 × 720 y 1920 × 1080. El
  cierre se ejecutó sobre la copia exacta del commit, sin los cambios de D1 (que seguía en curso).
- **D1**: `TestStage9CCancellationReports` (cuentas anuladas; modificaciones de cantidades; referencia del comprobante en
  devoluciones y correcciones; permisos de origen por API directa; PDF y Excel iguales a la pantalla) y las exportaciones
  de `internal/reports`; en `internal/operations`, los cuatro subtests 9C de `TestRestaurantAcceptance` (columnas en Go y
  PostgreSQL, comanda con las columnas de la impresora y reimpresión exacta tras cambiarlas, documentos comerciales con
  las de la impresora de tickets, prueba de ancho por la cola sin ventas ni movimientos); unitarias de columnas
  (`documents/columns_test.go`, `kitchen/printer_columns_test.go`, incluida la densidad de CUPS) y de imágenes del
  catálogo (`catalog/images_test.go`: el JPEG progresivo de 1 MiB con unos 61 000 barridos se rechaza en menos de 0.5 s y
  en la aceptación con PostgreSQL en menos de 2 s). Las equivalencias de la 9B con el valor predeterminado siguen
  aprobadas: `TestDefaultKitchenTemplatesMatchLegacyRenderer`, `TestPlainLayoutKeepsTransportBytesAndArguments`,
  `TestPlainLayoutKeepsESCPosBytesOverTCP`, `TestDefaultCommercialTemplatesMatchLegacyRenderer`,
  `TestDefaultCashTemplatesMatchLegacyRenderer` y la cadena 9B de `internal/operations`. Medición opcional
  (`ADEL_REPORTS_MEASURE=1`): con un año sintético de 607 214 eventos, «Cuentas anuladas» 21 ms y «Modificaciones de
  cantidades» 141 ms (mejor de 3); un índice parcial candidato no los cambió (20 ms y 141 ms), así que no se agregó.
- **C**: Rust con las 6 pruebas nuevas de `menu::tests` (estructura por plataforma, Edición con los elementos
  predefinidos y sin ⌘W, lista cerrada completa y única, ⌘ en macOS y Ctrl en Windows, eventos por la lista cerrada y
  `menu_set_state` con validación); Vitest con `commands/commands.test.ts` (15) y `commands/commandCenter.test.tsx` (13);
  Playwright con `workflow-stage9c-keyboard.spec.ts`, y después la comprobación sin desbordamiento de la paleta y de
  «Atajos de teclado» a 1280 × 720 y 1920 × 1080 en ambos temas (Chromium 6.2 s y WebKit 5.8 s).
- **C, cierre**: en Chromium falló `workflow-stage9b-designer.spec.ts` al cerrar los avisos antes de sus capturas (un aviso
  de éxito se cerró solo durante el clic y el clic se reintentó hasta agotar el tiempo); las tres especificaciones
  siguientes (`workflow-stage9b-identity`, `workflow-stage9c-keyboard` y `workflow-stage9c-navigation`) fallaron en
  cascada porque Playwright reinicia el proceso y pierde la terminal preparada. Se corrigió el ayudante de la prueba (el
  clic tolera que el aviso desaparezca y el bucle tiene tope; `a3f6534`) y se repitió Chromium sobre la copia exacta:
  14/14 (5.8 min).
- **B**: `components/Select.test.tsx` (15 pruebas) y una prueba nueva de promociones con selección múltiple en
  `catalogFixes.test.tsx`; 17 archivos de Vitest y 11 de Playwright actualizados al selector (ayudantes únicos
  `src/testing/select.ts` y `e2e/select.ts`). Capturas del selector corto, con búsqueda (zona horaria) y múltiple
  (promociones) en claro y oscuro a 1280 × 720. Dos ejecuciones previas encontraron y permitieron corregir dos defectos
  propios (la opción con valor vacío se mostraba como «Selecciona…»; en WebKit, abrir con el ratón no dejaba el foco en el
  disparador).
- **D2**: Vitest con `components/TimeField.test.tsx` (10) y `pages/plainLanguage.test.tsx` (4), 5 pruebas nuevas en
  `pages/restaurant/tablePanel.test.tsx` y once archivos actualizados; Playwright Chromium con
  `workflow-stage9c-tables-time.spec.ts` nueva y capturas a 1280 × 720 y 1920 × 1080 en ambos temas del mapa con el panel
  de una mesa ocupada, del buscador de mesas, del selector de hora abierto, de Configuración general y de los filtros con
  fecha y hora del Registro de movimientos. **WebKit no se pudo ejecutar**: la pantalla del equipo quedó bloqueada desde
  las 21:52 y, con la sesión bloqueada, WebKit no puede hacer clic ni en «Entrar» del acceso («<html> intercepts pointer
  events»); una copia exacta de `HEAD` sin los cambios de D2 falla igual. Por eso `workflow-stage9c-tables-time.spec.ts`
  no se ha ejecutado en WebKit fuera de la regresión final.

## Revisión independiente

Un agente de solo lectura revisó las fases 0, A, B y C y la parte D1 (`0c7e488..88d54f0`) en una copia exacta, con
reproducciones propias. **Sin hallazgos críticos ni altos.** Revisado sin problemas: permisos y visibilidad de la barra,
mosaicos, ruta, menú, paleta y atajos (Go sigue validando; inicio por terminal con `devices.manage`, lista cerrada y
auditoría); ninguna acción financiera o irreversible sin su confirmación; sin doble ejecución; Escape compatible con el
cobro, los diálogos y el selector; operaciones nuevas del puente cerradas; Edición sin ⌘W; toda la navegación por
`requestScreen`; valores del selector idénticos a los anteriores; reportes nuevos, columnas por impresora, apariencia de
usuarios nuevos y 0020 sobre un esquema 19 con datos.

Salida del revisor (copia de `88d54f0`): `tsc --noEmit` sin errores; `npm test` 66 archivos, 618 pruebas; `go vet` con y
sin `integration` sin hallazgos; `go test -race` `ok` en reports, catalog, logoraster, kitchen, documents, designer,
payments, administration y database; integración `database` 6.3 s, `administration` 62.6 s, `reports` 107.2 s,
`catalog` 8.6 s, `designer` 20.5 s y `operations` 235.7 s `ok`; `cargo test` 40 aprobadas y 1 ignorada; Playwright
Chromium con las dos especificaciones 9C solas sobre una base nueva, 2/2 (26.4 s). WebKit quedó sin verificar en la
revisión: `workflow-stage9c-navigation` sola falló 3 veces al pulsar «Entrar» e `inventory.spec.ts` (anterior a la 9C)
sola falla igual (entorno, no la 9C).

| Hallazgo                                                                                                                                                      | Corrección                                                                                              | Prueba                                                                                                                   |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| Medio · el límite de barridos JPEG del catálogo se evitaba con un byte suelto después de SOI                                                                  | `logoraster.CheckJPEGMarkers` recorre todo archivo que empiece con `FF D8` (`0dc0cfc`)                  | `logoraster` y `catalog` con el archivo de la revisión (1 MiB, ≈ 61 000 barridos y un byte suelto), rechazado en < 0.5 s |
| Bajo · «Cuenta sin mesa», «Nuevo pedido para llevar» y «Nueva cuenta en mesa» habilitadas con el cobro abierto; elegir una mesa en la paleta cerraba el cobro | Deshabilitadas con el cobro abierto, con su explicación (`41e31b7`)                                     | `commands/commands.test.ts`                                                                                              |
| Bajo · «Información para soporte» visible antes de iniciar sesión                                                                                             | El diagnóstico del acceso ya no la muestra; dentro de la sesión sigue con `settings.manage` (`41e31b7`) | `App.test.tsx`                                                                                                           |

## Cobertura de la etapa 9C

Correspondencia con las pruebas de §6 de la especificación. Rutas relativas a `desktop/src/` o `backend/internal/` según
la capa.

| Prueba exigida (§6)                                                    | Dónde y qué                                                                                                                                                                                                                                                                                                                   |
| ---------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Barra por permisos (administrador, cajero, mesero)                     | `navigation/navigation.test.tsx` («el administrador ve todas las secciones…», «el cajero ve Mesas, Para llevar y Caja…», «el mesero ve únicamente Mesas y Para llevar», «una sola opción de administración basta…»).                                                                                                          |
| Indicador de caja en sus tres estados                                  | `navigation.test.tsx` («verde con el turno abierto, ámbar en arqueo y gris sin turno»; menú rápido con «Abrir turno» e «Ir a Caja»; sin el turno visible, el botón lleva a Caja).                                                                                                                                             |
| Estado del servidor                                                    | `navigation.test.tsx` (derivado de la sesión y del canal; punto con descripción en línea y aviso visible si no).                                                                                                                                                                                                              |
| Botón de pantalla completa                                             | `navigation.test.tsx` y `components/WindowControls.test.tsx` (un botón de icono que alterna entre maximizar y reducir con su descripción).                                                                                                                                                                                    |
| Ruta de navegación y mosaicos por permisos                             | `navigation.test.tsx` (ruta con cada pantalla anterior tocable y pestaña como último tramo; siete mosaicos para el administrador; Personal y Sistema como segunda pantalla; Cobros con permisos de administración de cobros).                                                                                                 |
| Inicio por terminal                                                    | `navigation.test.tsx` (`resolveHome`: Mesas por omisión, la primera sección permitida, nunca «Estado del sistema»); `administration` (`TestTerminalHomeIsAClosedList` y los subtests `9C_*`); `database` (`TestStage9CTerminalHomeUpgrade`).                                                                                  |
| Selector: búsqueda, teclado, múltiple, accesibilidad                   | `components/Select.test.tsx` (15) y las pantallas actualizadas; Playwright con `e2e/select.ts` en todas las especificaciones que elegían opciones.                                                                                                                                                                            |
| Atajos que no actúan al escribir ni ejecutan acciones financieras      | `commands/commandCenter.test.tsx` («Ctrl+1–5 navegan… y no actúan mientras se escribe», «las acciones de caja no tienen atajo…», diálogo abierto, Escape del cobro) y `commands/commands.test.ts` («cada atajo es único y las acciones financieras no tienen atajo», «ninguna acción de caja registra nada»).                 |
| Paleta filtrada por permisos                                           | `commands.test.ts` («se filtra por permisos y contexto», búsqueda sin acentos, mesas con el cobro abierto) y `commandCenter.test.tsx` («Ctrl+K la abre con teclado completo, filtrada por permisos y contexto»).                                                                                                              |
| Rust: menú nativo y sus eventos; ninguna operación nueva sin su prueba | `menu::tests` (6) para `menu_subscribe` y `menu_set_state`; `api.rs` (`kitchen_printer_width_test_is_a_closed_post_with_identifier`).                                                                                                                                                                                         |
| Go: reportes de §5.1 con permisos y exportaciones                      | `reports` (`TestStage9CCancellationReports` con permisos por API directa y PDF y Excel iguales a la pantalla); `EXPORTABLE_REPORTS` igual al registro (`TestExportBridgeListsEveryReport`).                                                                                                                                   |
| Go: validación de JPEG del catálogo                                    | `catalog/images_test.go` (barridos, segmentos y byte suelto después de SOI) y `logoraster`; aceptación de `catalog`.                                                                                                                                                                                                          |
| §5.3: columnas por impresora                                           | `documents/columns_test.go`, `kitchen/printer_columns_test.go`, `operations/printer_columns_9c_test.go` (lista cerrada en Go y PostgreSQL, equivalencia sin cambios con el predeterminado, 48 y 32 columnas, reimpresión exacta, prueba de ancho sin ventas ni movimientos) y `designer/printer_preview_integration_test.go`. |
| §4.4: apariencia de usuarios nuevos                                    | `administration` (`stage9c_new_user_appearance`); `pages/plainLanguage.test.tsx` (etiquetas).                                                                                                                                                                                                                                 |
| Playwright: recorridos anteriores en la nueva navegación               | Las 12 especificaciones anteriores pasan con la barra superior (fases A, B y C en ambos motores; D2 solo en Chromium).                                                                                                                                                                                                        |
| Playwright: barra, mosaicos y ruta, captura reducida                   | `e2e/workflow-stage9c-navigation.spec.ts`.                                                                                                                                                                                                                                                                                    |
| Playwright: recorrido solo con teclado (atajos y paleta)               | `e2e/workflow-stage9c-keyboard.spec.ts` (con el menú nativo simulado).                                                                                                                                                                                                                                                        |
| Playwright: 1280 × 720 y 1920 × 1080 en ambos temas sin desbordamiento | `workflow-stage9c-navigation`, `workflow-stage9c-keyboard` y `workflow-stage9c-tables-time` (esta última solo en Chromium hasta la regresión final).                                                                                                                                                                          |

## Pendiente (pruebas físicas y de entorno)

No se afirma ninguna de estas:

- **Ventana nativa Tauri (WKWebView) con el menú real de macOS**: el menú en español, si `preventDefault` en el WebView
  evita el acelerador nativo (el despachador evita la doble ejecución en ambos casos, pero solo se probó con la
  simulación), los elementos que macOS agrega por su cuenta (Dictado y Emoji en Edición; posiblemente «Entrar en pantalla
  completa» en Ver) y el menú en pantalla completa.
- **Menú en Windows**: no se compiló ni se probó (solo su estructura en las pruebas Rust); en Windows los aceleradores
  pueden no llegar al menú con el foco en el WebView y entonces los atiende la capa de teclado de React.
- **Pantalla táctil física**: barra superior, mosaicos, selector, selector de hora y mapa de mesas.
- **Teclado virtual sobre el selector** (búsqueda), sobre el selector de hora y sobre la paleta: solo con eventos
  simulados en Vitest.
- **Papel a 48 y 32 columnas** y la **densidad fraccionaria de CUPS** en texto (`cpi` 13.71 y 12.8; 6.86 y 6.4 con el
  texto grande): la prueba de ancho existe para comprobarlo en cada impresora antes de elegir.
- Lector de pantalla real (VoiceOver), dos terminales reales, LAN con otra máquina y Mac Intel.

## Qué no se afirma

- Que el menú nativo se vea y se comporte en la ventana real como en la simulación de Playwright.
- Que una pulsación con acelerador llegue una sola vez en la ventana nativa: el despachador lo garantiza para las dos
  vías, pero la llegada real no se observó.
- Que una impresora concreta imprima completas 48 o 32 columnas, ni que respete la densidad fraccionaria de CUPS.
- Que el selector o la paleta funcionen bien con el teclado virtual en una pantalla táctil real.

## Regresión final

Ejecutada el 11 de noviembre de 2026 por un solo agente (sin subagentes) sobre una copia exacta de `d268600` (`git
archive`, versión 0.9.2, esquema 20), con el clúster PostgreSQL temporal 55499 y Go temporal en 8183; nada contra la
instalación real. Una sola pasada, sin fallos ni repeticiones (1338 s):

| Suite                    | Resultado                                                                                                                                                                                             |
| ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato                  | Prettier `--check`, gofmt y `cargo fmt --check` limpios                                                                                                                                               |
| Metadatos                | `sync-metadata.mjs --check`: «Metadatos coherentes» (0.9.2, esquema 20)                                                                                                                               |
| `go vet`                 | `./...` y `-tags=integration ./...` sin hallazgos                                                                                                                                                     |
| Go unitarias (`-race`)   | 22 paquetes `ok`                                                                                                                                                                                      |
| Integración PostgreSQL   | `database` 6.3 s, `administration` 62.2 s, `catalog` 8.7 s, `operations` 235.2 s, `payments` 2.2 s, `reports` 104.7 s, `reports/operational` 40.2 s, `designer` 20.4 s, `identity` 17.8 s: todas `ok` |
| TypeScript estricto      | `tsc --noEmit` sin errores                                                                                                                                                                            |
| Vitest (`npm test`)      | 68 archivos, 637 pruebas aprobadas                                                                                                                                                                    |
| Compilación del frontend | `npm run build` correcto                                                                                                                                                                              |
| Rust/Tauri               | `cargo test --locked --offline`: 40 aprobadas, 1 ignorada (Keychain real)                                                                                                                             |
| Playwright               | Chromium 15/15 (5.7 min) y WebKit 15/15 (5.6 min), incluidas las cuatro especificaciones de la 9C y todas las anteriores                                                                              |
| Paquete de escritorio    | `.app` 0.9.2 compilado en la copia aislada (10.00 MiB, Mach-O arm64)                                                                                                                                  |

La pantalla del equipo estaba bloqueada durante toda la corrida y aun así WebKit aprobó 15/15 (sin el síntoma «<html>
intercepts pointer events» que impidió ejecutar WebKit en la parte D2). Se saltan, como siempre, las mediciones opcionales
(`ADEL_INVENTORY_MEASURE`, `ADEL_REPORTS_MEASURE`) y la impresión física de cocina.

## Instalación real

La 0020 se aplicó a la instalación real el 11 de noviembre de 2026 (22:59–23:02, hora de Monterrey) con la
**autorización previa del propietario** («Cuando termines si todo está bien aplica los cambios»), después de aprobar la
regresión final: comprobación de solo lectura antes (ningún turno en arqueo ni trabajo de impresión pendiente), respaldo
`backups/adel_pos-20261112T045957Z-a3a2b3.dump` con restauración verificada (132 tablas idénticas), servidor detenido de
forma ordenada, `migrate` `applied:1` y repetición `applied:0`, servidor 0.9.2 con readiness 200 (PostgreSQL disponible,
esquema compatible) y `/api/v1/version` 0.9.2/20, comprobación de solo lectura después (solo cambian las migraciones y
las dos columnas nuevas; todos los conteos operativos, permisos, auditoría y ventas idénticos; la terminal con inicio en
Mesas; restricciones `devices_home_screen_check` y `kitchen_printers_columns_check` presentes) y `.app` 0.9.2 compilado y
abierto. Detalle en [ETAPA_9C_AVANCE.md](ETAPA_9C_AVANCE.md), «Instalación real». El recorrido en la ventana nativa
(barra, menú nativo real, atajos, selector, mapa y selector de hora) queda como verificación del propietario.
