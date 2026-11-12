import { useState } from 'react';
import { Field, Form, Notice } from '../../components/Forms';
import { createInventoryUnit, dimensionLabels, saveInventoryUnit } from '../../services/inventory';
import type {
  Dimension,
  InventoryExecute,
  InventoryUnit,
  UnitInput,
} from '../../services/inventory';
import { inventoryCostShape } from '../../services/numericInput';
import { ActiveToggle, DecimalTextField } from './InventoryFields';
import { formatQuantity, inventoryPermissionName } from './inventoryFormat';
import { Select } from '../../components/Select';

const dimensions = Object.keys(dimensionLabels) as Dimension[];

// Unidad base de cada dimensión (g, ml, pza y porción, equivalencia 1), tomada de las unidades del
// sistema que entrega Go.
function dimensionBase(units: InventoryUnit[], dimension: Dimension) {
  return units.find((unit) => unit.dimension === dimension && unit.system && unit.factor === '1');
}

function equivalence(unit: InventoryUnit, units: InventoryUnit[]) {
  const base = dimensionBase(units, unit.dimension);
  if (!base || base.id === unit.id) {
    return 'Unidad base de la dimensión';
  }
  return `1 ${unit.code} = ${formatQuantity(unit.factor, base)}`;
}

// Alta o edición de una unidad propia (por ejemplo «oz», masa, 28.349523125 g). La abreviatura y la
// dimensión no cambian después; la equivalencia tampoco cuando la unidad ya es base de un insumo.
function UnitEditor({
  unit,
  units,
  execute,
  busy,
  onClose,
}: {
  unit: InventoryUnit | null;
  units: InventoryUnit[];
  execute: InventoryExecute;
  busy: boolean;
  onClose: () => void;
}) {
  const [code, setCode] = useState(unit?.code ?? '');
  const [name, setName] = useState(unit?.name ?? '');
  const [dimension, setDimension] = useState<Dimension>(unit?.dimension ?? 'mass');
  const [factor, setFactor] = useState(unit?.factor ?? '');
  const [active, setActive] = useState(unit?.active ?? true);
  const base = dimensionBase(units, dimension);
  return (
    <Form
      label={unit ? 'Guardar unidad' : 'Crear unidad'}
      successMessage={unit ? 'Unidad guardada.' : 'Unidad creada.'}
      disabled={busy}
      onSave={async () => {
        if (unit) {
          const input: UnitInput = { revision: unit.revision, name, factor, active };
          await saveInventoryUnit(execute, unit.id, input);
        } else {
          await createInventoryUnit(execute, { code, name, dimension, factor, active });
        }
        onClose();
      }}
    >
      <div className="form-grid">
        <Field
          label="Abreviatura"
          hint="Minúsculas, dígitos o guion bajo (hasta 16). No cambia después."
        >
          <input
            required
            maxLength={16}
            autoComplete="off"
            value={code}
            disabled={unit !== null}
            onChange={(event) => setCode(event.target.value.toLowerCase())}
          />
        </Field>
        <Field label="Nombre de la unidad">
          <input
            required
            maxLength={40}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        <Field label="Dimensión" hint="Solo convierte con unidades de la misma dimensión.">
          <Select<Dimension>
            value={dimension}
            disabled={unit !== null}
            options={dimensions.map((value) => ({ value, label: dimensionLabels[value] }))}
            onChange={setDimension}
          />
        </Field>
        <DecimalTextField
          label="Equivalencia"
          value={factor}
          onChange={setFactor}
          shape={inventoryCostShape}
          required
          disabled={unit?.in_use ?? false}
          hint={
            unit?.in_use
              ? 'No cambia: la unidad ya es la unidad base de algún insumo.'
              : `Cuánto equivale 1 unidad en ${base ? `${base.name.toLowerCase()} (${base.code})` : 'la unidad base'}.`
          }
        />
      </div>
      <div className="button-row">
        <ActiveToggle
          label="Unidad activa"
          pressed={active}
          onChange={setActive}
          confirmDeactivation={unit?.active ?? false}
          title="Desactivar unidad"
          message={`Al guardar, «${unit?.name ?? ''}» dejará de ofrecerse como unidad.`}
        />
        <button type="button" className="secondary-button" onClick={onClose}>
          Cancelar
        </button>
      </div>
    </Form>
  );
}

// Unidades de medida por dimensión. Las del sistema (g, kg, ml, l, pza y porción) son de referencia
// e inmutables; paquete y caja no son unidades sino presentaciones del insumo.
export function UnitsAdmin({
  units,
  editable,
  execute,
  busy,
}: {
  units: InventoryUnit[];
  editable: boolean;
  execute: InventoryExecute;
  busy: boolean;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const current = units.find((unit) => unit.id === editing) ?? null;
  return (
    <div className="form-stack">
      <p className="muted text-sm">
        Paquetes, cajas y costales no son unidades: se configuran como presentaciones de cada
        insumo.
      </p>
      {!editable && (
        <Notice>
          Agregar unidades requiere el permiso «{inventoryPermissionName('inventory.items.manage')}
          ».
        </Notice>
      )}
      <div className="table-container cash-table">
        <table>
          <thead>
            <tr>
              <th scope="col">Unidad</th>
              <th scope="col">Dimensión</th>
              <th scope="col">Equivalencia</th>
              <th scope="col">Estado</th>
              <th scope="col">
                <span className="sr-only">Acciones</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {units.map((unit) => (
              <tr key={unit.id}>
                <td>
                  <strong>{unit.name}</strong>
                  <span className="muted text-xs"> · {unit.code}</span>
                </td>
                <td>{unit.dimension_label}</td>
                <td>{equivalence(unit, units)}</td>
                <td>
                  <div className="inventory-tags">
                    {unit.system && <span className="tag">Del sistema</span>}
                    {unit.in_use && <span className="tag">En uso</span>}
                    {!unit.active && <span className="tag">Inactiva</span>}
                  </div>
                </td>
                <td>
                  {editable && !unit.system && (
                    <button
                      type="button"
                      className="secondary-button"
                      aria-label={`Editar la unidad ${unit.name}`}
                      onClick={() => setEditing(unit.id)}
                    >
                      Editar
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {editable && editing === null && (
        <div className="button-row">
          <button type="button" className="secondary-button" onClick={() => setEditing('')}>
            + Nueva unidad
          </button>
        </div>
      )}
      {editable && editing !== null && (
        <UnitEditor
          key={current ? `${current.id}:${current.revision}` : 'new'}
          unit={current}
          units={units}
          execute={execute}
          busy={busy}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  );
}
