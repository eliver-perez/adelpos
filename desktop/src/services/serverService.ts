import { api } from './api';
import { countLabel } from './plural';

// Servicio del servidor (etapa 9G, fase A): estado y reinicio, en Mantenimiento y en la pantalla de
// «Solo servidor». Go decide qué está disponible (solo un servicio administrado por la computadora se
// puede reiniciar) y cuenta las terminales conectadas; la pantalla solo lo explica.

export interface ServiceStatus {
  // El servidor corre como servicio de la computadora: arranca con ella, aunque nadie inicie sesión.
  managed: boolean;
  restart_available: boolean;
  installation_mode: string;
  service_enabled: boolean;
  started_at: string;
  uptime_seconds: number;
  server_version: string;
  terminals: {
    connected: number;
    registered: number;
  };
  // Último respaldo automático: la fase F lo llena; mientras tanto llega vacío.
  last_backup: unknown;
}

export function readServiceStatus() {
  return api<ServiceStatus>('maintenance_service');
}

export function restartService() {
  return api<{ restarting: boolean }>('maintenance_service_restart', { body: {} });
}

// «Activo desde hace…» en palabras del restaurante.
export function uptimeLabel(seconds: number) {
  if (seconds < 60) {
    return 'menos de un minuto';
  }
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) {
    return countLabel(minutes, 'minuto', 'minutos');
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    const rest = minutes % 60;
    const hoursText = countLabel(hours, 'hora', 'horas');
    return rest > 0 ? `${hoursText} y ${countLabel(rest, 'minuto', 'minutos')}` : hoursText;
  }
  const days = Math.floor(hours / 24);
  return countLabel(days, 'día', 'días');
}

export function terminalsLabel(status: ServiceStatus) {
  const { connected, registered } = status.terminals;
  if (registered === 0) {
    return 'Todavía no hay otras terminales registradas.';
  }
  return `${countLabel(connected, 'terminal conectada', 'terminales conectadas')} de ${registered}.`;
}
