package inventory

import (
	"time"

	"local/ad-el-pos/server/internal/businessday"
)

// Día operativo (§0-B «Cancelaciones sin clasificar»). Se define con la hora de corte del
// establecimiento (inventory_settings.operating_day_cutoff, 05:00 por omisión) en su zona horaria
// (establishment.timezone) y no depende de los turnos de caja: un instante pertenece al día cuya
// fecha local tiene la hora de corte en o antes de él, hasta el corte del día siguiente. Una
// cancelación a las 02:00 del 11 con corte a las 05:00 pertenece al día operativo del 10 y vence
// a las 05:00 del 11.
//
// Desde la etapa 9A el cálculo vive en el paquete neutral businessday (la «Hora de inicio del día»
// de los reportes es la misma hora de corte); inventario lo usa sin cambiar su comportamiento,
// incluidos los cambios de horario.

// OperatingDay es el día operativo de un instante y el corte que lo cierra.
type OperatingDay = businessday.Day

// operatingDayOf calcula el día operativo de un instante con la hora de corte hour:minute.
func operatingDayOf(instant time.Time, location *time.Location, hour, minute int) OperatingDay {
	return businessday.DayOf(instant, location, hour, minute)
}
