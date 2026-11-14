import { useEffect, useState } from 'react';
import { isWindows } from '../../../commands/platform';
import { api } from '../../../services/api';
import { printDocumentSample } from '../../../services/documents';
import type {
  DocumentKind,
  PrinterCompatibility,
  SampleResult,
  TemplateLayout,
} from '../../../services/documents';
import { jobLabels } from '../../../services/kitchen';
import type { PrintJob } from '../../../services/kitchen';
import { errorMessage, notifyError, notifySuccess, notifyWarning } from '../../../services/notify';
import { ModalPanel } from '../ModalPanel';
import { PrinterSupport } from './PrinterSupport';
import type { RunOperation } from './useOperations';

// «Imprimir muestra»: elegir impresora (con su ancho y lo que puede imprimir), confirmar y seguir
// el estado del trabajo en la cola de siempre. Go agrega las líneas «MUESTRA · NO ES UN DOCUMENTO
// VÁLIDO» al inicio y al final y nunca crea ventas, pagos ni movimientos. Los mensajes siguen la
// regla de la etapa 7.1: «enviado a la cola», nunca «impreso»; un resultado incierto se revisa en
// Cocina → Cola de impresión.

const transportLabels: Record<PrinterCompatibility['transport'], string> = {
  cups: 'CUPS',
  escpos_tcp: 'ESC/POS por red',
  // Etapa 9G (fase E): la impresora USB de una terminal, que imprime su agente.
  agent: 'Impresora USB de una terminal',
};

// Transporte de la impresora como lo ve el restaurante. En Windows (etapa 9E) la cola del equipo es
// la de las impresoras instaladas en Windows; en macOS sigue siendo «CUPS».
function transportLabel(transport: PrinterCompatibility['transport']) {
  if (transport === 'cups' && isWindows()) {
    return 'Impresora de Windows';
  }
  return transportLabels[transport] ?? 'Impresora';
}

// Estados en los que el trabajo ya no cambia solo.
const settledStates = ['submitted', 'error', 'uncertain', 'cancelled', 'intervention'];

export function sampleStatusMessage(state: string) {
  switch (state) {
    case 'pending':
    case 'processing':
      return 'Muestra enviada a la cola de impresión.';
    case 'submitted':
      return 'Muestra aceptada por la impresora o la cola del sistema. Eso no confirma que salió el papel: revísalo en la impresora.';
    case 'cancelled':
      return 'El trabajo de la muestra se canceló.';
  }
  const label = jobLabels[state as PrintJob['state']] ?? 'Resultado sin confirmar';
  return `${label}: revisa el trabajo en Cocina → Cola de impresión.`;
}

function attention(state: string) {
  return ['error', 'uncertain', 'intervention'].includes(state);
}

export function SampleDialog({
  kind,
  layout,
  printers,
  canWatchQueue,
  run,
  onClose,
}: {
  kind: DocumentKind;
  layout: TemplateLayout;
  printers: PrinterCompatibility[] | null;
  // Con kitchen.queue.read y kitchen.stations.all se sigue el estado del trabajo.
  canWatchQueue: boolean;
  run: RunOperation;
  onClose: () => void;
}) {
  const [printerID, setPrinterID] = useState('');
  const [step, setStep] = useState<'choose' | 'confirm' | 'sent'>('choose');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<SampleResult | null>(null);
  const [state, setState] = useState('');
  const printer = printers?.find((item) => item.id === printerID) ?? null;

  // Sigue el trabajo en la cola hasta que se asienta (aceptado, error, incierto o intervención).
  useEffect(() => {
    if (!result || !canWatchQueue || settledStates.includes(state)) {
      return;
    }
    let active = true;
    const timer = window.setInterval(() => {
      void api<{ job: PrintJob }>('kitchen_job', { id: result.job_id })
        .then((detail) => {
          if (active) {
            setState(detail.job.state);
          }
        })
        .catch(() => undefined);
    }, 1500);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [result, canWatchQueue, state]);

  async function send() {
    if (!printer) {
      return;
    }
    setBusy(true);
    try {
      const sent = await run(`sample:${kind}`, { layout, printer: printer.id }, (operationID) =>
        printDocumentSample(operationID, kind, layout, printer.id),
      );
      setResult(sent);
      setState(sent.status);
      setStep('sent');
      const message = sampleStatusMessage(sent.status);
      if (attention(sent.status)) {
        notifyWarning(message, { persistent: true });
      } else {
        notifySuccess(message);
      }
    } catch (error) {
      notifyError(error, 'No se pudo enviar la muestra.');
    } finally {
      setBusy(false);
    }
  }

  const actions =
    step === 'choose' ? (
      <>
        <button type="button" className="secondary-button dialog-button" onClick={onClose}>
          Cancelar
        </button>
        <button
          type="button"
          className="primary-button dialog-button"
          disabled={!printer}
          onClick={() => setStep('confirm')}
        >
          Continuar
        </button>
      </>
    ) : step === 'confirm' ? (
      <>
        <button
          type="button"
          className="secondary-button dialog-button"
          disabled={busy}
          onClick={() => setStep('choose')}
        >
          Cambiar impresora
        </button>
        <button
          type="button"
          className="primary-button dialog-button"
          disabled={busy}
          onClick={() => void send()}
        >
          {busy ? 'Enviando…' : 'Imprimir muestra'}
        </button>
      </>
    ) : (
      <button type="button" className="primary-button dialog-button" onClick={onClose}>
        Listo
      </button>
    );

  return (
    <ModalPanel title="Imprimir una muestra" wide actions={actions} onClose={onClose}>
      <p className="designer-sample-mark">
        Es una prueba: se imprime con datos de muestra, marcada «MUESTRA · NO ES UN DOCUMENTO
        VÁLIDO» al inicio y al final. No crea ventas, pagos, movimientos de caja ni documentos.
      </p>
      {step === 'choose' && (
        <div className="designer-printer-list" role="group" aria-label="Impresora para la muestra">
          {printers === null && (
            <p className="muted" role="status">
              Consultando impresoras…
            </p>
          )}
          {printers?.length === 0 && (
            <p className="muted">
              No hay impresoras activas. Regístralas en Cocina y preparación → Impresoras.
            </p>
          )}
          {printers?.map((item) => (
            <button
              type="button"
              key={item.id}
              className={`designer-printer ${item.id === printerID ? 'designer-printer-selected' : ''}`}
              aria-pressed={item.id === printerID}
              onClick={() => setPrinterID(item.id)}
            >
              <strong>{item.name}</strong>
              <span className="muted text-xs">
                {transportLabel(item.transport)} · papel de {item.paper_width} mm · {item.columns}{' '}
                columnas
              </span>
              <PrinterSupport printer={item} />
            </button>
          ))}
        </div>
      )}
      {step === 'confirm' && printer && (
        <div className="form-stack">
          <p>
            Se enviará una muestra a <strong>{printer.name}</strong> (papel de {printer.paper_width}{' '}
            mm) con el borrador que ves en la vista previa, aunque no lo hayas guardado.
          </p>
          <PrinterSupport printer={printer} />
        </div>
      )}
      {step === 'sent' && result && (
        <div
          className={attention(state) ? 'checkout-result-warning' : 'checkout-result-flag'}
          role="status"
        >
          <p>{sampleStatusMessage(state)}</p>
          {result.message && <p className="muted text-xs">{errorMessage(result.message, '')}</p>}
          {!canWatchQueue && (
            <p className="muted text-xs">
              El estado posterior del trabajo se consulta en Cocina → Cola de impresión.
            </p>
          )}
        </div>
      )}
    </ModalPanel>
  );
}
