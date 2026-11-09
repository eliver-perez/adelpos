import type { ReactNode } from 'react';
import { Icon } from './Icon';

// Botón táctil que funciona como interruptor: el estado activo se resalta con fondo y borde y se
// anuncia con aria-pressed. No usa casillas ni marcas de verificación.
export function ToggleButton({
  label,
  pressed,
  onChange,
  disabled = false,
  icon,
  describedBy,
}: {
  label: string;
  pressed: boolean;
  onChange: (pressed: boolean) => void;
  disabled?: boolean;
  icon?: ReactNode;
  describedBy?: string;
}) {
  return (
    <button
      type="button"
      className={`toggle-button ${pressed ? 'toggle-button-on' : ''}`}
      aria-pressed={pressed}
      aria-describedby={describedBy}
      disabled={disabled}
      onClick={() => onChange(!pressed)}
    >
      {icon}
      <span>{label}</span>
    </button>
  );
}

// Iconos de los interruptores de impresión y entrega: los mismos SVG del conjunto único de Icon.
export function PrinterIcon() {
  return <Icon name="printer" size={18} />;
}

export function DeliveryIcon() {
  return <Icon name="delivery" size={18} />;
}
