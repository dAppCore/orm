// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

// Watch starts a change-data-capture subscription scoped to the bridge's
// current predicate state. Returns core.Seq[Event[T]].
//
//	seq, ok := core.Cast[core.Seq[Event[User]]](orm.Of[User](c).Watch(ctx))
func (b *Bridge[T]) Watch(ctx core.Context) core.Result {
	schema := b.schema()
	if schema.Name == "" {
		return core.Fail(core.NewCode("orm.schema.missing", "type does not implement Modelled"))
	}
	b.readIntent.Schema = schema
	if b.readIntent.Watch == nil {
		b.readIntent.Watch = &WatchOpts{}
	}
	medium, mr := b.resolveMedium(b.medium())
	if !mr.OK {
		return mr
	}
	caps := medium.Caps()
	if !caps.Watch && !caps.WatchPoll {
		return core.Fail(core.NewCode("orm.watch.unsupported", "medium does not support watch"))
	}
	if b.readIntent.Watch.PollInterval == 0 && !caps.Watch {
		return core.Fail(core.NewCode("orm.watch.unsupported", "native watch required but unsupported"))
	}
	r := medium.Watch(ctx, b.readIntent)
	if !r.OK {
		return r
	}
	payload, ok := r.Value.(*Payload)
	if !ok {
		return r
	}
	rawSeq, ok := payload.Data.(func(func(Event[map[string]any]) bool))
	if !ok {
		return r
	}
	seq := func(yield func(Event[T]) bool) {
		rawSeq(func(raw Event[map[string]any]) bool {
			var before T
			var after T
			if raw.Before != nil {
				if rr := populateStructFromRow(&before, schema, raw.Before); !rr.OK {
					return false
				}
			}
			if raw.After != nil {
				if rr := populateStructFromRow(&after, schema, raw.After); !rr.OK {
					return false
				}
			}
			return yield(Event[T]{
				Op:     raw.Op,
				Before: before,
				After:  after,
				Time:   raw.Time,
				Source: raw.Source,
			})
		})
	}
	payload.Data = core.Seq[Event[T]](seq)
	return core.Ok(payload)
}

// Live suppresses the initial snapshot — only live events emitted.
func (b *Bridge[T]) Live() *Bridge[T] {
	if b.readIntent.Watch == nil {
		b.readIntent.Watch = &WatchOpts{}
	}
	b.readIntent.Watch.Live = true
	return b
}

// WatchPoll sets the polling interval for Watch fallback.
// Pass 0 to require native CDC.
func (b *Bridge[T]) WatchPoll(d core.Duration) *Bridge[T] {
	if b.readIntent.Watch == nil {
		b.readIntent.Watch = &WatchOpts{}
	}
	b.readIntent.Watch.PollInterval = d
	return b
}
