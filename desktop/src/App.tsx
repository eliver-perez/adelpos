import { useEffect, useState } from 'react';
import { Icon } from './components/Icon';
import { StatusCard } from './components/StatusCard';
import { useConnection } from './hooks/useConnection';
import product from './generated/product.json';

function initialTheme(): boolean {
  try { const saved = localStorage.getItem('theme'); return saved ? saved === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches; }
  catch { return false; }
}

export function App() {
  const { config, diagnostics: d, configError, refreshing, refresh } = useConnection();
  const [dark, setDark] = useState(initialTheme);
  useEffect(() => { document.documentElement.dataset.theme = dark ? 'dark' : 'light'; try { localStorage.setItem('theme', dark ? 'dark' : 'light'); } catch { /* La preferencia no impide operar. */ } }, [dark]);
  const ready = d?.backend === 'connected' && d.postgres === 'available' && d.schema === 'compatible' && d.compatible;
  const pending = !d && !configError;
  const headline = configError ? 'Revisa la configuración' : pending ? 'Conectando con tu servidor' : ready ? 'Todo está conectado' : 'La conexión necesita atención';
  const description = configError ? 'No se pudo leer la configuración de esta terminal. Revisa el archivo de conexión y vuelve a abrir la aplicación.' : d?.message ?? 'Estamos comprobando la comunicación con el servidor y la disponibilidad de PostgreSQL.';
  const mode = config?.installation_mode === 'terminal-only' ? 'Solo terminal' : config ? 'Servidor + terminal' : 'Sin verificar';

  return <div className="app-shell min-h-screen">
    <aside className="sidebar flex flex-col border-r px-5 py-7">
      <div className="mb-12 flex items-center gap-3 px-2"><span className="brand-mark">A<span>·</span></span><div><p className="text-sm font-bold tracking-wide">{product.name}</p><p className="muted mt-0.5 text-[10px] tracking-[0.16em] uppercase">Punto de venta</p></div></div>
      <p className="muted mb-3 px-3 text-[10px] font-semibold tracking-[0.18em] uppercase">Espacio de trabajo</p>
      <div className="nav-selected flex items-center gap-3 rounded-xl px-4 py-3.5 text-sm font-medium" aria-current="page"><Icon name="activity"/>Estado del sistema</div>
      <div className="mt-auto px-3 pt-16"><div className="mb-4 h-px bg-[var(--border)]"/><p className="text-xs font-medium">Entorno de desarrollo</p><p className="muted mt-2 text-xs leading-relaxed">Base técnica · Etapa 01</p><p className="muted mt-6 font-mono text-[11px]">v{product.version}</p></div>
    </aside>

    <div className="min-w-0">
      <header className="flex h-20 items-center justify-between border-b border-[var(--border)] px-8 lg:px-12"><div className="flex items-center gap-3 text-xs"><span className="muted">Sistema</span><span className="muted">/</span><span>Conexión</span></div><div className="flex items-center gap-4"><span className="muted hidden text-xs sm:block">Aplicación de escritorio</span><button className="icon-button" onClick={() => setDark(!dark)} aria-label={dark ? 'Activar tema claro' : 'Activar tema oscuro'}><Icon name={dark ? 'sun' : 'moon'}/></button></div></header>
      <main className="mx-auto max-w-6xl px-8 py-10 lg:px-12">
        <div className="mb-9 flex flex-wrap items-end justify-between gap-4"><div><p className="eyebrow mb-3">TU PUNTO DE PARTIDA</p><h1 className="text-3xl font-semibold tracking-[-0.04em]">Estado del sistema</h1><p className="muted mt-3 text-sm">Una vista clara de la conexión de tu terminal.</p></div><span className="surface rounded-full px-3.5 py-2 text-xs">Etapa 01 <span className="muted px-1">/</span> Cimientos</span></div>

        <section className={`connection-banner mb-6 rounded-2xl p-7 ${ready ? 'is-ready' : ''}`} aria-live="polite" aria-atomic="true">
          <div className="flex items-start gap-5"><div className="banner-icon flex h-12 w-12 shrink-0 items-center justify-center rounded-full"><Icon name={ready ? 'check' : 'activity'}/></div><div className="min-w-0"><div className="mb-2 flex items-center gap-2"><span className={`status-dot ${ready ? 'good' : pending ? 'neutral' : 'warning'}`}/><span className="text-[10px] font-semibold tracking-[0.14em] uppercase">{pending ? 'Verificando' : ready ? 'Conexión verificada' : 'Verificación pendiente'}</span></div><h2 className="text-2xl font-semibold tracking-tight">{headline}</h2><p className="muted mt-2 max-w-2xl text-sm leading-relaxed">{description}</p></div></div>
        </section>

        <div className="mb-8 grid gap-4 sm:grid-cols-3" aria-live="polite">
          <StatusCard icon="server" label="Backend Go" value={!d ? 'Verificando…' : d.backend === 'connected' ? 'Conectado' : d.backend === 'invalid' ? 'Incompatible' : 'Desconectado'} description={d?.serverVersion ? `Versión del servidor ${d.serverVersion}` : 'Esperando respuesta del servidor'} state={!d ? 'neutral' : d.backend === 'connected' ? 'good' : 'warning'}/>
          <StatusCard icon="database" label="PostgreSQL" value={!d || d.postgres === 'unknown' ? 'Sin verificar' : d.postgres === 'available' ? 'Disponible' : 'No disponible'} description={d?.schema === 'compatible' ? 'Esquema verificado por el backend' : d?.schema === 'incompatible' ? 'Revisar las migraciones del servidor' : 'Disponibilidad informada por el backend'} state={!d || d.postgres === 'unknown' ? 'neutral' : d.postgres === 'available' && d.schema === 'compatible' ? 'good' : 'warning'}/>
          <StatusCard icon="monitor" label="Esta instalación" value={mode} description="Una aplicación completa, conectada a Go" state={config ? 'good' : 'neutral'}/>
        </div>

        <section className="surface overflow-hidden rounded-2xl"><div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--border)] px-6 py-5"><h2 className="text-sm font-semibold">Detalles de conexión</h2><span className="muted text-xs">Comprobación automática cada 5 s</span></div><dl className="px-6"><div className="detail-row"><dt>Dirección del servidor</dt><dd className="font-mono text-xs">{config?.server_url ?? 'Sin configurar'}</dd></div><div className="detail-row"><dt>Modo de conexión</dt><dd>{mode}</dd></div><div className="detail-row"><dt>Compatibilidad de API</dt><dd>{!d ? 'Sin verificar' : d.compatible ? `${product.api_version} · Compatible` : 'No verificada'}</dd></div><div className="detail-row border-0!"><dt>Versión de la aplicación</dt><dd className="font-mono text-xs">{product.version}</dd></div></dl><div className="flex flex-wrap items-center justify-between gap-4 border-t border-[var(--border)] bg-[var(--subtle)] px-6 py-4"><p className="muted text-xs">{d ? `Última comprobación: ${d.checkedAt.toLocaleTimeString('es-MX')}` : 'Aún no se ha completado una comprobación'}</p><button className="primary-button flex items-center justify-center gap-2.5" onClick={() => void refresh()} disabled={!config || refreshing}><Icon name="refresh" className={refreshing ? 'animate-spin' : ''}/>{refreshing ? 'Comprobando…' : 'Comprobar conexión'}</button></div></section>

        <footer className="muted mt-7 flex items-start gap-3 text-xs leading-relaxed"><Icon name="server" className="mt-0.5 h-4 w-4 shrink-0"/><p>El servidor funciona de forma independiente.<br className="sm:hidden"/> Puedes cerrar esta ventana sin detener las demás terminales.</p></footer>
      </main>
    </div>
  </div>;
}
