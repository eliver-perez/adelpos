import { Notice } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryCategory, InventoryExecute, InventoryUnit } from '../../services/inventory';
import { countLabel } from '../../services/plural';
import { CategoriesAdmin } from './CategoriesAdmin';
import { UnitsAdmin } from './UnitsAdmin';

// Secciones «Unidades» y «Categorías» del grupo Configuración (etapa 9A, §3.6). Hasta la etapa 8
// vivían como acordeones dentro de Insumos; se leen con inventory.read y se modifican con
// inventory.items.manage (Go lo vuelve a comprobar).

export function UnitsView({
  can,
  execute,
  busy,
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
}) {
  const units = useResource<InventoryUnit[]>('inventory_units', {
    reloadEvent: inventoryChangeEvent,
  });
  const unitList = units.data ?? [];
  const custom = unitList.filter((unit) => !unit.system).length;
  return (
    <section className="panel form-stack" aria-label="Unidades de medida">
      <div className="section-heading">
        <div>
          <h2>Unidades de medida</h2>
          <p className="muted text-xs">
            {units.data
              ? `${countLabel(unitList.length - custom, 'del sistema', 'del sistema')} · ${countLabel(custom, 'propia', 'propias')}`
              : 'Consultando unidades…'}
          </p>
        </div>
      </div>
      <Notice>{units.error}</Notice>
      {units.data && (
        <UnitsAdmin
          units={unitList}
          editable={can('inventory.items.manage')}
          execute={execute}
          busy={busy}
        />
      )}
    </section>
  );
}

export function CategoriesView({
  can,
  execute,
  busy,
  onViewItems,
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  onViewItems: (category: InventoryCategory) => void;
}) {
  const categories = useResource<InventoryCategory[]>('inventory_categories', {
    reloadEvent: inventoryChangeEvent,
  });
  const list = categories.data ?? [];
  const active = list.filter((category) => category.active).length;
  return (
    <section className="panel form-stack" aria-label="Categorías de insumos">
      <div className="section-heading">
        <div>
          <h2>Categorías de insumos</h2>
          <p className="muted text-xs">
            {categories.data
              ? `${countLabel(active, 'activa', 'activas')} de ${list.length}`
              : 'Consultando categorías…'}
          </p>
        </div>
      </div>
      <Notice>{categories.error}</Notice>
      {categories.data && (
        <CategoriesAdmin
          categories={list}
          editable={can('inventory.items.manage')}
          execute={execute}
          busy={busy}
          onViewItems={onViewItems}
        />
      )}
    </section>
  );
}
