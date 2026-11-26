import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Notice, PageTitle } from '../../components/Forms';
import { useLiveResource } from '../../hooks/useLiveResource';
import { useRestaurantMutation } from '../../hooks/useRestaurantMutation';
import { APIError } from '../../services/api';
import { notifyError, notifySuccess } from '../../services/notify';
import { jobLabels, preparationLabels, type ExecuteKitchen } from '../../services/kitchen';
import { serverDateTime } from '../../services/restaurant';
import { KitchenSettingsForm } from './KitchenSettingsForm';
import { KitchenTickets } from './KitchenTickets';
import { PreparationBoard } from './PreparationBoard';
import { PrintQueue } from './PrintQueue';
import { PrintersAdmin } from './PrintersAdmin';
import { RoutingAdmin } from './RoutingAdmin';
import { StationsAdmin } from './StationsAdmin';
import { PreparationSeat } from './PreparationSeat';
import { takeKitchenViewRequest } from './kitchenViewRequest';

export function KitchenPage({
  can,
  timezone,
  connectionLost = false,
  onRetryConnection,
}: {
  can: (key: string) => boolean;
  timezone: string;
  // Etapa 9F (§5.4): Preparación se actualiza sola; «Reintentar» solo cuando la barra indica «Sin
  // conexión» (como Mesas desde la 9D).
  connectionLost?: boolean;
  onRetryConnection?: () => void;
}) {
  // Etapa K1.1 (§4.2): el diagnóstico de una pantalla de cocina en Terminales puede pedir abrir otra pestaña.
  const [view, setView] = useState<string>(() => takeKitchenViewRequest() ?? 'preparation');
  const [queueVersion, setQueueVersion] = useState(0);
  // El pendiente de cocina vive en el almacén global (clave kitchen): cambiar de pantalla y volver
  // conserva el mismo operation_id para recuperar el resultado.
  const mutation = useRestaurantMutation((operation) => {
    window.dispatchEvent(new CustomEvent('adel-server-change'));
    if (
      ['kitchen_reprint', 'kitchen_rerender', 'kitchen_retry_job', 'kitchen_resolve_job'].includes(
        operation,
      )
    )
      setQueueVersion((version) => version + 1);
  }, 'kitchen');
  // Toda escritura de cocina pasa por aquí: un rechazo de Go se muestra como aviso persistente
  // (sistema único de mensajes); un resultado incierto no, porque queda en el panel de
  // recuperación con el mismo operation_id. Cada pantalla confirma su propio éxito.
  const execute: ExecuteKitchen = async (operation, options) => {
    try {
      return await mutation.execute(operation, options);
    } catch (problem) {
      if (problem instanceof APIError && problem.status === 409)
        window.dispatchEvent(new CustomEvent('adel-server-change'));
      if (!uncertainResult(problem)) {
        notifyError(problem, 'No se pudo completar la operación de cocina.');
      }
      throw problem;
    }
  };

  async function recover() {
    const problem = await mutation.retry();
    if (!problem) {
      notifySuccess('Operación de cocina recuperada.');
      return;
    }
    if (!uncertainResult(problem)) {
      notifyError(problem, 'No se pudo recuperar la operación de cocina.');
    }
  }
  const busy = mutation.busy || mutation.uncertain;
  const tabs = [
    { id: 'preparation', label: 'Preparación', allowed: can('kitchen.states.read') },
    { id: 'tickets', label: 'Comandas', allowed: can('kitchen.tickets.read') },
    {
      id: 'queue',
      label: 'Cola de impresión',
      allowed: can('kitchen.queue.read') && can('kitchen.stations.all'),
    },
    { id: 'stations', label: 'Estaciones', allowed: can('kitchen.stations.read') },
    { id: 'routing', label: 'Enrutamiento', allowed: can('kitchen.routing.manage') },
    { id: 'printers', label: 'Impresoras', allowed: can('kitchen.printers.manage') },
    {
      id: 'settings',
      label: 'Comandas y tiempos',
      allowed: can('kitchen.formats.manage') || can('kitchen.timing.manage'),
    },
    {
      id: 'history',
      label: 'Historial',
      allowed: can('kitchen.history.read') && can('kitchen.stations.all'),
    },
  ].filter((tab) => tab.allowed);
  const selected = tabs.some((tab) => tab.id === view) ? view : tabs[0]?.id;
  return (
    <div className="kitchen-page form-stack">
      <PageTitle
        title="Cocina y preparación"
        description="Comandas, estaciones e impresión. Preparación, pago y entrega son procesos independientes."
      />
      <div className="kitchen-navigation" role="group" aria-label="Funciones de cocina">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            className={selected === tab.id ? 'primary-button' : 'secondary-button'}
            aria-pressed={selected === tab.id}
            onClick={() => setView(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </div>
      {mutation.uncertain && (
        <div className="operation-recovery">
          <p>El resultado no pudo confirmarse. Recupera la misma operación antes de continuar.</p>
          <button
            className="primary-button"
            disabled={mutation.busy}
            onClick={() => void recover()}
          >
            Recuperar operación de cocina
          </button>
        </div>
      )}
      {selected === 'preparation' && (
        // Etapa L1 (Fase E): Preparación ocupa el cupo de pantalla de cocina mientras está abierta.
        <PreparationSeat>
          <PreparationBoard
            can={can}
            execute={execute}
            busy={busy}
            connectionLost={connectionLost}
            onRetry={onRetryConnection}
          />
        </PreparationSeat>
      )}
      {selected === 'tickets' && <KitchenTickets timezone={timezone} />}
      {selected === 'queue' && (
        <PrintQueue
          key={queueVersion}
          can={can}
          execute={execute}
          busy={busy}
          timezone={timezone}
        />
      )}
      {selected === 'stations' && <StationsAdmin can={can} execute={execute} busy={busy} />}
      {selected === 'routing' && <RoutingAdmin can={can} execute={execute} busy={busy} />}
      {selected === 'printers' && <PrintersAdmin can={can} execute={execute} busy={busy} />}
      {selected === 'settings' && <KitchenSettingsForm can={can} execute={execute} busy={busy} />}
      {selected === 'history' && <KitchenHistory timezone={timezone} />}
    </div>
  );
}

// Sin conexión o error ≥ 500: Go pudo haber registrado la operación; se recupera con el mismo
// identificador en lugar de mostrarla como rechazada.
function uncertainResult(problem: unknown) {
  return (
    problem instanceof APIError && (problem.code === 'CONNECTION_ERROR' || problem.status >= 500)
  );
}

function KitchenHistory({ timezone }: { timezone: string }) {
  const [page, setPage] = useState(1);
  const resource = useLiveResource<
    {
      id: string;
      kind: string;
      actor_name: string;
      device_name: string;
      created_at: string;
      after_value?: { reason?: string; state?: string; message?: string };
    }[]
  >('kitchen_history', { query: { page: String(page) } });
  return (
    <section className="panel form-stack">
      <h2>Historial de cocina e impresión</h2>
      <Notice>{resource.error}</Notice>
      {resource.data?.map((event) => (
        <Accordion
          key={event.id}
          title={historyLabels[event.kind] ?? 'Cambio de cocina'}
          summary={`${serverDateTime(event.created_at, timezone)} · ${event.actor_name}`}
        >
          <p>Terminal: {event.device_name}</p>
          {event.after_value?.reason && (
            <p>Motivo: {historyReason(event.kind, event.after_value.reason)}</p>
          )}
          {event.after_value?.state && (
            <p>
              Estado:{' '}
              {preparationLabels[event.after_value.state] ??
                jobLabels[event.after_value.state as keyof typeof jobLabels] ??
                event.after_value.state}
            </p>
          )}
          {event.after_value?.message && <p>{event.after_value.message}</p>}
        </Accordion>
      ))}
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
          disabled={(resource.data?.length ?? 0) < 100}
          onClick={() => setPage(page + 1)}
        >
          Más antiguos
        </button>
      </div>
    </section>
  );
}

const historyLabels: Record<string, string> = {
  'station.saved': 'Estación guardada',
  'station.access.saved': 'Estaciones de la terminal actualizadas',
  'printer.saved': 'Impresora guardada',
  'route.saved': 'Enrutamiento guardado',
  'settings.saved': 'Formato de comandas actualizado',
  'timing.saved': 'Programación y seguimiento actualizados',
  'release.release': 'Liberación manual revisada',
  'release.schedule': 'Programación del pedido actualizada',
  'release.automatic': 'Liberación automática revisada',
  'states.changed': 'Preparación actualizada',
  'change.retried': 'Modificación pendiente revisada',
  'kds.acknowledged': 'Recepción en pantalla de cocina confirmada',
  'print.test_requested': 'Prueba de impresión solicitada',
  'print.submitted': 'Documento enviado a la impresora',
  'print.error': 'Error de impresión confirmado',
  'print.intervention': 'Impresión requiere intervención',
  'print.uncertain': 'Resultado de impresión incierto',
  'print.recovered_uncertain': 'Trabajo interrumpido recuperado para revisión',
  'print.redirected': 'Trabajo enviado al respaldo configurado',
  'print.retry': 'Reintento de impresión autorizado',
  'print.resolve': 'Resultado de impresión resuelto',
  'print.reprinted': 'Copia adicional solicitada',
  // Etapa K1.1 (K11-D3 y K11-D4): lo que quedó sin enviar de una cuenta que ya no está abierta se da por terminado
  // (por la actualización a la 0.17.0, al cobrar y cerrar o al cancelar la cuenta) y el ajuste «Productos sin ruta».
  'release.dismissed': 'Pedido sin enviar dado por terminado',
  'change.dismissed': 'Aviso sin entregar dado por terminado',
  'unrouted.saved': 'Productos sin ruta actualizado',
};

// Motivos que escribe Go al dar por terminado un envío o un aviso de una cuenta que ya no está abierta (K11-D4). Los
// demás motivos los escribe una persona (por ejemplo, al reimprimir) y se muestran como están.
const dismissedReasons: Record<string, string> = {
  account_not_open: 'La cuenta ya no estaba abierta.',
  account_closed: 'La cuenta se cobró y se cerró.',
  account_cancelled: 'La cuenta se canceló.',
};

export function historyReason(kind: string, reason: string) {
  if (kind === 'release.dismissed' || kind === 'change.dismissed') {
    return dismissedReasons[reason] ?? 'La cuenta ya no estaba abierta.';
  }
  return reason;
}
