import { actionByID } from './actions';
import type { ActionID } from './actions';
import { ariaShortcut, currentPlatform, shortcutLabel } from './platform';

// Atajo de una acción para la descripción de su botón (title) y aria-keyshortcuts (§3.2: los atajos
// aparecen junto a cada opción en el menú nativo, en la paleta y en las descripciones de los
// botones).
export function shortcutHint(id: ActionID) {
  const shortcut = actionByID(id)?.shortcut;
  if (!shortcut) {
    return null;
  }
  const platform = currentPlatform();
  return { label: shortcutLabel(shortcut, platform), aria: ariaShortcut(shortcut, platform) };
}

// Propiedades para un botón que hace lo mismo que la acción: «Mesas · ⌘1».
export function shortcutProps(id: ActionID, label: string) {
  const hint = shortcutHint(id);
  if (!hint) {
    return {};
  }
  return { title: `${label} · ${hint.label}`, 'aria-keyshortcuts': hint.aria };
}
