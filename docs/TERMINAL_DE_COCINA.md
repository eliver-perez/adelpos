# Terminal de cocina

Guía corta para usar una computadora o pantalla como **terminal de cocina**: que cada área (por ejemplo cocina,
barra y postres) vea solo sus pedidos, con tarjetas grandes que cambian de color según el tiempo.

Se configura en cuatro pasos, siempre con un usuario administrador.

## 1. Crear las estaciones

En **Cocina › Estaciones › Nueva estación** crea un área por cada lugar donde se prepara algo: por ejemplo
«Plancha», «Freidora», «Barra» y «Postres».

- **Recepción de comandas**: «Solo pantalla de cocina» si esa área solo usará la pantalla; «Impresora y
  pantalla de cocina» o «Solo impresora» si también (o solo) imprime su comanda. Con impresora, elige cuál.
- **Tiempos y advertencias**: el **tiempo objetivo** de la estación (por ejemplo 15 minutos) y la
  **advertencia**. En la pantalla de cocina la tarjeta se pone **ámbar** desde la advertencia (si la advertencia
  es igual o mayor que el objetivo, desde el 75 % del objetivo) y **roja** al pasar el objetivo, con el texto
  «Por vencer» o «Atrasada».

## 2. Asignar los productos a cada estación

En **Cocina › Enrutamiento › Nueva ruta** indica a qué estaciones va cada **categoría** (por ejemplo, todos los
«Platillos» a la plancha) o cada **producto específico** (la ruta del producto gana a la de su categoría).

- Si eliges **dos estaciones** para un mismo producto, **las dos reciben el producto completo** (las dos lo
  preparan). Úsalo solo si de verdad lo preparan las dos.
- Para que **cada estación vea solo su parte** (por ejemplo, la hamburguesa en la plancha y las papas en la
  freidora), vende el platillo como **combo** (Catálogo) con sus componentes, y da a cada componente su propia
  ruta: la hamburguesa a la plancha y las papas a la freidora. Cada estación recibe solo su parte; al marcar
  «Marcar producto listo» en una, solo se marca esa parte, y el platillo queda listo (también en la cuenta)
  cuando todas sus partes lo están.
- Los **complementos** que se eligen dentro de un producto (por ejemplo, la guarnición de papas de una
  hamburguesa) viajan **con su producto** a la estación de ese producto; no se envían a otra estación aunque
  estén ligados a un producto con otra ruta.

## 3. Crear un usuario de cocina con sus estaciones

1. En **Administración › Personal › Roles**, da al rol **«Cocina»** (o a uno nuevo) solo los permisos de cocina:
   «Consultar preparación», «Actualizar preparación», «Consultar comandas por estación», «Confirmar recepción
   KDS» y «Recibir eventos autenticados» (para que la pantalla se actualice al momento).
2. En **Personal › Usuarios › Crear usuario**, crea el usuario con ese rol.
3. Ábrelo con **Administrar › Estaciones de cocina** y elige **«Estaciones que puede atender»**: todas (la opción
   predeterminada) o solo las suyas. Quien tiene el permiso «Operar todas las estaciones del establecimiento»
   (el administrador) siempre atiende todas.

Con solo permisos de cocina, la barra superior de ese usuario muestra únicamente **Cocina** y su menú.

## 4. Configurar la terminal

1. En **Administración › Sistema › Terminales › Editar**, elige **Cocina** como **pantalla inicial**: la terminal
   abrirá directo en **Cocina › Preparación**.
2. En **Cocina › Estaciones › Terminales de cocina**, elige la terminal y sus **estaciones que muestra**: todas
   (la opción predeterminada) o solo las de esa área.
3. En el mismo lugar puedes activar el **aviso sonoro al llegar una comanda nueva** (desactivado por omisión).
   Suena una vez por pedido nuevo de las estaciones que se ven; no suena al abrir la pantalla.

## Qué se ve en la terminal

- La terminal muestra **solo las estaciones que tiene asignadas y que el usuario puede atender**. Si no tienen
  ninguna en común, la pantalla lo dice (por ejemplo: «Esta terminal muestra Barra y Postres, y tu usuario
  atiende Cocina caliente: no hay estaciones en común. Pide que ajusten la terminal o tu usuario.»).
- Los pedidos aparecen **más antiguos primero**, con botones grandes («En preparación» y «Marcar producto listo»).
- La pantalla **se actualiza sola**. Si la barra superior dice **«Sin conexión»**, aparece **«Reintentar»**.
- La cola de impresión, el historial y liberar o programar pedidos a mano siguen siendo del administrador.

## Pendiente de comprobar

Esta guía se probó con la aplicación y pantallas simuladas. Falta comprobarla en el restaurante con una
pantalla táctil real, con el sonido del equipo y con dos terminales a la vez.
