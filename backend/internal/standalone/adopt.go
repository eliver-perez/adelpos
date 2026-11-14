package standalone

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/config"
)

// Paso de los datos de una instalación anterior a la carpeta del servicio (etapa 9G, decisión D4). En
// macOS los datos vivían en la carpeta del proyecto (~/Documents/…/.local), que un servicio del sistema no
// puede leer (TCC); por eso el paso lo ejecuta el usuario desde la consola, con el servicio sin servir:
//
//  1. Comprueba que la carpeta del servicio no tenga datos y que el servidor anterior esté detenido (sin
//     escrituras durante la copia).
//  2. Cuenta las filas de todas las tablas de la base anterior, la respalda con pg_dump en la carpeta de
//     respaldos del servicio, comprueba el respaldo con pg_restore --list y vuelve a contar (si algo
//     cambió en medio, no sigue).
//  3. Crea el clúster nuevo en la carpeta del servicio y lo arranca en un puerto temporal libre (el
//     anterior sigue ocupando el definitivo), restaura el respaldo y compara los conteos de todas las
//     tablas con los de la base anterior.
//  4. Si todo coincide, detiene el clúster nuevo y deja el marcador legacy-adopted.json. Si algo falla,
//     quita solo lo que creó (clúster, configuración y marcador del clúster) y conserva el respaldo.
//
// Nunca modifica ni borra la instalación anterior: solo la lee (pg_dump y conteos).

// AdoptReport es el resultado del paso de datos, que también queda en el marcador.
type AdoptReport struct {
	FromConfig     string         `json:"from_config"`
	SourceDatabase string         `json:"source_database"`
	Backup         string         `json:"backup"`
	CountsEqual    bool           `json:"counts_equal"`
	Tables         map[string]int `json:"tables"`
	AdoptedAt      time.Time      `json:"adopted_at"`
	ServerVersion  string         `json:"server_version"`
}

// HasData indica si la carpeta de datos ya tiene una instalación (clúster, configuración o marcadores):
// el paso de datos nunca escribe encima.
func HasData(layout Layout) bool {
	for _, path := range []string{layout.Cluster(), layout.ConfigFile(), layout.Marker(), layout.AdoptionMarker()} {
		if _, err := os.Lstat(path); err == nil {
			return true
		}
	}
	return false
}

// AdoptLegacy pasa los datos de la instalación anterior descrita por legacyConfigFile (su server.json) a
// la carpeta del servicio. Quien llama no debe tener el bloqueo de la carpeta: lo toma aquí.
func AdoptLegacy(ctx context.Context, layout Layout, logger *slog.Logger, legacyConfigFile, databaseName, version string) (AdoptReport, error) {
	legacyPath, err := filepath.Abs(legacyConfigFile)
	if err != nil {
		return AdoptReport{}, errors.New("ruta de la configuración anterior inválida")
	}
	legacy, err := config.Load(legacyPath)
	if err != nil {
		return AdoptReport{}, fmt.Errorf("la configuración anterior no es válida: %w", err)
	}
	if samePath(filepath.Dir(legacyPath), layout.DataDir) {
		return AdoptReport{}, errors.New("la configuración anterior no puede ser la de la carpeta del servicio")
	}
	if HasData(layout) {
		return AdoptReport{}, errors.New("la carpeta de datos del servicio ya tiene datos; no se escribirá encima")
	}
	if serverAnswers(legacy.ListenAddress) {
		return AdoptReport{}, errors.New("el servidor anterior sigue en marcha; detenlo antes de pasar los datos " +
			"(en desarrollo: scripts/dev.sh server-stop) para que nada cambie durante la copia")
	}
	lock, err := AcquireLock(layout)
	if err != nil {
		if errors.Is(err, ErrLocked) {
			return AdoptReport{}, errors.New("el servicio de AD-EL POS está usando la carpeta de datos; no se pasaron los datos")
		}
		return AdoptReport{}, err
	}
	defer lock.Release()
	// Comprobación repetida con el bloqueo tomado: otro proceso pudo crear datos mientras tanto.
	if HasData(layout) {
		return AdoptReport{}, errors.New("la carpeta de datos del servicio ya tiene datos; no se escribirá encima")
	}
	cluster := NewCluster(layout, logger)
	if err := cluster.prepareFolders(); err != nil {
		return AdoptReport{}, err
	}
	if err := checkPostgresTools(layout); err != nil {
		return AdoptReport{}, err
	}

	before, err := legacyCounts(ctx, legacy.Database)
	if err != nil {
		return AdoptReport{}, err
	}
	dump, err := dumpLegacy(ctx, layout, legacy.Database)
	if err != nil {
		return AdoptReport{}, err
	}
	after, err := legacyCounts(ctx, legacy.Database)
	if err != nil {
		return AdoptReport{}, err
	}
	if !reflect.DeepEqual(before, after) {
		return AdoptReport{}, errors.New("la base anterior cambió mientras se respaldaba; detén todo lo que la use y repite el paso")
	}
	logger.Info("legacy_backup_created", "file", filepath.Base(dump), "tables", len(before))

	restored, err := restoreLegacy(ctx, layout, cluster, databaseName, dump)
	if err != nil {
		removeAdoptedCluster(layout, cluster, logger)
		return AdoptReport{}, err
	}
	report := AdoptReport{
		FromConfig:     legacyPath,
		SourceDatabase: legacy.Database.Name,
		Backup:         filepath.Base(dump),
		CountsEqual:    reflect.DeepEqual(before, restored),
		Tables:         restored,
		AdoptedAt:      time.Now().UTC(),
		ServerVersion:  version,
	}
	if !report.CountsEqual {
		removeAdoptedCluster(layout, cluster, logger)
		return report, fmt.Errorf("los conteos restaurados difieren de la base anterior (%d tablas); no se usará la copia. "+
			"El respaldo %s se conserva", len(before), report.Backup)
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		removeAdoptedCluster(layout, cluster, logger)
		return report, err
	}
	if err := writePrivateFile(layout.AdoptionMarker(), append(body, '\n')); err != nil {
		removeAdoptedCluster(layout, cluster, logger)
		return report, err
	}
	return report, nil
}

// serverAnswers indica si algo responde en la dirección del servidor anterior.
func serverAnswers(address string) bool {
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		return false
	}
	connection.Close()
	return true
}

// legacyEnvironment son las variables de libpq para leer la base anterior con su propio rol, con los
// datos de conexión de su configuración (la contraseña solo en el entorno del proceso hijo).
func legacyEnvironment(database config.Database) []string {
	environment := []string{
		"PGHOST=" + database.Host,
		"PGPORT=" + strconv.Itoa(database.Port),
		"PGUSER=" + database.User,
		"PGPASSWORD=" + database.Password,
		"PGDATABASE=" + database.Name,
		"PGSSLMODE=" + database.SSLMode,
		"PGCONNECT_TIMEOUT=5",
		"PGCLIENTENCODING=UTF8",
		"LC_MESSAGES=C",
	}
	if database.SSLRootCert != "" {
		environment = append(environment, "PGSSLROOTCERT="+database.SSLRootCert)
	}
	return environment
}

// legacyCounts cuenta las filas de todas las tablas del esquema app de la base anterior.
func legacyCounts(ctx context.Context, database config.Database) (map[string]int, error) {
	connection, err := pgx.Connect(ctx, database.ConnectionString())
	if err != nil {
		return nil, errors.New("no se pudo conectar con la base anterior; comprueba que su PostgreSQL esté en marcha")
	}
	defer connection.Close(context.Background())
	counts, err := countTables(ctx, connection)
	if err != nil {
		return nil, err
	}
	if len(counts) == 0 {
		return nil, errors.New("la base anterior no tiene tablas de AD-EL POS")
	}
	return counts, nil
}

// dumpLegacy respalda la base anterior en la carpeta de respaldos del servicio y comprueba el respaldo.
func dumpLegacy(ctx context.Context, layout Layout, database config.Database) (string, error) {
	suffix, err := randomHex(3)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s-%s-instalacion-anterior.dump", database.Name, time.Now().UTC().Format("20060102T150405Z"), suffix)
	filename := filepath.Join(layout.Backups(), name)
	environment := legacyEnvironment(database)
	output, err := runCaptured(ctx, layout.tool("pg_dump"), []string{"--format=custom", "--no-owner", "--no-acl", "--file", filename}, environment)
	if err != nil {
		_ = os.Remove(filename)
		return "", fmt.Errorf("no se pudo respaldar la base anterior: %s", tail(output))
	}
	if _, err := runCaptured(ctx, layout.tool("pg_restore"), []string{"--list", filename}, environment); err != nil {
		_ = os.Remove(filename)
		return "", errors.New("el respaldo de la base anterior no se pudo leer; se descartó")
	}
	if err := privateFile(filename); err != nil {
		return "", err
	}
	return filename, nil
}

// restoreLegacy crea el clúster del servicio, lo arranca en un puerto temporal, restaura el respaldo y
// devuelve los conteos restaurados. El clúster queda detenido al terminar.
func restoreLegacy(ctx context.Context, layout Layout, cluster *Cluster, databaseName, dump string) (map[string]int, error) {
	if err := cluster.Ensure(ctx); err != nil {
		return nil, err
	}
	temporary, err := freePort(layout.Port)
	if err != nil {
		return nil, err
	}
	cluster.temporaryPort = temporary
	if err := cluster.Start(ctx); err != nil {
		return nil, err
	}
	defer func() {
		stopContext, done := context.WithTimeout(context.Background(), 90*time.Second)
		defer done()
		_ = cluster.Stop(stopContext)
	}()
	if err := ensureConfig(layout, databaseName); err != nil {
		return nil, err
	}
	cfg, err := config.Load(layout.ConfigFile())
	if err != nil {
		return nil, err
	}
	// Las conexiones de esta sesión van al puerto temporal; la configuración conserva el definitivo.
	running := layout
	running.Port = temporary
	if err := ensureDatabase(ctx, running, cfg.Database); err != nil {
		return nil, err
	}
	if err := restoreInto(ctx, running, cfg.Database, cfg.Database.Name, dump); err != nil {
		return nil, err
	}
	return tableCounts(ctx, running, cfg.Database.Name)
}

// freePort busca un puerto libre de loopback a partir del siguiente al definitivo.
func freePort(base int) (int, error) {
	for port := base + 1; port <= base+40 && port <= 65535; port++ {
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			continue
		}
		listener.Close()
		return port, nil
	}
	return 0, errors.New("no hay un puerto libre para preparar la base nueva")
}

// removeAdoptedCluster deshace lo que creó un paso de datos fallido (la carpeta estaba vacía antes):
// detiene el clúster nuevo y quita su carpeta, la configuración y el marcador del clúster. El respaldo de
// la base anterior y las bitácoras se conservan.
func removeAdoptedCluster(layout Layout, cluster *Cluster, logger *slog.Logger) {
	stopContext, done := context.WithTimeout(context.Background(), 90*time.Second)
	defer done()
	_ = cluster.Stop(stopContext)
	for _, path := range []string{layout.Cluster(), layout.ConfigFile(), layout.Marker()} {
		if err := os.RemoveAll(path); err != nil {
			logger.Error("adopt_cleanup_failed", "path", filepath.Base(path))
		}
	}
}
