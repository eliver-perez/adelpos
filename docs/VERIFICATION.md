# Verificación · Etapa 2

Fecha: 7 de noviembre de 2026, America/Monterrey. macOS 26.5.2, Apple Silicon ARM64. La evidencia anterior se conserva en [VERIFICATION_STAGE1.md](VERIFICATION_STAGE1.md).

## Pruebas reales de Go y PostgreSQL

`./scripts/dev.sh test-integration` ejecutó las suites en bases temporales independientes `adel_pos_test_*`; ambas terminaron correctamente y sus bases se eliminaron. No se reinicializó adel_pos.

- Inicialización local, repetición sin duplicados y creación de Caja 1 sin turnos/movimientos.
- Acceso admin/admin, advertencia no bloqueante y autorización para continuar usando administración.
- Cambio de contraseña, rechazo del secreto anterior y conservación al reconstruir la instancia del servicio sobre la misma base.
- Aprovisionamiento sa con credencial aleatoria exclusivamente de prueba, rechazo de sobrescritura, acceso normal, exclusión de listados y rechazo de modificación ordinaria por UUID.
- Usuarios: creación, edición y desactivación; roles personalizados: creación/edición.
- Excepciones individuales heredadas, permitidas y denegadas; rechazo HTTP real de operaciones sin permiso. Cambiar permisos de rol se aplica a la siguiente solicitud.
- Nuevo acceso revoca la sesión anterior e informa SESSION_REPLACED; vinculación de sesión/dispositivo, rechazo de identificador manipulado, caducidad y solicitudes anónimas.
- Invitación de terminal: consumo único; revocación de dispositivo y rechazo de operaciones posteriores.
- Persistencia de preferencias, fallback de pantalla inicial no autorizada y teclado por terminal.
- Consulta autorizada de auditoría, historial conservado tras desactivar usuarios y ausencia de secretos en eventos.
- Configuración general y zona horaria guardadas; cajas creadas/editadas; asociación de dispositivo; rechazo de desactivar una caja en uso; desasociación y desactivación posterior.
- Autorización local de recuperación y revocación administrativa de sesión.
- Acciones privilegiadas de soporte registradas; límite de intentos fallidos aplicado.
- Migraciones concurrentes/repetidas, rollback de lote fallido y rechazo de checksum/historial alterado o esquema más nuevo.

Se ejecutaron además compilación Go, `go test -race ./...` y `go vet ./...`, correctamente. Las pruebas TLS abren un servidor efímero local: una CA desconocida se rechaza, una CA autorizada se acepta y un hostname/SAN distinto se rechaza. Argon2id usa sales independientes y rechaza contraseña incorrecta o formato inválido.

La migración real 0002 se aplicó a adel_pos después de generar un respaldo. Una repetición real informó `applied: 0`; la 0001 se mantuvo intacta. El respaldo se verificó con pg_restore --list; no se ensayó su restauración completa.

## Interfaz y puente nativo

Las pruebas de componentes usan jsdom y sustituyen únicamente el transporte nativo. No son pruebas contra PostgreSQL ni una segunda máquina física. Se ejecutaron la navegación con advertencia inicial no bloqueante, selección inicial editable ADEL POS/zona OS, pérdida y recuperación visual de conexión y retorno al login por sesión reemplazada. Los seis tests de diagnóstico de etapa 1 se conservaron. TypeScript estricto y bundle Vite compilaron correctamente. Se probaron también los errores de contraseña actual y la revocación de terminal detectada desde el login.

El cierre final de pruebas frontend informó **12 tests aprobados**. Las pruebas Rust verifican destinos, modos, bloqueo de rutas/identificadores arbitrarios, eliminación de tokens antes de responder al WebView y conservación de sesión ante una contraseña actual incorrecta: **5 aprobadas y una prueba OS opt-in omitida del recorrido normal**. Se ejecutó explícitamente `cargo test real_system_vault_roundtrip -- --ignored`: guardó y recuperó una credencial temporal en el **Keychain real de macOS**, comprobó su persistencia y la eliminó. No leyó ni modificó perfiles existentes.

El `.app` 0.2.0 ARM64 compiló y abrió; se observaron peticiones reales desde React → IPC → HTTP Go. La verificación operativa usa el mismo ejecutable que `dev.sh open-app`, con configuración local explícita.

## Procesos y recuperación reales

`python3 tests/runtime_smoke.py` terminó correctamente tras corregir el control de estados de salida de procesos macOS:

- PostgreSQL detenido: live=200 y ready=503; PostgreSQL reiniciado: ready=200 con el mismo Go.
- Tauri cerrado por salida normal de macOS: proceso nativo terminado, **Go PID 89097 conservado** y ready=200.
- `.app` reabierto: consultas contra el mismo servidor.
- Go detenido ordenadamente: Tauri permaneció abierto. Go reiniciado: el mismo escritorio retomó consultas.
- Modo server-only: segundo Go temporal en 127.0.0.1:8182, disponibilidad real y SIGTERM correcto.

`python3 tests/runtime_smoke.py --terminal-only` también pasó: el mismo `.app` usó configuración externa sin iniciar Go adicional ni otra DB; el PID del servidor se conservó.

El primer intento detectó un fallo del script al inspeccionar comm durante la salida: Go ya había terminado, pero se informaba erróneamente un PID ajeno. Se corrigió sin retirar la comprobación de propiedad anterior a SIGTERM; la repetición completa pasó. No se enviaron señales a procesos ajenos.

## Comprobación final de la instalación real

La aplicación final quedó abierta y consultando Go, PID 89161, con readiness ready, PostgreSQL available y esquema compatible. Una consulta de solo lectura confirmó que el establecimiento real ya está configurado como **ADEL POS**, moneda **MXN**, zona **America/Monterrey**, que existe admin y que mantiene una sesión activa. No se imprimieron secretos. La identidad sa existe, pero su contraseña real sigue sin aprovisionarse; ese paso requiere la elección privada del propietario.

## Límites y pasos del propietario

- En la base real, la primera configuración y la contraseña privada sa pertenecen al propietario. El asistente propone ADEL POS y la zona OS. No se inventó una contraseña sa ni se trasladaron datos de prueba a adel_pos.
- La aceptación sa se verificó en la base temporal. La activación real requiere `./scripts/dev.sh provision-sa` con entrada local oculta del propietario; la identidad reservada sin credencial ya existe.
- No se automatizó un recorrido por todos los controles de la ventana nativa ni inspección por captura de pantalla. La captura de ventana no estuvo disponible en esta sesión. Se distinguen las pruebas de componentes de la apertura/peticiones reales de Tauri.
- No se probó una segunda máquina física LAN, Windows, Mac Intel, caída de energía, carga de muchas terminales ni restauración completa de backup.
- El teclado flotante no está implementado; sí su preferencia persistente. No hay WebSocket activo: las notificaciones de sesión se consultan cada cinco segundos.
- No hay firma/notarización comercial, instalador Windows, servicio automático instalado ni proveedor comercial de licencias.

## Comandos reproducibles

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh build-macos
# Sesión gráfica, solo servicios de desarrollo propios, sin trabajo real en curso:
python3 tests/runtime_smoke.py
python3 tests/runtime_smoke.py --terminal-only
# Prueba opt-in del almacén seguro real:
cd desktop/src-tauri
cargo test real_system_vault_roundtrip -- --ignored
```

Para invocar Cargo directamente en este Mac, cargar previamente el entorno con `export PROJECT_ROOT="$PWD"; source scripts/env.sh` desde la raíz. Los scripts no requieren copiar secretos a argumentos o logs. La evidencia operativa queda en `.local/runtime-verification.json` y `.local/terminal-mode-verification.json`; no versionar el directorio privado.
