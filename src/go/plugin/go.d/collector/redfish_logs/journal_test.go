// SPDX-License-Identifier: GPL-3.0-or-later

package redfish_logs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	sdkjournal "github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfishruntime"
)

type fixedJournalHost struct{}

func (fixedJournalHost) MachineID() sdkjournal.UUID {
	return sdkjournal.UUID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
}

func (fixedJournalHost) BootID() sdkjournal.UUID {
	return sdkjournal.UUID{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
}

func (fixedJournalHost) MonotonicUsec() uint64 { return 1234 }

func TestBackendDigest(t *testing.T) {
	t.Parallel()

	keyA, fullA := backendDigest("default")
	keyB, fullB := backendDigest("other")
	assert.Len(t, keyA, 32)
	assert.Len(t, fullA, 64)
	assert.Equal(t, fullA[:32], keyA)
	assert.NotEqual(t, keyA, keyB)
	assert.NotEqual(t, fullA, fullB)
}

func TestPrepareBackendDirectoryAdoptsStableIdentity(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("NETDATA_LOG_DIR", logDir)

	root, dir, key, err := prepareBackendDirectory("default")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(logDir, "redfish"), root)
	assert.Equal(t, filepath.Join(root, key), dir)

	info, err := os.Stat(dir)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Zero(t, info.Mode().Perm()&0o077)
	}

	_, secondDir, secondKey, err := prepareBackendDirectory("default")
	require.NoError(t, err)
	assert.Equal(t, dir, secondDir)
	assert.Equal(t, key, secondKey)
}

func TestPrepareBackendDirectoryRejectsMarkerMismatch(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("NETDATA_LOG_DIR", logDir)

	_, dir, _, err := prepareBackendDirectory("default")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, backendMarkerName), []byte(`{"version":1,"name":"other","digest":"bad"}`), 0o600))

	_, _, _, err = prepareBackendDirectory("default")
	require.ErrorContains(t, err, "does not match")
}

func TestEnsurePrivateDirectoryRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require additional privileges on Windows")
	}

	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	require.NoError(t, os.Mkdir(target, 0o700))
	link := filepath.Join(parent, "link")
	require.NoError(t, os.Symlink(target, link))
	require.ErrorContains(t, ensurePrivateDirectory(link), "not a real directory")
}

func TestJournalBackendAppendIsSynchronous(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "backend")
	require.NoError(t, os.Mkdir(dir, 0o700))

	backend, err := newJournalBackend(dir, 20<<20, fixedJournalHost{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = backend.Close() })

	entry := redfishruntime.JournalEntry{
		RealtimeUsec:       1_700_000_000_000_000,
		SourceRealtimeUsec: 1_699_999_999_000_000,
		Fields: map[string]string{
			"MESSAGE":            "test entry",
			"ND_LOG_SOURCE":      "redfish",
			"REDFISH_RECORD_KEY": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
	}
	result, err := backend.Append(context.Background(), []redfishruntime.JournalEntry{entry})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Committed)
	assert.EqualValues(t, 1, backend.received.Load())
	assert.EqualValues(t, 1, backend.committed.Load())

	stats, err := scanJournalDirectory(backend.dir)
	require.NoError(t, err)
	assert.Positive(t, stats.active)
	assert.Positive(t, stats.bytes)

	result, err = backend.Append(context.Background(), []redfishruntime.JournalEntry{entry, entry})
	require.NoError(t, err)
	assert.Zero(t, result.Committed)
	assert.Equal(t, 2, result.DuplicateSuppressed)
	assert.EqualValues(t, 2, backend.duplicates.Load())
}

func TestJournalBackendContainsClassifiesSeveralRecordKeys(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "backend")
	require.NoError(t, os.Mkdir(dir, 0o700))

	backend, err := newJournalBackend(dir, 20<<20, fixedJournalHost{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = backend.Close() })

	first := "1111111111111111111111111111111111111111111111111111111111111111"
	second := "2222222222222222222222222222222222222222222222222222222222222222"
	missing := "3333333333333333333333333333333333333333333333333333333333333333"
	_, err = backend.Append(context.Background(), []redfishruntime.JournalEntry{
		faultJournalEntry(first),
		faultJournalEntry(second),
	})
	require.NoError(t, err)

	retained, err := backend.Contains(context.Background(), []string{first, second, missing})
	require.NoError(t, err)
	assert.True(t, retained[first])
	assert.True(t, retained[second])
	assert.False(t, retained[missing])
}

type faultJournalState struct {
	visible       map[string]struct{}
	durable       map[string]struct{}
	openCount     int
	failAppendAt  int
	failFirstSync bool
}

type faultJournalWriter struct {
	state       *faultJournalState
	appendCount int
	index       int
}

func (w *faultJournalWriter) Append(fields []sdkjournal.Field, _ sdkjournal.EntryOptions) error {
	w.appendCount++
	if w.index == 1 && w.state.failAppendAt > 0 && w.appendCount == w.state.failAppendAt {
		return errors.New("injected append failure")
	}
	for _, field := range fields {
		if field.Name == "REDFISH_RECORD_KEY" {
			w.state.visible[string(field.Value)] = struct{}{}
		}
	}
	return nil
}

func (w *faultJournalWriter) Sync() error {
	if w.index == 1 && w.state.failFirstSync {
		w.state.failFirstSync = false
		return errors.New("injected sync failure")
	}
	for key := range w.state.visible {
		w.state.durable[key] = struct{}{}
	}
	return nil
}

func (*faultJournalWriter) EnforceRetention() error  { return nil }
func (*faultJournalWriter) Close() error             { return nil }
func (*faultJournalWriter) JournalDirectory() string { return "fault-journal" }

func newFaultJournalBackend(state *faultJournalState) *journalBackend {
	state.visible = make(map[string]struct{})
	state.durable = make(map[string]struct{})
	open := func() (journalWriter, error) {
		state.openCount++
		return &faultJournalWriter{state: state, index: state.openCount}, nil
	}
	writer, _ := open()
	backend := &journalBackend{
		log:        writer,
		openWriter: open,
		host:       fixedJournalHost{},
		dir:        "fault-journal",
	}
	backend.retainedLookup = func(keys []string) (map[string]struct{}, error) {
		result := make(map[string]struct{})
		for _, key := range keys {
			if _, ok := state.durable[key]; ok {
				result[key] = struct{}{}
			}
		}
		return result, nil
	}
	return backend
}

func faultJournalEntry(key string) redfishruntime.JournalEntry {
	return redfishruntime.JournalEntry{
		Fields: map[string]string{
			"MESSAGE":            "fault test",
			"REDFISH_RECORD_KEY": key,
		},
	}
}

func TestJournalBackendQuarantinesPartialAppendBeforeDuplicateClassification(t *testing.T) {
	state := &faultJournalState{failAppendAt: 2}
	backend := newFaultJournalBackend(state)
	entries := []redfishruntime.JournalEntry{
		faultJournalEntry("1111111111111111111111111111111111111111111111111111111111111111"),
		faultJournalEntry("2222222222222222222222222222222222222222222222222222222222222222"),
	}

	_, err := backend.Append(context.Background(), entries)
	require.ErrorContains(t, err, "injected append failure")
	require.Equal(t, 2, state.openCount, "an uncertain writer must be replaced")

	result, err := backend.Append(context.Background(), entries)
	require.NoError(t, err)
	require.Equal(t, 1, result.Committed)
	require.Equal(t, 1, result.DuplicateSuppressed)
}

func TestJournalBackendSynchronizesUncertainWriterBeforeDuplicateClassification(t *testing.T) {
	state := &faultJournalState{failFirstSync: true}
	backend := newFaultJournalBackend(state)
	entries := []redfishruntime.JournalEntry{
		faultJournalEntry("3333333333333333333333333333333333333333333333333333333333333333"),
		faultJournalEntry("4444444444444444444444444444444444444444444444444444444444444444"),
	}

	_, err := backend.Append(context.Background(), entries)
	require.ErrorContains(t, err, "injected sync failure")
	require.Equal(t, 2, state.openCount, "an uncertain writer must be replaced")

	result, err := backend.Append(context.Background(), entries)
	require.NoError(t, err)
	require.Zero(t, result.Committed)
	require.Equal(t, 2, result.DuplicateSuppressed)
}
