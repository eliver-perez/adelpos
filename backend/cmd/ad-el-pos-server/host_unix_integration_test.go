//go:build integration && !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/config"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/standalone"
)

// TestMain permite que la prueba ejecute el servidor real como otro proceso (el mismo binario de la
// prueba con ADEL_HOST_TEST_MAIN=1 entra a main): así las señales, el bloqueo de la carpeta y la salida
// son los del servicio de verdad.
func TestMain(m *testing.M) {
	if os.Getenv("ADEL_HOST_TEST_MAIN") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// Puertos de la prueba (fuera de los reales 55432/8181 y del clúster aislado 55499).
const (
	hostPGPort    = "55511"
	adoptPGPort   = "55512"
	hostListen    = "127.0.0.1:8192"
	legacyListen  = "127.0.0.1:8193"
	adoptedListen = "127.0.0.1:8194"
	// Escucha HTTPS de la red local del servicio en la prueba (etapa 9G, fase B): solo loopback.
	hostNetworkListen = "127.0.0.1:8196"
	readyTimeout      = 180 * time.Second
	stopTimeout       = 60 * time.Second
	commandTimeout    = 180 * time.Second
)

// lockedBuffer guarda la salida del proceso; la escriben las copias de exec y la lee la prueba.
type lockedBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (output *lockedBuffer) Write(data []byte) (int, error) {
	output.mutex.Lock()
	defer output.mutex.Unlock()
	return output.buffer.Write(data)
}

func (output *lockedBuffer) String() string {
	output.mutex.Lock()
	defer output.mutex.Unlock()
	return output.buffer.String()
}

type serverProcess struct {
	cmd    *exec.Cmd
	output *lockedBuffer
	// done recibe el resultado de Wait una sola vez; exited se cierra cuando el proceso terminó.
	done   chan error
	exited chan struct{}
}

func testPostgresBin(t *testing.T) string {
	t.Helper()
	candidates := []string{os.Getenv("ADEL_PG_BIN"), "/opt/homebrew/opt/postgresql@18/bin", "/usr/local/opt/postgresql@18/bin"}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(candidate, "initdb")); err == nil {
			return candidate
		}
	}
	t.Skip("falta PostgreSQL 18 (ADEL_PG_BIN o Homebrew)")
	return ""
}

func serverCommand(arguments ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], arguments...)
	cmd.Env = append(os.Environ(), "ADEL_HOST_TEST_MAIN=1", "ADEL_TEST_MODE=1")
	return cmd
}

// runServer ejecuta un comando del servidor hasta que termina y devuelve su salida y su código.
func runServer(t *testing.T, arguments ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := serverCommand(arguments...)
	output := &lockedBuffer{}
	cmd.Stdout = output
	cmd.Stderr = output
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return output.String(), exit.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return output.String(), 0
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		t.Fatalf("el comando %v no terminó: %s", arguments, output.String())
	}
	return "", -1
}

func startServer(t *testing.T, arguments ...string) *serverProcess {
	t.Helper()
	process := &serverProcess{
		cmd:    serverCommand(arguments...),
		output: &lockedBuffer{},
		done:   make(chan error, 1),
		exited: make(chan struct{}),
	}
	process.cmd.Stdout = process.output
	process.cmd.Stderr = process.output
	if err := process.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		err := process.cmd.Wait()
		close(process.exited)
		process.done <- err
	}()
	t.Cleanup(func() {
		select {
		case <-process.exited:
			return
		default:
		}
		_ = process.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-process.exited:
		case <-time.After(stopTimeout):
			_ = process.cmd.Process.Kill()
		}
	})
	return process
}

// waitExit espera a que el servidor termine y devuelve su código.
func (process *serverProcess) waitExit(t *testing.T) int {
	t.Helper()
	select {
	case err := <-process.done:
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0
	case <-time.After(stopTimeout):
		t.Fatalf("el servidor no terminó: %s", process.output.String())
	}
	return -1
}

func waitReady(t *testing.T, address string, process *serverProcess, serverLog string) {
	t.Helper()
	client := http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(readyTimeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-process.done:
			log, _ := os.ReadFile(serverLog)
			t.Fatalf("el servidor terminó antes de quedar listo (%v): %s\n%s", err, process.output.String(), log)
		default:
		}
		response, err := client.Get("http://" + address + "/api/v1/health/ready")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				return
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	log, _ := os.ReadFile(serverLog)
	t.Fatalf("el servidor no quedó listo: %s", log)
}

func getJSON(t *testing.T, address, route string) map[string]any {
	t.Helper()
	response, err := http.Get("http://" + address + "/api/v1/" + route)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func requireMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("%s tiene permisos %v, se esperaban %v", filepath.Base(path), info.Mode().Perm(), mode)
	}
}

func clusterRunning(layout standalone.Layout) bool {
	_, err := os.Stat(filepath.Join(layout.Cluster(), "postmaster.pid"))
	return err == nil
}

// TestServiceHostOnMacOS recorre el servicio de macOS (etapa 9G, fase A) con el binario real, un clúster
// temporal propio (puertos 55511 y 55512) y carpetas temporales: sin habilitación o en «Solo terminal» no
// arranca nada; habilitado crea el clúster, migra y sirve; un segundo servidor queda bloqueado; respaldo y
// ensayo de restauración con el servicio en marcha; quitar el archivo de habilitación lo detiene en orden;
// restaurar pausa y reanuda el servicio; SIGTERM lo detiene en orden; y adopt-legacy pasa los datos de una
// base anterior (en el clúster aislado de ADEL_TEST_DATABASE_URL), compara los conteos, deja el marcador,
// no toca la base anterior y se niega a repetirse o a correr con el servidor anterior en marcha.
func TestServiceHostOnMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("servicio de macOS")
	}
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("define ADEL_TEST_DATABASE_URL para la base anterior de adopt-legacy")
	}
	pgBin := testPostgresBin(t)
	root := t.TempDir()
	dataDir := filepath.Join(root, "AD-EL POS")
	layout := standalone.Layout{DataDir: dataDir}
	common := []string{"--data-dir", dataDir, "--pg-bin", pgBin, "--pg-port", hostPGPort, "--listen", hostListen}
	host := append([]string{"host"}, common...)

	// Sin archivo de habilitación y en «Solo terminal» no se inicia PostgreSQL ni Go.
	if output, code := runServer(t, host...); code != 0 {
		t.Fatalf("sin habilitación termina sin error: %d %s", code, output)
	}
	if err := standalone.WriteMode(layout, standalone.ModeTerminalOnly); err != nil {
		t.Fatal(err)
	}
	if err := standalone.SetServiceEnabled(layout, true); err != nil {
		t.Fatal(err)
	}
	if output, code := runServer(t, host...); code != 0 {
		t.Fatalf("«Solo terminal» termina sin error: %d %s", code, output)
	}
	if standalone.HasData(layout) {
		t.Fatal("«Solo terminal» no crea el clúster ni la configuración")
	}

	// Con el puerto de PostgreSQL ocupado (la instalación anterior sigue en marcha) no se crea nada.
	if err := standalone.WriteMode(layout, standalone.ModeServerTerminal); err != nil {
		t.Fatal(err)
	}
	busy, err := net.Listen("tcp", "127.0.0.1:"+hostPGPort)
	if err != nil {
		t.Fatal(err)
	}
	_, busyCode := runServer(t, host...)
	busy.Close()
	if busyCode == 0 || standalone.HasData(layout) {
		t.Fatalf("con el puerto ocupado no se crea el clúster: %d", busyCode)
	}
	if log, _ := os.ReadFile(layout.ServerLog()); !strings.Contains(string(log), "está ocupado") {
		t.Fatalf("debe explicar el puerto ocupado: %s", log)
	}

	// Habilitado: crea el clúster, migra y sirve.
	first := startServer(t, append(append([]string{}, host...), "--network-listen", hostNetworkListen)...)
	waitReady(t, hostListen, first, layout.ServerLog())
	// Etapa 9G, fase B: identidad propia en la carpeta del servicio y escucha HTTPS de la red local.
	checkServiceNetwork(t, layout, hostNetworkListen)
	version := getJSON(t, hostListen, "version")
	if version["test_mode"] != true {
		t.Fatalf("ADEL_TEST_MODE=1 debe informarse: %v", version)
	}
	requireMode(t, dataDir, 0700)
	requireMode(t, layout.ConfigFile(), 0600)
	requireMode(t, layout.AdminPassword(), 0600)
	requireMode(t, layout.Cluster(), 0700)
	if _, err := os.Stat(layout.Marker()); err != nil {
		t.Fatal("falta el marcador del clúster")
	}
	if !clusterRunning(layout) {
		t.Fatal("PostgreSQL debe estar en marcha")
	}

	// Un segundo servidor sobre la misma carpeta queda bloqueado.
	if _, code := runServer(t, host...); code == 0 {
		t.Fatal("un segundo servidor debe fallar")
	}
	if log, _ := os.ReadFile(layout.ServerLog()); !strings.Contains(string(log), "ya está en uso") {
		t.Fatalf("el segundo servidor debe explicar el bloqueo: %s", log)
	}

	// Respaldo y ensayo de restauración con el servicio en marcha (usan su PostgreSQL).
	output, code := runServer(t, append([]string{"backup"}, common...)...)
	if code != 0 || !strings.Contains(output, "Respaldo creado y comprobado") {
		t.Fatalf("respaldo: %d %s", code, output)
	}
	backups := standalone.LatestBackups(layout)
	if len(backups) != 1 {
		t.Fatalf("se esperaba un respaldo: %v", backups)
	}
	requireMode(t, filepath.Join(layout.Backups(), backups[0]), 0600)
	output, code = runServer(t, append([]string{"restore-check"}, common...)...)
	if code != 0 || !strings.Contains(output, "Ensayo de restauración correcto") {
		t.Fatalf("ensayo de restauración: %d %s", code, output)
	}

	// Al quitar el archivo de habilitación el servidor deja de servir, detiene PostgreSQL y conserva los
	// datos.
	if err := standalone.SetServiceEnabled(layout, false); err != nil {
		t.Fatal(err)
	}
	if code := first.waitExit(t); code != 0 {
		t.Fatalf("al dejar de ser servidor termina sin error: %d", code)
	}
	if clusterRunning(layout) {
		t.Fatal("PostgreSQL debe detenerse al dejar de ser servidor")
	}
	if !standalone.HasData(layout) {
		t.Fatal("los datos se conservan")
	}

	// Restaurar con el servicio en marcha lo pausa y lo reanuda.
	if err := standalone.SetServiceEnabled(layout, true); err != nil {
		t.Fatal(err)
	}
	second := startServer(t, host...)
	waitReady(t, hostListen, second, layout.ServerLog())
	restore := append([]string{"restore", "--file", filepath.Join(layout.Backups(), backups[0])}, common...)
	output, code = runServer(t, restore...)
	if code != 0 || !strings.Contains(output, "Respaldo restaurado") || !strings.Contains(output, "Servidor reanudado") {
		t.Fatalf("restaurar: %d %s", code, output)
	}
	if code := second.waitExit(t); code != 0 {
		t.Fatalf("el servidor pausado termina sin error: %d", code)
	}
	if !standalone.ServiceEnabled(layout) {
		t.Fatal("restaurar vuelve a habilitar el servicio")
	}
	safety := 0
	for _, name := range standalone.LatestBackups(layout) {
		if strings.HasSuffix(name, "-antes-de-restaurar.dump") {
			safety++
		}
	}
	if safety != 1 {
		t.Fatal("restaurar crea antes un respaldo de seguridad")
	}

	// Hallazgo 4 de la revisión: con la identidad dañada (falta identity.json) el servidor sigue atendiendo a la
	// propia computadora, sin escucha de red y con el emparejamiento desactivado.
	restoreIdentity := damageIdentity(t, layout)
	// SIGTERM (launchd al detenerlo o al apagar) cierra Go y después PostgreSQL.
	third := startServer(t, append(append([]string{}, host...), "--network-listen", hostNetworkListen)...)
	waitReady(t, hostListen, third, layout.ServerLog())
	checkWithoutIdentity(t, layout, hostListen, hostNetworkListen)
	restoreIdentity()
	if err := third.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if code := third.waitExit(t); code != 0 {
		t.Fatalf("SIGTERM termina sin error: %d", code)
	}
	if clusterRunning(layout) {
		t.Fatal("PostgreSQL debe detenerse con SIGTERM")
	}
	if log, _ := os.ReadFile(layout.ServerLog()); !strings.Contains(string(log), "server_shutdown_complete") || !strings.Contains(string(log), "cluster_stopped") {
		t.Fatalf("el cierre ordenado debe quedar en la bitácora: %s", log)
	}

	testAdoptLegacy(t, root, pgBin, dsn)
}

// testAdoptLegacy: la base anterior es la de ADEL_TEST_DATABASE_URL con todas las migraciones y algunos
// datos; su configuración es un server.json como el de .local.
func testAdoptLegacy(t *testing.T, root, pgBin, dsn string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	legacy := config.Config{
		Environment:      "development",
		InstallationMode: "server-terminal",
		ListenAddress:    legacyListen,
		AllowedOrigins:   []string{"tauri://localhost"},
		Database: config.Database{
			Host:           parsed.Host,
			Port:           int(parsed.Port),
			Name:           parsed.Database,
			User:           parsed.User,
			Password:       parsed.Password,
			SSLMode:        "disable",
			MaxConnections: 4,
		},
		LicenseMode: "development",
	}
	pool, err := database.Open(ctx, legacy.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.devices(name, secret_hash) VALUES ('Caja anterior 9G', md5(random()::text))`); err != nil {
		t.Fatal(err)
	}
	legacyDir := filepath.Join(root, "instalación anterior", ".local")
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatal(err)
	}
	legacyFile := filepath.Join(legacyDir, "server.json")
	body, _ := json.Marshal(legacy)
	if err := os.WriteFile(legacyFile, body, 0600); err != nil {
		t.Fatal(err)
	}
	legacyTables := func() map[string]int {
		counts := map[string]int{}
		rows, err := pool.Query(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = 'app'")
		if err != nil {
			t.Fatal(err)
		}
		tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		for _, table := range tables {
			var count int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM app."+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil {
				t.Fatal(err)
			}
			counts[table] = count
		}
		return counts
	}
	before := legacyTables()

	target := filepath.Join(root, "Servicio nuevo", "AD-EL POS")
	targetLayout := standalone.Layout{DataDir: target}
	adopt := []string{"adopt-legacy", "--from-config", legacyFile, "--data-dir", target, "--pg-bin", pgBin, "--pg-port", adoptPGPort, "--listen", adoptedListen}

	// Con el servidor anterior en marcha no se copia nada.
	listener, err := net.Listen("tcp", legacyListen)
	if err != nil {
		t.Fatal(err)
	}
	output, code := runServer(t, adopt...)
	listener.Close()
	if code == 0 || !strings.Contains(output, "sigue en marcha") || standalone.HasData(targetLayout) {
		t.Fatalf("con el servidor anterior en marcha no se copia: %d %s", code, output)
	}

	output, code = runServer(t, adopt...)
	if code != 0 || !strings.Contains(output, "Datos pasados y comprobados") {
		t.Fatalf("adopt-legacy: %d %s", code, output)
	}
	var report standalone.AdoptReport
	marker, err := os.ReadFile(targetLayout.AdoptionMarker())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(marker, &report); err != nil {
		t.Fatal(err)
	}
	if !report.CountsEqual || len(report.Tables) != len(before) || report.SourceDatabase != legacy.Database.Name {
		t.Fatalf("marcador inesperado: %+v", report)
	}
	for table, count := range before {
		if report.Tables[table] != count {
			t.Fatalf("la tabla %s no coincide: %d frente a %d", table, report.Tables[table], count)
		}
	}
	if strings.Contains(string(marker), legacy.Database.Password) {
		t.Fatal("el marcador no guarda secretos")
	}
	requireMode(t, targetLayout.AdoptionMarker(), 0600)
	requireMode(t, filepath.Join(targetLayout.Backups(), report.Backup), 0600)
	if clusterRunning(targetLayout) {
		t.Fatal("el clúster nuevo queda detenido")
	}
	// La base anterior no cambió.
	after := legacyTables()
	if fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatal("adopt-legacy no debe modificar la base anterior")
	}
	// Repetirlo sobre la carpeta con datos se niega.
	output, code = runServer(t, adopt...)
	if code == 0 || !strings.Contains(output, "ya tiene datos") {
		t.Fatalf("no se escribe encima: %d %s", code, output)
	}

	// El servicio arranca con los datos pasados (en el puerto definitivo de su configuración).
	if err := standalone.WriteMode(targetLayout, standalone.ModeServerOnly); err != nil {
		t.Fatal(err)
	}
	if err := standalone.SetServiceEnabled(targetLayout, true); err != nil {
		t.Fatal(err)
	}
	adopted := startServer(t, "host", "--data-dir", target, "--pg-bin", pgBin, "--pg-port", adoptPGPort)
	waitReady(t, adoptedListen, adopted, targetLayout.ServerLog())
	// En modo de pruebas, sin --network-listen el servicio no abre la escucha de red (nunca el 8443 real).
	if log, _ := os.ReadFile(targetLayout.ServerLog()); !strings.Contains(string(log), `"network_listen":""`) {
		t.Fatalf("en modo de pruebas no se abre la escucha de red por omisión: %s", log)
	}
	devices := 0
	cfg, err := config.Load(targetLayout.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Port != 55512 || cfg.ListenAddress != adoptedListen {
		t.Fatalf("configuración del servicio: %+v", cfg.Database)
	}
	adoptedPool, err := database.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer adoptedPool.Close()
	if err := adoptedPool.QueryRow(ctx, "SELECT count(*) FROM app.devices WHERE name = 'Caja anterior 9G'").Scan(&devices); err != nil || devices != 1 {
		t.Fatalf("los datos anteriores deben estar en el servicio: %d %v", devices, err)
	}
	adoptedPool.Close()
	if err := adopted.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if code := adopted.waitExit(t); code != 0 {
		t.Fatalf("SIGTERM termina sin error: %d", code)
	}
}
