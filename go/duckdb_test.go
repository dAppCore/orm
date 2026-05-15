// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"os"
	"path/filepath"
	"testing"

	core "dappco.re/go"
)

// TestDuckDB_RoundTrip_Good — open a fresh DuckDB file, register a
// schema, save a row, find it by PK, where-query it, delete it,
// confirm gone. Round-trip the persistence promise end-to-end.
func TestDuckDB_RoundTrip_Good(t *testing.T) {
	dir, err := os.MkdirTemp("", "orm-duck-")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "test.duckdb")

	r := NewDuckDB(path)
	if !r.OK {
		t.Fatalf("open: %v", r.Error())
	}
	d := r.Value.(*DuckDBMedium)
	defer d.Close()

	schema := Define(func(b *Builder) {
		b.Name("notes")
		b.PK("id")
		b.String("id").NotNull()
		b.String("title").NotNull()
		b.String("body")
	})
	d.RegisterTable("notes", schema)

	// Save a row via WriteIntent.
	row := map[string]any{"id": "n1", "title": "hello", "body": "world"}
	w := d.Write(core.Background(), WriteIntent{
		Op: OpSave, Schema: schema, Rows: []any{row},
	})
	if !w.OK {
		t.Fatalf("write: %v", w.Error())
	}

	// PK lookup.
	rd := d.Read(core.Background(), ReadIntent{
		Schema: schema, PK: []any{"n1"},
	})
	if !rd.OK {
		t.Fatalf("read: %v", rd.Error())
	}
	pl := rd.Value.(*Payload)
	rows := pl.Data.([]map[string]any)
	if len(rows) != 1 || rows[0]["title"] != "hello" {
		t.Fatalf("expected [{title:hello}], got %v", rows)
	}

	// Where-query.
	rd = d.Read(core.Background(), ReadIntent{
		Schema: schema,
		Where:  []Predicate{{Field: "body", Op: "=", Value: "world"}},
	})
	if !rd.OK {
		t.Fatalf("where read: %v", rd.Error())
	}
	if len(rd.Value.(*Payload).Data.([]map[string]any)) != 1 {
		t.Fatalf("where read should match 1 row")
	}

	// Re-save (upsert) — should overwrite, not duplicate.
	row2 := map[string]any{"id": "n1", "title": "updated", "body": "world"}
	w = d.Write(core.Background(), WriteIntent{
		Op: OpSave, Schema: schema, Rows: []any{row2},
	})
	if !w.OK {
		t.Fatalf("re-save: %v", w.Error())
	}
	rd = d.Read(core.Background(), ReadIntent{Schema: schema})
	rows = rd.Value.(*Payload).Data.([]map[string]any)
	if len(rows) != 1 || rows[0]["title"] != "updated" {
		t.Fatalf("upsert failed: %v", rows)
	}

	// Delete.
	w = d.Write(core.Background(), WriteIntent{
		Op: OpDelete, Schema: schema,
		Where: []Predicate{{Field: "id", Op: "=", Value: "n1"}},
	})
	if !w.OK {
		t.Fatalf("delete: %v", w.Error())
	}
	rd = d.Read(core.Background(), ReadIntent{Schema: schema})
	if len(rd.Value.(*Payload).Data.([]map[string]any)) != 0 {
		t.Fatalf("delete failed — rows remain")
	}
}

// TestDuckDB_Persists_Good — save a row, close, reopen the same
// path, confirm row still there. The whole point of the medium.
func TestDuckDB_Persists_Good(t *testing.T) {
	dir, err := os.MkdirTemp("", "orm-duck-")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "persist.duckdb")

	schema := Define(func(b *Builder) {
		b.Name("kv")
		b.PK("k")
		b.String("k").NotNull()
		b.String("v")
	})

	// Open #1, write, close.
	r1 := NewDuckDB(path)
	if !r1.OK {
		t.Fatalf("open1: %v", r1.Error())
	}
	d1 := r1.Value.(*DuckDBMedium)
	d1.RegisterTable("kv", schema)
	w := d1.Write(core.Background(), WriteIntent{
		Op: OpSave, Schema: schema,
		Rows: []any{map[string]any{"k": "name", "v": "snider"}},
	})
	if !w.OK {
		t.Fatalf("write: %v", w.Error())
	}
	if r := d1.Close(); !r.OK {
		t.Fatalf("close1: %v", r.Error())
	}

	// Open #2 — reads back.
	r2 := NewDuckDB(path)
	if !r2.OK {
		t.Fatalf("open2: %v", r2.Error())
	}
	d2 := r2.Value.(*DuckDBMedium)
	defer d2.Close()
	d2.RegisterTable("kv", schema) // CREATE IF NOT EXISTS — no-op
	rd := d2.Read(core.Background(), ReadIntent{Schema: schema, PK: []any{"name"}})
	if !rd.OK {
		t.Fatalf("read2: %v", rd.Error())
	}
	rows := rd.Value.(*Payload).Data.([]map[string]any)
	if len(rows) != 1 || rows[0]["v"] != "snider" {
		t.Fatalf("persistence broken: got %v", rows)
	}
}

// TestDuckDB_Mount_Ugly — bad path returns Fail, doesn't panic.
func TestDuckDB_Mount_Ugly(t *testing.T) {
	r := NewDuckDB("/this/path/does/not/exist/and/cant/be/created/orm.duckdb")
	if r.OK {
		t.Fatalf("expected Fail for unreachable path")
	}
}
