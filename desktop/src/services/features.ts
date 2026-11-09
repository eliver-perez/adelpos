import { api } from './api';

// Funciones activables del establecimiento (backend/internal/features). Go decide qué funciones
// existen, guarda su estado en PostgreSQL y lo hace cumplir; la interfaz solo oculta lo que no
// aplica según esta lista.

export type FeatureCode = 'usd_cash';

export interface EstablishmentFeature {
  code: FeatureCode | string;
  name: string;
  description: string;
  enabled: boolean;
  revision: number;
  updated_at: string | null;
  updated_by_name: string | null;
}

export interface FeatureInput {
  code: string;
  enabled: boolean;
  revision: number;
}

export function listFeatures() {
  return api<EstablishmentFeature[]>('settings_features');
}

// Cambiar una función no usa operation_id: es una escritura de configuración con revisión
// optimista, como la configuración general.
export function saveFeature(input: FeatureInput) {
  return api<EstablishmentFeature>('save_settings_feature', { body: input });
}

export function featureEnabled(
  features: EstablishmentFeature[] | null | undefined,
  code: FeatureCode,
) {
  return !!features?.find((feature) => feature.code === code)?.enabled;
}
