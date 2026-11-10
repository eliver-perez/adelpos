# Verificación · Etapa 7

Ejecución del **9 de noviembre de 2026**, zona America/Monterrey. Versión **0.7.0**, API v1, esquema **8** (migración `0008_cash_shifts_and_reconciliation.sql`). Evidencia anterior: [etapa 6.2](VERIFICATION_STAGE6_2.md), [etapa 6.1](VERIFICATION_STAGE6_1.md), [etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md), [etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y [etapa 1](VERIFICATION_STAGE1.md). Avance: [ETAPA_7_AVANCE.md](ETAPA_7_AVANCE.md). Guía: [CASH_SHIFTS_AND_RECONCILIATION.md](CASH_SHIFTS_AND_RECONCILIATION.md).

**Las pruebas no tocaron la instalación real.** Después, con autorización del propietario, se aplicó la 0008 y se instaló la 0.7.0 (ver «Instalación en la instalación real»). No se leyeron secretos de `.local` ni del Keychain, y ninguna prueba envió trabajos a CUPS, a una impresora ni a una conexión TCP.

## Entorno aislado

Copia del repositorio en el directorio temporal de la sesión (sin `.git`, `.local`, `.tools`, cachés ni artefactos), clúster PostgreSQL 18 temporal en el puerto 55499 (solo loopback), bases `adel_pos_test_*` desechables, Go temporal en 8183 para Playwright y `GOPROXY=off`, según `CLAUDE.md` §6. De la copia se excluyó `docs/etapas/ETAPA_7.md` (especificación del propietario sin seguimiento ni formato Prettier). La impresión y el pulso del cajón se probaron con el adaptador controlado de la cola (`recordingTransport`).

## Resultados ejecutados (regresión final sobre el árbol 0.7.0)

| Comprobación        | Resultado                                                                                                                                                                                                                                                                                                                                                                                                             |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato y metadatos | Prettier, gofmt y `cargo fmt --check` aprobados; `sync-metadata --check` aprobado con 0.7.0 y esquema 8.                                                                                                                                                                                                                                                                                                              |
| Go                  | `go vet` (con y sin `integration`) sin hallazgos; `go test -race ./...` `ok` en los doce paquetes con pruebas (administration, catalog, config, database, features, httpapi, kitchen, licensing, localsetup, operations, ordering, payments).                                                                                                                                                                         |
| PostgreSQL          | Integración `-race -tags=integration` `ok` en `database` (actualización de una base en esquema 7 a 8, con sus CHECK), `administration`, `catalog`, `operations` (106 s; `TestRestaurantAcceptance` con las etapas 4–6.2 y las pruebas de caja C1–C47), `payments` y `kitchen`. La prueba física opt-in `TestControlledCUPSPrinter` se omitió como siempre. Una base nueva aplica las ocho migraciones (`applied: 8`). |
| Frontend            | Vitest **175 pruebas en 30 archivos** (140 en 24 al inicio de la etapa); `tsc --noEmit` y `npm run build` correctos.                                                                                                                                                                                                                                                                                                  |
| Rust                | `cargo test --locked --offline`: 14 aprobadas, 1 ignorada deliberadamente (Keychain real, opt-in); incluye las rutas de caja de la lista cerrada del puente.                                                                                                                                                                                                                                                          |
| Navegadores         | Playwright Chromium y WebKit: `1 passed` en cada motor (recorrido completo, ver abajo).                                                                                                                                                                                                                                                                                                                               |
| Compilación macOS   | `tauri build --bundles app` en la copia aislada con destino temporal: `AD-EL POS.app` **0.7.0**, ejecutable Mach-O arm64, 9.48 MiB. No reemplazó el `.app` del proyecto ni se abrió.                                                                                                                                                                                                                                  |
| Revisión visual     | Capturas de Caja revisadas en las fases B y C (apertura, operaciones, arqueo ciego, resultado del cierre, vista previa de cortes y aviso del tope) a 1280 × 720 y 1920 × 1080, tema claro y oscuro: durante el arqueo no aparece ningún esperado; la prueba comprueba que no hay desplazamiento horizontal.                                                                                                           |
| Independencia       | Go y PostgreSQL se ejecutaron sin Tauri en todas las suites (Playwright usa un Go temporal real y el puente de pruebas en Node); los trabajadores de impresión procesaron documentos y comandos del cajón sin ventana.                                                                                                                                                                                                |

Cada fase repitió además sus suites por separado (fases A, B y C) con los mismos resultados en su momento; el detalle está en el avance.

Recorrido Playwright `e2e/cash-flow.ts` (además de las comprobaciones de etapas 4 a 6.2): Caja → apertura por denominaciones (2 × $200 + 1 × $100; Go calcula $500.00) → ingreso de $100 → retiro de $200 autoautorizado sin segunda contraseña → corte parcial con el esperado de $400.00 y su vista previa → tope de caja activado con $300.00 (aviso visible) y desactivado (sin aviso) → «Iniciar arqueo y pausar la caja» → el cobro de una cuenta queda bloqueado con el mensaje de Go → conteo ciego ($300.00 declarados, sin ninguna cifra del esperado en pantalla, borrador en el servidor) → confirmación con diálogo → resultado con faltante de $100.00 para el administrador → vista previa del corte definitivo → movimientos, historial y reportes del turno cerrado → el recorrido de cobro posterior abre un turno nuevo en la misma caja. Gastos, rechazos sin cifras, salidas excepcionales y dólares se cubren en la integración PostgreSQL y Vitest.

## Cobertura de la etapa 7 (integración PostgreSQL)

- Apertura con fondo $0, positivo y por denominaciones (total que no coincide rechazado); dos aperturas simultáneas (`SHIFT_ALREADY_OPEN`); reinicio; turno compartido y responsables.
- Ingresos con permiso y sin él; retiros y gastos autoautorizados, rechazados sin autorización, con supervisor distinto y con autoautorización indebida; retiro que excede el disponible sin cifras y excepcional autorizado; correcciones conservando la original.
- Cortes parciales sin modificar saldo, con snapshot correcto e idempotentes, con y sin `cash.expected.read`.
- Arqueo ciego: ningún endpoint devuelve esperado ni diferencias durante `closing` (administrador incluido); cierre sin diferencia, con faltante y con sobrante; cierre con cuentas abiertas; dos confirmaciones simultáneas; movimiento posterior al inicio detectado; cobro, devolución, corrección e ingreso rechazados con `SHIFT_CLOSING` y `SHIFT_CLOSED`; cancelar el arqueo reanuda la caja; reintento incierto sobre turno pausado o cerrado rechazado sin duplicar.
- MXN/USD: función desactivada; fondo en ambas monedas; cobro con dólares y cambio en pesos y viceversa; retiro en dólares; conteo y diferencias por moneda; equivalencias con el tipo de cambio del turno; corrección de operaciones en dólares.
- Regularización posterior al cierre en el turno vigente sin cambiar la diferencia del cerrado; fecha comercial de un turno que cruza medianoche; movimientos sin esperado sin el permiso.
- Impresión de comprobantes y cortes con y sin el esperado, fallo o ausencia de impresora sin cancelar el cierre, reimpresión idempotente y con permiso, vista previa sin escritura; cajón automático con efectivo y no con tarjeta, manual con y sin permiso, comando incierto sin reintento, protocolo incompatible rechazado; tope desactivado, activado solo para quien ve el esperado, ausente en el arqueo y nunca bloqueante; eventos `cash` sin importes y filtrados por permiso.

## No ejecutado o no verificado

- Ventana nativa Tauri (WKWebView), pantalla táctil física, Keychain real y dos terminales nativas simultáneas.
- Impresión física de comprobantes y cortes (papel y corte) y pulso físico del cajón con una impresora ESC/POS real: solo adaptador controlado y prueba unitaria de los bytes (`ESC p m 25 250`). Una impresora por CUPS no puede abrir el cajón.
- Actualización en tiempo real entre dos terminales reales: verificada con la API de eventos (integración), Vitest y el puente de pruebas.

## Instalación en la instalación real

9 de noviembre de 2026, con autorización del propietario, desde la rama `etapa-7`, con la aplicación cerrada:

1. **Preparación.** El servidor 0.6.2 estaba listo con esquema 7. La comprobación de solo lectura previa encontró: migraciones 1–7, sin turnos, 1 caja, 0 pagos, 5 cuentas y 94 permisos (todos en el administrador).
2. **Respaldo.** `backup` generó `backups/adel_pos-20261109T184542Z-fb3ebf.dump`, con el catálogo legible.
3. **Ensayo de restauración.** `restore-check.py` restauró el respaldo en una base temporal: **79 tablas con conteos idénticos** (`.local/stage8-restore-verification.json`).
4. **Detención.** `server-stop` detuvo el servidor de forma ordenada.
5. **Migración.** `migrate` → `applied:1`; la repetición → `applied:0`.
6. **Arranque.** `server-start` inició el servidor 0.7.0.
   - Readiness 200, con PostgreSQL disponible y esquema compatible.
   - `GET /api/v1/version` informa 0.7.0 y esquema 8.
7. **Comprobación de solo lectura posterior.** Transacción de solo lectura con los ayudantes del proyecto, sin imprimir credenciales.
   - Migraciones 1–8.
   - Mismos conteos de cajas, turnos, pagos, devoluciones, movimientos y cuentas.
   - 113 permisos: los 19 `cash.*` nuevos solo en el administrador; cajero, finanzas y supervisor sin cambios.
   - `cash_limit` y `usd_cash` desactivadas y sin tope configurado.
   - Denominaciones: 12 en pesos y 6 en dólares; 4 categorías de gasto.
   - Ninguna operación de efectivo, corte, cierre, cajón ni documento de caja.
8. **Aplicación.** `build-macos` generó el `.app` 0.7.0 (9.48 MiB) y `open-app` lo inició.
   - El servidor lo atendió con 28 respuestas 200 y dos 401 al arrancar (el registro no guarda la ruta).
   - No se recorrió la caja en la ventana nativa.

## Repetir las comprobaciones

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
```

`test-integration` y `test-browser` usan bases desechables en el clúster del proyecto (55432); para aislamiento total, seguir `CLAUDE.md` §6. Mientras `docs/etapas/ETAPA_7.md` siga sin formato, `format-check` lo reportará.
