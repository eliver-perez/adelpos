import { useState } from 'react';
import type { ReactNode } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field } from '../../components/Forms';
import { ToggleButton } from '../../components/ToggleButton';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryItem, ItemPage } from '../../services/inventory';
import { acceptDecimalText } from '../../services/numericInput';
import type { DecimalShape } from '../../services/numericInput';
import { countLabel } from '../../services/plural';
import { Select } from '../../components/Select';

// Cantidad, costo o equivalencia capturados como texto (nunca un campo numérico nativo): un carácter no válido
// se descarta al teclear y la cadena viaja a Go tal como se capturó; Go la vuelve a validar.
export function DecimalTextField({
  label,
  value,
  onChange,
  shape,
  required = false,
  disabled = false,
  hint,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  shape: DecimalShape;
  required?: boolean;
  disabled?: boolean;
  hint?: string;
  placeholder?: string;
}) {
  return (
    <Field label={label} hint={hint}>
      <input
        type="text"
        inputMode="decimal"
        autoComplete="off"
        value={value}
        required={required}
        disabled={disabled}
        placeholder={placeholder}
        // Un punto final («12.») queda a medio capturar: la validación nativa lo señala.
        pattern="[0-9]+([.][0-9]+)?"
        onChange={(event) => onChange(acceptDecimalText(event.target.value, value, shape))}
      />
    </Field>
  );
}

// Interruptor «activo» de un registro guardado. Desactivar se confirma antes (tono de peligro):
// el registro deja de ofrecerse aunque conserve su historial; el cambio se aplica al guardar.
export function ActiveToggle({
  label,
  pressed,
  onChange,
  confirmDeactivation,
  title,
  message,
  disabled = false,
}: {
  label: string;
  pressed: boolean;
  onChange: (pressed: boolean) => void;
  // Solo se pregunta al desactivar un registro que hoy está activo.
  confirmDeactivation: boolean;
  title: string;
  message: ReactNode;
  disabled?: boolean;
}) {
  const { confirm, dialog } = useConfirmDialog();

  async function change(next: boolean) {
    if (!next && confirmDeactivation) {
      const confirmed = await confirm({
        title,
        message,
        confirmLabel: 'Desactivar',
        cancelLabel: 'Mantener activo',
        tone: 'danger',
      });
      if (!confirmed) {
        return;
      }
    }
    onChange(next);
  }

  return (
    <>
      <ToggleButton
        label={label}
        pressed={pressed}
        disabled={disabled}
        onChange={(next) => void change(next)}
      />
      {dialog}
    </>
  );
}

// Paginación con los totales que entrega Go ({total, page, page_size}).
export function Pagination({
  page,
  pageSize,
  total,
  singular,
  plural,
  onPage,
}: {
  page: number;
  pageSize: number;
  total: number;
  singular: string;
  plural: string;
  onPage: (page: number) => void;
}) {
  const pages = Math.max(1, Math.ceil(total / Math.max(1, pageSize)));
  return (
    <div className="catalog-pagination">
      <span className="muted text-sm">
        {countLabel(total, singular, plural)} · Página {page} de {pages}
      </span>
      <div className="button-row">
        <button
          type="button"
          className="secondary-button"
          disabled={page <= 1}
          onClick={() => onPage(page - 1)}
        >
          Anterior
        </button>
        <button
          type="button"
          className="secondary-button"
          disabled={page >= pages}
          onClick={() => onPage(page + 1)}
        >
          Siguiente
        </button>
      </div>
    </div>
  );
}

// Selector de insumo con búsqueda por nombre, código o código de barras (también de sus
// presentaciones). La búsqueda va integrada en la lista (etapa 9C) y la resuelve Go: hasta 50
// coincidencias; el insumo elegido se conserva aunque deje de aparecer entre los resultados.
export function ItemPicker({
  label,
  value,
  onChange,
  activeOnly = true,
  emptyLabel = 'Elige un insumo',
  required = false,
}: {
  label: string;
  value: InventoryItem | null;
  onChange: (item: InventoryItem | null) => void;
  activeOnly?: boolean;
  emptyLabel?: string;
  required?: boolean;
}) {
  const [search, setSearch] = useState('');
  const query: Record<string, string> = { page_size: '50' };
  if (search.trim() !== '') {
    query.search = search.trim();
  }
  if (activeOnly) {
    query.active = 'true';
  }
  const results = useResource<ItemPage>('inventory_items', {
    query,
    reloadEvent: inventoryChangeEvent,
  });
  const found = results.data?.items ?? [];
  const options = value && !found.some((item) => item.id === value.id) ? [value, ...found] : found;
  return (
    <div className="inventory-picker">
      <Field label={label}>
        <Select
          value={value?.id ?? ''}
          required={required}
          placeholder={emptyLabel}
          options={[
            ...(required ? [] : [{ value: '', label: emptyLabel }]),
            ...options.map((item) => ({
              value: item.id,
              label: `${item.name} · ${item.code}${item.active ? '' : ' (inactivo)'}`,
            })),
          ]}
          searchLabel={`Buscar ${label.toLowerCase()}`}
          searchPlaceholder="Nombre, código o código de barras"
          onSearchChange={setSearch}
          loading={results.loading}
          onChange={(id) => onChange(options.find((item) => item.id === id) ?? null)}
        />
      </Field>
      {results.error && <small className="muted">{results.error}</small>}
    </div>
  );
}
