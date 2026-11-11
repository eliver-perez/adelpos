import { Accordion } from '../Accordion';
import type { ProductBreakdown, Quote } from '../../services/catalog';
import { formatDeduction, formatMoney, formatSignedMoney } from '../../services/money';

// Cargo que se suma al precio base: «+$10.00» (o «$0.00» sin cargo), con el formato único de
// importes. El importe lo calcula Go; aquí solo se presenta.
function addition(value: string) {
  return formatSignedMoney(value);
}

// Rótulo de la diferencia de un grupo. En modo valor es la equivalencia (con la opción «El precio
// puede bajar al cambiar complementos» puede ser negativa); en modo cantidad, el cargo por tramos o,
// con la opción, los tramos menos el valor de las porciones incluidas que se retiraron.
function differenceLabel(mode: string, priceCanDecrease: boolean | undefined) {
  if (mode === 'value') {
    return 'Diferencia';
  }
  return priceCanDecrease ? 'Tramos menos porciones retiradas' : 'Cargo por tramos';
}

// Efecto de la opción en el producto, con el ajuste que calculó Go (cero o negativo). Solo se
// inspecciona el signo del texto; no hay aritmética.
function complementEffect(adjustment: string | undefined) {
  if (adjustment?.startsWith('-')) {
    return `el precio bajó ${formatSignedMoney(adjustment)} por los complementos elegidos (incluido en «Complementos»).`;
  }
  return 'sin reducción con los complementos elegidos.';
}

export function ProductDetails({ value }: { value: ProductBreakdown }) {
  return (
    <div className="quote-product-details">
      <p className="muted text-xs">
        {value.price_origin} · Configuración r{value.revision}
      </p>
      <dl className="price-breakdown">
        <dt>Base</dt>
        <dd>{formatMoney(value.base_price)}</dd>
        <dt>Ingredientes</dt>
        <dd>{addition(value.ingredient_extras)}</dd>
        <dt>Complementos</dt>
        <dd>{addition(value.group_extras)}</dd>
        <dt>Componentes</dt>
        <dd>{addition(value.component_extras)}</dd>
        <dt>Precio por unidad</dt>
        <dd>{formatMoney(value.unit_price)}</dd>
      </dl>
      {value.price_can_decrease && (
        <p className="muted text-xs">
          El precio puede bajar al cambiar complementos:{' '}
          {complementEffect(value.complement_adjustment)}
        </p>
      )}
      {value.groups.map((group) => (
        <Accordion key={group.group_id} defaultOpen title={group.name}>
          <dl className="price-breakdown">
            <dt>Valor original incluido</dt>
            <dd>{formatMoney(group.original_value)}</dd>
            <dt>Valor solicitado</dt>
            <dd>{formatMoney(group.requested_value)}</dd>
            <dt>{differenceLabel(group.mode, value.price_can_decrease)}</dt>
            <dd>{formatSignedMoney(group.difference)}</dd>
            <dt>Extras explícitos separados</dt>
            <dd>{formatMoney(group.extras)}</dd>
          </dl>
          {group.options
            .filter((option) => option.requested !== '0' || option.included !== '0')
            .map((option) => (
              <p className="muted text-xs" key={option.option_id}>
                {option.name}: {option.included} incluido → {option.requested} solicitado
              </p>
            ))}
        </Accordion>
      ))}
      {value.ingredients.map((ingredient) => (
        <p className="muted text-xs" key={ingredient.ingredient_id}>
          {ingredient.name}: {ingredient.included} incluido → {ingredient.requested} solicitado ·{' '}
          {addition(ingredient.extra)}
        </p>
      ))}
      {value.components.map((component) => (
        <Accordion
          key={component.slot_id}
          title={`${component.name} × ${component.quantity}`}
          summary={`Cargo del componente: ${formatMoney(component.charge)}`}
        >
          {component.product && <ProductDetails value={component.product} />}
        </Accordion>
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
        <h2 className="quote-total">{formatMoney(value.total)}</h2>
        <p className="muted text-xs">
          {value.price_list_name} · {value.timezone}
        </p>
      </div>
      <dl className="price-breakdown">
        <dt>Subtotal</dt>
        <dd>{formatMoney(value.subtotal)}</dd>
        <dt>Promociones automáticas</dt>
        <dd>{formatDeduction(value.discount)}</dd>
        <dt>Total de previsualización</dt>
        <dd>{formatMoney(value.total)}</dd>
      </dl>
      {value.lines.map((line, index) => (
        <section className="catalog-section" key={index}>
          <h3>
            {line.name} × {line.quantity}
          </h3>
          <ProductDetails value={line} />
          <p className="text-sm">
            Subtotal {formatMoney(line.subtotal)} · Descuento {formatDeduction(line.discount)} ·
            Total {formatMoney(line.total)}
          </p>
        </section>
      ))}
      {value.promotions.map((promotion) => (
        <div
          className={
            promotion.applied ? 'quote-promotion quote-promotion-applied' : 'quote-promotion'
          }
          key={promotion.id}
        >
          <strong>
            {promotion.name} · {formatDeduction(promotion.discount)}
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
