import { useId, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { SegmentedControl } from '../../components/SegmentedControl';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryItem, InventoryUnit, Page } from '../../services/inventory';
import type {
  PurchaseLine,
  Supplier,
  SupplierDetail,
  SupplierItem,
  SupplierKind,
  SupplierReference,
} from '../../services/inventoryPurchasing';
import { formatDeduction, formatMoney } from '../../services/money';
import { amountShape, inventoryQuantityShape } from '../../services/numericInput';
import { validReason } from '../../services/payments';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { DecimalTextField, ItemPicker } from './InventoryFields';
import { formatQuantity } from './inventoryFormat';
import {
  emptyPurchaseLine,
  lineSignature,
  purchasePriceShape,
  receiptFolio,
  taxRateOptions,
  taxRateValue,
} from './purchasingFormat';
import type { PurchaseLineDraft } from './purchasingFormat';
import { Select } from '../../components/Select';

// Selector de proveedor con búsqueda por nombre comercial, razón social, RFC, contacto, teléfonos,
// página web o correo, integrada en la lista (etapa 9C). Consulta a Go hasta 50 coincidencias; el
// elegido se conserva aunque deje de aparecer. kind limita la modalidad: las órdenes de compra y las sugerencias solo ofrecen
// proveedores formales («Compras de mostrador» es solo para recepciones directas).
export function SupplierPicker({
  label,
  value,
  onChange,
  activeOnly = true,
  emptyLabel = 'Elige un proveedor',
  required = false,
  kind,
}: {
  label: string;
  value: SupplierReference | null;
  onChange: (supplier: SupplierReference | null) => void;
  activeOnly?: boolean;
  emptyLabel?: string;
  required?: boolean;
  kind?: SupplierKind;
}) {
  const [search, setSearch] = useState('');
  const query: Record<string, string> = { page_size: '50' };
  if (search.trim() !== '') {
    query.search = search.trim();
  }
  if (activeOnly) {
    query.active = 'true';
  }
  if (kind) {
    query.kind = kind;
  }
  const results = useResource<Page<Supplier>>('inventory_suppliers', {
    query,
    reloadEvent: inventoryChangeEvent,
  });
  const found: SupplierReference[] = (results.data?.items ?? []).map((supplier) => ({
    id: supplier.id,
    trade_name: supplier.trade_name,
    kind: supplier.kind,
    active: supplier.active,
  }));
  const options =
    value && !found.some((supplier) => supplier.id === value.id) ? [value, ...found] : found;
  return (
    <div className="inventory-picker">
      <Field label={label}>
        <Select
          value={value?.id ?? ''}
          required={required}
          placeholder={emptyLabel}
          options={[
            ...(required ? [] : [{ value: '', label: emptyLabel }]),
            ...options.map((supplier) => ({
              value: supplier.id,
              label: `${supplier.trade_name}${
                supplier.kind === 'counter' ? ' · compra de mostrador' : ''
              }${supplier.active ? '' : ' (inactivo)'}`,
            })),
          ]}
          searchLabel={`Buscar ${label.toLowerCase()}`}
          searchPlaceholder="Nombre, RFC, contacto o teléfono"
          onSearchChange={setSearch}
          loading={results.loading}
          onChange={(id) => onChange(options.find((supplier) => supplier.id === id) ?? null)}
        />
      </Field>
      {results.error && <small className="muted">{results.error}</small>}
    </div>
  );
}

// Unidad o presentación del renglón: la unidad base, otras unidades activas de su dimensión y las
// presentaciones activas (más la elegida aunque ya no esté activa).
export function MeasureSelect({
  label,
  item,
  units,
  value,
  onChange,
  disabled = false,
}: {
  label: string;
  item: InventoryItem | null;
  units: InventoryUnit[];
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  const otherUnits = item
    ? units.filter(
        (unit) =>
          unit.dimension === item.base_unit.dimension &&
          unit.id !== item.base_unit.id &&
          (unit.active || value === `unit:${unit.id}`),
      )
    : [];
  const presentations =
    item?.presentations.filter(
      (presentation) => presentation.active || value === `presentation:${presentation.id}`,
    ) ?? [];
  return (
    <Field label={label}>
      <Select
        value={value}
        disabled={disabled || !item}
        options={[
          {
            value: '',
            label: item
              ? `${item.base_unit.name} (${item.base_unit.code}) · unidad base`
              : 'Elige primero el insumo',
          },
          ...otherUnits.map((unit) => ({
            value: `unit:${unit.id}`,
            label: `${unit.name} (${unit.code})`,
          })),
          ...presentations.map((presentation) => ({
            value: `presentation:${presentation.id}`,
            label: `${presentation.name} (${formatQuantity(
              presentation.base_quantity,
              item?.base_unit,
            )})${presentation.active ? '' : ' · inactiva'}`,
          })),
        ]}
        onChange={onChange}
      />
    </Field>
  );
}

// Nombre de lo capturado («Caja con 24», «kilogramo») para las indicaciones del precio.
export function measureName(
  item: InventoryItem | null,
  measure: string,
  units: InventoryUnit[],
): string {
  if (!item) {
    return '';
  }
  if (measure.startsWith('presentation:')) {
    const id = measure.slice('presentation:'.length);
    return item.presentations.find((presentation) => presentation.id === id)?.name ?? '';
  }
  if (measure.startsWith('unit:')) {
    const id = measure.slice('unit:'.length);
    return units.find((unit) => unit.id === id)?.name.toLowerCase() ?? '';
  }
  return item.base_unit.name.toLowerCase();
}

// Último precio que el proveedor cobró por el insumo, tal como lo entrega Go.
export function lastPriceText(entry: SupplierItem | undefined, timezone: string) {
  if (!entry || entry.last_unit_price === null) {
    return '';
  }
  const parts = [`Último precio de este proveedor: ${formatMoney(entry.last_unit_price)} sin IVA`];
  if (entry.last_price_unit_label) {
    parts[0] = `${parts[0]} por ${entry.last_price_unit_label}`;
  }
  if (entry.last_price_at) {
    parts.push(serverDateTime(entry.last_price_at, timezone));
  }
  if (entry.last_receipt_folio !== null) {
    parts.push(receiptFolio(entry.last_receipt_folio));
  }
  return `${parts.join(' · ')}.`;
}

// Presentación sugerida al elegir el insumo: la habitual con el proveedor o la predeterminada de
// compra del insumo. Solo elige una opción existente; no calcula nada.
function suggestedMeasure(item: InventoryItem | null, supplierItem: SupplierItem | undefined) {
  if (!item) {
    return '';
  }
  const habitual = supplierItem?.presentation;
  if (habitual && habitual.active) {
    return `presentation:${habitual.id}`;
  }
  const purchase = item.presentations.find(
    (presentation) => presentation.active && presentation.purchase_default,
  );
  return purchase ? `presentation:${purchase.id}` : '';
}

// Importes de un renglón guardado, calculados por Go.
export function LineAmounts({ line }: { line: PurchaseLine }) {
  return (
    <dl className="inventory-facts purchase-line-amounts" aria-label="Importes del renglón">
      <div>
        <dt>Importe</dt>
        <dd>{formatMoney(line.gross)}</dd>
      </div>
      <div>
        <dt>Descuento</dt>
        <dd>{formatDeduction(line.discount)}</dd>
      </div>
      <div>
        <dt>Subtotal</dt>
        <dd>{formatMoney(line.subtotal)}</dd>
      </div>
      <div>
        <dt>IVA {line.tax_rate} %</dt>
        <dd>{formatMoney(line.tax)}</dd>
      </div>
      <div>
        <dt>Total</dt>
        <dd>
          <strong>{formatMoney(line.total)}</strong>
        </dd>
      </div>
    </dl>
  );
}

// Totales de un documento tal como los entrega Go (suma exacta de sus renglones).
export function DocumentTotals({
  label,
  subtotal,
  discount,
  tax,
  total,
  note,
}: {
  label: string;
  subtotal: string;
  discount: string;
  tax: string;
  total: string;
  note?: ReactNode;
}) {
  return (
    <section className="purchase-totals" aria-label={label}>
      <dl className="inventory-facts">
        <div>
          <dt>Subtotal</dt>
          <dd>{formatMoney(subtotal)}</dd>
        </div>
        <div>
          <dt>Descuento incluido</dt>
          <dd>{formatDeduction(discount)}</dd>
        </div>
        <div>
          <dt>IVA</dt>
          <dd>{formatMoney(tax)}</dd>
        </div>
        <div>
          <dt>Total</dt>
          <dd>
            <strong className="purchase-total-amount">{formatMoney(total)}</strong>
          </dd>
        </div>
      </dl>
      {note && <p className="muted text-xs">{note}</p>}
    </section>
  );
}

// Tasa de IVA de lista cerrada (16/8/0), con botones como en ventas.
export function TaxRateControl({
  value,
  onChange,
  label,
  disabled = false,
}: {
  value: number;
  onChange: (value: 16 | 8 | 0) => void;
  label: string;
  disabled?: boolean;
}) {
  return (
    <div className="field purchase-tax-field">
      <span>Tasa de IVA</span>
      <SegmentedControl
        label={label}
        value={String(value) as `${16 | 8 | 0}`}
        options={taxRateOptions}
        disabled={disabled}
        onChange={(next) => onChange(taxRateValue(next))}
      />
    </div>
  );
}

function PurchaseLineEditor({
  line,
  number,
  units,
  supplierItems,
  timezone,
  removable,
  onChange,
  onRemove,
}: {
  line: PurchaseLineDraft;
  number: number;
  units: InventoryUnit[];
  supplierItems: SupplierItem[];
  timezone: string;
  removable: boolean;
  onChange: (line: PurchaseLineDraft) => void;
  onRemove: () => void;
}) {
  // Un renglón guardado trae solo la referencia del insumo: se consulta el insumo completo para
  // ofrecer sus presentaciones.
  const loaded = useResource<InventoryItem>('inventory_item', {
    id: line.itemID,
    enabled: line.itemID !== '' && line.item === null,
    reloadEvent: inventoryChangeEvent,
  });
  const item = line.item ?? (loaded.data?.id === line.itemID ? loaded.data : null);
  const supplierItem = supplierItems.find((entry) => entry.item.id === line.itemID);
  const captured = measureName(item, line.measure, units);
  const lastPrice = lastPriceText(supplierItem, timezone);

  // Editar cualquier dato descarta los importes calculados del renglón guardado.
  function update(next: Partial<PurchaseLineDraft>) {
    onChange({ ...line, item, ...next, computed: null });
  }

  return (
    <fieldset className="repeat-card inventory-line purchase-line" aria-label={`Renglón ${number}`}>
      <div className="section-heading inventory-line-heading">
        <strong>Renglón {number}</strong>
        {removable && (
          <button type="button" className="secondary-button" onClick={onRemove}>
            Quitar renglón
          </button>
        )}
      </div>
      <ItemPicker
        label="Insumo"
        value={item}
        required
        emptyLabel={line.itemID !== '' && !item ? line.itemLabel : 'Elige un insumo'}
        onChange={(selected) =>
          update({
            item: selected,
            itemID: selected?.id ?? '',
            itemLabel: selected ? `${selected.name} · ${selected.code}` : '',
            measure: suggestedMeasure(
              selected,
              supplierItems.find((entry) => entry.item.id === selected?.id),
            ),
          })
        }
      />
      <div className="form-grid purchase-line-grid">
        <MeasureSelect
          label="Unidad o presentación"
          item={item}
          units={units}
          value={line.measure}
          onChange={(measure) => update({ measure })}
        />
        <DecimalTextField
          label="Cantidad"
          value={line.quantity}
          onChange={(quantity) => update({ quantity })}
          shape={inventoryQuantityShape}
          required
          hint={captured ? `En ${captured}.` : undefined}
        />
        <DecimalTextField
          label="Precio unitario sin IVA"
          value={line.unitPrice}
          onChange={(unitPrice) => update({ unitPrice })}
          shape={purchasePriceShape}
          required
          hint={lastPrice || `Por ${captured || 'unidad capturada'}; hasta 6 decimales.`}
        />
        <DecimalTextField
          label="Descuento comercial"
          value={line.discount}
          onChange={(discount) => update({ discount })}
          shape={amountShape}
          hint="En pesos, sobre el importe del renglón. Vacío: sin descuento."
        />
        <TaxRateControl
          label={`Tasa de IVA del renglón ${number}`}
          value={line.taxRate}
          onChange={(taxRate) => update({ taxRate })}
        />
      </div>
      {line.computed ? (
        <LineAmounts line={line.computed} />
      ) : (
        <p className="muted text-xs">
          Importe, IVA y total del renglón los calcula el servidor al guardar.
        </p>
      )}
    </fieldset>
  );
}

// Renglones de una orden o de una recepción directa (hasta 200, un insumo una sola vez: Go lo
// valida). Cada renglón conserva su clave aunque se quiten otros.
export function PurchaseLinesEditor({
  lines,
  units,
  supplierItems,
  timezone,
  allowEmpty,
  onChange,
}: {
  lines: PurchaseLineDraft[];
  units: InventoryUnit[];
  supplierItems: SupplierItem[];
  timezone: string;
  // Un borrador de orden puede guardarse sin renglones; una recepción directa no.
  allowEmpty: boolean;
  onChange: (lines: PurchaseLineDraft[]) => void;
}) {
  const nextKey = useRef(lines.reduce((highest, line) => Math.max(highest, line.key), 0) + 1);
  return (
    <div className="form-stack">
      {lines.map((line, index) => (
        <PurchaseLineEditor
          key={line.key}
          line={line}
          number={index + 1}
          units={units}
          supplierItems={supplierItems}
          timezone={timezone}
          removable={allowEmpty || lines.length > 1}
          onChange={(next) =>
            onChange(lines.map((entry) => (entry.key === next.key ? next : entry)))
          }
          onRemove={() => onChange(lines.filter((entry) => entry.key !== line.key))}
        />
      ))}
      {lines.length === 0 && <p className="muted text-sm">La orden aún no tiene renglones.</p>}
      <div className="button-row">
        <button
          type="button"
          className="secondary-button"
          disabled={lines.length >= 200}
          onClick={() => onChange([...lines, emptyPurchaseLine(nextKey.current++)])}
        >
          <IconLabel icon="plus">Agregar renglón</IconLabel>
        </button>
        <span className="muted text-xs">{countLabel(lines.length, 'renglón', 'renglones')}</span>
      </div>
    </div>
  );
}

// Insumos que surte el proveedor elegido (para la presentación habitual y el último precio).
export function useSupplierItems(supplierID: string) {
  const detail = useResource<SupplierDetail>('inventory_supplier', {
    id: supplierID,
    enabled: supplierID !== '',
    reloadEvent: inventoryChangeEvent,
  });
  if (!detail.data || detail.data.id !== supplierID) {
    return [];
  }
  return detail.data.items;
}

export function linesChanged(current: PurchaseLineDraft[], initial: PurchaseLineDraft[]) {
  if (current.length !== initial.length) {
    return true;
  }
  return current.some((line, index) => {
    const original = initial[index];
    return !original || lineSignature(line) !== lineSignature(original);
  });
}

// Acción con motivo obligatorio (cancelar o cerrar una orden, desvincular un gasto): el motivo
// se captura en el panel y la acción se confirma antes de enviarla. Go vuelve a validar 5–500.
export function ReasonAction({
  title,
  description,
  reasonLabel,
  actionLabel,
  confirmTitle,
  confirmMessage,
  busy,
  onConfirm,
  onCancel,
}: {
  title: string;
  description: ReactNode;
  reasonLabel: string;
  actionLabel: string;
  confirmTitle: string;
  confirmMessage: (reason: string) => ReactNode;
  busy: boolean;
  onConfirm: (reason: string) => Promise<boolean>;
  onCancel: () => void;
}) {
  const reasonID = useId();
  const [reason, setReason] = useState('');
  const [sending, setSending] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const ready = validReason(reason) && !busy && !sending;

  async function submit() {
    if (!ready) {
      return;
    }
    const confirmed = await confirm({
      title: confirmTitle,
      message: confirmMessage(reason.trim()),
      confirmLabel: actionLabel,
      cancelLabel: 'Revisar',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    setSending(true);
    try {
      const done = await onConfirm(reason.trim());
      if (done) {
        setReason('');
      }
    } finally {
      setSending(false);
    }
  }

  return (
    <section className="inventory-correction form-stack" aria-label={title}>
      <h3>{title}</h3>
      <div className="muted text-sm">{description}</div>
      <div className="field">
        <label htmlFor={reasonID}>{reasonLabel}</label>
        <textarea
          id={reasonID}
          rows={2}
          maxLength={500}
          aria-describedby={`${reasonID}-hint`}
          value={reason}
          onChange={(event) => setReason(event.target.value)}
        />
        <small id={`${reasonID}-hint`} className="muted">
          Entre 5 y 500 caracteres.
        </small>
      </div>
      <div className="button-row">
        <button
          type="button"
          className="danger-button"
          disabled={!ready}
          onClick={() => void submit()}
        >
          {sending ? 'Registrando…' : actionLabel}
        </button>
        <button type="button" className="secondary-button" onClick={onCancel}>
          Volver
        </button>
      </div>
      {dialog}
    </section>
  );
}
