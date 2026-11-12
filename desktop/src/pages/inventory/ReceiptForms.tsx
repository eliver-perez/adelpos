import { useState } from 'react';
import type { FormEvent, ReactNode } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Notice } from '../../components/Forms';
import { SegmentedControl } from '../../components/SegmentedControl';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import { APIError } from '../../services/api';
import type { InventoryExecute, InventoryUnit, Page, Warehouse } from '../../services/inventory';
import { recordReceipt } from '../../services/inventoryPurchasing';
import type {
  DirectReceiptInput,
  OrderReceiptInput,
  OrderReceiptLineInput,
  PurchaseOrder,
  PurchaseOrderLine,
  PurchaseTaxRate,
  Receipt,
  Supplier,
  SupplierReference,
} from '../../services/inventoryPurchasing';
import { amountShape, inventoryQuantityShape } from '../../services/numericInput';
import { notifyError, notifySuccess, notifyWarning } from '../../services/notify';
import { countLabel, pluralWord } from '../../services/plural';
import { DecimalTextField } from './InventoryFields';
import { formatQuantity } from './inventoryFormat';
import {
  PurchaseLinesEditor,
  SupplierPicker,
  TaxRateControl,
  useSupplierItems,
} from './PurchaseFields';
import {
  emptyPurchaseLine,
  isZeroText,
  optionalText,
  orderFolio,
  purchaseLineInput,
  purchasePriceShape,
  receiptFolio,
  uncertainResult,
} from './purchasingFormat';
import type { PurchaseLineDraft } from './purchasingFormat';
import { Select } from '../../components/Select';

// Formularios de recepción (inventory.purchases.receive): de una orden emitida o parcial y directa
// sin orden (además inventory.purchases.create). Cada envío lleva su operation_id: un reintento
// con el mismo identificador nunca registra dos veces. Si el resultado queda incierto, el
// formulario se bloquea hasta que se recupere desde el aviso de la sección (mismo identificador),
// para no volver a enviar la recepción con otro.

interface OrderReceiptLineDraft {
  lineNo: number;
  quantity: string;
  unitPrice: string;
  discount: string;
  taxRate: PurchaseTaxRate;
}

// Lo pendiente de cada renglón se propone tal como lo entrega Go; precio y tasa, los de la orden.
function orderReceiptDrafts(order: PurchaseOrder): OrderReceiptLineDraft[] {
  return order.lines.map((line) => ({
    lineNo: line.line_no,
    quantity: isZeroText(line.pending_quantity) ? '' : line.pending_quantity,
    unitPrice: line.unit_price,
    discount: '',
    taxRate: line.tax_rate,
  }));
}

function receivedLines(drafts: OrderReceiptLineDraft[]) {
  return drafts.filter((draft) => !isZeroText(draft.quantity));
}

// Cuerpo de la recepción de una orden: solo los renglones con cantidad; el precio y la tasa viajan
// solo si se cambiaron (factura real) y el descuento solo si se capturó (si no, Go prorratea el
// de la orden por lo recibido).
export function orderReceiptInput(
  order: PurchaseOrder,
  drafts: OrderReceiptLineDraft[],
  header: ReceiptHeader,
  allowExcess: boolean,
): OrderReceiptInput {
  const lines: OrderReceiptLineInput[] = receivedLines(drafts).map((draft) => {
    const ordered = order.lines.find((line) => line.line_no === draft.lineNo);
    const input: OrderReceiptLineInput = { order_line_no: draft.lineNo, quantity: draft.quantity };
    if (ordered && draft.unitPrice !== ordered.unit_price) {
      input.unit_price = draft.unitPrice;
    }
    if (ordered && draft.taxRate !== ordered.tax_rate) {
      input.tax_rate = draft.taxRate;
    }
    if (draft.discount.trim() !== '') {
      input.discount = draft.discount;
    }
    return input;
  });
  const input: OrderReceiptInput = {
    purchase_order_id: order.id,
    warehouse_id: header.warehouseID === '' ? null : header.warehouseID,
    lines,
  };
  if (optionalText(header.reference)) {
    input.supplier_reference = header.reference;
  }
  if (optionalText(header.notes)) {
    input.notes = header.notes;
  }
  if (allowExcess) {
    input.allow_excess = true;
  }
  return input;
}

export function directReceiptInput(
  supplier: SupplierReference | null,
  lines: PurchaseLineDraft[],
  header: ReceiptHeader,
  counterStore = '',
): DirectReceiptInput {
  const input: DirectReceiptInput = {
    supplier_id: supplier?.id ?? '',
    warehouse_id: header.warehouseID === '' ? null : header.warehouseID,
    lines: lines.map(purchaseLineInput),
  };
  if (optionalText(header.reference)) {
    input.supplier_reference = header.reference;
  }
  // El comercio solo describe una compra de mostrador (Go lo rechaza con un proveedor formal).
  if (supplier?.kind === 'counter' && optionalText(counterStore)) {
    input.counter_store = counterStore;
  }
  if (optionalText(header.notes)) {
    input.notes = header.notes;
  }
  return input;
}

interface ReceiptHeader {
  warehouseID: string;
  reference: string;
  notes: string;
}

// Almacén, referencia del proveedor (factura o remisión) y observaciones de la recepción.
function ReceiptHeaderFields({
  header,
  warehouses,
  onChange,
}: {
  header: ReceiptHeader;
  warehouses: Warehouse[];
  onChange: (header: ReceiptHeader) => void;
}) {
  const offered = warehouses.filter(
    (warehouse) => warehouse.active || warehouse.id === header.warehouseID,
  );
  return (
    <>
      <div className="form-grid">
        <Field label="Almacén de la recepción" hint="Donde entra la mercancía.">
          <Select
            required
            value={header.warehouseID}
            options={offered.map((warehouse) => ({
              value: warehouse.id,
              label: `${warehouse.name}${warehouse.active ? '' : ' (inactivo)'}`,
            }))}
            onChange={(warehouseID) => onChange({ ...header, warehouseID })}
          />
        </Field>
        <Field label="Referencia del proveedor" hint="Factura o remisión; opcional, una línea.">
          <input
            maxLength={60}
            autoComplete="off"
            value={header.reference}
            onChange={(event) => onChange({ ...header, reference: event.target.value })}
          />
        </Field>
      </div>
      <Field label="Observaciones de la recepción" hint="Opcional; hasta 2000 caracteres.">
        <textarea
          rows={2}
          maxLength={2000}
          value={header.notes}
          onChange={(event) => onChange({ ...header, notes: event.target.value })}
        />
      </Field>
    </>
  );
}

function defaultWarehouse(warehouses: Warehouse[], preferred: string) {
  if (preferred !== '' && warehouses.some((warehouse) => warehouse.id === preferred)) {
    return preferred;
  }
  const active = warehouses.filter((warehouse) => warehouse.active);
  return (active.find((warehouse) => warehouse.is_default) ?? active[0])?.id ?? '';
}

// Resultado de un envío: registrada, rechazada por exceder lo pendiente (con el mensaje de Go,
// para confirmarlo) o fallida (el error ya se avisó).
type SubmissionOutcome =
  { kind: 'recorded' } | { kind: 'excess'; message: string } | { kind: 'failed' };

// Envío de una recepción con su manejo de resultados: avisos de Go (excedente), resultado
// incierto (bloquea el formulario) y errores en español.
function useReceiptSubmission(execute: InventoryExecute) {
  const [sending, setSending] = useState(false);
  const [uncertain, setUncertain] = useState(false);

  async function send(
    input: OrderReceiptInput | DirectReceiptInput,
    onRecorded: (receipt: Receipt) => void,
  ): Promise<SubmissionOutcome> {
    setSending(true);
    try {
      const receipt = await recordReceipt(execute, input);
      notifySuccess(
        `${receiptFolio(receipt.folio)} registrada: ${countLabel(receipt.line_count, 'renglón', 'renglones')} en «${receipt.warehouse.name}».`,
      );
      for (const warning of receipt.warnings) {
        notifyWarning(warning.message);
      }
      onRecorded(receipt);
      return { kind: 'recorded' };
    } catch (problem) {
      if (problem instanceof APIError && problem.code === 'RECEIPT_EXCEEDS_PENDING') {
        // El exceso se confirma explícitamente y se vuelve a enviar con allow_excess.
        return { kind: 'excess', message: problem.message };
      }
      if (uncertainResult(problem)) {
        setUncertain(true);
      }
      notifyError(problem, 'No se pudo registrar la recepción.');
      return { kind: 'failed' };
    } finally {
      setSending(false);
    }
  }

  return { send, sending, uncertain };
}

function UncertainReceipt() {
  return (
    <div className="inventory-negative-notice" role="status">
      <p>
        No se pudo confirmar si la recepción quedó registrada. Recupérala con «Reintentar sin
        duplicar» (arriba): así nunca se registra dos veces. Después consulta el listado de
        recepciones antes de capturarla de nuevo.
      </p>
    </div>
  );
}

function ReceiptPanel({
  title,
  description,
  closeLabel,
  onClose,
  children,
}: {
  title: string;
  description: ReactNode;
  closeLabel: string;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <section className="panel form-stack" aria-label={title}>
      <div className="section-heading">
        <div>
          <h2>{title}</h2>
          <p className="muted text-xs">{description}</p>
        </div>
        <button type="button" className="secondary-button" onClick={onClose}>
          {closeLabel}
        </button>
      </div>
      {children}
    </section>
  );
}

function OrderReceiptLine({
  line,
  draft,
  onChange,
}: {
  line: PurchaseOrderLine;
  draft: OrderReceiptLineDraft;
  onChange: (draft: OrderReceiptLineDraft) => void;
}) {
  return (
    <fieldset
      className="repeat-card inventory-line purchase-line"
      aria-label={`Renglón ${line.line_no}: ${line.item.name}`}
    >
      <div className="section-heading inventory-line-heading">
        <strong>
          {line.item.name}
          <span className="muted text-xs"> · {line.capture_label}</span>
        </strong>
        {line.fully_received && <span className="tag tag-good">Recibido completo</span>}
      </div>
      <dl className="inventory-facts">
        <div>
          <dt>Solicitado</dt>
          <dd>{formatQuantity(line.quantity)}</dd>
        </div>
        <div>
          <dt>Recibido antes</dt>
          <dd>{formatQuantity(line.received_quantity)}</dd>
        </div>
        <div>
          <dt>Pendiente</dt>
          <dd>
            <strong>{formatQuantity(line.pending_quantity)}</strong>
          </dd>
        </div>
      </dl>
      <div className="form-grid purchase-line-grid">
        <DecimalTextField
          label="Cantidad recibida"
          value={draft.quantity}
          onChange={(quantity) => onChange({ ...draft, quantity })}
          shape={inventoryQuantityShape}
          hint={`En ${line.capture_label}. Vacío: no se recibe en esta recepción.`}
        />
        <DecimalTextField
          label="Precio unitario sin IVA"
          value={draft.unitPrice}
          onChange={(unitPrice) => onChange({ ...draft, unitPrice })}
          shape={purchasePriceShape}
          required={!isZeroText(draft.quantity)}
          hint="El de la orden; cámbialo si la factura trae otro."
        />
        <DecimalTextField
          label="Descuento del renglón"
          value={draft.discount}
          onChange={(discount) => onChange({ ...draft, discount })}
          shape={amountShape}
          hint="Vacío: la parte proporcional del descuento de la orden."
        />
        <TaxRateControl
          label={`Tasa de IVA de ${line.item.name}`}
          value={draft.taxRate}
          onChange={(taxRate) => onChange({ ...draft, taxRate })}
        />
      </div>
    </fieldset>
  );
}

// Recepción de una orden emitida o parcialmente recibida: propone lo pendiente de cada renglón
// (Go lo calcula) y admite el precio, el descuento y la tasa reales de la factura. Recibir más de
// lo pendiente exige confirmarlo explícitamente (allow_excess).
export function OrderReceiptForm({
  order,
  execute,
  busy,
  onRecorded,
  onClose,
}: {
  order: PurchaseOrder;
  execute: InventoryExecute;
  busy: boolean;
  onRecorded: (receipt: Receipt) => void;
  onClose: () => void;
}) {
  const [drafts, setDrafts] = useState(() => orderReceiptDrafts(order));
  const [header, setHeader] = useState<ReceiptHeader>({
    warehouseID: order.expected_warehouse.id,
    reference: '',
    notes: '',
  });
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const { confirm, dialog } = useConfirmDialog();
  const submission = useReceiptSubmission(execute);
  const warehouseList = warehouses.data ?? [];
  const warehouseID = defaultWarehouse(warehouseList, header.warehouseID);
  const received = receivedLines(drafts);
  const locked = busy || submission.sending || submission.uncertain;
  const folio = orderFolio(order.folio);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const effective = { ...header, warehouseID };
    const warehouseName =
      warehouseList.find((warehouse) => warehouse.id === warehouseID)?.name ?? '';
    const confirmed = await confirm({
      title: 'Registrar recepción',
      message: `Se ${pluralWord(received.length, 'registrará', 'registrarán')} ${countLabel(received.length, 'renglón', 'renglones')} de la ${folio.toLowerCase()} en «${warehouseName}». La recepción aumenta existencias y actualiza costos; no registra gastos de caja. Una recepción no se edita ni se borra.`,
      confirmLabel: 'Registrar recepción',
      cancelLabel: 'Revisar',
    });
    if (!confirmed) {
      return;
    }
    const outcome = await submission.send(
      orderReceiptInput(order, drafts, effective, false),
      onRecorded,
    );
    if (outcome.kind !== 'excess') {
      return;
    }
    const accepted = await confirm({
      title: 'Recibir más de lo pendiente',
      message: `${outcome.message} ¿Confirmas que llegó mercancía de más y quieres registrarla como excedente?`,
      confirmLabel: 'Registrar con excedente',
      cancelLabel: 'Corregir cantidades',
      tone: 'danger',
    });
    if (!accepted) {
      return;
    }
    await submission.send(orderReceiptInput(order, drafts, effective, true), onRecorded);
  }

  return (
    <ReceiptPanel
      title={`Recepción de la ${folio.toLowerCase()}`}
      description={`${order.supplier.trade_name} · ${order.state_label} · se propone lo pendiente de cada renglón`}
      closeLabel={`Volver a la ${folio.toLowerCase()}`}
      onClose={onClose}
    >
      {!order.receivable && (
        <Notice>La orden está «{order.state_label}»: no admite recepciones.</Notice>
      )}
      {submission.uncertain && <UncertainReceipt />}
      <form className="form-stack" onSubmit={(event) => void submit(event)}>
        <fieldset className="form-stack" disabled={locked}>
          <ReceiptHeaderFields
            header={{ ...header, warehouseID }}
            warehouses={warehouseList}
            onChange={setHeader}
          />
          {order.lines.map((line) => {
            const draft = drafts.find((entry) => entry.lineNo === line.line_no);
            if (!draft) {
              return null;
            }
            return (
              <OrderReceiptLine
                key={line.line_no}
                line={line}
                draft={draft}
                onChange={(next) =>
                  setDrafts((current) =>
                    current.map((entry) => (entry.lineNo === next.lineNo ? next : entry)),
                  )
                }
              />
            );
          })}
        </fieldset>
        <div className="form-actions">
          <button
            type="submit"
            className="primary-button"
            disabled={locked || received.length === 0 || !order.receivable || warehouseID === ''}
          >
            {submission.sending ? 'Registrando…' : 'Registrar recepción'}
          </button>
          <span className="muted text-xs">
            {countLabel(received.length, 'renglón por recibir', 'renglones por recibir')}
          </span>
        </div>
      </form>
      {dialog}
    </ReceiptPanel>
  );
}

// Modalidad de una recepción directa: con un proveedor registrado o como compra de mostrador.
export type DirectReceiptMode = 'formal' | 'counter';

// Proveedor especial «Compras de mostrador» (etapa 9A), si existe y está activo.
function useCounterSupplier(): SupplierReference | null {
  const counter = useResource<Page<Supplier>>('inventory_suppliers', {
    query: { kind: 'counter', active: 'true' },
    reloadEvent: inventoryChangeEvent,
  });
  const found = counter.data?.items.find((supplier) => supplier.kind === 'counter');
  if (!found) {
    return null;
  }
  return { id: found.id, trade_name: found.trade_name, kind: found.kind, active: found.active };
}

// Recepción directa, sin orden: flujo rápido de una sola pantalla con proveedor obligatorio y
// renglones con insumo, presentación o unidad, cantidad, precio, descuento y tasa de IVA. La
// modalidad «Compras de mostrador» (mercados, centrales de abastos, supermercados) usa el proveedor
// especial sin elegirlo y registra el comercio concreto como referencia opcional; se puede abrir
// preseleccionada desde Recepciones.
export function DirectReceiptForm({
  execute,
  busy,
  timezone,
  onRecorded,
  onClose,
  initialMode = 'formal',
}: {
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onRecorded: (receipt: Receipt) => void;
  onClose: () => void;
  initialMode?: DirectReceiptMode;
}) {
  const counterSupplier = useCounterSupplier();
  const [mode, setMode] = useState<DirectReceiptMode>(initialMode);
  const [formalSupplier, setFormalSupplier] = useState<SupplierReference | null>(null);
  const [counterStore, setCounterStore] = useState('');
  const counter = mode === 'counter';
  const supplier = counter ? counterSupplier : formalSupplier;
  const [lines, setLines] = useState<PurchaseLineDraft[]>([emptyPurchaseLine(1)]);
  const [header, setHeader] = useState<ReceiptHeader>({
    warehouseID: '',
    reference: '',
    notes: '',
  });
  const units = useResource<InventoryUnit[]>('inventory_units', {
    reloadEvent: inventoryChangeEvent,
  });
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const supplierItems = useSupplierItems(supplier?.id ?? '');
  const { confirm, dialog } = useConfirmDialog();
  const submission = useReceiptSubmission(execute);
  const warehouseList = warehouses.data ?? [];
  const warehouseID = defaultWarehouse(warehouseList, header.warehouseID);
  const locked = busy || submission.sending || submission.uncertain;

  async function submit(event: FormEvent) {
    event.preventDefault();
    const warehouseName =
      warehouseList.find((warehouse) => warehouse.id === warehouseID)?.name ?? '';
    const store = counter && counterStore.trim() !== '' ? ` (${counterStore.trim()})` : '';
    const confirmed = await confirm({
      title: counter ? 'Registrar compra de mostrador' : 'Registrar recepción directa',
      message: `Se registrará una recepción directa de «${supplier?.trade_name ?? ''}»${store} con ${countLabel(lines.length, 'renglón', 'renglones')} en «${warehouseName}». Aumenta existencias y actualiza costos; no registra gastos de caja. Una recepción no se edita ni se borra.`,
      confirmLabel: 'Registrar recepción',
      cancelLabel: 'Revisar',
    });
    if (!confirmed) {
      return;
    }
    await submission.send(
      directReceiptInput(supplier, lines, { ...header, warehouseID }, counter ? counterStore : ''),
      onRecorded,
    );
  }

  return (
    <ReceiptPanel
      title="Recepción directa"
      description="Mercancía recibida sin orden de compra. El proveedor es obligatorio; los importes se calculan al guardar."
      closeLabel="Volver a recepciones"
      onClose={onClose}
    >
      {submission.uncertain && <UncertainReceipt />}
      <form className="form-stack" onSubmit={(event) => void submit(event)}>
        <fieldset className="form-stack" disabled={locked}>
          <div className="inventory-subnav">
            <SegmentedControl
              label="Modalidad de la compra"
              value={mode}
              options={[
                { value: 'formal', label: 'Proveedor registrado' },
                {
                  value: 'counter',
                  label: 'Compras de mostrador',
                  disabled: counterSupplier === null,
                },
              ]}
              onChange={setMode}
            />
          </div>
          {counter && counterSupplier === null && (
            <Notice>
              «Compras de mostrador» no está disponible: está desactivado en Proveedores.
            </Notice>
          )}
          {!counter && (
            <SupplierPicker
              label="Proveedor"
              value={formalSupplier}
              required
              kind="formal"
              onChange={setFormalSupplier}
            />
          )}
          {counter && (
            <Field
              label="Comercio (opcional)"
              hint="Mercado, central de abastos, supermercado o tienda donde se compró; una línea."
            >
              <input
                maxLength={100}
                autoComplete="off"
                value={counterStore}
                placeholder="Por ejemplo: Central de abastos, bodega 12"
                onChange={(event) => setCounterStore(event.target.value)}
              />
            </Field>
          )}
          <ReceiptHeaderFields
            header={{ ...header, warehouseID }}
            warehouses={warehouseList}
            onChange={setHeader}
          />
          <PurchaseLinesEditor
            lines={lines}
            units={units.data ?? []}
            supplierItems={supplierItems}
            timezone={timezone}
            allowEmpty={false}
            onChange={setLines}
          />
        </fieldset>
        <div className="form-actions">
          <button
            type="submit"
            className="primary-button"
            disabled={locked || supplier === null || lines.length === 0 || warehouseID === ''}
          >
            {submission.sending ? 'Registrando…' : 'Registrar recepción'}
          </button>
          <span className="muted text-xs">{countLabel(lines.length, 'renglón', 'renglones')}</span>
        </div>
      </form>
      {dialog}
    </ReceiptPanel>
  );
}
