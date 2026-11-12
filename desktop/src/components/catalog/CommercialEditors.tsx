import { Field } from '../Forms';
import type { Lookups, OptionGroup, PriceList, Promotion } from '../../services/catalog';
import {
  Check,
  DecimalField,
  IntegerField,
  ReferenceField,
  RemoveButton,
  RepeatSection,
  ScheduleEditor,
  TierEditor,
} from './EditorFields';
import { MultiSelect, Select } from '../Select';

export function GroupEditor({
  value,
  onChange,
  lookups,
}: {
  value: OptionGroup;
  onChange: (value: OptionGroup) => void;
  lookups: Lookups;
}) {
  return (
    <>
      <Field label="Regla de precio del grupo">
        <Select
          value={value.pricing_mode}
          options={[
            { value: 'value', label: 'Equivalencia por valor incluido' },
            { value: 'quantity', label: 'Tramos por unidades adicionales' },
          ]}
          onChange={(pricing_mode) => onChange({ ...value, pricing_mode })}
        />
      </Field>
      <div className="form-grid">
        <DecimalField
          quantity
          label="Cantidad mínima (0 permite retirar todo)"
          value={value.min_quantity}
          onChange={(min_quantity) => onChange({ ...value, min_quantity })}
        />
        <DecimalField
          quantity
          optional
          label="Cantidad máxima"
          value={value.max_quantity}
          onChange={(max_quantity) => onChange({ ...value, max_quantity: max_quantity || null })}
        />
      </div>
      <RepeatSection
        title="Opciones y porciones"
        hint="El valor de referencia sirve para equivalencia. El cargo extra, separado y explícito, solo se aplica a unidades por encima de las incluidas de esa misma opción. El factor de porción no multiplica automáticamente el precio."
        onAdd={() =>
          onChange({
            ...value,
            options: [
              ...value.options,
              {
                id: '',
                complement_id: '',
                portion_id: null,
                reference_value: '0.00',
                extra_price: '0.00',
                max_quantity: null,
                active: true,
                tiers: [],
              },
            ],
          })
        }
      >
        {value.options.map((option, index) => {
          const update = (changes: Partial<typeof option>) =>
            onChange({
              ...value,
              options: value.options.map((item, position) =>
                position === index ? { ...item, ...changes } : item,
              ),
            });
          return (
            <div className="repeat-card" key={index}>
              <div className="form-grid">
                <ReferenceField
                  label="Complemento"
                  value={option.complement_id}
                  options={lookups.complements}
                  onChange={(complement_id) => update({ complement_id })}
                />
                <ReferenceField
                  optional
                  label="Porción"
                  value={option.portion_id}
                  options={lookups.portions}
                  onChange={(portion_id) => update({ portion_id: portion_id || null })}
                />
                <DecimalField
                  label="Valor de referencia de esta porción"
                  value={option.reference_value}
                  onChange={(reference_value) => update({ reference_value })}
                />
                <DecimalField
                  label="Cargo adicional fuera de equivalencia"
                  value={option.extra_price}
                  onChange={(extra_price) => update({ extra_price })}
                />
                <DecimalField
                  quantity
                  optional
                  label="Cantidad máxima de la opción"
                  value={option.max_quantity}
                  onChange={(max_quantity) => update({ max_quantity: max_quantity || null })}
                />
              </div>
              <Check
                label="Opción activa"
                value={option.active}
                onChange={(active) => update({ active })}
              />
              {value.pricing_mode === 'quantity' && (
                <TierEditor value={option.tiers} onChange={(tiers) => update({ tiers })} />
              )}
              <RemoveButton
                label="Quitar opción"
                onClick={() =>
                  onChange({
                    ...value,
                    options: value.options.filter((_, position) => position !== index),
                  })
                }
              />
            </div>
          );
        })}
      </RepeatSection>
    </>
  );
}

export function PriceListEditor({
  value,
  onChange,
  lookups,
}: {
  value: PriceList;
  onChange: (value: PriceList) => void;
  lookups: Lookups;
}) {
  return (
    <>
      <Check
        label="Lista predeterminada"
        value={value.is_default}
        onChange={(is_default) => onChange({ ...value, is_default })}
      />
      <Check
        label="Permitir promociones que incluyan esta lista"
        value={value.allow_promotions}
        onChange={(allow_promotions) => onChange({ ...value, allow_promotions })}
      />
      <Field label="Cuando no exista un precio específico">
        <Select
          value={value.fallback}
          options={[
            { value: 'base', label: 'Usar precio base de producto/presentación' },
            { value: 'reject', label: 'Rechazar el cálculo y solicitar precio explícito' },
          ]}
          onChange={(fallback) => onChange({ ...value, fallback })}
        />
      </Field>
      <RepeatSection
        title="Precios por producto o presentación"
        onAdd={() =>
          onChange({
            ...value,
            entries: [...value.entries, { product_id: '', variant_id: null, price: '0.00' }],
          })
        }
      >
        {value.entries.map((entry, index) => {
          const update = (changes: Partial<typeof entry>) =>
            onChange({
              ...value,
              entries: value.entries.map((item, position) =>
                position === index ? { ...item, ...changes } : item,
              ),
            });
          const variants =
            lookups.products.find((product) => product.id === entry.product_id)?.variants ?? [];
          return (
            <div className="repeat-card" key={index}>
              <div className="form-grid">
                <ReferenceField
                  label="Producto"
                  value={entry.product_id}
                  options={lookups.products}
                  onChange={(product_id) => update({ product_id, variant_id: null })}
                />
                <ReferenceField
                  optional
                  label="Presentación (vacío: producto base)"
                  value={entry.variant_id}
                  options={variants}
                  onChange={(variant_id) => update({ variant_id: variant_id || null })}
                />
                <DecimalField
                  label="Precio en esta lista"
                  value={entry.price}
                  onChange={(price) => update({ price })}
                />
              </div>
              <RemoveButton
                label="Quitar precio"
                onClick={() =>
                  onChange({
                    ...value,
                    entries: value.entries.filter((_, position) => position !== index),
                  })
                }
              />
            </div>
          );
        })}
      </RepeatSection>
    </>
  );
}

// Alcance de la promoción (etapa 9C, decisión 4 de la Fase 0): selección múltiple con búsqueda en
// lugar de una casilla por producto o categoría. Lo elegido queda a la vista como etiquetas que se
// quitan con un toque.
function ScopeSelector({
  title,
  options,
  selected,
  onChange,
}: {
  title: string;
  options: { id: string; name: string }[];
  selected: string[];
  onChange: (value: string[]) => void;
}) {
  return (
    <Field label={title}>
      <MultiSelect
        value={selected}
        placeholder="Ninguno"
        options={options.map((option) => ({ value: option.id, label: option.name }))}
        onChange={onChange}
      />
    </Field>
  );
}

export function PromotionEditor({
  value,
  onChange,
  lookups,
}: {
  value: Promotion;
  onChange: (value: Promotion) => void;
  lookups: Lookups;
}) {
  return (
    <>
      <Field label="Tipo de promoción automática">
        <Select
          value={value.kind}
          options={[
            { value: 'two_for_one', label: '2 × 1' },
            { value: 'percent', label: 'Descuento porcentual' },
            { value: 'quantity', label: 'Compra una cantidad y paga otra' },
            { value: 'special_price', label: 'Precio especial por unidad' },
          ]}
          onChange={(kind) => onChange({ ...value, kind })}
        />
      </Field>
      <div className="form-grid">
        {value.kind === 'percent' && (
          <DecimalField
            label="Porcentaje (hasta 100)"
            value={value.percent}
            onChange={(percent) => onChange({ ...value, percent })}
          />
        )}
        {value.kind === 'special_price' && (
          <DecimalField
            label="Precio especial"
            value={value.special_price}
            onChange={(special_price) => onChange({ ...value, special_price })}
          />
        )}
        {value.kind === 'quantity' && (
          <>
            <IntegerField
              label="Compra unidades"
              value={value.buy_quantity}
              min={1}
              max={1000}
              onChange={(buy_quantity) => onChange({ ...value, buy_quantity })}
            />
            <IntegerField
              label="Paga unidades"
              value={value.pay_quantity}
              max={1000}
              onChange={(pay_quantity) => onChange({ ...value, pay_quantity })}
            />
          </>
        )}
        <IntegerField
          label="Mínimo de unidades participantes"
          value={value.minimum_quantity}
          min={1}
          max={1000}
          onChange={(minimum_quantity) => onChange({ ...value, minimum_quantity })}
        />
        <IntegerField
          label="Prioridad (mayor primero)"
          value={value.priority}
          max={100000}
          min={-100000}
          onChange={(priority) => onChange({ ...value, priority })}
        />
      </div>
      <Check
        label="Combinable solo con otras promociones también combinables"
        value={value.stackable}
        onChange={(stackable) => onChange({ ...value, stackable })}
      />
      <Check
        label="Aplicar también a personalizaciones y extras del producto"
        value={value.include_extras}
        onChange={(include_extras) => onChange({ ...value, include_extras })}
      />
      <p className="muted text-sm">
        Productos o categorías: basta coincidir con cualquiera. En 2×1/cantidad se bonifican las
        unidades elegibles de menor importe. Los empates se resuelven por posición. Promociones con
        igual prioridad se ordenan por identificador.
      </p>
      <ScopeSelector
        title="Productos participantes"
        options={lookups.products}
        selected={value.product_ids}
        onChange={(product_ids) => onChange({ ...value, product_ids })}
      />
      <ScopeSelector
        title="Categorías participantes"
        options={lookups.categories}
        selected={value.category_ids}
        onChange={(category_ids) => onChange({ ...value, category_ids })}
      />
      <ScopeSelector
        title="Listas participantes (vacío: solo predeterminada)"
        options={lookups['price-lists']}
        selected={value.price_list_ids}
        onChange={(price_list_ids) => onChange({ ...value, price_list_ids })}
      />
      <ScheduleEditor
        value={value.availability}
        onChange={(availability) => onChange({ ...value, availability })}
      />
    </>
  );
}
