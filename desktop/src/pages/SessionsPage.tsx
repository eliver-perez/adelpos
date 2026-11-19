import { useState } from 'react';
import { dateTime, Notice, PageTitle } from '../components/Forms';
import { useResource } from '../hooks/useResource';
import { api } from '../services/api';
import { notifyError, notifySuccess } from '../services/notify';
import type { Session } from '../services/models';

// Por qué se cerró una sesión, en palabras del restaurante (etapa A3, §1: ningún código interno a la
// vista). Go guarda en app.sessions.revoke_reason uno de estos motivos (administration: routes.go,
// sessions_audit.go, devices.go, service.go, users.go, setup.go, auth.go y pin.go; legacyimport al
// desactivar a un usuario importado).
const sessionCloseReasons: Record<string, string> = {
  logout: 'Cerró su sesión',
  admin_revoked: 'Cerrada desde Sesiones',
  device_revoked: 'Terminal revocada',
  expired: 'Expirada',
  user_disabled: 'Usuario desactivado',
  support_reset: 'Cerrada al restablecer el acceso de soporte',
  replaced: 'Reemplazada por un acceso nuevo en esta terminal',
  password_changed: 'Cerrada al cambiar la contraseña',
  pin_changed: 'Cerrada al cambiar el PIN',
  pin_removed: 'Cerrada al quitar el PIN',
};

// sessionState es lo que dice la columna «Estado»: activa, expirada o por qué se cerró. Un motivo que
// esta versión no conoce se dice en general, nunca con su código.
export function sessionState(session: Session, now = Date.now()) {
  if (!session.revoked_at) {
    return new Date(session.expires_at).getTime() > now ? 'Activa' : 'Expirada';
  }
  return sessionCloseReasons[session.revoke_reason ?? ''] ?? 'Cerrada';
}

export function SessionsPage({ can }: { can: (key: string) => boolean }) {
  const resource = useResource<Session[]>('sessions');
  const [busy, setBusy] = useState(false);
  async function revoke(session: Session) {
    setBusy(true);
    try {
      await api('revoke_session', { id: session.id, body: {} });
      notifySuccess(`Sesión de ${session.username} revocada.`);
      await resource.reload();
    } catch (error) {
      notifyError(error, 'No se pudo revocar la sesión.');
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageTitle
        title="Sesiones"
        description="Una sesión activa por usuario. Se muestran los 200 accesos más recientes."
      >
        <button className="secondary-button" onClick={() => void resource.reload()}>
          Actualizar
        </button>
      </PageTitle>
      <Notice>{resource.error}</Notice>
      <div className="surface table-container">
        <table>
          <thead>
            <tr>
              <th>Usuario / terminal</th>
              <th>Acceso</th>
              <th>Expiración</th>
              <th>Estado</th>
              <th>Acciones</th>
            </tr>
          </thead>
          <tbody>
            {resource.data?.map((session) => {
              const active =
                !session.revoked_at && new Date(session.expires_at).getTime() > Date.now();
              return (
                <tr key={session.id}>
                  <td>
                    <strong>{session.username}</strong>
                    <small className="muted block">{session.device_name}</small>
                  </td>
                  <td>{dateTime(session.created_at)}</td>
                  <td>{dateTime(session.expires_at)}</td>
                  <td>{sessionState(session)}</td>
                  <td>
                    {active && can('sessions.manage') && (
                      <button
                        className="danger-button"
                        disabled={busy}
                        onClick={() => void revoke(session)}
                      >
                        Revocar sesión
                      </button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </>
  );
}
