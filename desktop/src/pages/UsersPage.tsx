import { useState } from 'react';
import { Field, Form, Notice, PageTitle } from '../components/Forms';
import { useResource } from '../hooks/useResource';
import { api } from '../services/api';
import type { Permission, Role, User } from '../services/models';
import { OtherUserPreferences } from './PreferencesPage';
import { UserKitchenStations } from './UserKitchenStations';
import { Select } from '../components/Select';

function UserEditor({
  user,
  roles,
  onSaved,
}: {
  user: User | null;
  roles: Role[];
  onSaved: () => Promise<void>;
}) {
  const [username, setUsername] = useState(user?.username ?? '');
  const [name, setName] = useState(user?.display_name ?? '');
  const [roleID, setRoleID] = useState(user?.role_id ?? '');
  const [active, setActive] = useState(user?.active ?? true);
  const [password, setPassword] = useState('');
  return (
    <Form
      label={user ? 'Guardar usuario' : 'Crear usuario'}
      successMessage={user ? 'Usuario guardado.' : 'Usuario creado.'}
      onSave={async () => {
        const body = {
          username,
          display_name: name,
          role_id: roleID,
          active,
          ...(!user ? { password } : {}),
        };
        try {
          await api(user ? 'update_user' : 'create_user', { id: user?.id, body });
        } finally {
          setPassword('');
        }
        await onSaved();
      }}
    >
      <div className="form-grid">
        <Field label="Usuario">
          <input
            required
            maxLength={40}
            autoComplete="off"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
        </Field>
        <Field label="Nombre visible">
          <input
            required
            maxLength={100}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        <Field label="Rol principal">
          <Select
            required
            value={roleID}
            placeholder="Selecciona un rol"
            options={[
              ...roles.map((role) => ({ value: role.id, label: role.name })),
              ...(user && !roles.some((role) => role.id === user.role_id)
                ? [{ value: user.role_id, label: user.role_name }]
                : []),
            ]}
            onChange={setRoleID}
          />
        </Field>
        {!user && (
          <Field label="Contraseña inicial">
            <input
              required
              minLength={8}
              maxLength={256}
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
          </Field>
        )}
      </div>
      <label className="checkbox-label">
        <input
          type="checkbox"
          checked={active}
          onChange={(event) => setActive(event.target.checked)}
        />
        Usuario activo
      </label>
    </Form>
  );
}

// «Asignar contraseña» (etapa 9H): solo para usuarios sin contraseña, como los importados del sistema
// anterior, que llegan sin acceso. Go la guarda como contraseña inicial y rechaza a quien ya tiene una
// (solo él la cambia desde su formulario personal).
export function AssignPasswordForm({
  user,
  onAssigned,
}: {
  user: User;
  onAssigned: () => Promise<void>;
}) {
  const [password, setPassword] = useState('');
  const [repeated, setRepeated] = useState('');
  return (
    <Form
      label="Asignar contraseña"
      successMessage="Contraseña asignada. Al entrar se le recomendará cambiarla."
      onSave={async () => {
        if (password !== repeated) {
          throw new Error('Las dos contraseñas no coinciden.');
        }
        try {
          await api('assign_user_password', { id: user.id, body: { password } });
        } finally {
          setPassword('');
          setRepeated('');
        }
        await onAssigned();
      }}
    >
      <p className="muted text-sm">
        {user.display_name} no tiene contraseña y no puede entrar. Asígnale una contraseña inicial y
        compártela con esa persona; al entrar se le recomendará cambiarla por una propia.
      </p>
      <div className="form-grid">
        <Field label="Contraseña inicial">
          <input
            required
            minLength={8}
            maxLength={256}
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </Field>
        <Field label="Escríbela otra vez">
          <input
            required
            minLength={8}
            maxLength={256}
            type="password"
            autoComplete="new-password"
            value={repeated}
            onChange={(event) => setRepeated(event.target.value)}
          />
        </Field>
      </div>
    </Form>
  );
}

// Excepción por permiso: la misma lista corta en cada uno de los ≈150 renglones (un solo arreglo
// compartido; el selector no monta su lista mientras está cerrado).
const overrideOptions = [
  { value: 'inherit', label: 'Heredar' },
  { value: 'allow', label: 'Permitir' },
  { value: 'deny', label: 'Denegar' },
];

function UserPermissionEditor({
  user,
  registry,
  editable,
  onSaved,
}: {
  user: User;
  registry: Permission[];
  editable: boolean;
  onSaved: () => Promise<void>;
}) {
  const resource = useResource<{
    effective: string[];
    overrides: { permission_key: string; effect: string }[];
  }>('user_permissions', { id: user.id });
  const [changes, setChanges] = useState<Record<string, string>>({});
  return (
    <>
      <Notice>{resource.error}</Notice>
      {resource.data && (
        <Form
          disabled={!editable}
          label="Guardar excepciones"
          successMessage="Excepciones de permisos guardadas."
          onSave={async () => {
            await api('save_user_permissions', { id: user.id, body: { overrides: changes } });
            await resource.reload();
            await onSaved();
          }}
        >
          <p className="muted text-sm">
            Permitir o denegar prevalece sobre el rol. Heredar elimina la excepción individual.
          </p>
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Permiso</th>
                  <th>Excepción</th>
                  <th>Efectivo actual</th>
                </tr>
              </thead>
              <tbody>
                {registry.map((permission) => {
                  const current =
                    resource.data?.overrides.find(
                      (override) => override.permission_key === permission.key,
                    )?.effect ?? 'inherit';
                  return (
                    <tr key={permission.key}>
                      <td>{permission.description}</td>
                      <td>
                        <Select
                          aria-label={`Excepción: ${permission.description}`}
                          value={changes[permission.key] ?? current}
                          options={overrideOptions}
                          onChange={(effect) =>
                            setChanges({ ...changes, [permission.key]: effect })
                          }
                        />
                      </td>
                      <td>
                        {resource.data?.effective.includes(permission.key)
                          ? 'Permitido'
                          : 'Denegado'}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </Form>
      )}
    </>
  );
}

export function UsersPage({
  can,
  onChanged,
}: {
  can: (key: string) => boolean;
  onChanged: () => Promise<void>;
}) {
  const users = useResource<User[]>('users');
  const roles = useResource<Role[]>('roles', { enabled: can('roles.read') });
  const registry = useResource<Permission[]>('permissions', { enabled: can('permissions.read') });
  const [selected, setSelected] = useState<User | null | undefined>(undefined);
  const [tab, setTab] = useState('details');
  // Etapa 9H: usuario sin contraseña al que se le asigna una.
  const [assigning, setAssigning] = useState<User | null>(null);
  async function saved() {
    await users.reload();
    await onChanged();
  }
  function edit(user: User | null) {
    setTab('details');
    setAssigning(null);
    setSelected(user);
  }
  return (
    <>
      <PageTitle
        title="Usuarios"
        description="Identidades del personal, roles principales y excepciones individuales."
      >
        {can('users.manage') && (
          <button className="primary-button" onClick={() => edit(null)}>
            Crear usuario
          </button>
        )}
      </PageTitle>
      <Notice>{users.error || roles.error || registry.error}</Notice>
      {assigning && (
        <section className="surface panel" aria-labelledby="assign-password-title">
          <div className="section-heading">
            <h2 id="assign-password-title">Asignar contraseña a {assigning.display_name}</h2>
            <button className="secondary-button" onClick={() => setAssigning(null)}>
              Cerrar
            </button>
          </div>
          <AssignPasswordForm
            key={assigning.id}
            user={assigning}
            onAssigned={async () => {
              setAssigning(null);
              await saved();
            }}
          />
        </section>
      )}
      {selected !== undefined && (
        <section className="surface panel">
          <div className="section-heading">
            <h2>{selected ? selected.display_name : 'Nuevo usuario'}</h2>
            <button className="secondary-button" onClick={() => setSelected(undefined)}>
              Cerrar editor
            </button>
          </div>
          {selected && (
            <div className="tabs">
              <button onClick={() => setTab('details')} aria-pressed={tab === 'details'}>
                Datos
              </button>
              {can('permissions.read') && (
                <button onClick={() => setTab('permissions')} aria-pressed={tab === 'permissions'}>
                  Permisos efectivos
                </button>
              )}
              {can('users.manage') && (
                <button onClick={() => setTab('preferences')} aria-pressed={tab === 'preferences'}>
                  Preferencias
                </button>
              )}
              {can('users.read') && (
                <button
                  onClick={() => setTab('kitchen-stations')}
                  aria-pressed={tab === 'kitchen-stations'}
                >
                  Estaciones de cocina
                </button>
              )}
            </div>
          )}
          {tab === 'details' && can('users.manage') && (
            <UserEditor
              key={selected?.id ?? 'new'}
              user={selected}
              roles={roles.data ?? []}
              onSaved={saved}
            />
          )}
          {tab === 'permissions' && selected && (
            <UserPermissionEditor
              key={selected.id}
              user={selected}
              registry={registry.data ?? []}
              editable={can('permissions.manage')}
              onSaved={saved}
            />
          )}
          {tab === 'preferences' && selected && (
            <OtherUserPreferences userID={selected.id} onSaved={onChanged} />
          )}
          {/* Etapa 9F (§5.2): «Estaciones que puede atender». */}
          {tab === 'kitchen-stations' && selected && (
            <UserKitchenStations key={selected.id} user={selected} editable={can('users.manage')} />
          )}
        </section>
      )}
      <div className="surface table-container">
        <table>
          <thead>
            <tr>
              <th>Usuario</th>
              <th>Nombre</th>
              <th>Rol</th>
              <th>Estado</th>
              <th>Acciones</th>
            </tr>
          </thead>
          <tbody>
            {users.data?.map((user) => (
              <tr key={user.id}>
                <td className="font-mono">{user.username}</td>
                <td>{user.display_name}</td>
                <td>{user.role_name}</td>
                <td>
                  <span className="users-state">
                    <span className={`tag ${user.active ? 'tag-good' : ''}`}>
                      {user.active ? 'Activo' : 'Inactivo'}
                    </span>
                    {/* Etapa 9H: sin contraseña no puede entrar (los importados del sistema anterior). */}
                    {user.has_password === false && (
                      <span className="tag tag-warning">Sin acceso</span>
                    )}
                  </span>
                </td>
                <td>
                  <span className="users-actions">
                    {can('users.manage') && user.has_password === false && (
                      <button
                        className="secondary-button"
                        onClick={() => {
                          setSelected(undefined);
                          setAssigning(user);
                        }}
                      >
                        Asignar contraseña
                      </button>
                    )}
                    {(can('users.manage') || can('permissions.read')) && (
                      <button
                        className="secondary-button"
                        onClick={() => {
                          edit(user);
                          if (!can('users.manage')) {
                            setTab('permissions');
                          }
                        }}
                      >
                        Administrar
                      </button>
                    )}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
