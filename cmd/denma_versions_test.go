package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
)

// versionsTx gives a transaction on the test database (go.sh) with the history's tables made in it, and rolls it
// back at the end.
func versionsTx(t *testing.T) *sqlx.Tx {
	t.Helper()
	host, name := os.Getenv("LISTMONK_db__host"), os.Getenv("LISTMONK_db__database")
	if host == "" || name == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("no test database in CI")
		}
		t.Skip("no test database")
	}
	db, err := sqlx.Connect("postgres", fmt.Sprintf("host=%s port=%s user=listmonk password=listmonk dbname=%s sslmode=disable",
		host, os.Getenv("LISTMONK_db__port"), name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback() })
	for _, q := range []string{"SET LOCAL check_function_bodies = off", denmaCampaignColumnsSQL, denmaPreviewTextSQL, denmaReplyToSQL,
		denmaFromContactSQL, denmaAutoTables, denmaVersionsSQL, "SET CONSTRAINTS ALL IMMEDIATE"} {
		if _, err := tx.Exec(q); err != nil {
			t.Fatalf("%v\n%.200s", err, q)
		}
	}
	return tx
}

func versionsCount(t *testing.T, tx *sqlx.Tx, where string) int {
	t.Helper()
	var n int
	if err := tx.Get(&n, `SELECT COUNT(*) FROM denma_versions WHERE `+where); err != nil {
		t.Fatal(err)
	}
	return n
}

// An item keeps its newest 30 saves, and the newest 5 of its set-aside edits apart from them.
func TestDenmaVersionsAsideCap(t *testing.T) {
	tx := versionsTx(t)
	var id int
	if err := tx.Get(&id, `INSERT INTO templates (name, type, subject, body) VALUES ('versions-test', 'campaign', '', 'x') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	k := denmaVersionKinds["template"]
	var last int64
	for i := 0; i < 40; i++ {
		if _, err := tx.Exec(`SELECT set_config('denma.bumped', '', true)`); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`UPDATE templates SET body = $2 WHERE id = $1`, id, fmt.Sprintf("body %d", i)); err != nil {
			t.Fatal(err)
		}
		if i%4 == 0 {
			var err error
			if last, err = denmaAddAside(tx, k, id, i, 0, "Me", "", []byte(`{"body": "mine"}`)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if n := versionsCount(t, tx, fmt.Sprintf("template_id = %d AND NOT set_aside", id)); n != denmaVersionsMax {
		t.Errorf("%d saves kept, want %d", n, denmaVersionsMax)
	}
	if n := versionsCount(t, tx, fmt.Sprintf("template_id = %d AND set_aside", id)); n != denmaVersionAsideKeep {
		t.Errorf("%d edits set aside kept, want %d", n, denmaVersionAsideKeep)
	}
	if n := versionsCount(t, tx, fmt.Sprintf("id = %d", last)); n != 1 {
		t.Error("the newest edit set aside isn't kept")
	}
	var body string
	if err := tx.Get(&body, `SELECT data->>'body' FROM denma_versions WHERE template_id = $1 AND NOT set_aside ORDER BY id DESC LIMIT 1`, id); err != nil || body != "body 39" {
		t.Errorf("the newest save is %q (%v)", body, err)
	}
}

// A year on, the prune leaves each template's and automation's newest save, and deletes what's older.
func TestDenmaVersionsPruneKeepsNewest(t *testing.T) {
	tx := versionsTx(t)
	var only, many, auto int
	if err := tx.Get(&only, `INSERT INTO templates (name, type, subject, body) VALUES ('versions-only', 'campaign', '', 'x') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Get(&many, `INSERT INTO templates (name, type, subject, body) VALUES ('versions-many', 'campaign', '', 'x') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Get(&auto, `INSERT INTO denma_automations (uuid, name, subject, trigger_type, send_email)
		VALUES (gen_random_uuid(), 'versions-auto', '', 'joins', false) RETURNING id`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := tx.Exec(`SELECT set_config('denma.bumped', '', true)`); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`UPDATE templates SET body = $2 WHERE id = $1`, many, fmt.Sprintf("body %d", i)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`UPDATE denma_automations SET name = $2 WHERE id = $1`, auto, fmt.Sprintf("versions-auto %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := denmaAddAside(tx, denmaVersionKinds["template"], many, 1, 0, "Me", "", []byte(`{"body": "mine"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE denma_versions SET saved_at = NOW() - INTERVAL '2 years' WHERE template_id IN ($1, $2) OR automation_id = $3`, only, many, auto); err != nil {
		t.Fatal(err)
	}

	if _, err := denmaPruneVersions(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		what, where string
		want        int
	}{
		{"a template's only version", fmt.Sprintf("template_id = %d", only), 1},
		{"a template's saves, the newest", fmt.Sprintf("template_id = %d AND NOT set_aside", many), 1},
		{"a template's edits set aside", fmt.Sprintf("template_id = %d AND set_aside", many), 0},
		{"an automation's saves, the newest", fmt.Sprintf("automation_id = %d", auto), 1},
	} {
		if n := versionsCount(t, tx, c.where); n != c.want {
			t.Errorf("%s: %d kept, want %d", c.what, n, c.want)
		}
	}
	var body, name string
	if err := tx.Get(&body, `SELECT data->>'body' FROM denma_versions WHERE template_id = $1 AND NOT set_aside`, many); err != nil || body != "body 2" {
		t.Errorf("the template's newest save is %q (%v)", body, err)
	}
	if err := tx.Get(&name, `SELECT data->>'name' FROM denma_versions WHERE automation_id = $1`, auto); err != nil || name != "versions-auto 2" {
		t.Errorf("the automation's newest save is %q (%v)", name, err)
	}
}

// An editor is recorded only for an item that exists, and a viewer isn't recorded.
func TestDenmaEditingOf(t *testing.T) {
	tx := versionsTx(t)
	var id int
	if err := tx.Get(&id, `INSERT INTO templates (name, type, subject, body) VALUES ('editing-test', 'campaign', '', 'x') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	k := denmaVersionKinds["template"]
	key := "editing-test/template/1"
	other := "editing-test/template/2"
	t.Cleanup(func() {
		denmaEditors.Lock()
		delete(denmaEditors.m, key)
		delete(denmaEditors.m, other)
		denmaEditors.Unlock()
	})
	has := func(k string) bool {
		denmaEditors.Lock()
		defer denmaEditors.Unlock()
		return len(denmaEditors.m[k]) > 0
	}

	_, err := denmaEditingOf(tx, k, other, 999999999, "tab-gone-1", 1, "Jane", false, false)
	var he *echo.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusNotFound {
		t.Errorf("a missing item: %v, want 404", err)
	}
	if has(other) {
		t.Error("a missing item's editor is recorded")
	}

	out, err := denmaEditingOf(tx, k, key, id, "tab-jane-1", 1, "Jane", false, false)
	if err != nil || len(out.Others) != 0 || !has(key) || out.Version == 0 {
		t.Errorf("an editor: %+v, %v", out, err)
	}
	out, err = denmaEditingOf(tx, k, key, id, "tab-view-1", 2, "Sam", false, true)
	if err != nil || len(out.Others) != 1 || out.Others[0] != "Jane" {
		t.Errorf("a viewer is told who edits: %+v, %v", out, err)
	}
	out, err = denmaEditingOf(tx, k, key, id, "tab-jane-1", 1, "Jane", false, false)
	if err != nil || len(out.Others) != 0 {
		t.Errorf("a viewer is listed as an editor: %+v, %v", out, err)
	}
}
