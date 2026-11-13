import { useState } from 'react';
import { Form, Notice } from '../../components/Forms';
import { Check, IntegerField } from '../../components/catalog/EditorFields';
import { DateTimeField } from '../../components/TimeField';
import { zonedInput } from '../../services/kitchen';
import type { ExecuteKitchen, Release } from '../../services/kitchen';

// Programación de un pedido pendiente de liberación (administrativa: kitchen.timing.manage y
// kitchen.stations.all). Se abre desde «Pendientes de liberación y avisos» de Preparación.
export function ScheduleEditor({
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
        successMessage={`Programación del pedido #${release.folio} guardada.`}
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
        {/* Etapa 9C (§2.1): fecha dd/mm/aaaa y hora con el selector, de 5 en 5. Go recibe el mismo
            «AAAA-MM-DDTHH:MM» en la zona del establecimiento. */}
        <DateTimeField
          label="Fecha y hora programada"
          hint="En la zona horaria del restaurante."
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
        <Check
          label="Liberar a cocina automáticamente a su hora"
          value={automatic}
          onChange={setAutomatic}
        />
        <p className="muted">
          Esta operación ajusta preparación; conserva cantidades, precios y la hora comercial del
          pedido.
        </p>
      </Form>
    </section>
  );
}
