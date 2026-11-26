# Avance · Etapa K1.1: pedidos mixtos, productos sin ruta y configuración guiada de cocina

Especificación del propietario: [docs/etapas/ETAPA_K1_1_COCINA_AJUSTES.md](etapas/ETAPA_K1_1_COCINA_AJUSTES.md)
(solo lectura). Rama `etapa-k1-1`, creada el 26/11/2026 desde `main` (`310d7d7`: servidor 0.16.0, esquema 29, meseros
0.4.1 y cocina 0.1.0, con la K1 fusionada). La 0.16.0 ya está instalada en la Mac del propietario (la aplicación de
`/Applications` dice 0.16.0), así que la `0029` es inmutable y la migración de esta etapa es una `0030` nueva.

Forma de trabajo: la de `CLAUDE.md` §0 y la §0 de la especificación (un agente en la Fase 0, como mucho dos a la vez,
pruebas aisladas en el clúster temporal 55499 y Go temporales, nunca la instalación real ni `licensing.sicrat.com`).
Los guiones de reproducción, los parches y los registros de las pruebas viven en el scratchpad de la sesión, fuera del
repositorio (`$SP/k11/`).

## Fase 0 · Diagnóstico con los casos del propietario

Árbol `$SP/wt/k11-p0`, rama `etapa-k1-1-p0` (desde `etapa-k1-1` `9139759`), `ISO_TAG=P0`, Go temporal en 8201/8461 con
la confianza de prueba de licencias (`ISO_LICENSE_TRUST` = `contracts/licencias/v1/fixtures/vectors.json`). Sin código
del producto, sin commits, sin emuladores. La especificación quedó intacta (md5 `9363961a…` antes y después).

### Qué se hizo

1. **Reproducción por la API** (`$SP/k11/repro/reproducir.py`, salida en `$SP/k11/repro/salida.txt`): base nueva,
   licencia de prueba firmada por `license-test` con los vectores (no se imprime), el catálogo y la configuración del
   propietario, **una pantalla de cocina emparejada por la API** (solicitud, revelado, código de 6 dígitos con la
   huella SPKI del certificado, aprobación, reclamo y `connect`), los cuatro pedidos y casos adicionales.
2. **Prueba automática de Go** que expresa lo que pide la §2/§6: `backend/internal/operations/stagek11_unrouted_test.go`
   (`TestStageK11UnroutedProducts`), guardada como parche en `$SP/k11/repro/prueba.patch` (sin confirmar; el árbol
   queda limpio). Hoy **falla en los casos 3 y 4** (y en las dos subpruebas que usan un pedido mixto).
3. **Lectura del código** de la causa, del panel, de lo que no necesita preparación, del aviso al mesero, de la tarjeta
   del KDS, de los cierres de cuenta, de la aplicación de cocina y del escritorio; propuesta para las fases A, B y C.

### 1. Reproducción

#### Los casos del propietario (Go temporal, API real)

| Pedido                        | Envío (`confirm`)                          | `kitchen_releases`                                            | Unidades liberadas / tareas | `kitchen-display/orders`        | Panel del escritorio (`GET kitchen/board` → `releases`) |
| ----------------------------- | ------------------------------------------ | ------------------------------------------------------------- | --------------------------- | ------------------------------- | ------------------------------------------------------- |
| Solo machacado                | 200, `released`                            | `released`, `[]`                                              | 1 / 1                       | tarjeta: 1× Machacado           | no aparece                                              |
| Machacado + hamburguesa       | 200, `released`                            | `released`, `[]`                                              | 2 / 2                       | tarjeta: Hamburguesa, Machacado | no aparece                                              |
| Machacado + Coca Cola         | 200, `pending_release` (los dos renglones) | **`blocked`**, `["Sin ruta de preparación: Coca Cola 600ml"]` | **0 / 0**                   | **sin tarjeta**                 | «Bloqueado por configuración»                           |
| Machacado + Coca Cola + papas | 200, `pending_release` (los tres)          | **`blocked`**, dos advertencias «Sin ruta…»                   | **0 / 0**                   | **sin tarjeta**                 | «Bloqueado por configuración»                           |

El envío **no falla** (200): la cuenta queda «Enviado · sin confirmar en cocina» y la pantalla nunca recibe nada. La
hipótesis del propietario es correcta.

#### Otros comportamientos reproducidos (misma salida)

- **«Liberar a preparación» sin corregir nada** (caso 3): `POST kitchen/releases/{id}/release` responde **200** con el
  pedido todavía `blocked` (revisión 2 → 3) y deja `release.release` en el historial. El escritorio muestra «Pedido #3
  liberado a preparación» con cualquier 200 (`PreparationBoard.tsx:271-279`): **dice que lo liberó aunque siga
  bloqueado**.
- **Cobrar el caso 3** (pago total en efectivo): la cuenta queda `closed` y el pendiente **sigue `blocked` en el panel**.
- **Cancelar la cuenta del caso 4**: la liberación pasa a `cancelled` y sale del panel, **sin ningún registro en
  `kitchen_history`**.
- **Cancelar una cuenta con algo ya en cocina cuya estación se desactivó** (más un segundo envío bloqueado por
  «Estación inactiva: Barra»): la cuenta queda `voided` con `preparation_state: change_pending`, el envío bloqueado
  **no se cancela** y el panel conserva **los dos**: «Bloqueado por configuración» y «Aviso de cancelación pendiente»
  («Aviso pendiente para estación inactiva: Cocina»). Contradice la §3 (una cuenta cancelada no deja nada).
- **Ruta «Sin preparación» para Refrescos** (lo que hoy evita el bloqueo): machacado + Coca Cola se libera (2 unidades
  liberadas, 1 tarea); la tarjeta solo tiene el machacado; el renglón de la Coca Cola dice `ready` desde el envío; al
  marcar el machacado en la pantalla la tarjeta se cierra, la cuenta queda `ready` y **un solo aviso** al mesero (el
  machacado).
- **Ruta a una estación desactivada** (Extras → Barra, Barra desactivada): machacado + papas queda `blocked`
  («Estación inactiva: Barra») y **tampoco llega el machacado**.
- **Para llevar programado al que se le quita la hora**: la liberación pasa de `scheduled` a `pending` y **nadie la
  libera** (el planificador solo mira las `scheduled`); el panel la muestra como «Pendiente de liberación».

#### Prueba automática (`TestStageK11UnroutedProducts`, paquete `operations`)

Usa las ayudas de la K1 (`kitchenDisplayFixture`, `kitchenDevice`, `orders`, `card`, `mark`, `notices`) y su propia base.
Comprueba, con la configuración del propietario:

1. los cuatro pedidos llegan a la pantalla con exactamente lo que tiene ruta (Coca Cola y papas no aparecen), ninguna
   liberación queda sin liberar, **todas las unidades se liberan una vez** (también las que no necesitan preparación,
   sin tarea, como hoy una ruta «Sin preparación») y ninguno queda en el panel;
2. la tarjeta de machacado + Coca Cola + papas sigue abierta y sin aviso hasta marcar el machacado; al marcarlo, la
   tarjeta se cierra, sale **un** aviso al mesero y la cuenta queda `ready`;
3. con Cocina en «Impresora y pantalla», la comanda impresa de machacado + Coca Cola lleva solo el machacado y hay un
   trabajo de impresión.

Resultado hoy: **falla** en «machacado + Coca Cola», «machacado + Coca Cola + papas» y en las subpruebas 2 y 3 (todas con
«la pantalla de cocina no recibió el pedido; liberaciones: [blocked …]»); pasan los casos 1 y 2
(`$SP/k11/repro/prueba-salida.txt`).

**Experimento temporal (revertido)** para comprobar que la prueba pide lo correcto: quitando solo la advertencia «Sin
ruta de preparación» de `routing.go:137-139`, la prueba **pasa completa** (`prueba-experimento.txt`). Se restauró el
archivo con `git checkout` y se comprobó su md5. Es decir: «sin ruta = no necesita preparación» ya da, con el código de
hoy, la tarjeta correcta, el aviso correcto, «Listo» correcto y la comanda correcta.

### 2. La causa, con el código

**Se confirma la hipótesis.** La regla está en `backend/internal/kitchen/releases.go`, `prepareRelease` (84-189):

- recorre las unidades activas no liberadas, resuelve la ruta de cada una con `resolveRouting` (`routing.go:104-184`) y
  **junta todas las advertencias del envío** (`releases.go:120` y `:131-133`);
- con **una sola** advertencia, `UPDATE … SET state='blocked', warnings=…` y `return` (`releases.go:138-142`): **ninguna**
  unidad del envío se libera, aunque las demás tengan ruta.

Advertencias que la disparan:

| Advertencia                           | Dónde                            | Cuándo                                                                                                                                                                                                                                                                 |
| ------------------------------------- | -------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| «Sin ruta de preparación: X»          | `routing.go:137-139`             | producto sin ruta propia, sin ruta de su categoría y sin estaciones heredadas de su combo, **sin componentes activos** (un producto simple, o un combo vacío: «un combo vacío sin ruta tampoco se interpreta como exención», `KITCHEN_AND_PRINTING.md:40`)             |
| «Estación inactiva: X»                | `routing.go:166-168`             | la ruta (o la heredada del combo) incluye una estación desactivada; basta una de varias                                                                                                                                                                                |
| «Impresora inactiva: X»               | `releases.go:124-135`            | una estación `printer` o `both` de la ruta tiene su impresora desactivada                                                                                                                                                                                              |
| Combos                                | `routing.go:120-123`, `:171-177` | un combo **con** componentes y sin ruta propia no advierte: no tiene parte y cada componente se resuelve con su propia ruta (o advierte él)                                                                                                                            |
| «Destino inválido» / estación borrada | —                                | no ocurre: `kitchen_route_stations.station_id` tiene FK (`0005:66-70`) y solo «Quitar datos de prueba» borra estaciones, sin tareas y con sus filas de ruta (`maintenance/apply.go:70-77`); un id inexistente haría fallar la transacción (`one`, `context.go:98-101`) |

En los cambios de algo ya liberado (`changes.go:26-129`) las mismas advertencias de los componentes nuevos (con la ruta
congelada, `:73`) y **«Aviso pendiente para estación inactiva: X»** (la estación que recibió la comanda está
desactivada, `:85-93`) dejan el cambio en `kitchen_pending_changes` («Modificación pendiente» / «Aviso de cancelación
pendiente», `:96-111`).

Qué pasa con un envío bloqueado (confirmado en la reproducción):

- **Unidades**: siguen activas sin fila en `kitchen_unit_releases` (`releases.go:152-157` no corre); **sin tareas**, sin
  comanda, sin trabajo de impresión ni evento `kitchen.released`.
- **Inventario**: `inventory.RecordReleases` (`releases.go:172-178`) no corre: **lo bloqueado nunca consume
  existencias**, ni siquiera lo que tenía ruta. Tampoco aparece en los reportes que leen la liberación canónica
  (`reports/kitchen.go`, `reports/products.go`).
- **Preparación**: `kitchen.VersionPreparation` devuelve `pending_release` («Enviado · sin confirmar en cocina»,
  `states.go:223-234`).
- **No se reintenta sola**: el planificador solo libera las `scheduled` automáticas (`worker.go:73-75`); configurar
  después la ruta no hace nada hasta «Liberar a preparación». Es la regla de la etapa 5 («nunca un envío aparente»,
  `KITCHEN_AND_PRINTING.md:40`) y lo que la 9D documentó (`:333-346`).

### 3. El panel «Pendientes de liberación y avisos»

**Quién lo llena**: `GET kitchen/board` (`kitchen.states.read`) devuelve `releases` (estados `pending`, `scheduled` y
`blocked`, sin filtrar por el estado de la cuenta, hasta 100; `reads.go:163-166`) y `pending_changes` (todos, hasta
100; `reads.go:170`), **solo a quien tiene `kitchen.stations.all`** (`reads.go:157-174`). El escritorio lo muestra si hay
alguno (`PreparationBoard.tsx:141-149`, componente `PendingReleases` en `:209-327`).

#### Qué casos lo llenan (en lenguaje sencillo)

| Lo que dice hoy                                                                                                                                      | Qué es                                                                                                                 | Cuándo sale                                                                                                                                                                                                        | Cuándo se quita                                                                                                                                                             |
| ---------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **«Bloqueado por configuración»** + renglones «Sin ruta de preparación: …», «Estación inactiva: …», «Impresora inactiva: …» (`:245-246`, `:262-264`) | Un envío que la cocina **nunca recibió** porque algo de él no tenía a dónde ir.                                        | Al enviar, si **un** producto no tiene ruta, va a una estación desactivada o su impresora está desactivada.                                                                                                        | Solo si alguien lo libera después de corregir, si se cancelan todas sus unidades (cancelar partidas o la cuenta) o si se le pone una hora. **Cobrar y cerrar no lo quita.** |
| **«Programado»** + «Programado: …» y «Libera automáticamente/manualmente: …» (`:247-260`)                                                            | Un pedido para llevar con hora: la cocina lo recibe a su hora (automático) o cuando alguien lo libera (manual).        | Al enviar un pedido con hora, o al «Ajustar programación» de cualquier pendiente.                                                                                                                                  | Al liberarse (a su hora si es automático o a mano), al cancelarse sus unidades o al quitarle la hora (pasa a «Pendiente de liberación»).                                    |
| **«Pendiente de liberación»** (`:249`)                                                                                                               | Un pedido que espera que alguien lo mande a cocina.                                                                    | Al **quitar la hora** a un pedido programado (`releases.go:338-342`, reproducido) y, en una base anterior a la etapa 5, los pedidos que importó la 0005 («Pedido anterior a etapa 5…», `0005:225-236`).            | Solo a mano o cancelando.                                                                                                                                                   |
| **«Aviso de modificación pendiente»** / **«Aviso de cancelación pendiente»** + advertencias (`:293-305`)                                             | Un cambio (modificar, cancelar partidas o «Cancelar cuenta») de algo que la cocina ya tenía, que no se le pudo avisar. | Si la estación que recibió la comanda está desactivada, o si un componente nuevo no tiene ruta o va a una estación desactivada. Mientras tanto no se pueden avanzar esas tareas (`kitchen_changes_test.go:58-65`). | Solo con «Revisar destinos y generar aviso» después de corregir (se **borra** la fila, `changes.go:127`). Cobrar, cerrar o cancelar **no** lo quitan.                       |

#### Qué hace cada botón

| Botón                                                                                                     | Qué hace                                                                                                                                                                                                                                                                                                                                                                        | Ruta                                                                                                       | Permisos                                                                                                             |
| --------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| **«Liberar a preparación»** (`:266-283`)                                                                  | Vuelve a intentar mandar el pedido a cocina con la configuración de **ahora** (`prepareRelease`). Si todavía falta algo, **sigue bloqueado y responde 200** (el escritorio dice «liberado»). Sobre una liberación ya liberada o cancelada no hace nada y también responde 200 (`releases.go:85-87`).                                                                            | `POST kitchen/releases/{id}/release` `{operation_id, revision}`                                            | `kitchen.release` (`handler.go:204-206`) + `kitchen.stations.all` (`releases.go:202-204`; el botón también lo exige) |
| **«Liberar anticipadamente»** (mismo botón, `:269`, `:281`)                                               | Lo mismo cuando la hora de liberación (`due_at`) todavía no llega.                                                                                                                                                                                                                                                                                                              | la misma                                                                                                   | además `kitchen.release_early` (`releases.go:216-221`); sin él, el botón se deshabilita                              |
| **«Ajustar programación»** (`:284-290` → `ScheduleEditor.tsx`)                                            | Abre «Programación de preparación · #folio»: fecha y hora (zona del restaurante), anticipación (0-1440 min) y «Liberar a cocina automáticamente a su hora». Guardar deja el pedido **`scheduled` y borra sus advertencias** (`releases.go:244-246`), aunque sea de una mesa o esté bloqueado: es lo que hace que un bloqueado «desaparezca» para volver a bloquearse a su hora. | `POST kitchen/releases/{id}/schedule` `{operation_id, revision, scheduled_local, automatic, lead_minutes}` | `kitchen.timing.manage` (`handler.go:207-209`) + `kitchen.stations.all`                                              |
| **«Revisar destinos y generar aviso»** (`:306-321`; la documentación lo llama «Revisar cambio pendiente») | Vuelve a procesar el cambio pendiente; si ya no hay advertencias, aplica el cambio a las tareas, imprime/muestra el aviso y borra la fila; si siguen, sube su revisión.                                                                                                                                                                                                         | `POST kitchen/changes/{event_id}/retry` `{operation_id, revision}`                                         | `kitchen.release` (`handler.go:201-203`) + `kitchen.stations.all` (`changes.go:244-246`)                             |

#### Por qué no se quitan los de cuentas cobradas o cerradas, y las canceladas

- **Cobrar y cerrar** pasa por `payments.closeIfSettled` (`payments/collect.go:636-664`; la llaman `collect.go:603`,
  `adjustments.go:289` y `delivery.go:50`): cambia la cuenta a `closed`, reconoce la venta y publica
  `restaurant.account.closed`, pero **no llama a cocina** (no hay evento comercial ni `kitchen.Integrate`). Y la lectura
  del panel no mira el estado de la cuenta (`reads.go:163-166`). Nada cambia el estado de esas liberaciones: quedan
  para siempre. Además, con la ruta ya configurada, «Liberar a preparación» **manda a la cocina el pedido de una cuenta
  ya cobrada** (crea tareas y la pantalla lo muestra): probablemente es lo que vio el propietario («solo desaparecen los
  que tienen todo en estaciones con ruta»).
- **Cancelar la cuenta** (9D) sí llega a cocina: `account.cancelled` → `kitchen.Integrate` → `integrateChange`, que
  cancela sin papel lo no liberado (`changes.go:121-124`) — reproducido, pero **sin registro en el historial de
  cocina**. Si además algo ya liberado está en una estación desactivada, `integrateChange` deja el aviso pendiente y
  **sale antes** (`changes.go:96-110`) sin cancelar lo no liberado: la cuenta cancelada deja **dos** pendientes
  (reproducido). La lista del detalle de la cuenta muestra una liberación `cancelled` como «Liberación pendiente», con
  sus advertencias viejas (`AccountKitchen.tsx:46-55`).
- Combinar, dividir o transferir mueven las unidades a otra cuenta, pero el panel muestra el folio de la cuenta original
  (`r.account_id`, `reads.go:164-165`).

#### Qué casos ya no pueden ocurrir con la §2 y cuáles quedan

- **Ya no pueden ocurrir**: «Bloqueado por configuración» de un envío nuevo (sin ruta, estación desactivada, impresora
  desactivada y combo vacío dejan de bloquear; decisión 1) y los avisos pendientes por componentes nuevos sin ruta.
- **Quedan**:
  1. **Pedidos programados** (automáticos o manuales).
  2. **Los bloqueados viejos de cuentas todavía abiertas** (anteriores a la 0.17.0; los de cuentas cerradas o canceladas
     se dan por terminados, decisión 4). Con la regla nueva «Liberar» ya los manda (lo que tiene ruta llega; lo demás
     no necesita preparación).
  3. **«Pendiente de liberación» por quitarle la hora a un programado**, salvo que se acepte la decisión 9.
  4. **El aviso de un cambio para una estación desactivada que ya tenía la comanda**, salvo que se elija la opción B de
     la duda 1.

### 4. Lo que no necesita preparación

- **Ruta «Sin preparación» (`mode='none'`)** hoy: `resolveRouting` la congela en el `routing_snapshot` y no crea parte
  (`routing.go:124-129`); `prepareRelease` **sí libera la unidad** (fila en `kitchen_unit_releases`, consumo de
  inventario) y **no crea tarea** (`releases.go:151-167`). En `VersionPreparation` (`states.go:202-241`) cuenta como
  liberada y no suma tareas: un renglón que solo tiene eso dice **`ready`** («Listo») desde el envío; en una cuenta
  mixta no adelanta ni detiene nada (cuenta `released` hasta que lo que tiene tarea está listo, luego `ready`).
  Reproducido.
- **Aviso de platillos listos al mesero (A2)**: no decide «ya está todo» por cuenta; avisa **por unidad**: una unidad
  activa cuyas tareas vivas (de su versión, no canceladas) están todas listas y la última se marcó en esta transacción
  (`recordReadyNotices`, `preparation.go:159-200`, condición `tasks.live>0 AND tasks.ready=tasks.live`, `:174-175`). Una
  unidad **sin tarea** tiene `live=0`: nunca avisa y no detiene el aviso de las demás. Reproducido (un aviso, el del
  machacado).
- **Tarjeta del KDS (K1-D5)**: se arma solo con `kitchen_tasks` de las estaciones de la pantalla (`display.go:494-512`) y
  se cierra cuando no queda tarea pendiente de la ronda (`roundStatus`, `display.go:342-364`). Una unidad sin tarea **no
  está** en la tarjeta: no la cierra ni la detiene. Reproducido.

### 5. Lo que ya está en la Mac del propietario (solo con el código y el esquema)

- **`app.kitchen_releases`** (`0005:85-104`): `state` con `CHECK (state IN ('pending','scheduled','blocked','released',
'cancelled'))` (`:91`) y `CHECK ((state='released') = (released_at IS NOT NULL))` (`:103`); `warnings jsonb`,
  `routing_snapshot`, `station_snapshot`, `context_snapshot`, `revision`. Ninguna migración posterior cambió la tabla
  (solo índices; la 0029 no la toca). Los 13 son filas **`blocked`** con advertencias «Sin ruta de preparación: …» (y
  quizá «Estación inactiva» o «Impresora inactiva»), sin `kitchen_unit_releases`, tareas, comandas, trabajos ni
  solicitudes de inventario de sus unidades.
- **`app.kitchen_pending_changes`** (`0005:109-117`): `event_id` (PK), `account_id`, `kind`, `payload`, `warnings`,
  `revision`, `created_at`; **sin estado** (hoy se borra al resolverse, `changes.go:127`). Probablemente no hay ninguno en
  la Mac (la captura solo tiene bloqueados), pero la regla debe cubrirlos.
- **`app.restaurant_accounts.state`**: `open`, `merged`, `voided`, `closed` (`0006:25-30`; `closed_at` ligado a
  `closed`); la cancelada es `voided` con su fila en `account_cancellations` (0021). Un para llevar cobrado y no
  entregado sigue `open` (`collect.go:644-646`).

**Regla de lectura** (filtrar en `board` las de cuentas no abiertas): sin migración para esto, pero los datos siguen
diciendo `blocked` para siempre, no deja registro en el historial (la §3 lo pide), habría que repetir el filtro en cada
lector y «Liberar» por la API seguiría pudiendo mandar a cocina el pedido de una cuenta cobrada.

**Migración en la `0030`** (recomendada; la etapa ya necesita la `0030` para el ajuste «Productos sin ruta» y el host
respalda antes de migrar, `cmd/ad-el-pos-server/host.go:138`):

- `kitchen_releases`: `state` `pending|scheduled|blocked` → `cancelled` (y `revision+1`) cuando **ninguna unidad activa
  de la liberación está en una cuenta abierta** (pertenencia actual `u.account_id`, no `r.account_id`), con una fila
  `release.dismissed` por cada una en `kitchen_history` (`before_value` con estado y advertencias, `after_value` con
  `{state:'cancelled', reason:'account_not_open', migration:'0030'}`, sin actor; `id` con `gen_random_uuid()`).
- `kitchen_pending_changes`: columna nueva `dismissed_at timestamptz` (y `dismissed_reason text`), puesta en las de
  cuentas no abiertas, con su fila `change.dismissed` en el historial.
- **No se tocan**: `restaurant_accounts`, `restaurant_orders`, `restaurant_items`, `restaurant_item_units`,
  `restaurant_item_versions`, `restaurant_events`, pagos, devoluciones, `cash_*`, `sales_recognitions`,
  `account_cancellations`, `inventory_*` (sin consumos nuevos), `kitchen_unit_releases`, `kitchen_tasks`,
  `kitchen_tickets`, `kitchen_print_jobs`, `audit_events`. Las advertencias y los `snapshot` de cada liberación quedan
  como estaban.
- **Después de la etapa** (puntos de entrada): al **cerrar** (`payments.closeIfSettled`, en su misma transacción y con
  el 7820148305 exclusivo que ya toma `payments/handler.go:98`, sin lock nuevo) y al **cancelar** (`kitchen.Integrate`
  → `integrateChange`, `releases.go:315-327`), con la misma regla y su historial (`reason: account_closed` /
  `account_cancelled`, con el actor de la operación) y un evento de cocina para que el panel se actualice. Combinar no
  necesita nada (las unidades siguen en una cuenta abierta). Reabrir una cuenta cerrada (devolución) no revive lo
  terminado.

### 6. Decisiones propuestas

1. **Un envío nunca se bloquea** (`resolveRouting` y `prepareRelease`, solo al resolver con la configuración vigente,
   no con la congelada):
   - un producto **hoja** sin ruta propia ni de su categoría ni heredada (hoy «Sin ruta…») sigue el ajuste «Productos sin
     ruta»: **«No necesitan preparación»** = sin parte ni tarea; **«Enviar a una estación»** = su parte va a esa estación
     (si está activa; si no, como «No necesitan preparación»). Un combo **con** componentes y sin ruta propia sigue
     igual que hoy (sin parte propia; cada componente con su ruta o con el ajuste): así no se duplica el combo en la
     estación del ajuste y **nunca** se trata un combo como `mode='none'`, que dejaría de visitar sus componentes
     (`routing.go:127-129`). Un combo vacío se trata como hoja.
   - **ruta a estaciones desactivadas**: se omiten las desactivadas; si no queda ninguna, el producto se trata como sin
     ruta. Aviso en Enrutamiento (decisión 5).
   - **impresora desactivada** de una estación `printer` o `both`: no bloquea; se crean las tareas y la comanda, y el
     trabajo queda en «Requiere intervención» con «La impresora X está desactivada.», como ya hace `enqueueJob`
     (`documents.go`, `inactivePrinterMessage`); con `both` la pantalla lo recibe igual. Se reintenta en la cola al
     reactivarla. Aviso en Enrutamiento.
   - **toda unidad activa del envío se libera una vez** (fila canónica + inventario), con o sin tarea, como hoy
     `mode='none'`. `prepareRelease` deja de escribir `blocked` (el valor queda en el `CHECK` por lo histórico).
   - **`routing_snapshot`**: se congela la decisión de cada producto resuelto por el ajuste o con estaciones omitidas,
     con dos campos opcionales nuevos de `Route` (`omitempty`, compatibles): `unrouted: true` (vino del ajuste) y
     `omitted_station_ids` (estaciones desactivadas que no recibieron). Así una modificación posterior sigue la misma
     decisión aunque después se configure la ruta.
   - **Qué se registra para el aviso**: nada por pedido aparte del `snapshot`; el aviso de Enrutamiento se calcula en
     vivo con la configuración (decisión 5).
     _Por qué_: es lo que pide la §2; el experimento muestra que «sin ruta = sin tarea» ya da la tarjeta, el aviso al
     mesero, «Listo» y la comanda correctos; liberar todas las unidades mantiene el inventario y los reportes como con
     `mode='none'`.
2. **Cambios de algo ya liberado** (`integrateChange`): los componentes nuevos sin ruta o a estaciones desactivadas
   siguen la decisión 1 (no dejan aviso pendiente). Las rutas congeladas no cambian (los avisos van a las estaciones
   que recibieron). **«Cancelar cuenta» cancela siempre lo no liberado** (mover `changes.go:121-124` antes del `return`
   de `:110`), con su historial. Lo que haga con «Aviso pendiente para estación inactiva» es la **duda 1**.
3. **Ajuste «Productos sin ruta»**: columna `unrouted_station_id uuid NULL REFERENCES app.kitchen_stations(id)` en
   `kitchen_settings` (sección «Fase A» de la `0030`): `NULL` = «No necesitan preparación» (omisión, sin cambiar nada al
   actualizar), una estación = «Enviar a una estación».
   - **Fuera de `Settings`** (que viaja en cada comanda, `models.go:13-25`), como hizo la K1 con los límites de color
     (`SettingsView`, `settings.go:60-79`); `PUT kitchen/settings` no lo toca (columnas explícitas, `settings.go:128`).
   - **Escritura**: `PUT kitchen/unrouted` `{operation_id, station_id: uuid|null, expected_station_id: uuid|null}` con
     `kitchen.routing.manage`; 409 `KITCHEN_UNROUTED_CHANGED` si lo guardado no es lo esperado (como `kitchen/access`,
     sin compartir la revisión de `kitchen_settings` para no chocar con «Comandas y tiempos»); historial `unrouted.saved`
     y el evento `kitchen.unrouted.changed` (configuración) que ya publica `Handle` (`handler.go:125-134`).
   - **Validación**: la estación existe y está activa (como `saveRoute`, `routing.go:45-51`), con cualquier recepción.
     Si después se desactiva: se permite; al enviar se trata como «No necesitan preparación» y la cobertura lo avisa. Si
     se borra (solo «Quitar datos de prueba», `maintenance/apply.go:77`): la vista previa lo lista en «configuración que
     se desvincula» y la eliminación lo vuelve `NULL` antes de borrar la estación (si no, la FK lo impide).
   - **Lectura**: en la cobertura (decisión 5) y como `unrouted_station_id` en `SettingsView`.
4. **Pendientes de cuentas no abiertas**: estado final **`cancelled`** para las liberaciones (ya está en el `CHECK`;
   ningún lector fuera de `kitchen` distingue el motivo) y **`dismissed_at`** para los cambios pendientes, siempre con
   historial (`release.dismissed` / `change.dismissed`, con el motivo); regla «ninguna unidad activa en una cuenta
   abierta»; puntos de entrada en `closeIfSettled` (función nueva de `kitchen`, p. ej. `kitchen.AccountClosed`) y en
   `account.cancelled`; lo existente con la migración `0030`. Los lectores de cambios pendientes filtran
   `dismissed_at IS NULL` (`reads.go:170`, `states.go:191-195`, `changes.go:97-99` y `:257-262`). Además, **«Liberar»
   responde 409** (p. ej. `KITCHEN_RELEASE_NOT_PENDING`) para una liberación ya liberada o cancelada o cuyas unidades ya
   no están en una cuenta abierta, en lugar de 200 sin hacer nada (o, hoy, de mandar a cocina el pedido de una cuenta
   cobrada). _Por qué_: la §3 pide estado + historial sin tocar ventas; la migración deja los datos coherentes y la misma
   regla sirve hacia adelante.
5. **Lectura nueva para el escritorio**: `GET kitchen/coverage` con `kitchen.stations.read` (el mínimo para ver la
   configuración de cocina; lo tiene quien ve Estaciones, y los tres usos lo necesitan), capacidad
   **`kitchen-coverage-v1`** (anuncia también `PUT kitchen/unrouted`). Go resuelve cada producto con la misma función que
   el envío (extraer de `resolveRouting` la resolución vigente de un producto); React no calcula rutas. Forma propuesta:

   ```json
   {
     "unrouted": {
       "station_id": null,
       "station_name": null,
       "station_active": null,
       "effective": "none"
     },
     "stations": [
       {
         "id": "…",
         "name": "Cocina",
         "active": true,
         "reception": "kds",
         "printer": null,
         "categories": [{ "id": "…", "name": "Desayunos", "products": 12 }],
         "products": [{ "id": "…", "name": "Malteada", "category_name": "Bebidas" }],
         "receives_unrouted": false,
         "display_ids": ["…"]
       }
     ],
     "no_preparation": { "categories": [{ "id": "…", "name": "Postres" }], "products": [] },
     "unrouted_items": [
       {
         "category_id": "…",
         "category_name": "Refrescos",
         "whole_category": true,
         "products": [{ "id": "…", "name": "Coca Cola 600ml", "reason": "no_route" }]
       }
     ],
     "displays": [
       { "device_id": "…", "name": "Tableta cocina", "all_stations": true, "station_ids": ["…"] }
     ],
     "warnings": [
       {
         "code": "station_without_routes",
         "station_id": "…",
         "message": "La estación Cocina no tiene categorías ni productos asignados."
       }
     ]
   }
   ```

   - `stations[].categories` son las categorías cuya ruta de categoría llega a esa estación (activa), con cuántos de sus
     productos llegan de verdad (sin los que tienen ruta propia a otro lado); `products`, las excepciones por producto.
   - `unrouted_items`: lo que no va a ninguna estación por una ruta (`reason`: `no_route` o `inactive_station`), por
     categoría; con `effective` el escritorio escribe «Sin ruta: Refrescos, Extras (no necesitan preparación)» o «(van
     a Cocina)». Los combos con componentes y sin ruta propia no se listan (sus componentes se resuelven solos).
   - `displays[].station_ids` son las estaciones que muestra cada pantalla (las `kds`/`both` de su selección o de todas),
     las de `loadDisplayScope` (`display.go:133-162`).
   - `warnings[].code`: `display_without_stations` («Esta pantalla no tiene estaciones.»), `display_no_screen_stations`
     («Ninguna de sus estaciones recibe comandas en pantalla.»), `station_without_routes` («La estación Cocina no tiene
     categorías ni productos asignados.»), `route_station_inactive` («La ruta de Extras va a Barra, que está desactivada:
     esos productos se tratan como productos sin ruta.»), `unrouted_station_inactive` y `station_printer_inactive`; cada
     uno con el id de lo que hay que corregir (`device_id`, `station_id`, `category_id`, `product_id`) para el botón que
     lleva a corregirlo. El texto viene de Go, listo para la pantalla.
   - Documentada en `openapi.json`, `docs/API_V1.md` y un paso de `TestContractV1`.

6. **Asistente al aprobar una pantalla de cocina (§4.1)**: **sin rutas nuevas** salvo la cobertura; usa las existentes en
   orden, cada una con su `operation_id`:
   1. estaciones de la pantalla: `GET kitchen/coverage`; crear «Cocina» con `POST kitchen/stations`
      (`kitchen.stations.manage`; recepción `both` con la impresora elegida entre `GET kitchen/outputs` —también
      `kitchen.stations.manage`— si hay una impresora activa, si no `kds`); cambiar una estación que solo imprime a
      «Impresora y pantalla» con `PATCH kitchen/stations/{id}` (la estación completa con su revisión, misma impresora);
   2. categorías: casillas con las categorías de la cobertura; marcar crea `POST kitchen/routes` o cambia
      `PATCH kitchen/routes/{id}` la ruta de categoría (`kitchen.routing.manage`); las que ya llegan a esa estación salen
      marcadas y no se desmarcan ahí (no existe borrar una ruta; se cambia en Enrutamiento);
   3. resumen: otra vez la cobertura («Esta pantalla recibirá: … No recibirá: … (no necesitan preparación).»).
   - **No hace falta `PUT kitchen/access`** al emparejar: una pantalla nueva muestra **todas** las estaciones
     (`kitchen_all_stations` es `true` por omisión, `0022:46`) y su `device_id` solo existe cuando la tableta reclama su
     credencial (`approve` responde `{state, name}`, `pairing.go:724`; el dispositivo se crea en `claim`, `:555-557`). En
     «Configurar qué recibe» de una pantalla ya emparejada sí se puede elegir con `PUT kitchen/access`
     (`kitchen.stations.manage`, solo estaciones `kds`/`both`).
   - **Permisos**: aprobar exige `devices.manage` (`pairing.go:633`); cada paso, el suyo. Sin él, el paso se muestra en
     solo lectura con «Pide a quien administra Cocina que lo configure» y se puede saltar. El rol administrador los tiene
     todos.
   - **Dónde**: al aprobar con éxito una solicitud `device_kind: 'kitchen'` en `PairingRequestItem.approve`
     (`DevicesPage.tsx:286-299`), con la capacidad; «Configurar qué recibe» junto a «Ver configuración» en la fila de la
     pantalla (`DevicesPage.tsx:1390-1401`) y en `KitchenDisplayConfiguration` (`:666-748`), donde va también el
     diagnóstico de la §4.2.
7. **Estaciones (§4.4)**: el apartado está en `TerminalStations.tsx:44-45` (título «Terminales de cocina», resumen
   «Estaciones que muestra cada terminal y aviso sonoro»), dentro de `StationsAdmin.tsx:93`; pasa a **«Computadoras en
   modo Cocina»** con «Las pantallas de cocina (tabletas) se configuran en Sistema › Terminales.». Cambian también las
   referencias de `DevicesPage.tsx:654` y `:1155-1156` («Cocina › Estaciones › Terminales de cocina»). La descripción de
   la recepción en pantalla está en `StationsAdmin.tsx:181-184`; propuesta: «Las pantallas de cocina muestran estos
   pedidos al momento. En Comandas se ve si la pantalla ya los recibió.».
8. **El panel (§3)**: nombre **«Por enviar a cocina»** (quedan más casos que los programados; si se elige la opción B de
   la duda 1 y la decisión 9, solo quedarían programados y los bloqueados viejos de cuentas abiertas, que se van solos
   al cerrarse: entonces «Pedidos programados»). Resumen «N pedidos · N avisos». Textos por caso:
   - programado automático: «Se envía a cocina solo el {día} a las {hora}.»; manual: «Programado para el {día} a las
     {hora}. Espera a que lo envíes a cocina.»;
   - bloqueado viejo de una cuenta abierta: «La cocina no recibió este pedido.» + «Se envió con la versión anterior, que
     lo detenía por un producto sin ruta.» (sin los renglones «Sin ruta de preparación»);
   - «Pendiente de liberación» (si no se acepta la decisión 9): «Espera a que lo envíes a cocina.»;
   - aviso para una estación desactivada (opción A): «No se pudo avisar a {estación}: está desactivada.» + «Actívala en
     Cocina › Estaciones y vuelve a avisar.».
     Botones con su texto corto debajo: **«Enviar a cocina ahora»** (hoy «Liberar a preparación» / «Liberar
     anticipadamente»): «La cocina lo recibe al momento.» (antes de su hora, «…antes de su hora.»); **«Cambiar la hora»**
     (hoy «Ajustar programación», solo en programados): «Cambia cuándo llega a cocina. No cambia el pedido ni su
     precio.»; **«Volver a avisar»** (hoy «Revisar destinos y generar aviso»): «Manda el cambio a la estación si ya está
     activa.». El mensaje de éxito sale solo si Go dice `released` (con la decisión 1 siempre lo dirá).
     Fuera del panel: el detalle de la cuenta debe decir «No se envió a cocina» para una liberación `cancelled` en vez de
     «Liberación pendiente» (`AccountKitchen.tsx:46-55`).
9. **Quitar la hora a un pedido programado lo envía en ese momento** (`releases.go:338-348`: después de pasarla a
   `pending`, `prepareRelease`, como un pedido inmediato en `createRelease`, `:76-80`). _Por qué_: hoy queda
   «Pendiente de liberación» sin que nadie lo libere (reproducido) y es el único origen vivo de ese caso.
10. **Aplicación de cocina (§5): no cambia** ni `cocina/` ni `android-core/` (sin APK nuevo). La tableta solo pide
    `kitchen-display/orders` y lo reemplaza completo con cada evento (`KitchenBoard.kt:115-121`, K1-D25); la tarjeta y su
    estado los arma Go con las tareas de sus estaciones (`display.go:494-512`; la tableta usa `status`, `Board.kt:153`,
    `:173`). Con la decisión 1, un pedido mixto crea las tareas de lo que tiene ruta y `persistDocuments` publica
    `kitchen.ticket.created` para cada estación, que la pantalla recibe (prueba K1 y reproducción: la tarjeta llega por
    la misma ruta que usa la tableta). La aplicación de meseros tampoco cambia (solo mostrará menos «Sin confirmar en
    cocina»).
11. **Pruebas de la §6** (dónde y qué suites existentes tocan los cambios):
    - **Go, Fase A** — `operations`: `TestStageK11UnroutedProducts` (el parche) ampliada con «Enviar a una estación»
      (Coca Cola y papas llegan a esa estación), estación desactivada (lo demás llega, aviso en la cobertura), impresora
      desactivada (trabajo en intervención, la pantalla recibe), combo sin ruta con componentes y combo vacío, una
      modificación con la ruta congelada; pendientes al cobrar, cerrar con entrega (para llevar), cancelar (también con
      un aviso pendiente), combinar (se conserva) y reabrir; «Liberar» 409. `database`: `TestStageK11Upgrade` (desde el
      esquema 29: bloqueados de cuentas `closed`, `voided`, `merged` y `open`, un programado de un para llevar cobrado
      sin entregar y un cambio pendiente; solo cambian los de cuentas no abiertas y ninguna tabla de operación).
      `apicontract`: `TestContractV1` con la cobertura y `PUT kitchen/unrouted`. `administration`:
      `testTestDataRemoval` con la estación del ajuste.
      Suites existentes que cambian: `TestRestaurantAcceptance` («cocina rutas inexistentes no aparentan liberación…»,
      `kitchen_scheduling_test.go:218-233`, espera `blocked`; «ruta histórica y modificación bloqueada…»,
      `kitchen_changes_test.go:31-80`, según la duda 1), `TestStage9DAccountCancellation` (`stage9d_accounts_test.go:308-312`
      espera un bloqueado; `testStage9DCancelPendingKitchen`, `:726-760`), y para correr: `TestStageK1KitchenDisplay`,
      `TestStageK1R1KitchenDisplaySecurity`, `TestStage9FKitchenStations`, `TestStageA2ReadyAlerts`, las de inventario de
      `operations`, `kitchen` (unitarias) y `./cmd/ad-el-pos-server` (con el candado).
    - **Vitest, Fase B**: panel (`kitchenMessages.test.tsx`, que pulsa «Liberar a preparación»; `preparationBoard.test.tsx`),
      asistente y diagnóstico (`stageK1Terminals.test.tsx`; `stageA3Terminals.test.tsx` y `terminalStations.test.tsx`
      tienen «Terminales de cocina»), Enrutamiento («Sin ruta» y el ajuste), Estaciones, `AccountKitchen`,
      `stage9dAccounts.test.tsx` (etiqueta `pending_release`) y `auditActions.test.ts` (nombres nuevos del historial);
      `cargo`: `kitchen_routes_are_closed_and_keep_write_methods` (`api.rs:1410`) con las dos rutas nuevas en la lista
      cerrada de `api.rs` (que también lee `e2e/native-driver.ts:444`).
    - **Playwright, Fase B** (Chromium y WebKit): `workflow-stagez-k11-assistant.spec.ts` (emparejar una pantalla sin
      estaciones con `e2e/pairing.ts`, esperando en `PAIRING_RATE_LIMITED`, y terminar con «Cocina» y dos categorías) y
      `workflow-stagez-k11-diagnostics.spec.ts` (cada aviso del diagnóstico y la lista «Sin ruta» de Enrutamiento, y el
      panel). Existentes para correr con ellas: `workflow-stagez-k1-terminals.spec.ts`,
      `workflow-stage9f-kitchen-stations.spec.ts` («Terminales de cocina»), `workflow-stage9d-accounts.spec.ts` (espera
      `pending_release` y «Enviado · sin confirmar en cocina» con un producto sin ruta, `:199`, `:271`, `:338`, `:360`:
      **dejará de ocurrir con la §2**; hay que provocarlo de otro modo o cambiar lo que comprueba) y
      `workflow-stage9d-cancel-account.spec.ts`.
    - **Android**: nada (decisión 10).
12. **Reparto**: la **Fase A** (servidor) hace las decisiones 1–5 y 9 (con el 409 de «Liberar»), la `0030`, el contrato y
    sus pruebas; la **Fase B** (escritorio) hace 6–8, «Productos sin ruta» y «Sin ruta» en Enrutamiento, el diagnóstico,
    los textos de Estaciones, `AccountKitchen`, `api.rs`/`api.ts` y sus pruebas; la **Fase C** no tiene trabajo. **La B
    puede empezar en paralelo** si se fijan antes: la forma de `GET kitchen/coverage` (con los códigos de aviso y sus
    textos), el cuerpo, la respuesta y los errores de `PUT kitchen/unrouted`, el nombre de la capacidad, los estados que
    seguirá devolviendo `board.releases` (`scheduled`, `pending` y `blocked` viejo) y que `pending_changes` ya no trae los
    terminados, el 409 de «Liberar» y los nombres nuevos del historial (`release.dismissed`, `change.dismissed`,
    `unrouted.saved`). La B probaría contra un Go simulado en Vitest y contra la rama A para Playwright.

### 7. Riesgos y dudas para el coordinador

1. **Duda 1 · avisos para una estación desactivada que ya recibió la comanda** («Aviso pendiente para estación
   inactiva», `changes.go:85-93`). **Opción A (recomendada, la más conservadora)**: se conservan como el único aviso
   pendiente del panel, se dan por terminados cuando la cuenta se cierra o se cancela, y «Cancelar cuenta» cancela lo
   no liberado sin esperar al aviso (la §3 lo exige). **Opción B**: el aviso se manda siempre a la estación que recibió la
   comanda (activa o no; su impresora o su pantalla), así ya no hay avisos pendientes y el panel sería «Pedidos
   programados». La B cambia la regla de la etapa 5 y de la 9D (`kitchen_changes_test.go:31-80`,
   `testStage9DCancelPendingKitchen`).
2. **Duda 2 · «cobrada»**: se propone leer «cobrada, cerrada o cancelada» como «la cuenta ya no está abierta». Un para
   llevar **cobrado y no entregado** sigue abierto y su pedido programado debe seguir llegando a la cocina; si el
   propietario espera otra cosa, hay que decidirlo.
3. **Duda 3 · inventario de los 13**: lo bloqueado nunca consumió existencias (ni lo que tenía ruta). Darlo por
   terminado solo cambia el estado (como pide la §3): ese consumo no se registra. Si el propietario usa «Control de
   inventario», sus existencias quedaron altas por esos pedidos.
4. **Duda 4 · marcar en el asistente una categoría que ya va a otra estación**: ¿«cambiar» la ruta a la estación de la
   pantalla (deja de ir a la otra) o agregarla (llega completa a las dos, §5.3 de la 9F)? Se recomienda cambiarla, con el
   aviso «Desayunos dejará de ir a Barra».
5. **Duda 5 · impresora para «Cocina» nueva**: con varias impresoras activas, el asistente debe preguntar cuál; con una,
   usarla; con ninguna, «Solo pantalla». Y si ya existe una estación «Cocina» (los nombres son únicos, `0005:40`),
   ofrecer usarla.
6. **Decisión 9** cambia cuándo se libera un pedido al que se le quita la hora; si se prefiere no tocarlo, el caso
   «Pendiente de liberación» se queda en el panel con su texto.
7. **Riesgo · bloqueados viejos de cuentas abiertas**: la §2 dice que no se reenvían solos; quedan en el panel hasta que
   alguien los envíe o la cuenta se cierre. Si la Mac tiene cuentas de prueba abiertas desde hace días, seguirán ahí.
8. **Riesgo · etiqueta 9D**: «Enviado · sin confirmar en cocina» casi deja de existir (solo `pending`); la especificación
   de la 9D en Playwright la provoca con un producto sin ruta y debe cambiar en la Fase B.
9. **Riesgo · `routing_snapshot`**: si la Fase A congela `mode='none'` para un combo, sus componentes dejan de llegar
   (`routing.go:127-129`); la decisión 1 lo evita aplicando el ajuste solo a hojas.
10. **Riesgo · migración**: la `0030` es la primera migración de la etapa sobre una base real con datos de la 0.16.0; la
    regla usa la pertenencia actual de las unidades y debe probarse con combinar y dividir (`TestStageK11Upgrade`).

### Pruebas realmente ejecutadas

| Prueba                                                                                                    | Resultado                                                                            |
| --------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `reproducir.py` contra el Go temporal P0 (base nueva cada vez; tres corridas, la última en `salida.txt`)  | Reproduce los casos 1–4 y los adicionales descritos (estado 0)                       |
| `go vet -tags=integration ./internal/operations` con la prueba nueva                                      | Sin hallazgos                                                                        |
| `gofmt -l` de la prueba nueva                                                                             | Sin cambios                                                                          |
| `ISO_TAG=P0 ISO_RUN='^TestStageK11UnroutedProducts$' iso.sh … go-int ./internal/operations`               | **Falla como se esperaba**: casos 3 y 4 y las subpruebas 2 y 3 (`prueba-salida.txt`) |
| La misma, con la advertencia «Sin ruta» quitada temporalmente de `routing.go` (revertido, md5 comprobado) | **Pasa** (`prueba-experimento.txt`)                                                  |
| md5 de `docs/etapas/ETAPA_K1_1_COCINA_AJUSTES.md` antes y después                                         | Igual                                                                                |

**No verificado**: nada de Vitest, Playwright, `cargo`, `go-unit` ni `fmt-check` (no hay código del producto); la
reproducción no usa la tableta real ni el escritorio (la pantalla es la API de la tableta y el panel es `GET
kitchen/board`); el contenido real de la Mac del propietario (no se leyó su base: lo de la §5 sale del código y el
esquema); la impresión física.

### Archivos

- `$SP/k11/repro/reproducir.py` (script), `$SP/k11/repro/salida.txt` (su salida), `$SP/k11/repro/prueba.patch`
  (la prueba de Go, para la Fase A), `$SP/k11/repro/prueba-salida.txt` y `prueba-experimento.txt`.
- Código citado: `backend/internal/kitchen/{releases,routing,changes,reads,handler,states,preparation,display,documents,
settings,worker}.go`, `backend/internal/payments/collect.go`, `backend/migrations/0005_kitchen_and_printing.sql`,
  `desktop/src/pages/kitchen/{PreparationBoard,ScheduleEditor,StationsAdmin,TerminalStations,RoutingAdmin}.tsx`,
  `desktop/src/pages/DevicesPage.tsx`, `desktop/src/pages/restaurant/AccountKitchen.tsx`,
  `cocina/app/src/main/kotlin/com/adelpos/cocina/board/{KitchenBoard,Board}.kt`.

### Decisiones del coordinador al cerrar la Fase 0

El contrato entre la Fase A y la Fase B (cobertura, «Productos sin ruta», los 409 y los nombres del historial) se fijó
con estas decisiones antes de empezar las dos fases en paralelo.

- **K11-D1. Un envío nunca se bloquea** (Decisión propuesta 1, aceptada como está). Un producto simple (o un combo
  vacío) sin ruta sigue el ajuste «Productos sin ruta»; de una ruta se omiten las estaciones desactivadas y, si no
  queda ninguna, el producto se trata como sin ruta; una impresora desactivada no detiene nada (el trabajo espera en
  «Requiere intervención» y con «Impresora y pantalla» la pantalla lo recibe igual). Toda unidad activa del envío se
  libera una vez (fila canónica e inventario), con o sin tarea, como hoy con «Sin preparación». Un combo con
  componentes y sin ruta propia sigue como hoy: el ajuste nunca se aplica a un combo como `mode='none'`. La decisión se
  congela en `routing_snapshot` (`unrouted`, `omitted_station_ids`, opcionales). `prepareRelease` deja de escribir
  `blocked`; el valor queda en el `CHECK` por las filas viejas.
- **K11-D2. Cambios de algo ya enviado** (Decisión propuesta 2 y **Duda 1, opción A**). Los componentes nuevos siguen
  K11-D1 y no dejan aviso pendiente; las rutas congeladas no cambian. El aviso de un cambio para una estación
  desactivada que ya tenía la comanda **se conserva** (regla de la etapa 5 y la 9D): es el único aviso que queda en el
  panel, y se da por terminado cuando la cuenta se cierra o se cancela. «Cancelar cuenta» siempre cancela lo no enviado,
  sin esperar a ese aviso, con su registro en el historial. _Por qué_: la opción B cambiaría una regla de la etapa 5 que
  esta etapa no pide cambiar.
- **K11-D3. Ajuste «Productos sin ruta»** (Decisión propuesta 3, aceptada). `kitchen_settings.unrouted_station_id` en
  la `0030` (`NULL` = «No necesitan preparación», el valor al actualizar); `PUT kitchen/unrouted` con
  `kitchen.routing.manage` y el valor esperado (409 `KITCHEN_UNROUTED_CHANGED`); fuera de `Settings`, que viaja en cada
  comanda. Una estación del ajuste que se desactiva se trata como «No necesitan preparación» y la cobertura lo avisa;
  «Quitar datos de prueba» la vuelve `NULL` antes de borrar la estación.
- **K11-D4. Pendientes de cuentas que ya no están abiertas** (Decisión propuesta 4 y **Duda 2**). «Cobrada, cerrada o
  cancelada» se lee como **«la cuenta ya no está abierta»** (cerrada o cancelada, por la pertenencia actual de las
  unidades). Un pedido para llevar cobrado y todavía no entregado sigue abierto y su pedido programado sigue llegando a
  la cocina a su hora (la cocina todavía tiene que prepararlo). Las liberaciones pasan a `cancelled` y los avisos
  pendientes reciben `dismissed_at`, siempre con historial (`release.dismissed`, `change.dismissed`, con el motivo); se
  aplica al cerrar (`payments.closeIfSettled`, en su transacción y con el lock 7820148305 que ya toma), al cancelar
  (`account.cancelled`) y, para lo que ya existe, en la migración `0030` (el `host` respalda antes de migrar). Nunca se
  tocan ventas, pagos, cuentas, cortes ni inventario. «Enviar a cocina ahora» responde **409
  `KITCHEN_RELEASE_NOT_PENDING`** con una liberación ya enviada, cancelada o de una cuenta que ya no está abierta (hoy
  responde 200 sin hacer nada, o manda a la cocina el pedido de una cuenta cobrada); «Volver a avisar» de un aviso
  terminado, **409 `KITCHEN_CHANGE_NOT_PENDING`**.
- **K11-D5. Cobertura del enrutamiento** (Decisión propuesta 5, aceptada con dos agregados: la lista `categories` con la
  ruta de cada categoría para el asistente y el texto exacto de cada aviso). `GET kitchen/coverage` con
  `kitchen.stations.read` y la capacidad `kitchen-coverage-v1`, resuelta en Go con la misma función que el envío.
- **K11-D6. Asistente al aprobar una pantalla de cocina** (Decisión propuesta 6 y **Dudas 4 y 5**). Sin rutas nuevas
  aparte de la cobertura. Marcar una categoría que ya va a otra estación **cambia** su ruta a la estación de la
  pantalla, con el aviso «Desayunos dejará de ir a Barra» (la especificación dice «crea o cambia la ruta»). Con varias
  estaciones en la pantalla, cada categoría marcada elige a cuál va (por omisión, la primera). La «Cocina» nueva usa la
  impresora de cocina activa si hay una, pregunta cuál si hay varias y es «Solo pantalla» si no hay ninguna. Si ya
  existe una estación «Cocina» (los nombres son únicos), se ofrece usarla: si solo imprime, cambiar su recepción; si está
  desactivada, activarla con recepción en pantalla. Cada paso exige su permiso; sin él, el paso se ve en solo lectura y
  se puede saltar.
- **K11-D7. Estaciones** (Decisión propuesta 7, aceptada): «Computadoras en modo Cocina» y la descripción nueva de la
  recepción en pantalla.
- **K11-D8. El panel se llama «Por enviar a cocina»** (Decisión propuesta 8). Con K11-D2 y K11-D9 quedan pedidos
  programados, pedidos que esperan a que alguien los envíe y avisos para una estación desactivada, así que «Pedidos
  programados» no los describe. Los botones pasan a «Enviar a cocina ahora», «Cambiar la hora» y «Volver a avisar»,
  cada uno con su texto corto. El mensaje de éxito sale solo si Go dice `released`. El detalle de la cuenta dice «No se
  envió a cocina» para una liberación cancelada.
- **K11-D9. Quitar la hora a un pedido programado no cambia** (Decisión propuesta 9, **no aceptada**). La
  especificación pide no agregar funciones fuera de lo que dice, y ese caso no viene de un producto sin ruta. El pedido
  sigue quedando «pendiente», y el panel lo explica en lenguaje sencillo: la cocina no lo recibe hasta que alguien toca
  «Enviar a cocina ahora». Se le dirá al propietario en la entrega.
- **K11-D10. La Fase C no tiene trabajo** (Decisión propuesta 10, aceptada). No cambian `cocina/` ni `android-core/`,
  no hay APK nuevo y no se usan emuladores.
- **K11-D11. Inventario de los pendientes viejos** (Duda 3). Lo que quedó bloqueado nunca consumió existencias, y darlo
  por terminado no registra ese consumo: solo cambia su estado, como pide la §3. Es una limitación para la entrega.
- **K11-D12. Reparto.** La Fase A hace K11-D1 a K11-D5 con la `0030` (esquema 30), el contrato y sus pruebas. Parte
  de la prueba de la Fase 0 (`TestStageK11UnroutedProducts`) y cambia las pruebas existentes que esperaban el bloqueo.
  La Fase B hace K11-D6 a K11-D8, «Productos sin ruta», la lista «Sin ruta» de Enrutamiento y el diagnóstico de
  Terminales, en paralelo con la A y con el contrato fijado. Corre Playwright después de recibir la Fase A.
- **K11-D13. Playwright.** Las especificaciones nuevas se llaman `workflow-stagez-k11-*.spec.ts`. La de la 9D que
  provocaba «Enviado · sin confirmar en cocina» con un producto sin ruta se cambia en la Fase B.
- **K11-D14. Versiones.** La `0030` es la única migración de la etapa, con una sección por fase. Si una prueba exige
  que `config/product.json` diga el esquema 30 junto con la migración, la Fase A cambia solo `schema_version`. La Fase D
  sube el servidor a 0.17.0.
