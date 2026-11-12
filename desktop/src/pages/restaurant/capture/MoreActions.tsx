import { useEffect, useId, useRef, useState } from 'react';
import { IconLabel } from '../../../components/Icon';
import type { IconName } from '../../../components/Icon';

export interface MoreAction<T extends string> {
  id: T;
  label: string;
  // Icono opcional a la izquierda del texto del botón.
  icon?: IconName;
  // Etapa 9D (§1.5): una acción que no se puede deshacer va con el estilo destructivo rojo.
  tone?: 'danger';
  // Etapa 9D (§5.1): aclaración breve debajo del texto, por ejemplo qué hacer primero para poder
  // cancelar la cuenta. No forma parte del nombre del botón: se anuncia como su descripción.
  hint?: string;
}

// Menú «Más…» de la cuenta: reúne las acciones menos frecuentes como botones grandes (sustituye
// a la lista desplegable «Acciones de cuenta…»). Se cierra al elegir, con Escape o al tocar fuera.
export function MoreActions<T extends string>({
  actions,
  disabled,
  onAction,
}: {
  actions: MoreAction<T>[];
  disabled: boolean;
  onAction: (action: T) => void;
}) {
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const menuID = useId();

  useEffect(() => {
    if (!open) return;
    function outside(event: PointerEvent) {
      if (container.current && !container.current.contains(event.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener('pointerdown', outside);
    return () => document.removeEventListener('pointerdown', outside);
  }, [open]);

  if (actions.length === 0) {
    return null;
  }
  return (
    <div
      className="capture-more"
      ref={container}
      onKeyDown={(event) => {
        if (event.key === 'Escape') {
          setOpen(false);
        }
      }}
    >
      <button
        type="button"
        className="secondary-button"
        aria-expanded={open}
        disabled={disabled}
        onClick={() => setOpen(!open)}
      >
        <IconLabel icon="more">Más…</IconLabel>
      </button>
      {open && (
        <div className="capture-more-menu" role="group" aria-label="Más acciones de la cuenta">
          {actions.map((action) => (
            <button
              type="button"
              key={action.id}
              className={`${action.tone === 'danger' ? 'danger-button' : 'secondary-button'} ${action.hint ? 'capture-more-hinted' : ''}`}
              aria-label={action.hint ? action.label : undefined}
              aria-describedby={action.hint ? `${menuID}-${action.id}` : undefined}
              onClick={() => {
                setOpen(false);
                onAction(action.id);
              }}
            >
              {action.icon ? (
                <IconLabel icon={action.icon}>{action.label}</IconLabel>
              ) : (
                action.label
              )}
              {action.hint && (
                <small id={`${menuID}-${action.id}`} className="capture-more-hint">
                  {action.hint}
                </small>
              )}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
