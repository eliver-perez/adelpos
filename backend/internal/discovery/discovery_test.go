package discovery

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestInstanceNameKeepsAccentsAndFitsALabel(t *testing.T) {
	cases := []struct {
		establishment string
		computer      string
		expected      string
	}{
		{"Fonda Doña Ñeli", "Mac de caja", "Fonda Doña Ñeli — Mac de caja"},
		{"  Fonda   Doña\tÑeli ", "Mac de caja\n", "Fonda Doña Ñeli — Mac de caja"},
		{"Tacos El Güero S.A.", "Caja", "Tacos El Güero SA — Caja"},
		{"", "Mac de caja", "Mac de caja"},
		{"Fonda Doña Ñeli", "", "Fonda Doña Ñeli"},
		{"Caja", "caja", "Caja"},
		{"", "", "AD-EL POS"},
		{"\u0007", " ", "AD-EL POS"},
	}
	for _, item := range cases {
		if got := InstanceName(item.establishment, item.computer); got != item.expected {
			t.Fatalf("InstanceName(%q, %q) = %q; se esperaba %q", item.establishment, item.computer, got, item.expected)
		}
	}
	// Nombres largos con caracteres de dos bytes: caben en 63 bytes sin partir ningún carácter y conservan la
	// computadora acortada.
	long := InstanceName(strings.Repeat("Ñ", 60), "Computadora de la caja principal del salón")
	if len(long) > 63 || !utf8.ValidString(long) {
		t.Fatalf("nombre largo inválido: %q (%d bytes)", long, len(long))
	}
	if !strings.HasPrefix(long, "ÑÑÑ") || !strings.Contains(long, " — Computadora de la") {
		t.Fatalf("nombre largo sin las dos partes: %q", long)
	}
	onlyComputer := InstanceName("", strings.Repeat("ó", 40))
	if len(onlyComputer) > 63 || !utf8.ValidString(onlyComputer) || onlyComputer != strings.Repeat("ó", 31) {
		t.Fatalf("computadora larga: %q (%d bytes)", onlyComputer, len(onlyComputer))
	}
}

func TestTXTRecordsCarryIdentityAndShortFingerprint(t *testing.T) {
	var fingerprint [32]byte
	for index := range fingerprint {
		fingerprint[index] = byte(index * 7)
	}
	records := TXTRecords("11111111-2222-4333-8444-555555555555", fingerprint, 8443)
	expected := []string{
		"txtvers=1",
		"server_id=11111111-2222-4333-8444-555555555555",
		"api=v1",
		"port=8443",
		"fp=00070e151c232a31",
	}
	if strings.Join(records, "|") != strings.Join(expected, "|") {
		t.Fatalf("TXT: %v", records)
	}
	for _, record := range records {
		key, _, found := strings.Cut(record, "=")
		if !found || key == "" || len(key) > 9 || len(record) > 255 {
			t.Fatalf("registro TXT inválido: %q", record)
		}
	}
}

// fakeRegistration simula la publicación del sistema.
type fakeRegistration struct {
	announcement Announcement
	done         chan struct{}
	stopped      chan struct{}
	once         sync.Once
}

func (registration *fakeRegistration) Done() <-chan struct{} {
	return registration.done
}

func (registration *fakeRegistration) Stop() {
	registration.once.Do(func() {
		close(registration.stopped)
	})
}

type fakeSystem struct {
	mu            sync.Mutex
	registrations []*fakeRegistration
	failures      int
	unsupported   bool
	started       chan *fakeRegistration
}

func (system *fakeSystem) register(_ context.Context, _ *slog.Logger, announcement Announcement) (registration, error) {
	system.mu.Lock()
	defer system.mu.Unlock()
	if system.unsupported {
		return nil, errUnsupported
	}
	if system.failures > 0 {
		system.failures--
		return nil, errors.New("dns-sd no arrancó")
	}
	created := &fakeRegistration{announcement: announcement, done: make(chan struct{}), stopped: make(chan struct{})}
	system.registrations = append(system.registrations, created)
	system.started <- created
	return created, nil
}

func waitRegistration(t *testing.T, started chan *fakeRegistration) *fakeRegistration {
	t.Helper()
	select {
	case created := <-started:
		return created
	case <-time.After(5 * time.Second):
		t.Fatal("no se publicó el anuncio")
		return nil
	}
}

func waitStopped(t *testing.T, registration *fakeRegistration) {
	t.Helper()
	select {
	case <-registration.stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("no se retiró el anuncio")
	}
}

func TestAnnouncerRepublishesOnChangesAndWithdrawsOnShutdown(t *testing.T) {
	system := &fakeSystem{started: make(chan *fakeRegistration, 4)}
	var mu sync.Mutex
	name := "Fonda Doña Ñeli — Mac de caja"
	source := func(context.Context) (Announcement, error) {
		mu.Lock()
		defer mu.Unlock()
		return Announcement{Instance: name, Port: 8443, TXT: []string{"txtvers=1"}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	announcer := &Announcer{Source: source, Interval: 20 * time.Millisecond, register: system.register}
	go func() {
		announcer.Run(ctx)
		close(finished)
	}()
	first := waitRegistration(t, system.started)
	if first.announcement.Instance != name {
		t.Fatalf("primer anuncio: %+v", first.announcement)
	}
	// Cambia el nombre del establecimiento: se retira el anterior y se publica el nuevo.
	mu.Lock()
	name = "Fonda Ñeli — Mac de caja"
	mu.Unlock()
	second := waitRegistration(t, system.started)
	waitStopped(t, first)
	if second.announcement.Instance != "Fonda Ñeli — Mac de caja" {
		t.Fatalf("segundo anuncio: %+v", second.announcement)
	}
	// Al apagar el servidor el anuncio se retira y Run termina.
	cancel()
	waitStopped(t, second)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("Run no terminó al apagar")
	}
}

func TestAnnouncerRetriesWhenTheSystemPublicationEnds(t *testing.T) {
	system := &fakeSystem{started: make(chan *fakeRegistration, 4), failures: 1}
	source := func(context.Context) (Announcement, error) {
		return Announcement{Instance: "Caja", Port: 8443}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	announcer := &Announcer{Source: source, Interval: time.Hour, register: system.register, retryBase: 10 * time.Millisecond}
	done := make(chan struct{})
	go func() {
		announcer.Run(ctx)
		close(done)
	}()
	// El primer intento falla y se reintenta; la publicación termina sola y se vuelve a publicar.
	first := waitRegistration(t, system.started)
	close(first.done)
	waitStopped(t, first)
	second := waitRegistration(t, system.started)
	cancel()
	waitStopped(t, second)
	<-done
}

func TestAnnouncerStopsQuietlyWhereTheSystemHasNoImplementation(t *testing.T) {
	system := &fakeSystem{started: make(chan *fakeRegistration, 1), unsupported: true}
	announcer := &Announcer{
		Source: func(context.Context) (Announcement, error) {
			return Announcement{Instance: "Caja", Port: 8443}, nil
		},
		register: system.register,
	}
	done := make(chan struct{})
	go func() {
		announcer.Run(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run debía terminar sin implementación del sistema")
	}
}
