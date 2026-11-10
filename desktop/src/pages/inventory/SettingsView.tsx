import { useState } from 'react';
import { Field, Form, Notice } from '../../components/Forms';
import { SegmentedControl } from '../../components/SegmentedControl';
import { ToggleButton } from '../../components/ToggleButton';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import { countFrequencyLabels, saveInventorySettings } from '../../services/inventory';
import type { CountFrequency, InventoryExecute, InventorySettings } from '../../services/inventory';
import { acceptIntegerText } from '../../services/numericInput';
import { serverDateTime } from '../../services/restaurant';

const frequencies = Object.keys(countFrequencyLabels) as CountFrequency[];
const cutoffPattern = '([01][0-9]|2[0-3]):[0-5][0-9]';

// Reconstruye HH:MM a partir de los dígitos tecleados (como el campo de fechas dd/mm/aaaa).
export function formatCutoffDigits(text: string): string | null {
  if (!/^[0-9:]*$/.test(text)) {
    return null;
  }
  const digits = text.replaceAll(':', '').slice(0, 4);
  if (digits.length <= 2) {
    return digits;
  }
  return `${digits.slice(0, 2)}:${digits.slice(2)}`;
}

function SettingsForm({
  settings,
  execute,
  busy,
  timezone,
}: {
  settings: InventorySettings;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
}) {
  const [taxInCost, setTaxInCost] = useState(settings.purchase_tax_in_cost);
  const [cutoff, setCutoff] = useState(settings.operating_day_cutoff);
  const [warnWithoutRecipe, setWarnWithoutRecipe] = useState(settings.warn_products_without_recipe);
  const [frequency, setFrequency] = useState<CountFrequency>(settings.count_frequency);
  const [interval, setIntervalDays] = useState(
    settings.count_interval_days === null ? '' : String(settings.count_interval_days),
  );
  return (
    <Form
      label="Guardar configuración"
      successMessage="Configuración de inventario guardada."
      disabled={busy}
      onSave={async () => {
        await saveInventorySettings(execute, {
          revision: settings.revision,
          purchase_tax_in_cost: taxInCost,
          operating_day_cutoff: cutoff,
          warn_products_without_recipe: warnWithoutRecipe,
          count_frequency: frequency,
          count_interval_days: frequency === 'custom' ? Number(interval) : null,
        });
      }}
    >
      <div className="inventory-setting">
        <ToggleButton
          label="El IVA de las compras forma parte del costo"
          pressed={taxInCost}
          onChange={setTaxInCost}
          describedBy="inventory-tax-description"
        />
        <p id="inventory-tax-description" className="muted text-sm">
          Activado, el costo del insumo es el total con IVA (lo normal cuando no se acredita el
          IVA). Desactivado, el costo es el subtotal sin IVA y el IVA se guarda aparte en la compra.
          El cambio solo afecta las recepciones futuras.
        </p>
      </div>
      <div className="form-grid">
        <Field
          label="Hora de corte del día operativo"
          hint="HH:MM en la zona del establecimiento. Lo registrado antes de esta hora cuenta para el día operativo anterior; no depende de los turnos de caja."
        >
          <input
            type="text"
            inputMode="numeric"
            autoComplete="off"
            required
            maxLength={5}
            pattern={cutoffPattern}
            placeholder="05:00"
            value={cutoff}
            onChange={(event) => {
              const formatted = formatCutoffDigits(event.target.value);
              if (formatted !== null) {
                setCutoff(formatted);
              }
            }}
          />
        </Field>
      </div>
      <div className="inventory-setting">
        <ToggleButton
          label="Avisar de productos sin receta"
          pressed={warnWithoutRecipe}
          onChange={setWarnWithoutRecipe}
          describedBy="inventory-recipe-description"
        />
        <p id="inventory-recipe-description" className="muted text-sm">
          Señala los productos que se venden sin receta de inventario. Nunca bloquea la venta.
        </p>
      </div>
      <div className="form-stack inventory-setting-frequency">
        <span className="inventory-setting-label">Frecuencia de conteo físico</span>
        <SegmentedControl
          label="Frecuencia de conteo físico"
          value={frequency}
          options={frequencies.map((value) => ({ value, label: countFrequencyLabels[value] }))}
          onChange={setFrequency}
        />
        {frequency === 'custom' && (
          <div className="form-grid">
            <Field label="Cada cuántos días" hint="De 1 a 365 días.">
              <input
                type="text"
                inputMode="numeric"
                autoComplete="off"
                required
                value={interval}
                onChange={(event) =>
                  setIntervalDays(acceptIntegerText(event.target.value, interval, 3))
                }
              />
            </Field>
          </div>
        )}
      </div>
      <p className="muted text-xs">
        Última modificación: {serverDateTime(settings.updated_at, timezone)}
        {settings.updated_by_name ? ` · ${settings.updated_by_name}` : ''}
      </p>
    </Form>
  );
}

// Pestaña «Configuración» (inventory.settings.manage): parámetros generales de inventario con
// revisión optimista. Si otra terminal los cambió, Go responde REVISION_CONFLICT: la pestaña carga
// la versión vigente (el formulario se vuelve a montar con su revisión) y el aviso lo explica.
export function SettingsView({
  execute,
  busy,
  timezone,
}: {
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
}) {
  const resource = useResource<InventorySettings>('inventory_settings', {
    reloadEvent: inventoryChangeEvent,
  });
  if (!resource.data) {
    return <Notice>{resource.error || 'Consultando la configuración de inventario…'}</Notice>;
  }
  return (
    <section className="panel form-stack" aria-labelledby="inventory-settings-title">
      <div>
        <h2 id="inventory-settings-title">Parámetros de inventario</h2>
        <p className="muted text-xs">
          Costos de compra, día operativo para reportes y conteos, y avisos de configuración.
        </p>
      </div>
      <Notice>{resource.error}</Notice>
      <SettingsForm
        key={resource.data.revision}
        settings={resource.data}
        execute={execute}
        busy={busy}
        timezone={timezone}
      />
    </section>
  );
}
