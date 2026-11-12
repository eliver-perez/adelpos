import { useEffect, useId, useRef } from 'react';
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

  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    closeButton.current?.focus();
    return () => {
      if (previous && previous.isConnected) {
        previous.focus();
      }
    };
  }, []);

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
