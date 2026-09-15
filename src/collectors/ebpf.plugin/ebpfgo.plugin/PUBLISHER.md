# Publisher Service (Phase 6)

## Overview

The `PublisherService` is a singleton coordinator that manages the unified metrics store for all eBPF collectors. It eliminates redundant per-collector store instances by providing a single shared memory publisher.

## Architecture

**Before Phase 6** (redundant stores):
```
CachestatCollector → metrix.CollectorStore
DCStatCollector   → metrix.CollectorStore
FDCollector       → metrix.CollectorStore
SocketCollector   → metrix.CollectorStore
DNSCollector      → metrix.CollectorStore

Result: 5 independent store instances, each allocating and managing SHM
```

**After Phase 6** (unified publisher):
```
PublisherService (singleton) → metrix.CollectorStore (shared)
         ↑
  ┌──────┼──────┬──────┬──────┬──────┐
  │      │      │      │      │      │
Cachestat DCstat FD  Socket  DNS   (other)

Result: All collectors write to single shared store
```

## Usage

### Getting the Publisher

Every collector acquires the singleton in its constructor:

```go
func NewSocketCollector() *SocketCollector {
	return &SocketCollector{
		Config:    SocketConfig{...},
		publisher: GetPublisher(),  // Singleton instance
	}
}
```

### Writing Metrics

Collectors write metrics via the publisher's store:

```go
func (c *SocketCollector) Collect(ctx context.Context) error {
	meter := c.publisher.MetricStore().Write().SnapshotMeter("")
	meter.Counter("ipv4_send").ObserveTotal(float64(snapshot.Ipv4Send))
	// ...
}
```

### MetricStore Interface

Collectors implement the CollectorV2 interface by delegating to the publisher:

```go
func (c *SocketCollector) MetricStore() metrix.CollectorStore {
	return c.publisher.MetricStore()
}
```

## Implementation Details

### Singleton Pattern

Uses `sync.Once` for thread-safe initialization:

```go
var (
	publisherInstance *PublisherService
	publisherOnce     sync.Once
)

func GetPublisher() *PublisherService {
	publisherOnce.Do(func() {
		publisherInstance = &PublisherService{
			store: metrix.NewCollectorStore(),
		}
	})
	return publisherInstance
}
```

**Thread safety**: Multiple collectors (even in parallel) all call `GetPublisher()` and receive the same instance without contention.

### RWMutex Protection

Store access is protected by `sync.RWMutex`:

```go
func (p *PublisherService) MetricStore() metrix.CollectorStore {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.store
}
```

This allows concurrent reads by all collectors during the same collection cycle.

## Benefits

1. **Single SHM Allocation**: Only one shared memory buffer for all collectors, reducing memory overhead
2. **Coordinated Publishing**: All metrics flow through a single coordinator, enabling:
   - Unified metric deduplication
   - Consistent timestamp handling
   - Coordinated cache management
3. **Simplified Lifecycle**: Publisher shutdown cleans up all collector metrics at once
4. **Future Extensibility**: Easy to add metrics aggregation, filtering, or routing logic

## Collectors Refactored

All five eBPF collectors now use the publisher:

- `CachestatCollector`
- `DCStatCollector`
- `FDCollector`
- `SocketCollector`
- `DNSCollector`

Each replaces its own `store metrix.CollectorStore` field with `publisher *PublisherService`.

## Closure and Cleanup

The publisher provides a `Close()` method for graceful shutdown:

```go
publisher := GetPublisher()
defer publisher.Close()
```

Called from `main()` after the agent's lifecycle completes.

## Future Enhancements

1. **Metrics Registry**: Add function to list all active metrics across collectors
2. **Filtering**: Per-collector metric filters before writing to SHM
3. **Rate Limiting**: Coordinated rate limiting across all collectors
4. **Telemetry**: Built-in SHM usage and performance metrics
