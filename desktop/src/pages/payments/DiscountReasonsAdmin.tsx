import { useState } from 'react';
import { Field, Form, Notice } from '../../components/Forms';
import { PositionField } from '../../components/PositionField';
import { ToggleButton } from '../../components/ToggleButton';
import { permissionName } from '../../services/payments';
import type { DiscountReason } from '../../services/payments';
import { orderSiblings, positionPayload } from '../../services/positions';
import type { ExecuteMutation } from '../restaurant/OperationsAdmin';

// Edición de un motivo de descuento: nombre, si exige descripción, estado y posición en la lista.
// Go valida el nombre, que quede al menos un motivo activo y la revisión.
function ReasonEditor({
  reason,
  reasons,
  execute,
  busy,
  onClose,
}: {
  reason: DiscountReason | null;
  reasons: DiscountReason[];
  execute: ExecuteMutation;
  busy: boolean;
  onClose: () => void;
}) {
  const [name, setName] = useState(reason?.name ?? '');
  const [requiresDescription, setRequiresDescription] = useState(
    reason?.requires_description ?? false,
  );
  const [active, setActive] = useState(reason?.active ?? true);
  const [after, setAfter] = useState<string | undefined>(undefined);
  const siblings = reasons.map((item) => ({
    id: item.id,
    name: item.name,
    note: item.active ? undefined : 'inactivo',
  }));
  return (
    <Form
      label={reason ? 'Guardar motivo' : 'Crear motivo'}
      successMessage={reason ? 'Motivo de descuento guardado.' : 'Motivo de descuento creado.'}
      disabled={busy}
      onSave={async () => {
        const body: Record<string, unknown> = {
          name,
          requires_description: requiresDescription,
          active,
        };
        const position = positionPayload(reasons, reason?.id ?? '', after);
        if (position) {
          body.position = position;
        }
        if (reason) {
          await execute('payments_save_discount_reason', {
            id: reason.id,
            body: { ...body, revision: reason.revision },
          });
        } else {
          await execute('payments_create_discount_reason', { body });
        }
        onClose();
      }}
    >
      <div className="form-grid">
        <Field label="Nombre del motivo" hint="Hasta 60 caracteres; se imprime en el ticket.">
          <input
            required
            maxLength={60}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        <PositionField
          label="Posición del motivo"
          siblings={siblings}
          recordID={reason?.id ?? ''}
          value={after}
          onChange={setAfter}
        />
      </div>
      <div className="button-row">
        <ToggleButton
          label="Exige descripción"
          pressed={requiresDescription}
          onChange={setRequiresDescription}
        />
        <ToggleButton label="Motivo activo" pressed={active} onChange={setActive} />
      </div>
      <p className="muted text-xs">
        Desactivar un motivo solo deja de ofrecerlo: los descuentos ya registrados lo conservan.
      </p>
      <button type="button" className="secondary-button" onClick={onClose}>
        Cancelar
      </button>
    </Form>
  );
}

// Catálogo de motivos de descuento y cortesía (etapa 7.1). Se consulta con «Consultar información
// financiera» y se administra con «Configurar políticas de cobro e impresión de tickets».
export function DiscountReasonsAdmin({
  reasons,
  can,
  execute,
  busy,
}: {
  reasons: DiscountReason[];
  can: (permission: string) => boolean;
  execute: ExecuteMutation;
  busy: boolean;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const ordered = orderSiblings(reasons);
  const current = ordered.find((reason) => reason.id === editing) ?? null;
  const editable = can('payments.settings.manage');
  return (
    <div className="form-stack">
      <p className="muted text-sm">
        Se eligen con botones al aplicar un descuento o una cortesía y aparecen en el historial de
        la cuenta, el ticket y la precuenta. «Exige descripción» pide al cajero explicar el motivo.
      </p>
      {!editable && (
        <Notice>
          Modificar los motivos requiere el permiso «{permissionName('payments.settings.manage')}».
        </Notice>
      )}
      <div className="cash-choice-grid" role="group" aria-label="Motivos de descuento">
        {ordered.map((reason) => (
          <button
            type="button"
            key={reason.id}
            className={editing === reason.id ? 'cash-choice-selected' : ''}
            aria-pressed={editing === reason.id}
            disabled={!editable}
            onClick={() => setEditing(reason.id)}
          >
            {reason.name}
            {reason.requires_description && (
              <span className="muted text-xs"> · exige descripción</span>
            )}
            {!reason.active && <span className="muted text-xs"> · inactivo</span>}
          </button>
        ))}
        {editable && (
          <button type="button" onClick={() => setEditing('')}>
            + Nuevo motivo
          </button>
        )}
      </div>
      {editable && editing !== null && (
        <ReasonEditor
          key={editing}
          reason={current}
          reasons={ordered}
          execute={execute}
          busy={busy}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  );
}
