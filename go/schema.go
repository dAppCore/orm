// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

// OrderBy describes an ordering directive for a query.
//
//	orm.Of[User](c).Order("created_at", "desc").Get()
type OrderBy struct {
	Field string `json:"field"`
	Desc  bool   `json:"desc,omitempty"`
}

// AggregateOp describes an aggregation operation for a query.
//
//	res := orm.Of[Order](c).Sum("total")
type AggregateOp struct {
	Op    string `json:"op"`    // "count" | "sum" | "min" | "max" | "avg"
	Field string `json:"field"` // empty for Count
}

// Relation describes a relationship between two models.
type Relation struct {
	Kind    string `json:"kind"`              // "hasMany" | "belongsTo" | "hasOne" | "manyMany"
	Name    string `json:"name"`              // relation name (e.g. "posts")
	FK      string `json:"fk,omitempty"`      // foreign key column
	Through string `json:"through,omitempty"` // pivot table for manyMany
	FKA     string `json:"fka,omitempty"`     // first FK for manyMany pivot
	FKB     string `json:"fkb,omitempty"`     // second FK for manyMany pivot
}

// Index describes an explicit index on one or more columns.
type Index struct {
	Fields []string `json:"fields"`
}

// Field describes a single column in a model's storage shape.
//
//	field := schema.FieldByName("email")
//	if field.NotNull { ... }
type Field struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`                     // "string" | "int" | "int64" | "bool" | "float" | "float64" | "time" | "json" | "bytes"
	Constraints    []string `json:"constraints,omitempty"`    // "pk" | "unique" | "notnull"
	Default        any      `json:"default,omitempty"`        // default value
	Format         string   `json:"format,omitempty"`         // "email" | "url" | "uuid" | "ipv4" | "ipv6" | "hostname" | "slug"
	Min            *int64   `json:"min,omitempty"`            // numeric or string-length minimum
	Max            *int64   `json:"max,omitempty"`            // numeric or string-length maximum
	Pattern        string   `json:"pattern,omitempty"`        // regex validation pattern
	OneOf          []any    `json:"oneOf,omitempty"`          // enum constraint values
	MaxBytes       *int64   `json:"maxBytes,omitempty"`       // blob max size
	CoerceName     string   `json:"coerce,omitempty"`         // named cross-type coercer function
	SearchableKind string   `json:"searchableKind,omitempty"` // "text" | "vector" | "hybrid"
	VectorDim      int      `json:"vectorDim,omitempty"`      // vector dimension for vector searchable fields
	Render         string   `json:"render,omitempty"`         // display renderer hint ("rfc3339", "symbol-then-amount")
}

// Schema describes a model's complete storage shape — table name, columns,
// indexes, and relations. It is pure data; serialisable to JSON for the
// polyglot contract.
//
//	schema := User{}.Schema()
//	data, _ := core.Cast[[]byte](core.JSONMarshal(schema))
type Schema struct {
	Name      string     `json:"name"`
	PK        []string   `json:"pk"`
	Fields    []Field    `json:"fields"`
	Indexes   []Index    `json:"indexes"`
	Relations []Relation `json:"relations"`
}

// FieldByName returns the Field with the given name, or nil if not found.
//
//	f, ok := schema.FieldByName("email")
func (s Schema) FieldByName(name string) (Field, bool) {
	for _, f := range s.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// Modelled is the contract every ORM-managed type implements.
//
//	func (User) Schema() orm.Schema { return orm.Define(...) }
type Modelled interface {
	Schema() Schema
}

// Builder constructs a Schema fluently.
//
//	orm.Define(func(b *orm.Builder) {
//	    b.Name("users")
//	    b.PK("id")
//	    b.String("email").Unique().NotNull()
//	})
type Builder struct {
	schema  Schema
	current *Field
}

// FieldBuilder chains constraints and modifiers onto the current Field.
type FieldBuilder struct {
	b *Builder
}

// Define is the entry point for Schema construction.
//
//	schema := orm.Define(func(b *orm.Builder) {
//	    b.Name("users")
//	    b.PK("id")
//	    b.String("email").Unique().NotNull()
//	})
func Define(fn func(*Builder)) Schema {
	b := &Builder{}
	fn(b)
	b.finaliseCurrent()
	return b.schema
}

func (b *Builder) finaliseCurrent() {
	if b.current != nil {
		b.schema.Fields = append(b.schema.Fields, *b.current)
		b.current = nil
	}
}

// Name sets the table name.
//
//	b.Name("users")
func (b *Builder) Name(name string) *Builder {
	b.schema.Name = name
	return b
}

// PK declares a primary key column. Repeated calls compose a composite PK.
//
//	b.PK("id")
//	b.PK("user_id")
//	b.PK("role_id")   // composite: user_id, role_id
func (b *Builder) PK(name string) *Builder {
	b.finaliseCurrent()
	if !hasString(b.schema.PK, name) {
		b.schema.PK = append(b.schema.PK, name)
	}
	if idx := b.fieldIndex(name); idx >= 0 {
		addFieldConstraint(&b.schema.Fields[idx], "pk")
		return b
	}
	b.schema.Fields = append(b.schema.Fields, Field{Name: name, Type: "int64", Constraints: []string{"pk"}})
	return b
}

// HasMany declares a one-to-many relation: Other.fk → This.id
//
//	b.HasMany("posts", "user_id")
func (b *Builder) HasMany(name, fk string) *Builder {
	b.schema.Relations = append(b.schema.Relations, Relation{
		Kind: "hasMany", Name: name, FK: fk,
	})
	return b
}

// BelongsTo declares a belongs-to relation: This.fk → Other.id
//
//	b.BelongsTo("user", "user_id")
func (b *Builder) BelongsTo(name, fk string) *Builder {
	b.schema.Relations = append(b.schema.Relations, Relation{
		Kind: "belongsTo", Name: name, FK: fk,
	})
	return b
}

// HasOne declares a one-to-one relation.
//
//	b.HasOne("profile", "user_id")
func (b *Builder) HasOne(name, fk string) *Builder {
	b.schema.Relations = append(b.schema.Relations, Relation{
		Kind: "hasOne", Name: name, FK: fk,
	})
	return b
}

// ManyMany declares a many-to-many pivot-table relation.
//
//	b.ManyMany("tags", "post_tags", "post_id", "tag_id")
func (b *Builder) ManyMany(name, through, fkA, fkB string) *Builder {
	b.schema.Relations = append(b.schema.Relations, Relation{
		Kind: "manyMany", Name: name, Through: through, FKA: fkA, FKB: fkB,
	})
	return b
}

// Index declares an explicit index on one or more columns.
//
//	b.Index("email")
//	b.Index("user_id", "role_id")
func (b *Builder) Index(cols ...string) *Builder {
	b.schema.Indexes = append(b.schema.Indexes, Index{Fields: cols})
	return b
}

// --- Field creation methods ---

func (b *Builder) beginField(name, typ string) *FieldBuilder {
	b.finaliseCurrent()
	if idx := b.fieldIndex(name); idx >= 0 {
		existing := b.schema.Fields[idx]
		b.schema.Fields = append(b.schema.Fields[:idx], b.schema.Fields[idx+1:]...)
		existing.Type = typ
		b.current = &existing
		return &FieldBuilder{b: b}
	}
	b.current = &Field{Name: name, Type: typ}
	if hasString(b.schema.PK, name) {
		addFieldConstraint(b.current, "pk")
	}
	return &FieldBuilder{b: b}
}

func (b *Builder) fieldIndex(name string) int {
	for i, field := range b.schema.Fields {
		if normalizeName(field.Name) == normalizeName(name) {
			return i
		}
	}
	return -1
}

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func addFieldConstraint(field *Field, c string) {
	for _, existing := range field.Constraints {
		if existing == c {
			return
		}
	}
	field.Constraints = append(field.Constraints, c)
}

// String declares a text column.
//
//	b.String("email").Unique().NotNull().Format("email")
func (b *Builder) String(name string) *FieldBuilder {
	return b.beginField(name, "string")
}

// Int declares an integer column.
//
//	b.Int("age").Min(0).Max(150)
func (b *Builder) Int(name string) *FieldBuilder {
	return b.beginField(name, "int")
}

// Int64 declares a 64-bit integer column.
//
//	b.Int64("user_id").NotNull()
func (b *Builder) Int64(name string) *FieldBuilder {
	return b.beginField(name, "int64")
}

// Bool declares a boolean column.
//
//	b.Bool("active").Default(true)
func (b *Builder) Bool(name string) *FieldBuilder {
	return b.beginField(name, "bool")
}

// Float declares a float column.
//
//	b.Float("rating").Min(0).Max(5)
func (b *Builder) Float(name string) *FieldBuilder {
	return b.beginField(name, "float")
}

// Float64 declares a 64-bit float column.
//
//	b.Float64("amount").NotNull()
func (b *Builder) Float64(name string) *FieldBuilder {
	return b.beginField(name, "float64")
}

// Time declares a timestamp column.
//
//	b.Time("created_at").Default("now")
func (b *Builder) Time(name string) *FieldBuilder {
	return b.beginField(name, "time")
}

// JSON declares a JSON-encoded payload column.
//
//	b.JSON("settings")
func (b *Builder) JSON(name string) *FieldBuilder {
	return b.beginField(name, "json")
}

// Bytes declares a binary blob column.
//
//	b.Bytes("avatar").MaxBytes(1048576)
func (b *Builder) Bytes(name string) *FieldBuilder {
	return b.beginField(name, "bytes")
}

// --- FieldBuilder chained modifiers ---

func (fb *FieldBuilder) addConstraint(c string) {
	for _, existing := range fb.b.current.Constraints {
		if existing == c {
			return
		}
	}
	fb.b.current.Constraints = append(fb.b.current.Constraints, c)
}

// Unique marks the field as unique.
//
//	b.String("email").Unique()
func (fb *FieldBuilder) Unique() *FieldBuilder {
	fb.addConstraint("unique")
	return fb
}

// NotNull marks the field as not-nullable.
//
//	b.String("name").NotNull()
func (fb *FieldBuilder) NotNull() *FieldBuilder {
	fb.addConstraint("notnull")
	return fb
}

// Default sets the default value for the field.
//
//	b.Bool("active").Default(true)
//	b.Time("created_at").Default("now")
func (fb *FieldBuilder) Default(v any) *FieldBuilder {
	fb.b.current.Default = v
	return fb
}

// Format declares the input shape for validation.
//
//	b.String("email").Format("email")
//	b.String("avatar").Format("url")
//	b.String("key").Format("uuid")
func (fb *FieldBuilder) Format(name string) *FieldBuilder {
	fb.b.current.Format = name
	return fb
}

// Min sets a numeric or string-length minimum bound.
//
//	b.Int("age").Min(0)
//	b.String("name").Min(1)
func (fb *FieldBuilder) Min(n int64) *FieldBuilder {
	fb.b.current.Min = &n
	return fb
}

// Max sets a numeric or string-length maximum bound.
//
//	b.Int("age").Max(150)
//	b.String("email").Max(254)
func (fb *FieldBuilder) Max(n int64) *FieldBuilder {
	fb.b.current.Max = &n
	return fb
}

// Pattern sets a regex validation pattern for the field.
//
//	b.String("code").Pattern(`^[A-Z]{3}-\d{4}$`)
func (fb *FieldBuilder) Pattern(re string) *FieldBuilder {
	fb.b.current.Pattern = re
	return fb
}

// OneOf declares an enum constraint — input rejected if not in the set.
//
//	b.String("tier").OneOf("free", "pro", "enterprise")
func (fb *FieldBuilder) OneOf(vals ...any) *FieldBuilder {
	fb.b.current.OneOf = vals
	return fb
}

// MaxBytes sets a blob column max size — input rejected if larger.
//
//	b.Bytes("avatar").MaxBytes(1048576)
func (fb *FieldBuilder) MaxBytes(n int64) *FieldBuilder {
	fb.b.current.MaxBytes = &n
	return fb
}

// Coerce declares a named cross-type coercer function that runs first
// in the validation chain.
//
//	b.Int("active").Coerce("BoolToInt")
func (fb *FieldBuilder) Coerce(name string) *FieldBuilder {
	fb.b.current.CoerceName = name
	return fb
}

// Searchable declares the field as searchable.
//
//	b.String("title").Searchable("text")
//	b.Bytes("embedding").Searchable("vector", 1536)
func (fb *FieldBuilder) Searchable(kind string, dim ...int) *FieldBuilder {
	fb.b.current.SearchableKind = kind
	if len(dim) > 0 {
		fb.b.current.VectorDim = dim[0]
	}
	return fb
}

// Render declares a display renderer hint for downstream consumers.
//
//	b.Time("created_at").Render("rfc3339")
//	b.Int("total").Render("symbol-then-amount")
func (fb *FieldBuilder) Render(template string) *FieldBuilder {
	fb.b.current.Render = template
	return fb
}

// SchemaFromJSON restores a Schema from its canonical JSON representation.
//
//	data, _ := core.Cast[[]byte](core.JSONMarshal(schema))
//	restored, _ := core.Cast[Schema](orm.SchemaFromJSON(data))
func SchemaFromJSON(data []byte) core.Result {
	var s Schema
	r := core.JSONUnmarshal(data, &s)
	if !r.OK {
		return r
	}
	return core.Ok(s)
}
