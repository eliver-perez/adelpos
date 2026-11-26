# Pantalla de cocina en una tableta Android

Guía corta para usar la aplicación **AD-EL POS Cocina** en una tableta de la cocina (etapa K1; desde la etapa K1.1,
también qué recibe cada pantalla, en la sección 4). Con ella, la cocina:

- Ve los pedidos en tarjetas, del más antiguo al más nuevo, apenas los manda un mesero o la caja, sin tocar nada.
- Marca listo cada platillo o el pedido completo, y el mesero recibe su aviso de **platillos listos** en su teléfono
  (ver «Avisos de platillos listos» en [ANDROID_INSTALAR.md](ANDROID_INSTALAR.md)).
- Oye un sonido con cada pedido nuevo y otro distinto con cada cancelación o cambio.

La **computadora principal** manda en todo: qué estaciones ve cada pantalla, los colores por tiempo de espera, la letra,
el tema, el sonido y la licencia se eligen ahí. La tableta solo muestra lo que ella dice. **La pantalla de cocina no
captura pedidos ni cobra.**

> **Versión de prueba.** Esta es la versión **0.1.0** de la aplicación de cocina. No viene de la tienda de Google
> (Play Store): se instala con un archivo y está firmada con una llave de desarrollo, por eso Android avisa al
> instalarla. Todavía no se ha probado en una tableta real, solo en los emuladores de la Mac de pruebas.

## 1. Lo que hace falta

- Una tableta con **Android 8.0 o posterior**, de preferencia de 10 pulgadas o más. La aplicación se usa siempre en
  horizontal.
- La **computadora principal con AD-EL POS 0.16.0 o posterior**. Con una versión anterior, la tableta dice **«Actualiza
  el servidor del restaurante»**. El asistente y los avisos de la sección 4 llegaron con la **0.17.0**.
- La **licencia activada** en la computadora principal, con **cupo de pantallas de cocina** (sección 7).
- La tableta en la **misma red wifi del restaurante** que la computadora principal (la red principal, no la de
  invitados).
- Un usuario con permiso para administrar terminales (el administrador lo tiene).

## 2. Instalar la aplicación

Los pasos son los mismos que los de la aplicación de meseros (sección 1 de [ANDROID_INSTALAR.md](ANDROID_INSTALAR.md)):

1. **Pasa el archivo `AD-EL-POS-Cocina-0.1.0.apk`** (el de la aplicación de cocina, no el de meseros) a la tableta,
   por una carpeta compartida, por correo o con un cable USB.
2. En la tableta, abre el archivo desde la app **Archivos** o **Descargas**.
3. **Permite instalar apps de origen desconocido** a la app con la que abriste el archivo: en el aviso de Android toca
   **«Configuración»** (o **«Ajustes»**), activa **«Permitir desde esta fuente»** y vuelve atrás.
4. Toca **«Instalar»**. Si **Play Protect** dice que no reconoce al desarrollador, elige instalarla de todas formas
   (a veces está en «Más detalles»).
5. Toca **«Abrir»**. La aplicación se llama **AD-EL POS Cocina** y tiene su propio ícono. Es otra aplicación distinta
   de la de meseros: si la tableta también tiene la de meseros, quedan las dos.

**La primera vez** que se abre, Android puede mostrar un aviso de que la aplicación está **a pantalla completa** y de
cómo salir de ella (en inglés, «Viewing full screen»): toca **«Entendido»** (o «Got it»). Sale una vez por
instalación.

**Instalar una versión nueva**: encima de la que ya tienes, con los mismos pasos. Se conserva el emparejamiento.

## 3. Emparejar la tableta como pantalla de cocina

1. Abre **AD-EL POS Cocina**. Aparece **«Buscar el servidor»** y busca la computadora principal en la red.
2. Revisa el **nombre de la pantalla** (por ejemplo «Cocina caliente» o «Barra»): así la verás en Terminales.
3. Toca la computadora principal de tu restaurante, en el botón **«Pedir código»**. Si no aparece, toca **«Buscar otra
   vez»** o escribe su dirección en **«Escribir la dirección»** (la muestra **Administración › Sistema › Terminales**,
   en «Terminales nuevas»).
4. La tableta muestra un **código de 6 dígitos** en grande. Vence a los 5 minutos; si vence, toca **«Pedir otro
   código»**.
5. En la **computadora principal**, abre **Administración › Sistema › Terminales**. En **«Terminales nuevas»** aparece
   la solicitud como **«Pantalla de cocina»**. Escribe el código de la tableta y toca **«Aprobar»**.
6. La tableta pasa sola a la pantalla de **Pedidos**. Si dice «No hay pedidos», ya está lista.
7. En la computadora principal se abre el asistente **«Configurar qué recibe»**, para que los pedidos lleguen a esa
   pantalla (sección 4).

Desde ese momento la tableta **abre directo en Pedidos**: al cerrarla y abrirla otra vez, al forzar su cierre o al
reiniciar la tableta, no hay que hacer nada más.

## 4. Que los pedidos lleguen a la pantalla

Para que un pedido aparezca en la tableta, en la computadora principal tienen que estar bien **tres cosas**:

- una **estación** activa que reciba sus comandas en pantalla;
- las **categorías** (o los productos) que prepara esa estación, en el **enrutamiento**;
- la **pantalla** mostrando esa estación.

Desde la versión **0.17.0**, un asistente lo hace contigo al aprobar la tableta, y Terminales avisa si falta algo.

### Con el asistente (lo más fácil)

Al tocar **«Aprobar»** (sección 3, paso 5) se abre **«Configurar qué recibe»**, con tres pasos:

1. **Estaciones de esta pantalla**: las que reciben comandas en pantalla («Solo pantalla de cocina» o «Impresora y
   pantalla de cocina»).
   - Si no hay ninguna, ofrece **crear «Cocina»**. Si tienes impresoras de cocina, eliges en cuál se imprime también la
     comanda (con una sola, ya viene elegida) o **«Sin impresora: solo pantalla»**; sin impresoras, la crea de solo
     pantalla.
   - Si ya tienes una estación «Cocina» que solo imprime o está desactivada, ofrece **usarla**.
   - Una estación que solo imprime tiene **«Recibir también en pantalla»**: se sigue imprimiendo y además se ve en la
     pantalla.

   Toca **«Siguiente»**.

2. **Qué categorías llegan**: marca las que se preparan en esa estación (por ejemplo, Desayunos y Hamburguesas) y toca
   **«Guardar y seguir»**.
   - Las que ya llegan salen marcadas. Para quitar una, cambia su ruta en **Cocina › Enrutamiento**.
   - Si una categoría iba a otra estación, el asistente lo dice antes de guardar («Desayunos dejará de ir a Barra.»).
   - No marques los refrescos ni lo que se sirve sin preparar: ver «Productos sin ruta», abajo.
3. **Resumen**: qué recibirá la pantalla y qué no, por ejemplo «Esta pantalla recibirá: Desayunos, Hamburguesas.» y «No
   recibirá: Refrescos, Extras (no necesitan preparación).». Toca **«Terminar»**.

Cada paso se puede saltar (**«Saltar este paso»** o **«Saltar el asistente»**). Para abrirlo otra vez, en
**Administración › Sistema › Terminales**, toca **«Configurar qué recibe»** en la fila de la pantalla (también está en
«Ver configuración»). En una pantalla que ya se conectó, el primer paso además deja elegir qué estaciones muestra.

Cada paso necesita su permiso: sin él se ve sin poder cambiarlo, con «Pide a quien administra Cocina que lo configure.».
El administrador los tiene todos.

### Sin el asistente: los pasos en orden

1. **Cocina › Estaciones**: crea la estación (por ejemplo, «Cocina») o abre la que ya tienes. Debe estar **activa** y, en
   **«Recepción de comandas»**, en **«Solo pantalla de cocina»** o **«Impresora y pantalla de cocina»**.
2. **Cocina › Enrutamiento**: manda a esa estación las categorías que se preparan ahí y, si hace falta, algún producto
   suelto. Lo que no tenga ruta sigue el ajuste **«Productos sin ruta»** (abajo).
3. **Administración › Sistema › Terminales**: en la pantalla, toca **«Editar»** y deja **«Mostrar todas las estaciones
   que reciben comandas en pantalla»** o elige esa estación (sección 5).
4. **Prueba con un pedido**: manda desde la caja o desde un teléfono de meseros un platillo de esas categorías. Llega a
   la tableta en unos segundos.

### «Productos sin ruta»

Un producto **sin ruta** es el que no tiene ruta propia ni la de su categoría en Cocina › Enrutamiento: por ejemplo,
la Coca Cola o las papas si nadie les puso estación.

**Desde la 0.17.0, un producto sin ruta nunca detiene el pedido**: lo que sí tiene ruta llega a su estación igual. Con
la 0.16.0, un pedido con un machacado y una Coca Cola no llegaba a la pantalla.

Qué pasa con lo que no tiene ruta se elige en **Cocina › Enrutamiento**, en **«Productos sin ruta»**:

- **«No necesitan preparación»** (así viene): no van a ninguna estación. Es lo normal para refrescos y extras que se
  sirven sin preparar. No salen en la tableta ni hacen esperar al pedido: la tarjeta se quita cuando está listo lo que
  sí se prepara, y el mesero recibe su aviso de platillos listos.
- **«Enviar a una estación»**: todo lo que no tenga ruta llega a la estación que elijas (por ejemplo, Cocina), a su
  pantalla o a su impresora.

Debajo, **«Lo que no va a ninguna estación»** dice qué no tiene ruta, por ejemplo «Sin ruta: Refrescos, Extras (no
necesitan preparación).». Si algo de esa lista sí se prepara, ponle su ruta en Enrutamiento.

Una ruta que va a una estación **desactivada** tampoco detiene nada: esos productos llegan a las otras estaciones de
su ruta o, si no le queda ninguna, se tratan como productos sin ruta. Enrutamiento lo avisa (abajo).

### Revisar qué recibe una pantalla

En **Administración › Sistema › Terminales**, **«Ver configuración»** de la pantalla tiene **«Qué recibe esta
pantalla»**: por cada estación, cómo recibe sus comandas, sus categorías (con cuántos productos) y los productos sueltos
que le llegan. Si algo falta, sale un aviso con un botón que lleva a corregirlo:

| Aviso                                                                                 | Qué significa                                                                                                     | Qué hacer                                                                                                    |
| ------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| «Esta pantalla no tiene estaciones.»                                                  | No muestra ninguna estación activa que reciba en pantalla (por ejemplo, la que tenía elegida se desactivó).       | **«Configurar qué recibe»**: elige o crea su estación.                                                       |
| «Ninguna de sus estaciones recibe comandas en pantalla.»                              | Sus estaciones existen, pero están en «Solo impresora».                                                           | **«Configurar qué recibe»** y, en la estación, **«Recibir también en pantalla»**.                            |
| «La estación Cocina no tiene categorías ni productos asignados.»                      | La estación recibe en pantalla, pero ninguna ruta le manda nada.                                                  | **«Elegir sus categorías»** abre el asistente en el paso de las categorías.                                  |
| «La impresora Cocina 80 de Cocina está desactivada: sus comandas esperan en la cola.» | Sus comandas no se imprimen hasta activar la impresora. Con «Impresora y pantalla de cocina», la pantalla las ve. | **«Ir a Cocina › Impresoras»** y actívala; lo que no salió, en Cocina › Cola de impresión, con «Reintentar». |

En **Cocina › Enrutamiento**, debajo de «Productos sin ruta», salen los avisos de las rutas (los de una ruta, con
**«Cambiar la ruta»**, que la abre para corregirla):

| Aviso                                                                                                     | Qué significa                                                                                      | Qué hacer                                                                         |
| --------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| «Extras va a Barra, que está desactivada: sus productos se tratan como productos sin ruta.»               | La ruta va a una estación desactivada. Mientras tanto, esos productos siguen «Productos sin ruta». | Cambia la ruta a una estación activa o activa la estación en Cocina › Estaciones. |
| «Los productos sin ruta iban a Barra, que está desactivada. Mientras siga así, no necesitan preparación.» | El ajuste «Productos sin ruta» apunta a una estación desactivada.                                  | Elige otra estación o «No necesitan preparación», o activa la estación.           |

## 5. Qué muestra cada pantalla: estaciones, letra, tema y sonido

**Las estaciones.** Cada estación de cocina (Cocina, Barra, Parrilla…) elige cómo recibe sus comandas, en la computadora
principal, en **Cocina › Estaciones**, en **«Recepción de comandas»**:

- **«Solo impresora»**: sus comandas solo se imprimen; ninguna pantalla las muestra.
- **«Solo pantalla de cocina»**: solo se ven en las pantallas de cocina.
- **«Impresora y pantalla de cocina»**: se imprimen y además se ven en las pantallas.

**Lo de cada pantalla.** En **Administración › Sistema › Terminales**, toca **«Editar»** en la pantalla de cocina:

- **Estaciones**: **«Mostrar todas las estaciones que reciben comandas en pantalla»** o, sin esa casilla, solo las que
  elijas en **«Estaciones que muestra esta pantalla»**.
- **«Tamaño de letra»**: Normal o Grande (todo un cuarto más grande).
- **«Tema»**: Oscuro o Claro.
- **«Sonido de pedidos»** (activado o desactivado) y su **«Volumen»**: Bajo, Medio o Alto.

Los cambios **llegan a la tableta al momento**, sin tocarla.

**Los colores por tiempo de espera** son los mismos para todas las tabletas. Se eligen en **Cocina › Comandas y
tiempos**, en **«Colores de las pantallas de cocina»**: el color de atención desde 10 minutos de espera y el de retraso
desde 20, si no los cambias. El modo Cocina de la computadora sigue con los tiempos de cada estación.

## 6. Usar la pantalla de pedidos

- **Cada tarjeta es un pedido**: la mesa (o «Para llevar» con el nombre del cliente), el mesero, la hora en que se
  envió y cuánto lleva esperando. La tarjeta cambia de color cuando el pedido lleva tiempo esperando y cuando ya está
  retrasado.
- **«NUEVO»** marca lo que se agregó después de enviar el pedido. **«CANCELADO»** se queda tachado en la tarjeta hasta
  que se marque el pedido.
- **Tocar un platillo lo marca listo**: se atenúa con una palomita. Cuando todo lo de una tarjeta está listo, la
  tarjeta se quita sola.
- **«Pedido listo»** marca la tarjeta completa. En las estaciones que se marcan **por pedido**, tocar un platillo no
  hace nada: la tarjeta lo dice y se usa el botón.
- **«Recientes»** (arriba) muestra lo que se marcó listo en los últimos 30 minutos. **«Recuperar»** lo regresa a la
  pantalla si se marcó por error (al mesero no le llega otro aviso).
- **Sonidos**: uno con cada pedido nuevo o agregado y otro distinto, más grave, con cada cancelación o cambio. Al
  abrir la aplicación no suena por los pedidos que ya estaban. Si la tableta estuvo sin conexión, al volver suena
  **una sola vez** por lo que llegó mientras tanto, no una vez por pedido. Además, la tarjeta nueva **parpadea** un
  momento en azul, para quien no escuchó.
- **Con muchos pedidos**, las tarjetas se acomodan en columnas; abajo se ve cuántos pedidos más hay y se desliza para
  verlos.

## 7. El cupo de pantallas de cocina

La licencia de la computadora principal dice **cuántas pantallas de cocina** se pueden usar a la vez
(**Configuración › Licencia**, y **Administración › Sistema › Terminales**, en «Cupos de la licencia»). Cada tableta
ocupa un cupo al conectarse, y lo **sigue ocupando aunque se corte la red** o se apague la tableta.

- **Sin cupo libre**, la tableta dice **«No hay cupo de pantalla de cocina libre (N de N en uso). Revoca otra pantalla
  en Terminales o amplía tu licencia.»** y lo vuelve a intentar sola cada 30 segundos.
- **«Liberar cupo»** (en Terminales, en la fila de la pantalla) deja el cupo libre para que lo use otra pantalla.
  **No es definitivo**: esa tableta dice **«Esta pantalla dejó su cupo de cocina.»** y, si alguien toca **«Usar esta
  pantalla»** en ella, lo pide otra vez (si hay cupo).
- **«Revocar»** (en Terminales) **sí es definitivo**: la tableta deja de estar emparejada, vuelve a «Buscar el servidor»
  y su cupo queda libre. Para usarla otra vez hay que emparejarla con un código nuevo.
- **«Volver a emparejar»** (desde la misma tableta, sección 8) hace lo mismo que «Revocar»: suelta su cupo y la deja
  lista para emparejarla de nuevo.

Con la **licencia vencida o revocada**, la cocina sigue mostrando y marcando pedidos, con un aviso arriba. **Sin
activar**, no muestra pedidos.

## 8. Ajustes en la tableta

Están escondidos para que el personal no los cambie por error:

1. **Mantén presionado el logotipo de AD-EL POS**, arriba a la izquierda, un par de segundos.
2. Escribe el **usuario y la contraseña de un administrador** (alguien que pueda administrar las terminales) y toca
   **«Abrir los ajustes»**. Hace falta conexión con la computadora principal.
3. Los ajustes muestran solo esto: la **dirección de la computadora principal**, el **estado de la conexión**, la
   **versión de la aplicación** y **«Volver a emparejar»**.

**«Volver a emparejar»** pide confirmación, deja el cupo de cocina libre en la computadora principal y vuelve a «Buscar
el servidor». Si la computadora principal no responde en ese momento, la tableta se desempareja de todos modos y lo
dice: entonces **revócala en Terminales** para soltar su cupo.

La contraseña no se guarda en la tableta: se olvida al cerrar los ajustes, y los ajustes **se cierran solos** si nadie
los toca en dos minutos.

## 9. Dejar la tableta lista para la cocina

### Que la pantalla no se apague

Mientras la aplicación está abierta, **la pantalla no se apaga**, aunque la tableta tenga un tiempo de espera de la
pantalla corto. Deja la tableta **conectada al cargador**.

### Fijar la aplicación en la pantalla

La **fijación de pantalla** de Android deja la tableta en AD-EL POS Cocina: el personal no puede salir a otra
aplicación ni a la pantalla de inicio sin quitar la fijación.

**Activar la fijación** (una vez):

- **Android 9 o posterior**: **Ajustes › Seguridad** (en algunas tabletas, **Seguridad y privacidad › Más ajustes de
  seguridad**) › **«Fijar apps»** o **«Fijación de pantalla»**: actívala. Conviene activar también **«Pedir PIN para
  dejar de fijar»** (o «Bloquear el dispositivo al dejar de fijar»).
- **Android 8**: **Ajustes › Seguridad y ubicación** (o **Seguridad**) › **«Fijar pantalla»**: actívala, y también
  «Solicitar PIN para dejar de fijar».

**Fijar AD-EL POS Cocina**:

1. Abre AD-EL POS Cocina.
2. Abre la vista de **apps recientes** (el botón cuadrado, o desliza hacia arriba desde el borde inferior y detente).
3. **Android 9 o posterior**: toca el **ícono de AD-EL POS Cocina** arriba de su vista previa y elige **«Fijar»**.
   **Android 8**: desliza la vista previa de AD-EL POS Cocina hacia arriba y toca el **ícono de alfiler** abajo a la
   derecha.
4. Android explica cómo quitar la fijación; toca **«Entendido»**.

**Quitar la fijación**: mantén presionados **Atrás** y **Recientes** a la vez (o, con navegación por gestos, desliza
hacia arriba y mantén). Si activaste el PIN, la tableta lo pide.

La aplicación **no se abre sola al encender la tableta**: después de reiniciarla, ábrela (vuelve directo a Pedidos) y
fíjala otra vez.

### El volumen de la tableta

Los sonidos de la pantalla de cocina salen por el **volumen de notificaciones** de la tableta, no por el de multimedia
(música y videos). En algunas tabletas se llama **«Volumen de tono y notificaciones»** o va junto con el del timbre.

- Sube ese volumen en **Ajustes › Sonido** (o con los botones de volumen, abriendo el control completo con los tres
  puntos o el engrane).
- El **«Volumen»** de Terminales (bajo, medio o alto) se aplica **sobre** el de la tableta: con el volumen de
  notificaciones en cero no suena nada.
- **No actives «No molestar»** en la tableta de la cocina: silencia los sonidos de la pantalla de cocina, como cualquier
  notificación.

## 10. Si no conecta

- Un corte corto de la red (de unos segundos) **no muestra nada**: la tableta sigue mostrando los pedidos, y lo que se
  marque en ese momento se manda solo cuando vuelve la conexión.
- Si el corte dura, arriba aparece **«Sin conexión con la computadora principal. Reconectando…»**. Mientras se vea, **no
  se puede marcar nada** y los pedidos pueden no estar al día. **Se reconecta sola**, sin tocar la tableta; al volver,
  el aviso se quita y la pantalla se pone al día.

Si el aviso no se quita:

1. Revisa que la **computadora principal esté encendida** y con AD-EL POS abierto.
2. Revisa que la tableta esté conectada a la **wifi del restaurante** (la misma de la computadora principal) y no a la
   de invitados ni a datos móviles.
3. Abre los **ajustes de la tableta** (sección 8): muestran la dirección que usa. Si la computadora principal cambió de
   dirección, la tableta la vuelve a encontrar sola en la red; para que no cambie, ver «Asigna una dirección fija a la
   computadora principal» en [INSTALAR_EN_RED.md](INSTALAR_EN_RED.md).

Otros avisos:

- **«La computadora principal tiene la fecha y hora incorrectas»**: se siguen viendo los pedidos, pero no se pueden
  marcar hasta corregir la hora de la computadora principal.
- **«Actualiza el servidor del restaurante»**: la computadora principal tiene una versión anterior a la 0.16.0.
- **«Empareja de nuevo»** o **«Buscar el servidor»**: la pantalla se revocó en Terminales o se perdió su emparejamiento.
  Emparéjala otra vez con un código (sección 3).
- **«No hay cupo de pantalla de cocina libre…»**: sección 7.

Si la tableta está conectada pero **un pedido no le llega**:

1. Revisa **«Ver configuración»** de la pantalla en Terminales: sus avisos dicen qué falta (sección 4).
2. Un **pedido para llevar con hora** todavía no llega a la cocina. Se ve en la computadora principal, en **Cocina ›
   Preparación**, en **«Por enviar a cocina»**, que dice si se envía solo a su hora o si espera a que lo envíes. Para
   mandarlo antes, **«Enviar a cocina ahora»**.
3. Si a un pedido con hora le quitaron la hora, también espera ahí a que alguien toque **«Enviar a cocina ahora»**.

## 11. Lo que todavía no hace esta versión de prueba

- No captura pedidos, no cobra y no imprime: eso sigue en la caja, los teléfonos de los meseros y las impresoras de
  cocina.
- No marca «En preparación»: solo listo (lo que se ponga «En preparación» en la computadora se ve igual como pendiente).
- No se abre sola al encender la tableta (sección 9).
- No suena con la aplicación cerrada: los sonidos son de la pantalla abierta.
- «Recientes» llega solo hasta 30 minutos atrás, y la pantalla muestra como mucho 200 pedidos a la vez.
- No viene de la tienda de Google y no se ha probado en una tableta real.
