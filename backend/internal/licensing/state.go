package licensing

import "time"

// Estado temporal de una licencia ya verificada (contrato §5.6), adaptado de
// `contracts/licencias/v1/verify/state.go`. Es lo único de fechas que calcula AD-EL POS: las fechas las firma
// el servidor de licencias y aquí no se recalculan.

// EffectiveNow es el máximo entre el reloj local y la última hora confiable. Atrasar el reloj nunca
// prolonga una licencia.
//
// La última hora confiable es el valor más reciente entre el «server_time» de las respuestas del servidor
// de licencias, el «issued_at» de las licencias aceptadas y la última hora local registrada mientras el
// sistema funcionaba. El gestor la guarda de forma persistente (store.go).
func EffectiveNow(localClock time.Time, lastTrusted *time.Time) time.Time {
	if lastTrusted != nil && lastTrusted.After(localClock) {
		return *lastTrusted
	}
	return localClock
}

// GraceLimit es el fin exclusivo de la tolerancia: expires_at + grace_days × 86 400 s. Nil en una perpetua.
func GraceLimit(license *License) *time.Time {
	if license.ExpiresAt == nil {
		return nil
	}
	limit := license.ExpiresAt.Add(time.Duration(license.GraceDays) * 24 * time.Hour)
	return &limit
}

// TemporalState calcula el estado de una licencia ya verificada.
//
//	revocada (license_status revoked):         revocada
//	perpetua:                                  vigente
//	ahora_efectivo < expires_at:               vigente
//	expires_at <= ahora_efectivo < límite:     tolerancia   (límite = expires_at + grace_days × 86 400 s)
//	ahora_efectivo >= límite:                  vencida
//
// El estado se calcula solo con expires_at y grace_days: un period_starts_at en el futuro es válido y no lo
// afecta.
func TemporalState(license *License, localClock time.Time, lastTrusted *time.Time) State {
	if license.LicenseStatus != "active" {
		return StateRevoked
	}
	if license.ExpiresAt == nil {
		return StateActive
	}
	now := EffectiveNow(localClock, lastTrusted)
	if now.Before(*license.ExpiresAt) {
		return StateActive
	}
	if now.Before(*GraceLimit(license)) {
		return StateGrace
	}
	return StateExpired
}
