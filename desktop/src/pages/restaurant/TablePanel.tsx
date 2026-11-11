import { Notice } from '../../components/Forms';
import { Icon, IconLabel } from '../../components/Icon';
import type { Lookups } from '../../services/catalog';
import { formatMoney } from '../../services/money';
import { accountLabel, elapsedTime, tableStateLabels } from '../../services/restaurant';
import type {
  Account,
  Bill,
  DraftItem,
  RestaurantTable,
  TableState,
} from '../../services/restaurant';
import type { BillAction } from './capture/BillFooter';
import { CheckoutButton, accountMoreActions } from './capture/BillFooter';
import { MoreActions } from './capture/MoreActions';

// Icono que acompaña al texto del estado en el encabezado del panel (no solo color).
const stateIcons: Partial<Record<TableState, 'clock' | 'receipt' | 'cash' | 'lock'>> = {
  occupied: 'clock',
  awaiting_payment: 'receipt',
  partial_payment: 'cash',
  out_of_service: 'lock',
};

function StateChip({ state }: { state: TableState }) {
  const icon = stateIcons[state];
  return (
    <span className={`table-panel-state table-panel-state-${state}`}>
      {icon && <Icon name={icon} size={14} />}
      {tableStateLabels[state]}
    </span>
  );
}

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

// Totales de una mesa sin cuenta: la misma fila que en una mesa ocupada, en cero.
function EmptyTotals() {
  return (
    <dl className="table-panel-totals" aria-label="Totales de la cuenta · precios con IVA incluido">
      <div>
        <dt>Total</dt>
        <dd>{formatMoney('0.00')}</dd>
      </div>
      <div>
        <dt>Pagado</dt>
        <dd>{formatMoney('0.00')}</dd>
      </div>
      <div className="table-panel-balance">
        <dt>Saldo</dt>
        <dd>{formatMoney('0.00')}</dd>
      </div>
    </dl>
  );
}

// Panel lateral de la mesa seleccionada en el mapa (etapa 6.2): nombre, estado y tiempo abierta;
// cuentas abiertas como botones; productos, totales y acciones de la cuenta elegida.
//
// Etapa 7.1: la mesa libre tiene la misma estructura (encabezado con su estado, área de cuenta
// vacía, totales en $0.00 y «Capturar» en el mismo lugar; Cobrar, Precuenta y Dividir quedan
// desactivados). «Capturar» y «Nueva cuenta» entran a la captura sin crear la cuenta: Go la crea
// con el primer producto. «Liberar mesa» solo aparece si Go marcó la cuenta como liberable, y
// «Poner fuera de servicio» / «Habilitar» requieren administrar mesas.
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
  onRelease,
  onService,
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
  onRelease: (account: Account) => void;
  onService: (outOfService: boolean) => void;
}) {
  const open = accounts ?? [];
  const occupied = open.length > 0;
  // El estado lo calcula Go; si el mapa aún no refleja una cuenta recién abierta, se muestra ocupada.
  const state: TableState = occupied && table.state === 'free' ? 'occupied' : table.state;
  const oldest = open.reduce((seconds, account) => Math.max(seconds, account.elapsed_seconds), 0);
  const inService = table.active && !table.out_of_service;
  const canOpen = can('accounts.open') && inService;
  const canManage = can('restaurant.tables.manage') && table.active;
  // Solo se muestra la cuenta elegida si sigue abierta en esta mesa.
  const current = bill && open.some((account) => account.id === bill.account.id) ? bill : null;
  const account = current?.account ?? null;
  const writable =
    !!account &&
    account.state === 'open' &&
    (account.mode !== 'takeaway' || can('takeaway.manage'));
  const activeLines = current?.lines.filter((line) => line.state === 'active').length ?? 0;
  // «Liberar mesa»: lo decide Go (releasable) y no debe haber productos nuevos sin guardar.
  const releasable =
    !!account && account.releasable && can('accounts.open') && localDraft.length === 0;
  // Habilitar procede siempre; poner fuera de servicio solo en una mesa sin cuentas abiertas.
  const serviceButton = canManage ? (
    table.out_of_service ? (
      <button
        type="button"
        className="secondary-button"
        disabled={blocked}
        onClick={() => onService(false)}
      >
        <IconLabel icon="unlock">Habilitar</IconLabel>
      </button>
    ) : !occupied && accounts !== null ? (
      // Etapa 9A (§33): el texto largo va en su propia fila de las acciones secundarias, a todo el
      // ancho del panel, para que quepa en una línea en cualquier tamaño de ventana.
      <button
        type="button"
        className="secondary-button table-service-action"
        disabled={blocked}
        onClick={() => onService(true)}
      >
        <IconLabel icon="lock">Poner fuera de servicio</IconLabel>
      </button>
    ) : null
  ) : null;
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
        <StateChip state={state} />
      </header>
      {accounts === null ? (
        <p className="muted table-panel-body">Consultando cuentas de la mesa…</p>
      ) : !occupied ? (
        <>
          <div className="table-panel-body table-panel-free">
            {!inService && (
              <Notice>La mesa está fuera de servicio y no admite cuentas nuevas.</Notice>
            )}
            <h3 className="table-panel-account">Sin cuenta abierta</h3>
            <p className="muted table-panel-empty-lines">
              {inService
                ? 'La cuenta se abre al guardar el primer producto; si sales sin capturar, la mesa sigue libre.'
                : 'Habilita la mesa para capturar en ella.'}
            </p>
          </div>
          <footer className="table-panel-footer">
            <EmptyTotals />
            <div className="capture-primary-actions">
              <button
                type="button"
                className="primary-button capture-send"
                disabled={blocked || !canOpen}
                onClick={onOpenAccount}
              >
                <IconLabel icon="edit">Capturar</IconLabel>
              </button>
              {can('payments.read') && can('payments.collect') && (
                <button type="button" className="primary-button capture-checkout" disabled>
                  <IconLabel icon="cash">Cobrar</IconLabel>
                </button>
              )}
            </div>
            <div
              className="capture-secondary-actions"
              role="group"
              aria-label="Acciones de la cuenta"
            >
              {can('payments.read') && (
                <button type="button" className="secondary-button" disabled>
                  <IconLabel icon="receipt">Precuenta</IconLabel>
                </button>
              )}
              {can('accounts.split') && (
                <button type="button" className="secondary-button" disabled>
                  <IconLabel icon="split">Dividir</IconLabel>
                </button>
              )}
              {serviceButton}
            </div>
          </footer>
        </>
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
                {releasable && (
                  <button
                    type="button"
                    className="secondary-button"
                    disabled={blocked}
                    onClick={() => onRelease(account)}
                  >
                    <IconLabel icon="unlock">Liberar mesa</IconLabel>
                  </button>
                )}
                {serviceButton}
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
