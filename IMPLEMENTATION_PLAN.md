# core/orm — Implementation Plan (Go v1)

This breaks the RFC into ordered, dependency-aware build phases. Each phase ships compilable, testable code. No phase begins with broken builds left from the previous one.

**Spec authority:** [RFC.md](./RFC.md). Where this plan and the RFC disagree, the RFC wins.

**Test convention** (per `plans/CLAUDE.md`): every public surface has `TestFilename_Function_{Good,Bad,Ugly}` — all three mandatory. No phase is "done" until its surface ships all three for every public symbol.

**Banned imports** (RFC §13): `fmt`, `log`, `errors`, `os`, `os/exec`, `strings`, `path/filepath`, `encoding/json`, `io`, `database/sql`, `iter`, `reflect`. Use `core.X` equivalents.

---

## Phase 0 — Module Bootstrap

**Goal:** empty module compiles, `go test` passes (zero tests).

- `go.mod` declares `module dappco.re/go/orm`, Go ≥ 1.26
- Single `require` for `dappco.re/go` (= `core/go`)
- `doc.go` package-level docstring describing what `orm` is, citing the two parables
- Smoke test `orm_test.go` that imports `dappco.re/go` and asserts `core.New() != nil`

**Acceptance:** `go build ./... && go test ./...` exit 0.

---

## Phase 1 — Schema Primitives (RFC §3, §5.2)

**Goal:** Schema types defined as pure data; round-trip JSON.

- `schema.go` — `Schema`, `Field`, `Relation`, `OrderBy`, `AggregateOp` structs (plain data, JSON-tagged)
- `schema.go` — `Builder` struct with chained methods: `Name`, `PK`, `String`/`Int`/`Int64`/`Bool`/`Float`/`Float64`/`Time`/`JSON`/`Bytes`, `Unique`, `NotNull`, `Default`, `Format`, `Min`, `Max`, `Pattern`, `OneOf`, `MaxBytes`, `Index`, `HasMany`, `BelongsTo`, `HasOne`, `ManyMany`
- `schema.go` — `Define(func(*Builder)) Schema` — builder entry
- `schema.go` — `SchemaFromJSON([]byte) core.Result` — round-trip
- `schema.go` — `Modelled` interface: `Schema() Schema`
- `schema_test.go` — Good/Bad/Ugly per public symbol; round-trip property test (`SchemaFromJSON(JSONMarshal(s)).DeepEqual(s)`)

**Acceptance:** every Schema fluently constructed in tests serialises to canonical JSON and round-trips byte-identical.

---

## Phase 2 — Intent Types (RFC §5.2)

**Goal:** `ReadIntent`, `WriteIntent`, `Predicate` as pure data.

- `intent.go` — `ReadIntent`, `WriteIntent`, `Predicate`, `WriteOp` (constants `OpInsert`, `OpUpdate`, `OpSave`, `OpDelete`)
- All structs JSON-marshalable
- `intent_test.go` — Good/Bad/Ugly; nested `Predicate.Group` for OR composition

**Acceptance:** Intent values can be JSON-marshalled, transmitted, unmarshalled, and compared by `core.DeepEqual`.

---

## Phase 3 — Input Shapers, Bidirectional (RFC §4.7, §4.8)

**Goal:** `input.go` ships every shaper with both `Coerce` and `Rehydrate`.

- `input.go` — `Shaper` interface: `Coerce(any) core.Result`, `Rehydrate(any) core.Result`
- Symmetric shapers (Rehydrate is identity): `String`, `Email`, `URL`, `UUID`, `IPv4`, `IPv6`, `Hostname`, `Slug`, `Int`, `Bool`, `Float`, `Pattern`, `OneOf`, `Min`, `Max`
- Asymmetric shapers (both halves explicit): `Time` (lenient parse ↔ canonical `core.Time`), `JSON` (struct ↔ canonical bytes), `Bytes` (with size limits)
- Cross-type coercers: `BoolToInt` / `IntToBool` / `StringToInt` / `IntToString` / `StringToBool` / `TimeToUnix` / `UnixToTime`
- `input.go` — `Apply(field Field, value any) core.Result` — dispatcher; reads field's declared modifiers and runs shapers in order (Coerce first, then NotNull, then Format, then Min/Max, then Pattern, then OneOf)
- `input.go` — `RehydrateApply(field Field, value any) core.Result` — output side
- `input_test.go` — Good/Bad/Ugly per shaper; round-trip property tests for asymmetric shapers (`Rehydrate(Coerce(x)) == x` for canonical inputs)

**Acceptance:** every error path produces a stable `orm.input.*` or `orm.output.*` code per RFC §8. Fuzz harness (Go 1.18+ native fuzz) on `Email`/`Slug`/`Int`/`Bool` shapers — no panics on arbitrary `any` input.

---

## Phase 4 — Schema Cache (RFC §3.4)

**Goal:** lazy, type-keyed cache backed by `core.Registry[Schema]`.

- `cache.go` — `cacheGet[T any](c *core.Core) Schema` — first call invokes `(*new(T)).Schema()` via `core.TypeOf` reflection; subsequent calls hit cache
- Cache lifecycle: cache-per-Core-instance (no package globals); test reset by creating a new Core
- `cache_test.go` — Good (cache hit returns same Schema), Bad (type without `Schema()` method → `orm.schema.missing`), Ugly (concurrent first-call from multiple goroutines doesn't double-call `Schema()`)

**Acceptance:** Schema method called exactly once per type per Core. Race-free under `go test -race`.

---

## Phase 5 — Medium Contract (RFC §5)

**Goal:** Medium interface defined; Capabilities + Payload + Meta + Mount registry.

- `medium.go` — `Medium` interface: `Caps()`, `Read(ctx, in) Result`, `Write(ctx, in) Result`, `Stream(ctx, in) Result`
- `medium.go` — `Capabilities`, `PredicateCaps` structs
- `medium.go` — `Payload {Data any, Meta Meta}` internal wrapper; `Meta` struct
- `medium.go` — `Mount(c *core.Core, name string, m Medium) core.Result` — registers via `core.Registry[Medium]`
- `medium.go` — `resolve(c *core.Core, name string) (Medium, core.Result)` — internal lookup; returns `orm.medium.notmounted` when missing
- `medium_test.go` — Good/Bad/Ugly per public; mount-twice-same-name behaviour (sealed registry rejects)

**Acceptance:** No concrete Medium implementations yet. The interface is the contract; Phase 6 ships the test Medium that exercises the full surface.

---

## Phase 6 — In-Memory Test Medium (RFC §9.1)

**Goal:** `Memium` implements full Capabilities; bridge tests use it exclusively.

- `memium.go` — `Memium` struct holding `map[string][]any` per table; full Capabilities (`{Predicates: all, Joins: true, Transactions: true (snapshot), Aggregates: true, Cursor: true, Introspect: true}`)
- `memium.go` — `MaskCaps(Capabilities)` — for testing degradation paths
- `memium.go` — Read implements predicate filter, ordering, limit/offset, eager With (linear-scan join)
- `memium.go` — Write implements Insert/Update/Save/Delete with optimistic-lock check
- `memium.go` — Stream returns `core.Seq[T]` over the in-memory rows
- `memium_test.go` — Good/Bad/Ugly per Medium method; capability-mask tests verify each `Capabilities` field actually gates behaviour

**Acceptance:** Memium is the SUT for Phases 7+. `go test ./...` requires zero external infrastructure.

---

## Phase 7 — Bridge Reads (RFC §4.1–§4.4, §4.8)

**Goal:** `*Bridge[T]` fluent chain produces ReadIntent → dispatches Memium → applies output shapers → returns `core.Result` with typed Payload.

- `orm.go` — `Of[T any](c *core.Core) *Bridge[T]` — generic entry; resolves Schema via cache, mounts default Medium
- `bridge.go` — `*Bridge[T]` struct holding `intent ReadIntent`, `c *core.Core`, accumulated errors
- `bridge.go` — `Where`, `OrWhere`, `WhereNotNull`, `WhereNull`, `WhereGroup` — every value passes through `input.Apply`
- `bridge.go` — `With`, `Order`, `Limit`, `Offset`
- `bridge.go` — terminal verbs: `Find(pk...) Result`, `First() Result`, `Get() Result`, `All() Result`, `Count() Result`, `Sum/Min/Max/Avg(field) Result`, `Stream() Result`
- `bridge.go` — every Read result post-processed via `output.Apply` (Phase 3) before populating typed struct
- `cast.go` — `Detail[T any](r core.Result) (T, Meta, bool)` — typed unwrap with meta access
- `bridge_test.go` — Good/Bad/Ugly per terminal verb against Memium; verify error code per failure mode

**Acceptance:** `orm.Of[User](c).Where("email","=","x@y").First()` round-trips through the full pipeline; bad input produces correct `orm.input.*` code; missing row produces `orm.notfound`.

---

## Phase 8 — Bridge Writes (RFC §4.5)

**Goal:** `Save`, `Delete`, `Insert`, predicate-`Update`/`DeleteAll` ship.

- `orm.go` — `Save(c, rows...)`, `Insert(c, rows...)`, `Delete(c, rows...)` — top-level helpers; T inferred from arg
- `bridge.go` — `Save(*T)`, `Insert(*T)`, `Delete(*T)` — typed-bridge form
- `bridge.go` — `Update(map[string]any) Result`, `DeleteAll() Result` — predicate-based variants
- All write paths run input shapers per Schema before WriteIntent forms
- `bridge_test.go` — Good/Bad/Ugly per write verb; verify Insert vs Update branch in Save by PK presence

**Acceptance:** Writing a User with bad email produces `orm.input.format` BEFORE the WriteIntent reaches the Medium.

---

## Phase 9 — Transactions (RFC §4.6)

**Goal:** `orm.Tx(c, fn)` opens a transaction on the Medium owning the resolved schema; bridge calls accept `WithTx(tx)` chain modifier.

- `orm.go` — `Tx(c, fn func(*core.Tx) core.Result) core.Result`
- `bridge.go` — `WithTx(*core.Tx) *Bridge[T]` — threads tx into Intent
- `medium.go` — `Capabilities.Transactions` gates whether Tx works; non-transactional Mediums return `orm.tx.unsupported`
- Cross-Medium tx (multiple Mediums touched in one body) returns `orm.tx.cross_medium`
- Memium implements snapshot-rollback transactions

**Acceptance:** Tx commits on `OK` return, rolls back on `!OK`; non-tx Medium fails immediately, not silently.

---

## Phase 10 — Capability Degradation (RFC §6)

**Goal:** bridge gracefully degrades when Memium's capabilities are masked.

- `bridge.go` — capability check on each verb; degradation behaviour per RFC §6 table
- `Strict()` chain modifier upgrades all silent degradations to hard fails (`orm.predicate.unsupported`, etc.)
- `NoFallback` in `With(name, NoFallback)` overrides the default for joins
- `bridge_test.go` — Mask each capability, verify default + `Strict()` behaviour matches the table

**Acceptance:** every cell of the §6 degradation table has a passing test.

---

## Phase 11 — DDL & Migrations (RFC §7)

**Goal:** `orm.DDL`, `orm.Diff`, `orm.Apply` ship.

- `ddl.go` — `DDL(c, schema, dialect) core.Result` — emits dialect-specific CREATE TABLE
- `ddl.go` — `Diff(c, schema) core.Result` — additive Change list against current Medium state (requires `Capabilities.Introspect`)
- `ddl.go` — `Apply(c, []Change) core.Result` — applies within tx where supported; records in `_orm_migrations` table
- `ddl_test.go` — Good/Bad/Ugly; verify additive-only (no destructive emissions even when columns/indexes go missing in Schema)

**Acceptance:** Memium-introspected Diff emits the right shape for an evolving Schema; bridge bootstraps `_orm_migrations` table on first Apply.

---

## Phase 12 — Errors, Polish, Worked Example (RFC §8, §14)

**Goal:** every error code from RFC §8 has a test; the §14 worked example compiles and runs against Memium.

- `errors.go` — sentinel `*core.Err` values per RFC §8; constructors via `core.NewCode`
- `errors_test.go` — round-trip every code through `r.Code() == "orm.X"`
- `examples/full_example_test.go` — the §14 worked example as an executable test (Mount Memium, declare User and Post, Save, Find, Tx, Stream — all assertions pass)

**Acceptance:** spec compliance via test enumeration; an agent that read only the RFC could rebuild the example and have it pass.

---

## Phase 13 — Service Form (RFC §4.9)

**Goal:** `orm.Register(c)` mounts the bridge as a `core.Service`; every verb registered as a typed `core.Action`. Both pkg-form and service-form work simultaneously against the same bridge internals.

- `service.go` — `Register(c *core.Core) core.Result` per the canonical service.go pattern (memory: `project_canonical_service_go_pattern.md`)
- `service.go` — `*Service` struct; `core.RegisterAction` calls for every Action listed in RFC §4.9
- DTO inputs typed per `go/CLAUDE.md` DTO Pattern — never loose `core.Options` for business logic; struct DTOs feed SDK codegen
- Service-form caller path: `c.Action("orm.find").Run(opts)` produces the same `core.Result` as `orm.Of[T](c).Find(...)`
- `service_test.go` — Good/Bad/Ugly per Action; verify pkg-form and service-form produce equivalent Results for the same logical query

**Acceptance:** `c.RegistryOf("actions").List("orm.*")` returns every Action from the §4.9 table. The §14 worked example also runs through service-form Actions and produces identical outcomes.

---

## Phase 14 — Schema Mounting via `c.Data()` (RFC §4.10)

**Goal:** schemas can be mounted as embedded JSON assets and consumed cross-package without source-level dependency.

- `mount.go` — `MountSchemas(c, prefix) core.Result` — scans `c.Data()` entries under prefix, parses each JSON file via `SchemaFromJSON`, registers in schema cache keyed by table name
- `mount.go` — `OfTable(c, table) *DynamicBridge` — string-keyed schema lookup; type-erased fluent surface
- `mount.go` — `*DynamicBridge` — same chainable verbs as `*Bridge[T]`, but rows come back as `map[string]any` (each value still rehydrated per Schema declaration)
- `mount_test.go` — Good (mount + OfTable + query round-trip); Bad (schema name conflict on mount); Ugly (Data prefix has no entries → `orm.schema.mount` with helpful message)

**Acceptance:** a schema published as JSON in package A's `c.Data()` is queryable from package B via `OfTable` without B importing A. Memium honours the type-erased path; rehydration applies identically.

---

## Phase 15 — Reactive Subscriptions (RFC §4.11)

**Goal:** `orm.Of[T](c).Watch(ctx)` returns `core.Seq[Event[T]]` with native CDC where supported, polling fallback where not.

- `watch.go` — `*Bridge[T].Watch(ctx core.Context) core.Result` — terminal verb returning Seq
- `watch.go` — `Event[T]`, `WatchOp` (`OpInitial`, `OpInsert`, `OpUpdate`, `OpDelete`), `WatchOpts`
- `watch.go` — chain modifiers: `.Live()` (suppress initial snapshot), `.WatchPoll(d)` (set polling interval; 0 = require native CDC)
- Capability handling per RFC §6 degradation table — native if `Capabilities.Watch`, polling diff if `WatchPoll`, hard fail otherwise
- `medium.go` — `Medium` interface gains `Watch(ctx, intent) core.Result` method
- Memium implements native broadcast: every Insert/Update/Delete fans out to subscribers whose Where matches
- `watch_test.go` — Good (initial replay + live changes); Bad (`Live()` against non-existent matching set); Ugly (subscriber dies mid-stream — verify ctx-cancel cleanup and `orm.watch.closed` for late iteration)

**Acceptance:** subscriber gets snapshot replay then live events; predicate boundary changes (row updated INTO or OUT OF the predicate set) emit correct OpInsert/OpDelete; ctx cancel cleans up Medium-side subscription.

---

## Phase 16 — Search Verb (RFC §4.12)

**Goal:** `orm.Of[T](c).Search(query, opts)` runs ranked search against `.Searchable(...)` Schema fields.

- `search.go` — `*Bridge[T].Search(query string, opts SearchOpts) core.Result`
- `search.go` — `SearchOpts`, `Ranked[T]`, `SearchSpec`
- `schema.go` — extend Builder with `.Searchable(kind string, dim ...int)` modifier (kind = "text" | "vector" | "hybrid"; dim required for vector)
- `medium.go` — `Medium` interface gains `Search(ctx, intent) core.Result` method
- Capability gating: `Capabilities.Search.Text/Vector/Hybrid/Facets` — bridge hard-fails on mode mismatch (`orm.search.unsupported`); vector dimension mismatch (`orm.search.dimension`) on rejected insert
- Memium implements basic case-insensitive substring text search; no vector in v1 (vectors require real indexing not in-memory)
- `search_test.go` — Good (text search returns ranked results); Bad (vector mode against text-only Medium); Ugly (Searchable fields populated nil — search query against nil-valued column returns empty, not error)

**Acceptance:** Memium returns ranked results sorted by score for text queries; capability mismatch produces correct error code; highlight populates `Ranked.Highlights` when requested and Medium supports it.

---

## Phase 17 — Aliased FROM with Column-Ref Predicates (RFC §4.13)

**Goal:** first-class joins, subqueries, and set operations via the relational-algebra-interpreter pattern. The biggest single surface in the v1 expansion.

- `alias.go` — type aliases: `A = map[string]any`, `ColRef`, `JoinSpec`, `JoinKind` constants, `SubRef`, `SetOp`, `SetOpKind`, `BridgeRef` interface
- `alias.go` — constructors: `Col(name) ColRef`, `LeftJoin/RightJoin/FullJoin(table) JoinSpec`, `Sub(b BridgeRef) SubRef`, `Union/Intersect/Except(builders...) BridgeRef`
- `alias.go` — `*Bridge[T].From(a A) *Bridge[T]` — terminal-modifier; populates `ReadIntent.Alias`
- `intent.go` — extend `ReadIntent` with `Alias map[string]any` and `SetOp *SetOp` fields
- Capability handling per RFC §6 — `Aliases`, `Subqueries`, `SetOps`, `JoinKinds` all hard-fail when missing
- Predicate.Value type-assertion: bridge detects `ColRef` and emits "compare column to column" semantics; falls through to literal otherwise
- Memium implements all four join kinds via linear-scan client-side joining; supports Sub via materialising the sub-result; supports Union/Intersect/Except via slice ops
- `alias_test.go` — Good per join kind + per set op + Sub; Bad (alias used in Where but not declared in From); Ugly (cross-Medium join attempt → `orm.aliases.unsupported` if Medium can't honour, NOT silent fallback)

**Acceptance:** every join kind round-trips against Memium; Sub-builders compose 2 levels deep; Union/Intersect/Except produce correct result-sets; capability mismatches fail with the specific error code from §8.

---

## File Coverage Targets

Per RFC §9.3 (extended for new phases):

| File | Target | Phase |
|------|--------|-------|
| `bridge.go` | 95% | 7, 8, 9, 10 |
| `schema.go` | 90% | 1, 16 |
| `intent.go` | 100% | 2, 17 |
| `medium.go` | 80% | 5, 15, 16 |
| `cache.go` | 90% | 4 |
| `errors.go` | 100% | 12 |
| `memium.go` | 90% | 6, 15, 16, 17 |
| `input.go` | 95% | 3 |
| `service.go` | 90% | 13 |
| `mount.go` | 90% | 14 |
| `watch.go` | 95% | 15 |
| `search.go` | 90% | 16 |
| `alias.go` | 95% | 17 |
| `ddl.go` | 85% | 11 |

---

## Convergence

Per the project's [Implementation Loop](../../host-uk/core/plans/CLAUDE.md), each phase finishes with a 4-zero validation: four consecutive mini passes producing zero new features = phase is at parity with its spec slice. When all 13 phases pass 4-zero rounds, the repo is at v1 alpha.1.

**Estimated phase count for full mini convergence:** 25-40 rounds (mini frequently finds features after declared convergence; the convergence signal is reliable but not predictive).
