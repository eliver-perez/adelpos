import {
  cashPrintMessage,
  currencyLabels,
  drawerMessage,
  printAttentionStatuses,
} from '../../services/cash';
import type { CashLimitWarning, DrawerOutcome, PrintOutcome } from '../../services/cash';
import { notifySuccess, notifyWarning } from '../../services/notify';

// Aviso del tope de caja (fase C). Solo se muestra si Go lo entregó: la función está activa, quien
// consulta puede ver el efectivo esperado y el turno no está en arqueo. La interfaz nunca compara
// importes con el tope ni deduce el aviso por su cuenta.
export function CashLimitNotice({ warning }: { warning?: CashLimitWarning | null }) {
  if (!warning) {
    return null;
  }
  const currencies = warning.currencies.map((currency) => currencyLabels[currency].toLowerCase());
  return (
    <div className="cash-limit-warning" role="status" aria-label="Tope de caja superado">
      <strong>Tope de caja superado</strong>
      <p>
        {warning.message} Moneda: {currencies.join(' y ')}. El aviso no impide cobrar.
      </p>
    </div>
  );
}

function attention(status: string | undefined) {
  return !!status && printAttentionStatuses.includes(status);
}

// Resultado de una impresión de caja: informa el envío a la cola o a la impresora, nunca que el
// papel salió, y aclara que un fallo no revierte la operación.
export function PrintNotice({
  outcome,
  subject,
}: {
  outcome?: PrintOutcome | null;
  subject: string;
}) {
  const message = cashPrintMessage(outcome, subject);
  if (!message) {
    return null;
  }
  return (
    <p className={attention(outcome?.status) ? 'checkout-result-warning' : 'checkout-result-flag'}>
      {message}
    </p>
  );
}

// Resultado de un comando del cajón: «Comando de apertura enviado», nunca «cajón abierto».
export function DrawerNotice({ outcome }: { outcome?: DrawerOutcome | null }) {
  const message = drawerMessage(outcome);
  if (!message) {
    return null;
  }
  return (
    <p className={attention(outcome?.status) ? 'checkout-result-warning' : 'checkout-result-flag'}>
      {message}
    </p>
  );
}

// Estados en los que el documento o el comando se entregó a la cola o a la impresora. Ninguno
// confirma papel impreso ni cajón abierto.
export const sentStatuses = ['pending', 'processing', 'submitted'];

// Resultado de una impresión de caja pedida como acción propia (imprimir o reimprimir un documento,
// imprimir un corte): se avisa con el sistema único de mensajes, con el mismo texto que el
// resultado en pantalla. Si no se envió, el aviso permanece porque exige revisar la impresión.
export function notifyPrintOutcome(outcome: PrintOutcome | null | undefined, subject: string) {
  const message = cashPrintMessage(outcome, subject);
  if (!message) {
    return;
  }
  if (!sentStatuses.includes(outcome?.status ?? '')) {
    notifyWarning(message, { persistent: true });
    return;
  }
  notifySuccess(message);
}

// Resultado del comando del cajón: «Comando de apertura enviado» (nunca «cajón abierto»). Un
// resultado incierto o fallido queda como aviso persistente: no se reintenta solo.
export function notifyDrawerOutcome(outcome: DrawerOutcome | null | undefined) {
  const message = drawerMessage(outcome);
  if (!message) {
    return;
  }
  if (!sentStatuses.includes(outcome?.status ?? '')) {
    notifyWarning(message, { persistent: true });
    return;
  }
  notifySuccess(message);
}
