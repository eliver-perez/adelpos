import { useState } from 'react';
import { Notice, PageTitle } from '../../components/Forms';
import { useBreadcrumbTail } from '../../navigation/NavigationContext';
import { AdjustmentsView } from './AdjustmentsView';
import { CategoriesView, UnitsView } from './ConfigurationViews';
import { CountsView } from './CountsView';
import { inventoryPermissionName } from './inventoryFormat';
import type { InventoryTarget } from './inventoryNavigation';
import { availableNavigation, groupOf, inventorySections } from './inventorySections';
import type { InventoryGroup, InventorySection } from './inventorySections';
import { ItemsView } from './ItemsView';
import { MovementsView } from './MovementsView';
import { PurchaseOrdersView } from './PurchaseOrdersView';
import { PurchaseSuggestionsView } from './PurchaseSuggestionsView';
import type { PurchasingNavigation } from './purchasingNavigation';
import { ReceiptsView } from './ReceiptsView';
import { RecipesView } from './RecipesView';
import { SettingsView } from './SettingsView';
import { StockView } from './StockView';
import { SummaryView } from './SummaryView';
import { SupplierReturnsView } from './SupplierReturnsView';
import { SuppliersView } from './SuppliersView';
import { TransfersView } from './TransfersView';
import { useInventoryMutation } from './useInventoryMutation';
import { WarehousesView } from './WarehousesView';

export type { InventorySection } from './inventorySections';

// Lo que una sección abre al llegar desde otra (una orden desde una recepción, un insumo desde una
// alerta, los insumos de una categoría…). La versión vuelve a montar la sección para aplicar los
// valores aunque ya estuviera abierta; elegir un grupo o una sección desde la navegación la abre
// sin destino.
interface Destination {
  version: number;
  target: InventoryTarget | null;
}

// Sección a la que lleva un destino: los de la etapa 8 se conservan y «purchases» con el apartado
// de sugerencias abre la sección propia de sugerencias (etapa 9A).
function targetSection(target: InventoryTarget): InventorySection {
  if (target.section === 'purchases' && target.subsection === 'suggestions' && !target.orderID) {
    return 'suggestions';
  }
  return target.section;
}

// La segunda fila se omite solo cuando el grupo tiene una única sección con su mismo nombre
// (Resumen); con una sola sección de otro nombre se muestra para que se sepa qué se abrió.
function showSections(group: { label: string; sections: { label: string }[] }) {
  return group.sections.length > 1 || group.sections[0]?.label !== group.label;
}

// Sección principal «Inventarios» (etapa 8, reorganizada en la 9A): cinco grupos — Resumen,
// Catálogo, Existencias, Compras y Configuración — con sus secciones en una segunda fila, cada una
// según sus permisos. Cada grupo recuerda la última sección abierta mientras la pantalla siga
// montada. Go es la autoridad de cantidades, existencias y costos: la pantalla muestra lo que
// entrega, sin aritmética de cantidades ni dinero. No cambia el menú principal de la aplicación.
export function InventoryPage({
  can,
  timezone,
  onOpenSettings,
}: {
  can: (permission: string) => boolean;
  timezone: string;
  // Abre la pantalla «Configuración» (funciones del establecimiento) para activar el control.
  onOpenSettings: () => void;
}) {
  const [section, setSection] = useState<InventorySection>('summary');
  // Última sección abierta de cada grupo, para volver a ella al regresar al grupo.
  const [remembered, setRemembered] = useState<Partial<Record<InventoryGroup, InventorySection>>>(
    {},
  );
  const [destination, setDestination] = useState<Destination>({ version: 0, target: null });
  const mutation = useInventoryMutation();
  const groups = availableNavigation(can);
  const allowed = groups.flatMap((group) => group.sections.map((entry) => entry.id));
  const fallback = allowed[0] ?? 'summary';
  const current = allowed.includes(section) ? section : fallback;
  const currentGroup = groups.find((group) => group.id === groupOf(current)) ?? null;
  // Etapa 9C: el apartado visible es el último tramo de la ruta («… › Inventarios › Recetas»).
  const currentLabel = inventorySections.find((entry) => entry.id === current)?.label ?? null;
  useBreadcrumbTail(currentLabel);

  function open(next: InventorySection, target: InventoryTarget | null) {
    setDestination((previous) => ({ version: previous.version + 1, target }));
    setSection(next);
    setRemembered((previous) => ({ ...previous, [groupOf(next)]: next }));
  }

  function navigate(target: InventoryTarget) {
    open(targetSection(target), target);
  }

  const navigation: PurchasingNavigation = {
    openOrder: (id) => navigate({ section: 'purchases', orderID: id }),
    openReceipt: (id) => navigate({ section: 'receipts', receiptID: id }),
    openSupplier: (id) => navigate({ section: 'suppliers', supplierID: id }),
  };

  // Elegir un grupo abre la última sección que se usó en él (o la primera permitida), en su
  // listado y sin el último documento enfocado.
  function chooseGroup(group: InventoryGroup) {
    const entry = groups.find((candidate) => candidate.id === group);
    const first = entry?.sections[0];
    if (!entry || !first) {
      return;
    }
    const last = remembered[group];
    const known = last !== undefined && entry.sections.some((candidate) => candidate.id === last);
    open(known ? last : first.id, null);
  }

  // Elegir una sección desde la segunda fila abre su listado, no el último documento enfocado.
  function chooseSection(target: InventorySection) {
    open(target, null);
  }

  // Destino vigente de la sección indicada (o null si se abrió desde la navegación).
  function arrival<S extends InventoryTarget['section']>(target: S) {
    const value = destination.target;
    return value && value.section === target
      ? (value as Extract<InventoryTarget, { section: S }>)
      : null;
  }

  function content() {
    const key = destination.version;
    switch (current) {
      case 'items':
        return (
          <ItemsView
            key={key}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            initialItemID={arrival('items')?.itemID ?? null}
            initialCategoryID={arrival('items')?.categoryID ?? ''}
          />
        );
      case 'recipes':
        return (
          <RecipesView
            key={key}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
            initialSearch={arrival('recipes')?.search ?? ''}
          />
        );
      case 'adjustments':
        return (
          <AdjustmentsView
            key={key}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
            onNavigate={navigate}
          />
        );
      case 'warehouses':
        return (
          <WarehousesView
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
          />
        );
      case 'stock': {
        const preset = arrival('stock');
        return (
          <StockView
            key={key}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            initialState={preset?.state ?? ''}
            initialSearch={preset?.search ?? ''}
            initialWarehouse={preset?.warehouseID ?? ''}
            timezone={timezone}
          />
        );
      }
      case 'movements':
        return <MovementsView key={key} timezone={timezone} />;
      case 'transfers':
        return (
          <TransfersView
            key={key}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
          />
        );
      case 'counts': {
        const preset = arrival('counts');
        return (
          <CountsView
            key={key}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
            initialCountID={preset?.countID ?? null}
            initialWarehouseID={preset?.warehouseID ?? null}
            onNavigate={navigate}
          />
        );
      }
      case 'purchases':
        return (
          <PurchaseOrdersView
            key={key}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
            initialOrderID={arrival('purchases')?.orderID ?? null}
            navigation={navigation}
          />
        );
      case 'suggestions':
        return (
          <PurchaseSuggestionsView
            key={key}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
            onOpenOrder={navigation.openOrder}
          />
        );
      case 'supplier-returns':
        return <SupplierReturnsView key={key} timezone={timezone} navigation={navigation} />;
      case 'receipts':
        return (
          <ReceiptsView
            key={key}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
            initialReceiptID={arrival('receipts')?.receiptID ?? null}
            navigation={navigation}
          />
        );
      case 'suppliers':
        return (
          <SuppliersView
            key={key}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
            initialSupplierID={arrival('suppliers')?.supplierID ?? null}
            navigation={navigation}
          />
        );
      case 'units':
        return <UnitsView key={key} can={can} execute={mutation.execute} busy={mutation.busy} />;
      case 'categories':
        return (
          <CategoriesView
            key={key}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            onViewItems={(category) => navigate({ section: 'items', categoryID: category.id })}
          />
        );
      case 'settings':
        return <SettingsView execute={mutation.execute} busy={mutation.busy} timezone={timezone} />;
      default:
        return (
          <SummaryView
            key={key}
            can={can}
            timezone={timezone}
            onNavigate={navigate}
            onOpenSettings={onOpenSettings}
          />
        );
    }
  }

  return (
    <div className="inventory-page form-stack">
      <PageTitle
        title="Inventarios"
        description="Insumos, recetas, almacenes, existencias y movimientos."
      />
      {!can('inventory.read') && (
        <Notice>
          Consultar inventario requiere el permiso «{inventoryPermissionName('inventory.read')}».
        </Notice>
      )}
      {mutation.uncertain && (
        <div className="operation-recovery" role="status">
          <p>
            El resultado del último cambio de inventario no pudo confirmarse. Recupéralo con
            «Reintentar sin duplicar» antes de realizar otro.
          </p>
          <button
            type="button"
            className="primary-button"
            disabled={mutation.running}
            onClick={() => void mutation.retry()}
          >
            Reintentar sin duplicar
          </button>
        </div>
      )}
      <nav className="inventory-navigation" aria-label="Inventarios">
        <div
          className="catalog-tabs inventory-groups"
          role="tablist"
          aria-label="Secciones de inventario"
        >
          {groups.map((group) => (
            <button
              type="button"
              key={group.id}
              role="tab"
              aria-selected={currentGroup?.id === group.id}
              className={currentGroup?.id === group.id ? 'tab-active' : ''}
              onClick={() => chooseGroup(group.id)}
            >
              {group.label}
            </button>
          ))}
        </div>
        {currentGroup && showSections(currentGroup) && (
          <div
            className="inventory-sections"
            role="tablist"
            aria-label={`Apartados de ${currentGroup.label}`}
          >
            {currentGroup.sections.map((entry) => (
              <button
                type="button"
                key={entry.id}
                role="tab"
                aria-selected={current === entry.id}
                className={current === entry.id ? 'tab-active' : ''}
                onClick={() => chooseSection(entry.id)}
              >
                {entry.label}
              </button>
            ))}
          </div>
        )}
      </nav>
      {can('inventory.read') && content()}
    </div>
  );
}
