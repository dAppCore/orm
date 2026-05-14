// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

func TestDDL_PrimaryKeyComma_Good(t *core.T) {
	s := orm.Define(func(b *orm.Builder) {
		b.Name("ddl_users")
		b.PK("id")
		b.Int64("id")
		b.String("name")
	})

	res := orm.DDL(core.New(), s, "sqlite")
	core.AssertTrue(t, res.OK)
	ddl := res.Value.(string)
	core.AssertTrue(t, core.Contains(ddl, "name TEXT,\n  PRIMARY KEY"))
}
