import { Accordion } from '../../components/Accordion';
import { Notice, PageTitle } from '../../components/Forms';
import { useResource } from '../../hooks/useResource';
import { useRestaurantMutation } from '../../hooks/useRestaurantMutation';
import { APIError } from '../../services/api';
import { featureEnabled } from '../../services/features';
import { formatMoney } from '../../services/money';
import { notifyError, notifySuccess } from '../../services/notify';
import type { EstablishmentFeature } from '../../services/features';
import { permissionName } from '../../services/payments';
import type { DiscountReason, PaymentMethod, PaymentSettings } from '../../services/payments';
import { countLabel } from '../../services/plural';
import type { ExecuteMutation } from '../restaurant/OperationsAdmin';
import { DiscountReasonsAdmin } from './DiscountReasonsAdmin';
import { MethodsAdmin } from './MethodsAdmin';
import { PaymentPoliciesForm } from './PaymentPoliciesForm';
import { ShiftOverview } from './ShiftOverview';
import { TaxRateSection } from './TaxRateSection';
import { USDSettingsSection } from './USDSettingsSection';

export const paymentsConfigurationKey = 'payments:configuration';

function changed() {
  window.dispatchEvent(new CustomEvent('adel-server-change'));
}

// Administración de cobros: métodos de pago, motivos de descuento, IVA, políticas y turno actual
// con sus movimientos.
export function PaymentsAdminPage({
  can,
  timezone,
}: {
  can: (permission: string) => boolean;
  timezone: string;
}) {
  const readable = can('payments.read');
  const settings = useResource<PaymentSettings>('payments_settings', { enabled: readable });
  const methods = useResource<PaymentMethod[]>('payments_methods', { enabled: readable });
  const reasons = useResource<DiscountReason[]>('payments_discount_reasons', {
    enabled: readable,
  });
  // El rango del tipo de cambio solo aparece con el pago en dólares activo.
  const features = useResource<EstablishmentFeature[]>('settings_features');
  const dollars = featureEnabled(features.data, 'usd_cash');
  const mutation = useRestaurantMutation(changed, paymentsConfigurationKey);
  const execute: ExecuteMutation = async (operation, options) => {
    try {
      return await mutation.execute(operation, options);
    } catch (problem) {
      // Una revisión obsoleta se resuelve consultando la versión vigente.
      if (problem instanceof APIError && problem.status === 409) {
        changed();
      }
      throw problem;
    }
  };
  const busy = mutation.busy || mutation.uncertain;

  // Reintento del cambio incierto: su resultado definitivo se avisa como el de cualquier acción.
  async function retry() {
    const problem = await mutation.retry();
    if (!problem) {
      notifySuccess('Cambio de configuración confirmado en el servidor.');
      return;
    }
    if (!mutation.isPending()) {
      notifyError(problem, 'No se pudo confirmar el cambio.');
    }
  }
  const activeMethods = methods.data?.filter((method) => method.active).length ?? 0;
  const activeReasons = reasons.data?.filter((reason) => reason.active).length ?? 0;

  return (
    <div className="payments-admin form-stack">
      <PageTitle
        title="Cobros"
        description="Métodos de pago, motivos de descuento, IVA, políticas de cobro y turno de caja. Los importes y las validaciones se calculan en el servidor."
      />
      {!readable && (
        <Notice>
          Consultar la configuración de cobro requiere el permiso «{permissionName('payments.read')}
          ».
        </Notice>
      )}
      {mutation.uncertain && (
        <div className="operation-recovery">
          <p>
            El resultado del último cambio no pudo confirmarse. Recupéralo con el mismo
            identificador antes de realizar otro.
          </p>
          <button
            type="button"
            className="primary-button"
            disabled={mutation.busy}
            onClick={() => void retry()}
          >
            Reintentar con el mismo identificador
          </button>
        </div>
      )}
      {readable && (
        <>
          <Accordion
            title="Métodos de pago"
            summary={
              methods.data
                ? `${countLabel(activeMethods, 'activo', 'activos')} de ${methods.data.length}`
                : 'Consultando métodos…'
            }
            defaultOpen
          >
            <Notice>{methods.error}</Notice>
            <MethodsAdmin methods={methods.data ?? []} can={can} execute={execute} busy={busy} />
          </Accordion>
          <Accordion
            title="Motivos de descuento"
            summary={
              reasons.data
                ? `${countLabel(activeReasons, 'activo', 'activos')} de ${reasons.data.length}`
                : 'Consultando motivos…'
            }
          >
            <Notice>{reasons.error}</Notice>
            <DiscountReasonsAdmin
              reasons={reasons.data ?? []}
              can={can}
              execute={execute}
              busy={busy}
            />
          </Accordion>
          <Accordion
            title="IVA"
            summary={
              settings.data
                ? `Tasa vigente: ${settings.data.tax_rate} %`
                : 'Consultando configuración…'
            }
          >
            <Notice>{settings.error}</Notice>
            {settings.data && (
              <TaxRateSection settings={settings.data} can={can} execute={execute} busy={busy} />
            )}
          </Accordion>
          {dollars && (
            <Accordion
              title="Pago en dólares"
              summary={
                settings.data
                  ? `Tipo de cambio de ${formatMoney(settings.data.usd_rate_min)} a ${formatMoney(settings.data.usd_rate_max)} · confirmación si difiere más de ${settings.data.usd_rate_alert_percent} %`
                  : 'Consultando configuración…'
              }
            >
              <Notice>{settings.error}</Notice>
              {settings.data && (
                <USDSettingsSection
                  key={settings.data.revision}
                  settings={settings.data}
                  can={can}
                  execute={execute}
                  busy={busy}
                />
              )}
            </Accordion>
          )}
          <Accordion
            title="Políticas de cobro"
            summary="Turno compartido, impresión de ticket, impresora y devoluciones"
          >
            <Notice>{settings.error}</Notice>
            {settings.data && (
              <PaymentPoliciesForm
                settings={settings.data}
                can={can}
                execute={execute}
                busy={busy}
              />
            )}
          </Accordion>
        </>
      )}
      <Accordion title="Turno actual" summary="Caja de esta terminal, movimientos y totales">
        <ShiftOverview can={can} timezone={timezone} />
      </Accordion>
    </div>
  );
}
