// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

func TestWatch_InitialSnapshot_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	s := ExtUser{}.Schema()
	mem.RegisterTable("ext_users", s)
	orm.RegisterSchema(c, s)
	mem.Insert("ext_users", map[string]any{
		"id": int64(1), "name": "Snider", "email": "s@h", "age": int64(42),
	})

	ctx, cancel := core.WithCancel(core.Background())
	defer cancel()

	res := orm.Of[ExtUser](c).Watch(ctx)
	core.AssertTrue(t, res.OK)
}

func TestWatch_Live_NoSnapshot_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	s := ExtUser{}.Schema()
	mem.RegisterTable("ext_users", s)
	orm.RegisterSchema(c, s)

	ctx, cancel := core.WithCancel(core.Background())
	defer cancel()

	res := orm.Of[ExtUser](c).Live().Watch(ctx)
	core.AssertTrue(t, res.OK)
}

func TestWatch_Unsupported_Bad(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	mem.MaskCaps(orm.Capabilities{
		Watch:     false,
		WatchPoll: false,
	})
	orm.Mount(c, "default", mem)
	s := ExtUser{}.Schema()
	mem.RegisterTable("ext_users", s)
	orm.RegisterSchema(c, s)

	ctx, cancel := core.WithCancel(core.Background())
	defer cancel()

	res := orm.Of[ExtUser](c).Watch(ctx)
	core.AssertFalse(t, res.OK)
	core.AssertEqual(t, "orm.watch.unsupported", res.Code())
}

func TestWatch_Live_Insert_Good(t *core.T) {
	c := core.New()
	mem := orm.NewMemium()
	orm.Mount(c, "default", mem)
	s := ExtUser{}.Schema()
	mem.RegisterTable("ext_users", s)
	orm.RegisterSchema(c, s)

	ctx, cancel := core.WithTimeout(core.Background(), 500*core.Millisecond)
	defer cancel()

	res := orm.Of[ExtUser](c).Live().Watch(ctx)
	core.AssertTrue(t, res.OK)
	seq, ok := orm.Cast[core.Seq[orm.Event[ExtUser]]](res)
	core.AssertTrue(t, ok)
	if !ok {
		t.Fatal("watch result did not contain typed event sequence")
	}

	got := make(chan orm.Event[ExtUser], 1)
	go func() {
		seq(func(ev orm.Event[ExtUser]) bool {
			got <- ev
			return false
		})
	}()

	mem.Insert("ext_users", map[string]any{
		"id": int64(2), "name": "Live", "email": "live@host.uk", "age": int64(9),
	})

	select {
	case ev := <-got:
		core.AssertEqual(t, orm.WatchInsert, ev.Op)
		core.AssertEqual(t, "Live", ev.After.Name)
	case <-ctx.Done():
		t.Fatal("timed out waiting for live watch event")
	}
}
