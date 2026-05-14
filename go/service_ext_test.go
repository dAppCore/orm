// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

func TestService_Register_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	s := ExtUser{}.Schema()
	mem.RegisterTable("ext_users", s)
	mem.Insert("ext_users", map[string]any{
		"id": int64(1), "name": "Snider", "email": "snider@host.uk", "age": int64(42),
	})

	orm.RegisterSchema(c, s)
	r := orm.Register(c)
	core.AssertTrue(t, r.OK)

	// Verify action registered
	action := c.Action("orm.find")
	core.AssertTrue(t, action.Exists())
}

func TestService_Find_Action_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	s := ExtUser{}.Schema()
	mem.RegisterTable("ext_users", s)
	orm.RegisterSchema(c, s)
	mem.Insert("ext_users", map[string]any{
		"id": int64(1), "name": "Test", "email": "t@host.uk", "age": int64(30),
	})

	orm.Register(c)

	r := c.Action("orm.find").Run(core.Background(), core.NewOptions(
		core.Option{Key: "table", Value: "ext_users"},
		core.Option{Key: "pk", Value: []any{int64(1)}},
	))
	core.AssertTrue(t, r.OK)
}

func TestService_Count_Action_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	s := ExtUser{}.Schema()
	mem.RegisterTable("ext_users", s)
	orm.RegisterSchema(c, s)
	mem.Insert("ext_users", map[string]any{
		"id": int64(1), "name": "A", "email": "a@h", "age": int64(1),
	})
	mem.Insert("ext_users", map[string]any{
		"id": int64(2), "name": "B", "email": "b@h", "age": int64(2),
	})

	orm.Register(c)

	r := c.Action("orm.count").Run(core.Background(), core.NewOptions(
		core.Option{Key: "table", Value: "ext_users"},
	))
	core.AssertTrue(t, r.OK)
}

func TestService_Save_Action_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	s := ExtUser{}.Schema()
	mem.RegisterTable("ext_users", s)
	orm.RegisterSchema(c, s)

	orm.Register(c)

	r := c.Action("orm.save").Run(core.Background(), core.NewOptions(
		core.Option{Key: "table", Value: "ext_users"},
		core.Option{Key: "rows", Value: []any{&ExtUser{ID: 1, Name: "X", Email: "x@h", Age: int64(5)}}},
	))
	core.AssertTrue(t, r.OK)
}
