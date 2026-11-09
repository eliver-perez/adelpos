import { useState } from 'react';
import { useConfirmDialog } from '../components/ConfirmDialog';
import { dateTime, Field, Form, Notice, PageTitle } from '../components/Forms';
import { useResource } from '../hooks/useResource';
import { api } from '../services/api';
import { notifyError, notifySuccess } from '../services/notify';
import type { Device, Register } from '../services/models';

function DeviceEditor({
  device,
  registers,
  reload,
}: {
  device: Device;
  registers: Register[];
  reload: () => Promise<void>;
}) {
  const [name, setName] = useState(device.name);
  const [registerID, setRegisterID] = useState(device.register_id ?? '');
  const [keyboard, setKeyboard] = useState(device.keyboard_mode);
  return (
    <section className="surface panel">
      <h2 className="mb-5">Editar {device.name}</h2>
      <Form
        successMessage="Terminal actualizada."
        onSave={async () => {
          await api('update_device', {
            id: device.id,
            body: { name, register_id: registerID || null, keyboard_mode: keyboard },
          });
          await reload();
        }}
      >
        <div className="form-grid">
          <Field label="Nombre">
            <input
              required
              maxLength={80}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </Field>
          <Field label="Caja física">
            <select value={registerID} onChange={(event) => setRegisterID(event.target.value)}>
              <option value="">Sin caja asociada</option>
              {registers
                .filter((register) => register.active)
                .map((register) => (
                  <option key={register.id} value={register.id}>
                    {register.name}
                  </option>
                ))}
            </select>
          </Field>
          <Field label="Teclado virtual">
            <select
              value={keyboard}
              onChange={(event) => setKeyboard(event.target.value as Device['keyboard_mode'])}
            >
              <option value="auto">Automático</option>
              <option value="always">Siempre habilitado</option>
              <option value="disabled">Desactivado</option>
            </select>
          </Field>
        </div>
      </Form>
    </section>
  );
}

export function DevicesPage({ can }: { can: (key: string) => boolean }) {
  const devices = useResource<Device[]>('devices');
  const registers = useResource<Register[]>('registers', { enabled: can('registers.read') });
  const [editing, setEditing] = useState<Device | null>(null);
  const [invite, setInvite] = useState<{ code: string; expires_at: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  async function invitation() {
    setBusy(true);
    try {
      setInvite(await api('invite_device', { body: {} }));
      notifySuccess('Código de autorización creado.');
    } catch (error) {
      notifyError(error, 'No se pudo crear la autorización.');
    } finally {
      setBusy(false);
    }
  }
  // Revocar es irreversible: se confirma con el diálogo estándar en su variante de peligro.
  async function revoke(device: Device) {
    const confirmed = await confirm({
      title: `Revocar ${device.name}`,
      message: 'La terminal perderá acceso inmediatamente, incluidas sus sesiones.',
      confirmLabel: 'Confirmar revocación',
      cancelLabel: 'Cancelar',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    setBusy(true);
    try {
      await api('revoke_device', { id: device.id, body: {} });
      notifySuccess(`Terminal ${device.name} revocada.`);
      await devices.reload();
    } catch (error) {
      notifyError(error, 'No se pudo revocar la terminal.');
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageTitle
        title="Terminales"
        description="Identidades de dispositivo independientes de los usuarios."
      >
        {can('devices.manage') && (
          <button className="primary-button" disabled={busy} onClick={() => void invitation()}>
            Autorizar otra terminal
          </button>
        )}
      </PageTitle>
      <Notice>{devices.error || registers.error}</Notice>
      {invite && (
        <section className="surface panel">
          <h2>Código de un solo uso</h2>
          <p className="muted my-3 text-sm">
            Introduce este código en la otra aplicación antes de {dateTime(invite.expires_at)}. No
            concede una sesión de usuario.
          </p>
          <code className="enrollment-code">{invite.code}</code>
          <button className="secondary-button mt-4" onClick={() => setInvite(null)}>
            Ocultar código
          </button>
        </section>
      )}
      {editing && (
        <>
          <DeviceEditor
            key={editing.id}
            device={editing}
            registers={registers.data ?? []}
            reload={devices.reload}
          />
          <button className="secondary-button mb-5" onClick={() => setEditing(null)}>
            Cerrar editor
          </button>
        </>
      )}
      <div className="surface table-container">
        <table>
          <thead>
            <tr>
              <th>Terminal</th>
              <th>Caja</th>
              <th>Última conexión</th>
              <th>Estado</th>
              <th>Acciones</th>
            </tr>
          </thead>
          <tbody>
            {devices.data?.map((device) => (
              <tr key={device.id}>
                <td>
                  <strong>{device.name}</strong>
                  <small className="muted block font-mono">{device.id}</small>
                </td>
                <td>{device.register_name ?? 'Sin asociación'}</td>
                <td>{device.last_seen_at ? dateTime(device.last_seen_at) : '—'}</td>
                <td>{device.revoked_at ? 'Revocada' : 'Autorizada'}</td>
                <td>
                  {can('devices.manage') && !device.revoked_at && (
                    <div className="button-row">
                      <button className="secondary-button" onClick={() => setEditing(device)}>
                        Editar
                      </button>
                      <button
                        className="danger-button"
                        disabled={busy}
                        onClick={() => void revoke(device)}
                      >
                        Revocar
                      </button>
                    </div>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {dialog}
    </>
  );
}
