import type { ReactNode } from 'react';

// Conjunto único de iconos de la aplicación: SVG propios de 24×24 con trazo uniforme de 1.6, sin
// relleno y sin librerías externas. Son decorativos (aria-hidden): un botón con icono conserva su
// texto, y un botón solo con icono debe llevar aria-label.
export type IconName =
  | 'server'
  | 'database'
  | 'monitor'
  | 'refresh'
  | 'sun'
  | 'moon'
  | 'check'
  | 'arrow'
  | 'activity'
  | 'back'
  | 'close'
  | 'plus'
  | 'more'
  | 'edit'
  | 'cash'
  | 'receipt'
  | 'split'
  | 'send'
  | 'printer'
  | 'transfer'
  | 'discount'
  | 'income'
  | 'withdrawal'
  | 'expense'
  | 'report'
  | 'count'
  | 'drawer'
  | 'register'
  | 'lock'
  | 'unlock'
  | 'trash'
  | 'refund'
  | 'delivery'
  | 'bag'
  | 'merge'
  | 'move'
  | 'ban'
  | 'clock'
  | 'charge'
  | 'success'
  | 'info'
  | 'alert'
  | 'error';

const paths: Record<IconName, ReactNode> = {
  server: (
    <>
      <rect x="4" y="3" width="16" height="7" rx="2" />
      <rect x="4" y="14" width="16" height="7" rx="2" />
      <path d="M8 6.5h.01M8 17.5h.01M12 6.5h5M12 17.5h5" />
    </>
  ),
  database: (
    <>
      <ellipse cx="12" cy="5" rx="8" ry="3" />
      <path d="M4 5v14c0 4 16 4 16 0V5M4 12c0 4 16 4 16 0" />
    </>
  ),
  monitor: (
    <>
      <rect x="3" y="3" width="18" height="13" rx="2" />
      <path d="M8 21h8M12 16v5" />
    </>
  ),
  refresh: (
    <>
      <path d="M20 7v5h-5M4 17v-5h5M5.6 7a7 7 0 0 1 11.5-2L20 8M4 16l2.9 3A7 7 0 0 0 18.4 17" />
    </>
  ),
  sun: (
    <>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M2 12h2M20 12h2M5 5l1 1M18 18l1 1M5 19l1-1M18 6l1-1" />
    </>
  ),
  moon: <path d="M20.8 13.4A9 9 0 0 1 10.6 3.2 9 9 0 1 0 20.8 13.4Z" />,
  check: <path d="m5 12 4 4L19 6" />,
  arrow: <path d="M4 12h16m-5-5 5 5-5 5" />,
  activity: (
    <>
      <path d="M3 12h4l3-8 4 16 3-8h4" />
    </>
  ),
  back: <path d="M20 12H4m5-5-5 5 5 5" />,
  close: <path d="M6 6l12 12M18 6 6 18" />,
  plus: <path d="M12 5v14M5 12h14" />,
  more: (
    <>
      <circle cx="5.5" cy="12" r="1.4" />
      <circle cx="12" cy="12" r="1.4" />
      <circle cx="18.5" cy="12" r="1.4" />
    </>
  ),
  edit: (
    <>
      <path d="M4 20h4L19 9l-4-4L4 16v4Z" />
      <path d="m13.5 6.5 4 4" />
    </>
  ),
  cash: (
    <>
      <rect x="2.5" y="6" width="19" height="12" rx="2" />
      <circle cx="12" cy="12" r="2.6" />
      <path d="M6 9.5v5M18 9.5v5" />
    </>
  ),
  receipt: (
    <>
      <path d="M6 3h12v18l-2-1.5-2 1.5-2-1.5-2 1.5-2-1.5L6 21V3Z" />
      <path d="M9 8h6M9 12h6M9 16h3" />
    </>
  ),
  split: (
    <>
      <path d="M12 21v-8M12 13 6.5 7.5M12 13l5.5-5.5" />
      <path d="M6 12V7h5M18 12V7h-5" />
    </>
  ),
  send: (
    <>
      <path d="M21 3 10 14" />
      <path d="M21 3 14 21l-4-7-7-4 18-7Z" />
    </>
  ),
  printer: (
    <>
      <path d="M7 9V3h10v6M7 17H5a2 2 0 0 1-2-2v-4a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v4a2 2 0 0 1-2 2h-2" />
      <rect x="7" y="14" width="10" height="7" rx="1" />
    </>
  ),
  transfer: <path d="M4 8h14l-3.5-3.5M20 16H6l3.5 3.5" />,
  discount: (
    <>
      <path d="M19 5 5 19" />
      <circle cx="7" cy="7" r="2.2" />
      <circle cx="17" cy="17" r="2.2" />
    </>
  ),
  income: (
    <>
      <path d="M12 3v11m-4-4 4 4 4-4" />
      <path d="M4 15v4a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-4" />
    </>
  ),
  withdrawal: (
    <>
      <path d="M12 14V3M8 7l4-4 4 4" />
      <path d="M4 15v4a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-4" />
    </>
  ),
  expense: (
    <>
      <path d="M4 7h15a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7Z" />
      <path d="M4 7l2-3h10l1 3M15 13.5h2" />
    </>
  ),
  report: (
    <>
      <rect x="5" y="4" width="14" height="17" rx="2" />
      <path d="M9 4V3h6v1M9 10h6M9 14h6M9 18h3" />
    </>
  ),
  count: (
    <>
      <rect x="5" y="3" width="14" height="18" rx="2" />
      <path d="M8 7h8M8.5 12h.01M12 12h.01M15.5 12h.01M8.5 16h.01M12 16h.01M15.5 16h.01" />
    </>
  ),
  drawer: (
    <>
      <rect x="3" y="10" width="18" height="10" rx="2" />
      <path d="M3 14h18M10 17h4M6 10V5h12v5" />
    </>
  ),
  register: (
    <>
      <path d="M4 21h16M5 21v-8h14v8" />
      <path d="M8 13V8h8v5M10 5h4v3h-4zM8 17h.01M12 17h.01M16 17h.01" />
    </>
  ),
  lock: (
    <>
      <rect x="5" y="11" width="14" height="10" rx="2" />
      <path d="M8 11V7a4 4 0 0 1 8 0v4" />
    </>
  ),
  unlock: (
    <>
      <rect x="5" y="11" width="14" height="10" rx="2" />
      <path d="M8 11V7a4 4 0 0 1 7.6-1.7" />
    </>
  ),
  trash: (
    <>
      <path d="M4 7h16M10 11v6M14 11v6" />
      <path d="M6 7l1 13h10l1-13M9 7V4h6v3" />
    </>
  ),
  refund: (
    <>
      <path d="M9 14 4 9l5-5" />
      <path d="M4 9h10a6 6 0 0 1 0 12h-3" />
    </>
  ),
  delivery: (
    <>
      <path d="M4 8h11v9H4zM15 11h3l2 3v3h-5" />
      <circle cx="7.5" cy="18" r="1.6" />
      <circle cx="17" cy="18" r="1.6" />
    </>
  ),
  bag: (
    <>
      <path d="M5 8h14l-1 13H6L5 8Z" />
      <path d="M9 8V6a3 3 0 0 1 6 0v2" />
    </>
  ),
  merge: (
    <>
      <path d="M7 3v5a4 4 0 0 0 4 4h1v9" />
      <path d="M17 3v5a4 4 0 0 1-4 4M9 18l3 3 3-3" />
    </>
  ),
  move: (
    <>
      <path d="M12 3v18M3 12h18" />
      <path d="m9 6 3-3 3 3M9 18l3 3 3-3M6 9l-3 3 3 3M18 9l3 3-3 3" />
    </>
  ),
  ban: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="m5.6 5.6 12.8 12.8" />
    </>
  ),
  clock: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </>
  ),
  charge: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 8v8M8 12h8" />
    </>
  ),
  success: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="m8 12 3 3 5-6" />
    </>
  ),
  info: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 11v5M12 8h.01" />
    </>
  ),
  alert: (
    <>
      <path d="M12 3.5 2.5 20h19L12 3.5Z" />
      <path d="M12 10v4.5M12 17h.01" />
    </>
  ),
  error: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="m9 9 6 6M15 9l-6 6" />
    </>
  ),
};

export function Icon({
  name,
  size = 22,
  className = '',
}: {
  name: IconName;
  size?: number;
  className?: string;
}) {
  return (
    <svg
      className={`app-icon ${className}`.trim()}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {paths[name]}
    </svg>
  );
}

// Icono a la izquierda del texto de un botón. El icono mide 18 px y es decorativo: el nombre
// accesible del botón sigue siendo su texto. El contenedor no cambia la distribución del botón.
export function IconLabel({ icon, children }: { icon: IconName; children: ReactNode }) {
  return (
    <span className="icon-label">
      <Icon name={icon} size={18} />
      <span className="icon-label-text">{children}</span>
    </span>
  );
}
