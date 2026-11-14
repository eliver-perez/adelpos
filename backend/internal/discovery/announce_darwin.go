//go:build darwin

package discovery

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// dnsSDPath es la herramienta de Bonjour del sistema (siempre presente en macOS).
const dnsSDPath = "/usr/bin/dns-sd"

// stopGrace es cuánto se espera a que dns-sd termine con SIGTERM antes de forzarlo.
const stopGrace = 3 * time.Second

// dnsSDRegistration es un `dns-sd -R` en marcha. Mientras el proceso vive, mDNSResponder publica el servicio;
// al terminarlo (SIGTERM), su conexión con mDNSResponder se cierra y el sistema retira el registro enviando la
// despedida (TTL 0) a la red.
//
// El proceso queda en el grupo de procesos del servidor: si launchd detiene el servicio, también lo termina
// (launchd termina el grupo de procesos del trabajo al acabar, salvo AbandonProcessGroup), así que no queda un
// anuncio huérfano. PostgreSQL no se ve afectado porque pg_ctl lo separa en su propia sesión.
type dnsSDRegistration struct {
	command *exec.Cmd
	done    chan struct{}
	once    sync.Once
}

func (registration *dnsSDRegistration) Done() <-chan struct{} {
	return registration.done
}

func (registration *dnsSDRegistration) Stop() {
	registration.once.Do(func() {
		select {
		case <-registration.done:
			return
		default:
		}
		_ = registration.command.Process.Signal(syscall.SIGTERM)
		timer := time.NewTimer(stopGrace)
		defer timer.Stop()
		select {
		case <-registration.done:
		case <-timer.C:
			_ = registration.command.Process.Kill()
			<-registration.done
		}
	})
}

// registerArguments son los argumentos de `dns-sd -R`: nombre de instancia, tipo, dominio, puerto y cada TXT
// como un argumento propio. Se pasan por separado (sin intérprete de órdenes): el nombre del establecimiento
// nunca se interpreta.
func registerArguments(announcement Announcement) []string {
	arguments := []string{
		"-R",
		announcement.Instance,
		ServiceType,
		Domain,
		strconv.Itoa(announcement.Port),
	}
	return append(arguments, announcement.TXT...)
}

// platformRegister publica el anuncio con Bonjour del sistema.
func platformRegister(_ context.Context, logger *slog.Logger, announcement Announcement) (registration, error) {
	if announcement.Instance == "" || announcement.Port <= 0 || announcement.Port > 65535 {
		return nil, errors.New("anuncio incompleto")
	}
	// No se usa CommandContext: al terminar se envía SIGTERM (no SIGKILL) para que el sistema despida el
	// registro en la red.
	command := exec.Command(dnsSDPath, registerArguments(announcement)...)
	output, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	command.Stderr = command.Stdout
	if err := command.Start(); err != nil {
		return nil, errors.New("no se pudo iniciar el anuncio de Bonjour")
	}
	registration := &dnsSDRegistration{command: command, done: make(chan struct{})}
	go func() {
		watchOutput(logger, announcement.Instance, output)
		_ = command.Wait()
		close(registration.done)
	}()
	return registration, nil
}

// watchOutput lee la salida de dns-sd solo para la bitácora: el registro activo y, si el sistema tuvo que
// renombrarlo por un nombre repetido en la red, el aviso. Nada depende del formato de esa salida.
func watchOutput(logger *slog.Logger, instance string, output io.Reader) {
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.Contains(line, "Name now registered and active"):
			logger.Info("discovery_registered", "instance", instance)
		case strings.Contains(strings.ToLower(line), "conflict"):
			logger.Warn("discovery_name_conflict", "instance", instance)
		}
	}
}
