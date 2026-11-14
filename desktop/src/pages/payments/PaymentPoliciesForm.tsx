import { useEffect, useState } from 'react';
import { Field, Form, Notice } from '../../components/Forms';
import { Check } from '../../components/catalog/EditorFields';
import { useResource } from '../../hooks/useResource';
import { acceptAmountText, toWireAmount } from '../../services/amountInput';
import { Select } from '../../components/Select';
import type { SelectOption } from '../../components/selectModel';
import { precheckNames } from '../../services/documentNames';
import { printerLabel } from '../../services/kitchen';
import type { Printer } from '../../services/kitchen';
import { permissionName } from '../../services/payments';
import type { PaymentSettings, SettingsInput } from '../../services/payments';
import type { ExecuteMutation } from '../restaurant/OperationsAdmin';

type Policies = ReturnType<typeof initialPolicies>;

// Compara solo los valores editables, sin la revisión.
function sameValues(left: Policies, right: Policies) {
  return (
    left.shared_shifts === right.shared_shifts &&
    left.print_receipt_default === right.print_receipt_default &&
    left.receipt_printer_id === right.receipt_printer_id &&
    left.threshold === right.threshold &&
    sameRegisterPrinters(left.register_printers, right.register_printers)
  );
}

function sameRegisterPrinters(left: Record<string, string>, right: Record<string, string>) {
  const keys = new Set([...Object.keys(left), ...Object.keys(right)]);
  return [...keys].every((key) => (left[key] ?? '') === (right[key] ?? ''));
}

// Impresora de tickets elegida para cada caja (etapa 9G, fase E, D10): vacío = la impresora general.
function initialRegisterPrinters(settings: PaymentSettings) {
  const chosen: Record<string, string> = {};
  for (const register of settings.register_printers ?? []) {
    chosen[register.register_id] = register.printer_id ?? '';
  }
  return chosen;
}

function initialPolicies(settings: PaymentSettings) {
  return {
    revision: settings.revision,
    shared_shifts: settings.shared_shifts,
    print_receipt_default: settings.print_receipt_default,
    receipt_printer_id: settings.receipt_printer_id,
    threshold: settings.refund_authorization_threshold ?? '',
    register_printers: initialRegisterPrinters(settings),
  };
}

// Impresoras que se ofrecen como impresora de tickets (revisión 9F, B3): solo las que pueden imprimir
// precuentas, tickets y documentos de caja (decisión D4). Las desactivadas y las de otra terminal sin terminal
// elegida se muestran deshabilitadas con el motivo; la que ya estaba elegida se conserva visible aunque haya
// dejado de servir (Go permite guardar lo demás sin cambiarla). Desde la etapa 9G (fase E) la impresora USB de
// una terminal sirve y dice en qué terminal está.
export function receiptPrinterOptions(printers: Printer[], empty = 'Sin asignar'): SelectOption[] {
  const options: SelectOption[] = [{ value: '', label: empty }];
  for (const printer of printers) {
    let reason = '';
    if (printer.transport === 'agent' && !printer.agent_device_id) {
      reason = 'Esta impresora todavía no puede usarse para imprimir';
    } else if (!printer.active) {
      reason = 'desactivada';
    }
    options.push({
      value: printer.id,
      label: printerLabel(printer),
      description: reason || undefined,
      disabled: reason !== '',
    });
  }
  return options;
}

// Políticas de cobro: turno compartido, impresión predeterminada, impresora y umbral de autorización de
// devoluciones. La revisión de partida evita sobrescribir cambios ajenos. El nombre visible de la precuenta
// (etapa 9F, §4.5) se elige en Administración › Documentos desde la revisión 9F (M3); aquí solo se informa.
export function PaymentPoliciesForm({
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
  const [value, setValue] = useState(() => initialPolicies(settings));
  const [base, setBase] = useState(settings);
  const dirty = !sameValues(value, initialPolicies(base));
  const editable = can('payments.settings.manage');
  const printersAllowed = can('kitchen.printers.manage');
  const printers = useResource<Printer[]>('kitchen_printers', { enabled: printersAllowed });
  const stale = settings.revision !== value.revision;

  // Sin cambios locales, el formulario sigue la versión vigente (por ejemplo, tras guardar el IVA).
  useEffect(() => {
    if (stale && !dirty) {
      setBase(settings);
      setValue(initialPolicies(settings));
    }
  }, [stale, dirty, settings]);
  const printerOptions = receiptPrinterOptions(printers.data ?? []);

  return (
    <div className="form-stack">
      {!editable && (
        <p className="muted text-sm">
          Consulta disponible. Cambiar las políticas requiere el permiso «
          {permissionName('payments.settings.manage')}».
        </p>
      )}
      {stale && (
        <Notice>
          La configuración de cobro cambió en otra terminal mientras editabas.{' '}
          <button
            type="button"
            className="secondary-button"
            onClick={() => {
              setBase(settings);
              setValue(initialPolicies(settings));
            }}
          >
            Cargar versión actual
          </button>
        </Notice>
      )}
      <Form
        label="Guardar políticas"
        successMessage="Políticas de cobro guardadas."
        disabled={!editable || busy || stale}
        onSave={async () => {
          const threshold = toWireAmount(value.threshold);
          const body: SettingsInput = {
            revision: value.revision,
            shared_shifts: value.shared_shifts,
            print_receipt_default: value.print_receipt_default,
            receipt_printer_id: value.receipt_printer_id,
            refund_authorization_threshold: threshold === '' ? null : threshold,
          };
          // Etapa 9G (fase E): la impresora de tickets de cada caja solo se envía si se puede elegir.
          if (printersAllowed) {
            body.register_printers = Object.entries(value.register_printers).map(
              ([register_id, printer_id]) => ({ register_id, printer_id: printer_id || null }),
            );
          }
          const saved = await execute<PaymentSettings>('payments_save_settings', {
            body: { ...body },
          });
          setBase(saved);
          setValue(initialPolicies(saved));
        }}
      >
        <Check
          label="Permitir que varios usuarios cobren en el mismo turno de caja"
          value={value.shared_shifts}
          onChange={(shared_shifts) => setValue({ ...value, shared_shifts })}
        />
        <Check
          label="Proponer la impresión del ticket de pago de forma predeterminada"
          value={value.print_receipt_default}
          onChange={(print_receipt_default) => setValue({ ...value, print_receipt_default })}
        />
        <div className="form-grid">
          <Field
            label="Umbral de autorización de devoluciones (opcional)"
            hint="Importe en pesos con hasta dos decimales. Vacío: sin umbral configurado."
          >
            <input
              type="text"
              inputMode="decimal"
              autoComplete="off"
              value={value.threshold}
              onChange={(event) =>
                setValue({
                  ...value,
                  threshold: acceptAmountText(event.target.value, value.threshold),
                })
              }
            />
          </Field>
          {printersAllowed ? (
            <Field
              label="Impresora de tickets"
              hint={`Imprime ${precheckNames[settings.precheck_name]?.plural ?? 'precuentas'}, tickets de pago y documentos de caja. Solo se ofrecen las impresoras activas que ya pueden imprimir.`}
            >
              <Select
                value={value.receipt_printer_id ?? ''}
                placeholder="Sin asignar"
                options={printerOptions}
                onChange={(id) => setValue({ ...value, receipt_printer_id: id || null })}
              />
            </Field>
          ) : (
            <Field
              label="Impresora de tickets"
              hint={`Elegirla requiere el permiso «${permissionName('kitchen.printers.manage')}».`}
            >
              <input readOnly value={settings.receipt_printer_name ?? 'Sin asignar'} />
            </Field>
          )}
        </div>
        <RegisterPrinters
          settings={settings}
          chosen={value.register_printers}
          printers={printers.data ?? []}
          editable={printersAllowed}
          onChange={(register_printers) => setValue({ ...value, register_printers })}
        />
        <p className="muted text-sm">
          {`Nombre de la precuenta: «${(precheckNames[settings.precheck_name] ?? precheckNames.precheck).label}». Se elige en Administración › Documentos, en «Identidad del establecimiento».`}
        </p>
        <Notice>{printers.error}</Notice>
      </Form>
    </div>
  );
}

// Impresora de tickets por caja (etapa 9G, fase E, D10): cada caja puede tener su propia impresora de
// precuentas, tickets de pago y documentos de caja (por ejemplo, la USB de la Caja 2, que imprime esa terminal);
// sin una propia, la caja usa la impresora de tickets general. Los documentos salen en la de la caja de la
// terminal que los pide.
function RegisterPrinters({
  settings,
  chosen,
  printers,
  editable,
  onChange,
}: {
  settings: PaymentSettings;
  chosen: Record<string, string>;
  printers: Printer[];
  editable: boolean;
  onChange: (next: Record<string, string>) => void;
}) {
  const registers = settings.register_printers ?? [];
  if (registers.length === 0) {
    return null;
  }
  const options = receiptPrinterOptions(printers, 'Usar la impresora de tickets general');
  return (
    <fieldset className="register-printers">
      <legend>Impresora de tickets por caja</legend>
      <p className="muted text-sm">
        Cada caja puede imprimir en su propia impresora, por ejemplo la impresora USB conectada a la
        terminal de esa caja. Sin una propia, usa la impresora de tickets general.
      </p>
      <div className="form-grid">
        {registers.map((register) =>
          editable ? (
            <Field
              key={register.register_id}
              label={`Impresora propia de ${register.register_name}`}
            >
              <Select
                value={chosen[register.register_id] ?? ''}
                placeholder="Usar la impresora de tickets general"
                options={options}
                onChange={(id) => onChange({ ...chosen, [register.register_id]: id })}
              />
            </Field>
          ) : (
            <Field
              key={register.register_id}
              label={`Impresora propia de ${register.register_name}`}
            >
              <input
                readOnly
                value={
                  register.printer_name
                    ? printerLabel({
                        name: register.printer_name,
                        terminal_name: register.terminal_name,
                      })
                    : 'La impresora de tickets general'
                }
              />
            </Field>
          ),
        )}
      </div>
    </fieldset>
  );
}
