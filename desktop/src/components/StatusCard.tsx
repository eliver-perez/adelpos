import { Icon } from './Icon';
import type { IconName } from './Icon';

export function StatusCard({
  icon,
  label,
  value,
  description,
  state,
}: {
  icon: IconName;
  label: string;
  value: string;
  description: string;
  state: 'good' | 'warning' | 'neutral';
}) {
  return (
    <section className="surface flex min-h-48 flex-col rounded-2xl p-6">
      <div className="mb-7 flex items-center justify-between">
        <span className="muted">
          <Icon name={icon} />
        </span>
        <span className={`status-dot ${state}`} aria-hidden="true" />
      </div>
      <p className="muted mb-1 text-sm">{label}</p>
      <h3 className="text-xl font-semibold tracking-tight">{value}</h3>
      <p className="muted mt-2 text-xs leading-relaxed">{description}</p>
    </section>
  );
}
