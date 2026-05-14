// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

// WriteOp is the operation type for WriteIntent.
type WriteOp int

const (
	OpInsert WriteOp = iota
	OpUpdate
	OpSave
	OpDelete
)

// SetOpKind is the kind of set operation (UNION, INTERSECT, EXCEPT).
type SetOpKind int

const (
	SetUnion SetOpKind = iota
	SetIntersect
	SetExcept
)

// WatchOpts configures a reactive Watch subscription.
type WatchOpts struct {
	PollInterval core.Duration // 0 = use Medium's native CDC; non-zero = poll-fallback
	Live         bool          // suppress initial snapshot
}

// SearchSpec describes a search query for the Search verb.
type SearchSpec struct {
	Query     string
	Mode      string // "text" | "vector" | "hybrid"
	Vector    []float32
	Limit     int
	Offset    int
	Highlight bool
	Facets    []string
}

// SetOp describes a set operation (UNION, INTERSECT, EXCEPT) combining multiple builders.
type SetOp struct {
	Kind     SetOpKind    `json:"kind"`
	Builders []ReadIntent `json:"builders"`
}

// ReadIntent is the typed payload sent to Medium.Read / Stream.
//
//	intent := ReadIntent{Schema: s, PK: []any{1}}
type ReadIntent struct {
	Schema    Schema         `json:"schema"`
	PK        []any          `json:"pk,omitempty"`
	Where     []Predicate    `json:"where,omitempty"`
	With      []string       `json:"with,omitempty"`
	Order     []OrderBy      `json:"order,omitempty"`
	Limit     int            `json:"limit,omitempty"`
	Offset    int            `json:"offset,omitempty"`
	Aggregate *AggregateOp   `json:"aggregate,omitempty"`
	Tx        *core.Tx       `json:"-"`
	Alias     map[string]any `json:"alias,omitempty"`
	SetOp     *SetOp         `json:"setOp,omitempty"`
	Watch     *WatchOpts     `json:"watch,omitempty"`
	Search    *SearchSpec    `json:"search,omitempty"`
	Strict    bool           `json:"strict,omitempty"`
}

// WriteIntent is the typed payload sent to Medium.Write.
//
//	res := m.Write(ctx, WriteIntent{Op: Save, Rows: []any{&user}})
type WriteIntent struct {
	Op      WriteOp        `json:"op"`
	Schema  Schema         `json:"schema"`
	Rows    []any          `json:"rows"`
	Where   []Predicate    `json:"where,omitempty"`
	Updates map[string]any `json:"updates,omitempty"`
	Tx      *core.Tx       `json:"-"`
}

// Predicate is a single filter condition. When Group is non-empty, the
// group forms a parenthesised OR block.
type Predicate struct {
	Field string      `json:"field"`
	Op    string      `json:"op"` // "=", "<", "in", "like", ...
	Value any         `json:"value,omitempty"`
	Group []Predicate `json:"group,omitempty"`
	OR    bool        `json:"or,omitempty"` // join with previous via OR (default AND)
}
