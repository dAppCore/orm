// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

// MountSchemas scans c.Data() entries under prefix, parses each JSON file
// via SchemaFromJSON, and registers each by table name.
func MountSchemas(c *core.Core, prefix string) core.Result {
	list := c.Data().List(prefix)
	if !list.OK {
		return core.Fail(core.NewCode("orm.schema.mount", "data prefix not found: "+prefix))
	}
	entries, ok := list.Value.([]core.FsDirEntry)
	if !ok {
		return core.Fail(core.NewCode("orm.schema.mount", "data prefix did not list filesystem entries"))
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !core.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := core.JoinPath(prefix, entry.Name())
		raw := c.Data().ReadString(path)
		if !raw.OK {
			return raw
		}
		schemaR := SchemaFromJSON([]byte(raw.Value.(string)))
		if !schemaR.OK {
			return schemaR
		}
		if r := RegisterSchema(c, schemaR.Value.(Schema)); !r.OK {
			return r
		}
		count++
	}
	if count == 0 {
		return core.Fail(core.NewCode("orm.schema.mount", "no schemas found under prefix: "+prefix))
	}
	return core.Ok(count)
}

// DynamicBridge is the type-erased bridge keyed by table name.
type DynamicBridge struct {
	c          *core.Core
	schema     Schema
	intent     ReadIntent
	mediumName string
	err        error
}

// OfTable returns a DynamicBridge for a registered schema table.
func OfTable(c *core.Core, table string) *DynamicBridge {
	schema, sr := schemaByName(c, table)
	if !sr.OK {
		return &DynamicBridge{c: c, err: resultError(sr, "orm.schema.not_found", "schema not found: "+table)}
	}
	return &DynamicBridge{
		c:          c,
		schema:     schema,
		intent:     ReadIntent{Schema: schema},
		mediumName: "default",
	}
}

func (db *DynamicBridge) On(name string) *DynamicBridge {
	if db.err != nil {
		return db
	}
	db.mediumName = name
	return db
}

func (db *DynamicBridge) Where(field, op string, value any) *DynamicBridge {
	if db.err != nil {
		return db
	}
	f, ok := db.schema.FieldByName(field)
	if !ok {
		db.err = core.NewCode("orm.input.field", "unknown field: "+field)
		return db
	}
	coerced := Apply(f, value)
	if !coerced.OK {
		db.err = resultError(coerced, "orm.input.coerce", "coercion failed for field: "+field)
		return db
	}
	db.intent.Where = append(db.intent.Where, Predicate{Field: field, Op: op, Value: coerced.Value})
	return db
}

func (db *DynamicBridge) OrWhere(field, op string, value any) *DynamicBridge {
	db.Where(field, op, value)
	if db.err == nil && len(db.intent.Where) > 0 {
		db.intent.Where[len(db.intent.Where)-1].OR = true
	}
	return db
}

func (db *DynamicBridge) WhereNull(field string) *DynamicBridge {
	if db.err != nil {
		return db
	}
	if _, ok := db.schema.FieldByName(field); !ok {
		db.err = core.NewCode("orm.input.field", "unknown field: "+field)
		return db
	}
	db.intent.Where = append(db.intent.Where, Predicate{Field: field, Op: "null"})
	return db
}

func (db *DynamicBridge) WhereNotNull(field string) *DynamicBridge {
	if db.err != nil {
		return db
	}
	if _, ok := db.schema.FieldByName(field); !ok {
		db.err = core.NewCode("orm.input.field", "unknown field: "+field)
		return db
	}
	db.intent.Where = append(db.intent.Where, Predicate{Field: field, Op: "not null"})
	return db
}

func (db *DynamicBridge) With(relations ...string) *DynamicBridge {
	db.intent.With = append(db.intent.With, relations...)
	return db
}

func (db *DynamicBridge) Order(field, direction string) *DynamicBridge {
	if db.err != nil {
		return db
	}
	if _, ok := db.schema.FieldByName(field); !ok {
		db.err = core.NewCode("orm.input.field", "unknown field: "+field)
		return db
	}
	db.intent.Order = append(db.intent.Order, OrderBy{Field: field, Desc: direction == "desc"})
	return db
}

func (db *DynamicBridge) Limit(n int) *DynamicBridge {
	db.intent.Limit = n
	return db
}

func (db *DynamicBridge) Offset(n int) *DynamicBridge {
	db.intent.Offset = n
	return db
}

// Find loads a row by primary key.
func (db *DynamicBridge) Find(pk ...any) core.Result {
	if db.err != nil {
		return core.Fail(db.err)
	}
	db.intent.PK = pk
	return singleMap(db.dispatchRead())
}

func (db *DynamicBridge) First() core.Result {
	if db.err != nil {
		return core.Fail(db.err)
	}
	db.intent.Limit = 1
	return singleMap(db.dispatchRead())
}

func (db *DynamicBridge) Get() core.Result {
	if db.err != nil {
		return core.Fail(db.err)
	}
	return maps(db.dispatchRead())
}

func (db *DynamicBridge) All() core.Result {
	return db.Get()
}

func (db *DynamicBridge) Count() core.Result {
	if db.err != nil {
		return core.Fail(db.err)
	}
	db.intent.Aggregate = &AggregateOp{Op: "count"}
	r := db.dispatchRead()
	if !r.OK {
		return r
	}
	if payload, ok := r.Value.(*Payload); ok {
		switch value := payload.Data.(type) {
		case []map[string]any:
			return core.Ok(int64(len(value)))
		case int64:
			return core.Ok(value)
		case int:
			return core.Ok(int64(value))
		}
	}
	switch value := r.Value.(type) {
	case int64:
		return core.Ok(value)
	case int:
		return core.Ok(int64(value))
	}
	return core.Fail(core.NewCode("orm.cast", "expected aggregate count"))
}

func (db *DynamicBridge) Stream() core.Result {
	if db.err != nil {
		return core.Fail(db.err)
	}
	medium, mr := db.resolveMedium()
	if !mr.OK {
		return mr
	}
	return medium.Stream(db.c.Context(), db.intent)
}

func (db *DynamicBridge) dispatchRead() core.Result {
	db.intent.Schema = db.schema
	medium, mr := db.resolveMedium()
	if !mr.OK {
		return mr
	}
	return medium.Read(db.c.Context(), db.intent)
}

func (db *DynamicBridge) resolveMedium() (Medium, core.Result) {
	name := db.mediumName
	if name == "" {
		name = "default"
	}
	return resolve(db.c, name)
}

func maps(r core.Result) core.Result {
	if !r.OK {
		return r
	}
	if payload, ok := r.Value.(*Payload); ok {
		if rows, ok := payload.Data.([]map[string]any); ok {
			return core.Ok(rows)
		}
	}
	if rows, ok := r.Value.([]map[string]any); ok {
		return core.Ok(rows)
	}
	return core.Fail(core.NewCode("orm.cast", "expected row slice"))
}

func singleMap(r core.Result) core.Result {
	rows := maps(r)
	if !rows.OK {
		return rows
	}
	list := rows.Value.([]map[string]any)
	if len(list) == 0 {
		return core.Fail(core.NewCode("orm.notfound", "no matching row"))
	}
	return core.Ok(list[0])
}
