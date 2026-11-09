import { useState } from 'react';
import { api, APIError } from '../services/api';
import type { Operation } from '../services/api';
import type { PreviewLine } from '../services/catalog';
import { emptyCustomization } from '../services/restaurant';
import type { Bill, DraftItem, EditorState, OrderQuote } from '../services/restaurant';
import type { CaptureProduct } from '../pages/restaurant/capture/captureRules';
import {
  draftUnits,
  maximumDraftLines,
  maximumDraftUnits,
} from '../pages/restaurant/capture/captureRules';
import {
  addLineOperation,
  applyOperations,
  quantityOperation,
} from '../pages/restaurant/capture/draftOperations';
import type { DraftOperation } from '../pages/restaurant/capture/draftOperations';
import type { DraftSyncEngine } from '../pages/restaurant/capture/draftSync';

function newID() {
  return crypto.randomUUID();
}

type Execute = <T>(
  operation: Operation,
  options: { id?: string; body?: Record<string, unknown> },
) => Promise<T>;

// Borrador local de la captura táctil para la cuenta mostrada: los toques se aplican primero en
// la terminal (motor de sincronización) y «Enviar a cocina» confirma con las reglas existentes.
export function useCaptureDraft({
  bill,
  engine,
  can,
  savedQuote,
  execute,
  onError,
  onConflict,
}: {
  bill: Bill | null;
  engine: DraftSyncEngine;
  can: (permission: string) => boolean;
  savedQuote: { data: OrderQuote | null; reload: () => Promise<void> };
  execute: Execute;
  onError: (message: string) => void;
  onConflict: () => void;
}) {
  const [sending, setSending] = useState(false);
  // Borrador visible de la cuenta: lo guardado más los toques pendientes de esta terminal.
  const draftView = bill ? engine.view(bill.account.id) : null;
  // Se usa el estado local mientras tenga toques pendientes o sea al menos tan reciente como la
  // cuenta mostrada; si no, la cuenta del servidor.
  const localView =
    bill &&
    draftView &&
    (draftView.pending || draftView.sending || draftView.baseRevision >= bill.account.revision)
      ? draftView
      : null;
  const localDraft: DraftItem[] = localView ? localView.items : (bill?.draft ?? []);
  // Borrador guardado en Go y revisión en la que se basa (la de su cotización).
  const persistedDraft: DraftItem[] = localView ? localView.base : (bill?.draft ?? []);
  const draftRevision = localView ? localView.baseRevision : (bill?.account.revision ?? 0);

  function writable() {
    return (
      !!bill &&
      bill.account.state === 'open' &&
      (bill.account.mode !== 'takeaway' || can('takeaway.manage')) &&
      can('orders.capture')
    );
  }

  // Aplica un toque local si respeta los límites del borrador que Go hará cumplir.
  function stage(operation: DraftOperation) {
    if (!bill || !writable()) return false;
    const next = applyOperations(localDraft, [operation]);
    if (next.length > maximumDraftLines || draftUnits(next) > maximumDraftUnits) {
      onError(
        `Envía a cocina este lote antes de agregar más productos (hasta ${maximumDraftLines} partidas y ${maximumDraftUnits} unidades por envío).`,
      );
      return false;
    }
    onError('');
    engine.stage(bill, operation);
    return true;
  }

  // Un toque en un producto sin elecciones obligatorias lo agrega (o suma a su partida sencilla).
  function addProduct(product: CaptureProduct) {
    const line: PreviewLine = {
      product_id: product.id,
      variant_id: null,
      quantity: 1,
      customization: emptyCustomization(),
    };
    return stage(addLineOperation(localDraft, line, newID));
  }

  function changeQuantity(item: DraftItem, delta: number) {
    return stage(quantityOperation(item, delta, newID));
  }

  // Lleva al borrador local la personalización del panel central: reemplaza la partida editada
  // o agrega una nueva (sumándola a una partida sencilla idéntica).
  function addEditorLine(editor: EditorState) {
    const operation: DraftOperation =
      editor.kind === 'draft' && editor.draftID
        ? { kind: 'replace', id: editor.draftID, item: { ...editor.line, id: editor.draftID } }
        : addLineOperation(localDraft, editor.line, newID);
    return stage(operation);
  }

  // «Enviar a cocina»: vacía la cola pendiente, obtiene la cotización vigente de Go y confirma
  // con su huella (las mismas reglas de confirmación de siempre: revisión, huella esperada,
  // operation_id y liberación a cocina).
  async function sendToKitchen() {
    if (!bill || sending) return;
    const accountID = bill.account.id;
    onError('');
    setSending(true);
    try {
      const synced = await engine.settle(accountID);
      const items = synced ? synced.items : bill.draft;
      const expectedRevision = synced ? synced.baseRevision : bill.account.revision;
      if (items.length === 0) return;
      const quote = await api<OrderQuote>('restaurant_preview', { id: accountID });
      if (quote.revision !== expectedRevision) {
        onError(
          'La cuenta cambió en otra terminal. Revisa los productos nuevos y vuelve a enviar.',
        );
        onConflict();
        return;
      }
      const shown = savedQuote.data;
      if (
        shown &&
        shown.account_id === accountID &&
        shown.revision === quote.revision &&
        shown.fingerprint !== quote.fingerprint
      ) {
        // Lo que vio el operador ya no coincide con el cálculo vigente: primero se revisa.
        onError(
          'El cálculo de Go cambió desde que se mostró. Revisa los importes y vuelve a enviar.',
        );
        void savedQuote.reload();
        return;
      }
      // Los toques que lleguen durante la confirmación se aplican después sobre su resultado.
      engine.hold(accountID);
      let confirmed: Bill | undefined;
      try {
        confirmed = await execute<Bill>('restaurant_confirm', {
          id: accountID,
          body: { revision: quote.revision, expected_fingerprint: quote.fingerprint },
        });
      } finally {
        engine.release(accountID, confirmed);
      }
    } catch (problem) {
      if (problem instanceof APIError && problem.code === 'QUOTE_CHANGED') {
        onError(
          'El cálculo cambió antes de enviar. Los productos nuevos se conservan: revisa los importes y vuelve a enviar.',
        );
        void savedQuote.reload();
      } else {
        onError(problem instanceof Error ? problem.message : 'No se pudo enviar a cocina.');
      }
    } finally {
      setSending(false);
    }
  }

  return {
    draftView,
    localDraft,
    persistedDraft,
    draftRevision,
    addProduct,
    changeQuantity,
    addEditorLine,
    sending,
    sendToKitchen,
  };
}
