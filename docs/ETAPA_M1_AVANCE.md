# Avance · Etapa M1: unir mesas, mesas divisibles y ajustes pendientes

Especificación del propietario: [docs/etapas/ETAPA_M1_MESAS.md](etapas/ETAPA_M1_MESAS.md) (solo lectura). Rama
`etapa-m1`, creada el 27/11/2026 desde `main` (`a33ada0`: servidor 0.17.0, esquema 30, meseros 0.4.1 y cocina 0.1.0,
con la K1.1 fusionada); `72ecadd` solo agrega la especificación. La 0.17.0 ya está instalada en la Mac del propietario
(la aplicación de `/Applications` dice 0.17.0), así que la `0030` es inmutable y la migración de esta etapa es una
`0031` nueva.

Forma de trabajo: la de `CLAUDE.md` §0 y la §0 de la especificación:

- un solo agente en la Fase 0 y, como mucho, dos a la vez;
- pruebas aisladas en el clúster temporal 55499 y Go temporales;
- nunca la instalación real ni `licensing.sicrat.com`;
- los emuladores del propietario (consolas 5554 y 5556) nunca se tocan.

Los guiones de reproducción, los parches y los registros de las pruebas viven en el scratchpad de la sesión, fuera del
repositorio (`$SP/m1/`).

## Fase 0 · Verificación, reproducción de los defectos y propuesta

Árbol `$SP/wt/m1-p0`, rama `etapa-m1-p0` (desde `etapa-m1` `72ecadd`), `ISO_TAG=P0`, Go temporal en 8201/8461 con la
confianza de prueba de licencias (`ISO_LICENSE_TRUST` = `contracts/licencias/v1/fixtures/vectors.json`). Sin código
del producto, sin commits y sin emuladores; el árbol quedó limpio y la especificación intacta (md5 `4750ae46…` antes y
después). Los guiones, sus salidas y los parches de las pruebas que fallan están en `$SP/m1/repro/`, fuera del
repositorio.

### Qué se hizo

1. **Lectura del código** de mesas, cuentas, mapa, cola sin conexión, reportes, documentos, cocina, permisos y
   concurrencia (§6.1), del importador y del sistema anterior (§6.2).
2. **Respaldo real, solo la tabla `mesas`** del paquete, con un programa de conteo en una copia `git archive` dentro de
   `$SP/legacy-data/` (0700). Solo salieron conteos y clases de nombres. Después se borró todo `$SP/legacy-data/`,
   `legacy-prepare --cleanup` no encontró restos, `docker ps -a` y `docker volume ls` quedaron vacíos y Docker se
   cerró. Docker se abrió una vez más después del aviso del coordinador (el propietario lo había cerrado).
3. **Reproducción por la API** (`$SP/m1/repro/reproducir.py`, salida en `salida.txt`): base nueva, licencia de prueba
   (no se imprime), la configuración del propietario (Cocina «Solo pantalla de cocina», Hamburguesas y Desayunos a
   Cocina, Coca Cola sin ruta con «No necesitan preparación», papas «Sin preparación»), una pantalla de cocina emparejada
   por la API y tres mesas.
4. **Tres pruebas automáticas que fallan hoy**, guardadas como parches:
   - Go de integración para §5.2;
   - Vitest para §5.1 y §5.2;
   - JVM de Android para §5.5.
5. **Propuesta** para las fases A a E (más abajo).

---

### 1. Mesas hoy (§6.1)

#### Modelo

- **Tablas** (`backend/migrations/0004_restaurant_operations.sql:2-29`):
  - `app.restaurant_areas`: `name` UNIQUE global (1–100), `sort_order`, `active`, `revision`.
  - `app.restaurant_tables`:
    - `area_id`, `name` de 1 a 60 caracteres, `UNIQUE (area_id, name)`;
    - `shape` `rectangle|round`, `x` 0–936, `y` 0–504, `width` 64–240, `height` 56–180, con
      `CHECK (x+width<=1000 AND y+height<=560)`;
    - `seats` 1–40, `active`, `out_of_service`, `revision`.
  - Ninguna migración posterior cambia estas dos tablas.
- **Cuenta**: `app.restaurant_accounts.table_id` es opcional y sin unicidad (`:44`), con un índice parcial
  `WHERE state='open'` (`:65`). Sus estados son `open|merged|voided|closed` (`0006:27`), con `merged_into_id` y
  `closed_at`.
- **Una mesa puede tener varias cuentas abiertas**, confirmado en la reproducción: la Mesa 1 quedó con
  `account_count=2`. La segunda cuenta se abre a sabiendas con `seen_account_ids`; sin ver la primera, Go responde
  `409 TABLE_HAS_ACCOUNT` (`accounts_write.go:198-226`).
- **«Mesa ocupada» la calcula Go en una sola consulta**: `tableQuery` (`operations/tables.go:28-49`):
  - `free` si no hay cuentas abiertas con `table_id` = la mesa;
  - `awaiting_payment` > `partial_payment` > `occupied` según la cuenta más avanzada;
  - `out_of_service` si la mesa está inactiva o fuera de servicio;
  - además `account_count`, `occupied_since`, `balance` (`payments.AccountStandingSQL`) y `long_open`.
- **Lo protegen**:
  - no se desactiva ni se pone fuera de servicio una mesa con `AccountCount>0` (`tables.go:141-148`,
    `table_service.go:35`);
  - no se desactiva un área con cuentas (`areas.go:46-55`).
  - **Hueco para las uniones**: el `PATCH` de una mesa puede cambiarle el área aunque tenga cuentas (`tables.go:149`).

#### Áreas y distribución

- **Rutas en Go**:
  - `POST/PATCH restaurant/areas`, con nombre recortado y `Position` (`internal/ordering`; `areas.go:12-117`);
  - `POST/PATCH restaurant/tables`, con `validateGeometry` (`tables.go:60-68`: lienzo 1000 × 560, 64–240 × 56–180,
    forma);
  - `PUT restaurant/layout`, hasta 100 posiciones con revisión cada una (`tables.go:164-217`);
  - `POST restaurant/tables/{id}/service` (`table_service.go`).
- **Eventos**: `Handle` publica `restaurant.<recurso>.changed` (por ejemplo `restaurant.tables.changed`,
  `restaurant.layout.changed`) en la misma transacción (`handler.go:94-99`).
- **Escritorio**:
  - el diseñador es `TableMap` con `designer` (`pages/restaurant/OperationsAdmin.tsx:351`);
  - el mapa operativo está en `components/restaurant/TableMap.tsx`.

#### Cuentas por mesa

- **Abrir** (`accounts_write.go:152-193`):
  - `POST restaurant/accounts` con `items` es la **creación diferida**: la cuenta y su borrador nacen en la misma
    operación al guardar el primer producto;
  - con `seen_account_ids`, Go rechaza si apareció una cuenta que la terminal no veía.
- **Clientes**:
  - el escritorio y Android solo llaman a esa ruta al guardar el primer producto;
  - Android: `TablesViewModel.kt:226-250` (`openCapture` con `seenAccountIds`) y `WaiterApi.kt:60`;
  - antes no hay nada en Go.
- **Nombre de la cuenta** (`account_names.go`):
  - sin nombre, la cuenta toma el de la mesa («referencia neutra»);
  - `movedAccountName` (`:61-73`) lo cambia al de la mesa nueva al moverla, si seguía siendo la referencia.
- **Mover**:
  - `POST …/move` (`accounts_write.go:281-339`, `accounts.move`) y `PATCH` con otro `table_id` (`:228-275`);
  - `validateTableTarget` solo pide la mesa activa y en servicio: **se puede mover a una mesa ocupada**.
- **Combinar cuentas (9D)** (`merges.go:5-82`):
  - cuentas abiertas de **cualquier mesa**, misma modalidad y lista;
  - el origen no puede tener pagos ni ajustes;
  - las unidades y el borrador pasan al destino;
  - el origen queda `merged`.
- **Cerrar**: el cobro, en `payments.closeIfSettled` (`payments/collect.go:637-674`, `state='closed'`). Una devolución
  puede **reabrir** (`reopenIfPending`, `:677-692`).
- **Cancelar**: `cancel-account` (`account_cancellation.go:202`, `voided`).
- **Descartar o liberar una cuenta vacía**: `discard` y `release` (`accounts_release.go:58-149`, `voided`).
- **Cuándo queda libre la mesa**: cuando ya no le queda ninguna cuenta `open` con su `table_id`. No hay un estado
  guardado de la mesa.

#### El mapa y la lista

- **Escritorio**:
  - lee `GET restaurant/overview` (áreas, mesas con su estado y ajustes) y `GET restaurant/accounts?table_id=`, por las
    órdenes de Tauri de `desktop/src-tauri/src/api.rs:228-239`;
  - **una ruta nueva exige agregarla ahí**;
  - se refresca con **cualquier** evento del SSE: `useRealtime.ts:96-127` lanza `adel-server-change` y los recursos se
    vuelven a leer;
  - `TableMap.tsx`:
    - cada mesa es un botón con posición absoluta;
    - `fitTables` (`:65-109`) da una escala, un origen y un desplazamiento;
    - `tableStyle` (`:511-525`) da `left = offsetX + (x − originX) × scale`.
  - **Un contorno de unión** es un elemento con posición absoluta del rectángulo que envuelve a sus mesas, con la misma
    fórmula.
- **Android**:
  - `RestaurantRepository` lee `restaurant/state` (cuentas abiertas y mesas) y `restaurant/overview`;
  - se refresca 400 ms después de **cualquier** evento del ámbito `restaurant` o `takeaway` (`:122-135`, `:166-199`,
    `:341`);
  - `TablesMap.kt:66-130` usa `TableMapFit.of` (`core/domain/.../TableMapFit.kt:46-65`, la regla del escritorio) y
    `Modifier.offset(left, top)`: el contorno se dibuja igual;
  - la lista y el panel filtran las cuentas por `tableId == mesa` (`TablesViewModel.kt:217-220`).

#### Cola sin conexión

- **Destino** (`offline_orders.go:126-134`): `account_id`, o `table_id` + `seen_account_ids` + `name`, o `takeaway`.
- **Con `table_id`** (`diagnoseOfflineTable`, `:467-534`):
  - mesa inexistente, inactiva o fuera de servicio → `table_unavailable`, que se corrige;
  - partidas que ya están en una cuenta abierta **de esa mesa** → `items_in_account` con `account_id`
    (`openAccountsWithItems`, `:574-596`, filtra por `a.table_id`);
  - cuentas abiertas que el mesero no veía → `table_has_account`, que se acepta con «Requiere tu revisión»;
  - si no hay nada de eso, abre una cuenta nueva en `table_id` (`:1001-1006`).
- **Con `account_id`**: va a esa cuenta, o `account_closed` si ya no está abierta.

#### Reportes por mesa

- **Instantánea al cerrar**: `app.sales_recognitions` copia `table_id`, `table_name`, `area_id` y `area_name` en ese
  momento (`0017_reports.sql:29-45`; `payments/recognition.go:118-125` lee la mesa actual de la cuenta; el relleno
  histórico de la 0017 está en `:158-170`).
- **«Ventas de salón»** (`reports/sales.go:294-305`): agrupa por `r.area_id,r.table_id` con el último `table_name`.
- **Leen la mesa viva de la cuenta**:
  - «En curso» (`reports/sales_progress.go:78`);
  - cancelaciones (`reports/cancellations.go:177,232,424`);
  - el filtro de mesa (`reports/filters.go:101,182`).
- **Del sistema anterior**: `app.legacy_sales.table_name` es texto copiado. `table_id` no tiene llave foránea.

#### Documentos impresos

- **Precuenta, ticket y comprobante**:
  - `payments/documents.go:104-110` → `documents.TableContext(name)` (`documents/table_names.go:31-36`);
  - imprimen **«Mesa 4»**, y «Mesa 1» si el nombre ya empieza con la palabra (nunca «Mesa Mesa 1»);
  - el nombre lo lee `payments/finance.go:28`, la mesa viva.
- **Comanda**:
  - la foto del contexto al liberar, `kitchen/releases.go:45-51` (`'table', t.name`);
  - la imprime `documents/render_kitchen.go:26` como **«Salón · Mesa 1»**: área · nombre de la mesa tal cual, sin
    anteponer «Mesa».

#### Cocina

- **KDS**:
  - `kitchen/display.go:588-630`: `title` = el nombre de la mesa viva; `subtitle` = el nombre de la cuenta si es
    distinto;
  - la aplicación de cocina pinta `card.title` tal cual (`cocina/.../ui/OrderCardView.kt:197`).
  - **Si Go manda «Mesa 5 + 6» en `title`, la aplicación de cocina lo muestra sin cambiar su código**: la Fase D no
    hace falta.
- **Preparación del escritorio**: `kitchen/reads.go:131-136` (`table_name`, la mesa viva).
- **Avisos de platillos listos**: `administration/ready_alerts.go:139`.
- **Evento de encabezado (K1-D7)**:
  - `displayAccountKinds` (`display.go:72-80`): `restaurant.account.updated|move|assign|merged|split|cancelled` y
    `restaurant.items.transferred`, solo de las cuentas con tareas en sus estaciones (`:1153-1202`);
  - **un cambio de nombre o de mesa de la cuenta ya lo dispara** (`account.updated`, `account.move`);
  - **renombrar la mesa no**: es `restaurant.tables.changed` y su tarjeta no se actualiza hasta otra lectura;
  - una unión necesitará su propio tipo de evento en esa lista.

#### Permisos

- **Dónde se registran**:
  - `administration/permission_catalog_entries.go`, con grupo, descripción y `requires` (el grupo «Mesas y cuentas»
    está en `:12-100`);
  - los conjuntos de fábrica están en `permission_catalog.go:208-290`.
- **«Restablecer permisos predeterminados»** (`role_defaults.go:52-115`) reemplaza **todo** el conjunto del rol.
- **Sesión con PIN**: lista cerrada `WaiterPermissions` (`waiter_scope.go:11-`), que se repite en `openapi.json`,
  `docs/API_V1.md` y `API_CONTRACTS.md` (lo vigila `waiter_scope_test.go`).
- **Cómo llegaron los permisos nuevos a las instalaciones existentes**:
  - las migraciones los conceden **solo al Administrador**: `0004:187-192`, `0006:293-295`, `0021:82-84` y `0028:60-63`;
  - con una excepción: `0025:113-116` concede `waiter_devices.use` a `administrator` **y `waiter`**;
  - los de para llevar (`takeaway.*`) son de la 0004 y la 0006, y solo al Administrador;
  - la 0026 a la 0030 no tocan roles; la K1 no agregó permisos (la pantalla usa la credencial del dispositivo);
  - el comentario de `permission_catalog.go:211-214` dice que una lista nueva «nunca cambia un rol sin que alguien toque
    el botón».
  - **Sin una concesión en la 0031, solo el Administrador del propietario tendría «Unir mesas»**: Mesero, Cajero y
    Supervisor tendrían que pulsar «Restablecer», que además borra sus ajustes.

#### Concurrencia

- **Lock de todas las escrituras de restaurante**: `operations.Handle` toma `pg_advisory_xact_lock(7820148305)`
  exclusivo antes de leer nada (`handler.go:54-60`). Lo hacen abrir una cuenta, confirmar un borrador, el pedido sin
  conexión, mover, combinar y cancelar.
- **Los demás también lo toman**: los cobros (`payments`), la pantalla de cocina, la importación y «Quitar datos de
  prueba».
- **Basta** para que dos terminales no ocupen la misma mesa en dos uniones: las operaciones quedan en serie y la segunda
  ve el estado de la primera.
- **Defensa en la base**: un índice único parcial en las mesas de las uniones abiertas (Decisión propuesta 2).
- **Lock nuevo**: no hace falta.

---

### 2. El importador (9H) y «Mesa 3», «3a, 3b, 3c» (§6.2 y §3)

#### Con el código

- **Lectura**: `legacyimport/load.go:264-268` lee de `mesas` solo `id`, `mesa` y `capacidad`.
- **Acomodo** (`buildTables`, `catalog_plan.go:415-461`):
  - omite la id 1 («para llevar», `:16-18`);
  - acomoda las demás **en el orden del paquete** en áreas «Salón», «Salón 2»… de 48 mesas, en una cuadrícula de 8 por
    renglón (celdas de 125 × 93, mesas de 100 × 72, `:25-37`);
  - la capacidad queda entre 1 y 40, y 4 si no tenía;
  - el nombre pasa por `cleanName` y `uniqueName` (exacto, con «(2)», `names.go:41-49`).
- **Segunda importación**:
  - reconoce las mesas por `legacy_id_map` (`entity='table'`, `legacy_key` = la id del anterior, `catalog_plan.go:146`);
  - **reescribe nombre, posición y capacidad** (`write_live.go:287-296`, `ON CONFLICT … x=excluded.x …`);
  - lo que ya no viene se borra o se desactiva (`:128-145`).
  - **Solo se importa sobre una base sin operación propia** (`clean.go:31-55`, también `restaurant_accounts`): al
    importar nunca hay cuentas, así que tampoco uniones abiertas.
- **Historial**: `app.legacy_sales` guarda `legacy_table_id`, `table_id` sin llave foránea y `table_name` copiado
  (`write_history.go:76-87`).
- **Sistema anterior** (`legacy-csharp/PoS`, sin leer `App.config`):
  - **no tiene ninguna idea de mesas unidas, grupos ni secciones**;
  - `mesas` es una lista plana (`PuntoDeVenta.cs:182`, `SELECT id, mesa FROM mesas WHERE id > 1`);
  - lo único que se parece es «Asignar mesa» (`AsignarMesa.cs`), que mueve un pedido de mesa.

#### Con el respaldo real (solo conteos)

- `mesas` tiene las columnas `id, mesa, capacidad` y **25 filas**:
  - **1 sin dígitos**: es la id 1, «para llevar»;
  - **20 «Mesa N»**;
  - **4 «Mesa N» + letra**;
  - **0** que sean solo un número y **0** otras.
- **Forma exacta**: las 24 tienen la forma «Mesa» + un espacio + número sin ceros a la izquierda (+ letra minúscula).
- **0** sin capacidad, **0** con espacios de más y **0** repetidas.
- **Las que nombró el propietario**:
  - **«Mesa 3» no existe**, ni tampoco «3»;
  - «3a», «3b» y «3c» no existen solas: **existen como «Mesa 3a», «Mesa 3b» y «Mesa 3c»** (letra minúscula y un
    espacio después de «Mesa»), las tres con capacidad;
  - **hay una cuarta sección del 3**, con la letra siguiente (d), también con capacidad.
- **Grupos que detectaría cada regla**:

  | Regla                                                     | Grupos                    | Dudosos o posibles falsos positivos |
  | --------------------------------------------------------- | ------------------------- | ----------------------------------- |
  | Estricta (base «Mesa N» o «N» y al menos dos «N» + letra) | **0** (la base no existe) | —                                   |
  | Sin base (Decisión propuesta 9)                           | **1** (el 3, 4 secciones) | 0                                   |

#### Propuesta de regla

Ver la Decisión propuesta 9. Con la regla estricta de la especificación, el grupo del propietario no se detectaría.

---

### 3. Reproducción de los defectos (§6.3)

#### §5.1 · Los renglones del escritorio no pasan a «Listo»

Reproducido con el Go temporal (`salida.txt`). Después de «Pedido listo» en la pantalla de cocina
(`POST kitchen-display/orders/ready`) **y también** después de «Listo» en Preparación del escritorio
(`POST kitchen/states`, como `PreparationBoard.tsx:60-75`), Go devuelve en las rutas que leen el escritorio y Android:

| Ruta                                                                                  | Antes      | Después |
| ------------------------------------------------------------------------------------- | ---------- | ------- |
| `GET restaurant/accounts/{id}`, `lines[].preparation_state`                           | `released` | `ready` |
| `GET restaurant/accounts?table_id=` y `restaurant/state`, `account.preparation_state` | `released` | `ready` |

**Causa: es de React, no de Go.** Go manda el estado correcto por renglón y Android lo pinta
(`OrderPanel.kt:513-519` → `WaiterBadges.kt:121`, «Listo»). El escritorio no lo pinta:

- **La captura** (`capture/BillLines.tsx:25-33`) pone siempre la etiqueta **«En cocina»** y deja el estado real como un
  texto gris al lado («En cocina · Listo»). Solo `cancelled` y `pending_release` cambian la etiqueta.
- **El panel de la mesa** (`TablePanel.tsx:53`) tiene **«En cocina» fijo** y no lee `preparation_state`. Es lo que vio
  el propietario: «todos los renglones siguen diciendo En cocina aunque se cierre y se abra».
- **La lista de cuentas** (`AccountList.tsx:24-35`) sí sigue el estado de la cuenta («Listo»).

La marca desde Preparación falla igual: es el mismo campo y el mismo dibujo.

**Prueba que falla**: `$SP/m1/repro/5.1-5.2-etiquetas-escritorio.patch` (Vitest,
`desktop/src/pages/restaurant/stageM1Lines.test.tsx`). Renderiza `ConfirmedLines` y `TablePanel` con renglones
`ready`, `preparing` y `released`. Hoy fallan las 4 pruebas (`Expected "Listo"`, `Received "En cocina"`; salida en
`5.1-vitest-salida.txt`).

#### §5.2 · Sin etiqueta para lo que no necesita preparación

Reproducido. La Coca Cola sin ruta y las papas «Sin preparación» se liberan **sin tareas**
(`kitchen_unit_releases` 1 y `kitchen_tasks` 0). En cuanto se envían, Go les da **`preparation_state = ready`**:
`kitchen.VersionPreparation` cuenta 0 tareas y 0 listas, y `total == ready` da «ready» (`kitchen/states.go:236`). Así
**no se distinguen de un platillo ya preparado**.

| Pantalla                       | Qué muestra hoy                                   |
| ------------------------------ | ------------------------------------------------- |
| Escritorio, captura            | «En cocina» con «Listo» en gris                   |
| Escritorio, panel de la mesa   | «En cocina»                                       |
| Android                        | **«Listo»** desde el primer momento               |
| Cuenta con solo esos productos | «Listo» en la cuenta (escritorio y Android)       |
| Cuenta mixta                   | Sigue la hamburguesa (`released` → `ready`), bien |

**Propuesta**: un campo nuevo de Go por renglón y por cuenta, `preparation_required` (Decisión propuesta 16). Así el
cliente no calcula nada.

**Pruebas que fallan**:

- **Go**: `$SP/m1/repro/5.2-sin-preparacion.patch` (`backend/internal/operations/stagem1_no_preparation_test.go`,
  `TestStageM1NoPreparationLines`). Hoy falla en sus 2 subpruebas: «`preparation_required` ausente;
  `preparation_state=ready`» (`5.2-go-salida.txt`).
- **Vitest**: la parte §5.2 del parche de §5.1.

#### §5.5 · Pantalla de la mesa en Android

- **Dónde se arma**: `TablesScreen.kt`:
  - `:126-127` filtra lo del mesero por mesa;
  - `:607-647` dibuja **«Lo tuyo en esta mesa»** (`strings.xml:52`): cada captura de Room y **cada pedido de la cola
    de esa mesa, también los que ya están `RECEIVED`**;
  - `:649-680` dibuja **«Cuentas abiertas»** (`strings.xml:26`): todas las cuentas de Go con `tableId == mesa`, con
    `AccountCard` (`:743-753`, «Mesa 2 · folio 27 · $280.00» + la etiqueta de la cuenta).
- **Por qué se ve dos veces**: un pedido recibido (`PendingOrder.receivedAccountId`, `OrderQueue.kt:300`) sigue en la
  cola, así que su cuenta aparece en las dos secciones.
- **«De ese mesero»**: hoy no se sabe. `AccountSummary` (`Accounts.kt:53-74`) no lee `assigned_user_id`, aunque Go lo
  manda en cada cuenta; solo trae `assignedName`. Las capturas y la cola sí son del mesero activo
  (`TablesViewModel.kt:39-50`).
- **Prueba que falla**: `$SP/m1/repro/5.5-tarjetas-mesa-android.patch` (JVM, `core:domain`):
  - `TableCards.kt`, una regla pura que hoy **reproduce exactamente** lo que dibuja `TablesScreen`;
  - `assignedUserId` opcional en `AccountSummary`;
  - `StageM1TableCardsTest`: fallan 2 de 3 (una sola tarjeta por cuenta con «Tu cuenta»; el pedido por enviar a una
    cuenta va en su tarjeta). La tercera, lo que no tiene cuenta como tarjeta propia, ya se cumple hoy y queda de guarda.
  - La Fase C reemplaza la regla y la usa en el panel. Salida en `5.5-android-salida.txt`.

---

### 4. Lo demás de la §5 (para el reparto)

| Punto                                   | Dónde                                                                                                                                                                                                                                                         | Hipótesis                                                                                                                                                                                                                                                            | Tamaño      |
| --------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------- |
| §5.3 Quitar la hora envía               | `kitchen/releases.go:338-357` (`account.updated`: sin hora deja la liberación en `pending`; K11-D9)                                                                                                                                                           | Si `scheduled_at` pasa a NULL y la liberación estaba `scheduled`, liberarla en la misma transacción (la ruta de `release`) en vez de `pending`                                                                                                                       | pequeño (A) |
| §5.4 Textos de Preparación              | `desktop/src/services/kitchen.ts:356` (`released: 'Liberado a preparación'`, que usan `PreparationTaskCard.tsx:34`, `BillLines`, `AccountKitchen.tsx` y `KitchenPage.tsx:195`); `PreparationBoard.tsx:197` («No hay unidades de preparación en este filtro.») | Cambiar los textos y revisar «unidad de preparación» en `pages/kitchen/`                                                                                                                                                                                             | pequeño (B) |
| §5.6 «Enviado · sin confirmar en coci…» | `StateBadge` con `maxLines=1` y elipsis (`WaiterBadges.kt:312-336`) dentro de `AccountCard`                                                                                                                                                                   | Dejar que la etiqueta salte de renglón o acortar el texto en la tableta vertical                                                                                                                                                                                     | pequeño (C) |
| §5.6 «Nuevo pe…»                        | `CaptureScreen.kt:286` (`capture_title_takeaway_new`, «Nuevo pedido para llevar») en la barra superior del teléfono                                                                                                                                           | Título en dos renglones o más corto en lo angosto                                                                                                                                                                                                                    | pequeño (C) |
| §5.6 «Terraz…»                          | `TablesScreen.kt:412-415` (`FilterChip`, `maxLines = 1`)                                                                                                                                                                                                      | Ancho mínimo o sin recorte; la fila ya se desliza                                                                                                                                                                                                                    | pequeño (C) |
| §5.6 «2 pedidos por en…»                | `PendingScreens.kt:239` (`pending_others_count`) y la insignia de Pendientes                                                                                                                                                                                  | Texto más corto o en dos renglones                                                                                                                                                                                                                                   | pequeño (C) |
| §5.6 «$0.00 · Borrador»                 | `RestaurantRepository.kt:158-199` (relectura 400 ms después del evento, `:341`)                                                                                                                                                                               | El panel pinta la cuenta guardada hasta la relectura: aplicar al momento el `Bill` que devuelve `confirm` en `openAccounts`. Medir con la marca de tiempo                                                                                                            | mediano (C) |
| §5.6 Contraste de las teclas del PIN    | `ReenterPinDialog.kt:111` → `core/ui/.../components/PinPad.kt`                                                                                                                                                                                                | Colores del tema claro dentro del diálogo; revisar contra AA                                                                                                                                                                                                         | pequeño (C) |
| §5.7 `takeaway t6` en Android 8         | `android/app/src/androidTest/.../TakeawayOnGoTest.kt:295-309`; A3-D32 (`docs/ETAPA_A3_AVANCE.md:2532-2543`)                                                                                                                                                   | El nombre llega a Go (`operations/takeaway.go:47-82` publica `restaurant.account.updated` en el ámbito `takeaway`) y en API 33 pasa. Sospecha: el canal de eventos en API 26 (SSE de OkHttp) o la relectura de la lista para llevar. Requiere `ADEL_A1_Tablet_API26` | mediano (C) |
| §5.8 `workflow-stage9d-accounts`        | `desktop/e2e/workflow-stage9d-accounts.spec.ts:326-332` (entra por la página y enseguida `auth/login` por la API con la misma terminal)                                                                                                                       | Mover el `call(driver,'POST','auth/login')` antes de `page.goto('/')`                                                                                                                                                                                                | pequeño (E) |
| §5.8 `workflow-stagel1-notices`         | `workflow-stagel1-notices.spec.ts:85-97` («al volver a entrar ese mismo día»); el día lo da `LicenseNotices.tsx:17-27` (zona del establecimiento)                                                                                                             | Fijar el reloj de la página (`page.clock`) a mediodía antes del paso; Go ya admite `ADEL_TEST_CLOCK_FILE` si hiciera falta                                                                                                                                           | pequeño (E) |
| §5.8 La cascada                         | `desktop/e2e/native-driver.ts:635-660` (`configured` sin terminal preparada en este proceso → «La aceptación debe comenzar con una base sin configurar»)                                                                                                      | Que cada especificación que necesita base limpia la pida al arnés (base nueva por especificación) o que el controlador recupere la terminal; revisar el `browser` de `iso.sh`                                                                                        | mediano (E) |

---

### 5. Decisiones propuestas

#### Modelo

1. **Las uniones son conjuntos de mesas; las cuentas siguen en sus mesas.** La 0031, sección A, agrega:
   - `app.table_unions`: `id`, `area_id`, `group_id` (nulo si es manual), `name` (el nombre vigente, calculado por
     Go), `state open|dissolved`, `opened_at`, `opened_by`, `opened_device_id`, `dissolved_at`, `dissolved_by`,
     `dissolve_reason` y `revision`;
   - `app.table_union_members`: `union_id`, `table_id`, `joined_at`, `joined_by`, `left_at` y `left_by`;
   - `app.table_union_events`: el historial de unir, agregar, quitar y deshacer, con quién, terminal y
     `operation_id`.

   **Una cuenta pertenece a la unión porque su `table_id` es una mesa miembro.** No hay columna nueva en
   `restaurant_accounts`.

   _Porqué_: todo lo que hoy lee `table_id` sigue igual (el cobro, la cocina, la cola, los reportes, la división y las
   cuentas que se reabren). Las cuentas cerradas no se tocan. Lo que se mueve con una cuenta a una mesa de la unión
   (mover, dividir hacia una cuenta nueva, un pedido sin conexión) queda en la unión sin código propio.
   - Al quitar una mesa que tiene cuentas, Go **la cambia de mesa dentro de la unión** (otro `table_id` miembro, en el
     mismo `UPDATE` de una cuenta abierta).
   - Al quedar una sola mesa, todas las cuentas pasan a ella. Así se cumple que «quitar una mesa no mueve ninguna
     cuenta» de la unión.

2. **Una mesa nunca está en dos uniones abiertas.** Se garantiza con dos cosas:
   - `CREATE UNIQUE INDEX … ON app.table_union_members(table_id) WHERE left_at IS NULL`;
   - deshacer la unión pone `left_at` a todos sus miembros.

   _Porqué_: es la última defensa si un camino de Go falla. La primera es el lock 7820148305.

3. **El nombre lo calcula Go.**
   - Las mesas se ordenan en **orden natural** (los números como números, sin distinguir mayúsculas): «2 + 10», nunca
     «10 + 2».
   - Si **todas** empiezan con la palabra «Mesa» (`documents.TableNameIncludesWord`), la unión se llama
     `"Mesa " + "5 + 6"`. Si no, `"5 + 6"`.
   - Una unión con **todas** las secciones de un grupo se llama como el grupo («Mesa 3»).
   - Los documentos, el KDS y la comanda usan `documents.TableContext(name)`: «Mesa 5 + 6» en los dos casos.
   - La capacidad es la suma de los `seats` de los miembros.

   _Porqué_: las 24 mesas del propietario se llaman «Mesa N». Sin esta regla su mapa diría «Mesa 5 + Mesa 6».

4. **Las ventas quedan en la unión sin tocar lo cerrado.** La 0031 agrega a `app.sales_recognitions` (las filas
   existentes quedan en NULL): `table_union_id uuid`, `table_union_name text` y `table_union_tables jsonb` (id y nombre
   de cada miembro). Las escribe `payments/recognition.go` al reconocer, con la unión vigente de `a.table_id`.
   - «Ventas de salón» agrupa por `coalesce(table_union_id, table_id)`, con la etiqueta `coalesce(table_union_name,
table_name)`: una sola fila «5 + 6», sin repartir la venta.
   - _Porqué_: es la instantánea de siempre (como `table_name`) y no exige leer las uniones históricas en cada reporte.

5. **Historial**:
   - en la unión, `table_union_events`;
   - en cada cuenta abierta de la unión, un `restaurant_events` por cambio («Se unió con la mesa 6», «Se agregó la mesa
     7», «Se quitó la mesa 6», «Se deshizo la unión»), con quién y la terminal, por `handler.event` y con el tipo nuevo
     `account.table_union`;
   - aparece en «Historial de la cuenta» sin rutas nuevas.

#### Mesas divisibles (§3)

6. **El grupo se guarda en dos tablas nuevas**:
   - `app.table_groups`: `id`, `area_id`, `name` (único por área, también frente a las mesas), `active`, `revision`,
     `source manual|legacy`, `legacy_batch_id` y `legacy_key`;
   - `app.table_group_sections`: `group_id`, `table_id` UNIQUE y `position`.
   - Cada sección sigue siendo una mesa normal.
   - Go valida que todas sean de la misma área y rechaza cambiar de área una mesa agrupada.
   - Se administra en Áreas y distribución con `restaurant.tables.manage`.

7. **El atajo «Mesa 3»** usa la misma unión.
   - Con todas las secciones libres es diferido como cualquier unión: Decisión propuesta 13 con `group_id`.
   - Con una sección ocupada, el atajo se rechaza: «La sección 3B está ocupada. Únela con las secciones libres para
     crecer.».
   - «Unir con…» desde la sección ocupada hacia las libres sí procede. Si las une todas, la unión se llama «Mesa 3».
   - Las combinaciones parciales se llaman «3A + 3B», o «Mesa 3a + 3b» con los nombres del propietario.

8. **En el mapa** el grupo se dibuja siempre con un marco tenue alrededor de sus secciones y su nombre. Cada sección
   sigue tocable.
   - La unión se dibuja con un contorno sólido, su nombre y la capacidad sumada. Las mesas unidas abren la unión.
   - Grupo y unión salen de `overview` (Decisión propuesta 14). El cliente no calcula nada más que el rectángulo con
     `fitTables` / `TableMapFit`.

9. **Regla del importador (9H)**: es una decisión del coordinador, porque la regla del encargo no detectaría el caso
   real.

   **Cuándo es un grupo**: al menos dos mesas `Mesa N<letra>` o `N<letra>` del mismo N, con letras únicas y
   consecutivas desde la a (sin distinguir mayúsculas), y **ninguna** mesa `Mesa N` ni `N`.

   **Qué se crea**:
   - un grupo «Mesa N» (`source legacy`, `legacy_key = "N"`) con esas secciones, en el orden de las letras;
   - **los nombres no cambian**: siguen «Mesa 3a»… (el historial tiene esos nombres y renombrar sería una decisión del
     propietario);
   - las secciones se acomodan **juntas y en el mismo renglón** de la cuadrícula. Si no caben en lo que queda del
     renglón, empiezan uno nuevo.

   **Qué se reporta como dudoso, en el informe y sin crear el grupo**:
   - si existe también `Mesa N` (no se sabe si esa mesa era la U completa ni qué hacer con su historial);
   - si las letras no son consecutivas;
   - si la letra está repetida;
   - si hay una sola sección.

   **Segunda importación**: los grupos `source legacy` se vuelven a calcular, porque la base siempre está sin operación
   propia. Los manuales se conservan y pierden las secciones que se retiren.

   **Con el respaldo real**: un grupo «Mesa 3» con **4** secciones (a, b, c y d), 0 dudosos.

   **La duda para el propietario**: dijo «3a, 3b, 3c» y en el paquete hay cuatro secciones.

#### Reglas en el servidor (§2.2 y §4)

10. **Errores**, todos con 409 salvo el permiso:

    | Código                         | Cuándo                                                                                        | Texto                                                                                            |
    | ------------------------------ | --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
    | `TABLE_UNION_AREA`             | Mesas de áreas distintas                                                                      | «Las mesas deben ser de la misma área.»                                                          |
    | `TABLE_UNION_OCCUPIED`         | Una mesa que se agrega está ocupada                                                           | «Solo se pueden unir mesas libres.» (en el detalle: «La mesa 6 ya está ocupada.», con la mesa)   |
    | `TABLE_UNION_BOTH_OCCUPIED`    | Se unen dos mesas ocupadas                                                                    | «La mesa 5 y la mesa 6 tienen cuentas abiertas. Para juntar sus cuentas usa «Combinar cuentas».» |
    | `TABLE_UNION_UNAVAILABLE`      | Mesa inactiva o fuera de servicio                                                             | «La mesa 6 está fuera de servicio o desactivada.»                                                |
    | `TABLE_IN_UNION`               | La mesa ya está en otra unión, o se abre una cuenta en una mesa unida que la terminal no veía | «La mesa 6 ya está unida en «Mesa 5 + 6».»                                                       |
    | `TABLE_GROUP_SECTION_OCCUPIED` | Atajo del grupo con una sección ocupada                                                       | «La sección 3B está ocupada. Únela con las secciones libres para crecer.»                        |
    | `TABLE_UNION_NOT_OPEN`         | La unión ya se deshizo                                                                        | «La unión ya se deshizo. Actualiza el mapa.»                                                     |
    | `TABLE_UNION_CONFLICT`         | Otra revisión                                                                                 | «La unión cambió en otra terminal. Actualiza el mapa.»                                           |
    | `FORBIDDEN`                    | Sin «Unir mesas»                                                                              | El 403 de siempre                                                                                |
    - **La mesa de inicio** puede estar libre u ocupada: si está ocupada, sus cuentas quedan en la unión por la
      Decisión propuesta 1.
    - **Quitar una mesa** no mueve cuentas fuera de la unión. Si queda una sola, la unión se deshace.

11. **La unión se deshace sola** cuando ya no le quedan cuentas abiertas. No se pone una llamada en cada ruta: se pone
    **al final de cada escritura** de `operations.Handle` (después de `dispatch`, con el 7820148305 ya tomado) y de
    `payments.Handle`. Ahí `tableunions.SettleEmpty(tx, actor)` deshace las uniones abiertas sin cuentas abiertas en sus
    miembros, con su historial y su evento.
    - Así cubre el cobro, el cierre, la cancelación, el descarte, la liberación, combinar cuentas, mover, dividir y
      transferir.
    - _Porqué_: un solo lugar que no se olvida en un camino nuevo.
    - La Fase A agrega una prueba de invariantes: ninguna unión abierta sin cuentas y ninguna con menos de dos mesas.
    - El coordinador decide si además se pone un disparador de restricción diferido que lo compruebe.
    - **Abierta por decidir**: una cuenta que se reabre por una devolución vuelve a su `table_id`. Si esa mesa ya está en
      otra unión, queda en esa unión.

12. **Permiso «Unir mesas»**:
    - clave `restaurant.tables.join`, en el grupo «Mesas y cuentas», con `requires: accounts.open,
restaurant.tables.read`;
    - en los conjuntos de fábrica de `waiter`, `cashier` y `supervisor` (el Administrador recibe todos), en
      `WaiterPermissions` y en la capacidad «Atender mesas» de la vista sencilla;
    - la 0031 lo inserta en `app.permissions` y **lo concede a los roles existentes `administrator`, `waiter`,
      `cashier` y `supervisor` que ya tengan sus `requires`**, como hizo la 0025 con `waiter`.
    - _Porqué_: la especificación lo pide en esos roles, y sin la migración el propietario tendría que pulsar
      «Restablecer», que además borra sus ajustes.
    - **Contradice el comentario de A3** (`permission_catalog.go:211-214`): lo decide el coordinador.

13. **Creación diferida**:
    - **Sin nada guardado**, la unión vive solo en la pantalla. El cliente pide su nombre, capacidad y problemas a
      `POST restaurant/table-unions/preview`, que no escribe y toma el lock compartido.
    - **Al guardar el primer producto**, viaja en `POST restaurant/accounts` con dos campos nuevos: `union_table_ids`
      (las otras mesas, o `union_group_id`) y `seen_union_id`. Go crea la unión y la cuenta en la misma operación. Si
      una mesa ya está ocupada o unida, responde 409 sin escribir nada y el cliente conserva la captura en pantalla,
      como hoy con `TABLE_HAS_ACCOUNT`.
    - **Al unir una mesa ya ocupada**, la unión nace en el momento (`POST restaurant/table-unions` con la mesa ocupada
      de inicio).

14. **Mesas inactivas o fuera de servicio**: no se unen.
    - Una mesa de una unión cuenta como ocupada para `tableQuery`: `account_count`, `state` y `balance` son los de la
      unión.
    - Así se aplican solas las reglas que ya existen: no desactivar, no poner fuera de servicio, no cambiar de área.

#### Concurrencia (§2.3)

15. **Orden de los locks**:
    - **No hace falta uno nuevo**: todas las rutas de uniones van por `operations.Handle`, con 7820148305 exclusivo
      antes de leer, y el índice único de la Decisión propuesta 2 es la red.
    - **Si dos terminales usan la misma mesa**, gana la primera en tomar el lock. La segunda recibe
      `TABLE_UNION_OCCUPIED` («La mesa 6 ya está ocupada.») o `TABLE_IN_UNION`, sin escribir nada.
    - **Abrir una cuenta en una mesa unida** sin haber visto la unión ni sus cuentas responde `TABLE_IN_UNION`. Los
      detalles traen la unión y sus cuentas para «agregar a esa cuenta» o «abrir otra en la unión».
      `requireNoUnseenAccounts` mira todas las cuentas de la unión.
    - **Revisor independiente**: lo pide la especificación.
    - **Prueba de concurrencia**: N terminales a la vez sobre las mismas mesas (abrir, unir, agregar, quitar, cobrar).
      Comprueba que ninguna mesa queda en dos uniones abiertas y que el número de cuentas abiertas es igual al de
      respuestas 200 que crearon cuenta.

#### API (§2.4)

16. **Rutas y campos nuevos**, todos compatibles:

    | Ruta                                                         | Qué hace                                                                                                                                                                                 |
    | ------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
    | `POST restaurant/table-unions/preview`                       | `{table_ids \| group_id}` → `{name, seats, table_ids (en orden natural), problems[]}`. No escribe                                                                                        |
    | `POST restaurant/table-unions`                               | `{operation_id, start_table_id, table_ids \| group_id}`. Solo con la mesa de inicio ocupada; con todas libres, 409 `TABLE_UNION_DEFERRED` («La unión se guarda con el primer producto.») |
    | `POST restaurant/table-unions/{id}/tables`                   | `{operation_id, revision, table_id}`. Agrega una mesa                                                                                                                                    |
    | `POST restaurant/table-unions/{id}/tables/{table_id}/remove` | `{operation_id, revision}`. Quita una mesa                                                                                                                                               |
    | `GET restaurant/table-unions/{id}`                           | La unión con sus miembros y su historial                                                                                                                                                 |
    | `GET/POST/PATCH restaurant/table-groups[/{id}]`              | Los grupos                                                                                                                                                                               |
    - **Idempotencia**: la de `restaurant_operations`, como todas.
    - **Eventos** en la misma transacción, ámbito `restaurant`:
      - `restaurant.table-unions.changed` por la regla genérica de `Handle` (`handler.go:94-99`), y
        `restaurant.table-groups.changed`;
      - por cada cuenta, `restaurant.account.table_union` (por `handler.event`), que **se agrega a
        `displayAccountKinds`** (`display.go:72-80`) para que el KDS vuelva a leer el encabezado.
      - Android y el escritorio se refrescan con cualquier evento del ámbito: no cambia su suscripción.
    - **Lecturas existentes**:
      - `overview` y `restaurant/state` agregan `unions[]` (`id`, `name`, `area_id`, `group_id`, `table_ids`, `seats`,
        `state`, `account_count`, `balance`, `occupied_since`, `revision`) y `groups[]`;
      - `Table` agrega `union_id` y `group_id`;
      - `Account` agrega `table_union` (`{id, name, table_ids}`, o null) y conserva `table_name` = su mesa, para la 0.4.1;
      - `listAccounts?table_id=` de una mesa unida devuelve las cuentas de la unión;
      - `ConfirmedLine` y `Account` agregan `preparation_required` (§5.2).
    - **Capacidad**: `table-unions-v1`. La Fase B y la Fase C esconden «Unir con…» y los grupos sin ella.
    - **Licencia** (`licensing/operations.go:275-319`): hoy una ruta que `classifyRestaurant` no conoce es
      `unknownOperation`, así que hay que agregar las nuevas:
      - unir, agregar y quitar → `RowCaptured` (F2, «reorganizar lo ya capturado», como `move` y `merge`);
      - `preview` y las lecturas → `RowRecords`;
      - `table-groups` → `RowAdministration`, como `tables` y `layout`;
      - la unión que nace con el primer producto viaja en `POST restaurant/accounts`, que ya es `RowCapture`.
      - No cambian los cupos.
    - **Tauri**: hay que agregar las órdenes nuevas en `desktop/src-tauri/src/api.rs` (Fase B, con su prueba de cargo).

#### Cola sin conexión de Android

17. **El destino sigue siendo la mesa** (`destination.table_id`) y se agrega `destination.seen_union_id` (opcional; la
    0.4.1 no lo manda). Cuando el pedido llega:

    | Caso                                                         | Qué hace Go                                                                                                                                                                                                                                                                                                                                                                                                                                                |
    | ------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
    | La mesa ahora está en una unión U                            | Va a U. `openAccountsWithItems` y las cuentas no vistas se buscan en todas las mesas de U. Partidas ya en una cuenta de U → `items_in_account` con esa cuenta. Cuentas de U que el mesero no veía → `table_has_account` aceptable, con el mensaje «La mesa 6 ahora está unida en «Mesa 5 + 6» y tiene cuentas abiertas (folio 27). Confirma para abrir otra cuenta en la unión o envía el pedido a esa cuenta.». Sin cuentas no vistas → cuenta nueva en U |
    | La unión que vio el mesero ya no existe y la mesa está libre | Cuenta nueva en la mesa, sin pedir confirmación                                                                                                                                                                                                                                                                                                                                                                                                            |
    | Se deshizo y la mesa la ocupó otra cuenta                    | `table_has_account`, como hoy                                                                                                                                                                                                                                                                                                                                                                                                                              |
    | La mesa está en otra unión, distinta de la que vio           | Igual que el primer caso, con el nombre nuevo                                                                                                                                                                                                                                                                                                                                                                                                              |
    | La mesa salió de la unión y quedó libre                      | Cuenta nueva en la mesa                                                                                                                                                                                                                                                                                                                                                                                                                                    |
    | Destino `account_id`                                         | Va a esa cuenta esté donde esté, o `account_closed` como hoy                                                                                                                                                                                                                                                                                                                                                                                               |
    - **No hay tipos de cambio nuevos**: la 0.4.1 entiende `table_has_account` y `items_in_account`, y «Requiere tu
      revisión» los muestra igual.
    - **Unir requiere conexión**: una unión que solo está en pantalla y pierde la conexión antes del primer producto
      **no se encola**. El pedido va a la mesa de inicio. Es una duda para el coordinador.

#### Documentos, reportes y cocina con «5 + 6»

18. **Un fragmento SQL común** en Go, por ejemplo `tableunions.PlaceSQL("a")` (como `payments.AccountStandingSQL`), da
    el nombre de la unión vigente de `a.table_id` o el de la mesa. Lo usan:

    | Lector                          | Dónde                                                                                       | Resultado                                                       |
    | ------------------------------- | ------------------------------------------------------------------------------------------- | --------------------------------------------------------------- |
    | Precuenta, ticket y comprobante | `payments/finance.go:28` → `TableContext`                                                   | «Mesa 5 + 6»                                                    |
    | Comanda                         | `kitchen/releases.go:51`, `context_snapshot.table` = `TableContext(nombre)` solo en uniones | «Salón · Mesa 5 + 6»                                            |
    | KDS                             | `display.go:591-596` y `cardHeader`, `title` = `TableContext(nombre)`                       | «Mesa 5 + 6» (la aplicación de cocina no cambia: no hay Fase D) |
    | Preparación del escritorio      | `kitchen/reads.go:131-136`                                                                  | El nombre de la unión                                           |
    | Avisos de platillos listos      | `ready_alerts.go:139`                                                                       | El nombre de la unión                                           |
    | «En curso» y cancelaciones      | `reports/sales_progress.go:78`, `reports/cancellations.go`                                  | El nombre de la unión                                           |
    | Reconocimiento de la venta      | `payments/recognition.go:118-125`                                                           | La instantánea de la Decisión propuesta 4                       |
    - **El nombre de las cuentas en la unión**:
      - una cuenta sin nombre abierta en la unión toma la referencia neutra = el nombre de la unión;
      - cuando la unión cambia, las cuentas abiertas que la conservan se renombran, como `movedAccountName`;
      - al deshacerse, vuelven al nombre de su mesa;
      - así el KDS no repite «Mesa 5» como subtítulo.

#### Escritorio y Android

19. **Escritorio (Fase B)**:
    - **«Unir con…»** en el menú de la mesa del mapa y de la lista:
      - se eligen las mesas libres de la misma área (las demás, deshabilitadas con su motivo de Go);
      - la vista previa de Go da el nombre y la capacidad;
      - el contorno y el nombre se dibujan sobre `TableMap`;
      - tocar cualquier mesa unida abre la unión (el panel de la «mesa» unión);
      - quitar o agregar mesas desde el panel.
    - **Grupos en Áreas y distribución**: crear, editar y quitar un grupo con sus secciones.
    - **Mensajes**: los textos de la Decisión propuesta 10, tal como los da Go.
    - **§5.1**: la etiqueta del renglón = `preparationLabel(state)` («Listo», «En preparación», «En cocina» solo para
      `released`) en `BillLines.tsx:25-33` y `TablePanel.tsx:53`.
    - **§5.2**: sin etiqueta si `preparation_required === false`.
    - **§5.4**: los textos de Preparación.

20. **Android (Fase C)**:
    - **Unir desde la tableta**: el mismo «Unir con…» en el panel de la mesa, solo con conexión y con la capacidad.
    - **Mapa**: el contorno en `TablesMap`, con el rectángulo de sus mesas por `TableMapFit`.
    - **Lista**: la unión es una sola entrada «Mesa 5 + 6».
    - **§5.5**:
      - la regla `TableCards`: una tarjeta por cuenta de Go con folio, total, envío y cocina, e «Imprimir precuenta»;
      - «Tu cuenta» si `assigned_user_id` = el mesero activo (se agrega `assignedUserId` al DTO y al mapeo);
      - un pedido de la cola o una captura **sin cuenta** es su propia tarjeta «Por enviar»;
      - los pedidos de la cola a una cuenta y los ya recibidos van dentro de su tarjeta;
      - se quita «Lo tuyo en esta mesa».
    - **§5.2**: `PreparationBadge` no se dibuja con `preparation_required == false`, por renglón y por cuenta.
    - **§5.6 y §5.7**: los de la tabla anterior.

#### Pruebas de la §7

21. **Dónde va cada una**:

    | Qué                                                                                                                                                                                              | Dónde                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
    | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
    | Servidor (unir, crecer, rechazos, agregar y quitar, deshacerse en cada camino, creación diferida, permiso, concurrencia, cola sin conexión, documentos y reportes con «5 + 6», nombre en el KDS) | Go de integración: un archivo nuevo `operations/stagem1_*_test.go` y `TestContractV1`                                                                                                                                                                                                                                                                                                                                                                                                                                     |
    | Reportes                                                                                                                                                                                         | `internal/reports`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
    | Importador                                                                                                                                                                                       | `internal/legacyimport`, con `MemorySource` y `legacysynth` con «Mesa 3a…»                                                                                                                                                                                                                                                                                                                                                                                                                                                |
    | §5.2                                                                                                                                                                                             | El parche de la Fase 0                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
    | §5.3                                                                                                                                                                                             | Go de integración nueva. No ubiqué una prueba existente que afirme que «queda pendiente» al quitar la hora: la Fase A revisa `kitchen_scheduling_test.go` y `stagek11_*_test.go`                                                                                                                                                                                                                                                                                                                                          |
    | Escritorio                                                                                                                                                                                       | Vitest (el parche §5.1 y §5.2) y Playwright `workflow-stagez-m1-*.spec.ts` (unir desde el mapa, contorno, grupo, rechazos, etiquetas) en Chromium y WebKit                                                                                                                                                                                                                                                                                                                                                                |
    | Android                                                                                                                                                                                          | JVM (`core:domain`: el parche §5.5, la regla del contorno y el nombre) y de extremo a extremo en las dos tabletas                                                                                                                                                                                                                                                                                                                                                                                                         |
    | Suites existentes que tocan los cambios                                                                                                                                                          | `operations` (tablas 7.1, 9D, 9G, A1 y offline), `kitchen` (display, K1), `payments` (reconocimiento, documentos), `reports`, `administration` (catálogo de permisos, `waiter_scope`, `role_defaults`), `legacyimport`; Playwright `workflow-stage9d-topbar-tables`, `workflow-stage9f-topbar-map`, `workflow-stage9c-tables-time`, `workflow-stage9d-accounts` y `restaurant.spec`; Vitest `tablePanel`, `stage9dTables` y `stage9dAccounts`; Android `StageA3PanelRulesTest`, `WaiterRulesTest` y `OrderQueueRulesTest` |

#### Reparto de fases (dos agentes a la vez)

22. **Orden y paralelo**:
    - **Primero, un contrato breve** que fija el coordinador antes de A, B y C: rutas, cuerpos, campos nuevos de
      `overview`, `state`, `Table`, `Account` y `ConfirmedLine`, códigos y textos de error, eventos, capacidad, reglas
      del nombre y regla de la cola. Con eso B y C trabajan contra un servidor simulado y se prueban al final contra A.
    - **En paralelo 1: A y B.**
      - **A, servidor**: la 0031, uniones, grupos, `SettleEmpty`, nombres en documentos, KDS y reportes, cola sin
        conexión, permiso, importador, §5.2 (`preparation_required`) y §5.3.
      - **B, escritorio**: §5.1, §5.2 y §5.4, y después uniones y grupos contra el contrato. Los Playwright de unir
        esperan a A.
    - **En paralelo 2: C y E.**
      - **C, Android**: §5.5 y §5.6 primero (no dependen de A, salvo `assignedUserId`, que Go ya manda), después unir
        y contorno, y §5.7 con el emulador API 26.
      - **E, pruebas frágiles**: solo especificaciones de Playwright y el controlador.
    - **D no hace falta**: el KDS muestra `title` tal cual.

#### Riesgos y dudas para el coordinador

23. **Decisiones funcionales que no están en la especificación**:
    1. **El importador y el caso real**: el propietario dijo «Mesa 3 y 3a, 3b, 3c»; el paquete trae «Mesa 3a» a «Mesa
       3d» y **no hay «Mesa 3»**. ¿Grupo de 4 secciones con la regla sin base? ¿Renombrar a «3A»…? Propuesta: no
       renombrar.
    2. **El permiso a los roles existentes** en la 0031 contradice el comentario de A3 (Decisión propuesta 12).
    3. **El nombre con «Mesa»**: «Mesa 5 + 6» en el mapa cuando las mesas se llaman «Mesa N». ¿O «5 + 6» y solo los
       documentos con «Mesa»?
    4. **El filtro de mesa en los reportes** (`filters.go`): ¿una venta de «5 + 6» aparece al filtrar por la mesa 6?
       Propuesta: sí, por `table_union_tables`.
    5. **Una cuenta reabierta** por una devolución cuya mesa ya está en otra unión queda en esa unión. ¿Correcto?
    6. **Unir sin conexión** en Android no se ofrece. ¿Se acepta?
    7. **§5.2 a nivel de cuenta**: una cuenta con solo productos sin preparación no lleva etiqueta de cocina en la lista
       ni en el panel. Hoy dice «Listo».
    8. **«Tu cuenta»** = el responsable de la cuenta (`assigned_user_id`), no quien la abrió. Es lo que usa el aviso de
       listo (K1-D5).
    9. **Una captura en curso de una cuenta nueva** en Android también es su propia tarjeta («Sin enviar»), además del
       pedido «Por enviar».

    **Riesgos técnicos**:
    - la 0.4.1 de los teléfonos no conoce las uniones: ve la mesa de inicio con las cuentas y las demás mesas
      «Ocupadas» sin cuentas, por la Decisión propuesta 14;
    - abrir una cuenta en una mesa unida desde la 0.4.1 recibe `TABLE_HAS_ACCOUNT` o `TABLE_IN_UNION`, y debe
      entenderse con su manejo genérico: verificarlo en la Fase C con la 0.4.1 instalada;
    - renombrar la mesa no actualiza la tarjeta del KDS (es de antes);
    - el `PATCH` de una mesa puede cambiarle el área con cuentas (es de antes): la Fase A lo cierra para las uniones y
      los grupos.

---

### Pruebas realmente ejecutadas

| Qué                                                                                                                    | Resultado                                                                             |
| ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| `legacy-prepare` con el respaldo real (copia de `etapa-m1-p0`, Docker local con la imagen ya descargada)               | Paquete creado (salida 0); solo se leyó `mesas`; conteos arriba                       |
| `legacy-prepare --cleanup`, `docker ps -a`, `docker volume ls`                                                         | Sin restos, sin contenedores ni volúmenes; `$SP/legacy-data/` borrada; Docker cerrado |
| `reproducir.py` contra el Go temporal P0 (base nueva; dos corridas, la última en `salida.txt`)                         | Reproduce §5.1 (KDS y Preparación) y §5.2                                             |
| `go-int ./internal/operations` con `ISO_RUN=TestStageM1NoPreparationLines` (parche §5.2)                               | **Falla** como se espera: 2 de 2 subpruebas                                           |
| `vitest run src/pages/restaurant/stageM1Lines.test.tsx` (parche §5.1 y §5.2; `tsc --noEmit` sin errores en el archivo) | **Falla** como se espera: 4 de 4                                                      |
| `gradle --offline :core:domain:test --tests …StageM1TableCardsTest` (parche §5.5, con `ktfmtFormat`)                   | **Falla** como se espera: 2 de 3; la tercera es de guarda                             |

**No verificado**:

- §5.6 y §5.7: solo se ubicó el código, sin emulador;
- el comportamiento real de la 0.4.1 frente a una unión;
- la regla del importador: solo se contó, sin una importación de prueba.

### Archivos

- `$SP/m1/repro/reproducir.py` (guion), `salida.txt` (su salida) y `bin/license-test` (compilado para el guion).
- `$SP/m1/repro/5.2-sin-preparacion.patch` y `5.2-go-salida.txt`.
- `$SP/m1/repro/5.1-5.2-etiquetas-escritorio.patch` y `5.1-vitest-salida.txt`.
- `$SP/m1/repro/5.5-tarjetas-mesa-android.patch` y `5.5-android-salida.txt`.
- `$SP/m1/importador-conteos.txt` (solo los conteos del respaldo real).

## Decisiones del coordinador al cerrar la Fase 0

Se aceptan las decisiones propuestas 1 a 22 de la Fase 0, con las precisiones de abajo. El contrato que siguen las fases
A, B y C es el de «Contrato de la etapa» (más abajo en este avance). Las dudas de la Fase 0 (punto 23) se resuelven así:

- **M1-D1. Modelo de uniones** (propuestas 1, 2 y 5). Una unión es un conjunto de mesas
  (`app.table_unions`, `app.table_union_members`, `app.table_union_events`). Una cuenta es de la unión porque su
  `table_id` es una mesa miembro: no hay columna nueva en `restaurant_accounts`. Al quitar la mesa que tiene las
  cuentas, Go las cambia a otra mesa de la unión. Así no salen de ella, como pide la §1.1. Al quedar una sola mesa, la
  unión se deshace y las cuentas quedan en esa mesa. Un índice único parcial impide que una mesa esté en dos uniones
  abiertas. El historial queda en la unión y, por cada cuenta abierta, en `restaurant_events` con el tipo
  `account.table_union`.
- **M1-D2. Nombre de la unión** (propuesta 3). Lo calcula Go:
  - las mesas van en orden natural, sin distinguir mayúsculas («2 + 10»);
  - si todas empiezan con la palabra «Mesa», el nombre es «Mesa 5 + 6»; si no, «5 + 6»;
  - una unión con todas las secciones de un grupo, y nada más, se llama como el grupo («Mesa 3»);
  - la capacidad es la suma.
    El mapa y la lista muestran ese nombre tal cual. Los documentos, la comanda y el KDS lo pasan por
    `documents.TableContext`, así que dicen «Mesa 5 + 6» con cualquiera de las dos formas de nombrar las mesas. Con las
    mesas del propietario («Mesa N»), el mapa dirá «Mesa 5 + 6». Es el mismo criterio que hoy: cada mesa se ve con su
    nombre.
- **M1-D3. Ventas y reportes** (propuesta 4). `sales_recognitions` guarda la instantánea de la unión al reconocer la
  venta (`table_union_id`, `table_union_name`, `table_union_tables`). Las filas existentes quedan en NULL y no se
  reescribe ninguna. «Ventas de salón» agrupa por unión sin repartir la venta. **Duda 4: sí**: al filtrar por la mesa 6
  aparecen las ventas de las uniones en las que estuvo, con el nombre de la unión.
- **M1-D4. Mesas divisibles** (propuestas 6, 7 y 8). Los grupos van en `app.table_groups` y
  `app.table_group_sections`. Cada sección es una mesa normal y no cambia de nombre. El atajo del grupo es una unión
  diferida con `group_id`. Una sección ocupada lo impide y da el mensaje de la §4. En el mapa, el grupo se dibuja con un
  marco tenue y su nombre, y la unión con un contorno sólido.
  - Para quitar un grupo se manda `PATCH active: false`: ya no se lee en ningún lado y su nombre vuelve a estar libre.
  - No se cambia un grupo con una sección en una unión abierta (`TABLE_GROUP_IN_USE`).
  - Una mesa agrupada no cambia de área.
- **M1-D5. Importador** (propuesta 9, ampliada). La §3 pide convertir «Mesa 3» y «3a, 3b, 3c», y la §7 pide probarlo
  con un paquete que tenga las cuatro. El respaldo real trae «Mesa 3a» a «Mesa 3d» y ninguna «Mesa 3». La regla cubre
  los dos casos:
  - **Hay grupo** cuando existen al menos dos mesas «Mesa N»+letra o «N»+letra del mismo N, con letras únicas y
    consecutivas desde la a (sin distinguir mayúsculas). Se crea el grupo «Mesa N» (o «N» si las secciones no llevan
    la palabra), con las secciones en el orden de las letras y **sin renombrarlas**. Se acomodan juntas en la
    cuadrícula, en un mismo renglón si caben.
  - **Si además existe la mesa «Mesa N» (o «N»)**, esa mesa se vuelve el grupo, como dice la §3. No se crea como mesa
    viva: su nombre lo toma el grupo y su historial de ventas sigue en `app.legacy_sales` con el nombre copiado. El
    informe lo dice.
  - **Se reporta como dudoso, sin crear el grupo**: letras no consecutivas o repetidas, o una sola sección. El
    propietario arma el grupo a mano en Áreas y distribución.
  - Una segunda importación vuelve a calcular los grupos del sistema anterior (`source legacy`).
  - Con el respaldo real resulta 1 grupo «Mesa 3» de 4 secciones, sin dudosos. En la entrega se le dice al propietario
    que hay una cuarta sección (3d).
- **M1-D6. Reglas y errores** (propuesta 10). Códigos y textos exactos del contrato. Una mesa se nombra con
  `documents.TableContext` («la Mesa 6»), así que nunca dice «mesa Mesa 6». Empezar desde una mesa libre y agregar una
  ocupada se rechaza con «Solo se pueden unir mesas libres…». Para crecer se empieza desde la ocupada.
- **M1-D7. La unión se deshace sola** (propuesta 11). Una sola llamada (`SettleEmpty`) al final de cada escritura de
  `operations.Handle` y de `payments.Handle`, con 7820148305 tomado.
  - No se pone disparador diferido. La Fase A agrega una prueba de invariantes: ninguna unión abierta sin cuentas
    abiertas ni con menos de dos mesas.
  - «Quitar datos de prueba» también quita las uniones de las cuentas de prueba, y los grupos y las secciones de las
    mesas de prueba.
  - **Duda 5: sí**: una cuenta que una devolución reabre vuelve a su mesa. Si esa mesa está en una unión abierta,
    queda en esa unión.
- **M1-D8. Permiso «Unir mesas»** (propuesta 12). Clave `restaurant.tables.join`, en el grupo «Mesas y cuentas», con
  `requires: accounts.open, restaurant.tables.read`.
  - Va en los conjuntos de fábrica de Mesero, Cajero y Supervisor (el Administrador recibe todos), en
    `WaiterPermissions` y en «Restablecer».
  - **La 0031 lo concede a los roles existentes** con código `administrator`, `waiter`, `cashier` y `supervisor` que ya
    tengan sus `requires`, igual que la 0025 con `waiter_devices.use`. Solo agrega, no quita nada.
  - El comentario de `permission_catalog.go:211-214` se actualiza: una lista nueva de fábrica sigue sin cambiar roles,
    pero un permiso nuevo que pide la especificación sí se concede en su migración.
- **M1-D9. Creación diferida** (propuesta 13). Sin nada guardado, la unión vive solo en la pantalla y Go solo da la
  vista previa (`POST restaurant/table-unions/preview`, no escribe). Al guardar el primer producto, la unión nace en
  `POST restaurant/accounts` con `union_table_ids` o `union_group_id`. Una mesa que se ocupó mientras tanto da 409 sin
  guardar nada, y el cliente conserva lo capturado.
  - **Clientes anteriores**: `seen_union_id` es opcional. Si el campo **no viene**, Go revisa `seen_account_ids`
    contra todas las cuentas de la unión (`TABLE_HAS_ACCOUNT`, que la 0.4.1 ya maneja). Si **viene** (`null` u otra
    id) y no es la unión vigente de la mesa, responde `TABLE_IN_UNION`.
- **M1-D10. Ocupación** (propuesta 14). Cada mesa de una unión se lee como ocupada, con el estado, el número de cuentas
  y el saldo de la unión. Las reglas de siempre (no desactivar, no poner fuera de servicio) se aplican solas. Además,
  una mesa en una unión no cambia de área (`TABLE_IN_UNION`).
- **M1-D11. Concurrencia** (propuesta 15). No hay lock nuevo: basta 7820148305 exclusivo y el índice único. La revisa
  un revisor independiente después de la Fase A, con su prueba de varias terminales a la vez.
- **M1-D12. API** (propuesta 16). La del contrato, con la capacidad `table-unions-v1`, filas nuevas en la tabla de la
  licencia y sin cambio de cupos.
- **M1-D13. Cola sin conexión** (propuesta 17, con un cambio en la duda 6). Va la tabla de casos del contrato. No hay
  tipos de cambio nuevos.
  - **Duda 6, decisión**: unir requiere conexión, y una unión que solo está en pantalla **nunca entra en la cola**.
  - Si Android pierde la conexión antes de guardarla, no cambia el pedido sin avisar. Conserva lo capturado y dice:
    «Sin conexión no se pueden unir mesas. Puedes enviar el pedido solo a la Mesa 5 o esperar a tener conexión.».
    Los botones son «Enviar solo a la Mesa 5» y «Esperar».
  - Así se cumple la §2.3: no se guarda nada a medias, se dice por qué y no se pierde lo capturado.
- **M1-D14. Documentos y cocina** (propuesta 18). Todos los lectores usan un mismo fragmento SQL de Go para el lugar de
  la cuenta (la unión vigente o la mesa). Una cuenta sin nombre en una unión toma el nombre de la unión y lo pierde al
  deshacerse, como `movedAccountName`. `restaurant.account.table_union` entra en `displayAccountKinds`.
  **No hay Fase D**: la aplicación de cocina pinta el título que manda Go. Cocina queda en 0.1.0.
- **M1-D15. §5.1 y §5.2**. La etiqueta del renglón del escritorio sigue el estado de Go, con los mismos textos que
  Android. Go agrega `preparation_required` por renglón y por cuenta: es falso cuando ninguna unidad del renglón
  necesita preparación después del envío. Si es falso, el renglón no lleva etiqueta de cocina, ni en el escritorio ni
  en Android. **Duda 7: sí**: una cuenta con solo productos así tampoco lleva etiqueta de cocina. Los combos siguen
  igual.
- **M1-D16. §5.5** (propuesta 20). Una tarjeta por cuenta de Go. **Duda 8: sí**: «Tu cuenta» sale cuando
  `assigned_user_id` es el mesero activo. **Duda 9: sí**: una captura o un pedido de la cola sin cuenta es su propia
  tarjeta, con la misma etiqueta «Por enviar».
- **M1-D17. §5.3**. Al quitar la hora a un pedido programado, Go lo libera en la misma transacción, como un pedido sin
  hora. Reemplaza K11-D9.
- **M1-D18. §5.4**. En Preparación:
  - la tarjeta liberada dice «Por preparar», para no confundirse con «En preparación» (`preparing`);
  - el filtro vacío dice «No hay pedidos con este filtro.»;
  - el resto de Preparación pasa por la misma regla de lenguaje sencillo.
    En las vistas de la cuenta, `released` sigue diciendo «En cocina».
- **M1-D19. Reparto**:
  - la Fase A (servidor) corre a la vez que la Fase E, que ya está en curso;
  - al terminar la E empieza la C (Android): primero lo que no depende del servidor nuevo (§5.5, §5.6 y §5.7) y luego
    las uniones;
  - al terminar la A, el revisor independiente de la concurrencia;
  - después la B (escritorio);
  - al final la F.
    Siempre son dos agentes a la vez como máximo. No hay Fase D.

## Contrato de la etapa

Lo fija el coordinador con las decisiones anteriores; la Fase A lo implementa y las fases B y C lo usan.

### 1. Capacidad y permiso

- Capacidad nueva en `GET version`: **`table-unions-v1`**. Sin ella, el escritorio y Android esconden «Unir con…», el
  contorno de unión, el marco de grupo, el atajo del grupo y la administración de grupos.
- Permiso nuevo **`restaurant.tables.join`** («Unir mesas»), grupo «Mesas y cuentas», `requires: accounts.open,
restaurant.tables.read`. Lo piden las rutas de §3 que escriben uniones y `union_table_ids`/`union_group_id` en
  `openAccount`. Va en `WaiterPermissions`.
- `preparation_required` (§4) y los campos de lectura **no** dependen de la capacidad: un cliente que no los recibe los
  trata como `true` / `null`.

### 2. Objetos

```jsonc
// TableUnion (en overview, restaurant/state y las respuestas de §3)
{
  "id": "uuid",
  "name": "Mesa 5 + 6",          // lo calcula Go (M1-D2); el cliente lo muestra tal cual
  "area_id": "uuid",
  "group_id": "uuid | null",     // con todas las secciones de un grupo, name = nombre del grupo
  "table_ids": ["uuid", "uuid"], // en orden natural del nombre
  "seats": 8,                    // suma de los asientos
  "state": "occupied | awaiting_payment | partial_payment", // el más avanzado de sus cuentas, como una mesa
  "account_count": 1,
  "balance": "280.00",
  "occupied_since": "RFC 3339",
  "occupied_seconds": 1234,
  "long_open": false,
  "revision": 3
}
// TableGroup (en overview y restaurant/state; solo los activos)
{ "id": "uuid", "area_id": "uuid", "name": "Mesa 3", "table_ids": ["3a", "3b", "3c"] /* en orden de sección */,
  "seats": 12, "revision": 1 }
```

- **`Table`** (overview, state): + `union_id` (uuid o `null`) y `group_id` (uuid o `null`). Una mesa de una unión
  tiene `state`, `account_count`, `balance`, `occupied_since`, `occupied_seconds` y `long_open` **de la unión**
  (M1-D10).
- **`getRestaurantOverview`** y **`restaurant/state`**: + `unions: TableUnion[]` (solo abiertas) y
  `groups: TableGroup[]` (solo activos).
- **`Account`**: + `table_union` (`{ "id", "name", "table_ids" }` o `null`); `table_id`/`table_name` siguen siendo los
  de su mesa (para la 0.4.1). Una cuenta sin nombre propio en una unión se llama como la unión (M1-D14). +
  `preparation_required: boolean` (§4).
- **`ConfirmedLine`**: + `preparation_required: boolean` (§4).
- **`listAccounts?table_id=X`**: si X está en una unión abierta, devuelve las cuentas abiertas de toda la unión.
- **`getTableUnion`** (`GET restaurant/table-unions/{id}`): el `TableUnion` (también disuelta, con `state:
"dissolved"`) + `tables: [{id, name, seats}]` + `history: [{at, kind: "joined" | "table_added" | "table_removed" |
"dissolved", table_id, table_name, user_name, device_name}]`.

### 3. Rutas nuevas (todas en `restaurant/*`, `operations.Handle`, 7820148305; idempotencia de `restaurant_operations`)

| operationId            | Ruta                                                          | Cuerpo → respuesta                                                                                                                                                                                                                           | Permiso                    |
| ---------------------- | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------- |
| `previewTableUnion`    | `POST /restaurant/table-unions/preview`                       | `{table_ids: [todas, ≥2]}` o `{group_id}` o `{union_id, table_ids: [las que se agregan]}` → 200 `{name, seats, table_ids, problems: [{code, message, table_id \| null}]}`. **No escribe** (lock compartido). `problems` vacío = se puede     | `restaurant.tables.join`   |
| `createTableUnion`     | `POST /restaurant/table-unions`                               | `{operation_id, start_table_id, table_ids: [las demás]}` o `{operation_id, start_table_id, group_id}` → 200 `{union}`. **Solo con la mesa de inicio ocupada** (sus cuentas quedan en la unión); con todas libres, 409 `TABLE_UNION_DEFERRED` | `restaurant.tables.join`   |
| `addTableToUnion`      | `POST /restaurant/table-unions/{id}/tables`                   | `{operation_id, revision, table_id}` → 200 `{union}`                                                                                                                                                                                         | `restaurant.tables.join`   |
| `removeTableFromUnion` | `POST /restaurant/table-unions/{id}/tables/{table_id}/remove` | `{operation_id, revision}` → 200 `{union, dissolved: boolean}` (si quedó una sola mesa, `dissolved: true` y `union.state: "dissolved"`)                                                                                                      | `restaurant.tables.join`   |
| `getTableUnion`        | `GET /restaurant/table-unions/{id}`                           | → 200 `{union}` (§2)                                                                                                                                                                                                                         | `restaurant.tables.read`   |
| `createTableGroup`     | `POST /restaurant/table-groups`                               | `{operation_id, area_id, name, table_ids: [≥2, en orden]}` → 200 `{group}`                                                                                                                                                                   | `restaurant.tables.manage` |
| `updateTableGroup`     | `PATCH /restaurant/table-groups/{id}`                         | `{revision, name?, table_ids?, active?}` → 200 `{group}`; `active: false` lo quita (M1-D4)                                                                                                                                                   | `restaurant.tables.manage` |

- **`openAccount`** (`POST /restaurant/accounts`) agrega, opcionales:
  - `union_table_ids: [uuid]` (las otras mesas) **o** `union_group_id: uuid` (`table_id` debe ser una sección del
    grupo): crea la unión y la cuenta en la misma operación (creación diferida, M1-D9); todas las mesas deben estar
    libres; si no, 409 sin guardar nada;
  - `seen_union_id: uuid | null`: la unión que el cliente veía en esa mesa (M1-D9: sin el campo, comportamiento de la
    0.4.1 con `seen_account_ids` revisado contra toda la unión).
- **Escritorio**: cada ruta nueva necesita su orden en `desktop/src-tauri/src/api.rs` (lista cerrada) con su prueba de
  `cargo`.

### 4. `preparation_required` (§5.2, M1-D15)

- **Renglón**: `false` si el renglón ya se liberó y **ninguna** de sus unidades generó tarea de preparación (producto
  sin ruta con «No necesitan preparación», ruta «Sin preparación»). Antes de liberarse (programado, pendiente) es
  `true`. Un combo con algún componente con tarea es `true` (los combos no cambian).
- **Cuenta**: `true` si al menos un renglón vigente es `true`; una cuenta sin renglones confirmados, `true`.
- **Clientes**: con `false`, ninguna etiqueta de cocina (ni en el renglón ni en la cuenta). Con `true`, la etiqueta del
  estado de Go: `pending_release` «Enviado · sin confirmar en cocina», `scheduled`, `released` «En cocina»,
  `preparing` «En preparación», `ready` «Listo», `cancelled` «Cancelado», y lo que ya diga Android para entregado
  (los textos del escritorio y de Android deben coincidir: revisa `WaiterBadges.kt`).

### 5. Errores (409 salvo que se diga; `{error: {code, message, details}}`; `message` exacto, para mostrarse)

`{mesa}` = `documents.TableContext(nombre)` («Mesa 6» con una mesa «6» o «Mesa 6»). `{unión}` = nombre de la unión.
`details` lleva `table_id` (y `union_id` cuando aplica).

| Código                         | Cuándo                                                                                                                    | `message`                                                                                               |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `TABLE_UNION_AREA`             | Mesas de áreas distintas (también en grupos)                                                                              | «Las mesas deben ser de la misma área.»                                                                 |
| `TABLE_UNION_OCCUPIED`         | Una mesa que se agrega tiene cuentas abiertas (también al guardar una unión diferida o por una carrera con otra terminal) | «Solo se pueden unir mesas libres: la {mesa} ya está ocupada.»                                          |
| `TABLE_UNION_BOTH_OCCUPIED`    | La de inicio y otra tienen cuentas abiertas                                                                               | «La {mesa A} y la {mesa B} tienen cuentas abiertas. Para juntar sus cuentas usa «Combinar cuentas».»    |
| `TABLE_UNION_UNAVAILABLE`      | Mesa inactiva o fuera de servicio                                                                                         | «La {mesa} está fuera de servicio o desactivada.»                                                       |
| `TABLE_IN_UNION`               | La mesa ya está en otra unión; `seen_union_id` no coincide; cambiar de área una mesa unida                                | «La {mesa} ya está unida con otras mesas ({unión}).» `details`: `union` (TableUnion) y `accounts` (ids) |
| `TABLE_GROUP_SECTION_OCCUPIED` | Atajo del grupo con una sección ocupada                                                                                   | «La sección {nombre de la mesa} está ocupada. Únela con las secciones libres para crecer.»              |
| `TABLE_UNION_NOT_OPEN`         | La unión ya se deshizo                                                                                                    | «Esta unión de mesas ya se deshizo.»                                                                    |
| `TABLE_UNION_CONFLICT`         | Revisión distinta                                                                                                         | «La unión cambió en otra terminal. Revisa las mesas e inténtalo otra vez.»                              |
| `TABLE_UNION_DEFERRED`         | `createTableUnion` con todas libres                                                                                       | «La unión se guarda con el primer producto.»                                                            |
| `TABLE_GROUP_NAME_TAKEN`       | Nombre de grupo repetido en el área (frente a mesas y grupos activos)                                                     | «Ya hay una mesa o un grupo con ese nombre en esta área.»                                               |
| `TABLE_GROUP_SECTION_TAKEN`    | La mesa ya es sección de otro grupo activo                                                                                | «La {mesa} ya es parte del grupo {grupo}.»                                                              |
| `TABLE_GROUP_IN_USE`           | Cambiar o quitar un grupo con una sección en una unión abierta                                                            | «El grupo tiene mesas unidas. Cámbialo cuando se liberen.»                                              |
| `TABLE_GROUP_SECTION_AREA`     | Cambiar de área una mesa agrupada                                                                                         | «Quita la mesa del grupo antes de cambiarla de área.»                                                   |
| 400 `OPERATION_VALIDATION`     | Menos de dos mesas, mesa repetida, ids inválidos                                                                          | el de siempre                                                                                           |
| 403 `FORBIDDEN`                | Sin «Unir mesas»                                                                                                          | el de siempre                                                                                           |

### 6. Eventos (misma transacción, ámbito `restaurant`)

- `restaurant.table-unions.changed` (regla genérica de `Handle`) y `restaurant.table-groups.changed`.
- `restaurant.account.table_union` por cada cuenta abierta afectada (unir, agregar, quitar, deshacer); entra en
  `displayAccountKinds` (el KDS vuelve a leer su encabezado).
- El escritorio y Android ya se refrescan con cualquier evento del ámbito: no cambian su suscripción.

### 7. Cola sin conexión (`restaurant/offline-orders`, M1-D13)

`destination` agrega `seen_union_id` (opcional; la 0.4.1 no lo manda). Al llegar un pedido con `table_id`:

| Caso                                                         | Qué hace Go                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| ------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| La mesa está ahora en una unión U                            | Va a U: las cuentas no vistas y las partidas ya recibidas se buscan en **todas** las mesas de U. Partidas ya en una cuenta de U → `items_in_account`. Cuentas de U no vistas → `table_has_account` (aceptable, «Requiere tu revisión»), con «La {mesa} ahora está unida con otras mesas ({unión}) y tiene cuentas abiertas. Confirma para abrir otra cuenta en la unión o envía el pedido a esa cuenta.». Sin cuentas no vistas → cuenta nueva en U |
| La unión que vio el mesero ya no existe y la mesa está libre | Cuenta nueva en la mesa, sin pedir confirmación                                                                                                                                                                                                                                                                                                                                                                                                     |
| Se deshizo y la mesa la ocupó otra cuenta                    | `table_has_account`, como hoy                                                                                                                                                                                                                                                                                                                                                                                                                       |
| La mesa está en otra unión distinta de la que vio            | Como el primer caso, con el nombre nuevo                                                                                                                                                                                                                                                                                                                                                                                                            |
| La mesa salió de la unión y quedó libre                      | Cuenta nueva en la mesa                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `destination.account_id`                                     | A esa cuenta esté donde esté, o `account_closed` como hoy                                                                                                                                                                                                                                                                                                                                                                                           |

- **No hay tipos de cambio nuevos.** Una unión que solo está en pantalla **nunca entra en la cola** (M1-D13): sin
  conexión, Android conserva lo capturado y ofrece «Enviar solo a la {mesa de inicio}» o «Esperar».

### 8. Lectores que muestran el nombre de la unión (M1-D14)

Precuenta, ticket y comprobante («Mesa 5 + 6»), comanda («Salón · Mesa 5 + 6»), KDS (`title`), Preparación del
escritorio, avisos de platillos listos, «En curso» y cancelaciones de reportes, y la instantánea de la venta
(`sales_recognitions`, M1-D3). Todos con el mismo fragmento SQL de Go.

### 9. Licencia (`licensing/operations.go`)

Unir, agregar y quitar → la fila de «reorganizar lo capturado» (como `move` y `merge`); `preview` y `getTableUnion` →
lecturas; `table-groups` → administración (como `tables` y `layout`); `openAccount` sigue igual. Sin cambio de cupos.

### 10. Qué ve cada cliente (para B y C; Go decide, el cliente solo muestra)

- **Mapa**: contorno sólido del rectángulo que envuelve las mesas de la unión (misma fórmula de escala que las mesas),
  con `name` y `seats`; tocar cualquier mesa unida abre la unión. Grupo activo: marco tenue con su nombre y su atajo
  («Unir Mesa 3» o tocar el nombre), cada sección sigue tocable.
- **Lista**: una sola entrada por unión con su nombre; las mesas unidas no salen por separado.
- **«Unir con…»** (en el menú de la mesa del mapa y de la lista, y en el panel de una unión para «Agregar mesa» y
  «Quitar mesa»): elegir mesas del área; la vista previa de Go da nombre, capacidad y problemas; las mesas ocupadas,
  de otra área, unidas o fuera de servicio se ven deshabilitadas. Desde una mesa ocupada → `createTableUnion` al
  momento; desde una libre → unión en pantalla (diferida) hasta el primer producto; salir sin capturar no guarda nada.
- **Panel de la unión**: el nombre de la unión como título, sus cuentas como las de una mesa, «Agregar mesa» y «Quitar
  mesa».
- Los textos de error son los de Go, tal cual.

## Fase E · Pruebas frágiles (§5.8)

Rama `etapa-m1-e` desde `etapa-m1` (`72ecadd`). Solo cambian especificaciones y ayudas de `desktop/e2e/`, y el arnés
aislado (`$SP/iso.sh`, fuera del repositorio). Nada del producto. `scripts/browser-tests.py` y
`scripts/windows/dev.mjs` no necesitaron cambios (ver «La cascada»).

| Commit     | Tema                                                                                                     |
| ---------- | -------------------------------------------------------------------------------------------------------- |
| `2b54627`  | La terminal preparada de la aceptación sobrevive al reinicio del proceso de Playwright (la cascada).     |
| `92264cd`  | `workflow-stage9d-accounts` entra por la API antes que la página; el mismo patrón en otras dos; guardia. |
| `8bc4d437` | `workflow-stagel1-notices` no depende de la hora real (reloj del navegador de la prueba).                |

### 1. `workflow-stage9d-accounts`: la sesión reemplazada (K1-D39, K11-D24)

**Causa.** En el puente (`desktop/e2e/native-driver.ts`) la página y la prueba comparten **una sola** sesión de la
terminal (la variable `sessionToken` de `installDriver`): la página entra por la orden `api_request` y la prueba por
`driver.request`, y las dos usan y actualizan el mismo token, igual que la aplicación real comparte la de Rust. Go
tiene una sola sesión por terminal. La 9D hacía: entrar por la API (datos), salir, entrar por la página y, en cuanto se
veía la barra superior, **otra vez `auth/login` por la API** (`workflow-stage9d-accounts.spec.ts:332` en `72ecadd`).
Ese segundo inicio de sesión reemplaza la sesión de la página; una petición de la página que ya iba en camino con el
token anterior recibe `SESSION_REPLACED` («Tu sesión se cerró porque el usuario inició sesión de nuevo en esta
terminal.», `backend/internal/administration/service.go:571`), la página vuelve a «Inicia sesión» y la prueba falla.
Pasa más en WebKit, que es más lento al cargar la primera pantalla. No es de la aplicación: la aplicación no tiene ese
segundo inicio de sesión.

**Corrección** (`92264cd`). La prueba entra por la API **antes** que la página (paso 1, como ya lo hacía) y, después de
entrar por la página, usa la sesión de la página, que el puente ya comparte: se quitó el segundo `auth/login`
(`workflow-stage9d-accounts.spec.ts:332`, con su comentario).

**El mismo patrón en otras especificaciones** (se buscaron todos los `auth/login` de `desktop/e2e/` y se revisó si la
página tenía la sesión abierta con la misma terminal en ese momento):

| Especificación                                    | Qué hacía                                                                                | Qué se hizo                          |
| ------------------------------------------------- | ---------------------------------------------------------------------------------------- | ------------------------------------ |
| `workflow-stage9b-designer.spec.ts` (paso 9)      | Con la página dentro (Documentos), `auth/login` por la API justo después de un clic      | Se quitó: usa la sesión de la página |
| `workflow-stage9d-cancel-account.spec.ts` (final) | Con la página dentro (Funciones, recién guardado el umbral), `auth/login` por la API     | Se quitó: usa la sesión de la página |
| `workflow-stage9h-adjustments.spec.ts:452`        | Vuelve a entrar **a propósito** con la misma terminal: prueba que la sesión se reemplaza | Se deja; lo cubre la guardia (abajo) |

Las demás entran por la API antes que la página, o cuando la página está en «Inicia sesión», o con otra terminal (las
de la L1 cambian la licencia con `otherTerminal` y las de la K1 abren `terminalPage`, cada una con su propia
credencial): no tienen la carrera.

**Guardia en el puente** (para el caso a propósito de la 9H y para especificaciones futuras):
`native-driver.ts:663-703`. Un `auth/login` de la prueba (`driver.request`, ahora `testRequest`) espera a que terminen
las peticiones de la página en camino (como mucho 15 s) y detiene las nuevas (`pageRequest`) hasta tener la sesión
nueva; después la página sigue con la sesión nueva, como antes. Las demás peticiones de la prueba y todas las de la
página no cambian. El canal de eventos y la exportación no pasan por ahí (no hacía falta).

### 2. `workflow-stagel1-notices`: la medianoche

**Causa.** El cierre de un aviso diario («Entendido») se recuerda en la computadora por usuario y por día, con la fecha
**del navegador** en la zona horaria del restaurante (`desktop/src/pages/license/LicenseNotices.tsx:17-27`,
`localStorage` con la clave `adel-license-notice:<tipo>:<usuario>:<AAAA-MM-DD>`). Go no interviene en ese día. La prueba
cierra el aviso y comprueba que al volver a entrar **ese mismo día** no sale; si cerró a las 23:59:58 y volvió a entrar
a las 00:00:05, ya era otro día y el aviso salía otra vez, como debe (K1-D39).

**Corrección** (`8bc4d437`). El reloj del navegador de la prueba (`page.clock.install` de Playwright,
`workflow-stagel1-notices.spec.ts:102`) empieza a las **12:00 de hoy en la zona del restaurante** (`middayToday`,
`:38`, con la zona de `acceptanceTimezone`, `native-driver.ts:25`, la misma que el puente configura en cada base) y de
ahí corre normal. La prueba dura unos segundos, así que nunca cruza la medianoche, sea la hora real que sea. Go sigue con
su propia hora (los días que faltan para el vencimiento los calcula Go, con su licencia de prueba), la aplicación no
cambia y la hora de la Mac no se toca. No se usó el reloj simulado de Go (`ADEL_BROWSER_CLOCK_FILE`): no mueve la
fecha que usa el aviso, que es la del navegador.

**Demostración** (cambio temporal, nunca confirmado): con la corrección y un salto del reloj del navegador a las
00:00:05 del día siguiente justo después de «Entendido», la prueba falla exactamente como en K1-D39
(`workflow-stagel1-notices.spec.ts:127`, «Aviso de la licencia» visible al volver a entrar). Sin el salto aprueba.

**Otras especificaciones que dependen del día real** (no se corrigieron: no es igual de sencillo):

- `workflow-reports-9a`: crea ventas y las busca en «Hoy» (`period=today`) y en «Semana actual» (`current_week`);
  fallaría si el día comercial de Go (corte a las 05:00, `backend/internal/businessday/day.go`) o la semana cambia en
  medio de la prueba (unos segundos cerca de las 05:00, o del lunes a las 05:00).
- Los reportes en pantalla con su periodo por omisión, «Hoy», en `workflow-stage9d-cancel-account` («Cuentas
  canceladas», «Cancelaciones por motivo») y `workflow-stage9h-adjustments` («Modificaciones de cantidades», «Productos
  cancelados»): lo mismo, solo si se cruza las 05:00 entre la cancelación y el reporte.
- Corregirlas pediría mover el reloj simulado de Go del servidor compartido, y la protección de la hora de la L1 bloquea
  la captura si después la hora regresa (la última actividad quedaría «en el futuro»). Se quedan anotadas; en la
  regresión basta con no empezar la suite cerca de las 05:00 (y ya no hace falta evitar las 00:00).
- Sin riesgo: `tomorrowAtOne` de la 9D y de `workflow-stagez-k11-diagnostics` (mañana a la 1 p. m., siempre en el
  futuro), el reporte de cuentas descartadas de la 9D con `current_year` (solo comprueba que no aparecen) y el año del
  historial importado de la 9H (sale del paquete sintético).

### 3. La cascada «La aceptación debe comenzar con una base sin configurar»

**Causa.** No hay una base por especificación: la suite completa comparte **un** Go temporal. La primera
especificación que llama a `installDriver` (en orden alfabético, `inventory.spec.ts`) lo configura por la API (`POST
setup`, la licencia de prueba y el catálogo mínimo) y guardaba la terminal que creó en un `Map` **en memoria**
(`preparedTerminals`, `native-driver.ts:40` en `72ecadd`); las siguientes la reutilizaban. Playwright corre con un solo
proceso de trabajo y, **cuando una prueba falla, descarta ese proceso y abre otro**: el `Map` se perdía y la siguiente
especificación encontraba la base configurada sin conocer su terminal, así que lanzaba «La aceptación debe comenzar con
una base sin configurar» (`native-driver.ts:659` en `72ecadd`). Por eso, después de cualquier falla, todas las
siguientes caían en unos 0,2 s sin haber probado nada (33 en la K1.1, 25 en la A1, 5 en la K1).

**¿Qué especificaciones necesitan una base sin configurar?** Ninguna de las que usan la base compartida: necesitan la
base **configurada por la aceptación** y la credencial de su terminal. Las únicas que necesitan una base limpia ya la
preparan ellas mismas, con su propio Go temporal:

- `workflow-stage9h-migration` («instalación limpia»: el segundo Go de la 9H, `ADEL_BROWSER_CLEAN_SERVER_URL`);
- `workflow-stagel1-activation` (el Go sin activar de la L1, `ADEL_BROWSER_UNLICENSED_SERVER_URL`).

**Decisión propuesta: no dar una base limpia a cada especificación; conservar la terminal de la corrida.** Una base por
especificación pediría unos 43 Go temporales con su base y su licencia (o recrear el estado de cada una), alargaría la
suite y dejaría de probar lo que hoy prueba la corrida completa: que las especificaciones conviven en una misma base
(nombres únicos, lo que una deja y la siguiente ajusta, como el teclado virtual que deja `restaurant.spec.ts`). Lo
razonable es que el reinicio del proceso no pierda la terminal:

- `native-driver.ts:38-85` (`2b54627`): la terminal se guarda también en `terminal-aceptacion.json`, junto al token de
  aprovisionamiento del Go temporal (en la carpeta de la corrida, donde ya están `identity` y `provisioning-token`;
  `scripts/browser-tests.py`, `scripts/windows/dev.mjs` y el arnés la crean por corrida y los dos primeros la borran al
  terminar), con permiso 0600 y el PID del proceso principal de Playwright (`process.ppid` del proceso de trabajo).
- Solo se reutiliza en **la misma corrida** (el proceso de trabajo nuevo es hijo del mismo proceso principal): una base
  configurada por otra corrida se sigue rechazando, con el mensaje de siempre más la razón
  (`native-driver.ts:757-759`).
- `ready` marca si la primera especificación terminó de preparar la base. Si falló a la mitad (por ejemplo, la licencia
  de prueba), las siguientes dicen «La preparación de la base temporal (la licencia de prueba o el catálogo mínimo) falló
  en una especificación anterior de esta corrida: revisa la primera falla.» (`native-driver.ts:728-732`) en lugar de
  pedir una base sin configurar.

Con eso, después de una falla las siguientes corren de verdad; si alguna cae, es por su propia causa o porque necesitaba
algo que la especificación fallida no llegó a crear (ver la prueba de la cascada, abajo).

### 4. El arnés: el Go de base limpia de la 9H

`$SP/iso.sh` (fuera del repositorio, reemplazado de forma atómica para no afectar al otro agente) acepta
`ISO_CLEAN_PORT` en `browser`: arranca el Go temporal con base limpia de la 9H, como `CLEAN_PORT` de
`scripts/browser-tests.py`, y exporta `ADEL_BROWSER_CLEAN_SERVER_URL`/`_PROVISIONING`. Con `ISO_LICENSE_SIM=1`,
`ISO_UNLICENSED_PORT` e `ISO_CLEAN_PORT`, una corrida del arnés tiene los mismos cuatro servidores que la suite completa
(ya no se omiten las 4 pruebas de «instalación limpia»). Envoltorio de la fase: `$SP/m1/e-run.sh <etiqueta> <motor>
[filtros]` (Go 8211, limpio 8212, sin activar 8213, simulador 8491, vista previa 1431).

### 5. Pruebas realmente corridas

Todas con el arnés (`ISO_TAG=E`) y los cuatro servidores de la suite completa: Go temporal compartido 8211, Go de base
limpia 8212, Go sin activar 8213, simulador de licencias 8491 (`license-test`, confianza en
`contracts/licencias/v1/fixtures/vectors.json`) y vista previa 1431. Registros en `$SP/m1/e-logs/`. La pantalla estuvo
sin bloquear en todas (`CGSSessionScreenIsLocked` ausente), así que ningún resultado de WebKit se debe a ella.

| Prueba                                                                                    | Motor    | Resultado                                                                                                           | Tiempo (Playwright / total) |
| ----------------------------------------------------------------------------------------- | -------- | ------------------------------------------------------------------------------------------------------------------- | --------------------------- |
| `workflow-stage9d-accounts` sola                                                          | Chromium | 1 de 1                                                                                                              | 35,3 s / 52 s               |
| `workflow-stage9d-accounts` sola                                                          | WebKit   | 1 de 1                                                                                                              | 32,7 s / 42 s               |
| `workflow-stagel1-notices` sola                                                           | Chromium | 1 de 1                                                                                                              | 20,1 s / 29 s               |
| `workflow-stagel1-notices` sola                                                           | WebKit   | 1 de 1                                                                                                              | 19,6 s / 26 s               |
| `workflow-stage9b-designer` sola                                                          | Chromium | 1 de 1                                                                                                              | 41,3 s / 48 s               |
| `workflow-stage9b-designer` sola                                                          | WebKit   | 1 de 1                                                                                                              | 41,5 s / 48 s               |
| `workflow-stage9d-cancel-account` sola                                                    | Chromium | 1 de 1                                                                                                              | 47,3 s / 54 s               |
| `workflow-stage9d-cancel-account` sola                                                    | WebKit   | 1 de 1                                                                                                              | 47,5 s / 54 s               |
| `workflow-stage9h-adjustments` sola                                                       | Chromium | 3 de 3                                                                                                              | 43,0 s / 51 s               |
| `workflow-stage9h-adjustments` sola                                                       | WebKit   | 3 de 3                                                                                                              | 39,0 s / 46 s               |
| Medianoche (temporal: el reloj del navegador salta a las 00:00:05 después de «Entendido») | Chromium | Falla en `workflow-stagel1-notices.spec.ts:127`, igual que K1-D39 (esperado)                                        | 31 s                        |
| Cascada antes (temporal: puente de `72ecadd` y falla provocada en `inventory.spec.ts`)    | Chromium | 2 fallidas: la provocada y `workflow-reports-export-9a` con «…base sin configurar» (esperado)                       | 20 s                        |
| Cascada después (la misma falla provocada, puente corregido)                              | Chromium | 1 fallida (la provocada) y `workflow-reports-export-9a` aprobada                                                    | 22 s                        |
| Preparación fallida (temporal: el catálogo mínimo falla a propósito)                      | Chromium | 2 fallidas: la provocada y la siguiente con «La preparación de la base temporal … falló …» (esperado)               | 24 s                        |
| **Suite completa con la falla provocada** en `inventory.spec.ts` (la primera)             | Chromium | **54 aprobadas y 1 fallida (la provocada)**; ninguna «base sin configurar»; las 4 de «instalación limpia» corrieron | 26,7 min (12:13–12:40)      |
| **Suite completa**                                                                        | Chromium | **55 de 55** (43 especificaciones, con las 4 de «instalación limpia»)                                               | 27,3 min (12:40–13:08)      |
| **Suite completa**                                                                        | WebKit   | **55 de 55** (43 especificaciones, con las 4 de «instalación limpia»)                                               | 26,9 min (13:08–13:35)      |

**La cascada, en detalle.** Con la falla provocada en la primera especificación (la que prepara la base, después de
prepararla), antes caían las 54 siguientes; ahora **ninguna cae**: ninguna especificación depende de lo que
`inventory.spec.ts` crea después de entrar. Si la falla fuera en otra especificación, solo caería lo que dependa de sus
datos; en las corridas solas sobre una base nueva de las cinco corregidas no apareció ninguna dependencia así.

Los cambios temporales (la falla provocada, el puente de `72ecadd`, el salto a medianoche y la preparación fallida) se
quitaron al terminar cada prueba y nunca se confirmaron; el árbol queda sin cambios sin confirmar.

Además: `fmt-check` (Prettier) y `tsc --noEmit` del escritorio (incluye `e2e/`) aprobados antes de cada commit; md5 de
`docs/etapas/ETAPA_M1_MESAS.md` sin cambio (`4750ae46956590f208bf486477bb0bbf`). No se corrió Vitest, `cargo` ni Go
(no cambió nada de eso). No verificado: Windows (`dev.mjs` no cambió, y `process.ppid` funciona igual en Windows).

### 6. Para el coordinador

- **Decisiones propuestas** (para numerar `M1-Dn`):
  1. La cascada se corrige conservando la terminal de la corrida en un archivo (no con una base limpia por
     especificación), por lo de la §3.
  2. La 9D (y la 9B y la 9D de cancelar cuenta) usan la sesión de la página después de entrar por ella; el reingreso a
     propósito de la 9H queda protegido por la guardia del puente (`testRequest`).
  3. Los avisos de la L1 usan el reloj del navegador de la prueba (`page.clock`), no el reloj simulado de Go, porque el
     día del aviso lo decide el navegador.
  4. Las dependencias del día comercial (05:00) de los reportes quedan anotadas sin corregir (§2).
- **Aviso para las fases A y B (no es de esta fase):** §5.3 («quitar la hora lo envía») y §5.2 («sin etiqueta para lo
  que no necesita preparación») cambian lo que comprueban `workflow-stage9d-accounts` (quitarle la hora a un para
  llevar programado lo deja en `pending_release` y muestra «Enviado · sin confirmar en cocina» / «Sin confirmar en
  cocina» en un producto sin ruta, líneas 314–319, 392 y 414) y `workflow-stagez-k11-diagnostics` (`unschedule`, línea
  340, «lo deja esperando», K11-D9). Esas especificaciones tendrán que actualizarse con esos cambios.
- **Para la regresión de la Fase F**: el arnés ya corre la suite completa con los mismos servidores que
  `scripts/browser-tests.py` (`ISO_LICENSE_SIM=1`, `ISO_UNLICENSED_PORT`, `ISO_CLEAN_PORT`; ejemplo en
  `$SP/m1/e-run.sh`). Ya no hace falta evitar la medianoche; sí no empezar cerca de las 05:00 (§2). Si una
  especificación falla, las siguientes ya no caen en cascada: hay que revisar cada falla.
- Ninguna decisión de la especificación ni de negocio cambió. No se tocó la instalación real, la hora de la Mac ni
  `licensing.sicrat.com`; no se usaron emuladores.

### Decisiones del coordinador al cerrar la Fase E

Se fusiona `etapa-m1-e` en `etapa-m1`. Se aceptan las cuatro decisiones propuestas:

- **M1-D20. La cascada.** La terminal de la corrida se guarda en `terminal-aceptacion.json`, junto al token del Go
  temporal y con el PID del proceso principal de Playwright. No se da una base limpia a cada especificación: la suite
  sigue probando que las especificaciones conviven en una misma base.
- **M1-D21. La sesión compartida.** Después de entrar por la página, la prueba usa la sesión de la página. Un
  `auth/login` de la prueba espera a que terminen las peticiones de la página (`testRequest`), lo que cubre el reingreso
  a propósito de la 9H y las especificaciones nuevas.
- **M1-D22. Los avisos de la licencia.** La prueba usa el reloj del navegador (`page.clock`), porque el día del aviso lo
  decide el navegador. La hora de la Mac no se toca.
- **M1-D23. El día comercial.** Las dependencias del corte de las 05:00 quedan anotadas sin corregir. La regresión de la
  Fase F no empieza cerca de las 05:00; la medianoche ya no importa.
- **Las especificaciones que cambian por §5.2 y §5.3** las actualiza la **Fase B**, que es la que corre Playwright, en
  cuanto la Fase A esté en `etapa-m1`:
  - `workflow-stage9d-accounts`, líneas 314–319 (quitar la hora ahora envía a cocina) y 392 y 414 (un producto sin ruta
    ya no lleva etiqueta de cocina);
  - `workflow-stagez-k11-diagnostics`, línea 340 (`unschedule` ya no deja el pedido esperando, M1-D17).

## Fase A · Servidor: uniones de mesas, mesas divisibles y ajustes de servidor de la §5

Árbol `$SP/wt/m1-a`, rama `etapa-m1-a` (desde `etapa-m1` `3614024`), `ISO_TAG=A`. Sin Go temporal, sin emuladores ni
Playwright: todo se probó con las pruebas de Go contra el clúster temporal 55499 (bases `adel_pos_test_a_*`). La
especificación quedó intacta (md5 `4750ae46…` antes y después de cada formato). Envoltorios de la fase (fuera del
repositorio): `$SP/m1/a-go.sh` (go en `backend/` con el entorno del arnés), `$SP/m1/a-int.sh` (como `go-int`, con `-v` y
`A_COUNT` repeticiones) y `$SP/m1/a-suites.sh`; registros en `$SP/m1/a-logs/`.

### Commits (rama `etapa-m1-a`)

| Commit    | Tema                                                                                                                                        |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `6e85bd6` | Migración 0031 (sección «Fase A»), esquema 31 y el paquete `internal/tableunions`                                                           |
| `914eca8` | Servidor: rutas de uniones y grupos, creación diferida, ocupación, cola, `SettleEmpty`, lectores, permiso, capacidad, licencia, §5.2 y §5.3 |
| `7d11a43` | Pruebas `stagem1_*` de operations y la respuesta del `PATCH` de la cuenta leída después de cocina                                           |
| `ece3c2c` | Importador (M1-D5) con sus pruebas y el paquete sintético                                                                                   |
| `08293e1` | Contrato v1 (openapi, `docs/API_V1.md`, `API_CONTRACTS.md`, `TestContractV1`) y los conjuntos de fábrica                                    |
| `07d02de` | Actualización 30 → 31 probada y el arreglo de la 0031 (llave diferida, sin llave al historial)                                              |
| `4ed60c7` | Prueba de reportes y la especificación de la K1.1 ajustada a M1-D17                                                                         |
| `a188629` | La prueba de reportes en `reports_test` y con una mesa del mismo área                                                                       |

### Qué se hizo

1. **Migración `0031_uniones_de_mesas.sql`**, sección «Fase A» (solo agrega): `app.table_groups` y
   `app.table_group_sections` (mesas divisibles), `app.table_unions`, `app.table_union_members` (con el índice único
   parcial `table_union_members_open_table_idx`: una mesa en una sola unión abierta) y `app.table_union_events`
   (historial con el nombre de la unión después de cada cambio), las tres columnas de la unión en
   `app.sales_recognitions` (NULL en lo que ya existía) y el permiso `restaurant.tables.join` con la concesión de
   M1-D8. `config/product.json` pasa a `schema_version: 31` (con la sincronía; la versión 0.18.0 queda para la Fase F).
2. **Paquete neutral `internal/tableunions`** (depende solo de pgx, `events` y `documents`; lo usan operations, payments,
   kitchen, reports y administration sin ciclos): el nombre (`UnionName`, orden natural `NaturalCompare`), los
   fragmentos SQL del lugar de una cuenta (`UnionNameSQL`, `UnionNameAtSQL`, `UnionIDSQL`, `GroupIDSQL`,
   `PlaceTablesSQL`), `ReadTables`/`Describe` (nombre, capacidad y grupo de un conjunto de mesas), la instantánea de la
   venta (`OpenSnapshot`) y **`SettleEmpty`**, el único lugar donde una unión se deshace sola.
3. **Rutas del contrato §3** en `operations.Handle` (`operations/table_unions.go` y `operations/table_groups.go`):
   `previewTableUnion`, `createTableUnion`, `addTableToUnion`, `removeTableFromUnion`, `getTableUnion`,
   `createTableGroup` y `updateTableGroup`, con los códigos y textos exactos del contrato §5, su idempotencia y sus
   eventos (§6).
4. **Creación diferida** en `openAccount` (`union_table_ids`, `union_group_id`) y **`seen_union_id`** (M1-D9);
   `listAccounts?table_id=` de una mesa unida devuelve las cuentas de toda la unión; `overview` y `restaurant/state`
   con `unions` y `groups`; `Table.union_id`/`group_id`; `Account.table_union`; la ocupación de la unión en cada mesa
   (M1-D10, `tableQuery`); el `PATCH` de una mesa unida o agrupada ya no cambia de área.
5. **El nombre de la unión en todos los lectores** (contrato §8): precuenta, ticket y comprobante (`payments/finance.go`,
   también de una cuenta ya cobrada), comanda (`kitchen/releases.go`, «Salón · Mesa 5 + 6»), título del KDS
   (`kitchen/display.go`), Preparación (`kitchen/reads.go`), avisos de listos (`administration/ready_alerts.go`),
   «En curso» y cancelaciones (`reports/`), «Ventas de salón» por unión y el filtro de mesa con las uniones en que
   estuvo (`reports/sales.go`, `reports/legacy.go`), y la instantánea de la venta (`payments/recognition.go`).
   `restaurant.account.table_union` entra en `displayAccountKinds`.
6. **Cola sin conexión** (contrato §7): `destination.seen_union_id` y la tabla de casos en `diagnoseOfflineTable`.
7. **Permiso «Unir mesas»** (M1-D8): catálogo («Mesas y cuentas», `requires: accounts.open, restaurant.tables.read`),
   conjuntos de fábrica de Mesero, Cajero y Supervisor (y por ellos el asistente y «Restablecer»), la vista sencilla
   «Atender mesas», `WaiterPermissions` y sus tres copias (openapi `pinLogin`, `docs/API_V1.md`, `API_CONTRACTS.md`), la
   concesión de la 0031 y el comentario de `permission_catalog.go` (factoryRolePermissions).
8. **Licencia** (`licensing/operations.go`, `classifyTableUnion`) y capacidad **`table-unions-v1`** (`httpapi/router.go`).
9. **Importador** (M1-D5, `legacyimport/table_groups.go`): la regla con y sin «Mesa N», las secciones juntas en la
   cuadrícula, lo dudoso al resumen previo y al informe en lenguaje sencillo, la segunda importación; `legacysynth` con
   «Mesa 3» + «3a, 3b, 3c», «Mesa 9a» a «Mesa 9d» y el dudoso «Mesa 12a, 12c».
10. **§5.2** `preparation_required` por renglón y por cuenta (`kitchen.VersionPreparationRequired` y
    `kitchen.AccountPreparationRequired`) y **§5.3** quitar la hora envía (`kitchen/releases.go`, `Integrate` de
    `account.updated`).
11. **Contrato**: `openapi.json` (7 operaciones y 5 esquemas nuevos, campos nuevos), `docs/API_V1.md` (tabla y sección
    «Uniones de mesas y mesas divisibles · etapa M1»), `API_CONTRACTS.md` (sección «Etapa M1») y su paso en
    `TestContractV1`.

### Decisiones propuestas

1. **Dónde vive el código.** Las rutas en `operations` (necesitan `handler.event`, `readAccount` y la idempotencia de
   `Handle`); lo común en `internal/tableunions`, neutral, para que payments, kitchen, reports y administration lean el
   nombre de la unión y payments deshaga uniones sin importar operations. _Porqué_: operations ya importa payments y
   kitchen; poner `SettleEmpty` en operations crearía un ciclo con payments.
2. **La ocupación de la unión se calcula una vez, en `tableQuery`** (`operations/tables.go:35`): las cuentas de una mesa
   son las de `PlaceTablesSQL` (todas las mesas de su unión abierta o solo ella). `TableUnion` toma su estado, cuentas,
   saldo y tiempo de cualquiera de sus mesas ya leídas (`unionFrom`). _Porqué_: un solo cálculo; las reglas de no
   desactivar ni poner fuera de servicio una mesa con cuentas se aplican solas a toda la unión.
3. **Un cambio de la unión no sube la revisión de la cuenta** (renombrar al unir, agregar, quitar o deshacer; mover las
   cuentas de la mesa que se quita a la primera que queda). _Porqué_: no es una edición de la cuenta; si subiera, una
   terminal que captura en esa cuenta recibiría `ACCOUNT_CONFLICT` por algo que hizo otra con las mesas. La unión sí
   tiene su revisión (`TABLE_UNION_CONFLICT`).
4. **El historial de la cuenta** registra cada cambio de su unión con `restaurant_events.kind = account.table_union`
   (`after.change`: `joined`, `table_added`, `table_removed`, `dissolved` o `renamed`), una fila por cuenta abierta y
   operación, con quién y la terminal. La unión que nace con el primer producto también lo registra en la cuenta nueva.
5. **Quitar la mesa que tiene las cuentas** las pasa a la primera mesa que queda en el orden del nombre (M1-D1); si queda
   una, la unión se deshace con `dissolve_reason = single_table` y las cuentas que se llamaban como la unión vuelven al
   nombre de esa mesa. `SettleEmpty` deshace con `no_accounts` (y, por defensa, una unión abierta con menos de dos mesas).
6. **El nombre de la unión en una cuenta cerrada o cancelada se lee en el instante** (`UnionNameAtSQL`: la unión de la
   que la mesa era miembro en ese momento, con el nombre de su último cambio hasta entonces). La precuenta o el ticket
   de una cuenta cobrada se reimprimen con «Mesa 5 + 6» aunque la unión se haya deshecho, y «Cuentas canceladas» dice la
   unión que tenía la cuenta al cancelarla. Lo vivo (cuentas abiertas, KDS, Preparación, avisos, «En curso») lee la unión
   abierta (`UnionNameSQL`).
7. **Comanda y KDS con `documents.TableContext` solo en uniones**: «Salón · Mesa 5 + 6» y `title` «Mesa 5 + 6» también
   con mesas «5» y «6»; una mesa sola sigue como antes. El subtítulo del KDS no repite una cuenta que se llama como la
   unión (con o sin «Mesa»).
8. **Renombrar una mesa unida** vuelve a nombrar su unión (y sus cuentas sin nombre propio), con el evento de cuenta
   `renamed`; el historial de la unión no tiene ese tipo (la lista del contrato es cerrada).
9. **Mesas divisibles**: `app.table_group_sections.table_id` es único (una mesa, un grupo); quitar un grupo borra sus
   secciones y deja la fila del grupo inactiva (la recuerdan las uniones). Un grupo quitado no se reactiva (400). Crear
   un grupo con una mesa unida responde `TABLE_GROUP_IN_USE`. El nombre se compara sin distinguir mayúsculas con las
   mesas del área y los grupos activos.
10. **`preparation_required`** (contrato §4): un renglón es `false` si todas sus unidades (de esa versión y ese estado en
    la cuenta) ya se liberaron y ninguna tiene tarea en `kitchen_tasks`; la regla es la misma para un renglón cancelado
    (no se le pone etiqueta de cocina si nunca la necesitó). La cuenta es `true` si alguna unidad activa la necesita o
    si no tiene unidades activas.
11. **§5.3** (M1-D17): al integrar `account.updated` sin hora, cocina libera en la misma transacción las liberaciones
    que acaban de quedar sin hora (`prepareRelease`, como un pedido sin hora). La respuesta del `PATCH` de la cuenta se
    lee después de integrar cocina (antes se leía antes), así que ya trae `released`.
12. **Importador** (M1-D5): la base «Mesa N» que se vuelve grupo no se crea ni se cuenta como mesa viva (la validación
    resta esas mesas, `expected.go`), sus ventas conservan su nombre y su área en `legacy_sales` (con `table_id` nulo),
    y el resumen y el informe lo dicen (`table_group_base`). Dudosos (`table_group_doubtful`): letras repetidas o no
    seguidas, una sola sección **y también más de una mesa con el mismo número** («Mesa 20» y «20»). Segunda
    importación: los grupos `source legacy` anteriores se quitan (inactivos, sin secciones) y se crean otra vez; un
    grupo del propietario que ya usa el nombre o una sección hace que ese grupo del anterior no se cree; un grupo del
    propietario que se queda con menos de dos secciones (o con secciones de otra área) se quita. `legacy_batch_id` va
    sin llave foránea (la 0024 prohíbe llaves de tablas operativas hacia el historial). El paquete de `legacysynth` usa
    «Mesa 9a» a «Mesa 9d» para el caso sin base porque ya trae una «Mesa 7» (con «Mesa 7a» a «7d» sería un grupo con
    base); el paquete en memoria de las pruebas (`MemorySource`) sí usa «Mesa 7a» a «Mesa 7d», sin «Mesa 7». En
    `legacysynth`, la «Mesa 3» de siempre (con ventas) se vuelve el grupo de «3a», «3b» y «3c».
13. **La 0031 hace inmediata la llave diferida de `sales_recognitions` antes de alterar la tabla** y la vuelve a diferir
    (`SET CONSTRAINTS … IMMEDIATE/DEFERRED`). _Porqué_: una actualización desde antes de la 0017 aplica el relleno de la
    0017 en la misma transacción y PostgreSQL no altera una tabla con comprobaciones pendientes (lo encontró
    `TestStage9AReportsUpgradeBackfillsClosedAccounts`).
14. **Las dudas del contrato** (`$SP/m1/contrato-dudas.md`): `details.accounts` como identificadores, 403
    `PERMISSION_DENIED`, la primera de `table_ids` es la mesa de inicio en la vista previa, `createTableUnion` con
    `group_id` desde una sección ocupada une todo el grupo, `seen_union_id` de una unión que ya no existe no rechaza,
    respuestas `{union}`/`{group}`, `ACCOUNT_CONFLICT` para la revisión de un grupo, límites de 40 mesas.

### Para el coordinador (decide)

- **«Quitar datos de prueba» (M1-D7, último punto) no aplica tal como está escrito**: `internal/maintenance` solo
  reconoce por firma exacta impresoras, estaciones, productos y categorías de las pruebas, y **nunca toca cuentas ni
  mesas** (no existen «cuentas de prueba» ni «mesas de prueba»). Las uniones y los grupos no apuntan a nada de lo que
  quita, así que «Quitar datos de prueba» sigue funcionando con ellos (sin cambio de código). Si se quiere que quite
  uniones o grupos de pruebas, habría que definir sus firmas (por ejemplo, los nombres de mesas de las especificaciones
  `workflow-stagez-m1-*` de la Fase B), y eso sería quitar mesas y cuentas, algo que hoy no hace.
- **La prueba que afirmaba K11-D9 existe y es de Playwright**: `desktop/e2e/workflow-stagez-k11-diagnostics.spec.ts`,
  paso 4 («Por enviar a cocina» con un pedido al que se le quitó la hora y «Enviar a cocina ahora» sobre él). La ajusté
  (el pedido sin hora ya no está en el panel y su preparación no es `pending_release` ni `scheduled`; «Enviar a cocina
  ahora» se prueba con el pedido programado), pero **no la corrí** (esta fase no usa Playwright): que la corra la Fase B
  o la F. En Go no había ninguna. El texto del escritorio «Espera a que lo envíes a cocina» del caso sin hora
  (`PendingKitchen.tsx:50-65`) queda sin uso para pedidos nuevos (Fase B), y `docs/KITCHEN_AND_PRINTING.md` (líneas
  342, 722-731 y 778) describe K11-D9 (Fase F).
- **Una cuenta reabierta por una devolución** conserva el nombre que tenía al cerrarse (por ejemplo «Mesa 5 + 6») aunque
  su unión ya se haya deshecho: lo cerrado no se reescribe. Al volver a una mesa unida queda en esa unión (M1-D7).

### Para el revisor de la concurrencia (§2.3, M1-D11)

- **Dónde se decide la ocupación**: `tableQuery` (`operations/tables.go:35`), con `tableunions.PlaceTablesSQL`
  (`tableunions/sql.go:40`). Las reglas de unir (una mesa ocupada = con cuentas propias abiertas; unida = miembro sin
  `left_at`) están en `tableunions.ReadTables` (`describe.go`) y `joinProblems` (`operations/table_unions.go:307`).
- **Orden de los locks en cada ruta nueva**: ninguno nuevo. Todas las rutas `restaurant/table-unions*` y
  `restaurant/table-groups*`, y `openAccount` con la unión diferida, van por `operations.Handle`, que toma
  `pg_advisory_xact_lock(7820148305)` **exclusivo antes de leer nada** (la vista previa y `getTableUnion`, compartido);
  luego valida, escribe, llama a `SettleEmpty` con el mismo lock y guarda la operación. `payments.Handle` toma el mismo
  lock exclusivo antes de leer y llama a `SettleEmpty` al final. Ninguna ruta nueva toma 7820148304, 7820148308 ni
  7820148311 por sí misma (los cupos de la licencia siguen en `dispatch`, después del manejador, como siempre). La última
  defensa es el índice único parcial `table_union_members_open_table_idx` (23505).
- **Quién llama a `SettleEmpty`** (`tableunions/settle.go:42`): `operations.Handle` después de `dispatch` en toda
  escritura (`operations/handler.go:94`; cubre cancelar, descartar, liberar, combinar, mover, dividir, transferir, la
  cola sin conexión y las rutas de uniones) y `payments.Handle` después de la acción (`payments/handler.go:140`; cubre
  cobrar, entregar, devolver y lo de caja). Deshacer con cuentas (quedó una mesa) lo hace `removeTableFromUnion` con
  `dissolveWithAccounts` (`operations/table_unions.go:869`); `SettleEmpty` lo repite por defensa.
- **Cómo correr la prueba de concurrencia** (`operations/stagem1_concurrency_test.go`, `TestStageM1Concurrency`): diez
  peticiones a la vez por ronda (abrir solas B, A y D; unir A+B, C+B y D+C con el primer producto; agregar G y B a una
  unión E+F, quitarle F y cobrarla, con la misma revisión), 25 rondas (`ADEL_M1_CONCURRENCY_ROUNDS`). Comprueba las
  invariantes (`expectStageM1Invariants`), que las cuentas abiertas sean las que respondieron 200 y que cada perdedora
  reciba un 409 del contrato; el registro trae la cuenta de resultados. Con el arnés:
  `ISO_TAG=A ISO_RUN=TestStageM1Concurrency $SP/iso.sh <árbol> go-int ./internal/operations` (una vez) o
  `A_RUN=TestStageM1Concurrency A_COUNT=3 $SP/m1/a-int.sh ./internal/operations` (con `-count=3` y `-v`).

### Archivos

- Migración: `backend/migrations/0031_uniones_de_mesas.sql`; `config/product.json` (`schema_version` 31) y la
  sincronía (`backend/internal/product/metadata.json`, `desktop/src/generated/product.json`).
- `backend/internal/tableunions/`: `doc.go`, `names.go`, `sql.go`, `describe.go`, `settle.go`, `snapshot.go`,
  `names_test.go`.
- `backend/internal/operations/`: `table_unions.go`, `table_groups.go` (nuevos); `handler.go` (rutas, permisos, vista
  previa de solo lectura, `SettleEmpty`), `tables.go` (ocupación, área, nombre), `models.go`, `accounts_read.go`
  (`table_union`, `preparation_required`, `listAccounts`), `accounts_write.go` (diferida, `seen_union_id`, respuesta del
  `PATCH`), `account_names.go`, `areas.go` (`overview`), `recovery.go`, `offline_orders.go`, `context.go` (permiso).
- `backend/internal/payments/`: `handler.go` (`SettleEmpty`), `recognition.go` (instantánea), `finance.go` (lugar).
- `backend/internal/kitchen/`: `releases.go` (comanda y §5.3), `display.go` (título, `displayAccountKinds`), `reads.go`
  (Preparación), `states.go` (§5.2).
- `backend/internal/reports/`: `sales.go`, `legacy.go`, `sales_progress.go`, `cancellations.go`.
- `backend/internal/administration/`: `permission_catalog_entries.go`, `permission_catalog.go`, `waiter_scope.go`,
  `setup.go` (comentario), `ready_alerts.go`, `recovery.go`.
- `backend/internal/licensing/operations.go`, `backend/internal/httpapi/router.go`.
- `backend/internal/legacyimport/`: `table_groups.go` (nuevo), `catalog_plan.go`, `expected.go`, `summary.go`,
  `write_live.go`, `write_history.go`; `backend/internal/legacysynth/generate.go`.
- Contrato: `backend/internal/apicontract/openapi.json`, `docs/API_V1.md`, `API_CONTRACTS.md`.
- Pruebas nuevas: `operations/stagem1_fixture_test.go`, `stagem1_unions_test.go`, `stagem1_groups_test.go`,
  `stagem1_offline_test.go`, `stagem1_readers_test.go`, `stagem1_schedule_test.go`, `stagem1_concurrency_test.go`,
  `stagem1_no_preparation_test.go` (el parche de la Fase 0); `reports/stagem1_unions_integration_test.go`;
  `legacyimport/stagem1_table_groups_test.go` y `stagem1_import_integration_test.go`;
  `database/stagem1_upgrade_test.go`; el paso `contractTableUnions` en `apicontract/contract_integration_test.go`.

### Pruebas que cambiaron y por qué

| Prueba                                                                                   | Cambio                                                                     | Porqué                                                                                                                 |
| ---------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `administration/permission_catalog_test.go` `TestFactoryRolesAreClosedUnderDependencies` | Tamaños 15/37/89                                                           | «Unir mesas» en Mesero, Cajero y Supervisor (M1-D8)                                                                    |
| `database/stagea1_upgrade_test.go` `stageA1Snapshot`                                     | Excluye `restaurant.tables.join` de `permissions` y `role_permissions`     | Las pruebas de actualización migran hasta la última; la concesión de la 0031 se comprueba en `stagem1_upgrade_test.go` |
| `database/stage9h_upgrade_test.go` `stage9HRowCounts`                                    | Excluye las tablas de uniones y grupos y el permiso nuevo                  | Igual: lo de la 0031 se comprueba aparte                                                                               |
| `database/existing_install_test.go` (etapa 8)                                            | Excluye `restaurant.tables.join` del conteo de permisos                    | Igual                                                                                                                  |
| `desktop/e2e/workflow-stagez-k11-diagnostics.spec.ts` paso 4                             | El pedido sin hora ya no espera; «Enviar a cocina ahora» con el programado | §5.3 (M1-D17) reemplaza K11-D9. **No corrida**                                                                         |

### Pruebas realmente ejecutadas

Todas con el arnés (`iso.sh … go-int`, una base `adel_pos_test_a_*` por paquete, borrada al final) o con `a-int.sh`
(lo mismo con `-v` y `-count`). Registros en `$SP/m1/a-logs/`.

| Qué                                                                                                                                                                                                      | Resultado                                                                                                                       |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `TestStageM1NoPreparationLines` (el parche de la Fase 0, §5.2)                                                                                                                                           | Pasa (antes fallaba en sus 2 subpruebas)                                                                                        |
| `TestStageM1TableUnions` (10 casos: diferida, crecer, rechazos, agregar y quitar, deshacerse en 8 caminos, reabierta, carrera de la diferida, permiso con y sin, con PIN, idempotencia, `seen_union_id`) | Pasa                                                                                                                            |
| `TestStageM1TableGroups` (atajo, parcial, sección ocupada, crecer desde ella, administración y rechazos)                                                                                                 | Pasa                                                                                                                            |
| `TestStageM1OfflineQueue` (las 6 filas del contrato §7)                                                                                                                                                  | Pasa                                                                                                                            |
| `TestStageM1UnionReaders` (precuenta, finanzas, comanda, KDS, Preparación, encabezado del KDS al cambiar la unión, aviso de listos, instantánea de la venta, cuenta cobrada)                             | Pasa                                                                                                                            |
| `TestStageM1RemovingScheduleSends` (§5.3)                                                                                                                                                                | Pasa                                                                                                                            |
| `TestStageM1Concurrency` (25 rondas de 10 peticiones a la vez)                                                                                                                                           | Pasa; con `-count=3`, pasa las 3 (cada corrida con victorias y rechazos en todos los intentos salvo el cobro, que gana siempre) |
| `TestStageM1UnionReports` (reports: En curso, Ventas de salón, filtro de mesa, Cuentas canceladas)                                                                                                       | Pasa                                                                                                                            |
| `TestStageM1LegacyTableGroups`, `TestStageM1LegacyGroupsWrapRow` (unitarias) y `TestStageM1LegacyImportTableGroups` (integración, dos importaciones)                                                     | Pasan                                                                                                                           |
| `TestStageM1Upgrade` (30 → 31)                                                                                                                                                                           | Pasa                                                                                                                            |
| `TestContractV1` (63 operaciones, con `contractTableUnions`)                                                                                                                                             | Pasa                                                                                                                            |
| `go-int ./internal/operations` completo                                                                                                                                                                  | Pasa dos veces (898 s en `7d11a43` y 898 s en `a188629`, el estado final)                                                       |
| `go-int ./internal/database` completo                                                                                                                                                                    | Pasa (37 s) después de arreglar la 0031 (ver decisión 13) y las tres pruebas de actualización anteriores                        |
| `go-int` legacyimport, apicontract, licensing, httpapi, reports/operational, payments, kitchen, administration, maintenance                                                                              | Pasan (21, 70, 12, 2, 50, 5, 7, 453 y 4 s)                                                                                      |
| `go-int ./internal/reports` completo (con `TestStageM1UnionReports`)                                                                                                                                     | Pasa (150 s)                                                                                                                    |
| `go-int ./cmd/ad-el-pos-server` (con el candado `$SP/m1/candado-cmd`, ya liberado)                                                                                                                       | Pasa (67 s)                                                                                                                     |
| `go-unit` (vet normal e `integration`, `go test -race ./...`, build y vet de Windows)                                                                                                                    | Aprobado                                                                                                                        |
| `format`, `fmt-check`, `sync-check`; md5 de la especificación                                                                                                                                            | Sin cambios pendientes; «Metadatos coherentes»; `4750ae46…`                                                                     |
| `npx tsc --noEmit` del escritorio (incluye `e2e/`)                                                                                                                                                       | Sin errores                                                                                                                     |

**No verificado**: la especificación de Playwright `workflow-stagez-k11-diagnostics.spec.ts` ajustada (no se corrió:
esta fase no usa Playwright); el comportamiento de la 0.4.1 de Android frente a una unión (Fase C); la regla del
importador con el respaldo real (nunca se tocó; con el paquete sintético y `MemorySource`).

### Lecturas del contrato de la Fase A

Ninguna bloquea: la Fase A implementó la lectura que se indica en cada una, sin cambiar el contrato. Las fases B y C
pueden confiar en lo que dice aquí (lo prueba `stagem1_*_test.go` y `TestContractV1`).

1. **`TABLE_IN_UNION`, `details.accounts`**: el contrato dice «`accounts` (ids)». Va como arreglo de identificadores
   (`["uuid", …]`, las cuentas abiertas de la unión en orden de apertura), no como `TableAccountRef` de
   `TABLE_HAS_ACCOUNT`. `details.union` es el `TableUnion` completo y `details.table_id`/`details.union_id` también van.
2. **403 sin «Unir mesas»**: el contrato dice «403 `FORBIDDEN` · el de siempre». El de siempre de `restaurant/*` es
   **`PERMISSION_DENIED`**; eso responde.
3. **`previewTableUnion` con `{table_ids}`**: el contrato no dice cuál es la mesa de inicio. **La primera de `table_ids` es
   la de inicio**: si está ocupada y otra también, `TABLE_UNION_BOTH_OCCUPIED`; si está libre y otra está ocupada,
   `TABLE_UNION_OCCUPIED` en la ocupada (M1-D6). La respuesta trae `table_ids` en el orden del nombre.
4. **`createTableUnion` con `group_id`**: con la mesa de inicio (una sección) ocupada y las demás libres, **une todo el
   grupo** (crecer desde la sección ocupada; se llama como el grupo). Con la de inicio libre y otra sección ocupada,
   `TABLE_GROUP_SECTION_OCCUPIED`; con dos ocupadas, `TABLE_UNION_BOTH_OCCUPIED`; con todas libres,
   `TABLE_UNION_DEFERRED`. La vista previa `{group_id}` (sin mesa de inicio) marca cada sección ocupada con
   `TABLE_GROUP_SECTION_OCCUPIED`, igual que `openAccount` con `union_group_id`.
5. **`seen_union_id` en una mesa que ya no está unida**: no se rechaza (la unión que vio la terminal se deshizo); se
   abre como siempre, con `seen_account_ids`. Solo responde `TABLE_IN_UNION` si la mesa **está** en una unión distinta
   de la vista (o `null`). Es la misma regla que la cola sin conexión.
6. **Forma de las respuestas**: `{union}` y `{group}` se leyeron como objetos con esa llave (`{"union": TableUnion}`,
   `{"group": TableGroup}`; `removeTableFromUnion`: `{"union", "dissolved"}`; `getTableUnion`: `{"union":
TableUnionDetail}`).
7. **Revisión distinta en `updateTableGroup`**: no hay código propio en el contrato; responde el genérico
   `ACCOUNT_CONFLICT` («La cuenta o distribución cambió en otra terminal…»), como las mesas y las áreas.
8. **Crear un grupo con una mesa que está en una unión abierta**: se rechaza con `TABLE_GROUP_IN_USE` (como cambiarlo);
   una unión toma su nombre de los grupos.
9. **Un grupo quitado (`active: false`)** no se vuelve a activar: `PATCH` responde 400 «Este grupo ya se quitó. Crea uno
   nuevo en Áreas y distribución.». Su respuesta al quitarlo es el grupo con sus secciones de antes y la revisión nueva.
10. **`TableUnion.state`**: `occupied | awaiting_payment | partial_payment | dissolved` (nunca `free`: una unión abierta
    siempre tiene cuentas).
11. **Mesas con y sin la palabra «Mesa» en una unión**: con «5» y «Mesa 6» el nombre es «5 + Mesa 6» (M1-D2 literal; en
    los documentos, «Mesa 5 + Mesa 6»). Con las mesas del propietario no pasa.
12. **Límites**: una unión y un grupo admiten hasta 40 mesas; un grupo, de 2 a 40; el nombre de un grupo, hasta 60
    caracteres.

### Decisiones del coordinador al cerrar la Fase A

Se fusiona `etapa-m1-a` en `etapa-m1`. Después de la fusión aprobaron `fmt-check`, `sync-check` y `tsc` del
escritorio, y el servidor quedó idéntico al de la rama de la Fase A.

- **M1-D24.** Se aceptan las decisiones propuestas 1 a 13 de la Fase A.
- **M1-D25. Lecturas del contrato.** Las 12 lecturas de arriba pasan a ser parte del contrato. Las fases B y C las
  usan tal cual:
  - `details.accounts` son identificadores;
  - el 403 es `PERMISSION_DENIED`;
  - en la vista previa, la primera de `table_ids` es la mesa de inicio;
  - las respuestas vienen como `{union}` y `{group}`;
  - un grupo quitado no se reactiva.
- **M1-D26. «Quitar datos de prueba».** Se retira el último punto de M1-D7. La especificación no lo pide, y esa función
  nunca toca mesas ni cuentas, así que no hay uniones ni grupos «de prueba» que quitar. Sigue funcionando con uniones y
  grupos (la suite de `maintenance` pasa) y no cambia.
- **M1-D27. Cuenta reabierta por una devolución.** Conserva el nombre que tenía al cerrarse, por ejemplo «Mesa 5 + 6»,
  aunque su unión ya se haya deshecho, porque lo cerrado no se reescribe. Si vuelve a una mesa unida, queda en esa
  unión. Es una limitación conocida y se dice en la entrega.
- **M1-D28. Lo que queda para otras fases.**
  - **Fase B**:
    - corre `workflow-stagez-k11-diagnostics`, que la A ajustó a M1-D17 sin correrla;
    - revisa el texto «Espera a que lo envíes a cocina» de `PendingKitchen.tsx`, que ya no se usa para pedidos nuevos.
  - **Fase F**: actualiza en `docs/KITCHEN_AND_PRINTING.md` lo que todavía describe K11-D9.

## Fase R · Revisor independiente de la concurrencia de las uniones (§2.3, M1-D11)

Rama `etapa-m1-r` (desde `etapa-m1` `531773d2`, con la Fase A fusionada), `ISO_TAG=R`. Sin Go temporal, sin emuladores
y sin Playwright. No se tocó código del producto: solo pruebas nuevas. Se leyó el código antes que la sección «Para el
revisor» de la Fase A y se comprobó cada afirmación de esa sección.

### Commits (rama `etapa-m1-r`)

| Commit     | Tema                                                                                                                        |
| ---------- | --------------------------------------------------------------------------------------------------------------------------- |
| `062b38b7` | Prueba propia de concurrencia `TestStageM1ReviewConcurrency` (`operations/stagem1_review_concurrency_test.go`)              |
| `baf5cf00` | `TestStageM1ReviewDeferredLeavesNothing` (`operations/stagem1_review_deferred_test.go`): la diferida rechazada no deja nada |

Las pruebas que muestran defectos **no** están en la rama: van como parche en `$SP/m1/revisor/` con su salida.

### Resumen

- **Las invariantes de la §2.3 se cumplen.** Ninguna corrida dejó una mesa en dos uniones abiertas, una unión abierta
  sin cuentas o con menos de dos mesas, una cuenta perdida, una respuesta 5xx o un rechazo fuera del contrato: la prueba
  de la Fase A pasó 20 de 20 con `-race` y la propia, con una mezcla más amplia (22 escrituras de tres terminales a la
  vez, en orden al azar), pasó 20 de 20 (200 rondas) con `-race`. El lock 7820148305 exclusivo se toma antes de leer en
  todos los caminos que escriben uniones, mesas o cuentas, y `SettleEmpty` cubre todos los que cambian el estado o la mesa de una
  cuenta.
- **Dos defectos que deben corregirse** (ninguno bloquea ni rompe la §2.3; los dos con prueba que falla):
  1. el nombre de una unión de más de 100 caracteres hace fallar con 400 unir, agregar, abrir una cuenta sin nombre,
     mover una cuenta y la cola sin conexión;
  2. las horas que se guardan son las del **inicio** de cada transacción y no las del orden del lock, así que el ticket
     de una cuenta cobrada en una unión puede decir la mesa sola u **otra unión**, y el historial de una unión puede ir
     hacia atrás. Pasa sin forzarlo: de 3 a 5 de cada ~46 cobros en uniones en la mezcla de 40 rondas.
- **Un punto menor** (defensa del índice único) y **dos puntos para decidir** (una lectura del contrato y una regla de
  nombres). Ver abajo.

### Hallazgos

#### R1. Una unión con nombre de más de 100 caracteres no se puede usar — debe corregirse

- **Dónde**: el nombre de la unión no tiene límite (`tableunions/names.go:39`, columna de hasta 4000 en la 0031), pero
  una cuenta sin nombre propio se llama como la unión (M1-D14) y `app.restaurant_accounts.name` admite de 1 a 100
  (`migrations/0004_restaurant_operations.sql:43`; `operations/accounts_write.go:16` también rechaza más de 100):
  - `operations/table_unions.go:558` (`renameUnionAccounts`, desde `createTableUnion` `:631`, `addTableToUnion` `:770` y
    renombrar una mesa unida, `tables.go:175` → `:221`; este último, por lectura del código, sin prueba): `UPDATE … SET
name` viola la comprobación (23514) y
    `administration/http.go:163` lo traduce al genérico 400 `VALIDATION_ERROR` «Los datos o sus relaciones no son
    válidos.»;
  - `operations/account_names.go:35` (`neutralAccountName`, que usan `fillAccountName` y `movedAccountName`): abrir sin
    nombre (también la diferida), mover a la unión (`accounts_write.go:375` → `:385`, 23514) y el pedido de la cola sin
    conexión (`offline_orders.go:931`) responden 400 `OPERATION_VALIDATION` «Revisa nombre/referencia (100), cliente
    (100)…», un texto que el mesero no puede corregir (no escribió ningún nombre).
- **Escenario**: área con mesas «Terraza 1» a «Terraza 10» (sin la palabra «Mesa», el nombre lleva cada nombre
  completo). La unión de las nueve primeras se llama «Terraza 1 + Terraza 2 + … + Terraza 9» (105 caracteres). Con las
  mesas «Mesa N» del propietario hacen falta unas 20 mesas de dos cifras («Mesa 10 + 11 + … + 29», 102).
  1. Cuenta sin nombre en «Terraza 1» → `POST restaurant/table-unions {start: Terraza 1, table_ids: 2…9}` → 400
     `VALIDATION_ERROR`.
  2. `POST restaurant/accounts {table_id: Terraza 1, union_table_ids: 2…9, items}` sin nombre → 400
     `OPERATION_VALIDATION`.
  3. Unión de 8 (93 caracteres) con su cuenta sin nombre + `POST …/tables {table_id: Terraza 9}` → 400 `VALIDATION_ERROR`.
  4. Con la unión de 9 creada desde una cuenta con nombre propio («Familia López»): abrir otra cuenta sin nombre en la
     unión → 400; mover a «Terraza 2» la cuenta sin nombre de «Terraza 10» → 400 `VALIDATION_ERROR`; un pedido de la cola
     sin conexión a «Terraza 3» → 400 `OPERATION_VALIDATION` (Android lo deja rechazado con ese texto).
     Nada queda a medias (la transacción se revierte entera), pero la unión, válida para la §1, no se puede usar.
- **Prueba**: `$SP/m1/revisor/nombre-largo-de-la-union.patch` (`TestStageM1ReviewLongUnionName`, 4 subpruebas, 6
  comprobaciones); salida en `nombre-largo-de-la-union.txt` (falla 6 de 6).
- **Para decidir al corregir** (no lo decide el revisor): recortar la referencia neutra de la cuenta a 100 caracteres
  (por ejemplo «Terraza 1 + Terraza 2 + …»), dejar a la cuenta el nombre de su mesa cuando el de la unión no cabe, o
  limitar las uniones a un nombre de hasta 100 caracteres con un rechazo claro (esto cambia el límite de 40 mesas de la
  lectura 12 de M1-D25).

#### R2. Las horas de la unión y del cobro no siguen el orden del lock — debe corregirse

- **Causa**: cada petición abre su transacción (`administration/service.go:255`), se autentica (entre otras cosas
  `UPDATE app.sessions SET last_seen_at` en `service.go:599`, que espera si otra petición de la misma sesión tiene la
  fila) y **después** toma 7820148305 (`operations/handler.go:57`, `payments/handler.go:100`). Todas las horas que se
  guardan son `now()`, la hora en que **empezó** la transacción: `closed_at` del cobro (`payments/collect.go:648`),
  `joined_at`/`created_at` (valor por omisión de la 0031), `left_at` y `dissolved_at` (`tableunions/settle.go:107` y
  `:111`, `operations/table_unions.go:826`). Una petición que empezó antes y tomó el lock después guarda una hora
  anterior a la de escrituras que ya se confirmaron antes que ella.
- **Efecto**: `tableunions.UnionNameAtSQL` (`tableunions/sql.go:24`) compara esas horas para saber en qué unión estaba
  la mesa «en ese instante». Lo usan el ticket, la precuenta y el comprobante de una cuenta cobrada o cancelada
  (`payments/finance.go:31`; el ticket del cobro se arma con esa lectura, `collect.go:608` y `:618`) y «Cuentas
  canceladas» (`reports/cancellations.go:59`). La venta (`sales_recognitions`, M1-D3) sí queda bien porque lee la unión
  abierta en la misma transacción, así que el ticket y la venta dicen lugares distintos.
- **Escenario 1** (forzado sin depender de la suerte: la prueba retiene la fila de la sesión de la caja para que el cobro
  espere con su transacción ya empezada): cuenta enviada en «Mesa 5»; la caja manda el cobro total (empieza a las _t₀_);
  mientras espera, el mesero une «Mesa 5 + 6» (`createTableUnion`, a las _t₁_ > _t₀_); el cobro termina. Resultado:
  venta «Mesa 5 + 6»; **ticket del cobro y reimpresión «Mesa 5»**; la unión queda `dissolved_at < opened_at`, sus dos
  mesas con `left_at < joined_at` y su historial dice «se deshizo» antes de «se unió».
- **Escenario 2**: cuentas en «Mesa 8 + 9»; la caja manda el cobro de la de «Mesa 9»; mientras espera, el mesero quita
  «Mesa 8» (la unión se deshace) y une «Mesa 9 + 10». Resultado: venta «Mesa 9 + 10»; **ticket y reimpresión «Mesa 8 +
  9»**, una unión que ya no existía cuando se cobró.
- **Sin forzarlo** pasa también: en la mezcla de la prueba propia (40 rondas, tres corridas) quedaron 3, 3 y 5 cobros en
  uniones cuyo ticket dice la mesa sola (de 45 a 50 cobros en uniones por corrida), además de mesas que salieron antes de
  entrar e historiales hacia atrás. En ese caso el ticket del momento sale bien y la que sale mal es la reimpresión: el
  cobro se confirmó primero y una escritura que había empezado antes deshizo la unión con una hora anterior al cobro.
  En el restaurante basta con que dos terminales actúen sobre la misma unión con milisegundos de diferencia, o que la
  caja tenga otra petición en curso (la pantalla se refresca con cada evento).
- **Prueba**: `$SP/m1/revisor/hora-de-la-transaccion.patch` (`TestStageM1ReviewClockOrder`, 2 subpruebas; falla 3 de 3
  con `-count=3`), salida y el experimento sin forzar en `hora-de-la-transaccion.txt`; el experimento como prueba
  temporal en `hora-de-la-transaccion-experimento.patch` (no falla: solo cuenta; necesita la prueba propia confirmada).
- **Para decidir al corregir**: el ticket de una cuenta cobrada puede leer la instantánea de su venta
  (`sales_recognitions.table_union_name`, escrita en la misma transacción que el cierre) en lugar de
  `UnionNameAtSQL`; para «Cuentas canceladas», guardar la unión en la cancelación (la 0031 admite columnas nuevas) o en
  el evento; y las horas de la unión tomarlas con `clock_timestamp()` después del lock, o mostrar el historial por su
  secuencia. `closed_at` y el día comercial no deberían cambiar en esta etapa.

#### R3. Una violación del índice único de miembros no daría el código del contrato — menor

- `table_union_members_open_table_idx` (23505) se traduce en `administration/http.go:161` a 409 `ALREADY_EXISTS` «Ya
  existe un registro con ese nombre o identificador.», no a `TABLE_IN_UNION`. Hoy **no se alcanza** por la API: toda
  inserción de miembros (`table_unions.go:520` y `:758`) se valida antes con el mismo predicado del índice
  (`left_at IS NULL`, `tableunions.UnionIDSQL`) y con 7820148305 exclusivo, y no hay otro escritor de miembros. Si se
  alcanzara, la transacción se revierte entera (no queda nada a medias) y no es un 500. Sugerencia: traducir esa
  restricción a `TABLE_IN_UNION` como `payments/shifts.go:415` hace con la suya. Sin prueba (no hay escenario por la API).

#### Para decidir (no son defectos del contrato vigente)

- **D-R1. `seen_union_id` de una unión que sigue abierta pero ya no tiene la mesa.** La lectura 5 de M1-D25 dice que
  `seen_union_id` de una mesa que ya no está unida no rechaza. Eso cubre también este caso: la terminal ve «Mesa 5 + 6 +
  7» y abre una cuenta nueva por «Mesa 7» con `seen_union_id` de esa unión; a la vez otra terminal quita «Mesa 7». La
  cuenta se abre sola en «Mesa 7» (`accounts_write.go:258`, `requireSeenUnion` sale sin error cuando la mesa no está en
  una unión; lo afirma la prueba de la A en `stagem1_unions_test.go:502-507`, cuyo comentario dice «una unión que ya no
  existe» aunque la unión sigue abierta) y la unión sigue abierta en «Mesa 5 + 6» sin ella: las dos terminales «ganan» y la cuenta queda en un lugar
  distinto del que veía el mesero, sin aviso. No pierde nada ni deja una mesa en dos lugares. Si se quiere que la
  perdedora reciba un mensaje (§2.3), el código del contrato que encaja es `TABLE_UNION_CONFLICT` («La unión cambió en
  otra terminal…») cuando la unión vista sigue abierta sin esa mesa; cambiaría la lectura 5 (la cola sin conexión ya
  tiene su regla propia en el contrato §7 y no cambiaría).
- **D-R2. Nombre de una mesa igual al de un grupo.** Crear o cambiar un grupo con el nombre de una mesa del área se
  rechaza (`TABLE_GROUP_NAME_TAKEN`, `table_groups.go:85`), pero crear o renombrar una mesa con el nombre de un grupo
  activo del área se acepta (`tables.go:112-169` no lo revisa): queda una mesa «Mesa 3» junto al grupo «Mesa 3», y la
  unión de todas sus secciones también se llama «Mesa 3». No es de concurrencia; el contrato solo habla del nombre del
  grupo.

### La prueba de concurrencia de la Fase A (`TestStageM1Concurrency`)

- **Sí es concurrente de verdad**: diez gorrutinas, cada una con su petición por `ServeHTTP`, su transacción y su
  conexión del pool, salen juntas de un canal cerrado. Pasó 20 de 20 con `-count=20 -race`.
- **Sus comprobaciones cubren la §2.3**: ninguna mesa en dos uniones abiertas, ninguna unión abierta sin cuentas o con
  menos de dos mesas, mesas de una sola área, ningún miembro abierto de una unión deshecha
  (`expectStageM1Invariants`); las cuentas abiertas son exactamente las que respondieron 200 (ninguna perdida ni
  duplicada); cada perdedora recibe un 409 de una lista del contrato; cada mesa unida se lee ocupada.
- **Lo que no cubre** (lo cubre la prueba propia):
  - el orden de salida es fijo y el cobro gana siempre (25 de 25 en las 20 corridas), así que «agregar G», «agregar B» y
    «quitar F» casi siempre encuentran la unión ya deshecha (`TABLE_UNION_NOT_OPEN` en 17, 21 y 20 de 25): agregar y
    quitar contra una unión abierta con cuentas casi no compiten;
  - no mezcla mover, combinar, descartar, la cola sin conexión, el atajo de un grupo, `seen_union_id` con una unión vista
    ni el `PATCH` de mesas (área, activa, fuera de servicio);
  - no comprueba que la perdedora no deje su clave de operación, ni el nombre de las cuentas (M1-D14), ni que la lista de
    cuentas de una mesa unida sea la de toda la unión, ni las horas (R2).

### La prueba propia (`TestStageM1ReviewConcurrency`)

- **Mezcla** (tres terminales: dos meseros con «Unir mesas» y la caja; 22 escrituras por ronda, en orden al azar y con
  0 a 2 ms de desfase): abrir solas «Mesa 1», «3B» y «Mesa 6» (esta sin ver la unión); unir con el primer producto «Mesa
  1 + 2», «Mesa 2 + 4», «6 + 5» (con «5» en borrador), «Mesa 11 + 12» y el atajo del grupo «Mesa 3»; crecer «Mesa 9»
  ocupada con «Mesa 8»; agregar «Mesa 8» y «Mesa 4» y quitar «Mesa 7» de «Mesa 6 + 7» con la misma revisión; mover la
  cuenta de «Mesa 10» a «Mesa 7»; combinar la única cuenta de la unión con la de «Mesa 10» y cobrarla a la vez;
  descartar la cuenta en borrador de «5»; abrir en la unión desde «Mesa 7» con `seen_union_id`; dos pedidos de la cola
  sin conexión (a «Mesa 7» con la unión vista y a «Mesa 2»); «Mesa 8» a la terraza, desactivar «Mesa 4» y «Mesa 12»
  fuera de servicio.
- **Al final de cada ronda**: las invariantes de la Fase A y además ninguna 5xx ni error genérico de la base; las rutas
  de uniones solo con códigos del §5; la clave de operación existe si y solo si respondió 200; las cuentas y las uniones
  nuevas son exactamente las de las respuestas 200; ninguna cuenta de antes desaparece; ninguna cuenta abierta en una
  mesa desactivada, fuera de servicio o de un área inactiva; ninguna mesa unida desactivada o fuera de servicio; cada
  cuenta sin nombre propio se llama como su lugar (M1-D14); la unión de un grupo tiene exactamente sus secciones; el
  historial de cada unión tiene su «joined» y, si se deshizo, su «dissolved»; el mapa lee cada mesa unida ocupada con
  las cuentas de la unión, y la lista de cuentas de una mesa unida trae las de toda la unión con su `table_union`.
- En 200 rondas (`-count=20`, 10 por corrida) cada carrera se resolvió de todas las formas posibles (totales del
  registro `$SP/m1/r-logs/review-concurrency-count20.log`):
  - cobrar la única cuenta de la unión: 147 veces ganó y 53 perdió contra la combinación (`BALANCE_CHANGED`); combinar:
    53 ganó, 137 encontró la cuenta ya cobrada (400) y 10 chocó con «mover la cuenta de Mesa 10» (`ACCOUNT_CONFLICT`);
  - agregar «Mesa 8»: 26 ganó, 26 `TABLE_UNION_CONFLICT`, 126 `TABLE_UNION_NOT_OPEN`, 13 `TABLE_UNION_AREA` (ya estaba en la
    terraza) y 9 `TABLE_IN_UNION` (ya la había tomado «Mesa 9»); quitar «Mesa 7»: 58 ganó, 46 `TABLE_UNION_CONFLICT`, 96
    `TABLE_UNION_NOT_OPEN`;
  - «Mesa 8 a la terraza»: 105 ganó y 95 `TABLE_IN_UNION`; «Mesa 12 fuera de servicio»: 164 ganó y 36
    `TABLE_HAS_OPEN_ACCOUNTS`; «unir Mesa 2 + 4»: 13 ganó, 93 `TABLE_IN_UNION`, 77 `TABLE_UNION_OCCUPIED`, 17
    `TABLE_UNION_UNAVAILABLE`;
  - abrir en la unión desde «Mesa 7» con la unión vista: 125 ganó (a veces ya sola en «Mesa 7», ver D-R1) y 75
    `TABLE_HAS_ACCOUNT`.
    Ninguna 5xx, ningún error genérico de la base y ninguna invariante rota. (El `BALANCE_CHANGED` al cobrar una cuenta que
    ya se combinó es de antes de la etapa y no se revisó.)

### Lo que se revisó y está bien

- **Lock antes de leer**: `operations.Handle` (`handler.go:57-63`) y `payments.Handle` (`handler.go:100-106`) toman
  7820148305 exclusivo antes de cualquier lectura de mesas o cuentas en las escrituras; antes solo hay permisos
  estáticos y validación del cuerpo. La vista previa y `getTableUnion` lo toman compartido y no escriben.
- **Orden de locks**: ninguna ruta nueva toma 7820148304, 7820148308 ni 7820148311; `SettleEmpty` y los eventos
  (7820148306) van después del trabajo, y los cupos (7820148311) siguen al final en `dispatch`.
- **Aislamiento**: las transacciones son READ COMMITTED, así que toda lectura después del lock ve lo último confirmado
  (no hay instantánea vieja que decida dentro del lock).
- **Todos los caminos que cambian el estado o la mesa de una cuenta** pasan por `operations.Handle` o `payments.Handle`
  (cancelar, descartar, liberar, combinar, mover, dividir, transferir, editar, la cola sin conexión, cobrar, entregar y
  la devolución que reabre); no hay otra escritura de `restaurant_accounts` en el servidor. `SettleEmpty` corre en los
  dos al final, en la misma transacción.
- **Miembros y uniones** solo se escriben en `operations/table_unions.go` (y el renombrado de una mesa unida,
  `tables.go:175`) y en `tableunions/settle.go`, siempre bajo el lock exclusivo; `Dissolve` pone `left_at` a todos los miembros, así que el índice parcial y las lecturas
  (`UnionIDSQL`, `PlaceTablesSQL`) nunca ven un miembro abierto de una unión deshecha.
- **Ocupación (M1-D10)**: `tableQuery` con `PlaceTablesSQL` decide desactivar y fuera de servicio (`PATCH` y
  `tables/{id}/service`); cambiar de área una mesa unida o agrupada se rechaza; desactivar un área con una unión se
  rechaza porque sus cuentas están en mesas del área; abrir sola una mesa unida entra en la unión, o con
  `seen_union_id` distinto da `TABLE_IN_UNION`, y `seen_account_ids` se revisa contra toda la unión.
- **Creación diferida (M1-D9)**: con una mesa ocupada o unida, una sección ocupada, un producto agotado o sin «Unir
  mesas», no queda ni cuenta, ni unión, ni miembros, ni historial, ni borrador, ni eventos, ni clave de operación, ni
  registro (salvo la denegación del 403); la misma clave con otro cuerpo da `IDEMPOTENCY_CONFLICT` y la repetición
  después de que la unión cambió devuelve la primera respuesta sin escribir (prueba `baf5cf00`).
- **Idempotencia** de las rutas de uniones y grupos: la de siempre (`restaurant_operations`, huella del cuerpo y la
  ruta), comprobada después del lock.
- **Eventos**: en la misma transacción (tabla `realtime_events`), se revierten con ella. La unión que nace con el primer
  producto no publica `restaurant.table-unions.changed` (la regla genérica excluye `accounts`), pero sí
  `restaurant.account.opened` y `restaurant.account.table_union`; el escritorio y Android se refrescan por ámbito, así que
  no falta nada.
- **Grupos**: el índice de nombre activo y la unicidad de secciones se validan antes con el mismo criterio; un grupo con
  una sección unida no cambia; crear un grupo con una mesa unida se rechaza.
- **Importador**: toma 7820148305 → 7820148304 → 7820148308 y **después** vuelve a comprobar que la base no tiene
  cuentas (`legacyimport/write.go:272-277`), así que nunca corre con uniones abiertas.
- **«Quitar datos de prueba»**: no toca mesas, áreas, cuentas, uniones ni grupos (`maintenance/apply.go`).
- **Cola sin conexión** (contrato §7): pasa por `Handle`; las cuentas no vistas y las partidas ya recibidas se buscan en
  toda la unión; un rechazo con confirmación no guarda la clave.

### Pruebas realmente corridas

Todas con `$SP/m1/r-int.sh` (como `iso.sh go-int`: base `adel_pos_test_r_*` en el clúster 55499, borrada al final; con
`-v`) o con `iso.sh go-int`. Registros en `$SP/m1/r-logs/` y `$SP/m1/revisor/`.

| Prueba                                                                                        | `-count`                            | `-race` | Resultado                                                                                      |
| --------------------------------------------------------------------------------------------- | ----------------------------------- | ------- | ---------------------------------------------------------------------------------------------- |
| `TestStageM1Concurrency` (Fase A, 25 rondas por corrida)                                      | 20                                  | sí      | Aprobó 20 de 20 (343 s). El cobro ganó 25 de 25 en todas las corridas                          |
| `TestStageM1ReviewConcurrency` (propia; 3 y 8 rondas, para afinarla)                          | 1 y 1                               | sí      | Aprobó                                                                                         |
| `TestStageM1ReviewConcurrency` (propia, 10 rondas por corrida)                                | 20                                  | sí      | Aprobó 20 de 20 (200 rondas, 371 s)                                                            |
| `TestStageM1ReviewDeferredLeavesNothing` (6 subpruebas)                                       | 1                                   | sí      | Aprobó                                                                                         |
| `TestStageM1ReviewLongUnionName` (parche R1, sin confirmar)                                   | 1 y 1 (antes y después de `format`) | sí      | **Falla**: 6 de 6 comprobaciones, en las dos corridas                                          |
| `TestStageM1ReviewClockOrder` (parche R2, sin confirmar)                                      | 1 y 3                               | sí      | **Falla**: las 2 subpruebas, en las 4 corridas                                                 |
| Experimento `TestStageM1ReviewNaturalClockTmp` (40 rondas de la mezcla propia; sin confirmar) | 1, tres veces                       | no      | Aprobó (solo cuenta): 5, 3 y 3 tickets distintos de la venta, de 50, 45 y 46 cobros en uniones |
| `iso.sh go-int ./internal/operations` (completo, con las dos pruebas confirmadas)             | 1                                   | sí      | Aprobó (933 s)                                                                                 |
| `iso.sh go-int ./internal/payments`                                                           | 1                                   | sí      | Aprobó (8 s)                                                                                   |
| `iso.sh go-int ./internal/kitchen`                                                            | 1                                   | sí      | Aprobó (8 s)                                                                                   |
| `go vet -tags=integration ./internal/operations`                                              | —                                   | —       | Sin avisos                                                                                     |
| `iso.sh format` y `fmt-check`; md5 de la especificación                                       | —                                   | —       | Sin cambios pendientes; `4750ae46956590f208bf486477bb0bbf` antes y después                     |

**No verificado**: el camino de renombrar una mesa unida con R1 (solo por lectura del código); «Cuentas canceladas» con
R2 (mismo fragmento `UnionNameAtSQL`, sin prueba propia). No se corrió `go-int ./cmd/ad-el-pos-server` (no hizo falta:
no se tocó código del producto) ni `go-unit`.

### Decisiones del coordinador al cerrar la revisión

Se fusionan en `etapa-m1` las dos pruebas del revisor que aprueban. La §2.3 se cumple. Los hallazgos los corrige la
Fase A con las pruebas del revisor (`$SP/m1/revisor/`):

- **M1-D29. R1, nombre largo.**
  - La unión conserva su nombre completo en el mapa, la lista, los documentos, la comanda y el KDS. El límite de 40
    mesas de M1-D25 sigue.
  - La referencia neutra de una cuenta sin nombre propio (M1-D14) se recorta cuando el nombre de la unión pasa de 100
    caracteres. El corte va en el último « + » que quepa, sin partir el nombre de una mesa, y termina en « + …», por
    ejemplo «Terraza 1 + Terraza 2 + … + Terraza 8 + …».
  - Vale en cada camino: unir, agregar, abrir, la unión diferida, mover, la cola sin conexión y renombrar una mesa
    unida.
  - El KDS no repite esa referencia recortada como subtítulo.
- **M1-D30. R2, horas fuera del orden del lock.**
  - El ticket, la precuenta y el comprobante de una cuenta cobrada leen el lugar de la instantánea de su venta
    (`sales_recognitions`), que se escribe en la misma transacción que el cierre.
  - Una cuenta cancelada guarda su unión al cancelarse, en la misma transacción. «Cuentas canceladas» y sus documentos
    leen ese dato. La Fase A elige el lugar (una columna nueva en la 0031 o el evento) y lo explica.
  - Las horas de la unión (apertura, entrada y salida de mesas, deshacerse y su historial) se toman con
    `clock_timestamp()` después del lock, y el historial se ordena también por su secuencia.
  - `UnionNameAtSQL` queda solo donde no hay instantánea.
  - `closed_at` y el día comercial no cambian.
- **M1-D31. R3.** La violación de `table_union_members_open_table_idx` responde `TABLE_IN_UNION`, como hace
  `payments/shifts.go` con la suya.
- **M1-D32. D-R1: `seen_union_id` de una unión abierta que ya no tiene la mesa.**
  - `openAccount` responde `TABLE_UNION_CONFLICT`, sin guardar nada, y el cliente conserva lo capturado. Así la
    perdedora recibe el porqué (§2.3) y el pedido no cae en un lugar distinto del que veía el mesero.
  - Esto corrige la lectura 5 de M1-D25 en este caso. Si la unión vista ya se deshizo y la mesa está libre, la cuenta se
    abre como siempre.
  - La cola sin conexión no cambia: sigue la tabla del contrato §7, «La mesa salió de la unión y quedó libre → cuenta
    nueva en la mesa».
- **M1-D33. D-R2: una mesa con el nombre de un grupo.**
  - Crear o renombrar una mesa, o cambiarla de área, con el nombre de un grupo activo de su área se rechaza con
    `TABLE_GROUP_NAME_TAKEN`, cuyo texto («Ya hay una mesa o un grupo con ese nombre en esta área.») ya cubre los dos
    casos.
  - Un choque con otra mesa sigue como hoy.

## Fase C · Aplicación de meseros: tarjeta única, ajustes de Android y unir mesas

Árbol `$SP/wt/m1-c`, rama `etapa-m1-c` (desde `etapa-m1` `8166b86a`; con la Fase A fusionada en `d5db25fd`),
`ISO_TAG=C`, Go temporal en 8241/8501 con la confianza de prueba de licencias. Emulador: **solo `ADEL_A1_Tablet_API26`
(5572)**; la copia de `Pixel_Tablet` (5570) no arranca mientras el `Pixel_Tablet` del propietario (5556) esté abierto sin
`-read-only` («Another emulator instance is running. Please close it or run all emulators with -read-only flag.»). No se
tocó el del propietario.

### Commits (rama `etapa-m1-c`)

| Commit     | Qué                                                                                               |
| ---------- | ------------------------------------------------------------------------------------------------- |
| `ceb6206d` | §5.5 una tarjeta por cuenta con «Tu cuenta» y §5.2 sin etiqueta de cocina sin preparación         |
| `e0d7f126` | §5.6 Mesas ve al momento la cuenta que responde Go («$0.00 · Borrador») y la suite `unions` u1–u4 |
| `57fd8918` | §5.6 textos cortados y contraste de las teclas del PIN; `StageM1ScreensTest`                      |
| `d5db25fd` | fusión de `etapa-m1` (Fase A)                                                                     |
| `cfcf09e0` | unir mesas (contrato §10), M1-D13, `seen_union_id`, Room v7                                       |
| `4376e95f` | suite `unions` u5–u10 y lo que encontraron                                                        |
| `09cfc12e` | meseros 0.5.0 (versionCode 6)                                                                     |
| `d5d84b2b` | §5.7 el teclado ya no tapa la lista de Para llevar en Android 8                                   |
| `415fea47` | `TABLE_UNION_CONFLICT` (M1-D32) y `TABLE_IN_UNION` con pruebas de JVM                             |

### Qué se hizo

1. **§5.5 · Una tarjeta por cuenta con «Tu cuenta»** (M1-D16). `TableCards` (`core:domain`, regla pura) arma el panel:
   - una tarjeta por cuenta abierta de Go con folio, total, el envío de lo del mesero en ella (el más urgente:
     «Requiere tu revisión», «Enviando», «Por enviar», una captura con productos sin enviar, o «Recibido por el
     restaurante») y la etiqueta de cocina, con «Imprimir precuenta» como antes;
   - «Tu cuenta» arriba a la derecha cuando `assigned_user_id` es el mesero activo (se lee del DTO de la cuenta);
   - lo que no tiene una cuenta abierta (un pedido de la cola o una captura de cuenta nueva con productos) es su propia
     tarjeta: el pedido con su estado («Por enviar»…) y la captura con «Por enviar» en el tono gris del borrador (nunca
     se envía sola, regla 18);
   - un pedido ya recibido va **dentro** de la tarjeta de su cuenta (antes salía dos veces); si su cuenta ya no está
     abierta, no se muestra;
   - se quitó «Lo tuyo en esta mesa»; las etiquetas de prueba `table-capture-…` y `table-order-…` siguen en el lugar que
     se toca (las suites existentes las usan).
2. **§5.2 · Sin etiqueta de cocina sin preparación** (M1-D15). `preparation_required` en `ConfirmedLine` y `Account`
   (ausente = `true`): sin etiqueta en el renglón de la captura, en la tarjeta del panel y en la lista de Para llevar.
   `released` dice «En cocina», como el escritorio (contrato §4; antes «Liberado a preparación»).
3. **§5.6 · Textos cortados, retraso y contraste** (causas abajo).
4. **Unir mesas** (§1, §3, contrato §10, M1-D13), con la capacidad `table-unions-v1` (en `AccessRepository`, de `GET
version`) y el permiso «Unir mesas» (`WaiterAction.JOIN_TABLES`, solo si Go lo dio):
   - **«Unir con…»** en el panel de una mesa: elegir mesas del área (las ocupadas, unidas o fuera de servicio,
     deshabilitadas con su motivo); la vista previa de Go da nombre, capacidad y problemas, tal cual. Desde una mesa
     ocupada, `createTableUnion` al momento; desde una libre, **la unión vive en la pantalla** (título, capacidad, «La
     unión se guarda con el primer producto…» y «Deshacer la unión») y nace con `union_table_ids`/`union_group_id` en
     `openAccount`. Salir sin capturar no guarda nada.
   - **Atajo del grupo** («Unir Mesa 3») en el panel de una sección y al tocar el nombre del marco en el mapa.
   - **Panel de la unión**: el nombre de Go como título, la capacidad, las cuentas de todas sus mesas (una tarjeta por
     cuenta) y «Agregar mesa» / «Quitar mesa» (con su nota: quitar no saca cuentas; con una sola mesa se deshace).
   - **Mapa**: el contorno sólido del rectángulo de sus mesas con `TableMapFit` (`UnionOutline`) con «Mesa 5 + 6 · 8
     lugares»; el grupo con un marco tenue y su nombre; las mesas se siguen tocando y abren la unión. **Lista**: una sola
     entrada por unión (`RestaurantView.entriesIn`).
   - **Errores de Go tal cual** en el diálogo.
   - **M1-D13 sin conexión**: unir pide conexión (los botones deshabilitados con «Para unir mesas necesitas conexión con
     la computadora principal.»). Una captura con una unión en pantalla que no se pudo guardar **nunca entra en la cola**:
     «Enviar a cocina» abre «Sin conexión no se pueden unir mesas. Puedes enviar el pedido solo a la Mesa 5 o esperar a
     tener conexión.» con «Enviar solo a la Mesa 5» y «Esperar» (si Go la rechazó, su mensaje y los mismos botones).
   - **`seen_union_id`** en `openAccount` y en `destination` de la cola: la unión que veía el mesero (`null` si ninguna);
     sin el dato (lo guardado por la 0.4.1, o un servidor sin la capacidad) no se manda. `TABLE_IN_UNION` se ofrece como
     `TABLE_HAS_ACCOUNT` con las cuentas de la unión (se leen su folio y su nombre).
   - **Room v7**: `captures.union_view` (JSON con la unión vista y la unión en pantalla), migración 6 → 7 con `ALTER
TABLE` y su caso en `AppDatabaseMigrationTest`. La cola no cambia de esquema (el destino JSON agrega
     `seen_union_known`/`seen_union_id` con valores por omisión).
5. **Versión 0.5.0** (versionCode 6) en `android/gradle.properties`.

### §5.6 · Causas y cifras

| Punto                                                 | Causa                                                                                                                                                                                                                                                                                                                                    | Arreglo                                                                                                                                                                                                                                                                                                                          |
| ----------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| «Enviado · sin confirmar en coci…» (tableta vertical) | Todas las etiquetas de estado (`StateBadge`, `WaiterBadges.kt`) eran de un renglón con puntos suspensivos; en el panel de 42 % de 800 dp quedan ≈ 237 dp para el texto y la etiqueta necesita ≈ 240                                                                                                                                      | La etiqueta baja a un segundo renglón por palabras (`maxLines = 2`); en la tarjeta, envío y cocina van en un `FlowRow`                                                                                                                                                                                                           |
| «Nuevo pe…» (teléfono vertical)                       | La barra común de `android-core` mide primero las acciones (mesero con su nombre y «En línea») y el título se queda con lo que sobra (≈ 124 dp para 264)                                                                                                                                                                                 | En el teléfono vertical (`currentTopBarHasRoom` falso) el título es «Nuevo pedido» y el mesero va solo con su inicial                                                                                                                                                                                                            |
| «Terraz…» (tableta vertical)                          | Las áreas iban en la misma fila que «Lista \| Mapa» y «Pendientes» con desplazamiento: con ≈ 160 dp, la segunda pestaña quedaba cortada en la orilla (visto en la captura de la API 26, «Terra:»)                                                                                                                                        | `TablesBar` (un `Layout` propio): si las pestañas no caben completas (`maxIntrinsicWidth`), bajan a su propia fila de todo el ancho; si caben, una sola fila como en la A3                                                                                                                                                       |
| «2 pedidos por en…» (teléfono)                        | Es la insignia de la barra (`ConnectionIndicator` de `android-core`), no la de Pendientes: el menú del mesero con su nombre, «En línea» y la insignia de un renglón no caben en 360–411 dp                                                                                                                                               | Sin tocar `android-core` (cocina queda en 0.1.0): en el teléfono vertical la raíz pasa 0 pedidos a la barra común y dibuja `PendingOrdersCompact` (el mismo texto, en dos renglones) y el mesero con su inicial                                                                                                                  |
| «$0.00 · Borrador» después de «Enviar a cocina»       | El panel pintaba la cuenta de la última lectura (la del primer producto: solo borrador, total $0.00 y `preparation_state: draft`); la respuesta de `confirmDraft` traía la cuenta nueva pero solo la usaba la cola; el panel esperaba la relectura que dispara el canal (400 ms después del evento, más `restaurant/state` y `overview`) | La cuenta de las respuestas de `openAccount`, `saveDraft` y `confirmDraft` se aplica al momento (`RestaurantRepository.applyAccount`, solo si su revisión no es más vieja)                                                                                                                                                       |
| Teclas de «Vuelve a escribir tu PIN» (tema claro)     | Las teclas eran `FilledTonalButton` (`secondaryContainer` = `graySoft` #EEF1F5) sobre el diálogo (`surfaceContainerHigh` = `banner` #EEF2F7): **1.01:1**, la tecla no se distinguía                                                                                                                                                      | Teclas del color del panel con borde gris del tema (`gray`): el borde contra el diálogo da **5.07:1** en el claro y **7.06:1** en el oscuro (AA pide 3:1 para el contorno de un control); el número sobre la tecla, **14.22:1**. «Borrar» con el mismo borde (el de Material daba 2.84:1). En la entrada (fondo #F6F7F9), 5.31:1 |

**El retraso, medido** (suite `unions` u2 en `ADEL_A1_Tablet_API26`, Go temporal local; el mesero espera a que Mesas ya
haya leído la cuenta con el borrador, como en la vida real, y la prueba mira la tarjeta cada 16 ms desde el toque de
«Enviar a cocina»; registro `AdelOnGo` «M1 · retraso», guardado en `$SP/m1/c-logs/retraso-antes-logcat.txt` y
`retraso-despues-logcat.txt`):

|                                            | Mesa 5  | Mesa 7  | Terraza 1 |
| ------------------------------------------ | ------- | ------- | --------- |
| Respuesta de Go («Recibido»)               | +46 ms  | +24 ms  | +25 ms    |
| **Antes**: la tarjeta con el total de Go   | +630 ms | +672 ms | +648 ms   |
| **Antes**: con «$0.00» a la vista          | 584 ms  | 648 ms  | 623 ms    |
| **Después**: la tarjeta con el total de Go | +15 ms  | +40 ms  | +12 ms    |
| **Después**: con «$0.00» a la vista        | 0 ms    | 0 ms    | 0 ms      |

### §5.7 · `takeaway t6` en Android 8

**Causa: de la aplicación (y solo en Android 8).** Reproducido en `ADEL_A1_Tablet_API26` (t1–t5 aprueban, t6 falla, tres
veces). Con un registro con hora de todo lo que pasa en t6 (canal, lecturas de Go, ediciones por enviar) se vio que **el
dato sí se pone al día**: la edición sale a los ≈ 0.9 s, Go la confirma, llega el evento `restaurant.account.updated` del
ámbito `takeaway` y la lectura de Go dice «Luisa Martínez» a los ≈ 1.5 s. Lo que no se veía era el pedido: la captura
de la pantalla al fallar (`capturas-C/api26-t6-antes-teclado-abierto.png`) muestra **el teclado abierto** después de
cerrar «Nombre y teléfono» (`WindowInsets` del teclado: 426 px; el método de entrada activo sin texto) y la lista en una
franja arriba de él con solo «Nuevo pedido» y el folio 1; el pedido editado quedaba debajo, fuera de la vista, y la
prueba (que no desliza) no lo encontraba. En Android 8, al cerrar el diálogo el teclado pasa a la ventana de la
actividad y se queda abierto; en Android 11+ se cierra solo (por eso aprobaba en los otros emuladores). Un mesero real
veía lo mismo: el teclado tapando la lista.

**Arreglo** (`core/capture/.../Takeaway.kt`, `CustomerDialog`, que usan Para llevar y la captura): al guardar o cancelar
se suelta el foco y se esconde el teclado, y al salir el diálogo de la composición se esconde también desde la ventana
de la actividad (150 ms después, cuando Android ya quitó la ventana del diálogo). Con eso `takeaway` t1–t9 aprueban en
Android 8. **Descartado**: el canal de eventos (abierto, entrega el evento a ≈ 1 s), la relectura (llega a ≈ 1.5 s) y la
regla de la lista (`TakeawayBoard`).

Los otros diálogos con campos de texto (el motivo de modificar o cancelar lo enviado) podrían dejar el teclado abierto
igual en Android 8; no se cambiaron porque nada lo reportó y su pantalla no queda tapada de la misma forma (no
verificado).

### Decisiones propuestas

1. **La captura sin cuenta dice «Por enviar» con el tono del borrador** (gris y lápiz), no el ámbar del pedido en la
   cola. _Porqué_: M1-D16 pide la misma etiqueta «Por enviar»; con el mismo color, el mesero creería que una captura
   saldrá sola, y la regla 18 dice que un borrador nunca se envía solo. El texto es el mismo; el tono y el ícono, no.
2. **En la tarjeta, el envío muestra lo más urgente de lo del mesero** (revisión, enviando, por enviar, captura, recibido)
   y se toca para abrir ese pedido o esa captura; la tarjeta entera abre la cuenta, como antes.
3. **`released` dice «En cocina» en Android** (contrato §4, M1-D18), igual que el escritorio en las vistas de la cuenta.
4. **Un renglón cancelado conserva «Cancelado»** aunque `preparation_required` sea `false` (anotado en
   `contrato-dudas.md`, Fase C).
5. **La barra del teléfono vertical sin tocar `android-core`**: el mesero solo con su inicial y «N pedidos por enviar»
   de meseros en dos renglones; «Nuevo pedido» como título. _Porqué_: cambiar `AdelTopBar` o `ConnectionIndicator`
   cambia el código de la aplicación de cocina, que M1-D14 deja en 0.1.0. **Para la Fase F**: con pedidos por enviar el
   título del teléfono sigue quedando corto (no cabe todo en 360–411 dp); verificar con `Pixel_8`.
6. **Las áreas bajan a su propia fila solo cuando no caben** (A3 pedía una fila; así ninguna pestaña queda cortada).
7. **Teclas del PIN con fondo del panel y borde gris** en la entrada y en «Vuelve a escribir tu PIN», en los dos temas.
8. **Unir pide conexión y sesión**: «Unir con…», el atajo, «Agregar mesa» y «Quitar mesa» se ven deshabilitados sin
   conexión con su motivo (necesitan la vista previa y la escritura de Go); «Deshacer la unión» en pantalla siempre se
   puede (no hay nada en Go).
9. **Una unión en pantalla vive en la pantalla** (`SavedStateHandle`): sobrevive a la rotación; elegir otra mesa la
   quita. La captura guarda la unión (Room v7) y no la pierde al reabrirse; solo «Enviar solo a la mesa» la quita.
10. **El título de la captura en una unión es el nombre de la unión** («Mesa 5 + 6»); el pedido guarda ese nombre.
11. **`TABLE_IN_UNION` y `TABLE_UNION_CONFLICT`** al abrir la cuenta: el primero se ofrece como «otra cuenta en la
    mesa» con las cuentas de la unión; el segundo (M1-D32) dice el texto de Go, vuelve a leer Mesas, conserva lo
    capturado y olvida la unión vista para el reintento.
12. **§5.7**: el arreglo del teclado está en `CustomerDialog` (Para llevar y la captura).
13. **Room v7** para la unión de la captura (columna nueva), con su migración probada y la actualización 0.4.1 → 0.5.0.

### Archivos

- **Dominio** (`android/core/domain/.../waiter/`): `TableCards.kt` (§5.5), `TableUnions.kt` (servicio, vista previa,
  `PendingUnion`, `UnionChoices`, `UnionOutline`, `TableContext`), `Restaurant.kt` (`TableUnion`, `TableGroup`,
  `entriesIn`, `unionOf`), `Accounts.kt` (`assignedUserId`, `preparationRequired`, `tableUnion`, `showsPreparation`),
  `Capture.kt` (`SeenUnion`, `UnionJoin`, `waitsForUnion`), `OrderQueue.kt`, `OrderFactory.kt`,
  `WaiterPermissions.kt` (`JOIN_TABLES`), `WaiterServices.kt` (`SendRequest.UnionPending`, `open`, `dropUnion`),
  `TakeawayBoard.kt`.
- **Datos** (`android/core/data/.../`): `waiter/TableUnionRepository.kt` (nuevo), `WaiterApi.kt` (4 rutas),
  `WaiterDtos.kt`, `WaiterMapper.kt` (`union`, `group`, `openBody`, `seenUnionJson`), `RestaurantRepository.kt`
  (`applyAccount`, uniones y grupos), `CaptureRepository.kt` (unión de la captura, `TABLE_IN_UNION`,
  `TABLE_UNION_CONFLICT`, `dropUnion`, `accountAnswered`, `placesChanged`), `OrderSender.kt`, `WaiterServer.kt`
  (`capabilities`), `access/AccessRepository.kt` (`serverCapabilities`), `db/AppDatabase.kt` (v7, `MIGRATION_6_7`),
  `waiter/db/WaiterEntities.kt` (`union_view`), `waiter/db/WaiterStorage.kt`, `schemas/…/7.json`.
- **Pantallas**: `feature/waiter/.../tables/TablesScreen.kt` (tarjetas, `TablesBar`, lista con uniones, panel de la
  unión, `JoinTablesDialog`), `TablesViewModel.kt` (unión en pantalla, diálogo), `TablesMap.kt` (contorno y marco),
  `WaiterDependencies.kt`; `core/capture/.../CaptureScreen.kt` (diálogo M1-D13, título corto), `CaptureViewModel.kt`,
  `Takeaway.kt` (§5.7), `OrderPanel.kt` (§5.2); `core/ui/.../waiter/WaiterBadges.kt` (dos renglones,
  `CaptureUnsentBadge`, `MineBadge`), `components/PinPad.kt` (contraste), `components/WaiterBadge.kt` (mesero compacto,
  `PendingOrdersCompact`); `feature/takeaway/.../TakeawayScreen.kt` (§5.2); `app/.../navigation/AdelPosApp.kt` (barra
  del teléfono), `AppContainer.kt`, `WaiterModeContainer.kt`; textos en `strings.xml` de `feature/waiter`,
  `core/capture` y `core/ui` (`strings_waiter.xml`).
- **Versión**: `android/gradle.properties` (0.5.0, 6).
- **Pruebas**: JVM `StageM1TableCardsTest`, `StageM1UnionsTest` (dominio), `StageM1UnionBodiesTest`,
  `TablePanelAccountsTest` (`applyAccount`), `CaptureSyncTest` (dos casos nuevos), `AppDatabaseTest` (v7);
  instrumentadas `StageM1ScreensTest`, `AppDatabaseMigrationTest` (6 → 7); de punta a punta `UnionsOnGoTest` (suite
  `unions`, u1–u10, en `scripts/android-e2e.sh`) y `TakeawayOnGoTest` t6 con su registro.

### Pruebas realmente ejecutadas

Todas en `ADEL_A1_Tablet_API26` (copia de solo lectura, consola 5572), contra el Go temporal `ISO_TAG=C` de este
árbol (desde `d5db25fd`, con la Fase A; antes, el de `etapa-m1` sin ella), base nueva por corrida. La instalación por
Gradle (`ddmlib`) se colgó en Android 8 (como en A3-D33): se instaló con `adb install` (`$SP/m1/c-install.sh`) y las
suites corrieron con `--no-install`.

| Prueba                                                                                                                                                                              | Dónde | Resultado                                                                                                                      |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- | ------------------------------------------------------------------------------------------------------------------------------ |
| JVM `:core:domain:test` (con `StageM1TableCardsTest` 10, `StageM1UnionsTest` 6)                                                                                                     | Mac   | aprobada, 0 fallas                                                                                                             |
| JVM `testDebugUnitTest` de todos los módulos (con `StageM1UnionBodiesTest` 4, `TablePanelAccountsTest` 2, `CaptureSyncTest` 12, `AppDatabaseTest`)                                  | Mac   | aprobada, 0 fallas                                                                                                             |
| `android-core :ui:testDebugUnitTest --rerun` (`PlainLanguageTest`, `ColorsOnlyInPaletteTest`, `PaletteContrastTest`)                                                                | Mac   | 7 de 7                                                                                                                         |
| `ktfmtCheck`, `iso.sh format` + `fmt-check`; md5 de la especificación antes y después                                                                                               | Mac   | sin cambios pendientes; `4750ae46…`                                                                                            |
| `unions` u1–u4 con Go sin la Fase A (tarjeta única, retraso antes y después, pedido sin conexión)                                                                                   | 5572  | 4 de 4                                                                                                                         |
| `unions` u1–u10 con la Fase A                                                                                                                                                       | 5572  | 10 de 10 (u7 falló una vez por la prueba: la lista del diálogo no se deslizaba; se agregó su etiqueta y se repitió solo u7)    |
| `takeaway` t1–t9 (con el arreglo de §5.7)                                                                                                                                           | 5572  | 9 de 9 (antes: t6 FALLA tres veces)                                                                                            |
| `adjustments` a1–a7                                                                                                                                                                 | 5572  | 7 de 7                                                                                                                         |
| `waiter` w1–w13c                                                                                                                                                                    | 5572  | 19 de 19                                                                                                                       |
| Actualización 0.4.1 → 0.5.0 (`$SP/m1/c-upgrade.sh`): 0.4.1 debug de una copia `git archive` de `main`, `waiter` w1 y w3 (pedido «Por enviar»), `adb install -r -t` de la 0.5.0 y w4 | 5572  | aprobada: mismo `firstInstallTime`, versionCode 5 → 6, sin volver a emparejar, el pedido sale una vez (Room 6 → 7 con la cola) |
| Firma: 0.4.1 de entrega y encima la 0.5.0 de entrega, sin abrirla; `apksigner verify --print-certs`                                                                                 | 5572  | mismo `firstInstallTime`; las dos con `fb6a4bac…7c86`                                                                          |
| Instrumentadas `StageM1ScreensTest` (4), `StageA3ScreensTest`, `WaiterScreensTest`, `TakeawayScreensTest`, `AppNavigationTest`, `ServerLinkScreensTest`                             | 5572  | 26 de 26                                                                                                                       |
| Instrumentada `AppDatabaseMigrationTest` (con 6 → 7)                                                                                                                                | 5572  | 8 de 8                                                                                                                         |

**Capturas** (tema claro, `$SP/m1/capturas-C/`): la tarjeta única en vertical y horizontal, el pedido «Por enviar», el
diálogo «Unir con…», el panel de la unión, el contorno en el mapa en las dos orientaciones, «Quitar mesa», la unión en
pantalla, el aviso de M1-D13, el atajo del grupo rechazado, «Mesa 3» unida, el refresco sin etiqueta, «Vuelve a escribir
tu PIN», las de `adjustments` a2 y a7 y la pantalla de t6 antes del arreglo (el teclado tapando la lista).

### Pendiente o no verificado

- **`Pixel_Tablet` (Android 17)**: no se corrió nada; su copia no arranca con el del propietario abierto. Lo cubre la
  Fase F o una corrida con el emulador del propietario cerrado.
- **Teléfono** (`Pixel_8`): la barra compacta, «Nuevo pedido» y la insignia en dos renglones solo se probaron con
  anchos fijos en la tableta (`StageM1ScreensTest`); lo verifica la Fase F.
- **M1-D29 a M1-D33**: la Fase A los está implementando; falta volver a fusionar `etapa-m1` y repetir `unions` contra
  ese Go. M1-D32 ya está en Android con prueba de JVM; M1-D29 no pide nada (se muestra el nombre que manda Go).
- **Suites no corridas** que tocan pantallas cambiadas: `field` (f4 usa `table-capture-…`), `precheck` (p1 toca la
  cuenta del panel), `alerts`, `license`, `access`, `catalog`: para la regresión de la Fase F.
- **Los otros diálogos con texto en Android 8** (motivo de modificar o cancelar): pueden dejar el teclado abierto igual
  que el del cliente; no se probaron.
- **La 0.4.1 frente a una unión** (riesgo de la Fase 0): no se probó con la 0.4.1 instalada.

### Para el coordinador

- Decidir si la copia de `Pixel_Tablet` se corre con el emulador del propietario cerrado, o si la Fase F lo cubre.
- Confirmar las decisiones propuestas 1, 4, 5 y 6 (son de presentación y no cambian reglas de Go).
- Avisar cuando M1-D29 a M1-D33 estén en `etapa-m1` para fusionar y repetir `unions` contra Go.

### Lecturas del contrato de la Fase C

Ninguna bloquea; Android implementó la lectura indicada.

1. **Renglón cancelado con `preparation_required: false`** (la Fase A lo manda `false` para un renglón cancelado que nunca
   necesitó preparación, decisión propuesta 10). Android sigue mostrando «Cancelado» en un renglón cancelado aunque Go
   diga `false`: es el estado del renglón, no una etiqueta de cocina, y sin él el mesero no distingue lo cancelado. Si el
   escritorio (Fase B) decide otra cosa, que coincidan.
2. **`seen_union_id` solo con la capacidad**: Android lo manda (id o `null`) solo si el servidor tiene `table-unions-v1`
   y la captura sabe qué unión veía; con un servidor sin la capacidad o lo guardado por la 0.4.1, el campo no va.
3. **`TABLE_IN_UNION` al abrir la cuenta**: como `details.accounts` son solo identificadores (M1-D25), Android lee cada
   cuenta (`GET restaurant/accounts/{id}`, hasta 6) para ofrecer «Agregar a «…»» con su folio y su nombre, igual que
   `TABLE_HAS_ACCOUNT`.
4. **`addTableToUnion` es de una mesa**: «Agregar mesa» y «Quitar mesa» eligen una mesa a la vez; la vista previa de
   agregar usa `{union_id, table_ids: [esa]}`.

### Decisiones del coordinador al cerrar la Fase C

Se fusiona `etapa-m1-c` en `etapa-m1`. La fase no cambió `android-core`, `cocina`, `backend` ni `desktop`.

- **M1-D34.** Se aceptan las decisiones propuestas 1 a 4 y 7 a 13 de la Fase C, y sus cuatro lecturas del contrato.
  Para el escritorio (Fase B) queda que un renglón cancelado conserva «Cancelado» aunque `preparation_required` sea
  `false`.
- **M1-D35. Las pestañas de área (propuesta 6).** Bajan a su propia fila solo cuando no caben; cuando caben, todo sigue
  en una fila como pidió la A3 (§4.7). La especificación M1 (§5.6) marca la pestaña cortada como defecto, y es la regla
  más reciente del propietario.
- **M1-D36. La barra del teléfono (propuesta 5).** La §5.6 pide que el título no se corte, así que no se deja para la
  Fase F. Lo termina la Fase C en los teléfonos de prueba.
  - Si se puede, solo en meseros.
  - Si hace falta cambiar `android-core`, el cambio no cambia lo que ve la aplicación de cocina. En ese caso cocina
    sube a **0.1.1**, como permite la especificación, con sus pruebas.
- **M1-D37. Emuladores de la etapa.** Mientras el propietario tiene abiertos sus `Pixel_Tablet` y `Pixel_8` sin
  `-read-only`, sus copias no arrancan. El coordinador creó dos AVD propios de la etapa con el mismo perfil y la misma
  imagen (Android 37, `google_apis_playstore`, 2 GiB, 10 GB), como los de la A1:
  - `ADEL_M1_Tablet_API37` (`pixel_tablet`, 2560×1600, 320 dpi), en lugar de la copia de `Pixel_Tablet`;
  - `ADEL_M1_Phone_API37` (`pixel_8`, 1080×2400, 420 dpi), en lugar de la copia de `Pixel_8` en la regresión.
- **Lo que le queda a la Fase C**, que se retoma al fusionar las correcciones de la Fase A:
  - repetir `unions` contra Go con M1-D29 a M1-D33 en las dos tabletas;
  - las suites de punta a punta en `ADEL_M1_Tablet_API37`;
  - M1-D36 en los teléfonos;
  - los otros diálogos con texto en Android 8;
  - la 0.4.1 frente a una unión;
  - `field` y `precheck`.
