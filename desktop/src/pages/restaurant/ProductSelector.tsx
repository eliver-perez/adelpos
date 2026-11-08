import { useState } from 'react';
import { CatalogImage } from '../../components/catalog/EditorFields';
import { Field, Notice } from '../../components/Forms';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { Lookups, Product } from '../../services/catalog';
import type { Account } from '../../services/restaurant';

interface CaptureProduct extends Product {
  available_now: boolean;
  availability_reason: string;
  effective_price?: string;
  price_origin?: string;
  pricing_error?: string;
}

export function ProductSelector({
  account,
  lookups,
  enabled,
  active,
  onSelect,
}: {
  account: Account | null;
  lookups: Lookups | null;
  enabled: boolean;
  active: boolean;
  onSelect: (product: Product) => void;
}) {
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('');
  const [page, setPage] = useState(1);
  const query: Record<string, string> = {
    search,
    category_id: category,
    page: String(page),
    page_size: '20',
    archived: 'false',
    active: 'true',
  };
  if (account) query.price_list_id = account.price_list_id;
  const products = useLiveResource<{ items: CaptureProduct[]; total: number }>('catalog_products', {
    query,
    enabled: enabled && active,
    interval: 10000,
  });

  return (
    <section className="operation-products">
      <div className="product-search-row">
        <Field label="Buscar producto">
          <input
            type="search"
            value={search}
            maxLength={100}
            placeholder="Nombre del producto…"
            onChange={(event) => {
              setSearch(event.target.value);
              setPage(1);
            }}
          />
        </Field>
        <button className="secondary-button" onClick={() => void products.reload()}>
          Actualizar
        </button>
      </div>
      <div className="category-strip" role="group" aria-label="Categorías de captura">
        <button
          type="button"
          aria-pressed={!category}
          className={!category ? 'selected-category' : ''}
          onClick={() => {
            setCategory('');
            setPage(1);
          }}
        >
          Todas
        </button>
        {lookups?.categories
          .filter((entry) => entry.active)
          .sort((first, second) => first.sort_order - second.sort_order)
          .map((entry) => (
            <button
              key={entry.id}
              type="button"
              aria-pressed={category === entry.id}
              className={category === entry.id ? 'selected-category' : ''}
              onClick={() => {
                setCategory(entry.id);
                setPage(1);
              }}
            >
              {entry.name}
            </button>
          ))}
      </div>
      <Notice>{products.error}</Notice>
      {!enabled && <p className="muted">La captura requiere permisos de catálogo y precios.</p>}
      {!account && (
        <p className="muted text-sm">Selecciona o abre una cuenta para capturar productos.</p>
      )}
      {products.loading && !products.data && <p className="muted">Consultando productos…</p>}
      <div className="operation-product-grid">
        {products.data?.items.map((product) => {
          const noPrice =
            !!product.pricing_error && !product.variants.some((variant) => variant.active);
          const disabled =
            !account ||
            account.state !== 'open' ||
            !product.available_now ||
            noPrice ||
            !!products.error;
          return (
            <button
              type="button"
              className="operation-product-card"
              key={product.id}
              disabled={disabled}
              onClick={() => onSelect(product)}
              title={product.availability_reason || product.pricing_error || product.price_origin}
            >
              {product.image_id && <CatalogImage id={product.image_id} />}
              <strong>{product.name}</strong>
              <span className="product-card-price">
                {product.effective_price
                  ? `$${product.effective_price}`
                  : product.pricing_error
                    ? 'Elegir presentación'
                    : `$${product.base_price}`}
              </span>
              {product.kind === 'combo' && (
                <span className="muted text-xs">Combo personalizable</span>
              )}
              {product.variants.length > 0 && (
                <span className="muted text-xs">Presentaciones disponibles</span>
              )}
              {!product.available_now && (
                <span className="product-unavailable">{product.availability_reason}</span>
              )}
              {noPrice && <span className="product-unavailable">{product.pricing_error}</span>}
            </button>
          );
        })}
      </div>
      {products.data?.total === 0 && (
        <p className="muted">No hay productos que coincidan con esta búsqueda.</p>
      )}
      <div className="operation-product-pagination">
        <button className="secondary-button" disabled={page <= 1} onClick={() => setPage(page - 1)}>
          Anterior
        </button>
        <span className="muted text-xs">Página {page}</span>
        <button
          className="secondary-button"
          disabled={!products.data || page * 20 >= products.data.total}
          onClick={() => setPage(page + 1)}
        >
          Siguiente
        </button>
      </div>
    </section>
  );
}
