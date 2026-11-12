import type { TableState } from '../../services/restaurant';
import type { IconName } from '../Icon';

// Icono de cada estado de mesa (etapa 7.1; desde la etapa 9D, §1.2, también «Libre»): el estado se
// reconoce por su color, por su texto y por su icono, igual en el mosaico del mapa, en la leyenda y
// en el panel de la mesa. Los colores salen de los tokens --table-* de styles.css.
export const tableStateIcons: Record<TableState, IconName> = {
  free: 'check',
  occupied: 'clock',
  awaiting_payment: 'receipt',
  partial_payment: 'cash',
  out_of_service: 'lock',
};
