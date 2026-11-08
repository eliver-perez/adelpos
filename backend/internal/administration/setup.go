package administration

import (
	"context"
	"crypto/subtle"
	"errors"
	"os"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5"
)

type EstablishmentInput struct {
	Name         string `json:"name"`
	Timezone     string `json:"timezone"`
	Currency     string `json:"currency"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	Address      string `json:"address"`
	Theme        string `json:"theme"`
	TerminalName string `json:"terminal_name,omitempty"`
}

func (input *EstablishmentInput) validate() error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Currency == "" {
		input.Currency = "MXN"
	}
	if input.Theme == "" {
		input.Theme = "light"
	}
	if len(input.Name) < 1 || len(input.Name) > 100 || input.Currency != "MXN" {
		return invalid("Indica un nombre de hasta 100 caracteres y moneda MXN.")
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return invalid("La zona horaria debe ser un identificador IANA válido.")
	}
	if input.Theme != "light" && input.Theme != "dark" {
		return invalid("El tema debe ser light o dark.")
	}
	if len(input.Phone) > 100 || len(input.Email) > 200 || len(input.Address) > 500 {
		return invalid("Los datos de contacto exceden la longitud permitida.")
	}
	return nil
}

func (service *Service) setup(ctx context.Context, tx pgx.Tx, request Request) (any, error) {
	secret, err := os.ReadFile(service.ProvisioningFile)
	expected := strings.TrimSpace(string(secret))
	if err != nil || !request.Local || len(expected) != 43 || subtle.ConstantTimeCompare([]byte(expected), []byte(request.ProvisioningToken)) != 1 {
		return nil, denied()
	}
	// Este lock hace idempotente el primer arranque incluso si dos ventanas envían el asistente.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7820148302)"); err != nil {
		return nil, err
	}
	var configured bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM app.establishment)").Scan(&configured); err != nil {
		return nil, err
	}
	if configured {
		return map[string]bool{"configured": true, "already_configured": true}, nil
	}
	var input EstablishmentInput
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.TerminalName) == "" || len(input.TerminalName) > 80 {
		return nil, invalid("Indica el nombre de esta terminal (máximo 80 caracteres).")
	}
	if err := ensureFoundations(ctx, tx); err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.establishment(name,timezone,currency,phone,email,address,theme)
        VALUES ($1,$2,$3,$4,$5,$6,$7)`, input.Name, input.Timezone, input.Currency, input.Phone, input.Email, input.Address, input.Theme)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.role_permissions(role_id,permission_key)
        SELECT r.id,p.key FROM app.roles r CROSS JOIN app.permissions p WHERE r.code='administrator'
        ON CONFLICT DO NOTHING`)
	if err != nil {
		return nil, err
	}
	// En una instalación nueva las migraciones corren antes de crear los roles: el administrador
	// recibe aquí el límite de descuento total que la migración 0006 concede en instalaciones existentes.
	_, err = tx.Exec(ctx, `INSERT INTO app.role_discount_limits(role_id,max_percent)
        SELECT id,100 FROM app.roles WHERE code='administrator' ON CONFLICT DO NOTHING`)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.role_permissions(role_id,permission_key)
        SELECT r.id,p.key FROM app.roles r CROSS JOIN app.permissions p
        WHERE (r.code='supervisor' AND p.key IN ('users.read','devices.read','sessions.read','registers.read'))
        OR (r.code IN ('cashier','finance') AND p.key='registers.read')
        ON CONFLICT DO NOTHING`)
	if err != nil {
		return nil, err
	}
	var adminID, registerID string
	err = tx.QueryRow(ctx, `INSERT INTO app.users(username,display_name,password_hash,role_id,initial_password)
        SELECT 'admin','Administrador',$1,id,true FROM app.roles WHERE code='administrator' RETURNING app.users.id`, HashPassword("admin")).Scan(&adminID)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, "INSERT INTO app.user_preferences(user_id,theme) VALUES ($1,$2)", adminID, input.Theme)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRow(ctx, "INSERT INTO app.registers(name) VALUES ('Caja 1') RETURNING id").Scan(&registerID)
	if err != nil {
		return nil, err
	}
	deviceToken := NewToken()
	var deviceID string
	err = tx.QueryRow(ctx, "INSERT INTO app.devices(name,secret_hash,register_id) VALUES ($1,$2,$3) RETURNING id", input.TerminalName, TokenHash(deviceToken), registerID).Scan(&deviceID)
	if err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, Principal{DeviceID: deviceID}, "setup.completed", "establishment", nil, nil, input, true); err != nil {
		return nil, err
	}
	return map[string]any{"configured": true, "device_id": deviceID, "device_token": deviceToken}, nil
}

// ProvisionSupport solo se llama desde CLI local. Nunca acepta secretos en argumentos o logs.
func (service *Service) ProvisionSupport(ctx context.Context, password string, reset bool) error {
	if len(password) < 12 || len(password) > 256 {
		return errors.New("la credencial de soporte debe tener entre 12 y 256 caracteres")
	}
	tx, err := service.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7820148302)"); err != nil {
		return err
	}
	if err := ensureFoundations(ctx, tx); err != nil {
		return err
	}
	var initialized bool
	var userID string
	if err := tx.QueryRow(ctx, "SELECT id,password_hash IS NOT NULL FROM app.users WHERE username='sa' FOR UPDATE").Scan(&userID, &initialized); err != nil {
		return err
	}
	if initialized && !reset {
		return errors.New("sa ya está aprovisionado; un restablecimiento requiere el comando local reset-sa")
	}
	_, err = tx.Exec(ctx, "UPDATE app.users SET password_hash=$1,active=true,updated_at=now() WHERE id=$2", HashPassword(password), userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "UPDATE app.sessions SET revoked_at=now(),revoke_reason='support_reset' WHERE user_id=$1 AND revoked_at IS NULL", userID)
	if err != nil {
		return err
	}
	action := "support.provisioned"
	if initialized {
		action = "support.credential_reset"
	}
	if err := audit(ctx, tx, Principal{}, action, "user", &userID, nil, map[string]bool{"credential_configured": true}, true); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
