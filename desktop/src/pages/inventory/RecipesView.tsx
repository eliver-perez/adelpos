import { useState } from 'react';
import { Notice } from '../../components/Forms';
import { SegmentedControl } from '../../components/SegmentedControl';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, InventoryUnit } from '../../services/inventory';
import { IngredientPortionsView } from './IngredientPortionsView';
import { PreparationsView } from './PreparationsView';
import { ProductionsView } from './ProductionsView';
import { RecipeEditorPanel } from './RecipeEditor';
import type { RecipeDraftTarget } from './RecipeEditor';
import { RecipeTargetsView } from './RecipeTargetsView';

type RecipesSection = 'targets' | 'preparations' | 'portions' | 'productions';

// Receta abierta en el panel: una existente por su identificador o una nueva con su destino.
type OpenRecipe = { recipeID: string; target: null } | { recipeID: ''; target: RecipeDraftTarget };

// Pestaña «Recetas» (inventory.read): productos y complementos del catálogo con su receta,
// preparaciones (subrecetas), porciones habituales de los ingredientes del catálogo y elaboraciones
// de las preparaciones almacenables. Las recetas se editan con inventory.recipes.manage y las
// preparaciones con inventory.subrecipes.manage; Go vuelve a comprobarlo en cada escritura.
export function RecipesView({
  can,
  execute,
  busy,
  timezone,
  initialSearch = '',
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  // Búsqueda inicial de productos (alerta «Producto sin receta» del resumen).
  initialSearch?: string;
}) {
  const [section, setSection] = useState<RecipesSection>('targets');
  const [open, setOpen] = useState<OpenRecipe | null>(null);
  const units = useResource<InventoryUnit[]>('inventory_units', {
    reloadEvent: inventoryChangeEvent,
  });
  const productionsVisible = can('inventory.production.record') || can('inventory.movements.read');
  const options: { value: RecipesSection; label: string }[] = [
    { value: 'targets', label: 'Productos y complementos' },
    { value: 'preparations', label: 'Preparaciones' },
    { value: 'portions', label: 'Porciones habituales' },
  ];
  if (productionsVisible) {
    options.push({ value: 'productions', label: 'Elaboraciones' });
  }
  const current = options.some((option) => option.value === section) ? section : 'targets';
  const unitList = units.data ?? [];

  if (open) {
    return (
      <RecipeEditorPanel
        key={open.recipeID || `new:${open.target?.ownerKind}:${open.target?.title}`}
        initialRecipeID={open.recipeID}
        target={open.target}
        units={unitList}
        can={can}
        execute={execute}
        busy={busy}
        timezone={timezone}
        onClose={() => setOpen(null)}
      />
    );
  }

  const openRecipe = (recipeID: string) => setOpen({ recipeID, target: null });
  const createRecipe = (target: RecipeDraftTarget) => setOpen({ recipeID: '', target });

  return (
    <div className="form-stack">
      <div className="inventory-subnav">
        <SegmentedControl
          label="Apartados de recetas"
          value={current}
          options={options}
          onChange={setSection}
        />
      </div>
      <Notice>{units.error}</Notice>
      {current === 'targets' && (
        <RecipeTargetsView
          editable={can('inventory.recipes.manage')}
          initialSearch={initialSearch}
          onOpenRecipe={openRecipe}
          onCreateRecipe={createRecipe}
        />
      )}
      {current === 'preparations' && (
        <PreparationsView
          editable={can('inventory.subrecipes.manage')}
          timezone={timezone}
          onOpenRecipe={openRecipe}
          onCreateRecipe={createRecipe}
        />
      )}
      {current === 'portions' && (
        <IngredientPortionsView
          editable={can('inventory.recipes.manage')}
          units={unitList}
          execute={execute}
          busy={busy}
          timezone={timezone}
        />
      )}
      {current === 'productions' && (
        <ProductionsView can={can} execute={execute} busy={busy} timezone={timezone} />
      )}
    </div>
  );
}
