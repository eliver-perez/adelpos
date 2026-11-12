# AD-EL POS — ETAPA 9E: COMPILACIÓN, PRUEBAS E INSTALADOR EN WINDOWS 11

Este prompt se ejecuta **en una computadora con Windows 11 de 64 bits**, sobre un clon recién hecho del repositorio. Parte de la versión que dejó la **9D** (0.9.3). Confirma la versión y el esquema reales en `config/product.json` y `CLAUDE.md`.

**Objetivos:**

1. Que AD-EL POS **compile y funcione en Windows**, con las mismas funciones que en macOS.
2. Que las pruebas automáticas pasen también en Windows.
3. Generar aquí mismo un **instalador de prueba (`.exe`, NSIS)**, instalarlo en esta computadora y verificarlo.

Es un instalador de **prueba para la presentación**, no la versión comercial: no requiere firma digital ni licencias.

**Ningún cambio puede romper la versión de macOS.** Todo lo específico de cada sistema va detrás de condiciones por plataforma: `//go:build windows` / `darwin` en Go, `#[cfg(target_os = …)]` en Rust y detección de plataforma en la interfaz.

Al terminar, la versión será **0.9.4**. No se esperan migraciones de base de datos.

---

## 0. FORMA DE TRABAJO

### Rama y especificaciones

- Trabaja en una rama nueva **`etapa-9e-windows`** creada desde `main`.
- Las especificaciones en `docs/etapas/` son de solo lectura. Lee este documento, `CLAUDE.md` y `docs/NOTAS_WINDOWS.md` (lo dejó la 9D con cómo se empaqueta hoy la aplicación en macOS y qué depende de macOS).
- Trabajas en Windows. Usa PowerShell o Git Bash según convenga, y no asumas comandos de macOS (`brew`, `security`, `lpr`, rutas `/Users/...`).

### Eficiencia

- La Fase 0 la hace **un solo agente**.
- **Máximo 2 agentes en total trabajando al mismo tiempo. Los subagentes no pueden lanzar otros subagentes.**
- En cada fase ejecuta solo las pruebas relacionadas con lo que cambió. La regresión completa solo en la Fase F.

### Autonomía

Aplica la regla de autonomía registrada en `CLAUDE.md`: si según tu criterio los cambios son correctos y las pruebas pasan, continúa sin pedir autorización.

En esta computadora eso incluye:

- **Instalar las herramientas de desarrollo** que falten. Usa `winget` y, donde se pueda, instalaciones por usuario.
- Compilar, **instalar y desinstalar AD-EL POS** las veces que haga falta.
- Hacer **commit, fusionar en `main` y hacer push** al terminar.

**Detente y pregunta únicamente si:**

- Una acción requiere reiniciar la computadora.
- Hay que cambiar configuración de seguridad de Windows: desactivar Defender, cambiar la política de ejecución del sistema completo o abrir puertos al exterior.
- Algo podría afectar datos o programas del usuario ajenos al proyecto.
- Un cambio obliga a modificar una decisión de negocio, o no se puede hacer sin arriesgar la versión de macOS.

**Firewall:** si Windows pide permiso de red al arrancar el servidor, avísale al propietario qué ventana va a aparecer y qué debe responder. No cambies reglas del firewall por tu cuenta.

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación del entorno y del código. |
| **A** | Herramientas de desarrollo y fin de línea. |
| **B** | Compilar y ejecutar en modo desarrollo; pruebas existentes en Windows. |
| **C** | Adaptaciones a Windows (§3). |
| **D** | Empaquetado: instalador NSIS (§4). |
| **E** | Instalación real y pruebas sobre lo instalado (§5). |
| **F** | Regresión completa, documentación, fusión en `main` y push. |

**Al terminar cada fase:** ejecuta sus pruebas, registra un resumen en `docs/ETAPA_9E_AVANCE.md`, haz un commit local y continúa.

---

## 0-B. DECISIONES DEL PROPIETARIO

**Si una sección parece decir otra cosa, prevalece esta.**

- **Windows 10 y 11 de 64 bits** (x64). No se requiere ARM.
- **El instalador es NSIS (`.exe`)**, por usuario o por equipo, lo que resulte más simple y estable con PostgreSQL incluido. Documenta la elección.
- **El instalador va sin firmar.** Es normal que aparezca el aviso de SmartScreen.
- **Los datos nunca viven en la carpeta de instalación.** Van en `%ProgramData%\AD-EL POS\` o `%LOCALAPPDATA%\AD-EL POS\`, según la elección anterior. **Desinstalar no borra los datos**, y reinstalar o actualizar los conserva.
- **La aplicación debe comportarse igual que en macOS:** mismas pantallas, reglas, reportes, impresión por red y respaldos. Solo cambia lo propio del sistema.
- **Atajos con Ctrl**, como se diseñó en la 9C.

---

## 1. FASE 0 — VERIFICACIÓN

Registra en el avance:

1. La versión de Windows (`winver`), la arquitectura, la memoria y el espacio libre.
2. Qué herramientas hay instaladas y con qué versión: Git, Go, Node y el gestor de paquetes que use el proyecto, Rust (`rustup`, con el destino `x86_64-pc-windows-msvc`), Visual Studio Build Tools con la carga «Desarrollo para el escritorio con C++», WebView2 y la CLI de Tauri.
3. Cómo arranca hoy la aplicación en macOS, según `docs/NOTAS_WINDOWS.md` y el código:
   - Cómo se incluyen y se inician el servidor Go y PostgreSQL (sidecars o recursos de Tauri).
   - Puertos.
   - Carpeta de datos.
   - Creación de la base de datos en el primer arranque.
   - Migraciones.
   - Secretos (Keychain).
   - Respaldos.
   - Bitácoras.
   - Impresión.
   - Cajón de dinero.
4. **Todo lo que dependa de macOS**, con archivo y línea: rutas escritas directamente, `exec` de comandos de macOS, CUPS o `lpr`, Keychain, señales de proceso de Unix (`SIGTERM`), permisos de archivo, nombres de archivo con caracteres no válidos en Windows, mayúsculas y minúsculas en rutas, y scripts de `package.json` o del Makefile que solo funcionen en bash.
5. El estado de fin de línea del repositorio, y si existe `.gitattributes`.

Con eso define el plan de las fases C y D y continúa.

---

## 2. FASES A Y B — ENTORNO, COMPILACIÓN Y PRUEBAS EN DESARROLLO

### 2.1 Herramientas

- Instala lo que falte con `winget`, en las versiones que pide el proyecto (revisa `go.mod`, `package.json`, `rust-toolchain` y `.nvmrc` si existen).
- Si Visual Studio Build Tools exige reiniciar, **detente y avisa**.

### 2.2 Fin de línea

Windows convierte los finales de línea a CRLF, y eso puede romper scripts, plantillas de impresión que deben ser idénticas byte a byte, migraciones y pruebas.

- Agrega o ajusta **`.gitattributes`** para normalizar a LF: código, SQL, plantillas, `*.sh` y datos de prueba.
- Marca como binarios los archivos que lo son: imágenes, fuentes y archivos de referencia de pruebas.
- Verifica que no aparezcan cambios espurios en `git status`.

### 2.3 Modo desarrollo

- Levanta PostgreSQL, el servidor Go y la interfaz en modo desarrollo en Windows.
- Si los scripts del proyecto solo funcionan en bash, agrega equivalentes multiplataforma (Node o Go) o variantes para PowerShell, sin quitar las de macOS.

### 2.4 Pruebas existentes

Ejecuta en Windows todas las suites:

- Go (`go test ./...`, incluidas las que usan PostgreSQL real).
- Las pruebas de la interfaz.
- Playwright.

Clasifica cada fallo como **defecto real de Windows** o **prueba dependiente de macOS**, y corrige ambos. Las pruebas deben pasar en los dos sistemas: por ejemplo, rutas con `filepath.Join` y comparaciones de texto sin depender de CRLF.

**Atención especial a:**

- **Las plantillas de documentos** de la etapa 9B, que deben ser idénticas byte a byte a las de macOS.
- **La generación de PDF y Excel** en Go: fuentes incluidas y no tomadas del sistema, acentos y «ñ».
- **Fechas y zona horaria:** la base de datos de zonas horarias de Go en Windows (incluye `time/tzdata` si hace falta) y el corte del día a las 05:00.

---

## 3. FASE C — ADAPTACIONES A WINDOWS

Cada adaptación va detrás de una condición por plataforma, con su prueba.

### 3.1 Procesos

- **PostgreSQL:** binarios oficiales de Windows (x64), en la misma versión mayor que en macOS. La aplicación inicializa el clúster en la carpeta de datos (`initdb`), lo arranca (`pg_ctl`) y lo detiene igual que en macOS.
- **Servidor Go:** compilado como `.exe`. **Sin ventanas de consola visibles** al arrancar procesos hijos (usa `CREATE_NO_WINDOW` o el equivalente).
- **Al cerrar la aplicación:** sin procesos huérfanos ni bloqueos de archivos. Windows no tiene `SIGTERM`, así que usa el apagado ordenado del servidor Go y `pg_ctl stop`.
- **Recuperación:** si la aplicación se cerró mal, el siguiente arranque detecta y limpia el estado (por ejemplo, `postmaster.pid`).

### 3.2 Puertos

Solo `127.0.0.1`, salvo que la configuración multiterminal actual ya exponga algo en la red. En ese caso documenta qué puerto y qué permiso del firewall se pedirá.

### 3.3 Secretos

Usa el **Administrador de credenciales de Windows** en lugar de Keychain, con la misma interfaz interna. Así el resto del código no cambia.

### 3.4 Rutas

Datos, respaldos, logotipos, exportaciones de PDF y Excel, bitácoras y temporales con las carpetas conocidas de Windows. Revisa también rutas con espacios y acentos (por ejemplo, un usuario «José»).

### 3.5 Impresión

- **Impresoras de red (ESC/POS por TCP):** deben funcionar igual que en macOS.
- **Impresoras USB e instaladas en Windows:** envía ESC/POS en modo **RAW a la cola de impresión de Windows** (API de impresión de Windows) en lugar de CUPS. En la configuración de impresoras, lista las impresoras instaladas en Windows para elegirlas.
- **Cajón de dinero:** por la impresora (red o USB). Si alguna variante no queda completa, documéntala como limitación sin bloquear el instalador.

### 3.6 Menú nativo y atajos

Verifica el menú nativo en Windows. Si no aporta, ocúltalo sin errores. Los atajos usan Ctrl, y la paleta de comandos debe funcionar.

### 3.7 Pantalla completa y ventana

El modo pantalla completa y el botón de la barra funcionan. La ventana recuerda su tamaño y posición.

### 3.8 Respaldos

Verifica que crear y restaurar un respaldo funcione en Windows, usando el `pg_dump` y el `pg_restore` incluidos.

---

## 4. FASE D — INSTALADOR

Genera con Tauri el **instalador NSIS** (`.exe`), que debe incluir:

- La aplicación, el servidor Go y los binarios de PostgreSQL necesarios, solo los que se usan, para no inflar el tamaño.
- El **instalador de arranque de WebView2**, para equipos que no lo tengan.
- Nombre «AD-EL POS», ícono, versión 0.9.4, accesos directos en el menú Inicio y en el escritorio (opcional), y desinstalador en «Aplicaciones instaladas».
- **Desinstalación:** quita la aplicación y deja los datos.

Registra el tamaño del instalador y la ruta donde quedó.

Agrega al repositorio un **script de compilación para Windows** (por ejemplo `scripts/build-windows.ps1`) que haga todo de principio a fin, para repetirlo en el futuro con un solo comando.

---

## 5. FASE E — INSTALACIÓN REAL Y PRUEBAS

Prueba sobre la aplicación **instalada**, no en modo desarrollo:

1. **Instalación limpia.** Abre la aplicación y verifica que crea la base de datos, que el asistente inicial funciona y que no aparecen ventanas de consola.
2. **Prueba de humo:**
   - Crea el establecimiento, un área con mesas y unos productos.
   - Abre turno.
   - Captura en una mesa, envía a cocina y cobra en efectivo (incluido cobro en dólares si está activo).
   - Crea un pedido para llevar.
   - Cancela una cuenta con motivo.
   - Haz corte parcial y cierre de turno.
   - Genera el reporte de ventas en PDF y en Excel, y ábrelos para comprobar acentos y logotipo.
   - Crea un respaldo y restáuralo.
3. **Persistencia:** cierra y vuelve a abrir la aplicación; reinicia solo los procesos de la aplicación, sin reiniciar Windows. Los datos siguen ahí.
4. **Actualización:** genera un segundo instalador con un cambio de versión de prueba e instálalo encima. Debe conservar los datos y aplicar las migraciones si las hubiera. Luego regresa la versión a 0.9.4.
5. **Desinstalación y reinstalación:** los datos se conservan.
6. **Usuario con acentos o espacios en la ruta,** si se puede simular sin crear cuentas de Windows; si no, déjalo como limitación.
7. **Rendimiento:** registra el tiempo de arranque y el consumo de memoria con la aplicación abierta.

Automatiza en un script lo que se pueda: instalación en silencio y verificación de que el servidor responde. Lo visual compruébalo con Playwright contra la aplicación en ejecución, o con capturas de pantalla si no es posible.

Deja la aplicación **instalada y lista** en esta computadora, con el establecimiento de prueba, para que el propietario la revise.

---

## 6. FASE F — REGRESIÓN, DOCUMENTACIÓN Y ENTREGA

- **Regresión completa en Windows:** todas las suites, Playwright y la compilación.
- **Para no romper macOS:**
  - Revisa que cada cambio específico de Windows esté detrás de una condición por plataforma.
  - Compila el código Go para `darwin` desde Windows (`GOOS=darwin go build ./...`) y verifica que la parte Rust de Tauri no tenga errores de compilación condicional evidentes.
  - Anota en el avance que la próxima sesión en la Mac debe ejecutar la regresión completa allí.
- **Actualiza `CLAUDE.md`:** versión 0.9.4, cómo compilar en Windows, el script de compilación, las carpetas de datos en Windows y las diferencias por plataforma.
- **Crea `docs/INSTALAR_EN_WINDOWS.md`** para el propietario, corto y sin tecnicismos:
  1. Dónde está el instalador y cómo copiarlo a otra computadora.
  2. Cómo instalarlo. Ante el aviso de SmartScreen («Windows protegió tu PC»), toca «Más información» y luego «Ejecutar de todas formas».
  3. Qué responder si Windows pide permiso de red.
  4. Dónde quedan los datos y cómo hacer un respaldo.
  5. Cómo desinstalar sin perder datos.
  6. Una lista de lo que el propietario debe probar a mano: pantalla táctil, impresora de red, impresora USB, cajón, pantalla completa y dos terminales si aplica.
- **Commit,** fusión en `main` y push.
- **Entrega final breve:**
  - Herramientas instaladas.
  - Cambios por fase.
  - Pruebas ejecutadas y resultados.
  - Ruta y tamaño del instalador.
  - Tiempo de arranque y memoria.
  - Limitaciones: lo que no se pudo probar sin hardware (impresora USB, cajón, pantalla táctil) y lo que falta para la versión comercial (firma digital, actualizaciones automáticas, licencias).
