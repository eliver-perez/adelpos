package reports

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ejecución aislada de las consultas de reportes (§0-B «Rendimiento»). La autenticación y los
// permisos ocurren antes, en la transacción corta habitual de administration, que se confirma; la
// consulta corre después en otra transacción:
//   - REPEATABLE READ READ ONLY: una sola fotografía para encabezado, detalle, totales y estado de
//     los turnos, y cualquier intento de escritura falla;
//   - statement_timeout de 30 s, lock_timeout de 2 s (solo toma AccessShareLock; protege contra una
//     migración o DDL en curso) e idle_in_transaction_session_timeout de 45 s;
//   - sin locks consultivos ni handlers de otros módulos (solo SQL propio y funciones puras);
//   - sobre un semáforo propio de Slots conexiones del pool compartido: si están ocupadas responde
//     de inmediato 503 REPORTS_BUSY en lugar de esperar, para que un reporte de un año nunca acapare
//     el pool que usan cobros, captura, cocina y SSE.

// Slots es el número de conexiones reservadas para reportes.
const Slots = 2

// Runner ejecuta lecturas de reportes con su semáforo.
type Runner struct {
	pool  *pgxpool.Pool
	slots chan struct{}
	// logo es la fuente del logotipo del restaurante para el encabezado de las exportaciones
	// (export_identity.go).
	logo LogoSource
}

// NewRunner crea un ejecutor con size conexiones simultáneas y sin logotipo del restaurante.
func NewRunner(pool *pgxpool.Pool, size int) *Runner {
	if size < 1 {
		size = 1
	}
	return &Runner{pool: pool, slots: make(chan struct{}, size), logo: NoRestaurantLogo}
}

// UseLogoSource fija la fuente del logotipo del restaurante (nil vuelve a NoRestaurantLogo) y
// devuelve el mismo ejecutor. Se llama al crearlo, antes de atender peticiones.
func (runner *Runner) UseLogoSource(source LogoSource) *Runner {
	if source == nil {
		source = NoRestaurantLogo
	}
	runner.logo = source
	return runner
}

func (runner *Runner) logoSource() LogoSource {
	if runner.logo == nil {
		return NoRestaurantLogo
	}
	return runner.logo
}

// Acquire toma un lugar del semáforo sin esperar. ok es false si están todos ocupados; release
// libera el lugar (las exportaciones de la fase C lo usan alrededor de su generación).
func (runner *Runner) Acquire() (release func(), ok bool) {
	select {
	case runner.slots <- struct{}{}:
		return func() { <-runner.slots }, true
	default:
		return nil, false
	}
}

// Read ejecuta read dentro de una transacción de solo lectura con fotografía consistente.
func (runner *Runner) Read(ctx context.Context, read func(tx pgx.Tx) error) error {
	release, ok := runner.Acquire()
	if !ok {
		return busy()
	}
	defer release()
	return runner.read(ctx, read)
}

func (runner *Runner) read(ctx context.Context, read func(tx pgx.Tx) error) error {
	tx, err := runner.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `SET LOCAL statement_timeout = '30s';
        SET LOCAL lock_timeout = '2s';
        SET LOCAL idle_in_transaction_session_timeout = '45s'`); err != nil {
		return err
	}
	if err := read(tx); err != nil {
		return translateDatabaseError(err)
	}
	return tx.Commit(ctx)
}

// translateDatabaseError convierte la cancelación por tiempo en un error público.
func translateDatabaseError(err error) error {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && (databaseError.Code == "57014" || databaseError.Code == "55P03") {
		return timedOut()
	}
	return err
}
