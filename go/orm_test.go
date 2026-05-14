// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	. "dappco.re/go"
)

func TestOrm_Bootstrap_Smoke(t *T) {
	c := New()
	AssertTrue(t, c != nil)
}
