import { useSyncExternalStore } from 'react';

// Estado único de la conexión con el servidor (etapa K1, Fase E; §7 de la especificación y decisión K1-D10), con la
// misma tolerancia que la aplicación Android desde la A3 (android-core, ServerLinkTracker):
//
// - una falla aislada no cambia nada visible;
// - «Sin conexión» solo con 3 fallas seguidas y 20 s sin ningún éxito desde la primera, o si el sistema dice que no hay
//   red (solo en una terminal por la red: la computadora principal habla con su propio servidor aunque no haya red);
// - «En línea» con el primer éxito;
// - «Reconectando» (ámbar) solo si el canal de eventos está caído más de esa misma tolerancia mientras Go sí responde.
//
// Lo alimentan la consulta de la sesión y cada respuesta de services/api.ts (cualquier respuesta HTTP es un éxito; solo
// no llegar al servidor es una falla) y los mensajes del canal de eventos (useRealtime: sus fallas cuentan también para
// «Sin conexión»; sus mensajes solo dicen que el canal está vivo). Lo leen la barra, la capa «Sin conexión con el
// servidor», el aviso persistente y los avisos de Mesas y de Preparación: nunca se contradicen.
//
// La causa del «Reconectando» falso de la 0.15.0 era otra (Rust trataba como falla la renovación normal del canal cada
// 30 s, realtime.rs); esto es la tolerancia que pide la §7 para que un corte breve tampoco se note.

export const linkTolerance = { failures: 3, milliseconds: 20_000 } as const;

export interface LinkSnapshot {
  // Sin conexión con el servidor (la barra en rojo y la capa que bloquea las operaciones).
  offline: boolean;
  // El canal de eventos está caído más de la tolerancia (con Go respondiendo, la barra dice «Reconectando»).
  channelDown: boolean;
}

// Racha de fallas seguidas de una fuente: cuántas y desde cuándo.
interface Streak {
  failures: number;
  since: number | null;
}

function exceeded(streak: Streak, now: number) {
  return (
    streak.since !== null &&
    streak.failures >= linkTolerance.failures &&
    now - streak.since >= linkTolerance.milliseconds
  );
}

// Reglas puras, con el reloj inyectable para las pruebas.
export class ServerLinkTracker {
  private requests: Streak = { failures: 0, since: null };
  private channel: Streak = { failures: 0, since: null };
  private offline = false;
  private channelDown = false;
  private systemOffline = false;
  private followSystem = false;

  constructor(private readonly now: () => number = () => Date.now()) {}

  // El servidor respondió (cualquier respuesta HTTP).
  answer() {
    this.requests = { failures: 0, since: null };
    // Sin red del sistema, una respuesta tardía no lo contradice: se espera a que vuelva la red.
    if (!(this.followSystem && this.systemOffline)) {
      this.offline = false;
    }
  }

  // Una petición no llegó al servidor (sin respuesta, plazo vencido, conexión rechazada).
  noAnswer() {
    this.requests.failures += 1;
    this.requests.since ??= this.now();
    this.decide();
  }

  // El canal de eventos se abrió o entregó un mensaje: el canal está vivo. No reinicia la racha de las peticiones: «Sin
  // conexión» lo deciden las peticiones a la API (la consulta de la sesión se repite cada pocos segundos y es la primera
  // en volver).
  channelAlive() {
    this.channel = { failures: 0, since: null };
    this.channelDown = false;
  }

  // El canal de eventos falló (Rust manda «reconnecting» solo con una falla real, nunca con la renovación normal).
  channelFailure() {
    this.channel.failures += 1;
    this.channel.since ??= this.now();
    this.noAnswer();
  }

  // El canal se cerró a propósito (otra vista, otra sesión): su racha deja de contar.
  channelClosed() {
    this.channel = { failures: 0, since: null };
    this.channelDown = false;
  }

  // Si se sigue la red del sistema (una terminal por la red; la computadora principal no).
  followSystemNetwork(follow: boolean) {
    this.followSystem = follow;
    if (follow && this.systemOffline) {
      this.offline = true;
    }
  }

  // El sistema informa que hay o no hay red. Sin red, «Sin conexión» al momento; al volver, hasta la primera respuesta.
  systemNetwork(online: boolean) {
    this.systemOffline = !online;
    if (!online && this.followSystem) {
      this.offline = true;
    }
  }

  // Vuelve a evaluar las condiciones que dependen del tiempo (la tienda lo llama con nextDecisionIn).
  decide() {
    const now = this.now();
    if (exceeded(this.requests, now)) {
      this.offline = true;
    }
    if (exceeded(this.channel, now)) {
      this.channelDown = true;
    }
  }

  // Milisegundos hasta que una racha en curso pueda cruzar la tolerancia (null si ninguna puede).
  nextDecisionIn(): number | null {
    const now = this.now();
    const pending = [this.requests, this.channel]
      .filter(
        (streak) =>
          streak.since !== null &&
          streak.failures >= linkTolerance.failures &&
          !exceeded(streak, now),
      )
      .map((streak) => streak.since! + linkTolerance.milliseconds - now);
    return pending.length > 0 ? Math.max(0, Math.min(...pending)) : null;
  }

  snapshot(): LinkSnapshot {
    return { offline: this.offline, channelDown: this.channelDown && !this.offline };
  }
}

// La tienda de la aplicación: un solo rastreador para toda la ventana.
let tracker = new ServerLinkTracker();
let current: LinkSnapshot = tracker.snapshot();
let timer: ReturnType<typeof setTimeout> | null = null;
const listeners = new Set<() => void>();

function publish() {
  const next = tracker.snapshot();
  if (next.offline !== current.offline || next.channelDown !== current.channelDown) {
    current = next;
    for (const listener of listeners) {
      listener();
    }
  }
  if (timer !== null) {
    clearTimeout(timer);
    timer = null;
  }
  const wait = tracker.nextDecisionIn();
  if (wait !== null) {
    timer = setTimeout(() => {
      timer = null;
      tracker.decide();
      publish();
    }, wait);
  }
}

function update(change: (link: ServerLinkTracker) => void) {
  change(tracker);
  publish();
}

export const serverLink = {
  answer: () => update((link) => link.answer()),
  noAnswer: () => update((link) => link.noAnswer()),
  channelAlive: () => update((link) => link.channelAlive()),
  channelFailure: () => update((link) => link.channelFailure()),
  channelClosed: () => update((link) => link.channelClosed()),
  followSystemNetwork: (follow: boolean) => update((link) => link.followSystemNetwork(follow)),
  systemNetwork: (online: boolean) => update((link) => link.systemNetwork(online)),
  snapshot: () => current,
  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },
  // Solo las pruebas: un rastreador nuevo con su reloj.
  reset(now?: () => number) {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
    tracker = new ServerLinkTracker(now);
    current = tracker.snapshot();
    for (const listener of listeners) {
      listener();
    }
  },
};

export function useServerLink(): LinkSnapshot {
  return useSyncExternalStore(serverLink.subscribe, serverLink.snapshot, serverLink.snapshot);
}
