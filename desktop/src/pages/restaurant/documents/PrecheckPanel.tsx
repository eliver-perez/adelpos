import { useState } from 'react';
import { Notice } from '../../../components/Forms';
import { useLiveResource } from '../../../hooks/useLiveResource';
import { useRestaurantMutation } from '../../../hooks/useRestaurantMutation';
import { APIError } from '../../../services/api';
import {
  documentJobLabels,
  documentsPendingKey,
  formatMoney,
  isZeroMoney,
  permissionName,
  printAttentionStates,
} from '../../../services/payments';
import type { CommercialDocument, PrintingResult } from '../../../services/payments';
import type { Bill } from '../../../services/restaurant';
import { UncertainOperation } from '../checkout/UncertainOperation';

const missingPrinter =
  'No hay impresora de tickets configurada. Configúrala en Cobros → Políticas de cobro.';

function printResultMessage(result: PrintingResult) {
  const status = result.status ?? '';
  const label = documentJobLabels[status] ?? 'Solicitud registrada';
  if (printAttentionStates.includes(status)) {
    return `Precuenta: ${label}. Se resuelve en Cocina → Cola de impresión.`;
  }
  return `Precuenta enviada a la cola de impresión: ${label}.`;
}

// Precuenta integrada en la operación: vista previa del documento que entrega Go (texto de 42
// columnas) y su impresión. Consultarla no registra venta, pago ni documento.
export function PrecheckPanel({
  bill,
  active,
  can,
  onClose,
}: {
  bill: Bill;
  active: boolean;
  can: (permission: string) => boolean;
  onClose: () => void;
}) {
  const accountID = bill.account.id;
  const document = useLiveResource<CommercialDocument>('payments_precheck', {
    id: accountID,
    enabled: active,
    interval: 30000,
  });
  const [result, setResult] = useState<PrintingResult | null>(null);
  const [notice, setNotice] = useState('');
  const mutation = useRestaurantMutation((_operation, value) => {
    setResult(value as PrintingResult);
    window.dispatchEvent(new CustomEvent('adel-server-change'));
  }, documentsPendingKey(accountID));
  const locked = mutation.busy || !!mutation.pending;
  const data = document.data;

  async function print() {
    if (mutation.isPending()) {
      return;
    }
    setResult(null);
    setNotice('');
    try {
      await mutation.execute('payments_print_precheck', { id: accountID, body: {} });
    } catch (problem) {
      if (problem instanceof APIError && problem.code === 'PRINTER_NOT_CONFIGURED') {
        setNotice(missingPrinter);
      }
    }
  }

  return (
    <section className="operation-inline-panel precheck-panel" aria-label="Precuenta">
      <div className="section-heading">
        <h2>Precuenta · #{bill.account.folio}</h2>
        <button type="button" className="secondary-button" onClick={onClose}>
          Volver
        </button>
      </div>
      <p className="muted text-sm">
        La precuenta solo informa el consumo: no registra venta, pago ni documento y no es un
        comprobante fiscal (CFDI).
      </p>
      <Notice>{document.error}</Notice>
      {mutation.pending && mutation.uncertain && (
        <UncertainOperation
          request={mutation.pending}
          methods={[]}
          busy={mutation.busy}
          onRetry={() => void mutation.retry()}
          onConsult={() => window.dispatchEvent(new CustomEvent('adel-server-change'))}
        />
      )}
      {notice && <Notice>{notice}</Notice>}
      {mutation.error && !mutation.uncertain && mutation.errorCode !== 'PRINTER_NOT_CONFIGURED' && (
        <Notice>{mutation.error}</Notice>
      )}
      {result && <Notice success>{printResultMessage(result)}</Notice>}
      {!data && !document.error && <p className="muted">Preparando la precuenta…</p>}
      {data && (
        <div className="precheck-layout">
          <dl className="checkout-figures precheck-summary">
            <div className="checkout-figure">
              <dt>{data.context}</dt>
              <dd>{data.local_time}</dd>
            </div>
            <div className="checkout-figure">
              <dt>Total</dt>
              <dd>{formatMoney(data.total)}</dd>
            </div>
            <div className="checkout-figure">
              <dt>IVA incluido</dt>
              <dd>{formatMoney(data.tax_total)}</dd>
            </div>
            <div className="checkout-figure">
              <dt>Pagado</dt>
              <dd>{formatMoney(data.paid)}</dd>
            </div>
            {!isZeroMoney(data.tips) && (
              <div className="checkout-figure">
                <dt>Propinas (fuera de la venta)</dt>
                <dd>{formatMoney(data.tips)}</dd>
              </div>
            )}
            <div className="checkout-figure checkout-figure-emphasis">
              <dt>Saldo pendiente</dt>
              <dd>{formatMoney(data.balance)}</dd>
            </div>
          </dl>
          <pre
            className="kitchen-document commercial-document"
            aria-label="Vista previa de la precuenta"
          >
            {data.content}
          </pre>
        </div>
      )}
      {can('payments.tickets.print') ? (
        <div>
          <button
            type="button"
            className="primary-button"
            disabled={locked || !data}
            onClick={() => void print()}
          >
            {mutation.busy ? 'Enviando precuenta…' : 'Imprimir precuenta'}
          </button>
        </div>
      ) : (
        <p className="muted text-xs">
          Imprimir requiere el permiso «{permissionName('payments.tickets.print')}».
        </p>
      )}
    </section>
  );
}
