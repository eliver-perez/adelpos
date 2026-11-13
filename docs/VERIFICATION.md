# Verificación · Etapa 9F

Estado al **13 de noviembre de 2026**. Versión **0.9.5**, API v1, esquema **22** (migración
`0022_printing_and_kitchen_stations.sql`; etapa 9F: regresión en Mac, impresión, cocina por estaciones y pendientes).
Evidencia detallada: [VERIFICATION_STAGE9F.md](VERIFICATION_STAGE9F.md). Evidencia anterior:
[etapa 9E](VERIFICATION_STAGE9E.md), [etapa 9D](VERIFICATION_STAGE9D.md), [etapa 9C](VERIFICATION_STAGE9C.md),
[etapa 9B](VERIFICATION_STAGE9B.md), [etapa 9A](VERIFICATION_STAGE9A.md), [etapa 8](VERIFICATION_STAGE8.md),
[etapa 7.1](VERIFICATION_STAGE7_1.md), [etapa 7](VERIFICATION_STAGE7.md), [etapa 6.2](VERIFICATION_STAGE6_2.md),
[etapa 6.1](VERIFICATION_STAGE6_1.md), [etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md),
[etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y
[etapa 1](VERIFICATION_STAGE1.md). Avance: [ETAPA_9F_AVANCE.md](ETAPA_9F_AVANCE.md). Windows:
[NOTAS_WINDOWS.md](NOTAS_WINDOWS.md) («Etapa 9F: qué verificar en Windows»). Terminal de cocina:
[TERMINAL_DE_COCINA.md](TERMINAL_DE_COCINA.md).

## Estado

- **Código**: fases 0-R, 0, D, A, E, B y C y las correcciones de la revisión independiente en la rama `etapa-9f` (creada
  desde `main`, que contiene la 9E), cada una con sus pruebas según el avance; fase F con la regresión final, la
  documentación, la instalación y la fusión en `main`.
- **macOS**: la 0.9.4 de la 9E aprobó la regresión completa en la Mac (Fase 0-R) y se instaló en la instalación real el
  13/11/2026, sin migraciones. La 0.9.5 se desarrolló y se probó en la Mac en copias aisladas, con un clúster PostgreSQL
  temporal.
- **Instalación de la 0022 y la 0.9.5**: aplicadas a la instalación real el 13/11/2026 (respaldo `backups/adel_pos-20261113T211630Z-a3fceb.dump` con restauración verificada: 134 tablas idénticas; `migrate` `applied:1` y repetición `applied:0`; readiness 200 con esquema compatible; servidor y `.app` 0.9.5); detalle en
  [ETAPA_9F_AVANCE.md](ETAPA_9F_AVANCE.md), «Fase F».
- **Windows**: nada de la 9F se ejecutó en Windows; solo se compiló y se revisó Go con `GOOS=windows`.

## Qué está verificado

En la Mac, con Go (`-race`), PostgreSQL temporal, Vitest, Rust y Playwright (Chromium y WebKit) contra un Go temporal en
modo de pruebas, por fase (detalle y conteos en [VERIFICATION_STAGE9F.md](VERIFICATION_STAGE9F.md)):

- Modo de pruebas: Go no arranca con `test_mode` contra una base que no sea `adel_pos_test_*` y el puente de las pruebas
  se niega a correr sin él; «Quitar datos de prueba» con firmas exactas, vista previa, respaldo real con `pg_dump` antes
  de borrar, idempotencia, promociones intactas y datos con operaciones desactivados sin tocar la operación.
- Impresión: aviso inmediato sin impresora válida, reintento sin motivo y motivo al reimprimir, cola de impresión
  acotada y sin datos internos, corte total, parcial y sin corte con avance **en los bytes** (no en papel), nombre de la
  precuenta en botones y en el encabezado impreso y «Mesa» sin repetir.
- Cocina: estaciones por terminal y por usuario con su intersección y el aviso sin estaciones en común, combo repartido
  por estación y partida lista al completar sus partes, colores por tiempo y Preparación sin «Actualizar».
- Barra compacta a 1200, 1280, 1440, 1500 y 1920 px y leyenda del mapa sin desplazamiento horizontal; «Cancelar
  unidades» con y sin permiso, con «Autorizó» en los reportes; ruta `void` retirada (410).
- Migración 0022 de 21 a 22 con datos.

## No ejecutado o no verificado

Papel y corte (total, parcial y avance, con y sin cortador); la terminal de cocina con pantalla táctil real, el aviso
sonoro en un equipo real y dos terminales de cocina a la vez; una cocinera sin `kitchen.stations.read`; el menú nativo
real con el nombre de la precuenta; la ventana nativa; todo Windows (lista en [NOTAS_WINDOWS.md](NOTAS_WINDOWS.md)); y
el error intermitente de WebKit «The provided value is non-finite», de origen no identificado. Detalle en
[VERIFICATION_STAGE9F.md](VERIFICATION_STAGE9F.md), «No verificado».

## Regresión final

Ejecutada el 13/11/2026 por el coordinador con un script, sobre una copia exacta de `2b26439` (`git archive`), con el
clúster PostgreSQL temporal 55499, Go temporal en 8183 y vista previa en 1421; la pantalla del equipo estuvo
desbloqueada antes y después de Playwright. 1802 s.

| Suite                                    | Resultado                                                                                                                                                                                         |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato (Prettier, `gofmt`, `cargo fmt`) | aprobado                                                                                                                                                                                          |
| Metadatos (`sync-metadata --check`)      | aprobado                                                                                                                                                                                          |
| `go vet` (normal y `-tags=integration`)  | aprobado                                                                                                                                                                                          |
| `GOOS=windows go build ./...` y `go vet` | aprobado                                                                                                                                                                                          |
| Go unitarias con `-race`                 | 24 paquetes `ok` (con `maintenance` y `stationscope`, nuevos)                                                                                                                                     |
| Integración PostgreSQL con `-race`       | 9 de 9: `database` 8.2 s, `administration` 84.5 s, `catalog` 8.9 s, `operations` 281.9 s, `payments` 2.4 s, `reports` 121.0 s, `reports/operational` 40.4 s, `designer` 20.2 s, `identity` 17.8 s |
| `tsc --noEmit`                           | sin errores                                                                                                                                                                                       |
| Vitest (`npm test`)                      | 86 archivos, 821 pruebas aprobadas                                                                                                                                                                |
| `npm run build`                          | correcto (aviso de tamaño de fragmentos ya conocido)                                                                                                                                              |
| `cargo test --locked --offline`          | 45 aprobadas, 1 ignorada                                                                                                                                                                          |
| Playwright Chromium                      | 24 de 24 (8.7 min)                                                                                                                                                                                |
| Playwright WebKit                        | 24 de 24 (8.8 min)                                                                                                                                                                                |
| `.app` (`tauri build --bundles app`)     | 0.9.5, Mach-O arm64, 13.56 MiB                                                                                                                                                                    |

Sin fallos ni repeticiones; el error intermitente de WebKit de la Fase 0-R no apareció. **Rust para Windows**: el
destino `x86_64-pc-windows-msvc` no está instalado en la Mac (y `cargo check` para MSVC exige además el compilador de
recursos y un compilador C de Windows), así que se revisó a mano: ningún cambio de Rust de la 9F (`api.rs`, `menu.rs`,
`server_host.rs` y la versión) está dentro de bloques condicionales por plataforma, de modo que el mismo código compila
en ambos sistemas (`menu_set_state` sigue registrado en Windows, donde el menú nativo se omite). Las mediciones de
volumen y la impresión física no corren por omisión (por diseño).

## Repetir las comprobaciones

En la Mac:

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
./scripts/dev.sh build-macos
```

`test-integration` y `test-browser` usan bases desechables `adel_pos_test_*` dentro del clúster del proyecto; para
aislamiento total, el procedimiento de `CLAUDE.md` §6 (con `test_mode: true` en la configuración del Go temporal). En
Windows: [VERIFICATION_STAGE9E.md](VERIFICATION_STAGE9E.md), «Repetir las comprobaciones», y la lista de la 9F en
[NOTAS_WINDOWS.md](NOTAS_WINDOWS.md).
