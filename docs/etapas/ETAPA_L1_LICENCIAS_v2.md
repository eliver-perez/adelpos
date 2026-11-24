# AD-EL POS — ETAPA L1: INTEGRACIÓN CON EL SERVIDOR DE LICENCIAS

Se ejecuta **en la Mac**, sobre `main`.

- **Parte de:** servidor 0.14.0 (esquema 27) y aplicación Android 0.3.0. Confirma los valores reales en `config/product.json`, `CLAUDE.md` y `android/CLAUDE.md`.
- **Al terminar:** servidor **0.15.0** y aplicación Android **0.4.0** (versionCode 4).

**Contenido:** el servidor Go del restaurante se activa con una licencia firmada por el servidor de licencias (`licensing.sicrat.com`, entorno `test`), aplica los estados y los cupos de la licencia en el backend, y protege la hora del equipo. Esta etapa **solo trabaja con el entorno de prueba**: las llaves y el servidor de producción todavía no existen.

**El contrato manda.** Las reglas están en el contrato de licencias V1.0 definitivo. Este documento solo agrega lo que el contrato deja a AD-EL POS (§5 a §7 de este documento) y la forma de trabajo. Si algo de aquí contradice al contrato, manda el contrato: anótalo en el avance y sigue con el contrato.

---

## 0. FORMA DE TRABAJO

### Rama y especificaciones

- Rama **`etapa-l1`** desde `main`. Las especificaciones en `docs/etapas/` son de solo lectura.
- Lee este documento, `CLAUDE.md`, `android/CLAUDE.md` y `docs/API_V1.md`.
- **Repositorio del servidor de licencias:** `https://github.com/eliver-perez/adelpos-license-server`, del mismo propietario. Clónalo **fuera** del repositorio de AD-EL POS (por ejemplo, en `~/src/adelpos-license-server`), en la etiqueta **`v1.0.0`**. De ahí salen:
  - el contrato (`CONTRATO_LICENCIAS_ADEL_POS.md`) y la guía de integración;
  - la carpeta `contracts/v1/` (OpenAPI, esquema, casos de prueba y verificador de referencia).
- **Lee del contrato** las §1 a §10, §12 y §13, y la guía completa. Las §5.1 a §5.5 y la §11 son del servidor de licencias: no se recalculan aquí.

### Eficiencia

- La Fase 0 la hace **un solo agente**.
- **Máximo 2 agentes en total trabajando al mismo tiempo. Los subagentes no pueden lanzar otros subagentes.**
- En cada fase, solo las pruebas de lo que cambió. La regresión completa, **una sola vez**, en la Fase H. Si una corrección de la regresión toca `backend/`, `config/` o código compartido, repite la regresión completa; si no, solo las suites afectadas.
- **Revisor independiente** (un agente que no escribió el código) **solo** para: la verificación de licencias (§10 del contrato), la asignación atómica de cupos y la protección de la hora. Son las partes de seguridad.
- **Android en dos emuladores durante las fases** (`Pixel_Tablet` y `ADEL_A1_Tablet_API26`); los cuatro solo si el propietario lo pide antes de una entrega.
- **Capturas de pantalla:** solo de las pantallas nuevas o cambiadas, en tema claro.

### Autonomía

Aplica la regla de autonomía de `CLAUDE.md` §0 y la protección contra el servicio real de la A1 a la A3.

- **El propietario autoriza** usar `licensing.sicrat.com` (entorno `test`) para las pruebas de la Fase G, con las tres licencias de prueba, y solo las pruebas que dice esa fase.
- **Los datos de la Mac son de prueba:** no hay que conservarlos por la licencia. Aun así, la regla de no borrar datos de operación sin preguntar sigue igual.
- **Paradas:**
  - Instalar el `.pkg` (contraseña de administrador).
  - Cualquier prueba que necesite el panel del servidor de licencias: la hace el propietario (Fase G, lista final).
  - Si el servidor de licencias no cumple el contrato o el contrato no alcanza: anótalo como defecto o como propuesta para la V1.1 y sigue con lo demás. **Nunca cambies el contrato ni los archivos de `contracts/v1/`.**

### Secretos

- **Códigos de licencia de prueba:** están en `~/.adel-pos-dev/licencias-prueba.txt`, fuera del repositorio, con una línea por licencia (`perpetua=…`, `mensual=…`, `prueba=…`). Si el archivo no existe, para y pídeselo al propietario.
  - **Nunca** se escriben en el repositorio, en commits, en el avance, en registros (`logs`), en mensajes de error ni en capturas. En el avance se nombran como «la Perpetua», «la Mensual» y «la Prueba».
  - La aplicación tampoco los escribe nunca en sus registros, como pide la §12 del contrato.
- **Llaves privadas** (la de licencias de cada instalación y la de soporte de §6.4): nunca en el repositorio ni en los respaldos.
- **Una versión de producción nunca confía en las llaves `test`** ni en las de los archivos de prueba, ni apunta a `licensing.sicrat.com`.

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación. |
| **A** | Núcleo: llave de licencias, verificador y estado (§1, §2). |
| **B** | Cliente en línea y sin internet (§3). |
| **C** | Estados en el backend y protección de la hora (§4, §6). |
| **D** | Cupos (§5). |
| **E** | Interfaz de escritorio (§7). |
| **F** | Android (§8). |
| **G** | Pruebas contra `licensing.sicrat.com` (§9). |
| **H** | Regresión completa, documentación, APK, `.pkg`, fusión y push (§10). |

**Al terminar cada fase:** ejecuta sus pruebas, registra un resumen en `docs/ETAPA_L1_AVANCE.md`, haz un commit y continúa.

### Fase 0: verificación

1. Clona el repositorio de licencias en `v1.0.0` y comprueba el SHA-256 del contrato: debe ser `e3435f16e27e11c85ac263ef2accfa72aad07cf30dbcdee0a0b8b22326314058`. Si no coincide, para y avisa.
   - Existe un archivo con encabezado «V1.1» y el mismo contenido. **Usa la V1.0 definitiva de la etiqueta.**
2. Copia `contracts/v1/` **sin cambios** a `contracts/licencias/v1/` en AD-EL POS. Copia también el contrato y la guía a `docs/licencias/`. Agrega `contracts/licencias/ORIGEN.md` con el repositorio, la etiqueta, el commit y los SHA-256.
3. Ejecuta el verificador de referencia con todos los casos (comando de la §2 de la guía). Todos deben pasar.
4. Revisa en el código actual:
   - `identity/` y cómo se crea la identidad (`server_id`, certificado ECDSA);
   - `sessions-summary` y cómo se cuentan hoy las sesiones;
   - los turnos de caja, el modo Cocina y cómo abre sesión cada tipo de terminal;
   - qué incluyen los respaldos.
5. Anota en el avance el plan de cada fase y cualquier duda del contrato.

---

## 1. LLAVE DE LICENCIAS Y LLAVES DE CONFIANZA

### 1.1 Llave de licencias de la instalación

- El servidor Go genera **una sola vez** una llave **Ed25519** de licencias en `identity/`, aparte del certificado ECDSA de la red local (§2 del contrato). Permisos de solo dueño.
- **No va en los respaldos.** Un respaldo restaurado en otra computadora queda «Sin activar con datos» y se activa (o se transfiere) ahí. Es el camino que prevé el contrato.
- **Huella del equipo:** junto con la licencia aceptada se guarda una huella del equipo (en Mac, el `IOPlatformUUID`; en Windows, `MachineGuid`, con su prueba de lectura, aunque se pruebe en la Fase W).
  - Si la carpeta de datos aparece en otro equipo (huella distinta), la licencia guardada se ignora y el estado es «Sin activar con datos», con el mensaje «Esta instalación se copió de otra computadora. Actívala o transfiere la licencia.».
  - La identidad nueva se regenera solo si el propietario lo confirma desde esa pantalla; documenta el comportamiento exacto en la Fase 0.

### 1.2 Llaves de confianza por entorno

- El entorno se fija **al compilar**: `development` (confía en `test`) o `production` (confía en las de producción, que todavía no existen).
  - Una compilación de producción sin llaves de producción **falla al compilar**: no puede salir un instalador de producción por accidente.
  - Las pruebas automáticas usan **solo** las `signing_keys` de `vectors.json`.
- **Llaves del entorno `test`** (públicas), exportadas por el propietario:

  ```json
  {
      "environment": "test",
      "keys": [
          {
              "kid": "adelpos-test-202611-1",
              "environment": "test",
              "public_key": "yxcFqz1rSQYi03-2slIpjx82xRhKkX_AtbguaV2zZ00"
          }
      ]
  }
  ```

  - Guárdalas como `config/licencias/llaves-test.json` y guarda **todas** las de la lista, asociadas a su `kid`.
  - Documenta en `docs/licencias/ROTAR_LLAVES.md` el procedimiento de rotación de la §3 de la guía.
- **Dirección base** configurable; en desarrollo, `https://licensing.sicrat.com`.

---

## 2. VERIFICADOR Y ESTADO

- Implementa la **§10 del contrato** en el backend, partiendo del verificador de referencia. Encabezado exacto, `kid` de confianza del entorno, firma, contenido según la §4, destinatario, revisión mayor que la más alta aceptada, y guardado persistente.
- **Pasa todos los casos de `vectors.json`** con una prueba que los recorre todos, igual que el verificador de referencia. Si un caso no coincide, el error está en la integración; los archivos no se editan.
- **Estado:**
  1. Sin licencia válida aceptada: «Sin activar», o «Sin activar con datos» si la base tiene operaciones.
  2. Con `license_status: revoked`: «Revocada», con el mensaje según `status_reason`.
  3. Si no, la §5.6: «Vigente», «Tolerancia» o «Vencida», con `ahora_efectivo = máximo(reloj_local, última_hora_confiable)`.
- Un archivo alterado, de una llave no confiable o de otra instalación **se ignora** y se sigue con la última licencia válida.

---

## 3. CLIENTE EN LÍNEA Y SIN INTERNET

- **En línea** (§8 del contrato): desafío, prueba de posesión, y las operaciones activar, transferir, consultar actualización y liberar.
  - Los reintentos usan el mismo cuerpo y el mismo `request_id`.
  - Se decide por `error.code`, nunca por el HTTP, con textos propios en español para cada código.
  - `TEMPORARY_UNAVAILABLE` y los errores de red nunca afectan una licencia válida ya instalada.
  - Se respeta `Retry-After`.
- **Consulta en segundo plano:** al arrancar y una vez al día, espaciada y sin bloquear nada. Cada `server_time` recibido actualiza la última hora confiable.
- **Transferencia:** solo cuando activar responde `ACTIVATION_LIMIT` con `transfer_available: true` **y** el usuario la confirma. `TRANSFER_LIMIT` muestra «Comunícate con soporte».
- **Sin internet** (§9): generar `.licreq` de `activate` (con el código, opcional), `renew` y `deactivate`, guardarlo como archivo, e importar un `.lic` que se verifica igual que una respuesta en línea.
- **Solo el servidor Go principal** habla con el servidor de licencias. Las terminales y Android solo muestran el estado.

---

## 4. ESTADOS EN EL BACKEND

- Aplica **la tabla de operaciones de la §6 del contrato en el backend, en cada operación protegida**, para escritorio, API v1 y Android. Las terminales solo muestran el estado.
- **Reglas firmes del contrato, sin excepciones:** ningún estado borra datos, impide cobrar una cuenta ya abierta en una instalación activada ni bloquea reportes, historial o respaldos.
- **Activar, transferir, renovar e importar un `.lic`** están permitidos en todos los estados.
- **Errores de la API:** un código de error estable para cada caso (licencia vencida, revocada, sin activar, sin cupo, hora incorrecta), para que escritorio y Android muestren el mensaje correcto. Documéntalos en `docs/API_V1.md`.
- **Permiso nuevo «Administrar la licencia»**, solo en el rol Admin de fábrica. Ver el estado de la licencia no requiere permiso especial.

---

## 5. CUPOS

Aplica la §7 del contrato. Reemplaza el conteo actual de `sessions-summary` (por tipo de dispositivo) por el de **lo que hace cada terminal**.

| Cupo | Lo ocupa | Se libera |
| --- | --- | --- |
| `cash_registers` | Un turno de caja abierto en una terminal | Al cerrar el turno, también desde otra computadora |
| `order_takers` | Una **terminal** que captura pedidos sin turno de caja abierto | Al cerrar sesión el último usuario de esa terminal, al abrir turno de caja en ella, o tras **15 minutos** sin actividad de captura en esa terminal |
| `kitchen_displays` | Una terminal en modo Cocina | Al salir del modo o cerrar la sesión |

- **Cupo de comandero por terminal, no por usuario.** Una tableta compartida donde entran varios meseros con su PIN ocupa **un** cupo, igual que cambiar de cajero no consume otra caja. Volver a «Selecciona tu usuario» a los 2 minutos **no** libera el cupo.
- **Cuándo se ocupa:** al primer intento de crear o modificar un pedido sin turno de caja. Una terminal que solo consulta no ocupa cupo.
- **Actividad** para los 15 minutos: crear o modificar pedidos, abrir cuentas o entrar a una mesa para capturar. El valor queda en la configuración del backend, no en la interfaz.
- **Asignación atómica** en PostgreSQL (bloqueo de fila o *advisory lock* dentro de la transacción). Una prueba de concurrencia: varias terminales a la vez por el último cupo, y solo una lo obtiene.
- **Licencia nueva con menos cupos:** no interrumpe a nadie; solo impide ocupar cupos nuevos hasta quedar dentro del límite.
- **Sin cupo:** el backend rechaza con su código. El mensaje dice qué cupo falta y cuántos usa la licencia, por ejemplo «No hay cupo de comandero libre (3 de 3 en uso). Pide que se cierre la sesión de otra terminal.».
- **Terminales (escritorio):** muestra qué cupo ocupa cada terminal y permite al administrador liberar un cupo de comandero o de cocina (cierra la sesión de esa terminal). Las cajas se liberan cerrando el turno, como ya existe.
- **Plan básico** (1 caja, 0 comanderos, 0 KDS): solo se captura en la terminal con turno de caja abierto. Ni tabletas ni modo Cocina. No hay excepción: los cupos salen de la licencia.

---

## 6. PROTECCIÓN DE LA HORA

### 6.1 Regla

- **La hora del equipo no puede estar antes de la última actividad del sistema.** La «última hora confiable» de la §5.6 se guarda de forma persistente y se alimenta de:
  - el `server_time` de las respuestas del servidor de licencias;
  - el `issued_at` de las licencias aceptadas;
  - la hora de la última operación registrada (pedidos, cobros, turnos, movimientos de inventario) y la hora local mientras el sistema funciona, al menos una vez por minuto.
- Se revisa al arrancar, cada minuto y en cada operación protegida, **solo en el servidor Go principal**: las terminales y Android usan la hora del servidor.
- **Margen:** 10 minutos, por ajustes normales del reloj y el reposo del equipo. Documenta el valor.

### 6.2 Bloqueo

Si `reloj_local < última_hora_confiable − margen`, el sistema entra en **«Hora del equipo incorrecta»**:

- **Se bloquean todas las operaciones que registran algo con fecha:** capturar, cobrar, cancelar, abrir y cerrar turno, movimientos de inventario. Es una excepción deliberada a la regla de cobro: un cobro con fecha atrasada descuadra los cortes.
- **Siguen disponibles:** reportes, historial, respaldos y la pantalla de licencia.
- **Si el reloj se corrige** y queda dentro del margen, el bloqueo se quita solo.

**Debe quedar claro por qué se bloqueó.** El usuario nunca debe confundir este bloqueo con un problema de licencia, de conexión o de permisos.

- **Pantalla de bloqueo** en la computadora principal, con título **«Sistema detenido por la fecha y hora de la computadora»** y estas partes, en lenguaje sencillo:
  1. **Qué pasó:** «La fecha y hora de esta computadora es **lunes 23 nov 2026, 10:15**. La última actividad registrada en el sistema fue el **lunes 23 nov 2026, 12:40**. La hora de la computadora no puede ser anterior a lo que ya se registró.».
  2. **Por qué se detiene:** «Para proteger tus cortes y reportes: si se registran ventas o cobros con una fecha atrasada, quedarían fuera de orden y los cortes no cuadrarían.».
  3. **Qué sí puedes hacer:** consultar reportes e historial y hacer respaldos.
  4. **Cómo resolverlo:**
     - «Corrige la fecha y hora de la computadora». En Mac y Windows, con una indicación breve de dónde se cambia.
     - «Comprobar la hora con internet» (§6.3).
     - «Si la hora de la computadora es correcta y el problema sigue, comunícate con soporte»: abre el desbloqueo de soporte (§6.4).
  5. **Aclaración:** «Tu licencia está bien. Este bloqueo es solo por la fecha y hora.». Se muestra solo si la licencia está vigente o en tolerancia. Si también hay un problema de licencia, se muestran los dos por separado.
- Las fechas van completas (día de la semana, día, mes, año y hora) en la zona horaria del restaurante.
- **Terminales de escritorio y Android:** cuando el servidor rechaza una operación por la hora, el mensaje lo dice explícitamente: «No se puede registrar: la computadora principal tiene la fecha y hora incorrectas. Avisa al encargado.». Nunca un error genérico.
- **Indicador:** mientras dure el bloqueo, todas las terminales muestran un aviso fijo con la misma causa.
- **Auditoría:** el motivo del bloqueo con las dos fechas.
- Cada bloqueo y cada desbloqueo quedan en la bitácora de auditoría.

### 6.3 Corrección con internet

El caso difícil: el equipo tuvo una falla, la fecha saltó al futuro, se registró actividad con esa fecha y luego la hora se corrigió. Ahora la hora real queda «antes» de la última actividad.

- Botón **«Comprobar la hora con internet»**: consulta el servidor de licencias y compara su `server_time` con el reloj local.
  - **Si coinciden** dentro del margen, el reloj está bien y la última hora confiable estaba en el futuro. Se reemplaza por `server_time` (es el único caso en que baja) y se registra en la auditoría con los dos valores.
  - **Si no coinciden,** muestra la hora del servidor y pide corregir el reloj a esa hora.
- Las operaciones registradas con fecha futura **no se modifican**. Se listan en la auditoría para que el propietario decida.

### 6.4 Desbloqueo de soporte, sin internet

Para que el propietario de AD-EL POS desbloquee una computadora sin internet:

- La pantalla de bloqueo muestra un **código de solicitud**: corto, legible por teléfono, ligado al `server_id`, a un número de un solo uso y a la última hora confiable.
- El propietario lo procesa con una herramienta de línea de comandos del repositorio (`tools/soporte-desbloqueo`). La herramienta firma una **respuesta** con la llave privada de soporte: `server_id`, el número de la solicitud y la nueva última hora confiable (por omisión, la hora actual del propietario).
- La respuesta se pega como texto (por ejemplo, por WhatsApp) o se importa como archivo. Se acepta **una sola vez**, solo en esa instalación y dentro de las 24 horas de la solicitud.
- **Llaves de soporte:**
  - Una por entorno: la de desarrollo la genera esta etapa en `~/.adel-pos-dev/soporte/`, fuera del repositorio. La de producción se genera con el paso a producción.
  - La pública se incluye al compilar; la privada nunca va en el repositorio ni en el instalador.
- Documenta el procedimiento en `docs/licencias/DESBLOQUEO_SOPORTE.md`. Anota en el avance como **propuesta para la V1.1** pasar este desbloqueo al panel del servidor de licencias.

---

## 7. INTERFAZ DE ESCRITORIO

- **Primer arranque sin licencia** («Sin activar» o «Sin activar con datos»): una pantalla de activación con el código de la licencia, el formato del código validado al escribirlo, y la opción «Activar sin internet» (`.licreq` y `.lic`).
- **Configuración › Licencia:**
  - Estado, plan, cliente, vencimiento (o «Perpetua»), días de tolerancia y cupos (en uso / límite) de cajas, comanderos y pantallas de cocina.
  - Botones: «Buscar actualización», «Transferir a esta computadora» (solo cuando aplica), «Liberar esta licencia» (con confirmación), «Solicitud sin internet», «Importar archivo de licencia».
  - El entorno se muestra solo en esta pantalla, en versiones de desarrollo: «Servidor de licencias de pruebas».
- **Avisos:**
  - **7 días antes** del vencimiento (suscripciones y pruebas): un aviso al iniciar sesión, una vez al día, para usuarios con permiso de administrar la licencia; un indicador discreto para los demás.
  - **En tolerancia:** un aviso diario visible para todos, con los días que quedan antes de que se bloquee la captura.
  - **Vencida o revocada:** un aviso fijo que explica qué se puede hacer (cobrar lo abierto, cortes, reportes) y qué no.
- **Bloqueo por hora:** la pantalla de §6 con «Comprobar la hora con internet» y «Desbloqueo de soporte».
- **Sin cupo:** el mensaje de §5 en el momento de la operación, no un error genérico.
- **Regla de textos:** ningún texto provisional o de desarrollo; los mensajes salen de los códigos de error, nunca del `message` del servidor de licencias.

---

## 8. ANDROID (0.4.0)

- El estado de la licencia llega por la API v1. Android **no** habla con el servidor de licencias.
- **Mensajes claros** con los códigos de error de §4:
  - Sin cupo de comandero: el mensaje de §5.
  - Licencia vencida, revocada o sin activar: «La licencia del sistema no permite capturar pedidos. Avisa al encargado.».
  - Hora incorrecta: «No se puede registrar: la computadora principal tiene la fecha y hora incorrectas. Avisa al encargado.». Con el aviso fijo de §6.2 mientras dure.
- **Cola sin conexión:** un pedido en cola que el servidor rechaza por licencia, cupo u hora **no se pierde ni se reintenta sin fin**. Queda marcado con el motivo y el mesero lo ve, igual que los rechazos que ya existen.
- Un aviso discreto en la barra cuando la licencia está en tolerancia.
- APK **0.4.0** con la llave de desarrollo de siempre.

---

## 9. PRUEBAS

### 9.1 Automáticas (sin internet, con las llaves de los archivos de prueba)

- Todos los casos de `vectors.json`, incluidos los estados temporales con sus límites exactos y el reloj atrasado.
- La tabla de operaciones de la §6 del contrato, estado por estado, en el backend y por la API v1.
- Cupos: ocupar, liberar por cada causa (incluidos los 15 minutos con reloj simulado), la prueba de concurrencia, la tableta compartida con varios PIN y la licencia nueva con menos cupos.
- Hora: bloqueo con su pantalla y su causa explicada (las dos fechas, el porqué y la aclaración de la licencia), el mensaje en terminales y Android, desbloqueo al corregir el reloj, corrección con `server_time` simulado, desbloqueo de soporte (válido, repetido, de otra instalación, vencido, alterado).
- Huella del equipo distinta y respaldo restaurado: «Sin activar con datos».
- Una compilación de producción sin llaves de producción falla.
- **Ninguna prueba automática llama a `licensing.sicrat.com`.** Una prueba lo comprueba, igual que la protección contra el servicio real.

### 9.2 Contra `licensing.sicrat.com` (Fase G)

- Corre con instancias de desarrollo **separadas del servicio instalado**, cada una con su carpeta de datos, su base temporal y su propia identidad. Lee los códigos de `~/.adel-pos-dev/licencias-prueba.txt`.
- Cada licencia es de un **plan distinto**. Anota en el avance el plan y los cupos que trajo cada una, y prueba los cupos reales de cada plan (en especial, el plan con 0 comanderos).
- **Qué licencia para qué:**
  - **La Prueba:** las pruebas 1 a 6 y 9 de la §7 de la guía (activar, repetir, otra instalación, transferir, transferir otra vez y confirmar, liberar y activar en otra). Es la única que se transfiere.
  - **La Mensual:** activar, consulta de actualización y su vencimiento informado; `.licreq` de `renew` generado y guardado.
  - **La Perpetua:** activar y liberar. **Nunca se transfiere.**
  - Las pruebas 11 a 14 con cualquiera: código inexistente, reintento y `REQUEST_CONFLICT`, `.lic` alterado o de otra instalación, y reloj atrasado (con reloj simulado, nunca cambiando la hora de la Mac).
- **Al terminar, libera las tres** para que queden disponibles. Anota en el avance qué quedó pendiente en el panel (por ejemplo, una transferencia por confirmar).
- **Lista para el propietario** en el avance y en la entrega final, con pasos concretos:
  - Prueba 7: renovar o cambiar de plan en el panel y pulsar «Buscar actualización».
  - Prueba 8: revocar en el panel y consultar (con la Prueba, al final).
  - Prueba 10: aprobar en **Sin internet** los `.licreq` que dejaste en `~/.adel-pos-dev/licreq/` e importar cada `.lic`.

### 9.3 Interfaz

- Playwright: activación, pantalla de licencia, avisos de vencimiento y tolerancia, bloqueo por hora y mensajes de sin cupo.
- Android en los dos emuladores: los mensajes de §8 y un pedido en cola rechazado por cupo.

---

## 10. DOCUMENTACIÓN Y ENTREGA

- **Documentación:**
  - `docs/licencias/INTEGRACION.md`: cómo está hecho, qué decide AD-EL POS (15 minutos, 7 días, hora, huella, desbloqueo) y los códigos de error.
  - `docs/licencias/PASO_A_PRODUCCION.md`: qué falta para la versión del restaurante (servidor de licencias de producción, sus llaves, la llave de soporte de producción y la compilación de producción).
  - `docs/licencias/ROTAR_LLAVES.md` y `docs/licencias/DESBLOQUEO_SOPORTE.md`.
  - Actualiza `CLAUDE.md`, `android/CLAUDE.md`, `docs/API_V1.md` y `docs/ANDROID_INSTALAR.md` (activar la computadora principal antes de emparejar).
- **Entregables:**
  - APK **0.4.0**.
  - `.pkg` **0.15.0** (desarrollo, confía en `test`), con su parada para instalar.
- Fusiona en `main` y haz push.
- **Después de instalar el `.pkg`,** la Mac queda «Sin activar con datos». **No la actives tú:** el propietario la activa con la Perpetua desde Configuración › Licencia como prueba final. Deja los pasos en la entrega.
- **Entrega final breve:**
  - Cambios por fase.
  - Plan y cupos de cada licencia de prueba.
  - Resultado de cada prueba de la §7 de la guía, y la lista para el propietario.
  - Defectos del servidor de licencias y propuestas para la V1.1, si las hubo.
  - Limitaciones.
