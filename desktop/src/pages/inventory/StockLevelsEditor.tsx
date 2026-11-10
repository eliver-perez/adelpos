import { useState } from 'react';
import type { FormEvent } from 'react';
import { Notice } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, InventoryItem, Page, Warehouse } from '../../services/inventory';
import { saveStockLevel } from '../../services/inventoryControl';
import type { StockLevel } from '../../services/inventoryControl';
import { notifyError, notifySuccess } from '../../services/notify';
import { inventoryQuantityShape } from '../../services/numericInput';
import { DecimalTextField } from './InventoryFields';
import { formatQuantity, inventoryPermissionName } from './inventoryFormat';

// Formulario del nivel de un almacén: mínimo y punto de reabastecimiento en la unidad base. Los
// dos vacíos quitan el nivel (el almacén vuelve a usar los del insumo). Go valida ≥ 0 y hasta
// 6 decimales y la revisión.
function LevelForm({
  item,
  warehouse,
  level,
  execute,
  busy,
  onDone,
}: {
  item: InventoryItem;
  warehouse: Warehouse;
  level: StockLevel | undefined;
  execute: InventoryExecute;
  busy: boolean;
  onDone: () => void;
}) {
  const [minimum, setMinimum] = useState(level?.min_stock ?? '');
  const [reorder, setReorder] = useState(level?.reorder_point ?? '');
  const [sending, setSending] = useState(false);
  const unitHint = `En ${item.base_unit.name.toLowerCase()} (${item.base_unit.code}). Vacío: sin ese nivel.`;

  async function save(clear: boolean, event?: FormEvent) {
    event?.preventDefault();
    setSending(true);
    try {
      await saveStockLevel(execute, {
        item_id: item.id,
        warehouse_id: warehouse.id,
        revision: level?.revision ?? 0,
        min_stock: clear || minimum.trim() === '' ? null : minimum,
        reorder_point: clear || reorder.trim() === '' ? null : reorder,
      });
      const removed = clear || (minimum.trim() === '' && reorder.trim() === '');
      notifySuccess(
        removed
          ? `Nivel de «${warehouse.name}» quitado: se usan los del insumo.`
          : `Nivel de «${warehouse.name}» guardado.`,
      );
      onDone();
    } catch (problem) {
      notifyError(problem, 'No se pudo guardar el nivel del almacén.');
    } finally {
      setSending(false);
    }
  }

  const locked = busy || sending;
  return (
    <form
      className="form-stack inventory-correction"
      aria-label={`Nivel de ${warehouse.name}`}
      onSubmit={(event) => void save(false, event)}
    >
      <div className="form-grid">
        <DecimalTextField
          label={`Mínimo en ${warehouse.name}`}
          value={minimum}
          onChange={setMinimum}
          shape={inventoryQuantityShape}
          disabled={locked}
          hint={unitHint}
        />
        <DecimalTextField
          label={`Reabastecer en ${warehouse.name}`}
          value={reorder}
          onChange={setReorder}
          shape={inventoryQuantityShape}
          disabled={locked}
          hint={unitHint}
        />
      </div>
      <div className="button-row">
        <button type="submit" className="primary-button" disabled={locked}>
          {sending ? 'Guardando…' : 'Guardar nivel'}
        </button>
        {level?.configured && (
          <button
            type="button"
            className="secondary-button"
            disabled={locked}
            onClick={() => void save(true)}
          >
            Quitar nivel
          </button>
        )}
        <button type="button" className="secondary-button" onClick={onDone}>
          Cancelar
        </button>
      </div>
    </form>
  );
}

function levelText(level: StockLevel | undefined, item: InventoryItem) {
  if (!level?.configured) {
    return 'Usa los niveles del insumo';
  }
  const parts: string[] = [];
  if (level.min_stock !== null) {
    parts.push(`Mín. ${formatQuantity(level.min_stock, item.base_unit)}`);
  }
  if (level.reorder_point !== null) {
    parts.push(`Reabastecer en ${formatQuantity(level.reorder_point, item.base_unit)}`);
  }
  return parts.join(' · ');
}

// «Niveles por almacén» de un insumo guardado (inventory.items.manage para cambiarlos). Sobrescriben
// el mínimo y el punto de reabastecimiento del insumo para ese almacén: las alertas del almacén y
// Existencias filtrada por él comparan su existencia con estos niveles; los del insumo se siguen
// comparando con la existencia total. La existencia y sus marcas solo llegan con
// inventory.stock.read.
export function StockLevelsEditor({
  item,
  editable,
  execute,
  busy,
}: {
  item: InventoryItem;
  editable: boolean;
  execute: InventoryExecute;
  busy: boolean;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const levels = useResource<Page<StockLevel>>('inventory_stock_levels', {
    query: { item_id: item.id, page_size: '100' },
    reloadEvent: inventoryChangeEvent,
  });
  const configured = levels.data?.items ?? [];
  const offered = (warehouses.data ?? []).filter(
    (warehouse) =>
      warehouse.active || configured.some((level) => level.warehouse.id === warehouse.id),
  );
  return (
    <section className="inventory-subsection form-stack" aria-labelledby="inventory-levels-title">
      <div>
        <h3 id="inventory-levels-title">Niveles por almacén</h3>
        <p className="muted text-xs">
          Mínimo y punto de reabastecimiento propios de cada almacén. Sin nivel propio, el almacén
          usa los del insumo, que se comparan con la existencia total.
        </p>
      </div>
      <Notice>{warehouses.error || levels.error}</Notice>
      {!editable && (
        <p className="muted text-xs">
          Cambiarlos requiere el permiso «{inventoryPermissionName('inventory.items.manage')}».
        </p>
      )}
      <ul className="inventory-list">
        {offered.map((warehouse) => {
          const level = configured.find((entry) => entry.warehouse.id === warehouse.id);
          return (
            <li key={warehouse.id} className="inventory-list-row control-level-row">
              <div className="inventory-list-text">
                <strong>{warehouse.name}</strong>
                <span className="muted text-xs">{levelText(level, item)}</span>
                {level?.stock_visible && level.quantity !== null && (
                  <span className="text-xs">
                    Existencia: {formatQuantity(level.quantity, item.base_unit)}
                  </span>
                )}
              </div>
              <div className="button-row">
                {level?.below_minimum && <span className="tag tag-warning">Bajo mínimo</span>}
                {level?.at_reorder_point && <span className="tag tag-warning">Reabastecer</span>}
                {editable && editing !== warehouse.id && (
                  <button
                    type="button"
                    className="secondary-button"
                    aria-label={`Editar el nivel de ${warehouse.name}`}
                    disabled={busy || editing !== null}
                    onClick={() => setEditing(warehouse.id)}
                  >
                    {level?.configured ? 'Editar nivel' : 'Definir nivel'}
                  </button>
                )}
              </div>
              {editable && editing === warehouse.id && (
                <LevelForm
                  key={`${warehouse.id}:${level?.revision ?? 0}`}
                  item={item}
                  warehouse={warehouse}
                  level={level}
                  execute={execute}
                  busy={busy}
                  onDone={() => setEditing(null)}
                />
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
