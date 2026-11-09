import type { Account, RestaurantTable } from '../../../services/restaurant';
import { serverDateTime } from '../../../services/restaurant';

// Nombre predeterminado de una cuenta de salón (AccountForm); no se repite junto al folio.
const defaultAccountName = 'Cuenta';

function accountLabel(account: Account) {
  const custom = account.name && account.name !== defaultAccountName;
  return custom ? `Cuenta #${account.folio} · ${account.name}` : `Cuenta #${account.folio}`;
}

function openedAt(account: Account, timezone: string) {
  return new Intl.DateTimeFormat('es-MX', { timeStyle: 'short', timeZone: timezone }).format(
    new Date(account.created_at),
  );
}

// Barra superior de la captura: regreso al mapa o al listado, mesa y cuenta en grande con sus
// datos, y las demás cuentas de la mesa como botones (sin listas desplegables).
export function CaptureTopBar({
  account,
  backLabel,
  table,
  tableAccounts,
  timezone,
  canOpenAccount,
  disabled,
  onBack,
  onChooseAccount,
  onNewAccount,
  onRefresh,
}: {
  account: Account | null;
  backLabel: string;
  table: RestaurantTable | null;
  tableAccounts: Account[];
  timezone: string;
  canOpenAccount: boolean;
  disabled: boolean;
  onBack: () => void;
  onChooseAccount: (id: string) => void;
  onNewAccount: () => void;
  onRefresh: () => void;
}) {
  const takeaway = account?.mode === 'takeaway';
  const title = !account
    ? 'Consultando cuenta…'
    : takeaway
      ? `Para llevar · Pedido #${account.folio}`
      : account.table_name
        ? `${account.table_name} · Cuenta #${account.folio}`
        : accountLabel(account);
  const details: string[] = [];
  if (account && takeaway) {
    details.push(account.customer_name || account.name || 'Cliente sin nombre');
    if (account.phone) details.push(account.phone);
    if (account.scheduled_local) {
      const moment = account.scheduled_at
        ? serverDateTime(account.scheduled_at, timezone)
        : account.scheduled_local.replace('T', ' ');
      const kind = account.schedule_kind === 'delivery' ? 'Entrega' : 'Preparación';
      details.push(`${kind} programada ${moment}`);
    } else {
      details.push('Sin hora programada');
    }
    if (account.origin) details.push(account.origin);
  } else if (account) {
    details.push(`Mesero: ${account.assigned_name}`);
    if (table) details.push(`${table.seats} ${table.seats === 1 ? 'lugar' : 'lugares'}`);
    details.push(`Abierta ${openedAt(account, timezone)}`);
    if (account.table_name && account.name && account.name !== defaultAccountName) {
      details.push(account.name);
    }
  }
  return (
    <header className="capture-topbar">
      <button type="button" className="secondary-button capture-topbar-back" onClick={onBack}>
        <span aria-hidden="true">‹</span> {backLabel}
      </button>
      <div className="capture-topbar-title">
        <h1>{title}</h1>
        {details.length > 0 && <p className="muted">{details.join(' · ')}</p>}
      </div>
      {!takeaway && (tableAccounts.length > 1 || canOpenAccount) && (
        <div className="capture-topbar-accounts" role="group" aria-label="Cuentas de la mesa">
          {tableAccounts.length > 1 &&
            tableAccounts.map((entry) => (
              <button
                type="button"
                key={entry.id}
                className="secondary-button"
                aria-pressed={entry.id === account?.id}
                disabled={disabled && entry.id !== account?.id}
                onClick={() => {
                  if (entry.id !== account?.id) onChooseAccount(entry.id);
                }}
              >
                {accountLabel(entry)}
              </button>
            ))}
          {canOpenAccount && (
            <button
              type="button"
              className="secondary-button capture-new-account"
              disabled={disabled}
              onClick={onNewAccount}
            >
              + Nueva cuenta
            </button>
          )}
        </div>
      )}
      <button
        type="button"
        className="secondary-button capture-topbar-refresh"
        aria-label="Actualizar"
        onClick={onRefresh}
      >
        ↻
      </button>
    </header>
  );
}
