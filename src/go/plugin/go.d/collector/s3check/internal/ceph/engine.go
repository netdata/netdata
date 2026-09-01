// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/fairqueue"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/probe"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/s3client"
)

type phase string

const (
	phaseWriteVisibility  phase = "write_visibility"
	phaseDeleteVisibility phase = "delete_visibility"
	phaseCleanup          phase = "cleanup"
)

type Options struct {
	Source      s3client.Client
	Destination s3client.Client

	SourceBucket      string
	DestinationBucket string
	Journal           *journal.Journal
	Generator         probe.Generator

	SourceRequestTimeout      time.Duration
	DestinationRequestTimeout time.Duration
	WriteObjective            time.Duration
	WriteTimeout              time.Duration
	DeleteObjective           time.Duration
	DeleteTimeout             time.Duration
	QueueCapacity             int
	CleanupBatch              int
	Now                       func() time.Time
}

type entry struct {
	Key          string     `json:"key"`
	Digest       string     `json:"digest"`
	CreatedAt    time.Time  `json:"created_at"`
	Phase        phase      `json:"phase"`
	PutAt        *time.Time `json:"put_at,omitempty"`
	VisibleAt    *time.Time `json:"visible_at,omitempty"`
	DeleteAt     *time.Time `json:"delete_at,omitempty"`
	CleanupAfter time.Time  `json:"cleanup_after,omitempty"`
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
	namespace         string

	sourceRequestTimeout      time.Duration
	destinationRequestTimeout time.Duration
	writeObjective            time.Duration
	writeTimeout              time.Duration
	deleteObjective           time.Duration
	deleteTimeout             time.Duration
	queueCapacity             int
	cleanupBatch              int
	now                       func() time.Time

	state      state
	diagnostic error
	locked     bool
	closed     bool
}

func New(opts Options) (*Engine, error) {
	switch {
	case opts.Source == nil:
		return nil, errors.New("Ceph source S3 client is required")
	case opts.Destination == nil:
		return nil, errors.New("Ceph destination S3 client is required")
	case strings.TrimSpace(opts.SourceBucket) == "":
		return nil, errors.New("Ceph source bucket is required")
	case strings.TrimSpace(opts.DestinationBucket) == "":
		return nil, errors.New("Ceph destination bucket is required")
	case opts.Journal == nil:
		return nil, errors.New("Ceph journal is required")
	case opts.Generator.OwnerID != opts.Journal.OwnerID():
		return nil, errors.New("Ceph generator and journal owners differ")
	case opts.SourceRequestTimeout <= 0:
		return nil, errors.New("Ceph source request timeout must be positive")
	case opts.DestinationRequestTimeout <= 0:
		return nil, errors.New("Ceph destination request timeout must be positive")
	case opts.WriteObjective <= 0 || opts.WriteTimeout <= 0 || opts.WriteObjective > opts.WriteTimeout:
		return nil, errors.New("Ceph write objective must be positive and not exceed its timeout")
	case opts.DeleteObjective <= 0 || opts.DeleteTimeout <= 0 || opts.DeleteObjective > opts.DeleteTimeout:
		return nil, errors.New("Ceph delete objective must be positive and not exceed its timeout")
	}
	namespace, err := opts.Generator.Namespace()
	if err != nil {
		return nil, fmt.Errorf("Ceph probe namespace: %w", err)
	}
	if opts.QueueCapacity == 0 {
		opts.QueueCapacity = contract.DefaultQueueCapacity
	}
	if opts.QueueCapacity < 1 || opts.QueueCapacity > contract.MaxQueueCapacity {
		return nil, fmt.Errorf("Ceph queue capacity must be between 1 and %d", contract.MaxQueueCapacity)
	}
	if opts.CleanupBatch == 0 {
		opts.CleanupBatch = contract.DefaultCleanupBatch
	}
	if opts.CleanupBatch < 1 || opts.CleanupBatch > opts.QueueCapacity {
		return nil, errors.New("Ceph cleanup batch must fit the queue capacity")
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
		namespace:                 namespace,
		sourceRequestTimeout:      opts.SourceRequestTimeout,
		destinationRequestTimeout: opts.DestinationRequestTimeout,
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
		return nil, fmt.Errorf("load Ceph ownership: %w", err)
	}
	if found {
		if err := engine.validateState(); err != nil {
			return nil, fmt.Errorf("validate Ceph ownership: %w", err)
		}
	}
	return engine, nil
}

func (e *Engine) Check(ctx context.Context) error {
	return e.validateProvider(ctx, nil)
}

func (e *Engine) validateProvider(ctx context.Context, operations *[]contract.OperationResult) error {
	if err := e.checkUnversioned(ctx, operations, e.source, e.sourceBucket, contract.EndpointSource); err != nil {
		return err
	}
	return e.checkUnversioned(ctx, operations, e.destination, e.destinationBucket, contract.EndpointDestination)
}

func (e *Engine) checkUnversioned(
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
		return fmt.Errorf("check Ceph %s bucket versioning: %w", endpoint, err)
	}
	if versioning.Status != s3client.VersioningDisabled {
		return fmt.Errorf("Ceph %s bucket must be unversioned, got %q", endpoint, versioning.Status)
	}
	return nil
}

func (e *Engine) Collect(ctx context.Context) (result contract.Result) {
	result = contract.Result{
		Mode:         contract.ModeCephMultisite,
		LastTerminal: contract.CloneProbe(e.state.LastTerminal),
	}
	e.diagnostic = nil
	defer func() {
		if result.Err == nil {
			result.Err = e.diagnostic
		}
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
	defer func() {
		result.Cleanup.Pending = len(e.state.Entries)
		result.Cleanup.Backpressure = len(e.state.Entries) >= e.queueCapacity
		result.LastTerminal = contract.CloneProbe(e.state.LastTerminal)
	}()
	if err := e.validateProvider(ctx, &result.Operations); err != nil {
		result.Err = fmt.Errorf("validate Ceph provider safety: %w", err)
		return result
	}

	cleanup, err := e.cleanupBacklog(ctx, e.cleanupBatch, &result.Operations)
	result.Cleanup = cleanup
	if err != nil {
		result.Err = fmt.Errorf("cleanup Ceph ownership: %w", err)
		return result
	}

	if e.state.ActiveKey != "" {
		result.Probe = e.advanceActive(ctx, &result.Operations)
		result.Cleanup.Pending = len(e.state.Entries)
		result.Cleanup.Backpressure = len(e.state.Entries) >= e.queueCapacity
		result.LastTerminal = contract.CloneProbe(e.state.LastTerminal)
		return result
	}
	if len(e.state.Entries) >= e.queueCapacity {
		result.Cleanup.Pending = len(e.state.Entries)
		result.Cleanup.Backpressure = true
		return result
	}

	object, err := e.generator.Next()
	if err != nil {
		result.Probe = e.setTerminal(contract.FailedProbe(contract.ReasonInternal))
		result.LastTerminal = contract.CloneProbe(e.state.LastTerminal)
		return result
	}
	now := e.now().UTC()
	e.state.Entries = append(e.state.Entries, entry{
		Key:       object.Key,
		Digest:    object.Digest,
		CreatedAt: now,
		Phase:     phaseWriteVisibility,
	})
	e.state.ActiveKey = object.Key
	if err := e.persist(); err != nil {
		e.state.Entries = e.state.Entries[:len(e.state.Entries)-1]
		e.state.ActiveKey = ""
		result.Probe = e.setTerminal(contract.FailedProbe(contract.ReasonOwnership))
		result.LastTerminal = contract.CloneProbe(e.state.LastTerminal)
		return result
	}

	if _, err := e.call(
		ctx,
		&result.Operations,
		contract.EndpointSource,
		contract.OperationPut,
		func(callCtx context.Context) error {
			_, callErr := e.source.Put(callCtx, e.sourceBucket, object.Key, object.Payload, s3client.PutOptions{IfNoneMatch: true})
			return callErr
		},
	); err != nil {
		owned := e.active()
		result.Probe = e.retireWithResult(owned, contract.FailedProbe(contract.ReasonRequest))
		result.Cleanup.Pending = len(e.state.Entries)
		result.LastTerminal = contract.CloneProbe(e.state.LastTerminal)
		return result
	}
	owned := e.active()
	putAt := e.now().UTC()
	owned.PutAt = &putAt
	if err := e.persist(); err != nil {
		owned.PutAt = nil
		result.Probe = e.retireWithResult(owned, contract.FailedProbe(contract.ReasonOwnership))
		result.Cleanup.Pending = len(e.state.Entries)
		result.LastTerminal = contract.CloneProbe(e.state.LastTerminal)
		return result
	}

	result.Probe = e.advanceActive(ctx, &result.Operations)
	result.Cleanup.Pending = len(e.state.Entries)
	result.Cleanup.Backpressure = len(e.state.Entries) >= e.queueCapacity
	result.LastTerminal = contract.CloneProbe(e.state.LastTerminal)
	return result
}

func (e *Engine) advanceActive(
	ctx context.Context,
	operations *[]contract.OperationResult,
) *contract.ProbeResult {
	owned := e.active()
	if owned == nil {
		return contract.FailedProbe(contract.ReasonInternal)
	}
	if owned.PutAt == nil {
		return e.retireWithResult(owned, contract.FailedProbe(contract.ReasonOwnership))
	}

	if owned.Phase == phaseWriteVisibility {
		var got s3client.GetResult
		_, err := e.call(
			ctx,
			operations,
			contract.EndpointDestination,
			contract.OperationWriteVisibility,
			func(callCtx context.Context) error {
				var callErr error
				got, callErr = e.destination.Get(
					callCtx,
					e.destinationBucket,
					owned.Key,
					"",
					probe.PayloadBytes,
				)
				return callErr
			},
		)
		now := e.now().UTC()
		lag := now.Sub(*owned.PutAt)
		write := contract.ObjectiveResultFor(lag, e.writeObjective)
		switch {
		case errors.Is(err, s3client.ErrObjectNotFound):
			if lag < e.writeTimeout {
				return &contract.ProbeResult{
					Status:          contract.StatusWaiting,
					Reason:          contract.ReasonNone,
					WriteVisibility: write,
				}
			}
			result := contract.FailedProbe(contract.ReasonVisibilityTimeout)
			result.WriteVisibility = write
			return e.retireWithResult(owned, result)
		case err != nil:
			return e.retireWithResult(owned, contract.FailedProbe(contract.ReasonRequest))
		case probe.Digest(got.Payload) != owned.Digest:
			result := contract.FailedProbe(contract.ReasonPayloadMismatch)
			result.PayloadCompared = true
			result.PayloadMismatch = true
			result.WriteVisibility = write
			return e.retireWithResult(owned, result)
		}

		visibleAt := now
		owned.VisibleAt = &visibleAt
		if _, err := e.call(
			ctx,
			operations,
			contract.EndpointSource,
			contract.OperationDelete,
			func(callCtx context.Context) error {
				_, callErr := e.source.Delete(callCtx, e.sourceBucket, owned.Key, s3client.DeleteOptions{})
				return callErr
			},
		); err != nil {
			result := contract.FailedProbe(contract.ReasonRequest)
			result.PayloadCompared = true
			result.WriteVisibility = write
			return e.retireWithResult(owned, result)
		}
		deletedAt := e.now().UTC()
		owned.DeleteAt = &deletedAt
		owned.Phase = phaseDeleteVisibility
		if err := e.persist(); err != nil {
			result := contract.FailedProbe(contract.ReasonOwnership)
			result.PayloadCompared = true
			return e.retireWithResult(owned, result)
		}
	}

	if owned.Phase != phaseDeleteVisibility || owned.VisibleAt == nil || owned.DeleteAt == nil {
		result := contract.FailedProbe(contract.ReasonOwnership)
		result.PayloadCompared = owned.VisibleAt != nil
		return e.retireWithResult(owned, result)
	}

	_, err := e.call(
		ctx,
		operations,
		contract.EndpointDestination,
		contract.OperationDeleteVisibility,
		func(callCtx context.Context) error {
			_, callErr := e.destination.Get(
				callCtx,
				e.destinationBucket,
				owned.Key,
				"",
				probe.PayloadBytes,
			)
			return callErr
		},
	)
	now := e.now().UTC()
	write := contract.ObjectiveResultFor(owned.VisibleAt.Sub(*owned.PutAt), e.writeObjective)
	deleteLag := now.Sub(*owned.DeleteAt)
	deletion := contract.ObjectiveResultFor(deleteLag, e.deleteObjective)
	switch {
	case errors.Is(err, s3client.ErrObjectNotFound):
		result := &contract.ProbeResult{
			Status:           contract.StatusSuccess,
			Reason:           contract.ReasonNone,
			PayloadCompared:  true,
			WriteVisibility:  write,
			DeleteVisibility: deletion,
		}
		e.removeActive()
		result = e.setTerminal(result)
		_ = e.persist()
		return result
	case err != nil:
		result := contract.FailedProbe(contract.ReasonRequest)
		result.PayloadCompared = true
		result.WriteVisibility = write
		return e.retireWithResult(owned, result)
	case deleteLag >= e.deleteTimeout:
		result := contract.FailedProbe(contract.ReasonDeleteTimeout)
		result.PayloadCompared = true
		result.WriteVisibility = write
		result.DeleteVisibility = deletion
		return e.retireWithResult(owned, result)
	default:
		return &contract.ProbeResult{
			Status:           contract.StatusWaiting,
			Reason:           contract.ReasonNone,
			PayloadCompared:  true,
			WriteVisibility:  write,
			DeleteVisibility: deletion,
		}
	}
}

func (e *Engine) Cleanup(ctx context.Context) {
	if e.closed {
		return
	}
	e.closed = true
	if e.locked {
		canCleanup := true
		if active := e.active(); active != nil {
			canCleanup = e.retire(active) == nil
		}
		if canCleanup && e.validateProvider(ctx, nil) == nil {
			_, _ = e.cleanupBacklog(ctx, e.cleanupBatch, nil)
		}
		_ = e.persist()
		e.journal.Unlock()
		e.locked = false
	}
	e.source.CloseIdleConnections()
	e.destination.CloseIdleConnections()
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
	selected, next := fairqueue.Select(keys, e.state.ActiveKey, e.state.CleanupCursor, limit)
	e.state.CleanupCursor = next
	for _, key := range selected {
		index := e.entryIndex(key)
		if index < 0 {
			continue
		}
		owned := &e.state.Entries[index]
		if owned.Phase != phaseCleanup {
			continue
		}
		now := e.now().UTC()

		_, sourceDeleteErr := e.call(
			ctx,
			operations,
			contract.EndpointSource,
			contract.OperationCleanup,
			func(callCtx context.Context) error {
				_, err := e.source.Delete(callCtx, e.sourceBucket, owned.Key, s3client.DeleteOptions{})
				return err
			},
		)
		if sourceDeleteErr == nil && owned.DeleteAt == nil {
			owned.DeleteAt = &now
			if deadline := now.Add(e.deleteTimeout); deadline.After(owned.CleanupAfter) {
				owned.CleanupAfter = deadline
			}
			if err := e.persist(); err != nil {
				return result, err
			}
		}
		_, destinationDeleteErr := e.call(
			ctx,
			operations,
			contract.EndpointDestination,
			contract.OperationCleanup,
			func(callCtx context.Context) error {
				_, err := e.destination.Delete(callCtx, e.destinationBucket, owned.Key, s3client.DeleteOptions{})
				return err
			},
		)
		if sourceDeleteErr != nil || destinationDeleteErr != nil {
			continue
		}

		sourceAbsent, sourceErr := e.absent(
			ctx,
			operations,
			e.source,
			e.sourceBucket,
			contract.EndpointSource,
			owned.Key,
		)
		destinationAbsent, destinationErr := e.absent(
			ctx,
			operations,
			e.destination,
			e.destinationBucket,
			contract.EndpointDestination,
			owned.Key,
		)
		if sourceErr != nil || destinationErr != nil {
			continue
		}
		if !sourceAbsent || !destinationAbsent || now.Before(owned.CleanupAfter) {
			continue
		}

		e.state.Entries = append(e.state.Entries[:index], e.state.Entries[index+1:]...)
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

func (e *Engine) absent(
	ctx context.Context,
	operations *[]contract.OperationResult,
	client s3client.Client,
	bucket string,
	endpoint contract.Endpoint,
	key string,
) (bool, error) {
	absent := false
	_, err := e.call(ctx, operations, endpoint, contract.OperationCleanup, func(callCtx context.Context) error {
		_, callErr := client.Get(callCtx, bucket, key, "", probe.PayloadBytes)
		if errors.Is(callErr, s3client.ErrObjectNotFound) {
			absent = true
			return nil
		}
		return callErr
	})
	return absent, err
}

func (e *Engine) call(
	parent context.Context,
	operations *[]contract.OperationResult,
	endpoint contract.Endpoint,
	name contract.Operation,
	fn func(context.Context) error,
) (time.Duration, error) {
	requestTimeout := e.sourceRequestTimeout
	if endpoint == contract.EndpointDestination {
		requestTimeout = e.destinationRequestTimeout
	}
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	started := time.Now()
	err := fn(ctx)
	duration := time.Since(started)
	cancel()
	if operations != nil {
		status := contract.StatusSuccess
		reason := contract.ReasonNone
		if err != nil && !errors.Is(err, s3client.ErrObjectNotFound) {
			status = contract.StatusFailed
			reason = contract.ReasonRequest
		}
		*operations = append(*operations, contract.OperationResult{
			Name:     name,
			Endpoint: endpoint,
			Status:   status,
			Reason:   reason,
			Duration: duration,
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
		return fmt.Errorf("take over Ceph ownership: %w", err)
	}
	if !locked {
		return errors.New("Ceph ownership is held by another runtime")
	}
	if found {
		e.state = authoritative
	} else {
		e.state = state{}
	}
	if err := e.validateState(); err != nil {
		e.journal.Unlock()
		return fmt.Errorf("validate authoritative Ceph ownership: %w", err)
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

func (e *Engine) removeActive() {
	for index := range e.state.Entries {
		if e.state.Entries[index].Key == e.state.ActiveKey {
			e.state.Entries = append(e.state.Entries[:index], e.state.Entries[index+1:]...)
			break
		}
	}
	e.state.ActiveKey = ""
}

func (e *Engine) moveToCleanup(owned *entry) {
	if owned == nil {
		e.state.ActiveKey = ""
		return
	}
	owned.Phase = phaseCleanup
	base := owned.CreatedAt
	if owned.PutAt != nil {
		base = *owned.PutAt
	}
	owned.CleanupAfter = base.Add(e.writeTimeout)
	if owned.PutAt == nil {
		deadline := e.now().UTC().Add(e.sourceRequestTimeout).Add(e.writeTimeout)
		if deadline.After(owned.CleanupAfter) {
			owned.CleanupAfter = deadline
		}
	}
	if owned.DeleteAt != nil {
		if deadline := owned.DeleteAt.Add(e.deleteTimeout); deadline.After(owned.CleanupAfter) {
			owned.CleanupAfter = deadline
		}
	}
	e.state.ActiveKey = ""
}

func (e *Engine) retire(owned *entry) error {
	previous := cloneState(e.state)
	e.moveToCleanup(owned)
	if err := e.persist(); err != nil {
		e.state = previous
		return err
	}
	return nil
}

func (e *Engine) retireWithResult(owned *entry, result *contract.ProbeResult) *contract.ProbeResult {
	previous := cloneState(e.state)
	e.moveToCleanup(owned)
	e.state.LastTerminal = contract.CloneProbe(result)
	if err := e.persist(); err != nil {
		e.state = previous
	}
	return contract.CloneProbe(result)
}

func (e *Engine) setTerminal(result *contract.ProbeResult) *contract.ProbeResult {
	e.state.LastTerminal = contract.CloneProbe(result)
	return contract.CloneProbe(result)
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
	err = fmt.Errorf("persist Ceph ownership: %w", err)
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
	activeFound := e.state.ActiveKey == ""
	for _, owned := range e.state.Entries {
		switch {
		case !strings.HasPrefix(owned.Key, e.namespace):
			return errors.New("journal entry is outside the owner namespace")
		case owned.CreatedAt.IsZero():
			return errors.New("journal entry has no creation time")
		case !probe.ValidDigest(owned.Digest):
			return errors.New("journal entry has invalid payload digest")
		case owned.Phase != phaseWriteVisibility &&
			owned.Phase != phaseDeleteVisibility &&
			owned.Phase != phaseCleanup:
			return fmt.Errorf("journal entry has invalid phase %q", owned.Phase)
		}
		if _, ok := seen[owned.Key]; ok {
			return errors.New("journal contains a duplicate key")
		}
		seen[owned.Key] = struct{}{}
		if owned.Key == e.state.ActiveKey {
			if owned.Phase == phaseCleanup {
				return errors.New("active journal entry is in cleanup")
			}
			activeFound = true
		} else if owned.Phase != phaseCleanup {
			return errors.New("non-active journal entry is outside cleanup")
		}
	}
	if !activeFound {
		return errors.New("active journal key is missing")
	}
	return nil
}

func cloneState(value state) state {
	cloned := value
	cloned.Entries = append([]entry(nil), value.Entries...)
	cloned.LastTerminal = contract.CloneProbe(value.LastTerminal)
	return cloned
}
