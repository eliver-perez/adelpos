# Verificación · Etapa 9G

Estado al **14 de noviembre de 2026**. Versión **0.10.0** (se fija en la Fase G), API v1, esquema **23** (migración
`0023_network_terminals_and_android.sql`, con las secciones D, B, F y E; etapa 9G: servidor del restaurante, terminales
en red y preparación para Android). Evidencia detallada: [VERIFICATION_STAGE9G.md](VERIFICATION_STAGE9G.md). Evidencia
anterior: [etapa 9F](VERIFICATION_STAGE9F.md), [etapa 9E](VERIFICATION_STAGE9E.md), [etapa 9D](VERIFICATION_STAGE9D.md),
[etapa 9C](VERIFICATION_STAGE9C.md), [etapa 9B](VERIFICATION_STAGE9B.md), [etapa 9A](VERIFICATION_STAGE9A.md),
[etapa 8](VERIFICATION_STAGE8.md), [etapa 7.1](VERIFICATION_STAGE7_1.md), [etapa 7](VERIFICATION_STAGE7.md),
[etapa 6.2](VERIFICATION_STAGE6_2.md), [etapa 6.1](VERIFICATION_STAGE6_1.md), [etapa 6](VERIFICATION_STAGE6.md),
[etapa 5](VERIFICATION_STAGE5.md), [etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md),
[etapa 2](VERIFICATION_STAGE2.md) y [etapa 1](VERIFICATION_STAGE1.md). Avance: [ETAPA_9G_AVANCE.md](ETAPA_9G_AVANCE.md).
Guía para el propietario: [INSTALAR_EN_RED.md](INSTALAR_EN_RED.md). Contrato para Android: [API_V1.md](API_V1.md).
Windows: [NOTAS_WINDOWS.md](NOTAS_WINDOWS.md) («Etapa 9G: qué hacer en la Fase W»).

## Estado

- **Código**: fases 0, A, D, B, F, C y E y las correcciones de las tres partes de la revisión independiente (fases B, D y
  F), cada una en su rama y fusionadas por el coordinador en `etapa-9g` (creada desde `main`, 0.9.5 con la 9F), con sus
  pruebas según el avance; fase G con la regresión final, la documentación, la instalación y la fusión en `main`.
  <!-- PENDIENTE-COORDINADOR: confirmar la fusión de las correcciones de D (e9d35fc) y el cierre de las fusiones de E y
  de las correcciones de F. -->
- **macOS**: todo se probó en la Mac en copias aisladas (clúster PostgreSQL temporal, Go temporal en modo de pruebas,
  carpetas «del sistema» temporales). Nada se instaló: el `.pkg` solo se compiló y se examinó.
- **Instalación real**: <!-- PENDIENTE-COORDINADOR: resultado de la instalación del .pkg 0.10.0, el paso de datos y
  la 0023 en la instalación real (Fase G, con el propietario). --> pendiente al escribir este documento; hasta entonces
  la instalación real sigue siendo la 0.9.5 de `.local` con `dev.sh`.
- **Windows**: nada de la 9G se ejecutó en Windows; solo se compiló y se revisó Go con `GOOS=windows`. El servicio de
  Windows, el instalador para todo el equipo y las pruebas cruzadas son la Fase W.

## Qué está verificado

En la Mac, con Go (`-race`), PostgreSQL temporal, Vitest, Rust y Playwright (Chromium y WebKit) contra un Go temporal en
modo de pruebas, por fase (detalle y conteos en [VERIFICATION_STAGE9G.md](VERIFICATION_STAGE9G.md)):

- Servicio y modalidades: el `host` de macOS como proceso real (habilitación, bloqueo, clúster propio, respaldo antes de
  migrar, cierre ordenado, restauración con el servicio en marcha, paso de datos `adopt-legacy` con conteos idénticos),
  las tres modalidades en Rust, React y Playwright y los scripts del instalador con destino falso; `.pkg` compilado y
  desarmado.
- Emparejamiento y cifrado: código correcto e incorrecto, vencido, 5 intentos, tope global y por grupo de origen,
  revocación inmediata (también del SSE por HTTPS), certificado cambiado (la terminal no se conecta), aprobación solo
  por la escucha local, escucha de red con filtro de orígenes y TLS ≥ 1.2, verificador de Rust con huella fijada y
  conteo de sesiones por tipo.
- Descubrimiento y red: anuncio real con el Bonjour del sistema y búsqueda de una sola vez (en la misma Mac), cambio de
  dirección con la huella fijada, capa «Sin conexión con el servidor» y dos terminales en la misma cuenta, con turnos,
  arqueo y tope por caja.
- API v1: contrato OpenAPI recorrido con el servidor real (26 operaciones), versión mínima de cliente (426),
  idempotencia con reintentos simultáneos, catálogo completo y por cambios, «Agotado» y los casos de los pedidos sin
  conexión.
- Agente de impresión con un ejecutor simulado, «La Caja 2 está desconectada» e impresora de tickets por caja.
- Respaldos programados con `pg_dump`/`pg_restore` reales: programación, retención solo de lo propio, verificación,
  lock 7820148309, carpeta elegida y avisos.
- Migración 0023 de 22 a 23 con datos.

## No ejecutado o no verificado

La instalación del `.pkg`, launchd y el arranque sin iniciar sesión en la computadora real; una segunda computadora en
la red (emparejamiento real, permiso «Red local», firewall, cambio de IP real, Wi-Fi frente a VPN); el Keychain real con
la credencial de una terminal emparejada; el diálogo nativo de carpeta y una USB real (con el permiso de privacidad del
servicio); papel, logotipo y cajón por la impresora USB de otra terminal; la carga con varias terminales; Android (no
existe); y todo Windows (Fase W). Detalle en [VERIFICATION_STAGE9G.md](VERIFICATION_STAGE9G.md), «No verificado».

## Regresión final

<!-- PENDIENTE-COORDINADOR: tabla de la regresión final de la Fase G (la misma de VERIFICATION_STAGE9G.md). -->

Pendiente al escribir este documento. La regresión final de la etapa anterior está en
[VERIFICATION_STAGE9F.md](VERIFICATION_STAGE9F.md), «Regresión final».

## Repetir las comprobaciones

En la Mac:

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
./scripts/dev.sh build-macos
```

`test-integration` y `test-browser` usan bases desechables `adel_pos_test_*` dentro del clúster del proyecto; para
aislamiento total, el procedimiento de `CLAUDE.md` §6 (con `test_mode: true` en la configuración del Go temporal).
Desde la 9G la tupla de integración incluye además `./internal/apicontract`, `./internal/backupschedule`,
`./cmd/ad-el-pos-server` (prueba del `host` de macOS, con PostgreSQL 18 de Homebrew o `ADEL_PG_BIN`) e
`./internal/discovery` (anuncio real acotado: necesita macOS y red), y `./scripts/dev.sh build-macos-pkg` compila el
instalador sin instalar nada. En Windows: [VERIFICATION_STAGE9E.md](VERIFICATION_STAGE9E.md), «Repetir las
comprobaciones», y las listas de la 9F y de la 9G en [NOTAS_WINDOWS.md](NOTAS_WINDOWS.md).
