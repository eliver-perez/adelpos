import { Notice, PageTitle } from '../components/Forms';
import { StatusCard } from '../components/StatusCard';
import { useConnection } from '../hooks/useConnection';
import product from '../generated/product.json';

export function StatusPage() {
  const { config, diagnostics, configError, refreshing, refresh } = useConnection();
  const connected = diagnostics?.backend === 'connected';
  const databaseReady =
    diagnostics?.postgres === 'available' && diagnostics.schema === 'compatible';
  const mode =
    config?.installation_mode === 'terminal-only' ? 'Solo terminal' : 'Servidor + terminal';
  return (
    <>
      <PageTitle
        title="Estado del sistema"
        description="Conectividad real entre escritorio, Go y PostgreSQL."
      >
        <button
          className="primary-button"
          disabled={refreshing || !config}
          onClick={() => void refresh()}
        >
          {refreshing ? 'Comprobando…' : 'Comprobar conexión'}
        </button>
      </PageTitle>
      <Notice success={Boolean(connected && databaseReady && diagnostics?.compatible)}>
        {configError
          ? 'No se pudo leer la configuración de conexión.'
          : (diagnostics?.message ?? 'Comprobando la conexión…')}
      </Notice>
      <div className="my-6 grid gap-4 sm:grid-cols-3">
        <StatusCard
          icon="server"
          label="Backend Go"
          value={connected ? 'Conectado' : diagnostics ? 'Desconectado' : 'Verificando…'}
          description={
            diagnostics?.serverVersion
              ? `Versión ${diagnostics.serverVersion}`
              : 'Esperando respuesta'
          }
          state={connected ? 'good' : 'warning'}
        />
        <StatusCard
          icon="database"
          label="PostgreSQL"
          value={
            databaseReady
              ? 'Disponible'
              : diagnostics?.postgres === 'unavailable'
                ? 'No disponible'
                : 'Sin verificar'
          }
          description={
            diagnostics?.schema === 'compatible'
              ? 'Esquema compatible'
              : 'Comprobación realizada por Go'
          }
          state={databaseReady ? 'good' : 'neutral'}
        />
        <StatusCard
          icon="monitor"
          label="Esta instalación"
          value={config ? mode : 'Sin verificar'}
          description="El servidor permanece independiente de esta ventana"
          state={config ? 'good' : 'neutral'}
        />
      </div>
      <section className="surface panel">
        <h2 className="mb-4">Detalles técnicos</h2>
        <dl>
          <div className="detail-row">
            <dt>Servidor</dt>
            <dd className="font-mono">{config?.server_url ?? 'Sin configurar'}</dd>
          </div>
          <div className="detail-row">
            <dt>Aplicación</dt>
            <dd>{product.version}</dd>
          </div>
          <div className="detail-row">
            <dt>API</dt>
            <dd>
              {product.api_version} · {diagnostics?.compatible ? 'Compatible' : 'Sin verificar'}
            </dd>
          </div>
          <div className="detail-row">
            <dt>Última comprobación</dt>
            <dd>{diagnostics?.checkedAt.toLocaleTimeString('es-MX') ?? 'Pendiente'}</dd>
          </div>
        </dl>
        <p className="muted mt-5 text-sm">Cerrar esta aplicación no detiene Go ni PostgreSQL.</p>
      </section>
    </>
  );
}
