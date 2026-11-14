import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { IconLabel } from '../components/Icon';

// Capa «Sin conexión con el servidor» de la terminal de escritorio (etapa 9G, fase C, §4.2 y decisión D11).
//
// - Aparece sobre la pantalla cuando la terminal pierde al servidor con una sesión abierta y **bloquea las
//   operaciones** sin desmontar nada: App.tsx deja el contenido inerte (sin clics ni teclado) debajo de esta capa,
//   de modo que lo capturado y no guardado sigue en su pantalla mientras la ventana siga abierta.
// - La barra superior queda a la vista y usable (indica «Sin conexión»); la capa empieza debajo de ella. En la
//   captura, la barra reducida es parte del contenido y queda cubierta.
// - Se reconecta sola con espera creciente (useSession) y dice cuándo es el próximo intento; «Reintentar» lo hace
//   ya. Al volver la conexión la capa desaparece y la terminal vuelve a consultar el estado vigente.

function barBottom() {
  const bar = document.querySelector('.app-topbar, .server-only-bar');
  return bar ? Math.max(0, Math.round(bar.getBoundingClientRect().bottom)) : 0;
}

export function ConnectionLostLayer({
  detail,
  retryAt,
  onRetry,
}: {
  detail: string;
  retryAt: number | null;
  onRetry: () => void;
}) {
  const [now, setNow] = useState(() => Date.now());
  const [top, setTop] = useState(0);
  const retry = useRef<HTMLButtonElement>(null);

  // La capa empieza donde termina la barra superior (cambia de alto con el ancho de la ventana).
  useLayoutEffect(() => {
    const measure = () => setTop(barBottom());
    measure();
    window.addEventListener('resize', measure);
    const bar = document.querySelector('.app-topbar, .server-only-bar');
    const observer =
      bar && typeof ResizeObserver !== 'undefined' ? new ResizeObserver(measure) : null;
    if (bar) {
      observer?.observe(bar);
    }
    return () => {
      window.removeEventListener('resize', measure);
      observer?.disconnect();
    };
  }, []);

  // El foco pasa a «Reintentar» para que el teclado no siga actuando sobre la pantalla de abajo.
  useLayoutEffect(() => {
    retry.current?.focus();
  }, []);

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 500);
    return () => window.clearInterval(timer);
  }, []);

  const seconds = retryAt === null ? 0 : Math.max(0, Math.ceil((retryAt - now) / 1000));
  return (
    <div className="connection-lost-layer" style={{ top }}>
      <section
        className="connection-lost-card"
        role="alertdialog"
        aria-labelledby="connection-lost-title"
        aria-describedby="connection-lost-description"
      >
        <h2 id="connection-lost-title">
          <IconLabel icon="alert">Sin conexión con el servidor</IconLabel>
        </h2>
        <p id="connection-lost-description">
          Esta terminal no se comunica con la computadora principal del restaurante. Las operaciones
          quedan en pausa hasta que vuelva la conexión.
        </p>
        <p className="connection-lost-kept">
          Lo que capturaste y todavía no se guardó se conserva en esta pantalla mientras no cierres
          la ventana.
        </p>
        {detail && <p className="muted connection-lost-detail">{detail}</p>}
        <p className="connection-lost-retry" aria-live="polite">
          {seconds > 0
            ? `Se vuelve a intentar sola en ${seconds} s.`
            : 'Intentando conectar con el servidor…'}
        </p>
        <div className="button-row">
          <button ref={retry} type="button" className="primary-button" onClick={onRetry}>
            <IconLabel icon="refresh">Reintentar</IconLabel>
          </button>
        </div>
      </section>
    </div>
  );
}
