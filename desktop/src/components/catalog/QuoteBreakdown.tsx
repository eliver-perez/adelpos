import type { ProductBreakdown, Quote } from '../../services/catalog';

function ProductDetails({ value }: { value: ProductBreakdown }) {
  return (
    <div className="quote-product-details">
      <p className="muted text-xs">
        {value.price_origin} · Configuración r{value.revision}
      </p>
      <dl className="price-breakdown">
        <dt>Base</dt>
        <dd>${value.base_price}</dd>
        <dt>Ingredientes</dt>
        <dd>+ ${value.ingredient_extras}</dd>
        <dt>Complementos</dt>
        <dd>+ ${value.group_extras}</dd>
        <dt>Componentes</dt>
        <dd>+ ${value.component_extras}</dd>
        <dt>Precio por unidad</dt>
        <dd>${value.unit_price}</dd>
      </dl>
      {value.groups.map((group) => (
        <details key={group.group_id} open className="quote-group">
          <summary>{group.name}</summary>
          <dl className="price-breakdown">
            <dt>Valor original incluido</dt>
            <dd>${group.original_value}</dd>
            <dt>Valor solicitado</dt>
            <dd>${group.requested_value}</dd>
            <dt>{group.mode === 'value' ? 'Diferencia positiva' : 'Cargo por tramos'}</dt>
            <dd>${group.difference}</dd>
            <dt>Extras explícitos separados</dt>
            <dd>${group.extras}</dd>
          </dl>
          {group.options
            .filter((option) => option.requested !== '0' || option.included !== '0')
            .map((option) => (
              <p className="muted text-xs" key={option.option_id}>
                {option.name}: {option.included} incluido → {option.requested} solicitado
              </p>
            ))}
        </details>
      ))}
      {value.ingredients.map((ingredient) => (
        <p className="muted text-xs" key={ingredient.ingredient_id}>
          {ingredient.name}: {ingredient.included} incluido → {ingredient.requested} solicitado · +$
          {ingredient.extra}
        </p>
      ))}
      {value.components.map((component) => (
        <details key={component.slot_id}>
          <summary>
            {component.name} × {component.quantity} · +${component.charge}
          </summary>
          {component.product && <ProductDetails value={component.product} />}
        </details>
      ))}
      {value.notes && <p className="muted text-sm">Preparación: {value.notes}</p>}
    </div>
  );
}

export function QuoteBreakdown({ value }: { value: Quote }) {
  return (
    <div className="form-stack">
      <div>
        <p className="eyebrow">Cálculo confirmado por Go</p>
        <h2 className="quote-total">${value.total}</h2>
        <p className="muted text-xs">
          {value.price_list_name} · {value.timezone}
        </p>
      </div>
      <dl className="price-breakdown">
        <dt>Subtotal</dt>
        <dd>${value.subtotal}</dd>
        <dt>Promociones automáticas</dt>
        <dd>− ${value.discount}</dd>
        <dt>Total de previsualización</dt>
        <dd>${value.total}</dd>
      </dl>
      {value.lines.map((line, index) => (
        <section className="catalog-section" key={index}>
          <h3>
            {line.name} × {line.quantity}
          </h3>
          <ProductDetails value={line} />
          <p className="text-sm">
            Subtotal ${line.subtotal} · Descuento ${line.discount} · Total ${line.total}
          </p>
        </section>
      ))}
      {value.promotions.map((promotion) => (
        <div
          className={promotion.applied ? 'notice notice-success' : 'quote-promotion'}
          key={promotion.id}
        >
          <strong>
            {promotion.name} · −${promotion.discount}
          </strong>
          <p className="text-xs">{promotion.reason}</p>
        </div>
      ))}
      <p className="muted text-xs">
        Calculado {new Date(value.calculated_at).toLocaleString('es-MX')}. Los precios se volverán a
        validar al cambiar la configuración. Esta herramienta no registra ventas.
      </p>
    </div>
  );
}
