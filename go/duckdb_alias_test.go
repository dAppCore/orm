// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"testing"

	core "dappco.re/go"
)

// duckAliasUser / duckAliasPost are the fixture models for the
// From(A{})/Sub()/Col() regression tests below — two related tables,
// lowercase snake_case Schema field names (matching the RFC §4.13 worked
// example verbatim: "u.id", "p.user_id", "p.published", "u.active") on
// top of Go-exported struct fields, the same casing split already used
// by AliasUser/AliasPost in alias_ext_test.go.
type duckAliasUser struct {
	ID     int64
	Name   string
	Active bool
}

func (duckAliasUser) Schema() Schema {
	return Define(func(b *Builder) {
		b.Name("duck_alias_users")
		b.PK("id")
		b.Int64("id")
		b.String("name")
		b.Bool("active")
	})
}

type duckAliasPost struct {
	ID        int64
	UserID    int64
	Title     string
	Published bool
}

func (duckAliasPost) Schema() Schema {
	return Define(func(b *Builder) {
		b.Name("duck_alias_posts")
		b.PK("id")
		b.Int64("id")
		b.Int64("user_id")
		b.String("title")
		b.Bool("published")
	})
}

// TestDuckDB_FromAlias_CrossJoin_Good — the minimal, unambiguous proof
// that From(A{}) actually reaches the emitted SQL. Regression for Mantis
// #45: Read() built "SELECT ... FROM <schema.Name>" unconditionally,
// never once looking at ReadIntent.Alias — an aliased query built SQL
// with the alias silently absent, indistinguishable from never having
// called From() at all. With NO WHERE clause at all, a correctly wired
// two-table From(A{}) is a cross join: 2 users x 3 posts = 6 rows. The
// dropped-alias bug instead falls back to "FROM duck_alias_users" alone
// = 2 rows.
func TestDuckDB_FromAlias_CrossJoin_Good(t *testing.T) {
	c := core.New()
	t.Cleanup(func() { Remove(c) })
	d := openTestDuckDB(t, "alias-crossjoin")
	Mount(c, "default", d)

	us := duckAliasUser{}.Schema()
	ps := duckAliasPost{}.Schema()
	d.RegisterTable(us.Name, us)
	d.RegisterTable(ps.Name, ps)
	RegisterSchema(c, us)
	RegisterSchema(c, ps)

	seedUsers := []map[string]any{
		{"id": int64(1), "name": "alice", "active": true},
		{"id": int64(2), "name": "bob", "active": true},
	}
	for _, row := range seedUsers {
		w := d.Write(core.Background(), WriteIntent{Op: OpInsert, Schema: us, Rows: []any{row}})
		if !w.OK {
			t.Fatalf("seed user %v: %v", row, w.Error())
		}
	}
	seedPosts := []map[string]any{
		{"id": int64(1), "user_id": int64(1), "title": "p1", "published": true},
		{"id": int64(2), "user_id": int64(1), "title": "p2", "published": false},
		{"id": int64(3), "user_id": int64(2), "title": "p3", "published": true},
	}
	for _, row := range seedPosts {
		w := d.Write(core.Background(), WriteIntent{Op: OpInsert, Schema: ps, Rows: []any{row}})
		if !w.OK {
			t.Fatalf("seed post %v: %v", row, w.Error())
		}
	}

	res := Of[duckAliasUser](c).From(A{"u": "duck_alias_users", "p": "duck_alias_posts"}).Get()
	if !res.OK {
		t.Fatalf("aliased cross join: %v", res.Error())
	}
	rows, ok := Cast[[]duckAliasUser](res)
	if !ok {
		t.Fatalf("expected []duckAliasUser, got %T", res.Value)
	}
	if len(rows) != 6 {
		t.Fatalf("cross join of 2 users x 3 posts should return 6 rows, got %d — From(A{}) is being dropped", len(rows))
	}
}

// TestDuckDB_FromAlias_JoinCondition_Good — the RFC §4.13 worked example,
// verbatim shape: From(A{"u":"users","p":"posts"}).Where("u.id","=",
// Col("p.user_id")).Where("p.published","=",true).Where("u.active","=",
// true).Get(). Exercises the full path: FROM-clause aliasing, Col()
// column-vs-literal predicates, alias-qualified WHERE field resolution
// against both the bridge's own schema (u.*) and a different registered
// schema (p.*), and SELECT-list qualification to the alias matching the
// query's own model (so a Of[duckAliasUser] query returns duckAliasUser
// rows even though posts also participates).
func TestDuckDB_FromAlias_JoinCondition_Good(t *testing.T) {
	c := core.New()
	t.Cleanup(func() { Remove(c) })
	d := openTestDuckDB(t, "alias-joincond")
	Mount(c, "default", d)

	us := duckAliasUser{}.Schema()
	ps := duckAliasPost{}.Schema()
	d.RegisterTable(us.Name, us)
	d.RegisterTable(ps.Name, ps)
	RegisterSchema(c, us)
	RegisterSchema(c, ps)

	seedUsers := []map[string]any{
		{"id": int64(1), "name": "alice", "active": true},
		{"id": int64(2), "name": "bob", "active": false}, // inactive — excluded by u.active
	}
	for _, row := range seedUsers {
		w := d.Write(core.Background(), WriteIntent{Op: OpInsert, Schema: us, Rows: []any{row}})
		if !w.OK {
			t.Fatalf("seed user %v: %v", row, w.Error())
		}
	}
	seedPosts := []map[string]any{
		{"id": int64(1), "user_id": int64(1), "title": "published", "published": true},
		{"id": int64(2), "user_id": int64(1), "title": "draft", "published": false}, // excluded by p.published
		{"id": int64(3), "user_id": int64(2), "title": "other-user", "published": true},
	}
	for _, row := range seedPosts {
		w := d.Write(core.Background(), WriteIntent{Op: OpInsert, Schema: ps, Rows: []any{row}})
		if !w.OK {
			t.Fatalf("seed post %v: %v", row, w.Error())
		}
	}

	res := Of[duckAliasUser](c).
		From(A{"u": "duck_alias_users", "p": "duck_alias_posts"}).
		Where("u.id", "=", Col("p.user_id")).
		Where("p.published", "=", true).
		Where("u.active", "=", true).
		Get()
	if !res.OK {
		t.Fatalf("aliased join query: %v", res.Error())
	}
	rows, ok := Cast[[]duckAliasUser](res)
	if !ok {
		t.Fatalf("expected []duckAliasUser, got %T", res.Value)
	}
	// Only alice: active AND owns a published post whose user_id matches
	// her id. Bob is inactive; alice's second post is a draft (would not
	// add a duplicate match even if active/published were reversed).
	if len(rows) != 1 || rows[0].ID != 1 || rows[0].Name != "alice" {
		t.Fatalf("expected exactly [alice], got %+v", rows)
	}
}

// TestDuckDB_FromAlias_SelfAlias_Good — single-table self-alias plus a
// plain (unqualified) Where — proves the SELECT-list/PK qualification
// path (qualifyCsv/qualifyField against the resolved base alias) works
// even without a join partner, and that resolveField's "not dotted"
// branch still validates against the bridge's own schema once From(A{})
// is active.
func TestDuckDB_FromAlias_SelfAlias_Good(t *testing.T) {
	c := core.New()
	t.Cleanup(func() { Remove(c) })
	d := openTestDuckDB(t, "alias-self")
	Mount(c, "default", d)

	us := duckAliasUser{}.Schema()
	d.RegisterTable(us.Name, us)

	seed := []map[string]any{
		{"id": int64(1), "name": "alice", "active": true},
		{"id": int64(2), "name": "bob", "active": false},
	}
	for _, row := range seed {
		w := d.Write(core.Background(), WriteIntent{Op: OpInsert, Schema: us, Rows: []any{row}})
		if !w.OK {
			t.Fatalf("seed %v: %v", row, w.Error())
		}
	}

	res := Of[duckAliasUser](c).From(A{"u": "duck_alias_users"}).Where("active", "=", true).Get()
	if !res.OK {
		t.Fatalf("self-aliased query: %v", res.Error())
	}
	rows, ok := Cast[[]duckAliasUser](res)
	if !ok {
		t.Fatalf("expected []duckAliasUser, got %T", res.Value)
	}
	if len(rows) != 1 || rows[0].Name != "alice" {
		t.Fatalf("expected [alice], got %+v", rows)
	}
}

// TestDuckDB_FromAlias_UnknownAlias_Bad — a Where() qualified with an
// alias never declared in From(A{}) must fail loudly with
// orm.input.field, not silently resolve to nothing or panic.
func TestDuckDB_FromAlias_UnknownAlias_Bad(t *testing.T) {
	c := core.New()
	t.Cleanup(func() { Remove(c) })
	d := openTestDuckDB(t, "alias-unknown")
	Mount(c, "default", d)

	us := duckAliasUser{}.Schema()
	d.RegisterTable(us.Name, us)

	res := Of[duckAliasUser](c).From(A{"u": "duck_alias_users"}).Where("z.id", "=", int64(1)).Get()
	if res.OK {
		t.Fatalf("expected failure for an alias not declared in From(A{}), got OK")
	}
	if res.Code() != "orm.input.field" {
		t.Fatalf("expected orm.input.field, got %q", res.Code())
	}
}

// TestDuckDB_Subquery_Good — a Sub() derived-table participant. Regression
// for Mantis #45's subquery half: the DuckDB SQL builder never emitted
// Sub() participants at all (same root cause as the alias drop — Read()
// never looked at ReadIntent.Alias), and separately GetIntent() never
// resolved Schema on a builder that was handed to Sub() instead of
// dispatched directly (fixed in alias.go) — without that fix every Sub()
// participant carried an empty Schema.Name and could never build.
func TestDuckDB_Subquery_Good(t *testing.T) {
	c := core.New()
	t.Cleanup(func() { Remove(c) })
	d := openTestDuckDB(t, "alias-subquery")
	Mount(c, "default", d)

	us := duckAliasUser{}.Schema()
	ps := duckAliasPost{}.Schema()
	d.RegisterTable(us.Name, us)
	d.RegisterTable(ps.Name, ps)
	RegisterSchema(c, us)
	RegisterSchema(c, ps)

	seedUsers := []map[string]any{
		{"id": int64(1), "name": "alice", "active": true},
		{"id": int64(2), "name": "bob", "active": false},
	}
	for _, row := range seedUsers {
		w := d.Write(core.Background(), WriteIntent{Op: OpInsert, Schema: us, Rows: []any{row}})
		if !w.OK {
			t.Fatalf("seed user %v: %v", row, w.Error())
		}
	}
	seedPosts := []map[string]any{
		{"id": int64(1), "user_id": int64(1), "title": "p1", "published": true},
		{"id": int64(2), "user_id": int64(2), "title": "p2", "published": true},
	}
	for _, row := range seedPosts {
		w := d.Write(core.Background(), WriteIntent{Op: OpInsert, Schema: ps, Rows: []any{row}})
		if !w.OK {
			t.Fatalf("seed post %v: %v", row, w.Error())
		}
	}

	activeUsers := Of[duckAliasUser](c).Where("active", "=", true)

	// "au" is a derived table (active users only), joined against the
	// real posts table by id — proves Sub() renders as a nested SELECT
	// inside the FROM clause instead of being silently dropped.
	res := Of[duckAliasUser](c).
		From(A{"au": Sub(activeUsers), "p": "duck_alias_posts"}).
		Where("au.id", "=", Col("p.user_id")).
		Get()
	if !res.OK {
		t.Fatalf("subquery join: %v", res.Error())
	}
	rows, ok := Cast[[]duckAliasUser](res)
	if !ok {
		t.Fatalf("expected []duckAliasUser, got %T", res.Value)
	}
	if len(rows) != 1 || rows[0].Name != "alice" {
		t.Fatalf("expected only alice (bob is excluded by the active-only subquery), got %+v", rows)
	}
}

// TestDuckDB_FromAlias_LeftJoin_Unsupported_Bad — LeftJoin/RightJoin/
// FullJoin aren't wired on the DuckDB lane (see Caps() doc comment: the
// alias vocabulary carries no explicit ON condition, so emitting one
// would mean guessing). This must fail loudly with orm.join.unsupported
// — via the bridge-level capability gate (checkAliasCapabilities in
// dispatchRead) — rather than silently execute as an inner join or an
// unconditional cross join that quietly returns wrong data.
func TestDuckDB_FromAlias_LeftJoin_Unsupported_Bad(t *testing.T) {
	c := core.New()
	t.Cleanup(func() { Remove(c) })
	d := openTestDuckDB(t, "alias-leftjoin")
	Mount(c, "default", d)

	us := duckAliasUser{}.Schema()
	ps := duckAliasPost{}.Schema()
	d.RegisterTable(us.Name, us)
	d.RegisterTable(ps.Name, ps)

	res := Of[duckAliasUser](c).
		From(A{"u": "duck_alias_users", "p": LeftJoin("duck_alias_posts")}).
		Get()
	if res.OK {
		t.Fatalf("expected LeftJoin to fail — outer joins aren't wired on the DuckDB lane yet, got OK")
	}
	if res.Code() != "orm.join.unsupported" {
		t.Fatalf("expected orm.join.unsupported, got %q", res.Code())
	}
}

// TestDuckDB_LeftJoin_DirectRead_Unsupported_Bad — defence in depth: a
// direct Medium.Read call that bypasses the bridge's capability gate
// entirely (the same low-level path duckdb_test.go's other tests use)
// hits the identical orm.join.unsupported failure inside
// buildDuckDBFrom itself, rather than silently emitting a LEFT JOIN with
// no ON condition.
func TestDuckDB_LeftJoin_DirectRead_Unsupported_Bad(t *testing.T) {
	d := openTestDuckDB(t, "alias-leftjoin-direct")
	us := duckAliasUser{}.Schema()
	ps := duckAliasPost{}.Schema()
	d.RegisterTable(us.Name, us)
	d.RegisterTable(ps.Name, ps)

	res := d.Read(core.Background(), ReadIntent{
		Schema: us,
		Alias:  map[string]any{"u": "duck_alias_users", "p": LeftJoin("duck_alias_posts")},
	})
	if res.OK {
		t.Fatalf("expected LeftJoin to fail at the Medium level too, got OK")
	}
	if res.Code() != "orm.join.unsupported" {
		t.Fatalf("expected orm.join.unsupported, got %q", res.Code())
	}
}

// TestDuckDB_SetOp_DegradesHonestly_Bad — Union/Intersect/Except
// (alias.go) return a bare BridgeRef with no Get()/Where(), so no typed
// Bridge call can ever populate ReadIntent.SetOp: there is no reachable
// path to wire SQL emission for it yet (see the Caps() doc comment).
// Before this fix, Caps().SetOps claimed true anyway — an overclaim of
// the exact same "declared capability, not actually honoured" shape as
// the Aliases/JoinKinds gap, just unreachable rather than silently wrong.
// This pins the honest capability declaration and proves the bridge's
// EXISTING (previously untriggered, because Caps lied) degradation
// machinery now actually fires for a directly-constructed SetOp intent.
func TestDuckDB_SetOp_DegradesHonestly_Bad(t *testing.T) {
	d := openTestDuckDB(t, "alias-setop")

	if d.Caps().SetOps {
		t.Fatalf("Caps().SetOps claims support, but no typed Bridge call can ever populate ReadIntent.SetOp")
	}

	degraded, capR := validateReadCapabilities(d.Caps(), ReadIntent{SetOp: &SetOp{Kind: SetUnion}})
	if !capR.OK {
		t.Fatalf("non-strict SetOp against an unsupporting medium should degrade, not fail: %v", capR.Error())
	}
	if degraded != "setops" {
		t.Fatalf(`expected degraded="setops", got %q`, degraded)
	}
}
