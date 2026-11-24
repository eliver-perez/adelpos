import { useState } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Notice, PageTitle } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { SegmentedControl } from '../../components/SegmentedControl';
import { useResource } from '../../hooks/useResource';
import {
  environmentLabel,
  expiryText,
  licenseDate,
  licenseSeats,
  licenseStateExplanation,
  licenseStateLabels,
  licenseTypeLabel,
  notActivated,
  seatLabels,
} from '../../services/license';
import type { LicenseStatus, LicenseSummary } from '../../services/license';
import {
  deactivateLicense,
  importLicenseFile,
  refreshLicense,
  saveOfflineRequest,
} from '../../services/licenseOperations';
import type { OfflineAction } from '../../services/licenseOperations';
import type { SessionsSummary } from '../../services/models';
import { notifyError, notifySuccess } from '../../services/notify';
import { countLabel } from '../../services/plural';
import { ActivationPanel } from './ActivationPanel';

// Configuración › Licencia (etapa L1, Fase E; §7 de la especificación). Ver el estado no pide permiso (L1-D21); los
// botones, solo con «Administrar la licencia» (Go lo vuelve a exigir). Los cupos en uso salen de sessions-summary, que
// pide «Ver terminales».

const offlineActionLabels: Record<OfflineAction, string> = {
  activate: 'Activación',
  renew: 'Renovación',
  deactivate: 'Liberación',
};

const offlineActionHints: Record<OfflineAction, string> = {
  activate: 'Para activar esta computadora sin internet.',
  renew: 'Para recibir la licencia renovada sin internet.',
  deactivate: 'Para liberar la licencia de esta computadora sin internet.',
};

// hasActivation: hay una activación que se puede consultar, renovar o liberar.
function hasActivation(status: LicenseStatus) {
  return status.state === 'active' || status.state === 'grace' || status.state === 'expired';
}

export function LicensePage({
  timezone,
  mainComputer,
  canReadDevices,
  summary,
  onChanged,
}: {
  timezone: string;
  mainComputer: boolean;
  canReadDevices: boolean;
  // El estado de la sesión (Fase C), con los días ya calculados.
  summary?: LicenseSummary;
  // Vuelve a leer la sesión (avisos, bloqueo y estado de la licencia).
  onChanged: () => Promise<void>;
}) {
  const resource = useResource<LicenseStatus>('license_status');
  const seats = useResource<SessionsSummary>('devices_sessions_summary', {
    enabled: canReadDevices,
  });
  const status = resource.data;

  async function changed() {
    await Promise.all([resource.reload(), seats.reload(), onChanged()]);
  }

  return (
    <>
      <PageTitle
        title="Licencia"
        description="El estado de la licencia de este sistema, su plan y sus cupos."
      />
      <Notice>{resource.error}</Notice>
      {status && (
        <>
          {(notActivated(status.state) || status.state === 'revoked') && (
            <ActivationPanel status={status} mainComputer={mainComputer} onChanged={changed} />
          )}
          <LicenseDetailsPanel
            status={status}
            summary={summary}
            timezone={timezone}
            seats={canReadDevices ? seats.data : null}
          />
          {status.can_manage ? (
            <LicenseActions status={status} onChanged={changed} />
          ) : (
            <p className="muted text-sm">
              Cambiar la licencia requiere el permiso «Administrar la licencia».
            </p>
          )}
        </>
      )}
    </>
  );
}

function LicenseDetailsPanel({
  status,
  summary,
  timezone,
  seats,
}: {
  status: LicenseStatus;
  summary?: LicenseSummary;
  timezone: string;
  seats: SessionsSummary | null;
}) {
  const license = status.license;
  const environment = environmentLabel(status.environment);
  return (
    <section className="surface panel form-stack" aria-labelledby="license-details-title">
      <h2 id="license-details-title">Estado de la licencia</h2>
      <p className={`license-state license-state-${status.state}`}>
        <strong>{licenseStateLabels[status.state]}</strong>
        <span>
          {licenseStateExplanation({
            ...status,
            grace_days_remaining: summary?.grace_days_remaining ?? null,
          })}
        </span>
      </p>
      <dl className="license-facts">
        {license && (
          <>
            <div>
              <dt>Plan</dt>
              <dd>
                {license.plan.custom ? `${license.plan.name} (personalizado)` : license.plan.name}
              </dd>
            </div>
            <div>
              <dt>Cliente</dt>
              <dd>{license.licensee_name}</dd>
            </div>
            <div>
              <dt>Tipo</dt>
              <dd>{licenseTypeLabel(license.license_type, license.billing_period)}</dd>
            </div>
            <div>
              <dt>Vencimiento</dt>
              <dd>{expiryText(license, timezone)}</dd>
            </div>
            <div>
              <dt>Días de tolerancia</dt>
              <dd>
                {license.perpetual ? 'No aplica' : countLabel(license.grace_days, 'día', 'días')}
              </dd>
            </div>
            {license.grace_ends_at && !license.perpetual && (
              <div>
                <dt>Fin de la tolerancia</dt>
                <dd>{licenseDate(license.grace_ends_at, timezone)}</dd>
              </div>
            )}
          </>
        )}
        {environment && (
          <div>
            <dt>Entorno</dt>
            <dd>{environment}</dd>
          </div>
        )}
      </dl>
      {license && (
        <div className="table-container">
          <table aria-label="Cupos de la licencia">
            <thead>
              <tr>
                <th>Cupo</th>
                {seats?.seats && <th>En uso</th>}
                <th>Límite</th>
              </tr>
            </thead>
            <tbody>
              {licenseSeats.map((seat) => (
                <tr key={seat}>
                  <td>{seatLabels[seat]}</td>
                  {seats?.seats && <td>{seats.seats[seat]?.in_use ?? 0}</td>}
                  <td>{license.limits[seat]}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function LicenseActions({
  status,
  onChanged,
}: {
  status: LicenseStatus;
  onChanged: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const active = hasActivation(status);
  const [chosen, setAction] = useState<OfflineAction>(active ? 'renew' : 'activate');
  // Activar solo sin una activación; renovar y liberar, solo con ella (Go lo vuelve a exigir).
  const allowed = (value: OfflineAction) => (value === 'activate' ? !active : active);
  const action: OfflineAction = allowed(chosen) ? chosen : active ? 'renew' : 'activate';
  const { confirm, dialog } = useConfirmDialog();

  async function run(task: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    try {
      await task();
    } finally {
      setBusy(false);
    }
  }

  async function refresh() {
    await run(async () => {
      try {
        await refreshLicense();
        notifySuccess('Licencia consultada: tiene la información más reciente.');
        await onChanged();
      } catch (error) {
        notifyError(error, 'No se pudo buscar la actualización de la licencia.');
      }
    });
  }

  async function release() {
    const confirmed = await confirm({
      title: 'Liberar esta licencia',
      message: (
        <>
          <p>
            La licencia queda libre para activarla en otra computadora. Esta computadora deja de
            poder capturar pedidos; lo registrado se conserva y se sigue cobrando lo que está
            abierto.
          </p>
          <p>Para volver a usarla aquí, hay que activarla otra vez con su código.</p>
        </>
      ),
      confirmLabel: 'Liberar esta licencia',
      cancelLabel: 'Conservar la licencia',
      tone: 'danger',
    });
    if (!confirmed) return;
    await run(async () => {
      try {
        await deactivateLicense();
        notifySuccess('La licencia se liberó de esta computadora.');
        await onChanged();
      } catch (error) {
        notifyError(error, 'No se pudo liberar la licencia.');
      }
    });
  }

  async function saveRequest() {
    await run(async () => {
      try {
        const saved = await saveOfflineRequest(action);
        if (saved.result === 'saved') {
          notifySuccess(`Solicitud guardada: ${saved.file_name}.`);
        }
      } catch (error) {
        notifyError(error, 'No se pudo guardar la solicitud.');
      }
    });
  }

  async function importFile() {
    await run(async () => {
      try {
        const imported = await importLicenseFile();
        if (imported.result === 'imported') {
          notifySuccess('Archivo de licencia importado.');
          await onChanged();
        }
      } catch (error) {
        notifyError(error, 'No se pudo importar el archivo de licencia.');
      }
    });
  }

  const offlineOptions = (['activate', 'renew', 'deactivate'] as OfflineAction[]).map((value) => ({
    value,
    label: offlineActionLabels[value],
    disabled: !allowed(value),
  }));

  return (
    <section className="surface panel form-stack" aria-labelledby="license-actions-title">
      <h2 id="license-actions-title">Administrar la licencia</h2>
      <div className="button-row">
        {active && (
          <button
            type="button"
            className="secondary-button"
            disabled={busy}
            onClick={() => void refresh()}
          >
            <IconLabel icon="refresh">Buscar actualización</IconLabel>
          </button>
        )}
        <button
          type="button"
          className="secondary-button"
          disabled={busy}
          onClick={() => void importFile()}
        >
          <IconLabel icon="arrow">Importar archivo de licencia</IconLabel>
        </button>
        {active && (
          <button
            type="button"
            className="danger-button"
            disabled={busy}
            onClick={() => void release()}
          >
            Liberar esta licencia
          </button>
        )}
      </div>
      <div
        className="license-offline form-stack"
        role="group"
        aria-labelledby="license-offline-title"
      >
        <h3 id="license-offline-title">Solicitud sin internet</h3>
        <p className="muted text-sm">
          Guarda la solicitud y envíala a soporte por correo o en una USB. Te responderán con un
          archivo de licencia para importarlo aquí.
        </p>
        <SegmentedControl
          label="Qué pedir"
          value={action}
          options={offlineOptions}
          disabled={busy}
          onChange={setAction}
        />
        <p className="muted text-sm">{offlineActionHints[action]}</p>
        <div className="button-row">
          <button
            type="button"
            className="secondary-button"
            disabled={busy}
            onClick={() => void saveRequest()}
          >
            <IconLabel icon="document">Guardar la solicitud</IconLabel>
          </button>
        </div>
      </div>
      {dialog}
    </section>
  );
}
