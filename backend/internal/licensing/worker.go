package licensing

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"time"
)

// Consulta en segundo plano (§3 de la especificación y §8.5 del contrato: «al arrancar y una vez al día» como valor
// orientativo). No es obligatoria para operar y nunca bloquea nada: corre en su propio trabajador, y en la bitácora
// solo deja códigos.
//
//   - La primera, entre 1 y 5 minutos después de arrancar (al azar, para que muchas computadoras que arrancan a la vez
//     no consulten juntas).
//   - Después, cada 24 h ± 1 h.
//   - Si falla (sin internet, TEMPORARY_UNAVAILABLE, RATE_LIMITED…), otra vez en 1 hora, o en lo que pida Retry-After
//     si es más.
//   - Solo con una activación instalada que no sea un cierre: una licencia revocada ya no cambia en el servidor, y su
//     consulta la hace el usuario con «Buscar actualización». Tampoco con la instalación copiada o sin huella.

// Schedule son las esperas del trabajador. Las pruebas usan unas cortas.
type Schedule struct {
	First   func() time.Duration
	Next    func() time.Duration
	Failure time.Duration
}

// DefaultSchedule es el calendario de la especificación.
func DefaultSchedule() Schedule {
	return Schedule{
		First: func() time.Duration {
			return time.Minute + randomDuration(4*time.Minute)
		},
		Next: func() time.Duration {
			return 23*time.Hour + randomDuration(2*time.Hour)
		},
		Failure: time.Hour,
	}
}

// randomDuration es una duración al azar entre 0 y limit.
func randomDuration(limit time.Duration) time.Duration {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return limit / 2
	}
	return time.Duration(binary.BigEndian.Uint64(raw[:]) % uint64(limit))
}

// RunBackground consulta la licencia en segundo plano hasta que se cancele el contexto (el apagado).
func (client *Client) RunBackground(ctx context.Context, schedule Schedule) {
	wait := schedule.First()
	for {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		wait = schedule.Next()
		if !client.shouldRefresh() {
			continue
		}
		_, err := client.Refresh(ctx)
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		wait = schedule.Failure
		var serverErr *ServerError
		if errors.As(err, &serverErr) && serverErr.RetryAfter > wait {
			wait = serverErr.RetryAfter
		}
	}
}

// shouldRefresh: hay una activación instalada, que no es un cierre, y se puede firmar con la identidad.
func (client *Client) shouldRefresh() bool {
	if client.problem != nil {
		return false
	}
	if _, ok := client.manager.currentActivation(); !ok {
		return false
	}
	if _, _, err := client.manager.signingIdentity(); err != nil {
		return false
	}
	return client.manager.Status().State != StateRevoked
}
