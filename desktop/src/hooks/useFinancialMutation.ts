import type { Operation } from '../services/api';
import { paymentPendingKey } from '../services/payments';
import { useRestaurantMutation } from './useRestaurantMutation';

// Todas las escrituras financieras de una cuenta comparten la misma clave del almacén global: una
// operación incierta de cualquier tipo bloquea las demás de esa cuenta hasta resolverla.
export function useFinancialMutation(
  accountID: string,
  onResult: (operation: Operation, result: unknown) => void,
) {
  return useRestaurantMutation((operation, result) => {
    window.dispatchEvent(new CustomEvent('adel-server-change'));
    onResult(operation, result);
  }, paymentPendingKey(accountID));
}

export type FinancialMutation = ReturnType<typeof useFinancialMutation>;
