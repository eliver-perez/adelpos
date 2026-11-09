import { toast } from 'sonner';
import { Icon } from './Icon';
import type { IconName } from './Icon';

export type NoticeTone = 'success' | 'info' | 'warning' | 'error';

const toneIcons: Record<NoticeTone, IconName> = {
  success: 'success',
  info: 'info',
  warning: 'alert',
  error: 'error',
};

const toneLabels: Record<NoticeTone, string> = {
  success: 'Operación completada',
  info: 'Información',
  warning: 'Atención',
  error: 'Error',
};

// Contenido de un aviso. Lo dibuja Sonner dentro de su pila, pero el marcado y el estilo son
// propios: tokens del tema, texto legible en pantalla táctil, botón de cierre de 44 px y la acción
// opcional. Los errores se anuncian como alerta; los demás, como estado.
export function AppToast({
  toastID,
  tone,
  message,
  description,
  action,
  code,
}: {
  toastID: string | number;
  tone: NoticeTone;
  message: string;
  description?: string;
  action?: {
    label: string;
    onClick: () => void;
  };
  code?: string;
}) {
  return (
    <div
      className={`app-toast app-toast-${tone}`}
      role={tone === 'error' ? 'alert' : 'status'}
      aria-label={toneLabels[tone]}
      data-tone={tone}
      data-error-code={code}
    >
      <Icon name={toneIcons[tone]} size={22} className="app-toast-icon" />
      <div className="app-toast-content">
        <p className="app-toast-message">{message}</p>
        {description && <p className="app-toast-description">{description}</p>}
      </div>
      {action && (
        <button
          type="button"
          className="app-toast-action"
          onClick={() => {
            action.onClick();
            toast.dismiss(toastID);
          }}
        >
          {action.label}
        </button>
      )}
      <button
        type="button"
        className="app-toast-close"
        aria-label="Cerrar aviso"
        onClick={() => toast.dismiss(toastID)}
      >
        <Icon name="close" size={20} />
      </button>
    </div>
  );
}
