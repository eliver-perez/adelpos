import { Accordion } from '../components/Accordion';
import { PageTitle } from '../components/Forms';
import { StatusCard } from '../components/StatusCard';
import { useConnection } from '../hooks/useConnection';
import { plainStatus } from '../services/systemStatus';
import type { CheckState } from '../services/systemStatus';
import product from '../generated/product.json';

const cardState: Record<CheckState, 'good' | 'warning' | 'neutral'> = {
  good: 'good',
  problem: 'warning',
  pending: 'neutral',
};

// Estado del sistema (etapa 9C, §4.1): ya no es una sección del menú ni la primera pantalla; se abre
// con «Ver estado del sistema» en Sistema → Configuración y, antes de iniciar sesión, desde el
// acceso. Muestra en lenguaje sencillo si el servidor responde, si hay conexión con la base de
// datos, la versión y qué hacer si algo falla. Los datos técnicos quedan plegados en «Información
// para soporte», solo para el administrador (support); antes de iniciar sesión no se sabe quién usa
// la terminal y el bloque no se muestra (revisión 9C).
export function StatusPage({ support = false }: { support?: boolean }) {
  const { config, diagnostics, configError, refreshing, refresh } = useConnection();
  const status = plainStatus(diagnostics, configError);
  const serverVersion = diagnostics?.serverVersion;
  return (
    <>
      <PageTitle
        title="Estado del sistema"
        description="Comprueba si esta terminal se comunica con el servidor de AD-EL POS."
      >
        <button
          className="primary-button"
          disabled={refreshing || !config}
          onClick={() => void refresh()}
        >
          {refreshing ? 'Comprobando…' : 'Comprobar de nuevo'}
        </button>
      </PageTitle>
      {/* Estado vigente (no el resultado de una acción): se queda en pantalla. */}
      <div
        className={`notice connection-banner system-status-summary ${
          status.tone === 'good' ? 'is-ready' : ''
        }`}
        role="status"
      >
        <strong>{status.summary}</strong>
        {status.advice && <p>Qué hacer: {status.advice}</p>}
      </div>
      <div className="my-6 grid gap-4 sm:grid-cols-3">
        <StatusCard
          icon="server"
          label="Servidor"
          value={
            status.server === 'good'
              ? 'Responde'
              : status.server === 'problem'
                ? 'No responde'
                : 'Comprobando…'
          }
          description="El equipo que guarda la información del restaurante."
          state={cardState[status.server]}
        />
        <StatusCard
          icon="database"
          label="Base de datos"
          value={
            status.database === 'good'
              ? 'Conectada'
              : status.database === 'problem'
                ? 'Con problemas'
                : 'Sin comprobar'
          }
          description="Donde se guardan ventas, cuentas y configuración."
          state={cardState[status.database]}
        />
        <StatusCard
          icon="monitor"
          label="Versión de AD-EL POS"
          value={product.version}
          description={
            status.versionMatches === 'good'
              ? 'La terminal y el servidor usan la misma versión.'
              : status.versionMatches === 'problem'
                ? `El servidor tiene la versión ${serverVersion ?? 'desconocida'}.`
                : 'Se compara con la del servidor al conectar.'
          }
          state={cardState[status.versionMatches]}
        />
      </div>
      {support && (
        <Accordion
          title="Información para soporte"
          summary="Datos técnicos de la conexión para quien da mantenimiento."
          className="support-information"
        >
          <dl>
            <div className="detail-row">
              <dt>Diagnóstico</dt>
              <dd>
                {configError
                  ? 'No se pudo leer la configuración de conexión.'
                  : (diagnostics?.message ?? 'Comprobando la conexión…')}
              </dd>
            </div>
            <div className="detail-row">
              <dt>Servidor</dt>
              <dd className="font-mono">{config?.server_url ?? 'Sin configurar'}</dd>
            </div>
            <div className="detail-row">
              <dt>Instalación</dt>
              <dd>
                {config
                  ? config.installation_mode === 'terminal-only'
                    ? 'Solo terminal'
                    : 'Servidor + terminal'
                  : 'Sin verificar'}
              </dd>
            </div>
            <div className="detail-row">
              <dt>Aplicación</dt>
              <dd>{product.version}</dd>
            </div>
            <div className="detail-row">
              <dt>Servidor (versión)</dt>
              <dd>{serverVersion ?? 'Sin respuesta'}</dd>
            </div>
            <div className="detail-row">
              <dt>API</dt>
              <dd>
                {product.api_version} · {diagnostics?.compatible ? 'Compatible' : 'Sin verificar'}
              </dd>
            </div>
            <div className="detail-row">
              <dt>Esquema</dt>
              <dd>
                {product.schema_version} ·{' '}
                {diagnostics?.schema === 'compatible'
                  ? 'Compatible'
                  : diagnostics?.schema === 'incompatible'
                    ? 'Requiere revisión'
                    : 'Sin verificar'}
              </dd>
            </div>
            <div className="detail-row">
              <dt>Última comprobación</dt>
              <dd>{diagnostics?.checkedAt.toLocaleTimeString('es-MX') ?? 'Pendiente'}</dd>
            </div>
          </dl>
          <p className="muted text-sm">Cerrar esta aplicación no detiene Go ni PostgreSQL.</p>
        </Accordion>
      )}
    </>
  );
}
