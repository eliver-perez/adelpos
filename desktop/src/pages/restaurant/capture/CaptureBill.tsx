import type { ReactNode } from 'react';
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
import { AccountCancellationRecord } from '../CancelAccount';
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

// Encabezado de la cuenta: preparación y, en el salón, el pago. Etapa 9D (§4.4): «Enviado · sin
// confirmar en cocina» se indica una sola vez en cada producto y ya no se repite aquí.
function headerDescription(account: Account) {
  const parts: string[] = [];
  if (account.preparation_state !== 'pending_release') {
    parts.push(
      `Preparación: ${preparationLabels[account.preparation_state] ?? account.preparation_state}`,
    );
  }
  if (account.mode !== 'takeaway') {
    parts.push(`Pago: ${paymentDescription(account)}`);
  }
  return parts.join(' · ');
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
  notice,
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
  // Aviso de la captura sin turno (etapa 7.1), junto a los totales y «Cobrar». Ocupa espacio de la
  // lista de partidas, no de la pantalla: el menú «Más…» conserva su altura.
  notice?: ReactNode;
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
  const pendingAccount = isPendingAccount(account);
  const writable =
    account.state === 'open' && (account.mode !== 'takeaway' || can('takeaway.manage'));
  const capturable = writable && can('orders.capture');
  // Etapa 9D (§4.2): fuera de la captura, una cuenta descartable para Go ofrece «Descartar cuenta
  // vacía» a la vista (dentro de la captura está en «Más…»).
  const discardVisible =
    !!onCapture && writable && account.discardable && can('accounts.open') && !pendingAccount;
  const header = pendingAccount ? '' : headerDescription(account);
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
        {pendingAccount ? (
          <div>
            <h2>{account.mode === 'takeaway' ? 'Pedido nuevo' : 'Cuenta nueva'}</h2>
            <p className="muted text-xs">Se abre al guardar el primer producto.</p>
          </div>
        ) : (
          <div>
            <h2>Cuenta #{account.folio}</h2>
            {header && <p className="muted text-xs">{header}</p>}
          </div>
        )}
        <p className="muted text-xs capture-sync-status" role="status">
          {status}
        </p>
      </header>
      <div className="capture-bill-lines">
        <Notice>{error}</Notice>
        {/* Etapa 9D (§3.3, decisión 7): fuera de la captura es el único botón principal del
            detalle de la cuenta; «Enviar a cocina» y «Cobrar» quedan con contorno. */}
        {onCapture && capturable && (
          <button type="button" className="primary-button capture-open-button" onClick={onCapture}>
            <IconLabel icon="edit">Capturar productos en esta cuenta</IconLabel>
          </button>
        )}
        {discardVisible && (
          <div className="bill-discard">
            <p className="muted text-xs">Sin productos enviados a cocina ni pagos.</p>
            <button
              type="button"
              className="danger-button bill-discard-button"
              disabled={blocked}
              onClick={() => onAction('discardAccount')}
            >
              <IconLabel icon="trash">Descartar cuenta vacía</IconLabel>
            </button>
          </div>
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
        {/* Etapa 9D (§5): una cuenta cancelada muestra su registro (motivo, quién canceló y quién
            autorizó, unidades e importe que calculó Go). */}
        {account.state === 'voided' && (
          <AccountCancellationRecord cancellation={bill.cancellation} timezone={bill.timezone} />
        )}
        {account.mode === 'takeaway' && !pendingAccount && (
          <div className="takeaway-dimensions">
            {/* Etapa 9D (§4.5): pago y entrega en un renglón para que quepan más productos. */}
            <span>
              Pago: {paymentDescription(account)} · Entrega:{' '}
              {deliveryStateDescriptions[account.delivery_state] ?? account.delivery_state}
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
                className="danger-button"
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
      {notice}
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
        emphasis={onCapture ? 'overview' : 'capture'}
      />
    </aside>
  );
}
