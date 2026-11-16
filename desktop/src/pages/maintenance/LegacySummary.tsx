import { Accordion } from '../../components/Accordion';
import { Icon } from '../../components/Icon';
import { formatMoney } from '../../services/money';
import {
  checkGroupLabels,
  checkValue,
  countLabels,
  failingChecks,
  formatCount,
  legacyDateTime,
  summarySentence,
} from '../../services/legacyMigration';
import type {
  LegacyBatch,
  LegacyCheck,
  LegacyPreview,
  LegacySummary,
  LegacyValidation,
} from '../../services/legacyMigration';

// Piezas del asistente de migración (etapa 9H, fase D): la barra de avance, el resumen de un paquete (el
// resumen previo del análisis y el del informe final) y la validación de §4.4. Las diferencias de dinero
// nunca se ocultan: van primero, en rojo y con el importe.

export function ProgressBar({ label, percent }: { label: string; percent: number }) {
  const value = Math.max(0, Math.min(100, Math.round(percent)));
  return (
    <div className="legacy-progress-row">
      <div
        className="legacy-progress"
        role="progressbar"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={value}
      >
        <span style={{ width: `${value}%` }} />
      </div>
      <span className="legacy-progress-value">{value} %</span>
    </div>
  );
}

export function BatchFacts({ batch }: { batch: LegacyBatch }) {
  return (
    <dl className="legacy-facts">
      <div>
        <dt>Respaldo del sistema anterior</dt>
        <dd>{legacyDateTime(batch.backup_taken_at)}</dd>
      </div>
      <div>
        <dt>Importado</dt>
        <dd>
          {legacyDateTime(batch.completed_at ?? batch.imported_at)} por {batch.imported_by}
        </dd>
      </div>
      <div>
        <dt>Ventas importadas</dt>
        <dd>
          {formatCount(batch.sales)}
          {batch.sales_first_at && batch.sales_last_at
            ? ` (del ${legacyDateTime(batch.sales_first_at)} al ${legacyDateTime(batch.sales_last_at)})`
            : ''}
        </dd>
      </div>
      {batch.has_differences && (
        <div>
          <dt>Validación</dt>
          <dd>
            <span className="tag tag-danger">Con diferencias</span>
          </dd>
        </div>
      )}
    </dl>
  );
}

function YearTotals({ summary }: { summary: LegacySummary }) {
  const years = summary.totals.sales_by_year ?? [];
  if (years.length === 0) {
    return null;
  }
  return (
    <div className="legacy-block">
      <h4>Total vendido por año</h4>
      <ul className="legacy-year-list">
        {years.map((entry) => (
          <li key={entry.year}>
            <span>{entry.year}</span>
            <strong>{formatMoney(entry.amount)}</strong>
          </li>
        ))}
      </ul>
    </div>
  );
}

function Incoherences({ summary }: { summary: LegacySummary }) {
  if (summary.flags.length === 0) {
    return <p className="muted text-sm">El paquete no tiene datos incoherentes.</p>;
  }
  return (
    <div className="legacy-block">
      <h4>Datos incoherentes</h4>
      <p className="muted text-sm">
        Se importan tal como estaban, con una marca y una nota; nunca se corrigen solos. Los que
        tienen folio aparecen en el informe en PDF.
      </p>
      <ul className="legacy-flag-list">
        {summary.flags.map((flag) => (
          <li key={`${flag.entity}:${flag.code}`}>
            <Icon name="alert" size={16} />
            <span>{flag.label}</span>
            <strong>{formatCount(flag.count)}</strong>
          </li>
        ))}
      </ul>
    </div>
  );
}

// PackageSummary es el resumen de un paquete: conteos, periodo, tasas, totales, avisos y datos incoherentes.
export function PackageSummary({ summary }: { summary: LegacySummary }) {
  const rates = Object.entries(summary.tax_rates).sort(
    ([left], [right]) => Number(left) - Number(right),
  );
  return (
    <div className="legacy-summary">
      <p className="legacy-sentence">{summarySentence(summary)}</p>
      <ul className="legacy-counts" aria-label="Conteos del paquete">
        {countLabels
          .filter((entry) => summary.counts[entry.key] !== undefined)
          .map((entry) => {
            const count = summary.counts[entry.key] ?? 0;
            return (
              <li key={entry.key}>
                <strong>{formatCount(count)}</strong>
                <span>{count === 1 ? entry.singular : entry.plural}</span>
              </li>
            );
          })}
      </ul>
      <div className="legacy-columns">
        <YearTotals summary={summary} />
        <div className="legacy-block">
          <h4>Otros totales del sistema anterior</h4>
          <ul className="legacy-year-list">
            <li>
              <span>Retiros de efectivo</span>
              <strong>{formatMoney(summary.totals.withdrawals)}</strong>
            </li>
            <li>
              <span>Salidas de efectivo</span>
              <strong>{formatMoney(summary.totals.expenses)}</strong>
            </li>
            <li>
              <span>Ventas canceladas (como las contaba el sistema anterior)</span>
              <strong>{formatMoney(summary.totals.cancelled_legacy_style)}</strong>
            </li>
          </ul>
          {rates.length > 0 && (
            <p className="muted text-sm">
              IVA de los productos:{' '}
              {rates
                .map(
                  ([rate, count]) =>
                    `${rate} % en ${formatCount(count)} producto${count === 1 ? '' : 's'}`,
                )
                .join(', ')}
              .
            </p>
          )}
        </div>
      </div>
      <Incoherences summary={summary} />
      {(summary.warnings.length > 0 || summary.notes.length > 0) && (
        <div className="legacy-block">
          <h4>Para tener en cuenta</h4>
          <ul className="legacy-notes">
            {summary.warnings.map((warning) => (
              <li key={warning}>{warning}</li>
            ))}
            {summary.notes.map((note) => (
              <li key={note.code}>{note.message}</li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

// PreviewStatus dice, antes de confirmar, si se puede importar y si se reemplaza un lote.
export function PreviewStatus({ preview }: { preview: LegacyPreview }) {
  const blockers = preview.blockers ?? [];
  if (!preview.clean) {
    return (
      <div className="legacy-alert legacy-alert-danger" role="alert">
        <strong>No se puede importar en esta instalación.</strong>
        <p>
          La importación solo se hace en una instalación sin operación propia ni catálogo, mesas o
          usuarios capturados en AD-EL POS (salvo lo que crea el asistente inicial). Empieza con una
          instalación nueva o limpia.
        </p>
        <ul>
          {blockers.map((blocker) => (
            <li key={blocker.code}>{blocker.message}</li>
          ))}
        </ul>
      </div>
    );
  }
  if (preview.will_replace && preview.current_batch) {
    return (
      <div className="legacy-alert legacy-alert-warning" role="status">
        <strong>Ya hay datos importados del sistema anterior.</strong>
        <p>
          Al importar este paquete se reemplaza completo lo importado antes (catálogo, mesas,
          usuarios e historial):
        </p>
        <BatchFacts batch={preview.current_batch} />
      </div>
    );
  }
  return null;
}

function CheckRow({ check }: { check: LegacyCheck }) {
  return (
    <tr className={check.ok ? '' : 'legacy-check-failed'}>
      <td>{check.label}</td>
      <td className="numeric">{checkValue(check, check.expected)}</td>
      <td className="numeric">{checkValue(check, check.actual)}</td>
      <td className="numeric">
        {check.ok ? (
          <span className="tag tag-good">Cuadra</span>
        ) : (
          <strong className="legacy-difference">{checkValue(check, check.difference)}</strong>
        )}
      </td>
    </tr>
  );
}

// ValidationReport es la validación de §4.4: lo que no cuadra va primero, con el importe de la diferencia.
export function ValidationReport({ validation }: { validation: LegacyValidation }) {
  const failing = failingChecks(validation);
  const groups = (Object.keys(checkGroupLabels) as LegacyCheck['group'][]).filter((group) =>
    validation.checks.some((check) => check.group === group),
  );
  return (
    <div className="legacy-validation">
      {failing.length === 0 ? (
        <div className="legacy-alert legacy-alert-good" role="status">
          <strong>Todo cuadra.</strong>
          <p>Los conteos y los totales importados coinciden exactamente con los del paquete.</p>
        </div>
      ) : (
        <div className="legacy-alert legacy-alert-danger" role="alert">
          <strong>
            {validation.has_money_differences
              ? 'Hay diferencias de dinero entre el paquete y lo importado.'
              : 'Hay conteos que no cuadran con el paquete.'}
          </strong>
          <p>
            Revísalas antes de usar el historial. La diferencia es lo importado menos lo esperado.
          </p>
          <ul className="legacy-failing">
            {failing.map((check) => (
              <li key={`${check.group}:${check.key}`}>
                <span>{check.label}</span>
                <span>
                  esperado {checkValue(check, check.expected)}, importado{' '}
                  {checkValue(check, check.actual)}
                </span>
                <strong className="legacy-difference">
                  diferencia {checkValue(check, check.difference)}
                </strong>
              </li>
            ))}
          </ul>
        </div>
      )}
      {(validation.notes ?? []).length > 0 && (
        <ul className="legacy-notes">
          {(validation.notes ?? []).map((note) => (
            <li key={note}>{note}</li>
          ))}
        </ul>
      )}
      {groups.map((group) => {
        const checks = validation.checks.filter((check) => check.group === group);
        const wrong = checks.filter((check) => !check.ok).length;
        // El detalle de cada grupo se pliega si todo cuadra; lo que no cuadra queda abierto y marcado.
        return (
          <Accordion
            key={group}
            className="legacy-check-group"
            title={checkGroupLabels[group]}
            summary={wrong === 0 ? 'Todo cuadra' : `${wrong} no cuadra${wrong === 1 ? '' : 'n'}`}
            hasError={wrong > 0}
          >
            <div className="table-container">
              <table className="legacy-checks">
                <thead>
                  <tr>
                    <th>Concepto</th>
                    <th className="numeric">En el paquete</th>
                    <th className="numeric">Importado</th>
                    <th className="numeric">Diferencia</th>
                  </tr>
                </thead>
                <tbody>
                  {checks.map((check) => (
                    <CheckRow key={`${check.group}:${check.key}`} check={check} />
                  ))}
                </tbody>
              </table>
            </div>
          </Accordion>
        );
      })}
    </div>
  );
}
