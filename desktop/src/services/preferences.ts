import { api } from './api';
import type { Preferences } from './models';

// Guarda la apariencia de un usuario (Mis preferencias o, con userID, la de otro usuario desde
// Usuarios). Desde la etapa 9C la usan el formulario de Mis preferencias y la acción «Tema claro u
// oscuro» del menú nativo y la paleta: es la misma operación. La pantalla inicial se reenvía sin
// cambios (Go conserva el campo por compatibilidad).
export async function savePreferences(preferences: Preferences, userID?: string) {
  await api(userID ? 'save_user_preferences' : 'save_preferences', {
    id: userID,
    body: { theme: preferences.theme, home_screen: preferences.home_screen },
  });
}
