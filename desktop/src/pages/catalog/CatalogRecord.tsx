import { CatalogImage } from '../../components/catalog/EditorFields';
import { api } from '../../services/api';
import type { CatalogDocument, Category, Product, Resource } from '../../services/catalog';

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
}: {
  record: CatalogDocument;
  resource: Resource;
  categoryName: string;
  editable: boolean;
  can: (permission: string) => boolean;
  onOpen: (id: string) => void;
  onViewProducts: () => void;
  onMoveUp?: () => void;
  runAction: (action: () => Promise<unknown>) => Promise<void>;
}) {
  const product =
    resource === 'products' ? (record as Product & { availability_reason?: string }) : null;
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
          <strong className="catalog-price">${product.base_price}</strong>
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
              void runAction(() =>
                api('catalog_product_state', {
                  id: product.id,
                  body: {
                    revision: product.revision,
                    active: product.active,
                    archived: !product.archived,
                  },
                }),
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
              void runAction(() =>
                api('catalog_product_state', {
                  id: product.id,
                  body: {
                    revision: product.revision,
                    active: !product.active,
                    archived: product.archived,
                  },
                }),
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
              void runAction(() =>
                api('catalog_product_availability', {
                  id: product.id,
                  body: {
                    revision: product.revision,
                    temporary_unavailable: !product.temporary_unavailable,
                    availability: product.availability,
                  },
                }),
              )
            }
          >
            {product.temporary_unavailable ? 'Disponible' : 'Pausar'}
          </button>
        )}
        {product && editable && (
          <button
            className="secondary-button"
            onClick={() => {
              const name = window.prompt('Nombre del nuevo producto', `${product.name} (copia)`);
              if (!name) return;
              const code = window.prompt('Código interno único para la copia');
              if (!code) return;
              void runAction(() =>
                api('catalog_duplicate_product', {
                  id: product.id,
                  body: { name, code, revision: product.revision },
                }),
              );
            }}
          >
            Duplicar
          </button>
        )}
      </div>
    </article>
  );
}
