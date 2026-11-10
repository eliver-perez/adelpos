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

- **Código**: fases 0, A, B y C y el servidor de la fase D de la etapa 8 en commits locales de la rama `etapa-8`, cada una
  con sus suites aprobadas en un entorno aislado (clúster PostgreSQL temporal en el puerto 55499 y Go temporal para
  Playwright), según [VERIFICATION_STAGE8.md](VERIFICATION_STAGE8.md). La interfaz de la fase D y la regresión final de la
  fase E estaban pendientes al redactar este documento.
- **Instalación real**: sigue en **0.7.1 y esquema 9**. Las migraciones 0010–0014 **no se han aplicado**: requieren
  autorización expresa del propietario, con respaldo, ensayo de restauración y el procedimiento de
  [DEVELOPMENT_MACOS.md](../DEVELOPMENT_MACOS.md). La última actualización registrada de la instalación real es la de la
  etapa 7.1 ([VERIFICATION_STAGE7_1.md](VERIFICATION_STAGE7_1.md), «Instalación real»).
- **Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432) ni enviaron trabajos a
  CUPS, a una impresora, a un cajón o a una conexión TCP.

## Qué está verificado

| Área                                           | Evidencia                                                                                                                                                                                                      |
| ---------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Motor de costos y conversiones                 | Unitarias exactas de `inventory/costing_test.go` y `units_test.go` (casos con cifras en [INVENTORY_AND_PURCHASING.md](INVENTORY_AND_PURCHASING.md#casos-verificados)).                                         |
| Libro, existencias, almacenes y permisos       | Integración PostgreSQL de la fase A: idempotencia por `entry_key`, inmutabilidad, consistencia de existencias y valuaciones, visibilidad de costos y eventos.                                                  |
| Recetas, porciones y consumo al liberar        | Unitarias de resolución y día operativo e integración de la fase B: personalizaciones, complementos, combos, empaques, solicitudes, trabajador, cancelaciones, reconciliador y medición del consumo asíncrono. |
| Indicaciones rápidas con efecto y complementos | Unitarias de catálogo, cocina y huellas, integración de la 0012 y de la captura, Vitest y Playwright.                                                                                                          |
| Compras y vínculo con caja                     | Unitarias de importes, prorrateo, RFC y devoluciones e integración de la fase C con la API de caja real.                                                                                                       |
| Ajustes, mermas, transferencias y conteos      | Unitarias e integración de la fase D (servidor): autorizaciones, conteo con ventas concurrentes, aplicación idempotente, niveles, alertas y sugerencias.                                                       |
| Actualización de instalaciones existentes      | `TestStage8UpgradeOfExistingInstallation`: una base simulada en esquema 9 con datos recibe 0010–0014 una sola vez sin cambiar sus datos.                                                                       |
| Puente nativo                                  | Pruebas Rust de la lista cerrada (81 operaciones `inventory_*`).                                                                                                                                               |
| Pantallas de las fases A–C                     | Vitest y Playwright Chromium/WebKit (`inventory`, `takeaway-quick-notes`, `stock-consumption` y `vendor-purchases`, junto con `restaurant` de las etapas 4–7.1).                                               |
| Etapas anteriores                              | En cada fase, la aceptación encadenada de `internal/operations` (etapas 4–7.1) y `restaurant.spec.ts`.                                                                                                         |

## No ejecutado o no verificado

- Aplicación de 0010–0014 a la instalación real y su respaldo y ensayo de restauración con estas migraciones.
- Ventana nativa Tauri (WKWebView), pantalla táctil física y teclado virtual en Inventarios y en la captura.
- Papel: ninguna comanda con indicaciones con efecto se imprimió.
- Volumen real, muchas terminales a la vez, LAN con otra máquina, Keychain real y Windows.
- Interfaz de la fase D y regresión final de la fase E (pendientes al redactar este documento).

## Regresión final (fase E)

**Pendiente.** La regresión completa sobre el árbol 0.8.0 y esquema 14 (estáticas, Go con `-race`, integración PostgreSQL,
Vitest, `tsc` y build, Rust, Playwright Chromium y WebKit y el paquete `.app`) se registrará aquí al terminar la fase E.

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
