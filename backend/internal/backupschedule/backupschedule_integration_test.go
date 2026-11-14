//go:build integration

package backupschedule

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/config"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/maintenance"
)

// clock es una hora simulada que las pruebas adelantan.
type clock struct {
	mutex   sync.Mutex
	current time.Time
}

func (value *clock) now() time.Time {
	value.mutex.Lock()
	defer value.mutex.Unlock()
	return value.current
}

func (value *clock) set(instant time.Time) {
	value.mutex.Lock()
	defer value.mutex.Unlock()
	value.current = instant
}

// testDatabase abre la base temporal (exige un nombre adel_pos_test_*), la deja vacía y la migra.
func testDatabase(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool, dsn
}

// configure crea el establecimiento (Monterrey, hora de inicio del día 05:00).
func configure(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO app.establishment(name, timezone)
        VALUES ('Respaldos de prueba', 'America/Monterrey')`); err != nil {
		t.Fatal(err)
	}
}

// actor crea un usuario y una terminal para «Respaldar ahora».
func actor(t *testing.T, pool *pgxpool.Pool, name string) (string, string) {
	t.Helper()
	ctx := context.Background()
	var role, user, device string
	err := pool.QueryRow(ctx, `INSERT INTO app.roles(code, name) VALUES ($1, $1) RETURNING id::text`, "rol_"+name).Scan(&role)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `INSERT INTO app.users(username, display_name, role_id) VALUES ($1, $1, $2) RETURNING id::text`,
		name, role).Scan(&user)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `INSERT INTO app.devices(name, secret_hash) VALUES ($1, $2) RETURNING id::text`,
		"Terminal "+name, "hash-"+name).Scan(&device)
	if err != nil {
		t.Fatal(err)
	}
	return user, device
}

func monterrey(t *testing.T, year int, month time.Month, day, hour, minute int) time.Time {
	t.Helper()
	return time.Date(year, month, day, hour, minute, 0, 0, mustLocation(t, "America/Monterrey"))
}

func runCount(t *testing.T, pool *pgxpool.Pool, condition string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM app.backup_runs WHERE "+condition).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// withSuffix son los archivos regulares de la carpeta con esa terminación.
func withSuffix(t *testing.T, directory, suffix string) []string {
	t.Helper()
	found := []string{}
	for _, name := range names(t, directory) {
		info, err := os.Lstat(filepath.Join(directory, name))
		if err == nil && info.Mode().IsRegular() && strings.HasSuffix(name, suffix) {
			found = append(found, name)
		}
	}
	return found
}

func saveSettings(t *testing.T, pool *pgxpool.Pool, settings Settings) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := SaveSchedule(ctx, tx, settings, ""); err != nil {
		t.Fatal(err)
	}
	if err := SaveDirectory(ctx, tx, settings.Directory, ""); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// Trabajador con un respaldo simulado: la programación con la hora de inicio del día y la zona del establecimiento,
// el respaldo pendiente al arrancar (la computadora estaba apagada a esa hora), sin repetir al cambiar la hora, la
// retención que solo borra sus archivos, la carpeta no disponible → predeterminada con aviso, el fallo de
// verificación → aviso hasta un respaldo correcto posterior con reintentos espaciados, las ejecuciones interrumpidas
// y los respaldos desactivados.
func TestScheduledBackupsWithSimulatedTarget(t *testing.T) {
	pool, _ := testDatabase(t)
	ctx := context.Background()
	root := t.TempDir()
	target := newFakeTarget(filepath.Join(root, "predeterminada"))
	now := &clock{current: monterrey(t, 2026, 11, 14, 9, 0)}
	runner := &Runner{Pool: pool, Target: target, Now: now.now}
	worker := &Worker{Runner: runner}

	t.Run("sin_establecimiento_no_respalda", func(t *testing.T) {
		plan, err := runner.Plan(ctx, pool)
		if err != nil || plan.Configured || plan.Due {
			t.Fatalf("plan sin establecimiento: %+v %v", plan, err)
		}
		worker.Tick(ctx)
		if runCount(t, pool, "true") != 0 {
			t.Fatal("respaldó antes del asistente inicial")
		}
	})
	configure(t, pool)

	t.Run("pendiente_al_arrancar", func(t *testing.T) {
		plan, err := runner.Plan(ctx, pool)
		if err != nil || !plan.Due || plan.Time != "05:00" || !plan.Slot.Equal(monterrey(t, 2026, 11, 14, 5, 0)) {
			t.Fatalf("plan: %+v %v", plan, err)
		}
		if !plan.Next.Equal(monterrey(t, 2026, 11, 15, 5, 0)) {
			t.Fatalf("siguiente: %v", plan.Next)
		}
		worker.Tick(ctx)
		worker.Tick(ctx)
		if runCount(t, pool, "kind = 'automatic' AND status = 'succeeded' AND verified") != 1 {
			t.Fatal("no hubo exactamente un respaldo automático correcto")
		}
		last, err := LastBackup(ctx, pool, now.now())
		if err != nil || last == nil || last.Location != "default" || last.FolderName != "predeterminada" {
			t.Fatalf("último respaldo: %+v %v", last, err)
		}
		if _, err := os.Stat(filepath.Join(target.directory, last.FileName)); err != nil {
			t.Fatal("no existe el archivo del respaldo")
		}
		var scheduled time.Time
		if err := pool.QueryRow(ctx, "SELECT scheduled_for FROM app.backup_runs").Scan(&scheduled); err != nil ||
			!scheduled.Equal(monterrey(t, 2026, 11, 14, 5, 0)) {
			t.Fatalf("instante programado: %v %v", scheduled, err)
		}
	})

	t.Run("cambiar_la_hora_no_repite", func(t *testing.T) {
		now.set(monterrey(t, 2026, 11, 14, 10, 0))
		saveSettings(t, pool, Settings{Enabled: true, Time: "14:00", Keep: 30})
		worker.Tick(ctx)
		if runCount(t, pool, "true") != 1 {
			t.Fatal("cambiar la hora repitió el respaldo del día")
		}
		now.set(monterrey(t, 2026, 11, 14, 14, 0).Add(30 * time.Second))
		worker.Tick(ctx)
		if runCount(t, pool, "kind = 'automatic' AND status = 'succeeded'") != 2 {
			t.Fatal("no respaldó a la hora nueva")
		}
	})

	t.Run("retencion", func(t *testing.T) {
		manual := FileName(target.database, "0a0b0c0d", KindManual, now.now(), "abcdef")
		touch(t, target.directory, manual)
		touch(t, target.directory, "notas.txt")
		saveSettings(t, pool, Settings{Enabled: true, Time: "14:00", Keep: 2})
		for day := 15; day <= 17; day++ {
			now.set(monterrey(t, 2026, 11, day, 14, 1))
			worker.Tick(ctx)
		}
		files := withSuffix(t, target.directory, "-automatico.dump")
		if len(files) != 2 {
			t.Fatalf("respaldos automáticos conservados: %v", files)
		}
		left := names(t, target.directory)
		if !slices.Contains(left, manual) || !slices.Contains(left, "notas.txt") {
			t.Fatalf("la retención borró archivos ajenos: %v", left)
		}
		if runCount(t, pool, "removed_at IS NOT NULL") != 3 || runCount(t, pool, "status = 'succeeded' AND removed_at IS NULL") != 2 {
			t.Fatal("no se anotaron los respaldos borrados")
		}
		state, err := runner.State(ctx, pool, true)
		if err != nil {
			t.Fatal(err)
		}
		available := 0
		for _, run := range state.Runs {
			if run.Available != nil && *run.Available {
				available++
			}
			if run.Removed && (run.Available == nil || *run.Available) {
				t.Fatalf("un respaldo borrado aparece disponible: %+v", run)
			}
		}
		if available != 2 || state.Alert != nil || !state.DirectoryChangeAvailable {
			t.Fatalf("estado: %d disponibles, aviso %+v", available, state.Alert)
		}
	})

	t.Run("carpeta_no_disponible_usa_la_predeterminada", func(t *testing.T) {
		usb := filepath.Join(root, "usb", "Respaldos")
		saveSettings(t, pool, Settings{Enabled: true, Time: "14:00", Keep: 2, Directory: usb})
		now.set(monterrey(t, 2026, 11, 18, 14, 1))
		worker.Tick(ctx)
		last, err := LastBackup(ctx, pool, now.now())
		if err != nil || last.Status != StatusSucceeded || last.Location != "default" || last.FallbackReason != ReasonMissing {
			t.Fatalf("respaldo en la predeterminada: %+v %v", last, err)
		}
		summary, err := runner.Summary(ctx, pool)
		if err != nil || summary.Alert == nil || summary.Alert.Kind != "fallback" || summary.Alert.Reason != ReasonMissing {
			t.Fatalf("aviso de carpeta no disponible: %+v %v", summary.Alert, err)
		}
		if _, err := os.Stat(usb); err == nil {
			t.Fatal("se creó la carpeta de la USB desconectada")
		}
		// Se conecta la USB: el siguiente respaldo queda en ella y el aviso desaparece.
		if err := os.MkdirAll(usb, 0700); err != nil {
			t.Fatal(err)
		}
		now.set(monterrey(t, 2026, 11, 19, 14, 1))
		worker.Tick(ctx)
		last, err = LastBackup(ctx, pool, now.now())
		if err != nil || last.Location != "custom" || last.FolderName != "Respaldos" || last.Directory != usb {
			t.Fatalf("respaldo en la carpeta elegida: %+v %v", last, err)
		}
		if summary, _ := runner.Summary(ctx, pool); summary.Alert != nil {
			t.Fatalf("el aviso siguió: %+v", summary.Alert)
		}
		saveSettings(t, pool, Settings{Enabled: true, Time: "14:00", Keep: 2})
	})

	t.Run("fallo_de_verificacion_avisa_hasta_un_respaldo_correcto", func(t *testing.T) {
		target.set(func(target *fakeTarget) {
			target.failVerify = true
		})
		now.set(monterrey(t, 2026, 11, 20, 14, 1))
		worker.Tick(ctx)
		summary, err := runner.Summary(ctx, pool)
		if err != nil || summary.Alert == nil || summary.Alert.Kind != "failed" || summary.Alert.Reason != FailureVerify ||
			summary.Alert.Failures != 1 {
			t.Fatalf("aviso de verificación fallida: %+v %v", summary.Alert, err)
		}
		if leftovers := withSuffix(t, target.directory, ".partial"); len(leftovers) != 0 {
			t.Fatalf("quedó el respaldo sin verificar: %v", leftovers)
		}
		// El reintento espera RetryDelay.
		now.set(now.now().Add(10 * time.Minute))
		worker.Tick(ctx)
		if runCount(t, pool, "status = 'failed'") != 1 {
			t.Fatal("reintentó antes de la espera")
		}
		now.set(now.now().Add(RetryDelay))
		worker.Tick(ctx)
		summary, _ = runner.Summary(ctx, pool)
		if summary.Alert == nil || summary.Alert.Failures != 2 || !summary.Alert.Since.Equal(monterrey(t, 2026, 11, 20, 14, 1).UTC()) {
			t.Fatalf("segundo fallo: %+v", summary.Alert)
		}
		target.set(func(target *fakeTarget) {
			target.failVerify = false
		})
		now.set(now.now().Add(RetryDelay))
		worker.Tick(ctx)
		if summary, _ := runner.Summary(ctx, pool); summary.Alert != nil || summary.LastBackup.Status != StatusSucceeded {
			t.Fatalf("un respaldo correcto posterior quita el aviso: %+v", summary)
		}
	})

	t.Run("sin_intentos_restantes", func(t *testing.T) {
		target.set(func(target *fakeTarget) {
			target.failDump = true
		})
		start := monterrey(t, 2026, 11, 21, 14, 1)
		for attempt := 0; attempt < MaxAttempts+2; attempt++ {
			now.set(start.Add(time.Duration(attempt) * (RetryDelay + time.Minute)))
			worker.Tick(ctx)
		}
		if failures := runCount(t, pool, "status = 'failed' AND failure_reason = 'dump_failed'"); failures != MaxAttempts {
			t.Fatalf("intentos del día: %d", failures)
		}
		target.set(func(target *fakeTarget) {
			target.failDump = false
		})
	})

	t.Run("fila_en_curso_huerfana", func(t *testing.T) {
		// Una ejecución que quedó en curso (el servidor se detuvo a media copia o no pudo registrar el resultado), con
		// su archivo temporal registrado; en la carpeta hay además un .partial ajeno con el mismo formato.
		now.set(monterrey(t, 2026, 11, 22, 14, 0))
		saveSettings(t, pool, Settings{Enabled: true, Time: "14:00", Keep: 2})
		own := FileName(target.database, "0a0b0c0d", KindAutomatic, now.now(), "a1a1a1") + partialSuffix
		foreign := FileName(target.database, "ffffffff", KindAutomatic, now.now(), "b2b2b2") + partialSuffix
		touch(t, target.directory, own)
		touch(t, target.directory, foreign)
		var id string
		err := pool.QueryRow(ctx, `INSERT INTO app.backup_runs(id, kind, status, scheduled_for, started_at, directory, partial_file)
            VALUES (gen_random_uuid(), 'automatic', 'running', $1, $1, $2, $3) RETURNING id::text`,
			now.now(), target.directory, own).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		if running, _ := Running(ctx, pool, now.now()); !running {
			t.Fatal("no aparece en curso")
		}
		// Un resto viejo ya no se muestra «en curso» ni detiene la programación.
		if running, _ := Running(ctx, pool, now.now().Add(StaleAfter+time.Minute)); running {
			t.Fatal("una fila en curso vieja aparece en curso")
		}
		now.set(monterrey(t, 2026, 11, 22, 14, 5))
		// La revisión del trabajador la cierra (el lock está libre) y respalda el día.
		worker.Tick(ctx)
		var status, reason string
		var partial *string
		if err := pool.QueryRow(ctx, "SELECT status, failure_reason, partial_file FROM app.backup_runs WHERE id = $1", id).
			Scan(&status, &reason, &partial); err != nil || status != StatusFailed || reason != FailureInterrupted || partial != nil {
			t.Fatalf("interrumpido: %s %s %v %v", status, reason, partial, err)
		}
		left := names(t, target.directory)
		if slices.Contains(left, own) {
			t.Fatal("no se borró el archivo temporal propio")
		}
		if !slices.Contains(left, foreign) {
			t.Fatal("se borró un archivo temporal ajeno")
		}
		if runCount(t, pool, "kind = 'automatic' AND status = 'succeeded' AND started_at >= '2026-11-22 20:00+00'") != 1 {
			t.Fatal("la fila huérfana detuvo el respaldo del día")
		}
	})

	t.Run("desactivados", func(t *testing.T) {
		saveSettings(t, pool, Settings{Enabled: false, Keep: 2})
		now.set(monterrey(t, 2026, 11, 23, 6, 0))
		before := runCount(t, pool, "true")
		worker.Tick(ctx)
		if runCount(t, pool, "true") != before {
			t.Fatal("respaldó con los respaldos automáticos desactivados")
		}
		state, err := runner.State(ctx, pool, false)
		if err != nil || state.NextRunAt != nil || state.Pending || state.Settings.Time != nil || state.Settings.EffectiveTime != "05:00" ||
			state.DirectoryChangeAvailable {
			t.Fatalf("estado desactivado: %+v %v", state, err)
		}
	})
}

// El trabajador real (con sus esperas acortadas) respalda lo pendiente al arrancar y, al apagarse el servidor a media
// copia, la ejecución queda registrada como interrumpida.
func TestWorkerRunsAndStopsInOrder(t *testing.T) {
	pool, _ := testDatabase(t)
	configure(t, pool)
	target := newFakeTarget(filepath.Join(t.TempDir(), "predeterminada"))
	runner := &Runner{Pool: pool, Target: target}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		(&Worker{Runner: runner, StartupDelay: 10 * time.Millisecond, Interval: 20 * time.Millisecond}).Run(ctx)
	}()
	deadline := time.Now().Add(20 * time.Second)
	for runCount(t, pool, "status = 'succeeded'") == 0 {
		if time.Now().After(deadline) {
			t.Fatal("el trabajador no respaldó lo pendiente")
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	if count := runCount(t, pool, "true"); count != 1 {
		t.Fatalf("el trabajador repitió el respaldo: %d", count)
	}
	// Un «Respaldar ahora» a media copia cuando el servidor se apaga queda interrumpido y el trabajador lo espera.
	user, device := actor(t, pool, "apagado")
	started := make(chan struct{}, 1)
	target.set(func(target *fakeTarget) {
		target.block = make(chan struct{})
		target.dumpStarted = started
	})
	if _, err := runner.StartManual(context.Background(), ManualRequest{OperationID: "8b0c3c8e-1d1c-4c55-9c62-6f5d3c2a1b01",
		UserID: user, DeviceID: device}, nil); err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("el trabajador no terminó")
	}
	var status, reason string
	if err := pool.QueryRow(context.Background(), "SELECT status, failure_reason FROM app.backup_runs WHERE kind = 'manual'").
		Scan(&status, &reason); err != nil || status != StatusFailed || reason != FailureInterrupted {
		t.Fatalf("respaldo interrumpido: %s %s %v", status, reason, err)
	}
}

// El lock 7820148309 evita respaldos duplicados: con un respaldo en curso, otro servidor sobre la misma base no
// respalda el mismo instante ni empieza «Respaldar ahora»; muchas revisiones simultáneas dejan un solo respaldo.
func TestLockPreventsDuplicates(t *testing.T) {
	pool, dsn := testDatabase(t)
	configure(t, pool)
	other, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	ctx := context.Background()
	target := newFakeTarget(filepath.Join(t.TempDir(), "predeterminada"))
	first := &Runner{Pool: pool, Target: target}
	second := &Runner{Pool: other, Target: target}
	plan, err := first.Plan(ctx, pool)
	if err != nil || !plan.Due {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	target.set(func(target *fakeTarget) {
		target.block = release
		target.dumpStarted = started
	})
	finished := make(chan error, 1)
	go func() {
		_, err := first.RunScheduled(ctx, plan.Slot)
		finished <- err
	}()
	<-started
	if _, err := second.RunScheduled(ctx, plan.Slot); !errors.Is(err, ErrBusy) {
		t.Fatalf("segundo servidor con un respaldo en curso: %v", err)
	}
	user, device := actor(t, pool, "duplicados")
	if _, err := second.StartManual(ctx, ManualRequest{OperationID: "0d8f4b7e-52a4-4d5e-8a1e-3f0b6f9a2c11",
		UserID: user, DeviceID: device}, nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("«Respaldar ahora» con un respaldo en curso: %v", err)
	}
	var locked bool
	if err := other.QueryRow(ctx, "SELECT pg_try_advisory_lock(7820148309)").Scan(&locked); err != nil || locked {
		t.Fatalf("el lock no estaba tomado: %v %v", locked, err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	target.set(func(target *fakeTarget) {
		target.block = nil
		target.dumpStarted = nil
	})
	if run, err := second.RunScheduled(ctx, plan.Slot); err != nil || run != nil {
		t.Fatalf("el instante ya tenía respaldo: %+v %v", run, err)
	}
	// Muchas revisiones a la vez desde los dos servidores en el instante siguiente: un solo respaldo.
	nextSlot := plan.Next
	clockAt := func() time.Time {
		return nextSlot.Add(time.Minute)
	}
	first.Now, second.Now = clockAt, clockAt
	var wait sync.WaitGroup
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func(runner *Runner) {
			defer wait.Done()
			_, _ = runner.RunScheduled(ctx, nextSlot)
		}([]*Runner{first, second}[index%2])
	}
	wait.Wait()
	if count := runCount(t, pool, "kind = 'automatic'"); count != 2 {
		t.Fatalf("respaldos automáticos: %d", count)
	}
	if target.dumpCount() != 2 {
		t.Fatalf("copias hechas: %d", target.dumpCount())
	}
	// Se soltó el lock al terminar.
	if err := other.QueryRow(ctx, "SELECT pg_try_advisory_lock(7820148309)").Scan(&locked); err != nil || !locked {
		t.Fatalf("el lock quedó tomado: %v %v", locked, err)
	}
	if _, err := other.Exec(ctx, "SELECT pg_advisory_unlock(7820148309)"); err != nil {
		t.Fatal(err)
	}
}

// «Respaldar ahora» es idempotente: la misma clave devuelve la misma ejecución (también con reenvíos simultáneos) sin
// respaldar otra vez, y con otro usuario o terminal es un conflicto.
func TestManualBackupIsIdempotent(t *testing.T) {
	pool, _ := testDatabase(t)
	configure(t, pool)
	ctx := context.Background()
	target := newFakeTarget(filepath.Join(t.TempDir(), "predeterminada"))
	runner := &Runner{Pool: pool, Target: target}
	user, device := actor(t, pool, "idempotente")
	request := ManualRequest{OperationID: "5f1f7c64-7f3e-4b9a-9d1a-0c2e8b7a6d51", UserID: user, DeviceID: device}
	var audited []Run
	var mutex sync.Mutex
	results := make(chan Started, 6)
	errorsFound := make(chan error, 6)
	var wait sync.WaitGroup
	for index := 0; index < 6; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			started, err := runner.StartManual(ctx, request, func(run Run) {
				mutex.Lock()
				defer mutex.Unlock()
				audited = append(audited, run)
			})
			if err != nil {
				errorsFound <- err
				return
			}
			results <- started
		}()
	}
	wait.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("reenvío simultáneo: %v", err)
	}
	ids := map[string]bool{}
	fresh := 0
	for started := range results {
		ids[started.Run.ID] = true
		if !started.Replayed {
			fresh++
			final := <-started.Done
			if final.Status != StatusSucceeded || !final.Verified || final.Kind != KindManual {
				t.Fatalf("respaldo manual: %+v", final)
			}
		}
	}
	runner.Wait()
	if len(ids) != 1 || fresh != 1 || runCount(t, pool, "kind = 'manual'") != 1 || target.dumpCount() != 1 {
		t.Fatalf("la misma clave respaldó más de una vez: %v %d", ids, fresh)
	}
	if len(audited) != 1 {
		t.Fatalf("auditorías: %d", len(audited))
	}
	again, err := runner.StartManual(ctx, request, nil)
	if err != nil || !again.Replayed || again.Run.Status != StatusSucceeded || !ids[again.Run.ID] {
		t.Fatalf("reenvío posterior: %+v %v", again, err)
	}
	other, otherDevice := actor(t, pool, "otro")
	if _, err := runner.StartManual(ctx, ManualRequest{OperationID: request.OperationID, UserID: other, DeviceID: otherDevice}, nil); !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("otro usuario con la misma clave: %v", err)
	}
	if _, err := runner.StartManual(ctx, ManualRequest{OperationID: request.OperationID, UserID: user, DeviceID: otherDevice}, nil); !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("otra terminal con la misma clave: %v", err)
	}
	// Los manuales no cuentan para la retención ni para el respaldo del día.
	files := names(t, target.directory)
	if len(files) != 1 || !strings.HasSuffix(files[0], "-manual.dump") {
		t.Fatalf("archivo manual: %v", files)
	}
	plan, err := runner.Plan(ctx, pool)
	if err != nil || !plan.Due {
		t.Fatal("un manual no sustituye al automático del día")
	}
}

// Respaldo real contra el clúster aislado: pg_dump y pg_restore de PostgreSQL 18 con la conexión de la base temporal,
// en carpetas dentro del directorio temporal; el archivo queda privado, se verifica de verdad y la retención borra
// solo los automáticos.
func TestRealBackupAgainstPostgres(t *testing.T) {
	pool, dsn := testDatabase(t)
	configure(t, pool)
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	sslMode := "disable"
	if value := parsed.RuntimeParams["sslmode"]; value != "" {
		sslMode = value
	}
	root := t.TempDir()
	cfg := config.Config{
		Database: config.Database{
			Host:     parsed.Host,
			Port:     int(parsed.Port),
			Name:     parsed.Database,
			User:     parsed.User,
			Password: parsed.Password,
			SSLMode:  sslMode,
		},
		BackupDirectory: filepath.Join(root, "backups"),
	}
	target := maintenance.NewDumpBackup(cfg, filepath.Join(root, ".local", "server.json"))
	if _, err := os.Stat(filepath.Join(target.PGBin, "pg_dump")); err != nil {
		t.Skip("no hay PostgreSQL 18 en este equipo para el respaldo real")
	}
	ctx := context.Background()
	now := &clock{current: time.Now()}
	runner := &Runner{Pool: pool, Target: target, Now: now.now}
	user, device := actor(t, pool, "real")
	started, err := runner.StartManual(ctx, ManualRequest{OperationID: "2a7c8d1e-9b3f-4e6a-8c5d-1f0e2d3c4b51", UserID: user, DeviceID: device}, nil)
	if err != nil {
		t.Fatal(err)
	}
	manual := <-started.Done
	if manual.Status != StatusSucceeded || !manual.Verified || manual.SizeBytes == 0 {
		t.Fatalf("respaldo manual real: %+v", manual)
	}
	filename := filepath.Join(manual.Directory, manual.FileName)
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("permisos: %v", info.Mode().Perm())
	}
	listing, err := exec.Command(filepath.Join(target.PGBin, "pg_restore"), "--list", filename).Output()
	if err != nil || !strings.Contains(string(listing), "TABLE DATA app backup_runs") {
		t.Fatalf("catálogo del respaldo real: %v", err)
	}
	// Automáticos en una carpeta elegida, con retención de 2.
	chosen := filepath.Join(root, "disco-externo")
	if err := os.Mkdir(chosen, 0700); err != nil {
		t.Fatal(err)
	}
	saveSettings(t, pool, Settings{Enabled: true, Keep: 2, Directory: chosen})
	for day := 0; day < 3; day++ {
		now.set(now.now().Add(24 * time.Hour))
		plan, err := runner.Plan(ctx, pool)
		if err != nil || !plan.Due {
			t.Fatalf("plan del día %d: %+v %v", day, plan, err)
		}
		run, err := runner.RunScheduled(ctx, plan.Slot)
		if err != nil || run == nil || run.Status != StatusSucceeded || run.Directory != chosen {
			t.Fatalf("respaldo automático real: %+v %v", run, err)
		}
	}
	files := withSuffix(t, chosen, "-automatico.dump")
	if len(files) != 2 {
		t.Fatalf("retención real: %v", files)
	}
	if _, err := os.Stat(filename); err != nil {
		t.Fatal("la retención borró el respaldo manual")
	}
	// La carpeta elegida desaparece (se desconectó el disco): el respaldo queda en la predeterminada.
	if err := os.RemoveAll(chosen); err != nil {
		t.Fatal(err)
	}
	now.set(now.now().Add(24 * time.Hour))
	plan, err := runner.Plan(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	run, err := runner.RunScheduled(ctx, plan.Slot)
	if err != nil || run == nil || run.Status != StatusSucceeded || !run.UsedDefault || run.FallbackReason != ReasonMissing ||
		run.Directory != target.DefaultDirectory() {
		t.Fatalf("respaldo real en la predeterminada: %+v %v", run, err)
	}
	if err := target.Verify(ctx, filepath.Join(run.Directory, run.FileName)); err != nil {
		t.Fatalf("verificación real: %v", err)
	}
}

// Revisión de la fase F (hallazgo 1): en una carpeta compartida (una USB o una carpeta de red que usan otra
// computadora u otra sucursal) la retención solo borra los respaldos automáticos que esta instalación registró, en el
// orden en que los registró; nunca los de otra instalación aunque tengan el mismo formato y fechas posteriores, nunca
// un .partial ajeno y nunca el respaldo recién hecho. Los nombres llevan el identificador de la instalación.
func TestSharedFolderKeepsOtherInstallationsFiles(t *testing.T) {
	pool, _ := testDatabase(t)
	configure(t, pool)
	ctx := context.Background()
	root := t.TempDir()
	shared := filepath.Join(root, "usb", "Respaldos")
	if err := os.MkdirAll(shared, 0700); err != nil {
		t.Fatal(err)
	}
	target := newFakeTarget(filepath.Join(root, "predeterminada"))
	foreign := []string{
		// Otra instalación con el formato nuevo y fechas posteriores (o un reloj adelantado).
		target.database + "-ffffffff-20301201T110000Z-cccccc-automatico.dump",
		target.database + "-ffffffff-20301202T110000Z-dddddd-automatico.dump",
		// Una instalación anterior a esta revisión (sin identificador) y un respaldo a medias ajeno.
		target.database + "-20301203T110000Z-eeeeee-automatico.dump",
		target.database + "-ffffffff-20301204T110000Z-aaaaaa-automatico.dump.partial",
		target.database + "-ffffffff-20301204T110000Z-bbbbbb-manual.dump",
	}
	for _, name := range foreign {
		touch(t, shared, name)
	}
	now := &clock{current: monterrey(t, 2026, 11, 14, 9, 0)}
	runner := &Runner{Pool: pool, Target: target, Now: now.now, InstallationID: "3f2a9c1b-0d4e-4f5a-8b6c-7d8e9f0a1b2c"}
	worker := &Worker{Runner: runner}
	saveSettings(t, pool, Settings{Enabled: true, Keep: 2, Directory: shared})
	newest := ""
	for day := 14; day <= 17; day++ {
		now.set(monterrey(t, 2026, 11, day, 9, 0))
		worker.Tick(ctx)
		last, err := LastBackup(ctx, pool, now.now())
		if err != nil || last == nil || last.Status != StatusSucceeded || last.Directory != shared {
			t.Fatalf("respaldo del día %d: %+v %v", day, last, err)
		}
		if !strings.Contains(last.FileName, "-3f2a9c1b-") {
			t.Fatalf("el nombre no lleva el identificador de la instalación: %s", last.FileName)
		}
		newest = last.FileName
		left := names(t, shared)
		if !slices.Contains(left, newest) {
			t.Fatalf("la retención borró el respaldo recién hecho: %v", left)
		}
		for _, name := range foreign {
			if !slices.Contains(left, name) {
				t.Fatalf("la retención borró un archivo de otra instalación (%s): %v", name, left)
			}
		}
		if own := withSuffix(t, shared, "-automatico.dump"); len(own) > 2+3 {
			t.Fatalf("archivos automáticos: %v", own)
		}
	}
	own := []string{}
	for _, name := range withSuffix(t, shared, "-automatico.dump") {
		if strings.Contains(name, "-3f2a9c1b-") {
			own = append(own, name)
		}
	}
	if len(own) != 2 || !slices.Contains(own, newest) {
		t.Fatalf("respaldos propios conservados: %v", own)
	}
	if runCount(t, pool, "removed_at IS NOT NULL") != 2 {
		t.Fatal("no se anotaron los respaldos propios borrados")
	}
	// Sin identidad del servidor, el nombre lleva el installation_id de la base.
	var installation string
	if err := pool.QueryRow(ctx, "SELECT installation_id::text FROM app.backup_settings").Scan(&installation); err != nil {
		t.Fatal(err)
	}
	runner.InstallationID = ""
	now.set(monterrey(t, 2026, 11, 18, 9, 0))
	worker.Tick(ctx)
	last, err := LastBackup(ctx, pool, now.now())
	if err != nil || !strings.Contains(last.FileName, "-"+InstallationTag(installation)+"-") {
		t.Fatalf("nombre sin identidad: %+v %v", last, err)
	}
}

// Revisión de la fase F (hallazgo 3): un respaldo hecho con el reloj adelantado no detiene la programación ni oculta un
// fallo posterior al corregir el reloj; sin respaldo correcto en más de un día y dos horas hay aviso «atrasado».
func TestClockAheadAndOverdueAlert(t *testing.T) {
	pool, _ := testDatabase(t)
	configure(t, pool)
	ctx := context.Background()
	target := newFakeTarget(filepath.Join(t.TempDir(), "predeterminada"))
	real := time.Now()
	now := &clock{current: real.Add(5 * 24 * time.Hour)}
	runner := &Runner{Pool: pool, Target: target, Now: now.now, InstallationID: "3f2a9c1b-0d4e-4f5a-8b6c-7d8e9f0a1b2c"}
	worker := &Worker{Runner: runner}
	// Con el reloj adelantado cinco días se hace un respaldo correcto.
	worker.Tick(ctx)
	if runCount(t, pool, "status = 'succeeded'") != 1 {
		t.Fatal("no se respaldó con el reloj adelantado")
	}
	// Se corrige el reloj (un día y tres horas después de configurar): ese respaldo «del futuro» no cuenta.
	now.set(real.Add(27 * time.Hour))
	summary, err := runner.Summary(ctx, pool)
	if err != nil || summary.Alert == nil || summary.Alert.Kind != "overdue" || summary.Alert.Reason != "never" ||
		summary.LastBackup != nil {
		t.Fatalf("aviso de respaldo atrasado: %+v %v", summary, err)
	}
	plan, err := runner.Plan(ctx, pool)
	if err != nil || !plan.Due {
		t.Fatalf("el respaldo del futuro detuvo la programación: %+v %v", plan, err)
	}
	// Un fallo después de corregir el reloj se avisa (el orden es el de la base, no el de las fechas).
	target.set(func(target *fakeTarget) {
		target.failVerify = true
	})
	worker.Tick(ctx)
	summary, _ = runner.Summary(ctx, pool)
	if summary.Alert == nil || summary.Alert.Kind != "failed" || summary.LastBackup == nil || summary.LastBackup.Status != StatusFailed {
		t.Fatalf("fallo después de un respaldo del futuro: %+v", summary)
	}
	target.set(func(target *fakeTarget) {
		target.failVerify = false
	})
	now.set(now.now().Add(RetryDelay + time.Minute))
	worker.Tick(ctx)
	if summary, _ := runner.Summary(ctx, pool); summary.Alert != nil || summary.LastBackup.Status != StatusSucceeded {
		t.Fatalf("un respaldo correcto quita el aviso: %+v", summary)
	}
	// Dos días sin respaldo correcto (la computadora apagada a la hora del respaldo y al encenderla falla todo lo
	// demás): aviso «atrasado» con la fecha del último correcto.
	lastSuccess, _ := LastBackup(ctx, pool, now.now())
	now.set(now.now().Add(30 * time.Hour))
	summary, _ = runner.Summary(ctx, pool)
	if summary.Alert == nil || summary.Alert.Kind != "overdue" || summary.Alert.Reason != "stale" ||
		!summary.Alert.Since.Equal(lastSuccess.StartedAt) {
		t.Fatalf("atrasado con un respaldo anterior: %+v", summary.Alert)
	}
	// Desactivados no avisan atraso; al activarlos de nuevo se mide desde entonces.
	saveSettings(t, pool, Settings{Enabled: false, Keep: 30})
	if summary, _ := runner.Summary(ctx, pool); summary.Alert != nil {
		t.Fatalf("desactivados: %+v", summary.Alert)
	}
	saveSettings(t, pool, Settings{Enabled: true, Keep: 30})
	now.set(time.Now())
	if summary, _ := runner.Summary(ctx, pool); summary.Alert != nil {
		t.Fatalf("recién activados: %+v", summary.Alert)
	}
}

// Fallo de verificación: el respaldo que no se puede leer no se conserva (ni con su nombre definitivo ni a medias) y
// el resultado es «verify_failed»; un respaldo correcto queda verificado con su nombre definitivo. El archivo temporal
// queda registrado antes de crearse.
func TestProduceVerifiesBeforeKeepingTheFile(t *testing.T) {
	pool, _ := testDatabase(t)
	configure(t, pool)
	ctx := context.Background()
	target := newFakeTarget(filepath.Join(t.TempDir(), "respaldos"))
	runner := &Runner{Pool: pool, Target: target, InstallationID: "3f2a9c1b-0d4e-4f5a-8b6c-7d8e9f0a1b2c", Now: func() time.Time {
		return time.Date(2026, 11, 14, 11, 0, 0, 0, time.UTC)
	}}
	start := func() Run {
		t.Helper()
		slot := time.Date(2026, 11, 14, 11, 0, 0, 0, time.UTC)
		run, err := runner.insertRun(ctx, Run{Kind: KindAutomatic, ScheduledFor: &slot, StartedAt: runner.now()})
		if err != nil {
			t.Fatal(err)
		}
		return run
	}
	run := start()
	result := runner.produce(ctx, run, Settings{Keep: 30})
	if result.failure != "" || !result.verified || result.size == 0 || !strings.Contains(result.fileName, "-3f2a9c1b-") {
		t.Fatalf("respaldo correcto: %+v", result)
	}
	if left := names(t, target.directory); !slices.Equal(left, []string{result.fileName}) {
		t.Fatalf("archivos: %v", left)
	}
	if !slices.Contains(target.verified, result.fileName+partialSuffix) {
		t.Fatalf("se verificó antes de darle su nombre definitivo: %v", target.verified)
	}
	var recorded string
	if err := pool.QueryRow(ctx, "SELECT partial_file FROM app.backup_runs WHERE id = $1", run.ID).Scan(&recorded); err != nil ||
		recorded != result.fileName+partialSuffix {
		t.Fatalf("archivo temporal registrado: %q %v", recorded, err)
	}
	final := runner.finish(run, result)
	if final.PartialFile != "" || final.Status != StatusSucceeded {
		t.Fatalf("al terminar se borra el registro del temporal: %+v", final)
	}
	target.set(func(target *fakeTarget) {
		target.failVerify = true
	})
	failed := runner.produce(ctx, start(), Settings{Keep: 30})
	if failed.failure != FailureVerify || failed.verified || failed.fileName != "" {
		t.Fatalf("verificación fallida: %+v", failed)
	}
	if left := names(t, target.directory); !slices.Equal(left, []string{result.fileName}) {
		t.Fatalf("quedó el respaldo que no se pudo verificar: %v", left)
	}
	target.set(func(target *fakeTarget) {
		target.failVerify = false
		target.failDump = true
	})
	if dump := runner.produce(ctx, start(), Settings{Keep: 30}); dump.failure != FailureDump {
		t.Fatalf("pg_dump fallido: %+v", dump)
	}
	target.set(func(target *fakeTarget) {
		target.failDump = false
		target.block = make(chan struct{})
	})
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if interrupted := runner.produce(canceled, start(), Settings{Keep: 30}); interrupted.failure != FailureInterrupted {
		t.Fatalf("interrumpido: %+v", interrupted)
	}
	if left := names(t, target.directory); len(left) != 1 {
		t.Fatalf("quedaron restos: %v", left)
	}
}
