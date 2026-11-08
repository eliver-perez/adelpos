import { Accordion } from '../../components/Accordion';
import { HistoryDetails } from '../../components/restaurant/HistoryDetails';
import { Notice } from '../../components/Forms';
import { useLiveResource } from '../../hooks/useLiveResource';
import { eventLabels, serverDateTime } from '../../services/restaurant';
import type { Bill, HistoryPage } from '../../services/restaurant';
import { useState } from 'react';

export function AccountHistory({ bill, onClose }: { bill: Bill; onClose: () => void }) {
  const [page, setPage] = useState(1);
  const history = useLiveResource<HistoryPage>('restaurant_history', {
    id: bill.account.id,
    query: { page: String(page) },
  });
  return (
    <section className="operation-inline-panel">
      <div className="section-heading">
        <h2>Historial · #{bill.account.folio}</h2>
        <button className="secondary-button" onClick={onClose}>
          Volver
        </button>
      </div>
      <Notice>{history.error}</Notice>
      {!history.data && <p className="muted">Consultando historial…</p>}
      {history.data?.items.map((event) => (
        <Accordion
          key={event.id}
          title={eventLabels[event.kind] ?? event.kind}
          summary={`${serverDateTime(event.created_at, bill.timezone)} · ${event.author_name} · ${event.device_name}`}
        >
          <p className="muted text-xs">
            Registro persistente #{event.sequence}. Integración con cocina pendiente.
          </p>
          <HistoryDetails event={event} />
        </Accordion>
      ))}
      <div className="button-row">
        <button className="secondary-button" disabled={page <= 1} onClick={() => setPage(page - 1)}>
          Más recientes
        </button>
        <button
          className="secondary-button"
          disabled={!history.data || page * 100 >= history.data.total}
          onClick={() => setPage(page + 1)}
        >
          Más antiguos
        </button>
      </div>
    </section>
  );
}
