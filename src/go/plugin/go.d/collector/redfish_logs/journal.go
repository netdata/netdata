// SPDX-License-Identifier: GPL-3.0-or-later

package redfish_logs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sdkjournal "github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/netdata/systemd-journal-sdk/go/journalhost"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/framework/filelock"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfishruntime"
)

const (
	backendMarkerName  = "identity.json"
	backendMarkerLimit = 2048
	backendLockName    = "writer"
)

type journalBackend struct {
	mu sync.Mutex

	log            journalWriter
	openWriter     func() (journalWriter, error)
	retainedLookup func([]string) (map[string]struct{}, error)
	host           journalHost
	dir            string
	maxBytes       uint64

	received          atomic.Uint64
	committed         atomic.Uint64
	duplicates        atomic.Uint64
	writeFailed       atomic.Uint64
	retentionFiles    atomic.Uint64
	retentionBytes    atomic.Uint64
	retentionFailed   atomic.Uint64
	syncDurationNanos atomic.Int64
}

type journalHost interface {
	MachineID() sdkjournal.UUID
	BootID() sdkjournal.UUID
	MonotonicUsec() uint64
}

type journalWriter interface {
	Append([]sdkjournal.Field, sdkjournal.EntryOptions) error
	Sync() error
	EnforceRetention() error
	Close() error
	JournalDirectory() string
}

type backendMarker struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
	Digest  string `json:"digest"`
}

func backendDigest(name string) (key, full string) {
	digest := sha256.Sum256([]byte("netdata:redfish:backend:v1\x00" + name))
	full = hex.EncodeToString(digest[:])
	return full[:32], full
}

func newJournalBackend(dir string, maxBytes uint64, host journalHost) (*journalBackend, error) {
	if host == nil {
		return nil, errors.New("nil journal host identity")
	}
	rotationSize := min(max(maxBytes/20, uint64(5<<20)), uint64(200<<20))
	config := sdkjournal.LogConfig{
		Source: "redfish",
		Options: sdkjournal.Options{
			MachineID:   host.MachineID(),
			BootID:      host.BootID(),
			Compact:     true,
			Compression: sdkjournal.CompressionNone,
			FileMode:    sdkjournal.JournalFileMode(0o600),
		},
		RotationPolicy:  sdkjournal.RotationPolicy{}.WithMaxFileSize(rotationSize),
		RetentionPolicy: sdkjournal.RetentionPolicy{}.WithMaxBytes(maxBytes),
		OpenMode:        sdkjournal.LogOpenEager,
		IdentityMode:    sdkjournal.LogIdentityStrict,
	}
	openWriter := func() (journalWriter, error) {
		return sdkjournal.NewLog(dir, config)
	}
	log, err := openWriter()
	if err != nil {
		return nil, fmt.Errorf("open Redfish journal: %w", err)
	}
	backend := &journalBackend{
		log:        log,
		openWriter: openWriter,
		host:       host,
		dir:        log.JournalDirectory(),
		maxBytes:   maxBytes,
	}
	backend.retainedLookup = func(keys []string) (map[string]struct{}, error) {
		return retainedRecordKeys(backend.dir, keys)
	}
	return backend, nil
}

func (b *journalBackend) Append(
	ctx context.Context,
	entries []redfishruntime.JournalEntry,
) (redfishruntime.AppendResult, error) {
	if len(entries) == 0 {
		return redfishruntime.AppendResult{}, nil
	}
	b.received.Add(uint64(len(entries)))

	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return redfishruntime.AppendResult{}, err
	}
	if b.log == nil {
		if err := b.reopenAndSyncLocked(); err != nil {
			b.writeFailed.Add(uint64(len(entries)))
			return redfishruntime.AppendResult{}, err
		}
	}

	recordKeys := make([]string, 0, len(entries))
	for _, entry := range entries {
		key := entry.Fields["REDFISH_RECORD_KEY"]
		if len(key) != 64 || !isLowerHex(key) {
			b.writeFailed.Add(uint64(len(entries)))
			return redfishruntime.AppendResult{}, errors.New("Redfish journal entry has no valid record key")
		}
		recordKeys = append(recordKeys, key)
	}
	retained, err := b.retainedLookup(recordKeys)
	if err != nil {
		b.writeFailed.Add(uint64(len(entries)))
		return redfishruntime.AppendResult{}, fmt.Errorf("query retained Redfish record keys: %w", err)
	}
	seen := make(map[string]struct{}, len(retained)+len(entries))
	for key := range retained {
		seen[key] = struct{}{}
	}
	pending := make([]redfishruntime.JournalEntry, 0, len(entries))
	duplicates := 0
	for index, entry := range entries {
		key := recordKeys[index]
		if _, ok := seen[key]; ok {
			duplicates++
			continue
		}
		seen[key] = struct{}{}
		pending = append(pending, entry)
	}
	if len(pending) == 0 {
		b.duplicates.Add(uint64(duplicates))
		return redfishruntime.AppendResult{DuplicateSuppressed: duplicates}, nil
	}

	before, _ := scanJournalDirectory(b.dir)
	for i, entry := range pending {
		fields := make([]sdkjournal.Field, 0, len(entry.Fields))
		names := make([]string, 0, len(entry.Fields))
		for name := range entry.Fields {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			fields = append(fields, sdkjournal.StringField(name, entry.Fields[name]))
		}
		options := sdkjournal.EntryOptions{
			RealtimeUsec:       entry.RealtimeUsec,
			RealtimeUsecSet:    true,
			MonotonicUsec:      b.host.MonotonicUsec(),
			MonotonicUsecSet:   true,
			BootID:             b.host.BootID(),
			SourceRealtimeUsec: entry.SourceRealtimeUsec,
		}
		if err := b.log.Append(fields, options); err != nil {
			b.writeFailed.Add(uint64(len(pending) - i))
			recoveryErr := b.reopenAndSyncLocked()
			return redfishruntime.AppendResult{}, errors.Join(err, recoveryErr)
		}
	}
	started := time.Now()
	if err := b.log.Sync(); err != nil {
		b.writeFailed.Add(uint64(len(pending)))
		recoveryErr := b.reopenAndSyncLocked()
		return redfishruntime.AppendResult{}, errors.Join(err, recoveryErr)
	}
	b.syncDurationNanos.Store(time.Since(started).Nanoseconds())
	if err := b.log.EnforceRetention(); err != nil {
		b.retentionFailed.Add(1)
		b.committed.Add(uint64(len(pending)))
		b.duplicates.Add(uint64(duplicates))
		return redfishruntime.AppendResult{
			Committed: len(pending), DuplicateSuppressed: duplicates,
		}, nil
	}
	after, _ := scanJournalDirectory(b.dir)
	if before.files > after.files {
		b.retentionFiles.Add(uint64(before.files - after.files))
	}
	if before.bytes > after.bytes {
		b.retentionBytes.Add(before.bytes - after.bytes)
	}
	b.committed.Add(uint64(len(pending)))
	b.duplicates.Add(uint64(duplicates))
	return redfishruntime.AppendResult{
		Committed: len(pending), DuplicateSuppressed: duplicates,
	}, nil
}

func (b *journalBackend) Contains(
	ctx context.Context,
	recordKeys []string,
) (map[string]bool, error) {
	result := make(map[string]bool, len(recordKeys))
	if len(recordKeys) == 0 {
		return result, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b.log == nil {
		if err := b.reopenAndSyncLocked(); err != nil {
			return nil, err
		}
	}
	if err := b.log.Sync(); err != nil {
		return nil, errors.Join(err, b.reopenAndSyncLocked())
	}
	for _, key := range recordKeys {
		if len(key) != 64 || !isLowerHex(key) {
			return nil, errors.New("invalid Redfish record key lookup")
		}
	}
	retained, err := b.retainedLookup(recordKeys)
	if err != nil {
		return nil, err
	}
	for _, key := range recordKeys {
		result[key] = false
	}
	for key := range retained {
		result[key] = true
	}
	return result, nil
}

func (b *journalBackend) reopenAndSyncLocked() error {
	var result error
	if b.log != nil {
		result = errors.Join(result, b.log.Close())
		b.log = nil
	}
	if b.openWriter == nil {
		return errors.Join(result, sdkjournal.ErrWriterClosed)
	}
	writer, err := b.openWriter()
	if err != nil {
		return errors.Join(result, fmt.Errorf("reopen Redfish journal after uncertain write: %w", err))
	}
	if err := writer.Sync(); err != nil {
		result = errors.Join(result, fmt.Errorf("sync reopened Redfish journal: %w", err))
		result = errors.Join(result, writer.Close())
		return result
	}
	b.log = writer
	b.dir = writer.JournalDirectory()
	return result
}

func retainedRecordKeys(dir string, recordKeys []string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	if len(recordKeys) == 0 {
		return result, nil
	}
	reader, err := sdkjournal.OpenDirectory(dir)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	unique := make(map[string]struct{}, len(recordKeys))
	for _, key := range recordKeys {
		if _, ok := unique[key]; ok {
			continue
		}
		reader.AddMatch([]byte("REDFISH_RECORD_KEY=" + key))
		unique[key] = struct{}{}
	}
	if err := reader.SeekHead(); err != nil {
		return nil, err
	}
	for {
		ok, err := reader.Step()
		if err != nil {
			return nil, err
		}
		if !ok {
			return result, nil
		}
		entry, err := reader.GetEntry()
		if err != nil {
			return nil, err
		}
		if value := string(entry.Fields["REDFISH_RECORD_KEY"]); value != "" {
			result[value] = struct{}{}
		}
	}
}

func isLowerHex(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func (b *journalBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.log == nil {
		return nil
	}
	err := b.log.Close()
	b.log = nil
	return err
}

type directoryStats struct {
	bytes    uint64
	files    int
	active   int
	archived int
}

func scanJournalDirectory(dir string) (directoryStats, error) {
	var stats directoryStats
	var firstInspectionError error
	inspectionFailures := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		return stats, err
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() ||
			(!strings.HasSuffix(entry.Name(), ".journal") && !strings.HasSuffix(entry.Name(), ".journal~")) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			inspectionFailures++
			if firstInspectionError == nil {
				firstInspectionError = fmt.Errorf("inspect journal file %q: %w", entry.Name(), err)
			}
			continue
		}
		stats.files++
		stats.bytes += uint64(max(info.Size(), 0))
		if strings.HasSuffix(entry.Name(), ".journal~") {
			stats.archived++
		} else {
			stats.active++
		}
	}
	if inspectionFailures > 0 {
		return stats, fmt.Errorf(
			"%d journal files could not be inspected; first failure: %w",
			inspectionFailures,
			firstInspectionError,
		)
	}
	return stats, nil
}

func prepareBackendDirectory(jobName string) (root, dir, key string, err error) {
	logDir := netdataLogDir()
	info, err := os.Stat(logDir)
	if err != nil {
		return "", "", "", fmt.Errorf("stat Netdata log directory: %w", err)
	}
	if !info.IsDir() {
		return "", "", "", errors.New("Netdata log path is not a directory")
	}

	root = filepath.Join(logDir, "redfish")
	if err := ensurePrivateDirectory(root); err != nil {
		return "", "", "", err
	}
	key, full := backendDigest(jobName)
	dir = filepath.Join(root, key)
	if err := ensurePrivateDirectory(dir); err != nil {
		return "", "", "", err
	}
	if err := adoptBackendMarker(dir, backendMarker{
		Version: 1,
		Name:    jobName,
		Digest:  full,
	}); err != nil {
		return "", "", "", err
	}
	return root, dir, key, nil
}

func ensurePrivateDirectory(dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(dir, 0o700); err != nil {
			return fmt.Errorf("create private Redfish directory: %w", err)
		}
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return fmt.Errorf("inspect private Redfish directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("private Redfish path is not a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("private Redfish directory permissions %04o are too broad", info.Mode().Perm())
	}
	opened, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open private Redfish directory: %w", err)
	}
	openedInfo, statErr := opened.Stat()
	closeErr := opened.Close()
	if statErr != nil || !os.SameFile(info, openedInfo) || !openedInfo.IsDir() {
		return errors.New("private Redfish directory changed during validation")
	}
	if closeErr != nil {
		return fmt.Errorf("close private Redfish directory: %w", closeErr)
	}
	return nil
}

func adoptBackendMarker(dir string, want backendMarker) error {
	path := filepath.Join(dir, backendMarkerName)
	payload, err := json.Marshal(want)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if len(payload) > backendMarkerLimit {
		return errors.New("Redfish backend identity marker is too large")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if _, writeErr := file.Write(payload); writeErr != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return fmt.Errorf("write Redfish backend marker: %w", writeErr)
		}
		if syncErr := file.Sync(); syncErr != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return fmt.Errorf("sync Redfish backend marker: %w", syncErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			_ = os.Remove(path)
			return fmt.Errorf("close Redfish backend marker: %w", closeErr)
		}
		if syncErr := syncDirectory(dir); syncErr != nil {
			_ = os.Remove(path)
			return fmt.Errorf("sync Redfish backend directory: %w", syncErr)
		}
		return nil
	}
	if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create Redfish backend marker: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("Redfish backend identity marker is not a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("Redfish backend identity marker permissions %04o are too broad", info.Mode().Perm())
	}
	if info.Size() <= 0 || info.Size() > backendMarkerLimit {
		return errors.New("Redfish backend identity marker has invalid size")
	}
	existingFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open Redfish backend marker: %w", err)
	}
	defer existingFile.Close()
	openedInfo, err := existingFile.Stat()
	if err != nil || !os.SameFile(info, openedInfo) || !openedInfo.Mode().IsRegular() {
		return errors.New("Redfish backend identity marker changed during validation")
	}
	existing, err := io.ReadAll(io.LimitReader(existingFile, backendMarkerLimit+1))
	if err != nil {
		return fmt.Errorf("read Redfish backend marker: %w", err)
	}
	if len(existing) > backendMarkerLimit {
		return errors.New("Redfish backend identity marker is too large")
	}
	var got backendMarker
	if err := json.Unmarshal(existing, &got); err != nil {
		return fmt.Errorf("decode Redfish backend marker: %w", err)
	}
	if got != want {
		return errors.New("Redfish backend identity marker does not match configured job")
	}
	return nil
}

func syncDirectory(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func acquireBackendLock(dir string) (*filelock.Locker, error) {
	locker := filelock.New(dir)
	ok, err := locker.Lock(backendLockName)
	if err != nil {
		return nil, fmt.Errorf("lock Redfish backend: %w", err)
	}
	if !ok {
		return nil, errors.New("Redfish backend already has an active writer")
	}
	return locker, nil
}

func loadJournalHost() (journalHost, error) {
	stateDir := filepath.Join(netdataLibDir(), "systemd-journal-sdk")
	hostPrefix := strings.TrimSpace(pluginconfig.HostPrefix())
	if hostPrefix != "" {
		hostPrefix = filepath.Clean(hostPrefix)
	}
	host, err := journalhost.Load(journalhost.LoadOptions{
		StateDir:             stateDir,
		HostFilesystemPrefix: hostPrefix,
	})
	if err != nil {
		return nil, fmt.Errorf("load journal host identity: %w", err)
	}
	return host, nil
}

func netdataLogDir() string {
	if value := strings.TrimSpace(os.Getenv("NETDATA_LOG_DIR")); value != "" {
		return filepath.Clean(value)
	}
	if value := strings.TrimSpace(buildinfo.LogDir); value != "" {
		return filepath.Clean(value)
	}
	return "/var/log/netdata"
}

func netdataLibDir() string {
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
