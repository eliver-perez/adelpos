import { Notice } from '../../components/Forms';
import { BrandLogo } from '../../components/BrandLogo';
import { useResource } from '../../hooks/useResource';
import { licenseStateExplanation } from '../../services/license';
import type { LicenseStatus } from '../../services/license';
import { ActivationPanel } from './ActivationPanel';

// Primer arranque sin licencia (etapa L1, Fase E; §7 de la especificación): al iniciar sesión en un sistema «Sin
// activar» o «Sin activar con datos» se ve esta pantalla, con la activación en línea y sin internet. «Continuar sin
// activar» entra al sistema, donde el aviso fijo sigue diciendo qué no se puede hacer y lleva a Configuración ›
// Licencia (con datos, por ejemplo, se pueden consultar los reportes).

export function ActivationStartPage({
  mainComputer,
  onChanged,
  onContinue,
  onLogout,
}: {
  mainComputer: boolean;
  onChanged: () => Promise<void>;
  onContinue: () => void;
  onLogout: () => void;
}) {
  const resource = useResource<LicenseStatus>('license_status');
  const status = resource.data;
  async function changed() {
    await Promise.all([resource.reload(), onChanged()]);
  }
  return (
    <main className="access-screen license-start">
      <div className="access-card license-start-card surface">
        <div className="mb-7 flex items-center gap-3">
          <BrandLogo />
        </div>
        <h1>Activa AD-EL POS</h1>
        <p className="muted mb-5 mt-2 text-sm">
          {status
            ? licenseStateExplanation(status)
            : 'Este sistema no tiene una licencia activada.'}{' '}
          Escribe el código de tu licencia para empezar a capturar pedidos.
        </p>
        <Notice>{resource.error}</Notice>
        {status && (
          <ActivationPanel status={status} mainComputer={mainComputer} onChanged={changed} />
        )}
        <div className="button-row mt-5">
          <button type="button" className="secondary-button" onClick={onContinue}>
            Continuar sin activar
          </button>
          <button type="button" className="secondary-button" onClick={onLogout}>
            Cerrar sesión
          </button>
        </div>
      </div>
    </main>
  );
}
