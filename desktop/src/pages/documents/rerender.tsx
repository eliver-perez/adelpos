import type { ConfirmRequest } from '../../components/ConfirmDialog';

// «Imprimir con la plantilla vigente» (etapa 9B): acción distinta de «Reimprimir». La reimpresión
// es una copia idéntica al original (mismos datos y misma versión de plantilla); esta acción genera
// otro documento con los mismos datos históricos pero con la plantilla activa hoy y la identidad
// vigente. Se ofrece junto a las reimpresiones existentes, solo con su permiso de reimprimir y
// documents.history.render_current, y siempre con esta confirmación.

export const renderCurrentPermission = 'documents.history.render_current';

export const renderCurrentLabel = 'Imprimir con la plantilla vigente';

export function renderCurrentConfirmation(subject: string): ConfirmRequest {
  return {
    title: renderCurrentLabel,
    message: (
      <>
        <p>
          «Reimprimir» envía una copia idéntica al original, con el formato con el que se imprimió.
        </p>
        <p>
          Esta acción vuelve a generar {subject} con sus mismos datos históricos (productos,
          importes y pagos no cambian) pero con la plantilla activa hoy y la identidad vigente del
          establecimiento, así que su presentación puede ser distinta.
        </p>
        <p>No crea ventas, pagos ni movimientos y queda registrada en la auditoría.</p>
      </>
    ),
    confirmLabel: renderCurrentLabel,
  };
}
