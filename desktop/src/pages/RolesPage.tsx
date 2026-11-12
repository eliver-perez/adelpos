import { useState } from 'react';
import { Field, Form, Notice, PageTitle } from '../components/Forms';
import { PermissionChecklist } from '../components/PermissionEditor';
import { useResource } from '../hooks/useResource';
import { api } from '../services/api';
import { acceptAmountText, toWireAmount } from '../services/amountInput';
import type { Permission, Role } from '../services/models';

// El límite es un porcentaje de 0 a 100 con hasta dos decimales; Go lo valida y además impide
// conceder un límite mayor que el del propio usuario.
function DiscountLimitField({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <Field
      label="Límite de descuento (%) · opcional"
      hint="Descuentos por encima de este límite requieren autorización de un supervisor. Vacío: sin límite configurado (0 %)."
    >
      <input
        type="text"
        inputMode="decimal"
        autoComplete="off"
        maxLength={6}
        value={value}
        onChange={(event) => onChange(acceptAmountText(event.target.value, value))}
      />
    </Field>
  );
}

function limitLabel(limit: string | null | undefined) {
  if (limit === null || limit === undefined || limit === '') {
    return 'Sin límite configurado (0 %)';
  }
  return `${limit} %`;
}

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
  const [discountLimit, setDiscountLimit] = useState(role?.discount_limit ?? '');
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
        successMessage="Rol guardado."
        onSave={async () => {
          await api(role ? 'update_role' : 'create_role', {
            id: role?.id,
            // La cadena vacía elimina el límite; enviar el valor vigente lo conserva.
            body: { name, permissions: selected, discount_limit: toWireAmount(discountLimit) },
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
        <DiscountLimitField value={discountLimit} onChange={setDiscountLimit} />
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
        description="Cada rol reúne los permisos de un puesto de trabajo; cada usuario tiene un rol."
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
              <th>Límite de descuento</th>
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
                <td>{limitLabel(role.discount_limit)}</td>
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
