import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Form, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { SegmentedControl } from '../../components/SegmentedControl';
import { ReferenceField, TextField } from '../../components/catalog/EditorFields';
import type { Lookups } from '../../services/catalog';
import { accountCustomName, takeawayOrigins } from '../../services/restaurant';
import type { Account, AccountInput, Bill } from '../../services/restaurant';
import type { ExecuteMutation } from './OperationsAdmin';

function initialInput(account: Account): AccountInput {
  return {
    // Una referencia neutra se muestra como campo vacío: guardar vacío la conserva en Go.
    name: account.mode === 'takeaway' ? account.name : accountCustomName(account),
    mode: account.mode,
    table_id: account.table_id,
    price_list_id: account.price_list_id,
    customer_name: account.customer_name,
    phone: account.phone,
    origin: account.origin,
    notes: account.notes,
    scheduled_local: account.scheduled_local,
    schedule_kind: account.schedule_kind ?? '',
    assigned_user_id: account.assigned_user_id,
  };
}

// Panel «Datos de la cuenta» (salón) o «Datos del pedido» (para llevar) dentro de la captura. Usa
// la edición existente de la cuenta (PATCH con revisión, operation_id y auditoría en Go). La mesa
// y el responsable se cambian con sus acciones propias; la hora programada solo cambia si se edita
// aquí, para no sustituir una reprogramación hecha en cocina.
export function AccountDetails({
  account,
  timezone,
  lookups,
  can,
  execute,
  busy,
  onClose,
}: {
  account: Account;
  timezone: string;
  lookups: Lookups | null;
  can: (permission: string) => boolean;
  execute: ExecuteMutation;
  busy: boolean;
  onClose: () => void;
}) {
  const takeaway = account.mode === 'takeaway';
  const [revision, setRevision] = useState(account.revision);
  const [input, setInput] = useState<AccountInput>(() => initialInput(account));
  const stale = account.revision !== revision;
  const customOrigin = !takeawayOrigins.includes(input.origin);
  const title = takeaway
    ? `Datos del pedido #${account.folio}`
    : `Datos de la cuenta #${account.folio}`;

  return (
    <section className="operation-inline-panel account-details" aria-label={title}>
      <div className="section-heading">
        <h2>{title}</h2>
        <button type="button" className="secondary-button" onClick={onClose}>
          <IconLabel icon="back">Volver</IconLabel>
        </button>
      </div>
      <Notice>
        {stale &&
          'La cuenta cambió en otra terminal. Tus campos se conservan; revisa los cambios antes de continuar.'}
      </Notice>
      {stale && (
        <button
          type="button"
          className="secondary-button"
          onClick={() => setRevision(account.revision)}
        >
          Revisé la cuenta actual; conservar mis campos
        </button>
      )}
      <Form
        disabled={busy || stale}
        label={takeaway ? 'Guardar datos del pedido' : 'Guardar datos de la cuenta'}
        successMessage={takeaway ? 'Datos del pedido guardados' : 'Datos de la cuenta guardados'}
        onSave={async () => {
          await execute<Bill>('restaurant_save_account', {
            id: account.id,
            body: {
              ...input,
              revision,
              table_id: account.table_id,
              assigned_user_id: account.assigned_user_id,
            },
          });
          onClose();
        }}
      >
        {takeaway ? (
          <>
            <div className="form-grid">
              <TextField
                label="Nombre del cliente (opcional)"
                value={input.customer_name}
                onChange={(customer_name) => setInput({ ...input, customer_name })}
              />
              <TextField
                label="Teléfono (opcional)"
                value={input.phone}
                onChange={(phone) => setInput({ ...input, phone })}
              />
            </div>
            <p className="muted text-xs">
              El nombre del cliente se sugiere pero no es obligatorio: sin nombre, el pedido se
              identifica como Pedido #{account.folio}.
            </p>
            <div className="field">
              <span>Origen del pedido</span>
              <SegmentedControl
                label="Origen del pedido"
                value={customOrigin ? '' : input.origin}
                options={takeawayOrigins.map((origin) => ({ value: origin, label: origin }))}
                onChange={(origin) => setInput({ ...input, origin })}
              />
            </div>
            <TextField
              label="Otro origen (opcional)"
              value={customOrigin ? input.origin : ''}
              onChange={(origin) => setInput({ ...input, origin })}
            />
            <Accordion
              title="Pedido programado"
              summary={
                input.scheduled_local
                  ? `${input.scheduled_local} · ${timezone}`
                  : 'Opcional; no se envía automáticamente a cocina'
              }
              defaultOpen={!!input.scheduled_local}
            >
              <Field label="Programación">
                <select
                  value={input.schedule_kind}
                  onChange={(event) =>
                    setInput({
                      ...input,
                      schedule_kind: event.target.value,
                      scheduled_local: event.target.value ? input.scheduled_local : '',
                    })
                  }
                >
                  <option value="">Sin programación</option>
                  <option value="preparation">Hora de preparación</option>
                  <option value="delivery">Hora prevista de entrega</option>
                </select>
              </Field>
              {input.schedule_kind && (
                <TextField
                  label={`Fecha y hora · ${timezone}`}
                  type="datetime-local"
                  required
                  value={input.scheduled_local}
                  onChange={(scheduled_local) => setInput({ ...input, scheduled_local })}
                />
              )}
            </Accordion>
          </>
        ) : (
          <>
            <TextField
              label="Nombre de la cuenta (opcional)"
              value={input.name}
              onChange={(name) => setInput({ ...input, name })}
            />
            <p className="muted text-xs">
              Sin nombre, la cuenta se identifica como Cuenta #{account.folio}.
            </p>
          </>
        )}
        <Field label="Observaciones">
          <textarea
            value={input.notes}
            rows={3}
            maxLength={2000}
            onChange={(event) => setInput({ ...input, notes: event.target.value })}
          />
        </Field>
        {can('orders.price_list.manage') && (
          <>
            <ReferenceField
              optional
              label="Lista de precios para nuevas capturas"
              value={input.price_list_id}
              options={lookups?.['price-lists'] ?? []}
              onChange={(price_list_id) =>
                setInput({ ...input, price_list_id: price_list_id || null })
              }
            />
            <p className="muted text-xs">
              Cambiar la lista afecta nuevas capturas y modificaciones cotizadas expresamente. Los
              importes ya confirmados permanecen guardados.
            </p>
          </>
        )}
      </Form>
    </section>
  );
}
