import type { ComboSlot, ProductReference } from '../../services/catalog';
import {
  DecimalField,
  IntegerField,
  ReferenceField,
  RemoveButton,
  RepeatSection,
  TextField,
  TierEditor,
} from './EditorFields';

export function ComboEditor({
  value,
  onChange,
  products,
}: {
  value: ComboSlot[];
  onChange: (value: ComboSlot[]) => void;
  products: ProductReference[];
}) {
  return (
    <RepeatSection
      title="Componentes del combo"
      hint="Referencias a productos existentes. El precio base del combo cubre las unidades incluidas; los adicionales usan tramos explícitos y conservan su identidad."
      onAdd={() =>
        onChange([
          ...value,
          {
            id: '',
            name: '',
            min_quantity: 0,
            max_quantity: 2,
            included_quantity: 0,
            default_quantity: 0,
            default_product_id: '',
            options: [],
          },
        ])
      }
    >
      {value.map((slot, index) => {
        const update = (changes: Partial<ComboSlot>) =>
          onChange(
            value.map((item, position) => (position === index ? { ...item, ...changes } : item)),
          );
        return (
          <div className="repeat-card" key={index}>
            <TextField
              label="Nombre del componente"
              required
              value={slot.name}
              onChange={(name) => update({ name })}
            />
            <div className="form-grid">
              <IntegerField
                label="Cantidad mínima"
                value={slot.min_quantity}
                max={100}
                onChange={(min_quantity) => update({ min_quantity })}
              />
              <IntegerField
                label="Cantidad máxima"
                value={slot.max_quantity}
                min={1}
                max={100}
                onChange={(max_quantity) => update({ max_quantity })}
              />
              <IntegerField
                label="Unidades cubiertas por precio base"
                value={slot.included_quantity}
                max={100}
                onChange={(included_quantity) => update({ included_quantity })}
              />
              <IntegerField
                label="Cantidad seleccionada originalmente"
                value={slot.default_quantity}
                max={100}
                onChange={(default_quantity) => update({ default_quantity })}
              />
              <ReferenceField
                label="Producto original"
                value={slot.default_product_id}
                options={products}
                onChange={(default_product_id) => update({ default_product_id })}
              />
            </div>
            <RepeatSection
              title="Productos permitidos y sustituciones"
              onAdd={() =>
                update({
                  options: [
                    ...slot.options,
                    { product_id: '', substitution_price: '0.00', tiers: [] },
                  ],
                })
              }
            >
              {slot.options.map((option, optionIndex) => {
                const change = (changes: Partial<typeof option>) =>
                  update({
                    options: slot.options.map((item, position) =>
                      position === optionIndex ? { ...item, ...changes } : item,
                    ),
                  });
                return (
                  <div key={optionIndex} className="repeat-card">
                    <ReferenceField
                      label="Producto permitido"
                      value={option.product_id}
                      options={products}
                      onChange={(product_id) => change({ product_id })}
                    />
                    <DecimalField
                      label="Cargo de sustitución por unidad cubierta"
                      value={option.substitution_price}
                      onChange={(substitution_price) => change({ substitution_price })}
                    />
                    <TierEditor value={option.tiers} onChange={(tiers) => change({ tiers })} />
                    <RemoveButton
                      onClick={() =>
                        update({
                          options: slot.options.filter((_, position) => position !== optionIndex),
                        })
                      }
                    />
                  </div>
                );
              })}
            </RepeatSection>
            <RemoveButton
              onClick={() => onChange(value.filter((_, position) => position !== index))}
            />
          </div>
        );
      })}
    </RepeatSection>
  );
}
