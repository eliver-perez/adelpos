import { useState } from 'react';
import { SegmentedControl } from '../../../components/SegmentedControl';
import type { DocumentKind, PaperWidth, TemplateLayout } from '../../../services/documents';
import { PaperStrip } from './PaperStrip';
import { usePreview } from './usePreview';

// Vista previa de solo lectura de un layout (una versión del historial, la versión que guardó otra
// persona): la IR de Go con datos de muestra, en 58 u 80 mm.
export function LayoutPreview({
  kind,
  layout,
  label,
  initialWidth = 80,
}: {
  kind: DocumentKind;
  layout: TemplateLayout | null;
  label: string;
  initialWidth?: PaperWidth;
}) {
  const [paperWidth, setPaperWidth] = useState<PaperWidth>(initialWidth);
  const { preview, problem, loading } = usePreview(
    layout ? { kind, layout, paperWidth, source: { sample: true } } : null,
  );
  return (
    <div className="designer-layout-preview">
      <div className="designer-preview-controls">
        <SegmentedControl
          label={`Ancho del papel de ${label}`}
          value={String(paperWidth) as '58' | '80'}
          options={[
            { value: '58', label: '58 mm' },
            { value: '80', label: '80 mm' },
          ]}
          onChange={(value) => setPaperWidth(Number(value) as PaperWidth)}
        />
        <span className="muted text-xs">
          {loading ? 'Actualizando…' : 'Datos de muestra · solo lectura'}
        </span>
      </div>
      {problem && (
        <p className="checkout-inline-note" role="alert">
          {problem.message}
        </p>
      )}
      <div className="designer-paper-stage designer-paper-stage-compact">
        {preview ? (
          <PaperStrip preview={preview} label={`${label}, papel de ${preview.paper_width} mm`} />
        ) : (
          !problem && (
            <p className="muted" role="status">
              Generando la vista previa…
            </p>
          )
        )}
      </div>
    </div>
  );
}
