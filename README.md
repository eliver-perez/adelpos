# AD-EL POS · Etapa 1

Cimientos ejecutables de un POS para restaurantes. Monolito modular Go, PostgreSQL y una aplicación completa React/TypeScript dentro de Tauri 2. La entrega contiene diagnósticos reales y configuración de instalación; no contiene módulos de venta, cuentas ni datos comerciales simulados.

## Inicio en este Mac

Desde una terminal:

```bash
cd "/Users/eliver/Documents/ADEL POS/ad-el-pos"
./scripts/dev.sh db-start
./scripts/dev.sh migrate
./scripts/dev.sh server-start
./scripts/dev.sh desktop
```

`server-start` crea un proceso independiente y registra su PID. Cerrar Tauri no lo detiene. Para detenerlo explícitamente: `./scripts/dev.sh server-stop`. PostgreSQL se detiene por separado con `./scripts/dev.sh db-stop`. No se instaló un servicio al iniciar sesión.

Para la primera preparación de una copia nueva, consulta [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md): requisitos → `install` → `db-init` → `migrate` → `server-start` → `desktop`. Se necesita Internet para descargar herramientas/dependencias; una vez instaladas, la ejecución local no requiere Internet.

## Estructura

```text
ad-el-pos/
├── backend/
│   ├── cmd/ad-el-pos-server/    # Entrada CLI: serve, migrate, check-config, version
│   ├── internal/
│   │   ├── config/             # JSON estricto y validación
│   │   ├── database/           # pgxpool, salud y migrador
│   │   ├── httpapi/            # API técnica v1, CORS y errores
│   │   ├── licensing/          # Único límite de licencias
│   │   ├── product/            # Metadatos generados y embebidos
│   │   └── events/             # Contrato reservado, transporte pendiente
│   └── migrations/            # SQL versionado embebido en el binario
├── desktop/
│   ├── src/                   # React: componentes, hooks, servicios, contratos
│   └── src-tauri/             # Ventana nativa y puente de diagnóstico limitado
├── config/                    # Metadatos canónicos y ejemplos sin secretos
├── scripts/                   # Desarrollo, DB, respaldo y procesos
├── ops/launchd/               # Plantilla de servicio; no instalada
├── assets/                    # Icono vectorial original
├── tests/                     # Verificaciones operativas reproducibles
├── docs/                      # Fuentes y evidencia de verificación
├── .local/                    # PRIVADO/ignorado: clúster, secretos, PID y logs
├── .tools/                    # Ignorado: toolchains locales de este Mac
└── bin/                       # Ignorado: ad-el-pos-server
```

Las pruebas unitarias viven junto al código. Las de PostgreSQL usan una base temporal dedicada y nunca limpian `adel_pos`.

## Comandos de verificación

```bash
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh build-server
./scripts/dev.sh build-macos
curl --fail http://127.0.0.1:8181/api/v1/health/ready
```

Versiones fijadas en `go.mod`/`go.sum`, `package-lock.json` y `Cargo.lock`. Revisa [docs/VERIFICATION.md](docs/VERIFICATION.md) para las comprobaciones realmente ejecutadas y [docs/DEPENDENCIES.md](docs/DEPENDENCIES.md) para versiones y fuentes oficiales.

## Documentación de continuidad

- [ARCHITECTURE.md](ARCHITECTURE.md): responsabilidades, decisiones y límites.
- [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md): ejecución, configuración y solución de problemas.
- [API_CONTRACTS.md](API_CONTRACTS.md): API implementada y futuras extensiones.
- [NEXT_STEPS_FOR_SOL.md](NEXT_STEPS_FOR_SOL.md): estado y siguiente tarea.

Esta es una compilación de desarrollo. El perfil `production` falla de forma explícita hasta integrar licenciamiento y controles comerciales. El nombre y el identificador `local.adel.pos.dev` son provisionales; no representan un dominio registrado.
