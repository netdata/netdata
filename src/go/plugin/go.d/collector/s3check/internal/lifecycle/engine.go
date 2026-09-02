// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/probe"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/s3client"
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

var errVersioningSafety = errors.New("lifecycle bucket versioning safety is not established during mutation")

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
		opts.QueueCapacity = contract.DefaultQueueCapacity
	}
	if opts.QueueCapacity < 1 || opts.QueueCapacity > contract.MaxQueueCapacity {
		return nil, fmt.Errorf("lifecycle queue capacity must be between 1 and %d", contract.MaxQueueCapacity)
	}
	if opts.CleanupBatch == 0 {
		opts.CleanupBatch = contract.DefaultCleanupBatch
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
		return result
	}

	object, err := e.generator.Next()
	if err != nil {
		result.Probe = e.finish(contract.FailedProbe(contract.ReasonInternal))
		return result
	}
	owned := entry{
		Key: object.Key,
	}
	e.state.Entries = append(e.state.Entries, owned)
	if err := e.persist(); err != nil {
		e.state.Entries = e.state.Entries[:len(e.state.Entries)-1]
		result.Probe = e.finish(contract.FailedProbe(contract.ReasonOwnership))
		return result
	}
	index := len(e.state.Entries) - 1

	var put s3client.PutResult
	if _, err := e.call(ctx, &result.Operations, contract.OperationPut, func(callCtx context.Context) error {
		var callErr error
		put, callErr = e.client.Put(callCtx, e.bucket, object.Key, object.Payload, s3client.PutOptions{
			IfNoneMatch: true,
		})
		return callErr
	}); err != nil {
		result.Probe = e.finish(contract.FailedProbe(contract.ReasonRequest))
		_ = e.persist()
		return result
	}
	e.state.Entries[index].PutConfirmed = true
	if err := e.persist(); err != nil {
		result.Probe = e.finish(contract.FailedProbe(contract.ReasonOwnership))
		return result
	}
	if put.VersionID != "" {
		result.Probe = e.finish(contract.FailedProbe(contract.ReasonRequest))
		result.Err = fmt.Errorf("%w: PUT returned a version ID", errVersioningSafety)
		if err := e.persist(); err != nil {
			result.Err = errors.Join(result.Err, err)
		}
		return result
	}

	var got s3client.GetResult
	if _, err := e.call(ctx, &result.Operations, contract.OperationRead, func(callCtx context.Context) error {
		var callErr error
		got, callErr = e.client.Get(callCtx, e.bucket, object.Key, "", probe.PayloadBytes)
		return callErr
	}); err != nil {
		result.Probe = e.finish(contract.FailedProbe(contract.ReasonRequest))
		_ = e.persist()
		return result
	}
	if probe.Digest(got.Payload) != object.Digest {
		probeResult := contract.FailedProbe(contract.ReasonPayloadMismatch)
		probeResult.PayloadCompared = true
		probeResult.PayloadMismatch = true
		result.Probe = e.finish(probeResult)
		_ = e.persist()
		return result
	}

	var page s3client.CurrentPage
	if _, err := e.call(ctx, &result.Operations, contract.OperationList, func(callCtx context.Context) error {
		var callErr error
		page, callErr = e.client.ListCurrent(callCtx, e.bucket, object.Key, 2)
		return callErr
	}); err != nil || page.Truncated || !slices.Contains(page.Keys, object.Key) {
		probeResult := contract.FailedProbe(contract.ReasonRequest)
		probeResult.PayloadCompared = true
		result.Probe = e.finish(probeResult)
		_ = e.persist()
		return result
	}

	if err := e.deleteUnversioned(ctx, &result.Operations, contract.OperationDelete, object.Key); err != nil {
		probeResult := contract.FailedProbe(contract.ReasonRequest)
		probeResult.PayloadCompared = true
		result.Probe = e.finish(probeResult)
		if errors.Is(err, errVersioningSafety) {
			result.Err = err
		}
		_ = e.persist()
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
	}); err != nil ||
		!absent {
		probeResult := contract.FailedProbe(contract.ReasonCleanup)
		probeResult.PayloadCompared = true
		result.Probe = e.finish(probeResult)
		_ = e.persist()
		return result
	}

	e.state.Entries = append(e.state.Entries[:index], e.state.Entries[index+1:]...)
	success := &contract.ProbeResult{
		Status:          contract.StatusSuccess,
		Reason:          contract.ReasonNone,
		PayloadCompared: true,
	}
	result.Probe = e.finish(success)
	if err := e.persist(); err != nil {
		probeResult := contract.FailedProbe(contract.ReasonOwnership)
		probeResult.PayloadCompared = true
		result.Probe = e.finish(probeResult)
	}
	return result
}

func (e *Engine) deleteUnversioned(
	ctx context.Context,
	operations *[]contract.OperationResult,
	operation contract.Operation,
	key string,
) error {
	if err := e.validateProvider(ctx, operations); err != nil {
		return fmt.Errorf("%w before DELETE: %v", errVersioningSafety, err)
	}

	var deleted s3client.DeleteResult
	if _, err := e.call(ctx, operations, operation, func(callCtx context.Context) error {
		var callErr error
		deleted, callErr = e.client.Delete(callCtx, e.bucket, key, s3client.DeleteOptions{})
		return callErr
	}); err != nil {
		return err
	}
	if deleted.VersionID != "" || deleted.DeleteMarker {
		return fmt.Errorf("%w: DELETE returned version metadata", errVersioningSafety)
	}
	return nil
}

func (e *Engine) Cleanup(ctx context.Context) {
	if e.closed {
		return
	}
	e.closed = true
	if e.locked {
		if e.journal.MutationError() == nil {
			if e.validateProvider(ctx, nil) == nil {
				_, _ = e.cleanupBacklog(ctx, e.cleanupBatch, nil)
			}
			_ = e.persist()
		}
		e.journal.Unlock()
		e.locked = false
	}
	e.client.CloseIdleConnections()
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
			Err:      err,
		})
	}
	return duration, err
}
