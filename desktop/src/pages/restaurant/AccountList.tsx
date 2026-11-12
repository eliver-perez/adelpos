import { preparationLabels } from '../../services/kitchen';
import { Field, Notice } from '../../components/Forms';
import {
  accountLabel,
  defaultTakeawayOrigin,
  elapsedTime,
  serverDateTime,
} from '../../services/restaurant';
import type { Account, AccountPage } from '../../services/restaurant';
import { formatMoney } from '../../services/money';
import { deliveryStateBadges, paymentStateBadges } from '../../services/payments';
import { countLabel } from '../../services/plural';
import { Select } from '../../components/Select';
import { Icon } from '../../components/Icon';
import { longOpenText } from '../../components/restaurant/TableMap';

// Cuenta abierta más horas que el umbral del establecimiento (§3.4); la marca la calcula Go.
function longOpen(account: Account) {
  return account.state === 'open' && account.long_open;
}

// Etapa 9D (§4.3): la tarjeta lleva los estados como etiquetas cortas en un solo renglón; el texto
// completo de la preparación queda en el título de su etiqueta.
const preparationTags: Record<string, string> = {
  without_items: 'Sin productos',
  draft: 'Borrador',
  confirmed_pending_dispatch: 'Por liberar',
  pending_release: 'Sin confirmar en cocina',
  scheduled: 'Programado',
  released: 'En cocina',
  preparing: 'En preparación',
  ready: 'Listo',
  change_pending: 'Cambio pendiente',
  cancelled: 'Cancelado',
};

// Estado de una cuenta que ya no está abierta (en los filtros de cerradas, combinadas o canceladas).
function closedStateText(account: Account) {
  if (account.state === 'closed') {
    return 'Cerrada · liquidada';
  }
  if (account.state === 'merged') {
    return 'Combinada';
  }
  // Etapa 9D: el estado cerrado sin venta reúne las cuentas canceladas y las vacías descartadas.
  return 'Cuenta cancelada o descartada';
}

// Mesa de la cuenta o, en un pedido para llevar, su origen cuando no es el de siempre. El cliente
// del pedido ya va en el nombre de la tarjeta.
function placeText(account: Account, takeaway: boolean) {
  if (takeaway || account.mode === 'takeaway') {
    return account.origin && account.origin !== defaultTakeawayOrigin ? account.origin : '';
  }
  return account.table_name ?? 'Sin mesa';
}

// Pago y entrega se muestran separados de la preparación, con los estados que calcula Go.
function FinanceBadges({ account, takeaway }: { account: Account; takeaway: boolean }) {
  return (
    <>
      <span className={`finance-badge finance-badge-${account.payment_state}`}>
        Pago: {paymentStateBadges[account.payment_state] ?? account.payment_state}
      </span>
      {account.payment_state === 'partial' && account.finance && (
        <span className="finance-badge">Saldo {formatMoney(account.finance.balance)}</span>
      )}
      {takeaway && (
        <span className={`finance-badge finance-badge-${account.delivery_state}`}>
          Entrega: {deliveryStateBadges[account.delivery_state] ?? account.delivery_state}
        </span>
      )}
    </>
  );
}

// Tarjeta compacta de Cuentas y Para llevar (etapa 9D, §4.3), en dos renglones: folio, mesa o
// cliente, tiempo e importe; debajo, preparación, pago, entrega y unidades como etiquetas. Sin la
// fecha de apertura (el tiempo transcurrido ya la dice) ni textos repetidos.
function AccountCard({
  account,
  selected,
  takeaway,
  timezone,
  longOpenHours,
  onSelect,
}: {
  account: Account;
  selected: boolean;
  takeaway: boolean;
  timezone: string;
  longOpenHours?: number;
  onSelect: (id: string) => void;
}) {
  const place = placeText(account, takeaway);
  const preparation = preparationLabels[account.preparation_state] ?? account.preparation_state;
  return (
    <button
      className={`account-list-card ${selected ? 'account-list-selected' : ''} ${takeaway ? `account-urgency-${account.urgency}` : ''} ${longOpen(account) ? 'account-long-open' : ''}`}
      aria-pressed={selected}
      onClick={() => onSelect(account.id)}
    >
      <span className="account-card-line">
        <strong className="account-card-title">{accountLabel(account)}</strong>
        {place && <span className="account-card-place">{place}</span>}
        <span className="account-card-figures">
          {/* Etapa 9D (§3.4): abierta más horas que el umbral (long_open, de Go): el tiempo en
              ámbar con el triángulo de advertencia. */}
          {longOpen(account) ? (
            <strong
              className="account-card-time account-long-open-time"
              title={longOpenText(longOpenHours)}
            >
              <Icon name="alert" size={13} />
              {elapsedTime(account.elapsed_seconds)}
              <span className="sr-only"> · {longOpenText(longOpenHours)}</span>
            </strong>
          ) : (
            <span className="account-card-time">{elapsedTime(account.elapsed_seconds)}</span>
          )}
          <strong className="account-card-amount">{formatMoney(account.totals.total)}</strong>
        </span>
      </span>
      <span className="account-card-tags">
        {account.state !== 'open' && (
          <span className="finance-badge account-card-state">{closedStateText(account)}</span>
        )}
        <span
          className={`finance-badge account-card-preparation account-preparation-${account.preparation_state}`}
          title={`Preparación: ${preparation}`}
        >
          {preparationTags[account.preparation_state] ?? preparation}
        </span>
        <FinanceBadges account={account} takeaway={takeaway} />
        {/* Unidades enviadas o, sin ellas, las que esperan en el borrador; sin ninguna, la etiqueta
            de preparación («Sin productos») ya lo dice. */}
        {account.state === 'open' && account.confirmed_quantity > 0 && (
          <span className="finance-badge">
            {countLabel(account.confirmed_quantity, 'unidad', 'unidades')}
          </span>
        )}
        {account.state === 'open' &&
          account.confirmed_quantity === 0 &&
          account.draft_quantity > 0 && (
            <span className="finance-badge">{account.draft_quantity} sin enviar</span>
          )}
        {account.scheduled_at && (
          <span className="finance-badge">
            {account.schedule_kind === 'delivery' ? 'Entrega' : 'Preparación'}{' '}
            {serverDateTime(account.scheduled_at, timezone)}
          </span>
        )}
        {takeaway && account.urgency !== 'normal' && (
          <span className={`finance-badge account-age-label account-age-${account.urgency}`}>
            {account.urgency === 'overdue' ? 'Antigüedad alta' : 'Revisar antigüedad'}
          </span>
        )}
      </span>
    </button>
  );
}

export function AccountList({
  value,
  selectedID,
  onSelect,
  search,
  onSearch,
  state,
  onStateChange,
  page,
  onPageChange,
  error,
  takeaway = false,
}: {
  value: AccountPage | null;
  selectedID: string;
  onSelect: (id: string) => void;
  search: string;
  onSearch: (value: string) => void;
  state: string;
  onStateChange: (value: string) => void;
  page: number;
  onPageChange: (value: number) => void;
  error: string;
  takeaway?: boolean;
}) {
  return (
    <section className="operation-account-list">
      <div className="account-list-filters">
        <Field label={takeaway ? 'Buscar folio, cliente o teléfono' : 'Buscar cuenta o folio'}>
          <input
            type="search"
            maxLength={100}
            value={search}
            onChange={(event) => {
              onSearch(event.target.value);
              onPageChange(1);
            }}
          />
        </Field>
        <Field label="Estado de cuenta">
          <Select
            value={state}
            options={[
              { value: 'open', label: 'Abiertas' },
              { value: 'closed', label: 'Cerradas y liquidadas' },
              { value: 'all', label: 'Todas con historial' },
              { value: 'merged', label: 'Combinadas' },
              { value: 'voided', label: 'Canceladas o descartadas' },
            ]}
            onChange={(next) => {
              onStateChange(next);
              onPageChange(1);
            }}
          />
        </Field>
      </div>
      {takeaway && <p className="muted text-xs">Más antiguos primero</p>}
      <Notice>{error}</Notice>
      {!value && !error && <p className="muted">Consultando cuentas…</p>}
      {value?.items.length === 0 && (
        <p className="muted">No hay cuentas que coincidan con este filtro.</p>
      )}
      <div
        className="account-list-items"
        role="group"
        aria-label={takeaway ? 'Pedidos para llevar' : 'Cuentas disponibles'}
      >
        {value?.items.map((account) => (
          <AccountCard
            key={account.id}
            account={account}
            selected={selectedID === account.id}
            takeaway={takeaway}
            timezone={value.timezone}
            longOpenHours={value.settings?.long_open_hours}
            onSelect={onSelect}
          />
        ))}
      </div>
      <div className="operation-product-pagination">
        <button
          className="secondary-button"
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          Anterior
        </button>
        <span className="muted text-xs">
          {countLabel(value?.total ?? 0, 'cuenta', 'cuentas')} · Página {page}
        </span>
        <button
          className="secondary-button"
          disabled={!value || page * value.page_size >= value.total}
          onClick={() => onPageChange(page + 1)}
        >
          Siguiente
        </button>
      </div>
    </section>
  );
}
