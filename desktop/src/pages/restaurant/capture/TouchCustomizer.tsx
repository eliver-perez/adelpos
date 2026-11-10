import { Accordion } from '../../../components/Accordion';
import { Notice } from '../../../components/Forms';
import { formatSignedMoney } from '../../../services/money';
import { CatalogImage } from '../../../components/catalog/EditorFields';
import {
  componentProductIDs,
  componentSelection,
  originalCustomization,
  withComponent,
} from '../../../components/catalog/customizationRules';
import { useResource } from '../../../hooks/useResource';
import { quickNotesFor } from '../../../services/catalog';
import type {
  ComboSlot,
  ComponentSelection,
  Configuration,
  Customization,
  Lookups,
  ProductBreakdown,
} from '../../../services/catalog';
import { QuantityStepper } from '../../../components/QuantityStepper';
import { positiveAmount } from './captureRules';
import { GroupBlock, IngredientBlock, VariantBlock } from './CustomizationBlocks';
import { KitchenNotes } from './KitchenNotes';
import { applicableQuickNotes, reconcileQuickNotes } from './quickNoteRules';

// Presentación táctil de la personalización. Reutiliza la configuración que entrega Go
// (`catalog_configuration`) y las mismas reglas de selección que ProductCustomizer; solo cambia
// la presentación: bloques con botones grandes en lugar de campos de formulario. Cada nivel (el
// producto y cada componente de combo) termina con sus indicaciones para cocina: las rápidas de
// la categoría de su propio producto, con efecto solo si aplican a sus ingredientes (etapa 8).
export function TouchCustomizer({
  productID,
  variantID,
  onVariantChange,
  value,
  onChange,
  lookups,
  breakdown,
  noteShortcuts,
  basePriceUnavailable = false,
  depth = 0,
}: {
  productID: string;
  variantID: string | null;
  onVariantChange: (value: string | null) => void;
  value: Customization;
  onChange: (value: Customization) => void;
  lookups: Lookups;
  breakdown: ProductBreakdown | null;
  noteShortcuts: string[];
  basePriceUnavailable?: boolean;
  depth?: number;
}) {
  const configuration = useResource<Configuration>('catalog_configuration', {
    id: productID,
    enabled: !!productID,
  });
  const data = configuration.data;
  if (!data || data.product.id !== productID) {
    return <Notice>{configuration.error || 'Consultando configuración…'}</Notice>;
  }
  if (depth > 8) {
    return <Notice>El combo supera el límite de profundidad.</Notice>;
  }
  const product = data.product;
  const categoryNotes = quickNotesFor(lookups, productID);
  const quickNotes = applicableQuickNotes(categoryNotes, product.ingredients);
  const hiddenTexts = categoryNotes
    .filter((note) => !quickNotes.some((option) => option.id === note.id))
    .map((note) => note.text);
  return (
    <div className="capture-customizer">
      {depth === 0 && (product.description || product.image_id) && (
        <div className="capture-customizer-lead">
          {product.image_id && <CatalogImage id={product.image_id} decorative />}
          {product.description && <p className="muted text-sm">{product.description}</p>}
        </div>
      )}
      <Notice>{data.availability_reason}</Notice>
      <VariantBlock
        product={product}
        variantID={variantID}
        onChange={onVariantChange}
        basePriceUnavailable={basePriceUnavailable}
      />
      {product.groups.map((binding) => {
        const group = data.groups.find((entry) => entry.id === binding.group_id);
        if (!group) {
          return null;
        }
        return (
          <GroupBlock
            key={binding.group_id}
            binding={binding}
            group={group}
            value={value}
            onChange={onChange}
            breakdown={breakdown}
          />
        );
      })}
      <IngredientBlock
        product={product}
        value={value}
        onChange={(next) => onChange(reconcileQuickNotes(next, quickNotes, product.ingredients))}
        lookups={lookups}
      />
      {product.slots.map((slot) => (
        <ComboComponentBlock
          key={slot.id}
          slot={slot}
          value={value}
          onChange={onChange}
          lookups={lookups}
          breakdown={breakdown?.components.find((entry) => entry.slot_id === slot.id) ?? null}
          noteShortcuts={noteShortcuts}
          depth={depth}
        />
      ))}
      <KitchenNotes
        value={value}
        onChange={onChange}
        shortcuts={noteShortcuts}
        quickNotes={quickNotes}
        hiddenTexts={hiddenTexts}
      />
    </div>
  );
}

// Componente de combo: el producto que lo ocupa como botones (el original y sus alternativas),
// la cantidad cuando el combo permite variarla y, en un acordeón, su propia personalización.
function ComboComponentBlock({
  slot,
  value,
  onChange,
  lookups,
  breakdown,
  noteShortcuts,
  depth,
}: {
  slot: ComboSlot;
  value: Customization;
  onChange: (value: Customization) => void;
  lookups: Lookups;
  breakdown: ProductBreakdown['components'][number] | null;
  noteShortcuts: string[];
  depth: number;
}) {
  const selection = componentSelection(value, slot);
  const products = componentProductIDs(slot);

  function change(next: ComponentSelection) {
    onChange(withComponent(value, next));
  }

  const selectedName =
    lookups.products.find((entry) => entry.id === selection.product_id)?.name ?? 'Componente';
  return (
    <section className="capture-block" aria-label={slot.name}>
      <header className="capture-block-header">
        <h3>{slot.name}</h3>
        <span className="capture-block-rule">
          Incluidas {slot.included_quantity}
          {slot.min_quantity !== slot.max_quantity
            ? ` · de ${slot.min_quantity} a ${slot.max_quantity}`
            : ''}
        </span>
      </header>
      {breakdown && positiveAmount(breakdown.charge) && (
        <p className="capture-block-charge">
          Cargo del componente {formatSignedMoney(breakdown.charge)}
        </p>
      )}
      <div className="capture-choices">
        {products.map((productID) => {
          const option = slot.options.find((entry) => entry.product_id === productID);
          const name = lookups.products.find((entry) => entry.id === productID)?.name ?? 'Producto';
          return (
            <button
              type="button"
              key={productID}
              className="capture-choice"
              aria-pressed={selection.product_id === productID}
              onClick={() => {
                if (selection.product_id === productID) return;
                change({
                  ...selection,
                  product_id: productID,
                  variant_id: null,
                  customization: originalCustomization(),
                });
              }}
            >
              <span>{name}</span>{' '}
              {option && positiveAmount(option.substitution_price) && (
                <span className="capture-choice-extra">
                  {formatSignedMoney(option.substitution_price)}
                </span>
              )}
            </button>
          );
        })}
      </div>
      {slot.min_quantity !== slot.max_quantity && (
        <QuantityStepper
          verb="Cantidad"
          name={slot.name}
          quantity={selection.quantity}
          minimum={slot.min_quantity}
          max={slot.max_quantity}
          onChange={(quantity) => change({ ...selection, quantity })}
        />
      )}
      {selection.quantity > 0 && (
        <Accordion title={`Personalizar ${selectedName}`}>
          <TouchCustomizer
            productID={selection.product_id}
            variantID={selection.variant_id}
            onVariantChange={(variant_id) => change({ ...selection, variant_id })}
            value={selection.customization}
            onChange={(customization) => change({ ...selection, customization })}
            lookups={lookups}
            breakdown={breakdown?.product ?? null}
            noteShortcuts={noteShortcuts}
            depth={depth + 1}
          />
        </Accordion>
      )}
    </section>
  );
}
