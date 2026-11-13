# Verificación · Etapa 9E (Windows 11)

Estado al **12 de noviembre de 2026**. Versión **0.9.4**, API v1, esquema **21** (sin migraciones nuevas). Rama
`etapa-9e-windows`, creada desde `main` (`6763379`, etapa 9D). Especificación:
[etapas/ETAPA_9E_WINDOWS.md](etapas/ETAPA_9E_WINDOWS.md). Avance con el detalle de cada fase:
[ETAPA_9E_AVANCE.md](ETAPA_9E_AVANCE.md). Este documento registra **solo lo que se ejecutó**.

## Equipo y entorno

| Dato         | Valor                                                                                                                  |
| ------------ | ---------------------------------------------------------------------------------------------------------------------- |
| Windows      | Windows 11 Pro 25H2, 10.0.26200 (x64), Intel Core i7-1355U, 31.7 GB de memoria                                         |
| Herramientas | Go 1.27.3, Node 24.22.2 / npm 11.19.1, Rust 1.99.1 (MSVC), Visual Studio Build Tools 2022 17.14, PostgreSQL 18.6 (EDB) |
| Ubicación    | dentro del proyecto (`.tools\`), salvo Build Tools (por equipo)                                                        |
| Sin hardware | sin pantalla táctil, sin impresoras de tickets ni cajón (la única impresora es «Microsoft Print to PDF»)               |
| Sin `-race`  | no hay compilador de C para cgo; las suites de Go corren sin el detector de carreras (en la Mac siguen con él)         |

Las suites de integración y de navegador usaron bases desechables `adel_pos_test_*` en el clúster de desarrollo del
proyecto (`.local\postgres`, 127.0.0.1:55432), con la aplicación instalada cerrada (comparten puertos). Las pruebas de
la aplicación instalada usaron su carpeta de datos real (`%LOCALAPPDATA%\AD-EL POS`) en este equipo de pruebas.

## Resultados por fase

| Fase | Qué se probó                                                                                                            | Resultado                                                                                                                                      |
| ---- | ----------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| A    | `go build ./...` y `GOOS=darwin GOARCH=arm64 go build ./...` tras `.gitattributes`; `git ls-files --eol`                | aprobado; 1 008 archivos `i/lf w/lf` y 5 binarios, sin cambios espurios                                                                        |
| B    | `go test ./...`, integración (9 paquetes), Vitest, `npm run build`, `cargo test`, Playwright Chromium                   | aprobado tras corregir 1 prueba dependiente de macOS (bits 0600), 1 prueba frágil por tiempos (Vitest) y el `icon.ico` faltante                |
| C1   | `internal/standalone` (8 pruebas, ciclo completo con PostgreSQL real), `cargo test` (46), servidor local con el `.exe`  | aprobado; primer arranque 26 s, cierre ordenado sin procesos, recuperación de cierres anómalos, respaldo y restauración (134 tablas idénticas) |
| C2   | `go test ./...`, integración de `designer`, `payments` y `operations`, Vitest (756), `cargo check` para macOS           | aprobado; `cargo check` hacia `aarch64-apple-darwin` sin errores ni avisos                                                                     |
| D    | `scripts\build-windows.ps1`                                                                                             | instalador NSIS generado                                                                                                                       |
| E    | aplicación **instalada**: instalación, prueba de humo de 8 pasos, persistencia, respaldo, actualización, desinstalación | aprobado (ver «Aplicación instalada»); defectos de Windows encontrados y corregidos con pruebas                                                |

## Aplicación instalada (fase E)

| Prueba                                              | Resultado                                                                                                                                                                                                                                                                           |
| --------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Instalación limpia en silencio                      | 5.8 s; ventana en 2.4 s; servidor listo en 14 s (crea el clúster y aplica 21 migraciones); **0 ventanas de consola**; entrada en «Aplicaciones instaladas»; 3 accesos en el menú Inicio                                                                                             |
| Prueba de humo (Playwright por CDP)                 | asistente («Fonda Doña Ñeli»), catálogo, cocina, impresora, mesas, captura, envío a cocina, turno, cobros (efectivo, dólares, tarjeta, descuento), para llevar, cancelación con motivo, corte parcial, cierre de turno                                                              |
| Reporte «Ventas por día» en PDF y Excel             | exportado con el diálogo nativo; PDF abierto en Edge y Excel revisado: acentos, «ñ» y logotipo correctos; venta neta $285.00 (la cuenta cancelada no cuenta)                                                                                                                        |
| Persistencia                                        | cerrar y abrir: la sesión vuelve del Administrador de credenciales y los datos siguen                                                                                                                                                                                               |
| Respaldo y restauración                             | respaldo con la aplicación abierta; ensayo con 134 tablas idénticas; negativa a restaurar con la aplicación abierta; restauración real con respaldo de seguridad previo                                                                                                             |
| Actualización                                       | instalador de prueba 0.9.5 encima de 0.9.4 con la aplicación abierta: se cierra, espera a Go y PostgreSQL, conserva datos, una sola entrada; regreso a 0.9.4                                                                                                                        |
| Desinstalación y reinstalación                      | quita programa, accesos y entrada; conserva base, configuración, respaldos y credencial; al reinstalar abre con los mismos datos                                                                                                                                                    |
| Usuario con acentos (simulado)                      | programa en «Programa José Ñandú» y `LOCALAPPDATA` en «Usuario José Ñandú»: servidor listo en 15.7 s, asistente e inicio de sesión, cierre en 4.7 s, desinstalación con datos conservados                                                                                           |
| Instalador de la fase E (`verify-installed.ps1`)    | 7.1 s; ventana en 2.39 s y servidor listo en 3.06 s con datos; 0 consolas; ≈ 790 MB de conjunto de trabajo (≈ 403 MB privados); cierre en 0.61 s sin procesos restantes                                                                                                             |
| Instalador definitivo (fase F, encima del anterior) | 5.6 s; ventana en 2.39 s y servidor listo en 2.45 s con datos; 0 consolas; ≈ 745 MB de conjunto de trabajo (≈ 388 MB privados: WebView2 180, servidor 123, PostgreSQL 78, aplicación 7); cierre en 1.1 s sin procesos ni `postmaster.pid`; abre con la sesión y los datos de prueba |

Instalador definitivo (con las correcciones de la fase F): `desktop\src-tauri\target\release\bundle\nsis\AD-EL POS_0.9.4_x64-setup.exe`,
36.7 MB (38 472 613 bytes), SHA-256 `365c6a91c265284c8ac30f3dc752aaf6c2118d99d263b8a5c40b21ddf4eeda84`, sin firma. Queda
instalado (y cerrado) en este equipo con los datos de prueba. El de la fase E (`e04c87b8…`) quedó sustituido.

## Regresión completa (fase F)

Con la versión 0.9.4: primero sobre el código de la fase E (`32b6ee5`) y, tras las correcciones de abajo, otra vez
`dev.ps1 test` y Playwright completo con los dos motores sobre el código final.

| Suite                                                                                       | Resultado                                                                                                                                                                                                                                  |
| ------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `dev.ps1 test`                                                                              | aprobada antes y después de las correcciones (1–2 min): Prettier, `sync --check`, `go test ./...`, `go vet`, Vitest **757/757** (76 archivos), `npm run build` (TypeScript + Vite) y `cargo test --locked` **46** (1 ignorada a propósito) |
| `go test -count=1 ./...` (sin caché)                                                        | **25 paquetes aprobados** (12 s)                                                                                                                                                                                                           |
| `dev.ps1 test-integration` (PostgreSQL 18.6 real)                                           | **9 paquetes aprobados** (`database`, `administration`, `catalog`, `operations`, `payments`, `reports`, `reports/operational`, `designer`, `identity`); solo se omiten las opcionales de hardware y de volumen                             |
| `internal/standalone` y `cmd/ad-el-pos-server` con `ADEL_STANDALONE_PG_BIN`                 | aprobadas: ciclo completo con carpeta con acentos (15.5 s) y respaldo antes de migrar sobre datos existentes (24 s)                                                                                                                        |
| `cargo test --locked -- --ignored` (Administrador de credenciales real)                     | aprobada: escribe, lee y borra una credencial temporal; no quedó ninguna                                                                                                                                                                   |
| `dev.ps1 test-browser` · Chromium                                                           | **19 de 19 aprobadas** en las tres corridas completas (8.3, 7.5 y 7.5 min; la última sobre el código final)                                                                                                                                |
| `dev.ps1 test-browser` · WebKit                                                             | **19 de 19 aprobadas** en la corrida final (8.0 min); antes, corridas con fallos que llevaron a las correcciones de abajo                                                                                                                  |
| `scripts\build-windows.ps1`                                                                 | aprobado dos veces; el definitivo, sobre el código final (arriba)                                                                                                                                                                          |
| macOS desde Windows: `GOOS=darwin GOARCH=arm64 go build ./...` y `go vet ./...`             | aprobados                                                                                                                                                                                                                                  |
| macOS desde Windows: `cargo check --locked` y `--tests` con `--target aarch64-apple-darwin` | aprobados, **0 errores y 0 avisos** (compilador sustituto para el archivo Objective-C de `objc2-exception-helper`; `cargo check` no enlaza)                                                                                                |

**Condiciones por plataforma** (revisión de los 77 archivos de código que cambiaron respecto a `main`): todo
`internal/standalone` y los comandos `host`, `backup`, `restore-check` y `restore` llevan `//go:build windows`
(`host_other.go` fuera de Windows); la parada (`stop_*.go`), la consola (`terminal_*.go`), las ACL (`fileacl_*.go`) y la
cola de impresión (`system_queue_unix.go`/`system_queue_windows.go`) tienen su variante por sistema, y el código
compartido de cocina solo entra a Windows por `windowsSpooler()`, que fuera de Windows devuelve CUPS. En Rust,
`server_host.rs` devuelve `None` fuera de Windows y `window_state`, el menú omitido y los textos del almacén van con
`#[cfg(target_os = …)]`. En la interfaz, los textos de impresión dependen de `isWindows()` y en macOS son idénticos
(`printersWindows.test.tsx`). Cambios compartidos a propósito (corrigen defectos en ambos sistemas): el cierre de SSE
al apagar (`administration/events.go`, `service.go`), el mapa de mesas (`TableMap.tsx`), el icono (`icon.icns` en
`tauri.conf.json`), `database.MigrationStatus` (solo lectura) y las correcciones de foco y de atajos de la regresión
(abajo).

**Lo que encontró la regresión** (Playwright WebKit en Windows corrió por primera vez completo; detalle en el avance,
«Fase F»), todo corregido y comprobado con la corrida final:

- **Carreras reales de la interfaz, en cualquier sistema** (cambios compartidos, sin efecto visible): la pantalla
  operativa publicaba su contexto y sus funciones para los atajos con efectos posteriores al pintado, y seis diálogos
  (hoja de Caja, `ConfirmDialog`, `ChoiceDialog`, `ModalPanel`, `QuickCategoryDialog` y la paleta) llevaban el foco
  igual; un atajo o Escape pulsado justo al aparecer algo usaba el estado anterior. Ahora con `useLayoutEffect`, como
  la 9D ya hacía en «Acerca de». `TableMap` gastaba su único intento de enfocar la mesa elegida del buscador en un
  render que aún mostraba el área anterior; ahora espera a la del área de la mesa.
- **Pruebas**: la de teclado seleccionaba texto con el modificador de los atajos (WebKit de Playwright en Windows se
  anuncia como Mac pero edita con Ctrl; ahora `ControlOrMeta+a`); la de indicaciones rápidas creaba catálogo después
  de abrir la aplicación (los cambios de catálogo no publican eventos en tiempo real; ahora antes); `dev.ps1
test-browser <filtro>` pasaba el filtro como nombre de proyecto (ahora antes de `--project`).
- **Intermitente sin reproducir**: una corrida falló en `restaurant.spec` (un clic en «Arqueo y cierre» que no abrió su
  panel); pasó en todas las demás.

## No verificado

- **La 0.9.4 en la Mac**: la próxima sesión en la Mac debe ejecutar allí la regresión completa
  (`./scripts/dev.sh test`, `test-integration` y `test-browser`, con `-race`) y `build-macos` antes de instalarla; la
  comprobación desde Windows solo cubre la compilación.
- Windows 10; pantalla táctil; impresoras de red y USB con papel, logotipo y corte; cajón por red o USB; dos
  terminales (LAN no preparada en Windows); una cuenta de Windows con acentos real (el perfil de WebView2 siguió en la
  carpeta real del usuario); el aviso del firewall (no apareció: todo es `127.0.0.1`).
- `provision-sa` y `reset-sa` en Windows (solo del propietario; compilan).

## Repetir las comprobaciones

```powershell
powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 db-start
powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 test
powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 test-integration
powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 test-browser
powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1
powershell -ExecutionPolicy Bypass -File scripts\windows\verify-installed.ps1 -Installer "desktop\src-tauri\target\release\bundle\nsis\AD-EL POS_0.9.4_x64-setup.exe"
```

La aplicación instalada debe estar cerrada antes de `db-start` y de las suites (mismos puertos). Comprobación de macOS
desde Windows: [../DEVELOPMENT_WINDOWS.md](../DEVELOPMENT_WINDOWS.md), «Comprobar macOS desde Windows».
