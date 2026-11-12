import { useEffect, useRef, useState } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Form, Notice } from '../../components/Forms';
import { ToggleButton } from '../../components/ToggleButton';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, InventoryUnit } from '../../services/inventory';
import {
  createRecipe,
  recipeOwnerKindLabels,
  saveRecipeVersion,
} from '../../services/inventoryRecipes';
import type {
  ItemReference,
  PreparationMode,
  Recipe,
  RecipeCreateInput,
  RecipeOwnerKind,
  RecipeVersionInput,
} from '../../services/inventoryRecipes';
import { formatMoney } from '../../services/money';
import { inventoryQuantityShape } from '../../services/numericInput';
import type { DecimalShape } from '../../services/numericInput';
import { serverDateTime } from '../../services/restaurant';
import { DecimalTextField } from './InventoryFields';
import { formatQuantity, inventoryPermissionName } from './inventoryFormat';
import { portionSourceLabels, recipeBaseUnit } from './recipeFormat';
import { DimensionUnitSelect, RecipeItemPicker } from './RecipePickers';
import { lineDraftFromRecipe, lineInput, RecipeLinesEditor } from './RecipeLinesEditor';
import type { LineDraft } from './RecipeLinesEditor';
import { RecipeVersionHistory } from './RecipeVersionHistory';
import { Select } from '../../components/Select';

// Merma de elaboración: porcentaje de 0 a menos de 100 con hasta cuatro decimales (límite de Go).
const wasteShape: DecimalShape = { decimals: 4, integerDigits: 2 };

// Destino de una receta nueva: producto (con variante opcional), complemento o preparación con su
// modalidad. Se fija al crearla y Go no permite cambiarlo después.
export interface RecipeDraftTarget {
  ownerKind: RecipeOwnerKind;
  productID: string | null;
  variantID: string | null;
  complementID: string | null;
  preparationMode: PreparationMode | null;
  // Nombre legible del destino («Filete · Grande», «Arroz»…).
  title: string;
}

interface RecipeFormState {
  name: string;
  active: boolean;
  yieldQuantity: string;
  yieldUnitID: string;
  waste: string;
  notes: string;
  // Insumo que produce una preparación almacenable (solo al crearla).
  preparationItem: ItemReference | null;
  lines: LineDraft[];
}

// Permiso de escritura según el tipo: recetas (producto y complemento) o subrecetas.
export function recipeManagePermission(ownerKind: RecipeOwnerKind) {
  return ownerKind === 'preparation' ? 'inventory.subrecipes.manage' : 'inventory.recipes.manage';
}

function initialRecipeForm(recipe: Recipe | null): RecipeFormState {
  if (!recipe) {
    return {
      name: '',
      active: true,
      yieldQuantity: '1',
      yieldUnitID: '',
      waste: '',
      notes: '',
      preparationItem: null,
      lines: [],
    };
  }
  return {
    name: recipe.name,
    active: recipe.active,
    yieldQuantity: recipe.version.yield_quantity,
    yieldUnitID: recipe.version.yield_unit.id,
    waste: recipe.version.production_waste_percent,
    notes: recipe.version.notes,
    preparationItem: recipe.preparation_item,
    lines: recipe.version.lines.map((line, index) => lineDraftFromRecipe(line, index + 1)),
  };
}

// Campos comunes de la versión (alta o versión nueva). En productos y complementos el rendimiento
// es en porciones y Go pone la unidad «porción»; la merma solo existe en preparaciones.
function versionFields(form: RecipeFormState, ownerKind: RecipeOwnerKind) {
  const fields: Omit<RecipeVersionInput, 'revision'> = {
    name: form.name,
    active: form.active,
    yield_quantity: form.yieldQuantity,
    notes: form.notes,
    lines: form.lines.map((line) => lineInput(line, ownerKind)),
  };
  if (ownerKind === 'preparation') {
    fields.yield_unit_id = form.yieldUnitID === '' ? null : form.yieldUnitID;
    fields.production_waste_percent = form.waste;
  }
  return fields;
}

export function recipeCreateInput(
  form: RecipeFormState,
  target: RecipeDraftTarget,
): RecipeCreateInput {
  const input: RecipeCreateInput = {
    owner_kind: target.ownerKind,
    ...versionFields(form, target.ownerKind),
  };
  if (target.ownerKind === 'product') {
    input.product_id = target.productID;
    input.variant_id = target.variantID;
  }
  if (target.ownerKind === 'complement') {
    input.complement_id = target.complementID;
  }
  if (target.ownerKind === 'preparation') {
    input.preparation_mode = target.preparationMode;
    if (target.preparationMode === 'stored') {
      input.preparation_item_id = form.preparationItem?.id ?? null;
    }
  }
  return input;
}

export function recipeVersionInput(form: RecipeFormState, recipe: Recipe): RecipeVersionInput {
  return { revision: recipe.revision, ...versionFields(form, recipe.owner_kind) };
}

// Interruptor «Receta activa». Activar y desactivar se confirman (desactivar con tono de peligro)
// porque, al guardar, la versión nueva cambia lo que consumen las liberaciones siguientes.
function RecipeActiveToggle({
  pressed,
  onChange,
  savedActive,
  nextVersion,
  name,
}: {
  pressed: boolean;
  onChange: (active: boolean) => void;
  // Estado de la versión vigente (null en una receta nueva: no se confirma nada).
  savedActive: boolean | null;
  nextVersion: number;
  name: string;
}) {
  const { confirm, dialog } = useConfirmDialog();

  async function change(next: boolean) {
    if (savedActive !== null && next !== savedActive) {
      const confirmed = await confirm(
        next
          ? {
              title: 'Activar receta',
              message: `Al guardar, la versión ${nextVersion} activa «${name}»: las liberaciones a cocina siguientes consumirán según esta receta.`,
              confirmLabel: 'Activar',
              cancelLabel: 'Mantener inactiva',
            }
          : {
              title: 'Desactivar receta',
              message: `Al guardar, la versión ${nextVersion} deja «${name}» inactiva: las liberaciones siguientes ya no consumirán con esta receta. Las versiones anteriores y sus consumos se conservan.`,
              confirmLabel: 'Desactivar',
              cancelLabel: 'Mantener activa',
              tone: 'danger',
            },
      );
      if (!confirmed) {
        return;
      }
    }
    onChange(next);
  }

  return (
    <>
      <ToggleButton
        label="Receta activa"
        pressed={pressed}
        onChange={(next) => void change(next)}
      />
      {dialog}
    </>
  );
}

// Unidad del rendimiento de una preparación nueva de elaboración inmediata: cualquier unidad activa,
// agrupada por dimensión. Después de crearla, la dimensión ya no cambia (Go: YIELD_DIMENSION_LOCKED).
function AnyUnitSelect({
  units,
  value,
  onChange,
}: {
  units: InventoryUnit[];
  value: string;
  onChange: (unitID: string) => void;
}) {
  const offered = units.filter((unit) => unit.active);
  const dimensions = Array.from(new Set(offered.map((unit) => unit.dimension)));
  return (
    <Field
      label="Unidad del rendimiento"
      hint="Define en qué se mide la preparación (masa, volumen, piezas o porciones); no cambia después."
    >
      <Select
        required
        value={value}
        placeholder="Elige la unidad"
        options={dimensions.map((dimension) => {
          const members = offered.filter((unit) => unit.dimension === dimension);
          return {
            label: members[0]?.dimension_label ?? dimension,
            options: members.map((unit) => ({
              value: unit.id,
              label: `${unit.name} (${unit.code})`,
            })),
          };
        })}
        onChange={onChange}
      />
    </Field>
  );
}

function YieldFields({
  form,
  recipe,
  ownerKind,
  preparationMode,
  units,
  update,
}: {
  form: RecipeFormState;
  recipe: Recipe | null;
  ownerKind: RecipeOwnerKind;
  preparationMode: PreparationMode | null;
  units: InventoryUnit[];
  update: <K extends keyof RecipeFormState>(key: K, value: RecipeFormState[K]) => void;
}) {
  if (ownerKind !== 'preparation') {
    return (
      <DecimalTextField
        label="Rendimiento (porciones)"
        value={form.yieldQuantity}
        onChange={(value) => update('yieldQuantity', value)}
        shape={inventoryQuantityShape}
        required
        hint="Porciones vendidas que salen de esta receta; cada una consume los componentes entre el rendimiento. Normalmente 1."
      />
    );
  }
  let unitControl;
  if (preparationMode === 'stored') {
    const dimension = form.preparationItem?.unit.dimension ?? null;
    unitControl = (
      <DimensionUnitSelect
        label="Unidad del rendimiento"
        units={units}
        dimension={dimension}
        value={form.yieldUnitID}
        onChange={(value) => update('yieldUnitID', value)}
        hint="En la dimensión de la unidad base del insumo preparado."
      />
    );
  } else if (recipe) {
    unitControl = (
      <DimensionUnitSelect
        label="Unidad del rendimiento"
        units={units}
        dimension={recipe.version.yield_unit.dimension}
        value={form.yieldUnitID}
        onChange={(value) => update('yieldUnitID', value)}
        hint="La dimensión del rendimiento no cambia: las recetas que usan la preparación están en ella."
      />
    );
  } else {
    unitControl = (
      <AnyUnitSelect
        units={units}
        value={form.yieldUnitID}
        onChange={(value) => update('yieldUnitID', value)}
      />
    );
  }
  return (
    <>
      <DecimalTextField
        label="Rendimiento por lote"
        value={form.yieldQuantity}
        onChange={(value) => update('yieldQuantity', value)}
        shape={inventoryQuantityShape}
        required
        hint="Cantidad que produce un lote con los componentes de abajo."
      />
      {unitControl}
      <DecimalTextField
        label="Merma de elaboración (%)"
        value={form.waste}
        onChange={(value) => update('waste', value)}
        shape={wasteShape}
        hint="De 0 a menos de 100. Descuenta el rendimiento efectivo de cada lote. Vacío: sin merma."
      />
    </>
  );
}

function RecipeForm({
  recipe,
  target,
  units,
  editable,
  execute,
  busy,
  onSaved,
  onDirtyChange,
}: {
  recipe: Recipe | null;
  target: RecipeDraftTarget;
  units: InventoryUnit[];
  editable: boolean;
  execute: InventoryExecute;
  busy: boolean;
  onSaved: (recipe: Recipe) => void;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const [initial] = useState(() => initialRecipeForm(recipe));
  const [form, setForm] = useState(initial);
  const ownerKind = recipe?.owner_kind ?? target.ownerKind;
  const preparationMode = recipe?.preparation_mode ?? target.preparationMode;
  const dirty = JSON.stringify(form) !== JSON.stringify(initial);
  const nextVersion = recipe ? recipe.revision + 1 : 1;
  const productIngredients = (recipe?.ingredients ?? []).map((ingredient) => ({
    id: ingredient.ingredient_id,
    name: ingredient.name,
  }));

  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty, onDirtyChange]);

  function update<K extends keyof RecipeFormState>(key: K, value: RecipeFormState[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  const nameRequired = ownerKind === 'preparation' && preparationMode === 'immediate';
  let nameHint = 'Vacío: el nombre del producto, variante o complemento.';
  if (ownerKind === 'preparation') {
    nameHint = nameRequired
      ? 'Por ejemplo «Salsa de la casa». Único entre las preparaciones.'
      : 'Vacío: el nombre del insumo preparado. Único entre las preparaciones.';
  }

  return (
    <Form
      label={recipe ? `Guardar como versión ${nextVersion}` : 'Crear receta (versión 1)'}
      successMessage={
        recipe ? `Receta guardada como versión ${nextVersion}.` : 'Receta creada como versión 1.'
      }
      disabled={busy || !editable}
      onSave={async () => {
        const saved = recipe
          ? await saveRecipeVersion(execute, recipe.id, recipeVersionInput(form, recipe))
          : await createRecipe(execute, recipeCreateInput(form, target));
        onDirtyChange(false);
        onSaved(saved);
      }}
    >
      <p className="recipe-version-note text-sm">
        {recipe
          ? `Guardar crea la versión ${nextVersion}, vigente desde ese momento. Las partidas ya liberadas conservan la versión con que se consumieron y las anteriores quedan en el historial.`
          : 'Al guardarla se crea la versión 1. Cada cambio posterior crea una versión nueva sin alterar los consumos ya registrados.'}
      </p>
      {ownerKind === 'preparation' && preparationMode === 'stored' && (
        <div className="form-stack">
          {recipe ? (
            <Field
              label="Insumo preparado que produce"
              hint="Se fija al crear la preparación; al venderse se consume este insumo."
            >
              <input
                readOnly
                value={
                  recipe.preparation_item
                    ? `${recipe.preparation_item.name} · ${recipe.preparation_item.code}`
                    : '—'
                }
              />
            </Field>
          ) : (
            <RecipeItemPicker
              label="Insumo preparado"
              kind="prepared"
              required
              emptyLabel="Elige el insumo que produce"
              value={form.preparationItem}
              onChange={(item) =>
                setForm((current) => ({
                  ...current,
                  preparationItem: item,
                  yieldUnitID: item?.unit.id ?? '',
                }))
              }
            />
          )}
          {!recipe && (
            <p className="muted text-xs">
              El insumo debe ser de tipo «Preparación almacenable» (se elige en Insumos). Su
              existencia aumenta con cada elaboración registrada.
            </p>
          )}
        </div>
      )}
      <div className="form-grid">
        <Field label="Nombre de la receta" hint={nameHint}>
          <input
            maxLength={100}
            required={nameRequired}
            value={form.name}
            placeholder={ownerKind === 'preparation' ? undefined : target.title}
            onChange={(event) => update('name', event.target.value)}
          />
        </Field>
        <YieldFields
          form={form}
          recipe={recipe}
          ownerKind={ownerKind}
          preparationMode={preparationMode}
          units={units}
          update={update}
        />
      </div>
      {recipe && ownerKind === 'preparation' && (
        <p className="muted text-xs">
          Rendimiento efectivo por lote de la versión vigente (descontada la merma):{' '}
          {formatQuantity(recipe.version.effective_yield, recipeBaseUnit(recipe, units))}.
        </p>
      )}
      <RecipeLinesEditor
        lines={form.lines}
        onChange={(lines) => update('lines', lines)}
        ownerKind={ownerKind}
        recipeID={recipe?.id ?? ''}
        units={units}
        productIngredients={productIngredients}
      />
      <Field label="Notas de la versión" hint="Opcional; hasta 1,000 caracteres.">
        <textarea
          maxLength={1000}
          value={form.notes}
          onChange={(event) => update('notes', event.target.value)}
        />
      </Field>
      <div className="button-row">
        <RecipeActiveToggle
          pressed={form.active}
          onChange={(value) => update('active', value)}
          savedActive={recipe ? recipe.active : null}
          nextVersion={nextVersion}
          name={recipe?.name ?? target.title}
        />
        {!form.active && (
          <span className="muted text-xs">
            Inactiva: las liberaciones siguientes no consumen con esta receta.
          </span>
        )}
      </div>
    </Form>
  );
}

// Costo teórico de la versión vigente: solo aparece si Go lo entrega (inventory.costs.read).
function RecipeCostPanel({ recipe, units }: { recipe: Recipe; units: InventoryUnit[] }) {
  const cost = recipe.cost;
  if (!cost.costs_visible) {
    return null;
  }
  const figures: { label: string; value: string }[] = [];
  if (recipe.owner_kind === 'preparation') {
    const base = recipeBaseUnit(recipe, units);
    figures.push({ label: 'Costo de un lote', value: formatMoney(cost.batch_cost_display) });
    figures.push({
      label: base ? `Costo por ${base.name.toLowerCase()} (${base.code})` : 'Costo por unidad base',
      value: formatMoney(cost.yield_unit_cost_display),
    });
  } else {
    figures.push({ label: 'Costo por porción', value: formatMoney(cost.unit_cost_display) });
    if (cost.takeaway_cost_display !== null) {
      figures.push({
        label: 'Adicional para llevar',
        value: formatMoney(cost.takeaway_cost_display),
      });
    }
  }
  return (
    <section className="panel form-stack" aria-labelledby="recipe-cost-title">
      <div>
        <h3 id="recipe-cost-title">Costo teórico de la versión vigente</h3>
        <p className="muted text-xs">
          Calculado por el servidor con el costo vigente de cada insumo. Las opciones de grupo y los
          componentes de combo tienen sus propias recetas y no se suman aquí.
        </p>
      </div>
      <div className="inventory-figures">
        {figures.map((figure) => (
          <div key={figure.label} className="inventory-figure">
            <span className="inventory-figure-label">{figure.label}</span>
            <strong>{figure.value}</strong>
          </div>
        ))}
      </div>
      {cost.cost_pending && (
        <p className="inventory-negative-notice" role="status">
          Algún componente no tiene costo conocido: el costo teórico no lo incluye.
        </p>
      )}
    </section>
  );
}

// Ingredientes del catálogo del producto con la porción que les aplica (renglón de esta receta,
// porción habitual o ninguna). Lo incluido por omisión lo consume cada unidad vendida.
function RecipeIngredientsPanel({ recipe }: { recipe: Recipe }) {
  if (recipe.owner_kind !== 'product' || recipe.ingredients.length === 0) {
    return null;
  }
  const costsVisible = recipe.cost.costs_visible;
  return (
    <section className="panel form-stack" aria-labelledby="recipe-ingredients-title">
      <div>
        <h3 id="recipe-ingredients-title">Ingredientes del catálogo</h3>
        <p className="muted text-xs">
          Cada ingrediente consume las porciones pedidas × su porción: «Sin» no consume, «Normal»
          una porción y «Extra» dos. Sin porción configurada no se inventa ningún consumo.
        </p>
      </div>
      <div className="table-container cash-table">
        <table className="inventory-table">
          <thead>
            <tr>
              <th scope="col">Ingrediente</th>
              <th scope="col" className="inventory-number">
                Incluido
              </th>
              <th scope="col">Porción</th>
              <th scope="col" className="inventory-number">
                Por porción
              </th>
              <th scope="col" className="inventory-number">
                Por unidad vendida
              </th>
              {costsVisible && (
                <th scope="col" className="inventory-number">
                  Costo
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {recipe.ingredients.map((ingredient) => (
              <tr key={ingredient.ingredient_id}>
                <td className="inventory-cell-name">
                  <strong>{ingredient.name}</strong>
                  <span className="muted text-xs inventory-cell-detail">
                    {[
                      ingredient.removable ? 'Se puede quitar' : '',
                      ingredient.addable ? 'Se puede agregar' : '',
                    ]
                      .filter((text) => text !== '')
                      .join(' · ')}
                  </span>
                </td>
                <td className="inventory-number">{formatQuantity(ingredient.included)}</td>
                <td>
                  {portionSourceLabels[ingredient.portion_source]}
                  {(ingredient.item || ingredient.preparation_name) && (
                    <span className="muted text-xs inventory-cell-detail">
                      {ingredient.item?.name ?? ingredient.preparation_name}
                    </span>
                  )}
                </td>
                <td className="inventory-number">
                  {formatQuantity(ingredient.portion_quantity, ingredient.item?.unit)}
                </td>
                <td className="inventory-number">
                  {formatQuantity(ingredient.included_quantity, ingredient.item?.unit)}
                </td>
                {costsVisible && (
                  <td className="inventory-number">
                    {formatMoney(ingredient.cost_display)}
                    {ingredient.cost_pending && (
                      <span className="muted text-xs inventory-cell-detail">Costo pendiente</span>
                    )}
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function RecipeUsesPanel({ recipe }: { recipe: Recipe }) {
  if (recipe.owner_kind !== 'preparation') {
    return null;
  }
  return (
    <section className="panel form-stack" aria-labelledby="recipe-uses-title">
      <h3 id="recipe-uses-title">Recetas que la usan</h3>
      {recipe.used_by.length === 0 ? (
        <p className="muted text-sm">Ninguna receta vigente usa esta preparación.</p>
      ) : (
        <ul className="inventory-list">
          {recipe.used_by.map((use) => (
            <li key={use.id} className="inventory-list-row">
              <div className="inventory-list-text">
                <strong>{use.name}</strong>
                <span className="muted text-xs">{recipeOwnerKindLabels[use.owner_kind]}</span>
              </div>
              {!use.active && <span className="tag">Inactiva</span>}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

// Panel de una receta: alta (con su destino fijo) o versión nueva de una existente, con su costo
// teórico, los ingredientes del catálogo, las recetas que usan la preparación y el historial. Al
// crearla, el panel continúa con la receta guardada. Los cambios de otra terminal recargan la
// receta (por SSE o por un conflicto de revisión, que vuelve a montar el formulario).
export function RecipeEditorPanel({
  initialRecipeID,
  target,
  units,
  can,
  execute,
  busy,
  timezone,
  onClose,
}: {
  initialRecipeID: string;
  // Destino de una receta nueva (initialRecipeID vacío).
  target: RecipeDraftTarget | null;
  units: InventoryUnit[];
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onClose: () => void;
}) {
  const [recipeID, setRecipeID] = useState(initialRecipeID);
  const [snapshot, setSnapshot] = useState<Recipe | null>(null);
  const dirty = useRef(false);
  const { confirm, dialog } = useConfirmDialog();
  const resource = useResource<Recipe>('inventory_recipe', {
    id: recipeID,
    enabled: recipeID !== '',
    reloadEvent: inventoryChangeEvent,
  });

  useEffect(() => {
    if (resource.data) {
      setSnapshot(resource.data);
    }
  }, [resource.data]);

  const recipe = recipeID === '' ? null : snapshot;
  const ownerKind = recipe?.owner_kind ?? target?.ownerKind ?? 'product';
  const permission = recipeManagePermission(ownerKind);
  const editable = can(permission);
  const draftTarget: RecipeDraftTarget = target ?? {
    ownerKind,
    productID: recipe?.product_id ?? null,
    variantID: recipe?.variant_id ?? null,
    complementID: recipe?.complement_id ?? null,
    preparationMode: recipe?.preparation_mode ?? null,
    title: recipe?.target_name ?? '',
  };

  async function close() {
    if (dirty.current) {
      const discard = await confirm({
        title: 'Cambios sin guardar',
        message: 'Esta receta tiene cambios que aún no se guardan. ¿Quieres descartarlos?',
        confirmLabel: 'Descartar cambios',
        cancelLabel: 'Seguir editando',
        tone: 'danger',
      });
      if (!discard) {
        return;
      }
    }
    onClose();
  }

  let subtitle = `${recipeOwnerKindLabels[ownerKind]} · receta nueva`;
  if (recipe) {
    subtitle = `${recipe.owner_kind_label} · ${recipe.target_name} · versión ${recipe.revision} vigente desde el ${serverDateTime(recipe.effective_from, timezone)}`;
  }
  const heading = recipe ? recipe.name : (target?.title ?? 'Receta');

  return (
    <section className="panel form-stack" aria-label={`Receta de ${heading}`}>
      <div className="section-heading">
        <div>
          <h2>{heading}</h2>
          <p className="muted text-xs">{subtitle}</p>
        </div>
        <div className="button-row">
          {recipe && (
            <span className={recipe.active ? 'tag tag-good' : 'tag'}>
              {recipe.active ? 'Activa' : 'Inactiva'}
            </span>
          )}
          <button type="button" className="secondary-button" onClick={() => void close()}>
            Volver al listado
          </button>
        </div>
      </div>
      {!editable && (
        <Notice>
          Consulta disponible. Editar esta receta requiere el permiso «
          {inventoryPermissionName(permission)}».
        </Notice>
      )}
      {recipeID !== '' && !recipe && <Notice>{resource.error || 'Consultando la receta…'}</Notice>}
      {(recipeID === '' || recipe) && (
        <RecipeForm
          key={recipe ? `${recipe.id}:${recipe.revision}` : 'new'}
          recipe={recipe}
          target={draftTarget}
          units={units}
          editable={editable}
          execute={execute}
          busy={busy}
          onDirtyChange={(value) => {
            dirty.current = value;
          }}
          onSaved={(saved) => {
            setSnapshot(saved);
            setRecipeID(saved.id);
          }}
        />
      )}
      {recipe && <RecipeCostPanel recipe={recipe} units={units} />}
      {recipe && <RecipeIngredientsPanel recipe={recipe} />}
      {recipe && <RecipeUsesPanel recipe={recipe} />}
      {recipe && <RecipeVersionHistory recipe={recipe} timezone={timezone} />}
      {dialog}
    </section>
  );
}
