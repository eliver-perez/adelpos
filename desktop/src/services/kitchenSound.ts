import { notifyInfo } from './notify';

// Aviso sonoro de la terminal de cocina (etapa 9F, §5.4): dos tonos cortos generados en el equipo con
// Web Audio, sin archivos ni red (la CSP solo admite el origen propio). Nunca interrumpe la pantalla.
//
// Revisión 9F (parte 3, B2): WebKit (la ventana de macOS) y WebView2 (Windows) solo dejan sonar un
// AudioContext que se creó o se reanudó durante un toque o una tecla del usuario; uno creado al llegar la
// primera comanda, sin que nadie haya tocado la pantalla, queda suspendido. Por eso la pantalla de
// Preparación desbloquea el sonido con el primer toque o tecla (unlockKitchenSound) y, si al llegar una
// comanda el equipo todavía no permite reproducir, avisa una sola vez «Toca la pantalla para activar el
// aviso sonoro» en lugar de callar el fallo.

type AudioContextConstructor = new () => AudioContext;

let context: AudioContext | null = null;
let blockedNoticeShown = false;

// Cuánto se espera a que un contexto suspendido se reanude fuera de un toque (WebKit puede dejar la
// promesa sin resolver).
const resumeWait = 300;

export const soundBlockedMessage = 'Toca la pantalla para activar el aviso sonoro.';

function audioContext(): AudioContext | null {
  if (context) {
    return context;
  }
  const scope = window as unknown as {
    AudioContext?: AudioContextConstructor;
    webkitAudioContext?: AudioContextConstructor;
  };
  const Constructor = scope.AudioContext ?? scope.webkitAudioContext;
  if (!Constructor) {
    return null;
  }
  context = new Constructor();
  return context;
}

function running(audio: AudioContext) {
  return audio.state === 'running';
}

// Desbloquea el sonido: se llama dentro de un toque o una tecla del usuario (o al activar «Aviso sonoro»),
// que es cuando el equipo permite crear o reanudar el contexto de audio. No hace nada si ya suena.
export function unlockKitchenSound() {
  try {
    const audio = audioContext();
    if (!audio || running(audio)) {
      return;
    }
    void audio.resume().catch(() => undefined);
  } catch {
    // Sin audio disponible: el aviso de comanda nueva lo explicará al intentar sonar.
  }
}

// Un tono con subida y bajada suaves para que no chasquee.
function tone(audio: AudioContext, frequency: number, start: number, duration: number) {
  const oscillator = audio.createOscillator();
  const gain = audio.createGain();
  oscillator.type = 'sine';
  oscillator.frequency.value = frequency;
  gain.gain.setValueAtTime(0.0001, start);
  gain.gain.exponentialRampToValueAtTime(0.35, start + 0.02);
  gain.gain.exponentialRampToValueAtTime(0.0001, start + duration);
  oscillator.connect(gain);
  gain.connect(audio.destination);
  oscillator.start(start);
  oscillator.stop(start + duration + 0.02);
}

function wait(milliseconds: number) {
  return new Promise<void>((resolve) => {
    window.setTimeout(resolve, milliseconds);
  });
}

// Reproduce el aviso de comanda nueva: dos tonos ascendentes de menos de medio segundo. Devuelve si sonó;
// si el equipo no lo permite (contexto suspendido sin un toque previo o sin audio), devuelve falso.
export async function playArrivalChime(): Promise<boolean> {
  try {
    const audio = audioContext();
    if (!audio) {
      return false;
    }
    if (!running(audio)) {
      await Promise.race([audio.resume().catch(() => undefined), wait(resumeWait)]);
    }
    if (!running(audio)) {
      return false;
    }
    const now = audio.currentTime;
    tone(audio, 880, now, 0.18);
    tone(audio, 1175, now + 0.2, 0.22);
    return true;
  } catch {
    return false;
  }
}

// Avisa una sola vez por sesión de la aplicación que hace falta tocar la pantalla para oír las comandas.
export function notifySoundBlocked() {
  if (blockedNoticeShown) {
    return;
  }
  blockedNoticeShown = true;
  notifyInfo(soundBlockedMessage, { id: 'kitchen-sound-blocked' });
}

// Solo para las pruebas: vuelve al estado inicial (sin contexto ni aviso mostrado).
export function resetKitchenSoundForTests() {
  context = null;
  blockedNoticeShown = false;
}
