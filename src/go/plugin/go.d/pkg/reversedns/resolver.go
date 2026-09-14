// SPDX-License-Identifier: GPL-3.0-or-later

package reversedns

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const (
	DefaultLookupTimeout = 500 * time.Millisecond
	DefaultPositiveTTL   = 24 * time.Hour
	DefaultNegativeTTL   = 5 * time.Minute
	DefaultMaxEntries    = 10_000
	DefaultMaxConcurrent = 32
)

var (
	ErrInvalidAddress = errors.New("invalid reverse DNS address")
	ErrNilContext     = errors.New("nil reverse DNS context")
)

// State is the cache state of a reverse-DNS result.
type State uint8

const (
	StateMiss State = iota
	StatePositive
	StateNegative
)

// Result is a normalized reverse-DNS result.
type Result struct {
	State State
	Name  string
}

// ScheduleState describes the outcome of a non-blocking lookup request.
type ScheduleState uint8

const (
	ScheduleInvalid ScheduleState = iota
	SchedulePositive
	ScheduleNegative
	ScheduleScheduled
	SchedulePending
	ScheduleSaturated
)

// LookupFunc performs a PTR lookup. Implementations must honor ctx cancellation.
type LookupFunc func(ctx context.Context, address string) ([]string, error)

// Config configures a Resolver. Non-positive limits and durations use defaults.
// Now must return non-decreasing values; Resolver also clamps concurrent
// observations to preserve expiry ordering.
type Config struct {
	Lookup        LookupFunc
	Now           func() time.Time
	LookupTimeout time.Duration
	PositiveTTL   time.Duration
	NegativeTTL   time.Duration
	MaxEntries    int
	MaxConcurrent int
}

type callState uint8

const (
	callQueued callState = iota
	callActive
)

type call struct {
	addr      netip.Addr
	state     callState
	done      chan struct{}
	result    Result
	completed bool
	waiters   int

	queuePrev *call
	queueNext *call
}

type callQueue struct {
	front *call
	back  *call
	len   int
}

func (q *callQueue) pushBack(c *call) {
	c.queuePrev = q.back
	c.queueNext = nil
	if q.back != nil {
		q.back.queueNext = c
	} else {
		q.front = c
	}
	q.back = c
	q.len++
}

func (q *callQueue) remove(c *call) {
	if c.queuePrev != nil {
		c.queuePrev.queueNext = c.queueNext
	} else {
		q.front = c.queueNext
	}
	if c.queueNext != nil {
		c.queueNext.queuePrev = c.queuePrev
	} else {
		q.back = c.queuePrev
	}
	c.queuePrev = nil
	c.queueNext = nil
	q.len--
}

func (q *callQueue) popFront() *call {
	c := q.front
	if c != nil {
		q.remove(c)
	}
	return c
}

// Resolver is a bounded, concurrency-safe reverse-DNS cache and scheduler.
// It owns no permanent worker goroutine and does not require closing.
type Resolver struct {
	mu sync.Mutex

	lookup         LookupFunc
	now            func() time.Time
	lookupTimeout  time.Duration
	positiveTTL    time.Duration
	negativeTTL    time.Duration
	maxEntries     int
	maxConcurrent  int
	protectedLimit int
	lastNow        time.Time

	cache          map[netip.Addr]*cacheEntry
	probationary   recencyList
	protected      recencyList
	positiveExpiry expiryList
	negativeExpiry expiryList

	calls  map[netip.Addr]*call
	queued callQueue
	active int
}

// New creates a Resolver.
func New(config Config) *Resolver {
	config = normalizeConfig(config)
	return &Resolver{
		lookup:         config.Lookup,
		now:            config.Now,
		lookupTimeout:  config.LookupTimeout,
		positiveTTL:    config.PositiveTTL,
		negativeTTL:    config.NegativeTTL,
		maxEntries:     config.MaxEntries,
		maxConcurrent:  config.MaxConcurrent,
		protectedLimit: config.MaxEntries * 4 / 5,
		cache:          make(map[netip.Addr]*cacheEntry),
		calls:          make(map[netip.Addr]*call),
	}
}

func normalizeConfig(config Config) Config {
	if config.Lookup == nil {
		config.Lookup = net.DefaultResolver.LookupAddr
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.LookupTimeout <= 0 {
		config.LookupTimeout = DefaultLookupTimeout
	}
	if config.PositiveTTL <= 0 {
		config.PositiveTTL = DefaultPositiveTTL
	}
	if config.NegativeTTL <= 0 {
		config.NegativeTTL = DefaultNegativeTTL
	}
	if config.MaxEntries <= 0 {
		config.MaxEntries = DefaultMaxEntries
	}
	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = DefaultMaxConcurrent
	}
	return config
}

// Lookup returns a fresh cached result without performing DNS I/O.
func (r *Resolver) Lookup(addr netip.Addr) Result {
	addr, ok := canonicalAddress(addr)
	if r == nil || !ok {
		return Result{State: StateMiss}
	}
	now := r.now()
	r.mu.Lock()
	now = r.advanceNowLocked(now)
	result, ok := r.lookupCacheLocked(addr, now)
	r.mu.Unlock()
	if !ok {
		return Result{State: StateMiss}
	}
	return result
}

// Resolve returns a cached result or waits for one coalesced lookup. Caller
// cancellation never cancels an already-active lookup.
func (r *Resolver) Resolve(ctx context.Context, addr netip.Addr) (Result, error) {
	addr, ok := canonicalAddress(addr)
	if r == nil || !ok {
		return Result{State: StateMiss}, ErrInvalidAddress
	}
	if ctx == nil {
		return Result{State: StateMiss}, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return Result{State: StateMiss}, err
	}

	now := r.now()
	r.mu.Lock()
	now = r.advanceNowLocked(now)
	if result, ok := r.lookupCacheLocked(addr, now); ok {
		r.mu.Unlock()
		return result, nil
	}
	if existing := r.calls[addr]; existing != nil {
		existing.waiters++
		r.mu.Unlock()
		return r.wait(ctx, existing)
	}

	c := &call{addr: addr, state: callQueued, done: make(chan struct{}), waiters: 1}
	r.calls[addr] = c
	r.queued.pushBack(c)
	launch := r.admitLocked()
	r.mu.Unlock()
	r.launch(launch)
	return r.wait(ctx, c)
}

// Schedule requests a lookup without blocking.
func (r *Resolver) Schedule(addr netip.Addr) ScheduleState {
	addr, ok := canonicalAddress(addr)
	if r == nil || !ok {
		return ScheduleInvalid
	}
	now := r.now()
	r.mu.Lock()
	now = r.advanceNowLocked(now)
	if result, ok := r.lookupCacheLocked(addr, now); ok {
		r.mu.Unlock()
		if result.State == StatePositive {
			return SchedulePositive
		}
		return ScheduleNegative
	}
	if r.calls[addr] != nil {
		r.mu.Unlock()
		return SchedulePending
	}
	if r.active >= r.maxConcurrent || r.queued.len > 0 {
		r.mu.Unlock()
		return ScheduleSaturated
	}

	c := &call{addr: addr, state: callActive, done: make(chan struct{})}
	r.calls[addr] = c
	r.active++
	r.mu.Unlock()
	go r.run(c)
	return ScheduleScheduled
}

func (r *Resolver) wait(ctx context.Context, c *call) (Result, error) {
	select {
	case <-c.done:
		return c.result, nil
	case <-ctx.Done():
		r.mu.Lock()
		if c.completed {
			result := c.result
			r.mu.Unlock()
			return result, nil
		}
		c.waiters--
		var launch []*call
		if c.state == callQueued && c.waiters == 0 {
			r.queued.remove(c)
			delete(r.calls, c.addr)
			launch = r.admitLocked()
		}
		r.mu.Unlock()
		r.launch(launch)
		return Result{State: StateMiss}, ctx.Err()
	}
}

func (r *Resolver) run(c *call) {
	ctx, cancel := context.WithTimeout(context.Background(), r.lookupTimeout)
	names, err := r.lookup(ctx, c.addr.String())
	cancel()

	result := Result{State: StateNegative}
	if err == nil {
		if name := normalizePTRNames(names); name != "" {
			result = Result{State: StatePositive, Name: name}
		}
	}
	r.complete(c, result, r.now())
}

func (r *Resolver) complete(c *call, result Result, now time.Time) {
	r.mu.Lock()
	now = r.advanceNowLocked(now)
	r.purgeExpiredLocked(now)
	r.insertCacheLocked(c.addr, result, now)
	c.result = result
	c.completed = true
	delete(r.calls, c.addr)
	r.active--
	close(c.done)
	launch := r.admitLocked()
	r.mu.Unlock()
	r.launch(launch)
}

func (r *Resolver) admitLocked() []*call {
	var launch []*call
	for r.active < r.maxConcurrent && r.queued.len > 0 {
		c := r.queued.popFront()
		c.state = callActive
		r.active++
		launch = append(launch, c)
	}
	return launch
}

func (r *Resolver) launch(calls []*call) {
	for _, c := range calls {
		go r.run(c)
	}
}

func (r *Resolver) advanceNowLocked(now time.Time) time.Time {
	if now.Before(r.lastNow) {
		return r.lastNow
	}
	r.lastNow = now
	return now
}

func canonicalAddress(addr netip.Addr) (netip.Addr, bool) {
	if !addr.IsValid() {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func normalizePTRNames(names []string) string {
	best := ""
	for _, name := range names {
		name = strings.TrimSpace(name)
		name = strings.TrimSuffix(name, ".")
		name = strings.ToLower(name)
		if name != "" && (best == "" || name < best) {
			best = name
		}
	}
	return best
}
