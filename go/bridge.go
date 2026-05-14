// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

// Bridge is the fluent query builder for a single model type T.
// Accumulates predicates and options then dispatches to a Medium on terminal verbs.
//
//	res := orm.Of[User](c).Where("email","=","x@y").First()
type Bridge[T any] struct {
	c          *core.Core
	readIntent ReadIntent
	mediumName string
	err        error
	strict     bool
}

// Where adds an AND predicate. Value passes through input.Apply per Schema.
//
//	.Where("active", "=", true)
func (b *Bridge[T]) Where(field, op string, value any) *Bridge[T] {
	if b.err != nil {
		return b
	}
	schema := b.schema()
	f, ok := schema.FieldByName(field)
	if !ok {
		b.err = core.NewCode("orm.input.field", "unknown field: "+field)
		return b
	}
	coerced := shapePredicateValue(f, op, value)
	if !coerced.OK {
		b.err = resultError(coerced, "orm.input.coerce", "coercion failed for field: "+field)
		return b
	}
	b.readIntent.Where = append(b.readIntent.Where, Predicate{Field: field, Op: op, Value: coerced.Value})
	return b
}

// OrWhere adds an OR predicate. Value passes through input.Apply per Schema.
//
//	.OrWhere("admin", "=", true)
func (b *Bridge[T]) OrWhere(field, op string, value any) *Bridge[T] {
	if b.err != nil {
		return b
	}
	schema := b.schema()
	f, ok := schema.FieldByName(field)
	if !ok {
		b.err = core.NewCode("orm.input.field", "unknown field: "+field)
		return b
	}
	coerced := shapePredicateValue(f, op, value)
	if !coerced.OK {
		b.err = resultError(coerced, "orm.input.coerce", "coercion failed for field: "+field)
		return b
	}
	b.readIntent.Where = append(b.readIntent.Where, Predicate{Field: field, Op: op, Value: coerced.Value, OR: true})
	return b
}

// WhereNotNull adds a "NOT NULL" predicate.
//
//	.WhereNotNull("email_verified_at")
func (b *Bridge[T]) WhereNotNull(field string) *Bridge[T] {
	if b.err != nil {
		return b
	}
	if _, ok := b.schema().FieldByName(field); !ok {
		b.err = core.NewCode("orm.input.field", "unknown field: "+field)
		return b
	}
	b.readIntent.Where = append(b.readIntent.Where, Predicate{Field: field, Op: "not null"})
	return b
}

// WhereNull adds an "IS NULL" predicate.
//
//	.WhereNull("deleted_at")
func (b *Bridge[T]) WhereNull(field string) *Bridge[T] {
	if b.err != nil {
		return b
	}
	if _, ok := b.schema().FieldByName(field); !ok {
		b.err = core.NewCode("orm.input.field", "unknown field: "+field)
		return b
	}
	b.readIntent.Where = append(b.readIntent.Where, Predicate{Field: field, Op: "null"})
	return b
}

// WhereGroup adds a parenthesised OR group.
//
//	.WhereGroup(func(g *orm.Group) {
//	    g.Where("tier", "=", "pro")
//	    g.OrWhere("admin", "=", true)
//	})
func (b *Bridge[T]) WhereGroup(fn func(*Group)) *Bridge[T] {
	if b.err != nil {
		return b
	}
	g := &Group{schema: b.schema()}
	fn(g)
	if g.err != nil {
		b.err = g.err
		return b
	}
	b.readIntent.Where = append(b.readIntent.Where, Predicate{Group: g.preds})
	return b
}

// Group is a builder for OR-grouped predicates within a WhereGroup.
type Group struct {
	schema Schema
	preds  []Predicate
	err    error
}

// Where adds an AND condition within the group.
func (g *Group) Where(field, op string, value any) *Group {
	g.add(field, op, value, false)
	return g
}

// OrWhere adds an OR condition within the group.
func (g *Group) OrWhere(field, op string, value any) *Group {
	g.add(field, op, value, true)
	return g
}

func (g *Group) add(field, op string, value any, or bool) {
	if g.err != nil {
		return
	}
	f, ok := g.schema.FieldByName(field)
	if !ok {
		g.err = core.NewCode("orm.input.field", "unknown field: "+field)
		return
	}
	coerced := shapePredicateValue(f, op, value)
	if !coerced.OK {
		g.err = resultError(coerced, "orm.input.coerce", "coercion failed for field: "+field)
		return
	}
	g.preds = append(g.preds, Predicate{Field: field, Op: op, Value: coerced.Value, OR: or})
}

// With declares eager-loading relations.
//
//	.With("posts")
//	.With("posts.comments")
func (b *Bridge[T]) With(relations ...string) *Bridge[T] {
	if b.err != nil {
		return b
	}
	b.readIntent.With = append(b.readIntent.With, relations...)
	return b
}

// Order adds an ordering directive.
//
//	.Order("created_at", "desc")
func (b *Bridge[T]) Order(field, direction string) *Bridge[T] {
	if b.err != nil {
		return b
	}
	desc := direction == "desc"
	b.readIntent.Order = append(b.readIntent.Order, OrderBy{Field: field, Desc: desc})
	return b
}

// Limit sets the maximum number of rows returned.
//
//	.Limit(20)
func (b *Bridge[T]) Limit(n int) *Bridge[T] {
	b.readIntent.Limit = n
	return b
}

// Offset sets the number of rows to skip.
//
//	.Offset(40)
func (b *Bridge[T]) Offset(n int) *Bridge[T] {
	b.readIntent.Offset = n
	return b
}

// Strict upgrades silent degradations to hard failures.
//
//	.Strict()
func (b *Bridge[T]) Strict() *Bridge[T] {
	b.strict = true
	b.readIntent.Strict = true
	return b
}

// On sets the Medium name to dispatch against. Defaults to "default".
//
//	.On("warehouse")
func (b *Bridge[T]) On(mediumName string) *Bridge[T] {
	b.mediumName = mediumName
	return b
}

func (b *Bridge[T]) medium() string {
	if b.mediumName == "" {
		return "default"
	}
	return b.mediumName
}

// WithTx threads a transaction through subsequent terminal calls.
func (b *Bridge[T]) WithTx(tx *core.Tx) *Bridge[T] {
	b.readIntent.Tx = tx
	return b
}

// Find loads a single row by primary key.
//
//	res := orm.Of[User](c).Find(1)
func (b *Bridge[T]) Find(pk ...any) core.Result {
	if b.err != nil {
		return core.Fail(b.err)
	}
	b.readIntent.PK = pk
	return b.dispatchRead()
}

// First loads the first matching row, fails with orm.notfound if zero rows.
//
//	res := orm.Of[User](c).Where("email", "=", "x@y").First()
func (b *Bridge[T]) First() core.Result {
	if b.err != nil {
		return core.Fail(b.err)
	}
	b.readIntent.Limit = 1
	return b.dispatchRead()
}

// Get loads all matching rows.
//
//	res := orm.Of[User](c).Where("active", "=", true).Get()
func (b *Bridge[T]) Get() core.Result {
	return b.dispatchRead()
}

// All loads every row in the table.
//
//	res := orm.Of[User](c).All()
func (b *Bridge[T]) All() core.Result {
	return b.dispatchRead()
}

// Count returns the number of matching rows.
//
//	n, ok := core.Cast[int64](orm.Of[User](c).Count())
func (b *Bridge[T]) Count() core.Result {
	if b.err != nil {
		return core.Fail(b.err)
	}
	b.readIntent.Aggregate = &AggregateOp{Op: "count"}
	return b.dispatchRead()
}

// Sum returns the sum of a numeric field.
//
//	res := orm.Of[Order](c).Sum("total")
func (b *Bridge[T]) Sum(field string) core.Result {
	if b.err != nil {
		return core.Fail(b.err)
	}
	b.readIntent.Aggregate = &AggregateOp{Op: "sum", Field: field}
	return b.dispatchRead()
}

// Min returns the minimum value of a field.
func (b *Bridge[T]) Min(field string) core.Result {
	if b.err != nil {
		return core.Fail(b.err)
	}
	b.readIntent.Aggregate = &AggregateOp{Op: "min", Field: field}
	return b.dispatchRead()
}

// Max returns the maximum value of a field.
func (b *Bridge[T]) Max(field string) core.Result {
	if b.err != nil {
		return core.Fail(b.err)
	}
	b.readIntent.Aggregate = &AggregateOp{Op: "max", Field: field}
	return b.dispatchRead()
}

// Avg returns the average value of a numeric field.
func (b *Bridge[T]) Avg(field string) core.Result {
	if b.err != nil {
		return core.Fail(b.err)
	}
	b.readIntent.Aggregate = &AggregateOp{Op: "avg", Field: field}
	return b.dispatchRead()
}

// Stream returns a core.Seq[T] iterator over matching rows.
//
//	seq, ok := core.Cast[core.Seq[User]](orm.Of[User](c).Stream())
func (b *Bridge[T]) Stream() core.Result {
	if b.err != nil {
		return core.Fail(b.err)
	}
	schema := b.schema()
	if schema.Name == "" {
		return core.Fail(core.NewCode("orm.schema.missing", "type does not implement Modelled"))
	}
	b.readIntent.Schema = schema
	medium, mr := b.resolveMedium(b.medium())
	if !mr.OK {
		return mr
	}
	r := medium.Stream(b.c.Context(), b.readIntent)
	if !r.OK {
		return r
	}
	// Rehydrate stream rows
	payload, ok := r.Value.(*Payload)
	if !ok {
		return core.Fail(core.NewCode("orm.cast", "medium returned unexpected payload type"))
	}
	rawSeq, ok := payload.Data.(func(yield func(map[string]any) bool))
	if !ok {
		return core.Fail(core.NewCode("orm.stream.data", "unexpected stream payload data type"))
	}
	seq := func(yield func(T) bool) {
		rawSeq(func(row map[string]any) bool {
			var val T
			populated := populateStructFromRow(&val, schema, row)
			if !populated.OK {
				return false
			}
			return yield(val)
		})
	}
	payload.Data = core.Seq[T](seq)
	return core.Ok(payload)
}

// schema returns the cached schema for type T.
func (b *Bridge[T]) schema() Schema {
	return cacheGet[T](b.c)
}

// resolveMedium looks up the named Medium from the Core's registry.
func (b *Bridge[T]) resolveMedium(name string) (Medium, core.Result) {
	return resolve(b.c, name)
}

// dispatchRead sends the accumulated ReadIntent to the Medium.
func (b *Bridge[T]) dispatchRead() core.Result {
	if b.err != nil {
		return core.Fail(b.err)
	}
	schema := b.schema()
	if schema.Name == "" {
		return core.Fail(core.NewCode("orm.schema.missing", "type does not implement Modelled"))
	}
	b.readIntent.Schema = schema

	medium, mr := b.resolveMedium(b.medium())
	if !mr.OK {
		return mr
	}
	caps := medium.Caps()
	if len(b.readIntent.Alias) > 0 && !caps.Aliases {
		return core.Fail(core.NewCode("orm.aliases.unsupported", "medium does not support aliased FROM"))
	}
	degraded, capR := validateReadCapabilities(caps, b.readIntent)
	if !capR.OK {
		return capR
	}

	r := medium.Read(b.c.Context(), b.readIntent)
	if !r.OK {
		return r
	}

	payload, ok := r.Value.(*Payload)
	if !ok {
		return core.Fail(core.NewCode("orm.cast", "medium returned unexpected payload"))
	}
	if degraded != "" {
		payload.Meta.Degraded = degraded
	}

	// Handle aggregates
	if b.readIntent.Aggregate != nil {
		rows, ok := payload.Data.([]map[string]any)
		if !ok {
			return withResultMeta(int64(0), payload.Meta)
		}
		var value any
		switch b.readIntent.Aggregate.Op {
		case "count":
			value = int64(len(rows))
		case "sum":
			value = aggregateSum(rows, b.readIntent.Aggregate.Field)
		case "min":
			value = aggregateMin(rows, b.readIntent.Aggregate.Field)
		case "max":
			value = aggregateMax(rows, b.readIntent.Aggregate.Field)
		case "avg":
			value = aggregateAvg(rows, b.readIntent.Aggregate.Field)
		default:
			value = int64(0)
		}
		return withResultMeta(value, payload.Meta)
	}

	// Handle single result (Find/First)
	if b.readIntent.PK != nil || b.readIntent.Limit == 1 {
		rows, ok := payload.Data.([]map[string]any)
		if !ok || len(rows) == 0 {
			return core.Fail(core.NewCode("orm.notfound", "no matching row"))
		}
		var val T
		populateR := populateStructFromRow(&val, schema, rows[0])
		if !populateR.OK {
			return populateR
		}
		return withResultMeta(val, payload.Meta)
	}

	// Handle multiple results (Get/All)
	rows, ok := payload.Data.([]map[string]any)
	if !ok {
		return core.Fail(core.NewCode("orm.cast", "medium returned unexpected data shape"))
	}
	result := make([]T, len(rows))
	for i, row := range rows {
		populateR := populateStructFromRow(&result[i], schema, row)
		if !populateR.OK {
			return populateR
		}
	}
	return withResultMeta(result, payload.Meta)
}

// Save persists row as insert-or-update.
func (b *Bridge[T]) Save(row *T) core.Result {
	return b.write(OpSave, []any{row}, nil)
}

// Insert explicitly inserts row.
func (b *Bridge[T]) Insert(row *T) core.Result {
	return b.write(OpInsert, []any{row}, nil)
}

// Delete removes row by primary key.
func (b *Bridge[T]) Delete(row *T) core.Result {
	return b.write(OpDelete, []any{row}, nil)
}

// Update applies values to rows matched by the current predicate chain.
func (b *Bridge[T]) Update(values map[string]any) core.Result {
	return b.write(OpUpdate, nil, values)
}

// DeleteAll deletes rows matched by the current predicate chain.
func (b *Bridge[T]) DeleteAll() core.Result {
	return b.write(OpDelete, nil, nil)
}

func (b *Bridge[T]) write(op WriteOp, rows []any, updates map[string]any) core.Result {
	if b.err != nil {
		return core.Fail(b.err)
	}
	schema := b.schema()
	if schema.Name == "" {
		return core.Fail(core.NewCode("orm.schema.missing", "type does not implement Modelled"))
	}
	medium, mr := b.resolveMedium(b.medium())
	if !mr.OK {
		return mr
	}
	processed := []any{}
	if len(rows) > 0 {
		shaped := shapeRows(schema, rows)
		if !shaped.OK {
			return shaped
		}
		processed = shaped.Value.([]any)
	}
	coercedUpdates := map[string]any{}
	if len(updates) > 0 {
		shaped := shapeUpdates(schema, updates)
		if !shaped.OK {
			return shaped
		}
		coercedUpdates = shaped.Value.(map[string]any)
	}
	return medium.Write(b.c.Context(), WriteIntent{
		Op:      op,
		Schema:  schema,
		Rows:    processed,
		Where:   b.readIntent.Where,
		Updates: coercedUpdates,
		Tx:      b.readIntent.Tx,
	})
}

// populateStructFromRow populates a typed struct from a row map using rehydration.
func populateStructFromRow(target any, schema Schema, row map[string]any) core.Result {
	v := core.ValueOf(target)
	if v.Kind() != core.KindPointer || v.IsNil() {
		return core.Fail(core.NewCode("orm.cast", "target must be non-nil pointer"))
	}
	elem := v.Elem()
	if elem.Kind() != core.KindStruct {
		return core.Fail(core.NewCode("orm.cast", "target must point to struct"))
	}
	t := elem.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		names, ok := fieldNames(field.Name, field.Tag.Get("json"))
		if !ok {
			continue
		}
		val, matchedName, exists := rowValueForField(row, names)
		if !exists {
			continue
		}
		schemaField := findSchemaField(schema, matchedName)
		if schemaField.Name == "" {
			schemaField = findSchemaFieldForNames(schema, names)
		}
		if schemaField.Name != "" {
			rehydrated := RehydrateApply(schemaField, val)
			if rehydrated.OK {
				setFieldValue(elem.Field(i), rehydrated.Value)
			}
		} else {
			setFieldValue(elem.Field(i), val)
		}
	}
	return core.Ok(target)
}

// setFieldValue sets a struct field value with proper type conversion.
func setFieldValue(field core.Value, val any) {
	if val == nil {
		return
	}
	if !field.CanSet() {
		return
	}
	source := core.ValueOf(val)
	if source.IsValid() {
		if source.Type().AssignableTo(field.Type()) {
			field.Set(source)
			return
		}
		if source.Type().ConvertibleTo(field.Type()) {
			field.Set(source.Convert(field.Type()))
			return
		}
	}
	switch field.Kind() {
	case core.KindString:
		if s, ok := val.(string); ok {
			field.SetString(s)
		}
	case core.KindInt64, core.KindInt:
		switch v := val.(type) {
		case int64:
			field.SetInt(v)
		case int:
			field.SetInt(int64(v))
		case float64:
			field.SetInt(int64(v))
		}
	case core.KindBool:
		if b, ok := val.(bool); ok {
			field.SetBool(b)
		}
	case core.KindFloat64:
		switch v := val.(type) {
		case float64:
			field.SetFloat(v)
		case int64:
			field.SetFloat(float64(v))
		case int:
			field.SetFloat(float64(v))
		}
	}
}

func aggregateSum(rows []map[string]any, field string) any {
	var sum float64
	allInt := true
	seen := false
	for _, row := range rows {
		if val, ok := row[field]; ok {
			num, ok, isInt := numericValue(val)
			if !ok {
				continue
			}
			sum += num
			allInt = allInt && isInt
			seen = true
		}
	}
	if !seen {
		return int64(0)
	}
	return aggregateNumber(sum, allInt)
}

func aggregateMin(rows []map[string]any, field string) any {
	var min float64
	allInt := true
	seen := false
	for _, row := range rows {
		if val, ok := row[field]; ok {
			num, ok, isInt := numericValue(val)
			if !ok {
				continue
			}
			if !seen || num < min {
				min = num
			}
			allInt = allInt && isInt
			seen = true
		}
	}
	if !seen {
		return int64(0)
	}
	return aggregateNumber(min, allInt)
}

func aggregateMax(rows []map[string]any, field string) any {
	var max float64
	allInt := true
	seen := false
	for _, row := range rows {
		if val, ok := row[field]; ok {
			num, ok, isInt := numericValue(val)
			if !ok {
				continue
			}
			if !seen || num > max {
				max = num
			}
			allInt = allInt && isInt
			seen = true
		}
	}
	if !seen {
		return int64(0)
	}
	return aggregateNumber(max, allInt)
}

func aggregateAvg(rows []map[string]any, field string) float64 {
	var sum float64
	var count int64
	for _, row := range rows {
		if val, ok := row[field]; ok {
			num, ok, _ := numericValue(val)
			if !ok {
				continue
			}
			sum += num
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func findSchemaField(schema Schema, fieldName string) Field {
	for _, f := range schema.Fields {
		if normalizeName(f.Name) == normalizeName(fieldName) {
			return f
		}
	}
	return Field{}
}

func numericValue(v any) (float64, bool, bool) {
	switch val := v.(type) {
	case int64:
		return float64(val), true, true
	case int:
		return float64(val), true, true
	case int32:
		return float64(val), true, true
	case float64:
		return val, true, false
	}
	return 0, false, false
}

func aggregateNumber(value float64, allInt bool) any {
	if allInt {
		return int64(value)
	}
	return value
}

func validateReadCapabilities(caps Capabilities, in ReadIntent) (string, core.Result) {
	var degraded []string
	if in.Aggregate != nil && !caps.Aggregates {
		degraded = append(degraded, "aggregates")
	}
	if len(in.With) > 0 && !caps.Joins {
		degraded = append(degraded, "joins")
	}
	if !predicatesSupported(caps.Predicates, in.Where) {
		degraded = append(degraded, "predicates")
	}
	if in.SetOp != nil && !caps.SetOps {
		degraded = append(degraded, "setops")
	}
	if len(degraded) == 0 {
		return "", core.Ok(nil)
	}
	if in.Strict {
		code := capabilityErrorCode(degraded[0])
		return "", core.Fail(core.NewCode(code, "medium does not support "+degraded[0]))
	}
	return core.Join(",", degraded...), core.Ok(nil)
}

func capabilityErrorCode(name string) string {
	switch name {
	case "aggregates":
		return "orm.aggregates.unsupported"
	case "joins":
		return "orm.joins.unsupported"
	case "predicates":
		return "orm.predicates.unsupported"
	case "setops":
		return "orm.setops.unsupported"
	default:
		return "orm.capability.unsupported"
	}
}

func predicatesSupported(caps PredicateCaps, preds []Predicate) bool {
	for _, pred := range preds {
		if len(pred.Group) > 0 {
			if !predicatesSupported(caps, pred.Group) {
				return false
			}
			continue
		}
		if !predicateSupported(caps, pred.Op) {
			return false
		}
	}
	return true
}

func predicateSupported(caps PredicateCaps, op string) bool {
	switch core.Lower(op) {
	case "=", "!=":
		return caps.Equality
	case "<", "<=", ">", ">=":
		return caps.Comparison
	case "in", "not in":
		return caps.In
	case "like", "not like":
		return caps.Like
	case "null", "not null":
		return caps.Null
	case "between":
		return caps.Between
	default:
		return false
	}
}
