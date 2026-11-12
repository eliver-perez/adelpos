import { useRef } from 'react';
import { IconLabel } from '../../components/Icon';
import { SegmentedControl } from '../../components/SegmentedControl';
import { ToggleButton } from '../../components/ToggleButton';
import type { Dimension, InventoryUnit } from '../../services/inventory';
import type {
  ItemReference,
  RecipeComponentKind,
  RecipeLine,
  RecipeLineInput,
  RecipeOwnerKind,
} from '../../services/inventoryRecipes';
import { formatMoney } from '../../services/money';
import { inventoryQuantityShape } from '../../services/numericInput';
import { DecimalTextField } from './InventoryFields';
import { formatQuantity } from './inventoryFormat';
import {
  CatalogIngredientPicker,
  DimensionUnitSelect,
  PreparationPicker,
  RecipeItemPicker,
} from './RecipePickers';
import type { IngredientChoice, PreparationChoice } from './RecipePickers';

// Renglón capturado de una receta. El tipo decide qué se elige:
//   - item: un insumo (fijo por unidad vendida o por lote);
//   - preparation: una subreceta (se descompone o se consume su insumo preparado);
//   - ingredient: un ingrediente del catálogo del producto con el insumo o la preparación que
//     consume cada porción (sobrescribe la porción habitual solo para este producto).
// La cantidad viaja como se capturó, en la unidad elegida; Go la convierte a la unidad base.
export interface LineDraft {
  key: number;
  kind: RecipeComponentKind;
  item: ItemReference | null;
  preparation: PreparationChoice | null;
  ingredient: IngredientChoice | null;
  // Qué consume una porción del ingrediente del catálogo (solo renglones de ingrediente).
  consumes: 'item' | 'preparation';
  quantity: string;
  unitID: string;
  takeawayOnly: boolean;
  // Renglón tal como lo guardó Go (conversión a la unidad base y costo del renglón).
  saved: RecipeLine | null;
}

export const componentKindLabels: Record<RecipeComponentKind, string> = {
  item: 'Insumo',
  preparation: 'Preparación',
  ingredient: 'Ingrediente del catálogo',
};

// Tipos de renglón que admite cada tipo de receta (Go rechaza los demás).
export function allowedLineKinds(ownerKind: RecipeOwnerKind): RecipeComponentKind[] {
  if (ownerKind === 'product') {
    return ['item', 'preparation', 'ingredient'];
  }
  return ['item', 'preparation'];
}

// «Solo para llevar» aplica a insumos y preparaciones de recetas de producto o complemento.
export function takeawayAllowed(ownerKind: RecipeOwnerKind, kind: RecipeComponentKind) {
  return ownerKind !== 'preparation' && kind !== 'ingredient';
}

export function emptyLineDraft(kind: RecipeComponentKind, key: number): LineDraft {
  return {
    key,
    kind,
    item: null,
    preparation: null,
    ingredient: null,
    consumes: 'item',
    quantity: '',
    unitID: '',
    takeawayOnly: false,
    saved: null,
  };
}

export function lineDraftFromRecipe(line: RecipeLine, key: number): LineDraft {
  const preparation =
    line.preparation_recipe_id === null
      ? null
      : {
          id: line.preparation_recipe_id,
          name: line.preparation_name ?? '',
          dimension: line.base_unit.dimension,
          unitID: line.base_unit.id,
        };
  return {
    key,
    kind: line.component_kind,
    item: line.item,
    preparation,
    ingredient:
      line.ingredient_id === null
        ? null
        : { id: line.ingredient_id, name: line.ingredient_name ?? '' },
    consumes: line.item ? 'item' : 'preparation',
    quantity: line.quantity,
    unitID: line.unit.id,
    takeawayOnly: line.takeaway_only,
    saved: line,
  };
}

// Componente que realmente consume el renglón (el insumo o la preparación).
function consumesItem(line: LineDraft) {
  if (line.kind === 'ingredient') {
    return line.consumes === 'item';
  }
  return line.kind === 'item';
}

// Dimensión de las unidades del renglón: la de la unidad base del insumo o la del rendimiento de
// la preparación, tal como las entrega Go.
export function lineDimension(line: LineDraft): Dimension | null {
  if (consumesItem(line)) {
    return line.item?.unit.dimension ?? null;
  }
  return line.preparation?.dimension ?? null;
}

// Cuerpo del renglón para Go, con la cantidad y la unidad tal como se capturaron.
export function lineInput(line: LineDraft, ownerKind: RecipeOwnerKind): RecipeLineInput {
  const input: RecipeLineInput = {
    component_kind: line.kind,
    quantity: line.quantity,
    unit_id: line.unitID === '' ? null : line.unitID,
  };
  if (line.kind === 'ingredient') {
    input.ingredient_id = line.ingredient?.id ?? null;
  }
  if (consumesItem(line)) {
    input.item_id = line.item?.id ?? null;
  } else {
    input.preparation_recipe_id = line.preparation?.id ?? null;
  }
  if (takeawayAllowed(ownerKind, line.kind)) {
    input.takeaway_only = line.takeawayOnly;
  }
  return input;
}

function unitHint(line: LineDraft) {
  if (consumesItem(line) && line.item) {
    const unit = line.item.unit;
    return `Se convierte a ${unit.name.toLowerCase()} (${unit.code}), la unidad base del insumo.`;
  }
  if (!consumesItem(line) && line.preparation) {
    return 'Se convierte a la unidad base de la preparación.';
  }
  return undefined;
}

// Conversión y costo que guardó Go, mientras la cantidad y la unidad no cambien.
function SavedLineDetail({ line }: { line: LineDraft }) {
  const saved = line.saved;
  if (!saved || saved.quantity !== line.quantity || saved.unit.id !== line.unitID) {
    return null;
  }
  const parts = [`En la unidad base: ${formatQuantity(saved.base_quantity, saved.base_unit)}`];
  if (saved.cost_display !== null) {
    parts.push(`costo del renglón ${formatMoney(saved.cost_display)}`);
  }
  if (saved.cost_pending) {
    parts.push('costo pendiente');
  }
  return <p className="muted text-xs recipe-line-detail">{parts.join(' · ')}</p>;
}

function RecipeLineEditor({
  line,
  number,
  ownerKind,
  recipeID,
  units,
  productIngredients,
  onChange,
  onRemove,
}: {
  line: LineDraft;
  number: number;
  ownerKind: RecipeOwnerKind;
  recipeID: string;
  units: InventoryUnit[];
  productIngredients: IngredientChoice[];
  onChange: (line: LineDraft) => void;
  onRemove: () => void;
}) {
  const quantityHint =
    line.kind === 'ingredient'
      ? 'Por porción del ingrediente: se multiplica por las porciones pedidas (Sin, Normal o Extra).'
      : undefined;
  return (
    <fieldset className="repeat-card inventory-line recipe-line" aria-label={`Renglón ${number}`}>
      <div className="section-heading inventory-line-heading">
        <strong>
          Renglón {number} · {componentKindLabels[line.kind]}
        </strong>
        <button type="button" className="danger-button" onClick={onRemove}>
          Quitar renglón
        </button>
      </div>
      {line.kind === 'ingredient' && (
        <>
          <CatalogIngredientPicker
            value={line.ingredient}
            required
            productIngredients={productIngredients}
            onChange={(ingredient) => onChange({ ...line, ingredient, saved: null })}
          />
          <SegmentedControl
            label="Qué consume cada porción"
            value={line.consumes}
            options={[
              { value: 'item', label: 'Un insumo' },
              { value: 'preparation', label: 'Una preparación' },
            ]}
            onChange={(consumes) =>
              onChange({
                ...line,
                consumes,
                item: null,
                preparation: null,
                unitID: '',
                saved: null,
              })
            }
          />
        </>
      )}
      {consumesItem(line) ? (
        <RecipeItemPicker
          label="Insumo"
          value={line.item}
          required
          onChange={(item) => onChange({ ...line, item, unitID: item?.unit.id ?? '', saved: null })}
        />
      ) : (
        <PreparationPicker
          label="Preparación"
          value={line.preparation}
          excludeID={ownerKind === 'preparation' ? recipeID : undefined}
          required
          onChange={(preparation) =>
            onChange({ ...line, preparation, unitID: preparation?.unitID ?? '', saved: null })
          }
        />
      )}
      <div className="form-grid">
        <DecimalTextField
          label="Cantidad"
          value={line.quantity}
          onChange={(quantity) => onChange({ ...line, quantity })}
          shape={inventoryQuantityShape}
          required
          hint={quantityHint}
        />
        <DimensionUnitSelect
          label="Unidad"
          units={units}
          dimension={lineDimension(line)}
          value={line.unitID}
          onChange={(unitID) => onChange({ ...line, unitID })}
          hint={unitHint(line)}
        />
      </div>
      {takeawayAllowed(ownerKind, line.kind) && (
        <div className="button-row">
          <ToggleButton
            label="Solo para llevar"
            pressed={line.takeawayOnly}
            onChange={(takeawayOnly) => onChange({ ...line, takeawayOnly })}
          />
          <span className="muted text-xs">
            Se consume solo en pedidos para llevar y de ventanilla; en salón se ignora.
          </span>
        </div>
      )}
      <SavedLineDetail line={line} />
    </fieldset>
  );
}

// Renglones de la receta con alta por tipo. Cada guardado los envía completos: la versión nueva
// reemplaza a la vigente sin modificar las anteriores.
export function RecipeLinesEditor({
  lines,
  onChange,
  ownerKind,
  recipeID,
  units,
  productIngredients,
}: {
  lines: LineDraft[];
  onChange: (lines: LineDraft[]) => void;
  ownerKind: RecipeOwnerKind;
  recipeID: string;
  units: InventoryUnit[];
  productIngredients: IngredientChoice[];
}) {
  const nextKey = useRef(lines.reduce((highest, line) => Math.max(highest, line.key), 0) + 1);

  function add(kind: RecipeComponentKind) {
    const key = nextKey.current;
    nextKey.current += 1;
    onChange([...lines, emptyLineDraft(kind, key)]);
  }

  const addLabels: Record<RecipeComponentKind, string> = {
    item: 'Agregar insumo',
    preparation: 'Agregar preparación',
    ingredient: 'Agregar ingrediente del catálogo',
  };

  return (
    <section className="form-stack" aria-labelledby="recipe-lines-title">
      <div>
        <h3 id="recipe-lines-title">Componentes</h3>
        <p className="muted text-xs">
          {ownerKind === 'preparation'
            ? 'Insumos y preparaciones de un lote, con su cantidad y unidad.'
            : 'Insumos y preparaciones de una unidad vendida y, en productos, la porción propia de sus ingredientes del catálogo.'}
        </p>
      </div>
      {lines.length === 0 && (
        <p className="muted">Sin componentes. Una receta activa necesita al menos uno.</p>
      )}
      {lines.map((line, index) => (
        <RecipeLineEditor
          key={line.key}
          line={line}
          number={index + 1}
          ownerKind={ownerKind}
          recipeID={recipeID}
          units={units}
          productIngredients={productIngredients}
          onChange={(changed) =>
            onChange(lines.map((entry) => (entry.key === line.key ? changed : entry)))
          }
          onRemove={() => onChange(lines.filter((entry) => entry.key !== line.key))}
        />
      ))}
      <div className="button-row">
        {allowedLineKinds(ownerKind).map((kind) => (
          <button key={kind} type="button" className="secondary-button" onClick={() => add(kind)}>
            <IconLabel icon="plus">{addLabels[kind]}</IconLabel>
          </button>
        ))}
      </div>
    </section>
  );
}
