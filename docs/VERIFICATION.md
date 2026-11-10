# Verificación · Etapa 8

Estado al **10 de noviembre de 2026**, zona America/Monterrey. Versión **0.8.0**, API v1, esquema **14** (migraciones
`0010_inventory_core.sql` a `0014_inventory_control.sql`, etapa 8: inventarios, recetas, almacenes, compras y costos).
Evidencia detallada por fase: [VERIFICATION_STAGE8.md](VERIFICATION_STAGE8.md). Evidencia anterior:
[etapa 7.1](VERIFICATION_STAGE7_1.md), [etapa 7](VERIFICATION_STAGE7.md), [etapa 6.2](VERIFICATION_STAGE6_2.md),
[etapa 6.1](VERIFICATION_STAGE6_1.md), [etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md),
[etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y
[etapa 1](VERIFICATION_STAGE1.md). Avance: [ETAPA_8_AVANCE.md](ETAPA_8_AVANCE.md). Guía:
[INVENTORY_AND_PURCHASING.md](INVENTORY_AND_PURCHASING.md).

## Estado

- **Código**: fases 0, A, B, C y D (servidor e interfaz), versión y documentación de la fase E y las correcciones de la
  revisión independiente (fase F del avance) en commits locales de la rama `etapa-8` (hasta `3a145e9`), cada parte con sus
  suites aprobadas en un entorno aislado (clúster PostgreSQL temporal en el puerto 55499 y Go temporal para Playwright),
  según [VERIFICATION_STAGE8.md](VERIFICATION_STAGE8.md). La regresión final de la fase E sobre una copia exacta del commit
  `3a145e9` aprobó completa (ver «Regresión final»).
- **Instalación real**: en **0.8.0 y esquema 14**. 0010–0014 se aplicaron a la instalación real el 10 de noviembre de 2026 con autorización del propietario (respaldo `backups/adel_pos-20261110T212540Z-35f5fe.dump` con restauración verificada: 92 tablas idénticas; `migrate` `applied:5` y repetición `applied:0`; readiness 200 con esquema compatible; servidor y `.app` 0.8.0); detalle en [ETAPA_8_AVANCE.md](ETAPA_8_AVANCE.md), «Instalación real».
- **Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432) ni enviaron trabajos a
  CUPS, a una impresora, a un cajón o a una conexión TCP.

## Qué está verificado

| Área                                           | Evidencia                                                                                                                                                                                                                                                                                                                                                         |
| ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Motor de costos y conversiones                 | Unitarias exactas de `inventory/costing_test.go` y `units_test.go` (casos con cifras en [INVENTORY_AND_PURCHASING.md](INVENTORY_AND_PURCHASING.md#casos-verificados)).                                                                                                                                                                                            |
| Libro, existencias, almacenes y permisos       | Integración PostgreSQL de la fase A: idempotencia por `entry_key`, inmutabilidad, consistencia de existencias y valuaciones, visibilidad de costos y eventos.                                                                                                                                                                                                     |
| Recetas, porciones y consumo al liberar        | Unitarias de resolución y día operativo e integración de la fase B: personalizaciones, complementos, combos, empaques, solicitudes, trabajador, cancelaciones, reconciliador y medición del consumo asíncrono.                                                                                                                                                    |
| Indicaciones rápidas con efecto y complementos | Unitarias de catálogo, cocina y huellas, integración de la 0012 y de la captura, Vitest y Playwright.                                                                                                                                                                                                                                                             |
| Compras y vínculo con caja                     | Unitarias de importes, prorrateo, RFC y devoluciones e integración de la fase C con la API de caja real.                                                                                                                                                                                                                                                          |
| Ajustes, mermas, transferencias y conteos      | Unitarias e integración de la fase D (servidor): autorizaciones, conteo con ventas concurrentes, aplicación idempotente, niveles, alertas y sugerencias.                                                                                                                                                                                                          |
| Correcciones de la revisión (fase F)           | Motor con elaboraciones y reversiones con existencia negativa, entradas sin costo (`COST_REQUIRED`), último costo válido e invariantes en 200 secuencias pseudoaleatorias; integración de la valuación por insumo con `costs.read` + `valuation.read` y de las fugas de existencias cerradas; prueba de mutación que reintroduce los seis defectos y los detecta. |
| Actualización de instalaciones existentes      | `TestStage8UpgradeOfExistingInstallation`: una base simulada en esquema 9 con datos recibe 0010–0014 una sola vez sin cambiar sus datos.                                                                                                                                                                                                                          |
| Puente nativo                                  | Pruebas Rust de la lista cerrada (81 operaciones `inventory_*`).                                                                                                                                                                                                                                                                                                  |
| Pantallas de las fases A–D                     | Vitest (404 pruebas en 46 archivos tras la interfaz de la fase D) y Playwright Chromium/WebKit (`inventory`, `takeaway-quick-notes`, `stock-consumption`, `vendor-purchases` y `warehouse-control`, junto con `restaurant` de las etapas 4–7.1: 6 aprobadas por motor).                                                                                           |
| Etapas anteriores                              | En cada fase, la aceptación encadenada de `internal/operations` (etapas 4–7.1) y `restaurant.spec.ts`.                                                                                                                                                                                                                                                            |

## No ejecutado o no verificado

- Ventana nativa Tauri (WKWebView), pantalla táctil física y teclado virtual en Inventarios y en la captura.
- Papel: ninguna comanda con indicaciones con efecto se imprimió.
- Volumen real, muchas terminales a la vez, LAN con otra máquina, Keychain real y Windows.

## Regresión final (fase E)

**Aprobada** el 10 de noviembre de 2026 sobre una copia exacta del commit `3a145e9` (0.8.0, esquema 14, con las
correcciones de la fase F y la interfaz de la fase D; los commits posteriores solo cambian documentación), con el clúster
temporal 55499 y Go temporal en un puerto propio:

- Prettier, gofmt, `cargo fmt`, `sync-metadata --check`, `go vet` y `go vet -tags=integration`: aprobados.
- Go unitarias con `-race`: todos los paquetes `ok`.
- Integración PostgreSQL con `-race`: `database` (3.5 s, actualización desde el esquema 9 hasta la 0014), `administration`
  (55.0 s), `catalog` (6.8 s), `operations` (216.5 s, aceptación encadenada de las etapas 4–8), `payments` (1.5 s),
  `kitchen` (1.3 s) e `inventory` (1.5 s): `ok`.
- Vitest: 46 archivos y 404 pruebas aprobadas; `tsc --noEmit` y build sin errores.
- `cargo test --locked --offline`: 20 aprobadas y 1 ignorada (la de Keychain de siempre).
- Playwright en Chromium y en WebKit: 6 de 6 aprobadas en cada motor (`inventory`, `restaurant`, `stock-consumption`,
  `takeaway-quick-notes`, `vendor-purchases` y `warehouse-control`), 3.6 min por motor, sin reintentos.
- Paquete de escritorio: `tauri build --bundles app` en la copia aislada, con `CARGO_TARGET_DIR` en el scratchpad: `AD-EL
POS.app` 0.8.0, ejecutable Mach-O arm64, 9.6 MB. No se instaló ni se abrió.
- Las consultas de solo lectura previstas para la instalación real se probaron contra una base temporal migrada hasta el
  esquema 14 (migración repetida: `applied:0`).

## Repetir las comprobaciones

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
```

`test-integration` y `test-browser` usan bases desechables en el clúster del proyecto (55432); para aislamiento total,
seguir `CLAUDE.md` §6. Las pruebas de inventario viven en `internal/operations` (encadenadas a `TestRestaurantAcceptance`) y
en `internal/database`, que ya están en `scripts/database.py`. La medición del consumo es opcional
(`ADEL_INVENTORY_MEASURE=1`). Las especificaciones del propietario `docs/etapas/ETAPA_7.md`, `ETAPA_7_1.md` y `ETAPA_8.md`
no tienen formato Prettier (`prettier --check`, 10 de noviembre de 2026) y no se formatean: mientras sigan así,
`format-check` las reportará.
