import { useState } from 'react';
import { Field, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { PreparationMode, RecipePage, RecipeSummary } from '../../services/inventoryRecipes';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { formatQuantity } from './inventoryFormat';
import type { RecipeDraftTarget } from './RecipeEditor';
import { Select } from '../../components/Select';

// Una sola página de hasta 100 preparaciones (el máximo de Go) se reparte entre las dos
// modalidades; con más, la búsqueda acota el listado.
const pageSize = 100;

const modeSections: {
  mode: PreparationMode;
  title: string;
  description: string;
  create: string;
}[] = [
  {
    mode: 'immediate',
    title: 'Elaboración inmediata',
    description:
      'Se preparan al momento: cada vez que una receta las usa se descuentan directamente sus insumos de origen.',
    create: 'Nueva preparación inmediata',
  },
  {
    mode: 'stored',
    title: 'Preparadas previamente (almacenables)',
    description:
      'Se elaboran por lotes y se guardan como existencia de un insumo preparado. Al venderse se consume ese insumo, nunca otra vez sus ingredientes.',
    create: 'Nueva preparación almacenable',
  },
];

export function newPreparationTarget(mode: PreparationMode): RecipeDraftTarget {
  return {
    ownerKind: 'preparation',
    productID: null,
    variantID: null,
    complementID: null,
    preparationMode: mode,
    title: mode === 'immediate' ? 'Nueva preparación inmediata' : 'Nueva preparación almacenable',
  };
}

function PreparationTable({
  recipes,
  editable,
  timezone,
  onOpen,
}: {
  recipes: RecipeSummary[];
  editable: boolean;
  timezone: string;
  onOpen: (recipeID: string) => void;
}) {
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <thead>
          <tr>
            <th scope="col">Preparación</th>
            <th scope="col" className="inventory-number">
              Rendimiento por lote
            </th>
            <th scope="col">Componentes</th>
            <th scope="col">Versión vigente</th>
            <th scope="col">Estado</th>
            <th scope="col">
              <span className="sr-only">Acciones</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {recipes.map((recipe) => (
            <tr key={recipe.id}>
              <td className="inventory-cell-name">
                <strong>{recipe.name}</strong>
              </td>
              <td className="inventory-number">
                {formatQuantity(recipe.yield_quantity, recipe.yield_unit)}
              </td>
              <td>{countLabel(recipe.line_count, 'componente', 'componentes')}</td>
              <td>
                Versión {recipe.revision}
                <span className="muted text-xs inventory-cell-detail">
                  Desde el {serverDateTime(recipe.effective_from, timezone)}
                  {recipe.updated_by_name ? ` · ${recipe.updated_by_name}` : ''}
                </span>
              </td>
              <td>
                <span className={recipe.active ? 'tag tag-good' : 'tag'}>
                  {recipe.active ? 'Activa' : 'Inactiva'}
                </span>
              </td>
              <td>
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`${editable ? 'Editar' : 'Ver'} la preparación ${recipe.name}`}
                  onClick={() => onOpen(recipe.id)}
                >
                  {editable ? 'Editar' : 'Ver'}
                </button>
              </td>
            </tr>
          ))}
          {recipes.length === 0 && (
            <tr>
              <td colSpan={6} className="muted">
                No hay preparaciones de esta modalidad con estos filtros.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

// Apartado «Preparaciones» (subrecetas): las de elaboración inmediata y las almacenables, en
// secciones separadas. Se editan con inventory.subrecipes.manage.
export function PreparationsView({
  editable,
  timezone,
  onOpenRecipe,
  onCreateRecipe,
}: {
  editable: boolean;
  timezone: string;
  onOpenRecipe: (recipeID: string) => void;
  onCreateRecipe: (target: RecipeDraftTarget) => void;
}) {
  const [search, setSearch] = useState('');
  const [active, setActive] = useState('');
  const query: Record<string, string> = { owner_kind: 'preparation', page_size: String(pageSize) };
  if (search.trim() !== '') {
    query.search = search.trim();
  }
  if (active !== '') {
    query.active = active;
  }
  const list = useResource<RecipePage>('inventory_recipes', {
    query,
    reloadEvent: inventoryChangeEvent,
  });
  const recipes = list.data?.items ?? [];
  return (
    <div className="form-stack">
      <p className="muted text-sm">
        Subrecetas reutilizables (salsas, aderezos, arroz preparado…). No se pueden formar ciclos:
        una preparación no puede usarse a sí misma, ni a través del insumo que produce.
      </p>
      <section className="panel catalog-filters" aria-label="Filtros de preparaciones">
        <Field label="Buscar preparación">
          <input
            type="search"
            maxLength={100}
            value={search}
            placeholder="Nombre de la preparación"
            onChange={(event) => setSearch(event.target.value)}
          />
        </Field>
        <Field label="Estado de la preparación">
          <Select
            value={active}
            options={[
              { value: '', label: 'Todas' },
              { value: 'true', label: 'Activas' },
              { value: 'false', label: 'Inactivas' },
            ]}
            onChange={setActive}
          />
        </Field>
      </section>
      <Notice>{list.error}</Notice>
      {!list.data && !list.error && <p className="muted">Consultando preparaciones…</p>}
      {list.data && list.data.total > pageSize && (
        <Notice>
          Se muestran las primeras {pageSize} de {list.data.total} preparaciones: usa la búsqueda
          para encontrar las demás.
        </Notice>
      )}
      {list.data &&
        modeSections.map((section) => {
          const members = recipes.filter((recipe) => recipe.preparation_mode === section.mode);
          return (
            <section
              key={section.mode}
              className="panel form-stack"
              aria-labelledby={`preparations-${section.mode}-title`}
            >
              <div className="section-heading">
                <div>
                  <h2 id={`preparations-${section.mode}-title`}>{section.title}</h2>
                  <p className="muted text-xs">{section.description}</p>
                </div>
                {editable && (
                  <button
                    type="button"
                    className="secondary-button"
                    onClick={() => onCreateRecipe(newPreparationTarget(section.mode))}
                  >
                    <IconLabel icon="plus">{section.create}</IconLabel>
                  </button>
                )}
              </div>
              <PreparationTable
                recipes={members}
                editable={editable}
                timezone={timezone}
                onOpen={onOpenRecipe}
              />
            </section>
          );
        })}
    </div>
  );
}
