# Verificación · Etapa 9G

Verificación por fases del **13 y 14 de noviembre de 2026**, en la Mac (Apple Silicon, macOS 26.5.2), en ramas locales
por fase que el coordinador fusionó en `etapa-9g` (creada desde `main` = 0.9.5, esquema 22, con la 9F fusionada): fase
A `dcce2c9` y `aa748b7`; fase D `f670382`, `7842d60`, `dbf6c19` y `545683b` (fusión con A en `72b81dc` y `67741a2`);
fase B `7cb3231`, `e487660` y `5b9cb88` (fusión en `bf6fa95` y `c502706`); fase F `7bf10ba` y `39a0390` (fusión en
`a7989e9` y `b84415c`); fase C `62b6400` y `43e714f` (fusión en `146ffc2` y `65bb484`); correcciones de la revisión de
la fase B `809740f` (fusión en `6e92f7a`); fase E `6731013`, `47e4d29` y `0e13097` y correcciones de la revisión de la
fase F `3948fdc` (fusiones en `dc638d1` y `39b8b0c`); correcciones de la revisión de la fase D `e9d35fc` y `9eb8a49`
<!-- PENDIENTE-COORDINADOR: commit de la fusión de las correcciones de D y de esta documentación (Fase G). -->.

Versión **0.10.0** (se fija en la Fase G), API v1, migración `0023_network_terminals_and_android.sql` (esquema **23**,
con las secciones D, B, F y E). Especificación: [etapas/ETAPA_9G.md](etapas/ETAPA_9G.md). Avance con el detalle de cada
fase: [ETAPA_9G_AVANCE.md](ETAPA_9G_AVANCE.md). Evidencia anterior: [etapa 9F](VERIFICATION_STAGE9F.md) y las
anteriores desde [VERIFICATION.md](VERIFICATION.md). Este documento registra **solo lo que se ejecutó**.

**Alcance de esta evidencia.** Las cifras son las que cada agente registró sobre su propia copia y las de los cierres
del coordinador sobre una copia exacta (`git archive`) de cada fusión, según el avance. Al escribir este documento, el
avance no registra todavía el cierre del coordinador de las fusiones de la fase E (`39b8b0c`) y de las correcciones de
la fase F (`dc638d1`), y las correcciones de la fase D (`e9d35fc`, `9eb8a49`) tienen las pruebas de su agente pero aún
no estaban fusionadas: lo que validará el código completo es la «Regresión final».

**Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432, `backups/`, el `.app`
instalado, el puerto 8443 ni `/Library`): ninguna usó `sudo`, `installer` ni `launchctl` (salvo el `launchctl`
simulado de las pruebas de los scripts del instalador), ni escribió en `/Library`, `/Applications` o
`~/Library/LaunchDaemons`. Las carpetas «del sistema» de las pruebas fueron carpetas temporales. Lo único que se hizo
con la instalación real fue lo que registra la Fase 0: la Mac se había reiniciado y la instalación estaba detenida; al
terminar la Fase 0 el coordinador la volvió a arrancar con el mismo código de `main` (`db-start` y `server-start`,
readiness 200, 0.9.5 / esquema 22, `test_mode: false`), sin abrir la ventana. Ninguna prueba envió trabajos a una
impresora, a un cajón ni a CUPS real (el agente de impresión se probó con un ejecutor simulado de `lpstat`/`lp`).

## Entorno aislado

Un árbol de trabajo (`git worktree`) por fase en el directorio temporal de la sesión, un clúster PostgreSQL temporal
en el puerto 55499 (solo loopback) compartido por los agentes, una base `adel_pos_test_*` desechable por paquete y por
motor de Playwright, `GOPROXY=off`, Rust con su propia carpeta de compilación por fase y Go temporal y vista previa en
puertos propios: 8184/1422 en la A (más clústeres propios de la prueba del `host` en 55511–55513 y Go en 8192–8194),
8185/1423 en la D, 8186/1424 en la B y sus correcciones (escucha HTTPS de prueba en `127.0.0.1:8196` y `8197`),
8187/1425 en la F y sus correcciones (`TMPDIR` dentro del directorio temporal), 8188/1426 en la C (anuncios mDNS de
prueba de unos segundos con nombres únicos y puertos 8201/8202 sin escucha) y 8190/1428 en la E. El Go temporal de
Playwright corre con `test_mode: true`. La pantalla del equipo estuvo desbloqueada en las corridas de Playwright
registradas (`CGSSessionScreenIsLocked` ausente), así que WebKit es una aprobación real.

**Pruebas automatizadas, no físicas.** Vitest con jsdom y Playwright con Chromium y WebKit sustituyen el puente nativo
por la simulación de `e2e/native-driver.ts` (modalidad, emparejamiento, búsqueda, segunda terminal, diálogo de carpeta
y agente de impresión simulados). **No** sustituyen la ventana nativa, el instalador real, launchd, el Keychain real,
una segunda computadora en la red, el permiso «Red local» de macOS, una USB real, papel ni Windows. Las dos terminales
de las pruebas son dos credenciales en la misma Mac.

## Fase 0 · verificación contra el código

Un solo agente, de solo lectura. Hallazgos, plan y decisiones D1–D14 del coordinador en el avance («Fase 0»); D15
(identidad del servidor y respaldos) se agregó tras la fase B. Ninguna cambió la especificación.

## Resultados ejecutados por fase

«Agente»: corrida del agente de la fase sobre su copia; «cierre»: corrida del coordinador sobre una copia exacta de la
fusión. Integración con `-tags=integration` y `-race`; tiempos de `go test`. «Estáticas» = formato (Prettier, `gofmt`,
`cargo fmt`; `docs/etapas` intacto por md5), `sync --check`, `go vet` (normal e `integration`) y `GOOS=windows`
(`go build` y `go vet`).

| Fase o parte (commit)                        | Estáticas y Go unitarias       | Integración PostgreSQL                                                                                                                                                                                                   | Interfaz y Rust                                                                                                                                                                                                                      |
| -------------------------------------------- | ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| A, agente (`aa748b7`)                        | Aprobadas; Go con `-race` 14 s | `cmd/ad-el-pos-server` (`TestServiceHostOnMacOS`) 41–43 s, `administration` 82 s                                                                                                                                         | `cargo test` 54 aprobadas, 1 ignorada; `tsc`; Vitest 87 archivos, 835 pruebas; Playwright 4/4 Chromium y 4/4 WebKit; scripts del instalador con destino falso: 31 comprobaciones; `.pkg` compilado y examinado (20 MB, sin instalar) |
| A, cierre (`aa748b7`)                        | Aprobadas                      | `cmd/ad-el-pos-server` 43 s, `administration` 92 s                                                                                                                                                                       | `tsc`, Vitest y `cargo test` aprobados                                                                                                                                                                                               |
| D, agente (`545683b`)                        | Aprobadas; Go con `-race` 17 s | 10 de 10: `database` 11 s, `administration` 94 s, `catalog` 22 s, `operations` 306 s, `apicontract` 9 s, `payments` 4 s, `reports` 126 s, `reports/operational` 45 s, `designer` 25 s, `identity` 21 s                   | `cargo test` 47 aprobadas, 1 ignorada; `tsc`; Vitest 89 archivos, 830 pruebas; Playwright 2/2 y 5/5 en Chromium y en WebKit                                                                                                          |
| A + D, cierre (`72b81dc`, `67741a2`)         | Aprobadas                      | 11 de 11 (`operations` 310 s, `reports` 130 s, `administration` 92 s, `reports/operational` 45 s, `cmd/ad-el-pos-server` 43 s y los demás)                                                                               | `cargo test`; Playwright 7/7 en Chromium y en WebKit; Vitest falló en `compatibility.test.tsx` (no simulaba `installation_state`), corregido en `67741a2` y repetido con `tsc` aprobado                                              |
| B, agente (`5b9cb88`)                        | Aprobadas; Go con `-race` 15 s | `administration` 108 s (`testPairingAndTerminals` con las dos escuchas reales), `cmd/ad-el-pos-server` 46 s                                                                                                              | `cargo test` 65 aprobadas, 2 ignoradas; prueba en vivo Rust → Go aprobada; `tsc`; Vitest 88 archivos, 847 pruebas; Playwright 7/7 en Chromium y en WebKit                                                                            |
| A + D + B, cierre (`bf6fa95`, `c502706`)     | Aprobadas                      | 11 de 11 (`administration` 106 s)                                                                                                                                                                                        | `cargo test`, `tsc` y Vitest; Playwright falló en `workflow-stage9g-terminals` (la prueba de «Agotado» enrolaba «Caja 2 9G»), corregido en `c502706`: 6/6 en Chromium y en WebKit                                                    |
| F, agente (`39a0390`)                        | Aprobadas; Go con `-race` 15 s | `backupschedule` 7 s (con `pg_dump`/`pg_restore` reales), `administration` 105 s, `database` 11 s, `cmd/ad-el-pos-server` 49 s                                                                                           | `cargo test` 61 aprobadas, 1 ignorada; `tsc`; Vitest 91 archivos, 856 pruebas; Playwright 6/6 en Chromium y en WebKit (y 3/3 sobre el último commit)                                                                                 |
| A + D + B + F, cierre (`a7989e9`, `b84415c`) | Aprobadas                      | 12 de 12 (`operations` 308 s, `reports` 130 s, `administration` 117 s, `reports/operational` 45 s, `cmd/ad-el-pos-server` 43 s y los demás)                                                                              | `tsc`, Vitest, `cargo test`; Playwright 10/10 en Chromium y en WebKit                                                                                                                                                                |
| C, agente (`43e714f`)                        | Aprobadas; Go con `-race` 16 s | `discovery` 5 s (anuncio real acotado), `operations` (`TestStage9GTwoTerminals`) 13 s, `apicontract` 7 s (26 operaciones), `administration` 97 s, `cmd/ad-el-pos-server` 42 s                                            | `cargo test` 75 aprobadas, 3 ignoradas; prueba en vivo Rust ↔ Go aprobada; `tsc`; Vitest 92 archivos, 861 pruebas; suite `app` (97 s, `Info.plist` con las claves de red local); Playwright 11/11 en Chromium y en WebKit            |
| + C, cierre (`146ffc2`, `65bb484`)           | Aprobadas                      | 13 de 13 (con `discovery`)                                                                                                                                                                                               | `tsc`, Vitest, `cargo test`; Playwright 14/14 en Chromium y en WebKit                                                                                                                                                                |
| Correcciones de B, agente (`809740f`)        | Aprobadas; Go con `-race` 16 s | `administration` 116 s, `cmd/ad-el-pos-server` 46 s                                                                                                                                                                      | `cargo test` 73 aprobadas, 2 ignoradas; prueba en vivo Rust → Go aprobada; `tsc`; Vitest 92 archivos, 870 pruebas; Playwright 4/4 en Chromium y en WebKit                                                                            |
| + correcciones de B, cierre (`6e92f7a`)      | Aprobadas                      | 13 de 13                                                                                                                                                                                                                 | `tsc`, Vitest, `cargo test` 81 aprobadas, 3 ignoradas; Playwright 14/14 en Chromium y en WebKit                                                                                                                                      |
| E, agente (`0e13097`)                        | Aprobadas; Go con `-race` 21 s | `operations` 321 s (con `TestStage9GPrintAgent`; la primera corrida falló en `C37` por el texto nuevo del cajón, corregido), `database` 10 s, `administration` 117 s, `payments` 3 s, `designer` 23 s, `apicontract` 9 s | `cargo test` 88 aprobadas, 3 ignoradas; `tsc`; Vitest 94 archivos, 880 pruebas; Playwright 8/8 en Chromium y en WebKit                                                                                                               |
| Correcciones de F, agente (`3948fdc`)        | Aprobadas; Go con `-race` 17 s | `backupschedule` 10 s, `administration` 121 s, `database` 12 s, `cmd/ad-el-pos-server` 47 s                                                                                                                              | `tsc`; Vitest 93 archivos, 876 pruebas; Playwright 3/3 en Chromium y en WebKit; sin cambios de Rust                                                                                                                                  |
| Correcciones de D, agente (`9eb8a49`)        | Aprobadas; Go con `-race` 16 s | `apicontract` 10 s, `database` 15 s, `catalog` 22 s, `cmd/ad-el-pos-server` 45 s (restauración real que cambia la época del catálogo), `administration` 120 s, `operations` 316 s                                        | Playwright 3/3 en Chromium y en WebKit (la primera corrida de `workflow-stage9g-sold-out` falló porque esperaba que desapareciera el aviso del cálculo, que es correcto; se ajustó la especificación); sin cambios en `desktop/src`  |
| + E y correcciones de F, cierre (`39b8b0c`)  | PENDIENTE-COORDINADOR          | PENDIENTE-COORDINADOR                                                                                                                                                                                                    | PENDIENTE-COORDINADOR                                                                                                                                                                                                                |
| + correcciones de D, cierre                  | PENDIENTE-COORDINADOR          | PENDIENTE-COORDINADOR                                                                                                                                                                                                    | PENDIENTE-COORDINADOR                                                                                                                                                                                                                |

**Pruebas en vivo y comprobaciones fuera de las suites**: prueba en vivo de Rust contra un Go temporal
(`request_pairing` real con `reqwest` + `rustls` + el verificador: la huella de la bitácora es la de `openssl`, HTTP por
la escucha de red → 400, TLS 1.1 rechazado, la otra huella no conecta y el código de Rust coincide con el que se deriva
de lo guardado por Go), búsqueda real de `discovery.rs` contra un anuncio de Go de 12 s (sin `dns-sd` vivo al
terminar), sondeo previo de mDNSResponder (responde la consulta de una sola vez por unicast con PTR, SRV, TXT, AAAA y
A) y el `.pkg` desarmado con `pkgutil --expand` (tres opciones, `hostArchitectures="arm64"`, aplicación no reubicable
con firma ad hoc de paquete verificada, scripts idénticos a los del repositorio y plantilla del plist válida). El
servidor del paquete, con `ADEL_TEST_MODE=1`, rechaza la carpeta real sin crearla.

**Controles negativos**: renombrar un campo de la respuesta de «Agotado» hizo fallar `TestContractV1` (cambio
revertido, fase D).

## Revisión independiente

Un solo agente de solo lectura, en tres partes, sobre copias exactas (`b84415c`), con pruebas desechables propias
contra el clúster aislado; las correcciones las hicieron agentes de cada fase en ramas nuevas.

**Parte 1 · emparejamiento y cifrado (fase B).** Sin hallazgos críticos.

| Hallazgo                                                                                                                           | Corrección (`809740f`)                                                                                                                                                          |
| ---------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Alto · un intermediario podía «moler» solicitudes (3 000 creaciones y cancelaciones en 3,3 s; ≈ 1–5 % de éxito por emparejamiento) | Tope global de 30 solicitudes creadas cada 10 minutos (contando canceladas y rechazadas) y 5 por grupo de origen (IPv4, /64 de IPv6, todo el enlace local); aviso en Terminales |
| Medio · llenar la base desde la red sin credencial                                                                                 | Conteos por índice sobre la ventana, limpieza por lotes en un trabajador (`PairingCleaner`) y auditoría solo al revelar                                                         |
| Bajo · consultas de estado simultáneas podían dejar un dispositivo huérfano                                                        | Una sola consulta en curso en Rust, credencial reclamada conservada y consultas encadenadas en React                                                                            |
| Bajo · una identidad incompleta impedía arrancar todo el servidor                                                                  | Identidad creada de forma atómica; sin identidad cargable el servidor sigue con la escucha local (`503 PAIRING_UNAVAILABLE`)                                                    |
| Bajo · el filtro por ULA aceptaba nodos de VPN superpuestas                                                                        | Solo orígenes dentro de la subred de una interfaz local que no sea punto a punto                                                                                                |
| Extra · un origen de enlace local IPv6 con zona no podía emparejarse                                                               | Se quita la zona antes de leer la IP                                                                                                                                            |

**Parte 2 · idempotencia y pedidos sin conexión (fase D).** Sin hallazgos críticos ni altos.

| Hallazgo                                                                                                    | Corrección (`e9d35fc` y `9eb8a49`)                                                                                                                           |
| ----------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Medio · «Solo cambios desde X» perdía productos archivados al cambiar una lista de precios o el IVA general | La descarga entrega también los archivados que cambiaron después de X                                                                                        |
| Bajo · una base restaurada reutilizaba números de versión del catálogo                                      | Época del catálogo (`catalog_epoch`), que cambia sola al restaurar o pasar a otro clúster; `since` exige `epoch` y otra época entrega todo con `reset: true` |
| Bajo · el borrador aceptaba productos agotados                                                              | `PUT …/draft` y abrir una cuenta rechazan lo que se agrega de un producto agotado (una partida ya guardada se conserva o se reduce)                          |
| Bajo · el reenvío confirmado de un pedido sin conexión recibía `IDEMPOTENCY_CONFLICT` sin el resultado      | `409 OFFLINE_ORDER_ALREADY_ACCEPTED` con `details.result` para el mismo usuario y terminal                                                                   |
| Bajo · «Agotado» de un componente o complemento no llegaba a los combos y grupos que lo contienen           | La descarga agrega esos combos y grupos con la disponibilidad de cada opción                                                                                 |

**Parte 3 · respaldos (fase F).** Sin hallazgos críticos ni altos.

| Hallazgo                                                                                                           | Corrección (`3948fdc`)                                                                                                                        |
| ------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------- |
| Medio · la retención y la limpieza de `.partial` borraban respaldos ajenos en una carpeta compartida (reproducido) | Solo se tocan archivos registrados por esta instalación en `backup_runs`, ordenados por `seq`; nombres con el identificador de la instalación |
| Medio · en el servicio el respaldo se creaba con `pg_dump --file` y solo pasaba a 0600 al terminar                 | `fileacl.CreatePrivate` (exclusivo, sin seguir enlaces, 0600 desde el primer byte) y `pg_dump` a la salida estándar                           |
| Medio · sin aviso si el respaldo dejaba de intentarse (reloj adelantado, fila «en curso» huérfana)                 | Ejecuciones «del futuro» ignoradas, orden por `seq`, filas huérfanas cerradas y aviso `overdue` a las 26 h                                    |
| Bajo · la validación de la carpeta no revisaba las carpetas superiores ni resolvía enlaces                         | Ruta real guardada; rechazadas las carpetas dentro de datos de PostgreSQL o del servidor                                                      |

## Cobertura de la etapa 9G

Correspondencia con las pruebas de §9 de la especificación. Rutas relativas a `backend/`, `desktop/src/`,
`desktop/src-tauri/src/` o `desktop/e2e/` según la capa.

| Prueba exigida (§9)                                                                               | Dónde y qué                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| ------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Servicio: sigue activo al cerrar la ventana y arranca sin sesión                                  | `cmd/ad-el-pos-server/host_unix_integration_test.go` (`TestServiceHostOnMacOS`: el binario real como proceso, habilitación, bloqueo, `SIGTERM` en orden, reinicio y restauración con el servicio en marcha). **En la computadora real: pendiente** (instalación de la Fase G; ver «No verificado»)                                                                                                                                                                                                                          |
| Modalidades: «Solo terminal» sin PostgreSQL; cambio sin perder datos                              | `TestServiceHostOnMacOS` (sin habilitación o en «Solo terminal» termina sin crear nada; quitar la habilitación detiene y conserva los datos), `installation.rs` (7 pruebas), `pages/stage9gModality.test.tsx` y `workflow-stage9g-modality.spec.ts`; scripts del instalador con destino falso                                                                                                                                                                                                                               |
| Emparejamiento: código correcto e incorrecto, vencido, intentos, revocación, certificado cambiado | `internal/administration` (`testPairingAndTerminals` y las pruebas del tope global y de los conteos por índice), `pairing.rs` (verificador con intercambio TLS completo en memoria), `pages/stage9gPairing.test.tsx` y `workflow-stage9g-terminals.spec.ts` (revocación < 8 s)                                                                                                                                                                                                                                              |
| Descubrimiento, conexión manual y cambio de IP                                                    | `internal/discovery` (anuncio real y búsqueda acotados), `discovery.rs` (dos servidores TLS reales con huella correcta e incorrecta; cambio de dirección que reescribe `connection.json`), prueba en vivo Rust ↔ Go y `workflow-stage9g-two-terminals.spec.ts` (búsqueda simulada). Cambio de IP real: pendiente                                                                                                                                                                                                            |
| Reconexión y recuperación del estado                                                              | `App.test.tsx` (capa «Sin conexión»), `workflow-stage9d-topbar-tables.spec.ts` (paso 10), `operations/stage9g_offline_test.go` (`GET restaurant/state` con permisos y terminal revocada) y `TestStage9GTwoTerminals`                                                                                                                                                                                                                                                                                                        |
| Dos terminales en la misma cuenta                                                                 | `operations/stage9g_two_terminals_test.go` (`ACCOUNT_CONFLICT`, dos confirmaciones simultáneas con un solo envío, turnos, arqueo y tope por caja) y `workflow-stage9g-two-terminals.spec.ts`                                                                                                                                                                                                                                                                                                                                |
| API: contrato v1 y versión mínima de cliente                                                      | `apicontract` (`TestContractV1`, 26 operaciones validadas contra el esquema), `httpapi` (426 por versión vieja y, por la escucha de red, sin cabecera) y `services/compatibility.test.ts`                                                                                                                                                                                                                                                                                                                                   |
| Idempotencia (§5.2)                                                                               | `catalog/stage9g_test.go` (4 reintentos simultáneos de «Agotado»), `operations/stage9g_offline_test.go` (5 aperturas y 5 confirmaciones simultáneas, respuesta perdida, otra carga, otro usuario o terminal) y «Respaldar ahora» (6 envíos simultáneos)                                                                                                                                                                                                                                                                     |
| Catálogo completo y por cambios; disponibilidad                                                   | `catalog/stage9g_test.go` (`TestStage9GCatalog`: completa, sin cambios, precio, agotado, archivado, borrado, categoría, `reset`, imagen 200/304/404; con las correcciones de D, archivados en «solo cambios», época obligatoria y distinta, combos y grupos con un componente agotado), restauración real que cambia la época (`TestServiceHostOnMacOS`), `operations/stage9g_offline_test.go` (borrador y apertura con un agotado) y `workflow-stage9g-sold-out.spec.ts` (otra terminal agota lo que ya está en la cuenta) |
| Pedidos sin conexión (§5.5)                                                                       | `operations/stage9g_offline_test.go`: cambio de precio, producto agotado y eliminado, cuenta cerrada, doble envío (también el reenvío confirmado tras una repetición tardía que sí entró: `OFFLINE_ORDER_ALREADY_ACCEPTED` con el resultado) y confirmación con catálogo viejo; `TestContractV1` valida esa respuesta                                                                                                                                                                                                       |
| Agente de impresión: terminal conectada y desconectada                                            | `operations` (`TestStage9GPrintAgent`), `kitchen/agent_test.go`, `print_agent.rs` (8 pruebas con ejecutor simulado), `pages/stage9gPrintAgent.test.tsx` y `workflow-stage9g-print-agent.spec.ts`                                                                                                                                                                                                                                                                                                                            |
| Respaldos: programación, retención, verificación y aviso                                          | `backupschedule` (unitarias e integración, también con `pg_dump`/`pg_restore` reales, lock 7820148309 y carpeta compartida), `administration` (`TestStage9GBackupRoutes`), `database` (`checkStage9GBackups`), `pages/stage9gBackups.test.tsx` y `workflow-stage9g-backups.spec.ts`                                                                                                                                                                                                                                         |
| Conteo de sesiones por tipo de dispositivo                                                        | `testPairingAndTerminals` (Android aparte; sin permiso, 403) y `pages/stage9gPairing.test.tsx`                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Migración 0023                                                                                    | `database/stage9g_android_upgrade_test.go` (de 22 a 23 con datos, con las comprobaciones de las secciones B, F y E) y las pruebas de actualización anteriores adaptadas                                                                                                                                                                                                                                                                                                                                                     |
| Go para Windows desde la Mac                                                                      | `GOOS=windows` `go build` y `go vet` (también con `-tags=integration`) en cada fase y en cada cierre                                                                                                                                                                                                                                                                                                                                                                                                                        |
| Regresión completa                                                                                | «Regresión final»                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |

## No verificado

No se afirma ninguna de estas:

- **Instalación y servicio en la computadora real** (Fase G con el propietario): instalar el `.pkg` (ventana de la
  contraseña de administrador), el `postinstall` como root (`chown`, `root:wheel`, `launchctl bootstrap system`; solo
  se probó sin root con destino falso y `launchctl` simulado), que launchd lance el servicio al cargarlo y al encender
  sin iniciar sesión, que lo relance tras «Reiniciar» y al crear el archivo de habilitación (supuestos de `PathState`
  1.a–c del avance, fase A), el aviso «Elementos en segundo plano añadidos», el acceso al Keychain desde la aplicación
  instalada en `/Applications` y el paso de datos `adopt-legacy` sobre la instalación real (solo se probó con una base
  del clúster temporal). No hay `shellcheck` en el equipo: los scripts se comprobaron con `bash -n` y su ejecución con
  destino falso.
- **Red con una segunda computadora** (Fase W): emparejar de verdad, la escucha 8443 recibiendo conexiones de la red,
  el permiso «Red local» de macOS (las claves están en el `.app` compilado; con firma ad hoc puede volver a pedirse en
  cada compilación), el firewall (desactivado en esta Mac), una terminal en la Wi-Fi aceptada y otra por una VPN de
  túnel rechazada, el cambio de IP real por DHCP, cortar la red de verdad y dos terminales físicas a la vez.
- **Búsqueda mDNS**: solo IPv4 y solo por la interfaz de salida predeterminada (en una computadora con varias redes
  puede no encontrar el servidor); `dns-sd` es una herramienta «para uso interactivo» (solo se usan sus argumentos);
  un `serve` de desarrollo matado con SIGKILL deja su `dns-sd` vivo.
- **Keychain real** con la cuenta nueva `adel-server:{server_id}|main` (las pruebas de Rust no escriben en el
  Keychain).
- **Respaldos**: el diálogo nativo de carpeta real (solo compilado y simulado), una USB real conectada y desconectada
  (simulada con carpetas que aparecen y desaparecen) y el permiso de privacidad de macOS de un `LaunchDaemon` para
  escribir en una USB o un disco externo (puede recibir `EPERM`; el paso «Acceso total al disco» de
  [INSTALAR_EN_RED.md](INSTALAR_EN_RED.md) está sin comprobar). `scripts/restore-check.py` puede decir «difieren» si
  un respaldo automático empieza entre `dev.sh backup` y la comprobación (repetir).
- **Agente de impresión en hardware**: papel, logotipo, corte y cajón reales por una impresora USB conectada a otra Mac,
  el pulso del cajón por `lp -o raw` y un trabajo grande con logotipo (los bytes viajan en base64, límite 4 MB).
- **Carga**: cada terminal abierta consulta al agente cada 3 s y cada flujo SSE escribe cada 750 ms; el lock 7820148305
  serializa todas las escrituras. Sin medir con varias terminales (prueba de carga de la Fase W).
- **Android**: no existe; el contrato se probó con un cliente simulado en Go (integración) y desde Node (Playwright).
- **Windows**: nada de la 9G se ejecutó en Windows (solo `GOOS=windows` para Go; Rust para Windows no se compiló: no
  hay destino de Windows en la Mac). Lista en [NOTAS_WINDOWS.md](NOTAS_WINDOWS.md), «Etapa 9G: qué hacer en la Fase
  W».
- Pendientes de etapas anteriores sin cambio: papel y corte, cajón físico, pantalla táctil física, ventana nativa y
  diálogo nativo real de guardado.

## Regresión final

<!-- PENDIENTE-COORDINADOR: regresión completa de la Fase G sobre una copia exacta del commit final (formato, metadatos,
go vet normal e integration, GOOS=windows, Go unitarias con -race, integración de los 13 paquetes de la tupla, tsc,
Vitest, npm run build, cargo test, Playwright Chromium y WebKit completos y .app 0.10.0), con fecha, commit, tiempos y
estado de la pantalla. -->

Pendiente al escribir este documento.

## Instalación real

<!-- PENDIENTE-COORDINADOR: procedimiento de la Fase G con el propietario (avance, Fase A, «Procedimiento para la Fase
G»): respaldo y ensayo, .pkg 0.10.0 con su SHA-256, paso de datos con adopt-legacy (tablas con conteos idénticos),
modalidad «Servidor y terminal», 0023 aplicada por el servicio, verificaciones (readiness, versión, cerrar la ventana,
«Reiniciar», reinicio de la Mac sin iniciar sesión), comprobación de solo lectura antes y después y fusión en main. -->

Pendiente al escribir este documento.
