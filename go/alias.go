// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

// A is the alias map type — declares which tables (or sub-references) participate.
type A map[string]any

// ColRef wraps a column reference so the bridge distinguishes "compare to a column"
// from "compare to a literal".
type ColRef struct {
	Name string
}

// JoinKind describes the join type.
type JoinKind int

const (
	JoinInner JoinKind = iota
	JoinLeft
	JoinRight
	JoinFull
)

// JoinSpec wraps a table name with a join kind for outer joins.
type JoinSpec struct {
	Table string
	Kind  JoinKind
}

// SubRef wraps a sub-builder as an aliased subquery participant.
type SubRef struct {
	Builder ReadIntent
}

// Col wraps a column reference.
//
//	.Where("u.id", "=", orm.Col("p.user_id"))
func Col(name string) ColRef {
	return ColRef{Name: name}
}

// LeftJoin returns a JoinSpec for LEFT JOIN.
func LeftJoin(table string) JoinSpec {
	return JoinSpec{Table: table, Kind: JoinLeft}
}

// RightJoin returns a JoinSpec for RIGHT JOIN.
func RightJoin(table string) JoinSpec {
	return JoinSpec{Table: table, Kind: JoinRight}
}

// FullJoin returns a JoinSpec for FULL JOIN.
func FullJoin(table string) JoinSpec {
	return JoinSpec{Table: table, Kind: JoinFull}
}

// BridgeRef is the type-erased bridge interface — any *Bridge[T] satisfies it.
type BridgeRef interface {
	GetIntent() ReadIntent
}

// cutDotted splits an "alias.column"-shaped reference at its first dot,
// reporting whether one was found. The strings.Cut equivalent, spelled
// via core.Index since this repo's vendored core-go snapshot predates
// core.Cut.
func cutDotted(s string) (before, after string, found bool) {
	idx := core.Index(s, ".")
	if idx < 0 {
		return s, "", false
	}
	return s[:idx], s[idx+1:], true
}

// aliasTableName extracts the underlying table name from a From(A{})
// participant, regardless of which of the three declared shapes (plain
// string, JoinSpec, SubRef) it is. Shared by the bridge (field-reference
// resolution in Where/OrWhere) and every SQL-shaped Medium (FROM-clause
// construction) so both sides agree on what a given alias points at.
// Empty for a participant type outside the declared vocabulary, or a
// SubRef whose sub-builder carries no Schema.
func aliasTableName(participant any) string {
	switch v := participant.(type) {
	case string:
		return v
	case JoinSpec:
		return v.Table
	case SubRef:
		return v.Builder.Schema.Name
	default:
		return ""
	}
}

// joinKindSupported reports whether caps declares support for kind.
func joinKindSupported(caps JoinCaps, kind JoinKind) bool {
	switch kind {
	case JoinInner:
		return caps.Inner
	case JoinLeft:
		return caps.Left
	case JoinRight:
		return caps.Right
	case JoinFull:
		return caps.Full
	default:
		return false
	}
}

// checkAliasCapabilities validates each From(A{}) participant against the
// Medium's declared JoinKinds / Subqueries capabilities. Mirrors the
// existing top-level Aliases gate in dispatchRead: a participant the
// Medium can't honour fails fast with a code naming which capability was
// missing (RFC §8: orm.join.unsupported / orm.subquery.unsupported)
// instead of silently building a query the Medium can't actually run.
func checkAliasCapabilities(caps Capabilities, alias map[string]any) core.Result {
	for _, participant := range alias {
		switch v := participant.(type) {
		case JoinSpec:
			if !joinKindSupported(caps.JoinKinds, v.Kind) {
				return core.Fail(ErrJoinUnsupported)
			}
		case SubRef:
			if !caps.Subqueries {
				return core.Fail(ErrSubqueryUnsupported)
			}
		}
	}
	return core.Ok(nil)
}

// GetIntent returns the bridge's current ReadIntent, resolving Schema
// first if the bridge hasn't dispatched yet. Schema is normally only
// populated inside dispatchRead at terminal-verb time (Get/Find/...) —
// but Sub() and Union/Intersect/Except call GetIntent on a builder that
// is handed over BEFORE any terminal verb runs, so without this a Sub()
// participant would always carry an empty Schema.Name: no columns to
// select, no table to query, the subquery unbuildable regardless of how
// the SQL builder renders it.
func (b *Bridge[T]) GetIntent() ReadIntent {
	if b.readIntent.Schema.Name == "" {
		b.readIntent.Schema = b.schema()
	}
	return b.readIntent
}

// Sub wraps a sub-builder as an aliased subquery participant.
func Sub(b BridgeRef) SubRef {
	return SubRef{Builder: b.GetIntent()}
}

// From declares query aliases — moves the bridge into multi-table mode.
func (b *Bridge[T]) From(a A) *Bridge[T] {
	b.readIntent.Alias = a
	return b
}

// Union combines multiple builders with UNION.
func Union(builders ...BridgeRef) BridgeRef {
	return setOperation(SetUnion, builders...)
}

// Intersect combines multiple builders with INTERSECT.
func Intersect(builders ...BridgeRef) BridgeRef {
	return setOperation(SetIntersect, builders...)
}

// Except combines multiple builders with EXCEPT.
func Except(builders ...BridgeRef) BridgeRef {
	return setOperation(SetExcept, builders...)
}

type setOpBridge struct {
	intent ReadIntent
}

func (s setOpBridge) GetIntent() ReadIntent {
	return s.intent
}

func setOperation(kind SetOpKind, builders ...BridgeRef) BridgeRef {
	intents := make([]ReadIntent, 0, len(builders))
	for _, builder := range builders {
		if builder != nil {
			intents = append(intents, builder.GetIntent())
		}
	}
	if len(intents) == 0 {
		return nil
	}
	return setOpBridge{intent: ReadIntent{SetOp: &SetOp{Kind: kind, Builders: intents}}}
}
