import { Notice } from '../../../components/Forms';
import { useConfirmDialog } from '../../../components/ConfirmDialog';
import { usePendingEntry } from '../../../hooks/pendingOperations';
import type { Lookups } from '../../../services/catalog';
import { preparationLabels } from '../../../services/kitchen';
import { IconLabel } from '../../../components/Icon';
import { formatMoney } from '../../../services/money';
import {
  deliveryStateDescriptions,
  paymentPendingKey,
  paymentStateDescriptions,
} from '../../../services/payments';
import { isPendingAccount } from '../../../services/restaurant';
import type {
  Account,
  Bill,
  ConfirmedLine,
  DraftItem,
  OrderQuote,
} from '../../../services/restaurant';
import { TakeawayDelivery } from '../documents/TakeawayDelivery';
import { ConfirmedLines, DraftLines } from './BillLines';
import type { QuotedDraftLine } from './BillLines';
import { BillFooter } from './BillFooter';
import type { BillAction } from './BillFooter';
import { sameDraftItem } from './captureRules';
import type { DraftView } from './draftSync';

export type { BillAction };

// Estado de pago con el saldo que calculó Go; la interfaz no suma importes.
function paymentDescription(account: Account) {
  const label = paymentStateDescriptions[account.payment_state] ?? account.payment_state;
  if (account.payment_state === 'partial' && account.finance) {
    return `${label} · saldo ${formatMoney(account.finance.balance)}`;
  }
  return label;
}

function closedDescription(account: Account) {
  if (account.mode === 'takeaway') {
    return 'Pedido liquidado y entregado; la cuenta está cerrada.';
  }
  return 'Cuenta liquidada y cerrada; la mesa quedó libre.';
}

// Importes de Go para las partidas nuevas: la cotización del borrador guardado se asocia por
// posición a sus partidas y solo se muestra si el contenido local sigue siendo el mismo.
export function quotedDraftLines(
  quote: OrderQuote | null,
  persisted: DraftItem[],
  persistedRevision: number,
  accountID: string,
): Map<string, QuotedDraftLine> {
  const result = new Map<string, QuotedDraftLine>();
  if (!quote?.quote || quote.account_id !== accountID || quote.revision !== persistedRevision) {
    return result;
  }
  persisted.forEach((item, index) => {
    const line = quote.quote?.lines[index];
    if (line) {
      result.set(item.id, { total: line.total, breakdown: line });
    }
  });
  return result;
}

export function CaptureBill({
  bill,
  error,
  loading,
  lookups,
  quote,
  quoteError,
  localDraft,
  persistedDraft,
  persistedRevision,
  draftView,
  can,
  blocked,
  linesLocked,
  sending,
  onIncrement,
  onDecrement,
  onCustomize,
  onLineEdit,
  onLineCancel,
  onAction,
  onSend,
  onCheckout,
  onDeliverCheckout,
  onMergedTarget,
  onKeepMine,
  onDiscard,
  onCapture,
}: {
  bill: Bill | null;
  error: string;
  loading: boolean;
  lookups: Lookups | null;
  quote: OrderQuote | null;
  quoteError: string;
  localDraft: DraftItem[];
  persistedDraft: DraftItem[];
  persistedRevision: number;
  draftView: DraftView | null;
  can: (permission: string) => boolean;
  blocked: boolean;
  linesLocked: boolean;
  sending: boolean;
  onIncrement: (item: DraftItem) => void;
  onDecrement: (item: DraftItem) => void;
  onCustomize: (item: DraftItem) => void;
  onLineEdit: (line: ConfirmedLine) => void;
  onLineCancel: (line: ConfirmedLine) => void;
  onAction: (action: BillAction) => void;
  onSend: () => void;
  onCheckout: () => void;
  onDeliverCheckout: () => void;
  onMergedTarget: (id: string) => void;
  onKeepMine: () => void;
  onDiscard: () => void;
  onCapture?: () => void;
}) {
  const { confirm, dialog } = useConfirmDialog();
  const paymentPending = usePendingEntry(paymentPendingKey(bill?.account.id ?? ''));
  if (!bill) {
    return (
      <aside className="capture-bill" aria-label="Cuenta actual">
        <header className="capture-bill-header">
          <h2>Cuenta</h2>
        </header>
        <div className="capture-bill-lines">
          <Notice>{error}</Notice>
          <p className="muted capture-empty">
            {loading
              ? 'Consultando cuenta…'
              : 'Selecciona una cuenta o abre una nueva para comenzar.'}
          </p>
        </div>
      </aside>
    );
  }
  const account = bill.account;
  const writable =
    account.state === 'open' && (account.mode !== 'takeaway' || can('takeaway.manage'));
  const capturable = writable && can('orders.capture');
  const quoted = quotedDraftLines(quote, persistedDraft, persistedRevision, account.id);
  const pending = !!draftView && (draftView.pending || draftView.sending);
  // La cotización corresponde exactamente al borrador visible: no hay toques sin guardar.
  const clean =
    !pending &&
    localDraft.length === persistedDraft.length &&
    localDraft.every((item, index) => {
      const persisted = persistedDraft[index];
      return !!persisted && sameDraftItem(item, persisted);
    }) &&
    !!quote &&
    quote.account_id === account.id &&
    quote.revision === persistedRevision;
  // Estado de la sincronización del borrador (los productos nuevos se guardan solos).
  const status = draftView?.sending
    ? 'Guardando…'
    : draftView?.pending
      ? 'Por guardar…'
      : localDraft.length > 0
        ? 'Guardado'
        : '';

  return (
    <aside className="capture-bill" aria-label="Cuenta actual">
      {dialog}
      <header className="capture-bill-header">
        {isPendingAccount(account) ? (
          <div>
            <h2>Cuenta nueva</h2>
            <p className="muted text-xs">Se abre al guardar el primer producto.</p>
          </div>
        ) : (
          <div>
            <h2>Cuenta #{account.folio}</h2>
            <p className="muted text-xs">
              Preparación:{' '}
              {preparationLabels[account.preparation_state] ?? account.preparation_state}
              {account.mode !== 'takeaway' && ` · Pago: ${paymentDescription(account)}`}
            </p>
          </div>
        )}
        <p className="muted text-xs capture-sync-status" role="status">
          {status}
        </p>
      </header>
      <div className="capture-bill-lines">
        <Notice>{error}</Notice>
        {onCapture && capturable && (
          <button
            type="button"
            className="secondary-button capture-open-button"
            onClick={onCapture}
          >
            <IconLabel icon="edit">Capturar productos en esta cuenta</IconLabel>
          </button>
        )}
        {account.state === 'merged' && (
          <Notice>
            Esta cuenta se combinó y conserva su historial.{' '}
            {account.merged_into_id && (
              <button
                type="button"
                className="secondary-button"
                onClick={() => onMergedTarget(account.merged_into_id!)}
              >
                Ver cuenta resultante
              </button>
            )}
          </Notice>
        )}
        {account.state === 'closed' && <Notice>{closedDescription(account)}</Notice>}
        {account.state === 'voided' && (
          <Notice>Cuenta anulada sin productos activos. Su historial permanece disponible.</Notice>
        )}
        {account.mode === 'takeaway' && (
          <div className="takeaway-dimensions">
            <span>Pago: {paymentDescription(account)}</span>
            <span>
              Entrega: {deliveryStateDescriptions[account.delivery_state] ?? account.delivery_state}
            </span>
            {account.customer_name && (
              <strong>
                {account.customer_name}
                {account.phone ? ` · ${account.phone}` : ''}
              </strong>
            )}
            <TakeawayDelivery
              account={account}
              timezone={bill.timezone}
              can={can}
              disabled={blocked}
              onCollect={onDeliverCheckout}
            />
          </div>
        )}
        {draftView?.conflict && (
          <div className="capture-conflict" role="alert">
            <p>
              Otra terminal cambió esta cuenta mientras agregabas productos. Tus productos nuevos se
              conservan en esta terminal: revisa la cuenta actual y decide.
            </p>
            <div className="button-row">
              <button type="button" className="primary-button" onClick={onKeepMine}>
                Conservar mis productos nuevos
              </button>
              <button
                type="button"
                className="secondary-button"
                onClick={() =>
                  void confirm({
                    title: 'Descartar productos nuevos',
                    message:
                      'Se quitarán de esta terminal los productos nuevos que aún no se guardaron en la cuenta.',
                    confirmLabel: 'Descartar productos',
                    cancelLabel: 'Conservarlos',
                    tone: 'danger',
                  }).then((confirmed) => {
                    if (confirmed) onDiscard();
                  })
                }
              >
                Descartar mis cambios
              </button>
            </div>
          </div>
        )}
        {draftView?.error && !draftView.uncertain && <Notice>{draftView.error}</Notice>}
        {localDraft.length > 0 && <Notice>{quoteError}</Notice>}
        <ConfirmedLines
          lines={bill.lines}
          editable={writable}
          can={can}
          disabled={blocked || linesLocked}
          onEdit={onLineEdit}
          onCancel={onLineCancel}
        />
        <DraftLines
          items={localDraft}
          quoted={quoted}
          lookups={lookups}
          editable={capturable}
          disabled={blocked || linesLocked || !!draftView?.conflict}
          onIncrement={onIncrement}
          onDecrement={onDecrement}
          onCustomize={onCustomize}
        />
        {bill.lines.length === 0 && localDraft.length === 0 && (
          <p className="muted capture-empty">La cuenta aún no tiene productos.</p>
        )}
        {account.notes && <p className="bill-account-notes">Observaciones: {account.notes}</p>}
      </div>
      <BillFooter
        account={account}
        localDraft={localDraft}
        activeLines={bill.lines.filter((line) => line.state === 'active').length}
        quote={quote}
        clean={clean}
        can={can}
        blocked={blocked}
        linesLocked={linesLocked}
        sending={sending}
        conflict={!!draftView?.conflict}
        paymentPending={!!paymentPending}
        onAction={onAction}
        onSend={onSend}
        onCheckout={onCheckout}
      />
    </aside>
  );
}
