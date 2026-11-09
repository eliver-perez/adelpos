import { Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import type { Lookups } from '../../services/catalog';
import { formatMoney } from '../../services/money';
import { accountLabel, elapsedTime } from '../../services/restaurant';
import type { Account, Bill, DraftItem, RestaurantTable } from '../../services/restaurant';
import type { BillAction } from './capture/BillFooter';
import { CheckoutButton, accountMoreActions } from './capture/BillFooter';
import { MoreActions } from './capture/MoreActions';

const stateLabels = { free: 'Libre', occupied: 'Ocupada', out_of_service: 'Fuera de servicio' };

// Productos de la cuenta en el panel: enviados con «En cocina» y su importe de Go; nuevos (aún sin
// enviar) con «Nuevo», sin importe (la cotización vive en la captura).
function PanelLines({
  bill,
  localDraft,
  lookups,
}: {
  bill: Bill;
  localDraft: DraftItem[];
  lookups: Lookups | null;
}) {
  const active = bill.lines.filter((line) => line.state === 'active');
  if (active.length === 0 && localDraft.length === 0) {
    return <p className="muted table-panel-empty-lines">La cuenta aún no tiene productos.</p>;
  }
  return (
    <ul className="table-panel-lines" aria-label="Productos de la cuenta">
      {active.map((line) => (
        <li key={line.version_id} className="table-panel-line">
          <span className="table-panel-line-name">
            {line.quantity} × {line.quote_line.name}
          </span>
          <span className="capture-tag">En cocina</span>
          <strong className="table-panel-line-total">{formatMoney(line.totals.total)}</strong>
        </li>
      ))}
      {localDraft.map((item) => {
        const name =
          lookups?.products.find((product) => product.id === item.product_id)?.name ??
          'Producto del catálogo';
        return (
          <li key={item.id} className="table-panel-line table-panel-line-new">
            <span className="table-panel-line-name">
              {item.quantity} × {name}
            </span>
            <span className="capture-tag capture-tag-new">Nuevo</span>
            <span className="table-panel-line-total muted">Sin enviar</span>
          </li>
        );
      })}
    </ul>
  );
}

// Totales, pagado y saldo tal como los calcula Go (payments); la interfaz no suma importes.
function PanelTotals({ account }: { account: Account }) {
  const finance = account.finance;
  return (
    <dl className="table-panel-totals" aria-label="Totales de la cuenta · precios con IVA incluido">
      {finance ? (
        <>
          <div>
            <dt>Total</dt>
            <dd>{formatMoney(finance.total)}</dd>
          </div>
          <div>
            <dt>Pagado</dt>
            <dd>{formatMoney(finance.paid)}</dd>
          </div>
          <div className="table-panel-balance">
            <dt>Saldo</dt>
            <dd>{formatMoney(finance.balance)}</dd>
          </div>
        </>
      ) : (
        <div className="table-panel-balance">
          <dt>Enviado a cocina</dt>
          <dd>{formatMoney(account.totals.total)}</dd>
        </div>
      )}
    </dl>
  );
}

// Panel lateral de la mesa seleccionada en el mapa (etapa 6.2): nombre, estado y tiempo abierta;
// cuentas abiertas como botones; productos, totales y acciones de la cuenta elegida. Mesa libre:
// «Abrir cuenta y capturar» abre la cuenta sin formulario y entra directo a la captura.
export function TablePanel({
  table,
  accounts,
  bill,
  billError,
  localDraft,
  lookups,
  can,
  blocked,
  onChooseAccount,
  onCapture,
  onOpenAccount,
  onAction,
  onCheckout,
}: {
  table: RestaurantTable;
  // Cuentas abiertas de la mesa; null mientras se consultan.
  accounts: Account[] | null;
  bill: Bill | null;
  billError: string;
  localDraft: DraftItem[];
  lookups: Lookups | null;
  can: (permission: string) => boolean;
  blocked: boolean;
  onChooseAccount: (id: string) => void;
  onCapture: () => void;
  onOpenAccount: () => void;
  onAction: (action: BillAction) => void;
  onCheckout: () => void;
}) {
  const open = accounts ?? [];
  const occupied = open.length > 0;
  const state = occupied ? 'occupied' : table.state;
  const oldest = open.reduce((seconds, account) => Math.max(seconds, account.elapsed_seconds), 0);
  const canOpen = can('accounts.open') && table.active && !table.out_of_service;
  // Solo se muestra la cuenta elegida si sigue abierta en esta mesa.
  const current = bill && open.some((account) => account.id === bill.account.id) ? bill : null;
  const account = current?.account ?? null;
  const writable =
    !!account &&
    account.state === 'open' &&
    (account.mode !== 'takeaway' || can('takeaway.manage'));
  const activeLines = current?.lines.filter((line) => line.state === 'active').length ?? 0;
  // El estado se muestra en la etiqueta del encabezado; aquí, capacidad y tiempo abierta.
  const details = [`Mesa para ${table.seats}`];
  if (occupied) {
    details.push(`Abierta hace ${elapsedTime(oldest)}`);
  }

  return (
    <aside className="table-panel" aria-label={`Panel de ${table.name}`}>
      <header className="table-panel-header">
        <div>
          <h2>{table.name}</h2>
          <p className="muted">{details.join(' · ')}</p>
        </div>
        <span className={`table-panel-state table-panel-state-${state}`}>{stateLabels[state]}</span>
      </header>
      {accounts === null ? (
        <p className="muted table-panel-body">Consultando cuentas de la mesa…</p>
      ) : !occupied ? (
        <div className="table-panel-body table-panel-free">
          {table.out_of_service || !table.active ? (
            <Notice>La mesa está fuera de servicio y no admite cuentas nuevas.</Notice>
          ) : (
            <p className="muted">Mesa libre. Abre una cuenta y captura sin llenar formularios.</p>
          )}
          {canOpen && (
            <button
              type="button"
              className="primary-button table-panel-primary"
              disabled={blocked}
              onClick={onOpenAccount}
            >
              <IconLabel icon="plus">Abrir cuenta y capturar</IconLabel>
            </button>
          )}
        </div>
      ) : (
        <>
          {(open.length > 1 || !current || canOpen) && (
            <div
              className="table-panel-accounts"
              role="group"
              aria-label="Cuentas abiertas de la mesa"
            >
              {(open.length > 1 || !current ? open : []).map((entry) => (
                <button
                  type="button"
                  key={entry.id}
                  className="secondary-button"
                  aria-pressed={entry.id === account?.id}
                  disabled={blocked && entry.id !== account?.id}
                  onClick={() => {
                    if (entry.id !== account?.id) onChooseAccount(entry.id);
                  }}
                >
                  {accountLabel(entry)}
                </button>
              ))}
              {canOpen && (
                <button
                  type="button"
                  className="secondary-button capture-new-account"
                  disabled={blocked}
                  onClick={onOpenAccount}
                >
                  <IconLabel icon="plus">Nueva cuenta</IconLabel>
                </button>
              )}
            </div>
          )}
          <div className="table-panel-body">
            <Notice>{billError}</Notice>
            {current && account ? (
              <>
                <h3 className="table-panel-account">{accountLabel(account)}</h3>
                <p className="muted text-xs">Mesero: {account.assigned_name}</p>
                <PanelLines bill={current} localDraft={localDraft} lookups={lookups} />
                {account.notes && (
                  <p className="bill-account-notes">Observaciones: {account.notes}</p>
                )}
              </>
            ) : (
              <p className="muted">Elige una cuenta de la mesa.</p>
            )}
          </div>
          {current && account && (
            <footer className="table-panel-footer">
              <PanelTotals account={account} />
              <div className="capture-primary-actions">
                <button
                  type="button"
                  className="primary-button capture-send"
                  disabled={blocked}
                  onClick={onCapture}
                >
                  <IconLabel icon="edit">Capturar</IconLabel>
                </button>
                <CheckoutButton
                  account={account}
                  can={can}
                  disabled={blocked}
                  onCheckout={onCheckout}
                />
              </div>
              <div
                className="capture-secondary-actions"
                role="group"
                aria-label="Acciones de la cuenta"
              >
                {can('payments.read') && (
                  <button
                    type="button"
                    className="secondary-button"
                    disabled={blocked || account.confirmed_quantity === 0}
                    onClick={() => onAction('precheck')}
                  >
                    <IconLabel icon="receipt">Precuenta</IconLabel>
                  </button>
                )}
                {writable && can('accounts.split') && (
                  <button
                    type="button"
                    className="secondary-button"
                    disabled={blocked || activeLines === 0}
                    onClick={() => onAction('split')}
                  >
                    <IconLabel icon="split">Dividir</IconLabel>
                  </button>
                )}
                <MoreActions
                  actions={accountMoreActions(account, can, localDraft.length)}
                  disabled={blocked}
                  onAction={onAction}
                />
              </div>
            </footer>
          )}
        </>
      )}
    </aside>
  );
}
