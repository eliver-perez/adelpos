package catalog

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"local/ad-el-pos/server/internal/ordering"
)

// Indicaciones rápidas por categoría (etapa 7.1, decisión 17). Cada categoría tiene su lista
// ordenada de indicaciones sin precio («Sin cebolla», «Sin hielo»); la captura las ofrece como
// botones al personalizar un producto de esa categoría y las guarda como texto en
// customization.notes, igual que las escritas, por lo que llegan a la comanda sin cambios en
// cocina. Se administran con catalog.categories.manage, con revisión optimista, orden visual por
// posiciones y auditoría en la misma transacción. La captura las recibe en los lookups.
//
// Etapa 8 (0012): una indicación puede tener un efecto explícito sobre un ingrediente del catálogo
// (omit, normal o extra; none = solo instrucción de cocina). El efecto se configura aquí y se
// valida al cotizar en quick_note_effects.go; nunca se deduce del texto.

// maximumQuickNotes limita las indicaciones de una categoría: son atajos de un toque.
const maximumQuickNotes = 40

type CategoryQuickNote struct {
	ID         string `json:"id"`
	CategoryID string `json:"category_id"`
	Text       string `json:"text"`
	// Effect es none (solo instrucción), omit, normal o extra; IngredientID e IngredientName
	// identifican el ingrediente vinculado y son nulos sin efecto.
	Effect         string    `json:"effect"`
	IngredientID   *string   `json:"ingredient_id"`
	IngredientName *string   `json:"ingredient_name"`
	Active         bool      `json:"active"`
	SortOrder      int       `json:"sort_order"`
	Revision       int       `json:"revision"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// quickNoteInput admite Effect e IngredientID opcionales: sin efecto en el cuerpo, una alta queda
// como solo instrucción y una edición conserva el efecto vigente (clientes de la etapa 7.1).
type quickNoteInput struct {
	CategoryID   string             `json:"category_id"`
	Revision     int                `json:"revision"`
	Text         string             `json:"text"`
	Active       bool               `json:"active"`
	Effect       *string            `json:"effect"`
	IngredientID *string            `json:"ingredient_id"`
	Position     *ordering.Position `json:"position"`
}

const quickNoteColumns = `SELECT n.id,n.category_id,n.text,n.effect,n.ingredient_id,i.name,n.active,n.sort_order,n.revision,n.updated_at
        FROM app.catalog_category_quick_notes n LEFT JOIN app.catalog_ingredients i ON i.id=n.ingredient_id`

func scanQuickNote(row interface{ Scan(...any) error }) (*CategoryQuickNote, error) {
	var note CategoryQuickNote
	if err := row.Scan(
		&note.ID,
		&note.CategoryID,
		&note.Text,
		&note.Effect,
		&note.IngredientID,
		&note.IngredientName,
		&note.Active,
		&note.SortOrder,
		&note.Revision,
		&note.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &note, nil
}

func readQuickNote(ctx context.Context, tx pgx.Tx, id string) (*CategoryQuickNote, error) {
	note, err := scanQuickNote(tx.QueryRow(ctx, quickNoteColumns+" WHERE n.id=$1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &Error{Status: 404, Code: "NOT_FOUND", Message: "La indicación rápida no existe."}
	}
	return note, err
}

// readQuickNotesByID carga las indicaciones indicadas (activas o no) para validar una selección;
// los identificadores ya tienen formato de UUID.
func readQuickNotesByID(ctx context.Context, tx pgx.Tx, ids []string) (map[string]CategoryQuickNote, error) {
	result := map[string]CategoryQuickNote{}
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := tx.Query(ctx, quickNoteColumns+" WHERE n.id = ANY($1::uuid[])", ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		note, err := scanQuickNote(rows)
		if err != nil {
			return nil, err
		}
		result[note.ID] = *note
	}
	return result, rows.Err()
}

// listQuickNotes devuelve todas las indicaciones, activas e inactivas, en el orden de sus
// categorías y, dentro de cada una, en su orden visual. La captura usa solo las activas.
func listQuickNotes(ctx context.Context, tx pgx.Tx) ([]CategoryQuickNote, error) {
	rows, err := tx.Query(ctx, quickNoteColumns+` JOIN app.catalog_categories c ON c.id=n.category_id
        ORDER BY c.sort_order,c.name,c.id,n.sort_order,n.text,n.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notes := []CategoryQuickNote{}
	for rows.Next() {
		note, err := scanQuickNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, *note)
	}
	return notes, rows.Err()
}

// validate normaliza y revisa el texto: 1 a 80 caracteres, sin comas (las indicaciones de una
// partida se unen con comas en customization.notes) ni saltos de línea o caracteres de control,
// porque se imprimen en la comanda.
func (input *quickNoteInput) validate() error {
	input.Text = strings.TrimSpace(input.Text)
	length := utf8.RuneCountInString(input.Text)
	if length < 1 || length > 80 {
		return invalid("La indicación rápida debe tener entre 1 y 80 caracteres.")
	}
	if strings.Contains(input.Text, ",") {
		return invalid("La indicación rápida no admite comas: la captura separa las indicaciones con comas.")
	}
	if strings.IndexFunc(input.Text, func(character rune) bool {
		return unicode.IsControl(character) || unicode.Is(unicode.Cf, character)
	}) >= 0 {
		return invalid("La indicación rápida no admite saltos de línea ni caracteres de control.")
	}
	return nil
}

// Efectos de una indicación rápida sobre su ingrediente vinculado (0012).
const (
	QuickNoteEffectNone   = "none"
	QuickNoteEffectOmit   = "omit"
	QuickNoteEffectNormal = "normal"
	QuickNoteEffectExtra  = "extra"
)

func knownQuickNoteEffect(effect string) bool {
	switch effect {
	case QuickNoteEffectNone, QuickNoteEffectOmit, QuickNoteEffectNormal, QuickNoteEffectExtra:
		return true
	}
	return false
}

// resolveEffect devuelve el efecto y el ingrediente que se guardarán. Sin efecto en el cuerpo, una
// alta queda como solo instrucción y una edición conserva los de la indicación (previous). Un
// efecto exige un ingrediente existente del catálogo; «solo instrucción» no admite ingrediente.
// No se piden gramos: la cantidad la fija el efecto sobre lo configurado en cada producto.
func (input *quickNoteInput) resolveEffect(ctx context.Context, tx pgx.Tx, previous *CategoryQuickNote) (string, *string, error) {
	if input.Effect == nil {
		if input.IngredientID != nil {
			return "", nil, invalid("Indica el tipo de efecto junto con el ingrediente vinculado.")
		}
		if previous != nil {
			return previous.Effect, previous.IngredientID, nil
		}
		return QuickNoteEffectNone, nil, nil
	}
	effect := *input.Effect
	if !knownQuickNoteEffect(effect) {
		return "", nil, invalid("El efecto de la indicación debe ser solo instrucción, omitir, porción habitual o porción extra.")
	}
	if effect == QuickNoteEffectNone {
		if input.IngredientID != nil {
			return "", nil, invalid("Una indicación que es solo instrucción de cocina no se vincula a un ingrediente.")
		}
		return effect, nil, nil
	}
	if input.IngredientID == nil || *input.IngredientID == "" {
		return "", nil, invalid("Elige el ingrediente al que se aplica el efecto de la indicación.")
	}
	if err := validateID(*input.IngredientID); err != nil {
		return "", nil, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM app.catalog_ingredients WHERE id=$1)", *input.IngredientID).Scan(&exists); err != nil {
		return "", nil, err
	}
	if !exists {
		return "", nil, invalid("El ingrediente vinculado a la indicación no existe.")
	}
	return effect, input.IngredientID, nil
}

func duplicateQuickNote(err error) bool {
	var problem *pgconn.PgError
	return errors.As(err, &problem) && problem.Code == "23505"
}

var errDuplicateQuickNote = &Error{
	Status:  409,
	Code:    "DUPLICATE_QUICK_NOTE",
	Message: "La categoría ya tiene esa indicación rápida.",
}

// quickNotes atiende catalog/quick-notes (POST) y catalog/quick-notes/{id} (PATCH). El permiso
// se comprueba antes de leer con bloqueo o escribir; el lock exclusivo del catálogo ya lo tomó
// Handle.
func quickNotes(ctx context.Context, tx pgx.Tx, request Request, parts []string, audit Audit) (any, error) {
	if err := request.Actor.Require("catalog.categories.manage"); err != nil {
		return nil, err
	}
	var input quickNoteInput
	switch {
	case len(parts) == 1 && request.Method == "POST":
		if err := decode(request.Body, &input); err != nil {
			return nil, err
		}
		return createQuickNote(ctx, tx, input, audit)
	case len(parts) == 2 && request.Method == "PATCH":
		if err := validateID(parts[1]); err != nil {
			return nil, err
		}
		if err := decode(request.Body, &input); err != nil {
			return nil, err
		}
		return updateQuickNote(ctx, tx, parts[1], input, audit)
	}
	return nil, &Error{Status: 405, Code: "METHOD_NOT_ALLOWED", Message: "Método no permitido para las indicaciones rápidas."}
}

func createQuickNote(ctx context.Context, tx pgx.Tx, input quickNoteInput, audit Audit) (any, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	if input.Revision != 0 {
		return nil, invalid("Las indicaciones nuevas no deben reutilizar revisiones.")
	}
	if err := validateID(input.CategoryID); err != nil {
		return nil, err
	}
	if _, err := readDocument(ctx, tx, "categories", input.CategoryID); errors.Is(err, pgx.ErrNoRows) {
		return nil, invalid("La categoría de la indicación no existe.")
	} else if err != nil {
		return nil, err
	}
	effect, ingredientID, err := input.resolveEffect(ctx, tx, nil)
	if err != nil {
		return nil, err
	}
	id := newID()
	if input.Position != nil {
		if _, err := ordering.Validate(input.Position, id); err != nil {
			return nil, invalid(err.Error())
		}
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM app.catalog_category_quick_notes WHERE category_id=$1", input.CategoryID).Scan(&count); err != nil {
		return nil, err
	}
	if count >= maximumQuickNotes {
		return nil, invalid("Una categoría admite hasta 40 indicaciones rápidas.")
	}
	// Una indicación nueva va al final de su categoría salvo que se elija otra posición.
	_, err = tx.Exec(ctx, `INSERT INTO app.catalog_category_quick_notes(id,category_id,text,effect,ingredient_id,active,sort_order)
        VALUES ($1,$2,$3,$4,$5,$6,(SELECT coalesce(max(sort_order),0)+10 FROM app.catalog_category_quick_notes WHERE category_id=$2))`,
		id, input.CategoryID, input.Text, effect, ingredientID, input.Active)
	if duplicateQuickNote(err) {
		return nil, errDuplicateQuickNote
	}
	if err != nil {
		return nil, err
	}
	if err := placeQuickNote(ctx, tx, input, id, true, audit); err != nil {
		return nil, err
	}
	saved, err := readQuickNote(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	return saved, audit(ctx, tx, "catalog.quick-notes.created", "quick-notes", &id, nil, saved)
}

// updateQuickNote cambia el texto, el efecto, el estado o la posición. Una indicación no cambia de
// categoría. Las partidas que ya la usan conservan su texto: está guardado en cada partida.
func updateQuickNote(ctx context.Context, tx pgx.Tx, id string, input quickNoteInput, audit Audit) (any, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	previous, err := readQuickNote(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if input.CategoryID != "" && input.CategoryID != previous.CategoryID {
		return nil, invalid("Una indicación rápida no cambia de categoría; créala en la otra categoría.")
	}
	if input.Revision != previous.Revision {
		return nil, conflict()
	}
	if input.Position != nil {
		if _, err := ordering.Validate(input.Position, id); err != nil {
			return nil, invalid(err.Error())
		}
	}
	effect, ingredientID, err := input.resolveEffect(ctx, tx, previous)
	if err != nil {
		return nil, err
	}
	input.CategoryID = previous.CategoryID
	// Las partidas confirmadas congelan la indicación resuelta (texto, efecto e ingrediente) en su
	// quote_line: cambiarla aquí solo afecta lo que se cotice después.
	_, err = tx.Exec(ctx, `UPDATE app.catalog_category_quick_notes
        SET text=$2,effect=$3,ingredient_id=$4,active=$5,revision=revision+1,updated_at=now()
        WHERE id=$1`, id, input.Text, effect, ingredientID, input.Active)
	if duplicateQuickNote(err) {
		return nil, errDuplicateQuickNote
	}
	if err != nil {
		return nil, err
	}
	if err := placeQuickNote(ctx, tx, input, id, false, audit); err != nil {
		return nil, err
	}
	saved, err := readQuickNote(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	return saved, audit(ctx, tx, "catalog.quick-notes.updated", "quick-notes", &id, previous, saved)
}

// placeQuickNote coloca la indicación dentro de su categoría y audita la renumeración de las
// demás en la misma transacción.
func placeQuickNote(ctx context.Context, tx pgx.Tx, input quickNoteInput, id string, joining bool, audit Audit) error {
	if input.Position == nil {
		return nil
	}
	after, err := ordering.Validate(input.Position, id)
	if err != nil {
		return invalid(err.Error())
	}
	result, err := ordering.Place(ctx, tx, ordering.CategoryQuickNotes, input.CategoryID, id, after, joining)
	var problem *ordering.Error
	if errors.As(err, &problem) {
		return invalid(problem.Message)
	}
	if err != nil {
		return err
	}
	if len(result.Changes) == 0 {
		return nil
	}
	return audit(ctx, tx, "catalog.quick-notes.reordered", "quick-notes", &id, nil, map[string]any{
		"placed_id":   id,
		"category_id": input.CategoryID,
		"after":       after,
		"sort_order":  result.SortOrder,
		"changes":     result.Changes,
	})
}
