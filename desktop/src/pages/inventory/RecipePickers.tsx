import { useState } from 'react';
import { Field } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { Dimension, InventoryUnit, ItemKind, ItemPage } from '../../services/inventory';
import type {
  IngredientPortionPage,
  ItemReference,
  RecipePage,
} from '../../services/inventoryRecipes';
import { Select } from '../../components/Select';
import type { SelectItem } from '../../components/Select';

// Selectores de componentes de receta: insumos, preparaciones e ingredientes del catálogo, con
// búsqueda en Go (hasta 50 coincidencias) integrada en la lista (etapa 9C). El elemento elegido se
// conserva aunque deje de aparecer en una búsqueda nueva. Ninguno muestra identificadores: solo
// nombres y códigos internos.

// Preparación elegida como componente: su dimensión (la de su rendimiento) decide las unidades
// en que se captura la cantidad.
export interface PreparationChoice {
  id: string;
  name: string;
  dimension: Dimension;
  // Unidad propuesta al elegirla (la de su rendimiento).
  unitID: string;
}

export interface IngredientChoice {
  id: string;
  name: string;
}

function searchQuery(search: string, extra: Record<string, string>) {
  const query: Record<string, string> = { page_size: '50', ...extra };
  if (search.trim() !== '') {
    query.search = search.trim();
  }
  return query;
}

export function RecipeItemPicker({
  label,
  value,
  onChange,
  kind,
  required = false,
  emptyLabel = 'Elige un insumo',
}: {
  label: string;
  value: ItemReference | null;
  onChange: (item: ItemReference | null) => void;
  // Solo insumos de este tipo (por ejemplo, el insumo preparado de una almacenable).
  kind?: ItemKind;
  required?: boolean;
  emptyLabel?: string;
}) {
  const [search, setSearch] = useState('');
  const extra: Record<string, string> = { active: 'true' };
  if (kind) {
    extra.kind = kind;
  }
  const results = useResource<ItemPage>('inventory_items', {
    query: searchQuery(search, extra),
    reloadEvent: inventoryChangeEvent,
  });
  const found: ItemReference[] = (results.data?.items ?? []).map((item) => ({
    id: item.id,
    code: item.code,
    name: item.name,
    kind: item.kind,
    active: item.active,
    unit: item.base_unit,
  }));
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

export function PreparationPicker({
  label,
  value,
  onChange,
  excludeID,
  required = false,
}: {
  label: string;
  value: PreparationChoice | null;
  onChange: (preparation: PreparationChoice | null) => void;
  // La preparación que se edita no puede usarse a sí misma.
  excludeID?: string;
  required?: boolean;
}) {
  const [search, setSearch] = useState('');
  const results = useResource<RecipePage>('inventory_recipes', {
    query: searchQuery(search, { owner_kind: 'preparation', active: 'true' }),
    reloadEvent: inventoryChangeEvent,
  });
  const found = (results.data?.items ?? [])
    .filter((recipe) => recipe.id !== excludeID)
    .map((recipe) => ({
      choice: {
        id: recipe.id,
        name: recipe.name,
        dimension: recipe.yield_unit.dimension,
        unitID: recipe.yield_unit.id,
      },
      mode: recipe.preparation_mode_label ?? '',
    }));
  const options =
    value && !found.some((entry) => entry.choice.id === value.id)
      ? [{ choice: value, mode: '' }, ...found]
      : found;
  return (
    <div className="inventory-picker">
      <Field label={label}>
        <Select
          value={value?.id ?? ''}
          required={required}
          placeholder="Elige una preparación"
          options={[
            ...(required ? [] : [{ value: '', label: 'Elige una preparación' }]),
            ...options.map((entry) => ({
              value: entry.choice.id,
              label: `${entry.choice.name}${entry.mode ? ` · ${entry.mode}` : ''}`,
            })),
          ]}
          searchLabel={`Buscar ${label.toLowerCase()}`}
          searchPlaceholder="Nombre de la preparación"
          onSearchChange={setSearch}
          loading={results.loading}
          onChange={(id) => {
            const selected = options.find((entry) => entry.choice.id === id);
            onChange(selected?.choice ?? null);
          }}
        />
      </Field>
      {results.error && <small className="muted">{results.error}</small>}
      {results.data && results.data.total === 0 && search.trim() === '' && (
        <small className="muted">
          Aún no hay preparaciones activas. Se crean en el apartado «Preparaciones».
        </small>
      )}
    </div>
  );
}

// Ingrediente del catálogo cuya porción sobrescribe el renglón. Primero los ingredientes del
// producto (cuando la receta ya existe) y después los demás del catálogo que coincidan.
export function CatalogIngredientPicker({
  value,
  onChange,
  productIngredients,
  required = false,
}: {
  value: IngredientChoice | null;
  onChange: (ingredient: IngredientChoice | null) => void;
  productIngredients: IngredientChoice[];
  required?: boolean;
}) {
  const [search, setSearch] = useState('');
  const results = useResource<IngredientPortionPage>('inventory_ingredient_portions', {
    query: searchQuery(search, { active: 'true' }),
    reloadEvent: inventoryChangeEvent,
  });
  const own = new Set(productIngredients.map((ingredient) => ingredient.id));
  const others: IngredientChoice[] = (results.data?.items ?? [])
    .filter((portion) => !own.has(portion.ingredient_id))
    .map((portion) => ({ id: portion.ingredient_id, name: portion.ingredient_name }));
  const known = [...productIngredients, ...others];
  const extra = value && !known.some((ingredient) => ingredient.id === value.id) ? [value] : [];
  const options = [...extra, ...known];
  const toOption = (ingredient: IngredientChoice) => ({
    value: ingredient.id,
    label: ingredient.name,
  });
  const items: SelectItem[] = [];
  if (!required) {
    items.push({ value: '', label: 'Elige un ingrediente' });
  }
  items.push(...extra.map(toOption));
  if (productIngredients.length > 0) {
    items.push({ label: 'Ingredientes del producto', options: productIngredients.map(toOption) });
  }
  if (others.length > 0) {
    items.push({ label: 'Otros ingredientes del catálogo', options: others.map(toOption) });
  }
  return (
    <div className="inventory-picker">
      <Field label="Ingrediente del catálogo">
        <Select
          value={value?.id ?? ''}
          required={required}
          placeholder="Elige un ingrediente"
          options={items}
          searchLabel="Buscar ingrediente del catálogo"
          searchPlaceholder="Nombre del ingrediente"
          onSearchChange={setSearch}
          loading={results.loading}
          onChange={(id) => onChange(options.find((ingredient) => ingredient.id === id) ?? null)}
        />
      </Field>
      {results.error && <small className="muted">{results.error}</small>}
    </div>
  );
}

// Unidades activas de la dimensión que indica Go (la de la unidad base del insumo o del
// rendimiento de la preparación), más la elegida aunque esté desactivada. Sin dimensión (todavía
// no se elige el componente) el control queda deshabilitado.
export function DimensionUnitSelect({
  label,
  units,
  dimension,
  value,
  onChange,
  hint,
  disabled = false,
}: {
  label: string;
  units: InventoryUnit[];
  dimension: Dimension | null;
  value: string;
  onChange: (unitID: string) => void;
  hint?: string;
  disabled?: boolean;
}) {
  const offered = dimension
    ? units.filter((unit) => unit.dimension === dimension && (unit.active || unit.id === value))
    : [];
  return (
    <Field label={label} hint={hint}>
      <Select
        required
        value={value}
        disabled={disabled || dimension === null}
        placeholder={dimension === null ? 'Elige primero el componente' : 'Elige la unidad'}
        options={offered.map((unit) => ({ value: unit.id, label: `${unit.name} (${unit.code})` }))}
        onChange={onChange}
      />
    </Field>
  );
}
