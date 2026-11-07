# Verificación · Etapa 3

Fecha: 7 de noviembre de 2026, America/Monterrey. macOS Apple Silicon. Versión 0.3.0, esquema 3. Evidencia previa: [etapa 1](VERIFICATION_STAGE1.md) y [etapa 2](VERIFICATION_STAGE2.md).

## Resultado ejecutado

- `./scripts/dev.sh test`: correcto. Incluye Prettier, gofmt, Rustfmt, metadatos coherentes, Go con detector de carreras y vet, pruebas frontend, TypeScript estricto, Vite y Rust con lockfile.
- Frontend: **19 pruebas aprobadas** en cuatro archivos. Conserva las pruebas anteriores y añade administración del catálogo, permisos de consulta, selección estructurada, retiro/restauración de todos los complementos, conservación de otras selecciones, reinicio de un hijo de combo sin perder observaciones del padre, descarte de respuestas atrasadas y recuperación tras autorización pendiente del Llavero.
- Rust: **7 aprobadas, 1 opt-in omitida**. Incluye rutas de catálogo cerradas, métodos correctos, bloqueo de identificadores/rutas arbitrarias, eliminación de tokens antes del WebView, conservación ante contraseña actual incorrecta y ausencia de credenciales vacías durante espera del almacén seguro. No se volvió a ejecutar el roundtrip opt-in real del Keychain en esta etapa; la ejecución de etapa 2 permanece documentada.
- `./scripts/dev.sh test-integration`: correcto. Tres suites sobre bases temporales distintas `adel_pos_test_*`, eliminadas al terminar: migrador, administración y catálogo. No se utilizó adel_pos para sembrar escenarios de prueba.
- `./scripts/dev.sh build-macos`: correcto. Bundle ARM64 `desktop/src-tauri/target/release/bundle/macos/AD-EL POS.app`, versión 0.3.0, aproximadamente 9.31 MiB.

## Motor y aceptación de catálogo

Las pruebas unitarias usan el motor Go real y reloj fijo; las de aceptación usan HTTP, autenticación y PostgreSQL reales. Las pruebas de interfaz sustituyen únicamente el transporte nativo; no se presentan como pruebas de persistencia.

Verificados:

- Crear, editar, desactivar y reordenar categorías; revisión obsoleta rechazada sin sobrescribir; búsqueda, filtros y paginación.
- Crear/editar productos, presentaciones, configuración reutilizable, duplicado con identidades nuevas, archivo y consulta de archivados.
- Ingredientes removibles y adicionales con precio explícito; cero cargos implícitos; límites y rechazo de cantidades inválidas.
- Retirar todos los complementos manteniendo $150; reemplazarlos completamente; arroz/ensalada → papas/ensalada = $155; doble papas = $165. Valores incluido/solicitado, diferencia y extras separados.
- Redondeo comercial exacto: medio centavo a un centavo; rechazo de precisión monetaria excesiva; porcentajes por línea y totales exactos.
- Grupos por cantidad con tramos; rechazo de fracciones o adicionales sin reglas explícitas.
- Combo que reutiliza hamburguesa/papas: **$90 / $100 / $140** para 0/1/2 papas; sustitución permitida y rechazo de ciclo. Personalización del hijo cobrada una vez, sin sumar otra vez su base. Hijo inactivo impide el cálculo.
- Listas alternativas y origen del precio, precio exacto por presentación, respaldo base y rechazo por precio faltante; cambio de lista predeterminada incrementa y audita la desplazada.
- 2×1 con dos y tres unidades; compra/paga; porcentaje y precio especial; expiración, horarios, prioridad, incompatibilidad y combinación explícita sobre remanente; inclusión/exclusión de plataformas.
- Zona IANA del establecimiento, fin exclusivo de horarios y cierre 24:00; categoría/producto inactivos, pausa temporal, archivo y excepción autorizada con motivo/auditoría.
- PNG válido guardado y consultado; archivo inválido y PNG truncado rechazados. El código limita a 1 MiB y 2048 × 2048; no se probó cada posible archivo malformado.
- Permisos efectivos vigentes, rechazos HTTP, auditoría transaccional y ocultación de dinero sin permiso incluso en los snapshots de auditoría. Pruebas unitarias adicionales impiden cambiar condiciones económicas, horario o estado de presentaciones sin sus permisos.
- Toda la aceptación de etapa 2 volvió a pasar: setup idempotente, admin, sa exclusivamente temporal de prueba, roles/excepciones, sesiones, terminales, preferencias, cajas y auditoría. No se cambió el mecanismo reservado sa real.

## Respaldo y actualización de la instalación

Se creó `backups/adel_pos-20261107T220425Z-654dee.dump`, se verificó su catálogo y se **restauró completamente** en una base temporal exclusiva. Coincidieron los conteos de usuarios (2), terminales (1), establecimiento (1) y esquema (2). La base de ensayo se eliminó. No se restauró encima de adel_pos.

Después se detuvo Go, se aplicó 0003: `applied: 1`. Una repetición real informó `applied: 0`. 0001 y 0002 no se modificaron. Go reinició y readiness respondió 200: PostgreSQL disponible y esquema compatible.

Una lectura posterior confirmó **ADEL POS**, **America/Monterrey**, **MXN**, dos usuarios, una terminal, cero categorías y cero productos ficticios. Existe una lista activa predeterminada y se añadieron los quince permisos de catálogo al administrador. No se borraron datos, credenciales ni configuración.

Evidencia privada, sin secretos impresos: `.local/stage3-restore-verification.json` y `.local/stage3-installation-verification.json`. Estos archivos y el respaldo no se versionan.

## Ventana y procesos reales

El recorrido inicial `tests/runtime_smoke.py` verificó caída/recuperación de PostgreSQL conservando Go y modo server-only temporal. Se detuvo al esperar consultas del escritorio: la muestra de pila del proceso identificó una espera del Llavero de macOS (`SecKeychainFindGenericPassword`) para acceder a la credencial existente. Ese primer intento quedó incompleto; después de la autorización del propietario se repitió correctamente, como se detalla a continuación.

Se corrigió el arranque: la recuperación del almacén seguro ocurre en segundo plano, mantiene viva la ventana y muestra **Revisa el acceso seguro** con acceso a diagnóstico. Nunca sustituye una espera/denegación por credenciales vacías ni guarda tokens en archivos. La recuperación de interfaz y el estado nativo pendiente tienen pruebas automatizadas aprobadas.

La compilación final se ejecutó además con un namespace temporal de diagnóstico, sin enrolar usuarios/dispositivos ni acceder al perfil real. Con esa instancia se verificó:

- Consultas reales React → IPC → Go desde el `.app` 0.3.0.
- SIGTERM del escritorio conserva Go y PostgreSQL; reabrir retoma consultas contra el mismo Go.
- Detener Go conserva el escritorio; reiniciar Go permite a esa misma instancia recuperar consultas.
- Modo terminal-only consulta al servidor existente, sin iniciar una segunda base o servidor.

Resultado del perfil aislado en `.local/stage3-native-verification.json`. El perfil temporal y su proceso se retiraron.

El propietario confirmó después la autorización del Llavero. Se comprobó en PostgreSQL que la aplicación normal retomó consultas autenticadas con la sesión existente, sin leer ni imprimir secretos. Se ejecutaron completos y correctamente ambos recorridos con ese perfil real:

- `python3 tests/runtime_smoke.py`: caída/recuperación de PostgreSQL sin reiniciar Go, proceso server-only temporal, cierre normal del escritorio conservando Go, reapertura, detención de Go manteniendo Tauri y recuperación de consultas en la misma ventana.
- `python3 tests/runtime_smoke.py --terminal-only`: el mismo `.app` consultó el servidor existente, cuyo PID se conservó; después se retiró el proceso de prueba y se reabrió la aplicación con su configuración normal.

La comprobación final confirmó una sesión admin vigente y actividad autenticada de los últimos quince segundos, esquema 3 y readiness 200 con PostgreSQL disponible. Go quedó activo, PID 95050. La autorización del Llavero y estos recorridos ya no están pendientes.

Evidencia privada: `.local/runtime-verification.json`, `.local/terminal-mode-verification.json` y `.local/stage3-real-profile-verification.json`. No contienen tokens ni contraseñas y no se versionan.

## Límites reales

- No se automatizó cada control de la ventana nativa ni se capturó su presentación a 1280 × 720. Sí se probaron componentes, TypeScript, bundle y transporte nativo real tanto con perfil aislado como con el perfil normal autenticado.
- No se midieron carga de un catálogo grande, muchas terminales, segunda máquina LAN, caída de energía, Windows ni Mac Intel. Los límites explícitos de selectores, cálculo e imágenes están en [CATALOG.md](CATALOG.md).
- Firma/notarización comercial, licencias, servicio instalado, teclado flotante y eventos WebSocket siguen pendientes de sus etapas. No existen pedidos, cobros ni movimientos físicos de inventario.

## Repetir comprobaciones

```bash
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh build-macos
./scripts/dev.sh db-start
./scripts/dev.sh server-start
./scripts/dev.sh open-app
```

Sin trabajo real en curso, `python3 tests/runtime_smoke.py` y `python3 tests/runtime_smoke.py --terminal-only` permiten completar nuevamente los recorridos habituales. Son pruebas operativas opt-in que interrumpen únicamente los servicios propios de desarrollo.
