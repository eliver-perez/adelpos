import { useEffect, useState } from 'react';
import type { FormEvent } from 'react';
import { Field } from '../../components/Forms';
import { errorMessage } from '../../services/notify';
import { fetchFilterOptions } from '../../services/reports';
import type { FilterOption, ReportFilter } from '../../services/reports';

// Filtros adicionales de un reporte (§7): solo los que el reporte declara en el catálogo de Go
// (caja, turno, usuario, área, mesa, producto, categoría, método de pago, motivo, modalidad…). Las
// opciones de los filtros por identificador salen de reports/filters/{fuente} y se consultan una
// vez por fuente; elegir una opción aplica el filtro (una consulta por elección, nunca por tecla).

export type OptionLoader = (source: string) => Promise<FilterOption[]>;

// Fuentes con búsqueda en Go (hasta 100 coincidencias): productos e insumos.
const searchableSources = ['products', 'inventory_items'];

// Clave con la que la pantalla conserva el valor de un filtro al cambiar de reporte: el parámetro
// y su fuente, para que un mismo nombre de parámetro con otra fuente (por ejemplo «category_id» de
// categorías del catálogo y de categorías de insumos) nunca reciba un identificador ajeno.
export function filterStateKey(filter: ReportFilter) {
  return `${filter.key}:${filter.source ?? ''}`;
}

const allLabel = 'Sin filtro';

function SourceSelect({
  filter,
  value,
  onChange,
  loadOptions,
  disabled,
}: {
  filter: ReportFilter;
  value: string;
  onChange: (value: string) => void;
  loadOptions: OptionLoader;
  disabled: boolean;
}) {
  const [options, setOptions] = useState<FilterOption[] | null>(null);
  const [error, setError] = useState('');
  const source = filter.source ?? '';
  useEffect(() => {
    let current = true;
    loadOptions(source)
      .then((loaded) => {
        if (current) {
          setOptions(loaded);
        }
      })
      .catch((failure: unknown) => {
        if (current) {
          setError(errorMessage(failure, 'No se pudieron cargar las opciones.'));
        }
      });
    return () => {
      current = false;
    };
  }, [loadOptions, source]);
  return (
    <Field label={filter.label} hint={error || undefined}>
      <select
        value={value}
        disabled={disabled || options === null}
        onChange={(event) => onChange(event.target.value)}
      >
        <option value="">{options === null && !error ? 'Cargando opciones…' : allLabel}</option>
        {(options ?? []).map((option) => (
          <option key={option.value} value={option.value}>
            {option.detail ? `${option.label} · ${option.detail}` : option.label}
          </option>
        ))}
      </select>
    </Field>
  );
}

// Producto o insumo: Go devuelve hasta 100 coincidencias; la búsqueda se consulta al pulsar
// «Buscar».
function SearchableFilter({
  filter,
  value,
  onChange,
  disabled,
}: {
  filter: ReportFilter;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  const [search, setSearch] = useState('');
  const [options, setOptions] = useState<FilterOption[] | null>(null);
  const [error, setError] = useState('');
  const [selected, setSelected] = useState<FilterOption | null>(null);

  async function load(text: string) {
    setError('');
    try {
      const response = await fetchFilterOptions(filter.source ?? '', text);
      setOptions(response.options);
    } catch (failure) {
      setError(errorMessage(failure, 'No se pudo hacer la búsqueda.'));
    }
  }

  // Solo al montar: las búsquedas siguientes las pide la persona con «Buscar».
  useEffect(() => {
    void load('');
  }, []);

  function submit(event: FormEvent) {
    event.preventDefault();
    void load(search.trim());
  }

  const list = [...(options ?? [])];
  if (selected && value === selected.value && !list.some((entry) => entry.value === value)) {
    list.unshift(selected);
  }
  return (
    <div className="report-search-filter">
      <form
        className="inline-field"
        role="search"
        aria-label={`Buscar ${filter.label.toLowerCase()}`}
        onSubmit={submit}
      >
        <Field label={`Buscar ${filter.label.toLowerCase()}`}>
          <input
            type="search"
            autoComplete="off"
            maxLength={80}
            value={search}
            disabled={disabled}
            onChange={(event) => setSearch(event.target.value)}
          />
        </Field>
        <button type="submit" className="secondary-button" disabled={disabled}>
          Buscar
        </button>
      </form>
      <Field label={filter.label} hint={error || undefined}>
        <select
          value={value}
          disabled={disabled || options === null}
          onChange={(event) => {
            const option = list.find((entry) => entry.value === event.target.value) ?? null;
            setSelected(option);
            onChange(event.target.value);
          }}
        >
          <option value="">{allLabel}</option>
          {list.map((option) => (
            <option key={option.value} value={option.value}>
              {option.detail ? `${option.label} · ${option.detail}` : option.label}
            </option>
          ))}
        </select>
      </Field>
    </div>
  );
}

// Texto libre (hasta 80 caracteres): se aplica con «Aplicar» o con Intro.
function TextFilter({
  filter,
  value,
  onChange,
  disabled,
}: {
  filter: ReportFilter;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  const [text, setText] = useState(value);
  useEffect(() => setText(value), [value]);
  return (
    <form
      className="inline-field report-text-filter"
      onSubmit={(event) => {
        event.preventDefault();
        onChange(text.trim());
      }}
    >
      <Field label={filter.label}>
        <input
          type="text"
          autoComplete="off"
          maxLength={80}
          value={text}
          disabled={disabled}
          onChange={(event) => setText(event.target.value)}
        />
      </Field>
      <button
        type="submit"
        className="secondary-button"
        disabled={disabled || text.trim() === value}
      >
        Aplicar
      </button>
    </form>
  );
}

export function ReportFilterFields({
  filters,
  values,
  onChange,
  loadOptions,
  disabled = false,
}: {
  filters: ReportFilter[];
  values: Record<string, string>;
  onChange: (key: string, value: string) => void;
  loadOptions: OptionLoader;
  disabled?: boolean;
}) {
  if (filters.length === 0) {
    return null;
  }
  return (
    <div className="report-filter-fields" role="group" aria-label="Filtros adicionales">
      {filters.map((filter) => {
        const stateKey = filterStateKey(filter);
        const value = values[stateKey] ?? '';
        const change = (next: string) => onChange(stateKey, next);
        if (filter.type === 'choice') {
          return (
            <Field key={filter.key} label={filter.label}>
              <select
                value={value === '' ? (filter.default ?? '') : value}
                disabled={disabled}
                onChange={(event) => change(event.target.value)}
              >
                {!filter.default && <option value="">{allLabel}</option>}
                {(filter.options ?? []).map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </Field>
          );
        }
        if (filter.source && searchableSources.includes(filter.source)) {
          return (
            <SearchableFilter
              key={filter.key}
              filter={filter}
              value={value}
              onChange={change}
              disabled={disabled}
            />
          );
        }
        if (filter.source) {
          return (
            <SourceSelect
              key={filter.key}
              filter={filter}
              value={value}
              onChange={change}
              loadOptions={loadOptions}
              disabled={disabled}
            />
          );
        }
        return (
          <TextFilter
            key={filter.key}
            filter={filter}
            value={value}
            onChange={change}
            disabled={disabled}
          />
        );
      })}
    </div>
  );
}
