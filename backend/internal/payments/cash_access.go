package payments

// Reglas de acceso y de estado de los turnos de caja. Todas se evalúan en Go, dentro de la
// transacción y antes de cualquier escritura.

// closingMessage explica que la caja está pausada por el arqueo de cierre y quién lo inició.
func closingMessage(shift *Shift) string {
	message := "La caja " + shift.RegisterName + " está en arqueo de cierre"
	if shift.ClosingStartedByName != nil {
		message += " (lo inició " + *shift.ClosingStartedByName + ")"
	}
	return message + ". Registra la operación en otra caja con turno abierto o espera a que termine el arqueo."
}

// shiftStateError rechaza operar sobre un turno pausado por el arqueo de cierre o ya cerrado. El
// servidor es la fuente de verdad: una operación pertenece al turno solo si su transacción se
// confirmó; un reintento posterior recibe este error y debe registrarse como operación nueva.
func shiftStateError(shift *Shift) error {
	switch shift.State {
	case shiftClosing:
		return conflict("SHIFT_CLOSING", closingMessage(shift))
	case shiftClosed:
		return conflict("SHIFT_CLOSED", "El turno de "+shift.RegisterName+" ya está cerrado. Si la operación sigue haciendo falta, regístrala como nueva en un turno abierto.")
	}
	return nil
}

// requireShiftAccess permite trabajar con el turno de la caja de la terminal; los turnos de otras
// cajas exigen cash.shifts.read_all.
func (handler *service) requireShiftAccess(shift *Shift) error {
	register, err := handler.deviceRegister()
	if err != nil {
		return err
	}
	if register != nil && register.ID == shift.RegisterID {
		return nil
	}
	return handler.request.Actor.Require(permissionShiftsReadAll)
}

// readableShift aplica las reglas de consulta de un turno: la ruta ya exigió su permiso base, otra
// caja exige cash.shifts.read_all y un turno cerrado exige cash.history.read, salvo para quien lo
// cerró (que ve lo que declaró y que el turno quedó cerrado).
func (handler *service) readableShift(id string) (*Shift, error) {
	shift, err := handler.readShift("s.id=$1", id)
	if err != nil {
		return nil, err
	}
	if shift == nil {
		return nil, notFound("El turno no existe.")
	}
	if err := handler.requireShiftAccess(shift); err != nil {
		return nil, err
	}
	closedByActor := shift.ClosedBy != nil && *shift.ClosedBy == handler.request.Actor.UserID
	if shift.State == shiftClosed && !closedByActor {
		if err := handler.request.Actor.Require(permissionHistoryRead); err != nil {
			return nil, err
		}
	}
	return shift, nil
}

// operableShift valida el turno en el que se registra una operación de efectivo: debe existir,
// estar abierto (ni en arqueo ni cerrado), pertenecer a la caja de la terminal y permitir al
// actor operar según la política de turno compartido. El identificador viaja en la ruta, de modo
// que el reintento de una operación incierta nunca cae en otro turno.
func (handler *service) operableShift(id string) (*Shift, error) {
	shift, err := handler.readShift("s.id=$1", id)
	if err != nil {
		return nil, err
	}
	if shift == nil {
		return nil, notFound("El turno no existe.")
	}
	if err := shiftStateError(shift); err != nil {
		return nil, err
	}
	register, err := handler.deviceRegister()
	if err != nil {
		return nil, err
	}
	if register == nil || register.ID != shift.RegisterID {
		return nil, conflict("SHIFT_NOT_IN_REGISTER", "El turno pertenece a "+shift.RegisterName+
			". Las operaciones de efectivo se registran desde una terminal asociada a esa caja.")
	}
	if err := handler.requireSharedShift(shift); err != nil {
		return nil, err
	}
	return shift, nil
}

// expectedVisible decide si una respuesta puede incluir el efectivo esperado, las diferencias o
// cualquier total del que se deduzcan. Durante el arqueo de cierre la captura es ciega para todos,
// incluido el administrador; fuera de él depende de cash.expected.read.
func (handler *service) expectedVisible(shift *Shift) bool {
	if shift.State == shiftClosing {
		return false
	}
	return handler.request.Actor.Can(permissionExpectedRead)
}
