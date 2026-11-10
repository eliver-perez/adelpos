import { useState } from 'react';
import type { FormEvent } from 'react';
import { OptionalDateField } from '../../components/catalog/DateField';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, Page } from '../../services/inventory';
import { linkCashExpense, unlinkCashExpense } from '../../services/inventoryPurchasing';
import type {
  CashExpense,
  CashLink,
  CashShiftState,
  Receipt,
} from '../../services/inventoryPurchasing';
import { formatMoney } from '../../services/money';
import { notifyError, notifySuccess } from '../../services/notify';
import { amountShape } from '../../services/numericInput';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { DecimalTextField, Pagination } from './InventoryFields';
import { ReasonAction } from './PurchaseFields';
import {
  canLinkCash,
  cashLinkTone,
  isZeroText,
  purchasingPermissionName,
  receiptFolio,
  uncertainResult,
} from './purchasingFormat';

const pageSize = 20;

const shiftStateLabels: Record<CashShiftState, string> = {
  open: 'Turno abierto',
  closing: 'Turno en arqueo',
  closed: 'Turno cerrado',
};

// Descripción legible de un gasto: concepto, categoría, caja y fecha (sin identificadores).
function expenseTitle(expense: CashExpense) {
  const parts = [expense.concept];
  if (expense.category_name && expense.category_name !== expense.concept) {
    parts.push(expense.category_name);
  }
  return parts.join(' · ');
}

function expenseDetail(expense: CashExpense, timezone: string) {
  const parts = [
    expense.register_name,
    shiftStateLabels[expense.shift_state],
    serverDateTime(expense.created_at, timezone),
    expense.actor_name,
  ];
  if (expense.reference) {
    parts.push(`Ref. ${expense.reference}`);
  }
  return parts.join(' · ');
}

interface ExpenseFilter {
  from: string;
  until: string;
  min: string;
  max: string;
  page: number;
}

function expenseQuery(receipt: Receipt, filter: ExpenseFilter) {
  const query: Record<string, string> = {
    receipt_id: receipt.id,
    page: String(filter.page),
    page_size: String(pageSize),
  };
  if (filter.from !== '') {
    query.from = filter.from;
  }
  if (filter.until !== '') {
    query.until = filter.until;
  }
  if (filter.min !== '') {
    query.min_amount = filter.min;
  }
  if (filter.max !== '') {
    query.max_amount = filter.max;
  }
  return query;
}

// Vínculo total o parcial con el gasto elegido. El importe se captura (o se copia de lo disponible
// del gasto o de lo pendiente de la recepción, tal como lo entrega Go); Go valida que no exceda
// ninguno de los dos y explica el rechazo.
function LinkExpenseForm({
  receipt,
  expense,
  execute,
  busy,
  timezone,
  onLinked,
  onCancel,
}: {
  receipt: Receipt;
  expense: CashExpense;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onLinked: () => void;
  onCancel: () => void;
}) {
  const [amount, setAmount] = useState('');
  const [notes, setNotes] = useState('');
  const [sending, setSending] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const locked = busy || sending || uncertain;
  const folio = receiptFolio(receipt.folio);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const confirmed = await confirm({
      title: 'Vincular gasto de caja',
      message: `Se vinculará ${formatMoney(amount)} del gasto «${expenseTitle(expense)}» (${expense.register_name}) con la ${folio.toLowerCase()} de «${receipt.supplier.trade_name}». El vínculo no modifica el gasto ni el turno de caja.`,
      confirmLabel: 'Vincular gasto',
      cancelLabel: 'Revisar',
    });
    if (!confirmed) {
      return;
    }
    setSending(true);
    try {
      const saved = await linkCashExpense(execute, receipt.id, {
        cash_operation_id: expense.id,
        amount,
        ...(notes.trim() !== '' ? { notes } : {}),
      });
      notifySuccess(`Gasto vinculado con la ${receiptFolio(saved.folio).toLowerCase()}.`, {
        description: `Vinculado en total: ${formatMoney(saved.linked_amount)} · por vincular: ${formatMoney(saved.linkable_amount)}.`,
      });
      onLinked();
    } catch (problem) {
      if (uncertainResult(problem)) {
        setUncertain(true);
      }
      notifyError(problem, 'No se pudo vincular el gasto.');
    } finally {
      setSending(false);
    }
  }

  return (
    <form
      className="repeat-card form-stack"
      aria-label="Vincular el gasto elegido"
      onSubmit={(event) => void submit(event)}
    >
      <div className="section-heading inventory-line-heading">
        <div>
          <strong>{expenseTitle(expense)}</strong>
          <p className="muted text-xs">{expenseDetail(expense, timezone)}</p>
        </div>
        <strong>{formatMoney(expense.amount, expense.currency)}</strong>
      </div>
      {uncertain && (
        <div className="inventory-negative-notice" role="status">
          <p>
            No se pudo confirmar el vínculo. Recupéralo con «Reintentar con el mismo identificador»
            (arriba) antes de intentarlo otra vez.
          </p>
        </div>
      )}
      <fieldset className="form-stack" disabled={locked}>
        <div className="form-grid">
          <DecimalTextField
            label="Importe a vincular"
            value={amount}
            onChange={setAmount}
            shape={amountShape}
            required
            hint="Vínculo total o parcial, en pesos."
          />
          <Field label="Nota del vínculo" hint="Opcional; hasta 500 caracteres.">
            <input
              maxLength={500}
              value={notes}
              onChange={(event) => setNotes(event.target.value)}
            />
          </Field>
        </div>
        <div className="button-row">
          <button
            type="button"
            className="secondary-button"
            onClick={() => setAmount(expense.available_amount)}
          >
            Disponible del gasto: {formatMoney(expense.available_amount)}
          </button>
          <button
            type="button"
            className="secondary-button"
            onClick={() => setAmount(receipt.linkable_amount)}
          >
            Por vincular de la recepción: {formatMoney(receipt.linkable_amount)}
          </button>
        </div>
      </fieldset>
      <div className="button-row">
        <button
          type="submit"
          className="primary-button"
          disabled={locked || amount === '' || isZeroText(amount)}
        >
          {sending ? 'Vinculando…' : 'Vincular gasto'}
        </button>
        <button type="button" className="secondary-button" onClick={onCancel}>
          Elegir otro gasto
        </button>
      </div>
      {dialog}
    </form>
  );
}

// Búsqueda de gastos candidatos (solo kind «expense» que la sesión puede leer con las reglas de
// caja), el más reciente primero. Go marca cuáles se pueden vincular y por qué no los demás
// (en dólares, corregidos o vinculados por completo).
function CashExpenseSearch({
  receipt,
  execute,
  busy,
  timezone,
  onClose,
}: {
  receipt: Receipt;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onClose: () => void;
}) {
  const [filter, setFilter] = useState<ExpenseFilter>({
    from: '',
    until: '',
    min: '',
    max: '',
    page: 1,
  });
  const [selected, setSelected] = useState<CashExpense | null>(null);
  const list = useResource<Page<CashExpense>>('inventory_cash_expenses', {
    query: expenseQuery(receipt, filter),
    reloadEvent: inventoryChangeEvent,
  });

  if (selected) {
    return (
      <LinkExpenseForm
        receipt={receipt}
        expense={selected}
        execute={execute}
        busy={busy}
        timezone={timezone}
        onLinked={onClose}
        onCancel={() => setSelected(null)}
      />
    );
  }

  return (
    <section className="repeat-card form-stack" aria-label="Gastos de caja para vincular">
      <div className="section-heading inventory-line-heading">
        <div>
          <strong>Gastos de caja para vincular</strong>
          <p className="muted text-xs">
            Por vincular de esta recepción: {formatMoney(receipt.linkable_amount)}. Solo aparecen
            los gastos que tus permisos de caja permiten consultar.
          </p>
        </div>
        <button type="button" className="secondary-button" onClick={onClose}>
          Cerrar búsqueda
        </button>
      </div>
      <div className="catalog-filters inventory-inline-filters">
        <OptionalDateField
          label="Gastos desde"
          value={filter.from}
          onChange={(from) => setFilter({ ...filter, from, page: 1 })}
        />
        <OptionalDateField
          label="Gastos hasta"
          value={filter.until}
          onChange={(until) => setFilter({ ...filter, until, page: 1 })}
        />
        <DecimalTextField
          label="Importe mínimo"
          value={filter.min}
          onChange={(min) => setFilter({ ...filter, min, page: 1 })}
          shape={amountShape}
        />
        <DecimalTextField
          label="Importe máximo"
          value={filter.max}
          onChange={(max) => setFilter({ ...filter, max, page: 1 })}
          shape={amountShape}
        />
      </div>
      <Notice>{list.error}</Notice>
      {!list.data && !list.error && <p className="muted">Consultando gastos de caja…</p>}
      {list.data && (
        <>
          <div className="table-container cash-table">
            <table className="inventory-table">
              <caption className="sr-only">Gastos de caja candidatos</caption>
              <thead>
                <tr>
                  <th scope="col">Gasto</th>
                  <th scope="col" className="inventory-number">
                    Importe
                  </th>
                  <th scope="col" className="inventory-number">
                    Vinculado
                  </th>
                  <th scope="col" className="inventory-number">
                    Disponible
                  </th>
                  <th scope="col">Estado</th>
                  <th scope="col">
                    <span className="sr-only">Acciones</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {list.data.items.map((expense) => (
                  <tr key={expense.id}>
                    <td className="inventory-cell-name">
                      <strong>{expenseTitle(expense)}</strong>
                      <span className="muted text-xs inventory-cell-detail">
                        {expenseDetail(expense, timezone)}
                      </span>
                    </td>
                    <td className="inventory-number">
                      {formatMoney(expense.amount, expense.currency)}
                    </td>
                    <td className="inventory-number">
                      {formatMoney(expense.linked_amount, expense.currency)}
                      {expense.linked_to_receipt && !isZeroText(expense.linked_to_receipt) && (
                        <span className="muted text-xs inventory-cell-detail">
                          Con esta recepción: {formatMoney(expense.linked_to_receipt)}
                        </span>
                      )}
                    </td>
                    <td className="inventory-number">
                      {formatMoney(expense.available_amount, expense.currency)}
                    </td>
                    <td>
                      {expense.linkable ? (
                        <span className="tag tag-good">Se puede vincular</span>
                      ) : (
                        <span className="tag tag-warning">
                          {expense.unavailable_label ?? expense.status_label}
                        </span>
                      )}
                      {expense.status !== 'recorded' && (
                        <span className="muted text-xs inventory-cell-detail">
                          {expense.status_label}
                        </span>
                      )}
                    </td>
                    <td>
                      <button
                        type="button"
                        className="secondary-button"
                        disabled={!expense.linkable || busy}
                        aria-label={`Elegir el gasto ${expenseTitle(expense)} de ${formatMoney(expense.amount, expense.currency)}`}
                        onClick={() => setSelected(expense)}
                      >
                        Elegir
                      </button>
                    </td>
                  </tr>
                ))}
                {list.data.items.length === 0 && (
                  <tr>
                    <td colSpan={6} className="muted">
                      No hay gastos de caja que puedas consultar con estos filtros. Sin caja
                      asignada a esta terminal ni permiso para otras cajas, la lista queda vacía.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <Pagination
            page={list.data.page}
            pageSize={list.data.page_size}
            total={list.data.total}
            singular="gasto"
            plural="gastos"
            onPage={(page) => setFilter({ ...filter, page })}
          />
        </>
      )}
    </section>
  );
}

function linkExpenseText(link: CashLink, timezone: string) {
  if (!link.expense_visible || !link.expense) {
    return {
      title: 'Gasto de caja',
      detail: 'Tus permisos de caja no permiten consultar los datos de este gasto.',
    };
  }
  return {
    title: `${expenseTitle(link.expense)} · ${formatMoney(link.expense.amount, link.expense.currency)}`,
    detail: expenseDetail(link.expense, timezone),
  };
}

function CashLinksTable({
  receipt,
  timezone,
  editable,
  busy,
  onUnlink,
}: {
  receipt: Receipt;
  timezone: string;
  editable: boolean;
  busy: boolean;
  onUnlink: (link: CashLink) => void;
}) {
  if (receipt.cash_links.length === 0) {
    return <p className="muted text-sm">La recepción no está vinculada con ningún gasto.</p>;
  }
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <caption className="sr-only">Vínculos con gastos de caja</caption>
        <thead>
          <tr>
            <th scope="col">Gasto</th>
            <th scope="col" className="inventory-number">
              Vinculado
            </th>
            <th scope="col">Estado</th>
            <th scope="col">Vinculó</th>
            <th scope="col">
              <span className="sr-only">Acciones</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {receipt.cash_links.map((link) => {
            const text = linkExpenseText(link, timezone);
            return (
              <tr key={link.id}>
                <td className="inventory-cell-name">
                  <strong>{text.title}</strong>
                  <span className="muted text-xs inventory-cell-detail">{text.detail}</span>
                  {link.notes && (
                    <span className="muted text-xs inventory-cell-detail">Nota: {link.notes}</span>
                  )}
                </td>
                <td className="inventory-number">{formatMoney(link.amount)}</td>
                <td>
                  <span className={cashLinkTone(link.status)}>{link.status_label}</span>
                  {link.unlinked_at && (
                    <span className="muted text-xs inventory-cell-detail">
                      {serverDateTime(link.unlinked_at, timezone)}
                      {link.unlinked_by_name ? ` · ${link.unlinked_by_name}` : ''}
                      {link.unlink_reason ? ` · ${link.unlink_reason}` : ''}
                    </span>
                  )}
                </td>
                <td>
                  {link.linked_by_name}
                  <span className="muted text-xs inventory-cell-detail">
                    {serverDateTime(link.linked_at, timezone)}
                  </span>
                </td>
                <td>
                  {editable && link.active && (
                    <button
                      type="button"
                      className="danger-button"
                      disabled={busy}
                      aria-label={`Desvincular el gasto ${text.title}`}
                      onClick={() => onUnlink(link)}
                    >
                      Desvincular
                    </button>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

// Vínculo opcional de la recepción con gastos de caja ya registrados (§20): una compra no genera
// un gasto ni un gasto una compra. Vincular y desvincular exigen inventory.purchases.link_cash y
// cash.movements.read; Go aplica además las reglas de caja (otra caja, turnos cerrados), valida
// moneda, estado e importes y nunca modifica el turno. Un gasto corregido o regularizado después
// del vínculo se señala en el estado del vínculo hasta desvincularlo.
export function CashLinksSection({
  receipt,
  can,
  execute,
  busy,
  timezone,
}: {
  receipt: Receipt;
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
}) {
  const [searching, setSearching] = useState(false);
  const [unlinking, setUnlinking] = useState<CashLink | null>(null);
  const editable = canLinkCash(can);
  const activeLinks = receipt.cash_links.filter((link) => link.active).length;

  async function unlink(link: CashLink, reason: string) {
    try {
      const saved = await unlinkCashExpense(execute, link.id, { reason });
      notifySuccess(`Gasto desvinculado de la ${receiptFolio(saved.folio).toLowerCase()}.`, {
        description: `Por vincular: ${formatMoney(saved.linkable_amount)}.`,
      });
      setUnlinking(null);
      return true;
    } catch (problem) {
      notifyError(problem, 'No se pudo desvincular el gasto.');
      return false;
    }
  }

  const missing = !editable
    ? [
        can('inventory.purchases.link_cash')
          ? ''
          : purchasingPermissionName('inventory.purchases.link_cash'),
        can('cash.movements.read') ? '' : purchasingPermissionName('cash.movements.read'),
      ].filter(Boolean)
    : [];

  return (
    <section className="inventory-subsection form-stack" aria-label="Vínculo con gastos de caja">
      <div className="section-heading">
        <div>
          <h3>Vínculo con gastos de caja</h3>
          <p className="muted text-xs">
            Opcional: relaciona la recepción con el gasto con que se pagó. No crea ni modifica
            gastos ni turnos, y una recepción no implica un pago.
          </p>
        </div>
        {editable && !searching && (
          <button
            type="button"
            className="secondary-button"
            disabled={busy}
            onClick={() => {
              setUnlinking(null);
              setSearching(true);
            }}
          >
            <IconLabel icon="expense">Vincular con un gasto</IconLabel>
          </button>
        )}
      </div>
      <dl className="inventory-facts">
        <div>
          <dt>Total de la recepción</dt>
          <dd>{formatMoney(receipt.total)}</dd>
        </div>
        <div>
          <dt>Vinculado</dt>
          <dd>
            {formatMoney(receipt.linked_amount)} ·{' '}
            {countLabel(activeLinks, 'vínculo vigente', 'vínculos vigentes')}
          </dd>
        </div>
        <div>
          <dt>Por vincular</dt>
          <dd>
            <strong>{formatMoney(receipt.linkable_amount)}</strong>
          </dd>
        </div>
      </dl>
      {receipt.cash_link_attention && (
        <div className="inventory-negative-notice" role="status">
          <p>
            Un gasto vinculado se corrigió o regularizó en caja después del vínculo. El vínculo
            sigue contando hasta desvincularlo: revísalo y desvincúlalo si ya no corresponde.
          </p>
        </div>
      )}
      {missing.length > 0 && (
        <p className="muted text-xs">
          Vincular o desvincular requiere {missing.length === 1 ? 'el permiso' : 'los permisos'}{' '}
          {missing.map((name) => `«${name}»`).join(' y ')}.
        </p>
      )}
      <CashLinksTable
        receipt={receipt}
        timezone={timezone}
        editable={editable}
        busy={busy}
        onUnlink={(link) => {
          setSearching(false);
          setUnlinking(link);
        }}
      />
      {unlinking && (
        <ReasonAction
          key={unlinking.id}
          title="Desvincular el gasto"
          description={`Se registra la desvinculación con su motivo; el vínculo se conserva en el historial y el gasto y su turno no cambian. Vinculado: ${formatMoney(unlinking.amount)}.`}
          reasonLabel="Motivo de la desvinculación"
          actionLabel="Desvincular"
          confirmTitle="Desvincular gasto de caja"
          confirmMessage={(reason) =>
            `Se desvinculará ${formatMoney(unlinking.amount)} de la ${receiptFolio(receipt.folio).toLowerCase()} con el motivo «${reason}».`
          }
          busy={busy}
          onConfirm={(reason) => unlink(unlinking, reason)}
          onCancel={() => setUnlinking(null)}
        />
      )}
      {searching && (
        <CashExpenseSearch
          receipt={receipt}
          execute={execute}
          busy={busy}
          timezone={timezone}
          onClose={() => setSearching(false)}
        />
      )}
    </section>
  );
}
