import { useState } from 'react';
import { OptionalDateField } from '../../components/catalog/DateField';
import { Notice } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { Page } from '../../services/inventory';
import type { SupplierReference, SupplierReturnSummary } from '../../services/inventoryPurchasing';
import { formatMoney } from '../../services/money';
import { serverDateTime } from '../../services/restaurant';
import { Pagination } from './InventoryFields';
import { isZeroText, receiptFolio, returnFolio } from './purchasingFormat';
import type { PurchasingNavigation } from './purchasingNavigation';
import { SupplierPicker } from './PurchaseFields';

const pageSize = 25;

interface ReturnFilter {
  supplier: SupplierReference | null;
  from: string;
  until: string;
  page: number;
}

function returnQuery(filter: ReturnFilter) {
  const query: Record<string, string> = {
    page: String(filter.page),
    page_size: String(pageSize),
  };
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

function ReturnsTable({
  page,
  timezone,
  navigation,
}: {
  page: Page<SupplierReturnSummary>;
  timezone: string;
  navigation: PurchasingNavigation;
}) {
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <thead>
          <tr>
            <th scope="col">Devolución</th>
            <th scope="col">Fecha</th>
            <th scope="col">Proveedor</th>
            <th scope="col">Almacén</th>
            <th scope="col">Motivo</th>
            <th scope="col">Responsable</th>
            <th scope="col" className="inventory-number">
              Valor asociado
            </th>
            <th scope="col">
              <span className="sr-only">Acciones</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {page.items.map((entry) => (
            <tr key={entry.id}>
              <td className="inventory-cell-name">
                <strong>{returnFolio(entry.folio)}</strong>
                <span className="muted text-xs inventory-cell-detail">
                  De la {receiptFolio(entry.receipt_folio).toLowerCase()}
                  {entry.receipt_supplier_reference
                    ? ` · Ref. ${entry.receipt_supplier_reference}`
                    : ''}
                </span>
              </td>
              <td>{serverDateTime(entry.occurred_at, timezone)}</td>
              <td className="inventory-cell-name">
                {entry.supplier.trade_name}
                {entry.supplier.kind === 'counter' && (
                  <span className="muted text-xs inventory-cell-detail">Compra de mostrador</span>
                )}
              </td>
              <td>{entry.warehouse.name}</td>
              <td>{entry.reason}</td>
              <td>{entry.actor_name}</td>
              <td className="inventory-number">
                {formatMoney(entry.value)}
                {entry.costs_visible &&
                  entry.unabsorbed_display &&
                  !isZeroText(entry.unabsorbed) && (
                    <span className="muted text-xs inventory-cell-detail">
                      No absorbido: {formatMoney(entry.unabsorbed_display)}
                    </span>
                  )}
              </td>
              <td>
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`Abrir la ${receiptFolio(entry.receipt_folio).toLowerCase()} de la ${returnFolio(entry.folio).toLowerCase()}`}
                  onClick={() => navigation.openReceipt(entry.receipt_id)}
                >
                  Ver recepción
                </button>
              </td>
            </tr>
          ))}
          {page.items.length === 0 && (
            <tr>
              <td colSpan={8} className="muted">
                No hay devoluciones a proveedor con estos filtros.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

// Sección «Devoluciones a proveedor» (etapa 9A): listado de todas las devoluciones registradas, con
// proveedor y fechas de negocio, y acceso a la recepción de origen (donde se registran, con
// inventory.supplier_returns.record). Solo lectura; Go entrega los importes y oculta el valor no
// absorbido a quien no ve costos.
export function SupplierReturnsView({
  timezone,
  navigation,
}: {
  timezone: string;
  navigation: PurchasingNavigation;
}) {
  const [filter, setFilter] = useState<ReturnFilter>({
    supplier: null,
    from: '',
    until: '',
    page: 1,
  });
  const list = useResource<Page<SupplierReturnSummary> & { costs_visible: boolean }>(
    'inventory_supplier_returns',
    { query: returnQuery(filter), reloadEvent: inventoryChangeEvent },
  );
  return (
    <div className="form-stack">
      <div className="section-heading">
        <p className="muted text-sm">
          Mercancía devuelta a proveedores al costo original de su recepción. Una devolución
          disminuye existencias y no registra ingresos de caja; se registra desde el detalle de la
          recepción.
        </p>
      </div>
      <section className="panel catalog-filters" aria-label="Filtros de devoluciones a proveedor">
        <SupplierPicker
          label="Proveedor"
          value={filter.supplier}
          activeOnly={false}
          emptyLabel="Todos los proveedores"
          onChange={(supplier) => setFilter({ ...filter, supplier, page: 1 })}
        />
        <OptionalDateField
          label="Fecha de negocio desde"
          value={filter.from}
          onChange={(from) => setFilter({ ...filter, from, page: 1 })}
        />
        <OptionalDateField
          label="Fecha de negocio hasta"
          value={filter.until}
          onChange={(until) => setFilter({ ...filter, until, page: 1 })}
        />
      </section>
      <Notice>{list.error}</Notice>
      {!list.data && !list.error && <p className="muted">Consultando devoluciones…</p>}
      {list.data && (
        <section className="panel">
          <ReturnsTable page={list.data} timezone={timezone} navigation={navigation} />
          <Pagination
            page={list.data.page}
            pageSize={list.data.page_size}
            total={list.data.total}
            singular="devolución"
            plural="devoluciones"
            onPage={(page) => setFilter({ ...filter, page })}
          />
        </section>
      )}
    </div>
  );
}
