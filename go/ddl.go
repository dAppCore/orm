// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

// Change describes an additive schema change (never destructive).
type Change struct {
	Op   string // "add_table" | "add_column" | "add_index"
	Info string // change description
}

// DDL emits dialect-specific CREATE TABLE DDL for a schema.
//
//	res := orm.DDL(c, User{}.Schema(), "duckdb")
func DDL(c *core.Core, schema Schema, dialect string) core.Result {
	var b stringsBuilder
	b.WriteString("CREATE TABLE ")
	b.WriteString(schema.Name)
	b.WriteString(" (\n")

	for i, f := range schema.Fields {
		b.WriteString("  ")
		b.WriteString(f.Name)
		b.WriteString(" ")
		b.WriteString(dialectType(f.Type, dialect))

		if hasConstraint(f, "notnull") {
			b.WriteString(" NOT NULL")
		}
		if hasConstraint(f, "unique") {
			b.WriteString(" UNIQUE")
		}
		if f.Default != nil {
			b.WriteString(" DEFAULT ")
			b.WriteString(anyToString(f.Default))
		}

		if i < len(schema.Fields)-1 || len(schema.PK) > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}

	if len(schema.PK) > 0 {
		b.WriteString("  PRIMARY KEY (")
		for i, pk := range schema.PK {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(pk)
		}
		b.WriteString(")\n")
	}

	b.WriteString(")")

	_ = c
	return core.Ok(b.String())
}

// Diff compares a declared schema against the current Medium state.
// Returns an additive list of Changes — never destructive.
//
//	diff := orm.Diff(c, schema)
func Diff(c *core.Core, schema Schema) core.Result {
	medium, mr := resolve(c, "default")
	if !mr.OK {
		return mr
	}
	caps := medium.Caps()
	if !caps.Introspect {
		return core.Ok([]Change{})
	}
	var changes []Change

	// For Memium, read existing state and diff
	if tables, ok := medium.(interface{ hasTable(string) bool }); ok {
		if !tables.hasTable(schema.Name) {
			changes = append(changes, Change{Op: "add_table", Info: schema.Name})
		}
		return core.Ok(changes)
	}

	r := medium.Read(c.Context(), ReadIntent{Schema: schema})

	if r.OK {
		payload, ok := r.Value.(*Payload)
		if !ok {
			return core.Ok(changes)
		}
		rows, rowsOK := payload.Data.([]map[string]any)
		if rowsOK && len(rows) == 0 {
			// Table doesn't exist yet — add it
			changes = append(changes, Change{Op: "add_table", Info: schema.Name})
		}
	} else {
		changes = append(changes, Change{Op: "add_table", Info: schema.Name})
	}

	return core.Ok(changes)
}

// ApplyChanges applies additive schema changes within a transaction (where supported).
//
//	orm.ApplyChanges(c, changes)
func ApplyChanges(c *core.Core, changes []Change) core.Result {
	if len(changes) == 0 {
		return core.Ok(nil)
	}
	medium, mr := resolve(c, "default")
	if !mr.OK {
		return mr
	}
	tables, ok := medium.(interface {
		RegisterTable(string, Schema)
	})
	if ok {
		for _, change := range changes {
			if change.Op != "add_table" {
				continue
			}
			schema, sr := schemaByName(c, change.Info)
			if !sr.OK {
				return sr
			}
			tables.RegisterTable(schema.Name, schema)
		}
	}
	return core.Ok(changes)
}

func dialectType(typ, dialect string) string {
	switch typ {
	case "string":
		if dialect == "postgres" || dialect == "mariadb" {
			return "VARCHAR(255)"
		}
		return "TEXT"
	case "int":
		return "INTEGER"
	case "int64":
		if dialect == "postgres" {
			return "BIGINT"
		}
		return "INTEGER"
	case "bool":
		if dialect == "sqlite" {
			return "INTEGER"
		}
		return "BOOLEAN"
	case "float", "float64":
		if dialect == "postgres" {
			return "DOUBLE PRECISION"
		}
		return "REAL"
	case "time":
		if dialect == "postgres" {
			return "TIMESTAMP"
		}
		return "DATETIME"
	case "json":
		if dialect == "postgres" {
			return "JSONB"
		}
		return "TEXT"
	case "bytes":
		return "BLOB"
	default:
		return "TEXT"
	}
}

func anyToString(v any) string {
	switch val := v.(type) {
	case string:
		return "'" + core.Replace(val, "'", "''") + "'"
	case bool:
		if val {
			return "1"
		}
		return "0"
	case int64:
		return core.FormatInt(val, 10)
	case int:
		return core.Itoa(val)
	case float64:
		return core.Sprintf("%g", val)
	default:
		return "NULL"
	}
}

type stringsBuilder struct {
	data []byte
}

func (b *stringsBuilder) WriteString(s string) {
	b.data = append(b.data, s...)
}

func (b *stringsBuilder) String() string {
	return string(b.data)
}
