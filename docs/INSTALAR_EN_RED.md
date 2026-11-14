# Instalar AD-EL POS en red

Guía corta para usar AD-EL POS con **una computadora principal y varias terminales** (cajas, cocina) en la red del
restaurante. Desde la versión 0.10.0 (etapa 9G):

- La **computadora principal** guarda la base de datos del restaurante y atiende a todas las terminales. Arranca sola
  al encender la computadora, aunque nadie inicie sesión ni abra la ventana, y sigue funcionando al cerrar la ventana.
- Las **terminales** se conectan a la computadora principal por la red del restaurante. Ninguna terminal se conecta sin
  que un administrador la apruebe en la computadora principal, y todo lo que viaja por la red va cifrado.
- Si una terminal pierde la conexión, muestra «Sin conexión con el servidor», no deja vender y se reconecta sola.

> **Por ahora solo en Mac.** La computadora principal y las terminales de esta guía son Mac. Windows (servicio,
> instalador para todo el equipo y terminales en red) llega en la Fase W de la etapa 9G y esta guía se completará
> entonces.

> **Si esta Mac ya tenía AD-EL POS** (una versión anterior a la 0.10.0), no instales sin soporte: antes hay que pasar
> los datos del restaurante a la carpeta nueva, con respaldo y comprobación. Soporte tiene el procedimiento en
> [DEVELOPMENT_MACOS.md](../DEVELOPMENT_MACOS.md).

## 1. Instalar la computadora principal

Elige una computadora que pueda quedarse encendida durante el servicio (puede ser la de la caja principal).

1. **Antes de instalar**, soporte deja instalado en esa Mac el programa de base de datos que usa AD-EL POS
   (PostgreSQL 18). Si falta, el instalador lo avisa y no instala nada.
2. Copia el instalador `AD-EL POS_0.10.0.pkg` a la Mac y haz doble clic en él.
   <!-- PENDIENTE-COORDINADOR: tamaño del .pkg 0.10.0 y su SHA-256 cuando se compile en la Fase G. -->
   Si macOS avisa que no puede comprobar el instalador (no está firmado; es normal en esta versión), abre
   **Ajustes del Sistema › Privacidad y seguridad** y toca **«Abrir de todos modos»**.
3. En **Tipo de instalación** deja marcada la opción **«Servidor del restaurante»** y toca **Instalar**.
4. Aparece la ventana de macOS que pide la **contraseña de administrador** de la computadora (o Touch ID). Es la de la
   Mac, no la de AD-EL POS.
5. macOS muestra el aviso **«Elementos en segundo plano añadidos»**: déjalo permitido (en Ajustes del Sistema ›
   General › Ítems de inicio y extensiones, AD-EL POS debe quedar con «Permitir en segundo plano» activado). Es lo que
   permite que el servidor arranque solo.
6. Abre **AD-EL POS** desde Aplicaciones. Pregunta **«¿Cómo se usará esta computadora?»**:
   - **Servidor y terminal**: la computadora principal que además se usa como caja (lo más común).
   - **Solo servidor**: una computadora apartada que nadie usa para vender. Solo muestra una pantalla pequeña con el
     estado del servidor, las terminales conectadas, el último respaldo y el acceso a Administración.
7. En una instalación nueva aparece el asistente inicial (nombre del restaurante, zona horaria) y después se entra con
   **admin / admin**. Cambia esa contraseña en cuanto puedas.
8. macOS puede pedir permiso para que AD-EL POS use el **Llavero** (sus credenciales guardadas): escribe la contraseña
   de la sesión y toca **«Permitir siempre»**.

La modalidad se puede cambiar después en **Administración › Sistema › Mantenimiento › «Modalidad de esta
computadora»**, con confirmación. Cambiarla **nunca borra la base de datos**: si esta computadora deja de ser la
principal, sus datos se quedan guardados en ella. En esa misma pantalla, **«Servicio del servidor»** dice si el
servidor funciona y ofrece **«Reiniciar»** si hiciera falta.

### Asigna una dirección fija a la computadora principal

Las terminales encuentran solas a la computadora principal aunque su dirección en la red cambie, pero es más rápido y
más seguro que la computadora principal tenga **siempre la misma dirección**:

1. En la computadora principal, en **Administración › Sistema › Terminales**, aparece **la dirección de esta
   computadora** (por ejemplo `192.168.1.10`, o un nombre que termina en `.local`). Anótala.
2. Entra a la configuración de tu **módem o router** (normalmente desde un navegador; la dirección y la contraseña
   vienen en una etiqueta del módem o te las da tu proveedor de internet) y busca la opción **«Reserva de DHCP»**,
   **«IP fija»** o **«Dirección estática»**. Asigna a la computadora principal la dirección que anotaste.
3. Si no sabes hacerlo, pídele a tu proveedor de internet o a soporte que «reserven la IP de la computadora principal».
   Cambiar la configuración del módem es cosa de quien lo administra: si no estás seguro, no lo hagas solo.

## 2. Instalar una terminal y emparejarla con el código

1. En la Mac de la terminal, abre el mismo instalador `AD-EL POS_0.10.0.pkg`. En **Tipo de instalación**
   **desmarca «Servidor del restaurante»** (así queda como «Solo terminal»: no necesita la base de datos). Escribe la
   contraseña de administrador de esa Mac cuando la pida.
2. Abre **AD-EL POS** y elige **«Solo terminal»**. Aparece **«Buscar el servidor»**.
3. Escribe el **nombre de esta terminal** (por ejemplo «Caja 2»; se puede cambiar después).
4. macOS pregunta si AD-EL POS puede **buscar y conectarse a dispositivos de la red local**: toca **Permitir**. Sin ese
   permiso la terminal no puede encontrar ni conectarse a la computadora principal.
5. En **«Computadoras principales en la red»** aparece tu restaurante (por ejemplo «Fonda Doña Ñeli — Mac de caja»).
   Toca **«Pedir código a …»**. Si no aparece, usa **«Escribir la dirección»** con la dirección que anotaste en el
   paso anterior (ver la sección 3).
6. La terminal muestra un **código de 6 dígitos en grande** (por ejemplo `481 926`) y cuánto tiempo le queda. El
   código **vence a los 5 minutos**; si vence, toca «Pedir otro código».
7. En la **computadora principal**, con un usuario que tenga permiso de administrar terminales, abre
   **Administración › Sistema › Terminales**. En **«Terminales nuevas»** aparece la solicitud con su nombre, sistema y
   hora. **Escribe el código que muestra la terminal** y toca **«Aprobar»**. Si no reconoces la solicitud, toca
   **«Rechazar»**.
   - Solo se aprueba desde la computadora principal: desde otra terminal no se puede.
   - Tras 5 códigos incorrectos la solicitud se rechaza sola y hay que pedir otro código.
8. La terminal se reinicia y muestra el acceso normal. Puede aparecer otra vez el permiso del **Llavero**: «Permitir
   siempre». Inicia sesión con tu usuario.

Después, en la computadora principal, en **Sistema › Terminales**, la terminal aparece con su tipo, sistema, usuario
actual y si está **en línea**. Con **«Editar»** eliges su caja, su pantalla de inicio y su teclado; con **«Ver
configuración»** ves sus estaciones de cocina y sus impresoras. Las estaciones de una terminal de cocina se eligen en
**Cocina › Estaciones** ([TERMINAL_DE_COCINA.md](TERMINAL_DE_COCINA.md)).

**Un usuario, una sesión.** Cada usuario puede tener la sesión abierta en una sola terminal a la vez: si entra en otra,
la primera se cierra. Da a cada persona su propio usuario.

### Impresora USB conectada a una caja

Una impresora de tickets conectada por USB a una terminal (por ejemplo, la de la Caja 2) también funciona:

1. Conecta la impresora a esa computadora e instálala en macOS con su controlador.
2. Abre AD-EL POS en esa terminal (basta con que esté abierta, aunque nadie haya iniciado sesión).
3. En **Cocina › Impresoras**, agrega una **«Impresora USB de una terminal»**: elige la terminal y después la impresora
   de su lista.
4. Para que los tickets de esa caja salgan en su impresora: **Cobros › Políticas de cobro › «Impresora de tickets por
   caja»**. Una caja sin impresora propia usa la impresora de tickets general.

Si esa terminal está apagada o cerrada, lo que tenga que imprimir **espera** y la cola de impresión dice «La Caja 2
está desconectada»; al encenderla y abrir AD-EL POS sale solo, sin reintentar.

## 3. Si una terminal no encuentra el servidor

Revisa en este orden:

1. **La computadora principal está encendida.** No hace falta abrir AD-EL POS en ella: el servidor arranca solo.
2. **Las dos están en la misma red** (el mismo módem o el mismo Wi-Fi). Una red de invitados o una red que aísla a los
   equipos entre sí no deja que se vean: usa la red principal del restaurante.
3. **Permiso «Red local» de la terminal**: en **Ajustes del Sistema › Privacidad y seguridad › Red local**, AD-EL POS
   debe estar activado. Sin él no funciona ni la búsqueda ni la dirección escrita a mano.
4. **Escribe la dirección a mano**: en la computadora principal, **Sistema › Terminales** muestra su dirección; en la
   terminal, en «Buscar el servidor», usa **«Escribir la dirección»**. Sirve en redes que bloquean la búsqueda
   automática.
5. Si la computadora principal tiene activado el **firewall de macOS** (Ajustes del Sistema › Red › Firewall), debe
   permitir las conexiones entrantes de AD-EL POS.
   <!-- PENDIENTE-COORDINADOR: el firewall de la Mac de pruebas está desactivado; confirmar con la instalación real si
   hace falta este paso y cómo se ve el aviso. -->

Otros avisos que puedes ver en una terminal:

- **«Sin conexión con el servidor»**: la terminal perdió la red o la computadora principal se apagó. La pantalla queda
  bloqueada pero **lo que capturaste no se pierde** mientras la ventana siga abierta; la terminal reintenta sola (o
  toca «Reintentar») y, al volver, se pone al día.
- **La dirección de la computadora principal cambió**: no hay que hacer nada; la terminal la vuelve a encontrar sola
  (la primera operación tarda un par de segundos). Por eso conviene la dirección fija de la sección 1.
- **«El servidor cambió su identidad…» o «Esta terminal necesita emparejarse de nuevo»**: la computadora principal ya
  no es la misma que se aprobó (se reinstaló, se restauró en otra computadora) o la terminal fue revocada. Toca
  **«Emparejar de nuevo»** y repite la sección 2. Si nadie cambió la computadora principal, avisa a soporte antes:
  la terminal se niega a conectarse a un equipo que no puede comprobar, a propósito.
- **«Actualiza esta terminal»** o **«Actualiza el servidor»**: la terminal y la computadora principal tienen versiones
  que no se entienden. Instala en las dos la misma versión.
- **Demasiadas solicitudes de código**: si en 10 minutos se pidieron muchos códigos, la computadora principal deja de
  aceptar solicitudes un rato y **Sistema › Terminales** lo avisa. Espera 10 minutos y vuelve a pedir el código. Si
  nadie del restaurante las pidió, avisa a soporte: puede ser un intento de alguien en la red.

## 4. Revocar una terminal perdida o robada

1. En **Administración › Sistema › Terminales** (con permiso de administrar terminales), busca la terminal.
2. Toca **«Revocar»** y confirma. La terminal **se desconecta en ese momento**, su sesión se cierra y ya no puede hacer
   nada. Queda anotado en el **Registro de movimientos**.
3. Si la terminal fue robada, cambia también la contraseña de los usuarios que la usaban (Personal › Usuarios).
4. Si aparece después, se puede volver a usar: en esa terminal, «Emparejar de nuevo» y apruébala otra vez con su código
   (sección 2).

## 5. Dónde están los respaldos y cómo cambiar su carpeta

La computadora principal respalda sola la base de datos del restaurante **todos los días** a la «Hora de inicio del
día» (05:00 si no la cambiaste) y conserva los **últimos 30** respaldos automáticos; los más viejos se borran solos. Si
la computadora estaba apagada a esa hora, el respaldo se hace unos minutos después de encenderla. Cada respaldo se
revisa al crearse.

- **Dónde están**: en la carpeta de respaldos de la computadora principal (en la Mac,
  `/Library/Application Support/AD-EL POS/backups`) o en la carpeta que elijas. En **Administración › Sistema ›
  Mantenimiento › Respaldos** ves el último respaldo, si fue correcto, dónde está y la lista de los más recientes (la
  ruta completa está en «Información para soporte»). Ahí mismo cambias la hora, cuántos conservar o desactivas el
  respaldo automático, y tienes **«Respaldar ahora»**.
- **Cambiar la carpeta** (por ejemplo, a otro disco o a una memoria USB): en la **computadora principal**, en
  Mantenimiento › Respaldos, toca **«Elegir carpeta»** y elige la carpeta en la ventana que se abre. Desde otra
  terminal no se puede. Para volver a la de siempre, **«Usar la carpeta predeterminada»**.
- **Permiso de macOS para una USB o un disco externo**: el servidor de AD-EL POS funciona como un servicio de la
  computadora y macOS puede no dejarle guardar en una USB. Si al elegir la carpeta dice «El servidor no puede guardar
  archivos en esa carpeta», en la computadora principal abre **Ajustes del Sistema › Privacidad y seguridad › Acceso
  total al disco**, toca **«+»**, agrega `/Applications/AD-EL POS.app/Contents/MacOS/ad-el-pos-server` (en la ventana
  para elegir, ⌘⇧G permite escribir esa ruta) y actívalo; después, en Mantenimiento, **«Reiniciar»** el servicio y
  vuelve a elegir la carpeta. Tras instalar una versión nueva puede hacer falta darlo otra vez.
  <!-- PENDIENTE-COORDINADOR: este paso viene de la corrección de la Fase F y todavía no se comprobó con una USB real;
  confirmarlo (o corregirlo) en la instalación real antes de dar la guía por definitiva. -->
- **Si la USB no está conectada** a la hora del respaldo, se guarda en la carpeta predeterminada y aparece un aviso en
  la pantalla de inicio del administrador. Si un respaldo **falla**, o si pasa más de un día sin un respaldo correcto
  («No hay un respaldo reciente de la base de datos»), el aviso dice qué pasó y sigue hasta que haya un respaldo
  correcto; corrige lo que diga y toca **«Respaldar ahora»**.
- **Cuida la USB**: quien la tenga puede leer los datos del restaurante. Guárdala en un lugar seguro.

### Restaurar un respaldo

Restaurar **no se hace desde la aplicación**: lo hace un administrador o soporte en la computadora principal, con una
ventana de Terminal. Primero se ensaya y después se restaura:

```text
"/Applications/AD-EL POS.app/Contents/MacOS/ad-el-pos-server" restore-check --file <respaldo>
"/Applications/AD-EL POS.app/Contents/MacOS/ad-el-pos-server" restore --file <respaldo>
```

El respaldo debe estar en la carpeta de respaldos de AD-EL POS (copia ahí el de la USB si hace falta). `restore`
**detiene el servidor** mientras dura, guarda antes un respaldo del estado actual y lo vuelve a arrancar; mientras
tanto las terminales muestran «Sin conexión» y después se reconectan solas. Después de restaurar, **las terminales y
los teléfonos vuelven a descargar el catálogo completo solos** (no hay que hacer nada en ellos).

**La identidad de la computadora principal no viaja en los respaldos.** Cada computadora principal tiene una identidad
propia (con la que las terminales comprueban que se conectan al servidor correcto), y por seguridad no se guarda en los
respaldos, que pueden ir a una USB. Restaurar en **la misma** computadora no cambia nada. Restaurar en **otra**
computadora (por ejemplo, porque la principal se descompuso) le da una identidad nueva: **todas las terminales tendrán
que emparejarse de nuevo** (sección 2).
