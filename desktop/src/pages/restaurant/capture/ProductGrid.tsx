import { useEffect, useState } from 'react';
import { CatalogImage } from '../../../components/catalog/EditorFields';
import { Notice } from '../../../components/Forms';
import { Icon } from '../../../components/Icon';
import { shortcutHint } from '../../../commands/hints';
import { ToggleButton } from '../../../components/ToggleButton';
import { useLiveResource } from '../../../hooks/useLiveResource';
import type { Lookups } from '../../../services/catalog';
import { formatMoney } from '../../../services/money';
import type { Account } from '../../../services/restaurant';
import { categoryStyle } from './CategoryColumn';
import { productSelectable, requiresChoices } from './captureRules';
import type { CaptureProduct } from './captureRules';

const pageSize = 100;

// Precio mostrado en el botón: siempre el que calculó Go para la lista de la cuenta.
function priceLabel(product: CaptureProduct) {
  if (product.effective_price) {
    return formatMoney(product.effective_price);
  }
  if (product.pricing_error) {
    return product.variants.some((variant) => variant.active)
      ? 'Elegir presentación'
      : 'Sin precio';
  }
  return formatMoney(product.base_price);
}

export function ProductGrid({
  account,
  lookups,
  categoryID,
  search,
  onSearch,
  showImages,
  onShowImages,
  enabled,
  active,
  writable,
  onAdd,
  onCustomize,
}: {
  account: Account | null;
  lookups: Lookups | null;
  categoryID: string;
  search: string;
  onSearch: (value: string) => void;
  showImages: boolean;
  onShowImages: (value: boolean) => void;
  enabled: boolean;
  active: boolean;
  writable: boolean;
  onAdd: (product: CaptureProduct) => void;
  onCustomize: (product: CaptureProduct) => void;
}) {
  const [page, setPage] = useState(1);
  useEffect(() => setPage(1), [categoryID]);
  const searching = search.trim().length > 0;
  const query: Record<string, string> = {
    page: String(page),
    page_size: String(pageSize),
    archived: 'false',
    active: 'true',
  };
  if (searching) {
    query.search = search.trim();
  } else {
    query.category_id = categoryID;
  }
  // Una cuenta pendiente sin lista conocida consulta los precios de la lista predeterminada.
  if (account?.price_list_id) query.price_list_id = account.price_list_id;
  const products = useLiveResource<{ items: CaptureProduct[]; total: number }>('catalog_products', {
    query,
    enabled: enabled && active && !!account && (searching || !!categoryID),
    interval: 10000,
  });
  const colors = new Map(lookups?.categories.map((entry) => [entry.id, entry.color]) ?? []);
  const groups = lookups?.groups ?? [];

  return (
    <section className="capture-products" aria-label="Productos">
      <div className="capture-products-toolbar">
        <label className="capture-search">
          <input
            type="search"
            value={search}
            maxLength={100}
            placeholder="Buscar por nombre o código…"
            aria-label="Buscar producto por nombre o código"
            aria-keyshortcuts={shortcutHint('edit.find')?.aria}
            data-screen-search
            onChange={(event) => {
              onSearch(event.target.value);
              setPage(1);
            }}
          />
        </label>
        <ToggleButton label="Mostrar imágenes" pressed={showImages} onChange={onShowImages} />
      </div>
      <Notice>{products.error}</Notice>
      {!enabled && <p className="muted">La captura requiere permisos de catálogo y precios.</p>}
      {enabled && !searching && !categoryID && (
        <p className="muted capture-empty">Elige una categoría para ver sus productos.</p>
      )}
      {products.loading && !products.data && (
        <p className="muted capture-empty">Consultando productos…</p>
      )}
      <div className="capture-product-grid">
        {products.data?.items.map((product) => {
          const selectable = productSelectable(product) && writable && !products.error;
          const needsChoices = requiresChoices(product, groups);
          return (
            <div
              className={`capture-product ${showImages && product.image_id ? 'capture-product-with-image' : ''}`}
              key={product.id}
              style={categoryStyle(colors.get(product.category_id))}
            >
              <button
                type="button"
                className="capture-product-main"
                disabled={!selectable}
                onClick={() => (needsChoices ? onCustomize(product) : onAdd(product))}
              >
                <span className="capture-product-stripe" aria-hidden="true" />
                {showImages && product.image_id && (
                  <CatalogImage id={product.image_id} decorative />
                )}
                <strong className="capture-product-name">{product.name}</strong>{' '}
                <span className="capture-product-price">{priceLabel(product)}</span>{' '}
                {needsChoices && <span className="capture-product-tag">Opciones</span>}
                {!product.available_now && (
                  <span className="product-unavailable">{product.availability_reason}</span>
                )}
              </button>
              {selectable && !needsChoices && (
                <button
                  type="button"
                  className="capture-product-customize"
                  aria-label={`Personalizar ${product.name}`}
                  onClick={() => onCustomize(product)}
                >
                  <Icon name="edit" size={20} />
                </button>
              )}
            </div>
          );
        })}
      </div>
      {products.data?.total === 0 && (
        <p className="muted capture-empty">
          {searching
            ? 'No hay productos que coincidan con esta búsqueda.'
            : 'Esta categoría no tiene productos disponibles.'}
        </p>
      )}
      {products.data && products.data.total > pageSize && (
        <div className="operation-product-pagination">
          <button
            className="secondary-button"
            disabled={page <= 1}
            onClick={() => setPage(page - 1)}
          >
            Anterior
          </button>
          <span className="muted text-xs">Página {page}</span>
          <button
            className="secondary-button"
            disabled={page * pageSize >= products.data.total}
            onClick={() => setPage(page + 1)}
          >
            Siguiente
          </button>
        </div>
      )}
    </section>
  );
}
