import { useEffect, useRef, useState } from 'react';
import { useCountPreview } from '../../hooks/usePaymentPreview';
import { useLiveResource } from '../../hooks/useLiveResource';
import { APIError } from '../../services/api';
import { toWireAmount } from '../../services/amountInput';
import type { ClosingView, CountPreviewInput, Denomination } from '../../services/cash';
import type { Shift } from '../../services/payments';
import { acceptQuantity, countLines, extraText, quantitiesOf } from './countCapture';
import type { CountQuantities } from './countCapture';
import type { CashMutation } from './useCashMutation';

// Captura local del arqueo: cantidades por denominación, importe adicional identificado y
// observaciones. Son cadenas tal como se teclean; Go calcula importes y totales.
export interface ClosingCapture {
  quantities: CountQuantities;
  extraMXN: string;
  extraUSD: string;
  extraNote: string;
  notes: string;
}

// Retardo del guardado automático del borrador en el servidor tras el último cambio.
const autosaveDelay = 900;

const emptyCapture: ClosingCapture = {
  quantities: {},
  extraMXN: '',
  extraUSD: '',
  extraNote: '',
  notes: '',
};

function captureOf(view: ClosingView): ClosingCapture {
  return {
    quantities: quantitiesOf(view.count),
    extraMXN: extraText(view.count?.extra_mxn),
    extraUSD: extraText(view.count?.extra_usd),
    extraNote: view.count?.extra_note ?? '',
    notes: view.count?.notes ?? '',
  };
}

// Cuerpo del conteo que valida Go (borrador y vista previa usan el mismo contrato).
function countInput(capture: ClosingCapture, denominations: Denomination[]) {
  const input: CountPreviewInput = { lines: countLines(capture.quantities, denominations) };
  const extraMXN = toWireAmount(capture.extraMXN);
  const extraUSD = toWireAmount(capture.extraUSD);
  if (extraMXN) {
    input.extra_mxn = extraMXN;
  }
  if (extraUSD) {
    input.extra_usd = extraUSD;
  }
  if (capture.extraNote.trim()) {
    input.extra_note = capture.extraNote.trim();
  }
  return input;
}

// Borrador del arqueo de cierre guardado en el servidor (PUT shifts/{id}/closing/count con
// revisión) para que sobreviva a la navegación y al cierre de la aplicación. La captura local se
// conserva siempre: ante COUNT_CHANGED (otra terminal guardó antes) se recarga la revisión del
// servidor, se avisa y el guardado automático se detiene hasta que el cajero lo confirme.
export function useClosingDraft({
  shift,
  denominations,
  mutation,
}: {
  shift: Shift;
  denominations: Denomination[];
  mutation: CashMutation;
}) {
  const closing = useLiveResource<ClosingView>('payments_shift_closing', {
    id: shift.id,
    enabled: shift.state === 'closing',
    interval: 15000,
  });
  const [capture, setCapture] = useState<ClosingCapture>(emptyCapture);
  const [loadedCount, setLoadedCount] = useState('');
  const [baseRevision, setBaseRevision] = useState(0);
  const [dirty, setDirty] = useState(false);
  const [conflict, setConflict] = useState(false);
  const [failedKey, setFailedKey] = useState('');
  // Versión de la captura local: un guardado solo limpia «sin guardar» si nada cambió mientras viajaba.
  const version = useRef(0);
  const server = closing.data?.count ?? null;

  // Adopta el borrador del servidor al iniciar y cuando otra terminal lo cambia sin que haya
  // cambios locales; con cambios locales, solo se toma su revisión tras un conflicto.
  useEffect(() => {
    if (!closing.data || !server) {
      return;
    }
    if (server.id !== loadedCount) {
      setCapture(captureOf(closing.data));
      setLoadedCount(server.id);
      setBaseRevision(server.revision);
      setDirty(false);
      return;
    }
    // Las revisiones solo crecen: una respuesta de sondeo anterior a un guardado propio no
    // sustituye la captura ya guardada.
    if (server.revision <= baseRevision) {
      return;
    }
    if (conflict) {
      setBaseRevision(server.revision);
      return;
    }
    if (!dirty) {
      setCapture(captureOf(closing.data));
      setBaseRevision(server.revision);
    }
  }, [closing.data, server, loadedCount, baseRevision, conflict, dirty]);

  const input = countInput(capture, denominations);
  const serialized = JSON.stringify({ input, notes: capture.notes.trim() });
  const preview = useCountPreview(shift.id, { ...input, shift_id: shift.id });

  function update(patch: Partial<ClosingCapture>) {
    if (mutation.locked) {
      return;
    }
    version.current += 1;
    setCapture((current) => ({ ...current, ...patch }));
    setDirty(true);
    mutation.clearError();
  }

  function changeQuantity(id: string, next: string) {
    const previous = capture.quantities[id] ?? '';
    update({ quantities: { ...capture.quantities, [id]: acceptQuantity(next, previous) } });
  }

  // Guarda el borrador con la revisión en la que se basa la captura. Devuelve la nueva revisión o
  // null si no se guardó.
  async function save(): Promise<number | null> {
    if (mutation.isPending()) {
      return null;
    }
    const started = version.current;
    const key = serialized;
    try {
      const view = await mutation.execute<ClosingView>('payments_shift_save_count', {
        id: shift.id,
        body: { revision: baseRevision, ...input, notes: capture.notes.trim() },
      });
      const revision = view.count?.revision ?? baseRevision;
      setBaseRevision(revision);
      setFailedKey('');
      if (version.current === started) {
        setDirty(false);
      }
      return revision;
    } catch (problem) {
      setFailedKey(key);
      if (problem instanceof APIError && problem.code === 'COUNT_CHANGED') {
        setConflict(true);
        void closing.reload();
      }
      return null;
    }
  }

  // Tras un conflicto, guardar de nuevo exige una acción explícita del cajero.
  async function saveAfterConflict() {
    setConflict(false);
    setFailedKey('');
    return save();
  }

  const shouldAutosave =
    dirty && !conflict && !mutation.locked && !mutation.uncertain && failedKey !== serialized;
  useEffect(() => {
    if (!shouldAutosave) {
      return;
    }
    const timer = window.setTimeout(() => void save(), autosaveDelay);
    return () => window.clearTimeout(timer);
    // save usa el estado vigente del render; el temporizador se reinicia con cada cambio.
  }, [shouldAutosave, serialized]);

  return {
    closing,
    capture,
    update,
    changeQuantity,
    preview,
    dirty,
    conflict,
    baseRevision,
    save,
    saveAfterConflict,
    clearConflict: () => setConflict(false),
  };
}

export type ClosingDraft = ReturnType<typeof useClosingDraft>;
