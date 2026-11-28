# Unir mesas y mesas divisibles

Guía corta para **unir mesas** cuando llega un grupo grande y para configurar una **mesa divisible** (por ejemplo, la
mesa en U: «Mesa 3» formada por 3A, 3B y 3C). Funciona igual en la computadora (caja) y en los teléfonos y tabletas de
los meseros, desde la versión 0.18.0 de la computadora principal y la 0.5.0 de la aplicación de meseros.

La **computadora principal decide todo**: qué mesas se pueden unir, el nombre de la unión y si alguien se adelantó desde
otra terminal. Si algo no se puede, lo dice con un mensaje claro y no guarda nada a medias.

## 1. Qué es una unión de mesas

- Una unión es **un grupo de mesas que se atienden como una sola**: «Mesa 5 + 6». Tiene sus cuentas, su mesero, sus
  pedidos, su precuenta y su cobro, igual que una mesa normal.
- Su **nombre** son las mesas en orden («Mesa 5 + 6», «Mesa 2 + 10») y su **capacidad**, la suma de lugares.
- En el **mapa** se ve como **un solo contorno** que abarca sus mesas, con el nombre y los lugares («Mesa 5 + 6 · 8
  lugares»). En la **lista** es una sola entrada. Al tocar cualquiera de sus mesas se abre la unión: mientras dure, sus
  mesas no se abren por separado.
- Los **documentos** (comanda, precuenta, ticket y comprobante), la **pantalla de cocina** y **Preparación** dicen
  «Mesa 5 + 6».
- Los **reportes por mesa** muestran la venta en «5 + 6», **sin repartirla** entre las mesas. Si filtras por la mesa 6,
  también salen las ventas de las uniones en las que estuvo.
- El **historial de la cuenta** dice cuándo se unieron, se agregaron o se quitaron mesas, y quién lo hizo.

## 2. Unir mesas

Las mesas tienen que ser **de la misma área**.

1. Toca la mesa en el mapa (o búscala en la lista) para abrir su panel y toca **«Unir con…»**.
2. Elige las mesas que se agregan. Las que no se pueden se ven apagadas con el motivo: «Ocupada», «Unida con otras
   mesas» o «Fuera de servicio o desactivada».
3. Abajo se ve el nombre y la capacidad de cómo quedaría («Mesa 5 + 6 · 8 lugares») o el motivo por el que no se puede.
   Toca **«Unir»**.

**Desde una mesa libre.** La unión queda **solo en la pantalla** hasta que se guarda el primer producto: el panel lo
dice («La unión se guarda con el primer producto. Si sales sin capturar nada, no queda guardada.») y tiene **«Deshacer
la unión»**. Si sales sin capturar nada, no queda nada guardado y las mesas siguen libres. Si mientras capturas otra
terminal ocupa una de esas mesas, al guardar el primer producto la computadora lo dice («Solo se pueden unir mesas
libres: la Mesa 6 ya está ocupada.»), **no guarda nada y conserva lo capturado**: puedes enviar el pedido solo a la
mesa donde empezaste o esperar.

**Desde una mesa ocupada** (llegaron más personas a una mesa que ya tiene cuenta): la unión se guarda al momento y las
cuentas de esa mesa pasan a la unión, sin perder nada.

**No se unen dos mesas ocupadas.** Si las dos tienen cuentas, la computadora lo dice: «La Mesa 5 y la Mesa 6 tienen
cuentas abiertas. Para juntar sus cuentas usa «Combinar cuentas».». Para que una mesa crezca, empieza siempre desde
la ocupada y agrega las libres.

**En el teléfono o la tableta, unir necesita conexión** con la computadora principal. Sin conexión los botones se ven
apagados («Para unir mesas necesitas conexión con la computadora principal.»). Si se pierde la conexión con una unión
que todavía no se guardaba, el pedido no se manda a otro lado sin avisar: «Sin conexión no se pueden unir mesas. Puedes
enviar el pedido solo a la Mesa 5 o esperar a tener conexión.», con **«Enviar solo a la Mesa 5»** y **«Esperar»**.

## 3. Agregar o quitar mesas de una unión

En el panel de la unión (su nombre es el título):

- **«Agregar mesa»**: elige una mesa libre de la misma área. El nombre de la unión cambia solo («Mesa 5 + 6 + 7»).
- **«Quitar mesa»**: elige una de sus mesas. **Quitar una mesa no saca ninguna cuenta de la unión.** Si al quitarla
  queda una sola mesa, la unión se deshace y las cuentas quedan en esa mesa.

## 4. Cuándo se deshace sola

La unión **se deshace sola en cuanto ya no le quedan cuentas abiertas**: al cobrar y cerrar la última cuenta, al
cancelarla, al descartar una cuenta vacía o al liberar la mesa. Sus mesas vuelven a estar **libres y separadas**. No
hay que hacer nada más.

## 5. Mesas divisibles (la mesa en U)

Una mesa divisible es un **grupo de mesas que se dibujan juntas**, por ejemplo «Mesa 3» formada por 3A, 3B y 3C.

- **Cada sección es una mesa normal**: se abre, se cobra y se libera por separado.
- **«Unir Mesa 3»** une todas las secciones de un toque, con el mismo mecanismo de la sección 2; la unión se llama
  «Mesa 3».
- **Combinaciones parciales**: 3A + 3B dejando libre la 3C, con «Unir con…», como cualquier unión. Se llama con sus
  secciones («Mesa 3A + 3B» o «3A + 3B», según cómo se llamen las mesas).
- **Una sección ocupada impide unir la «Mesa 3» completa**: la computadora lo dice («La sección 3B está ocupada. Únela
  con las secciones libres para crecer.»). Para crecer, abre la sección ocupada, toca «Unir con…» y elige las libres:
  sus cuentas se conservan.
- En el mapa, el grupo se ve con un **marco tenue y su nombre**, y cada sección se distingue y se toca por separado.
  Tocar el nombre del marco es el atajo **«Unir Mesa 3»** (mientras ninguna sección esté unida); también está en el
  panel de cada sección. Cuando las tres están unidas, se ve como «Mesa 3».

### Configurar una mesa divisible

Se hace en la computadora, con un usuario que pueda administrar mesas.

1. Crea primero las secciones como **mesas normales** del área (3A, 3B y 3C), en **Mesas › Áreas y distribución**, y
   acomódalas juntas en el mapa.
2. En la misma pantalla, en el área, abre **«Mesas divisibles»** y toca **«Crear grupo»**.
3. Escribe el **«Nombre del grupo»** («Mesa 3») y elige las **«Secciones, en orden»** (3A, 3B y 3C). Las secciones
   conservan sus nombres y se guardan en el orden en que las eliges. Toca **«Guardar grupo»**.

Para **cambiarlo**, toca **«Editar»** en el grupo (nombre o secciones). Para **quitarlo**, toca **«Quitar grupo»** y
confirma: sus mesas siguen como mesas normales, con sus nombres y sus cuentas; para volver a tenerlo, crea el grupo otra
vez.

Lo que la computadora no deja, con su motivo:

- un grupo con el nombre de otra mesa o de otro grupo del área: «Ya hay una mesa o un grupo con ese nombre en esta
  área.» (tampoco deja crear o renombrar una mesa con el nombre de un grupo);
- una mesa en dos grupos: «La Mesa 3A ya es parte del grupo Mesa 3.»;
- cambiar o quitar un grupo mientras alguna de sus secciones está unida: «El grupo tiene mesas unidas. Cámbialo cuando
  se liberen.»;
- cambiar de área una mesa que es sección de un grupo: «Quita la mesa del grupo antes de cambiarla de área.».

**Si migraste el sistema anterior**, la importación ya arma las mesas divisibles que reconoce por su nombre (por
ejemplo, «Mesa 3a», «Mesa 3b» y «Mesa 3c»). Lo que no reconoce con seguridad lo dice en el resumen y se arma aquí a mano
([MIGRAR_SISTEMA_ANTERIOR.md](MIGRAR_SISTEMA_ANTERIOR.md), «Mesas divisibles del sistema anterior»).

## 6. El permiso «Unir mesas»

Para unir, agregar o quitar mesas hace falta el permiso **«Unir mesas»** (en **Administración › Personal › Roles**, grupo
«Mesas y cuentas»). Lo traen ya marcado los roles **Mesero, Cajero, Supervisor y Administrador**. En una computadora que ya estaba
instalada, la actualización a la 0.18.0 se lo da a esos cuatro roles si ya podían abrir cuentas y ver las mesas; los
roles que creaste no cambian (márcalo a mano si lo necesitan). **«Restablecer permisos predeterminados»** también lo
marca. Para configurar mesas divisibles basta el permiso de siempre, «Administrar mesas y distribución».

Las uniones **no cambian la licencia**, los cupos ni quién puede cobrar.

## 7. Bueno saber

- **Una cuenta que se reabre con una devolución** conserva el nombre que tenía al cerrarse («Mesa 5 + 6»), aunque la
  unión ya se haya deshecho. Si su mesa está ahora en otra unión, la cuenta queda en esa unión.
- **«Modificaciones de cantidades»** (reportes): si dos terminales cambian a la vez, con milisegundos de diferencia, la
  misma unión, ese renglón puede decir la mesa sola en lugar de la unión. No cambia ningún importe.
- **Una terminal de caja o un teléfono con una versión anterior** (por ejemplo, la 0.4.1 de meseros) no ve las uniones:
  ve cada mesa por separado. Si un mesero con la 0.4.1 captura en una mesa unida, la computadora le avisa que ya hay
  una cuenta en la mesa y le ofrece agregar a esa cuenta o abrir otra, que queda dentro de la unión. No se pierde nada,
  pero conviene actualizar todos los dispositivos.
