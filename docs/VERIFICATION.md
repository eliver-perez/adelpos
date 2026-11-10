# Verificación · Etapa 7.1

Verificación por fases del **9 de noviembre de 2026**, zona America/Monterrey, antes de cada commit local de la rama `etapa-7.1` (fase A `9eb7f47`, fase B `58c461e`, fase C `8433b63`, fase D `2df3ecb`). Versión **0.7.1**, API v1, esquema **9** (migración `0009_operation_tables_cash_and_reasons.sql`). Evidencia anterior: [etapa 7](VERIFICATION_STAGE7.md), [etapa 6.2](VERIFICATION_STAGE6_2.md), [etapa 6.1](VERIFICATION_STAGE6_1.md), [etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md), [etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y [etapa 1](VERIFICATION_STAGE1.md). Avance: [ETAPA_7_1_AVANCE.md](ETAPA_7_1_AVANCE.md). Guías: [RESTAURANT_OPERATIONS.md](RESTAURANT_OPERATIONS.md), [CASH_SHIFTS_AND_RECONCILIATION.md](CASH_SHIFTS_AND_RECONCILIATION.md), [KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md), [PAYMENTS_AND_REFUNDS.md](PAYMENTS_AND_REFUNDS.md) y [CATALOG.md](CATALOG.md).

**Alcance de esta evidencia.** Las cifras son las que cada fase registró en el avance. Se obtuvieron con `config/product.json` todavía en 0.7.0 y esquema 8 (la 0009 ya estaba embebida desde la fase C); la versión 0.7.1 y el esquema 9 se fijaron en la fase E. La regresión completa sobre el árbol 0.7.1 y la compilación del `.app` 0.7.1 corresponden a «Regresión final (fase E)».

**Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432): la 0009 se aplicó a la instalación real después de la regresión final, con el procedimiento que registra «Instalación real». Ninguna prueba envió trabajos ni pulsos a CUPS, a una impresora, a un cajón ni a una conexión TCP. Las únicas consultas al CUPS de este Mac fueron lecturas (`lpstat`, `lpoptions`) de la fase C para investigar el criterio de datos raw y grabar sus salidas de prueba.

## Entorno aislado

Copia del repositorio sincronizada con el árbol de trabajo en cada fase, clúster PostgreSQL temporal en el puerto 55499, bases `adel_pos_test_*` desechables y Go temporal en 8183 para Playwright, según `CLAUDE.md` §6. Guiones auxiliares de la copia: `tools/suite.sh` (estáticas), `tools/iter_snap.sh` (integración) y `tools/format71.sh` (formato, fases C y D).

Dobles de prueba: el adaptador controlado de la cola (`recordingTransport`) para impresión y cajón; la consulta de colas simulada `controlledQueues` de la aceptación, que evalúa atributos simulados con `kitchen.EvaluateRawQueue` y nunca ejecuta `lpstat`, `lpoptions` ni `lp`; y un ejecutor simulado en las unitarias de `kitchen`, que registra argumentos y el contenido del archivo del pulso sin ejecutar nada.

## Resultados ejecutados por fase

| Comprobación             | Fase A (`9eb7f47`)                                     | Fase B (`58c461e`)                               | Fase C (`8433b63`)                               | Fase D (`2df3ecb`)                                                          |
| ------------------------ | ------------------------------------------------------ | ------------------------------------------------ | ------------------------------------------------ | --------------------------------------------------------------------------- |
| Estáticas                | Aprobadas                                              | Aprobadas                                        | Aprobadas                                        | Aprobadas                                                                   |
| Go unitarias             | `ok` en todos los paquetes, con `moneytext` y `plural` | `ok` con `-race`                                 | `ok` con `-race`, con las de CUPS de `kitchen`   | `ok` con `-race`, con `payments` y `ordering`                               |
| Integración PostgreSQL   | Seis paquetes `ok`                                     | Seis `ok`; `operations` 120,9 s                  | Seis `ok`; `operations` 121,0 s                  | Seis `ok`; `operations` 126,4 s                                             |
| Vitest                   | 276 pruebas en 36 archivos                             | 290 en 36                                        | 306 en 37                                        | 317 en 39                                                                   |
| `tsc` y `npm run build`  | Sin errores                                            | Sin errores                                      | Sin errores                                      | Sin errores                                                                 |
| Rust (`cargo test`)      | 14 aprobadas, 1 ignorada                               | 14 aprobadas, 1 ignorada                         | 14 aprobadas, 1 ignorada                         | 15 aprobadas, 1 ignorada                                                    |
| Playwright               | 1 aprobada en Chromium y 1 en WebKit                   | Igual (46,2 s y 45,1 s)                          | Igual                                            | Igual (55,1 s y 53,5 s)                                                     |
| Repetición independiente | Resultados de arriba, sobre la copia sincronizada      | Antes del commit, iguales (`operations` 123,0 s) | Antes del commit, iguales (`operations` 125,8 s) | Tras agregar `reason_name`, todas las suites iguales (`operations` 126,7 s) |

- **Estáticas**: Prettier, gofmt, `cargo fmt`, `sync-metadata` y `go vet`, con y sin la etiqueta `integration`.
- **Integración PostgreSQL**: `database`, `administration`, `catalog`, `operations`, `payments` y `kitchen`. `operations` encadena `TestRestaurantAcceptance` con las etapas 4–6.2, la caja de la etapa 7 (C1–C47) y las pruebas nuevas de la 7.1, de modo que cada fase repitió las suites de las etapas anteriores. La prueba física opt-in `TestControlledCUPSPrinter` quedó omitida en todas las fases. En las fases C y D, una segunda ejecución detallada confirmó `TestStage71UpgradeOfExistingInstallation`, C37–C54 (fase C) y las doce subpruebas `71D_` (fase D).
- **Rust**: la prueba ignorada es la de Keychain real (opt-in). La fase D sumó `discount_reasons_and_quick_notes_routes_are_closed`.
- **Playwright**: una sola prueba por motor que recorre las etapas 4 a 7.1 (`e2e/restaurant.spec.ts` y los recorridos que encadena).

## Cobertura de la etapa 7.1

### Go: integración PostgreSQL y unitarias

| Prueba exigida (§5 de la especificación)        | Dónde y qué                                                                                                                                                                                                                                                                                                                                                     |
| ----------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Cuenta creada con el primer producto            | `operations/tables_stage71_test.go` (fase B): cuenta y borrador en una sola operación (`account.opened` y `draft.saved` con el mismo `operation_id`), reintento idempotente sin otra cuenta, `IDEMPOTENCY_CONFLICT` con otro contenido, `PUT …/draft` y envío a cocina posteriores, validaciones del borrador inicial sin escribir.                             |
| Salida sin capturar sin registro                | Misma prueba: tras las consultas de la captura no hay cuentas, borradores, operaciones, eventos, outbox, auditoría ni pedidos nuevos.                                                                                                                                                                                                                           |
| Dos terminales en la misma mesa libre           | Misma prueba: `TABLE_HAS_ACCOUNT` con la cuenta de la otra terminal y sin registros, «Agregar a esa cuenta» conservando su borrador, «Abrir una cuenta nueva», dos aperturas simultáneas (una cuenta y un 409) y «Nueva cuenta» en mesa ocupada.                                                                                                                |
| Liberar mesa vacía y rechazo con contenido      | Misma prueba: mesero con `accounts.open`, idempotente, auditada una vez, motivo fijo, evento y mesa libre; `ACCOUNT_NOT_EMPTY` sin registros con borrador, unidades canceladas, pago, ajuste cancelado, precuenta, cambio de cocina pendiente y combinación recibida.                                                                                           |
| Estados de mesa calculados                      | Misma prueba: libre, ocupada con el tiempo de la cuenta más antigua, esperando pago, invalidación por un cambio posterior y por un ajuste, pago parcial posterior que no la invalida, pago parcial, prioridad y saldo sumado con varias cuentas, y mesa libre al liquidar.                                                                                      |
| Fuera de servicio con y sin cuenta              | Misma prueba: permiso, idempotencia, auditoría, evento, sin cuentas nuevas, revisión obsoleta, estado repetido y habilitar; rechazo con cuenta abierta por la operación y por el PATCH; permisos comprobados antes de escribir.                                                                                                                                 |
| Cajón por CUPS raw con adaptador                | `operations/cash_stage71_test.go` C48–C52 (fase C): cola USB compatible, AirPrint incompatible con su motivo, cola sin respuesta, desactivar sin consultar la cola, pulso por CUPS con efectivo y ninguno con tarjeta, apertura manual con y sin permiso, «Probar apertura» con y sin permiso e idempotente, resultado incierto sin reintento. C37 actualizada. |
| Transporte CUPS raw (unitarias)                 | `kitchen/cups_raw_test.go`: detección con las salidas grabadas de las cuatro colas de este Mac, consultas de solo lectura con argumentos separados, `lp -d <cola> -o raw -n 1 -t ADEL-cajon-<trabajo> <archivo>` con el ejecutor simulado, bytes del pulso, archivo eliminado y resultados incierto, error y cola ya no compatible sin llamar a `lp`.           |
| Auditoría de rechazos por efectivo insuficiente | `operations/cash_stage71_test.go` C53–C54: rechazo sin cifras para el cajero y sin registros, auditoría `success=false` con usuario, terminal, turno, caja, tipo, moneda, importe y autorización, sin el esperado; consulta rechazada sin `cash.expected.read`, completa con él y ciega durante el arqueo.                                                      |
| Motivos de descuento                            | `operations/stage71_phase_d_test.go` (fase D): siembra y orden, descuento y cortesía con motivo, «Otro» con y sin descripción, compatibilidad sin `reason_id`, motivo inactivo, renombrar sin alterar lo registrado, administración con y sin permiso, precuenta, ticket e historial. Unitarias de la plantilla en `payments/documents_test.go`.                |
| Total en dólares en precuenta y ticket          | Misma prueba: ausente sin la función, sin tipo de cambio o sin turno en la caja de la terminal; presente con el tipo del turno sin cambiar los importes en pesos. Unitarias: línea después del TOTAL y ajuste a 58 mm.                                                                                                                                          |
| Indicaciones por categoría                      | Misma prueba: permisos, validaciones (comas, vacías, más de 80, saltos de línea, mayúsculas repetidas, categoría inexistente), edición con revisión, orden, lookups y llegada a la comanda («NOTA: Bien cocida, Sin cebolla, Sin sal»).                                                                                                                         |
| Formato de importes y plurales (fase A)         | `moneytext/moneytext_test.go` y `plural/plural_test.go` con tablas compartidas con TypeScript (`testdata/cases.json`); `payments/documents_test.go`, `payments/cash_documents_test.go` y `kitchen/template_test.go` actualizados al formato con miles.                                                                                                          |
| Actualización desde el esquema 8                | `database/existing_install_test.go` (`TestStage71UpgradeOfExistingInstallation` con `checkStage71PhaseD`): turno, cajón y comando manual conservados; CHECK del cajón con su nombre y el motivo `test`; índice parcial; descuento de la etapa 6 con `reason_id` nulo; siembra; once restricciones con nombre e índice único; migración repetible.               |

### Vitest

- **Avisos y diálogo de confirmación** (fase A): `components/messages.test.tsx` (el éxito se cierra solo, el puntero encima no detiene el cierre, el error permanece hasta tocarlo y no muestra el código interno, acción opcional, sustitución por texto o `id`; diálogo de peligro, foco inicial en la opción segura y atrapado, Escape cancela), `pages/adminMessages.test.tsx`, `pages/cash/notices.test.tsx` y `pages/kitchen/kitchenMessages.test.tsx`. En la fase D, tres éxitos y un error apilados con temporizadores falsos.
- **Formato de importes** (fase A): `services/money.test.ts`, con la tabla compartida con Go (miles, `$` y `US$`, signo antes del símbolo, «—» sin valor, cifras que un `number` redondearía). **Plurales**: `services/plural.test.ts`.
- **Mesas** (fase B): `capture/draftSync.test.ts`, `capture/capture.test.tsx` y `restaurant/tablePanel.test.tsx` (creación con el primer guardado, toques que se anulan, `TABLE_HAS_ACCOUNT` con sus dos opciones, panel de mesa libre, «Liberar mesa», fuera de servicio y leyenda con los cinco estados).
- **Tarjetas de Caja y cierre del panel tras registrar** (fase C): `pages/cash/stage71.test.tsx` (icono grande, acento por tipo, `aria-pressed` sin casillas; un solo aviso, recarga del turno e «Imprimir comprobante»; atajo «Caja» con sus tres estados y permisos; aviso sin turno; filtro «Intentos rechazados»; configuración del cajón y «Probar apertura»).
- **Fase D**: `adjustments/adjustments.test.tsx`, `capture/capture.test.tsx` (indicaciones de la categoría), `catalog/quickNotes.test.tsx` y `payments/discountReasons.test.tsx`.

### Playwright (Chromium y WebKit)

- `e2e/restaurant.spec.ts` (fase B): leyenda con los cinco estados; panel de mesa libre en claro y oscuro a 1280 × 720 y 1920 × 1080; mesa libre → capturar → salir sin productos → mesa libre (sin cuentas en Go); doble toque en «Capturar»; mesa libre → producto → cuenta creada con el primer guardado → enviar a cocina; «Nueva cuenta» sin productos no crea cuenta; «Liberar mesa» sobre una cuenta vacía creada con la API de prueba; precuenta impresa → «Esperando pago».
- `e2e/cash-flow.ts` (fase C): aviso sin turno con «Abrir turno»; ingreso de $100 desde el atajo «Caja» de la captura con su aviso e «Imprimir comprobante» (no se pulsa: el destino no tiene hardware), posición y cierre del aviso; «Ir a Caja»; tarjetas en ambos temas; retiro desde Caja con el panel cerrado; corte parcial; atajo «En arqueo».
- `e2e/payments-flow.ts` (fase D): descuento del 10 % con «Atención» y total en dólares en la precuenta en pantalla.
- `e2e/notices.ts`: posición de los avisos y cierre con el puntero encima (fase A); en todo el recorrido, cada éxito se retira antes de 5 s y ningún aviso tapado muestra su contenido (fase D).
- `expectBillFooterFits` (en `restaurant.spec.ts`, fase D): el pie de la cuenta de la captura no se desborda a 1280 × 720 ni a 1920 × 1080, en claro y oscuro.

### Revisión visual

Playwright tomó capturas en claro y oscuro del panel de mesa libre y de la hoja del atajo «Caja» (a 1280 × 720 y 1920 × 1080), de la mesa «Esperando pago» y de las tarjetas de Caja. En la fase C se detectaron el pie de la cuenta recortado a 1280 × 720 y, en las capturas, avisos anteriores que seguían visibles bajo el más reciente; ambos se corrigieron en la fase D con las comprobaciones de Playwright citadas arriba.

## No ejecutado o no verificado

- **Ventana nativa Tauri (WKWebView)**: avisos, diálogos, mapa de mesas, atajo «Caja» y tarjetas solo se comprobaron con Vitest (jsdom) y Playwright. La compatibilidad de Sonner con la CSP se comprobó en el código de Tauri y en `dist/index.html`, no en la ventana.
- **Pantalla táctil física**: no probada; la supresión de la pausa por «hover» de los avisos solo se comprobó con Playwright.
- **Papel**: no se imprimió ninguna precuenta ni ticket (con motivo o total en dólares), comprobante, corte ni comanda con indicaciones rápidas. En la fase D la impresora de tickets de las pruebas era un agente no disponible y los trabajos quedaron en intervención.
- **Cajón físico por CUPS**: no se ejecutó `lp` ni se envió ningún pulso; solo el adaptador controlado y el ejecutor simulado. La prueba física queda pendiente hasta que el propietario tenga una impresora USB con cajón, y no existe una prueba de hardware del cajón. Tampoco hay pulso físico por ESC/POS TCP (como en la etapa 7).
- **Keychain real**: no probado (la prueba Rust correspondiente sigue ignorada).
- **LAN** con otra máquina y dos terminales nativas simultáneas: «Esta mesa ya tiene una cuenta» y las aperturas simultáneas solo se verificaron con la integración PostgreSQL y Vitest.
- **Windows**: no probado ni preparado. El envío del pulso por el sistema de impresión de Windows no existe; solo queda preparado detrás de la interfaz `Transport`.
- **Sin recorrido Playwright**: indicaciones rápidas en la captura y administración de motivos e indicaciones (solo Vitest e integración de Go).
- **Sin medición de volumen**: la regla de cuenta vacía consulta tablas sin índice por cuenta.

## Regresión final (fase E)

Ejecutada el 9 de noviembre de 2026 con la versión 0.7.1 y `schema_version: 9`, sobre la copia aislada sincronizada con el repositorio (clúster temporal 127.0.0.1:55499, Go temporal en 8183):

- **Estáticas** (`tools/suite.sh`): Prettier («All matched files use Prettier code style!», sin las especificaciones del propietario), gofmt sin archivos, `cargo fmt --check`, `sync-metadata --check`, `go vet` y `go vet -tags=integration`, todos con salida 0.
- **Go unitarias** con `-race`: `ok` en todos los paquetes, incluidos `moneytext` y `plural`.
- **Vitest**: 39 archivos y 317 pruebas aprobadas; `tsc` y `npm run build` sin errores.
- **Rust**: `cargo test`, 15 aprobadas, 0 fallidas y 1 ignorada (Keychain real).
- **Integración PostgreSQL** (`tools/iter_snap.sh`): `database` (2,4 s), `administration` (53,7 s), `catalog` (6,2 s), `operations` (126,3 s), `payments` (1,4 s) y `kitchen` (1,4 s) en `ok`. La prueba de impresora física quedó omitida porque es opcional. Los paquetes con pruebas de integración son los mismos que lista `scripts/database.py`, más `kitchen`.
- **Playwright**: 1 aprobada en Chromium (1,0 min) y 1 en WebKit (1,0 min).
- **Paquete de escritorio**: `tauri build --bundles app` en la copia aislada, con `CARGO_TARGET_DIR` en el scratchpad: `AD-EL POS.app` 0.7.1, ejecutable Mach-O arm64, 9,5 MB.
- Ninguna suite envió trabajos a una impresora ni a un cajón real.
- `./scripts/dev.sh format-check` sobre el repositorio completo no aprueba solo por las especificaciones del propietario `docs/etapas/ETAPA_7.md` y `ETAPA_7_1.md`, que no se formatean.

## Instalación real

El 9 de noviembre de 2026, con autorización previa del propietario para aplicar los cambios al terminar si todo aprobaba: respaldo `backups/adel_pos-20261110T022252Z-c7e1e4.dump` con restauración verificada (90 tablas idénticas, `.local/stage9-restore-verification.json`), `server-stop` ordenado, `migrate` `applied:1` y repetición `applied:0`, servidor 0.7.1 iniciado con readiness 200 (PostgreSQL disponible, esquema compatible) y `/api/v1/version` con 0.7.1 y esquema 9, y `.app` 0.7.1 compilado con `build-macos` y abierto con `open-app`. Comprobaciones de solo lectura antes y después: migraciones 1–9; 90 → 92 tablas en `app`; los mismos conteos de cuentas (4 abiertas y 2 cerradas), pagos (2), ajustes (0), turnos (1 cerrado), comandos del cajón (0), categorías (4), auditoría (107) y permisos (113, ninguno nuevo); los cinco motivos de descuento activos en su orden, solo «Otro» con descripción; ningún ajuste con motivo ni indicación rápida; el CHECK del cajón admite `test`, existe el índice de rechazos y las once restricciones con nombre de la 0009; `usd_cash` desactivada. El recorrido en la ventana nativa no se verificó. La 0009 es ahora inmutable; la siguiente ampliación requiere 0010.

## Repetir las comprobaciones

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
```

`test-integration` y `test-browser` usan bases desechables en el clúster del proyecto (55432); para aislamiento total, seguir `CLAUDE.md` §6. `docs/etapas/ETAPA_7.md` y `docs/etapas/ETAPA_7_1.md` no tienen formato Prettier (`prettier --check`, 9 de noviembre de 2026): mientras sigan así, `format-check` los reportará.
