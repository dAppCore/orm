// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

// Stable error codes for bridge dispatch failures.
var (
	ErrNotFound            = core.NewCode("orm.notfound", "no matching row")
	ErrAmbiguous           = core.NewCode("orm.ambiguous", "First matched more than one row")
	ErrSchemaMissing       = core.NewCode("orm.schema.missing", "type does not implement Modelled")
	ErrSchemaInvalid       = core.NewCode("orm.schema.invalid", "schema declaration is malformed")
	ErrMediumNotMounted    = core.NewCode("orm.medium.notmounted", "named medium not registered")
	ErrMediumUnsupported   = core.NewCode("orm.medium.unsupported", "medium cannot honour intent shape")
	ErrPredicateUnsupport  = core.NewCode("orm.predicate.unsupported", "medium cannot honour predicate")
	ErrJoinsUnsupported    = core.NewCode("orm.joins.unsupported", "medium does not support joins")
	ErrAggregateUnsupport  = core.NewCode("orm.aggregate.unsupported", "medium cannot compute aggregate")
	ErrTxUnsupported       = core.NewCode("orm.tx.unsupported", "medium does not support transactions")
	ErrTxCrossMedium       = core.NewCode("orm.tx.cross_medium", "transaction touched multiple mediums")
	ErrConstraint          = core.NewCode("orm.constraint", "unique/null/check violation")
	ErrConflict            = core.NewCode("orm.conflict", "optimistic-lock version mismatch on save")
	ErrCast                = core.NewCode("orm.cast", "value could not be cast to expected type")
	ErrInputField          = core.NewCode("orm.input.field", "predicate referenced unknown field")
	ErrInputType           = core.NewCode("orm.input.type", "value could not be coerced to field type")
	ErrInputNull           = core.NewCode("orm.input.null", "required field received nil/empty")
	ErrInputFormat         = core.NewCode("orm.input.format", "value failed format validation")
	ErrInputRange          = core.NewCode("orm.input.range", "value out of allowed range")
	ErrInputPattern        = core.NewCode("orm.input.pattern", "value does not match pattern")
	ErrInputOneOf          = core.NewCode("orm.input.oneof", "value not in allowed set")
	ErrInputCoerce         = core.NewCode("orm.input.coerce", "coercer rejected the value")
	ErrOutputType          = core.NewCode("orm.output.type", "medium returned un-hydratable value")
	ErrOutputFormat        = core.NewCode("orm.output.format", "stored value violates format on rehydrate")
	ErrOutputCoerce        = core.NewCode("orm.output.coerce", "rehydrate shaper rejected stored value")
	ErrOutputCrypt         = core.NewCode("orm.output.crypt", "decryption failed during rehydration")
	ErrOutputJSON          = core.NewCode("orm.output.json", "JSON column unmarshal failed during rehydration")
	ErrServiceExists       = core.NewCode("orm.service.exists", "orm.Register called twice on same Core")
	ErrActionUnknown       = core.NewCode("orm.action.unknown", "service-form action not registered")
	ErrSchemaMount         = core.NewCode("orm.schema.mount", "MountSchemas failed — bad JSON, prefix missing, or name conflict")
	ErrWatchUnsupported    = core.NewCode("orm.watch.unsupported", "medium does not support Watch")
	ErrWatchClosed         = core.NewCode("orm.watch.closed", "stream consumed after ctx cancel or subscription end")
	ErrSearchUnsupported   = core.NewCode("orm.search.unsupported", "search mode not supported by medium")
	ErrSearchDimension     = core.NewCode("orm.search.dimension", "vector search dimension mismatch")
	ErrAliasesUnsupported  = core.NewCode("orm.aliases.unsupported", "medium does not support aliased FROM")
	ErrJoinUnsupported     = core.NewCode("orm.join.unsupported", "medium does not support requested join kind")
	ErrSubqueryUnsupported = core.NewCode("orm.subquery.unsupported", "medium does not support subqueries")
	ErrSetOpUnsupported    = core.NewCode("orm.setop.unsupported", "medium does not support set operations")
)

func resultError(r core.Result, code, message string) error {
	if err, ok := r.Value.(error); ok {
		return err
	}
	return core.NewCode(code, message)
}
