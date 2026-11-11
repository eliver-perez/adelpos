import { useState } from 'react';
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
  GroupOption,
  Lookups,
  OptionGroup,
  Product,
  ProductBreakdown,
  ProductGroup,
} from '../../../services/catalog';
import { formatMoney, formatSignedMoney } from '../../../services/money';
import { nonZeroAmount, positiveAmount } from './captureRules';
import { displayQuantity, fromThousandths, toThousandths, unit } from './quantities';

// Bloques táctiles de la personalización en el panel central. Cada opción es un botón grande con
// aria-pressed; la selección es la misma estructura que construye ProductCustomizer y Go la
// valida y cotiza.

function extraLabel(amount: string | undefined) {
  return positiveAmount(amount) ? formatSignedMoney(amount) : '';
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

// Precio que se muestra en la tarjeta de una opción, tal como lo configura el catálogo (Go valida
// y calcula el cargo real): en grupos por equivalencia, el valor de referencia de la opción
// («$30.00») y, si existe, su extra explícito («+$10.00»); en grupos por cantidad, el extra
// explícito o, sin él, el precio del primer tramo adicional. La interfaz no suma importes.
function optionPrices(group: OptionGroup, option: GroupOption): string[] {
  const prices: string[] = [];
  if (group.pricing_mode === 'value' && positiveAmount(option.reference_value)) {
    prices.push(formatMoney(option.reference_value));
  }
  if (positiveAmount(option.extra_price)) {
    prices.push(formatSignedMoney(option.extra_price));
  } else if (group.pricing_mode === 'quantity' && positiveAmount(option.tiers[0]?.unit_price)) {
    prices.push(formatSignedMoney(option.tiers[0]?.unit_price));
  }
  return prices;
}

// Cambio de precio del grupo con lo que devolvió Go: «+$5.00», «−$10.00» o «Sin cambio». La
// diferencia solo es negativa cuando el producto tiene activada la opción «El precio puede bajar
// al cambiar complementos» (etapa 9A); con ella se muestra el efecto real que aplicó Go
// (`applied_difference`), que con los límites del precio puede ser menor que la diferencia de
// valor: el grupo nunca promete más de lo que bajó el precio. Mientras Go recalcula,
// «Calculando…». Si hay equivalencia y extras se muestran los dos importes de Go.
function groupDelta(charge: ProductBreakdown['groups'][number] | undefined): string {
  if (!charge) {
    return 'Calculando…';
  }
  const equivalence = charge.applied_difference ?? charge.difference;
  const difference = nonZeroAmount(equivalence);
  const extras = positiveAmount(charge.extras);
  if (difference && extras) {
    return `Equivalencia ${formatSignedMoney(equivalence)} · Extras ${formatSignedMoney(charge.extras)}`;
  }
  if (difference) {
    return formatSignedMoney(equivalence);
  }
  if (extras) {
    return formatSignedMoney(charge.extras);
  }
  return 'Sin cambio';
}

function portionsLabel(quantity: number) {
  const text = displayQuantity(fromThousandths(quantity));
  return `${text} ${quantity === unit ? 'porción' : 'porciones'}`;
}

// Grupo de complementos o guarniciones. Con máximo de una porción es una elección única (un toque
// sustituye, como en la etapa 6.1). Con varias porciones (etapa 8, ajuste de la captura): un
// toque en la tarjeta suma una porción, el botón «−» de la propia tarjeta resta una, la cantidad
// se ve grande y sustituir es de dos toques («−» en arroz y toque en pasta). El encabezado
// muestra las porciones elegidas respecto de las incluidas («2 de 2») y el cambio de precio que
// calculó Go; al intentar pasar del máximo aparece un aviso en el propio grupo.
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
  const [warning, setWarning] = useState('');
  const selection = groupSelection(value, binding);
  const limits = groupLimits(binding, group);
  const minimum = toThousandths(limits.minimum);
  const maximum = limits.maximum === null ? null : toThousandths(limits.maximum);
  const total = selection.reduce((sum, option) => sum + toThousandths(option.quantity), 0);
  const included = binding.included.reduce(
    (sum, option) => sum + toThousandths(option.quantity),
    0,
  );
  // Con máximo de una porción el grupo se comporta como una elección única.
  const single = maximum === unit;
  const charge = breakdown?.groups.find((entry) => entry.group_id === group.id);
  const counter =
    included > 0
      ? `${displayQuantity(fromThousandths(total))} de ${displayQuantity(fromThousandths(included))}`
      : `${displayQuantity(fromThousandths(total))} ${total === unit ? 'elegida' : 'elegidas'}`;

  function quantityOf(optionID: string) {
    return toThousandths(selection.find((entry) => entry.option_id === optionID)?.quantity);
  }

  function change(next: Customization) {
    setWarning('');
    onChange(next);
  }

  function remove(optionID: string) {
    change(
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
    change(withGroupOption(value, binding, optionID, fromThousandths(quantity)));
  }

  // Un toque suma una porción si cabe en el máximo del grupo y en el de la opción; si no, avisa
  // en el grupo en lugar de desactivar la tarjeta sin explicación.
  function addPortion(option: GroupOption, name: string) {
    const quantity = quantityOf(option.id);
    const optionMaximum = option.max_quantity === null ? null : toThousandths(option.max_quantity);
    if (maximum !== null && total + unit > maximum) {
      setWarning(
        `${group.name} admite hasta ${portionsLabel(maximum)}: toca «−» en otra opción para cambiarla.`,
      );
      return;
    }
    if (optionMaximum !== null && quantity + unit > optionMaximum) {
      setWarning(`${name} admite hasta ${portionsLabel(optionMaximum)}.`);
      return;
    }
    setQuantity(option.id, quantity + unit);
  }

  function chooseSingle(option: GroupOption) {
    if (quantityOf(option.id) > 0) {
      // Una elección obligatoria no se deja vacía con un toque.
      if (minimum === 0) remove(option.id);
      return;
    }
    change(withGroupSelection(value, group.id, [{ option_id: option.id, quantity: '1' }]));
  }

  return (
    <section className="capture-block" aria-label={group.name}>
      <header className="capture-block-header">
        <h3>{group.name}</h3>
        <span className="capture-block-meta">
          <span
            className="capture-block-count"
            aria-label={`${displayQuantity(fromThousandths(total))} porciones elegidas de ${displayQuantity(fromThousandths(included))} incluidas`}
          >
            {counter}
          </span>
          <span
            className={`capture-block-rule ${minimum > 0 && total < minimum ? 'capture-block-required' : ''}`}
          >
            {groupRule(minimum, maximum)}
          </span>
        </span>
      </header>
      <p className="capture-block-charge" aria-live="polite">
        {groupDelta(charge)}
      </p>
      {warning && (
        <p className="capture-block-warning" role="status">
          {warning}
        </p>
      )}
      <div className="capture-choices">
        {group.options.map((option) => {
          const quantity = quantityOf(option.id);
          const selected = quantity > 0;
          // La porción solo se nombra cuando la opción tiene una porción propia configurada.
          const optionName = option.name ?? 'Opción';
          const name =
            option.portion_id && option.portion_name
              ? `${optionName} · ${option.portion_name}`
              : optionName;
          const prices = optionPrices(group, option);
          if (single) {
            return (
              <div className="capture-choice-cell" key={option.id}>
                <button
                  type="button"
                  className="capture-choice"
                  aria-pressed={selected}
                  disabled={!option.active}
                  onClick={() => chooseSingle(option)}
                >
                  <span>{name}</span>{' '}
                  {prices.map((price) => (
                    <span className="capture-choice-extra" key={price}>
                      {price}
                    </span>
                  ))}
                  {!option.active && <span className="capture-choice-extra">No disponible</span>}
                </button>
              </div>
            );
          }
          return (
            <div
              className={`capture-portion-card ${selected ? 'capture-portion-selected' : ''}`}
              key={option.id}
              role="group"
              aria-label={`${name}: ${selected ? portionsLabel(quantity) : 'sin elegir'}`}
            >
              <button
                type="button"
                className="capture-portion-add"
                aria-pressed={selected}
                aria-label={`Agregar una porción de ${name}`}
                disabled={!option.active}
                onClick={() => addPortion(option, name)}
              >
                <span className="capture-portion-text">
                  <span>{name}</span>
                  {prices.map((price) => (
                    <span className="capture-choice-extra" key={price}>
                      {price}
                    </span>
                  ))}
                  {!option.active && <span className="capture-choice-extra">No disponible</span>}
                </span>
                {selected && (
                  <span className="capture-portion-quantity" aria-hidden="true">
                    {displayQuantity(fromThousandths(quantity))}
                  </span>
                )}
              </button>
              {selected && (
                <button
                  type="button"
                  className="secondary-button capture-portion-remove"
                  aria-label={`Quitar una porción de ${name}`}
                  onClick={() => setQuantity(option.id, quantity - unit)}
                >
                  −
                </button>
              )}
            </div>
          );
        })}
      </div>
      {minimum === 0 && total > 0 && !single && (
        <button
          type="button"
          className="secondary-button capture-block-clear"
          onClick={() => change(withoutGroupOptions(value, group.id))}
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
