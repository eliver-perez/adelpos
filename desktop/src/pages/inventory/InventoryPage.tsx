import { useState } from 'react';
import { Notice, PageTitle } from '../../components/Forms';
import { AdjustmentsView } from './AdjustmentsView';
import { CountsView } from './CountsView';
import { inventoryPermissionName } from './inventoryFormat';
import type { InventoryTarget } from './inventoryNavigation';
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
import { TransfersView } from './TransfersView';
import { useInventoryMutation } from './useInventoryMutation';
import { WarehousesView } from './WarehousesView';

export type InventorySection =
  | 'summary'
  | 'items'
  | 'recipes'
  | 'warehouses'
  | 'stock'
  | 'movements'
  | 'transfers'
  | 'adjustments'
  | 'counts'
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

// Pestañas en el orden de la especificación (§25): Resumen, Insumos, Recetas, Almacenes,
// Existencias, Movimientos, Transferencias, Ajustes y mermas, Conteos, Compras, Recepciones,
// Proveedores y Configuración. Cada una se muestra con alguno de los permisos con los que Go deja
// leer su contenido (Transferencias: transferir o leer movimientos; Conteos: realizarlos o
// aprobarlos; Compras, Recepciones y Proveedores: cualquiera de los seis permisos de compras). No se
// muestran pestañas vacías. Go vuelve a comprobar cada permiso en cada consulta y escritura.
const sections: SectionDefinition[] = [
  { id: 'summary', label: 'Resumen', permission: 'inventory.read' },
  { id: 'items', label: 'Insumos', permission: 'inventory.read' },
  { id: 'recipes', label: 'Recetas', permission: 'inventory.read' },
  { id: 'warehouses', label: 'Almacenes', permission: 'inventory.read' },
  { id: 'stock', label: 'Existencias', permission: 'inventory.stock.read' },
  { id: 'movements', label: 'Movimientos', permission: 'inventory.movements.read' },
  {
    id: 'transfers',
    label: 'Transferencias',
    permission: ['inventory.transfers.record', 'inventory.movements.read'],
  },
  { id: 'adjustments', label: 'Ajustes y mermas', permission: 'inventory.read' },
  {
    id: 'counts',
    label: 'Conteos',
    permission: ['inventory.counts.perform', 'inventory.counts.approve'],
  },
  { id: 'purchases', label: 'Compras', permission: purchasingReadPermissions },
  { id: 'receipts', label: 'Recepciones', permission: purchasingReadPermissions },
  { id: 'suppliers', label: 'Proveedores', permission: purchasingReadPermissions },
  { id: 'settings', label: 'Configuración', permission: 'inventory.settings.manage' },
];

function allowed(entry: SectionDefinition, can: (permission: string) => boolean) {
  const permissions = Array.isArray(entry.permission) ? entry.permission : [entry.permission];
  return permissions.some((permission) => can(permission));
}

// Lo que una pestaña abre al llegar desde otra (una orden desde una recepción, un insumo desde una
// alerta…). La versión vuelve a montar la pestaña para aplicar los valores aunque ya estuviera
// abierta; elegir una pestaña desde la barra la abre sin destino.
interface Destination {
  version: number;
  target: InventoryTarget | null;
}

// Sección principal «Inventarios» (etapa 8): resumen, insumos, recetas, almacenes, existencias,
// movimientos, transferencias, ajustes y mermas, conteos, compras, recepciones, proveedores y
// configuración, cada pestaña según sus permisos. Go es la autoridad de cantidades,
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
  const [destination, setDestination] = useState<Destination>({ version: 0, target: null });
  const mutation = useInventoryMutation();
  const available = sections.filter((entry) => allowed(entry, can));
  const current = available.some((entry) => entry.id === section) ? section : 'summary';

  function navigate(target: InventoryTarget) {
    setDestination((previous) => ({ version: previous.version + 1, target }));
    setSection(target.section);
  }

  const navigation: PurchasingNavigation = {
    openOrder: (id) => navigate({ section: 'purchases', orderID: id }),
    openReceipt: (id) => navigate({ section: 'receipts', receiptID: id }),
    openSupplier: (id) => navigate({ section: 'suppliers', supplierID: id }),
  };

  // Elegir una pestaña desde la barra abre su listado, no el último documento enfocado.
  function chooseSection(target: InventorySection) {
    setDestination((previous) => ({ version: previous.version + 1, target: null }));
    setSection(target);
  }

  // Destino vigente de la pestaña indicada (o null si se abrió desde la barra).
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
      case 'purchases': {
        const preset = arrival('purchases');
        return (
          <PurchaseOrdersView
            key={key}
            can={can}
            execute={mutation.execute}
            busy={mutation.busy}
            timezone={timezone}
            initialOrderID={preset?.orderID ?? null}
            initialSubsection={preset?.subsection ?? 'orders'}
            navigation={navigation}
          />
        );
      }
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
