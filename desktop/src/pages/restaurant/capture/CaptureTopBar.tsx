import { Icon, IconLabel } from '../../../components/Icon';
import type { Account, RestaurantTable } from '../../../services/restaurant';
import {
  accountCustomName,
  accountLabel,
  serverDateTime,
  takeawayCustomerName,
} from '../../../services/restaurant';

function openedAt(account: Account, timezone: string) {
  return new Intl.DateTimeFormat('es-MX', { timeStyle: 'short', timeZone: timezone }).format(
    new Date(account.created_at),
  );
}

// Título de la captura: la cuenta se identifica por su folio; la referencia neutra que Go asigna a
// una cuenta sin nombre no se presenta como nombre capturado.
function captureTitle(account: Account | null) {
  if (!account) {
    return 'Consultando cuenta…';
  }
  if (account.mode === 'takeaway') {
    return `Para llevar · Pedido #${account.folio}`;
  }
  if (account.table_name) {
    return `${account.table_name} · Cuenta #${account.folio}`;
  }
  return accountLabel(account);
}

function captureDetails(account: Account, table: RestaurantTable | null, timezone: string) {
  const details: string[] = [];
  if (account.mode === 'takeaway') {
    details.push(takeawayCustomerName(account) || 'Sin nombre del cliente');
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
    return details;
  }
  details.push(`Mesero: ${account.assigned_name}`);
  // La cuenta no guarda comensales (pendiente de migración): se informa la capacidad de la mesa.
  if (table) details.push(`Mesa para ${table.seats}`);
  details.push(`Abierta ${openedAt(account, timezone)}`);
  const custom = accountCustomName(account);
  if (account.table_name && custom) {
    details.push(custom);
  }
  return details;
}

// Barra superior de la captura: regreso al mapa o al listado, mesa y cuenta en grande con sus
// datos, «Datos de la cuenta» (o «Datos del pedido») y las demás cuentas de la mesa como botones
// (sin listas desplegables). Tocar el título también abre los datos.
export function CaptureTopBar({
  account,
  backLabel,
  table,
  tableAccounts,
  timezone,
  canOpenAccount,
  canEditDetails,
  disabled,
  onBack,
  onChooseAccount,
  onNewAccount,
  onDetails,
  onRefresh,
}: {
  account: Account | null;
  backLabel: string;
  table: RestaurantTable | null;
  tableAccounts: Account[];
  timezone: string;
  canOpenAccount: boolean;
  canEditDetails: boolean;
  disabled: boolean;
  onBack: () => void;
  onChooseAccount: (id: string) => void;
  onNewAccount: () => void;
  onDetails: () => void;
  onRefresh: () => void;
}) {
  const takeaway = account?.mode === 'takeaway';
  const title = captureTitle(account);
  const details = account ? captureDetails(account, table, timezone) : [];
  // Un pedido para llevar enviado a cocina sin nombre no se bloquea: se avisa con discreción y se
  // identifica por su número de pedido.
  const unnamedSent =
    !!account && takeaway && account.confirmed_quantity > 0 && !takeawayCustomerName(account);
  const detailsLabel = takeaway ? 'Datos del pedido' : 'Datos de la cuenta';
  return (
    <header className="capture-topbar">
      <button type="button" className="secondary-button capture-topbar-back" onClick={onBack}>
        <IconLabel icon="back">{backLabel}</IconLabel>
      </button>
      <div className="capture-topbar-title">
        <h1>
          {account && canEditDetails ? (
            <button
              type="button"
              className="capture-topbar-title-button"
              title={detailsLabel}
              disabled={disabled}
              onClick={onDetails}
            >
              {title}
            </button>
          ) : (
            title
          )}
        </h1>
        {details.length > 0 && <p className="muted">{details.join(' · ')}</p>}
      </div>
      {unnamedSent && (
        <p className="capture-unnamed" role="status">
          Pedido sin nombre · se identifica como Pedido #{account.folio}
        </p>
      )}
      {account && canEditDetails && (
        <button
          type="button"
          className="secondary-button capture-topbar-details"
          disabled={disabled}
          onClick={onDetails}
        >
          {detailsLabel}
        </button>
      )}
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
              <IconLabel icon="plus">Nueva cuenta</IconLabel>
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
        <Icon name="refresh" size={20} />
      </button>
    </header>
  );
}
