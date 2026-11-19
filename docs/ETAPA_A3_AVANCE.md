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
