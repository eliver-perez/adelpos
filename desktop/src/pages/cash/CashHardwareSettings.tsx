import { useId, useState } from 'react';
import { Field, Form, Notice } from '../../components/Forms';
import { SegmentedControl } from '../../components/SegmentedControl';
import { ToggleButton } from '../../components/ToggleButton';
import { useLiveResource } from '../../hooks/useLiveResource';
import { acceptAmountText, toWireAmount } from '../../services/amountInput';
import type { Operation } from '../../services/api';
import type { CashLimitSettings, DrawerPrinter, DrawerSettings } from '../../services/cash';
import { permissionName } from '../../services/payments';

type Execute = <T>(
  operation: Operation,
  options: { id?: string; body?: Record<string, unknown> },
) => Promise<T>;

// Importe de un tope tal como lo guarda Go, editable como texto; null significa sin tope.
function editableAmount(value: string | null) {
  return value ?? '';
}

// Tope de caja (función activable cash_limit). Es global: Go lo compara con el efectivo esperado de
// cada caja por separado y solo avisa a quien puede consultar el esperado; nunca bloquea cobros.
export function CashLimitAdmin({ execute }: { execute: Execute }) {
  const limit = useLiveResource<CashLimitSettings>('payments_cash_limit', { interval: 60000 });
  if (limit.error) {
    return <Notice>{limit.error}</Notice>;
  }
  if (!limit.data) {
    return <p className="muted">Consultando el tope de caja…</p>;
  }
  if (!limit.data.enabled) {
    return (
      <p className="muted text-sm">
        El tope de caja es una función activable y está desactivada: no tiene ningún efecto. Se
        activa en Configuración → Funciones del establecimiento con el permiso «
        {permissionName('settings.features.manage')}».
      </p>
    );
  }
  return (
    <CashLimitForm
      key={`${limit.data.revision}-${limit.data.usd_cash}`}
      settings={limit.data}
      execute={execute}
    />
  );
}

function CashLimitForm({ settings, execute }: { settings: CashLimitSettings; execute: Execute }) {
  const [mxn, setMXN] = useState(editableAmount(settings.cash_limit_mxn));
  const [usd, setUSD] = useState(editableAmount(settings.cash_limit_usd));
  return (
    <Form
      label="Guardar tope de caja"
      onSave={async () => {
        const body: Record<string, unknown> = {
          revision: settings.revision,
          cash_limit_mxn: toWireAmount(mxn),
        };
        if (settings.usd_cash) {
          body.cash_limit_usd = toWireAmount(usd);
        }
        await execute('payments_save_cash_limit', { body });
      }}
    >
      <p className="muted text-sm">
        Tope global que se aplica a cada caja por separado. Cuando el efectivo esperado de una caja
        lo supera, quienes pueden consultar el esperado ven un aviso que sugiere un retiro. Nunca
        bloquea cobros. Déjalo vacío para no usar tope.
      </p>
      <div className="form-grid">
        <Field label="Tope en pesos" hint="Vacío: sin tope en pesos.">
          <input
            type="text"
            inputMode="decimal"
            autoComplete="off"
            placeholder="Sin tope"
            value={mxn}
            onChange={(event) => setMXN(acceptAmountText(event.target.value, mxn))}
          />
        </Field>
        {settings.usd_cash && (
          <Field label="Tope en dólares" hint="Vacío: sin tope en dólares.">
            <input
              type="text"
              inputMode="decimal"
              autoComplete="off"
              placeholder="Sin tope"
              value={usd}
              onChange={(event) => setUSD(acceptAmountText(event.target.value, usd))}
            />
          </Field>
        )}
      </div>
    </Form>
  );
}

// Cajón de dinero por caja física. Solo se admiten impresoras ESC/POS por red, que entregan el
// comando de apertura sin alterar; Go lo valida al guardar y al enviar cada comando.
export function DrawerAdmin({ execute }: { execute: Execute }) {
  const drawers = useLiveResource<{ drawers: DrawerSettings[]; printers: DrawerPrinter[] }>(
    'payments_cash_drawers',
    { interval: 60000 },
  );
  if (drawers.error) {
    return <Notice>{drawers.error}</Notice>;
  }
  if (!drawers.data) {
    return <p className="muted">Consultando los cajones…</p>;
  }
  const { printers } = drawers.data;
  return (
    <div className="form-stack">
      <p className="muted text-sm">
        El cajón se abre con un comando ESC/POS que envía la impresora a la que está conectado. Solo
        sirven impresoras ESC/POS por red: CUPS altera los bytes y el agente de impresión aún no
        existe. Con el cajón activado se abre tras cobros y devoluciones en efectivo e ingresos,
        retiros y gastos; nunca con pagos electrónicos. La pantalla indica que el comando se envió,
        no que el cajón se abrió.
      </p>
      {printers.every((printer) => !printer.compatible) && (
        <Notice>
          No hay impresoras ESC/POS por red registradas. Regístralas en Cocina y preparación →
          Impresoras.
        </Notice>
      )}
      {drawers.data.drawers.map((drawer) => (
        <DrawerEditor
          key={`${drawer.register_id}-${drawer.revision}`}
          drawer={drawer}
          printers={printers}
          execute={execute}
        />
      ))}
    </div>
  );
}

function DrawerEditor({
  drawer,
  printers,
  execute,
}: {
  drawer: DrawerSettings;
  printers: DrawerPrinter[];
  execute: Execute;
}) {
  const printerID = useId();
  const [printer, setPrinter] = useState(drawer.printer_id ?? '');
  const [pin, setPin] = useState<'0' | '1'>(drawer.pulse_pin === 1 ? '1' : '0');
  const [enabled, setEnabled] = useState(drawer.enabled);
  return (
    <Form
      label={`Guardar cajón de ${drawer.register_name}`}
      onSave={async () => {
        await execute('payments_save_cash_drawer', {
          id: drawer.register_id,
          body: {
            revision: drawer.revision,
            printer_id: printer || null,
            pulse_pin: Number(pin),
            enabled,
          },
        });
      }}
    >
      <h4>{drawer.register_name}</h4>
      <div className="form-grid">
        <div className="field">
          <label htmlFor={printerID}>Impresora del cajón</label>
          <select
            id={printerID}
            value={printer}
            onChange={(event) => setPrinter(event.target.value)}
          >
            <option value="">Sin impresora</option>
            {printers.map((option) => (
              <option
                key={option.id}
                value={option.id}
                disabled={!option.compatible || !option.active}
              >
                {option.name}
                {!option.compatible ? ' · no compatible (no es ESC/POS por red)' : ''}
                {option.compatible && !option.active ? ' · desactivada' : ''}
              </option>
            ))}
          </select>
        </div>
        <div className="field">
          <span>Conector del pulso</span>
          <SegmentedControl<'0' | '1'>
            label={`Conector del cajón de ${drawer.register_name}`}
            value={pin}
            onChange={setPin}
            options={[
              { value: '0', label: 'Pin 2' },
              { value: '1', label: 'Pin 5' },
            ]}
          />
        </div>
        <div className="field">
          <span>Estado</span>
          <ToggleButton
            label={enabled ? 'Cajón activado' : 'Cajón desactivado'}
            pressed={enabled}
            onChange={setEnabled}
          />
        </div>
      </div>
    </Form>
  );
}
