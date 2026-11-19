import { useState } from 'react';
import { useConfirmDialog } from '../components/ConfirmDialog';
import { Field, Form, Notice, PageTitle } from '../components/Forms';
import { GroupedPermissionEditor, PermissionChecklist } from '../components/PermissionEditor';
import { useResource } from '../hooks/useResource';
import { api, APIError } from '../services/api';
import { acceptAmountText, toWireAmount } from '../services/amountInput';
import type { Permission, Role } from '../services/models';
import { notifyError, notifyInfo, notifySuccess, notifyWarning } from '../services/notify';
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

  // Revisión R5 (A3-D20): tras ROLE_CHANGED, los permisos que Go tiene ahora, para que un «Guardar
  // rol» posterior no pise sin aviso el cambio de la otra persona.
  async function currentPermissions(): Promise<string[] | null> {
    try {
      const roles = await api<Role[]>('roles');
      return roles.find((candidate) => candidate.id === role.id)?.permissions ?? null;
    } catch {
      return null;
    }
  }

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
        const current = await currentPermissions();
        if (current) {
          notifyWarning(
            `Otra persona cambió los permisos de ${role.name}: se cargaron los que tiene ahora y se descartaron los cambios sin guardar de esta pantalla.`,
          );
          return current;
        }
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

// Revisión R2 (A3-D17): la pantalla no ofrece «Restablecer permisos predeterminados» donde Go lo
// rechazaría (las reglas de role_defaults.go): el rol propio, un rol o un conjunto de fábrica con
// permisos que quien edita no tiene, o un límite de descuento mayor que el suyo. En su lugar dice por
// qué. Go sigue validando todo al restablecer.
export function roleDefaultsBlocker({
  role,
  catalog,
  can,
  currentRoleID,
  actorLimit,
}: {
  role: Role;
  catalog: PermissionCatalog;
  can: (key: string) => boolean;
  currentRoleID?: string;
  actorLimit: number;
}): string | null {
  if (currentRoleID && role.id === currentRoleID) {
    return 'Tu propio rol no se restablece desde tu usuario: pídeselo a otro administrador.';
  }
  if (role.permissions.some((key) => !can(key))) {
    return 'Este rol tiene permisos que tu usuario no tiene: solo un administrador con esos permisos puede restablecerlo.';
  }
  const factory = role.code ? (catalog.factory_permissions?.[role.code] ?? []) : [];
  if (factory.some((key) => !can(key))) {
    return 'Los permisos predeterminados de este rol incluyen permisos que tu usuario no tiene: solo un administrador con esos permisos puede restablecerlo.';
  }
  if (Number(role.discount_limit ?? 0) > actorLimit) {
    return 'Este rol tiene un límite de descuento mayor que el tuyo: solo un administrador con un límite igual o mayor puede restablecerlo.';
  }
  return null;
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
  defaultsBlocker,
  onSaved,
  onCancel,
}: {
  role: Role | null;
  registry: Permission[];
  catalog: PermissionCatalog | null;
  // Por qué no se ofrece «Restablecer permisos predeterminados» (null: se ofrece).
  defaultsBlocker: string | null;
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
          {factory && catalog && !defaultsBlocker && (
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
      {factory && catalog && defaultsBlocker && (
        <p className="muted mb-4 text-sm">{defaultsBlocker}</p>
      )}
      <Form
        label="Guardar rol"
        successMessage="Rol guardado."
        onSave={async () => {
          await api(role ? 'update_role' : 'create_role', {
            id: role?.id,
            // La cadena vacía elimina el límite; enviar el valor vigente lo conserva.
            body: { name, permissions: selected, discount_limit: toWireAmount(discountLimit) },
          });
          setSaved(selected);
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
  currentRoleID,
  support = false,
}: {
  can: (key: string) => boolean;
  onChanged: () => Promise<void>;
  // El rol de la sesión y si es el soporte reservado (auth/me): Go no deja restablecer el propio rol, y
  // el límite de descuento de quien edita es el de su rol (el soporte, 100 %).
  currentRoleID?: string;
  support?: boolean;
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
  // Límite de descuento de quien edita: el de su rol (el soporte, 100 %). Sin saber su rol, decide Go.
  const ownRole = roles.data?.find((role) => role.id === currentRoleID);
  const actorLimit = support
    ? 100
    : currentRoleID
      ? Number(ownRole?.discount_limit ?? 0)
      : Number.POSITIVE_INFINITY;
  // El rol en edición como está ahora en la lista (después de guardar o restablecer).
  const current = editing ? (roles.data?.find((role) => role.id === editing.id) ?? editing) : null;
  const defaultsBlocker =
    current && catalog.data
      ? roleDefaultsBlocker({
          role: current,
          catalog: catalog.data,
          can,
          currentRoleID,
          actorLimit,
        })
      : null;
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
          defaultsBlocker={defaultsBlocker}
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
