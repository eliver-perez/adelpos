import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { errorMessage } from '../../services/notify';
import { licenseCapability } from '../../services/license';
import { setKitchenDisplayMode } from '../../services/licenseOperations';
import { serverHas } from '../../services/serverCapabilities';

// Cupo de pantalla de cocina de Preparación (etapa L1, Fase E; §5 de la especificación, L1-D7 y L1-D28). Al entrar a
// Preparación la terminal avisa a Go (POST kitchen/display-mode con active: true) y al salir lo libera (active: false).
// Sin cupo, se dice qué cupo falta y cuántos usa la licencia (el texto sale de services/license.ts) en lugar de mostrar
// el tablero. Con un servidor anterior (sin license-v1) no se avisa nada y Preparación se ve como siempre.
//
// Los avisos se envían en orden (una cadena de promesas): si se entra y se sale muy rápido, el último que llega a Go
// es el de salir.

let pending: Promise<unknown> = Promise.resolve();

function sendInOrder(active: boolean) {
  const next = pending.then(() => setKitchenDisplayMode(active));
  pending = next.catch(() => undefined);
  return next;
}

type SeatState = { phase: 'asking' } | { phase: 'granted' } | { phase: 'denied'; message: string };

export function PreparationSeat({ children }: { children: ReactNode }) {
  const tracked = serverHas(licenseCapability);
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<SeatState>({ phase: tracked ? 'asking' : 'granted' });

  useEffect(() => {
    if (!tracked) {
      return;
    }
    let active = true;
    setState({ phase: 'asking' });
    sendInOrder(true)
      .then(() => {
        if (active) setState({ phase: 'granted' });
      })
      .catch((error: unknown) => {
        if (active)
          setState({
            phase: 'denied',
            message: errorMessage(error, 'No se pudo abrir Preparación en esta terminal.'),
          });
      });
    return () => {
      active = false;
      // Salir nunca falla por la licencia en Go; si no hay conexión, el cupo se libera al cerrar la sesión.
      void sendInOrder(false).catch(() => undefined);
    };
  }, [tracked, attempt]);

  if (state.phase === 'granted') {
    return <>{children}</>;
  }
  if (state.phase === 'asking') {
    return (
      <p className="muted" role="status">
        Abriendo Preparación…
      </p>
    );
  }
  return (
    <section
      className="surface panel form-stack preparation-seat"
      aria-labelledby="preparation-seat-title"
    >
      <h2 id="preparation-seat-title">Preparación no está disponible en esta terminal</h2>
      <p role="alert">{state.message}</p>
      <div className="button-row">
        <button
          type="button"
          className="secondary-button"
          onClick={() => setAttempt((current) => current + 1)}
        >
          Volver a intentar
        </button>
      </div>
    </section>
  );
}
