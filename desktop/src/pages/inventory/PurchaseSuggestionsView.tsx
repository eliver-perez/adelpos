import { useState } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryCategory, InventoryExecute } from '../../services/inventory';
import { draftOrderFromSuggestions } from '../../services/inventoryControl';
import type { PurchaseSuggestion, PurchaseSuggestionPage } from '../../services/inventoryControl';
import type { PurchaseOrder, SupplierReference } from '../../services/inventoryPurchasing';
import { formatMoney } from '../../services/money';
import { notifyError, notifySuccess } from '../../services/notify';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { Pagination } from './InventoryFields';
import { formatQuantity } from './inventoryFormat';
import { SupplierPicker } from './PurchaseFields';
import { isZeroText, orderFolio, purchasingPermissionName } from './purchasingFormat';
import { Select } from '../../components/Select';

const pageSize = 100;

interface SuggestionGroup {
  key: string;
  supplier: SupplierReference | null;
  suggestions: PurchaseSuggestion[];
}

// Agrupa por proveedor preferido conservando el orden de Go (los que no tienen, al final).
function groupSuggestions(items: PurchaseSuggestion[]): SuggestionGroup[] {
  const groups: SuggestionGroup[] = [];
  for (const suggestion of items) {
    const key = suggestion.supplier?.id ?? 'none';
    const group = groups.find((entry) => entry.key === key);
    if (group) {
      group.suggestions.push(suggestion);
    } else {
      groups.push({ key, supplier: suggestion.supplier, suggestions: [suggestion] });
    }
  }
  return groups;
}

// Cantidad sugerida en la presentación de compra («2 · Caja con 24») o en la unidad base.
function suggestedText(suggestion: PurchaseSuggestion) {
  // Sin permiso de existencias Go puede omitir la cantidad sugerida.
  if (!suggestion.suggested_quantity) {
    return '—';
  }
  if (suggestion.presentation) {
    return `${formatQuantity(suggestion.suggested_quantity)} · ${suggestion.capture_label}`;
  }
  return formatQuantity(suggestion.suggested_quantity, suggestion.unit ?? suggestion.item.unit);
}

function SuggestionRow({
  suggestion,
  page,
  selected,
  selectable,
  timezone,
  onToggle,
}: {
  suggestion: PurchaseSuggestion;
  page: PurchaseSuggestionPage;
  selected: boolean;
  selectable: boolean;
  timezone: string;
  onToggle: () => void;
}) {
  const unit = suggestion.item.unit;
  return (
    <tr>
      {selectable && (
        <td>
          <button
            type="button"
            className={`toggle-button control-select-button ${selected ? 'toggle-button-on' : ''}`}
            aria-pressed={selected}
            aria-label={`Incluir ${suggestion.item.name} en el borrador`}
            onClick={onToggle}
          >
            <span>{selected ? 'Incluido' : 'Incluir'}</span>
          </button>
        </td>
      )}
      <td className="inventory-cell-name">
        <strong>{suggestion.item.name}</strong>
        <span className="muted text-xs inventory-cell-detail">
          {suggestion.item.code}
          {suggestion.category_name ? ` · ${suggestion.category_name}` : ''}
        </span>
        {suggestion.below_minimum && <span className="tag tag-warning">Bajo mínimo</span>}
      </td>
      {page.stock_visible && (
        <td className="inventory-number">{formatQuantity(suggestion.stock, unit)}</td>
      )}
      {page.stock_visible && (
        <td className="inventory-number">
          {formatQuantity(suggestion.pending_quantity, unit)}
          {!isZeroText(suggestion.draft_quantity) && (
            <span className="muted text-xs inventory-cell-detail">
              {formatQuantity(suggestion.draft_quantity, unit)} en borradores
            </span>
          )}
        </td>
      )}
      <td className="inventory-number">
        {formatQuantity(suggestion.reorder_point, unit)}
        <span className="muted text-xs inventory-cell-detail">
          {suggestion.reorder_source === 'item' ? 'Del insumo' : 'Suma de sus almacenes'}
        </span>
      </td>
      {page.stock_visible && (
        <td className="inventory-number">{formatQuantity(suggestion.need_quantity, unit)}</td>
      )}
      <td className="inventory-number">
        <strong>{suggestedText(suggestion)}</strong>
        <span className="muted text-xs inventory-cell-detail">
          {formatQuantity(suggestion.suggested_base_quantity, unit)}
        </span>
      </td>
      {page.purchasing_visible && (
        <td>
          {suggestion.last_unit_price !== null ? (
            <>
              {formatMoney(suggestion.last_unit_price)}
              {suggestion.last_price_label ? ` / ${suggestion.last_price_label}` : ''}
              {suggestion.last_price_at && (
                <span className="muted text-xs inventory-cell-detail">
                  {serverDateTime(suggestion.last_price_at, timezone)}
                </span>
              )}
            </>
          ) : (
            '—'
          )}
        </td>
      )}
    </tr>
  );
}

// Un proveedor con sus sugerencias. «Crear orden en borrador» (inventory.purchases.create) es la
// única acción y se confirma: Go crea un borrador con las cantidades sugeridas, nunca lo emite, y
// responde SUGGESTION_CHANGED sin crear nada si alguna sugerencia ya no aplica.
function SuggestionGroupPanel({
  group,
  page,
  creatable,
  execute,
  busy,
  timezone,
  onCreated,
  onChanged,
}: {
  group: SuggestionGroup;
  page: PurchaseSuggestionPage;
  creatable: boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onCreated: (order: PurchaseOrder) => void;
  onChanged: () => void;
}) {
  const [excluded, setExcluded] = useState<string[]>([]);
  const [supplier, setSupplier] = useState<SupplierReference | null>(group.supplier);
  const [sending, setSending] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const selected = group.suggestions.filter((suggestion) => !excluded.includes(suggestion.item.id));
  const title = group.supplier ? group.supplier.trade_name : 'Sin proveedor preferido';
  const selectable = creatable && page.purchasing_visible;

  async function create() {
    if (!supplier || selected.length === 0) {
      return;
    }
    const confirmed = await confirm({
      title: 'Crear orden en borrador',
      message: (
        <>
          <p>
            Se creará un borrador para «{supplier.trade_name}» con{' '}
            {countLabel(selected.length, 'insumo', 'insumos')} y las cantidades sugeridas:
          </p>
          <ul>
            {selected.map((suggestion) => (
              <li key={suggestion.item.id}>
                {suggestion.item.name}: {suggestedText(suggestion)}
              </li>
            ))}
          </ul>
          <p>
            No se emite, no modifica existencias ni registra gastos de caja. Revisa precios e IVA
            antes de emitirla.
          </p>
        </>
      ),
      confirmLabel: 'Crear borrador',
      cancelLabel: 'Revisar',
    });
    if (!confirmed) {
      return;
    }
    setSending(true);
    try {
      const order = await draftOrderFromSuggestions(execute, {
        supplier_id: supplier.id,
        item_ids: selected.map((suggestion) => suggestion.item.id),
      });
      notifySuccess(
        `${orderFolio(order.folio)} creada como borrador para «${order.supplier.trade_name}».`,
        { description: 'Revísala y emítela desde Compras.' },
      );
      onCreated(order);
    } catch (problem) {
      notifyError(problem, 'No se pudo crear el borrador.');
      onChanged();
    } finally {
      setSending(false);
    }
  }

  return (
    <section className="inventory-subsection form-stack" aria-label={`Sugerencias: ${title}`}>
      <div className="section-heading">
        <div>
          <h3>{title}</h3>
          <p className="muted text-xs">
            {countLabel(group.suggestions.length, 'insumo', 'insumos')}
            {group.supplier ? ' con este proveedor preferido' : ''}
          </p>
        </div>
      </div>
      <div className="table-container cash-table">
        <table className="inventory-table">
          <thead>
            <tr>
              {selectable && (
                <th scope="col">
                  <span className="sr-only">Incluir</span>
                </th>
              )}
              <th scope="col">Insumo</th>
              {page.stock_visible && (
                <th scope="col" className="inventory-number">
                  Existencia
                </th>
              )}
              {page.stock_visible && (
                <th scope="col" className="inventory-number">
                  Pendiente de recibir
                </th>
              )}
              <th scope="col" className="inventory-number">
                Reabastecer en
              </th>
              {page.stock_visible && (
                <th scope="col" className="inventory-number">
                  Necesidad
                </th>
              )}
              <th scope="col" className="inventory-number">
                Sugerido
              </th>
              {page.purchasing_visible && <th scope="col">Último precio</th>}
            </tr>
          </thead>
          <tbody>
            {group.suggestions.map((suggestion) => (
              <SuggestionRow
                key={suggestion.item.id}
                suggestion={suggestion}
                page={page}
                selected={!excluded.includes(suggestion.item.id)}
                selectable={selectable}
                timezone={timezone}
                onToggle={() =>
                  setExcluded((current) =>
                    current.includes(suggestion.item.id)
                      ? current.filter((id) => id !== suggestion.item.id)
                      : [...current, suggestion.item.id],
                  )
                }
              />
            ))}
          </tbody>
        </table>
      </div>
      {selectable && (
        <div className="form-stack">
          {!group.supplier && (
            <SupplierPicker
              label="Proveedor del borrador"
              value={supplier}
              required
              kind="formal"
              onChange={setSupplier}
            />
          )}
          <div className="button-row">
            <button
              type="button"
              className="primary-button"
              disabled={busy || sending || !supplier || selected.length === 0}
              onClick={() => void create()}
            >
              <IconLabel icon="plus">{sending ? 'Creando…' : 'Crear orden en borrador'}</IconLabel>
            </button>
            <span className="muted text-xs">
              {countLabel(selected.length, 'insumo incluido', 'insumos incluidos')}
            </span>
          </div>
        </div>
      )}
      {dialog}
    </section>
  );
}

// «Sugerencias de compra» (pestaña Compras): insumos comprados con punto de reabastecimiento cuya
// existencia más lo pendiente de recibir no lo alcanza, con lo sugerido en la presentación de
// compra. Go calcula necesidad, redondeo y presentación; la pantalla no compara ni suma. Nunca se
// emite una orden automáticamente.
export function PurchaseSuggestionsView({
  can,
  execute,
  busy,
  timezone,
  onOpenOrder,
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onOpenOrder: (orderID: string) => void;
}) {
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('');
  const [page, setPage] = useState(1);
  const categories = useResource<InventoryCategory[]>('inventory_categories', {
    reloadEvent: inventoryChangeEvent,
  });
  const query: Record<string, string> = { page: String(page), page_size: String(pageSize) };
  if (search.trim() !== '') {
    query.search = search.trim();
  }
  if (category !== '') {
    query.category_id = category;
  }
  const list = useResource<PurchaseSuggestionPage>('inventory_purchase_suggestions', {
    query,
    reloadEvent: inventoryChangeEvent,
  });
  const creatable = can('inventory.purchases.create');
  const data = list.data;
  const groups = data ? groupSuggestions(data.items) : [];
  return (
    <div className="form-stack">
      <p className="muted text-sm">
        Insumos cuyo punto de reabastecimiento no alcanzan la existencia y lo pendiente de recibir,
        con lo sugerido en su presentación de compra. Los borradores no cuentan como pendiente.
        Ninguna orden se emite sola.
      </p>
      {!creatable && (
        <Notice>
          Consulta disponible. Crear el borrador requiere el permiso «
          {purchasingPermissionName('inventory.purchases.create')}».
        </Notice>
      )}
      <section className="panel catalog-filters" aria-label="Filtros de sugerencias de compra">
        <Field label="Buscar insumo">
          <input
            type="search"
            maxLength={100}
            value={search}
            placeholder="Nombre o código"
            onChange={(event) => {
              setSearch(event.target.value);
              setPage(1);
            }}
          />
        </Field>
        <Field label="Categoría">
          <Select
            value={category}
            options={[
              { value: '', label: 'Todas' },
              ...(categories.data ?? []).map((entry) => ({ value: entry.id, label: entry.name })),
            ]}
            onChange={(next) => {
              setCategory(next);
              setPage(1);
            }}
          />
        </Field>
      </section>
      <Notice>{list.error}</Notice>
      {!data && !list.error && <p className="muted">Consultando sugerencias de compra…</p>}
      {data && (
        <section className="panel form-stack" aria-label="Sugerencias de compra">
          {data.items.length === 0 && (
            <p className="muted">
              Ningún insumo necesita reabastecerse con los puntos de reabastecimiento configurados.
            </p>
          )}
          {groups.map((group) => (
            <SuggestionGroupPanel
              key={`${group.key}:${data.page}`}
              group={group}
              page={data}
              creatable={creatable}
              execute={execute}
              busy={busy}
              timezone={timezone}
              onCreated={(order) => onOpenOrder(order.id)}
              onChanged={() => void list.reload()}
            />
          ))}
          {data.total > data.page_size && (
            <Pagination
              page={data.page}
              pageSize={data.page_size}
              total={data.total}
              singular="insumo"
              plural="insumos"
              onPage={setPage}
            />
          )}
        </section>
      )}
    </div>
  );
}
