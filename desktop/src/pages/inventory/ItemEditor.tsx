import { useEffect, useRef, useState } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Form, Notice } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import { createInventoryItem, saveInventoryItem } from '../../services/inventory';
import type {
  InventoryCategory,
  InventoryExecute,
  InventoryItem,
  InventoryUnit,
  ItemInput,
  ItemKind,
} from '../../services/inventory';
import { formatMoney } from '../../services/money';
import { inventoryCostShape, inventoryQuantityShape } from '../../services/numericInput';
import { ActiveToggle, DecimalTextField } from './InventoryFields';
import { inventoryPermissionName } from './inventoryFormat';
import { PresentationsEditor } from './PresentationsEditor';

interface ItemFormState {
  code: string;
  barcode: string;
  name: string;
  description: string;
  categoryID: string;
  baseUnitID: string;
  // Comprado o preparación almacenable (la que produce una elaboración, fase B).
  kind: ItemKind;
  active: boolean;
  referenceCost: string;
  minStock: string;
  reorderPoint: string;
}

function initialForm(item: InventoryItem | null): ItemFormState {
  return {
    code: item?.code ?? '',
    barcode: item?.barcode ?? '',
    name: item?.name ?? '',
    description: item?.description ?? '',
    categoryID: item?.category_id ?? '',
    baseUnitID: item?.base_unit.id ?? '',
    kind: item?.kind ?? 'purchased',
    active: item?.active ?? true,
    referenceCost: item?.reference_cost ?? '',
    minStock: item?.min_stock ?? '',
    reorderPoint: item?.reorder_point ?? '',
  };
}

function optionalText(value: string) {
  if (value.trim() === '') {
    return null;
  }
  return value;
}

// Cuerpo del insumo para Go. El costo de referencia solo viaja si quien guarda puede administrar
// costos y lo cambió (o cambió la unidad base, porque el costo es por unidad base): omitirlo
// conserva el vigente. El tipo se envía siempre (Go pondría «comprado» si faltara); el de un
// insumo que produce una preparación almacenable no cambia (Go: ITEM_PRODUCED_BY_RECIPE).
export function itemInput(
  form: ItemFormState,
  item: InventoryItem | null,
  costEditable: boolean,
): ItemInput {
  const input: ItemInput = {
    code: form.code,
    barcode: optionalText(form.barcode),
    name: form.name,
    description: form.description,
    category_id: form.categoryID === '' ? null : form.categoryID,
    base_unit_id: form.baseUnitID,
    kind: form.kind,
    active: form.active,
    min_stock: optionalText(form.minStock),
    reorder_point: optionalText(form.reorderPoint),
  };
  if (costEditable) {
    const unitChanged = item !== null && item.base_unit.id !== form.baseUnitID;
    const costChanged = form.referenceCost !== (item?.reference_cost ?? '');
    if (costChanged || unitChanged) {
      input.reference_cost = optionalText(form.referenceCost);
    }
  }
  if (item) {
    input.revision = item.revision;
  }
  return input;
}

// Unidades que se ofrecen como unidad base: las activas, agrupadas por dimensión, más la vigente
// del insumo aunque esté desactivada.
function BaseUnitSelect({
  units,
  value,
  onChange,
  locked,
}: {
  units: InventoryUnit[];
  value: string;
  onChange: (value: string) => void;
  locked: boolean;
}) {
  const offered = units.filter((unit) => unit.active || unit.id === value);
  const dimensions = Array.from(new Set(offered.map((unit) => unit.dimension)));
  return (
    <select
      required
      value={value}
      disabled={locked}
      onChange={(event) => onChange(event.target.value)}
    >
      <option value="">Elige la unidad base</option>
      {dimensions.map((dimension) => {
        const members = offered.filter((unit) => unit.dimension === dimension);
        return (
          <optgroup key={dimension} label={members[0]?.dimension_label ?? dimension}>
            {members.map((unit) => (
              <option key={unit.id} value={unit.id}>
                {unit.name} ({unit.code})
              </option>
            ))}
          </optgroup>
        );
      })}
    </select>
  );
}

function ItemForm({
  item,
  categories,
  units,
  costsVisible,
  can,
  execute,
  busy,
  onSaved,
  onDirtyChange,
}: {
  item: InventoryItem | null;
  categories: InventoryCategory[];
  units: InventoryUnit[];
  costsVisible: boolean;
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  onSaved: (item: InventoryItem) => void;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const [initial] = useState(() => initialForm(item));
  const [form, setForm] = useState(initial);
  const editable = can('inventory.items.manage');
  const costEditable = costsVisible && can('inventory.costs.manage');
  const baseUnit = units.find((unit) => unit.id === form.baseUnitID) ?? null;
  const unitLocked = item !== null && (item.has_movements || item.presentations.length > 0);
  const unitChanged = item !== null && item.base_unit.id !== form.baseUnitID;
  const offeredCategories = categories.filter(
    (category) => category.active || category.id === form.categoryID,
  );
  const dirty = JSON.stringify(form) !== JSON.stringify(initial);

  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty, onDirtyChange]);

  function update<K extends keyof ItemFormState>(key: K, value: ItemFormState[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  const unitHint = baseUnit ? `En ${baseUnit.name.toLowerCase()} (${baseUnit.code}).` : undefined;
  let costHint = baseUnit ? `Por ${baseUnit.name.toLowerCase()} (unidad base).` : undefined;
  if (unitChanged && item?.reference_cost) {
    costHint = 'El costo es por unidad base: confirma el costo por la unidad nueva.';
  }
  let unitLockHint: string | undefined;
  if (unitLocked) {
    unitLockHint = item?.has_movements
      ? 'No cambia: el insumo ya tiene movimientos en esta unidad.'
      : 'No cambia mientras el insumo tenga presentaciones.';
  }

  return (
    <Form
      label={item ? 'Guardar insumo' : 'Crear insumo'}
      successMessage={
        item ? 'Insumo guardado.' : 'Insumo creado. Ya puedes agregar sus presentaciones.'
      }
      disabled={busy || !editable}
      onSave={async () => {
        const input = itemInput(form, item, costEditable);
        const saved = item
          ? await saveInventoryItem(execute, item.id, input)
          : await createInventoryItem(execute, input);
        onDirtyChange(false);
        onSaved(saved);
      }}
    >
      <div className="form-grid">
        <Field label="Nombre del insumo">
          <input
            required
            maxLength={100}
            value={form.name}
            onChange={(event) => update('name', event.target.value)}
          />
        </Field>
        <Field label="Código interno">
          <input
            required
            maxLength={40}
            autoComplete="off"
            value={form.code}
            onChange={(event) => update('code', event.target.value)}
          />
        </Field>
        <Field label="Código de barras" hint="Opcional; único entre insumos y presentaciones.">
          <input
            maxLength={64}
            autoComplete="off"
            value={form.barcode}
            onChange={(event) => update('barcode', event.target.value)}
          />
        </Field>
        <Field label="Categoría">
          <select
            value={form.categoryID}
            onChange={(event) => update('categoryID', event.target.value)}
          >
            <option value="">Sin categoría</option>
            {offeredCategories.map((category) => (
              <option key={category.id} value={category.id}>
                {category.name}
                {category.active ? '' : ' (inactiva)'}
              </option>
            ))}
          </select>
        </Field>
        <Field
          label="Unidad base"
          hint={unitLockHint ?? 'Las existencias, costos y recetas se expresan en esta unidad.'}
        >
          <BaseUnitSelect
            units={units}
            value={form.baseUnitID}
            onChange={(value) => update('baseUnitID', value)}
            locked={unitLocked}
          />
        </Field>
        <Field
          label="Tipo de insumo"
          hint="Una preparación almacenable aumenta con cada elaboración registrada en Recetas."
        >
          <select
            value={form.kind}
            onChange={(event) => update('kind', event.target.value as ItemKind)}
          >
            <option value="purchased">Comprado</option>
            <option value="prepared">Preparación almacenable</option>
          </select>
        </Field>
        {costEditable && (
          <DecimalTextField
            label="Costo de referencia"
            value={form.referenceCost}
            onChange={(value) => update('referenceCost', value)}
            shape={inventoryCostShape}
            hint={costHint ? `${costHint} Vacío: sin costo de referencia.` : undefined}
          />
        )}
        {costsVisible && !costEditable && (
          <Field
            label="Costo de referencia"
            hint={`Cambiarlo requiere el permiso «${inventoryPermissionName('inventory.costs.manage')}».`}
          >
            <input
              readOnly
              value={item?.reference_cost_display ? formatMoney(item.reference_cost_display) : '—'}
            />
          </Field>
        )}
        <DecimalTextField
          label="Existencia mínima"
          value={form.minStock}
          onChange={(value) => update('minStock', value)}
          shape={inventoryQuantityShape}
          hint={unitHint ? `${unitHint} Vacío: sin alerta.` : 'Vacío: sin alerta.'}
        />
        <DecimalTextField
          label="Punto de reabastecimiento"
          value={form.reorderPoint}
          onChange={(value) => update('reorderPoint', value)}
          shape={inventoryQuantityShape}
          hint={unitHint ? `${unitHint} Vacío: sin alerta.` : 'Vacío: sin alerta.'}
        />
      </div>
      <Field label="Descripción">
        <textarea
          maxLength={2000}
          value={form.description}
          onChange={(event) => update('description', event.target.value)}
        />
      </Field>
      <div className="button-row">
        <ActiveToggle
          label="Insumo activo"
          pressed={form.active}
          onChange={(value) => update('active', value)}
          confirmDeactivation={item?.active ?? false}
          title="Desactivar insumo"
          message={`Al guardar, «${item?.name ?? ''}» dejará de ofrecerse para nuevas capturas. Conserva sus existencias, presentaciones y movimientos.`}
        />
        {item?.kind === 'prepared' && <span className="tag">{item.kind_label}</span>}
      </div>
      {!form.active && item?.active && (
        <p className="muted text-xs">
          Desactivar un insumo solo deja de ofrecerlo: conserva sus existencias y movimientos.
        </p>
      )}
    </Form>
  );
}

// Panel de alta y edición de un insumo: datos generales y, una vez creado, sus presentaciones. Al
// crear, el panel continúa con el insumo guardado para agregar presentaciones sin volver al
// listado. Los cambios de otra terminal recargan el insumo (por SSE o por un conflicto de revisión).
export function ItemEditorPanel({
  initialItemID,
  categories,
  units,
  costsVisible,
  can,
  execute,
  busy,
  onClose,
}: {
  initialItemID: string;
  categories: InventoryCategory[];
  units: InventoryUnit[];
  costsVisible: boolean;
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  onClose: () => void;
}) {
  const [itemID, setItemID] = useState(initialItemID);
  const [snapshot, setSnapshot] = useState<InventoryItem | null>(null);
  const dirty = useRef(false);
  const { confirm, dialog } = useConfirmDialog();
  const resource = useResource<InventoryItem>('inventory_item', {
    id: itemID,
    enabled: itemID !== '',
    reloadEvent: inventoryChangeEvent,
  });

  useEffect(() => {
    if (resource.data) {
      setSnapshot(resource.data);
    }
  }, [resource.data]);

  const item = itemID === '' ? null : snapshot;

  async function close() {
    if (dirty.current) {
      const discard = await confirm({
        title: 'Cambios sin guardar',
        message: 'Este insumo tiene cambios que aún no se guardan. ¿Quieres descartarlos?',
        confirmLabel: 'Descartar cambios',
        cancelLabel: 'Seguir editando',
        tone: 'danger',
      });
      if (!discard) {
        return;
      }
    }
    onClose();
  }

  return (
    <section
      className="panel form-stack"
      aria-label={item ? `Insumo ${item.name}` : 'Nuevo insumo'}
    >
      <div className="section-heading">
        <div>
          <h2>{item ? item.name : itemID === '' ? 'Nuevo insumo' : 'Insumo'}</h2>
          <p className="muted text-xs">
            {item
              ? `${item.code} · unidad base ${item.base_unit.name.toLowerCase()} (${item.base_unit.code})`
              : 'Se guardará al confirmar'}
          </p>
        </div>
        <button type="button" className="secondary-button" onClick={() => void close()}>
          Volver al listado
        </button>
      </div>
      {!can('inventory.items.manage') && (
        <Notice>
          Consulta disponible. Editar insumos requiere el permiso «
          {inventoryPermissionName('inventory.items.manage')}».
        </Notice>
      )}
      {itemID !== '' && !item && <Notice>{resource.error || 'Consultando el insumo…'}</Notice>}
      {(itemID === '' || item) && (
        <ItemForm
          key={item ? `${item.id}:${item.revision}` : 'new'}
          item={item}
          categories={categories}
          units={units}
          costsVisible={item ? item.costs_visible : costsVisible}
          can={can}
          execute={execute}
          busy={busy}
          onDirtyChange={(value) => {
            dirty.current = value;
          }}
          onSaved={(saved) => {
            setSnapshot(saved);
            setItemID(saved.id);
          }}
        />
      )}
      {item && (
        <PresentationsEditor
          item={item}
          units={units}
          editable={can('inventory.items.manage')}
          execute={execute}
          busy={busy}
          onChanged={setSnapshot}
        />
      )}
      {dialog}
    </section>
  );
}
