package backupschedule

import (
	"context"
	"errors"
	"time"

	"local/ad-el-pos/server/internal/businessday"
)

// DefaultStartupDelay es la espera al arrancar el servidor antes de la primera revisión: deja que termine de arrancar
// la computadora (y que se monte un disco externo o una USB) y que las terminales se reconecten antes de respaldar lo
// que quedó pendiente mientras estuvo apagada.
const DefaultStartupDelay = 2 * time.Minute

// DefaultInterval es cada cuánto revisa el trabajador si toca respaldar.
const DefaultInterval = 30 * time.Second

// Worker hace el respaldo automático diario. Arranca con el servidor, como los trabajadores de cocina, inventario y
// «Agotado» (main.go), y se detiene con su contexto: un respaldo en curso se interrumpe y queda registrado.
type Worker struct {
	Runner *Runner
	// StartupDelay y Interval cambian las esperas (las pruebas las acortan); cero usa los valores por omisión.
	StartupDelay time.Duration
	Interval     time.Duration
}

// Run revisa hasta que se cancela el contexto y espera a que terminen los respaldos de «Respaldar ahora» en curso.
func (worker *Worker) Run(ctx context.Context) {
	runner := worker.Runner
	runner.bind(ctx)
	defer runner.Wait()
	if err := runner.RecoverInterrupted(ctx); err != nil && ctx.Err() == nil {
		runner.logWarning("backup_recovery_unavailable", "reason", err.Error())
	}
	delay := worker.StartupDelay
	if delay == 0 {
		delay = DefaultStartupDelay
	}
	interval := worker.Interval
	if interval == 0 {
		interval = DefaultInterval
	}
	startup := time.NewTimer(delay)
	defer startup.Stop()
	select {
	case <-ctx.Done():
		return
	case <-startup.C:
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		worker.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Tick revisa una vez si toca el respaldo automático y, si toca, lo hace.
func (worker *Worker) Tick(ctx context.Context) {
	runner := worker.Runner
	// Una fila «en curso» sin un respaldo vivo (el servidor se detuvo o no pudo registrar el resultado) se cierra como
	// interrumpida en cuanto el lock está libre: así no deja la programación ni la pantalla detenidas.
	if running, err := anyRunning(ctx, runner.Pool); err == nil && running {
		if err := runner.RecoverInterrupted(ctx); err != nil && ctx.Err() == nil {
			runner.logWarning("backup_recovery_unavailable", "reason", err.Error())
		}
	}
	plan, err := runner.Plan(ctx, runner.Pool)
	if err != nil {
		if ctx.Err() == nil {
			runner.logWarning("backup_schedule_unavailable", "reason", err.Error())
		}
		return
	}
	if !plan.Due {
		return
	}
	if _, err := runner.RunScheduled(ctx, plan.Slot); err != nil && !errors.Is(err, ErrBusy) && ctx.Err() == nil {
		runner.logWarning("backup_scheduled_unavailable", "reason", err.Error())
	}
}

// Plan es la programación vigente: la hora, el último instante programado y el siguiente, y si toca respaldar.
type Plan struct {
	// Configured: el establecimiento ya existe (antes del asistente inicial no hay nada que respaldar).
	Configured bool
	Settings   Settings
	// Cutoff es la «Hora de inicio del día» y Time la hora efectiva del respaldo (HH:MM).
	Cutoff string
	Time   string
	Slot   time.Time
	Next   time.Time
	Due    bool
}

// Plan calcula la programación con la configuración, la zona horaria del establecimiento y la hora de inicio del
// día.
func (runner *Runner) Plan(ctx context.Context, querier Querier) (Plan, error) {
	settings, err := LoadSettings(ctx, querier)
	if err != nil {
		return Plan{}, err
	}
	day, err := businessday.Load(ctx, querier)
	if err != nil {
		return Plan{}, err
	}
	var configured bool
	if err := querier.QueryRow(ctx, "SELECT EXISTS(SELECT FROM app.establishment)").Scan(&configured); err != nil {
		return Plan{}, err
	}
	hour, minute := day.Hour, day.Minute
	if settings.Time != "" {
		hour, minute, err = ParseTime(settings.Time)
		if err != nil {
			return Plan{}, err
		}
	}
	now := runner.now()
	plan := Plan{
		Configured: configured,
		Settings:   settings,
		Cutoff:     day.Cutoff(),
		Time:       businessday.Config{Hour: hour, Minute: minute}.Cutoff(),
		Slot:       LastSlot(now, day.Location, hour, minute),
		Next:       NextSlot(now, day.Location, hour, minute),
	}
	if !configured || !settings.Enabled {
		return plan, nil
	}
	history, err := slotHistory(ctx, querier, plan.Slot, now)
	if err != nil {
		return Plan{}, err
	}
	plan.Due = Due(now, history)
	return plan, nil
}
