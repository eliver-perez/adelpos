import { Accordion } from '../components/Accordion';
import { precheckName } from '../services/documentNames';
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

// Etapa 9D (§5.1): nombre en palabras del restaurante de las operaciones de «Cancelar cuenta» y de
// su catálogo y umbral. Se muestra sobre el código, que sigue siendo el que se filtra.
const actionLabels: Record<string, string> = {
  'restaurant.account.cancelled': 'Cuenta cancelada',
  // Registros anteriores a la etapa 9F: la ruta de esta anulación se retiró.
  'restaurant.account.void': 'Cuenta vacía cancelada con motivo libre',
  'restaurant.account.discarded': 'Cuenta vacía descartada',
  'restaurant.account.released': 'Cuenta vacía liberada',
  'restaurant.supervisor_authorized': 'Autorización de supervisor',
  'restaurant.cancel_reason_created': 'Motivo de cancelación creado',
  'restaurant.cancel_reason_updated': 'Motivo de cancelación modificado',
  'restaurant.cancel_reasons_reordered': 'Motivos de cancelación reordenados',
  'settings.long_open_hours_changed': 'Umbral de cuenta abierta cambiado',
  'inventory.cancellation_preclassified': 'Cancelación preclasificada como merma',
  // Etapa 9F: «Quitar datos de prueba» (también el intento sin respaldo, con resultado fallido).
  'maintenance.test_data_removed': 'Datos de prueba quitados',
  // Etapa 9G (fase A): reinicio del servicio del servidor desde Mantenimiento.
  'maintenance.service_restarted': 'Servidor reiniciado',
  // Etapa 9G (fase B): emparejamiento y administración de terminales.
  'device.pairing_requested': 'Terminal nueva pidió su código',
  'device.pairing_approved': 'Terminal aprobada',
  'device.pairing_code_failed': 'Código de terminal incorrecto',
  'device.pairing_rejected': 'Terminal rechazada',
  'device.pairing_cancelled': 'Terminal canceló su solicitud',
  'device.paired': 'Terminal emparejada',
  'device.updated': 'Terminal modificada',
  'device.revoked': 'Terminal revocada',
  // Etapa 9F (fase B): impresoras (corte de papel incluido), reintentos sin motivo y reimpresiones con
  // motivo, y la configuración de cobro.
  'kitchen.printer.saved': 'Impresora guardada',
  'kitchen.print.retry': 'Reintento de impresión',
  'kitchen.print.resolve': 'Resultado de impresión resuelto',
  'kitchen.print.reprinted': 'Copia adicional solicitada',
  'payments.settings_changed': 'Políticas de cobro cambiadas',
  // Con el nombre visible que eligió el restaurante (revisión 9F, M3), que desde entonces se elige en
  // Administración › Documentos.
  get 'payments.precheck_printed'() {
    return `${precheckName().label} ${precheckName().printedAdjective}`;
  },
  'documents.precheck_name_changed': 'Nombre de la precuenta cambiado',
  // Revisión 9F (B2): otra copia con la plantilla vigente, con motivo.
  'payments.document_rerendered': 'Documento impreso con la plantilla vigente',
  'cash.document_rerendered': 'Documento de caja impreso con la plantilla vigente',
  'payments.receipt_printed': 'Ticket de pago impreso después del pago',
  'payments.document_retried': 'Reintento de impresión de un documento',
  'payments.document_reprinted': 'Reimpresión de un documento',
  'cash.document_retried': 'Reintento de impresión de un documento de caja',
  'cash.document_reprinted': 'Reimpresión de un documento de caja',
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
          <Field label="Código de la operación" hint="Tal como aparece en la columna Operación.">
            <input
              placeholder="Por ejemplo: user.updated"
              value={action}
              onChange={(event) => setAction(event.target.value)}
            />
          </Field>
          <Field
            label="Código de la terminal"
            hint="El que aparece bajo su nombre en Sistema → Terminales."
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
                  <small className="muted block">
                    {actorKinds[event.actor_kind] ?? event.actor_kind}
                  </small>
                </td>
                <td>
                  {actionLabels[event.action] && (
                    <span className="block">{actionLabels[event.action]}</span>
                  )}
                  <span className="font-mono text-xs">{event.action}</span>
                  <small className="muted block">{event.entity_type}</small>
                </td>
                <td>{event.success ? 'Correcto' : 'Rechazado'}</td>
                <td>
                  <Accordion title="Ver cambios">
                    <dl className="audit-details">
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
