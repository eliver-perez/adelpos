import { useEffect, useId, useLayoutEffect, useRef } from 'react';
import type { KeyboardEvent, ReactNode } from 'react';
import { trapFocus } from '../components/ConfirmDialog';
import { IconLabel } from '../components/Icon';

// Marco de los diálogos de la fase C («Atajos de teclado» y «Acerca de AD-EL POS»): modal con el
// foco atrapado, Escape para cerrar y el foco devuelto al control anterior, como ConfirmDialog.
export function CommandDialog({
  title,
  className = '',
  onClose,
  children,
}: {
  title: string;
  className?: string;
  onClose: () => void;
  children: ReactNode;
}) {
  const titleID = useId();
  const panel = useRef<HTMLDivElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);

  // Etapa 9D: el foco entra al diálogo antes de dibujarlo (no después, como con useEffect), para
  // que un Escape pulsado en cuanto aparece llegue al diálogo. En WebKit, con el efecto posterior,
  // un Escape inmediato caía fuera del diálogo y no lo cerraba (falla intermitente de
  // workflow-stage9c-keyboard).
  useLayoutEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    closeButton.current?.focus();
    return () => {
      if (previous && previous.isConnected) {
        previous.focus();
      }
    };
  }, []);

  // Si el foco quedó fuera del diálogo (otra parte de la pantalla lo tomó), Escape lo cierra igual.
  // La capa de teclado no atiende Escape con un diálogo abierto; dentro del diálogo lo atiende
  // handleKeyDown.
  useEffect(() => {
    function escape(event: globalThis.KeyboardEvent) {
      const node = panel.current;
      if (event.key !== 'Escape' || event.defaultPrevented || !node) {
        return;
      }
      if (node.contains(document.activeElement)) {
        return;
      }
      event.preventDefault();
      onClose();
    }
    document.addEventListener('keydown', escape);
    return () => document.removeEventListener('keydown', escape);
  }, [onClose]);

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      onClose();
      return;
    }
    if (event.key === 'Tab' && panel.current) {
      trapFocus(event, panel.current);
    }
  }

  return (
    <div className="dialog-backdrop command-dialog-backdrop">
      <div
        ref={panel}
        className={`dialog-panel command-dialog ${className}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        onKeyDown={handleKeyDown}
      >
        <div className="command-dialog-heading">
          <h2 id={titleID}>{title}</h2>
          <button ref={closeButton} type="button" className="secondary-button" onClick={onClose}>
            <IconLabel icon="close">Cerrar</IconLabel>
          </button>
        </div>
        <div className="command-dialog-body">{children}</div>
      </div>
    </div>
  );
}
