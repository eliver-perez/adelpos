import type { BackupRun, BackupState } from '../services/backups';

// Respaldos de las pruebas que simulan el puente nativo (etapa 9G, fase F): sin respaldos todavía, con la carpeta
// predeterminada y la programación de una instalación nueva.
export function backupState(overrides: Partial<BackupState> = {}): BackupState {
  return {
    settings: {
      enabled: true,
      time: null,
      effective_time: '05:00',
      business_day_cutoff: '05:00',
      keep: 30,
      directory: { custom: false, path: '', folder_name: '', volume_name: '', available: false },
      default_directory: {
        custom: false,
        path: '/Library/Application Support/AD-EL POS/backups',
        folder_name: 'backups',
        volume_name: '',
        available: true,
      },
    },
    next_run_at: '2026-11-15T11:00:00Z',
    pending: false,
    running: false,
    last_backup: null,
    last_success: null,
    alert: null,
    runs: [],
    directory_change_available: true,
    ...overrides,
  };
}

export function backupRun(overrides: Partial<BackupRun> = {}): BackupRun {
  return {
    id: '00000000-0000-4000-8000-0000000000b1',
    kind: 'automatic',
    status: 'succeeded',
    started_at: '2026-11-14T11:00:00Z',
    finished_at: '2026-11-14T11:00:04Z',
    file_name: 'adel_pos-20261114T110000Z-a1b2c3-automatico.dump',
    size_bytes: 5_662_310,
    location: 'default',
    folder_name: 'backups',
    volume_name: '',
    directory: '/Library/Application Support/AD-EL POS/backups',
    used_default_directory: true,
    fallback_reason: '',
    verified: true,
    failure_reason: '',
    removed: false,
    available: true,
    ...overrides,
  };
}
