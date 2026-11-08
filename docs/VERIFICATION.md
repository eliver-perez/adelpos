# Verificación · Etapa 5

Ejecución del **8 de noviembre de 2026**, zona America/Monterrey. Proyecto existente, sin reinicialización. Versión **0.5.0**, API v1, esquema **5**. Evidencia anterior: [etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y [etapa 1](VERIFICATION_STAGE1.md).

## Resultados ejecutados

| Comprobación          | Resultado                                                                                                                                                                                                                                 |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato y metadatos   | gofmt, Prettier, cargo fmt y sincronización aprobados. Revisión manual de instrucciones largas del módulo nuevo.                                                                                                                          |
| `scripts/dev.sh test` | Go con race detector y vet; frontend **31 pruebas en 9 archivos**; TypeScript estricto y build Vite; Rust **10 aprobadas**, una prueba opt-in de escritura real de Keychain omitida deliberadamente.                                      |
| PostgreSQL            | Las cuatro suites de database, administration, catalog y operations pasaron en bases aleatorias `adel_pos_test_*`, eliminadas al terminar. La aceptación operativa se amplió y se repitió después de revisar combos, traslados y formato. |
| Navegadores           | Recorrido completo aprobado en Chromium y WebKit con Go/PostgreSQL reales; producción frontend y puente IPC de prueba exclusivo de Node.                                                                                                  |
| Bundle macOS          | `.app` ARM64 0.5.0 compilado correctamente, 9.42 MiB.                                                                                                                                                                                     |
| Fullscreen nativo     | Inicio verdadero, salida a ventana verdadera y regreso verdadero. Tauri reportó contenido de **3456×2168 píxeles físicos** al iniciar y al regresar.                                                                                      |
| Instalación real      | Respaldo restaurado, migración 0005 `applied:1`, repetición `applied:0`, checksums 0001–0004 intactos y readiness 200.                                                                                                                    |
| Procesos              | Recuperación ante caída de PostgreSQL/Go, cierre/reapertura Tauri, server-only y terminal-only aprobados.                                                                                                                                 |
| Autenticación nativa  | Dispositivo y sesión existentes con actividad reciente, comprobada con booleanos de `last_seen_at`; no se leyeron ni mostraron tokens.                                                                                                    |
| Hardware              | Una comanda técnica enviada a `Printer_POS_80`; CUPS aceptó `Printer_POS_80-219`. No se confirmó papel o corte físico.                                                                                                                    |

Los logs y reportes privados están excluidos del repositorio. Principales: `.local/stage5-test-final.log`, `stage5-integration-final.log`, `stage5-operations-final.log`, `stage5-browser-final.log`, `stage5-build.log`, `stage5-window-verification.json`, `stage5-installation-verification.json`, `stage5-native-auth-verification.json`, `stage5-runtime.log`, `stage5-terminal.log` y `stage5-hardware.json`.

## Aceptación funcional y matriz solicitada

La suite `TestRestaurantAcceptance` utiliza HTTP autenticado, permisos efectivos, transacciones y PostgreSQL reales. Los transportes controlados de las pruebas de cola no imprimen físicamente. El opt-in de hardware se ejecutó por separado una sola vez. La tabla identifica cobertura y distingue los límites físicos.

| N.º | Escenario                     | Evidencia o límite                                                                                                                      |
| --- | ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Crear/editar estaciones       | API de aceptación y formularios de ambos navegadores; revisión optimista.                                                               |
| 2   | Desactivar estación           | Edición real; modificación comercial conserva aviso pendiente y rechaza avance de versión anterior.                                     |
| 3   | Ruta por categoría            | Configuración API/UI y liberación de productos/componentes.                                                                             |
| 4   | Excepción por producto        | Producto con dos estaciones prevalece sobre categoría con una; cambio posterior no reescribe pedido liberado.                           |
| 5   | Varias estaciones             | Dos unidades comerciales producen cuatro tareas y solo dos marcadores canónicos.                                                        |
| 6   | Destinos duplicados           | Se rechaza una ruta con la misma estación repetida.                                                                                     |
| 7   | Ruta inválida                 | Pedido conservado/bloqueado, advertencias y exención Sin preparación explícita.                                                         |
| 8   | Snapshot de ruta              | Edición de ruta actual no elimina el destino histórico de una modificación.                                                             |
| 9   | Comanda de confirmado         | Documentos/cola persistidos en la misma integración transaccional.                                                                      |
| 10  | Borrador sin comanda          | Conteos antes/después de guardar borrador sin nuevos trabajos.                                                                          |
| 11  | Liberación única              | Repetir confirmación y liberar nuevamente no duplica documentos ni marcadores.                                                          |
| 12  | Nuevos productos normales     | Agregar otro lote produce dos documentos `normal`, sin aviso de modificación para ese lote.                                             |
| 13  | MODIFICACIÓN                  | Documentos por estaciones afectadas y pérdida del estado listo anterior.                                                                |
| 14  | CANCELACIÓN                   | Avisos por estación, cantidad cancelada e identidades conservadas.                                                                      |
| 15  | Copia sin REIMPRESIÓN         | Mismo contenido histórico, nuevo trabajo físico, sin otra liberación.                                                                   |
| 16  | Sin ADICIÓN                   | Plantilla y documentos entregados al adaptador controlado excluyen esa etiqueta.                                                        |
| 17  | Complementos/notas            | Documento conserva Guarniciones y notas con café/ñ; pruebas anteriores cubren variantes, porciones e ingredientes.                      |
| 18  | Dividir sin imprimir          | Conteos de trabajos/liberaciones permanecen tras división de cuenta preparada.                                                          |
| 19  | Mover mesa sin imprimir       | Traslado de cuenta preparada conserva conteo de trabajos. Transferencias/combinaciones previas conservan UUID y centavos.               |
| 20  | Configurar impresoras         | API/UI, validación de destinos y prueba CUPS real independiente.                                                                        |
| 21  | Impresora compartida          | Dos estaciones printer/both referencian una impresora.                                                                                  |
| 22  | Tickets independientes        | Dos documentos/trabajos para estaciones de la misma impresora.                                                                          |
| 23  | Ticket unificado              | Un documento con dos secciones y un trabajo físico.                                                                                     |
| 24  | Corte por capacidad           | Validación y bytes ESC/POS de corte autorizado; no se certificó corte físico.                                                           |
| 25  | Sin intercalación             | Dos trabajadores independientes: el segundo no envía mientras el primero conserva el lock por impresora.                                |
| 26  | Fallos registrados            | Transporte controlado devuelve error y persiste intentos/resultados.                                                                    |
| 27  | Pendientes tras reinicio      | Trabajador nuevo consume cola persistida; la cola no depende del proceso Tauri.                                                         |
| 28  | Detectar incertidumbre        | Lease de procesamiento vencida se recupera como `uncertain`.                                                                            |
| 29  | Sin reenvío ciego             | Ningún nuevo envío automático ni respaldo ante resultado desconocido.                                                                   |
| 30  | Reintentos concurrentes       | UUID idempotente, locks y rechazo de copia adicional si otra de la familia está pendiente.                                              |
| 31  | Redirección autorizada        | Respaldo explícito solo tras tres errores sin envío, sin cadena automática adicional.                                                   |
| 32  | Avance manual                 | HTTP y acciones gráficas En preparación/Listo con revisión.                                                                             |
| 33  | Avance por partida            | Cambia solo las tareas seleccionadas/autorizadas.                                                                                       |
| 34  | Avance por pedido             | Configuración order rechaza actualización de partida y acepta lote completo.                                                            |
| 35  | Consolidación                 | Una estación lista no oculta la otra; ambas listas completan la cuenta.                                                                 |
| 36  | Pago/entrega separados        | Preparación lista conserva `not_recorded` / `not_delivered`.                                                                            |
| 37  | Programado automático         | Vencimiento/anticipación persistidos y ejecución del trabajador Go.                                                                     |
| 38  | Programado manual             | Liberación anticipada explícita con permiso correspondiente.                                                                            |
| 39  | Reinicio sin doble liberación | Dos instancias sucesivas del planificador conservan un marcador por unidad.                                                             |
| 40  | Tiempos/advertencias          | Configuración, valores y antigüedad entregados por Go y presentados en el tablero; no es certificación de tiempos bajo carga comercial. |
| 41  | Conexión autenticada          | SSE HTTP real con sesión/dispositivo existentes en base temporal.                                                                       |
| 42  | Rechazo anónimo               | SSE sin credenciales devuelve 401.                                                                                                      |
| 43  | Otras terminales              | Cambio confirmado recibido en stream y filtrado por permisos/estación de otra terminal lógica.                                          |
| 44  | Replay                        | Snapshot desde cursor previo recupera eventos; cursor consumido no repite el lote.                                                      |
| 45  | Deduplicación                 | UUID únicos en lote y hook React que descarta repetidos y mensajes de identidad anterior.                                               |
| 46  | Estado completo               | Más de 5000 secuencias producen reset; hook invalida y conserva sondeo de recuperación.                                                 |
| 47  | Revocación conectada          | Stream de sesión revocada informa 401 y cierra.                                                                                         |
| 48  | Inicio fullscreen             | Comprobación real Tauri con `is_fullscreen` y tamaño físico.                                                                            |
| 49  | Salir/regresar                | APIs nativas reales y prueba del control visible React.                                                                                 |
| 50  | Resoluciones                  | 1280×720, 1920×1080, 2560×1440, 1024×768 y 900×1200 en ambos motores.                                                                   |
| 51  | Conservar formulario          | La cantidad en personalización permanece al redimensionar.                                                                              |
| 52  | Temas                         | Capturas y recorrido claro/oscuro aprobados.                                                                                            |
| 53  | Acordeones                    | Enter/Espacio, conservación de valores y componente compartido, además de pruebas unitarias.                                            |
| 54  | Mouse/táctil                  | Interacción con mouse, teclado y teclado virtual; no se dispone de certificación en pantalla táctil física.                             |
| 55  | Go race/vet                   | Aprobados.                                                                                                                              |
| 56  | PostgreSQL                    | Cuatro paquetes en bases temporales; conservación de etapas anteriores.                                                                 |
| 57  | Frontend                      | 31 pruebas aprobadas.                                                                                                                   |
| 58  | TypeScript estricto           | `tsc --noEmit` aprobado.                                                                                                                |
| 59  | Rust                          | 10 pruebas aprobadas; Keychain opt-in de escritura omitida, lectura existente sí verificada nativamente.                                |
| 60  | macOS                         | Bundle ARM64 compilado y ejecutado.                                                                                                     |
| 61  | Procesos independientes       | Pruebas nativas de cierre, desconexión y recuperación aprobadas.                                                                        |
| 62  | Etapas anteriores             | Suites de migración, identidad, administración, catálogo y restaurante conservadas y aprobadas.                                         |
| 63  | Migración sin pérdida         | Restauración completa de 46 tablas, checksums anteriores conservados y 63 tablas finales sin reducción de conteos anteriores.           |

Además se verificó el enrutamiento de papas elegidas dentro de un combo: dos combos mantienen dos unidades canónicas y sus componentes llegan con cantidad dos por unidad a la estación explícita. La recepción KDS conserva una sola fila ante repetición y no equivale a preparación o impresión. La cola rechaza copiar un resultado incierto o acumular otra copia sin resolver.

## Interfaz y fullscreen

El recorrido gráfico usa frontend compilado, backend real y PostgreSQL temporal por motor. Configura impresora de tipo agente claramente no disponible, dos estaciones, ruta por categoría y agrupación unificada. Captura/confirmación generan preparación y documentos reales; el destino sin agente permanece en intervención, y una resolución manual cancela solo el trabajo de impresión. No se imprime desde estas pruebas gráficas.

Se verificaron también áreas/mesas, distribución mediante teclado, borrador, confirmación, edición con motivo, historial, división y pedido programado de Ventanilla. Las capturas de monitores grandes muestran columnas fluidas sin los antiguos límites máximos; los formularios mantienen su selección. Temas, acordeones, teclado virtual y ausencia de errores JavaScript se comprobaron en ambos motores. Capturas revisadas en `.local/browser-artifacts/chromium/` y `webkit/`.

La prueba nativa opt-in consultó la ventana Tauri real: fullscreen inicial, modo ventana y regreso aprobados. La medida física reportada fue 3456×2168; no se confundió con el viewport simulado de Playwright. Conserva modo ventana normal y Spaces. No se certificaron todos los monitores, equipos táctiles o gestos de macOS.

## Respaldo y migración real

Respaldo custom: `backups/adel_pos-20261108T150810Z-902294.dump`. Se restauró completamente con `pg_restore --exit-on-error` en una base temporal y se compararon las **46 tablas existentes**, con conteos iguales. Se eliminó esa base temporal. Evidencia en `.local/stage5-restore-verification.json`.

Después se detuvo solo Go, se aplicó 0005 (`applied:1`), se repitió (`applied:0`) y se reinició el servidor. Las cinco migraciones quedan aplicadas; los nombres/checksums de las cuatro anteriores coinciden con la línea base. La 0005 añade **17 tablas y 20 permisos**, sin modificar usuarios o contraseñas ni crear estaciones/impresoras/productos de ejemplo. Se verificaron **63 tablas finales**, sin reducción de conteos anteriores.

La instalación conserva **ADEL POS**, **America/Monterrey**, **MXN**, dos usuarios y una terminal. Catálogo, áreas, mesas, cuentas y configuración de destinos reales siguen sin datos ficticios. Readiness devuelve 200, PostgreSQL disponible/esquema compatible, y `/version` devuelve 0.5.0/esquema 5/eventos disponibles. Reporte privado: `.local/stage5-installation-verification.json`.

## Aplicación nativa y recuperación

Se abrió el bundle 0.5.0 usando el perfil de credenciales existente; actividad reciente de dispositivo y sesión se confirmó sin leer tokens. No se cambiaron sa, contraseñas ni la zona configurable.

`python3 tests/runtime_smoke.py` aprobado: caída de PostgreSQL conserva Go y live=200/ready=503; recuperación vuelve a ready=200 sin reiniciar Go; proceso server-only temporal en 8182; cerrar/reabrir Tauri conserva Go; detener Go conserva la ventana y al reiniciar recupera consultas sin reiniciar Tauri. `--terminal-only` también aprobado. Se retiraron los procesos temporales y quedaron los servicios propios activos.

Durante los primeros intentos, el propietario cerró manualmente las ventanas; confirmó esa intervención. Se repitieron las comprobaciones manteniéndolas abiertas y pasaron. No se atribuyó ese cierre voluntario a un fallo de conexión. Los reportes aprobados se archivaron como `.local/stage5-runtime-verification.json` y `stage5-terminal-mode-verification.json`.

## Hardware y límites

Se descubrieron cuatro colas CUPS locales; se seleccionó expresamente la térmica USB **Printer_POS_80**, sin utilizar la impresora de oficina predeterminada. Primero se generó una vista previa privada sin enviar. Después se ejecutó un solo test opt-in, una copia de 80 mm sin corte, con el transporte CUPS real y cola/intento persistidos en base temporal.

Resultado: `submitted`, referencia **Printer_POS_80-219**, mensaje **Aceptado por CUPS; no confirma salida física de papel**, `physical_confirmed:false`. Se solicitó la limpieza exclusivamente de ese trabajo de ensayo si aún seguía en el spooler. La base temporal se eliminó. No se afirmó salida de papel, legibilidad final, corte o compatibilidad física de otros modelos. No se repitió automáticamente el envío.

ESC/POS TCP está implementado y sus bytes/acentos/corte autorizado se verificaron sin una segunda impresora de red física. El agente futuro informa indisponibilidad; no existe un agente funcional ni una aplicación KDS independiente. Las pruebas de cola usan adaptadores controlados para fallos, incertidumbre, serialización y respaldo.

Persisten los límites de licenciamiento comercial, firma/notarización, servicio instalado, carga elevada, muchas terminales/otra máquina LAN, Mac Intel, Windows, Android, resistencia a corte eléctrico, retención y respaldo periódico. No hay cobros, entrega habilitada, turnos ni inventario físico. El replay de 5000 secuencias es una ventana lógica; aún falta retención física. Una base y una impresora no comparten commit: se conserva incertidumbre en vez de prometer exactamente una copia física.

## Correcciones previas a la etapa 6

Ejecución del **8 de noviembre de 2026**, tras corregir H1, M1, H2, M3, M4 y B1 del [diagnóstico](DIAGNOSTICO_ETAPAS_1_5.md#11-correcciones-aplicadas-antes-de-la-etapa-6). Versión y esquema sin cambios (0.5.0 / 5); **no hay migraciones nuevas**.

| Comprobación                        | Resultado                                                                                                                                                                                                                                                                                                                                                                                                        |
| ----------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `./scripts/dev.sh test`             | Aprobado: Prettier, gofmt, cargo fmt, metadatos, Go con race detector y vet, **31 pruebas frontend en 9 archivos**, TypeScript y build Vite, Rust **10 aprobadas** y 1 opt-in omitida.                                                                                                                                                                                                                           |
| `./scripts/dev.sh test-integration` | Aprobado en bases `adel_pos_test_*` eliminadas al terminar: database, administration, catalog y operations. Nuevas: `TestDeniedOperationRollsBackItsWrites` (3 casos), subpruebas `H1_*` (4) y `M1_*` de `TestStageTwoAcceptance`, y en `TestRestaurantAcceptance` la reprogramación de cocina ante ediciones de la cuenta y el respaldo tras un envío autorizado. `TestControlledCUPSPrinter` omitida (opt-in). |
| `./scripts/dev.sh test-browser`     | Aprobado en Chromium y WebKit.                                                                                                                                                                                                                                                                                                                                                                                   |
| Detección de los defectos           | Las pruebas nuevas de H1, M1, M3 y M4 se ejecutaron contra el código anterior en una copia aislada y fallaron allí; la de H2 ejercita la función nueva `runProtected`.                                                                                                                                                                                                                                           |
| CSS (B1)                            | Ninguna variable `var(--…)` usada queda sin definir.                                                                                                                                                                                                                                                                                                                                                             |

No se ejecutaron `build-macos`, `runtime_smoke.py` ni la impresión física: no cambiaron Tauri ni los procesos, y el ensayo de procesos detiene los servicios reales. El servidor Go que está en ejecución en la instalación real conserva el binario anterior hasta que el propietario lo reinicie; `bin/ad-el-pos-server` se recompiló durante `test-browser`.

## Repetir las comprobaciones

```bash
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
./scripts/dev.sh build-macos
python3 tests/runtime_smoke.py
python3 tests/runtime_smoke.py --terminal-only
```

Ejecutar los dos ensayos de procesos secuencialmente, sin pruebas PostgreSQL ni trabajo real simultáneo. La impresión física es opt-in separada; las suites ordinarias nunca la ejecutan. Uso, contratos y políticas en [KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md), [API_CONTRACTS.md](../API_CONTRACTS.md) y [DEVELOPMENT_MACOS.md](../DEVELOPMENT_MACOS.md).
