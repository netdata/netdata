// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/filelock"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("s3check", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery:        defaultUpdateEvery,
			AutoDetectionRetry: 0,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			UpdateEvery:          defaultUpdateEvery,
			Prefix:               defaultPrefix,
			PathStyle:            true,
			MaxRetries:           defaultMaxRetries,
			LatencyThresholdMS:   0,
			Mode:                 modeSingle,
			RPOThresholdMS:       defaultRPOThresholdMS,
			ReplicationTimeoutMS: defaultReplicationTimeoutMS,
			DeleteThresholdMS:    defaultDeleteThresholdMS,
			DeleteTimeoutMS:      defaultDeleteTimeoutMS,
			VerifyDelete:         true,
			ClientConfig: web.ClientConfig{
				Timeout:           confopt.Duration(defaultTimeout),
				NotFollowRedirect: true,
			},
		},
		store:          metrix.NewCollectorStore(),
		stats:          newStageStats(),
		multisiteStats: newMultisiteStats(),
		now:            time.Now,
		randomRead:     readRandomBytes,
		machineGUID:    readAgentMachineGUID,
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store metrix.CollectorStore

	httpClient            *http.Client
	client                s3Client
	newClient             func(Config) (*http.Client, s3Client, error)
	destinationHTTPClient *http.Client
	destinationClient     s3Client
	newDestinationClient  func(DestinationConfig, int) (*http.Client, s3Client, error)

	stats          stageStats
	multisiteStats multisiteStats

	stateStore            *ownershipStateStore
	pendingOwnershipState *ownershipState
	stateDir              string
	stateLock             *filelock.Locker
	stateLockName         string
	stateLockHeld         bool
	ownerLockName         string
	ownerLockHeld         bool
	handoffLockHeld       bool
	probePayload          []byte

	machineGUID func() string
	now         func() time.Time
	randomRead  func(int) ([]byte, error)
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}
	if c.Name == "" {
		c.Name = "s3check"
	}
	if c.stateDir == "" {
		c.stateDir = buildinfo.VarLibDir
	}
	statePath := ownershipStatePath(c.stateDir, c.Name)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		return fmt.Errorf("create s3check ownership state directory: %w", err)
	}
	machineGUID := c.machineGUID()
	if machineGUID == "" {
		return errors.New("Agent machine GUID is unavailable")
	}
	c.stateLock = filelock.New(filepath.Dir(statePath))
	c.stateLockName = strings.TrimSuffix(filepath.Base(statePath), filepath.Ext(statePath))
	c.ownerLockName = c.stateLockName + ".owner"
	c.stateStore = newOwnershipStateStore(
		statePath,
		c.ownershipFingerprint(),
		multisiteOwnerTag(machineGUID, c.Name),
		c.Mode,
		c.Prefix,
		c.destinationStatePrefix(),
	)
	c.stateStore.now = c.now
	_, stateErr := c.stateStore.load()
	if stateErr != nil {
		return fmt.Errorf("load s3check ownership state %q: %w", c.stateStore.path, stateErr)
	}
	if c.newClient == nil {
		c.newClient = newAWSS3Client
	}
	if c.newDestinationClient == nil {
		c.newDestinationClient = newAWSS3ClientFromDestination
	}

	httpClient, client, err := c.newClient(c.Config)
	if err != nil {
		return fmt.Errorf("create source S3 client: %w", err)
	}
	c.httpClient = httpClient
	c.client = client
	c.stats = newStageStats()
	c.multisiteStats = newMultisiteStats()

	if c.Mode == modeMultisite {
		destinationHTTPClient, destinationClient, destErr := c.newDestinationClient(*c.Destination, c.MaxRetries)
		if destErr != nil {
			c.httpClient.CloseIdleConnections()
			return fmt.Errorf("create destination S3 client: %w", destErr)
		}
		c.destinationHTTPClient = destinationHTTPClient
		c.destinationClient = destinationClient
	}

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if c.client == nil {
		return errors.New("S3 client is not initialized")
	}
	state, stateErr := c.stateStore.load()
	if stateErr != nil {
		return fmt.Errorf("load s3check ownership state: %w", stateErr)
	}
	// Pending ownership recovery reports failures from Collect; transient
	// startup requests must not abandon unresolved objects. The execution
	// reservation serializes this identity through promotion or rejection.
	if state != nil {
		if err := c.reserveJobState(ctx); err != nil {
			return err
		}
		return nil
	}
	if err := checkUnversionedBucket(ctx, c.client, c.Bucket, "source"); err != nil {
		return err
	}
	if c.Mode == modeMultisite {
		if err := checkUnversionedBucket(ctx, c.destinationClient, c.Destination.Bucket, "destination"); err != nil {
			return err
		}
	}
	// The execution reservation survives until the promoted collector's first
	// Collect. Owner liveness is reserved separately in Collect, after the job
	// manager has quiesced an incumbent, so same-name replacement cannot deadlock.
	if err := c.reserveJobState(ctx); err != nil {
		return err
	}
	if err := c.reserveOwnershipHandoff(); err != nil {
		c.releaseJobState()
		return err
	}
	defer c.releaseOwnershipHandoff()
	state, stateErr = c.stateStore.load()
	if stateErr != nil {
		c.releaseJobState()
		return fmt.Errorf("load s3check ownership state: %w", stateErr)
	}
	if state != nil {
		c.releaseJobState()
		return errors.New("s3check ownership state appeared concurrently; retry configuration")
	}
	return nil
}

func (c *Collector) destinationStatePrefix() string {
	if c.Mode != modeMultisite || c.Destination == nil {
		return ""
	}
	return c.Destination.Prefix
}

func readAgentMachineGUID() string {
	if guid := strings.TrimSpace(os.Getenv("NETDATA_REGISTRY_UNIQUE_ID")); guid != "" {
		return guid
	}
	return strings.TrimSpace(pluginconfig.RegistryUniqueID())
}

func (c *Collector) reserveJobState(ctx context.Context) error {
	if c.stateLock == nil {
		return errors.New("s3check job lock is not initialized")
	}

	// A same-identity replacement can Check while the incumbent is still in
	// Collect. Wait for that bounded cycle instead of rejecting the handoff; the
	// Agent-wide handoff lock is deliberately not held while waiting.
	waitCtx, cancel := context.WithTimeout(ctx, c.worstCaseDuration())
	defer cancel()
	for {
		held, err := c.tryReserveJobState()
		if err != nil {
			return err
		}
		if held {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("lock s3check job state: %w", waitCtx.Err())
		case <-time.After(jobLockPollInterval):
		}
	}
}

func (c *Collector) reserveRuntimeJobState() error {
	// A promoted runtime reuses its Check reservation for the first collection.
	// If a rejected same-identity candidate holds it instead, fail immediately;
	// that candidate releases the reservation in Cleanup.
	if c.stateLockHeld {
		return nil
	}
	held, err := c.tryReserveJobState()
	if err != nil || held {
		return err
	}
	return errors.New("s3check job state is changing concurrently")
}

func (c *Collector) tryReserveJobState() (bool, error) {
	locked, err := c.stateLock.Lock(c.stateLockName)
	if err != nil {
		return false, fmt.Errorf("lock s3check job state: %w", err)
	}
	if !locked {
		return false, nil
	}
	c.stateLockHeld = true
	return true, nil
}

func (c *Collector) reserveOwnershipHandoff() error {
	if c.stateLock == nil {
		return errors.New("s3check handoff lock is not initialized")
	}
	locked, err := c.stateLock.Lock("s3check-handoff")
	if err != nil {
		return fmt.Errorf("lock s3check ownership handoff: %w", err)
	}
	if !locked {
		return errors.New("s3check ownership is changing concurrently")
	}
	c.handoffLockHeld = true
	return nil
}

func (c *Collector) releaseJobState() {
	if c.stateLock == nil || !c.stateLockHeld {
		return
	}
	c.stateLock.Unlock(c.stateLockName)
	c.stateLockHeld = false
}

func (c *Collector) reserveOwnerLock(ctx context.Context) error {
	if c.stateLock == nil {
		return errors.New("s3check owner lock is not initialized")
	}
	waitCtx, cancel := context.WithTimeout(ctx, c.worstCaseDuration())
	defer cancel()
	for {
		locked, err := c.stateLock.Lock(c.ownerLockName)
		if err != nil {
			return fmt.Errorf("lock s3check ownership owner: %w", err)
		}
		if locked {
			c.ownerLockHeld = true
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("lock s3check ownership owner: %w", waitCtx.Err())
		case <-time.After(jobLockPollInterval):
		}
	}
}

func (c *Collector) releaseOwnerLock() {
	if c.stateLock == nil || !c.ownerLockHeld {
		return
	}
	c.stateLock.Unlock(c.ownerLockName)
	c.ownerLockHeld = false
}

func (c *Collector) loadOwnershipForCycle() error {
	if c.pendingOwnershipState != nil {
		return nil
	}
	state, err := c.stateStore.load()
	if err != nil {
		return fmt.Errorf("load s3check ownership state: %w", err)
	}
	c.pendingOwnershipState = state
	return nil
}

func (c *Collector) saveOwnership(state *ownershipState) error {
	if state == nil {
		return errors.New("save nil ownership state")
	}
	state.UpdateEvery = c.UpdateEvery
	now := c.now()
	if state.HeartbeatAt.IsZero() || now.After(state.HeartbeatAt) {
		state.HeartbeatAt = now
	}
	return c.stateStore.save(state)
}

func (c *Collector) touchOwnership() error {
	if c.pendingOwnershipState == nil {
		return nil
	}
	return c.saveOwnership(c.pendingOwnershipState)
}

var errUnresolvedForeignOwnership = errors.New("another s3check job has unresolved object ownership")

func (c *Collector) reserveOwnershipPublication() error {
	if err := c.reserveOwnershipHandoff(); err != nil {
		return err
	}
	unresolved, err := unresolvedOwnershipStateExists(
		filepath.Dir(c.stateStore.path), c.stateStore.path,
	)
	if err == nil && !unresolved {
		return nil
	}
	c.releaseOwnershipHandoff()
	if err != nil {
		return err
	}
	return errUnresolvedForeignOwnership
}

func publicationFailureReason(err error) string {
	if errors.Is(err, errUnresolvedForeignOwnership) {
		return reasonOrphanCleanupPending
	}
	return reasonInternal
}

func (c *Collector) releaseOwnershipHandoff() {
	if c.stateLock == nil || !c.handoffLockHeld {
		return
	}
	c.stateLock.Unlock("s3check-handoff")
	c.handoffLockHeld = false
}

func checkUnversionedBucket(ctx context.Context, client s3Client, bucket, label string) error {
	status, _, err := client.GetBucketVersioning(ctx, bucket)
	if err != nil {
		return fmt.Errorf(
			"%s S3 bucket versioning check failed; verify the endpoint, credentials, bucket, and s3:GetBucketVersioning permission",
			label,
		)
	}
	if status != "" {
		return fmt.Errorf("unsupported %s S3 bucket versioning status %q; use a dedicated unversioned bucket", label, status)
	}
	return nil
}

func (c *Collector) writeSetupFailure(reason string, cause error) {
	if cause != nil {
		c.Warningf("s3check setup failed: %v", cause)
	}
	if c.Mode == modeMultisite {
		cycle := newMultisiteCycle()
		cycle.phases[multisiteSetup].fail(reason)
		c.writeMultisiteMetrics(cycle)
		return
	}
	results := newStageResults()
	results[stageSetup].fail(reason)
	c.writeMetrics(results)
}

func (c *Collector) Collect(ctx context.Context) error {
	if c.client == nil {
		return errors.New("S3 client is not initialized")
	}

	cycleCtx, cancel := context.WithTimeout(ctx, c.worstCaseDuration())
	defer cancel()

	// The per-job reservation from Check is idempotent and remains held for
	// this invocation; unrelated jobs use their own locks.
	if err := c.reserveRuntimeJobState(); err != nil {
		c.writeSetupFailure(reasonInternal, err)
		return ctx.Err()
	}
	if err := c.reserveOwnerLock(cycleCtx); err != nil {
		c.writeSetupFailure(reasonInternal, err)
		return ctx.Err()
	}
	defer c.releaseJobState()

	if err := c.loadOwnershipForCycle(); err != nil {
		c.writeSetupFailure(reasonInternal, err)
		return ctx.Err()
	}
	if c.pendingOwnershipState == nil {
		unresolved, unresolvedErr := unresolvedOwnershipStateExists(
			filepath.Dir(c.stateStore.path), c.stateStore.path,
		)
		if unresolvedErr != nil || unresolved {
			if unresolvedErr == nil {
				unresolvedErr = errUnresolvedForeignOwnership
			}
			c.writeSetupFailure(reasonOrphanCleanupPending, unresolvedErr)
			return ctx.Err()
		}
	}

	if c.Mode == modeMultisite {
		cycle := newMultisiteCycle()
		c.collectMultisite(cycleCtx, cycle)
		c.writeMultisiteMetrics(cycle)
		return ctx.Err()
	}

	results := newStageResults()
	c.collect(cycleCtx, results)
	c.writeMetrics(results)

	// A collector-local deadline is represented as a stage timeout. Only cancellation of
	// the runtime-provided context aborts the metric cycle.
	return ctx.Err()
}

func (c *Collector) Cleanup(_ context.Context) {
	if c.Mode == modeMultisite {
		if c.pendingOwnershipState != nil && c.stateStore != nil {
			// Runtime cancellation may already have propagated; cleanup still gets its own
			// bounded deadline so a reachable write path is not abandoned merely by shutdown.
			if err := c.markOwnershipRetired(); err == nil {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), c.shutdownCleanupDuration())
				_ = c.cleanupPendingOwnershipObject(cleanupCtx)
				cancel()
			}
		}
	} else if c.pendingOwnershipState != nil && c.stateStore != nil {
		retireErr := c.markOwnershipRetired()
		// Shutdown cleans the active exact key only after retirement is durable.
		// Aged reconciliation batches remain on disk for a matching job to recover.
		if retireErr == nil && c.pendingOwnershipState.SourceKey != "" {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), c.shutdownCleanupDuration())
			results := newStageResults()
			_ = c.cleanupOwnedSingleState(cleanupCtx, results)
			cancel()
		}
	}

	// Only the collector instance that loaded ownership during Collect may clean
	// it. A configuration Test or rejected candidate remains read-only.
	c.releaseJobState()
	c.releaseOwnerLock()
	c.releaseOwnershipHandoff()

	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	if c.destinationHTTPClient != nil {
		c.destinationHTTPClient.CloseIdleConnections()
	}
}

func (c *Collector) markOwnershipRetired() error {
	state := c.pendingOwnershipState
	if state == nil || state.RetiredAt != nil {
		return nil
	}
	now := c.now()
	state.RetiredAt = &now
	if err := c.saveOwnership(state); err != nil {
		state.RetiredAt = nil
		return fmt.Errorf("retire s3check ownership state: %w", err)
	}
	return nil
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
