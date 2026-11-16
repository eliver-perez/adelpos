# Verificación · Etapa 9H

Estado al **16 de noviembre de 2026**. Versión **0.11.0**, API v1, esquema **24** (migración `0024_legacy_migration.sql`,
con las secciones A y C; etapa 9H: migración del sistema anterior con todo su historial y pendientes —«Cancelar
unidades» sin autorización de supervisor, precio unitario por renglón y sesiones en varios dispositivos—). Evidencia
detallada: [VERIFICATION_STAGE9H.md](VERIFICATION_STAGE9H.md). Evidencia anterior: [etapa 9G](VERIFICATION_STAGE9G.md),
[etapa 9F](VERIFICATION_STAGE9F.md), [etapa 9E](VERIFICATION_STAGE9E.md), [etapa 9D](VERIFICATION_STAGE9D.md),
[etapa 9C](VERIFICATION_STAGE9C.md), [etapa 9B](VERIFICATION_STAGE9B.md), [etapa 9A](VERIFICATION_STAGE9A.md),
[etapa 8](VERIFICATION_STAGE8.md), [etapa 7.1](VERIFICATION_STAGE7_1.md), [etapa 7](VERIFICATION_STAGE7.md),
[etapa 6.2](VERIFICATION_STAGE6_2.md), [etapa 6.1](VERIFICATION_STAGE6_1.md), [etapa 6](VERIFICATION_STAGE6.md),
[etapa 5](VERIFICATION_STAGE5.md), [etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md),
[etapa 2](VERIFICATION_STAGE2.md) y [etapa 1](VERIFICATION_STAGE1.md). Avance: [ETAPA_9H_AVANCE.md](ETAPA_9H_AVANCE.md).
Guía para el propietario: [MIGRAR_SISTEMA_ANTERIOR.md](MIGRAR_SISTEMA_ANTERIOR.md). Análisis y formato:
[MIGRACION_ANALISIS.md](MIGRACION_ANALISIS.md) y [MIGRACION_FORMATO.md](MIGRACION_FORMATO.md). Windows:
[NOTAS_WINDOWS.md](NOTAS_WINDOWS.md).

## Estado

- **Código**: fases 0, A, B, C (partes 1 y 2), D, E y F y las correcciones de las revisiones independientes de B y C,
  cada una en su rama y fusionadas por el coordinador en `etapa-9h` (creada desde `main`, 0.10.0 con la 9G), con sus
  pruebas según el avance; la Fase E encontró un defecto del reemplazo del lote con el volumen real (D-1), corregido y
  comprobado otra vez con el respaldo real.
- **macOS**: todo se probó en la Mac en copias aisladas (clúster PostgreSQL temporal 55499, Go temporal en modo de
  pruebas), **con datos sintéticos** salvo la Fase 0 (análisis con consultas agregadas) y la Fase E (respaldo real en
  una base temporal aislada, borrado todo al terminar). Nada se instaló: el `.pkg` 0.11.0 solo se compiló y se examinó.
- **Instalación real**: el servicio de la Mac sigue en 0.10.0 (esquema 23) hasta que el propietario instale el `.pkg`
  0.11.0 con «Servidor del restaurante» marcada; el `host` respalda y aplica la 0024 al arrancar. La importación del
  sistema anterior en la instalación real la hace el propietario desde el asistente.
- **Windows**: nada de la 9H se ejecutó en Windows; solo se compiló y se revisó Go con `GOOS=windows`.

## Qué está verificado

En la Mac, con Go (`-race`), PostgreSQL temporal, Vitest, Rust y Playwright (Chromium y WebKit) contra un Go temporal en
modo de pruebas (detalle y conteos en [VERIFICATION_STAGE9H.md](VERIFICATION_STAGE9H.md)):

- Pendientes: «Cancelar unidades» y reducir cantidades sin supervisor (con quién lo hizo en el Registro de movimientos y
  en «Modificaciones de cantidades»), «Mostrar precio unitario» con y sin espacio (42/48 y 30/32 columnas) y las
  plantillas predeterminadas idénticas byte a byte, y el mismo usuario con sesión en dos terminales.
- Lector y paquete: formato `.adelmig` (frase incorrecta, paquete dañado, versión desconocida, límites), lector con un
  SQL Server temporal en Docker real (respaldo sintético), una preparación a la vez y limpieza de restos, y la
  comprobación de que el índice de git no tiene datos reales.
- Importación: correspondencia de cada tipo de dato con importes exactos, «IVA estimado», fecha de reconocimiento en el
  pago que liquida la venta, reemplazo completo del lote (también con estadísticas del lote anterior), rechazo en una
  instalación con operación propia, interrupción sin datos a medias, recuperación de corridas, historial de solo
  lectura, caja e inventario sin cambios y validación §4.4.
- Reportes: 27 reportes con el filtro «Datos» y, sin lote, exactamente los de antes.
- Asistente: preparar (simulado), importar con un paquete sintético real, resumen previo, confirmación, avance, informe
  y PDF, reemplazo con confirmación, «Asignar contraseña» y su primer inicio de sesión, menú Archivo y paleta.
- **Respaldo real** (Fase E, cifras agregadas en el avance): preparar en unos 36 s (paquete de 16.6 MiB), importar en
  unos 22 s, «Todo cuadra.» (42 de 42) con los totales iguales al centavo a los calculados en SQL Server, y el reemplazo
  después de la corrección en unos 29 s con los mismos resultados.
- Migración 0024 de 23 a 24 con datos.

## No ejecutado o no verificado

La instalación del `.pkg` 0.11.0 y la 0024 en la instalación real; la ventana nativa con sus diálogos reales para
elegir el respaldo y el paquete, el lector lanzado por la aplicación con Docker Desktop y que Tauri permita en la
ventana real las 11 operaciones nuevas (compiladas y simuladas); el PDF del informe impreso; el papel con el precio
unitario; dos terminales reales en la red; y todo Windows (lector con Docker Desktop de Windows, carpeta de trabajo con
ACL, cancelación por la entrada estándar). Detalle en [VERIFICATION_STAGE9H.md](VERIFICATION_STAGE9H.md), «No
verificado».

## Regresión final

Copia exacta de `80afeba` (`git archive`; después solo cambió Markdown), clúster temporal 55499, Go temporal 8183, vista
previa 1421 y Go limpio 8193, 16/11/2026, unos 2 250 s, todo aprobado a la primera: formato (con la comprobación de
datos reales sobre el índice de git), `sync --check`, `go vet` (normal e `integration`), `GOOS=windows` build y vet, Go
unitarias con `-race` (35 paquetes), integración PostgreSQL 14 de 14 (`operations` 324 s, `reports` 135 s,
`administration` 120 s; `reports/operational` completo fuera de la madrugada), el lector con Docker real y el respaldo
sintético (26 pruebas), `tsc`, Vitest (95 archivos, 902 pruebas), `npm run build`, `cargo test` (106 aprobadas, 3
ignoradas), Playwright Chromium 42 de 42 y WebKit 42 de 42 (todas las especificaciones) y los scripts del instalador
(31 comprobaciones). Detalle en [ETAPA_9H_AVANCE.md](ETAPA_9H_AVANCE.md), «Fase F · Regresión final».

## Repetir las comprobaciones

En la Mac, **desde el 14/11/2026 la instalación real es el servicio** (PostgreSQL en 55432 y Go en 8181): no usar
`./scripts/dev.sh test-integration` ni `test-browser` (necesitan `db-start` sobre 55432) ni ningún comando de `dev.sh`
que actúe sobre la base. Sin efectos sobre datos:

```bash
./scripts/dev.sh format-check
./scripts/dev.sh check-legacy-data
./scripts/dev.sh test
./scripts/dev.sh build-macos-pkg
```

Las pruebas de integración y de Playwright se corren con el procedimiento aislado de `CLAUDE.md` §6 (clúster temporal en
otro puerto, bases `adel_pos_test_*`, Go temporal con `test_mode: true` y, desde la 9H, el segundo Go temporal con base
limpia de `ADEL_BROWSER_CLEAN_SERVER_URL`). La tupla de integración de `scripts/database.py` incluye desde la 9H
`./internal/legacyimport` (14 paquetes); la prueba del lector con Docker real va aparte, con
`-tags 'integration legacydocker' ./internal/legacyread`. Mediciones opcionales: `ADEL_LEGACY_MEASURE=1`
(`TestLegacyImportVolume`, con un reemplazo) y `ADEL_REPORTS_MEASURE=1`. En Windows:
[VERIFICATION_STAGE9E.md](VERIFICATION_STAGE9E.md), «Repetir las comprobaciones», y las listas de la 9F, la 9G y la 9H
en [NOTAS_WINDOWS.md](NOTAS_WINDOWS.md).
