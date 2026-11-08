import { useEffect, useRef, useState } from 'react';
import type { Device } from '../services/models';

type EditableField = HTMLInputElement | HTMLTextAreaElement;

// Un campo (o un contenedor) marcado con data-virtual-keyboard="off" queda excluido de forma
// explícita; por ejemplo, los importes de cobro que se capturan con el teclado numérico fijo.
export function excludedFromVirtualKeyboard(target: Element) {
  return target.closest('[data-virtual-keyboard="off"]') !== null;
}

function eligibleField(target: EventTarget | null): target is EditableField {
  if (target instanceof Element && excludedFromVirtualKeyboard(target)) {
    return false;
  }
  if (target instanceof HTMLTextAreaElement) return !target.disabled && !target.readOnly;
  return (
    target instanceof HTMLInputElement &&
    !target.disabled &&
    !target.readOnly &&
    ['text', 'search', 'tel', 'email', 'number', 'url'].includes(target.type)
  );
}

function fieldLabel(field: EditableField) {
  return (
    field.getAttribute('aria-label') ||
    field.closest('label')?.querySelector('span')?.textContent ||
    field.placeholder ||
    'Campo de texto'
  );
}

export function replaceFieldSelection(field: EditableField, text: string, backwards = false) {
  const start = field.selectionStart ?? field.value.length;
  const end = field.selectionEnd ?? field.value.length;
  const first = backwards && start === end ? Math.max(0, start - 1) : start;
  const next = field.value.slice(0, first) + text + field.value.slice(end);
  if (field.maxLength > 0 && next.length > field.maxLength) return;
  const prototype =
    field instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(prototype, 'value')?.set;
  setter?.call(field, next);
  field.dispatchEvent(new Event('input', { bubbles: true }));
  const caret = first + text.length;
  if (field instanceof HTMLTextAreaElement || ['text', 'search', 'tel', 'url'].includes(field.type))
    field.setSelectionRange(caret, caret);
}

export function VirtualKeyboard({ mode }: { mode: Device['keyboard_mode'] }) {
  const [field, setField] = useState<EditableField | null>(null);
  const [visible, setVisible] = useState(false);
  const [uppercase, setUppercase] = useState(false);
  const keyboard = useRef<HTMLDivElement>(null);
  const numeric =
    field instanceof HTMLInputElement &&
    (field.type === 'number' ||
      field.type === 'tel' ||
      ['numeric', 'decimal'].includes(field.inputMode));
  const decimal =
    field instanceof HTMLInputElement &&
    (field.inputMode === 'decimal' || (field.type === 'number' && field.step !== '1'));

  useEffect(() => {
    if (mode === 'disabled') {
      setVisible(false);
      setField(null);
      return;
    }
    function focused(event: FocusEvent) {
      if (!eligibleField(event.target)) {
        if (!keyboard.current?.contains(event.target as Node)) {
          setVisible(false);
          setField(null);
        }
        return;
      }
      setField(event.target);
      const touch =
        (window.matchMedia?.('(pointer: coarse)').matches ?? false) || navigator.maxTouchPoints > 0;
      setVisible(mode === 'always' || touch);
    }
    document.addEventListener('focusin', focused);
    return () => document.removeEventListener('focusin', focused);
  }, [mode]);

  useEffect(() => {
    const height = visible && field?.isConnected ? '252px' : '0px';
    document.documentElement.style.setProperty('--virtual-keyboard-height', height);
    if (visible)
      requestAnimationFrame(() => field?.scrollIntoView?.({ block: 'center', behavior: 'smooth' }));
    return () => document.documentElement.style.setProperty('--virtual-keyboard-height', '0px');
  }, [visible, field]);

  useEffect(() => {
    if (!field) return;
    const observer = new MutationObserver(() => {
      if (!field.isConnected) {
        setField(null);
        setVisible(false);
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
    return () => observer.disconnect();
  }, [field]);

  if (mode === 'disabled' || !field?.isConnected) return null;
  if (!visible)
    return (
      <button
        type="button"
        className="keyboard-reopen secondary-button"
        onPointerDown={(event) => event.preventDefault()}
        onClick={() => setVisible(true)}
      >
        Teclado
      </button>
    );
  const rows = numeric
    ? ['123', '456', '789', decimal ? '0.' : '0+']
    : ['1234567890', 'qwertyuiop', 'asdfghjklñ', 'zxcvbnm', 'áéíóú@.-'];

  return (
    <div
      className={`virtual-keyboard ${numeric ? 'virtual-keyboard-numeric' : ''}`}
      ref={keyboard}
      role="group"
      aria-label={numeric ? 'Teclado virtual numérico' : 'Teclado virtual alfanumérico'}
      onPointerDown={(event) => event.preventDefault()}
    >
      <div className="keyboard-heading">
        <strong>{fieldLabel(field)}</strong>
        <button type="button" className="secondary-button" onClick={() => setVisible(false)}>
          Cerrar teclado
        </button>
      </div>
      <div className="keyboard-keys">
        {rows.map((row) => (
          <div className="keyboard-row" key={row}>
            {Array.from(row).map((letter) => {
              const text = uppercase ? letter.toUpperCase() : letter;
              return (
                <button
                  type="button"
                  key={letter}
                  onClick={() => replaceFieldSelection(field, text)}
                  aria-label={`Escribir ${text}`}
                >
                  {text}
                </button>
              );
            })}
          </div>
        ))}
        <div className="keyboard-row keyboard-controls">
          {!numeric && (
            <button type="button" aria-pressed={uppercase} onClick={() => setUppercase(!uppercase)}>
              Mayús
            </button>
          )}
          {!numeric && (
            <button
              type="button"
              className="keyboard-space"
              onClick={() => replaceFieldSelection(field, ' ')}
            >
              Espacio
            </button>
          )}
          <button type="button" onClick={() => replaceFieldSelection(field, '', true)}>
            Borrar
          </button>
          <button type="button" onClick={() => setVisible(false)}>
            Listo
          </button>
        </div>
      </div>
    </div>
  );
}
