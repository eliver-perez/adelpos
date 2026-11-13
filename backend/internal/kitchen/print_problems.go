package kitchen

import "strings"

// Qué le pasó a una impresión, en una clave de una lista cerrada (etapa 9F, §4.2 y §4.3). Cada resultado
// del transporte y cada trabajo que se encola sin poder salir llevan su clave (Outcome.Code, guardada en
// kitchen_print_jobs.result y kitchen_print_attempts.result); la interfaz la traduce a una frase del
// restaurante con el nombre de la impresora («La impresora de caja no responde»), sin términos técnicos.
// El mensaje técnico del transporte se conserva para soporte.
const (
	// ProblemUnreachable: la impresora o la cola del equipo no respondió; nada salió en papel.
	ProblemUnreachable = "unreachable"
	// ProblemInactive: la impresora (o una estación del documento) está desactivada.
	ProblemInactive = "inactive"
	// ProblemUnsupported: la impresora es de otra terminal (agente), que todavía no puede imprimir.
	ProblemUnsupported = "unsupported"
	// ProblemSetup: la impresora necesita revisar su configuración (destino guardado, opción de corte del
	// controlador o una cola que convierte los datos).
	ProblemSetup = "setup"
	// ProblemUncertain: no se sabe si el documento salió; se revisa el papel antes de pedir otra copia.
	ProblemUncertain = "uncertain"
)

// Textos de los trabajos que no pueden salir, para el restaurante (antes «El destino está inactivo o
// requiere un agente de impresión aún no implementado» y «El destino requiere un agente de impresión
// futuro»).
const (
	unsupportedPrinterMessage = "Esta impresora todavía no puede usarse para imprimir."
)

// inactivePrinterMessage dice qué impresora está desactivada.
func inactivePrinterMessage(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "La impresora está desactivada."
	}
	return "La impresora " + name + " está desactivada."
}

// JobProblemSQL es la expresión SQL de la clave del problema de un trabajo de impresión (alias de
// kitchen_print_jobs): la del último resultado guardado o, si no la tiene (trabajos anteriores a la 9F),
// la que corresponde a su estado. Solo los estados que exigen atención tienen problema.
func JobProblemSQL(alias string) string {
	return `CASE WHEN ` + alias + `.state IN ('error','intervention','uncertain') THEN coalesce(` + alias + `.result->>'code',
        CASE ` + alias + `.state WHEN 'error' THEN 'unreachable' WHEN 'intervention' THEN 'setup' ELSE 'uncertain' END) END`
}

// outcomeWithCode completa la clave del problema de un resultado que no la trae, según su estado.
func outcomeWithCode(outcome Outcome) Outcome {
	if outcome.Code != "" {
		return outcome
	}
	switch outcome.State {
	case "error":
		outcome.Code = ProblemUnreachable
	case "intervention":
		outcome.Code = ProblemSetup
	case "uncertain":
		outcome.Code = ProblemUncertain
	}
	return outcome
}
