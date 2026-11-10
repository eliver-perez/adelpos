import { useState } from 'react';
import { OptionalDateField } from '../../components/catalog/DateField';
import { Field, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, InventoryWarning, Page, Warehouse } from '../../services/inventory';
import type {
  PurchaseOrder,
  PurchaseOrderSummary,
  Receipt,
  ReceiptSummary,
} from '../../services/inventoryPurchasing';
import { formatMoney } from '../../services/money';
import { serverDateTime } from '../../services/restaurant';
import { Pagination } from './InventoryFields';
import {
  isZeroText,
  orderDateText,
  orderFolio,
  orderStateTone,
  purchasingPermissionName,
  receiptFolio,
} from './purchasingFormat';
import type { PurchasingNavigation } from './purchasingNavigation';
import { ReceiptDetail } from './ReceiptDetail';
import { DirectReceiptForm, OrderReceiptForm } from './ReceiptForms';

const pageSize = 25;

interface ReceiptFilter {
  search: string;
  direct: string;
  warehouse: string;
  from: string;
  until: string;
  page: number;
}

function receiptQuery(filter: ReceiptFilter) {
  const query: Record<string, string> = {
    page: String(filter.page),
    page_size: String(pageSize),
  };
  if (filter.search.trim() !== '') {
    query.search = filter.search.trim();
  }
  if (filter.direct !== '') {
    query.direct = filter.direct;
  }
  if (filter.warehouse !== '') {
    query.warehouse_id = filter.warehouse;
  }
  if (filter.from !== '') {
    query.from = filter.from;
  }
  if (filter.until !== '') {
    query.until = filter.until;
  }
  return query;
}

function ReceiptFilters({
  filter,
  warehouses,
  onChange,
}: {
  filter: ReceiptFilter;
  warehouses: Warehouse[];
  onChange: (filter: ReceiptFilter) => void;
}) {
  return (
    <section className="panel catalog-filters" aria-label="Filtros de recepciones">
      <Field label="Buscar recepción">
        <input
          type="search"
          maxLength={100}
          value={filter.search}
          placeholder="Folio, referencia del proveedor o proveedor"
          onChange={(event) => onChange({ ...filter, search: event.target.value, page: 1 })}
        />
      </Field>
      <Field label="Tipo de recepción">
        <select
          value={filter.direct}
          onChange={(event) => onChange({ ...filter, direct: event.target.value, page: 1 })}
        >
          <option value="">Todas</option>
          <option value="false">De una orden de compra</option>
          <option value="true">Directas</option>
        </select>
      </Field>
      <Field label="Almacén">
        <select
          value={filter.warehouse}
          onChange={(event) => onChange({ ...filter, warehouse: event.target.value, page: 1 })}
        >
          <option value="">Todos</option>
          {warehouses.map((warehouse) => (
            <option key={warehouse.id} value={warehouse.id}>
              {warehouse.name}
            </option>
          ))}
        </select>
      </Field>
      <OptionalDateField
        label="Fecha de negocio desde"
        value={filter.from}
        onChange={(from) => onChange({ ...filter, from, page: 1 })}
      />
      <OptionalDateField
        label="Fecha de negocio hasta"
        value={filter.until}
        onChange={(until) => onChange({ ...filter, until, page: 1 })}
      />
    </section>
  );
}

// Estado del vínculo con gastos de la recepción, con lo que entrega Go.
function linkSummary(receipt: ReceiptSummary) {
  if (receipt.cash_link_attention) {
    return <span className="tag tag-warning">Gasto vinculado corregido</span>;
  }
  if (isZeroText(receipt.linked_amount)) {
    return <span className="muted">Sin vincular</span>;
  }
  return (
    <span>
      {formatMoney(receipt.linked_amount)}
      <span className="muted text-xs inventory-cell-detail">
        Por vincular: {formatMoney(receipt.linkable_amount)}
      </span>
    </span>
  );
}

function ReceiptsTable({
  page,
  costsVisible,
  timezone,
  onOpen,
}: {
  page: Page<ReceiptSummary>;
  costsVisible: boolean;
  timezone: string;
  onOpen: (receipt: ReceiptSummary) => void;
}) {
  const columns = costsVisible ? 10 : 9;
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <thead>
          <tr>
            <th scope="col">Recepción</th>
            <th scope="col">Fecha</th>
            <th scope="col">Proveedor</th>
            <th scope="col">Origen</th>
            <th scope="col">Almacén</th>
            <th scope="col">Recibió</th>
            <th scope="col" className="inventory-number">
              Total
            </th>
            {costsVisible && (
              <th scope="col" className="inventory-number">
                Valor de inventario
              </th>
            )}
            <th scope="col">Gasto de caja</th>
            <th scope="col">
              <span className="sr-only">Acciones</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {page.items.map((receipt) => (
            <tr key={receipt.id}>
              <td className="inventory-cell-name">
                <strong>{receiptFolio(receipt.folio)}</strong>
                {receipt.supplier_reference && (
                  <span className="muted text-xs inventory-cell-detail">
                    Ref. {receipt.supplier_reference}
                  </span>
                )}
              </td>
              <td>{serverDateTime(receipt.occurred_at, timezone)}</td>
              <td>{receipt.supplier.trade_name}</td>
              <td>
                {receipt.direct ? 'Directa' : orderFolio(receipt.purchase_order_folio)}
                {receipt.excess && (
                  <span className="tag tag-warning inventory-inline-tag">Excedente</span>
                )}
              </td>
              <td>{receipt.warehouse.name}</td>
              <td>{receipt.received_by_name}</td>
              <td className="inventory-number">
                {formatMoney(receipt.total)}
                <span className="muted text-xs inventory-cell-detail">
                  {receipt.tax_in_cost ? 'IVA en el costo' : 'IVA fuera del costo'}
                </span>
              </td>
              {costsVisible && (
                <td className="inventory-number">{formatMoney(receipt.inventory_value_display)}</td>
              )}
              <td>{linkSummary(receipt)}</td>
              <td>
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`Abrir la ${receiptFolio(receipt.folio).toLowerCase()} de ${receipt.supplier.trade_name}`}
                  onClick={() => onOpen(receipt)}
                >
                  Abrir
                </button>
              </td>
            </tr>
          ))}
          {page.items.length === 0 && (
            <tr>
              <td colSpan={columns} className="muted">
                No hay recepciones con estos filtros.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

// Órdenes que admiten recepción (emitidas o parcialmente recibidas) para elegir cuál recibir.
function OpenOrderChooser({
  onChoose,
  onClose,
}: {
  onChoose: (order: PurchaseOrderSummary) => void;
  onClose: () => void;
}) {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const query: Record<string, string> = {
    state: 'open',
    page: String(page),
    page_size: String(pageSize),
  };
  if (search.trim() !== '') {
    query.search = search.trim();
  }
  const list = useResource<Page<PurchaseOrderSummary>>('inventory_purchase_orders', {
    query,
    reloadEvent: inventoryChangeEvent,
  });
  return (
    <section className="panel form-stack" aria-label="Elegir la orden por recibir">
      <div className="section-heading">
        <div>
          <h2>Recibir una orden de compra</h2>
          <p className="muted text-xs">Órdenes emitidas o parcialmente recibidas.</p>
        </div>
        <button type="button" className="secondary-button" onClick={onClose}>
          Volver a recepciones
        </button>
      </div>
      <Field label="Buscar orden">
        <input
          type="search"
          maxLength={100}
          value={search}
          placeholder="Folio exacto o proveedor"
          onChange={(event) => {
            setSearch(event.target.value);
            setPage(1);
          }}
        />
      </Field>
      <Notice>{list.error}</Notice>
      {!list.data && !list.error && <p className="muted">Consultando órdenes…</p>}
      {list.data && (
        <>
          <div className="table-container cash-table">
            <table className="inventory-table">
              <thead>
                <tr>
                  <th scope="col">Orden</th>
                  <th scope="col">Fecha</th>
                  <th scope="col">Proveedor</th>
                  <th scope="col">Estado</th>
                  <th scope="col" className="inventory-number">
                    Total
                  </th>
                  <th scope="col">
                    <span className="sr-only">Acciones</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {list.data.items.map((order) => (
                  <tr key={order.id}>
                    <td>{orderFolio(order.folio)}</td>
                    <td>{orderDateText(order.order_date)}</td>
                    <td>{order.supplier.trade_name}</td>
                    <td>
                      <span className={orderStateTone(order.state)}>{order.state_label}</span>
                    </td>
                    <td className="inventory-number">{formatMoney(order.total)}</td>
                    <td>
                      <button
                        type="button"
                        className="primary-button"
                        aria-label={`Recibir la ${orderFolio(order.folio).toLowerCase()} de ${order.supplier.trade_name}`}
                        onClick={() => onChoose(order)}
                      >
                        Recibir
                      </button>
                    </td>
                  </tr>
                ))}
                {list.data.items.length === 0 && (
                  <tr>
                    <td colSpan={6} className="muted">
                      No hay órdenes emitidas pendientes de recibir.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <Pagination
            page={list.data.page}
            pageSize={list.data.page_size}
            total={list.data.total}
            singular="orden"
            plural="órdenes"
            onPage={setPage}
          />
        </>
      )}
    </section>
  );
}

function OrderReceiptLoader({
  orderID,
  execute,
  busy,
  onRecorded,
  onClose,
}: {
  orderID: string;
  execute: InventoryExecute;
  busy: boolean;
  onRecorded: (receipt: Receipt) => void;
  onClose: () => void;
}) {
  const order = useResource<PurchaseOrder>('inventory_purchase_order', {
    id: orderID,
    reloadEvent: inventoryChangeEvent,
  });
  if (!order.data || order.data.id !== orderID) {
    return <Notice>{order.error || 'Consultando la orden…'}</Notice>;
  }
  return (
    <OrderReceiptForm
      order={order.data}
      execute={execute}
      busy={busy}
      onRecorded={onRecorded}
      onClose={onClose}
    />
  );
}

type ReceiptsMode =
  | { kind: 'list' }
  | { kind: 'receipt'; receiptID: string; warnings: InventoryWarning[] }
  | { kind: 'direct' }
  | { kind: 'choose' }
  | { kind: 'receive'; orderID: string };

// Pestaña «Recepciones»: recepciones de mercancía (de una orden o directas) con búsqueda, tipo,
// almacén y fechas; detalle con lo solicitado, recibido y pendiente, costo recibido (solo con
// inventory.costs.read), devoluciones a proveedor y vínculo con gastos de caja. Recibir exige
// inventory.purchases.receive y la recepción directa además inventory.purchases.create.
export function ReceiptsView({
  can,
  execute,
  busy,
  timezone,
  initialReceiptID,
  navigation,
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  initialReceiptID: string | null;
  navigation: PurchasingNavigation;
}) {
  const [mode, setMode] = useState<ReceiptsMode>(
    initialReceiptID
      ? { kind: 'receipt', receiptID: initialReceiptID, warnings: [] }
      : { kind: 'list' },
  );
  const [filter, setFilter] = useState<ReceiptFilter>({
    search: '',
    direct: '',
    warehouse: '',
    from: '',
    until: '',
    page: 1,
  });
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const list = useResource<Page<ReceiptSummary> & { costs_visible: boolean }>(
    'inventory_receipts',
    { query: receiptQuery(filter), reloadEvent: inventoryChangeEvent },
  );
  const receive = can('inventory.purchases.receive');
  const direct = receive && can('inventory.purchases.create');
  const recorded = (receipt: Receipt) =>
    setMode({ kind: 'receipt', receiptID: receipt.id, warnings: receipt.warnings });
  const back = () => setMode({ kind: 'list' });

  if (mode.kind === 'receipt') {
    return (
      <ReceiptDetail
        receiptID={mode.receiptID}
        warnings={mode.warnings}
        can={can}
        execute={execute}
        busy={busy}
        timezone={timezone}
        closeLabel="Volver a recepciones"
        onClose={back}
        navigation={navigation}
      />
    );
  }
  if (mode.kind === 'direct') {
    return (
      <DirectReceiptForm
        execute={execute}
        busy={busy}
        timezone={timezone}
        onRecorded={recorded}
        onClose={back}
      />
    );
  }
  if (mode.kind === 'choose') {
    return (
      <OpenOrderChooser
        onChoose={(order) => setMode({ kind: 'receive', orderID: order.id })}
        onClose={back}
      />
    );
  }
  if (mode.kind === 'receive') {
    return (
      <OrderReceiptLoader
        orderID={mode.orderID}
        execute={execute}
        busy={busy}
        onRecorded={recorded}
        onClose={() => setMode({ kind: 'choose' })}
      />
    );
  }

  const data = list.data;
  return (
    <div className="form-stack">
      <div className="section-heading">
        <p className="muted text-sm">
          Entradas de mercancía con su proveedor, almacén y responsable. Una recepción aumenta
          existencias y actualiza costos; no registra pagos ni gastos de caja.
        </p>
        <div className="button-row">
          {receive && (
            <button
              type="button"
              className="secondary-button"
              onClick={() => setMode({ kind: 'choose' })}
            >
              <IconLabel icon="receipt">Recibir una orden</IconLabel>
            </button>
          )}
          {direct && (
            <button
              type="button"
              className="primary-button"
              onClick={() => setMode({ kind: 'direct' })}
            >
              <IconLabel icon="plus">Recepción directa</IconLabel>
            </button>
          )}
        </div>
      </div>
      {!receive && (
        <Notice>
          Consulta disponible. Recibir mercancía requiere el permiso «
          {purchasingPermissionName('inventory.purchases.receive')}».
        </Notice>
      )}
      {receive && !direct && (
        <p className="muted text-xs">
          La recepción directa (sin orden) requiere además el permiso «
          {purchasingPermissionName('inventory.purchases.create')}».
        </p>
      )}
      <ReceiptFilters filter={filter} warehouses={warehouses.data ?? []} onChange={setFilter} />
      <Notice>{list.error}</Notice>
      {!data && !list.error && <p className="muted">Consultando recepciones…</p>}
      {data && (
        <section className="panel">
          <ReceiptsTable
            page={data}
            costsVisible={data.costs_visible}
            timezone={timezone}
            onOpen={(receipt) => setMode({ kind: 'receipt', receiptID: receipt.id, warnings: [] })}
          />
          <Pagination
            page={data.page}
            pageSize={data.page_size}
            total={data.total}
            singular="recepción"
            plural="recepciones"
            onPage={(page) => setFilter({ ...filter, page })}
          />
        </section>
      )}
    </div>
  );
}
