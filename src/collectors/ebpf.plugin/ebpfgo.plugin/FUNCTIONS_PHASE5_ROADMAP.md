# Phase 5: Function Handlers - Implementation Roadmap

## Completed

### Socket Collector: network-protocols ✓

**Status**: COMPLETE

- `socketFunctionStore` holds per-cycle metrics
- `socketGlobalState` computes deltas from kernel snapshots
- Metrics written to MetricStore via SnapshotMeter
- `handleNetworkProtocolsFunction()` returns full JSON table via `buildNetworkProtocolsJSON()`
- Table response includes TCP/UDP statistics with rates, connections, errors, bytes

**Files**:
- `socket_collector_v2.go` — CollectorV2 with function support
- `socket_function.go` — Legacy stdin dispatcher (can be removed)
- `socket_global.go` — Global state and publish structures

### DNS Collector: dns-queries (Stub)

**Status**: PLACEHOLDER - Phase 5 follow-up

- `dnsFunctionStore` created (minimal state tracking)
- `handleDNSQueriesFunction()` returns placeholder response
- Full implementation deferred (requires flow aggregation)

**Why Deferred**: DNS uses flow-based data model (not global counters). Aggregating flows into a table response is more complex than socket's counter-delta approach.

**Files**:
- `dns_function.go` — Function handler stub
- `dns_collector_v2.go` — CollectorV2 with fnStore field

---

## TODO - Phase 5 Extensions

### Cachestat Collector: cachestat-stats

**What Exists**:
- `cachestatGlobalState` — delta computation from counters ✓
- `cachestatGlobalPublish` — per-cycle metrics ✓
- Collector already computes deltas in Collect() ✓

**Work Required**:
1. Create `cachestat_function.go`:
   - `cachestatFunctionStore` (like socket's)
   - `handleCachestatStatsFunction()` → JSON table response
   - Table schema: hit ratio, dirty pages, insert rates per cycle

2. Update `cachestat_collector_v2.go`:
   - Add `fnStore *cachestatFunctionStore` field
   - Initialize in `NewCachestatCollector()`
   - Call `c.fnStore.update(publish)` in `Collect()`

3. Create table builder similar to `buildNetworkProtocolsJSON()`:
   - Takes `cachestatGlobalPublish`
   - Returns JSON table with cache metrics

**Effort**: Medium (follows socket pattern exactly)

---

### DCstat Collector: dcstat-stats

**What Exists**:
- `dcstatGlobalState` — delta computation ✓
- `dcstatGlobalPublish` — per-cycle metrics ✓

**Work Required**:
1. Create `dcstat_function.go` (same pattern as cachestat)
2. Update `dcstat_collector_v2.go`
3. Create `buildDCStatJSON()` table builder

**Table Schema**: Directory cache hit/miss rates, insertion rates, per-cycle deltas

**Effort**: Medium (follows socket pattern exactly)

---

### FD Collector: fd-stats

**What Exists**:
- `fdGlobalState` — delta computation ✓
- `fdGlobalPublish` — per-cycle metrics (open/close calls, errors) ✓

**Work Required**:
1. Create `fd_function.go` (same pattern)
2. Update `fd_collector_v2.go`
3. Create `buildFDStatsJSON()` table builder

**Table Schema**: File descriptor open/close rates, errors, per-cycle totals

**Effort**: Medium (follows socket pattern exactly)

---

## Pattern Template

All new function handlers follow this structure:

### Step 1: Create function file (e.g., `xyz_function.go`)
```go
type xyzFunctionStore struct {
	mu      sync.RWMutex
	publish xyzGlobalPublish
	hasData bool
}

func (s *xyzFunctionStore) update(p xyzGlobalPublish) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publish = p
	s.hasData = true
}

func (c *XYZCollector) handleXYZFunction() (string, error) {
	publish, hasData := c.fnStore.snapshot()
	if !hasData {
		return "", fmt.Errorf("data not available")
	}
	// Call buildXYZJSON(publish, c.Config.UpdateEvery, expires)
}
```

### Step 2: Update collector (e.g., `xyz_collector_v2.go`)
```go
type XYZCollector struct {
	...
	fnStore *xyzFunctionStore
}

func NewXYZCollector() *XYZCollector {
	return &XYZCollector{
		...
		fnStore: newXYZFunctionStore(),
	}
}

func (c *XYZCollector) Collect(ctx context.Context) error {
	...
	publish, ok := c.state.Update(snapshot)
	if !ok { return nil }
	...
	if c.fnStore != nil {
		c.fnStore.update(publish)
	}
	return nil
}
```

### Step 3: Implement table builder (e.g., `buildXYZJSON()`)
- Takes `xyzGlobalPublish` + `updateEvery` + `expires`
- Returns JSON table with columns, data, charts, group_by
- Follow `buildNetworkProtocolsJSON()` structure

---

## Implementation Order Recommendation

1. **Cachestat** (simplest — mirrors socket exactly)
2. **FD** (straightforward counter aggregation)
3. **DCstat** (similar to FD)
4. **DNS** (complex — requires flow aggregation)

---

## Integration with Agent Framework

Once function handlers are complete, the agent's `composition.ProcessIngress` will automatically route FUNCTION calls to collectors. No manual stdin dispatch needed.

**Example flow**:
```
Agent receives: FUNCTION uid 10 "cachestat-stats" ...
  ↓
ProcessIngress detects collector name "cachestat"
  ↓
Routes to CachestatCollector.handleCachestatStatsFunction()
  ↓
Handler returns JSON table
  ↓
Agent formats pluginsd response via FUNCRESULT
```

No `runStdinDispatcher` equivalent needed — the agent framework handles all protocol details.
