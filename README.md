# AD-EL POS · Etapa 3

Aplicación de escritorio para restaurantes: monolito modular Go, PostgreSQL y React/TypeScript/Tailwind dentro de Tauri 2. Conserva configuración del establecimiento, usuarios, roles, permisos, sesiones, terminales, cajas físicas y auditoría. Añade catálogo, ingredientes, personalizaciones, combos, listas y promociones con un motor decimal exacto en Go. No contiene ventas ni movimientos de inventario.

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

## Catálogo y precios

En **Catálogo** administra categorías, productos independientes, presentaciones opcionales, ingredientes, complementos, porciones, grupos reutilizables, combos, listas de precios y promociones. Incluye imágenes opcionales, horarios, archivo lógico, búsqueda, filtros y paginación. La edición rechaza revisiones obsoletas de otra terminal.

En **Probar personalización** retira o sustituye todos los complementos, cambia cantidades, ingredientes, componentes y observaciones. Go valida y devuelve el desglose exacto; no se crea una venta. Las listas conservan la identidad del producto y las promociones explican su aplicación o exclusión. Los PNG originales locales cambian con el tema.

Consulta [docs/CATALOG.md](docs/CATALOG.md) para reglas y ejemplos reproducibles: filete $150 → $155 al cambiar arroz por papas y combo con papas $90 / $100 / $140. La migración nueva es `0003_catalog_and_pricing.sql`; 0001 y 0002 se conservan. Antes de migrar una instalación existente, generar respaldo con `./scripts/dev.sh backup` y ensayar su restauración en una base temporal.

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
│   ├── internal/httpapi/       # HTTP, diagnósticos, CORS y errores
│   ├── internal/localsetup/    # Certificados locales
│   └── migrations/             # 0001, 0002 y 0003; inmutables después de aplicarse
├── desktop/src/
│   ├── components/             # Formularios, permisos y presentación reutilizables
│   ├── hooks/                  # Sesión, recursos y conectividad
│   ├── pages/                  # Pantallas administrativas reales
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
```

Las pruebas de integración crean y eliminan únicamente bases aleatorias `adel_pos_test_*`. No borran `adel_pos`. Consulta los resultados y límites reales en [docs/VERIFICATION.md](docs/VERIFICATION.md).

## Documentación

- [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md): preparación, ejecución, terminales, TLS y recuperación local.
- [ARCHITECTURE.md](ARCHITECTURE.md): responsabilidades y límites.
- [API_CONTRACTS.md](API_CONTRACTS.md): rutas y permisos.
- [docs/CATALOG.md](docs/CATALOG.md): reglas comerciales, uso y límites del catálogo.
- [docs/CODING_CONVENTIONS.md](docs/CODING_CONVENTIONS.md): reglas permanentes de legibilidad.
- [NEXT_STEPS_FOR_SOL.md](NEXT_STEPS_FOR_SOL.md): continuidad y decisiones pendientes.

La distribución comercial sigue bloqueada hasta integrar licenciamiento. Firma/notarización Apple, servicio comercial persistente e instaladores Windows siguen pendientes. El bundle ID `local.adel.pos.dev` es provisional y no representa un dominio registrado.
