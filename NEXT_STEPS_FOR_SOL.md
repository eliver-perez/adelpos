# Continuidad para Sol High

## Estado real

Etapa 1 implementada: Go independiente, PostgreSQL real, migración técnica 0001, API v1 de diagnósticos, React/TypeScript/Tailwind dentro de Tauri 2, configuración para los tres modos y scripts de desarrollo macOS. Consulta [docs/VERIFICATION.md](docs/VERIFICATION.md) para los resultados reales, incluidas limitaciones; no reemplazar evidencia por supuestos.

No hay productos, pedidos, mesas, caja, pagos, inventarios, Android, KDS, servidor comercial de licencias ni instaladores Windows. `/api/v1/events` responde 501 de forma intencional. `events.Envelope` es un punto de extensión, no un WebSocket activo. `licensing.CheckStartup` permite únicamente desarrollo; producción falla hasta integrar su proveedor. No afirmar que existen usuarios/permisos, sincronización offline, idempotencia, outbox o gestión de terminales por tener su diseño documentado.

## Leer primero

1. `ARCHITECTURE.md` y `API_CONTRACTS.md` para contratos/límites.
2. `DEVELOPMENT_MACOS.md` para entorno y comandos exactos.
3. `backend/cmd/ad-el-pos-server/main.go` y `backend/internal/httpapi/router.go` para composición/API.
4. `backend/internal/database/migrate.go` para invariantes de migración.
5. `desktop/src/services/diagnostics.ts`, `hooks/useConnection.ts`, `src-tauri/src/main.rs` para flujo real.

## Convenciones a preservar

- Un solo monolito modular Go y una sola aplicación de escritorio completa. El backend no es sidecar de Tauri.
- Datos y autorización en Go; ni React ni Rust hablan con PostgreSQL. El bridge nativo actual solo admite tres rutas técnicas fijas.
- No tocar clústeres ajenos; `.local` y `.tools` son locales, privados e ignorados. No imprimir configuración/secretos.
- `config/product.json` centraliza metadatos; `./scripts/dev.sh sync` regenera y `test` revisa coherencia. Mantener lockfiles.
- Nunca modificar una migración aplicada: crear la siguiente, actualizar `schema_version` y probar avance/repetición/fallo. No introducir SQL no transaccional sin revisar el migrador.
- Localhost por defecto; no ampliar CORS/CSP a comodines para resolver una prueba. No habilitar escrituras remotas sin identidad real.
- Errores públicos genéricos, request IDs y logs JSON sin secretos. Comentarios útiles en español, nombres descriptivos.
- Tipado estricto, estados de diagnóstico reales, componentes reutilizables y sin paneles comerciales ficticios.
- Dinero exacto, timestamps UTC, zona IANA de negocio separada, FK explícitas y transacciones por invariante.

## Siguiente tarea concreta propuesta: identidad local y permisos

Antes del primer módulo comercial, diseñar e implementar un corte vertical de autenticación de usuario y enrolamiento/revocación de terminales. Empezar por una breve decisión de amenazas y flujo de alta local del primer administrador; confirmar requisitos de credenciales/PIN y operación offline. No crear un administrador con contraseña predeterminada ni un endpoint de alta remoto anónimo.

Entrega sugerida: migración 0002 con usuarios, dispositivos, sesiones y permisos mínimos justificados; hashing de contraseñas apropiado; sesión revocable; middleware Go de autenticación/autorización; pantalla de inicio de sesión Tauri; un endpoint protegido de identidad; auditoría mínima. Mantener diagnósticos no sensibles accesibles. Pruebas de acceso anónimo rechazado, permiso insuficiente, revocación, expiración y dos terminales concurrentes.

Antes de activar LAN operativa, cerrar también TLS/confianza local y almacenamiento seguro de credenciales en escritorio. No convertir el perfil development en producción para sortear el límite de licencias. No desarrollar todavía catálogo/pedidos hasta acordar el alcance siguiente con el usuario.

## Decisiones pendientes y riesgos

- Nombre definitivo, bundle ID controlado, versionado de actualizaciones y migración de configuración al renombrar.
- Credenciales usuario vs PIN; roles, permisos, bootstrap seguro y recuperación del administrador.
- Certificados/confianza LAN, descubrimiento de servidor y firewall. HTTP LAN actual es solo diagnóstico de desarrollo.
- Dónde guardar datos comerciales y logs, usuario de servicio, retención/rotación, LaunchDaemon y Windows Service.
- Compatibilidad simultánea de versiones API/esquema. Actualmente el cliente exige coincidencia de esquema exacta.
- Outbox/eventos, límites WebSocket, cursor/replay y recuperación KDS; operación UUID y almacenamiento idempotente antes de Android offline.
- Proveedor de licencias, firmas, caché offline, derechos por módulo y reglas de gracia. No hay protocolo definitivo.
- Política de backups, verificación de restauración y fallos de energía. El comando de respaldo no sustituye una política operativa.
- Distribución firmada/notarizada macOS, empaquetado Windows, impresoras y procesos de actualización: no implementados.
- El role de aplicación es propietario de su base para facilitar migraciones en desarrollo; separar rol migrador/rol runtime en producción.
- Pruebas realizadas en este Mac ARM64; no inferir compatibilidad probada con Windows, Mac Intel u otras versiones de PostgreSQL.

## Comprobación antes de continuar

```bash
./scripts/dev.sh db-start
./scripts/dev.sh server-start
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh desktop
```

Preservar la independencia de procesos: cerrar/reabrir Tauri debe mantener el mismo Go y recuperar conexión. No borrar `.local` como solución genérica a un error.
