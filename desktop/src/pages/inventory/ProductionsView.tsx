import { useState } from 'react';
import type { FormEvent } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, Warehouse } from '../../services/inventory';
import { recordProduction } from '../../services/inventoryRecipes';
import type {
  Production,
  ProductionInput,
  ProductionPage,
  Recipe,
  RecipePage,
} from '../../services/inventoryRecipes';
import { formatMoney } from '../../services/money';
import { notifyError, notifySuccess, notifyWarning } from '../../services/notify';
import { inventoryQuantityShape } from '../../services/numericInput';
import { serverDateTime } from '../../services/restaurant';
import { DecimalTextField, Pagination } from './InventoryFields';
import { formatQuantity, inventoryPermissionName } from './inventoryFormat';
import { Select } from '../../components/Select';

const pageSize = 25;

// Cuerpo de la elaboración con las cadenas tal como se capturaron: Go calcula lo esperado
// (lotes × rendimiento efectivo), la merma y el valor; lo producido vacío es lo esperado.
export function productionInput(
  recipeID: string,
  warehouseID: string,
  batches: string,
  produced: string,
  notes: string,
): ProductionInput {
  const input: ProductionInput = { recipe_id: recipeID, warehouse_id: warehouseID, batches };
  if (produced.trim() !== '') {
    input.produced_quantity = produced;
  }
  if (notes.trim() !== '') {
    input.notes = notes;
  }
  return input;
}

// Resultado de una elaboración tal como lo devuelve Go: insumos consumidos, lo producido y la merma.
function ProductionResult({
  production,
  timezone,
  onAnother,
  onClose,
}: {
  production: Production;
  timezone: string;
  onAnother: () => void;
  onClose: () => void;
}) {
  const unit = production.item.unit;
  return (
    <section className="panel form-stack" aria-label="Elaboración registrada">
      <div className="section-heading">
        <div>
          <h2>
            Elaboración registrada · {production.recipe_name} (versión {production.recipe_version})
          </h2>
          <p className="muted text-xs">
            {serverDateTime(production.occurred_at, timezone)} · {production.warehouse.name}
            {production.actor_name ? ` · ${production.actor_name}` : ''}
          </p>
        </div>
        <div className="button-row">
          <button type="button" className="secondary-button" onClick={onAnother}>
            Registrar otra
          </button>
          <button type="button" className="secondary-button" onClick={onClose}>
            Volver a elaboraciones
          </button>
        </div>
      </div>
      <div className="inventory-figures">
        <div className="inventory-figure">
          <span className="inventory-figure-label">Lotes</span>
          <strong>{formatQuantity(production.batches)}</strong>
        </div>
        <div className="inventory-figure">
          <span className="inventory-figure-label">Esperado</span>
          <strong>{formatQuantity(production.expected_quantity, unit)}</strong>
        </div>
        <div className="inventory-figure">
          <span className="inventory-figure-label">Producido de {production.item.name}</span>
          <strong>{formatQuantity(production.produced_quantity, unit)}</strong>
        </div>
        <div className="inventory-figure">
          <span className="inventory-figure-label">Merma de elaboración</span>
          <strong>{formatQuantity(production.waste_quantity, unit)}</strong>
        </div>
        {production.costs_visible && (
          <div className="inventory-figure">
            <span className="inventory-figure-label">Valor consumido</span>
            <strong>{formatMoney(production.consumed_value_display)}</strong>
            {production.unit_cost_display && (
              <span className="muted text-xs">
                {formatMoney(production.unit_cost_display)} / {unit.code}
              </span>
            )}
          </div>
        )}
      </div>
      <div className="table-container cash-table">
        <table className="inventory-table">
          <caption className="sr-only">Insumos consumidos</caption>
          <thead>
            <tr>
              <th scope="col">Insumo consumido</th>
              <th scope="col" className="inventory-number">
                Cantidad
              </th>
              <th scope="col" className="inventory-number">
                Existencia después
              </th>
              {production.costs_visible && (
                <th scope="col" className="inventory-number">
                  Valor
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {production.lines.map((line) => (
              <tr key={line.item.id}>
                <td className="inventory-cell-name">
                  <strong>{line.item.name}</strong>
                  <span className="muted text-xs inventory-cell-detail">{line.item.code}</span>
                </td>
                <td className="inventory-number">
                  {formatQuantity(line.quantity, line.item.unit)}
                </td>
                <td className="inventory-number">
                  {formatQuantity(line.stock_after, line.item.unit)}
                </td>
                {production.costs_visible && (
                  <td className="inventory-number">{formatMoney(line.value_display)}</td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function ProductionForm({
  execute,
  busy,
  timezone,
  onClose,
}: {
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onClose: () => void;
}) {
  const [recipeID, setRecipeID] = useState('');
  const [warehouseID, setWarehouseID] = useState('');
  const [batches, setBatches] = useState('');
  const [produced, setProduced] = useState('');
  const [notes, setNotes] = useState('');
  const [sending, setSending] = useState(false);
  const [result, setResult] = useState<Production | null>(null);
  const { confirm, dialog } = useConfirmDialog();
  const preparations = useResource<RecipePage>('inventory_recipes', {
    query: { owner_kind: 'preparation', active: 'true', page_size: '100' },
    reloadEvent: inventoryChangeEvent,
  });
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const detail = useResource<Recipe>('inventory_recipe', {
    id: recipeID,
    enabled: recipeID !== '',
    reloadEvent: inventoryChangeEvent,
  });
  const stored = (preparations.data?.items ?? []).filter(
    (recipe) => recipe.preparation_mode === 'stored',
  );
  const available = (warehouses.data ?? []).filter((warehouse) => warehouse.active);
  const fallback = available.find((warehouse) => warehouse.is_default) ?? available[0];
  const selectedWarehouse = warehouseID === '' ? (fallback?.id ?? '') : warehouseID;
  const recipe = detail.data && detail.data.id === recipeID ? detail.data : null;
  const unit = recipe?.preparation_item?.unit ?? null;
  const locked = busy || sending;

  function reset() {
    setBatches('');
    setProduced('');
    setNotes('');
    setResult(null);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    const name = stored.find((entry) => entry.id === recipeID)?.name ?? '';
    const warehouse = available.find((entry) => entry.id === selectedWarehouse)?.name ?? '';
    const confirmed = await confirm({
      title: 'Registrar elaboración',
      message: `Se registrará la elaboración de «${name}» en «${warehouse}» (lotes: ${formatQuantity(batches)}): consume sus insumos y produce el insumo preparado. Una elaboración no se anula; una corrección posterior es un ajuste.`,
      confirmLabel: 'Registrar elaboración',
      cancelLabel: 'Revisar',
    });
    if (!confirmed) {
      return;
    }
    setSending(true);
    try {
      const recorded = await recordProduction(
        execute,
        productionInput(recipeID, selectedWarehouse, batches, produced, notes),
      );
      setResult(recorded);
      notifySuccess(
        `Elaboración registrada: ${formatQuantity(recorded.produced_quantity, recorded.item.unit)} de ${recorded.item.name}.`,
      );
      for (const warning of recorded.warnings) {
        notifyWarning(warning.message);
      }
    } catch (problem) {
      notifyError(problem, 'No se pudo registrar la elaboración.');
    } finally {
      setSending(false);
    }
  }

  if (result) {
    return (
      <ProductionResult
        production={result}
        timezone={timezone}
        onAnother={reset}
        onClose={onClose}
      />
    );
  }

  let producedHint = 'Vacío: lo esperado (lotes × rendimiento efectivo).';
  if (recipe && unit) {
    producedHint = `En ${unit.name.toLowerCase()} (${unit.code}). Vacío: lo esperado; cada lote rinde ${formatQuantity(recipe.version.effective_yield, unit)} descontada la merma.`;
  }

  return (
    <section className="panel form-stack" aria-labelledby="production-form-title">
      <div className="section-heading">
        <div>
          <h2 id="production-form-title">Registrar elaboración</h2>
          <p className="muted text-xs">
            Para preparaciones almacenables: consume la versión vigente de su receta por lote y suma
            lo producido a la existencia del insumo preparado.
          </p>
        </div>
        <button type="button" className="secondary-button" onClick={onClose}>
          Volver a elaboraciones
        </button>
      </div>
      {preparations.data && stored.length === 0 && (
        <Notice>
          No hay preparaciones almacenables activas. Créalas en el apartado «Preparaciones».
        </Notice>
      )}
      <form className="form-stack" onSubmit={(event) => void submit(event)}>
        <fieldset className="form-stack" disabled={locked}>
          <div className="form-grid">
            <Field label="Preparación elaborada">
              <Select
                required
                value={recipeID}
                placeholder="Elige la preparación"
                options={stored.map((entry) => ({ value: entry.id, label: entry.name }))}
                onChange={setRecipeID}
              />
            </Field>
            <Field
              label="Almacén de la elaboración"
              hint="Donde se consumen los insumos y entra lo producido."
            >
              <Select
                required
                value={selectedWarehouse}
                options={available.map((warehouse) => ({
                  value: warehouse.id,
                  label: warehouse.name,
                }))}
                onChange={setWarehouseID}
              />
            </Field>
            <DecimalTextField
              label="Lotes elaborados"
              value={batches}
              onChange={setBatches}
              shape={inventoryQuantityShape}
              required
              hint="Admite fracciones, por ejemplo 0.5 para medio lote."
            />
            <DecimalTextField
              label="Cantidad producida real"
              value={produced}
              onChange={setProduced}
              shape={inventoryQuantityShape}
              hint={producedHint}
            />
          </div>
          <Field label="Nota de la elaboración" hint="Opcional; hasta 500 caracteres.">
            <input
              maxLength={500}
              value={notes}
              onChange={(event) => setNotes(event.target.value)}
            />
          </Field>
        </fieldset>
        <div className="form-actions">
          <button type="submit" className="primary-button" disabled={locked || recipeID === ''}>
            {sending ? 'Registrando…' : 'Registrar elaboración'}
          </button>
        </div>
      </form>
      {dialog}
    </section>
  );
}

// Apartado «Elaboraciones»: registro (inventory.production.record) y listado de las elaboraciones
// de preparaciones almacenables (también con inventory.movements.read), la más reciente primero.
export function ProductionsView({
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
  const [recording, setRecording] = useState(false);
  const [page, setPage] = useState(1);
  const list = useResource<ProductionPage>('inventory_productions', {
    query: { page: String(page), page_size: String(pageSize) },
    reloadEvent: inventoryChangeEvent,
  });
  const recordable = can('inventory.production.record');

  if (recording) {
    return (
      <ProductionForm
        execute={execute}
        busy={busy}
        timezone={timezone}
        onClose={() => setRecording(false)}
      />
    );
  }

  const data = list.data;
  const costsVisible = data?.costs_visible ?? false;
  return (
    <div className="form-stack">
      <div className="section-heading">
        <p className="muted text-sm">
          Elaboraciones de preparaciones almacenables: lo consumido, lo producido y la merma de cada
          lote.
        </p>
        {recordable && (
          <button type="button" className="primary-button" onClick={() => setRecording(true)}>
            <IconLabel icon="plus">Registrar elaboración</IconLabel>
          </button>
        )}
      </div>
      {!recordable && (
        <Notice>
          Consulta disponible. Registrar elaboraciones requiere el permiso «
          {inventoryPermissionName('inventory.production.record')}».
        </Notice>
      )}
      <Notice>{list.error}</Notice>
      {!data && !list.error && <p className="muted">Consultando elaboraciones…</p>}
      {data && (
        <section className="panel">
          <div className="table-container cash-table">
            <table className="inventory-table">
              <thead>
                <tr>
                  <th scope="col">Fecha</th>
                  <th scope="col">Preparación</th>
                  <th scope="col">Almacén</th>
                  <th scope="col" className="inventory-number">
                    Lotes
                  </th>
                  <th scope="col" className="inventory-number">
                    Esperado
                  </th>
                  <th scope="col" className="inventory-number">
                    Producido
                  </th>
                  <th scope="col" className="inventory-number">
                    Merma
                  </th>
                  {costsVisible && (
                    <th scope="col" className="inventory-number">
                      Valor
                    </th>
                  )}
                  <th scope="col">Responsable</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((production) => (
                  <tr key={production.id}>
                    <td>{serverDateTime(production.occurred_at, timezone)}</td>
                    <td className="inventory-cell-name">
                      <strong>{production.recipe_name}</strong>
                      <span className="muted text-xs inventory-cell-detail">
                        Versión {production.recipe_version} · {production.item.name}
                      </span>
                    </td>
                    <td>{production.warehouse.name}</td>
                    <td className="inventory-number">{formatQuantity(production.batches)}</td>
                    <td className="inventory-number">
                      {formatQuantity(production.expected_quantity, production.item.unit)}
                    </td>
                    <td className="inventory-number">
                      {formatQuantity(production.produced_quantity, production.item.unit)}
                    </td>
                    <td className="inventory-number">
                      {formatQuantity(production.waste_quantity, production.item.unit)}
                    </td>
                    {costsVisible && (
                      <td className="inventory-number">
                        {formatMoney(production.consumed_value_display)}
                      </td>
                    )}
                    <td>{production.actor_name ?? '—'}</td>
                  </tr>
                ))}
                {data.items.length === 0 && (
                  <tr>
                    <td colSpan={costsVisible ? 9 : 8} className="muted">
                      Aún no hay elaboraciones registradas.
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
            singular="elaboración"
            plural="elaboraciones"
            onPage={setPage}
          />
        </section>
      )}
    </div>
  );
}
