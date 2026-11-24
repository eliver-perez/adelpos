import { useState } from 'react';
import { IconLabel } from '../../components/Icon';
import { licenseNotices } from '../../services/license';
import type { LicenseNotice, LicenseSummary } from '../../services/license';

// Avisos de la licencia y de la hora (etapa L1, Fase E; §7 y §6.2 de la especificación). Go decide el estado y los
// días; aquí solo se elige cómo se presenta cada aviso:
//
// - fijos (no se cierran mientras dure la causa): la hora incorrecta, sin activar, vencida y revocada;
// - diarios (se cierran y vuelven al día siguiente): la tolerancia, para todos, y los 7 días antes del vencimiento de
//   una suscripción o una prueba, para quien tiene «Administrar la licencia»;
// - un indicador discreto para los demás en esos 7 días.
//
// El cierre de un aviso diario se recuerda en esta computadora por usuario y por día (localStorage); si el almacén no
// está disponible, el aviso simplemente vuelve a aparecer.

function today(timezone: string) {
  try {
    return new Intl.DateTimeFormat('en-CA', { timeZone: timezone || undefined }).format(new Date());
  } catch {
    return new Date().toISOString().slice(0, 10);
  }
}

function dismissalKey(kind: string, userID: string, timezone: string) {
  return `adel-license-notice:${kind}:${userID}:${today(timezone)}`;
}

function dismissedToday(kind: string, userID: string, timezone: string) {
  try {
    return window.localStorage.getItem(dismissalKey(kind, userID, timezone)) === '1';
  } catch {
    return false;
  }
}

function rememberDismissal(kind: string, userID: string, timezone: string) {
  try {
    window.localStorage.setItem(dismissalKey(kind, userID, timezone), '1');
  } catch {
    // Sin almacén, el aviso vuelve a aparecer: no se pierde nada.
  }
}

export function LicenseNotices({
  license,
  userID,
  timezone,
  canManage,
  mainComputer,
  screen,
  compact = false,
  onOpenLicense,
  onOpenClock,
}: {
  license: LicenseSummary;
  userID: string;
  timezone: string;
  canManage: boolean;
  mainComputer: boolean;
  // Pantalla visible: en la de la licencia o en la de la fecha y hora no se repite su propio enlace.
  screen: string;
  // En la captura, en un renglón.
  compact?: boolean;
  onOpenLicense: () => void;
  onOpenClock: () => void;
}) {
  const [dismissed, setDismissed] = useState<string[]>([]);
  const notices = licenseNotices(license, { canManage, mainComputer }).filter(
    (notice) =>
      notice.presentation !== 'daily' ||
      (!dismissed.includes(notice.kind) && !dismissedToday(notice.kind, userID, timezone)),
  );
  // La pantalla de la fecha y hora ya explica todo: ahí no se repite su aviso.
  const visible = notices.filter((notice) => !(notice.kind === 'clock' && screen === 'clock'));
  if (visible.length === 0) {
    return null;
  }
  function dismiss(notice: LicenseNotice) {
    rememberDismissal(notice.kind, userID, timezone);
    setDismissed((current) => [...current, notice.kind]);
  }
  return (
    <div className={`license-notices ${compact ? 'license-notices-compact' : ''}`}>
      {visible.map((notice) =>
        notice.presentation === 'indicator' ? (
          <p key={notice.kind} className="license-indicator" role="status">
            {notice.title}.
          </p>
        ) : (
          <div
            key={notice.kind}
            className={`license-notice license-notice-${notice.tone}`}
            role={notice.presentation === 'fixed' ? 'alert' : 'status'}
            aria-label={
              notice.kind === 'clock' ? 'Aviso de la fecha y hora' : 'Aviso de la licencia'
            }
          >
            <div>
              <strong>{notice.title}</strong>
              {notice.description && !compact && <p>{notice.description}</p>}
            </div>
            <div className="button-row">
              {notice.kind === 'clock' && mainComputer && (
                <button type="button" className="secondary-button" onClick={onOpenClock}>
                  <IconLabel icon="clock">Ver cómo resolverlo</IconLabel>
                </button>
              )}
              {notice.kind !== 'clock' && screen !== 'license' && (
                <button type="button" className="secondary-button" onClick={onOpenLicense}>
                  <IconLabel icon="key">
                    {notice.kind === 'not_activated' && canManage ? 'Activar' : 'Ver la licencia'}
                  </IconLabel>
                </button>
              )}
              {notice.presentation === 'daily' && (
                <button type="button" className="secondary-button" onClick={() => dismiss(notice)}>
                  Entendido
                </button>
              )}
            </div>
          </div>
        ),
      )}
    </div>
  );
}
