package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
)

// A center's pool loads only when it resolves names in the center's schema
// alone. Needs the test database (tests/go.sh).
func TestDenmaCheckSearchPath(t *testing.T) {
	host, name := os.Getenv("LISTMONK_db__host"), os.Getenv("LISTMONK_db__database")
	if host == "" || name == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("no test database in CI")
		}
		t.Skip("no test database")
	}
	open := func(params string) *sqlx.DB {
		db, err := sqlx.Open("postgres", fmt.Sprintf("host=%s port=%s user=listmonk password=listmonk dbname=%s sslmode=disable %s",
			host, os.Getenv("LISTMONK_db__port"), name, params))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		return db
	}
	if _, err := open("").Exec(`CREATE SCHEMA IF NOT EXISTS center_check_sp`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = open("").Exec(`DROP SCHEMA IF EXISTS center_check_sp`) })

	for _, c := range []struct {
		params, schema string
		ok             bool
	}{
		{"search_path=center_check_sp", "center_check_sp", true},
		{"search_path=public", "center_check_sp", false},
		{"", "center_check_sp", false},
		{"search_path=center_check_sp,public", "center_check_sp", false},
		{"search_path=center_no_such_schema", "center_no_such_schema", false},
	} {
		if err := denmaCheckSearchPath(open(c.params), c.schema); (err == nil) != c.ok {
			t.Errorf("%q for %s: %v, want ok %v", c.params, c.schema, err, c.ok)
		}
	}
}
