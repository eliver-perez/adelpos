import { Notice } from '../../../components/Forms';
import { IconLabel } from '../../../components/Icon';
import { useLiveResource } from '../../../hooks/useLiveResource';
import { useRestaurantMutation } from '../../../hooks/useRestaurantMutation';
import { precheckName, usePrecheckName } from '../../../services/documentNames';
import type { PrecheckName } from '../../../services/documentNames';
import { notifySuccess, notifyWarning } from '../../../services/notify';
import {
  documentJobLabels,
  documentsPendingKey,
  formatMoney,
  isZeroMoney,
  permissionName,
  printAttentionStates,
} from '../../../services/payments';
import type { CommercialDocument, PrintingResult } from '../../../services/payments';
import { notifyPrinterRejection } from '../../../services/printProblems';
import type { Bill } from '../../../services/restaurant';
import { UncertainOperation } from '../checkout/UncertainOperation';
import { notifyActionError } from '../checkout/actionErrors';

// Primera letra en mayúscula («La precuenta», «El ticket de venta»).
function capitalized(text: string) {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

// Resultado de la impresión como aviso: «enviada», nunca «impresa». Un trabajo que exige atención
// (incierto…) se avisa como advertencia que dura más; si no sale en papel, la lista de documentos de
// la cuenta lo explica y ofrece «Reintentar» (etapa 9F).
function notifyPrintResult(result: PrintingResult, name: PrecheckName) {
  const status = result.status ?? '';
  const label = documentJobLabels[status] ?? 'Solicitud registrada';
  if (printAttentionStates.includes(status)) {
    notifyWarning(`${name.label}: ${label}. Revísalo en los documentos de la cuenta.`);
    return;
  }
  notifySuccess(`${name.label} ${name.sent} a la cola de impresión: ${label}.`);
}

// Sin una impresora válida (etapa 9F, decisión D4) el aviso dice qué pasa y ofrece «Configurar
// impresora» y «Reintentar»; los demás rechazos son los de Go.
function notifyPrintFailure(problem: unknown, name: PrecheckName, retry: () => void) {
  if (notifyPrinterRejection(problem, retry, 'printer-missing:precheck')) {
    return;
  }
  notifyActionError(problem, `No se pudo enviar ${name.withArticle} a imprimir.`);
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
  const name = usePrecheckName();
  const mutation = useRestaurantMutation((_operation, value) => {
    notifyPrintResult(value as PrintingResult, precheckName());
    window.dispatchEvent(new CustomEvent('adel-server-change'));
  }, documentsPendingKey(accountID));
  const locked = mutation.busy || !!mutation.pending;
  const data = document.data;

  async function print() {
    if (mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute('payments_print_precheck', { id: accountID, body: {} });
    } catch (problem) {
      notifyPrintFailure(problem, precheckName(), () => void print());
    }
  }

  async function retry() {
    const problem = await mutation.retry();
    if (problem) {
      notifyPrintFailure(problem, precheckName(), () => void print());
    }
  }

  return (
    <section className="operation-inline-panel precheck-panel" aria-label={name.label}>
      <div className="section-heading">
        <h2>
          {name.label} · #{bill.account.folio}
        </h2>
        <button type="button" className="secondary-button" onClick={onClose}>
          <IconLabel icon="back">Volver</IconLabel>
        </button>
      </div>
      <p className="muted text-sm">
        {capitalized(name.withArticle)} solo informa el consumo: no registra venta, pago ni
        documento y no es un comprobante fiscal (CFDI).
      </p>
      <Notice>{document.error}</Notice>
      {mutation.pending && mutation.uncertain && (
        <UncertainOperation
          request={mutation.pending}
          methods={[]}
          busy={mutation.busy}
          onRetry={() => void retry()}
          onConsult={() => window.dispatchEvent(new CustomEvent('adel-server-change'))}
        />
      )}
      {!data && !document.error && <p className="muted">Preparando {name.withArticle}…</p>}
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
            aria-label={`Vista previa ${name.ofWithArticle}`}
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
            <IconLabel icon="printer">
              {mutation.busy ? `Enviando ${name.lower}…` : `Imprimir ${name.lower}`}
            </IconLabel>
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
