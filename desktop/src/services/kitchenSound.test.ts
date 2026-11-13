// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Revisión 9F (parte 3, B2): el aviso sonoro de la terminal de cocina. WebKit y WebView2 dejan suspendido
// un contexto de audio creado fuera de un toque del usuario y no lo reanudan fuera de un gesto; el primer
// toque o tecla lo desbloquea y, si al llegar una comanda todavía no puede sonar, se avisa una sola vez.

const notify = vi.hoisted(() => ({ info: vi.fn() }));
vi.mock('./notify', () => ({ notifyInfo: notify.info }));

import {
  notifySoundBlocked,
  playArrivalChime,
  resetKitchenSoundForTests,
  soundBlockedMessage,
  unlockKitchenSound,
} from './kitchenSound';

// Contexto de audio simulado: empieza suspendido; resume() solo lo reanuda dentro de un gesto del usuario.
class FakeAudioContext {
  static instances: FakeAudioContext[] = [];
  static gesture = false;
  state: 'suspended' | 'running' = 'suspended';
  currentTime = 0;
  destination = {};
  oscillators = 0;

  constructor() {
    FakeAudioContext.instances.push(this);
  }

  resume() {
    if (FakeAudioContext.gesture) {
      this.state = 'running';
      return Promise.resolve();
    }
    // WebKit deja la promesa sin resolver fuera de un gesto.
    return new Promise<void>(() => undefined);
  }

  createOscillator() {
    this.oscillators += 1;
    return {
      type: '',
      frequency: { value: 0 },
      connect: () => undefined,
      start: () => undefined,
      stop: () => undefined,
    };
  }

  createGain() {
    return {
      gain: {
        setValueAtTime: () => undefined,
        exponentialRampToValueAtTime: () => undefined,
      },
      connect: () => undefined,
    };
  }
}

beforeEach(() => {
  resetKitchenSoundForTests();
  FakeAudioContext.instances = [];
  FakeAudioContext.gesture = false;
  (window as unknown as { AudioContext: unknown }).AudioContext = FakeAudioContext;
});

afterEach(() => {
  vi.clearAllMocks();
  delete (window as unknown as { AudioContext?: unknown }).AudioContext;
});

describe('aviso sonoro de la terminal de cocina', () => {
  it('sin un toque previo no puede sonar y lo informa; después del primer toque suena', async () => {
    expect(await playArrivalChime()).toBe(false);
    expect(FakeAudioContext.instances[0]?.oscillators).toBe(0);
    // El primer toque de la pantalla reanuda el contexto dentro del gesto.
    FakeAudioContext.gesture = true;
    unlockKitchenSound();
    FakeAudioContext.gesture = false;
    await Promise.resolve();
    expect(FakeAudioContext.instances[0]?.state).toBe('running');
    expect(await playArrivalChime()).toBe(true);
    expect(FakeAudioContext.instances).toHaveLength(1);
    expect(FakeAudioContext.instances[0]?.oscillators).toBe(2);
  });

  it('sin audio en el equipo no suena ni falla', async () => {
    delete (window as unknown as { AudioContext?: unknown }).AudioContext;
    expect(await playArrivalChime()).toBe(false);
    expect(() => unlockKitchenSound()).not.toThrow();
  });

  it('el aviso «Toca la pantalla» se muestra una sola vez', () => {
    notifySoundBlocked();
    notifySoundBlocked();
    expect(notify.info).toHaveBeenCalledTimes(1);
    expect(notify.info).toHaveBeenCalledWith(soundBlockedMessage, { id: 'kitchen-sound-blocked' });
    expect(soundBlockedMessage).toBe('Toca la pantalla para activar el aviso sonoro.');
  });
});
