//go:build !darwin

package discovery

import (
	"context"
	"log/slog"
)

// platformRegister todavía no existe fuera de macOS. En Windows (Fase W) se implementará en announce_windows.go
// con el respondedor mDNS del sistema (Windows 10 y posteriores): DnsServiceRegister y DnsServiceDeRegister de
// dnsapi.dll con DNS_SERVICE_INSTANCE (nombre «instancia._adelpos._tcp.local», puerto y los mismos TXTRecords),
// sin abrir un socket propio en el puerto 5353. Mientras tanto el servidor funciona igual y las terminales
// escriben la dirección a mano.
func platformRegister(context.Context, *slog.Logger, Announcement) (registration, error) {
	return nil, errUnsupported
}
