// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

// Medium is the seam between the bridge and a backend.
//
//	var m Medium = &SQLMedium{db: db}
//	res := m.Read(ctx, intent)
type Medium interface {
	// Caps reports what this Medium can honour server-side.
	Caps() Capabilities

	// Read dispatches a ReadIntent and returns rows.
	Read(ctx core.Context, in ReadIntent) core.Result

	// Write dispatches a WriteIntent (Insert/Update/Save/Delete).
	Write(ctx core.Context, in WriteIntent) core.Result

	// Stream dispatches a ReadIntent and returns core.Seq[T].
	Stream(ctx core.Context, in ReadIntent) core.Result

	// Watch dispatches a ReadIntent with WatchOpts and returns core.Seq[Event].
	Watch(ctx core.Context, in ReadIntent) core.Result

	// Search dispatches a ReadIntent with SearchSpec and returns []RankedResult.
	Search(ctx core.Context, in ReadIntent) core.Result
}

// Capabilities declares what a Medium can honour server-side.
type Capabilities struct {
	Predicates   PredicateCaps `json:"predicates"`
	Joins        bool          `json:"joins"`
	Transactions bool          `json:"transactions"`
	Aggregates   bool          `json:"aggregates"`
	Cursor       bool          `json:"cursor"`
	Introspect   bool          `json:"introspect"`
	Watch        bool          `json:"watch"`
	WatchPoll    bool          `json:"watchPoll"`
	Search       SearchCaps    `json:"search"`
	Aliases      bool          `json:"aliases"`
	Subqueries   bool          `json:"subqueries"`
	SetOps       bool          `json:"setOps"`
	JoinKinds    JoinCaps      `json:"joinKinds"`
}

// PredicateCaps declares which operators the Medium honours server-side.
type PredicateCaps struct {
	Equality   bool `json:"equality"`   // = !=
	Comparison bool `json:"comparison"` // < <= > >=
	In         bool `json:"in"`         // IN / NOT IN
	Like       bool `json:"like"`       // LIKE / NOT LIKE
	Null       bool `json:"null"`       // IS NULL / IS NOT NULL
	Between    bool `json:"between"`    // BETWEEN x AND y
}

// SearchCaps declares search capabilities for the Search verb.
type SearchCaps struct {
	Text   bool `json:"text"`   // text search (full-text indexing)
	Vector bool `json:"vector"` // vector search (embedding similarity)
	Hybrid bool `json:"hybrid"` // hybrid (text + vector reranking)
	Facets bool `json:"facets"` // facet aggregation
}

// JoinCaps declares which join kinds the Medium supports.
type JoinCaps struct {
	Inner bool `json:"inner"`
	Left  bool `json:"left"`
	Right bool `json:"right"`
	Full  bool `json:"full"`
}

// FullCaps returns default maximum capabilities — used by Memium and
// full-featured SQL Mediums.
func FullCaps() Capabilities {
	return Capabilities{
		Predicates: PredicateCaps{
			Equality:   true,
			Comparison: true,
			In:         true,
			Like:       true,
			Null:       true,
			Between:    true,
		},
		Joins:        true,
		Transactions: true,
		Aggregates:   true,
		Cursor:       true,
		Introspect:   true,
		Watch:        true,
		WatchPoll:    true,
		Search: SearchCaps{
			Text:   true,
			Vector: true,
			Hybrid: true,
			Facets: true,
		},
		Aliases:    true,
		Subqueries: true,
		SetOps:     true,
		JoinKinds: JoinCaps{
			Inner: true,
			Left:  true,
			Right: true,
			Full:  true,
		},
	}
}

// Meta carries dispatch metadata alongside a result.
type Meta struct {
	Medium   string        `json:"medium"`
	Duration core.Duration `json:"duration"`
	RowsRead int64         `json:"rowsRead,omitempty"`
	RowsWrit int64         `json:"rowsWrit,omitempty"`
	Degraded string        `json:"degraded,omitempty"`
	LastID   any           `json:"lastID,omitempty"`
}

// Payload wraps a result value with metadata for internal dispatch.
type Payload struct {
	Data any
	Meta Meta
}

// mediumRegistries holds per-Core Medium registries.
var mediumRegistries = &core.SyncMap{}

// Mount registers a Medium by name in the Core's Medium registry.
//
//	mySQL := orm_sql.New(c, "duckdb", "file:./data.db")
//	orm.Mount(c, "default", mySQL)
func Mount(c *core.Core, name string, m Medium) core.Result {
	reg := getOrCreateMediumReg(c)
	return reg.Set(name, m)
}

// resolve looks up a Medium by name from the Core's registry.
// Returns orm.medium.notmounted when the name is not registered.
func resolve(c *core.Core, name string) (Medium, core.Result) {
	reg := getOrCreateMediumReg(c)
	r := reg.Get(name)
	if !r.OK {
		return nil, core.Fail(core.NewCode("orm.medium.notmounted", "medium not mounted: "+name))
	}
	return r.Value.(Medium), core.Ok(nil)
}

// getOrCreateMediumReg returns the per-Core Medium registry, creating one if needed.
func getOrCreateMediumReg(c *core.Core) *core.Registry[Medium] {
	if existing, ok := mediumRegistries.Load(c); ok {
		return existing.(*core.Registry[Medium])
	}
	cache := core.NewRegistry[Medium]()
	actual, _ := mediumRegistries.LoadOrStore(c, cache)
	return actual.(*core.Registry[Medium])
}

// Remove cleans up the per-Core registries for the given Core instance.
// Call in test teardown to prevent test pollution when Core addresses are reused.
func Remove(c *core.Core) {
	schemaCaches.Delete(c)
	mediumRegistries.Delete(c)
}
