import { useEffect, useRef, useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { Board, ExecuteKitchen, Release, Task } from '../../services/kitchen';
import {
  notifySoundBlocked,
  playArrivalChime,
  unlockKitchenSound,
} from '../../services/kitchenSound';
import { notifySuccess } from '../../services/notify';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { Select } from '../../components/Select';
import { PreparationTaskCard } from './PreparationTaskCard';
import { ScheduleEditor } from './ScheduleEditor';
import { newArrivals, noCommonStationsMessage } from './preparationModel';

// Preparación en modo cocina (etapa 9F, §5.4): tarjetas legibles a distancia con botones grandes, color
// por tiempo contra el objetivo (con texto), más antiguas primero, solo las estaciones visibles en esta
// terminal para el usuario (Go calcula la intersección) y aviso sonoro opcional de la terminal. La
// pantalla se actualiza sola (sondeo de 5 s y eventos en tiempo real): no tiene «Actualizar» y
// «Reintentar» solo aparece cuando la barra indica «Sin conexión», como Mesas.
export function PreparationBoard({
  can,
  execute,
  busy,
  connectionLost = false,
  onRetry,
}: {
  can: (key: string) => boolean;
  execute: ExecuteKitchen;
  busy: boolean;
  connectionLost?: boolean;
  onRetry?: () => void;
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
  useArrivalChime(board, `${station}|${state}|${page}`);
  useSoundUnlock(!!board?.sound_alert);
  const grouped = new Map<string, Task[]>();
  for (const task of board?.tasks ?? []) {
    const key = `${task.release_id}/${task.station_id}/${task.instructions.item_id}/${task.instructions.version_id}/${task.state}`;
    grouped.set(key, [...(grouped.get(key) ?? []), task]);
  }
  // Sin estaciones en común entre la terminal y el usuario, Go lo informa: se explica en lugar de
  // mostrar una pantalla vacía. Solo cuenta lo que Go dice expresamente (common: false).
  const noCommonStations = board?.station_scope?.common === false;

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
      notifySuccess(
        next === 'ready'
          ? `${first.instructions.name} · listo en ${first.station_name}`
          : `${first.instructions.name} · en preparación`,
      );
      // Conservar el bloqueo hasta recibir las revisiones nuevas evita que un segundo
      // toque rápido envíe el siguiente avance con la revisión anterior.
      await resource.reload();
    } catch {
      // El controlador común muestra el rechazo o conserva el resultado incierto para recuperarlo.
    } finally {
      setChanging(false);
    }
  }

  // «Reintentar» vuelve a consultar el servidor y la preparación.
  function retry() {
    onRetry?.();
    void resource.reload();
  }

  return (
    <div className="form-stack kitchen-board">
      <div className="kitchen-filters">
        <Field label="Estación de preparación">
          <Select
            value={station}
            options={[
              { value: '', label: 'Todas mis estaciones' },
              ...(board?.stations ?? []).map((entry) => ({ value: entry.id, label: entry.name })),
            ]}
            onChange={(next) => {
              setStation(next);
              setPage(1);
            }}
          />
        </Field>
        <Field label="Avance de preparación">
          <Select
            value={state}
            options={[
              { value: '', label: 'Pendientes y en preparación' },
              { value: 'ready', label: 'Listos' },
              { value: 'all', label: 'Todos los estados' },
            ]}
            onChange={(next) => {
              setState(next);
              setPage(1);
            }}
          />
        </Field>
        {connectionLost && (
          <button className="secondary-button kitchen-retry" onClick={retry}>
            <IconLabel icon="refresh">Reintentar</IconLabel>
          </button>
        )}
      </div>
      <Notice>{resource.error}</Notice>
      {board && noCommonStations && (
        <section
          className="kitchen-scope-notice"
          role="status"
          aria-label="Sin estaciones en común"
        >
          <strong>Sin estaciones en común</strong>
          <p>{noCommonStationsMessage(board.station_scope)}</p>
        </section>
      )}
      {board && (board.releases.length > 0 || board.pending_changes.length > 0) && (
        <PendingReleases
          board={board}
          can={can}
          execute={execute}
          busy={busy}
          changing={changing}
          onSchedule={setSchedule}
        />
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
      {!noCommonStations && (
        <>
          <div className="section-heading">
            <h2>Preparación por estación</h2>
            <span className="muted text-sm">Más antiguos primero</span>
          </div>
          <div className="kitchen-board-grid kitchen-task-grid">
            {Array.from(grouped.entries()).map(([key, tasks]) => (
              <PreparationTaskCard
                key={key}
                tasks={tasks}
                canManage={can('kitchen.states.manage')}
                disabled={busy || changing}
                onAdvance={(next) => void advance(tasks, next)}
              />
            ))}
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
        </>
      )}
    </div>
  );
}

// Pedidos pendientes de liberación y avisos de modificación o cancelación por revisar. Son
// administrativos: Go solo los entrega con kitchen.stations.all, sin depender de la terminal.
function PendingReleases({
  board,
  can,
  execute,
  busy,
  changing,
  onSchedule,
}: {
  board: Board;
  can: (key: string) => boolean;
  execute: ExecuteKitchen;
  busy: boolean;
  changing: boolean;
  onSchedule: (release: Release) => void;
}) {
  return (
    <Accordion
      title="Pendientes de liberación y avisos"
      summary={`${countLabel(board.releases.length, 'pedido', 'pedidos')} · ${countLabel(
        board.pending_changes.length,
        'cambio por revisar',
        'cambios por revisar',
      )}`}
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
                  Programado: {serverDateTime(release.scheduled_at, board.timezone)}
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
                      })
                        .then(() =>
                          notifySuccess(`Pedido #${release.folio} liberado a preparación`),
                        )
                        .catch(() => undefined)
                    }
                  >
                    {early ? 'Liberar anticipadamente' : 'Liberar a preparación'}
                  </button>
                )}
                {can('kitchen.timing.manage') && can('kitchen.stations.all') && (
                  <button className="secondary-button" onClick={() => onSchedule(release)}>
                    Ajustar programación
                  </button>
                )}
              </div>
            </article>
          );
        })}
        {board.pending_changes.map((change) => (
          <article className="kitchen-preparation-card" key={change.event_id}>
            {/* Etapa 9D: «Cancelar cuenta» (account.cancelled) también es una cancelación. */}
            <strong>
              Aviso de{' '}
              {change.kind === 'item.cancelled' || change.kind === 'account.cancelled'
                ? 'cancelación'
                : 'modificación'}{' '}
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
                  })
                    .then(() => notifySuccess('Destinos revisados y aviso generado'))
                    .catch(() => undefined)
                }
              >
                Revisar destinos y generar aviso
              </button>
            )}
          </article>
        ))}
      </div>
    </Accordion>
  );
}

// Aviso sonoro opcional de la terminal (etapa 9F): suena cuando llega una comanda nueva de las
// estaciones visibles. Lo que ya estaba al abrir la pantalla o al cambiar de filtro, página o estaciones
// visibles es la base y no suena; tampoco los cambios de estado de tareas ya vistas ni, sin el aviso
// activado en la terminal, nada.
function useArrivalChime(board: Board | null, filterKey: string) {
  const seen = useRef<{ key: string; releases: Set<string> } | null>(null);
  useEffect(() => {
    if (!board) {
      return;
    }
    const key = `${filterKey}|${JSON.stringify(board.station_scope)}`;
    const current = seen.current;
    if (!current || current.key !== key || current.releases.size > 5000) {
      seen.current = { key, releases: new Set(board.tasks.map((task) => task.release_id)) };
      return;
    }
    const arrivals = newArrivals(current.releases, board.tasks);
    for (const task of board.tasks) {
      current.releases.add(task.release_id);
    }
    if (board.sound_alert && arrivals.size > 0) {
      // Revisión 9F (parte 3, B2): si el equipo todavía no permite reproducir (nadie tocó la pantalla), se
      // avisa una sola vez en lugar de callar el fallo.
      void playArrivalChime().then((played) => {
        if (!played) {
          notifySoundBlocked();
        }
      });
    }
  }, [board, filterKey]);
}

// Con el aviso sonoro activado en la terminal, el primer toque o tecla en la pantalla crea o reanuda el
// contexto de audio (revisión 9F, parte 3, B2): WebKit y WebView2 solo lo dejan sonar si se hace durante
// un gesto del usuario. Se escucha en la fase de captura para no depender de qué control se toque.
function useSoundUnlock(enabled: boolean) {
  useEffect(() => {
    if (!enabled) {
      return;
    }
    document.addEventListener('pointerdown', unlockKitchenSound, true);
    document.addEventListener('keydown', unlockKitchenSound, true);
    return () => {
      document.removeEventListener('pointerdown', unlockKitchenSound, true);
      document.removeEventListener('keydown', unlockKitchenSound, true);
    };
  }, [enabled]);
}
