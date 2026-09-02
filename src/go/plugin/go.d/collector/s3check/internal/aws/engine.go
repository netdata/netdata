// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/probe"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/s3client"
)

const (
	maxListPages      = 4
	maxListedVersions = 32
	listPageSize      = 16
)

var (
	errOwnershipInvariant = errors.New("AWS ownership invariant failed")
	errJournalPersistence = errors.New("persist AWS ownership")
)

func ownershipError(message string) error {
	return fmt.Errorf("%w: %s", errOwnershipInvariant, message)
}

func reasonForError(err error) contract.Reason {
	if errors.Is(err, errOwnershipInvariant) {
		return contract.ReasonOwnership
	}
	return contract.ReasonRequest
}

type phase string

const (
	phasePutIntent       phase = "put_intent"
	phaseReconcilePut    phase = "reconcile_put"
	phaseWaitObject      phase = "wait_object"
	phaseDeleteIntent    phase = "delete_intent"
	phaseReconcileDelete phase = "reconcile_delete"
	phaseWaitMarker      phase = "wait_marker"
	phaseExactCleanup    phase = "exact_cleanup"
	phaseBlocked         phase = "blocked"
)

type Options struct {
	Source      s3client.Client
	Destination s3client.Client

	SourceBucket      string
	DestinationBucket string
	ProbePrefix       string
	Journal           *journal.Journal
	Generator         probe.Generator

	SourceRequestTimeout      time.Duration
	DestinationRequestTimeout time.Duration
	UpdateEvery               time.Duration
	WriteObjective            time.Duration
	WriteTimeout              time.Duration
	DeleteObjective           time.Duration
	DeleteTimeout             time.Duration
	QueueCapacity             int
	CleanupBatch              int
	Now                       func() time.Time
}

type entry struct {
	Key       string    `json:"key"`
	Digest    string    `json:"digest"`
	CreatedAt time.Time `json:"created_at"`
	Phase     phase     `json:"phase"`

	SourceObjectID      string `json:"source_object_id,omitempty"`
	SourceObjectETag    string `json:"source_object_etag,omitempty"`
	DestinationObjectID string `json:"destination_object_id,omitempty"`
	SourceMarkerID      string `json:"source_marker_id,omitempty"`
	DestinationMarkerID string `json:"destination_marker_id,omitempty"`

	PutAt           *time.Time `json:"put_at,omitempty"`
	VisibleAt       *time.Time `json:"visible_at,omitempty"`
	DeleteAt        *time.Time `json:"delete_at,omitempty"`
	MarkerAt        *time.Time `json:"marker_at,omitempty"`
	DeleteAttemptAt *time.Time `json:"delete_attempt_at,omitempty"`
	PutAbsentSince  *time.Time `json:"put_absent_since,omitempty"`

	MeasureWrite  bool `json:"measure_write,omitempty"`
	MeasureDelete bool `json:"measure_delete,omitempty"`
	ObjectSeen    bool `json:"object_seen,omitempty"`
	MarkerSeen    bool `json:"marker_seen,omitempty"`
}

type state struct {
	Entries       []entry               `json:"entries"`
	ActiveKey     string                `json:"active_key,omitempty"`
	CleanupCursor int                   `json:"cleanup_cursor,omitempty"`
	LastTerminal  *contract.ProbeResult `json:"last_terminal,omitempty"`
}

type Engine struct {
	source      s3client.Client
	destination s3client.Client

	sourceBucket      string
	destinationBucket string
	journal           *journal.Journal
	generator         probe.Generator
	keyPrefix         string

	sourceRequestTimeout      time.Duration
	destinationRequestTimeout time.Duration
	updateEvery               time.Duration
	writeObjective            time.Duration
	writeTimeout              time.Duration
	deleteObjective           time.Duration
	deleteTimeout             time.Duration
	queueCapacity             int
	cleanupBatch              int
	now                       func() time.Time

	state      state
	durable    state
	diagnostic error
	locked     bool
	closed     bool
}

func New(opts Options) (*Engine, error) {
	switch {
	case opts.Source == nil:
		return nil, errors.New("AWS source S3 client is required")
	case opts.Destination == nil:
		return nil, errors.New("AWS destination S3 client is required")
	case strings.TrimSpace(opts.SourceBucket) == "":
		return nil, errors.New("AWS source bucket is required")
	case strings.TrimSpace(opts.DestinationBucket) == "":
		return nil, errors.New("AWS destination bucket is required")
	case strings.TrimSpace(opts.ProbePrefix) == "":
		return nil, errors.New("AWS probe prefix is required")
	case opts.Generator.Prefix != opts.ProbePrefix:
		return nil, errors.New("AWS generator and configured probe prefixes differ")
	case opts.Journal == nil:
		return nil, errors.New("AWS journal is required")
	case opts.Generator.OwnerID != opts.Journal.OwnerID():
		return nil, errors.New("AWS generator and journal owners differ")
	case opts.SourceRequestTimeout <= 0:
		return nil, errors.New("AWS source request timeout must be positive")
	case opts.DestinationRequestTimeout <= 0:
		return nil, errors.New("AWS destination request timeout must be positive")
	case opts.UpdateEvery <= 0:
		return nil, errors.New("AWS update interval must be positive")
	case opts.WriteObjective <= 0 || opts.WriteTimeout <= 0 || opts.WriteObjective > opts.WriteTimeout:
		return nil, errors.New("AWS write objective must be positive and not exceed its timeout")
	case opts.DeleteObjective <= 0 || opts.DeleteTimeout <= 0 || opts.DeleteObjective > opts.DeleteTimeout:
		return nil, errors.New("AWS delete objective must be positive and not exceed its timeout")
	}
	keyPrefix, err := opts.Generator.KeyPrefix()
	if err != nil {
		return nil, fmt.Errorf("AWS probe key prefix: %w", err)
	}
	if opts.QueueCapacity == 0 {
		opts.QueueCapacity = contract.DefaultQueueCapacity
	}
	if opts.QueueCapacity < 1 || opts.QueueCapacity > contract.MaxQueueCapacity {
		return nil, fmt.Errorf("AWS queue capacity must be between 1 and %d", contract.MaxQueueCapacity)
	}
	if opts.CleanupBatch == 0 {
		opts.CleanupBatch = min(contract.DefaultCleanupBatch, opts.QueueCapacity)
	}
	if opts.CleanupBatch < 1 || opts.CleanupBatch > opts.QueueCapacity {
		return nil, errors.New("AWS cleanup batch must fit the queue capacity")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}

	engine := &Engine{
		source:                    opts.Source,
		destination:               opts.Destination,
		sourceBucket:              opts.SourceBucket,
		destinationBucket:         opts.DestinationBucket,
		journal:                   opts.Journal,
		generator:                 opts.Generator,
		keyPrefix:                 keyPrefix,
		sourceRequestTimeout:      opts.SourceRequestTimeout,
		destinationRequestTimeout: opts.DestinationRequestTimeout,
		updateEvery:               opts.UpdateEvery,
		writeObjective:            opts.WriteObjective,
		writeTimeout:              opts.WriteTimeout,
		deleteObjective:           opts.DeleteObjective,
		deleteTimeout:             opts.DeleteTimeout,
		queueCapacity:             opts.QueueCapacity,
		cleanupBatch:              opts.CleanupBatch,
		now:                       opts.Now,
	}
	found, err := engine.journal.Load(&engine.state)
	if err != nil {
		return nil, fmt.Errorf("load AWS ownership: %w", err)
	}
	if found {
		if err := engine.validateState(); err != nil {
			return nil, fmt.Errorf("validate AWS ownership: %w", err)
		}
	}
	engine.durable = cloneState(engine.state)
	return engine, nil
}

func (e *Engine) Check(ctx context.Context) error {
	_, err := e.validateProvider(ctx, nil, e.keyPrefix)
	return err
}

func (e *Engine) validateProvider(
	ctx context.Context,
	operations *[]contract.OperationResult,
	keys ...string,
) ([]s3client.ReplicationRule, error) {
	if err := e.checkVersioning(ctx, operations, e.source, e.sourceBucket, contract.EndpointSource); err != nil {
		return nil, err
	}
	if err := e.checkVersioning(ctx, operations, e.destination, e.destinationBucket, contract.EndpointDestination); err != nil {
		return nil, err
	}
	var rules []s3client.ReplicationRule
	_, err := e.call(
		ctx,
		operations,
		contract.EndpointSource,
		contract.OperationSetup,
		func(callCtx context.Context) error {
			var callErr error
			rules, callErr = e.source.BucketReplication(callCtx, e.sourceBucket)
			return callErr
		},
	)
	if err != nil {
		return nil, fmt.Errorf("check AWS source replication policy: %w", err)
	}
	for _, key := range keys {
		if err := e.validateReplicationRules(rules, key); err != nil {
			return nil, err
		}
	}
	return rules, nil
}

func (e *Engine) validateReplicationRules(rules []s3client.ReplicationRule, key string) error {
	var effective *s3client.ReplicationRule
	for _, rule := range rules {
		if !rule.Enabled || rule.TagFiltered || !strings.HasPrefix(key, rule.Prefix) {
			continue
		}
		if rule.DestinationBucket != e.destinationBucket {
			return fmt.Errorf(
				"AWS source has an additional applicable replication destination %q outside collector ownership",
				rule.DestinationBucket,
			)
		}
		if effective == nil || rule.Priority > effective.Priority {
			candidate := rule
			effective = &candidate
		}
	}
	if effective == nil {
		return errors.New("AWS source has no enabled applicable replication rule for the configured destination")
	}
	if !effective.DeleteMarkerReplication {
		return errors.New(
			"AWS effective replication rule for the configured destination does not replicate delete markers",
		)
	}
	return nil
}

func (e *Engine) checkVersioning(
	ctx context.Context,
	operations *[]contract.OperationResult,
	client s3client.Client,
	bucket string,
	endpoint contract.Endpoint,
) error {
	var versioning s3client.BucketVersioningResult
	_, err := e.call(ctx, operations, endpoint, contract.OperationSetup, func(callCtx context.Context) error {
		var callErr error
		versioning, callErr = client.BucketVersioning(callCtx, bucket)
		return callErr
	})
	if err != nil {
		return fmt.Errorf("check AWS %s bucket versioning: %w", endpoint, err)
	}
	if versioning.Status != s3client.VersioningEnabled {
		return fmt.Errorf("AWS %s bucket versioning must be enabled, got %q", endpoint, versioning.Status)
	}
	if versioning.MFADelete {
		return fmt.Errorf("AWS %s bucket must not have MFA Delete enabled", endpoint)
	}
	return nil
}

func (e *Engine) Collect(ctx context.Context) (result contract.Result) {
	result = contract.Result{
		Mode:         contract.ModeAWSReplication,
		LastTerminal: contract.CloneProbe(e.state.LastTerminal),
	}
	e.diagnostic = nil
	defer func() {
		if result.Err == nil {
			result.Err = e.diagnostic
		}
	}()
	defer func() {
		// A state loaded before takeover may be stale while another runtime owns the journal.
		if !e.locked {
			return
		}
		result.Cleanup.Pending = len(e.state.Entries)
		result.Cleanup.Backpressure = len(e.state.Entries) >= e.queueCapacity
		result.LastTerminal = contract.CloneProbe(e.state.LastTerminal)
	}()
	if e.closed {
		result.Probe = contract.FailedProbe(contract.ReasonInternal)
		return result
	}
	if err := e.takeover(); err != nil {
		result.Probe = contract.FailedProbe(contract.ReasonOwnership)
		result.Err = err
		return result
	}
	keys := e.ownedKeys()
	if len(keys) == 0 {
		keys = append(keys, e.keyPrefix)
	}
	rules, err := e.validateProvider(ctx, &result.Operations, keys...)
	if err != nil {
		result.Err = fmt.Errorf("validate AWS provider safety: %w", err)
		return result
	}

	cleanup, cleanupErr := e.cleanupBacklog(ctx, e.cleanupBatch, &result.Operations)
	result.Cleanup = cleanup
	if cleanupErr != nil {
		result.Err = fmt.Errorf("cleanup AWS ownership: %w", cleanupErr)
		return result
	}
	if active := e.active(); active != nil {
		result.Probe = e.advanceActive(ctx, active, &result.Operations)
	} else if len(e.state.Entries) < e.queueCapacity {
		result.Probe, err = e.startProbe(ctx, &result.Operations, rules)
		if err != nil {
			result.Err = fmt.Errorf("validate AWS generated key policy: %w", err)
		}
	}
	return result
}

func (e *Engine) Cleanup(ctx context.Context) {
	if e.closed {
		return
	}
	e.closed = true
	if e.locked {
		if e.journal.MutationError() == nil {
			keys := e.ownedKeys()
			if len(keys) == 0 {
				_ = e.persist()
			} else if _, err := e.validateProvider(ctx, nil, keys...); err == nil {
				canCleanup := true
				if active := e.active(); active != nil {
					e.state.ActiveKey = ""
					canCleanup = e.persist() == nil
				}
				if canCleanup {
					_, _ = e.cleanupBacklog(ctx, e.cleanupBatch, nil)
				}
			}
		}
		e.journal.Unlock()
		e.locked = false
	}
	e.source.CloseIdleConnections()
	e.destination.CloseIdleConnections()
}

func (e *Engine) ownedKeys() []string {
	keys := make([]string, 0, len(e.state.Entries))
	for _, owned := range e.state.Entries {
		keys = append(keys, owned.Key)
	}
	return keys
}

func (e *Engine) takeover() error {
	if err := e.journal.MutationError(); err != nil {
		return fmt.Errorf("continue AWS ownership: %w", err)
	}
	if e.locked {
		return nil
	}
	var authoritative state
	locked, found, err := e.journal.TryTakeover(&authoritative)
	if err != nil {
		return fmt.Errorf("take over AWS ownership: %w", err)
	}
	if !locked {
		return errors.New("AWS ownership is held by another runtime")
	}
	if found {
		e.state = authoritative
	} else {
		e.state = state{}
	}
	if err := e.validateState(); err != nil {
		e.journal.Unlock()
		return fmt.Errorf("validate authoritative AWS ownership: %w", err)
	}
	e.durable = cloneState(e.state)
	e.locked = true
	return nil
}

func (e *Engine) persist() error {
	var err error
	if len(e.state.Entries) == 0 {
		err = e.journal.Clear()
	} else {
		err = e.journal.Save(e.state)
	}
	if err == nil {
		e.durable = cloneState(e.state)
		return nil
	}
	e.state = cloneState(e.durable)
	err = fmt.Errorf("%w: %w", errJournalPersistence, err)
	e.diagnostic = errors.Join(e.diagnostic, err)
	return err
}

func (e *Engine) active() *entry {
	if e.state.ActiveKey == "" {
		return nil
	}
	for index := range e.state.Entries {
		if e.state.Entries[index].Key == e.state.ActiveKey {
			return &e.state.Entries[index]
		}
	}
	return nil
}

func (e *Engine) remove(key string) {
	for index := range e.state.Entries {
		if e.state.Entries[index].Key == key {
			e.state.Entries = append(e.state.Entries[:index], e.state.Entries[index+1:]...)
			break
		}
	}
	if e.state.ActiveKey == key {
		e.state.ActiveKey = ""
	}
}

func (e *Engine) retire(owned *entry) {
	if owned != nil && e.state.ActiveKey == owned.Key {
		e.state.ActiveKey = ""
	}
}

func (e *Engine) persistTerminal(result *contract.ProbeResult) *contract.ProbeResult {
	e.state.LastTerminal = contract.CloneProbe(result)
	_ = e.persist()
	return contract.CloneProbe(result)
}

func cloneState(value state) state {
	cloned := value
	cloned.Entries = append([]entry(nil), value.Entries...)
	cloned.LastTerminal = contract.CloneProbe(value.LastTerminal)
	return cloned
}

func (e *Engine) validateState() error {
	if len(e.state.Entries) > e.queueCapacity {
		return fmt.Errorf("journal has %d entries, capacity is %d", len(e.state.Entries), e.queueCapacity)
	}
	if e.state.CleanupCursor < 0 {
		return errors.New("journal cleanup cursor is negative")
	}
	seen := make(map[string]struct{}, len(e.state.Entries))
	activeFound := e.state.ActiveKey == ""
	for _, owned := range e.state.Entries {
		switch {
		case !strings.HasPrefix(owned.Key, e.keyPrefix):
			return errors.New("journal entry is outside the owner namespace")
		case owned.CreatedAt.IsZero():
			return errors.New("journal entry has no creation time")
		case !probe.ValidDigest(owned.Digest):
			return errors.New("journal entry has invalid payload digest")
		case !validPhase(owned.Phase):
			return fmt.Errorf("journal entry has invalid phase %q", owned.Phase)
		}
		if err := validateEntryPhase(owned); err != nil {
			return fmt.Errorf("journal entry %q: %w", owned.Key, err)
		}
		if _, ok := seen[owned.Key]; ok {
			return errors.New("journal contains a duplicate key")
		}
		seen[owned.Key] = struct{}{}
		if owned.Key == e.state.ActiveKey {
			activeFound = true
		}
	}
	if !activeFound {
		return errors.New("active journal key is missing")
	}
	return nil
}

func validateEntryPhase(owned entry) error {
	if owned.MeasureWrite && owned.PutAt == nil {
		return errors.New("measured write has no confirmed PUT time")
	}
	if owned.MeasureDelete && owned.DeleteAt == nil {
		return errors.New("measured delete has no confirmed marker time")
	}
	requireObject := func() error {
		if owned.SourceObjectID == "" || owned.SourceObjectETag == "" {
			return errors.New("source object identity is incomplete")
		}
		return nil
	}
	requireReplicatedObject := func() error {
		if err := requireObject(); err != nil {
			return err
		}
		if owned.DestinationObjectID == "" || !owned.ObjectSeen || owned.VisibleAt == nil {
			return errors.New("destination object identity is incomplete")
		}
		return nil
	}

	switch owned.Phase {
	case phasePutIntent, phaseReconcilePut:
		if owned.SourceObjectID != "" || owned.SourceObjectETag != "" || owned.ObjectSeen || owned.MarkerSeen {
			return errors.New("PUT intent contains later-phase ownership")
		}
	case phaseWaitObject:
		return requireObject()
	case phaseDeleteIntent:
		if err := requireReplicatedObject(); err != nil {
			return err
		}
		if owned.SourceMarkerID != "" {
			return errors.New("delete intent already contains a source marker")
		}
	case phaseReconcileDelete:
		if err := requireReplicatedObject(); err != nil {
			return err
		}
		if owned.DeleteAttemptAt == nil {
			return errors.New("delete reconciliation has no attempt time")
		}
	case phaseWaitMarker:
		if err := requireReplicatedObject(); err != nil {
			return err
		}
		if owned.SourceMarkerID == "" || owned.DeleteAttemptAt == nil {
			return errors.New("source marker identity is incomplete")
		}
	case phaseExactCleanup:
		if err := requireReplicatedObject(); err != nil {
			return err
		}
		if owned.SourceMarkerID == "" || owned.DestinationMarkerID == "" ||
			!owned.MarkerSeen || owned.MarkerAt == nil {
			return errors.New("replicated marker identity is incomplete")
		}
	case phaseBlocked:
		// Blocked state is intentionally non-mutating and retains whatever proof
		// was available when an invariant failed.
	}
	return nil
}

func validPhase(value phase) bool {
	switch value {
	case phasePutIntent, phaseReconcilePut, phaseWaitObject, phaseDeleteIntent,
		phaseReconcileDelete, phaseWaitMarker, phaseExactCleanup, phaseBlocked:
		return true
	default:
		return false
	}
}
