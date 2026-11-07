import type { Lookups, Product } from '../../services/catalog';
import {
  Check,
  DecimalField,
  ImageEditor,
  IntegerField,
  ReferenceField,
  ScheduleEditor,
  TextField,
} from './EditorFields';
import { VariantEditor, IngredientRulesEditor, ProductGroupsEditor } from './ProductDetailsEditors';
import { ComboEditor } from './ComboEditor';

export function ProductEditor({
  value,
  onChange,
  lookups,
  can,
}: {
  value: Product;
  onChange: (value: Product) => void;
  lookups: Lookups;
  can: (permission: string) => boolean;
}) {
  return (
    <>
      <div className="form-grid">
        <ReferenceField
          label="Categoría"
          value={value.category_id}
          options={lookups.categories}
          onChange={(category_id) => onChange({ ...value, category_id })}
        />
        <TextField
          label="Código interno único"
          required
          value={value.code}
          onChange={(code) => onChange({ ...value, code })}
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
        <IntegerField
          label="Orden visual"
          value={value.sort_order}
          onChange={(sort_order) => onChange({ ...value, sort_order })}
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
