package database

import (
	"testing"
	"testing/fstest"
)

func TestMigrationOrderAndChecksum(t *testing.T) {
	files := fstest.MapFS{"0001_first.sql": &fstest.MapFile{Data: []byte("SELECT 1;")}}
	first, err := ReadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	files["0001_first.sql"].Data = []byte("SELECT 2;")
	second, _ := ReadMigrations(files)
	if first[0].Checksum == second[0].Checksum {
		t.Fatal("checksum failed to detect change")
	}
	files["0003_gap.sql"] = &fstest.MapFile{Data: []byte("SELECT 3;")}
	if _, err := ReadMigrations(files); err == nil {
		t.Fatal("gap accepted")
	}
	if _, err := ReadMigrations(fstest.MapFS{}); err == nil {
		t.Fatal("empty migrations accepted")
	}
}
