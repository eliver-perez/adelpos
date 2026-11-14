package backupschedule

import (
	"context"
	"time"
)

// RunsShown es cuántas ejecuciones recientes muestra Mantenimiento.
const RunsShown = 20

// DirectoryView describe una carpeta para la pantalla: nombre visible y si está disponible. Path (la ruta completa)
// solo aparece en «Información para soporte».
type DirectoryView struct {
	Custom     bool   `json:"custom"`
	Path       string `json:"path"`
	FolderName string `json:"folder_name"`
	VolumeName string `json:"volume_name"`
	Available  bool   `json:"available"`
}

// SettingsView es la configuración tal como la muestra Mantenimiento.
type SettingsView struct {
	Enabled bool `json:"enabled"`
	// Time es la hora elegida o null si sigue la «Hora de inicio del día» (BusinessDayCutoff).
	Time              *string       `json:"time"`
	EffectiveTime     string        `json:"effective_time"`
	BusinessDayCutoff string        `json:"business_day_cutoff"`
	Keep              int           `json:"keep"`
	Directory         DirectoryView `json:"directory"`
	DefaultDirectory  DirectoryView `json:"default_directory"`
}

// State es la sección «Respaldos» de Sistema › Mantenimiento.
type State struct {
	Settings SettingsView `json:"settings"`
	// NextRunAt es el siguiente instante programado (null con los respaldos desactivados); Pending indica que el
	// respaldo del último instante programado todavía no se hace (se hará en unos minutos).
	NextRunAt   *time.Time `json:"next_run_at"`
	Pending     bool       `json:"pending"`
	Running     bool       `json:"running"`
	LastBackup  *RunView   `json:"last_backup"`
	LastSuccess *RunView   `json:"last_success"`
	Alert       *Alert     `json:"alert"`
	Runs        []RunView  `json:"runs"`
	// DirectoryChangeAvailable: la carpeta solo se elige desde la computadora principal (conexión local).
	DirectoryChangeAvailable bool `json:"directory_change_available"`
}

// Summary es lo que necesitan el aviso de la pantalla de inicio y el estado del sistema.
type Summary struct {
	LastBackup *RunView `json:"last_backup"`
	Alert      *Alert   `json:"alert"`
	Running    bool     `json:"running"`
}

// directoryView describe la carpeta configurada o la predeterminada. Solo comprueba que exista (sin escribir en ella):
// la prueba de escritura se hace al elegirla y en cada respaldo.
func directoryView(path string, custom bool) DirectoryView {
	view := DirectoryView{Custom: custom, Path: path}
	if path == "" {
		return view
	}
	view.FolderName, view.VolumeName = Describe(path)
	reason := CheckExists(path)
	view.Available = reason == ""
	return view
}

// State arma la sección «Respaldos». local indica si la petición llega de la propia computadora principal.
func (runner *Runner) State(ctx context.Context, querier Querier, local bool) (State, error) {
	plan, err := runner.Plan(ctx, querier)
	if err != nil {
		return State{}, err
	}
	settings := plan.Settings
	state := State{
		Settings: SettingsView{
			Enabled:           settings.Enabled,
			EffectiveTime:     plan.Time,
			BusinessDayCutoff: plan.Cutoff,
			Keep:              settings.Keep,
			Directory:         directoryView(settings.Directory, settings.Directory != ""),
			DefaultDirectory:  directoryView(runner.Target.DefaultDirectory(), false),
		},
		Pending:                  plan.Due,
		DirectoryChangeAvailable: local,
		Runs:                     []RunView{},
	}
	if settings.Time != "" {
		value := settings.Time
		state.Settings.Time = &value
	}
	if settings.Enabled {
		next := plan.Next.UTC()
		state.NextRunAt = &next
	}
	if state.Running, err = Running(ctx, querier); err != nil {
		return State{}, err
	}
	if state.LastBackup, err = LastBackup(ctx, querier); err != nil {
		return State{}, err
	}
	success, err := lastRun(ctx, querier, "status = 'succeeded'")
	if err != nil {
		return State{}, err
	}
	if success != nil {
		view := View(*success, true)
		state.LastSuccess = &view
	}
	if state.Alert, err = ComputeAlert(ctx, querier, settings.Directory); err != nil {
		return State{}, err
	}
	runs, err := ListRuns(ctx, querier, RunsShown)
	if err != nil {
		return State{}, err
	}
	for _, run := range runs {
		state.Runs = append(state.Runs, View(run, true))
	}
	return state, nil
}

// SummaryOf arma el resumen del último respaldo y el aviso.
func SummaryOf(ctx context.Context, querier Querier) (Summary, error) {
	settings, err := LoadSettings(ctx, querier)
	if err != nil {
		return Summary{}, err
	}
	var summary Summary
	if summary.LastBackup, err = LastBackup(ctx, querier); err != nil {
		return Summary{}, err
	}
	if summary.Alert, err = ComputeAlert(ctx, querier, settings.Directory); err != nil {
		return Summary{}, err
	}
	if summary.Running, err = Running(ctx, querier); err != nil {
		return Summary{}, err
	}
	return summary, nil
}
