import { useId, useState } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Form, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { SegmentedControl } from '../../components/SegmentedControl';
import { ToggleButton } from '../../components/ToggleButton';
import { useLiveResource } from '../../hooks/useLiveResource';
import { acceptAmountText, toWireAmount } from '../../services/amountInput';
import type { Operation } from '../../services/api';
import { drawerConnectionLabel } from '../../services/cash';
import type {
  CashLimitSettings,
  DrawerOutcome,
  DrawerPrinter,
  DrawerSettings,
} from '../../services/cash';
import { permissionName } from '../../services/payments';
import { notifyDrawerOutcome } from './CashNotices';
import { PendingCashOperation } from './PendingCashOperation';
import { useCashMutation } from './useCashMutation';
import { Select } from '../../components/Select';

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
      successMessage="Tope de caja guardado."
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

// Cajón de dinero por caja física. Se admiten impresoras ESC/POS por red y, desde la etapa 7.1
// (modo mixto), colas CUPS que admitan datos raw (USB, puerto serie o AppSocket): Go consulta la
// cola con lpstat y lpoptions, sin imprimir nada, y explica por qué una cola no sirve. Lo valida al
// guardar y otra vez antes de enviar cada comando.
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
        El cajón se abre con un comando ESC/POS que envía la impresora a la que está conectado: una
        impresora ESC/POS por red o una cola CUPS (por ejemplo, una impresora USB) que admita datos
        raw. El servidor consulta cada cola sin imprimir nada y explica por qué una no sirve. Con el
        cajón activado se abre tras cobros y devoluciones en efectivo e ingresos, retiros y gastos;
        nunca con pagos electrónicos. La pantalla indica que el comando se envió, no que el cajón se
        abrió.
      </p>
      {printers.every((printer) => !printer.compatible) && (
        <Notice>
          No hay impresoras compatibles con el cajón: registra una impresora ESC/POS por red o una
          cola CUPS con datos raw en Cocina y preparación → Impresoras.
        </Notice>
      )}
      <IncompatiblePrinters printers={printers} />
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

// Explica, sin ocultarlas, por qué las impresoras activas no sirven para el cajón (motivo de Go).
function IncompatiblePrinters({ printers }: { printers: DrawerPrinter[] }) {
  const rejected = printers.filter((printer) => printer.active && !printer.compatible);
  if (rejected.length === 0) {
    return null;
  }
  return (
    <div className="cash-terminology" aria-label="Impresoras que no sirven para el cajón">
      <strong>Impresoras que no sirven para el cajón</strong>
      <ul className="cash-detail-list">
        {rejected.map((printer) => (
          <li key={printer.id}>
            {printer.name} · {drawerConnectionLabel(printer)}: {printer.reason}
          </li>
        ))}
      </ul>
    </div>
  );
}

// Clave del almacén de pendientes de «Probar apertura» de una caja: una prueba incierta se
// reintenta con el mismo identificador y Go devuelve el mismo comando, sin otro pulso.
export function drawerTestPendingKey(registerID: string) {
  return `cash-drawer-test:${registerID}`;
}

// «Probar apertura» (etapa 7.1): envía un pulso con la configuración guardada, con
// cash.settings.manage, y Go lo audita con el motivo test. Se confirma antes porque el cajón se
// abre físicamente en su caja; el aviso solo informa que el comando se envió.
function DrawerTest({
  drawer,
  printerName,
  unsaved,
}: {
  drawer: DrawerSettings;
  printerName: string;
  unsaved: boolean;
}) {
  const { confirm, dialog } = useConfirmDialog();
  const mutation = useCashMutation(
    drawer.register_id,
    (_, result) => notifyDrawerOutcome(result as DrawerOutcome),
    drawerTestPendingKey(drawer.register_id),
  );
  const ready = !!drawer.printer_id && !unsaved && !mutation.locked;

  async function test() {
    if (!ready || mutation.isPending()) {
      return;
    }
    const accepted = await confirm({
      title: `¿Probar la apertura del cajón de ${drawer.register_name}?`,
      message: `Se enviará el comando de apertura a ${printerName}. Hazlo con alguien frente a la caja: la prueba queda registrada y auditada.`,
      confirmLabel: 'Enviar prueba de apertura',
      cancelLabel: 'No enviar',
    });
    if (!accepted) {
      return;
    }
    try {
      await mutation.execute<DrawerOutcome>('payments_test_cash_drawer', {
        id: drawer.register_id,
        body: {},
      });
    } catch (problem) {
      mutation.reportFailure(problem, 'No se pudo enviar la prueba de apertura del cajón.');
    }
  }

  return (
    <div className="form-stack">
      {dialog}
      <div className="button-row">
        <button
          type="button"
          className="secondary-button"
          disabled={!ready}
          onClick={() => void test()}
        >
          <IconLabel icon="drawer">
            {mutation.busy ? 'Enviando prueba…' : 'Probar apertura'}
          </IconLabel>
        </button>
      </div>
      {!drawer.printer_id && (
        <p className="muted text-xs">Guarda la impresora del cajón para probar la apertura.</p>
      )}
      {drawer.printer_id && unsaved && (
        <p className="muted text-xs">Guarda los cambios para probar la apertura con ellos.</p>
      )}
      <PendingCashOperation mutation={mutation} />
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
  const unsaved =
    printer !== (drawer.printer_id ?? '') ||
    pin !== (drawer.pulse_pin === 1 ? '1' : '0') ||
    enabled !== drawer.enabled;
  const selected = printers.find((option) => option.id === printer) ?? null;
  return (
    <div className="form-stack">
      <Form
        label={`Guardar cajón de ${drawer.register_name}`}
        successMessage={`Cajón de ${drawer.register_name} guardado.`}
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
            <Select
              id={printerID}
              value={printer}
              options={[
                { value: '', label: 'Sin impresora' },
                ...printers.map((option) => ({
                  value: option.id,
                  label: `${option.name} · ${drawerConnectionLabel(option)}${
                    !option.active ? ' · desactivada' : ''
                  }${option.active && !option.compatible ? ' · no compatible' : ''}`,
                  disabled: !option.compatible || !option.active,
                })),
              ]}
              onChange={setPrinter}
            />
            {selected && selected.active && !selected.compatible && (
              <small className="muted">{selected.reason}</small>
            )}
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
      <DrawerTest
        drawer={drawer}
        printerName={drawer.printer_name ?? 'la impresora del cajón'}
        unsaved={unsaved}
      />
    </div>
  );
}
