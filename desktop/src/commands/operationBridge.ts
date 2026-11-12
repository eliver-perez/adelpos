import type { RestaurantView } from '../hooks/useRestaurantWorkspace';

// Puente entre la pantalla operativa (Mesas y Para llevar) y el registro de acciones (etapa 9C,
// fase C). La pantalla informa su contexto (para habilitar opciones del menú, la paleta y los
// atajos) y entrega las mismas funciones que usan sus botones; el registro nunca escribe al
// servidor por su cuenta.

// Contexto que habilita o deshabilita acciones. Solo valores simples: App lo guarda en su estado y
// la pantalla lo vuelve a informar únicamente cuando cambia.
export interface OperationCommandState {
  // La operación está a la vista (Mesas o Para llevar).
  active: boolean;
  view: RestaurantView;
  captureMode: boolean;
  checkoutOpen: boolean;
  // Hay una operación en curso o con resultado incierto: los botones están desactivados.
  blocked: boolean;
  // Mesa en la que «Nueva cuenta en mesa» abriría la captura (la del panel lateral o la de la
  // captura), o null si no hay ninguna lista.
  tableForNewAccount: string | null;
  // «Precuenta» está disponible en la cuenta a la vista.
  precheckReady: boolean;
}

export interface OperationTable {
  id: string;
  name: string;
  areaName: string;
}

// Funciones de los botones de la pantalla operativa.
export interface OperationCommandHandlers {
  // «Capturar» o «Nueva cuenta» en la mesa de tableForNewAccount.
  newTableAccount: () => void;
  // «Cuenta sin mesa» (Mesas → Cuentas).
  accountWithoutTable: () => void;
  // «Nuevo pedido» (Para llevar).
  newTakeaway: () => void;
  // «Precuenta» de la cuenta a la vista.
  openPrecheck: () => void;
  // «‹ Mesas» o «‹ Para llevar» de la barra reducida de la captura.
  leaveCapture: () => void;
  // Tocar la mesa en el mapa.
  chooseTable: (id: string) => void;
  // Mesas del mapa para buscarlas en la paleta («mesa 5»).
  tables: () => OperationTable[];
  // Escape: cierra el panel abierto o vuelve un nivel (de la captura al mapa o al listado, y del
  // panel de la mesa al mapa). Devuelve false si no hay nada que cerrar.
  back: () => boolean;
}
