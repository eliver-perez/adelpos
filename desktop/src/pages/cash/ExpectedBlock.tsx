import { cashDifferenceLabels } from '../../services/cash';
import type { CashDifferences, CashExpectation } from '../../services/cash';
import { formatDollars, formatMoney } from '../../services/payments';

// Efectivo esperado y diferencias por moneda. Este bloque solo se renderiza con cifras que Go
// entregó (cash.expected.read y fuera del arqueo): la interfaz nunca las calcula ni las deduce, y
// quien lo usa decide no montarlo durante el arqueo de cierre.

export function ExpectedFigures({
  expected,
  title = 'Efectivo esperado en caja',
}: {
  expected: CashExpectation;
  title?: string;
}) {
  return (
    <div className="shift-summary cash-expected" aria-label={title}>
      <div>
        {title} · pesos
        <strong>{formatMoney(expected.mxn)}</strong>
      </div>
      <div>
        {title} · dólares
        <strong>{formatDollars(expected.usd)}</strong>
      </div>
      {expected.usd_rate && (
        <div>
          Dólares en pesos (a {formatMoney(expected.usd_rate)})
          <strong>{formatMoney(expected.usd_equivalent_mxn)}</strong>
        </div>
      )}
      {expected.total_equivalent_mxn && (
        <div>
          Total equivalente en pesos
          <strong>{formatMoney(expected.total_equivalent_mxn)}</strong>
        </div>
      )}
    </div>
  );
}

export function DifferenceFigures({ differences }: { differences: CashDifferences }) {
  return (
    <div className="shift-summary cash-differences" aria-label="Diferencias del arqueo">
      <div className={`cash-result-${differences.result_mxn}`}>
        Diferencia en pesos · {cashDifferenceLabels[differences.result_mxn]}
        <strong>{formatMoney(differences.mxn)}</strong>
      </div>
      <div className={`cash-result-${differences.result_usd}`}>
        Diferencia en dólares · {cashDifferenceLabels[differences.result_usd]}
        <strong>{formatDollars(differences.usd)}</strong>
      </div>
      {differences.equivalent_mxn && (
        <div>
          Equivalente total en pesos
          <strong>{formatMoney(differences.equivalent_mxn)}</strong>
        </div>
      )}
    </div>
  );
}
