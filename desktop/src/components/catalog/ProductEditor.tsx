import type { Lookups, Product } from '../../services/catalog';
import { productSiblings } from '../../services/catalogPositions';
import { PositionField } from '../PositionField';
import { Check, DecimalField, ReferenceField, ScheduleEditor, TextField } from './EditorFields';
import { ImageEditor } from './ImageEditor';
import { VariantEditor, IngredientRulesEditor, ProductGroupsEditor } from './ProductDetailsEditors';
import { ComboEditor } from './ComboEditor';

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
