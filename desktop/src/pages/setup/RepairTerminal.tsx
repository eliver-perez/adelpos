import { useState } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Notice } from '../../components/Forms';
import { restartApplication } from '../../services/installation';
import { forgetPairing, pairingError } from '../../services/pairing';
import { AccessFrame } from '../AccessPages';

// Volver a emparejar una terminal (etapa 9G, fase B): cuando la computadora principal la revocó o cuando
// el servidor cambió de identidad, la terminal deja de usar ese servidor (Rust conserva la modalidad y
// borra solo el servidor emparejado de su conexión) y la aplicación se reinicia en «Buscar el servidor».

export function useRepairTerminal() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const { confirm, dialog } = useConfirmDialog();

  async function repair() {
    const confirmed = await confirm({
      title: 'Emparejar de nuevo',
      message: (
        <>
          <p>Esta terminal dejará de usar el servidor con el que estaba emparejada.</p>
          <p>
            Después escribe la dirección de la computadora principal y pide un código nuevo. Los
            datos del restaurante no cambian.
          </p>
        </>
      ),
      confirmLabel: 'Emparejar de nuevo',
      cancelLabel: 'Volver',
    });
    if (!confirmed) {
      return;
    }
    setBusy(true);
    setError('');
    try {
      await forgetPairing();
      await restartApplication();
    } catch (problem) {
      setError(pairingError(problem, 'No se pudo preparar el nuevo emparejamiento.'));
      setBusy(false);
    }
  }

  return { repair, busy, error, dialog };
}

// Pantalla de una terminal emparejada que perdió su autorización (revocada en Terminales).
export function RepairTerminalPage({ message }: { message: string }) {
  const { repair, busy, error, dialog } = useRepairTerminal();
  return (
    <AccessFrame
      title="Esta terminal necesita emparejarse de nuevo"
      description="La computadora principal retiró la autorización de esta terminal."
    >
      <Notice>{message}</Notice>
      <Notice>{error}</Notice>
      <div className="button-row">
        <button
          type="button"
          className="primary-button"
          disabled={busy}
          onClick={() => void repair()}
        >
          Emparejar de nuevo
        </button>
      </div>
      {dialog}
    </AccessFrame>
  );
}
