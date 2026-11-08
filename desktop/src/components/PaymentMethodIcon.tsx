import type { ReactNode } from 'react';
import type { MethodIcon } from '../services/payments';

const paths: Record<MethodIcon, ReactNode> = {
  cash: (
    <>
      <rect x="2.5" y="6" width="19" height="12" rx="2" />
      <circle cx="12" cy="12" r="2.6" />
      <path d="M6 9.5v5M18 9.5v5" />
    </>
  ),
  card: (
    <>
      <rect x="2.5" y="5" width="19" height="14" rx="2" />
      <path d="M2.5 9.5h19M6 15h4" />
    </>
  ),
  transfer: (
    <>
      <path d="M4 8h14l-3.5-3.5M20 16H6l3.5 3.5" />
    </>
  ),
  platform: (
    <>
      <rect x="6.5" y="2.5" width="11" height="19" rx="2.2" />
      <path d="M10.5 18.5h3" />
    </>
  ),
  voucher: (
    <>
      <path d="M3 7.5A1.5 1.5 0 0 1 4.5 6h15A1.5 1.5 0 0 1 21 7.5V10a2 2 0 0 0 0 4v2.5a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 16.5V14a2 2 0 0 0 0-4Z" />
      <path d="M14.5 6v12" strokeDasharray="1.6 1.8" />
    </>
  ),
  other: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M8 12h.01M12 12h.01M16 12h.01" />
    </>
  ),
};

// Icono del catálogo cerrado de iconos de métodos de pago definido en Go.
export function PaymentMethodIcon({
  icon,
  className = '',
}: {
  icon: MethodIcon;
  className?: string;
}) {
  return (
    <svg
      className={className}
      width="22"
      height="22"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[icon] ?? paths.other}
    </svg>
  );
}
