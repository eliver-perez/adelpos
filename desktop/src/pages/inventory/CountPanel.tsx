import { useEffect, useState } from 'react';
import type { FormEvent } from 'react';
import { Accordion } from '../../components/Accordion';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Notice } from '../../components/Forms';
import { Icon, IconLabel } from '../../components/Icon';
import { SegmentedControl } from '../../components/SegmentedControl';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, InventoryItem, InventoryUnit } from '../../services/inventory';
import {
  applyCount,
  approveCount,
  cancelCount,
  captureCountLines,
  saveCount,
  submitCount,
} from '../../services/inventoryControl';
import type { Count, CountCaptureInput, CountLine } from '../../services/inventoryControl';
import { formatSignedMoney } from '../../services/money';
import { notifyError, notifySuccess, notifyWarning } from '../../services/notify';
import { acceptDecimalText, inventoryQuantityShape } from '../../services/numericInput';
import { countLabel, pluralWord } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import {
  capturedText,
  countFolio,
  countStateTone,
  differenceLabels,
  differenceTone,
  measureInput,
} from './controlFormat';
import { costRequiredOptions, WarningList } from './ControlShared';
import { ItemPicker } from './InventoryFields';
import { formatQuantity, formatSignedQuantity, inventoryPermissionName } from './inventoryFormat';
import type { InventoryNavigate } from './inventoryNavigation';
import { MeasureSelect, ReasonAction } from './PurchaseFields';
import { Select } from '../../components/Select';

// Renglones que se muestran a la vez; «Mostrar más» agrega otro bloque (la hoja puede tener hasta
// 2000 insumos).
const visibleStep = 50;
// Go acepta de 1 a 200 renglones por guardado.
const captureChunk = 200;

type LineView = 'all' | 'pending' | 'counted';

// Captura local de un renglón aún no guardada: el texto tal como se tecleó y la unidad elegida.
interface LineDraft {
  quantity: string;
  measure: string;
}

// Unidad o presentación con la que se capturó el renglón (vacío: la unidad base).
function lineMeasure(line: CountLine) {
  if (line.presentation) {
    return `presentation:${line.presentation.id}`;
  }
  if (line.unit && line.unit.id !== line.item.unit.id) {
    return `unit:${line.unit.id}`;
  }
  return '';
}

function original(line: CountLine): LineDraft {
  return { quantity: line.captured_quantity ?? '', measure: lineMeasure(line) };
}

function changed(line: CountLine, draft: LineDraft | undefined) {
  if (!draft) {
    return false;
  }
  const base = original(line);
  return draft.quantity !== base.quantity || draft.measure !== base.measure;
}

// Búsqueda rápida por nombre o código, sin distinguir mayúsculas ni acentos.
function normalized(text: string) {
  return text.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
}

function matches(line: CountLine, search: string, view: LineView) {
  if (view === 'pending' && line.counted) {
    return false;
  }
  if (view === 'counted' && !line.counted) {
    return false;
  }
  const term = normalized(search.trim());
  if (term === '') {
    return true;
  }
  return (
    normalized(line.item.name).includes(term) ||
    normalized(line.item.code).includes(term) ||
    normalized(line.category_name ?? '').includes(term)
  );
}

// Unidad del renglón. Al principio solo ofrece la unidad base y la de la captura guardada; la
// opción «Otra unidad o presentación…» consulta ese insumo para ofrecer sus demás unidades y
// presentaciones (sin consultar todos los insumos de la hoja).
function CountMeasure({
  line,
  value,
  units,
  disabled,
  onChange,
}: {
  line: CountLine;
  value: string;
  units: InventoryUnit[];
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const loaded = useResource<InventoryItem>('inventory_item', {
    id: line.item.id,
    enabled: expanded,
    reloadEvent: inventoryChangeEvent,
  });
  const item = loaded.data?.id === line.item.id ? loaded.data : null;
  const label = `Unidad de ${line.item.name}`;
  if (item) {
    return (
      <MeasureSelect
        label={label}
        item={item}
        units={units}
        value={value}
        disabled={disabled}
        onChange={onChange}
      />
    );
  }
  return (
    <label className="field count-line-measure">
      <span>{label}</span>
      <Select
        value={value}
        disabled={disabled || expanded}
        options={[
          { value: '', label: `${line.item.unit.name} (${line.item.unit.code})` },
          ...(line.presentation
            ? [{ value: `presentation:${line.presentation.id}`, label: line.presentation.name }]
            : []),
          ...(line.unit && line.unit.id !== line.item.unit.id
            ? [{ value: `unit:${line.unit.id}`, label: `${line.unit.name} (${line.unit.code})` }]
            : []),
        ]}
        actions={[
          {
            label: expanded ? 'Consultando unidades…' : 'Otra unidad o presentación…',
            icon: 'more',
            disabled: expanded,
            onSelect: () => setExpanded(true),
          },
        ]}
        onChange={onChange}
      />
      {loaded.error && <small className="muted">{loaded.error}</small>}
    </label>
  );
}

// Teórico, diferencia y existencia actual que calcula Go (solo con theoretical_visible).
function LineDifference({ line, applied }: { line: CountLine; applied: boolean }) {
  if (!line.counted) {
    return <p className="muted text-xs">Sin contar: no se ajustará.</p>;
  }
  return (
    <dl className="count-line-figures">
      <div>
        <dt>{applied ? 'Teórico aplicado' : 'Teórico a esa hora'}</dt>
        <dd>{formatQuantity(line.theoretical_quantity, line.item.unit)}</dd>
      </div>
      <div>
        <dt>Diferencia</dt>
        <dd>
          <strong>{formatSignedQuantity(line.difference, line.item.unit)}</strong>{' '}
          {line.difference_kind && (
            <span className={differenceTone(line.difference_kind)}>
              {differenceLabels[line.difference_kind]}
            </span>
          )}
        </dd>
      </div>
      <div>
        <dt>Existencia actual</dt>
        <dd>{formatQuantity(line.current_quantity, line.item.unit)}</dd>
      </div>
      {applied && (
        <div>
          <dt>Movimiento</dt>
          <dd>
            {line.movement_label ?? 'Sin ajuste'}
            {line.value_display !== null && ` · ${formatSignedMoney(line.value_display)}`}
          </dd>
        </div>
      )}
    </dl>
  );
}

function CountLineRow({
  line,
  draft,
  editable,
  saving,
  theoreticalVisible,
  applied,
  units,
  timezone,
  onDraft,
  onRemove,
}: {
  line: CountLine;
  draft: LineDraft | undefined;
  editable: boolean;
  saving: boolean;
  theoreticalVisible: boolean;
  applied: boolean;
  units: InventoryUnit[];
  timezone: string;
  onDraft: (draft: LineDraft) => void;
  onRemove: (() => void) | null;
}) {
  const current = draft ?? original(line);
  const dirty = changed(line, draft);
  const className = [
    'count-line',
    line.counted ? 'count-line-counted' : '',
    dirty ? 'count-line-dirty' : '',
  ].join(' ');
  return (
    <li className={className} aria-label={`Renglón de ${line.item.name}`}>
      <div className="count-line-item">
        <strong>{line.item.name}</strong>
        <span className="muted text-xs">
          {line.item.code}
          {line.category_name ? ` · ${line.category_name}` : ''}
          {line.item.active ? '' : ' · inactivo'}
        </span>
        {line.counted && line.captured_at ? (
          <span className="text-xs">
            <span className="tag tag-good">Contado</span>{' '}
            {capturedText(line.captured_quantity, line.presentation, line.unit, line.item.unit)}
            {(line.presentation || (line.unit && line.unit.id !== line.item.unit.id)) &&
              ` = ${formatQuantity(line.counted_quantity, line.item.unit)}`}
            {` · ${serverDateTime(line.captured_at, timezone)}`}
            {line.captured_by_name ? ` · ${line.captured_by_name}` : ''}
          </span>
        ) : (
          <span className="text-xs">
            <span className="tag">Sin contar</span>
          </span>
        )}
        {dirty && <span className="count-line-unsaved text-xs">Sin guardar</span>}
      </div>
      {editable && (
        <div className="count-line-capture">
          <label className="field count-line-quantity">
            <span>Cantidad contada</span>
            <input
              type="text"
              inputMode="decimal"
              autoComplete="off"
              aria-label={`Cantidad contada de ${line.item.name}`}
              value={current.quantity}
              disabled={saving}
              placeholder="Sin contar"
              pattern="[0-9]+([.][0-9]+)?"
              onChange={(event) =>
                onDraft({
                  ...current,
                  quantity: acceptDecimalText(
                    event.target.value,
                    current.quantity,
                    inventoryQuantityShape,
                  ),
                })
              }
            />
          </label>
          <CountMeasure
            line={line}
            value={current.measure}
            units={units}
            disabled={saving}
            onChange={(measure) => onDraft({ ...current, measure })}
          />
          <div className="button-row count-line-actions">
            {line.counted && (
              <button
                type="button"
                className="secondary-button"
                disabled={saving || current.quantity === ''}
                aria-label={`Borrar la captura de ${line.item.name}`}
                onClick={() => onDraft({ ...current, quantity: '' })}
              >
                Borrar captura
              </button>
            )}
            {onRemove && (
              <button
                type="button"
                className="secondary-button"
                disabled={saving}
                aria-label={`Quitar ${line.item.name} del conteo`}
                onClick={onRemove}
              >
                Quitar
              </button>
            )}
          </div>
        </div>
      )}
      {theoreticalVisible && (
        <div className="count-line-difference">
          <LineDifference line={line} applied={applied} />
        </div>
      )}
      {!theoreticalVisible && applied && line.movement_label && (
        <p className="muted text-xs">{line.movement_label}</p>
      )}
    </li>
  );
}

function CountSteps({ count, timezone }: { count: Count; timezone: string }) {
  const steps: { label: string; value: string }[] = [
    { label: 'Almacén', value: count.warehouse.name },
    {
      label: 'Alcance',
      value: count.category ? `${count.scope_label} · ${count.category.name}` : count.scope_label,
    },
    {
      label: 'Creó',
      value: `${count.created_by_name} · ${serverDateTime(count.created_at, timezone)}`,
    },
  ];
  const optional: [string, string | null, string | null][] = [
    ['Envió a revisión', count.submitted_by_name, count.submitted_at],
    ['Aprobó', count.approved_by_name, count.approved_at],
    ['Aplicó', count.applied_by_name, count.applied_at],
    ['Canceló', count.cancelled_by_name, count.cancelled_at],
  ];
  for (const [label, name, at] of optional) {
    if (name && at) {
      steps.push({ label, value: `${name} · ${serverDateTime(at, timezone)}` });
    }
  }
  if (count.cancel_reason) {
    steps.push({ label: 'Motivo de la cancelación', value: count.cancel_reason });
  }
  if (count.reference_at) {
    steps.push({
      label: 'Hora de referencia',
      value: serverDateTime(count.reference_at, timezone),
    });
  }
  if (count.state === 'applied' && count.difference_lines !== null) {
    steps.push({
      label: 'Diferencias aplicadas',
      value: countLabel(count.difference_lines, 'renglón', 'renglones'),
    });
  }
  if (count.costs_visible && count.value_display !== null) {
    steps.push({ label: 'Valor del ajuste', value: formatSignedMoney(count.value_display) });
  }
  return (
    <dl className="inventory-facts" aria-label="Pasos del conteo">
      {steps.map((step) => (
        <div key={step.label}>
          <dt>{step.label}</dt>
          <dd>{step.value}</dd>
        </div>
      ))}
    </dl>
  );
}

// Agregar insumos a un conteo abierto (PATCH con la revisión vigente). Go rechaza el que ya está
// en otro conteo sin terminar del mismo almacén.
function AddCountItem({
  count,
  execute,
  busy,
  onChanged,
}: {
  count: Count;
  execute: InventoryExecute;
  busy: boolean;
  onChanged: (count: Count) => void;
}) {
  const [item, setItem] = useState<InventoryItem | null>(null);
  const [sending, setSending] = useState(false);
  async function add() {
    if (!item) {
      return;
    }
    setSending(true);
    try {
      const saved = await saveCount(execute, count.id, {
        revision: count.revision,
        add_item_ids: [item.id],
      });
      notifySuccess(`«${item.name}» agregado al ${countFolio(count.folio).toLowerCase()}.`);
      setItem(null);
      onChanged(saved);
    } catch (problem) {
      notifyError(problem, 'No se pudo agregar el insumo al conteo.');
    } finally {
      setSending(false);
    }
  }
  return (
    <div className="form-stack">
      <ItemPicker label="Insumo para agregar" value={item} activeOnly={false} onChange={setItem} />
      <div className="button-row">
        <button
          type="button"
          className="secondary-button"
          disabled={!item || busy || sending}
          onClick={() => void add()}
        >
          <IconLabel icon="plus">{sending ? 'Agregando…' : 'Agregar al conteo'}</IconLabel>
        </button>
      </div>
    </div>
  );
}

// Envío, aprobación, aplicación y cancelación con la revisión vigente. Cada paso exige su permiso
// (Go lo vuelve a comprobar) y aplicar se confirma: registra los movimientos de ajuste y no se
// repite.
function CountActions({
  count,
  can,
  execute,
  busy,
  pendingDrafts,
  onChanged,
  onNavigate,
}: {
  count: Count;
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  pendingDrafts: number;
  onChanged: (count: Count) => void;
  onNavigate?: InventoryNavigate;
}) {
  const [cancelling, setCancelling] = useState(false);
  const [sending, setSending] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const canPerform = can('inventory.counts.perform');
  const canApprove = can('inventory.counts.approve');
  const folio = countFolio(count.folio);
  const submittable = count.submittable && canPerform;
  const approvable = count.approvable && canApprove;
  const applicable = count.applicable && canApprove;
  const cancellable =
    count.cancellable && (count.state === 'open' ? canPerform || canApprove : canApprove);
  // Go avisa cuántos renglones quedan sin contar (no se ajustarán al aplicar).
  const uncounted = count.warnings.find((warning) => warning.code === 'COUNT_UNCOUNTED_LINES');
  // Renglones contados con diferencia según el cálculo vigente de Go (solo con teórico visible).
  const withDifference = count.lines.filter(
    (line) => line.counted && line.difference_kind !== null && line.difference_kind !== 'none',
  ).length;

  async function transition(
    action: typeof submitCount,
    success: string,
    failure: string,
    request: { title: string; message: string; confirmLabel: string },
  ) {
    const confirmed = await confirm({ ...request, cancelLabel: 'Revisar' });
    if (!confirmed) {
      return;
    }
    setSending(true);
    try {
      const saved = await action(execute, count.id, { revision: count.revision });
      notifySuccess(success);
      for (const warning of saved.warnings) {
        if (warning.code === 'NEGATIVE_STOCK') {
          notifyWarning(warning.message);
        }
      }
      onChanged(saved);
    } catch (problem) {
      // COST_REQUIRED: un sobrante de un insumo sin costo; el conteo no se aplicó y se reintenta
      // después de configurar el costo de referencia.
      notifyError(problem, failure, costRequiredOptions(problem, onNavigate));
    } finally {
      setSending(false);
    }
  }

  if (cancelling) {
    return (
      <ReasonAction
        title={`Cancelar el ${folio.toLowerCase()}`}
        description="El conteo cancelado no ajusta existencias y queda en el historial con el motivo."
        reasonLabel="Motivo de la cancelación"
        actionLabel="Cancelar conteo"
        confirmTitle={`Cancelar el ${folio.toLowerCase()}`}
        confirmMessage={(reason) =>
          `Se cancelará con el motivo «${reason}». Lo capturado no se aplicará y no se puede deshacer.`
        }
        busy={busy}
        onCancel={() => setCancelling(false)}
        onConfirm={async (reason) => {
          try {
            const saved = await cancelCount(execute, count.id, {
              revision: count.revision,
              reason,
            });
            notifySuccess(`${folio} cancelado: no ajustó existencias.`);
            setCancelling(false);
            onChanged(saved);
            return true;
          } catch (problem) {
            notifyError(problem, 'No se pudo cancelar el conteo.');
            return false;
          }
        }}
      />
    );
  }

  if (!submittable && !approvable && !applicable && !cancellable) {
    if (count.state === 'submitted' || count.state === 'approved') {
      return (
        <Notice>
          {count.state === 'submitted' ? 'Enviado a revisión' : 'Aprobado'}: aprobarlo y aplicarlo
          requiere el permiso «{inventoryPermissionName('inventory.counts.approve')}».
        </Notice>
      );
    }
    return null;
  }

  const locked = busy || sending;
  return (
    <div className="button-row control-actions count-actions">
      {submittable && (
        <button
          type="button"
          className="primary-button"
          disabled={locked || pendingDrafts > 0 || count.counted_lines === 0}
          onClick={() =>
            void transition(
              submitCount,
              `${folio} enviado a revisión.`,
              'No se pudo enviar el conteo.',
              {
                title: `Enviar el ${folio.toLowerCase()} a revisión`,
                message: `Se enviará con ${count.counted_lines} de ${countLabel(count.line_count, 'insumo contado', 'insumos contados')}.${uncounted ? ` ${uncounted.message}` : ''} Después ya no se captura: lo aprueba y aplica quien tiene el permiso.`,
                confirmLabel: 'Enviar a revisión',
              },
            )
          }
        >
          <IconLabel icon="send">Enviar a revisión</IconLabel>
        </button>
      )}
      {approvable && (
        <button
          type="button"
          className="primary-button"
          disabled={locked}
          onClick={() =>
            void transition(approveCount, `${folio} aprobado.`, 'No se pudo aprobar el conteo.', {
              title: `Aprobar el ${folio.toLowerCase()}`,
              message:
                'Se aprobarán las cantidades capturadas. Los ajustes se registran después, al aplicarlo.',
              confirmLabel: 'Aprobar conteo',
            })
          }
        >
          <IconLabel icon="check">Aprobar</IconLabel>
        </button>
      )}
      {applicable && (
        <button
          type="button"
          className="primary-button"
          disabled={locked}
          onClick={() =>
            void transition(
              applyCount,
              `${folio} aplicado: se registraron sus ajustes.`,
              'No se pudo aplicar el conteo.',
              {
                title: `Aplicar el ${folio.toLowerCase()}`,
                message: `${count.theoretical_visible ? `Con las diferencias calculadas ahora, ${pluralWord(withDifference, 'se registrará', 'se registrarán')} ${countLabel(withDifference, 'movimiento de ajuste', 'movimientos de ajuste')} (faltantes y sobrantes) con la hora de cada renglón. ` : ''}Antes se calculan los consumos de ventas pendientes y se conservan los movimientos posteriores a cada captura. No se puede deshacer ni aplicar dos veces.`,
                confirmLabel: 'Aplicar ajustes',
              },
            )
          }
        >
          <IconLabel icon="inventory">Aplicar ajustes</IconLabel>
        </button>
      )}
      {cancellable && (
        <button
          type="button"
          className="secondary-button"
          disabled={locked}
          onClick={() => setCancelling(true)}
        >
          Cancelar conteo
        </button>
      )}
      {submittable && pendingDrafts > 0 && (
        <span className="muted text-xs">Guarda el avance antes de enviarlo a revisión.</span>
      )}
      {dialog}
    </div>
  );
}

// Panel de un conteo físico: hoja de captura pensada para tableta (renglones grandes, búsqueda
// rápida y filtro de pendientes), avance guardado por bloques, diferencias calculadas por Go con
// la existencia teórica a la hora de cada renglón (solo con theoretical_visible: quien solo
// cuenta, cuenta a ciegas), pasos con quién y cuándo, y transiciones según el estado y el permiso.
export function CountPanel({
  countID,
  initial,
  can,
  execute,
  busy,
  timezone,
  onClose,
  onNavigate,
}: {
  countID: string;
  initial: Count | null;
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onClose: () => void;
  onNavigate?: InventoryNavigate;
}) {
  const [snapshot, setSnapshot] = useState<Count | null>(initial);
  const [drafts, setDrafts] = useState<Record<string, LineDraft>>({});
  const [search, setSearch] = useState('');
  const [view, setView] = useState<LineView>('all');
  const [visible, setVisible] = useState(visibleStep);
  const [saving, setSaving] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const resource = useResource<Count>('inventory_count', {
    id: countID,
    reloadEvent: inventoryChangeEvent,
  });
  const units = useResource<InventoryUnit[]>('inventory_units', {
    reloadEvent: inventoryChangeEvent,
  });

  useEffect(() => {
    if (resource.data) {
      setSnapshot(resource.data);
    }
  }, [resource.data]);

  const count = snapshot?.id === countID ? snapshot : null;
  const canPerform = can('inventory.counts.perform');
  const editable = !!count && count.editable && canPerform;
  const dirtyLines = count ? count.lines.filter((line) => changed(line, drafts[line.item.id])) : [];

  function updateDraft(line: CountLine, draft: LineDraft) {
    setDrafts((current) => ({ ...current, [line.item.id]: draft }));
  }

  function refreshed(saved: Count) {
    setSnapshot(saved);
  }

  // Guarda los renglones cambiados en bloques de hasta 200: cada bloque es una transacción corta
  // de Go con su propio operation_id, así que la venta sigue y otras terminales pueden capturar a
  // la vez. La hora de cada renglón la fija Go al guardar.
  async function saveProgress(event?: FormEvent) {
    event?.preventDefault();
    if (!count || dirtyLines.length === 0) {
      return;
    }
    setSaving(true);
    let saved = 0;
    try {
      for (let start = 0; start < dirtyLines.length; start += captureChunk) {
        const chunk = dirtyLines.slice(start, start + captureChunk);
        const input: CountCaptureInput = {
          lines: chunk.map((line) => {
            const draft = drafts[line.item.id] ?? original(line);
            return {
              item_id: line.item.id,
              counted_quantity: draft.quantity === '' ? null : draft.quantity,
              ...measureInput(draft.measure),
            };
          }),
        };
        const result = await captureCountLines(execute, count.id, input);
        saved += chunk.length;
        setSnapshot(result);
        setDrafts((current) => {
          const next = { ...current };
          for (const line of chunk) {
            delete next[line.item.id];
          }
          return next;
        });
      }
      notifySuccess(`Avance guardado: ${countLabel(saved, 'renglón', 'renglones')}.`, {
        description: 'Cada renglón guarda la hora en que se capturó.',
      });
    } catch (problem) {
      notifyError(
        problem,
        saved > 0
          ? `Se guardaron ${countLabel(saved, 'renglón', 'renglones')}; el resto no se pudo guardar.`
          : 'No se pudo guardar el avance del conteo.',
      );
    } finally {
      setSaving(false);
    }
  }

  async function removeLine(line: CountLine) {
    if (!count) {
      return;
    }
    if (line.counted) {
      const confirmed = await confirm({
        title: 'Quitar un insumo contado',
        message: `«${line.item.name}» ya tiene una cantidad capturada. Al quitarlo del conteo se descarta su captura.`,
        confirmLabel: 'Quitar del conteo',
        cancelLabel: 'Conservar',
        tone: 'danger',
      });
      if (!confirmed) {
        return;
      }
    }
    try {
      const saved = await saveCount(execute, count.id, {
        revision: count.revision,
        remove_item_ids: [line.item.id],
      });
      notifySuccess(`«${line.item.name}» se quitó del ${countFolio(count.folio).toLowerCase()}.`);
      setDrafts((current) => {
        const next = { ...current };
        delete next[line.item.id];
        return next;
      });
      setSnapshot(saved);
    } catch (problem) {
      notifyError(problem, 'No se pudo quitar el insumo del conteo.');
    }
  }

  async function close() {
    if (dirtyLines.length > 0) {
      const discard = await confirm({
        title: 'Captura sin guardar',
        message: `${countLabel(dirtyLines.length, 'renglón tiene', 'renglones tienen')} cantidades sin guardar. ¿Quieres descartarlas?`,
        confirmLabel: 'Descartar captura',
        cancelLabel: 'Seguir contando',
        tone: 'danger',
      });
      if (!discard) {
        return;
      }
    }
    onClose();
  }

  const heading = count ? `${countFolio(count.folio)} · ${count.warehouse.name}` : 'Conteo';
  const filtered = count ? count.lines.filter((line) => matches(line, search, view)) : [];
  const shown = filtered.slice(0, visible);
  const applied = count?.state === 'applied';
  const locked = busy || saving;

  return (
    <section className="panel form-stack count-panel" aria-label={heading}>
      <div className="section-heading">
        <div>
          <h2>{heading}</h2>
          {count && (
            <p className="muted text-xs">
              <span className={countStateTone(count.state)}>{count.state_label}</span>
              {` · ${count.counted_lines} de ${countLabel(count.line_count, 'insumo contado', 'insumos contados')}`}
            </p>
          )}
        </div>
        <button type="button" className="secondary-button" onClick={() => void close()}>
          Volver al listado
        </button>
      </div>
      {!count && <Notice>{resource.error || 'Consultando el conteo…'}</Notice>}
      {count && (
        <>
          {(count.state === 'open' ||
            count.state === 'submitted' ||
            count.state === 'approved') && (
            <div className="count-sales-note" role="note">
              <Icon name="clock" />
              <p>
                La venta sigue durante el conteo. Cada cantidad guarda la hora en que se capturó y
                se compara con la existencia teórica a esa hora; los movimientos posteriores se
                conservan al aplicar.
              </p>
            </div>
          )}
          {!count.theoretical_visible && (
            <Notice>
              Conteo a ciegas: la existencia teórica y las diferencias solo las ve quien aprueba
              conteos o consulta existencias.
            </Notice>
          )}
          <WarningList warnings={count.warnings} label="Avisos del conteo" />
          <CountSteps count={count} timezone={timezone} />
          {count.notes && (
            <p className="text-sm">
              <strong>Notas: </strong>
              {count.notes}
            </p>
          )}
          <CountActions
            key={`${count.id}:${count.revision}`}
            count={count}
            can={can}
            execute={execute}
            busy={locked}
            pendingDrafts={dirtyLines.length}
            onChanged={refreshed}
            onNavigate={onNavigate}
          />
          {editable && (
            <Accordion title="Agregar insumos al conteo" summary="Mientras está abierto">
              <AddCountItem count={count} execute={execute} busy={locked} onChanged={refreshed} />
            </Accordion>
          )}
          <form className="form-stack" onSubmit={(event) => void saveProgress(event)}>
            <div className="count-toolbar">
              <label className="field count-search">
                <span>Buscar en la hoja</span>
                <input
                  type="search"
                  maxLength={100}
                  autoComplete="off"
                  value={search}
                  placeholder="Nombre, código o categoría"
                  onChange={(event) => {
                    setSearch(event.target.value);
                    setVisible(visibleStep);
                  }}
                />
              </label>
              <div className="field">
                <span>Mostrar</span>
                <SegmentedControl
                  label="Renglones que se muestran"
                  value={view}
                  options={[
                    { value: 'all', label: `Todos (${count.line_count})` },
                    { value: 'pending', label: 'Sin contar' },
                    { value: 'counted', label: 'Contados' },
                  ]}
                  onChange={(next) => {
                    setView(next);
                    setVisible(visibleStep);
                  }}
                />
              </div>
            </div>
            <ul className="count-lines" aria-label="Hoja de conteo">
              {shown.map((line) => (
                <CountLineRow
                  key={line.item.id}
                  line={line}
                  draft={drafts[line.item.id]}
                  editable={editable}
                  saving={locked}
                  theoreticalVisible={count.theoretical_visible}
                  applied={applied}
                  units={units.data ?? []}
                  timezone={timezone}
                  onDraft={(draft) => updateDraft(line, draft)}
                  onRemove={editable && count.line_count > 1 ? () => void removeLine(line) : null}
                />
              ))}
              {filtered.length === 0 && (
                <li className="muted count-line-empty">No hay renglones con esta búsqueda.</li>
              )}
            </ul>
            {filtered.length > shown.length && (
              <div className="button-row">
                <button
                  type="button"
                  className="secondary-button"
                  onClick={() => setVisible((current) => current + visibleStep)}
                >
                  Mostrar más renglones
                </button>
                <span className="muted text-xs">
                  {shown.length} de {countLabel(filtered.length, 'renglón', 'renglones')}
                </span>
              </div>
            )}
            {editable && (
              <div className="form-actions count-save-bar">
                <button
                  type="submit"
                  className="primary-button"
                  disabled={locked || dirtyLines.length === 0}
                >
                  {saving ? 'Guardando…' : 'Guardar avance'}
                </button>
                <span className="muted text-xs">
                  {dirtyLines.length > 0
                    ? `${countLabel(dirtyLines.length, 'renglón', 'renglones')} sin guardar. Vacío y guardado borra la captura.`
                    : 'Sin cambios por guardar.'}
                </span>
              </div>
            )}
          </form>
        </>
      )}
      {dialog}
    </section>
  );
}
