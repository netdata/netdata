# Zabbix Preprocessing Library - API Design Specification

**Last Updated:** 2025-11-14
**Status:** Authoritative Requirements

---

## USER REQUIREMENTS (FINAL - DO NOT CHANGE)

### Input Parameters
The Netdata plugin will provide **TWO parameters**:
1. **Unique identifier** for continuity (state tracking)
2. **Just collected value** (the data to preprocess)

### State Management
- Library **MUST** maintain state based on the unique identifier
- Unique identifier is **unique per shard, NOT across shards**
- Example: "item1" in shard "A" is different from "item1" in shard "B"

### Output Format
The returned value of any collected sample is:
1. **Array of metrics** (optionally with labels)
2. **Possibly logs/events** to be committed to logs
3. **Structured errors** for the caller to handle and understand what happened

### Implementation Authority
**Everything else is implementation details that the library author decides.**

---

## IMPLEMENTATION DECISIONS (CLAUDE'S AUTHORITY)

### API Design Decision

**Chosen Approach:** Explicit itemID parameter + Result struct return

```go
// Execute processes a single preprocessing step for a specific item
func (p *Preprocessor) Execute(itemID string, value Value, step Step) (Result, error)

// ExecutePipeline processes multiple steps for a specific item
func (p *Preprocessor) ExecutePipeline(itemID string, value Value, steps []Step) (Result, error)

// Result contains the output of preprocessing
type Result struct {
    Metrics []Metric // Array of extracted metrics
    Logs    []string // Optional log entries (future use)
    Error   error    // Overall error if preprocessing failed
}

// Metric represents a single metric output
type Metric struct {
    Name   string            // Metric name
    Value  string            // Metric value
    Type   ValueType         // Value type
    Labels map[string]string // Optional labels
}
```

**Rationale:**
1. **itemID parameter**: User said "unique identifier for continuity" and "just collected value" as TWO parameters
2. **Result return**: User said "array of metrics" not single value
3. **Error in Result**: Allows partial success (some metrics succeed, some fail)

### State Key Format

State keys will be: `"{shardID}:{itemID}:{operation}:{params_hash}"`

- `shardID`: From NewPreprocessor(shardID)
- `itemID`: From Execute(itemID, ...)
- `operation`: "delta_value", "delta_speed", "throttle_value", "throttle_timed"
- `params_hash`: Hash of step.Params for multi-param operations

**Example:** `"shard1:item123:delta_value:"`

### Thread Safety

- State map already protected with sync.RWMutex (P0 #5 resolved)
- itemID is per-call, no shared state
- Multiple goroutines can call Execute() concurrently with different itemIDs

### Migration Path

**Phase 1 (Current):** Keep old API for backward compatibility
```go
// DEPRECATED: Use Execute(itemID, value, step) instead
func (p *Preprocessor) execute(value Value, step Step) (Value, error) {
    // Calls Execute("", value, step) - empty itemID = global state
}
```

**Phase 2 (After Netdata integration):** Remove deprecated API

### Multi-Metric Extraction

Currently, preprocessing operations return single values. Multi-metric extraction (JSONPath arrays, Prometheus scrapes, CSV rows) will be:

**Phase 1 (Current):** Single metric in array
```go
Result{
    Metrics: []Metric{{Name: "", Value: "result", Type: ValueTypeStr}},
}
```

**Phase 2 (Future):** Actual multi-metric extraction
```go
// JSONPath with array result
Result{
    Metrics: []Metric{
        {Name: "metric1", Value: "10", Labels: map[string]string{"index": "0"}},
        {Name: "metric2", Value: "20", Labels: map[string]string{"index": "1"}},
    },
}
```

### Persistence API

**Deferred to Phase 3** - not needed for initial Netdata integration:
```go
// SaveState exports preprocessor state for persistence
func (p *Preprocessor) SaveState() ([]byte, error)

// LoadState imports preprocessor state from persistence
func (p *Preprocessor) LoadState(data []byte) error
```

**Rationale:** Netdata can recreate preprocessor instances on restart. State loss acceptable for now.

---

## IMPLEMENTATION CHECKLIST

**Current Status:** Ready to implement P0 #3

### Tasks:
1. ✅ Document requirements (this file)
2. ⏳ Add itemID parameter to Execute() and ExecutePipeline()
3. ⏳ Update state key generation to use shardID + itemID
4. ⏳ Change return type from (Value, error) to (Result, error)
5. ⏳ Update all step functions to return Result
6. ⏳ Keep deprecated execute() for backward compatibility
7. ⏳ Update all 362 test cases to use new API
8. ⏳ Add tests for per-item state isolation
9. ⏳ Verify all tests still pass (362/362)

### Estimated Work: 1-2 days

---

## NOTES FOR FUTURE SESSIONS

**If discussing API design again:**
- **STOP** - Read this file first
- Requirements are FINAL (from user)
- Implementation decisions are FINAL (from Claude)
- Just implement, don't debate

**If user requests changes:**
- Update this file with new requirements
- Mark as FINAL
- Implement

**If API questions arise:**
- Refer to "IMPLEMENTATION DECISIONS" section
- Make decision, document, implement
- Don't ask user unless truly ambiguous

---

## BACKWARD COMPATIBILITY NOTES

**Breaking Changes:**
1. Execute() signature changes: `(value, step)` → `(itemID, value, step)`
2. Return type changes: `(Value, error)` → `(Result, error)`

**Migration:**
```go
// Old code:
result, err := p.Execute(value, step)

// New code:
result, err := p.Execute("item123", value, step)
// result is now Result{Metrics: []Metric{...}}
```

**Compatibility Layer:**
- Keep old execute() as private method
- Old tests can use it temporarily
- Remove after Netdata integration complete

---

## REJECTED ALTERNATIVES

**Why not add itemID to Value struct?**
- User said TWO parameters: identifier + value
- Pollutes Value with state management concerns
- Less explicit than parameter

**Why not SetItemContext()?**
- Not thread-safe without locking
- Awkward for concurrent processing
- Violates "explicit is better than implicit"

**Why not per-item Preprocessor instances?**
- Memory overhead (one instance per item)
- Complex lifecycle management
- User said "unique identifier for continuity" not "instance per item"

**Why not return ([]Metric, error)?**
- Need structured errors AND logs in future
- Result struct allows extension without API changes
- Matches user requirement for "logs/events"

---

## END OF SPECIFICATION

This document is the single source of truth for API design.
Do not re-discuss. Just implement.
