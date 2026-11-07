# Versiones y fuentes

Verificado durante la entrega del 7 de noviembre de 2026. Se consultó documentación oficial antes de integrar las dependencias. Las versiones efectivamente instaladas están fijadas por lockfiles; no se usa `latest` en los comandos diarios.

| Componente             | Versión usada            | Motivo/fuente primaria                                                                                                                                                          |
| ---------------------- | ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go                     | 1.27.3 (`go.mod` 1.27.0) | Binario estable ARM64 con SHA-256 verificado; [descargas](https://go.dev/dl/).                                                                                                  |
| PostgreSQL             | 18.6                     | Versión estable soportada, [política](https://www.postgresql.org/support/versioning/) y [initdb](https://www.postgresql.org/docs/18/app-initdb.html).                           |
| pgx                    | 5.12.0                   | Driver y pool nativo Go; [pgxpool](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool).                                                                                         |
| Node                   | 24.22.2                  | Rama LTS compatible con Vite, archivo oficial con SHA-256; [versiones](https://nodejs.org/en/about/previous-releases).                                                          |
| React / ReactDOM       | 19.3.1                   | Rama estable documentada; [versiones React](https://react.dev/versions).                                                                                                        |
| TypeScript             | 5.9.3                    | Versión estable compatible con el tooling seleccionado; [documentación 5.9](https://www.typescriptlang.org/docs/handbook/release-notes/typescript-5-9.html).                    |
| Vite / plugin-react    | 8.3.6 / 6.2.1            | Bundler de frontend; [guía](https://vite.dev/guide/).                                                                                                                           |
| Tailwind / plugin Vite | 4.3.5                    | Estilos locales y temas CSS; [integración oficial](https://tailwindcss.com/docs/installation/using-vite).                                                                       |
| Tauri Rust             | 2.12.5                   | Ventana y IPC; [requisitos](https://v2.tauri.app/start/prerequisites/), [capabilities](https://v2.tauri.app/security/capabilities/), [CSP](https://v2.tauri.app/security/csp/). |
| Tauri CLI / JS API     | 2.12.4                   | Herramientas compatibles de la misma línea v2; [configuración](https://v2.tauri.app/reference/config/).                                                                         |
| Rust                   | 1.99.1                   | Toolchain estable mínimo de Rustup, local al proyecto.                                                                                                                          |
| reqwest                | 0.12.28                  | HTTP nativo restringido para destinos configurables sin abrir CSP; TLS Rustls, sin proxy automático; [documentación](https://docs.rs/reqwest/0.12.28/reqwest/).                 |
| Vitest                 | 5.1.0                    | Pruebas del contrato/servicio frontend; [guía](https://vitest.dev/guide/).                                                                                                      |

Go estándar cubre HTTP, JSON, logs, configuración, señales y hashing. Las dependencias directas del backend son pgx, decimal y x/crypto/x/term, detalladas por etapa a continuación. No hay Redis, broker, Docker, ORM, servicio cloud ni framework Go adicional. Rust y paquetes npm son herramientas de compilación; no deben instalarse en cada terminal comercial. Python 3 y Bash se usan únicamente en scripts de desarrollo macOS.

Otras referencias: [pg_dump y respaldo](https://www.postgresql.org/docs/18/backup-dump.html), [bundle macOS Tauri](https://v2.tauri.app/distribute/macos-application-bundle/), [launchd de Apple](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html).

## Incorporaciones de la etapa 2

- Prettier 3.9.10, fijado localmente; [instalación oficial](https://prettier.io/docs/install) y [API](https://prettier.io/docs/api).
- golang.org/x/crypto 0.58.0: Argon2id; [documentación oficial](https://pkg.go.dev/golang.org/x/crypto/argon2).
- golang.org/x/term 0.47.0: entrada oculta local; [documentación](https://pkg.go.dev/golang.org/x/term).
- keyring 3.6.3: almacén seguro nativo; [documentación de la versión utilizada](https://docs.rs/keyring/3.6.3/keyring/). No es una dependencia de red en ejecución.
- @testing-library/react 16.3.3 y jsdom 31.0.0: pruebas de componentes, solo desarrollo.
- Rustfmt y Black 25.11.0 se utilizaron para expandir/formatear código Rust/Python existente. Black está en .tools/python y no es dependencia de ejecución; Rustfmt se incluye en la comprobación de formato.

La política de sesiones se contrastó con [OWASP](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html); TLS usa la [biblioteca estándar Go](https://pkg.go.dev/crypto/tls). Go/Rust siguen con lockfiles; no se incorporó ORM, servicio cloud, microservicio o broker.

## Incorporaciones de la etapa 3

- `github.com/shopspring/decimal` **1.6.0**, fijado en go.mod/go.sum: aritmética decimal exacta y redondeo explícito; [documentación primaria](https://pkg.go.dev/github.com/shopspring/decimal).
- Persistencia monetaria [numeric de PostgreSQL](https://www.postgresql.org/docs/18/datatype-numeric.html); cadenas en JSON. No se añadió biblioteca financiera al frontend.
- Validación de PNG/JPEG mediante [image de Go](https://pkg.go.dev/image) y decodificadores estándar. Los logos originales se integran como [assets locales de Vite](https://vite.dev/guide/assets).

No se añadieron dependencias npm ni servicios externos para el catálogo. Se conservan los lockfiles y las versiones existentes.
