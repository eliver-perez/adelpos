import { useEffect, useId, useState } from 'react';
import type { FormEvent } from 'react';
import { OptionalDateField } from '../catalog/DateField';
import { Field } from '../Forms';
import { SegmentedControl } from '../SegmentedControl';
import { ToggleButton } from '../ToggleButton';
import type { FilterOption, PeriodKind, ResolvedPeriod } from '../../services/reports';
import {
  immediatePeriods,
  isoWeekRange,
  monthNames,
  periodComplete,
  prefillPeriod,
  rangeText,
  shiftIsoWeek,
  shortPeriodLabels,
} from './period';
import type { PeriodSelection } from './period';
import { Select } from '../Select';

// Filtro de periodo reutilizable de Reportes y del resumen ejecutivo (§7). Los periodos rápidos
// (hoy, ayer, semana, mes y año actuales) se aplican con un toque; los que necesitan un dato (día,
// semana del año, mes, año o rango) se aplican al completarlo con «Aplicar periodo» o con Intro, de
// modo que nunca se consulta por cada tecla. El periodo que vale es el que resuelve Go: su etiqueta
// se muestra como «Periodo aplicado».

const knownPeriods: PeriodKind[] = [
  'today',
  'yesterday',
  'current_week',
  'current_month',
  'current_year',
  'day',
  'iso_week',
  'month',
  'year',
  'range',
];

// «Semana 53 de 2026: del 28 de diciembre de 2026 al 3 de enero de 2027, de lunes a domingo».
function weekDescription(year: string, week: string, range: { first: string; last: string }) {
  const days = rangeText(range.first, range.last);
  return `Semana ${Number(week)} de ${year}: d${days.slice(1)}, de lunes a domingo (ISO 8601).`;
}

function digitsOnly(text: string, length: number) {
  if (!/^\d*$/.test(text)) {
    return null;
  }
  return text.slice(0, length);
}

export function PeriodFilter({
  periods,
  value,
  onChange,
  resolved,
  comparison = null,
  allowCompare,
  dayStart,
  disabled = false,
}: {
  periods: FilterOption[];
  value: PeriodSelection;
  onChange: (next: PeriodSelection) => void;
  // Último periodo que resolvió Go para la consulta mostrada (null mientras no hay resultado).
  resolved: ResolvedPeriod | null;
  comparison?: ResolvedPeriod | null;
  allowCompare: boolean;
  dayStart?: string;
  disabled?: boolean;
}) {
  const [draft, setDraft] = useState(value);
  const weekFieldID = useId();

  // Un periodo aplicado desde fuera (otro reporte, una tarjeta del resumen) sustituye la edición.
  useEffect(() => {
    setDraft(value);
  }, [value]);

  const available = knownPeriods
    .map((kind) => {
      const option = periods.find((entry) => entry.value === kind);
      if (!option) {
        return null;
      }
      return { value: kind, label: shortPeriodLabels[kind] ?? option.label };
    })
    .filter((entry): entry is { value: PeriodKind; label: string } => entry !== null);
  const quick = available.filter((entry) => immediatePeriods.includes(entry.value));
  const specific = available.filter((entry) => !immediatePeriods.includes(entry.value));
  const needsFields = !immediatePeriods.includes(draft.period);
  const complete = periodComplete(draft);
  const changed = JSON.stringify(draft) !== JSON.stringify(value);

  function choose(kind: PeriodKind) {
    if (immediatePeriods.includes(kind)) {
      const next = { ...value, period: kind };
      setDraft(next);
      onChange(next);
      return;
    }
    const next = prefillPeriod(draft, kind, resolved);
    setDraft(next);
    if (periodComplete(next)) {
      onChange(next);
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    if (complete) {
      onChange(draft);
    }
  }

  function moveWeek(offset: number) {
    const week = shiftIsoWeek(draft.isoYear, draft.isoWeek, offset);
    if (!week) {
      return;
    }
    const next = { ...draft, isoYear: String(week.year), isoWeek: String(week.week) };
    setDraft(next);
    onChange(next);
  }

  const weekRange = isoWeekRange(draft.isoYear, draft.isoWeek);

  return (
    <form className="period-filter" aria-label="Periodo" onSubmit={submit}>
      <div className="period-presets">
        {quick.length > 0 && (
          <SegmentedControl
            label="Periodos rápidos"
            value={draft.period}
            options={quick}
            disabled={disabled}
            onChange={choose}
          />
        )}
        {specific.length > 0 && (
          <SegmentedControl
            label="Elegir periodo"
            value={draft.period}
            options={specific}
            disabled={disabled}
            onChange={choose}
          />
        )}
      </div>
      {needsFields && (
        <div className="period-fields">
          {draft.period === 'day' && (
            <OptionalDateField
              label="Día"
              value={draft.date}
              onChange={(date) => setDraft({ ...draft, date })}
            />
          )}
          {draft.period === 'iso_week' && (
            <>
              <Field label="Año (ISO)">
                <input
                  type="text"
                  inputMode="numeric"
                  autoComplete="off"
                  maxLength={4}
                  value={draft.isoYear}
                  onChange={(event) => {
                    const year = digitsOnly(event.target.value, 4);
                    if (year !== null) {
                      setDraft({ ...draft, isoYear: year });
                    }
                  }}
                />
              </Field>
              {/* La etiqueta se asocia solo al número de semana: los botones de semana anterior y
                  siguiente quedan fuera de ella con su propio nombre. */}
              <div className="field">
                <label htmlFor={weekFieldID}>Semana</label>
                <div className="inline-field">
                  <button
                    type="button"
                    className="secondary-button"
                    aria-label="Semana anterior"
                    disabled={disabled || !weekRange}
                    onClick={() => moveWeek(-1)}
                  >
                    ‹
                  </button>
                  <input
                    id={weekFieldID}
                    type="text"
                    inputMode="numeric"
                    autoComplete="off"
                    maxLength={2}
                    value={draft.isoWeek}
                    onChange={(event) => {
                      const week = digitsOnly(event.target.value, 2);
                      if (week !== null) {
                        setDraft({ ...draft, isoWeek: week });
                      }
                    }}
                  />
                  <button
                    type="button"
                    className="secondary-button"
                    aria-label="Semana siguiente"
                    disabled={disabled || !weekRange}
                    onClick={() => moveWeek(1)}
                  >
                    ›
                  </button>
                </div>
              </div>
            </>
          )}
          {draft.period === 'month' && (
            <>
              <Field label="Mes">
                <Select
                  value={draft.month}
                  placeholder="Elige un mes"
                  options={monthNames.map((name, index) => ({
                    value: String(index + 1),
                    label: name.charAt(0).toUpperCase() + name.slice(1),
                  }))}
                  onChange={(month) => setDraft({ ...draft, month })}
                />
              </Field>
              <Field label="Año">
                <input
                  type="text"
                  inputMode="numeric"
                  autoComplete="off"
                  maxLength={4}
                  value={draft.year}
                  onChange={(event) => {
                    const year = digitsOnly(event.target.value, 4);
                    if (year !== null) {
                      setDraft({ ...draft, year });
                    }
                  }}
                />
              </Field>
            </>
          )}
          {draft.period === 'year' && (
            <Field label="Año">
              <input
                type="text"
                inputMode="numeric"
                autoComplete="off"
                maxLength={4}
                value={draft.year}
                onChange={(event) => {
                  const year = digitsOnly(event.target.value, 4);
                  if (year !== null) {
                    setDraft({ ...draft, year });
                  }
                }}
              />
            </Field>
          )}
          {draft.period === 'range' && (
            <>
              <OptionalDateField
                label="Desde (incluido)"
                value={draft.from}
                onChange={(from) => setDraft({ ...draft, from })}
              />
              <OptionalDateField
                label="Hasta (incluido)"
                value={draft.until}
                onChange={(until) => setDraft({ ...draft, until })}
              />
            </>
          )}
          <button
            type="submit"
            className="primary-button period-apply"
            disabled={disabled || !complete || !changed}
          >
            Aplicar periodo
          </button>
        </div>
      )}
      {draft.period === 'iso_week' && (
        <p className="period-week-range text-sm" aria-live="polite">
          {weekRange
            ? weekDescription(draft.isoYear, draft.isoWeek, weekRange)
            : 'Indica un año y una semana que existan (de 1 a 52 o 53).'}
        </p>
      )}
      <div className="period-summary">
        {allowCompare && (
          <ToggleButton
            label="Comparar con el periodo anterior"
            pressed={value.compare}
            disabled={disabled}
            onChange={(compare) => {
              const next = { ...value, compare };
              setDraft({ ...draft, compare });
              onChange(next);
            }}
          />
        )}
        <dl className="period-resolved">
          <div>
            <dt>Periodo aplicado</dt>
            <dd>
              {resolved ? resolved.label : 'Se mostrará al consultar.'}
              {resolved?.in_progress && <span className="tag tag-info">En curso</span>}
            </dd>
          </div>
          {allowCompare && value.compare && comparison && (
            <div>
              <dt>Comparado con</dt>
              <dd>{comparison.label}</dd>
            </div>
          )}
          {dayStart && (
            <div>
              <dt>Inicio del día</dt>
              <dd>{dayStart}</dd>
            </div>
          )}
        </dl>
      </div>
    </form>
  );
}
