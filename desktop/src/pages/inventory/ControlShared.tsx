import { Field } from '../../components/Forms';
import { Icon } from '../../components/Icon';
import { APIError } from '../../services/api';
import type { InventoryWarning, Warehouse } from '../../services/inventory';
import type { NotifyOptions } from '../../services/notify';
import type { InventoryNavigate } from './inventoryNavigation';

// Avisos que devuelve Go y que nunca bloquean: existencia negativa, renglones sin contar,
// consumos o cancelaciones pendientes… Se muestran tal como llegan, sin códigos internos.
export function WarningList({
  warnings,
  label = 'Avisos del servidor',
}: {
  warnings: InventoryWarning[];
  label?: string;
}) {
  if (warnings.length === 0) {
    return null;
  }
  return (
    <div className="inventory-negative-notice" role="status" aria-label={label}>
      <Icon name="alert" />
      <ul>
        {warnings.map((warning, index) => (
          <li key={`${warning.code}:${index}`}>{warning.message}</li>
        ))}
      </ul>
    </div>
  );
}

// Selector de almacén activo (más el elegido aunque ya no lo esté). Go vuelve a exigir que el
// almacén esté activo al registrar.
export function WarehouseSelect({
  label,
  warehouses,
  value,
  onChange,
  hint,
  exclude = '',
  emptyLabel,
}: {
  label: string;
  warehouses: Warehouse[];
  value: string;
  onChange: (value: string) => void;
  hint?: string;
  // Almacén que no se ofrece (el origen de una transferencia en el selector de destino).
  exclude?: string;
  // Opción vacía («Elige el almacén»); sin ella el selector siempre tiene un almacén.
  emptyLabel?: string;
}) {
  const offered = warehouses.filter(
    (warehouse) => (warehouse.active || warehouse.id === value) && warehouse.id !== exclude,
  );
  return (
    <Field label={label} hint={hint}>
      <select required value={value} onChange={(event) => onChange(event.target.value)}>
        {emptyLabel !== undefined && <option value="">{emptyLabel}</option>}
        {offered.map((warehouse) => (
          <option key={warehouse.id} value={warehouse.id}>
            {warehouse.name}
            {warehouse.active ? '' : ' (inactivo)'}
          </option>
        ))}
      </select>
    </Field>
  );
}

// Almacén que se propone al abrir un formulario: el predeterminado o el primero activo.
export function defaultWarehouseID(warehouses: Warehouse[]) {
  const active = warehouses.filter((warehouse) => warehouse.active);
  return (active.find((warehouse) => warehouse.is_default) ?? active[0])?.id ?? '';
}

// Una entrada (sobrante, ajuste positivo o aplicación de un conteo) de un insumo sin costo responde
// 409 COST_REQUIRED con el nombre del insumo: el aviso lleva a Insumos (al insumo, si Go entrega su
// identificador en los detalles) para configurar su costo de referencia y reintentar.
export function costRequiredOptions(
  problem: unknown,
  onNavigate: InventoryNavigate | undefined,
): NotifyOptions | undefined {
  if (!(problem instanceof APIError) || problem.code !== 'COST_REQUIRED' || !onNavigate) {
    return undefined;
  }
  const details = problem.details as { item_id?: unknown } | null | undefined;
  const itemID = typeof details?.item_id === 'string' ? details.item_id : undefined;
  return {
    action: {
      label: itemID ? 'Abrir insumo' : 'Ir a Insumos',
      onClick: () => onNavigate({ section: 'items', itemID }),
    },
  };
}
