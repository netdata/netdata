// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

type stageID string
type stageState string

const (
	stageSetup   stageID = "setup"
	stagePut     stageID = "put"
	stageGet     stageID = "get"
	stageList    stageID = "list"
	stageDelete  stageID = "delete"
	stageCleanup stageID = "cleanup"

	stateOK      stageState = "ok"
	stateFailed  stageState = "failed"
	stateSkipped stageState = "skipped"

	reasonOK                   = "ok"
	reasonNotRun               = "not_run"
	reasonInternal             = "internal"
	reasonTimeout              = "timeout"
	reasonRequestFailed        = "request_failed"
	reasonPayloadMismatch      = "payload_mismatch"
	reasonNotVisible           = "not_visible"
	reasonStillPresent         = "still_present"
	reasonOrphanCleanupPending = "orphan_cleanup_pending"
)

var stageOrder = []stageID{stageSetup, stagePut, stageGet, stageList, stageDelete, stageCleanup}

type stageResult struct {
	state      stageState
	reason     string
	duration   time.Duration
	attempts   int
	retries    int
	operations int
}

type stageResults map[stageID]*stageResult

type stageCounters struct {
	operations int64
	attempts   int64
	retries    int64
	failures   int64
}

type stageStats map[stageID]*stageCounters

func newStageResults() stageResults {
	results := make(stageResults, len(stageOrder))
	for _, stage := range stageOrder {
		results[stage] = &stageResult{state: stateSkipped, reason: reasonNotRun}
	}
	return results
}

func newStageStats() stageStats {
	stats := make(stageStats, len(stageOrder))
	for _, stage := range stageOrder {
		stats[stage] = &stageCounters{}
	}
	return stats
}

func (r *stageResult) succeed() {
	r.state = stateOK
	r.reason = reasonOK
}

func (r *stageResult) fail(reason string) {
	r.state = stateFailed
	r.reason = reason
}

func (r *stageResult) addOperation(attempts int) {
	if attempts <= 0 {
		return
	}
	r.attempts += attempts
	r.operations++
	if attempts > 1 {
		r.retries += attempts - 1
	}
}

func (s *stageCounters) add(result *stageResult) {
	s.operations += int64(result.operations)
	s.attempts += int64(result.attempts)
	s.retries += int64(result.retries)
	if result.state == stateFailed {
		s.failures++
	}
}

func readRandomBytes(size int) ([]byte, error) {
	payload := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Collector) collect(ctx context.Context, results stageResults) {
	if !c.cleanupInterruptedObject(ctx, results) {
		return
	}
	if !c.prepareCollection(ctx, results) {
		return
	}

	payload, key, ok := c.setupProbe(ctx, results)
	if !ok {
		return
	}
	payloadHash := sha256.Sum256(payload)

	c.currentKey = key
	c.objectMayExist = true
	c.cleanupCompleted = false

	if !c.putProbe(ctx, key, payload, results) {
		c.deleteAndVerify(ctx, key, results)
		return
	}
	if !c.getProbe(ctx, key, payload, payloadHash, results) {
		c.deleteAndVerify(ctx, key, results)
		return
	}
	if !c.listProbe(ctx, key, results) {
		c.deleteAndVerify(ctx, key, results)
		return
	}
	c.deleteAndVerify(ctx, key, results)
}

func (c *Collector) cleanupInterruptedObject(ctx context.Context, results stageResults) bool {
	if c.currentKey == "" || !c.objectMayExist {
		return true
	}

	ok := c.cleanupStage(ctx, c.currentKey, results)
	c.finishPendingSetup(results)
	if ok {
		c.currentKey = ""
		c.objectMayExist = false
		c.cleanupCompleted = true
	}
	return false
}

func (c *Collector) prepareCollection(ctx context.Context, results stageResults) bool {
	// Reconcile the dedicated prefix before every new write. A timed-out PUT can
	// commit after an immediate cleanup check; the next cycle's LIST therefore has
	// to remain eligible instead of relying on a once-per-process orphan scan.
	start := c.now()
	keys, truncated, attempts, err := c.client.ListObjects(ctx, c.Bucket, c.Prefix, maxCleanupListKeys)
	elapsed := c.now().Sub(start)
	setup := results[stageSetup]
	setup.duration += elapsed
	setup.addOperation(attempts)
	if err != nil {
		setup.fail(errorReason(err))
		return false
	}

	probeKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if isProbeKey(c.Prefix, key) {
			probeKeys = append(probeKeys, key)
		}
	}
	if len(probeKeys) == 0 && !truncated {
		setup.succeed()
		return true
	}

	batch := probeKeys
	if len(batch) > cleanupBatchSize {
		batch = batch[:cleanupBatchSize]
	}
	for _, key := range batch {
		start = c.now()
		attempts, err = c.client.DeleteObject(ctx, c.Bucket, key)
		elapsed = c.now().Sub(start)
		cleanup := results[stageCleanup]
		cleanup.duration += elapsed
		cleanup.addOperation(attempts)
		if err != nil {
			cleanup.fail(errorReason(err))
			c.finishPendingSetup(results)
			return false
		}
	}
	results[stageCleanup].succeed()
	c.finishPendingSetup(results)
	return false
}

func (c *Collector) finishPendingSetup(results stageResults) {
	setup := results[stageSetup]
	setup.state = stateFailed
	setup.reason = reasonOrphanCleanupPending
}

func (c *Collector) setupProbe(_ context.Context, results stageResults) ([]byte, string, bool) {
	start := c.now()
	payload, payloadErr := c.randomRead(probePayloadBytes)
	var key string
	var keyErr error
	if payloadErr == nil {
		key, keyErr = c.newProbeKey()
	}
	results[stageSetup].duration += c.now().Sub(start)

	if payloadErr != nil || keyErr != nil {
		results[stageSetup].fail(reasonInternal)
		return nil, "", false
	}
	results[stageSetup].succeed()
	return payload, key, true
}

func (c *Collector) newProbeKey() (string, error) {
	suffix, err := c.randomRead(8)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%sprobe-%d-%s.bin", c.Prefix, c.now().UnixNano(), hex.EncodeToString(suffix)), nil
}

func isProbeKey(prefix, key string) bool {
	return strings.HasPrefix(key, prefix) && probeKeyRE.MatchString(strings.TrimPrefix(key, prefix))
}

func (c *Collector) putProbe(ctx context.Context, key string, payload []byte, results stageResults) bool {
	start := c.now()
	attempts, err := c.client.PutObject(ctx, c.Bucket, key, payload)
	result := results[stagePut]
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}
	result.succeed()
	return true
}

func (c *Collector) getProbe(ctx context.Context, key string, payload []byte, payloadHash [sha256.Size]byte, results stageResults) bool {
	start := c.now()
	body, attempts, err := c.client.GetObject(ctx, c.Bucket, key, int64(len(payload)+1))
	result := results[stageGet]
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}

	bodyHash := sha256.Sum256(body)
	if !bytes.Equal(payloadHash[:], bodyHash[:]) {
		result.fail(reasonPayloadMismatch)
		return false
	}
	result.succeed()
	return true
}

func (c *Collector) listProbe(ctx context.Context, key string, results stageResults) bool {
	start := c.now()
	keys, _, attempts, err := c.client.ListObjects(ctx, c.Bucket, c.Prefix, 100)
	result := results[stageList]
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}
	for _, listedKey := range keys {
		if listedKey == key {
			result.succeed()
			return true
		}
	}
	result.fail(reasonNotVisible)
	return false
}

func (c *Collector) deleteAndVerify(ctx context.Context, key string, results stageResults) {
	start := c.now()
	attempts, err := c.client.DeleteObject(ctx, c.Bucket, key)
	result := results[stageDelete]
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
	} else {
		result.succeed()
	}

	if !c.cleanupStage(ctx, key, results) {
		return
	}
	c.currentKey = ""
	c.objectMayExist = false
	c.cleanupCompleted = true
}

func (c *Collector) cleanupStage(ctx context.Context, key string, results stageResults) bool {
	result := results[stageCleanup]

	start := c.now()
	exists, attempts, err := c.client.ObjectExists(ctx, c.Bucket, key)
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}
	if !exists {
		result.succeed()
		return true
	}

	start = c.now()
	attempts, err = c.client.DeleteObject(ctx, c.Bucket, key)
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}

	start = c.now()
	exists, attempts, err = c.client.ObjectExists(ctx, c.Bucket, key)
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}
	if exists {
		result.fail(reasonStillPresent)
		return false
	}
	result.succeed()
	return true
}

func (c *Collector) ensureObjectGone(ctx context.Context, key string) error {
	exists, _, err := c.client.ObjectExists(ctx, c.Bucket, key)
	if err != nil || !exists {
		return err
	}
	if _, err = c.client.DeleteObject(ctx, c.Bucket, key); err != nil {
		return err
	}
	exists, _, err = c.client.ObjectExists(ctx, c.Bucket, key)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("probe object still exists")
	}
	return nil
}

func errorReason(err error) string {
	if err == nil {
		return reasonOK
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return reasonTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return reasonTimeout
	}
	return reasonRequestFailed
}
