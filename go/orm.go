// SPDX-License-Identifier: EUPL-1.2

// Package orm — top-level entry points.
package orm

import (
	"dappco.re/go"
)

// Of is the generic entry point for the typed fluent bridge.
// Resolves the schema cache, creates a Bridge[T], defaults Medium to "default".
//
//	res := orm.Of[User](c).Find(1)
//	user, _ := core.Cast[User](res)
func Of[T any](c *core.Core) *Bridge[T] {
	return &Bridge[T]{
		c: c,
	}
}

// Save persists rows (insert or update based on PK presence).
//
//	orm.Save(c, &user)
//	orm.Save(c, &user, &post)
func Save(c *core.Core, rows ...any) core.Result {
	rows, tx := splitWriteArgs(rows)
	if len(rows) == 0 {
		return core.Ok(nil)
	}

	schema, sr := schemaForRows(rows)
	if !sr.OK {
		return sr
	}
	medium, mr := resolve(c, "default")
	if !mr.OK {
		return mr
	}
	processedRows := shapeRows(schema, rows)
	if !processedRows.OK {
		return processedRows
	}

	return medium.Write(c.Context(), WriteIntent{
		Op:     OpSave,
		Schema: schema,
		Rows:   processedRows.Value.([]any),
		Tx:     tx,
	})
}

// Tx opens a transaction on the Medium that owns the resolved schema,
// executes fn, and commits or rolls back based on fn's result.
//
//	tres := orm.Tx(c, func(tx *core.Tx) core.Result {
//	    user.Tier = "pro"
//	    return orm.Save(c, &user, orm.WithTx(tx))
//	})
func Tx(c *core.Core, fn func(*core.Tx) core.Result) core.Result {
	medium, mr := resolve(c, "default")
	if !mr.OK {
		return mr
	}
	caps := medium.Caps()
	if !caps.Transactions {
		return core.Fail(core.NewCode("orm.tx.unsupported", "medium does not support transactions"))
	}

	if snapshot, ok := medium.(interface {
		snapshot() any
		restore(any)
	}); ok {
		state := snapshot.snapshot()
		r := fn(nil)
		if !r.OK {
			snapshot.restore(state)
		}
		return r
	}

	r := fn(nil)
	return r
}

// WithTx returns an Option for threading a transaction handle into bridge calls.
//
//	orm.Save(c, &user, orm.WithTx(tx))
func WithTx(tx *core.Tx) core.Option {
	return core.Option{Key: "tx", Value: tx}
}

// Insert performs an explicit insert (fails if PK already present on some Mediums).
//
//	orm.Insert(c, &user)
func Insert(c *core.Core, rows ...any) core.Result {
	rows, tx := splitWriteArgs(rows)
	if len(rows) == 0 {
		return core.Ok(nil)
	}
	schema, sr := schemaForRows(rows)
	if !sr.OK {
		return sr
	}
	medium, mr := resolve(c, "default")
	if !mr.OK {
		return mr
	}
	processedRows := shapeRows(schema, rows)
	if !processedRows.OK {
		return processedRows
	}
	return medium.Write(c.Context(), WriteIntent{
		Op:     OpInsert,
		Schema: schema,
		Rows:   processedRows.Value.([]any),
		Tx:     tx,
	})
}

// Delete removes rows from their table.
//
//	orm.Delete(c, &user)
func Delete(c *core.Core, rows ...any) core.Result {
	rows, tx := splitWriteArgs(rows)
	if len(rows) == 0 {
		return core.Ok(nil)
	}
	schema, sr := schemaForRows(rows)
	if !sr.OK {
		return sr
	}
	medium, mr := resolve(c, "default")
	if !mr.OK {
		return mr
	}
	processedRows := shapeRows(schema, rows)
	if !processedRows.OK {
		return processedRows
	}
	return medium.Write(c.Context(), WriteIntent{
		Op:     OpDelete,
		Schema: schema,
		Rows:   processedRows.Value.([]any),
		Tx:     tx,
	})
}

func schemaForRows(rows []any) (Schema, core.Result) {
	var schema Schema
	for i, row := range rows {
		modelled, ok := row.(Modelled)
		if !ok {
			return Schema{}, core.Fail(core.NewCode("orm.cast", "value does not implement Modelled"))
		}
		rowSchema := modelled.Schema()
		if i == 0 {
			schema = rowSchema
			continue
		}
		if !core.DeepEqual(schema, rowSchema) {
			return Schema{}, core.Fail(core.NewCode("orm.cast", "rows must share one schema"))
		}
	}
	return schema, core.Ok(nil)
}

func splitWriteArgs(items []any) ([]any, *core.Tx) {
	rows := make([]any, 0, len(items))
	var tx *core.Tx
	for _, item := range items {
		opt, ok := item.(core.Option)
		if ok && opt.Key == "tx" {
			tx, _ = opt.Value.(*core.Tx)
			continue
		}
		rows = append(rows, item)
	}
	return rows, tx
}
