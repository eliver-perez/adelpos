import { useState } from 'react';
import { Form, Notice } from '../../components/Forms';
import { IntegerField, TextField } from '../../components/catalog/EditorFields';
import type { Bill, ConfirmedLine } from '../../services/restaurant';
import type { ExecuteMutation } from './OperationsAdmin';

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
  const stale = revision !== bill.account.revision;
  return (
    <section className="operation-inline-panel">
      <div className="section-heading">
        <h2>Cancelar unidades · {line.quote_line.name}</h2>
        <button className="secondary-button" onClick={onClose}>
          Volver sin cancelar
        </button>
      </div>
      <Notice>
        {stale
          ? 'La cuenta cambió. Vuelve a abrir la cancelación desde su versión actual; tus demás selecciones se conservan.'
          : 'Se conservará la partida y el motivo en el historial. Esta operación no registra una devolución financiera.'}
      </Notice>
      <Form
        disabled={busy || stale}
        label="Confirmar cancelación de unidades"
        onSave={async () => {
          await execute('restaurant_cancel', {
            id: bill.account.id,
            body: { revision, version_id: line.version_id, quantity, reason },
          });
          onClose();
        }}
      >
        <IntegerField
          label="Unidades por cancelar"
          value={quantity}
          min={1}
          max={line.quantity}
          onChange={setQuantity}
        />
        <TextField label="Motivo de cancelación" value={reason} onChange={setReason} required />
      </Form>
    </section>
  );
}
