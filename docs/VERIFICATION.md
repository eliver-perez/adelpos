# Verificación · Etapa 6

Ejecución del **8 de noviembre de 2026**, zona America/Monterrey. Versión **0.6.0**, API v1, esquema **6** en el código. Evidencia anterior: [etapa 5](VERIFICATION_STAGE5.md) (incluye las correcciones previas a esta etapa), [etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y [etapa 1](VERIFICATION_STAGE1.md). Avance por fases: [ETAPA_6_AVANCE.md](ETAPA_6_AVANCE.md).

**La instalación real no se modificó.** La migración 0006 no se aplicó a `adel_pos` (sigue en esquema 5); el servidor real y `bin/ad-el-pos-server` continúan en 0.5.0; no se reiniciaron servicios, no se restauraron respaldos ni se leyeron secretos de `.local` o del Keychain.

## Entorno aislado

Todas las pruebas se ejecutaron en una copia del repositorio en el directorio temporal de la sesión (sin `.git`, `.local`, `.tools`, cachés ni artefactos), con un clúster PostgreSQL 18 temporal en el puerto 55499 (solo loopback), cachés de Go, Cargo y npm en el directorio temporal y `GOPROXY=off`. Las bases `adel_pos_test_*` se crearon y eliminaron por cada suite. Playwright usó un Go temporal en 8183 contra ese clúster. Es el procedimiento descrito en `CLAUDE.md` §6; equivale a `scripts/dev.sh test`, `test-integration` y `test-browser`, pero sin usar el clúster del proyecto (55432).

Cada fase se verificó sobre la combinación exacta que se confirmó en su commit local: 6A (backend), 6B (interfaz 6B con backend 6A), 6C (interfaz 6C con backend 6A + 6C, sin documentos de 6D) y 6D (árbol completo).

## Resultados ejecutados (árbol completo de la etapa)

| Comprobación        | Resultado                                                                                                                                                                                                                               |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato y metadatos | `scripts/dev.sh format` y `format-check` (Prettier, gofmt, `cargo fmt --check`) aprobados; `sync-metadata --check` aprobado.                                                                                                            |
| Go                  | `go vet ./...` y `go vet -tags=integration ./...` sin hallazgos; `go test -race ./...` aprobado en todos los paquetes con pruebas, incluido `internal/payments`.                                                                        |
| PostgreSQL          | Integración de `database`, `administration`, `catalog`, `operations` y `payments` aprobada. La aceptación de cobros vive en `operations` (`payments_acceptance_test.go`, `payments_adjustments_test.go`, `payments_documents_test.go`). |
| Frontend            | Vitest **72 pruebas en 16 archivos** aprobadas (31 en 9 archivos al inicio de la etapa); `tsc --noEmit` estricto y `npm run build` correctos.                                                                                           |
| Rust                | `cargo test --locked --offline`: **11 aprobadas**, 1 ignorada deliberadamente (escritura real en Keychain, opt-in). Incluye la lista cerrada de operaciones `payments_*`.                                                               |
| Navegadores         | Playwright Chromium y WebKit: recorrido completo aprobado en ambos motores, ahora con turno, cobros, precuenta, descuento, ticket, documentos y entrega para llevar (ver abajo).                                                        |
| Bundle macOS        | `tauri build --bundles app` en la copia aislada (destino de compilación temporal, sin reemplazar el `.app` del proyecto): `AD-EL POS.app` ARM64 **0.6.0**, 9.43 MiB. No se abrió.                                                       |
| Revisión visual     | Capturas con transporte simulado en Chromium a 1280×720, 1024×768 y 1920×1080, temas claro y oscuro: sin desplazamiento horizontal, objetivos ≥ 44 px y sin errores de consola. No sustituye WKWebView nativo ni pantalla táctil.       |
| Instalación real    | **No ejecutado** por diseño: requiere autorización del propietario (respaldo, ensayo de restauración, `server-stop`, `migrate`, `server-start`).                                                                                        |

## Aceptación funcional (§24 de la especificación)

Pruebas PostgreSQL con Go real, transacciones, locks e idempotencia reales:

- **Cobros (1–10)**: total en efectivo con cambio y cierre; tarjeta con referencia opcional y obligatoria; parcial y mixto sucesivo con cambio de un abono; importes inválidos (`-1`, `1.234`, `abc`, `0`, `1..2`, `1e2`) y sobrepagos rechazados.
- **Productos y cuentas (11–20)**: cobro de una partida, de una unidad y parcial de una selección; integridad de combos; división y combinación con pagos; transferencias de cantidades pendientes; productos nuevos tras un abono; repartidor de centavos exacto (también en pruebas unitarias de `payments`).
- **Descuentos y extras (21–30)**: por producto y general con base e IVA reducidos; 100 % y cortesía sin pago ficticio; límite por rol; autorización puntual, sesión del supervisor intacta; cargos generales independientes de complementos; auditoría de cambios.
- **Propinas y devoluciones (31–42)**: propinas en efectivo y electrónicas separadas de la venta; devolución total con otro método, parcial de un pago, por productos y sin duplicados; con permiso directo, por umbral y con supervisor; corrección de método en turno abierto conservando los movimientos originales.
- **Caja (43–50)**: apertura mínima con fondo $0, rechazo sin turno, un turno por caja, movimientos ligados a caja, turno y usuario; concurrencia entre cajeros; saldo conservado al reiniciar Go.
- **Tickets y entrega (51–59)**: precuenta sin registrar venta; ticket con interruptor; pago válido aunque falle la impresora o no haya impresora; reimpresión idéntica sin duplicar cobros; entrega solo de pedidos liquidados, pagado pero pendiente de entrega y entrega directa.
- **Integridad y seguridad (61–70)**: mismo identificador recupera el resultado, otro contenido responde conflicto, doble envío; cobro simultáneo desde dos terminales; sesión sin permisos y revocada; un 403 no deja escrituras (S2, 70). La auditoría financiera (67) se comprueba en cargos, cancelaciones, cambio de IVA y autorizaciones (sin contraseñas); los cálculos decimales exactos (68), en las pruebas unitarias de importes, repartidor e IVA; la conservación de precios confirmados (69), en A4 y en las ediciones que conservan importes.
- **IVA (A1–A7)**: tasas 16/8/0 desde la configuración; rechazo en Go y PostgreSQL de `15`, `0.16`, `16.0`, `-8`, `abc`, `08`, `+8` y vacío; base + IVA = total; cambio de tasa sin alterar cuentas anteriores, auditado y protegido por permiso; descuentos, cortesías, cargos y propinas en el desglose; devolución con la tasa original.
- **Partidas con pago (P1–P6)** y **seguridad heredada (S1–S5)**: bloqueos sobre unidades liquidadas y parciales; transferencia de unidades libres; total nunca menor que lo pagado; combinación con origen sin pagos; unidad liberada tras devolución; delegación del límite de descuento; límite de intentos; autoautorización rechazada; autorización incompleta tratada como requerida. Las pruebas negativas de H1 y M1 se conservan de las correcciones previas.

Pruebas de interfaz (Vitest con jsdom):

- **S6**: un pago incierto conserva identificador y cuerpo al desmontar y volver, y el reintento usa el mismo `operation_id` sin contraseña.
- **S7**: teclado de cobro con `+`, punto, doble punto y tercer decimal sin vaciar ni corromper el importe.
- Métodos sin casillas, cambio desde la previsualización, doble envío bloqueado, `BALANCE_CHANGED`, autorización de supervisor, ajustes, devoluciones, correcciones, impresión, precuenta, documentos y entrega.

Recorrido Playwright (Chromium y WebKit, 1280×720), sobre la operación existente de etapas 4 y 5:

1. Impresora de tickets configurada en Cobros → Políticas con el destino controlado de aceptación (sin hardware).
2. Cuenta de salón de $150: apertura de turno con fondo de $500 mediante el teclado fijo; abono de $100 en efectivo con $200 recibidos y $100 de cambio calculados por Go; ticket encolado (requiere intervención por el destino sin hardware) sin afectar el pago.
3. Precuenta con IVA 16 % ($129.31 + $20.69), pagos registrados, saldo de $50 y leyenda CFDI; impresión encolada.
4. Descuento general de 10 % ($15) dentro del límite del administrador, sin autorización adicional.
5. Pago de los $35 restantes con tarjeta de débito: cuenta liquidada y cerrada; documentos de la cuenta con ticket y precuenta; sin desplazamiento horizontal.
6. Pedido para llevar: «Marcar como entregado» con saldo abre el cobro con la entrega activada; pago exacto en efectivo y entrega registrada.

## No ejecutado o no verificado

- Aplicar la 0006 a la instalación real, compilar e instalar 0.6.0 en ella y repetir `restore-check.py`: requiere autorización.
- Ventana nativa Tauri con WKWebView, Keychain real, pantalla completa de 0.6.0 y `tests/runtime_smoke.py`: no se abrió la aplicación nativa ni se interrumpieron servicios.
- Impresión física de tickets o precuentas: ninguna prueba envió a una impresora real.
- Autorización de supervisor y devoluciones en el navegador: cubiertas por Go y Vitest, no por Playwright (el recorrido usa al administrador, cuyo límite es 100 %).
- Actualización en tiempo real del tablero tras cobros y entregas (60): los eventos `payments.recorded` y `takeaway.delivered` se escriben en la misma transacción y la interfaz recarga ante eventos y por sondeo, pero no hay una prueba automática que lo compruebe entre dos terminales.
- Pantalla táctil física, LAN con otra máquina, muchas terminales, Mac Intel y Windows.

## Repetir las comprobaciones

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
./scripts/dev.sh build-macos
```

`test-integration` y `test-browser` crean y eliminan bases `adel_pos_test_*` en el clúster del proyecto (55432); para aislamiento total, seguir el procedimiento de `CLAUDE.md` §6. `build-macos` reemplaza el `.app` de `desktop/src-tauri/target/release/bundle/macos/`; no ejecutarlo mientras la instalación real deba seguir en 0.5.0.
