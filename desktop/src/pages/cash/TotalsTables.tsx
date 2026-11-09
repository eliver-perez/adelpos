import { currencyLabels } from '../../services/cash';
import type { PhysicalCash, ShiftTotals } from '../../services/cash';
import { formatDollars, formatMoney } from '../../services/money';
import { countLabel } from '../../services/plural';
import { currencyAmount } from './DenominationCount';

// Totales informativos de un turno, tal como los calculó Go para un corte o para el resumen en
// vivo. No incluyen el efectivo esperado: ese bloque solo se muestra cuando Go lo entrega.

function CountFigures({ totals }: { totals: ShiftTotals }) {
  return (
    <div className="shift-summary" aria-label="Totales del turno">
      <div>
        Ventas cobradas
        <strong>{formatMoney(totals.sales)}</strong>
      </div>
      <div>
        Propinas
        <strong>{formatMoney(totals.tips)}</strong>
      </div>
      <div>
        Devoluciones
        <strong>{formatMoney(totals.refunded)}</strong>
      </div>
      <div>
        Descuentos · {totals.discount_count}
        <strong>{formatMoney(totals.discounts)}</strong>
      </div>
      <div>
        Cortesías · {totals.courtesy_count}
        <strong>{formatMoney(totals.courtesies)}</strong>
      </div>
      <div>
        Registros
        <strong>
          {countLabel(totals.payments, 'cobro', 'cobros')} ·{' '}
          {countLabel(totals.refunds, 'devolución', 'devoluciones')} ·{' '}
          {countLabel(totals.cash_operations, 'operación de efectivo', 'operaciones de efectivo')}
        </strong>
      </div>
      {totals.regularizations > 0 && (
        <div>
          Regularizaciones de turnos cerrados
          <strong>{totals.regularizations}</strong>
        </div>
      )}
    </div>
  );
}

function MethodsTable({ totals }: { totals: ShiftTotals }) {
  return (
    <div className="table-container cash-table">
      <table>
        <caption className="cash-table-caption">Ventas por método de pago</caption>
        <thead>
          <tr>
            <th scope="col">Método</th>
            <th scope="col">Ventas</th>
            <th scope="col">Propinas</th>
            <th scope="col">Devoluciones</th>
            <th scope="col">Neto</th>
          </tr>
        </thead>
        <tbody>
          {totals.methods.map((method) => (
            <tr key={`${method.method_name}-${method.is_cash}`}>
              <td>
                <strong>{method.method_name}</strong>
                {method.is_cash && <span className="muted"> · efectivo</span>}
              </td>
              <td>{formatMoney(method.sales)}</td>
              <td>{formatMoney(method.tips)}</td>
              <td>{formatMoney(method.refunds)}</td>
              <td>{formatMoney(method.net)}</td>
            </tr>
          ))}
          {totals.methods.length === 0 && (
            <tr>
              <td colSpan={5} className="muted">
                Sin cobros registrados.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

function CashOperationsTable({ totals }: { totals: ShiftTotals }) {
  return (
    <div className="table-container cash-table">
      <table>
        <caption className="cash-table-caption">Ingresos, retiros y gastos por moneda</caption>
        <thead>
          <tr>
            <th scope="col">Moneda</th>
            <th scope="col">Ingresos</th>
            <th scope="col">Retiros</th>
            <th scope="col">Gastos</th>
            <th scope="col">Correcciones que entran</th>
            <th scope="col">Correcciones que salen</th>
          </tr>
        </thead>
        <tbody>
          {totals.cash.map((item) => (
            <tr key={item.currency}>
              <td>
                <strong>{currencyLabels[item.currency]}</strong>
              </td>
              <td>{currencyAmount(item.currency, item.income)}</td>
              <td>{currencyAmount(item.currency, item.withdrawals)}</td>
              <td>{currencyAmount(item.currency, item.expenses)}</td>
              <td>{currencyAmount(item.currency, item.corrections_in)}</td>
              <td>{currencyAmount(item.currency, item.corrections_out)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function TotalsTables({ totals }: { totals: ShiftTotals }) {
  return (
    <div className="form-stack">
      <CountFigures totals={totals} />
      <MethodsTable totals={totals} />
      <CashOperationsTable totals={totals} />
    </div>
  );
}

// Efectivo recibido y entregado en cobros y devoluciones, por moneda (solo si Go lo entrega).
export function PhysicalFigures({ physical }: { physical: PhysicalCash }) {
  return (
    <div className="shift-summary" aria-label="Efectivo recibido y entregado">
      <div>
        Pesos recibidos / entregados
        <strong>
          {formatMoney(physical.mxn_in)} / {formatMoney(physical.mxn_out)}
        </strong>
      </div>
      <div>
        Dólares recibidos / entregados
        <strong>
          {formatDollars(physical.usd_in)} / {formatDollars(physical.usd_out)}
        </strong>
      </div>
    </div>
  );
}
