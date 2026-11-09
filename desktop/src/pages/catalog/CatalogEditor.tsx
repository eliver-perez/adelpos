import { useState } from 'react';
import { Field, Form } from '../../components/Forms';
import {
  Check,
  DecimalField,
  ReferenceField,
  ScheduleEditor,
  TextField,
} from '../../components/catalog/EditorFields';
import { ColorField } from '../../components/catalog/ColorField';
import { PositionField } from '../../components/PositionField';
import { categorySiblings } from '../../services/catalogPositions';
import { suggestProductCode } from '../../services/productCode';
import { ImageEditor } from '../../components/catalog/ImageEditor';
import { ProductEditor } from '../../components/catalog/ProductEditor';
import {
  GroupEditor,
  PriceListEditor,
  PromotionEditor,
} from '../../components/catalog/CommercialEditors';
import type {
  CatalogDocument,
  Category,
  Complement,
  Ingredient,
  Lookups,
  OptionGroup,
  Portion,
  PriceList,
  Product,
  Promotion,
  Resource,
} from '../../services/catalog';

export function CatalogEditor({
  resource,
  value,
  onChange,
  lookups,
  can,
  onSave,
  onSaveAndNew,
  saveAndNewLabel,
  editable,
  focusName = false,
}: {
  resource: Resource;
  value: CatalogDocument;
  onChange: (value: CatalogDocument) => void;
  lookups: Lookups;
  can: (permission: string) => boolean;
  onSave: () => Promise<void>;
  // Alta continua: guarda y abre un documento nuevo del mismo recurso. Devuelve el texto del aviso.
  onSaveAndNew?: () => Promise<string | void>;
  // Etiqueta con la concordancia del recurso («otra categoría», «otro producto»).
  saveAndNewLabel?: string;
  editable: boolean;
  // Enfoca el nombre al abrir (después de «Guardar y crear otra»).
  focusName?: boolean;
}) {
  // En el alta de productos y combos el código interno se sugiere a partir del nombre hasta que
  // el usuario lo edita a mano. Al editar un producto existente nunca cambia.
  const [codeEdited, setCodeEdited] = useState(false);
  const suggestCode = resource === 'products' && !value.id && !codeEdited;

  function changeName(name: string) {
    if (suggestCode) {
      const product = value as Product;
      const code = suggestProductCode(
        name,
        lookups.products.map((entry) => entry.code),
      );
      onChange({ ...product, name, code });
      return;
    }
    onChange({ ...value, name });
  }

  function fields() {
    switch (resource) {
      case 'products':
        return (
          <ProductEditor
            value={value as Product}
            onChange={onChange}
            lookups={lookups}
            can={can}
            onCodeEdited={() => setCodeEdited(true)}
          />
        );
      case 'categories': {
        const category = value as Category;
        return (
          <>
            <div className="form-grid">
              <PositionField
                label="Posición de la categoría"
                siblings={categorySiblings(lookups)}
                recordID={category.id}
                value={category.position?.after}
                onChange={(after) => onChange({ ...category, position: { after } })}
              />
              <ColorField
                value={category.color}
                palette={lookups.palette ?? []}
                onChange={(color) => onChange({ ...category, color })}
              />
            </div>
            {can('catalog.images.manage') && (
              <ImageEditor
                id={category.image_id}
                onChange={(image_id) => onChange({ ...category, image_id })}
              />
            )}
            {can('catalog.availability.manage') && (
              <ScheduleEditor
                value={category.availability}
                onChange={(availability) => onChange({ ...category, availability })}
              />
            )}
          </>
        );
      }
      case 'ingredients':
        return (
          <TextField
            label="Unidad de preparación"
            required
            value={(value as Ingredient).unit}
            onChange={(unit) => onChange({ ...(value as Ingredient), unit })}
          />
        );
      case 'complements': {
        const complement = value as Complement;
        return (
          <>
            <p className="muted text-sm">
              Un complemento puede ser independiente o referenciar un producto o ingrediente
              existente. No tiene un precio global implícito.
            </p>
            <ReferenceField
              optional
              label="Producto relacionado"
              value={complement.product_id}
              options={lookups.products}
              onChange={(product_id) =>
                onChange({ ...complement, product_id: product_id || null, ingredient_id: null })
              }
            />
            <ReferenceField
              optional
              label="Ingrediente relacionado"
              value={complement.ingredient_id}
              options={lookups.ingredients}
              onChange={(ingredient_id) =>
                onChange({ ...complement, ingredient_id: ingredient_id || null, product_id: null })
              }
            />
          </>
        );
      }
      case 'portions':
        return (
          <DecimalField
            quantity
            label="Factor físico de porción (mayor que 0)"
            value={(value as Portion).factor}
            onChange={(factor) => onChange({ ...(value as Portion), factor })}
          />
        );
      case 'groups':
        return <GroupEditor value={value as OptionGroup} onChange={onChange} lookups={lookups} />;
      case 'price_lists':
        return <PriceListEditor value={value as PriceList} onChange={onChange} lookups={lookups} />;
      case 'promotions':
        return <PromotionEditor value={value as Promotion} onChange={onChange} lookups={lookups} />;
    }
  }
  return (
    <Form
      onSave={onSave}
      onSaveAndNew={onSaveAndNew}
      saveAndNewLabel={saveAndNewLabel}
      disabled={!editable}
      label={value.id ? 'Guardar configuración' : 'Crear registro'}
    >
      <div className="form-grid">
        <TextField
          label="Nombre"
          required
          autoFocus={focusName}
          value={value.name}
          onChange={changeName}
        />
        {(resource !== 'products' || can('catalog.products.state') || !value.id) && (
          <Check
            label="Activo"
            value={value.active}
            onChange={(active) => onChange({ ...value, active })}
          />
        )}
      </div>
      <Field label="Descripción opcional">
        <textarea
          rows={2}
          maxLength={2000}
          value={value.description}
          onChange={(event) => onChange({ ...value, description: event.target.value })}
        />
      </Field>
      {fields()}
    </Form>
  );
}
