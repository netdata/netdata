// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/journaltest"
	sdkjournal "github.com/netdata/systemd-journal-sdk/go/journal"
)

const (
	testJournalCompatibleSealed       = 1 << 0
	testJournalIncompatibleCompact    = 1 << 4
	testJournalIncompatibleCompressed = (1 << 0) | (1 << 1) | (1 << 3)
)

type staticHostProvider struct {
	machineID sdkjournal.UUID
	bootID    sdkjournal.UUID
	nextMono  atomic.Uint64
}

func newTestHostProvider() *staticHostProvider {
	machineID, err := sdkjournal.ParseUUID("00112233445566778899aabbccddeeff")
	if err != nil {
		panic(err)
	}
	bootID, err := sdkjournal.ParseUUID("ffeeddccbbaa99887766554433221100")
	if err != nil {
		panic(err)
	}
	p := &staticHostProvider{machineID: machineID, bootID: bootID}
	p.nextMono.Store(999)
	return p
}

func (p *staticHostProvider) MachineID() sdkjournal.UUID { return p.machineID }
func (p *staticHostProvider) BootID() sdkjournal.UUID    { return p.bootID }
func (p *staticHostProvider) MonotonicUsec() uint64      { return p.nextMono.Add(1) }

func newTestSDKWriter(dir string, cfg Config) (*sdkWriter, error) {
	return newSDKWriter(dir, cfg, newTestHostProvider())
}

func writeTestFields(w *sdkWriter, fields []decodedField, realtimeUsec, monotonicUsec int64) error {
	payloads := make([][]byte, 0, len(fields))
	var binaryFields int
	for _, field := range fields {
		payload := make([]byte, 0, len(field.Name)+1+len(field.Value))
		payload = append(payload, field.Name...)
		payload = append(payload, '=')
		payload = append(payload, field.Value...)
		payloads = append(payloads, payload)
		if journalFieldNeedsBinary(field.Value) {
			binaryFields++
		}
	}
	return w.writeRaw(payloads, binaryFields, realtimeUsec, monotonicUsec)
}

func TestNewSDKWriterEagerOpenCreatesSDKJournalDirectory(t *testing.T) {
	dir := t.TempDir()
	w, err := newTestSDKWriter(dir, Config{RotateSize: 200 * bytesPerMB})
	require.NoError(t, err)
	defer w.close()

	journalDir := w.log.JournalDirectory()
	activePath := w.log.ActivePath()
	assert.NotEmpty(t, journalDir)
	assert.NotEqual(t, filepath.Clean(dir), filepath.Clean(journalDir))
	assert.DirExists(t, journalDir)
	assert.NotEmpty(t, activePath)
	assert.FileExists(t, activePath)
	assert.True(t, strings.HasPrefix(filepath.Base(activePath), "snmp-traps@"), "active journal path = %s", activePath)
}

func TestNewSDKWriterCreatesCompactUnsealedUncompressedJournal(t *testing.T) {
	dir := t.TempDir()
	w, err := newTestSDKWriter(dir, Config{RotateSize: 200 * bytesPerMB})
	require.NoError(t, err)

	fields := []decodedField{
		{Name: "MESSAGE", Value: []byte("sdk compact flag entry")},
		{Name: "SYSLOG_IDENTIFIER", Value: []byte("sdk-test")},
	}
	now := time.Now().UnixMicro()
	require.NoError(t, writeTestFields(w, fields, now, now))
	activePath := w.log.ActivePath()
	require.NoError(t, w.close())

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

func TestSDKWriterWriteAndQueryWithJournalctl(t *testing.T) {
	journaltest.RequireJournalctl(t)

	dir := t.TempDir()
	w, err := newTestSDKWriter(dir, Config{RotateSize: 200 * bytesPerMB})
	require.NoError(t, err)

	fields := []decodedField{
		{Name: "MESSAGE", Value: []byte("sdk trap entry")},
		{Name: "PRIORITY", Value: []byte("4")},
		{Name: "SYSLOG_IDENTIFIER", Value: []byte("sdk-test")},
		{Name: "TRAP_REPORT_TYPE", Value: []byte("trap")},
		{Name: "TRAP_OID", Value: []byte("1.3.6.1.6.3.1.1.5.1")},
		{Name: "TRAP_CATEGORY", Value: []byte("security")},
	}
	now := time.Now().UnixMicro()
	require.NoError(t, writeTestFields(w, fields, now, now))
	require.NoError(t, w.sync())

	out := journaltest.RunJournalctl(t, dir, "TRAP_CATEGORY=security")
	assert.Contains(t, out, "sdk trap entry")
	assert.Contains(t, out, "TRAP_CATEGORY")

	require.NoError(t, w.close())
}

func TestSDKWriterCWE117InjectionNotQueryableAsField(t *testing.T) {
	journaltest.RequireJournalctl(t)

	dir := t.TempDir()
	w, err := newTestSDKWriter(dir, Config{RotateSize: 200 * bytesPerMB})
	require.NoError(t, err)

	fields := []decodedField{
		{Name: "MESSAGE", Value: []byte("real_value\nFAKE_FIELD=spoofed")},
		{Name: "PRIORITY", Value: []byte("4")},
		{Name: "SYSLOG_IDENTIFIER", Value: []byte("sdk-test")},
		{Name: "TRAP_CATEGORY", Value: []byte("security")},
	}
	now := time.Now().UnixMicro()
	require.NoError(t, writeTestFields(w, fields, now, now))
	require.NoError(t, w.close())

	out := journaltest.RunJournalctlAllowEmpty(t, dir, "FAKE_FIELD=spoofed")
	assert.Empty(t, strings.TrimSpace(out))
}

func TestSDKWriterCountsBinaryEncodedFields(t *testing.T) {
	dir := t.TempDir()
	w, err := newTestSDKWriter(dir, Config{RotateSize: 200 * bytesPerMB})
	require.NoError(t, err)
	defer w.close()

	fields := []decodedField{
		{Name: "MESSAGE", Value: []byte("hello\nworld")},
		{Name: "PRIORITY", Value: []byte("4")},
	}
	require.NoError(t, writeTestFields(w, fields, 1000, 1000))
	assert.Equal(t, uint64(1), w.binaryFieldCount())
}

func TestWriterCloseReturnsWorkerFailure(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{RotateSize: 200 * bytesPerMB}
	w, err := newTestSDKWriter(dir, cfg)
	require.NoError(t, err)

	tw := newWriter(w, cfg, 10, nil)
	require.NoError(t, tw.Start())
	require.NoError(t, tw.Write(nil))

	firstErr := tw.Close()
	require.ErrorIs(t, firstErr, errNilEntry)

	secondErr := tw.Close()
	require.ErrorIs(t, secondErr, errNilEntry)
}
