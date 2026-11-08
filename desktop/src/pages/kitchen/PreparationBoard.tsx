import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Form, Notice } from '../../components/Forms';
import { Check, IntegerField, TextField } from '../../components/catalog/EditorFields';
import { useLiveResource } from '../../hooks/useLiveResource';
import { preparationLabels, zonedInput } from '../../services/kitchen';
import type { Board, ExecuteKitchen, Release, Task } from '../../services/kitchen';
import { elapsedTime, serverDateTime } from '../../services/restaurant';

export function PreparationBoard({
  can,
  execute,
  busy,
}: {
  can: (key: string) => boolean;
  execute: ExecuteKitchen;
  busy: boolean;
}) {
  const [station, setStation] = useState('');
  const [state, setState] = useState('');
  const [page, setPage] = useState(1);
  const [changing, setChanging] = useState(false);
  const [schedule, setSchedule] = useState<Release | null>(null);
  const resource = useLiveResource<Board>('kitchen_board', {
    query: { station_id: station, state, page: String(page) },
  });
  const board = resource.data;
  const grouped = new Map<string, Task[]>();
  for (const task of board?.tasks ?? []) {
    const key = `${task.release_id}/${task.station_id}/${task.instructions.item_id}/${task.instructions.version_id}/${task.state}`;
    grouped.set(key, [...(grouped.get(key) ?? []), task]);
  }

  async function advance(tasks: Task[], next: 'preparing' | 'ready') {
    const first = tasks[0];
    if (!first) return;
    const body =
      first.tracking === 'order'
        ? {
            release_id: first.release_id,
            station_id: first.station_id,
            revision: first.release_revision,
            state: next,
          }
        : { tasks: tasks.map((task) => ({ id: task.id, revision: task.revision })), state: next };
    setChanging(true);
    try {
      await execute('kitchen_save_states', { body });
      // Conservar el bloqueo hasta recibir las revisiones nuevas evita que un segundo
      // toque rápido envíe el siguiente avance con la revisión anterior.
      await resource.reload();
    } catch {
      // El controlador común conserva el error y permite recuperar un resultado incierto.
    } finally {
      setChanging(false);
    }
  }

  return (
    <div className="form-stack">
      <div className="kitchen-filters">
        <Field label="Estación de preparación">
          <select
            value={station}
            onChange={(event) => {
              setStation(event.target.value);
              setPage(1);
            }}
          >
            <option value="">Todas las estaciones autorizadas</option>
            {board?.stations.map((entry) => (
              <option key={entry.id} value={entry.id}>
                {entry.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Avance de preparación">
          <select
            value={state}
            onChange={(event) => {
              setState(event.target.value);
              setPage(1);
            }}
          >
            <option value="">Pendientes y en preparación</option>
            <option value="ready">Listos</option>
            <option value="all">Todos los estados</option>
          </select>
        </Field>
        <button className="secondary-button" onClick={() => void resource.reload()}>
          Actualizar preparación
        </button>
      </div>
      <Notice>{resource.error}</Notice>
      {board && (board.releases.length > 0 || board.pending_changes.length > 0) && (
        <Accordion
          title="Pendientes de liberación y avisos"
          summary={`${board.releases.length} pedidos · ${board.pending_changes.length} cambios por revisar`}
          defaultOpen
        >
          <div className="kitchen-board-grid">
            {board.releases.map((release) => {
              const early =
                !!release.due_at &&
                new Date(release.due_at).getTime() > new Date(board.server_time).getTime();
              return (
                <article className="kitchen-preparation-card" key={release.id}>
                  <strong>
                    #{release.folio} · {release.account_name}
                  </strong>
                  <p>
                    {release.state === 'blocked'
                      ? 'Bloqueado por configuración'
                      : release.state === 'scheduled'
                        ? 'Programado'
                        : 'Pendiente de liberación'}
                  </p>
                  {release.scheduled_at && (
                    <p className="muted">
                      Programado: {serverDateTime(release.scheduled_at, board.timezone)} ·{' '}
                      {board.timezone}
                    </p>
                  )}
                  {release.due_at && (
                    <p>
                      Libera {release.automatic ? 'automáticamente' : 'manualmente'}:{' '}
                      {serverDateTime(release.due_at, board.timezone)}
                    </p>
                  )}
                  {release.warnings.map((warning, index) => (
                    <Notice key={index}>{warning}</Notice>
                  ))}
                  <div className="button-row">
                    {can('kitchen.release') && can('kitchen.stations.all') && (
                      <button
                        className="primary-button"
                        disabled={busy || (early && !can('kitchen.release_early'))}
                        onClick={() =>
                          void execute('kitchen_release', {
                            id: release.id,
                            body: { revision: release.revision },
                          }).catch(() => undefined)
                        }
                      >
                        {early ? 'Liberar anticipadamente' : 'Liberar a preparación'}
                      </button>
                    )}
                    {can('kitchen.timing.manage') && can('kitchen.stations.all') && (
                      <button className="secondary-button" onClick={() => setSchedule(release)}>
                        Ajustar programación
                      </button>
                    )}
                  </div>
                </article>
              );
            })}
            {board.pending_changes.map((change) => (
              <article className="kitchen-preparation-card" key={change.event_id}>
                <strong>
                  Aviso de {change.kind === 'item.cancelled' ? 'cancelación' : 'modificación'}{' '}
                  pendiente
                </strong>
                {change.warnings.map((warning, index) => (
                  <Notice key={index}>{warning}</Notice>
                ))}
                {can('kitchen.release') && can('kitchen.stations.all') && (
                  <button
                    className="secondary-button"
                    disabled={busy || changing}
                    onClick={() =>
                      void execute('kitchen_retry_change', {
                        id: change.event_id,
                        body: { revision: change.revision },
                      }).catch(() => undefined)
                    }
                  >
                    Revisar destinos y generar aviso
                  </button>
                )}
              </article>
            ))}
          </div>
        </Accordion>
      )}
      {schedule && board && (
        <ScheduleEditor
          key={schedule.id}
          release={schedule}
          current={board.releases.find((entry) => entry.id === schedule.id)}
          timezone={board.timezone}
          execute={execute}
          busy={busy}
          onClose={() => setSchedule(null)}
        />
      )}
      <div className="section-heading">
        <h2>Preparación por estación</h2>
        <span className="muted text-sm">Más antiguos primero · estado del servidor</span>
      </div>
      <div className="kitchen-board-grid">
        {Array.from(grouped.entries()).map(([key, tasks]) => {
          const task = tasks[0]!;
          const late = task.elapsed_seconds >= task.overdue_minutes * 60;
          const warning = task.elapsed_seconds >= task.warning_minutes * 60;
          return (
            <article
              className={`kitchen-preparation-card ${late ? 'kitchen-late' : warning ? 'kitchen-warning' : ''}`}
              key={key}
            >
              <div className="section-heading">
                <strong>{task.station_name}</strong>
                <span className="kitchen-state-label">{preparationLabels[task.state]}</span>
              </div>
              <p>
                #{task.folio} · {task.account_name} · {task.table_name ?? 'Sin mesa'}
              </p>
              <h3>
                {tasks.length} × {task.instructions.name}
              </h3>
              <p className="muted">
                {elapsedTime(task.elapsed_seconds)} · objetivo {task.target_minutes} min
              </p>
              {task.instructions.notes && (
                <p className="kitchen-preparation-note">{task.instructions.notes}</p>
              )}
              <Accordion
                title="Instrucciones y variantes"
                summary={`${task.instructions.parts.length} componentes de preparación`}
              >
                {task.instructions.parts.map((part) => (
                  <div key={part.path}>
                    <strong>
                      {part.quantity} × {part.name}
                      {part.variant ? ` · ${part.variant}` : ''}
                    </strong>
                    {part.notes && <p>{part.notes}</p>}
                  </div>
                ))}
                <p className="muted text-sm">
                  Consulta Comandas para el documento completo con ingredientes y complementos.
                </p>
              </Accordion>
              {task.state !== 'ready' &&
                task.state !== 'cancelled' &&
                can('kitchen.states.manage') && (
                  <div className="button-row">
                    {task.state === 'released' && (
                      <button
                        className="secondary-button"
                        disabled={busy || changing}
                        onClick={() => void advance(tasks, 'preparing')}
                      >
                        {task.tracking === 'order' ? 'Pedido en preparación' : 'En preparación'}
                      </button>
                    )}
                    <button
                      className="primary-button"
                      disabled={busy || changing}
                      onClick={() => void advance(tasks, 'ready')}
                    >
                      {task.tracking === 'order'
                        ? 'Pedido listo en estación'
                        : 'Marcar producto listo'}
                    </button>
                  </div>
                )}
            </article>
          );
        })}
      </div>
      {board?.tasks.length === 0 && (
        <div className="panel">
          <p className="muted">No hay unidades de preparación en este filtro.</p>
        </div>
      )}
      <div className="button-row">
        <button
          className="secondary-button"
          disabled={page === 1}
          onClick={() => setPage(page - 1)}
        >
          Anteriores
        </button>
        <span>Página {page}</span>
        <button
          className="secondary-button"
          disabled={(board?.tasks.length ?? 0) < 250}
          onClick={() => setPage(page + 1)}
        >
          Siguientes
        </button>
      </div>
    </div>
  );
}

function ScheduleEditor({
  release,
  current,
  timezone,
  execute,
  busy,
  onClose,
}: {
  release: Release;
  current?: Release;
  timezone: string;
  execute: ExecuteKitchen;
  busy: boolean;
  onClose: () => void;
}) {
  const [date, setDate] = useState(zonedInput(release.scheduled_at, timezone));
  const [automatic, setAutomatic] = useState(release.automatic);
  const [lead, setLead] = useState(release.lead_minutes);
  return (
    <section className="panel form-stack">
      <div className="section-heading">
        <h2>Programación de preparación · #{release.folio}</h2>
        <button className="secondary-button" onClick={onClose}>
          Cerrar programación
        </button>
      </div>
      <Notice>
        {current?.revision !== release.revision &&
          'El pedido cambió. Reabre la programación para revisar su estado vigente.'}
      </Notice>
      <Form
        disabled={busy || current?.revision !== release.revision}
        label="Guardar programación"
        onSave={async () => {
          await execute('kitchen_schedule', {
            id: release.id,
            body: {
              revision: release.revision,
              scheduled_local: date,
              automatic,
              lead_minutes: lead,
            },
          });
          onClose();
        }}
      >
        <TextField
          label={`Fecha y hora programada · ${timezone}`}
          type="datetime-local"
          required
          value={date}
          onChange={setDate}
        />
        <IntegerField
          label="Anticipación en minutos"
          min={0}
          max={1440}
          value={lead}
          onChange={setLead}
        />
        <Check label="Liberación automática desde Go" value={automatic} onChange={setAutomatic} />
        <p className="muted">
          Esta operación ajusta preparación; conserva cantidades, precios y la hora comercial del
          pedido.
        </p>
      </Form>
    </section>
  );
}
