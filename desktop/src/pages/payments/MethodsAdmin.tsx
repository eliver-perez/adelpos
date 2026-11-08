import { useState } from 'react';
import { Field, Form, Notice } from '../../components/Forms';
import { PaymentMethodIcon } from '../../components/PaymentMethodIcon';
import { SegmentedControl } from '../../components/SegmentedControl';
import { Check } from '../../components/catalog/EditorFields';
import {
  methodIconLabels,
  methodIcons,
  permissionName,
  referenceModeLabels,
} from '../../services/payments';
import type { MethodInput, PaymentMethod, ReferenceMode } from '../../services/payments';
import type { ExecuteMutation } from '../restaurant/OperationsAdmin';

function initialInput(method: PaymentMethod | null, nextOrder: number): MethodInput {
  if (!method) {
    return {
      revision: 0,
      name: '',
      icon: 'other',
      active: true,
      sort_order: nextOrder,
      is_cash: false,
      reference_mode: 'optional',
      allows_tip: false,
      allows_refund: true,
    };
  }
  return {
    revision: method.revision,
    name: method.name,
    icon: method.icon,
    active: method.active,
    sort_order: method.sort_order,
    is_cash: method.is_cash,
    reference_mode: method.reference_mode,
    allows_tip: method.allows_tip,
    allows_refund: method.allows_refund,
  };
}

// El orden es un entero (no dinero): se captura como texto para no depender de type="number".
function OrderField({ value, onChange }: { value: number; onChange: (value: number) => void }) {
  return (
    <Field label="Orden" hint="De 0 a 10000; los métodos se muestran de menor a mayor.">
      <input
        type="text"
        inputMode="numeric"
        maxLength={5}
        value={String(value)}
        onChange={(event) => {
          const digits = event.target.value.replace(/[^0-9]/g, '');
          onChange(digits === '' ? 0 : Number.parseInt(digits, 10));
        }}
      />
    </Field>
  );
}

function MethodEditor({
  method,
  nextOrder,
  editable,
  execute,
  busy,
  onClose,
}: {
  method: PaymentMethod | null;
  nextOrder: number;
  editable: boolean;
  execute: ExecuteMutation;
  busy: boolean;
  onClose: () => void;
}) {
  const [value, setValue] = useState<MethodInput>(() => initialInput(method, nextOrder));
  // La revisión de partida detecta cambios de otra terminal sin sobrescribirlos en silencio.
  const stale = !!method && method.revision !== value.revision;

  function change(patch: Partial<MethodInput>) {
    setValue((current) => {
      const next = { ...current, ...patch };
      // Un método de efectivo entrega cambio y no admite referencia electrónica (regla de Go).
      if (next.is_cash) {
        next.reference_mode = 'none';
      }
      return next;
    });
  }

  return (
    <section className="surface panel">
      <div className="section-heading">
        <h2>{method ? `Editar ${method.name}` : 'Nuevo método de pago'}</h2>
        <button type="button" className="secondary-button" onClick={onClose}>
          Cerrar editor
        </button>
      </div>
      {stale && (
        <Notice>
          Este método cambió en otra terminal. Carga la versión actual antes de guardar.{' '}
          <button
            type="button"
            className="secondary-button"
            onClick={() => setValue(initialInput(method, nextOrder))}
          >
            Cargar versión actual
          </button>
        </Notice>
      )}
      <Form
        label={method ? 'Guardar método' : 'Crear método'}
        disabled={!editable || busy}
        onSave={async () => {
          const body = { ...value };
          if (method) {
            const saved = await execute<PaymentMethod>('payments_save_method', {
              id: method.id,
              body,
            });
            // La nueva revisión guardada es la base de la siguiente edición.
            setValue(initialInput(saved, nextOrder));
          } else {
            await execute('payments_create_method', { body });
            onClose();
          }
        }}
      >
        <div className="form-grid">
          <Field label="Nombre visible">
            <input
              required
              maxLength={60}
              value={value.name}
              onChange={(event) => change({ name: event.target.value })}
            />
          </Field>
          <OrderField value={value.sort_order} onChange={(sort_order) => change({ sort_order })} />
        </div>
        <div className="field">
          <span>Icono</span>
          <div className="payment-icon-options" role="group" aria-label="Icono del método">
            {methodIcons.map((icon) => (
              <button
                type="button"
                key={icon}
                className={`payment-method-button ${value.icon === icon ? 'payment-method-selected' : ''}`}
                aria-pressed={value.icon === icon}
                onClick={() => change({ icon })}
              >
                <PaymentMethodIcon icon={icon} className="payment-method-icon" />
                <span>{methodIconLabels[icon]}</span>
              </button>
            ))}
          </div>
        </div>
        <div className="form-grid">
          <div className="field">
            <span>Naturaleza</span>
            <SegmentedControl
              label="Naturaleza del método"
              value={value.is_cash ? 'cash' : 'electronic'}
              onChange={(nature) => change({ is_cash: nature === 'cash' })}
              options={[
                { value: 'cash', label: 'Efectivo' },
                { value: 'electronic', label: 'No efectivo' },
              ]}
            />
          </div>
          <div className="field">
            <span>Referencia de la operación</span>
            <SegmentedControl<ReferenceMode>
              label="Referencia de la operación"
              value={value.reference_mode}
              onChange={(reference_mode) => change({ reference_mode })}
              disabled={value.is_cash}
              options={[
                { value: 'none', label: referenceModeLabels.none },
                { value: 'optional', label: referenceModeLabels.optional },
                { value: 'required', label: referenceModeLabels.required },
              ]}
            />
            {value.is_cash && (
              <small className="muted">El efectivo no admite referencia electrónica.</small>
            )}
          </div>
        </div>
        <Check
          label="Método activo (disponible en la pantalla de cobro)"
          value={value.active}
          onChange={(active) => change({ active })}
        />
        <Check
          label="Permite registrar propina"
          value={value.allows_tip}
          onChange={(allows_tip) => change({ allows_tip })}
        />
        <Check
          label="Permite devoluciones con este método"
          value={value.allows_refund}
          onChange={(allows_refund) => change({ allows_refund })}
        />
        <p className="muted text-xs">
          Renombrar un método no altera los pagos históricos: cada pago conserva el nombre con el
          que se registró.
        </p>
      </Form>
    </section>
  );
}

export function MethodsAdmin({
  methods,
  can,
  execute,
  busy,
}: {
  methods: PaymentMethod[];
  can: (permission: string) => boolean;
  execute: ExecuteMutation;
  busy: boolean;
}) {
  const [editing, setEditing] = useState<string | null | undefined>(undefined);
  const editable = can('payments.methods.manage');
  const selected = methods.find((method) => method.id === editing) ?? null;
  const nextOrder = methods.reduce((highest, method) => Math.max(highest, method.sort_order), 0);
  return (
    <div className="form-stack">
      {!editable && (
        <p className="muted text-sm">
          Consulta disponible. Editar métodos requiere el permiso «
          {permissionName('payments.methods.manage')}».
        </p>
      )}
      {editable && (
        <div>
          <button
            type="button"
            className="primary-button"
            disabled={busy}
            onClick={() => setEditing(null)}
          >
            Nuevo método de pago
          </button>
        </div>
      )}
      {editing !== undefined && (
        <MethodEditor
          key={editing ?? 'new'}
          method={selected}
          nextOrder={nextOrder + 10}
          editable={editable}
          execute={execute}
          busy={busy}
          onClose={() => setEditing(undefined)}
        />
      )}
      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>Método</th>
              <th>Naturaleza</th>
              <th>Referencia</th>
              <th>Propina</th>
              <th>Devolución</th>
              <th>Orden</th>
              <th>Estado</th>
              <th>Acciones</th>
            </tr>
          </thead>
          <tbody>
            {methods.map((method) => (
              <tr key={method.id}>
                <td>
                  <span className="flex items-center gap-2">
                    <PaymentMethodIcon icon={method.icon} />
                    <strong>{method.name}</strong>
                  </span>
                </td>
                <td>{method.is_cash ? 'Efectivo' : 'No efectivo'}</td>
                <td>{referenceModeLabels[method.reference_mode]}</td>
                <td>{method.allows_tip ? 'Permitida' : 'No'}</td>
                <td>{method.allows_refund ? 'Permitida' : 'No'}</td>
                <td>{method.sort_order}</td>
                <td>{method.active ? 'Activo' : 'Inactivo'}</td>
                <td>
                  {editable && (
                    <button
                      type="button"
                      className="secondary-button"
                      onClick={() => setEditing(method.id)}
                    >
                      Editar
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
