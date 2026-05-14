// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	. "dappco.re/go"
	"dappco.re/go/orm"
)

type CacheUser struct {
	ID int64
}

func (CacheUser) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("cache_users")
		b.PK("id")
		b.Int64("id")
	})
}

func TestSchemaCache_Modelled_Good(t *T) {
	schema := CacheUser{}.Schema()
	AssertEqual(t, "cache_users", schema.Name)
	AssertEqual(t, 1, len(schema.PK))
	AssertEqual(t, "id", schema.PK[0])
}

func TestSchemaCache_NewCore_Ugly(t *T) {
	c1 := New()
	c2 := New()
	AssertTrue(t, c1 != c2)
}
