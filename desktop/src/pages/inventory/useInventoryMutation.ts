import { notifyInventoryChanged } from '../../hooks/useInventoryEvents';
import { useRestaurantMutation } from '../../hooks/useRestaurantMutation';
import { APIError } from '../../services/api';
import { inventoryPendingKey, inventoryRefreshCodes } from '../../services/inventory';
import type { InventoryExecute } from '../../services/inventory';
import { notifyError, notifySuccess } from '../../services/notify';

// Mensaje de un conflicto de inventario. Go explica qué registro cambió («La unidad cambió en otra
// terminal…»); la pantalla ya cargó la versión vigente, así que se pide revisarla en lugar de
// «actualizar».
export function conflictMessage(problem: APIError) {
  if (problem.code === 'REVISION_CONFLICT') {
    const subject = problem.message.split('. ')[0]?.trim() || 'El registro cambió en otra terminal';
    const sentence = subject.endsWith('.') ? subject : `${subject}.`;
    return `${sentence} Se cargaron los datos vigentes: revísalos y vuelve a guardar.`;
  }
  return `${problem.message} Se cargaron los datos vigentes.`;
}

// Escrituras de la sección Inventarios. Todas llevan operation_id (useRestaurantMutation) y viven
// en el almacén global de pendientes con la clave «inventory»: un resultado incierto sobrevive al
// cambio de pestaña o de pantalla y se recupera con el mismo identificador. Al confirmarse una
// escritura, la pestaña visible vuelve a consultar sus datos; ante un conflicto de revisión o de
// idempotencia también, y el error explica que se cargó la versión vigente.
export function useInventoryMutation() {
  const mutation = useRestaurantMutation(() => notifyInventoryChanged(), inventoryPendingKey);

  const execute: InventoryExecute = async (operation, options) => {
    try {
      return await mutation.execute(operation, options);
    } catch (problem) {
      if (problem instanceof APIError && inventoryRefreshCodes.includes(problem.code)) {
        notifyInventoryChanged();
        throw new APIError(problem.code, conflictMessage(problem), problem.status, problem.details);
      }
      throw problem;
    }
  };

  // Reintento con el mismo operation_id y el mismo cuerpo; el resultado se avisa como el de
  // cualquier acción.
  async function retry() {
    const problem = await mutation.retry();
    if (!problem) {
      notifySuccess('Cambio de inventario confirmado.');
      return;
    }
    if (!mutation.isPending()) {
      notifyError(problem, 'No se pudo confirmar el cambio de inventario.');
    }
  }

  return {
    execute,
    retry,
    running: mutation.busy,
    uncertain: mutation.uncertain,
    // Mientras hay una operación en curso o sin resultado confirmado no se envía otra.
    busy: mutation.busy || mutation.uncertain,
  };
}

export type InventoryMutation = ReturnType<typeof useInventoryMutation>;
