// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

type SearchableDoc struct {
	ID    int64
	Title string
	Body  string
}

func (SearchableDoc) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("docs")
		b.PK("id")
		b.String("title").Searchable("text")
		b.String("body").Searchable("text")
	})
}

func TestSearch_Text_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	s := SearchableDoc{}.Schema()
	mem.RegisterTable("docs", s)
	orm.RegisterSchema(c, s)
	mem.Insert("docs", map[string]any{
		"id": int64(1), "title": "Hello World", "body": "This is a test document",
	})
	mem.Insert("docs", map[string]any{
		"id": int64(2), "title": "Goodbye Moon", "body": "Nothing here",
	})

	res := orm.Of[SearchableDoc](c).Search("hello", orm.SearchOpts{Limit: 10})
	core.AssertTrue(t, res.OK)

	results, ok := orm.Cast[[]orm.Ranked[SearchableDoc]](res)
	core.AssertTrue(t, ok)
	core.AssertNotEmpty(t, results)
}

func TestSearch_NoMatch_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	s := SearchableDoc{}.Schema()
	mem.RegisterTable("docs", s)
	orm.RegisterSchema(c, s)
	mem.Insert("docs", map[string]any{
		"id": int64(1), "title": "Alpha", "body": "Beta",
	})

	res := orm.Of[SearchableDoc](c).Search("zzz", orm.SearchOpts{Limit: 10})
	core.AssertTrue(t, res.OK)

	results, ok := orm.Cast[[]orm.Ranked[SearchableDoc]](res)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, 0, len(results))
}

func TestSearch_Unsupported_Bad(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	mem.MaskCaps(orm.Capabilities{
		Search: orm.SearchCaps{Text: false},
	})
	orm.Mount(c, "default", mem)
	s := SearchableDoc{}.Schema()
	mem.RegisterTable("docs", s)
	orm.RegisterSchema(c, s)

	res := orm.Of[SearchableDoc](c).Search("test", orm.SearchOpts{Limit: 10})
	core.AssertFalse(t, res.OK)
	core.AssertEqual(t, "orm.search.unsupported", res.Code())
}
