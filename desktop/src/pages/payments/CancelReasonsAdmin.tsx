import { useState } from 'react';
import { Field, Form, Notice } from '../../components/Forms';
import { PositionField } from '../../components/PositionField';
import { ToggleButton } from '../../components/ToggleButton';
import { permissionName } from '../../services/payments';
import { orderSiblings, positionPayload } from '../../services/positions';
import type { CancelReason } from '../../services/restaurant';
import type { ExecuteMutation } from '../restaurant/OperationsAdmin';

// Edición de un motivo de cancelación de cuentas (etapa 9D, §5.2): nombre, si exige descripción,
// si lo enviado a cocina cuenta como merma, estado y posición. Go valida el nombre (1 a 60
// caracteres, sin repetirse), que quede al menos un motivo activo y la revisión; renombrar o
// cambiar las marcas no altera las cancelaciones ya registradas.
function CancelReasonEditor({
  reason,
  reasons,
  execute,
  busy,
  onClose,
}: {
  reason: CancelReason | null;
  reasons: CancelReason[];
  execute: ExecuteMutation;
  busy: boolean;
  onClose: () => void;
}) {
  const [name, setName] = useState(reason?.name ?? '');
  const [requiresDescription, setRequiresDescription] = useState(
    reason?.requires_description ?? false,
  );
  const [preparedIsWaste, setPreparedIsWaste] = useState(reason?.prepared_is_waste ?? false);
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
      successMessage={reason ? 'Motivo de cancelación guardado.' : 'Motivo de cancelación creado.'}
      disabled={busy}
      onSave={async () => {
        const body: Record<string, unknown> = {
          name,
          requires_description: requiresDescription,
          prepared_is_waste: preparedIsWaste,
          active,
        };
        const position = positionPayload(reasons, reason?.id ?? '', after);
        if (position) {
          body.position = position;
        }
        if (reason) {
          await execute('restaurant_save_cancel_reason', {
            id: reason.id,
            body: { ...body, revision: reason.revision },
          });
        } else {
          await execute('restaurant_create_cancel_reason', { body });
        }
        onClose();
      }}
    >
      <div className="form-grid">
        <Field
          label="Nombre del motivo"
          hint="Hasta 60 caracteres; aparece en el reporte «Cuentas canceladas» y en el historial."
        >
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
        <ToggleButton
          label="Lo enviado a cocina cuenta como merma"
          pressed={preparedIsWaste}
          onChange={setPreparedIsWaste}
        />
        <ToggleButton label="Motivo activo" pressed={active} onChange={setActive} />
      </div>
      <p className="muted text-xs">
        Desactivar un motivo solo deja de ofrecerlo: las cuentas ya canceladas lo conservan con su
        nombre y sus marcas de entonces.
      </p>
      <button type="button" className="secondary-button" onClick={onClose}>
        Cancelar
      </button>
    </Form>
  );
}

// Catálogo de motivos de cancelación de cuentas (etapa 9D), junto a los motivos de descuento y con
// su mismo estilo. Se consulta con «Consultar cuentas e importes» y se administra con «Configurar
// políticas de cobro e impresión de tickets», el mismo permiso de los motivos de descuento.
export function CancelReasonsAdmin({
  reasons,
  can,
  execute,
  busy,
}: {
  reasons: CancelReason[];
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
        Se eligen con botones al cancelar una cuenta y aparecen en el reporte «Cuentas canceladas» y
        en el historial de la cuenta. «Exige descripción» pide explicar el motivo; con «Lo enviado a
        cocina cuenta como merma», lo ya preparado de la cuenta queda como merma.
      </p>
      {!editable && (
        <Notice>
          Modificar los motivos requiere el permiso «{permissionName('payments.settings.manage')}».
        </Notice>
      )}
      <div className="cash-choice-grid" role="group" aria-label="Motivos de cancelación">
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
            {reason.prepared_is_waste && (
              <span className="muted text-xs"> · cuenta como merma</span>
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
        <CancelReasonEditor
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
