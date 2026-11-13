import { useId, useLayoutEffect, useRef } from 'react';
import type { KeyboardEvent, ReactNode } from 'react';
import { trapFocus } from '../../components/ConfirmDialog';
import { Icon } from '../../components/Icon';

// Panel modal de la pantalla Documentos (nombre de una plantilla, nota de la versión, historial,
// muestra impresa). Mismo aspecto y comportamiento que ConfirmDialog: foco atrapado, foco inicial
// dentro del panel, Escape cierra y el foco vuelve al control anterior. «wide» lo agranda para el
// historial y la muestra, que incluyen una vista previa.
export function ModalPanel({
  title,
  children,
  actions,
  onClose,
  wide = false,
  closeLabel = 'Cerrar',
}: {
  title: string;
  children: ReactNode;
  actions?: ReactNode;
  onClose: () => void;
  wide?: boolean;
  closeLabel?: string;
}) {
  const titleID = useId();
  const panel = useRef<HTMLDivElement>(null);

  // El foco entra antes de pintar (etapa 9E, como «Acerca de» en la 9D): con un efecto normal,
  // un Escape pulsado justo al aparecer el diálogo llegaba fuera de él y no lo cerraba.
  useLayoutEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    // El foco va al primer campo; si no hay campos, al primer botón (cerrar).
    const field = panel.current?.querySelector<HTMLElement>(
      'input:not([disabled]):not([tabindex="-1"]), textarea:not([disabled]), [role="combobox"]:not([disabled])',
    );
    const button = panel.current?.querySelector<HTMLElement>('button:not([disabled])');
    (field ?? button)?.focus();
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
    <div className="dialog-backdrop">
      <div
        ref={panel}
        className={`dialog-panel documents-modal ${wide ? 'documents-modal-wide' : ''}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        onKeyDown={handleKeyDown}
      >
        <div className="documents-modal-heading">
          <h2 id={titleID}>{title}</h2>
          <button type="button" className="icon-button" aria-label={closeLabel} onClick={onClose}>
            <Icon name="close" />
          </button>
        </div>
        <div className="documents-modal-body">{children}</div>
        {actions && <div className="dialog-actions">{actions}</div>}
      </div>
    </div>
  );
}
