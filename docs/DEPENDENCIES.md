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

Go estándar cubre HTTP, JSON, logs, configuración, señales y hashing. Las dependencias directas del backend son pgx, decimal, x/crypto, x/term, x/text y, desde la etapa 9A, excelize, fpdf y x/image (fuentes), detalladas por etapa a continuación. No hay Redis, broker, Docker, ORM, servicio cloud ni framework Go adicional. Rust y paquetes npm son herramientas de compilación; no deben instalarse en cada terminal comercial. Python 3 y Bash se usan únicamente en scripts de desarrollo macOS.

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

## Incorporaciones de la etapa 7.1

- `sonner` **2.0.8** (licencia MIT), dependencia de ejecución del escritorio fijada de forma exacta en `desktop/package.json` y `desktop/package-lock.json` (integridad SHA-512 del registro npm). Motivo: el sistema único de mensajes de la etapa 7.1 (avisos de éxito que se cierran solos, errores persistentes y acción opcional); la especificación pide Sonner adaptado a los tokens visuales. No tiene dependencias propias; sus dependencias de pares son `react` y `react-dom` ^18 o ^19 (`@types/react` opcional), satisfechas por React 19.3.1. Se usa en modo sin estilos propios: cada aviso es un componente de la aplicación (`components/AppToast.tsx`) con los tokens de `styles.css` en tema claro y oscuro. Sonner inserta en tiempo de ejecución su hoja de estilos de posicionamiento y animación (sin recursos de red). [Repositorio oficial](https://github.com/emilkowalski/sonner) y [documentación](https://sonner.emilkowal.ski/).
- Sin librería de iconos: los iconos de los botones amplían el conjunto propio `components/Icon.tsx` (SVG de 24 × 24 con trazo 1.6).
- Backend: sin dependencias nuevas. Los paquetes `internal/moneytext` (formato de importes impresos) e `internal/plural` (concordancia de contadores) usan solo la biblioteca estándar y `shopspring/decimal`, ya fijado.

## Incorporaciones de la etapa 9A

Descarga única con red (10/11/2026) a las cachés del proyecto (`.cache/go-mod` y `.tools/cargo/registry`); después
`go build`/`go test` con `GOPROXY=off -mod=readonly` y `cargo test --locked --offline` funcionan sin red. Todas las
versiones se publicaron hace más de dos semanas (corte: 27/10/2026).

### Backend (Go), fijadas en `backend/go.mod` y `backend/go.sum`

- `github.com/xuri/excelize/v2` **2.11.0** (BSD-3-Clause; 06/07/2026): exportación `.xlsx` real generada en Go con la
  misma consulta que la pantalla (escritura por flujo, estilos y formatos numéricos, panel congelado); sin red y sin
  cgo. No se usa v2.11.1 (02/11/2026, demasiado reciente). Indirectas: `github.com/richardlehane/mscfb` 1.0.7 y
  `github.com/richardlehane/msoleps` 1.0.6 (Apache-2.0), `github.com/tiendc/go-deepcopy` 1.7.2 (MIT),
  `github.com/xuri/efp` 0.0.1 y `github.com/xuri/nfp` 0.0.2-0.20250530014748-2ddeb826f9a9 (BSD-3-Clause),
  `golang.org/x/net` 0.60.0 (BSD-3-Clause). [Documentación](https://xuri.me/excelize/).
- `codeberg.org/go-pdf/fpdf` **0.12.0** (MIT; 18/05/2026): PDF de reportes con fuentes UTF-8 incrustadas (Type0 /
  Identity-H con `/ToUnicode`: el texto se puede copiar y buscar). El paquete usado solo importa la biblioteca
  estándar. Se usa la ruta de Codeberg porque `github.com/go-pdf/fpdf` está archivado.
  [Repositorio](https://codeberg.org/go-pdf/fpdf).
- `golang.org/x/image` **0.47.0** (BSD-3-Clause; 09/10/2026): solo las fuentes Go Regular y Go Bold
  (`font/gofont/goregular`, `font/gofont/gobold`), compiladas en el binario como bytes (sin archivos externos); cubren
  acentos, «ñ», «¿¡», ««»», «—», «…» y tienen dígitos tabulares. Fuentes de Bigelow & Holmes con la licencia del
  proyecto Go (su aviso de copyright se reproduce abajo, en «Aviso de licencia de las fuentes Go»). No incluyen ideogramas CJK ni emojis (en el PDF se
  verían como un recuadro; el Excel los conserva). [Go fonts](https://go.dev/blog/go-fonts).
- `go mod tidy` además movió `golang.org/x/text` 0.43.0 (ya presente) de indirecta a directa: `internal/kitchen` ya la
  importaba. Ninguna otra versión cambió (MVS conserva `x/crypto` 0.58.0, `x/sys` 0.49.0, `x/text` 0.43.0).
- Hashes `h1:` principales: `codeberg.org/go-pdf/fpdf v0.12.0 h1:g8E/1VqGqB2lZUUaqQrrTnA0IEJLPTTX1DZ0qS/ZmhU=`,
  `github.com/xuri/excelize/v2 v2.11.0 h1:HxaEFl6sRN2+8J5a8HaKq+0M4FsjBGMnWWtjOCPSG88=`,
  `golang.org/x/image v0.47.0 h1:za/8gVz++PPZqmZJBNSISICN5Rrj5zixP/8K++fgmqo=`.

#### Aviso de licencia de las fuentes Go

Los PDF de reportes incrustan las fuentes Go Regular y Go Bold (`golang.org/x/image/font/gofont`), compiladas en el servidor. Su licencia exige reproducir este aviso en la documentación de la distribución binaria (texto original de `font/gofont/ttfs/README` en `golang.org/x/image` v0.47.0):

```text
Copyright (c) 2016 Bigelow & Holmes Inc.. All rights reserved.

Distribution of this font is governed by the following license. If you do not
agree to this license, including the disclaimer, do not distribute or modify
this font.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

	* Redistributions of source code must retain the above copyright notice,
	  this list of conditions and the following disclaimer.

	* Redistributions in binary form must reproduce the above copyright notice,
	  this list of conditions and the following disclaimer in the documentation
	  and/or other materials provided with the distribution.

	* Neither the name of Google Inc. nor the names of its contributors may be
	  used to endorse or promote products derived from this software without
	  specific prior written permission.

DISCLAIMER: THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO,
THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

### Escritorio (Rust), fijadas en `desktop/src-tauri/Cargo.toml` (`=2.8.3`) y `Cargo.lock`

- `tauri-plugin-dialog` **2.8.3** (Apache-2.0 OR MIT; 14/10/2026; 2.8.4 es demasiado reciente): diálogo nativo
  «Guardar como» usado **solo desde Rust** en la operación cerrada `export_report` (`src/export.rs`). La ventana no
  tiene permisos `dialog:*` ni `fs:*` (`capabilities/main.json` sin cambios; prueba Rust). El plugin inyecta un script
  que redirige `window.alert`/`window.confirm` a su comando `message`, que la ACL rechaza: la aplicación no los usa
  (`ConfirmDialog`). Arrastra `tauri-plugin-fs` **2.7.1**, `tauri-plugin` **2.7.2** (Apache-2.0 OR MIT; 14/10/2026) y
  `rfd` **0.16.1** (MIT; 22/10/2026). `tauri-plugin-fs` se compila pero no se registra: Rust escribe con `std::fs`.
  `Cargo.lock` solo agrega esos cuatro paquetes. [Documentación](https://v2.tauri.app/plugin/dialog/).

Sin dependencias npm nuevas. La interfaz de Reportes dibuja sus gráficos con componentes SVG propios (`desktop/src/components/reports/`), sin librería de gráficos.
