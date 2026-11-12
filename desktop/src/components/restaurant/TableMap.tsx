import type { CSSProperties, KeyboardEvent, PointerEvent } from 'react';
import { useEffect, useRef, useState } from 'react';
import { shortcutHint } from '../../commands/hints';
import { formatMoney } from '../../services/money';
import { countLabel } from '../../services/plural';
import {
  elapsedTime,
  operationalTables,
  searchTables,
  tableStateLabels,
} from '../../services/restaurant';
import type { Area, RestaurantTable, TableState } from '../../services/restaurant';
import { Icon } from '../Icon';
import type { IconName } from '../Icon';

interface TableMapProps {
  areas: Area[];
  tables: RestaurantTable[];
  areaID: string;
  selectedTableID?: string;
  onAreaChange: (id: string) => void;
  // Tocar una mesa del mapa o elegirla en el buscador (que puede estar en otra área).
  onSelect: (table: RestaurantTable) => void;
  designer?: boolean;
  onPosition?: (id: string, x: number, y: number) => void;
}

// Icono de cada estado (etapa 7.1): además del color, el estado se lee en texto y con su icono.
// Los colores y los estados son los de la 7.1 (§0-B de la etapa 9C): no cambian.
const stateIcons: Record<TableState, IconName | null> = {
  free: null,
  occupied: 'clock',
  awaiting_payment: 'receipt',
  partial_payment: 'cash',
  out_of_service: 'lock',
};

// Nombre accesible del mosaico: nombre y estado primero (así lo buscan las pruebas y los lectores
// de pantalla), después cuentas, tiempo ocupada, precuenta o saldo y la capacidad de la mesa. El
// tiempo y el saldo los calcula Go; aquí solo se presentan.
function accessibleName(table: RestaurantTable) {
  const parts = [
    table.name,
    tableStateLabels[table.state],
    countLabel(table.account_count, 'cuenta', 'cuentas'),
  ];
  if (table.account_count > 0) {
    parts.push(elapsedTime(table.occupied_seconds));
  }
  if (table.state === 'awaiting_payment') {
    parts.push('precuenta impresa');
  }
  if (table.state === 'partial_payment') {
    parts.push(`saldo ${formatMoney(table.balance)}`);
  }
  if (table.state !== 'out_of_service') {
    parts.push(`mesa para ${table.seats}`);
  }
  return parts.join(', ');
}

// Mosaico de la mesa (etapa 9C, §4): icono de mesa y nombre grande; estado en texto con su icono;
// tiempo ocupada y capacidad con sus iconos; saldo en pago parcial y número de cuentas si hay más
// de una. La cuenta no guarda comensales: el icono de personas indica la capacidad de la mesa
// (lugares), como explica la leyenda. En mesas pequeñas se ocultan primero los datos secundarios
// (consultas de contenedor en styles.css); el nombre accesible los conserva todos.
function TableTile({ table }: { table: RestaurantTable }) {
  const icon = stateIcons[table.state];
  const withAccounts = table.account_count > 0;
  return (
    <span className="table-tile">
      <span className="table-tile-name">
        <Icon name="table" size={14} className="table-tile-icon" />
        <strong>{table.name}</strong>
      </span>
      <span className="table-tile-state">
        {icon && <Icon name={icon} size={13} />}
        {tableStateLabels[table.state]}
      </span>
      {table.state === 'partial_payment' && (
        <span className="table-tile-balance">Saldo {formatMoney(table.balance)}</span>
      )}
      {table.state !== 'out_of_service' && (
        <span className="table-tile-details">
          {withAccounts && (
            <span className="table-tile-time">
              <Icon name="clock" size={12} />
              {elapsedTime(table.occupied_seconds)}
            </span>
          )}
          <span className="table-tile-seats">
            <Icon name="users" size={12} />
            {table.seats}
          </span>
        </span>
      )}
      {table.account_count > 1 && (
        <span className="table-tile-accounts">
          {countLabel(table.account_count, 'cuenta', 'cuentas')}
        </span>
      )}
    </span>
  );
}

// Buscador de mesas del encabezado del mapa (etapa 9C, §4). Participa en ⌘F / Ctrl+F de la fase C
// (data-screen-search). Las coincidencias aparecen como botones con su área y su estado; elegir
// una muestra su área y abre su panel, igual que tocarla en el mapa. Intro elige la primera y
// Escape borra lo escrito.
function TableSearch({
  query,
  onQuery,
  onEnter,
}: {
  query: string;
  onQuery: (query: string) => void;
  onEnter: () => void;
}) {
  const find = shortcutHint('edit.find');

  function keyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'Enter') {
      event.preventDefault();
      onEnter();
      return;
    }
    // Escape con texto solo borra la búsqueda; sin texto, la capa de teclado sale del campo.
    if (event.key === 'Escape' && query !== '') {
      event.preventDefault();
      onQuery('');
    }
  }

  return (
    <div className="table-search" role="search" aria-label="Mesas">
      <Icon name="search" size={18} className="table-search-icon" />
      <input
        type="search"
        value={query}
        maxLength={60}
        placeholder="Buscar mesa"
        aria-label="Buscar mesa por nombre, número o área"
        aria-keyshortcuts={find?.aria}
        title={find ? `Buscar mesa · ${find.label}` : undefined}
        autoComplete="off"
        data-screen-search
        onChange={(event) => onQuery(event.target.value)}
        onKeyDown={keyDown}
      />
      {query !== '' && (
        <button
          type="button"
          className="table-search-clear"
          aria-label="Borrar la búsqueda de mesas"
          onClick={() => onQuery('')}
        >
          <Icon name="close" size={18} />
        </button>
      )}
    </div>
  );
}

const visibleResults = 8;

function SearchResults({
  query,
  results,
  onChoose,
}: {
  query: string;
  results: ReturnType<typeof searchTables>;
  onChoose: (table: RestaurantTable) => void;
}) {
  const shown = results.slice(0, visibleResults);
  const hidden = results.length - shown.length;
  return (
    <div className="table-search-results">
      <p className="muted table-search-status" role="status">
        {results.length === 0
          ? `Ninguna mesa coincide con «${query.trim()}».`
          : countLabel(results.length, 'mesa encontrada', 'mesas encontradas')}
        {hidden > 0 && ` · se muestran ${shown.length}; escribe más para acotar.`}
      </p>
      {shown.length > 0 && (
        <div className="table-search-options" role="group" aria-label="Mesas encontradas">
          {shown.map(({ table, area }) => (
            <button
              key={table.id}
              type="button"
              className="secondary-button table-search-option"
              aria-label={`${table.name}, ${area.name}, ${tableStateLabels[table.state]}`}
              onClick={() => onChoose(table)}
            >
              <Icon name="table" size={18} />
              <span className="table-search-option-text">
                <strong>{table.name}</strong>
                <span>
                  {area.name} · {tableStateLabels[table.state]}
                </span>
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// Leyenda del mapa con los cinco estados (muestra de color, icono y texto) y el significado de los
// iconos de tiempo y capacidad.
const legend: TableState[] = [
  'free',
  'occupied',
  'awaiting_payment',
  'partial_payment',
  'out_of_service',
];

export function TableMap({
  areas,
  tables,
  areaID,
  selectedTableID,
  onAreaChange,
  onSelect,
  designer = false,
  onPosition,
}: TableMapProps) {
  const canvas = useRef<HTMLDivElement>(null);
  const dragging = useRef<{
    id: string;
    clientX: number;
    clientY: number;
    x: number;
    y: number;
    width: number;
    height: number;
  } | null>(null);
  const [query, setQuery] = useState('');
  const availableAreas = designer ? areas : areas.filter((area) => area.active);
  const visible = tables.filter(
    (table) => table.area_id === areaID && (designer || table.active || table.account_count > 0),
  );
  const searching = !designer && query.trim() !== '';
  const results = searching
    ? searchTables(operationalTables(tables, areas), areas, query, areaID)
    : [];

  // Elegir una mesa encontrada la muestra (con su área) y lleva el foco a su mosaico: así Escape
  // cierra su panel y los atajos (⌘N, ⌘P) actúan, porque el foco ya no está en un campo.
  const focusTable = useRef('');
  function choose(table: RestaurantTable) {
    setQuery('');
    focusTable.current = table.id;
    onSelect(table);
  }
  useEffect(() => {
    const id = focusTable.current;
    if (!id) {
      return;
    }
    // Un solo intento, en el render que sigue a la elección (la página ya cambió de área).
    focusTable.current = '';
    canvas.current?.querySelector<HTMLElement>(`[data-table-id="${id}"]`)?.focus();
  });

  function move(event: PointerEvent) {
    const current = dragging.current;
    const rectangle = canvas.current?.getBoundingClientRect();
    if (!current || !rectangle || !onPosition || rectangle.width === 0 || rectangle.height === 0)
      return;
    const x = Math.round(current.x + ((event.clientX - current.clientX) * 1000) / rectangle.width);
    const y = Math.round(current.y + ((event.clientY - current.clientY) * 560) / rectangle.height);
    onPosition(
      current.id,
      Math.max(0, Math.min(1000 - current.width, x)),
      Math.max(0, Math.min(560 - current.height, y)),
    );
  }

  return (
    <div className={`table-map ${designer ? 'table-map-designer' : ''}`}>
      {/* Etapa 9C (§4): encabezado del mapa con el selector de área y, en la operación, el
          buscador de mesas. */}
      <div className="table-map-header">
        <div className="area-tabs" role="group" aria-label="Áreas del restaurante">
          {availableAreas.map((area) => (
            <button
              key={area.id}
              className={areaID === area.id ? 'selected-area' : ''}
              onClick={() => onAreaChange(area.id)}
              type="button"
              aria-pressed={areaID === area.id}
            >
              {area.name}
              {!area.active ? ' · inactiva' : ''}
            </button>
          ))}
        </div>
        {!designer && (
          <TableSearch
            query={query}
            onQuery={setQuery}
            onEnter={() => {
              if (results[0]) {
                choose(results[0].table);
              }
            }}
          />
        )}
      </div>
      {searching && <SearchResults query={query} results={results} onChoose={choose} />}
      <div className="table-map-viewport">
        <div
          className="table-canvas"
          ref={canvas}
          aria-label={designer ? 'Diseñador de distribución de mesas' : 'Mapa de mesas'}
        >
          {visible.length === 0 && (
            <p className="table-map-empty muted">
              {availableAreas.length
                ? 'Esta área aún no tiene mesas.'
                : 'Crea un área y sus mesas para comenzar.'}
            </p>
          )}
          {visible.map((table) => {
            const style: CSSProperties = {
              left: `min(${table.x / 10}%, calc(100% - 44px))`,
              top: `min(${table.y / 5.6}%, calc(100% - 44px))`,
              width: `${table.width / 10}%`,
              height: `${table.height / 5.6}%`,
            };
            return (
              <button
                key={table.id}
                type="button"
                data-table-id={table.id}
                style={style}
                className={`restaurant-table table-${table.state} table-${table.shape} ${selectedTableID === table.id ? 'selected-table' : ''}`}
                aria-label={accessibleName(table)}
                aria-pressed={selectedTableID === table.id}
                onClick={() => onSelect(table)}
                onPointerDown={(event) => {
                  if (!designer || !onPosition) return;
                  event.currentTarget.setPointerCapture?.(event.pointerId);
                  dragging.current = {
                    id: table.id,
                    clientX: event.clientX,
                    clientY: event.clientY,
                    x: table.x,
                    y: table.y,
                    width: table.width,
                    height: table.height,
                  };
                }}
                onPointerMove={move}
                onPointerUp={() => {
                  dragging.current = null;
                }}
                onPointerCancel={() => {
                  dragging.current = null;
                }}
                onKeyDown={(event) => {
                  if (!designer || !onPosition) return;
                  const directions: Record<string, [number, number]> = {
                    ArrowLeft: [-1, 0],
                    ArrowRight: [1, 0],
                    ArrowUp: [0, -1],
                    ArrowDown: [0, 1],
                  };
                  const direction = directions[event.key];
                  if (!direction) return;
                  event.preventDefault();
                  const step = event.shiftKey ? 1 : 10;
                  onPosition(
                    table.id,
                    Math.max(0, Math.min(1000 - table.width, table.x + direction[0] * step)),
                    Math.max(0, Math.min(560 - table.height, table.y + direction[1] * step)),
                  );
                }}
              >
                <TableTile table={table} />
              </button>
            );
          })}
        </div>
      </div>
      <div className="map-legend muted">
        <ul className="map-legend-states" aria-label="Estados de las mesas">
          {legend.map((state) => {
            const icon = stateIcons[state];
            return (
              <li key={state} className="map-legend-state">
                <i className={`legend-swatch legend-${state}`} aria-hidden="true" />
                {icon && <Icon name={icon} size={12} />}
                {tableStateLabels[state]}
              </li>
            );
          })}
        </ul>
        <ul className="map-legend-states" aria-label="Datos de cada mesa">
          <li className="map-legend-state">
            <Icon name="clock" size={12} />
            Tiempo ocupada
          </li>
          <li className="map-legend-state">
            <Icon name="users" size={12} />
            Capacidad (lugares)
          </li>
        </ul>
        {designer && <span>Arrastra o usa flechas; Shift mueve de uno en uno.</span>}
      </div>
    </div>
  );
}
