import { useState } from 'react';
import { CatalogImage } from '../../components/catalog/EditorFields';
import { Field } from '../../components/Forms';
import { api } from '../../services/api';
import type { CatalogDocument, Category, Product, Resource } from '../../services/catalog';
import { formatMoney } from '../../services/money';
import { followsSuggestion, suggestProductCode } from '../../services/productCode';

export function CatalogRecord({
  record,
  resource,
  categoryName,
  editable,
  can,
  onOpen,
  onViewProducts,
  onMoveUp,
  runAction,
  existingCodes = [],
}: {
  record: CatalogDocument;
  resource: Resource;
  categoryName: string;
  editable: boolean;
  can: (permission: string) => boolean;
  onOpen: (id: string) => void;
  onViewProducts: () => void;
  onMoveUp?: () => void;
  // Ejecuta la acción, confirma el resultado con un aviso y devuelve si Go la aceptó.
  runAction: (action: () => Promise<unknown>, success?: string) => Promise<boolean>;
  // Códigos de producto conocidos, para sugerir el de la copia (etapa 9A).
  existingCodes?: string[];
}) {
  const product =
    resource === 'products' ? (record as Product & { availability_reason?: string }) : null;
  const [duplicating, setDuplicating] = useState(false);
  return (
    <article className="catalog-record" key={record.id}>
      {(resource === 'products' || resource === 'categories') && (
        <CatalogImage id={(record as Product | Category).image_id} />
      )}
      <div className="catalog-record-body">
        <h3>{record.name}</h3>
        <p className="muted text-sm">
          {product ? `${product.code} · ${categoryName}` : record.description}
        </p>
        <span className={`status-pill ${record.active ? 'status-good' : 'status-muted'}`}>
          {record.active ? 'Activo' : 'Inactivo'}
        </span>
        {product?.archived && <span className="status-pill status-muted">Archivado</span>}
        {product && can('catalog.prices.read') && (
          <strong className="catalog-price">{formatMoney(product.base_price)}</strong>
        )}
        {product?.availability_reason && (
          <p className="muted text-xs">{product.availability_reason}</p>
        )}
      </div>
      <div className="button-row">
        <button className="secondary-button" onClick={() => void onOpen(record.id)}>
          {editable ? 'Editar' : 'Consultar'}
        </button>
        {resource === 'categories' && (
          <button className="secondary-button" onClick={onViewProducts}>
            Ver productos
          </button>
        )}
        {onMoveUp && (
          <button
            className="secondary-button"
            aria-label={`Subir ${record.name}`}
            onClick={onMoveUp}
          >
            ↑
          </button>
        )}
        {product && can('catalog.products.state') && (
          <button
            className="secondary-button"
            onClick={() =>
              void runAction(
                () =>
                  api('catalog_product_state', {
                    id: product.id,
                    body: {
                      revision: product.revision,
                      active: product.active,
                      archived: !product.archived,
                    },
                  }),
                product.archived ? `«${product.name}» restaurado.` : `«${product.name}» archivado.`,
              )
            }
          >
            {product.archived ? 'Restaurar' : 'Archivar'}
          </button>
        )}
        {product && can('catalog.products.state') && (
          <button
            className="secondary-button"
            onClick={() =>
              void runAction(
                () =>
                  api('catalog_product_state', {
                    id: product.id,
                    body: {
                      revision: product.revision,
                      active: !product.active,
                      archived: product.archived,
                    },
                  }),
                product.active ? `«${product.name}» desactivado.` : `«${product.name}» activado.`,
              )
            }
          >
            {product.active ? 'Desactivar' : 'Activar'}
          </button>
        )}
        {product && can('catalog.availability.manage') && (
          <button
            className="secondary-button"
            onClick={() =>
              void runAction(
                () =>
                  api('catalog_product_availability', {
                    id: product.id,
                    body: {
                      revision: product.revision,
                      temporary_unavailable: !product.temporary_unavailable,
                      availability: product.availability,
                    },
                  }),
                product.temporary_unavailable
                  ? `«${product.name}» vuelve a estar disponible.`
                  : `«${product.name}» quedó pausado.`,
              )
            }
          >
            {product.temporary_unavailable ? 'Disponible' : 'Pausar'}
          </button>
        )}
        {product && editable && (
          <button
            type="button"
            className="secondary-button"
            aria-expanded={duplicating}
            onClick={() => setDuplicating(!duplicating)}
          >
            Duplicar
          </button>
        )}
      </div>
      {product && duplicating && (
        <DuplicateProductForm
          product={product}
          existingCodes={existingCodes}
          onCancel={() => setDuplicating(false)}
          onDuplicate={async (name, code) => {
            const duplicated = await runAction(
              () =>
                api('catalog_duplicate_product', {
                  id: product.id,
                  body: { name, code, revision: product.revision },
                }),
              `Copia «${name}» creada.`,
            );
            // Si Go rechaza la copia, el formulario sigue abierto con lo capturado.
            if (duplicated) {
              setDuplicating(false);
            }
          }}
        />
      )}
    </article>
  );
}

// Datos de la copia capturados dentro de la tarjeta. Sustituye a window.prompt, que el WebView
// nativo no muestra (el botón «Duplicar» no hacía nada). El código interno se sugiere a partir del
// nombre de la copia con la regla de la etapa 6.1 (etapa 9A) y sigue al nombre mientras no se edite;
// Go valida que sea único entre productos y presentaciones.
function DuplicateProductForm({
  product,
  existingCodes,
  onCancel,
  onDuplicate,
}: {
  product: Product;
  existingCodes: string[];
  onCancel: () => void;
  onDuplicate: (name: string, code: string) => Promise<void>;
}) {
  const suggest = (value: string) => suggestProductCode(value, existingCodes);
  const [name, setName] = useState(`${product.name} (copia)`);
  const [code, setCode] = useState(() => suggest(`${product.name} (copia)`));

  function changeName(value: string) {
    if (followsSuggestion(code, suggest(name))) {
      setCode(suggest(value));
    }
    setName(value);
  }

  return (
    <form
      className="duplicate-form"
      aria-label={`Duplicar ${product.name}`}
      onSubmit={(event) => {
        event.preventDefault();
        void onDuplicate(name.trim(), code.trim());
      }}
    >
      <Field label="Nombre de la copia">
        <input
          value={name}
          required
          maxLength={100}
          onChange={(event) => changeName(event.target.value)}
        />
      </Field>
      <Field
        label="Código interno de la copia"
        hint="Se sugiere a partir del nombre mientras no lo edites."
      >
        <input
          value={code}
          required
          maxLength={60}
          onChange={(event) => setCode(event.target.value)}
        />
      </Field>
      <div className="button-row">
        <button type="submit" className="primary-button">
          Crear copia
        </button>
        <button type="button" className="secondary-button" onClick={onCancel}>
          Cancelar
        </button>
      </div>
    </form>
  );
}
