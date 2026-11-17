# Teléfonos y tabletas de meseros

Guía corta para usar la aplicación **AD-EL POS para Android** en los teléfonos y tabletas de los meseros (etapa A1).
Con ella cada mesero:

- Entra con **su nombre y su PIN** en un teléfono o una tableta que comparten varios meseros.
- Ve las mesas y sus cuentas, captura los pedidos y los manda a cocina con **«Enviar a cocina»**.
- Sigue capturando **sin conexión**: los pedidos se mandan solos cuando vuelve la red.

La **computadora principal** sigue mandando en todo: precios, impuestos, permisos y estados los decide ella al recibir
cada pedido. Los precios que muestra el teléfono son de referencia. **El teléfono o la tableta no cobra**: se cobra en
la caja, como siempre.

> **Versión de prueba.** Esta es la versión **0.1.0** de la aplicación. No viene de la tienda de Google (Play Store):
> se instala con un archivo y está firmada con una llave de desarrollo, por eso Android avisa al instalarla. Todavía no
> se ha probado en un teléfono ni en una tableta reales (solo en los emuladores de la Mac de pruebas).

## 1. Instalar la aplicación

Sirve cualquier teléfono o tableta con **Android 8.0 o posterior**.

1. **Pasa el archivo `AD-EL-POS-Android-0.1.0.apk`** al teléfono o la tableta, de la forma que te sea más fácil:
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
pedidos sin enviar) y avisa a soporte.

**Si desinstalas la aplicación** (o borras sus datos en los Ajustes de Android), el dispositivo olvida el
emparejamiento y se pierden los pedidos que no se hayan enviado. Al volver a instalarla hay que emparejarlo de nuevo
(sección 3). En la computadora principal, revoca el registro anterior de ese dispositivo (sección 7).

## 2. Antes de emparejar

- **La computadora principal debe tener AD-EL POS 0.12.0 o posterior.** Lo ves en la computadora principal, en
  **Administración › Sistema › Configuración › «Ver estado del sistema»**, en «Versión de AD-EL POS». Si tiene una
  versión anterior, el teléfono lo dice antes de pedir el código (y también si ya estaba emparejado):
  **«Actualiza el servidor del restaurante»**, con «Volver a comprobar» y «Elegir otra computadora principal». Lo que
  hay que actualizar es la computadora principal: soporte o el administrador instala la 0.12.0 con su instalador (pide
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
mesero que lo usa y si está **en línea**. Con **«Editar»** cambias su nombre y su bloqueo por inactividad (sección 4).

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

Para bajar la cantidad de algo ya enviado hacen falta los dos últimos. Si a un mesero le falta un permiso, el
dispositivo lo dice («No tienes permiso para ver las mesas…», «Sin permiso para enviar a cocina.») y la computadora
principal rechaza lo que no tenga permitido. El permiso de marcar productos como agotados no hace falta: desde el
teléfono no se marcan.

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

## 6. Si no encuentra la computadora principal

Revisa en este orden:

1. **La misma wifi.** El dispositivo debe estar en la red principal del restaurante, no en la de invitados. Una red
   que aísla a los equipos entre sí tampoco deja que se vean. Si la wifi se cae, el teléfono puede pasarse a sus datos
   móviles y ahí no ve la computadora principal.
2. **La computadora principal está encendida** y tiene AD-EL POS **0.12.0 o posterior** (sección 2). No hace falta
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
  encima de la actual (sección 1) y **no la desinstales**, porque se perderían los pedidos sin enviar.

## 7. Retirar un teléfono o una tableta

1. En **Administración › Sistema › Terminales**, busca el dispositivo, toca **«Revocar»** y **«Confirmar
   revocación»**. Desde ese momento ya no puede hacer nada en el restaurante, y queda en el Registro de movimientos.
2. En cuanto el dispositivo se conecta con la computadora principal, muestra **«Este dispositivo fue retirado del
   restaurante»** y borra lo que guardaba: su autorización, los meseros y sus PIN, y el catálogo.
3. Si tenía pedidos sin enviar, los muestra antes: «Este dispositivo fue retirado del restaurante; estos pedidos no se
   enviaron». **Nunca llegaron a cocina**: si todavía hacen falta, captúralos en otra terminal. Se quedan en la
   bitácora del dispositivo hasta que toques **«Borrar la bitácora»**.
4. Si lo perdieron o lo robaron y no vuelve a conectarse, no se entera, pero de todos modos ya no puede hacer nada en
   el restaurante. Por precaución, cambia el PIN de los meseros que lo usaban.
5. Para volver a usarlo, toca **«Emparejar de nuevo»** en el dispositivo y apruébalo otra vez con su código (sección 3).

## 8. Lo que todavía no hace esta versión de prueba

- **Para llevar**, **imprimir la precuenta** desde el dispositivo y el **aviso de platillos listos**: llegan en la
  siguiente etapa (A2).
- **No cobra**, ni mesas ni Para llevar: se cobra en la caja.
- No marca productos como agotados (se hace en la caja).
- **No se ha probado en un teléfono ni en una tableta reales**, solo en emuladores.
- Es una **versión de prueba** que no viene de la tienda de Google: se instala con el archivo y las versiones nuevas
  también.
