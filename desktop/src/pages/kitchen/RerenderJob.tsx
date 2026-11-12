import { useState } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Notice } from '../../components/Forms';
import { ReferenceField, TextField } from '../../components/catalog/EditorFields';
import type { ExecuteKitchen, Printer, PrintJob, TicketDocument } from '../../services/kitchen';
import { notifySuccess } from '../../services/notify';
import { renderCurrentConfirmation, renderCurrentLabel } from '../documents/rerender';

// Etapa 9B: «Imprimir con la plantilla vigente» desde la cola de cocina. Solo para comandas y
// avisos de modificación o cancelación; los tickets, precuentas y documentos de caja se imprimen
// con la plantilla vigente desde Cobros o Caja. Go exige kitchen.reprint, kitchen.stations.all y
// documents.history.render_current (otra impresora, además, kitchen.printers.manage) y un motivo.

const kitchenKinds = ['normal', 'modification', 'cancellation'];

export function RerenderJob({
  job,
  document,
  printers,
  canRedirect,
  execute,
  busy,
  onClose,
}: {
  job: PrintJob;
  document: TicketDocument | null;
  printers: Printer[];
  canRedirect: boolean;
  execute: ExecuteKitchen;
  busy: boolean;
  onClose: () => void;
}) {
  const [reason, setReason] = useState('');
  const [printerID, setPrinterID] = useState('');
  const [sending, setSending] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const unsupported = document !== null && !kitchenKinds.includes(document.kind);
  const shortReason = reason.trim().length < 5;

  async function send() {
    if (!(await confirm(renderCurrentConfirmation('la comanda')))) {
      return;
    }
    setSending(true);
    try {
      const body: Record<string, unknown> = { job_id: job.id, reason: reason.trim() };
      if (printerID) {
        body.printer_id = printerID;
      }
      await execute('kitchen_rerender', { body });
      notifySuccess('Comanda con la plantilla vigente enviada a la cola de impresión.');
      onClose();
    } catch {
      // KitchenPage ya avisó el rechazo de Go; un resultado incierto queda en su panel.
    } finally {
      setSending(false);
    }
  }

  if (unsupported) {
    return (
      <Notice>
        Solo las comandas y los avisos de cocina se imprimen con la plantilla vigente desde aquí.
        Los tickets y las precuentas se imprimen desde la cuenta en Cobros; los documentos de caja,
        desde Caja.
      </Notice>
    );
  }
  return (
    <div className="form-stack">
      {dialog}
      <p className="muted text-sm">
        «Solicitar otra copia» reimprime el documento idéntico. Esta acción genera la comanda con
        sus datos originales y la plantilla activa hoy.
      </p>
      <TextField
        label="Motivo de la impresión con la plantilla vigente"
        required
        value={reason}
        onChange={setReason}
        hint="Al menos 5 caracteres; queda en el Registro de movimientos."
      />
      {canRedirect && (
        <ReferenceField
          optional
          label="Otra impresora (vacío: la del trabajo original)"
          value={printerID}
          options={printers}
          onChange={setPrinterID}
        />
      )}
      <div className="button-row">
        <button
          type="button"
          className="primary-button"
          disabled={busy || sending || shortReason || job.state === 'processing'}
          onClick={() => void send()}
        >
          {sending ? 'Enviando…' : renderCurrentLabel}
        </button>
      </div>
    </div>
  );
}
