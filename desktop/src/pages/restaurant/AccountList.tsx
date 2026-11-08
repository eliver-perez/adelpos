import { preparationLabels } from '../../services/kitchen';
import { Field, Notice } from '../../components/Forms';
import { elapsedTime, serverDateTime } from '../../services/restaurant';
import type { AccountPage } from '../../services/restaurant';

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
          <select
            value={state}
            onChange={(event) => {
              onStateChange(event.target.value);
              onPageChange(1);
            }}
          >
            <option value="open">Abiertas</option>
            <option value="all">Todas con historial</option>
            <option value="merged">Combinadas</option>
            <option value="voided">Anuladas sin productos activos</option>
          </select>
        </Field>
      </div>
      {takeaway && (
        <p className="muted text-xs">Más antiguos primero · Antigüedad calculada por el servidor</p>
      )}
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
          <button
            key={account.id}
            className={`account-list-card ${selectedID === account.id ? 'account-list-selected' : ''} ${takeaway ? `account-urgency-${account.urgency}` : ''}`}
            aria-pressed={selectedID === account.id}
            onClick={() => onSelect(account.id)}
          >
            <div className="account-list-card-heading">
              <strong>
                #{account.folio} · {account.customer_name || account.name}
              </strong>
              <strong>Confirmado: ${account.totals.total}</strong>
            </div>
            <div className="account-list-card-detail">
              <span>{serverDateTime(account.created_at, value.timezone)}</span>
              <strong>{elapsedTime(account.elapsed_seconds)}</strong>
            </div>
            <p>{preparationLabels[account.preparation_state] ?? account.preparation_state}</p>
            <div className="account-list-card-detail">
              <span>{takeaway ? account.origin : (account.table_name ?? 'Sin mesa')}</span>
              <span>
                {account.state === 'open'
                  ? `${account.confirmed_quantity} unidades confirmadas`
                  : account.state === 'merged'
                    ? 'Cuenta combinada'
                    : 'Cuenta anulada'}
              </span>
            </div>
            {account.scheduled_at && (
              <p>
                Programado: {serverDateTime(account.scheduled_at, value.timezone)} ·{' '}
                {account.schedule_kind === 'delivery' ? 'entrega prevista' : 'preparación'}
              </p>
            )}
            {takeaway && account.urgency !== 'normal' && (
              <span className="account-age-label">
                {account.urgency === 'overdue' ? 'Antigüedad alta' : 'Revisar antigüedad'}
              </span>
            )}
          </button>
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
          {value?.total ?? 0} cuentas · Página {page}
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
