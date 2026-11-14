//go:build !windows

package main

// Comandos del servidor como servicio de macOS (etapa 9G, fase A; decisiones D3, D4 y D7):
//
//   - host: lo lanza launchd (LaunchDaemon local.adel.pos.server, ops/launchd) con la cuenta del usuario
//     que instaló. Si la modalidad es «Solo terminal» o no existe el archivo de habilitación, termina sin
//     iniciar PostgreSQL ni Go. Si no, hace lo mismo que el host de Windows (bloqueo de la carpeta, clúster
//     propio con marcador, arranque o adopción de PostgreSQL, respaldo antes de migrar, migración y
//     «serve») y, mientras sirve, vigila el archivo de habilitación y la modalidad: si dejan de pedir
//     servidor, deja de servir y detiene PostgreSQL conservando los datos. Con SIGTERM o SIGINT (launchd al
//     detenerlo o al apagar la computadora) cierra Go en orden y después PostgreSQL (pg_ctl stop -m fast).
//   - backup, restore-check y restore (host.go): como en Windows. Restaurar pausa el servicio (quita el
//     archivo de habilitación y espera a que suelte la carpeta) y lo reanuda al terminar.
//   - adopt-legacy --from-config <server.json anterior>: pasa los datos de la instalación anterior a la
//     carpeta del servicio (internal/standalone/adopt.go). Lo ejecuta el usuario desde la consola.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/config"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/lanlistener"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/localsetup"
	"local/ad-el-pos/server/internal/product"
	"local/ad-el-pos/server/internal/standalone"
)

var hostCommands = map[string]bool{
	"host":          true,
	"backup":        true,
	"restore-check": true,
	"restore":       true,
	"adopt-legacy":  true,
}

// noInstallationDataMessage explica que la carpeta de datos todavía no tiene servidor.
const noInstallationDataMessage = "todavía no hay datos del servidor de AD-EL POS en esta computadora"

// serviceWatchInterval es cada cuánto el servidor revisa el archivo de habilitación y la modalidad (las
// pruebas lo acortan).
var serviceWatchInterval = 2 * time.Second

// restartDelay da tiempo a que salga la respuesta del reinicio antes de empezar el cierre ordenado.
var restartDelay = 300 * time.Millisecond

func runHostCommand(ctx context.Context, logger *slog.Logger, command string, options hostOptions, logToFile bool) error {
	layout, err := hostLayout(options)
	if err != nil {
		return err
	}
	// Modo de pruebas (etapa 9F, D1; en macOS desde la 9G): con ADEL_TEST_MODE=1 ningún comando usa la
	// carpeta de datos real del servicio (/Library/Application Support/AD-EL POS). Se comprueba antes de
	// crear o abrir nada en la carpeta.
	testMode, err := hostTestMode(layout)
	if err != nil {
		return err
	}
	switch command {
	case "host":
		network, err := hostNetworkAddress(options.networkListen, testMode)
		if err != nil {
			return err
		}
		return runHost(ctx, logger, layout, logToFile, testMode, network)
	case "adopt-legacy":
		return runAdoptLegacy(ctx, logger, layout, options, logToFile)
	}
	return runConsoleMaintenance(ctx, logger, command, layout, options, logToFile)
}

// hostNetworkAddress decide la escucha HTTPS de la red local del servicio (etapa 9G, D1): el servicio solo
// sirve con una modalidad con servidor, así que la abre siempre en :8443 (todas las interfaces). «off» la
// apaga y una dirección explícita la sustituye (pruebas). En modo de pruebas (ADEL_TEST_MODE=1) sin dirección
// explícita no se abre: una prueba nunca ocupa el 8443 real por omisión.
func hostNetworkAddress(flag string, testMode bool) (string, error) {
	switch {
	case flag == "off":
		return "", nil
	case flag != "":
		if err := config.CheckNetworkAddress(flag, standalone.ServerAddress); err != nil {
			return "", err
		}
		return flag, nil
	case testMode:
		return "", nil
	}
	return lanlistener.DefaultAddress, nil
}

func runHost(ctx context.Context, logger *slog.Logger, layout standalone.Layout, logToFile bool, testMode bool, networkAddress string) error {
	// El servicio no tiene consola: su bitácora va a la carpeta de datos (launchd guarda aparte la
	// salida estándar en logs/service.log para los fallos anteriores a este punto).
	if !logToFile {
		output, err := openLogFile(layout.ServerLog())
		if err != nil {
			return err
		}
		*logger = *slog.New(slog.NewJSONHandler(output, nil))
	}
	wanted, mode, err := standalone.ServiceWanted(layout)
	if err != nil {
		return err
	}
	if !wanted {
		// «Solo terminal» o servicio deshabilitado: no se inicia PostgreSQL ni Go. Termina sin error y
		// launchd no lo relanza mientras no exista el archivo de habilitación.
		logger.Info("host_disabled", "mode", mode, "service_enabled", standalone.ServiceEnabled(layout))
		return nil
	}
	lock, err := standalone.AcquireLock(layout)
	if err != nil {
		return err
	}
	defer lock.Release()
	// Antes de crear el clúster la primera vez, el puerto de PostgreSQL debe estar libre: si lo ocupa la
	// instalación anterior (por ejemplo, la de desarrollo antes de pasar sus datos con adopt-legacy), no
	// se crea nada y la carpeta sigue lista para recibir esos datos.
	if !standalone.ClusterInitialized(layout) && portInUse(layout.Port) {
		return fmt.Errorf("el puerto %d de PostgreSQL está ocupado por otro programa (¿la instalación anterior sigue en marcha?); no se creó la base", layout.Port)
	}
	serving, stop := context.WithCancel(ctx)
	defer stop()
	started := time.Now()
	control := &serviceControl{
		layout:  layout,
		started: started,
		// launchd es el proceso 1: solo así hay quien relance el servidor después de reiniciarlo.
		managed: os.Getppid() == 1,
		stop:    stop,
		logger:  logger,
	}
	go watchService(serving, layout, logger, stop)
	logger.Info("host_starting", "version", product.Read().Version, "data_dir", layout.DataDir, "mode", mode, "managed", control.managed, "test_mode", testMode)
	cluster := standalone.NewCluster(layout, logger)
	// PostgreSQL se detiene al final pase lo que pase (después del cierre de Go), también si la
	// preparación falló después de iniciarlo.
	defer func() {
		stopContext, done := context.WithTimeout(context.Background(), 90*time.Second)
		defer done()
		_ = cluster.Stop(stopContext)
	}()
	cfg, err := standalone.Prepare(serving, cluster, product.Read().DatabaseName)
	if err != nil {
		return err
	}
	if err := licensing.CheckStartup(cfg.Environment, cfg.LicenseMode); err != nil {
		return err
	}
	pool, err := database.Open(serving, cfg.Database)
	if err != nil {
		return errors.New("no se pudo crear el pool PostgreSQL")
	}
	defer pool.Close()
	if err := migrateWithBackup(serving, logger, layout, cfg, pool); err != nil {
		return err
	}
	logger.Info("host_ready", "duration_ms", time.Since(started).Milliseconds())
	backups := installationBackup{layout: layout, database: cfg.Database}
	application := newApplication(pool, layout.ConfigFile(), backups, logger)
	application.Host = control
	// La identidad del servidor (certificado propio y server_id) vive en la carpeta de datos del servicio.
	network := networkOptions{Address: networkAddress, IdentityDir: filepath.Join(layout.DataDir, localsetup.IdentityFolder)}
	return serveHTTP(serving, logger, cfg, pool, application, testMode, network)
}

// portInUse indica si algo escucha en el puerto de loopback.
func portInUse(port int) bool {
	connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		return false
	}
	connection.Close()
	return true
}

// watchService detiene el servidor cuando la computadora deja de ser servidor: se quitó el archivo de
// habilitación o la modalidad pasó a «Solo terminal». Los datos se conservan; un archivo de modalidad
// ilegible no detiene nada (se registra y se sigue sirviendo).
func watchService(ctx context.Context, layout standalone.Layout, logger *slog.Logger, stop context.CancelFunc) {
	ticker := time.NewTicker(serviceWatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		wanted, mode, err := standalone.ServiceWanted(layout)
		if err != nil {
			logger.Warn("service_mode_unreadable", "reason", err.Error())
			continue
		}
		if !wanted {
			logger.Info("service_disabled_stopping", "mode", mode, "service_enabled", standalone.ServiceEnabled(layout))
			stop()
			return
		}
	}
}

// serviceControl es el HostControl del servicio de macOS para Mantenimiento (estado y reinicio).
type serviceControl struct {
	layout  standalone.Layout
	started time.Time
	managed bool
	stop    context.CancelFunc
	logger  *slog.Logger
	once    sync.Once
}

func (control *serviceControl) HostStatus() administration.HostStatus {
	_, mode, err := standalone.ServiceWanted(control.layout)
	if err != nil {
		mode = ""
	}
	return administration.HostStatus{
		Managed:          control.managed,
		InstallationMode: mode,
		ServiceEnabled:   standalone.ServiceEnabled(control.layout),
		StartedAt:        control.started,
	}
}

// Restart termina el servidor en orden poco después de responder; launchd lo vuelve a lanzar porque el
// archivo de habilitación existe (KeepAlive → PathState). Sin launchd o sin ese archivo nadie lo
// relanzaría: entonces no se reinicia.
func (control *serviceControl) Restart() bool {
	if !control.managed || !standalone.ServiceEnabled(control.layout) {
		return false
	}
	control.once.Do(func() {
		control.logger.Info("service_restart_requested")
		time.AfterFunc(restartDelay, control.stop)
	})
	return true
}

// pauseServerForRestore pausa el servicio para restaurar: quita el archivo de habilitación (el servidor
// deja de servir y detiene PostgreSQL) y espera a tomar el bloqueo de la carpeta. Devuelve el bloqueo y la
// función que reanuda el servicio (vuelve a crear el archivo; launchd lo relanza).
func pauseServerForRestore(ctx context.Context, logger *slog.Logger, layout standalone.Layout) (*standalone.HostLock, func(), error) {
	if !standalone.ServiceEnabled(layout) {
		return nil, nil, errors.New("otro proceso de AD-EL POS está usando la carpeta de datos; espera a que termine")
	}
	fmt.Println("Pausando el servidor para restaurar el respaldo…")
	if err := standalone.SetServiceEnabled(layout, false); err != nil {
		return nil, nil, err
	}
	logger.Info("service_paused_for_restore")
	resume := func() {
		if err := standalone.SetServiceEnabled(layout, true); err != nil {
			fmt.Println("No se pudo reanudar el servidor: " + err.Error())
			return
		}
		logger.Info("service_resumed_after_restore")
		fmt.Println("Servidor reanudado.")
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		lock, err := standalone.AcquireLock(layout)
		if err == nil {
			return lock, resume, nil
		}
		if !errors.Is(err, standalone.ErrLocked) {
			resume()
			return nil, nil, err
		}
		if time.Now().After(deadline) {
			resume()
			return nil, nil, errors.New("el servidor no se detuvo a tiempo; no se restauró nada")
		}
		select {
		case <-ctx.Done():
			resume()
			return nil, nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// runAdoptLegacy ejecuta el paso de datos de la instalación anterior desde la consola.
func runAdoptLegacy(ctx context.Context, logger *slog.Logger, layout standalone.Layout, options hostOptions, logToFile bool) error {
	if options.fromConfig == "" {
		return errors.New("indica la configuración de la instalación anterior con --from-config")
	}
	if !logToFile {
		if output, err := openLogFile(layout.MaintenanceLog()); err == nil {
			*logger = *slog.New(slog.NewJSONHandler(output, nil))
		}
	}
	fmt.Println("Pasando los datos de la instalación anterior a la carpeta del servicio…")
	report, err := standalone.AdoptLegacy(ctx, layout, logger, options.fromConfig, product.Read().DatabaseName, product.Read().Version)
	if err != nil {
		fmt.Println("No se pudo completar: " + err.Error())
		return err
	}
	logger.Info("legacy_adopted", "backup", report.Backup, "tables", len(report.Tables))
	fmt.Printf("Datos pasados y comprobados: %d tablas con conteos idénticos.\n", len(report.Tables))
	fmt.Println("Respaldo de la instalación anterior: " + report.Backup)
	fmt.Println("La instalación anterior no se modificó. Para que el servidor empiece a servir, elige «Servidor y terminal» o «Solo servidor» en la aplicación.")
	return nil
}
