// SPDX-License-Identifier: EUPL-1.2

package orm

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

// GetIntent returns the bridge's current ReadIntent.
func (b *Bridge[T]) GetIntent() ReadIntent {
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
