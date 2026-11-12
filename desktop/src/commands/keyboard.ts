// Reglas de la capa de teclado (etapa 9C, fase C): cuándo un atajo no debe actuar y a qué buscador
// lleva ⌘F / Ctrl+F.

const textInputTypes = new Set([
  '',
  'text',
  'search',
  'email',
  'password',
  'tel',
  'url',
  'number',
  'date',
  'datetime-local',
  'time',
  'month',
  'week',
]);

const editableRoles = new Set(['textbox', 'searchbox', 'combobox', 'spinbutton']);

// ¿El foco está en un campo donde se escribe? Ahí los atajos no actúan (salvo Escape).
export function isEditable(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  if (target.isContentEditable) {
    return true;
  }
  if (target instanceof HTMLTextAreaElement || target instanceof HTMLSelectElement) {
    return true;
  }
  if (target instanceof HTMLInputElement) {
    return textInputTypes.has(target.type);
  }
  const role = target.getAttribute('role');
  return !!role && editableRoles.has(role);
}

// Elementos que se ven en pantalla: la operación oculta con display: none, un acordeón cerrado
// (inert) o un elemento hidden no cuentan.
export function isShown(element: Element) {
  for (let current: Element | null = element; current; current = current.parentElement) {
    if (!(current instanceof HTMLElement)) {
      continue;
    }
    if (current.hidden || current.inert) {
      return false;
    }
    const style = getComputedStyle(current);
    if (style.display === 'none' || style.visibility === 'hidden') {
      return false;
    }
  }
  return true;
}

// Diálogos modales, hojas, menús y listas desplegables abiertos. Cada uno atiende su propio Escape
// (ConfirmDialog, la hoja y el menú rápido de Caja, «Más…», el menú del usuario, la paleta de
// colores, el selector de la fase B…), así que mientras alguno está abierto la capa global no
// actúa.
const overlaySelector = [
  '[aria-modal="true"]',
  '[role="dialog"]',
  '[role="alertdialog"]',
  '[role="listbox"]',
  '[role="menu"]',
  '.cash-sheet-backdrop',
  '.cash-shortcut-menu',
  '.topbar-menu',
  '.capture-more-menu',
  // Etapa 9D (§4.5): menú de un producto de la cuenta.
  '.capture-line-menu',
  '.color-palette',
].join(',');

export function overlayOpen(root: ParentNode = document) {
  // Una lista de opciones solo cuenta si está desplegada a la vista.
  return Array.from(root.querySelectorAll(overlaySelector)).some(isShown);
}

export function modalOpen(root: ParentNode = document) {
  return Array.from(root.querySelectorAll('[aria-modal="true"]')).some(isShown);
}

// Un control propio con foco (por ejemplo, el gráfico de tendencia con sus flechas y su Escape) se
// queda con Escape.
export function ownsEscape(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  if (target === document.body || target === document.documentElement) {
    return false;
  }
  const native = target.matches('button, a[href], input, select, textarea, summary');
  return !native && target.hasAttribute('tabindex');
}

// Pantalla con un formulario a la vista: Escape no sale de ella para no descartar lo capturado.
export function formShown(root: ParentNode) {
  return Array.from(root.querySelectorAll('form')).some(isShown);
}

// Buscador de la pantalla visible: el primero a la vista dentro del contenido (productos en la
// captura, cuentas en Para llevar, tablas en Administración). Una pantalla puede marcar el suyo con
// data-screen-search.
export function screenSearch(root: ParentNode): HTMLInputElement | null {
  const candidates = Array.from(
    root.querySelectorAll<HTMLInputElement>(
      'input[data-screen-search], input[type="search"]:not([disabled])',
    ),
  ).filter(isShown);
  const marked = candidates.find((input) => input.hasAttribute('data-screen-search'));
  return marked ?? candidates[0] ?? null;
}
