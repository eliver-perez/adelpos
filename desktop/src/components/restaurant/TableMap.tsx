import type { CSSProperties, PointerEvent } from 'react';
import { useRef } from 'react';
import type { Area, RestaurantTable } from '../../services/restaurant';

interface TableMapProps {
  areas: Area[];
  tables: RestaurantTable[];
  areaID: string;
  selectedTableID?: string;
  onAreaChange: (id: string) => void;
  onSelect: (table: RestaurantTable) => void;
  designer?: boolean;
  onPosition?: (id: string, x: number, y: number) => void;
}

const stateLabels = { free: 'Libre', occupied: 'Ocupada', out_of_service: 'Fuera de servicio' };

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
  const availableAreas = designer ? areas : areas.filter((area) => area.active);
  const visible = tables.filter(
    (table) => table.area_id === areaID && (designer || table.active || table.account_count > 0),
  );

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
                style={style}
                className={`restaurant-table table-${table.state} table-${table.shape} ${selectedTableID === table.id ? 'selected-table' : ''}`}
                aria-label={`${table.name}, ${stateLabels[table.state]}, ${table.account_count} cuentas`}
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
                <strong>{table.name}</strong>
                <span>
                  {table.account_count
                    ? `${table.account_count} cuentas`
                    : stateLabels[table.state]}
                </span>
              </button>
            );
          })}
        </div>
      </div>
      <div className="map-legend muted">
        <span>
          <i className="legend-dot legend-free" />
          Libre
        </span>
        <span>
          <i className="legend-dot legend-occupied" />
          Ocupada
        </span>
        <span>
          <i className="legend-dot legend-unavailable" />
          Fuera de servicio
        </span>
        {designer && <span>Arrastra o usa flechas; Shift mueve de uno en uno.</span>}
      </div>
    </div>
  );
}
