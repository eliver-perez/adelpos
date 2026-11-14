package discovery

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Cliente DNS-SD de las pruebas: una consulta mDNS de una sola vez desde un puerto efímero (RFC 6762 §6.7), como
// la que hace la terminal en Rust (discovery.rs), escrita aparte con dnsmessage para comprobar el anuncio real
// en la red de forma independiente. Solo lo usan las pruebas.

// found es un servidor encontrado: instancia, destino, puerto, TXT y direcciones IPv4.
type found struct {
	Instance  string
	Target    string
	Port      int
	TXT       map[string]string
	Addresses []netip.Addr
}

var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

// serviceName es el nombre completo del tipo de servicio («_adelpos._tcp.local.»).
func serviceName(service string) string {
	return service + "." + Domain + "."
}

// buildQuery arma la consulta PTR del tipo de servicio con el identificador dado.
func buildQuery(id uint16, service string) ([]byte, error) {
	name, err := dnsmessage.NewName(serviceName(service))
	if err != nil {
		return nil, err
	}
	builder := dnsmessage.NewBuilder(make([]byte, 0, 512), dnsmessage.Header{ID: id})
	builder.EnableCompression()
	if err := builder.StartQuestions(); err != nil {
		return nil, err
	}
	if err := builder.Question(dnsmessage.Question{Name: name, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}); err != nil {
		return nil, err
	}
	return builder.Finish()
}

// parseTXT convierte los registros «clave=valor» en un mapa (las claves sin distinguir mayúsculas).
func parseTXT(records []string) map[string]string {
	values := map[string]string{}
	for _, record := range records {
		key, value, _ := strings.Cut(record, "=")
		if key != "" {
			values[strings.ToLower(key)] = value
		}
	}
	return values
}

// firstLabel es la instancia legible: la primera etiqueta del nombre completo.
func firstLabel(name string) string {
	label, _, _ := strings.Cut(name, ".")
	return label
}

// parseResponse lee una respuesta (respuestas y registros adicionales) y agrega lo que encuentre a servers,
// por nombre completo de la instancia. Solo acepta respuestas a la consulta con ese identificador.
func parseResponse(packet []byte, id uint16, service string, servers map[string]*found) error {
	var parser dnsmessage.Parser
	header, err := parser.Start(packet)
	if err != nil {
		return err
	}
	if !header.Response || header.ID != id {
		return errors.New("no es la respuesta a esta consulta")
	}
	if err := parser.SkipAllQuestions(); err != nil {
		return err
	}
	target := strings.ToLower(serviceName(service))
	instances := map[string]*found{}
	byTarget := map[string][]netip.Addr{}
	records := []dnsmessage.Resource{}
	answers, err := parser.AllAnswers()
	if err != nil {
		return err
	}
	records = append(records, answers...)
	if err := parser.SkipAllAuthorities(); err != nil {
		return err
	}
	additionals, err := parser.AllAdditionals()
	if err != nil {
		return err
	}
	records = append(records, additionals...)
	for _, record := range records {
		if body, ok := record.Body.(*dnsmessage.PTRResource); ok && strings.EqualFold(record.Header.Name.String(), target) {
			name := body.PTR.String()
			if instances[name] == nil {
				instances[name] = &found{Instance: firstLabel(name), TXT: map[string]string{}}
			}
		}
	}
	for _, record := range records {
		name := record.Header.Name.String()
		switch body := record.Body.(type) {
		case *dnsmessage.SRVResource:
			if server := instances[name]; server != nil {
				server.Target = body.Target.String()
				server.Port = int(body.Port)
			}
		case *dnsmessage.TXTResource:
			if server := instances[name]; server != nil {
				server.TXT = parseTXT(body.TXT)
			}
		case *dnsmessage.AResource:
			key := strings.ToLower(name)
			byTarget[key] = append(byTarget[key], netip.AddrFrom4(body.A))
		}
	}
	for name, server := range instances {
		server.Addresses = append(server.Addresses, byTarget[strings.ToLower(server.Target)]...)
		if previous := servers[name]; previous != nil && len(server.Addresses) == 0 {
			continue
		}
		servers[name] = server
	}
	return nil
}

// lookup busca el tipo de servicio durante wait: envía la consulta al grupo mDNS (dos veces, por si se pierde
// una) y junta las respuestas unicast que llegan al puerto efímero.
func lookup(ctx context.Context, service string, wait time.Duration) ([]found, error) {
	connection, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	var raw [2]byte
	_, _ = rand.Read(raw[:])
	id := binary.BigEndian.Uint16(raw[:])
	query, err := buildQuery(id, service)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	servers := map[string]*found{}
	buffer := make([]byte, 9000)
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := connection.WriteToUDP(query, mdnsGroup); err != nil {
			return nil, err
		}
		until := time.Now().Add(wait / 2)
		if attempt == 1 {
			until = deadline
		}
		for time.Now().Before(until) && ctx.Err() == nil {
			_ = connection.SetReadDeadline(until)
			length, _, err := connection.ReadFromUDP(buffer)
			if err != nil {
				break
			}
			_ = parseResponse(buffer[:length], id, service, servers)
		}
	}
	result := make([]found, 0, len(servers))
	for _, server := range servers {
		result = append(result, *server)
	}
	return result, nil
}

// buildResponse arma una respuesta como la de mDNSResponder a una consulta de una sola vez: PTR en las
// respuestas y SRV, TXT y A en los adicionales, con compresión de nombres.
func buildResponse(t *testing.T, id uint16, instance, host string, port uint16, txt []string, address [4]byte) []byte {
	t.Helper()
	service := dnsmessage.MustNewName(serviceName(ServiceType))
	full := dnsmessage.MustNewName(instance + "." + serviceName(ServiceType))
	target := dnsmessage.MustNewName(host)
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: id, Response: true, Authoritative: true})
	builder.EnableCompression()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(builder.StartQuestions())
	must(builder.Question(dnsmessage.Question{Name: service, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}))
	must(builder.StartAnswers())
	header := func(name dnsmessage.Name, kind dnsmessage.Type) dnsmessage.ResourceHeader {
		return dnsmessage.ResourceHeader{Name: name, Type: kind, Class: dnsmessage.ClassINET, TTL: 10}
	}
	must(builder.PTRResource(header(service, dnsmessage.TypePTR), dnsmessage.PTRResource{PTR: full}))
	must(builder.StartAdditionals())
	must(builder.SRVResource(header(full, dnsmessage.TypeSRV), dnsmessage.SRVResource{Target: target, Port: port}))
	must(builder.TXTResource(header(full, dnsmessage.TypeTXT), dnsmessage.TXTResource{TXT: txt}))
	must(builder.AResource(header(target, dnsmessage.TypeA), dnsmessage.AResource{A: address}))
	packet, err := builder.Finish()
	must(err)
	return packet
}

func TestQueryAndResponseRoundTripWithAccents(t *testing.T) {
	query, err := buildQuery(0x4a21, ServiceType)
	if err != nil {
		t.Fatal(err)
	}
	// Encabezado: identificador, sin banderas, una pregunta; la pregunta es «_adelpos._tcp.local» PTR IN.
	if binary.BigEndian.Uint16(query[0:2]) != 0x4a21 || binary.BigEndian.Uint16(query[2:4]) != 0 ||
		binary.BigEndian.Uint16(query[4:6]) != 1 {
		t.Fatalf("encabezado de la consulta: %x", query[:12])
	}
	expected := "\x08_adelpos\x04_tcp\x05local\x00\x00\x0c\x00\x01"
	if string(query[12:]) != expected {
		t.Fatalf("pregunta: %q", query[12:])
	}
	var fingerprint [32]byte
	fingerprint[0] = 0xab
	instance := InstanceName("Fonda Doña Ñeli", "Mac de caja")
	txt := TXTRecords("11111111-2222-4333-8444-555555555555", fingerprint, 8443)
	packet := buildResponse(t, 0x4a21, instance, "Mac-de-caja.local.", 8443, txt, [4]byte{192, 168, 1, 20})
	servers := map[string]*found{}
	if err := parseResponse(packet, 0x4a21, ServiceType, servers); err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("servidores: %+v", servers)
	}
	for name, server := range servers {
		if name != instance+"._adelpos._tcp.local." || server.Instance != "Fonda Doña Ñeli — Mac de caja" {
			t.Fatalf("instancia: %q %q", name, server.Instance)
		}
		if server.Port != 8443 || server.Target != "Mac-de-caja.local." {
			t.Fatalf("destino: %+v", server)
		}
		if server.TXT["server_id"] != "11111111-2222-4333-8444-555555555555" || server.TXT["fp"] != "ab00000000000000" ||
			server.TXT["api"] != "v1" || server.TXT["port"] != "8443" {
			t.Fatalf("TXT: %v", server.TXT)
		}
		if len(server.Addresses) != 1 || server.Addresses[0] != netip.MustParseAddr("192.168.1.20") {
			t.Fatalf("direcciones: %v", server.Addresses)
		}
	}
	// Otra consulta (otro identificador) o una consulta en lugar de una respuesta no se aceptan.
	if err := parseResponse(packet, 0x1111, ServiceType, map[string]*found{}); err == nil {
		t.Fatal("se aceptó la respuesta de otra consulta")
	}
	if err := parseResponse(query, 0x4a21, ServiceType, map[string]*found{}); err == nil {
		t.Fatal("se aceptó una consulta como respuesta")
	}
	// Un paquete cortado no hace fallar al lector con pánico.
	for cut := 0; cut < len(packet); cut += 7 {
		_ = parseResponse(packet[:cut], 0x4a21, ServiceType, map[string]*found{})
	}
}
