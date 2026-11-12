import { Accordion } from '../../components/Accordion';
import { Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { Board, Release } from '../../services/kitchen';
import { preparationLabels } from '../../services/kitchen';
import type { Bill } from '../../services/restaurant';

// Etapa 9D (§4.4, Fase 0 punto 4): Go registra una advertencia por unidad («Sin ruta de
// preparación: producto» se repetía por cada unidad). Cada advertencia distinta se muestra una sola
// vez, con cuántas veces se registró.
export function distinctWarnings(warnings: string[]) {
  const counts = new Map<string, number>();
  for (const warning of warnings) {
    counts.set(warning, (counts.get(warning) ?? 0) + 1);
  }
  return Array.from(counts, ([text, count]) => ({ text, count }));
}

export function AccountKitchen({
  bill,
  allStations,
  onClose,
}: {
  bill: Bill;
  allStations: boolean;
  onClose: () => void;
}) {
  const board = useLiveResource<Board>('kitchen_board', {
    query: { account_id: bill.account.id, state: 'all' },
  });
  const releases = useLiveResource<Release[]>('kitchen_releases', {
    query: { account_id: bill.account.id },
    enabled: allStations,
  });
  return (
    <section className="operation-inline-panel form-stack">
      <div className="section-heading">
        <h2>Preparación · #{bill.account.folio}</h2>
        <button className="secondary-button" onClick={onClose}>
          <IconLabel icon="back">Volver</IconLabel>
        </button>
      </div>
      <p>{preparationLabels[bill.account.preparation_state]}</p>
      <Notice>{board.error || releases.error}</Notice>
      {releases.data?.map((release) => (
        <Accordion
          key={release.id}
          title={
            release.state === 'released'
              ? 'Pedido liberado'
              : release.state === 'scheduled'
                ? 'Pedido programado'
                : 'Liberación pendiente'
          }
          summary={release.created_at}
        >
          {distinctWarnings(release.warnings).map((warning) => (
            <Notice key={warning.text}>
              {warning.count > 1 ? `${warning.text} (×${warning.count})` : warning.text}
            </Notice>
          ))}
          <p className="muted">
            El seguimiento y la resolución de impresión se administran en Cocina y preparación.
          </p>
        </Accordion>
      ))}
      {board.data?.tasks.map((task) => (
        <article className="kitchen-account-task" key={task.id}>
          <strong>
            {task.station_name} · {task.instructions.name}
          </strong>
          <p>{preparationLabels[task.state]}</p>
          {task.instructions.notes && <p>{task.instructions.notes}</p>}
        </article>
      ))}
      {board.data?.tasks.length === 0 && (
        <p className="muted">
          No hay tareas en tus estaciones autorizadas. Revisa los avisos de liberación y la
          configuración de rutas.
        </p>
      )}
      <p className="muted">
        La aceptación del transporte no confirma papel. Los estados de pago y entrega permanecen
        separados.
      </p>
    </section>
  );
}
