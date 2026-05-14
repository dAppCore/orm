// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

type FindDTO struct {
	Table string   `json:"table"`
	PK    []any    `json:"pk"`
	Tx    *core.Tx `json:"-"`
}

type GetDTO struct {
	Table  string      `json:"table"`
	Where  []Predicate `json:"where"`
	With   []string    `json:"with"`
	Order  []OrderBy   `json:"order"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
	Tx     *core.Tx    `json:"-"`
}

type SaveDTO struct {
	Table string   `json:"table"`
	Rows  []any    `json:"rows"`
	Tx    *core.Tx `json:"-"`
}

type UpdateDTO struct {
	Table string         `json:"table"`
	Where []Predicate    `json:"where"`
	Set   map[string]any `json:"set"`
	Tx    *core.Tx       `json:"-"`
}

type AggregateDTO struct {
	Table string      `json:"table"`
	Op    AggregateOp `json:"op"`
	Where []Predicate `json:"where"`
	Tx    *core.Tx    `json:"-"`
}

type DDLDTO struct {
	Schema  Schema `json:"schema"`
	Dialect string `json:"dialect"`
}

// Service exposes the bridge as a core.Service with Action registrations.
type Service struct {
	c *core.Core
}

// Register mounts the ORM service and registers action handlers.
func Register(c *core.Core) core.Result {
	s := &Service{c: c}
	if r := c.RegisterService("orm", s); !r.OK {
		return r
	}
	c.Action("orm.find", s.handleFind)
	c.Action("orm.first", s.handleFirst)
	c.Action("orm.get", s.handleGet)
	c.Action("orm.all", s.handleAll)
	c.Action("orm.count", s.handleCount)
	c.Action("orm.aggregate", s.handleAggregate)
	c.Action("orm.save", s.handleSave)
	c.Action("orm.insert", s.handleInsert)
	c.Action("orm.delete", s.handleDelete)
	c.Action("orm.update", s.handleUpdate)
	c.Action("orm.delete_all", s.handleDeleteAll)
	c.Action("orm.ddl", s.handleDDL)
	c.Action("orm.diff", s.handleDiff)
	c.Action("orm.apply", s.handleApply)
	c.Action("orm.mount_schemas", s.handleMountSchemas)
	return core.Ok(true)
}

func (s *Service) ServiceName() string { return "orm" }

func (s *Service) resolve(table string) (Schema, Medium, core.Result) {
	schema, sr := schemaByName(s.c, table)
	if !sr.OK {
		return Schema{}, nil, sr
	}
	medium, mr := resolve(s.c, "default")
	if !mr.OK {
		return Schema{}, nil, mr
	}
	return schema, medium, core.Ok(nil)
}

func (s *Service) handleFind(ctx core.Context, opts core.Options) core.Result {
	table := opts.String("table")
	pkOpt := opts.Get("pk")
	pk, ok := pkOpt.Value.([]any)
	if !pkOpt.OK || !ok || len(pk) == 0 {
		return core.Fail(core.NewCode("orm.input.pk", "pk must be a non-empty array"))
	}
	schema, medium, r := s.resolve(table)
	if !r.OK {
		return r
	}
	if len(pk) != len(schema.PK) {
		return core.Fail(core.NewCode("orm.input.pk", "primary key length mismatch"))
	}
	preds := make([]Predicate, 0, len(pk))
	for i, value := range pk {
		preds = append(preds, Predicate{Field: schema.PK[i], Op: "=", Value: value})
	}
	shaped := shapePredicates(schema, preds)
	if !shaped.OK {
		return shaped
	}
	pk = make([]any, 0, len(shaped.Value.([]Predicate)))
	for _, pred := range shaped.Value.([]Predicate) {
		pk = append(pk, pred.Value)
	}
	return singleMap(medium.Read(ctx, ReadIntent{Schema: schema, PK: pk}))
}

func (s *Service) handleFirst(ctx core.Context, opts core.Options) core.Result {
	dto := getDTO(opts)
	schema, medium, r := s.resolve(dto.Table)
	if !r.OK {
		return r
	}
	where := shapePredicates(schema, dto.Where)
	if !where.OK {
		return where
	}
	dto.Limit = 1
	return singleMap(medium.Read(ctx, ReadIntent{Schema: schema, Where: where.Value.([]Predicate), Limit: dto.Limit, Tx: dto.Tx}))
}

func (s *Service) handleGet(ctx core.Context, opts core.Options) core.Result {
	dto := getDTO(opts)
	schema, medium, r := s.resolve(dto.Table)
	if !r.OK {
		return r
	}
	where := shapePredicates(schema, dto.Where)
	if !where.OK {
		return where
	}
	return maps(medium.Read(ctx, ReadIntent{Schema: schema, Where: where.Value.([]Predicate), With: dto.With, Order: dto.Order, Limit: dto.Limit, Offset: dto.Offset, Tx: dto.Tx}))
}

func (s *Service) handleAll(ctx core.Context, opts core.Options) core.Result {
	table := opts.String("table")
	schema, medium, r := s.resolve(table)
	if !r.OK {
		return r
	}
	return maps(medium.Read(ctx, ReadIntent{Schema: schema}))
}

func (s *Service) handleCount(ctx core.Context, opts core.Options) core.Result {
	dto := getDTO(opts)
	schema, medium, r := s.resolve(dto.Table)
	if !r.OK {
		return r
	}
	where := shapePredicates(schema, dto.Where)
	if !where.OK {
		return where
	}
	rows := maps(medium.Read(ctx, ReadIntent{Schema: schema, Where: where.Value.([]Predicate)}))
	if !rows.OK {
		return rows
	}
	return core.Ok(int64(len(rows.Value.([]map[string]any))))
}

func (s *Service) handleAggregate(ctx core.Context, opts core.Options) core.Result {
	table := opts.String("table")
	op, _ := opts.Get("op").Value.(AggregateOp)
	where, _ := opts.Get("where").Value.([]Predicate)
	schema, medium, r := s.resolve(table)
	if !r.OK {
		return r
	}
	shaped := shapePredicates(schema, where)
	if !shaped.OK {
		return shaped
	}
	rows := maps(medium.Read(ctx, ReadIntent{Schema: schema, Where: shaped.Value.([]Predicate)}))
	if !rows.OK {
		return rows
	}
	return aggregateResult(rows.Value.([]map[string]any), op)
}

func (s *Service) handleSave(ctx core.Context, opts core.Options) core.Result {
	return s.write(ctx, opts, OpSave)
}

func (s *Service) handleInsert(ctx core.Context, opts core.Options) core.Result {
	return s.write(ctx, opts, OpInsert)
}

func (s *Service) handleDelete(ctx core.Context, opts core.Options) core.Result {
	return s.write(ctx, opts, OpDelete)
}

func (s *Service) handleUpdate(ctx core.Context, opts core.Options) core.Result {
	table := opts.String("table")
	where, _ := opts.Get("where").Value.([]Predicate)
	set, _ := opts.Get("set").Value.(map[string]any)
	schema, medium, r := s.resolve(table)
	if !r.OK {
		return r
	}
	shapedWhere := shapePredicates(schema, where)
	if !shapedWhere.OK {
		return shapedWhere
	}
	shapedSet := shapeUpdates(schema, set)
	if !shapedSet.OK {
		return shapedSet
	}
	return medium.Write(ctx, WriteIntent{Op: OpUpdate, Schema: schema, Where: shapedWhere.Value.([]Predicate), Updates: shapedSet.Value.(map[string]any)})
}

func (s *Service) handleDeleteAll(ctx core.Context, opts core.Options) core.Result {
	table := opts.String("table")
	where, _ := opts.Get("where").Value.([]Predicate)
	schema, medium, r := s.resolve(table)
	if !r.OK {
		return r
	}
	shaped := shapePredicates(schema, where)
	if !shaped.OK {
		return shaped
	}
	return medium.Write(ctx, WriteIntent{Op: OpDelete, Schema: schema, Where: shaped.Value.([]Predicate)})
}

func (s *Service) handleDDL(ctx core.Context, opts core.Options) core.Result {
	schema, _ := opts.Get("schema").Value.(Schema)
	dialect := opts.String("dialect")
	return DDL(s.c, schema, dialect)
}

func (s *Service) handleDiff(ctx core.Context, opts core.Options) core.Result {
	schema, _ := opts.Get("schema").Value.(Schema)
	return Diff(s.c, schema)
}

func (s *Service) handleApply(ctx core.Context, opts core.Options) core.Result {
	changes, _ := opts.Get("changes").Value.([]Change)
	_ = ctx
	return ApplyChanges(s.c, changes)
}

func (s *Service) handleMountSchemas(ctx core.Context, opts core.Options) core.Result {
	return MountSchemas(s.c, opts.String("prefix"))
}

func (s *Service) write(ctx core.Context, opts core.Options, op WriteOp) core.Result {
	table := opts.String("table")
	rows, _ := opts.Get("rows").Value.([]any)
	schema, medium, r := s.resolve(table)
	if !r.OK {
		return r
	}
	shaped := shapeRows(schema, rows)
	if !shaped.OK {
		return shaped
	}
	return medium.Write(ctx, WriteIntent{Op: op, Schema: schema, Rows: shaped.Value.([]any)})
}

func getDTO(opts core.Options) GetDTO {
	dto := GetDTO{Table: opts.String("table"), Limit: opts.Int("limit"), Offset: opts.Int("offset")}
	dto.Where, _ = opts.Get("where").Value.([]Predicate)
	dto.With, _ = opts.Get("with").Value.([]string)
	dto.Order, _ = opts.Get("order").Value.([]OrderBy)
	dto.Tx, _ = opts.Get("tx").Value.(*core.Tx)
	return dto
}

func aggregateResult(rows []map[string]any, op AggregateOp) core.Result {
	switch op.Op {
	case "count":
		return core.Ok(int64(len(rows)))
	case "sum":
		return core.Ok(aggregateSum(rows, op.Field))
	case "min":
		return core.Ok(aggregateMin(rows, op.Field))
	case "max":
		return core.Ok(aggregateMax(rows, op.Field))
	case "avg":
		return core.Ok(aggregateAvg(rows, op.Field))
	default:
		return core.Fail(core.NewCode("orm.aggregate.unsupported", "unknown aggregate op: "+op.Op))
	}
}
