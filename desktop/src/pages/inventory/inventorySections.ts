import { purchasingReadPermissions } from './purchasingFormat';

// Navegación de Inventarios en dos niveles (etapa 9A, §3.6): cinco grupos fijos en la primera fila y,
// dentro de cada grupo, sus secciones en una segunda fila. Sustituye las trece pestañas planas de la
// etapa 8, que a 1280 × 720 se repartían en dos filas desiguales. Las secciones conservan los
// identificadores de la etapa 8 (los destinos de InventoryTarget siguen funcionando) y se agregan
// las que antes vivían dentro de otra pestaña: sugerencias de compra (de «Compras»), unidades y
// categorías (acordeones de «Insumos») y la lista nueva de devoluciones a proveedor.

export type InventorySection =
  | 'summary'
  | 'items'
  | 'recipes'
  | 'warehouses'
  | 'suppliers'
  | 'stock'
  | 'movements'
  | 'transfers'
  | 'adjustments'
  | 'counts'
  | 'purchases'
  | 'suggestions'
  | 'receipts'
  | 'supplier-returns'
  | 'units'
  | 'categories'
  | 'settings';

export type InventoryGroup = 'summary' | 'catalog' | 'stock' | 'purchasing' | 'configuration';

export interface SectionDefinition {
  id: InventorySection;
  label: string;
  group: InventoryGroup;
  // La sección se muestra con cualquiera de los permisos indicados: los mismos con los que Go deja
  // leer su contenido. Go vuelve a comprobar cada permiso en cada consulta y escritura.
  permission: string | string[];
}

export interface GroupDefinition {
  id: InventoryGroup;
  label: string;
}

export const inventoryGroups: GroupDefinition[] = [
  { id: 'summary', label: 'Resumen' },
  { id: 'catalog', label: 'Catálogo' },
  { id: 'stock', label: 'Existencias' },
  { id: 'purchasing', label: 'Compras' },
  { id: 'configuration', label: 'Configuración' },
];

// Secciones en el orden de cada grupo. Permisos de visibilidad:
//   - Resumen, Insumos, Recetas, Almacenes, Ajustes y mermas, Unidades y Categorías: inventory.read
//     (editar unidades y categorías exige además inventory.items.manage);
//   - Existencias: inventory.stock.read; Movimientos: inventory.movements.read;
//   - Transferencias: transferir o leer movimientos; Conteos: realizarlos o aprobarlos;
//   - Proveedores, Órdenes, Sugerencias, Recepciones y Devoluciones a proveedor: cualquiera de los
//     seis permisos de compras;
//   - Parámetros: inventory.settings.manage.
export const inventorySections: SectionDefinition[] = [
  { id: 'summary', label: 'Resumen', group: 'summary', permission: 'inventory.read' },
  { id: 'items', label: 'Insumos', group: 'catalog', permission: 'inventory.read' },
  { id: 'recipes', label: 'Recetas', group: 'catalog', permission: 'inventory.read' },
  { id: 'warehouses', label: 'Almacenes', group: 'catalog', permission: 'inventory.read' },
  {
    id: 'suppliers',
    label: 'Proveedores',
    group: 'catalog',
    permission: purchasingReadPermissions,
  },
  { id: 'stock', label: 'Existencias', group: 'stock', permission: 'inventory.stock.read' },
  {
    id: 'movements',
    label: 'Movimientos',
    group: 'stock',
    permission: 'inventory.movements.read',
  },
  {
    id: 'transfers',
    label: 'Transferencias',
    group: 'stock',
    permission: ['inventory.transfers.record', 'inventory.movements.read'],
  },
  { id: 'adjustments', label: 'Ajustes y mermas', group: 'stock', permission: 'inventory.read' },
  {
    id: 'counts',
    label: 'Conteos',
    group: 'stock',
    permission: ['inventory.counts.perform', 'inventory.counts.approve'],
  },
  {
    id: 'purchases',
    label: 'Órdenes de compra',
    group: 'purchasing',
    permission: purchasingReadPermissions,
  },
  {
    id: 'suggestions',
    label: 'Sugerencias de compra',
    group: 'purchasing',
    permission: purchasingReadPermissions,
  },
  {
    id: 'receipts',
    label: 'Recepciones',
    group: 'purchasing',
    permission: purchasingReadPermissions,
  },
  {
    id: 'supplier-returns',
    label: 'Devoluciones a proveedor',
    group: 'purchasing',
    permission: purchasingReadPermissions,
  },
  { id: 'units', label: 'Unidades', group: 'configuration', permission: 'inventory.read' },
  { id: 'categories', label: 'Categorías', group: 'configuration', permission: 'inventory.read' },
  {
    id: 'settings',
    label: 'Parámetros',
    group: 'configuration',
    permission: 'inventory.settings.manage',
  },
];

export function sectionAllowed(entry: SectionDefinition, can: (permission: string) => boolean) {
  const permissions = Array.isArray(entry.permission) ? entry.permission : [entry.permission];
  return permissions.some((permission) => can(permission));
}

export function groupOf(section: InventorySection): InventoryGroup {
  return inventorySections.find((entry) => entry.id === section)?.group ?? 'summary';
}

// Grupos visibles con sus secciones permitidas, en orden. Un grupo sin secciones permitidas no se
// muestra (no hay pestañas vacías).
export function availableNavigation(can: (permission: string) => boolean) {
  return inventoryGroups
    .map((group) => ({
      ...group,
      sections: inventorySections.filter(
        (entry) => entry.group === group.id && sectionAllowed(entry, can),
      ),
    }))
    .filter((group) => group.sections.length > 0);
}
