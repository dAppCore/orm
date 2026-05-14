// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

// Search runs a ranked search over fields the Schema declared Searchable.
//
//	res := orm.Of[Document](c).Search("eloquent monks", orm.SearchOpts{Limit: 20})
func (b *Bridge[T]) Search(query string, opts SearchOpts) core.Result {
	schema := b.schema()
	if schema.Name == "" {
		return core.Fail(core.NewCode("orm.schema.missing", "type does not implement Modelled"))
	}
	medium, mr := b.resolveMedium(b.medium())
	if !mr.OK {
		return mr
	}
	caps := medium.Caps().Search
	mode := opts.Mode
	if mode == "" {
		if caps.Text && caps.Vector && caps.Hybrid {
			mode = "hybrid"
		} else if len(opts.Vector) > 0 && caps.Vector {
			mode = "vector"
		} else if caps.Text {
			mode = "text"
		} else if caps.Vector {
			mode = "vector"
		} else if caps.Hybrid {
			mode = "hybrid"
		}
	}
	if mode == "" {
		return core.Fail(core.NewCode("orm.search.unsupported", "no search mode supported"))
	}
	if (mode == "text" && !caps.Text) || (mode == "vector" && !caps.Vector) || (mode == "hybrid" && !caps.Hybrid) {
		return core.Fail(core.NewCode("orm.search.unsupported", "search mode not supported: "+mode))
	}
	if len(opts.Facets) > 0 && !caps.Facets {
		return core.Fail(core.NewCode("orm.search.unsupported", "search facets not supported"))
	}
	if mode == "vector" || mode == "hybrid" {
		if !searchVectorDimensionMatches(schema, opts.Vector) {
			return core.Fail(core.NewCode("orm.search.dimension", "vector dimension does not match schema"))
		}
	}

	b.readIntent.Search = &SearchSpec{
		Query:     query,
		Mode:      mode,
		Vector:    opts.Vector,
		Limit:     opts.Limit,
		Offset:    opts.Offset,
		Highlight: opts.Highlight,
		Facets:    opts.Facets,
	}
	b.readIntent.Schema = schema

	r := medium.Search(b.c.Context(), b.readIntent)
	if !r.OK {
		return r
	}
	payload, ok := r.Value.(*Payload)
	if !ok {
		return core.Fail(core.NewCode("orm.search.payload", "unexpected payload type from medium"))
	}
	raw, ok := payload.Data.([]RankedResult)
	if !ok {
		return core.Fail(core.NewCode("orm.search.data", "unexpected search data type from medium"))
	}
	out := make([]Ranked[T], 0, len(raw))
	for _, item := range raw {
		var value T
		if populated := populateStructFromRow(&value, schema, item.Value); !populated.OK {
			return populated
		}
		out = append(out, Ranked[T]{
			Value:      value,
			Score:      item.Score,
			Highlights: item.Highlights,
			Distance:   item.Distance,
			Facets:     item.Facets,
		})
	}
	payload.Data = out
	return core.Ok(payload)
}

func searchVectorDimensionMatches(schema Schema, vector []float32) bool {
	if len(vector) == 0 {
		return true
	}
	for _, field := range schema.Fields {
		if field.SearchableKind != "vector" && field.SearchableKind != "hybrid" {
			continue
		}
		if field.VectorDim == len(vector) {
			return true
		}
	}
	return false
}

// SearchOpts configures a Search query.
type SearchOpts struct {
	Limit     int
	Offset    int
	Highlight bool
	Facets    []string
	Mode      string
	Vector    []float32
}

// Ranked is a search result with score and optional highlights.
type Ranked[T any] struct {
	Value      T
	Score      float64
	Highlights map[string]string
	Distance   float64
	Facets     map[string]map[string]int
}
