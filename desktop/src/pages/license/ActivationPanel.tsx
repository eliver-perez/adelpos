import { useId, useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { IconLabel } from '../../components/Icon';
import { APIError } from '../../services/api';
import {
  copiedInstallationMessage,
  formatLicenseKey,
  licenseKeyProblem,
} from '../../services/license';
import type { LicenseStatus } from '../../services/license';
import {
  activateLicense,
  importLicenseFile,
  regenerateIdentity,
  saveOfflineRequest,
  transferLicense,
} from '../../services/licenseOperations';
import { notifyError, notifySuccess } from '../../services/notify';
import { readServiceStatus, restartService } from '../../services/serverService';

// Activación de la licencia (etapa L1, Fase E; §7 de la especificación, contrato §8.3, §8.4 y §9). La usan la pantalla
// del primer arranque sin licencia y Configuración › Licencia.
//
// - El código se valida al escribirlo con el formato de L1-D4 (Go lo vuelve a validar).
// - «Transferir a esta computadora» aparece solo cuando activar respondió ACTIVATION_LIMIT con transfer_available, y se
//   pide con la confirmación del usuario. Sin transferencia disponible o con TRANSFER_LIMIT: «Comunícate con soporte».
// - «Activar sin internet»: guardar la solicitud .licreq (Rust, con el diálogo nativo) e importar el .lic.
// - La instalación copiada (L1-D11): el mensaje de la §1.1 y, solo en la computadora principal y con el permiso, crear
//   la identidad nueva con su explicación y su confirmación.
//
// Los textos de cada rechazo salen de services/license.ts (los pone services/api.ts).

export function ActivationPanel({
  status,
  mainComputer,
  onChanged,
}: {
  status: LicenseStatus;
  mainComputer: boolean;
  // Vuelve a leer la licencia (y la sesión) después de un cambio.
  onChanged: () => Promise<void>;
}) {
  const fieldID = useId();
  const problemID = useId();
  const [key, setKey] = useState('');
  const [touched, setTouched] = useState(false);
  const [busy, setBusy] = useState(false);
  // Activar respondió ACTIVATION_LIMIT con transferencia disponible para este código.
  const [transferKey, setTransferKey] = useState('');
  const [restart, setRestart] = useState<'none' | 'pending' | 'manual'>('none');
  const { confirm, dialog } = useConfirmDialog();
  const problem = licenseKeyProblem(key);
  const canManage = status.can_manage;

  async function run(action: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    try {
      await action();
    } finally {
      setBusy(false);
    }
  }

  async function activate() {
    setTouched(true);
    if (problem) return;
    await run(async () => {
      try {
        await activateLicense(key);
        setTransferKey('');
        setKey('');
        notifySuccess('Licencia activada.');
        await onChanged();
      } catch (error) {
        if (
          error instanceof APIError &&
          error.code === 'LICENSE_ACTIVATION_LIMIT' &&
          (error.details as { transfer_available?: unknown } | undefined)?.transfer_available ===
            true
        ) {
          setTransferKey(key);
          return;
        }
        setTransferKey('');
        notifyError(error, 'No se pudo activar la licencia.');
      }
    });
  }

  async function transfer() {
    const confirmed = await confirm({
      title: 'Transferir la licencia a esta computadora',
      message: (
        <>
          <p>
            La licencia está activa en otra computadora. Al transferirla, esa computadora deja de
            poder capturar pedidos en cuanto consulte la licencia con internet.
          </p>
          <p>
            Solo se puede hacer una transferencia por tu cuenta a la vez: otra antes de que la
            computadora anterior se entere necesita a soporte.
          </p>
        </>
      ),
      confirmLabel: 'Transferir a esta computadora',
      cancelLabel: 'No transferir',
    });
    if (!confirmed) return;
    await run(async () => {
      try {
        await transferLicense(transferKey);
        setTransferKey('');
        setKey('');
        notifySuccess('Licencia transferida a esta computadora.');
        await onChanged();
      } catch (error) {
        setTransferKey('');
        notifyError(error, 'No se pudo transferir la licencia.');
      }
    });
  }

  async function saveRequest() {
    await run(async () => {
      try {
        const saved = await saveOfflineRequest('activate', problem ? undefined : key);
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

  async function newIdentity() {
    const confirmed = await confirm({
      title: 'Crear una identidad nueva para esta computadora',
      message: (
        <>
          <p>
            Esta computadora recibe una identidad propia para activar o transferir la licencia aquí.
          </p>
          <p>
            Las terminales y los teléfonos emparejados por la red dejan de funcionar y hay que
            emparejarlos de nuevo. La identidad anterior se guarda aparte para soporte.
          </p>
          <p>No se borra ni cambia ningún dato del restaurante.</p>
        </>
      ),
      confirmLabel: 'Crear la identidad nueva',
      cancelLabel: 'No cambiar nada',
      tone: 'danger',
    });
    if (!confirmed) return;
    await run(async () => {
      try {
        const result = await regenerateIdentity();
        notifySuccess('Identidad nueva creada para esta computadora.');
        if (result.restart_required) {
          // La escucha de la red sigue con la identidad anterior hasta reiniciar el servidor.
          const service = await readServiceStatus().catch(() => null);
          setRestart(service?.restart_available ? 'pending' : 'manual');
        }
        await onChanged();
      } catch (error) {
        notifyError(error, 'No se pudo crear la identidad nueva.');
      }
    });
  }

  async function restartServer() {
    await run(async () => {
      try {
        await restartService();
        setRestart('none');
        notifySuccess('El servidor se está reiniciando.');
      } catch (error) {
        notifyError(error, 'No se pudo reiniciar el servidor.');
      }
    });
  }

  if (!canManage) {
    return (
      <section className="surface panel form-stack" aria-labelledby={`${fieldID}-title`}>
        <h2 id={`${fieldID}-title`}>Activar la licencia</h2>
        {status.copied && <p className="license-copied">{copiedInstallationMessage}</p>}
        <p className="muted text-sm">
          Activar la licencia requiere el permiso «Administrar la licencia». Pide al administrador
          que la active.
        </p>
      </section>
    );
  }

  return (
    <section
      className="surface panel form-stack license-activation"
      aria-labelledby={`${fieldID}-title`}
    >
      <h2 id={`${fieldID}-title`}>Activar la licencia</h2>
      {status.copied && (
        <div className="license-copied" role="status">
          <strong>{copiedInstallationMessage}</strong>
          {status.identity_regeneration_available ? (
            <>
              <p>
                Para activarla o transferirla aquí, esta computadora necesita una identidad propia.
                Las terminales emparejadas por la red se emparejan de nuevo después; ningún dato del
                restaurante cambia.
              </p>
              <button
                type="button"
                className="danger-button"
                disabled={busy}
                onClick={() => void newIdentity()}
              >
                Crear una identidad nueva para esta computadora
              </button>
            </>
          ) : (
            <p>
              {mainComputer
                ? 'La identidad nueva la crea quien tiene el permiso «Administrar la licencia».'
                : 'La identidad nueva se crea en la computadora principal.'}
            </p>
          )}
        </div>
      )}
      {restart !== 'none' && (
        <div className="license-restart" role="status">
          <p>
            Para terminar, reinicia el servidor de esta computadora: así las terminales pueden
            emparejarse con la identidad nueva.
          </p>
          {restart === 'pending' ? (
            <button
              type="button"
              className="primary-button"
              disabled={busy}
              onClick={() => void restartServer()}
            >
              Reiniciar el servidor
            </button>
          ) : (
            <p className="muted text-sm">Reinicia esta computadora para terminar.</p>
          )}
        </div>
      )}
      <form
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          void activate();
        }}
      >
        <div className="field">
          <label htmlFor={fieldID}>Código de la licencia</label>
          <input
            id={fieldID}
            className="license-key-input"
            value={key}
            autoComplete="off"
            autoCapitalize="characters"
            spellCheck={false}
            maxLength={24}
            aria-describedby={problemID}
            aria-invalid={touched && problem !== null ? true : undefined}
            onChange={(event) => {
              setKey(formatLicenseKey(event.target.value));
              setTouched(true);
              setTransferKey('');
            }}
          />
          <small
            id={problemID}
            className={touched && problem ? 'field-problem' : 'muted'}
            aria-live="polite"
          >
            {touched && problem
              ? problem
              : problem
                ? '15 letras y números en grupos de 3, 4, 4 y 4.'
                : 'El código tiene el formato correcto.'}
          </small>
        </div>
        <div className="button-row">
          <button type="submit" className="primary-button" disabled={busy || problem !== null}>
            <IconLabel icon="key">{busy ? 'Activando…' : 'Activar'}</IconLabel>
          </button>
        </div>
      </form>
      {transferKey && (
        <div className="license-transfer" role="region" aria-label="Transferir la licencia">
          <p>
            La licencia ya está activa en otra computadora. Puedes transferirla a esta computadora.
          </p>
          <button
            type="button"
            className="primary-button"
            disabled={busy}
            onClick={() => void transfer()}
          >
            <IconLabel icon="transfer">Transferir a esta computadora</IconLabel>
          </button>
        </div>
      )}
      <Accordion
        title="Activar sin internet"
        summary="Con una solicitud y el archivo de licencia que te envían"
      >
        <ol className="license-steps">
          <li>
            <p>
              Guarda la solicitud de activación. Si escribiste el código de la licencia arriba, va
              en la solicitud.
            </p>
            <button
              type="button"
              className="secondary-button"
              disabled={busy}
              onClick={() => void saveRequest()}
            >
              <IconLabel icon="document">Guardar la solicitud</IconLabel>
            </button>
          </li>
          <li>
            <p>
              Envíala a soporte por correo o en una USB. Te responderán con un archivo de licencia
              (termina en .lic).
            </p>
          </li>
          <li>
            <p>Importa el archivo de licencia que te enviaron.</p>
            <button
              type="button"
              className="secondary-button"
              disabled={busy}
              onClick={() => void importFile()}
            >
              <IconLabel icon="arrow">Importar archivo de licencia</IconLabel>
            </button>
          </li>
        </ol>
      </Accordion>
      {dialog}
    </section>
  );
}
