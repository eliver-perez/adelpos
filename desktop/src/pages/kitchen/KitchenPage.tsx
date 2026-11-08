import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Notice, PageTitle } from '../../components/Forms';
import { useLiveResource } from '../../hooks/useLiveResource';
import { useRestaurantMutation } from '../../hooks/useRestaurantMutation';
import { APIError } from '../../services/api';
import { jobLabels, preparationLabels, type ExecuteKitchen } from '../../services/kitchen';
import { serverDateTime } from '../../services/restaurant';
import { KitchenSettingsForm } from './KitchenSettingsForm';
import { KitchenTickets } from './KitchenTickets';
import { PreparationBoard } from './PreparationBoard';
import { PrintQueue } from './PrintQueue';
import { PrintersAdmin } from './PrintersAdmin';
import { RoutingAdmin } from './RoutingAdmin';
import { StationsAdmin } from './StationsAdmin';

export function KitchenPage({
  can,
  timezone,
}: {
  can: (key: string) => boolean;
  timezone: string;
}) {
  const [view, setView] = useState('preparation');
  const [queueVersion, setQueueVersion] = useState(0);
  const mutation = useRestaurantMutation((operation) => {
    window.dispatchEvent(new CustomEvent('adel-server-change'));
    if (['kitchen_reprint', 'kitchen_retry_job', 'kitchen_resolve_job'].includes(operation))
      setQueueVersion((version) => version + 1);
  });
  const execute: ExecuteKitchen = async (operation, options) => {
    try {
      return await mutation.execute(operation, options);
    } catch (problem) {
      if (problem instanceof APIError && problem.status === 409)
        window.dispatchEvent(new CustomEvent('adel-server-change'));
      throw problem;
    }
  };
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
        description="Comandas, estaciones y colas persistentes. Preparación, pago y entrega son procesos independientes."
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
      <Notice>{mutation.error}</Notice>
      {mutation.uncertain && (
        <div className="operation-recovery">
          <p>El resultado no pudo confirmarse. Recupera la misma operación antes de continuar.</p>
          <button
            className="primary-button"
            disabled={mutation.busy}
            onClick={() => void mutation.retry()}
          >
            Recuperar operación de cocina
          </button>
        </div>
      )}
      {selected === 'preparation' && <PreparationBoard can={can} execute={execute} busy={busy} />}
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
      {selected === 'routing' && <RoutingAdmin execute={execute} busy={busy} />}
      {selected === 'printers' && <PrintersAdmin can={can} execute={execute} busy={busy} />}
      {selected === 'settings' && <KitchenSettingsForm can={can} execute={execute} busy={busy} />}
      {selected === 'history' && <KitchenHistory timezone={timezone} />}
    </div>
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
          {event.after_value?.reason && <p>Motivo: {event.after_value.reason}</p>}
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
  'station.access.saved': 'Acceso de terminal actualizado',
  'printer.saved': 'Impresora guardada',
  'route.saved': 'Enrutamiento guardado',
  'settings.saved': 'Formato de comandas actualizado',
  'timing.saved': 'Programación y seguimiento actualizados',
  'release.release': 'Liberación manual revisada',
  'release.schedule': 'Programación del pedido actualizada',
  'release.automatic': 'Liberación automática revisada',
  'states.changed': 'Preparación actualizada',
  'change.retried': 'Modificación pendiente revisada',
  'kds.acknowledged': 'Recepción KDS confirmada',
  'print.test_requested': 'Prueba de impresión solicitada',
  'print.submitted': 'Documento aceptado por el transporte',
  'print.error': 'Error de impresión confirmado',
  'print.intervention': 'Impresión requiere intervención',
  'print.uncertain': 'Resultado de impresión incierto',
  'print.recovered_uncertain': 'Trabajo interrumpido recuperado para revisión',
  'print.redirected': 'Trabajo enviado al respaldo configurado',
  'print.retry': 'Reintento de impresión autorizado',
  'print.resolve': 'Resultado de impresión resuelto',
  'print.reprinted': 'Copia adicional solicitada',
};
