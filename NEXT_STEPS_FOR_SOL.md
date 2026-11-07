# Continuidad para Sol · Estado de la etapa 3

## Implementado

Etapa 3: migración 0003, catálogo real, variantes opcionales, ingredientes, complementos/porciones, grupos reutilizables y cantidades originales por producto, combos por referencia con tramos, listas y promociones. Motor decimal único en `internal/catalog`, permisos vigentes, auditoría transaccional y revisiones contra sobrescritura. Interfaz Catálogo y componente `ProductCustomizer` reutilizado en Probar personalización. Logos originales integrados por tema. Leer [docs/CATALOG.md](docs/CATALOG.md) y el contrato antes de extender cálculos.

Conservada la arquitectura de la etapa 1. Migración 0002 y administración real en Go/PostgreSQL: establecimiento, siete roles iniciales, usuarios, permisos por rol y excepciones individuales, sesiones únicas por usuario, dispositivos autorizados, invitaciones de un solo uso, cajas físicas, preferencias y auditoría transaccional. Tauri utiliza Keychain y operaciones enumeradas; el frontend contiene las pantallas correspondientes.

Nombre inicial solicitado por el propietario: **ADEL POS** para el establecimiento; la marca provisional de la aplicación sigue centralizada en config/product.json. Zona horaria configurable, inicialmente la del OS. No reintroducir una zona fija por defecto en el asistente.

admin/admin se crea una sola vez durante setup y mantiene una advertencia no bloqueante hasta cambiar la contraseña. Esta política fue aprobada expresamente. sa existe como identidad reservada sin credencial utilizable hasta que el propietario ejecute provision-sa y elija personalmente un secreto; no inventarlo ni solicitarlo por chat. Ver DEVELOPMENT_MACOS.md.

## Organización y convenciones

- Leer AGENTS.md y docs/CODING_CONVENTIONS.md. No ofuscar ni compactar código. Go con bloques expandidos/gofmt; React y TypeScript con Prettier y JSX anidado legible; revisar manualmente lo que no expande el formateador.
- Go sigue siendo independiente de Tauri y único acceso a PostgreSQL. No convertirlo en sidecar ni introducir microservicios.
- `administration.Service.Execute` delimita las transacciones. Revisa service/auth/setup y archivos de recursos antes de ampliar operaciones.
- La autorización se consulta en PostgreSQL por petición; ninguna caché React puede conceder permisos. Excepción individual explícita prevalece sobre rol.
- Mantener separación entre secreto de dispositivo, sesión y contraseña. Los tokens duraderos nunca llegan al WebView ni a logs/auditoría.
- No modificar sa desde la administración ordinaria ni permitir asignar el rol reservado. Su ocultamiento no lo excluye de auditoría.
- La 0001, 0002 y 0003 aplicadas son inmutables. Añadir la próxima migración y actualizar schema_version; nunca borrar .local para resolver errores.
- `config/product.json` es canónico. `sync` usa Prettier también para archivos generados; mantener lockfiles.
- Loopback por defecto. LAN exige HTTPS, CA/hostname verificados e IP privada explícita. No ampliar CORS/CSP ni agregar plugins amplios para eludir un fallo.
- Caja física no equivale a turno; no introducir fondos ni movimientos ficticios.
- Dinero exacto con numeric/shopspring decimal y cadena JSON; instantes UTC y zona IANA de negocio independiente.

## Verificar antes de continuar

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh db-start
./scripts/dev.sh server-start
./scripts/dev.sh open-app
```

docs/VERIFICATION.md distingue pruebas ejecutadas, pruebas con transporte simulado de interfaz y comprobaciones operativas reales. No presentar una prueba de componentes como prueba de una segunda máquina física. No incluir datos/secretos de .local en commits o respuestas.

## Limitaciones y decisiones pendientes

El teclado flotante no se implementó: se guarda su preferencia por terminal, como permite el alcance. La notificación de sesión reemplazada/revocada se obtiene por polling cada cinco segundos; Go rechaza inmediatamente las operaciones inválidas. `/events` sigue en 501; no hay WebSocket activo ni outbox/replay.

No existen pedidos, mesas, ventas, pagos, turnos, movimientos de inventario, Android, KDS ni servidor comercial de licencias. El perfil production continúa bloqueado hasta integrar licenciamiento. El rol PostgreSQL de desarrollo es dueño de la base; separar runtime/migrador para producción. Keychain se prueba en macOS; Windows, servicios, firma/notarización e instaladores requieren su etapa propia.

Faltan políticas de retención de sesiones/auditoría/intentos y una política operativa de respaldos con restauración periódica. La CA local tiene vigencia y SAN explícitos; renovación/rotación y distribución comercial de confianza necesitan diseño. Revisar amenazas de acceso físico al usuario OS que controla la configuración privada y la base; las herramientas CLI de recuperación presuponen esa autorización local.

## Siguiente tarea propuesta

La siguiente etapa propuesta es captura de pedidos, únicamente con sus criterios autorizados. Reutilizar las identidades del catálogo y `ProductCustomizer`; solicitar un cálculo nuevo de Go al confirmar, conservar snapshots comerciales e implementar idempotencia en la transacción de pedido. No volver a programar precios en TypeScript ni deducir inventario todavía. La ventana de previsualización actual no es una venta ni un pedido persistido.

Antes de ampliar: revisar límites de selectores/respuestas y medir carga. El editor carga referencias de catálogo y tiene un límite explícito de 5000 productos; implementar búsqueda remota antes de crecer más. La disponibilidad por horario no sustituye el estado activo. Los combos requieren validación de hijos y ciclos. El cambio de lista predeterminada incrementa y audita también la revisión de la lista desplazada.

La elección privada de contraseña sa sigue perteneciendo al propietario y no bloquea esta continuidad. No modificar su mecanismo. Mantener respaldo/restauración verificable antes de cambios reales; ver evidencia ejecutada en docs/VERIFICATION.md.

Antes de Android/KDS: contrato de operación UUID, idempotencia durable en la transacción, outbox y reanudación de eventos. Antes de uso comercial: integración de licencias, TLS/certificados operativos, roles DB separados, firma, actualizaciones, backups y servicio persistente.

El arranque del Llavero usa OnceLock en segundo plano para mantener la ventana activa durante el consentimiento de macOS; nunca tratar una autorización pendiente como credenciales vacías. Las nuevas compilaciones de desarrollo pueden requerir consentimiento OS. La prueba nativa con namespace temporal verifica transporte y procesos; no demuestra el acceso al perfil real si su autorización está pendiente. Leer la evidencia actual antes de declarar esa verificación completa.
