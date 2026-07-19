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

// TestDuckDB_Raw_Good — Raw() exposes the underlying *sql.DB so
// callers can issue non-Intent SQL (CREATE VIEW, read_json_auto,
// EXPLAIN). Round-trip a tiny table through Raw end-to-end.
func TestDuckDB_Raw_Good(t *testing.T) {
	dir, err := os.MkdirTemp("", "orm-duck-raw-")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	r := NewDuckDB(filepath.Join(dir, "raw.duckdb"))
	if !r.OK {
		t.Fatalf("open: %v", r.Error())
	}
	d := r.Value.(*DuckDBMedium)
	defer d.Close()

	db := d.Raw()
	if db == nil {
		t.Fatalf("Raw() returned nil on live Medium")
	}

	if _, err := db.Exec(`CREATE TABLE rawkv (k TEXT PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatalf("raw create: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO rawkv VALUES (?, ?)`, "hello", "world"); err != nil {
		t.Fatalf("raw insert: %v", err)
	}

	var v string
	if err := db.QueryRow(`SELECT v FROM rawkv WHERE k = ?`, "hello").Scan(&v); err != nil {
		t.Fatalf("raw select: %v", err)
	}
	if v != "world" {
		t.Fatalf("raw round-trip: got %q want %q", v, "world")
	}
}

// TestDuckDB_Raw_NilReceiver_Bad — Raw() on a nil receiver returns
// nil instead of panicking, so guard callers don't need to nil-check
// the Medium pointer themselves before calling Raw().
func TestDuckDB_Raw_NilReceiver_Bad(t *testing.T) {
	var d *DuckDBMedium
	if db := d.Raw(); db != nil {
		t.Fatalf("Raw() on nil receiver returned %v, want nil", db)
	}
}

// TestDuckDB_Raw_IntentInterop_Mixed — a row written via Raw is
// visible to a subsequent Intent Read against the same table, and
// vice versa. The two surfaces share state; Raw isn't a separate
// connection.
func TestDuckDB_Raw_IntentInterop_Mixed(t *testing.T) {
	dir, err := os.MkdirTemp("", "orm-duck-mixed-")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	r := NewDuckDB(filepath.Join(dir, "mixed.duckdb"))
	if !r.OK {
		t.Fatalf("open: %v", r.Error())
	}
	d := r.Value.(*DuckDBMedium)
	defer d.Close()

	schema := Define(func(b *Builder) {
		b.Name("kv")
		b.PK("k")
		b.String("k").NotNull()
		b.String("v")
	})
	d.RegisterTable("kv", schema)

	// Side A — write via Raw, read via Intent.
	if _, err := d.Raw().Exec(`INSERT INTO kv VALUES (?, ?)`, "from-raw", "alpha"); err != nil {
		t.Fatalf("raw insert: %v", err)
	}
	rd := d.Read(core.Background(), ReadIntent{Schema: schema, PK: []any{"from-raw"}})
	if !rd.OK {
		t.Fatalf("intent read: %v", rd.Error())
	}
	rows := rd.Value.(*Payload).Data.([]map[string]any)
	if len(rows) != 1 || rows[0]["v"] != "alpha" {
		t.Fatalf("raw→intent: got %v", rows)
	}

	// Side B — write via Intent, read via Raw.
	w := d.Write(core.Background(), WriteIntent{
		Op: OpSave, Schema: schema,
		Rows: []any{map[string]any{"k": "from-intent", "v": "beta"}},
	})
	if !w.OK {
		t.Fatalf("intent save: %v", w.Error())
	}
	var got string
	if err := d.Raw().QueryRow(`SELECT v FROM kv WHERE k = ?`, "from-intent").Scan(&got); err != nil {
		t.Fatalf("raw scan: %v", err)
	}
	if got != "beta" {
		t.Fatalf("intent→raw: got %q want %q", got, "beta")
	}
}

// openTestDuckDB opens a fresh DuckDB file under a per-test temp dir and
// registers cleanup — the setup every test above hand-rolls. Factored out
// here since the predicate/alias regression tests below add enough of
// them to make the duplication worth cutting (mirrors setupBridgeTest's
// role for the Memium-backed tests in bridge_test.go).
func openTestDuckDB(t *testing.T, name string) *DuckDBMedium {
	t.Helper()
	dir, err := os.MkdirTemp("", "orm-duck-"+name+"-")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	r := NewDuckDB(filepath.Join(dir, name+".duckdb"))
	if !r.OK {
		t.Fatalf("open: %v", r.Error())
	}
	d := r.Value.(*DuckDBMedium)
	t.Cleanup(func() { d.Close() })
	return d
}

// duckNullRow is the fixture model for the WhereNull/WhereNotNull/
// WhereGroup regression tests — exercised through the real Bridge
// (Of[T](c)...) rather than direct Medium.Read calls, since Mantis #45
// was found in consumer use of the query builder, not of the Intent
// shape directly.
type duckNullRow struct {
	ID   string
	Name string
}

func (duckNullRow) Schema() Schema {
	return Define(func(b *Builder) {
		b.Name("duck_null_rows")
		b.PK("ID")
		b.String("ID").NotNull()
		b.String("Name")
	})
}

// TestDuckDB_WhereNull_Good — WhereNull("field") must return only rows
// where the field IS NULL, on the DuckDB lane. Regression for Mantis #45:
// buildDuckDBPredicate matched Predicate.Op against "is null" / "is not
// null", but WhereNull/WhereNotNull (bridge.go) — and the RFC §4.2
// operator table — actually emit "null" / "not null". The switch never
// matched, the clause fell through to the empty default, and the query
// came back completely unfiltered.
func TestDuckDB_WhereNull_Good(t *testing.T) {
	d := openTestDuckDB(t, "wherenull")
	schema := duckNullRow{}.Schema()
	d.RegisterTable(schema.Name, schema)

	c := core.New()
	t.Cleanup(func() { Remove(c) })
	Mount(c, "default", d)

	seed := []map[string]any{
		{"ID": "1", "Name": "alice"},
		{"ID": "2", "Name": nil},
	}
	for _, row := range seed {
		w := d.Write(core.Background(), WriteIntent{Op: OpInsert, Schema: schema, Rows: []any{row}})
		if !w.OK {
			t.Fatalf("seed write %v: %v", row, w.Error())
		}
	}

	res := Of[duckNullRow](c).WhereNull("Name").Get()
	if !res.OK {
		t.Fatalf("WhereNull query: %v", res.Error())
	}
	rows, ok := Cast[[]duckNullRow](res)
	if !ok {
		t.Fatalf("expected []duckNullRow, got %T", res.Value)
	}
	if len(rows) != 1 || rows[0].ID != "2" {
		t.Fatalf(`WhereNull("Name") should return only the null row (id=2), got %+v`, rows)
	}
}

// TestDuckDB_WhereNotNull_Good — the sibling of WhereNull, same root
// cause ("not null" vs "is not null"), same fix.
func TestDuckDB_WhereNotNull_Good(t *testing.T) {
	d := openTestDuckDB(t, "wherenotnull")
	schema := duckNullRow{}.Schema()
	d.RegisterTable(schema.Name, schema)

	c := core.New()
	t.Cleanup(func() { Remove(c) })
	Mount(c, "default", d)

	seed := []map[string]any{
		{"ID": "1", "Name": "alice"},
		{"ID": "2", "Name": nil},
	}
	for _, row := range seed {
		w := d.Write(core.Background(), WriteIntent{Op: OpInsert, Schema: schema, Rows: []any{row}})
		if !w.OK {
			t.Fatalf("seed write %v: %v", row, w.Error())
		}
	}

	res := Of[duckNullRow](c).WhereNotNull("Name").Get()
	if !res.OK {
		t.Fatalf("WhereNotNull query: %v", res.Error())
	}
	rows, ok := Cast[[]duckNullRow](res)
	if !ok {
		t.Fatalf("expected []duckNullRow, got %T", res.Value)
	}
	if len(rows) != 1 || rows[0].ID != "1" {
		t.Fatalf(`WhereNotNull("Name") should return only the non-null row (id=1), got %+v`, rows)
	}
}

// TestDuckDB_DeleteWhereNull_Good — the WhereNull fix lives in the shared
// buildDuckDBWhere/buildDuckDBPredicate pair, which writeUpdate/
// writeDelete also call. Proves the blast radius: DeleteAll (and Update)
// scoped by a null predicate were equally broken — this pins that a
// delete now removes exactly the null-matching row, not zero rows
// (clause silently dropped → WHERE vanishes → nothing matches TRUE AND,
// or in DELETE's case the predicate is simply absent from the statement,
// which for DeleteAll with ONLY a null predicate means "DELETE FROM t"
// with no WHERE at all — every row gone).
func TestDuckDB_DeleteWhereNull_Good(t *testing.T) {
	d := openTestDuckDB(t, "deletewherenull")
	schema := duckNullRow{}.Schema()
	d.RegisterTable(schema.Name, schema)

	seed := []map[string]any{
		{"ID": "1", "Name": "alice"},
		{"ID": "2", "Name": nil},
	}
	for _, row := range seed {
		w := d.Write(core.Background(), WriteIntent{Op: OpInsert, Schema: schema, Rows: []any{row}})
		if !w.OK {
			t.Fatalf("seed write %v: %v", row, w.Error())
		}
	}

	del := d.Write(core.Background(), WriteIntent{
		Op:     OpDelete,
		Schema: schema,
		Where:  []Predicate{{Field: "Name", Op: "null"}},
	})
	if !del.OK {
		t.Fatalf("delete: %v", del.Error())
	}

	res := d.Read(core.Background(), ReadIntent{Schema: schema, Order: []OrderBy{{Field: "ID"}}})
	if !res.OK {
		t.Fatalf("post-delete read: %v", res.Error())
	}
	rows := res.Value.(*Payload).Data.([]map[string]any)
	if len(rows) != 1 || rows[0]["ID"] != "1" {
		t.Fatalf("DeleteAll(WhereNull) should remove only id=2, got %v", rows)
	}
}

// TestDuckDB_WhereGroup_Good — a WhereGroup-shaped nested OR block
// (Predicate.Group) was completely unhandled by buildDuckDBPredicate: a
// group predicate's own Op is always "" (WhereGroup never sets one, see
// bridge.go), which matched no case in the switch and silently vanished
// — the same drop pattern as WhereNull, but for a whole parenthesised
// clause rather than one comparison. Reproduces the RFC §4.2 worked
// example verbatim: active=true AND (tier='pro' OR admin=true).
func TestDuckDB_WhereGroup_Good(t *testing.T) {
	d := openTestDuckDB(t, "wheregroup")
	schema := Define(func(b *Builder) {
		b.Name("group_rows")
		b.PK("id")
		b.String("id").NotNull()
		b.Bool("active")
		b.String("tier")
		b.Bool("admin")
	})
	d.RegisterTable("group_rows", schema)

	seed := []map[string]any{
		{"id": "1", "active": true, "tier": "pro", "admin": false},  // active AND (pro OR admin) -> match via tier
		{"id": "2", "active": true, "tier": "free", "admin": true},  // active AND (pro OR admin) -> match via admin
		{"id": "3", "active": true, "tier": "free", "admin": false}, // active but neither pro nor admin -> no match
		{"id": "4", "active": false, "tier": "pro", "admin": true},  // not active -> no match regardless of group
	}
	for _, row := range seed {
		w := d.Write(core.Background(), WriteIntent{Op: OpInsert, Schema: schema, Rows: []any{row}})
		if !w.OK {
			t.Fatalf("seed write %v: %v", row, w.Error())
		}
	}

	res := d.Read(core.Background(), ReadIntent{
		Schema: schema,
		Where: []Predicate{
			{Field: "active", Op: "=", Value: true},
			{Group: []Predicate{
				{Field: "tier", Op: "=", Value: "pro"},
				{Field: "admin", Op: "=", Value: true, OR: true},
			}},
		},
		Order: []OrderBy{{Field: "id"}},
	})
	if !res.OK {
		t.Fatalf("grouped read: %v", res.Error())
	}
	rows := res.Value.(*Payload).Data.([]map[string]any)
	if len(rows) != 2 || rows[0]["id"] != "1" || rows[1]["id"] != "2" {
		t.Fatalf(`active=true AND (tier='pro' OR admin=true): got %v, want rows "1" and "2"`, rows)
	}
}

// --- Neighbour audit: every other predicate/clause buildDuckDBPredicate
// emits, pinned individually per the #45 audit requirement — "verified
// correct" and "fixed" both get a test, not just the two named defects.

// duckPredicateFixture opens a fresh DuckDB medium, registers a small
// fixture table, and seeds three rows spanning the value ranges the
// audit tests below exercise (equality / comparison / in / like /
// between). Shared so each audit test only has to build and run its own
// Predicate.
func duckPredicateFixture(t *testing.T) (*DuckDBMedium, Schema) {
	t.Helper()
	d := openTestDuckDB(t, "predicates")
	schema := Define(func(b *Builder) {
		b.Name("pred_rows")
		b.PK("id")
		b.String("id").NotNull()
		b.String("name")
		b.Int64("score")
	})
	d.RegisterTable("pred_rows", schema)
	rows := []map[string]any{
		{"id": "a", "name": "alice", "score": int64(10)},
		{"id": "b", "name": "bob", "score": int64(20)},
		{"id": "c", "name": "carol", "score": int64(30)},
	}
	for _, row := range rows {
		w := d.Write(core.Background(), WriteIntent{Op: OpInsert, Schema: schema, Rows: []any{row}})
		if !w.OK {
			t.Fatalf("seed write %v: %v", row, w.Error())
		}
	}
	return d, schema
}

// duckReadIDs runs a Read with the given predicates and returns the
// matching "id" values in ascending order.
func duckReadIDs(t *testing.T, d *DuckDBMedium, schema Schema, where []Predicate) []string {
	t.Helper()
	res := d.Read(core.Background(), ReadIntent{Schema: schema, Where: where, Order: []OrderBy{{Field: "id"}}})
	if !res.OK {
		t.Fatalf("read: %v", res.Error())
	}
	rows := res.Value.(*Payload).Data.([]map[string]any)
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r["id"].(string)
	}
	return ids
}

// TestDuckDB_WhereEquality_Good — "=" / "!=": verified correct, unchanged
// by the #45 fix. Pinned as part of the neighbour audit.
func TestDuckDB_WhereEquality_Good(t *testing.T) {
	d, schema := duckPredicateFixture(t)
	eq := duckReadIDs(t, d, schema, []Predicate{{Field: "name", Op: "=", Value: "bob"}})
	if len(eq) != 1 || eq[0] != "b" {
		t.Fatalf("= : got %v, want [b]", eq)
	}
	neq := duckReadIDs(t, d, schema, []Predicate{{Field: "name", Op: "!=", Value: "bob"}})
	if len(neq) != 2 || neq[0] != "a" || neq[1] != "c" {
		t.Fatalf("!= : got %v, want [a c]", neq)
	}
}

// TestDuckDB_WhereComparison_Good — "<" "<=" ">" ">=": verified correct.
func TestDuckDB_WhereComparison_Good(t *testing.T) {
	d, schema := duckPredicateFixture(t)
	gt := duckReadIDs(t, d, schema, []Predicate{{Field: "score", Op: ">", Value: int64(10)}})
	if len(gt) != 2 || gt[0] != "b" || gt[1] != "c" {
		t.Fatalf("> : got %v, want [b c]", gt)
	}
	lte := duckReadIDs(t, d, schema, []Predicate{{Field: "score", Op: "<=", Value: int64(20)}})
	if len(lte) != 2 || lte[0] != "a" || lte[1] != "b" {
		t.Fatalf("<= : got %v, want [a b]", lte)
	}
}

// TestDuckDB_WhereIn_Good — "in" with a non-empty set: verified correct.
func TestDuckDB_WhereIn_Good(t *testing.T) {
	d, schema := duckPredicateFixture(t)
	in := duckReadIDs(t, d, schema, []Predicate{{Field: "name", Op: "in", Value: []any{"alice", "carol"}}})
	if len(in) != 2 || in[0] != "a" || in[1] != "c" {
		t.Fatalf("in : got %v, want [a c]", in)
	}
}

// TestDuckDB_WhereNotIn_Good — "not in" had no case at all in the
// original switch (only "in" was handled) — silently dropped, same
// failure class as WhereNull. Fixed.
func TestDuckDB_WhereNotIn_Good(t *testing.T) {
	d, schema := duckPredicateFixture(t)
	notIn := duckReadIDs(t, d, schema, []Predicate{{Field: "name", Op: "not in", Value: []any{"alice", "carol"}}})
	if len(notIn) != 1 || notIn[0] != "b" {
		t.Fatalf("not in : got %v, want [b]", notIn)
	}
}

// TestDuckDB_WhereInEmptySet_Bad — an empty IN set must match zero rows
// (relational algebra: "x IN ()" is always false), not the whole table
// and not a query-breaking SQL error. Before the fix this built literally
// invalid SQL — `"name" IN ()` — which DuckDB rejects outright rather
// than filtering anything. Fixed: degrades to the constant "1=0".
func TestDuckDB_WhereInEmptySet_Bad(t *testing.T) {
	d, schema := duckPredicateFixture(t)
	ids := duckReadIDs(t, d, schema, []Predicate{{Field: "name", Op: "in", Value: []any{}}})
	if len(ids) != 0 {
		t.Fatalf("empty IN set should match zero rows, got %v", ids)
	}
}

// TestDuckDB_WhereNotInEmptySet_Good — the mirror: an empty NOT IN set
// excludes nothing, so it must match every row. Fixed: degrades to "1=1".
func TestDuckDB_WhereNotInEmptySet_Good(t *testing.T) {
	d, schema := duckPredicateFixture(t)
	ids := duckReadIDs(t, d, schema, []Predicate{{Field: "name", Op: "not in", Value: []any{}}})
	if len(ids) != 3 {
		t.Fatalf("empty NOT IN set should match every row, got %v", ids)
	}
}

// TestDuckDB_WhereLike_Good — "like": verified correct.
func TestDuckDB_WhereLike_Good(t *testing.T) {
	d, schema := duckPredicateFixture(t)
	like := duckReadIDs(t, d, schema, []Predicate{{Field: "name", Op: "like", Value: "a%"}})
	if len(like) != 1 || like[0] != "a" {
		t.Fatalf("like : got %v, want [a]", like)
	}
}

// TestDuckDB_WhereNotLike_Good — "not like" had no case in the original
// switch — silently dropped. Fixed.
func TestDuckDB_WhereNotLike_Good(t *testing.T) {
	d, schema := duckPredicateFixture(t)
	notLike := duckReadIDs(t, d, schema, []Predicate{{Field: "name", Op: "not like", Value: "a%"}})
	if len(notLike) != 2 || notLike[0] != "b" || notLike[1] != "c" {
		t.Fatalf("not like : got %v, want [b c]", notLike)
	}
}

// TestDuckDB_WhereBetween_Good — "between": verified correct.
func TestDuckDB_WhereBetween_Good(t *testing.T) {
	d, schema := duckPredicateFixture(t)
	between := duckReadIDs(t, d, schema, []Predicate{{Field: "score", Op: "between", Value: []any{int64(15), int64(25)}}})
	if len(between) != 1 || between[0] != "b" {
		t.Fatalf("between : got %v, want [b]", between)
	}
}
