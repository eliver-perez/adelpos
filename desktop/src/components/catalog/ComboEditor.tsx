import type { ComboOption, ComboSlot, ProductReference } from '../../services/catalog';
import {
  DecimalField,
  IntegerField,
  ReferenceField,
  RemoveButton,
  RepeatSection,
  TextField,
  TierEditor,
} from './EditorFields';

// El producto original de un componente siempre está permitido de forma implícita (Go lo agrega si
// falta). Su configuración de tramos vive en la opción cuyo producto coincide con el original; las
// demás opciones son alternativas y la sección de alternativas es opcional.

function originalOption(slot: ComboSlot): ComboOption | undefined {
  if (slot.default_product_id === '') {
    return undefined;
  }
  return slot.options.find((option) => option.product_id === slot.default_product_id);
}

function alternatives(slot: ComboSlot): ComboOption[] {
  const original = originalOption(slot);
  return slot.options.filter((option) => option !== original);
}

// Una opción sin cargo de sustitución ni tramos no lleva configuración propia. El cargo se
// compara por su forma textual («0», «0.00»), sin convertir dinero a número.
function isBlank(option: ComboOption): boolean {
  const noCharge = /^0*(\.0*)?$/.test(option.substitution_price || '0');
  return noCharge && option.tiers.length === 0;
}

// Cambiar el original conserva sus tramos si el nuevo producto ya era una alternativa. El original
// anterior se retira si no tenía configuración; si la tenía, permanece como alternativa.
export function changeOriginal(slot: ComboSlot, productID: string): ComboSlot {
  const previous = originalOption(slot);
  let options = slot.options;
  if (previous && isBlank(previous)) {
    options = options.filter((option) => option !== previous);
  }
  if (productID && !options.some((option) => option.product_id === productID)) {
    options = [{ product_id: productID, substitution_price: '0.00', tiers: [] }, ...options];
  }
  return { ...slot, default_product_id: productID, options };
}

function newSlot(): ComboSlot {
  return {
    id: '',
    name: '',
    min_quantity: 0,
    max_quantity: 2,
    included_quantity: 0,
    default_quantity: 0,
    default_product_id: '',
    options: [],
  };
}

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
      onAdd={() => onChange([...value, newSlot()])}
    >
      {value.map((slot, index) => (
        <ComboSlotEditor
          key={index}
          position={index + 1}
          slot={slot}
          products={products}
          onChange={(next) =>
            onChange(value.map((item, position) => (position === index ? next : item)))
          }
          onRemove={() => onChange(value.filter((_, position) => position !== index))}
        />
      ))}
    </RepeatSection>
  );
}

function ComboSlotEditor({
  position,
  slot,
  products,
  onChange,
  onRemove,
}: {
  position: number;
  slot: ComboSlot;
  products: ProductReference[];
  onChange: (slot: ComboSlot) => void;
  onRemove: () => void;
}) {
  const update = (changes: Partial<ComboSlot>) => onChange({ ...slot, ...changes });
  const original = originalOption(slot);
  const others = alternatives(slot);

  function changeOption(option: ComboOption, changes: Partial<ComboOption>) {
    update({
      options: slot.options.map((item) => (item === option ? { ...item, ...changes } : item)),
    });
  }

  // Las alternativas no pueden repetir el original ni otra alternativa (Go lo rechaza igual).
  function choicesFor(option: ComboOption) {
    return products.filter(
      (product) =>
        product.id === option.product_id ||
        (product.id !== slot.default_product_id &&
          !slot.options.some((item) => item.product_id === product.id)),
    );
  }

  return (
    <div className="repeat-card" aria-label={`Componente ${position}`} role="group">
      <div className="repeat-card-heading">
        <strong>Componente {position}</strong>
        <RemoveButton label="Quitar componente" onClick={onRemove} />
      </div>
      <TextField
        label="Nombre del componente"
        required
        value={slot.name}
        onChange={(name) => update({ name })}
      />
      <div className="form-grid">
        <ReferenceField
          label="Producto original"
          value={slot.default_product_id}
          options={products}
          onChange={(productID) => onChange(changeOriginal(slot, productID))}
        />
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
      </div>
      {original && (
        <TierEditor
          value={original.tiers}
          onChange={(tiers) => changeOption(original, { tiers })}
        />
      )}
      <RepeatSection
        title="Productos permitidos y sustituciones"
        hint="Opcional. El producto original siempre está permitido; agrega aquí solo alternativas."
        onAdd={() =>
          update({
            options: [...slot.options, { product_id: '', substitution_price: '0.00', tiers: [] }],
          })
        }
      >
        {others.map((option, optionIndex) => (
          <div
            key={optionIndex}
            className="repeat-card"
            role="group"
            aria-label={`Alternativa ${optionIndex + 1}`}
          >
            <div className="repeat-card-heading">
              <strong>Alternativa {optionIndex + 1}</strong>
              <RemoveButton
                label="Quitar alternativa"
                onClick={() => update({ options: slot.options.filter((item) => item !== option) })}
              />
            </div>
            <ReferenceField
              label="Producto permitido"
              value={option.product_id}
              options={choicesFor(option)}
              onChange={(product_id) => changeOption(option, { product_id })}
            />
            <DecimalField
              label="Cargo de sustitución por unidad cubierta"
              value={option.substitution_price}
              onChange={(substitution_price) => changeOption(option, { substitution_price })}
            />
            <TierEditor
              value={option.tiers}
              onChange={(tiers) => changeOption(option, { tiers })}
            />
          </div>
        ))}
      </RepeatSection>
    </div>
  );
}
