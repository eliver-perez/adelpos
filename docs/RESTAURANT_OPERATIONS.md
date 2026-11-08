# Operación del restaurante · Etapa 4

Versión 0.4.0, esquema 4. Continúa el catálogo y la seguridad existentes. `internal/operations` registra cuentas y pedidos; `internal/catalog` sigue siendo el único motor financiero.

## Recorrido de operación

1. En **Restaurante → Áreas y distribución**, crear áreas y mesas. El orden de área, forma, capacidad, dimensiones y posiciones persisten. Arrastrar o usar flechas en el diseñador y pulsar **Guardar distribución**; Shift mueve una unidad lógica.
2. En **Mesas**, seleccionar área y mesa. Las cuentas de esa mesa se eligen en la misma pantalla. Una mesa admite varias cuentas. **Cuentas** permite localizar cuentas de salón, incluidas las que no tienen mesa.
3. Abrir una cuenta, seleccionar producto/categoría/búsqueda, cantidad, presentación y personalizaciones. El panel derecho muestra la cuenta y el total calculado por Go. Los complementos admiten retirar todos los originales cuando el mínimo del grupo lo permite.
4. **Guardar en borrador** persiste la captura. **Confirmar pedido** es una segunda acción explícita: registra un lote con sus precios, promociones y configuración histórica. Agregar productos después crea otro lote al confirmar.
5. **Modificar** una partida confirmada abre la misma personalización y exige motivo. **Cancelar unidades** permite retirar una cantidad entera, con motivo e historial. El mesero necesita los permisos correspondientes; no se exige un supervisor universal.

Cerrar el editor con **Conservar y volver**, cambiar de cuenta o de vista interna conserva la selección local. El aviso permite retomarla o descartarla expresamente. Una recarga o cierre completo del proceso descarta esa selección aún no guardada; el borrador persistido permanece en PostgreSQL.

## Estados reales

| Dimensión            | Estados implementados                               | Transición                                                                                                                       |
| -------------------- | --------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| Cuenta               | `open`, `merged`, `voided`                          | Abrir; combinar hacia otra cuenta; anular solo sin unidades activas ni borradores.                                               |
| Captura              | Borrador persistido                                 | Guardar/reemplazar mediante revisión; confirmar vacía el lote pendiente.                                                         |
| Unidades             | `active`, `cancelled`                               | Cancelación explícita; no se eliminan sus identidades.                                                                           |
| Preparación derivada | `without_items`, `draft` y estados reales de cocina | Depende de unidades activas y borrador; la etapa 5 informa liberación/tareas reales, sin confundirlas con papel o recepción KDS. |
| Pago                 | `not_recorded`                                      | No hay módulo de cobro en esta etapa.                                                                                            |
| Entrega              | `not_delivered`                                     | No existe comando para entregar sin liquidación.                                                                                 |

La mesa es libre si no tiene cuentas abiertas; ocupada si tiene una o más. Fuera de servicio impide nuevas asignaciones, pero permite consultar cuentas existentes. No se desactiva una mesa o área con cuentas abiertas. Desactivar conserva referencias históricas. Anular una cuenta vacía o combinarla retira su ocupación; la liquidación normal se integrará con cobros.

## Precios históricos y cambios

El servidor vuelve a calcular al confirmar y compara la huella revisada por el operador. Si cambió catálogo, disponibilidad, lista o promoción, responde `QUOTE_CHANGED` y conserva el borrador. La huella excluye únicamente la hora de cálculo; incluye el contexto, las revisiones y el resultado comercial.

Cada lote guarda snapshot del catálogo, lista y cotización. Los cambios posteriores del catálogo no actualizan pedidos confirmados.

- Cambiar solo observaciones de preparación conserva los centavos históricos. Reducir cantidad sin cambiar receta conserva los importes de las unidades restantes y exige además permiso de cancelar.
- Cambiar presentación, receta/complementos o aumentar cantidad cotiza expresamente la parte afectada con el catálogo vigente. El operador revisa el resultado antes de **Confirmar modificación**. El resto de la cuenta permanece intacto.
- Las promociones automáticas se calculan por lote nuevo o modificación cotizada. Agregar otro lote no redistribuye descuentos de los anteriores.
- Las listas se seleccionan con `orders.price_list.manage`; sin selección se utiliza la predeterminada existente. No hay reglas automáticas de lista por modalidad todavía.

Los importes usan decimal y `numeric(18,2)`; JSON los transporta como cadenas. Cada unidad comercial tiene UUID, ordinal, subtotal, descuento y total exactos. Los centavos residuales se asignan determinísticamente a los primeros ordinales. Por ejemplo, $40 entre tres unidades mantiene $13.34, $13.33 y $13.33. Transferir conserva esos importes.

## División, combinación y transferencias

**Dividir cuenta** selecciona unidades enteras de una o más partidas y crea otra cuenta heredando mesa, modalidad, lista, cliente, programación y responsable. Se puede repetir para obtener más cuentas. **Transferir productos** mueve unidades a una cuenta existente. **Combinar cuentas** mueve todas las unidades activas y los borradores y marca el origen como combinado. La cuenta de origen y sus cancelaciones conservan historial.

La cuenta destino debe estar abierta y compartir modalidad y lista. Para llevar exige además el mismo cliente, teléfono y programación. Los destinos de mesa deben estar activos y disponibles. Cambiar responsable y cambiar mesa son acciones independientes con permisos propios.

Se mueven las mismas unidades, sin clonar identidades, recalcular descuentos ni crear otro pedido/preparación. Origen, destino, revisión, cantidades e historial se actualizan en una sola transacción. Las fracciones de una unidad comercial no se dividen; las cantidades decimales de ingredientes y porciones permanecen dentro de su personalización.

## Para llevar y ventanilla

**Para llevar** presenta listado izquierdo y detalle/cuenta derecho. El orden ascendente usa la creación del servidor; muestra hora, tiempo transcurrido, cliente/referencia, origen, importe confirmado y programación. Busca por folio, referencia, cliente o teléfono. La selección permanece mientras se actualiza el listado.

Mostrador, Teléfono y Ventanilla son sugerencias de origen de la misma modalidad `takeaway`. No se crean mesas ni un directorio de clientes. Preparación, pago y entrega se muestran separados. Advertencia y antigüedad alta usan umbrales configurables; por defecto 15 y 30 minutos. El sondeo actualiza la antigüedad cada cinco segundos.

La programación admite hora local de preparación o entrega prevista. Go la interpreta en la zona IANA configurada del establecimiento, conserva el instante UTC y devuelve la representación local. Rechaza horas locales inexistentes. En una hora ambigua por cambio estacional se utiliza la resolución de `time.ParseInLocation`; no hay selector de primera/segunda ocurrencia. La etapa 5 permite liberación manual o automática con política capturada por pedido y trabajador Go; no habilita entrega.

## Concurrencia y recuperación

Cada escritura lleva `operation_id` UUID y revisión del agregado. El resultado se guarda en `restaurant_operations` dentro de la misma transacción. Repetir actor, dispositivo, ruta y cuerpo devuelve el resultado original; reutilizar UUID con otro contenido da `IDEMPOTENCY_CONFLICT`. Una revisión obsoleta da `REVISION_CONFLICT`; nunca sobrescribe silenciosamente.

El cliente guarda el UUID/cuerpo en memoria mientras desconoce el resultado de una interrupción. **Recuperar resultado** reintenta esa misma operación y después consulta el estado actual. Bloquea otras escrituras hasta resolverla. Tras cerrar el proceso, consultar el estado del servidor antes de repetir manualmente una operación incierta; no hay cola offline persistente.

Lecturas y previsualizaciones usan lock compartido de operación; escrituras usan lock exclusivo transaccional. El catálogo tiene su lock separado. Esto prioriza consistencia y simplifica dos terminales; no se ha medido carga comercial elevada. El sondeo es de cinco segundos, se detiene cuando la vista está inactiva/oculta y descarta respuestas fuera de orden. Go revalida permisos, sesión y dispositivo en cada petición.

## Integración de cocina desde etapa 5

`restaurant_events` es un diario durable con secuencia, operación, cuenta relacionada, autor, terminal, instante y valores anterior/posterior. Confirmaciones, modificaciones, cancelaciones y transferencias guardan las identidades necesarias para interpretar sus cambios. Se conserva el estado legado del diario; `kitchen_event_receipts` registra su integración transaccional y `realtime_events` publica invalidaciones.

La etapa 5 implementa estaciones, rutas, liberaciones, documentos, cola persistente CUPS/ESC-POS y SSE autenticado con cursor/replay. La preparación y sus contratos KDS se describen en [KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md); no hay aplicación KDS independiente ni movimientos de inventario. Un traslado contable conserva las unidades y no genera otra preparación.

## Componentes visuales y límites

`Accordion` reemplaza los `details/summary` anteriores en horarios, listas/promociones, hijos de combo, desgloses, excepción de disponibilidad y auditoría. Usa botón de encabezado, resumen, chevron, aria, panel montado, `inert`, transición breve y `prefers-reduced-motion`. Revela errores nativos, errores indicados por prop y errores de formularios asincrónicos. Mantiene valores al cerrar.

`VirtualKeyboard` respeta `auto`, `always` y `disabled` por terminal. Incluye alfanumérico, numérico, acentos, selección, borrar y cierre; conserva teclado físico y excluye contraseñas. Reserva espacio y permite desplazar formularios por encima del teclado. Fecha/hora utiliza el control nativo del sistema.

El mapa usa un lienzo lógico de 1000×560 y desplazamiento interno. La captura limita cada lote a 20 partidas y 1000 unidades totales; una selección de traslado admite hasta 100 partidas. Listados de cuentas paginan hasta 100 por página; historial, 100 eventos por página. El catálogo conserva su límite de 5000 referencias. Las respuestas IPC siguen limitadas a 2 MiB: cuentas e historiales excepcionalmente grandes necesitan paginación de detalle antes de uso a gran escala. Snapshots completos por lote incrementan almacenamiento; falta política de retención y medición de rendimiento.

## Verificación reproducible

```bash
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
./scripts/dev.sh build-macos
```

La primera instalación de navegadores de prueba requiere `cd desktop` y `PLAYWRIGHT_BROWSERS_PATH="$PWD/../.cache/playwright" npx playwright install chromium webkit`. Playwright es dependencia de desarrollo fijada. El ejecutor crea una base exclusiva por navegador, ejecuta Go real, sirve el frontend compilado y sustituye únicamente el transporte IPC en Node; los secretos nunca llegan al navegador. No agrega un acceso web alternativo al producto. La compilación y los procesos nativos se verifican por separado; ver [VERIFICATION.md](VERIFICATION.md).
