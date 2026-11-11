# Verificación · Etapa 9B

Verificación por fases del **11 de noviembre de 2026**, zona America/Monterrey, antes de cada commit local de la rama
`etapa-9b`: fase 0 `fd1de81`, fase A `9a4884d`, fase B (partes B1 y B2) `5fe33a2` y fase C `f3509d3`; la fase D agrega la
versión, la documentación, la revisión independiente y sus correcciones. Versión **0.9.1**, API v1, esquema **19**
(migraciones `0018_document_templates.sql` y `0019_document_identity.sql`). Evidencia anterior:
[etapa 9A](VERIFICATION_STAGE9A.md), [etapa 8](VERIFICATION_STAGE8.md), [etapa 7.1](VERIFICATION_STAGE7_1.md),
[etapa 7](VERIFICATION_STAGE7.md), [etapa 6.2](VERIFICATION_STAGE6_2.md), [etapa 6.1](VERIFICATION_STAGE6_1.md),
[etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md), [etapa 4](VERIFICATION_STAGE4.md),
[etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y [etapa 1](VERIFICATION_STAGE1.md). Avance:
[ETAPA_9B_AVANCE.md](ETAPA_9B_AVANCE.md). Guía: [DOCUMENT_DESIGNER.md](DOCUMENT_DESIGNER.md).

**Alcance de esta evidencia.** Las cifras son las que cada fase registró en el avance y, para los cierres de fase, en sus
registros (en el directorio temporal de la sesión). Hasta la fase C se obtuvieron con `config/product.json` todavía en
0.9.0 y esquema 17 (las migraciones nuevas ya estaban embebidas y el migrador aplicaba 19); la versión 0.9.1 y el esquema
19 se fijaron en la fase D. La regresión final de la fase D se registra al final de este documento y en
[VERIFICATION.md](VERIFICATION.md).

**Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432) ; las migraciones 0018
y 0019 se aplicaron después, con autorización del propietario (ver «Instalación real»). No se leyeron secretos y
ninguna prueba envió trabajos a CUPS real, a una impresora, a un cajón ni a una conexión TCP fuera de loopback.

## Entorno aislado

Copias del repositorio en el directorio temporal de la sesión, una por fase o parte (sin `.git`, `.local`, `.tools`,
cachés, artefactos ni las especificaciones del propietario), clúster PostgreSQL temporal en el puerto 55499 (solo
loopback), una base `adel_pos_test_*` desechable por paquete, `GOPROXY=off` y Go temporal para Playwright en puertos
propios de cada parte (8186/1424 en la fase C), según `CLAUDE.md` §6. Las fases A y B1 se desarrollaron en paralelo; B1 se
verificó además sobre `HEAD` más solo sus archivos. Cada cierre de fase se ejecutó sobre una copia exacta del commit
(`git archive`).

**Transportes simulados, no hardware.** Las pruebas de impresión usan el transporte controlado de la cola, un puerto TCP
de loopback que registra los bytes ESC/POS y un ejecutor simulado de `lpstat`, `lpoptions` y `lp` que registra los
argumentos y el archivo sin ejecutar nada. Comprueban bytes y argumentos, **no papel**.

## Resultados ejecutados por fase

| Fase (commit)         | Estáticas                                                         | Go unitarias (`-race`)                                                                             | Integración PostgreSQL (`-race`)                                                                                                                            | Vitest                                                                         | `tsc` y build           | Rust (`cargo test`)      | Playwright (Chromium y WebKit)                                                   |
| --------------------- | ----------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ | ----------------------- | ------------------------ | -------------------------------------------------------------------------------- |
| 0 (`fd1de81`)         | —                                                                 | —                                                                                                  | —                                                                                                                                                           | —                                                                              | —                       | —                        | —                                                                                |
| A, agente             | `go vet` con y sin `integration`                                  | `ok`: documents, kitchen, payments, designer, administration, database, operations                 | `designer` 17.8 s, `database` 6.3 s, `administration` 55.6 s, `payments` 2.2 s, `operations` 230.7 s: `ok`                                                  | `services/documents.test.ts` y `api.test.ts` (7)                               | —                       | 32 aprobadas, 1 ignorada | —                                                                                |
| A, cierre (`9a4884d`) | Prettier sin `docs/etapas/`, gofmt, `cargo fmt --check`, `go vet` | `ok`: documents, designer, kitchen, payments, administration, database                             | `designer` 17.2 s, `database` 5.8 s, `administration` 54.6 s, `payments` 2.1 s, `operations` 230.9 s: `ok`                                                  | `src/services`: 90 en 7 archivos                                               | `tsc` sin errores       | 32 y 1                   | —                                                                                |
| B1, agente            | `gofmt -l`, `go vet` con y sin `integration`                      | `ok`: logoraster, brand, reports                                                                   | `reports` 93 s, `reports/operational` 40 s: `ok`                                                                                                            | —                                                                              | —                       | —                        | —                                                                                |
| B2, agente            | `gofmt -l`, `go vet` con y sin `integration`                      | `ok`: identity, logoraster, brand, documents, designer, kitchen, administration, reports, payments | `identity` 16 s, `designer` 18 s, `database` 6 s, `reports` 95 s, `administration` 56 s, `operations` 226 s: `ok`                                           | `identityPanel.test.tsx` 11, `useRealtime.test.tsx` 4, `src/services` 105 en 9 | Sin errores; build `ok` | 33 y 1                   | Especificación de identidad escrita, no ejecutada (faltaba la pantalla de C)     |
| B, cierre (`5fe33a2`) | Prettier sin `docs/etapas/`, gofmt, `cargo fmt --check`, `go vet` | `ok`: identity, logoraster, brand, documents, designer, kitchen, reports, administration, payments | `identity` 15.9 s, `designer` 17.5 s, `database` 6.0 s, `reports` 91.8 s, `reports/operational` 40.2 s, `administration` 54.2 s, `operations` 226.3 s: `ok` | `src/services` 105 en 9; identidad y `useRealtime` 15 en 2                     | `tsc` sin errores       | 33 y 1                   | —                                                                                |
| C, agente             | —                                                                 | No aplica (sin cambios de backend)                                                                 | No aplica                                                                                                                                                   | 529 en 60 archivos (39 nuevas)                                                 | Sin errores; build `ok` | —                        | Diseñador, identidad, `restaurant` y `stock-consumption`: 1 de 1 por motor       |
| C, cierre (`f3509d3`) | Prettier sin `docs/etapas/`, gofmt, `cargo fmt --check`           | No aplica                                                                                          | No aplica                                                                                                                                                   | 529 en 60                                                                      | Sin errores; build `ok` | —                        | Completo: 3 aprobadas y 9 fallidas por motor (ver abajo); corregido en la fase D |

«—»: la fase no registró esa suite (no cambiaba esa capa o la cubre otra fase).

- **A**: equivalencia byte a byte contra copias literales de las plantillas anteriores: 6 144 comandas y avisos (4 tipos
  de ticket × 128 combinaciones de opciones de Cocina × 6 documentos × 2 anchos, incluida la prueba técnica), 168
  precuentas y tickets y más de 1 000 comprobantes y cortes, a 58 y 80 mm; bytes ESC/POS y argumentos de `lp` del camino
  plano sin cambios. La integración de `operations` incluyó la cadena de aceptación completa (etapas 4–9A) y las
  aceptaciones de la 9B.
- **B1**: se verificó sobre `HEAD` más solo sus archivos (la fase A estaba a medias en el árbol) y antes en su copia
  sincronizada. `TestReportExports` (63 s) incluyó el subtest «encabezado con el logotipo del restaurante o el de AD-EL
  POS»; comparado con el código de la 9A, las cadenas del PDF de 5 renglones y la hoja «Datos» del Excel de 40 renglones
  son idénticas. Exportación de 50 000 renglones: PDF de 4.7 MB en 10.4 s y XLSX de 2.7 MB en 7.1 s (con `-race`).
- **B2**: `TestDocumentIdentity` con 14 subpruebas (lectura combinada, textos con revisión, idempotencia y conflicto, PNG
  y JPEG con conversiones de 58 y 80 mm y vistas previas, rechazos de SVG, GIF, PNG truncado, más de 1 MiB, 2049 px, vacío
  y base64 inválido, reemplazo conservando el anterior, reutilización de conversiones, fuente de reportes en transacción
  de solo lectura sin locks, retiro, inmutabilidad, dos administradores y mismo `operation_id` concurrente, permisos y
  `access.denied` por HTTP, carga mayor de 64 KiB con auditoría sin bytes, evento SSE solo para
  `documents.templates.read`, PDF y Excel con logotipo y con el respaldo, y conexión con el renderizador: muestra por
  ESC/POS a un puerto de loopback con `GS v 0` y los bits guardados, y copias idénticas tras reemplazar y retirar el
  logotipo). En `kitchen`, `TestRealLogoThroughCUPSQueues`: logotipo real por CUPS raw con `GS v 0` y por CUPS sin raw con
  el texto, aviso y `submitted` (ejecutor simulado). En `operations`, `testStage9BLogo`: comanda real con logotipo, IR con
  la conversión de 80 mm, bits cargados por el trabajador y reimpresión idéntica tras reemplazar y retirar.
- **C, agente**: Playwright en su copia (puertos 8186/1424): `workflow-stage9b-designer.spec.ts` (entonces
  `documents-designer.spec.ts`; Chromium 15.3 s, WebKit 14.7 s), `workflow-stage9b-identity.spec.ts` (entonces
  `documents-identity.spec.ts`; 4.2 s y 3.7 s), `restaurant.spec.ts` (55.7 s y 55.0 s) y `stock-consumption.spec.ts`
  (27.9 s y 26.7 s): 1 de 1 cada una en cada motor, ejecutadas por separado.
- **C, cierre**: la corrida **completa** de Playwright sobre la copia exacta de `f3509d3` dio, en cada motor, 3 aprobadas
  y 9 fallidas (`restaurant`, `stock-consumption`, `table-price-decrease`, `takeaway-quick-notes`, `vendor-purchases`,
  `warehouse-control`, `workflow-inventory-9a`, `workflow-reports-9a` y `workflow-reports-export-9a`; aprobaron las dos de
  la 9B e `inventory.spec.ts`). Causa: todas las especificaciones comparten la base de cada motor y corren en orden
  alfabético; con el nombre `documents-*.spec.ts`, las de la 9B corrían antes que `restaurant.spec.ts` y consumían el folio
  1 que esta espera, y las siguientes fallaron en cascada. Corrección de la fase D: se renombraron a
  `workflow-stage9b-designer.spec.ts` y `workflow-stage9b-identity.spec.ts`, que corren después de las existentes. La
  corrida completa con los nombres nuevos forma parte de la regresión final.

## Revisión independiente (fase D)

Un agente de solo lectura revisó el diff `main..5fe33a2` (backend y `api.rs` idénticos en `f3509d3`) en copias exactas
del commit, con el clúster temporal 55499. **Resultado: sin hallazgos críticos ni altos, 5 medios y 8 bajos, ninguna fuga
de datos protegidos; la equivalencia de las plantillas predeterminadas es exacta.** Salida registrada por el revisor:
`go vet` con y sin `integration` sin hallazgos; `go test -race` `ok` en documents, designer, payments, logoraster,
identity, brand, reports y kitchen; integración `designer` 17.4 s, `payments` 2.2 s, `identity` 15.95 s y `database`
6.09 s `ok`; reproducciones propias de cada hallazgo (pruebas Go y Vitest que fallaban a propósito), entre ellas un JPEG
progresivo válido de menos de 1 MiB con unos 61 000 barridos que tardaba 1 min 10 s en validarse.

Correcciones aplicadas en la fase D (comportamiento documentado en [DOCUMENT_DESIGNER.md](DOCUMENT_DESIGNER.md);
detalle en el avance). Cada una con una prueba que **falló antes** de corregir (copia de `HEAD` más las pruebas) y pasa
después:

| Hallazgo                                   | Corrección                                                                                                      | Prueba                                                                                                         |
| ------------------------------------------ | --------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| Medio 1 · logotipo como último bloque      | salto de línea antes de `GS v 0`                                                                                | `documents/escpos_test.go`                                                                                     |
| Medio 2 · JPEG progresivo lento            | ≤ 100 barridos y ≤ 1024 segmentos antes de decodificar; validación antes del lock de la identidad               | `logoraster/validate_scans_test.go` (el JPEG de 61 000 barridos se rechaza en < 0.5 s); integración `identity` |
| Medio 3 · «Importes» de la lista de cocina | solo «Según Cocina» u «Ocultar» (`yes` → 422); las muestras aplican la configuración de Cocina                  | `documents/kitchen_prices_test.go`, `kitchen_sample_settings_test.go`; integración `designer`                  |
| Medio 4 · colas CUPS raw                   | opción por impresora `accepts_escpos` («Esta impresora entiende ESC/POS (formato y logotipo)»), desactivada     | `kitchen/transport_formatted_test.go`, `designer/printers_test.go`, `printersAdmin.test.tsx`                   |
| Medio 5, Bajos 1 y 2 · panel de identidad  | campos en solo lectura con una operación pendiente; vista previa tras reemplazar; recarga fallida sin conflicto | `pages/documents/identity/identityPanel.test.tsx` (tres pruebas nuevas)                                        |
| Bajo 4 · reimpresión y plantilla vigente   | bloqueo mutuo (`409 PRINT_COPY_PENDING`)                                                                        | `operations/documents_stage9b_test.go` (cocina, tickets y caja, en ambos sentidos)                             |
| Bajo 5 · impresora de otro ancho           | el raster de otro ancho se omite con aviso                                                                      | `kitchen/transport_formatted_test.go`                                                                          |
| Bajo 6 · `GET documents/printers`          | límite total de 4 s para CUPS, sin locks consultivos                                                            | `designer/printers_test.go`                                                                                    |
| Bajo 7 · vista previa de caja real         | datos guardados (los anteriores a la 9B, con aviso)                                                             | `operations/documents_stage9b_test.go`                                                                         |
| Bajo 8 · `GET documents/recent`            | comprobantes con `cash.movements.read`; una sola consulta                                                       | `operations/documents_stage9b_test.go`                                                                         |
| Bajo 3 · autor de un archivo repetido      | sin cambio de comportamiento (se reutiliza el original por SHA-256); documentado                                | —                                                                                                              |
| Observación de la 0019                     | llaves compuestas con el ancho de papel                                                                         | integración `identity`                                                                                         |

Salida del agente de correcciones (copia aislada, clúster 55499): `go vet` con y sin `integration` sin hallazgos;
`go test -race` `ok` en documents, kitchen, designer, identity, logoraster, payments, reports, database y operations;
integración `database` (6.2 s), `designer` (18.0 s), `identity` (17.8 s) y `operations` (234.7 s) `ok`; `tsc --noEmit`
sin errores; Vitest de `pages/documents`, `pages/kitchen`, `pages/restaurant/documents` y `hooks` (12 archivos, 82
pruebas) aprobado. Fuera de alcance y sin cambiar: `catalog/images.go` (etapa 3) tiene el mismo riesgo de los barridos de
JPEG.

Las pruebas de estas correcciones y su salida forman parte de la regresión final de la fase D.

## Cobertura de la etapa 9B

Correspondencia con las pruebas obligatorias de §31 de la especificación. Rutas relativas a `backend/internal/` salvo que
se indique otra.

| Prueba exigida (§31)                                         | Dónde y qué                                                                                                                                                                                                                                              |
| ------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Crear, editar, duplicar, versionar, restaurar predeterminada | `designer/designer_integration_test.go` (`TestDocumentDesignerAcceptance`); `operations/documents_stage9b_test.go` («9B_restaurar_predeterminadas»); Vitest de `pages/documents/templates` y `designer`; Playwright `workflow-stage9b-designer.spec.ts`. |
| Cargar logotipo                                              | `logoraster` (validación y conversión), `identity` (`TestDocumentIdentity`), Vitest `identityPanel.test.tsx`, Playwright `workflow-stage9b-identity.spec.ts`.                                                                                            |
| Vista previa, cambio 58/80 mm, campos dinámicos              | `documents/render_test.go`; `designer` («vista previa con datos de muestra»); `operations` («9B_vista_previa_con_comanda_real»); Vitest del editor; Playwright del diseñador (58/80).                                                                    |
| Control de permisos                                          | `designer` e `identity` por API directa (403 y `access.denied`); `operations` (plantilla vigente sin `documents.history.render_current`); permisos de la 0018 iguales a Go solo en el administrador (`documents/migration_test.go`, `database`).         |
| Rechazo de elementos peligrosos                              | `documents/validate_test.go` (tipos desconocidos e incompatibles, protegidos fuera de cortes, claves desconocidas, límites, aviso fiscal, controles); `logoraster` (SVG, GIF, WebP, HTML, BMP, truncados, dimensiones enormes).                          |
| Conservación de históricos y reimpresión exacta              | `operations/documents_stage9b_test.go` (comanda, precuenta, ticket, comprobante y corte reimpresos idénticos tras editar la plantilla); `documents_logo_9b_test.go` (tras reemplazar y retirar el logotipo); Playwright del diseñador (por la API).      |
| Impresión de muestra sin generar venta                       | `designer` (conteos de ventas, pagos, movimientos y documentos sin cambio); Playwright del diseñador.                                                                                                                                                    |
| Compatibilidad con la cola y control de inciertos            | `operations` (trabajos en la cola de siempre, resultado incierto con el transporte controlado); `kitchen/transport_formatted_test.go` y `transport_logo_test.go`.                                                                                        |
| Predeterminadas idénticas (§0-B)                             | `kitchen/legacy_template_test.go` y `template_equivalence_test.go`, `payments/legacy_documents_test.go` y `documents_equivalence_test.go` (cifras en «A»); semilla de la 0018 igual a Go (`documents/migration_test.go`, `designer`).                    |
| Vista previa e impresión con la misma IR                     | Un solo `documents.Render` para ambas (`documents/render_test.go`); `operations` (IR guardada en el trabajo con logotipo y cargada por el trabajador).                                                                                                   |
| Bloques protegidos sin permiso y en arqueo                   | `operations` («9B_esperado_ausente_sin_permiso_y_durante_el_arqueo»); `documents/validate_test.go` (rechazo fuera de cortes).                                                                                                                            |
| Plantilla vigente como acción distinta                       | `operations` (comandas, tickets y caja: permiso, auditoría, ticket `document_rerender`, sin `payment_documents` ni `cash_documents`; documento anterior a la 9B con `DOCUMENT_DATA_UNAVAILABLE`); Vitest `rerender.test.tsx`.                            |
| Logotipo a monocromo 58/80; cola sin raster                  | `logoraster` (umbral y Floyd–Steinberg exactos, anchos, alineación, altura máxima); `kitchen` (`TestRealLogoThroughCUPSQueues`: CUPS sin raw imprime el texto con aviso y `submitted`).                                                                  |
| Observación con caracteres de control                        | `documents/validate_test.go` y `render_test.go` (saneamiento al guardar y al renderizar); equivalencia de cocina (`kitchen/template_equivalence_test.go`) y comercial (`payments/documents_equivalence_test.go`) con textos que llevan ESC y GS.         |
| Dos administradores                                          | `designer` (`409 REVISION_CONFLICT` sin sobrescribir); `identity` (conflicto e idempotencia concurrente); Vitest del editor (diálogo de conflicto con borrador conservado).                                                                              |
| Reportes y exportaciones de la 9A sin cambios                | `reports` (`TestReportExports`, `TestReportsAcceptance`, `reports/operational`); cadenas del PDF y hoja «Datos» idénticas a la 9A; `export_identity_test.go` solo cambia el encabezado.                                                                  |
| Regresiones generales                                        | En cada fase, la aceptación encadenada de `internal/operations` (etapas 4–9A y 9B) y las suites de `administration`, `payments` y `database`; Playwright de `restaurant` y `stock-consumption` en C. Regresión completa: fase D.                         |

## Mediciones

- Raster de la marca de AD-EL POS al 60 %: 384 × 77 puntos (3 696 bytes) en 58 mm y 576 × 115 (8 280 bytes) en 80 mm; al
  100 % en 80 mm, 13 824 bytes. Máximo: 15 360 bytes (58 mm) y 23 040 (80 mm). Conversión de la marca de 2048 × 682 ≈ 20 ms
  sin `-race` (Apple M1 Max).
- Encabezado de los reportes: marca de 504 × 168 px en PNG de 27 886 bytes; PDF de 5 renglones 63 632 bytes con la marca
  frente a 35 410 sin imagen; XLSX de 3 renglones 39 324 frente a 9 859. Aumento fijo por archivo.
- Mediciones de un solo equipo, sin varias terminales.

## Pendiente (pruebas físicas y de entorno)

No se afirma ninguna de estas; quedan para cuando el hardware y el entorno estén disponibles (§32):

- **Papel**: tickets, precuentas, comandas, avisos, comprobantes y cortes con plantillas propias, negrita, tamaños dobles y
  **logotipo** en las impresoras reales del propietario (ESC/POS por red y USB por CUPS); corte del papel; muestras
  impresas. Las impresoras de 80 mm con 512 puntos imprimibles podrían recortar el logotipo.
- **Cajón** conectado a una impresora de red y a una impresora USB por CUPS.
- **Pantalla táctil física**: editor (arrastre, segmentos del papel menores de 44 px), identidad y teclado virtual.
- **Dos terminales reales** editando la misma plantilla o la identidad (solo con dos sesiones en pruebas de integración y
  Vitest).
- **Ventana nativa Tauri (WKWebView)**: pantalla Documentos, editor, carga del logotipo con el selector de archivos
  nativo, protección de cambios sin guardar al cerrar la ventana (no la cubre).
- **Diálogo nativo real de guardado** de las exportaciones con el encabezado nuevo (solo la simulación de Playwright).

## Qué no se afirma

- Que un documento con formato o logotipo salga bien en papel: solo se comprobaron los bytes y los argumentos que se
  envían, con adaptadores simulados.
- Que una cola CUPS que admite raw entienda ESC/POS: por eso existe la opción por impresora, desactivada por omisión.
- Que `submitted` signifique papel impreso: sigue siendo aceptación de CUPS o TCP.
- Que la vista previa coincida con el papel de una impresora con otro número de puntos o de columnas que 384/576 y 30/42.

## Regresión final de la fase D

Ejecutada el 11 de noviembre de 2026 sobre copias exactas de la rama (`git archive`, sin `.git`, `.local` ni cachés), con
un clúster PostgreSQL temporal propio (puerto 55499, solo loopback) y Go temporal en 8183; nada contra la instalación
real. Primero sobre `31a0ea1` (código y documentación de la fase D) y después sobre `40c8d17`, que solo corrige la
especificación Playwright del diseñador (resolver la copia exacta antes de imprimir con la plantilla vigente, por el
bloqueo mutuo de copias de la revisión): Vitest y Playwright se repitieron sobre `40c8d17`; lo demás no cambia entre ambos
commits.

| Suite                    | Resultado                                                                                                                                                                                                                  |
| ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato                  | Prettier (sin `docs/etapas/`), gofmt y `cargo fmt --check` limpios                                                                                                                                                         |
| Metadatos                | `sync-metadata.mjs --check`: «Metadatos coherentes» (0.9.1, esquema 19)                                                                                                                                                    |
| `go vet`                 | `./...` y `-tags=integration ./...` sin hallazgos                                                                                                                                                                          |
| Go unitarias (`-race`)   | 22 paquetes `ok`, entre ellos `documents`, `designer`, `identity`, `logoraster`, `brand`, `kitchen`, `payments` y `reports`                                                                                                |
| Integración PostgreSQL   | `database` 5.7 s, `administration` 53.6 s, `catalog` 7.0 s, `operations` 232.3 s, `payments` 2.4 s, `reports` 90.8 s, `reports/operational` 40.1 s, `designer` 17.4 s, `identity` 18.0 s: todas `ok`                       |
| TypeScript estricto      | `tsc --noEmit` sin errores                                                                                                                                                                                                 |
| Vitest (`npm test`)      | 62 archivos, 536 pruebas aprobadas                                                                                                                                                                                         |
| Compilación del frontend | `npm run build` correcto                                                                                                                                                                                                   |
| Rust/Tauri               | `cargo test --locked --offline`: 33 aprobadas, 1 ignorada (Keychain real)                                                                                                                                                  |
| Playwright               | Chromium 12/12 (4.7 min) y WebKit 12/12 (4.7 min), incluidas `workflow-stage9b-designer.spec.ts` y `workflow-stage9b-identity.spec.ts` y las anteriores (operación, cocina, cobros, caja, inventarios, compras y reportes) |
| Paquete de escritorio    | `.app` 0.9.1 compilado en la copia aislada (`tauri build --bundles app`, 9.94 MiB, Mach-O arm64); no se reemplazó ni se abrió el de la instalación                                                                         |

La primera corrida de la regresión dio dos incidencias, ninguna de producto: Vitest se invocó sin el filtro `src` del
proyecto y recogió las especificaciones de Playwright (se repitió con `npm test`), y la especificación del diseñador no
resolvía la copia exacta antes de la impresión con la plantilla vigente, que la corrección del bloqueo mutuo rechaza a
propósito (la falla de la especificación de identidad fue en cascada: al fallar una prueba, Playwright reinicia el proceso
y pierde la terminal preparada). Corregida en `40c8d17`.

No verificado (pruebas físicas pendientes, §32): papel de tickets, precuentas, comandas, cortes y del logotipo; cajón por
red y por USB/CUPS; pantalla táctil física; dos terminales reales; ventana nativa (WKWebView) y diálogo nativo de
guardado. Ninguna se da por hecha con los adaptadores simulados.

## Instalación real

0018 y 0019 se aplicaron a la instalación real el 11 de noviembre de 2026 con autorización del propietario: respaldo
`backups/adel_pos-20261111T185713Z-bd1868.dump` con restauración verificada (127 tablas idénticas), servidor detenido
de forma ordenada, `migrate` `applied:2` y repetición `applied:0`, servidor 0.9.1 con readiness 200 (PostgreSQL
disponible, esquema compatible) y `/api/v1/version` 0.9.1/19, comprobaciones de solo lectura antes y después (solo
cambian migraciones, tablas 127 → 132 y permisos 147 → 153; todos los conteos operativos idénticos; plantillas del
sistema, permisos solo en el administrador e identidad vacía como se esperaba) y `.app` 0.9.1 compilado y abierto.
Detalle en [ETAPA_9B_AVANCE.md](ETAPA_9B_AVANCE.md), «Instalación real». El recorrido en la ventana nativa sigue
pendiente de verificación por el propietario.
