import { useState } from 'react';
import { SegmentedControl } from '../../components/SegmentedControl';
import { notifyError, notifySuccess } from '../../services/notify';
import { permissionName, taxRateLabels } from '../../services/payments';
import type { PaymentSettings, TaxRate } from '../../services/payments';
import type { ExecuteMutation } from '../restaurant/OperationsAdmin';

const rates: TaxRate[] = ['16', '8', '0'];

// La tasa de IVA se elige de una lista cerrada (16 %, 8 % o 0 %): no existe campo de captura.
export function TaxRateSection({
  settings,
  can,
  execute,
  busy,
}: {
  settings: PaymentSettings;
  can: (permission: string) => boolean;
  execute: ExecuteMutation;
  busy: boolean;
}) {
  const [selected, setSelected] = useState<TaxRate>(settings.tax_rate);
  const [saving, setSaving] = useState(false);
  const editable = can('payments.tax.manage');

  async function save() {
    setSaving(true);
    try {
      await execute('payments_save_tax', {
        body: { revision: settings.revision, tax_rate: selected },
      });
      notifySuccess(`Tasa de IVA guardada: ${selected} %.`);
    } catch (problem) {
      notifyError(problem, 'No se pudo guardar la tasa.');
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="form-stack">
      <div>
        <p className="muted text-xs">Tasa vigente del establecimiento</p>
        <p className="tax-rate-current">
          {settings.tax_rate} % · {taxRateLabels[settings.tax_rate]}
        </p>
      </div>
      <p className="muted text-sm">
        Los precios ya incluyen IVA; el impuesto se desglosa del total. Cambiar la tasa solo afecta
        a los pedidos que se confirmen después del cambio y nunca recalcula cuentas, pagos ni
        tickets anteriores.
      </p>
      <SegmentedControl<TaxRate>
        label="Tasa de IVA"
        value={selected}
        onChange={setSelected}
        disabled={!editable || busy || saving}
        options={rates.map((rate) => ({ value: rate, label: `${rate} %` }))}
      />
      {editable ? (
        <div>
          <button
            type="button"
            className="primary-button"
            disabled={busy || saving || selected === settings.tax_rate}
            onClick={() => void save()}
          >
            {saving ? 'Guardando…' : `Guardar tasa de ${selected} %`}
          </button>
        </div>
      ) : (
        <p className="muted text-sm">
          Cambiar la tasa requiere el permiso «{permissionName('payments.tax.manage')}».
        </p>
      )}
    </div>
  );
}
