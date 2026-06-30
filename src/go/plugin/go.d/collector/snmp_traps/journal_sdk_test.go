// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdkjournal "github.com/netdata/systemd-journal-sdk/go/journal"
)

const (
	testJournalCompatibleSealed       = 1 << 0
	testJournalIncompatibleCompact    = 1 << 4
	testJournalIncompatibleCompressed = (1 << 0) | (1 << 1) | (1 << 3)
)

func TestNewJournalWriterEagerOpenCreatesSDKJournalDirectory(t *testing.T) {
	dir := t.TempDir()
	w, err := newTestJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
	require.NoError(t, err)
	defer w.Close()

	assert.NotEmpty(t, w.JournalDirectory())
	assert.NotEqual(t, filepath.Clean(dir), filepath.Clean(w.JournalDirectory()))
	assert.DirExists(t, w.JournalDirectory())
	assert.NotEmpty(t, w.ActivePath())
	assert.FileExists(t, w.ActivePath())
	assert.True(t, strings.HasPrefix(filepath.Base(w.ActivePath()), "snmp-traps@"), "active journal path = %s", w.ActivePath())
}

func TestNewJournalWriterCreatesCompactUnsealedUncompressedJournal(t *testing.T) {
	dir := t.TempDir()
	w, err := newTestJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
	require.NoError(t, err)

	fields := []JournalField{
		{Name: "MESSAGE", Value: []byte("sdk compact flag entry")},
		{Name: "SYSLOG_IDENTIFIER", Value: []byte("sdk-test")},
	}
	now := time.Now().UnixMicro()
	require.NoError(t, w.WriteEntry(fields, now, now))
	activePath := w.ActivePath()
	require.NoError(t, w.Close())

	r, err := sdkjournal.OpenFileWithOptions(activePath, sdkjournal.ReaderOptions{})
	require.NoError(t, err)
	defer r.Close()

	header := r.Header()
	assert.NotZero(t, header.IncompatibleFlags()&testJournalIncompatibleCompact)
	assert.Zero(t, header.IncompatibleFlags()&testJournalIncompatibleCompressed)
	assert.Zero(t, header.CompatibleFlags()&testJournalCompatibleSealed)
	assert.Equal(t, w.host.MachineID(), header.MachineID())
	assert.Equal(t, w.host.BootID(), header.TailEntryBootID())
	assert.Equal(t, uint64(1), header.NEntries())
}

func TestJournalWriterWriteAndQueryWithJournalctl(t *testing.T) {
	requireJournalctl(t)

	dir := t.TempDir()
	w, err := newTestJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
	require.NoError(t, err)

	fields := []JournalField{
		{Name: "MESSAGE", Value: []byte("sdk trap entry")},
		{Name: "PRIORITY", Value: []byte("4")},
		{Name: "SYSLOG_IDENTIFIER", Value: []byte("sdk-test")},
		{Name: "TRAP_REPORT_TYPE", Value: []byte("trap")},
		{Name: "TRAP_OID", Value: []byte("1.3.6.1.6.3.1.1.5.1")},
		{Name: "TRAP_CATEGORY", Value: []byte("security")},
	}
	now := time.Now().UnixMicro()
	require.NoError(t, w.WriteEntry(fields, now, now))
	require.NoError(t, w.Sync())

	out := runJournalctl(t, w.JournalDirectory(), "TRAP_CATEGORY=security")
	assert.Contains(t, out, "sdk trap entry")
	assert.Contains(t, out, "TRAP_CATEGORY")

	require.NoError(t, w.Close())
}

func TestJournalWriterCWE117InjectionNotQueryableAsField(t *testing.T) {
	requireJournalctl(t)

	dir := t.TempDir()
	w, err := newTestJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
	require.NoError(t, err)

	fields := []JournalField{
		{Name: "MESSAGE", Value: []byte("real_value\nFAKE_FIELD=spoofed")},
		{Name: "PRIORITY", Value: []byte("4")},
		{Name: "SYSLOG_IDENTIFIER", Value: []byte("sdk-test")},
		{Name: "TRAP_CATEGORY", Value: []byte("security")},
	}
	now := time.Now().UnixMicro()
	require.NoError(t, w.WriteEntry(fields, now, now))
	journalDir := w.JournalDirectory()
	require.NoError(t, w.Close())

	out := runJournalctlAllowEmpty(t, journalDir, "FAKE_FIELD=spoofed")
	assert.Empty(t, strings.TrimSpace(out))
}

func TestJournalWriterCountsBinaryEncodedFields(t *testing.T) {
	dir := t.TempDir()
	w, err := newTestJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
	require.NoError(t, err)
	defer w.Close()

	fields := []JournalField{
		{Name: "MESSAGE", Value: []byte("hello\nworld")},
		{Name: "PRIORITY", Value: []byte("4")},
	}
	require.NoError(t, w.WriteEntry(fields, 1000, 1000))
	assert.Equal(t, uint64(0), w.BinaryEncodedFields())
}

func TestJournalTrapWriterCloseReturnsWorkerFailure(t *testing.T) {
	dir := t.TempDir()
	w, err := newTestJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
	require.NoError(t, err)

	tw := newJournalTrapWriter(w, 10)
	require.NoError(t, tw.Write(nil))

	firstErr := tw.Close()
	require.ErrorIs(t, firstErr, errNilEntry)

	secondErr := tw.Close()
	require.ErrorIs(t, secondErr, errNilEntry)
}

func requireJournalctl(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("journalctl"); err != nil {
		t.Skip("journalctl not found")
	}
}

func runJournalctl(t *testing.T, dir, match string) string {
	t.Helper()
	out := runJournalctlAllowEmpty(t, dir, match)
	if strings.TrimSpace(out) == "" {
		t.Fatalf("journalctl returned empty output for %s in %s", match, dir)
	}
	return out
}

func runJournalctlAllowEmpty(t *testing.T, dir, match string) string {
	t.Helper()
	cmd := exec.Command("journalctl", "--directory="+dir, match, "-o", "json", "--no-pager")
	out, err := cmd.CombinedOutput()
	if err != nil && strings.TrimSpace(string(out)) != "" {
		t.Fatalf("journalctl failed: %v\n%s", err, string(out))
	}
	return string(out)
}
