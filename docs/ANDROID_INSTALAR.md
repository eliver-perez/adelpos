# Teléfonos y tabletas de meseros

Guía corta para usar la aplicación **AD-EL POS para Android** en los teléfonos y tabletas de los meseros (etapas A1 y
A2). Con ella cada mesero:

- Entra con **su nombre y su PIN** en un teléfono o una tableta que comparten varios meseros.
- Ve las mesas y sus cuentas, captura los pedidos y los manda a cocina con **«Enviar a cocina»**.
- Toma pedidos **Para llevar** con el mismo catálogo y ve si ya están listos y pagados (sección 6).
- Imprime la **precuenta** de una mesa o de un pedido para llevar en la impresora asignada a ese dispositivo
  (sección 7).
- Recibe un **aviso cuando la cocina termina sus platillos**, aunque la pantalla esté apagada o la aplicación cerrada
  (sección 8).
- Sigue capturando **sin conexión**: los pedidos se mandan solos cuando vuelve la red.

La **computadora principal** sigue mandando en todo: precios, impuestos, permisos y estados los decide ella al recibir
cada pedido. Los precios que muestra el teléfono son de referencia. **El teléfono o la tableta no cobra**: se cobra en
la caja, como siempre.

> **Versión de prueba.** Esta es la versión **0.2.0** de la aplicación. No viene de la tienda de Google (Play Store):
> se instala con un archivo y está firmada con una llave de desarrollo, por eso Android avisa al instalarla. Todavía no
> se ha probado en un teléfono ni en una tableta reales (solo en los emuladores de la Mac de pruebas).

## 1. Instalar la aplicación

Sirve cualquier teléfono o tableta con **Android 8.0 o posterior**.

1. **Pasa el archivo `AD-EL-POS-Android-0.2.0.apk`** al teléfono o la tableta, de la forma que te sea más fácil:
   - **Carpeta compartida** (por ejemplo, Google Drive): súbelo desde la computadora y ábrelo en el dispositivo.
   - **Correo**: envíalo como archivo adjunto y ábrelo desde el correo del dispositivo. Algunos correos (Gmail, por
     ejemplo) no dejan adjuntar archivos `.apk`; si te pasa, usa una carpeta compartida.
   - **Cable USB**: conecta el dispositivo a una computadora, toca en el aviso del dispositivo **«Transferencia de
     archivos»** y copia el archivo a su carpeta **Descargas** (Download). En una Windows aparece como una unidad más;
     en una Mac hace falta un programa aparte para ver los archivos del teléfono.
2. En el dispositivo, abre el archivo (desde la app **Archivos** o **Descargas**, o desde el aviso de descarga).
3. **Permite instalar apps de origen desconocido.** Desde Android 8, el permiso se da a la app con la que abriste el
   archivo (Archivos, Drive, Chrome o el correo), no a todo el teléfono:
   - Al abrir el archivo, Android avisa que por seguridad no puede instalar apps de esa fuente. Toca **«Configuración»**
     (o **«Ajustes»**), activa **«Permitir desde esta fuente»** y vuelve atrás.
   - Si no aparece el aviso, búscalo en los **Ajustes** del dispositivo: escribe «instalar apps desconocidas» en el
     buscador de Ajustes, elige la app con la que abres el archivo y activa **«Permitir desde esta fuente»**.
   - Los nombres cambian un poco según la marca y la versión de Android.
4. Toca **«Instalar»**. Si aparece un aviso de **Play Protect** que dice que no reconoce al desarrollador, busca en el
   aviso la opción para instalarla de todas formas (a veces está en «Más detalles»): es normal en una versión que no
   viene de la tienda.
5. Toca **«Abrir»**. La aplicación se llama **AD-EL POS**.
6. Si quieres, vuelve a desactivar «Permitir desde esta fuente» en esa app: la aplicación ya quedó instalada.

**Instalar una versión nueva.** Instálala **encima** de la que ya tienes, con los mismos pasos: se conservan el
emparejamiento, los meseros y los pedidos sin enviar. Eso funciona si la versión nueva viene firmada con la misma llave;
si Android dice que no puede instalarla porque choca con la que ya está, **no la desinstales** (sobre todo si hay
pedidos sin enviar) y avisa a soporte. **De la 0.1.0 a la 0.2.0 es distinto**: no se instala encima (ver «Pasar de la
0.1.0 a la 0.2.0», al final de esta sección).

**Si desinstalas la aplicación** (o borras sus datos en los Ajustes de Android), el dispositivo olvida el
emparejamiento y se pierden los pedidos que no se hayan enviado. Al volver a instalarla hay que emparejarlo de nuevo
(sección 3). En la computadora principal, revoca el registro anterior de ese dispositivo (sección 10).

### Pasar de la 0.1.0 a la 0.2.0

Desde la 0.2.0, la aplicación tiene su **nombre interno definitivo**. Por eso, para Android la 0.2.0 es **otra
aplicación**: no reemplaza a la 0.1.0 (si la instalas sin quitar la anterior, quedan dos «AD-EL POS» en el
dispositivo), no se queda con nada de lo que guardaba la 0.1.0 y hay que emparejarla de nuevo. Los meseros, sus PIN,
las mesas, las cuentas y el catálogo no se pierden: están en la computadora principal y la 0.2.0 los vuelve a tomar
de ahí.

En cada teléfono o tableta que tenga la 0.1.0, en este orden:

1. **Envía lo pendiente.** Abre la 0.1.0 con conexión y entra a **«Pedidos pendientes»**. Espera a que no quede
   ningún pedido «Por enviar» ni «Enviando» (si uno tarda, toca **«Reintentar ya»**) y resuelve los que digan
   «Requiere tu revisión» con **«Revisar»**. Arriba debe decir «En línea» y ningún pedido por enviar. Lo que no se
   envíe **se pierde** al desinstalar. Si ya actualizaste la computadora principal, la 0.1.0 sigue funcionando con
   ella para esto.
2. **Desinstala la 0.1.0**: en los **Ajustes** del dispositivo, **Aplicaciones › AD-EL POS › Desinstalar** (o
   mantén presionado su ícono y elige «Desinstalar»). Los nombres cambian un poco según la marca.
3. **Retira el dispositivo en la computadora principal**: en **Administración › Sistema › Terminales**, busca ese
   teléfono o esa tableta, toca **«Revocar»** y **«Confirmar revocación»** (sección 10). Así su autorización anterior
   ya no sirve.
4. **Instala la 0.2.0** con los pasos del principio de esta sección y **empareja** el dispositivo con su código
   (sección 3). Puedes ponerle el mismo nombre de antes. Cada mesero entra con su PIN de siempre; la primera vez en
   este dispositivo, con conexión (sin conexión solo entran los que ya entraron antes con conexión en él).
5. **Acepta los permisos del aviso de platillos listos.** La primera vez que un mesero entra con conexión, la 0.2.0
   explica y pide dos permisos nuevos: **mostrar notificaciones** (Android 13 o posterior; toca **«Continuar»** y
   después **«Permitir»**) y **quitar la optimización de batería** (toca **«Continuar»** y después **«Permitir»**).
   Sin ellos la aplicación sigue funcionando, pero los avisos no se ven o pueden llegar tarde con la pantalla apagada
   (sección 8, «Si los avisos no llegan»). Si no das el de notificaciones, la explicación vuelve a salir cada vez que un
   mesero entra con conexión, hasta que lo des. La 0.1.0 no tenía avisos: no hay nada que pasar de una a otra.

Si en un dispositivo nunca se instaló la 0.1.0, no hace falta nada de esto: instala la 0.2.0 directamente. Si por
error quedaron las dos, la 0.1.0 es la que ya estaba emparejada (en **Ajustes › Aplicaciones › AD-EL POS** se ve la
versión de cada una): envía lo pendiente desde ella y sigue desde el paso 2.

## 2. Antes de emparejar

- **La computadora principal debe tener AD-EL POS 0.13.0 o posterior.** Lo ves en la computadora principal, en
  **Administración › Sistema › Configuración › «Ver estado del sistema»**, en «Versión de AD-EL POS». Si tiene una
  versión anterior, el teléfono lo dice antes de pedir el código (y también si ya estaba emparejado):
  **«Actualiza el servidor del restaurante»**, con «Volver a comprobar» y «Elegir otra computadora principal». Lo que
  hay que actualizar es la computadora principal: soporte o el administrador instala la 0.13.0 con su instalador (pide
  la contraseña de administrador de la Mac). Cómo se instala la computadora principal:
  [INSTALAR_EN_RED.md](INSTALAR_EN_RED.md).
- **El teléfono o la tableta debe estar en la misma red wifi del restaurante** que la computadora principal (la red
  principal, no la de invitados).
- Necesitas un usuario con permiso para administrar terminales en la computadora principal (el administrador lo
  tiene).
- Conviene que la computadora principal tenga **siempre la misma dirección** en la red: ver «Asigna una dirección fija
  a la computadora principal» en [INSTALAR_EN_RED.md](INSTALAR_EN_RED.md).

## 3. Emparejar el teléfono o la tableta con la computadora principal

1. Abre **AD-EL POS** en el dispositivo. Aparece **«Buscar el servidor»** y empieza a buscar la computadora principal
   en la red.
2. Revisa el **nombre del dispositivo** (por omisión «Tableta 1» o «Teléfono 1»). Usa uno que lo distinga, como
   «Tableta 2» o «Teléfono terraza»; se puede cambiar después.
3. En **«Computadoras principales en la red»** aparece tu restaurante. Toca su botón, el que dice **«Pedir
   código»**.
   - Si no aparece, toca **«Buscar otra vez»**.
   - O escríbela a mano en **«Escribir la dirección»**: en la computadora principal, **Administración › Sistema ›
     Terminales** muestra en «Terminales nuevas» la dirección de esa computadora (por ejemplo `192.168.1.20`).
     Escríbela tal cual en «Dirección de la computadora principal» y toca **«Pedir código»**.
4. El dispositivo muestra un **código de 6 dígitos en grande** y cuánto falta para que venza. El código **vence a los
   5 minutos**; si vence, toca **«Pedir otro código»**. Ahí mismo puedes corregir el nombre con **«Cambiar nombre»**.
5. En la **computadora principal**, abre **Administración › Sistema › Terminales**. En **«Terminales nuevas»**
   aparece la solicitud con su nombre, «Teléfono o tableta», su sistema y la hora. Escribe en **«Código que muestra
   …»** el código del dispositivo y toca **«Aprobar»**. Si no reconoces la solicitud, toca **«Rechazar»**.
   - Solo se aprueba desde la computadora principal: desde otra terminal no se puede.
   - Tras 5 códigos incorrectos la solicitud se rechaza sola y hay que pedir otro código.
6. El dispositivo dice **«Emparejamiento listo.»** y pasa a la pantalla de entrada, **«¿Quién eres?»**. Si dice
   «Todavía no hay meseros con PIN», sigue con la sección 4.

Después, en **Administración › Sistema › Terminales**, el dispositivo aparece con su tipo («Teléfono o tableta»), el
mesero que lo usa y si está **en línea**. Con **«Editar»** cambias su nombre, su bloqueo por inactividad (sección 4),
su **impresora de precuentas** (sección 7) y sus **avisos de platillos listos** (sección 8). Estas dos últimas opciones
solo aparecen en la computadora principal con la 0.13.0 (o en una terminal que también la tenga).

## 4. Dar permiso y PIN a los meseros

Hazlo en la computadora principal (o en una terminal que ya tenga la 0.12.0: en una versión anterior estas opciones no
aparecen).

### Permisos del rol Mesero

Desde la 0.12.0, el rol **Mesero** ya trae el permiso de entrar con PIN; los demás los das tú (en una instalación
nueva no trae ningún otro). Abre **Administración › Personal › Roles**, toca **«Editar permisos»** en el rol Mesero,
revisa que tenga marcados estos y toca **«Guardar rol»**:

| Para que el mesero pueda…                  | Marca                                                                                 |
| ------------------------------------------ | ------------------------------------------------------------------------------------- |
| Entrar con su PIN                          | «Entrar con PIN en teléfonos y tabletas de meseros» (ya viene marcado)                |
| Ver las áreas y las mesas                  | «Consultar áreas del restaurante» y «Consultar mesas y ocupación real»                |
| Ver las cuentas de una mesa                | «Consultar cuentas e importes»                                                        |
| Empezar una cuenta nueva                   | «Abrir cuentas»                                                                       |
| Capturar y cambiar lo que no se ha enviado | «Capturar y editar borradores de pedidos»                                             |
| Ver el catálogo y los precios              | «Consultar catálogo y configuraciones» y «Consultar precios y previsualizar cálculos» |
| Mandar a cocina                            | «Confirmar pedidos»                                                                   |
| Ver al momento los cambios de otras cajas  | «Recibir eventos autenticados»                                                        |
| Modificar lo ya enviado (opcional)         | «Modificar partidas confirmadas sin aprobación adicional»                             |
| Cancelar unidades ya enviadas (opcional)   | «Cancelar partidas o cantidades confirmadas»                                          |

Para bajar la cantidad de algo ya enviado hacen falta los dos últimos.

Para **Para llevar** (sección 6), marca además:

| Para que el mesero pueda…                                          | Marca                                                   |
| ------------------------------------------------------------------ | ------------------------------------------------------- |
| Ver los pedidos para llevar y sus estados (y la opción en el menú) | «Consultar tablero de pedidos para llevar y ventanilla» |
| Tomar pedidos para llevar y anotar el nombre y el teléfono         | «Administrar datos de pedidos para llevar»              |
| Marcar «Entregado» un pedido ya pagado (opcional)                  | «Registrar entrega de pedidos para llevar»              |

Las dos primeras van juntas (sin la segunda solo ve la lista). Para capturar hacen falta además los de arriba («Abrir
cuentas», «Capturar y editar borradores de pedidos», «Confirmar pedidos», catálogo y precios). Sin la tercera, el
mesero ve si el pedido está pagado y entregado, pero la entrega se marca en la caja. El mesero **nunca cobra** y no se
entrega un pedido sin pagar: si no está pagado, el teléfono dice «Cobrar en caja antes de entregar». Para llevar
necesita la computadora principal en la versión **0.13.0**. Si a un mesero le falta un permiso, el
dispositivo lo dice («No tienes permiso para ver las mesas…», «Sin permiso para enviar a cocina.») y la computadora
principal rechaza lo que no tenga permitido. El permiso de marcar productos como agotados no hace falta: desde el
teléfono no se marcan.

Para **imprimir la precuenta** desde el teléfono o la tableta (sección 7), marca además:

| Para que el mesero pueda…                                 | Marca                           |
| --------------------------------------------------------- | ------------------------------- |
| Imprimir la precuenta (o el ticket de venta, o la cuenta) | «Imprimir tickets y precuentas» |

No hace falta «Reimprimir tickets»: volver a imprimir arma otra precuenta con lo que tenga la cuenta en ese momento,
como en la caja. Sin el permiso, el botón no aparece. Imprimir desde el dispositivo necesita la computadora principal
en la versión **0.13.0** y una impresora asignada a ese dispositivo (sección 7).

**Una sesión con PIN solo permite lo del mesero**, aunque la persona sea administrador o supervisor: lo
administrativo se sigue haciendo en la computadora con su contraseña.

### Asignar el PIN

- **El administrador lo asigna** en **Administración › Personal › Usuarios**: toca **«Asignar PIN»** (o «Cambiar
  PIN») en la persona, escribe el PIN dos veces, su **«Nombre corto»** (así aparece en el botón de la pantalla de
  entrada; si lo dejas vacío, se usa su nombre) y el **«Color del distintivo»**, y toca **«Asignar PIN»**.
- **O cada quien define el suyo** en la computadora, desde el menú de su usuario (arriba, con su nombre) › **«Mi
  PIN»**, con su contraseña actual. «Mi PIN» aparece solo a quien tiene el permiso de entrar con PIN. Tu propio PIN
  de administrador también se define ahí.
- El PIN tiene de **4 a 6 dígitos** y no puede ser fácil de adivinar: la computadora principal rechaza números
  iguales, seguidos o repetidos (1234, 0000, 1212) y los más usados.
- En la pantalla de entrada del dispositivo solo aparecen los usuarios **activos, con PIN y con el permiso** de entrar
  con PIN.
- Al cambiar o quitar el PIN de alguien se cierran sus sesiones en los teléfonos y tabletas. Si tenía algo capturado,
  el dispositivo le pide **«Vuelve a escribir tu PIN»** y conserva lo que llevaba; sus pedidos por enviar esperan a que
  vuelva a entrar con su PIN nuevo y con conexión.

### Si alguien se equivoca de PIN

- Tras **5 errores en 15 minutos**, el PIN de esa persona se bloquea **5 minutos** (en todos los dispositivos). El
  dispositivo dice «Demasiados intentos. Intenta de nuevo a las …».
- Tras **3 bloqueos seguidos en 24 horas** sin una entrada correcta, el PIN queda **bloqueado hasta que un
  administrador lo desbloquee**: en **Personal › Usuarios** aparece «PIN bloqueado: pide desbloqueo»; toca
  **«Desbloquear»**. Todo queda en el Registro de movimientos.
- **Sin conexión** solo pueden entrar los meseros que ya entraron antes **con conexión en ese mismo dispositivo**.

### Bloqueo por inactividad y cambio de mesero

- Tras **2 minutos sin uso** (lo puedes cambiar por dispositivo en **Administración › Sistema › Terminales ›
  «Editar»** del teléfono o la tableta, en «Bloqueo por inactividad»), el dispositivo vuelve a «¿Quién eres?». **No se
  bloquea** mientras el mesero tiene una captura a medias o pedidos sin enviar.
- Para pasarle el dispositivo a otro mesero: toca el nombre del mesero arriba y **«Cambiar de mesero»**. No se puede
  mientras ese mesero tenga **pedidos sin enviar**; una captura a medias sí se queda guardada con él y el siguiente no
  la ve.

## 5. Sin conexión

Arriba, siempre se ve si el dispositivo está **«En línea»**, **«Sin conexión»** y cuántos pedidos hay **por enviar**.
Cada pedido tiene uno de estos estados:

| Estado                        | Qué significa                                                      |
| ----------------------------- | ------------------------------------------------------------------ |
| «Borrador»                    | Capturado, sin mandar. Un borrador **nunca** se va solo a cocina.  |
| «Por enviar»                  | Tocó «Enviar a cocina» sin conexión: se manda solo cuando vuelva.  |
| «Enviando»                    | Se está mandando a la computadora principal.                       |
| «Recibido por el restaurante» | La computadora principal lo recibió. Solo entonces se muestra así. |
| «Requiere tu revisión»        | Algo cambió mientras tanto: el mesero lo ve antes de que entre.    |

- Los pedidos «Por enviar» se mandan solos al volver la red, **aunque la aplicación esté cerrada** o la esté usando
  otro mesero, y no se pierden al cerrar la aplicación ni al reiniciar el dispositivo. Si uno tarda, abre la
  aplicación o toca **«Pedidos pendientes» › «Reintentar ya»**. Nunca se duplica un pedido por reintentarlo.
- **«Requiere tu revisión»** aparece cuando cambió un precio («Hamburguesa: $120.00 → $130.00»), se agotó un producto
  («Malteada de fresa: agotado. Quítalo del pedido.») o la mesa ya se cobró («La cuenta «Mesa 4» ya se cobró.»). El
  mesero lo abre en **«Pedidos pendientes» › «Revisar»**, ve la lista de cambios y elige **«Aceptar los cambios y
  enviar»**, quitar o corregir lo que haga falta, o **«Descartar el pedido»**. Para eso necesita conexión y su PIN.
- Sin conexión se ven las mesas de la última vez que hubo conexión. **Modificar o cancelar lo ya enviado** necesita
  conexión.
- **No se puede cambiar de mesero** con pedidos sin enviar (sección 4).
- **El dispositivo no cobra.** Las cuentas se cobran en la caja.

## 6. Para llevar

Abajo de la pantalla están **«Mesas»** y **«Para llevar»** (en el teléfono y en la tableta). «Para llevar» solo
aparece si el rol del mesero tiene el permiso de ver los pedidos para llevar (sección 4).

**Tomar un pedido.**

1. En «Para llevar», toca **«Nuevo pedido»**: entra directo al catálogo, sin formularios. Se captura igual que en una
   mesa (búsqueda, personalizar, cambiar o quitar lo que no se ha enviado).
2. Si quieres, anota el **nombre del cliente** y su **teléfono** con **«Nombre y teléfono»** (en la cuenta del pedido;
   en el teléfono, en «Ver la cuenta»). Los dos son opcionales y se pueden escribir o cambiar **en cualquier momento**:
   antes del primer producto, mientras capturas o después, desde la lista.
3. Toca **«Enviar a cocina»**, como en una mesa. Vuelves a la lista de Para llevar.

- **Con conexión**, el pedido aparece en la computadora principal en cuanto guardas el primer producto (con su folio).
  **Sin conexión**, aparece cuando se envía: mientras tanto se ve en la lista como «Por enviar» y se manda solo al
  volver la red, con la misma cola y los mismos estados que los pedidos de las mesas (sección 5), incluido «Requiere tu
  revisión».
- Si sales de un «Nuevo pedido» **sin agregar productos**, no queda nada: ni en la computadora principal ni en el
  dispositivo (aunque hayas escrito el nombre).
- El nombre y el teléfono que cambies **sin conexión** se guardan en el dispositivo y se mandan solos en cuanto el
  mesero que los cambió entra con su PIN con conexión. Mientras tanto, la lista dice que se guardarán al volver la
  conexión. Si se cambian dos veces, queda el último.

**La lista.** Muestra los pedidos para llevar abiertos, del más antiguo al más reciente, con su **folio**, el nombre
(si lo anotaron), cuánto tiempo llevan, cómo va la **cocina** («Pendiente», «En preparación», «Listo», o «Sin enviar a
cocina») y el **pago** («Sin pagar», «Pago parcial», «Pagado»), como los dice la computadora principal. Arriba
aparecen los de este dispositivo que todavía no llegan a la computadora principal («Por enviar», o una captura sin
enviar a cocina). Se actualiza sola con conexión; **sin conexión** muestra lo último que se vio y a qué hora («Sin
conexión: es lo último que se vio, a las 14:35.»). Toca un pedido para seguir capturando en él. El estado de la cocina
solo cambia si la cocina marca los platillos en la pantalla de cocina.

**Entregar.**

- **«Entregado»** aparece solo cuando el pedido **ya está pagado** y el mesero tiene el permiso de registrar entregas
  (sección 4). Necesita conexión. Un pedido pagado y entregado sale de la lista.
- Si no está pagado, dice **«Cobrar en caja antes de entregar»**: el teléfono **no cobra**; se cobra en la caja, como
  siempre, y después se entrega.
- Sin el permiso de registrar entregas, el mesero ve si está pagado y entregado, y la entrega se marca en la caja.

## 7. Imprimir la precuenta

En la cuenta de una mesa o de un pedido para llevar (en el teléfono, en «Ver la cuenta»), al final, está el botón
**«Imprimir precuenta»**. Dice el nombre elegido en **Administración › Documentos**: «Imprimir precuenta», «Imprimir
ticket de venta» o «Imprimir cuenta», y el papel sale con ese mismo título. Aparece cuando la cuenta ya está en la
computadora principal (con el primer producto guardado) y el rol tiene el permiso (sección 4).

- La precuenta es **la misma que imprime la caja** (la misma plantilla), con lo que tenga la cuenta en ese momento y
  **«Atendió: <el mesero que la imprime>»**. Tocar el botón otra vez arma otra precuenta, como en la caja.
- Sale **solo** en la impresora asignada a ese teléfono o tableta (abajo). Debajo del botón se ve a dónde va («Se
  imprime en Caja 1.»).
- **Necesita conexión.** Sin conexión el botón no se puede tocar y dice **«Para imprimir necesitas conexión con la
  computadora principal.»**. Nunca queda pendiente para después: cuando vuelva la conexión, tócalo otra vez.
- Mientras sigues en la cuenta, el dispositivo revisa unos segundos qué pasó con la precuenta, sin detener la captura:

| El dispositivo dice                                       | Qué pasa y qué hacer                                                                                                                                                           |
| --------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| «Enviada a Caja 1» (o el nombre de la impresora)          | Llegó a esa impresora (o a la cola de esa caja).                                                                                                                               |
| «La Caja 2 está desconectada.»                            | La impresora es la USB de esa caja y ahí AD-EL POS está cerrado, o la computadora está apagada o sin red. Se imprime sola en cuanto esa caja vuelva; no hace falta reintentar. |
| «La impresora no responde.» con **«Reintentar»**          | No salió en papel. Revisa que esté encendida, con papel y conectada, y toca «Reintentar» (manda la misma precuenta, sin pedir motivo).                                         |
| «La impresora necesita revisión: avisa al administrador.» | La impresora no está bien configurada (se revisa en Cocina › Impresoras). Con «Reintentar», cuando esté corregida.                                                             |
| «No se sabe si salió en papel…»                           | Revisa el papel en la impresora antes de imprimir otra.                                                                                                                        |
| «Este dispositivo no tiene impresora asignada.»           | Hay que elegirle una impresora (abajo). La precuenta no se manda a ninguna otra.                                                                                               |
| «La impresora … está desactivada.»                        | La impresora asignada no puede imprimir ahora: el administrador la revisa en Terminales o en Cocina › Impresoras.                                                              |

### Impresora de precuentas por dispositivo

Cada teléfono o tableta imprime sus precuentas en **su** impresora. Para elegirla, en la computadora principal:

1. Abre **Administración › Sistema › Terminales** y toca **«Editar»** en la fila del teléfono o la tableta.
2. En **«Impresora de precuentas»**, elige una impresora de red o la impresora USB de una caja (aparece como «… · en
   Caja 1»). Las que no sirven aparecen deshabilitadas con el motivo.
3. Toca **«Guardar cambios»**. **«Ver configuración»** muestra cuál tiene.

- **Sin impresora, ese dispositivo no imprime precuentas**: nunca las manda a otra impresora (tampoco a la de tickets
  de Cobros). El mesero ve «Este dispositivo no tiene impresora asignada» y el aviso de pedírsela al administrador.
- Si es la impresora USB de una caja, esa caja debe tener **AD-EL POS abierto**; si no, el teléfono dice «La Caja 2
  está desconectada» y la precuenta sale cuando vuelva a abrirse.
- El dispositivo toma el cambio la próxima vez que se abre una cuenta (o que un mesero entra con conexión).

## 8. Avisos de platillos listos

Cuando la cocina marca **«Listo»** un platillo en la **pantalla de cocina** (KDS), el mesero responsable de esa cuenta
recibe un aviso en su teléfono o su tableta, **aunque la pantalla esté apagada o la aplicación cerrada**: «Mesa 4 · 2
platillos listos» con «2 × Filete de res» debajo, o «Para llevar #12 (Juan) · 1 platillo listo». Nunca muestra
precios. Los avisos de una misma mesa o pedido se juntan en uno solo que va sumando, y con varias cuentas aparece un
resumen. Se ve también en «¿Quién eres?» y en la pantalla de bloqueo.

- **Al tocarlo**, la aplicación abre esa cuenta o ese pedido. Si el dispositivo está en «¿Quién eres?», primero pide
  el PIN del mesero del aviso (ya elegido) y después abre la cuenta. Si entra otro mesero, solo se abre si él puede
  ver esa cuenta. Si mientras tanto la cuenta se combinó con otra, abre esa; si ya se cobró o se canceló, dice **«Esta
  cuenta ya no está abierta»**.
- **Si el mesero ya está viendo esa cuenta** en la aplicación, el aviso aparece arriba de la pantalla unos segundos,
  en lugar de la notificación.
- **Solo funciona si la cocina marca «Listo» en la pantalla de cocina.** Si la cocina solo imprime comandas, no hay
  avisos; la función queda lista para cuando se use.
- **A quién le llega**: al mesero responsable de la cuenta (quien la abrió o a quien se le transfirió), en el
  dispositivo donde usó su sesión más recientemente (entre los que tienen «Avisar platillos listos» activado); si no
  tiene ninguna abierta, en todos los dispositivos donde entró con su PIN durante el día. **Transferir la cuenta**
  cambia a quién llegan los platillos que todavía no estaban listos. A los demás meseros y a los demás dispositivos no
  les llega.
- Un dispositivo solo recibe lo que la cocina terminó **después** de que el mesero entró en él. Una sesión que empezó
  antes de la «Hora de inicio del día» solo cuenta mientras el mesero la siga usando (en la última media hora).
- **No se repiten ni se pierden**: si se corta la wifi justo cuando la cocina marca «Listo», el aviso llega una sola
  vez en cuanto vuelve la conexión. Lo único que no llega después es lo que la cocina terminó antes de la «Hora de
  inicio del día» y no alcanzó a llegar (por ejemplo, por un corte de red que cruza esa hora).
- No hace falta ningún permiso nuevo para el rol Mesero. Requiere la computadora principal en la versión **0.13.0**.

**«AD-EL POS conectado».** Para que los avisos lleguen con la pantalla apagada, Android exige que la aplicación muestre
una **notificación permanente** mientras mantiene la conexión con la computadora principal: «AD-EL POS conectado»
(«Sin conexión con la computadora principal. Reintentando…» si se cortó la red). Aparece cuando un mesero entra con su
PIN **con conexión** y se queda mientras haya meseros que entraron ese día en el dispositivo (o que siguen con su sesión
abierta); desaparece sola cuando ya no hay a quién avisar (al terminar el día comercial, a la «Hora de inicio del día»
de la computadora principal, si nadie sigue con su sesión), si se apaga «Avisar platillos listos», al retirar el
dispositivo o al emparejarlo de nuevo. Si al terminar el día comercial la computadora principal está apagada, también
desaparece unos 10 minutos después (salvo que un mesero esté usando la aplicación); vuelve cuando un mesero entra con
su PIN con conexión. Mientras se conecta dice «Conectando con la computadora principal…». Se ve discreta, sin sonido. Al tocarla se abre la ayuda
«Avisos de platillos listos». Después de reiniciar el teléfono vuelve sola (desde el primer desbloqueo) si el día
comercial no ha terminado. En los equipos con **Android 8.0**, Android agrega junto a ella su propia notificación,
que dice que AD-EL POS se está ejecutando en segundo plano: es del sistema, no es un error y se puede dejar así.

**Sonido y vibración.** Se eligen por dispositivo en la computadora principal: **Administración › Sistema ›
Terminales**, **«Editar»** en el teléfono o la tableta, **«Avisos de platillos listos»**: **«Avisar platillos listos»**
(activado al principio), **«Sonido»** y **«Vibración»**, y **«Guardar cambios»**; **«Ver configuración»** los muestra.
El cambio llega al dispositivo en unos segundos; los avisos que estaban a la vista se quitan y los siguientes ya salen
con lo nuevo. Android también deja ajustarlos en sus Ajustes, en **Aplicaciones › AD-EL POS › Notificaciones ›
«Platillos listos»** (siempre hay un solo canal con ese nombre).

**Si los avisos no llegan.** En el teléfono o la tableta, toca el nombre del mesero arriba a la derecha y elige
**«Avisos de platillos listos»** (o toca la notificación «AD-EL POS conectado»): la ayuda dice qué falta y lleva a los
Ajustes de Android.

- **Notificaciones desactivadas**: toca **«Abrir ajustes de notificaciones»** y actívalas (y el canal «Platillos
  listos»). Sin ellas los avisos no se ven. Mientras estén desactivadas, Mesas muestra arriba «Las notificaciones de
  AD-EL POS están desactivadas…» con **«Cómo activarlas»**, y en Android 13 o más reciente la aplicación vuelve a
  pedir el permiso cada vez que un mesero entra con su PIN con conexión.
- **Optimización de batería**: toca **«Quitar la restricción de batería»** y **«Permitir»**. Sin esto, algunos
  teléfonos retrasan o cortan los avisos con la pantalla apagada. La aplicación gasta muy poca batería: solo mantiene
  la conexión.
- **«Forzar detención»** (en Ajustes › Aplicaciones) o **«Detener»** en el Administrador de tareas de Android paran
  los avisos **hasta que se vuelve a abrir AD-EL POS**. No los uses para «cerrar» la aplicación: quitarla de recientes
  no detiene los avisos.
- Un mesero que entró **sin conexión** no recibe avisos en ese dispositivo hasta su primera entrada con conexión (la
  computadora principal no sabe que entró).
- Algunas marcas de teléfonos cierran las aplicaciones en segundo plano por su cuenta; si pasa, busca en los Ajustes
  de esa marca la opción para dejar que AD-EL POS siga activa.

## 9. Si no encuentra la computadora principal

Revisa en este orden:

1. **La misma wifi.** El dispositivo debe estar en la red principal del restaurante, no en la de invitados. Una red
   que aísla a los equipos entre sí tampoco deja que se vean. Si la wifi se cae, el teléfono puede pasarse a sus datos
   móviles y ahí no ve la computadora principal.
2. **La computadora principal está encendida** y tiene AD-EL POS **0.13.0 o posterior** (sección 2). No hace falta
   abrir AD-EL POS en ella: el servidor arranca solo.
3. **Escribe la dirección a mano** en «Escribir la dirección» (sección 3, paso 3). Sirve en redes que no dejan buscar.
4. **Permisos de la aplicación.** Esta versión no pide permisos de Android para conectarse. Si alguna vez Android
   pregunta por **«Dispositivos cercanos»** o la aplicación muestra «Permiso para la red del restaurante», toca
   **«Permitir»**: sin él no se conecta. Si lo negaste, dalo en los Ajustes del dispositivo, en **Aplicaciones ›
   AD-EL POS › Permisos › Dispositivos cercanos**.
5. **VPN en el teléfono.** Si el dispositivo tiene una VPN encendida (por ejemplo, de una app de seguridad), apágala:
   puede impedir que vea la computadora principal.
6. Lo demás (firewall de la computadora principal, demasiadas solicitudes de código) está en la sección 3 de
   [INSTALAR_EN_RED.md](INSTALAR_EN_RED.md).

Otros avisos que puede mostrar el dispositivo:

- **Si la computadora principal cambió de dirección** en la red, no hay que hacer nada: el dispositivo la vuelve a
  encontrar solo.
- **«… necesita emparejarse de nuevo»** («El servidor del restaurante cambió su identidad…» o «La computadora
  principal ya no reconoce…»): pasa si la computadora principal se reinstaló, se restauró en **otra** computadora o se
  cambió. Toca **«Emparejar de nuevo»** y repite la sección 3. Antes, el dispositivo muestra los pedidos sin enviar,
  que pasan a su bitácora. Si nadie cambió la computadora principal, avisa a soporte antes: el dispositivo se niega a
  conectarse a una computadora que no puede comprobar, a propósito.
- **«Actualiza esta aplicación»**: la computadora principal pide una versión más nueva de la aplicación. Instálala
  encima de la actual (sección 1) y **no la desinstales**, porque se perderían los pedidos sin enviar. De la 0.1.0 a
  la 0.2.0, en cambio, sigue «Pasar de la 0.1.0 a la 0.2.0» (sección 1).

## 10. Retirar un teléfono o una tableta

1. En **Administración › Sistema › Terminales**, busca el dispositivo, toca **«Revocar»** y **«Confirmar
   revocación»**. Desde ese momento ya no puede hacer nada en el restaurante, y queda en el Registro de movimientos.
2. En cuanto el dispositivo se conecta con la computadora principal, muestra **«Este dispositivo fue retirado del
   restaurante»** y borra lo que guardaba: su autorización, los meseros y sus PIN, y el catálogo. Los avisos de
   platillos listos y «AD-EL POS conectado» se detienen.
3. Si tenía pedidos sin enviar, los muestra antes: «Este dispositivo fue retirado del restaurante; estos pedidos no se
   enviaron». **Nunca llegaron a cocina**: si todavía hacen falta, captúralos en otra terminal. Se quedan en la
   bitácora del dispositivo hasta que toques **«Borrar la bitácora»**.
4. Si lo perdieron o lo robaron y no vuelve a conectarse, no se entera, pero de todos modos ya no puede hacer nada en
   el restaurante. Por precaución, cambia el PIN de los meseros que lo usaban.
5. Para volver a usarlo, toca **«Emparejar de nuevo»** en el dispositivo y apruébalo otra vez con su código (sección 3).

## 11. Lo que todavía no hace esta versión de prueba

- El aviso de platillos listos **no se ha probado con la pantalla apagada en un dispositivo real** (Doze y gestores de
  batería de cada marca), solo en emuladores.
- La precuenta desde el teléfono **no se ha impreso en papel real**: se probó con una impresora simulada.
- **No cobra**, ni mesas ni Para llevar: se cobra en la caja.
- No marca productos como agotados (se hace en la caja).
- **No se ha probado en un teléfono ni en una tableta reales**, solo en emuladores.
- Es una **versión de prueba** que no viene de la tienda de Google: se instala con el archivo y las versiones nuevas
  también.
