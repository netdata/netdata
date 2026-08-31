// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/filelock"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_MultisiteWaitsForDelayedVisibilityAndMeasuresLag(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Len(t, source.objects, 1)
	assert.Empty(t, destination.objects)
	assert.Equal(t, 6, len(source.calls)+len(destination.calls))

	sourceToDestination(t, collr, source, destination, false)
	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "ok",
		multisitePhaseLabels(multisiteReplication, reasonOK),
	)])
	assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("multisite_payload_mismatch", multisiteSiteLabels())])
	require.Contains(t, metrics, metricKey("multisite_replication_lag_ms", multisiteSiteLabels()))
	require.Len(t, destination.objects, 1)
	assert.True(t, strings.HasPrefix(onlyMapKey(t, destination.objects), collr.stateStore.destinationProbePrefix))

	delete(destination.objects, onlyMapKey(t, destination.objects))
	metrics, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "ok", multisiteDeleteWaitLabels(reasonOK),
	)])
	assert.Contains(t, metrics, metricKey("multisite_delete_lag_ms", multisiteSiteLabels()))
	assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("multisite_delete_exceeded", multisiteSiteLabels())])
	assert.Empty(t, source.objects)
	assert.Empty(t, destination.objects)
	require.FileExists(t, collr.stateStore.path)

	metrics, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	metrics = finishMultisiteCleanup(t, collr)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "ok", multisitePhaseLabels(multisiteCleanup, reasonOK),
	)])
	assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
}

func TestCollector_MultisiteReportsPayloadMismatchAndCleansBothSites(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	sourceToDestination(t, collr, source, destination, true)

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "failed",
		multisitePhaseLabels(multisiteReplication, reasonPayloadMismatch),
	)])
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_payload_mismatch", multisiteSiteLabels())])
	require.Len(t, source.objects, 1)
	require.Len(t, destination.objects, 1)
	require.Equal(t, reasonPayloadMismatch, collr.pendingOwnershipState.TerminalReason)

	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, source.objects)
	assert.Empty(t, destination.objects)
	finishMultisiteCleanup(t, collr)

}

func TestCollector_MultisiteReportsRPOBreachAndNeverVisibleTimeout(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	collr.RPOThresholdMS = 1
	collr.ReplicationTimeoutMS = 20

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	past := collr.pendingOwnershipState.CreatedAt
	collr.pendingOwnershipState.SourcePutAt = &past
	require.NoError(t, collr.stateStore.save(collr.pendingOwnershipState))

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_rpo_exceeded", multisiteSiteLabels())])
	assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "failed", multisitePhaseLabels(multisiteReplication, reasonReplicationTimeout),
	)])
	require.Len(t, source.objects, 1)

	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, source.objects)
	assert.Empty(t, destination.objects)
	finishMultisiteCleanup(t, collr)
}

func TestCollector_MultisiteReportsDeletePropagationBreach(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	collr.DeleteThresholdMS = 1

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	sourceToDestination(t, collr, source, destination, false)

	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Len(t, destination.objects, 1)

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_delete_exceeded", multisiteSiteLabels())])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "waiting",
		multisitePhaseLabels(multisiteDeleteWait, reasonStillPresent),
	)])
}

func TestCollector_MultisiteResumesCrashSafePendingState(t *testing.T) {
	collr, source, _ := newMultisiteTestCollector(t)
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.FileExists(t, collr.stateStore.path)
	require.Len(t, source.objects, 1)

	collr.pendingOwnershipState.Phase = string(multisiteSourcePut)
	collr.pendingOwnershipState.SourcePutAt = nil
	collr.pendingOwnershipState.SourcePutAttempted = true
	require.NoError(t, collr.stateStore.save(collr.pendingOwnershipState))
	require.Equal(t, string(multisiteSourcePut), collr.pendingOwnershipState.Phase)

	collr.releaseOwnerLock()
	restarted, restartedSource, restartedDestination := newMultisiteTestCollectorAt(t, collr.stateStore.path)
	*restartedSource = *source
	restarted.stateStore = collr.stateStore
	loaded, err := restarted.stateStore.load()
	require.NoError(t, err)
	require.Equal(t, collr.pendingOwnershipState, loaded)
	restarted.pendingOwnershipState = loaded

	// Recovery proves that the pre-PUT state actually committed before advancing.
	metrics, err := collecttest.CollectScalarSeries(restarted, metrix.ReadFlatten())
	require.NoError(t, err)
	require.NotNil(t, restarted.pendingOwnershipState.SourcePutAt)
	require.Equal(t, string(multisiteReplication), restarted.pendingOwnershipState.Phase)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "ok", multisitePhaseLabels(multisiteSourcePut, reasonRecovered),
	)])

	restartedClock := &steppingClock{now: restarted.pendingOwnershipState.CreatedAt.Add(time.Second), step: 2 * time.Millisecond}
	restarted.now = restartedClock.tick
	restarted.stateStore.now = restarted.now
	sourceToDestination(t, restarted, restartedSource, restartedDestination, false)
	_, err = collecttest.CollectScalarSeries(restarted, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, restartedSource.objects)
	require.Len(t, restartedDestination.objects, 1)

	delete(restartedDestination.objects, onlyMapKey(t, restartedDestination.objects))
	_, err = collecttest.CollectScalarSeries(restarted, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, restartedDestination.objects)
	require.FileExists(t, restarted.stateStore.path)

	_, err = collecttest.CollectScalarSeries(restarted, metrix.ReadFlatten())
	require.NoError(t, err)
	finishMultisiteCleanup(t, restarted)
}

func TestCollector_MultisiteAbandonsPreparedStateWhenSourceNeverCommitted(t *testing.T) {
	collr, source, _ := newMultisiteTestCollector(t)
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.FileExists(t, collr.stateStore.path)
	source.objects = make(map[string][]byte)
	collr.pendingOwnershipState.Phase = string(multisiteSourcePut)
	collr.pendingOwnershipState.SourcePutAt = nil
	collr.pendingOwnershipState.SourcePutAttempted = true
	require.NoError(t, collr.stateStore.save(collr.pendingOwnershipState))

	collr.releaseOwnerLock()
	restarted, _, _ := newMultisiteTestCollectorAt(t, collr.stateStore.path)
	restarted.stateStore = collr.stateStore
	restarted.pendingOwnershipState = collr.pendingOwnershipState
	restartedAt := collr.pendingOwnershipState.CreatedAt.Add(time.Second)
	restarted.now = func() time.Time { return restartedAt }
	restarted.stateStore.now = restarted.now

	metrics, err := collecttest.CollectScalarSeries(restarted, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "failed", multisitePhaseLabels(multisiteSourcePut, reasonRestartAbandoned),
	)])
	require.FileExists(t, restarted.stateStore.path)
	require.NotNil(t, restarted.pendingOwnershipState)

	_, err = collecttest.CollectScalarSeries(restarted, metrix.ReadFlatten())
	require.NoError(t, err)
	finishMultisiteCleanup(t, restarted)
	assert.Nil(t, restarted.pendingOwnershipState)
}

func TestMultisiteStateIsSanitizedAndRejectsChangedRoute(t *testing.T) {
	collr, _, _ := newMultisiteTestCollector(t)
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	raw, err := os.ReadFile(collr.stateStore.path)
	require.NoError(t, err)
	state := string(raw)
	assert.Contains(t, state, `"payload_digest"`)
	assert.NotContains(t, state, "test-access-key-id")
	assert.NotContains(t, state, "test-secret-access-key")
	assert.NotContains(t, state, "test-destination-access-key-id")
	assert.NotContains(t, state, "test-destination-secret-access-key")
	assert.NotContains(t, state, "127.0.0.1")
	fileInfo, err := os.Stat(collr.stateStore.path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())
	dirInfo, err := os.Stat(filepath.Dir(collr.stateStore.path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	assert.NoFileExists(t, collr.stateStore.path+".tmp")

	changed := collr.Config
	changed.Destination.Prefix = "changed/"
	changedCollector := &Collector{
		Config: changed,
	}
	_, err = newOwnershipStateStore(
		collr.stateStore.path, changedCollector.ownershipFingerprint(), collr.stateStore.ownerTag, modeMultisite,
		changedCollector.Prefix, changedCollector.Destination.Prefix,
	).load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different source, destination, or mode settings")
}

func cleanupBatchCount(keys []ownedKey) int {
	bases := make(map[string]struct{}, len(keys))
	for _, owned := range keys {
		bases[path.Base(owned.Key)] = struct{}{}
	}
	return len(bases)
}

func requireChangedMultisiteRouteBlocked(t *testing.T, collr *Collector) *ownershipStateStore {
	t.Helper()

	changed := collr.Config
	changed.Destination = &DestinationConfig{}
	*changed.Destination = *collr.Destination
	changed.Destination.Prefix = "changed-route/"
	changedCollector := &Collector{Config: changed}
	changedStore := newOwnershipStateStore(
		collr.stateStore.path, changedCollector.ownershipFingerprint(), collr.stateStore.ownerTag, modeMultisite,
		changed.Prefix, changed.Destination.Prefix,
	)
	_, err := changedStore.load()
	require.Error(t, err)
	return changedStore
}

func finishMultisiteCleanup(t *testing.T, collr *Collector) map[string]metrix.SampleValue {
	t.Helper()

	initial, err := collr.stateStore.load()
	require.NoError(t, err)
	require.NotNil(t, initial)
	cycles := 4 * cleanupBatchCount(initial.PendingKeys)

	var metrics map[string]metrix.SampleValue
	for range cycles {
		state, err := collr.stateStore.load()
		require.NoError(t, err)
		require.NotNil(t, state)
		require.NotNil(t, state.CleanupQuarantinedAt)

		base := state.CreatedAt
		if state.HeartbeatAt.After(base) {
			base = state.HeartbeatAt
		}
		if state.CleanupQuarantinedAt.After(base) {
			base = *state.CleanupQuarantinedAt
		}
		if state.CleanupConfirmedAt != nil && state.CleanupConfirmedAt.After(base) {
			base = *state.CleanupConfirmedAt
		}
		delay := time.Duration(collr.UpdateEvery)*time.Second + time.Second
		if state.CleanupConfirmedAt == nil {
			delay = collr.multisiteCleanupHorizon() + time.Second
		}
		aged := base.Add(delay)
		state.CleanupQuarantinedAt = &base
		if state.CleanupConfirmedAt != nil {
			confirmed := base
			state.CleanupConfirmedAt = &confirmed
		}
		collr.now = func() time.Time { return aged }
		collr.stateStore.now = collr.now
		require.NoError(t, collr.stateStore.save(state))
		collr.pendingOwnershipState = state

		var collectErr error
		metrics, collectErr = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
		require.NoError(t, collectErr)
		if _, statErr := os.Stat(collr.stateStore.path); errors.Is(statErr, os.ErrNotExist) {
			assert.Nil(t, collr.pendingOwnershipState)
			return metrics
		}
		require.NoError(t, err)
	}
	require.FailNow(t, "multisite cleanup did not finish")
	return nil
}

func newMultisiteTestCollector(t *testing.T) (*Collector, *fakeS3Client, *fakeS3Client) {
	t.Helper()
	return newMultisiteTestCollectorAt(t, filepath.Join(t.TempDir(), "pending.json"))
}

func newMultisiteTestCollectorAt(t *testing.T, statePath string) (*Collector, *fakeS3Client, *fakeS3Client) {
	t.Helper()

	source := &fakeS3Client{
		objects:   make(map[string][]byte),
		staleKeys: make(map[string]bool),
		failures:  make(map[string]error),
		attempts:  make(map[string]int),
	}
	destination := &fakeS3Client{
		objects:   make(map[string][]byte),
		staleKeys: make(map[string]bool),
		failures:  make(map[string]error),
		attempts:  make(map[string]int),
	}

	collr := New()
	collr.Config = validMultisiteTestConfig()
	collr.client = source
	collr.destinationClient = destination
	collr.machineGUID = func() string { return "test-machine-guid" }
	ownerTag := multisiteOwnerTag("test-machine-guid", collr.Name)
	collr.stateStore = newOwnershipStateStore(
		statePath, collr.ownershipFingerprint(), ownerTag, modeMultisite, collr.Prefix, collr.Destination.Prefix,
	)
	collr.stateLock = filelock.New(filepath.Dir(statePath))
	collr.stateLockName = strings.TrimSuffix(filepath.Base(statePath), filepath.Ext(statePath))
	collr.ownerLockName = collr.stateLockName + ".owner"
	clock := &steppingClock{
		now:  time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
		step: 2 * time.Millisecond,
	}
	collr.now = clock.tick
	collr.stateStore.now = collr.now

	sequence := 0
	collr.randomRead = func(size int) ([]byte, error) {
		sequence++
		payload := make([]byte, size)
		for i := range payload {
			payload[i] = byte(sequence + i)
		}
		return payload, nil
	}
	return collr, source, destination
}

func validMultisiteTestConfig() Config {
	config := validTestConfig()
	config.Name = "ceph-site-a-to-site-b"
	config.Mode = modeMultisite
	config.SourceSite = "site-a"
	config.Destination = &DestinationConfig{
		Site:            "site-b",
		Endpoint:        "http://127.0.0.1:9001",
		Region:          "us-east-1",
		Bucket:          "netdata-s3check-destination",
		Prefix:          "netdata-s3check-destination/",
		AccessKeyID:     "test-destination-access-key-id",
		SecretAccessKey: "test-destination-secret-access-key",
		Timeout:         confopt.Duration(defaultTimeout),
	}
	config.RPOThresholdMS = 3600000
	config.ReplicationTimeoutMS = 7200000
	config.DeleteThresholdMS = 3600000
	config.DeleteTimeoutMS = 7200000
	config.VerifyDelete = true
	return config
}

func multisiteStaleProbeKey(index int, ownerTag string) string {
	const digits = "0123456789abcdef"
	suffix := make([]byte, 16)
	for i := range suffix {
		suffix[i] = digits[(index+i)%len(digits)]
	}
	return multisiteProbePrefix(defaultPrefix, ownerTag) + "probe-1-" + string(suffix) + "-" + ownerTag + ".bin"
}

func multisiteStaleDestinationKey(index int, ownerTag string) string {
	sourceKey := multisiteStaleProbeKey(index, ownerTag)
	return multisiteProbePrefix("netdata-s3check-destination/", ownerTag) + path.Base(sourceKey)
}

func multisiteStaleProbeKeys(count int, ownerTag string) []string {
	keys := make([]string, count)
	for i := range count {
		name := fmt.Sprintf("probe-1-%016x-%s.bin", i+1, ownerTag)
		keys[i] = multisiteProbePrefix(defaultPrefix, ownerTag) + name
	}
	return keys
}

func sourceToDestination(t *testing.T, collr *Collector, source, destination *fakeS3Client, mutate bool) {
	t.Helper()
	require.Len(t, source.objects, 1)
	key := onlyMapKey(t, source.objects)
	require.NotNil(t, collr.pendingOwnershipState)
	require.Equal(t, key, collr.pendingOwnershipState.SourceKey)
	payload := append([]byte(nil), source.objects[key]...)
	if mutate {
		payload = append(payload, 'x')
	}
	destination.objects[collr.pendingOwnershipState.DestinationKey] = payload
}

func onlyMapKey(t *testing.T, objects map[string][]byte) string {
	t.Helper()
	require.Len(t, objects, 1)
	for key := range objects {
		return key
	}
	t.Fatal("object map is empty")
	return ""
}

func multisiteSiteLabels() metrix.Labels {
	return metrix.Labels{
		"source_site":      "site-a",
		"destination_site": "site-b",
	}
}

func multisitePhaseLabels(phase multisitePhase, reason string) metrix.Labels {
	labels := multisiteSiteLabels()
	labels["phase"] = string(phase)
	labels["reason"] = reason
	return labels
}

func multisiteDeleteWaitLabels(reason string) metrix.Labels {
	return multisitePhaseLabels(multisiteDeleteWait, reason)
}

func TestCollector_MultisiteReemitsPayloadMismatchDuringRestartCleanup(t *testing.T) {
	collr, _, _ := newMultisiteTestCollector(t)
	createdAt := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	state := &ownershipState{
		Version: ownershipStateVersion, ConfigFingerprint: collr.ownershipFingerprint(),
		Phase: string(multisiteCleanup), SourceKey: multisiteStaleProbeKey(6, collr.stateStore.ownerTag), DestinationKey: multisiteStaleDestinationKey(6, collr.stateStore.ownerTag),
		PayloadDigest: strings.Repeat("0", 64), CreatedAt: createdAt, TerminalReason: reasonPayloadMismatch,
	}
	require.NoError(t, collr.stateStore.save(state))
	collr.pendingOwnershipState = state

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_payload_mismatch", multisiteSiteLabels())])
	finishMultisiteCleanup(t, collr)
}

func TestMultisiteStateRejectsFuturePersistedTimestamps(t *testing.T) {
	collr, _, _ := newMultisiteTestCollector(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Second)
	state := &ownershipState{
		Version: ownershipStateVersion, ConfigFingerprint: collr.ownershipFingerprint(),
		Phase: string(multisiteReplication), SourceKey: multisiteStaleProbeKey(7, collr.stateStore.ownerTag),
		DestinationKey: multisiteStaleDestinationKey(7, collr.stateStore.ownerTag),
		PayloadDigest:  strings.Repeat("0", 64), CreatedAt: now, SourcePutAttempted: true, SourcePutAt: &future,
	}
	state.OwnerTag = collr.stateStore.ownerTag
	state.Mode = modeMultisite
	raw, marshalErr := json.Marshal(state)
	require.NoError(t, marshalErr)
	require.NoError(t, os.WriteFile(collr.stateStore.path, raw, 0o600))

	store := collr.stateStore
	store.now = func() time.Time { return now }
	_, err := store.load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source_put_at timestamp is in the future")
}

func TestMultisiteStateRejectsInvalidCleanupQuarantineMarker(t *testing.T) {
	collr, _, _ := newMultisiteTestCollector(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	tests := map[string]func(*ownershipState){
		"single-site mode": func(state *ownershipState) { state.Mode = modeSingle },
		"active keys remain": func(state *ownershipState) {
			state.SourceKey = multisiteStaleProbeKey(8, collr.stateStore.ownerTag)
			state.DestinationKey = multisiteStaleDestinationKey(8, collr.stateStore.ownerTag)
			state.PayloadDigest = strings.Repeat("0", 64)
		},
		"no pending keys": func(state *ownershipState) { state.PendingKeys = nil },
		"timestamp before creation": func(state *ownershipState) {
			before := state.CreatedAt.Add(-time.Second)
			state.CleanupQuarantinedAt = &before
		},
		"non-cleanup phase": func(state *ownershipState) { state.Phase = string(multisiteReplication) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			ownerTag := collr.stateStore.ownerTag
			marker := now.Add(time.Second)
			state := &ownershipState{
				Version: ownershipStateVersion, ConfigFingerprint: collr.ownershipFingerprint(), OwnerTag: ownerTag,
				Mode: modeMultisite, Phase: string(multisiteCleanup), CreatedAt: now,
				PendingKeys: []ownedKey{
					{Scope: ownershipSource, Key: multisiteStaleProbeKey(9, ownerTag)},
					{Scope: ownershipDestination, Key: multisiteStaleDestinationKey(9, ownerTag)},
				},
				CleanupQuarantinedAt: &marker,
			}
			mutate(state)
			raw, marshalErr := json.Marshal(state)
			require.NoError(t, marshalErr)
			require.NoError(t, os.WriteFile(collr.stateStore.path, raw, 0o600))
			store := collr.stateStore
			store.now = func() time.Time { return now.Add(2 * time.Second) }
			_, err := store.load()
			require.Error(t, err)
		})
	}
}

func TestMultisitePendingKeysRequireCleanupPhase(t *testing.T) {
	collr, _, _ := newMultisiteTestCollector(t)
	now := time.Date(2026, 8, 28, 18, 0, 0, 0, time.UTC)
	state := &ownershipState{
		Version: ownershipStateVersion, ConfigFingerprint: collr.ownershipFingerprint(),
		OwnerTag: collr.stateStore.ownerTag, Mode: modeMultisite, Phase: string(multisiteSourcePut),
		PendingKeys: []ownedKey{{Scope: ownershipSource, Key: multisiteStaleProbeKey(21, collr.stateStore.ownerTag)}},
		CreatedAt:   now,
	}
	raw, marshalErr := json.Marshal(state)
	require.NoError(t, marshalErr)
	require.NoError(t, os.WriteFile(collr.stateStore.path, raw, 0o600))
	store := collr.stateStore
	store.now = func() time.Time { return now.Add(time.Second) }
	_, err := store.load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pending keys require the cleanup phase")
}

func TestCollector_MultisiteOneSidedSourceOwnsDestinationCounterpart(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	sourceKey := multisiteStaleProbeKey(19, collr.stateStore.ownerTag)
	destinationKey := multisiteStaleDestinationKey(19, collr.stateStore.ownerTag)
	source.staleKeys[sourceKey] = true

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, source.staleKeys)
	state, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.Len(t, state.PendingKeys, 2)
	require.Contains(t, state.PendingKeys, ownedKey{Scope: ownershipSource, Key: sourceKey})
	require.Contains(t, state.PendingKeys, ownedKey{Scope: ownershipDestination, Key: destinationKey})
	require.NotNil(t, state.CleanupQuarantinedAt)

	requireChangedMultisiteRouteBlocked(t, collr)

	destination.objects[destinationKey] = []byte("delayed-replica")
	finishMultisiteCleanup(t, collr)
	assert.Empty(t, destination.objects)
}

func TestCollector_MultisiteReconcilesBothPrefixesBeforeNewWrite(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	sourceKey := multisiteStaleProbeKey(1, collr.stateStore.ownerTag)
	destinationKey := multisiteStaleDestinationKey(1, collr.stateStore.ownerTag)
	source.staleKeys[sourceKey] = true
	destination.staleKeys[destinationKey] = true

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, source.staleKeys)
	assert.Empty(t, destination.staleKeys)
	assert.Empty(t, source.objects)

	state, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.Len(t, state.PendingKeys, 2)
	require.NotNil(t, state.CleanupQuarantinedAt)
	requireChangedMultisiteRouteBlocked(t, collr)

	finishMultisiteCleanup(t, collr)
	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Len(t, source.objects, 1)
	assert.Empty(t, destination.objects)
}

func TestCollector_MultisiteCleanupBatchesCounterpartPairsTogether(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	base := time.Date(2026, 8, 28, 20, 0, 0, 0, time.UTC)
	collr.now = func() time.Time { return base }
	collr.stateStore.now = collr.now
	firstSource := multisiteStaleProbeKey(40, collr.stateStore.ownerTag)
	firstDestination := multisiteStaleDestinationKey(40, collr.stateStore.ownerTag)
	secondSource := multisiteStaleProbeKey(41, collr.stateStore.ownerTag)
	secondDestination := multisiteStaleDestinationKey(41, collr.stateStore.ownerTag)
	keys := []ownedKey{
		{Scope: ownershipDestination, Key: firstDestination},
		{Scope: ownershipSource, Key: firstSource},
		{Scope: ownershipDestination, Key: secondDestination},
		{Scope: ownershipSource, Key: secondSource},
	}
	for _, owned := range keys {
		if owned.Scope == ownershipSource {
			source.staleKeys[owned.Key] = true
		} else {
			destination.staleKeys[owned.Key] = true
		}
	}
	state := newMultisitePendingCleanupState(collr, keys, base, base)
	require.NoError(t, collr.saveOwnership(state))
	collr.pendingOwnershipState = state

	collr.now = func() time.Time { return base.Add(time.Duration(collr.UpdateEvery)*time.Second + time.Second) }
	collr.stateStore.now = collr.now
	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "waiting", multisitePhaseLabels(multisiteCleanup, reasonQuarantinePending),
	)])
	require.Equal(t, []string{firstSource}, deletedKeys(source))
	require.Equal(t, []string{firstDestination}, deletedKeys(destination))

	remaining, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.Len(t, remaining.PendingKeys, 4)
	require.Nil(t, remaining.CleanupConfirmedAt)
	require.NotNil(t, remaining.CleanupQuarantinedAt)

	requireChangedMultisiteRouteBlocked(t, collr)

	finishMultisiteCleanup(t, collr)
	assert.Empty(t, source.staleKeys)
	assert.Empty(t, destination.staleKeys)
}

func deletedKeys(client *fakeS3Client) []string {
	keys := make([]string, 0)
	for _, call := range client.calls {
		if call.operation == "delete" {
			keys = append(keys, call.key)
		}
	}
	return keys
}

func TestCollector_MultisiteReconciliationOwnsFullSupportedKeySet(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	sourceKeys := multisiteStaleProbeKeys(maxOwnedKeys/2, collr.stateStore.ownerTag)
	for _, sourceKey := range sourceKeys {
		source.staleKeys[sourceKey] = true
		destinationKey := multisiteProbePrefix("netdata-s3check-destination/", collr.stateStore.ownerTag) + path.Base(sourceKey)
		destination.staleKeys[destinationKey] = true
	}

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	state, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.Len(t, state.PendingKeys, maxOwnedKeys)

	requireChangedMultisiteRouteBlocked(t, collr)

	finishMultisiteCleanup(t, collr)
	assert.Empty(t, source.staleKeys)
	assert.Empty(t, destination.staleKeys)
	assert.NoFileExists(t, collr.stateStore.path)
}

func TestCollector_MultisitePartialReconciliationDurablyBlocksRoute(t *testing.T) {
	tests := map[string]struct {
		failingEndpoint  string
		sourceStale      bool
		destinationStale bool
	}{
		"destination list fails after source discovery": {
			failingEndpoint: "destination", sourceStale: true,
		},
		"source list fails after destination discovery": {
			failingEndpoint: "source", destinationStale: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, source, destination := newMultisiteTestCollector(t)
			sourceKey := multisiteStaleProbeKey(42, collr.stateStore.ownerTag)
			destinationKey := multisiteStaleDestinationKey(42, collr.stateStore.ownerTag)
			if test.sourceStale {
				source.staleKeys[sourceKey] = true
			}
			if test.destinationStale {
				destination.staleKeys[destinationKey] = true
			}
			if test.failingEndpoint == "source" {
				source.failures["list"] = errors.New("secret-source-list-failure")
			} else {
				destination.failures["list"] = errors.New("secret-destination-list-failure")
			}

			metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
				"multisite_status", "failed", multisitePhaseLabels(multisiteSetup, reasonRequestFailed),
			)])
			assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
			require.FileExists(t, collr.stateStore.path)
			blocker, loadErr := collr.stateStore.load()
			require.NoError(t, loadErr)
			assert.True(t, blocker.ReconciliationPending)
			require.Len(t, blocker.PendingKeys, 2)
			assert.Contains(t, blocker.PendingKeys, ownedKey{Scope: ownershipSource, Key: sourceKey})
			assert.Contains(t, blocker.PendingKeys, ownedKey{Scope: ownershipDestination, Key: destinationKey})
			assert.NotContains(t, source.operations(), "delete")
			assert.NotContains(t, destination.operations(), "delete")

			requireChangedMultisiteRouteBlocked(t, collr)

			if test.failingEndpoint == "source" {
				delete(source.failures, "list")
			} else {
				delete(destination.failures, "list")
			}
			_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			ready, loadErr := collr.stateStore.load()
			require.NoError(t, loadErr)
			assert.False(t, ready.ReconciliationPending)
			require.Len(t, ready.PendingKeys, 2)
			require.NotNil(t, ready.CleanupQuarantinedAt)
			assert.NotContains(t, source.staleKeys, sourceKey)
			assert.NotContains(t, destination.staleKeys, destinationKey)

			finishMultisiteCleanup(t, collr)
		})
	}
}

func TestCollector_MultisiteResumedPartialDiscoverySurvivesObjectRemoval(t *testing.T) {
	tests := map[string]struct {
		discoveredEndpoint string
	}{
		"source discovery remains durable":      {discoveredEndpoint: "source"},
		"destination discovery remains durable": {discoveredEndpoint: "destination"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, source, destination := newMultisiteTestCollector(t)
			destination.failures["list"] = errors.New("secret-first-list-failure")
			_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			initial, loadErr := collr.stateStore.load()
			require.NoError(t, loadErr)
			require.True(t, initial.ReconciliationPending)
			require.Empty(t, initial.PendingKeys)

			sourceKey := multisiteStaleProbeKey(43, collr.stateStore.ownerTag)
			destinationKey := multisiteStaleDestinationKey(43, collr.stateStore.ownerTag)
			if test.discoveredEndpoint == "source" {
				source.staleKeys[sourceKey] = true
			} else {
				delete(destination.failures, "list")
				source.failures["list"] = errors.New("secret-source-list-failure")
				destination.staleKeys[destinationKey] = true
			}
			_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			partial, loadErr := collr.stateStore.load()
			require.NoError(t, loadErr)
			require.True(t, partial.ReconciliationPending)
			require.Len(t, partial.PendingKeys, 2)
			require.Contains(t, partial.PendingKeys, ownedKey{Scope: ownershipSource, Key: sourceKey})
			require.Contains(t, partial.PendingKeys, ownedKey{Scope: ownershipDestination, Key: destinationKey})

			if test.discoveredEndpoint == "source" {
				delete(source.staleKeys, sourceKey)
			} else {
				delete(destination.staleKeys, destinationKey)
			}
			_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			retained, loadErr := collr.stateStore.load()
			require.NoError(t, loadErr)
			require.True(t, retained.ReconciliationPending)
			require.Len(t, retained.PendingKeys, 2)

			requireChangedMultisiteRouteBlocked(t, collr)

			delete(source.failures, "list")
			delete(destination.failures, "list")
			_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			ready, loadErr := collr.stateStore.load()
			require.NoError(t, loadErr)
			assert.False(t, ready.ReconciliationPending)
			require.Len(t, ready.PendingKeys, 2)
			require.NotNil(t, ready.CleanupQuarantinedAt)
			finishMultisiteCleanup(t, collr)
		})
	}
}

func TestCollector_MultisiteKeylessPartialReconciliationBlocksAndClears(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	destination.failures["list"] = errors.New("secret-destination-list-failure")

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "failed", multisitePhaseLabels(multisiteSetup, reasonRequestFailed),
	)])
	require.FileExists(t, collr.stateStore.path)
	blocker, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	assert.True(t, blocker.ReconciliationPending)
	assert.Empty(t, blocker.PendingKeys)

	requireChangedMultisiteRouteBlocked(t, collr)

	delete(destination.failures, "list")
	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.NotNil(t, collr.pendingOwnershipState)
	assert.False(t, collr.pendingOwnershipState.ReconciliationPending)
	require.NotEmpty(t, collr.pendingOwnershipState.SourceKey)
	require.Len(t, source.objects, 1)
}

func TestCollector_MultisiteShutdownRetainsKeylessReconciliationBlocker(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	destination.failures["list"] = errors.New("secret-destination-list-failure")

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.FileExists(t, collr.stateStore.path)
	operations := len(source.operations()) + len(destination.operations())

	collr.Cleanup(context.Background())
	require.FileExists(t, collr.stateStore.path)
	blocker, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	assert.True(t, blocker.ReconciliationPending)
	assert.Empty(t, blocker.PendingKeys)
	require.NotNil(t, blocker.RetiredAt)
	assert.Equal(t, operations, len(source.operations())+len(destination.operations()))
}

func TestCollector_MultisiteReconciliationFailsClosedAboveOwnedKeyLimit(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	for _, key := range multisiteStaleProbeKeys(maxOwnedKeys+1, collr.stateStore.ownerTag) {
		source.staleKeys[key] = true
	}

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "failed", multisitePhaseLabels(multisiteSetup, reasonInternal),
	)])
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
	assert.Equal(t, []string{"list"}, source.operations())
	assert.Equal(t, []string{"list"}, destination.operations())
	assert.Len(t, source.staleKeys, maxOwnedKeys+1)
	require.FileExists(t, collr.stateStore.path)
	blocker, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	assert.True(t, blocker.ReconciliationPending)
	assert.Empty(t, blocker.PendingKeys)
	assert.Nil(t, blocker.CleanupQuarantinedAt)

	requireChangedMultisiteRouteBlocked(t, collr)
}

func TestCollector_MultisiteCleanupRefreshFailsClosedAboveOwnedKeyLimit(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	base := time.Date(2026, 8, 28, 21, 0, 0, 0, time.UTC)
	collr.now = func() time.Time { return base }
	collr.stateStore.now = collr.now
	keys := multisiteStaleProbeKeys(maxOwnedKeys+1, collr.stateStore.ownerTag)
	for _, key := range keys {
		source.staleKeys[key] = true
	}
	destinationKey := multisiteProbePrefix("netdata-s3check-destination/", collr.stateStore.ownerTag) + path.Base(keys[0])
	destination.staleKeys[destinationKey] = true
	pending := []ownedKey{
		{Scope: ownershipSource, Key: keys[0]},
		{Scope: ownershipDestination, Key: destinationKey},
	}
	state := newMultisitePendingCleanupState(collr, pending, base, time.Time{})
	require.NoError(t, collr.saveOwnership(state))
	collr.pendingOwnershipState = state

	collr.now = func() time.Time { return base.Add(collr.multisiteCleanupHorizon() + time.Second) }
	collr.stateStore.now = collr.now
	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "failed", multisitePhaseLabels(multisiteCleanup, reasonInternal),
	)])
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
	assert.Equal(t, []string{"versioning", "list"}, source.operations())
	assert.Equal(t, []string{"versioning"}, destination.operations())
	assert.Len(t, source.staleKeys, maxOwnedKeys+1)
	assert.Contains(t, destination.staleKeys, destinationKey)
	require.FileExists(t, collr.stateStore.path)
	remaining, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.Len(t, remaining.PendingKeys, 2)
}

func TestCollector_MultisiteCleanupStopsBeforeDeleteWhenVersioningDrifts(t *testing.T) {
	tests := map[string]struct {
		endpoint string
		status   string
		failure  error
	}{
		"source becomes versioned":      {endpoint: "source", status: "Enabled"},
		"destination becomes versioned": {endpoint: "destination", status: "Suspended"},
		"source versioning check fails": {
			endpoint: "source", failure: errors.New("secret-endpoint-detail"),
		},
		"destination versioning check fails": {
			endpoint: "destination", failure: errors.New("secret-endpoint-detail"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, source, destination := newMultisiteTestCollector(t)
			createdAt := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
			fingerprint := collr.ownershipFingerprint()
			ownerTag := collr.stateStore.ownerTag
			sourceKey := multisiteStaleProbeKey(11, ownerTag)
			destinationKey := multisiteStaleDestinationKey(11, ownerTag)
			state := &ownershipState{
				Version: ownershipStateVersion, ConfigFingerprint: fingerprint, Phase: string(multisiteCleanup),
				SourceKey: sourceKey, DestinationKey: destinationKey, PayloadDigest: strings.Repeat("0", 64),
				CreatedAt: createdAt,
			}
			source.objects[sourceKey] = []byte("payload")
			destination.objects[destinationKey] = []byte("payload")
			require.NoError(t, collr.stateStore.save(state))
			collr.pendingOwnershipState = state

			if test.endpoint == "source" {
				source.versioningStatus = test.status
				if test.failure != nil {
					source.failures["versioning"] = test.failure
				}
			} else {
				destination.versioningStatus = test.status
				if test.failure != nil {
					destination.failures["versioning"] = test.failure
				}
			}

			wantReason := reasonRequestFailed
			if test.status != "" {
				wantReason = reasonBucketVersioned
			}
			metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
				"multisite_status", "failed", multisitePhaseLabels(multisiteCleanup, wantReason),
			)])
			assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
			assert.NotContains(t, source.operations(), "delete")
			assert.NotContains(t, destination.operations(), "delete")
			require.FileExists(t, collr.stateStore.path)
			require.NotNil(t, collr.pendingOwnershipState)

			_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.NotContains(t, source.operations(), "delete")
			assert.NotContains(t, destination.operations(), "delete")
			require.Len(t, source.objects, 1)
			require.Len(t, destination.objects, 1)
			require.FileExists(t, collr.stateStore.path)
			require.NotNil(t, collr.pendingOwnershipState)
		})
	}
}

func newMultisitePendingCleanupState(collr *Collector, keys []ownedKey, quarantinedAt, confirmedAt time.Time) *ownershipState {
	state := &ownershipState{
		Phase: string(multisiteCleanup), PendingKeys: keys, CreatedAt: quarantinedAt,
		CleanupQuarantinedAt: &quarantinedAt, TerminalReason: reasonReplicationTimeout,
	}
	if !confirmedAt.IsZero() {
		state.CleanupConfirmedAt = &confirmedAt
	}
	return state
}

func TestCollector_MultisiteCleanupWaitsForConfiguredHorizon(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	base := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	collr.now = func() time.Time { return base }
	collr.stateStore.now = collr.now
	keys := []ownedKey{
		{Scope: ownershipSource, Key: multisiteStaleProbeKey(31, collr.stateStore.ownerTag)},
		{Scope: ownershipDestination, Key: multisiteStaleDestinationKey(31, collr.stateStore.ownerTag)},
	}
	state := newMultisitePendingCleanupState(collr, keys, base, time.Time{})
	require.NoError(t, collr.saveOwnership(state))
	collr.pendingOwnershipState = state

	beforeHorizon := base.Add(time.Duration(collr.UpdateEvery)*time.Second + time.Second)
	collr.now = func() time.Time { return beforeHorizon }
	collr.stateStore.now = collr.now
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, source.operations())
	assert.Empty(t, destination.operations())

	atHorizon := base.Add(collr.multisiteCleanupHorizon() + time.Second)
	collr.now = func() time.Time { return atHorizon }
	collr.stateStore.now = collr.now
	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, []string{"versioning", "list"}, source.operations())
	assert.Equal(t, []string{"versioning", "list"}, destination.operations())
	confirmed, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.NotNil(t, confirmed.CleanupConfirmedAt)
}

func TestCollector_MultisiteCleanupListFailureRemainsCategorical(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	base := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	collr.now = func() time.Time { return base }
	collr.stateStore.now = collr.now
	keys := []ownedKey{
		{Scope: ownershipSource, Key: multisiteStaleProbeKey(30, collr.stateStore.ownerTag)},
		{Scope: ownershipDestination, Key: multisiteStaleDestinationKey(30, collr.stateStore.ownerTag)},
	}
	state := newMultisitePendingCleanupState(collr, keys, base, time.Time{})
	require.NoError(t, collr.saveOwnership(state))
	collr.pendingOwnershipState = state
	source.failures["list"] = errors.New("secret-list-failure")

	atHorizon := base.Add(collr.multisiteCleanupHorizon() + time.Second)
	collr.now = func() time.Time { return atHorizon }
	collr.stateStore.now = collr.now
	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "failed", multisitePhaseLabels(multisiteCleanup, reasonRequestFailed),
	)])
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
	assert.Equal(t, []string{"versioning", "list"}, source.operations())
	assert.Equal(t, []string{"versioning"}, destination.operations())
	for key := range metrics {
		assert.NotContains(t, key, "secret-list-failure")
	}
}

func TestCollector_MultisiteCleanupConfirmationResetsForRemainingBatch(t *testing.T) {
	collr, _, _ := newMultisiteTestCollector(t)
	base := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	collr.now = func() time.Time { return base }
	collr.stateStore.now = collr.now
	keys := []ownedKey{
		{Scope: ownershipDestination, Key: multisiteStaleDestinationKey(28, collr.stateStore.ownerTag)},
		{Scope: ownershipSource, Key: multisiteStaleProbeKey(28, collr.stateStore.ownerTag)},
		{Scope: ownershipSource, Key: multisiteStaleProbeKey(29, collr.stateStore.ownerTag)},
	}
	state := newMultisitePendingCleanupState(collr, keys, base, base)
	require.NoError(t, collr.saveOwnership(state))
	collr.pendingOwnershipState = state

	eligible := base.Add(time.Duration(collr.UpdateEvery)*time.Second + time.Second)
	collr.now = func() time.Time { return eligible }
	collr.stateStore.now = collr.now
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.FileExists(t, collr.stateStore.path)
	remaining, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.Len(t, remaining.PendingKeys, 1)
	require.Nil(t, remaining.CleanupConfirmedAt)
	require.NotNil(t, remaining.CleanupQuarantinedAt)
	assert.True(t, remaining.CleanupQuarantinedAt.After(base))
}

func TestCollector_MultisitePayloadMismatchPersistsDuringCleanupQuarantine(t *testing.T) {
	collr, _, _ := newMultisiteTestCollector(t)
	base := time.Date(2026, 8, 28, 14, 0, 0, 0, time.UTC)
	collr.now = func() time.Time { return base }
	collr.stateStore.now = collr.now
	keys := []ownedKey{
		{Scope: ownershipSource, Key: multisiteStaleProbeKey(26, collr.stateStore.ownerTag)},
		{Scope: ownershipDestination, Key: multisiteStaleDestinationKey(26, collr.stateStore.ownerTag)},
	}
	state := newMultisitePendingCleanupState(collr, keys, base, time.Time{})
	state.TerminalReason = reasonPayloadMismatch
	require.NoError(t, collr.saveOwnership(state))
	collr.pendingOwnershipState = state

	collr.now = func() time.Time { return base.Add(collr.multisiteCleanupHorizon() - time.Second) }
	collr.stateStore.now = collr.now
	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_payload_mismatch", multisiteSiteLabels())])
	assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "waiting", multisitePhaseLabels(multisiteCleanup, reasonQuarantinePending),
	)])
}

func TestCollector_MultisiteCrashedRetryWaitsForDestructiveGrace(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	base := time.Date(2026, 8, 28, 15, 0, 0, 0, time.UTC)
	collr.now = func() time.Time { return base }
	collr.stateStore.now = collr.now
	keys := []ownedKey{
		{Scope: ownershipSource, Key: multisiteStaleProbeKey(25, collr.stateStore.ownerTag)},
		{Scope: ownershipDestination, Key: multisiteStaleDestinationKey(25, collr.stateStore.ownerTag)},
	}
	state := newMultisitePendingCleanupState(collr, keys, base, time.Time{})
	state.CleanupDeleteAttempted = true
	require.NoError(t, collr.saveOwnership(state))
	collr.pendingOwnershipState = state

	beforeGrace := base.Add(collr.multisiteCleanupHorizon() + time.Second)
	collr.now = func() time.Time { return beforeGrace }
	collr.stateStore.now = collr.now
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, source.operations())
	assert.Empty(t, destination.operations())

	afterGrace := base.Add(collr.multisiteCleanupHorizon() + collr.multisiteDestructiveRetryGrace() + time.Second)
	collr.now = func() time.Time { return afterGrace }
	collr.stateStore.now = collr.now
	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	confirmed, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.NotNil(t, confirmed.CleanupConfirmedAt)
	assert.False(t, confirmed.CleanupDeleteAttempted)
}

func TestCollector_MultisiteJournalClearFailureRetainsOriginalKeys(t *testing.T) {
	collr, _, _ := newMultisiteTestCollector(t)
	base := time.Date(2026, 8, 28, 16, 0, 0, 0, time.UTC)
	collr.now = func() time.Time { return base }
	collr.stateStore.now = collr.now
	keys := []ownedKey{
		{Scope: ownershipSource, Key: multisiteStaleProbeKey(24, collr.stateStore.ownerTag)},
		{Scope: ownershipDestination, Key: multisiteStaleDestinationKey(24, collr.stateStore.ownerTag)},
	}
	state := newMultisitePendingCleanupState(collr, keys, base, base)
	require.NoError(t, collr.saveOwnership(state))
	collr.pendingOwnershipState = state
	originalPath := collr.stateStore.path

	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocker, []byte("blocker"), 0o600))
	collr.stateStore.path = filepath.Join(blocker, "pending.json")
	cycle := newMultisiteCycle()
	assert.False(t, collr.finishMultisiteBatch(cycle, keys))
	assert.Len(t, collr.pendingOwnershipState.PendingKeys, 2)

	collr.stateStore.path = originalPath
	assert.True(t, collr.finishMultisiteBatch(cycle, keys))
	assert.Nil(t, collr.pendingOwnershipState)
	assert.NoFileExists(t, originalPath)
}

func TestCollector_SingleSiteJournalClearFailureRetainsOriginalKeys(t *testing.T) {
	collr, client := newTestCollector(t)
	base := time.Date(2026, 8, 28, 19, 0, 0, 0, time.UTC)
	collr.now = func() time.Time { return base }
	collr.stateStore.now = collr.now
	keys := []ownedKey{
		{Scope: ownershipSingle, Key: staleProbeKey(23)},
		{Scope: ownershipSingle, Key: staleProbeKey(22)},
	}
	state := &ownershipState{
		Phase: string(multisiteCleanup), PendingKeys: keys, CreatedAt: base,
	}
	require.NoError(t, collr.saveOwnership(state))
	collr.pendingOwnershipState = state
	originalPath := collr.stateStore.path

	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocker, []byte("blocker"), 0o600))
	invalidPath := filepath.Join(blocker, "pending.json")
	deletes := 0
	client.afterDelete = func() {
		deletes++
		if deletes == 2 {
			collr.stateStore.path = invalidPath
		}
	}

	results := newStageResults()
	assert.False(t, collr.cleanupOwnedKeyBatch(context.Background(), results, cleanupBatchSize))
	require.Len(t, collr.pendingOwnershipState.PendingKeys, 1)
	retained := collr.pendingOwnershipState.PendingKeys[0].Key
	require.Contains(t, keys, ownedKey{Scope: ownershipSingle, Key: retained})

	collr.stateStore.path = originalPath
	loaded, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.Len(t, loaded.PendingKeys, 1)
	assert.Equal(t, retained, loaded.PendingKeys[0].Key)

	client.afterDelete = nil
	results = newStageResults()
	assert.True(t, collr.cleanupOwnedKeyBatch(context.Background(), results, cleanupBatchSize))
	assert.Nil(t, collr.pendingOwnershipState)
	assert.NoFileExists(t, originalPath)
}

func TestCollector_MultisiteRetryFailureResetsConfirmationBeforeDelete(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	base := time.Date(2026, 8, 28, 13, 0, 0, 0, time.UTC)
	collr.now = func() time.Time { return base }
	collr.stateStore.now = collr.now
	destinationKey := multisiteStaleDestinationKey(27, collr.stateStore.ownerTag)
	keys := []ownedKey{
		{Scope: ownershipSource, Key: multisiteStaleProbeKey(27, collr.stateStore.ownerTag)},
		{Scope: ownershipDestination, Key: destinationKey},
	}
	state := newMultisitePendingCleanupState(collr, keys, base, base)
	require.NoError(t, collr.saveOwnership(state))
	collr.pendingOwnershipState = state
	destination.objects[destinationKey] = []byte("late-copy")
	destination.afterDelete = func() {
		destination.objects[destinationKey] = []byte("late-copy-after-delete")
	}

	eligible := base.Add(time.Duration(collr.UpdateEvery)*time.Second + time.Second)
	collr.now = func() time.Time { return eligible }
	collr.stateStore.now = collr.now
	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "failed", multisitePhaseLabels(multisiteCleanup, reasonStillPresent),
	)])

	reset, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.Nil(t, reset.CleanupConfirmedAt)
	require.NotNil(t, reset.CleanupQuarantinedAt)
	assert.True(t, reset.CleanupQuarantinedAt.After(base))
	assert.Contains(t, destination.objects, destinationKey)

	beforeNewHorizon := reset.CleanupQuarantinedAt.Add(time.Duration(collr.UpdateEvery)*time.Second + time.Second)
	collr.now = func() time.Time { return beforeNewHorizon }
	collr.stateStore.now = collr.now
	collr.pendingOwnershipState = reset
	operations := len(source.operations()) + len(destination.operations())
	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, operations, len(source.operations())+len(destination.operations()))
}

func TestCollector_MultisiteCleanupQuarantineCatchesLateArrivalBeforeRouteChange(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	state := collr.pendingOwnershipState
	require.NotNil(t, state)
	source.objects[state.SourceKey] = []byte("late-source")
	destination.objects[state.DestinationKey] = []byte("late-destination")
	state.Phase = string(multisiteCleanup)
	state.TerminalReason = reasonReplicationTimeout
	require.NoError(t, collr.stateStore.save(state))

	var destinationProofAt time.Time
	destination.afterHead = func() { destinationProofAt = collr.now() }
	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, source.objects)
	assert.Empty(t, destination.objects)
	require.FileExists(t, collr.stateStore.path)
	quarantined, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.NotNil(t, quarantined.CleanupQuarantinedAt)
	require.Len(t, quarantined.PendingKeys, 2)
	assert.True(t, quarantined.CleanupQuarantinedAt.After(destinationProofAt))

	changedStore := requireChangedMultisiteRouteBlocked(t, collr)

	for _, owned := range quarantined.PendingKeys {
		if owned.Scope == ownershipSource {
			source.objects[owned.Key] = []byte("late-source")
		} else {
			destination.objects[owned.Key] = []byte("late-destination")
		}
	}
	finishMultisiteCleanup(t, collr)
	assert.Empty(t, source.objects)
	assert.Empty(t, destination.objects)

	_, loadErr = changedStore.load()
	require.NoError(t, loadErr)
}

func TestCollector_MultisitePendingCleanupRechecksSourceAfterDestinationWork(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	state := collr.pendingOwnershipState
	require.NotNil(t, state)
	sourceKey, destinationKey := state.SourceKey, state.DestinationKey
	state.Phase = string(multisiteCleanup)
	state.TerminalReason = reasonReplicationTimeout
	require.NoError(t, collr.stateStore.save(state))

	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	quarantined, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.Len(t, quarantined.PendingKeys, 2)

	aged := quarantined.HeartbeatAt.Add(collr.multisiteCleanupHorizon() + time.Second)
	base := quarantined.HeartbeatAt
	quarantined.CleanupQuarantinedAt = &base
	collr.now = func() time.Time { return aged }
	collr.stateStore.now = collr.now
	require.NoError(t, collr.stateStore.save(quarantined))
	collr.pendingOwnershipState = quarantined

	destination.afterDelete = func() {
		source.objects[sourceKey] = []byte("late-source-during-destination-work")
	}
	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.FileExists(t, collr.stateStore.path)
	finishMultisiteCleanup(t, collr)
	assert.Empty(t, source.objects)
	assert.Empty(t, destination.objects)
	assert.NotContains(t, source.objects, sourceKey)
	assert.NotContains(t, destination.objects, destinationKey)
}

func TestCollector_MultisitePendingCleanupKeepsLateDestinationJournaled(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	state := collr.pendingOwnershipState
	require.NotNil(t, state)
	sourceKey, destinationKey := state.SourceKey, state.DestinationKey
	state.Phase = string(multisiteCleanup)
	state.TerminalReason = reasonReplicationTimeout
	require.NoError(t, collr.stateStore.save(state))

	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	quarantined, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.Len(t, quarantined.PendingKeys, 2)

	aged := quarantined.HeartbeatAt.Add(collr.multisiteCleanupHorizon() + time.Second)
	base := quarantined.HeartbeatAt
	quarantined.CleanupQuarantinedAt = &base
	collr.now = func() time.Time { return aged }
	collr.stateStore.now = collr.now
	require.NoError(t, collr.stateStore.save(quarantined))
	collr.pendingOwnershipState = quarantined

	destination.objects[destinationKey] = []byte("initial-destination")
	source.afterDelete = func() {
		destination.objects[destinationKey] = []byte("late-destination-during-source-work")
	}
	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Contains(t, destination.objects, destinationKey)
	require.FileExists(t, collr.stateStore.path)
	retained, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.Len(t, retained.PendingKeys, 2)

	source.afterDelete = nil
	finishMultisiteCleanup(t, collr)
	assert.Empty(t, source.objects)
	assert.Empty(t, destination.objects)
	assert.NotContains(t, source.objects, sourceKey)
	assert.NotContains(t, destination.objects, destinationKey)
}

func TestMultisiteFingerprintAcceptsEquivalentEndpointSpellings(t *testing.T) {
	collr, _, _ := newMultisiteTestCollector(t)
	require.Equal(t, canonicalEndpointKey("http://example.net"), canonicalEndpointKey("HTTP://Example.NET:80"))
	require.Equal(t, canonicalEndpointKey("http://example.net"), canonicalEndpointKey("http://example.net."))
	require.Equal(t, canonicalEndpointKey("http://[::1]:9000"), canonicalEndpointKey("http://[0:0:0:0:0:0:0:1]:9000"))
	require.Equal(t, canonicalEndpointKey("https://example.net"), canonicalEndpointKey("https://example.net:0443"))
	require.Equal(t, canonicalEndpointKey("http://example.net:080"), canonicalEndpointKey("http://example.net"))
	require.Equal(t, canonicalEndpointKey("http://[fe80::1%25eth0]:9000"), canonicalEndpointKey("http://[fe80:0:0:0:0:0:0:1%25eth0]:9000"))
	require.NotEqual(t, canonicalEndpointKey("http://[fe80::1%25eth0]:9000"), canonicalEndpointKey("http://[fe80::1%25ETH0]:9000"))
	require.Equal(t, canonicalEndpointKey("http://[fe80::1%253]:9000"), canonicalEndpointKey("http://[fe80:0:0:0:0:0:0:1%2503]:9000"))
	fingerprint := collr.ownershipFingerprint()
	ownerTag := collr.stateStore.ownerTag
	createdAt := time.Date(2026, 8, 26, 11, 0, 0, 0, time.UTC)
	state := &ownershipState{
		Version: ownershipStateVersion, ConfigFingerprint: fingerprint, Phase: string(multisiteCleanup),
		SourceKey: multisiteStaleProbeKey(17, ownerTag), DestinationKey: multisiteStaleDestinationKey(17, ownerTag),
		PayloadDigest: strings.Repeat("0", 64), CreatedAt: createdAt,
	}
	require.NoError(t, collr.stateStore.save(state))

	changed := collr.Config
	changed.Destination = &DestinationConfig{}
	*changed.Destination = *collr.Destination
	changed.Endpoint = collr.Endpoint + "/"
	changed.Destination.Endpoint = "http://127.0.0.1:9001/"
	changedCollector := &Collector{Config: changed}
	changedFingerprint := changedCollector.ownershipFingerprint()
	require.Equal(t, fingerprint, changedFingerprint)

	changedStore := newOwnershipStateStore(
		collr.stateStore.path, changedFingerprint, collr.stateStore.ownerTag, modeMultisite,
		changedCollector.Prefix, changedCollector.Destination.Prefix,
	)
	loaded, err := changedStore.load()
	require.NoError(t, err)
	require.Equal(t, state, loaded)
}

func TestCollector_MultisiteDoesNotWriteWhenBucketVersioningDrifts(t *testing.T) {
	tests := map[string]func(source, destination *fakeS3Client){
		"source becomes versioned":      func(source, _ *fakeS3Client) { source.versioningStatus = "Enabled" },
		"destination becomes versioned": func(_, destination *fakeS3Client) { destination.versioningStatus = "Suspended" },
	}

	for name, drift := range tests {
		t.Run(name, func(t *testing.T) {
			collr, source, destination := newMultisiteTestCollector(t)
			drift(source, destination)

			metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
				"multisite_status", "failed", multisitePhaseLabels(multisiteSetup, reasonBucketVersioned),
			)])
			assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
			assert.NotContains(t, source.operations(), "put")
			assert.NotContains(t, destination.operations(), "get")
			assert.Empty(t, source.objects)
			assert.Empty(t, destination.objects)
			assert.NoFileExists(t, collr.stateStore.path)
		})
	}
}

func TestMultisiteStateRejectsOversizedAndUnboundedInput(t *testing.T) {
	collr, _, _ := newMultisiteTestCollector(t)
	require.NoError(t, os.WriteFile(collr.stateStore.path, []byte(strings.Repeat("x", 32769)), 0o600))
	_, err := collr.stateStore.load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pending state file is too large")

	fingerprint := collr.ownershipFingerprint()
	ownerTag := collr.stateStore.ownerTag
	state := &ownershipState{
		Version: ownershipStateVersion, ConfigFingerprint: fingerprint,
		Phase:          strings.Repeat("A", 512),
		SourceKey:      multisiteStaleProbeKey(19, ownerTag),
		DestinationKey: multisiteStaleDestinationKey(19, ownerTag),
		PayloadDigest:  strings.Repeat("0", 64), CreatedAt: time.Date(2026, 8, 26, 11, 0, 0, 0, time.UTC),
	}
	state.OwnerTag = collr.stateStore.ownerTag
	state.Mode = modeMultisite
	raw, marshalErr := json.Marshal(state)
	require.NoError(t, marshalErr)
	require.NoError(t, os.WriteFile(collr.stateStore.path, raw, 0o600))
	_, err = collr.stateStore.load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pending state has an invalid phase")
	assert.NotContains(t, err.Error(), "AAAA")
}

func TestCollector_MultisiteSourceDeleteStopsWhenVersioningDrifts(t *testing.T) {
	tests := map[string]struct {
		status  string
		failure error
	}{
		"source becomes versioned":      {status: "Enabled"},
		"source versioning check fails": {failure: errors.New("secret-endpoint-detail")},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, source, destination := newMultisiteTestCollector(t)
			_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			sourceToDestination(t, collr, source, destination, false)
			source.versioningStatus = test.status
			if test.failure != nil {
				source.failures["versioning"] = test.failure
			}

			wantReason := reasonRequestFailed
			if test.status != "" {
				wantReason = reasonBucketVersioned
			}
			metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
				"multisite_status", "failed", multisitePhaseLabels(multisiteSourceDelete, wantReason),
			)])
			assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
			assert.NotContains(t, source.operations(), "delete")
			require.Len(t, source.objects, 1)
			require.Len(t, destination.objects, 1)
			require.FileExists(t, collr.stateStore.path)
			require.NotNil(t, collr.pendingOwnershipState)

			_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.NotContains(t, source.operations(), "delete")
			require.Len(t, source.objects, 1)
			require.Len(t, destination.objects, 1)
			require.FileExists(t, collr.stateStore.path)
			require.NotNil(t, collr.pendingOwnershipState)
		})
	}
}

func TestCollector_MultisitePrefixReconciliationStopsWhenVersioningDrifts(t *testing.T) {
	tests := map[string]struct {
		endpoint string
		status   string
		failure  error
	}{
		"source becomes versioned":      {endpoint: "source", status: "Enabled"},
		"destination becomes versioned": {endpoint: "destination", status: "Suspended"},
		"source versioning check fails": {
			endpoint: "source", failure: errors.New("secret-endpoint-detail"),
		},
		"destination versioning check fails": {
			endpoint: "destination", failure: errors.New("secret-endpoint-detail"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, source, destination := newMultisiteTestCollector(t)
			source.staleKeys[multisiteStaleProbeKey(13, collr.stateStore.ownerTag)] = true
			destination.staleKeys[multisiteStaleDestinationKey(13, collr.stateStore.ownerTag)] = true
			if test.endpoint == "source" {
				source.versioningStatus = test.status
				if test.failure != nil {
					source.failures["versioning"] = test.failure
				}
			} else {
				destination.versioningStatus = test.status
				if test.failure != nil {
					destination.failures["versioning"] = test.failure
				}
			}

			wantReason := reasonRequestFailed
			if test.status != "" {
				wantReason = reasonBucketVersioned
			}
			metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
				"multisite_status", "failed", multisitePhaseLabels(multisiteCleanup, wantReason),
			)])
			assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])

			state, loadErr := collr.stateStore.load()
			require.NoError(t, loadErr)
			require.Len(t, state.PendingKeys, 2)
			require.NotNil(t, state.CleanupQuarantinedAt)

			requireChangedMultisiteRouteBlocked(t, collr)

			source.versioningStatus = ""
			destination.versioningStatus = ""
			delete(source.failures, "versioning")
			delete(destination.failures, "versioning")
			finishMultisiteCleanup(t, collr)
			assert.Empty(t, source.staleKeys)
			assert.Empty(t, destination.staleKeys)
		})
	}
}

func TestMultisiteStateRejectsKeysOutsideConfiguredRoutePrefixes(t *testing.T) {
	collr, _, _ := newMultisiteTestCollector(t)
	createdAt := time.Date(2026, 8, 26, 11, 0, 0, 0, time.UTC)
	fingerprint := collr.ownershipFingerprint()
	ownerTag := collr.stateStore.ownerTag
	sourceKey := multisiteStaleProbeKey(12, ownerTag)
	destinationKey := multisiteStaleDestinationKey(12, ownerTag)

	tests := map[string]struct {
		mutate   func(*ownershipState)
		expected string
	}{
		"wrong source prefix": {
			mutate:   func(state *ownershipState) { state.SourceKey = "outside-route/" + path.Base(sourceKey) },
			expected: "source probe key is outside",
		},
		"wrong destination prefix": {
			mutate:   func(state *ownershipState) { state.DestinationKey = "outside-route/" + path.Base(destinationKey) },
			expected: "destination probe key is outside",
		},
		"valid basename in the wrong source namespace": {
			mutate: func(state *ownershipState) {
				state.SourceKey = defaultPrefix + "nested/" + path.Base(sourceKey)
			},
			expected: "source probe key is outside",
		},
		"valid basename nested below the route prefix": {
			mutate: func(state *ownershipState) {
				state.SourceKey = collr.stateStore.sourceProbePrefix + "nested/" + path.Base(sourceKey)
			},
			expected: "source probe key is outside",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state := &ownershipState{
				Version: ownershipStateVersion, ConfigFingerprint: fingerprint, Phase: string(multisiteCleanup),
				SourceKey: sourceKey, DestinationKey: destinationKey, PayloadDigest: strings.Repeat("0", 64),
				CreatedAt: createdAt,
			}
			test.mutate(state)

			err := collr.stateStore.save(state)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.expected)

			raw, marshalErr := json.Marshal(state)
			require.NoError(t, marshalErr)
			require.NoError(t, os.WriteFile(collr.stateStore.path, raw, 0o600))
			_, err = collr.stateStore.load()
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.expected)
		})
	}
}

func TestCollector_MultisiteCleanupDeletesPendingObjectAndState(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Len(t, source.objects, 1)

	collr.Cleanup(context.Background())
	assert.Empty(t, source.objects)
	assert.Empty(t, destination.objects)
	finishMultisiteCleanup(t, collr)
}

func TestCollector_InitRejectsPendingStateForChangedRoute(t *testing.T) {
	collr, _ := newMultisiteInitCollector(t)
	require.NoError(t, collr.Init(context.Background()))
	state := &ownershipState{
		Phase: string(multisiteReplication), SourceKey: multisiteStaleProbeKey(3, collr.stateStore.ownerTag), DestinationKey: multisiteStaleDestinationKey(3, collr.stateStore.ownerTag), PayloadDigest: strings.Repeat("0", 64),
		CreatedAt: time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
	}
	require.NoError(t, collr.stateStore.save(state))

	changed, _ := newMultisiteInitCollector(t)
	changed.Config = collr.Config
	changed.Config.Destination.Prefix = "changed-route/"
	changed.stateDir = filepath.Dir(filepath.Dir(filepath.Dir(collr.stateStore.path)))

	err := changed.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different source, destination, or mode settings")
	assert.Contains(t, err.Error(), "previous configuration may still own probe objects")
	assert.Contains(t, err.Error(), changed.stateStore.path)
}

func TestCollector_MultisiteSkipsDeleteMeasurementWhenDisabledButCleansDestination(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	collr.VerifyDelete = false

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	sourceToDestination(t, collr, source, destination, false)

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "skipped",
		multisitePhaseLabels(multisiteDeleteWait, reasonDeleteVerificationDisabled),
	)])
	assert.Empty(t, source.objects)
	require.Len(t, destination.objects, 1)

	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, destination.objects)
	finishMultisiteCleanup(t, collr)
}

func TestCollector_MultisiteRemovesDestinationAfterDeleteDeadline(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	collr.DeleteThresholdMS = 1
	collr.DeleteTimeoutMS = 6

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	sourceToDestination(t, collr, source, destination, false)

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Len(t, destination.objects, 1)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "failed", multisitePhaseLabels(multisiteDeleteWait, reasonDeleteTimeout),
	)])
	require.Len(t, destination.objects, 1)

	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, source.objects)
	assert.Empty(t, destination.objects)
	finishMultisiteCleanup(t, collr)
}

func TestCollector_MultisiteCleanupPersistsPhaseBeforeDestructiveShutdown(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Len(t, source.objects, 1)
	destination.failures["delete"] = errors.New("destination cleanup failure")

	collr.Cleanup(context.Background())
	assert.Empty(t, source.objects)
	assert.FileExists(t, collr.stateStore.path)
	state, err := collr.stateStore.load()
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, string(multisiteCleanup), state.Phase)
	assert.Equal(t, reasonShutdownCleanup, state.TerminalReason)
}

func TestMultisiteStateRejectsMalformedTimestampDependencies(t *testing.T) {
	collr, _ := newMultisiteInitCollector(t)
	require.NoError(t, collr.Init(context.Background()))
	createdAt := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	sourcePutAt := createdAt.Add(time.Second)
	visibleAt := sourcePutAt.Add(time.Second)
	tests := map[string]*ownershipState{
		"delete without visibility": {
			Version: ownershipStateVersion, ConfigFingerprint: collr.ownershipFingerprint(), Phase: string(multisiteSourceDelete), SourceKey: multisiteStaleProbeKey(4, collr.stateStore.ownerTag), DestinationKey: multisiteStaleDestinationKey(4, collr.stateStore.ownerTag),
			PayloadDigest: strings.Repeat("0", 64), CreatedAt: createdAt, SourcePutAttempted: true,
			SourcePutAt: &sourcePutAt, SourceDeletedAt: &visibleAt,
		},
		"disappearance without delete": {
			Version: ownershipStateVersion, ConfigFingerprint: collr.ownershipFingerprint(), Phase: string(multisiteCleanup), SourceKey: multisiteStaleProbeKey(5, collr.stateStore.ownerTag), DestinationKey: multisiteStaleDestinationKey(5, collr.stateStore.ownerTag),
			PayloadDigest: strings.Repeat("0", 64), CreatedAt: createdAt, SourcePutAttempted: true,
			SourcePutAt: &sourcePutAt, DestinationVisibleAt: &visibleAt, DestinationGoneAt: &visibleAt,
		},
	}
	for name, state := range tests {
		t.Run(name, func(t *testing.T) {
			state.OwnerTag = collr.stateStore.ownerTag
			state.Mode = modeMultisite
			err := state.validate(
				collr.ownershipFingerprint(), collr.stateStore.ownerTag, modeMultisite,
				collr.stateStore.sourceProbePrefix, collr.stateStore.destinationProbePrefix, time.Now(),
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "without")
		})
	}
}

func TestCollector_MultisiteJobsWithPendingJournalsRecoverConcurrently(t *testing.T) {
	root := t.TempDir()
	jobs := make([]*Collector, 0, 2)
	for _, name := range []string{"forward", "reverse"} {
		collr, _ := newMultisiteInitCollector(t)
		collr.Name = name
		collr.stateDir = root
		require.NoError(t, collr.Init(context.Background()))
		jobs = append(jobs, collr)
	}

	createdAt := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	for _, collr := range jobs {
		state := &ownershipState{
			ConfigFingerprint: collr.ownershipFingerprint(), Phase: string(multisiteCleanup),
			SourceKey:      multisiteStaleProbeKey(22, collr.stateStore.ownerTag),
			DestinationKey: multisiteStaleDestinationKey(22, collr.stateStore.ownerTag),
			PayloadDigest:  strings.Repeat("0", 64), CreatedAt: createdAt,
		}
		require.NoError(t, collr.stateStore.save(state))
	}

	// Reinitialize both collectors after all journals exist. Each finds its own
	// journal and must recover independently of the other pending direction.
	for index, name := range []string{"forward", "reverse"} {
		restarted, _ := newMultisiteInitCollector(t)
		restarted.Name = name
		restarted.stateDir = root
		require.NoErrorf(t, restarted.Init(context.Background()), "job %s", name)
		require.NotNil(t, restarted.stateStore)
		_ = index
	}
}

func TestCollector_MultisiteSourceSiteChangesOwnershipFingerprint(t *testing.T) {
	collr, _, _ := newMultisiteTestCollector(t)
	changed := collr.Config
	changed.SourceSite = "replacement-site"
	changedCollector := &Collector{Config: changed}
	require.NotEqual(t, collr.ownershipFingerprint(), changedCollector.ownershipFingerprint())
}

func TestCollector_MultisiteOwnerNamespacesDoNotCollide(t *testing.T) {
	forward, source, _ := newMultisiteTestCollector(t)
	_, err := collecttest.CollectScalarSeries(forward, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Len(t, source.objects, 1)

	otherPath := filepath.Join(t.TempDir(), "pending.json")
	other, otherSource, _ := newMultisiteTestCollectorAt(t, otherPath)
	other.machineGUID = func() string { return "other-agent-machine-guid" }
	other.stateStore = newOwnershipStateStore(
		otherPath, other.ownershipFingerprint(), multisiteOwnerTag("other-agent-machine-guid", other.Name),
		modeMultisite, other.Prefix, other.Destination.Prefix,
	)
	other.stateLock = filelock.New(filepath.Dir(otherPath))
	other.stateLockName = "s3check"
	other.ownerLockName = other.stateLockName + ".owner"
	require.NotEqual(t, forward.stateStore.ownerTag, other.stateStore.ownerTag)

	_, err = collecttest.CollectScalarSeries(other, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Len(t, otherSource.objects, 1)

	_, err = collecttest.CollectScalarSeries(forward, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Len(t, source.objects, 1)
	assert.Len(t, otherSource.objects, 1)
}

func TestCollector_OtherJobUnresolvedOwnershipBlocksNewCandidate(t *testing.T) {
	incumbent, _ := newMultisiteInitCollector(t)
	require.NoError(t, incumbent.Init(context.Background()))
	createdAt := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	state := &ownershipState{
		ConfigFingerprint: incumbent.ownershipFingerprint(),
		Phase:             string(multisiteCleanup),
		SourceKey:         multisiteStaleProbeKey(20, incumbent.stateStore.ownerTag),
		DestinationKey:    multisiteStaleDestinationKey(20, incumbent.stateStore.ownerTag),
		PayloadDigest:     strings.Repeat("0", 64),
		CreatedAt:         createdAt,
		UpdateEvery:       defaultUpdateEvery,
		HeartbeatAt:       createdAt,
	}
	require.NoError(t, incumbent.stateStore.save(state))

	candidate, _ := newMultisiteInitCollector(t)
	candidate.stateDir = incumbent.stateDir
	candidate.Name = "other-s3check-job"
	require.NoError(t, candidate.Init(context.Background()))

	metrics, err := collecttest.CollectScalarSeries(candidate, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
	require.FileExists(t, incumbent.stateStore.path)
}

func TestCollector_MultisiteCandidateCleanupDoesNotAdoptIncumbentState(t *testing.T) {
	root := t.TempDir()
	statePath := ownershipStatePath(root, "ceph-site-a-to-site-b")
	incumbent, source, destination := newMultisiteTestCollectorAt(t, statePath)
	sourceKey := multisiteStaleProbeKey(19, incumbent.stateStore.ownerTag)
	destinationKey := multisiteStaleDestinationKey(19, incumbent.stateStore.ownerTag)
	state := &ownershipState{
		Phase: string(multisiteSourcePut), SourceKey: sourceKey, DestinationKey: destinationKey,
		PayloadDigest: strings.Repeat("0", 64), CreatedAt: incumbent.now(), SourcePutAttempted: true,
	}
	require.NoError(t, incumbent.saveOwnership(state))
	source.objects[sourceKey] = []byte("payload")
	destination.objects[destinationKey] = []byte("payload")

	candidate, candidateSource, candidateDestination := newMultisiteTestCollectorAt(t, statePath)
	candidate.newClient = func(Config) (*http.Client, s3Client, error) {
		return &http.Client{}, candidateSource, nil
	}
	candidate.newDestinationClient = func(DestinationConfig, int) (*http.Client, s3Client, error) {
		return &http.Client{}, candidateDestination, nil
	}
	candidate.now = incumbent.now
	candidate.stateDir = root
	require.NoError(t, candidate.Init(context.Background()))
	require.NoError(t, candidate.Check(context.Background()))
	candidate.Cleanup(context.Background())

	assert.Empty(t, candidateSource.operations())
	assert.Empty(t, candidateDestination.operations())
	assert.Contains(t, source.objects, sourceKey)
	assert.Contains(t, destination.objects, destinationKey)
	require.FileExists(t, candidate.stateStore.path)
	loaded, err := candidate.stateStore.load()
	require.NoError(t, err)
	assert.Equal(t, sourceKey, loaded.SourceKey)
	assert.Equal(t, destinationKey, loaded.DestinationKey)
}

func TestCollector_ForeignOwnershipJournalLifecycleClassification(t *testing.T) {
	writeForeignState := func(t *testing.T, root string, heartbeat time.Time, retired bool) (*Collector, string) {
		t.Helper()
		statePath := ownershipStatePath(root, "incumbent-s3check-job")
		incumbent, _ := newTestCollectorAt(t, statePath)
		configureSingleTestIdentity(incumbent, "incumbent-s3check-job", statePath)
		now := time.Now().UTC()
		incumbent.now = func() time.Time { return now }
		incumbent.stateStore.now = incumbent.now
		state := &ownershipState{
			Phase: string(multisiteSourcePut), SourceKey: staleProbeKey(18, incumbent.stateStore.ownerTag),
			CreatedAt: now, UpdateEvery: defaultUpdateEvery, HeartbeatAt: heartbeat,
			SourcePutAttempted: true,
		}
		if retired {
			retiredAt := now
			state.RetiredAt = &retiredAt
		}
		require.NoError(t, incumbent.stateStore.save(state))
		require.NoError(t, incumbent.reserveOwnerLock(context.Background()))
		return incumbent, statePath
	}

	t.Run("a fresh live foreign journal does not block a differently named job", func(t *testing.T) {
		root := t.TempDir()
		incumbent, _ := writeForeignState(t, root, time.Now().UTC(), false)
		defer incumbent.releaseOwnerLock()

		candidateStatePath := ownershipStatePath(root, "other-s3check-job")
		candidate, candidateClient := newTestCollectorAt(t, candidateStatePath)
		configureSingleTestIdentity(candidate, "other-s3check-job", candidateStatePath)
		candidate.newClient = func(Config) (*http.Client, s3Client, error) {
			return &http.Client{}, candidateClient, nil
		}
		candidate.stateDir = root
		require.NoError(t, candidate.Init(context.Background()))
		_, err := collecttest.CollectScalarSeries(candidate, metrix.ReadFlatten())
		require.NoError(t, err)
		require.FileExists(t, candidate.stateStore.path)
	})

	t.Run("a fresh journal with no live owner blocks new writes and retries later", func(t *testing.T) {
		root := t.TempDir()
		incumbent, incumbentStatePath := writeForeignState(t, root, time.Now().UTC(), false)

		candidateStatePath := ownershipStatePath(root, "other-s3check-job")
		candidate, candidateClient := newTestCollectorAt(t, candidateStatePath)
		configureSingleTestIdentity(candidate, "other-s3check-job", candidateStatePath)
		candidate.newClient = func(Config) (*http.Client, s3Client, error) {
			return &http.Client{}, candidateClient, nil
		}
		candidate.stateDir = root
		require.NoError(t, candidate.Init(context.Background()))

		incumbent.releaseOwnerLock()
		metrics, err := collecttest.CollectScalarSeries(candidate, metrix.ReadFlatten())
		require.NoError(t, err)
		assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
			"stage_status", "failed", stageLabels(stageSetup, reasonOrphanCleanupPending),
		)])
		assert.Empty(t, candidateClient.operations())
		require.NoFileExists(t, candidate.stateStore.path)

		require.NoError(t, os.Remove(incumbentStatePath))
		_, err = collecttest.CollectScalarSeries(candidate, metrix.ReadFlatten())
		require.NoError(t, err)
		require.FileExists(t, candidate.stateStore.path)
	})

	t.Run("a stale foreign journal blocks new writes", func(t *testing.T) {
		root := t.TempDir()
		incumbent, _ := writeForeignState(t, root, staleHeartbeat(), false)
		defer incumbent.releaseOwnerLock()

		candidateStatePath := ownershipStatePath(root, "other-s3check-job")
		candidate, candidateClient := newTestCollectorAt(t, candidateStatePath)
		configureSingleTestIdentity(candidate, "other-s3check-job", candidateStatePath)
		candidate.newClient = func(Config) (*http.Client, s3Client, error) {
			return &http.Client{}, candidateClient, nil
		}
		candidate.stateDir = root
		require.NoError(t, candidate.Init(context.Background()))

		incumbent.releaseOwnerLock()
		metrics, err := collecttest.CollectScalarSeries(candidate, metrix.ReadFlatten())
		require.NoError(t, err)
		assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
			"stage_status", "failed", stageLabels(stageSetup, reasonOrphanCleanupPending),
		)])
		assert.Empty(t, candidateClient.operations())
	})

	t.Run("a retired foreign journal blocks new writes", func(t *testing.T) {
		root := t.TempDir()
		incumbent, _ := writeForeignState(t, root, time.Now().UTC(), true)
		defer incumbent.releaseOwnerLock()

		candidateStatePath := ownershipStatePath(root, "other-s3check-job")
		candidate, candidateClient := newTestCollectorAt(t, candidateStatePath)
		configureSingleTestIdentity(candidate, "other-s3check-job", candidateStatePath)
		candidate.newClient = func(Config) (*http.Client, s3Client, error) {
			return &http.Client{}, candidateClient, nil
		}
		candidate.stateDir = root
		require.NoError(t, candidate.Init(context.Background()))

		metrics, err := collecttest.CollectScalarSeries(candidate, metrix.ReadFlatten())
		require.NoError(t, err)
		assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
			"stage_status", "failed", stageLabels(stageSetup, reasonOrphanCleanupPending),
		)])
		assert.Empty(t, candidateClient.operations())
	})
}

func staleHeartbeat() time.Time {
	return time.Now().UTC().Add(-(3*defaultUpdateEvery)*time.Second - cycleProcessingMargin - time.Second)
}

func TestUnresolvedOwnershipFileRejectsMalformedFreshJournal(t *testing.T) {
	now := time.Now().UTC()
	validTag := multisiteOwnerTag("test-machine-guid", "incumbent-s3check-job")
	tests := map[string]struct {
		mode      string
		sourceKey string
	}{
		"invalid active key": {modeSingle, "not-a-probe-key"},
		"invalid mode":       {"unknown-mode", staleProbeKey(16, validTag)},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state := &ownershipState{
				Version: ownershipStateVersion, ConfigFingerprint: strings.Repeat("0", 64), OwnerTag: validTag,
				Mode: test.mode, Phase: string(multisiteSourcePut), SourceKey: test.sourceKey, CreatedAt: now,
				UpdateEvery: defaultUpdateEvery, HeartbeatAt: now, SourcePutAttempted: true,
			}
			raw, err := json.Marshal(state)
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "foreign.json")
			require.NoError(t, os.WriteFile(path, raw, 0o600))

			unresolved, err := unresolvedOwnershipFile(path, now)
			require.Error(t, err)
			assert.True(t, unresolved)
			assert.Contains(t, err.Error(), "validate another s3check ownership state")
		})
	}
}

func TestUnresolvedOwnershipFileBlocksFreshCrashedOwner(t *testing.T) {
	now := time.Now().UTC()
	tag := multisiteOwnerTag("test-machine-guid", "crashed-s3check-job")
	state := &ownershipState{
		Version: ownershipStateVersion, ConfigFingerprint: strings.Repeat("0", 64), OwnerTag: tag,
		Mode: modeSingle, Phase: string(multisiteSourcePut), SourceKey: staleProbeKey(15, tag),
		CreatedAt: now, UpdateEvery: defaultUpdateEvery, HeartbeatAt: now, SourcePutAttempted: true,
	}
	raw, err := json.Marshal(state)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "crashed.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))

	unresolved, err := unresolvedOwnershipFile(path, now)
	require.NoError(t, err)
	assert.True(t, unresolved)
}

func TestCollector_CleanupSkipsDestructiveWorkWhenRetirementCannotPersist(t *testing.T) {
	makeStateStoreUnwritable := func(t *testing.T, collr *Collector) {
		t.Helper()
		blocker := filepath.Join(t.TempDir(), "not-a-directory")
		require.NoError(t, os.WriteFile(blocker, []byte("blocker"), 0o600))
		collr.stateStore.path = filepath.Join(blocker, "pending.json")
	}

	t.Run("single-site", func(t *testing.T) {
		collr, client := newTestCollector(t)
		state := &ownershipState{
			Phase: string(multisiteSourcePut), SourceKey: staleProbeKey(14), CreatedAt: collr.now(),
			HeartbeatAt: collr.now(), SourcePutAttempted: true,
		}
		collr.pendingOwnershipState = state
		client.objects[state.SourceKey] = []byte("payload")
		makeStateStoreUnwritable(t, collr)

		require.Error(t, collr.markOwnershipRetired())
		require.Nil(t, state.RetiredAt)
		collr.Cleanup(context.Background())
		assert.Empty(t, client.operations())
		assert.Contains(t, client.objects, state.SourceKey)
		assert.Nil(t, state.RetiredAt)
	})

	t.Run("multisite", func(t *testing.T) {
		collr, source, destination := newMultisiteTestCollector(t)
		sourceKey := multisiteStaleProbeKey(13, collr.stateStore.ownerTag)
		destinationKey := multisiteStaleDestinationKey(13, collr.stateStore.ownerTag)
		state := &ownershipState{
			Phase: string(multisiteSourcePut), SourceKey: sourceKey, DestinationKey: destinationKey,
			PayloadDigest: strings.Repeat("0", 64), CreatedAt: collr.now(), HeartbeatAt: collr.now(),
			SourcePutAttempted: true,
		}
		collr.pendingOwnershipState = state
		source.objects[sourceKey] = []byte("payload")
		destination.objects[destinationKey] = []byte("payload")
		makeStateStoreUnwritable(t, collr)

		collr.Cleanup(context.Background())
		assert.Empty(t, source.operations())
		assert.Empty(t, destination.operations())
		assert.Contains(t, source.objects, sourceKey)
		assert.Contains(t, destination.objects, destinationKey)
		assert.Nil(t, state.RetiredAt)
	})
}

func TestCollector_MultisiteShutdownDefersPendingKeyCleanup(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	state := &ownershipState{
		Phase: string(multisiteCleanup), TerminalReason: reasonOrphanCleanupPending, CreatedAt: collr.now(),
		PendingKeys: []ownedKey{
			{Scope: ownershipSource, Key: multisiteStaleProbeKey(11, collr.stateStore.ownerTag)},
			{Scope: ownershipDestination, Key: multisiteStaleDestinationKey(12, collr.stateStore.ownerTag)},
		},
	}
	collr.pendingOwnershipState = state
	source.staleKeys[state.PendingKeys[0].Key] = true
	destination.staleKeys[state.PendingKeys[1].Key] = true

	collr.Cleanup(context.Background())
	assert.Equal(t, []string{"versioning"}, source.operations())
	assert.Equal(t, []string{"versioning"}, destination.operations())
	assert.NotContains(t, source.operations(), "delete")
	assert.NotContains(t, destination.operations(), "delete")
	assert.Len(t, source.staleKeys, 1)
	assert.Len(t, destination.staleKeys, 1)
	require.FileExists(t, collr.stateStore.path)
	loaded, err := collr.stateStore.load()
	require.NoError(t, err)
	require.NotNil(t, loaded.RetiredAt)
	require.Len(t, loaded.PendingKeys, 2)
}

func TestCollector_MultisiteReconciliationOwnershipSurvivesRouteChange(t *testing.T) {
	collr, source, _ := newMultisiteTestCollector(t)
	source.staleKeys[multisiteStaleProbeKey(21, collr.stateStore.ownerTag)] = true
	source.versioningStatus = "Enabled"

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.FileExists(t, collr.stateStore.path)

	changed := collr.Config
	changed.Destination = &DestinationConfig{}
	*changed.Destination = *collr.Destination
	changed.Destination.Prefix = "changed-route/"
	changedCandidate := &Collector{Config: changed, machineGUID: func() string { return "test-machine-guid" }}
	changedStore := newOwnershipStateStore(
		collr.stateStore.path, changedCandidate.ownershipFingerprint(), collr.stateStore.ownerTag, modeMultisite,
		changed.Prefix, changed.Destination.Prefix,
	)
	_, err = changedStore.load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pending state belongs")
	require.FileExists(t, collr.stateStore.path)
	assert.Len(t, source.staleKeys, 1)
}

func TestCollector_MultisiteReverseDirectionDoesNotReconcileActiveRoute(t *testing.T) {
	forward, source, _ := newMultisiteTestCollector(t)
	_, err := collecttest.CollectScalarSeries(forward, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Len(t, source.objects, 1)

	reverseConfig := validMultisiteTestConfig()
	reverseConfig.Name = "ceph-site-b-to-site-a"
	reverseConfig.SourceSite = "site-b"
	reverseConfig.Endpoint = forward.Destination.Endpoint
	reverseConfig.Bucket = forward.Destination.Bucket
	reverseConfig.Prefix = forward.Destination.Prefix
	reverseConfig.Destination = &DestinationConfig{
		Site: "site-a", Endpoint: forward.Endpoint, Region: forward.Region,
		Bucket: forward.Bucket, Prefix: forward.Prefix,
		AccessKeyID: "test-access-key-id", SecretAccessKey: "test-secret-access-key",
		Timeout: confopt.Duration(defaultTimeout),
	}
	reverseSource := &fakeS3Client{
		objects: make(map[string][]byte), staleKeys: make(map[string]bool),
		failures: make(map[string]error), attempts: make(map[string]int),
	}
	reverse := New()
	reverse.Config = reverseConfig
	reverse.client = reverseSource
	reverse.destinationClient = source
	reverseStatePath := filepath.Join(t.TempDir(), "pending.json")
	reverse.machineGUID = func() string { return "test-machine-guid" }
	reverse.stateStore = newOwnershipStateStore(
		reverseStatePath, reverse.ownershipFingerprint(),
		multisiteOwnerTag("test-machine-guid", reverse.Name), modeMultisite, reverse.Prefix, reverse.Destination.Prefix,
	)
	reverse.stateLock = filelock.New(filepath.Dir(reverseStatePath))
	reverse.stateLockName = strings.TrimSuffix(filepath.Base(reverseStatePath), filepath.Ext(reverseStatePath))
	reverse.now = forward.now
	reverse.randomRead = forward.randomRead

	_, err = collecttest.CollectScalarSeries(reverse, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Len(t, source.objects, 1)
	assert.Len(t, reverseSource.objects, 1)
}

func TestCollector_MultisiteDoesNotTurnDestinationRequestFailureIntoRPO(t *testing.T) {
	collr, _, destination := newMultisiteTestCollector(t)
	collr.RPOThresholdMS = 1
	collr.ReplicationTimeoutMS = 7200000
	destination.failures["get"] = errors.New("secret-endpoint-detail")

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "failed", multisitePhaseLabels(multisiteReplication, reasonRequestFailed),
	)])
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
	assert.NotContains(t, metrics, metricKey("multisite_rpo_exceeded", multisiteSiteLabels()))
}

func TestCollector_MultisiteDoesNotTurnDestinationHeadFailureIntoDeleteObjective(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	collr.DeleteThresholdMS = 1

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	sourceToDestination(t, collr, source, destination, false)
	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Len(t, destination.objects, 1)

	destination.failures["head"] = errors.New("secret-endpoint-detail")
	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "failed", multisitePhaseLabels(multisiteDeleteWait, reasonRequestFailed),
	)])
	assert.NotContains(t, metrics, metricKey("multisite_delete_exceeded", multisiteSiteLabels()))
}

func TestCollector_MultisiteUsesPersistedDeleteAttemptAcrossRestart(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	collr.DeleteThresholdMS = 90000
	collr.DeleteTimeoutMS = 7200000

	createdAt := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	sourcePutAt := createdAt
	visibleAt := createdAt.Add(time.Second)
	deleteAttemptAt := visibleAt.Add(10 * time.Minute)
	key := multisiteStaleProbeKey(8, collr.stateStore.ownerTag)
	destinationKey := multisiteStaleDestinationKey(8, collr.stateStore.ownerTag)
	state := &ownershipState{
		Version: ownershipStateVersion, ConfigFingerprint: collr.ownershipFingerprint(),
		Phase: string(multisiteSourceDelete), SourceKey: key, DestinationKey: destinationKey,
		PayloadDigest: strings.Repeat("0", 64), CreatedAt: createdAt, SourcePutAttempted: true,
		SourcePutAt: &sourcePutAt, DestinationVisibleAt: &visibleAt, SourceDeleteAttemptedAt: &deleteAttemptAt,
	}
	destination.objects[destinationKey] = []byte("payload")
	clock := &steppingClock{now: deleteAttemptAt.Add(2 * time.Minute), step: time.Millisecond}
	collr.now = clock.tick
	collr.stateStore.now = collr.now
	require.NoError(t, collr.stateStore.save(state))
	collr.pendingOwnershipState = state

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_delete_exceeded", multisiteSiteLabels())])
	assert.Empty(t, source.objects)
	require.Len(t, destination.objects, 1)

	delete(destination.objects, destinationKey)
	metrics, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Contains(t, metrics, metricKey("multisite_delete_lag_ms", multisiteSiteLabels()))
	assert.GreaterOrEqual(t, float64(metrics[metricKey("multisite_delete_lag_ms", multisiteSiteLabels())]), 90000.0)
}

func TestCollector_MultisiteEnforcesReplicationDeadlineAfterRequest(t *testing.T) {
	collr, source, destination := newMultisiteTestCollector(t)
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	sourceToDestination(t, collr, source, destination, false)

	now := time.Date(2026, 8, 26, 13, 0, 0, 0, time.UTC)
	clock := &steppingClock{now: now, step: 2 * time.Millisecond}
	collr.now = clock.tick
	collr.stateStore.now = collr.now
	past := now.Add(-2 * time.Millisecond)
	collr.pendingOwnershipState.SourcePutAt = &past
	collr.ReplicationTimeoutMS = 5
	require.NoError(t, collr.stateStore.save(collr.pendingOwnershipState))

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "failed", multisitePhaseLabels(multisiteReplication, reasonReplicationTimeout),
	)])
	assert.NotContains(t, metrics, metricKey("multisite_replication_lag_ms", multisiteSiteLabels()))
}

func TestCollector_MultisiteEnforcesDeleteDeadlineAfterRequest(t *testing.T) {
	collr, _, destination := newMultisiteTestCollector(t)
	now := time.Date(2026, 8, 26, 14, 0, 0, 0, time.UTC)
	sourcePutAt := now.Add(-10 * time.Millisecond)
	visibleAt := now.Add(-6 * time.Millisecond)
	deleteStartedAt := now.Add(-3 * time.Millisecond)
	key := multisiteStaleProbeKey(9, collr.stateStore.ownerTag)
	destinationKey := multisiteStaleDestinationKey(9, collr.stateStore.ownerTag)
	state := &ownershipState{
		Version: ownershipStateVersion, ConfigFingerprint: collr.ownershipFingerprint(),
		Phase: string(multisiteDeleteWait), SourceKey: key, DestinationKey: destinationKey,
		PayloadDigest: strings.Repeat("0", 64), CreatedAt: sourcePutAt, SourcePutAttempted: true,
		SourcePutAt: &sourcePutAt, DestinationVisibleAt: &visibleAt,
		SourceDeleteAttemptedAt: &deleteStartedAt, SourceDeletedAt: &deleteStartedAt,
	}
	destination.objects[destinationKey] = []byte("payload")
	clock := &steppingClock{now: now, step: 2 * time.Millisecond}
	collr.now = clock.tick
	collr.stateStore.now = collr.now
	require.NoError(t, collr.stateStore.save(state))
	collr.pendingOwnershipState = state
	collr.DeleteTimeoutMS = 5

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"multisite_status", "failed", multisitePhaseLabels(multisiteDeleteWait, reasonDeleteTimeout),
	)])
	assert.NotContains(t, metrics, metricKey("multisite_delete_lag_ms", multisiteSiteLabels()))
}

func TestCollector_SingleModeRejectsPendingMultisiteState(t *testing.T) {
	multisite, _ := newMultisiteInitCollector(t)
	require.NoError(t, multisite.Init(context.Background()))
	createdAt := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	state := &ownershipState{
		Phase: string(multisiteCleanup), SourceKey: multisiteStaleProbeKey(10, multisite.stateStore.ownerTag),
		DestinationKey: multisiteStaleDestinationKey(10, multisite.stateStore.ownerTag),
		PayloadDigest:  strings.Repeat("0", 64), CreatedAt: createdAt,
	}
	require.NoError(t, multisite.stateStore.save(state))

	single := New()
	single.Config = validTestConfig()
	single.machineGUID = func() string { return "test-machine-guid" }
	single.stateDir = t.TempDir()
	single.Name = multisite.Name
	single.stateDir = filepath.Dir(filepath.Dir(filepath.Dir(multisite.stateStore.path)))
	single.newClient = func(Config) (*http.Client, s3Client, error) { return &http.Client{}, &fakeS3Client{}, nil }

	err := single.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pending state belongs to different source, destination, or mode settings")
}
