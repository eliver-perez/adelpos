import { useEffect, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { OptionalDateField } from '../../components/catalog/DateField';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { SegmentedControl } from '../../components/SegmentedControl';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type {
  InventoryExecute,
  InventoryUnit,
  InventoryWarning,
  Page,
  Warehouse,
} from '../../services/inventory';
import { createPurchaseOrder, savePurchaseOrder } from '../../services/inventoryPurchasing';
import type {
  PurchaseOrder,
  PurchaseOrderInput,
  PurchaseOrderStateFilter,
  PurchaseOrderSummary,
  Receipt,
  SupplierReference,
} from '../../services/inventoryPurchasing';
import { formatMoney } from '../../services/money';
import { notifyError, notifySuccess } from '../../services/notify';
import { countLabel } from '../../services/plural';
import { Pagination } from './InventoryFields';
import {
  DocumentTotals,
  linesChanged,
  PurchaseLinesEditor,
  SupplierPicker,
  useSupplierItems,
} from './PurchaseFields';
import {
  OrderActions,
  OrderFacts,
  OrderLinesTable,
  OrderReceiptsTable,
} from './PurchaseOrderDetail';
import {
  emptyPurchaseLine,
  orderDateText,
  orderFolio,
  orderStateOptions,
  orderStateTone,
  purchaseLineDraft,
  purchaseLineInput,
  purchasingPermissionName,
} from './purchasingFormat';
import type { PurchaseLineDraft } from './purchasingFormat';
import type { PurchasingNavigation } from './purchasingNavigation';
import { PurchaseSuggestionsView } from './PurchaseSuggestionsView';
import { ReceiptDetail } from './ReceiptDetail';
import { OrderReceiptForm } from './ReceiptForms';

const pageSize = 25;

export type PurchasesSubsection = 'orders' | 'suggestions';

interface OrderFilter {
  search: string;
  state: PurchaseOrderStateFilter | '';
  supplier: SupplierReference | null;
  from: string;
  until: string;
  page: number;
}

function orderQuery(filter: OrderFilter) {
  const query: Record<string, string> = {
    page: String(filter.page),
    page_size: String(pageSize),
  };
  if (filter.search.trim() !== '') {
    query.search = filter.search.trim();
  }
  if (filter.state !== '') {
    query.state = filter.state;
  }
  if (filter.supplier) {
    query.supplier_id = filter.supplier.id;
  }
  if (filter.from !== '') {
    query.from = filter.from;
  }
  if (filter.until !== '') {
    query.until = filter.until;
  }
  return query;
}

interface OrderFormState {
  supplier: SupplierReference | null;
  orderDate: string;
  warehouseID: string;
  notes: string;
  lines: PurchaseLineDraft[];
}

function initialOrderForm(order: PurchaseOrder | null): OrderFormState {
  return {
    supplier: order ? order.supplier : null,
    orderDate: order?.order_date ?? '',
    warehouseID: order?.expected_warehouse.id ?? '',
    notes: order?.notes ?? '',
    lines: order
      ? order.lines.map((line, index) => purchaseLineDraft(line, index + 1))
      : [emptyPurchaseLine(1)],
  };
}

// Cuerpo de la orden para Go: datos y renglones tal como se capturaron (PATCH reemplaza todos los
// renglones). La fecha vacía es hoy en la zona del establecimiento; el almacén vacío, el general.
export function purchaseOrderInput(
  form: OrderFormState,
  order: PurchaseOrder | null,
): PurchaseOrderInput {
  const input: PurchaseOrderInput = {
    supplier_id: form.supplier?.id ?? '',
    expected_warehouse_id: form.warehouseID === '' ? null : form.warehouseID,
    notes: form.notes,
    lines: form.lines.map(purchaseLineInput),
  };
  if (form.orderDate !== '') {
    input.order_date = form.orderDate;
  }
  if (order) {
    input.revision = order.revision;
  }
  return input;
}

function formChanged(form: OrderFormState, initial: OrderFormState) {
  return (
    form.supplier?.id !== initial.supplier?.id ||
    form.orderDate !== initial.orderDate ||
    form.warehouseID !== initial.warehouseID ||
    form.notes !== initial.notes ||
    linesChanged(form.lines, initial.lines)
  );
}

function OrderFilters({
  filter,
  onChange,
}: {
  filter: OrderFilter;
  onChange: (filter: OrderFilter) => void;
}) {
  return (
    <section className="panel catalog-filters" aria-label="Filtros de órdenes de compra">
      <Field label="Buscar orden">
        <input
          type="search"
          maxLength={100}
          value={filter.search}
          placeholder="Folio exacto o proveedor"
          onChange={(event) => onChange({ ...filter, search: event.target.value, page: 1 })}
        />
      </Field>
      <Field label="Estado de la orden">
        <select
          value={filter.state}
          onChange={(event) =>
            onChange({
              ...filter,
              state: event.target.value as PurchaseOrderStateFilter | '',
              page: 1,
            })
          }
        >
          <option value="">Todos</option>
          {orderStateOptions.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </Field>
      <SupplierPicker
        label="Proveedor"
        value={filter.supplier}
        activeOnly={false}
        emptyLabel="Todos los proveedores"
        onChange={(supplier) => onChange({ ...filter, supplier, page: 1 })}
      />
      <OptionalDateField
        label="Fecha de la orden desde"
        value={filter.from}
        onChange={(from) => onChange({ ...filter, from, page: 1 })}
      />
      <OptionalDateField
        label="Fecha de la orden hasta"
        value={filter.until}
        onChange={(until) => onChange({ ...filter, until, page: 1 })}
      />
    </section>
  );
}

function OrdersTable({
  page,
  onOpen,
}: {
  page: Page<PurchaseOrderSummary>;
  onOpen: (order: PurchaseOrderSummary) => void;
}) {
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <thead>
          <tr>
            <th scope="col">Orden</th>
            <th scope="col">Fecha</th>
            <th scope="col">Proveedor</th>
            <th scope="col">Almacén previsto</th>
            <th scope="col">Renglones</th>
            <th scope="col">Recepciones</th>
            <th scope="col" className="inventory-number">
              Total
            </th>
            <th scope="col">Estado</th>
            <th scope="col">
              <span className="sr-only">Acciones</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {page.items.map((order) => (
            <tr key={order.id}>
              <td className="inventory-cell-name">
                <strong>{orderFolio(order.folio)}</strong>
                <span className="muted text-xs inventory-cell-detail">{order.created_by_name}</span>
              </td>
              <td>{orderDateText(order.order_date)}</td>
              <td>{order.supplier.trade_name}</td>
              <td>{order.expected_warehouse.name}</td>
              <td>{countLabel(order.line_count, 'renglón', 'renglones')}</td>
              <td>{countLabel(order.receipt_count, 'recepción', 'recepciones')}</td>
              <td className="inventory-number">{formatMoney(order.total)}</td>
              <td>
                <span className={orderStateTone(order.state)}>{order.state_label}</span>
              </td>
              <td>
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`Abrir la ${orderFolio(order.folio).toLowerCase()} de ${order.supplier.trade_name}`}
                  onClick={() => onOpen(order)}
                >
                  Abrir
                </button>
              </td>
            </tr>
          ))}
          {page.items.length === 0 && (
            <tr>
              <td colSpan={9} className="muted">
                No hay órdenes de compra con estos filtros.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

// Editor del borrador (inventory.purchases.create): proveedor, fecha, almacén previsto,
// observaciones y renglones. Los importes y totales los calcula Go al guardar; la pantalla solo
// muestra los del borrador guardado.
function PurchaseOrderEditor({
  order,
  can,
  execute,
  busy,
  timezone,
  onSaved,
  onDirtyChange,
}: {
  order: PurchaseOrder | null;
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onSaved: (order: PurchaseOrder) => void;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const [initial] = useState(() => initialOrderForm(order));
  const [form, setForm] = useState(initial);
  const [saving, setSaving] = useState(false);
  const units = useResource<InventoryUnit[]>('inventory_units', {
    reloadEvent: inventoryChangeEvent,
  });
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const supplierItems = useSupplierItems(form.supplier?.id ?? '');
  const dirty = formChanged(form, initial);
  const offered = (warehouses.data ?? []).filter(
    (warehouse) => warehouse.active || warehouse.id === form.warehouseID,
  );
  const fallback = offered.find((warehouse) => warehouse.is_default) ?? offered[0];
  const selectedWarehouse = form.warehouseID === '' ? (fallback?.id ?? '') : form.warehouseID;
  const locked = busy || saving;

  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty, onDirtyChange]);

  function update<K extends keyof OrderFormState>(key: K, value: OrderFormState[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    try {
      const input = purchaseOrderInput({ ...form, warehouseID: selectedWarehouse }, order);
      const saved = order
        ? await savePurchaseOrder(execute, order.id, input)
        : await createPurchaseOrder(execute, input);
      notifySuccess(
        order
          ? `Borrador de la ${orderFolio(saved.folio).toLowerCase()} guardado.`
          : `${orderFolio(saved.folio)} creada como borrador.`,
        { description: `Total calculado: ${formatMoney(saved.total)}.` },
      );
      onDirtyChange(false);
      onSaved(saved);
    } catch (problem) {
      notifyError(problem, 'No se pudo guardar el borrador.');
    } finally {
      setSaving(false);
    }
  }

  let totalsNote = 'Los importes los calcula el servidor al guardar el borrador.';
  if (order && dirty) {
    totalsNote =
      'Totales del borrador guardado. Guarda el borrador para que el servidor calcule los cambios.';
  } else if (order) {
    totalsNote = 'Calculados por el servidor: suma exacta de los renglones.';
  }

  return (
    <form className="form-stack" onSubmit={(event) => void submit(event)}>
      <fieldset className="form-stack" disabled={locked}>
        <SupplierPicker
          label="Proveedor"
          value={form.supplier}
          required
          onChange={(supplier) => update('supplier', supplier)}
        />
        <div className="form-grid">
          <OptionalDateField
            label="Fecha de la orden"
            value={form.orderDate}
            onChange={(value) => update('orderDate', value)}
          />
          <Field
            label="Almacén de destino previsto"
            hint="Donde se propondrá recibir la mercancía."
          >
            <select
              required
              value={selectedWarehouse}
              onChange={(event) => update('warehouseID', event.target.value)}
            >
              {offered.map((warehouse) => (
                <option key={warehouse.id} value={warehouse.id}>
                  {warehouse.name}
                  {warehouse.active ? '' : ' (inactivo)'}
                </option>
              ))}
            </select>
          </Field>
        </div>
        <p className="muted text-xs">
          Fecha vacía: hoy. No puede ser futura. Crear o emitir la orden no modifica existencias ni
          registra gastos de caja.
        </p>
        <PurchaseLinesEditor
          lines={form.lines}
          units={units.data ?? []}
          supplierItems={supplierItems}
          timezone={timezone}
          allowEmpty
          onChange={(lines) => update('lines', lines)}
        />
        <Field label="Observaciones de la orden" hint="Opcional; hasta 2000 caracteres.">
          <textarea
            rows={2}
            maxLength={2000}
            value={form.notes}
            onChange={(event) => update('notes', event.target.value)}
          />
        </Field>
      </fieldset>
      {order && (
        <DocumentTotals
          label="Totales de la orden"
          subtotal={order.subtotal}
          discount={order.discount}
          tax={order.tax}
          total={order.total}
          note={totalsNote}
        />
      )}
      {!order && <p className="muted text-sm">{totalsNote}</p>}
      <div className="form-actions">
        <button
          type="submit"
          className="primary-button"
          disabled={locked || !can('inventory.purchases.create') || (order !== null && !dirty)}
        >
          {saving ? 'Guardando…' : order ? 'Guardar borrador' : 'Crear borrador'}
        </button>
        {dirty && order && <span className="muted text-xs">Cambios sin guardar</span>}
      </div>
    </form>
  );
}

type PanelMode =
  | { kind: 'order' }
  | { kind: 'receive' }
  | { kind: 'receipt'; receiptID: string; warnings: InventoryWarning[] };

// Panel de una orden: editor del borrador (con permiso de crear) o detalle con lo solicitado,
// recibido y pendiente, sus recepciones y las acciones que admite su estado. Desde aquí se
// registra cada recepción de la orden (12 + 8 de 20 cajas, por ejemplo) sin cambiar de pestaña.
function PurchaseOrderPanel({
  initialOrderID,
  can,
  execute,
  busy,
  timezone,
  navigation,
  onClose,
}: {
  initialOrderID: string;
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  navigation: PurchasingNavigation;
  onClose: () => void;
}) {
  const [orderID, setOrderID] = useState(initialOrderID);
  const [snapshot, setSnapshot] = useState<PurchaseOrder | null>(null);
  const [mode, setMode] = useState<PanelMode>({ kind: 'order' });
  const [dirty, setDirty] = useState(false);
  const dirtyRef = useRef(false);
  const { confirm, dialog } = useConfirmDialog();
  const resource = useResource<PurchaseOrder>('inventory_purchase_order', {
    id: orderID,
    enabled: orderID !== '',
    reloadEvent: inventoryChangeEvent,
  });

  useEffect(() => {
    if (resource.data) {
      setSnapshot(resource.data);
    }
  }, [resource.data]);

  const order = orderID === '' ? null : snapshot?.id === orderID ? snapshot : null;
  const editing = orderID === '' || (order !== null && order.editable);
  const creatable = can('inventory.purchases.create');

  function markDirty(value: boolean) {
    dirtyRef.current = value;
    setDirty(value);
  }

  async function close() {
    if (dirtyRef.current) {
      const discard = await confirm({
        title: 'Cambios sin guardar',
        message: 'Este borrador tiene cambios que aún no se guardan. ¿Quieres descartarlos?',
        confirmLabel: 'Descartar cambios',
        cancelLabel: 'Seguir editando',
        tone: 'danger',
      });
      if (!discard) {
        return;
      }
    }
    onClose();
  }

  if (order && mode.kind === 'receive') {
    return (
      <OrderReceiptForm
        order={order}
        execute={execute}
        busy={busy}
        onRecorded={(receipt: Receipt) =>
          setMode({ kind: 'receipt', receiptID: receipt.id, warnings: receipt.warnings })
        }
        onClose={() => setMode({ kind: 'order' })}
      />
    );
  }
  if (mode.kind === 'receipt') {
    return (
      <ReceiptDetail
        receiptID={mode.receiptID}
        warnings={mode.warnings}
        can={can}
        execute={execute}
        busy={busy}
        timezone={timezone}
        closeLabel={order ? `Volver a la ${orderFolio(order.folio).toLowerCase()}` : 'Volver'}
        onClose={() => setMode({ kind: 'order' })}
        navigation={navigation}
      />
    );
  }

  let heading = 'Orden de compra';
  if (orderID === '') {
    heading = 'Nueva orden de compra';
  } else if (order) {
    heading = `${orderFolio(order.folio)} · ${order.supplier.trade_name}`;
  }

  return (
    <section className="panel form-stack" aria-label={heading}>
      <div className="section-heading">
        <div>
          <h2>{heading}</h2>
          <p className="muted text-xs">
            {order ? (
              <>
                <span className={orderStateTone(order.state)}>{order.state_label}</span>
                {` · ${formatMoney(order.total)} · ${countLabel(order.line_count, 'renglón', 'renglones')}`}
              </>
            ) : (
              'Borrador: se guardará al confirmar'
            )}
          </p>
        </div>
        <button type="button" className="secondary-button" onClick={() => void close()}>
          Volver al listado
        </button>
      </div>
      {orderID !== '' && !order && <Notice>{resource.error || 'Consultando la orden…'}</Notice>}
      {order && <OrderFacts order={order} timezone={timezone} />}
      {order && (
        <OrderActions
          order={order}
          can={can}
          execute={execute}
          busy={busy}
          dirty={dirty}
          onChanged={(saved) => {
            markDirty(false);
            setSnapshot(saved);
          }}
          onReceive={() => setMode({ kind: 'receive' })}
        />
      )}
      {editing && creatable && (orderID === '' || order) && (
        <PurchaseOrderEditor
          key={order ? `${order.id}:${order.revision}` : 'new'}
          order={order}
          can={can}
          execute={execute}
          busy={busy}
          timezone={timezone}
          onDirtyChange={markDirty}
          onSaved={(saved) => {
            setSnapshot(saved);
            setOrderID(saved.id);
          }}
        />
      )}
      {editing && !creatable && orderID !== '' && (
        <Notice>
          Borrador en consulta. Editarlo requiere el permiso «
          {purchasingPermissionName('inventory.purchases.create')}».
        </Notice>
      )}
      {order && (!editing || !creatable) && (
        <>
          <OrderLinesTable order={order} />
          <DocumentTotals
            label="Totales de la orden"
            subtotal={order.subtotal}
            discount={order.discount}
            tax={order.tax}
            total={order.total}
          />
          {order.notes && (
            <p className="text-sm">
              <strong>Observaciones: </strong>
              {order.notes}
            </p>
          )}
        </>
      )}
      {order && order.state !== 'draft' && (
        <section className="inventory-subsection form-stack" aria-label="Recepciones de la orden">
          <h3>Recepciones</h3>
          <OrderReceiptsTable
            order={order}
            timezone={timezone}
            onOpen={(id) => setMode({ kind: 'receipt', receiptID: id, warnings: [] })}
          />
        </section>
      )}
      {dialog}
    </section>
  );
}

// Pestaña «Compras»: órdenes de compra con filtros por estado, proveedor y fechas y paginación
// (y, en su propio apartado, las sugerencias de compra de la fase D);
// alta y edición del borrador (inventory.purchases.create); emisión, cancelación y cierre
// administrativo (inventory.purchases.authorize); y recepción de lo pendiente
// (inventory.purchases.receive). Go valida permisos, estados y montos en cada escritura.
export function PurchaseOrdersView({
  can,
  execute,
  busy,
  timezone,
  initialOrderID,
  initialSubsection = 'orders',
  navigation,
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  initialOrderID: string | null;
  // Apartado con el que se abre la pestaña (el resumen lleva a las sugerencias de compra).
  initialSubsection?: PurchasesSubsection;
  navigation: PurchasingNavigation;
}) {
  const [subsection, setSubsection] = useState<PurchasesSubsection>(initialSubsection);
  const [filter, setFilter] = useState<OrderFilter>({
    search: '',
    state: '',
    supplier: null,
    from: '',
    until: '',
    page: 1,
  });
  // null: listado; '': orden nueva; id: orden abierta.
  const [open, setOpen] = useState<string | null>(initialOrderID);
  const list = useResource<Page<PurchaseOrderSummary>>('inventory_purchase_orders', {
    query: orderQuery(filter),
    enabled: subsection === 'orders',
    reloadEvent: inventoryChangeEvent,
  });
  const creatable = can('inventory.purchases.create');

  if (open !== null) {
    return (
      <PurchaseOrderPanel
        key={open}
        initialOrderID={open}
        can={can}
        execute={execute}
        busy={busy}
        timezone={timezone}
        navigation={navigation}
        onClose={() => setOpen(null)}
      />
    );
  }

  const subnav = (
    <div className="inventory-subnav">
      <SegmentedControl
        label="Apartados de compras"
        value={subsection}
        options={[
          { value: 'orders', label: 'Órdenes de compra' },
          { value: 'suggestions', label: 'Sugerencias de compra' },
        ]}
        onChange={setSubsection}
      />
    </div>
  );

  if (subsection === 'suggestions') {
    return (
      <div className="form-stack">
        {subnav}
        <PurchaseSuggestionsView
          can={can}
          execute={execute}
          busy={busy}
          timezone={timezone}
          onOpenOrder={(orderID) => {
            setSubsection('orders');
            setOpen(orderID);
          }}
        />
      </div>
    );
  }

  return (
    <div className="form-stack">
      {subnav}
      <div className="section-heading">
        <p className="muted text-sm">
          Órdenes de compra a proveedores: borrador, emisión y recepción total o parcial. Una orden
          no modifica existencias ni registra gastos de caja; las existencias aumentan al recibir.
        </p>
        {creatable && (
          <button type="button" className="primary-button" onClick={() => setOpen('')}>
            <IconLabel icon="plus">Nueva orden de compra</IconLabel>
          </button>
        )}
      </div>
      {!creatable && (
        <Notice>
          Consulta disponible. Crear órdenes requiere el permiso «
          {purchasingPermissionName('inventory.purchases.create')}».
        </Notice>
      )}
      <OrderFilters filter={filter} onChange={setFilter} />
      <Notice>{list.error}</Notice>
      {!list.data && !list.error && <p className="muted">Consultando órdenes de compra…</p>}
      {list.data && (
        <section className="panel">
          <OrdersTable page={list.data} onOpen={(order) => setOpen(order.id)} />
          <Pagination
            page={list.data.page}
            pageSize={list.data.page_size}
            total={list.data.total}
            singular="orden"
            plural="órdenes"
            onPage={(page) => setFilter({ ...filter, page })}
          />
        </section>
      )}
    </div>
  );
}
