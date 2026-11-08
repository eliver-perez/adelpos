import type { ReactNode } from 'react';
import { formatMoney, isZeroMoney } from '../../../services/payments';
import type { Finance, FinanceLine } from '../../../services/payments';
import type { Bill, ConfirmedLine } from '../../../services/restaurant';
import type { ProductBreakdown } from '../../../services/catalog';
import { selectableUnits } from './captureState';

export type CheckoutAccountAction = 'split' | 'transfer';

// Resume las personalizaciones relevantes de la partida confirmada sin recalcular importes.
function customizationSummary(breakdown: ProductBreakdown): string[] {
  const details: string[] = [];
  for (const ingredient of breakdown.ingredients) {
    if (ingredient.requested === ingredient.included) {
      continue;
    }
    if (ingredient.requested === '0') {
      details.push(`Sin ${ingredient.name}`);
    } else {
      details.push(`${ingredient.name} × ${ingredient.requested}`);
    }
  }
  for (const group of breakdown.groups) {
    for (const option of group.options) {
      if (option.requested === option.included) {
        continue;
      }
      if (option.requested === '0') {
        details.push(`Sin ${option.name}`);
      } else {
        details.push(`${option.name} × ${option.requested}`);
      }
    }
  }
  for (const component of breakdown.components) {
    details.push(`${component.name} × ${component.quantity}`);
  }
  return details;
}

function SummaryRow({
  label,
  value,
  variant = 'normal',
}: {
  label: ReactNode;
  value: string;
  variant?: 'normal' | 'total' | 'balance';
}) {
  return (
    <div className={`checkout-summary-row checkout-summary-${variant}`}>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

function LineSelector({
  line,
  quantity,
  onChange,
  disabled,
}: {
  line: FinanceLine;
  quantity: number;
  onChange: (quantity: number) => void;
  disabled: boolean;
}) {
  const available = selectableUnits(line);
  if (available === 0) {
    return null;
  }
  return (
    <div className="checkout-line-selector" role="group" aria-label={`Cobrar ${line.name}`}>
      <span className="muted text-xs">Cobrar</span>
      <button
        type="button"
        className="secondary-button"
        aria-label={`Quitar una unidad de ${line.name}`}
        disabled={disabled || quantity === 0}
        onClick={() => onChange(quantity - 1)}
      >
        −
      </button>
      <output aria-label={`Unidades seleccionadas de ${line.name}`}>{quantity}</output>
      <button
        type="button"
        className="secondary-button"
        aria-label={`Agregar una unidad de ${line.name}`}
        disabled={disabled || quantity >= available}
        onClick={() => onChange(quantity + 1)}
      >
        +
      </button>
      <span className="muted text-xs">de {available}</span>
    </div>
  );
}

function CheckoutLine({
  line,
  restaurantLine,
  quantity,
  selectable,
  locked,
  onChange,
}: {
  line: FinanceLine;
  restaurantLine: ConfirmedLine | undefined;
  quantity: number;
  selectable: boolean;
  locked: boolean;
  onChange: (quantity: number) => void;
}) {
  const details = restaurantLine ? customizationSummary(restaurantLine.quote_line) : [];
  const notes = restaurantLine?.request.customization?.notes ?? '';
  const promotion = restaurantLine?.quote_line.discount ?? '0.00';
  return (
    <article
      className={`checkout-line ${quantity > 0 ? 'checkout-line-selected' : ''}`}
      aria-label={`${line.quantity} × ${line.name}`}
    >
      <div className="checkout-line-heading">
        <strong>
          {line.quantity} × {line.name}
        </strong>
        <strong>{formatMoney(line.due)}</strong>
      </div>
      {details.length > 0 && <p className="checkout-line-details">{details.join(' · ')}</p>}
      {notes && <p className="checkout-line-details">Preparación: {notes}</p>}
      <div className="checkout-line-status">
        {!isZeroMoney(promotion) && (
          <span className="status-pill">Promoción −{formatMoney(promotion)}</span>
        )}
        {line.settled_quantity > 0 && (
          <span className="status-pill status-good">{line.settled_quantity} pagadas</span>
        )}
        {line.partial_quantity > 0 && (
          <span className="status-pill">{line.partial_quantity} con abono</span>
        )}
        {!isZeroMoney(line.remaining) && line.remaining !== line.due && (
          <span className="status-pill">Pendiente {formatMoney(line.remaining)}</span>
        )}
      </div>
      {selectable && (
        <LineSelector line={line} quantity={quantity} onChange={onChange} disabled={locked} />
      )}
    </article>
  );
}

// Área de cuenta y productos: partidas confirmadas, totales de Go y selección por cobrar.
export function CheckoutAccount({
  finance,
  bill,
  selections,
  onSelectionChange,
  onClearSelection,
  selectionEnabled,
  selectionLocked,
  selectionTotal,
  selectionValidating,
  accountActions,
  onAccountAction,
  onClose,
}: {
  finance: Finance;
  bill: Bill;
  selections: Record<string, number>;
  onSelectionChange: (versionID: string, quantity: number) => void;
  onClearSelection: () => void;
  selectionEnabled: boolean;
  selectionLocked: boolean;
  selectionTotal: string | null;
  selectionValidating: boolean;
  accountActions: { id: CheckoutAccountAction; label: string }[];
  onAccountAction: (action: CheckoutAccountAction) => void;
  onClose: () => void;
}) {
  const hasSelection = Object.values(selections).some((quantity) => quantity > 0);
  const restaurantLines = new Map(
    bill.lines.filter((line) => line.state === 'active').map((line) => [line.version_id, line]),
  );
  const location = finance.mode === 'takeaway' ? 'Para llevar' : (finance.table_name ?? 'Sin mesa');

  return (
    <section className="checkout-area checkout-account" aria-label="Cuenta y productos">
      <header className="checkout-area-header">
        <div className="min-w-0">
          <p className="eyebrow">
            {location} · Cuenta #{finance.folio}
          </p>
          <h2>{bill.account.customer_name || finance.name}</h2>
        </div>
        <button type="button" className="secondary-button" onClick={onClose}>
          Volver a la cuenta
        </button>
      </header>
      <div className="checkout-area-body">
        {bill.draft.length > 0 && (
          <p className="checkout-inline-note">
            La cuenta tiene productos en borrador sin confirmar; no forman parte del saldo.
          </p>
        )}
        {finance.lines.length === 0 && (
          <p className="muted text-sm">La cuenta no tiene productos confirmados por cobrar.</p>
        )}
        {finance.lines.map((line) => (
          <CheckoutLine
            key={line.version_id}
            line={line}
            restaurantLine={restaurantLines.get(line.version_id)}
            quantity={selections[line.version_id] ?? 0}
            selectable={selectionEnabled}
            locked={selectionLocked}
            onChange={(quantity) => onSelectionChange(line.version_id, quantity)}
          />
        ))}
      </div>
      <footer className="checkout-area-footer">
        {hasSelection && (
          <div className="checkout-selection-total" aria-live="polite">
            <span>Total de la selección</span>
            <strong>
              {selectionTotal && !selectionValidating ? formatMoney(selectionTotal) : 'Calculando…'}
            </strong>
            <button
              type="button"
              className="secondary-button"
              disabled={selectionLocked}
              onClick={onClearSelection}
            >
              Limpiar selección
            </button>
          </div>
        )}
        <dl className="checkout-summary">
          <SummaryRow label="Productos" value={formatMoney(finance.items_total)} />
          {!isZeroMoney(finance.item_discounts) && (
            <SummaryRow
              label="Descuentos por partida"
              value={`−${formatMoney(finance.item_discounts)}`}
            />
          )}
          {!isZeroMoney(finance.account_discounts) && (
            <SummaryRow
              label="Descuentos generales"
              value={`−${formatMoney(finance.account_discounts)}`}
            />
          )}
          {!isZeroMoney(finance.charges) && (
            <SummaryRow label="Cargos" value={formatMoney(finance.charges)} />
          )}
          <SummaryRow label="Total" value={formatMoney(finance.total)} variant="total" />
          <SummaryRow label="Abonos registrados" value={formatMoney(finance.paid)} />
          {!isZeroMoney(finance.refunded) && (
            <SummaryRow label="Devoluciones" value={formatMoney(finance.refunded)} />
          )}
          {!isZeroMoney(finance.tips) && (
            <SummaryRow label="Propinas (aparte)" value={formatMoney(finance.tips)} />
          )}
          {finance.tax.map((line) => (
            <SummaryRow
              key={line.rate}
              label={`IVA ${line.rate} % incluido`}
              value={formatMoney(line.tax)}
            />
          ))}
          <SummaryRow
            label="Saldo pendiente"
            value={formatMoney(finance.balance)}
            variant="balance"
          />
        </dl>
        {accountActions.length > 0 && (
          <div className="button-row checkout-account-actions">
            {accountActions.map((action) => (
              <button
                type="button"
                key={action.id}
                className="secondary-button"
                onClick={() => onAccountAction(action.id)}
              >
                {action.label}
              </button>
            ))}
          </div>
        )}
      </footer>
    </section>
  );
}
