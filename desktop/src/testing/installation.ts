import type { InstallationState } from '../services/installation';

// Modalidad de las pruebas que simulan el puente nativo (etapa 9G): la computadora principal, como hasta
// la 9F, ya elegida.
export const serverTerminalInstallation: InstallationState = {
  chosen: true,
  mode: 'server-terminal',
  server_configured: true,
  can_change: true,
  service_installed: true,
  service_enabled: true,
  server_data_kept: false,
};
