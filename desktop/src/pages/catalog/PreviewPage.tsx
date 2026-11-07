import { useEffect, useState } from 'react';
import { Notice, PageTitle } from '../../components/Forms';
import {
  Check,
  IntegerField,
  ReferenceField,
  TextField,
} from '../../components/catalog/EditorFields';
import {
  ProductCustomizer,
  originalCustomization,
} from '../../components/catalog/ProductCustomizer';
import { QuoteBreakdown } from '../../components/catalog/QuoteBreakdown';
import { useResource } from '../../hooks/useResource';
import { api } from '../../services/api';
import type { Lookups, PreviewLine, Quote } from '../../services/catalog';

const emptyLine = (): PreviewLine => ({
  product_id: '',
  variant_id: null,
  quantity: 1,
  customization: originalCustomization(),
});

export function PreviewPage({ can }: { can: (permission: string) => boolean }) {
  const lookup = useResource<Lookups>('catalog_lookups');
  const [lines, setLines] = useState<PreviewLine[]>([emptyLine()]);
  const [priceList, setPriceList] = useState('');
  const [override, setOverride] = useState(false);
  const [reason, setReason] = useState('');
  const [quote, setQuote] = useState<Quote | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const data = lookup.data;
  const requestKey = JSON.stringify({
    lines,
    price_list_id: priceList || null,
    override_availability: override,
    override_reason: reason,
  });

  useEffect(() => {
    let current = true;
    setQuote(null);
    setError('');
    if (lines.some((line) => !line.product_id)) {
      setBusy(false);
      return;
    }
    if (override && reason.trim().length < 5) {
      setError('Explica la excepción de disponibilidad antes de calcular.');
      setBusy(false);
      return;
    }
    setBusy(true);
    const timer = window.setTimeout(() => {
      void api<Quote>('catalog_preview', { body: JSON.parse(requestKey) })
        .then((result) => {
          if (current) setQuote(result);
        })
        .catch((problem: unknown) => {
          if (current)
            setError(problem instanceof Error ? problem.message : 'No se pudo calcular.');
        })
        .finally(() => {
          if (current) setBusy(false);
        });
    }, 220);
    // Una respuesta tardía nunca sustituye el cálculo de una selección más reciente.
    return () => {
      current = false;
      window.clearTimeout(timer);
    };
  }, [requestKey, refresh]);

  function updateLine(index: number, changes: Partial<PreviewLine>) {
    setLines((current) =>
      current.map((line, position) => (position === index ? { ...line, ...changes } : line)),
    );
  }

  return (
    <>
      <PageTitle
        title="Probar personalización"
        description="Comprueba productos reales y promociones con el motor del servidor, sin crear pedidos ni ventas."
      />
      <Notice>{lookup.error}</Notice>
      <div className="preview-layout">
        <section className="panel form-stack">
          {data && (
            <>
              <ReferenceField
                optional
                label="Lista de precios (vacío: predeterminada)"
                value={priceList}
                options={data['price-lists']}
                onChange={setPriceList}
              />
              {lines.map((line, index) => (
                <section className="preview-line form-stack" key={index}>
                  <div className="section-heading">
                    <h2>Producto {index + 1}</h2>
                    {lines.length > 1 && (
                      <button
                        className="secondary-button"
                        onClick={() => setLines(lines.filter((_, position) => position !== index))}
                      >
                        Retirar de prueba
                      </button>
                    )}
                  </div>
                  <div className="form-grid">
                    <ReferenceField
                      label="Producto a comprobar"
                      value={line.product_id}
                      options={data.products.filter((product) => !product.archived)}
                      onChange={(product_id) => updateLine(index, { ...emptyLine(), product_id })}
                    />
                    <IntegerField
                      label="Unidades para probar promociones"
                      value={line.quantity}
                      min={1}
                      max={1000}
                      onChange={(quantity) => updateLine(index, { quantity })}
                    />
                  </div>
                  <ProductCustomizer
                    productID={line.product_id}
                    variantID={line.variant_id}
                    onVariantChange={(variant_id) => updateLine(index, { variant_id })}
                    value={line.customization}
                    onChange={(customization) => updateLine(index, { customization })}
                    onReset={() =>
                      updateLine(index, {
                        customization: originalCustomization(),
                        variant_id: null,
                      })
                    }
                    lookups={data}
                  />
                </section>
              ))}
              <button
                className="secondary-button"
                disabled={lines.length >= 20}
                onClick={() => setLines([...lines, emptyLine()])}
              >
                Agregar otro producto a la prueba
              </button>
              {can('catalog.availability.override') && (
                <details className="catalog-section">
                  <summary>Excepción autorizada de disponibilidad</summary>
                  <Check
                    label="Ignorar pausa temporal y horarios (queda auditado)"
                    value={override}
                    onChange={setOverride}
                  />
                  {override && (
                    <TextField
                      label="Motivo obligatorio de la excepción"
                      value={reason}
                      onChange={setReason}
                      required
                    />
                  )}
                </details>
              )}
            </>
          )}
        </section>
        <aside className="panel quote-panel" aria-live="polite" aria-busy={busy}>
          <div className="section-heading">
            <h2>Desglose del precio</h2>
            <button className="secondary-button" onClick={() => setRefresh(refresh + 1)}>
              Recalcular
            </button>
          </div>
          <Notice>{error}</Notice>
          {busy && (
            <p className="muted" role="status">
              Validando selección y calculando…
            </p>
          )}
          {quote && !busy && <QuoteBreakdown value={quote} />}
          {!quote && !busy && !error && (
            <p className="muted">
              Selecciona un producto para consultar su precio exacto y modificar la preparación.
            </p>
          )}
        </aside>
      </div>
    </>
  );
}
