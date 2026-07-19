// SPDX-License-Identifier: EUPL-1.2

// DuckDB-backed Medium — the canonical persistent backing for orm
// tables. Lives in the orm package (not in consumers) so every
// orm-using product (core/ide, lthn/desktop, future bundles)
// shares one persistent baseline instead of reinventing it.
//
// Capabilities: equality / comparison / IN / LIKE / NULL / BETWEEN,
// joins, aggregates, transactions, cursor, introspect. No native
// watch (use poll-fallback). DDL is generated from the Schema on
// RegisterTable so a fresh database file becomes useable immediately.
//
// Pairs with the in-memory Memium that already mounts under "default"
// — consumers re-mount whichever they need under "default" to swap
// backends without touching call sites.
//
// Origin: promoted from core/ide/pkg/server/orm_duckdb_medium.go
// (Snider 2026-05-15) — moving the implementation into orm itself
// closes the gap that core/ide had its own private medium nobody
// else could share.

package orm

import (
	"database/sql"
	"sync"
	"time"

	core "dappco.re/go"

	// duckdb driver — pulled in transitively by go-store via the same
	// duckdb-go-bindings dep. Adding it here makes the Medium usable
	// without consumers needing to import the driver themselves.
	_ "github.com/duckdb/duckdb-go/v2"
)

// DuckDBMedium implements Medium against a DuckDB *sql.DB connection.
//
// Usage example:
//
//	d, r := orm.NewDuckDB("/path/to/orm.duckdb")
//	if !r.OK { return r }
//	defer d.Close()
//	d.RegisterTable("notes", noteSchema)
//	orm.Mount(c, "default", d)
type DuckDBMedium struct {
	mu      sync.Mutex
	conn    *sql.DB
	path    string
	schemas map[string]Schema
}

// NewDuckDB connects to the given DuckDB file (creates it if missing,
// opens read-write). Failure lets the caller fall back to Memium
// without aborting startup.
//
// Usage example:
//
//	r := orm.NewDuckDB("~/Lethean/data/orm.duckdb")
//	if r.OK { d := r.Value.(*orm.DuckDBMedium); _ = d }
func NewDuckDB(path string) core.Result {
	conn, err := sql.Open("duckdb", path)
	if err != nil {
		return core.Fail(core.Errorf("orm.NewDuckDB open %s: %w", path, err))
	}
	if err := conn.Ping(); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			core.Warn("orm.duck.ping_close", "error", closeErr)
		}
		return core.Fail(core.Errorf("orm.NewDuckDB ping %s: %w", path, err))
	}
	return core.Ok(&DuckDBMedium{
		conn:    conn,
		path:    path,
		schemas: make(map[string]Schema),
	})
}

// Path returns the on-disk file the Medium is bound to. Useful for
// telemetry / "where am I writing" surfaces.
func (m *DuckDBMedium) Path() string {
	if m == nil {
		return ""
	}
	return m.path
}

// Raw returns the underlying *sql.DB for cases where the Intent API
// doesn't fit — raw SQL, CREATE VIEW over external sources, DuckDB
// extensions like read_json_auto / read_parquet / FTS / EXPLAIN.
// Nil on a nil receiver so callers can guard cleanly.
//
// Prefer Read / Write / Stream where the Intent shape covers the
// query — Raw skips Medium's portability guarantees (the same call
// won't run against Memium or future Mediums) and the Caps gate.
// Use it when the workload is intrinsically DuckDB-shaped (live
// views over JSONL, parquet scans, columnar analytics).
//
// Usage example:
//
//	d, r := orm.NewDuckDB("/path/to/orm.duckdb")
//	if !r.OK { return r }
//	db := d.Raw()
//	if _, err := db.ExecContext(ctx,
//	    `CREATE OR REPLACE VIEW r1 AS
//	     SELECT * FROM read_json_auto(?, format='newline_delimited',
//	         ignore_errors=true, union_by_name=true)`, glob); err != nil {
//	    return core.Fail(err)
//	}
func (m *DuckDBMedium) Raw() *sql.DB {
	if m == nil {
		return nil
	}
	return m.conn
}

// Close releases the DuckDB connection. Safe on a nil receiver.
func (m *DuckDBMedium) Close() core.Result {
	if m == nil || m.conn == nil {
		return core.Ok(nil)
	}
	if err := m.conn.Close(); err != nil {
		return core.Fail(err)
	}
	return core.Ok(nil)
}

// RegisterTable seeds DuckDB with a CREATE TABLE IF NOT EXISTS for
// the given schema. Idempotent — safe to call repeatedly. Schemas
// registered here ALSO get tracked in the Medium's own map so
// future introspection / migration code can see what's mounted.
func (m *DuckDBMedium) RegisterTable(name string, schema Schema) {
	if schema.Name == "" {
		schema.Name = name
	}
	if result := m.registerTable(schema); !result.OK {
		core.Warn("orm.duck.register_table", "table", schema.Name, "error", result.Value)
	}
}

func (m *DuckDBMedium) registerTable(schema Schema) core.Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.schemas[schema.Name] = schema

	cols := make([]string, 0, len(schema.Fields))
	for _, f := range schema.Fields {
		cols = append(cols, core.Sprintf(`"%s" %s`, f.Name, duckDBType(f.Type)))
	}
	if len(schema.PK) > 0 {
		cols = append(cols, core.Sprintf(`PRIMARY KEY (%s)`, quoteCsv(schema.PK)))
	}
	stmt := core.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (%s)`, schema.Name, core.Join(", ", cols...))
	if _, err := m.conn.Exec(stmt); err != nil {
		return core.Fail(core.Errorf("orm.duck ensure table %s: %w", schema.Name, err))
	}
	return core.Ok(nil)
}

// --- Medium impl ---

// Caps reports the predicate / join / aggregate set DuckDB honours
// server-side. Watch is unsupported; the bridge can degrade via
// WatchPoll when consumers need real-time semantics.
func (m *DuckDBMedium) Caps() Capabilities {
	return Capabilities{
		Predicates: PredicateCaps{
			Equality:   true,
			Comparison: true,
			In:         true,
			Like:       true,
			Null:       true,
			Between:    true,
		},
		Joins:        true,
		Transactions: true,
		Aggregates:   true,
		Cursor:       true,
		Introspect:   true,
		Watch:        false,
		WatchPoll:    true,
		Aliases:      true,
		Subqueries:   true,
		SetOps:       true,
		JoinKinds:    JoinCaps{Inner: true, Left: true, Right: true, Full: true},
	}
}

// Read dispatches a ReadIntent to a SELECT query (or aggregate
// short-circuit). PK lookups bypass the predicate builder for
// the common find-by-id path.
func (m *DuckDBMedium) Read(ctx core.Context, in ReadIntent) core.Result {
	start := time.Now()

	if in.Aggregate != nil {
		return m.readAggregate(ctx, in, start)
	}

	cols := duckSchemaFields(in.Schema)
	if len(cols) == 0 {
		return core.Fail(core.Errorf("orm.duck.Read: schema %q has no fields", in.Schema.Name))
	}

	sb := core.NewBuilder()
	args := []any{}
	sb.WriteString("SELECT ")
	sb.WriteString(quoteCsv(cols))
	sb.WriteString(` FROM "`)
	sb.WriteString(in.Schema.Name)
	sb.WriteString(`"`)

	if len(in.PK) > 0 && len(in.PK) == len(in.Schema.PK) {
		sb.WriteString(" WHERE ")
		for i, pk := range in.Schema.PK {
			if i > 0 {
				sb.WriteString(" AND ")
			}
			sb.WriteString(`"`)
			sb.WriteString(pk)
			sb.WriteString(`" = ?`)
			args = append(args, in.PK[i])
		}
	} else if len(in.Where) > 0 {
		clause, wargs := buildDuckDBWhere(in.Where)
		if clause != "" {
			sb.WriteString(" WHERE ")
			sb.WriteString(clause)
			args = append(args, wargs...)
		}
	}

	if len(in.Order) > 0 {
		sb.WriteString(" ORDER BY ")
		parts := make([]string, 0, len(in.Order))
		for _, o := range in.Order {
			dir := "ASC"
			if o.Desc {
				dir = "DESC"
			}
			parts = append(parts, core.Sprintf(`"%s" %s`, o.Field, dir))
		}
		sb.WriteString(core.Join(", ", parts...))
	}
	if in.Limit > 0 {
		sb.WriteString(core.Sprintf(" LIMIT %d", in.Limit))
	}
	if in.Offset > 0 {
		sb.WriteString(core.Sprintf(" OFFSET %d", in.Offset))
	}

	rows, err := m.conn.Query(sb.String(), args...)
	if err != nil {
		return core.Fail(core.Errorf("orm.duck.Read query: %w (sql=%s)", err, sb.String()))
	}
	defer func() {
		if err := rows.Close(); err != nil {
			core.Warn("orm.duck.rows_close", "error", err)
		}
	}()

	scanned := scanDuckDBRows(rows, cols)
	if !scanned.OK {
		return scanned
	}
	out := scanned.Value.([]map[string]any)
	return core.Ok(&Payload{
		Data: out,
		Meta: Meta{
			Medium:   "duckdb",
			Duration: core.Duration(time.Since(start)),
			RowsRead: int64(len(out)),
		},
	})
}

func (m *DuckDBMedium) readAggregate(_ core.Context, in ReadIntent, start time.Time) core.Result {
	op := core.Upper(in.Aggregate.Op)
	expr := "*"
	if in.Aggregate.Field != "" {
		expr = core.Sprintf(`"%s"`, in.Aggregate.Field)
	}
	sb := core.NewBuilder()
	sb.WriteString(core.Sprintf(`SELECT %s(%s) AS agg FROM "%s"`, op, expr, in.Schema.Name))
	args := []any{}
	if len(in.Where) > 0 {
		clause, wargs := buildDuckDBWhere(in.Where)
		if clause != "" {
			sb.WriteString(" WHERE ")
			sb.WriteString(clause)
			args = append(args, wargs...)
		}
	}
	row := m.conn.QueryRow(sb.String(), args...)
	var v any
	if err := row.Scan(&v); err != nil {
		return core.Fail(core.Errorf("orm.duck.Aggregate scan: %w", err))
	}
	return core.Ok(&Payload{
		Data: v,
		Meta: Meta{
			Medium:   "duckdb",
			Duration: core.Duration(time.Since(start)),
		},
	})
}

// Write dispatches a WriteIntent (Insert / Save / Update / Delete).
// Save uses ON CONFLICT (pk) DO UPDATE so re-saving an existing row
// upserts in place — matches Memium's semantics.
func (m *DuckDBMedium) Write(_ core.Context, in WriteIntent) core.Result {
	start := time.Now()
	switch in.Op {
	case OpInsert, OpSave:
		return m.writeInsert(in, start)
	case OpUpdate:
		return m.writeUpdate(in, start)
	case OpDelete:
		return m.writeDelete(in, start)
	default:
		return core.Fail(core.Errorf("orm.duck.Write: unknown op %v", in.Op))
	}
}

func (m *DuckDBMedium) writeInsert(in WriteIntent, start time.Time) core.Result {
	if len(in.Rows) == 0 {
		return core.Ok(&Payload{Meta: Meta{Medium: "duckdb"}})
	}
	cols := duckSchemaFields(in.Schema)
	tx, err := m.conn.Begin()
	if err != nil {
		return core.Fail(core.Errorf("orm.duck.Write begin: %w", err))
	}
	rowsWrit := int64(0)
	for _, raw := range in.Rows {
		row, ok := rawToDuckDBMap(raw)
		if !ok {
			if err := tx.Rollback(); err != nil {
				core.Warn("orm.duck.rollback", "error", err)
			}
			return core.Fail(core.Errorf("orm.duck.Write: row not coercible to map[string]any"))
		}
		args := make([]any, 0, len(cols))
		for _, c := range cols {
			args = append(args, row[c])
		}
		insertCols := quoteCsv(cols)
		placeholders := questionPlaceholdersN(len(cols))
		stmt := core.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s)`, in.Schema.Name, insertCols, placeholders)
		if in.Op == OpSave && len(in.Schema.PK) > 0 {
			updateClauses := make([]string, 0, len(cols))
			for _, c := range cols {
				skip := false
				for _, pk := range in.Schema.PK {
					if pk == c {
						skip = true
						break
					}
				}
				if !skip {
					updateClauses = append(updateClauses, core.Sprintf(`"%s" = excluded."%s"`, c, c))
				}
			}
			if len(updateClauses) > 0 {
				stmt += core.Sprintf(` ON CONFLICT (%s) DO UPDATE SET %s`, quoteCsv(in.Schema.PK), core.Join(", ", updateClauses...))
			}
		}
		if _, err := tx.Exec(stmt, args...); err != nil {
			if rollErr := tx.Rollback(); rollErr != nil {
				core.Warn("orm.duck.rollback", "error", rollErr)
			}
			return core.Fail(core.Errorf("orm.duck.Write insert: %w (sql=%s)", err, stmt))
		}
		rowsWrit++
	}
	if err := tx.Commit(); err != nil {
		return core.Fail(core.Errorf("orm.duck.Write commit: %w", err))
	}
	return core.Ok(&Payload{
		Meta: Meta{
			Medium:   "duckdb",
			Duration: core.Duration(time.Since(start)),
			RowsWrit: rowsWrit,
		},
	})
}

func (m *DuckDBMedium) writeUpdate(in WriteIntent, start time.Time) core.Result {
	if len(in.Updates) == 0 {
		return core.Ok(&Payload{Meta: Meta{Medium: "duckdb"}})
	}
	sets := make([]string, 0, len(in.Updates))
	args := make([]any, 0, len(in.Updates))
	for k, v := range in.Updates {
		sets = append(sets, core.Sprintf(`"%s" = ?`, k))
		args = append(args, v)
	}
	stmt := core.Sprintf(`UPDATE "%s" SET %s`, in.Schema.Name, core.Join(", ", sets...))
	if len(in.Where) > 0 {
		clause, wargs := buildDuckDBWhere(in.Where)
		if clause != "" {
			stmt += " WHERE " + clause
			args = append(args, wargs...)
		}
	}
	res, err := m.conn.Exec(stmt, args...)
	if err != nil {
		return core.Fail(core.Errorf("orm.duck.Write update: %w (sql=%s)", err, stmt))
	}
	rowsWrit, _ := res.RowsAffected()
	return core.Ok(&Payload{
		Meta: Meta{
			Medium:   "duckdb",
			Duration: core.Duration(time.Since(start)),
			RowsWrit: rowsWrit,
		},
	})
}

func (m *DuckDBMedium) writeDelete(in WriteIntent, start time.Time) core.Result {
	stmt := core.Sprintf(`DELETE FROM "%s"`, in.Schema.Name)
	args := []any{}
	if len(in.Where) > 0 {
		clause, wargs := buildDuckDBWhere(in.Where)
		if clause != "" {
			stmt += " WHERE " + clause
			args = append(args, wargs...)
		}
	}
	res, err := m.conn.Exec(stmt, args...)
	if err != nil {
		return core.Fail(core.Errorf("orm.duck.Write delete: %w (sql=%s)", err, stmt))
	}
	rowsWrit, _ := res.RowsAffected()
	return core.Ok(&Payload{
		Meta: Meta{
			Medium:   "duckdb",
			Duration: core.Duration(time.Since(start)),
			RowsWrit: rowsWrit,
		},
	})
}

// Stream falls through to Read for v1 — DuckDB cursor streaming via
// the driver's Rows interface is feasible but adds complexity.
func (m *DuckDBMedium) Stream(ctx core.Context, in ReadIntent) core.Result {
	return m.Read(ctx, in)
}

// Watch is unsupported on DuckDB without a poll-fallback wrapper.
// Caps declares WatchPoll=true so the bridge degrades gracefully.
func (m *DuckDBMedium) Watch(_ core.Context, _ ReadIntent) core.Result {
	return core.Fail(core.Errorf("orm.duck.Watch: not implemented (use poll fallback)"))
}

// Search is unsupported v1. DuckDB's FTS extension is a separate
// lane — when wired, route here.
func (m *DuckDBMedium) Search(_ core.Context, _ ReadIntent) core.Result {
	return core.Fail(core.Errorf("orm.duck.Search: not implemented"))
}

// --- helpers ---

// duckSchemaFields returns the field NAMES from a Schema (renamed
// from the core/ide-side `fieldNames` to avoid colliding with
// orm/tags.go's go-name/json-tag resolver of the same name).
func duckSchemaFields(s Schema) []string {
	out := make([]string, 0, len(s.Fields))
	for _, f := range s.Fields {
		out = append(out, f.Name)
	}
	return out
}

func quoteCsv(names []string) string {
	if len(names) == 0 {
		return ""
	}
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = core.Sprintf(`"%s"`, n)
	}
	return core.Join(", ", parts...)
}

func questionPlaceholdersN(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return core.Join(",", parts...)
}

// buildDuckDBWhere translates a list of Predicate into a SQL boolean
// expression + positional args, in emission order so the args line up
// with the "?" placeholders that appear in the returned clause.
func buildDuckDBWhere(preds []Predicate) (string, []any) {
	args := []any{}
	clause := buildDuckDBWhereInto(preds, &args)
	return clause, args
}

// buildDuckDBWhereInto renders preds into a joined boolean expression,
// appending args (in emission order) into the caller's shared slice.
// Recursed into by buildDuckDBPredicate for a Predicate.Group — a
// WhereGroup's parenthesised block is structurally identical to a
// top-level Where chain (each member combines via its own OR flag), so
// one function serves both without duplicating the joiner logic. Also
// fixes a latent off-by-construction in the original loop, which joined
// on "i == 0" (the predicate's position) rather than "no clause emitted
// yet" — a dropped leading predicate left a dangling " AND "/" OR " glued
// to the front of the next clause. Unreachable via the public Bridge API
// today (every op it can emit now renders something), but the same
// silent-corruption shape as the rest of this file's bugs, so fixed
// alongside them.
func buildDuckDBWhereInto(preds []Predicate, args *[]any) string {
	parts := make([]string, 0, len(preds))
	for _, p := range preds {
		clause := buildDuckDBPredicate(p, args)
		if clause == "" {
			continue
		}
		if len(parts) == 0 {
			parts = append(parts, clause)
			continue
		}
		joiner := " AND "
		if p.OR {
			joiner = " OR "
		}
		parts = append(parts, joiner+clause)
	}
	return core.Join("", parts...)
}

// buildDuckDBPredicate renders one Predicate. Operator strings match the
// canonical vocabulary the bridge actually emits (RFC §4.2 / bridge.go /
// shape.go): "null" / "not null" for WhereNull/WhereNotNull (NOT
// "is null" / "is not null" — that mismatch was the root cause of Mantis
// #45: WhereNull's clause never matched any case here, fell through to
// the empty default, and the query came back completely unfiltered),
// "in" / "not in", "like" / "not like", plus the comparison operators and
// "between". A Predicate.Group (from WhereGroup) was previously
// unhandled entirely — its own Op is always "" (WhereGroup never sets
// one), which matched no case and silently vanished too.
func buildDuckDBPredicate(p Predicate, args *[]any) string {
	if len(p.Group) > 0 {
		inner := buildDuckDBWhereInto(p.Group, args)
		if inner == "" {
			return ""
		}
		return core.Sprintf("(%s)", inner)
	}

	op := core.Lower(p.Op)
	col := core.Sprintf(`"%s"`, p.Field)
	switch op {
	case "=", "!=", "<>", "<", "<=", ">", ">=":
		*args = append(*args, p.Value)
		return core.Sprintf("%s %s ?", col, p.Op)
	case "in":
		return buildDuckDBInClause(col, p.Value, args, false)
	case "not in":
		return buildDuckDBInClause(col, p.Value, args, true)
	case "like":
		*args = append(*args, p.Value)
		return core.Sprintf("%s LIKE ?", col)
	case "not like":
		*args = append(*args, p.Value)
		return core.Sprintf("%s NOT LIKE ?", col)
	case "null":
		return core.Sprintf("%s IS NULL", col)
	case "not null":
		return core.Sprintf("%s IS NOT NULL", col)
	case "between":
		values, ok := p.Value.([]any)
		if !ok || len(values) != 2 {
			return "1=0"
		}
		*args = append(*args, values[0], values[1])
		return core.Sprintf("%s BETWEEN ? AND ?", col)
	}
	return ""
}

// buildDuckDBInClause renders IN / NOT IN. An empty value set can't be
// written as SQL "IN ()" — invalid syntax (DuckDB included) that fails
// the whole query at execution time rather than filtering anything — so
// it degrades to the relational-algebra-correct constant instead: "x IN
// ()" never matches (1=0), "x NOT IN ()" always matches (1=1). Mirrors
// Memium's inSlice/!inSlice semantics for the empty-set case exactly
// (memium.go's inSlice returns false against an empty target slice
// regardless of val, so "in" matches nothing and "not in" — the negation
// — matches everything).
func buildDuckDBInClause(col string, value any, args *[]any, negate bool) string {
	values, ok := value.([]any)
	if !ok {
		return ""
	}
	if len(values) == 0 {
		if negate {
			return "1=1"
		}
		return "1=0"
	}
	placeholders := questionPlaceholdersN(len(values))
	for _, v := range values {
		*args = append(*args, v)
	}
	verb := "IN"
	if negate {
		verb = "NOT IN"
	}
	return core.Sprintf("%s %s (%s)", col, verb, placeholders)
}

func duckDBType(t string) string {
	switch t {
	case "int", "int64":
		return "BIGINT"
	case "int32":
		return "INTEGER"
	case "string":
		return "TEXT"
	case "bool":
		return "BOOLEAN"
	case "float", "float64":
		return "DOUBLE"
	case "time":
		return "TIMESTAMP"
	case "json":
		return "JSON"
	case "bytes":
		return "BLOB"
	default:
		return "TEXT"
	}
}

func scanDuckDBRows(rows *sql.Rows, cols []string) core.Result {
	out := []map[string]any{}
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cols {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return core.Fail(core.Errorf("orm.duck.scan: %w", err))
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			v := raw[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			row[c] = v
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return core.Fail(err)
	}
	return core.Ok(out)
}

func rawToDuckDBMap(raw any) (map[string]any, bool) {
	if m, ok := raw.(map[string]any); ok {
		return m, true
	}
	data := core.JSONMarshalString(raw)
	out := map[string]any{}
	r := core.JSONUnmarshalString(data, &out)
	if !r.OK {
		return nil, false
	}
	return out, true
}
