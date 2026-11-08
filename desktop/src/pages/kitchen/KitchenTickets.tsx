import { useState } from 'react';
import { Notice } from '../../components/Forms';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { TicketDocument, TicketSummary } from '../../services/kitchen';
import { serverDateTime } from '../../services/restaurant';

export function KitchenTickets({ timezone }: { timezone: string }) {
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState('');
  const list = useLiveResource<TicketSummary[]>('kitchen_tickets', {
    query: { page: String(page) },
  });
  const detail = useLiveResource<TicketDocument>('kitchen_ticket', {
    id: selected || undefined,
    enabled: !!selected,
  });
  const labels = {
    normal: 'Comanda',
    modification: 'MODIFICACIÓN',
    cancellation: 'CANCELACIÓN',
    test: 'Prueba autorizada',
  };
  return (
    <div className="kitchen-editor-layout">
      <section className="panel form-stack">
        <h2>Comandas y recepción</h2>
        <Notice>{list.error}</Notice>
        {list.data?.map((ticket) => (
          <button
            className="kitchen-list-button"
            key={ticket.id}
            onClick={() => setSelected(ticket.id)}
          >
            <strong>
              {labels[ticket.kind]} · Cuenta #{ticket.folio}
            </strong>
            <span>{serverDateTime(ticket.created_at, timezone)}</span>
            {ticket.pending_kds && (
              <span className="kitchen-pending-kds">Recepción KDS pendiente</span>
            )}
          </button>
        ))}
        {list.data?.length === 0 && (
          <p className="muted">Las comandas se generan al liberar productos confirmados.</p>
        )}
        <div className="button-row">
          <button
            className="secondary-button"
            disabled={page === 1}
            onClick={() => setPage(page - 1)}
          >
            Más recientes
          </button>
          <button
            className="secondary-button"
            disabled={(list.data?.length ?? 0) < 100}
            onClick={() => setPage(page + 1)}
          >
            Más antiguas
          </button>
        </div>
      </section>
      <section className="panel form-stack">
        <h2>Documento de preparación</h2>
        <Notice>{detail.error}</Notice>
        {detail.data ? (
          <>
            <p className="muted">
              Vista del documento guardado. La recepción del transporte y el avance de preparación
              se consultan por separado.
            </p>
            <pre className="kitchen-document">{detail.data.preview}</pre>
          </>
        ) : (
          <p className="muted">
            Selecciona una comanda para consultar sus instrucciones históricas.
          </p>
        )}
      </section>
    </div>
  );
}
