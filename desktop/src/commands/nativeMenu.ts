import { Channel, invoke, isTauri } from '@tauri-apps/api/core';

// Puente del menú nativo (etapa 9C, fase C) con sus dos operaciones cerradas de Rust
// (src-tauri/src/menu.rs):
// - menu_subscribe(channel): Rust envía { action } por el canal cada vez que se elige una opción
//   propia del menú. Una sola suscripción por carga de la vista (la siguiente sustituye a la
//   anterior en Rust), compartida por quien escuche.
// - menu_set_state(enabled): React informa qué opciones están habilitadas; Rust rechaza cualquier
//   identificador fuera de la lista cerrada.

interface MenuMessage {
  action?: unknown;
}

let listener: ((action: string) => void) | null = null;
let subscribed = false;

export function listenNativeMenu(onAction: (action: string) => void): () => void {
  listener = onAction;
  if (!subscribed && isTauri()) {
    subscribed = true;
    try {
      const channel = new Channel<MenuMessage>();
      channel.onmessage = (message) => {
        if (typeof message?.action === 'string') {
          listener?.(message.action);
        }
      };
      Promise.resolve()
        .then(() => invoke('menu_subscribe', { channel }))
        .catch(() => {
          subscribed = false;
        });
    } catch {
      // Sin canal disponible (pruebas sin puente nativo): no hay menú que escuchar.
      subscribed = false;
    }
  }
  return () => {
    if (listener === onAction) {
      listener = null;
    }
  };
}

// El centro de comandos lo llama solo cuando cambia el conjunto habilitado.
export function reportNativeMenuState(enabled: string[]) {
  if (!isTauri()) {
    return;
  }
  Promise.resolve()
    .then(() => invoke('menu_set_state', { enabled }))
    .catch(() => {
      // Sin puente nativo (pruebas) o con un identificador rechazado: el menú conserva su estado.
    });
}
