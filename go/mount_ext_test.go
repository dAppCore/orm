// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

func TestOfTable_Find_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	s := ExtUser{}.Schema()
	mem.RegisterTable("ext_users", s)
	orm.RegisterSchema(c, s)
	mem.Insert("ext_users", map[string]any{
		"id": int64(1), "name": "Snider", "email": "snider@h", "age": int64(42),
	})

	res := orm.OfTable(c, "ext_users").Find(int64(1))
	core.AssertTrue(t, res.OK)

	row, ok := core.Cast[map[string]any](res)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, "Snider", row["name"])
}

func TestOfTable_Get_Good(t *core.T) {
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

	res := orm.OfTable(c, "ext_users").Get()
	core.AssertTrue(t, res.OK)

	rows, ok := core.Cast[[]map[string]any](res)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, 2, len(rows))
}

func TestOfTable_Find_Bad(t *core.T) {
	c := core.New()
	res := orm.OfTable(c, "nonexistent").Find(int64(1))
	core.AssertFalse(t, res.OK)
	core.AssertEqual(t, "orm.schema.not_found", res.Code())
}

func TestOfTable_Where_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	s := ExtUser{}.Schema()
	mem.RegisterTable("ext_users", s)
	orm.RegisterSchema(c, s)
	mem.Insert("ext_users", map[string]any{
		"id": int64(1), "name": "Target", "email": "t@h", "age": int64(10),
	})
	mem.Insert("ext_users", map[string]any{
		"id": int64(2), "name": "Other", "email": "o@h", "age": int64(20),
	})

	res := orm.OfTable(c, "ext_users").Where("name", "=", "Target").Get()
	core.AssertTrue(t, res.OK)

	rows, ok := core.Cast[[]map[string]any](res)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, 1, len(rows))
	core.AssertEqual(t, "Target", rows[0]["name"])
}
