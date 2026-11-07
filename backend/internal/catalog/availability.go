package catalog

import (
	"strings"
	"time"
)

func validateSchedule(schedule Schedule) error {
	for _, date := range []string{schedule.StartsOn, schedule.EndsOn} {
		if date != "" {
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return invalid("Las fechas deben usar AAAA-MM-DD.")
			}
		}
	}
	if schedule.StartsOn != "" && schedule.EndsOn != "" && schedule.StartsOn > schedule.EndsOn {
		return invalid("La fecha inicial no puede ser posterior a la final.")
	}
	seenDays := map[int]bool{}
	for _, day := range schedule.Days {
		if day < 0 || day > 6 || seenDays[day] {
			return invalid("Los días deben ser únicos, desde domingo 0 hasta sábado 6.")
		}
		seenDays[day] = true
	}
	for index, window := range schedule.Windows {
		_, startError := time.Parse("15:04", window.Start)
		_, endError := time.Parse("15:04", window.End)
		validEnd := endError == nil || window.End == "24:00"
		if startError != nil || !validEnd || window.Start >= window.End || len(window.Start) != 5 || len(window.End) != 5 {
			return invalid("Cada horario requiere HH:mm inicial anterior al final; separa los horarios que cruzan medianoche.")
		}
		for _, previous := range schedule.Windows[:index] {
			if window.Start < previous.End && previous.Start < window.End {
				return invalid("Los intervalos horarios no deben superponerse.")
			}
		}
	}
	return nil
}

func scheduleAvailable(schedule Schedule, now time.Time, location *time.Location) bool {
	local := now.In(location)
	date := local.Format("2006-01-02")
	if schedule.StartsOn != "" && date < schedule.StartsOn {
		return false
	}
	if schedule.EndsOn != "" && date > schedule.EndsOn {
		return false
	}
	if len(schedule.Days) > 0 {
		found := false
		for _, day := range schedule.Days {
			found = found || day == int(local.Weekday())
		}
		if !found {
			return false
		}
	}
	if len(schedule.Windows) == 0 {
		return true
	}
	clock := local.Format("15:04")
	for _, window := range schedule.Windows {
		if strings.Compare(clock, window.Start) >= 0 && strings.Compare(clock, window.End) < 0 {
			return true
		}
	}
	return false
}
