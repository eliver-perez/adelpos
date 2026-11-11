package inventory

import (
	"strings"
)

// Código interno sugerido de insumos (etapa 9A, §3.4). La interfaz forma el código a partir del
// nombre con la regla de la etapa 6.1 (desktop/src/services/productCode.ts: minúsculas, sin
// acentos, guiones y hasta 40 caracteres para insumos) y pregunta aquí si está libre, porque el
// listado de insumos es paginado y la pantalla no conoce todos los códigos. Go es la autoridad:
// esta consulta solo informa y sugiere; al guardar, el índice único lower(code) sigue respondiendo
// DUPLICATE_CODE. Nunca cambia el código de un insumo existente.
//
// Los códigos se comparan con la misma normalización que el índice único: lower() de PostgreSQL
// con la configuración regional de la base (C en las instalaciones del proyecto, que solo pasa a
// minúsculas las letras ASCII: «PAÑO» y «paño» son códigos distintos para el índice). Go nunca
// usa strings.ToLower para esto, porque con «Ñ» o vocales acentuadas daría otra clave y podría
// sugerir un código ocupado.

// maximumItemCode es el largo máximo del código interno de un insumo (CHECK de la 0010).
const maximumItemCode = 40

// maximumCodeSuffix acota los sufijos -2, -3… que se prueban para sugerir un código libre.
const maximumCodeSuffix = 9999

// ItemCodeCheck es el resultado de comprobar un código interno de insumo.
type ItemCodeCheck struct {
	// Code es el código comprobado, recortado.
	Code string `json:"code"`
	// Available indica que ningún otro insumo lo usa (sin distinguir mayúsculas).
	Available bool `json:"available"`
	// Suggestion es el mismo código si está libre o el primero libre con sufijo -2, -3… (recortando
	// la base para no pasar de 40 caracteres), la misma regla de suggestProductCode.
	Suggestion string `json:"suggestion"`
	// TakenBy es el nombre del insumo que ya lo usa, si lo hay.
	TakenBy *string `json:"taken_by"`
}

// trimCode recorta un código a maximum caracteres sin dejar guiones al final, como trimTo en
// productCode.ts.
func trimCode(code string, maximum int) string {
	runes := []rune(code)
	if len(runes) > maximum {
		runes = runes[:maximum]
	}
	return strings.TrimRight(string(runes), "-")
}

// codeStems son los textos cuya clave del índice necesita la sugerencia: la base y la base
// recortada para cada largo de sufijo (de «-2» a «-9999»).
func codeStems(base string, maximum int) []string {
	stems := []string{base}
	for length := 2; length <= len("-"+itoa(maximumCodeSuffix)); length++ {
		stems = append(stems, trimCode(base, maximum-length))
	}
	return stems
}

// suggestFreeCode propone un código libre a partir de base: base si no está tomado o base con el
// primer sufijo -N libre. taken contiene las claves del índice único de los códigos existentes y
// lower da la clave de un texto con la misma normalización (en el servidor, lower() de
// PostgreSQL). El sufijo «-N» solo tiene guion y dígitos, que lower() nunca cambia, así que la
// clave de un candidato es la de su base recortada más el sufijo. Es una función pura. Solo si los
// 9 998 sufijos estuvieran ocupados devuelve la base (no hay código libre que sugerir).
func suggestFreeCode(base string, lower func(string) string, taken map[string]bool, maximum int) string {
	if !taken[lower(base)] {
		return base
	}
	for attempt := 2; attempt <= maximumCodeSuffix; attempt++ {
		suffix := "-" + itoa(attempt)
		stem := trimCode(base, maximum-len(suffix))
		if !taken[lower(stem)+suffix] {
			return stem + suffix
		}
	}
	return base
}

// indexKeys calcula con PostgreSQL la clave del índice único (lower) de cada texto.
func (handler *service) indexKeys(texts []string) (map[string]string, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT stem,lower(stem) FROM unnest($1::text[]) AS stem`, texts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := map[string]string{}
	for rows.Next() {
		var text, key string
		if err := rows.Scan(&text, &key); err != nil {
			return nil, err
		}
		keys[text] = key
	}
	return keys, rows.Err()
}

// checkItemCode atiende GET inventory/item-codes?code=…[&item_id=…]: indica si el código está libre
// (excluido el insumo que se edita, item_id) y sugiere uno libre. Lee con inventory.read, igual que
// el listado de insumos que ya muestra los códigos.
func (handler *service) checkItemCode(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "code", "item_id"); err != nil {
		return nil, err
	}
	code := strings.TrimSpace(query.Get("code"))
	if err := validateName(&code, "El código interno del insumo", maximumItemCode); err != nil {
		return nil, err
	}
	itemID, err := optionalQueryID(query, "item_id", "insumo")
	if err != nil {
		return nil, err
	}
	excluded := ""
	if itemID != nil {
		excluded = *itemID
	}
	// Códigos que comparten el principio con la base: cubren la base y todas sus variantes con sufijo
	// (la base recortada para el sufijo más largo sigue siendo un prefijo de la base).
	prefix := trimCode(code, maximumItemCode-len("-"+itoa(maximumCodeSuffix)))
	escaper := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	rows, err := handler.tx.Query(handler.ctx, `SELECT lower(code),name,lower(code)=lower($1)
        FROM app.inventory_items WHERE lower(code) LIKE lower($2) || '%' AND id::text<>$3`,
		code, escaper.Replace(prefix), excluded)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := ItemCodeCheck{Code: code, Available: true}
	taken := map[string]bool{}
	for rows.Next() {
		var existing, name string
		var exact bool
		if err := rows.Scan(&existing, &name, &exact); err != nil {
			return nil, err
		}
		taken[existing] = true
		if exact {
			result.Available = false
			holder := name
			result.TakenBy = &holder
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	keys, err := handler.indexKeys(codeStems(code, maximumItemCode))
	if err != nil {
		return nil, err
	}
	lower := func(text string) string {
		return keys[text]
	}
	result.Suggestion = suggestFreeCode(code, lower, taken, maximumItemCode)
	return result, nil
}
