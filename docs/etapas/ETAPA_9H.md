# AD-EL POS — ETAPA 9H: MIGRACIÓN DEL SISTEMA ANTERIOR Y AJUSTES PENDIENTES

Se ejecuta **en la Mac**. Parte de la versión que dejó la 9G en `main` (0.10.0, esquema 23). Confirma la versión y el esquema reales en `config/product.json` y `CLAUDE.md`.

**Contenido:**

1. **Migración del sistema anterior** (C#/WinForms con SQL Server 2012) a AD-EL POS, **con todo su historial**, mediante un asistente sencillo que se pueda repetir.
2. **Pendientes:**
   - Verificar los ajustes de la 9F.
   - Precio unitario por renglón.
   - Sesiones en varios dispositivos.

Al terminar, la versión será **0.11.0**.

---

## 0. FORMA DE TRABAJO

### Rama y especificaciones

- Rama nueva **`etapa-9h`** desde `main`. Las especificaciones en `docs/etapas/` son de solo lectura.
- Lee este documento, `CLAUDE.md` (incluido §21, el sistema anterior como referencia), `NEXT_STEPS_FOR_SOL.md` y lo que necesites de `docs/`.

### Eficiencia

- La Fase 0 la hace **un solo agente**.
- **Máximo 2 agentes en total trabajando al mismo tiempo. Los subagentes no pueden lanzar otros subagentes.**
- En cada fase, solo las pruebas de lo que cambió; la regresión completa, una sola vez, en la Fase F.
- **Revisor independiente solo para la migración** (Fases B y C), porque toca datos reales y dinero.

### Autonomía

Aplica la regla de autonomía de `CLAUDE.md` §0.

**La instalación en la Mac ahora es el servicio:** se actualiza con el `.pkg` y la contraseña de administrador del propietario, así que esa parte es una parada. Prepara todo y avísale.

### Datos reales del restaurante: reglas obligatorias

- El respaldo del sistema anterior está **fuera del repositorio**, en `ADEL POS/possys.bak` (al mismo nivel que `ADEL POS/legacy-csharp/`). Es un `.bak` de **SQL Server 2012**.
- **Nunca** se copia al repositorio, ni el respaldo ni nada extraído de él: tablas, CSV, el paquete de migración o capturas con datos. **Nunca** se sube a GitHub ni se pega en documentos del repositorio.
- Agrega a `.gitignore` patrones que lo impidan (`*.bak`, `*.adelmig`, `legacy-data/`) y una prueba o comprobación que falle si alguno aparece en el índice de git.
- Las pruebas automáticas usan **datos sintéticos** que imitan la estructura del sistema anterior, creados por ti. Nunca usan el respaldo real.
- En `docs/` solo pueden aparecer **conteos y nombres de tablas o columnas**, nunca datos de clientes, ventas, usuarios ni contraseñas.
- **Las contraseñas del sistema anterior no se migran ni se leen** más allá de lo necesario para saber que existen.
- **La importación en la instalación real (el servicio de la Mac) la decide el propietario**, desde el asistente. Tú la pruebas en bases temporales.

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación: estado de lo pendiente de la 9F y análisis del sistema anterior (§1). |
| **A** | Pendientes y ajustes (§2). |
| **B** | Lectura del respaldo y paquete de migración (§3). |
| **C** | Correspondencia e importación (§4). |
| **D** | Asistente de migración en la interfaz (§5). |
| **E** | Prueba completa con el respaldo real en una base temporal (§6). |
| **F** | Regresión completa, documentación, `.pkg`, fusión y push. |

**Al terminar cada fase:** ejecuta sus pruebas, registra un resumen en `docs/ETAPA_9H_AVANCE.md`, haz un commit y continúa.

---

## 0-B. DECISIONES DEL PROPIETARIO

**Si una sección parece decir otra cosa, prevalece esta.**

- **Se migra todo el historial**, no solo el catálogo. El sistema anterior se diseñó lo más parecido posible a este, así que la correspondencia debe aprovechar al máximo lo que existe.
- **La migración se repite:** el restaurante sigue operando con el sistema anterior. Ahora se prueba, y al pasar a producción se vuelve a cargar con un respaldo más reciente.
- **Interfaz sencilla:** es una herramienta para un solo restaurante, la usa solo el propietario y no se vende como función.
- **«Cancelar unidades»** no requiere autorización de supervisor: basta registrar quién lo hizo.
- **Precio unitario por renglón:** bloque **opcional** del diseñador de documentos.
- **Un usuario puede tener sesión activa a la vez en distintos dispositivos** (por ejemplo, una caja y un teléfono).

---

## 1. FASE 0 — VERIFICACIÓN Y ANÁLISIS

### 1.1 Estado de la 9F

Lee `docs/ETAPA_9F_AVANCE.md` y el código, y registra si quedaron hechos. **Haz en la Fase A solo lo que falte:**

1. **Barra superior compacta** (ventana sin maximizar, unos 1500 px):
   - El isotipo de la marca en lugar de la «A» genérica.
   - El contorno de foco del logotipo solo al navegar con teclado.
   - Caja con el estado del turno.
   - «En línea» con texto o ayuda emergente.
2. **Leyenda del mapa de mesas** sin desplazamiento horizontal en ventanas angostas.
3. **Ruta antigua de anulación de cuentas** retirada o redirigida a «Cancelar cuenta».
4. **Memoria de trabajo de Claude Code** igual a la regla de autonomía de `CLAUDE.md` §0, sin las reglas viejas («nunca push», «autorizar cada migración»).

Además, si la 9F agregó autorización de supervisor a «Cancelar unidades», revisa §2.2.

### 1.2 Análisis del sistema anterior

Con el **código** de `legacy-csharp` (en especial `database/Base de Datos.sql` y los formularios) y con el **respaldo**, una vez restaurado en el SQL Server temporal (§3.1), registra en `docs/MIGRACION_ANALISIS.md`, **solo con estructura y conteos**:

1. **Tablas, columnas, claves y relaciones**, y el conteo de filas de cada tabla.
2. **Qué representa cada tabla** y a qué concepto de AD-EL POS corresponde.
3. **Cómo se manejaba el IVA.** El propietario recuerda que era por producto, pero no está seguro. Revisa si los precios incluían IVA, qué tasas aparecen y si cada venta guardaba el IVA calculado.
4. **Cómo se registraban:**
   - Ventas, renglones, complementos o modificadores y descuentos.
   - Cancelaciones.
   - Formas de pago, incluidos dólares y tipo de cambio.
   - Cortes y turnos, egresos y retiros.
   - Mesas y para llevar.
   - Usuarios y clientes.
   - Inventario, si existía.
5. **Rango de fechas** de las ventas, y si hay datos dañados o incoherentes: ventas sin renglones, totales que no cuadran, productos borrados con ventas, fechas imposibles.
6. **Una propuesta de correspondencia**, tabla por tabla y campo por campo, con lo que no tiene equivalente y qué se hará con ello.

**Si la correspondencia obliga a una decisión de negocio que este documento no resuelve, detente y pregunta.** Por ejemplo: qué hacer con ventas cuyo total no cuadra con sus renglones, o un concepto del sistema anterior sin equivalente que afecte dinero.

---

## 2. FASE A — PENDIENTES

### 2.1 Lo que falte de la 9F

Haz lo que la Fase 0 encontró pendiente (§1.1), con sus pruebas.

### 2.2 «Cancelar unidades»

Decisión del propietario: **no requiere autorización de supervisor.** Basta con el permiso propio y que quede registrado quién lo hizo. Si la 9F agregó la autorización, quítala y conserva el registro de quién canceló en el Registro de movimientos y en el reporte «Modificaciones de cantidades». La columna «Autorizó» queda vacía o se quita en ese reporte.

### 2.3 Precio unitario por renglón

- Agrega al diseñador de documentos un **bloque o una opción del bloque de renglones**: **«Mostrar precio unitario»**, **desactivada por defecto**.
- **Activada,** cada renglón muestra el precio comercial de la unidad junto a la cantidad («3 × $150.00 … $450.00»). El importe del renglón sigue siendo el exacto calculado por el servidor.
- **Con los anchos de 42/48 y 30/32 columnas:** si no cabe en el mismo renglón, el precio unitario va en un segundo renglón debajo del nombre.
- **Las plantillas predeterminadas no cambian** y siguen idénticas byte a byte.

### 2.4 Sesiones en varios dispositivos

- **Un usuario puede tener una sesión activa por dispositivo,** en varios dispositivos a la vez. Iniciar sesión en un dispositivo solo cierra la sesión anterior **de ese mismo dispositivo**.
- El conteo de sesiones activas por tipo de dispositivo de la 9G **cuenta sesiones, no usuarios.**
- Revisa que turnos de caja, arqueo y permisos sigan funcionando por terminal. Documenta el cambio en `CLAUDE.md` y en `docs/API_V1.md` si afecta al contrato.

---

## 3. FASE B — LECTURA DEL RESPALDO Y PAQUETE DE MIGRACIÓN

### 3.1 SQL Server temporal

- El `.bak` se restaura en un **SQL Server temporal en Docker**, solo para leerlo. Usa la imagen oficial de Microsoft (2019 o 2022), que acepta respaldos de 2012 y corre en Apple Silicon con la emulación de Docker Desktop.
- Si Docker Desktop no está instalado, **detente y avisa al propietario** con instrucciones claras para instalarlo.
- El contenedor:
  - Escucha **solo en `127.0.0.1`**.
  - Usa una contraseña generada al momento que no se guarda.
  - Recibe el respaldo por un volumen de **solo lectura**.
  - **Se elimina al terminar**, junto con su volumen de datos.
- **Nunca se conecta a un SQL Server real** del restaurante.

### 3.2 Paquete de migración

La lectura produce un **paquete de migración**: un solo archivo `.adelmig`.

- **Contenido:** los datos extraídos en un formato intermedio propio y documentado (por ejemplo, JSON por tabla), con:
  - La versión del formato.
  - La fecha del respaldo.
  - Los conteos de cada tabla.
  - Una huella de contenido.
- **Seguridad:** el paquete se cifra con una frase de paso que el propietario elige, porque contiene datos reales y puede viajar en una USB.
- **Uso:** un solo paquete sirve para importar en **cualquier** computadora con AD-EL POS, Mac o Windows, **sin Docker**.
- **Por qué dos pasos:** al pasar a producción, el propietario puede preparar el paquete en su Mac e importarlo en la computadora del restaurante.
- **El lector del sistema anterior está separado del servidor,** como un comando propio o un subcomando del servidor, para que el servidor normal nunca dependa de Docker ni de SQL Server.

---

## 4. FASE C — CORRESPONDENCIA E IMPORTACIÓN

### 4.1 Qué se importa

**Todo lo que tenga equivalente,** según el análisis aprobado de la Fase 0:

- **Catálogo:** categorías, productos, presentaciones, precios, complementos y combos. Cada producto conserva su **tasa de IVA** según lo que se encuentre.
- **Áreas y mesas.**
- **Usuarios:** sin contraseñas. Quedan **sin acceso** hasta que el administrador les asigne una contraseña o un PIN. Sus roles y permisos se aproximan a los roles de AD-EL POS, con una tabla de correspondencia en el análisis.
- **Clientes,** si existían.
- **Historial completo:**
  - Ventas con sus renglones, complementos, descuentos y cancelaciones.
  - Pagos, incluidos los dólares con su tipo de cambio.
  - Turnos, cortes, egresos y retiros.
- **Inventario,** si existía: existencias a la fecha del respaldo y, si es razonable, sus movimientos.

### 4.2 Cómo se guarda el historial

- **El historial importado es de solo lectura y queda marcado como «Sistema anterior»:** origen, folio original y fecha original. Nunca se reabre, se cobra, se cancela ni se modifica.
- **Los importes se conservan exactamente** como los registró el sistema anterior: precios, IVA, descuentos, totales y pagos. **Nunca se recalculan** con las reglas actuales. Si el sistema anterior no guardaba el IVA de cada venta, se calcula con la tasa del producto vigente en esa venta, si se puede saber, y se marca como «IVA estimado».
- **Las ventas importadas aparecen en los reportes** de ventas, productos, formas de pago, por día y por turno, distinguibles con un filtro «Origen: Sistema anterior / AD-EL POS». Así el propietario puede comparar contra años anteriores.
- **El historial no altera la operación actual:** ni la caja ni los turnos abiertos ni el inventario vigente. Los folios importados no consumen ni chocan con la numeración nueva.
- **Datos incoherentes** (ventas cuyo total no cuadra, renglones de productos borrados): se importan tal como estaban, con una marca y una nota, y se listan en el informe. No se corrigen solos. Si la Fase 0 encontró casos que requieren decisión, ya se habrán preguntado.

### 4.3 Repetir la migración

- **Cada importación es un lote** con su identificador, la fecha del respaldo y quién la hizo.
- **Al importar de nuevo, el lote anterior se reemplaza completo** (catálogo, mesas, usuarios e historial importados), en una sola transacción o con un mecanismo equivalente que nunca deje datos a medias.
- **La importación solo se permite en una instalación sin operación propia:** sin ventas, turnos ni movimientos hechos en AD-EL POS, salvo los de un lote importado anterior. Si hay operación propia, el asistente lo explica y no importa. En la Mac de pruebas, el propietario podrá empezar con una instalación nueva o limpia.
- **Antes de importar se hace siempre un respaldo verificado** de la base actual.

### 4.4 Validación posterior

Al terminar, la importación compara contra los conteos y totales del paquete:

- Número de productos, mesas, usuarios, ventas y pagos.
- **Total vendido por año y por forma de pago.**
- Total de egresos.

Si algo no cuadra, el asistente lo muestra claramente. **Las diferencias de dinero nunca se ocultan.**

---

## 5. FASE D — ASISTENTE DE MIGRACIÓN

**Dónde:** en el menú **Archivo › «Migrar del sistema anterior…»** y en **Administración › Mantenimiento**. Solo para usuarios con permiso de administrador del sistema, y solo en la computadora principal.

**Pasos del asistente,** sencillos y sin tecnicismos:

1. **Preparar paquete** (solo en una computadora con Docker):
   - Elegir el `.bak`.
   - Elegir dónde guardar el paquete y su frase de paso.
   - Ver el avance.
   - Al terminar, el resumen: rango de fechas y conteos.
2. **Importar paquete:**
   - Elegir el `.adelmig` y escribir la frase de paso.
   - Ver el **resumen previo**: «312 productos, 18 mesas, 6 usuarios, 48 230 ventas del 03/2018 al 11/2026…», con advertencias y datos incoherentes.
   - Confirmar.
   - Ver el avance.
   - Ver el **informe final** con la validación de §4.4, descargable en PDF.
3. **Si ya existe un lote importado,** el asistente lo dice y ofrece reemplazarlo, con confirmación.

**Comportamiento esperado:**

- Una importación grande no bloquea la aplicación ni el servidor. Corre como trabajo en segundo plano y se puede seguir su avance.
- Si se interrumpe, no deja datos a medias.
- Todo queda en el Registro de movimientos: preparación, importación, reemplazo y resultado.

---

## 6. FASE E — PRUEBA COMPLETA CON EL RESPALDO REAL

- Con el `.bak` real:
  1. Prepara el paquete.
  2. Impórtalo en una **base temporal aislada**, nunca en la instalación real.
  3. Revisa la validación de §4.4.
- Recorre en la interfaz, sobre esa base temporal, algunos reportes del historial y compáralos con lo que se puede consultar del sistema anterior: el total de un día, de un mes y de un año. Registra los resultados en el avance, **solo con cifras agregadas**.
- **Mide los tiempos** de preparación e importación y el tamaño del paquete.
- **La importación en la instalación real la hará el propietario** desde el asistente cuando quiera. Déjale en el avance una lista de pasos.

---

## 7. PRUEBAS

- **Datos sintéticos** que imiten la estructura del sistema anterior: catálogo con IVA por producto, ventas con complementos, descuentos y cancelaciones, dólares, cortes y egresos, y casos incoherentes.
- **Correspondencia:** cada tipo de dato, importes exactos sin recalcular y el «IVA estimado» cuando aplique.
- **Repetición:**
  - Reemplazar el lote completo.
  - Rechazo en una instalación con operación propia.
  - Interrupción a la mitad sin datos a medias.
- **Paquete:**
  - Frase de paso incorrecta.
  - Paquete dañado.
  - Versión del formato desconocida.
- **El historial importado no se puede reabrir, cobrar ni cancelar;** los reportes lo filtran por origen; la caja y el inventario actuales no cambian.
- **Asistente** (Playwright, con un paquete sintético).
- **Datos reales:** la comprobación de que no queda nada de ellos en el índice de git.
- **Fase A:**
  - Precio unitario con y sin espacio.
  - Plantillas predeterminadas idénticas.
  - «Cancelar unidades» sin autorización.
  - Sesiones en dos dispositivos.
  - Lo que falte de la 9F.
- **Regresión completa** en la Fase F (Go con `-race`, integración, Vitest, Rust, Playwright en Chromium y WebKit) y `GOOS=windows`.

---

## 8. DOCUMENTACIÓN Y ENTREGA

- Actualiza `CLAUDE.md`:
  - Versión 0.11.0 y esquema nuevo.
  - Migración del sistema anterior.
  - Reglas de datos reales.
  - Sesiones por dispositivo.
  - Precio unitario.
- Crea **`docs/MIGRAR_SISTEMA_ANTERIOR.md`** para el propietario, corto y sin tecnicismos:
  1. Instalar Docker Desktop.
  2. Preparar el paquete.
  3. Importarlo.
  4. Repetirlo al pasar a producción.
  5. Qué hacer si la validación no cuadra.
- `docs/MIGRACION_ANALISIS.md` solo con estructura, correspondencia y conteos.
- Compila el `.pkg`. **Parada:** pide al propietario instalarlo con «Servidor del restaurante» marcada.
- Fusiona en `main` y haz push.
- **Entrega final breve:**
  - Resultado de la Fase 0 sobre la 9F.
  - Cómo se manejaba el IVA en el sistema anterior.
  - Qué se importa y qué no.
  - Resultado de la prueba con el respaldo real: conteos, totales por año, tiempos y diferencias.
  - Pruebas.
  - Migración del esquema.
  - Pasos para que el propietario importe en su instalación.
