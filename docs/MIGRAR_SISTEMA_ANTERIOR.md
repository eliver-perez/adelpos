# Migrar el sistema anterior

Guía corta para pasar a AD-EL POS **todo lo del sistema anterior**: productos, categorías, complementos, mesas,
usuarios y el historial completo de ventas, pagos, cortes y retiros. Desde la versión 0.11.0 (etapa 9H).

Se hace en dos pasos, con el asistente **«Migrar del sistema anterior»**:

1. **Preparar el paquete**: AD-EL POS lee el respaldo del sistema anterior (el archivo `.bak`) y guarda sus datos en
   un solo archivo, el **paquete** (`.adelmig`), protegido con una frase de paso que eliges tú.
2. **Importar el paquete** en la computadora principal del restaurante.

La migración **se puede repetir**: mientras el restaurante sigue vendiendo con el sistema anterior, puedes probarla
las veces que quieras; al pasar a producción, se vuelve a hacer con un respaldo reciente y reemplaza completo lo
importado antes.

**Lo importado del sistema anterior es de solo lectura**: sus ventas se ven en los reportes, marcadas como «Sistema
anterior», pero nunca se reabren, se cobran ni se cancelan, y no cambian la caja, los turnos ni el inventario de
AD-EL POS.

> **Cuida el respaldo y el paquete.** Los dos tienen los datos reales del restaurante. El paquete va cifrado con la
> frase de paso, pero guárdalo como guardarías el respaldo: no lo compartas ni lo subas a internet.

## Dónde está el asistente

Solo en la **computadora principal** (la que guarda los datos del restaurante) y con un usuario administrador:

- **Administración › Sistema › Mantenimiento**, en la sección **«Migrar del sistema anterior»**: toca **«Abrir el
  asistente de migración»**.
- En la Mac, también en el menú **Archivo › «Migrar del sistema anterior…»**.
- O escribe «Migrar» en la paleta de comandos (⌘K en la Mac, Ctrl+K en Windows).

Desde una terminal, el asistente solo dice que la migración se hace en la computadora principal.

## 1. Instalar Docker Desktop (solo para preparar el paquete)

Para **preparar** el paquete hace falta **Docker Desktop**, un programa gratuito que AD-EL POS usa para leer el
respaldo del sistema anterior. **Para importar no hace falta**: el paquete se puede importar en cualquier computadora
principal con AD-EL POS, aunque no tenga Docker.

1. Descárgalo de `https://www.docker.com/products/docker-desktop/`:
   - **Mac**: elige la versión para tu tipo de Mac (con chip de Apple o con Intel), abre el archivo descargado y
     arrastra Docker a Aplicaciones.
   - **Windows**: descarga el instalador para Windows y ábrelo; puede pedir la contraseña de administrador y reiniciar
     la computadora.
2. Abre **Docker Desktop** y espera a que diga que está listo. La primera vez acepta sus condiciones de uso.
3. Déjalo abierto mientras preparas el paquete. Después puedes cerrarlo.

Para leer el respaldo, AD-EL POS usa dentro de Docker **SQL Server Express**, la edición gratuita de Microsoft: lo crea
solo para leer el respaldo, sin conexión a la red ni a ningún SQL Server del restaurante, y lo borra al terminar. Al
preparar un paquete se aceptan las condiciones de licencia de SQL Server Express de Microsoft. La primera vez se
descarga (unos 500 MB).

Si al preparar el paquete el asistente dice:

- **«Falta instalar Docker Desktop»**: instálalo como dice arriba y vuelve a intentarlo.
- **«Docker Desktop no está abierto»**: ábrelo, espera a que diga que está listo y vuelve a intentarlo.
- **«Docker Desktop necesita un ajuste»** (Windows): Docker está usando contenedores de Windows; cámbialo a
  contenedores de Linux desde su menú y vuelve a intentarlo.
- **«Falta memoria para el lector»**: en la configuración de Docker Desktop (Resources), dale al menos 2 GB de memoria.
- **«Docker Desktop apunta a otra computadora»**: en Docker Desktop elige el Docker de esta computadora.
- **«No se pudo descargar el lector»**: la primera vez se descarga el lector del sistema anterior; revisa la conexión a
  internet y vuelve a intentarlo.

## 2. Preparar el paquete

1. Copia el respaldo del sistema anterior (el archivo `.bak`) a la computadora con Docker Desktop, o conecta la USB
   donde lo tienes.
2. Abre el asistente y, en **«1. Preparar el paquete»**, toca **«Elegir el respaldo»** y elige el archivo `.bak`.
3. Toca **«Elegir dónde guardarlo»** y elige la carpeta y el nombre del paquete (se propone
   `sistema-anterior-AAAA-MM-DD.adelmig`). Si ya existe un paquete con ese nombre, elige otro: nunca se reemplaza.
4. Escribe la **frase de paso** dos veces: al menos 8 caracteres. **Guárdala en un lugar seguro: si se olvida, no se
   puede recuperar** (nadie, ni soporte, puede abrir el paquete sin ella) y habrá que preparar el paquete otra vez.
5. Toca **«Preparar el paquete»**. Verás el avance por pasos («Preparar el lector del sistema anterior», «Iniciar el
   lector temporal», «Abrir el respaldo», «Leer los datos», «Guardar el paquete cifrado» y «Borrar el lector
   temporal»). Puedes salir de la pantalla y seguir usando AD-EL POS; al volver verás el avance. Si hace falta,
   **«Cancelar la preparación»**: no queda nada a medias.
6. Al terminar aparece **«El paquete está listo.»** con su tamaño, la fecha del respaldo, el rango de fechas de las
   ventas y cuántos productos, categorías, mesas, usuarios, ventas, pagos y cortes trae. El paquete queda elegido en el
   paso 2 para importarlo en esta computadora; también puedes llevarlo en una USB a la computadora principal del
   restaurante.

**Cuánto tarda**: con el respaldo actual del restaurante (unos 126 MB, siete años de ventas), preparar el paquete
tarda menos de un minuto y el paquete pesa unos 17 MB. Después, revisar el paquete tarda unos segundos e importarlo,
menos de un minuto.

Las contraseñas del sistema anterior **no se copian**: el paquete solo sabe qué usuarios tenían una.

## 3. Importar el paquete

La importación solo se permite en una instalación **sin operación propia** (sin ventas, cobros, turnos de caja ni
movimientos hechos en AD-EL POS) y **sin catálogo, mesas ni usuarios capturados en AD-EL POS** (salvo lo que crea el
asistente inicial, como el usuario `admin` y la Caja 1). Si la instalación ya tiene algo de eso, el asistente dice
**«No se puede importar en esta instalación.»**, lista lo que encontró (por ejemplo «Hay 3 cuentas registradas en
AD-EL POS.») y no importa nada: hace falta una instalación nueva o limpia (pídesela a soporte).

1. Antes de importar, revisa en **Administración › Sistema › Configuración** que la **«Zona horaria del restaurante»**
   sea la correcta: con ella se leen las fechas y horas del sistema anterior.
2. En **«2. Importar el paquete»**, toca **«Elegir el paquete»** (puede estar en una USB) y escribe su **frase de
   paso**.
3. Toca **«Revisar el paquete»**. AD-EL POS sube el paquete y lo revisa; **todavía no se guarda nada**. Si la frase no
   es correcta («La frase de paso no es correcta.»), corrígela y vuelve a tocar «Revisar el paquete».
4. Lee el **resumen previo**: cuántos productos, mesas, usuarios y ventas trae y de qué periodo (por ejemplo «312
   productos, 18 mesas, 6 usuarios y 48 230 ventas del 03/2018 al 11/2026.»), el IVA de los productos, el total vendido
   por año, los retiros y las salidas de efectivo, las ventas canceladas como las contaba el sistema anterior, los
   **datos incoherentes** y **«Para tener en cuenta»** (por ejemplo, que el sistema anterior no guardaba dólares: los
   pagos en dólares quedaron como efectivo en pesos).
5. Toca **«Importar»** y confirma. Antes de importar, AD-EL POS hace **un respaldo de la base de datos** (aparece en
   Mantenimiento › Respaldos).
6. Verás el avance («Haciendo un respaldo de la base de datos», «Guardando el historial de ventas», «Comprobando
   conteos y totales»…). La importación corre aparte: puedes salir de la pantalla y seguir usando AD-EL POS. **Si se
   interrumpe, no deja datos a medias.**
7. Al terminar aparece el **informe de la importación** con la validación: conteos, total vendido por año, pagos por
   forma de pago y año y otros totales, comparados con lo que traía el paquete. Si todo coincide dice **«Todo
   cuadra.»**. Toca **«Descargar informe (PDF)»** y guárdalo.

Después de importar:

- **Usuarios**: los del sistema anterior aparecen en **Administración › Personal › Usuarios** con la etiqueta **«Sin
  acceso»** (no tienen contraseña). Para que alguien pueda entrar, toca **«Asignar contraseña»**, escribe una
  **contraseña inicial** dos veces y compártela con esa persona; al entrar se le recomendará cambiarla por una propia.
- **Reportes**: los reportes de ventas, productos, formas de pago, por día y por turno incluyen el historial. El filtro
  **«Datos»** elige «AD-EL POS y sistema anterior» (lo normal, para comparar con años anteriores), «Solo AD-EL POS» o
  «Solo sistema anterior», y la columna «Datos» dice de dónde viene cada renglón. Un reporte abarca hasta 400 días:
  compara un año a la vez.
- Todo queda anotado en el **Registro de movimientos**: la preparación, la importación y su resultado.

## 4. Repetirlo al pasar a producción

Cuando decidas dejar el sistema anterior:

1. **Cierra el sistema anterior** (que nadie esté vendiendo) y haz con él un **respaldo final**.
2. Prepara un paquete nuevo con ese respaldo (sección 2).
3. Impórtalo en la computadora principal del restaurante (sección 3). Si ya había algo importado, el asistente dice
   **«Ya hay datos importados del sistema anterior.»** con la fecha del respaldo y quién lo importó, y el botón cambia a
   **«Reemplazar lo importado»** (en rojo). Al confirmarlo:
   - El historial importado antes **se borra** y se importa el del paquete nuevo.
   - El catálogo, las mesas y los usuarios importados **se actualizan con lo del paquete**: los usuarios vuelven a tener
     el nombre de usuario, el nombre visible, el rol y el estado (activo o inactivo) del sistema anterior, y **conservan
     la contraseña que les asignaste**. Lo mismo pasa con los productos, las categorías, los complementos y las mesas
     importados: **lo que les hayas cambiado en AD-EL POS se pierde**. Haz esos ajustes después de la importación
     final.
   - Lo importado antes que ya no viene en el paquete se borra si nada lo usa; si ya se usó, se desactiva y queda como
     retirado.
   - Reemplazar no se puede deshacer, salvo restaurando el respaldo que se hace antes.
4. **En cuanto haya operación propia en AD-EL POS** (la primera venta, el primer turno de caja o el primer movimiento),
   **ya no se podrá volver a importar**. Por eso la importación final va justo antes de empezar a vender con AD-EL POS.

**Preparar en una computadora e importar en otra**: puedes preparar el paquete en tu Mac (la que tiene Docker
Desktop), copiarlo a una USB y, en la computadora principal del restaurante, elegirlo desde la USB e importarlo con la
misma frase de paso. Esa computadora **no necesita Docker**.

## 5. Si la validación no cuadra

**Las diferencias de dinero nunca se ocultan.** Si algo no coincide, el informe lo dice primero, en rojo: **«Hay
diferencias de dinero entre el paquete y lo importado.»** (o «Hay conteos que no cuadran con el paquete.»), con cada
concepto, lo que traía el paquete, lo importado y la diferencia (lo importado menos lo esperado). AD-EL POS avisa
«Importación terminada con diferencias. Revisa el informe.» y el lote queda marcado **«Con diferencias»**.

No es lo mismo que los **datos incoherentes** ni que la **«Diferencia del sistema anterior»**:

- **Datos incoherentes** (en el resumen previo y en el informe): cosas que el sistema anterior guardó así, como ventas
  cuyo total no coincide con sus productos, pagos repetidos o en cero, o cortes sin fecha. Se importan **tal como
  estaban**, con una marca y una nota, y nunca se corrigen solas. No son un error de la importación.
- **«Diferencia del sistema anterior»** (en los reportes): cuando una venta del sistema anterior tiene un total que no
  coincide con sus productos, los reportes por venta, día, turno y forma de pago usan el total que guardó el sistema
  anterior, y lo que no coincide aparece como este concepto aparte. Los reportes de productos solo cuentan los
  productos.
- **Ventas canceladas**: el sistema anterior las contaba como vendidas; AD-EL POS no. Por eso el resumen muestra aparte
  «Ventas canceladas (como las contaba el sistema anterior)», para que puedas comparar con sus reportes.

Una **diferencia de la validación**, en cambio, quiere decir que lo importado no coincide con lo que traía el paquete,
y no debería pasar. Si la ves:

1. **No uses el historial importado** para tomar decisiones hasta aclararlo.
2. **Descarga el informe (PDF)** y guárdalo en un lugar seguro (tiene datos del restaurante). Avisa a soporte con él.
3. Para volver a como estaba antes de importar, soporte puede **restaurar el respaldo que se hizo antes de importar**
   (su nombre aparece en el informe, también en el PDF, y en Mantenimiento › Respaldos). Restaurar se hace por línea de comandos en la
   computadora principal, como cualquier respaldo ([INSTALAR_EN_RED.md](INSTALAR_EN_RED.md), «Restaurar un
   respaldo»).
4. También puedes preparar el paquete otra vez con el mismo respaldo e importarlo de nuevo: reemplaza completo lo
   importado (sección 4).

## Pendiente de comprobar

Esta guía se probó con datos de prueba que imitan al sistema anterior y con la ventana simulada. Falta comprobar en
la computadora real las ventanas para elegir archivos, el PDF impreso y **todo en Windows** (preparar con Docker Desktop
para Windows e importar).
