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

// Selectores de componentes de receta: insumos, preparaciones e ingredientes del catálogo, con
// búsqueda en Go (hasta 50 coincidencias). El elemento elegido se conserva aunque deje de aparecer
// en una búsqueda nueva. Ninguno muestra identificadores: solo nombres y códigos internos.

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
      <Field label={`Buscar ${label.toLowerCase()}`}>
        <input
          type="search"
          maxLength={100}
          autoComplete="off"
          value={search}
          placeholder="Nombre, código o código de barras"
          onChange={(event) => setSearch(event.target.value)}
        />
      </Field>
      <Field label={label}>
        <select
          value={value?.id ?? ''}
          required={required}
          onChange={(event) => {
            onChange(options.find((item) => item.id === event.target.value) ?? null);
          }}
        >
          <option value="">{emptyLabel}</option>
          {options.map((item) => (
            <option key={item.id} value={item.id}>
              {item.name} · {item.code}
              {item.active ? '' : ' (inactivo)'}
            </option>
          ))}
        </select>
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
      <Field label={`Buscar ${label.toLowerCase()}`}>
        <input
          type="search"
          maxLength={100}
          autoComplete="off"
          value={search}
          placeholder="Nombre de la preparación"
          onChange={(event) => setSearch(event.target.value)}
        />
      </Field>
      <Field label={label}>
        <select
          value={value?.id ?? ''}
          required={required}
          onChange={(event) => {
            const selected = options.find((entry) => entry.choice.id === event.target.value);
            onChange(selected?.choice ?? null);
          }}
        >
          <option value="">Elige una preparación</option>
          {options.map((entry) => (
            <option key={entry.choice.id} value={entry.choice.id}>
              {entry.choice.name}
              {entry.mode ? ` · ${entry.mode}` : ''}
            </option>
          ))}
        </select>
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
  return (
    <div className="inventory-picker">
      <Field label="Buscar ingrediente del catálogo">
        <input
          type="search"
          maxLength={100}
          autoComplete="off"
          value={search}
          placeholder="Nombre del ingrediente"
          onChange={(event) => setSearch(event.target.value)}
        />
      </Field>
      <Field label="Ingrediente del catálogo">
        <select
          value={value?.id ?? ''}
          required={required}
          onChange={(event) => {
            onChange(options.find((ingredient) => ingredient.id === event.target.value) ?? null);
          }}
        >
          <option value="">Elige un ingrediente</option>
          {extra.map((ingredient) => (
            <option key={ingredient.id} value={ingredient.id}>
              {ingredient.name}
            </option>
          ))}
          {productIngredients.length > 0 && (
            <optgroup label="Ingredientes del producto">
              {productIngredients.map((ingredient) => (
                <option key={ingredient.id} value={ingredient.id}>
                  {ingredient.name}
                </option>
              ))}
            </optgroup>
          )}
          {others.length > 0 && (
            <optgroup label="Otros ingredientes del catálogo">
              {others.map((ingredient) => (
                <option key={ingredient.id} value={ingredient.id}>
                  {ingredient.name}
                </option>
              ))}
            </optgroup>
          )}
        </select>
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
      <select
        required
        value={value}
        disabled={disabled || dimension === null}
        onChange={(event) => onChange(event.target.value)}
      >
        {dimension === null && <option value="">Elige primero el componente</option>}
        {dimension !== null && value === '' && <option value="">Elige la unidad</option>}
        {offered.map((unit) => (
          <option key={unit.id} value={unit.id}>
            {unit.name} ({unit.code})
          </option>
        ))}
      </select>
    </Field>
  );
}
