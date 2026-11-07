import { Field, Notice } from '../Forms';
import { useResource } from '../../hooks/useResource';
import type {
  ComponentSelection,
  Configuration,
  Customization,
  Lookups,
} from '../../services/catalog';
import { CatalogImage, DecimalField, IntegerField, ReferenceField } from './EditorFields';

export const originalCustomization = (): Customization => ({
  ingredients: [],
  groups: [],
  components: [],
  notes: '',
});

export function ProductCustomizer({
  productID,
  variantID,
  onVariantChange,
  value,
  onChange,
  onReset,
  lookups,
  depth = 0,
}: {
  productID: string;
  variantID: string | null;
  onVariantChange: (value: string | null) => void;
  value: Customization;
  onChange: (value: Customization) => void;
  onReset: () => void;
  lookups: Lookups;
  depth?: number;
}) {
  const configuration = useResource<Configuration>('catalog_configuration', {
    id: productID,
    enabled: !!productID,
  });
  if (!productID) return null;
  const data = configuration.data;
  if (!data || data.product.id !== productID)
    return <Notice>{configuration.error || 'Consultando configuración…'}</Notice>;
  const product = data.product;
  if (depth > 8) return <Notice>El combo supera el límite de profundidad.</Notice>;
  function changeComponent(selection: ComponentSelection) {
    onChange({
      ...value,
      components: [
        ...value.components.filter((entry) => entry.slot_id !== selection.slot_id),
        selection,
      ],
    });
  }
  return (
    <div className="product-customizer form-stack">
      <div className="button-row">
        <CatalogImage id={product.image_id} />
        <div>
          <h3>{product.name}</h3>
          <p className="muted text-sm">{product.description}</p>
          <p className="muted text-xs">
            Configuración r{product.revision} · {data.timezone}
          </p>
        </div>
      </div>
      <Notice>{data.availability_reason}</Notice>
      {product.variants.length > 0 && (
        <ReferenceField
          optional
          label="Presentación (vacío: producto base)"
          value={variantID}
          options={product.variants}
          onChange={(id) => onVariantChange(id || null)}
        />
      )}
      {product.ingredients.length > 0 && (
        <section className="catalog-section">
          <h3>Ingredientes y preparación</h3>
          {product.ingredients.map((ingredient) => (
            <DecimalField
              quantity
              key={ingredient.ingredient_id}
              label={`${ingredient.name} · incluido ${ingredient.included_quantity} · adicional $${ingredient.extra_price}`}
              value={
                value.ingredients.find((entry) => entry.ingredient_id === ingredient.ingredient_id)
                  ?.quantity ?? ingredient.included_quantity
              }
              onChange={(quantity) =>
                onChange({
                  ...value,
                  ingredients: [
                    ...value.ingredients.filter(
                      (entry) => entry.ingredient_id !== ingredient.ingredient_id,
                    ),
                    { ingredient_id: ingredient.ingredient_id, quantity },
                  ],
                })
              }
            />
          ))}
        </section>
      )}
      {product.groups.map((binding) => {
        const group = data.groups.find((entry) => entry.id === binding.group_id);
        if (!group) return null;
        const selection =
          value.groups.find((entry) => entry.group_id === binding.group_id)?.options ??
          binding.included;
        return (
          <section className="catalog-section" key={binding.group_id}>
            <h3>{group.name}</h3>
            <p className="muted text-sm">
              {group.pricing_mode === 'value'
                ? 'Equivalencia por valor: se cobra solamente la diferencia positiva.'
                : 'Unidades adicionales con tramos explícitos.'}{' '}
              Mínimo: {binding.min_quantity ?? group.min_quantity}. Máximo:{' '}
              {binding.max_quantity ?? group.max_quantity ?? 'sin límite'}.
            </p>
            {group.options.map((option) => (
              <div className="customization-option" key={option.id}>
                <div>
                  <strong>
                    {option.name} · {option.portion_name}
                  </strong>
                  <p className="muted text-xs">
                    Incluido:{' '}
                    {binding.included.find((entry) => entry.option_id === option.id)?.quantity ??
                      '0'}{' '}
                    · Valor: ${option.reference_value} · Extra explícito: ${option.extra_price}
                    {!option.active ? ' · inactivo' : ''}
                  </p>
                </div>
                <DecimalField
                  quantity
                  label="Porciones solicitadas"
                  value={selection.find((entry) => entry.option_id === option.id)?.quantity ?? '0'}
                  onChange={(quantity) =>
                    onChange({
                      ...value,
                      groups: [
                        ...value.groups.filter((entry) => entry.group_id !== binding.group_id),
                        {
                          group_id: binding.group_id,
                          options: [
                            ...selection.filter((entry) => entry.option_id !== option.id),
                            { option_id: option.id, quantity },
                          ],
                        },
                      ],
                    })
                  }
                />
              </div>
            ))}
            <button
              type="button"
              className="secondary-button"
              onClick={() =>
                onChange({
                  ...value,
                  groups: [
                    ...value.groups.filter((entry) => entry.group_id !== binding.group_id),
                    { group_id: binding.group_id, options: [] },
                  ],
                })
              }
            >
              Retirar todos los complementos de este grupo
            </button>
          </section>
        );
      })}
      {product.slots.map((slot) => {
        const selection = value.components.find((entry) => entry.slot_id === slot.id) ?? {
          slot_id: slot.id,
          product_id: slot.default_product_id,
          variant_id: null,
          quantity: slot.default_quantity,
          customization: originalCustomization(),
        };
        const options = lookups.products.filter((product) =>
          slot.options.some((option) => option.product_id === product.id),
        );
        return (
          <section className="catalog-section" key={slot.id}>
            <h3>{slot.name}</h3>
            <p className="muted text-sm">
              Incluidas en el precio del combo: {slot.included_quantity} unidades.
            </p>
            <div className="form-grid">
              <ReferenceField
                label="Producto del componente"
                value={selection.product_id}
                options={options}
                onChange={(product_id) =>
                  changeComponent({
                    ...selection,
                    product_id,
                    variant_id: null,
                    customization: originalCustomization(),
                  })
                }
              />
              <IntegerField
                label="Cantidad de componente"
                value={selection.quantity}
                min={slot.min_quantity}
                max={slot.max_quantity}
                onChange={(quantity) => changeComponent({ ...selection, quantity })}
              />
            </div>
            {selection.quantity > 0 && (
              <details>
                <summary>Personalizar este componente</summary>
                <ProductCustomizer
                  productID={selection.product_id}
                  variantID={selection.variant_id}
                  onVariantChange={(variant_id) => changeComponent({ ...selection, variant_id })}
                  value={selection.customization}
                  onChange={(customization) => changeComponent({ ...selection, customization })}
                  onReset={() =>
                    changeComponent({
                      ...selection,
                      variant_id: null,
                      customization: originalCustomization(),
                    })
                  }
                  lookups={lookups}
                  depth={depth + 1}
                />
              </details>
            )}
          </section>
        );
      })}
      <Field label="Observaciones para preparación (sin cargo automático)">
        <textarea
          rows={2}
          maxLength={500}
          placeholder="Sin cebolla, salsa aparte…"
          value={value.notes}
          onChange={(event) => onChange({ ...value, notes: event.target.value })}
        />
      </Field>
      <button type="button" className="secondary-button" onClick={onReset}>
        Restablecer configuración original
      </button>
    </div>
  );
}
