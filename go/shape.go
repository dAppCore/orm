// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

func shapeRows(schema Schema, rows []any) core.Result {
	processed := make([]any, 0, len(rows))
	for _, row := range rows {
		rowMap := rowToSchemaMap(schema, row)
		for _, f := range schema.Fields {
			if val, ok := rowMap[f.Name]; ok {
				coerced := Apply(f, val)
				if !coerced.OK {
					return coerced
				}
				rowMap[f.Name] = coerced.Value
			}
		}
		processed = append(processed, rowMap)
	}
	return core.Ok(processed)
}

func shapeUpdates(schema Schema, updates map[string]any) core.Result {
	coerced := make(map[string]any, len(updates))
	for name, val := range updates {
		f, ok := schema.FieldByName(name)
		if !ok {
			return core.Fail(core.NewCode("orm.input.field", "unknown field: "+name))
		}
		r := Apply(f, val)
		if !r.OK {
			return r
		}
		coerced[name] = r.Value
	}
	return core.Ok(coerced)
}

func shapePredicates(schema Schema, preds []Predicate) core.Result {
	if len(preds) == 0 {
		return core.Ok(preds)
	}
	out := make([]Predicate, len(preds))
	for i, pred := range preds {
		shaped := pred
		if len(pred.Group) > 0 {
			group := shapePredicates(schema, pred.Group)
			if !group.OK {
				return group
			}
			shaped.Group = group.Value.([]Predicate)
			out[i] = shaped
			continue
		}
		if pred.Field == "" {
			out[i] = shaped
			continue
		}
		f, ok := schema.FieldByName(pred.Field)
		if !ok {
			return core.Fail(core.NewCode("orm.input.field", "unknown field: "+pred.Field))
		}
		if pred.Op != "null" && pred.Op != "not null" {
			coerced := shapePredicateValue(f, pred.Op, pred.Value)
			if !coerced.OK {
				return coerced
			}
			shaped.Value = coerced.Value
		}
		out[i] = shaped
	}
	return core.Ok(out)
}

func shapePredicateValue(field Field, op string, value any) core.Result {
	// ColRef names a column ("compare to a column"), not a literal — the
	// field-type shapers (int64, format validators, min/max…) have no
	// business running against it. Pass it through untouched so the
	// Medium's SQL builder can recognise it and emit a column reference
	// instead of a bound parameter. See alias.go's Col() doc comment.
	if _, isColRef := value.(ColRef); isColRef {
		return core.Ok(value)
	}
	switch core.Lower(op) {
	case "in", "not in":
		values := valueSlice(value)
		if values == nil {
			return core.Fail(core.NewCode("orm.input.type", "expected slice for "+op))
		}
		out := make([]any, len(values))
		for i, item := range values {
			coerced := Apply(field, item)
			if !coerced.OK {
				return coerced
			}
			out[i] = coerced.Value
		}
		return core.Ok(out)
	case "between":
		values := valueSlice(value)
		if len(values) != 2 {
			return core.Fail(core.NewCode("orm.input.type", "expected two bounds for between"))
		}
		out := make([]any, 2)
		for i, item := range values {
			coerced := Apply(field, item)
			if !coerced.OK {
				return coerced
			}
			out[i] = coerced.Value
		}
		return core.Ok(out)
	default:
		return Apply(field, value)
	}
}

func valueSlice(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	v := core.ValueOf(value)
	if !v.IsValid() || v.Kind() != core.KindSlice {
		return nil
	}
	out := make([]any, v.Len())
	for i := range v.Len() {
		out[i] = v.Index(i).Interface()
	}
	return out
}
