// Package lanlistener es la escucha HTTPS del servidor en la red local (etapa 9G, fase B; decisión D1).
//
// El servidor conserva su escucha HTTP en 127.0.0.1:8181 para la ventana de la propia computadora y, con
// una modalidad con servidor, agrega esta escucha HTTPS en todas las interfaces (puerto 8443 por omisión)
// para las demás terminales:
//
//   - Solo acepta conexiones cuyo origen sea loopback, una red privada (RFC 1918), enlace local o una
//     dirección IPv6 única local (ULA). Las demás se cierran al aceptarlas, antes del intercambio TLS.
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

// AllowedPeer indica si una dirección de origen puede conectarse: loopback, privada (RFC 1918 y ULA
// fc00::/7) o de enlace local (169.254.0.0/16 y fe80::/10). Las direcciones IPv4 mapeadas en IPv6 se
// evalúan como IPv4. Todo lo demás (Internet, CGNAT 100.64.0.0/10, multidifusión, sin especificar) no.
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

// peerAllowed lee la IP de la dirección remota de una conexión TCP.
func peerAllowed(remote net.Addr) bool {
	switch value := remote.(type) {
	case *net.TCPAddr:
		return AllowedPeer(value.IP)
	default:
		host, _, err := net.SplitHostPort(remote.String())
		if err != nil {
			return false
		}
		return AllowedPeer(net.ParseIP(host))
	}
}

// filteredListener cierra al aceptarlas las conexiones de orígenes no permitidos.
type filteredListener struct {
	net.Listener
}

func (listener filteredListener) Accept() (net.Conn, error) {
	for {
		connection, err := listener.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if peerAllowed(connection.RemoteAddr()) {
			return connection, nil
		}
		_ = connection.Close()
	}
}

// Filter envuelve una escucha para aceptar solo orígenes permitidos (se exporta para probarlo).
func Filter(listener net.Listener) net.Listener {
	return filteredListener{Listener: listener}
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
	return &Server{http: server, listener: tls.NewListener(Filter(raw), server.TLSConfig)}, nil
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
