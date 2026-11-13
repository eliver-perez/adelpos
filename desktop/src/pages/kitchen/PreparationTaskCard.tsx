import { preparationLabels } from '../../services/kitchen';
import type { Task } from '../../services/kitchen';
import { elapsedTime } from '../../services/restaurant';
import { accountContext, taskUrgency, urgencyLabels, visibleParts } from './preparationModel';

// Tarjeta de una partida en una estación en el modo cocina de Preparación (etapa 9F, §5.4): cantidad y
// producto en grande, la parte que prepara esta estación a la vista (no plegada), el tiempo contra el
// objetivo con su color y su texto («Por vencer», «Atrasada») y botones grandes para tocar con las manos
// ocupadas.
export function PreparationTaskCard({
  tasks,
  canManage,
  disabled,
  onAdvance,
}: {
  tasks: Task[];
  canManage: boolean;
  disabled: boolean;
  onAdvance: (next: 'preparing' | 'ready') => void;
}) {
  const task = tasks[0]!;
  const finished = task.state === 'ready' || task.state === 'cancelled';
  const urgency = finished
    ? 'normal'
    : taskUrgency(task.elapsed_seconds, task.warning_minutes, task.target_minutes);
  const parts = visibleParts(task);
  return (
    <article
      className={`kitchen-preparation-card kitchen-task-card kitchen-card-${urgency}`}
      data-urgency={urgency}
    >
      <div className="section-heading">
        <strong className="kitchen-task-station">{task.station_name}</strong>
        <span className="kitchen-state-label">{preparationLabels[task.state]}</span>
      </div>
      <p className="kitchen-task-context">
        #{task.folio} · {accountContext(task.account_name, task.table_name)}
      </p>
      <h3>
        {tasks.length} × {task.instructions.name}
      </h3>
      {parts.length > 0 && (
        <ul className="kitchen-task-parts" aria-label={`Parte de ${task.station_name}`}>
          {parts.map((part) => (
            <li key={part.path}>
              <strong>
                {part.quantity * tasks.length} × {part.name}
                {part.variant ? ` · ${part.variant}` : ''}
              </strong>
              {part.notes && <span>{part.notes}</span>}
            </li>
          ))}
        </ul>
      )}
      {task.instructions.notes && (
        <p className="kitchen-preparation-note">{task.instructions.notes}</p>
      )}
      <p className="kitchen-task-time">
        <span className="kitchen-task-elapsed">{elapsedTime(task.elapsed_seconds)}</span>
        <span className="muted">objetivo {task.target_minutes} min</span>
        {urgency !== 'normal' && (
          <span className={`kitchen-urgency kitchen-urgency-${urgency}`}>
            {urgencyLabels[urgency]}
          </span>
        )}
      </p>
      {!finished && canManage && (
        <div className="kitchen-task-actions">
          {task.state === 'released' && (
            <button
              className="secondary-button kitchen-action-button"
              disabled={disabled}
              onClick={() => onAdvance('preparing')}
            >
              {task.tracking === 'order' ? 'Pedido en preparación' : 'En preparación'}
            </button>
          )}
          <button
            className="primary-button kitchen-action-button"
            disabled={disabled}
            onClick={() => onAdvance('ready')}
          >
            {task.tracking === 'order' ? 'Pedido listo en estación' : 'Marcar producto listo'}
          </button>
        </div>
      )}
    </article>
  );
}
