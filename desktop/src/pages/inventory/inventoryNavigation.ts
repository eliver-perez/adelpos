import type { StockState } from '../../services/inventory';

// Destino de una navegación dentro de Inventarios: una pestaña con lo que debe abrir al llegar
// (un documento, una búsqueda, un almacén o un apartado). Lo usan el resumen y sus alertas para
// llevar al registro relacionado sin mostrar identificadores; InventoryPage cambia de pestaña y
// vuelve a montarla con estos valores iniciales.
export type InventoryTarget =
  | { section: 'stock'; state?: StockState | ''; search?: string; warehouseID?: string }
  | { section: 'items'; itemID?: string }
  | { section: 'recipes'; search: string }
  | { section: 'movements' }
  | { section: 'transfers' }
  | { section: 'adjustments' }
  | { section: 'counts'; countID?: string; warehouseID?: string }
  | { section: 'purchases'; orderID?: string; subsection?: 'orders' | 'suggestions' }
  | { section: 'receipts'; receiptID: string }
  | { section: 'suppliers'; supplierID: string };

export type InventoryNavigate = (target: InventoryTarget) => void;
