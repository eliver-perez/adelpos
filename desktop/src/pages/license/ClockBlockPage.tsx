import { useEffect, useId, useState } from 'react';
import { IconLabel } from '../../components/Icon';
import { APIError } from '../../services/api';
import {
  clockLicenseDependsOnDate,
  clockLicenseGraceText,
  clockLicenseMayClearText,
  clockLicenseOkText,
  clockLicenseSeparateText,
  clockScreenTitle,
  clockWhyText,
  licenseStateExplanation,
  licenseStateLabels,
  supportCodeGroups,
} from '../../services/license';
import type { LicenseSummary } from '../../services/license';
import {
  checkTimeWithInternet,
  importSupportResponse,
  requestSupportCode,
  unlockWithSupportText,
} from '../../services/licenseOperations';
import type { SupportRequest } from '../../services/licenseOperations';
import { errorMessage, notifyError, notifySuccess } from '../../services/notify';

// «Sistema detenido por la fecha y hora de la computadora» (etapa L1, Fase E; §6.2 de la especificación), en la
// computadora principal. Debe quedar claro por qué se bloqueó: nunca se confunde con la licencia, la conexión ni los
// permisos. Sus cinco partes, tal como las escribe la especificación:
//
//  1. qué pasó, con las dos fechas completas en la zona horaria del restaurante (las da Go);
//  2. por qué se detiene;
//  3. qué sí se puede hacer;
//  4. cómo resolverlo: corregir el reloj (Mac y Windows), «Comprobar la hora con internet» (§6.3) y el desbloqueo de
//     soporte (§6.4) con su código de solicitud, pegando la respuesta o importándola como archivo;
//  5. «Tu licencia está bien…», solo si la licencia está vigente o en tolerancia. Si también hay un problema de
//     licencia, los dos se muestran por separado. En Tolerancia o Vencida se aclara que corregir la fecha y hora puede
//     resolver también el aviso de la licencia (L1-D58: un salto al futuro sube la hora con la que Go calcula el
//     estado); en Revocada o Sin activar, que es otro problema.
//
// El bloqueo se quita solo en cuanto el reloj se corrige: la sesión vuelve a leer el estado cada pocos segundos.

export function ClockBlockPage({
  license,
  canOpenReports,
  canOpenBackups,
  onOpenReports,
  onOpenBackups,
  onOpenLicense,
  onHome,
  onChanged,
}: {
  license: LicenseSummary;
  canOpenReports: boolean;
  canOpenBackups: boolean;
  onOpenReports: () => void;
  onOpenBackups: () => void;
  onOpenLicense: () => void;
  onHome: () => void;
  onChanged: () => Promise<void>;
}) {
  const clock = license.clock;
  if (!clock.blocked) {
    return (
      <section className="surface panel form-stack clock-block" aria-labelledby="clock-ok-title">
        <h1 id="clock-ok-title">La fecha y hora de la computadora es correcta</h1>
        <p>El sistema funciona con normalidad: ya se puede registrar de nuevo.</p>
        <div className="button-row">
          <button type="button" className="primary-button" onClick={onHome}>
            Volver al inicio
          </button>
        </div>
      </section>
    );
  }
  return (
    <div className="clock-block form-stack">
      <section className="surface panel form-stack" aria-labelledby="clock-title">
        <h1 id="clock-title">{clockScreenTitle}</h1>
        <div className="clock-part" role="group" aria-labelledby="clock-what-title">
          <h2 id="clock-what-title">Qué pasó</h2>
          <p>
            La fecha y hora de esta computadora es <strong>{clock.local_time_text}</strong>. La
            última actividad registrada en el sistema fue el{' '}
            <strong>{clock.last_activity_time_text}</strong>. La hora de la computadora no puede ser
            anterior a lo que ya se registró.
          </p>
        </div>
        <div className="clock-part" role="group" aria-labelledby="clock-why-title">
          <h2 id="clock-why-title">Por qué se detiene</h2>
          <p>{clockWhyText}</p>
        </div>
        <div className="clock-part" role="group" aria-labelledby="clock-can-title">
          <h2 id="clock-can-title">Qué sí puedes hacer</h2>
          <p>Consultar reportes e historial y hacer respaldos.</p>
          {(canOpenReports || canOpenBackups) && (
            <div className="button-row">
              {canOpenReports && (
                <button type="button" className="secondary-button" onClick={onOpenReports}>
                  <IconLabel icon="report">Ir a Reportes</IconLabel>
                </button>
              )}
              {canOpenBackups && (
                <button type="button" className="secondary-button" onClick={onOpenBackups}>
                  <IconLabel icon="database">Ir a Respaldos</IconLabel>
                </button>
              )}
            </div>
          )}
        </div>
        <ClockSolutions onChanged={onChanged} />
        {clock.license_ok ? (
          <div className="clock-license-ok" role="note">
            <p>{clockLicenseOkText}</p>
            {clockLicenseDependsOnDate(license.state) && (
              <p className="muted text-sm">{clockLicenseGraceText}</p>
            )}
          </div>
        ) : (
          <div className="clock-license-problem" role="group" aria-labelledby="clock-license-title">
            <h2 id="clock-license-title">
              Además, la licencia: {licenseStateLabels[license.state]}
            </h2>
            <p>{licenseStateExplanation(license)}</p>
            <p className="muted text-sm">
              {clockLicenseDependsOnDate(license.state)
                ? clockLicenseMayClearText
                : clockLicenseSeparateText}
            </p>
            <div className="button-row">
              <button type="button" className="secondary-button" onClick={onOpenLicense}>
                <IconLabel icon="key">Ver la licencia</IconLabel>
              </button>
            </div>
          </div>
        )}
      </section>
    </div>
  );
}

function ClockSolutions({ onChanged }: { onChanged: () => Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const [mismatch, setMismatch] = useState('');
  const [support, setSupport] = useState(false);

  async function check() {
    if (busy) return;
    setBusy(true);
    setMismatch('');
    try {
      const result = await checkTimeWithInternet();
      if (result.clock.blocked) {
        notifySuccess(
          'La hora de esta computadora coincide con la de internet, pero sigue siendo anterior a la última actividad. Usa el desbloqueo de soporte.',
        );
      } else {
        notifySuccess('La hora coincide con la de internet: el sistema se desbloqueó.');
      }
      await onChanged();
    } catch (error) {
      if (error instanceof APIError && error.code === 'LICENSE_TIME_MISMATCH') {
        // La hora de internet queda a la vista para corregir el reloj a esa hora.
        setMismatch(error.message);
      } else {
        notifyError(error, 'No se pudo comprobar la hora con internet.');
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="clock-part" role="group" aria-labelledby="clock-how-title">
      <h2 id="clock-how-title">Cómo resolverlo</h2>
      <ol className="clock-steps">
        <li>
          <strong>Corrige la fecha y hora de la computadora.</strong>
          <ul className="clock-platforms">
            <li>
              En Mac: menú Apple › Configuración del Sistema › General › Fecha y hora. Activa
              «Ajustar la hora automáticamente».
            </li>
            <li>
              En Windows: Configuración › Hora e idioma › Fecha y hora. Activa «Establecer la hora
              automáticamente».
            </li>
          </ul>
          <p className="muted text-sm">
            En cuanto la hora sea correcta, el sistema se desbloquea solo.
          </p>
        </li>
        <li>
          <p>Si la hora ya es correcta, compruébala con internet.</p>
          <button
            type="button"
            className="primary-button"
            disabled={busy}
            onClick={() => void check()}
          >
            <IconLabel icon="refresh">
              {busy ? 'Comprobando…' : 'Comprobar la hora con internet'}
            </IconLabel>
          </button>
          {mismatch && (
            <p className="clock-mismatch" role="alert">
              {mismatch}
            </p>
          )}
        </li>
        <li>
          <p>
            Si la hora de la computadora es correcta y el problema sigue, comunícate con soporte.
          </p>
          {support ? (
            <SupportUnlockPanel onChanged={onChanged} />
          ) : (
            <button type="button" className="secondary-button" onClick={() => setSupport(true)}>
              <IconLabel icon="unlock">Desbloqueo de soporte</IconLabel>
            </button>
          )}
        </li>
      </ol>
    </div>
  );
}

// Desbloqueo de soporte sin internet (§6.4): el código de solicitud, corto y legible por teléfono; la respuesta se pega
// como texto (por ejemplo, de WhatsApp) o se importa como archivo. Vale una sola vez, en esta computadora y dentro de
// las 24 horas de la solicitud.
export function SupportUnlockPanel({ onChanged }: { onChanged: () => Promise<void> }) {
  const responseID = useId();
  const [request, setRequest] = useState<SupportRequest | null>(null);
  const [requestError, setRequestError] = useState('');
  const [response, setResponse] = useState('');
  const [busy, setBusy] = useState(false);

  async function load(renew: boolean) {
    setBusy(true);
    setRequestError('');
    try {
      setRequest(await requestSupportCode(renew));
    } catch (error) {
      setRequestError(errorMessage(error, 'No se pudo preparar el código de solicitud.'));
    } finally {
      setBusy(false);
    }
  }

  // Al abrir el desbloqueo se pide el código: el pendiente si sigue vigente o uno nuevo (lo decide Go).
  useEffect(() => {
    let active = true;
    requestSupportCode(false)
      .then((value) => {
        if (active) setRequest(value);
      })
      .catch((error: unknown) => {
        if (active)
          setRequestError(errorMessage(error, 'No se pudo preparar el código de solicitud.'));
      });
    return () => {
      active = false;
    };
  }, []);

  async function finish(unlocked: { clock: { blocked: boolean } }) {
    setResponse('');
    notifySuccess(
      unlocked.clock.blocked
        ? 'Se aplicó la respuesta de soporte, pero la hora de la computadora sigue siendo anterior. Corrígela.'
        : 'Se aplicó la respuesta de soporte: el sistema se desbloqueó.',
    );
    await onChanged();
  }

  async function unlockText() {
    if (busy || response.trim() === '') return;
    setBusy(true);
    try {
      await finish(await unlockWithSupportText(response));
    } catch (error) {
      notifyError(error, 'No se pudo aplicar la respuesta de soporte.');
    } finally {
      setBusy(false);
    }
  }

  async function unlockFile() {
    if (busy) return;
    setBusy(true);
    try {
      const imported = await importSupportResponse();
      if (imported.result === 'unlocked') {
        await finish(imported.unlock);
      }
    } catch (error) {
      notifyError(error, 'No se pudo aplicar la respuesta de soporte.');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="support-unlock form-stack" role="region" aria-label="Desbloqueo de soporte">
      <p>
        Dicta o envía este código a soporte. Te responderán con un texto o un archivo para
        desbloquear esta computadora.
      </p>
      {requestError && (
        <p className="clock-mismatch" role="alert">
          {requestError}
        </p>
      )}
      {request && (
        <>
          <p className="support-code" aria-label="Código de solicitud">
            {supportCodeGroups(request.request_code).map((group, index) => (
              <span key={index}>{group}</span>
            ))}
          </p>
          <p className="muted text-sm">
            Vale hasta el {request.expires_at_text} y solo para esta computadora.
          </p>
        </>
      )}
      <div className="button-row">
        <button
          type="button"
          className="secondary-button"
          disabled={busy}
          onClick={() => void load(true)}
        >
          Pedir otro código
        </button>
      </div>
      <label className="field" htmlFor={responseID}>
        <span>Respuesta de soporte</span>
        <textarea
          id={responseID}
          rows={4}
          value={response}
          spellCheck={false}
          onChange={(event) => setResponse(event.target.value)}
        />
      </label>
      <div className="button-row">
        <button
          type="button"
          className="primary-button"
          disabled={busy || response.trim() === ''}
          onClick={() => void unlockText()}
        >
          <IconLabel icon="unlock">Desbloquear</IconLabel>
        </button>
        <button
          type="button"
          className="secondary-button"
          disabled={busy}
          onClick={() => void unlockFile()}
        >
          <IconLabel icon="arrow">Importar la respuesta</IconLabel>
        </button>
      </div>
    </div>
  );
}
