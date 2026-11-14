import { createHash, randomBytes, X509Certificate } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';

// Etapa 9G, fase B: la terminal nueva simulada por la API de Go, como lo hace Rust en
// src-tauri/src/pairing.rs: compromiso SHA-256 de su número aleatorio, número del servidor, revelación y
// código de 6 dígitos = SHA-256(huella SPKI ‖ nT ‖ nS ‖ id), primeros 8 bytes big-endian módulo 10⁶. El
// servidor temporal de las pruebas escucha solo en loopback (sin la escucha de red), así que la huella sale
// del certificado que Go creó en la carpeta identity junto a su configuración (la misma carpeta del token
// de aprovisionamiento); con Rust sale del intercambio TLS.

const serverURL = process.env.ADEL_BROWSER_SERVER_URL ?? 'http://127.0.0.1:8183';

export function serverFingerprint(): Buffer {
  const directory = dirname(process.env.ADEL_BROWSER_PROVISIONING!);
  const pem = readFileSync(join(directory, 'identity', 'server-cert.pem'), 'utf8');
  const spki = new X509Certificate(pem).publicKey.export({ type: 'spki', format: 'der' });
  return createHash('sha256').update(spki).digest();
}

export function pairingCode(
  fingerprint: Buffer,
  terminalNonce: Buffer,
  serverNonce: Buffer,
  requestID: string,
) {
  const digest = createHash('sha256')
    .update(fingerprint)
    .update(terminalNonce)
    .update(serverNonce)
    .update(Buffer.from(requestID.replace(/-/g, ''), 'hex'))
    .digest();
  return (digest.readBigUInt64BE(0) % 1000000n).toString().padStart(6, '0');
}

export interface PairingSession {
  requestID: string;
  secret: string;
  serverID: string;
  serverName: string;
  name: string;
  expiresAt: string;
  code: string;
}

interface Reply {
  status: number;
  body: Record<string, unknown>;
}

export async function pairingCall(
  method: string,
  route: string,
  secret?: string,
  body?: unknown,
): Promise<Reply> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (secret) {
    headers['X-Pairing-Secret'] = secret;
  }
  const response = await fetch(`${serverURL}/api/v1/${route}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  return { status: response.status, body: (await response.json()) as Record<string, unknown> };
}

function failure(reply: Reply) {
  const error = reply.body.error as { message?: string } | undefined;
  return error?.message ?? `respuesta ${reply.status}`;
}

// Pide el código: solicitud con el compromiso, número del servidor y revelación del número propio.
export async function requestPairingCode(name: string, os = 'Windows'): Promise<PairingSession> {
  const nonce = randomBytes(32);
  const created = await pairingCall('POST', 'pairing/requests', undefined, {
    name,
    device_kind: 'desktop',
    os,
    app_version: '0.10.0',
    commitment: createHash('sha256').update(nonce).digest('hex'),
  });
  if (created.status !== 200) {
    throw new Error(failure(created));
  }
  const requestID = String(created.body.request_id);
  const secret = String(created.body.pairing_secret);
  const revealed = await pairingCall('POST', `pairing/requests/${requestID}/reveal`, secret, {
    terminal_nonce: nonce.toString('hex'),
  });
  if (revealed.status !== 200) {
    throw new Error(failure(revealed));
  }
  return {
    requestID,
    secret,
    serverID: String(created.body.server_id),
    serverName: String(created.body.server_name),
    name: String(created.body.name),
    expiresAt: String(created.body.expires_at),
    code: pairingCode(
      serverFingerprint(),
      nonce,
      Buffer.from(String(created.body.server_nonce), 'hex'),
      requestID,
    ),
  };
}

export async function pairingState(session: PairingSession) {
  const reply = await pairingCall('GET', `pairing/requests/${session.requestID}`, session.secret);
  if (reply.status !== 200) {
    throw new Error(failure(reply));
  }
  return String(reply.body.state);
}

// Reclama la credencial (una sola vez): device_id y device_token, o el rechazo de Go.
export async function claimPairing(session: PairingSession) {
  return pairingCall('POST', `pairing/requests/${session.requestID}/claim`, session.secret, {});
}

export async function renamePairingRequest(session: PairingSession, name: string) {
  return pairingCall('POST', `pairing/requests/${session.requestID}/name`, session.secret, {
    name,
  });
}

export async function cancelPairingRequest(session: PairingSession) {
  return pairingCall('POST', `pairing/requests/${session.requestID}/cancel`, session.secret, {});
}

// Misma lista cerrada de direcciones que Rust (normalize_address): IPv4 privada o de enlace local, IPv6
// única local o de enlace local, o un nombre «.local», con «https://» y puerto opcionales.
export function validPairingAddress(input: string) {
  const text = input.trim().replace(/^https:\/\//i, '');
  if (text === '' || /\s|\/|\?|#|@/.test(text)) {
    return false;
  }
  const bracketed = /^\[([0-9a-f:]+)\](?::\d{1,5})?$/i.exec(text);
  if (bracketed) {
    return /^(fc|fd|fe[89ab])/i.test(bracketed[1]!);
  }
  const host = text.replace(/:\d{1,5}$/, '').toLowerCase();
  const ipv4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(host);
  if (ipv4) {
    const [a, b] = [Number(ipv4[1]), Number(ipv4[2])];
    return (
      a === 10 ||
      (a === 172 && b >= 16 && b <= 31) ||
      (a === 192 && b === 168) ||
      (a === 169 && b === 254)
    );
  }
  return /^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+local$/.test(host) && host !== '.local';
}
