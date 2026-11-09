import { useEffect, useState } from 'react';
import { Accordion } from '../../../components/Accordion';
import { Notice } from '../../../components/Forms';
import { QuantityStepper } from '../../../components/QuantityStepper';
import { TextField } from '../../../components/catalog/EditorFields';
import { QuoteBreakdown } from '../../../components/catalog/QuoteBreakdown';
import { originalCustomization } from '../../../components/catalog/customizationRules';
import { api } from '../../../services/api';
import type { Lookups } from '../../../services/catalog';
import type { Bill, DraftItem, EditorState, OrderQuote } from '../../../services/restaurant';
import type { ExecuteMutation } from '../OperationsAdmin';
import { KitchenNotes } from './KitchenNotes';
import { TouchCustomizer } from './TouchCustomizer';

// Borrador con la partida en edición: se agrega al final (nueva) o sustituye a la del borrador.
export function tentativeItems(editor: EditorState, localDraft: DraftItem[]): DraftItem[] {
  if (editor.kind === 'draft' && editor.draftID) {
    const id = editor.draftID;
    if (localDraft.some((item) => item.id === id)) {
      return localDraft.map((item) => (item.id === id ? { ...editor.line, id } : item));
    }
    // Si la partida ya no está en el borrador (por ejemplo, se envió), se vuelve a crear.
    return [...localDraft, { ...editor.line, id }];
  }
  return [...localDraft, { ...editor.line, id: editor.localID }];
}

// Personalización en el panel central de la captura. La cotización tentativa la calcula Go
// (`restaurant_draft_preview` sobre el borrador local vigente o `restaurant_edit_preview` para una
// partida confirmada); el total del botón principal es el de esa cotización.
export function CustomizationPanel({
  editor,
  onChange,
  bill,
  localDraft,
  draftRevision,
  lookups,
  noteShortcuts,
  execute,
  busy,
  onBack,
  onSave,
}: {
  editor: EditorState;
  onChange: (value: EditorState) => void;
  bill: Bill;
  localDraft: DraftItem[];
  draftRevision: number;
  lookups: Lookups;
  noteShortcuts: string[];
  execute: ExecuteMutation;
  busy: boolean;
  onBack: () => void;
  onSave: () => void;
}) {
  const [preview, setPreview] = useState<OrderQuote | null>(null);
  const [error, setError] = useState('');
  const [calculating, setCalculating] = useState(false);
  const confirmed = editor.kind === 'confirmed';
  const stale = confirmed && editor.revision !== bill.account.revision;
  const items = confirmed ? [] : tentativeItems(editor, localDraft);
  const position = items.findIndex((item) => item.id === (editor.draftID ?? editor.localID));
  const requestKey = JSON.stringify(
    confirmed
      ? {
          revision: editor.revision,
          version_id: editor.confirmedVersionID,
          line: editor.line,
          reason: editor.reason,
        }
      : { revision: draftRevision, items },
  );
  const productName =
    lookups.products.find((entry) => entry.id === editor.line.product_id)?.name ?? 'Producto';

  useEffect(() => {
    let current = true;
    setPreview(null);
    setError('');
    if (stale) {
      setCalculating(false);
      return;
    }
    setCalculating(true);
    // Breve espera para no cotizar cada toque intermedio; una respuesta atrasada se descarta.
    const timer = window.setTimeout(() => {
      void api<OrderQuote>(confirmed ? 'restaurant_edit_preview' : 'restaurant_draft_preview', {
        id: editor.accountID,
        body: JSON.parse(requestKey),
      })
        .then((result) => {
          if (current) setPreview(result);
        })
        .catch((problem: unknown) => {
          if (current) {
            setError(problem instanceof Error ? problem.message : 'No se pudo calcular.');
          }
        })
        .finally(() => {
          if (current) setCalculating(false);
        });
    }, 180);
    return () => {
      current = false;
      window.clearTimeout(timer);
    };
  }, [requestKey, stale, confirmed, editor.accountID]);

  const quoteLine = preview?.quote?.lines[confirmed ? 0 : position] ?? null;
  const ready = !!preview && !!quoteLine && !calculating && !stale && !error;

  function rebase() {
    if (
      !bill.lines.some(
        (line) => line.version_id === editor.confirmedVersionID && line.state === 'active',
      )
    ) {
      setError(
        'La versión confirmada cambió o fue transferida. Abre su versión actual desde la cuenta.',
      );
      return;
    }
    onChange({ ...editor, revision: bill.account.revision });
  }

  async function confirmEdit() {
    if (!preview || !ready) return;
    setError('');
    try {
      await execute('restaurant_edit', {
        id: editor.accountID,
        body: {
          ...(JSON.parse(requestKey) as Record<string, unknown>),
          expected_fingerprint: preview.fingerprint,
        },
      });
    } catch (problem) {
      setError(problem instanceof Error ? problem.message : 'No se pudo modificar.');
    }
  }

  const title = confirmed ? `Modificar ${productName}` : productName;
  const explanation = confirmed
    ? 'Cambia la preparación de la partida enviada. Se conservará el historial de la versión anterior.'
    : editor.kind === 'draft'
      ? 'Ajusta la partida nueva. Los cambios se guardan en la cuenta sin enviarse a cocina.'
      : 'Elige las opciones y toca «Agregar a la cuenta».';

  return (
    <section className="capture-customization" aria-label={`Personalizar ${productName}`}>
      <header className="capture-customization-header">
        <button type="button" className="secondary-button capture-back" onClick={onBack}>
          Volver
        </button>
        <div>
          <h2>{title}</h2>
          <p className="muted text-sm">{explanation}</p>
        </div>
      </header>
      <div className="capture-customization-body">
        <Notice>{error}</Notice>
        {stale && (
          <>
            <Notice>
              Otra terminal cambió la cuenta. Tus selecciones siguen aquí. Compara la cuenta actual
              antes de confirmar la modificación.
            </Notice>
            <button type="button" className="secondary-button" onClick={rebase}>
              Revisé los cambios; conservar esta selección
            </button>
          </>
        )}
        <fieldset disabled={busy} className="capture-customization-fields">
          <TouchCustomizer
            productID={editor.line.product_id}
            variantID={editor.line.variant_id}
            onVariantChange={(variant_id) =>
              onChange({ ...editor, line: { ...editor.line, variant_id } })
            }
            value={editor.line.customization}
            onChange={(customization) =>
              onChange({ ...editor, line: { ...editor.line, customization } })
            }
            lookups={lookups}
            breakdown={quoteLine}
            noteShortcuts={noteShortcuts}
            basePriceUnavailable={editor.basePriceUnavailable}
          />
          <KitchenNotes
            value={editor.line.customization.notes}
            onChange={(notes) =>
              onChange({
                ...editor,
                line: {
                  ...editor.line,
                  customization: { ...editor.line.customization, notes },
                },
              })
            }
            shortcuts={noteShortcuts}
          />
          {confirmed && (
            <TextField
              required
              label="Motivo de modificación"
              value={editor.reason}
              onChange={(reason) => onChange({ ...editor, reason })}
            />
          )}
          {preview?.quote && (
            <Accordion
              title="Desglose calculado por el servidor"
              summary={`Cuenta resultante: $${preview.combined.total}`}
            >
              <p className="muted text-sm">{preview.pricing_policy}</p>
              <QuoteBreakdown value={preview.quote} />
            </Accordion>
          )}
          <button
            type="button"
            className="secondary-button capture-reset"
            onClick={() =>
              onChange({
                ...editor,
                line: {
                  ...editor.line,
                  variant_id: null,
                  customization: originalCustomization(),
                },
              })
            }
          >
            Restablecer configuración original
          </button>
        </fieldset>
      </div>
      <footer className="capture-customization-footer">
        <QuantityStepper
          verb="Cantidad"
          name={productName}
          quantity={editor.line.quantity}
          minimum={1}
          max={1000}
          showMax={false}
          large
          disabled={busy}
          onChange={(quantity) => onChange({ ...editor, line: { ...editor.line, quantity } })}
        />
        <button
          type="button"
          className="primary-button capture-add"
          disabled={busy || !ready || (confirmed && editor.reason.trim().length < 5)}
          onClick={() => (confirmed ? void confirmEdit() : onSave())}
        >
          {confirmed
            ? 'Confirmar modificación'
            : editor.kind === 'draft'
              ? 'Guardar cambios'
              : 'Agregar a la cuenta'}
          {quoteLine && !calculating ? ` · $${quoteLine.total}` : ''}
        </button>
        <p className="muted text-xs capture-add-status" role="status">
          {calculating ? 'Calculando en el servidor…' : ''}
        </p>
      </footer>
    </section>
  );
}
