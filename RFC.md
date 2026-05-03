---
Status: Aspirational
module: dappco.re/go/orm
repo: core/orm
lang: go
tier: lib
depends:
  - code/core/go
  - code/core/go/io
tags:
  - orm
  - data
  - bridge
  - medium
  - query
  - eloquent
  - polyglot
---

# core/orm RFC — Typed Fluent Communications Bridge

> An agent should be able to use this store from this document alone.

**Module:** `dappco.re/go/orm`
**Repository:** `core/orm`
**Files:** ~10

> *Two Monks.* The bridge is stateless. Cross the data, set it down, walk on. No dirty-tracking, no session managers, no lazy proxies. Writes are visible at the call site.
>
> *Chinese Farmer.* No backend is pre-judged. Capability is declared by the Medium, never assumed by the bridge. The Medium honours what it can and fails honestly on what it can't.

---

## 1. Overview

`core/orm` is a typed fluent communications layer that adapts. Eloquent's reading rhythm in Go, without Eloquent's machinery. The bridge produces *intent* (Find / Where / Save); a Medium *transports* that intent per its native capabilities. Same call lands against DuckDB today, Postgres tomorrow, a Borg.DataNode next week — the bridge withholds judgment about which backend is "right"; it just transports.

**What it is not.** Not a heavy ORM. No schema language. No codegen factory. No query optimiser. No proxy objects. No lazy relations to lecture about N+1.

**What it is.** A small, typed, fluent surface (`orm.Of[T]()`, `orm.Save`) over a unified Medium contract. The Eloquent reading rhythm Go has been missing, in ~10 files, zero codegen, single binary.

**Consumed two ways:** as a normal Go package (`import "dappco.re/go/orm"`) — the v1 form — or, in a future phase, mounted as a service inside a Conclave runtime via `c.Service("orm")`. v1 is package-form only; service-form is deferred.

---

## 2. Architecture

Three layers, each with a single job:

```
┌────────────────────────────────────────────────────────────────┐
│  Caller code                                                   │
│    user, _ := core.Cast[User](orm.Of[User](c).Find(1))         │
│    orm.Save(c, &user)                                          │
└─────────────────────────┬──────────────────────────────────────┘
                          │ produces Intent (data, no execution)
                          ▼
┌────────────────────────────────────────────────────────────────┐
│  core/orm — the bridge (THIS REPO)                             │
│  • Schema declaration via Schema() method on T                 │
│  • Generic typed surface: orm.Of[T](c), orm.Save/Delete/Tx     │
│  • Intent types: ReadIntent, WriteIntent, Predicate            │
│  • Capability check: ask Medium what it can honour             │
│  • Schema cache (lazy, registry-backed)                        │
└─────────────────────────┬──────────────────────────────────────┘
                          │ dispatches Intent → Medium
                          ▼
┌────────────────────────────────────────────────────────────────┐
│  Medium implementations (separate packages, opt-in via import) │
│  • orm-sql       — wraps core.SQLOpen / core.DB                │
│  • orm-kv        — wraps go-store KV                           │
│  • orm-medium    — wraps go-io.Medium for blob/RPC             │
│  • In-tree memium.go — in-memory test Medium for go test       │
└─────────────────────────┬──────────────────────────────────────┘
                          ▼
       DuckDB / SQLite / Postgres / KV / S3 / DataNode / ...
```

**File table:**

| File | Purpose |
|------|---------|
| `orm.go` | `orm.Of[T](c)` generic entry; `orm.Save`, `orm.Delete`, `orm.Tx` top-level helpers |
| `bridge.go` | `*Bridge[T]` fluent chain — `Where`, `With`, `Order`, `Find`, `First`, `Get`, `Stream`, `Count`, etc. |
| `schema.go` | `Schema`, `Field`, `Relation`, `Define`, `Builder` |
| `intent.go` | `ReadIntent`, `WriteIntent`, `Predicate`, `OrderBy`, `AggregateOp` (pure data) |
| `medium.go` | `Medium` interface, `Capabilities`, `PredicateCaps`, `Payload`, `Meta` |
| `cast.go` | ORM-specific helpers — `orm.Detail[T]` (value + meta) |
| `cache.go` | Schema cache backed by `core.Registry[Schema]` |
| `memium.go` | In-memory test Medium implementing the full contract |
| `errors.go` | Stable error codes — `orm.notfound`, `orm.medium.unsupported`, etc. |
| `input.go` | Bidirectional shapers — `Coerce` (write) + `Rehydrate` (read) + `Apply` dispatcher (§4.7, §4.8) |
| `service.go` | `Register(c)` + `*Service` + Action handler registrations (§4.9 — canonical service.go pattern) |
| `mount.go` | `MountSchemas`, `OfTable`, `*DynamicBridge` (§4.10) |
| `watch.go` | `*Bridge[T].Watch`, `Event[T]`, `WatchOp`, `WatchOpts`, polling fallback (§4.11) |
| `search.go` | `*Bridge[T].Search`, `SearchOpts`, `Ranked[T]`, `SearchSpec` (§4.12) |
| `alias.go` | `A`, `Col`, `ColRef`, `Sub`, `SubRef`, `LeftJoin`/`RightJoin`/`FullJoin`, `Union`/`Intersect`/`Except`, `*Bridge[T].From` (§4.13) |
| `ddl.go` | DDL emission, schema diff, change application (§7) |
| `*_test.go` | Good/Bad/Ugly tests per surface |

**Critical property:** an Intent is *just data*. Construct it, log it, serialise it, replay it, route it across machines. The bridge is a function from caller fluency → Intent → Medium dispatch. Pure pipe; no hidden execution; no globals beyond the lazy schema cache.

**What core/orm does NOT own:**
- Drivers — concrete Medium impls live in sibling packages, opted in via import
- Connection pooling — Medium's job (or `*core.DB` for SQL Mediums)
- SQL string generation — the SQL Medium's job
- Migration execution — see §7

---

## 3. Model Declaration

A model is any Go type that satisfies `orm.Modelled`:

```go
// Modelled is the contract every ORM-managed type implements.
//
//   func (User) Schema() orm.Schema { return orm.Define(...) }
type Modelled interface {
    Schema() Schema
}
```

The bridge calls `(*new(T)).Schema()` once per type, caches the result. No `init()` hook, no global registration list, no struct tags.

### 3.1 The Builder Surface

Schema is built fluently — same DSL shape as queries:

```go
package mymod

import (
    "dappco.re/go/core"
    "dappco.re/go/orm"
)

// User is the canonical user row.
//
//   user, _ := core.Cast[User](orm.Of[User](c).Find(1))
type User struct {
    ID        int64
    Email     string
    Name      string
    Active    bool
    CreatedAt core.Time
}

// Schema declares User's storage shape.
//
//   schema := User{}.Schema()
func (User) Schema() orm.Schema {
    return orm.Define(func(b *orm.Builder) {
        b.Name("users")
        b.PK("id")
        b.String("email").Unique().NotNull()
        b.String("name").NotNull()
        b.Bool("active").Default(true)
        b.Time("created_at").Default("now")
        b.HasMany("posts", "user_id")
        b.Index("email")
    })
}
```

### 3.2 Field Builder Verbs

| Verb | Meaning |
|------|---------|
| `String(name)` | text column |
| `Int(name)` / `Int64(name)` | integer column |
| `Bool(name)` | boolean column |
| `Float(name)` / `Float64(name)` | float column |
| `Time(name)` | timestamp column |
| `JSON(name)` | JSON-encoded payload column |
| `Bytes(name)` | binary blob column |
| `PK(name)` | primary key (single or composite via repeated calls) |
| `Unique()` | constraint chained on the field |
| `NotNull()` | constraint chained on the field |
| `Default(v)` | default value chained on the field |
| `Format(name)` | declares input shape (`"email"`, `"url"`, `"uuid"`, `"ipv4"`, `"ipv6"`, `"hostname"`, `"slug"`) — see §4.7 |
| `Min(n)` / `Max(n)` | numeric bounds, or string-length bounds when applied to text fields |
| `Pattern(re)` | regex-validated string |
| `OneOf(vals...)` | enum constraint — input rejected if not in the set |
| `MaxBytes(n)` | blob column max size — input rejected if larger |
| `Index(cols...)` | explicit index, table-level |
| `HasMany(name, fk)` | relation: `Other.fk` → `This.id` |
| `BelongsTo(name, fk)` | relation: `This.fk` → `Other.id` |
| `HasOne(name, fk)` | 1:1 relation |
| `ManyMany(name, through, fkA, fkB)` | pivot-table relation |

### 3.3 Schema is Data, Not Code

The builder produces a plain `Schema` struct — no closures captured, no functions stored. Means it serialises:

```go
schema := User{}.Schema()
data, _ := core.Cast[[]byte](core.JSONMarshal(schema))   // canonical JSON
restored, _ := core.Cast[Schema](orm.SchemaFromJSON(data)) // round-trips
```

This is the polyglot bridge: `User::schema()` in PHP and `User.schema()` in TS produce the same JSON shape. core/orm/php and core/orm/ts can read each other's schemas, validate cross-language migrations, share a single migration history.

### 3.4 Lazy Schema Cache

First call to `orm.Of[User](c)` triggers `User{}.Schema()` once and stashes the result keyed by `core.TypeOf(User{}).String()` in a `core.Registry[Schema]`. Subsequent calls hit the cache. The cache is sealed via `Registry.Seal()` after first use of `core.New()` returns — no new schemas registered after startup unless the registry is explicitly reopened in tests.

### 3.5 Composite Primary Keys

Repeated `b.PK(...)` calls compose:

```go
b.PK("user_id")
b.PK("role_id")
```

…produces `PRIMARY KEY (user_id, role_id)` server-side. `orm.Of[UserRole](c).Find(uid, rid)` accepts variadic for composite.

### 3.6 No Tag-DSL Anywhere

Tags can carry transport-specific hints (`db:"-"` to exclude from a Medium) but no schema info — schema is the `Schema()` method, full stop. Agents read the method body and have full understanding of the table without parsing tag mini-languages.

---

## 4. Query API

Every bridge call returns `core.Result`. Errors flow through `Result.Value` when `OK == false`, per the canonical pattern (see §8 for stable error codes).

### 4.1 Reads

```go
// By primary key
res := orm.Of[User](c).Find(1)
if !res.OK { return res }
user, ok := core.Cast[User](res)

// Composite PK
res := orm.Of[UserRole](c).Find(uid, rid)

// First match (no PK), error if zero rows
res := orm.Of[User](c).Where("email", "=", "x@y").First()

// All matches
res := orm.Of[User](c).Where("active", "=", true).Get()
users, ok := core.Cast[[]User](res)

// Everything
res := orm.Of[User](c).All()

// Count
res := orm.Of[User](c).Where("active", "=", true).Count()
n, ok := core.Cast[int64](res)

// Aggregates
res := orm.Of[Order](c).Where("paid", "=", true).Sum("total")
res := orm.Of[Order](c).Where("paid", "=", true).Avg("total")
res := orm.Of[Order](c).Where("paid", "=", true).Min("total")
res := orm.Of[Order](c).Where("paid", "=", true).Max("total")

// Pagination
res := orm.Of[User](c).Order("created_at", "desc").Limit(20).Offset(40).Get()
```

### 4.2 Where Chains

```go
res := orm.Of[User](c).
    Where("active", "=", true).
    Where("tier", "in", []string{"pro", "plus"}).
    WhereNotNull("email_verified_at").
    OrWhere("admin", "=", true).
    Get()
```

**Operators (v1):** `=`, `!=`, `<`, `<=`, `>`, `>=`, `in`, `not in`, `like`, `null`, `not null`, `between`. Each produces a `Predicate` node in the Intent — Medium translates to its native dialect.

**Group syntax for OR:**

```go
res := orm.Of[User](c).
    Where("active", "=", true).
    WhereGroup(func(g *orm.Group) {
        g.Where("tier", "=", "pro")
        g.OrWhere("admin", "=", true)
    }).
    Get()
// → WHERE active = true AND (tier = 'pro' OR admin = true)
```

### 4.3 Relations — Eager Only, Never Lazy

```go
// Eager load via With
res := orm.Of[User](c).With("posts").Find(1)

// Nested With
res := orm.Of[User](c).With("posts.comments").Find(1)

// Relation as standalone query
res := orm.Of[User](c).Posts(user).Where("published", "=", true).Get()
// Sugar for: orm.Of[Post](c).Where("user_id","=",user.ID).Where("published","=",true).Get()
```

**No lazy loading anywhere, ever.** Accessing `user.Posts` on a struct that wasn't loaded with `With("posts")` returns the Go zero value (empty slice). No panic, no silent N+1, no hidden round trip. *Two Monks*: if you didn't carry it across the bridge, you don't have it.

### 4.4 Streams for Large Reads

```go
res := orm.Of[Event](c).Where("ts", ">", cutoff).Stream()
if !res.OK { return res }
seq, ok := core.Cast[core.Seq[Event]](res)
if !ok { return res }
for e := range seq {
    // process one row
}
```

Stream returns a `core.Seq[T]` backed by the Medium's native cursor where supported. For Mediums without cursors (e.g. KV scan), the bridge falls back to chunked reads — capability declared by the Medium (§5).

### 4.5 Writes — Explicit Bridge Calls, Always

```go
// Single insert/update — same call, branches on PK presence
res := orm.Save(c, &user)         // user.ID == 0 → INSERT; populated → UPDATE
res := orm.Save(c, &user, &post)  // batch: variadic, one Intent per type

// Typed-bridge form (equivalent to orm.Save when type is known)
res := orm.Of[User](c).Save(&user)

// Explicit insert (fail if PK already present)
res := orm.Insert(c, &user)

// Explicit update by predicate (no struct round-trip)
res := orm.Of[User](c).
    Where("active", "=", false).
    Where("last_login", "<", cutoff).
    Update(map[string]any{"archived": true})

// Delete
res := orm.Delete(c, &user)
res := orm.Of[User](c).Where("archived", "=", true).DeleteAll()
```

`Save` is the canonical write — Insert-or-Update branch becomes an UPSERT where the Medium supports it, two-step otherwise.

### 4.6 Transactions

`*core.Tx` (= `sql.Tx`) is the canonical transaction handle. Bridge calls accept it via `WithTx`:

```go
res := orm.Tx(c, func(tx *core.Tx) core.Result {
    r := orm.Of[User](c).WithTx(tx).Find(1)
    if !r.OK { return r }
    user, _ := core.Cast[User](r)
    user.Tier = "pro"
    return orm.Save(c, &user, orm.WithTx(tx))
})
```

`orm.Tx` opens a transaction on the Medium that owns the resolved schema. Cross-Medium transactions are not supported in v1 — bridge returns code `orm.tx.cross_medium` if a transaction body touches multiple Mediums.

For Mediums declaring `Capabilities.Transactions == false`, `orm.Tx` returns code `orm.tx.unsupported` immediately — no silent degradation (silent transaction-skipping corrupts data; never acceptable).

### 4.7 Input Coercion — The Security Spine

Every value crossing into a Predicate, Save Intent, or Update map passes through a Schema-declared *input shaper* before reaching Intent. The contract is **shape-or-reject** — what comes out matches the column's declared shape exactly, or the call returns a Result with `OK=false` and a stable code under `orm.input.*`. *How* each shaper achieves "shape-correct output" is its own choice:

- **Strict-validate** (UUID, IPv4, IPv6): no cleaning is meaningful — a value either matches the format or it doesn't.
- **Clean-then-validate** (Email, Slug, Time, String): actively shape the input towards the declared form — `input.Email("John Smith <john@host.uk>")` extracts `john@host.uk`; `input.Slug("Hello World!!")` returns `hello-world`; `input.Time("Jan 5 2026 14:30")` parses lenient formats and emits canonical `core.Time`. Cleaning is allowed, *misshapen output is not*.
- **Coerce** (when `.Coerce(fn)` is declared on the column): cross-type translation runs first — `false → 0`, `int → bool`, `core.Time → Unix int`, etc.

Whether a shaper cleans or strict-rejects, the security guarantee is the same: nothing reaches Intent that doesn't match the column's declared shape.

**Why this matters.** Go's situation is genuinely different from PHP's, and the difference is asymmetric:

- **At the SQL-generation seam (output)**, Go is naturally cleaner. Its smaller native type palette (`string` / `int` / `float64` / `bool` / `[]byte` / `time.Time`) means values are already typed by the time they reach `db.Exec(...)` — drivers parameterise them, no string-mashing-into-SQL like PHP forced you to defend against. PHP-style "the variable was a string but PHP treated it as an int in numeric context" doesn't exist here.
- **At the input seam (HTTP boundary)**, Go gives you nothing. POST bodies arrive as raw bytes; query strings hand you `string`s; JSON decoders produce `map[string]any` where every number was a string microseconds ago; multipart forms don't pretend types exist. Pure Go-to-Go code rarely sees `"1"` colliding with `1` — code arriving from an HTTP handler always does. *That* is where the threat surface lives, and where Schema-declared input shapers earn their keep.

Combined effect: every value crossing into a Predicate or Save Intent is coerced to its column's declared shape OR rejected. The bridge has *no path that skips coercion*. SQL injection becomes structurally impossible — *no string ever reaches SQL that wasn't already cast to its column's declared type, then parameterised at the driver*. Form content-type changes break shape → Schema rejects → Predicate never forms → query refused. The defensive discipline that made WorkStationCommerce PHP5 uncrackable, ported to Go's stricter type discipline plus the bonus that `.Coerce(input.X)` lets you opt INTO useful cross-type translations (like `false → 0`) when you actually want them.

#### Where coercion fires

```go
// 1. Predicate values — every Where()'s 3rd arg
res := orm.Of[User](c).Where("email", "=", userInput).Get()
//                                           ^^^^^^^^^ coerced via input.Email per Schema declaration
// If userInput is "'; DROP TABLE--" → Result{OK: false, Code: "orm.input.format"}

// 2. Save / Insert struct fields — every column in &user
res := orm.Save(c, &user)
// Each field of user coerced per Schema before forming WriteIntent

// 3. Update map values — every value in the map
res := orm.Of[User](c).Where("id", "=", uid).Update(map[string]any{
    "email": userInput,   // coerced via input.Email
    "tier":  tier,         // coerced via input.OneOf("free","pro","plus")
})
```

#### The shaper set (`input.go`)

Each shaper takes `any`, returns `core.Result` with the typed value or an `orm.input.*` error code. Reject-not-cleanse: bad input is rejected, not softened.

```go
func String(v any) core.Result      // any → string, rejects nil/non-string
func Email(v any) core.Result       // any → string, validates RFC 5322 shape
func URL(v any) core.Result         // any → string, validates URL shape
func UUID(v any) core.Result        // any → string, validates UUID shape
func IPv4(v any) core.Result        // any → string, validates IPv4
func IPv6(v any) core.Result        // any → string, validates IPv6
func Hostname(v any) core.Result    // any → string, validates hostname
func Slug(v any) core.Result        // any → string, [a-z0-9-]+
func Int(v any) core.Result         // any → int64; strict — only int-shaped types
func Bool(v any) core.Result        // any → bool; strict — only bool
func Float(v any) core.Result       // any → float64; strict
func Time(v any) core.Result        // any → core.Time; RFC 3339 strings or core.Time
func JSON(v any) core.Result        // any → []byte (canonical JSON)
func Bytes(v any) core.Result       // any → []byte
func Pattern(re string, v any) core.Result        // any → string, regex-validated
func OneOf(vals []any, v any) core.Result         // any → matched value or reject
func Min(n int64, v any) core.Result              // numeric/length minimum
func Max(n int64, v any) core.Result              // numeric/length maximum
```

#### Cross-Type Coercion — The Neat Tricks

Go doesn't auto-coerce between types, which is usually what you want. But sometimes you *do* want a bool stored as 0/1, or a string parsed as an int, or a Time as Unix seconds. Schema declares these explicitly via `.Coerce(fn)`:

```go
func BoolToInt(v any) core.Result    // accepts bool, returns int64 (false=0, true=1)
func IntToBool(v any) core.Result    // accepts int, returns bool (0=false, else=true)
func StringToInt(v any) core.Result  // accepts numeric string, returns int64
func IntToString(v any) core.Result  // accepts int, returns string
func StringToBool(v any) core.Result // accepts "true"/"false"/"1"/"0"/"yes"/"no"/"on"/"off"
func TimeToUnix(v any) core.Result   // accepts core.Time, returns int64 (Unix seconds)
func UnixToTime(v any) core.Result   // accepts int64, returns core.Time
```

Declared on a column via `.Coerce(fn)` modifier:

```go
b.Int("active").Coerce(input.BoolToInt)
// Now both directions work transparently:
//   orm.Save(c, &row{Active: true})           — stores 1
//   orm.Of[Row](c).Where("active","=",false)  — coerces to 0
```

`Coerce` runs *first* in the validation chain — type translation precedes semantic validation:

```
Coerce → type-shaper (String/Int/Bool/...) → NotNull → Format → Min/Max → Pattern → OneOf
```

Each coercer is opt-in per column; they don't apply implicitly. Go's "explicit is better than implicit" stance, with the convenience of declared coercion when you want it.
```

#### How the bridge dispatches shapers

Internal — not consumer-facing — but illustrates the contract:

```go
// In bridge.go — every Where call routes through input.Apply
func (b *Bridge[T]) Where(field, op string, value any) *Bridge[T] {
    schema := b.schema()
    f, ok := schema.FieldByName(field)
    if !ok {
        return b.invalid(core.NewCode("orm.input.field", "unknown field: "+field))
    }
    coerced := input.Apply(f, value)  // dispatches per Schema declaration
    if !coerced.OK {
        return b.invalid(coerced.Value.(error))
    }
    b.intent.Where = append(b.intent.Where, Predicate{Field: field, Op: op, Value: coerced.Value})
    return b
}
```

`input.Apply(f, v)` walks the field's declared type AND every modifier (`Format`, `Min`, `Max`, `Pattern`, `OneOf`, `NotNull`, `MaxBytes`) in declaration order — earlier layers must pass before later layers run. First failure wins; success returns the final coerced value.

#### Layered validation example

```go
b.String("email").NotNull().Format("email").Max(254).Pattern(`@host\.uk$`)
// On Where("email", "=", v):
// 1. input.String(v)        — reject non-string                  → orm.input.type
// 2. NotNull()              — reject empty/nil                    → orm.input.null
// 3. input.Email(v)         — RFC 5322 shape                      → orm.input.format
// 4. input.Max(254, v)      — RFC 5321 maximum length             → orm.input.range
// 5. input.Pattern(re, v)   — additional application constraint   → orm.input.pattern
```

Each layer's distinct error code lets callers grep specifics:

```go
res := orm.Of[User](c).Where("email", "=", userInput).Get()
if !res.OK {
    switch res.Code() {
    case "orm.input.format":  // tell user "that's not an email"
    case "orm.input.range":   // tell user "too long"
    case "orm.input.pattern": // tell user "must be a host.uk address"
    }
}
```

#### Architectural property

The bridge has **no path that skips coercion**. Opting out is impossible — every entry point (Where, Save, Update, Insert) routes through `input.Apply` before forming Intent. Future extraction of `input.go` to a sibling `dappco.re/go/input` package post-v1 makes the same primitive available to core/api request bodies, core/php-form fields, and any other input-receiving primitive — one sanitiser, every consumer, the same defensive posture.

### 4.8 Output Rehydration — The Symmetric Seam

Writes flatten into canonical storage; reads rehydrate back to typed Go. Same Schema declaration, both directions, same "shape-or-reject" guarantee. This is the consumption-side dual of §4.7 — the bridge enforces the column's declared shape on the way out just as strictly as on the way in.

**Why this matters.** A Medium gives back rows in *its* native shape, not necessarily the consumer's expected shape:

- DuckDB / Postgres return native bool; SQLite returns int 0/1; the bridge normalises to Go `bool`
- DuckDB / Postgres return `time.Time`; an HTTP-RPC Medium returns ISO-8601 string; a unix-time-store returns int64 seconds — the bridge normalises to `core.Time`
- A JSON column comes back as text or `[]byte` — the bridge unmarshals to the Schema-declared inner type
- An encrypted column comes back as ciphertext bytes — the bridge decrypts to plaintext per Schema-declared `Crypt(...)` modifier
- A Medium that flattens everything to strings (JSON-RPC, Borg.DataNode wire format) needs each typed column rehydrated according to Schema

If the bridge didn't enforce shape on read, the consumer would have to know which Medium they were talking to — and the "everything is Medium" promise leaks. Shape enforcement on the read seam is what keeps the abstraction honest.

#### Shapers are bidirectional from day one

Every shaper has two methods. Most shapers' `Rehydrate` is identity (stored email IS canonical email). Asymmetric shapers (Time, JSON, Money, encrypted) implement both halves explicitly.

```go
// In input.go (or rename to shapers.go) — same package
type Shaper interface {
    Coerce(v any) core.Result      // INPUT direction: external → declared shape
    Rehydrate(v any) core.Result   // OUTPUT direction: stored shape → typed Go
}

// Symmetric shapers — Rehydrate is identity:
//   Email, UUID, Slug, IPv4, IPv6, Hostname, Pattern, OneOf, String, Bool, Int, Float

// Asymmetric shapers — both halves explicit:
//   Time   (wire ↔ core.Time; lenient parse on input, canonical core.Time on output)
//   JSON   (struct ↔ canonical JSON bytes; marshal on input, unmarshal on output)
//   Bytes  (any ↔ []byte; with size limits on both directions)
//   Money  (cents int ↔ a consumer-defined Money struct via .Coerce + .Rehydrate — illustrative; orm ships no built-in money type in v1)
//   Crypt  (plaintext ↔ ciphertext bytes; encrypt on input, decrypt on output)
//   BoolToInt and friends (cross-type Coerce — Rehydrate is the inverse function)
```

#### Where rehydration fires

```go
// 1. Reading a row — every column rehydrated per Schema before populating struct
res := orm.Of[User](c).Find(1)
user, _ := core.Cast[User](res)
// user.CreatedAt is core.Time regardless of Medium wire format
// user.Profile  is the typed JSON-decoded struct, not raw bytes
// user.Active   is bool regardless of SQLite int storage

// 2. Reading a slice — same per-row, per-column rehydration
res = orm.Of[User](c).Where("active","=",true).Get()
users, _ := core.Cast[[]User](res)

// 3. Streaming — rehydration happens as the iterator yields each row
res = orm.Of[Event](c).Stream()
seq, _ := core.Cast[core.Seq[Event]](res)
for e := range seq {
    // e.Payload (JSON column) already unmarshalled to typed struct
}

// 4. Aggregates — typed-correct return values
res = orm.Of[Order](c).Sum("total")
total, _ := core.Cast[int64](res)   // not a string from a JSON medium

// 5. Returning rows from a Save (RETURNING clause where Medium supports it)
res = orm.Save(c, &user)   // user.ID populated, types-correct
```

#### Rehydration error codes

Same `orm.input.*` keyspace mirrored under `orm.output.*` for failures during rehydration:

```
orm.output.type      — Medium returned a value the shaper can't rehydrate to declared type
orm.output.format    — Medium-supplied value violates declared Format on rehydration
orm.output.coerce    — .Rehydrate(fn) shaper rejected the stored value
orm.output.crypt     — decryption failed (key, padding, or wire corruption)
orm.output.json      — JSON column unmarshal failed
```

The bridge surfaces these as `Result.OK == false` like any other failure — caller forwards or handles via `r.Code()`. A shape-failed read never fills a struct field with the wrong type silently.

#### Locale awareness — bridge does *normalisation*, display does *localisation*

A core.Time read from a Postgres column comes back UTC. The bridge gives the consumer `core.Time` — that's the canonical typed shape. The consumer's display layer (CoreGUI, CoreTS UI, an HTTP response renderer) applies locale rules (timezone display, date format, currency formatting). The bridge's rehydration job ends at *typed canonical Go*; locale rendering is a higher concern.

Schema can declare hints for downstream renderers:

```go
b.Time("created_at").TZ("UTC").Render("rfc3339")
b.Money("total", "GBP").Render("symbol-then-amount")
```

These Render hints travel with the Schema's JSON shape, polyglot consumers honour them. The bridge itself ignores Render — it stops at typed canonical.

#### Architectural property

The bridge has **no path that skips rehydration**. Every read path (`Find`, `First`, `Get`, `All`, `Stream`, `Sum/Count/Avg/Min/Max`, `RETURNING` from Save/Insert/Update) routes through `output.Apply(schema, row)` before yielding to the consumer. The same defensive discipline that protects writes protects reads — a Medium can't sneak a wrong-shape value into a consumer struct.

### 4.9 Service-Form Consumption

The bridge ships in two equally-canonical consumption modes — both wrap the *same internal bridge*; neither is privileged. The pkg-form (`orm.Of[T](c)`) is what every section above this has shown. Service-form mounts the bridge inside `core.Service` so its verbs become `core.Action` registrations — auto-cross-exposed via core/cli, core/mcp, core/api, and any Conclave runtime that consumes the Action registry.

```go
package orm

// Register satisfies the canonical service.go pattern (see go-i18n / go-process).
// Registers the Service in c.Service("orm") and wires every bridge verb as an Action.
//
//   r := orm.Register(c)
//   if !r.OK { return r }
func Register(c *core.Core) core.Result

// Service is the canonical OrmService — registered once per Core, exposes
// every bridge verb as a typed core.Action handler.
type Service struct { /* internal */ }
```

**Actions registered** (DTO inputs typed per RFC §13 Conventions; output Result.Value wraps Payload as in §4.14):

| Action | Input DTO | Output |
|--------|-----------|--------|
| `orm.find` | `{Table string, PK []any, Tx *core.Tx}` | row (typed via Schema rehydration) |
| `orm.first` | `{Table string, Where []Predicate, Tx *core.Tx}` | row |
| `orm.get` | `{Table string, Where []Predicate, With []string, Order []OrderBy, Limit, Offset int, Tx *core.Tx}` | rows |
| `orm.all` | `{Table string, Tx *core.Tx}` | rows |
| `orm.count` | `{Table string, Where []Predicate, Tx *core.Tx}` | int64 |
| `orm.aggregate` | `{Table string, Op AggregateOp, Where []Predicate, Tx *core.Tx}` | scalar |
| `orm.save` | `{Table string, Rows []any, Tx *core.Tx}` | write meta |
| `orm.insert` | `{Table string, Rows []any, Tx *core.Tx}` | write meta |
| `orm.delete` | `{Table string, Rows []any, Tx *core.Tx}` | write meta |
| `orm.update` | `{Table string, Where []Predicate, Set map[string]any, Tx *core.Tx}` | write meta |
| `orm.delete_all` | `{Table string, Where []Predicate, Tx *core.Tx}` | write meta |
| `orm.tx` | `{Body string}` (named action to run inside tx) | tx outcome |
| `orm.watch` | `{Table string, Where []Predicate}` | stream handle (§4.11) |
| `orm.search` | `{Table string, Query string, Opts SearchOpts}` | ranked rows (§4.12) |
| `orm.ddl` | `{Schema Schema, Dialect string}` | string |
| `orm.diff` | `{Schema Schema}` | []Change |
| `orm.apply` | `{Changes []Change}` | apply meta |
| `orm.mount` | `{Name string, MediumRef string}` | OK |
| `orm.mount_schemas` | `{Prefix string}` | count |

**Both consumption modes work simultaneously** against the same Mounted Mediums and the same schema cache:

```go
// Pkg-form (typed, ergonomic)
res := orm.Of[User](c).Find(1)

// Service-form (action-dispatched, IPC-friendly, language-agnostic)
res := c.Action("orm.find").Run(core.NewOptions(
    core.Option{Key: "table", Value: "users"},
    core.Option{Key: "pk", Value: []any{1}},
))
```

The service-form is what makes core/orm a Conclave citizen — every action is callable from PHP via core/api, from TS via core/mcp, from another Go process via IPC, and from CLI via core/cli, *without core/orm needing to know about any of those consumers*. Action registration is the contract; everything downstream auto-binds via the canonical service.go pattern.

**Action handlers must accept typed DTO structs**, never loose `core.Options` (per `go/CLAUDE.md` — DTO Pattern). The DTO struct is the contract; the SDK generators (CoreTS, CorePHP) produce typed clients from the Go struct definitions. Loose Options handlers fail the AX audit.

### 4.10 Schema Mounting via `c.Data()`

Schemas can be published as embedded JSON assets in any package, mounted into `c.Data()`, and consumed cross-package without source-level dependency. The path makes core/orm a polyglot consumer: a PHP-native package can ship its schemas as JSON, a Go consumer mounts them, queries them via `orm.OfTable`.

```go
//go:embed schemas/*.json
var schemasFS embed.FS

func init() {
    // Producer-side: mount schemas as data
    c.Data().New(core.NewOptions(
        core.Option{Key: "name", Value: "myapp/schemas"},
        core.Option{Key: "source", Value: schemasFS},
        core.Option{Key: "path", Value: "schemas"},
    ))
}

// Consumer-side: pull every schema under the data prefix into the cache.
//
//   r := orm.MountSchemas(c, "myapp/schemas")
//   if !r.OK { return r }
//   count := core.MustCast[int](r)
func MountSchemas(c *core.Core, prefix string) core.Result
```

Each `.json` entry under the prefix is parsed via `SchemaFromJSON`, registered in the schema cache keyed by its declared `name` (table name).

#### Type-erased queries via `OfTable`

When the consumer doesn't have the Go type at compile time (cross-language consumption, dynamic queries from config, generated SDK code), use the table-keyed variant:

```go
// OfTable returns a DynamicBridge keyed by table name. Same fluent surface
// as Of[T], but rows come back as map[string]any with each value rehydrated
// per the Schema's column declarations.
//
//   res := orm.OfTable(c, "users").Find(1)
//   row := core.MustCast[map[string]any](res)
//   email := row["email"].(string)
func OfTable(c *core.Core, table string) *DynamicBridge

// DynamicBridge is the type-erased bridge — same chainable verbs as Bridge[T],
// but no compile-time type. Rehydration still applies; map values are typed
// per Schema declaration (Time fields come back as core.Time, not string).
type DynamicBridge struct { /* internal */ }
```

The shape guarantee from §4.8 still holds — every column comes back rehydrated. The only loss is compile-time type checking on the row variable.

**Polyglot use case:** core/php-myapp publishes its User schema as JSON in `c.Data()`. A Go service in the same Conclave runtime calls `orm.MountSchemas(c, "core/php-myapp/schemas")` once, then runs `orm.OfTable(c, "users").Where("active","=",true).Get()` — without importing any PHP-side code, without code-generation, without keeping a parallel Go struct in sync.

### 4.11 Reactive Subscriptions (`Watch`)

`Watch` returns a stream of change events for rows matching the bridge's predicate state. The stream begins with `OpInitial` events (snapshot of currently-matching rows) and continues with live `OpInsert`/`OpUpdate`/`OpDelete` events as they occur. Stream closes when the consumer's `core.Context` is cancelled or when the underlying Medium subscription ends.

```go
// Watch starts a change-data-capture subscription scoped to the bridge's
// current predicate state. Returns core.Seq[Event[T]].
//
//   ctx, cancel := core.WithCancel(c.Context())
//   defer cancel()
//   res := orm.Of[User](c).Where("active","=",true).Watch(ctx)
//   if !res.OK { return res }
//   seq := core.MustCast[core.Seq[orm.Event[User]]](res)
//   for ev := range seq {
//       switch ev.Op {
//       case orm.OpInitial: handleSnapshot(ev.After)
//       case orm.OpInsert:  handleInsert(ev.After)
//       case orm.OpUpdate:  handleUpdate(ev.Before, ev.After)
//       case orm.OpDelete:  handleDelete(ev.Before)
//       }
//   }
func (b *Bridge[T]) Watch(ctx core.Context) core.Result

// Event[T] is a single observed change.
type Event[T any] struct {
    Op     WatchOp
    Before T            // populated on Update / Delete; zero on Insert / Initial
    After  T            // populated on Insert / Update / Initial; zero on Delete
    Time   core.Time
    Source string       // medium identifier
}

type WatchOp int

const (
    OpInitial WatchOp = iota   // snapshot replay (yielded once per matching row at start)
    OpInsert                    // new row matches predicate
    OpUpdate                    // row updated; predicate may have begun or ended matching
    OpDelete                    // row deleted; was matching predicate before deletion
)
```

#### Capability-aware behaviour

Two capability flags govern Watch behaviour:

```go
type Capabilities struct {
    // ...
    Watch     bool   // server-side change-data-capture (Postgres LISTEN/NOTIFY, MongoDB streams, etc.)
    WatchPoll bool   // bridge can poll-fall-back when CDC unavailable
}
```

| Caps | Behaviour |
|------|-----------|
| `Watch == true` | Native CDC subscription; sub-second latency |
| `Watch == false && WatchPoll == true` | Bridge polls the Where predicate at `WatchPollInterval` (default 1s, settable via `.WatchPoll(d)` chain modifier); diff-emits Insert/Update/Delete events from successive snapshots |
| `Watch == false && WatchPoll == false` | Returns `orm.watch.unsupported` |

#### Filtering and snapshot-vs-live

The bridge's accumulated `Where` chain filters both the snapshot and live events. An Update that takes a row OUT of the predicate set emits as `OpUpdate` with `After == zero` (consumer must check); an Update that brings a row INTO the set emits as `OpInsert`. This shape makes "set membership change" trivial to consume.

`.Live()` chain modifier suppresses initial snapshot — only live events emitted:

```go
res := orm.Of[Event](c).Where("ts", ">", core.Now()).Live().Watch(ctx)
// no replay, only events from now forward
```

Memium implements `Watch == true` natively — every `Insert`/`Update`/`Delete` against the in-memory rows broadcasts to all matching subscribers via in-process channel. Snapshot replay reads current matching rows on subscribe.

### 4.12 Search Verb

`Search` runs a backend-dependent ranked query against fields declared `.Searchable(...)` in the Schema. Three modes — text, vector, hybrid — gated by Medium capability.

```go
// Search runs a ranked search over fields the Schema declared Searchable.
//
//   res := orm.Of[Document](c).Search("eloquent monks", orm.SearchOpts{
//       Limit: 20, Highlight: true,
//   })
//   if !res.OK { return res }
//   results := core.MustCast[[]orm.Ranked[Document]](res)
//   for _, r := range results {
//       core.Println(r.Score, r.Highlights["title"], r.Value.Title)
//   }
func (b *Bridge[T]) Search(query string, opts SearchOpts) core.Result

type SearchOpts struct {
    Limit     int
    Offset    int
    Highlight bool        // request highlighted snippets where supported
    Facets    []string    // request facet aggregations on these fields
    Mode      string      // "text" | "vector" | "hybrid"; empty = capability-determined
    Vector    []float32   // populated when Mode == "vector" or "hybrid"
}

type Ranked[T any] struct {
    Value      T
    Score      float64
    Highlights map[string]string  // field → highlighted snippet
    Distance   float64            // for vector mode (1.0 = identical, 0.0 = orthogonal)
    Facets     map[string]map[string]int  // facet field → value → count (when requested)
}
```

#### Schema modifiers

```go
b.String("title").Searchable("text")                      // full-text indexed
b.JSON("body").Searchable("text")                         // full-text indexed (text extracted from JSON)
b.Bytes("embedding").Searchable("vector", 1536)            // vector indexed; dimension fixed at 1536
b.String("description").Searchable("hybrid")               // both text + vector if Medium supports
```

The dimension on vector columns is part of the Schema contract — Mediums reject inserts that don't match.

#### Capability gates

```go
type Capabilities struct {
    // ...
    Search SearchCaps
}

type SearchCaps struct {
    Text   bool   // text search
    Vector bool   // vector search
    Hybrid bool   // hybrid (text + vector reranking)
    Facets bool   // facet aggregation
}
```

When `Mode` is unset and the Medium has both Text and Vector, the bridge defaults to Hybrid if available, otherwise Text. Explicit `Mode == "vector"` against a non-vector Medium returns `orm.search.unsupported`.

Memium implements basic case-insensitive substring text search (no vectors in v1; vector work belongs to a Medium with proper indexing). This is enough for unit-testing the bridge surface; production use requires a real Medium.

### 4.13 Aliased Queries — The Relational-Algebra Foundation

Aliases declare *who* participates in a query; predicates declare *how* they relate. The JOIN-vs-WHERE conceptual split disappears, and the same vocabulary scales recursively to outer joins, subqueries, CTEs, and set operations.

```go
// A is the alias map type — declares which tables (or sub-references) participate.
//
//   orm.A{"u": "users", "p": "posts"}
//   orm.A{"u": "users", "p": orm.LeftJoin("posts")}
//   orm.A{"u": "users", "x": orm.Sub(otherBuilder)}
type A map[string]any   // value: string | JoinSpec | SubRef

// Col wraps a column reference so the bridge distinguishes "compare to a column"
// from "compare to a literal". This is the Go-side discriminator that PHP's
// MySQL/PHP5 lib achieved via runtime string-vs-array type inspection.
//
//   .Where("u.id", "=", orm.Col("p.user_id"))   // join condition
//   .Where("u.email", "=", "snider@x")           // literal compare
func Col(name string) ColRef
type ColRef struct { Name string }

// JoinSpec wraps a table name with a join kind for outer joins.
type JoinSpec struct {
    Table string
    Kind  JoinKind
}

type JoinKind int
const (
    JoinInner JoinKind = iota
    JoinLeft
    JoinRight
    JoinFull
)

func LeftJoin(table string) JoinSpec   // returns JoinSpec{table, JoinLeft}
func RightJoin(table string) JoinSpec
func FullJoin(table string) JoinSpec

// SubRef wraps a sub-builder as an aliased subquery participant.
//
//   sub := orm.Of[Post](c).Where("published","=",true)
//   res := orm.Of[User](c).
//       From(orm.A{"u": "users", "p": orm.Sub(sub)}).
//       Where("u.id", "=", orm.Col("p.user_id")).
//       Get()
func Sub(b BridgeRef) SubRef
type SubRef struct { Builder BridgeRef }

// BridgeRef is the type-erased bridge interface — any *Bridge[T] satisfies it.
type BridgeRef interface { intent() ReadIntent }
```

#### Bridge surface

```go
// From declares query aliases — moves the bridge into multi-table mode.
//
//   res := orm.Of[User](c).
//       From(orm.A{"u": "users", "p": "posts"}).
//       Where("u.id", "=", orm.Col("p.user_id")).
//       Where("p.published", "=", true).
//       Where("u.active", "=", true).
//       Get()
func (b *Bridge[T]) From(a A) *Bridge[T]
```

#### Set operations

```go
// Set operations combine entire builders.
//
//   res := orm.Union(
//       orm.Of[User](c).Where("active","=",true),
//       orm.Of[User](c).Where("admin","=",true),
//   ).Order("email").Get()
func Union(builders ...BridgeRef) BridgeRef
func Intersect(builders ...BridgeRef) BridgeRef
func Except(builders ...BridgeRef) BridgeRef
```

#### Intent extension

```go
type ReadIntent struct {
    // ...existing fields...
    Alias map[string]any  // alias → string | JoinSpec | SubRef
    SetOp *SetOp          // populated for UNION / INTERSECT / EXCEPT
}

type SetOp struct {
    Kind     SetOpKind  // Union | Intersect | Except
    Builders []ReadIntent
}

type SetOpKind int
const (
    SetUnion SetOpKind = iota
    SetIntersect
    SetExcept
)
```

#### Capability gates

```go
type Capabilities struct {
    // ...
    Aliases    bool      // From(A{...}) with aliased predicates
    Subqueries bool      // Sub(...) participants
    SetOps     bool      // UNION / INTERSECT / EXCEPT
    JoinKinds  JoinCaps  // which join kinds the Medium supports
}

type JoinCaps struct {
    Inner bool
    Left  bool
    Right bool
    Full  bool
}
```

Mediums lacking `Aliases` reject any `From(A{})` call with `orm.aliases.unsupported`. Mediums with `Aliases == true` but missing a specific JoinKind reject that kind specifically (`orm.join.unsupported`). The bridge has no client-side join fallback for SQL-shaped Mediums in v1 — joining is a server-side concern (cross-Medium join is firmly out-of-scope; that's a v2 problem).

Memium implements the full alias set with all four join kinds via linear-scan client-side joining (acceptable for in-memory test data; not a production approach).

#### Why this is the bedrock

The PHP5 query class this pattern came from wasn't a query builder — it was a *relational-algebra interpreter* that happened to emit SQL. Same is true here. Once aliases-plus-column-refs is the unit of composition:

- **Outer joins** = join-kind metadata on the alias declaration; no new verbs
- **Subqueries** = a sub-builder is just another aliased participant; nesting composes
- **CTEs** = same shape, different scope; a v1.1 modifier on `Sub()` declares CTE-binding
- **Set operations** = predicates over named result-sets

One vocabulary, every level of complexity. This is what core/orm has to be, not just what it could optionally be.

### 4.14 Result.Value Payload Shape

Bridge wraps return values in an internal `Payload` so meta travels alongside data:

```go
// Internal — orm uses this to thread Meta through Result.Value
type Payload struct {
    Data any   // User, []User, int64, core.Seq[T], etc.
    Meta Meta
}

// core.Cast[T] knows to unwrap .Data automatically
user, ok := core.Cast[User](res)

// orm.Detail returns both data and meta when meta matters
user, meta, ok := orm.Detail[User](res)
```

`core.Result` stays virgin (no Meta method, no struct change). Meta lives inside Value via the `orm.Payload` wrapper, accessed only by orm helpers.

```go
type Meta struct {
    Medium   string         // medium identifier (e.g. "duckdb", "memium")
    Duration core.Duration  // total wall-clock for the dispatch
    RowsRead int64
    RowsWrit int64
    Degraded string         // "" if Medium honoured Intent fully; populated when bridge degraded gracefully (see §6)
    LastID   any            // populated on Insert
}
```

---

## 5. The Medium Contract

Every Medium that participates implements:

```go
// Medium is the seam between the bridge and a backend.
//
//   var m Medium = &SQLMedium{db: db}
//   res := m.Read(ctx, intent)
type Medium interface {
    // Caps reports what this Medium can honour server-side.
    Caps() Capabilities

    // Read dispatches a ReadIntent and returns rows.
    //
    //   res := m.Read(ctx, ReadIntent{Schema: s, PK: []any{1}})
    Read(ctx core.Context, in ReadIntent) core.Result

    // Write dispatches a WriteIntent (Insert/Update/Save/Delete).
    //
    //   res := m.Write(ctx, WriteIntent{Op: Save, Rows: []any{&user}})
    Write(ctx core.Context, in WriteIntent) core.Result

    // Stream dispatches a ReadIntent and returns core.Seq[T].
    //
    //   res := m.Stream(ctx, ReadIntent{Schema: s, Where: preds})
    Stream(ctx core.Context, in ReadIntent) core.Result

    // Watch dispatches a ReadIntent with WatchOpts and returns core.Seq[Event[T]].
    // Required iff Caps().Watch || Caps().WatchPoll. (§4.11)
    Watch(ctx core.Context, in ReadIntent) core.Result

    // Search dispatches a ReadIntent with SearchSpec and returns []Ranked[T].
    // Required iff any sub-cap of Caps().Search is true. (§4.12)
    Search(ctx core.Context, in ReadIntent) core.Result
}
```

### 5.1 Capabilities

```go
type Capabilities struct {
    Predicates   PredicateCaps  // operators honoured server-side
    Joins        bool           // native JOIN (legacy single-table With(...) eager-load)
    Transactions bool           // BEGIN/COMMIT/ROLLBACK
    Aggregates   bool           // COUNT/SUM/MIN/MAX/AVG server-side
    Cursor       bool           // server-side cursor for Stream
    Introspect   bool           // schema reflection (for migrations)

    // Advanced surfaces (RFC §4.9–§4.13)
    Watch        bool           // server-side change-data-capture
    WatchPoll    bool           // bridge can poll-fall-back for Watch
    Search       SearchCaps     // text/vector/hybrid search (§4.12)
    Aliases      bool           // From(A{...}) multi-table queries (§4.13)
    Subqueries   bool           // Sub(...) participants
    SetOps       bool           // UNION / INTERSECT / EXCEPT
    JoinKinds    JoinCaps       // which join kinds the Medium supports server-side
}

type PredicateCaps struct {
    Equality   bool  // = !=
    Comparison bool  // < <= > >=
    In         bool  // IN / NOT IN
    Like       bool  // LIKE / NOT LIKE
    Null       bool  // IS NULL / IS NOT NULL
    Between    bool  // BETWEEN x AND y
}

type SearchCaps struct {
    Text   bool   // text search (full-text indexing)
    Vector bool   // vector search (embedding similarity)
    Hybrid bool   // hybrid (text + vector reranking)
    Facets bool   // facet aggregation
}

type JoinCaps struct {
    Inner bool
    Left  bool
    Right bool
    Full  bool
}
```

### 5.2 Intents — Pure Data

```go
// ReadIntent is the typed payload sent to Medium.Read / Stream.
//
//   intent := ReadIntent{Schema: s, PK: []any{1}}
type ReadIntent struct {
    Schema    Schema
    PK        []any           // for Find with PK values
    Where     []Predicate     // ANDed at top level
    With      []string        // eager relations
    Order     []OrderBy
    Limit     int
    Offset    int
    Aggregate *AggregateOp    // optional Count/Sum/etc.
    Tx        *core.Tx        // optional transaction
    Alias     map[string]any  // alias → string | JoinSpec | SubRef (§4.13)
    SetOp     *SetOp          // populated for UNION / INTERSECT / EXCEPT (§4.13)
    Watch     *WatchOpts      // populated when produced by .Watch() (§4.11)
    Search    *SearchSpec     // populated when produced by .Search() (§4.12)
    Strict    bool            // .Strict() chain modifier — fail on degradation
}

type WatchOpts struct {
    PollInterval core.Duration  // 0 = use Medium's native CDC; non-zero = poll-fallback
    Live         bool           // suppress initial snapshot
}

type SearchSpec struct {
    Query     string
    Mode      string     // "text" | "vector" | "hybrid"
    Vector    []float32
    Limit     int
    Offset    int
    Highlight bool
    Facets    []string
}

type SetOp struct {
    Kind     SetOpKind
    Builders []ReadIntent
}

type SetOpKind int
const (
    SetUnion SetOpKind = iota
    SetIntersect
    SetExcept
)

type WriteIntent struct {
    Op       WriteOp           // Insert | Update | Save | Delete
    Schema   Schema
    Rows     []any             // structs to write
    Where    []Predicate       // for predicate-based update/delete
    Updates  map[string]any    // for explicit-map update
    Tx       *core.Tx
}

type Predicate struct {
    Field string
    Op    string                // "=", "<", "in", "like", ...
    Value any
    Group []Predicate           // nested OR-group when non-empty
    OR    bool                  // join with previous via OR (default AND)
}

type OrderBy struct {
    Field string
    Desc  bool
}

type AggregateOp struct {
    Op    string  // "count" | "sum" | "min" | "max" | "avg"
    Field string  // empty for Count
}

type WriteOp int

const (
    OpInsert WriteOp = iota
    OpUpdate
    OpSave
    OpDelete
)
```

Pure-data Intent means: serialise to JSON for cross-process dispatch, replay for testing, log for audit, route to a remote Medium over RPC. The bridge is a *function* from caller chain → Intent → Medium dispatch.

### 5.3 Medium Resolution

A Medium is registered at consumer init time:

```go
// In consumer code, before first orm.Of[T](c) call:
mySQL := orm_sql.New(c, "duckdb", "file:./data.db")
orm.Mount(c, "default", mySQL)
```

The bridge resolves the Medium by name, defaulting to `"default"`. Per-call override:

```go
res := orm.Of[User](c).On("warehouse").Get()
// dispatches against the Medium registered as "warehouse"
```

Internal storage: `core.Registry[Medium]` keyed by name. Sealed after first dispatch.

---

## 6. Capability-Aware Degradation

When the resolved Medium lacks a capability the Intent requires, the bridge has three modes per dimension:

| Missing Capability | Default Behaviour | Caller Override |
|--------------------|-------------------|-----------------|
| `Predicates.Like` (or any predicate) | Fetch broader set, filter client-side, set `Meta.Degraded = "client_filter:like"` | `.Strict()` → fail with `orm.predicate.unsupported` |
| `Joins` (with `With(...)`) | Issue follow-up reads per relation (caller asked for `With`, gets the data) | `.With("posts", orm.NoFallback)` → fail with `orm.joins.unsupported` |
| `Aggregates` | Stream rows, compute client-side, set `Meta.Degraded = "client_aggregate:sum"` | `.Strict()` → fail with `orm.aggregate.unsupported` |
| `Transactions` | **HARD FAIL.** Return `orm.tx.unsupported`. No silent degradation. | (none — silence here corrupts data) |
| `Cursor` (with `Stream()`) | Chunked reads under the hood; bridge synthesises `core.Seq[T]` | (none — degradation is invisible to caller) |
| `Introspect` (for migrations) | Skip schema-diff features for this Medium | (none — caller must declare DDL manually) |
| `Watch == false && WatchPoll == true` | Bridge polls Where predicate at `.WatchPoll(d)` interval; diff-emits Insert/Update/Delete from successive snapshots | `.WatchPoll(0)` requires native CDC; missing it → `orm.watch.unsupported` |
| `Watch == false && WatchPoll == false` | **HARD FAIL.** Return `orm.watch.unsupported`. | (none) |
| `Search.Text/Vector/Hybrid` (sub-cap missing for requested Mode) | **HARD FAIL.** Return `orm.search.unsupported`. Search semantics don't degrade safely. | (none) |
| `Aliases` (with `From(A{})`) | **HARD FAIL.** Return `orm.aliases.unsupported`. Cross-Medium client-side joining is firmly out-of-scope in v1. | (none) |
| `JoinKinds.Left/Right/Full` (specific kind missing) | **HARD FAIL.** Return `orm.join.unsupported` with the specific kind in the message. | (none) |
| `Subqueries` (with `Sub(...)`) | **HARD FAIL.** Return `orm.subquery.unsupported`. | (none) |
| `SetOps` (with `Union/Intersect/Except`) | **HARD FAIL.** Return `orm.setop.unsupported`. | (none) |

Caller can opt into stricter handling at the call site:

```go
res := orm.Of[User](c).
    With("posts", orm.NoFallback).
    Where("name", "like", "%snider%").Strict().
    Get()
```

`.Strict()` upgrades any silent client-side degradation in the chain to a hard failure with the corresponding `orm.X.unsupported` code.

---

## 7. Migrations

Migrations are emitted *from* schema, applied via the SQL Medium's `core.SQLOpen → *core.DB` handle. v1 is deliberately light:

### 7.1 DDL Emission

```go
// Emit CREATE TABLE for a single schema
res := orm.DDL(c, User{}.Schema(), "duckdb")
// Result.Value is string — the dialect-specific CREATE TABLE
```

The Medium's SQL dialect is selected via the second argument; the SQL Medium implements `Dialect() string` to identify itself. Supported dialects in v1: `duckdb`, `sqlite`, `postgres`, `mariadb`.

### 7.2 Schema Diff (Introspect-Capable Mediums Only)

```go
// Compare current Medium state against declared schema
res := orm.Diff(c, User{}.Schema())
// Result.Value is []orm.Change — additive operations, never destructive
```

`orm.Diff` returns a list of `Change` records (AddColumn, AddIndex, AddTable). It NEVER emits drops or destructive renames. Destructive migrations are an explicit caller decision, not an automated diff.

### 7.3 Apply

```go
res := orm.Apply(c, []Change{...})
```

Applies changes within a transaction (where the Medium supports it). Records applied changes in a Medium-internal `_orm_migrations` table — schema-versioned, not file-versioned. The migration table's own schema is bootstrapped on first Apply.

### 7.4 Out of Scope for v1

- Down-migrations (rollbacks)
- Cross-Medium migrations
- Data transformations (column type changes that need data conversion)
- Migration squashing
- Production safety guards (locking, online-DDL strategies)

These belong in a sibling `core/orm-migrate` package or a higher-level tool — not in the bridge.

---

## 8. Error Codes

ORM errors are constructed via `core.NewCode(code, msg)` or wrapped via `core.WrapCode(err, code, op, msg)`. Stable codes form a flat keyspace agents grep on:

| Code | Meaning |
|------|---------|
| `orm.notfound` | `Find` / `First` matched zero rows |
| `orm.ambiguous` | `First` matched more than one (some Mediums) |
| `orm.schema.missing` | Type does not implement `Modelled` (no `Schema()` method found via reflection) |
| `orm.schema.invalid` | Schema declared but malformed (e.g., no PK, conflicting field types) |
| `orm.medium.notmounted` | Named Medium not registered with `orm.Mount` |
| `orm.medium.unsupported` | Medium doesn't recognise the Intent shape |
| `orm.predicate.unsupported` | `.Strict()` chain hit a predicate the Medium can't honour |
| `orm.joins.unsupported` | `.NoFallback` `With(...)` hit a Medium without `Capabilities.Joins` |
| `orm.aggregate.unsupported` | `.Strict()` chain hit an aggregate the Medium can't compute |
| `orm.tx.unsupported` | `orm.Tx` against a Medium without `Capabilities.Transactions` |
| `orm.tx.cross_medium` | Transaction body touched multiple Mediums (not supported in v1) |
| `orm.constraint` | Unique / null / check violation; wraps Medium's native error |
| `orm.conflict` | Optimistic-lock / version-mismatch on Save |
| `orm.cast` | Caller's `core.Cast[T]` couldn't unwrap `Result.Value` |
| `orm.input.field` | Predicate referenced a field not declared in Schema |
| `orm.input.type` | Value couldn't be coerced to the column's declared type (e.g., string passed where Bool expected) |
| `orm.input.null` | Required field (`NotNull()`) received nil/empty |
| `orm.input.format` | Value failed `Format(...)` validation (email, url, uuid, etc.) |
| `orm.input.range` | Value failed `Min` / `Max` bounds |
| `orm.input.pattern` | Value failed `Pattern(re)` regex validation |
| `orm.input.oneof` | Value not in `OneOf(...)` enum set |
| `orm.input.coerce` | `.Coerce(fn)` shaper rejected the value |
| `orm.output.type` | Medium returned a value the shaper can't rehydrate to declared type |
| `orm.output.format` | Medium-supplied value violates declared Format on rehydration |
| `orm.output.coerce` | `.Rehydrate(fn)` shaper rejected the stored value |
| `orm.output.crypt` | Decryption failed during rehydration (key mismatch, padding, wire corruption) |
| `orm.output.json` | JSON column unmarshal failed during rehydration |
| `orm.service.exists` | `orm.Register(c)` called twice on the same Core (sealed registry rejects duplicate) |
| `orm.action.unknown` | Service-form caller invoked an Action not registered (typo, or feature unsupported) |
| `orm.schema.mount` | `MountSchemas(c, prefix)` failed — bad JSON, prefix not in c.Data(), schema name conflict |
| `orm.watch.unsupported` | Medium has neither `Watch` nor `WatchPoll` capability; or `.WatchPoll(0)` against a non-CDC Medium |
| `orm.watch.closed` | Stream consumer iterated after ctx cancel or upstream subscription end |
| `orm.search.unsupported` | Search Mode requested without matching `SearchCaps` sub-capability |
| `orm.search.dimension` | Vector search query dimension didn't match the Schema-declared dimension |
| `orm.aliases.unsupported` | `From(A{...})` against a Medium with `Capabilities.Aliases == false` |
| `orm.join.unsupported` | Specific JoinKind (Left/Right/Full) not in `JoinCaps`; message identifies the kind |
| `orm.subquery.unsupported` | `Sub(...)` against a Medium with `Capabilities.Subqueries == false` |
| `orm.setop.unsupported` | `Union/Intersect/Except` against a Medium with `Capabilities.SetOps == false` |

Caller idiom:

```go
res := orm.Of[User](c).Find(99999)
if !res.OK {
    if res.Code() == "orm.notfound" {
        // expected — handle as "not present"
    }
    return res
}
```

For `core.ErrNoRows` and `core.ErrTxDone` (existing core/go sentinels), the bridge re-emits as `orm.notfound` and `orm.tx.closed` respectively to keep the keyspace under one prefix. The original error remains accessible via `core.Root(res.Value.(error))`.

---

## 9. Testing

### 9.1 In-Memory Test Medium (`memium.go`)

```go
// Memium is the in-memory test Medium implementing the full Capabilities.
//
//   m := orm.NewMemium()
//   orm.Mount(c, "default", m)
type Memium struct { /* ... */ }

func NewMemium() *Memium
```

`Memium` honours every capability — full predicate set, joins (relational links resolved client-side), transactions (snapshot-rollback), cursor streaming, aggregates, introspect. The bridge unit tests run exclusively against Memium; `go test` in core/orm requires zero external infrastructure.

Memium also exposes capability *masks* for testing degradation paths:

```go
m := orm.NewMemium()
m.MaskCaps(orm.Capabilities{
    Predicates: orm.PredicateCaps{Equality: true}, // only =, no Like/Comparison/etc.
    Joins:      false,
    Transactions: true,
    Aggregates: false,
})
// Bridge tests against this Memium hit the degradation paths
```

### 9.2 Test Naming — Good/Bad/Ugly

Per `plans/CLAUDE.md` convention, every public surface has three test functions, all mandatory:

```go
// TestBridge_Find_Good — happy path, Medium honours Intent fully
func TestBridge_Find_Good(t *testing.T) { /* ... */ }

// TestBridge_Find_Bad — caller error (bad PK type, missing schema)
func TestBridge_Find_Bad(t *testing.T) { /* ... */ }

// TestBridge_Find_Ugly — Medium-side failure or capability degradation
func TestBridge_Find_Ugly(t *testing.T) { /* ... */ }
```

### 9.3 Coverage Targets

- `bridge.go` — 95% (every fluent verb has Good/Bad/Ugly)
- `schema.go` — 90%
- `intent.go` — 100% (pure data; trivial)
- `medium.go` — 80% (interface + Capabilities; Memium covers concrete behaviour)
- `cache.go` — 90%
- `errors.go` — 100%
- `memium.go` — 90%

---

## 10. v1 Scope

### 10.1 In Scope

- Pkg-form consumption (`import "dappco.re/go/orm"`)
- `orm.Of[T](c)` typed bridge with full fluent chain
- `orm.Save / Delete / Insert / Tx` top-level helpers
- Schema declaration via `Schema() orm.Schema` method
- `orm.Define(fn)` builder DSL
- `Modelled` contract
- Lazy schema cache backed by `core.Registry[Schema]`
- Medium contract + Capabilities
- In-memory test Medium (Memium)
- Error codes under `orm.*` keyspace
- Eager relation loading via `With`
- Streaming via `core.Seq[T]`
- SQL transactions via `*core.Tx`
- Lightweight migrations: DDL emission, additive Diff, Apply
- 4 relation kinds: HasMany, BelongsTo, HasOne, ManyMany (no PolyMorphic in v1)
- **Input coercion** (§4.7): Schema-declared type-shapers + cross-type coercers; bridge has no path that skips coercion; SQL injection structurally impossible
- **Output rehydration** (§4.8): bidirectional shapers — same Schema declaration enforces shape on read; bridge has no path that skips rehydration; Medium wire-format quirks (SQLite int-as-bool, JSON medium all-strings, encrypted columns) normalised before reaching consumer struct
- **Service-form consumption** (§4.9): `orm.Register(c)` mounts the bridge as a `core.Service`; every verb exposed as a typed `core.Action` for cross-language IPC and SDK generation; both pkg-form and service-form work simultaneously
- **Schema mounting via `c.Data()`** (§4.10): publish schemas as embedded JSON; consume cross-package via `orm.MountSchemas(c, prefix)` + `orm.OfTable(c, name)` for type-erased queries
- **Reactive subscriptions** (§4.11): `orm.Of[T](c).Watch(ctx)` returns `core.Seq[Event[T]]`; native CDC where supported, polling fallback where not; Memium implements native broadcast
- **Search verb** (§4.12): `orm.Of[T](c).Search(query, opts)` over Schema-declared `.Searchable(...)` fields; text / vector / hybrid modes gated by Medium SearchCaps
- **Aliased FROM with column-ref predicates** (§4.13): `orm.A{}` + `orm.Col(...)` first-class joins; `orm.Sub(...)` subqueries; `orm.Union/Intersect/Except` set operations; the relational-algebra-interpreter foundation

### 10.2 Out of Scope (Deferred)

- Concrete SQL Medium impl in core/orm (lives in sibling package `dappco.re/go/orm-sql`)
- Concrete KV / blob / RPC Medium impls (live in sibling packages)
- Cross-Medium transactions
- Cross-Medium joins / subqueries (single-Medium only in v1)
- Down-migrations / destructive schema diffs
- Window functions
- Recursive CTEs (non-recursive aliased Sub already covers most use cases via §4.13)
- Polymorphic relations
- Soft-delete (caller adds `deleted_at` field manually + `WhereNull`)
- Optimistic locking (callers can add `version` column + `Save` constraint manually)
- Hooks / observers / events
- Query result caching
- Connection pooling beyond what `*core.DB` provides

### 10.3 Polyglot Future

PHP and TS implementations of the same contract (`User::schema()`, `User.schema()` returning identical JSON shape) live in:

- `dappco.re/php/orm` — Eloquent-style adapter exposing `Orm::of<User>()` PHP rhythm
- `dappco.re/ts/orm` — Drizzle-shape adapter exposing `Orm.of<User>()` TS rhythm

Both consume the schema produced by the Go side (or produce schemas the Go side can consume) — single migration history, shared Mediums where transport allows. v1 is Go-only; PHP/TS bindings are post-v1.

---

## 11. File Layout

```
core/orm/
├── orm.go              — top-level: Of[T](c), Save, Delete, Insert, Tx, Mount
├── bridge.go           — *Bridge[T] fluent chain
├── schema.go           — Schema, Field, Relation, Builder, Define
├── intent.go           — ReadIntent, WriteIntent, Predicate, OrderBy, AggregateOp
├── medium.go           — Medium interface, Capabilities, PredicateCaps, Payload, Meta
├── cast.go             — orm.Detail[T] (value + meta)
├── cache.go            — schema cache backed by core.Registry[Schema]
├── memium.go           — in-memory test Medium (full Capabilities including Watch + Search + Aliases)
├── errors.go           — stable error codes
├── input.go            — bidirectional shapers (Coerce + Rehydrate), Apply dispatcher (§4.7, §4.8)
├── service.go          — Register(c), *Service, Action handler registrations (§4.9)
├── mount.go            — MountSchemas, OfTable, *DynamicBridge (§4.10)
├── watch.go            — *Bridge[T].Watch, Event[T], WatchOp, polling fallback (§4.11)
├── search.go           — *Bridge[T].Search, SearchOpts, Ranked[T] (§4.12)
├── alias.go            — A, Col, Sub, JoinSpec, Union/Intersect/Except, *Bridge[T].From (§4.13)
├── ddl.go              — DDL emission, Diff, Apply (§7)
├── orm_test.go
├── bridge_test.go
├── schema_test.go
├── intent_test.go
├── medium_test.go
├── memium_test.go
├── input_test.go
├── service_test.go
├── mount_test.go
├── watch_test.go
├── search_test.go
├── alias_test.go
├── ddl_test.go
└── README.md
```

Total: ~18 source files + tests. No external dependencies beyond `dappco.re/go/core` and `dappco.re/go/io` (for the future Medium impls in sibling packages).

---

## 12. Polyglot Contract

The `Schema` JSON shape is the cross-language contract. Both PHP and TS implementations produce/consume the same:

```json
{
  "name": "users",
  "pk": ["id"],
  "fields": [
    {"name": "id", "type": "int64", "constraints": ["pk"]},
    {"name": "email", "type": "string", "constraints": ["unique", "notnull"]},
    {"name": "name", "type": "string", "constraints": ["notnull"]},
    {"name": "active", "type": "bool", "default": true},
    {"name": "created_at", "type": "time", "default": "now"}
  ],
  "indexes": [
    {"fields": ["email"]}
  ],
  "relations": [
    {"kind": "hasMany", "name": "posts", "fk": "user_id"}
  ]
}
```

**PHP equivalent** (`dappco.re/php/orm`):

```php
class User extends \Core\Model {
    public static function schema(): \Core\Orm\Schema {
        return \Core\Orm\Schema::define(fn($b) => $b
            ->name('users')
            ->pk('id')
            ->string('email')->unique()->notNull()
            ->string('name')->notNull()
            ->bool('active')->default(true)
            ->time('created_at')->default('now')
            ->hasMany('posts', 'user_id')
            ->index('email'));
    }
}
```

**TS equivalent** (`dappco.re/ts/orm`):

```ts
class User {
    static schema(): Schema {
        return define(b => b
            .name('users')
            .pk('id')
            .string('email').unique().notNull()
            .string('name').notNull()
            .bool('active').default(true)
            .time('created_at').default('now')
            .hasMany('posts', 'user_id')
            .index('email'));
    }
}
```

Same contract, three languages, one schema JSON.

---

## 13. Banned Imports & Conventions

Per `plans/CLAUDE.md` and `plans/code/core/go/CLAUDE.md`, core/orm code must use core/go primitives for the following:

| Banned | Use Instead |
|--------|-------------|
| `fmt` | `core.Sprintf`, `core.Print`, `core.Concat` |
| `log` | `core.Log()` via `*core.Core` |
| `errors` | `core.E`, `core.Wrap`, `core.WrapCode`, `core.NewCode`, `core.Is`, `core.As` |
| `os` | `c.Fs()` |
| `os/exec` | `c.Process()` |
| `strings` | `core.Contains`, `core.HasPrefix`, `core.Split`, `core.Join`, etc. |
| `path/filepath` | `core.JoinPath`, `core.PathBase`, `core.PathDir` |
| `encoding/json` | `core.JSONMarshal`, `core.JSONUnmarshal` |
| `io` | go-io Medium (when needed); core/orm itself depends only on `core.Seq` for streaming |
| `database/sql` | `core.SQLOpen`, `core.DB`, `core.Tx`, `core.NullString` / `NullInt64` / `NullBool` / `NullFloat64` / `NullTime`, `core.ErrNoRows`, `core.ErrTxDone` |
| `iter` | `core.Seq`, `core.Seq2`, `core.Pull`, `core.Pull2` |
| `reflect` | `core.TypeOf`, `core.ValueOf`, `core.Kind*`, `core.DeepEqual`, `core.Zero` |

**Conventions:**
- `declare(strict_types=1)` equivalent: every Go file includes the SPDX header
- Test naming: `TestFilename_Function_{Good,Bad,Ugly}` — all three mandatory
- Comments are usage examples, not prose descriptions (per AX Principle 2)
- Errors carry stable codes from §8
- Every public function/type has a doc comment with executable usage example

---

## 14. Worked Example

A complete consumer scenario showing every primitive composing:

```go
package main

import (
    "dappco.re/go/core"
    "dappco.re/go/orm"
    orm_sql "dappco.re/go/orm-sql"
)

type User struct {
    ID    int64
    Email string
    Tier  string
}

func (User) Schema() orm.Schema {
    return orm.Define(func(b *orm.Builder) {
        b.Name("users")
        b.PK("id")
        b.String("email").Unique().NotNull()
        b.String("tier").Default("free")
    })
}

type Post struct {
    ID     int64
    UserID int64
    Title  string
}

func (Post) Schema() orm.Schema {
    return orm.Define(func(b *orm.Builder) {
        b.Name("posts")
        b.PK("id")
        b.Int64("user_id").NotNull()
        b.String("title").NotNull()
        b.BelongsTo("user", "user_id")
    })
}

func main() {
    c := core.New()
    defer c.Error().Recover()

    // Mount a Medium (concrete impl from sibling pkg)
    sqlMed := orm_sql.New(c, "duckdb", "file:./app.db")
    if !orm.Mount(c, "default", sqlMed).OK {
        c.Exit(1)
    }

    // First-time: diff declared schemas against Medium state, apply additive changes
    for _, s := range []orm.Schema{User{}.Schema(), Post{}.Schema()} {
        diff := orm.Diff(c, s)
        if !diff.OK {
            core.Println("diff error:", diff.Error())
            c.Exit(1)
        }
        changes, _ := core.Cast[[]orm.Change](diff)
        if len(changes) > 0 {
            if !orm.Apply(c, changes).OK {
                c.Exit(1)
            }
        }
    }

    // Insert
    user := User{Email: "snider@host.uk.com", Tier: "pro"}
    if !orm.Save(c, &user).OK {
        c.Exit(1)
    }

    // Read
    res := orm.Of[User](c).Where("tier", "=", "pro").Get()
    users, _ := core.Cast[[]User](res)
    for _, u := range users {
        core.Println(u.Email)
    }

    // Read with relation
    res = orm.Of[User](c).With("posts").Find(user.ID)
    if !res.OK {
        if res.Code() == "orm.notfound" {
            core.Println("user disappeared")
            return
        }
        return
    }
    fetched, _ := core.Cast[User](res)
    _ = fetched

    // Transaction
    tres := orm.Tx(c, func(tx *core.Tx) core.Result {
        r := orm.Of[User](c).WithTx(tx).Find(user.ID)
        if !r.OK { return r }
        u, _ := core.Cast[User](r)
        u.Tier = "enterprise"
        return orm.Save(c, &u, orm.WithTx(tx))
    })
    if !tres.OK {
        core.Println("tx failed:", tres.Error())
    }

    // Stream
    stream := orm.Of[Post](c).Where("user_id", "=", user.ID).Stream()
    if stream.OK {
        seq, _ := core.Cast[core.Seq[Post]](stream)
        for p := range seq {
            core.Println(p.Title)
        }
    }
}
```

This example exercises: Mount, DDL, Save (Insert), Get with predicate, Find with eager With, Tx, Stream, error code introspection, `core.Cast[T]`, `Result.OK` checks, Result forwarding. An agent can reproduce the entire surface from this listing alone.

---

## 15. Aspirations Beyond v1

The v1 surface (§§4.1–4.13) is already substantial — service-form, schema mounting, reactive subscriptions, search, aliased queries with column-refs all ship in v1. The aspirations below are the remaining post-v1 additions; all are additive and don't break v1's surface.

- **Conclave fleet sync** — gateway-side replication of work-state tables (issues, plans, memories) via Intent-replay over network transport; bridge already produces serialisable Intents, so a Conclave-side Medium that consumes Intents over IPC is a natural extension
- **Multi-Medium Saga** — long-running transactions spanning Mediums via compensating writes; opt-in via `orm.Saga(c, fn)`. v1 hard-rejects cross-Medium transactions; Saga is the relaxed form, with explicit compensation declared at each step
- **Recursive CTEs** — extends `Sub()` with a `.Recursive()` modifier so a sub-builder can reference its own alias. v1 supports non-recursive Sub, which covers most use cases
- **Window functions** — `OVER (PARTITION BY ... ORDER BY ...)` as a chainable verb on aggregates: `orm.Of[Order](c).Window().Partition("user_id").Order("ts").Sum("total")`. Requires Medium capability `Capabilities.Windows`
- **Polymorphic relations** — `b.MorphTo("subject")` where the related table is determined by a discriminator column. Common in CMS/comment systems; v1 omits because the alternative (separate FK columns per type) is cleaner
- **Soft-delete first-class** — `b.SoftDelete()` Schema modifier auto-adds a `deleted_at` column AND auto-injects `WhereNull("deleted_at")` into every read; `WithTrashed()` chain modifier opts back in. v1 makes callers do this manually; promotion is a clarity win, not an architectural shift
- **Cross-Medium read** — same query against multiple Mediums merging results client-side (e.g., archive in S3 + recent in Postgres union). v1 firmly single-Medium; cross-Medium is opt-in via a `MultiMedium` adapter that orchestrates per-medium queries
- **Audit-trail Medium adapter** — wraps any Medium so every WriteIntent is journalled to a separate audit Medium before/after the actual write. Composes with Memium for testing, with go-store for journal storage
- **Vector index types** — currently `b.Bytes("e").Searchable("vector", N)` declares vector but doesn't declare index strategy (HNSW vs IVF vs flat). Post-v1 modifier: `.Index("hnsw", IndexOpts{M: 16, EfConstruction: 200})`

Each of these is additive — none require breaking the v1 surface. v1 is deliberately the substantial first cut; v1.1 lands the saga and recursive sub; v2 is where multi-Medium and cross-cutting concerns land.

---

> **Two Monks.** *"I put it down hours ago. Why are you still carrying it?"*
>
> **Chinese Farmer.** *"Maybe."*
