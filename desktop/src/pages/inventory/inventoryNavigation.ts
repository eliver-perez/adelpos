import type { StockState } from '../../services/inventory';

// Destino de una navegación dentro de Inventarios: una sección con lo que debe abrir al llegar
// (un documento, una búsqueda, un almacén, una categoría o un apartado). Lo usan el resumen y sus
// alertas, las categorías («Ver insumos») y los documentos de compras para llevar al registro
// relacionado sin mostrar identificadores; InventoryPage cambia de grupo y de sección y vuelve a
// montarla con estos valores iniciales. Los destinos de la etapa 8 se conservan: «purchases» con
// subsection 'suggestions' abre la sección de sugerencias (etapa 9A).
export type InventoryTarget =
  | { section: 'stock'; state?: StockState | ''; search?: string; warehouseID?: string }
  | { section: 'items'; itemID?: string; categoryID?: string }
  | { section: 'recipes'; search: string }
  | { section: 'movements' }
  | { section: 'transfers' }
  | { section: 'adjustments' }
  | { section: 'counts'; countID?: string; warehouseID?: string }
  | { section: 'purchases'; orderID?: string; subsection?: 'orders' | 'suggestions' }
  | { section: 'suggestions' }
  | { section: 'receipts'; receiptID: string }
  | { section: 'supplier-returns'; returnID?: string }
  | { section: 'suppliers'; supplierID: string }
  | { section: 'categories' }
  | { section: 'units' };

export type InventoryNavigate = (target: InventoryTarget) => void;
