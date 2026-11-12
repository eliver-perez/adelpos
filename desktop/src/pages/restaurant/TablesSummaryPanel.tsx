import { Icon, IconLabel } from '../../components/Icon';
import { tableStateIcons } from '../../components/restaurant/tableStateIcons';
import { countLabel, pluralWord } from '../../services/plural';
import type { Area, RestaurantTable, TableState } from '../../services/restaurant';

// Estados que resume el panel (§3.1 de la etapa 9D), en el orden de la leyenda. «Fuera de servicio»
// no se resume: se menciona aparte si hay alguna.
export const summaryStates: TableState[] = [
  'free',
  'occupied',
  'awaiting_payment',
  'partial_payment',
];

const summaryWords: Record<string, [string, string]> = {
  free: ['Libre', 'Libres'],
  occupied: ['Ocupada', 'Ocupadas'],
  awaiting_payment: ['Esperando pago', 'Esperando pago'],
  partial_payment: ['Pago parcial', 'Pago parcial'],
};

// Mesas del área que muestra el mapa operativo: las activas y las desactivadas que aún tienen
// cuentas abiertas (la misma regla del mapa).
export function areaTables(tables: RestaurantTable[], areaID: string) {
  return tables.filter(
    (table) => table.area_id === areaID && (table.active || table.account_count > 0),
  );
}

// Cuántas mesas del área hay en cada estado. El estado lo calcula Go; aquí solo se cuentan las
// mesas para presentarlas (no hay importes ni permisos).
export function countTableStates(tables: RestaurantTable[]) {
  const counts: Record<TableState, number> = {
    free: 0,
    occupied: 0,
    awaiting_payment: 0,
    partial_payment: 0,
    out_of_service: 0,
  };
  for (const table of tables) {
    counts[table.state] += 1;
  }
  return counts;
}

// Panel lateral de Mesas sin mesa elegida (etapa 9D, §3.1): el panel está siempre a la vista con el
// mismo ancho. Muestra el resumen del área (libres, ocupadas, esperando pago y pago parcial; cada
// número resalta esas mesas en el mapa y se vuelve a tocar para quitar el resaltado), las acciones
// rápidas «Cuenta sin mesa» y «Nuevo pedido para llevar» (las mismas funciones de sus botones) y el
// texto de ayuda que antes estaba sobre el mapa.
export function TablesSummaryPanel({
  area,
  tables,
  loading,
  highlight,
  onHighlight,
  canOpenWithoutTable,
  canOpenTakeaway,
  blocked,
  onAccountWithoutTable,
  onNewTakeaway,
}: {
  area: Area | null;
  // Mesas del área a la vista (areaTables).
  tables: RestaurantTable[];
  loading: boolean;
  highlight: TableState | null;
  onHighlight: (state: TableState | null) => void;
  canOpenWithoutTable: boolean;
  canOpenTakeaway: boolean;
  blocked: boolean;
  onAccountWithoutTable: () => void;
  onNewTakeaway: () => void;
}) {
  const counts = countTableStates(tables);
  const quickActions = canOpenWithoutTable || canOpenTakeaway;
  return (
    <aside className="table-panel table-summary-panel" aria-label="Resumen del área">
      <header className="table-panel-header">
        <div className="table-panel-title">
          <h2>
            <Icon name="table" size={22} className="table-panel-icon" />
            {area?.name ?? 'Mesas'}
          </h2>
          <p className="table-panel-facts muted">
            {loading ? 'Consultando áreas y mesas…' : countLabel(tables.length, 'mesa', 'mesas')}
          </p>
        </div>
      </header>
      <div className="table-panel-body table-summary-body">
        <div className="table-summary-counts" role="group" aria-label="Mesas por estado">
          {summaryStates.map((state) => {
            const count = counts[state];
            const [singular, plural] = summaryWords[state]!;
            const word = pluralWord(count, singular, plural);
            return (
              <button
                key={state}
                type="button"
                className={`table-summary-count table-summary-${state}`}
                aria-pressed={highlight === state}
                aria-label={`${count} ${word.toLowerCase()} · resaltar en el mapa`}
                data-state={state}
                disabled={count === 0 && highlight !== state}
                onClick={() => onHighlight(highlight === state ? null : state)}
              >
                <span className="table-summary-number">{count}</span>
                <span className="table-summary-label">
                  <Icon name={tableStateIcons[state]} size={14} />
                  {word}
                </span>
              </button>
            );
          })}
        </div>
        {counts.out_of_service > 0 && (
          <p className="muted text-xs table-summary-note">
            <Icon name={tableStateIcons.out_of_service} size={13} />
            {countLabel(counts.out_of_service, 'mesa', 'mesas')} fuera de servicio
          </p>
        )}
        <p className="muted text-sm table-summary-help">
          Toca una mesa para ver sus cuentas, abrir una nueva o capturar productos. Toca un número
          para resaltar esas mesas en el mapa.
        </p>
      </div>
      {quickActions && (
        <footer className="table-panel-footer">
          <div className="table-summary-actions" role="group" aria-label="Acciones rápidas">
            {canOpenWithoutTable && (
              <button
                type="button"
                className="secondary-button"
                disabled={blocked}
                onClick={onAccountWithoutTable}
              >
                <IconLabel icon="plus">Cuenta sin mesa</IconLabel>
              </button>
            )}
            {canOpenTakeaway && (
              <button
                type="button"
                className="secondary-button"
                disabled={blocked}
                onClick={onNewTakeaway}
              >
                <IconLabel icon="bag">Nuevo pedido para llevar</IconLabel>
              </button>
            )}
          </div>
        </footer>
      )}
    </aside>
  );
}
