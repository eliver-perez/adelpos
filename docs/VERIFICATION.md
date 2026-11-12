# Verificación · Etapa 9D

Estado al **12 de noviembre de 2026**, zona America/Monterrey. Versión **0.9.3**, API v1, esquema **21** (migración
`0021_account_cancellation.sql`, etapa 9D: ajustes de interfaz, color de marca y cancelar cuenta). Evidencia detallada
por fase: [VERIFICATION_STAGE9D.md](VERIFICATION_STAGE9D.md). Evidencia anterior: [etapa 9C](VERIFICATION_STAGE9C.md),
[etapa 9B](VERIFICATION_STAGE9B.md), [etapa 9A](VERIFICATION_STAGE9A.md), [etapa 8](VERIFICATION_STAGE8.md),
[etapa 7.1](VERIFICATION_STAGE7_1.md), [etapa 7](VERIFICATION_STAGE7.md), [etapa 6.2](VERIFICATION_STAGE6_2.md),
[etapa 6.1](VERIFICATION_STAGE6_1.md), [etapa 6](VERIFICATION_STAGE6.md), [etapa 5](VERIFICATION_STAGE5.md),
[etapa 4](VERIFICATION_STAGE4.md), [etapa 3](VERIFICATION_STAGE3.md), [etapa 2](VERIFICATION_STAGE2.md) y
[etapa 1](VERIFICATION_STAGE1.md). Avance: [ETAPA_9D_AVANCE.md](ETAPA_9D_AVANCE.md). Notas para Windows:
[NOTAS_WINDOWS.md](NOTAS_WINDOWS.md).

## Estado

- **Código**: fase 0 (`ecae96d`), backend de las fases D y E (`b84e3be`), fase A (`4abe78d`), fase E de la interfaz
  (`2c00064`), fase B (`7506535`), fase C (`ffcaed7`), correcciones de la parte 1 de la revisión independiente (`d723e92`)
  y fase D de la interfaz (`81ff531`), en commits locales de la rama `etapa-9d` (creada desde `main`, que contiene la 9C),
  cada parte con sus suites aprobadas en un entorno aislado (clúster PostgreSQL temporal en el puerto 55499 y Go temporal
  para Playwright), según [VERIFICATION_STAGE9D.md](VERIFICATION_STAGE9D.md); fase F con la versión 0.9.3, la
  documentación y la regresión final.
- **Revisión independiente**: parte 1 (backend, fases A y E) sin defectos críticos ni altos, con uno medio y siete bajos,
  corregidos en `d723e92` (el bajo B6 quedó como nota sin cambio de cálculo); parte 2 (fases B y C y las correcciones) sin
  defectos críticos, altos ni medios, con dos observaciones bajas que atendió la fase D. La revisión de la fase D (parte 3) estaba en curso al escribir este documento.
- **Primeras corridas de WebKit con fallas en cascada** (fases A, C y D): un fallo inicial y el reinicio del trabajador
  de Playwright arrastraron a las especificaciones siguientes; la causa de las dos primeras (foco del diálogo «Acerca de»)
  se corrigió en la fase B y la de la fase D fue un clic en «Administración» que no navegó antes de cualquier paso de la
  fase; las repeticiones y las corridas finales aprobaron todo en ambos motores.
- **Regresión final (fase F)**: ver «Regresión final».
- **Instalación real**: **sigue en 0.9.2 y esquema 20**. La 0021 y la 0.9.3 **todavía no se aplican**; cuando se
  apliquen, el registro (respaldo, ensayo de restauración, `migrate`, verificación posterior e instalación del `.app`)
  quedará en [ETAPA_9D_AVANCE.md](ETAPA_9D_AVANCE.md), «Instalación real».
- **Las pruebas no tocaron la instalación real** (`.local`, Keychain, servidor 8181, clúster 55432) ni enviaron trabajos
  a CUPS real, a una impresora, a un cajón o a una conexión TCP fuera de loopback; las consultas de la Fase 0 a la
  instalación real fueron de solo lectura.

## Qué está verificado

Todo con Go, PostgreSQL temporal, Vitest, Rust y Playwright (Chromium y WebKit) en entorno aislado, con el puente nativo
simulado (`e2e/native-driver.ts`) y, en impresión, solo con el transporte controlado de la cola. «Esperando pago», «3 d
14 h», `long_open` y «Sin conexión» los presenta el puente de la prueba en las capturas; la cancelación, los pagos
parciales y los reportes usan datos reales de Go.

| Área                                    | Evidencia                                                                                                                                                                                                                                                                                          |
| --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Migración 0021                          | `TestStage9DAccountCancellationUpgrade` (sobre un esquema 20 con datos: motivos sembrados, umbral 12, `accounts.cancel` solo para el administrador, sin tocar cuentas ni eventos existentes).                                                                                                      |
| Color de marca y colores en tokens (§1) | `styles.test.ts` (sin colores escritos fuera de los tokens, 148 nombres de color, contraste AA en ambos temas, sin verde, uva frente a azul), `components/stage9dStyles.test.tsx` (avisos, estados con icono, destructivos) y Playwright `workflow-stage9d-colors` con capturas en claro y oscuro. |
| Destructivos y cursor (§1.5–1.6)        | Recorrido del código con la lista justificada de reversibles; `cursor: not-allowed` y `progress` con `aria-busy` comprobados con el estilo calculado en Chromium y WebKit.                                                                                                                         |
| Barra superior (§2)                     | `navigation/navigation.test.tsx`, `stage9dLayout.test.ts` y Playwright `workflow-stage9d-topbar-tables` (orden, mismo ancho, 52–56 px, sin cortes a 1280 × 720 y 1920 × 1080 en ambos temas, «Sin conexión»).                                                                                      |
| Mesas (§3)                              | `pages/restaurant/stage9dTables.test.tsx` (panel siempre visible, resumen con resaltado, escala, mínimo de 64 px, «d h», `long_open`, sin «Actualizar») y Playwright (35 mesas que se desplazan arrastrando con el ratón a 1280 × 720).                                                            |
| Cuentas y Para llevar (§4)              | `stage9dAccounts.test.tsx` (18) y Playwright `workflow-stage9d-accounts` (nada guardado sin productos, descarte sin motivo y fuera de «Cuentas canceladas», tarjetas de 44–64 px, «Enviado · sin confirmar en cocina», menú por producto).                                                         |
| Cancelar cuenta (§5)                    | `TestStage9DAccountCancellation` (permiso propio y supervisor, preparados y no preparados, pagos parciales y ajustes, «Otro» sin descripción, `ACCOUNT_DISCARDABLE`, estación inactiva, idempotencia), `cancelAccount.test.tsx` y Playwright `workflow-stage9d-cancel-account`.                    |
| Reportes de cancelaciones               | `TestStage9DCancelledAccountsReports` («Cuentas canceladas» con «Autorizó», total por motivo, exclusión de ventas, filtros, permisos, PDF y Excel) y el recorrido de Playwright con ambas exportaciones.                                                                                           |
| Umbral de cuenta abierta                | Subtests `9D_umbral_*` de `internal/administration` (lectura, permiso, validación, revisión, auditoría, mapa y tablero) y `stage9dCancelAdmin.test.tsx`.                                                                                                                                           |
| Puente                                  | Rust: las siete operaciones nuevas cerradas y `restaurant_void` sin ruta; 95 reportes en `EXPORTABLE_REPORTS`.                                                                                                                                                                                     |
| Revisión independiente                  | Revisión de solo lectura en dos partes, con reproducciones propias; sus correcciones tienen prueba.                                                                                                                                                                                                |

## No ejecutado o no verificado

- **Pantalla táctil física** y, en particular, **el arrastre del mapa con el dedo** en hardware real (se probó con el
  ratón de Playwright).
- **Cursor en la ventana nativa**: el puntero «no disponible» real de macOS en la ventana Tauri (WKWebView); solo el
  estilo calculado del navegador.
- **Impresión real del aviso de cancelación** de una cuenta en la impresora de una estación.
- **Windows**: nada de la 9D se compiló ni probó allí ([NOTAS_WINDOWS.md](NOTAS_WINDOWS.md)).
- La ventana nativa en general, dos terminales reales, LAN con otra máquina, Keychain real, lector de pantalla real y
  Mac Intel.
- Pendientes de etapas anteriores sin cambio: papel con formato y logotipo (9B), papel a 48/32 columnas y menú nativo
  real (9C), cajón físico y diálogo nativo real de guardado (9A).

## Regresión final

PENDIENTE: la completa el coordinador con la salida real

## Repetir las comprobaciones

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
```

`test-integration` y `test-browser` usan bases desechables en el clúster del proyecto (55432); para aislamiento total,
seguir `CLAUDE.md` §6. Las pruebas de integración de la 9D viven en `internal/database` (actualización a la 0021),
`internal/administration` (umbral y permiso), `internal/reports` (cuentas canceladas y total por motivo) e
`internal/operations` (`TestStage9DAccountCancellation`, junto a la aceptación encadenada). Las especificaciones
Playwright comparten la base de cada motor y corren en orden alfabético: las de la 9D se llaman
`workflow-stage9d-*.spec.ts` para correr después de las existentes; no conviene filtrarlas con `documents-` (la ruta
temporal contiene «Documents-ADEL-POS»). Con la pantalla del equipo bloqueada, Playwright WebKit puede no hacer clic
(«<html> intercepts pointer events»): repetir WebKit con la sesión desbloqueada y reportarlo como no verificado.
