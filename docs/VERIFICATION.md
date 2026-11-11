# Verificación · Etapa 9B

Estado al **11 de noviembre de 2026**, zona America/Monterrey. Versión **0.9.1**, API v1, esquema **19** (migraciones
`0018_document_templates.sql` y `0019_document_identity.sql`, etapa 9B: identidad del establecimiento y diseñador visual
de documentos). Evidencia detallada por fase: [VERIFICATION_STAGE9B.md](VERIFICATION_STAGE9B.md). Evidencia anterior:
[etapa 9A](VERIFICATION_STAGE9A.md), [etapa 8](VERIFICATION_STAGE8.md), [etapa 7.1](VERIFICATION_STAGE7_1.md),
[etapa 7](VERIFICATION_STAGE7.md), [etapa 6.2](VERIFICATION_STAGE6_2.md), [etapa 6.1](VERIFICATION_STAGE6_1.md),
[etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md), [etapa 4](VERIFICATION_STAGE4.md),
[etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y [etapa 1](VERIFICATION_STAGE1.md). Avance:
[ETAPA_9B_AVANCE.md](ETAPA_9B_AVANCE.md). Guía: [DOCUMENT_DESIGNER.md](DOCUMENT_DESIGNER.md).

## Estado

- **Código**: fases 0 (`fd1de81`), A (`9a4884d`), B (`5fe33a2`, partes B1 y B2) y C (`f3509d3`) en commits locales de la
  rama `etapa-9b`, cada una con sus suites aprobadas en un entorno aislado (clúster PostgreSQL temporal en el puerto 55499
  y Go temporal para Playwright), según [VERIFICATION_STAGE9B.md](VERIFICATION_STAGE9B.md). En la fase D: versión 0.9.1 y
  esquema 19, documentación, una **revisión independiente** (sin hallazgos críticos ni altos ni fugas de datos protegidos)
  y sus correcciones, y el cambio de nombre de las especificaciones Playwright de la 9B para que corran después de las
  existentes.
- **Regresión final (fase D)**: ver «Regresión final de la fase D».
- **Instalación real**: **0.9.1 y esquema 19**. Las migraciones 0018 y 0019 se aplicaron el 11 de noviembre de 2026 con
  autorización del propietario: respaldo `backups/adel_pos-20261111T185713Z-bd1868.dump` con restauración verificada
  (127 tablas idénticas), `migrate` `applied:2` y repetición `applied:0`, readiness 200 con esquema compatible y
  `/api/v1/version` 0.9.1/19, comprobaciones de solo lectura antes y después, y servidor y `.app` 0.9.1 instalados
  (registro en [ETAPA_9B_AVANCE.md](ETAPA_9B_AVANCE.md), «Instalación real»).
- **Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432) ni enviaron trabajos
  a CUPS real, a una impresora, a un cajón o a una conexión TCP fuera de loopback.

## Qué está verificado

Todo con Go, PostgreSQL temporal, Vitest, Rust y Playwright en entorno aislado; la impresión, **solo con adaptadores
simulados** (transporte controlado de la cola, puerto TCP de loopback y ejecutor simulado de CUPS).

| Área                                                | Evidencia                                                                                                                                                                                                                                               |
| --------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Predeterminadas idénticas a los formatos anteriores | Equivalencia byte a byte contra copias literales: 6 144 comandas y avisos, 168 precuentas y tickets y más de 1 000 comprobantes y cortes, a 58 y 80 mm; bytes ESC/POS y argumentos de `lp` del camino plano sin cambios; semilla de la 0018 igual a Go. |
| Modelo, validación y seguridad de plantillas        | Unitarias de `documents` (tipos desconocidos e incompatibles, protegidos fuera de cortes, claves desconocidas, límites, aviso fiscal, controles) e integración de `designer` (permisos, idempotencia, `422` con el bloque).                             |
| Plantillas y versiones                              | `TestDocumentDesignerAcceptance`: crear, duplicar, guardar, renombrar, activar, restaurar, inmutabilidad y dos administradores con `409 REVISION_CONFLICT`.                                                                                             |
| Impresión, historia y reimpresión                   | Aceptación encadenada de `internal/operations`: versión activa guardada, reimpresión idéntica tras editar (comanda, precuenta, ticket, comprobante y corte), plantilla vigente con permiso y auditoría, protegidos sin permiso y en arqueo.             |
| Transportes con formato y logotipo                  | `kitchen`: plano idéntico, atributos, `GS v 0`, CUPS raw y sin raw con ejecutor simulado; comanda y muestra con logotipo por un puerto de loopback.                                                                                                     |
| Muestras                                            | `designer`: muestras sin ventas, pagos, movimientos ni documentos, con el resultado de la cola; Playwright del diseñador.                                                                                                                               |
| Identidad y logotipo                                | `logoraster` (validación, umbral y Floyd–Steinberg exactos, anchos, alineación, altura), `brand` y `TestDocumentIdentity` (14 subpruebas: textos, PNG y JPEG, rechazos, reemplazo, retiro, inmutabilidad, concurrencia, permisos y SSE).                |
| Encabezado de los reportes PDF y Excel              | `reports` (`TestReportExports` con el subtest del encabezado; cadenas del PDF y hoja «Datos» idénticas a la 9A) y unitarias de `export_identity_test.go`.                                                                                               |
| Interfaz                                            | Vitest (529 pruebas en 60 archivos en la fase C) y Playwright Chromium y WebKit `workflow-stage9b-designer` y `workflow-stage9b-identity`, sin desbordamiento a 1280 × 720 y 1920 × 1080 en ambos temas.                                                |
| Puente nativo                                       | Pruebas Rust de la lista cerrada `documents_*`, de la identidad y de la plantilla vigente (33 aprobadas, 1 ignorada en el cierre de la fase B).                                                                                                         |
| Revisión independiente                              | Revisión de solo lectura de las fases 0–B con reproducciones propias; sus correcciones se documentan en [VERIFICATION_STAGE9B.md](VERIFICATION_STAGE9B.md) y se prueban en la regresión final.                                                          |

## No ejecutado o no verificado

- **Papel**: ningún documento con plantilla propia, formato o logotipo se imprimió; el logotipo en papel es una prueba
  física pendiente del propietario. Tampoco el cajón conectado a una impresora de red o USB.
- **Ventana nativa Tauri (WKWebView)**, selector de archivos nativo para el logotipo, diálogo nativo real de guardado de
  las exportaciones, **pantalla táctil física** y teclado virtual en Documentos.
- **Dos terminales reales**, volumen real, LAN con otra máquina, Keychain real y Windows.
- **Instalación real**: el recorrido de la 9B en la ventana nativa (Documentos, identidad, editor, muestra en una
  impresora real) queda como verificación del propietario.

## Regresión final de la fase D

Aprobada el 11 de noviembre de 2026 sobre copias exactas de `31a0ea1` y `40c8d17` (clúster temporal 55499): formato,
metadatos, `go vet`, 22 paquetes Go con `-race`, integración PostgreSQL de los nueve paquetes (`database`,
`administration`, `catalog`, `operations`, `payments`, `reports`, `reports/operational`, `designer` e `identity`),
`tsc --noEmit`, Vitest (62 archivos, 536 pruebas), `npm run build`, `cargo test` (33 aprobadas, 1 ignorada), Playwright
Chromium 12/12 y WebKit 12/12, y el `.app` 0.9.1 compilado en la copia aislada. Detalle en
[VERIFICATION_STAGE9B.md](VERIFICATION_STAGE9B.md) y en el avance. Las pruebas físicas siguen pendientes.

## Repetir las comprobaciones

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
```

`test-integration` y `test-browser` usan bases desechables en el clúster del proyecto (55432); para aislamiento total,
seguir `CLAUDE.md` §6. Las pruebas de integración de la 9B viven en `internal/designer`, `internal/identity` (ambos en
`scripts/database.py`), `internal/database` e `internal/operations` (encadenadas a `TestRestaurantAcceptance`); las
unitarias, en `internal/documents`, `logoraster`, `brand`, `kitchen`, `payments` y `reports`. Desde la etapa 9B,
`docs/etapas/` está en `.prettierignore`: `./scripts/dev.sh format` y `format-check` ya no tocan ni reportan las
especificaciones del propietario (ver `CLAUDE.md` §6). Las especificaciones Playwright comparten la base de
cada motor y corren en orden alfabético: una nueva debe ordenarse después de las existentes, y no conviene filtrarlas con
`documents-` (el filtro no distingue mayúsculas y la ruta temporal contiene «Documents-ADEL-POS»).
