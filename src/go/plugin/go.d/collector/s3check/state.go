// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/filelock"
)

const (
	ownershipStateVersion  = 2
	maxOwnershipStateBytes = 32 * 1024
	maxOwnedKeys           = 32
	maxOwnershipStateFiles = 256
)

type ownershipScope string

const (
	ownershipSource      ownershipScope = "source"
	ownershipSingle      ownershipScope = "single"
	ownershipDestination ownershipScope = "destination"
)

type multisitePhase string

const (
	multisiteSetup        multisitePhase = "setup"
	multisiteSourcePut    multisitePhase = "source_put"
	multisiteReplication  multisitePhase = "replication_wait"
	multisiteSourceDelete multisitePhase = "source_delete"
	multisiteDeleteWait   multisitePhase = "delete_wait"
	multisiteCleanup      multisitePhase = "cleanup"
)

type ownedKey struct {
	Scope ownershipScope `json:"scope"`
	Key   string         `json:"key"`
}

type ownershipState struct {
	Version                 int        `json:"version"`
	ConfigFingerprint       string     `json:"config_fingerprint"`
	OwnerTag                string     `json:"owner_tag"`
	Mode                    string     `json:"mode"`
	Phase                   string     `json:"phase"`
	PendingKeys             []ownedKey `json:"pending_keys,omitempty"`
	ReconciliationPending   bool       `json:"reconciliation_pending,omitempty"`
	SourceKey               string     `json:"source_key"`
	DestinationKey          string     `json:"destination_key"`
	PayloadDigest           string     `json:"payload_digest"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdateEvery             int        `json:"update_every"`
	HeartbeatAt             time.Time  `json:"heartbeat_at"`
	RetiredAt               *time.Time `json:"retired_at,omitempty"`
	SourcePutAttempted      bool       `json:"source_put_attempted"`
	SourcePutAt             *time.Time `json:"source_put_at,omitempty"`
	DestinationVisibleAt    *time.Time `json:"destination_visible_at,omitempty"`
	SourceDeleteAttemptedAt *time.Time `json:"source_delete_attempted_at,omitempty"`
	SourceDeletedAt         *time.Time `json:"source_deleted_at,omitempty"`
	DestinationGoneAt       *time.Time `json:"destination_gone_at,omitempty"`
	QuarantinedAt           *time.Time `json:"quarantined_at,omitempty"`
	CleanupQuarantinedAt    *time.Time `json:"cleanup_quarantined_at,omitempty"`
	CleanupConfirmedAt      *time.Time `json:"cleanup_confirmed_at,omitempty"`
	CleanupDeleteAttempted  bool       `json:"cleanup_delete_attempted,omitempty"`
	TerminalReason          string     `json:"terminal_reason,omitempty"`
}

type ownershipStateStore struct {
	path                   string
	fingerprint            string
	ownerTag               string
	mode                   string
	sourceProbePrefix      string
	destinationProbePrefix string
	now                    func() time.Time
}

func newOwnershipStateStore(
	path, fingerprint, ownerTag, mode, sourcePrefix, destinationPrefix string,
) *ownershipStateStore {
	return &ownershipStateStore{
		path:                   path,
		fingerprint:            fingerprint,
		ownerTag:               ownerTag,
		mode:                   mode,
		sourceProbePrefix:      multisiteProbePrefix(sourcePrefix, ownerTag),
		destinationProbePrefix: multisiteProbePrefix(destinationPrefix, ownerTag),
		now:                    time.Now,
	}
}

func (s *ownershipStateStore) load() (*ownershipState, error) {
	file, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open pending state: %w", err)
	}
	defer file.Close() //nolint:errcheck // Nothing beyond closing is possible after the bounded read.

	raw, readErr := io.ReadAll(io.LimitReader(file, maxOwnershipStateBytes+1))
	if readErr != nil {
		return nil, fmt.Errorf("read pending state: %w", readErr)
	}
	if len(raw) > maxOwnershipStateBytes {
		return nil, errors.New("pending state file is too large")
	}

	state := &ownershipState{}
	if err := json.Unmarshal(raw, state); err != nil {
		return nil, fmt.Errorf("decode pending state: %w", err)
	}
	if err := state.validate(
		s.fingerprint, s.ownerTag, s.mode, s.sourceProbePrefix, s.destinationProbePrefix, s.now(),
	); err != nil {
		return nil, err
	}
	return state, nil
}

func (s *ownershipStateStore) save(state *ownershipState) error {
	if state == nil {
		return errors.New("save nil pending state")
	}
	state.Version = ownershipStateVersion
	state.ConfigFingerprint = s.fingerprint
	state.OwnerTag = s.ownerTag
	state.Mode = s.mode
	if err := state.validate(
		s.fingerprint, s.ownerTag, s.mode, s.sourceProbePrefix, s.destinationProbePrefix, s.now(),
	); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode pending state: %w", err)
	}
	raw = append(raw, '\n')

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create pending state directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("restrict pending state directory: %w", err)
	}

	tmpPath := s.path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create pending state temporary file: %w", err)
	}
	if _, err = file.Write(raw); err != nil {
		_ = file.Close()
		return fmt.Errorf("write pending state temporary file: %w", err)
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync pending state temporary file: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close pending state temporary file: %w", err)
	}
	if err = os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("publish pending state: %w", err)
	}
	return syncStateDir(dir)
}

func (s *ownershipStateStore) clear() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove pending state: %w", err)
	}
	_ = os.Remove(s.path + ".tmp")
	return syncStateDir(filepath.Dir(s.path))
}

func syncStateDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open pending state directory: %w", err)
	}
	syncErr := handle.Sync()
	closeErr := handle.Close()
	if syncErr != nil {
		return fmt.Errorf("sync pending state directory: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close pending state directory: %w", closeErr)
	}
	return nil
}

func unresolvedOwnershipStateExists(dir, ownPath string) (bool, error) {
	handle, err := os.Open(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("open s3check ownership state directory: %w", err)
	}
	defer handle.Close() //nolint:errcheck // The bounded directory scan has no additional close action.

	jsonFiles := 0
	for {
		entries, readErr := handle.ReadDir(maxOwnershipStateFiles + 1)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return false, fmt.Errorf("read s3check ownership state directory: %w", readErr)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			jsonFiles++
			if jsonFiles > maxOwnershipStateFiles {
				return true, errors.New("too many s3check ownership state files")
			}
			path := filepath.Join(dir, entry.Name())
			if path == ownPath {
				continue
			}
			unresolved, inspectErr := unresolvedOwnershipFile(path, time.Now())
			if inspectErr != nil || unresolved {
				return unresolved, inspectErr
			}
		}
		if errors.Is(readErr, io.EOF) || len(entries) == 0 {
			return false, nil
		}
	}
}

func unresolvedOwnershipFile(path string, now time.Time) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("open s3check ownership state: %w", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maxOwnershipStateBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return false, fmt.Errorf("read s3check ownership state: %w", readErr)
	}
	if closeErr != nil {
		return false, fmt.Errorf("close s3check ownership state: %w", closeErr)
	}
	if len(raw) > maxOwnershipStateBytes {
		return true, errors.New("another s3check ownership state file is too large")
	}
	state := &ownershipState{}
	if decodeErr := json.Unmarshal(raw, state); decodeErr != nil {
		return true, fmt.Errorf("decode another s3check ownership state: %w", decodeErr)
	}
	if state.Version != ownershipStateVersion {
		return true, fmt.Errorf("unsupported s3check ownership state version %d", state.Version)
	}
	if !isOwnershipHash(state.ConfigFingerprint, sha256.Size*2) ||
		!isOwnershipHash(state.OwnerTag, 16*2) {
		return true, errors.New("another s3check ownership state has an invalid identity")
	}
	sourcePrefix, destinationPrefix := foreignOwnershipPrefixes(state)
	if err := state.validate(
		state.ConfigFingerprint, state.OwnerTag, state.Mode, sourcePrefix, destinationPrefix, now,
	); err != nil {
		return true, fmt.Errorf("validate another s3check ownership state: %w", err)
	}
	if !state.hasActiveObject() && len(state.PendingKeys) == 0 && !state.ReconciliationPending {
		return true, errors.New("another s3check ownership state owns no keys")
	}
	if state.RetiredAt != nil {
		return true, nil
	}

	// A timestamp alone cannot distinguish a fresh healthy owner from a process
	// that crashed immediately after refreshing its heartbeat. The owner holds
	// a dedicated liveness lock for life, so a free lock means an unresolved orphan even
	// while its heartbeat is still fresh.
	liveOwner, lockErr := ownershipOwnerIsLive(path)
	if lockErr != nil {
		return true, lockErr
	}
	if !liveOwner {
		return true, nil
	}

	if state.UpdateEvery <= 0 || state.HeartbeatAt.IsZero() {
		return true, errors.New("another s3check ownership state has no live heartbeat")
	}
	if state.HeartbeatAt.After(now) {
		return true, errors.New("another s3check ownership state heartbeat is in the future")
	}
	// A live collector refreshes its journal every scheduled collection. Treat
	// recent journals as active owners and stale journals as fail-closed orphans.
	staleAfter := 3*time.Duration(state.UpdateEvery)*time.Second + cycleProcessingMargin
	return now.Sub(state.HeartbeatAt) >= staleAfter, nil
}

func ownershipOwnerIsLive(path string) (bool, error) {
	locker := filelock.New(filepath.Dir(path))
	lockName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + ".owner"
	acquired, err := locker.Lock(lockName)
	if err != nil {
		return false, fmt.Errorf("probe s3check ownership owner lock: %w", err)
	}
	if acquired {
		locker.Unlock(lockName)
		return false, nil
	}
	return true, nil
}

func foreignOwnershipPrefixes(state *ownershipState) (sourcePrefix, destinationPrefix string) {
	if state.SourceKey != "" {
		sourcePrefix = ownershipKeyPrefix(state.SourceKey)
	}
	if state.DestinationKey != "" {
		destinationPrefix = ownershipKeyPrefix(state.DestinationKey)
	}
	for _, owned := range state.PendingKeys {
		switch owned.Scope {
		case ownershipSource, ownershipSingle:
			if sourcePrefix == "" {
				sourcePrefix = ownershipKeyPrefix(owned.Key)
			}
		case ownershipDestination:
			if destinationPrefix == "" {
				destinationPrefix = ownershipKeyPrefix(owned.Key)
			}
		}
	}
	if destinationPrefix == "" {
		destinationPrefix = sourcePrefix
	}
	return sourcePrefix, destinationPrefix
}

func ownershipKeyPrefix(key string) string {
	base := path.Base(key)
	return strings.TrimSuffix(key, base)
}

func isOwnershipHash(value string, expectedLen int) bool {
	if len(value) != expectedLen {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (state *ownershipState) clone() *ownershipState {
	copied := *state
	copied.PendingKeys = append([]ownedKey(nil), state.PendingKeys...)
	return &copied
}

func (state *ownershipState) ownsKey(owned ownedKey) bool {
	return slices.Contains(state.PendingKeys, owned)
}

func (state *ownershipState) ownsOrAddsKey(owned ownedKey) bool {
	if state.ownsKey(owned) {
		return true
	}
	if len(state.PendingKeys) >= maxOwnedKeys {
		return false
	}
	state.PendingKeys = append(state.PendingKeys, owned)
	return true
}

func (state *ownershipState) removeOwnedKey(owned ownedKey) {
	filtered := state.PendingKeys[:0]
	for _, existing := range state.PendingKeys {
		if existing != owned {
			filtered = append(filtered, existing)
		}
	}
	state.PendingKeys = filtered
}

func (state *ownershipState) hasActiveObject() bool {
	return state.SourceKey != "" || state.DestinationKey != "" || state.PayloadDigest != ""
}

func (state *ownershipState) validate(
	fingerprint, ownerTag, mode, sourceProbePrefix, destinationProbePrefix string,
	now time.Time,
) error {
	if state.Version != ownershipStateVersion {
		return fmt.Errorf("unsupported pending state version %d", state.Version)
	}
	if mode != modeSingle && mode != modeMultisite {
		return errors.New("pending state has an invalid mode")
	}
	if state.ConfigFingerprint != fingerprint {
		return errors.New("pending state belongs to different source, destination, or mode settings; " +
			"the previous configuration may still own probe objects. Remove those objects from the previous owner prefixes " +
			"before deleting this state file")
	}
	if state.OwnerTag != ownerTag || state.Mode != mode {
		return errors.New("pending state belongs to a different owner or job identity")
	}
	switch multisitePhase(state.Phase) {
	case multisiteSourcePut, multisiteReplication, multisiteSourceDelete, multisiteDeleteWait, multisiteCleanup:
	default:
		return errors.New("pending state has an invalid phase")
	}
	if len(state.PendingKeys) > maxOwnedKeys {
		return errors.New("pending state owns too many keys")
	}
	if state.HeartbeatAt.After(now) || (state.RetiredAt != nil && state.RetiredAt.After(now)) {
		return errors.New("pending state has a future ownership timestamp")
	}
	active := state.SourceKey != "" || state.DestinationKey != "" || state.PayloadDigest != ""
	if state.ReconciliationPending {
		if multisitePhase(state.Phase) != multisiteCleanup || active ||
			state.SourcePutAttempted || state.QuarantinedAt != nil || state.CleanupQuarantinedAt != nil ||
			state.CleanupConfirmedAt != nil || state.CleanupDeleteAttempted || state.TerminalReason != "" {
			return errors.New("pending state has an invalid reconciliation marker")
		}
		if state.SourcePutAt != nil || state.DestinationVisibleAt != nil ||
			state.SourceDeleteAttemptedAt != nil || state.SourceDeletedAt != nil || state.DestinationGoneAt != nil {
			return errors.New("pending reconciliation marker contains lifecycle timestamps")
		}
	}

	seenKeys := make(map[ownedKey]struct{}, len(state.PendingKeys))
	for _, owned := range state.PendingKeys {
		if _, duplicate := seenKeys[owned]; duplicate {
			return errors.New("pending state contains a duplicate owned key")
		}
		seenKeys[owned] = struct{}{}
		var prefix string
		switch {
		case mode == modeSingle && owned.Scope == ownershipSingle:
			prefix = sourceProbePrefix
		case mode == modeMultisite && owned.Scope == ownershipSource:
			prefix = sourceProbePrefix
		case mode == modeMultisite && owned.Scope == ownershipDestination:
			prefix = destinationProbePrefix
		default:
			return errors.New("pending state contains a key for an unsupported endpoint")
		}
		if !isMultisiteProbeKey(owned.Key, prefix, ownerTag) {
			return errors.New("pending state contains a key outside its owner namespace")
		}
	}

	if mode == modeMultisite && state.QuarantinedAt != nil {
		return errors.New("multisite pending state contains a single-site quarantine timestamp")
	}
	if state.CleanupQuarantinedAt != nil {
		if mode != modeMultisite || multisitePhase(state.Phase) != multisiteCleanup ||
			state.hasActiveObject() || len(state.PendingKeys) == 0 {
			return errors.New("pending state has an invalid cleanup quarantine marker")
		}
		if state.CleanupQuarantinedAt.Before(state.CreatedAt) {
			return errors.New("pending state cleanup quarantine timestamp is before probe creation")
		}
		if state.CleanupConfirmedAt != nil && state.CleanupConfirmedAt.Before(*state.CleanupQuarantinedAt) {
			return errors.New("pending state cleanup confirmation precedes its quarantine")
		}
		if state.CleanupDeleteAttempted && state.CleanupConfirmedAt != nil {
			return errors.New("pending state has cleanup confirmation during a delete attempt")
		}
	}
	if state.CleanupQuarantinedAt == nil && (state.CleanupConfirmedAt != nil || state.CleanupDeleteAttempted) {
		return errors.New("pending state has cleanup progress without quarantine")
	}

	if mode == modeSingle {
		if multisitePhase(state.Phase) != multisiteSourcePut && multisitePhase(state.Phase) != multisiteCleanup {
			return errors.New("single-site pending state has an invalid phase")
		}
		if state.DestinationKey != "" {
			return errors.New("single-site pending state has a destination key")
		}
		if active && !isMultisiteProbeKey(state.SourceKey, sourceProbePrefix, ownerTag) {
			return errors.New("single-site pending state key is outside its owner namespace")
		}
		if !active && len(state.PendingKeys) == 0 && !state.ReconciliationPending {
			return errors.New("single-site pending state owns no keys")
		}
		if state.CreatedAt.IsZero() {
			return errors.New("single-site pending state has no creation timestamp")
		}
		if state.CreatedAt.After(now) {
			return errors.New("single-site pending state creation timestamp is in the future")
		}
		if state.QuarantinedAt != nil && state.QuarantinedAt.After(now) {
			return errors.New("single-site quarantine timestamp is in the future")
		}
		if state.CleanupQuarantinedAt != nil || state.CleanupConfirmedAt != nil || state.CleanupDeleteAttempted {
			return errors.New("single-site pending state contains multisite cleanup state")
		}
		if active && !state.SourcePutAttempted {
			return errors.New("single-site pending state has an unattempted active write")
		}
		if state.SourcePutAt != nil || state.DestinationVisibleAt != nil || state.SourceDeleteAttemptedAt != nil ||
			state.SourceDeletedAt != nil || state.DestinationGoneAt != nil {
			return errors.New("single-site pending state contains multisite timestamps")
		}
		return nil
	}

	if !active && len(state.PendingKeys) > 0 && multisitePhase(state.Phase) != multisiteCleanup {
		return errors.New("pending keys require the cleanup phase")
	}
	if active {
		if !isMultisiteProbeKey(state.SourceKey, sourceProbePrefix, ownerTag) {
			return errors.New("pending state source probe key is outside the configured owner prefix or malformed")
		}
		if !isMultisiteProbeKey(state.DestinationKey, destinationProbePrefix, ownerTag) {
			return errors.New("pending state destination probe key is outside the configured owner prefix or malformed")
		}
		if sourceKeyBase(state.SourceKey) != sourceKeyBase(state.DestinationKey) {
			return errors.New("pending state source and destination keys do not identify the same probe")
		}
		if len(state.PayloadDigest) != sha256.Size*2 {
			return errors.New("pending state has an invalid payload digest")
		}
		if _, err := hex.DecodeString(state.PayloadDigest); err != nil {
			return errors.New("pending state has an invalid payload digest")
		}
	} else if len(state.PendingKeys) == 0 && !state.ReconciliationPending {
		return errors.New("multisite pending state owns no keys")
	}
	if state.CreatedAt.IsZero() {
		return errors.New("pending state has no creation timestamp")
	}
	timestamps := []struct {
		name  string
		value *time.Time
	}{
		{name: "source_put_at", value: state.SourcePutAt},
		{name: "destination_visible_at", value: state.DestinationVisibleAt},
		{name: "source_delete_attempted_at", value: state.SourceDeleteAttemptedAt},
		{name: "source_deleted_at", value: state.SourceDeletedAt},
		{name: "destination_gone_at", value: state.DestinationGoneAt},
		{name: "cleanup_quarantined_at", value: state.CleanupQuarantinedAt},
		{name: "cleanup_confirmed_at", value: state.CleanupConfirmedAt},
	}
	if state.CreatedAt.After(now) {
		return errors.New("pending state creation timestamp is in the future")
	}
	for _, timestamp := range timestamps {
		if timestamp.value != nil && timestamp.value.After(now) {
			return fmt.Errorf("pending state %s timestamp is in the future", timestamp.name)
		}
	}
	if state.DestinationVisibleAt != nil && state.SourcePutAt == nil {
		return errors.New("pending state has visibility without a source write timestamp")
	}
	if state.SourcePutAt != nil {
		if !state.SourcePutAttempted {
			return errors.New("pending state has a source write timestamp without an attempted write")
		}
		if state.SourcePutAt.Before(state.CreatedAt) {
			return errors.New("pending state has a source write timestamp before probe creation")
		}
	}
	if state.DestinationVisibleAt != nil && state.DestinationVisibleAt.Before(*state.SourcePutAt) {
		return errors.New("pending state has destination visibility before the source write")
	}
	if state.SourceDeleteAttemptedAt != nil {
		if state.DestinationVisibleAt == nil {
			return errors.New("pending state has a source delete attempt without destination visibility")
		}
		if state.SourceDeleteAttemptedAt.Before(*state.DestinationVisibleAt) {
			return errors.New("pending state has a source delete attempt before destination visibility")
		}
	}
	if state.SourceDeletedAt != nil {
		if state.DestinationVisibleAt == nil {
			return errors.New("pending state has a source delete without destination visibility")
		}
		if state.SourceDeleteAttemptedAt != nil && state.SourceDeletedAt.Before(*state.SourceDeleteAttemptedAt) {
			return errors.New("pending state has a source delete completion before its delete attempt")
		}
		if state.SourceDeletedAt.Before(*state.DestinationVisibleAt) {
			return errors.New("pending state has a source delete before destination visibility")
		}
	}
	if state.DestinationGoneAt != nil {
		if state.SourceDeletedAt == nil {
			return errors.New("pending state has destination disappearance without a source delete")
		}
		if state.DestinationGoneAt.Before(*state.SourceDeletedAt) {
			return errors.New("pending state has destination disappearance before the source delete")
		}
	}
	return nil
}

func sourceKeyBase(key string) string {
	return path.Base(key)
}

func isMultisiteProbeKey(key, expectedPrefix, routeTag string) bool {
	base, valid := strings.CutPrefix(key, expectedPrefix)
	if !valid || strings.Contains(base, "/") {
		return false
	}
	if !multisiteProbeKeyRE.MatchString(base) {
		return false
	}
	parts := strings.Split(strings.TrimSuffix(base, ".bin"), "-")
	return parts[len(parts)-1] == routeTag
}

func multisiteOwnerTag(machineGUID, jobName string) string {
	digest := sha256.Sum256([]byte("netdata:s3check:owner:v1\x00" + machineGUID + "\x00" + jobName))
	return hex.EncodeToString(digest[:16])
}

func multisiteProbePrefix(prefix, ownerTag string) string {
	return prefix + ownerTag + "/"
}

func ownershipStatePath(root, jobName string) string {
	digest := sha256.Sum256([]byte(jobName))
	return filepath.Join(root, "go.d", "s3check", hex.EncodeToString(digest[:])+".json")
}

func (c *Collector) ownershipFingerprint() string {
	source := c.sourceEndpoint()
	payload := struct {
		Mode                 string `json:"mode"`
		SourceSite           string `json:"source_site,omitempty"`
		SourceEndpoint       string `json:"source_endpoint"`
		SourceRegion         string `json:"source_region"`
		SourceBucket         string `json:"source_bucket"`
		SourcePrefix         string `json:"source_prefix"`
		SourcePathStyle      bool   `json:"source_path_style"`
		DestinationSite      string `json:"destination_site,omitempty"`
		DestinationEndpoint  string `json:"destination_endpoint,omitempty"`
		DestinationRegion    string `json:"destination_region,omitempty"`
		DestinationBucket    string `json:"destination_bucket,omitempty"`
		DestinationPrefix    string `json:"destination_prefix,omitempty"`
		DestinationPathStyle bool   `json:"destination_path_style,omitempty"`
	}{
		Mode: c.Mode, SourceSite: c.SourceSite, SourceEndpoint: canonicalEndpointKey(source.Endpoint), SourceRegion: source.Region,
		SourceBucket: source.Bucket, SourcePrefix: source.Prefix, SourcePathStyle: source.PathStyle,
	}
	if c.Mode == modeMultisite {
		destination := c.destinationEndpoint()
		payload.DestinationSite = c.Destination.Site
		payload.DestinationEndpoint = canonicalEndpointKey(destination.Endpoint)
		payload.DestinationRegion = destination.Region
		payload.DestinationBucket = destination.Bucket
		payload.DestinationPrefix = destination.Prefix
		payload.DestinationPathStyle = destination.PathStyle
	}
	raw, _ := json.Marshal(payload)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
