// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/framework/filelock"
)

const (
	cursorMagic         = "NDRFCUR1"
	cursorVersion       = uint16(2)
	maxCursorPayload    = 16 << 20
	maxCursorExactKeys  = 65_536
	cursorLockName      = "producer"
	cursorNamespaceLock = "namespace"
	// Retirement needs the exact on-disk path created by framework/filelock.
	cursorLockFileSuffix   = ".collector.lock"
	cursorDirectoryName    = "redfish/cursors"
	cursorCleanupBatchSize = 32
	cursorCleanupInterval  = time.Minute
	cursorLockRetryDelay   = 10 * time.Millisecond

	cursorSizeOffset        = len(cursorMagic) + 2
	cursorRetentionOffset   = cursorSizeOffset + 4
	cursorMetadataCRCOffset = cursorRetentionOffset + 8
	cursorChecksumOffset    = cursorMetadataCRCOffset + 4
	cursorHeaderSize        = cursorChecksumOffset + sha256.Size
)

type logSourceCursor struct {
	Generation          uint64   `json:"generation"`
	Mode                string   `json:"mode,omitempty"`
	Ordering            string   `json:"ordering,omitempty"`
	Continuation        string   `json:"continuation,omitempty"`
	ClientContext       string   `json:"client_context,omitempty"`
	ContextDirty        bool     `json:"context_dirty,omitempty"`
	ReconcileStarted    bool     `json:"reconcile_started,omitempty"`
	ReconcileExpected   int      `json:"reconcile_expected,omitempty"`
	ReconcileFetched    int      `json:"reconcile_fetched,omitempty"`
	ReconcileSourceKeys []string `json:"reconcile_source_keys,omitempty"`
	ReconcileRecordKeys []string `json:"reconcile_record_keys,omitempty"`
	ContinuationKeys    []string `json:"continuation_keys,omitempty"`
	Initialized         bool     `json:"initialized,omitempty"`
	ExactComplete       bool     `json:"exact_complete,omitempty"`
	CompleteCountKnown  bool     `json:"complete_count_known,omitempty"`
	LastCompleteCount   int      `json:"last_complete_count,omitempty"`
	BoundaryUsec        int64    `json:"boundary_usec,omitempty"`
	BoundaryKeys        []string `json:"boundary_keys,omitempty"`
	ExactRecordKeys     []string `json:"exact_record_keys,omitempty"`
	LastCompleteUsec    int64    `json:"last_complete_usec,omitempty"`
	LastActiveUsec      int64    `json:"last_active_usec,omitempty"`
}

type cursorPayload struct {
	EndpointKey         string                     `json:"endpoint_key"`
	OriginDigest        string                     `json:"origin_digest"`
	OrphanRetentionNsec int64                      `json:"-"`
	LastActiveUsec      int64                      `json:"last_active_usec"`
	Sources             map[string]logSourceCursor `json:"sources"`
}

type cursorRetentionMetadata struct {
	retention time.Duration
	modTime   time.Time
}

type cursorCleanupRoot struct {
	after     string
	nextSweep time.Time
	scans     uint64
}

type cursorCleanupGate struct {
	mu    sync.Mutex
	roots map[string]cursorCleanupRoot
}

var sharedCursorCleanupGate = newCursorCleanupGate()

type cursorCoordinator struct {
	mu sync.Mutex

	endpointKey  string
	originDigest string
	orphanTTL    time.Duration
	root         string
	path         string
	locker       *filelock.Locker
	claimed      bool
	loaded       bool
	payload      cursorPayload

	dirty       bool
	persistWait time.Duration
	nextPersist time.Time
	persistErr  error

	payloadBytes int
	fixedBytes   int
	sourceBytes  map[string]int
	cleanup      *cursorCleanupGate
}

func newCursorCleanupGate() *cursorCleanupGate {
	return &cursorCleanupGate{roots: make(map[string]cursorCleanupRoot)}
}

func newCursorCoordinator(endpointKey, origin string, orphanTTL time.Duration) *cursorCoordinator {
	full := stableKey("netdata:redfish:endpoint:v1", origin, digestHexChars)
	c := &cursorCoordinator{
		endpointKey:  endpointKey,
		originDigest: full,
		orphanTTL:    orphanTTL,
		payload: cursorPayload{
			EndpointKey:         endpointKey,
			OriginDigest:        full,
			OrphanRetentionNsec: orphanTTL.Nanoseconds(),
			Sources:             make(map[string]logSourceCursor),
		},
		cleanup: sharedCursorCleanupGate,
	}
	c.resetPayloadSizeLocked()
	return c
}

func (c *cursorCoordinator) Claim(ctx context.Context) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.claimed {
		return true, nil
	}
	root := filepath.Join(netdataStateDir(), filepath.FromSlash(cursorDirectoryName))
	if err := ensureCursorDirectory(root); err != nil {
		return false, err
	}
	namespace, err := lockCursorNamespace(ctx, root)
	if err != nil {
		return false, fmt.Errorf("lock Redfish cursor namespace: %w", err)
	}
	locker := filelock.New(root)
	ok, err := locker.Lock(c.endpointKey + "-" + cursorLockName)
	namespace.UnlockAll()
	if err != nil {
		return false, fmt.Errorf("lock Redfish endpoint cursor: %w", err)
	}
	if !ok {
		return false, nil
	}
	c.root = root
	c.path = filepath.Join(root, c.endpointKey+".cursor")
	c.locker = locker
	c.claimed = true
	if err := c.loadLocked(); err != nil {
		locker.UnlockAll()
		c.locker = nil
		c.claimed = false
		return false, err
	}
	c.cleanupOrphansLocked(time.Now())
	return true, nil
}

func (c *cursorCoordinator) Source(key string) logSourceCursor {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneLogCursor(c.payload.Sources[key])
}

func (c *cursorCoordinator) UpdateSource(
	key string,
	cursor logSourceCursor,
	now time.Time,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.updateSourceLocked(key, cursor, now)
}

func (c *cursorCoordinator) Persist(now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.claimed {
		return nil
	}
	c.cleanupOrphansLocked(now)
	c.pruneInactiveSourcesLocked(now)
	if !c.dirty {
		return nil
	}
	if !c.nextPersist.IsZero() && now.Before(c.nextPersist) {
		return fmt.Errorf(
			"Redfish cursor checkpoint is pending retry after a prior failure: %w",
			c.persistErr,
		)
	}
	if err := c.persistLocked(); err != nil {
		if c.persistWait == 0 {
			c.persistWait = time.Second
		} else {
			c.persistWait = min(c.persistWait*2, time.Minute)
		}
		c.nextPersist = now.Add(c.persistWait)
		c.persistErr = err
		return err
	}
	c.dirty = false
	c.persistWait = 0
	c.nextPersist = time.Time{}
	c.persistErr = nil
	return nil
}

func (c *cursorCoordinator) TouchSources(keys []string, now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	updates := make(map[string]logSourceCursor, len(keys))
	for _, key := range keys {
		source, ok := c.payload.Sources[key]
		if !ok {
			continue
		}
		source.LastActiveUsec = now.UnixMicro()
		updates[key] = source
	}
	if len(updates) == 0 {
		return nil
	}
	lastActive := now.UnixMicro()
	fixed := c.fixedSizeFor(lastActive)
	size := c.payloadBytes + fixed - c.fixedBytes
	encoded := make(map[string]int, len(updates))
	for key, source := range updates {
		entrySize := cursorSourceEntrySize(key, source)
		encoded[key] = entrySize
		size += entrySize - c.sourceBytes[key]
	}
	if size > maxCursorPayload {
		return fmt.Errorf(
			"Redfish endpoint cursor activity checkpoint requires %d bytes; limit is %d",
			size,
			maxCursorPayload,
		)
	}
	for key, source := range updates {
		c.payload.Sources[key] = source
		c.sourceBytes[key] = encoded[key]
	}
	c.payload.LastActiveUsec = lastActive
	c.fixedBytes = fixed
	c.payloadBytes = size
	c.dirty = true
	return nil
}

func (c *cursorCoordinator) pruneInactiveSourcesLocked(now time.Time) {
	if c.orphanTTL == 0 {
		return
	}
	cutoff := now.Add(-c.orphanTTL).UnixMicro()
	pruned := false
	for key, source := range c.payload.Sources {
		if source.LastActiveUsec > cutoff {
			continue
		}
		delete(c.payload.Sources, key)
		pruned = true
	}
	if pruned {
		c.payload.LastActiveUsec = now.UnixMicro()
		c.dirty = true
		c.resetPayloadSizeLocked()
	}
}

// CheckpointSource durably records state that must precede a state-advancing
// source request. It deliberately bypasses ordinary checkpoint backoff.
func (c *cursorCoordinator) CheckpointSource(key string, cursor logSourceCursor, now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.claimed {
		return errors.New("Redfish cursor is not claimed")
	}
	if err := c.updateSourceLocked(key, cursor, now); err != nil {
		return err
	}
	if err := c.persistLocked(); err != nil {
		return err
	}
	c.dirty = false
	c.persistWait = 0
	c.nextPersist = time.Time{}
	c.persistErr = nil
	return nil
}

func (c *cursorCoordinator) Close() {
	c.mu.Lock()
	locker := c.locker
	c.locker = nil
	c.claimed = false
	c.mu.Unlock()
	if locker != nil {
		locker.UnlockAll()
	}
}

func (c *cursorCoordinator) loadLocked() error {
	if c.loaded {
		return nil
	}
	info, err := os.Lstat(c.path)
	if errors.Is(err, os.ErrNotExist) {
		c.loaded = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect Redfish cursor: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
		info.Size() <= 0 || info.Size() > int64(maxCursorPayload+cursorHeaderSize) {
		return errors.New("Redfish cursor is not a bounded regular file")
	}
	file, err := os.Open(c.path)
	if err != nil {
		return fmt.Errorf("open Redfish cursor: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return errors.New("Redfish cursor changed during validation")
	}
	raw, err := io.ReadAll(io.LimitReader(file, int64(maxCursorPayload+cursorHeaderSize+1)))
	if err != nil {
		return fmt.Errorf("read Redfish cursor: %w", err)
	}
	payload, err := decodeCursor(raw)
	if err != nil {
		return err
	}
	if payload.EndpointKey != c.endpointKey || payload.OriginDigest != c.originDigest {
		return errors.New("Redfish cursor identity does not match the configured endpoint")
	}
	if payload.Sources == nil {
		payload.Sources = make(map[string]logSourceCursor)
	}
	for key, source := range payload.Sources {
		if len(key) != 64 || !isLowerHex(key) {
			return errors.New("Redfish cursor contains an invalid source identity")
		}
		source.BoundaryKeys = boundedSortedKeys(source.BoundaryKeys, maxCursorExactKeys)
		source.ExactRecordKeys = boundedSortedKeys(source.ExactRecordKeys, maxCursorExactKeys)
		source.ReconcileSourceKeys = boundedSortedKeys(source.ReconcileSourceKeys, maxCursorExactKeys)
		source.ReconcileRecordKeys = boundedSortedKeys(source.ReconcileRecordKeys, maxCursorExactKeys)
		source.ContinuationKeys = boundedSortedKeys(source.ContinuationKeys, maxCursorExactKeys)
		if err := validateLogCursor(source); err != nil {
			return fmt.Errorf("Redfish cursor source %q: %w", key, err)
		}
		payload.Sources[key] = source
	}
	retention := c.orphanTTL.Nanoseconds()
	if payload.OrphanRetentionNsec != retention {
		payload.OrphanRetentionNsec = retention
		c.dirty = true
	}
	c.payload = payload
	c.resetPayloadSizeLocked()
	c.loaded = true
	return nil
}

func (c *cursorCoordinator) persistLocked() error {
	c.payload.EndpointKey = c.endpointKey
	c.payload.OriginDigest = c.originDigest
	c.payload.OrphanRetentionNsec = c.orphanTTL.Nanoseconds()
	if err := validateCursorPayload(c.payload); err != nil {
		return err
	}
	raw, err := encodeCursor(c.payload)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.root, "."+c.endpointKey+".cursor-*")
	if err != nil {
		return fmt.Errorf("create Redfish cursor temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("protect Redfish cursor temporary file: %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		return fmt.Errorf("write Redfish cursor: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync Redfish cursor: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close Redfish cursor: %w", err)
	}
	if err := os.Rename(tmpPath, c.path); err != nil {
		return fmt.Errorf("replace Redfish cursor: %w", err)
	}
	if err := syncCursorDirectory(c.root); err != nil {
		return fmt.Errorf("sync Redfish cursor directory: %w", err)
	}
	committed = true
	return nil
}

func validateCursorPayload(payload cursorPayload) error {
	if payload.EndpointKey == "" || payload.OriginDigest == "" {
		return errors.New("Redfish cursor identity is incomplete")
	}
	if payload.OrphanRetentionNsec < 0 {
		return errors.New("Redfish cursor orphan retention is negative")
	}
	for key, source := range payload.Sources {
		if len(key) != 64 || !isLowerHex(key) {
			return errors.New("Redfish cursor contains an invalid source identity")
		}
		if err := validateLogCursor(source); err != nil {
			return fmt.Errorf("Redfish cursor source %q: %w", key, err)
		}
	}
	return nil
}

func encodeCursor(payload cursorPayload) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode Redfish cursor payload: %w", err)
	}
	if len(body) > maxCursorPayload {
		return nil, errors.New("Redfish cursor exceeds the internal payload limit")
	}
	checksum := sha256.Sum256(body)
	var result bytes.Buffer
	result.Grow(cursorHeaderSize + len(body))
	result.WriteString(cursorMagic)
	_ = binary.Write(&result, binary.BigEndian, cursorVersion)
	_ = binary.Write(&result, binary.BigEndian, uint32(len(body)))
	_ = binary.Write(&result, binary.BigEndian, uint64(payload.OrphanRetentionNsec))
	_ = binary.Write(&result, binary.BigEndian, crc32.ChecksumIEEE(result.Bytes()))
	result.Write(checksum[:])
	result.Write(body)
	return result.Bytes(), nil
}

func decodeCursor(raw []byte) (cursorPayload, error) {
	if len(raw) < cursorHeaderSize || string(raw[:len(cursorMagic)]) != cursorMagic {
		return cursorPayload{}, errors.New("Redfish cursor has invalid magic")
	}
	version := binary.BigEndian.Uint16(raw[len(cursorMagic):])
	if version != cursorVersion {
		return cursorPayload{}, fmt.Errorf("unsupported Redfish cursor version %d", version)
	}
	size := binary.BigEndian.Uint32(raw[cursorSizeOffset:])
	if size > maxCursorPayload || int(size) != len(raw)-cursorHeaderSize {
		return cursorPayload{}, errors.New("Redfish cursor has an invalid declared length")
	}
	wantMetadataCRC := binary.BigEndian.Uint32(raw[cursorMetadataCRCOffset:])
	if crc32.ChecksumIEEE(raw[:cursorMetadataCRCOffset]) != wantMetadataCRC {
		return cursorPayload{}, errors.New("Redfish cursor metadata checksum mismatch")
	}
	retention := binary.BigEndian.Uint64(raw[cursorRetentionOffset:])
	if retention > uint64(1<<63-1) {
		return cursorPayload{}, errors.New("Redfish cursor has invalid orphan retention")
	}
	want := raw[cursorChecksumOffset : cursorChecksumOffset+sha256.Size]
	body := raw[cursorHeaderSize:]
	got := sha256.Sum256(body)
	if !bytes.Equal(want, got[:]) {
		return cursorPayload{}, errors.New("Redfish cursor checksum mismatch")
	}
	var payload cursorPayload
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&payload); err != nil {
		return cursorPayload{}, fmt.Errorf("decode Redfish cursor payload: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return cursorPayload{}, errors.New("Redfish cursor payload contains trailing JSON")
		}
		return cursorPayload{}, fmt.Errorf("decode Redfish cursor payload trailer: %w", err)
	}
	payload.OrphanRetentionNsec = int64(retention)
	return payload, nil
}

func (c *cursorCoordinator) updateSourceLocked(key string, cursor logSourceCursor, now time.Time) error {
	if len(key) != 64 || !isLowerHex(key) {
		return errors.New("reject invalid Redfish source cursor identity")
	}
	cursor.LastActiveUsec = now.UnixMicro()
	cursor.BoundaryKeys = boundedSortedKeys(cursor.BoundaryKeys, maxCursorExactKeys)
	cursor.ExactRecordKeys = boundedSortedKeys(cursor.ExactRecordKeys, maxCursorExactKeys)
	cursor.ReconcileSourceKeys = boundedSortedKeys(cursor.ReconcileSourceKeys, maxCursorExactKeys)
	cursor.ReconcileRecordKeys = boundedSortedKeys(cursor.ReconcileRecordKeys, maxCursorExactKeys)
	cursor.ContinuationKeys = boundedSortedKeys(cursor.ContinuationKeys, maxCursorExactKeys)
	if err := validateLogCursor(cursor); err != nil {
		return fmt.Errorf("reject invalid Redfish source cursor: %w", err)
	}
	lastActive := now.UnixMicro()
	fixed := c.fixedSizeFor(lastActive)
	entrySize := cursorSourceEntrySize(key, cursor)
	size := c.payloadBytes + fixed - c.fixedBytes + entrySize
	if prior, ok := c.sourceBytes[key]; ok {
		size -= prior
	} else if len(c.payload.Sources) > 0 {
		size++
	}
	if size > maxCursorPayload {
		return fmt.Errorf(
			"Redfish source cursor exceeds the endpoint checkpoint capacity: requires %d bytes; limit is %d",
			size,
			maxCursorPayload,
		)
	}
	c.payload.Sources[key] = cloneLogCursor(cursor)
	c.payload.LastActiveUsec = lastActive
	c.sourceBytes[key] = entrySize
	c.fixedBytes = fixed
	c.payloadBytes = size
	c.dirty = true
	return nil
}

func (c *cursorCoordinator) fixedSizeFor(lastActiveUsec int64) int {
	payload := c.payload
	payload.LastActiveUsec = lastActiveUsec
	payload.Sources = map[string]logSourceCursor{}
	raw, _ := json.Marshal(payload)
	return len(raw)
}

func (c *cursorCoordinator) resetPayloadSizeLocked() {
	c.fixedBytes = c.fixedSizeFor(c.payload.LastActiveUsec)
	c.sourceBytes = make(map[string]int, len(c.payload.Sources))
	c.payloadBytes = c.fixedBytes
	for key, source := range c.payload.Sources {
		c.sourceBytes[key] = cursorSourceEntrySize(key, source)
		c.payloadBytes += c.sourceBytes[key]
	}
	if len(c.payload.Sources) > 1 {
		c.payloadBytes += len(c.payload.Sources) - 1
	}
}

func cursorSourceEntrySize(key string, cursor logSourceCursor) int {
	rawKey, _ := json.Marshal(key)
	rawCursor, _ := json.Marshal(cursor)
	return len(rawKey) + 1 + len(rawCursor)
}

func validateLogCursor(cursor logSourceCursor) error {
	if cursor.Generation > 1<<62 {
		return errors.New("generation is out of range")
	}
	switch cursor.Mode {
	case "", "full", "incremental", "client_context", "recovery":
	default:
		return errors.New("mode is invalid")
	}
	switch cursor.Ordering {
	case "", "unknown", "ascending", "descending", "unordered":
	default:
		return errors.New("ordering is invalid")
	}
	if len(cursor.Continuation) > maxURIBytes {
		return errors.New("continuation is too long")
	}
	if cursor.ClientContext != "" &&
		(len(cursor.ClientContext) != 32 || !isLowerHex(cursor.ClientContext)) {
		return errors.New("client context is invalid")
	}
	for _, value := range []int64{
		cursor.BoundaryUsec, cursor.LastCompleteUsec, cursor.LastActiveUsec,
	} {
		if value < 0 {
			return errors.New("timestamp is negative")
		}
	}
	if cursor.ReconcileExpected < 0 || cursor.ReconcileFetched < 0 ||
		cursor.ReconcileFetched > cursor.ReconcileExpected {
		return errors.New("reconciliation counters are invalid")
	}
	if cursor.LastCompleteCount < 0 {
		return errors.New("last complete count is invalid")
	}
	for _, values := range [][]string{
		cursor.BoundaryKeys,
		cursor.ExactRecordKeys,
		cursor.ReconcileSourceKeys,
		cursor.ReconcileRecordKeys,
		cursor.ContinuationKeys,
	} {
		if len(values) > maxCursorExactKeys {
			return errors.New("exact-key count exceeds the limit")
		}
		for _, value := range values {
			if len(value) != 64 || !isLowerHex(value) {
				return errors.New("exact-key value is invalid")
			}
		}
	}
	return nil
}

func cloneLogCursor(src logSourceCursor) logSourceCursor {
	src.BoundaryKeys = append([]string(nil), src.BoundaryKeys...)
	src.ExactRecordKeys = append([]string(nil), src.ExactRecordKeys...)
	src.ReconcileSourceKeys = append([]string(nil), src.ReconcileSourceKeys...)
	src.ReconcileRecordKeys = append([]string(nil), src.ReconcileRecordKeys...)
	src.ContinuationKeys = append([]string(nil), src.ContinuationKeys...)
	return src
}

func boundedSortedKeys(values []string, limit int) []string {
	if len(values) == 0 {
		return nil
	}
	result := append([]string(nil), values...)
	sort.Strings(result)
	result = compactStrings(result)
	if len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func isLowerHex(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func ensureCursorDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create Redfish cursor directory: %w", err)
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("inspect Redfish cursor directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("Redfish cursor path is not a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("Redfish cursor directory permissions %04o are too broad", info.Mode().Perm())
	}
	return nil
}

func (c *cursorCoordinator) cleanupOrphansLocked(now time.Time) {
	if c.cleanup == nil || c.root == "" {
		return
	}
	c.cleanup.sweep(c.root, c.path, now)
}

func (g *cursorCleanupGate) sweep(root, currentPath string, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	state := g.roots[root]
	if !state.nextSweep.IsZero() && now.Before(state.nextSweep) {
		return
	}
	state.nextSweep = now.Add(cursorCleanupInterval)
	state.scans++
	entries, err := os.ReadDir(root)
	if err != nil {
		g.roots[root] = state
		return
	}
	type cleanupCandidate struct {
		name        string
		endpointKey string
		kind        byte
	}
	candidates := make([]cleanupCandidate, 0, len(entries))
	hasState := make(map[string]struct{})
	lockCandidates := make([]cleanupCandidate, 0)
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			continue
		}
		if endpointKey, ok := cursorSnapshotEndpointKey(name); ok {
			hasState[endpointKey] = struct{}{}
			candidates = append(candidates, cleanupCandidate{name: name, endpointKey: endpointKey, kind: 's'})
			continue
		}
		if endpointKey, ok := cursorTemporaryEndpointKey(name); ok {
			hasState[endpointKey] = struct{}{}
			candidates = append(candidates, cleanupCandidate{name: name, endpointKey: endpointKey, kind: 't'})
			continue
		}
		if endpointKey, ok := cursorEndpointLockKey(name); ok {
			lockCandidates = append(lockCandidates, cleanupCandidate{name: name, endpointKey: endpointKey, kind: 'l'})
		}
	}
	for _, candidate := range lockCandidates {
		if _, ok := hasState[candidate.endpointKey]; !ok {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		state.after = ""
		g.roots[root] = state
		return
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].name < candidates[j].name })
	start := sort.Search(len(candidates), func(i int) bool {
		return candidates[i].name > state.after
	})
	if start == len(candidates) {
		start = 0
		state.after = ""
	}
	checked := 0
	for i := start; i < len(candidates) && checked < cursorCleanupBatchSize; i++ {
		candidate := candidates[i]
		name := candidate.name
		state.after = name
		checked++
		path := filepath.Join(root, name)
		if path == currentPath {
			continue
		}
		switch candidate.kind {
		case 't':
			withCursorEndpointLock(root, candidate.endpointKey, func() bool {
				info, err := os.Lstat(path)
				if err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular() &&
					now.Sub(info.ModTime()) > time.Hour {
					if err := os.Remove(path); err == nil || errors.Is(err, os.ErrNotExist) {
						if _, err := os.Lstat(filepath.Join(root, candidate.endpointKey+".cursor")); errors.Is(err, os.ErrNotExist) {
							return true
						}
					}
				}
				return false
			})
		case 's':
			withCursorEndpointLock(root, candidate.endpointKey, func() bool {
				metadata, err := readCursorRetentionMetadata(path)
				if err == nil && metadata.retention > 0 && now.Sub(metadata.modTime) > metadata.retention {
					if err := os.Remove(path); err == nil || errors.Is(err, os.ErrNotExist) {
						return true
					}
				}
				return false
			})
		case 'l':
			withCursorEndpointLock(root, candidate.endpointKey, func() bool {
				if _, err := os.Lstat(filepath.Join(root, candidate.endpointKey+".cursor")); errors.Is(err, os.ErrNotExist) {
					return true
				}
				return false
			})
		}
	}
	if start+checked >= len(candidates) {
		state.after = ""
	}
	g.roots[root] = state
}

func cursorSnapshotEndpointKey(name string) (string, bool) {
	endpointKey, ok := strings.CutSuffix(name, ".cursor")
	return endpointKey, ok && len(endpointKey) == endpointKeyHexChars && isLowerHex(endpointKey)
}

func cursorTemporaryEndpointKey(name string) (string, bool) {
	const prefixBytes = 1 + endpointKeyHexChars
	if len(name) <= prefixBytes+len(".cursor-") || name[0] != '.' ||
		!strings.HasPrefix(name[prefixBytes:], ".cursor-") {
		return "", false
	}
	endpointKey := name[1:prefixBytes]
	return endpointKey, isLowerHex(endpointKey)
}

func cursorEndpointLockKey(name string) (string, bool) {
	suffix := "-" + cursorLockName + cursorLockFileSuffix
	endpointKey, ok := strings.CutSuffix(name, suffix)
	return endpointKey, ok && len(endpointKey) == endpointKeyHexChars && isLowerHex(endpointKey)
}

func cursorEndpointLockPath(root, endpointKey string) string {
	return filepath.Join(root, endpointKey+"-"+cursorLockName+cursorLockFileSuffix)
}

func lockCursorNamespace(ctx context.Context, root string) (*filelock.Locker, error) {
	locker := filelock.New(root)
	for {
		ok, err := locker.Lock(cursorNamespaceLock)
		if err != nil {
			return nil, err
		}
		if ok {
			return locker, nil
		}
		timer := time.NewTimer(cursorLockRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func withCursorEndpointLock(root, endpointKey string, action func() bool) {
	namespace := filelock.New(root)
	ok, err := namespace.Lock(cursorNamespaceLock)
	if err != nil || !ok {
		return
	}
	defer namespace.UnlockAll()
	locker := filelock.New(root)
	ok, err = locker.Lock(endpointKey + "-" + cursorLockName)
	if err != nil || !ok {
		return
	}
	retireLock := action()
	locker.UnlockAll()
	if retireLock {
		// The namespace lock prevents a claimant from opening a replacement inode.
		_ = os.Remove(cursorEndpointLockPath(root, endpointKey))
	}
}

func readCursorRetentionMetadata(path string) (cursorRetentionMetadata, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return cursorRetentionMetadata{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
		info.Size() < int64(cursorHeaderSize) || info.Size() > int64(maxCursorPayload+cursorHeaderSize) {
		return cursorRetentionMetadata{}, errors.New("Redfish cursor is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return cursorRetentionMetadata{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return cursorRetentionMetadata{}, errors.New("Redfish cursor changed during retention inspection")
	}
	header := make([]byte, cursorHeaderSize)
	if _, err := io.ReadFull(file, header); err != nil {
		return cursorRetentionMetadata{}, err
	}
	if string(header[:len(cursorMagic)]) != cursorMagic {
		return cursorRetentionMetadata{}, errors.New("Redfish cursor has invalid magic")
	}
	version := binary.BigEndian.Uint16(header[len(cursorMagic):])
	if version != cursorVersion {
		return cursorRetentionMetadata{}, fmt.Errorf("unsupported Redfish cursor version %d", version)
	}
	size := binary.BigEndian.Uint32(header[cursorSizeOffset:])
	if size > maxCursorPayload || int64(size)+int64(cursorHeaderSize) != info.Size() {
		return cursorRetentionMetadata{}, errors.New("Redfish cursor has an invalid declared length")
	}
	wantMetadataCRC := binary.BigEndian.Uint32(header[cursorMetadataCRCOffset:])
	if crc32.ChecksumIEEE(header[:cursorMetadataCRCOffset]) != wantMetadataCRC {
		return cursorRetentionMetadata{}, errors.New("Redfish cursor metadata checksum mismatch")
	}
	retention := binary.BigEndian.Uint64(header[cursorRetentionOffset:])
	if retention > uint64(1<<63-1) {
		return cursorRetentionMetadata{}, errors.New("Redfish cursor has invalid orphan retention")
	}
	return cursorRetentionMetadata{retention: time.Duration(retention), modTime: opened.ModTime()}, nil
}

func syncCursorDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func netdataStateDir() string {
	if value := strings.TrimSpace(pluginconfig.VarLibDir()); value != "" {
		return filepath.Clean(value)
	}
	if value := strings.TrimSpace(os.Getenv("NETDATA_LIB_DIR")); value != "" {
		return filepath.Clean(value)
	}
	if value := strings.TrimSpace(buildinfo.VarLibDir); value != "" {
		return filepath.Clean(value)
	}
	return filepath.Clean(buildinfo.DefaultVarLibDir)
}
