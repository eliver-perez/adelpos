# Instalar AD-EL POS en Windows

Guía corta para instalar la versión de prueba de AD-EL POS en una computadora con **Windows 10 u 11 de 64 bits**.
Es un instalador de prueba para la presentación: no está firmado ni tiene licencias.

## 1. Dónde está el instalador

En la computadora donde se preparó, el instalador está en:

```text
C:\Users\Expediente\Documents\ADEL POS\desktop\src-tauri\target\release\bundle\nsis\AD-EL POS_0.9.4_x64-setup.exe
```

Es un solo archivo. Para llevarlo a otra computadora, cópialo a una memoria USB (o envíalo por una carpeta
compartida) y ábrelo desde ahí en la otra computadora. No necesita Internet: incluye todo lo que usa (también
la base de datos), y si la computadora no tiene el componente WebView2 de Microsoft, lo instala él mismo.

## 2. Cómo instalarlo

1. Haz doble clic en `AD-EL POS_0.9.4_x64-setup.exe`.
2. Windows mostrará **«Windows protegió tu PC»** porque el instalador no está firmado. Es normal en esta versión
   de prueba: toca **«Más información»** y luego **«Ejecutar de todas formas»**.
3. Sigue el asistente: carpeta del menú Inicio (deja «AD-EL POS») y, al final, marca si quieres un acceso
   directo en el escritorio. No pide contraseña de administrador: se instala solo para el usuario actual.
4. Abre **AD-EL POS** desde el menú Inicio. La primera vez tarda hasta un minuto en preparar la base de datos;
   la pantalla dice que se está preparando y continúa sola.
5. Sigue el asistente inicial (nombre del restaurante, zona horaria y nombre de la terminal). Después entra con
   el usuario **admin** y la contraseña **admin**, y cámbiala en «Cambiar contraseña».

## 3. Si Windows pide permiso de red

No debería aparecer: en esta versión AD-EL POS solo se comunica dentro de la misma computadora y no abre
conexiones hacia la red. Si alguna vez aparece una ventana de **«Firewall de Windows Defender»** que pregunta si
permites que «ad-el-pos-server» o «postgres» se comuniquen en redes, responde **«Cancelar»** (o deja
desmarcadas las casillas): la aplicación funciona igual. La conexión de varias terminales por red local todavía
no está preparada en Windows.

## 4. Dónde quedan los datos y cómo hacer un respaldo

- Los datos del restaurante (ventas, cobros, cortes, catálogo, configuración) **no** están en la carpeta del
  programa. Están en la carpeta de datos del usuario: escribe `%LOCALAPPDATA%\AD-EL POS` en la barra de
  direcciones del Explorador de archivos. El programa está aparte, en `%LOCALAPPDATA%\Programs\AD-EL POS`.
- **Crear un respaldo**: en el menú Inicio, carpeta «AD-EL POS», abre **«Crear respaldo de AD-EL POS»**. Se
  abre una ventana que crea el respaldo, lo comprueba y muestra su nombre; presiona Entrar para cerrarla. Puede
  hacerse con la aplicación abierta o cerrada.
- **Ver los respaldos**: en la misma carpeta del menú Inicio, abre **«Respaldos de AD-EL POS»**. Copia los
  archivos `.dump` a una memoria USB u otra computadora para guardarlos fuera del equipo.
- Restaurar un respaldo reemplaza todos los datos actuales: pide ayuda a soporte para hacerlo.

## 5. Cómo desinstalar sin perder datos

1. Cierra AD-EL POS.
2. Abre **Configuración → Aplicaciones → Aplicaciones instaladas**, busca **AD-EL POS** y elige
   **Desinstalar**.
3. Deja **desmarcada** la casilla que ofrece borrar los datos de la aplicación. Aun si la marcas, esa casilla
   solo borra preferencias de la ventana, no los datos del restaurante.

La carpeta `%LOCALAPPDATA%\AD-EL POS` (datos y respaldos) se conserva. Si vuelves a instalar AD-EL POS, o
instalas una versión nueva encima, la aplicación encuentra esos datos y sigue con ellos.

## 6. Qué debes probar a mano

Esto no se pudo comprobar automáticamente en la computadora de pruebas:

- [ ] **Pantalla táctil**: tocar mesas, productos y el teclado del cobro; arrastrar el mapa de mesas con el dedo.
- [ ] **Impresora de red** (ESC/POS por red): registrarla en Cocina → Impresoras, imprimir una prueba, una
      comanda y un ticket, y revisar el papel, los acentos, el logotipo y el corte.
- [ ] **Impresora USB** instalada en Windows: elegirla en Cocina → Impresoras («Impresora instalada en
      Windows»), imprimir una prueba y revisar el papel.
- [ ] **Cajón de dinero** conectado a la impresora (por red o USB): «Probar apertura» en Caja → Configuración.
- [ ] **Pantalla completa**: entrar y salir con el botón de la barra y con F11; cerrar en modo ventana y volver
      a abrir (debe recordar tamaño y posición).
- [ ] **Dos terminales**, si aplica: todavía no está preparado en Windows (ver punto 3).
