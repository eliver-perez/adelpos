import type { IconName } from '../components/Icon';

// Registro único de secciones de la aplicación (etapa 9C, fase A). Es la sola fuente de la
// navegación: la barra superior, los mosaicos de Administración, la ruta de navegación y la
// pantalla de inicio lo leen; la fase C lo usará también para el menú nativo, la paleta y los
// atajos. La aplicación no tiene rutas: App.tsx muestra la pantalla elegida y toda navegación pasa
// por un único requestScreen, que respeta la protección de cambios sin guardar.
//
// Las reglas de permisos son las de la etapa anterior (la tabla `screens` de PreferencesPage, que
// se movió aquí): solo deciden qué se muestra. Go vuelve a validar cada consulta y cada escritura.

export type ScreenID =
  | 'restaurant'
  | 'takeaway'
  | 'kitchen'
  | 'cash'
  | 'administration'
  | 'catalog'
  | 'inventory'
  | 'reports'
  | 'documents'
  | 'payments'
  | 'staff'
  | 'users'
  | 'roles'
  | 'sessions'
  | 'system'
  | 'settings'
  | 'devices'
  | 'registers'
  | 'audit'
  | 'status'
  | 'preferences'
  | 'password';

// operation: secciones de la barra superior; administration: Administración y lo que contiene;
// personal: pantallas del menú del usuario.
export type SectionGroup = 'operation' | 'administration' | 'personal';

// Pantalla de inicio de una terminal (devices.home_screen): lista cerrada de Go y del CHECK
// devices_home_screen_check de la migración 0020.
export type TerminalHome = 'tables' | 'takeaway' | 'kitchen' | 'cash';

export interface SectionEntry {
  id: ScreenID;
  label: string;
  icon: IconName;
  group: SectionGroup;
  // Pantalla que la contiene (mosaico padre o pantalla desde la que se abre): forma la ruta.
  parent?: ScreenID;
  // Línea de descripción del mosaico.
  description?: string;
  // Permiso exacto para ver la pantalla.
  permission?: string;
  // Basta con uno de estos permisos.
  anyPermission?: string[];
  // Además del anterior, todos estos.
  allPermissions?: string[];
  // Pantalla de mosaicos que agrupa otras: se ve si alguna de sus pantallas se ve.
  grouping?: boolean;
  // Se abre desde otra pantalla (no aparece como mosaico).
  hiddenTile?: boolean;
  // Valor de devices.home_screen que abre esta sección.
  home?: TerminalHome;
}

export const sections: SectionEntry[] = [
  {
    id: 'restaurant',
    label: 'Mesas',
    icon: 'table',
    group: 'operation',
    permission: 'accounts.read',
    home: 'tables',
  },
  {
    id: 'takeaway',
    label: 'Para llevar',
    icon: 'bag',
    group: 'operation',
    permission: 'takeaway.read',
    allPermissions: ['accounts.read'],
    home: 'takeaway',
  },
  {
    id: 'kitchen',
    label: 'Cocina',
    icon: 'kitchen',
    group: 'operation',
    permission: 'kitchen.states.read',
    home: 'kitchen',
  },
  {
    id: 'cash',
    label: 'Caja',
    icon: 'cash',
    group: 'operation',
    permission: 'cash.shifts.read',
    home: 'cash',
  },
  {
    id: 'administration',
    label: 'Administración',
    icon: 'grid',
    group: 'administration',
    grouping: true,
    description: 'Catálogo, inventarios, reportes, documentos, cobros, personal y sistema.',
  },
  {
    id: 'catalog',
    label: 'Catálogo',
    icon: 'tag',
    group: 'administration',
    parent: 'administration',
    permission: 'catalog.read',
    description:
      'Categorías, productos, combos, complementos, porciones, grupos, listas de precios y promociones.',
  },
  {
    id: 'inventory',
    label: 'Inventarios',
    icon: 'inventory',
    group: 'administration',
    parent: 'administration',
    permission: 'inventory.read',
    description: 'Insumos, recetas, almacenes, existencias, compras y conteos.',
  },
  {
    id: 'reports',
    label: 'Reportes',
    icon: 'report',
    group: 'administration',
    parent: 'administration',
    permission: 'reports.read',
    description: 'Resumen ejecutivo y reportes de ventas, caja, cobros e inventario.',
  },
  {
    id: 'documents',
    label: 'Documentos',
    icon: 'document',
    group: 'administration',
    parent: 'administration',
    permission: 'documents.templates.read',
    description: 'Plantillas de tickets y documentos e identidad del establecimiento.',
  },
  {
    // Decisión del coordinador (Fase 0, 5): el mosaico Cobros se ve solo con los permisos de
    // administración de cobros; el turno actual se consulta en Caja. No cambia ningún permiso.
    id: 'payments',
    label: 'Cobros',
    icon: 'card',
    group: 'administration',
    parent: 'administration',
    anyPermission: ['payments.methods.manage', 'payments.settings.manage', 'payments.tax.manage'],
    description: 'Métodos de pago, IVA, políticas de cobro y motivos de descuento.',
  },
  {
    id: 'staff',
    label: 'Personal',
    icon: 'users',
    group: 'administration',
    parent: 'administration',
    grouping: true,
    description: 'Usuarios, roles y sesiones.',
  },
  {
    id: 'users',
    label: 'Usuarios',
    icon: 'user',
    group: 'administration',
    parent: 'staff',
    permission: 'users.read',
    description: 'Cuentas de acceso, permisos individuales y apariencia de cada usuario.',
  },
  {
    id: 'roles',
    label: 'Roles',
    icon: 'shield',
    group: 'administration',
    parent: 'staff',
    permission: 'roles.read',
    description: 'Permisos de cada rol y su límite de descuento.',
  },
  {
    id: 'sessions',
    label: 'Sesiones',
    icon: 'key',
    group: 'administration',
    parent: 'staff',
    permission: 'sessions.read',
    description: 'Sesiones abiertas en cada terminal.',
  },
  {
    id: 'system',
    label: 'Sistema',
    icon: 'gear',
    group: 'administration',
    parent: 'administration',
    grouping: true,
    description: 'Configuración general, terminales, cajas físicas y registro de movimientos.',
  },
  {
    id: 'settings',
    label: 'Configuración',
    icon: 'gear',
    group: 'administration',
    parent: 'system',
    permission: 'settings.read',
    description:
      'Datos del establecimiento, hora de inicio del día, funciones y estado del sistema.',
  },
  {
    id: 'devices',
    label: 'Terminales',
    icon: 'monitor',
    group: 'administration',
    parent: 'system',
    permission: 'devices.read',
    description: 'Terminales autorizadas, su caja, su teclado y su pantalla inicial.',
  },
  {
    id: 'registers',
    label: 'Cajas físicas',
    icon: 'register',
    group: 'administration',
    parent: 'system',
    permission: 'registers.read',
    description: 'Cajas del establecimiento que usan las terminales.',
  },
  {
    // §4.2: «Auditoría» se llama «Registro de movimientos»; la clave audit.read, la ruta y los
    // datos no cambian.
    id: 'audit',
    label: 'Registro de movimientos',
    icon: 'history',
    group: 'administration',
    parent: 'system',
    permission: 'audit.read',
    description: 'Quién hizo cada cambio administrativo y cuándo.',
  },
  {
    // §4.1: «Estado del sistema» ya no es una sección ni el inicio; se abre con «Ver estado del
    // sistema» en Configuración.
    id: 'status',
    label: 'Estado del sistema',
    icon: 'activity',
    group: 'administration',
    parent: 'settings',
    permission: 'settings.read',
    hiddenTile: true,
  },
  {
    id: 'preferences',
    label: 'Mis preferencias',
    icon: 'user',
    group: 'personal',
  },
  {
    id: 'password',
    label: 'Cambiar contraseña',
    icon: 'key',
    group: 'personal',
  },
];

export function sectionByID(id: ScreenID): SectionEntry {
  const entry = sections.find((candidate) => candidate.id === id);
  if (!entry) {
    throw new Error(`Sección desconocida: ${id}`);
  }
  return entry;
}

// Una pantalla se autoriza con su permiso propio, alguno de sus alternativos y todos los
// adicionales. Una pantalla de mosaicos se autoriza si alguna de las que agrupa se autoriza.
export function screenAuthorized(entry: SectionEntry, permissions: string[]): boolean {
  if (entry.grouping) {
    return sections.some(
      (child) =>
        child.parent === entry.id && !child.hiddenTile && screenAuthorized(child, permissions),
    );
  }
  if (entry.permission && !permissions.includes(entry.permission)) {
    return false;
  }
  if (entry.anyPermission && !entry.anyPermission.some((key) => permissions.includes(key))) {
    return false;
  }
  if (entry.allPermissions && !entry.allPermissions.every((key) => permissions.includes(key))) {
    return false;
  }
  return true;
}

export function canOpen(id: ScreenID, permissions: string[]) {
  return screenAuthorized(sectionByID(id), permissions);
}

// Secciones operativas de la barra superior que el usuario puede ver, en su orden.
export function barSections(permissions: string[]) {
  return sections.filter(
    (entry) => entry.group === 'operation' && screenAuthorized(entry, permissions),
  );
}

// Mosaicos visibles de una pantalla de mosaicos (Administración, Personal o Sistema).
export function tilesOf(parent: ScreenID, permissions: string[]) {
  return sections.filter(
    (entry) => entry.parent === parent && !entry.hiddenTile && screenAuthorized(entry, permissions),
  );
}

// Ruta de navegación desde Administración hasta la pantalla (incluida). Vacía fuera de
// Administración.
export function breadcrumbOf(id: ScreenID): SectionEntry[] {
  const entry = sectionByID(id);
  if (entry.group !== 'administration') {
    return [];
  }
  const chain: SectionEntry[] = [];
  let current: SectionEntry | undefined = entry;
  while (current) {
    chain.unshift(current);
    current = current.parent ? sectionByID(current.parent) : undefined;
  }
  return chain;
}

// Sección resaltada en la barra superior para la pantalla visible.
export function barHighlight(id: ScreenID): ScreenID | null {
  const entry = sectionByID(id);
  if (entry.group === 'operation') {
    return entry.id;
  }
  if (entry.group === 'administration') {
    return 'administration';
  }
  return null;
}

export const terminalHomes: { value: TerminalHome; label: string }[] = [
  { value: 'tables', label: 'Mesas' },
  { value: 'takeaway', label: 'Para llevar' },
  { value: 'kitchen', label: 'Cocina' },
  { value: 'cash', label: 'Caja' },
];

// Pantalla en la que abre la aplicación y a la que vuelve el logotipo (§1.5 y §4.1): la de la
// terminal (Mesas si no informa ninguna); si el usuario no puede verla, la primera sección de la
// barra que sí puede ver; si no ve ninguna, Administración, y si tampoco, Mis preferencias. Nunca
// «Estado del sistema». La preferencia por usuario ya no decide el inicio.
export function resolveHome(home: string | undefined, permissions: string[]): ScreenID {
  const wanted = sections.find((entry) => entry.home === (home || 'tables'));
  if (wanted && screenAuthorized(wanted, permissions)) {
    return wanted.id;
  }
  const first = barSections(permissions)[0];
  if (first) {
    return first.id;
  }
  if (canOpen('administration', permissions)) {
    return 'administration';
  }
  return 'preferences';
}
