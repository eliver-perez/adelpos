import { useEffect, useState } from 'react';
import type { FormEvent } from 'react';
import { OptionalDateField } from '../../components/catalog/DateField';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, InventoryUnit, Warehouse } from '../../services/inventory';
import { recordTransfer } from '../../services/inventoryControl';
import type {
  Transfer,
  TransferInput,
  TransferPage,
  TransferSummary,
} from '../../services/inventoryControl';
import { formatMoney } from '../../services/money';
import { notifyError, notifySuccess, notifyWarning } from '../../services/notify';
import { acceptIntegerText } from '../../services/numericInput';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { capturedText, controlLineInput, emptyControlLine, transferFolio } from './controlFormat';
import type { ControlLineDraft } from './controlFormat';
import { ControlLinesEditor } from './ControlLinesEditor';
import { defaultWarehouseID, WarehouseSelect, WarningList } from './ControlShared';
import { Pagination } from './InventoryFields';
import { formatQuantity, inventoryPermissionName } from './inventoryFormat';
import { measureName } from './PurchaseFields';
import { Select } from '../../components/Select';

const pageSize = 25;

interface TransferFilter {
  warehouse: string;
  from: string;
  until: string;
  search: string;
  page: number;
}

function transferQuery(filter: TransferFilter) {
  const query: Record<string, string> = {
    page: String(filter.page),
    page_size: String(pageSize),
  };
  if (filter.warehouse !== '') {
    query.warehouse_id = filter.warehouse;
  }
  if (filter.from !== '') {
    query.from = filter.from;
  }
  if (filter.until !== '') {
    query.until = filter.until;
  }
  if (filter.search.trim() !== '') {
    query.search = filter.search.trim();
  }
  return query;
}

function TransferFilters({
  filter,
  warehouses,
  onChange,
}: {
  filter: TransferFilter;
  warehouses: Warehouse[];
  onChange: (filter: TransferFilter) => void;
}) {
  return (
    <section className="panel catalog-filters" aria-label="Filtros de transferencias">
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
      <Field label="Almacén de origen o destino">
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
        label="Desde"
        value={filter.from}
        onChange={(from) => onChange({ ...filter, from, page: 1 })}
      />
      <OptionalDateField
        label="Hasta"
        value={filter.until}
        onChange={(until) => onChange({ ...filter, until, page: 1 })}
      />
    </section>
  );
}

function TransfersTable({
  page,
  timezone,
  onOpen,
}: {
  page: TransferPage;
  timezone: string;
  onOpen: (transfer: TransferSummary) => void;
}) {
  const columns = 5 + (page.costs_visible ? 1 : 0);
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <thead>
          <tr>
            <th scope="col">Transferencia</th>
            <th scope="col">Fecha</th>
            <th scope="col">De → a</th>
            <th scope="col">Renglones</th>
            {page.costs_visible && (
              <th scope="col" className="inventory-number">
                Valor trasladado
              </th>
            )}
            <th scope="col">
              <span className="sr-only">Acciones</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {page.items.map((transfer) => (
            <tr key={transfer.id}>
              <td className="inventory-cell-name">
                <strong>{transferFolio(transfer.folio)}</strong>
                <span className="muted text-xs inventory-cell-detail">{transfer.actor_name}</span>
              </td>
              <td>{serverDateTime(transfer.occurred_at, timezone)}</td>
              <td>
                {transfer.from_warehouse.name} → {transfer.to_warehouse.name}
              </td>
              <td>{countLabel(transfer.line_count, 'renglón', 'renglones')}</td>
              {page.costs_visible && (
                <td className="inventory-number">{formatMoney(transfer.value_display)}</td>
              )}
              <td>
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`Abrir la ${transferFolio(transfer.folio).toLowerCase()}`}
                  onClick={() => onOpen(transfer)}
                >
                  Abrir
                </button>
              </td>
            </tr>
          ))}
          {page.items.length === 0 && (
            <tr>
              <td colSpan={columns} className="muted">
                No hay transferencias con estos filtros.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

// Alta de una transferencia (inventory.transfers.record): origen y destino distintos y renglones
// con la cantidad sin signo. Se confirma antes de enviar. Go registra en una sola transacción la
// salida y la entrada vinculadas con el mismo valor, sin cambiar el costo del insumo; una
// existencia negativa en el origen se avisa, no se bloquea.
function TransferForm({
  execute,
  busy,
  onRecorded,
  onCancel,
}: {
  execute: InventoryExecute;
  busy: boolean;
  onRecorded: (transfer: Transfer) => void;
  onCancel: () => void;
}) {
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const units = useResource<InventoryUnit[]>('inventory_units', {
    reloadEvent: inventoryChangeEvent,
  });
  const [fromID, setFromID] = useState('');
  const [toID, setToID] = useState('');
  const [notes, setNotes] = useState('');
  const [lines, setLines] = useState<ControlLineDraft[]>([emptyControlLine(1)]);
  const [sending, setSending] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const warehouseList = warehouses.data ?? [];
  const unitList = units.data ?? [];
  const origin = fromID || defaultWarehouseID(warehouseList);
  const sameWarehouse = origin !== '' && origin === toID;
  const locked = busy || sending;
  const activeCount = warehouseList.filter((warehouse) => warehouse.active).length;

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (toID === '' || sameWarehouse) {
      return;
    }
    const from = warehouseList.find((warehouse) => warehouse.id === origin);
    const to = warehouseList.find((warehouse) => warehouse.id === toID);
    const confirmed = await confirm({
      title: 'Registrar transferencia',
      message: (
        <>
          <p>
            De «{from?.name ?? ''}» a «{to?.name ?? ''}»,{' '}
            {countLabel(lines.length, 'renglón', 'renglones')}:
          </p>
          <ul>
            {lines.map((line) => (
              <li key={line.key}>
                {line.item?.name ?? 'Insumo sin elegir'}: {line.quantity}{' '}
                {measureName(line.item, line.measure, unitList)}
              </li>
            ))}
          </ul>
          <p>
            Se registran la salida y la entrada vinculadas con el mismo costo. No crea compras ni
            ventas y una existencia negativa en el origen no bloquea el traslado.
          </p>
        </>
      ),
      confirmLabel: 'Registrar transferencia',
      cancelLabel: 'Revisar',
    });
    if (!confirmed) {
      return;
    }
    const input: TransferInput = {
      from_warehouse_id: origin,
      to_warehouse_id: toID,
      lines: lines.map((line) => controlLineInput(line, false)),
    };
    if (notes.trim() !== '') {
      input.notes = notes.trim();
    }
    setSending(true);
    try {
      const saved = await recordTransfer(execute, input);
      notifySuccess(
        `${transferFolio(saved.folio)} registrada: de «${saved.from_warehouse.name}» a «${saved.to_warehouse.name}».`,
      );
      for (const warning of saved.warnings) {
        notifyWarning(warning.message);
      }
      onRecorded(saved);
    } catch (problem) {
      notifyError(problem, 'No se pudo registrar la transferencia.');
    } finally {
      setSending(false);
    }
  }

  return (
    <section className="panel form-stack" aria-label="Nueva transferencia">
      <div className="section-heading">
        <div>
          <h2>Nueva transferencia</h2>
          <p className="muted text-xs">
            Traslado de mercancía entre almacenes con salida y entrada vinculadas.
          </p>
        </div>
        <button type="button" className="secondary-button" onClick={onCancel}>
          Volver al listado
        </button>
      </div>
      <Notice>{warehouses.error || units.error}</Notice>
      {warehouses.data && activeCount < 2 && (
        <Notice>
          Se necesitan dos almacenes activos para transferir. Agrégalos en la pestaña Almacenes.
        </Notice>
      )}
      <form className="form-stack" onSubmit={(event) => void submit(event)}>
        <fieldset className="form-stack" disabled={locked}>
          <div className="form-grid">
            <WarehouseSelect
              label="Almacén de origen"
              warehouses={warehouseList}
              value={origin}
              onChange={(value) => {
                setFromID(value);
                if (value === toID) {
                  setToID('');
                }
              }}
              hint="De donde sale la mercancía."
            />
            <WarehouseSelect
              label="Almacén de destino"
              warehouses={warehouseList}
              value={toID}
              exclude={origin}
              emptyLabel="Elige el destino"
              onChange={setToID}
              hint="Distinto del origen."
            />
          </div>
          {sameWarehouse && (
            <p className="inventory-negative-text text-xs" role="alert">
              El origen y el destino deben ser almacenes distintos.
            </p>
          )}
          <ControlLinesEditor
            lines={lines}
            units={unitList}
            direction={null}
            fixedHint="Sale del origen y entra al destino con el mismo costo."
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
        <div className="form-actions">
          <button
            type="submit"
            className="primary-button"
            disabled={locked || origin === '' || toID === '' || sameWarehouse}
          >
            {sending ? 'Registrando…' : 'Registrar transferencia'}
          </button>
          <span className="muted text-xs">{countLabel(lines.length, 'renglón', 'renglones')}</span>
        </div>
      </form>
      {dialog}
    </section>
  );
}

function TransferLinesTable({ transfer }: { transfer: Transfer }) {
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <caption className="sr-only">
          Renglones de la {transferFolio(transfer.folio).toLowerCase()}
        </caption>
        <thead>
          <tr>
            <th scope="col">Insumo</th>
            <th scope="col">Capturado</th>
            <th scope="col" className="inventory-number">
              En unidad base
            </th>
            <th scope="col" className="inventory-number">
              {`En ${transfer.from_warehouse.name} después`}
            </th>
            <th scope="col" className="inventory-number">
              {`En ${transfer.to_warehouse.name} después`}
            </th>
            {transfer.costs_visible && (
              <th scope="col" className="inventory-number">
                Valor
              </th>
            )}
          </tr>
        </thead>
        <tbody>
          {transfer.lines.map((line) => (
            <tr key={line.line_no}>
              <td className="inventory-cell-name">
                <strong>{line.item.name}</strong>
                <span className="muted text-xs inventory-cell-detail">{line.item.code}</span>
              </td>
              <td>
                {capturedText(line.captured_quantity, line.presentation, line.unit, line.item.unit)}
              </td>
              <td className="inventory-number">{formatQuantity(line.quantity, line.item.unit)}</td>
              <td
                className={`inventory-number ${line.from_stock_after.startsWith('-') ? 'inventory-negative-text' : ''}`}
              >
                {formatQuantity(line.from_stock_after, line.item.unit)}
              </td>
              <td className="inventory-number">
                {formatQuantity(line.to_stock_after, line.item.unit)}
              </td>
              {transfer.costs_visible && (
                <td className="inventory-number">{formatMoney(line.value_display)}</td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// Detalle de una transferencia: origen, destino, responsable y renglones con las existencias de
// ambos almacenes después del traslado, tal como las calculó Go.
function TransferPanel({
  transferID,
  initial,
  timezone,
  onClose,
}: {
  transferID: string;
  initial: Transfer | null;
  timezone: string;
  onClose: () => void;
}) {
  const [snapshot, setSnapshot] = useState<Transfer | null>(initial);
  const writeWarnings = initial?.warnings ?? [];
  const resource = useResource<Transfer>('inventory_transfer', {
    id: transferID,
    reloadEvent: inventoryChangeEvent,
  });

  useEffect(() => {
    if (resource.data) {
      setSnapshot(resource.data);
    }
  }, [resource.data]);

  const transfer = snapshot?.id === transferID ? snapshot : null;
  const heading = transfer ? transferFolio(transfer.folio) : 'Transferencia';
  const warnings = transfer && transfer.warnings.length > 0 ? transfer.warnings : writeWarnings;
  return (
    <section className="panel form-stack" aria-label={heading}>
      <div className="section-heading">
        <div>
          <h2>
            {transfer
              ? `${heading} · ${transfer.from_warehouse.name} → ${transfer.to_warehouse.name}`
              : heading}
          </h2>
          {transfer && (
            <p className="muted text-xs">
              {serverDateTime(transfer.occurred_at, timezone)} · {transfer.actor_name} ·{' '}
              {countLabel(transfer.line_count, 'renglón', 'renglones')}
            </p>
          )}
        </div>
        <button type="button" className="secondary-button" onClick={onClose}>
          Volver al listado
        </button>
      </div>
      {!transfer && <Notice>{resource.error || 'Consultando la transferencia…'}</Notice>}
      {transfer && (
        <>
          <WarningList warnings={warnings} />
          <dl className="inventory-facts">
            <div>
              <dt>Origen</dt>
              <dd>{transfer.from_warehouse.name}</dd>
            </div>
            <div>
              <dt>Destino</dt>
              <dd>{transfer.to_warehouse.name}</dd>
            </div>
            <div>
              <dt>Registró</dt>
              <dd>{transfer.actor_name}</dd>
            </div>
            {transfer.costs_visible && transfer.value_display !== null && (
              <div>
                <dt>Valor trasladado</dt>
                <dd>{formatMoney(transfer.value_display)}</dd>
              </div>
            )}
          </dl>
          <TransferLinesTable transfer={transfer} />
          {transfer.notes && (
            <p className="text-sm">
              <strong>Observaciones: </strong>
              {transfer.notes}
            </p>
          )}
        </>
      )}
    </section>
  );
}

type TransferMode =
  { kind: 'list' } | { kind: 'new' } | { kind: 'open'; id: string; initial: Transfer | null };

// Pestaña «Transferencias» (inventory.transfers.record o inventory.movements.read): listado con
// filtros por almacén, fechas y folio; alta con inventory.transfers.record y detalle de cada
// traslado. Las transferencias no se editan ni se borran: un error se corrige con otra en
// sentido contrario.
export function TransfersView({
  can,
  execute,
  busy,
  timezone,
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
}) {
  const [mode, setMode] = useState<TransferMode>({ kind: 'list' });
  const [filter, setFilter] = useState<TransferFilter>({
    warehouse: '',
    from: '',
    until: '',
    search: '',
    page: 1,
  });
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const list = useResource<TransferPage>('inventory_transfers', {
    query: transferQuery(filter),
    enabled: mode.kind === 'list',
    reloadEvent: inventoryChangeEvent,
  });
  const recordable = can('inventory.transfers.record');

  if (mode.kind === 'new') {
    return (
      <TransferForm
        execute={execute}
        busy={busy}
        onRecorded={(saved) => setMode({ kind: 'open', id: saved.id, initial: saved })}
        onCancel={() => setMode({ kind: 'list' })}
      />
    );
  }
  if (mode.kind === 'open') {
    return (
      <TransferPanel
        key={mode.id}
        transferID={mode.id}
        initial={mode.initial}
        timezone={timezone}
        onClose={() => setMode({ kind: 'list' })}
      />
    );
  }

  return (
    <div className="form-stack">
      <div className="section-heading">
        <p className="muted text-sm">
          Traslados entre almacenes: cada renglón registra una salida del origen y una entrada al
          destino con el mismo valor. No crean compras ni ventas.
        </p>
        {recordable && (
          <button type="button" className="primary-button" onClick={() => setMode({ kind: 'new' })}>
            <IconLabel icon="transfer">Nueva transferencia</IconLabel>
          </button>
        )}
      </div>
      {!recordable && (
        <Notice>
          Consulta disponible. Transferir requiere el permiso «
          {inventoryPermissionName('inventory.transfers.record')}».
        </Notice>
      )}
      <TransferFilters filter={filter} warehouses={warehouses.data ?? []} onChange={setFilter} />
      <Notice>{list.error}</Notice>
      {!list.data && !list.error && <p className="muted">Consultando transferencias…</p>}
      {list.data && (
        <section className="panel" aria-label="Transferencias registradas">
          <TransfersTable
            page={list.data}
            timezone={timezone}
            onOpen={(transfer) => setMode({ kind: 'open', id: transfer.id, initial: null })}
          />
          <Pagination
            page={list.data.page}
            pageSize={list.data.page_size}
            total={list.data.total}
            singular="transferencia"
            plural="transferencias"
            onPage={(page) => setFilter({ ...filter, page })}
          />
        </section>
      )}
    </div>
  );
}
