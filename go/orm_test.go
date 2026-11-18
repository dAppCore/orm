// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	. "github.com/dappcore/go"
)

func TestOrm_Bootstrap_Smoke(t *T) {
	c := New()
	AssertTrue(t, c != nil)
}
