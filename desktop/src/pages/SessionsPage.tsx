import { useState } from 'react';
import { dateTime, Notice, PageTitle } from '../components/Forms';
import { useResource } from '../hooks/useResource';
import { api } from '../services/api';
import type { Session } from '../services/models';

export function SessionsPage({ can }: { can: (key: string) => boolean }) {
  const resource = useResource<Session[]>('sessions');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  async function revoke(id: string) {
    setBusy(true);
    setError('');
    try {
      await api('revoke_session', { id, body: {} });
      await resource.reload();
    } catch (error) {
      setError(error instanceof Error ? error.message : 'No se pudo revocar.');
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
      <Notice>{resource.error || error}</Notice>
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
                  <td>{active ? 'Activa' : (session.revoke_reason ?? 'Expirada')}</td>
                  <td>
                    {active && can('sessions.manage') && (
                      <button
                        className="danger-button"
                        disabled={busy}
                        onClick={() => void revoke(session.id)}
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
