import { useState } from 'react';
import { Field, Form, Notice, PageTitle } from '../components/Forms';
import { PermissionChecklist } from '../components/PermissionEditor';
import { useResource } from '../hooks/useResource';
import { api } from '../services/api';
import type { Permission, Role } from '../services/models';

function RoleEditor({
  role,
  registry,
  onSaved,
  onCancel,
}: {
  role: Role | null;
  registry: Permission[];
  onSaved: () => Promise<void>;
  onCancel: () => void;
}) {
  const [name, setName] = useState(role?.name ?? '');
  const [selected, setSelected] = useState(role?.permissions ?? []);
  return (
    <section className="surface panel">
      <div className="section-heading">
        <h2>{role ? 'Editar rol' : 'Nuevo rol'}</h2>
        <button type="button" onClick={onCancel} className="secondary-button">
          Cerrar editor
        </button>
      </div>
      <Form
        label="Guardar rol"
        onSave={async () => {
          await api(role ? 'update_role' : 'create_role', {
            id: role?.id,
            body: { name, permissions: selected },
          });
          await onSaved();
        }}
      >
        <Field label="Nombre del rol">
          <input
            required
            maxLength={80}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        <PermissionChecklist registry={registry} selected={selected} onChange={setSelected} />
      </Form>
    </section>
  );
}

export function RolesPage({
  can,
  onChanged,
}: {
  can: (key: string) => boolean;
  onChanged: () => Promise<void>;
}) {
  const roles = useResource<Role[]>('roles');
  const registry = useResource<Permission[]>('permissions', { enabled: can('permissions.read') });
  const [editing, setEditing] = useState<Role | null | undefined>(undefined);
  const editable = can('roles.manage') && can('permissions.manage') && can('permissions.read');
  return (
    <>
      <PageTitle
        title="Roles y permisos"
        description="Perfiles persistentes para las operaciones implementadas."
      >
        {editable && (
          <button className="primary-button" onClick={() => setEditing(null)}>
            Crear rol
          </button>
        )}
      </PageTitle>
      <Notice>{roles.error || registry.error}</Notice>
      {editing !== undefined && (
        <RoleEditor
          key={editing?.id ?? 'new'}
          role={editing}
          registry={registry.data ?? []}
          onCancel={() => setEditing(undefined)}
          onSaved={async () => {
            await roles.reload();
            await onChanged();
          }}
        />
      )}
      <div className="surface table-container">
        <table>
          <thead>
            <tr>
              <th>Rol</th>
              <th>Permisos concedidos</th>
              <th>Origen</th>
              <th>Acciones</th>
            </tr>
          </thead>
          <tbody>
            {roles.data?.map((role) => (
              <tr key={role.id}>
                <td>
                  <strong>{role.name}</strong>
                </td>
                <td>{role.permissions.length}</td>
                <td>{role.code ? 'Inicial' : 'Personalizado'}</td>
                <td>
                  {editable && (
                    <button className="secondary-button" onClick={() => setEditing(role)}>
                      Editar permisos
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
