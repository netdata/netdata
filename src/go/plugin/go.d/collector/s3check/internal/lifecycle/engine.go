// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/fairqueue"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/probe"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/s3client"
)

const (
	defaultQueueCapacity = 8
	defaultCleanupBatch  = 2
	maxQueueCapacity     = 32
)

type Options struct {
	Client    s3client.Client
	Bucket    string
	Journal   *journal.Journal
	Generator probe.Generator

	RequestTimeout time.Duration
	UpdateEvery    time.Duration
	QueueCapacity  int
	CleanupBatch   int
	Now            func() time.Time
}

type entry struct {
	Key              string     `json:"key"`
	Digest           string     `json:"digest"`
	CreatedAt        time.Time  `json:"created_at"`
	PutConfirmed     bool       `json:"put_confirmed"`
	AbsentObservedAt *time.Time `json:"absent_observed_at,omitempty"`
}

type state struct {
	Entries       []entry               `json:"entries"`
	CleanupCursor int                   `json:"cleanup_cursor,omitempty"`
	LastTerminal  *contract.ProbeResult `json:"last_terminal,omitempty"`
}

type Engine struct {
	client    s3client.Client
	bucket    string
	journal   *journal.Journal
	generator probe.Generator
	namespace string

	requestTimeout time.Duration
	updateEvery    time.Duration
	queueCapacity  int
	cleanupBatch   int
	now            func() time.Time

	state      state
	diagnostic error
	locked     bool
	closed     bool
}

func New(opts Options) (*Engine, error) {
	if opts.Client == nil {
		return nil, errors.New("lifecycle S3 client is required")
	}
	if strings.TrimSpace(opts.Bucket) == "" {
		return nil, errors.New("lifecycle bucket is required")
	}
	if opts.Journal == nil {
		return nil, errors.New("lifecycle journal is required")
	}
	if opts.Generator.OwnerID != opts.Journal.OwnerID() {
		return nil, errors.New("lifecycle generator and journal owners differ")
	}
	if opts.RequestTimeout <= 0 {
		return nil, errors.New("lifecycle request timeout must be positive")
	}
	if opts.UpdateEvery <= 0 {
		return nil, errors.New("lifecycle update interval must be positive")
	}
	namespace, err := opts.Generator.Namespace()
	if err != nil {
		return nil, fmt.Errorf("lifecycle probe namespace: %w", err)
	}
	if opts.QueueCapacity == 0 {
		opts.QueueCapacity = defaultQueueCapacity
	}
	if opts.QueueCapacity < 1 || opts.QueueCapacity > maxQueueCapacity {
		return nil, fmt.Errorf("lifecycle queue capacity must be between 1 and %d", maxQueueCapacity)
	}
	if opts.CleanupBatch == 0 {
		opts.CleanupBatch = defaultCleanupBatch
	}
	if opts.CleanupBatch < 1 || opts.CleanupBatch > opts.QueueCapacity {
		return nil, errors.New("lifecycle cleanup batch must fit the queue capacity")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}

	engine := &Engine{
		client:         opts.Client,
		bucket:         opts.Bucket,
		journal:        opts.Journal,
		generator:      opts.Generator,
		namespace:      namespace,
		requestTimeout: opts.RequestTimeout,
		updateEvery:    opts.UpdateEvery,
		queueCapacity:  opts.QueueCapacity,
		cleanupBatch:   opts.CleanupBatch,
		now:            opts.Now,
	}
	found, err := engine.journal.Load(&engine.state)
	if err != nil {
		return nil, fmt.Errorf("load lifecycle ownership: %w", err)
	}
	if found {
		if err := engine.validateState(); err != nil {
			return nil, fmt.Errorf("validate lifecycle ownership: %w", err)
		}
	}
	return engine, nil
}

func (e *Engine) Check(ctx context.Context) error {
	return e.validateProvider(ctx, nil)
}

func (e *Engine) validateProvider(ctx context.Context, operations *[]contract.OperationResult) error {
	var versioning s3client.BucketVersioningResult
	_, err := e.call(ctx, operations, contract.OperationSetup, func(callCtx context.Context) error {
		var callErr error
		versioning, callErr = e.client.BucketVersioning(callCtx, e.bucket)
		return callErr
	})
	if err != nil {
		return fmt.Errorf("check lifecycle bucket versioning: %w", err)
	}
	if versioning.Status != s3client.VersioningDisabled {
		return fmt.Errorf("lifecycle bucket must be unversioned, got %q", versioning.Status)
	}
	return nil
}

func (e *Engine) Collect(ctx context.Context) (result contract.Result) {
	result = contract.Result{
		Mode:         contract.ModeLifecycle,
		LastTerminal: cloneProbeResult(e.state.LastTerminal),
	}
	e.diagnostic = nil
	defer func() {
		if result.Err == nil {
			result.Err = e.diagnostic
		}
	}()
	if e.closed {
		result.Probe = failedProbe(contract.ReasonInternal)
		return result
	}
	if err := e.takeover(); err != nil {
		result.Probe = failedProbe(contract.ReasonOwnership)
		result.Err = err
		return result
	}
	defer func() {
		result.Cleanup.Pending = len(e.state.Entries)
		result.Cleanup.Backpressure = len(e.state.Entries) >= e.queueCapacity
		result.LastTerminal = cloneProbeResult(e.state.LastTerminal)
	}()
	if err := e.validateProvider(ctx, &result.Operations); err != nil {
		result.Err = fmt.Errorf("validate lifecycle provider safety: %w", err)
		return result
	}

	cleanup, cleanupErr := e.cleanupBacklog(ctx, e.cleanupBatch, &result.Operations)
	result.Cleanup = cleanup
	if cleanupErr != nil {
		result.Err = fmt.Errorf("cleanup lifecycle ownership: %w", cleanupErr)
		return result
	}
	if len(e.state.Entries) >= e.queueCapacity {
		result.Cleanup.Pending = len(e.state.Entries)
		result.Cleanup.Backpressure = true
		result.LastTerminal = cloneProbeResult(e.state.LastTerminal)
		return result
	}

	object, err := e.generator.Next()
	if err != nil {
		result.Probe = e.finish(failedProbe(contract.ReasonInternal))
		result.LastTerminal = cloneProbeResult(e.state.LastTerminal)
		return result
	}
	owned := entry{
		Key:       object.Key,
		Digest:    object.Digest,
		CreatedAt: e.now().UTC(),
	}
	e.state.Entries = append(e.state.Entries, owned)
	if err := e.persist(); err != nil {
		e.state.Entries = e.state.Entries[:len(e.state.Entries)-1]
		result.Probe = e.finish(failedProbe(contract.ReasonOwnership))
		result.LastTerminal = cloneProbeResult(e.state.LastTerminal)
		return result
	}
	index := len(e.state.Entries) - 1

	if _, err := e.call(ctx, &result.Operations, contract.OperationPut, func(callCtx context.Context) error {
		_, callErr := e.client.Put(callCtx, e.bucket, object.Key, object.Payload, s3client.PutOptions{IfNoneMatch: true})
		return callErr
	}); err != nil {
		result.Probe = e.finish(failedProbe(contract.ReasonRequest))
		_ = e.persist()
		result.Cleanup.Pending = len(e.state.Entries)
		result.LastTerminal = cloneProbeResult(e.state.LastTerminal)
		return result
	}
	e.state.Entries[index].PutConfirmed = true
	if err := e.persist(); err != nil {
		result.Probe = e.finish(failedProbe(contract.ReasonOwnership))
		result.Cleanup.Pending = len(e.state.Entries)
		result.LastTerminal = cloneProbeResult(e.state.LastTerminal)
		return result
	}

	var got s3client.GetResult
	if _, err := e.call(ctx, &result.Operations, contract.OperationRead, func(callCtx context.Context) error {
		var callErr error
		got, callErr = e.client.Get(callCtx, e.bucket, object.Key, "", probe.PayloadBytes)
		return callErr
	}); err != nil {
		result.Probe = e.finish(failedProbe(contract.ReasonRequest))
		_ = e.persist()
		result.Cleanup.Pending = len(e.state.Entries)
		result.LastTerminal = cloneProbeResult(e.state.LastTerminal)
		return result
	}
	if probe.Digest(got.Payload) != object.Digest {
		probeResult := failedProbe(contract.ReasonPayloadMismatch)
		probeResult.PayloadCompared = true
		probeResult.PayloadMismatch = true
		result.Probe = e.finish(probeResult)
		_ = e.persist()
		result.Cleanup.Pending = len(e.state.Entries)
		result.LastTerminal = cloneProbeResult(e.state.LastTerminal)
		return result
	}

	var page s3client.CurrentPage
	if _, err := e.call(ctx, &result.Operations, contract.OperationList, func(callCtx context.Context) error {
		var callErr error
		page, callErr = e.client.ListCurrent(callCtx, e.bucket, object.Key, 2)
		return callErr
	}); err != nil || page.Truncated || !slices.Contains(page.Keys, object.Key) {
		probeResult := failedProbe(contract.ReasonRequest)
		probeResult.PayloadCompared = true
		result.Probe = e.finish(probeResult)
		_ = e.persist()
		result.Cleanup.Pending = len(e.state.Entries)
		result.LastTerminal = cloneProbeResult(e.state.LastTerminal)
		return result
	}

	if _, err := e.call(ctx, &result.Operations, contract.OperationDelete, func(callCtx context.Context) error {
		_, callErr := e.client.Delete(callCtx, e.bucket, object.Key, s3client.DeleteOptions{})
		return callErr
	}); err != nil {
		probeResult := failedProbe(contract.ReasonRequest)
		probeResult.PayloadCompared = true
		result.Probe = e.finish(probeResult)
		_ = e.persist()
		result.Cleanup.Pending = len(e.state.Entries)
		result.LastTerminal = cloneProbeResult(e.state.LastTerminal)
		return result
	}
	absent := false
	if _, err := e.call(ctx, &result.Operations, contract.OperationDeleteVisibility, func(callCtx context.Context) error {
		_, callErr := e.client.Get(callCtx, e.bucket, object.Key, "", probe.PayloadBytes)
		if errors.Is(callErr, s3client.ErrObjectNotFound) {
			absent = true
			return nil
		}
		return callErr
	}); err != nil || !absent {
		probeResult := failedProbe(contract.ReasonCleanup)
		probeResult.PayloadCompared = true
		result.Probe = e.finish(probeResult)
		_ = e.persist()
		result.Cleanup.Pending = len(e.state.Entries)
		result.LastTerminal = cloneProbeResult(e.state.LastTerminal)
		return result
	}

	e.state.Entries = append(e.state.Entries[:index], e.state.Entries[index+1:]...)
	success := &contract.ProbeResult{
		Status: contract.StatusSuccess, Reason: contract.ReasonNone, PayloadCompared: true,
	}
	result.Probe = e.finish(success)
	if err := e.persist(); err != nil {
		probeResult := failedProbe(contract.ReasonOwnership)
		probeResult.PayloadCompared = true
		result.Probe = e.finish(probeResult)
	}
	result.Cleanup.Pending = len(e.state.Entries)
	result.LastTerminal = cloneProbeResult(e.state.LastTerminal)
	return result
}

func (e *Engine) Cleanup(ctx context.Context) {
	if e.closed {
		return
	}
	e.closed = true
	if e.locked {
		_, _ = e.cleanupBacklog(ctx, e.cleanupBatch, nil)
		_ = e.persist()
		e.journal.Unlock()
		e.locked = false
	}
	e.client.CloseIdleConnections()
}

func (e *Engine) cleanupBacklog(
	ctx context.Context,
	limit int,
	operations *[]contract.OperationResult,
) (contract.CleanupResult, error) {
	result := contract.CleanupResult{}
	keys := make([]string, 0, len(e.state.Entries))
	for _, owned := range e.state.Entries {
		keys = append(keys, owned.Key)
	}
	selected, next := fairqueue.Select(keys, "", e.state.CleanupCursor, limit)
	e.state.CleanupCursor = next
	for _, key := range selected {
		index := e.entryIndex(key)
		if index < 0 {
			continue
		}
		owned := &e.state.Entries[index]
		result.Attempted++

		_, deleteErr := e.call(ctx, operations, contract.OperationCleanup, func(callCtx context.Context) error {
			_, err := e.client.Delete(callCtx, e.bucket, owned.Key, s3client.DeleteOptions{})
			return err
		})
		if deleteErr != nil {
			result.Failed++
			continue
		}

		absent := false
		_, getErr := e.call(ctx, operations, contract.OperationCleanup, func(callCtx context.Context) error {
			_, err := e.client.Get(callCtx, e.bucket, owned.Key, "", probe.PayloadBytes)
			if errors.Is(err, s3client.ErrObjectNotFound) {
				absent = true
				return nil
			}
			return err
		})
		if getErr != nil {
			result.Failed++
			continue
		}
		if !absent {
			owned.AbsentObservedAt = nil
			if err := e.persist(); err != nil {
				return result, err
			}
			continue
		}
		if !owned.PutConfirmed {
			now := e.now().UTC()
			if owned.AbsentObservedAt == nil {
				owned.AbsentObservedAt = &now
				if err := e.persist(); err != nil {
					return result, err
				}
				continue
			}
			if now.Sub(*owned.AbsentObservedAt) < e.updateEvery {
				continue
			}
		}

		e.state.Entries = append(e.state.Entries[:index], e.state.Entries[index+1:]...)
		result.Removed++
		if err := e.persist(); err != nil {
			return result, err
		}
	}
	if len(e.state.Entries) == 0 {
		e.state.CleanupCursor = 0
	} else {
		e.state.CleanupCursor %= len(e.state.Entries)
		if len(selected) > 0 {
			if err := e.persist(); err != nil {
				return result, err
			}
		}
	}
	result.Pending = len(e.state.Entries)
	result.Backpressure = result.Pending >= e.queueCapacity
	return result, nil
}

func (e *Engine) call(
	parent context.Context,
	operations *[]contract.OperationResult,
	name contract.Operation,
	fn func(context.Context) error,
) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(parent, e.requestTimeout)
	started := time.Now()
	err := fn(ctx)
	duration := time.Since(started)
	cancel()
	if operations != nil {
		status := contract.StatusSuccess
		reason := contract.ReasonNone
		if err != nil {
			status = contract.StatusFailed
			reason = contract.ReasonRequest
		}
		*operations = append(*operations, contract.OperationResult{
			Name:     name,
			Endpoint: contract.EndpointSource,
			Status:   status,
			Reason:   reason,
			Duration: duration,
			Calls:    1,
			Err:      err,
		})
	}
	return duration, err
}

func (e *Engine) takeover() error {
	if e.locked {
		return nil
	}
	var authoritative state
	locked, found, err := e.journal.TryTakeover(&authoritative)
	if err != nil {
		return fmt.Errorf("take over lifecycle ownership: %w", err)
	}
	if !locked {
		return errors.New("lifecycle ownership is held by another runtime")
	}
	if found {
		e.state = authoritative
	} else {
		e.state = state{}
	}
	if err := e.validateState(); err != nil {
		e.journal.Unlock()
		return fmt.Errorf("validate authoritative lifecycle ownership: %w", err)
	}
	e.locked = true
	return nil
}

func (e *Engine) entryIndex(key string) int {
	for index := range e.state.Entries {
		if e.state.Entries[index].Key == key {
			return index
		}
	}
	return -1
}

func (e *Engine) finish(result *contract.ProbeResult) *contract.ProbeResult {
	e.state.LastTerminal = cloneProbeResult(result)
	return cloneProbeResult(result)
}

func (e *Engine) persist() error {
	var err error
	if len(e.state.Entries) == 0 {
		err = e.journal.Clear()
	} else {
		err = e.journal.Save(e.state)
	}
	if err == nil {
		return nil
	}
	err = fmt.Errorf("persist lifecycle ownership: %w", err)
	e.diagnostic = errors.Join(e.diagnostic, err)
	return err
}

func (e *Engine) validateState() error {
	if len(e.state.Entries) > e.queueCapacity {
		return fmt.Errorf("journal has %d entries, capacity is %d", len(e.state.Entries), e.queueCapacity)
	}
	if e.state.CleanupCursor < 0 {
		return errors.New("journal cleanup cursor is negative")
	}
	seen := make(map[string]struct{}, len(e.state.Entries))
	for _, owned := range e.state.Entries {
		switch {
		case !strings.HasPrefix(owned.Key, e.namespace):
			return errors.New("journal entry is outside the owner namespace")
		case owned.CreatedAt.IsZero():
			return errors.New("journal entry has no creation time")
		case !validDigest(owned.Digest):
			return errors.New("journal entry has invalid payload digest")
		}
		if _, ok := seen[owned.Key]; ok {
			return errors.New("journal contains a duplicate key")
		}
		seen[owned.Key] = struct{}{}
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func failedProbe(reason contract.Reason) *contract.ProbeResult {
	return &contract.ProbeResult{Status: contract.StatusFailed, Reason: reason}
}

func cloneProbeResult(value *contract.ProbeResult) *contract.ProbeResult {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
