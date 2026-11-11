# Verificación · Etapa 9A

Estado al **10 de noviembre de 2026**, zona America/Monterrey. Versión **0.9.0**, API v1, esquema **17** (migraciones
`0015_catalog_price_and_precheck.sql`, `0016_inventory_suppliers_counter.sql` y `0017_reports.sql`, etapa 9A: ajustes
pendientes de catálogo e inventarios, reportes, indicadores y exportaciones). Evidencia detallada por parte:
[VERIFICATION_STAGE9A.md](VERIFICATION_STAGE9A.md). Evidencia anterior: [etapa 8](VERIFICATION_STAGE8.md),
[etapa 7.1](VERIFICATION_STAGE7_1.md), [etapa 7](VERIFICATION_STAGE7.md), [etapa 6.2](VERIFICATION_STAGE6_2.md),
[etapa 6.1](VERIFICATION_STAGE6_1.md), [etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md),
[etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y
[etapa 1](VERIFICATION_STAGE1.md). Avance: [ETAPA_9A_AVANCE.md](ETAPA_9A_AVANCE.md). Guía: [REPORTS.md](REPORTS.md).

## Estado

- **Código**: fases 0, A (partes A1 y A2), B (partes B1 y B2) y C (partes C1 y C2) en commits locales de la rama
  `etapa-9a` (hasta `793c099`), cada parte con sus suites aprobadas en un entorno aislado (clúster PostgreSQL temporal en
  el puerto 55499 y Go temporal para Playwright), según [VERIFICATION_STAGE9A.md](VERIFICATION_STAGE9A.md). La versión
  0.9.0 y el esquema 17 se fijaron en la fase D, con `sync-metadata --check`, `go vet`, `go test`, `tsc` y `cargo test`
  aprobados sobre el árbol con la versión nueva.
- **Regresión final (fase D)**: **aprobada** sobre `74e23c9` (ver «Regresión final»).
- **Instalación real**: sigue en **0.8.0 y esquema 14**. Las migraciones 0015–0017 **no se han aplicado**: requieren
  autorización expresa del propietario, con respaldo y ensayo de restauración.
- **Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432) ni enviaron trabajos
  a CUPS, a una impresora, a un cajón o a una conexión TCP.

## Qué está verificado

| Área                                                       | Evidencia                                                                                                                                                                                                                                               |
| ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Precio de complementos configurable (§3.1)                 | Unitarias de `catalog/price_decrease_test.go` (JSON idéntico al anterior sin la opción, límites, promociones, combos) e integración de cotización, confirmación, IVA, promoción y descuento manual; Vitest y Playwright `table-price-decrease`.         |
| Vigencia de la precuenta y «Poner fuera de servicio»       | Integración con dos carreras controladas y control negativo con el predicado anterior; Playwright en una línea a 1280 × 720 y 1920 × 1080 en ambos temas.                                                                                               |
| Proveedores, compras de mostrador, código y categorías     | Unitarias e integración de inventario, actualización desde el esquema 14, Vitest y Playwright `workflow-inventory-9a`.                                                                                                                                  |
| Día comercial y periodos                                   | Unitarias de `businessday` (01:30 con 00:00 y 05:00, cambios de horario, semana ISO 53 y cambio de año) e integración cambiando la hora por la API sin reescribir registros.                                                                            |
| Reconocimiento de ventas y relleno de la 0017              | Unitarias del reconocimiento contra el desglose de la cuenta; aceptación con cierre, reliquidación, devolución en su día y cuenta reabierta; relleno igual a Go y actualización desde el esquema 14.                                                    |
| Reportes de ventas, productos, pagos y descuentos          | `TestReportsAcceptance` (15 subpruebas: totales del detalle iguales a los resumidos, paginación, combos, divisiones, pagos parciales y mixtos, propinas, dólares, motivos históricos, permisos por API directa).                                        |
| Reportes de caja, inventarios, costos, compras y empleados | `TestOperationalReports` (14 subpruebas: paridad con Caja, arqueo excluido, esperado ausente sin permiso, márgenes «Sin costo», «Estimado» y firme, compras sin duplicar el gasto, empleados sin doble conteo; en todos los reportes, detalle = total). |
| Solo lectura, semáforo y concurrencia                      | `TestReportRunnerIsReadOnlyAndBounded` (escritura rechazada con 25006, sin locks consultivos, `REPORTS_BUSY`, `REPORT_TIMEOUT`) y cobro sin esperar a un reporte; medición de volumen opcional.                                                         |
| Exportación PDF y Excel                                    | Unitarias de `export_test.go` e integración `TestReportExports` (igual a la pantalla, 25 reportes sin columnas protegidas en ambos formatos, fórmulas neutralizadas, auditoría, 50 000 renglones); Rust de `export.rs`.                                 |
| Interfaz de Reportes                                       | Vitest (`reports.test.tsx`, 27 pruebas, y `reportExport.test.ts`) y Playwright `workflow-reports-9a` y `workflow-reports-export-9a` sin desbordamiento a 1280 × 720 y 1920 × 1080 en ambos temas.                                                       |
| Puente nativo                                              | Pruebas Rust de la lista cerrada `reports_*` y de `export_report` (29 aprobadas, 1 ignorada).                                                                                                                                                           |
| Etapas anteriores                                          | En cada parte, la aceptación encadenada de `internal/operations` (etapas 4–8) y, en A1, B1 y C2, Playwright completo (8, 8 y 10 especificaciones por motor).                                                                                            |

## No ejecutado o no verificado

- Ventana nativa Tauri (WKWebView), diálogo nativo real de guardado y escritura en disco desde la ventana, pantalla
  táctil física y teclado virtual en Reportes, en la captura y en Inventarios.
- Papel: ninguna precuenta ni comanda de la 9A se imprimió.
- Volumen real, muchas terminales a la vez, LAN con otra máquina, Keychain real y Windows.
- Respaldo, ensayo de restauración y migración de la instalación real con 0015–0017.

## Regresión final (fase D)

**Aprobada** el 10 de noviembre de 2026 sobre una copia exacta del commit `74e23c9` (0.9.0, esquema 17, con las
correcciones de la revisión independiente), con el clúster temporal 55499 y Go temporal en un puerto propio:

- Prettier, gofmt, `cargo fmt`, `sync-metadata --check`, `go vet` y `go vet -tags=integration`: aprobados.
- Go unitarias con `-race`: todos los paquetes `ok`.
- Integración PostgreSQL con `-race`: `database` (5.2 s), `administration` (54.5 s), `catalog` (7.0 s), `operations`
  (216.3 s, aceptación encadenada de las etapas 4–9A), `payments` (1.5 s), `kitchen` (1.4 s), `inventory` (1.5 s),
  `reports` (88.4 s) y `reports/operational` (38.9 s): `ok`.
- Vitest: 54 archivos y 473 pruebas; `tsc --noEmit` y build sin errores.
- `cargo test --locked --offline`: 31 aprobadas y 1 ignorada (la de Keychain real).
- Playwright en Chromium y en WebKit: 10 de 10 en cada motor (4.4 y 4.3 min), sin reintentos.
- `tauri build --bundles app` en la copia aislada: `AD-EL POS.app` 0.9.0 (9.9 MiB), sin instalarlo ni abrirlo.
- Las comprobaciones de solo lectura previstas para la instalación real se ejecutaron contra una base temporal migrada
  hasta el esquema 17 (repetir la migración dio `applied:0`).

## Repetir las comprobaciones

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
```

`test-integration` y `test-browser` usan bases desechables en el clúster del proyecto (55432); para aislamiento total,
seguir `CLAUDE.md` §6. Las pruebas de reportes viven en `internal/reports`, `internal/reports/operational` y
`internal/database`, que están en `scripts/database.py`; las de los ajustes de la fase A, en `internal/operations`
(encadenadas a `TestRestaurantAcceptance`), `internal/catalog` e `internal/database`. Las mediciones de volumen son
opcionales (`ADEL_REPORTS_MEASURE=1` y, para el inventario, `ADEL_INVENTORY_MEASURE=1`). Las especificaciones del
propietario `docs/etapas/ETAPA_7.md`, `ETAPA_7_1.md`, `ETAPA_8.md` y `ETAPA_9A.md` no tienen formato Prettier
(`prettier --check`, 10 de noviembre de 2026) y no se formatean: mientras sigan así, `format-check` las reportará.
