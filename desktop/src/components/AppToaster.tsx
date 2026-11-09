import { useEffect, useRef } from 'react';
import type { CSSProperties } from 'react';
import { Toaster } from 'sonner';

// Eventos del puntero que Sonner usa para desplegar la pila y pausar los avisos al pasar el ratón.
const hoverEvents = [
  'mouseover',
  'mouseout',
  'mousemove',
  'pointerover',
  'pointerout',
  'pointerdown',
  'pointerup',
];

// Pila única de avisos de la aplicación (Sonner). Se monta una sola vez en App.tsx.
//
// - Arriba al centro, debajo de la cabecera: las acciones principales de cobro y captura están
//   abajo y a la derecha, y no quedan tapadas. La altura de la cabecera la fija styles.css con
//   --toast-offset-top (más baja en modo operación).
// - Sin estilos propios de Sonner: cada aviso es un AppToast con los tokens del tema.
// - Apilada: solo el aviso más reciente se ve completo y los anteriores asoman detrás; así la pila
//   ocupa la altura de un aviso y no cubre las barras de herramientas de cobro y captura.
// - Sin pausa por «hover»: en una pantalla táctil el puntero se queda donde fue el último toque, y
//   si un aviso aparece justo ahí Sonner desplegaría la pila y detendría su cierre automático. El
//   contenedor detiene esos eventos antes de que lleguen a Sonner; los botones de cerrar y de acción
//   siguen funcionando con el clic.
export function AppToaster() {
  const host = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const element = host.current;
    if (!element) {
      return;
    }
    const stop = (event: Event) => {
      event.stopPropagation();
    };
    // React calcula la entrada del puntero a partir del «mouseout» del elemento que se abandona,
    // que está fuera de la pila: ese evento se detiene en la fase de captura del documento cuando
    // el destino es un aviso. La aplicación no usa onMouseEnter ni onMouseLeave.
    const stopEntering = (event: Event) => {
      const related = (event as MouseEvent).relatedTarget;
      if (related instanceof Node && element.contains(related)) {
        event.stopPropagation();
      }
    };
    for (const name of hoverEvents) {
      element.addEventListener(name, stop);
    }
    document.addEventListener('mouseout', stopEntering, true);
    document.addEventListener('pointerout', stopEntering, true);
    return () => {
      for (const name of hoverEvents) {
        element.removeEventListener(name, stop);
      }
      document.removeEventListener('mouseout', stopEntering, true);
      document.removeEventListener('pointerout', stopEntering, true);
    };
  }, []);

  return (
    <div ref={host} className="app-toaster-host">
      <Toaster
        position="top-center"
        visibleToasts={3}
        gap={10}
        theme="light"
        offset={{
          top: 'var(--toast-offset-top, 92px)',
        }}
        mobileOffset={{
          top: 'var(--toast-offset-top, 92px)',
        }}
        containerAriaLabel="Avisos"
        style={
          {
            '--width': 'min(480px, calc(100vw - 32px))',
          } as CSSProperties
        }
        toastOptions={{
          unstyled: true,
          className: 'app-toast-frame',
        }}
      />
    </div>
  );
}
