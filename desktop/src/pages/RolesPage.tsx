import { useState } from 'react';
import { useConfirmDialog } from '../components/ConfirmDialog';
import { Field, Form, Notice, PageTitle } from '../components/Forms';
import { GroupedPermissionEditor, PermissionChecklist } from '../components/PermissionEditor';
import { useResource } from '../hooks/useResource';
import { api, APIError } from '../services/api';
import { acceptAmountText, toWireAmount } from '../services/amountInput';
import type { Permission, Role } from '../services/models';
import { notifyError, notifyInfo, notifySuccess } from '../services/notify';
import { catalogPermissionName } from '../services/permissionCatalog';
import type {
  PermissionCatalog,
  RoleDefaultsPreview,
  RoleDefaultsResult,
} from '../services/permissionCatalog';
import { permissionGroupsCapability, serverHas } from '../services/serverCapabilities';

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

// Lista de nombres para la confirmación de «Restablecer permisos predeterminados».
function DefaultsChanges({
  title,
  keys,
  catalog,
}: {
  title: string;
  keys: string[];
  catalog: PermissionCatalog;
}) {
  if (keys.length === 0) {
    return null;
  }
  return (
    <div className="mt-3">
      <strong>
        {title} ({keys.length}):
      </strong>
      <ul className="list-disc pl-5">
        {keys.map((key) => (
          <li key={key}>{catalogPermissionName(catalog, key)}</li>
        ))}
      </ul>
    </div>
  );
}

// Etapa A3 (§3.1, A3-D6): «Restablecer permisos predeterminados» de un rol de fábrica. Primero la
// vista previa de Go (qué se agrega y qué se quita), después la confirmación con lo que mostró: si el
// rol cambió mientras tanto, Go responde ROLE_CHANGED y no cambia nada. Las reglas (quién puede, rol
// propio, último administrador) las aplica Go como al editar.
function useRoleDefaults(role: Role, catalog: PermissionCatalog) {
  const { confirm, dialog } = useConfirmDialog();
  const [busy, setBusy] = useState(false);

  async function restore(unsaved: boolean): Promise<string[] | null> {
    setBusy(true);
    try {
      const preview = await api<RoleDefaultsPreview>('role_defaults', { id: role.id });
      if (preview.add.length === 0 && preview.remove.length === 0) {
        notifyInfo(`${role.name} ya tiene sus permisos predeterminados.`);
        return preview.current;
      }
      const confirmed = await confirm({
        title: `Restablecer permisos de ${role.name}`,
        message: (
          <>
            <p>El rol quedará con los permisos con que llega una instalación nueva.</p>
            <DefaultsChanges title="Se agregarán" keys={preview.add} catalog={catalog} />
            <DefaultsChanges title="Se quitarán" keys={preview.remove} catalog={catalog} />
            <p className="mt-3">
              No cambian el límite de descuento del rol ni los permisos propios de cada usuario. El
              cambio queda en el Registro de movimientos.
            </p>
            {unsaved && (
              <p className="mt-3">Se pierden los cambios sin guardar de esta pantalla.</p>
            )}
          </>
        ),
        confirmLabel: 'Restablecer permisos',
        cancelLabel: 'Volver',
      });
      if (!confirmed) {
        return null;
      }
      const result = await api<RoleDefaultsResult>('restore_role_defaults', {
        id: role.id,
        body: { expected: preview.current },
      });
      notifySuccess(`Permisos predeterminados de ${role.name} restablecidos.`);
      return result.role.permissions;
    } catch (error) {
      if (error instanceof APIError && error.code === 'ROLE_CHANGED') {
        notifyError(error, 'Los permisos del rol cambiaron. Vuelve a intentarlo.');
        return null;
      }
      notifyError(error, 'No se pudieron restablecer los permisos.');
      return null;
    } finally {
      setBusy(false);
    }
  }

  return { restore, busy, dialog };
}

function RoleDefaultsButton({
  role,
  catalog,
  unsaved,
  onRestored,
}: {
  role: Role;
  catalog: PermissionCatalog;
  unsaved: boolean;
  onRestored: (permissions: string[]) => Promise<void>;
}) {
  const { restore, busy, dialog } = useRoleDefaults(role, catalog);
  return (
    <>
      <button
        type="button"
        className="secondary-button"
        disabled={busy}
        aria-busy={busy}
        onClick={async () => {
          const permissions = await restore(unsaved);
          if (permissions) {
            await onRestored(permissions);
          }
        }}
      >
        Restablecer permisos predeterminados
      </button>
      {dialog}
    </>
  );
}

function RoleEditor({
  role,
  registry,
  catalog,
  onSaved,
  onCancel,
}: {
  role: Role | null;
  registry: Permission[];
  catalog: PermissionCatalog | null;
  onSaved: () => Promise<void>;
  onCancel: () => void;
}) {
  const [name, setName] = useState(role?.name ?? '');
  const [selected, setSelected] = useState(role?.permissions ?? []);
  const [saved, setSaved] = useState(role?.permissions ?? []);
  const [discountLimit, setDiscountLimit] = useState(role?.discount_limit ?? '');
  // Solo los roles de fábrica que informa Go tienen el botón (no el Administrador ni los del usuario).
  const factory = role?.code && catalog?.factory_roles.includes(role.code) ? role : null;
  const unsaved = selected.length !== saved.length || selected.some((key) => !saved.includes(key));
  return (
    <section className="surface panel">
      <div className="section-heading">
        <h2>{role ? 'Editar rol' : 'Nuevo rol'}</h2>
        <div className="button-row">
          {factory && catalog && (
            <RoleDefaultsButton
              role={factory}
              catalog={catalog}
              unsaved={unsaved}
              onRestored={async (permissions) => {
                setSelected(permissions);
                setSaved(permissions);
                await onSaved();
              }}
            />
          )}
          <button type="button" onClick={onCancel} className="secondary-button">
            Cerrar editor
          </button>
        </div>
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
        {catalog ? (
          <GroupedPermissionEditor catalog={catalog} selected={selected} onChange={setSelected} />
        ) : (
          <PermissionChecklist registry={registry} selected={selected} onChange={setSelected} />
        )}
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
  // Etapa A3: con un servidor que tiene el catálogo agrupado, la pantalla nueva; con uno anterior, la
  // lista de siempre.
  const grouped = serverHas(permissionGroupsCapability);
  const registry = useResource<Permission[]>('permissions', {
    enabled: can('permissions.read') && !grouped,
  });
  const catalog = useResource<PermissionCatalog>('permission_catalog', {
    enabled: can('permissions.read') && grouped,
  });
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
      <Notice>{roles.error || registry.error || catalog.error}</Notice>
      {editing !== undefined && (
        <RoleEditor
          key={editing?.id ?? 'new'}
          role={editing}
          registry={registry.data ?? []}
          catalog={grouped ? catalog.data : null}
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
