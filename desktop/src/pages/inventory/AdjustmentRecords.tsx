import { useEffect, useState } from 'react';
import type { FormEvent, ReactNode } from 'react';
import { OptionalDateField } from '../../components/catalog/DateField';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { SegmentedControl } from '../../components/SegmentedControl';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, InventoryUnit, Warehouse } from '../../services/inventory';
import {
  adjustmentReasonDirection,
  adjustmentReasonLabels,
  authorizeAdjustment,
  recordAdjustment,
  rejectAdjustment,
} from '../../services/inventoryControl';
import type {
  Adjustment,
  AdjustmentInput,
  AdjustmentKind,
  AdjustmentPage,
  AdjustmentReason,
  AdjustmentState,
  AdjustmentSummary,
} from '../../services/inventoryControl';
import { formatSignedMoney } from '../../services/money';
import { notifyError, notifySuccess, notifyWarning } from '../../services/notify';
import { acceptIntegerText } from '../../services/numericInput';
import { countLabel, pluralWord } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import {
  adjustmentFolio,
  adjustmentKindOptions,
  adjustmentStateOptions,
  adjustmentStateTone,
  capturedText,
  controlLineInput,
  directionLabels,
  effectiveDirection,
  emptyControlLine,
  reasonOptions,
} from './controlFormat';
import type { ControlLineDraft } from './controlFormat';
import { ControlLinesEditor } from './ControlLinesEditor';
import {
  costRequiredOptions,
  defaultWarehouseID,
  WarehouseSelect,
  WarningList,
} from './ControlShared';
import { Pagination } from './InventoryFields';
import { formatQuantity, formatSignedQuantity, inventoryPermissionName } from './inventoryFormat';
import type { InventoryNavigate } from './inventoryNavigation';
import { measureName, ReasonAction } from './PurchaseFields';
import { Select } from '../../components/Select';

const pageSize = 25;

// Leer ajustes y mermas: cualquiera de los permisos de registro o autorización, o leer
// movimientos (Go aplica la misma regla).
export function canReadAdjustments(can: (permission: string) => boolean) {
  return (
    can('inventory.adjustments.record') ||
    can('inventory.adjustments.authorize') ||
    can('inventory.waste.record') ||
    can('inventory.movements.read')
  );
}

interface AdjustmentFilter {
  kind: AdjustmentKind | '';
  state: AdjustmentState | '';
  reason: AdjustmentReason | '';
  warehouse: string;
  from: string;
  until: string;
  search: string;
  page: number;
}

function adjustmentQuery(filter: AdjustmentFilter) {
  const query: Record<string, string> = {
    page: String(filter.page),
    page_size: String(pageSize),
  };
  const optional: [string, string][] = [
    ['kind', filter.kind],
    ['state', filter.state],
    ['reason', filter.reason],
    ['warehouse_id', filter.warehouse],
    ['from', filter.from],
    ['until', filter.until],
    ['search', filter.search.trim()],
  ];
  for (const [key, value] of optional) {
    if (value !== '') {
      query[key] = value;
    }
  }
  return query;
}

function AdjustmentFilters({
  filter,
  warehouses,
  onChange,
}: {
  filter: AdjustmentFilter;
  warehouses: Warehouse[];
  onChange: (filter: AdjustmentFilter) => void;
}) {
  return (
    <section className="panel catalog-filters" aria-label="Filtros de ajustes y mermas">
      <Field label="Folio">
        <input
          type="search"
          inputMode="numeric"
          autoComplete="off"
          value={filter.search}
          placeholder="Folio exacto"
          onChange={(event) =>
            onChange({
              ...filter,
              search: acceptIntegerText(event.target.value, filter.search, 9),
              page: 1,
            })
          }
        />
      </Field>
      <Field label="Tipo">
        <Select<AdjustmentKind | ''>
          value={filter.kind}
          options={[{ value: '', label: 'Ajustes y mermas' }, ...adjustmentKindOptions]}
          onChange={(kind) => onChange({ ...filter, kind, page: 1 })}
        />
      </Field>
      <Field label="Estado del ajuste">
        <Select<AdjustmentState | ''>
          value={filter.state}
          options={[{ value: '', label: 'Todos' }, ...adjustmentStateOptions]}
          onChange={(state) => onChange({ ...filter, state, page: 1 })}
        />
      </Field>
      <Field label="Motivo">
        <Select<AdjustmentReason | ''>
          value={filter.reason}
          options={[{ value: '', label: 'Todos' }, ...reasonOptions('discrepancy')]}
          onChange={(reason) => onChange({ ...filter, reason, page: 1 })}
        />
      </Field>
      <Field label="Almacén">
        <Select
          value={filter.warehouse}
          options={[
            { value: '', label: 'Todos' },
            ...warehouses.map((warehouse) => ({
              value: warehouse.id,
              label: `${warehouse.name}${warehouse.active ? '' : ' (inactivo)'}`,
            })),
          ]}
          onChange={(warehouse) => onChange({ ...filter, warehouse, page: 1 })}
        />
      </Field>
      <OptionalDateField
        label="Registrado desde"
        value={filter.from}
        onChange={(from) => onChange({ ...filter, from, page: 1 })}
      />
      <OptionalDateField
        label="Registrado hasta"
        value={filter.until}
        onChange={(until) => onChange({ ...filter, until, page: 1 })}
      />
    </section>
  );
}

function AdjustmentsTable({
  page,
  timezone,
  onOpen,
}: {
  page: AdjustmentPage;
  timezone: string;
  onOpen: (adjustment: AdjustmentSummary) => void;
}) {
  const columns = 7 + (page.costs_visible ? 1 : 0);
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <thead>
          <tr>
            <th scope="col">Documento</th>
            <th scope="col">Registro</th>
            <th scope="col">Motivo</th>
            <th scope="col">Almacén</th>
            <th scope="col">Renglones</th>
            {page.costs_visible && (
              <th scope="col" className="inventory-number">
                Valor
              </th>
            )}
            <th scope="col">Estado</th>
            <th scope="col">
              <span className="sr-only">Acciones</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {page.items.map((adjustment) => (
            <tr
              key={adjustment.id}
              className={
                adjustment.state === 'pending_authorization' ? 'control-row-pending' : undefined
              }
            >
              <td className="inventory-cell-name">
                <strong>{adjustmentFolio(adjustment.kind, adjustment.folio)}</strong>
                <span className="muted text-xs inventory-cell-detail">{adjustment.kind_label}</span>
              </td>
              <td>
                {serverDateTime(adjustment.requested_at, timezone)}
                <span className="muted text-xs inventory-cell-detail">
                  {adjustment.requested_by_name}
                </span>
              </td>
              <td>
                {adjustment.reason_label}
                {adjustment.reason_detail && (
                  <span className="muted text-xs inventory-cell-detail">
                    {adjustment.reason_detail}
                  </span>
                )}
              </td>
              <td>{adjustment.warehouse.name}</td>
              <td>{countLabel(adjustment.line_count, 'renglón', 'renglones')}</td>
              {page.costs_visible && (
                <td className="inventory-number">{formatSignedMoney(adjustment.value_display)}</td>
              )}
              <td>
                <span className={adjustmentStateTone(adjustment.state)}>
                  {adjustment.state_label}
                </span>
              </td>
              <td>
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`Abrir ${adjustmentFolio(adjustment.kind, adjustment.folio).toLowerCase()}`}
                  onClick={() => onOpen(adjustment)}
                >
                  Abrir
                </button>
              </td>
            </tr>
          ))}
          {page.items.length === 0 && (
            <tr>
              <td colSpan={columns} className="muted">
                No hay ajustes ni mermas con estos filtros.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

// Descripción de un renglón capturado para la confirmación, con el texto tal como se tecleó.
function lineDescription(
  line: ControlLineDraft,
  kind: AdjustmentKind,
  reason: AdjustmentReason | '',
  units: InventoryUnit[],
) {
  const measure = measureName(line.item, line.measure, units);
  const direction = effectiveDirection(kind, reason, line.direction);
  let sense = direction === 'out' ? 'faltante' : 'sobrante';
  if (kind === 'waste') {
    sense = 'baja';
  }
  return `${line.item?.name ?? 'Insumo sin elegir'}: ${sense} de ${line.quantity} ${measure}`;
}

// Alta de un ajuste de discrepancia o de una merma operativa. El motivo es de lista cerrada (con
// botones) y cada renglón se captura sin signo: el sentido lo indica «Faltante» o «Sobrante» (una
// merma siempre es salida). Go valida el sentido contra el motivo, convierte la cantidad a la
// unidad base y decide si se aplica (con permiso de autorizar) o queda pendiente.
function AdjustmentForm({
  kind,
  canAuthorize,
  execute,
  busy,
  onRecorded,
  onCancel,
  onNavigate,
}: {
  kind: AdjustmentKind;
  canAuthorize: boolean;
  execute: InventoryExecute;
  busy: boolean;
  onRecorded: (adjustment: Adjustment) => void;
  onCancel: () => void;
  onNavigate?: InventoryNavigate;
}) {
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const units = useResource<InventoryUnit[]>('inventory_units', {
    reloadEvent: inventoryChangeEvent,
  });
  const [reason, setReason] = useState<AdjustmentReason | ''>('');
  const [detail, setDetail] = useState('');
  const [warehouseID, setWarehouseID] = useState('');
  const [notes, setNotes] = useState('');
  const [lines, setLines] = useState<ControlLineDraft[]>([emptyControlLine(1)]);
  const [sending, setSending] = useState(false);
  const [reasonMissing, setReasonMissing] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const warehouseList = warehouses.data ?? [];
  const unitList = units.data ?? [];
  const selectedWarehouse = warehouseID || defaultWarehouseID(warehouseList);
  const waste = kind === 'waste';
  const locked = busy || sending;
  const allowed = reason === '' ? 'both' : adjustmentReasonDirection[reason];
  const title = waste ? 'Registrar merma' : 'Registrar ajuste';

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (reason === '') {
      setReasonMissing(true);
      return;
    }
    const warehouse = warehouseList.find((entry) => entry.id === selectedWarehouse);
    let effect = canAuthorize
      ? 'Se aplicará de inmediato con un movimiento compensatorio por renglón.'
      : 'Quedará pendiente de autorización: no modifica existencias hasta que otra persona con el permiso de autorizar lo apruebe.';
    if (waste) {
      effect = 'La merma se aplica de inmediato y descuenta del almacén.';
    }
    const confirmed = await confirm({
      title,
      message: (
        <>
          <p>
            {waste ? 'Merma' : 'Ajuste de discrepancia'} por «{adjustmentReasonLabels[reason]}» en «
            {warehouse?.name ?? ''}» con {countLabel(lines.length, 'renglón', 'renglones')}:
          </p>
          <ul>
            {lines.map((line) => (
              <li key={line.key}>{lineDescription(line, kind, reason, unitList)}</li>
            ))}
          </ul>
          <p>
            {effect} Los movimientos del libro no se editan ni se borran; una existencia negativa no
            bloquea el registro.
          </p>
        </>
      ),
      confirmLabel: title,
      cancelLabel: 'Revisar',
    });
    if (!confirmed) {
      return;
    }
    const input: AdjustmentInput = {
      kind,
      reason,
      warehouse_id: selectedWarehouse,
      lines: lines.map((line) =>
        controlLineInput(
          { ...line, direction: effectiveDirection(kind, reason, line.direction) },
          true,
        ),
      ),
    };
    if (detail.trim() !== '') {
      input.reason_detail = detail.trim();
    }
    if (notes.trim() !== '') {
      input.notes = notes.trim();
    }
    setSending(true);
    try {
      const saved = await recordAdjustment(execute, input);
      const folio = adjustmentFolio(saved.kind, saved.folio);
      if (saved.kind === 'waste') {
        notifySuccess(`${folio} registrada y aplicada.`);
      } else if (saved.state === 'pending_authorization') {
        notifySuccess(`${folio} registrado: queda pendiente de autorización.`);
      } else {
        notifySuccess(`${folio} registrado y aplicado.`);
      }
      for (const warning of saved.warnings) {
        notifyWarning(warning.message);
      }
      onRecorded(saved);
    } catch (problem) {
      notifyError(
        problem,
        waste ? 'No se pudo registrar la merma.' : 'No se pudo registrar el ajuste.',
        costRequiredOptions(problem, onNavigate),
      );
    } finally {
      setSending(false);
    }
  }

  return (
    <section className="panel form-stack" aria-label={title}>
      <div className="section-heading">
        <div>
          <h2>{title}</h2>
          <p className="muted text-xs">
            {waste
              ? 'Pérdida operativa: rotura, derrame, caducidad o consumo extraordinario. Solo registra salidas.'
              : 'Diferencia entre lo registrado y lo físico. No se edita la existencia: se registra un movimiento compensatorio.'}
          </p>
        </div>
        <button type="button" className="secondary-button" onClick={onCancel}>
          Volver al listado
        </button>
      </div>
      <Notice>{warehouses.error || units.error}</Notice>
      <form className="form-stack" onSubmit={(event) => void submit(event)}>
        <fieldset className="form-stack" disabled={locked}>
          <div className="field control-reason-field">
            <span>Motivo</span>
            <SegmentedControl
              label={waste ? 'Motivo de la merma' : 'Motivo del ajuste'}
              value={reason}
              options={reasonOptions(kind)}
              onChange={(next) => {
                setReason(next);
                setReasonMissing(false);
              }}
            />
            {reasonMissing && (
              <small className="inventory-negative-text" role="alert">
                Elige el motivo.
              </small>
            )}
          </div>
          <div className="form-grid">
            <WarehouseSelect
              label={waste ? 'Almacén de la merma' : 'Almacén del ajuste'}
              warehouses={warehouseList}
              value={selectedWarehouse}
              onChange={setWarehouseID}
            />
            <Field
              label={reason === 'other' ? 'Descripción del motivo' : 'Detalle del motivo'}
              hint={
                reason === 'other'
                  ? 'Obligatoria con «Otro»: de 3 a 200 caracteres.'
                  : 'Opcional; hasta 200 caracteres.'
              }
            >
              <input
                maxLength={200}
                minLength={reason === 'other' ? 3 : undefined}
                required={reason === 'other'}
                value={detail}
                onChange={(event) => setDetail(event.target.value)}
              />
            </Field>
          </div>
          {!waste && allowed !== 'both' && (
            <p className="muted text-xs">
              «{adjustmentReasonLabels[reason as AdjustmentReason]}» solo registra{' '}
              {allowed === 'out' ? 'faltantes (salidas)' : 'sobrantes (entradas)'}.
            </p>
          )}
          <ControlLinesEditor
            lines={lines}
            units={unitList}
            direction={
              waste
                ? null
                : { forced: allowed === 'both' ? null : allowed, labels: directionLabels }
            }
            fixedHint="La merma solo registra salidas: se descuenta del almacén."
            onChange={setLines}
          />
          <Field label="Observaciones" hint="Opcional; hasta 500 caracteres.">
            <textarea
              rows={2}
              maxLength={500}
              value={notes}
              onChange={(event) => setNotes(event.target.value)}
            />
          </Field>
        </fieldset>
        {!waste && !canAuthorize && (
          <p className="muted text-xs">
            Sin el permiso «{inventoryPermissionName('inventory.adjustments.authorize')}» el ajuste
            queda pendiente hasta que otra persona lo autorice.
          </p>
        )}
        <div className="form-actions">
          <button
            type="submit"
            className="primary-button"
            disabled={locked || selectedWarehouse === ''}
          >
            {sending ? 'Registrando…' : title}
          </button>
          <span className="muted text-xs">{countLabel(lines.length, 'renglón', 'renglones')}</span>
        </div>
      </form>
      {dialog}
    </section>
  );
}

function AdjustmentFacts({ adjustment, timezone }: { adjustment: Adjustment; timezone: string }) {
  const facts: { label: string; value: string }[] = [
    { label: 'Tipo', value: adjustment.kind_label },
    {
      label: 'Motivo',
      value: adjustment.reason_detail
        ? `${adjustment.reason_label} · ${adjustment.reason_detail}`
        : adjustment.reason_label,
    },
    { label: 'Almacén', value: adjustment.warehouse.name },
    {
      label: 'Registró',
      value: `${adjustment.requested_by_name} · ${serverDateTime(adjustment.requested_at, timezone)}`,
    },
  ];
  if (adjustment.authorized_by_name && adjustment.authorized_at) {
    facts.push({
      label: 'Autorizó',
      value: `${adjustment.authorized_by_name} · ${serverDateTime(adjustment.authorized_at, timezone)}`,
    });
  }
  if (adjustment.applied_at) {
    facts.push({
      label: 'Aplicado',
      value: adjustment.authorized_by_name
        ? serverDateTime(adjustment.applied_at, timezone)
        : `${serverDateTime(adjustment.applied_at, timezone)} · al registrarse`,
    });
  }
  if (adjustment.rejected_by_name && adjustment.rejected_at) {
    facts.push({
      label: 'Rechazó',
      value: `${adjustment.rejected_by_name} · ${serverDateTime(adjustment.rejected_at, timezone)}`,
    });
  }
  if (adjustment.rejection_reason) {
    facts.push({ label: 'Motivo del rechazo', value: adjustment.rejection_reason });
  }
  if (adjustment.costs_visible && adjustment.value_display !== null) {
    facts.push({ label: 'Valor neto', value: formatSignedMoney(adjustment.value_display) });
  }
  return (
    <dl className="inventory-facts">
      {facts.map((fact) => (
        <div key={fact.label}>
          <dt>{fact.label}</dt>
          <dd>{fact.value}</dd>
        </div>
      ))}
    </dl>
  );
}

function AdjustmentLinesTable({ adjustment }: { adjustment: Adjustment }) {
  const pending = adjustment.state === 'pending_authorization';
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <caption className="sr-only">
          {adjustment.kind === 'waste' ? 'Renglones de la merma' : 'Renglones del ajuste'}{' '}
          {adjustment.folio}
        </caption>
        <thead>
          <tr>
            <th scope="col">Insumo</th>
            <th scope="col">Capturado</th>
            <th scope="col" className="inventory-number">
              En unidad base
            </th>
            <th scope="col">Movimiento</th>
            <th scope="col" className="inventory-number">
              Existencia después
            </th>
            {adjustment.costs_visible && (
              <th scope="col" className="inventory-number">
                Valor
              </th>
            )}
          </tr>
        </thead>
        <tbody>
          {adjustment.lines.map((line) => (
            <tr key={line.line_no}>
              <td className="inventory-cell-name">
                <strong>{line.item.name}</strong>
                <span className="muted text-xs inventory-cell-detail">
                  {line.item.code} · {line.direction === 'out' ? 'Salida' : 'Entrada'}
                </span>
              </td>
              <td>
                {capturedText(
                  line.captured_quantity,
                  line.presentation,
                  line.unit,
                  line.item.unit,
                  true,
                )}
              </td>
              <td className="inventory-number">
                {formatSignedQuantity(line.quantity, line.item.unit)}
              </td>
              <td>
                {line.movement_label ?? (pending ? 'Pendiente de autorización' : 'Sin movimiento')}
              </td>
              <td className="inventory-number">
                {line.stock_after === null ? '—' : formatQuantity(line.stock_after, line.item.unit)}
              </td>
              {adjustment.costs_visible && (
                <td className="inventory-number">{formatSignedMoney(line.value_display)}</td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// Autorizar o rechazar un ajuste pendiente (inventory.adjustments.authorize). Go entrega
// authorizable solo si la sesión tiene el permiso y no registró el ajuste: quien lo registró nunca
// ve los botones (Go además responde SELF_AUTHORIZATION_FORBIDDEN).
function AdjustmentDecision({
  adjustment,
  execute,
  busy,
  onChanged,
  onNavigate,
}: {
  adjustment: Adjustment;
  execute: InventoryExecute;
  busy: boolean;
  onChanged: (adjustment: Adjustment) => void;
  onNavigate?: InventoryNavigate;
}) {
  const [rejecting, setRejecting] = useState(false);
  const [sending, setSending] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const folio = adjustmentFolio(adjustment.kind, adjustment.folio);

  if (!adjustment.authorizable) {
    return (
      <Notice>
        Pendiente de autorización: no modifica existencias hasta que otra persona con el permiso «
        {inventoryPermissionName('inventory.adjustments.authorize')}» lo autorice. Quien lo registró
        no puede autorizarlo ni rechazarlo.
      </Notice>
    );
  }

  async function authorize() {
    const confirmed = await confirm({
      title: `Autorizar el ${folio.toLowerCase()}`,
      message: `Se ${pluralWord(adjustment.line_count, 'aplicará su renglón', 'aplicarán sus renglones')} en «${adjustment.warehouse.name}» con la hora de la autorización. Los movimientos del libro no se editan ni se borran.`,
      confirmLabel: 'Autorizar y aplicar',
      cancelLabel: 'Revisar',
    });
    if (!confirmed) {
      return;
    }
    setSending(true);
    try {
      const saved = await authorizeAdjustment(execute, adjustment.id, {
        revision: adjustment.revision,
      });
      notifySuccess(`${folio} autorizado y aplicado.`);
      for (const warning of saved.warnings) {
        notifyWarning(warning.message);
      }
      onChanged(saved);
    } catch (problem) {
      notifyError(
        problem,
        'No se pudo autorizar el ajuste.',
        costRequiredOptions(problem, onNavigate),
      );
    } finally {
      setSending(false);
    }
  }

  if (rejecting) {
    return (
      <ReasonAction
        title={`Rechazar el ${folio.toLowerCase()}`}
        description="El ajuste rechazado no modifica existencias y queda en el historial con el motivo."
        reasonLabel="Motivo del rechazo"
        actionLabel="Rechazar ajuste"
        confirmTitle={`Rechazar el ${folio.toLowerCase()}`}
        confirmMessage={(reason) => `Se rechazará con el motivo «${reason}». No se puede deshacer.`}
        busy={busy}
        onCancel={() => setRejecting(false)}
        onConfirm={async (reason) => {
          try {
            const saved = await rejectAdjustment(execute, adjustment.id, {
              revision: adjustment.revision,
              reason,
            });
            notifySuccess(`${folio} rechazado: no modificó existencias.`);
            setRejecting(false);
            onChanged(saved);
            return true;
          } catch (problem) {
            notifyError(problem, 'No se pudo rechazar el ajuste.');
            return false;
          }
        }}
      />
    );
  }

  return (
    <div className="button-row control-actions">
      <button
        type="button"
        className="primary-button"
        disabled={busy || sending}
        onClick={() => void authorize()}
      >
        {sending ? 'Autorizando…' : 'Autorizar y aplicar'}
      </button>
      <button
        type="button"
        className="secondary-button"
        disabled={busy || sending}
        onClick={() => setRejecting(true)}
      >
        Rechazar
      </button>
      <span className="muted text-xs">
        Registrado por otra persona: puedes autorizarlo o rechazarlo con motivo.
      </span>
      {dialog}
    </div>
  );
}

// Detalle de un ajuste o merma: datos, renglones con su movimiento y existencia después, avisos de
// Go y, si está pendiente, autorización o rechazo.
function AdjustmentPanel({
  adjustmentID,
  initial,
  execute,
  busy,
  timezone,
  onClose,
  onNavigate,
}: {
  adjustmentID: string;
  initial: Adjustment | null;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onClose: () => void;
  onNavigate?: InventoryNavigate;
}) {
  const [snapshot, setSnapshot] = useState<Adjustment | null>(initial);
  // Los avisos (existencia negativa) llegan en la respuesta de la escritura que los produjo; se
  // conservan mientras el panel siga abierto aunque la consulta posterior ya no los traiga.
  const [writeWarnings, setWriteWarnings] = useState(initial?.warnings ?? []);
  const resource = useResource<Adjustment>('inventory_adjustment', {
    id: adjustmentID,
    reloadEvent: inventoryChangeEvent,
  });

  useEffect(() => {
    if (resource.data) {
      setSnapshot(resource.data);
    }
  }, [resource.data]);

  function changed(saved: Adjustment) {
    setSnapshot(saved);
    setWriteWarnings(saved.warnings);
  }

  const adjustment = snapshot?.id === adjustmentID ? snapshot : null;
  const warnings =
    adjustment && adjustment.warnings.length > 0 ? adjustment.warnings : writeWarnings;
  const heading = adjustment ? adjustmentFolio(adjustment.kind, adjustment.folio) : 'Ajuste';

  return (
    <section className="panel form-stack" aria-label={heading}>
      <div className="section-heading">
        <div>
          <h2>{adjustment ? `${heading} · ${adjustment.warehouse.name}` : heading}</h2>
          {adjustment && (
            <p className="muted text-xs">
              <span className={adjustmentStateTone(adjustment.state)}>
                {adjustment.state_label}
              </span>
              {` · ${countLabel(adjustment.line_count, 'renglón', 'renglones')}`}
            </p>
          )}
        </div>
        <button type="button" className="secondary-button" onClick={onClose}>
          Volver al listado
        </button>
      </div>
      {!adjustment && <Notice>{resource.error || 'Consultando el ajuste…'}</Notice>}
      {adjustment && (
        <>
          <WarningList warnings={warnings} />
          <AdjustmentFacts adjustment={adjustment} timezone={timezone} />
          {adjustment.state === 'pending_authorization' && (
            <AdjustmentDecision
              key={adjustment.revision}
              adjustment={adjustment}
              execute={execute}
              busy={busy}
              onChanged={changed}
              onNavigate={onNavigate}
            />
          )}
          <AdjustmentLinesTable adjustment={adjustment} />
          {adjustment.notes && (
            <p className="text-sm">
              <strong>Observaciones: </strong>
              {adjustment.notes}
            </p>
          )}
        </>
      )}
    </section>
  );
}

type RecordsMode =
  | { kind: 'list' }
  | { kind: 'new'; adjustmentKind: AdjustmentKind }
  | { kind: 'open'; id: string; initial: Adjustment | null };

// Ajustes de discrepancia y mermas operativas: alta de ajustes (inventory.adjustments.record) y
// mermas (inventory.waste.record), listado con filtros por tipo, estado, motivo, almacén y fechas
// (con los permisos de lectura de Go) y autorización o rechazo de pendientes
// (inventory.adjustments.authorize, nunca el propio). Ninguna existencia se edita: cada renglón es
// un movimiento del libro. «before» y «after» son las secciones de cancelaciones de la pestaña,
// que se muestran junto al listado y se ocultan mientras se captura o se consulta un documento.
export function AdjustmentRecords({
  can,
  execute,
  busy,
  timezone,
  onNavigate,
  before,
  after,
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onNavigate?: InventoryNavigate;
  before?: ReactNode;
  after?: ReactNode;
}) {
  const [mode, setMode] = useState<RecordsMode>({ kind: 'list' });
  const [filter, setFilter] = useState<AdjustmentFilter>({
    kind: '',
    state: '',
    reason: '',
    warehouse: '',
    from: '',
    until: '',
    search: '',
    page: 1,
  });
  const readable = canReadAdjustments(can);
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const list = useResource<AdjustmentPage>('inventory_adjustments', {
    query: adjustmentQuery(filter),
    enabled: readable && mode.kind === 'list',
    reloadEvent: inventoryChangeEvent,
  });
  const canRecord = can('inventory.adjustments.record');
  const canWaste = can('inventory.waste.record');

  if (mode.kind === 'new') {
    return (
      <AdjustmentForm
        key={mode.adjustmentKind}
        kind={mode.adjustmentKind}
        canAuthorize={can('inventory.adjustments.authorize')}
        execute={execute}
        busy={busy}
        onRecorded={(saved) => setMode({ kind: 'open', id: saved.id, initial: saved })}
        onCancel={() => setMode({ kind: 'list' })}
        onNavigate={onNavigate}
      />
    );
  }
  if (mode.kind === 'open') {
    return (
      <AdjustmentPanel
        key={mode.id}
        adjustmentID={mode.id}
        initial={mode.initial}
        execute={execute}
        busy={busy}
        timezone={timezone}
        onClose={() => setMode({ kind: 'list' })}
        onNavigate={onNavigate}
      />
    );
  }

  return (
    <div className="form-stack">
      <div className="section-heading">
        <p className="muted text-sm">
          Ajustes por diferencias con lo físico, mermas operativas y cancelaciones por clasificar.
          No se edita la existencia: cada renglón registra un movimiento; una existencia negativa se
          avisa, no se bloquea.
        </p>
        {(canRecord || canWaste) && (
          <div className="button-row">
            {canRecord && (
              <button
                type="button"
                className="primary-button"
                onClick={() => setMode({ kind: 'new', adjustmentKind: 'discrepancy' })}
              >
                <IconLabel icon="plus">Registrar ajuste</IconLabel>
              </button>
            )}
            {canWaste && (
              <button
                type="button"
                className={canRecord ? 'secondary-button' : 'primary-button'}
                onClick={() => setMode({ kind: 'new', adjustmentKind: 'waste' })}
              >
                <IconLabel icon="trash">Registrar merma</IconLabel>
              </button>
            )}
          </div>
        )}
      </div>
      {before}
      {readable && (
        <section className="panel form-stack" aria-labelledby="inventory-adjustments-title">
          <div>
            <h2 id="inventory-adjustments-title">Ajustes y mermas registrados</h2>
            <p className="muted text-xs">
              Del más reciente al más antiguo. Un ajuste pendiente de autorización no modifica
              existencias hasta que otra persona lo autoriza.
            </p>
          </div>
          <AdjustmentFilters
            filter={filter}
            warehouses={warehouses.data ?? []}
            onChange={setFilter}
          />
          <Notice>{list.error}</Notice>
          {!list.data && !list.error && <p className="muted">Consultando ajustes y mermas…</p>}
          {list.data && (
            <>
              <AdjustmentsTable
                page={list.data}
                timezone={timezone}
                onOpen={(adjustment) => setMode({ kind: 'open', id: adjustment.id, initial: null })}
              />
              <Pagination
                page={list.data.page}
                pageSize={list.data.page_size}
                total={list.data.total}
                singular="documento"
                plural="documentos"
                onPage={(page) => setFilter({ ...filter, page })}
              />
            </>
          )}
        </section>
      )}
      {after}
    </div>
  );
}
