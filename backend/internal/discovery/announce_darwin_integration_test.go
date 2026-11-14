//go:build integration && darwin

package discovery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Prueba acotada del anuncio real (etapa 9G, fase C): publica con Bonjour del sistema un servidor de prueba con
// un nombre de instancia único y unos segundos de vida, lo busca con la consulta de una sola vez desde un
// puerto efímero (como la terminal) y comprueba que al terminar se retira y no queda ningún dns-sd. Usa el tipo
// real `_adelpos._tcp`: una terminal que buscara en esos segundos vería «AD-EL POS prueba …», que nunca
// responde (puerto 8201 sin escucha).
func TestAnnounceAndFindOnTheLocalNetwork(t *testing.T) {
	var raw [6]byte
	_, _ = rand.Read(raw[:])
	tag := hex.EncodeToString(raw[:])
	instance := InstanceName("AD-EL POS prueba "+tag, "Ñeli")
	serverID := fmt.Sprintf("%s-0000-4000-8000-%s", tag[:8], tag)
	var fingerprint [32]byte
	copy(fingerprint[:], raw[:])
	announcement := Announcement{Instance: instance, Port: 8201, TXT: TXTRecords(serverID, fingerprint, 8201)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan struct{})
	announcer := &Announcer{
		Source: func(context.Context) (Announcement, error) {
			return announcement, nil
		},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Interval: time.Hour,
	}
	go func() {
		announcer.Run(ctx)
		close(finished)
	}()
	find := func() *found {
		servers, err := lookup(context.Background(), ServiceType, 1200*time.Millisecond)
		if err != nil {
			t.Fatalf("consulta mDNS: %v", err)
		}
		for index := range servers {
			if servers[index].Instance == instance {
				return &servers[index]
			}
		}
		return nil
	}
	var server *found
	for deadline := time.Now().Add(10 * time.Second); server == nil && time.Now().Before(deadline); {
		server = find()
	}
	if server == nil {
		t.Fatalf("no se encontró el anuncio %q en la red local", instance)
	}
	if server.Port != 8201 || server.TXT["server_id"] != serverID || server.TXT["fp"] != ShortFingerprint(fingerprint) ||
		server.TXT["api"] != "v1" || server.TXT["port"] != "8201" || server.TXT["txtvers"] != "1" {
		t.Fatalf("anuncio con datos distintos: %+v", server)
	}
	private := false
	for _, address := range server.Addresses {
		if address.Is4() && (address.IsPrivate() || address.IsLinkLocalUnicast()) {
			private = true
		}
	}
	if !private {
		t.Fatalf("el anuncio no trae una dirección IPv4 de la red local: %v", server.Addresses)
	}
	t.Logf("anuncio encontrado: %q en el puerto %d con %d direcciones IPv4", server.Instance, server.Port, len(server.Addresses))
	// Al apagar: el anuncio se retira y dns-sd termina.
	cancel()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("el anuncio no se retiró a tiempo")
	}
	gone := false
	for deadline := time.Now().Add(6 * time.Second); !gone && time.Now().Before(deadline); {
		gone = find() == nil
	}
	if !gone {
		t.Fatal("el anuncio sigue visible después de retirarlo")
	}
	if output, err := exec.Command("/usr/bin/pgrep", "-f", tag).Output(); err == nil && len(strings.TrimSpace(string(output))) > 0 {
		t.Fatalf("quedó un dns-sd del anuncio de prueba: %s", output)
	}
}

// TestHoldAnnouncementForLiveSearch mantiene un anuncio de prueba unos segundos para la búsqueda real de Rust
// (discovery.rs, live_search_finds_a_test_announcement; dtools/live-discovery-C.sh). Solo corre con
// ADEL_DISCOVERY_HOLD_SERVER_ID (el identificador del anuncio); dura como máximo 12 s.
func TestHoldAnnouncementForLiveSearch(t *testing.T) {
	serverID := os.Getenv("ADEL_DISCOVERY_HOLD_SERVER_ID")
	if serverID == "" {
		t.Skip("solo para la búsqueda real de Rust")
	}
	var fingerprint [32]byte
	announcement := Announcement{
		Instance: InstanceName("AD-EL POS prueba Rust "+serverID[:8], "Ñeli"),
		Port:     8202,
		TXT:      TXTRecords(serverID, fingerprint, 8202),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	announcer := &Announcer{
		Source: func(context.Context) (Announcement, error) {
			return announcement, nil
		},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Interval: time.Hour,
	}
	announcer.Run(ctx)
}
