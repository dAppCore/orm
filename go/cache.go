// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

// schemaCaches holds per-Core schema caches — each Core instance
// gets its own Registry[Schema], populated lazily on first access.
var schemaCaches = &core.SyncMap{}
var schemaCacheMu core.Mutex

// cacheGet returns the cached Schema for T from the Core's schema cache,
// invoking Schema() on a zero-value of T and caching the result on first access.
func cacheGet[T any](c *core.Core) Schema {
	cache := getOrCreateCache(c)
	typeKey := core.TypeOf((*T)(nil)).Elem().String()

	r := cache.Get(typeKey)
	if r.OK {
		return r.Value.(Schema)
	}

	schemaCacheMu.Lock()
	defer schemaCacheMu.Unlock()

	r = cache.Get(typeKey)
	if r.OK {
		return r.Value.(Schema)
	}

	// First access — invoke Schema() on new zero value
	zero := core.Zero(core.TypeOf((*T)(nil)).Elem()).Interface()
	modelled, ok := zero.(Modelled)
	if !ok {
		cache.Set(typeKey, Schema{})
		return Schema{}
	}

	schema := modelled.Schema()
	cache.Set(typeKey, schema)
	if schema.Name != "" {
		cache.Set(schema.Name, schema)
	}
	return schema
}

// getOrCreateCache returns the per-Core schema cache, creating one if needed.
func getOrCreateCache(c *core.Core) *core.Registry[Schema] {
	if existing, ok := schemaCaches.Load(c); ok {
		return existing.(*core.Registry[Schema])
	}
	cache := core.NewRegistry[Schema]()
	// Use LoadOrStore for concurrent first-access
	actual, _ := schemaCaches.LoadOrStore(c, cache)
	return actual.(*core.Registry[Schema])
}

// RegisterSchema registers a schema by table name for dynamic/service-form use.
func RegisterSchema(c *core.Core, schema Schema) core.Result {
	if schema.Name == "" {
		return core.Fail(core.NewCode("orm.schema.invalid", "schema name is required"))
	}
	return getOrCreateCache(c).Set(schema.Name, schema)
}

func schemaByName(c *core.Core, table string) (Schema, core.Result) {
	r := getOrCreateCache(c).Get(table)
	if !r.OK {
		return Schema{}, core.Fail(core.NewCode("orm.schema.not_found", "schema not found: "+table))
	}
	return r.Value.(Schema), core.Ok(nil)
}
