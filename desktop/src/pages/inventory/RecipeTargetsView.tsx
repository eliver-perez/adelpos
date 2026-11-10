import { useState } from 'react';
import { Field, Notice } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import { recipeTargetStatusLabels } from '../../services/inventoryRecipes';
import type {
  RecipeTarget,
  RecipeTargetKind,
  RecipeTargetPage,
  RecipeTargetStatus,
} from '../../services/inventoryRecipes';
import { pluralWord } from '../../services/plural';
import { Pagination } from './InventoryFields';
import { targetKindLabels } from './recipeFormat';
import type { RecipeDraftTarget } from './RecipeEditor';

const pageSize = 25;

interface TargetFilter {
  search: string;
  kind: RecipeTargetKind | '';
  state: RecipeTargetStatus | '';
  active: string;
  page: number;
}

function targetQuery(filter: TargetFilter) {
  const query: Record<string, string> = {
    page: String(filter.page),
    page_size: String(pageSize),
  };
  if (filter.search.trim() !== '') {
    query.search = filter.search.trim();
  }
  if (filter.kind !== '') {
    query.kind = filter.kind;
  }
  if (filter.state !== '') {
    query.state = filter.state;
  }
  if (filter.active !== '') {
    query.active = filter.active;
  }
  return query;
}

export function targetName(target: RecipeTarget) {
  return target.variant_name ? `${target.name} · ${target.variant_name}` : target.name;
}

export function draftTargetFor(target: RecipeTarget): RecipeDraftTarget {
  return {
    ownerKind: target.kind === 'complement' ? 'complement' : 'product',
    productID: target.product_id,
    variantID: target.variant_id,
    complementID: target.complement_id,
    preparationMode: null,
    title: targetName(target),
  };
}

const statusTagClass: Record<RecipeTargetStatus, string> = {
  with_recipe: 'tag tag-good',
  without_recipe: 'tag tag-warning',
  no_preparation: 'tag',
};

// Qué consume el destino además (o en lugar) de su receta: porciones de sus ingredientes del
// catálogo o el producto o ingrediente vinculado del complemento. Solo describe lo que entrega Go.
function configurationDetail(target: RecipeTarget) {
  const parts: string[] = [];
  if (target.kind !== 'complement' && target.ingredients_total > 0) {
    const noun = pluralWord(target.ingredients_total, 'ingrediente', 'ingredientes');
    parts.push(
      `${target.ingredients_with_portion} de ${target.ingredients_total} ${noun} con porción`,
    );
  }
  if (target.linked_product_name) {
    parts.push(`Vinculado al producto «${target.linked_product_name}»`);
  }
  if (target.linked_ingredient_name) {
    parts.push(`Vinculado al ingrediente «${target.linked_ingredient_name}»`);
  }
  if (target.kind === 'variant' && !target.recipe_id && target.inherited_recipe_id) {
    parts.push('Usa la receta del producto');
  }
  if (target.status !== 'with_recipe') {
    parts.push(
      target.configured ? 'Consume por porciones o vínculo' : 'No consume inventario todavía',
    );
  }
  return parts;
}

function TargetFilters({
  filter,
  onChange,
}: {
  filter: TargetFilter;
  onChange: (filter: TargetFilter) => void;
}) {
  return (
    <section className="panel catalog-filters" aria-label="Filtros de destinos de receta">
      <Field label="Buscar producto o complemento">
        <input
          type="search"
          maxLength={100}
          value={filter.search}
          placeholder="Nombre, variante o categoría"
          onChange={(event) => onChange({ ...filter, search: event.target.value, page: 1 })}
        />
      </Field>
      <Field label="Tipo de destino">
        <select
          value={filter.kind}
          onChange={(event) =>
            onChange({ ...filter, kind: event.target.value as RecipeTargetKind | '', page: 1 })
          }
        >
          <option value="">Todos</option>
          <option value="product">Productos</option>
          <option value="variant">Variantes</option>
          <option value="complement">Complementos</option>
        </select>
      </Field>
      <Field label="Estado de la receta">
        <select
          value={filter.state}
          onChange={(event) =>
            onChange({ ...filter, state: event.target.value as RecipeTargetStatus | '', page: 1 })
          }
        >
          <option value="">Todos</option>
          <option value="without_recipe">Sin receta</option>
          <option value="with_recipe">Con receta</option>
          <option value="no_preparation">Sin preparación</option>
        </select>
      </Field>
      <Field label="Disponibilidad en el catálogo">
        <select
          value={filter.active}
          onChange={(event) => onChange({ ...filter, active: event.target.value, page: 1 })}
        >
          <option value="true">Activos</option>
          <option value="false">Inactivos</option>
          <option value="">Todos</option>
        </select>
      </Field>
    </section>
  );
}

// Apartado «Productos y complementos» de la pestaña Recetas: los destinos del catálogo con su
// estado de receta, para configurar lo pendiente con el filtro «Sin receta».
export function RecipeTargetsView({
  editable,
  onOpenRecipe,
  onCreateRecipe,
}: {
  editable: boolean;
  onOpenRecipe: (recipeID: string) => void;
  onCreateRecipe: (target: RecipeDraftTarget) => void;
}) {
  const [filter, setFilter] = useState<TargetFilter>({
    search: '',
    kind: '',
    state: '',
    active: 'true',
    page: 1,
  });
  const list = useResource<RecipeTargetPage>('inventory_recipe_targets', {
    query: targetQuery(filter),
    reloadEvent: inventoryChangeEvent,
  });
  const page = list.data;
  return (
    <div className="form-stack">
      <p className="muted text-sm">
        Productos, variantes y complementos del catálogo. Los productos sin receta se siguen
        vendiendo; solo consumen lo que tengan configurado por porciones.
      </p>
      <TargetFilters filter={filter} onChange={setFilter} />
      <Notice>{list.error}</Notice>
      {!page && !list.error && <p className="muted">Consultando destinos de receta…</p>}
      {page && (
        <section className="panel">
          <div className="table-container cash-table">
            <table className="inventory-table">
              <thead>
                <tr>
                  <th scope="col">Destino</th>
                  <th scope="col">Tipo</th>
                  <th scope="col">Estado</th>
                  <th scope="col">Configuración</th>
                  <th scope="col">Receta</th>
                  <th scope="col">
                    <span className="sr-only">Acciones</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {page.items.map((target) => (
                  <TargetRow
                    key={`${target.kind}:${target.product_id ?? ''}:${target.variant_id ?? ''}:${target.complement_id ?? ''}`}
                    target={target}
                    editable={editable}
                    onOpenRecipe={onOpenRecipe}
                    onCreateRecipe={onCreateRecipe}
                  />
                ))}
                {page.items.length === 0 && (
                  <tr>
                    <td colSpan={6} className="muted">
                      No hay productos ni complementos con estos filtros.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <Pagination
            page={page.page}
            pageSize={page.page_size}
            total={page.total}
            singular="destino"
            plural="destinos"
            onPage={(next) => setFilter({ ...filter, page: next })}
          />
        </section>
      )}
    </div>
  );
}

function TargetRow({
  target,
  editable,
  onOpenRecipe,
  onCreateRecipe,
}: {
  target: RecipeTarget;
  editable: boolean;
  onOpenRecipe: (recipeID: string) => void;
  onCreateRecipe: (target: RecipeDraftTarget) => void;
}) {
  const name = targetName(target);
  const details = configurationDetail(target);
  const ownRecipe = target.recipe_id;
  const inheritedRecipe = target.inherited_recipe_id;
  return (
    <tr>
      <td className="inventory-cell-name">
        <strong>{name}</strong>
        <span className="muted text-xs inventory-cell-detail">
          {target.category_name ?? 'Complemento del catálogo'}
          {target.active ? '' : ' · inactivo'}
        </span>
      </td>
      <td>
        <div className="inventory-tags">
          <span className="tag">{targetKindLabels[target.kind]}</span>
          {target.product_kind === 'combo' && <span className="tag">Combo</span>}
        </div>
      </td>
      <td>
        <span className={statusTagClass[target.status]}>
          {recipeTargetStatusLabels[target.status]}
        </span>
      </td>
      <td>
        {details.length === 0 ? (
          '—'
        ) : (
          <span className="inventory-cell-stack text-xs">
            {details.map((detail) => (
              <span key={detail}>{detail}</span>
            ))}
          </span>
        )}
      </td>
      <td>
        {target.recipe_id ? (
          <>
            {target.recipe_name}
            {target.recipe_active === false && (
              <span className="tag inventory-inline-tag">Inactiva</span>
            )}
          </>
        ) : (
          '—'
        )}
      </td>
      <td>
        <div className="button-row recipe-row-actions">
          {ownRecipe && (
            <button
              type="button"
              className="secondary-button"
              aria-label={`${editable ? 'Editar' : 'Ver'} la receta de ${name}`}
              onClick={() => onOpenRecipe(ownRecipe)}
            >
              {editable ? 'Editar receta' : 'Ver receta'}
            </button>
          )}
          {!ownRecipe && inheritedRecipe && (
            <button
              type="button"
              className="secondary-button"
              aria-label={`Ver la receta del producto de ${name}`}
              onClick={() => onOpenRecipe(inheritedRecipe)}
            >
              Ver receta del producto
            </button>
          )}
          {!ownRecipe && editable && (
            <button
              type="button"
              className="primary-button"
              aria-label={`Configurar la receta de ${name}`}
              onClick={() => onCreateRecipe(draftTargetFor(target))}
            >
              Configurar receta
            </button>
          )}
        </div>
      </td>
    </tr>
  );
}
