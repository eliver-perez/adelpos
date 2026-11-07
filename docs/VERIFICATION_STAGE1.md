# Verificación de la entrega

Fecha: **7 de noviembre de 2026**, zona America/Monterrey. Equipo: macOS **26.5.2**, Apple Silicon **arm64**. Estas comprobaciones se ejecutaron; no son una lista de pruebas propuestas.

| Comprobación                    | Resultado real                                                                                                                                                                                                            |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go: compilación ARM64           | `bin/ad-el-pos-server` construido correctamente.                                                                                                                                                                          |
| Go: `go test -race ./...`       | Configuración, migrador, API y límite de licencias: PASS, sin carreras detectadas.                                                                                                                                        |
| Go: `go vet ./...`              | PASS.                                                                                                                                                                                                                     |
| JSON estricto/configuración     | Rechazo de campos desconocidos, JSON adicional, secretos ausentes, modo terminal en Go, TLS inseguro remoto, CORS comodín, exposición no autorizada y pool inválido. Override de secreto y escape de contraseña probados. |
| API técnica                     | Liveness, readiness, versión, 404/405/501, CORS y panic sanitizado comprobados por tests HTTP.                                                                                                                            |
| PostgreSQL real                 | Clúster 18.6 exclusivo, puerto 55432, SCRAM y rol `adel_app`; conexión desde Go verificada.                                                                                                                               |
| Migración real 0001             | Aplicada en `adel_pos`; ejecución posterior sin reaplicar.                                                                                                                                                                |
| Integración de migraciones      | Base temporal aleatoria: dos migradores concurrentes, repetición, rollback de SQL fallido, historial/checksum alterados y esquema más nuevo. PASS; base de prueba eliminada.                                              |
| Respaldo                        | `pg_dump --format=custom` generado; catálogo leído con `pg_restore --list`. No se ejecutó una restauración completa.                                                                                                      |
| Frontend                        | TypeScript estricto y bundle Vite correctos.                                                                                                                                                                              |
| Pruebas frontend                | 6 tests: disponible, caída DB, esquema incompatible, API incompatible, respuesta inválida y conexión rechazada. PASS.                                                                                                     |
| Rust                            | 2 tests de destinos y configuración; PASS.                                                                                                                                                                                |
| Tauri desarrollo                | Ventana nativa ejecutada con Vite; consultas reales periódicas registradas por Go.                                                                                                                                        |
| `.app` release ARM64            | Bundle construido y abierto por LaunchServices sin Vite. Ventana nativa de 1220 × 880 identificada con CoreGraphics. Consultas de React → IPC Rust → Go confirmadas en logs.                                              |
| Modo `server-only`              | Segundo Go en 127.0.0.1:8182, con PostgreSQL real; readiness 200; cierre SIGTERM ordenado. Proceso temporal terminado al concluir.                                                                                        |
| Modo `terminal-only`            | Mismo `.app` con `ADEL_TERMINAL_CONFIG` externo: consulta a Go existente, conservando su PID. Restaurado después al modo normal.                                                                                          |
| Cierre de Tauri                 | Aplicación cerrada mediante el evento de salida normal de macOS. Su proceso terminó y Go **mantuvo PID 74967**, con readiness 200.                                                                                        |
| Reapertura de Tauri             | `.app` reabierto, retomó sus tres GET técnicos contra el mismo Go.                                                                                                                                                        |
| Caída y recuperación PostgreSQL | Con Go activo: PostgreSQL detenido → live 200, ready 503; PostgreSQL reiniciado → ready 200 **sin reiniciar Go**.                                                                                                         |
| Caída y recuperación Go         | Go detenido ordenadamente: `.app` permaneció activo. Go reiniciado: el mismo `.app` volvió a consultar sin reiniciarse.                                                                                                   |
| Metadatos/launchd               | `sync-metadata.mjs --check` y `plutil -lint` de la plantilla: PASS. No se instaló launchd.                                                                                                                                |
| Secretos locales                | Configuración y archivos de contraseñas con permisos 0600; `.local` excluido por `.gitignore`.                                                                                                                            |

Evidencia local reproducible: `.local/runtime-verification.json`, `.local/terminal-mode-verification.json`, `.local/server.log`, `.local/postgres.log`. No subir los directorios privados; el registro de esta página no contiene contraseñas ni DSN.

## Límites de la comprobación

- Se verificó el ciclo real de la aplicación y sus consultas. La captura automática de la ventana falló con `could not create image from window`; no se afirma una inspección visual por captura ni una prueba automatizada de todos los controles táctiles/temas.
- Las pruebas operativas de caída/intercambio de modos usan procesos y configuración propios. No se probó una segunda máquina física por LAN; `terminal-only` se verificó contra el mismo servidor local.
- La firma que informa `codesign` es ad hoc del linker, sin TeamIdentifier; no hay firma de distribución ni notarización.
- No se probó restauración completa de respaldo, caída abrupta de energía, carga de muchas terminales, macOS Intel, Windows, Android ni KDS.
- La producción comercial está deliberadamente bloqueada por el límite de licencias. No existe autenticación de negocio, WebSocket activo ni instalador comercial.

## Repetir las pruebas

```bash
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh build-macos
python3 tests/runtime_smoke.py
python3 tests/runtime_smoke.py --terminal-only
```

Los dos últimos comandos interrumpen **solo los servicios de desarrollo del proyecto** y necesitan una sesión gráfica macOS. No ejecutarlos sobre datos reales ni mientras otra ventana de desarrollo esté consultando el mismo backend. Los scripts dejan PostgreSQL, Go y el `.app` abiertos al terminar. Las pruebas Rust/TypeScript aisladas no sustituyen esta verificación operativa.
