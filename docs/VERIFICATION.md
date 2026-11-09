# Verificación · Etapa 6.1

Ejecución del **8 de noviembre de 2026**, zona America/Monterrey. Versión **0.6.1**, API v1, esquema **7** en el código. Evidencia anterior: [etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md), [etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y [etapa 1](VERIFICATION_STAGE1.md). Avance por fases: [ETAPA_6_1_AVANCE.md](ETAPA_6_1_AVANCE.md).

**La instalación real no se modificó.** Sigue en 0.6.0 con esquema 6; la migración 0007 espera la autorización del propietario. La única interacción con ella fue una consulta de solo lectura (transacción de solo lectura) para verificar §1.8 de la especificación: ninguna categoría, producto, presentación ni promoción tenía fechas de disponibilidad guardadas. No se leyeron secretos de `.local` ni del Keychain.

## Entorno aislado

Copia del repositorio en el directorio temporal de la sesión (sin `.git`, `.local`, `.tools`, cachés ni artefactos), clúster PostgreSQL 18 temporal en el puerto 55499 (solo loopback), bases `adel_pos_test_*` creadas y eliminadas por suite, Go temporal en 8183 para Playwright y cachés de Go, Cargo y npm en el directorio temporal con `GOPROXY=off`. Es el procedimiento de `CLAUDE.md` §6. Cada fase (A–D) se verificó antes de su commit local; la tabla siguiente es la regresión final sobre el árbol completo con la versión 0.6.1.

## Resultados ejecutados (árbol completo de la etapa)

| Comprobación        | Resultado                                                                                                                                                                                                                                              |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Formato y metadatos | `scripts/dev.sh format` y `format-check` (Prettier, gofmt, `cargo fmt --check`) aprobados; `sync-metadata --check` aprobado con 0.6.1 / esquema 7.                                                                                                     |
| Go                  | `go vet ./...` y `go vet -tags=integration ./...` sin hallazgos; `go test -race ./...` aprobado en todos los paquetes con pruebas, incluidos los nuevos `internal/ordering` e `internal/features`.                                                     |
| PostgreSQL          | Integración de `database`, `administration`, `catalog`, `operations` y `payments` aprobada. Incluye la actualización de una instalación en esquema 6 a la 0007 (permisos solo al administrador, repetible) y los CHECK de dólares sin huecos por NULL. |
| Frontend            | Vitest **132 pruebas en 23 archivos** aprobadas (72 en 16 archivos al inicio de la etapa); `tsc --noEmit` estricto y `npm run build` correctos.                                                                                                        |
| Rust                | `cargo test --locked --offline`: **12 aprobadas**, 1 ignorada deliberadamente (escritura real en Keychain, opt-in). Incluye las rutas nuevas de la lista cerrada (`settings/features`, tipo de cambio y rango del dólar).                              |
| Navegadores         | Playwright Chromium y WebKit: recorrido completo aprobado en ambos motores con la nueva captura táctil y el cobro en dólares (ver abajo).                                                                                                              |
| Bundle macOS        | `tauri build --bundles app` en la copia aislada (destino de compilación temporal, sin reemplazar el `.app` 0.6.0 del proyecto): `AD-EL POS.app` ARM64 **0.6.1**, 9.45 MiB. No se abrió.                                                                |
| Revisión visual     | Capturas de la captura táctil en Chromium y WebKit a 1280 × 720, 1920 × 1080 y 1024 × 768, temas claro y oscuro, incluida la personalización en el panel central y un pedido para llevar. No sustituye WKWebView nativo ni pantalla táctil.            |
| Instalación real    | Sin cambios: 0.6.0 / esquema 6. Solo la consulta de solo lectura descrita arriba.                                                                                                                                                                      |

## Aceptación por fase (§5 de la especificación)

- **Errores (§1)**: una prueba por error que falla con el código anterior (comprobado sobre una instantánea de `HEAD`): `pages/catalog/catalogFixes.test.tsx` (volver al listado sin `window.confirm`, campos numéricos, combos sin «Productos permitidos», selectores sin preselección, botones de quitar, imagen en español, fechas vacías), aceptación de catálogo en Go (original implícito, autorreferencia y original faltante) y la medición de la mesa redonda de 100 × 100 en Playwright a tres tamaños de ventana (con el CSS anterior la diferencia era de 87 px).
- **Formularios (§2)**: unitarias de `ordering`, paleta (`catalog/palette_test.go`), zona horaria y pantalla de inicio heredada (`administration/security_test.go`); aceptación de posiciones de categorías, productos por categoría, áreas, estaciones y métodos de pago (hermano ajeno rechazado sin cambios parciales, renumeración con revisión y auditoría) y de la paleta con color heredado; Vitest de selector de posición, color, horarios, alta continua de mesas y catálogo, código sugerido, vista previa y configuración.
- **Funciones, IVA y dólares (§3)**: funciones activables con la API rechazando dólares con la función desactivada; IVA por producto (herencia, tasa efectiva por unidad, cuentas mixtas 16/8/0 con base + IVA exacto por tasa en precuenta, ticket y devolución); dólares (conversión, pesos y dólares en la misma operación, cambio en ambas monedas, movimientos por moneda, tipo de cambio fuera de rango y confirmación por diferencia, idempotencia con el mismo `operation_id` y concurrencia entre dos cobros); Vitest de la sección de funciones, el selector de IVA, la apertura de turno con tipo de cambio y la captura en dólares.
- **Captura táctil (§4)**: Vitest de la sincronización del borrador (antirrebote, una sola petición en vuelo, sin perder toques, conflicto e incertidumbre), un toque que agrega y suma, personalización en el panel central, «Enviar a cocina» con huella y desactivado sin productos nuevos, cuentas como botones, «Más…» y totales de Go.

Recorrido Playwright (Chromium y WebKit, 1280 × 720 y otros tamaños), sobre la operación de etapas 4 a 6:

1. Configuración: función «Pago en dólares» activada; impresora de tickets con destino controlado; área con posición y mesa redonda de 100 × 100 dibujada como círculo; barra de acciones visible mientras el formulario continúa.
2. Captura: categoría → producto directo (un toque, suma de cantidad) → producto con opciones personalizado en el panel central (cambio de guarnición con cargo) → «Enviar a cocina» → preparación en cocina, modificación, cancelación, división de cuenta e historial.
3. Cobro: apertura de turno con fondo y tipo de cambio, abono en efectivo con cambio en ambas monedas, ticket, precuenta con IVA, descuento, pago con tarjeta y documentos.
4. Para llevar en la misma pantalla de captura: «Cobrar» desactivado sin productos enviados; cobro con $100 + US$5 y cambio entregado en dólares (US$2.30, equivalente a $42.50); entrega registrada.

## No ejecutado o no verificado

- Migración 0007 sobre la instalación real, compilación e inicio de 0.6.1 sobre ella y `restore-check.py`: requieren autorización.
- Ventana nativa Tauri (WKWebView), pantalla completa, Keychain real y `tests/runtime_smoke.py`.
- Pantalla táctil física: los tamaños de los objetivos se comprobaron en navegador, no con toque real.
- Impresión física de tickets con dólares o precuentas.
- Autorización de supervisor y devoluciones en el navegador (cubiertas por Go y Vitest).
- Actualización en tiempo real entre dos terminales durante la captura (cubierta por la lógica de conflicto y sondeo, sin prueba automática con dos terminales).
- Rendimiento con un catálogo grande o muchas terminales; LAN con otra máquina; Mac Intel y Windows.

## Repetir las comprobaciones

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
./scripts/dev.sh build-macos
```

`test-integration` y `test-browser` crean y eliminan bases `adel_pos_test_*` en el clúster del proyecto (55432); para aislamiento total, seguir el procedimiento de `CLAUDE.md` §6. `build-macos` reemplaza el `.app` de `desktop/src-tauri/target/release/bundle/macos/`: **no ejecutarlo mientras la instalación real deba seguir en 0.6.0** (antes de aplicar la 0007).
