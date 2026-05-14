// SPDX-License-Identifier: EUPL-1.2

package orm_test

import "dappco.re/go/orm"

type ExtUser struct {
	ID    int64
	Name  string
	Email string
	Age   int64
}

func (ExtUser) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("ext_users")
		b.PK("id")
		b.Int64("id")
		b.String("name")
		b.String("email")
		b.Int64("age")
	})
}
