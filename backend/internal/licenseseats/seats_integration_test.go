//go:build integration

package licenseseats_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/licenseseats"
	"local/ad-el-pos/server/internal/licenseseats/seatstest"
)

// Pruebas del gestor de cupos contra PostgreSQL (etapa L1, Fase D), sin el resto del servidor: la asignación atómica
// con transacciones simultáneas en conexiones distintas, y cada causa de ocupar y liberar con el reloj simulado. El
// recorrido por la API real está en internal/operations (stagel1_seats_test.go).

type seatsDatabase struct {
	t    *testing.T
	ctx  context.Context
	pool *pgxpool.Pool
	user string
}

// newSeatsDatabase migra una base temporal desde cero y crea el usuario dueño de las sesiones y los turnos.
func newSeatsDatabase(t *testing.T) *seatsDatabase {
	t.Helper()
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Una conexión por transacción simultánea, más las de la preparación.
	config.MaxConns = 24
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	reset := func() {
		if _, err := pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
			t.Fatal(err)
		}
	}
	reset()
	t.Cleanup(reset)
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	db := &seatsDatabase{t: t, ctx: ctx, pool: pool}
	var role string
	if err := pool.QueryRow(ctx, "INSERT INTO app.roles(name) VALUES ('Rol de cupos') RETURNING id::text").Scan(&role); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO app.users(username,display_name,role_id) VALUES ('cupos','Cupos',$1)
        RETURNING id::text`, role).Scan(&db.user); err != nil {
		t.Fatal(err)
	}
	return db
}

func randomHex() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}

// terminal es una terminal de escritorio con una sesión vigente (y, si se pide, su propia caja física).
type terminal struct {
	device   string
	session  string
	register string
}

func (db *seatsDatabase) terminal(name string, withRegister bool) terminal {
	db.t.Helper()
	var created terminal
	if withRegister {
		if err := db.pool.QueryRow(db.ctx, "INSERT INTO app.registers(name) VALUES ($1) RETURNING id::text", "Caja "+name).
			Scan(&created.register); err != nil {
			db.t.Fatal(err)
		}
	}
	err := db.pool.QueryRow(db.ctx, `INSERT INTO app.devices(name,secret_hash,register_id) VALUES ($1,$2,nullif($3,'')::uuid)
        RETURNING id::text`, name, randomHex(), created.register).Scan(&created.device)
	if err != nil {
		db.t.Fatal(err)
	}
	created.session = db.openSession(created.device)
	return created
}

// openSession abre una sesión vigente doce horas, como issueSession (expires_at con la hora de Go).
func (db *seatsDatabase) openSession(device string) string {
	db.t.Helper()
	var session string
	if _, err := db.pool.Exec(db.ctx, "UPDATE app.sessions SET revoked_at=now(),revoke_reason='replaced' WHERE device_id=$1 AND revoked_at IS NULL", device); err != nil {
		db.t.Fatal(err)
	}
	err := db.pool.QueryRow(db.ctx, `INSERT INTO app.sessions(user_id,device_id,token_hash,expires_at) VALUES ($1,$2,$3,$4)
        RETURNING id::text`, db.user, device, randomHex(), time.Now().Add(12*time.Hour)).Scan(&session)
	if err != nil {
		db.t.Fatal(err)
	}
	return session
}

func (db *seatsDatabase) closeSessions(device string) {
	db.t.Helper()
	if _, err := db.pool.Exec(db.ctx, "UPDATE app.sessions SET revoked_at=now(),revoke_reason='logout' WHERE device_id=$1 AND revoked_at IS NULL", device); err != nil {
		db.t.Fatal(err)
	}
}

// openShift abre un turno en la caja de la terminal (como payments, con su clave de operación).
func (db *seatsDatabase) openShift(opener terminal) string {
	db.t.Helper()
	tx, err := db.pool.Begin(db.ctx)
	if err != nil {
		db.t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var operation, shift string
	if err := tx.QueryRow(db.ctx, `INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        VALUES (gen_random_uuid(),$1,$2,'POST payments/shifts','prueba','{}') RETURNING id::text`, db.user, opener.device).
		Scan(&operation); err != nil {
		db.t.Fatal(err)
	}
	if err := tx.QueryRow(db.ctx, `INSERT INTO app.cash_shifts(id,register_id,opening_float,opened_by,opened_device_id,operation_id)
        VALUES (gen_random_uuid(),$1,0,$2,$3,$4) RETURNING id::text`, opener.register, db.user, opener.device, operation).
		Scan(&shift); err != nil {
		db.t.Fatal(err)
	}
	if err := tx.Commit(db.ctx); err != nil {
		db.t.Fatal(err)
	}
	return shift
}

func (db *seatsDatabase) closeShift(shift string) {
	db.t.Helper()
	if _, err := db.pool.Exec(db.ctx, `UPDATE app.cash_shifts SET state='closed',closed_at=now(),closed_by=$2,
        closed_device_id=opened_device_id WHERE id=$1`, shift, db.user); err != nil {
		db.t.Fatal(err)
	}
}

// within ejecuta fn en una transacción propia y la confirma si no falla.
func (db *seatsDatabase) within(fn func(tx pgx.Tx) error) error {
	db.t.Helper()
	tx, err := db.pool.Begin(db.ctx)
	if err != nil {
		db.t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(db.ctx)
}

// usage es el conteo de cupos en uso del gestor.
func (db *seatsDatabase) usage(manager *licenseseats.Manager) map[licenseseats.Seat]licenseseats.Usage {
	db.t.Helper()
	var result map[licenseseats.Seat]licenseseats.Usage
	if err := db.within(func(tx pgx.Tx) error {
		var err error
		result, err = manager.Usage(db.ctx, tx)
		return err
	}); err != nil {
		db.t.Fatal(err)
	}
	return result
}

func (db *seatsDatabase) inUse(manager *licenseseats.Manager, seat licenseseats.Seat) int {
	db.t.Helper()
	return db.usage(manager)[seat].InUse
}

func (db *seatsDatabase) rows(kind string) int {
	db.t.Helper()
	var count int
	if err := db.pool.QueryRow(db.ctx, "SELECT count(*)::int FROM app.license_seats WHERE kind=$1", kind).Scan(&count); err != nil {
		db.t.Fatal(err)
	}
	return count
}

// unavailable exige el rechazo por cupo con sus números.
func unavailable(t *testing.T, err error, seat licenseseats.Seat, inUse, limit int) {
	t.Helper()
	var problem *licenseseats.UnavailableError
	if !errors.As(err, &problem) || problem.Seat != seat || problem.InUse != inUse || problem.Limit != limit {
		t.Fatalf("esperado sin cupo de %s (%d de %d): %v", seat, inUse, limit, err)
	}
}

// TestSeatAssignmentIsAtomic: varias transacciones en conexiones distintas piden a la vez el último cupo de cada tipo, y
// solo una lo obtiene; se repite varias veces en la misma corrida. Cada transacción espera a que todas hayan empezado
// antes de pedirlo, así que todas compiten por el mismo cupo.
func TestSeatAssignmentIsAtomic(t *testing.T) {
	db := newSeatsDatabase(t)
	const rounds = 12
	const contenders = 8
	source := seatstest.NewSource(2, 2, 2)
	manager := licenseseats.NewManager(licenseseats.Options{Limits: source})
	// La terminal que ya ocupa uno de los dos cupos de cada tipo durante toda la prueba.
	holder := db.terminal("Ocupa uno", true)
	holderShift := db.openShift(holder)
	if err := db.within(func(tx pgx.Tx) error {
		if err := manager.Capture(db.ctx, tx, holder.device); err != nil {
			return err
		}
		return manager.SetKitchenDisplay(db.ctx, tx, holder.device, holder.session, true)
	}); err != nil {
		t.Fatal(err)
	}
	other := db.terminal("Comandero fijo", false)
	if err := db.within(func(tx pgx.Tx) error {
		return manager.Capture(db.ctx, tx, other.device)
	}); err != nil {
		t.Fatal(err)
	}
	if usage := db.usage(manager); usage[licenseseats.CashRegisters].InUse != 1 || usage[licenseseats.OrderTakers].InUse != 1 ||
		usage[licenseseats.KitchenDisplays].InUse != 1 {
		t.Fatalf("cupos ocupados antes de competir: %+v", usage)
	}
	type contender struct {
		name  string
		seat  licenseseats.Seat
		claim func(tx pgx.Tx, member terminal) error
		// prepare crea la terminal que compite (con su caja y su turno, si los necesita).
		prepare func(index int) terminal
		// clean deja libre otra vez el cupo ganado.
		clean func(winner terminal)
	}
	kinds := []contender{
		{
			name: "comandero", seat: licenseseats.OrderTakers,
			claim: func(tx pgx.Tx, member terminal) error {
				return manager.Capture(db.ctx, tx, member.device)
			},
			prepare: func(index int) terminal {
				return db.terminal(fmt.Sprintf("Tableta %d %s", index, randomHex()[:6]), false)
			},
			clean: func(winner terminal) {
				db.closeSessions(winner.device)
			},
		},
		{
			name: "caja", seat: licenseseats.CashRegisters,
			claim: func(tx pgx.Tx, member terminal) error {
				return manager.Capture(db.ctx, tx, member.device)
			},
			prepare: func(index int) terminal {
				member := db.terminal(fmt.Sprintf("Caja %d %s", index, randomHex()[:6]), true)
				db.openShift(member)
				return member
			},
			clean: func(winner terminal) {
				var shift string
				if err := db.pool.QueryRow(db.ctx, `SELECT shift_id::text FROM app.license_seats
                    WHERE device_id=$1 AND kind='cash_register'`, winner.device).Scan(&shift); err != nil {
					t.Fatal(err)
				}
				db.closeShift(shift)
			},
		},
		{
			name: "cocina", seat: licenseseats.KitchenDisplays,
			claim: func(tx pgx.Tx, member terminal) error {
				return manager.SetKitchenDisplay(db.ctx, tx, member.device, member.session, true)
			},
			prepare: func(index int) terminal {
				return db.terminal(fmt.Sprintf("Cocina %d %s", index, randomHex()[:6]), false)
			},
			clean: func(winner terminal) {
				if err := db.within(func(tx pgx.Tx) error {
					return manager.SetKitchenDisplay(db.ctx, tx, winner.device, winner.session, false)
				}); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, kind := range kinds {
		for round := range rounds {
			members := make([]terminal, contenders)
			for index := range members {
				members[index] = kind.prepare(index)
			}
			started := sync.WaitGroup{}
			started.Add(contenders)
			ready := make(chan struct{})
			results := make([]error, contenders)
			finished := sync.WaitGroup{}
			for index, member := range members {
				finished.Add(1)
				go func() {
					defer finished.Done()
					tx, err := db.pool.Begin(db.ctx)
					if err != nil {
						started.Done()
						results[index] = err
						return
					}
					defer tx.Rollback(context.Background())
					// La transacción ya tiene su conexión y su foto: espera a las demás antes de pedir el cupo.
					if _, err := tx.Exec(db.ctx, "SELECT 1"); err != nil {
						started.Done()
						results[index] = err
						return
					}
					started.Done()
					<-ready
					if err := kind.claim(tx, member); err != nil {
						results[index] = err
						return
					}
					results[index] = tx.Commit(db.ctx)
				}()
			}
			started.Wait()
			close(ready)
			finished.Wait()
			winners := []terminal{}
			for index, err := range results {
				if err == nil {
					winners = append(winners, members[index])
					continue
				}
				unavailable(t, err, kind.seat, 2, 2)
			}
			if len(winners) != 1 {
				t.Fatalf("%s, vuelta %d: %d terminales obtuvieron el último cupo (%v)", kind.name, round+1, len(winners), results)
			}
			if inUse := db.inUse(manager, kind.seat); inUse != 2 {
				t.Fatalf("%s, vuelta %d: %d cupos en uso con límite 2", kind.name, round+1, inUse)
			}
			kind.clean(winners[0])
			if inUse := db.inUse(manager, kind.seat); inUse != 1 {
				t.Fatalf("%s, vuelta %d: el cupo ganado no se liberó (%d en uso)", kind.name, round+1, inUse)
			}
		}
	}
	// La terminal fija conservó sus cupos todo el tiempo.
	db.closeShift(holderShift)
	if inUse := db.inUse(manager, licenseseats.CashRegisters); inUse != 0 {
		t.Fatalf("cajas en uso al cerrar el último turno: %d", inUse)
	}
}

// TestSeatRules recorre cada causa de ocupar y liberar con el reloj simulado (L1-D16).
func TestSeatRules(t *testing.T) {
	db := newSeatsDatabase(t)
	source := seatstest.NewSource(1, 1, 1)
	clock := seatstest.NewClock(time.Now())
	manager := licenseseats.NewManager(licenseseats.Options{Limits: source, Now: clock.Now})
	capture := func(member terminal) error {
		return db.within(func(tx pgx.Tx) error {
			return manager.Capture(db.ctx, tx, member.device)
		})
	}
	renew := func(member terminal) {
		t.Helper()
		if err := db.within(func(tx pgx.Tx) error {
			return manager.Renew(db.ctx, tx, member.device)
		}); err != nil {
			t.Fatal(err)
		}
	}
	kitchen := func(member terminal, active bool) error {
		return db.within(func(tx pgx.Tx) error {
			return manager.SetKitchenDisplay(db.ctx, tx, member.device, member.session, active)
		})
	}
	sweep := func() {
		t.Helper()
		if err := db.within(func(tx pgx.Tx) error {
			return manager.Sweep(db.ctx, tx)
		}); err != nil {
			t.Fatal(err)
		}
	}
	first := db.terminal("Comandero uno", false)
	second := db.terminal("Comandero dos", false)

	t.Run("comandero: 15 minutos sin captura, con la actividad renovada por las previas", func(t *testing.T) {
		// Una previa sin cupo no ocupa nada.
		renew(first)
		if rows := db.rows("order_taker"); rows != 0 {
			t.Fatalf("la previa ocupó un cupo: %d", rows)
		}
		if err := capture(first); err != nil {
			t.Fatal(err)
		}
		unavailable(t, capture(second), licenseseats.OrderTakers, 1, 1)
		clock.Advance(10 * time.Minute)
		renew(first)
		clock.Advance(14 * time.Minute)
		// 24 minutos desde la captura, 14 desde la previa: sigue ocupado.
		unavailable(t, capture(second), licenseseats.OrderTakers, 1, 1)
		clock.Advance(time.Minute)
		// Justo 15 minutos sin actividad: se libera.
		if inUse := db.inUse(manager, licenseseats.OrderTakers); inUse != 0 {
			t.Fatalf("comanderos en uso a los 15 minutos: %d", inUse)
		}
		// La previa de una terminal cuyo cupo venció no lo recupera.
		renew(first)
		if inUse := db.inUse(manager, licenseseats.OrderTakers); inUse != 0 {
			t.Fatalf("la previa recuperó un cupo vencido: %d", inUse)
		}
		if err := capture(second); err != nil {
			t.Fatal(err)
		}
		unavailable(t, capture(first), licenseseats.OrderTakers, 1, 1)
	})

	t.Run("comandero: la terminal que se queda sin sesiones lo libera y no lo recupera al volver a entrar", func(t *testing.T) {
		db.closeSessions(second.device)
		if inUse := db.inUse(manager, licenseseats.OrderTakers); inUse != 0 {
			t.Fatalf("comanderos en uso sin sesiones: %d", inUse)
		}
		sweep()
		second.session = db.openSession(second.device)
		if inUse := db.inUse(manager, licenseseats.OrderTakers); inUse != 0 {
			t.Fatalf("la sesión nueva recuperó el cupo sin pedirlo: %d", inUse)
		}
		// Sin barrido, la primera asignación que lo ve libre lo borra: tampoco vuelve.
		db.closeSessions(first.device)
		first.session = db.openSession(first.device)
		if err := capture(first); err != nil {
			t.Fatal(err)
		}
		db.closeSessions(first.device)
		if err := capture(second); err != nil {
			t.Fatal(err)
		}
		first.session = db.openSession(first.device)
		if usage := db.inUse(manager, licenseseats.OrderTakers); usage != 1 {
			t.Fatalf("comanderos en uso: %d", usage)
		}
		unavailable(t, capture(first), licenseseats.OrderTakers, 1, 1)
	})

	t.Run("caja por terminal: abrir turno, capturar en la caja compartida y cerrar el turno", func(t *testing.T) {
		source.Set(2, 1, 1)
		opener := db.terminal("Caja con turno", true)
		shift := db.openShift(opener)
		// La segunda terminal de la misma caja física.
		var partner terminal
		if err := db.pool.QueryRow(db.ctx, `INSERT INTO app.devices(name,secret_hash,register_id) VALUES ('Caja compartida',$1,$2)
            RETURNING id::text`, randomHex(), opener.register).Scan(&partner.device); err != nil {
			t.Fatal(err)
		}
		partner.session = db.openSession(partner.device)
		// La terminal comandera de esa caja deja de serlo al abrir el turno.
		if err := db.within(func(tx pgx.Tx) error {
			return manager.OpenShift(db.ctx, tx, opener.device)
		}); err != nil {
			t.Fatal(err)
		}
		if err := capture(partner); err != nil {
			t.Fatal(err)
		}
		// La terminal que ya tiene su caja captura otra vez sin ocupar otra.
		if err := capture(partner); err != nil {
			t.Fatal(err)
		}
		if inUse := db.inUse(manager, licenseseats.CashRegisters); inUse != 2 {
			t.Fatalf("cajas en uso con dos terminales en la misma caja: %d", inUse)
		}
		third := db.terminal("Tercera caja", true)
		db.openShift(third)
		unavailable(t, capture(third), licenseseats.CashRegisters, 2, 2)
		db.closeShift(shift)
		if inUse := db.inUse(manager, licenseseats.CashRegisters); inUse != 0 {
			t.Fatalf("cajas en uso al cerrar el turno compartido: %d", inUse)
		}
		if err := capture(third); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("abrir turno libera los comanderos de esa caja", func(t *testing.T) {
		source.Set(3, 3, 1)
		cashier := db.terminal("Comandero que abre", true)
		if err := capture(cashier); err != nil {
			t.Fatal(err)
		}
		before := db.inUse(manager, licenseseats.OrderTakers)
		db.openShift(cashier)
		if err := db.within(func(tx pgx.Tx) error {
			return manager.OpenShift(db.ctx, tx, cashier.device)
		}); err != nil {
			t.Fatal(err)
		}
		if after := db.inUse(manager, licenseseats.OrderTakers); after != before-1 {
			t.Fatalf("comanderos en uso antes y después de abrir turno: %d y %d", before, after)
		}
	})

	t.Run("cocina: entrar, salir y cerrar la sesión que entró", func(t *testing.T) {
		source.Set(3, 3, 1)
		screen := db.terminal("Pantalla de cocina", false)
		other := db.terminal("Otra pantalla", false)
		if err := kitchen(screen, true); err != nil {
			t.Fatal(err)
		}
		// Entrar otra vez desde la misma terminal no ocupa otro cupo.
		if err := kitchen(screen, true); err != nil {
			t.Fatal(err)
		}
		unavailable(t, kitchen(other, true), licenseseats.KitchenDisplays, 1, 1)
		if err := kitchen(screen, false); err != nil {
			t.Fatal(err)
		}
		if err := kitchen(other, true); err != nil {
			t.Fatal(err)
		}
		// La sesión que entró se cierra: se libera aunque la terminal abra otra.
		db.closeSessions(other.device)
		other.session = db.openSession(other.device)
		if inUse := db.inUse(manager, licenseseats.KitchenDisplays); inUse != 0 {
			t.Fatalf("pantallas de cocina en uso al cerrar la sesión: %d", inUse)
		}
		if err := kitchen(screen, true); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("licencia nueva con menos cupos: nadie se interrumpe y no se ocupan nuevos", func(t *testing.T) {
		source.Set(3, 3, 1)
		clock.Advance(20 * time.Minute)
		holders := []terminal{db.terminal("Con cupo 1", false), db.terminal("Con cupo 2", false), db.terminal("Con cupo 3", false)}
		for _, member := range holders {
			if err := capture(member); err != nil {
				t.Fatal(err)
			}
		}
		source.Set(3, 1, 1)
		late := db.terminal("Sin cupo", false)
		for _, member := range holders {
			if err := capture(member); err != nil {
				t.Fatalf("la licencia nueva interrumpió a una terminal con cupo: %v", err)
			}
		}
		unavailable(t, capture(late), licenseseats.OrderTakers, 3, 1)
		db.closeSessions(holders[0].device)
		db.closeSessions(holders[1].device)
		unavailable(t, capture(late), licenseseats.OrderTakers, 1, 1)
		db.closeSessions(holders[2].device)
		if err := capture(late); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("sin licencia válida no se ocupa nada y salir de Preparación sigue funcionando", func(t *testing.T) {
		source.Withdraw()
		member := db.terminal("Sin licencia", false)
		if err := capture(member); !errors.Is(err, licenseseats.ErrNoValidLicense) {
			t.Fatalf("captura sin licencia válida: %v", err)
		}
		if err := kitchen(member, true); !errors.Is(err, licenseseats.ErrNoValidLicense) {
			t.Fatalf("Preparación sin licencia válida: %v", err)
		}
		if err := kitchen(member, false); err != nil {
			t.Fatal(err)
		}
		renew(member)
		if usage := db.usage(manager); usage[licenseseats.OrderTakers].Limit != 0 || usage[licenseseats.CashRegisters].Limit != 0 {
			t.Fatalf("límites sin licencia válida: %+v", usage)
		}
	})
}
