package kitchen

import (
	"strconv"
	"strings"
)

// Qué le pasó a una impresión, en una clave de una lista cerrada (etapa 9F, §4.2 y §4.3). Cada resultado
// del transporte y cada trabajo que se encola sin poder salir llevan su clave (Outcome.Code, guardada en
// kitchen_print_jobs.result y kitchen_print_attempts.result); la interfaz la traduce a una frase del
// restaurante con el nombre de la impresora («La impresora de caja no responde»), sin términos técnicos.
// El mensaje técnico del transporte se conserva para soporte.
const (
	// ProblemUnreachable: la impresora no respondió (conexión de red, cola del equipo que no contestó a
	// tiempo); nada salió en papel.
	ProblemUnreachable = "unreachable"
	// ProblemNotFound: la impresora o su cola no existe en el equipo servidor (cola CUPS inexistente,
	// impresora de Windows no instalada o renombrada; revisión 9F, B4). Reintentar no lo resuelve: se
	// configura la impresora.
	ProblemNotFound = "not_found"
	// ProblemInactive: la impresora (o una estación del documento) está desactivada.
	ProblemInactive = "inactive"
	// ProblemUnsupported: la impresora es de otra terminal (agente), que todavía no puede imprimir.
	ProblemUnsupported = "unsupported"
	// ProblemSetup: la impresora necesita revisar su configuración (destino guardado, opción de corte del
	// controlador o una cola que convierte los datos).
	ProblemSetup = "setup"
	// ProblemUncertain: no se sabe si el documento salió; se revisa el papel antes de pedir otra copia.
	ProblemUncertain = "uncertain"
	// ProblemAgentOffline: la impresora es de una terminal cuyo agente de impresión no está trabajando (la
	// aplicación está cerrada, la computadora apagada o sin red; etapa 9G, fase E, D13). El trabajo sigue
	// pendiente y se imprime solo cuando la terminal vuelve; la interfaz dice «La Caja 2 está desconectada».
	ProblemAgentOffline = "agent_offline"
)

// AgentOfflineSeconds es cuánto tiempo sin consultas de su agente de impresión basta para considerar
// desconectada a una terminal: el agente consulta cada pocos segundos mientras la aplicación está abierta.
const AgentOfflineSeconds = 30

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
// la que corresponde a su estado. Solo los estados que exigen atención tienen problema. Desde la etapa 9G
// (fase E) un trabajo que espera a la impresora de una terminal desconectada (pendiente, en proceso o con un error
// que todavía se reintentará solo) tiene el problema agent_offline, calculado con la última consulta de su agente.
func JobProblemSQL(alias string) string {
	return `CASE WHEN (` + alias + `.state IN ('pending','processing') OR (` + alias + `.state='error' AND ` + alias + `.attempts<3))
        AND ` + agentOfflineSQL(alias) + ` THEN 'agent_offline'
        WHEN ` + alias + `.state IN ('error','intervention','uncertain') THEN coalesce(` + alias + `.result->>'code',
        CASE ` + alias + `.state WHEN 'error' THEN 'unreachable' WHEN 'intervention' THEN 'setup' ELSE 'uncertain' END) END`
}

// agentDeviceSQL es la terminal de la impresora de un trabajo según su fotografía (NULL si no es la impresora de
// una terminal): la fotografía decide qué agente reclama el trabajo.
func agentDeviceSQL(alias string) string {
	return `(CASE WHEN ` + alias + `.printer_snapshot->>'transport'='agent' THEN nullif(` + alias +
		`.printer_snapshot->>'agent_device_id','') END)`
}

// agentOfflineSQL indica que el trabajo es de la impresora de una terminal cuyo agente no consulta desde hace más
// de AgentOfflineSeconds (o que fue revocada).
func agentOfflineSQL(alias string) string {
	return agentDeviceSQL(alias) + ` IS NOT NULL AND NOT EXISTS(SELECT FROM app.devices agent_terminal
        WHERE agent_terminal.id::text=` + agentDeviceSQL(alias) + ` AND agent_terminal.revoked_at IS NULL
        AND agent_terminal.print_agent_seen_at > now() - make_interval(secs => ` + strconv.Itoa(AgentOfflineSeconds) + `))`
}

// JobTerminalSQL es el nombre de la terminal de la impresora de un trabajo (NULL si no es de una terminal), para
// que la interfaz diga «La Caja 2 está desconectada».
func JobTerminalSQL(alias string) string {
	return `(SELECT agent_terminal.name FROM app.devices agent_terminal WHERE agent_terminal.id::text=` + agentDeviceSQL(alias) + `)`
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
