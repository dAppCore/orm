# core/orm — Typed Fluent Communications Bridge

> Eloquent's reading rhythm in Go, without Eloquent's machinery.
> A stateless bridge that produces *intent*; a Medium *transports* it.
> Same call lands against DuckDB today, Postgres tomorrow, a Borg.DataNode next week.

## What's Here

This repo is the cross-language home for `core/orm`. Per RFC §12 (Polyglot Contract), the same `Schema()` declaration ports across Go, PHP, and TS implementations producing identical JSON shape.

```
core/orm/
├── RFC.md                  ← canonical spec (1740 lines, self-contained)
├── IMPLEMENTATION_PLAN.md  ← phased build order for the v1 Go implementation
├── README.md               ← this file
├── go.work                 ← Go workspace declaring orm + core/go (no `replace` directives anywhere)
├── externals/
│   └── core-go/            ← bundled snapshot of dappco.re/go (the foundation framework)
└── go/                     ← Go implementation (v1 target)
    ├── go.mod
    └── *.go                ← candidate writes implementation here
```

PHP and TS bindings (post-v1) live in sibling `php/` and `ts/` subfolders when added.

## Workspace Setup

The repo ships with `go.work` at the root and a vendored snapshot of `dappco.re/go` (= core/go) under `externals/core-go/`. This means:

- **No `replace` directives in any `go.mod`** — workspace mode resolves the dep
- **Self-contained** — clone the repo, run `go build ./...` from `go/`, no GOPROXY required
- **Candidates do not modify `go.work`, `externals/`, or any file under them** — those are the immutable baseline; modifying them is grounds for grading dock (counts as cheating the dependency contract)

To verify the workspace resolves:

```bash
cd core/orm
go work sync                  # idempotent; should exit 0
cd go && go build ./...       # builds (currently empty — populated by IMPLEMENTATION_PLAN.md phases)
```

To run *outside* the workspace (production / CI module-mode), set `GOWORK=off` and ensure `dappco.re/go` is reachable via your GOPROXY.

## Spec Status

`RFC.md` passed Opus-class validation 2026-05-03:
- All `core.*` references resolve to actual primitives in `dappco.re/go`
- No undefined types
- No banned-import violations in example code
- 15 sections, 28 subsections, 1258 lines — self-contained per `code/` prickle convention

## Two Design Parables

> **Two Monks.** *"I put it down hours ago. Why are you still carrying it?"*
> The bridge is stateless. Cross the data, set it down, walk on. No dirty-tracking.
> Writes are visible at the call site (`orm.Save(c, &user)`, never `user.Save()`).

> **Chinese Farmer.** *"Maybe."*
> No backend is pre-judged. Capability is declared by the Medium, not assumed by the bridge.
> The Medium honours what it can; bridge degrades gracefully or fails honestly.

## Quick API Tour

```go
import "dappco.re/go/orm"

// Reads
res := orm.Of[User](c).Find(1)
user, _ := core.Cast[User](res)

res = orm.Of[User](c).Where("active","=",true).With("posts").Get()
users, _ := core.Cast[[]User](res)

// Writes (visible bridge crossing)
orm.Save(c, &user)
orm.Delete(c, &user)

// Schema lives on the type
func (User) Schema() orm.Schema {
    return orm.Define(func(b *orm.Builder) {
        b.Name("users")
        b.PK("id")
        b.String("email").Unique().NotNull().Format("email")
        b.HasMany("posts", "user_id")
    })
}
```

Full surface in [RFC.md §4](./RFC.md).

## License

EUPL-1.2.
