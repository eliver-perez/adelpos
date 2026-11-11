import { useEffect } from 'react';

// Protección al salir de una pantalla con cambios sin guardar (etapa 9B, editor de plantillas).
// Una pantalla registra el aviso mientras tenga cambios; la navegación principal de App.tsx lo
// consulta antes de cambiar de pantalla y pide confirmación con ConfirmDialog (nunca con
// window.confirm, que el WebView de macOS no muestra). Al cerrar o recargar la ventana se usa el
// aviso estándar del navegador.

const guards = new Set<{ message: string }>();

export function leaveWarning(): string | null {
  for (const guard of guards) {
    return guard.message;
  }
  return null;
}

export function useLeaveGuard(active: boolean, message: string) {
  useEffect(() => {
    if (!active) {
      return;
    }
    const guard = { message };
    guards.add(guard);
    const beforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
    };
    window.addEventListener('beforeunload', beforeUnload);
    return () => {
      guards.delete(guard);
      window.removeEventListener('beforeunload', beforeUnload);
    };
  }, [active, message]);
}
