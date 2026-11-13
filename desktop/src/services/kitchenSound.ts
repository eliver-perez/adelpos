// Aviso sonoro de la terminal de cocina (etapa 9F, §5.4): dos tonos cortos generados en el equipo con
// Web Audio, sin archivos ni red (la CSP solo admite el origen propio). Nunca interrumpe la pantalla: si
// el equipo no permite reproducir sonido, el aviso se omite en silencio.

type AudioContextConstructor = new () => AudioContext;

let context: AudioContext | null = null;

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

// Reproduce el aviso de comanda nueva: dos tonos ascendentes de menos de medio segundo.
export async function playArrivalChime() {
  try {
    const audio = audioContext();
    if (!audio) {
      return;
    }
    if (audio.state === 'suspended') {
      await audio.resume();
    }
    const now = audio.currentTime;
    tone(audio, 880, now, 0.18);
    tone(audio, 1175, now + 0.2, 0.22);
  } catch {
    // Sin sonido disponible no se avisa; la tarjeta nueva sigue apareciendo en la pantalla.
  }
}
