package administration

import (
	"context"
	"time"
)

// CreateLocalEnrollment recupera una terminal desde una sesión local autorizada del sistema operativo.
// No autentica usuarios ni devuelve privilegios; el código es temporal y de un solo uso.
func (service *Service) CreateLocalEnrollment(ctx context.Context) (string, error) {
	tx, err := service.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())
	code := NewToken()
	var enrollmentID string
	expires := time.Now().Add(10 * time.Minute)
	err = tx.QueryRow(ctx, "INSERT INTO app.enrollments(code_hash,expires_at) VALUES ($1,$2) RETURNING id", TokenHash(code), expires).Scan(&enrollmentID)
	if err != nil {
		return "", err
	}
	if err := audit(ctx, tx, Principal{}, "device.local_enrollment_issued", "enrollment", &enrollmentID, nil, map[string]any{"expires_at": expires}, true); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return code, nil
}
