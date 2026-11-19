# Etapa A3 · Avance

Ajustes de la aplicación Android y del escritorio tras las pruebas del propietario con la A2: ningún texto provisional
en la interfaz, un solo estado de conexión tolerante en Android, la pantalla de permisos agrupada por área con roles de
fábrica y «Restablecer permisos predeterminados», los ajustes de la aplicación Android (§4) y los de Terminales en el
escritorio (§5). Especificación: [docs/etapas/ETAPA_A3_AJUSTES.md](etapas/ETAPA_A3_AJUSTES.md). Rama `etapa-a3`, creada
desde `origin/main` (`5096f80`: servidor 0.13.0, esquema 26, Android 0.2.0, con la A2 fusionada). El servicio de la Mac
ya está en 0.13.0, esquema 26 (el propietario instaló el `.pkg` de la A2). Al terminar: aplicación **0.3.0** y servidor
**0.14.0**.

Coordinador y como máximo dos agentes a la vez, cada fase en su árbol de trabajo, probada en copias aisladas (clúster
temporal 55499, Go temporal en modo de pruebas con su escucha de red en loopback y emuladores sin ventana en modo de
solo lectura). **Ninguna prueba usa el servicio real de la Mac** (8181, 8443, 55432 ni su carpeta) y se mantiene la
protección de la A1 y la A2 contra su anuncio en la red (tipo de anuncio de pruebas `_adelpostest._tcp`). Durante las
fases, Android en `Pixel_Tablet` (Android 17) y `ADEL_A1_Phone_API33` (Android 13); los cuatro emuladores solo en la
regresión final. Sin dispositivo físico. Ningún commit lleva líneas de atribución (`CLAUDE.md` §0).

---

## Fase 0 · Verificación

Agente único de la Fase 0, en `$SP/wt/p0` (rama `etapa-a3-0`, `05af2b3`), sin cambios en el repositorio ni commits.
Todo lo experimental vivió en `$SP/spike-a3/` (copia con `git archive`, desechable): un Go temporal (`ISO_TAG=P0`,
8191/8451, base `adel_pos_test_p0_…`), `Pixel_Tablet` (5580) y `ADEL_A1_Phone_API33` (5582), siempre con
`AdelTestRunner` (NSD `_adelpostest._tcp`) y emparejando por la dirección escrita `10.0.2.2:8451`; nunca la red real.
Las rutas son del árbol `wt/p0` salvo que digan `spike`.

Resumen de lo que decide las fases A–E:

- **La causa de las desconexiones está encontrada y reproducida** (§0.3): OkHttp guarda una conexión libre hasta 5 minutos,
  Go la cierra a los 60 s (`IdleTimeout`), OkHttp **no revisa** una conexión reutilizada para un `GET` y la aplicación
  desactivó los reintentos de OkHttp; la comprobación periódica de **60 s exactos** cae siempre sobre esa conexión muerta.
  Una sola falla marca el dispositivo «sin conexión» mientras la barra (que solo mira la red de Android) dice «En línea».
  Reproducida tal como la vio el propietario: al pasar a «¿Quién eres?» por el bloqueo por inactividad, «Sin conexión con la
  computadora principal…» con la barra en «En línea», 32 s, en 4 de 5 bloqueos. Con el pool de 30 s (una línea), 0 fallas.
- **La 0.3.0 no necesita nada nuevo del servidor** (§0.1): sigue funcionando con un 0.13.0; `min_client_versions.android`
  no sube.
- **El servidor solo cambia para el escritorio y en textos** (§0.2, §0.4, §0.6): unos mensajes de Go con «etapa 7» o
  «todavía no»; catálogo de permisos con grupos, descripciones, dependencias y capacidades; roles de fábrica en el
  asistente; «Restablecer permisos predeterminados»; Terminales por tipo; y una sola `0027` (Terminales: `android` con
  «Cocina» o «Caja» → «Mesas» y su `CHECK`). Ninguna de esas rutas está en el contrato v1.
- **Una contradicción menor para decidir** (§0.7): el contrato v1 no documenta `POST restaurant/accounts/{id}/discard` ni
  `discardable`, que Android ya usa y que el punto 10 necesita.

### 0.1 Versiones

| Pieza                 | Hoy                                                                                     | Al terminar la A3                                          | Cómo                                                                                                                                                                                                                                                                                                                                                                                                                       |
| --------------------- | --------------------------------------------------------------------------------------- | ---------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Android               | `android/gradle.properties:10-11`: `adelpos.versionName=0.2.0`, `adelpos.versionCode=2` | `0.3.0` / `3`                                              | Solo esas dos líneas: es el único lugar (APK, `BuildConfig.VERSION_NAME` → `X-Client-Version`, `android/CLAUDE.md` «Versión de la aplicación»).                                                                                                                                                                                                                                                                            |
| Servidor y escritorio | `config/product.json`: `version` 0.13.0, `schema_version` 26                            | 0.14.0 y esquema **27** (si se agrega la `0027`, ver §0.6) | Editar `config/product.json` y `./scripts/dev.sh sync` (`scripts/sync-metadata.mjs`). El cambio a 0.13.0 de la A2 (`cb9b77f`) tocó 8 archivos: `config/product.json`, `backend/internal/product/metadata.json`, `desktop/src/generated/product.json`, `desktop/package.json`, `desktop/package-lock.json`, `desktop/src-tauri/Cargo.toml`, `Cargo.lock` y `tauri.conf.json`. El esquema sube en el commit de la migración. |

- **¿Necesita la 0.3.0 algo nuevo del servidor?** No. Todo lo de §4 usa lo que el 0.13.0 ya entrega:
  - pantalla inicial: `terminal.home_screen` ya viene en `getIdentity` y `getRecoveryState` (`administration/devices.go:52`,
    `routes.go:298`, `recovery.go:41`; esquema `Terminal` del `openapi.json`);
  - descartar cuenta vacía: `POST restaurant/accounts/{id}/discard` (exige `accounts.open`, `operations/handler.go:251`) ya lo
    usa la aplicación (`core/data/…/waiter/WaiterApi.kt:108-121`) y `discardable` ya viene en cada cuenta de
    `restaurant/state` (`operations/accounts_read.go:79`, `operations/recovery.go:31`);
  - precuenta desde el panel: las mismas rutas de la A2;
  - conexión tolerante: solo cliente.
    Por eso **no** hace falta capacidad nueva en `GET version` ni en `ServerCompatibility.check`
    (`core/domain/…/server/ServerCompatibility.kt:58-72`); `MIN_SERVER_VERSION` sigue en 0.13.0.
- **`min_client_versions.android`** se queda en `0.1.0`: el servidor 0.14.0 no rompe nada de la 0.2.0 (lo que cambia en Go
  es del escritorio y la validación de Terminales). Decisión propuesta, como A2-D6.
- Si la Fase D hace que Go rechace «Cocina» en un `android` (§0.6), una 0.3.0 contra un 0.13.0 puede leer `kitchen` o
  `cash`: la aplicación los trata como «Mesas» (prueba JVM).

### 0.2 Regla de textos (§1)

#### Inventario

Búsqueda en `desktop/src/**/*.{ts,tsx}` (sin `*.test.*`, `testing/` ni los `*TestData.ts`, que solo usan las pruebas) y
en **todos** los recursos de Android (`res/values*/` de los 7 módulos: además de `strings.xml`, `core/ui/…/strings_waiter.xml`,
`core/ui/…/strings_catalog.xml` y `feature/waiter/…/strings_catalog_sample.xml`) y en las cadenas de Kotlin que llegan a la
pantalla. Frases: «todavía no», «aún no», «etapa», «fase», «próximamente», «provisional», «por ahora», «más adelante», «en
desarrollo», «de prueba», «muestra», identificadores (UUID, `device_id`, códigos crudos).

**Provisionales o de desarrollo (quitar o reescribir):**

| Ruta:línea                                                                                           | Texto                                                                                                                                           | Propuesta                                                                                                                                                                                                          |
| ---------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `desktop/src/pages/DevicesPage.tsx:394`                                                              | «Solo se cuentan; todavía no se limita ninguna.»                                                                                                | Quitar la frase (las cifras se explican solas).                                                                                                                                                                    |
| `desktop/src/pages/restaurant/checkout/useCorrectionFlow.ts:24`                                      | «…Usa una regularización posterior (etapa 7) o, si corresponde, una devolución.»                                                                | «…Usa una regularización en Caja o, si corresponde, una devolución.»                                                                                                                                               |
| `desktop/src/pages/DevicesPage.tsx:84`                                                               | `unsupported: 'todavía no puede usarse para imprimir'`                                                                                          | `'no se puede usar para imprimir'`                                                                                                                                                                                 |
| `desktop/src/pages/payments/PaymentPoliciesForm.tsx:63` y `desktop/src/services/printProblems.ts:69` | «Esta impresora todavía no puede usarse para imprimir»                                                                                          | «Esta impresora no se puede usar para imprimir.»                                                                                                                                                                   |
| `desktop/src/pages/DevicesPage.tsx:332`                                                              | «Esta computadora todavía no recibe terminales de la red.»                                                                                      | «Esta computadora no recibe terminales de la red.»                                                                                                                                                                 |
| `desktop/src/pages/DevicesPage.tsx:338`                                                              | «…minutos: por ahora no se aceptan más solicitudes.»                                                                                            | «…minutos. Espera unos minutos para pedir otro.»                                                                                                                                                                   |
| `desktop/src/pages/restaurant/checkout/CheckoutPanel.tsx:473`                                        | «…por ahora solo acepta pesos.»                                                                                                                 | «Este turno no tiene tipo de cambio del dólar: solo acepta pesos.»                                                                                                                                                 |
| `desktop/src/pages/DevicesPage.tsx:874`                                                              | «Código: `<uuid>`» bajo el nombre de la terminal                                                                                                | Quitar de la tabla; en «Ver configuración», «Identificador» (§5.3, Fase D).                                                                                                                                        |
| `desktop/src/pages/AuditPage.tsx:191-192`                                                            | La clave cruda (`device.updated`) y `entity_type` debajo de la etiqueta                                                                         | Decisión propuesta: la clave solo si no hay etiqueta (`actionLabels`), y `entity_type` con nombre (es la sección de soporte del registro; si se prefiere conservarla, va a la lista de excepciones con su porqué). |
| `feature/waiter/…/strings_catalog_sample.xml` (todo) y `CatalogSample*.kt`                           | Pantalla de muestra de la Fase D de la A1 («Agregado (muestra)», «…el pedido de una mesa llega con el modo Mesero.»). Sin ruta en la navegación | Decisión propuesta: **quitar** la pantalla, sus textos y `CatalogSampleScreenTest` (el catálogo ya lo prueban `CatalogComponentsTest` y las de captura).                                                           |
| Permisos de Go (`kitchen/handler.go:42`)                                                             | «Confirmar recepción KDS» (sigla)                                                                                                               | Nombre nuevo «Confirmar que la cocina recibió la comanda» (Fase B, con los nombres visibles).                                                                                                                      |

**«Todavía no» / «aún no» que describen un estado real (no son provisionales, pero la comprobación los encuentra).**
Propuesta: reescribirlos (es trivial y deja la regla sin excepciones para estas dos frases):

| Ruta:línea                                                                                                                                                                                                                                                                                                                                                                                                     | Texto                                                                  | Reescritura                                     |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- | ----------------------------------------------- |
| `desktop/src/navigation/ConnectionLostLayer.tsx:76`                                                                                                                                                                                                                                                                                                                                                            | «Lo que capturaste y todavía no se guardó…»                            | «Lo que capturaste sin guardar…»                |
| `desktop/src/components/restaurant/TableMap.tsx:590`                                                                                                                                                                                                                                                                                                                                                           | «Esta área aún no tiene mesas.»                                        | «Esta área no tiene mesas.»                     |
| `desktop/src/pages/DevicesPage.tsx:528`, `pages/kitchen/PrintersAdmin.tsx:635`                                                                                                                                                                                                                                                                                                                                 | «…todavía no informó sus impresoras.»                                  | «…no ha informado sus impresoras.»              |
| `desktop/src/pages/UserPin.tsx:256`                                                                                                                                                                                                                                                                                                                                                                            | «Todavía no tienes PIN.»                                               | «No tienes PIN.»                                |
| `pages/restaurant/TablePanel.tsx:44`, `capture/CaptureBill.tsx:332`                                                                                                                                                                                                                                                                                                                                            | «La cuenta aún no tiene productos.»                                    | «La cuenta no tiene productos.»                 |
| `pages/restaurant/CancelAccount.tsx:75`                                                                                                                                                                                                                                                                                                                                                                        | «Lo que aún no se libera a preparación…»                               | «Lo que no se ha liberado a preparación…»       |
| `capture/CaptureBill.tsx:297`                                                                                                                                                                                                                                                                                                                                                                                  | «…productos nuevos que aún no se guardaron…»                           | «…productos nuevos sin guardar…»                |
| `checkout/PaymentMethods.tsx:123`, `catalog/CatalogPage.tsx:361`, `inventory/PurchaseFields.tsx:465`, `PurchaseOrderDetail.tsx:179`, `ProductionsView.tsx:446`, `RecipePickers.tsx:163`, `WarehousesView.tsx:328`, `SupplierSections.tsx:307`, `CategoriesAdmin.tsx:262`, `maintenance/BackupSection.tsx:257,325`, `kitchen/PrintQueue.tsx:467`, `services/serverService.ts:56`, `services/backups.ts:262,283` | «Aún no hay…», «Todavía no hay…»                                       | «No hay…»                                       |
| `CatalogPage.tsx:78`, `RecipeEditor.tsx:691`, `PurchaseOrdersView.tsx:467`, `SuppliersView.tsx:475`, `ItemEditor.tsx:480`                                                                                                                                                                                                                                                                                      | «…tiene cambios que aún no se guardan…»                                | «…tiene cambios sin guardar…»                   |
| `pages/setup/FindServerPage.tsx:340`                                                                                                                                                                                                                                                                                                                                                                           | «Esta terminal todavía no está conectada al servidor del restaurante.» | «Esta terminal no está conectada…»              |
| Android `feature/access/…/strings.xml:24-25`                                                                                                                                                                                                                                                                                                                                                                   | «%1$s todavía no está conectada…»                                      | «%1$s no está conectada…»                       |
| Android `feature/access/…/strings.xml:84`                                                                                                                                                                                                                                                                                                                                                                      | «Todavía no hay meseros con PIN…»                                      | «No hay meseros con PIN…»                       |
| Android `feature/access/…/strings.xml:131`                                                                                                                                                                                                                                                                                                                                                                     | «Estos pedidos todavía no se envían…»                                  | «Estos pedidos no se han enviado…»              |
| Android `feature/waiter/…/strings.xml:14`                                                                                                                                                                                                                                                                                                                                                                      | «Todavía no hay mesas guardadas…»                                      | «No hay mesas guardadas…»                       |
| Android `feature/takeaway/…/strings.xml:10`                                                                                                                                                                                                                                                                                                                                                                    | «Sin conexión: todavía no hay pedidos…»                                | «Sin conexión: no hay pedidos…»                 |
| Android `core/capture/…/strings.xml:32`                                                                                                                                                                                                                                                                                                                                                                        | «…lo que descartaste todavía está guardado…»                           | Se queda («todavía está» no es «todavía no»).   |
| Android `core/capture/…/strings.xml:141`                                                                                                                                                                                                                                                                                                                                                                       | «Este dispositivo todavía no tiene impresora asignada.»                | «Este dispositivo no tiene impresora asignada.» |
| Android `core/data/…/waiter/OrderQueueRepository.kt:434` (bitácora visible)                                                                                                                                                                                                                                                                                                                                    | «(descartado, sin quitar todavía de la cuenta)»                        | «(descartado, falta quitarlo de la cuenta)».    |

**Mensajes de Go que llegan a la pantalla** (el escritorio y Android muestran el `message` de Go tal cual; buscados en las
cadenas de `backend/internal`, sin pruebas ni comentarios):

| Ruta:línea                                                       | Texto                                                                                                                                                                 | Propuesta                                                                                                                                            |
| ---------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| `backend/internal/payments/cash_closing.go:435`                  | «El turno se cerró antes de la etapa 7 y no tiene arqueo registrado.»                                                                                                 | «Este turno no tiene arqueo registrado.»                                                                                                             |
| `backend/internal/kitchen/print_problems.go:44`                  | «Esta impresora todavía no puede usarse para imprimir.»                                                                                                               | «Esta impresora no se puede usar para imprimir.»                                                                                                     |
| `backend/internal/httpapi/router.go:131`                         | «Los eventos en tiempo real aún no están habilitados.»                                                                                                                | «Los avisos al momento no están disponibles en este servidor.»                                                                                       |
| `backend/internal/administration/pairing.go:240,341,537,678`     | «…todavía no está preparado…», «…todavía no puede emparejar terminales.», «La solicitud todavía no se aprueba…», «La terminal todavía no termina de pedir su código…» | «…no está preparado…», «…no puede emparejar terminales ahora.», «La solicitud no se ha aprobado…», «La terminal no ha terminado de pedir su código…» |
| `backend/internal/inventory/alerts.go:537`                       | «…aún no se ha aplicado ningún conteo…»                                                                                                                               | «…no se ha aplicado ningún conteo…»                                                                                                                  |
| `backend/internal/legacyimport/uploads.go:286`, `manager.go:206` | «El paquete todavía no termina de subirse.»                                                                                                                           | «El paquete no ha terminado de subirse.»                                                                                                             |

Legítimos en Go: «provisional» de los costos de inventario (`inventory/movements.go:38`, `reports/costs.go:42,51`: es el
término del costo estimado de inventario) y los mensajes de línea de órdenes o del registro del servicio
(`cmd/…/host_*.go`, `standalone/adopt.go:78`, `config/config.go:99`, `licensing/licensing.go:10`: no son interfaz).
Pruebas que afirman textos que cambian: `desktop/e2e/workflow-stage9g-terminals.spec.ts:124`,
`desktop/src/pages/stage9gPairing.test.tsx`, `desktop/src/pages/kitchen/printingStage9f.test.tsx`,
`backend/internal/operations/printing_stage9f_test.go`, `backend/internal/kitchen/paper_cut_test.go`.

**Falsos positivos legítimos** (funciones reales, quedan): «Quitar datos de prueba» y todo Mantenimiento › datos de prueba
(`MaintenancePage.tsx`, `AuditPage.tsx:36`, `navigation/sections.ts:258`), «Impresión de prueba» (`PrintersAdmin.tsx:480,485`),
«datos de muestra» y «Imprimir muestras» del diseñador (permisos `documents.*`), «Retirar de prueba» de la
previsualización de precios (`catalog/PreviewPage.tsx:125`; decisión propuesta: «Quitar producto», más claro), `'provisional'`
de `services/inventory.ts:44` (es un código de estado, no se muestra crudo), y en Android las plantillas de Go mostradas tal
cual. Los comentarios con «etapa»/«fase» (XML `<!-- -->`, `//`, `/* */`) no son interfaz. Datos técnicos permitidos solo en
soporte: `StatusPage.tsx` y `services/diagnostics.ts` (ya excluidos en `plainLanguage.test.tsx:27`), «Datos para soporte» de
Android y el código de soporte de `EntryMessage.SupportProblem` (`feature/access/…/PinEntryViewModel.kt:38`).

Observación sin cambio propuesto (no la pide §1): Android dice «Buscar el servidor» (`feature/access/…/strings.xml:7`) y
«servidor del restaurante» en `repair_confirm_text`; el resto dice «computadora principal». Si el coordinador quiere, Fase A
lo unifica.

#### Comprobación automática

- **Escritorio**: ampliar `desktop/src/pages/plainLanguage.test.tsx` (ya recorre las fuentes con `import.meta.glob`, quita
  comentarios y numera líneas) con un segundo `describe` «textos sin notas de desarrollo (A3, §1)» que use el mismo
  recorrido, excluya además `'!../**/*TestData.ts'` y busque, sin distinguir mayúsculas:
  `/todav[ií]a no/`, `/\baún no\b|\baun no\b/`, `/\betapa\b/`, `/\bfase [0-9A-Z]\b/`, `/pr[oó]ximamente/`, `/provisional/`
  (excepto la línea del tipo de `services/inventory.ts`), `/por ahora/`, `/m[aá]s adelante/`, `/en desarrollo/`,
  `/versi[oó]n de prueba/`, `/\(muestra\)/`. Además, para identificadores: `/Código: </`, `/\{[a-z_.]*\.id\}<\/span>/` en
  JSX visible. **Excepciones**: una lista `allowedDevelopmentWords: { file, pattern, reason }[]` dentro de la prueba (cada
  una con su porqué), para que agregar una exija escribir el motivo. Prueba nueva aparte no hace falta.
- **Android**: una prueba JVM en `app/src/test/…/PlainLanguageTest.kt` (como `ColorsOnlyInPaletteTest`, que ya recorre el
  proyecto desde `app`) que lea **todos** los `src/main/res/values*/*.xml` de todos los módulos (los `<string>`, `<item>` de
  `<plurals>` y `<string-array>`, sin los comentarios XML) y las cadenas literales de `src/main/kotlin/**/*.kt` (sin
  comentarios, sin las líneas de `require`, `check`, `error(`, `Log.` ni `Exception(`, que no se muestran), con la misma
  lista de frases y la misma forma de excepciones (`file + patrón + motivo`). Kotlin solo debería tener los pocos textos de
  `core:data` que se muestran (`OrderSender.kt:438-440`, `CaptureRepository.kt:1371-1372`, `OrderQueueRepository.kt:434,436,448`).
- **Go**: una prueba unitaria (`backend/internal/apicontract/plain_language_test.go` o un paquete propio) que recorra
  `backend/internal/**/*.go` sin `_test.go`, lea las cadenas con `go/parser` + `go/ast` (sin comentarios, exacto) y busque
  la misma lista, con excepciones `{archivo, patrón, motivo}` (los costos «provisional»).
- La regla escrita va a `CLAUDE.md` (§«Reglas permanentes desde la etapa A3») y a `android/CLAUDE.md` (regla 2 ampliada),
  nombrando las dos pruebas.

### 0.3 Conexión tolerante (§2)

#### Mapa del estado de conexión hoy

Hay **tres fuentes independientes** que nadie junta:

| Fuente                                                   | Dónde                                                                                                                                                                                                                               | Qué mide                                                                                                 | Quién la lee                                                                                                                                                                                                                                                                                     |
| -------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Red de Android** (`LinkState`)                         | `core/data/…/network/ConnectionMonitor.kt:26-61` + `core/domain/…/connection/DefaultNetworkTracker.kt`                                                                                                                              | Solo si hay red predeterminada sin bloquear (`registerDefaultNetworkCallback`); **nunca** si Go responde | `ShellViewModel.kt:26-34` → `ConnectionSummary` → `AdelTopBar` → `ConnectionIndicator.kt:60` («En línea»/«Sin conexión»); `AccessRepository.isOnline` (`AppContainer.kt:198`); `precheck.connected` (`WaiterModeContainer.kt:121-124`); `AccessViewModel.kt:67-73` (al volver la red, `recheck`) |
| **Estado del dispositivo** (`DeviceStatus.Ready.online`) | `core/data/…/access/AccessRepository.kt`: `markOffline()` 637-643 con **una sola falla** (`applyErrorLocked` 608-630 desde `checkLocked` 327-362 y `refreshRosterLocked` 366-382); vuelve a `true` solo con un `pin-users` correcto | `GET version` + `GET auth/pin-users`                                                                     | `roster()` 118-134 → `PinEntryScreen.kt:193-200` («Sin conexión con la computadora principal: solo pueden entrar los meseros que ya entraron antes…», `entry_offline`) y `entry_empty_offline`; **`enter()` 145-156: con `online=false` entra directo sin conexión, sin sesión de Go**           |
| **Lecturas de cada pantalla**                            | `RestaurantRepository.kt:134-161` (`RestaurantStatus.live`, false si `state` u `overview` fallan o no hay sesión)                                                                                                                   | Mesas y cuentas                                                                                          | `TablesScreen.kt:262-268` («Sin conexión: se ven las mesas de la última vez…»); `TakeawayScreen.kt:219-226`                                                                                                                                                                                      |

Otros consumidores: `activeWaiter.hasServerSession` («Entraste sin conexión», `core/ui/…/WaiterBadge.kt:111-113`; captura
`CaptureViewModel.kt:205-207`; «Entregado», `TakeawayViewModel.kt:105`; precuenta `CaptureViewModel.kt:223-227`); el canal de
eventos (`RealtimeClient`, `core/data/…/realtime/RealtimeClient.kt:123-165`, lectura muerta a los 10 s, `:348`) solo cambia
`RealtimeState`, que ninguna pantalla muestra; el canal de avisos (`ReadyAlertsChannel`/`ReadyAlertsClient`, 45 s,
`ReadyAlertsClient.kt:349`) cambia la notificación permanente «Sin conexión con la computadora principal. Reintentando…»
(`app/…/ready/ReadyAlertNotifications.kt:183`, `ReadyAlertsController.kt:226-236`); el catálogo (`CatalogUpdateTriggers.kt`,
cada 5 min, `:89`) dice «No se pudo actualizar el catálogo»; la cola (`OrderSender`) reintenta con WorkManager.

Temporizadores con red: comprobación del dispositivo **cada 60 s** en primer plano (`AccessViewModel.kt:76-85,139`), lista
de «¿Quién eres?» cada 30 s (`PinEntryScreen.kt:69-74,102`), catálogo cada 5 min, canal de eventos que Go cierra cada 30 s
(`administration/events.go:118`) y canal de avisos que Go cierra cada 5 min con `: ping` cada 15 s
(`administration/ready_alerts.go:41-45`).

**Cómo se contradicen**: la barra solo mira la red de Android (hay wifi → «En línea») mientras «¿Quién eres?» mira el
último resultado de `GET version`/`pin-users` y Mesas el de su última lectura. Una sola petición fallida deja el aviso de esa
pantalla hasta su siguiente lectura correcta (30–60 s en «¿Quién eres?»; en Mesas hasta el siguiente evento o «Actualizar»),
con la barra en «En línea». Y lo más grave: **con `online=false`, un mesero que escribe su PIN entra sin conexión** (sin
sesión de Go), así que Mesas dice «Sin conexión: se ven las mesas de la última vez…» y el menú «Entraste sin conexión» hasta
que vuelva a entrar, aunque la computadora principal responda.

#### Reproducción e investigación

Instrumentación solo en la copia (`spike`): `core/data/…/api/NetLog.kt` (un `EventListener` de OkHttp que registra cada
llamada: conexión nueva o reutilizada, **cuánto estuvo libre en el pool**, duración y causa de cada falla) conectado en
`PinnedHttpClient` y en el `RequestSent` de `ServerApi`; una línea en `AccessRepository.markOffline` con quién la llamó; y una
prueba instrumentada `IdleProbeOnGoTest` (empareja con el Go temporal como `AccessOnGoTest`, entra con un mesero, se queda sin
tocar nada y cada segundo registra la barra, los avisos de «¿Quién eres?» y de Mesas y los estados internos). Guion:
`$SP/spike-a3/probe.sh`; registros en `$SP/spike-a3/run/`; resumen con `$SP/spike-a3/scripts/analyze.py`.

**Corrida 1 (sin arreglo)**, en paralelo, 26 min cada emulador: 13 min con el mesero en Mesas (bloqueo por inactividad en
3600 s para que no saliera) y 13 min en «¿Quién eres?», avisos de platillos listos encendidos:

|                                                 | `Pixel_Tablet` (Android 17)                                                                                                 | `ADEL_A1_Phone_API33` (Android 13)        |
| ----------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------- |
| Llamadas a Go                                   | 96                                                                                                                          | 95                                        |
| Fallas de red                                   | **12** (más 1 «Socket closed» del canal de eventos que se cancela al cambiar de mesero: no es falla)                        | **13** (+1 igual)                         |
| Todas las fallas                                | `GET version` de la comprobación de 60 s, `IOException: unexpected end of stream … EOFException: \n not found` a los 2–8 ms | igual                                     |
| Conexión reutilizada con ≥ 60 s libre           | 12, **todas fallaron**                                                                                                      | 13, **todas fallaron**                    |
| Conexión reutilizada con < 60 s libre que falló | 0                                                                                                                           | 0                                         |
| `markOffline`                                   | 12, todas `applyErrorLocked < checkLocked`                                                                                  | 13                                        |
| Inactividad de la conexión que falló            | 60 007–60 015 ms (y una de 175 253 ms)                                                                                      | 60 006–60 016 ms (y 175 743 y 235 738 ms) |

Ejemplo (tableta):

```text
11:00:37.836 end GET version code=200 conn=99974226 reused=true idle=54806ms
11:00:37.842 end GET auth/pin-users code=200 conn=99974226 idle=2ms
11:01:37.850 acquire GET version conn=99974226 use=7 reused=true idle=60007ms
11:01:37.855 FAILED GET version … IOException: unexpected end of stream on https://10.0.2.2:8451/... cause=java.io.EOFException: \n not found
11:01:37.858 applyError Unreachable
11:01:37.859 markOffline desde applyErrorLocked:629 < checkLocked:355
11:02:37.864 FAILED GET version conn=99456755 reused=true idle=175253ms …
```

En la corrida 1 la pantalla no llegó a mostrar el aviso (0 de 742 s en cada fase) porque Mesas no lee
`DeviceStatus.online` y porque en una prueba instrumentada el reloj de Compose no avanza solo: el `LaunchedEffect` de 30 s de
«¿Quién eres?» no corría. Las corridas A–D lo corrigen (`compose.mainClock.advanceTimeBy` cada segundo).

**Corridas A–D** (reloj de Compose avanzando en tiempo real):

| Corrida                                             | Guion                                                                                                                           | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| --------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A · sin arreglo · `Pixel_Tablet`                    | 15 min en «¿Quién eres?» (sin mesero)                                                                                           | 84 llamadas, **0 fallas**: la lista de cada 30 s (`pin-users`) reutiliza una sola conexión con ≤ 31,6 s libre; el canal de avisos usa la suya y se reabre al instante. La pantalla de entrada **sola** no falla                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| B · **con el pool de 30 s** · `ADEL_A1_Phone_API33` | 8 min en Mesas + 8 min en «¿Quién eres?»                                                                                        | 90 llamadas, **0 fallas**, 0 `markOffline`; máxima inactividad reutilizada 25 s (las demás se descartaron y se abrieron nuevas: 15 conexiones nuevas en 16 min)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| C · sin arreglo · `Pixel_Tablet`                    | Bloqueo por inactividad de fábrica (120 s): Mesas → a los 2 min «¿Quién eres?» → 45 s después el mesero vuelve a entrar; 14 min | 133 llamadas, **12 fallas**, todas `GET` sobre una conexión con ≥ 60 s libre (`version` ×7, `auth/pin-users` ×4 —la primera lista de «¿Quién eres?» tras el bloqueo, con 120–121 s libre—, y al volver a entrar `restaurant/state` y `auth/me` con 168 s); 10 `markOffline`. **En 4 de los 5 bloqueos por inactividad «¿Quién eres?» mostró «Sin conexión con la computadora principal…» con la barra en «En línea», 32 s cada vez** (120 de 781 s). Una vez Mesas marcó su lectura `live=false` al volver a entrar (se corrigió 34 ms después con otra lectura) y la lectura de `auth/me` (nombre e impresora de la precuenta) falló sin aviso |
| D · **con el pool de 30 s** · `ADEL_A1_Phone_API33` | El mismo guion que C                                                                                                            | 139 llamadas, **0 fallas**, 0 `markOffline`, **0 de 782 s con aviso**, 5 bloqueos por inactividad y 5 entradas con sesión; máxima inactividad reutilizada 26,9 s (31 conexiones nuevas en 14 min)                                                                                                                                                                                                                                                                                                                                                                                                                                               |

**La captura del propietario, reproducida** (corrida C, tableta): en Mesas la comprobación de 60 s falla
(`11:44:48.106 FAILED GET version … idle=60007ms`); a los 6 s el bloqueo por inactividad lleva a «¿Quién eres?», cuya
primera lista reutiliza la otra conexión, libre desde hacía 121 s, y también falla
(`11:44:54.009 FAILED GET auth/pin-users … idle=121033ms`, `markOffline desde … refreshRosterLocked:379`): la pantalla
muestra «Sin conexión con la computadora principal: solo pueden entrar los meseros que ya entraron antes…» **con la barra
en «En línea»** (`barra=En línea avisoEntrada=true link=ONLINE`) durante unos 32 s, hasta la siguiente lista
(`11:45:25 … code=200`, conexión nueva). Se repitió en 4 de los 5 bloqueos por inactividad de la corrida: «varias veces»
con la tableta abierta y sin usar.

La entrada con PIN mientras el dispositivo está marcado sin conexión (entra sin sesión de Go, `AccessRepository.kt:145-156`)
se deduce del código; en las corridas el mesero volvió a entrar 45 s después del bloqueo, cuando ya había una lista
correcta, y siempre entró con sesión (no se reprodujo).

**Causa (con evidencia):**

1. **Go cierra una conexión keep-alive sin uso a los 60 s exactos** (`IdleTimeout: 60 * time.Second` en
   `lanlistener/lanlistener.go:237` y `cmd/ad-el-pos-server/main.go:265`). Comprobado desde la Mac contra la escucha de red
   del Go temporal (`$SP/spike-a3/scripts/idle_reuse.py`): la misma conexión TLS reutilizada a los 30, 58, 59,5 y 59,95 s
   responde 200; a los 60,05, 61 y 75 s, «Remote end closed connection without response». La escucha es HTTP/1.1 (sin
   ALPN: `openssl s_client -alpn h2,http/1.1` → «No ALPN negotiated»), así que no hay `GOAWAY` de HTTP/2 que avise.
2. **OkHttp 5.5.1 conserva una conexión libre hasta 5 minutos** (pool por omisión) y **no revisa una conexión reutilizada
   si la petición es `GET`**: en el bytecode, `RealRoutePlanner` hace `doExtensiveHealthChecks = method != "GET"` y
   `RealConnection.isHealthy` solo prueba el socket si la conexión estuvo libre ≥ 10 s **y** se pidió la revisión extensa
   (`javap` sobre `okhttp-android-5.5.1.aar`). Para un `GET`, OkHttp cuenta con reintentar si falla.
3. **La aplicación desactivó ese reintento** (`retryOnConnectionFailure(false)`, `PinnedHttpClient.kt:58`, a propósito para
   las escrituras con clave de operación), y nada repite la lectura: la falla llega como `AppError.Unreachable`.
4. **La comprobación periódica es de 60 s exactos** (`AccessViewModel.kt:139`): con un mesero en Mesas el canal de eventos
   ocupa una conexión y la comprobación usa la otra, que quedó libre justo 60 s + unos milisegundos → siempre muerta. Por
   eso falla cada vez que esa conexión está en el pool (12–13 veces en 26 min), con la Mac sin carga. Y al pasar de Mesas a
   «¿Quién eres?» (bloqueo por inactividad), la primera lista toma la conexión que la comprobación dejó libre más de 60 s
   (corrida C: 121 s) y falla también: es el aviso que ve el propietario.
5. **Una sola falla cambia el estado** (`markOffline`, `AccessRepository.kt:637`), y cada pantalla tiene su propio estado.

Descartado con evidencia: los tiempos de espera de OkHttp (las fallas tardan 2–8 ms; conexión 5 s y lectura 15 s,
`PinnedHttpClient.kt:14-15`, no intervienen), la reapertura de TLS (las 110 lecturas que abrieron conexión nueva, con su intercambio TLS, tardaron 10–554 ms, mediana
55 ms, y ninguna falló), el `ReadTimeout` de 15 s de Go sobre los canales (el canal de eventos medido con `curl` dura 30,01 s con 41
mensajes; Go 1.27.3 quita el plazo de lectura al empezar la lectura de fondo, `net/http/server.go:741`), el canal de avisos (6 reaperturas por corrida, todas
inmediatas sobre su propia conexión, ninguna falla), el modo de inactividad del emulador (`svc power stayon true`, la pantalla
siguió encendida) y `ConnectivityManager` (la red nunca se reportó perdida: `link=ONLINE` todo el tiempo).

No verificado: un equipo físico y una wifi real (pérdidas de paquetes, cambio de punto de acceso). La tolerancia de abajo
cubre eso aunque la causa principal esté corregida. Observación sin verificar: el escritorio usa `reqwest` con su pool por
omisión (`desktop/src-tauri/src/main.rs:213`); no se investigó si le pasa algo parecido (fuera del alcance de §2).

#### Diseño propuesto (Fase A, Android)

**Corregir la causa** (`core:data`, `PinnedHttpClient`):

- `connectionPool(ConnectionPool(maxIdleConnections = 5, keepAliveDuration = 30 s))`: OkHttp descarta una conexión libre a
  los 30 s, antes de que Go la cierre a los 60 s (margen para la latencia y el reloj). Probado en las corridas B y D: 0 fallas.
- Además, **una lectura (`GET`) que falla sin respuesta en una conexión reutilizada se repite una vez enseguida** con una
  conexión nueva (en `ServerApi.execute`/`WaiterApi`, solo `GET`; las escrituras siguen con su `operation_id` y sus reglas).
  Es la red de seguridad si Go o un equipo intermedio cierra antes. `retryOnConnectionFailure` sigue en `false`.
- Go no cambia: subir su `IdleTimeout` solo movería la ventana (OkHttp guarda 5 min).

**Un solo estado de conexión** (`ServerLink`):

- Lógica pura en `core:domain/connection/ServerLinkTracker` (JVM, con reloj inyectado) y su implementación en `core:data`
  (`ServerLinkMonitor`), creada una vez en `AppContainer` (como `connectionMonitor`). Estados: `ONLINE` («En línea»),
  `OFFLINE` («Sin conexión») y, internamente, «dudando» (se sigue mostrando «En línea»).
- **Lo alimentan**: (a) la red de Android (`ConnectionMonitor`): sin red → `OFFLINE` **al instante**; red de vuelta → una
  sonda enseguida; (b) **cada respuesta de Go**: un `EventListener`/interceptor único en `PinnedHttpClient` (todas las rutas
  pasan por ahí: `ServerApi`, `WaiterApi`, `RealtimeClient`, `ReadyAlertsClient`, catálogo e imágenes). Cualquier respuesta
  HTTP (también 4xx/5xx: Go respondió) o mensaje del canal es un **éxito**; una `IOException` sin respuesta es una
  **falla**; una cancelación propia (cambio de mesero, salir de primer plano) y el cierre normal de un canal no cuentan.
- **Tolerancia**: pasa a `OFFLINE` solo con **≥ 3 fallas seguidas y ≥ 20 s desde la primera de la racha sin ningún éxito**;
  mientras duda, sondea `GET health/live` cada 5 s para confirmarlo rápido. Vuelve a `ONLINE` con el **primer éxito**.
  Porqué 20 s: una falla aislada (conexión cerrada, reintento) se resuelve en milisegundos; un cambio de punto de acceso o
  una renovación de DHCP tarda unos segundos; con 3 sondas cada 5 s más el plazo de conexión de 5 s, 20 s distingue
  una caída real sin dejar al mesero mucho tiempo creyendo que hay conexión. Se documenta como constante con su porqué en
  `ServerLinkTracker` y en `android/CLAUDE.md` (regla 13).
- **Lo leen todos**: `ShellViewModel` (barra) deja de leer la red sola y lee `ServerLink` + pedidos por enviar;
  `AccessRepository` usa `ServerLink` en lugar de su `online` propio (`roster()`, `enter()`: entra directo sin conexión
  **solo** con `OFFLINE`, nunca por una falla aislada); Mesas y Para llevar distinguen «sin conexión» (de `ServerLink`) de
  «lectura vieja» (su `live`, que deja de decir «Sin conexión» si `ServerLink` está `ONLINE` y en su lugar reintenta);
  precuenta y «Entregado» usan `ServerLink` en vez de `precheck.connected`; la notificación del canal de avisos dice «Sin
  conexión…» solo con `OFFLINE` (su protocolo no cambia: solo informa sus éxitos y fallas y lee el estado común).
- **Revisión según el uso**: con un mesero activo y el canal de eventos abierto, **el canal es la revisión** (un mensaje
  cada 750 ms): no hace falta otra petición. Sin canal (en «¿Quién eres?» o sin sesión) o con el dispositivo inactivo, la
  revisión es cada **60 s** y se salta si hubo otro éxito en ese lapso (la lista de «¿Quién eres?» cuenta). `GET version`
  (capacidades) se espacia a 5 min y al volver a primer plano o la red; `pin-users` sigue al abrir «¿Quién eres?» y cada
  60 s mientras está a la vista (hoy 30 s).
- **Reintento en el momento**: al tocar algo que necesita a Go (entrar con PIN, abrir una mesa o una cuenta, «Actualizar»,
  imprimir la precuenta, «Entregado», «Descartar cuenta vacía»), si `ServerLink` no está `OFFLINE` o la falla es nueva, se
  intenta (y un `GET` se repite una vez) **antes** de mostrar el error; con `OFFLINE` pero con red de Android, se sondea
  primero. Las escrituras con `operation_id` conservan su regla actual (repetir con la misma clave).
- **No cambia**: el protocolo del canal de avisos (A2: cursor, `Last-Event-ID`, espera creciente, A2-D12) ni el del canal de
  eventos; la regla de la cola sin conexión (regla 18, `WorkManager` sin restricción de red, `retryNow`); la entrada sin
  conexión (A1-D1, A1-D16).

**Pruebas**: JVM `ServerLinkTrackerTest` con tiempo virtual (una falla aislada no cambia; tres en 20 s sí; dos en 25 s no;
el primer éxito vuelve; sin red al instante; la sonda cada 5 s mientras duda; revisión cada 60 s inactivo y ninguna con el
canal abierto); `PinnedHttpClientTest` con MockWebServer (una conexión libre más que el pool no se reutiliza; un `GET` sobre
una conexión cerrada por el servidor se repite una vez y un `POST` no); `AccessRepository` (con una falla aislada la entrada
con PIN sigue siendo con conexión); instrumentada con `FakeAccess` (barra y avisos siempre iguales); contra el Go temporal,
una `ConnectionOnGoTest` (como el spike, con el reloj de Compose avanzando) de 10 min en Mesas y en «¿Quién eres?» sin
ninguna falla; y en la Fase E la hora sin uso de §6 en `Pixel_Tablet`, con el bloqueo por inactividad de fábrica (120 s) y
el mesero volviendo a entrar cada vez, sin avisos falsos.

### 0.4 Permisos agrupados y roles de fábrica (§3)

#### Hoy

- **Roles**: los crea `ensureFoundations` (`administration/permissions.go:78-109`) solo en el asistente (`setup.go:88`) y al
  aprovisionar soporte (`setup.go:159`), con `ON CONFLICT DO NOTHING`: `administrator` «Administrador», `supervisor`,
  `cashier` «Cajero», `waiter` «Mesero», `kitchen` «Cocina», `inventory` «Inventarios», `finance` «Finanzas» y `support`
  «Soporte reservado» (`reserved = true`, no se lista: `roles.go:15`). **Cocina e Inventarios ya existen** en toda
  instalación. Un rol creado por el usuario tiene `code` nulo (`roles.go:70`); la pantalla ya lo distingue («Inicial» /
  «Personalizado», `RolesPage.tsx:145`).
- **Permisos de fábrica hoy** (`setup.go:94-118`): el administrador todos; supervisor `users.read`, `devices.read`,
  `sessions.read`, `registers.read`; cajero y finanzas `registers.read`; mesero `waiter_devices.use`; cocina e inventarios,
  ninguno.
- **Catálogo**: 156 permisos en memoria en Go (`administration.Permissions`, `permissions.go:23-66`, más los de cada
  módulo), `{key, description}`; la tabla `app.permissions(key, description)` (`0002…sql:10-13`) solo guarda la clave para
  las llaves foráneas. **No hay módulo, grupo, dependencias ni descripción corta**: `description` es en realidad el nombre
  visible («Consultar cuentas e importes»). `GET permissions` (exige `permissions.read`) devuelve la lista en memoria
  (`routes.go:204-211`).
- **Editar un rol**: `POST roles` y `PATCH roles/{id}` con `{name, permissions, discount_limit}` (`roles.go:17-133`): exige
  `roles.manage` y `permissions.manage`; rechaza claves desconocidas; **«No puedes conceder a un rol permisos que tú no
  tienes»** (`requireWithin`, :51) y «No puedes editar un rol con permisos que tú no tienes» (:100); nadie edita su propio
  rol (:93); los reservados no (:89); `lockAdministration` y `preserveAdministration` (nunca quedarse sin administrador);
  reemplaza el conjunto completo; auditoría `role.created`/`role.updated` con antes y después (:132). Los permisos se leen
  de PostgreSQL en cada petición (no hay caché en la sesión).
- **Pantalla**: `desktop/src/pages/RolesPage.tsx` con `components/PermissionEditor.tsx` (una cuadrícula plana de 156
  casillas con `description`); `roles.test.tsx` (2 casos del límite de descuento). Las rutas de roles y permisos **no están
  en el contrato v1** (38 rutas en `openapi.json`): son del escritorio, por el puente de Rust
  (`desktop/src-tauri/src/api.rs:101-102,668`); una ruta nueva exige agregarla ahí con su prueba de `cargo`.

#### Conjuntos de fábrica propuestos

Cada conjunto está **cerrado bajo las dependencias** de abajo (comprobado con `$SP/spike-a3/scripts/roles.py`).

- **Mesero (`waiter`, 14)** — la tabla de `docs/ANDROID_INSTALAR.md` §4 con Para llevar e imprimir precuenta, sin
  «Modificar» ni «Cancelar unidades»: `waiter_devices.use`, `restaurant.areas.read`, `restaurant.tables.read`,
  `accounts.read`, `accounts.open`, `orders.capture`, `orders.confirm`, `catalog.read`, `catalog.prices.read`,
  `events.read`, `takeaway.read`, `takeaway.manage`, `takeaway.deliver`, `payments.tickets.print`. Son 16 de
  `WaiterPermissions` (`waiter_scope.go:14-49`) menos `orders.edit` y `orders.cancel`. «Entregado» (`takeaway.deliver`) va
  marcado porque §3.1 solo exceptúa los dos; Go sigue impidiendo entregar sin pagar (`BALANCE_PENDING`).
- **Cajero (`cashier`, 36)** — Caja, cobro y toma de pedidos en el mostrador: lo de mesas y captura del mesero (sin
  `waiter_devices.use`), Para llevar con «Entregado», `kitchen.states.read` y `kitchen.stations.read` (ver preparación),
  cobro `payments.read`, `collect`, `collect_partial`, `collect_selection`, `tips.record`, `discounts.apply` (dentro del
  límite del rol, que queda en 0 % como hoy), `charges.create`, `tickets.print`, `tickets.reprint`; caja
  `cash.shifts.read`, `shifts.open`, `counts.perform`, `shifts.close`, `partial_cuts.create`, `income.record`,
  `withdrawals.record`, `expenses.record`, `movements.read`, `reports.print`, `drawer.open`, `payments.usd_rate.manage`
  (tipo de cambio del turno) y `registers.read` (hoy ya lo tiene). Sin autorizaciones (retiros, correcciones, devoluciones,
  descuentos mayores), que son del supervisor.
- **Cocina (`kitchen`, 8)** — la pantalla de preparación (sección Cocina exige `kitchen.states.read`,
  `navigation/sections.ts:96`; rutas `kitchen/handler.go:158-204`): `kitchen.stations.read`, `tickets.read`,
  `states.read`, `states.manage`, `kds.ack`, `history.read`, `reprint`, `events.read`. Sin `kitchen.stations.all` (cada
  terminal muestra sus estaciones) ni la configuración.
- **Supervisor (`supervisor`, 89)** — todo lo del cajero y cocina más: `waiter_devices.use`, `orders.edit`, `orders.cancel`,
  `orders.history.read`, cuentas (`manage`, `cancel`, `split`, `merge`, `transfer`, `move`, `transfer_items`),
  autorizaciones de cobro (`discounts.authorize`, `courtesy`, `charges.manage`, `corrections`, `refunds.execute`,
  `refunds.authorize`) y de caja (`outflows.authorize`, `corrections.record`, `corrections.authorize`, `shifts.read_all`,
  `closing.cancel_any`, `history.read`, `expected.read`, `closures.review`, `reports.reprint`), cocina (`release`,
  `release_early`, `queue.read`, `queue.resolve`, `queue.retry`, `printers.test`), disponibilidad del catálogo
  (`availability.manage`, `availability.override`, `sold_out.manage`), consulta de personal y terminales (`users.read`,
  `devices.read`, `sessions.read`, `sessions.manage`), `audit.read` y reportes de operación (`reports.read`, `sales`,
  `payments`, `discounts`, `cash`, `employees`, `export.pdf`, `export.excel`). Sin administrar catálogo, personal ni sistema.
- **Inventarios (`inventory`, 20)** — `inventory.read`, `items.manage`, `stock.read`, `movements.read`, `costs.read`,
  `recipes.manage`, `subrecipes.manage`, `production.record`, `waste.record`, `suppliers.manage`, `purchases.create`,
  `purchases.receive`, `supplier_returns.record`, `adjustments.record`, `counts.perform`, `transfers.record`,
  `catalog.read` (las recetas son de productos), `reports.read`, `reports.inventory.read`, `reports.purchases.read`. Sin
  autorizar (ajustes, conteos, compras), costos de referencia, valuación ni parámetros.
- **Finanzas (`finance`, 28)** — todos los `reports.*` (10), `payments.read`, `accounts.read`, `orders.history.read`, caja
  de consulta (`shifts.read`, `shifts.read_all`, `history.read`, `expected.read`, `movements.read`, `closures.review`),
  `registers.read`, inventario de consulta y costos (`inventory.read`, `stock.read`, `costs.read`, `valuation.read`,
  `movements.read`), `inventory.purchases.authorize`, `inventory.purchases.link_cash` y `audit.read`.
- **Administrador**: todos, como hoy (`setup.go:95-99`).

La tabla de cada rol con la clave, el nombre visible y el porqué está en el **anexo** al final (generada de
`$SP/spike-a3/roles.json`). Decisión propuesta: el **revisor independiente** de §0 revisa estos
conjuntos y el restablecimiento antes de cerrar la Fase B.

**Instalación existente**: la `0027` no toca roles (§3.1). Los conjuntos se aplican **solo en el asistente** (`setup.go`,
donde hoy se conceden los de fábrica; los roles nacen después de las migraciones) y con «Restablecer permisos
predeterminados» rol por rol. Cocina e Inventarios no hay que crearlos.

#### Agrupación (cada permiso en exactamente un grupo; 156 en total)

| Grupo                | N.º | Permisos                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| -------------------- | --- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Mesas y cuentas      | 16  | `waiter_devices.use`, `events.read`, `restaurant.areas.read`, `restaurant.areas.manage`, `restaurant.tables.read`, `restaurant.tables.manage`, `accounts.read`, `accounts.open`, `accounts.manage`, `accounts.cancel`, `accounts.split`, `accounts.merge`, `accounts.transfer`, `accounts.move`, `accounts.transfer_items`, `orders.history.read`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| Captura y pedidos    | 5   | `orders.capture`, `orders.confirm`, `orders.edit`, `orders.cancel`, `orders.price_list.manage`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Para llevar          | 4   | `restaurant.settings.manage`, `takeaway.read`, `takeaway.manage`, `takeaway.deliver`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| Cocina e impresión   | 19  | `kitchen.stations.read`, `kitchen.stations.manage`, `kitchen.stations.all`, `kitchen.routing.manage`, `kitchen.printers.manage`, `kitchen.printers.test`, `kitchen.formats.manage`, `kitchen.timing.manage`, `kitchen.release`, `kitchen.release_early`, `kitchen.tickets.read`, `kitchen.states.read`, `kitchen.states.manage`, `kitchen.queue.read`, `kitchen.queue.resolve`, `kitchen.queue.retry`, `kitchen.reprint`, `kitchen.history.read`, `kitchen.kds.ack`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| Caja y cobros        | 40  | `payments.read`, `payments.collect`, `payments.collect_partial`, `payments.collect_selection`, `payments.methods.manage`, `payments.settings.manage`, `payments.tax.manage`, `payments.discounts.apply`, `payments.discounts.authorize`, `payments.courtesy`, `payments.charges.create`, `payments.charges.manage`, `payments.tips.record`, `payments.corrections`, `payments.refunds.execute`, `payments.refunds.authorize`, `payments.tickets.print`, `payments.tickets.reprint`, `cash.shifts.open`, `cash.shifts.read`, `payments.usd_rate.manage`, `cash.shifts.read_all`, `cash.income.record`, `cash.withdrawals.record`, `cash.expenses.record`, `cash.outflows.authorize`, `cash.corrections.record`, `cash.corrections.authorize`, `cash.movements.read`, `cash.partial_cuts.create`, `cash.counts.perform`, `cash.shifts.close`, `cash.closing.cancel_any`, `cash.history.read`, `cash.expected.read`, `cash.closures.review`, `cash.reports.print`, `cash.reports.reprint`, `cash.drawer.open`, `cash.settings.manage` |
| Catálogo             | 16  | `catalog.read`, `catalog.prices.read`, `catalog.prices.manage`, `catalog.products.manage`, `catalog.products.state`, `catalog.categories.manage`, `catalog.ingredients.manage`, `catalog.complements.manage`, `catalog.groups.manage`, `catalog.combos.manage`, `catalog.price_lists.manage`, `catalog.promotions.manage`, `catalog.availability.manage`, `catalog.availability.override`, `catalog.images.manage`, `catalog.sold_out.manage`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| Inventario           | 24  | `inventory.read`, `inventory.items.manage`, `inventory.costs.read`, `inventory.costs.manage`, `inventory.stock.read`, `inventory.warehouses.manage`, `inventory.movements.read`, `inventory.adjustments.authorize`, `inventory.valuation.read`, `inventory.settings.manage`, `inventory.recipes.manage`, `inventory.subrecipes.manage`, `inventory.production.record`, `inventory.waste.record`, `inventory.suppliers.manage`, `inventory.purchases.create`, `inventory.purchases.authorize`, `inventory.purchases.receive`, `inventory.supplier_returns.record`, `inventory.purchases.link_cash`, `inventory.adjustments.record`, `inventory.counts.perform`, `inventory.counts.approve`, `inventory.transfers.record`                                                                                                                                                                                                                                                                                                            |
| Reportes             | 10  | `reports.read`, `reports.sales.read`, `reports.payments.read`, `reports.discounts.read`, `reports.cash.read`, `reports.inventory.read`, `reports.purchases.read`, `reports.employees.read`, `reports.export.pdf`, `reports.export.excel`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Personal             | 8   | `users.read`, `users.manage`, `roles.read`, `roles.manage`, `permissions.read`, `permissions.manage`, `sessions.read`, `sessions.manage`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Terminales y sistema | 14  | `devices.read`, `devices.manage`, `settings.read`, `settings.manage`, `settings.features.manage`, `registers.read`, `registers.manage`, `audit.read`, `documents.templates.read`, `documents.templates.manage`, `documents.preview.real`, `documents.samples.print`, `documents.identity.manage`, `documents.history.render_current`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |

Decisiones propuestas de la agrupación: `waiter_devices.use` y `events.read` en «Mesas y cuentas» (son lo que el mesero
busca ahí); `restaurant.settings.manage` (umbrales del tablero) en «Para llevar»; `payments.tickets.*` en «Caja y
cobros»; `documents.*` en «Terminales y sistema» (Administración › Documentos); `catalog.sold_out.manage` en «Catálogo».

#### Descripciones cortas

Hoy no existen: `description` es el nombre visible. Propuesta: **todo en Go, en un solo archivo nuevo**
`backend/internal/administration/permission_catalog.go` con, por clave, el **nombre visible** (el `description` actual,
corregido donde haga falta, por ejemplo «KDS»), la **descripción corta** en lenguaje claro (una frase: «Ver las cuentas
abiertas y sus importes»), el **grupo** y **lo que necesita**. Los módulos siguen registrando sus claves; el catálogo
completa lo demás y una prueba unitaria exige que las 156 claves tengan nombre, descripción, grupo y dependencias válidas
(sin ciclos ni claves desconocidas). Sin migración: `app.permissions` sigue solo para las llaves foráneas (la interfaz nunca
la lee). La Fase B redacta las 156 descripciones.

#### Dependencias (deducidas de Go)

Reglas (138 dependencias en 122 permisos; lista completa en `$SP/spike-a3/roles.json`):

- Toda ruta `restaurant/accounts/*` y `payments/accounts/*` exige `accounts.read` (`operations/handler.go:112`,
  `payments/handler.go:146-155`): `accounts.*`, `orders.*`, `payments.read`, `payments.tickets.print` y `takeaway.read`
  → `accounts.read`.
- Capturar, confirmar y modificar exigen catálogo y precios (`operations/handler.go:132,148-149`): `orders.capture`,
  `orders.confirm`, `orders.edit` → `catalog.read`, `catalog.prices.read`; `orders.confirm` → `orders.capture` (el pedido
  sin conexión exige ambos, `handler.go:107`); `orders.cancel` → `orders.edit` («para bajar la cantidad hacen falta los
  dos», `ANDROID_INSTALAR.md` §4).
- `accounts.open` → `restaurant.tables.read` → `restaurant.areas.read`; `restaurant.*.manage` → su `.read`.
- Para llevar: una cuenta para llevar se lee con `takeaway.read` y se escribe con `takeaway.manage`
  (`operations/queries.go:139-147`), y abrirla exige `accounts.open` + `takeaway.manage` (`handler.go:124-127`,
  `offline_orders.go`): `takeaway.manage` → `takeaway.read`, `accounts.open`, `orders.capture`; `takeaway.deliver` →
  `takeaway.read` (`payments/collect.go:482`).
- Cobro: `payments.collect`, `discounts.apply`, `courtesy`, `charges.*`, `corrections`, `refunds.execute`,
  `tickets.reprint` → `payments.read`; `collect_partial`, `collect_selection`, `tips.record` → `payments.collect`
  (`payments/collect.go:288-298`); los `*.authorize` → su acción.
- Caja: todo `cash.*` y `payments.usd_rate.manage` → `cash.shifts.read` (la sección Caja, `sections.ts:104`);
  `cash.shifts.close` → `cash.counts.perform`.
- Todo `inventory.*` → `inventory.read` (`inventory/handler.go:177`); todo `reports.*` → `reports.read`
  (`reports/handler.go:61`, `registry.go:183`); todo `catalog.*` → `catalog.read`; `catalog.prices.manage` →
  `catalog.prices.read`.
- Cocina: `states.manage` → `states.read`; `kds.ack` → `tickets.read`; `tickets.read` y `states.read` →
  `stations.read`; `queue.resolve`/`retry` → `queue.read`; `stations.manage` → `stations.read`; `release_early` → `release`.
- Administración: `*.manage` → su `.read`; `roles.manage` → `roles.read`, `permissions.manage` (`roles.go:24,42`);
  `permissions.manage` → `permissions.read`.

**Único lugar**: el mismo `permission_catalog.go`. **Cómo lo lee la interfaz**: ruta nueva `GET permissions/catalog`
(`permissions.read`) que devuelve `{groups: [{code, name, permissions}], permissions: [{key, name, description, group,
requires}], capabilities: [{code, name, permissions}], factory_roles: [code]}`; `GET permissions` no cambia (escritorios
anteriores). Capacidad nueva `permission-groups-v1` en `GET version` para que el escritorio 0.14.0 muestre la pantalla nueva
solo con un servidor que la tenga (con uno anterior, la cuadrícula de hoy). Go **no** impone las dependencias al guardar
(sería un cambio de comportamiento para roles existentes): solo las informa; cada ruta sigue validando sus permisos.

#### Capacidades de la vista sencilla

Se definen en el mismo catálogo de Go, junto a las dependencias (la interfaz no tiene listas propias):

| Capacidad             | Permisos (más sus dependencias)                                                                                                                                                                                                                                                                                                                                                                                                    |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Atender mesas         | `waiter_devices.use`, `restaurant.areas.read`, `restaurant.tables.read`, `accounts.read`, `accounts.open`, `orders.capture`, `orders.confirm`, `catalog.read`, `catalog.prices.read`, `events.read`, `payments.tickets.print`                                                                                                                                                                                                      |
| Para llevar           | `takeaway.read`, `takeaway.manage`, `takeaway.deliver`                                                                                                                                                                                                                                                                                                                                                                             |
| Cobrar                | `payments.read`, `payments.collect`, `payments.collect_partial`, `payments.collect_selection`, `payments.tips.record`, `payments.discounts.apply`, `payments.charges.create`, `payments.tickets.print`, `payments.tickets.reprint`                                                                                                                                                                                                 |
| Manejar caja          | `cash.shifts.read`, `cash.shifts.open`, `cash.counts.perform`, `cash.shifts.close`, `cash.partial_cuts.create`, `cash.income.record`, `cash.withdrawals.record`, `cash.expenses.record`, `cash.movements.read`, `cash.reports.print`, `cash.drawer.open`, `payments.usd_rate.manage`, `registers.read`                                                                                                                             |
| Cocina                | los 8 del rol Cocina                                                                                                                                                                                                                                                                                                                                                                                                               |
| Ver reportes          | los 10 `reports.*`                                                                                                                                                                                                                                                                                                                                                                                                                 |
| Administrar catálogo  | los 16 `catalog.*`                                                                                                                                                                                                                                                                                                                                                                                                                 |
| Administrar personal  | los 8 de «Personal»                                                                                                                                                                                                                                                                                                                                                                                                                |
| Configurar el sistema | `settings.*`, `devices.*`, `registers.manage`, `audit.read`, `documents.*`, la configuración de cocina (`kitchen.stations.manage`, `routing.manage`, `printers.manage`, `formats.manage`, `timing.manage`), `restaurant.areas.manage`, `restaurant.tables.manage`, `restaurant.settings.manage`, `payments.methods.manage`, `payments.settings.manage`, `payments.tax.manage`, `cash.settings.manage`, `inventory.settings.manage` |

Decisión propuesta: agregar **«Manejar inventario»** (los 20 del rol Inventarios sin reportes) y **«Autorizar y
corregir»** (las autorizaciones del supervisor); §3.2 dice «opciones como…», y sin ellas Inventarios y Supervisor no se
pueden armar desde la vista sencilla. Parcial: el rol tiene algunos pero no todos los permisos de la capacidad (los cuenta Go
o la interfaz con la lista de Go).

#### «Restablecer permisos predeterminados»

- **Rutas** (escritorio, no v1): `GET roles/{id}/defaults` → `{role, factory: [...], add: [...], remove: [...]}` (vista
  previa) y `POST roles/{id}/defaults` con `{operation_id, expected: [...]}` (los permisos que mostró la vista previa; si el
  rol cambió mientras tanto, 409 `ROLE_CHANGED`). Ambas exigen `roles.manage`, `permissions.manage` y `permissions.read`,
  como editar. Agregar las dos al puente de Rust (`api.rs`).
- **Quién**: quien puede editar ese rol hoy, con **las mismas reglas que `PATCH roles/{id}`**: no su propio rol, no un rol
  reservado, `requireWithin` sobre los permisos **actuales y los de fábrica** (nadie concede así lo que no tiene),
  `lockAdministration` + `preserveAdministration`, y auditoría `role.defaults_restored` con antes y después en el Registro
  de movimientos (`AuditPage.tsx` con su etiqueta).
- **De fábrica o del usuario**: por `app.roles.code` (`waiter`, `cashier`, `kitchen`, `supervisor`, `inventory`,
  `finance`); un rol con `code` nulo no tiene botón y la ruta responde 404/400. **Administrador**: decisión propuesta, sin
  botón (siempre tiene todo y nadie edita su propio rol); el soporte reservado tampoco.
- Solo cambia `role_permissions`; no toca el límite de descuento ni las excepciones por usuario (`user_permissions`), y se
  dice en la confirmación.
- **Para el revisor independiente**: escalada (que el conjunto de fábrica nunca dé algo que el actor no tiene; prueba con
  un actor sin `payments.refunds.authorize` que restablece Supervisor → 403); el rol propio y el último administrador; roles
  en uso (las sesiones abiertas no guardan permisos: se leen en cada petición, así que el cambio vale en la siguiente;
  Android guarda `waiter_permissions` solo para mostrar y Go rechaza igual); carrera entre vista previa y confirmación
  (`expected`); que la lista de fábrica de una versión nueva no cambie roles sin que alguien toque el botón; la auditoría.
  **El servidor sigue validando cada permiso en cada ruta.**

### 0.5 Ajustes de Android (§4)

| #   | Hoy (`ruta:línea`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | Cambio                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | Riesgo                                                                                                                                         |
| --- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | `AdelTopBar` no tiene icono de navegación (`core/ui/…/components/AdelTopBar.kt:25-51`); Atrás es solo el del sistema (`BackHandler` de `AdaptivePanes` y la pila de navigation-compose, `navigation/AdelPosApp.kt:163-167`). Al salir de una captura, la captura se queda guardada con su mesero sin decir nada (`CaptureViewModel.kt:520-523`, `closeIfIdle` solo si está vacía)                                                                                                                                                                                                                               | `AdelTopBar(onBack: (() -> Unit)?)` con la flecha (48 dp, `AdelIcons`) en Captura, Pendientes, Revisión y la cuenta/mesa a un toque; la flecha llama a `onBackPressedDispatcher.onBackPressed()` para pasar por los mismos `BackHandler`. Al volver a Mesas o Para llevar desde una captura con productos sin enviar, un aviso breve: «Lo capturado en Mesa 4 quedó guardado sin enviar a cocina. Lo encuentras en la mesa o en Pendientes.»                                                                                                                                                                                                                                                                                              | Doble Atrás (flecha + panel); la flecha no debe saltarse el `BackHandler` del panel; atrás predictivo de Android 16                            |
| 2   | Título fijo «Captura» (`core/capture/…/strings.xml:6`, `CaptureScreen.kt:255`, `AdelPosApp.kt:191`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | `captureTitle(capture)`: «Mesa 4» (nombre de la mesa) o «Para llevar #12» (folio de Go; sin folio todavía, «Para llevar · Nombre» o «Nuevo pedido para llevar», reutilizando `Takeaway.kt`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | El título necesita la captura del `ViewModel` (hoy se calcula en la raíz)                                                                      |
| 3   | `CatalogSearchField` con «Buscar producto por nombre o código» (`core/ui/…/strings_catalog.xml:5`), que se parte en dos renglones salvo en la forma compacta (`CatalogBrowser.kt:136-180`)                                                                                                                                                                                                                                                                                                                                                                                                                      | Etiqueta «Buscar producto», siempre `maxLines = 1`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | Mínimo                                                                                                                                         |
| 4   | «Ver la cuenta (%1$d)» con el conteo de lo propio (`core/capture/…/strings.xml:7`, `CaptureScreen.kt:91-93`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | «Ver la cuenta» y, con productos sin enviar, «Ver la cuenta · 2 por enviar» (plurales)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | `AdaptivePanes.openSecondaryLabel` es un texto: solo cambia el texto                                                                           |
| 5   | Título «Entrar» (`feature/access/…/strings.xml:8`), «¿Quién eres?» (:81), «Toca tu nombre y escribe tu PIN.» (:82), «Entrar» (:89, también en `ReenterPinDialog.kt:141`), «Elegir otro nombre» (:90). Sin logotipo                                                                                                                                                                                                                                                                                                                                                                                              | Los textos nuevos de §4.5 y el logotipo en la barra junto al título solo en las pantallas de entrada: PNG claro y oscuro de `assets/brand/` reducidos en `core/ui/src/main/res/drawable-nodpi/` y `drawable-night-nodpi/` (un PNG no es un color: `ColorsOnlyInPaletteTest` no lo toca)                                                                                                                                                                                                                                                                                                                                                                                                                                                   | Tamaño del APK (pocos KB si se reducen); el título no debe empujar el indicador                                                                |
| 6   | Título «Pedidos pendientes» (`core/capture/…/strings.xml:75`, `PendingScreens.kt:570`) y botón de Mesas «Pedidos pendientes» (`feature/waiter/…/strings.xml:11`); «Reintentar ya» siempre visible, deshabilitado sin pendientes (`PendingScreens.kt:100-107`)                                                                                                                                                                                                                                                                                                                                                   | «Pendientes» en ambos; flecha (punto 1); «Reintentar ya» solo si hay algo que reintentar                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | Mínimo                                                                                                                                         |
| 7   | Mesas: campo «Buscar mesa» de ancho completo, fila «Lista \| Mapa» + «Pedidos pendientes», áreas solo dentro del mapa (`TablesScreen.kt:185-235`, `TablesMap.kt:62-76`); «Ver la mesa» (`TablesScreen.kt:166`, `openSecondaryLabel`). El mapa escala el lienzo fijo 1000 × 560 entero al espacio (`TablesMap.kt:77-87`, `TableCanvas.scaleToFit`): en la tableta vertical el panel de Mesas mide ~la mitad del ancho y el lienzo queda diminuto arriba con todo el alto vacío; en el teléfono horizontal (≈ 914 × 411 dp) el campo, la fila y las áreas se comen el alto y `weight(1f)` deja al mapa sin altura | Una fila: `Lista \| Mapa`, áreas como pestañas desplazables y «Pendientes»; sin búsqueda; tocar la mesa abre su panel (en el teléfono, sin «Ver la mesa»). Mapa como el escritorio: portar `fitTables` (`desktop/src/components/restaurant/TableMap.tsx:38-104`: escala por el **rectángulo que ocupan las mesas del área**, lado menor de una mesa ≥ 64 px, escala ≤ 2, margen 16, desplazamiento en ambos ejes si no caben) a `core:domain` con los mismos casos que su prueba del escritorio, y `horizontalScroll`+`verticalScroll` en Compose                                                                                                                                                                                         | El tamaño mínimo táctil (48 dp); quitar la búsqueda quita una función (la pide §4.7)                                                           |
| 8   | Tarjetas de cuenta del panel (`TablesScreen.kt:490-499` con `WorkRow`, 527-560): `Row { Column(weight 1f) { título; detalle }; badge }`. Compose mide primero el hijo **sin peso** (la etiqueta, `WaiterBadges.kt:299-321`, sin límite de ancho ni `maxLines`) con todo el ancho; con una etiqueta larga («Pendiente de liberación», «En preparación» con ícono y bordes) en un panel angosto la etiqueta toma casi todo y a la columna con peso le queda ~0 dp: el texto se parte letra por letra en vertical. **Esa es la causa.**                                                                            | Tarjeta en dos renglones: arriba «Mesa 6 · folio 12 · $755.00» (una línea, `maxLines = 1`, elipsis) y abajo la etiqueta corta; la etiqueta con `maxLines = 1` en todo `StateBadge`. Prueba de Compose en 320, 360, 400, 600 y 840 dp                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | El mismo `WorkRow` lo usan capturas y pedidos del panel: cambiarlos juntos                                                                     |
| 9   | La precuenta solo dentro de la cuenta (`OrderPanel.kt:235-236`) y solo si hay `PRINT_PRECHECK` (`CaptureScreen.kt:143-157`); sin permiso no aparece                                                                                                                                                                                                                                                                                                                                                                                                                                                             | En cada tarjeta de cuenta del panel, un botón «Imprimir precuenta» con el mismo `PrecheckPrintBar`/`PrecheckSection` de `core:capture` (`Precheck.kt:37-101`; `feature:waiter` ya depende de `core:capture`) y la misma lógica `PrecheckPrinting` (`core:domain`) en `TablesViewModel` (una impresión siguiéndose a la vez, por cuenta; `PrecheckRepository` sin cambios). Sin permiso: el botón deshabilitado con «Tu usuario no tiene permiso para imprimir la precuenta. Pídelo en la computadora principal.»; sin impresora (`PrecheckSetup.printer == null` ya leído): «Este dispositivo no tiene impresora asignada. Asígnala en la computadora principal, en Terminales.» (Go manda igual). En la cuenta, lo mismo (hoy se oculta) | Repetir el estado de impresión por cuenta; la lectura de `getIdentity` ya ocurre al entrar                                                     |
| 10  | El escritorio usa `POST restaurant/accounts/{id}/discard` (9D, exige `accounts.open` + `accounts.read`, y `takeaway.manage` si es para llevar: `operations/handler.go:251,124-127`; `accounts_release.go:99-150`) con `discardable` de Go. `accounts.open` está en `WaiterPermissions` (`waiter_scope.go:23`). Android ya tiene `WaiterApi.discardAccount` (`WaiterApi.kt:108-121`, lo usa el descarte de la captura) pero no lee `discardable`                                                                                                                                                                 | En la tarjeta de una cuenta con `discardable = true` (Go lo calcula; en `restaurant/state` cada cuenta lo trae, `recovery.go:31`), «Descartar cuenta vacía» con confirmación, con conexión y sesión (nunca a la cola), `operation_id` y `revision`; `ACCOUNT_NOT_EMPTY` → volver a leer. Sin `accounts.open`, el botón con su explicación                                                                                                                                                                                                                                                                                                                                                                                                 | La ruta y `discardable` no están documentados en el contrato v1 (ver §0.7)                                                                     |
| 11  | `ProductCard` normal: imagen 4:3 o el marcador con la letra grande (`CatalogBrowser.kt:250-296`, `ProductImage.kt:60-67`); compacta solo con poca altura                                                                                                                                                                                                                                                                                                                                                                                                                                                        | Sin imagen (`imageId == null`): teléfono, tarjeta compacta de nombre y precio (sin el área de imagen); tableta, la misma tarjeta sin imagen y más baja. Con imagen, como hoy                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | Cuadrícula con alturas distintas (filas con y sin imagen): usar alturas por fila o tarjeta compacta para todas las de esa categoría sin imagen |
| 12  | Siempre abre Mesas (`AdelPosApp.kt:422`, `startDestination = TablesDestination`). Go guarda `devices.home_screen` (0020: `tables`, `takeaway`, `kitchen`, `cash`; `CHECK devices_home_screen_check`) y **ya lo envía al `android`** en `terminal.home_screen` de `getIdentity` y `getRecoveryState` (`devices.go:52`, `routes.go:298`, `recovery.go:41`; esquema `Terminal` del contrato). `pinLogin` y `devices/{id}/configuration` no lo traen; Android ya lee `getIdentity` al entrar con conexión (`IdentityDto.terminal`, `PrecheckRepository.kt:34-49,147`)                                               | **Sin cambio en el servidor**: agregar `home_screen` a `IdentityTerminalDto`, guardarlo con el `precheck` de `restaurant_snapshot` (sin cambiar Room) y, al entrar un mesero, navegar a Para llevar si es `takeaway` **y** tiene `takeaway.read`; cualquier otro valor (`tables`, `kitchen`, `cash`, desconocido) → Mesas. Sin conexión, lo último guardado                                                                                                                                                                                                                                                                                                                                                                               | La primera entrada sin conexión no lo conoce (Mesas)                                                                                           |

### 0.6 Terminales en el escritorio (§5)

Tipos: `desktop`, `android` y `kitchen` (`0023…sql:219-220`, `DevicesPage.tsx:42-46`). Lo que ve cada uno hoy y la propuesta:

| Campo (`DevicesPage.tsx`)                                                            | Hoy                                                                                                                  | `desktop` / `kitchen`                                        | `android`                                                                                                  |
| ------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------- |
| Caja física (editor :626-637, ver :437-439, tabla :881)                              | Todos                                                                                                                | Sí                                                           | **No** (no cobra): en la tabla «—» en lugar de «Sin asociación»; fuera del editor y de «Ver configuración» |
| Teclado virtual (:638-648, :441-443)                                                 | Todos                                                                                                                | Sí                                                           | **No** (Android tiene el suyo)                                                                             |
| Pantalla inicial (:706-728, :433-435, tabla :882)                                    | Todos, con `terminalHomes` = Mesas, Para llevar, Cocina, Caja (`navigation/sections.ts:390-395`) y la nota de Cocina | Las cuatro                                                   | **Mesas y Para llevar**; sin la nota de Cocina                                                             |
| Bloqueo por inactividad, Impresora de precuentas, Avisos (:574-584, :444-464)        | Solo `android` (ya)                                                                                                  | No                                                           | Sí                                                                                                         |
| Estaciones que muestra en Cocina, aviso sonoro, impresoras de la terminal (:465-490) | Todos en «Ver configuración»                                                                                         | Sí                                                           | **No**                                                                                                     |
| «Código: `<uuid>`» (tabla :873-875)                                                  | Todos                                                                                                                | Se quita de la tabla; «Identificador» en «Ver configuración» | Igual                                                                                                      |

- **Migración** (`0027`, sección «Fase D»): `UPDATE app.devices SET home_screen = 'tables' WHERE kind = 'android' AND
home_screen IN ('kitchen', 'cash');` y después `ALTER TABLE app.devices ADD CONSTRAINT devices_android_home_check CHECK
(kind <> 'android' OR home_screen IN ('tables', 'takeaway'))`. Decisión propuesta: también «Caja» → «Mesas» (§5.1: «solo
  Mesas y Para llevar» y «el mismo criterio al resto»).
- **Go debe rechazar** `home_screen` distinto de `tables`/`takeaway` en un `android` en `PATCH devices/{id}`
  (`devices.go:140-166`, junto a `validTerminalHome`): 400 «En un teléfono o tableta de meseros la pantalla inicial es Mesas
  o Para llevar.» Prueba de integración en `administration` (y el `CHECK` lo garantiza en la base). Para la caja, Go sigue
  aceptando `register_id` nulo (el escritorio enviará `null` para un `android`); decisión propuesta: no rechazar más campos
  (un escritorio 0.13.0 manda `keyboard_mode` y `register_id` de siempre).
- «Ver configuración» lee `devices/{id}/configuration` (`DevicesPage.tsx:399-403`); el identificador ya viene en `device.id`.
- Pruebas: vitest de `DevicesPage` (opciones por tipo, «—», sin «Código:») y Playwright de Terminales.

### 0.7 Plan para el coordinador

#### Cambios del servidor por fase

| Fase | Cambio en Go                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | Contrato v1 (`openapi.json` + `docs/API_V1.md`)                               | `GET version`                                                             | `0027`                                                                                                                                                                                                                    |
| ---- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- | ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A    | Solo textos: los mensajes de §0.2 (Go) y su prueba de textos. La conexión tolerante es solo de Android                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | No cambia la forma; si algún mensaje está citado en `API_V1.md`, se actualiza | —                                                                         | —                                                                                                                                                                                                                         |
| B    | `administration/permission_catalog.go` (nombre, descripción corta, grupo y dependencias de los 156 permisos, capacidades y conjuntos de fábrica, con su prueba unitaria); `setup.go` concede los conjuntos de fábrica (sustituye las líneas 94-118 salvo el administrador); `GET permissions/catalog`; `GET` y `POST roles/{id}/defaults` con las reglas de `PATCH roles/{id}` y auditoría `role.defaults_restored`; pruebas de integración (instalación nueva con un mesero que atiende una mesa sin tocar permisos —también contra el Go temporal en Android—, restablecer con vista previa, escalada rechazada, rol propio, último administrador, `ROLE_CHANGED`). Puente de Rust (`api.rs`) con sus pruebas de `cargo` | No (rutas del escritorio); se documentan en `API_CONTRACTS.md`                | `permission-groups-v1` (el escritorio esconde la pantalla nueva sin ella) | No hace falta                                                                                                                                                                                                             |
| C    | Ninguno obligatorio. Documentar en el contrato lo que Android ya usa: `POST restaurant/accounts/{id}/discard` (`discardAccount`) y los campos `discardable`/`releasable` de `Account` en `openapi.json`, `docs/API_V1.md` y `TestContractV1` (sin cambiar comportamiento)                                                                                                                                                                                                                                                                                                                                                                                                                                                  | Sí, solo documentación                                                        | —                                                                         | —                                                                                                                                                                                                                         |
| D    | `PATCH devices/{id}` rechaza en un `android` una pantalla inicial que no sea `tables`/`takeaway` (prueba de integración); versión 0.14.0 y esquema 27                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | No (Terminales es del escritorio)                                             | —                                                                         | **Sí, la única**: sección «Fase D» con el `UPDATE` (`kitchen`/`cash` → `tables` en `android`) y el `CHECK devices_android_home_check`; caso en las pruebas de migración (`scripts/database.py` ya cubre `administration`) |
| E    | —                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | —                                                                             | —                                                                         | —                                                                                                                                                                                                                         |

Si la Fase B necesitara guardar algo (no debería: todo vive en Go), su sección iría también en la `0027` (una sola
migración con una sección por fase; la de D es la que la crea, o la crea B si llega antes: el coordinador decide el orden de
fusión).

#### Dependencias y paralelización con dos agentes

- **Agente 1 · Android**: **A-Android** (pool de 30 s, reintento de `GET`, `ServerLink` y sus consumidores, textos de
  Android con `PlainLanguageTest`, quitar `CatalogSample*`, regla 13 de `android/CLAUDE.md`) y después **C** (los 12
  puntos). C depende de A en `AdelTopBar` (la flecha y el logotipo van sobre la barra que A deja leyendo `ServerLink`) y en
  `TablesScreen`/`TakeawayScreen` (el aviso de sin conexión): hacerlos en ese orden en el mismo agente evita choques. Versión
  0.3.0 / 3 al final de C.
- **Agente 2 · servidor y escritorio**: **A-escritorio** (textos del escritorio y de Go, sus dos pruebas y la regla en
  `CLAUDE.md`), **B** (permisos y roles) y **D** (Terminales, `0027`, 0.14.0). B y D tocan `DevicesPage.tsx` solo en D y
  `RolesPage.tsx`/`PermissionEditor.tsx` solo en B.
- **Archivos que se pisarían** y a quién le tocan: `CLAUDE.md` (agente 2; el agente 1 deja su texto en su `doc-*.md` y el
  coordinador lo une en E), `android/CLAUDE.md` (agente 1), `docs/ANDROID_INSTALAR.md` (agente 2 en B: roles de fábrica;
  los cambios de §4 que nombra —«Pendientes», «Ingresar», pantalla inicial— los anota el agente 1 y se unen en E),
  `docs/API_V1.md` y `openapi.json` (agente 2, la documentación de C la hace él en D), `desktop/src/pages/DevicesPage.tsx`
  (agente 2 en A —textos de las líneas 84, 332, 338, 394, 528— y en D: el mismo agente), `backend/internal/kitchen/print_problems.go`
  (agente 2). El agente 1 no toca `backend/` ni `desktop/`; el agente 2 no toca `android/`.
- Fase E con **un** agente (cuando terminen los dos): regresión completa, documentación, APK 0.3.0 con la llave de siempre,
  `.pkg` 0.14.0 (parada), fusión y push.

#### Estrategia de pruebas

- **A**: JVM `ServerLinkTrackerTest` (tiempo virtual) y `PinnedHttpClientTest` (MockWebServer); `AccessRepository` con una
  falla aislada; instrumentadas de la barra y los avisos con `FakeAccess`; `PlainLanguageTest` (Android),
  `plainLanguage.test.tsx` (escritorio) y la de Go; contra el Go temporal una `ConnectionOnGoTest` corta (10 min en Mesas y
  en «¿Quién eres?», con el bloqueo por inactividad de fábrica y el reloj de Compose avanzando, como el spike) en
  `Pixel_Tablet`, sin fallas en el registro. Las pruebas de Go/escritorio de los textos que cambian (§0.2).
- **B**: Go unitario del catálogo (156 claves completas, sin ciclos, conjuntos cerrados) e integración de las rutas; vitest
  de la pantalla (grupos, «5 de 12», marcar/quitar grupo, buscador, dependencias, aviso al quitar, capacidad parcial);
  Playwright `workflow-stagea3-roles.spec.ts` (Chromium y WebKit); `cargo test` del puente; integración de Go de la
  instalación nueva: un mesero nuevo con su PIN abre una cuenta, captura y envía sin tocar permisos (el mismo paso con la
  aplicación, contra el Go temporal, en la Fase E). Pruebas existentes que cuentan los permisos de los roles después del
  asistente pueden cambiar (revisarlas). Revisor independiente.
- **C**: JVM del mapa (`fitTables` con los casos del escritorio), del título, de la pantalla inicial y de
  `discardable`; instrumentadas de cada punto en `Pixel_Tablet` y `ADEL_A1_Phone_API33`, vertical y horizontal (rotación
  con `UiAutomation.setRotation`), la tarjeta de cuenta en 320–840 dp, y contra el Go temporal la pantalla inicial Para
  llevar, la precuenta desde el panel y «Descartar cuenta vacía».
- **D**: integración de Go (`PATCH` rechaza `kitchen` en un `android`, migración `kitchen`/`cash` → `tables`), vitest y
  Playwright de Terminales.
- **E**: regresión completa (`go-unit`, `go-int` de todos los paquetes, `vitest`, `build`, `cargo`, Playwright en los dos
  motores, Android JVM + Lint + instrumentadas en los **cuatro** AVD con conteo de XML y `android-e2e.sh --suite all`), la
  **hora sin uso** en `Pixel_Tablet` con la Mac sin carga (una `ConnectionOnGoTest` de 60 min: ninguna falla de red en el
  registro, ningún aviso de sin conexión, barra y avisos iguales) y **capturas** de las pantallas principales en ambas
  aplicaciones (Android: «Selecciona tu usuario», Mesas en lista y mapa, panel de la mesa, captura, Pendientes, Para llevar,
  en teléfono y tableta y ambos temas; escritorio: Roles en vista sencilla y detallada, Terminales con un `android`, Sesiones
  activas sin la frase provisional).

#### Contradicciones y decisiones

- **Parada posible (decisión de negocio menor)**: el contrato v1 no documenta `discardAccount` ni `discardable`, que Android
  ya usa desde la A1 (R6) y que el punto 10 necesita. Decisión propuesta: documentarlos en la Fase C/D sin cambiar el
  comportamiento (no es una ruta nueva). No veo otra contradicción entre la especificación y el sistema.
- Decisiones propuestas (no paradas): umbral de 20 s / 3 fallas y sonda cada 5 s (§0.3); pool de 30 s y repetir un `GET`
  una vez; revisión cada 60 s inactivo y ninguna con el canal de eventos abierto; conjuntos de fábrica de §0.4 (revisor
  independiente); `takeaway.deliver` marcado en Mesero; «Manejar inventario» y «Autorizar y corregir» como capacidades
  extra; sin botón de restablecer en Administrador; dependencias informadas por Go pero no impuestas al guardar; «Caja» →
  «Mesas» además de «Cocina» en los `android`; quitar la pantalla de muestra `CatalogSample`; la clave cruda del Registro de
  movimientos solo sin etiqueta; los textos reescritos de §0.2; `min_client_versions.android` sin cambio.

### Pruebas y experimentos de la Fase 0

| Qué                                                                                               | Dónde                                                                                                              | Resultado                                                                                                           |
| ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------- |
| Catálogo de permisos de Go (156)                                                                  | `go run` de un programa del spike (`$SP/spike-a3/src/backend/cmd/p0dump`) que imprime `administration.Permissions` | 156 claves en `$SP/spike-a3/permissions.json`                                                                       |
| Grupos y conjuntos de fábrica                                                                     | `$SP/spike-a3/scripts/groups.py` y `roles.py`                                                                      | Cada permiso en un solo grupo (156); los 6 conjuntos cerrados bajo las 138 dependencias propuestas                  |
| Inactividad de Go (keep-alive)                                                                    | `$SP/spike-a3/scripts/idle_reuse.py 8451 …` desde la Mac                                                           | Reutilizar a 30/58/59,5/59,95 s: 200; a 60,05/61/75 s: «Remote end closed connection»                               |
| Protocolo de la escucha de red                                                                    | `openssl s_client -alpn h2,http/1.1`                                                                               | HTTP/1.1 («No ALPN negotiated»)                                                                                     |
| Plazo de lectura de Go en los canales                                                             | `net/http/server.go:741` de Go 1.27.3 (`.tools/go`)                                                                | `startBackgroundRead` quita el plazo de lectura                                                                     |
| Duración del canal de eventos                                                                     | `curl -N` a `GET events` (escucha local, terminal principal)                                                       | 30,01 s, 41 mensajes `changes`                                                                                      |
| Revisión de OkHttp 5.5.1                                                                          | `javap` sobre `okhttp-android-5.5.1.aar` (`RealRoutePlanner`, `RealConnection.isHealthy`)                          | Sin revisión extensa para `GET`; revisión del socket solo con ≥ 10 s libre y revisión extensa                       |
| Corrida 1, sin arreglo (13 min Mesas + 13 min «¿Quién eres?»)                                     | `IdleProbeOnGoTest` en `Pixel_Tablet` y `ADEL_A1_Phone_API33` a la vez                                             | 12 y 13 fallas, todas `GET version` sobre una conexión libre ≥ 60 s; 0 fallas con < 60 s                            |
| Corrida A, sin arreglo (15 min «¿Quién eres?», reloj de Compose en tiempo real)                   | `Pixel_Tablet`                                                                                                     | 84 llamadas, 0 fallas: la lista cada 30 s mantiene viva una sola conexión                                           |
| Corrida B, **con** el pool de 30 s (8 min Mesas + 8 min «¿Quién eres?»)                           | `ADEL_A1_Phone_API33`                                                                                              | 90 llamadas, 0 fallas, conexión reutilizada con 25 s libre como máximo                                              |
| Corrida C, sin arreglo, bloqueo por inactividad de fábrica (120 s) y el mesero volviendo a entrar | `Pixel_Tablet`, 14 min                                                                                             | 12 fallas (todas con ≥ 60 s libre); aviso «Sin conexión…» con la barra «En línea» en 4 de 5 bloqueos, 32 s cada uno |
| Corrida D, **con** el pool de 30 s, mismo guion que C                                             | `ADEL_A1_Phone_API33`, 14 min                                                                                      | 0 fallas, 0 avisos en 782 s, 5 bloqueos y 5 entradas con sesión                                                     |

No se ejecutó ninguna prueba del repositorio (la Fase 0 no cambia código). Todo lo de `spike` es desechable; el repositorio
y `$SP/wt/p0` quedaron sin cambios (`git status` limpio) y `docs/etapas/ETAPA_A3_AJUSTES.md` sin tocar.

### Decisiones del coordinador al cerrar la Fase 0

Ninguna es una parada: ninguna cambia una decisión de la especificación ni una regla de negocio.

- **A3-D1 · Conexión tolerante** (§0.3, §2). Se corrige la causa en el cliente: `ConnectionPool(5, 30 s)` en
  `PinnedHttpClient` (OkHttp suelta la conexión libre antes de que Go la cierre a los 60 s) y un `GET` que falla sin
  respuesta se repite **una vez** con una conexión nueva (las escrituras conservan su `operation_id` y sus reglas;
  `retryOnConnectionFailure` sigue en `false`). Go no cambia. Un solo estado `ServerLink` (lógica pura en `core:domain`,
  una instancia en `AppContainer`) alimentado por la red de Android y por cada respuesta de Go; «Sin conexión» con **≥ 3
  fallas seguidas y ≥ 20 s sin ningún éxito** (sonda `health/live` cada 5 s mientras duda) o al instante si Android no
  tiene red; «En línea» con el primer éxito. Revisión: ninguna extra con el canal de eventos abierto; sin canal o
  inactivo, cada 60 s y se salta si hubo otro éxito; `GET version` cada 5 min. Al tocar algo que necesita a Go, se
  intenta antes de mostrar un error. No cambian el canal de avisos de la A2 ni la regla de la cola sin conexión.
- **A3-D2 · Roles de fábrica** (§0.4, §3.1). Los conjuntos del anexo (Mesero 14, Cajero 36, Cocina 8, Supervisor 89,
  Inventarios 20, Finanzas 28; Administrador todos), cerrados bajo las dependencias, **sujetos a la revisión
  independiente** de §0 de la especificación. Se conceden solo en el asistente de una instalación nueva (`setup.go`) y con
  «Restablecer permisos predeterminados»; la `0027` no toca roles. Cocina e Inventarios ya existen en toda instalación.
- **A3-D3 · «Entregado» en Mesero** (`takeaway.deliver`): marcado. §3.1 pide todo lo de la tabla de la guía y solo
  exceptúa «Modificar» y «Cancelar unidades»; Go sigue sin entregar un pedido sin pagar (`BALANCE_PENDING`).
- **A3-D4 · Dependencias**: se definen en un solo lugar de Go (el catálogo de permisos) y la interfaz las lee de ahí; la
  interfaz marca sola lo que se necesita y avisa al quitar. Go **no rechaza** al guardar un rol que no las cumpla (un rol
  existente de la instalación real puede no cumplirlas y debe poder editarse); sigue validando cada permiso en cada ruta.
- **A3-D5 · Capacidades**: las nueve de §3.2 más «Manejar inventario» y «Autorizar y corregir» (§3.2 dice «opciones
  como…»; sin ellas, Inventarios y Supervisor no se arman desde la vista sencilla). Definidas en el mismo catálogo de Go.
- **A3-D6 · Restablecer**: solo en los roles de fábrica con `code` (`waiter`, `cashier`, `kitchen`, `supervisor`,
  `inventory`, `finance`); sin botón en el Administrador (siempre tiene todo y nadie edita su propio rol), en el soporte
  reservado ni en los roles del usuario. Las mismas reglas que `PATCH roles/{id}` (nadie concede lo que no tiene, no el
  propio rol, nunca quedarse sin administrador), vista previa con lo que se agrega y se quita, confirmación contra lo que
  mostró la vista previa (`ROLE_CHANGED` si cambió) y auditoría `role.defaults_restored`. No toca el límite de descuento ni
  las excepciones por usuario (se dice en la confirmación).
- **A3-D7 · Pantalla inicial de un `android`**: solo Mesas y Para llevar. La `0027` (la única de la etapa, sección «Fase
  D») pasa a «Mesas» los `android` con «Cocina» **o «Caja»** guardada (§5.1: «cada tipo solo ve lo que le aplica») y
  agrega el `CHECK`; el `PATCH devices/{id}` rechaza otra pantalla inicial en un `android`. La aplicación trata cualquier
  valor desconocido como Mesas (una 0.3.0 contra un 0.13.0).
- **A3-D8 · `discardAccount` en el contrato**: se documentan `POST restaurant/accounts/{id}/discard` y los campos
  `discardable`/`releasable` de `Account` en `openapi.json`, `docs/API_V1.md` y `TestContractV1`, sin cambiar su
  comportamiento (Android los usa desde la A1; el punto 10 de §4 los necesita). Lo hace el agente del servidor en la Fase D.
- **A3-D9 · Versiones**: Android 0.3.0 / `versionCode` 3 y servidor 0.14.0, esquema 27. La 0.3.0 no necesita nada nuevo de
  Go: sin capacidad nueva en `ServerCompatibility.check` (`MIN_SERVER_VERSION` sigue en 0.13.0) y
  `min_client_versions.android` sigue en 0.1.0 (como A2-D6). La pantalla nueva de permisos del escritorio se esconde sin
  la capacidad `permission-groups-v1` (una terminal 0.14.0 contra un 0.13.0).
- **A3-D10 · Pantalla de muestra del catálogo** (`CatalogSample*`, sin ruta en la navegación desde la A1): se quita con
  sus textos de desarrollo.
- **A3-D11 · Textos**: se aceptan las reescrituras de §0.2. Los «todavía no»/«aún no» que describen un estado real se
  reescriben (no se exceptúan); las excepciones de la comprobación automática quedan escritas con su porqué junto a la
  comprobación.

### Plan de las fases siguientes

Dos agentes a la vez como máximo, cada uno en su árbol de trabajo y su rama desde `etapa-a3`, y el coordinador fusiona
cada fase en `etapa-a3` con su resumen en este avance:

- **Agente de Android** (rama `etapa-a3-and`): Fase A en Android (A3-D1, textos de Android con su comprobación, A3-D10,
  regla en `android/CLAUDE.md`) y después la Fase C (los 12 puntos de §4, versión 0.3.0 al final). No toca `backend/` ni
  `desktop/`.
- **Agente del servidor y el escritorio** (rama `etapa-a3-srv`): Fase A en el escritorio y en Go (textos y sus
  comprobaciones, regla en `CLAUDE.md`), Fase D (Terminales, `0027`, A3-D8) y Fase B (permisos agrupados y roles de
  fábrica). No toca `android/`. B va al final para que la revisión independiente corra mientras el agente de Android
  sigue con la Fase C.
- **Revisión independiente** de §3 (A3-D2, A3-D6) en cuanto termine la Fase B; sus correcciones las hace el agente del
  servidor.
- **Fase E** con un solo agente: regresión completa (servidor, escritorio y Android en los cuatro emuladores), la hora sin
  uso, las capturas, la documentación, el APK 0.3.0 y el `.pkg` 0.14.0 (parada: lo instala el propietario).

### Anexo · Conjuntos de fábrica con su nombre visible y su porqué

**Mesero (waiter)** (14)

| Clave                    | Nombre visible (hoy)                                  | Por qué                                                                                |
| ------------------------ | ----------------------------------------------------- | -------------------------------------------------------------------------------------- |
| `accounts.open`          | Abrir cuentas                                         | Abrir la cuenta con el primer producto; descartar la cuenta vacía                      |
| `accounts.read`          | Consultar cuentas e importes                          | Toda ruta de cuentas lo exige (`operations/handler.go:112`, `payments/handler.go:147`) |
| `catalog.prices.read`    | Consultar precios y previsualizar cálculos            | Capturar exige catálogo y precios (`handler.go:132,148`)                               |
| `catalog.read`           | Consultar catálogo y configuraciones                  | Capturar exige catálogo y precios (`handler.go:132,148`)                               |
| `events.read`            | Recibir eventos autenticados                          | Ver al momento los cambios de otras terminales y de cocina                             |
| `orders.capture`         | Capturar y editar borradores de pedidos               | Capturar y «Enviar a cocina» (también sin conexión)                                    |
| `orders.confirm`         | Confirmar pedidos                                     | Capturar y «Enviar a cocina» (también sin conexión)                                    |
| `payments.tickets.print` | Imprimir tickets y precuentas                         | Imprimir la precuenta o el ticket                                                      |
| `restaurant.areas.read`  | Consultar áreas del restaurante                       | Ver Mesas (`restaurant/overview`, `restaurant/state`)                                  |
| `restaurant.tables.read` | Consultar mesas y ocupación real                      | Ver Mesas (`restaurant/overview`, `restaurant/state`)                                  |
| `takeaway.deliver`       | Registrar entrega de pedidos para llevar              | Marcar «Entregado» un pedido pagado (Go rechaza si no está pagado)                     |
| `takeaway.manage`        | Administrar datos de pedidos para llevar              | Para llevar: ver la lista y tomar pedidos con nombre y teléfono                        |
| `takeaway.read`          | Consultar tablero de pedidos para llevar y ventanilla | Para llevar: ver la lista y tomar pedidos con nombre y teléfono                        |
| `waiter_devices.use`     | Entrar con PIN en teléfonos y tabletas de meseros     | Entra con PIN en el teléfono o la tableta                                              |

**Cajero (cashier)** (36)

| Clave                        | Nombre visible (hoy)                                  | Por qué                                                                                                                   |
| ---------------------------- | ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `accounts.open`              | Abrir cuentas                                         | Abrir la cuenta con el primer producto; descartar la cuenta vacía                                                         |
| `accounts.read`              | Consultar cuentas e importes                          | Toda ruta de cuentas lo exige (`operations/handler.go:112`, `payments/handler.go:147`)                                    |
| `cash.counts.perform`        | Realizar arqueos                                      | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.drawer.open`           | Abrir el cajón de dinero manualmente                  | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.expenses.record`       | Registrar gastos pagados desde caja                   | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.income.record`         | Registrar ingresos de efectivo                        | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.movements.read`        | Consultar movimientos de efectivo                     | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.partial_cuts.create`   | Realizar cortes parciales                             | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.reports.print`         | Imprimir cortes y comprobantes de caja                | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.shifts.close`          | Cerrar turnos de caja                                 | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.shifts.open`           | Abrir turno de caja                                   | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.shifts.read`           | Consultar turnos y movimientos de caja                | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.withdrawals.record`    | Registrar retiros de efectivo                         | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `catalog.prices.read`        | Consultar precios y previsualizar cálculos            | Capturar exige catálogo y precios (`handler.go:132,148`)                                                                  |
| `catalog.read`               | Consultar catálogo y configuraciones                  | Capturar exige catálogo y precios (`handler.go:132,148`)                                                                  |
| `events.read`                | Recibir eventos autenticados                          | Ver al momento los cambios de otras terminales y de cocina                                                                |
| `kitchen.states.read`        | Consultar preparación                                 | Ver la preparación de los pedidos (sección Cocina, solo lectura)                                                          |
| `kitchen.stations.read`      | Consultar configuración de estaciones                 | Ver la preparación de los pedidos (sección Cocina, solo lectura)                                                          |
| `orders.capture`             | Capturar y editar borradores de pedidos               | Capturar y «Enviar a cocina» (también sin conexión)                                                                       |
| `orders.confirm`             | Confirmar pedidos                                     | Capturar y «Enviar a cocina» (también sin conexión)                                                                       |
| `payments.charges.create`    | Registrar cargos extras generales                     | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.collect`           | Registrar cobros                                      | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.collect_partial`   | Registrar pagos parciales                             | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.collect_selection` | Cobrar productos y cantidades seleccionados           | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.discounts.apply`   | Aplicar descuentos manuales dentro del límite del rol | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.read`              | Consultar información financiera y pagos de cuentas   | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.tickets.print`     | Imprimir tickets y precuentas                         | Imprimir la precuenta o el ticket                                                                                         |
| `payments.tickets.reprint`   | Reimprimir tickets                                    | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.tips.record`       | Registrar propinas                                    | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.usd_rate.manage`   | Capturar el tipo de cambio del dólar del turno        | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `registers.read`             | Consultar cajas físicas                               | Ver las cajas físicas (ya lo tiene hoy, `setup.go:114`)                                                                   |
| `restaurant.areas.read`      | Consultar áreas del restaurante                       | Ver Mesas (`restaurant/overview`, `restaurant/state`)                                                                     |
| `restaurant.tables.read`     | Consultar mesas y ocupación real                      | Ver Mesas (`restaurant/overview`, `restaurant/state`)                                                                     |
| `takeaway.deliver`           | Registrar entrega de pedidos para llevar              | Marcar «Entregado» un pedido pagado (Go rechaza si no está pagado)                                                        |
| `takeaway.manage`            | Administrar datos de pedidos para llevar              | Para llevar: ver la lista y tomar pedidos con nombre y teléfono                                                           |
| `takeaway.read`              | Consultar tablero de pedidos para llevar y ventanilla | Para llevar: ver la lista y tomar pedidos con nombre y teléfono                                                           |

**Cocina (kitchen)** (8)

| Clave                   | Nombre visible (hoy)                  | Por qué                                                                    |
| ----------------------- | ------------------------------------- | -------------------------------------------------------------------------- |
| `events.read`           | Recibir eventos autenticados          | Ver al momento los cambios de otras terminales y de cocina                 |
| `kitchen.history.read`  | Consultar historial de cocina         | Pantalla de preparación: comandas, estados, «Listo», reimprimir, historial |
| `kitchen.kds.ack`       | Confirmar recepción KDS               | Pantalla de preparación: comandas, estados, «Listo», reimprimir, historial |
| `kitchen.reprint`       | Solicitar reimpresión                 | Pantalla de preparación: comandas, estados, «Listo», reimprimir, historial |
| `kitchen.states.manage` | Actualizar preparación                | Pantalla de preparación: comandas, estados, «Listo», reimprimir, historial |
| `kitchen.states.read`   | Consultar preparación                 | Pantalla de preparación: comandas, estados, «Listo», reimprimir, historial |
| `kitchen.stations.read` | Consultar configuración de estaciones | Pantalla de preparación: comandas, estados, «Listo», reimprimir, historial |
| `kitchen.tickets.read`  | Consultar comandas por estación       | Pantalla de preparación: comandas, estados, «Listo», reimprimir, historial |

**Supervisor (supervisor)** (89)

| Clave                           | Nombre visible (hoy)                                                    | Por qué                                                                                                                   |
| ------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `accounts.cancel`               | Cancelar cuentas                                                        | Modificar y cancelar lo enviado, dividir, combinar, mover y transferir cuentas, historial                                 |
| `accounts.manage`               | Editar datos de cuentas                                                 | Modificar y cancelar lo enviado, dividir, combinar, mover y transferir cuentas, historial                                 |
| `accounts.merge`                | Combinar cuentas compatibles                                            | Modificar y cancelar lo enviado, dividir, combinar, mover y transferir cuentas, historial                                 |
| `accounts.move`                 | Mover cuentas entre mesas                                               | Modificar y cancelar lo enviado, dividir, combinar, mover y transferir cuentas, historial                                 |
| `accounts.open`                 | Abrir cuentas                                                           | Abrir la cuenta con el primer producto; descartar la cuenta vacía                                                         |
| `accounts.read`                 | Consultar cuentas e importes                                            | Toda ruta de cuentas lo exige (`operations/handler.go:112`, `payments/handler.go:147`)                                    |
| `accounts.split`                | Dividir cuentas                                                         | Modificar y cancelar lo enviado, dividir, combinar, mover y transferir cuentas, historial                                 |
| `accounts.transfer`             | Transferir la responsabilidad de cuentas                                | Modificar y cancelar lo enviado, dividir, combinar, mover y transferir cuentas, historial                                 |
| `accounts.transfer_items`       | Transferir partidas y cantidades entre cuentas                          | Modificar y cancelar lo enviado, dividir, combinar, mover y transferir cuentas, historial                                 |
| `audit.read`                    | Consultar el registro de movimientos                                    | Revisar el Registro de movimientos                                                                                        |
| `cash.closing.cancel_any`       | Cancelar el arqueo de cierre iniciado por otro usuario                  | Supervisar la caja: autorizar, corregir, revisar y consultar otras cajas                                                  |
| `cash.closures.review`          | Comentar cierres de caja                                                | Supervisar la caja: autorizar, corregir, revisar y consultar otras cajas                                                  |
| `cash.corrections.authorize`    | Autorizar correcciones y regularizaciones de efectivo                   | Supervisar la caja: autorizar, corregir, revisar y consultar otras cajas                                                  |
| `cash.corrections.record`       | Corregir y regularizar movimientos de efectivo                          | Supervisar la caja: autorizar, corregir, revisar y consultar otras cajas                                                  |
| `cash.counts.perform`           | Realizar arqueos                                                        | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.drawer.open`              | Abrir el cajón de dinero manualmente                                    | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.expected.read`            | Consultar importes esperados y diferencias                              | Supervisar la caja: autorizar, corregir, revisar y consultar otras cajas                                                  |
| `cash.expenses.record`          | Registrar gastos pagados desde caja                                     | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.history.read`             | Consultar turnos y cortes históricos                                    | Supervisar la caja: autorizar, corregir, revisar y consultar otras cajas                                                  |
| `cash.income.record`            | Registrar ingresos de efectivo                                          | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.movements.read`           | Consultar movimientos de efectivo                                       | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.outflows.authorize`       | Autorizar retiros y gastos de efectivo                                  | Supervisar la caja: autorizar, corregir, revisar y consultar otras cajas                                                  |
| `cash.partial_cuts.create`      | Realizar cortes parciales                                               | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.reports.print`            | Imprimir cortes y comprobantes de caja                                  | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.reports.reprint`          | Reimprimir cortes y comprobantes de caja                                | Supervisar la caja: autorizar, corregir, revisar y consultar otras cajas                                                  |
| `cash.shifts.close`             | Cerrar turnos de caja                                                   | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.shifts.open`              | Abrir turno de caja                                                     | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.shifts.read`              | Consultar turnos y movimientos de caja                                  | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `cash.shifts.read_all`          | Consultar turnos de otras cajas                                         | Supervisar la caja: autorizar, corregir, revisar y consultar otras cajas                                                  |
| `cash.withdrawals.record`       | Registrar retiros de efectivo                                           | Manejar su turno: abrir, arqueo, cortes, ingresos, retiros, gastos, cierre, cajón e impresión                             |
| `catalog.availability.manage`   | Modificar disponibilidad y horarios                                     | Pausar, autorizar excepciones y marcar agotados durante el servicio                                                       |
| `catalog.availability.override` | Autorizar excepciones de disponibilidad en previsualización             | Pausar, autorizar excepciones y marcar agotados durante el servicio                                                       |
| `catalog.prices.read`           | Consultar precios y previsualizar cálculos                              | Capturar exige catálogo y precios (`handler.go:132,148`)                                                                  |
| `catalog.read`                  | Consultar catálogo y configuraciones                                    | Capturar exige catálogo y precios (`handler.go:132,148`)                                                                  |
| `catalog.sold_out.manage`       | Marcar productos como agotados y disponibles otra vez desde la captura  | Pausar, autorizar excepciones y marcar agotados durante el servicio                                                       |
| `devices.read`                  | Consultar terminales                                                    | Ver personal, terminales y sesiones; cerrar una sesión olvidada                                                           |
| `events.read`                   | Recibir eventos autenticados                                            | Ver al momento los cambios de otras terminales y de cocina                                                                |
| `kitchen.history.read`          | Consultar historial de cocina                                           | Operar y resolver la cocina y la cola de impresión                                                                        |
| `kitchen.kds.ack`               | Confirmar recepción KDS                                                 | Operar y resolver la cocina y la cola de impresión                                                                        |
| `kitchen.printers.test`         | Solicitar impresión de prueba                                           | Operar y resolver la cocina y la cola de impresión                                                                        |
| `kitchen.queue.read`            | Consultar trabajos e intentos de impresión                              | Operar y resolver la cocina y la cola de impresión                                                                        |
| `kitchen.queue.resolve`         | Resolver errores y resultados inciertos                                 | Operar y resolver la cocina y la cola de impresión                                                                        |
| `kitchen.queue.retry`           | Reintentar errores confirmados                                          | Operar y resolver la cocina y la cola de impresión                                                                        |
| `kitchen.release`               | Liberar pedidos a preparación                                           | Operar y resolver la cocina y la cola de impresión                                                                        |
| `kitchen.release_early`         | Liberar anticipadamente pedidos programados                             | Operar y resolver la cocina y la cola de impresión                                                                        |
| `kitchen.reprint`               | Solicitar reimpresión                                                   | Operar y resolver la cocina y la cola de impresión                                                                        |
| `kitchen.states.manage`         | Actualizar preparación                                                  | Operar y resolver la cocina y la cola de impresión                                                                        |
| `kitchen.states.read`           | Consultar preparación                                                   | Operar y resolver la cocina y la cola de impresión                                                                        |
| `kitchen.stations.read`         | Consultar configuración de estaciones                                   | Operar y resolver la cocina y la cola de impresión                                                                        |
| `kitchen.tickets.read`          | Consultar comandas por estación                                         | Operar y resolver la cocina y la cola de impresión                                                                        |
| `orders.cancel`                 | Cancelar partidas o cantidades confirmadas                              | Modificar y cancelar lo enviado, dividir, combinar, mover y transferir cuentas, historial                                 |
| `orders.capture`                | Capturar y editar borradores de pedidos                                 | Capturar y «Enviar a cocina» (también sin conexión)                                                                       |
| `orders.confirm`                | Confirmar pedidos                                                       | Capturar y «Enviar a cocina» (también sin conexión)                                                                       |
| `orders.edit`                   | Modificar partidas confirmadas sin aprobación adicional                 | Modificar y cancelar lo enviado, dividir, combinar, mover y transferir cuentas, historial                                 |
| `orders.history.read`           | Consultar historial de pedidos y cuentas                                | Modificar y cancelar lo enviado, dividir, combinar, mover y transferir cuentas, historial                                 |
| `payments.charges.create`       | Registrar cargos extras generales                                       | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.charges.manage`       | Modificar o cancelar cargos extras                                      | Autorizaciones y correcciones de cobro (del supervisor)                                                                   |
| `payments.collect`              | Registrar cobros                                                        | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.collect_partial`      | Registrar pagos parciales                                               | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.collect_selection`    | Cobrar productos y cantidades seleccionados                             | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.corrections`          | Anular o corregir pagos                                                 | Autorizaciones y correcciones de cobro (del supervisor)                                                                   |
| `payments.courtesy`             | Otorgar cortesías                                                       | Autorizaciones y correcciones de cobro (del supervisor)                                                                   |
| `payments.discounts.apply`      | Aplicar descuentos manuales dentro del límite del rol                   | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.discounts.authorize`  | Autorizar descuentos superiores al límite                               | Autorizaciones y correcciones de cobro (del supervisor)                                                                   |
| `payments.read`                 | Consultar información financiera y pagos de cuentas                     | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.refunds.authorize`    | Autorizar devoluciones                                                  | Autorizaciones y correcciones de cobro (del supervisor)                                                                   |
| `payments.refunds.execute`      | Ejecutar devoluciones                                                   | Autorizaciones y correcciones de cobro (del supervisor)                                                                   |
| `payments.tickets.print`        | Imprimir tickets y precuentas                                           | Imprimir la precuenta o el ticket                                                                                         |
| `payments.tickets.reprint`      | Reimprimir tickets                                                      | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.tips.record`          | Registrar propinas                                                      | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `payments.usd_rate.manage`      | Capturar el tipo de cambio del dólar del turno                          | Cobrar en caja (pagos, parciales, por productos, propinas, descuentos dentro del límite, cargos, tickets, tipo de cambio) |
| `registers.read`                | Consultar cajas físicas                                                 | Ver las cajas físicas (ya lo tiene hoy, `setup.go:114`)                                                                   |
| `reports.cash.read`             | Consultar reportes de caja                                              | Reportes (requieren `reports.read`)                                                                                       |
| `reports.discounts.read`        | Consultar reportes de descuentos, cortesías y autorizaciones            | Reportes (requieren `reports.read`)                                                                                       |
| `reports.employees.read`        | Consultar reportes por empleado                                         | Reportes (requieren `reports.read`)                                                                                       |
| `reports.export.excel`          | Exportar reportes a Excel                                               | Reportes (requieren `reports.read`)                                                                                       |
| `reports.export.pdf`            | Exportar reportes a PDF                                                 | Reportes (requieren `reports.read`)                                                                                       |
| `reports.payments.read`         | Consultar reportes de cobros, devoluciones y propinas                   | Reportes (requieren `reports.read`)                                                                                       |
| `reports.read`                  | Consultar el módulo Reportes y el resumen ejecutivo                     | Reportes (requieren `reports.read`)                                                                                       |
| `reports.sales.read`            | Consultar reportes de ventas, productos, categorías, cocina y operación | Reportes (requieren `reports.read`)                                                                                       |
| `restaurant.areas.read`         | Consultar áreas del restaurante                                         | Ver Mesas (`restaurant/overview`, `restaurant/state`)                                                                     |
| `restaurant.tables.read`        | Consultar mesas y ocupación real                                        | Ver Mesas (`restaurant/overview`, `restaurant/state`)                                                                     |
| `sessions.manage`               | Revocar sesiones                                                        | Ver personal, terminales y sesiones; cerrar una sesión olvidada                                                           |
| `sessions.read`                 | Consultar sesiones                                                      | Ver personal, terminales y sesiones; cerrar una sesión olvidada                                                           |
| `takeaway.deliver`              | Registrar entrega de pedidos para llevar                                | Marcar «Entregado» un pedido pagado (Go rechaza si no está pagado)                                                        |
| `takeaway.manage`               | Administrar datos de pedidos para llevar                                | Para llevar: ver la lista y tomar pedidos con nombre y teléfono                                                           |
| `takeaway.read`                 | Consultar tablero de pedidos para llevar y ventanilla                   | Para llevar: ver la lista y tomar pedidos con nombre y teléfono                                                           |
| `users.read`                    | Consultar usuarios                                                      | Ver personal, terminales y sesiones; cerrar una sesión olvidada                                                           |
| `waiter_devices.use`            | Entrar con PIN en teléfonos y tabletas de meseros                       | Entra con PIN en el teléfono o la tableta                                                                                 |

**Inventarios (inventory)** (20)

| Clave                               | Nombre visible (hoy)                                                    | Por qué                                                                                                                   |
| ----------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `catalog.read`                      | Consultar catálogo y configuraciones                                    | Las recetas son de productos del catálogo                                                                                 |
| `inventory.adjustments.record`      | Registrar ajustes de inventario                                         | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.costs.read`              | Consultar costos (unitarios, valores, costo teórico)                    | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.counts.perform`          | Realizar conteos físicos                                                | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.items.manage`            | Administrar insumos, unidades y presentaciones                          | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.movements.read`          | Consultar movimientos (cantidades; valores solo con consulta de costos) | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.production.record`       | Registrar elaboraciones de subrecetas almacenables                      | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.purchases.create`        | Crear órdenes de compra                                                 | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.purchases.receive`       | Recibir mercancía                                                       | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.read`                    | Consultar inventario (sección y resumen)                                | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.recipes.manage`          | Administrar recetas y porciones habituales                              | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.stock.read`              | Consultar existencias (cantidades, sin costos)                          | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.subrecipes.manage`       | Administrar subrecetas (preparaciones)                                  | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.supplier_returns.record` | Registrar devoluciones a proveedores                                    | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.suppliers.manage`        | Administrar proveedores                                                 | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.transfers.record`        | Transferir mercancía entre almacenes                                    | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `inventory.waste.record`            | Registrar mermas y clasificar cancelaciones                             | Operar inventario: insumos, recetas, elaboraciones, mermas, proveedores, compras, recepción, ajustes, conteos y traspasos |
| `reports.inventory.read`            | Consultar reportes de inventarios                                       | Reportes (requieren `reports.read`)                                                                                       |
| `reports.purchases.read`            | Consultar reportes de compras y proveedores                             | Reportes (requieren `reports.read`)                                                                                       |
| `reports.read`                      | Consultar el módulo Reportes y el resumen ejecutivo                     | Reportes (requieren `reports.read`)                                                                                       |

**Finanzas (finance)** (28)

| Clave                           | Nombre visible (hoy)                                                    | Por qué                                                                                   |
| ------------------------------- | ----------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| `accounts.read`                 | Consultar cuentas e importes                                            | Toda ruta de cuentas lo exige (`operations/handler.go:112`, `payments/handler.go:147`)    |
| `audit.read`                    | Consultar el registro de movimientos                                    | Revisar el Registro de movimientos                                                        |
| `cash.closures.review`          | Comentar cierres de caja                                                | Consultar y revisar turnos, cortes y diferencias                                          |
| `cash.expected.read`            | Consultar importes esperados y diferencias                              | Consultar y revisar turnos, cortes y diferencias                                          |
| `cash.history.read`             | Consultar turnos y cortes históricos                                    | Consultar y revisar turnos, cortes y diferencias                                          |
| `cash.movements.read`           | Consultar movimientos de efectivo                                       | Consultar turnos y movimientos de caja                                                    |
| `cash.shifts.read`              | Consultar turnos y movimientos de caja                                  | Consultar turnos y movimientos de caja                                                    |
| `cash.shifts.read_all`          | Consultar turnos de otras cajas                                         | Consultar y revisar turnos, cortes y diferencias                                          |
| `inventory.costs.read`          | Consultar costos (unitarios, valores, costo teórico)                    | Consultar existencias, costos y valuación; autorizar compras y vincularlas con gastos     |
| `inventory.movements.read`      | Consultar movimientos (cantidades; valores solo con consulta de costos) | Consultar existencias, costos y valuación; autorizar compras y vincularlas con gastos     |
| `inventory.purchases.authorize` | Emitir (autorizar), cancelar y cerrar órdenes de compra                 | Consultar existencias, costos y valuación; autorizar compras y vincularlas con gastos     |
| `inventory.purchases.link_cash` | Vincular compras con gastos de caja                                     | Consultar existencias, costos y valuación; autorizar compras y vincularlas con gastos     |
| `inventory.read`                | Consultar inventario (sección y resumen)                                | Consultar existencias, costos y valuación; autorizar compras y vincularlas con gastos     |
| `inventory.stock.read`          | Consultar existencias (cantidades, sin costos)                          | Consultar existencias, costos y valuación; autorizar compras y vincularlas con gastos     |
| `inventory.valuation.read`      | Consultar valuaciones (valor total del inventario por insumo/almacén)   | Consultar existencias, costos y valuación; autorizar compras y vincularlas con gastos     |
| `orders.history.read`           | Consultar historial de pedidos y cuentas                                | Modificar y cancelar lo enviado, dividir, combinar, mover y transferir cuentas, historial |
| `payments.read`                 | Consultar información financiera y pagos de cuentas                     | Consultar cobros y pagos                                                                  |
| `registers.read`                | Consultar cajas físicas                                                 | Ver las cajas físicas (ya lo tiene hoy, `setup.go:114`)                                   |
| `reports.cash.read`             | Consultar reportes de caja                                              | Reportes (requieren `reports.read`)                                                       |
| `reports.discounts.read`        | Consultar reportes de descuentos, cortesías y autorizaciones            | Reportes (requieren `reports.read`)                                                       |
| `reports.employees.read`        | Consultar reportes por empleado                                         | Reportes (requieren `reports.read`)                                                       |
| `reports.export.excel`          | Exportar reportes a Excel                                               | Reportes (requieren `reports.read`)                                                       |
| `reports.export.pdf`            | Exportar reportes a PDF                                                 | Reportes (requieren `reports.read`)                                                       |
| `reports.inventory.read`        | Consultar reportes de inventarios                                       | Reportes (requieren `reports.read`)                                                       |
| `reports.payments.read`         | Consultar reportes de cobros, devoluciones y propinas                   | Reportes (requieren `reports.read`)                                                       |
| `reports.purchases.read`        | Consultar reportes de compras y proveedores                             | Reportes (requieren `reports.read`)                                                       |
| `reports.read`                  | Consultar el módulo Reportes y el resumen ejecutivo                     | Reportes (requieren `reports.read`)                                                       |
| `reports.sales.read`            | Consultar reportes de ventas, productos, categorías, cocina y operación | Reportes (requieren `reports.read`)                                                       |

---

## Fase A · Escritorio y Go: regla de textos (§1)

Agente del servidor y el escritorio, en `$SP/wt/srv` (rama `etapa-a3-srv`, desde `etapa-a3` = `4c7231b`), 19 de
noviembre de 2026. Sin cambios en `android/`, `scripts/android-e2e.sh` ni en este avance. Pruebas en copias aisladas
(`ISO_TAG=SV`: clúster temporal 55499, Go temporal en 8187 y vista previa en 1427 para Playwright); ninguna contra el
servicio real. Commits: ver «Commits» al final.

### Qué se hizo

**Textos reescritos o quitados.** Todos los de §0.2 del escritorio y de Go (A3-D11: los «todavía no»/«aún no» que
describen un estado real se reescriben, no se exceptúan), más los que §0.2 no listó y encontraron las comprobaciones
nuevas:

| Dónde                                                                                                                                                                    | Antes                                                                                                                                                                 | Ahora                                                                                                                                                                                     |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Sesiones activas (`DevicesPage.tsx`, `SessionsSummaryPanel`)                                                                                                             | «Solo se cuentan; todavía no se limita ninguna.»                                                                                                                      | Se quitó: quedan el título y las cifras por tipo                                                                                                                                          |
| Terminales nuevas (`DevicesPage.tsx`)                                                                                                                                    | «Esta computadora todavía no recibe terminales de la red.»                                                                                                            | «Esta computadora no recibe terminales de la red.»                                                                                                                                        |
| Tope de solicitudes (`DevicesPage.tsx`)                                                                                                                                  | «…en los últimos 10 minutos: por ahora no se aceptan más solicitudes.»                                                                                                | «…en los últimos 10 minutos. Espera un poco antes de pedir otro.» (§0.2 proponía «Espera unos minutos…»; así no se repite «minutos»)                                                      |
| Impresora de precuentas y de tickets, avisos de impresión (`DevicesPage.tsx`, `PaymentPoliciesForm.tsx`, `services/printProblems.ts`) y Go (`kitchen/print_problems.go`) | «…todavía no puede usarse para imprimir»                                                                                                                              | «…no se puede usar para imprimir»                                                                                                                                                         |
| Impresoras de la computadora (`DevicesPage.tsx`, `kitchen/PrintersAdmin.tsx`)                                                                                            | «Todavía no informó sus impresoras.»                                                                                                                                  | «No ha informado sus impresoras.»                                                                                                                                                         |
| Corrección de un método con el turno cerrado (`checkout/useCorrectionFlow.ts`)                                                                                           | «Usa una regularización posterior (etapa 7)…»                                                                                                                         | «Usa una regularización en Caja…» (en Caja › Movimientos se llama «Corrección o regularización»)                                                                                          |
| Cobro sin tipo de cambio (`checkout/CheckoutPanel.tsx`)                                                                                                                  | «…por ahora solo acepta pesos.»                                                                                                                                       | «…solo acepta pesos.»                                                                                                                                                                     |
| Previsualización de precios (`catalog/PreviewPage.tsx`)                                                                                                                  | «Retirar de prueba»                                                                                                                                                   | «Quitar producto» (sigue en rojo, como decidió la 9D; `stage9dStyles.test.tsx` al día)                                                                                                    |
| Registro de movimientos (`AuditPage.tsx`)                                                                                                                                | El código crudo (`device.updated`) y `entity_type` bajo el nombre de la operación                                                                                     | El nombre; el código solo si la operación no tiene nombre. El código y el «Tipo de registro» pasan a «Ver cambios», con los identificadores; las ayudas de los filtros dicen dónde verlos |
| 28 textos más del escritorio («Aún no hay…», «Todavía no hay…», «…cambios que aún no se guardan…», «La cuenta aún no tiene productos.», «Todavía no tienes PIN.», etc.)  | Los de la segunda tabla de §0.2                                                                                                                                       | Las reescrituras de §0.2 («No hay…», «…cambios sin guardar…», «La cuenta no tiene productos.», «No tienes PIN.»…)                                                                         |
| Puente nativo (Rust; §0.2 no lo revisó): `src-tauri/src/main.rs:113` y `installation.rs:299`                                                                             | «Esta terminal todavía no está conectada a un servidor.», «…la modalidad todavía no se puede cambiar desde aquí.»                                                     | Sin «todavía»                                                                                                                                                                             |
| Go, emparejamiento (`administration/pairing.go`)                                                                                                                         | «…todavía no está preparado…», «…todavía no puede emparejar terminales.», «…todavía no se aprueba…», «…todavía no termina de pedir su código…»                        | «…no está preparado…», «…no puede emparejar terminales ahora.», «…no se ha aprobado…», «…no ha terminado de pedir su código…»                                                             |
| Go, caja (`payments/cash_closing.go:423` y `:435`; la `:423` no estaba en §0.2)                                                                                          | «El turno de X aún no está cerrado.», «El turno se cerró antes de la etapa 7 y no tiene arqueo registrado.»                                                           | «El turno de X no está cerrado.», «Este turno no tiene arqueo registrado.»                                                                                                                |
| Go, otros                                                                                                                                                                | `httpapi/router.go:131`, `inventory/alerts.go:537`, `legacyimport/uploads.go:286` y `manager.go:206`, `reports/costs.go:54` («…o aún no liberada», no estaba en §0.2) | «Los avisos al momento no están disponibles en este servidor.», «…no se ha aplicado ningún conteo…», «El paquete no ha terminado de subirse.», «…o sin liberar»                           |
| Go, mensajes que no llegan a la interfaz (`config/config.go:99`, `licensing/licensing.go:10`)                                                                            | «…aún no habilitada», «…no está implementado en etapa 1»                                                                                                              | Reescritos en lugar de exceptuarlos (cambio trivial; así la lista de excepciones queda corta)                                                                                             |

Las pruebas que buscaban los textos viejos se actualizaron (Vitest: `stage9gPairing`, `stage9gPrintAgent`,
`stage9gModality`, `stage9gBackups`, `stageA1Pin`, `tablePanel`, `formsPhaseB`, `catalog`, `catalogFixes`,
`inventoryRecipes`, `printingStage9f`, `stage9dStyles`; Playwright: `workflow-stage9g-terminals`, que ahora comprueba que
Sesiones activas **no** tiene la frase, y `workflow-stage9g-modality`; Go: `kitchen/paper_cut_test.go` y
`operations/printing_stage9f_test.go`). Documentación que citaba los textos: `API_CONTRACTS.md`,
`docs/KITCHEN_AND_PRINTING.md`, `docs/NOTAS_WINDOWS.md` y `CLAUDE.md` (la descripción de Terminales y la lista de botones
rojos). Los avances de etapas anteriores no se tocan (son historia). `openapi.json` y `docs/API_V1.md` no citan ninguno
de los mensajes cambiados.

**«Código: …» de la tabla de Terminales** (`DevicesPage.tsx:873`): se queda para la Fase D, como dice el encargo (§5.3:
quitarlo y, si hace falta, «Identificador» en «Ver configuración»). La comprobación del escritorio ya busca ese patrón y
lo tiene como **única excepción temporal** escrita con su porqué; la Fase D quita la línea y la excepción (si no, la
prueba falla porque la excepción sobra). La ayuda del filtro «Código de la terminal» del Registro de movimientos ya no
remite a esa columna (ahora: «El que aparece como «Terminal» en «Ver cambios» de sus movimientos»).

### Comprobaciones automáticas

- **Escritorio** (`desktop/src/pages/plainLanguage.test.tsx`, segundo `describe` «textos sin notas de desarrollo (etapa
  A3, §1)»): recorre todas las fuentes de `desktop/src` (sin `*.test.*`, `testing/` ni `*TestData.ts`) **y las del
  puente nativo `desktop/src-tauri/src/*.rs`** (sin su módulo `#[cfg(test)]`, porque sus mensajes también llegan a la
  pantalla), sin comentarios, y busca sin distinguir mayúsculas: «todavía no», «aún no»/«aun no», «etapa», «fase X»,
  «próximamente», «provisional», «por ahora», «más adelante», «en desarrollo», «versión de prueba», «(muestra)», y dos
  patrones de identificadores a la vista (`Código: <` y un `{….id}`/`{…_id}` como texto de una etiqueta). Las
  excepciones son una lista `{ file, line, reason }` en la propia prueba: `services/inventory.ts` (`'provisional'`, el
  código de la base del costo que manda Go, nunca se muestra crudo) y la temporal de `DevicesPage.tsx` (Fase D). Una
  excepción que ya no hace falta hace fallar la prueba. El recorrido y la limpieza de comentarios se comparten con la
  prueba de la 9C (`interfaceSources`, `forEachCodeLine`), que ahora también excluye `*TestData.ts`.
- **Go** (`backend/internal/apicontract/plain_language_test.go`, `TestPlainLanguageMessages`, prueba unitaria: corre en
  `go-unit`): recorre todo `backend/internal` sin `_test.go` ni `testdata/`, lee cada cadena con `go/parser` (los
  comentarios no forman parte del árbol), y busca las mismas frases. Excepciones `{file, phrase, reason}`:
  `inventory/costing.go`, `inventory/movements.go` y `reports/costs.go` («provisional», el término del costo estimado de
  inventario y sus códigos en las consultas) y `standalone/adopt.go` («en desarrollo», mensaje de la orden `adopt-legacy`
  en la terminal de la Mac, que nombra el entorno de desarrollo). También falla si una excepción sobra, y si lee menos de
  300 archivos (para no aprobar en vacío).
- **Que detectan de verdad**: antes de las reescrituras, la prueba de Go encontró exactamente las 14 cadenas de la tabla;
  con una línea plantada en `services/backups.ts` (`'Esto todavía no funciona (etapa 9).'`) y otra en
  `src-tauri/src/vault.rs` (`"Próximamente en la etapa 10"`), la del escritorio falló con `archivo:línea (frase)`; las
  dos líneas se quitaron después (el árbol no las tiene).

### Regla en `CLAUDE.md`

Sección nueva **«Reglas permanentes desde la etapa A3»** (antes de «13–14»): ningún texto provisional o de desarrollo en
la interfaz ni en los mensajes de Go que llegan a la pantalla, qué frases, cómo decir un estado real, dónde van los
datos técnicos (Información para soporte y «Ver cambios» del Registro de movimientos), las tres comprobaciones (la del
escritorio con el puente nativo, `TestPlainLanguageMessages` y la prueba de textos de Android, citada de forma genérica:
«ver `android/CLAUDE.md`»; **el coordinador ajusta el nombre al unir**) y la regla de las excepciones (cada una con su
porqué; una frase nueva se agrega en las tres).

### Decisiones propuestas

- **Registro de movimientos**: el nombre de la operación en la columna y el código solo si no tiene nombre (como
  proponía §0.2); el código y el tipo de registro no se quitan, pasan a «Ver cambios», que ya era la parte técnica de
  cada movimiento (identificadores y estado antes/después), porque el filtro «Código de la operación» los necesita.
  `entity_type` no se tradujo a nombres: los tipos están repartidos por todos los paquetes de Go y un diccionario
  completo sería una función nueva; en «Ver cambios» se muestra como dato técnico.
- **Rust** entra en la comprobación del escritorio: §1 pide revisar la aplicación completa y el puente devuelve mensajes
  que la interfaz muestra tal cual.
- **Mensajes de Go que no llegan a la interfaz** (`config.go`, `licensing.go`): reescritos en lugar de exceptuados;
  `adopt.go` sí queda como excepción porque «en desarrollo» ahí es correcto (nombra el entorno y su orden).

### Para que decida el coordinador

1. **Textos con nota de etapa guardados por migraciones** (no son cadenas de Go ni del escritorio; ninguna comprobación
   los ve):
   - `0018_document_templates.sql:118` guarda en cada instalación la nota de la versión 1 de las 10 plantillas del
     sistema: «Formato predeterminado, igual al anterior a la etapa 9B». **Se ve en la interfaz**: Administración ›
     Documentos (`templates/KindTemplates.tsx:102`, «… · Formato predeterminado, igual al anterior a la etapa 9B») y en
     el historial del diseñador (`HistoryDialog.tsx:118`). Propuesta: en la `0027` (Fase D), una sección «Fase A» con
     `UPDATE app.document_template_versions SET note = 'Formato predeterminado' WHERE note = 'Formato predeterminado, igual al anterior a la etapa 9B';`
     (no es un dato de operación: es la nota de una versión de plantilla) y su caso en la prueba de subida del esquema.
     Sin tu visto bueno no lo hago (el plan decía que la Fase A no tenía migración).
   - `0005_kitchen_and_printing.sql:232`: los pedidos anteriores a la etapa 5 quedaron con la advertencia «Pedido
     anterior a etapa 5: revisar y liberar expresamente.» (`kitchen_releases.warnings`, que Preparación muestra en una
     liberación pendiente). Es un dato de operación de pedidos viejos (en una instalación nueva no existe); propuesta: no
     tocarlo (reescribir datos de operación pide autorización del propietario).
2. **«Código: …» de Terminales** sigue en la Fase D (con su excepción temporal en la prueba), como dice el encargo.

### Pruebas ejecutadas

Todas con el arnés `$SP/iso.sh` y `ISO_TAG=SV`, en `$SP/wt/srv`, sobre el código de los commits (la única corrección
posterior a la primera corrida, `financial.test.tsx`, es de Vitest y Vitest se repitió completo después).

| Qué                                                                                                                                | Resultado                                                                                                                                              |
| ---------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `TestPlainLanguageMessages` antes de reescribir los mensajes de Go                                                                 | Falló con las 14 cadenas esperadas (comprueba que detecta)                                                                                             |
| `go-unit` (vet normal e `integration`, `go test -race` de todo, `GOOS=windows` build y vet)                                        | Aprobado: 35 paquetes `ok` (5 sin pruebas), `TestPlainLanguageMessages` incluida                                                                       |
| `go-int ./internal/operations` con `ISO_RUN=TestRestaurantAcceptance` (contiene `9F_D4`, que comprueba el mensaje de la impresora) | `ok` (238 s)                                                                                                                                           |
| Vitest con `tsc --noEmit` (`iso.sh vitest`, suite completa), primera corrida                                                       | 925 de 926: fallaba `financial.test.tsx` (buscaba «regularización posterior \(etapa 7\)» con la expresión escapada, que mi búsqueda no vio); corregida |
| Vitest con `tsc --noEmit`, segunda corrida                                                                                         | **97 archivos, 926 pruebas aprobadas**                                                                                                                 |
| Comprobación del escritorio con frases plantadas en `services/backups.ts` y `src-tauri/src/vault.rs`                               | Falló con `archivo:línea (frase)` en ambos (comprueba que detecta, también en Rust); líneas quitadas                                                   |
| `cargo test --locked --offline` (cambiaron dos mensajes de Rust)                                                                   | 107 aprobadas, 3 ignoradas                                                                                                                             |
| Playwright Chromium: `workflow-stage9g-terminals` y `workflow-stage9g-modality` (las que buscan textos que cambiaron)              | 3 de 3                                                                                                                                                 |
| Playwright WebKit: las mismas                                                                                                      | 3 de 3                                                                                                                                                 |
| `format`, `fmt-check`, `sync-check`                                                                                                | Aprobados («Metadatos coherentes.»)                                                                                                                    |
| `md5 -q docs/etapas/*.md \| md5` antes y después de formatear                                                                      | `3555c65b1300ce044695adcc1ec19f0d` las dos veces (igual al de `$SP/etapas-md5-before.txt`)                                                             |

No ejecutado (no lo pide la fase): la regresión completa, `build`, el resto de las especificaciones de Playwright y
`go-int` de otros paquetes (ninguna otra prueba de integración compara los mensajes cambiados: las de emparejamiento
comparan códigos, y `host_unix_network_integration_test` busca «identidad» en el otro mensaje de `PAIRING_UNAVAILABLE`,
que no cambió). **No verificado**: cómo se ven los textos nuevos en pantalla (las capturas son de la Fase E).

### Commits (rama `etapa-a3-srv`, sin push ni fusión)

- `2b5c6fb` etapa A3 fase A: mensajes de Go sin frases provisionales ni notas de etapas … y la comprobación
  TestPlainLanguageMessages …
- `bad4985` etapa A3 fase A: textos del escritorio y del puente nativo sin frases provisionales, notas de etapas ni
  códigos internos a la vista … y la comprobación «textos sin notas de desarrollo» en plainLanguage.test.tsx …
- `a7208f1` etapa A3 fase A: regla permanente de textos en CLAUDE.md … y los mensajes reescritos en API_CONTRACTS.md,
  KITCHEN_AND_PRINTING.md y NOTAS_WINDOWS.md

### Decisiones del coordinador sobre la Fase A (escritorio y servidor)

- **A3-D12 · Nota de la plantilla predeterminada**: la nota «Formato predeterminado, igual al anterior a la etapa 9B»
  que la `0018` dejó en la primera versión de cada plantilla se ve en Administración › Documentos y en el historial del
  diseñador. Es configuración, no un dato de operación: la `0027` agrega una sección «Fase A» que la cambia a «Formato
  predeterminado» **solo donde el texto es exactamente ese**, con su caso en la prueba de subida del esquema. La hace el
  agente del servidor en la Fase D (donde nace la `0027`).
- **A3-D13 · Advertencia de pedidos anteriores a la etapa 5** (`0005`: «Pedido anterior a etapa 5: revisar y liberar
  expresamente.» en las liberaciones de cocina que existían al aplicarla): es un dato de operación, así que **no se
  reescribe** sin autorización del propietario (`CLAUDE.md` §0). Queda como limitación en la entrega final.
- **«Espera un poco antes de pedir otro.»** en el tope de emparejamientos: aceptado.

---

## Fase D · Terminales en el escritorio (§5), migración 0027, A3-D8 y versión 0.14.0

Agente del servidor y el escritorio, en `$SP/wt/srv` (rama `etapa-a3-srv`, desde `a7208f1`, la cabeza de la Fase A que se
fusionó en `etapa-a3` con `ac50dc9`), 19 de noviembre de 2026. Sin cambios en `android/`, `scripts/android-e2e.sh` ni en este avance. Pruebas
en copias aisladas (`ISO_TAG=SV`: clúster temporal 55499, Go temporal en 8187 y vista previa en 1427 para Playwright);
ninguna contra el servicio real. Commits: ver «Commits» al final.

### Qué se hizo

**Terminales por tipo** (`desktop/src/pages/DevicesPage.tsx`, §5.1–§5.3 y la tabla de §0.6):

- Un `android` (teléfono o tableta de meseros, `isWaiterDevice`): «Pantalla inicial» ofrece solo **Mesas** y **Para
  llevar** (`terminalHomeOptions`); un valor guardado que no le aplica (un servidor 0.13.0 que todavía tuviera «Cocina»)
  abre el editor en Mesas (`editableHome`). «Editar» ya no muestra «Caja física», «Teclado virtual» ni la nota «Con
  «Cocina» abre en Preparación…». «Ver configuración» ya no muestra la caja, el teclado, las estaciones que muestra en
  Cocina, el aviso sonoro, las impresoras de la terminal, «Impresoras de la computadora» ni la ayuda de Terminales de
  cocina. Siguen el bloqueo por inactividad, la impresora de precuentas y los avisos (A1/A2).
- Las terminales de escritorio (`desktop`) y de cocina (`kitchen`) conservan todo (las cuatro pantallas, caja, teclado,
  estaciones, aviso sonoro e impresoras).
- Tabla: la columna «Caja» de un `android` es «—» (§5.2); **ninguna fila muestra el identificador** («Código: …» se
  quitó, §5.3). «Ver configuración» de cualquier terminal termina con **«Identificador»** (el `id`), porque soporte y el
  filtro por terminal del Registro de movimientos lo necesitan («si hace falta» de §5.3). En el Registro de
  movimientos el filtro pasa a «Identificador de la terminal», con la ayuda «El que aparece en «Ver configuración» de
  Sistema › Terminales, o como «Terminal» en «Ver cambios» de sus movimientos.».
- La comprobación de textos (`plainLanguage.test.tsx`) pierde la excepción temporal de «Código» de la Fase A y gana una
  para la línea del «Identificador» de «Ver configuración», con su porqué (§5.3 lo pide ahí).

**Go** (sin ruta nueva ni capacidad nueva):

- `PATCH devices/{id}` (`administration/devices.go`): en un `android`, un `home_screen` que no sea `tables` o `takeaway`
  responde **`400 VALIDATION_ERROR` «En un teléfono o tableta de meseros la pantalla inicial es Mesas o Para llevar.»**
  sin escribir ni auditar (`androidTerminalHomeScreens`, `validAndroidHome`). Sin el campo, se conserva lo guardado,
  como siempre.
- `GET kitchen/access` (`kitchen/reads.go`, Cocina › Estaciones › Terminales de cocina) ya no lista los `android`: eran
  justo los campos que §0.6 quita a un `android` (estaciones y aviso sonoro) y su ayuda decía «elige «Cocina» como
  pantalla inicial en Sistema › Terminales», que ya no se puede. `PUT kitchen/access` no cambia. Ver «Decisiones».

**Migración** `backend/migrations/0027_android_a3_adjustments.sql`, la única de la etapa, con dos secciones:

- **«Fase A»** (A3-D12): `UPDATE` de la nota de las versiones de plantilla de «Formato predeterminado, igual al anterior a
  la etapa 9B» a «Formato predeterminado», **solo donde el texto es exactamente ese** (las diez versiones 1 de las
  plantillas del sistema que sembró la 0018). Las versiones son inmutables por el disparador
  `document_template_versions_insert_only` de la 0018, que rechaza todo `UPDATE`: la sección lo desactiva **solo para esa
  sentencia** y lo vuelve a activar enseguida, dentro de la misma transacción (`ALTER TABLE` toma un bloqueo exclusivo de
  la tabla hasta el final; si algo falla, se deshace con el disparador activo). Ver «Para el coordinador».
- **«Fase D»** (A3-D7): `UPDATE app.devices SET home_screen='tables' WHERE kind='android' AND home_screen IN
('kitchen','cash')` y `CHECK devices_android_home_check (kind <> 'android' OR home_screen IN ('tables','takeaway'))`.
- No toca roles ni permisos (A3-D2). Comentarios en español con el porqué de cada sentencia.

**A3-D8** (sin cambiar el comportamiento): `discardAccount` (`POST /restaurant/accounts/{id}/discard`, `{operation_id,
revision}` → `Bill`) y los campos `discardable` y `releasable` de `Account` (obligatorios: Go los manda en toda cuenta;
`TestContractV1` lo confirma con cada respuesta que trae una cuenta) en `backend/internal/apicontract/openapi.json` y
`docs/API_V1.md` (tabla de operaciones y sección «Descartar una cuenta vacía»). `TestContractV1` ahora usa la ruta como
parte del contrato (antes la llamaba como preparación, sin validar): la cuenta con solo borrador es `discardable` y no
`releasable`, la que tiene productos enviados responde `409 ACCOUNT_NOT_EMPTY`, y el descarte deja la cuenta `voided` y
sin borrador.

**Versión** 0.14.0 y esquema 27 en `config/product.json` con `node scripts/sync-metadata.mjs` (lo que hace `./scripts/dev.sh
sync`), más `desktop/package-lock.json` y `desktop/src-tauri/Cargo.lock` a mano (el script no los toca y `cargo --locked`
los exige al día). `min_client_versions` sin cambio (A3-D9).

**Documentación**: `API_CONTRACTS.md` (sección «Etapa A3 · Terminales por tipo de dispositivo»), `CLAUDE.md` (sección
«Terminales por tipo de dispositivo (etapa A3)» y la regla permanente «Cada tipo de terminal solo ve lo que le aplica»)
y `docs/API_V1.md`.

### Pruebas nuevas o ampliadas

- **Go, unidad**: `TestAndroidTerminalHomeIsTablesOrTakeaway` (`administration/terminal_home_test.go`).
- **Go, integración** (`administration`): `A3_android_solo_mesas_o_para_llevar` dentro de `testTerminalHome`
  (`TestStageTwoAcceptance`): un `android` nuevo inicia en Mesas; «Cocina» y «Caja» dan `400` con el mensaje exacto, sin
  cambiar ni auditar; Para llevar y Mesas se aceptan y se auditan; una edición sin el campo conserva la guardada; la base
  rechaza Cocina y Caja con `devices_android_home_check`; `GET kitchen/access` lista la terminal de escritorio y no el
  `android`.
- **Go, subida del esquema** (`database`): `TestStageA3Upgrade`, de 26 a 27 con datos de la A1 y la A2 más un `android`
  con cada pantalla, una terminal de escritorio con Caja, una de cocina con Cocina y una plantilla propia con dos notas
  parecidas pero no exactas: ninguna fila de ninguna tabla cambia salvo las dos columnas esperadas; solo las diez notas
  exactas cambian; el disparador queda activo (`tgenabled = 'O'`) y un `UPDATE`/`DELETE` de una versión se sigue
  rechazando; los dos `android` con Cocina y Caja pasan a Mesas y los demás conservan la suya; el `CHECK` rechaza Cocina y
  Caja en un `android` (también al insertar) y no toca escritorio ni cocina. La instantánea compartida de las pruebas de
  la A1 y la A2 (`stageA1Snapshot`) excluye ahora la nota de `document_template_versions` (la 0027 la reescribe a
  propósito y las dos pruebas suben hasta el esquema vigente), como ya excluía el permiso de la 0025.
- **Contrato**: `TestContractV1` con `discardAccount` (arriba).
- **Vitest**: `pages/stageA3Terminals.test.tsx` (6 pruebas: opciones del `android` y lo que envía, un `android` con Cocina
  guardada abre en Mesas, escritorio y cocina con las cuatro, «—» y sin identificador en la tabla, «Ver configuración»
  del `android` y del escritorio).
- **Playwright**: `workflow-stagea2-terminals.spec.ts` ampliada con los pasos 6 a 8 (mismo teléfono emparejado, para no
  pedir otro código: límite de 5 por origen cada 10 minutos): el escritorio conserva las cuatro pantallas y la caja; en
  la tabla la caja del teléfono es «—» y no hay «Código» ni su identificador; el teléfono ofrece solo Mesas y Para
  llevar, sin caja ni teclado, y Go guarda Para llevar; un `PATCH` con Cocina da `400` con el mensaje; «Ver
  configuración» dice el identificador y no la caja ni las estaciones. No hizo falta una especificación nueva.

### Decisiones propuestas

- **La caja y el teclado de un `android` se envían sin cambio** en lugar de `null` (lo que proponía §0.6): con `null`,
  editar un teléfono que tuviera una caja con un turno abierto daría `409 SHIFT_OPEN` y no se podría guardar nada durante
  el servicio. Go sigue sin rechazar esos campos en un `android` (un escritorio 0.13.0 los manda).
- **Terminales de cocina sin los `android`** (`GET kitchen/access`): aplica la fila de §0.6 (estaciones y aviso sonoro:
  «No» en un `android») en la otra pantalla donde se editan, y quita una ayuda que remitía a una opción que ya no existe.
  Es una línea de SQL, compatible (la lista solo tiene menos elementos), con su prueba de integración. Si prefieres no
  tocar Cocina, se revierte sin afectar lo demás.
- **«Identificador» en «Ver configuración»**: §5.3 lo deja a criterio («si hace falta»); hace falta para el filtro del
  Registro de movimientos y para soporte.

### Para el coordinador

- **A3-D12 y la inmutabilidad de las versiones**: el `UPDATE` que decidiste choca con el disparador de la 0018 que hace
  inmutables las versiones de plantilla. Lo resolví desactivándolo solo para esa sentencia dentro de la migración (con
  la prueba de que vuelve a quedar activo y sigue rechazando cambios). Requiere que quien migra sea el dueño de la
  tabla, como en el clúster aislado. En la instalación real debería serlo: `standalone` restaura con `--no-owner
--role=<usuario de la aplicación>` (`standalone/backup.go:171`) y la 0026 ya hizo `ALTER TABLE app.devices` con ese
  rol al instalar el `.pkg` 0.13.0; pero para esta tabla **no está verificado** en la instalación real (se verá cuando
  el `host` aplique la 0027 con el `.pkg` 0.14.0, después de su respaldo `…-antes-de-migrar.dump`; si fallara, la
  migración se deshace entera, porque el migrador aplica todo en una sola transacción). Si prefieres no tocar la inmutabilidad,
  la alternativa es quitar la sección «Fase A» y dejar la nota como limitación (como A3-D13).

### Pruebas ejecutadas

Todas con el arnés `$SP/iso.sh` y `ISO_TAG=SV`, en `$SP/wt/srv`, sobre el código de los commits (después de las corridas
solo se escribió este resumen).

| Qué                                                                                                                                                                                 | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `go-int ./internal/database` (todas las pruebas de subida del esquema, con la 0027)                                                                                                 | `ok` (18 s); `TestStageA3Upgrade` sola: `ok`                                                                                                                                                                                                                                                                                                                                                                                                     |
| `go-int ./internal/apicontract` (`TestContractV1` con `discardAccount` y `discardable`/`releasable` obligatorios en `Account`)                                                      | `ok` (40 s)                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `go-int ./internal/administration`, completo                                                                                                                                        | Dos corridas: todo aprobado **salvo `TestStageA1WaiterPIN`**, que falló las dos veces por tiempo con la Mac cargada (carga media de 55 y de 17; dos emuladores y Android Studio del otro agente): la primera con `context deadline exceeded` (su plazo es de 3 minutos) y la segunda con un `503` en `PUT auth/pin` dentro de su prueba de bloqueo largo y las siguientes en cascada. Esa prueba no envía `home_screen` ni lee `kitchen/access`. |
| `go-int ./internal/administration` con `ISO_RUN=TestStageA1WaiterPIN`, sola, con carga 8–15                                                                                         | `ok` (126 s)                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `go-int ./internal/operations` con `ISO_RUN=TestStage9FKitchenStations` (lee `GET kitchen/access`)                                                                                  | `ok` (18 s)                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `go-unit` (vet normal e `integration`, `go test -race` de todo, `GOOS=windows` build y vet)                                                                                         | Aprobado: 35 paquetes `ok`                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Vitest con `tsc --noEmit` (suite completa)                                                                                                                                          | **98 archivos, 932 pruebas aprobadas** (6 nuevas en `stageA3Terminals.test.tsx`)                                                                                                                                                                                                                                                                                                                                                                 |
| `cargo test --locked --offline` (versión 0.14.0 en `Cargo.toml` y `Cargo.lock`)                                                                                                     | 107 aprobadas, 3 ignoradas                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Playwright Chromium: `workflow-stagea2-terminals` sola (con los pasos nuevos)                                                                                                       | 1 de 1                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Playwright Chromium: `workflow-stage9f-kitchen-stations`, `workflow-stage9g-print-agent`, `workflow-stage9g-terminals` y `workflow-stagea1-pin` (Terminales y Terminales de cocina) | 5 de 5                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Playwright WebKit: las mismas dos corridas                                                                                                                                          | 1 de 1 y 5 de 5                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `format`, `fmt-check`, `sync-check`                                                                                                                                                 | Aprobados («Metadatos coherentes.»)                                                                                                                                                                                                                                                                                                                                                                                                              |
| `md5 -q docs/etapas/*.md \| md5` antes y después de formatear                                                                                                                       | `3555c65b1300ce044695adcc1ec19f0d` las dos veces                                                                                                                                                                                                                                                                                                                                                                                                 |

La especificación de la A2 se corrió sola, en su propia corrida (Go y base nuevos), para no ser la sexta solicitud de
emparejamiento desde `127.0.0.1` en 10 minutos (en la suite completa sigue esperando el límite, como en la A2). **No
ejecutado** (no lo pide la fase): la regresión completa de Playwright, `build`, `go-int` de los demás paquetes y Android.
**No verificado**: la 0027 en la instalación real (ver «Para el coordinador») y cómo se ven las pantallas (capturas de la
Fase E).

### Commits (rama `etapa-a3-srv`, sin push ni fusión)

- `838562d` etapa A3 fase D: migración 0027 (sección «Fase A» … A3-D12 …; sección «Fase D» … A3-D7), versión 0.14.0 y
  esquema 27 con metadatos sincronizados (min_client_versions sin cambio) y TestStageA3Upgrade
- `3d7f9b0` etapa A3 fase D: PATCH devices/{id} rechaza en un android una pantalla inicial que no sea Mesas o Para llevar
  (400 con mensaje claro) y Terminales de cocina (GET kitchen/access) sin los android, con sus pruebas
- `2d3c7a7` etapa A3 fase D: discardAccount y los campos discardable y releasable de Account en el contrato v1
  (openapi.json, docs/API_V1.md y TestContractV1), sin cambiar el comportamiento (A3-D8)
- `6460a9f` etapa A3 fase D: Terminales por tipo de dispositivo (…), con stageA3Terminals.test.tsx y los pasos nuevos de
  workflow-stagea2-terminals
- `7e9a957` etapa A3 fase D: Terminales por tipo de dispositivo, la 0027 y discardAccount en CLAUDE.md (con la regla
  permanente «Cada tipo de terminal solo ve lo que le aplica») y API_CONTRACTS.md

### Decisiones del coordinador sobre la Fase D

- **A3-D12 (forma final)**: la sección «Fase A» de la `0027` desactiva el disparador de inmutabilidad
  `document_template_versions_insert_only` solo para su `UPDATE` y lo vuelve a activar en la misma transacción
  (`TestStageA3Upgrade` comprueba que queda activo y que sigue rechazando `UPDATE` y `DELETE`). Exige que el rol que
  migra sea dueño de la tabla. En la instalación real lo es: la base del servicio se restauró con `pg_restore --no-owner
--no-acl --role=<usuario de la aplicación>` (`internal/standalone/backup.go:170-171`, usado por `adopt-legacy`), así
  que todas las tablas son de ese rol, y la `0026` ya hizo `ALTER TABLE app.devices` con él en la instalación real.
- Aceptadas las tres decisiones del agente: el escritorio envía sin cambio la caja y el teclado guardados de un
  `android` (con `null`, una caja con turno abierto daría `SHIFT_OPEN`); Cocina › Estaciones › Terminales de cocina ya no
  lista los `android` (§5.1: cada tipo solo ve lo que le aplica); `workflow-stagea2-terminals` se amplió en lugar de una
  especificación nueva (un emparejamiento menos, por el límite de 5 cada 10 minutos).
- `TestStageA1WaiterPIN` falló por tiempo en las dos corridas completas de `administration` con la Mac muy cargada por
  los emuladores del otro agente (carga 17–55) y pasó sola: se vigila en la regresión de la Fase E.

---

## Fase A · Android (regla de textos y conexión tolerante)

Agente de Android en `$SP/wt/and` (rama `etapa-a3-and`, desde `etapa-a3` = `4c7231b`). Sin cambios en `backend/`,
`desktop/`, `CLAUDE.md` de la raíz, `docs/ETAPA_A3_AVANCE.md` ni el contrato. Pruebas en copias aisladas: Go temporal
`ISO_TAG=AN` (8193/8453, base `adel_pos_test_an_…`), `Pixel_Tablet` (5580) y `ADEL_A1_Phone_API33` (5582) de solo
lectura, siempre con `AdelTestRunner` y emparejando por la dirección escrita `10.0.2.2:8453`; nunca la red real.

### Qué se hizo

**Conexión tolerante (A3-D1, §2).**

- **La causa, corregida en el cliente** (`core/data/…/api/PinnedHttpClient.kt`): el pool guarda las conexiones libres
  **30 s** (`IDLE_CONNECTION_SECONDS`, con su porqué: Go las cierra a los 60 s y OkHttp no revisa una conexión
  reutilizada antes de un `GET`). Un interceptor único, `ServerAnswersInterceptor` (`core/data/…/api/`), **repite una
  vez** con una conexión nueva (suelta antes las libres del pool) una lectura que falló sin respuesta; nunca una
  escritura (llevan su `operation_id`), ni un canal (`Accept: text/event-stream`: su protocolo no cambia), ni un plazo
  vencido (duplicaría la espera), ni otro certificado. `retryOnConnectionFailure` sigue en `false`. Go no cambia.
- **El estado único** `ServerLink` (`core/domain/…/connection/ServerLink.kt`) con sus reglas puras en
  `ServerLinkTracker` (reloj inyectado) y su implementación `ServerLinkMonitor` (`core/data/…/network/`), **una
  instancia en `AppContainer.serverLink`** (arranca en `AdelPosApplication`). Lo alimentan la red de Android
  (`ConnectionMonitor`: sin red, «Sin conexión» al instante) y **cada respuesta de Go** del cliente del servidor
  emparejado (`PairedConnection` crea su cliente con el interceptor, que informa `answered`/`noAnswer`/`repeated`:
  entrada, catálogo e imágenes, modo Mesero, canal de eventos y canal de avisos pasan por él). Cualquier respuesta HTTP
  es un éxito; una cancelación propia no cuenta.
- **Tolerancia y revisión** (constantes con su porqué en `ServerLinkTracker`): «Sin conexión» con **3 fallas seguidas
  y 20 s desde la primera sin ningún éxito**; «En línea» con el primer éxito; al volver la red, sigue «Sin conexión»
  hasta que Go responda y se sondea enseguida. Sonda `GET health/live` cada **5 s** mientras duda, cada **10 s** con
  «Sin conexión» (buscando el servidor en otra dirección, como el cambio de IP), **ninguna** con el canal de eventos
  abierto, cada **60 s** sin él (se salta si hubo otro éxito en ese lapso) y nada en segundo plano. Sin servidor
  emparejado el estado sigue a la red de Android. La comprobación de versiones y lista (`AccessViewModel`) pasa de 60 s
  a **5 minutos** (y al volver la conexión); la lista de «¿Quién eres?» de 30 s a **60 s** (y al cambiar el estado).
- **Reintento en el momento** (`ServerLink.confirm`): con «Sin conexión» y red de Android, la entrada con PIN, la
  lista, la comprobación y la precuenta preguntan a Go en ese momento (sonda con plazo de conexión de 2 s, sin buscar en
  otra dirección) antes de decir que falta conexión.
- **Lo leen todos** (nadie decide «sin conexión» por su cuenta):
  - la barra (`ShellViewModel`);
  - la entrada (`AccessRepository`): se quitó el `online` de `DeviceStatus.Ready` y `markOffline`; la lista sin
    conexión y la entrada **directa** sin conexión solo con «Sin conexión» tras la sonda del momento; si Go no
    responde al PIN, sigue el verificador local como antes (A1-D1); una falla aislada ya no deja al mesero sin sesión
    de Go;
  - «¿Quién eres?» (`PinEntryScreen`): el aviso «Sin conexión…» sale del estado único; sin la lista de Go y con «En
    línea», un indicador de espera (nunca «Sin conexión») y se vuelve a pedir a los 5 s;
  - Mesas y Para llevar (`TablesScreen`, `TakeawayScreen`): «Sin conexión…» solo con el estado único; una lectura sin
    respuesta con «En línea» se repite sola a los 5 s (`RestaurantRepository`) y al volver la conexión se vuelve a leer;
  - la precuenta (`PrecheckRepository.connected` y `confirmConnection`) y «Entregado»: sin respuesta con «En línea»
    dicen «No se pudo imprimir…» / «No se pudo marcar entregado…» (`PrintAnswer.NoAnswer`), nunca «necesitas
    conexión» con la barra en «En línea»;
  - la notificación permanente del canal de avisos (`ReadyOngoing.of(canal, estado)`): «Sin conexión…» solo con el
    estado único; el protocolo del canal (cursor, `Last-Event-ID`, espera creciente, A2-D12) no cambia.
- **No cambian**: el canal de eventos, el canal de avisos, la regla de la cola sin conexión (regla 18, WorkManager sin
  restricción de red, sus disparadores siguen con la red de Android) y la entrada sin conexión (A1-D1, A1-D16).

**Textos (§1, A3-D11, A3-D10).** Las filas de Android de §0.2: «%1$s no está conectada…», «No hay meseros con
PIN…», «Estos pedidos no se han enviado…», «No hay mesas guardadas…», «Sin conexión: no hay pedidos para llevar
guardados…», «Este dispositivo no tiene impresora asignada.» y, en la bitácora, «(descartado, falta quitarlo de la
cuenta)». Se quitó la pantalla de muestra `CatalogSample*` con sus textos y sus dos pruebas. La comprobación
automática es `PlainLanguageTest` (`core/ui/src/test/…/PlainLanguageTest.kt`): recorre los recursos de las carpetas
`values` de **todos** los módulos (cada `<string>`, `<plurals>` y `<string-array>`, sin comentarios XML) y las cadenas
literales de Kotlin de `src/main` (con un lector propio que quita comentarios, también anidados, y plantillas `${…}`;
sin las líneas de `require`, `check`, `error(`, `Log.` y `…Exception(`) y falla con «todavía no», «aún no», «etapa»,
«fase X», «próximamente», «provisional», «por ahora», «más adelante», «en desarrollo», «versión de prueba» y
«(muestra)». Excepciones: una lista `allowed` con archivo, patrón y porqué (hoy vacía); una excepción que ya no
encuentra nada también falla. Una segunda prueba comprueba el lector (comentarios ignorados, textos encontrados).

**Reglas** (`android/CLAUDE.md`): regla 2 ampliada con la regla de textos y su prueba; regla 13 con el estado único,
la tolerancia, la revisión, el reintento en el momento y la causa; «Estructura», «Pruebas» (las nuevas, la suite
`connection`, los servidores falsos en HTTP/1.1), «Precuenta», «Para llevar» y «Aviso de platillos listos» al día.
`docs/ANDROID_INSTALAR.md` fuera de su §4: «No hay meseros con PIN» (§3), la tolerancia en §5 y la notificación en §8.

### Archivos principales

- `android/core/domain/src/main/kotlin/com/adelpos/meseros/core/domain/connection/ServerLink.kt` y
  `ServerLinkTracker.kt` (reglas y constantes).
- `android/core/data/src/main/kotlin/com/adelpos/meseros/core/data/api/PinnedHttpClient.kt`,
  `ServerAnswersInterceptor.kt`; `…/network/ServerLinkMonitor.kt`; `…/access/AccessRepository.kt`,
  `PairedConnection.kt` (`live`); `…/waiter/RestaurantRepository.kt`, `PrecheckRepository.kt`;
  `…/ready/ReadyAlertsChannel.kt` (`ReadyOngoing.of`).
- `android/app/src/main/kotlin/com/adelpos/meseros/AppContainer.kt` (`serverLink`), `ShellViewModel.kt`,
  `WaiterModeContainer.kt`, `ready/ReadyAlertsController.kt`.
- `android/feature/access/…/AccessViewModel.kt`, `pin/PinEntryScreen.kt`; `android/feature/waiter/…/tables/`;
  `android/feature/takeaway/…/TakeawayScreen.kt`, `TakeawayViewModel.kt`; `android/core/capture/…/CaptureDependencies.kt`
  (`link`).
- Pruebas: `ServerLinkTrackerTest`, `PinnedHttpClientTest`, `AccessRepositoryTest` (3 casos nuevos), `TestServerLink`,
  `PlainLanguageTest`, `ServerLinkScreensTest`, `ConnectionOnGoTest`; `scripts/android-e2e.sh --suite connection`.

### Decisiones propuestas

- **Decisión propuesta · Entrada con PIN ante una falla de `pin-login`**: con el estado «En línea», si Go no responde al
  PIN se sigue con el verificador local (entrada sin conexión), como en la A1 (A1-D1: el mesero no espera). Lo que la
  A3 quita es la entrada **directa** sin conexión por una falla aislada anterior. Alternativa no tomada: mostrar «No se
  pudo conectar…» hasta que el estado diga «Sin conexión» (hasta 20 s sin poder entrar).
- **Decisión propuesta · Sin respuesta tras intentar con «En línea»**: la precuenta y «Entregado» dicen «No se pudo…
  Inténtalo de nuevo» en lugar de «necesitas conexión» (que solo sale con «Sin conexión»): así nunca contradicen a la
  barra. Es la única diferencia visible en esos dos textos.
- **Decisión propuesta · La comprobación de texto va en `core:ui`** (junto a `ColorsOnlyInPaletteTest`, que ya recorre
  todos los módulos con sus entradas declaradas en Gradle) y no en `app` como proponía §0.2: `app` no tiene pruebas de
  la JVM ni esas entradas.
- **Decisión propuesta · Los servidores falsos de las pruebas de la JVM hablan HTTP/1.1 sin ALPN**
  (`protocolNegotiationEnabled = false` en `FakeGoServer` y `PinnedHttpClientTest`), como la escucha de red de Go: con
  HTTP/2 (lo que MockWebServer negocia por omisión con TLS) una conexión cerrada por el servidor se ve como un plazo
  vencido y no como «unexpected end of stream».
- **Sin cambio (observación de §0.2)**: «Buscar el servidor» y «servidor del restaurante» se quedan; no los pide §1 y
  el coordinador no lo decidió. Los textos de la Fase C («Ingresar», «Selecciona tu usuario»…) no se tocaron.

### Commits (rama `etapa-a3-and`, desde `4c7231b`)

| Commit    | Título                                                                                                      |
| --------- | ----------------------------------------------------------------------------------------------------------- |
| `3f22e55` | etapa A3 fase A (Android): se quita la pantalla de muestra del catálogo (A3-D10)                            |
| `365e74f` | etapa A3 fase A (Android): textos sin notas de desarrollo y su comprobación automática (§1, A3-D11)         |
| `458d38d` | etapa A3 fase A (Android): estado único de conexión y punto único del cliente (A3-D1)                       |
| `fe4ff51` | etapa A3 fase A (Android): todos leen el estado único de conexión (A3-D1)                                   |
| `52a0b42` | etapa A3 fase A (Android): ConnectionOnGoTest y scripts/android-e2e.sh --suite connection (§6)              |
| `93b767c` | etapa A3 fase A (Android): android/CLAUDE.md y la guía con la regla de textos y el estado único de conexión |

### Pruebas (lo que realmente se ejecutó)

| Qué                                                                                                                       | Dónde                                                                                                                                      | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                             |
| ------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| JVM y Robolectric SDK 36: `:core:domain:test testDebugUnitTest`                                                           | Mac, `iso.sh wt/and gradle` (JBR de Android Studio)                                                                                        | **369/369**: `core:domain` 162 (`ServerLinkTrackerTest` 14 nuevas), `core:data` 178 (`PinnedHttpClientTest` 6 nuevas, `AccessRepositoryTest` 16 con 3 nuevas, `AccessSafetyTest` 13, `PrecheckRepositoryTest` 5, `ReadyAlertsChannelTest` 18), `core:ui` 12 (`PlainLanguageTest` 2 nuevas), `feature:access` 7, `feature:takeaway` 5, `feature:waiter` 5                                                                              |
| Android Lint (`lintDebug`, los 7 módulos) y `ktfmtCheck`                                                                  | Mac                                                                                                                                        | Sin problemas                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `fmt-check` del repositorio (Prettier, gofmt, cargo fmt) y md5 de `docs/etapas/*.md`                                      | Mac                                                                                                                                        | Sin cambios pendientes; especificaciones intactas (`3555c65b…` antes y después)                                                                                                                                                                                                                                                                                                                                                       |
| Instrumentadas `:app:connectedDebugAndroidTest :core:data:connectedDebugAndroidTest`                                      | `Pixel_Tablet` (Android 17, 5580), con las aplicaciones de prueba desinstaladas antes                                                      | **1 XML por módulo**. `app`: 106 pruebas, **37 corridas, 0 fallas**, 69 omitidas (las `*OnGoTest`, sin argumentos); entre ellas `ServerLinkScreensTest` 3, `AccessScreensTest` 5, `AppShellTest` 2, `AppNavigationTest` 4, `ConnectionOnDeviceTest` 1 (modo avión real), `ReadyAlertsOnDeviceTest` 6, `WaiterScreensTest` 4, `TakeawayScreensTest` 4, `PrecheckScreensTest` 3. `core:data`: 20, **18 corridas, 0 fallas**, 2 omitidas |
| Las mismas                                                                                                                | `ADEL_A1_Phone_API33` (Android 13, 5582)                                                                                                   | **1 XML por módulo**. `app`: 106, **37 corridas, 0 fallas**, 69 omitidas; `core:data`: 20, **18 corridas, 0 fallas**, 2 omitidas                                                                                                                                                                                                                                                                                                      |
| `ConnectionOnGoTest` de **10 minutos** (`scripts/android-e2e.sh --suite connection`, `ADELPOS_E2E_CONNECTION_MINUTES=10`) | `Pixel_Tablet` solo (el teléfono apagado), Go temporal `ISO_TAG=AN` (8193/8453), Mac sin otra carga, avisos de platillos listos encendidos | **APROBADA**: «0 de 557 s con «Sin conexión» a la vista; sin respuesta 0, lecturas repetidas 0, respuestas de Go 60; bloqueos 3, entradas 3 (sin sesión 0)». Bloqueos a los 120 s exactos y cada vuelta con sesión de Go (registro en `$SP/and-e2e-connection/`)                                                                                                                                                                      |

Comparación con la Fase 0 (corrida C, mismo guion, sin arreglo): 12 fallas y el aviso falso en 4 de 5 bloqueos; ahora
0 fallas, 0 repeticiones y 0 segundos con aviso. Las 60 respuestas de Go en 10 minutos (una cada 10 s en promedio,
contando las lecturas al volver a entrar, la lista de «¿Quién eres?» y el canal de avisos) muestran la revisión
espaciada.

### No verificado

- La hora sin uso de §6 (60 minutos): la hace la Fase E con la misma suite (`ADELPOS_E2E_CONNECTION_MINUTES=60`).
- `ConnectionOnGoTest` solo en `Pixel_Tablet` (lo pedido); no en el teléfono ni en Android 8 (`ADEL_A1_Tablet_API26`,
  Fase E).
- Las demás suites de punta a punta (`access`, `catalog`, `waiter`, `field`, `takeaway`, `precheck`, `alerts`) no se
  volvieron a correr en esta fase; cambian por esta fase: `AccessOnGoTest` paso 1 (ahora comprueba el estado único) y
  los tiempos con un servidor que no responde (la tolerancia de 20 s). Quedan para la regresión de la Fase E.
- `core:ui` instrumentada no se corrió (su código no cambió; `PlainLanguageTest` es de la JVM).
- Un equipo físico y una wifi real (pérdida de paquetes, cambio de punto de acceso): la tolerancia lo cubre en teoría.
- Observación: en `ConnectionOnGoTest` el estado arrancó en «Sin conexión» y pasó a «En línea» 2 s después de abrir el
  proceso (`AdelLink`, antes de emparejar): `ConnectivityManager.activeNetwork` dio null al crear el estado. Antes de la
  A3 la barra también tomaba ese primer valor; ahora sigue en «Sin conexión» hasta la primera respuesta de Go (o hasta
  la red, sin servidor emparejado). No se investigó más.

### Para el coordinador

- Unir el texto propuesto de arriba a `CLAUDE.md` de la raíz y este resumen al avance.
- Decidir (opcional) si «Buscar el servidor» / «servidor del restaurante» se unifican a «computadora principal»
  (observación de §0.2 sin cambio).
- Nada del servidor hizo falta en esta fase.

### Decisiones del coordinador sobre la Fase A (Android)

- **A3-D14**: se aceptan las cuatro decisiones propuestas del agente (con «En línea», un PIN sin respuesta de Go sigue
  con el verificador local de A1-D1; «No se pudo… Inténtalo de nuevo» en la precuenta y «Entregado» sin respuesta;
  `PlainLanguageTest` en `core:ui`; servidores falsos con HTTP/1.1 sin ALPN, como Go).
- **A3-D15 · «Buscar el servidor» y «Servidor del restaurante» se quedan**: son nombres reales y vigentes del producto
  (la pantalla «Buscar el servidor» y la modalidad «Servidor del restaurante» del escritorio y del instalador, 9G), no
  textos provisionales; §1 no los pide y unificarlos sería un cambio de terminología fuera de esta etapa.
- **Estado al arrancar**: el agente vio que, al crearse el proceso, el estado empezaba en «Sin conexión» unos 2 s porque
  Android todavía no había informado ninguna red. Un estado aún no conocido no debe mostrar «Sin conexión» (§2: el
  indicador y los avisos nunca dicen algo falso): se corrige al empezar la Fase C (el estado inicial es «dudando», que
  se ve «En línea», hasta la primera respuesta de Go, el primer aviso de red de Android o el plazo de la tolerancia).
- El texto para `CLAUDE.md` de la raíz sobre Android (y el nombre `PlainLanguageTest` en la regla de la A3) se une en
  la Fase E, para no pisar los cambios del agente del servidor en la Fase B.

---

## Fase B · Permisos agrupados y roles de fábrica (§3)

Agente del servidor y el escritorio, en `$SP/wt/srv` (rama `etapa-a3-srv`, sobre la Fase D que el coordinador fusionó en
`etapa-a3` con `699cb3b`), 19 de noviembre de 2026. Sin migración (A3-D2: todo vive en Go; la `0027` no se tocó). Sin
cambios en `android/` ni en este avance. Pruebas en copias aisladas (`ISO_TAG=SV`: clúster temporal 55499, Go temporal en
8187 y vista previa en 1427); ninguna contra el servicio real. Commits: ver «Commits» al final.

### Qué se hizo

**Go: el catálogo de permisos en un solo lugar** (A3-D4/D5).

- `backend/internal/administration/permission_catalog.go`: los diez grupos de §3.2 en su orden, las **once capacidades**
  de la vista sencilla (las nueve de la especificación más «Manejar inventario» y «Autorizar y corregir», A3-D5), los
  **conjuntos de fábrica** de los seis roles (`factoryRolePermissions`, A3-D2 y A3-D3, los del anexo de la Fase 0 sin
  cambios: Mesero 14, Cajero 36, Cocina 8, Supervisor 89 —88 desde la revisión R4, A3-D19—, Inventarios 20, Finanzas 28) y las funciones que arman la
  respuesta (`buildPermissionCatalog`), cierran un conjunto bajo sus dependencias (`closePermissions`) y dan el conjunto
  de un rol (`factoryPermissions`, `FactoryRolePermissions` para las pruebas de otros paquetes).
- `permission_catalog_entries.go`: los **156 permisos**, cada uno con su grupo, su **descripción corta** en lenguaje
  claro (una frase, redactadas en esta fase), **lo que necesita** (`requires`, las 138 dependencias de la Fase 0) y, solo
  en dos, un nombre visible que corrige el del módulo: `events.read` («Ver al momento los cambios de otras terminales»,
  antes «Recibir eventos autenticados») y `kitchen.kds.ack` («Confirmar que la cocina recibió la comanda», antes con la
  sigla KDS, §0.2). Los módulos siguen registrando sus claves y su nombre (`Permissions`); el nombre registrado no se tocó
  porque las pruebas comparan el de Go con el que sembraron las migraciones.
- **`GET permissions/catalog`** (`permissions.read`, `routes.go`) con `{groups, permissions, capabilities,
factory_roles}`. `GET permissions` no cambia.
- **Capacidad `permission-groups-v1`** en `GET version` (`httpapi/router.go`, con su caso en `clientversion_test.go`).
- **Asistente** (`setup.go`): cada rol de fábrica recibe su conjunto en una instalación nueva (sustituye lo que concedía
  hasta la A2, que ya estaba dentro de cada conjunto). El Administrador sigue con todos. Una instalación existente no
  cambia.
- **«Restablecer permisos predeterminados»** (`role_defaults.go`, A3-D6): `GET roles/{id}/defaults` (vista previa:
  `current`, `factory`, `add`, `remove`) y `POST roles/{id}/defaults` con `{expected}`; reglas en «Para el revisor».
- **Puente de Rust** (`api.rs`): `permission_catalog`, `role_defaults` y `restore_role_defaults` con su prueba
  `stage_a3_permission_routes_are_closed`, en el formato que lee `e2e/native-driver.ts` (el driver las tomó solo).

**Escritorio** (`pages/RolesPage.tsx`, `components/PermissionEditor.tsx`, `services/permissionCatalog.ts`):

- Pestaña inicial **«Vista sencilla»**: una casilla por capacidad que marca su conjunto con lo que necesita, o lo quita
  si está completa (sin quitar lo que pertenece a otra capacidad completa); parcial con `aria-checked="mixed"`, la casilla
  indeterminada y «Parcial: 1 de 9».
- **«Vista detallada»**: un `Accordion` por grupo con «5 de 12» en el resumen, «Marcar todo el grupo» y «Quitar todo el
  grupo», el buscador por nombre y descripción (sin acentos ni mayúsculas; abre las secciones con coincidencias) y la
  descripción corta de cada permiso.
- **Dependencias** leídas del catálogo: marcar agrega lo que hace falta y lo dice («También se marcó lo que necesita: …»);
  quitar avisa qué dejará de funcionar («Sin lo que quitaste dejarán de funcionar: …») y no quita nada más (§3.2, A3-D4).
- **«Restablecer permisos predeterminados»** en el editor, solo para los roles de `factory_roles` (no el Administrador ni
  los del usuario): vista previa de Go, `ConfirmDialog` con lo que se agregará y lo que se quitará con sus nombres (y que
  no cambian el límite de descuento ni los permisos propios de cada usuario; si hay cambios sin guardar, que se pierden),
  confirmación con `expected` = lo que mostró la vista previa, aviso y la selección del editor al día; con `ROLE_CHANGED`,
  el mensaje de Go.
- Registro de movimientos: «Permisos predeterminados del rol restablecidos» (`AuditPage.tsx`).
- Sin `permission-groups-v1`, la lista de siempre (`PermissionChecklist` y `GET permissions`).

**Documentación**: `docs/ANDROID_INSTALAR.md` §4 («Permisos del rol Mesero»: ya viene todo marcado; lo opcional,
«Modificar» y «Cancelar unidades», y cómo darlo en la vista detallada; la pantalla nueva; y «Restablecer» en una
instalación existente), `API_CONTRACTS.md` (sección «Etapa A3 · permisos agrupados y roles de fábrica»), `CLAUDE.md`
(composición del backend, seguridad, «Permisos agrupados y roles de fábrica (etapa A3)» y la regla permanente «El
catálogo de permisos está en un solo lugar») y `docs/API_V1.md` (la capacidad nueva en la lista).

### Decisiones propuestas

- **Sin `operation_id` en la confirmación**: `POST roles/{id}/defaults` lleva solo `{expected}`, como `PATCH
roles/{id}` (que tampoco lo lleva). El reintento es seguro igual: si el rol ya tiene exactamente sus permisos de
  fábrica, responde `200` con `changed: false` sin escribir ni auditar (el patrón de `users/{id}/kitchen-stations` de la
  9F); si no y los permisos ya no son los mostrados, `409 ROLE_CHANGED`.
- **Orden de las comprobaciones**: primero si el rol tiene conjunto (un rol del usuario o el Administrador responden
  `400` aunque sea el rol propio de quien pide), después el rol propio y la delegación.
- **`permissions.read` además de `roles.manage` y `permissions.manage`** en las dos rutas (la vista previa enumera
  permisos y la pantalla ya pedía los tres para editar). Es más estricto que `PATCH`, nunca menos.
- **Capacidades al quitarlas**: se quitan sus permisos salvo los que pertenecen a otra capacidad completa; lo que
  necesitaba lo quitado se avisa, no se quita (igual que en la vista detallada).
- **Auditoría**: antes, el rol (como `role.updated`); después, `{role, added, removed}` para que el Registro diga qué
  cambió.
- **Prueba de la Fase A en la A1**: el armado del servidor de prueba de `TestStageA1WaiterPIN` pasó a
  `newPinEnvironment` (mismo código, sin cambios de comportamiento) para que lo use también la prueba nueva, sin copiarlo.

### Para el revisor

Revisión independiente de §3 (roles de fábrica y restablecimiento; A3-D2 y A3-D6). Archivos:

- `backend/internal/administration/permission_catalog.go` (conjuntos de fábrica, capacidades, grupos) y
  `permission_catalog_entries.go` (grupo, descripción y `requires` de cada permiso).
- `backend/internal/administration/role_defaults.go` (las dos rutas), `roles.go` (el despacho a `roleDefaults` antes de
  `roles.manage`), `routes.go` (`permissions/catalog`), `setup.go` (concesión en el asistente).
- Comparar con `roles.go` (`PATCH roles/{id}`) y `delegation.go` (`requireWithin`, `lockAdministration`,
  `preserveAdministration`, `requireLimitWithin`, `fullAdministrators`).
- Pruebas: `permission_catalog_test.go` (unidad) y `role_defaults_acceptance_test.go` (`TestStageA3FactoryRoles`).
- Escritorio: `desktop/src/pages/RolesPage.tsx` (`useRoleDefaults`, que envía `expected`) y
  `services/permissionCatalog.ts` (reglas de la pantalla; no deciden nada de seguridad).

Reglas de seguridad implementadas en `role_defaults.go`:

1. **Quién**: `roles.manage`, `permissions.manage` y `permissions.read` en las dos rutas (`403` si falta uno; una sesión
   con PIN nunca los tiene, `WaiterPermissions`).
2. **Qué roles**: solo los de `factoryRoleCodes` por `app.roles.code` (`waiter`, `cashier`, `kitchen`, `supervisor`,
   `inventory`, `finance`); reservado (`support`) `403`; sin conjunto (código nulo o `administrator`) `400`; inexistente
   `404`.
3. **Rol propio**: `roleID == principal.RoleID` → `403` «No puedes modificar tu propio rol.».
4. **`requireWithin`**: el actor debe tener **todos los permisos actuales** del rol («No puedes editar un rol con
   permisos que tú no tienes.») **y todos los de fábrica** («No puedes conceder a un rol permisos que tú no tienes.»),
   ambos `403`, en la vista previa y en la confirmación. Más `requireLimitWithin` (el límite de descuento del rol no
   mayor que el del actor), como al editar.
5. **Último administrador**: `lockAdministration` (mismo lock que `PATCH roles/{id}`) antes de leer el rol `FOR
UPDATE`, y el cambio dentro de `preserveAdministration` (`400` «Debe permanecer al menos un administrador ordinario
   activo.» si `fullAdministrators` pasaría de más de cero a cero; `sa` no cuenta).
6. **Carrera vista previa ↔ confirmación**: `expected` (lo `current` que mostró la vista previa) se compara como conjunto
   con los permisos leídos bajo el lock; distinto → `409 ROLE_CHANGED` sin escribir. Ya en los de fábrica → `200
changed: false` sin escribir ni auditar.
7. **Alcance**: solo `DELETE` + `INSERT` en `role_permissions` del rol; ni `role_discount_limits` ni `user_permissions`.
8. **Auditoría** en la misma transacción: `role.defaults_restored`, entidad `role`, antes el rol y después `{role, added,
removed}`.
9. **Sin cambios silenciosos**: la `0027` no toca roles; el asistente concede los conjuntos solo en una instalación nueva;
   una versión nueva de `factoryRolePermissions` no cambia roles existentes sin el botón.
10. **El servidor sigue validando cada permiso en cada ruta**; Go no impone `requires` al guardar (A3-D4).

Puntos que vale la pena mirar: que el Supervisor de fábrica no dé algo que un supervisor no deba tener (el conjunto es el
del anexo de la Fase 0); que `events.read` y `waiter_devices.use` estén en «Mesas y cuentas»; y que la capacidad
«Autorizar y corregir» (A3-D5) reúna lo que corresponde.

### Pruebas nuevas o ampliadas

- **Go, unidad** (`permission_catalog_test.go`): los 156 permisos registrados están en el catálogo y cada uno en un solo
  grupo de los diez, con descripción de una frase y `requires` (aunque vacío); dependencias conocidas, sin
  autodependencias ni ciclos (recorrido en profundidad) y las que pide la especificación (`accounts.open` →
  `accounts.read`, `orders.cancel` → `orders.edit`…); los seis conjuntos de fábrica con su tamaño del anexo, sin
  desconocidos ni repetidos y **cerrados bajo las dependencias**; el Mesero de fábrica es `WaiterPermissions` menos
  `orders.edit` y `orders.cancel` y una sesión con PIN lo conserva completo; las once capacidades con permisos conocidos;
  sin la sigla KDS en nombres ni descripciones. `TestPlainLanguageMessages` (Fase A) revisa además las 156 descripciones.
- **Go, integración** (`TestStageA3FactoryRoles`, `administration`): instalación nueva con cada rol de fábrica igual a su
  conjunto y el Administrador con todos; **un mesero nuevo con el rol Mesero y su PIN entra en una tableta, ve las mesas,
  abre una cuenta con dos productos y la envía a cocina sin tocar permisos**, y Go le rechaza cancelar unidades, los
  cobros y la vista previa de roles (`403`); el catálogo (156 permisos, 10 grupos, 11 capacidades, 6 roles de fábrica) y
  `403` sin `permissions.read`; restablecer con vista previa (lo que se agrega y se quita), sin escribir en la vista
  previa, con la auditoría (antes, agregado y quitado), sin tocar el límite de descuento ni la excepción del usuario,
  repetido con `changed: false` y sin otra auditoría, y `400` sin `expected`; `409 ROLE_CHANGED` si el rol cambió entre la
  vista previa y la confirmación (y con la vista previa nueva sí); escalada rechazada (un jefe de personal sin, por
  ejemplo, autorizar devoluciones no restablece el Supervisor, ni en la vista previa) y el rol con un permiso que el
  actor no tiene; el rol propio; un rol propio del usuario y el Administrador `400`, el soporte `403`, uno inexistente
  `404`; y **nunca sin administrador** (el único administrador completo en el Supervisor, restablecido por `sa`: `400` y
  nada cambia).
- **Pruebas existentes que contaban los permisos tras el asistente**, al día: `pin_acceptance_test.go` (el Mesero tenía
  solo `waiter_devices.use`), `long_open_hours_acceptance_test.go` («Cancelar cuentas» solo del Administrador; ahora
  también del Supervisor de fábrica) y `operations/inventory_core_test.go` (Inventarios vacío; ahora con su conjunto). La
  de PIN se reorganizó para compartir el armado del servidor (`newPinEnvironment`).
- **Rust**: `stage_a3_permission_routes_are_closed`.
- **Vitest** (`pages/stageA3Roles.test.tsx`, 11): las reglas con datos de Go (marcar con lo que se necesita, quitar con
  lo que deja de funcionar, capacidad completa, parcial o ninguna, quitar una capacidad conservando otra completa, el
  buscador sin acentos); la pantalla abre en la vista sencilla con la parcial; marcar capacidades y guardar; grupos con
  «x de y», marcar y quitar el grupo con su aviso; dependencias en la vista detallada; el buscador por nombre y
  descripción; «Restablecer» solo en el rol de fábrica (no en el Administrador ni en un rol propio) con la vista previa,
  la confirmación con `expected` y la selección al día; `ROLE_CHANGED`; y sin la capacidad, la lista de siempre.
- **Playwright** `workflow-stagea3-roles.spec.ts` (después de las de la A2 y antes de `workflow-zz-test-data`): el Mesero
  de una instalación nueva con sus permisos de fábrica; el Administrador sin el botón; la vista sencilla («Atender
  mesas» y «Para llevar» completas, «Cobrar» parcial «1 de 9»); la detallada («3 de 4», «2 de 5» → «4 de 5» al marcar
  «Cancelar…», que marca «Modificar…»), el aviso al quitar «Consultar cuentas e importes», quitar y marcar todo «Para
  llevar», el buscador por la descripción; guardar; restablecer con «Se quitarán (3)», confirmación y el Mesero otra vez
  con sus permisos de fábrica; y la etiqueta en el Registro de movimientos. Deja el Mesero como llegó.

### Pruebas ejecutadas

Todas con el arnés `$SP/iso.sh` y `ISO_TAG=SV`, en `$SP/wt/srv`, sobre el código de los commits (la única corrección
posterior a la primera corrida es de una prueba de integración de `administration`, que se repitió completa).

| Qué                                                                                                   | Resultado                                                                                                                                                               |
| ----------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `go-unit` (vet normal e `integration`, `go test -race` de todo, `GOOS=windows` build y vet)           | Aprobado: 35 paquetes `ok` (con el catálogo y `TestPlainLanguageMessages`)                                                                                              |
| `cargo test --locked --offline`                                                                       | 108 aprobadas (una nueva), 3 ignoradas                                                                                                                                  |
| Vitest con `tsc --noEmit` (suite completa)                                                            | **99 archivos, 943 pruebas aprobadas** (11 nuevas)                                                                                                                      |
| `go-int ./internal/administration`, primera corrida                                                   | Todo aprobado salvo `9D_permiso_cancelar_cuentas` (esperaba «Cancelar cuentas» solo en el Administrador; ahora lo trae también el Supervisor de fábrica): prueba al día |
| `go-int ./internal/administration`, segunda corrida completa                                          | `ok` (255 s): `TestStageA3FactoryRoles`, `TestStageTwoAcceptance` y `TestStageA1WaiterPIN` (con `newPinEnvironment`) incluidas                                          |
| `TestStageA3FactoryRoles` sola, mientras se escribía                                                  | `ok` (27 s)                                                                                                                                                             |
| `go-int ./internal/apicontract` (`TestContractV1` con el asistente nuevo)                             | `ok` (43 s)                                                                                                                                                             |
| `go-int ./internal/operations` con `ISO_RUN=TestRestaurantAcceptance` (incluye `inventory_core`)      | `ok` (263 s)                                                                                                                                                            |
| Playwright Chromium: `workflow-stagea1-pin` (usa el rol Mesero de fábrica) y `workflow-stagea3-roles` | 2 de 2                                                                                                                                                                  |
| Playwright WebKit: las mismas                                                                         | 2 de 2                                                                                                                                                                  |
| `format`, `fmt-check`, `sync-check`                                                                   | Aprobados                                                                                                                                                               |
| `md5 -q docs/etapas/*.md \| md5` antes y después de formatear                                         | `3555c65b1300ce044695adcc1ec19f0d` las dos veces                                                                                                                        |

No ejecutado (no lo pide la fase): la regresión completa (otras especificaciones de Playwright que crean usuarios con
roles propios, `go-int` de los demás paquetes), `build` y Android. **No verificado**: el paso de la aplicación Android
contra el Go temporal con un mesero de fábrica (el plan lo deja para la Fase E: «el mismo paso con la aplicación»); en
Go ya está probado por HTTP con las cabeceras de la aplicación 0.1.0.

### Commits (rama `etapa-a3-srv`, sin push ni fusión)

- `726b837` etapa A3 fase B: catálogo de permisos en un solo lugar (…) con GET permissions/catalog y la capacidad
  permission-groups-v1, los roles de fábrica en el asistente y «Restablecer permisos predeterminados» (…), con sus pruebas
- `37d0ca8` etapa A3 fase B: puente nativo con permission_catalog, role_defaults y restore_role_defaults, y su prueba de
  cargo
- `15de037` etapa A3 fase B: pantalla de permisos de un rol agrupada por área (…) y «Restablecer permisos
  predeterminados» con vista previa en los roles de fábrica, escondida sin permission-groups-v1, con stageA3Roles.test.tsx
  y workflow-stagea3-roles
- `2915e2b` etapa A3 fase B: arquitectura de permisos y roles de fábrica en CLAUDE.md, API_CONTRACTS.md y API_V1.md, y
  docs/ANDROID_INSTALAR.md §4

### Correcciones de la revisión

La revisión independiente (`$SP/doc-rev.md`, rama `etapa-a3-rev`) no encontró ningún hallazgo alto. Las pruebas del
revisor se trajeron sin cambios con `git cherry-pick` (`60eac80` ← `c17cb74`, `TestStageA3FactoryRolesReview`; `2169862`
← `7c40f41`, `stageA3RolesReview.test.tsx`) y las dos pasan con las correcciones, decididas por el coordinador:

| N.º | Decisión   | Corrección                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | Commit                               | Pruebas                                                                                                                                                                                                                                                                                                                                                    |
| --- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| R1  | A3-D16 (b) | `POST sessions/{id}/revoke` (`sessions_audit.go`) aplica `requireManageableUser` sobre el dueño de una sesión ajena (sus permisos efectivos y su límite de descuento dentro de los del actor): `403` «No puedes cerrar la sesión de un usuario con permisos que tú no tienes.». La propia se cierra siempre. El Supervisor de fábrica conserva `sessions.manage`. Es la única ruta de `sessions.*` que cierra sesiones; las otras que lo hacen (desactivar a un usuario, cambiar su PIN) ya aplicaban la regla, y revocar una terminal es de `devices.manage` (administra la terminal, no al usuario). | `a0ef54e`                            | Del revisor: el Supervisor de fábrica no cierra la del Administrador (`403`, el Administrador sigue con `auth/me` `200`). Mía (`A3_supervisor_cierra_sesiones_solo_de_quien_puede_administrar`): el mensaje exacto, y el Supervisor sí cierra la de un cajero (`200`, el cajero `401 SESSION_REVOKED`, `session.revoked` en el Registro).                  |
| R2  | A3-D17     | El catálogo informa además `factory_permissions` (el conjunto de cada rol de fábrica; datos fijos, con `permissions.read`). `App.tsx` pasa a `RolesPage` el `role_id` y `support` de `auth/me`, y el límite de descuento de quien edita sale del de su rol (el soporte, 100 %). `roleDefaultsBlocker` no ofrece el botón —y dice por qué en palabras del restaurante— con el rol propio, un rol o un conjunto de fábrica con permisos que quien edita no tiene, o un límite de descuento mayor: ningún clic normal llega a un `403`. Go no se aflojó.                                                  | `a0ef54e` (Go), `f917894` (pantalla) | Del revisor (rol con un permiso que quien edita no tiene: sin botón). Mías: el rol propio, el conjunto de fábrica con un permiso que no se tiene, el límite mayor y el soporte (sí se ofrece), sin ninguna llamada a `role_defaults`; `TestPermissionCatalogCoversEveryRegisteredPermission` y `A3_catalogo_de_permisos` comprueban `factory_permissions`. |
| R3  | A3-D18     | Con una búsqueda escrita, los botones del grupo actúan solo sobre lo que se ve y lo dicen («Marcar los 2 que se ven», «Marcar el que se ve», «Quitar el que se ve»); lo que eso necesita se sigue marcando solo, con su aviso. Sin búsqueda, «Marcar todo el grupo» y «Quitar todo el grupo», como antes.                                                                                                                                                                                                                                                                                              | `f917894`                            | Vitest «con una búsqueda, los botones del grupo actúan solo sobre lo que se ve» (lo que no se ve queda sin marcar) y el paso nuevo de `workflow-stagea3-roles`.                                                                                                                                                                                            |
| R4  | A3-D19     | Sin `catalog.availability.manage` en el Supervisor de fábrica (88; conserva `catalog.availability.override` y `catalog.sold_out.manage`). Sigue cerrado bajo las dependencias. El anexo de este documento (y `API_CONTRACTS.md`) dice 88.                                                                                                                                                                                                                                                                                                                                                              | `a0ef54e`                            | `TestFactoryRolesAreClosedUnderDependencies` (88 y cerrado), `TestStageA3FactoryRoles` (el Supervisor de la instalación nueva igual a su conjunto).                                                                                                                                                                                                        |
| R5  | A3-D20     | Tras `409 ROLE_CHANGED`, la pantalla vuelve a leer la lista de roles, pone en el editor los permisos que el rol tiene ahora y avisa que otra persona los cambió y que se descartaron los cambios sin guardar; un «Guardar rol» posterior parte de lo de ahora.                                                                                                                                                                                                                                                                                                                                         | `f917894`                            | Vitest «si el rol cambió entre la vista previa y la confirmación, lo dice y carga lo de ahora»: el editor queda con el cambio de la otra persona y el `PATCH` siguiente lo conserva.                                                                                                                                                                       |

Ajuste de una prueba mía: `stage9dStyles.test.tsx` ya no lista «Quitar todo el grupo» entre los botones reversibles,
porque su texto ahora se arma según la búsqueda (sigue con el estilo secundario). Documentación: `API_CONTRACTS.md`
(`factory_permissions`, la regla de `sessions/{id}/revoke` y lo de la pantalla) y `CLAUDE.md` (seguridad y la pantalla).
Commit de documentación: `159e538`.

Pruebas ejecutadas después de las correcciones (`ISO_TAG=SV`, sobre el código de los commits):

| Qué                                                                                                | Resultado                                                                      |
| -------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `go-int ./internal/administration` con `ISO_RUN=TestStageA3FactoryRoles` (la mía y la del revisor) | `ok` (58 s)                                                                    |
| `go-unit`                                                                                          | Aprobado: 35 paquetes `ok`                                                     |
| Vitest con `tsc --noEmit` (suite completa)                                                         | **100 archivos, 946 pruebas aprobadas** (con la del revisor y las nuevas mías) |
| `go-int ./internal/administration` completo                                                        | `ok` (272 s)                                                                   |
| Playwright `workflow-stagea3-roles` Chromium / WebKit                                              | 1 de 1 / 1 de 1                                                                |
| `format`, `fmt-check`, `sync-check`; `md5 -q docs/etapas/*.md \| md5`                              | Aprobados; `3555c65b1300ce044695adcc1ec19f0d` antes y después                  |

`cargo` no se corrió: `api.rs` no cambió en las correcciones.

---

## Revisión independiente · Fase B (roles de fábrica y «Restablecer permisos predeterminados»)

Revisor independiente de §0 de la especificación («sin revisor independiente, salvo para los roles de fábrica y el
restablecimiento de permisos (§3)»), 19 de noviembre de 2026. Árbol `$SP/wt/rev`, rama `etapa-a3-rev`, sobre la cabeza de
la Fase B (`2915e2b`); se revisó `git diff 7e9a957..2915e2b` contra la especificación, A3-D2 a A3-D6 y el anexo del
avance. Cada afirmación del resumen del autor se comprobó en el código. Pruebas con el arnés (`ISO_TAG=RV`, clúster temporal
55499, Go temporal 8189 y vista previa 1429 solo para Playwright); nada contra el servicio real. Sin cambios en el código del
autor: solo dos pruebas nuevas en mi rama (abajo).

### Hallazgos

| N.º    | Gravedad | Dónde                                                                                                                          | Escenario (entrada → resultado incorrecto)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | Prueba                                                                                                                                                                                              | Corrección propuesta                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| ------ | -------- | ------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **R1** | Media    | `backend/internal/administration/permission_catalog.go:260` (`sessions.manage` en el Supervisor) con `sessions_audit.go:20-37` | El Supervisor de fábrica trae `sessions.manage` («Revocar sesiones», grupo **Personal**, parte de la capacidad «Administrar personal») y `POST sessions/{id}/revoke` no aplica la regla de delegación (`delegation.go`: «nadie administra a un usuario con más permisos que los suyos»; solo protege a `sa`). En una **instalación nueva**, un usuario con el rol Supervisor ve en Sesiones activas el botón de revocar en las sesiones del Administrador (`SessionsPage.tsx:59`) y las cierra cuando quiera: `POST sessions/{sesión del admin}/revoke` → `200 {"ok":true}` y el `auth/me` siguiente del Administrador → `401 SESSION_REVOKED`. No es una escalada de permisos, pero es administrar a un usuario con más permisos (y cortar a cualquier cajero o supervisor en su turno) concedido de fábrica. La ruta ya existía; la Fase B la vuelve disponible de fábrica. | `c17cb74`, `TestStageA3FactoryRolesReview/RV_supervisor_de_fabrica_no_cierra_la_sesion_del_administrador` (**falla hoy**: revocar 200, `auth/me` 401). Pasa con cualquiera de las dos correcciones. | Decisión del coordinador (toca A3-D2 o una ruta existente): **(a)** quitar `sessions.manage` del Supervisor de fábrica (queda `sessions.read`; el Supervisor pasa a 88 y hay que ajustar el anexo y `TestFactoryRolesAreClosedUnderDependencies`), o **(b)** en `POST sessions/{id}/revoke`, `requireManageableUser` sobre el dueño de la sesión (salvo la propia), como `users/{id}`, lo que también protege a los roles personalizados. Recomiendo (b) y, si «cerrar una sesión olvidada» de un mesero es la razón, conservar el permiso en el Supervisor. |
| **R2** | Baja     | `desktop/src/pages/RolesPage.tsx:188,195`                                                                                      | «Restablecer permisos predeterminados» se muestra en todo rol de fábrica si quien edita tiene `roles.manage`, `permissions.manage` y `permissions.read`, aunque Go lo vaya a rechazar: (1) un rol con permisos que quien edita no tiene (la pantalla lo sabe con `can`) → la vista previa responde `403` «No puedes editar un rol con permisos que tú no tienes.»; (2) el propio rol de quien edita (`RolesPage` no recibe el `role_id` de la sesión) → `403` «No puedes modificar tu propio rol.»; (3) un conjunto de fábrica con permisos que quien edita no tiene (el catálogo solo informa los códigos, no los conjuntos) → `403`; (4) un rol con un límite de descuento mayor que el suyo. Cada clic deja además un `access.denied` en el Registro de movimientos. Go sigue protegiendo: es solo la pantalla, pero el encargo pide esconderlo donde Go lo rechaza.       | `7c40f41`, `desktop/src/pages/stageA3RolesReview.test.tsx` (caso 1; **falla hoy**: el botón aparece). Los casos 2 a 4, por lectura del código.                                                      | Esconder el botón (o mostrarlo deshabilitado con la explicación) si `role.id` es el `role_id` de la sesión o si `role.permissions.some((key) => !can(key))`; para (3), que el catálogo informe los conjuntos (`factory_roles: [{code, permissions}]`) y comparar con `can`, o mostrar sin más el mensaje de Go (hoy ya lo muestra el aviso). (4) puede quedar con el mensaje de Go.                                                                                                                                                                          |
| **R3** | Baja     | `desktop/src/components/PermissionEditor.tsx:245` (con el buscador de `:213`)                                                  | Con una búsqueda escrita, cada sección muestra solo las coincidencias, pero «Marcar todo el grupo» marca **todo** el grupo, también lo que no se ve, y sin aviso (el aviso solo cuenta lo que se marcó por dependencia). Ejemplo real: buscar «precuenta» deja en «Caja y cobros» solo «Imprimir tickets y precuentas»; «Marcar todo el grupo» marca los 40 de Caja y cobros (devoluciones, autorizaciones, configuración de caja y de cobros…). Comprobado con un caso de Vitest que no se confirmó: con «PIN» en el buscador, «Marcar todo el grupo» en «Mesas y cuentas» deja «2 de 2» y marca «Consultar cuentas e importes», que no estaba a la vista, sin ningún aviso. Go no deja conceder más de lo que el actor tiene, y se ve el «x de y» antes de guardar.                                                                                                         | Caso ad hoc de Vitest (no confirmado: la corrección es de diseño).                                                                                                                                  | Mientras haya una búsqueda, que «Marcar todo el grupo» y «Quitar todo el grupo» actúen solo sobre lo visible (y lo digan: «Marcar los 3 que coinciden»), o esconderlos mientras se busca.                                                                                                                                                                                                                                                                                                                                                                    |
| **R4** | Baja     | `backend/internal/administration/permission_catalog.go:248` (`catalog.availability.manage` en el Supervisor)                   | El anexo dice del Supervisor «Sin administrar catálogo», pero `catalog.availability.manage` («Modificar disponibilidad y horarios») le deja cambiar en Catálogo los **horarios** de disponibilidad de cualquier producto (`PATCH catalog/products/{id}/availability`, `actions.go:241`; `ProductEditor.tsx:190`, `CatalogRecord.tsx:118`), que es configuración del catálogo. Lo operativo del turno ya lo cubre `catalog.sold_out.manage` («Marcar productos como agotados…»), que también trae.                                                                                                                                                                                                                                                                                                                                                                             | Lectura del código.                                                                                                                                                                                 | Decisión del coordinador sobre A3-D2: quitar `catalog.availability.manage` del Supervisor (88, con `catalog.availability.override` si tampoco se quiere; no rompe el cierre bajo dependencias) o dejarlo y corregir el porqué del anexo («y los horarios de disponibilidad»).                                                                                                                                                                                                                                                                                |
| **R5** | Baja     | `desktop/src/pages/RolesPage.tsx:122-126`                                                                                      | Con `409 ROLE_CHANGED` la pantalla avisa, pero no vuelve a leer el rol ni la lista: el editor conserva la selección de cuando se abrió. Si después se toca «Guardar rol», `PATCH roles/{id}` (que no tiene control de concurrencia, como hasta la A2) reemplaza sin aviso el cambio que hizo la otra persona y que el 409 acababa de detectar. Go hace lo correcto; es la pantalla tras el 409.                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | Lectura del código (la prueba del autor «si el rol cambió entre la vista previa y la confirmación…» confirma que la selección no cambia).                                                           | Tras `ROLE_CHANGED`, `await onSaved()` (recarga la lista) y poner la selección del editor en lo que Go tiene ahora (otra `GET roles/{id}/defaults` da `current`), avisando que se descartaron los cambios sin guardar.                                                                                                                                                                                                                                                                                                                                       |

Ningún hallazgo de gravedad alta: no encontré ningún camino por el que el restablecimiento (vista previa o confirmación) dé
o quite algo que `PATCH roles/{id}` no permitiría, ni una instalación existente que cambie sola.

### Lo que comprobé y está bien

- **Reglas del restablecimiento** (`role_defaults.go`), contra `roles.go` y `delegation.go`: las dos rutas exigen
  `roles.manage`, `permissions.manage` y `permissions.read` antes de leer nada (más estricto que `PATCH`); reservado `403`
  antes del código; sin conjunto (rol del usuario con `code` nulo, Administrador, código vacío) `400`; el propio rol `403`;
  `requireWithin` sobre los permisos **actuales** y los **de fábrica** y `requireLimitWithin`, en la vista previa y en la
  confirmación; inexistente `404` (`pgx.ErrNoRows` → `publicFault`), identificador que no es UUID `400` (`routes.go:200`).
  La confirmación decodifica, exige `expected`, toma `lockAdministration` y después la fila del rol `FOR UPDATE` (el mismo
  orden que `PATCH roles/{id}` y `PATCH users/{id}`), repite todas las comprobaciones bajo el lock y solo entonces escribe
  (`DELETE` + `INSERT` de `role_permissions` dentro de `preserveAdministration`) y audita. **Ningún camino escribe antes de
  validar**; además un `403` revierte el savepoint (`runProtected`) y solo deja `access.denied`. `changed: false` exige las
  mismas comprobaciones y no escribe. Solo toca `role_permissions`: el límite y `user_permissions` quedan igual (prueba del
  autor). La transacción es `READ COMMITTED` (sin nivel explícito), así que lo leído después del lock está al día.
- **Carreras**: dos confirmaciones a la vez → una `changed: true`, la otra `changed: false`, un solo registro; dos
  restablecimientos a la vez que juntos dejarían el sistema sin administrador completo → uno `200` y otro `400` (mis
  pruebas); vista previa ↔ confirmación con `409 ROLE_CHANGED` y repetir la confirmación (pruebas del autor). Las ediciones
  de usuarios que tienen el rol (`PATCH users/{id}`, `PUT users/{id}/permissions`) toman el mismo lock consultivo.
- **`GET permissions/catalog`**: exige `permissions.read` (`403` sin él, prueba del autor) y solo devuelve datos fijos del
  código (grupos, descripciones, dependencias, capacidades y los **códigos** de los roles de fábrica, no sus conjuntos); una
  sesión con PIN nunca lo tiene (`WaiterPermissions`). Está antes de la validación de UUID de `routes.go:200`.
- **Conjuntos de fábrica**: los seis de Go son **exactamente** los del anexo (comparé las tablas del anexo con
  `factoryRolePermissions` con un script: 14, 36, 8, 89, 20 y 28, iguales); **cerrados** bajo `requires` y **sin ciclos**
  (mi script sobre el catálogo completo, además de la prueba unitaria). El Mesero es la tabla de la guía §4 (en `7e9a957`)
  con Para llevar, «Entregado» (A3-D3) y la precuenta, sin `orders.edit` ni `orders.cancel`, y está dentro de
  `WaiterPermissions` (una sesión con PIN lo conserva entero). Ninguno concede `users.manage`, `roles.*`, `permissions.*`,
  `devices.manage`, `settings.*`, `registers.manage`, `documents.*` ni configuración de cobros, caja, cocina o inventario;
  las autorizaciones solo en el Supervisor (y autorizar compras en Finanzas). Lo que me pareció fuera de puesto está en R1
  y R4. Cada rol abre en una pantalla que puede usar (Finanzas, con `accounts.read` sin mesas, ve solo la pestaña Cuentas
  de `OperationalPage`; Cocina abre en Cocina; Inventarios en Administración).
- **Solo en el asistente y con el botón**: los únicos que escriben `role_permissions` fuera de migraciones antiguas son
  `setup.go` (detrás de `configured`, con el lock del asistente), `roles.go` y `role_defaults.go`; `ensureFoundations`
  solo inserta permisos y roles con `ON CONFLICT DO NOTHING`; la `0027` no toca roles y `TestStageA3Upgrade` exige que
  ninguna otra fila cambie; nada al arrancar (`cmd/`), ni la importación del sistema anterior (solo asigna roles por
  código).
- **Dependencias y validación**: un solo lugar (`permission_catalog_entries.go`); `permissionCatalogEntries` solo lo usan
  el catálogo y `closePermissions` (ninguna ruta). La interfaz no puede guardar una clave que Go no valide
  (`knownPermission` y `requireWithin` en `POST`/`PATCH roles`). Go no impone `requires` (A3-D4) y **cada ruta sigue
  validando**: un rol con «Abrir cuentas» sin «Consultar cuentas e importes» se guarda y `POST restaurant/accounts` responde
  `403` sin abrir nada (mi prueba), además de la del autor (el Mesero de fábrica no cancela unidades ni cobra).
- **Auditoría**: `role.defaults_restored` en la misma transacción, con el rol de antes y `{role, added, removed}` después;
  la etiqueta en `AuditPage.tsx`; la vista previa no escribe nada.
- **Puente de Rust**: tres operaciones nuevas en la lista cerrada con método y ruta fijos (`GET permissions/catalog`, `GET`
  y `POST roles/{id}/defaults`), identificador validado por `resource_id`; React no elige ruta ni método.
- **Escritorio**: el botón no aparece en el Administrador ni en un rol del usuario; sin `permission-groups-v1`, la lista de
  siempre y nada del catálogo (pruebas del autor, que pasan en mi corrida).

### No verificado

- Playwright en WebKit (solo corrí Chromium) y nada de Android (no aplica a esta revisión).
- Los casos 2 a 4 de R2 y R4–R5 solo por lectura del código (R2.2 no se puede montar: `RolesPage` no recibe la sesión).
- Si el negocio quiere que el Supervisor cierre sesiones del Administrador (R1) o edite horarios (R4): es decisión del
  coordinador; lo señalo porque contradice la regla de delegación y el «sin administrar catálogo» del anexo.

### Pruebas ejecutadas (todas con `ISO_TAG=RV`)

| Qué                                                                                              | Resultado                                                                                                                                                                                                     |
| ------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `go-int ./internal/administration` con `ISO_RUN=TestStageA3FactoryRoles` (la del autor y la mía) | La del autor aprobada; la mía: 4 subpruebas aprobadas (límite de descuento, dos confirmaciones, dependencias con la ruta que valida, dos restablecimientos sin administrador) y **R1 falla** como se esperaba |
| `go-int ./internal/administration` con `ISO_RUN=TestStageA3FactoryRoles$` (solo la del autor)    | `ok` (29 s)                                                                                                                                                                                                   |
| `go-int ./internal/administration` completo, con mi prueba                                       | 274 s: todo aprobado salvo **la subprueba de R1**, la única que falla (como se esperaba)                                                                                                                      |
| `go-unit` (vet normal e `integration`, `test -race` de todo, `GOOS=windows`)                     | Aprobado (35 paquetes `ok`)                                                                                                                                                                                   |
| `vitest` (con `tsc --noEmit`)                                                                    | 100 archivos, 944 pruebas: 943 aprobadas y **1 falla, la de R2** (las 11 del autor aprueban)                                                                                                                  |
| Caso ad hoc de Vitest de R3 (no confirmado, borrado)                                             | Aprobado: confirma el comportamiento descrito                                                                                                                                                                 |
| `cargo test` (puente nativo)                                                                     | 108 aprobadas, 3 ignoradas (`stage_a3_permission_routes_are_closed` incluida)                                                                                                                                 |
| Playwright Chromium `workflow-stagea3-roles`                                                     | 1 de 1                                                                                                                                                                                                        |
| Scripts propios sobre el catálogo volcado de Go (cierre, ciclos, anexo contra Go)                | Sin ciclos ni claves desconocidas; seis conjuntos cerrados e iguales al anexo                                                                                                                                 |
| `format`, `fmt-check`; `md5 -q docs/etapas/*.md \| md5` antes y después                          | Aprobados; `3555c65b1300ce044695adcc1ec19f0d` las dos veces                                                                                                                                                   |

### Commits (rama `etapa-a3-rev`, sin push ni fusión)

- `c17cb74` etapa A3 revisión de la fase B: `TestStageA3FactoryRolesReview`
  (`backend/internal/administration/role_defaults_review_acceptance_test.go`; la subprueba de R1 falla hasta corregirlo).
- `7c40f41` etapa A3 revisión de la fase B: `desktop/src/pages/stageA3RolesReview.test.tsx` (R2; falla hasta corregirlo).

### Decisiones del coordinador sobre la revisión de la Fase B

Ningún hallazgo alto; ninguno obligó a cambiar la especificación. Correcciones del autor en `etapa-a3-srv` (`60eac80`,
`2169862` —las pruebas del revisor, sin cambios—, `a0ef54e`, `f917894` y `159e538`), fusionadas con la Fase B en
`215fd81`; las dos pruebas del revisor, que fallaban, pasan.

- **A3-D16 (R1)**: `POST sessions/{id}/revoke` aplica la regla de delegación (`requireManageableUser` sobre el dueño de
  una sesión ajena, como al editar un usuario, su PIN o su contraseña); la sesión propia se cierra siempre. El
  Supervisor de fábrica conserva «Revocar sesiones» y ya no puede cerrar la del Administrador. Endurece una ruta
  existente sin cambiar ninguna decisión de la especificación.
- **A3-D17 (R2)**: la pantalla no ofrece «Restablecer permisos predeterminados» donde Go lo rechazaría (rol propio, rol o
  conjunto de fábrica con permisos que quien edita no tiene, límite de descuento mayor) y dice por qué; el catálogo
  informa el conjunto de cada rol de fábrica (`factory_permissions`, con `permissions.read`). Go no se aflojó.
- **A3-D18 (R3)**: con una búsqueda escrita, «Marcar» y «Quitar» del grupo actúan solo sobre lo que se ve y lo dicen.
- **A3-D19 (R4)**: el Supervisor de fábrica no trae `catalog.availability.manage` (horarios de venta del catálogo: es
  administrar el catálogo); queda en **88** permisos y conserva `catalog.availability.override` y
  `catalog.sold_out.manage`. Sustituye la fila del Supervisor del anexo de la Fase 0.
- **A3-D20 (R5)**: tras un 409 `ROLE_CHANGED` la pantalla vuelve a leer el rol y avisa, para que guardar no pise sin aviso
  el cambio de otra persona.

---

## Fase C · Android (los 12 ajustes de §4)

Agente de Android en `$SP/wt/and` (rama `etapa-a3-and`, con `etapa-a3` unida después de la Fase A). Sin cambios en
`backend/`, `desktop/`, `CLAUDE.md` de la raíz, `docs/ETAPA_A3_AVANCE.md`, el contrato ni la §4 de
`docs/ANDROID_INSTALAR.md`. Pruebas en copias aisladas: Go temporal `ISO_TAG=AN` (8193/8453, base `adel_pos_test_an_…`,
una nueva por dispositivo), `Pixel_Tablet` (5580, Android 17) y `ADEL_A1_Phone_API33` (5582, Android 13) de solo
lectura, siempre con `AdelTestRunner` y emparejando por la dirección escrita; nunca la red real. Sin servidor Go nuevo:
todo lo que usa la Fase C ya lo da Go 0.13.0 (`discardable` y `draft_quantity` en `restaurant/state`,
`POST restaurant/accounts/{id}/discard` de la 9D, `terminal.home_screen` de `getIdentity`).

### Antes de los 12 puntos: el estado al arrancar (pedido del coordinador)

Commit `4fa6331`. Un estado todavía no conocido ya no dice «Sin conexión»: al arrancar el estado único **duda** (la
barra dice «En línea») hasta la primera respuesta de Go, el primer aviso de red de Android que diga que no hay red, o el
plazo de la tolerancia (20 s); con eso decide. `ServerLinkTracker(initialNetwork: LinkState?)` (null = sin saber),
`startingUp()`/`startupExpired()`/`nextDecisionIn()`; `ConnectionMonitor.reportedLinkState` ya no manda el
«sin red» inicial que Android da antes de conocer la red; `ServerLinkMonitor` vuelve a decidir solo al vencer el plazo.
JVM: 5 pruebas nuevas en `ServerLinkTrackerTest` (19 en total).

### Commits (rama `etapa-a3-and`, sin push ni merge)

- `4fa6331` etapa A3 fase C (Android): un estado todavía no conocido no dice «Sin conexión» al arrancar
- `d835155` etapa A3 fase C (Android): los 12 ajustes de §4 en teléfono y tableta
- `cec2486` etapa A3 fase C (Android): versión 0.3.0 (versionCode 3)
- `1726e09` etapa A3 fase C (Android): android/CLAUDE.md y la guía con los ajustes de pantallas y la 0.3.0

### Qué se hizo (§4)

1. **Flecha de regreso** (`AdelTopBar(onBack)`, 48 dp, ícono `AdelIcons.Back`, «Volver»): en Captura, la cuenta a un
   toque del teléfono, Pendientes, Revisión y Para llevar (cuando está encima de Mesas). Llama a
   `onBackPressedDispatcher.onBackPressed()`: pasa por los mismos `BackHandler` que la tecla Atrás (primero cierra el
   panel a un toque, después la captura). **Una sola flecha**: el panel a un toque de `AdaptivePanes` se registra en
   `LocalTopBarBack` (`TopBarBack`, `core:ui`) y ya no dibuja su «Volver a …» propio. Al salir de una captura con
   productos sin enviar (regla A1: queda guardada con su mesero), la raíz muestra «Lo capturado en Mesa 4 quedó guardado
   sin enviar a cocina. Lo encuentras en la mesa o en Pendientes.» (o «…en Para llevar o en Pendientes.»;
   `CaptureRoute(onLeaveUnsent)`, `SystemBackHandler` de `core:ui` porque `core:capture` no tiene `activity-compose`).
   El aviso se muestra al llegar a la pantalla a la que se vuelve y se quita al pasar a otra: en el teléfono tapaba
   «Ver la cuenta» de la captura siguiente (lo encontró la prueba de punta a punta `a4`).
2. **Título** (`CaptureTitle.of`, `core:domain`): «Mesa 4», «Para llevar #12» (el folio de Go solo si es de esa cuenta),
   «Para llevar · Nombre» o «Nuevo pedido para llevar»; `CaptureRoute(onTitleChange)` lo da a la barra.
3. **«Buscar producto»** en un renglón siempre (`CatalogSearchField`, `maxLines = 1` con elipsis).
4. **«Ver la cuenta»** y, con productos sin enviar, «Ver la cuenta · 2 por enviar» (plurales).
5. **Entrada**: título «Ingresar» con el logotipo (PNG de `assets/brand/` reducidos a 384 × 128 en
   `core/ui/src/main/res/drawable-nodpi/`, claro u oscuro según el tema, 28 dp de alto; ≈ 53 KB los dos), «Selecciona tu
   usuario», «Selecciona tu usuario y escribe tu PIN.», botón «Ingresar», «Elegir otro usuario».
6. **Pendientes**: título «Pendientes» (también el botón de Mesas), flecha y «Reintentar ya» solo si hay algo que
   reintentar.
7. **Mesas compacta** (`TablesScreen`, `TablesMap`): sin «Buscar mesa» (se quitaron `TableSearch` y
   `TableCanvas.scaleToFit`); una fila con «Lista | Mapa» (un interruptor propio y compacto: el `SegmentedButton` de
   Material reserva 26 dp para un ícono en cada opción y en el teléfono vertical las áreas no cabían), las áreas como
   pestañas (`FilterChip`, desplazables si no
   caben, la elegida siempre traída completa a la vista; el área elegida en `SavedStateHandle`, `tables.mapArea`) y
   «Pendientes»; la lista muestra solo el área elegida
   («Esta área no tiene mesas.» si no tiene); sin «Ver la mesa» (tocar la mesa abre su panel). El mapa llena su espacio
   con `TableMapFit` (`core:domain`, puerto de `fitTables` del escritorio: escala por el rectángulo de las mesas del área,
   lado menor ≥ 64, escala ≤ 2, margen 16, centrado si sobra y desplazamiento horizontal y vertical si no caben).
8. **Tarjetas de cuenta** (`AccountCard`, pública para su prueba): dos renglones, «Mesa 6 · folio 12 · $755.00» en una
   línea con elipsis y abajo la etiqueta; todas las etiquetas de estado (`StateBadge`, `TableStateBadge`) con
   `maxLines = 1`; las capturas y pedidos del panel (`WorkRow`) también en dos renglones.
9. **«Imprimir precuenta» en cada cuenta del panel**: el mismo `PrecheckPrintBar` de `core:capture` (`framed = false`)
   con un `PrecheckPrinting` en `TablesViewModel` (una impresión siguiéndose a la vez). Sin permiso: el botón
   deshabilitado con «Tu usuario no tiene permiso para imprimir la precuenta. Pídelo en la computadora principal.»
   (`PrecheckSection(allowed = false)`, también en la cuenta, donde antes se ocultaba); sin impresora conocida: «Este
   dispositivo no tiene impresora asignada. Asígnala en la computadora principal, en Terminales.»; sin conexión, «Para
   imprimir necesitas conexión…». Go manda igual (si no hay impresora, su respuesta lo dice).
10. **«Descartar cuenta vacía»** en cada cuenta que Go marca `discardable` (`AccountDto.discardable`, sin el campo = no):
    confirmación, `POST restaurant/accounts/{id}/discard` con `operation_id` y `revision`, con conexión y la sesión del
    mesero, nunca en la cola (`RestaurantRepository.discardEmptyAccount`, `DiscardAccountResult`); sin respuesta se
    repite una vez con la misma clave; `ACCOUNT_NOT_EMPTY`/`ACCOUNT_CONFLICT` → se vuelve a leer y se dice «La cuenta
    cambió en otra terminal…». Permiso: `accounts.open` + `accounts.read` (`WaiterAction.DISCARD_EMPTY_ACCOUNT`); sin él,
    el botón deshabilitado con su porqué. La confirmación dice, como el escritorio, que no tiene productos enviados a
    cocina, pagos ni registros de cocina, cuántos productos sin enviar se borran (`draft_quantity`) y que no es una
    cancelación.
11. **Producto sin imagen**: la tarjeta compacta (nombre y precio, sin la letra grande) en teléfono y tableta.
12. **Pantalla inicial** (A3-D7): `terminal.home_screen` de `getIdentity` (`IdentityTerminalDto.homeScreen`), guardado
    con la precuenta (`PrecheckSetup.home`, sin cambiar Room); `DeviceHome.fromWire`: solo `takeaway` abre Para llevar y
    solo con `takeaway.read`; cualquier otro valor o ninguno, Mesas. La raíz lo aplica una vez por entrada (espera hasta
    5 s la lectura de Go al entrar con conexión) y solo si el mesero sigue en Mesas sin haber tocado nada.

**Sin duplicar (regla 20)**: la flecha, el título, el aviso al salir, «Ver la cuenta», Pendientes y la precuenta viven en
`core:ui`/`core:capture`/`core:domain`; `feature:waiter` solo agrega lo del panel de la mesa. **Textos (§1)**: todos
nuevos sin notas de desarrollo; `PlainLanguageTest` pasa. Comentarios del código con los nombres viejos de pantalla
(«¿Quién eres?», «Pedidos pendientes», «Elegir otro nombre») actualizados.

**Versión**: `android/gradle.properties` → `adelpos.versionName=0.3.0`, `adelpos.versionCode=3`.

**Documentación**: `android/CLAUDE.md` (encabezado, regla 11 con la flecha única, los nombres nuevos, estructura,
versión, pruebas, la suite `adjustments` y la sección nueva «Ajustes de pantallas (etapa A3, §4)»; la precuenta sin
permiso); `docs/ANDROID_INSTALAR.md` fuera de su §4 (0.3.0 y su archivo, «Selecciona tu usuario», «Pendientes», la
subsección nueva «Entrar y moverse en la aplicación» al final de la §3 —entrada, pantalla inicial, Mesas, las cuentas
de una mesa con la precuenta y «Descartar cuenta vacía», la captura, la flecha y el aviso—, la precuenta desde el
panel y sin permiso en la §7). `scripts/android-e2e.sh`: suite `adjustments` (`a1`–`a6`, con capturas), también en
`all`.

### Decisiones que propongo (el coordinador las numera)

1. **Flecha en Para llevar**: Para llevar siempre va encima de Mesas en la pila (Mesas es el inicio), así que su flecha
   regresa a Mesas, también cuando la pantalla inicial del dispositivo es Para llevar (hace lo mismo que la tecla
   Atrás). Mesas no tiene flecha.
2. **«Descartar cuenta vacía» espera a lo de este dispositivo**: si el mesero activo tiene en este dispositivo una
   captura o un pedido de la cola de esa cuenta, el botón se ve deshabilitado con «Tienes una captura de esta cuenta en
   este dispositivo: ábrela para enviarla o descartarla.» (descartarla en Go dejaría esa captura sin cuenta). Las
   capturas de otro mesero en el mismo dispositivo no se ven (regla de la A1) y no se revisan: si alguien descarta esa
   cuenta, esa captura termina en «Requiere tu revisión» como cualquier cuenta cerrada. Go decide si se puede.
3. **La confirmación del descarte** sigue el texto del escritorio (`OperationalPage.discardAccount`): Go marca
   `discardable` también una cuenta con productos sin enviar (borrador guardado), así que el diálogo dice cuántos se
   borran.
4. **Pantalla inicial**: una vez por entrada y solo si el mesero no se movió; si Go responde después de 5 s, se queda
   en Mesas (la siguiente entrada ya usa lo guardado).
5. **Precuenta en el panel**: un seguimiento a la vez; imprimir otra cuenta deja de seguir la anterior (como al cambiar
   de cuenta en la captura).
6. **Áreas desplazables**: junto a «Lista | Mapa» y «Pendientes» caben pocas pestañas (en el teléfono vertical de
   411 dp, una completa y parte de la siguiente; un teléfono más angosto no lo probé); la fila se desliza, la pestaña cortada indica que hay
   más y la elegida siempre se trae completa a la vista. Si un restaurante tiene muchas áreas y se prefiere otra cosa:
   «Lista | Mapa» solo con íconos en anchos chicos (no hay íconos de lista y mapa en `Icon.tsx` del escritorio) o las
   áreas en un segundo renglón (contradice «una sola fila» de §4.7). No hice ninguna de las dos.
7. **Tableta, dos paneles en la entrada**: el panel derecho repite «Selecciona tu usuario y escribe tu PIN.» (ya repetía
   el texto viejo, `entry_help` en los dos). Lo dejé igual; se puede cambiar por otro texto si el coordinador lo pide.

### Pruebas ejecutadas (resultado real)

Todas con el código final (`1726e09`), salvo donde se dice. Las instrumentadas con la aplicación y sus pruebas
desinstaladas antes y contando los XML (`$SP/and-count-xml.py`); copias en `$SP/and-results/`.

| Prueba                                                                                            | Dónde                        | Resultado                                                                                                                                                                                                                                                                                                         |
| ------------------------------------------------------------------------------------------------- | ---------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| JVM `core:domain:test` + `testDebugUnitTest` (todos los módulos), con `--rerun-tasks`             | Mac                          | 381 pruebas, 0 fallas: domain 172 (incluye `StageA3PanelRulesTest` y los 19 de `ServerLinkTrackerTest`), data 179 (incluye `TablePanelAccountsTest`), ui 12 (`PlainLanguageTest`, `ColorsOnlyInPaletteTest`), access 7, waiter 6, takeaway 5. `core:ui` vuelto a correr con `--rerun` tras el último cambio: 12/0 |
| `lintDebug` (todos), `ktfmtCheck`, `fmt-check` (Prettier)                                         | Mac                          | sin errores (lint encontró y se corrigió la cantidad `many` que faltaba en un plural nuevo)                                                                                                                                                                                                                       |
| Instrumentadas `:app` (final, `C7`)                                                               | `ADEL_A1_Phone_API33` (5582) | 119 pruebas, 44 corridas, 75 omitidas (las de Go sin argumentos), 0 fallas; `StageA3ScreensTest` 7/7, `DeviceLayoutTest` 2/2                                                                                                                                                                                      |
| Instrumentadas `:app` (final, `C7`)                                                               | `Pixel_Tablet` (5580)        | 119 / 44 corridas / 75 omitidas / 0 fallas; `StageA3ScreensTest` 7/7, `DeviceLayoutTest` 2/2                                                                                                                                                                                                                      |
| Instrumentadas `:core:ui` y `:core:data` (código de `core:ui` igual desde entonces)               | 5582 y 5580                  | ui 14/14, data 18 corridas + 2 omitidas, 0 fallas en los dos                                                                                                                                                                                                                                                      |
| `scripts/android-e2e.sh --suite adjustments` (`a1`–`a6`), servidor temporal nuevo por dispositivo | 5582                         | 6/6 aprobados                                                                                                                                                                                                                                                                                                     |
| `scripts/android-e2e.sh --suite adjustments`                                                      | 5580                         | 6/6 aprobados                                                                                                                                                                                                                                                                                                     |
| Capturas de pantalla (vertical y horizontal)                                                      | los dos                      | 44 en `$SP/shots-C/telefono-ADEL_A1_Phone_API33/` y `$SP/shots-C/tableta-Pixel_Tablet/`                                                                                                                                                                                                                           |

Lo que fallaba en el camino y se corrigió (todo vuelto a correr después):

- `StageA3ScreensTest` en la tableta: el título de la cuenta se buscaba en el árbol junto (el encabezado que se toca
  junta sus textos) y la pestaña de Terraza estaba fuera de la vista; y la flecha medía **40 dp** (`TopAppBar` deja
  así su `IconButton`): ahora se fija en 48 dp (regla 12).
- `a2` (mismo motivo del árbol junto) y `a4` en el teléfono: el aviso «Lo capturado…» (4 s) **tapaba «Ver la cuenta»**
  de la captura siguiente y el toque no llegaba al botón. Corregido en la aplicación (el aviso es de la pantalla a la
  que se vuelve y se quita al pasar a otra) y la prueba lo comprueba en Pendientes.
- Las capturas del teléfono vertical mostraban la pestaña del área cortada («alón A3»): no cabía ni una junto a
  «Lista | Mapa» y «Pendientes». Ahora la elegida se trae a la vista y «Lista | Mapa» es un interruptor compacto;
  `StageA3ScreensTest` comprueba a 400 dp que la pestaña elegida se ve completa.
- Una vuelta de las instrumentadas en la tableta (`C3`) falló en `AppNavigationTest` y `MainNavigationLayoutTest`
  porque **no desinstalé** la aplicación que dejó la prueba de punta a punta (con «pantalla inicial: Para llevar»
  guardada, entraba a Para llevar como debe). Con la aplicación desinstalada pasaron (dos vueltas de esas dos clases
  y la vuelta completa `C5`, y después `C7`).

### No verificado

- Dispositivo físico (ningún teléfono ni tableta reales), papel de una impresora real.
- `ADEL_A1_Tablet_API26` (Android 8) y `Pixel_8` no se corrieron en esta fase (solo los dos emuladores del encargo).
- Las suites de punta a punta anteriores (`access`, `catalog`, `waiter`, `field`, `takeaway`, `precheck`, `alerts`,
  `connection`) no se volvieron a correr en esta fase. Usan las mismas etiquetas (con un panel abierto en el teléfono la flecha
  conserva la etiqueta `pane-back` y «Ver la cuenta» la de `pane-open-secondary`), pero Mesas cambió (sin búsqueda ni
  «Ver la mesa») y el aviso al salir es nuevo: conviene correr `--suite all` en la Fase E.
- La prueba «Descartar cuenta vacía» con `ACCOUNT_NOT_EMPTY` de verdad (otra terminal que llena la cuenta) solo está en
  la JVM (`TablePanelAccountsTest`), no contra el Go temporal.

### Decisiones del coordinador sobre la Fase C

Fusionada en `beeb183`. Fuera de `android/`, la Fase C solo cambió `docs/ANDROID_INSTALAR.md` y `scripts/android-e2e.sh`.

- **A3-D21**: se aceptan las siete decisiones propuestas del agente. Sobre la primera: en la aplicación, Mesas es la base
  de la navegación y Para llevar va encima, así que la flecha de Para llevar regresa a Mesas también cuando la pantalla
  inicial del dispositivo es Para llevar (lo mismo que la tecla Atrás); la barra «Mesas | Para llevar» sigue siempre a
  la vista y §4.12 se cumple porque el mesero entra directo en Para llevar.
- La guía §4 (que el agente no tocaba) dice ahora «Selecciona tu usuario» en el bloqueo por inactividad; la frase sobre
  el botón de la precuenta sin permiso ya la había reescrito la Fase B.
- El texto para `CLAUDE.md` de la raíz sobre la 0.3.0 se une en la Fase E con el de la Fase A.
