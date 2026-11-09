import {
  groupLimits,
  groupSelection,
  ingredientQuantity,
  withGroupOption,
  withGroupSelection,
  withIngredient,
  withoutGroupOptions,
  withoutIngredientChange,
} from '../../../components/catalog/customizationRules';
import type {
  Customization,
  Lookups,
  OptionGroup,
  Product,
  ProductBreakdown,
  ProductGroup,
} from '../../../services/catalog';
import { positiveAmount } from './captureRules';
import { displayQuantity, fromThousandths, toThousandths, unit } from './quantities';

// Bloques táctiles de la personalización en el panel central. Cada opción es un botón grande con
// aria-pressed; la selección es la misma estructura que construye ProductCustomizer y Go la
// valida y cotiza.

function extraLabel(amount: string | undefined) {
  return positiveAmount(amount) ? `+$${amount}` : '';
}

export function VariantBlock({
  product,
  variantID,
  onChange,
  basePriceUnavailable,
}: {
  product: Product;
  variantID: string | null;
  onChange: (value: string | null) => void;
  basePriceUnavailable: boolean;
}) {
  const variants = product.variants.filter((variant) => variant.active);
  if (variants.length === 0) {
    return null;
  }
  return (
    <section className="capture-block" aria-label="Presentación">
      <header className="capture-block-header">
        <h3>Presentación</h3>
        <span className="capture-block-rule">{basePriceUnavailable ? 'Elige 1' : 'Opcional'}</span>
      </header>
      <div className="capture-choices">
        {!basePriceUnavailable && (
          <button
            type="button"
            className="capture-choice"
            aria-pressed={variantID === null}
            onClick={() => onChange(null)}
          >
            <span>Presentación base</span>
          </button>
        )}
        {variants.map((variant) => (
          <button
            type="button"
            key={variant.id}
            className="capture-choice"
            aria-pressed={variantID === variant.id}
            onClick={() => onChange(variant.id)}
          >
            <span>{variant.name}</span>
          </button>
        ))}
      </div>
    </section>
  );
}

function groupRule(minimum: number, maximum: number | null) {
  if (minimum === unit && maximum === unit) {
    return 'Obligatorio · elige 1';
  }
  const upTo = maximum === null ? '' : ` · hasta ${fromThousandths(maximum)}`;
  if (minimum > 0) {
    return `Obligatorio · al menos ${fromThousandths(minimum)}${upTo}`;
  }
  return maximum === unit ? 'Opcional · elige 1' : `Opcional${upTo}`;
}

export function GroupBlock({
  binding,
  group,
  value,
  onChange,
  breakdown,
}: {
  binding: ProductGroup;
  group: OptionGroup;
  value: Customization;
  onChange: (value: Customization) => void;
  breakdown: ProductBreakdown | null;
}) {
  const selection = groupSelection(value, binding);
  const limits = groupLimits(binding, group);
  const minimum = toThousandths(limits.minimum);
  const maximum = limits.maximum === null ? null : toThousandths(limits.maximum);
  const total = selection.reduce((sum, option) => sum + toThousandths(option.quantity), 0);
  // Con máximo de una porción el grupo se comporta como una elección única.
  const single = maximum === unit;
  const charge = breakdown?.groups.find((entry) => entry.group_id === group.id);

  function quantityOf(optionID: string) {
    return toThousandths(selection.find((entry) => entry.option_id === optionID)?.quantity);
  }

  function remove(optionID: string) {
    onChange(
      withGroupSelection(
        value,
        group.id,
        selection.filter((entry) => entry.option_id !== optionID),
      ),
    );
  }

  function setQuantity(optionID: string, quantity: number) {
    if (quantity <= 0) {
      remove(optionID);
      return;
    }
    onChange(withGroupOption(value, binding, optionID, fromThousandths(quantity)));
  }

  return (
    <section className="capture-block" aria-label={group.name}>
      <header className="capture-block-header">
        <h3>{group.name}</h3>
        <span
          className={`capture-block-rule ${minimum > 0 && total < minimum ? 'capture-block-required' : ''}`}
        >
          {groupRule(minimum, maximum)}
        </span>
      </header>
      {charge && (positiveAmount(charge.difference) || positiveAmount(charge.extras)) && (
        <p className="capture-block-charge">
          {positiveAmount(charge.difference) &&
            `Diferencia por equivalencia +$${charge.difference}`}
          {positiveAmount(charge.difference) && positiveAmount(charge.extras) && ' · '}
          {positiveAmount(charge.extras) && `Extras +$${charge.extras}`}
        </p>
      )}
      <div className="capture-choices">
        {group.options.map((option) => {
          const quantity = quantityOf(option.id);
          const selected = quantity > 0;
          const optionMaximum =
            option.max_quantity === null ? null : toThousandths(option.max_quantity);
          const room =
            (maximum === null || total + unit <= maximum) &&
            (optionMaximum === null || quantity + unit <= optionMaximum);
          const stepper = selected && !single && (maximum === null || maximum > unit);
          // La porción solo se nombra cuando la opción tiene una porción propia configurada.
          const name =
            option.portion_id && option.portion_name
              ? `${option.name} · ${option.portion_name}`
              : option.name;
          return (
            <div className="capture-choice-cell" key={option.id}>
              <button
                type="button"
                className="capture-choice"
                aria-pressed={selected}
                disabled={!option.active || (!selected && !single && !room)}
                onClick={() => {
                  if (single) {
                    if (selected) {
                      // Una elección obligatoria no se deja vacía con un toque.
                      if (minimum === 0) remove(option.id);
                      return;
                    }
                    onChange(
                      withGroupSelection(value, group.id, [
                        { option_id: option.id, quantity: '1' },
                      ]),
                    );
                    return;
                  }
                  setQuantity(option.id, selected ? 0 : unit);
                }}
              >
                <span>{name}</span>{' '}
                {extraLabel(option.extra_price) && (
                  <span className="capture-choice-extra">{extraLabel(option.extra_price)}</span>
                )}
                {!option.active && <span className="capture-choice-extra">No disponible</span>}
              </button>
              {stepper && (
                <div
                  className="capture-choice-stepper"
                  role="group"
                  aria-label={`Porciones de ${name}`}
                >
                  <button
                    type="button"
                    className="secondary-button"
                    aria-label={`Quitar una porción de ${name}`}
                    onClick={() => setQuantity(option.id, quantity - unit)}
                  >
                    −
                  </button>
                  <output>{displayQuantity(fromThousandths(quantity))}</output>
                  <button
                    type="button"
                    className="secondary-button"
                    aria-label={`Agregar una porción de ${name}`}
                    disabled={!room}
                    onClick={() => setQuantity(option.id, quantity + unit)}
                  >
                    +
                  </button>
                </div>
              )}
            </div>
          );
        })}
      </div>
      {minimum === 0 && total > 0 && !single && (
        <button
          type="button"
          className="secondary-button capture-block-clear"
          onClick={() => onChange(withoutGroupOptions(value, group.id))}
        >
          Sin {group.name.toLocaleLowerCase('es')}
        </button>
      )}
    </section>
  );
}

// Quitar ingredientes removibles («pedido especial» del sistema anterior) y agregar una porción
// extra de los agregables, con el cargo configurado en el catálogo.
export function IngredientBlock({
  product,
  value,
  onChange,
  lookups,
}: {
  product: Product;
  value: Customization;
  onChange: (value: Customization) => void;
  lookups: Lookups;
}) {
  const editable = product.ingredients.filter(
    (ingredient) => ingredient.removable || ingredient.addable,
  );
  if (editable.length === 0) {
    return null;
  }
  return (
    <section className="capture-block" aria-label="Ingredientes">
      <header className="capture-block-header">
        <h3>Ingredientes</h3>
        <span className="capture-block-rule">Opcional</span>
      </header>
      <div className="capture-choices">
        {editable.map((ingredient) => {
          const name =
            ingredient.name ??
            lookups.ingredients.find((entry) => entry.id === ingredient.ingredient_id)?.name ??
            'Ingrediente';
          const requested = toThousandths(ingredientQuantity(value, ingredient));
          const included = toThousandths(ingredient.included_quantity);
          const maximum = toThousandths(ingredient.max_quantity);
          const extra = Math.min(included + unit, maximum > 0 ? maximum : included + unit);
          return (
            <div className="capture-choice-group" key={ingredient.ingredient_id}>
              {ingredient.removable && included > 0 && (
                <button
                  type="button"
                  className="capture-choice"
                  aria-pressed={requested === 0}
                  onClick={() =>
                    onChange(
                      requested === 0
                        ? withoutIngredientChange(value, ingredient.ingredient_id)
                        : withIngredient(value, ingredient.ingredient_id, '0'),
                    )
                  }
                >
                  <span>Sin {name.toLocaleLowerCase('es')}</span>
                </button>
              )}
              {ingredient.addable && extra > included && (
                <button
                  type="button"
                  className="capture-choice"
                  aria-pressed={requested > included}
                  onClick={() =>
                    onChange(
                      requested > included
                        ? withoutIngredientChange(value, ingredient.ingredient_id)
                        : withIngredient(value, ingredient.ingredient_id, fromThousandths(extra)),
                    )
                  }
                >
                  <span>Extra {name.toLocaleLowerCase('es')}</span>{' '}
                  {extraLabel(ingredient.extra_price) && (
                    <span className="capture-choice-extra">
                      {extraLabel(ingredient.extra_price)}
                    </span>
                  )}
                </button>
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}
