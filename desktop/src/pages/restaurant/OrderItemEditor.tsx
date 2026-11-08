import { useEffect, useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Notice } from '../../components/Forms';
import { IntegerField, TextField } from '../../components/catalog/EditorFields';
import {
  ProductCustomizer,
  originalCustomization,
} from '../../components/catalog/ProductCustomizer';
import { QuoteBreakdown } from '../../components/catalog/QuoteBreakdown';
import { api } from '../../services/api';
import type { Lookups } from '../../services/catalog';
import type { Bill, DraftItem, EditorState, OrderQuote } from '../../services/restaurant';
import type { ExecuteMutation } from './OperationsAdmin';

export function tentativeDraft(editor: EditorState): DraftItem[] {
  const item: DraftItem = { id: editor.draftID ?? editor.localID, ...editor.line };
  if (!editor.draftID) return [...editor.baseDraft, item];
  return editor.baseDraft.map((entry) => (entry.id === editor.draftID ? item : entry));
}

export function OrderItemEditor({
  value,
  onChange,
  bill,
  lookups,
  execute,
  busy,
  onClose,
  onPreview,
}: {
  value: EditorState;
  onChange: (value: EditorState) => void;
  bill: Bill;
  lookups: Lookups;
  execute: ExecuteMutation;
  busy: boolean;
  onClose: () => void;
  onPreview: (value: OrderQuote | null) => void;
}) {
  const [preview, setPreview] = useState<OrderQuote | null>(null);
  const [error, setError] = useState('');
  const [calculating, setCalculating] = useState(false);
  const stale = value.revision !== bill.account.revision;
  const requestKey = JSON.stringify(
    value.confirmedVersionID
      ? {
          revision: value.revision,
          version_id: value.confirmedVersionID,
          line: value.line,
          reason: value.reason,
        }
      : { revision: value.revision, items: tentativeDraft(value) },
  );

  useEffect(() => {
    let current = true;
    setPreview(null);
    onPreview(null);
    setError('');
    if (stale) {
      setCalculating(false);
      return;
    }
    setCalculating(true);
    const timer = window.setTimeout(() => {
      void api<OrderQuote>(
        value.confirmedVersionID ? 'restaurant_edit_preview' : 'restaurant_draft_preview',
        { id: value.accountID, body: JSON.parse(requestKey) },
      )
        .then((result) => {
          if (current) {
            setPreview(result);
            onPreview(result);
          }
        })
        .catch((problem: unknown) => {
          if (current)
            setError(problem instanceof Error ? problem.message : 'No se pudo calcular.');
        })
        .finally(() => {
          if (current) setCalculating(false);
        });
    }, 180);
    return () => {
      current = false;
      window.clearTimeout(timer);
    };
  }, [requestKey, stale, value.accountID, value.confirmedVersionID, onPreview]);

  function rebase() {
    if (value.draftID && !bill.draft.some((item) => item.id === value.draftID)) {
      setError(
        'La partida del borrador ya no existe. Conserva tus selecciones y vuelve a capturarla como producto nuevo si corresponde.',
      );
      return;
    }
    if (
      value.confirmedVersionID &&
      !bill.lines.some(
        (line) => line.version_id === value.confirmedVersionID && line.state === 'active',
      )
    ) {
      setError(
        'La versión confirmada cambió o fue transferida. Abre su versión actual desde la cuenta.',
      );
      return;
    }
    onChange({ ...value, revision: bill.account.revision, baseDraft: bill.draft });
  }

  async function save() {
    if (!preview || stale || calculating) return;
    setError('');
    try {
      if (value.confirmedVersionID) {
        await execute('restaurant_edit', {
          id: value.accountID,
          body: {
            ...(JSON.parse(requestKey) as Record<string, unknown>),
            expected_fingerprint: preview.fingerprint,
          },
        });
      } else {
        await execute('restaurant_save_draft', {
          id: value.accountID,
          body: { revision: value.revision, items: tentativeDraft(value) },
        });
      }
      onClose();
    } catch (problem) {
      setError(problem instanceof Error ? problem.message : 'No se pudo guardar la selección.');
    }
  }

  return (
    <section className="operation-inline-panel operation-item-editor">
      <div className="section-heading">
        <h2>
          {value.confirmedVersionID ? 'Modificar producto confirmado' : 'Personalizar producto'}
        </h2>
        <button className="secondary-button" onClick={onClose}>
          Conservar y volver
        </button>
      </div>
      <Notice>{error}</Notice>
      {stale && (
        <Notice>
          Otra terminal cambió la cuenta. Tus selecciones siguen aquí. Compara la cuenta actual y
          actualiza la revisión antes de guardar.
        </Notice>
      )}
      {stale && (
        <button className="secondary-button" onClick={rebase}>
          Revisé los cambios; conservar esta selección
        </button>
      )}
      <fieldset disabled={busy} className="form-stack">
        <IntegerField
          label="Cantidad del producto"
          value={value.line.quantity}
          min={1}
          max={1000}
          onChange={(quantity) => onChange({ ...value, line: { ...value.line, quantity } })}
        />
        <ProductCustomizer
          productID={value.line.product_id}
          variantID={value.line.variant_id}
          onVariantChange={(variant_id) =>
            onChange({ ...value, line: { ...value.line, variant_id } })
          }
          value={value.line.customization}
          onChange={(customization) =>
            onChange({ ...value, line: { ...value.line, customization } })
          }
          onReset={() =>
            onChange({
              ...value,
              line: { ...value.line, customization: originalCustomization(), variant_id: null },
            })
          }
          lookups={lookups}
        />
        {value.confirmedVersionID && (
          <TextField
            required
            label="Motivo de modificación"
            value={value.reason}
            onChange={(reason) => onChange({ ...value, reason })}
          />
        )}
      </fieldset>
      {calculating && (
        <p className="muted" role="status">
          Calculando la cuenta en Go…
        </p>
      )}
      {preview?.quote && (
        <>
          <p className="muted text-sm">{preview.pricing_policy}</p>
          <Accordion
            title="Desglose calculado por el servidor"
            summary={`Cuenta resultante: $${preview.combined.total}`}
          >
            <QuoteBreakdown value={preview.quote} />
          </Accordion>
        </>
      )}
      <div className="operation-editor-actions">
        <button
          className="primary-button"
          disabled={
            busy ||
            calculating ||
            stale ||
            !preview ||
            (!!value.confirmedVersionID && value.reason.trim().length < 5)
          }
          onClick={() => void save()}
        >
          {value.confirmedVersionID ? 'Confirmar modificación' : 'Guardar en borrador'}
        </button>
        <p className="muted text-xs">
          {value.confirmedVersionID
            ? 'Se conservará el historial de la versión anterior.'
            : 'Guardar la selección no confirma ni envía un pedido a cocina.'}
        </p>
      </div>
    </section>
  );
}
