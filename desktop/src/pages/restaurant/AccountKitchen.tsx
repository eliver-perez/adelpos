import { Accordion } from '../../components/Accordion';
import { Notice } from '../../components/Forms';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { Board, Release } from '../../services/kitchen';
import { preparationLabels } from '../../services/kitchen';
import type { Bill } from '../../services/restaurant';

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
          Volver
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
          {release.warnings.map((warning, index) => (
            <Notice key={index}>{warning}</Notice>
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
