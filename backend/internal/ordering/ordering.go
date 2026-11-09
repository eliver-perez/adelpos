// Package ordering coloca un registro en una lista de hermanos ordenada por sort_order.
//
// La interfaz ya no captura números de orden: el usuario elige «Al inicio» o «Después de X» y
// Go recalcula la numeración. El cálculo es una función pura (Plan) y la aplicación en
// PostgreSQL (Place) ocurre dentro de la transacción del módulo que guarda el registro, de modo
// que el guardado, la renumeración y su auditoría se confirman o revierten juntos.
package ordering

import (
	"regexp"
	"slices"
)

// Step es la distancia entre posiciones consecutivas después de renumerar.
const Step = 10

// Position es la posición elegida por el usuario. After vacío coloca el registro al inicio; un
// UUID lo coloca inmediatamente después de ese hermano. El campo es obligatorio cuando se envía
// la posición: un objeto vacío no se interpreta como «al inicio» por omisión.
type Position struct {
	After *string `json:"after"`
}

// Error describe una posición inválida; cada módulo lo traduce a su propio error 400.
type Error struct {
	Message string
}

func (problem *Error) Error() string {
	return problem.Message
}

var identifierPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var (
	errMissing = &Error{
		Message: "Indica la posición del registro: al inicio o después de otro registro.",
	}
	errNotSibling = &Error{
		Message: "La posición elegida no es válida: el registro indicado no existe o no pertenece a la misma lista.",
	}
	errSelf = &Error{
		Message: "Un registro no puede colocarse después de sí mismo.",
	}
)

// Validate revisa la forma de la posición antes de cualquier escritura. La pertenencia del
// hermano se comprueba después, contra la lista bloqueada en la transacción.
func Validate(position *Position, id string) (string, error) {
	if position == nil || position.After == nil {
		return "", errMissing
	}
	after := *position.After
	if after == "" {
		return "", nil
	}
	if !identifierPattern.MatchString(after) {
		return "", errNotSibling
	}
	if after == id {
		return "", errSelf
	}
	return after, nil
}

// Sibling es un registro de la lista con su orden actual.
type Sibling struct {
	ID        string
	SortOrder int
}

// Change es un registro cuyo número de orden cambia al aplicar la posición.
type Change struct {
	ID   string `json:"id"`
	From int    `json:"from"`
	To   int    `json:"to"`
}

// Arrange devuelve los identificadores en su nuevo orden: retira id de la lista y lo inserta al
// inicio (after vacío) o inmediatamente después de after. after debe ser otro hermano.
func Arrange(siblings []string, id, after string) ([]string, error) {
	if after == id && after != "" {
		return nil, errSelf
	}
	remaining := make([]string, 0, len(siblings)+1)
	for _, sibling := range siblings {
		if sibling != id {
			remaining = append(remaining, sibling)
		}
	}
	insertAt := 0
	if after != "" {
		index := slices.Index(remaining, after)
		if index < 0 {
			return nil, errNotSibling
		}
		insertAt = index + 1
	}
	return slices.Insert(remaining, insertAt, id), nil
}

// Plan calcula la renumeración que coloca id en la posición indicada. siblings llega en el orden
// actual (sort_order, nombre, identificador). Si id no está en siblings, el registro se incorpora
// a la lista (es nuevo o cambió de grupo) y su número anterior no cuenta.
//
// Reglas, pensadas para no alterar revisiones de hermanos sin necesidad:
//   - Un registro existente que conserva su lugar y ya tiene un número estrictamente entre los de
//     sus vecinos no cambia: guardar sin mover no renumera la lista.
//   - Un registro que se incorpora al final recibe el mayor número de la lista más Step, sin
//     tocar a los demás (la lista vacía empieza en Step).
//   - En cualquier otro caso (movimiento, inserción intermedia o empates heredados) toda la lista
//     se renumera en múltiplos de Step (10, 20, 30…) y solo se informan los registros cuyo número
//     cambió.
func Plan(siblings []Sibling, id, after string) ([]Change, error) {
	current := make([]string, 0, len(siblings))
	orders := map[string]int{}
	highest := 0
	for _, sibling := range siblings {
		current = append(current, sibling.ID)
		orders[sibling.ID] = sibling.SortOrder
		highest = max(highest, sibling.SortOrder)
	}
	arranged, err := Arrange(current, id, after)
	if err != nil {
		return nil, err
	}
	_, present := orders[id]
	if present && slices.Equal(arranged, current) && strictlyPlaced(siblings, id) {
		return []Change{}, nil
	}
	if !present && arranged[len(arranged)-1] == id {
		return []Change{{ID: id, From: 0, To: highest + Step}}, nil
	}
	changes := []Change{}
	for index, identifier := range arranged {
		next := (index + 1) * Step
		previous, known := orders[identifier]
		if !known || previous != next {
			changes = append(changes, Change{ID: identifier, From: previous, To: next})
		}
	}
	return changes, nil
}

// strictlyPlaced indica si el número de id es mayor que el de su vecino anterior y menor que el
// del siguiente; con empates, su lugar dependería del nombre y no de la posición elegida.
func strictlyPlaced(siblings []Sibling, id string) bool {
	for index, sibling := range siblings {
		if sibling.ID != id {
			continue
		}
		if index > 0 && siblings[index-1].SortOrder >= sibling.SortOrder {
			return false
		}
		if index < len(siblings)-1 && siblings[index+1].SortOrder <= sibling.SortOrder {
			return false
		}
		return true
	}
	return false
}
