// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

const resultMetaLimit = 1024

var resultMetas = &resultMetaCache{items: make(map[string]Meta)}

type resultMetaCache struct {
	mu    core.Mutex
	items map[string]Meta
	order []string
}

func (c *resultMetaCache) Store(key string, meta Meta) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[key]; !exists {
		c.order = append(c.order, key)
	}
	c.items[key] = meta
	for len(c.order) > resultMetaLimit {
		oldest := c.order[0]
		copy(c.order, c.order[1:])
		c.order = c.order[:len(c.order)-1]
		delete(c.items, oldest)
	}
}

func (c *resultMetaCache) Load(key string) (Meta, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	meta, ok := c.items[key]
	return meta, ok
}

// Cast unwraps ORM Payload values before applying core.Cast.
func Cast[T any](r core.Result) (T, bool) {
	var zero T
	if !r.OK {
		return zero, false
	}
	if payload, ok := r.Value.(*Payload); ok {
		val, ok := payload.Data.(T)
		if ok {
			return val, true
		}
		return zero, false
	}
	return core.Cast[T](r)
}

// MustCast unwraps ORM Payload values and panics on mismatch.
func MustCast[T any](r core.Result) T {
	v, ok := Cast[T](r)
	if !ok {
		panic("orm: cast failed")
	}
	return v
}

// Detail extracts both data and meta from a Result produced by the bridge.
//
//	user, meta, ok := orm.Detail[User](res)
func Detail[T any](r core.Result) (T, Meta, bool) {
	var zero T
	if !r.OK {
		return zero, Meta{}, false
	}
	// Try Payload wrapper first
	if payload, ok := r.Value.(*Payload); ok {
		val, ok := payload.Data.(T)
		if !ok {
			return zero, Meta{}, false
		}
		return val, payload.Meta, true
	}
	// Try direct cast (for raw values from bridge)
	val, ok := Cast[T](r)
	if !ok {
		return zero, Meta{}, false
	}
	if meta, ok := lookupResultMeta(val); ok {
		return val, meta, true
	}
	return val, Meta{}, true
}

func withResultMeta(v any, meta Meta) core.Result {
	if key := resultMetaKey(v); key != "" {
		resultMetas.Store(key, meta)
	}
	return core.Ok(v)
}

func lookupResultMeta(v any) (Meta, bool) {
	key := resultMetaKey(v)
	if key == "" {
		return Meta{}, false
	}
	meta, ok := resultMetas.Load(key)
	if !ok {
		return Meta{}, false
	}
	return meta, true
}

func resultMetaKey(v any) string {
	if v == nil {
		return ""
	}
	value := core.ValueOf(v)
	for value.IsValid() && (value.Kind() == core.KindPointer || value.Kind() == core.KindInterface) {
		if value.IsNil() {
			return ""
		}
		value = value.Elem()
	}
	if value.IsValid() {
		v = value.Interface()
	}
	if data := core.JSONMarshal(v); data.OK {
		return core.Sprintf("%T:%s", v, string(data.Value.([]byte)))
	}
	return core.Sprintf("%T:%#v", v, v)
}
