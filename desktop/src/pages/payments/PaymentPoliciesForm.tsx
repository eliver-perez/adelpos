import { useEffect, useState } from 'react';
import { Field, Form, Notice } from '../../components/Forms';
import { Check } from '../../components/catalog/EditorFields';
import { useResource } from '../../hooks/useResource';
import { acceptAmountText, toWireAmount } from '../../services/amountInput';
import { Select } from '../../components/Select';
import type { SelectOption } from '../../components/selectModel';
import { precheckNames } from '../../services/documentNames';
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
    left.threshold === right.threshold
  );
}

function initialPolicies(settings: PaymentSettings) {
  return {
    revision: settings.revision,
    shared_shifts: settings.shared_shifts,
    print_receipt_default: settings.print_receipt_default,
    receipt_printer_id: settings.receipt_printer_id,
    threshold: settings.refund_authorization_threshold ?? '',
  };
}

// Impresoras que se ofrecen como impresora de tickets (revisión 9F, B3): solo las que pueden imprimir
// precuentas, tickets y documentos de caja (decisión D4). Las desactivadas y las de otra terminal se
// muestran deshabilitadas con el motivo; la que ya estaba elegida se conserva visible aunque haya dejado
// de servir (Go permite guardar lo demás sin cambiarla).
export function receiptPrinterOptions(printers: Printer[]): SelectOption[] {
  const options: SelectOption[] = [{ value: '', label: 'Sin asignar' }];
  for (const printer of printers) {
    let reason = '';
    if (printer.transport === 'agent') {
      reason = 'Esta impresora todavía no puede usarse para imprimir';
    } else if (!printer.active) {
      reason = 'desactivada';
    }
    options.push({
      value: printer.id,
      label: printer.name,
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
        <p className="muted text-sm">
          {`Nombre de la precuenta: «${(precheckNames[settings.precheck_name] ?? precheckNames.precheck).label}». Se elige en Administración › Documentos, en «Identidad del establecimiento».`}
        </p>
        <Notice>{printers.error}</Notice>
      </Form>
    </div>
  );
}
