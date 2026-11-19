// Etapa A3 (§3.2 de docs/etapas/ETAPA_A3_AJUSTES.md): el catálogo de permisos lo informa Go
// (GET permissions/catalog, backend/internal/administration/permission_catalog.go) con los grupos, la
// descripción corta, lo que necesita cada permiso y las capacidades de la vista sencilla. Aquí solo
// viven las reglas de la pantalla con esos datos (marcar lo que se necesita, avisar qué deja de
// funcionar, el estado de cada capacidad y el buscador); la interfaz no tiene listas propias y Go
// sigue validando cada permiso en cada ruta.

export interface CatalogPermission {
  key: string;
  name: string;
  description: string;
  group: string;
  requires: string[];
}

export interface PermissionGroup {
  code: string;
  name: string;
  permissions: string[];
}

export interface PermissionCapability {
  code: string;
  name: string;
  description: string;
  permissions: string[];
}

export interface PermissionCatalog {
  groups: PermissionGroup[];
  permissions: CatalogPermission[];
  capabilities: PermissionCapability[];
  // Roles de fábrica (por código) que tienen «Restablecer permisos predeterminados».
  factory_roles: string[];
}

// Vista previa de «Restablecer permisos predeterminados» (GET roles/{id}/defaults).
export interface RoleDefaultsPreview {
  role: { id: string; code: string; name: string };
  current: string[];
  factory: string[];
  add: string[];
  remove: string[];
}

// Resultado de la confirmación (POST roles/{id}/defaults).
export interface RoleDefaultsResult {
  role: { id: string; permissions: string[] };
  changed: boolean;
  added: string[];
  removed: string[];
}

// Un cambio de la selección con lo que se marcó solo y lo que dejará de funcionar, para avisarlo.
export interface SelectionChange {
  selected: string[];
  added: string[];
  broken: string[];
}

export type CapabilityState = 'all' | 'some' | 'none';

function requirementsOf(catalog: PermissionCatalog) {
  const requires = new Map<string, string[]>();
  for (const permission of catalog.permissions) {
    requires.set(permission.key, permission.requires);
  }
  return requires;
}

// catalogPermissionName es el nombre visible de un permiso (la clave si Go no lo informa).
export function catalogPermissionName(catalog: PermissionCatalog, key: string) {
  return catalog.permissions.find((permission) => permission.key === key)?.name ?? key;
}

// withRequirements agrega a la selección los permisos pedidos y, uno por uno, lo que necesitan,
// hasta que no falte nada. added son los que se marcaron solos (los que no se pidieron).
export function withRequirements(
  catalog: PermissionCatalog,
  selected: string[],
  wanted: string[],
): SelectionChange {
  const requires = requirementsOf(catalog);
  const result = [...selected];
  const added: string[] = [];
  const pending = [...wanted];
  while (pending.length > 0) {
    const key = pending.shift()!;
    if (!result.includes(key)) {
      result.push(key);
      if (!wanted.includes(key)) {
        added.push(key);
      }
    }
    for (const need of requires.get(key) ?? []) {
      if (!result.includes(need)) {
        pending.push(need);
      }
    }
  }
  return { selected: result, added, broken: [] };
}

// brokenBy son los permisos que quedan en la selección y necesitan (directa o indirectamente)
// alguno de los que se quitaron: dejarán de funcionar.
export function brokenBy(catalog: PermissionCatalog, remaining: string[], removed: string[]) {
  const requires = requirementsOf(catalog);
  const needsRemoved = (key: string, visited: Set<string>): boolean => {
    for (const need of requires.get(key) ?? []) {
      if (removed.includes(need)) {
        return true;
      }
      if (!visited.has(need)) {
        visited.add(need);
        if (needsRemoved(need, visited)) {
          return true;
        }
      }
    }
    return false;
  };
  return remaining.filter((key) => needsRemoved(key, new Set([key])));
}

// withoutPermissions quita los permisos indicados (no quita otros: solo avisa cuáles dejarán de
// funcionar, §3.2).
export function withoutPermissions(
  catalog: PermissionCatalog,
  selected: string[],
  removed: string[],
): SelectionChange {
  const remaining = selected.filter((key) => !removed.includes(key));
  return { selected: remaining, added: [], broken: brokenBy(catalog, remaining, removed) };
}

// capabilityState: completa si el rol tiene todos sus permisos, parcial si solo una parte.
export function capabilityState(
  capability: PermissionCapability,
  selected: string[],
): CapabilityState {
  const held = capability.permissions.filter((key) => selected.includes(key)).length;
  if (held === 0) {
    return 'none';
  }
  return held === capability.permissions.length ? 'all' : 'some';
}

// toggleCapability marca la capacidad completa (con lo que necesita) si no lo está; si está completa,
// quita sus permisos salvo los que forman parte de otra capacidad completa, que se conserva.
export function toggleCapability(
  catalog: PermissionCatalog,
  selected: string[],
  capability: PermissionCapability,
): SelectionChange {
  if (capabilityState(capability, selected) !== 'all') {
    return withRequirements(catalog, selected, capability.permissions);
  }
  const kept = new Set<string>();
  for (const other of catalog.capabilities) {
    if (other.code !== capability.code && capabilityState(other, selected) === 'all') {
      other.permissions.forEach((key) => kept.add(key));
    }
  }
  return withoutPermissions(
    catalog,
    selected,
    capability.permissions.filter((key) => !kept.has(key)),
  );
}

// Texto sin mayúsculas ni acentos, para que «camara» encuentre «Cámara».
function folded(text: string) {
  return text.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
}

// matchesSearch: el buscador filtra por el nombre y la descripción corta.
export function matchesSearch(permission: CatalogPermission, search: string) {
  const wanted = folded(search.trim());
  if (wanted === '') {
    return true;
  }
  return (
    folded(permission.name).includes(wanted) || folded(permission.description).includes(wanted)
  );
}
