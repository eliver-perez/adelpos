import { IconLabel } from '../../../components/Icon';
import { SegmentedControl } from '../../../components/SegmentedControl';
import type { Bill } from '../../../services/restaurant';
import { adjustmentsAvailable } from '../adjustments/AccountAdjustments';
import type { CheckoutMode } from './CheckoutPanel';

export type CheckoutShortcut = 'adjustments' | 'precheck' | 'split' | 'transfer';

// Acciones contextuales de la pantalla de cobro, en la barra de la vista operativa: cambiar entre
// cobro y devoluciones y abrir los mismos componentes de las acciones de la cuenta.
export function CheckoutToolbar({
  bill,
  mode,
  can,
  onMode,
  onShortcut,
}: {
  bill: Bill;
  mode: CheckoutMode;
  can: (permission: string) => boolean;
  onMode: (mode: CheckoutMode) => void;
  onShortcut: (shortcut: CheckoutShortcut) => void;
}) {
  const account = bill.account;
  const writable =
    account.state === 'open' && (account.mode !== 'takeaway' || can('takeaway.manage'));
  // Con el permiso de cobro se puede solicitar la autorización de un supervisor para devolver.
  const refundAllowed = can('payments.refunds.execute') || can('payments.collect');
  const hasPayments = account.payment_state !== 'not_recorded';
  return (
    <div className="checkout-toolbar" role="group" aria-label="Acciones del cobro">
      {refundAllowed && (
        <SegmentedControl<CheckoutMode>
          label="Operación de la pantalla de cobro"
          value={mode}
          onChange={onMode}
          options={[
            { value: 'collect', label: 'Cobro' },
            {
              value: 'refund',
              label: 'Devoluciones',
              disabled: !hasPayments && mode !== 'refund',
            },
          ]}
        />
      )}
      {can('payments.read') && account.confirmed_quantity > 0 && (
        <button type="button" className="secondary-button" onClick={() => onShortcut('precheck')}>
          <IconLabel icon="receipt">Precuenta</IconLabel>
        </button>
      )}
      {adjustmentsAvailable(account, can) && (
        <button
          type="button"
          className="secondary-button"
          onClick={() => onShortcut('adjustments')}
        >
          <IconLabel icon="discount">Descuentos y cargos</IconLabel>
        </button>
      )}
      {writable && can('accounts.split') && (
        <button type="button" className="secondary-button" onClick={() => onShortcut('split')}>
          <IconLabel icon="split">Dividir cuenta</IconLabel>
        </button>
      )}
      {writable && can('accounts.transfer_items') && (
        <button type="button" className="secondary-button" onClick={() => onShortcut('transfer')}>
          <IconLabel icon="transfer">Transferir productos</IconLabel>
        </button>
      )}
    </div>
  );
}
