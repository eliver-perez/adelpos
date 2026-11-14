import { useState } from 'react';
import { Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import {
  chooseInstallationMode,
  includesServer,
  installationError,
  restartApplication,
} from '../../services/installation';
import type { InstallationMode, InstallationState } from '../../services/installation';
import { AccessFrame } from '../AccessPages';
import { ModalityOptions } from './ModalityOptions';

// Modalidad sugerida en el primer arranque: la que dejó el instalador. Con la opción «Servidor del
// restaurante», «Servidor y terminal»; sin ella, «Solo terminal». Sin servicio instalado (desarrollo o
// copia manual), «Servidor y terminal», como hasta la 9F.
export function suggestedMode(state: InstallationState): InstallationMode {
  if (state.service_installed && !state.service_enabled) {
    return 'terminal-only';
  }
  return 'server-terminal';
}

// Elección de la modalidad en el primer arranque (etapa 9G, §2.3): aparece cuando la terminal todavía no
// tiene su configuración. Rust guarda la elección y la aplicación se reinicia para usarla.
export function ModalityChoicePage({ state }: { state: InstallationState }) {
  const [mode, setMode] = useState<InstallationMode>(() => suggestedMode(state));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const serverMissing = includesServer(mode) && !state.service_installed;

  async function save() {
    setSaving(true);
    setError('');
    try {
      await chooseInstallationMode(mode);
      await restartApplication();
    } catch (problem) {
      setError(installationError(problem));
      setSaving(false);
    }
  }

  return (
    <AccessFrame
      title="¿Cómo se usará esta computadora?"
      description="Elige la modalidad de esta computadora. Podrás cambiarla después en Mantenimiento; nunca se borran los datos."
    >
      <div className="modality-choice" aria-busy={saving}>
        <ModalityOptions value={mode} onChange={setMode} disabled={saving} />
        {serverMissing && (
          <p className="muted modality-note" role="note">
            El servidor del restaurante no está instalado como servicio en esta computadora: no
            arrancará solo al encenderla. Para instalarlo, vuelve a abrir el instalador de AD-EL POS
            con la opción «Servidor del restaurante».
          </p>
        )}
        <Notice>{error}</Notice>
        <div className="button-row">
          <button
            type="button"
            className="primary-button"
            disabled={saving}
            onClick={() => void save()}
          >
            <IconLabel icon="check">Continuar</IconLabel>
          </button>
        </div>
      </div>
    </AccessFrame>
  );
}
