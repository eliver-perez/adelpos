package backupschedule

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// fakeTarget sustituye al respaldo de la instalación: escribe un archivo pequeño en lugar de pg_dump y lo «verifica»
// leyendo su contenido. Puede fallar a pedido en la creación o en la verificación, y cuenta las llamadas.
type fakeTarget struct {
	mutex       sync.Mutex
	directory   string
	database    string
	protected   []string
	failDump    bool
	failVerify  bool
	dumps       []string
	verified    []string
	block       chan struct{}
	dumpStarted chan struct{}
}

func newFakeTarget(directory string) *fakeTarget {
	return &fakeTarget{directory: directory, database: "adel_pos_test_respaldos"}
}

func (target *fakeTarget) Backup(ctx context.Context) (string, error) {
	return "", errors.New("no se usa en estas pruebas")
}

func (target *fakeTarget) DefaultDirectory() string {
	return target.directory
}

func (target *fakeTarget) ProtectedDirectories() []string {
	return target.protected
}

func (target *fakeTarget) DatabaseName() string {
	return target.database
}

func (target *fakeTarget) Dump(ctx context.Context, filename string) error {
	target.mutex.Lock()
	target.dumps = append(target.dumps, filename)
	fail := target.failDump
	block := target.block
	started := target.dumpStarted
	target.mutex.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if fail {
		return errors.New("fallo simulado de pg_dump")
	}
	return os.WriteFile(filename, []byte("PGDMP respaldo simulado"), 0600)
}

func (target *fakeTarget) Verify(ctx context.Context, filename string) error {
	target.mutex.Lock()
	target.verified = append(target.verified, filepath.Base(filename))
	fail := target.failVerify
	target.mutex.Unlock()
	if fail {
		return errors.New("fallo simulado de pg_restore --list")
	}
	content, err := os.ReadFile(filename)
	if err != nil || string(content) != "PGDMP respaldo simulado" {
		return errors.New("respaldo ilegible")
	}
	return nil
}

func (target *fakeTarget) set(change func(*fakeTarget)) {
	target.mutex.Lock()
	defer target.mutex.Unlock()
	change(target)
}

func (target *fakeTarget) dumpCount() int {
	target.mutex.Lock()
	defer target.mutex.Unlock()
	return len(target.dumps)
}
