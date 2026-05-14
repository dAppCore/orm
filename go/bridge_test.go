// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	. "dappco.re/go"
	"dappco.re/go/orm"
)

type BridgeUser struct {
	ID        int64
	Email     string
	Name      string
	Active    bool
	CreatedAt Time
}

func (BridgeUser) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("bridge_users")
		b.PK("ID")
		b.Int64("ID")
		b.String("Email").Unique().NotNull().Format("email")
		b.String("Name").NotNull()
		b.Bool("Active").Default(true)
		b.Time("CreatedAt")
	})
}

func setupBridgeTest(t *T) (*Core, *orm.Memium) {
	c := New()
	m := orm.NewMemium()
	orm.Mount(c, "default", m)
	t.Cleanup(func() {
		orm.Remove(c)
	})
	return c, m
}

func TestBridge_Find_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()
	m.Write(c.Context(), orm.WriteIntent{
		Op:     orm.OpInsert,
		Schema: schema,
		Rows:   []any{map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "snider", "Active": true}},
	})

	res := orm.Of[BridgeUser](c).Find(int64(1))
	AssertTrue(t, res.OK)

	user, ok := Detail[BridgeUser](res)
	AssertTrue(t, ok)
	AssertEqual(t, "snider", user.Name)
}

func TestBridge_Find_Bad(t *T) {
	c, _ := setupBridgeTest(t)

	res := orm.Of[BridgeUser](c).Find(999)
	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.notfound", res.Code())
}

func TestBridge_Find_Ugly(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()

	// Insert with nil name to check rehydration
	m.Write(c.Context(), orm.WriteIntent{
		Op:     orm.OpInsert,
		Schema: schema,
		Rows:   []any{map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": nil, "Active": true}},
	})

	res := orm.Of[BridgeUser](c).Find(int64(1))
	// Should still succeed even with nil field
	AssertTrue(t, res.OK)
}

func TestBridge_Where_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()

	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "alice", "Active": true},
	}})
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(2), "Email": "c@d.com", "Name": "bob", "Active": false},
	}})

	// Use the bridge
	res := orm.Of[BridgeUser](c).Where("Active", "=", true).Get()
	AssertTrue(t, res.OK)
	users, ok := Cast[[]BridgeUser](res)
	AssertTrue(t, ok)
	AssertEqual(t, 1, len(users))
	AssertEqual(t, "alice", users[0].Name)
}

func TestBridge_Where_Bad(t *T) {
	c, _ := setupBridgeTest(t)

	res := orm.Of[BridgeUser](c).Where("Email", "=", "not-an-email").Get()
	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.input.format", res.Code())
}

func TestBridge_OrWhere_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()

	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "alice", "Active": false},
	}})
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(2), "Email": "c@d.com", "Name": "bob", "Active": false},
	}})

	res := orm.Of[BridgeUser](c).
		Where("Name", "=", "alice").
		OrWhere("Name", "=", "bob").
		Get()
	AssertTrue(t, res.OK)
	users, _ := Cast[[]BridgeUser](res)
	AssertEqual(t, 2, len(users))
}

func TestBridge_WhereNotNull_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()

	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "alice", "Active": true},
	}})
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(2), "Email": "c@d.com", "Name": nil, "Active": true},
	}})

	res := orm.Of[BridgeUser](c).WhereNotNull("Name").Get()
	AssertTrue(t, res.OK)
	users, _ := Cast[[]BridgeUser](res)
	AssertEqual(t, 1, len(users))
}

func TestBridge_WhereGroup_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()

	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "alice", "Active": true},
	}})
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(2), "Email": "c@d.com", "Name": "bob", "Active": false},
	}})

	res := orm.Of[BridgeUser](c).
		Where("Active", "=", true).
		WhereGroup(func(g *orm.Group) {
			g.Where("Name", "=", "alice")
			g.OrWhere("Name", "=", "bob")
		}).
		Get()
	AssertTrue(t, res.OK)
}

func TestBridge_Order_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()

	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(2), "Email": "b@b.com", "Name": "bob", "Active": true},
	}})
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@a.com", "Name": "alice", "Active": true},
	}})

	res := orm.Of[BridgeUser](c).Order("Name", "asc").Get()
	AssertTrue(t, res.OK)
	users, _ := Cast[[]BridgeUser](res)
	AssertEqual(t, 2, len(users))
	AssertEqual(t, "alice", users[0].Name)
}

func TestBridge_Count_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()

	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "alice", "Active": true},
	}})
	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(2), "Email": "b@b.com", "Name": "bob", "Active": true},
	}})

	res := orm.Of[BridgeUser](c).Count()
	AssertTrue(t, res.OK)
	n, ok := Cast[int64](res)
	AssertTrue(t, ok)
	AssertEqual(t, int64(2), n)
}

func TestBridge_All_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()

	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "alice", "Active": true},
	}})

	res := orm.Of[BridgeUser](c).All()
	AssertTrue(t, res.OK)
	users, _ := Cast[[]BridgeUser](res)
	AssertEqual(t, 1, len(users))
}

func TestBridge_Stream_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()

	m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
		map[string]any{"ID": int64(1), "Email": "a@b.com", "Name": "alice", "Active": true},
	}})

	res := orm.Of[BridgeUser](c).Stream()
	AssertTrue(t, res.OK)
}

func TestBridge_NoMedium_Bad(t *T) {
	c := New()
	res := orm.Of[BridgeUser](c).Find(1)
	AssertTrue(t, !res.OK)
	AssertEqual(t, "orm.medium.notmounted", res.Code())
}

func TestBridge_Limit_Good(t *T) {
	c, m := setupBridgeTest(t)
	schema := BridgeUser{}.Schema()

	for i := int64(1); i <= 5; i++ {
		m.Write(c.Context(), orm.WriteIntent{Op: orm.OpInsert, Schema: schema, Rows: []any{
			map[string]any{"ID": i, "Email": Sprintf("user%d@b.com", i), "Name": "user", "Active": true},
		}})
	}

	res := orm.Of[BridgeUser](c).Limit(2).Get()
	AssertTrue(t, res.OK)
	users, _ := Cast[[]BridgeUser](res)
	AssertEqual(t, 2, len(users))
}

// Helper to extract typed data from a bridge Result
func Detail[T any](r Result) (T, bool) {
	val, _, ok := orm.Detail[T](r)
	return val, ok
}
