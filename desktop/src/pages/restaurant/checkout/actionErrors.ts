import { APIError } from '../../../services/api';
import { notifyError } from '../../../services/notify';

// Errores de las escrituras financieras y de documentos de una cuenta (etapa 7.1).
//
// Un resultado incierto (sin conexión o error ≥ 500) no es un error de la acción: la operación
// pudo registrarse y su panel de recuperación permanece en pantalla con «Reintentar con el mismo
// identificador». Cualquier otro rechazo de Go se muestra como aviso persistente.

export function uncertainProblem(problem: unknown) {
  return (
    problem instanceof APIError && (problem.code === 'CONNECTION_ERROR' || problem.status >= 500)
  );
}

export function notifyActionError(problem: unknown, fallback: string) {
  if (uncertainProblem(problem)) {
    return;
  }
  notifyError(problem, fallback);
}
