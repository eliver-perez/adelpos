package lanlistener

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/localsetup"
)

func TestAllowedPeersAreLoopbackPrivateLinkLocalAndULA(t *testing.T) {
	allowed := []string{
		"127.0.0.1", "127.8.9.10", "::1",
		"10.0.0.5", "172.16.0.1", "172.31.255.254", "192.168.1.20",
		"169.254.10.20", "fe80::1", "fd12:3456:789a::1", "fc00::5",
		"::ffff:192.168.1.20", "::ffff:127.0.0.1",
	}
	for _, text := range allowed {
		if !AllowedPeer(net.ParseIP(text)) {
			t.Errorf("%s debe aceptarse", text)
		}
	}
	rejected := []string{
		"8.8.8.8", "1.1.1.1", "172.32.0.1", "192.169.0.1", "100.64.0.1", "11.0.0.1",
		"2001:db8::1", "2606:4700::1111", "::ffff:8.8.8.8",
		"224.0.0.251", "ff02::fb", "0.0.0.0", "::", "255.255.255.255",
	}
	for _, text := range rejected {
		if AllowedPeer(net.ParseIP(text)) {
			t.Errorf("%s debe rechazarse", text)
		}
	}
	if AllowedPeer(nil) {
		t.Error("sin dirección no se acepta")
	}
}

// fakeConnection es una conexión con la dirección remota elegida por la prueba.
type fakeConnection struct {
	net.Conn
	remote net.Addr
	mutex  sync.Mutex
	closed bool
}

func (connection *fakeConnection) RemoteAddr() net.Addr {
	return connection.remote
}

func (connection *fakeConnection) Close() error {
	connection.mutex.Lock()
	defer connection.mutex.Unlock()
	connection.closed = true
	return nil
}

func (connection *fakeConnection) isClosed() bool {
	connection.mutex.Lock()
	defer connection.mutex.Unlock()
	return connection.closed
}

type fakeListener struct {
	pending []*fakeConnection
}

func (listener *fakeListener) Accept() (net.Conn, error) {
	if len(listener.pending) == 0 {
		return nil, errors.New("sin conexiones")
	}
	next := listener.pending[0]
	listener.pending = listener.pending[1:]
	return next, nil
}

func (listener *fakeListener) Close() error {
	return nil
}

func (listener *fakeListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 8443}
}

func TestFilterClosesForeignOriginsOnAccept(t *testing.T) {
	public := &fakeConnection{remote: &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 4000}}
	carrier := &fakeConnection{remote: &net.TCPAddr{IP: net.ParseIP("100.64.1.2"), Port: 4001}}
	local := &fakeConnection{remote: &net.TCPAddr{IP: net.ParseIP("192.168.1.30"), Port: 4002}}
	filtered := Filter(&fakeListener{pending: []*fakeConnection{public, carrier, local}})
	accepted, err := filtered.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if accepted != local {
		t.Fatal("debe entregar la primera conexión de la red local")
	}
	if !public.isClosed() || !carrier.isClosed() || local.isClosed() {
		t.Fatal("las conexiones de otros orígenes se cierran al aceptarlas")
	}
	if _, err := filtered.Accept(); err == nil {
		t.Fatal("el error de la escucha debe propagarse")
	}
}

// La escucha real: TLS 1.2 como mínimo, el certificado del servidor y las peticiones marcadas como «de la
// red» aunque vengan de 127.0.0.1.
func TestListenServesTLS12WithTheServerCertificateAndMarksRequests(t *testing.T) {
	identity, err := localsetup.EnsureIdentity(filepath.Join(t.TempDir(), localsetup.IdentityFolder))
	if err != nil {
		t.Fatal(err)
	}
	seen := make(chan bool, 4)
	server, err := Listen("127.0.0.1:0", identity.Certificate, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen <- FromNetwork(request)
		_, _ = io.WriteString(writer, "ok")
	}))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- server.Serve()
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		if err := <-done; !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("cierre inesperado: %v", err)
		}
	}()
	address := server.Addr().String()

	old, err := tls.Dial("tcp", address, &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS11})
	if err == nil {
		old.Close()
		t.Fatal("TLS 1.1 no debe aceptarse")
	}

	var presented [32]byte
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			presented = localsetup.SPKIFingerprint(state.PeerCertificates[0])
			return nil
		},
	}}}
	response, err := client.Get("https://" + address + "/api/v1/health/live")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if presented != identity.Fingerprint {
		t.Fatal("la escucha debe presentar el certificado del servidor")
	}
	if marked := <-seen; !marked {
		t.Fatal("la petición por la escucha de red debe marcarse aunque venga de loopback")
	}
	plain, err := http.Get("http://" + address + "/api/v1/health/live")
	if err == nil {
		plain.Body.Close()
		if plain.StatusCode < 400 {
			t.Fatal("la escucha de red no atiende HTTP sin cifrar")
		}
	}
	request, _ := http.NewRequest("GET", "http://127.0.0.1/", nil)
	if FromNetwork(request) {
		t.Fatal("una petición sin marca no es de la red")
	}
}
