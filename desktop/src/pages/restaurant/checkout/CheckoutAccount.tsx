import type { ReactNode } from 'react';
import { IconLabel } from '../../../components/Icon';
import { QuantityStepper } from '../../../components/QuantityStepper';
import { formatDeduction } from '../../../services/money';
import { formatMoney, isZeroMoney } from '../../../services/payments';
import { countLabel } from '../../../services/plural';
import type { Finance, FinanceLine } from '../../../services/payments';
import type { Bill, ConfirmedLine } from '../../../services/restaurant';
import type { ProductBreakdown } from '../../../services/catalog';

// Selección de unidades por partida para la operación vigente (cobro o devolución). Las
// cantidades son enteras; el importe de la selección siempre lo calcula Go.
export interface LineSelection {
  verb: string;
  available: (line: FinanceLine) => number;
  selections: Record<string, number>;
  onChange: (versionID: string, quantity: number) => void;
  onClear: () => void;
  locked: boolean;
  totalLabel: string;
  total: string | null;
  validating: boolean;
}

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

function CheckoutLine({
  line,
  restaurantLine,
  selection,
}: {
  line: FinanceLine;
  restaurantLine: ConfirmedLine | undefined;
  selection: LineSelection | null;
}) {
  const details = restaurantLine ? customizationSummary(restaurantLine.quote_line) : [];
  const notes = restaurantLine?.request.customization?.notes || line.notes || '';
  const quantity = selection?.selections[line.version_id] ?? 0;
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
          <span className="status-pill">Promoción {formatDeduction(promotion)}</span>
        )}
        {line.settled_quantity > 0 && (
          <span className="status-pill status-good">
            {countLabel(line.settled_quantity, 'pagada', 'pagadas')}
          </span>
        )}
        {line.partial_quantity > 0 && (
          <span className="status-pill">{line.partial_quantity} con abono</span>
        )}
        {!isZeroMoney(line.remaining) && line.remaining !== line.due && (
          <span className="status-pill">Pendiente {formatMoney(line.remaining)}</span>
        )}
      </div>
      {selection && (
        <QuantityStepper
          verb={selection.verb}
          name={line.name}
          quantity={quantity}
          max={selection.available(line)}
          disabled={selection.locked}
          onChange={(next) => selection.onChange(line.version_id, next)}
        />
      )}
    </article>
  );
}

// Área de cuenta y productos: partidas confirmadas, totales de Go y selección de la operación.
export function CheckoutAccount({
  finance,
  bill,
  selection,
  onClose,
}: {
  finance: Finance;
  bill: Bill;
  selection: LineSelection | null;
  onClose: () => void;
}) {
  const hasSelection =
    !!selection && Object.values(selection.selections).some((quantity) => quantity > 0);
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
          <IconLabel icon="back">Volver a la cuenta</IconLabel>
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
            selection={selection}
          />
        ))}
      </div>
      <footer className="checkout-area-footer">
        {selection && hasSelection && (
          <div className="checkout-selection-total" aria-live="polite">
            <span>{selection.totalLabel}</span>
            <strong>
              {selection.total && !selection.validating
                ? formatMoney(selection.total)
                : 'Calculando…'}
            </strong>
            <button
              type="button"
              className="secondary-button"
              disabled={selection.locked}
              onClick={selection.onClear}
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
              value={formatDeduction(finance.item_discounts)}
            />
          )}
          {!isZeroMoney(finance.account_discounts) && (
            <SummaryRow
              label="Descuentos generales"
              value={formatDeduction(finance.account_discounts)}
            />
          )}
          {!isZeroMoney(finance.charges) && (
            <SummaryRow label="Cargos" value={formatMoney(finance.charges)} />
          )}
          <SummaryRow label="Total" value={formatMoney(finance.total)} variant="total" />
          {/* Go entrega lo pagado neto de devoluciones; lo devuelto se muestra solo como referencia. */}
          <SummaryRow
            label={isZeroMoney(finance.refunded) ? 'Abonos registrados' : 'Pagado neto'}
            value={formatMoney(finance.paid)}
          />
          {!isZeroMoney(finance.refunded) && (
            <SummaryRow label="Devuelto" value={formatMoney(finance.refunded)} />
          )}
          {!isZeroMoney(finance.tips) && (
            <SummaryRow label="Propinas (fuera de la venta)" value={formatMoney(finance.tips)} />
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
      </footer>
    </section>
  );
}
