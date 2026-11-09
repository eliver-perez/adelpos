import { useCallback, useEffect, useId, useRef, useState } from 'react';
import type { ReactNode } from 'react';

// Confirmaciones dentro de la aplicación. El WebView nativo de Tauri en macOS no muestra
// window.confirm, window.alert ni window.prompt: devuelve un valor vacío sin mostrar nada, y la
// pantalla parece no responder. Toda confirmación debe usar este componente.

export interface ConfirmRequest {
  title: string;
  message: string;
  confirmLabel: string;
  cancelLabel?: string;
}

interface PendingConfirmation extends ConfirmRequest {
  resolve: (confirmed: boolean) => void;
}

export function ConfirmDialog({
  title,
  message,
  confirmLabel,
  cancelLabel = 'Cancelar',
  onConfirm,
  onCancel,
}: ConfirmRequest & {
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const titleID = useId();
  const messageID = useId();
  const cancelButton = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    // El foco inicial va a la opción que no descarta nada.
    cancelButton.current?.focus();
  }, []);

  return (
    <div className="dialog-backdrop">
      <div
        className="dialog-panel"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={titleID}
        aria-describedby={messageID}
        onKeyDown={(event) => {
          if (event.key === 'Escape') {
            event.preventDefault();
            onCancel();
          }
        }}
      >
        <h2 id={titleID}>{title}</h2>
        <p id={messageID}>{message}</p>
        <div className="dialog-actions">
          <button ref={cancelButton} type="button" className="secondary-button" onClick={onCancel}>
            {cancelLabel}
          </button>
          <button type="button" className="primary-button" onClick={onConfirm}>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}

// Devuelve una función confirm() basada en promesas y el diálogo que la página debe renderizar.
export function useConfirmDialog(): {
  confirm: (request: ConfirmRequest) => Promise<boolean>;
  dialog: ReactNode;
} {
  const [pending, setPending] = useState<PendingConfirmation | null>(null);

  const confirm = useCallback((request: ConfirmRequest) => {
    return new Promise<boolean>((resolve) => {
      setPending({ ...request, resolve });
    });
  }, []);

  function finish(confirmed: boolean) {
    pending?.resolve(confirmed);
    setPending(null);
  }

  const dialog = pending ? (
    <ConfirmDialog
      title={pending.title}
      message={pending.message}
      confirmLabel={pending.confirmLabel}
      cancelLabel={pending.cancelLabel}
      onConfirm={() => finish(true)}
      onCancel={() => finish(false)}
    />
  ) : null;

  return { confirm, dialog };
}
