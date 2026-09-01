// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"context"
	"encoding/hex"
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
	defaultQueueCapacity = 8
	defaultCleanupBatch  = 2
	maxQueueCapacity     = 32
	maxListPages         = 4
	maxListedVersions    = 32
	listPageSize         = 16
)

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

	RequestTimeout  time.Duration
	UpdateEvery     time.Duration
	WriteObjective  time.Duration
	WriteTimeout    time.Duration
	DeleteObjective time.Duration
	DeleteTimeout   time.Duration
	QueueCapacity   int
	CleanupBatch    int
	Now             func() time.Time
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

	PutAt     *time.Time `json:"put_at,omitempty"`
	VisibleAt *time.Time `json:"visible_at,omitempty"`
	DeleteAt  *time.Time `json:"delete_at,omitempty"`
	MarkerAt  *time.Time `json:"marker_at,omitempty"`

	MeasureWrite  bool `json:"measure_write,omitempty"`
	MeasureDelete bool `json:"measure_delete,omitempty"`
	ObjectSeen    bool `json:"object_seen,omitempty"`
	MarkerSeen    bool `json:"marker_seen,omitempty"`
}

type state struct {
	Entries      []entry               `json:"entries"`
	ActiveKey    string                `json:"active_key,omitempty"`
	LastTerminal *contract.ProbeResult `json:"last_terminal,omitempty"`
}

type Engine struct {
	source      s3client.Client
	destination s3client.Client

	sourceBucket      string
	destinationBucket string
	probePrefix       string
	journal           *journal.Journal
	generator         probe.Generator

	requestTimeout  time.Duration
	writeObjective  time.Duration
	writeTimeout    time.Duration
	deleteObjective time.Duration
	deleteTimeout   time.Duration
	queueCapacity   int
	cleanupBatch    int
	now             func() time.Time

	state  state
	locked bool
	closed bool
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
	case opts.RequestTimeout <= 0:
		return nil, errors.New("AWS request timeout must be positive")
	case opts.UpdateEvery <= 0:
		return nil, errors.New("AWS update interval must be positive")
	case opts.WriteObjective <= 0 || opts.WriteTimeout <= 0 || opts.WriteObjective > opts.WriteTimeout:
		return nil, errors.New("AWS write objective must be positive and not exceed its timeout")
	case opts.DeleteObjective <= 0 || opts.DeleteTimeout <= 0 || opts.DeleteObjective > opts.DeleteTimeout:
		return nil, errors.New("AWS delete objective must be positive and not exceed its timeout")
	}
	if opts.QueueCapacity == 0 {
		opts.QueueCapacity = defaultQueueCapacity
	}
	if opts.QueueCapacity < 1 || opts.QueueCapacity > maxQueueCapacity {
		return nil, fmt.Errorf("AWS queue capacity must be between 1 and %d", maxQueueCapacity)
	}
	if opts.CleanupBatch == 0 {
		opts.CleanupBatch = defaultCleanupBatch
		if opts.CleanupBatch > opts.QueueCapacity {
			opts.CleanupBatch = opts.QueueCapacity
		}
	}
	if opts.CleanupBatch < 1 || opts.CleanupBatch > opts.QueueCapacity {
		return nil, errors.New("AWS cleanup batch must fit the queue capacity")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}

	engine := &Engine{
		source: opts.Source, destination: opts.Destination,
		sourceBucket: opts.SourceBucket, destinationBucket: opts.DestinationBucket,
		probePrefix: opts.ProbePrefix, journal: opts.Journal, generator: opts.Generator,
		requestTimeout: opts.RequestTimeout,
		writeObjective: opts.WriteObjective, writeTimeout: opts.WriteTimeout,
		deleteObjective: opts.DeleteObjective, deleteTimeout: opts.DeleteTimeout,
		queueCapacity: opts.QueueCapacity, cleanupBatch: opts.CleanupBatch, now: opts.Now,
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
	return engine, nil
}

func (e *Engine) Check(ctx context.Context) error {
	if err := e.checkVersioning(ctx, e.source, e.sourceBucket, "source"); err != nil {
		return err
	}
	if err := e.checkVersioning(ctx, e.destination, e.destinationBucket, "destination"); err != nil {
		return err
	}
	rules, err := e.source.BucketReplication(ctx, e.sourceBucket)
	if err != nil {
		return fmt.Errorf("check AWS source replication policy: %w", err)
	}
	for _, rule := range rules {
		if rule.Enabled &&
			rule.DestinationBucket == e.destinationBucket &&
			strings.HasPrefix(e.probePrefix, rule.Prefix) &&
			!rule.TagFiltered &&
			rule.DeleteMarkerReplication {
			return nil
		}
	}
	return errors.New("AWS source has no enabled applicable replication rule with delete-marker replication")
}

func (e *Engine) checkVersioning(
	ctx context.Context,
	client s3client.Client,
	bucket, endpoint string,
) error {
	status, err := client.BucketVersioning(ctx, bucket)
	if err != nil {
		return fmt.Errorf("check AWS %s bucket versioning: %w", endpoint, err)
	}
	if status != s3client.VersioningEnabled {
		return fmt.Errorf("AWS %s bucket versioning must be enabled, got %q", endpoint, status)
	}
	return nil
}

func (e *Engine) Collect(ctx context.Context) contract.Result {
	result := contract.Result{
		Mode: contract.ModeAWSReplication, LastTerminal: cloneProbeResult(e.state.LastTerminal),
	}
	if e.closed {
		result.Probe = failedProbe(contract.ReasonInternal)
		return result
	}
	if !e.acquire() {
		result.Probe = failedProbe(contract.ReasonOwnership)
		return result
	}

	result.Cleanup = e.cleanupBacklog(ctx, e.cleanupBatch, &result.Operations)
	if active := e.active(); active != nil {
		result.Probe = e.advanceActive(ctx, active, &result.Operations)
	} else if len(e.state.Entries) < e.queueCapacity {
		result.Probe = e.startProbe(ctx, &result.Operations)
	}
	result.Cleanup.Pending = len(e.state.Entries)
	result.Cleanup.Backpressure = len(e.state.Entries) >= e.queueCapacity
	result.LastTerminal = cloneProbeResult(e.state.LastTerminal)
	return result
}

func (e *Engine) Cleanup(ctx context.Context) {
	if e.closed {
		return
	}
	e.closed = true
	if e.locked {
		if active := e.active(); active != nil {
			e.state.ActiveKey = ""
			_ = e.persist()
		}
		_ = e.cleanupBacklog(ctx, e.cleanupBatch, nil)
		_ = e.persist()
		e.journal.Unlock()
		e.locked = false
	}
	e.source.CloseIdleConnections()
	e.destination.CloseIdleConnections()
}

func (e *Engine) acquire() bool {
	if e.locked {
		return true
	}
	locked, err := e.journal.TryLock()
	if err != nil || !locked {
		return false
	}
	e.locked = true
	return true
}

func (e *Engine) persist() error {
	if len(e.state.Entries) == 0 {
		return e.journal.Clear()
	}
	return e.journal.Save(e.state)
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

func (e *Engine) finishTerminal(result *contract.ProbeResult) *contract.ProbeResult {
	e.state.LastTerminal = cloneProbeResult(result)
	_ = e.persist()
	return cloneProbeResult(result)
}

func (e *Engine) validateState() error {
	if len(e.state.Entries) > e.queueCapacity {
		return fmt.Errorf("journal has %d entries, capacity is %d", len(e.state.Entries), e.queueCapacity)
	}
	namespace := e.generator.Prefix + e.journal.OwnerID()[:16] + "/"
	seen := make(map[string]struct{}, len(e.state.Entries))
	activeFound := e.state.ActiveKey == ""
	for _, owned := range e.state.Entries {
		switch {
		case !strings.HasPrefix(owned.Key, namespace):
			return errors.New("journal entry is outside the owner namespace")
		case owned.CreatedAt.IsZero():
			return errors.New("journal entry has no creation time")
		case !validDigest(owned.Digest):
			return errors.New("journal entry has invalid payload digest")
		case !validPhase(owned.Phase):
			return fmt.Errorf("journal entry has invalid phase %q", owned.Phase)
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

func validPhase(value phase) bool {
	switch value {
	case phasePutIntent, phaseReconcilePut, phaseWaitObject, phaseDeleteIntent,
		phaseReconcileDelete, phaseWaitMarker, phaseExactCleanup, phaseBlocked:
		return true
	default:
		return false
	}
}

func validDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
