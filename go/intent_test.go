// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	. "dappco.re/go"
	"dappco.re/go/orm"
)

func TestIntent_ReadIntent_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("users")
		b.PK("id")
		b.String("email")
	})

	intent := orm.ReadIntent{
		Schema: schema,
		PK:     []any{42},
		Where: []orm.Predicate{
			{Field: "email", Op: "=", Value: "x@y"},
		},
		With:  []string{"posts"},
		Order: []orm.OrderBy{{Field: "created_at", Desc: true}},
		Limit: 10,
	}

	r := JSONMarshal(intent)
	AssertTrue(t, r.OK)

	var restored orm.ReadIntent
	unR := JSONUnmarshal(r.Value.([]byte), &restored)
	AssertTrue(t, unR.OK)

	AssertEqual(t, schema.Name, restored.Schema.Name)
	AssertEqual(t, 1, len(restored.Where))
	AssertEqual(t, "email", restored.Where[0].Field)
	AssertEqual(t, "=", restored.Where[0].Op)
}

func TestIntent_WriteIntent_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("users")
		b.PK("id")
		b.String("email")
	})

	intent := orm.WriteIntent{
		Op:     orm.OpSave,
		Schema: schema,
		Rows:   []any{},
		Where:  []orm.Predicate{{Field: "id", Op: "=", Value: 1}},
	}

	r := JSONMarshal(intent)
	AssertTrue(t, r.OK)
	AssertTrue(t, len(r.Value.([]byte)) > 0)

	var restored orm.WriteIntent
	unR := JSONUnmarshal(r.Value.([]byte), &restored)
	AssertTrue(t, unR.OK)
	AssertEqual(t, orm.OpSave, restored.Op)
}

func TestIntent_PredicateGroup_Good(t *T) {
	pred := orm.Predicate{
		Field: "active",
		Op:    "=",
		Value: true,
		Group: []orm.Predicate{
			{Field: "tier", Op: "=", Value: "pro", OR: true},
			{Field: "admin", Op: "=", Value: true},
		},
	}

	r := JSONMarshal(pred)
	AssertTrue(t, r.OK)

	var restored orm.Predicate
	unR := JSONUnmarshal(r.Value.([]byte), &restored)
	AssertTrue(t, unR.OK)

	AssertEqual(t, "active", restored.Field)
	AssertEqual(t, 2, len(restored.Group))
	AssertEqual(t, "tier", restored.Group[0].Field)
	AssertTrue(t, restored.Group[0].OR)
}

func TestIntent_WriteOps_Good(t *T) {
	AssertEqual(t, 0, int(orm.OpInsert))
	AssertEqual(t, 1, int(orm.OpUpdate))
	AssertEqual(t, 2, int(orm.OpSave))
	AssertEqual(t, 3, int(orm.OpDelete))
}

func TestIntent_SetOps_Good(t *T) {
	AssertEqual(t, 0, int(orm.SetUnion))
	AssertEqual(t, 1, int(orm.SetIntersect))
	AssertEqual(t, 2, int(orm.SetExcept))
}

func TestIntent_SearchSpec_Good(t *T) {
	spec := orm.SearchSpec{
		Query:     "eloquent monks",
		Mode:      "text",
		Limit:     20,
		Highlight: true,
	}

	r := JSONMarshal(spec)
	AssertTrue(t, r.OK)

	var restored orm.SearchSpec
	unR := JSONUnmarshal(r.Value.([]byte), &restored)
	AssertTrue(t, unR.OK)
	AssertEqual(t, "eloquent monks", restored.Query)
	AssertEqual(t, "text", restored.Mode)
}

func TestIntent_WatchOpts_Good(t *T) {
	opts := orm.WatchOpts{
		PollInterval: 5 * Second,
		Live:         true,
	}

	r := JSONMarshal(opts)
	AssertTrue(t, r.OK)

	var restored orm.WatchOpts
	unR := JSONUnmarshal(r.Value.([]byte), &restored)
	AssertTrue(t, unR.OK)
	AssertTrue(t, restored.Live)
}

func TestIntent_SetOp_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("users")
		b.PK("id")
		b.String("email")
	})

	setOp := orm.SetOp{
		Kind: orm.SetUnion,
		Builders: []orm.ReadIntent{
			{Schema: schema, Where: []orm.Predicate{{Field: "active", Op: "=", Value: true}}},
			{Schema: schema, Where: []orm.Predicate{{Field: "admin", Op: "=", Value: true}}},
		},
	}

	r := JSONMarshal(setOp)
	AssertTrue(t, r.OK)

	var restored orm.SetOp
	unR := JSONUnmarshal(r.Value.([]byte), &restored)
	AssertTrue(t, unR.OK)
	AssertEqual(t, orm.SetUnion, restored.Kind)
	AssertEqual(t, 2, len(restored.Builders))
}

func TestIntent_ReadIntentEmptyPK_Ugly(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("users")
		b.PK("id")
	})

	intent := orm.ReadIntent{
		Schema: schema,
		PK:     nil,
	}

	r := JSONMarshal(intent)
	AssertTrue(t, r.OK)

	var restored orm.ReadIntent
	unR := JSONUnmarshal(r.Value.([]byte), &restored)
	AssertTrue(t, unR.OK)
	AssertEqual(t, 0, len(restored.PK))
}

func TestIntent_WriteIntentMultipleRows_Ugly(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("users")
		b.PK("id")
	})

	intent := orm.WriteIntent{
		Op:     orm.OpSave,
		Schema: schema,
		Rows:   []any{"row1", "row2", "row3"},
	}

	r := JSONMarshal(intent)
	AssertTrue(t, r.OK)
	AssertTrue(t, len(r.Value.([]byte)) > 0)
}

func TestIntent_PredicateBetween_Ugly(t *T) {
	pred := orm.Predicate{
		Field: "age",
		Op:    "between",
		Value: []int64{18, 65},
	}

	r := JSONMarshal(pred)
	AssertTrue(t, r.OK)

	var restored orm.Predicate
	unR := JSONUnmarshal(r.Value.([]byte), &restored)
	AssertTrue(t, unR.OK)
	AssertEqual(t, "between", restored.Op)
}
