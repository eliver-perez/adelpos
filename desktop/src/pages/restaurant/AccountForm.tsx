import { useState } from 'react';
import { Field, Form, Notice } from '../../components/Forms';
import { Accordion } from '../../components/Accordion';
import { ReferenceField, TextField } from '../../components/catalog/EditorFields';
import type { Lookups } from '../../services/catalog';
import type { Account, AccountInput, Bill, Overview } from '../../services/restaurant';
import type { ExecuteMutation } from './OperationsAdmin';

export function AccountForm({
  mode,
  timezone,
  tableID,
  current,
  overview,
  lookups,
  can,
  execute,
  busy,
  onClose,
}: {
  mode: Account['mode'];
  timezone: string;
  tableID: string | null;
  current?: Account;
  overview: Overview | null;
  lookups: Lookups | null;
  can: (permission: string) => boolean;
  execute: ExecuteMutation;
  busy: boolean;
  onClose: () => void;
}) {
  const [revision, setRevision] = useState(current?.revision ?? 0);
  const [input, setInput] = useState<AccountInput>(() =>
    current
      ? {
          name: current.name,
          mode: current.mode,
          table_id: current.table_id,
          price_list_id: current.price_list_id,
          customer_name: current.customer_name,
          phone: current.phone,
          origin: current.origin,
          notes: current.notes,
          scheduled_local: current.scheduled_local,
          schedule_kind: current.schedule_kind ?? '',
          assigned_user_id: current.assigned_user_id,
        }
      : {
          name: mode === 'takeaway' ? 'Para llevar' : 'Cuenta',
          mode,
          table_id: tableID,
          price_list_id: null,
          customer_name: '',
          phone: '',
          origin: 'Mostrador',
          notes: '',
          scheduled_local: '',
          schedule_kind: '',
          assigned_user_id: null,
        },
  );
  const stale = !!current && current.revision !== revision;
  const table = overview?.tables.find(
    (entry) => entry.id === (current?.table_id ?? input.table_id),
  );

  return (
    <section className="operation-inline-panel">
      <div className="section-heading">
        <h2>
          {current
            ? `Datos de la cuenta #${current.folio}`
            : mode === 'takeaway'
              ? 'Nuevo pedido para llevar'
              : 'Abrir cuenta'}
        </h2>
        <button className="secondary-button" onClick={onClose}>
          Volver
        </button>
      </div>
      <Notice>
        {stale &&
          'La cuenta cambió en otra terminal. Tus campos se conservan; revisa los cambios antes de continuar.'}
      </Notice>
      {stale && (
        <button
          className="secondary-button"
          onClick={() => setRevision(current?.revision ?? revision)}
        >
          Revisé la cuenta actual; conservar mis campos
        </button>
      )}
      <Form
        disabled={busy || stale}
        label={current ? 'Guardar datos de cuenta' : 'Abrir cuenta y capturar'}
        onSave={async () => {
          const body = {
            ...input,
            revision,
            table_id: current ? current.table_id : input.table_id,
            assigned_user_id: current ? current.assigned_user_id : input.assigned_user_id,
          };
          await execute<Bill>(current ? 'restaurant_save_account' : 'restaurant_open_account', {
            id: current?.id,
            body,
          });
          onClose();
        }}
      >
        <TextField
          label="Nombre o referencia de cuenta"
          required
          value={input.name}
          onChange={(name) => setInput({ ...input, name })}
        />
        {input.mode === 'dine_in' &&
          (current ? (
            <p className="muted text-sm">
              Mesa actual: {current.table_name ?? 'Sin mesa'}. Usa Cambiar mesa para trasladar la
              cuenta.
            </p>
          ) : (
            <ReferenceField
              optional
              label="Mesa (vacío: cuenta sin mesa)"
              value={input.table_id}
              options={(overview?.tables ?? []).filter(
                (entry) => entry.active && !entry.out_of_service,
              )}
              onChange={(table_id) => setInput({ ...input, table_id: table_id || null })}
            />
          ))}
        {table?.out_of_service && (
          <Notice>
            La mesa está fuera de servicio; sus cuentas existentes conservan el historial.
          </Notice>
        )}
        {input.mode === 'takeaway' && (
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
            <Field label="Origen del pedido">
              <input
                list="takeaway-origins"
                value={input.origin}
                maxLength={80}
                onChange={(event) => setInput({ ...input, origin: event.target.value })}
              />
              <datalist id="takeaway-origins">
                <option value="Mostrador" />
                <option value="Teléfono" />
                <option value="Ventanilla" />
              </datalist>
            </Field>
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
        )}
        <Field label="Observaciones de la cuenta">
          <textarea
            value={input.notes}
            rows={2}
            maxLength={2000}
            onChange={(event) => setInput({ ...input, notes: event.target.value })}
          />
        </Field>
        {can('orders.price_list.manage') && (
          <ReferenceField
            optional
            label="Lista de precios para nuevas capturas"
            value={input.price_list_id}
            options={lookups?.['price-lists'] ?? []}
            onChange={(price_list_id) =>
              setInput({ ...input, price_list_id: price_list_id || null })
            }
          />
        )}
        {current && (
          <p className="muted text-xs">
            Cambiar la lista afecta nuevas capturas y modificaciones cotizadas expresamente. Los
            importes ya confirmados permanecen guardados.
          </p>
        )}
      </Form>
    </section>
  );
}
