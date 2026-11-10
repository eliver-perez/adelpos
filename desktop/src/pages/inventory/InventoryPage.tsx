import { useState } from 'react';
import { Notice, PageTitle } from '../../components/Forms';
import type { StockState } from '../../services/inventory';
import { AdjustmentsView } from './AdjustmentsView';
import { inventoryPermissionName } from './inventoryFormat';
import { ItemsView } from './ItemsView';
import { MovementsView } from './MovementsView';
import { PurchaseOrdersView } from './PurchaseOrdersView';
import { purchasingReadPermissions } from './purchasingFormat';
import type { PurchasingNavigation } from './purchasingNavigation';
import { ReceiptsView } from './ReceiptsView';
import { RecipesView } from './RecipesView';
import { SettingsView } from './SettingsView';
import { StockView } from './StockView';
import { SummaryView } from './SummaryView';
import { SuppliersView } from './SuppliersView';
import { useInventoryMutation } from './useInventoryMutation';
import { WarehousesView } from './WarehousesView';

export type InventorySection =
  | 'summary'
  | 'items'
  | 'recipes'
  | 'warehouses'
  | 'stock'
  | 'movements'
  | 'adjustments'
  | 'purchases'
  | 'receipts'
  | 'suppliers'
  | 'settings';

interface SectionDefinition {
  id: InventorySection;
  label: string;
  // La pestaña se muestra con cualquiera de los permisos indicados.
  permission: string | string[];
}

// Pestañas en el orden de la especificación (§25). La fase B agrega Recetas y Ajustes y mermas
// (por ahora con las cancelaciones por clasificar); la fase C, Compras, Recepciones y Proveedores
// (con cualquiera de los seis permisos de compras, como la lectura en Go); la fase D agrega
// Transferencias y Conteos. No se muestran pestañas vacías. Go vuelve a comprobar cada permiso
// en cada consulta y escritura.
const sections: SectionDefinition[] = [
  { id: 'summary', label: 'Resumen', permission: 'inventory.read' },
  { id: 'items', label: 'Insumos', permission: 'inventory.read' },
  { id: 'recipes', label: 'Recetas', permission: 'inventory.read' },
  { id: 'warehouses', label: 'Almacenes', permission: 'inventory.read' },
  { id: 'stock', label: 'Existencias', permission: 'inventory.stock.read' },
  { id: 'movements', label: 'Movimientos', permission: 'inventory.movements.read' },
  { id: 'adjustments', label: 'Ajustes y mermas', permission: 'inventory.read' },
  { id: 'purchases', label: 'Compras', permission: purchasingReadPermissions },
  { id: 'receipts', label: 'Recepciones', permission: purchasingReadPermissions },
  { id: 'suppliers', label: 'Proveedores', permission: purchasingReadPermissions },
  { id: 'settings', label: 'Configuración', permission: 'inventory.settings.manage' },
];

function allowed(entry: SectionDefinition, can: (permission: string) => boolean) {
  const permissions = Array.isArray(entry.permission) ? entry.permission : [entry.permission];
  return permissions.some((permission) => can(permission));
}

// Documento que una pestaña de compras abre al llegar desde otra (una orden desde una recepción,
// una recepción desde el historial de un proveedor…). La versión vuelve a montar la pestaña.
interface PurchasingFocus {
  section: InventorySection;
  id: string | null;
  version: number;
}

// Sección principal «Inventarios» (etapa 8): resumen, insumos, recetas, almacenes, existencias,
// movimientos, ajustes y mermas, compras, recepciones, proveedores y configuración, cada pestaña
// según sus permisos. Go es la autoridad de cantidades,
// existencias y costos: la pantalla muestra lo que entrega, sin aritmética de cantidades ni dinero.
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
  // Filtro con el que el resumen abre Existencias (por ejemplo, solo las negativas). El contador
  // vuelve a montar la vista para aplicar el filtro aunque ya estuviera abierta.
  const [stockPreset, setStockPreset] = useState<{ state: StockState | ''; version: number }>({
    state: '',
    version: 0,
  });
  const [focus, setFocus] = useState<PurchasingFocus>({ section: 'summary', id: null, version: 0 });
  const mutation = useInventoryMutation();
  const available = sections.filter((entry) => allowed(entry, can));
  const current = available.some((entry) => entry.id === section) ? section : 'summary';

  function openDocument(target: InventorySection, id: string) {
    setFocus((previous) => ({ section: target, id, version: previous.version + 1 }));
    setSection(target);
  }

  const navigation: PurchasingNavigation = {
    openOrder: (id) => openDocument('purchases', id),
    openReceipt: (id) => openDocument('receipts', id),
    openSupplier: (id) => openDocument('suppliers', id),
  };

  // Elegir una pestaña desde la barra abre su listado, no el último documento enfocado.
  function chooseSection(target: InventorySection) {
    setFocus((previous) => ({ section: target, id: null, version: previous.version + 1 }));
    setSection(target);
  }

  function focusedID(target: InventorySection) {
    return focus.section === target ? focus.id : null;
  }

  function showStock(state: StockState | '') {
    setStockPreset((previous) => ({ state, version: previous.version + 1 }));
    setSection('stock');
  }

  function content() {
    switch (current) {
      case 'items':
        return <ItemsView can={can} execute={mutation.execute} busy={mutation.busy} />;
      case 'recipes':
        return (
          <RecipesView
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
          />
        );
      case 'adjustments':
        return (
          <AdjustmentsView
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
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
      case 'stock':
        return (
          <StockView
            key={stockPreset.version}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            initialState={stockPreset.state}
            timezone={timezone}
          />
        );
      case 'movements':
        return <MovementsView timezone={timezone} />;
      case 'purchases':
        return (
          <PurchaseOrdersView
            key={focus.version}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
            initialOrderID={focusedID('purchases')}
            navigation={navigation}
          />
        );
      case 'receipts':
        return (
          <ReceiptsView
            key={focus.version}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
            initialReceiptID={focusedID('receipts')}
            navigation={navigation}
          />
        );
      case 'suppliers':
        return (
          <SuppliersView
            key={focus.version}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
            initialSupplierID={focusedID('suppliers')}
            navigation={navigation}
          />
        );
      case 'settings':
        return <SettingsView execute={mutation.execute} busy={mutation.busy} timezone={timezone} />;
      default:
        return (
          <SummaryView
            can={can}
            timezone={timezone}
            onShowStock={showStock}
            onShowSection={setSection}
            onOpenSettings={onOpenSettings}
          />
        );
    }
  }

  return (
    <div className="inventory-page form-stack">
      <PageTitle
        title="Inventarios"
        description="Insumos, recetas, almacenes, existencias y movimientos. Las cantidades, conversiones y costos los calcula y valida el servidor."
      />
      {!can('inventory.read') && (
        <Notice>
          Consultar inventario requiere el permiso «{inventoryPermissionName('inventory.read')}».
        </Notice>
      )}
      {mutation.uncertain && (
        <div className="operation-recovery" role="status">
          <p>
            El resultado del último cambio de inventario no pudo confirmarse. Recupéralo con el
            mismo identificador antes de realizar otro.
          </p>
          <button
            type="button"
            className="primary-button"
            disabled={mutation.running}
            onClick={() => void mutation.retry()}
          >
            Reintentar con el mismo identificador
          </button>
        </div>
      )}
      <div className="catalog-tabs" role="tablist" aria-label="Secciones de inventario">
        {available.map((entry) => (
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
      {can('inventory.read') && content()}
    </div>
  );
}
