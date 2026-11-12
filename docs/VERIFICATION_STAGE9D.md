# Verificación · Etapa 9D

Verificación por fases del **12 de noviembre de 2026**, zona America/Monterrey, en la rama local `etapa-9d` (creada desde
`main` = `723ca7a`, que contiene la 9C): fase 0 `ecae96d`; backend de las fases D y E `b84e3be`; fase A `4abe78d`; fase E
(interfaz) `2c00064`; fase B `7506535`; fase C `ffcaed7`; correcciones de la revisión independiente (parte 1) `d723e92`;
fase D (interfaz) `81ff531`. API v1, migración `0021_account_cancellation.sql` (esquema 21).
Evidencia anterior: [etapa 9C](VERIFICATION_STAGE9C.md), [etapa 9B](VERIFICATION_STAGE9B.md),
[etapa 9A](VERIFICATION_STAGE9A.md), [etapa 8](VERIFICATION_STAGE8.md) y las anteriores desde
[VERIFICATION.md](VERIFICATION.md). Avance: [ETAPA_9D_AVANCE.md](ETAPA_9D_AVANCE.md). Especificación:
[etapas/ETAPA_9D_v2.md](etapas/ETAPA_9D_v2.md).

**Alcance de esta evidencia.** Las cifras son las que cada fase registró en el avance y en sus registros de ejecución
(directorio temporal de la sesión), más los cierres que el coordinador ejecutó sobre una copia exacta de cada commit. En
todas las fases `config/product.json` decía 0.9.2 con el esquema 21 (la 0021 embebida y el migrador aplicando 21
migraciones en cada base temporal); la versión 0.9.3 se fija en la fase F. La regresión completa sobre el commit final
está en «Regresión final».

**Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432, `backups/`, el `.app`
instalado). Las consultas a la instalación real de la Fase 0 fueron de solo lectura (`transaction_read_only = on`). No se
leyeron secretos y ninguna prueba envió trabajos a CUPS real, a una impresora, a un cajón ni a una conexión TCP fuera de
loopback. **La 0021 se aplicó a la instalación real el 12/11/2026** (ver «Instalación real»).

## Entorno aislado

Copias del repositorio en el directorio temporal de la sesión, una por parte (sin `.git`, `.local`, `.tools`, cachés ni
artefactos), clúster PostgreSQL temporal en el puerto 55499 (solo loopback), una base `adel_pos_test_*` desechable por
paquete, `GOPROXY=off` y Go temporal para Playwright en puertos propios de cada parte (8184/1422 en la fase A, 8187/1425
en la E, 8186/1424 en B y C y en las correcciones, 8188/1426 en la D y 8189/1427 en la revisión), según `CLAUDE.md` §6.
Las partes se desarrollaron en parte en paralelo (BE con A; E con B y C; las correcciones con D); cada cierre del
coordinador se ejecutó sobre una copia exacta del commit.

**Pruebas automatizadas, no físicas.** Vitest con jsdom y Playwright con Chromium y WebKit sustituyen el puente nativo
por la simulación de `e2e/native-driver.ts`. **No** sustituyen la ventana nativa (WKWebView), el puntero real de macOS,
una pantalla táctil física, el papel de una impresora ni Windows. El cursor se comprobó con el estilo calculado del
navegador; el arrastre del mapa, con el ratón de Playwright (eventos de puntero); la impresión, con el transporte
controlado de la cola (**bytes, no papel**). En las capturas, «Esperando pago» (exige una precuenta impresa, es decir,
una impresora de tickets), «3 d 14 h» y `long_open` (Go los calcula con su hora; la prueba no puede envejecer una cuenta)
los presenta el puente de la prueba, y «Sin conexión» se simula rechazando las peticiones en el puente. La cancelación de
cuentas, los pagos parciales y los reportes usan datos reales de Go.

## Resultados ejecutados por fase

| Fase o parte (commit)                                         | Estáticas                                                                                | Go unitarias (`-race`)                                                                       | Integración PostgreSQL (`-race`)                                                                                                                                                 | Vitest (`npm test`)                                  | `tsc` y build           | Rust (`cargo test`)                                | Playwright                                                                                                                                                         |
| ------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------- | ----------------------- | -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 0 (`ecae96d`), solo lectura                                   | —                                                                                        | —                                                                                            | Reproducciones en una base temporal (prueba de diagnóstico no versionada) y ensayo de sintaxis de la 0021 propuesta sobre 0001–0020                                              | —                                                    | —                       | —                                                  | —                                                                                                                                                                  |
| Backend D y E, agente BE                                      | `gofmt` sin diferencias; `go vet` con y sin `integration`                                | `ok`: operations, administration, payments, kitchen, inventory, reports, ordering y database | `database` 7.3 s, `administration` 66.2 s, `reports` 125.2 s, `reports/operational` 41.9 s, `operations` 262.0 s; sobre el árbol final, `operations` 256.1 s y `reports` 125.3 s | `src/services` y `tablePanel.test.tsx` (110 pruebas) | `tsc` sin errores       | 41 aprobadas; `rustfmt --check` limpio             | — (la suite completa quedó para la interfaz y la regresión)                                                                                                        |
| Backend, cierre (`b84e3be`)                                   | `sync --check` «Metadatos coherentes»; gofmt limpio; `go vet`                            | 22 paquetes `ok`                                                                             | `database` 7.6 s                                                                                                                                                                 | 68 archivos, 637 pruebas                             | `tsc` sin errores       | 41 aprobadas, 1 ignorada                           | —                                                                                                                                                                  |
| A, agente (`ecae96d` + A)                                     | —                                                                                        | No aplica (sin Go)                                                                           | No aplica                                                                                                                                                                        | 70 archivos, 666 pruebas                             | Sin errores; build `ok` | No aplica (sin Rust)                               | Chromium 16/16 (6.0 min) y WebKit 16/16 (6.1 min); la primera corrida de WebKit, 12/16 (ver abajo)                                                                 |
| E, agente (`4abe78d` + E)                                     | Prettier sin diferencias en sus archivos                                                 | No aplica (sin Go)                                                                           | No aplica                                                                                                                                                                        | 72 archivos, 688 pruebas                             | Sin errores; build `ok` | —                                                  | Chromium 17/17 (6.8 min) y WebKit 17/17 (6.7 min); tras dos ajustes menores, la especificación nueva 1/1 por motor (46.6 s y 45.3 s)                               |
| E, cierre (`2c00064`)                                         | Prettier `ok`                                                                            | —                                                                                            | —                                                                                                                                                                                | 72 archivos, 688 pruebas                             | Sin errores; build `ok` | —                                                  | —                                                                                                                                                                  |
| B, agente BC (`2c00064` + B)                                  | —                                                                                        | No aplica                                                                                    | No aplica                                                                                                                                                                        | 73 archivos, 693 pruebas                             | Sin errores; build `ok` | —                                                  | `workflow-stage9c-keyboard`, `-navigation`, `workflow-stage9d-cancel-account` y `-colors`: 4/4 en Chromium (1.5 min) y en WebKit (1.4 min)                         |
| C, agente BC (`2c00064` + B + C)                              | —                                                                                        | No aplica                                                                                    | No aplica                                                                                                                                                                        | 74 archivos, 714 pruebas                             | Sin errores; build `ok` | —                                                  | Suite completa al final de B y C: Chromium 18/18 (7.0 min) y WebKit 18/18 (6.9 min); la primera corrida de WebKit, 12/17 (ver abajo)                               |
| C, cierre (`ffcaed7`)                                         | Prettier `ok`                                                                            | —                                                                                            | —                                                                                                                                                                                | 74 archivos, 714 pruebas                             | Sin errores; build `ok` | —                                                  | —                                                                                                                                                                  |
| Correcciones de la revisión, parte 1, agente Fix1 (`d723e92`) | `gofmt -l` sin salida; `go vet` con y sin `integration`; Prettier limpio en sus archivos | `ok`: operations, inventory, reports, kitchen y payments                                     | `database` 7.0 s, `reports` 125.1 s, `operations` 251.4 s (repetida sobre el código final: 254.8 s)                                                                              | 74 archivos, 719 pruebas                             | Sin errores; build `ok` | 41 aprobadas, 1 ignorada; `rustfmt --check` limpio | `workflow-stage9d-cancel-account` y `-colors`: 2/2 en Chromium (1.3 min) y en WebKit (1.2 min)                                                                     |
| D (interfaz), agente D (`d723e92` + D)                        | Prettier sin diferencias en sus 29 archivos                                              | No aplica (sin Go)                                                                           | No aplica                                                                                                                                                                        | 75 archivos, 738 pruebas                             | Sin errores; build `ok` | No aplica (sin Rust)                               | Suite completa: Chromium 19/19 (7.4 min) y WebKit 19/19 (7.3 min); la primera corrida de WebKit falló (ver abajo); la especificación nueva repetida, 1/1 por motor |
| D, cierre (`81ff531`)                                         | Prettier `ok`                                                                            | —                                                                                            | —                                                                                                                                                                                | 75 archivos, 738 pruebas                             | Sin errores; build `ok` | —                                                  | —                                                                                                                                                                  |

«—»: la parte no registró esa suite (no cambiaba esa capa o la cubrió otra). Las pruebas de integración se ejecutaron con
`-tags=integration`; los tiempos son los de `go test`. Las fases A, B, C, D y E (interfaz) no cambiaron Go ni Rust.

- **0**: verificación de solo lectura contra el código y consultas de solo lectura a la instalación real (0 estaciones,
  rutas e impresoras; 4 liberaciones bloqueadas por «Sin ruta de preparación», 23 advertencias y 15 unidades activas sin
  liberar; 8 cuentas abiertas: #1 y #3 de mesa con una partida de borrador, #5, #8, #9 y #10 para llevar vacías, #2 y #7
  con productos). En una base temporal se reprodujo que «Liberar mesa» y «Anular cuenta vacía» rechazan las #1 y #3, que
  sin configuración la liberación queda bloqueada y configurar después la cocina no la reintenta, y que Go ya acepta
  `items` al abrir una cuenta para llevar. Decisiones del coordinador 1–11 en el avance.
- **Backend D y E** (`b84e3be`): integración `internal/database` con `TestStage9DAccountCancellationUpgrade` (la 0021
  sobre un esquema 20 con datos), `internal/administration` (subtests `9D_umbral_lectura_y_permiso`,
  `9D_umbral_validacion_revision_y_auditoria`, `9D_umbral_en_el_mapa_y_tablero_para_llevar` y
  `9D_permiso_cancelar_cuentas`), `internal/reports` (`TestStage9DCancelledAccountsReports` y la prueba de la 9C ajustada)
  y `internal/operations` (aceptación encadenada completa más `TestStage9DAccountCancellation`: catálogo de motivos;
  cancelar con permiso propio con cocina, inventario con merma, mesa libre e idempotencia; con supervisor y sus rechazos;
  pagos parciales, descuentos y motivos inválidos; descartar cuentas vacías y con borrador; creación con el primer
  producto sin mesa y para llevar; umbral de cuenta abierta). Rust: la prueba de las siete rutas nuevas
  (`stage9d_account_cancellation_routes_are_closed_and_keep_write_methods`).
- **A**: Vitest `src/styles.test.ts` (colores escritos, valores de la marca, contraste AA de 29 pares y 3:1 de 9 en ambos
  temas, sin verde, uva frente a azul, asignación de la paleta, avisos, importes, destructivos, sección activa, cursores y
  selección) y `src/components/stage9dStyles.test.tsx` (avisos por tipo con su icono, estados de mesa con icono en
  mosaico, leyenda y panel, destructivos de `ConfirmDialog`, `ChoiceDialog` y `Form` con `aria-busy`, menú «Más…» y
  recorrido del código). Playwright `workflow-stage9d-colors.spec.ts` (colores calculados contra los tokens en ambos
  temas, iconos de los cinco estados, cursor `not-allowed` en «Cobrar» deshabilitado y `progress` con `aria-busy`, ocupada
  seleccionada con contorno de 4 px, saldo en el color del texto, «Liberar mesa» y su confirmación en rojo, los cuatro
  avisos y la barra en cuatro estados) con 48 capturas en claro y oscuro a 1280 × 720 y 1920 × 1080. **Primera corrida de
  WebKit**: falló `workflow-stage9c-keyboard` al cerrar «Acerca de» con Escape y, por el reinicio del trabajador, las
  tres siguientes (12/16); la segunda corrida completa, con la versión final, aprobó todo en ambos motores. La causa se
  corrigió en la fase B.
- **E**: Vitest `pages/restaurant/cancelAccount.test.tsx` (10: permiso propio con importe de Go, confirmación roja y mesa
  libre; supervisor desde el principio con la contraseña retirada y el rechazo en su formulario; «Otro» sin descripción y
  el 422 junto al campo; bloqueo por pagos con «Ir a devoluciones» y sin permiso de devoluciones; bloqueo por ajustes;
  para llevar sin productos enviados; reintento incierto con el mismo `operation_id`; reintento incierto con supervisor que
  pide repetir la contraseña; cuenta que cambió en otra terminal) y `pages/stage9dCancelAdmin.test.tsx` (12: catálogo de
  motivos, umbral, registro de la cuenta cancelada con y sin supervisor, historial de `account.cancelled` y
  `account.discarded`, efecto en cocina, filtro «Canceladas o descartadas» y recorrido sin textos de «anular» cuentas).
  Playwright `workflow-stage9d-cancel-account.spec.ts` (datos reales de Go): bloqueo por un abono real, cancelación con
  permiso propio («Cliente se retiró sin pagar», 3 unidades enviadas, Mesa E1 libre), cancelación por un usuario sin
  `accounts.cancel` con la autorización del administrador («Otro» con descripción, Mesa E2 libre), registro con quién
  canceló y quién autorizó, «Cuentas canceladas» con «Autorizó» y «Cancelaciones por motivo» en pantalla y exportados a
  PDF y Excel por la operación cerrada simulada, motivo nuevo con la marca de merma y umbral guardado (la prueba lo deja
  en 12 h). 56 capturas sin desbordamiento horizontal. Comprobación adicional sobre el árbol con los cambios en curso de
  BC: `tsc` sin errores y `npm test` 74 archivos, 714 pruebas.
- **B**: Vitest `navigation/navigation.test.tsx` (orden de las cuatro secciones, Caja a la derecha antes del servidor, el
  usuario y pantalla completa, Caja en dos renglones en sus tres estados, Caja sin turno visible, «Sin conexión»),
  `App.test.tsx` («Sin conexión» y recuperación), `src/stage9dLayout.test.ts` y una prueba nueva del foco de «Acerca de» en
  `commands/commandCenter.test.tsx`. Playwright `workflow-stage9d-topbar-tables.spec.ts` (orden, mismo ancho ±1 px, Caja a
  la derecha en dos renglones, alturas de 52–56 px, ningún texto cortado ni desbordamiento a 1280 × 720 y 1920 × 1080 en
  ambos temas, «En línea» e indicador del turno en azul, menú de Caja dentro de la ventana, «Sin conexión» en rojo); se
  actualizaron `e2e/navigation.ts` y `workflow-stage9c-navigation.spec.ts`.
- **C**: Vitest `pages/restaurant/stage9dTables.test.tsx` (16: formato «d h», escala que ve todas las mesas, mínimo de
  64 px con desplazamiento, tope del doble y centrado, leyenda dentro del marco, diseñador proporcional, advertencia por
  `long_open`, resumen por estado con resaltado y acciones rápidas, panel siempre visible, un solo principal, «Abierta
  hace 3 d 14 h» en ámbar, sin «Actualizar» y «Reintentar» solo sin conexión) y las reglas de Mesas de
  `src/stage9dLayout.test.ts`. Playwright en `workflow-stage9d-topbar-tables.spec.ts`: panel de 360 px con y sin mesa, mapa
  y panel a la misma altura, resumen y resaltado, escala automática con la misma escala (una mesa redonda sigue redonda),
  35 mesas que a 1280 × 720 no caben con 64 px y se desplazan arrastrando sin elegir mesa (y caben a 1920 × 1080), «3 d
  14 h» y ámbar en la mesa, el panel y la tarjeta, botones principales, sin «Actualizar», «Reintentar» sin conexión, el
  diseñador que mueve y guarda una mesa y «Nuevo pedido para llevar» del resumen. **Primera corrida de WebKit** (con la
  fase E aún sin commit): la misma falla de `workflow-stage9c-keyboard` y cuatro en cascada (12/17); con el foco del
  diálogo corregido en la fase B, la segunda corrida y la final aprobaron todo en ambos motores.
- **Correcciones de la parte 1**: `TestStage9DAccountCancellation` ampliada (rechazo `ACCOUNT_DISCARDABLE` de una cuenta
  para llevar vacía y de una de mesa solo con borrador, con permiso propio, sin permiso, con contraseña incorrecta que no
  cuenta en el límite de intentos y con «Otro» sin descripción, sin escribir nada; subprueba nueva con una estación
  inactiva: respuesta, lectura y reintento idempotente en `change_pending` y, tras reactivar y reintentar cocina, un solo
  aviso y todo cancelado; nombre del supervisor en el evento; preclasificación en `GET inventory/cancellations` y su paso a
  «Clasificada por el encargado» al corregirla). Sin las correcciones, la subprueba de cocina y la de descarte fallan
  (comprobado). Vitest: detector de los 148 nombres de color, etiquetas destructivas por propiedad, `RemoveButton`
  secundario y la preclasificación en Ajustes y mermas. Rust: `restaurant_void` sin ruta.
- **D** (creación diferida, descartar cuentas vacías, tarjetas compactas, textos del detalle, menú por producto y
  pestañas de Mesas): Vitest nuevo `pages/restaurant/stage9dAccounts.test.tsx` (18: «Nuevo pedido» sin crear nada y salir
  sin dejar nada; el pedido nace con el primer producto —para llevar, sin mesa, origen «Para llevar»— y el siguiente toque
  guarda el borrador del pedido real; «Cuenta sin mesa» desde el resumen y desde Cuentas; rechazo de Go al crear
  explicado sin código interno; «Nuevo pedido para llevar» del resumen que respeta `requestScreen`; paleta y menú nativo;
  descartar en Mesas —botón y «Más…», sin «Liberar mesa» ni «Cancelar cuenta», confirmación roja, cuerpo solo con
  revisión y `operation_id`, mesa libre—; una cuenta con productos enviados conserva «Cancelar cuenta»; Para llevar con
  un solo principal y el listado que pasa al siguiente; descarte dentro de la captura y con toques sin guardar; tarjetas
  de dos renglones y pestañas de 44 px; textos de cocina una vez por producto y no en el encabezado; desglose sin versión
  ni descuento en cero; combos sin nombres repetidos; `choiceLabel`, `chosenComplements` y `distinctWarnings`; menú por
  producto —renglón, «⋯», Escape, tocar fuera, uno a la vez, «Cancelar unidades» rojo— y productos nuevos con − 1 + y
  «Personalizar») y `draftSync.test.ts` (+1: pedido y cuenta sin mesa en el motor); ajustadas `capture.test.tsx`,
  `tablePanel.test.tsx` y `stage9dTables.test.tsx`. Playwright nuevo `workflow-stage9d-accounts.spec.ts` (datos reales de
  Go): «Nuevo pedido» y «Cuenta sin mesa» sin productos no crean nada (consultado en Go antes y después) y con el primero
  nacen sin mesa; descarte desde «Más…» de la captura, desde el detalle de Para llevar y de Cuentas y desde el panel de
  una mesa con solo borrador, sin motivo, cuenta en `voided` y **ninguna en «Cuentas canceladas»**; la mesa con un
  producto enviado conserva «Cancelar cuenta»; tarjetas de 44–64 px, cinco o más a la vista a 1280 × 720 y las ocho a
  1920 × 1080; «Enviado · sin confirmar en cocina» con un producto sin ruta de preparación (liberación bloqueada) una vez
  y no en el encabezado; menú por producto con «Cancelar unidades» rojo en una línea; pestañas de 44 px. Se actualizaron
  `restaurant.spec.ts`, `stock-consumption.spec.ts`, `table-price-decrease.spec.ts`, `takeaway-quick-notes.spec.ts` y
  `workflow-stage9d-topbar-tables.spec.ts`. 32 capturas (Chromium y WebKit, claro y oscuro, 1280 × 720 y 1920 × 1080)
  sin desbordamiento horizontal. **Primera corrida de WebKit** (código previo a las observaciones O1 y O2): falló al
  inicio de `stock-consumption` (el clic en «Administración» no navegó, antes de cualquier paso de la fase D) y, por el
  reinicio del trabajador, las 16 siguientes no encontraron la base preparada; la repetición de WebKit aprobó 19/19 y la
  corrida final también. La especificación nueva se ordena antes de `workflow-stage9d-cancel-account` y usa nombres
  propios (Salón D, Mesas D1–D3, Clientes D1–D7) para no cruzarse con las demás.

## Revisión independiente

Un agente de solo lectura revisó la etapa en dos partes, en copias exactas, con reproducciones propias que no se
versionaron.

**Parte 1** (`ecae96d..2c00064`: backend D y E, fase A y fase E). **Sin defectos críticos ni altos**; uno medio y siete
bajos. Salida del revisor (copia de `2c00064`): `gofmt -l` sin salida; `go vet` con y sin `integration` correcto;
`go test -race ./...` todos los paquetes `ok`; integración `database` 8.2 s, `reports` 131.7 s, `administration` 67.2 s y
`operations` 262.6 s `ok`; su reproducción `TestReview9D` PASS; `tsc` sin errores; `npm test` 72 archivos y 688 pruebas;
build correcto; `cargo test` 41 aprobadas y 1 ignorada; Playwright Chromium con las dos especificaciones de la 9D, 2/2.
**WebKit no lo ejecutó.**

| Hallazgo                                                                                                               | Corrección (`d723e92`)                                                                                                                                       |
| ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| M1 · medio · una cuenta vacía o solo con borrador se podía «cancelar» y quedaba en los reportes como cancelación de $0 | Go responde `409 ACCOUNT_DISCARDABLE` antes del motivo y del supervisor; la fase D (`81ff531`) ofrece «Descartar cuenta vacía» en lugar de «Cancelar cuenta» |
| B1 · la preclasificación aparecía como «Clasificada por el encargado» a nombre de quien canceló                        | `preclassified` y «Preclasificada al cancelar la cuenta»; en Ajustes y mermas, «Clasificó: Al cancelar la cuenta»                                            |
| B2 · la respuesta y el resultado idempotente se armaban antes de integrar cocina                                       | Se leen después de cocina (`change_pending` con una estación inactiva)                                                                                       |
| B3 · se pintaban de rojo acciones que se pueden deshacer                                                               | Los «Quitar» reversibles vuelven al estilo secundario, con una lista justificada en la prueba                                                                |
| B4 · la ruta anterior `void` seguía en el puente                                                                       | `restaurant_void` salió de `api.rs` y de `services/api.ts`; Go conserva la ruta                                                                              |
| B5 · el historial mostraba el usuario del supervisor, no su nombre                                                     | El evento guarda `authorized_by_name`                                                                                                                        |
| B6 · las unidades canceladas una por una antes de «Cancelar cuenta» no cuentan para el motivo                          | Sin cambio de cálculo (decisión del coordinador); documentado en [REPORTS.md](REPORTS.md)                                                                    |
| B7 · huecos en las pruebas de la fase A (nombres de color y etiquetas por propiedad)                                   | 148 nombres de color y colores del sistema; `confirmLabel` y `label` revisados                                                                               |

**Parte 2** (`2c00064..d723e92`: fases B y C y las correcciones). **Sin defectos críticos, altos ni medios**; M1, B1, B2,
B3, B4, B5 y B7 resueltos y B6 como nota. Salida del revisor (copia de `d723e92`): `gofmt -l` sin salida; `go vet` con y
sin `integration` correcto; `go test -race` de operations, inventory, kitchen y reports `ok`; integración `operations`
259.0 s `ok`; `TestReview9D` actualizada PASS; `tsc` sin errores; `npm test` 74 archivos y 719 pruebas; build correcto;
`cargo test` 41 aprobadas y 1 ignorada; Playwright con `workflow-stage9c-keyboard`, `-navigation`, las tres
`workflow-stage9d-*` y su sonda propia: **6/6 en Chromium y 6/6 en WebKit**. Observaciones bajas: **O1**, «Nuevo pedido
para llevar» del resumen de Mesas no esperaba el resultado de `requestScreen` (hoy sin efecto); **O2**, las pestañas
«Mesas / Cuentas / Áreas y distribución» medían 38 px (anterior a la 9D). Pendiente para la fase D: ofrecer «Descartar»
en las cuentas `discardable` y ocultar o explicar «Cancelar cuenta» en ellas. **Las tres cosas las atendió la fase D**
(`81ff531`): el resumen espera `requestScreen` (que devuelve `Promise<boolean>`), las pestañas miden 44 px y las cuentas
descartables ofrecen «Descartar cuenta vacía» en lugar de «Cancelar cuenta». La revisión de la fase D (parte 3) estaba
en curso al escribir este documento.

## Cobertura de la etapa 9D

Correspondencia con las pruebas de §7 de la especificación. Rutas relativas a `desktop/src/`, `desktop/e2e/` o
`backend/internal/` según la capa.

| Prueba exigida (§7)                                                          | Dónde y qué                                                                                                                                                                                                                                                                                                                                |
| ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Panel de mesas siempre visible                                               | `pages/restaurant/stage9dTables.test.tsx` y `workflow-stage9d-topbar-tables.spec.ts` (360 px con y sin mesa, a la altura del mapa).                                                                                                                                                                                                        |
| Mapa con ajuste de escala y desplazamiento                                   | `stage9dTables.test.tsx` (escala con todas las mesas, mínimo de 64 px, tope del doble) y Playwright (35 mesas que se desplazan arrastrando a 1280 × 720 y caben a 1920 × 1080; arrastre con el ratón, no con el dedo).                                                                                                                     |
| Nuevo pedido para llevar sin productos que no deja nada guardado             | Go: `TestStage9DAccountCancellation` («creación con el primer producto sin mesa y para llevar»). Interfaz: `stage9dAccounts.test.tsx` y `workflow-stage9d-accounts.spec.ts` (nada en Go antes del primer producto; también «Cuenta sin mesa»).                                                                                             |
| Descartar cuentas vacías                                                     | Go: `TestStage9DAccountCancellation` («descartar cuentas vacías y con borrador», `ACCOUNT_DISCARDABLE`) y `TestStage9DCancelledAccountsReports` (los descartes no aparecen). Interfaz: `stage9dAccounts.test.tsx` y `workflow-stage9d-accounts.spec.ts` (desde «Más…», el detalle y el panel de la mesa; ninguna en «Cuentas canceladas»). |
| Tarjetas compactas                                                           | `stage9dAccounts.test.tsx` (dos renglones) y `workflow-stage9d-accounts.spec.ts` (44–64 px; cinco o más a la vista a 1280 × 720 y las ocho a 1920 × 1080).                                                                                                                                                                                 |
| Menú por producto                                                            | `stage9dAccounts.test.tsx` (renglón, «⋯», Escape, tocar fuera, uno a la vez, «Cancelar unidades» rojo) y `workflow-stage9d-accounts.spec.ts` (con el ratón de Playwright, no con el dedo).                                                                                                                                                 |
| Barra superior a 1280 px                                                     | `navigation/navigation.test.tsx`, `stage9dLayout.test.ts` y `workflow-stage9d-topbar-tables.spec.ts` (sin cortes ni desbordamiento a 1280 × 720 y 1920 × 1080 en ambos temas, «Sin conexión» incluido).                                                                                                                                    |
| Cursor `not-allowed` en controles deshabilitados                             | `styles.test.ts` (reglas) y `workflow-stage9d-colors.spec.ts` (estilo calculado en Chromium y WebKit; `progress` solo con `aria-busy`).                                                                                                                                                                                                    |
| Cancelar cuenta con y sin permiso (supervisor)                               | `TestStage9DAccountCancellation` (permiso propio; supervisor con sus rechazos: sin autorización, autoautorización, contraseña incorrecta, límite de intentos, sin `accounts.cancel`), `cancelAccount.test.tsx` y `workflow-stage9d-cancel-account.spec.ts` (usuario real sin el permiso).                                                  |
| Con productos preparados y no preparados                                     | `TestStage9DAccountCancellation`: con «Control de inventario» activado, el motivo con marca de merma preclasifica «Preparado / merma» y el que no la tiene deja «por clasificar»; lo no liberado se cancela sin papel y lo liberado recibe el aviso de cancelación.                                                                        |
| Con pagos parciales (debe exigir la devolución)                              | `TestStage9DAccountCancellation` («pagos parciales, descuentos y motivos inválidos»: `ACCOUNT_HAS_PAYMENTS` y `ACCOUNT_HAS_ADJUSTMENTS`), `cancelAccount.test.tsx` y Playwright con un abono real.                                                                                                                                         |
| Motivo «Otro» sin descripción (debe rechazarse)                              | `TestStage9DAccountCancellation` (`422 CANCEL_DESCRIPTION_REQUIRED`) y `cancelAccount.test.tsx` (el 422 junto al campo).                                                                                                                                                                                                                   |
| Exclusión de ventas y «Cuentas canceladas» con «Autorizó» y total por motivo | `TestStage9DCancelledAccountsReports` (excluidas de ventas; «Autorizó»; filtros; total por motivo; PDF y Excel) y `workflow-stage9d-cancel-account.spec.ts` (en pantalla y exportados).                                                                                                                                                    |
| Tiempos: formato «d h» y advertencia a partir del umbral                     | `stage9dTables.test.tsx`; Go: subtests `9D_umbral_*` de `internal/administration` y «umbral de cuenta abierta por mucho tiempo» de `TestStage9DAccountCancellation`; Playwright con `long_open` presentado por el puente.                                                                                                                  |
| Colores: sin colores escritos y capturas en claro y oscuro                   | `styles.test.ts` y `components/stage9dStyles.test.tsx`; capturas de Mesas, Mesas › Cuentas, Para llevar, la barra en cuatro estados y los cuatro avisos en claro y oscuro (`workflow-stage9d-colors`, fase A).                                                                                                                             |
| Migración 0021                                                               | `TestStage9DAccountCancellationUpgrade` (sobre un esquema 20 con datos; no modifica cuentas ni eventos existentes).                                                                                                                                                                                                                        |
| Regresión completa                                                           | «Regresión final».                                                                                                                                                                                                                                                                                                                         |

## Pendiente (pruebas físicas y de entorno)

No se afirma ninguna de estas:

- **Pantalla táctil física**: barra, panel de Mesas, resumen, menú por producto y, en particular, el **arrastre del mapa
  con el dedo** en hardware real (se probó con el ratón de Playwright; en una pantalla táctil el navegador puede tomar el
  desplazamiento por su cuenta).
- **Cursor en la ventana nativa**: que macOS muestre el cursor «no disponible» sobre un control deshabilitado en la
  ventana Tauri (WKWebView); solo se comprobó el estilo calculado del navegador.
- **Impresión real del aviso de cancelación** de una cuenta en la impresora de una estación (solo el transporte
  controlado de la cola).
- **Windows**: nada de la 9D se compiló ni probó en Windows (ver [NOTAS_WINDOWS.md](NOTAS_WINDOWS.md)).
- La ventana nativa en general (barra, colores, Mesas y «Cancelar cuenta»), las barras de desplazamiento siempre visibles
  del mapa en la ventana nativa, dos terminales reales y un lector de pantalla real.
- Pendientes de etapas anteriores sin cambio: papel con formato y logotipo (9B), papel a 48/32 columnas (9C), cajón
  físico, menú nativo real y diálogo nativo real de guardado.

## Qué no se afirma

- Que la escala y el arrastre del mapa se sientan bien con el dedo en una pantalla táctil real.
- Que el aviso de cancelación de una cuenta salga en papel: `submitted` es aceptación del transporte, no papel.
- Que «Esperando pago», «3 d 14 h» y la marca ámbar se hayan visto con datos reales envejecidos: en las capturas los
  presenta el puente de la prueba (Go los calcula y sus pruebas de integración los cubren).

## Regresión final

Ejecutada el 12/11/2026 (14:04–14:30, 1576 s) por un solo agente con el script del coordinador, sobre una copia exacta
de `a79aee0` (`git archive`), con el clúster temporal 55499, Go temporal en 8183 y Vite en 1421; la pantalla del equipo
estuvo desbloqueada durante todo Playwright (comprobado cinco veces), así que WebKit es una aprobación real.

| Suite                                    | Resultado                                                                                                                                                                                                  |
| ---------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato (Prettier, `gofmt`, `cargo fmt`) | aprobado                                                                                                                                                                                                   |
| Metadatos (`sync-metadata --check`)      | aprobado                                                                                                                                                                                                   |
| `go vet` (normal y `-tags=integration`)  | aprobado                                                                                                                                                                                                   |
| Go unitarias con `-race`                 | 22 paquetes `ok`                                                                                                                                                                                           |
| Integración PostgreSQL con `-race`       | 9 de 9 paquetes: `database` 7.2 s, `administration` 64.7 s, `catalog` 8.8 s, `operations` 250.8 s, `payments` 2.4 s, `reports` 121.4 s, `reports/operational` 40.7 s, `designer` 20.4 s, `identity` 17.8 s |
| `tsc --noEmit`                           | sin errores                                                                                                                                                                                                |
| Vitest (`npm test`)                      | 75 archivos, 751 pruebas aprobadas                                                                                                                                                                         |
| `npm run build`                          | correcto (solo el aviso de tamaño de fragmentos ya conocido)                                                                                                                                               |
| `cargo test --locked --offline`          | 41 aprobadas, 1 ignorada                                                                                                                                                                                   |
| Playwright Chromium                      | 19 de 19 aprobadas (7.4 min)                                                                                                                                                                               |
| Playwright WebKit                        | 19 de 19 aprobadas (7.3 min)                                                                                                                                                                               |
| `.app` (`tauri build --bundles app`)     | 0.9.3, Mach-O arm64, 10.02 MiB                                                                                                                                                                             |

Sin fallos ni repeticiones. Las mediciones de volumen y la impresión física no corren por omisión (por diseño).

## Instalación real

La 0021 y la 0.9.3 se aplicaron a la instalación real el 12 de noviembre de 2026 (respaldo `backups/adel_pos-20261112T203123Z-ec18a4.dump` con restauración verificada: 132 tablas idénticas; `migrate` `applied:1` y repetición `applied:0`; readiness 200 con esquema compatible; servidor y `.app` 0.9.3). Detalle y
comprobaciones de solo lectura antes y después en [ETAPA_9D_AVANCE.md](ETAPA_9D_AVANCE.md), «Instalación real».
