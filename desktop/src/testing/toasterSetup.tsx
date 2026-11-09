import { flushSync } from 'react-dom';
import { createRoot } from 'react-dom/client';
import type { Root } from 'react-dom/client';
import { toast } from 'sonner';
import { afterEach, beforeEach } from 'vitest';
import { AppToaster } from '../components/AppToaster';

// Preparación común de Vitest: en las pruebas con DOM se monta la pila de avisos igual que en
// App.tsx, de modo que una pantalla probada por separado muestra sus mensajes de éxito y error
// como en la aplicación. Cada prueba empieza sin avisos: al terminar se descartan todos (Sonner
// conserva un estado global) y se desmonta la pila.

let root: Root | null = null;
let host: HTMLElement | null = null;

beforeEach(() => {
  if (typeof document === 'undefined') {
    return;
  }
  host = document.createElement('div');
  host.dataset.testToaster = 'true';
  document.body.appendChild(host);
  const created = createRoot(host);
  root = created;
  flushSync(() => {
    created.render(<AppToaster />);
  });
});

afterEach(() => {
  toast.dismiss();
  if (root) {
    root.unmount();
    root = null;
  }
  host?.remove();
  host = null;
});
