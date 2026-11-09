import { createElement } from 'react';
import { toast } from 'sonner';
import { AppToast } from '../components/AppToast';
import type { NoticeTone } from '../components/AppToast';
import { APIError } from './api';

// Sistema único de mensajes de la aplicación (etapa 7.1). Toda confirmación o error de una acción
// del usuario pasa por aquí y se muestra en el AppToaster montado una sola vez en App.tsx.
//
// Criterio: el resultado de una acción (guardar, enviar, cobrar, imprimir…) es un mensaje; lo que
// describe el estado vigente de una pantalla (validación de un campo, una operación incierta por
// resolver, un conflicto del borrador, un error al cargar un panel o el cambio por entregar) sigue
// en su lugar dentro de la pantalla.
//
// Reglas:
// - el éxito se cierra solo a los pocos segundos;
// - el error no se cierra solo: exige que la persona lo lea y lo cierre;
// - el texto es el que entrega Go en español; el código interno nunca se muestra (queda como dato
//   oculto del aviso para depuración);
// - ningún mensaje de éxito debe incluir datos protegidos, como el efectivo esperado.

export interface NotifyAction {
  label: string;
  onClick: () => void;
}

export interface NotifyOptions {
  // Texto secundario debajo del mensaje.
  description?: string;
  // Acción opcional, por ejemplo «Imprimir comprobante».
  action?: NotifyAction;
  // Identificador estable: un aviso nuevo con el mismo id sustituye al anterior.
  id?: string;
  // Fuerza que el aviso permanezca hasta que se cierre.
  persistent?: boolean;
}

export const noticeDurations: Record<NoticeTone, number> = {
  success: 4000,
  info: 5000,
  warning: 8000,
  error: Infinity,
};

const genericError = 'No se pudo completar la operación.';

// Códigos con forma de constante («CASH_INSUFFICIENT»): si un mensaje llega así, no se muestra.
const internalCodePattern = /^[A-Z][A-Z0-9_]+$/;

function show(tone: NoticeTone, message: string, options: NotifyOptions = {}, code?: string) {
  const id = options.id ?? `${tone}:${message}`;
  const duration = options.persistent ? Infinity : noticeDurations[tone];
  return toast.custom(
    (toastID) =>
      createElement(AppToast, {
        toastID,
        tone,
        message,
        description: options.description,
        action: options.action,
        code,
      }),
    {
      id,
      duration,
    },
  );
}

export function notifySuccess(message: string, options?: NotifyOptions) {
  return show('success', message, options);
}

export function notifyInfo(message: string, options?: NotifyOptions) {
  return show('info', message, options);
}

export function notifyWarning(message: string, options?: NotifyOptions) {
  return show('warning', message, options);
}

// Texto en español para una persona a partir de un error de la API, del puente nativo o de una
// cadena. Nunca devuelve el código interno: si el mensaje falta o tiene forma de código, usa el
// texto de respaldo.
export function errorMessage(error: unknown, fallback: string = genericError) {
  let message = '';
  if (typeof error === 'string') {
    message = error;
  } else if (error instanceof Error) {
    message = error.message;
  }
  message = message.trim();
  if (message === '' || internalCodePattern.test(message)) {
    return fallback;
  }
  return message;
}

// Error de una acción: permanece hasta que se cierra. Acepta el error capturado o un texto.
export function notifyError(error: unknown, fallback?: string, options?: NotifyOptions) {
  const code = error instanceof APIError ? error.code : undefined;
  return show('error', errorMessage(error, fallback), options, code);
}

// Cierra un aviso por su id (por ejemplo, el de conexión cuando el servidor vuelve a responder).
export function dismissNotice(id: string) {
  toast.dismiss(id);
}
