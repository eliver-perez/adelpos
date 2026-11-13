import { useEffect, useId, useRef, useState } from 'react';
import { Accordion } from '../../../components/Accordion';
import type { Lookups, ProductBreakdown } from '../../../services/catalog';
import { Icon, IconLabel } from '../../../components/Icon';
import { preparationLabels } from '../../../services/kitchen';
import { formatDeduction, formatMoney, formatSignedMoney } from '../../../services/money';
import type { ConfirmedLine, DraftItem } from '../../../services/restaurant';
import {
  chosenComplements,
  describeBreakdown,
  describeSelection,
  nonZeroAmount,
} from './captureRules';

// Importe y desglose de Go de una partida nueva, si la cotización corresponde a su contenido.
export interface QuotedDraftLine {
  total: string;
  breakdown: ProductBreakdown;
}

// Estado de cocina de un producto enviado, una sola vez por producto (etapa 9D, §4.4): «Cancelado»;
// «Enviado · sin confirmar en cocina» si su liberación quedó bloqueada (antes «Pendiente de
// liberación · revisar cocina», repetido en el encabezado); si no, «En cocina» con su preparación.
function kitchenState(line: ConfirmedLine) {
  if (line.state === 'cancelled') {
    return { tag: 'Cancelado', tone: 'capture-tag-cancelled', detail: '' };
  }
  if (line.preparation_state === 'pending_release') {
    return { tag: preparationLabels.pending_release!, tone: 'capture-tag-unconfirmed', detail: '' };
  }
  return { tag: 'En cocina', tone: '', detail: preparationLabels[line.preparation_state ?? ''] };
}

// «Desglose» de un producto enviado (etapa 9D, §4.4): solo los complementos elegidos, los cargos y
// el descuento que calculó Go, cuando los hay; sin número de versión ni «Descuento $0.00». Sin nada
// que desglosar no se muestra.
function LineBreakdown({ line }: { line: ConfirmedLine }) {
  const breakdown = line.quote_line;
  const chosen = chosenComplements(breakdown);
  const charges = [
    { label: 'Cargo por ingredientes', value: breakdown.ingredient_extras },
    { label: 'Cargo por complementos', value: breakdown.group_extras },
    { label: 'Cargo por componentes', value: breakdown.component_extras },
  ].filter((charge) => nonZeroAmount(charge.value));
  const discounted = nonZeroAmount(line.totals.discount);
  if (chosen.length === 0 && charges.length === 0 && !discounted) {
    return null;
  }
  return (
    <Accordion
      title="Desglose"
      summary={discounted ? `Descuento ${formatDeduction(line.totals.discount)}` : undefined}
    >
      <ul className="capture-line-breakdown">
        {chosen.map((entry, index) => (
          <li key={`elegido-${index}`}>{entry}</li>
        ))}
        {charges.map((charge) => (
          <li key={charge.label} className="capture-line-breakdown-amount">
            <span>{charge.label}</span>
            <span>{formatSignedMoney(charge.value)}</span>
          </li>
        ))}
        {discounted && (
          <li className="capture-line-breakdown-amount">
            <span>Descuento</span>
            <span>{formatDeduction(line.totals.discount)}</span>
          </li>
        )}
      </ul>
    </Accordion>
  );
}

function ConfirmedLineItem({
  line,
  canEdit,
  canCancel,
  disabled,
  open,
  onToggle,
  onClose,
  onEdit,
  onCancel,
}: {
  line: ConfirmedLine;
  canEdit: boolean;
  canCancel: boolean;
  disabled: boolean;
  open: boolean;
  onToggle: () => void;
  onClose: () => void;
  onEdit: (line: ConfirmedLine) => void;
  onCancel: (line: ConfirmedLine) => void;
}) {
  const container = useRef<HTMLElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const menuID = useId();
  const cancelled = line.state === 'cancelled';
  const actionable = !cancelled && (canEdit || canCancel);
  const title = `${line.quantity} × ${line.quote_line.name}`;
  const detail = describeBreakdown(line.quote_line);
  const kitchen = kitchenState(line);

  // El menú se cierra al tocar fuera del producto o con Escape (también si se abrió tocando el
  // renglón, sin foco dentro; la capa global de teclado no actúa mientras está abierto).
  useEffect(() => {
    if (!open) return;
    function outside(event: PointerEvent) {
      if (container.current && !container.current.contains(event.target as Node)) {
        onClose();
      }
    }
    function escape(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        onClose();
      }
    }
    // El menú abierto queda a la vista dentro de la lista de productos.
    menu.current?.scrollIntoView?.({ block: 'nearest' });
    document.addEventListener('pointerdown', outside);
    document.addEventListener('keydown', escape);
    return () => {
      document.removeEventListener('pointerdown', outside);
      document.removeEventListener('keydown', escape);
    };
  }, [open, onClose]);

  return (
    <article
      ref={container}
      className={`capture-line capture-line-confirmed ${cancelled ? 'capture-line-cancelled' : ''} ${open ? 'capture-line-menu-open' : ''}`}
      aria-label={title}
    >
      {/* Etapa 9D (§4.5): tocar el renglón (o «⋯») abre el menú del producto. El renglón es el área
          táctil grande; «⋯» es el control accesible con el teclado y el lector de pantalla. */}
      <div
        className={`capture-line-row ${actionable && !disabled ? 'capture-line-tappable' : ''}`}
        onClick={actionable && !disabled ? onToggle : undefined}
      >
        <div className="capture-line-body">
          <div className="capture-line-heading">
            <strong className="capture-line-name">{title}</strong>
            <strong className="capture-line-total">{formatMoney(line.totals.total)}</strong>
          </div>
          <div className="capture-line-meta">
            <span className={`capture-tag ${kitchen.tone}`}>{kitchen.tag}</span>
            {kitchen.detail && <span className="muted">{kitchen.detail}</span>}
          </div>
          {detail.length > 0 && <p className="capture-line-detail">{detail.join(' · ')}</p>}
        </div>
        {actionable && (
          <button
            type="button"
            className="secondary-button capture-line-more"
            aria-label={`Acciones de ${title}`}
            aria-expanded={open}
            aria-controls={open ? menuID : undefined}
            disabled={disabled}
            onClick={(event) => {
              event.stopPropagation();
              onToggle();
            }}
          >
            <Icon name="more" size={20} />
          </button>
        )}
      </div>
      {open && actionable && (
        <div
          ref={menu}
          id={menuID}
          className="capture-line-menu"
          role="group"
          aria-label={`Acciones de ${title}`}
        >
          {canEdit && (
            <button
              type="button"
              className="secondary-button"
              disabled={disabled}
              onClick={() => {
                onClose();
                onEdit(line);
              }}
            >
              <IconLabel icon="edit">Modificar</IconLabel>
            </button>
          )}
          {canCancel && (
            <button
              type="button"
              className="danger-button"
              disabled={disabled}
              onClick={() => {
                onClose();
                onCancel(line);
              }}
            >
              <IconLabel icon="trash">Cancelar unidades</IconLabel>
            </button>
          )}
        </div>
      )}
      <LineBreakdown line={line} />
    </article>
  );
}

// Productos enviados a cocina. Etapa 9D (§4.5): «Modificar» y «Cancelar unidades» (en rojo) ya no
// ocupan un renglón en cada producto; van en el menú del producto, uno abierto a la vez.
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
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const close = useRef(() => setMenuFor(null)).current;
  // Mientras otra acción ocupa la cuenta (un panel, un envío) el menú no queda abierto.
  useEffect(() => {
    if (disabled) setMenuFor(null);
  }, [disabled]);
  return (
    <>
      {lines.map((line) => (
        <ConfirmedLineItem
          key={`${line.version_id}-${line.state}`}
          line={line}
          canEdit={editable && can('orders.edit')}
          // Etapa 9F (§7.2): sin «Cancelar partidas» se puede pedir la autorización de un supervisor
          // con el permiso de abrir cuentas (la ruta de Go acepta cualquiera de los dos).
          canCancel={editable && (can('orders.cancel') || can('accounts.open'))}
          disabled={disabled}
          open={menuFor === line.version_id && line.state !== 'cancelled'}
          onToggle={() =>
            setMenuFor((current) => (current === line.version_id ? null : line.version_id))
          }
          onClose={close}
          onEdit={onEdit}
          onCancel={onCancel}
        />
      ))}
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
