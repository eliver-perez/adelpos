import type { ReactNode } from 'react';

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

export function PrinterIcon() {
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M7 9V3h10v6M7 17H5a2 2 0 0 1-2-2v-4a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v4a2 2 0 0 1-2 2h-2" />
      <rect x="7" y="14" width="10" height="7" rx="1" />
    </svg>
  );
}

export function DeliveryIcon() {
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M4 8h11v9H4zM15 11h3l2 3v3h-5" />
      <circle cx="7.5" cy="18" r="1.6" />
      <circle cx="17" cy="18" r="1.6" />
    </svg>
  );
}
