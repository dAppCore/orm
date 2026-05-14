// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

type AliasUser struct {
	ID   int64
	Name string
}

func (AliasUser) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("users")
		b.PK("id")
		b.String("name")
	})
}

type AliasPost struct {
	ID     int64
	UserID int64
	Title  string
}

func (AliasPost) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("posts")
		b.PK("id")
		b.Int64("user_id")
		b.String("title")
	})
}

func TestAlias_From_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	us := AliasUser{}.Schema()
	ps := AliasPost{}.Schema()
	mem.RegisterTable("users", us)
	mem.RegisterTable("posts", ps)
	orm.RegisterSchema(c, us)
	orm.RegisterSchema(c, ps)

	mem.Insert("users", map[string]any{"id": int64(1), "name": "Snider"})
	mem.Insert("posts", map[string]any{"id": int64(1), "user_id": int64(1), "title": "Hello"})

	res := orm.Of[AliasUser](c).Where("name", "=", "Snider").Get()
	core.AssertTrue(t, res.OK)
}

func TestAlias_From_Unsupported_Bad(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	mem.MaskCaps(orm.Capabilities{
		Aliases: false,
	})
	orm.Mount(c, "default", mem)

	s := AliasUser{}.Schema()
	mem.RegisterTable("users", s)
	orm.RegisterSchema(c, s)

	res := orm.Of[AliasUser](c).From(orm.A{"u": "users"}).Get()
	core.AssertFalse(t, res.OK)
	core.AssertEqual(t, "orm.aliases.unsupported", res.Code())
}

func TestAliasUser_Get_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	s := AliasUser{}.Schema()
	mem.RegisterTable("users", s)
	mem.Insert("users", map[string]any{"id": int64(1), "name": "Alice"})
	mem.Insert("users", map[string]any{"id": int64(2), "name": "Bob"})

	res := orm.Of[AliasUser](c).Get()
	core.AssertTrue(t, res.OK)
	_ = res
}

func TestAliasUser_StringWhere_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	us := AliasUser{}.Schema()
	ps := AliasPost{}.Schema()
	mem.RegisterTable("users", us)
	mem.RegisterTable("posts", ps)
	orm.RegisterSchema(c, us)
	orm.RegisterSchema(c, ps)

	mem.Insert("users", map[string]any{"id": int64(1), "name": "Snider"})
	mem.Insert("posts", map[string]any{"id": int64(1), "user_id": int64(1), "title": "Post"})

	res := orm.Of[AliasUser](c).Where("name", "=", "Snider").Get()
	core.AssertTrue(t, res.OK)
}
