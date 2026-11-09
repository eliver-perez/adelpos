import {
  cashPrintMessage,
  currencyLabels,
  drawerMessage,
  printAttentionStatuses,
} from '../../services/cash';
import type { CashLimitWarning, DrawerOutcome, PrintOutcome } from '../../services/cash';

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
