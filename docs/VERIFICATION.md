# Verificación · Etapa 9C

Estado al **11 de noviembre de 2026**, zona America/Monterrey. Versión **0.9.2**, API v1, esquema **20** (migración
`0020_terminal_home_and_printer_columns.sql`, etapa 9C: nueva interfaz principal, navegación, teclado y ajustes).
Evidencia detallada por fase: [VERIFICATION_STAGE9C.md](VERIFICATION_STAGE9C.md). Evidencia anterior:
[etapa 9B](VERIFICATION_STAGE9B.md), [etapa 9A](VERIFICATION_STAGE9A.md), [etapa 8](VERIFICATION_STAGE8.md),
[etapa 7.1](VERIFICATION_STAGE7_1.md), [etapa 7](VERIFICATION_STAGE7.md), [etapa 6.2](VERIFICATION_STAGE6_2.md),
[etapa 6.1](VERIFICATION_STAGE6_1.md), [etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md),
[etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y
[etapa 1](VERIFICATION_STAGE1.md). Avance: [ETAPA_9C_AVANCE.md](ETAPA_9C_AVANCE.md). Guía:
[NAVIGATION_AND_SHORTCUTS.md](NAVIGATION_AND_SHORTCUTS.md).

## Estado

- **Código**: fases 0 (`4a0a60f`), A (`e575ad0`), B (`88d54f0`), C (`1feaa59`) y D (partes D1 `e73b6b4` y D2 `ab755eb`)
  en commits locales de la rama `etapa-9c`, cada una con sus suites aprobadas en un entorno aislado (clúster PostgreSQL
  temporal en el puerto 55499 y Go temporal para Playwright), según [VERIFICATION_STAGE9C.md](VERIFICATION_STAGE9C.md);
  una **revisión independiente** sin hallazgos críticos ni altos y sus correcciones (`0dc0cfc` y `41e31b7`); fase E con la
  versión 0.9.2 y el esquema 20 (`d268600`) y esta documentación.
- **Excepción conocida**: la parte D2 no pudo ejecutar Playwright WebKit (pantalla del equipo bloqueada; también falla así
  una copia sin sus cambios). `workflow-stage9c-tables-time.spec.ts` solo se ejecutó en Chromium hasta la regresión final.
- **Regresión final (fase E)**: ver «Regresión final».
- **Instalación real**: **0.9.2 y esquema 20**. La 0020 se aplicó el 11 de noviembre de 2026 con la autorización previa del propietario: respaldo `backups/adel_pos-20261112T045957Z-a3a2b3.dump` con restauración verificada (132 tablas idénticas), `migrate` `applied:1` y repetición `applied:0`, readiness 200 con esquema compatible, `/api/v1/version` 0.9.2/20, comprobaciones de solo lectura antes y después, y servidor y `.app` 0.9.2 instalados (registro en [ETAPA_9C_AVANCE.md](ETAPA_9C_AVANCE.md), «Instalación real»).
- **Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432) ni enviaron trabajos
  a CUPS real, a una impresora, a un cajón o a una conexión TCP fuera de loopback.

## Qué está verificado

Todo con Go, PostgreSQL temporal, Vitest, Rust y Playwright (Chromium y WebKit) en entorno aislado, con el puente nativo
simulado (`e2e/native-driver.ts`, que simula también el menú) y, en impresión, solo con adaptadores simulados.

| Área                                         | Evidencia                                                                                                                                                                                                                                                     |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Barra superior y captura reducida            | `navigation/navigation.test.tsx` (barra para administrador, cajero y mesero; Caja en sus tres estados con el menú rápido de la 7.1; estado del servidor; pantalla completa) y Playwright `workflow-stage9c-navigation`.                                       |
| Administración por mosaicos y ruta           | `navigation.test.tsx` (mosaicos por permisos, Personal y Sistema como segunda pantalla, ruta tocable con la pestaña como último tramo) y Playwright de navegación.                                                                                            |
| Inicio por terminal (0020)                   | `resolveHome` en Vitest; aceptación de `internal/administration` (lectura, edición con `devices.manage` y auditoría, lista cerrada en Go y en la base); `TestStage9CTerminalHomeUpgrade` (la 0020 sobre un esquema 19 con datos).                             |
| Todas las pantallas en la navegación nueva   | Las 12 especificaciones Playwright anteriores pasan con la barra superior en ambos motores (cierres de las fases A, B y C).                                                                                                                                   |
| Selector único                               | `components/Select.test.tsx` (15 pruebas) y las pantallas actualizadas; ningún `<select>` en la interfaz; Playwright con el ayudante `e2e/select.ts`.                                                                                                         |
| Teclado, menú nativo y paleta                | `commands/commands.test.ts` y `commandCenter.test.tsx` (atajos que no actúan al escribir, sin atajos financieros, Escape compatible con el cobro, sin doble ejecución, paleta filtrada); Rust `menu::tests` (6); Playwright `workflow-stage9c-keyboard`.      |
| Mesas, selector de hora, textos y apariencia | `TimeField.test.tsx` (10), `plainLanguage.test.tsx` (4), `tablePanel.test.tsx`; aceptación de la apariencia de usuarios nuevos; Playwright `workflow-stage9c-tables-time` (Chromium).                                                                         |
| Reportes nuevos (§5.1)                       | `TestStage9CCancellationReports` (cuentas anuladas, modificaciones de cantidades, referencias en devoluciones y correcciones, permisos por API directa, PDF y Excel); 94 reportes en `EXPORTABLE_REPORTS`.                                                    |
| Imágenes del catálogo (§5.2)                 | `catalog/images_test.go` y `logoraster` (barridos, segmentos y byte suelto después de SOI; 1 MiB con ≈ 61 000 barridos rechazado en < 0.5 s).                                                                                                                 |
| Columnas por impresora (§5.3)                | Equivalencia de la 9B intacta con el predeterminado; salida a 48 y 32 columnas; valor fuera de la lista rechazado en Go y en PostgreSQL; reimpresión exacta tras cambiar las columnas; prueba de ancho sin ventas ni movimientos; vista previa por impresora. |
| Revisión independiente                       | Revisión de solo lectura de las fases 0–C y D1 con reproducciones propias; sus tres correcciones tienen prueba.                                                                                                                                               |

## No ejecutado o no verificado

- **Ventana nativa Tauri (WKWebView) con el menú real de macOS** (incluida la llegada doble de un atajo con acelerador,
  que el despachador cubre pero no se observó) y **el menú en Windows** (no se compiló).
- **Pantalla táctil física** y **teclado virtual** sobre el selector, el selector de hora y la paleta.
- **Papel a 48 y 32 columnas** y la densidad fraccionaria de CUPS en texto: prueba física pendiente con la prueba de ancho.
- Lector de pantalla real, dos terminales reales, LAN con otra máquina, Keychain real, Mac Intel y Windows.
- Pendientes de etapas anteriores que siguen sin cambio: papel con formato y logotipo (9B), cajón físico, diálogo nativo
  real de guardado (9A).

## Regresión final

Aprobada el 11 de noviembre de 2026 sobre una copia exacta de `d268600` (0.9.2, esquema 20), en una sola pasada y por un solo agente: formato, metadatos, `go vet`, 22 paquetes Go con `-race`, integración PostgreSQL de los nueve paquetes, `tsc --noEmit`, Vitest (68 archivos, 637 pruebas), `npm run build`, `cargo test` (40 aprobadas, 1 ignorada), Playwright Chromium 15/15 y WebKit 15/15 y el `.app` 0.9.2 compilado en la copia aislada. Detalle en [VERIFICATION_STAGE9C.md](VERIFICATION_STAGE9C.md).

## Repetir las comprobaciones

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
```

`test-integration` y `test-browser` usan bases desechables en el clúster del proyecto (55432); para aislamiento total,
seguir `CLAUDE.md` §6. Las pruebas de integración de la 9C viven en `internal/administration` (inicio por terminal y
apariencia), `internal/database` (actualización a la 0020), `internal/reports` (reportes nuevos), `internal/catalog`,
`internal/designer` (vista previa por impresora) e `internal/operations` (columnas por impresora, encadenadas a
`TestRestaurantAcceptance`). Las especificaciones Playwright comparten la base de cada motor y corren en orden
alfabético: las de la 9C se llaman `workflow-stage9c-*.spec.ts` para correr después de las existentes; no conviene
filtrarlas con `documents-` (la ruta temporal contiene «Documents-ADEL-POS»). Con la pantalla del equipo bloqueada,
Playwright WebKit no puede hacer clic («<html> intercepts pointer events»): repetir WebKit con la sesión desbloqueada.
