# AD-EL POS · Etapa 5

Aplicación de escritorio para restaurantes: monolito modular Go, PostgreSQL y React/TypeScript/Tailwind dentro de Tauri 2. Conserva configuración del establecimiento, usuarios, roles, permisos, sesiones, terminales, cajas físicas y auditoría. Añade catálogo, ingredientes, personalizaciones, combos, listas y promociones con un motor decimal exacto en Go. Incluye áreas, mesas, cuentas, pedidos persistidos y operación para llevar. Integra cocina, estaciones, rutas, comandas, cola durable de impresión y eventos autenticados. Inicia en pantalla completa nativa con salida y regreso visibles; las pantallas aprovechan la resolución disponible. No registra cobros ni movimientos físicos de inventario.

## Inicio en este Mac

```bash
cd "/Users/eliver/Documents/ADEL POS/ad-el-pos"
./scripts/dev.sh db-start
./scripts/dev.sh migrate
./scripts/dev.sh prepare-setup
./scripts/dev.sh server-start
./scripts/dev.sh open-app
```

Para desarrollo con recarga: `./scripts/dev.sh desktop`. Para construir el `.app`: `./scripts/dev.sh build-macos`. La compilación está en `desktop/src-tauri/target/release/bundle/macos/AD-EL POS.app`.

En una instalación nueva aparece el asistente local. El nombre inicial del establecimiento es **ADEL POS** y la zona horaria se obtiene del sistema operativo; ambos son editables. Al guardar se crea **Caja 1**, sin abrir turnos ni registrar fondos, y se autoriza la terminal principal. Después accede con **admin / admin**. La advertencia de contraseña inicial recomienda cambiarla, pero permite continuar.

El asistente no vuelve a inicializar una instalación configurada ni restablece contraseñas. Los usuarios y cambios se guardan en PostgreSQL. Cerrar Tauri conserva Go y PostgreSQL; ambos se detienen de forma explícita con `server-stop` y `db-stop`.

## Mesas, cuentas y pedidos

En **Restaurante → Áreas y distribución** crea áreas y mesas y guarda su distribución. En **Mesas**, selecciona mesa y cuenta sin salir de la pantalla; admite varias cuentas por mesa y cuentas sin mesa. Captura productos del catálogo, personaliza con el mismo editor y guarda el borrador. **Confirmar pedido** registra un lote separado con precios y promociones históricos. Un mesero autorizado puede modificar o cancelar con motivo, sin aprobación universal de supervisor.

Desde **Acciones de cuenta** divide cantidades enteras, transfiere productos, combina cuentas, cambia mesa/responsable o consulta historial. Las unidades conservan identidad, personalización y centavos; las transferencias no generan preparación adicional. Una revisión obsoleta conserva la captura para revisión. Una respuesta incierta se recupera con el mismo UUID para evitar duplicados.

**Para llevar** utiliza listado a la izquierda y cuenta a la derecha, orden por antigüedad, búsqueda, avisos configurables y programación en la zona horaria del establecimiento. Ventanilla es un origen de esta misma modalidad. Confirmar crea una intención durable y resuelve la liberación real a cocina; la configuración incompleta deja advertencias y el pedido conservado. Preparación, pago y entrega permanecen separados; no se permite marcar entregado sin el futuro proceso de cobro.

Los acordeones comparten un componente accesible en pantallas anteriores y nuevas. El teclado virtual sigue la preferencia de cada terminal. Consulta [docs/RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md) para ciclo de vida, precios, concurrencia, límites y recorrido de uso. La integración actual está en [Cocina e impresión](docs/KITCHEN_AND_PRINTING.md).

## Cocina e impresión

En **Cocina y preparación** configura impresoras, estaciones y rutas por categoría/producto. Admite varios destinos por producto, varias estaciones en una impresora y documentos independientes o unificados con secciones. Los productos nuevos generan comandas normales; las ediciones y cancelaciones producen avisos históricos. La reimpresión conserva la presentación original.

La cola persiste intentos y resultados. CUPS o ESC/POS TCP aceptado significa recepción del transporte; no confirma salida de papel. Un resultado incierto exige revisión con motivo y nunca se reenvía automáticamente. Go procesa impresión y pedidos programados aunque Tauri esté cerrado. El estado listo requiere completar todas las estaciones y permanece separado de pago/entrega.

Los eventos utilizan SSE autenticado desde Rust, con outbox, cursor, replay, deduplicación y recuperación de estado completo. Se prepararon contratos de recepción/estado para el futuro KDS; no se creó una aplicación KDS ni agente remoto. Consulta [la guía técnica y operativa](docs/KITCHEN_AND_PRINTING.md) y [la evidencia de verificación](docs/VERIFICATION.md).

## Catálogo y precios

En **Catálogo** administra categorías, productos independientes, presentaciones opcionales, ingredientes, complementos, porciones, grupos reutilizables, combos, listas de precios y promociones. Incluye imágenes opcionales, horarios, archivo lógico, búsqueda, filtros y paginación. La edición rechaza revisiones obsoletas de otra terminal.

En **Probar personalización** retira o sustituye todos los complementos, cambia cantidades, ingredientes, componentes y observaciones. Go valida y devuelve el desglose exacto; no se crea una venta. Las listas conservan la identidad del producto y las promociones explican su aplicación o exclusión. Los PNG originales locales cambian con el tema.

Consulta [docs/CATALOG.md](docs/CATALOG.md) para reglas y ejemplos reproducibles: filete $150 → $155 al cambiar arroz por papas y combo con papas $90 / $100 / $140. La migración de catálogo es `0003_catalog_and_pricing.sql`; la etapa 4 añade `0004_restaurant_operations.sql` y la etapa 5 `0005_kitchen_and_printing.sql`. Las migraciones anteriores se conservan. Antes de migrar una instalación existente, generar respaldo con `./scripts/dev.sh backup` y ensayar su restauración en una base temporal.

## Credencial reservada de soporte

La identidad `sa` está reservada y excluida de la administración ordinaria. Su contraseña la debe elegir el propietario en una terminal local:

```bash
./scripts/dev.sh provision-sa
```

El comando solicita y confirma el secreto sin eco, guarda solo Argon2id y audita la operación. No proporcionar contraseñas en argumentos, variables de entorno, chat o archivos versionados. Mientras el propietario no complete este paso, la identidad existe sin una contraseña utilizable. Un restablecimiento excepcional usa `./scripts/dev.sh reset-sa`, con confirmación local explícita y auditoría.

## Estructura

```text
ad-el-pos/
├── backend/
│   ├── cmd/ad-el-pos-server/     # Servidor, migraciones y mantenimiento local
│   ├── internal/administration/ # Identidad, permisos y recursos administrativos
│   ├── internal/catalog/       # Catálogo, disponibilidad y motor decimal único
│   ├── internal/config/        # Parámetros técnicos del servidor
│   ├── internal/database/      # Pool y migrador transaccional
│   ├── internal/kitchen/       # Preparación, documentos, transportes y trabajadores
│   ├── internal/events/        # Outbox, cursor y replay autenticado
│   ├── internal/operations/    # Mesas, cuentas, pedidos, unidades e historial
│   ├── internal/httpapi/       # HTTP, diagnósticos, CORS y errores
│   ├── internal/localsetup/    # Certificados locales
│   └── migrations/             # 0001 a 0005; inmutables después de aplicarse
├── desktop/src/
│   ├── components/             # Formularios, permisos y presentación reutilizables
│   ├── hooks/                  # Sesión, recursos y conectividad
│   ├── pages/                  # Administración y operación del restaurante
│   └── services/               # Contratos tipados y puente API
├── desktop/src-tauri/src/       # Rutas enumeradas, TLS y credenciales en Keychain
├── config/                     # Identidad canónica y ejemplos sin secretos
├── scripts/                    # Desarrollo, formato, procesos y PostgreSQL
├── tests/                      # Pruebas operativas macOS
├── ops/launchd/                # Plantilla; no instalada automáticamente
└── docs/                       # Convenciones, dependencias y evidencia
```

`.local`, `.tools`, cachés, respaldos y artefactos están excluidos del repositorio. Node/Rust son herramientas de construcción; el `.app` funciona sin Vite. PostgreSQL y Go son procesos independientes.

## Verificación

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
```

Las pruebas de integración crean y eliminan únicamente bases aleatorias `adel_pos_test_*`. No borran `adel_pos`. Consulta los resultados y límites reales en [docs/VERIFICATION.md](docs/VERIFICATION.md).

## Documentación

- [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md): preparación, ejecución, terminales, TLS y recuperación local.
- [ARCHITECTURE.md](ARCHITECTURE.md): responsabilidades y límites.
- [API_CONTRACTS.md](API_CONTRACTS.md): rutas y permisos.
- [docs/CATALOG.md](docs/CATALOG.md): reglas comerciales, uso y límites del catálogo.
- [docs/RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md): flujo de pedidos, unidades y concurrencia.
- [docs/CODING_CONVENTIONS.md](docs/CODING_CONVENTIONS.md): reglas permanentes de legibilidad.
- [NEXT_STEPS_FOR_SOL.md](NEXT_STEPS_FOR_SOL.md): continuidad y decisiones pendientes.

La distribución comercial sigue bloqueada hasta integrar licenciamiento. Firma/notarización Apple, servicio comercial persistente e instaladores Windows siguen pendientes. El bundle ID `local.adel.pos.dev` es provisional y no representa un dominio registrado.
