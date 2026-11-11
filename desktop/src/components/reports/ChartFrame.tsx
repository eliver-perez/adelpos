import { useId, useState } from 'react';
import type { ReactNode } from 'react';

// Marco común de los gráficos de Reportes: título, descripción, leyenda (con dos o más series),
// el gráfico y su tabla alternativa accesible («Ver como tabla»). Los colores de las series salen
// de los tokens --chart-* de styles.css (paleta validada en claro y oscuro); el texto siempre usa
// los tokens de texto.

// Tono de una serie: posición fija de la paleta categórica o el gris del periodo anterior.
export type SeriesTone = 1 | 2 | 3 | 4 | 5 | 'previous';

export interface LegendEntry {
  label: string;
  tone: SeriesTone;
  shape: 'line' | 'bar';
}

export interface ChartTable {
  columns: string[];
  // La primera celda de cada renglón es su encabezado.
  rows: string[][];
}

export function toneClass(tone: SeriesTone) {
  return tone === 'previous' ? 'chart-tone-previous' : `chart-tone-${tone}`;
}

export function ChartFrame({
  title,
  description,
  legend = [],
  table,
  busy = false,
  children,
}: {
  title: string;
  description?: string;
  legend?: LegendEntry[];
  table: ChartTable;
  busy?: boolean;
  children: ReactNode;
}) {
  const [showTable, setShowTable] = useState(false);
  const titleID = useId();
  return (
    <figure
      className={`chart-frame ${busy ? 'chart-busy' : ''}`}
      aria-labelledby={titleID}
      aria-busy={busy}
    >
      <figcaption className="chart-heading">
        <div>
          <h3 id={titleID}>{title}</h3>
          {description && <p className="muted text-xs">{description}</p>}
        </div>
        <button
          type="button"
          className="secondary-button"
          aria-pressed={showTable}
          onClick={() => setShowTable((current) => !current)}
        >
          {showTable ? 'Ver gráfico' : 'Ver como tabla'}
        </button>
      </figcaption>
      {legend.length > 1 && !showTable && (
        <ul className="chart-legend" aria-label="Leyenda">
          {legend.map((entry) => (
            <li key={entry.label}>
              <span
                className={`chart-key chart-key-${entry.shape} ${toneClass(entry.tone)}`}
                aria-hidden="true"
              />
              {entry.label}
            </li>
          ))}
        </ul>
      )}
      {showTable ? (
        <div
          className="table-container chart-table"
          role="region"
          aria-label={`Datos de ${title}`}
          tabIndex={0}
        >
          <table>
            <thead>
              <tr>
                {table.columns.map((column) => (
                  <th key={column} scope="col">
                    {column}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {table.rows.map((row, index) => (
                <tr key={`${row[0]}-${index}`}>
                  {row.map((cell, cellIndex) =>
                    cellIndex === 0 ? (
                      <th key={cellIndex} scope="row">
                        {cell}
                      </th>
                    ) : (
                      <td key={cellIndex} className="numeric-cell">
                        {cell}
                      </td>
                    ),
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        children
      )}
    </figure>
  );
}
