import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { linkTolerance, serverLink, ServerLinkTracker } from './serverLink';

// Etapa K1 (Fase E, §7 y K1-D10): la tolerancia de la A3 en el escritorio, con tiempo virtual.
describe('estado único de conexión', () => {
  let now = 1_000_000;
  const advance = (milliseconds: number) => {
    now += milliseconds;
  };
  let tracker: ServerLinkTracker;
  beforeEach(() => {
    now = 1_000_000;
    tracker = new ServerLinkTracker(() => now);
  });

  it('una falla aislada no muestra nada, aunque pase mucho tiempo', () => {
    tracker.noAnswer();
    advance(10 * linkTolerance.milliseconds);
    tracker.decide();
    expect(tracker.snapshot()).toEqual({ offline: false, channelDown: false });
  });

  it('tres fallas seguidas durante 20 s sí, y vuelve con el primer éxito', () => {
    tracker.noAnswer();
    advance(5_000);
    tracker.noAnswer();
    advance(5_000);
    tracker.noAnswer();
    // Tres fallas en 10 s: todavía no.
    expect(tracker.snapshot().offline).toBe(false);
    expect(tracker.nextDecisionIn()).toBe(10_000);
    advance(10_000);
    tracker.decide();
    expect(tracker.snapshot().offline).toBe(true);
    tracker.answer();
    expect(tracker.snapshot().offline).toBe(false);
  });

  it('un corte de 1 a 5 s no se nota: el éxito reinicia la racha', () => {
    for (const pause of [1_000, 3_000, 5_000]) {
      tracker.noAnswer();
      advance(pause);
      tracker.noAnswer();
      tracker.answer();
      advance(30_000);
      tracker.decide();
      expect(tracker.snapshot().offline).toBe(false);
    }
  });

  it('el canal caído más de la tolerancia con Go respondiendo es «Reconectando»', () => {
    tracker.channelAlive();
    for (let attempt = 0; attempt < 4; attempt++) {
      tracker.channelFailure();
      // La sesión sigue respondiendo cada 5 s.
      advance(5_000);
      tracker.answer();
      advance(2_000);
    }
    tracker.decide();
    expect(tracker.snapshot()).toEqual({ offline: false, channelDown: true });
    tracker.channelAlive();
    expect(tracker.snapshot().channelDown).toBe(false);
  });

  it('sin red del sistema, al momento en una terminal por la red; nunca en la computadora principal', () => {
    tracker.systemNetwork(false);
    expect(tracker.snapshot().offline).toBe(false);
    tracker.followSystemNetwork(true);
    expect(tracker.snapshot().offline).toBe(true);
    // Una respuesta tardía no lo contradice; al volver la red, espera el primer éxito.
    tracker.answer();
    expect(tracker.snapshot().offline).toBe(true);
    tracker.systemNetwork(true);
    expect(tracker.snapshot().offline).toBe(true);
    tracker.answer();
    expect(tracker.snapshot().offline).toBe(false);
  });
});

describe('la tienda del estado único', () => {
  afterEach(() => {
    vi.useRealTimers();
    serverLink.reset();
  });

  it('avisa al cruzar la tolerancia aunque no llegue otra falla', () => {
    vi.useFakeTimers();
    serverLink.reset(() => Date.now());
    const listener = vi.fn();
    const stop = serverLink.subscribe(listener);
    serverLink.noAnswer();
    serverLink.noAnswer();
    serverLink.noAnswer();
    expect(serverLink.snapshot().offline).toBe(false);
    vi.advanceTimersByTime(linkTolerance.milliseconds);
    expect(serverLink.snapshot().offline).toBe(true);
    expect(listener).toHaveBeenCalled();
    serverLink.answer();
    expect(serverLink.snapshot().offline).toBe(false);
    stop();
  });
});
