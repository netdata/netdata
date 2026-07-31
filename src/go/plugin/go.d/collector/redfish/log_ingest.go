// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfishruntime"
)

const (
	logAppendBatchSize       = 256
	maxLogEntriesPerPage     = 5_000
	maxLogRequestsPerPage    = 512
	maxLogResponseBytesCycle = 32 << 20
)

var (
	errClientContextUnsupported = errors.New("Redfish clientcontext is unsupported")
	errClientContextRestart     = errors.New("Redfish clientcontext must be restarted")
)

type logServiceCounters struct {
	Fetched             uint64
	Committed           uint64
	DuplicateSuppressed uint64
	Failed              uint64
	Reconciled          uint64
	LastComplete        time.Time
	NewestSource        time.Time
	LastMode            string
	LastPollDuration    time.Duration
	Reconciliation      map[string]uint64
	Blocked             bool
	Backfilling         bool
}

type logProducerState struct {
	mu                sync.Mutex
	services          map[string]logServiceCounters
	admitted          map[string]struct{}
	clientUnsupported map[string]bool
}

func (s *logProducerState) initialize() {
	if s.services == nil {
		s.services = make(map[string]logServiceCounters)
	}
	if s.admitted == nil {
		s.admitted = make(map[string]struct{})
	}
	if s.clientUnsupported == nil {
		s.clientUnsupported = make(map[string]bool)
	}
}

type collectedLogEntry struct {
	Journal       redfishruntime.JournalEntry
	RecordKey     string
	SourceKey     string
	SourceUsec    int64
	HasSourceTime bool
}

func (c *protocolClient) collectLogServices(
	ctx context.Context,
	graph *resourceGraph,
	observedAt time.Time,
	stats *wireStats,
) ([]hardwareObservation, string, map[string]int, []string, error) {
	counts := map[string]int{"limit": dereferenceInt(c.config.Logs.MaxServices)}
	if !c.config.Logs.enabled() {
		return nil, "disabled", counts, nil, nil
	}

	selectedNodes := c.filterSelectedSystem(graph, graph.emittedNodes())
	current := make(map[string]*graphNode)
	present := make(map[string]struct{})
	for _, node := range selectedNodes {
		if node.Kind != "log_service" || node.URI == "" {
			continue
		}
		present[node.Key] = struct{}{}
		counts["discovered"]++
		if c.logMatcher != nil && !c.logMatcher.MatchString(node.URI) {
			continue
		}
		counts["selected"]++
		if !c.metricPlacementReady(node) {
			continue
		}
		current[node.Key] = node
	}
	membershipComplete := logServiceMembershipComplete(graph, selectedNodes)

	c.logState.mu.Lock()
	if membershipComplete {
		c.logState.pruneAbsentLocked(present)
		c.logState.admitted = make(map[string]struct{}, len(current))
		if len(current) <= dereferenceInt(c.config.Logs.MaxServices) {
			for key := range current {
				c.logState.admitted[key] = struct{}{}
			}
		}
	}
	admittedKeys := make([]string, 0, len(c.logState.admitted))
	for key := range c.logState.admitted {
		admittedKeys = append(admittedKeys, key)
	}
	c.logState.mu.Unlock()
	sort.Strings(admittedKeys)
	logOrder := c.fairnessOrder("log-services", len(admittedKeys))

	if membershipComplete && counts["selected"] > dereferenceInt(c.config.Logs.MaxServices) {
		return nil, "over_limit", counts, nil, nil
	}
	if !membershipComplete {
		return c.logServiceMetricSnapshots(current, "unknown", observedAt), "unknown", counts, nil, nil
	}
	counts["admitted"] = len(admittedKeys)
	if c.logRuntime == nil || !c.logRuntime.BackendAvailable(c.logBackend) {
		return c.logServiceMetricSnapshots(current, "paused_backend", observedAt), "ready", counts, nil, nil
	}
	if c.cursor == nil {
		return c.logServiceMetricSnapshots(current, "blocked_source", observedAt), "unknown", counts,
			[]string{"Redfish log cursor is not configured"}, nil
	}
	claimed, err := c.cursor.Claim(ctx)
	if err != nil {
		return c.logServiceMetricSnapshots(current, "blocked_source", observedAt), "unknown", counts,
			[]string{boundedDiagnostic(err.Error())}, nil
	}
	if !claimed {
		return c.logServiceMetricSnapshots(current, "unknown", observedAt), "duplicate_owner", counts, nil, nil
	}
	activeCursorKeys := make([]string, 0, len(admittedKeys))
	for _, key := range admittedKeys {
		if node := current[key]; node != nil {
			activeCursorKeys = append(activeCursorKeys, c.logSourceCursorKey(node.URI))
		}
	}
	var diagnostics boundedDiagnosticAccumulator
	var failures boundedErrorAccumulator
	if err := c.cursor.TouchSources(activeCursorKeys, observedAt); err != nil {
		diagnostics.Add(err.Error())
		failures.Add(err)
	}
	attemptedServices := 0
	defer func() {
		step := attemptedServices
		if attemptedServices == len(admittedKeys) {
			step = 1
		}
		c.advanceFairnessCursor("log-services", len(admittedKeys), step)
	}()
	clientContextSupported := false
	if service := graph.findURI("service", "/redfish/v1/"); service != nil {
		clientContextSupported, _ = boolValueAt(service.Data, "ProtocolFeaturesSupported.ClientContextQuery")
	}
	for _, index := range logOrder {
		key := admittedKeys[index]
		node := current[key]
		if node == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			failures.Add(err)
			break
		}
		attemptedServices++
		if err := c.pollLogService(ctx, node, observedAt, stats, clientContextSupported); err != nil {
			failures.Add(fmt.Errorf("LogService %s: %w", node.URI, err))
			diagnostics.Add(fmt.Sprintf("LogService %s: %v", node.URI, err))
		}
	}
	if err := c.cursor.Persist(time.Now()); err != nil {
		// Journal durability already happened. Retain the new in-memory cursor,
		// continue normally, and let the coordinator's bounded backoff retry.
		diagnostics.Add("persist Redfish log cursor: " + err.Error())
	}
	return c.logServiceMetricSnapshots(current, "", observedAt), "ready", counts, diagnostics.Values(), failures.Err()
}

func (s *logProducerState) pruneAbsentLocked(present map[string]struct{}) {
	s.initialize()
	for key := range s.services {
		if _, ok := present[key]; !ok {
			delete(s.services, key)
		}
	}
	for key := range s.clientUnsupported {
		if _, ok := present[key]; !ok {
			delete(s.clientUnsupported, key)
		}
	}
}

func logServiceMembershipComplete(graph *resourceGraph, selectedNodes []*graphNode) bool {
	if graph == nil {
		return false
	}
	slices := make(map[string]bool)
	for _, slice := range graph.Slices {
		if slice.ChildKind != "log_service" {
			continue
		}
		slices[slice.ParentKey+"\x00"+slice.Path] = slice.Complete
	}
	for _, parent := range selectedNodes {
		if parent.Kind != "system" && parent.Kind != "chassis" && parent.Kind != "manager" {
			continue
		}
		if parent.AcquisitionState != "readable" || parent.Data == nil {
			return false
		}
		for _, relationship := range relationshipsFor(parent.Kind) {
			if relationship.ChildKind != "log_service" || !relationshipModeled(parent.Data, relationship.Path) {
				continue
			}
			complete, exists := slices[parent.Key+"\x00"+relationship.Path]
			if !exists || !complete {
				return false
			}
		}
	}
	return true
}

func relationshipModeled(data map[string]any, path string) bool {
	value, exists := jsonPath(data, path)
	return exists && value != nil
}

func (c *protocolClient) pollLogService(
	ctx context.Context,
	node *graphNode,
	observedAt time.Time,
	stats *wireStats,
	clientContextSupported bool,
) error {
	started := time.Now()
	entriesObject, ok := node.Data["Entries"].(map[string]any)
	if !ok {
		return errors.New("LogService does not advertise Entries")
	}
	entriesURI, ok := stringValue(entriesObject["@odata.id"])
	if !ok {
		return errors.New("LogService does not advertise Entries")
	}
	backend, ok := c.logRuntime.AcquireBackend(c.logBackend)
	if !ok {
		return redfishruntime.ErrBackendUnavailable
	}
	defer backend.Release()
	serviceCursorKey := c.logSourceCursorKey(node.URI)
	cursor := c.cursor.Source(serviceCursorKey)
	if !cursor.Initialized {
		cursor.Initialized = true
		cursor.ExactComplete = true
	}
	if cursor.ContextDirty {
		// The service may have advanced this opaque key before the process
		// stopped. Never reuse it after an unacknowledged result.
		cursor.ClientContext = ""
		cursor.ContextDirty = false
		cursor.Mode = "recovery"
		c.incrementReconciliation(node.Key, "client_context_restarted")
	}

	fullEvery := c.config.Logs.FullReconciliationEvery.Duration()
	fullDue := cursor.LastCompleteUsec == 0 || cursor.Mode == "recovery" ||
		cursor.ReconcileStarted || cursor.Continuation != ""
	if !fullDue && fullEvery > 0 {
		fullDue = observedAt.Sub(time.UnixMicro(cursor.LastCompleteUsec)) >= fullEvery
	}

	mode := "full"
	var entries []map[string]any
	var fullPage *logEntryPage
	if !fullDue && clientContextSupported && !c.clientContextUnsupported(node.Key) {
		if cursor.ClientContext == "" {
			contextKey, err := newClientContextKey()
			if err != nil {
				return err
			}
			cursor.ClientContext = contextKey
		}
		cursor.ContextDirty = true
		if err := c.cursor.CheckpointSource(serviceCursorKey, cursor, observedAt); err != nil {
			// A stateful request without this pre-checkpoint can lose entries
			// after a crash. Use the ordinary exact fallback for this poll.
			cursor.ContextDirty = false
			cursor.ClientContext = ""
			cursor.Mode = "recovery"
			c.incrementReconciliation(node.Key, "client_context_restarted")
			fullDue = true
		} else {
			var err error
			var skipped int
			entries, skipped, err = c.fetchClientContextEntries(
				ctx, entriesURI, cursor.ClientContext, stats,
			)
			switch {
			case err == nil:
				mode = "client_context"
				if skipped > 0 {
					c.updateLogCounters(node.Key, func(state *logServiceCounters) {
						state.Failed += uint64(skipped)
					})
				}
			case errors.Is(err, errClientContextUnsupported):
				c.disableClientContext(node.Key)
				cursor.ContextDirty = false
				cursor.ClientContext = ""
				c.incrementReconciliation(node.Key, "client_context_unsupported")
				fullDue = true
			default:
				// The source-side position may have advanced. Abandon this
				// key and recover from an ordinary exact scan.
				cursor.ContextDirty = false
				cursor.ClientContext = ""
				c.incrementReconciliation(node.Key, "client_context_restarted")
				fullDue = true
			}
		}
	}
	if fullDue || mode != "client_context" {
		if !cursor.ReconcileStarted {
			cursor.ReconcileStarted = true
			cursor.ReconcileExpected = 0
			cursor.ReconcileFetched = 0
			cursor.ReconcileSourceKeys = nil
			cursor.ReconcileRecordKeys = nil
			cursor.ContinuationKeys = nil
			cursor.Continuation = ""
		}
		target := entriesURI
		opaque := false
		if cursor.Continuation != "" {
			target = cursor.Continuation
			opaque = true
		}
		pageKey, err := c.logContinuationKey(target, opaque)
		if err != nil {
			return err
		}
		if stringSliceContains(cursor.ContinuationKeys, pageKey) {
			c.updateLogCounters(node.Key, func(state *logServiceCounters) {
				state.Failed++
				state.LastMode = "full"
				state.Blocked = true
			})
			cursor.Mode = "recovery"
			return errors.Join(
				errors.New("LogEntry pagination loop across collection cycles"),
				c.cursor.UpdateSource(serviceCursorKey, cursor, observedAt),
			)
		}
		page, err := c.fetchLogEntryPage(ctx, target, opaque, stats)
		if err != nil {
			c.updateLogCounters(node.Key, func(state *logServiceCounters) {
				state.Failed++
				state.LastMode = "unknown"
				state.Blocked = true
			})
			cursor.Mode = "recovery"
			cursorErr := c.cursor.UpdateSource(serviceCursorKey, cursor, observedAt)
			return errors.Join(err, cursorErr)
		}
		cursor.ContinuationKeys = append(cursor.ContinuationKeys, pageKey)
		if cursor.ReconcileFetched == 0 && cursor.Continuation == "" &&
			len(cursor.ReconcileRecordKeys) == 0 {
			cursor.ReconcileExpected = page.Count
		} else if cursor.ReconcileExpected != page.Count {
			c.updateLogCounters(node.Key, func(state *logServiceCounters) {
				state.Failed++
				state.LastMode = "full"
				state.Blocked = true
			})
			cursor.Mode = "recovery"
			cursor.ReconcileStarted = false
			cursor.Continuation = ""
			cursor.ContinuationKeys = nil
			err := errors.New("LogEntry @odata.count changed during reconciliation")
			return errors.Join(err, c.cursor.UpdateSource(serviceCursorKey, cursor, observedAt))
		}
		fullPage = &page
		reconcileSources := make(map[string]struct{}, len(cursor.ReconcileSourceKeys)+len(page.SourceKeys))
		for _, key := range cursor.ReconcileSourceKeys {
			reconcileSources[key] = struct{}{}
		}
		for _, key := range page.SourceKeys {
			if _, exists := reconcileSources[key]; exists {
				cursor.Mode = "recovery"
				cursor.ReconcileStarted = false
				cursor.Continuation = ""
				cursor.ContinuationKeys = nil
				c.updateLogCounters(node.Key, func(state *logServiceCounters) {
					state.Failed++
					state.Blocked = true
				})
				err := errors.New("LogEntry reconciliation contains duplicate source membership")
				return errors.Join(err, c.cursor.UpdateSource(serviceCursorKey, cursor, observedAt))
			}
			reconcileSources[key] = struct{}{}
		}
		if len(reconcileSources) > maxCursorExactKeys {
			cursor.Mode = "recovery"
			cursor.ReconcileStarted = false
			cursor.Continuation = ""
			cursor.ContinuationKeys = nil
			c.updateLogCounters(node.Key, func(state *logServiceCounters) {
				state.Failed++
				state.Blocked = true
			})
			err := errors.New("LogEntry reconciliation exceeds the exact membership evidence capacity")
			return errors.Join(err, c.cursor.UpdateSource(serviceCursorKey, cursor, observedAt))
		}
		cursor.ReconcileSourceKeys = mapKeys(reconcileSources)
		if page.Skipped > 0 {
			c.updateLogCounters(node.Key, func(state *logServiceCounters) {
				state.Failed += uint64(page.Skipped)
			})
		}
		cursor.ReconcileFetched += page.Observed
		cursor.Continuation = page.Next
		countMismatch := cursor.ReconcileFetched > cursor.ReconcileExpected ||
			(cursor.Continuation == "" && cursor.ReconcileFetched != cursor.ReconcileExpected)
		if countMismatch {
			cursor.Mode = "recovery"
			cursor.ReconcileStarted = false
			cursor.Continuation = ""
			cursor.ContinuationKeys = nil
			c.updateLogCounters(node.Key, func(state *logServiceCounters) {
				state.Failed++
				state.Blocked = true
			})
			err := fmt.Errorf(
				"LogEntry reconciliation observed %d members, advertised %d",
				cursor.ReconcileFetched,
				cursor.ReconcileExpected,
			)
			return errors.Join(err, c.cursor.UpdateSource(serviceCursorKey, cursor, observedAt))
		}
		if cursor.Continuation != "" {
			nextKey, err := c.logContinuationKey(cursor.Continuation, true)
			if err != nil {
				return err
			}
			if stringSliceContains(cursor.ContinuationKeys, nextKey) {
				cursor.Mode = "recovery"
				cursor.ReconcileStarted = false
				cursor.Continuation = ""
				cursor.ContinuationKeys = nil
				c.updateLogCounters(node.Key, func(state *logServiceCounters) {
					state.Failed++
					state.Blocked = true
				})
				err := errors.New("LogEntry pagination loop")
				return errors.Join(err, c.cursor.UpdateSource(serviceCursorKey, cursor, observedAt))
			}
		}
		entries = page.Entries
		mode = "full"
	}

	seen := make(map[string]struct{}, len(cursor.ExactRecordKeys))
	for _, key := range cursor.ExactRecordKeys {
		seen[key] = struct{}{}
	}
	boundaryKeys := make(map[string]struct{}, len(cursor.BoundaryKeys))
	for _, key := range cursor.BoundaryKeys {
		boundaryKeys[key] = struct{}{}
	}

	var pending []collectedLogEntry
	var ambiguous []collectedLogEntry
	var newest time.Time
	ordering := "unknown"
	orderingValid := true
	var previousSourceUsec int64
	havePrevious := false
	scanKeys := make(map[string]struct{}, len(cursor.ReconcileRecordKeys)+len(entries))
	for _, key := range cursor.ReconcileRecordKeys {
		scanKeys[key] = struct{}{}
	}
	for _, raw := range entries {
		c.updateLogCounters(node.Key, func(state *logServiceCounters) { state.Fetched++ })
		entry, err := c.normalizeLogEntry(node, raw, cursor.Generation, observedAt)
		if err != nil {
			c.updateLogCounters(node.Key, func(state *logServiceCounters) { state.Failed++ })
			continue
		}
		if mode == "full" {
			scanKeys[entry.RecordKey] = struct{}{}
		}
		if mode == "full" {
			if !entry.HasSourceTime {
				orderingValid = false
			} else if havePrevious {
				switch {
				case entry.SourceUsec > previousSourceUsec && ordering == "descending":
					orderingValid = false
				case entry.SourceUsec < previousSourceUsec && ordering == "ascending":
					orderingValid = false
				case ordering == "unknown" && entry.SourceUsec > previousSourceUsec:
					ordering = "ascending"
				case ordering == "unknown" && entry.SourceUsec < previousSourceUsec:
					ordering = "descending"
				}
			}
			previousSourceUsec = entry.SourceUsec
			havePrevious = true
		}
		if _, ok := seen[entry.RecordKey]; ok ||
			entry.SourceUsec == cursor.BoundaryUsec && containsKey(boundaryKeys, entry.RecordKey) {
			c.updateLogCounters(node.Key, func(state *logServiceCounters) { state.DuplicateSuppressed++ })
			continue
		}
		if !cursor.ExactComplete && !entry.HasSourceTime {
			ambiguous = append(ambiguous, entry)
			continue
		}
		if cursor.BoundaryUsec > 0 && entry.SourceUsec < cursor.BoundaryUsec {
			if !cursor.ExactComplete {
				ambiguous = append(ambiguous, entry)
				continue
			}
			c.incrementReconciliation(node.Key, "outside_tail_recovered_exact")
		}
		pending = append(pending, entry)
		if entry.HasSourceTime && entry.SourceUsec > 0 {
			when := time.UnixMicro(entry.SourceUsec)
			if when.After(newest) {
				newest = when
			}
		}
	}
	if len(ambiguous) > 0 {
		keys := make([]string, len(ambiguous))
		for i := range ambiguous {
			keys[i] = ambiguous[i].RecordKey
		}
		retained, proofErr := backend.Contains(ctx, keys)
		if proofErr != nil {
			return fmt.Errorf("classify ambiguous historical LogEntries: %w", proofErr)
		}
		unproved := 0
		for _, entry := range ambiguous {
			if retained[entry.RecordKey] {
				seen[entry.RecordKey] = struct{}{}
				c.updateLogCounters(node.Key, func(state *logServiceCounters) {
					state.DuplicateSuppressed++
				})
				continue
			}
			unproved++
		}
		if unproved > 0 {
			c.incrementReconciliation(node.Key, "outside_tail_ambiguous")
			c.updateLogCounters(node.Key, func(state *logServiceCounters) {
				state.Failed += uint64(unproved)
				state.Blocked = true
			})
			cursor.Mode = "recovery"
			cursorErr := c.cursor.UpdateSource(serviceCursorKey, cursor, observedAt)
			return errors.Join(
				fmt.Errorf(
					"%d historical LogEntries cannot be classified after exact evidence expired",
					unproved,
				),
				cursorErr,
			)
		}
	}

	sort.SliceStable(pending, func(i, j int) bool {
		if pending[i].SourceUsec != pending[j].SourceUsec {
			return pending[i].SourceUsec < pending[j].SourceUsec
		}
		return pending[i].RecordKey < pending[j].RecordKey
	})
	for len(pending) > 0 {
		size := min(logAppendBatchSize, len(pending))
		batch := pending[:size]
		journalEntries := make([]redfishruntime.JournalEntry, len(batch))
		for i := range batch {
			journalEntries[i] = batch[i].Journal
		}
		appendResult, err := backend.Append(ctx, journalEntries)
		if err != nil {
			c.updateLogCounters(node.Key, func(state *logServiceCounters) {
				state.Failed += uint64(len(batch))
			})
			if mode == "client_context" {
				c.incrementReconciliation(node.Key, "client_context_restarted")
				cursor.ClientContext = ""
				cursor.Mode = "recovery"
				if cursorErr := c.cursor.UpdateSource(serviceCursorKey, cursor, observedAt); cursorErr != nil {
					return errors.Join(err, cursorErr)
				}
			}
			return err
		}
		if appendResult.Committed < 0 || appendResult.DuplicateSuppressed < 0 ||
			appendResult.Committed+appendResult.DuplicateSuppressed != len(batch) {
			return fmt.Errorf(
				"journal classified %d committed and %d duplicate of %d LogEntries",
				appendResult.Committed, appendResult.DuplicateSuppressed, len(batch),
			)
		}
		c.updateLogCounters(node.Key, func(state *logServiceCounters) {
			state.Committed += uint64(appendResult.Committed)
			state.DuplicateSuppressed += uint64(appendResult.DuplicateSuppressed)
		})
		for _, entry := range batch {
			seen[entry.RecordKey] = struct{}{}
			if !entry.HasSourceTime {
				continue
			}
			if entry.SourceUsec > cursor.BoundaryUsec {
				cursor.BoundaryUsec = entry.SourceUsec
				boundaryKeys = map[string]struct{}{entry.RecordKey: {}}
			} else if entry.SourceUsec == cursor.BoundaryUsec {
				boundaryKeys[entry.RecordKey] = struct{}{}
			}
		}
		pending = pending[size:]
	}

	cursor.Mode = mode
	cursor.BoundaryKeys = mapKeys(boundaryKeys)
	if len(seen) <= maxCursorExactKeys {
		cursor.ExactRecordKeys = mapKeys(seen)
	} else {
		cursor.ExactRecordKeys = boundedSortedKeys(mapKeys(seen), maxCursorExactKeys)
		cursor.ExactComplete = false
	}
	if mode == "client_context" {
		cursor.ContextDirty = false
	}
	fullComplete := false
	if mode == "full" && fullPage != nil {
		cursor.ReconcileRecordKeys = boundedSortedKeys(mapKeys(scanKeys), maxCursorExactKeys)
		if cursor.Continuation == "" {
			boundaryLost := cursor.ReconcileExpected == 0 &&
				cursor.CompleteCountKnown && cursor.LastCompleteCount > 0
			if boundaryLost {
				if err := advanceLogGeneration(&cursor); err != nil {
					return err
				}
				c.incrementReconciliation(node.Key, "boundary_lost")
			}
			cursor.Mode = mode
			cursor.ReconcileStarted = false
			cursor.ReconcileExpected = 0
			cursor.ReconcileFetched = 0
			cursor.ReconcileSourceKeys = nil
			cursor.ReconcileRecordKeys = nil
			cursor.ContinuationKeys = nil
			cursor.LastCompleteUsec = observedAt.UnixMicro()
			cursor.CompleteCountKnown = true
			cursor.LastCompleteCount = fullPage.Count
			fullComplete = true
		}
	}
	if fullComplete {
		cursor.LastCompleteUsec = observedAt.UnixMicro()
		if !orderingValid {
			cursor.Ordering = "unordered"
			c.incrementReconciliation(node.Key, "ordering_disqualified")
		} else {
			cursor.Ordering = ordering
		}
		c.incrementReconciliation(node.Key, "completed")
	}
	if err := c.cursor.UpdateSource(serviceCursorKey, cursor, observedAt); err != nil {
		return err
	}
	c.updateLogCounters(node.Key, func(state *logServiceCounters) {
		if fullComplete {
			state.Reconciled++
			state.LastComplete = observedAt
		}
		state.LastMode = mode
		state.LastPollDuration = time.Since(started)
		state.Blocked = false
		state.Backfilling = mode == "full" && !fullComplete
		if newest.After(state.NewestSource) {
			state.NewestSource = newest
		}
	})
	return nil
}

type logEntryPage struct {
	Entries    []map[string]any
	SourceKeys []string
	Count      int
	Next       string
	Observed   int
	Skipped    int
}

func (c *protocolClient) fetchLogEntryPage(
	ctx context.Context,
	rawURI string,
	opaque bool,
	stats *wireStats,
) (logEntryPage, error) {
	target, err := c.resolveURI(c.root, rawURI, opaque)
	if err != nil {
		return logEntryPage{}, err
	}
	if err := consumeCollectionPageBudget(ctx); err != nil {
		return logEntryPage{}, err
	}
	pageCtx := withLogPageBudget(ctx)
	response, err := c.do(
		pageCtx,
		protocolRequest{method: http.MethodGet, target: target, auth: c.currentAuth(true)},
		stats,
		true,
		http.StatusOK,
	)
	if err != nil {
		return logEntryPage{}, err
	}
	var page struct {
		ODataID   string          `json:"@odata.id"`
		ODataType string          `json:"@odata.type"`
		Count     *int            `json:"Members@odata.count"`
		Members   json.RawMessage `json:"Members"`
		NextLink  string          `json:"Members@odata.nextLink"`
	}
	if err := decodeJSON(response, &page); err != nil {
		response.finish(err)
		return logEntryPage{}, err
	}
	pageID, err := c.resolveURI(response.url, page.ODataID, false)
	if err != nil ||
		!sameResourceIdentity(canonicalResourceURI(pageID), canonicalResourceURI(response.url)) {
		err = errors.New("LogEntry collection identity does not match the requested collection")
		response.finish(err)
		return logEntryPage{}, err
	}
	if err := validateCollectionSchemaType(page.ODataType, "log_entry"); err != nil {
		response.finish(err)
		return logEntryPage{}, err
	}
	if page.Count == nil || *page.Count < 0 {
		err := errors.New("LogEntry page has no valid @odata.count")
		response.finish(err)
		return logEntryPage{}, err
	}
	if len(page.Members) == 0 || string(bytes.TrimSpace(page.Members)) == "null" {
		err := errors.New("LogEntry page has no Members array")
		response.finish(err)
		return logEntryPage{}, err
	}
	var members []map[string]any
	if err := decodeJSONBytes(page.Members, &members); err != nil {
		err = errors.New("LogEntry Members is not an array")
		response.finish(err)
		return logEntryPage{}, err
	}
	if len(members) > maxLogEntriesPerPage {
		err := fmt.Errorf(
			"LogEntry page has %d members, limit is %d",
			len(members), maxLogEntriesPerPage,
		)
		response.finish(err)
		return logEntryPage{}, err
	}
	if err := consumeCollectionMemberBudget(ctx, len(members)); err != nil {
		response.finish(err)
		return logEntryPage{}, err
	}
	pageURL := response.url
	response.finish(nil)
	entries, skipped, err := c.resolveLogEntryMembers(pageCtx, pageURL, members, stats)
	if err != nil {
		return logEntryPage{}, err
	}
	sourceKeys, err := c.uniqueLogEntrySourceKeys(entries)
	if err != nil {
		return logEntryPage{}, err
	}
	next := ""
	if page.NextLink != "" {
		resolved, err := c.resolveURI(pageURL, page.NextLink, true)
		if err != nil {
			return logEntryPage{}, err
		}
		if resolved.String() == pageURL.String() {
			return logEntryPage{}, errors.New("LogEntry pagination loop")
		}
		next = resolved.RequestURI()
	}
	return logEntryPage{
		Entries:    entries,
		SourceKeys: sourceKeys,
		Count:      *page.Count,
		Next:       next,
		Observed:   len(members),
		Skipped:    skipped,
	}, nil
}

func (c *protocolClient) fetchClientContextEntries(
	ctx context.Context,
	rawURI string,
	contextKey string,
	stats *wireStats,
) ([]map[string]any, int, error) {
	target, err := c.resolveURI(c.root, rawURI, false)
	if err != nil {
		return nil, 0, err
	}
	target = addQuery(target, url.Values{"clientcontext": []string{contextKey}})
	collectionIdentity := canonicalResourceURI(target)
	seenPages := make(map[string]struct{})
	expectedCount := -1
	skipped := 0
	var entries []map[string]any
	sourceKeys := make(map[string]struct{})
	for {
		if err := consumeCollectionPageBudget(ctx); err != nil {
			return nil, skipped, err
		}
		if _, ok := seenPages[target.String()]; ok {
			return nil, skipped, errors.New("clientcontext pagination loop")
		}
		seenPages[target.String()] = struct{}{}
		pageCtx := withLogPageBudget(ctx)
		response, err := c.doOnce(
			pageCtx,
			protocolRequest{method: http.MethodGet, target: target, auth: c.currentAuth(true)},
			stats,
		)
		if err != nil {
			recordWireFailure(stats, classifyError(err))
			return nil, skipped, err
		}
		if response.status != http.StatusOK {
			recordWireFailure(stats, classifyHTTPStatus(response.status))
			if response.status == http.StatusBadRequest ||
				response.status == http.StatusNotImplemented {
				return nil, skipped, errClientContextUnsupported
			}
			if response.status == http.StatusNotFound || response.status == http.StatusUnauthorized {
				return nil, skipped, errClientContextRestart
			}
			return nil, skipped, statusError{
				status: response.status,
				class:  classifyHTTPStatus(response.status),
				path:   response.url.EscapedPath(),
			}
		}
		var page struct {
			ODataID       string          `json:"@odata.id"`
			ODataType     string          `json:"@odata.type"`
			Count         *int            `json:"Members@odata.count"`
			Members       json.RawMessage `json:"Members"`
			NextLink      string          `json:"Members@odata.nextLink"`
			ClientContext string          `json:"@Redfish.ClientContext"`
			Status        string          `json:"@Redfish.ClientContextStatus"`
			NextEntry     *string         `json:"@Redfish.NextEntry"`
		}
		if err := decodeJSON(response, &page); err != nil {
			response.finish(err)
			return nil, skipped, err
		}
		pageID, err := c.resolveURI(response.url, page.ODataID, false)
		if err != nil ||
			!sameResourceIdentity(canonicalResourceURI(pageID), collectionIdentity) {
			err = errors.New("clientcontext collection identity does not match the requested collection")
			response.finish(err)
			return nil, skipped, err
		}
		if err := validateCollectionSchemaType(page.ODataType, "log_entry"); err != nil {
			response.finish(err)
			return nil, skipped, err
		}
		if page.Count == nil || *page.Count < 0 {
			err := errors.New("clientcontext page has no valid @odata.count")
			response.finish(err)
			return nil, skipped, err
		}
		if expectedCount < 0 {
			expectedCount = *page.Count
		} else if expectedCount != *page.Count {
			err := errors.New("clientcontext @odata.count changed between pages")
			response.finish(err)
			return nil, skipped, err
		}
		if page.ClientContext != contextKey {
			response.finish(errClientContextRestart)
			return nil, skipped, errClientContextRestart
		}
		switch strings.ToLower(strings.TrimSpace(page.Status)) {
		case "applied":
		case "unsupported":
			response.finish(errClientContextUnsupported)
			return nil, skipped, errClientContextUnsupported
		default:
			response.finish(errClientContextRestart)
			return nil, skipped, errClientContextRestart
		}
		if page.NextEntry == nil || strings.TrimSpace(*page.NextEntry) == "" {
			response.finish(errClientContextRestart)
			return nil, skipped, errClientContextRestart
		}
		if len(page.Members) == 0 || string(bytes.TrimSpace(page.Members)) == "null" {
			err := errors.New("clientcontext page has no Members array")
			response.finish(err)
			return nil, skipped, err
		}
		var members []map[string]any
		if err := decodeJSONBytes(page.Members, &members); err != nil {
			err = errors.New("clientcontext Members is not an array")
			response.finish(err)
			return nil, skipped, err
		}
		if len(members) > maxLogEntriesPerPage {
			err := fmt.Errorf(
				"clientcontext page has %d members, limit is %d",
				len(members), maxLogEntriesPerPage,
			)
			response.finish(err)
			return nil, skipped, err
		}
		if err := consumeCollectionMemberBudget(ctx, len(members)); err != nil {
			response.finish(err)
			return nil, skipped, err
		}
		response.finish(nil)
		resolved, pageSkipped, err := c.resolveLogEntryMembers(pageCtx, target, members, stats)
		if err != nil {
			return nil, skipped + pageSkipped, err
		}
		keys, err := c.uniqueLogEntrySourceKeys(resolved)
		if err != nil {
			return nil, skipped + pageSkipped, err
		}
		for _, key := range keys {
			if _, exists := sourceKeys[key]; exists {
				return nil, skipped + pageSkipped, errors.New(
					"clientcontext result contains duplicate source membership",
				)
			}
			sourceKeys[key] = struct{}{}
		}
		entries = append(entries, resolved...)
		skipped += pageSkipped
		if page.NextLink == "" {
			break
		}
		target, err = c.resolveURI(response.url, page.NextLink, true)
		if err != nil {
			return nil, skipped, err
		}
	}
	return entries, skipped, nil
}

func (c *protocolClient) uniqueLogEntrySourceKeys(entries []map[string]any) ([]string, error) {
	keys := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		key, err := c.logEntrySourceKey("", entry)
		if err != nil {
			continue
		}
		if _, exists := seen[key]; exists {
			return nil, errors.New("LogEntry page contains duplicate source membership")
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (c *protocolClient) resolveLogEntryMembers(
	ctx context.Context,
	collectionURI *url.URL,
	members []map[string]any,
	stats *wireStats,
) ([]map[string]any, int, error) {
	result := make([]map[string]any, 0, len(members))
	skipped := 0
	for _, member := range members {
		if rawID, ok := stringValue(member["@odata.id"]); ok {
			resolved, err := c.resolveURI(collectionURI, rawID, false)
			if err != nil {
				skipped++
				continue
			}
			if logEntryMemberIsLinkOnly(member) {
				response, err := c.do(
					ctx,
					protocolRequest{method: http.MethodGet, target: resolved, auth: c.currentAuth(true)},
					stats,
					true,
					http.StatusOK,
				)
				if err != nil {
					return result, skipped, err
				}
				if err := decodeJSON(response, &member); err != nil {
					response.finish(err)
					skipped++
					continue
				}
				bodyID, ok := stringValue(member["@odata.id"])
				if !ok {
					err := errors.New("linked LogEntry has no usable @odata.id")
					response.finish(err)
					skipped++
					continue
				}
				bodyTarget, err := c.resolveURI(response.url, bodyID, false)
				if err != nil ||
					!sameResourceIdentity(canonicalResourceURI(bodyTarget), canonicalResourceURI(response.url)) {
					err = errors.New("linked LogEntry identity does not match the requested resource")
					response.finish(err)
					skipped++
					continue
				}
				odataType, _ := stringValue(member["@odata.type"])
				if err := validateResourceSchemaType("log_entry", odataType); err != nil {
					response.finish(err)
					skipped++
					continue
				}
				if err := validateRequiredLogEntryProperties(member, true); err != nil {
					response.finish(err)
					skipped++
					continue
				}
				response.finish(nil)
			}
		}
		result = append(result, member)
	}
	return result, skipped, nil
}

func logEntryMemberIsLinkOnly(member map[string]any) bool {
	if _, ok := stringValue(member["@odata.id"]); !ok {
		return false
	}
	for key := range member {
		if strings.HasPrefix(key, "@odata.") || strings.HasPrefix(key, "@Redfish.") {
			continue
		}
		return false
	}
	return true
}

func (c *protocolClient) normalizeLogEntry(
	service *graphNode,
	raw map[string]any,
	generation uint64,
	observedAt time.Time,
) (collectedLogEntry, error) {
	if err := validateRequiredLogEntryProperties(raw, false); err != nil {
		return collectedLogEntry{}, err
	}
	odataType, _ := stringValue(raw["@odata.type"])
	if err := validateResourceSchemaType("log_entry", odataType); err != nil {
		return collectedLogEntry{}, err
	}
	canonical, err := json.Marshal(raw)
	if err != nil {
		return collectedLogEntry{}, err
	}
	revision := sha256.Sum256(canonical)
	revisionDigest := hex.EncodeToString(revision[:])

	identityKind, identityValue, entryURI, err := c.logEntryIdentity(service.URI, raw)
	if err != nil {
		return collectedLogEntry{}, err
	}
	sourceKey := c.logEntrySourceKeyFromIdentity(service.URI, identityKind, identityValue)
	generationSourceKey := stableKey(
		"netdata:redfish:log-source:v1",
		strings.Join([]string{
			c.origin,
			service.URI,
			strconv.FormatUint(generation, 10),
			sourceKey,
		}, "\x00"),
		64,
	)
	recordKey := stableKey("netdata:redfish:log-record:v1", generationSourceKey+"\x00"+revisionDigest, 64)
	sourceTime, sourceUsec, hasSourceTime := sourceLogTime(raw, observedAt)
	hostKey, hostName, nodeGUID := c.logHostAttribution(service)
	fields := map[string]string{
		"ND_LOG_SOURCE":              "redfish",
		"MESSAGE":                    logMessage(raw),
		"PRIORITY":                   logPriority(stringAt(raw, "Severity")),
		"SYSLOG_IDENTIFIER":          c.endpointJob,
		"REDFISH_OBSERVED_AT":        observedAt.UTC().Format(time.RFC3339Nano),
		"REDFISH_BACKEND":            c.logBackend,
		"REDFISH_ENDPOINT_JOB":       c.endpointJob,
		"REDFISH_ENDPOINT_KEY":       stableKey("netdata:redfish:endpoint:v1", c.origin, endpointKeyHexChars),
		"REDFISH_ENDPOINT_ORIGIN":    c.origin,
		"REDFISH_HOST_KEY":           hostKey,
		"REDFISH_HOST_NAME":          hostName,
		"REDFISH_RESOURCE_KIND":      "log_service",
		"REDFISH_RESOURCE_URI":       service.URI,
		"REDFISH_LOG_SERVICE_URI":    service.URI,
		"REDFISH_LOG_SERVICE_ID":     service.Doc.ID,
		"REDFISH_ENTRY_URI":          entryURI,
		"REDFISH_SOURCE_KEY":         generationSourceKey,
		"REDFISH_RECORD_KEY":         recordKey,
		"REDFISH_STREAM_GENERATION":  strconv.FormatUint(generation, 10),
		"REDFISH_JSON":               string(canonical),
		"_SOURCE_REALTIME_TIMESTAMP": strconv.FormatUint(sourceTime, 10),
	}
	if hostName != "" {
		fields["_HOSTNAME"] = hostName
	}
	if nodeGUID != "" {
		fields["ND_NIDL_NODE"] = nodeGUID
	}
	copyLogFields(fields, raw)
	removeEmptyFields(fields)
	return collectedLogEntry{
		Journal: redfishruntime.JournalEntry{
			RealtimeUsec:       uint64(observedAt.UnixMicro()),
			SourceRealtimeUsec: sourceTime,
			Fields:             fields,
		},
		RecordKey:     recordKey,
		SourceKey:     sourceKey,
		SourceUsec:    sourceUsec,
		HasSourceTime: hasSourceTime,
	}, nil
}

func (c *protocolClient) logEntrySourceKey(serviceURI string, raw map[string]any) (string, error) {
	identityKind, identityValue, _, err := c.logEntryIdentity(serviceURI, raw)
	if err != nil {
		return "", err
	}
	return c.logEntrySourceKeyFromIdentity(serviceURI, identityKind, identityValue), nil
}

func (c *protocolClient) logEntrySourceKeyFromIdentity(
	serviceURI, identityKind, identityValue string,
) string {
	return stableKey(
		"netdata:redfish:log-membership:v1",
		strings.Join([]string{c.origin, serviceURI, identityKind, identityValue}, "\x00"),
		64,
	)
}

func (c *protocolClient) logSourceCursorKey(serviceURI string) string {
	return stableKey(
		"netdata:redfish:log-cursor-source:v1",
		c.origin+"\x00"+serviceURI,
		64,
	)
}

func (c *protocolClient) logEntryIdentity(
	serviceURI string,
	raw map[string]any,
) (kind, value, entryURI string, err error) {
	if rawIdentity, present := raw["@odata.id"]; present {
		candidate, ok := stringValue(rawIdentity)
		if !ok {
			return "", "", "", errors.New("LogEntry has an invalid @odata.id")
		}
		target, resolveErr := c.resolveURI(c.root, candidate, false)
		if resolveErr != nil {
			return "", "", "", fmt.Errorf("LogEntry has an invalid @odata.id: %w", resolveErr)
		}
		entryURI = canonicalResourceURI(target)
		return "entry_uri", entryURI, entryURI, nil
	}
	if id, ok := stringValue(raw["Id"]); ok {
		return "entry_id", id, "", nil
	}
	eventID := stringAt(raw, "EventId")
	messageID := stringAt(raw, "MessageId")
	entryType := stringAt(raw, "EntryType")
	generatorID := stringAt(raw, "GeneratorId")
	timestamp := stringAt(raw, "EventTimestamp")
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		timestamp = stringAt(raw, "Created")
	}
	if eventID == "" || messageID == "" || entryType == "" || generatorID == "" {
		return "", "", "", errors.New("LogEntry has no usable identity")
	}
	if _, parseErr := time.Parse(time.RFC3339Nano, timestamp); parseErr != nil {
		return "", "", "", errors.New("LogEntry fallback identity has no valid timestamp")
	}
	return "event_tuple", structuralTuple(
		eventID, messageID, timestamp, entryType, generatorID,
	), "", nil
}

func sourceLogTime(raw map[string]any, observedAt time.Time) (uint64, int64, bool) {
	for _, field := range []string{"EventTimestamp", "Created"} {
		value := stringAt(raw, field)
		if value == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil && parsed.UnixMicro() >= 0 {
			return uint64(parsed.UnixMicro()), parsed.UnixMicro(), true
		}
	}
	return uint64(observedAt.UnixMicro()), observedAt.UnixMicro(), false
}

func (c *protocolClient) logHostAttribution(service *graphNode) (key, name, guid string) {
	if len(service.SystemOwners) == 1 {
		for _, system := range service.SystemOwners {
			scope := c.systemHostScope(system)
			return system.Key, firstNonEmpty(system.Doc.Name, system.URI), scope.GUID
		}
	}
	scope := c.serviceHostScope()
	return resourceKey(c.origin, "service", "/redfish/v1/"), "Redfish Service", scope.GUID
}

func logMessage(raw map[string]any) string {
	if message := stringAt(raw, "Message"); message != "" {
		return message
	}
	if id := stringAt(raw, "MessageId"); id != "" {
		if args, ok := raw["MessageArgs"]; ok {
			if encoded, err := json.Marshal(args); err == nil {
				return id + " " + string(encoded)
			}
		}
		return id
	}
	if code := stringAt(raw, "EntryCode"); code != "" {
		return code
	}
	return "Redfish LogEntry"
}

func logPriority(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return "2"
	case "warning":
		return "4"
	case "ok":
		return "6"
	default:
		return "5"
	}
}

var indexedLogFields = map[string]string{
	"Id":                      "REDFISH_ENTRY_ID",
	"EntryType":               "REDFISH_ENTRY_TYPE",
	"EntryCode":               "REDFISH_ENTRY_CODE",
	"Severity":                "REDFISH_SEVERITY",
	"MessageId":               "REDFISH_MESSAGE_ID",
	"Created":                 "REDFISH_CREATED",
	"Modified":                "REDFISH_MODIFIED",
	"EventTimestamp":          "REDFISH_EVENT_TIMESTAMP",
	"EventId":                 "REDFISH_EVENT_ID",
	"EventType":               "REDFISH_EVENT_TYPE",
	"EventGroupId":            "REDFISH_EVENT_GROUP_ID",
	"Resolved":                "REDFISH_RESOLVED",
	"Resolution":              "REDFISH_RESOLUTION",
	"SensorType":              "REDFISH_SENSOR_TYPE",
	"SensorNumber":            "REDFISH_SENSOR_NUMBER",
	"Originator":              "REDFISH_ORIGINATOR",
	"OriginatorType":          "REDFISH_ORIGINATOR_TYPE",
	"Username":                "REDFISH_USERNAME",
	"OriginAddress":           "REDFISH_ORIGIN_ADDRESS",
	"OverflowErrorCount":      "REDFISH_OVERFLOW_COUNT",
	"FirstOverflowTimestamp":  "REDFISH_FIRST_OVERFLOW_TIMESTAMP",
	"LastOverflowTimestamp":   "REDFISH_LAST_OVERFLOW_TIMESTAMP",
	"AdditionalDataURI":       "REDFISH_ADDITIONAL_DATA_URI",
	"AdditionalDataSizeBytes": "REDFISH_ADDITIONAL_DATA_SIZE",
	"DiagnosticDataType":      "REDFISH_DIAGNOSTIC_DATA_TYPE",
	"OEMDiagnosticDataType":   "REDFISH_DIAGNOSTIC_DATA_TYPE",
}

func copyLogFields(fields map[string]string, raw map[string]any) {
	for source, target := range indexedLogFields {
		value, exists := raw[source]
		if !exists || value == nil {
			continue
		}
		switch value := value.(type) {
		case string:
			fields[target] = value
		case bool:
			fields[target] = strconv.FormatBool(value)
		default:
			if encoded, err := json.Marshal(value); err == nil {
				fields[target] = string(encoded)
			}
		}
	}
	if args, exists := raw["MessageArgs"]; exists {
		if encoded, err := json.Marshal(args); err == nil {
			fields["REDFISH_MESSAGE_ARGS_JSON"] = string(encoded)
		}
	}
}

func removeEmptyFields(fields map[string]string) {
	for key, value := range fields {
		if value == "" {
			delete(fields, key)
		}
	}
}

func stringAt(data map[string]any, path string) string {
	value, _ := stringValueAt(data, path)
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func containsKey(values map[string]struct{}, key string) bool {
	_, ok := values[key]
	return ok
}

func mapKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	if len(result) > maxCursorExactKeys {
		result = result[len(result)-maxCursorExactKeys:]
	}
	return result
}

func (c *protocolClient) updateLogCounters(key string, update func(*logServiceCounters)) {
	c.logState.mu.Lock()
	state := c.logState.services[key]
	if state.Reconciliation == nil {
		state.Reconciliation = make(map[string]uint64)
	}
	update(&state)
	c.logState.services[key] = state
	c.logState.mu.Unlock()
}

func (c *protocolClient) incrementReconciliation(key, event string) {
	c.updateLogCounters(key, func(state *logServiceCounters) {
		state.Reconciliation[event]++
	})
}

func (c *protocolClient) clientContextUnsupported(key string) bool {
	c.logState.mu.Lock()
	defer c.logState.mu.Unlock()
	return c.logState.clientUnsupported[key]
}

func (c *protocolClient) disableClientContext(key string) {
	c.logState.mu.Lock()
	defer c.logState.mu.Unlock()
	c.logState.clientUnsupported[key] = true
}

func newClientContextKey() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("create Redfish clientcontext key: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func (c *protocolClient) logServiceMetricSnapshots(
	nodes map[string]*graphNode,
	forcedState string,
	observedAt time.Time,
) []hardwareObservation {
	c.logState.mu.Lock()
	defer c.logState.mu.Unlock()
	var result []hardwareObservation
	keys := make([]string, 0, len(nodes))
	for key := range nodes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		node := nodes[key]
		state := c.logState.services[key]
		ingestion := forcedState
		if ingestion == "" {
			if state.Blocked {
				ingestion = "blocked_source"
			} else if state.Backfilling {
				ingestion = "backfilling"
			} else {
				ingestion = "current"
			}
		}
		labels := c.metricLabels(node, nil)
		if node.Doc.ID != "" {
			labels = append(labels, metrix.Label{Key: "log_service_id", Value: node.Doc.ID})
		}
		scope := c.scopeForNode(node)
		result = append(result,
			stateObservation("log_service_ingestion_state", ingestion, []string{"current", "backfilling", "paused_backend", "blocked_source", "unknown"}, labels, scope),
			hardwareObservation{Metric: "log_service_pipeline_fetched", Value: float64(state.Fetched), Counter: true, Labels: labels, Scope: scope},
			hardwareObservation{Metric: "log_service_pipeline_committed", Value: float64(state.Committed), Counter: true, Labels: labels, Scope: scope},
			hardwareObservation{Metric: "log_service_pipeline_duplicate_suppressed", Value: float64(state.DuplicateSuppressed), Counter: true, Labels: labels, Scope: scope},
			hardwareObservation{Metric: "log_service_pipeline_failed", Value: float64(state.Failed), Counter: true, Labels: labels, Scope: scope},
			stateObservation("log_service_scan_mode", firstNonEmpty(state.LastMode, "unknown"), []string{"client_context", "incremental", "full", "unordered", "unknown"}, labels, scope),
		)
		for _, event := range []string{
			"completed",
			"client_context_restarted",
			"client_context_unsupported",
			"outside_tail_recovered_exact",
			"outside_tail_ambiguous",
			"ordering_disqualified",
			"boundary_lost",
			"etag_unreliable",
		} {
			result = append(result, hardwareObservation{
				Metric:  "log_service_reconciliation_" + event,
				Value:   float64(state.Reconciliation[event]),
				Counter: true,
				Labels:  labels,
				Scope:   scope,
			})
		}
		if state.LastPollDuration > 0 {
			result = append(result, hardwareObservation{
				Metric: "log_service_poll_duration_seconds",
				Value:  state.LastPollDuration.Seconds(),
				Labels: labels,
				Scope:  scope,
			})
		}
		if !state.LastComplete.IsZero() {
			result = append(result, hardwareObservation{
				Metric: "log_service_full_reconciliation_age_seconds",
				Value:  max(observedAt.Sub(state.LastComplete).Seconds(), 0),
				Labels: labels,
				Scope:  scope,
			})
		}
		if !state.NewestSource.IsZero() {
			result = append(result, hardwareObservation{
				Metric: "log_service_source_lag_seconds",
				Value:  max(observedAt.Sub(state.NewestSource).Seconds(), 0),
				Labels: labels,
				Scope:  scope,
			})
		}
	}
	return result
}

func addQuery(target *url.URL, values url.Values) *url.URL {
	copy := *target
	query := copy.Query()
	for key, list := range values {
		for _, value := range list {
			query.Add(key, value)
		}
	}
	copy.RawQuery = query.Encode()
	return &copy
}

func (c *protocolClient) logContinuationKey(raw string, opaque bool) (string, error) {
	target, err := c.resolveURI(c.root, raw, opaque)
	if err != nil {
		return "", err
	}
	return stableKey(
		"netdata:redfish:log-continuation:v1",
		target.RequestURI(),
		64,
	), nil
}

func stringSliceContains(values []string, target string) bool {
	return slices.Contains(values, target)
}

func advanceLogGeneration(cursor *logSourceCursor) error {
	if cursor.Generation >= 1<<62 {
		return errors.New("Redfish LogEntry stream generation overflow")
	}
	cursor.Generation++
	cursor.Mode = "recovery"
	cursor.Ordering = "unknown"
	cursor.Continuation = ""
	cursor.ClientContext = ""
	cursor.ContextDirty = false
	cursor.ReconcileStarted = false
	cursor.ReconcileExpected = 0
	cursor.ReconcileFetched = 0
	cursor.ReconcileSourceKeys = nil
	cursor.ReconcileRecordKeys = nil
	cursor.ContinuationKeys = nil
	cursor.ExactComplete = true
	cursor.BoundaryUsec = 0
	cursor.BoundaryKeys = nil
	cursor.ExactRecordKeys = nil
	cursor.LastCompleteUsec = 0
	cursor.CompleteCountKnown = false
	cursor.LastCompleteCount = 0
	return nil
}

func boolValueAt(data map[string]any, path string) (bool, bool) {
	value, ok := jsonPath(data, path)
	if !ok {
		return false, false
	}
	result, ok := value.(bool)
	return result, ok
}
