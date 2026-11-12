import { createContext, useContext, useEffect } from 'react';
import type { ScreenID } from './sections';

// Navegación compartida (etapa 9C): la pantalla visible, el inicio de la terminal y el único
// requestScreen de App.tsx, que respeta la protección de cambios sin guardar. Los mosaicos, la ruta
// y, en la fase C, el menú nativo, la paleta y los atajos navegan siempre por aquí.
export interface NavigationValue {
  screen: ScreenID;
  home: ScreenID;
  requestScreen: (next: ScreenID) => void;
  // Último tramo de la ruta que informa la pantalla visible (por ejemplo, su pestaña actual).
  setTail: (owner: ScreenID, label: string | null) => void;
}

export const NavigationContext = createContext<NavigationValue | null>(null);

export function useNavigation() {
  return useContext(NavigationContext);
}

// Un módulo con pestañas informa la pestaña actual para el último tramo de la ruta
// («Administración › Catálogo › Productos»). Fuera del armazón (pruebas aisladas) no hace nada.
export function useBreadcrumbTail(label: string | null) {
  const navigation = useContext(NavigationContext);
  const screen = navigation?.screen;
  const setTail = navigation?.setTail;
  useEffect(() => {
    if (!screen || !setTail) {
      return;
    }
    setTail(screen, label);
    return () => setTail(screen, null);
  }, [screen, setTail, label]);
}
