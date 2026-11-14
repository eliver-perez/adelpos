// Package discovery anuncia el servidor del restaurante en la red local (etapa 9G, fase C, §4.1).
//
// Las terminales encuentran solas la computadora principal: el servidor se anuncia por mDNS/DNS-SD como un
// servicio `_adelpos._tcp` con un nombre de instancia legible («Fonda Doña Ñeli — Mac de caja»: el nombre del
// establecimiento y el de la computadora) y un registro TXT con su identificador (`server_id`), la versión de
// la API, el puerto HTTPS de la escucha de red y una huella corta del certificado. La terminal (Rust,
// discovery.rs) hace una consulta de una sola vez, elige por `server_id` y comprueba la huella completa al
// conectarse: el anuncio solo sirve para encontrar la dirección, nunca para confiar (un nombre o un TXT iguales
// con otra huella no se aceptan).
//
// Implementación (decisión de la fase C): el anuncio lo hace el respondedor mDNS **del sistema**, no uno propio:
//
//   - macOS: Bonjour (mDNSResponder) por medio de `/usr/bin/dns-sd -R` como proceso hijo (announce_darwin.go).
//     Un respondedor propio tendría que abrir otro socket en el puerto 5353 junto a mDNSResponder; con
//     SO_REUSEPORT los datagramas unicast a ese puerto llegan a un solo socket y podría quitarle respuestas al
//     sistema. Con el del sistema no hay segundo socket, y el sistema resuelve los nombres repetidos, varias
//     interfaces, IPv6, el cambio de IP de la propia computadora y la despedida al retirarse.
//   - Windows (Fase W): el respondedor mDNS del sistema de Windows 10 y posteriores con la API DNS-SD de
//     dnsapi.dll (DnsServiceRegister / DnsServiceDeRegister), detrás de la misma función platformRegister.
//     Hoy announce_other.go responde «no disponible» y el servidor sigue funcionando (las terminales escriben
//     la dirección a mano).
//
// El anuncio solo existe mientras la escucha de red está activa (modalidad con servidor) y nunca en el modo de
// pruebas, salvo en la prueba acotada de este paquete con un nombre de instancia único y unos segundos de vida.
package discovery

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// ServiceType es el tipo de servicio DNS-SD del servidor (también en NSBonjourServices del Info.plist de
	// la aplicación y en discovery.rs).
	ServiceType = "_adelpos._tcp"
	// Domain es el dominio de mDNS.
	Domain = "local"
	// APIVersion es la versión de la API que anuncia el TXT.
	APIVersion = "v1"
	// maxLabel es el largo máximo de una etiqueta DNS (el nombre de instancia completo), en bytes UTF-8.
	maxLabel = 63
	// maxComputer es lo que se reserva para el nombre de la computadora cuando el nombre completo no cabe.
	maxComputer = 24
	// separator une el nombre del establecimiento y el de la computadora.
	separator = " — "
	// fallbackName se usa si no hay establecimiento ni nombre de computadora.
	fallbackName = "AD-EL POS"
)

// errUnsupported indica que el sistema todavía no tiene implementación del anuncio (Windows hasta la Fase W).
var errUnsupported = errors.New("anuncio en la red local no disponible en este sistema")

// Announcement es lo que se publica: nombre de instancia, puerto de la escucha HTTPS y registros TXT.
type Announcement struct {
	Instance string
	Port     int
	TXT      []string
}

func (announcement Announcement) equal(other Announcement) bool {
	return announcement.Instance == other.Instance &&
		announcement.Port == other.Port &&
		slices.Equal(announcement.TXT, other.TXT)
}

// cleanName quita caracteres de control, junta los espacios y quita los puntos. Los puntos son válidos en una
// instancia DNS-SD, pero varios clientes (incluida la biblioteca dnsmessage de Go) rechazan una etiqueta con
// puntos; «S.A.» queda «SA».
func cleanName(text string) string {
	var builder strings.Builder
	for _, character := range text {
		switch {
		case character == '.':
			continue
		case unicode.IsControl(character) || character == utf8.RuneError:
			builder.WriteRune(' ')
		default:
			builder.WriteRune(character)
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

// truncate corta un texto a un máximo de bytes UTF-8 sin partir un carácter y sin dejar espacios al final.
func truncate(text string, maximum int) string {
	if len(text) <= maximum {
		return text
	}
	cut := maximum
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return strings.TrimRight(text[:cut], " -—")
}

// InstanceName arma el nombre legible del anuncio: «establecimiento — computadora». El nombre completo cabe en
// una etiqueta DNS (63 bytes UTF-8): si no cabe, el de la computadora se acorta a 24 bytes y el del
// establecimiento ocupa el resto, sin partir ningún carácter.
func InstanceName(establishment, computer string) string {
	establishment = cleanName(establishment)
	computer = cleanName(computer)
	switch {
	case establishment == "" && computer == "":
		return fallbackName
	case establishment == "":
		return truncate(computer, maxLabel)
	case computer == "" || strings.EqualFold(establishment, computer):
		return truncate(establishment, maxLabel)
	}
	full := establishment + separator + computer
	if len(full) <= maxLabel {
		return full
	}
	computer = truncate(computer, maxComputer)
	establishment = truncate(establishment, maxLabel-len(separator)-len(computer))
	if establishment == "" {
		return truncate(computer, maxLabel)
	}
	return establishment + separator + computer
}

// ShortFingerprint son los primeros 8 bytes de la huella SPKI en hexadecimal (16 caracteres). La terminal la
// usa solo para descartar pronto un anuncio que no corresponde; la comprobación de verdad es la huella
// completa en el intercambio TLS.
func ShortFingerprint(fingerprint [32]byte) string {
	return hex.EncodeToString(fingerprint[:8])
}

// TXTRecords son los registros TXT del anuncio (cada uno «clave=valor», muy por debajo de los 255 bytes de un
// registro): versión del formato, identificador del servidor, versión de la API, puerto HTTPS y huella corta.
func TXTRecords(serverID string, fingerprint [32]byte, port int) []string {
	return []string{
		"txtvers=1",
		"server_id=" + serverID,
		"api=" + APIVersion,
		"port=" + strconv.Itoa(port),
		"fp=" + ShortFingerprint(fingerprint),
	}
}

// Source calcula el anuncio vigente (el nombre del establecimiento puede cambiar mientras el servidor corre).
type Source func(ctx context.Context) (Announcement, error)

// registration es una publicación en marcha.
type registration interface {
	// Done se cierra si la publicación terminó por su cuenta (por ejemplo, se cerró el proceso del sistema).
	Done() <-chan struct{}
	// Stop retira la publicación (el respondedor del sistema envía la despedida) y espera a que termine.
	Stop()
}

// registrar abre una publicación; platformRegister es la de cada sistema (announce_*.go).
type registrar func(ctx context.Context, logger *slog.Logger, announcement Announcement) (registration, error)

// Announcer mantiene el anuncio mientras corre: lo vuelve a publicar si cambia el nombre o si la publicación
// del sistema termina sola (con espera creciente) y lo retira al terminar el contexto.
type Announcer struct {
	Source Source
	Logger *slog.Logger
	// Interval es cada cuánto se revisa si cambió el anuncio (60 s por omisión).
	Interval time.Duration
	// register sustituye a platformRegister en las pruebas y retryBase acorta la espera de los reintentos.
	register  registrar
	retryBase time.Duration
}

const (
	defaultInterval = time.Minute
	firstRetry      = 5 * time.Second
	maxRetry        = time.Minute
)

// Run publica el anuncio hasta que termina el contexto. Si el sistema no tiene implementación, lo informa una
// vez y termina sin error: el servidor sigue atendiendo y las terminales pueden escribir la dirección.
func (announcer *Announcer) Run(ctx context.Context) {
	logger := announcer.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	register := announcer.register
	if register == nil {
		register = platformRegister
	}
	interval := announcer.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	base := announcer.retryBase
	if base <= 0 {
		base = firstRetry
	}
	var current registration
	var running Announcement
	retry := base
	stop := func() {
		if current != nil {
			current.Stop()
			current = nil
			logger.Info("discovery_withdrawn", "instance", running.Instance)
		}
	}
	defer stop()
	for {
		wait := interval
		desired, err := announcer.Source(ctx)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			// Sin el nombre del establecimiento (base no disponible) se conserva el anuncio que haya.
			logger.Warn("discovery_source_failed", "reason", err.Error())
			if current == nil {
				wait = retry
				retry = min(retry*2, maxRetry)
			}
		case current == nil || !running.equal(desired):
			stop()
			started, err := register(ctx, logger, desired)
			if errors.Is(err, errUnsupported) {
				logger.Info("discovery_unavailable", "reason", err.Error())
				return
			}
			if err != nil {
				logger.Warn("discovery_failed", "reason", err.Error())
				wait = retry
				retry = min(retry*2, maxRetry)
				break
			}
			current, running = started, desired
			retry = base
			logger.Info("discovery_started", "instance", desired.Instance, "port", desired.Port)
		}
		var done <-chan struct{}
		if current != nil {
			done = current.Done()
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-done:
			timer.Stop()
			// La publicación del sistema terminó sola: se vuelve a publicar después de una espera creciente.
			logger.Warn("discovery_stopped", "instance", running.Instance)
			current.Stop()
			current = nil
			pause := time.NewTimer(retry)
			retry = min(retry*2, maxRetry)
			select {
			case <-ctx.Done():
				pause.Stop()
				return
			case <-pause.C:
			}
		case <-timer.C:
		}
	}
}
