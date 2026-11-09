import type { Lookups, Product } from '../../services/catalog';
import {
  Check,
  DecimalField,
  ReferenceField,
  RemoveButton,
  RepeatSection,
  ScheduleEditor,
  TextField,
} from './EditorFields';

interface ProductDetailsProps {
  value: Product;
  onChange: (value: Product) => void;
  lookups: Lookups;
  can: (permission: string) => boolean;
}

export function VariantEditor({ value, onChange, can }: ProductDetailsProps) {
  return (
    <RepeatSection
      title="Presentaciones opcionales"
      hint="Deja esta sección vacía para un producto independiente. Las presentaciones omitidas se conservan inactivas."
      onAdd={() =>
        onChange({
          ...value,
          variants: [
            ...value.variants,
            {
              id: '',
              name: '',
              code: '',
              barcode: null,
              price: value.base_price,
              active: true,
              availability: { days: [], windows: [] },
            },
          ],
        })
      }
    >
      {value.variants.map((variant, index) => (
        <div key={index} className="repeat-card">
          <div className="form-grid">
            <TextField
              label="Nombre de presentación"
              required
              value={variant.name}
              onChange={(name) =>
                onChange({
                  ...value,
                  variants: value.variants.map((item, position) =>
                    position === index ? { ...item, name } : item,
                  ),
                })
              }
            />
            <TextField
              label="Código de presentación"
              required
              value={variant.code}
              onChange={(code) =>
                onChange({
                  ...value,
                  variants: value.variants.map((item, position) =>
                    position === index ? { ...item, code } : item,
                  ),
                })
              }
            />
            <DecimalField
              label="Precio de presentación"
              value={variant.price}
              onChange={(price) =>
                onChange({
                  ...value,
                  variants: value.variants.map((item, position) =>
                    position === index ? { ...item, price } : item,
                  ),
                })
              }
            />
            <TextField
              label="Código de barras"
              value={variant.barcode ?? ''}
              onChange={(barcode) =>
                onChange({
                  ...value,
                  variants: value.variants.map((item, position) =>
                    position === index ? { ...item, barcode: barcode || null } : item,
                  ),
                })
              }
            />
          </div>
          <Check
            label="Presentación activa"
            value={variant.active}
            onChange={(active) =>
              onChange({
                ...value,
                variants: value.variants.map((item, position) =>
                  position === index ? { ...item, active } : item,
                ),
              })
            }
          />
          {can('catalog.availability.manage') && (
            <ScheduleEditor
              value={variant.availability}
              onChange={(availability) =>
                onChange({
                  ...value,
                  variants: value.variants.map((item, position) =>
                    position === index ? { ...item, availability } : item,
                  ),
                })
              }
            />
          )}
          <RemoveButton
            label="Quitar presentación"
            onClick={() =>
              onChange({
                ...value,
                variants: value.variants.filter((_, position) => position !== index),
              })
            }
          />
        </div>
      ))}
    </RepeatSection>
  );
}

export function IngredientRulesEditor({ value, onChange, lookups }: ProductDetailsProps) {
  return (
    <RepeatSection
      title="Ingredientes y preparación"
      hint="El precio adicional es explícito por unidad que exceda la cantidad incluida. La cantidad de receta prepara el futuro inventario y no genera cargos."
      onAdd={() =>
        onChange({
          ...value,
          ingredients: [
            ...value.ingredients,
            {
              ingredient_id: '',
              included_quantity: '1',
              max_quantity: '3',
              removable: true,
              addable: false,
              extra_price: '0.00',
              recipe_quantity: '0',
            },
          ],
        })
      }
    >
      {value.ingredients.map((ingredient, index) => {
        const update = (changes: Partial<typeof ingredient>) =>
          onChange({
            ...value,
            ingredients: value.ingredients.map((item, position) =>
              position === index ? { ...item, ...changes } : item,
            ),
          });
        return (
          <div className="repeat-card" key={index}>
            <div className="form-grid">
              <ReferenceField
                label="Ingrediente"
                value={ingredient.ingredient_id}
                options={lookups.ingredients}
                onChange={(ingredient_id) => update({ ingredient_id })}
              />
              <DecimalField
                quantity
                label="Cantidad incluida"
                value={ingredient.included_quantity}
                onChange={(included_quantity) => update({ included_quantity })}
              />
              <DecimalField
                quantity
                label="Cantidad máxima"
                value={ingredient.max_quantity}
                onChange={(max_quantity) => update({ max_quantity })}
              />
              <DecimalField
                label="Cargo por unidad adicional"
                value={ingredient.extra_price}
                onChange={(extra_price) => update({ extra_price })}
              />
              <DecimalField
                quantity
                label="Cantidad para receta futura"
                value={ingredient.recipe_quantity}
                onChange={(recipe_quantity) => update({ recipe_quantity })}
              />
            </div>
            <div className="button-row">
              <Check
                label="Se puede retirar"
                value={ingredient.removable}
                onChange={(removable) => update({ removable })}
              />
              <Check
                label="Se puede agregar"
                value={ingredient.addable}
                onChange={(addable) => update({ addable })}
              />
              <RemoveButton
                label="Quitar ingrediente"
                onClick={() =>
                  onChange({
                    ...value,
                    ingredients: value.ingredients.filter((_, position) => position !== index),
                  })
                }
              />
            </div>
          </div>
        );
      })}
    </RepeatSection>
  );
}

export function ProductGroupsEditor({ value, onChange, lookups }: ProductDetailsProps) {
  return (
    <RepeatSection
      title="Grupos de complementos"
      hint="El mínimo habitual es 0: permite retirar o reemplazar todos los originales sin bajar el precio base."
      onAdd={() =>
        onChange({
          ...value,
          groups: [
            ...value.groups,
            { group_id: '', min_quantity: null, max_quantity: null, included: [] },
          ],
        })
      }
    >
      {value.groups.map((binding, index) => {
        const group = lookups.groups.find((entry) => entry.id === binding.group_id);
        const update = (changes: Partial<typeof binding>) =>
          onChange({
            ...value,
            groups: value.groups.map((item, position) =>
              position === index ? { ...item, ...changes } : item,
            ),
          });
        return (
          <div className="repeat-card" key={index}>
            <ReferenceField
              label="Grupo reutilizable"
              value={binding.group_id}
              options={lookups.groups}
              onChange={(group_id) => update({ group_id, included: [] })}
            />
            <div className="form-grid">
              <DecimalField
                quantity
                optional
                label="Mínimo específico"
                value={binding.min_quantity}
                onChange={(min_quantity) => update({ min_quantity: min_quantity || null })}
              />
              <DecimalField
                quantity
                optional
                label="Máximo específico"
                value={binding.max_quantity}
                onChange={(max_quantity) => update({ max_quantity: max_quantity || null })}
              />
            </div>
            {group?.options.map((option) => (
              <DecimalField
                quantity
                key={option.id}
                label={`Incluido: ${option.name} · ${option.portion_name}`}
                value={
                  binding.included.find((entry) => entry.option_id === option.id)?.quantity ?? '0'
                }
                onChange={(quantity) =>
                  update({
                    included: [
                      ...binding.included.filter((entry) => entry.option_id !== option.id),
                      { option_id: option.id, quantity },
                    ],
                  })
                }
              />
            ))}
            <RemoveButton
              label="Quitar grupo"
              onClick={() =>
                onChange({
                  ...value,
                  groups: value.groups.filter((_, position) => position !== index),
                })
              }
            />
          </div>
        );
      })}
    </RepeatSection>
  );
}
