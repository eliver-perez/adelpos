import type { Lookups, Product, ProductTaxRate } from '../../services/catalog';
import { productSiblings } from '../../services/catalogPositions';
import { PositionField } from '../PositionField';
import { SegmentedControl } from '../SegmentedControl';
import { Check, DecimalField, ReferenceField, ScheduleEditor, TextField } from './EditorFields';
import { ImageEditor } from './ImageEditor';
import { VariantEditor, IngredientRulesEditor, ProductGroupsEditor } from './ProductDetailsEditors';
import { ComboEditor } from './ComboEditor';

type TaxChoice = 'general' | '16' | '8' | '0';

const taxChoices: { value: TaxChoice; label: string }[] = [
  { value: 'general', label: 'Tasa general del establecimiento' },
  { value: '16', label: '16 %' },
  { value: '8', label: '8 %' },
  { value: '0', label: '0 %' },
];

function taxChoice(rate: ProductTaxRate | null | undefined): TaxChoice {
  if (rate === 16 || rate === 8 || rate === 0) {
    return String(rate) as TaxChoice;
  }
  return 'general';
}

function choiceRate(choice: TaxChoice): ProductTaxRate | null {
  if (choice === '16') {
    return 16;
  }
  if (choice === '8') {
    return 8;
  }
  if (choice === '0') {
    return 0;
  }
  return null;
}

// Tasa de IVA del producto (y del combo, que la aplica a todo su importe): botones de una lista
// cerrada, sin captura libre. Cambiarla exige el permiso de precios, como el precio, y Go la
// audita con el guardado. Solo afecta a lo que se confirme después.
export function ProductTaxField({
  value,
  onChange,
  editable,
}: {
  value: ProductTaxRate | null | undefined;
  onChange: (rate: ProductTaxRate | null) => void;
  editable: boolean;
}) {
  return (
    <div className="field product-tax-field">
      <span>Tasa de IVA</span>
      <SegmentedControl<TaxChoice>
        label="Tasa de IVA del producto"
        value={taxChoice(value)}
        onChange={(choice) => onChange(choiceRate(choice))}
        disabled={!editable}
        options={taxChoices}
      />
      <small className="muted">
        {editable
          ? 'Los precios incluyen IVA. La tasa general es la configurada en Cobros; cambiar la tasa solo afecta a los pedidos que se confirmen después.'
          : 'Cambiar la tasa requiere el permiso «Modificar precios y valores comerciales».'}
      </small>
    </div>
  );
}

export function ProductEditor({
  value,
  onChange,
  lookups,
  can,
  onCodeEdited,
}: {
  value: Product;
  onChange: (value: Product) => void;
  lookups: Lookups;
  can: (permission: string) => boolean;
  // Avisa que el código se editó a mano: desde entonces deja de sugerirse a partir del nombre.
  onCodeEdited?: () => void;
}) {
  return (
    <>
      <div className="form-grid">
        <ReferenceField
          label="Categoría"
          value={value.category_id}
          options={lookups.categories}
          // Otra categoría es otra lista de posiciones: la elección anterior deja de aplicar y se
          // propone «después del último» de la nueva categoría.
          onChange={(category_id) => onChange({ ...value, category_id, position: undefined })}
        />
        <TextField
          label="Código interno único"
          required
          value={value.code}
          hint={value.id ? undefined : 'Se sugiere a partir del nombre mientras no lo edites.'}
          onChange={(code) => {
            onCodeEdited?.();
            onChange({ ...value, code });
          }}
        />
        <TextField
          label="Código de barras opcional"
          value={value.barcode ?? ''}
          onChange={(barcode) => onChange({ ...value, barcode: barcode || null })}
        />
        <DecimalField
          label="Precio base anunciado"
          value={value.base_price}
          onChange={(base_price) => onChange({ ...value, base_price })}
        />
        <ProductTaxField
          value={value.tax_rate}
          onChange={(tax_rate) => onChange({ ...value, tax_rate })}
          editable={can('catalog.prices.manage')}
        />
        <PositionField
          label="Posición en su categoría"
          siblings={productSiblings(lookups, value.category_id)}
          recordID={value.id}
          value={value.position?.after}
          onChange={(after) => onChange({ ...value, position: { after } })}
          hint={value.category_id ? undefined : 'Elige primero la categoría.'}
        />
      </div>
      {can('catalog.products.state') && (
        <Check
          label="Archivado"
          value={value.archived}
          onChange={(archived) => onChange({ ...value, archived })}
        />
      )}
      {can('catalog.images.manage') && (
        <ImageEditor
          id={value.image_id}
          onChange={(image_id) => onChange({ ...value, image_id })}
        />
      )}
      {can('catalog.availability.manage') && (
        <>
          <Check
            label="No disponible temporalmente"
            value={value.temporary_unavailable}
            onChange={(temporary_unavailable) => onChange({ ...value, temporary_unavailable })}
          />
          <ScheduleEditor
            value={value.availability}
            onChange={(availability) => onChange({ ...value, availability })}
          />
        </>
      )}
      <VariantEditor value={value} onChange={onChange} lookups={lookups} can={can} />
      <IngredientRulesEditor value={value} onChange={onChange} lookups={lookups} can={can} />
      <ProductGroupsEditor value={value} onChange={onChange} lookups={lookups} can={can} />
      {value.kind === 'combo' && can('catalog.combos.manage') && (
        <ComboEditor
          value={value.slots}
          onChange={(slots) => onChange({ ...value, slots })}
          products={lookups.products.filter((product) => product.id !== value.id)}
        />
      )}
    </>
  );
}
