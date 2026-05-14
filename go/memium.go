// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

// Memium is the in-memory test Medium implementing the full Capabilities.
// Rows are stored as map[string]any per table. Used exclusively by bridge
// unit tests — go test requires zero external infrastructure.
//
//	m := orm.NewMemium()
//	orm.Mount(c, "default", m)
type Memium struct {
	tables   map[string][]map[string]any
	schemas  map[string]Schema
	watchers map[string][]*memiumWatcher
	caps     Capabilities
	mu       core.RWMutex
	nextID   int64
}

type memiumWatcher struct {
	preds    []Predicate
	ch       chan Event[map[string]any]
	done     chan struct{}
	mu       core.Mutex
	queue    []Event[map[string]any]
	draining bool
	closed   bool
}

// NewMemium creates a Memium with full capabilities enabled.
func NewMemium() *Memium {
	return &Memium{
		tables:   make(map[string][]map[string]any),
		schemas:  make(map[string]Schema),
		watchers: make(map[string][]*memiumWatcher),
		caps:     FullCaps(),
	}
}

// Caps reports the current capabilities (may be masked for degradation tests).
func (m *Memium) Caps() Capabilities {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.caps
}

// MaskCaps sets the capabilities to the given mask — used for testing
// degradation paths.
func (m *Memium) MaskCaps(caps Capabilities) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.caps = caps
}

// RegisterTable seeds Memium metadata and ensures the table exists.
func (m *Memium) RegisterTable(name string, schema Schema) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tables == nil {
		m.tables = make(map[string][]map[string]any)
	}
	if m.schemas == nil {
		m.schemas = make(map[string]Schema)
	}
	if m.watchers == nil {
		m.watchers = make(map[string][]*memiumWatcher)
	}
	m.schemas[name] = schema
	if _, ok := m.tables[name]; !ok {
		m.tables[name] = []map[string]any{}
	}
}

func (m *Memium) hasTable(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.tables[name]
	return ok
}

// Insert appends a raw row to a table for tests and fixtures.
func (m *Memium) Insert(table string, row map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tables == nil {
		m.tables = make(map[string][]map[string]any)
	}
	cp := make(map[string]any, len(row))
	for k, v := range row {
		cp[k] = v
	}
	m.tables[table] = append(m.tables[table], cp)
	m.fanOut(table, WatchInsert, nil, cp)
}

// Read dispatches a ReadIntent and returns matching rows.
func (m *Memium) Read(ctx core.Context, in ReadIntent) core.Result {
	start := core.Now()
	m.mu.RLock()
	defer m.mu.RUnlock()

	table := in.Schema.Name
	rows, ok := m.tables[table]
	if !ok {
		return core.Ok(&Payload{
			Data: []map[string]any{},
			Meta: Meta{Medium: "memium", Duration: core.Since(start)},
		})
	}

	// Apply predicates
	filtered := m.filterRows(rows, in.Where)
	if len(in.PK) > 0 && len(in.Schema.PK) > 0 {
		pkPreds := make([]Predicate, 0, len(in.PK))
		for i, value := range in.PK {
			if i >= len(in.Schema.PK) {
				break
			}
			pkPreds = append(pkPreds, Predicate{Field: in.Schema.PK[i], Op: "=", Value: value})
		}
		filtered = m.filterRows(filtered, pkPreds)
	}
	filtered = cloneRows(filtered)

	// Apply ordering
	if len(in.Order) > 0 {
		m.sortRows(filtered, in.Order)
	}

	// Apply limit/offset
	if in.Offset > 0 {
		if in.Offset < len(filtered) {
			filtered = filtered[in.Offset:]
		} else {
			filtered = nil
		}
	}
	if in.Limit > 0 && in.Limit < len(filtered) {
		filtered = filtered[:in.Limit]
	}

	// Apply With (eager loading via linear-scan client-side join)
	if len(in.With) > 0 {
		filtered = m.eagerLoad(filtered, in.Schema, in.With)
	}

	return core.Ok(&Payload{
		Data: filtered,
		Meta: Meta{
			Medium:   "memium",
			Duration: core.Since(start),
			RowsRead: int64(len(filtered)),
		},
	})
}

// Write dispatches a WriteIntent (Insert/Update/Save/Delete).
func (m *Memium) Write(ctx core.Context, in WriteIntent) core.Result {
	start := core.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	table := in.Schema.Name
	if m.tables[table] == nil {
		m.tables[table] = make([]map[string]any, 0)
	}

	var rowsWritt int64

	if in.Op == OpUpdate && len(in.Rows) == 0 {
		written, update := m.updateMatching(table, in.Schema, in.Where, in.Updates)
		if !update.OK {
			return update
		}
		rowsWritt = written
		return core.Ok(&Payload{
			Data: rowMap{},
			Meta: Meta{Medium: "memium", Duration: core.Since(start), RowsWrit: rowsWritt},
		})
	}

	if in.Op == OpDelete && len(in.Rows) == 0 && len(in.Where) > 0 {
		predRows := m.filterRows(m.tables[table], in.Where)
		newRows := m.tables[table][:0]
		for _, row := range m.tables[table] {
			if !m.rowInSlice(row, predRows) {
				newRows = append(newRows, row)
			} else {
				m.fanOut(table, WatchDelete, row, nil)
				rowsWritt++
			}
		}
		m.tables[table] = newRows
		return core.Ok(&Payload{
			Data: rowMap{},
			Meta: Meta{Medium: "memium", Duration: core.Since(start), RowsWrit: rowsWritt},
		})
	}

	for _, row := range in.Rows {
		rowMap, ok := row.(map[string]any)
		if !ok {
			rowMap = rowToSchemaMap(in.Schema, row)
		}

		switch in.Op {
		case OpInsert:
			m.nextID++
			if len(in.Schema.PK) > 0 {
				// If no PK set, assign auto ID to first PK field
				pkField := in.Schema.PK[0]
				if _, exists := rowMap[pkField]; !exists || isZero(rowMap[pkField]) {
					rowMap[pkField] = m.nextID
				}
			}
			if unique := m.checkUnique(table, in.Schema, rowMap, -1); !unique.OK {
				return unique
			}
			stored := cloneRow(rowMap)
			m.tables[table] = append(m.tables[table], stored)
			m.fanOut(table, WatchInsert, nil, stored)
			rowsWritt++

		case OpUpdate:
			written, update := m.updateMatching(table, in.Schema, in.Where, in.Updates)
			if !update.OK {
				return update
			}
			rowsWritt += written

		case OpSave:
			// Find by PK
			if len(in.Schema.PK) > 0 {
				found := false
				for i, existing := range m.tables[table] {
					if m.matchPK(existing, rowMap, in.Schema.PK) {
						before := cloneRow(existing)
						candidate := cloneRow(existing)
						// Update
						for k, v := range rowMap {
							candidate[k] = v
						}
						if unique := m.checkUnique(table, in.Schema, candidate, i); !unique.OK {
							return unique
						}
						for k, v := range candidate {
							m.tables[table][i][k] = v
						}
						m.fanOut(table, WatchUpdate, before, m.tables[table][i])
						found = true
						rowsWritt++
						break
					}
				}
				if !found {
					m.nextID++
					pkField := in.Schema.PK[0]
					if _, exists := rowMap[pkField]; !exists || isZero(rowMap[pkField]) {
						rowMap[pkField] = m.nextID
					}
					if unique := m.checkUnique(table, in.Schema, rowMap, -1); !unique.OK {
						return unique
					}
					stored := cloneRow(rowMap)
					m.tables[table] = append(m.tables[table], stored)
					m.fanOut(table, WatchInsert, nil, stored)
					rowsWritt++
				}
			} else {
				if unique := m.checkUnique(table, in.Schema, rowMap, -1); !unique.OK {
					return unique
				}
				stored := cloneRow(rowMap)
				m.tables[table] = append(m.tables[table], stored)
				m.fanOut(table, WatchInsert, nil, stored)
				rowsWritt++
			}

		case OpDelete:
			if len(in.Rows) > 0 {
				for _, delRow := range in.Rows {
					delMap, ok := delRow.(map[string]any)
					if !ok {
						delMap = rowToSchemaMap(in.Schema, delRow)
					}
					for i, existing := range m.tables[table] {
						if m.matchPK(existing, delMap, in.Schema.PK) {
							before := cloneRow(existing)
							m.tables[table] = append(m.tables[table][:i], m.tables[table][i+1:]...)
							m.fanOut(table, WatchDelete, before, nil)
							rowsWritt++
							break
						}
					}
				}
			}
		}
	}

	return core.Ok(&Payload{
		Data: rowMap{},
		Meta: Meta{
			Medium:   "memium",
			Duration: core.Since(start),
			RowsWrit: rowsWritt,
		},
	})
}

// Stream dispatches a ReadIntent and returns core.Seq[map[string]any].
func (m *Memium) Stream(ctx core.Context, in ReadIntent) core.Result {
	r := m.Read(ctx, in)
	if !r.OK {
		return r
	}
	payload := r.Value.(*Payload)
	rows, _ := payload.Data.([]map[string]any)

	seq := func(yield func(map[string]any) bool) {
		for _, row := range rows {
			select {
			case <-ctx.Done():
				return
			default:
				if !yield(row) {
					return
				}
			}
		}
	}

	return core.Ok(&Payload{
		Data: seq,
		Meta: payload.Meta,
	})
}

// Watch implements CDC subscriptions — inserts, updates, and deletes
// broadcast through in-process channels.
func (m *Memium) Watch(ctx core.Context, in ReadIntent) core.Result {
	table := in.Schema.Name
	watcher := &memiumWatcher{
		preds: in.Where,
		ch:    make(chan Event[map[string]any], 64),
		done:  make(chan struct{}),
	}
	m.mu.Lock()
	if m.watchers == nil {
		m.watchers = make(map[string][]*memiumWatcher)
	}
	m.watchers[table] = append(m.watchers[table], watcher)
	snapshot := make([]map[string]any, 0)
	for _, row := range m.filterRows(m.tables[table], in.Where) {
		snapshot = append(snapshot, cloneRow(row))
	}
	m.mu.Unlock()

	live := in.Watch != nil && in.Watch.Live
	seq := func(yield func(Event[map[string]any]) bool) {
		defer m.removeWatcher(table, watcher)
		if !live {
			for _, row := range snapshot {
				if !yield(Event[map[string]any]{
					Op:     WatchInitial,
					After:  row,
					Time:   core.Now(),
					Source: "memium",
				}) {
					return
				}
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-watcher.ch:
				mapped, ok := m.watchEventForPredicates(ev, watcher.preds)
				if !ok {
					continue
				}
				if !yield(mapped) {
					return
				}
			}
		}
	}

	return core.Ok(&Payload{
		Data: seq,
		Meta: Meta{Medium: "memium", RowsRead: int64(len(snapshot))},
	})
}

func (m *Memium) removeWatcher(table string, watcher *memiumWatcher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	watchers := m.watchers[table]
	for i, existing := range watchers {
		if existing == watcher {
			m.watchers[table] = append(watchers[:i], watchers[i+1:]...)
			watcher.close()
			return
		}
	}
}

func (m *Memium) fanOut(table string, op WatchOp, before, after map[string]any) {
	for _, watcher := range m.watchers[table] {
		ev := Event[map[string]any]{
			Op:     op,
			Before: cloneRow(before),
			After:  cloneRow(after),
			Time:   core.Now(),
			Source: "memium",
		}
		watcher.enqueue(ev)
	}
}

func (w *memiumWatcher) enqueue(ev Event[map[string]any]) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	select {
	case w.ch <- ev:
		w.mu.Unlock()
		return
	default:
	}
	w.queue = append(w.queue, ev)
	if !w.draining {
		w.draining = true
		go w.drain()
	}
	w.mu.Unlock()
}

func (w *memiumWatcher) drain() {
	for {
		w.mu.Lock()
		if w.closed || len(w.queue) == 0 {
			w.draining = false
			w.mu.Unlock()
			return
		}
		ev := w.queue[0]
		copy(w.queue, w.queue[1:])
		w.queue = w.queue[:len(w.queue)-1]
		w.mu.Unlock()

		select {
		case w.ch <- ev:
		case <-w.done:
			return
		}
	}
}

func (w *memiumWatcher) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.closed = true
	w.queue = nil
	close(w.done)
}

func (m *Memium) watchEventForPredicates(ev Event[map[string]any], preds []Predicate) (Event[map[string]any], bool) {
	beforeMatch := ev.Before != nil && m.matchPredicates(ev.Before, preds)
	afterMatch := ev.After != nil && m.matchPredicates(ev.After, preds)
	switch ev.Op {
	case WatchInsert:
		return ev, afterMatch
	case WatchDelete:
		return ev, beforeMatch
	case WatchUpdate:
		switch {
		case beforeMatch && afterMatch:
			return ev, true
		case !beforeMatch && afterMatch:
			ev.Op = WatchInsert
			ev.Before = nil
			return ev, true
		case beforeMatch && !afterMatch:
			ev.Op = WatchDelete
			ev.After = nil
			return ev, true
		default:
			return Event[map[string]any]{}, false
		}
	case WatchInitial:
		return ev, true
	default:
		return Event[map[string]any]{}, false
	}
}

// Search implements basic case-insensitive substring text search.
func (m *Memium) Search(ctx core.Context, in ReadIntent) core.Result {
	start := core.Now()
	m.mu.RLock()
	defer m.mu.RUnlock()

	table := in.Schema.Name
	rows, ok := m.tables[table]
	if !ok {
		return core.Ok(&Payload{
			Meta: Meta{Medium: "memium", Duration: core.Since(start)},
		})
	}

	var results []RankedResult
	if in.Search != nil && in.Search.Query != "" {
		query := core.Lower(in.Search.Query)
		limit := in.Search.Limit
		if limit <= 0 {
			limit = len(rows)
		}

		type scored struct {
			row   map[string]any
			score float64
			high  map[string]string
		}
		var scoredList []scored
		for _, row := range rows {
			var bestScore float64
			highlights := make(map[string]string)
			for _, f := range in.Schema.Fields {
				if f.SearchableKind != "" {
					if val, ok := row[f.Name]; ok {
						s, _ := val.(string)
						sLower := core.Lower(s)
						if core.Contains(sLower, query) {
							score := float64(len(query)) / float64(len(sLower)+1)
							if score > bestScore {
								bestScore = score
							}
							if in.Search.Highlight {
								highlights[f.Name] = s
							}
						}
					}
				}
			}
			if bestScore > 0 {
				scoredList = append(scoredList, scored{row: row, score: bestScore, high: highlights})
			}
		}

		// Sort by score descending
		for i := 0; i < len(scoredList); i++ {
			for j := i + 1; j < len(scoredList); j++ {
				if scoredList[j].score > scoredList[i].score {
					scoredList[i], scoredList[j] = scoredList[j], scoredList[i]
				}
			}
		}

		for i, s := range scoredList {
			if i >= limit {
				break
			}
			results = append(results, RankedResult{
				Value:      s.row,
				Score:      s.score,
				Highlights: s.high,
			})
		}
	}

	return core.Ok(&Payload{
		Data: results,
		Meta: Meta{
			Medium:   "memium",
			Duration: core.Since(start),
			RowsRead: int64(len(results)),
		},
	})
}

// --- Helper types ---

// Event describes a single observed change in a Watch subscription.
type Event[T any] struct {
	Op     WatchOp
	Before T
	After  T
	Time   core.Time
	Source string
}

// WatchOp is the operation type for a Watch event.
type WatchOp int

const (
	WatchInitial WatchOp = iota
	WatchInsert
	WatchUpdate
	WatchDelete
)

const (
	OpInitial = WatchInitial
)

// RankedResult is a search result with score and optional highlights.
type RankedResult struct {
	Value      map[string]any
	Score      float64
	Highlights map[string]string
	Distance   float64
	Facets     map[string]map[string]int
}

type rowMap map[string]any

// --- Internal helpers ---

// filterRows applies predicate filters to a slice of rows.
func (m *Memium) filterRows(rows []map[string]any, preds []Predicate) []map[string]any {
	if len(preds) == 0 {
		return rows
	}

	var result []map[string]any
	for _, row := range rows {
		if m.matchPredicates(row, preds) {
			result = append(result, row)
		}
	}
	return result
}

// matchPredicates checks if a row matches all predicates.
// Predicates with OR:true form an OR chain with the previous predicate.
func (m *Memium) matchPredicates(row map[string]any, preds []Predicate) bool {
	if len(preds) == 0 {
		return true
	}

	// Group consecutive predicates into: AND of OR-groups
	// Example: A, B(OR), C, D(OR), E(OR) → (A OR B) AND C AND (D OR E)
	i := 0
	for i < len(preds) {
		// Gather current OR-group starting at i
		start := i
		for i+1 < len(preds) && preds[i+1].OR {
			i++
		}
		// Now preds[start..i] is an OR-group
		// If any member matches, the group matches
		groupMatch := false
		for j := start; j <= i; j++ {
			if m.matchSinglePredicate(row, preds[j]) {
				groupMatch = true
				break
			}
		}
		if !groupMatch {
			return false
		}
		i++
	}
	return true
}

// matchSinglePredicate checks if a single row matches a single predicate (no OR/group logic).
func (m *Memium) matchSinglePredicate(row map[string]any, p Predicate) bool {
	if len(p.Group) > 0 {
		return m.matchGroup(row, p.Group)
	}
	val, exists := row[p.Field]
	op := core.Lower(p.Op)

	switch op {
	case "null":
		return !exists || val == nil
	case "not null":
		return exists && val != nil
	}
	if !exists {
		return false
	}

	switch op {
	case "=":
		return equal(val, p.Value)
	case "!=":
		return !equal(val, p.Value)
	case "<":
		return compareNum(val, p.Value) < 0
	case "<=":
		return compareNum(val, p.Value) <= 0
	case ">":
		return compareNum(val, p.Value) > 0
	case ">=":
		return compareNum(val, p.Value) >= 0
	case "in":
		return inSlice(val, p.Value)
	case "not in":
		return !inSlice(val, p.Value)
	case "like":
		return matchLike(val, p.Value)
	case "not like":
		return !matchLike(val, p.Value)
	case "between":
		return matchBetween(val, p.Value)
	default:
		return false
	}
}

// matchGroup checks if any predicate in a group matches (OR semantics).
func (m *Memium) matchGroup(row map[string]any, group []Predicate) bool {
	for _, p := range group {
		if m.matchSinglePredicate(row, p) {
			return true
		}
	}
	return false
}

func equal(a, b any) bool {
	return core.DeepEqual(a, b)
}

func compareNum(a, b any) int {
	aFloat := toFloat(a)
	bFloat := toFloat(b)
	if aFloat < bFloat {
		return -1
	}
	if aFloat > bFloat {
		return 1
	}
	return 0
}

func toFloat(v any) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case float64:
		return val
	case int32:
		return float64(val)
	default:
		return 0
	}
}

func inSlice(val, target any) bool {
	slice, ok := target.([]any)
	if !ok {
		return false
	}
	for _, item := range slice {
		if equal(val, item) {
			return true
		}
	}
	return false
}

func matchLike(val, pattern any) bool {
	s, ok1 := val.(string)
	p, ok2 := pattern.(string)
	if !ok1 || !ok2 {
		return false
	}
	return simpleLike(s, p)
}

func simpleLike(s, pattern string) bool {
	// Strip leading/trailing % for simple contains check
	if core.HasPrefix(pattern, "%") && core.HasSuffix(pattern, "%") {
		inner := pattern[1 : len(pattern)-1]
		return core.Contains(s, inner)
	}
	if core.HasPrefix(pattern, "%") {
		return core.HasSuffix(s, pattern[1:])
	}
	if core.HasSuffix(pattern, "%") {
		return core.HasPrefix(s, pattern[:len(pattern)-1])
	}
	return s == pattern
}

func matchBetween(val, target any) bool {
	pair, ok := target.([]any)
	if !ok || len(pair) != 2 {
		return false
	}
	return compareNum(val, pair[0]) >= 0 && compareNum(val, pair[1]) <= 0
}

// sortRows sorts a slice of rows (in-place) by the given orderings.
func (m *Memium) sortRows(rows []map[string]any, order []OrderBy) {
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if compareRows(rows[i], rows[j], order) > 0 {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
}

func compareRows(a, b map[string]any, order []OrderBy) int {
	for _, o := range order {
		cmp := compareValues(a[o.Field], b[o.Field])
		if o.Desc {
			cmp = -cmp
		}
		if cmp != 0 {
			return cmp
		}
	}
	return 0
}

func compareValues(a, b any) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	switch av := a.(type) {
	case string:
		if bv, ok := b.(string); ok {
			if av < bv {
				return -1
			}
			if av > bv {
				return 1
			}
			return 0
		}
	case int64:
		return compareNum(av, b)
	case int:
		return compareNum(av, b)
	case float64:
		return compareNum(av, b)
	case bool:
		if bv, ok := b.(bool); ok {
			if !av && bv {
				return -1
			}
			if av && !bv {
				return 1
			}
			return 0
		}
	case core.Time:
		if bv, ok := b.(core.Time); ok {
			if av.Before(bv) {
				return -1
			}
			if av.After(bv) {
				return 1
			}
			return 0
		}
	}
	return 0
}

// eagerLoad performs client-side eager loading for With relations.
func (m *Memium) eagerLoad(rows []map[string]any, schema Schema, with []string) []map[string]any {
	for _, relationName := range with {
		// Find the relation in the schema
		for _, rel := range schema.Relations {
			if rel.Name == relationName {
				sourcePK := primaryKeyName(schema)
				relatedTable, _ := m.findTableByName(rel.Name)
				if relatedTable == "" {
					continue
				}
				relatedSchema := m.schemas[relatedTable]
				relatedPK := primaryKeyName(relatedSchema)
				switch rel.Kind {
				case "hasMany":
					// For each row, load related rows where FK matches
					for i, row := range rows {
						if pkVal, ok := row[sourcePK]; ok {
							var matches []map[string]any
							for _, relRow := range m.tables[relatedTable] {
								if equal(relRow[rel.FK], pkVal) {
									matches = append(matches, cloneRow(relRow))
								}
							}
							rows[i][rel.Name] = matches
						}
					}

				case "belongsTo":
					for i, row := range rows {
						if fkVal, ok := row[rel.FK]; ok {
							for _, relRow := range m.tables[relatedTable] {
								if equal(relRow[relatedPK], fkVal) {
									rows[i][rel.Name] = cloneRow(relRow)
									break
								}
							}
						}
					}

				case "hasOne":
					for i, row := range rows {
						if pkVal, ok := row[sourcePK]; ok {
							for _, relRow := range m.tables[relatedTable] {
								if equal(relRow[rel.FK], pkVal) {
									rows[i][rel.Name] = cloneRow(relRow)
									break
								}
							}
						}
					}
				}
			}
		}
	}
	return rows
}

// findTableByName looks up a table name from relations by trying the relation name.
func (m *Memium) findTableByName(name string) (string, []map[string]any) {
	if rows, ok := m.tables[name]; ok {
		return name, rows
	}
	return "", nil
}

func primaryKeyName(schema Schema) string {
	if len(schema.PK) > 0 {
		return schema.PK[0]
	}
	return "id"
}

func isZero(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case int64:
		return val == 0
	case int:
		return val == 0
	case string:
		return val == ""
	case float64:
		return val == 0
	}
	return false
}

// matchPK checks if two rows have the same primary key values.
func (m *Memium) matchPK(a, b map[string]any, pkFields []string) bool {
	for _, pk := range pkFields {
		av := a[pk]
		bv := b[pk]
		if !equal(av, bv) {
			return false
		}
	}
	return true
}

// rowInSlice checks if a row exists in a slice of rows.
func (m *Memium) rowInSlice(row map[string]any, slice []map[string]any) bool {
	for _, r := range slice {
		if core.DeepEqual(row, r) {
			return true
		}
	}
	return false
}

func (m *Memium) updateMatching(table string, schema Schema, where []Predicate, updates map[string]any) (int64, core.Result) {
	predRows := m.filterRows(m.tables[table], where)
	candidates := make(map[int]map[string]any)
	for i := range m.tables[table] {
		if !m.rowInSlice(m.tables[table][i], predRows) {
			continue
		}
		candidate := cloneRow(m.tables[table][i])
		for k, v := range updates {
			candidate[k] = v
		}
		candidates[i] = candidate
	}
	if unique := m.checkUniqueBatch(table, schema, candidates); !unique.OK {
		return 0, unique
	}
	var rowsWritt int64
	for i, candidate := range candidates {
		before := cloneRow(m.tables[table][i])
		m.tables[table][i] = candidate
		m.fanOut(table, WatchUpdate, before, candidate)
		rowsWritt++
	}
	return rowsWritt, core.Ok(nil)
}

func (m *Memium) checkUnique(table string, schema Schema, row map[string]any, ignoreIndex int) core.Result {
	for _, field := range schema.Fields {
		if !hasConstraint(field, "unique") {
			continue
		}
		value, ok := row[field.Name]
		if !ok || isNilOrZero(value) {
			continue
		}
		for i, existing := range m.tables[table] {
			if i == ignoreIndex {
				continue
			}
			if equal(existing[field.Name], value) {
				return core.Fail(core.NewCode("orm.constraint", "unique constraint violation: "+field.Name))
			}
		}
	}
	return core.Ok(nil)
}

func (m *Memium) checkUniqueBatch(table string, schema Schema, candidates map[int]map[string]any) core.Result {
	for _, field := range schema.Fields {
		if !hasConstraint(field, "unique") {
			continue
		}
		seen := make(map[string]bool)
		for i, existing := range m.tables[table] {
			row := existing
			if candidate, ok := candidates[i]; ok {
				row = candidate
			}
			value, ok := row[field.Name]
			if !ok || isNilOrZero(value) {
				continue
			}
			key := core.Sprintf("%#v", value)
			if seen[key] {
				return core.Fail(core.NewCode("orm.constraint", "unique constraint violation: "+field.Name))
			}
			seen[key] = true
		}
	}
	return core.Ok(nil)
}

func cloneRow(row map[string]any) map[string]any {
	if row == nil {
		return nil
	}
	out := make(map[string]any, len(row))
	for k, v := range row {
		out[k] = v
	}
	return out
}

func cloneRows(rows []map[string]any) []map[string]any {
	if rows == nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		out[i] = cloneRow(row)
	}
	return out
}

type memiumSnapshot struct {
	tables map[string][]map[string]any
	nextID int64
}

func (m *Memium) snapshot() any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tables := make(map[string][]map[string]any, len(m.tables))
	for name, rows := range m.tables {
		tables[name] = cloneRows(rows)
	}
	return memiumSnapshot{tables: tables, nextID: m.nextID}
}

func (m *Memium) restore(state any) {
	snapshot, ok := state.(memiumSnapshot)
	if !ok {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tables = snapshot.tables
	m.nextID = snapshot.nextID
}

// structToMap converts a struct to map[string]any using reflection.
func structToMap(s any) map[string]any {
	result := make(map[string]any)
	v := core.ValueOf(s)
	if v.Kind() == core.KindPointer {
		if v.IsNil() {
			return result
		}
		v = v.Elem()
	}
	if v.Kind() != core.KindStruct {
		return result
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		fieldVal := v.Field(i).Interface()
		name, ok := storageFieldName(field.Name, field.Tag.Get("json"))
		if !ok {
			continue
		}
		result[name] = fieldVal
	}
	return result
}

func rowToSchemaMap(schema Schema, row any) map[string]any {
	if m, ok := row.(map[string]any); ok {
		out := make(map[string]any, len(m))
		for k, v := range m {
			out[k] = v
		}
		return out
	}
	raw := structToMap(row)
	out := make(map[string]any, len(raw))
	for _, f := range schema.Fields {
		for key, val := range raw {
			if normalizeName(key) == normalizeName(f.Name) {
				out[f.Name] = val
				break
			}
		}
	}
	for _, pk := range schema.PK {
		if _, exists := out[pk]; exists {
			continue
		}
		for key, val := range raw {
			if normalizeName(key) == normalizeName(pk) {
				out[pk] = val
				break
			}
		}
	}
	return out
}

func normalizeName(name string) string {
	name = core.Lower(name)
	var out []rune
	for _, r := range name {
		if r == '_' || r == '-' {
			continue
		}
		out = append(out, r)
	}
	return string(out)
}
