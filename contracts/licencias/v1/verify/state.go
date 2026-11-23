package main

import "time"

// Estados temporales de una licencia válida (contrato, secciones 5.6 y 6).
const (
	StateActive  = "active"  // vigente
	StateGrace   = "grace"   // en tolerancia
	StateExpired = "expired" // vencida
	StateRevoked = "revoked" // revocada, por el motivo que diga «status_reason»
)

// EffectiveNow es el máximo entre el reloj local y la última hora confiable. Atrasar el
// reloj nunca prolonga una licencia.
//
// La última hora confiable es el valor más reciente entre el «server_time» de las respuestas
// del servidor de licencias, el «issued_at» de las licencias aceptadas y la última hora local
// registrada mientras el sistema funcionaba. La instalación la guarda de forma persistente.
func EffectiveNow(localClock time.Time, lastTrusted *time.Time) time.Time {
	if lastTrusted != nil && lastTrusted.After(localClock) {
		return *lastTrusted
	}
	return localClock
}

// TemporalState calcula el estado de una licencia ya verificada.
//
//	perpetua:                                  vigente
//	ahora_efectivo < expires_at:               vigente
//	expires_at <= ahora_efectivo < límite:     tolerancia   (límite = expires_at + grace_days × 86 400 s)
//	ahora_efectivo >= límite:                  vencida
func TemporalState(license *License, localClock time.Time, lastTrusted *time.Time) string {
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
	limit := license.ExpiresAt.Add(time.Duration(license.GraceDays) * 24 * time.Hour)
	if now.Before(limit) {
		return StateGrace
	}
	return StateExpired
}
