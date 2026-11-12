import { useEffect, useRef } from 'react';
import { Accordion } from '../../../components/Accordion';
import { ProductDetails } from '../../../components/catalog/QuoteBreakdown';
import type { Lookups, ProductBreakdown } from '../../../services/catalog';
import { IconLabel } from '../../../components/Icon';
import { preparationLabels } from '../../../services/kitchen';
import { formatMoney } from '../../../services/money';
import type { ConfirmedLine, DraftItem } from '../../../services/restaurant';
import { describeBreakdown, describeSelection } from './captureRules';

// Importe y desglose de Go de una partida nueva, si la cotización corresponde a su contenido.
export interface QuotedDraftLine {
  total: string;
  breakdown: ProductBreakdown;
}

export function ConfirmedLines({
  lines,
  editable,
  can,
  disabled,
  onEdit,
  onCancel,
}: {
  lines: ConfirmedLine[];
  editable: boolean;
  can: (permission: string) => boolean;
  disabled: boolean;
  onEdit: (line: ConfirmedLine) => void;
  onCancel: (line: ConfirmedLine) => void;
}) {
  return (
    <>
      {lines.map((line) => {
        const cancelled = line.state === 'cancelled';
        const detail = describeBreakdown(line.quote_line);
        const preparation = preparationLabels[line.preparation_state ?? ''];
        return (
          <article
            className={`capture-line capture-line-confirmed ${cancelled ? 'capture-line-cancelled' : ''}`}
            key={`${line.version_id}-${line.state}`}
            aria-label={`${line.quantity} × ${line.quote_line.name}`}
          >
            <div className="capture-line-heading">
              <strong className="capture-line-name">
                {line.quantity} × {line.quote_line.name}
              </strong>
              <strong className="capture-line-total">{formatMoney(line.totals.total)}</strong>
            </div>
            <div className="capture-line-meta">
              <span className={`capture-tag ${cancelled ? 'capture-tag-cancelled' : ''}`}>
                {cancelled ? 'Cancelado' : 'En cocina'}
              </span>
              {!cancelled && preparation && <span className="muted">{preparation}</span>}
            </div>
            {detail.length > 0 && <p className="capture-line-detail">{detail.join(' · ')}</p>}
            {!cancelled && editable && (can('orders.edit') || can('orders.cancel')) && (
              <div className="capture-line-actions">
                {can('orders.edit') && (
                  <button
                    type="button"
                    className="secondary-button"
                    disabled={disabled}
                    onClick={() => onEdit(line)}
                  >
                    <IconLabel icon="edit">Modificar</IconLabel>
                  </button>
                )}
                {can('orders.cancel') && (
                  <button
                    type="button"
                    className="danger-button"
                    disabled={disabled}
                    onClick={() => onCancel(line)}
                  >
                    <IconLabel icon="trash">Cancelar unidades</IconLabel>
                  </button>
                )}
              </div>
            )}
            <Accordion
              title="Desglose"
              summary={`Versión ${line.version} · Descuento ${formatMoney(line.totals.discount)}`}
            >
              <ProductDetails value={line.quote_line} />
            </Accordion>
          </article>
        );
      })}
    </>
  );
}

export function DraftLines({
  items,
  quoted,
  lookups,
  editable,
  disabled,
  onIncrement,
  onDecrement,
  onCustomize,
}: {
  items: DraftItem[];
  quoted: Map<string, QuotedDraftLine>;
  lookups: Lookups | null;
  editable: boolean;
  disabled: boolean;
  onIncrement: (item: DraftItem) => void;
  onDecrement: (item: DraftItem) => void;
  onCustomize: (item: DraftItem) => void;
}) {
  const last = useRef<HTMLElement>(null);
  const units = items.reduce((total, item) => total + item.quantity, 0);
  const previousUnits = useRef(units);

  // Al agregar un producto, la partida nueva más reciente queda a la vista.
  useEffect(() => {
    if (units > previousUnits.current) {
      last.current?.scrollIntoView?.({ block: 'nearest' });
    }
    previousUnits.current = units;
  }, [units]);

  return (
    <>
      {items.map((item, index) => {
        const name =
          lookups?.products.find((product) => product.id === item.product_id)?.name ??
          'Producto del catálogo';
        const quote = quoted.get(item.id);
        const detail = quote
          ? describeBreakdown(quote.breakdown)
          : describeSelection(item, lookups);
        return (
          <article
            className="capture-line capture-line-new"
            key={item.id}
            ref={index === items.length - 1 ? last : undefined}
            aria-label={`${item.quantity} × ${name}`}
          >
            <div className="capture-line-heading">
              <strong className="capture-line-name">{name}</strong>
              <strong className="capture-line-total" aria-busy={!quote}>
                {quote ? formatMoney(quote.total) : '…'}
              </strong>
            </div>
            {detail.length > 0 && <p className="capture-line-detail">{detail.join(' · ')}</p>}
            <div className="capture-line-controls">
              <span className="capture-tag capture-tag-new">Nuevo</span>
              <button
                type="button"
                className="secondary-button capture-step"
                aria-label={`Quitar una unidad de ${name}`}
                disabled={!editable || disabled}
                onClick={() => onDecrement(item)}
              >
                −
              </button>
              <output className="capture-line-quantity" aria-label={`Cantidad de ${name}`}>
                {item.quantity}
              </output>
              <button
                type="button"
                className="secondary-button capture-step"
                aria-label={`Agregar una unidad de ${name}`}
                disabled={!editable || disabled}
                onClick={() => onIncrement(item)}
              >
                +
              </button>
              <button
                type="button"
                className="secondary-button capture-line-customize"
                disabled={!editable || disabled}
                onClick={() => onCustomize(item)}
              >
                Personalizar
              </button>
            </div>
          </article>
        );
      })}
    </>
  );
}
