import type { CSSProperties } from 'react';
import type { Category, Lookups } from '../../../services/catalog';
import { countLabel } from '../../../services/plural';

export interface CaptureCategory {
  category: Category;
  products: number;
}

// Categorías de la captura: activas, disponibles ahora según Go, con al menos un producto
// activo y en su orden visual.
export function captureCategories(lookups: Lookups | null): CaptureCategory[] {
  if (!lookups) {
    return [];
  }
  const counts = new Map<string, number>();
  for (const product of lookups.products) {
    if (product.active && !product.archived) {
      counts.set(product.category_id, (counts.get(product.category_id) ?? 0) + 1);
    }
  }
  return lookups.categories
    .filter((category) => category.active && category.available_now !== false)
    .filter((category) => (counts.get(category.id) ?? 0) > 0)
    .sort(
      (first, second) =>
        first.sort_order - second.sort_order || first.name.localeCompare(second.name, 'es'),
    )
    .map((category) => ({ category, products: counts.get(category.id) ?? 0 }));
}

// Color de marca de la categoría (paleta de la fase B). Sin color, la marca usa el borde del tema
// y el resaltado de la categoría activa usa el color de acento.
export function categoryStyle(color: string | undefined): CSSProperties {
  return {
    '--category-color': color || 'var(--border)',
    '--category-accent': color || 'var(--accent)',
  } as CSSProperties;
}

export function CategoryColumn({
  categories,
  selectedID,
  onSelect,
}: {
  categories: CaptureCategory[];
  selectedID: string;
  onSelect: (id: string) => void;
}) {
  return (
    <nav className="capture-categories" aria-label="Categorías">
      {categories.length === 0 && (
        <p className="muted text-sm capture-empty">
          No hay categorías disponibles en este horario.
        </p>
      )}
      {categories.map(({ category, products }) => (
        <button
          type="button"
          key={category.id}
          className={`capture-category ${selectedID === category.id ? 'capture-category-active' : ''}`}
          style={categoryStyle(category.color)}
          aria-pressed={selectedID === category.id}
          onClick={() => onSelect(category.id)}
        >
          <span className="capture-category-mark" aria-hidden="true" />
          <span className="capture-category-text">
            <strong>{category.name}</strong>
            <span className="muted">{countLabel(products, 'producto', 'productos')}</span>
          </span>
        </button>
      ))}
    </nav>
  );
}
