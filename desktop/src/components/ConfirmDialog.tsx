import { useCallback, useId, useLayoutEffect, useRef, useState } from 'react';
import type { KeyboardEvent, ReactNode } from 'react';
import { Icon } from './Icon';

// Diálogo de confirmación estándar (etapa 7.1, a partir del diálogo de la etapa 6.1 para «Volver al
// listado»). El WebView nativo de Tauri en macOS no muestra window.confirm, window.alert ni
// window.prompt: devuelve un valor vacío sin mostrar nada, y la pantalla parece no responder. Toda
// confirmación debe usar este componente.
//
// - tone 'danger' diferencia la acción destructiva o irreversible (cancelar productos, devolver,
//   descartar cambios) con color propio e icono de advertencia;
// - el foco queda atrapado dentro del diálogo mientras está abierto y vuelve al control anterior
//   al cerrarse;
// - Escape equivale a cancelar; el foco inicial va a la opción que no descarta nada;
// - botones de 56 px o más para pantalla táctil.

export type ConfirmTone = 'default' | 'danger';

export interface ConfirmRequest {
  title: string;
  message: ReactNode;
  confirmLabel: string;
  cancelLabel?: string;
  tone?: ConfirmTone;
}

interface PendingConfirmation extends ConfirmRequest {
  resolve: (confirmed: boolean) => void;
}

const focusableSelector = [
  'button:not([disabled])',
  'a[href]',
  // El campo oculto de validación del selector (tabindex -1) no cuenta como destino del foco.
  'input:not([disabled]):not([type="hidden"]):not([tabindex="-1"])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',');

// Mantiene el foco dentro del panel: Tab en el último control vuelve al primero y Mayús+Tab en el
// primero va al último. También la usa la hoja del atajo «Caja» (etapa 7.1).
export function trapFocus(event: KeyboardEvent<HTMLElement>, panel: HTMLElement) {
  const focusable = Array.from(panel.querySelectorAll<HTMLElement>(focusableSelector));
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (!first || !last) {
    event.preventDefault();
    return;
  }
  const active = document.activeElement;
  const outside = !panel.contains(active);
  if (event.shiftKey && (active === first || outside)) {
    event.preventDefault();
    last.focus();
    return;
  }
  if (!event.shiftKey && (active === last || outside)) {
    event.preventDefault();
    first.focus();
  }
}

export function ConfirmDialog({
  title,
  message,
  confirmLabel,
  cancelLabel = 'Cancelar',
  tone = 'default',
  onConfirm,
  onCancel,
}: ConfirmRequest & {
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const titleID = useId();
  const messageID = useId();
  const panel = useRef<HTMLDivElement>(null);
  const cancelButton = useRef<HTMLButtonElement>(null);

  // El foco entra antes de pintar (etapa 9E, como «Acerca de» en la 9D): con un efecto normal,
  // un Escape pulsado justo al aparecer el diálogo llegaba fuera de él y no lo cerraba.
  useLayoutEffect(() => {
    // El foco inicial va a la opción que no descarta nada; al cerrar vuelve a donde estaba.
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    cancelButton.current?.focus();
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
      onCancel();
      return;
    }
    if (event.key === 'Tab' && panel.current) {
      trapFocus(event, panel.current);
    }
  }

  const danger = tone === 'danger';
  return (
    <div className="dialog-backdrop">
      <div
        ref={panel}
        className={`dialog-panel ${danger ? 'dialog-panel-danger' : ''}`}
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={titleID}
        aria-describedby={messageID}
        onKeyDown={handleKeyDown}
      >
        <div className="dialog-heading">
          {danger && <Icon name="alert" size={26} className="dialog-heading-icon" />}
          <h2 id={titleID}>{title}</h2>
        </div>
        <div id={messageID} className="dialog-message">
          {message}
        </div>
        <div className="dialog-actions">
          <button
            ref={cancelButton}
            type="button"
            className="secondary-button dialog-button"
            onClick={onCancel}
          >
            {cancelLabel}
          </button>
          <button
            type="button"
            className={`${danger ? 'danger-solid-button' : 'primary-button'} dialog-button`}
            data-tone={tone}
            onClick={onConfirm}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}

export interface DialogChoice {
  label: string;
  onSelect: () => void;
  // Etapa 9D (§1.5): una opción que descarta algo sin poder deshacerlo va en rojo.
  tone?: ConfirmTone;
}

// Clase del botón de una opción: la primera es la principal; una opción destructiva, roja.
function choiceClass(choice: DialogChoice, index: number) {
  if (choice.tone === 'danger') {
    return 'danger-button';
  }
  return index === 0 ? 'primary-button' : 'secondary-button';
}

// Diálogo con varias opciones equivalentes y sin «Cancelar» (etapa 7.1, «Esta mesa ya tiene una
// cuenta»): ninguna descarta nada y el operador debe elegir una. Mismo aspecto, foco atrapado y
// botones de 56 px que ConfirmDialog; Escape no elige por el operador. La primera opción recibe
// el foco y se destaca como principal.
export function ChoiceDialog({
  title,
  message,
  choices,
  busy = false,
}: {
  title: string;
  message: ReactNode;
  choices: DialogChoice[];
  busy?: boolean;
}) {
  const titleID = useId();
  const messageID = useId();
  const panel = useRef<HTMLDivElement>(null);
  const firstChoice = useRef<HTMLButtonElement>(null);

  // El foco entra antes de pintar (etapa 9E, como «Acerca de» en la 9D): con un efecto normal,
  // un Escape pulsado justo al aparecer el diálogo llegaba fuera de él y no lo cerraba.
  useLayoutEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    firstChoice.current?.focus();
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
        className="dialog-panel"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={titleID}
        aria-describedby={messageID}
        onKeyDown={handleKeyDown}
      >
        <div className="dialog-heading">
          <Icon name="info" size={26} className="dialog-heading-icon" />
          <h2 id={titleID}>{title}</h2>
        </div>
        <div id={messageID} className="dialog-message">
          {message}
        </div>
        <div className="dialog-actions dialog-choices" aria-busy={busy || undefined}>
          {choices.map((choice, index) => (
            <button
              key={choice.label}
              ref={index === 0 ? firstChoice : undefined}
              type="button"
              className={`${choiceClass(choice, index)} dialog-button`}
              data-tone={choice.tone ?? 'default'}
              disabled={busy}
              onClick={choice.onSelect}
            >
              {choice.label}
            </button>
          ))}
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
      tone={pending.tone}
      onConfirm={() => finish(true)}
      onCancel={() => finish(false)}
    />
  ) : null;

  return { confirm, dialog };
}
