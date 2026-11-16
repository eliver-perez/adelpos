package legacyimport

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"
)

// StatusView es el estado de la migración para el asistente.
type StatusView struct {
	MaxUploadBytes int64        `json:"max_upload_bytes"`
	PartSize       int64        `json:"part_size"`
	CurrentBatch   *BatchView   `json:"current_batch"`
	ActiveRun      *RunView     `json:"active_run"`
	RecentRuns     []RunView    `json:"recent_runs"`
	Uploads        []UploadView `json:"uploads"`
}

// Status lee el estado: lote vigente, corrida activa, últimas corridas y las subidas abiertas de quien pregunta.
func (manager *Manager) Status(ctx context.Context, tx pgx.Tx, actor Actor) (StatusView, error) {
	status := StatusView{MaxUploadBytes: manager.maxUploadBytes(), PartSize: PartSize, Uploads: manager.ListUploads(actor)}
	current, err := CurrentBatchID(ctx, tx)
	if err != nil {
		return status, err
	}
	if current != "" {
		if status.CurrentBatch, err = loadBatchView(ctx, tx, current); err != nil {
			return status, err
		}
	}
	if status.ActiveRun, err = ActiveRun(ctx, tx); err != nil {
		return status, err
	}
	status.RecentRuns, err = RecentRuns(ctx, tx, 10)
	return status, err
}

// ListUploads son las subidas abiertas del usuario y la terminal, de la más reciente a la más antigua.
func (manager *Manager) ListUploads(actor Actor) []UploadView {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	manager.ensureState()
	views := []UploadView{}
	for _, entry := range manager.uploads {
		if entry.userID == actor.UserID && entry.deviceID == actor.DeviceID {
			views = append(views, entry.view)
		}
	}
	sort.Slice(views, func(left, right int) bool {
		return views[left].CreatedAt.After(views[right].CreatedAt)
	})
	return views
}

// BatchDetailView es un lote con su informe y su validación.
type BatchDetailView struct {
	Batch      BatchView       `json:"batch"`
	Report     json.RawMessage `json:"report"`
	Validation json.RawMessage `json:"validation"`
}

// BatchDetail lee un lote.
func BatchDetail(ctx context.Context, tx pgx.Tx, id string) (*BatchDetailView, error) {
	batch, err := loadBatchView(ctx, tx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &Error{Code: CodeUploadMissing, Message: "El lote no existe."}
	}
	if err != nil {
		return nil, err
	}
	detail := &BatchDetailView{Batch: *batch}
	var report, validation []byte
	if err := tx.QueryRow(ctx, `SELECT report, validation FROM app.legacy_import_batches WHERE id = $1`, id).
		Scan(&report, &validation); err != nil {
		return nil, err
	}
	detail.Report, detail.Validation = json.RawMessage("null"), json.RawMessage("null")
	if report != nil {
		detail.Report = report
	}
	if validation != nil {
		detail.Validation = validation
	}
	return detail, nil
}
