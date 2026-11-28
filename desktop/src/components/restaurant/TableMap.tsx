import type { CSSProperties, KeyboardEvent, MouseEvent, PointerEvent, RefObject } from 'react';
import { precheckName } from '../../services/documentNames';
import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { shortcutHint } from '../../commands/hints';
import { formatMoney } from '../../services/money';
import { countLabel } from '../../services/plural';
import {
  elapsedTime,
  operationalTables,
  searchTables,
  tableStateLabels,
} from '../../services/restaurant';
import type {
  Area,
  RestaurantTable,
  ScreenUnion,
  TableGroup,
  TableState,
  TableUnion,
} from '../../services/restaurant';
import { Icon } from '../Icon';
import { tableStateIcons } from './tableStateIcons';

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
  // Etapa 9D (§3.2): escala automática al espacio disponible (mapa operativo de Mesas). Sin ella,
  // el lienzo proporcional de 1000 × 560 de siempre (diseñador y consulta en «Áreas y
  // distribución»).
  autoFit?: boolean;
  // Etapa 9D (§3.1): estado que el resumen del panel resalta en el mapa (null: ninguno).
  highlight?: TableState | null;
  // Umbral de cuenta abierta por mucho tiempo (settings.long_open_hours de Go), para el texto de la
  // advertencia; la marca long_open de cada mesa la calcula Go.
  longOpenHours?: number;
  // Etapa M1 (contrato §10): las uniones abiertas (contorno sólido con su nombre y su capacidad), los grupos activos
  // (marco tenue con su nombre) y la unión que vive en la pantalla (contorno discontinuo). Sin la capacidad
  // table-unions-v1, vacíos.
  unions?: TableUnion[];
  groups?: TableGroup[];
  screenUnion?: ScreenUnion | null;
  // Atajo de un grupo al tocar su nombre en el marco («Unir Mesa 3»); sin él, el nombre no se toca.
  onGroupShortcut?: (group: TableGroup) => void;
}

// Etapa 9D (§3.2): escala automática del mapa operativo. Al abrir un área, el mapa calcula una sola
// escala (píxeles por unidad del espacio lógico de 1000 × 560) para que se vean todas sus mesas en el
// espacio disponible, con la misma escala para todas. Ninguna mesa queda con su lado menor por
// debajo de unos 64 px: si con ese tamaño no caben, el mapa se desplaza (arrastrando con el dedo y
// con barras). La escala tampoco pasa del doble, para que pocas mesas no se vuelvan enormes. Solo
// presentación: las posiciones y medidas guardadas en Go no cambian.
export const minimumTableSide = 64;
export const maximumMapScale = 2;
const fitMargin = 16;

export interface TableFit {
  scale: number;
  // Esquina superior izquierda del conjunto de mesas, en unidades lógicas.
  originX: number;
  originY: number;
  // Margen en píxeles antes de la primera mesa (centra el conjunto si sobra espacio).
  offsetX: number;
  offsetY: number;
  // Medidas del lienzo en píxeles (al menos las del espacio visible).
  width: number;
  height: number;
  // Las mesas no caben con el tamaño mínimo: el mapa se desplaza.
  overflow: boolean;
}

type TableBox = Pick<RestaurantTable, 'x' | 'y' | 'width' | 'height'>;

export function fitTables(
  tables: TableBox[],
  viewport: { width: number; height: number },
): TableFit | null {
  if (tables.length === 0 || viewport.width <= 0 || viewport.height <= 0) {
    return null;
  }
  const originX = Math.min(...tables.map((table) => table.x));
  const originY = Math.min(...tables.map((table) => table.y));
  const spanX = Math.max(1, Math.max(...tables.map((table) => table.x + table.width)) - originX);
  const spanY = Math.max(1, Math.max(...tables.map((table) => table.y + table.height)) - originY);
  const available = {
    width: Math.max(1, viewport.width - fitMargin * 2),
    height: Math.max(1, viewport.height - fitMargin * 2),
  };
  const fit = Math.min(available.width / spanX, available.height / spanY);
  const smallest = Math.max(
    1,
    Math.min(...tables.map((table) => Math.min(table.width, table.height))),
  );
  const minimum = minimumTableSide / smallest;
  const scale = Math.max(minimum, Math.min(fit, maximumMapScale));
  const content = {
    width: spanX * scale + fitMargin * 2,
    height: spanY * scale + fitMargin * 2,
  };
  // Medio píxel de tolerancia: el redondeo del punto flotante no debe crear barras de un píxel.
  const overflowX = content.width > viewport.width + 0.5;
  const overflowY = content.height > viewport.height + 0.5;
  return {
    scale,
    originX,
    originY,
    offsetX: fitMargin + Math.max(0, (viewport.width - content.width) / 2),
    offsetY: fitMargin + Math.max(0, (viewport.height - content.height) / 2),
    width: overflowX ? Math.ceil(content.width) : viewport.width,
    height: overflowY ? Math.ceil(content.height) : viewport.height,
    overflow: overflowX || overflowY,
  };
}

// Medidas del lienzo en el estilo. El espacio visible puede medir fracciones de píxel (por ejemplo, con
// la pantalla de Windows al 125 %) y clientWidth/clientHeight lo informan redondeado: un lienzo con esa
// medida podía pasarse una fracción, mostrar la barra de desplazamiento (que en Windows ocupa espacio),
// encoger el espacio, quitar la barra y repetir sin fin (etapa 9E). En la dimensión que no se desborda,
// el lienzo se pide 1 px menor y su min-width/min-height de 100 % lo estira a la medida exacta.
export function canvasStyle(fit: TableFit, viewport: { width: number; height: number }) {
  const width = fit.width > viewport.width ? fit.width : Math.max(0, fit.width - 1);
  const height = fit.height > viewport.height ? fit.height : Math.max(0, fit.height - 1);
  return { width: `${width}px`, height: `${height}px` };
}

// Medida del espacio visible del mapa (sin barras de desplazamiento). Sin ResizeObserver (pruebas
// sin diseño) queda en cero y el mapa usa la distribución proporcional de siempre.
function useViewportSize(element: RefObject<HTMLDivElement | null>, enabled: boolean) {
  const [size, setSize] = useState({ width: 0, height: 0 });
  useLayoutEffect(() => {
    const node = element.current;
    if (!enabled || !node) {
      return;
    }
    const measure = () => {
      const next = { width: node.clientWidth, height: node.clientHeight };
      setSize((current) =>
        current.width === next.width && current.height === next.height ? current : next,
      );
    };
    measure();
    if (typeof ResizeObserver === 'undefined') {
      return;
    }
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, [element, enabled]);
  return size;
}

// Distancia que debe recorrer el dedo (o el puntero) para que el toque se tome como arrastre del
// mapa y no como toque de una mesa.
const panThreshold = 8;

// Nombre accesible del mosaico: nombre y estado primero (así lo buscan las pruebas y los lectores
// de pantalla), después cuentas, tiempo ocupada, precuenta o saldo y la capacidad de la mesa. El
// tiempo y el saldo los calcula Go; aquí solo se presentan.
function accessibleName(table: RestaurantTable, longOpenHours?: number, union?: TableUnion) {
  const parts = [
    table.name,
    tableStateLabels[table.state],
    countLabel(table.account_count, 'cuenta', 'cuentas'),
  ];
  // Etapa M1: una mesa unida se abre como la unión; el nombre accesible lo dice.
  if (union) {
    parts.splice(1, 0, `unida en ${union.name}`);
  }
  if (table.account_count > 0) {
    parts.push(elapsedTime(table.occupied_seconds));
  }
  if (table.state === 'awaiting_payment') {
    // Con el nombre visible que eligió el restaurante (etapa 9F, §4.5).
    const precheck = precheckName();
    parts.push(`${precheck.lower} ${precheck.printedAdjective}`);
  }
  if (table.state === 'partial_payment') {
    parts.push(`saldo ${formatMoney(table.balance)}`);
  }
  if (table.long_open) {
    parts.push(longOpenText(longOpenHours));
  }
  if (table.state !== 'out_of_service') {
    parts.push(`mesa para ${table.seats}`);
  }
  return parts.join(', ');
}

// Texto de la advertencia de cuenta abierta por mucho tiempo (§3.4).
export function longOpenText(hours?: number) {
  return hours ? `abierta hace más de ${hours} h` : 'abierta por mucho tiempo';
}

// Mosaico de la mesa (etapa 9C, §4): icono de mesa y nombre grande; estado en texto con su icono;
// tiempo ocupada y capacidad con sus iconos; saldo en pago parcial y número de cuentas si hay más
// de una. La cuenta no guarda comensales: el icono de personas indica la capacidad de la mesa
// (lugares), como explica la leyenda. En mesas pequeñas se ocultan primero los datos secundarios
// (consultas de contenedor en styles.css); el nombre accesible los conserva todos.
//
// Etapa 9D (§3.4): una mesa cuya cuenta lleva abierta más horas que el umbral (long_open, de Go)
// muestra su tiempo en una etiqueta ámbar con el triángulo de advertencia, distinta del color de
// cualquier estado.
function TableTile({ table, longOpenHours }: { table: RestaurantTable; longOpenHours?: number }) {
  const withAccounts = table.account_count > 0;
  return (
    <span className="table-tile">
      <span className="table-tile-name">
        <Icon name="table" size={14} className="table-tile-icon" />
        <strong>{table.name}</strong>
      </span>
      <span className="table-tile-state">
        <Icon name={tableStateIcons[table.state]} size={13} />
        {tableStateLabels[table.state]}
      </span>
      {table.state === 'partial_payment' && (
        <span className="table-tile-balance">Saldo {formatMoney(table.balance)}</span>
      )}
      {table.state !== 'out_of_service' && (
        <span className="table-tile-details">
          {withAccounts && (
            <span
              className={`table-tile-time ${table.long_open ? 'table-tile-long-open' : ''}`}
              title={table.long_open ? longOpenText(longOpenHours) : undefined}
            >
              <Icon name={table.long_open ? 'alert' : 'clock'} size={12} />
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

// Etapa M1 (contrato §10): en la lista de mesas encontradas, una unión es una sola entrada con su nombre (la primera
// de sus mesas que coincide); sus mesas no salen por separado.
export function searchEntries(results: ReturnType<typeof searchTables>, unions: TableUnion[]) {
  const seen = new Set<string>();
  const entries: (ReturnType<typeof searchTables>[number] & { union: TableUnion | null })[] = [];
  for (const result of results) {
    const union = unions.find((entry) => entry.table_ids.includes(result.table.id)) ?? null;
    if (union && seen.has(union.id)) {
      continue;
    }
    if (union) {
      seen.add(union.id);
    }
    entries.push({ ...result, union });
  }
  return entries;
}

function SearchResults({
  query,
  results,
  onChoose,
}: {
  query: string;
  results: ReturnType<typeof searchEntries>;
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
          {shown.map(({ table, area, union }) => (
            <button
              key={table.id}
              type="button"
              className="secondary-button table-search-option"
              aria-label={`${union?.name ?? table.name}, ${area.name}, ${tableStateLabels[table.state]}`}
              onClick={() => onChoose(table)}
            >
              <Icon name="table" size={18} />
              <span className="table-search-option-text">
                <strong>{union?.name ?? table.name}</strong>
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
  autoFit = false,
  highlight = null,
  longOpenHours,
  unions = [],
  groups = [],
  screenUnion = null,
  onGroupShortcut,
}: TableMapProps) {
  const canvas = useRef<HTMLDivElement>(null);
  const viewport = useRef<HTMLDivElement>(null);
  const dragging = useRef<{
    id: string;
    clientX: number;
    clientY: number;
    x: number;
    y: number;
    width: number;
    height: number;
  } | null>(null);
  // Arrastre del mapa operativo cuando las mesas no caben (§3.2).
  const panning = useRef<{
    pointerID: number;
    clientX: number;
    clientY: number;
    left: number;
    top: number;
    moved: boolean;
  } | null>(null);
  const swallowClick = useRef(false);
  const [panActive, setPanActive] = useState(false);
  const [query, setQuery] = useState('');
  const availableAreas = designer ? areas : areas.filter((area) => area.active);
  const visible = tables.filter(
    (table) => table.area_id === areaID && (designer || table.active || table.account_count > 0),
  );
  const searching = !designer && query.trim() !== '';
  const results = searching
    ? searchEntries(searchTables(operationalTables(tables, areas), areas, query, areaID), unions)
    : [];
  // El diseñador de «Áreas y distribución» conserva el lienzo proporcional de 1000 × 560 (las
  // posiciones se editan sobre el espacio completo); el mapa operativo se ajusta a las mesas.
  const fitMode = autoFit && !designer;
  const size = useViewportSize(viewport, fitMode);
  const fit = fitMode ? fitTables(visible, size) : null;

  // Al abrir otra área el mapa vuelve a su esquina superior izquierda.
  useEffect(() => {
    viewport.current?.scrollTo?.({ left: 0, top: 0 });
  }, [areaID]);

  // Al resaltar un estado desde el resumen, la primera mesa resaltada queda a la vista.
  useEffect(() => {
    if (!highlight) {
      return;
    }
    canvas.current
      ?.querySelector<HTMLElement>('.table-highlighted')
      ?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
  }, [highlight, areaID]);

  // Elegir una mesa encontrada la muestra (con su área) y lleva el foco a su mosaico: así Escape
  // cierra su panel y los atajos (⌘N, ⌘P) actúan, porque el foco ya no está en un campo.
  const focusTable = useRef<{ id: string; area: string } | null>(null);
  function choose(table: RestaurantTable) {
    setQuery('');
    focusTable.current = { id: table.id, area: table.area_id };
    onSelect(table);
  }
  useEffect(() => {
    const pending = focusTable.current;
    if (!pending) {
      return;
    }
    // Se espera al render que ya muestra el área de la mesa: el mapa puede volver a dibujarse antes
    // con el área anterior (se observó en WebKit en Windows, etapa 9E) y entonces el mosaico todavía
    // no existe. En ese render hay un solo intento.
    if (pending.area !== areaID) {
      return;
    }
    focusTable.current = null;
    canvas.current?.querySelector<HTMLElement>(`[data-table-id="${pending.id}"]`)?.focus();
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

  // Arrastrar el mapa con el dedo o el puntero lo desplaza; el toque que termina un arrastre no
  // elige la mesa sobre la que empezó. Con el tacto nativo el navegador puede tomar el
  // desplazamiento por su cuenta (pointercancel): entonces este arrastre se descarta.
  function panStart(event: PointerEvent<HTMLDivElement>) {
    swallowClick.current = false;
    const node = viewport.current;
    if (!fitMode || !node || event.button !== 0) {
      return;
    }
    if (node.scrollWidth <= node.clientWidth && node.scrollHeight <= node.clientHeight) {
      return;
    }
    panning.current = {
      pointerID: event.pointerId,
      clientX: event.clientX,
      clientY: event.clientY,
      left: node.scrollLeft,
      top: node.scrollTop,
      moved: false,
    };
  }

  function panMove(event: PointerEvent<HTMLDivElement>) {
    const current = panning.current;
    const node = viewport.current;
    if (!current || !node || current.pointerID !== event.pointerId) {
      return;
    }
    const dx = event.clientX - current.clientX;
    const dy = event.clientY - current.clientY;
    if (!current.moved) {
      if (Math.hypot(dx, dy) < panThreshold) {
        return;
      }
      current.moved = true;
      setPanActive(true);
      node.setPointerCapture?.(event.pointerId);
    }
    node.scrollLeft = current.left - dx;
    node.scrollTop = current.top - dy;
  }

  function panEnd(event: PointerEvent<HTMLDivElement>) {
    const current = panning.current;
    if (!current || current.pointerID !== event.pointerId) {
      return;
    }
    panning.current = null;
    setPanActive(false);
    if (current.moved && event.type === 'pointerup') {
      swallowClick.current = true;
    }
  }

  function clickCapture(event: MouseEvent<HTMLDivElement>) {
    if (swallowClick.current) {
      swallowClick.current = false;
      event.preventDefault();
      event.stopPropagation();
    }
  }

  // Posición y medida de cada mesa: en píxeles con la escala automática (mapa operativo) o en
  // proporción del lienzo de 1000 × 560 (diseñador y pruebas sin diseño).
  function tableStyle(table: RestaurantTable): CSSProperties {
    if (fit) {
      return {
        left: `${fit.offsetX + (table.x - fit.originX) * fit.scale}px`,
        top: `${fit.offsetY + (table.y - fit.originY) * fit.scale}px`,
        width: `${table.width * fit.scale}px`,
        height: `${table.height * fit.scale}px`,
      };
    }
    return {
      left: `min(${table.x / 10}%, calc(100% - 44px))`,
      top: `min(${table.y / 5.6}%, calc(100% - 44px))`,
      width: `${table.width / 10}%`,
      height: `${table.height / 5.6}%`,
    };
  }

  // Etapa M1 (contrato §10): rectángulo que envuelve un conjunto de mesas del área a la vista, con la misma fórmula
  // que cada mesa y un margen alrededor (el contorno de una unión y el marco de un grupo). null si ninguna está a la
  // vista.
  function boxStyle(ids: string[], margin: number): CSSProperties | null {
    const members = visible.filter((table) => ids.includes(table.id));
    if (members.length === 0) {
      return null;
    }
    const left = Math.min(...members.map((table) => table.x));
    const top = Math.min(...members.map((table) => table.y));
    const right = Math.max(...members.map((table) => table.x + table.width));
    const bottom = Math.max(...members.map((table) => table.y + table.height));
    if (fit) {
      return {
        left: `${fit.offsetX + (left - fit.originX) * fit.scale - margin}px`,
        top: `${fit.offsetY + (top - fit.originY) * fit.scale - margin}px`,
        width: `${(right - left) * fit.scale + margin * 2}px`,
        height: `${(bottom - top) * fit.scale + margin * 2}px`,
      };
    }
    return {
      left: `calc(${left / 10}% - ${margin}px)`,
      top: `calc(${top / 5.6}% - ${margin}px)`,
      width: `calc(${(right - left) / 10}% + ${margin * 2}px)`,
      height: `calc(${(bottom - top) / 5.6}% + ${margin * 2}px)`,
    };
  }
  const areaGroups = groups.filter((group) => group.area_id === areaID);
  const areaUnions = designer ? [] : unions.filter((union) => union.area_id === areaID);
  const unionOf = (table: RestaurantTable) =>
    areaUnions.find((union) => union.table_ids.includes(table.id));
  // La unión elegida (o la que vive en la pantalla desde la mesa elegida) se ve elegida completa.
  const selectedIDs = new Set<string>(selectedTableID ? [selectedTableID] : []);
  for (const union of areaUnions) {
    if (selectedTableID && union.table_ids.includes(selectedTableID)) {
      union.table_ids.forEach((id) => selectedIDs.add(id));
    }
  }
  if (screenUnion && screenUnion.startTableID === selectedTableID) {
    screenUnion.allTableIDs.forEach((id) => selectedIDs.add(id));
  }

  const pannable = !!fit?.overflow;
  return (
    <div
      className={`table-map ${designer ? 'table-map-designer' : ''} ${
        fitMode ? 'table-map-operation' : ''
      }`}
    >
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
      {/* Etapa 9D (§3.2): el mapa y su leyenda en un mismo marco; la leyenda va dentro, en una sola
          línea, y solo las mesas se desplazan. */}
      <div className="table-map-frame">
        <div
          className={`table-map-viewport ${pannable ? 'table-map-pannable' : ''} ${
            panActive ? 'table-map-panning' : ''
          }`}
          ref={viewport}
          onPointerDown={panStart}
          onPointerMove={panMove}
          onPointerUp={panEnd}
          onPointerCancel={panEnd}
          onClickCapture={clickCapture}
        >
          <div
            className={`table-canvas ${fit ? 'table-canvas-fit' : ''}`}
            ref={canvas}
            aria-label={designer ? 'Diseñador de distribución de mesas' : 'Mapa de mesas'}
            style={fit ? canvasStyle(fit, size) : undefined}
            data-scale={fit ? fit.scale.toFixed(4) : undefined}
            data-overflow={fit ? String(fit.overflow) : undefined}
          >
            {visible.length === 0 && (
              <p className="table-map-empty muted">
                {availableAreas.length
                  ? 'Esta área no tiene mesas.'
                  : 'Crea un área y sus mesas para comenzar.'}
              </p>
            )}
            {/* Etapa M1: el marco tenue de cada mesa divisible, con su nombre; tocar el nombre es su atajo («Unir
                Mesa 3») mientras ninguna sección está unida. Cada sección sigue siendo una mesa que se toca. */}
            {areaGroups.map((group) => {
              const style = boxStyle(group.table_ids, 12);
              if (!style) {
                return null;
              }
              const joined = visible.some(
                (table) => group.table_ids.includes(table.id) && !!table.union_id,
              );
              return (
                <div
                  key={`grupo-${group.id}`}
                  className="table-group-frame"
                  style={style}
                  data-group-id={group.id}
                >
                  {onGroupShortcut && !joined && !designer ? (
                    <button
                      type="button"
                      className="table-group-label table-group-shortcut"
                      aria-label={`Unir ${group.name}`}
                      title={`Unir ${group.name}`}
                      onClick={() => onGroupShortcut(group)}
                    >
                      <Icon name="table" size={12} />
                      {group.name}
                    </button>
                  ) : (
                    <span className="table-group-label">{group.name}</span>
                  )}
                </div>
              );
            })}
            {/* Etapa M1: el contorno sólido de cada unión abierta con el nombre y la capacidad que calcula Go, y el
                discontinuo de la unión que vive en la pantalla (nace con el primer producto). Las mesas siguen
                tocándose por debajo y abren la unión. */}
            {areaUnions.map((union) => {
              const style = boxStyle(union.table_ids, 6);
              return style ? (
                <div
                  key={`union-${union.id}`}
                  className={`table-union-outline table-union-${union.state}`}
                  style={style}
                  data-union-id={union.id}
                  aria-hidden="true"
                >
                  <span className="table-union-label">
                    {union.name} · {countLabel(union.seats, 'lugar', 'lugares')}
                  </span>
                </div>
              ) : null;
            })}
            {!designer &&
              screenUnion &&
              (() => {
                const style = boxStyle(screenUnion.allTableIDs, 6);
                return style ? (
                  <div
                    className="table-union-outline table-union-screen"
                    style={style}
                    aria-hidden="true"
                  >
                    <span className="table-union-label">
                      {screenUnion.name} · {countLabel(screenUnion.seats, 'lugar', 'lugares')} · sin
                      guardar
                    </span>
                  </div>
                ) : null;
              })()}
            {visible.map((table) => {
              const highlighted = !!highlight && table.state === highlight;
              return (
                <button
                  key={table.id}
                  type="button"
                  data-table-id={table.id}
                  style={tableStyle(table)}
                  className={`restaurant-table table-${table.state} table-${table.shape} ${
                    selectedIDs.has(table.id) ? 'selected-table' : ''
                  } ${highlighted ? 'table-highlighted' : ''} ${
                    table.long_open ? 'table-long-open' : ''
                  } ${unionOf(table) ? 'table-in-union' : ''}`}
                  aria-label={accessibleName(table, longOpenHours, unionOf(table))}
                  aria-pressed={selectedIDs.has(table.id)}
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
                  <TableTile table={table} longOpenHours={longOpenHours} />
                </button>
              );
            })}
          </div>
        </div>
        <div className="map-legend muted">
          <ul className="map-legend-states" aria-label="Estados de las mesas">
            {legend.map((state) => (
              <li key={state} className="map-legend-state">
                <i className={`legend-swatch legend-${state}`} aria-hidden="true" />
                <Icon name={tableStateIcons[state]} size={12} />
                {tableStateLabels[state]}
              </li>
            ))}
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
    </div>
  );
}
