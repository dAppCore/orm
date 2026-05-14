// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	. "dappco.re/go"
	"dappco.re/go/orm"
)

func testSchema(name string) orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name(name)
		b.PK("id")
		b.Int64("id")
		b.String("name").NotNull()
	})
}

func TestMemium_New_Good(t *T) {
	m := orm.NewMemium()
	caps := m.Caps()
	AssertTrue(t, caps.Predicates.Equality)
	AssertTrue(t, caps.Predicates.Comparison)
	AssertTrue(t, caps.Transactions)
}

func TestMemium_ReadEmpty_Good(t *T) {
	m := orm.NewMemium()
	ctx := Background()
	intent := orm.ReadIntent{Schema: testSchema("users")}
	r := m.Read(ctx, intent)
	AssertTrue(t, r.OK)
}

func TestMemium_WriteInsert_Good(t *T) {
	m := orm.NewMemium()
	ctx := Background()
	schema := testSchema("users")

	row := map[string]any{"id": int64(1), "name": "snider"}
	intent := orm.WriteIntent{
		Op:     orm.OpInsert,
		Schema: schema,
		Rows:   []any{row},
	}
	r := m.Write(ctx, intent)
	AssertTrue(t, r.OK)

	// Read back
	readIntent := orm.ReadIntent{Schema: schema}
	readR := m.Read(ctx, readIntent)
	AssertTrue(t, readR.OK)
	payload := readR.Value.(*orm.Payload)
	rows := payload.Data.([]map[string]any)
	AssertEqual(t, 1, len(rows))
	AssertEqual(t, "snider", rows[0]["name"])
}

func TestMemium_WriteSave_Good(t *T) {
	m := orm.NewMemium()
	ctx := Background()
	schema := testSchema("users")

	// Insert via Save
	row := map[string]any{"id": int64(1), "name": "snider"}
	r := m.Write(ctx, orm.WriteIntent{Op: orm.OpSave, Schema: schema, Rows: []any{row}})
	AssertTrue(t, r.OK)

	// Read — should have 1 row
	readR := m.Read(ctx, orm.ReadIntent{Schema: schema})
	payload := readR.Value.(*orm.Payload)
	AssertEqual(t, 1, len(payload.Data.([]map[string]any)))

	// Update via Save
	row["name"] = "updated"
	r = m.Write(ctx, orm.WriteIntent{Op: orm.OpSave, Schema: schema, Rows: []any{row}})
	AssertTrue(t, r.OK)

	// Read — should still have 1 row, name updated
	readR = m.Read(ctx, orm.ReadIntent{Schema: schema})
	payload = readR.Value.(*orm.Payload)
	rows := payload.Data.([]map[string]any)
	AssertEqual(t, 1, len(rows))
	AssertEqual(t, "updated", rows[0]["name"])
}

func TestMemium_WriteDelete_Good(t *T) {
	m := orm.NewMemium()
	ctx := Background()
	schema := testSchema("users")

	row := map[string]any{"id": int64(1), "name": "snider"}
	m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{row}})

	// Delete
	r := m.Write(ctx, orm.WriteIntent{
		Op:     orm.OpDelete,
		Schema: schema,
		Rows:   []any{row},
	})
	AssertTrue(t, r.OK)

	// Read should be empty
	readR := m.Read(ctx, orm.ReadIntent{Schema: schema})
	payload := readR.Value.(*orm.Payload)
	AssertEqual(t, 0, len(payload.Data.([]map[string]any)))
}

func TestMemium_FilterEquals_Good(t *T) {
	m := orm.NewMemium()
	ctx := Background()
	schema := testSchema("users")

	m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"id": int64(1), "name": "alice"},
	}})
	m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"id": int64(2), "name": "bob"},
	}})

	intent := orm.ReadIntent{
		Schema: schema,
		Where:  []orm.Predicate{{Field: "name", Op: "=", Value: "alice"}},
	}
	r := m.Read(ctx, intent)
	payload := r.Value.(*orm.Payload)
	rows := payload.Data.([]map[string]any)
	AssertEqual(t, 1, len(rows))
	AssertEqual(t, "alice", rows[0]["name"])
}

func TestMemium_FilterComparison_Good(t *T) {
	m := orm.NewMemium()
	ctx := Background()
	schema := testSchema("users")

	m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"id": int64(5), "name": "five"},
	}})
	m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"id": int64(15), "name": "fifteen"},
	}})

	intent := orm.ReadIntent{
		Schema: schema,
		Where:  []orm.Predicate{{Field: "id", Op: ">", Value: int64(10)}},
	}
	r := m.Read(ctx, intent)
	payload := r.Value.(*orm.Payload)
	rows := payload.Data.([]map[string]any)
	AssertEqual(t, 1, len(rows))
}

func TestMemium_Order_Good(t *T) {
	m := orm.NewMemium()
	ctx := Background()
	schema := testSchema("users")

	m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"id": int64(3), "name": "z"},
	}})
	m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"id": int64(1), "name": "a"},
	}})

	intent := orm.ReadIntent{
		Schema: schema,
		Order:  []orm.OrderBy{{Field: "name", Desc: false}},
	}
	r := m.Read(ctx, intent)
	payload := r.Value.(*orm.Payload)
	rows := payload.Data.([]map[string]any)
	AssertEqual(t, "a", rows[0]["name"])
	AssertEqual(t, "z", rows[1]["name"])
}

func TestMemium_LimitOffset_Good(t *T) {
	m := orm.NewMemium()
	ctx := Background()
	schema := testSchema("users")

	for i := int64(1); i <= 5; i++ {
		m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
			map[string]any{"id": i, "name": "user" + Itoa(int(i))},
		}})
	}

	intent := orm.ReadIntent{
		Schema: schema,
		Offset: 2,
		Limit:  2,
	}
	r := m.Read(ctx, intent)
	payload := r.Value.(*orm.Payload)
	rows := payload.Data.([]map[string]any)
	AssertEqual(t, 2, len(rows))
}

func TestMemium_MaskCaps_Good(t *T) {
	m := orm.NewMemium()
	mask := orm.Capabilities{
		Predicates: orm.PredicateCaps{Equality: true},
		Joins:      false,
	}
	m.MaskCaps(mask)

	caps := m.Caps()
	AssertTrue(t, caps.Predicates.Equality)
	AssertTrue(t, !caps.Predicates.Comparison)
	AssertTrue(t, !caps.Joins)
}

func TestMemium_Stream_Good(t *T) {
	m := orm.NewMemium()
	ctx := Background()
	schema := testSchema("users")

	m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"id": int64(1), "name": "a"},
	}})

	intent := orm.ReadIntent{Schema: schema}
	r := m.Stream(ctx, intent)
	AssertTrue(t, r.OK)
}

func TestMemium_Search_Good(t *T) {
	m := orm.NewMemium()
	ctx := Background()
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("docs")
		b.PK("id")
		b.Int64("id")
		b.String("title").Searchable("text")
	})

	m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"id": int64(1), "title": "Eloquent Monks"},
	}})
	m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"id": int64(2), "title": "Chinese Farmer"},
	}})

	searchIntent := orm.ReadIntent{
		Schema: schema,
		Search: &orm.SearchSpec{Query: "monks", Limit: 10},
	}
	r := m.Search(ctx, searchIntent)
	AssertTrue(t, r.OK)
	payload := r.Value.(*orm.Payload)
	results := payload.Data.([]orm.RankedResult)
	AssertTrue(t, len(results) > 0)
}

func TestMemium_InsertAutoID_Ugly(t *T) {
	m := orm.NewMemium()
	ctx := Background()
	schema := testSchema("users")

	// Insert without ID — Memium should auto-assign
	row := map[string]any{"name": "auto"}
	r := m.Write(ctx, orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{row}})
	AssertTrue(t, r.OK)

	readR := m.Read(ctx, orm.ReadIntent{Schema: schema})
	payload := readR.Value.(*orm.Payload)
	rows := payload.Data.([]map[string]any)
	AssertEqual(t, 1, len(rows))
	AssertTrue(t, rows[0]["id"] != nil)
}
