import type { Operation } from '../../services/api';
import { cashPrintMessage, drawerMessage } from '../../services/cash';
import type {
  CreatedCashReport,
  DrawerOutcome,
  PrintOutcome,
  RecordedCashOperation,
} from '../../services/cash';
import { notifySuccess, notifyWarning } from '../../services/notify';
import type { NotifyAction } from '../../services/notify';
import { cashDocumentPendingKey } from './CashDocumentActions';
import { notifyPrintOutcome, sentStatuses } from './CashNotices';
import { reportTitle } from './CashReportView';
import { authorizationText, recordedOperationTitle } from './RecordedOperation';
import { useCashMutation } from './useCashMutation';

// Flujo tras registrar en caja (etapa 7.1, decisión 12): un ingreso, retiro, gasto o corte parcial
// confirmado cierra su panel (o la hoja del atajo «Caja») y se confirma con un solo aviso con el
// tipo, el importe formateado y el concepto. El envío del comprobante y el comando del cajón van en
// la descripción con su texto de siempre («enviado», nunca «impreso» ni «cajón abierto»); si
// necesitan atención, además queda un aviso persistente. Si el comprobante no se imprimió y la
// persona tiene cash.reports.print, el aviso ofrece imprimirlo con la impresión posterior existente.
// Ningún aviso incluye el efectivo esperado.

// Escrituras cuyo resultado cierra el panel y se anuncia.
const recordedOperations: Operation[] = [
  'payments_shift_income',
  'payments_shift_withdrawal',
  'payments_shift_expense',
];

function attentionNeeded(status: string | undefined) {
  return !sentStatuses.includes(status ?? '');
}

// Separa el estado de la impresión y del cajón: lo enviado acompaña al aviso de éxito; lo que
// exige revisar queda en su propio aviso persistente.
function deliveryNotes(
  printing: PrintOutcome | undefined,
  subject: string,
  drawer?: DrawerOutcome,
) {
  const notes: string[] = [];
  const warnings: string[] = [];
  function classify(message: string, status: string | undefined) {
    if (!message) {
      return;
    }
    if (attentionNeeded(status)) {
      warnings.push(message);
    } else {
      notes.push(message);
    }
  }
  classify(cashPrintMessage(printing, subject), printing?.status);
  classify(drawerMessage(drawer), drawer?.status);
  return { notes, warnings };
}

// Aviso de un ingreso, retiro o gasto registrado. printVoucher solo se recibe con permiso de
// imprimir; la acción aparece si el comprobante aún no tiene documento.
export function announceRecordedOperation(
  operation: RecordedCashOperation,
  printVoucher?: () => void,
) {
  const { notes, warnings } = deliveryNotes(operation.printing, 'la operación', operation.drawer);
  const details: string[] = [];
  if (operation.kind !== 'income') {
    details.push(
      `${authorizationText(operation)}${operation.exceptional ? ' · Salida excepcional' : ''}.`,
    );
  }
  details.push(...notes);
  let action: NotifyAction | undefined;
  if (printVoucher && !operation.printing?.document_id) {
    action = { label: 'Imprimir comprobante', onClick: printVoucher };
  }
  notifySuccess(recordedOperationTitle(operation), {
    description: details.length > 0 ? details.join(' ') : undefined,
    action,
  });
  for (const warning of warnings) {
    notifyWarning(warning, { persistent: true });
  }
}

// Aviso de un corte parcial generado: sin cifras, porque el corte contiene el esperado. Ofrece
// imprimirlo si no se imprimió al generarlo.
export function announcePartialCut(report: CreatedCashReport, printCut?: () => void) {
  const { notes, warnings } = deliveryNotes(report.printing, 'el corte');
  let action: NotifyAction | undefined;
  if (printCut && !report.printing?.document_id) {
    action = { label: 'Imprimir corte', onClick: printCut };
  }
  notifySuccess(`${reportTitle(report)} generado.`, {
    description: notes.length > 0 ? notes.join(' ') : undefined,
    action,
  });
  for (const warning of warnings) {
    notifyWarning(warning, { persistent: true });
  }
}

export type RecordedKind = 'operation' | 'partial';

// Escrituras de caja de un turno con el flujo tras registrar. mutation es la escritura del turno
// (la misma clave del almacén global de pendientes de la etapa 7): su resultado se anuncia también
// cuando llega por el reintento de una operación incierta. printer imprime después el comprobante
// o el corte desde la acción del aviso, con su propia clave para no bloquear la caja.
export function useCashRecording(
  shiftID: string,
  can: (permission: string) => boolean,
  onRecorded?: (kind: RecordedKind) => void,
) {
  const printer = useCashMutation(
    shiftID,
    (_, result) => notifyPrintOutcome(result as PrintOutcome, 'el documento'),
    cashDocumentPendingKey(shiftID),
  );

  function printLater(operation: Operation, id: string) {
    if (!can('cash.reports.print')) {
      return undefined;
    }
    return () => {
      if (printer.isPending()) {
        return;
      }
      printer
        .execute<PrintOutcome>(operation, { id, body: {} })
        .catch((problem: unknown) =>
          printer.reportFailure(problem, 'No se pudo enviar el documento a imprimir.'),
        );
    };
  }

  const mutation = useCashMutation(shiftID, (operation, result) => {
    if (recordedOperations.includes(operation)) {
      const recorded = result as RecordedCashOperation;
      announceRecordedOperation(recorded, printLater('payments_print_cash_operation', recorded.id));
      onRecorded?.('operation');
      return;
    }
    if (operation === 'payments_shift_partial_cut') {
      const report = result as CreatedCashReport;
      announcePartialCut(report, printLater('payments_print_cash_report', report.id));
      onRecorded?.('partial');
    }
  });

  return { mutation, printer };
}
