import { useCallback, useEffect, useState } from 'react';
import { Field, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import {
  chooseInstallationMode,
  installationError,
  modeLabels,
  restartApplication,
} from '../../services/installation';
import type { InstallationState } from '../../services/installation';
import {
  cancelPairing,
  defaultTerminalName,
  groupedCode,
  pairingError,
  readPairing,
  renamePairing,
  startPairing,
} from '../../services/pairing';
import type { PairingView } from '../../services/pairing';
import { AccessFrame } from '../AccessPages';

// «Buscar el servidor» de una terminal «Solo terminal» que todavía no se conecta a ninguno (etapa 9G,
// §2.3 y §3.2).
//
// Fase B: dirección escrita a mano y emparejamiento con código. La terminal propone la dirección (Rust la
// valida: red local del restaurante), pide el código y lo muestra en grande con el nombre propuesto
// (editable) mientras espera; en la computadora principal un administrador escribe ese código en
// Administración › Sistema › Terminales. Aprobada, Rust guarda la credencial y el servidor emparejado y la
// aplicación se reinicia con el acceso normal.
//
// PUNTO DE INTEGRACIÓN DE LA FASE C: la lista de servidores encontrados en la red (descubrimiento mDNS)
// va arriba de la dirección escrita a mano y entra a la misma pantalla del código (startPairing).

// Cada cuánto se consulta si la computadora principal ya aprobó el código.
export const pairingPollMilliseconds = 2000;

function remaining(expiresAt: string, now: number) {
  const seconds = Math.max(0, Math.floor((Date.parse(expiresAt) - now) / 1000));
  const minutes = Math.floor(seconds / 60);
  return `${minutes}:${String(seconds % 60).padStart(2, '0')}`;
}

function PairingCodePanel({
  view,
  onChange,
  onRestart,
}: {
  view: PairingView;
  onChange: (next: PairingView | null) => void;
  onRestart: () => void;
}) {
  const [now, setNow] = useState(() => Date.now());
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(view.name);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const waiting = view.state === 'pending';

  // Mientras espera, consulta el estado; aprobado, Rust ya guardó la credencial y la conexión. Las consultas
  // se encadenan: la siguiente se programa cuando termina la anterior, nunca dos a la vez (revisión, hallazgo
  // 3: guardar la credencial puede esperar el diálogo del Llavero). Un error no detiene la espera: Rust
  // conserva la credencial reclamada y la siguiente consulta vuelve a intentar guardarla.
  useEffect(() => {
    if (!waiting) {
      return;
    }
    let active = true;
    let timer: number | undefined;
    const poll = () => {
      readPairing()
        .then((next) => {
          if (active) {
            setError('');
            onChange(next);
          }
        })
        .catch((problem: unknown) => {
          if (active) {
            setError(pairingError(problem, 'No se pudo consultar la computadora principal.'));
          }
        })
        .finally(() => {
          if (active) {
            timer = window.setTimeout(poll, pairingPollMilliseconds);
          }
        });
    };
    timer = window.setTimeout(poll, pairingPollMilliseconds);
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [waiting, onChange]);

  useEffect(() => {
    if (!waiting) {
      return;
    }
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [waiting]);

  // Emparejada: la aplicación se reinicia para conectarse al servidor.
  useEffect(() => {
    if (view.state === 'paired') {
      const timer = window.setTimeout(onRestart, 1500);
      return () => window.clearTimeout(timer);
    }
  }, [view.state, onRestart]);

  async function saveName() {
    setBusy(true);
    setError('');
    try {
      onChange(await renamePairing(name));
      setEditing(false);
    } catch (problem) {
      setError(pairingError(problem, 'No se pudo cambiar el nombre.'));
    } finally {
      setBusy(false);
    }
  }

  async function cancel() {
    setBusy(true);
    try {
      await cancelPairing();
    } finally {
      setBusy(false);
      onChange(null);
    }
  }

  return (
    <div className="pairing-panel" aria-busy={busy}>
      {view.server_name && (
        <p className="muted pairing-server">
          Computadora principal de <strong>{view.server_name}</strong>
        </p>
      )}
      {waiting && (
        <>
          <p className="pairing-instructions">
            En la computadora principal, un administrador entra a Administración › Sistema ›
            Terminales y escribe este código:
          </p>
          <p
            className="pairing-code"
            aria-label={`Código de emparejamiento ${view.code.split('').join(' ')}`}
          >
            {groupedCode(view.code)}
          </p>
          <p className="muted pairing-expiry" role="timer">
            El código vence en {remaining(view.expires_at, now)}.
          </p>
        </>
      )}
      {editing ? (
        <div className="pairing-name-editor">
          <Field label="Nombre de esta terminal">
            <input
              required
              maxLength={80}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </Field>
          <div className="button-row">
            <button
              type="button"
              className="primary-button"
              disabled={busy || name.trim() === ''}
              onClick={() => void saveName()}
            >
              Guardar nombre
            </button>
            <button
              type="button"
              className="secondary-button"
              disabled={busy}
              onClick={() => {
                setName(view.name);
                setEditing(false);
              }}
            >
              Cancelar
            </button>
          </div>
        </div>
      ) : (
        <p className="pairing-name">
          Esta terminal se llamará <strong>{view.name}</strong>
          {waiting && (
            <button
              type="button"
              className="secondary-button"
              disabled={busy}
              onClick={() => setEditing(true)}
            >
              <IconLabel icon="edit">Cambiar nombre</IconLabel>
            </button>
          )}
        </p>
      )}
      <p className="find-server-status" role="status">
        {waiting && 'Esperando la aprobación de la computadora principal…'}
        {view.state === 'paired' &&
          'Terminal emparejada. La aplicación se reinicia para conectarse al servidor.'}
        {view.state === 'rejected' &&
          'La computadora principal rechazó esta terminal. Si fue un error, pide otro código.'}
        {view.state === 'expired' && 'El código venció antes de aprobarse. Pide otro código.'}
        {view.state === 'cancelled' && 'Se canceló la solicitud.'}
        {view.state === 'claimed' &&
          'Esta terminal ya recibió su autorización. Para repetirlo, pide otro código.'}
      </p>
      <Notice>{error}</Notice>
      <div className="button-row">
        {waiting && (
          <button
            type="button"
            className="secondary-button"
            disabled={busy}
            onClick={() => void cancel()}
          >
            Cancelar
          </button>
        )}
        {!waiting && view.state !== 'paired' && (
          <button type="button" className="primary-button" onClick={() => onChange(null)}>
            Pedir otro código
          </button>
        )}
      </div>
    </div>
  );
}

export function FindServerPage({ state }: { state: InstallationState }) {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [address, setAddress] = useState('');
  const [name, setName] = useState(defaultTerminalName);
  const [pairing, setPairing] = useState<PairingView | null>(null);
  const { confirm, dialog } = useConfirmDialog();
  // Estable entre renders: el aviso «Terminal emparejada» espera su plazo antes de reiniciar.
  const restart = useCallback(() => void restartApplication(), []);

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

  async function requestCode() {
    setSaving(true);
    setError('');
    try {
      setPairing(await startPairing(address, name));
    } catch (problem) {
      setError(pairingError(problem));
    } finally {
      setSaving(false);
    }
  }

  return (
    <AccessFrame
      title="Buscar el servidor"
      description="Esta computadora es una terminal: se conecta a la computadora principal del restaurante."
    >
      {pairing ? (
        <PairingCodePanel view={pairing} onChange={setPairing} onRestart={restart} />
      ) : (
        <div className="find-server" aria-busy={saving}>
          <p className="find-server-status" role="status">
            Esta terminal todavía no está conectada al servidor del restaurante.
          </p>
          <p className="muted find-server-help">
            Escribe la dirección de la computadora principal. La encuentras en esa computadora, en
            Administración › Sistema › Terminales.
          </p>
          <form
            className="find-server-form"
            onSubmit={(event) => {
              event.preventDefault();
              void requestCode();
            }}
          >
            <Field label="Dirección de la computadora principal">
              <input
                required
                autoComplete="off"
                spellCheck={false}
                placeholder="Por ejemplo 192.168.1.20"
                maxLength={270}
                value={address}
                onChange={(event) => setAddress(event.target.value)}
              />
            </Field>
            <Field label="Nombre de esta terminal">
              <input
                required
                maxLength={80}
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </Field>
            <div className="button-row">
              <button
                type="submit"
                className="primary-button"
                disabled={saving || address.trim() === '' || name.trim() === ''}
              >
                Pedir código
              </button>
            </div>
          </form>
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
      )}
      {dialog}
    </AccessFrame>
  );
}
