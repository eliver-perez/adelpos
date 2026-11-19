import { Accordion } from '../components/Accordion';
import { auditActionLabel } from '../services/auditActions';
import { useState } from 'react';
import { dateTime, Field, Notice, PageTitle } from '../components/Forms';
import { useResource } from '../hooks/useResource';
import { countLabel } from '../services/plural';
import { DateTimeField } from '../components/TimeField';
import type { AuditPage as AuditPageData } from '../services/models';

// Quién hizo el movimiento, en palabras del restaurante (Go guarda un código por tipo de autor).
const actorKinds: Record<string, string> = {
  user: 'Usuario',
  device: 'Terminal',
  local_owner: 'Equipo servidor',
  process: 'Proceso automático',
  migration: 'Actualización del sistema',
  // Etapa 9G (fase B): terminal nueva que todavía no se empareja.
  pairing: 'Terminal por emparejar',
};

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
      {/* Etapa 9C (§4.2): la antigua «Auditoría» se llama «Registro de movimientos»; el permiso
          audit.read, la ruta y los datos no cambian. */}
      <PageTitle
        title="Registro de movimientos"
        description="Historial de accesos y cambios administrativos: quién los hizo, cuándo y desde qué terminal."
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
          <Field label="Usuario (nombre de acceso exacto)">
            <input value={actor} onChange={(event) => setActor(event.target.value)} />
          </Field>
          <Field
            label="Código de la operación"
            hint="Tal como aparece en «Ver cambios» de cada movimiento."
          >
            <input
              placeholder="Por ejemplo: user.updated"
              value={action}
              onChange={(event) => setAction(event.target.value)}
            />
          </Field>
          <Field
            label="Identificador de la terminal"
            hint="El que aparece en «Ver configuración» de Sistema › Terminales, o como «Terminal» en «Ver cambios» de sus movimientos."
          >
            <input value={device} onChange={(event) => setDevice(event.target.value)} />
          </Field>
          {/* Etapa 9C (§2.1): fecha y hora con el selector, de minuto en minuto. Sin hora, «Desde»
              propone 00:00 y «Hasta» 23:59; el filtro se sigue enviando en la hora de esta terminal
              convertida a UTC, como antes. */}
          <DateTimeField
            label="Desde"
            step={1}
            defaultTime="00:00"
            value={from}
            onChange={setFrom}
          />
          <DateTimeField
            label="Hasta"
            step={1}
            defaultTime="23:59"
            value={until}
            onChange={setUntil}
          />
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
              <th>Usuario</th>
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
                  <small className="muted block">{actorKinds[event.actor_kind] ?? 'Otro'}</small>
                </td>
                <td>
                  {/* Etapa A3 (§1, A3-D22): la operación con su nombre, o un texto general si esta
                      versión no la conoce; nunca el código, que queda en «Ver cambios» junto a los
                      demás datos técnicos del movimiento (los que se usan para filtrar). */}
                  {auditActionLabel(event.action)}
                </td>
                <td>{event.success ? 'Correcto' : 'Rechazado'}</td>
                <td>
                  <Accordion title="Ver cambios">
                    <dl className="audit-details">
                      <dt>Código de la operación</dt>
                      <dd className="font-mono">{event.action}</dd>
                      <dt>Tipo de registro</dt>
                      <dd className="font-mono">{event.entity_type}</dd>
                      <dt>Registro afectado</dt>
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
