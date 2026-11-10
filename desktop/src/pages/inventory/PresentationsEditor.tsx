import { useState } from 'react';
import { Field, Form } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { ToggleButton } from '../../components/ToggleButton';
import { createPresentation, savePresentation } from '../../services/inventory';
import type {
  InventoryExecute,
  InventoryItem,
  InventoryPresentation,
  InventoryUnit,
  PresentationInput,
} from '../../services/inventory';
import { inventoryQuantityShape } from '../../services/numericInput';
import { countLabel } from '../../services/plural';
import { ActiveToggle, DecimalTextField } from './InventoryFields';
import { formatQuantity } from './inventoryFormat';

// Alta o edición de una presentación de compra («Caja con 24», «Costal 25 kg»). La equivalencia se
// captura en la unidad base o en otra unidad de la misma dimensión; Go la convierte y la guarda
// en la unidad base. Al editar se muestra ya convertida a la unidad base.
function PresentationForm({
  item,
  presentation,
  units,
  execute,
  busy,
  onSaved,
  onCancel,
}: {
  item: InventoryItem;
  presentation: InventoryPresentation | null;
  units: InventoryUnit[];
  execute: InventoryExecute;
  busy: boolean;
  onSaved: (item: InventoryItem) => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState(presentation?.name ?? '');
  const [quantity, setQuantity] = useState(presentation?.base_quantity ?? '');
  const [unitID, setUnitID] = useState(item.base_unit.id);
  const [barcode, setBarcode] = useState(presentation?.barcode ?? '');
  const [active, setActive] = useState(presentation?.active ?? true);
  const [purchaseDefault, setPurchaseDefault] = useState(presentation?.purchase_default ?? false);
  // Solo unidades activas de la misma dimensión que la unidad base del insumo.
  const sameDimension = units.filter(
    (unit) =>
      unit.dimension === item.base_unit.dimension && (unit.active || unit.id === item.base_unit.id),
  );
  return (
    <Form
      label={presentation ? 'Guardar presentación' : 'Agregar presentación'}
      successMessage={presentation ? 'Presentación guardada.' : 'Presentación agregada.'}
      disabled={busy}
      onSave={async () => {
        const input: PresentationInput = {
          name,
          quantity,
          unit_id: unitID === item.base_unit.id ? null : unitID,
          barcode: barcode.trim() === '' ? null : barcode,
          active,
          purchase_default: purchaseDefault,
        };
        if (presentation) {
          input.revision = presentation.revision;
        }
        const saved = presentation
          ? await savePresentation(execute, presentation.id, input)
          : await createPresentation(execute, item.id, input);
        onSaved(saved);
      }}
    >
      <div className="form-grid">
        <Field label="Nombre de la presentación" hint="Por ejemplo «Caja con 24» o «Costal 25 kg».">
          <input
            required
            maxLength={60}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        <Field label="Código de barras de la presentación" hint="Opcional.">
          <input
            maxLength={64}
            autoComplete="off"
            value={barcode}
            onChange={(event) => setBarcode(event.target.value)}
          />
        </Field>
        <DecimalTextField
          label="Contenido"
          value={quantity}
          onChange={setQuantity}
          shape={inventoryQuantityShape}
          required
          hint="Cuánto del insumo trae una presentación."
        />
        <Field
          label="Unidad del contenido"
          hint={`Go lo convierte a ${item.base_unit.name.toLowerCase()} (${item.base_unit.code}).`}
        >
          <select value={unitID} onChange={(event) => setUnitID(event.target.value)}>
            {sameDimension.map((unit) => (
              <option key={unit.id} value={unit.id}>
                {unit.name} ({unit.code})
              </option>
            ))}
          </select>
        </Field>
      </div>
      <div className="button-row">
        <ToggleButton
          label="Predeterminada para compra"
          pressed={purchaseDefault}
          onChange={(value) => {
            setPurchaseDefault(value);
            if (value) {
              setActive(true);
            }
          }}
        />
        <ActiveToggle
          label="Presentación activa"
          pressed={active}
          confirmDeactivation={presentation?.active ?? false}
          title="Desactivar presentación"
          message={`Al guardar, «${presentation?.name ?? ''}» dejará de ofrecerse para capturar cantidades.`}
          onChange={(value) => {
            setActive(value);
            if (!value) {
              setPurchaseDefault(false);
            }
          }}
        />
        <button type="button" className="secondary-button" onClick={onCancel}>
          Cancelar
        </button>
      </div>
      <p className="muted text-xs">
        Solo una presentación es la predeterminada para compra: marcar otra le quita la marca a la
        anterior.
      </p>
    </Form>
  );
}

// Presentaciones de compra de un insumo ya guardado. La conversión caja → piezas vive en la
// presentación del insumo, nunca como una equivalencia universal (§4).
export function PresentationsEditor({
  item,
  units,
  editable,
  execute,
  busy,
  onChanged,
}: {
  item: InventoryItem;
  units: InventoryUnit[];
  editable: boolean;
  execute: InventoryExecute;
  busy: boolean;
  onChanged: (item: InventoryItem) => void;
}) {
  // null: sin formulario; '': presentación nueva; id: presentación en edición.
  const [editing, setEditing] = useState<string | null>(null);
  const current = item.presentations.find((presentation) => presentation.id === editing) ?? null;
  const activeCount = item.presentations.filter((presentation) => presentation.active).length;
  return (
    <section
      className="inventory-subsection form-stack"
      aria-labelledby="inventory-presentations-title"
    >
      <div className="section-heading">
        <div>
          <h3 id="inventory-presentations-title">Presentaciones de compra</h3>
          <p className="muted text-xs">
            {item.presentations.length === 0
              ? 'Sin presentaciones: se compra y se cuenta en la unidad base.'
              : `${countLabel(activeCount, 'activa', 'activas')} de ${item.presentations.length}`}
          </p>
        </div>
        {editable && editing === null && (
          <button
            type="button"
            className="secondary-button"
            disabled={busy}
            onClick={() => setEditing('')}
          >
            <IconLabel icon="plus">Nueva presentación</IconLabel>
          </button>
        )}
      </div>
      {item.presentations.length > 0 && (
        <ul className="inventory-list">
          {item.presentations.map((presentation) => (
            <li key={presentation.id} className="inventory-list-row">
              <div className="inventory-list-text">
                <strong>{presentation.name}</strong>
                <span className="muted text-xs">
                  {formatQuantity(presentation.base_quantity, item.base_unit)}
                  {presentation.barcode ? ` · código ${presentation.barcode}` : ''}
                </span>
              </div>
              <div className="button-row">
                {presentation.purchase_default && (
                  <span className="tag tag-good">Predeterminada para compra</span>
                )}
                {!presentation.active && <span className="tag">Inactiva</span>}
                {editable && (
                  <button
                    type="button"
                    className="secondary-button"
                    aria-label={`Editar la presentación ${presentation.name}`}
                    disabled={busy || editing !== null}
                    onClick={() => setEditing(presentation.id)}
                  >
                    Editar
                  </button>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
      {editable && editing !== null && (
        <PresentationForm
          key={current ? `${current.id}:${current.revision}` : 'new'}
          item={item}
          presentation={current}
          units={units}
          execute={execute}
          busy={busy}
          onCancel={() => setEditing(null)}
          onSaved={(saved) => {
            onChanged(saved);
            setEditing(null);
          }}
        />
      )}
    </section>
  );
}
