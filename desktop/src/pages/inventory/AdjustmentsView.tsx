import { useState } from 'react';
import { OptionalDateField } from '../../components/catalog/DateField';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Notice } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute } from '../../services/inventory';
import { cancellationStateLabels, classifyCancellation } from '../../services/inventoryRecipes';
import type {
  Cancellation,
  CancellationPage,
  CancellationState,
} from '../../services/inventoryRecipes';
import { notifyError, notifySuccess } from '../../services/notify';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { AdjustmentRecords } from './AdjustmentRecords';
import { ConsumptionLinesTable, ConsumptionTraceView } from './ConsumptionTrace';
import { Pagination } from './InventoryFields';
import { inventoryPermissionName } from './inventoryFormat';
import type { InventoryNavigate } from './inventoryNavigation';
import { cancellationAccount, operatingDateText } from './recipeFormat';

const pageSize = 25;

type ClassifiedState = Exclude<CancellationState, 'pending'>;

const classificationEffects: Record<ClassifiedState, string> = {
  not_prepared: 'Su consumo se revirtió con movimientos compensatorios.',
  prepared_waste: 'El consumo se conserva y queda registrado como merma.',
};

function otherState(state: CancellationState): ClassifiedState {
  return state === 'not_prepared' ? 'prepared_waste' : 'not_prepared';
}

// Quién clasificó: la persona o, en las automáticas, el cierre del día operativo.
function classifiedBy(cancellation: Cancellation) {
  if (cancellation.classified_by_name) {
    return cancellation.classified_by_name;
  }
  if (cancellation.resolution === 'automatic') {
    return 'Por cierre del día operativo';
  }
  return '—';
}

// Clasifica o corrige una cancelación con la revisión vigente y avisa el resultado que devuelve Go.
// Después del corte Go la registra como corrección (inventory.adjustments.authorize).
function useClassification(execute: InventoryExecute) {
  const [saving, setSaving] = useState('');
  async function classify(
    cancellation: Cancellation,
    state: ClassifiedState,
    notes: string | null = null,
  ) {
    setSaving(cancellation.id);
    try {
      const saved = await classifyCancellation(execute, cancellation.id, {
        revision: cancellation.revision,
        state,
        notes: notes && notes.trim() !== '' ? notes : null,
      });
      notifySuccess(`${saved.product_name}: ${saved.state_label}.`, {
        description: classificationEffects[state],
      });
      return true;
    } catch (problem) {
      notifyError(problem, 'No se pudo clasificar la cancelación.');
      return false;
    } finally {
      setSaving('');
    }
  }
  return { classify, saving };
}

function CancellationFacts({
  cancellation,
  timezone,
}: {
  cancellation: Cancellation;
  timezone: string;
}) {
  return (
    <dl className="inventory-facts">
      <div>
        <dt>Cuenta</dt>
        <dd>{cancellationAccount(cancellation)}</dd>
      </div>
      <div>
        <dt>Cancelada</dt>
        <dd>{serverDateTime(cancellation.cancelled_at, timezone)}</dd>
      </div>
      <div>
        <dt>Día operativo</dt>
        <dd>{operatingDateText(cancellation.operating_date)}</dd>
      </div>
      <div>
        <dt>{cancellation.expired ? 'Corte del día operativo' : 'Límite para clasificar'}</dt>
        <dd>{serverDateTime(cancellation.deadline_at, timezone)}</dd>
      </div>
    </dl>
  );
}

function PendingCancellationCard({
  cancellation,
  timezone,
  busy,
  saving,
  canTrace,
  onClassify,
  onShowTrace,
}: {
  cancellation: Cancellation;
  timezone: string;
  busy: boolean;
  saving: boolean;
  canTrace: boolean;
  onClassify: (state: ClassifiedState) => void;
  onShowTrace: () => void;
}) {
  const correction = cancellation.expired;
  return (
    <article
      className="inventory-cancellation"
      aria-label={`Cancelación de ${cancellation.product_name}`}
    >
      <div className="section-heading">
        <div>
          <h3>{cancellation.product_name}</h3>
          {correction && (
            <p className="inventory-negative-text text-xs">
              Pasó el corte del día operativo: solo se corrige con autorización.
            </p>
          )}
        </div>
        {canTrace && (
          <button type="button" className="secondary-button" onClick={onShowTrace}>
            Ver consumo de la partida
          </button>
        )}
      </div>
      <CancellationFacts cancellation={cancellation} timezone={timezone} />
      <ConsumptionLinesTable
        lines={cancellation.lines}
        costsVisible={cancellation.costs_visible}
        caption={`Consumo de ${cancellation.product_name}`}
      />
      {cancellation.can_classify ? (
        <div className="button-row inventory-classify-actions">
          <button
            type="button"
            className="secondary-button"
            disabled={busy || saving}
            onClick={() => onClassify('not_prepared')}
          >
            {correction ? 'Corregir: No preparado' : 'No preparado'}
          </button>
          <button
            type="button"
            className="secondary-button"
            disabled={busy || saving}
            onClick={() => onClassify('prepared_waste')}
          >
            {correction ? 'Corregir: Preparado (merma)' : 'Preparado (merma)'}
          </button>
          <span className="muted text-xs">
            «No preparado» revierte su consumo; «Preparado (merma)» lo conserva como merma.
          </span>
        </div>
      ) : (
        <p className="muted text-xs">
          Clasificarla requiere el permiso «
          {inventoryPermissionName(cancellation.required_permission)}».
        </p>
      )}
    </article>
  );
}

// Partidas canceladas después de liberarse a cocina que esperan su clasificación. Hasta el corte
// del día operativo las clasifica quien tiene inventory.waste.record; al cierre del día, el
// proceso de Go las considera «No preparado» y revierte su consumo.
function PendingCancellations({
  execute,
  busy,
  timezone,
  canTrace,
  onShowTrace,
}: {
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  canTrace: boolean;
  onShowTrace: (unitID: string) => void;
}) {
  const [page, setPage] = useState(1);
  const { classify, saving } = useClassification(execute);
  const { confirm, dialog } = useConfirmDialog();
  const list = useResource<CancellationPage>('inventory_cancellations', {
    query: { state: 'pending', page: String(page), page_size: String(pageSize) },
    reloadEvent: inventoryChangeEvent,
  });
  const data = list.data;

  async function choose(cancellation: Cancellation, state: ClassifiedState) {
    if (cancellation.expired) {
      const confirmed = await confirm({
        title: 'Corregir una cancelación vencida',
        message: `«${cancellation.product_name}» pasó el corte del día operativo. Se registrará como «${cancellationStateLabels[state]}» en una corrección autorizada: ${classificationEffects[state]}`,
        confirmLabel: 'Registrar corrección',
        cancelLabel: 'Revisar',
        tone: 'danger',
      });
      if (!confirmed) {
        return;
      }
    }
    await classify(cancellation, state);
  }

  return (
    <section className="panel form-stack" aria-labelledby="inventory-pending-cancellations">
      <div className="section-heading inventory-pending-heading">
        <div>
          <h2 id="inventory-pending-cancellations">Cancelaciones por clasificar</h2>
          <p className="muted text-xs">
            Partidas canceladas después de liberarse a cocina. Al cierre del día operativo las que
            sigan pendientes se consideran «No preparado» y su consumo se revierte.
          </p>
        </div>
        {data && (
          <span className={data.pending > 0 ? 'tag tag-warning' : 'tag tag-good'}>
            {countLabel(data.pending, 'pendiente', 'pendientes')}
          </span>
        )}
      </div>
      <Notice>{list.error}</Notice>
      {!data && !list.error && <p className="muted">Consultando cancelaciones…</p>}
      {data && data.items.length === 0 && (
        <p className="muted">No hay cancelaciones por clasificar.</p>
      )}
      {data &&
        data.items.map((cancellation) => (
          <PendingCancellationCard
            key={`${cancellation.id}:${cancellation.revision}`}
            cancellation={cancellation}
            timezone={timezone}
            busy={busy}
            saving={saving === cancellation.id}
            canTrace={canTrace}
            onClassify={(state) => void choose(cancellation, state)}
            onShowTrace={() => onShowTrace(cancellation.restaurant_unit_id)}
          />
        ))}
      {data && data.total > data.page_size && (
        <Pagination
          page={data.page}
          pageSize={data.page_size}
          total={data.total}
          singular="cancelación"
          plural="cancelaciones"
          onPage={setPage}
        />
      )}
      {dialog}
    </section>
  );
}

// Cambio de una clasificación ya registrada: antes del corte, con inventory.waste.record; después
// (o tras la automática), corrección con inventory.adjustments.authorize y confirmación de peligro.
function ClassificationChange({
  cancellation,
  execute,
  busy,
  onDone,
}: {
  cancellation: Cancellation;
  execute: InventoryExecute;
  busy: boolean;
  onDone: () => void;
}) {
  const [notes, setNotes] = useState('');
  const { classify, saving } = useClassification(execute);
  const { confirm, dialog } = useConfirmDialog();
  const target = otherState(cancellation.state);
  const correction = cancellation.expired;

  async function apply() {
    const confirmed = await confirm({
      title: correction ? 'Corregir la clasificación' : 'Cambiar la clasificación',
      message: `«${cancellation.product_name}» pasará de «${cancellation.state_label}» a «${cancellationStateLabels[target]}». ${classificationEffects[target]}${correction ? ' Se registrará como corrección autorizada.' : ''}`,
      confirmLabel: correction ? 'Registrar corrección' : 'Cambiar clasificación',
      cancelLabel: 'Revisar',
      tone: correction ? 'danger' : 'default',
    });
    if (!confirmed) {
      return;
    }
    if (await classify(cancellation, target, notes)) {
      onDone();
    }
  }

  return (
    <section
      className="inventory-correction form-stack"
      aria-label={`${correction ? 'Corrección' : 'Cambio'} de la clasificación de ${cancellation.product_name}`}
    >
      <p className="text-sm">
        <strong>{cancellation.product_name}</strong> · {cancellationAccount(cancellation)}: de «
        {cancellation.state_label}» a «{cancellationStateLabels[target]}».
      </p>
      <Field
        label="Motivo del cambio"
        hint="Opcional; hasta 500 caracteres. Queda en la auditoría."
      >
        <input maxLength={500} value={notes} onChange={(event) => setNotes(event.target.value)} />
      </Field>
      <div className="button-row">
        <button
          type="button"
          className={correction ? 'danger-button' : 'primary-button'}
          disabled={busy || saving !== ''}
          onClick={() => void apply()}
        >
          {correction ? 'Aplicar corrección' : 'Cambiar clasificación'}
        </button>
        <button type="button" className="secondary-button" onClick={onDone}>
          Cancelar
        </button>
      </div>
      {dialog}
    </section>
  );
}

interface HistoryFilter {
  state: CancellationState | '';
  from: string;
  until: string;
  page: number;
}

function historyQuery(filter: HistoryFilter) {
  const query: Record<string, string> = {
    page: String(filter.page),
    page_size: String(pageSize),
  };
  if (filter.state !== '') {
    query.state = filter.state;
  }
  if (filter.from !== '') {
    query.from = filter.from;
  }
  if (filter.until !== '') {
    query.until = filter.until;
  }
  return query;
}

// Historial de cancelaciones clasificadas, incluidas las automáticas del cierre del día operativo.
function CancellationHistory({
  execute,
  busy,
  timezone,
}: {
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
}) {
  const [filter, setFilter] = useState<HistoryFilter>({ state: '', from: '', until: '', page: 1 });
  const [changing, setChanging] = useState<Cancellation | null>(null);
  const list = useResource<CancellationPage>('inventory_cancellations', {
    query: historyQuery(filter),
    reloadEvent: inventoryChangeEvent,
  });
  const data = list.data;
  return (
    <section className="panel form-stack" aria-labelledby="inventory-cancellation-history">
      <div>
        <h2 id="inventory-cancellation-history">Historial de clasificaciones</h2>
        <p className="muted text-xs">
          Clasificaciones manuales, automáticas al cierre del día operativo y correcciones
          autorizadas. Después del corte solo se corrigen con el permiso «
          {inventoryPermissionName('inventory.adjustments.authorize')}».
        </p>
      </div>
      <div className="catalog-filters inventory-inline-filters" aria-label="Filtros del historial">
        <Field label="Clasificación">
          <select
            value={filter.state}
            onChange={(event) =>
              setFilter({
                ...filter,
                state: event.target.value as CancellationState | '',
                page: 1,
              })
            }
          >
            <option value="">Todas</option>
            <option value="not_prepared">No preparado</option>
            <option value="prepared_waste">Preparado / merma</option>
            <option value="pending">Pendientes de clasificar</option>
          </select>
        </Field>
        <OptionalDateField
          label="Día operativo desde"
          value={filter.from}
          onChange={(from) => setFilter({ ...filter, from, page: 1 })}
        />
        <OptionalDateField
          label="Día operativo hasta"
          value={filter.until}
          onChange={(until) => setFilter({ ...filter, until, page: 1 })}
        />
      </div>
      {changing && (
        <ClassificationChange
          key={`${changing.id}:${changing.revision}`}
          cancellation={changing}
          execute={execute}
          busy={busy}
          onDone={() => setChanging(null)}
        />
      )}
      <Notice>{list.error}</Notice>
      {data && (
        <>
          <div className="table-container cash-table">
            <table className="inventory-table">
              <thead>
                <tr>
                  <th scope="col">Partida</th>
                  <th scope="col">Cancelada</th>
                  <th scope="col">Día operativo</th>
                  <th scope="col">Clasificación</th>
                  <th scope="col">Clasificó</th>
                  <th scope="col">
                    <span className="sr-only">Acciones</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((cancellation) => (
                  <tr key={cancellation.id}>
                    <td className="inventory-cell-name">
                      <strong>{cancellation.product_name}</strong>
                      <span className="muted text-xs inventory-cell-detail">
                        {cancellationAccount(cancellation)}
                      </span>
                    </td>
                    <td>{serverDateTime(cancellation.cancelled_at, timezone)}</td>
                    <td>{operatingDateText(cancellation.operating_date)}</td>
                    <td>
                      <span
                        className={cancellation.state === 'pending' ? 'tag tag-warning' : 'tag'}
                      >
                        {cancellation.state_label}
                      </span>
                      {cancellation.resolution_label && (
                        <span className="muted text-xs inventory-cell-detail">
                          {cancellation.resolution_label}
                        </span>
                      )}
                      {cancellation.notes && (
                        <span className="muted text-xs inventory-cell-detail">
                          {cancellation.notes}
                        </span>
                      )}
                    </td>
                    <td>
                      {classifiedBy(cancellation)}
                      {cancellation.classified_at && (
                        <span className="muted text-xs inventory-cell-detail">
                          {serverDateTime(cancellation.classified_at, timezone)}
                        </span>
                      )}
                    </td>
                    <td>
                      {cancellation.state !== 'pending' && cancellation.can_classify && (
                        <button
                          type="button"
                          className="secondary-button"
                          aria-label={`${cancellation.expired ? 'Corregir' : 'Cambiar'} la clasificación de ${cancellation.product_name}`}
                          disabled={busy}
                          onClick={() => setChanging(cancellation)}
                        >
                          {cancellation.expired ? 'Corregir' : 'Cambiar'}
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
                {data.items.length === 0 && (
                  <tr>
                    <td colSpan={6} className="muted">
                      No hay cancelaciones con estos filtros.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <Pagination
            page={data.page}
            pageSize={data.page_size}
            total={data.total}
            singular="cancelación"
            plural="cancelaciones"
            onPage={(next) => setFilter({ ...filter, page: next })}
          />
        </>
      )}
    </section>
  );
}

// Pestaña «Ajustes y mermas» (inventory.read): alta y listado de ajustes de discrepancia y mermas
// operativas (fase D) y las cancelaciones por clasificar con su historial (fase B), en una sola
// vista: primero las cancelaciones pendientes (vencen al corte del día operativo), después los
// ajustes y mermas registrados y al final el historial de clasificaciones.
export function AdjustmentsView({
  can,
  execute,
  busy,
  timezone,
  onNavigate,
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  // Lleva a otra pestaña (por ejemplo, al insumo sin costo de un sobrante rechazado).
  onNavigate?: InventoryNavigate;
}) {
  const [trace, setTrace] = useState<string | null>(null);
  const canTrace = can('inventory.movements.read');
  if (trace) {
    return (
      <ConsumptionTraceView
        unitID={trace}
        timezone={timezone}
        closeLabel="Volver a ajustes y mermas"
        onClose={() => setTrace(null)}
      />
    );
  }
  return (
    <AdjustmentRecords
      can={can}
      execute={execute}
      busy={busy}
      timezone={timezone}
      onNavigate={onNavigate}
      before={
        <PendingCancellations
          execute={execute}
          busy={busy}
          timezone={timezone}
          canTrace={canTrace}
          onShowTrace={setTrace}
        />
      }
      after={<CancellationHistory execute={execute} busy={busy} timezone={timezone} />}
    />
  );
}
