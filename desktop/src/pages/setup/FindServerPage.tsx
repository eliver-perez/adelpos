import { useState } from 'react';
import { Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import {
  chooseInstallationMode,
  installationError,
  modeLabels,
  restartApplication,
} from '../../services/installation';
import type { InstallationState } from '../../services/installation';
import { AccessFrame } from '../AccessPages';

// «Buscar el servidor» de una terminal «Solo terminal» que todavía no se conecta a ninguno (etapa 9G,
// §2.3).
//
// PUNTO DE INTEGRACIÓN DE LAS FASES B Y C: aquí se agregan la lista de servidores encontrados en la red
// (Rust, descubrimiento mDNS), la dirección escrita a mano (Rust la valida) y el emparejamiento con
// código. Cuando Rust guarde el servidor emparejado en connection.json, `server_configured` pasa a true y
// App.tsx muestra el acceso normal en lugar de esta pantalla. Hasta entonces solo informa que la terminal
// aún no está conectada.
export function FindServerPage({ state }: { state: InstallationState }) {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const { confirm, dialog } = useConfirmDialog();

  async function useAsServer() {
    const confirmed = await confirm({
      title: 'Usar esta computadora como servidor',
      message: (
        <>
          <p>
            Esta computadora pasará a «{modeLabels['server-terminal']}»: guardará los datos del
            restaurante y atenderá a las demás terminales.
          </p>
          <p>La aplicación se reinicia para aplicar el cambio.</p>
        </>
      ),
      confirmLabel: 'Usar como servidor',
      cancelLabel: 'Seguir como terminal',
    });
    if (!confirmed) {
      return;
    }
    setSaving(true);
    setError('');
    try {
      await chooseInstallationMode('server-terminal');
      await restartApplication();
    } catch (problem) {
      setError(installationError(problem));
      setSaving(false);
    }
  }

  return (
    <AccessFrame
      title="Buscar el servidor"
      description="Esta computadora es una terminal: se conecta a la computadora principal del restaurante."
    >
      <div className="find-server" aria-busy={saving}>
        <p className="find-server-status" role="status">
          Esta terminal todavía no está conectada al servidor del restaurante.
        </p>
        <p className="muted find-server-help">
          Pronto podrás buscar la computadora principal en la red y emparejar esta terminal con un
          código. Mientras tanto, usa AD-EL POS en la computadora principal.
        </p>
        {state.server_data_kept && (
          <p className="muted find-server-help" role="note">
            Esta computadora fue el servidor del restaurante: sus datos se conservan y no se borró
            nada. Si vuelve a ser el servidor, los usará de nuevo.
          </p>
        )}
        <Notice>{error}</Notice>
        {state.can_change && (
          <div className="button-row">
            <button
              type="button"
              className="secondary-button"
              disabled={saving}
              onClick={() => void useAsServer()}
            >
              <IconLabel icon="server">Usar esta computadora como servidor</IconLabel>
            </button>
          </div>
        )}
      </div>
      {dialog}
    </AccessFrame>
  );
}
