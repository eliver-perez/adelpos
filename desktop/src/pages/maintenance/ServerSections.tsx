import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { useLiveResource } from '../../hooks/useLiveResource';
import {
  chooseInstallationMode,
  includesServer,
  installationError,
  modeLabels,
  readInstallation,
  restartApplication,
} from '../../services/installation';
import type { InstallationMode, InstallationState } from '../../services/installation';
import { notifyError, notifySuccess } from '../../services/notify';
import { restartService, terminalsLabel, uptimeLabel } from '../../services/serverService';
import type { ServiceStatus } from '../../services/serverService';
import { ModalityOptions } from '../setup/ModalityOptions';

// Mantenimiento › «Servicio del servidor» y «Modalidad de esta computadora» (etapa 9G, fase A; §2.2 y
// §2.3). El estado del servicio y el reinicio los decide Go (solo un servicio de la computadora se puede
// reiniciar, y launchd lo vuelve a lanzar); la modalidad la guarda Rust. Cambiar de modalidad nunca borra
// la base: si la computadora deja de ser servidor, el servidor deja de atender y los datos se conservan.

// Tiempo para leer el aviso antes de que la aplicación se reinicie con la modalidad nueva.
const restartDelay = 1500;

export function ServiceSection() {
  const status = useLiveResource<ServiceStatus>('maintenance_service', { interval: 15000 });
  const [restarting, setRestarting] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const data = status.data;

  async function restart() {
    const confirmed = await confirm({
      title: 'Reiniciar el servidor',
      message: (
        <>
          <p>El servidor se detiene en orden y vuelve a arrancar solo en unos segundos.</p>
          <p>
            Mientras tanto las terminales muestran que no hay conexión y se reconectan solas.
            Conviene hacerlo cuando no se esté cobrando.
          </p>
        </>
      ),
      confirmLabel: 'Reiniciar el servidor',
      cancelLabel: 'No reiniciar',
    });
    if (!confirmed) {
      return;
    }
    setRestarting(true);
    try {
      await restartService();
      notifySuccess('El servidor se está reiniciando.', {
        description: 'Vuelve a estar disponible en unos segundos.',
      });
    } catch (error) {
      notifyError(error, 'No se pudo reiniciar el servidor.');
    } finally {
      setRestarting(false);
    }
  }

  return (
    <section
      className="surface panel maintenance-panel"
      aria-labelledby="maintenance-service-title"
      aria-busy={restarting}
    >
      <div className="section-heading">
        <h2 id="maintenance-service-title">Servicio del servidor</h2>
      </div>
      {data && (
        <div className="maintenance-service-state" role="status">
          <p>
            <span
              className={`status-dot ${data.managed ? 'good' : 'neutral'}`}
              aria-hidden="true"
            />
            {data.managed
              ? 'El servidor funciona como servicio de la computadora: arranca solo al encenderla, aunque nadie inicie sesión, y vuelve a arrancar si se detiene.'
              : 'El servidor no está instalado como servicio de la computadora: no arranca solo al encenderla y no se puede reiniciar desde aquí.'}
          </p>
          <p className="muted">
            {`Activo desde hace ${uptimeLabel(data.uptime_seconds)}. ${terminalsLabel(data)}`}
          </p>
        </div>
      )}
      <Notice>{status.error}</Notice>
      {data?.restart_available && (
        <div className="button-row">
          <button
            type="button"
            className="secondary-button"
            disabled={restarting}
            onClick={() => void restart()}
          >
            <IconLabel icon="refresh">Reiniciar</IconLabel>
          </button>
        </div>
      )}
      {dialog}
    </section>
  );
}

// Lo que cambia con la modalidad nueva, para la confirmación.
function changeMessage(
  current: InstallationMode,
  next: InstallationMode,
  state: InstallationState,
) {
  const lines: ReactNode[] = [];
  if (includesServer(current) && next === 'terminal-only') {
    lines.push(
      'Esta computadora dejará de atender a las terminales: el servidor se detiene.',
      'La base de datos se conserva en esta computadora y no se borra nada. Las demás terminales no podrán trabajar hasta que se conecten a otro servidor o esta computadora vuelva a serlo.',
    );
  } else if (next === 'server-only') {
    lines.push(
      'En esta computadora ya no se venderá: la aplicación mostrará el estado del servidor y Administración.',
      'El servidor sigue atendiendo a las terminales.',
    );
  } else if (current === 'server-only' && next === 'server-terminal') {
    lines.push('Esta computadora volverá a usarse también como caja. El servidor sigue igual.');
  } else {
    lines.push('Esta computadora guardará los datos del restaurante y atenderá a las terminales.');
    if (state.server_data_kept) {
      lines.push('Usará de nuevo los datos que conservó de cuando era el servidor.');
    }
  }
  if (includesServer(next) && !state.service_installed) {
    lines.push(
      'El servidor del restaurante no está instalado como servicio en esta computadora: no arrancará solo al encenderla.',
    );
  }
  lines.push('La aplicación se reinicia para aplicar el cambio.');
  return (
    <>
      {lines.map((line, index) => (
        <p key={index}>{line}</p>
      ))}
    </>
  );
}

export function ModalitySection() {
  const [state, setState] = useState<InstallationState | null>(null);
  const [selected, setSelected] = useState<InstallationMode | null>(null);
  const [loadError, setLoadError] = useState('');
  const [saving, setSaving] = useState(false);
  const { confirm, dialog } = useConfirmDialog();

  useEffect(() => {
    let active = true;
    readInstallation()
      .then((value) => {
        if (active) {
          setState(value);
          setSelected(value.mode);
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setLoadError(installationError(error));
        }
      });
    return () => {
      active = false;
    };
  }, []);

  async function change(next: InstallationMode) {
    if (!state || next === state.mode) {
      return;
    }
    const confirmed = await confirm({
      title: next === 'terminal-only' ? 'Dejar de ser el servidor' : 'Cambiar la modalidad',
      message: changeMessage(state.mode, next, state),
      confirmLabel: `Cambiar a «${modeLabels[next]}»`,
      cancelLabel: 'Conservar la modalidad',
    });
    if (!confirmed) {
      return;
    }
    setSaving(true);
    try {
      await chooseInstallationMode(next);
      notifySuccess(`Modalidad cambiada a «${modeLabels[next]}».`, {
        description: 'La aplicación se reinicia para aplicar el cambio.',
      });
      window.setTimeout(() => {
        void restartApplication().catch((error: unknown) => {
          notifyError(installationError(error), 'No se pudo reiniciar la aplicación.');
        });
      }, restartDelay);
    } catch (error) {
      notifyError(installationError(error), 'No se pudo cambiar la modalidad.');
      setSaving(false);
    }
  }

  return (
    <section
      className="surface panel maintenance-panel"
      aria-labelledby="maintenance-modality-title"
      aria-busy={saving}
    >
      <div className="section-heading">
        <h2 id="maintenance-modality-title">Modalidad de esta computadora</h2>
      </div>
      <Notice>{loadError}</Notice>
      {state && selected && (
        <>
          <p className="muted maintenance-help">
            {state.can_change
              ? 'Elige cómo se usa esta computadora. Cambiar la modalidad nunca borra los datos del restaurante.'
              : `Esta computadora está instalada como «${modeLabels[state.mode]}». En esta computadora la modalidad se elige al instalar AD-EL POS.`}
          </p>
          {state.can_change && (
            <>
              <ModalityOptions
                value={selected}
                current={state.mode}
                disabled={saving}
                onChange={setSelected}
              />
              <div className="button-row maintenance-modality-actions">
                <button
                  type="button"
                  className="primary-button"
                  disabled={saving || selected === state.mode}
                  onClick={() => void change(selected)}
                >
                  <IconLabel icon="check">Cambiar la modalidad</IconLabel>
                </button>
              </div>
            </>
          )}
        </>
      )}
      {dialog}
    </section>
  );
}
