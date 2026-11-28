import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react';
import type { KeyboardEvent } from 'react';
import { trapFocus } from '../../components/ConfirmDialog';
import { Icon } from '../../components/Icon';
import { countLabel } from '../../services/plural';
import { operationalTables, previewUnion } from '../../services/restaurant';
import type {
  Overview,
  RestaurantTable,
  TableGroup,
  TableUnion,
  UnionPreview,
} from '../../services/restaurant';

// Etapa M1 (§1 y §3; contrato §10): «Unir con…», el atajo de una mesa divisible, «Agregar mesa» y «Quitar mesa».
// Go decide todo: qué mesas se pueden unir, el nombre, la capacidad y qué pasa con las cuentas. Este diálogo solo
// ofrece las mesas del área, pide la vista previa de Go (no escribe nada) y muestra sus problemas tal cual; la
// escritura la hace la pantalla con lo elegido.

export type JoinRequest =
  // «Unir con…» desde una mesa libre u ocupada (la mesa de inicio).
  | { kind: 'join'; start: RestaurantTable }
  // El atajo de un grupo («Unir Mesa 3»), desde una de sus secciones.
  | { kind: 'group'; group: TableGroup; start: RestaurantTable }
  // «Agregar mesa» y «Quitar mesa» de una unión abierta.
  | { kind: 'add'; union: TableUnion }
  | { kind: 'remove'; union: TableUnion };

// Lo elegido en el diálogo: las mesas que se agregan, se unen o se quitan (sin la de inicio) y la vista previa de Go.
export interface JoinChoice {
  tableIDs: string[];
  preview: UnionPreview | null;
}

// Por qué una mesa no se ofrece (contrato §10): ocupada, unida o fuera de servicio, según lo que manda Go en el mapa.
// Es solo presentación: lo que vale es la vista previa de Go.
export function unavailableReason(table: RestaurantTable, unions: TableUnion[]) {
  if (!table.active || table.out_of_service || table.state === 'out_of_service') {
    return 'Fuera de servicio o desactivada';
  }
  if (table.union_id || unions.some((union) => union.table_ids.includes(table.id))) {
    return 'Unida con otras mesas';
  }
  if (table.state !== 'free') {
    return 'Ocupada';
  }
  return '';
}

function dialogTitle(request: JoinRequest) {
  switch (request.kind) {
    case 'join':
      return `Unir ${request.start.name} con…`;
    case 'group':
      return `Unir ${request.group.name}`;
    case 'add':
      return `Agregar una mesa a ${request.union.name}`;
    case 'remove':
      return `Quitar una mesa de ${request.union.name}`;
  }
}

const confirmLabels: Record<JoinRequest['kind'], string> = {
  join: 'Unir',
  group: 'Unir',
  add: 'Agregar',
  remove: 'Quitar',
};

// Mesas que se ofrecen: las del área (en el orden del mapa); al unir, sin la de inicio; al agregar, sin las de la
// unión; al quitar, solo las de la unión.
function offeredTables(request: JoinRequest, overview: Overview) {
  const tables = operationalTables(overview.tables, overview.areas);
  switch (request.kind) {
    case 'join':
      return tables.filter(
        (table) => table.area_id === request.start.area_id && table.id !== request.start.id,
      );
    case 'add':
      return tables.filter(
        (table) =>
          table.area_id === request.union.area_id && !request.union.table_ids.includes(table.id),
      );
    case 'remove':
      return request.union.table_ids
        .map((id) => overview.tables.find((table) => table.id === id))
        .filter((table): table is RestaurantTable => !!table);
    case 'group':
      return [];
  }
}

// Cuerpo de la vista previa de Go: unir (la primera es la de inicio, M1-D25), el grupo o agregar a la unión.
function previewBody(request: JoinRequest, chosen: string[]) {
  switch (request.kind) {
    case 'join':
      return chosen.length > 0 ? { table_ids: [request.start.id, ...chosen] } : null;
    case 'group':
      return { group_id: request.group.id };
    case 'add':
      return chosen.length > 0 ? { union_id: request.union.id, table_ids: chosen } : null;
    case 'remove':
      return null;
  }
}

export function JoinTablesDialog({
  request,
  overview,
  busy,
  error,
  onCancel,
  onConfirm,
}: {
  request: JoinRequest;
  overview: Overview;
  // Mientras Go responde la escritura.
  busy: boolean;
  // Rechazo de la escritura (el texto de Go), para corregir sin cerrar el diálogo.
  error: string;
  onCancel: () => void;
  onConfirm: (choice: JoinChoice) => void;
}) {
  const titleID = useId();
  const panel = useRef<HTMLDivElement>(null);
  const cancelButton = useRef<HTMLButtonElement>(null);
  const [chosen, setChosen] = useState<string[]>([]);
  const [preview, setPreview] = useState<UnionPreview | null>(null);
  const [checking, setChecking] = useState(false);
  const [previewError, setPreviewError] = useState('');
  const generation = useRef(0);
  const unions = overview.unions ?? [];
  const offered = offeredTables(request, overview);
  // Al unir se eligen varias mesas; al agregar o quitar, una a la vez (addTableToUnion es de una mesa).
  const multiple = request.kind === 'join';
  const body = previewBody(request, chosen);
  const bodyKey = JSON.stringify(body);

  useLayoutEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    cancelButton.current?.focus();
    return () => {
      if (previous && previous.isConnected) {
        previous.focus();
      }
    };
  }, []);

  // La vista previa se pide de nuevo cada vez que cambia lo elegido; una respuesta vieja se ignora.
  useEffect(() => {
    const current = ++generation.current;
    setPreview(null);
    setPreviewError('');
    const next = JSON.parse(bodyKey) as Parameters<typeof previewUnion>[0] | null;
    if (!next) {
      setChecking(false);
      return;
    }
    setChecking(true);
    previewUnion(next).then(
      (answer) => {
        if (current === generation.current) {
          setPreview(answer);
          setChecking(false);
        }
      },
      (problem: unknown) => {
        if (current === generation.current) {
          setPreviewError(
            problem instanceof Error && problem.message
              ? problem.message
              : 'No se pudo revisar la unión. Inténtalo de nuevo.',
          );
          setChecking(false);
        }
      },
    );
  }, [bodyKey]);

  function toggle(id: string) {
    setChosen((current) => {
      if (current.includes(id)) {
        return current.filter((entry) => entry !== id);
      }
      return multiple ? [...current, id] : [id];
    });
  }

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      if (!busy) {
        onCancel();
      }
      return;
    }
    if (event.key === 'Tab' && panel.current) {
      trapFocus(event, panel.current);
    }
  }

  const needsPreview = request.kind !== 'remove';
  const allowed =
    !busy &&
    (request.kind === 'group' || chosen.length > 0) &&
    (!needsPreview || (!!preview && preview.problems.length === 0 && !checking));

  return (
    <div className="dialog-backdrop">
      <div
        ref={panel}
        className="dialog-panel join-tables-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        onKeyDown={handleKeyDown}
      >
        <div className="dialog-heading">
          <Icon name="table" size={26} className="dialog-heading-icon" />
          <h2 id={titleID}>{dialogTitle(request)}</h2>
        </div>
        {request.kind !== 'group' && (
          <div
            className="join-tables-options"
            role="group"
            aria-label={request.kind === 'remove' ? 'Mesas de la unión' : 'Mesas del área'}
          >
            {offered.length === 0 && <p className="muted">No hay otras mesas en esta área.</p>}
            {offered.map((table) => {
              const reason = request.kind === 'remove' ? '' : unavailableReason(table, unions);
              const selected = chosen.includes(table.id);
              return (
                <button
                  key={table.id}
                  type="button"
                  className={`secondary-button join-table-option ${selected ? 'join-table-selected' : ''}`}
                  aria-pressed={selected}
                  disabled={!!reason || busy}
                  onClick={() => toggle(table.id)}
                >
                  <strong>{table.name}</strong>
                  <span className="muted">
                    {reason || countLabel(table.seats, 'lugar', 'lugares')}
                  </span>
                </button>
              );
            })}
          </div>
        )}
        {request.kind === 'remove' && (
          <p className="muted">
            Al quitar una mesa, ninguna cuenta sale de la unión. Si queda una sola mesa, la unión se
            deshace.
          </p>
        )}
        {needsPreview && (
          <div className="join-tables-preview" role="status" aria-live="polite">
            {checking && <p className="muted">Revisando con la computadora principal…</p>}
            {preview && !checking && (
              <p className="join-tables-name">
                <strong>{preview.name}</strong> · {countLabel(preview.seats, 'lugar', 'lugares')}
              </p>
            )}
            {preview?.problems.map((problem, index) => (
              <p key={`${problem.code}-${index}`} className="join-tables-problem">
                {problem.message}
              </p>
            ))}
            {previewError && <p className="join-tables-problem">{previewError}</p>}
          </div>
        )}
        {error && (
          <p className="join-tables-problem" role="alert">
            {error}
          </p>
        )}
        <div className="dialog-actions">
          <button
            ref={cancelButton}
            type="button"
            className="secondary-button dialog-button"
            disabled={busy}
            onClick={onCancel}
          >
            Cancelar
          </button>
          <button
            type="button"
            className="primary-button dialog-button"
            disabled={!allowed}
            aria-busy={busy || undefined}
            onClick={() => onConfirm({ tableIDs: chosen, preview })}
          >
            {confirmLabels[request.kind]}
          </button>
        </div>
      </div>
    </div>
  );
}
