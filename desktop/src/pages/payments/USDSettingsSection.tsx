import { useState } from 'react';
import { Field } from '../../components/Forms';
import { acceptAmountText, toWireAmount } from '../../services/amountInput';
import { formatMoney } from '../../services/money';
import { notifyError, notifySuccess } from '../../services/notify';
import { permissionName } from '../../services/payments';
import type { PaymentSettings, USDSettingsInput } from '../../services/payments';
import type { ExecuteMutation } from '../restaurant/OperationsAdmin';

// Rango aceptable del tipo de cambio y porcentaje de diferencia contra el turno anterior que exige
// confirmación al capturarlo. Solo se muestra con el pago en dólares activo; Go valida y rechaza
// estos datos si la función está desactivada.
export function USDSettingsSection({
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
  const [minimum, setMinimum] = useState(settings.usd_rate_min);
  const [maximum, setMaximum] = useState(settings.usd_rate_max);
  const [alert, setAlert] = useState(settings.usd_rate_alert_percent);
  const [saving, setSaving] = useState(false);
  const editable = can('payments.settings.manage');

  async function save() {
    setSaving(true);
    try {
      const body: USDSettingsInput = {
        revision: settings.revision,
        usd_rate_min: toWireAmount(minimum),
        usd_rate_max: toWireAmount(maximum),
        usd_rate_alert_percent: toWireAmount(alert),
      };
      const saved = await execute<PaymentSettings>('payments_save_usd_settings', {
        body: { ...body },
      });
      notifySuccess(
        `Rango guardado: de ${formatMoney(saved.usd_rate_min)} a ${formatMoney(saved.usd_rate_max)} por dólar; confirmación si difiere más de ${saved.usd_rate_alert_percent} %.`,
      );
    } catch (problem) {
      notifyError(problem, 'No se pudo guardar el rango.');
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="form-stack">
      <p className="muted text-sm">
        El tipo de cambio se captura al abrir cada turno y queda fijo durante el turno. Fuera de
        este rango se rechaza; si difiere del turno anterior más del porcentaje indicado, se pide
        una confirmación adicional. Las devoluciones se registran siempre en pesos.
      </p>
      <div className="form-grid">
        <Field label="Tipo de cambio mínimo (pesos por dólar)">
          <input
            type="text"
            inputMode="decimal"
            autoComplete="off"
            value={minimum}
            disabled={!editable || busy || saving}
            onChange={(event) => setMinimum(acceptAmountText(event.target.value, minimum))}
          />
        </Field>
        <Field label="Tipo de cambio máximo (pesos por dólar)">
          <input
            type="text"
            inputMode="decimal"
            autoComplete="off"
            value={maximum}
            disabled={!editable || busy || saving}
            onChange={(event) => setMaximum(acceptAmountText(event.target.value, maximum))}
          />
        </Field>
        <Field
          label="Diferencia que pide confirmación (%)"
          hint="Contra el último tipo de cambio capturado en otro turno."
        >
          <input
            type="text"
            inputMode="decimal"
            autoComplete="off"
            value={alert}
            disabled={!editable || busy || saving}
            onChange={(event) => setAlert(acceptAmountText(event.target.value, alert))}
          />
        </Field>
      </div>
      {editable ? (
        <div>
          <button
            type="button"
            className="primary-button"
            disabled={busy || saving}
            onClick={() => void save()}
          >
            {saving ? 'Guardando…' : 'Guardar rango del tipo de cambio'}
          </button>
        </div>
      ) : (
        <p className="muted text-sm">
          Cambiar el rango requiere el permiso «{permissionName('payments.settings.manage')}».
        </p>
      )}
    </div>
  );
}
