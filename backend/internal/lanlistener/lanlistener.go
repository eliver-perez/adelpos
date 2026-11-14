// Package lanlistener es la escucha HTTPS del servidor en la red local (etapa 9G, fase B; decisión D1).
//
// El servidor conserva su escucha HTTP en 127.0.0.1:8181 para la ventana de la propia computadora y, con
// una modalidad con servidor, agrega esta escucha HTTPS en todas las interfaces (puerto 8443 por omisión)
// para las demás terminales:
//
//   - Solo acepta conexiones de loopback o de un equipo de la misma red local: la dirección de origen debe ser
//     privada (RFC 1918), de enlace local o IPv6 única local (ULA) **y** estar dentro de la subred de una interfaz
//     activa de esta computadora que no sea punto a punto ni túnel (revisión independiente, hallazgo 5: las VPN
//     como Tailscale o ZeroTier reparten direcciones ULA o privadas a equipos fuera del restaurante). La lista de
//     interfaces se vuelve a leer cada 30 s y, si un origen no coincide, como mucho cada 2 s (las direcciones
//     cambian por DHCP). Las demás conexiones se cierran al aceptarlas, antes del intercambio TLS. Riesgo que
//     queda: una VPN que se presenta como una interfaz de red normal (no punto a punto) con una subred privada.
//   - TLS 1.2 o superior con el certificado propio del servidor (localsetup.EnsureIdentity); las terminales
//     fijan su huella durante el emparejamiento.
//   - Toda petición que llega por aquí queda marcada como «de la red» (FromNetwork): las rutas locales
//     (asistente inicial, aprovisionamiento, aprobación de emparejamiento) nunca se aceptan por esta escucha,
//     aunque la conexión venga de la propia computadora (127.0.0.1 también es un origen permitido aquí). Así
//     la condición «local» depende de la escucha y no solo de la IP de origen.
//   - Los eventos en tiempo real (SSE) usan la misma conexión cifrada.
package lanlistener

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// DefaultAddress es la escucha de red del servicio: todas las interfaces, IPv4 e IPv6, puerto 8443.
const DefaultAddress = ":8443"

type networkKey struct{}

// FromNetwork indica si la petición llegó por la escucha de red. El valor lo pone el propio servidor de
// esta escucha en el contexto de la petición: el cliente no puede falsificarlo con cabeceras.
func FromNetwork(request *http.Request) bool {
	marked, _ := request.Context().Value(networkKey{}).(bool)
	return marked
}

// WithNetwork marca un contexto como «llegó por la escucha de red». Lo usa la propia escucha (Listen) y las
// pruebas de las rutas que se comportan distinto por la red (etapa 9G, fase C: versión del cliente obligatoria).
func WithNetwork(ctx context.Context) context.Context {
	return context.WithValue(ctx, networkKey{}, true)
}

// AllowedPeer indica si la clase de una dirección de origen es aceptable: loopback, privada (RFC 1918 y ULA
// fc00::/7) o de enlace local (169.254.0.0/16 y fe80::/10). Las direcciones IPv4 mapeadas en IPv6 se
// evalúan como IPv4. Todo lo demás (Internet, CGNAT 100.64.0.0/10, multidifusión, sin especificar) no. Además
// de la clase, la escucha exige que el origen esté en la subred de una interfaz local (peerFilter).
func AllowedPeer(address net.IP) bool {
	if address == nil {
		return false
	}
	if v4 := address.To4(); v4 != nil {
		address = v4
	}
	if address.IsUnspecified() || address.IsMulticast() {
		return false
	}
	return address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast()
}

// LocalNetwork es una red de una interfaz activa de esta computadora.
type LocalNetwork struct {
	Network      *net.IPNet
	PointToPoint bool
}

// SystemNetworks enumera las redes de las interfaces activas que no son loopback.
func SystemNetworks() ([]LocalNetwork, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	networks := []LocalNetwork{}
	for _, item := range interfaces {
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := item.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			if network, ok := address.(*net.IPNet); ok {
				networks = append(networks, LocalNetwork{Network: network, PointToPoint: item.Flags&net.FlagPointToPoint != 0})
			}
		}
	}
	return networks, nil
}

const (
	networksRefresh = 30 * time.Second
	networksRetry   = 2 * time.Second
)

// peerFilter decide si un origen pertenece a la red local: loopback, o clase aceptable (AllowedPeer) dentro de la
// subred de una interfaz que no sea punto a punto.
type peerFilter struct {
	mutex    sync.Mutex
	source   func() ([]LocalNetwork, error)
	now      func() time.Time
	networks []LocalNetwork
	loaded   time.Time
}

func newPeerFilter(source func() ([]LocalNetwork, error)) *peerFilter {
	return &peerFilter{source: source, now: time.Now}
}

func (filter *peerFilter) refresh(now time.Time) {
	filter.loaded = now
	networks, err := filter.source()
	if err == nil {
		filter.networks = networks
	}
}

func (filter *peerFilter) onLink(address net.IP) bool {
	for _, network := range filter.networks {
		if network.PointToPoint || network.Network == nil {
			continue
		}
		if network.Network.Contains(address) {
			return true
		}
	}
	return false
}

func (filter *peerFilter) allowed(address net.IP) bool {
	if address == nil {
		return false
	}
	if v4 := address.To4(); v4 != nil {
		address = v4
	}
	if address.IsLoopback() {
		return true
	}
	if !AllowedPeer(address) {
		return false
	}
	filter.mutex.Lock()
	defer filter.mutex.Unlock()
	now := filter.now()
	if filter.loaded.IsZero() || now.Sub(filter.loaded) >= networksRefresh {
		filter.refresh(now)
	}
	if filter.onLink(address) {
		return true
	}
	// La dirección de la computadora pudo cambiar (DHCP): se vuelve a leer, como mucho cada 2 s.
	if now.Sub(filter.loaded) >= networksRetry {
		filter.refresh(now)
		return filter.onLink(address)
	}
	return false
}

// peerAllowed lee la IP de la dirección remota de una conexión TCP.
func (filter *peerFilter) peerAllowed(remote net.Addr) bool {
	switch value := remote.(type) {
	case *net.TCPAddr:
		return filter.allowed(value.IP)
	default:
		host, _, err := net.SplitHostPort(remote.String())
		if err != nil {
			return false
		}
		return filter.allowed(net.ParseIP(host))
	}
}

// filteredListener cierra al aceptarlas las conexiones de orígenes no permitidos.
type filteredListener struct {
	net.Listener
	filter *peerFilter
}

func (listener filteredListener) Accept() (net.Conn, error) {
	for {
		connection, err := listener.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if listener.filter.peerAllowed(connection.RemoteAddr()) {
			return connection, nil
		}
		_ = connection.Close()
	}
}

// Filter envuelve una escucha para aceptar solo orígenes de la red local según las interfaces que entrega
// networks (SystemNetworks en el servidor; las pruebas simulan interfaces).
func Filter(listener net.Listener, networks func() ([]LocalNetwork, error)) net.Listener {
	return filteredListener{Listener: listener, filter: newPeerFilter(networks)}
}

// TLSConfig es la configuración TLS de la escucha: TLS 1.2 como mínimo y el certificado del servidor.
func TLSConfig(certificate tls.Certificate) *tls.Config {
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
	}
}

// Server es la escucha HTTPS de red en marcha.
type Server struct {
	http     *http.Server
	listener net.Listener
}

// Listen abre la escucha (filtro de orígenes y TLS) y prepara el servidor HTTP con los mismos plazos que
// la escucha local. handler recibe las peticiones ya marcadas como «de la red».
func Listen(address string, certificate tls.Certificate, handler http.Handler) (*Server, error) {
	raw, err := net.Listen("tcp", address)
	if err != nil {
		return nil, errors.New("no se pudo abrir la escucha segura de la red local; compruebe que el puerto esté libre")
	}
	marked := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		handler.ServeHTTP(writer, request.WithContext(WithNetwork(request.Context())))
	})
	server := &http.Server{
		Handler:           marked,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
		TLSConfig:         TLSConfig(certificate),
		// Los intercambios TLS fallidos (exploradores de puertos, clientes sin la huella) no llenan la
		// bitácora del servicio.
		ErrorLog: log.New(io.Discard, "", 0),
	}
	return &Server{http: server, listener: tls.NewListener(Filter(raw, SystemNetworks), server.TLSConfig)}, nil
}

// Addr es la dirección efectiva (con el puerto elegido si se pidió el 0).
func (server *Server) Addr() net.Addr {
	return server.listener.Addr()
}

// Serve atiende hasta Shutdown; devuelve http.ErrServerClosed al cerrarse en orden.
func (server *Server) Serve() error {
	return server.http.Serve(server.listener)
}

// RegisterOnShutdown agrega una función que corre al empezar el cierre (los flujos SSE terminan).
func (server *Server) RegisterOnShutdown(function func()) {
	server.http.RegisterOnShutdown(function)
}

// Shutdown cierra la escucha en orden.
func (server *Server) Shutdown(ctx context.Context) error {
	return server.http.Shutdown(ctx)
}

// Close cierra la escucha de inmediato.
func (server *Server) Close() error {
	return server.http.Close()
}
