# Verificación · Etapa 6.2

Ejecución del **9 de noviembre de 2026**, zona America/Monterrey. Versión **0.6.2**, API v1, esquema **7** (sin migraciones nuevas). Evidencia anterior: [etapa 6.1](VERIFICATION_STAGE6_1.md), [etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md), [etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y [etapa 1](VERIFICATION_STAGE1.md). Avance: [ETAPA_6_2_AVANCE.md](ETAPA_6_2_AVANCE.md).

**Las pruebas no tocaron la instalación real.** Después, con autorización del propietario, se instaló la 0.6.2 (ver «Instalación en la instalación real»). No se leyeron secretos de `.local` ni del Keychain.

## Entorno aislado

Copia del repositorio en el directorio temporal de la sesión (sin `.git`, `.local`, `.tools`, cachés ni artefactos), clúster PostgreSQL 18 temporal en el puerto 55499 (solo loopback), bases `adel_pos_test_*` desechables, Go temporal en 8183 para Playwright y `GOPROXY=off`, según `CLAUDE.md` §6. De la copia se excluyó `docs/etapas/ETAPA_7.md` (especificación del propietario sin seguimiento y sin formato Prettier, ajena a esta etapa).

## Resultados ejecutados

| Comprobación        | Resultado                                                                                                                                                                                                                                                                                                                       |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato y metadatos | Prettier, gofmt y `cargo fmt --check` aprobados; `sync-metadata --check` aprobado con 0.6.2.                                                                                                                                                                                                                                    |
| Go                  | `go vet` (con y sin `integration`) sin hallazgos; `go test -race ./...` aprobado en todos los paquetes.                                                                                                                                                                                                                         |
| PostgreSQL          | Integración de `database`, `administration`, `catalog`, `operations` y `payments` aprobada, incluida `quick_open_acceptance_test.go` (apertura sin nombre en salón y para llevar, edición que vacía el nombre, cambio de mesa, mismo `operation_id` = una sola cuenta, `IDEMPOTENCY_CONFLICT` y división sin nombre rechazada). |
| Frontend            | Vitest **140 pruebas en 24 archivos** (132 en 23 al inicio de la etapa); `tsc --noEmit` y `npm run build` correctos.                                                                                                                                                                                                            |
| Rust                | `cargo test --locked --offline`: 12 aprobadas, 1 ignorada deliberadamente (Keychain real, opt-in).                                                                                                                                                                                                                              |
| Navegadores         | Playwright Chromium y WebKit: recorrido completo aprobado en ambos motores (ver abajo).                                                                                                                                                                                                                                         |
| Revisión visual     | Capturas del panel lateral (mesa libre y ocupada) a 1280 × 720 y 1920 × 1080, tema claro y oscuro, en Chromium y WebKit: sin desplazamiento horizontal y con las acciones principales visibles.                                                                                                                                 |
| Instalación real    | Servidor y `.app` 0.6.2 instalados después de las pruebas, con autorización (sin migraciones).                                                                                                                                                                                                                                  |

Recorrido Playwright (además de las comprobaciones de etapas 4 a 6.1): mesa libre → panel lateral → «Abrir cuenta y capturar» con dos clics simultáneos → una sola cuenta en el servidor → productos → «Enviar a cocina»; mesa ocupada → panel lateral → «Capturar» y «Cobrar»; «+ Nueva cuenta» en mesa ocupada; para llevar → «Nuevo pedido» entra directo a la captura → «Datos del pedido» → envío a cocina sin nombre con el aviso «Pedido sin nombre».

## No ejecutado o no verificado

- Ventana nativa Tauri, pantalla táctil física, Keychain real e impresión física.
- Actualización en tiempo real del panel lateral entre dos terminales (usa SSE y sondeo existentes; sin prueba automática con dos terminales).
- Número de comensales: no implementado (requiere migración; ver el avance).

## Instalación en la instalación real

9 de noviembre de 2026, con autorización del propietario y la aplicación cerrada: `server-stop` (0.6.1 detenido de forma ordenada); `migrate` → `applied:0` (sin migraciones pendientes, esquema 7); `server-start` → 0.6.2, readiness 200, PostgreSQL disponible y esquema compatible; `build-macos` → `.app` 0.6.2 de 9.45 MiB; `open-app` → aplicación iniciada y atendida por el servidor (respuestas 200). No se recorrió el flujo en la ventana nativa.

## Repetir las comprobaciones

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
```

`test-integration` y `test-browser` usan bases desechables en el clúster del proyecto (55432); para aislamiento total, seguir `CLAUDE.md` §6. Mientras `docs/etapas/ETAPA_7.md` siga sin formato, `format-check` lo reportará.
