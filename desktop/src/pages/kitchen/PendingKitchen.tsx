import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { api } from '../../services/api';
import type { Board, ExecuteKitchen, Release } from '../../services/kitchen';
import { notifySuccess, notifyWarning } from '../../services/notify';
import { countLabel } from '../../services/plural';

// Etapa K1.1 (§3, K11-D8): el panel «Por enviar a cocina» de Preparación, con un servidor que anuncia
// kitchen-coverage-v1. Con la regla nueva un envío ya no se detiene por un producto sin ruta, así que aquí solo
// quedan los pedidos programados, los que esperan a que alguien los mande, los detenidos por una versión anterior
// (de cuentas todavía abiertas) y los avisos que no llegaron a una estación desactivada. Go solo los entrega con
// kitchen.stations.all; los de una cuenta cobrada, cerrada o cancelada ya no vienen.

type PendingChange = Board['pending_changes'][number];

// Día y hora en la zona del restaurante: «el 26/11/26 a las 1:30 p.m.» (o «a la 1:…»).
export function serverDayAndTime(value: string, timezone: string) {
  const day = new Intl.DateTimeFormat('es-MX', { dateStyle: 'short', timeZone: timezone }).format(
    new Date(value),
  );
  const time = new Intl.DateTimeFormat('es-MX', { timeStyle: 'short', timeZone: timezone }).format(
    new Date(value),
  );
  return `el ${day} ${/^1:/.test(time) ? 'a la' : 'a las'} ${time}`;
}

// Termina una oración sin repetir el punto de «p.m.».
function sentence(text: string) {
  return text.endsWith('.') ? text : `${text}.`;
}

// Qué pasa con un pedido, en lenguaje sencillo: un título y una línea que lo explica (K11-D8 y K11-D9).
export function releaseCase(release: Release, timezone: string) {
  if (release.state === 'scheduled') {
    // Automático: llega a cocina a su hora de envío (la hora programada menos la anticipación). Manual: se dice la
    // hora programada y que espera a que alguien lo mande.
    if (release.automatic) {
      const when = release.due_at ?? release.scheduled_at;
      return {
        title: 'Programado: se envía solo a su hora',
        detail: when
          ? sentence(`Se envía a cocina solo ${serverDayAndTime(when, timezone)}`)
          : 'Se envía a cocina solo a su hora.',
      };
    }
    const when = release.scheduled_at ?? release.due_at;
    return {
      title: 'Programado: espera a que lo envíes',
      detail: when
        ? `${sentence(`Programado para ${serverDayAndTime(when, timezone)}`)} Espera a que lo envíes a cocina.`
        : 'Espera a que lo envíes a cocina.',
    };
  }
  if (release.state === 'blocked') {
    // Solo los de antes de la 0.17.0: un envío nuevo ya no se detiene. Sin los renglones «Sin ruta de preparación».
    return {
      title: 'La cocina no recibió este pedido',
      detail:
        'Se envió con la versión anterior, que lo detenía por un producto sin ruta o una estación desactivada.',
    };
  }
  // Un pedido al que le quitaron la hora con la 0.17.0 (K11-D9): la cocina no lo recibe hasta que alguien lo envía.
  // Desde la etapa M1 (M1-D17) quitar la hora lo envía en el momento, así que solo quedan así los de antes.
  return {
    title: 'Espera a que lo envíes a cocina',
    detail: 'La cocina no lo recibe hasta que toques «Enviar a cocina ahora».',
  };
}

// Qué dice el aviso de un cambio que no llegó a la cocina (K11-D2). Con las estaciones desactivadas que informa Go
// (inactive_stations, contrato §6): «No se pudo avisar a X: está desactivada.» y cómo corregirlo. Sin ese dato (un
// servidor que no lo trae), las advertencias de Go tal cual: el escritorio no interpreta su texto.
export function pendingChangeLines(change: PendingChange) {
  const stations = change.inactive_stations ?? [];
  if (stations.length > 0) {
    return {
      lines: stations.map((station) => `No se pudo avisar a ${station.name}: está desactivada.`),
      help:
        stations.length === 1
          ? 'Actívala en Cocina › Estaciones y vuelve a avisar.'
          : 'Actívalas en Cocina › Estaciones y vuelve a avisar.',
    };
  }
  return {
    lines: change.warnings.filter((warning, index, all) => all.indexOf(warning) === index),
    help: 'Revisa la configuración de cocina y vuelve a avisar.',
  };
}

export function pendingChangeTitle(change: PendingChange) {
  // Etapa 9D: «Cancelar cuenta» (account.cancelled) también es una cancelación.
  const cancellation = change.kind === 'item.cancelled' || change.kind === 'account.cancelled';
  return cancellation
    ? 'La cocina no recibió una cancelación'
    : 'La cocina no recibió una modificación';
}

export function PendingKitchen({
  board,
  can,
  execute,
  busy,
  changing,
  onSchedule,
  onChanged,
}: {
  board: Board;
  can: (key: string) => boolean;
  execute: ExecuteKitchen;
  busy: boolean;
  changing: boolean;
  onSchedule: (release: Release) => void;
  onChanged: () => Promise<void>;
}) {
  const [sending, setSending] = useState('');
  const manage = can('kitchen.stations.all');
  const locked = busy || changing || sending !== '';

  // «Enviar a cocina ahora»: el éxito solo se dice si Go responde que el pedido quedó enviado. Un 409 del
  // contrato (KITCHEN_RELEASE_NOT_PENDING: ya se envió o su cuenta ya no está abierta) lo avisa el controlador
  // común con el mensaje de Go; aquí se vuelve a leer el panel.
  async function send(release: Release) {
    setSending(release.id);
    try {
      const saved = await execute<Release>('kitchen_release', {
        id: release.id,
        body: { revision: release.revision },
      });
      if (saved?.state === 'released') {
        notifySuccess(`Pedido #${release.folio} enviado a cocina.`);
      } else if (saved?.state === 'cancelled') {
        notifyWarning(`El pedido #${release.folio} ya no tenía nada que enviar a cocina.`);
      } else {
        notifyWarning(`La cocina no recibió el pedido #${release.folio}: sigue pendiente.`);
      }
    } catch {
      // El controlador común ya mostró el rechazo o dejó el resultado incierto para recuperarlo.
    } finally {
      setSending('');
      await onChanged();
    }
  }

  // «Volver a avisar»: Go responde lo mismo aunque la estación siga desactivada; se vuelve a leer el panel para
  // decir si el aviso salió o sigue pendiente. Un aviso ya terminado responde 409 (KITCHEN_CHANGE_NOT_PENDING).
  async function retry(change: PendingChange) {
    setSending(change.event_id);
    try {
      await execute('kitchen_retry_change', {
        id: change.event_id,
        body: { revision: change.revision },
      });
      const fresh = await api<Board>('kitchen_board', {
        query: { station_id: '', state: '', page: '1' },
      });
      if (fresh.pending_changes.some((entry) => entry.event_id === change.event_id)) {
        notifyWarning('La estación sigue desactivada: el aviso no llegó.');
      } else {
        notifySuccess('Aviso enviado a cocina.');
      }
    } catch {
      // El controlador común ya mostró el rechazo o dejó el resultado incierto para recuperarlo.
    } finally {
      setSending('');
      await onChanged();
    }
  }

  return (
    <Accordion
      title="Por enviar a cocina"
      summary={`${countLabel(board.releases.length, 'pedido', 'pedidos')} · ${countLabel(
        board.pending_changes.length,
        'aviso',
        'avisos',
      )}`}
      defaultOpen
    >
      <div className="kitchen-board-grid">
        {board.releases.map((release) => {
          const early =
            !!release.due_at &&
            new Date(release.due_at).getTime() > new Date(board.server_time).getTime();
          const earlyDenied = early && !can('kitchen.release_early');
          const explained = releaseCase(release, board.timezone);
          return (
            <article
              className="kitchen-preparation-card kitchen-pending-card"
              key={release.id}
              aria-label={`Pedido #${release.folio}`}
            >
              <strong>
                #{release.folio} · {release.account_name}
              </strong>
              <p className="kitchen-pending-title">{explained.title}</p>
              <p className="muted">{explained.detail}</p>
              <div className="kitchen-pending-actions">
                {can('kitchen.release') && manage && (
                  <div className="kitchen-pending-action">
                    <button
                      className="primary-button"
                      disabled={locked || earlyDenied}
                      onClick={() => void send(release)}
                    >
                      Enviar a cocina ahora
                    </button>
                    <small className="muted">
                      {earlyDenied
                        ? 'Para enviarlo antes de su hora hace falta el permiso de enviar a cocina antes de tiempo.'
                        : early
                          ? 'La cocina lo recibe al momento, antes de su hora.'
                          : 'La cocina lo recibe al momento.'}
                    </small>
                  </div>
                )}
                {release.state === 'scheduled' && can('kitchen.timing.manage') && manage && (
                  <div className="kitchen-pending-action">
                    <button
                      className="secondary-button"
                      disabled={locked}
                      onClick={() => onSchedule(release)}
                    >
                      Cambiar la hora
                    </button>
                    <small className="muted">
                      Cambia cuándo llega a cocina. No cambia el pedido ni su precio.
                    </small>
                  </div>
                )}
              </div>
            </article>
          );
        })}
        {board.pending_changes.map((change) => (
          <article
            className="kitchen-preparation-card kitchen-pending-card"
            key={change.event_id}
            aria-label={pendingChangeTitle(change)}
          >
            <p className="kitchen-pending-title">{pendingChangeTitle(change)}</p>
            {pendingChangeLines(change).lines.map((line) => (
              <p key={line}>{line}</p>
            ))}
            <p className="muted">{pendingChangeLines(change).help}</p>
            {can('kitchen.release') && manage && (
              <div className="kitchen-pending-action">
                <button
                  className="secondary-button"
                  disabled={locked}
                  onClick={() => void retry(change)}
                >
                  Volver a avisar
                </button>
                <small className="muted">Manda el cambio a la estación si ya está activa.</small>
              </div>
            )}
          </article>
        ))}
      </div>
    </Accordion>
  );
}
