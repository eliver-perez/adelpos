import { Accordion } from '../components/Accordion';
import { useState } from 'react';
import { dateTime, Field, Notice, PageTitle } from '../components/Forms';
import { useResource } from '../hooks/useResource';
import { countLabel } from '../services/plural';
import type { AuditPage as AuditPageData } from '../services/models';

export function AuditPage() {
  const [actor, setActor] = useState('');
  const [action, setAction] = useState('');
  const [device, setDevice] = useState('');
  const [from, setFrom] = useState('');
  const [until, setUntil] = useState('');
  const [query, setQuery] = useState<Record<string, string>>({ page: '1', page_size: '25' });
  const resource = useResource<AuditPageData>('audit', { query });
  return (
    <>
      <PageTitle
        title="Auditoría"
        description="Historial persistente de accesos y cambios administrativos."
      />
      <form
        className="surface panel form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          setQuery({
            actor,
            action,
            device_id: device,
            from: from ? new Date(from).toISOString() : '',
            until: until ? new Date(until).toISOString() : '',
            page: '1',
            page_size: '25',
          });
        }}
      >
        <div className="form-grid">
          <Field label="Actor (usuario exacto)">
            <input value={actor} onChange={(event) => setActor(event.target.value)} />
          </Field>
          <Field label="Operación exacta">
            <input
              placeholder="Por ejemplo: user.updated"
              value={action}
              onChange={(event) => setAction(event.target.value)}
            />
          </Field>
          <Field label="ID de terminal">
            <input value={device} onChange={(event) => setDevice(event.target.value)} />
          </Field>
          <Field label="Desde">
            <input
              type="datetime-local"
              value={from}
              onChange={(event) => setFrom(event.target.value)}
            />
          </Field>
          <Field label="Hasta">
            <input
              type="datetime-local"
              value={until}
              onChange={(event) => setUntil(event.target.value)}
            />
          </Field>
        </div>
        <div>
          <button className="primary-button" type="submit">
            Aplicar filtros
          </button>
        </div>
      </form>
      <Notice>{resource.error}</Notice>
      <div className="surface table-container">
        <table>
          <thead>
            <tr>
              <th>Momento</th>
              <th>Actor</th>
              <th>Operación</th>
              <th>Resultado</th>
              <th>Detalles</th>
            </tr>
          </thead>
          <tbody>
            {resource.data?.items.map((event) => (
              <tr key={event.id}>
                <td>{dateTime(event.occurred_at)}</td>
                <td>
                  {event.actor_username}
                  <small className="muted block">{event.actor_kind}</small>
                </td>
                <td>
                  <span className="font-mono text-xs">{event.action}</span>
                  <small className="muted block">{event.entity_type}</small>
                </td>
                <td>{event.success ? 'Correcto' : 'Rechazado'}</td>
                <td>
                  <Accordion title="Ver cambios">
                    <dl className="audit-details">
                      <dt>Entidad</dt>
                      <dd>{event.entity_id ?? '—'}</dd>
                      <dt>Terminal</dt>
                      <dd>{event.device_id ?? 'Local'}</dd>
                      <dt>Antes</dt>
                      <dd>
                        <pre>{JSON.stringify(event.before_state, null, 2)}</pre>
                      </dd>
                      <dt>Después</dt>
                      <dd>
                        <pre>{JSON.stringify(event.after_state, null, 2)}</pre>
                      </dd>
                    </dl>
                  </Accordion>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="pagination">
        <span className="muted text-sm">
          {countLabel(resource.data?.total ?? 0, 'evento', 'eventos')} · Página {query.page}
        </span>
        <div className="button-row">
          <button
            className="secondary-button"
            disabled={Number(query.page) <= 1}
            onClick={() => setQuery({ ...query, page: String(Number(query.page) - 1) })}
          >
            Anterior
          </button>
          <button
            className="secondary-button"
            disabled={!resource.data || Number(query.page) * 25 >= resource.data.total}
            onClick={() => setQuery({ ...query, page: String(Number(query.page) + 1) })}
          >
            Siguiente
          </button>
        </div>
      </div>
    </>
  );
}
