// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

func setupExtWrite(t *core.T) (*core.Core, *orm.Memium) {
	c := core.New()
	mem := orm.NewMemium()
	core.AssertTrue(t, orm.Mount(c, "default", mem).OK)
	s := ExtUser{}.Schema()
	mem.RegisterTable("ext_users", s)
	core.AssertTrue(t, orm.RegisterSchema(c, s).OK)
	return c, mem
}

func TestBridge_Save_Good(t *core.T) {
	c, _ := setupExtWrite(t)

	res := orm.Of[ExtUser](c).Save(&ExtUser{ID: 1, Name: "Saved", Email: "saved@host.uk", Age: 7})
	core.AssertTrue(t, res.OK)

	found := orm.Of[ExtUser](c).Find(int64(1))
	core.AssertTrue(t, found.OK)
	user, ok := core.Cast[ExtUser](found)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, "Saved", user.Name)
}

func TestBridge_Update_Good(t *core.T) {
	c, mem := setupExtWrite(t)
	mem.Insert("ext_users", map[string]any{"id": int64(1), "name": "Old", "email": "old@host.uk", "age": int64(7)})

	res := orm.Of[ExtUser](c).Where("id", "=", int64(1)).Update(map[string]any{"name": "New"})
	core.AssertTrue(t, res.OK)

	found := orm.Of[ExtUser](c).Find(int64(1))
	user, ok := core.Cast[ExtUser](found)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, "New", user.Name)
}

func TestBridge_DeleteAll_Good(t *core.T) {
	c, mem := setupExtWrite(t)
	mem.Insert("ext_users", map[string]any{"id": int64(1), "name": "Gone", "email": "gone@host.uk", "age": int64(7)})

	res := orm.Of[ExtUser](c).Where("id", "=", int64(1)).DeleteAll()
	core.AssertTrue(t, res.OK)

	found := orm.Of[ExtUser](c).Find(int64(1))
	core.AssertFalse(t, found.OK)
	core.AssertEqual(t, "orm.notfound", found.Code())
}

func TestOrm_Save_Good(t *core.T) {
	c, _ := setupExtWrite(t)

	res := orm.Save(c, &ExtUser{ID: 2, Name: "Top", Email: "top@host.uk", Age: 8})
	core.AssertTrue(t, res.OK)

	found := orm.Of[ExtUser](c).Find(int64(2))
	user, ok := core.Cast[ExtUser](found)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, "Top", user.Name)
}

func TestBridge_On_Good(t *core.T) {
	c := core.New()
	primary := orm.NewMemium()
	archive := orm.NewMemium()
	core.AssertTrue(t, orm.Mount(c, "default", primary).OK)
	core.AssertTrue(t, orm.Mount(c, "archive", archive).OK)
	s := ExtUser{}.Schema()
	primary.RegisterTable("ext_users", s)
	archive.RegisterTable("ext_users", s)
	core.AssertTrue(t, orm.RegisterSchema(c, s).OK)
	archive.Insert("ext_users", map[string]any{"id": int64(9), "name": "Archived", "email": "arch@host.uk", "age": int64(1)})

	found := orm.Of[ExtUser](c).On("archive").Find(int64(9))
	core.AssertTrue(t, found.OK)
	user, ok := core.Cast[ExtUser](found)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, "Archived", user.Name)
}

type Unmodelled struct {
	ID int64
}

func TestBridge_Modelled_Bad(t *core.T) {
	c := core.New()
	core.AssertTrue(t, orm.Mount(c, "default", orm.NewMemium()).OK)

	res := orm.Of[Unmodelled](c).Find(int64(1))
	core.AssertFalse(t, res.OK)
	core.AssertEqual(t, "orm.schema.missing", res.Code())
}
