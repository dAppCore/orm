// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	. "dappco.re/go"
	"dappco.re/go/orm"
)

// testMedium is a minimal Medium implementation for testing the contract.
type testMedium struct{}

func (testMedium) Caps() orm.Capabilities { return orm.FullCaps() }
func (testMedium) Read(ctx Context, in orm.ReadIntent) Result {
	return Ok("read")
}
func (testMedium) Write(ctx Context, in orm.WriteIntent) Result {
	return Ok("write")
}
func (testMedium) Stream(ctx Context, in orm.ReadIntent) Result {
	return Ok("stream")
}
func (testMedium) Watch(ctx Context, in orm.ReadIntent) Result {
	return Ok("watch")
}
func (testMedium) Search(ctx Context, in orm.ReadIntent) Result {
	return Ok("search")
}

func TestMedium_Mount_Good(t *T) {
	c := New()
	m := testMedium{}
	r := orm.Mount(c, "default", m)
	AssertTrue(t, r.OK)
}

func TestMedium_MountTwice_Ugly(t *T) {
	c := New()
	m := testMedium{}
	orm.Mount(c, "default", m)
	// Mounting same name twice should still work (Registry allows overwrite)
	r := orm.Mount(c, "default", m)
	AssertTrue(t, r.OK)
}

func TestMedium_FullCaps_Good(t *T) {
	caps := orm.FullCaps()
	AssertTrue(t, caps.Predicates.Equality)
	AssertTrue(t, caps.Predicates.Comparison)
	AssertTrue(t, caps.Predicates.In)
	AssertTrue(t, caps.Predicates.Like)
	AssertTrue(t, caps.Predicates.Null)
	AssertTrue(t, caps.Predicates.Between)
	AssertTrue(t, caps.Joins)
	AssertTrue(t, caps.Transactions)
	AssertTrue(t, caps.Aggregates)
	AssertTrue(t, caps.Cursor)
	AssertTrue(t, caps.Introspect)
	AssertTrue(t, caps.Watch)
	AssertTrue(t, caps.WatchPoll)
	AssertTrue(t, caps.Search.Text)
	AssertTrue(t, caps.Search.Vector)
	AssertTrue(t, caps.Search.Hybrid)
	AssertTrue(t, caps.Search.Facets)
	AssertTrue(t, caps.Aliases)
	AssertTrue(t, caps.Subqueries)
	AssertTrue(t, caps.SetOps)
	AssertTrue(t, caps.JoinKinds.Inner)
	AssertTrue(t, caps.JoinKinds.Left)
	AssertTrue(t, caps.JoinKinds.Right)
	AssertTrue(t, caps.JoinKinds.Full)
}

func TestMedium_Payload_Good(t *T) {
	meta := orm.Meta{
		Medium:   "memium",
		Duration: 5 * Millisecond,
		RowsRead: 42,
	}

	r := JSONMarshal(meta)
	AssertTrue(t, r.OK)

	var restored orm.Meta
	unR := JSONUnmarshal(r.Value.([]byte), &restored)
	AssertTrue(t, unR.OK)
	AssertEqual(t, "memium", restored.Medium)
	AssertEqual(t, int64(42), restored.RowsRead)
}
