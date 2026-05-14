// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	. "dappco.re/go"
	"dappco.re/go/orm"
)

func TestBridge_DetailCarriesMeta_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "alice", "Active": true},
	}})

	res := orm.Of[BridgeUser](c).Where("Name", "=", "alice").Get()
	users, meta, ok := orm.Detail[[]BridgeUser](res)
	AssertTrue(t, ok)
	AssertEqual(t, 1, len(users))
	AssertEqual(t, "memium", meta.Medium)
	AssertEqual(t, int64(1), meta.RowsRead)
}

func TestBridge_WhereGroup_AppliesInputShape_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "alice", "Active": true},
	}})

	res := orm.Of[BridgeUser](c).WhereGroup(func(g *orm.Group) {
		g.Where("Email", "=", "not-an-email")
	}).Get()

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.input.format", res.Code())
}

func TestBridge_WhereInShapesEachValue_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "alice", "Active": true},
	}})
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(2), "Email": "b@b.com", "Name": "bob", "Active": true},
	}})

	res := orm.Of[BridgeUser](c).Where("Name", "in", []string{"alice", "bob"}).Get()

	AssertTrue(t, res.OK)
	users, ok := orm.Cast[[]BridgeUser](res)
	AssertTrue(t, ok)
	AssertEqual(t, 2, len(users))
}

func TestBridge_WhereBetweenShapesBounds_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "alice", "Active": true},
	}})
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(3), "Email": "b@b.com", "Name": "bob", "Active": true},
	}})

	res := orm.Of[BridgeUser](c).Where("ID", "between", []any{"1", "2"}).Get()

	AssertTrue(t, res.OK)
	users, ok := orm.Cast[[]BridgeUser](res)
	AssertTrue(t, ok)
	AssertEqual(t, 1, len(users))
	if len(users) > 0 {
		AssertEqual(t, "alice", users[0].Name)
	}
}

func TestService_Save_AppliesInputShape_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)
	orm.RegisterSchema(c, schema)
	orm.Register(c)

	res := c.Action("orm.save").Run(Background(), NewOptions(
		Option{Key: "table", Value: "bridge_users"},
		Option{Key: "rows", Value: []any{&BridgeUser{ID: 1, Email: "not-an-email", Name: "bad", Active: true}}},
	))

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.input.format", res.Code())
}

func TestService_AggregateReturnsScalar_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)
	orm.RegisterSchema(c, schema)
	orm.Register(c)
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "alice", "Active": true},
	}})
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(2), "Email": "b@b.com", "Name": "bob", "Active": true},
	}})

	res := c.Action("orm.aggregate").Run(Background(), NewOptions(
		Option{Key: "table", Value: "bridge_users"},
		Option{Key: "op", Value: orm.AggregateOp{Op: "count"}},
	))

	n, ok := orm.Cast[int64](res)
	AssertTrue(t, ok)
	AssertEqual(t, int64(2), n)
}

func TestMemium_ReadOrderDoesNotMutateStorage_Good(t *T) {
	m := orm.NewMemium()
	schema := testSchema("users")
	ctx := Background()
	m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"id": int64(1), "name": "z"},
	}})
	m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"id": int64(2), "name": "a"},
	}})

	ordered := m.Read(ctx, orm.ReadIntent{Schema: schema, Order: []orm.OrderBy{{Field: "name"}}})
	AssertTrue(t, ordered.OK)

	unordered := m.Read(ctx, orm.ReadIntent{Schema: schema})
	payload := unordered.Value.(*orm.Payload)
	rows := payload.Data.([]map[string]any)
	AssertEqual(t, "z", rows[0]["name"])
	AssertEqual(t, "a", rows[1]["name"])
}

func TestMemium_WriteClonesCallerRows_Good(t *T) {
	m := orm.NewMemium()
	schema := testSchema("users")
	ctx := Background()
	row := map[string]any{"id": int64(1), "name": "original"}

	res := m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{row}})
	row["name"] = "mutated"
	read := m.Read(ctx, orm.ReadIntent{Schema: schema})

	AssertTrue(t, res.OK)
	payload := read.Value.(*orm.Payload)
	rows := payload.Data.([]map[string]any)
	AssertEqual(t, "original", rows[0]["name"])
}

func TestMemium_EnforcesUniqueConstraints_Bad(t *T) {
	c, _ := setupBridgeTest(t)
	first := &BridgeUser{ID: 1, Email: "dupe@host.uk", Name: "one", Active: true}
	second := &BridgeUser{ID: 2, Email: "dupe@host.uk", Name: "two", Active: true}

	ok := orm.Of[BridgeUser](c).Save(first)
	dupe := orm.Of[BridgeUser](c).Save(second)

	AssertTrue(t, ok.OK)
	AssertTrue(t, !dupe.OK)
	AssertEqual(t, "orm.constraint", dupe.Code())
}

func TestMemium_UniqueUpdateFailureDoesNotPartiallyMutate_Bad(t *T) {
	c, _ := setupBridgeTest(t)
	one := &BridgeUser{ID: 1, Email: "one@host.uk", Name: "one", Active: true}
	two := &BridgeUser{ID: 2, Email: "two@host.uk", Name: "two", Active: true}
	AssertTrue(t, orm.Of[BridgeUser](c).Save(one).OK)
	AssertTrue(t, orm.Of[BridgeUser](c).Save(two).OK)

	res := orm.Of[BridgeUser](c).Where("Active", "=", true).Update(map[string]any{
		"Email": "same@host.uk",
	})
	all := orm.Of[BridgeUser](c).Order("ID", "asc").Get()
	users, ok := orm.Cast[[]BridgeUser](all)

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.constraint", res.Code())
	AssertTrue(t, ok)
	AssertEqual(t, "one@host.uk", users[0].Email)
	AssertEqual(t, "two@host.uk", users[1].Email)
}

func TestWatch_LiveDoesNotDropBufferedBurst_Good(t *T) {
	c, mem := setupBridgeTest(t)
	schema := ExtUser{}.Schema()
	mem.RegisterTable("ext_users", schema)
	orm.RegisterSchema(c, schema)

	ctx, cancel := WithTimeout(Background(), 800*Millisecond)
	defer cancel()

	res := orm.Of[ExtUser](c).Live().Watch(ctx)
	AssertTrue(t, res.OK)
	seq, ok := orm.Cast[Seq[orm.Event[ExtUser]]](res)
	AssertTrue(t, ok)

	const total = 90
	for i := 0; i < total; i++ {
		mem.Insert("ext_users", map[string]any{
			"id":    int64(i + 1),
			"name":  "burst",
			"email": "burst@host.uk",
			"age":   int64(i),
		})
	}

	got := make(chan int, 1)
	go func() {
		count := 0
		seq(func(ev orm.Event[ExtUser]) bool {
			if ev.Op == orm.WatchInsert {
				count++
			}
			return count < total
		})
		got <- count
	}()

	select {
	case count := <-got:
		AssertEqual(t, total, count)
	case <-ctx.Done():
		t.Fatal("timed out waiting for burst watch events")
	}
}

func TestTx_RollsBackMemiumWritesOnFailure_Good(t *T) {
	c, _ := setupBridgeTest(t)
	user := &BridgeUser{ID: 1, Email: "a@b.com", Name: "alice", Active: true}

	res := orm.Tx(c, func(tx *Tx) Result {
		save := orm.Of[BridgeUser](c).WithTx(tx).Save(user)
		AssertTrue(t, save.OK)
		return Fail(NewCode("test.rollback", "force rollback"))
	})

	AssertTrue(t, !res.OK)
	found := orm.Of[BridgeUser](c).Find(int64(1))
	AssertTrue(t, !found.OK)
	AssertEqual(t, "orm.notfound", found.Code())
}

var countingCacheMu Mutex
var countingCacheCalls int

type CountingCacheUser struct {
	ID int64
}

func (CountingCacheUser) Schema() orm.Schema {
	countingCacheMu.Lock()
	countingCacheCalls++
	countingCacheMu.Unlock()
	Sleep(10 * Millisecond)
	return orm.Define(func(b *orm.Builder) {
		b.Name("counting_cache_users")
		b.PK("id")
	})
}

func TestSchemaCache_ConcurrentFirstAccessCallsSchemaOnce_Good(t *T) {
	c := New()
	countingCacheMu.Lock()
	countingCacheCalls = 0
	countingCacheMu.Unlock()

	var wg WaitGroup
	for i := 0; i < 32; i++ {
		wg.Go(func() {
			orm.Of[CountingCacheUser](c).Where("id", "=", int64(1))
		})
	}
	wg.Wait()

	countingCacheMu.Lock()
	got := countingCacheCalls
	countingCacheMu.Unlock()
	AssertEqual(t, 1, got)
}

type OtherSaveModel struct {
	ID int64
}

func (OtherSaveModel) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("other_save_models")
		b.PK("id")
	})
}

func TestSaveRejectsMixedSchemas_Bad(t *T) {
	c, _ := setupBridgeTest(t)

	res := orm.Save(c,
		&BridgeUser{ID: 1, Email: "one@host.uk", Name: "one", Active: true},
		&OtherSaveModel{ID: 2},
	)

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.cast", res.Code())
}

func TestDeleteAppliesInputShape_Bad(t *T) {
	c, _ := setupBridgeTest(t)
	ok := orm.Save(c, &BridgeUser{ID: 1, Email: "one@host.uk", Name: "one", Active: true})
	bad := orm.Delete(c, &BridgeUser{ID: 1, Email: "not-an-email", Name: "one", Active: true})
	found := orm.Of[BridgeUser](c).Find(int64(1))

	AssertTrue(t, ok.OK)
	AssertTrue(t, !bad.OK)
	AssertEqual(t, "orm.input.format", bad.Code())
	AssertTrue(t, found.OK)
}

func TestService_AggregateUnknownOp_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)
	orm.RegisterSchema(c, schema)
	orm.Register(c)

	res := c.Action("orm.aggregate").Run(Background(), NewOptions(
		Option{Key: "table", Value: "bridge_users"},
		Option{Key: "op", Value: orm.AggregateOp{Op: "median", Field: "ID"}},
	))

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.aggregate.unsupported", res.Code())
}

func TestBuilder_PKCreatesUsableField_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("users")
		b.PK("id")
	})

	field, ok := schema.FieldByName("id")
	AssertTrue(t, ok)
	AssertEqual(t, "int64", field.Type)
}

func TestBuilder_PKDoesNotDuplicateDeclaredField_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("users")
		b.PK("id")
		b.Int64("id")
	})

	count := 0
	for _, field := range schema.Fields {
		if field.Name == "id" {
			count++
		}
	}
	AssertEqual(t, 1, count)
}

func TestInput_IntRejectsFractionalFloat_Bad(t *T) {
	res := orm.Apply(orm.Field{Name: "age", Type: "int64"}, 1.5)

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.input.type", res.Code())
}

func TestInput_UnknownCoercerFails_Bad(t *T) {
	res := orm.Apply(orm.Field{Name: "active", Type: "int64", CoerceName: "MissingCoercer"}, true)

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.input.coerce", res.Code())
}

func TestInput_FloatMinMaxUseFloatComparison_Bad(t *T) {
	min := int64(1)
	max := int64(1)
	minRes := orm.Apply(orm.Field{Name: "amount", Type: "float64", Min: &min}, 0.5)
	maxRes := orm.Apply(orm.Field{Name: "amount", Type: "float64", Max: &max}, 1.5)

	AssertTrue(t, !minRes.OK)
	AssertEqual(t, "orm.input.range", minRes.Code())
	AssertTrue(t, !maxRes.OK)
	AssertEqual(t, "orm.input.range", maxRes.Code())
}

func TestSetOp_EmptyBuildersReturnsNil_Bad(t *T) {
	AssertNil(t, orm.Union())
	AssertNil(t, orm.Intersect())
	AssertNil(t, orm.Except())
}

func TestService_FindMissingPK_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)
	orm.RegisterSchema(c, schema)
	orm.Register(c)

	res := c.Action("orm.find").Run(Background(), NewOptions(
		Option{Key: "table", Value: "bridge_users"},
	))

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.input.pk", res.Code())
}

func TestService_FindPKLengthMismatch_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)
	orm.RegisterSchema(c, schema)
	orm.Register(c)

	res := c.Action("orm.find").Run(Background(), NewOptions(
		Option{Key: "table", Value: "bridge_users"},
		Option{Key: "pk", Value: []any{int64(1), int64(2)}},
	))

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.input.pk", res.Code())
}

func TestBridge_HydratesTimeField_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	createdAt := UnixTime(1714291200)
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "alice", "Active": true, "CreatedAt": createdAt},
	}})

	res := orm.Of[BridgeUser](c).Find(int64(1))
	user, _, ok := orm.Detail[BridgeUser](res)

	AssertTrue(t, ok)
	AssertEqual(t, createdAt.Unix(), user.CreatedAt.Unix())
}

func TestDDL_DefaultsEscapeStringsAndPreserveFloats_Good(t *T) {
	c := New()
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("products")
		b.PK("id")
		b.String("title").Default("O'Reilly")
		b.Float64("price").Default(1.5)
	})

	res := orm.DDL(c, schema, "sqlite")

	AssertTrue(t, res.OK)
	ddl := res.Value.(string)
	AssertTrue(t, Contains(ddl, "DEFAULT 'O''Reilly'"))
	AssertTrue(t, Contains(ddl, "DEFAULT 1.5"))
}

func TestDiff_ExistingEmptyMemiumTableHasNoAddTable_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)

	res := orm.Diff(c, schema)

	AssertTrue(t, res.OK)
	changes := res.Value.([]orm.Change)
	AssertEqual(t, 0, len(changes))
}

func TestApplyChanges_AddTableRegistersMemiumTable_Good(t *T) {
	c, _ := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	orm.RegisterSchema(c, schema)
	changes := []orm.Change{{Op: "add_table", Info: "bridge_users"}}

	apply := orm.ApplyChanges(c, changes)
	diff := orm.Diff(c, schema)

	AssertTrue(t, apply.OK)
	AssertTrue(t, diff.OK)
	AssertEqual(t, 0, len(diff.Value.([]orm.Change)))
}

func TestBridge_StrictRejectsUnsupportedAggregate_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)
	m.MaskCaps(orm.Capabilities{Predicates: orm.PredicateCaps{Equality: true}})

	res := orm.Of[BridgeUser](c).Strict().Count()

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.aggregates.unsupported", res.Code())
}

func TestBridge_StrictRejectsUnsupportedPredicate_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)
	m.MaskCaps(orm.Capabilities{Predicates: orm.PredicateCaps{Equality: true}, Aggregates: true})

	res := orm.Of[BridgeUser](c).Where("ID", ">", int64(0)).Strict().Get()

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.predicates.unsupported", res.Code())
}

func TestBridge_StrictRejectsUnsupportedWith_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)
	m.MaskCaps(orm.Capabilities{
		Predicates: orm.PredicateCaps{Equality: true},
		Aggregates: true,
		Joins:      false,
	})

	res := orm.Of[BridgeUser](c).With("posts").Strict().Get()

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.joins.unsupported", res.Code())
}

type VectorDoc struct {
	ID        int64
	Title     string
	Embedding []byte
}

func (VectorDoc) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("vector_docs")
		b.PK("id")
		b.String("title").Searchable("text")
		b.Bytes("embedding").Searchable("vector", 3)
	})
}

func TestSearch_VectorDimensionMismatch_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := VectorDoc{}.Schema()
	m.RegisterTable("vector_docs", schema)

	res := orm.Of[VectorDoc](c).Search("needle", orm.SearchOpts{
		Mode:   "vector",
		Vector: []float32{1, 2},
	})

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.search.dimension", res.Code())
}

func TestSearch_FacetsUnsupported_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := SearchableDoc{}.Schema()
	m.RegisterTable("docs", schema)
	m.MaskCaps(orm.Capabilities{
		Search: orm.SearchCaps{Text: true, Facets: false},
	})

	res := orm.Of[SearchableDoc](c).Search("hello", orm.SearchOpts{
		Facets: []string{"Title"},
	})

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.search.unsupported", res.Code())
}

func TestSearch_DefaultModeUsesAvailableVectorCapability_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := VectorDoc{}.Schema()
	m.RegisterTable("vector_docs", schema)
	m.MaskCaps(orm.Capabilities{
		Search: orm.SearchCaps{Vector: true},
	})
	m.Insert("vector_docs", map[string]any{"id": int64(1), "title": "needle", "embedding": []byte{1, 2, 3}})

	res := orm.Of[VectorDoc](c).Search("needle", orm.SearchOpts{Vector: []float32{1, 2, 3}})

	AssertTrue(t, res.OK)
}

func TestSearch_VectorModeRequiresVectorField_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := SearchableDoc{}.Schema()
	m.RegisterTable("docs", schema)
	m.Insert("docs", map[string]any{"id": int64(1), "title": "needle", "body": "text only"})

	res := orm.Of[SearchableDoc](c).Search("needle", orm.SearchOpts{
		Mode:   "vector",
		Vector: []float32{1, 2, 3},
	})

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.search.dimension", res.Code())
}

func TestBridge_DeleteRowDoesNotDeletePredicateMatches_Bad(t *T) {
	c, _ := setupBridgeTest(t)
	one := &BridgeUser{ID: 1, Email: "one@host.uk", Name: "one", Active: true}
	two := &BridgeUser{ID: 2, Email: "two@host.uk", Name: "two", Active: true}
	AssertTrue(t, orm.Of[BridgeUser](c).Save(one).OK)
	AssertTrue(t, orm.Of[BridgeUser](c).Save(two).OK)

	res := orm.Of[BridgeUser](c).Where("Active", "=", true).Delete(one)
	foundOne := orm.Of[BridgeUser](c).Find(int64(1))
	foundTwo := orm.Of[BridgeUser](c).Find(int64(2))

	AssertTrue(t, res.OK)
	AssertTrue(t, !foundOne.OK)
	AssertTrue(t, foundTwo.OK)
}

func TestDynamicBridge_OnResolvesMediumAtDispatch_Good(t *T) {
	c := New()
	m := orm.NewMemium()
	orm.Mount(c, "warehouse", m)
	t.Cleanup(func() {
		orm.Remove(c)
	})
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)
	orm.RegisterSchema(c, schema)
	m.Insert("bridge_users", map[string]any{
		"ID":     int64(1),
		"Email":  "warehouse@host.uk",
		"Name":   "warehouse",
		"Active": true,
	})

	res := orm.OfTable(c, "bridge_users").On("warehouse").Get()

	AssertTrue(t, res.OK)
	rows, ok := orm.Cast[[]map[string]any](res)
	AssertTrue(t, ok)
	AssertEqual(t, 1, len(rows))
}

func TestDynamicBridge_WhereNullValidatesField_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)
	orm.RegisterSchema(c, schema)

	res := orm.OfTable(c, "bridge_users").WhereNull("Missing").Get()

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.input.field", res.Code())
}

func TestDynamicBridge_OrderValidatesField_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)
	orm.RegisterSchema(c, schema)

	res := orm.OfTable(c, "bridge_users").Order("Missing", "asc").Get()

	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.input.field", res.Code())
}

func TestMountSchemasSkipsNonJSONSidecars_Good(t *T) {
	fs := (&Fs{}).New("/")
	dir := t.TempDir()
	RequireTrue(t, fs.EnsureDir(Path(dir, "schemas")).OK)
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("mounted_sidecar_users")
		b.PK("id")
	})
	raw := JSONMarshal(schema)
	RequireTrue(t, raw.OK)
	RequireTrue(t, WriteFile(Path(dir, "schemas", "user.json"), raw.Value.([]byte), 0o644).OK)
	RequireTrue(t, WriteFile(Path(dir, "schemas", "README.md"), []byte("not json"), 0o644).OK)

	c := New()
	c.Data().New(NewOptions(
		Option{Key: "name", Value: "schemas"},
		Option{Key: "source", Value: DirFS(dir)},
		Option{Key: "path", Value: "schemas"},
	))
	t.Cleanup(func() {
		orm.Remove(c)
	})

	res := orm.MountSchemas(c, "schemas/.")

	AssertTrue(t, res.OK)
	if res.OK {
		AssertEqual(t, 1, res.Value.(int))
	}
}

type TaggedJSONUser struct {
	ID    int64  `json:"id"`
	Label string `json:"display_label"`
}

func (TaggedJSONUser) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("tagged_json_users")
		b.PK("id")
		b.String("display_label")
	})
}

func TestBridge_UsesJSONTagsForWriteAndHydrate_Good(t *T) {
	c, _ := setupBridgeTest(t)

	save := orm.Of[TaggedJSONUser](c).Save(&TaggedJSONUser{ID: 1, Label: "visible"})
	found := orm.Of[TaggedJSONUser](c).Find(int64(1))
	user, ok := orm.Cast[TaggedJSONUser](found)

	AssertTrue(t, save.OK)
	AssertTrue(t, found.OK)
	AssertTrue(t, ok)
	AssertEqual(t, "visible", user.Label)
}

type SortMetric struct {
	ID     int64
	Bucket string
	Score  int64
}

func (SortMetric) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("sort_metrics")
		b.PK("ID")
		b.Int64("ID")
		b.String("Bucket")
		b.Int64("Score")
	})
}

func TestMemium_OrderUsesAllKeysInPriorityOrder_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := SortMetric{}.Schema()
	m.RegisterTable("sort_metrics", schema)
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Bucket": "a", "Score": int64(1)},
		map[string]any{"ID": int64(2), "Bucket": "b", "Score": int64(100)},
		map[string]any{"ID": int64(3), "Bucket": "a", "Score": int64(2)},
		map[string]any{"ID": int64(4), "Bucket": "b", "Score": int64(50)},
	}})

	res := orm.Of[SortMetric](c).Order("Bucket", "asc").Order("Score", "desc").Get()
	items, ok := orm.Cast[[]SortMetric](res)

	AssertTrue(t, res.OK)
	AssertTrue(t, ok)
	if len(items) < 4 {
		AssertEqual(t, 4, len(items))
		return
	}
	AssertEqual(t, int64(3), items[0].ID)
	AssertEqual(t, int64(1), items[1].ID)
	AssertEqual(t, int64(2), items[2].ID)
	AssertEqual(t, int64(4), items[3].ID)
}

func TestMemium_WhereNullMatchesMissingField_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)
	m.Insert("bridge_users", map[string]any{"ID": int64(1), "Email": "missing@host.uk", "Active": true})

	res := orm.Of[BridgeUser](c).WhereNull("Name").Get()
	users, ok := orm.Cast[[]BridgeUser](res)

	AssertTrue(t, res.OK)
	AssertTrue(t, ok)
	AssertEqual(t, 1, len(users))
	if len(users) == 0 {
		return
	}
	AssertEqual(t, int64(1), users[0].ID)
}

func TestMemium_NotLikePredicate_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.RegisterTable("bridge_users", schema)
	m.Insert("bridge_users", map[string]any{"ID": int64(1), "Email": "alice@host.uk", "Name": "Alice", "Active": true})
	m.Insert("bridge_users", map[string]any{"ID": int64(2), "Email": "bob@host.uk", "Name": "Bob", "Active": true})

	res := orm.Of[BridgeUser](c).Where("Name", "not like", "A%").Get()
	users, ok := orm.Cast[[]BridgeUser](res)

	AssertTrue(t, res.OK)
	AssertTrue(t, ok)
	AssertEqual(t, 1, len(users))
	if len(users) == 0 {
		return
	}
	AssertEqual(t, "Bob", users[0].Name)
}

type OwnerByKey struct {
	Key int64
}

func (OwnerByKey) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("owners_by_key")
		b.PK("key")
		b.HasMany("pets_by_key", "owner_key")
	})
}

type PetByKey struct {
	PetKey   int64
	OwnerKey int64
	Name     string
}

func (PetByKey) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("pets_by_key")
		b.PK("pet_key")
		b.Int64("owner_key")
		b.String("name")
	})
}

func TestMemium_EagerLoadUsesSchemaPKAndClonesRelatedRows_Good(t *T) {
	m := orm.NewMemium()
	ownerSchema := OwnerByKey{}.Schema()
	petSchema := PetByKey{}.Schema()
	m.RegisterTable("owners_by_key", ownerSchema)
	m.RegisterTable("pets_by_key", petSchema)
	m.Insert("owners_by_key", map[string]any{"key": int64(7)})
	m.Insert("pets_by_key", map[string]any{"pet_key": int64(11), "owner_key": int64(7), "name": "spot"})

	res := m.Read(Background(), orm.ReadIntent{Schema: ownerSchema, With: []string{"pets_by_key"}})
	payload := res.Value.(*orm.Payload)
	rows := payload.Data.([]map[string]any)
	if len(rows) == 0 {
		t.Fatal("expected owner row")
	}
	pets, ok := rows[0]["pets_by_key"].([]map[string]any)
	AssertTrue(t, ok)
	AssertEqual(t, 1, len(pets))
	if len(pets) == 0 {
		return
	}
	AssertEqual(t, "spot", pets[0]["name"])

	pets[0]["name"] = "mutated"
	again := m.Read(Background(), orm.ReadIntent{Schema: ownerSchema, With: []string{"pets_by_key"}})
	againPayload := again.Value.(*orm.Payload)
	againRows := againPayload.Data.([]map[string]any)
	againPets := againRows[0]["pets_by_key"].([]map[string]any)
	AssertEqual(t, "spot", againPets[0]["name"])
}

type FloatMetric struct {
	ID     int64
	Amount float64
}

func (FloatMetric) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("float_metrics")
		b.PK("ID")
		b.Int64("ID")
		b.Float64("Amount")
	})
}

func TestAggregateFloatPreservesFraction_Bad(t *T) {
	c, m := setupBridgeTest(t)
	schema := FloatMetric{}.Schema()
	m.RegisterTable("float_metrics", schema)
	m.Insert("float_metrics", map[string]any{"ID": int64(1), "Amount": 1.5})
	m.Insert("float_metrics", map[string]any{"ID": int64(2), "Amount": 2.25})

	sum := orm.Of[FloatMetric](c).Sum("Amount")
	avg := orm.Of[FloatMetric](c).Avg("Amount")
	total, totalOK := orm.Cast[float64](sum)
	mean, meanOK := orm.Cast[float64](avg)

	AssertTrue(t, sum.OK)
	AssertTrue(t, avg.OK)
	AssertTrue(t, totalOK)
	AssertTrue(t, meanOK)
	AssertEqual(t, 3.75, total)
	AssertEqual(t, 1.875, mean)
}
