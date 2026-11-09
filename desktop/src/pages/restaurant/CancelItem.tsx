import { useState } from 'react';
import type { FormEvent } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { IntegerField, TextField } from '../../components/catalog/EditorFields';
import { notifyError, notifySuccess } from '../../services/notify';
import { countLabel } from '../../services/plural';
import type { Bill, ConfirmedLine } from '../../services/restaurant';
import { uncertainError } from './capture/draftSync';
import type { ExecuteMutation } from './OperationsAdmin';

// Cancelación de unidades confirmadas. Antes de escribir se confirma con el diálogo estándar de
// peligro (etapa 7.1): la acción retira unidades enviadas a cocina. El resultado se avisa con el
// sistema único de mensajes; un resultado incierto queda en «Recuperar resultado» de la página.
export function CancelItem({
  bill,
  line,
  execute,
  busy,
  onClose,
}: {
  bill: Bill;
  line: ConfirmedLine;
  execute: ExecuteMutation;
  busy: boolean;
  onClose: () => void;
}) {
  const [revision] = useState(bill.account.revision);
  const [quantity, setQuantity] = useState(line.quantity);
  const [reason, setReason] = useState('');
  const [saving, setSaving] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const stale = revision !== bill.account.revision;
  const locked = busy || stale || saving;
  const name = line.quote_line.name;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (locked) {
      return;
    }
    const units = countLabel(quantity, 'unidad', 'unidades');
    const confirmed = await confirm({
      title: `¿Cancelar ${units} de ${name}?`,
      message: (
        <>
          <p>Se conservarán la partida y el motivo en el historial.</p>
          <p>Esta operación no registra una devolución financiera.</p>
        </>
      ),
      confirmLabel: 'Cancelar unidades',
      cancelLabel: 'Conservar unidades',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    setSaving(true);
    try {
      await execute('restaurant_cancel', {
        id: bill.account.id,
        body: { revision, version_id: line.version_id, quantity, reason },
      });
      notifySuccess(`Cancelación registrada: ${units} de ${name}`);
      onClose();
    } catch (problem) {
      if (!uncertainError(problem)) {
        notifyError(problem, 'No se pudieron cancelar las unidades.');
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="operation-inline-panel">
      {dialog}
      <div className="section-heading">
        <h2>Cancelar unidades · {name}</h2>
        <button className="secondary-button" onClick={onClose}>
          <IconLabel icon="back">Volver sin cancelar</IconLabel>
        </button>
      </div>
      <Notice>
        {stale
          ? 'La cuenta cambió. Vuelve a abrir la cancelación desde su versión actual; tus demás selecciones se conservan.'
          : 'Se conservará la partida y el motivo en el historial. Esta operación no registra una devolución financiera.'}
      </Notice>
      <form className="form-stack" onSubmit={(event) => void submit(event)}>
        <fieldset disabled={locked} className="form-stack">
          <IntegerField
            label="Unidades por cancelar"
            value={quantity}
            min={1}
            max={line.quantity}
            onChange={setQuantity}
          />
          <TextField label="Motivo de cancelación" value={reason} onChange={setReason} required />
        </fieldset>
        <div className="form-actions">
          <button type="submit" className="primary-button" disabled={locked}>
            <IconLabel icon="trash">
              {saving ? 'Cancelando…' : 'Confirmar cancelación de unidades'}
            </IconLabel>
          </button>
        </div>
      </form>
    </section>
  );
}
