# Verificación · Etapa 9E

Estado al **12 de noviembre de 2026**. Versión **0.9.4**, API v1, esquema **21** (sin migraciones nuevas; etapa 9E:
compilación, pruebas e instalador en Windows 11). Evidencia detallada: [VERIFICATION_STAGE9E.md](VERIFICATION_STAGE9E.md).
Evidencia anterior: [etapa 9D](VERIFICATION_STAGE9D.md), [etapa 9C](VERIFICATION_STAGE9C.md),
[etapa 9B](VERIFICATION_STAGE9B.md), [etapa 9A](VERIFICATION_STAGE9A.md), [etapa 8](VERIFICATION_STAGE8.md),
[etapa 7.1](VERIFICATION_STAGE7_1.md), [etapa 7](VERIFICATION_STAGE7.md), [etapa 6.2](VERIFICATION_STAGE6_2.md),
[etapa 6.1](VERIFICATION_STAGE6_1.md), [etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md),
[etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y
[etapa 1](VERIFICATION_STAGE1.md). Avance: [ETAPA_9E_AVANCE.md](ETAPA_9E_AVANCE.md). Windows:
[NOTAS_WINDOWS.md](NOTAS_WINDOWS.md), [../DEVELOPMENT_WINDOWS.md](../DEVELOPMENT_WINDOWS.md) e
[INSTALAR_EN_WINDOWS.md](INSTALAR_EN_WINDOWS.md).

## Estado

- **Código**: fases 0, A, B, C (partes 1 y 2), D y E en la rama `etapa-9e-windows` (creada desde `main`, que contiene la
  9D), cada una con sus pruebas según el avance; fase F con la regresión completa en Windows, la documentación y la
  fusión en `main`.
- **Windows 11 Pro de 64 bits**: compila, pasa todas las suites (Go sin `-race`) y tiene un instalador NSIS de prueba
  (36.7 MB, sin firma) **instalado y verificado** en el equipo de pruebas, con un establecimiento de prueba.
- **macOS**: solo se comprobó la compilación desde Windows (`GOOS=darwin` para Go y `cargo check` hacia
  `aarch64-apple-darwin`). **La 0.9.4 no se probó ni se instaló en la Mac**, cuya instalación real sigue en la 0.9.3 y
  el esquema 21: la próxima sesión allí debe ejecutar la regresión completa antes de instalarla.

## Qué está verificado

En Windows 11, con PostgreSQL 18.6 real, Vitest, Rust, Playwright (Chromium y WebKit) contra un Go temporal y, sobre la
aplicación **instalada**, con PowerShell y Playwright por CDP:

- Instalación en silencio, primer arranque (clúster y 21 migraciones), sin ventanas de consola, prueba de humo de la
  operación completa con exportación PDF y Excel, persistencia, respaldo y restauración, actualización encima con la
  aplicación abierta, desinstalación con datos conservados, reinstalación con los mismos datos y un usuario con
  acentos simulado.
- Arranque con datos (instalador definitivo): ventana en 2.4 s y servidor en 2.5 s; memoria ≈ 745 MB de conjunto de
  trabajo (≈ 388 MB privados); cierre ordenado en 1.1 s sin procesos ni `postmaster.pid`.
- La regresión encontró y corrigió carreras de foco y de atajos de la interfaz (también aplican a macOS) y dos
  problemas de las pruebas; detalle en [VERIFICATION_STAGE9E.md](VERIFICATION_STAGE9E.md).
- Impresión por la cola de Windows: solo con el administrador de impresión simulado y consultas de lectura reales
  (la única impresora del equipo, virtual, se rechaza). ESC/POS por red, solo con pruebas de loopback.

## No ejecutado o no verificado

La regresión en la Mac; Windows 10; pantalla táctil; impresoras de red y USB con papel; cajón; dos terminales (LAN
no preparada en Windows); una cuenta de Windows con acentos real; `provision-sa`/`reset-sa` en Windows. Lista para el
propietario en [INSTALAR_EN_WINDOWS.md](INSTALAR_EN_WINDOWS.md), «Qué debes probar a mano».

## Regresión final

| Suite                                            | Resultado                                          |
| ------------------------------------------------ | -------------------------------------------------- |
| `dev.ps1 test`                                   | aprobada (Vitest 757/757, Rust 46)                 |
| `go test -count=1 ./...`                         | 25 paquetes aprobados                              |
| `dev.ps1 test-integration`                       | 9 paquetes aprobados                               |
| Instalación de Windows con PostgreSQL real       | aprobada (`standalone` y respaldo antes de migrar) |
| Administrador de credenciales real               | aprobada                                           |
| Playwright Chromium                              | 19 de 19 (código final)                            |
| Playwright WebKit                                | 19 de 19 (tras corregir lo que encontró)           |
| `build-windows.ps1`                              | instalador 0.9.4 generado                          |
| Compilación para macOS desde Windows (Go y Rust) | aprobada, sin avisos                               |

## Repetir las comprobaciones

En Windows: [VERIFICATION_STAGE9E.md](VERIFICATION_STAGE9E.md), «Repetir las comprobaciones». En la Mac:

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
./scripts/dev.sh build-macos
```
