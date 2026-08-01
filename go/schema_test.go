// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	"slices"

	. "dappco.re/go"
	"dappco.re/go/orm"
)

// Test model for schema tests
type TestUser struct {
	ID    int64
	Email string
	Name  string
}

func (TestUser) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("users")
		b.PK("id")
		b.String("email").Unique().NotNull().Format("email").Max(254)
		b.String("name").NotNull()
	})
}

func TestSchema_Define_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("users")
		b.PK("id")
		b.String("email").Unique().NotNull().Format("email").Max(254)
		b.String("name").NotNull().Min(1).Max(255)
		b.Bool("active").Default(true)
		b.Time("created_at").Default("now")
		b.Float64("score").Min(0).Max(100)
		b.HasMany("posts", "user_id")
		b.Index("email")
	})

	AssertEqual(t, "users", schema.Name)
	AssertEqual(t, 1, len(schema.PK))
	AssertEqual(t, "id", schema.PK[0])
	AssertEqual(t, 6, len(schema.Fields))

	id := schema.Fields[0]
	AssertEqual(t, "id", id.Name)
	AssertEqual(t, "int64", id.Type)
	AssertTrue(t, containsConst(id.Constraints, "pk"))

	email := schema.Fields[1]
	AssertEqual(t, "email", email.Name)
	AssertEqual(t, "string", email.Type)
	AssertEqual(t, "email", email.Format)
	AssertTrue(t, *email.Max == 254)
	AssertTrue(t, containsConst(email.Constraints, "unique"))
	AssertTrue(t, containsConst(email.Constraints, "notnull"))

	active := schema.Fields[3]
	AssertEqual(t, "bool", active.Type)
	AssertEqual(t, true, active.Default)

	createdAt := schema.Fields[4]
	AssertEqual(t, "time", createdAt.Type)
	AssertEqual(t, "now", createdAt.Default)

	score := schema.Fields[5]
	AssertEqual(t, "float64", score.Type)
	AssertTrue(t, *score.Min == 0)
	AssertTrue(t, *score.Max == 100)

	AssertEqual(t, 1, len(schema.Relations))
	AssertEqual(t, "hasMany", schema.Relations[0].Kind)
	AssertEqual(t, "posts", schema.Relations[0].Name)

	AssertEqual(t, 1, len(schema.Indexes))
	AssertEqual(t, 1, len(schema.Indexes[0].Fields))
	AssertEqual(t, "email", schema.Indexes[0].Fields[0])
}

func TestSchema_Modelled_Good(t *T) {
	u := TestUser{}
	schema := u.Schema()
	AssertEqual(t, "users", schema.Name)
	AssertEqual(t, "id", schema.PK[0])
}

func TestSchema_CompositePK_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("user_roles")
		b.PK("user_id")
		b.PK("role_id")
		b.Int64("user_id")
		b.Int64("role_id")
	})

	AssertEqual(t, 2, len(schema.PK))
	AssertEqual(t, "user_id", schema.PK[0])
	AssertEqual(t, "role_id", schema.PK[1])
}

func TestSchema_Relations_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("posts")
		b.PK("id")
		b.Int64("id")
		b.Int64("user_id").NotNull()
		b.String("title").NotNull()
		b.BelongsTo("user", "user_id")
		b.HasMany("comments", "post_id")
		b.HasOne("metadata", "post_id")
		b.ManyMany("tags", "post_tags", "post_id", "tag_id")
	})

	AssertEqual(t, 4, len(schema.Relations))

	AssertEqual(t, "belongsTo", schema.Relations[0].Kind)
	AssertEqual(t, "user", schema.Relations[0].Name)
	AssertEqual(t, "user_id", schema.Relations[0].FK)

	AssertEqual(t, "hasMany", schema.Relations[1].Kind)
	AssertEqual(t, "hasOne", schema.Relations[2].Kind)

	AssertEqual(t, "manyMany", schema.Relations[3].Kind)
	AssertEqual(t, "tags", schema.Relations[3].Name)
	AssertEqual(t, "post_tags", schema.Relations[3].Through)
	AssertEqual(t, "post_id", schema.Relations[3].FKA)
	AssertEqual(t, "tag_id", schema.Relations[3].FKB)
}

func TestSchema_OneOf_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("items")
		b.PK("id")
		b.String("tier").OneOf("free", "pro", "enterprise")
	})

	f, ok := schema.FieldByName("tier")
	AssertTrue(t, ok)
	AssertEqual(t, 3, len(f.OneOf))
	AssertEqual(t, "free", f.OneOf[0])
	AssertEqual(t, "pro", f.OneOf[1])
	AssertEqual(t, "enterprise", f.OneOf[2])
}

func TestSchema_Pattern_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("items")
		b.PK("id")
		b.String("code").Pattern(`^[A-Z]{3}-\d{4}$`)
	})

	f, ok := schema.FieldByName("code")
	AssertTrue(t, ok)
	AssertEqual(t, `^[A-Z]{3}-\d{4}$`, f.Pattern)
}

func TestSchema_BytesMaxBytes_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("files")
		b.PK("id")
		b.Bytes("data").MaxBytes(1048576)
	})

	f, ok := schema.FieldByName("data")
	AssertTrue(t, ok)
	AssertEqual(t, "bytes", f.Type)
	AssertTrue(t, *f.MaxBytes == 1048576)
}

func TestSchema_Coerce_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("items")
		b.PK("id")
		b.Int("active").Coerce("BoolToInt")
	})

	f, ok := schema.FieldByName("active")
	AssertTrue(t, ok)
	AssertEqual(t, "BoolToInt", f.CoerceName)
}

func TestSchema_Searchable_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("docs")
		b.PK("id")
		b.String("title").Searchable("text")
		b.Bytes("embedding").Searchable("vector", 1536)
		b.String("description").Searchable("hybrid")
	})

	title, _ := schema.FieldByName("title")
	AssertEqual(t, "text", title.SearchableKind)
	AssertEqual(t, 0, title.VectorDim)

	emb, _ := schema.FieldByName("embedding")
	AssertEqual(t, "vector", emb.SearchableKind)
	AssertEqual(t, 1536, emb.VectorDim)

	desc, _ := schema.FieldByName("description")
	AssertEqual(t, "hybrid", desc.SearchableKind)
}

func TestSchema_Render_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("items")
		b.PK("id")
		b.Time("created_at").Render("rfc3339")
	})

	f, _ := schema.FieldByName("created_at")
	AssertEqual(t, "rfc3339", f.Render)
}

func TestSchema_FieldByName_Bad(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("users")
		b.PK("id")
		b.String("email")
	})

	_, ok := schema.FieldByName("nonexistent")
	AssertTrue(t, !ok)
}

func TestSchema_JSONRoundTrip_Good(t *T) {
	schema := orm.Define(func(b *orm.Builder) {
		b.Name("users")
		b.PK("id")
		b.String("email").Unique().NotNull().Format("email")
		b.String("name").NotNull()
		b.Bool("active").Default(true)
		b.HasMany("posts", "user_id")
		b.Index("email")
	})

	r := JSONMarshal(schema)
	AssertTrue(t, r.OK)
	data := r.Value.([]byte)

	restoreR := orm.SchemaFromJSON(data)
	AssertTrue(t, restoreR.OK)
	restored, ok := Cast[orm.Schema](restoreR)
	AssertTrue(t, ok)

	AssertEqual(t, schema.Name, restored.Name)
	AssertEqual(t, len(schema.PK), len(restored.PK))
	AssertEqual(t, len(schema.Fields), len(restored.Fields))
	AssertEqual(t, len(schema.Relations), len(restored.Relations))
	AssertEqual(t, len(schema.Indexes), len(restored.Indexes))

	// Verify specific field round-trips
	for i, f := range schema.Fields {
		AssertEqual(t, f.Name, restored.Fields[i].Name)
		AssertEqual(t, f.Type, restored.Fields[i].Type)
	}
}

func TestSchema_JSONRoundTrip_Bad(t *T) {
	r := orm.SchemaFromJSON([]byte("{invalid json"))
	AssertTrue(t, !r.OK)
}

// containsConst checks if a constraint string exists in the constraints slice
func containsConst(constraints []string, c string) bool {
	return slices.Contains(constraints, c)
}
