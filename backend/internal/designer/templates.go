package designer

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
)

// Plantillas y versiones. Una plantilla del sistema (una por tipo) es de solo lectura: se duplica
// para editarla. Guardar crea siempre una versión nueva e inmutable; restaurar una versión
// anterior es guardar su layout como versión nueva.

func appendEvent(ctx context.Context, tx pgx.Tx, kind string, aggregateID *string) error {
	return events.Append(ctx, tx, kind, documents.Scope, aggregateID, nil)
}

// VersionSummary es la versión vigente de una plantilla o un renglón de su historial.
type VersionSummary struct {
	ID            string    `json:"id"`
	Number        int       `json:"number"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedByName *string   `json:"created_by_name"`
	BlockCount    int       `json:"block_count"`
}

// TemplateSummary es una plantilla en la lista de su tipo.
type TemplateSummary struct {
	ID             string         `json:"id"`
	Kind           documents.Kind `json:"kind"`
	KindName       string         `json:"kind_name"`
	Name           string         `json:"name"`
	System         bool           `json:"system"`
	Active         bool           `json:"active"`
	Revision       int            `json:"revision"`
	CurrentVersion VersionSummary `json:"current_version"`
	CreatedAt      time.Time      `json:"created_at"`
	CreatedByName  *string        `json:"created_by_name"`
	UpdatedAt      time.Time      `json:"updated_at"`
	UpdatedByName  *string        `json:"updated_by_name"`
}

// TemplateDetail es una plantilla con el layout de su versión vigente.
type TemplateDetail struct {
	TemplateSummary
	Layout json.RawMessage `json:"layout"`
}

// VersionDetail es una versión con su layout.
type VersionDetail struct {
	VersionSummary
	TemplateID   string          `json:"template_id"`
	TemplateName string          `json:"template_name"`
	Kind         documents.Kind  `json:"kind"`
	Layout       json.RawMessage `json:"layout"`
}

const templateQuery = `SELECT t.id,t.kind,t.name,t.system,t.active,t.revision,t.created_at,creator.display_name,t.updated_at,
    updater.display_name,v.id,v.number,v.note,v.created_at,author.display_name,jsonb_array_length(v.layout->'blocks'),v.layout
    FROM app.document_templates t JOIN app.document_template_versions v ON v.id=t.current_version_id
    LEFT JOIN app.users creator ON creator.id=t.created_by LEFT JOIN app.users updater ON updater.id=t.updated_by
    LEFT JOIN app.users author ON author.id=v.created_by`

func scanTemplate(row pgx.Row) (*TemplateDetail, error) {
	var detail TemplateDetail
	var kind string
	var layout []byte
	err := row.Scan(&detail.ID, &kind, &detail.Name, &detail.System, &detail.Active, &detail.Revision, &detail.CreatedAt,
		&detail.CreatedByName, &detail.UpdatedAt, &detail.UpdatedByName, &detail.CurrentVersion.ID, &detail.CurrentVersion.Number,
		&detail.CurrentVersion.Note, &detail.CurrentVersion.CreatedAt, &detail.CurrentVersion.CreatedByName,
		&detail.CurrentVersion.BlockCount, &layout)
	if err != nil {
		return nil, err
	}
	detail.Kind = documents.Kind(kind)
	detail.KindName = detail.Kind.Name()
	detail.Layout = layout
	return &detail, nil
}

// loadTemplate lee una plantilla; forUpdate la bloquea hasta el final de la transacción. El bloqueo
// se toma con una consulta propia y la lectura va después, en otra sentencia: así, si otro
// administrador guardaba en ese momento, se lee su versión ya confirmada (y su nueva revisión).
func (handler *service) loadTemplate(id string, forUpdate bool) (*TemplateDetail, error) {
	if forUpdate {
		var locked string
		err := handler.tx.QueryRow(handler.ctx, "SELECT id FROM app.document_templates WHERE id=$1 FOR UPDATE", id).Scan(&locked)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notFound("La plantilla no existe.")
		}
		if err != nil {
			return nil, err
		}
	}
	detail, err := scanTemplate(handler.tx.QueryRow(handler.ctx, templateQuery+" WHERE t.id=$1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, notFound("La plantilla no existe.")
	}
	return detail, err
}

// listTemplates devuelve las plantillas de un tipo (o de todos), la activa primero.
func (handler *service) listTemplates(_ []string) (any, error) {
	kind := handler.request.Query.Get("kind")
	if kind != "" {
		if _, found := documents.ParseKind(kind); !found {
			return nil, invalid("El tipo de documento no existe.")
		}
	}
	rows, err := handler.tx.Query(handler.ctx, templateQuery+` WHERE ($1='' OR t.kind=$1)
        ORDER BY t.kind,t.active DESC,t.system DESC,lower(t.name)`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []TemplateSummary{}
	for rows.Next() {
		detail, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, detail.TemplateSummary)
	}
	return result, rows.Err()
}

func (handler *service) readTemplate(parts []string) (any, error) {
	return handler.loadTemplate(parts[1], false)
}

// cleanName valida el nombre de una plantilla (1–80 caracteres, sin controles).
func cleanName(value string) (string, error) {
	name := strings.TrimSpace(documents.Printable(value))
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || len([]rune(name)) > 80 {
		return "", invalid("El nombre de la plantilla debe tener de 1 a 80 caracteres.")
	}
	return name, nil
}

// nameTaken traduce el índice único de nombre por tipo a un conflicto con mensaje propio.
func nameTaken(err error) error {
	var problem *pgconn.PgError
	if errors.As(err, &problem) && problem.Code == "23505" && problem.ConstraintName == "document_templates_name_key" {
		return conflict("TEMPLATE_NAME_TAKEN", "Ya existe una plantilla con ese nombre para este tipo de documento.")
	}
	return err
}

// createTemplate crea una plantilla del restaurante (inactiva) con su versión 1: la copia de una
// versión existente del mismo tipo (duplicar) o, sin origen, la plantilla predeterminada del tipo.
func (handler *service) createTemplate(_ []string) (any, error) {
	var input struct {
		OperationID     string  `json:"operation_id"`
		Kind            string  `json:"kind"`
		Name            string  `json:"name"`
		SourceVersionID *string `json:"source_version_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	kind, found := documents.ParseKind(input.Kind)
	if !found {
		return nil, invalid("El tipo de documento no existe.")
	}
	name, err := cleanName(input.Name)
	if err != nil {
		return nil, err
	}
	template := documents.Default(kind)
	note := "Creada desde la plantilla predeterminada."
	if input.SourceVersionID != nil {
		if err := validID(*input.SourceVersionID); err != nil {
			return nil, err
		}
		source, err := handler.versionDetail(*input.SourceVersionID)
		if err != nil {
			return nil, err
		}
		if source.Kind != kind {
			return nil, conflict("SOURCE_KIND_MISMATCH", "La versión de origen pertenece a otro tipo de documento.")
		}
		template, err = documents.ParseTemplate(kind, source.Layout)
		if err != nil {
			return nil, templateInvalid(err)
		}
		note = "Copia de «" + source.TemplateName + "», versión " + strconv.Itoa(source.Number) + "."
	}
	templateID := events.NewID()
	versionID := events.NewID()
	actor := handler.request.Actor.UserID
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.document_templates(id,kind,name,system,active,current_version_id,
        created_by,updated_by) VALUES ($1,$2,$3,false,false,$4,$5,$5)`, templateID, string(kind), name, versionID, actor)
	if err != nil {
		return nil, nameTaken(err)
	}
	if _, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.document_template_versions(id,template_id,number,layout,note,created_by)
        VALUES ($1,$2,1,$3,$4,$5)`, versionID, templateID, template.Encode(), note, actor); err != nil {
		return nil, err
	}
	detail, err := handler.loadTemplate(templateID, false)
	if err != nil {
		return nil, err
	}
	if err := handler.audit(handler.ctx, handler.tx, "documents.template_created", "document_template", &templateID, nil, map[string]any{
		"kind": kind, "name": name, "version_id": versionID, "source_version_id": input.SourceVersionID,
		"block_count": len(template.Blocks),
	}); err != nil {
		return nil, err
	}
	return detail, handler.changed(templateID)
}

// lockedEditable bloquea una plantilla del restaurante y comprueba su revisión.
func (handler *service) lockedEditable(id string, revision int) (*TemplateDetail, error) {
	current, err := handler.loadTemplate(id, true)
	if err != nil {
		return nil, err
	}
	if current.System {
		return nil, conflict("TEMPLATE_READ_ONLY", "La plantilla predeterminada no se modifica: duplícala para editarla.")
	}
	if current.Revision != revision {
		return nil, revisionConflict(current)
	}
	return current, nil
}

// revisionConflict avisa que otro administrador guardó antes; el cliente conserva su borrador.
func revisionConflict(current *TemplateDetail) error {
	return &Error{
		Status: 409, Code: "REVISION_CONFLICT",
		Message: "Otro administrador cambió esta plantilla. Tu borrador se conserva: revisa la versión vigente antes de volver a guardar.",
		Details: map[string]any{"revision": current.Revision, "current_version_id": current.CurrentVersion.ID},
	}
}

// saveTemplate guarda el layout como versión nueva de la plantilla.
func (handler *service) saveTemplate(parts []string) (any, error) {
	var input struct {
		OperationID string          `json:"operation_id"`
		Revision    int             `json:"revision"`
		Layout      json.RawMessage `json:"layout"`
		Note        string          `json:"note"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	note := strings.TrimSpace(strings.Join(strings.Fields(documents.Printable(input.Note)), " "))
	if len([]rune(note)) > 200 {
		return nil, invalid("La nota de la versión admite como máximo 200 caracteres.")
	}
	current, err := handler.lockedEditable(parts[1], input.Revision)
	if err != nil {
		return nil, err
	}
	template, err := documents.ParseTemplate(current.Kind, input.Layout)
	if err != nil {
		return nil, templateInvalid(err)
	}
	versionID := events.NewID()
	number := current.CurrentVersion.Number
	if err := handler.tx.QueryRow(handler.ctx, "SELECT max(number) FROM app.document_template_versions WHERE template_id=$1",
		current.ID).Scan(&number); err != nil {
		return nil, err
	}
	number++
	actor := handler.request.Actor.UserID
	stored := template.Encode()
	if _, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.document_template_versions(id,template_id,number,layout,note,created_by)
        VALUES ($1,$2,$3,$4,$5,$6)`, versionID, current.ID, number, stored, note, actor); err != nil {
		return nil, err
	}
	if _, err := handler.tx.Exec(handler.ctx, `UPDATE app.document_templates SET current_version_id=$1,revision=revision+1,
        updated_by=$2,updated_at=now() WHERE id=$3`, versionID, actor, current.ID); err != nil {
		return nil, err
	}
	detail, err := handler.loadTemplate(current.ID, false)
	if err != nil {
		return nil, err
	}
	// La auditoría no vuelca el layout: identifica la versión, su número, sus bloques y su tamaño.
	if err := handler.audit(handler.ctx, handler.tx, "documents.template_saved", "document_template", &current.ID, map[string]any{
		"version_id": current.CurrentVersion.ID, "number": current.CurrentVersion.Number,
	}, map[string]any{
		"version_id": versionID, "number": number, "block_count": len(template.Blocks), "bytes": len(stored), "note": note,
	}); err != nil {
		return nil, err
	}
	return detail, handler.changed(current.ID)
}

// renameTemplate cambia el nombre de una plantilla del restaurante.
func (handler *service) renameTemplate(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Revision    int    `json:"revision"`
		Name        string `json:"name"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	name, err := cleanName(input.Name)
	if err != nil {
		return nil, err
	}
	current, err := handler.lockedEditable(parts[1], input.Revision)
	if err != nil {
		return nil, err
	}
	if _, err := handler.tx.Exec(handler.ctx, `UPDATE app.document_templates SET name=$1,revision=revision+1,updated_by=$2,
        updated_at=now() WHERE id=$3`, name, handler.request.Actor.UserID, current.ID); err != nil {
		return nil, nameTaken(err)
	}
	detail, err := handler.loadTemplate(current.ID, false)
	if err != nil {
		return nil, err
	}
	if err := handler.audit(handler.ctx, handler.tx, "documents.template_renamed", "document_template", &current.ID,
		map[string]string{"name": current.Name}, map[string]string{"name": name}); err != nil {
		return nil, err
	}
	return detail, handler.changed(current.ID)
}

// lockKind bloquea todas las plantillas de un tipo, en orden de identificador, y devuelve la activa.
// Dos activaciones simultáneas del mismo tipo se serializan sin interbloquearse.
func (handler *service) lockKind(kind documents.Kind) (*string, error) {
	var active *string
	rows, err := handler.tx.Query(handler.ctx, "SELECT id,active FROM app.document_templates WHERE kind=$1 ORDER BY id FOR UPDATE",
		string(kind))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var current bool
		if err := rows.Scan(&id, &current); err != nil {
			return nil, err
		}
		if current {
			copied := id
			active = &copied
		}
	}
	return active, rows.Err()
}

// activate deja activa la plantilla indicada y desactiva la anterior del mismo tipo. Quien llama
// ya bloqueó las plantillas del tipo (lockKind). Solo la plantilla activada cambia de revisión.
func (handler *service) activate(target *TemplateDetail, previous *string, action string) (any, error) {
	if previous != nil && *previous == target.ID {
		return target, nil
	}
	actor := handler.request.Actor.UserID
	if previous != nil {
		if _, err := handler.tx.Exec(handler.ctx, "UPDATE app.document_templates SET active=false,updated_at=now() WHERE id=$1", *previous); err != nil {
			return nil, err
		}
	}
	if _, err := handler.tx.Exec(handler.ctx, `UPDATE app.document_templates SET active=true,revision=revision+1,updated_by=$1,
        updated_at=now() WHERE id=$2`, actor, target.ID); err != nil {
		return nil, err
	}
	detail, err := handler.loadTemplate(target.ID, false)
	if err != nil {
		return nil, err
	}
	if err := handler.audit(handler.ctx, handler.tx, action, "document_template", &target.ID, map[string]any{
		"active_template_id": previous,
	}, map[string]any{"active_template_id": target.ID, "kind": target.Kind, "version_id": target.CurrentVersion.ID}); err != nil {
		return nil, err
	}
	return detail, handler.changed(target.ID)
}

// templateKind lee el tipo de una plantilla (inmutable) sin bloquearla.
func (handler *service) templateKind(id string) (documents.Kind, error) {
	var kind string
	err := handler.tx.QueryRow(handler.ctx, "SELECT kind FROM app.document_templates WHERE id=$1", id).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", notFound("La plantilla no existe.")
	}
	return documents.Kind(kind), err
}

// activateTemplate activa una plantilla (también la del sistema) comprobando su revisión.
func (handler *service) activateTemplate(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Revision    int    `json:"revision"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	kind, err := handler.templateKind(parts[1])
	if err != nil {
		return nil, err
	}
	previous, err := handler.lockKind(kind)
	if err != nil {
		return nil, err
	}
	current, err := handler.loadTemplate(parts[1], false)
	if err != nil {
		return nil, err
	}
	if current.Revision != input.Revision {
		return nil, revisionConflict(current)
	}
	return handler.activate(current, previous, "documents.template_activated")
}

// restoreDefault activa la plantilla del sistema del tipo (el formato predeterminado).
func (handler *service) restoreDefault(_ []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Kind        string `json:"kind"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	kind, found := documents.ParseKind(input.Kind)
	if !found {
		return nil, invalid("El tipo de documento no existe.")
	}
	previous, err := handler.lockKind(kind)
	if err != nil {
		return nil, err
	}
	system, err := handler.loadTemplate(documents.SystemTemplateID(kind), false)
	if err != nil {
		return nil, err
	}
	return handler.activate(system, previous, "documents.default_restored")
}

// listVersions devuelve el historial de una plantilla, la más reciente primero.
func (handler *service) listVersions(parts []string) (any, error) {
	if _, err := handler.loadTemplate(parts[1], false); err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT v.id,v.number,v.note,v.created_at,u.display_name,jsonb_array_length(v.layout->'blocks')
        FROM app.document_template_versions v LEFT JOIN app.users u ON u.id=v.created_by
        WHERE v.template_id=$1 ORDER BY v.number DESC LIMIT 200`, parts[1])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []VersionSummary{}
	for rows.Next() {
		var item VersionSummary
		if err := rows.Scan(&item.ID, &item.Number, &item.Note, &item.CreatedAt, &item.CreatedByName, &item.BlockCount); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (handler *service) versionDetail(id string) (*VersionDetail, error) {
	var detail VersionDetail
	var kind string
	var layout []byte
	err := handler.tx.QueryRow(handler.ctx, `SELECT v.id,v.number,v.note,v.created_at,u.display_name,jsonb_array_length(v.layout->'blocks'),
        v.template_id,t.name,t.kind,v.layout FROM app.document_template_versions v JOIN app.document_templates t ON t.id=v.template_id
        LEFT JOIN app.users u ON u.id=v.created_by WHERE v.id=$1`, id).Scan(&detail.ID, &detail.Number, &detail.Note, &detail.CreatedAt,
		&detail.CreatedByName, &detail.BlockCount, &detail.TemplateID, &detail.TemplateName, &kind, &layout)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, notFound("La versión de plantilla no existe.")
	}
	if err != nil {
		return nil, err
	}
	detail.Kind = documents.Kind(kind)
	detail.Layout = layout
	return &detail, nil
}

func (handler *service) readVersion(parts []string) (any, error) {
	return handler.versionDetail(parts[1])
}
