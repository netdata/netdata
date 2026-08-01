// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	sdkjournal "github.com/netdata/systemd-journal-sdk/go/journal"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/hostidentity"
)

type staticJournalHostProvider struct {
	machineID sdkjournal.UUID
	bootID    sdkjournal.UUID
	nextMono  atomic.Uint64
}

func newTestJournalHostProvider() *staticJournalHostProvider {
	machineID, err := sdkjournal.ParseUUID("00112233445566778899aabbccddeeff")
	if err != nil {
		panic(err)
	}
	bootID, err := sdkjournal.ParseUUID("ffeeddccbbaa99887766554433221100")
	if err != nil {
		panic(err)
	}
	provider := &staticJournalHostProvider{
		machineID: machineID,
		bootID:    bootID,
	}
	provider.nextMono.Store(999)
	return provider
}

func (p *staticJournalHostProvider) MachineID() sdkjournal.UUID {
	return p.machineID
}

func (p *staticJournalHostProvider) BootID() sdkjournal.UUID {
	return p.bootID
}

func (p *staticJournalHostProvider) MonotonicUsec() uint64 {
	return p.nextMono.Add(1)
}

func newTestJournalWriter(dir string, cfg JournalConfig) (*JournalWriter, error) {
	return newJournalWriterWithHostProvider(dir, cfg, newTestJournalHostProvider())
}

func withTestBuildinfoVarLibDir(t *testing.T, dir string) {
	t.Helper()
	old := buildinfo.VarLibDir
	buildinfo.VarLibDir = dir
	t.Cleanup(func() {
		buildinfo.VarLibDir = old
	})
}

func TestNetdataJournalHostStateDirUsesNetdataLibDir(t *testing.T) {
	if pluginconfig.VarLibDir() != "" {
		t.Skip("pluginconfig VarLibDir is already initialized")
	}

	libDir := filepath.Join(t.TempDir(), "varlib")
	t.Setenv(netdataLibDirEnv, libDir)
	withTestBuildinfoVarLibDir(t, "/opt/netdata/var/lib/netdata")

	want := filepath.Join(libDir, journalHostStateDirName)
	if got := netdataJournalHostStateDir(); got != want {
		t.Fatalf("netdataJournalHostStateDir() = %q, want %q", got, want)
	}
}

func TestNetdataJournalHostStateDirUsesBuildinfoVarLibDir(t *testing.T) {
	if pluginconfig.VarLibDir() != "" {
		t.Skip("pluginconfig VarLibDir is already initialized")
	}

	t.Setenv(netdataLibDirEnv, "")
	libDir := filepath.Join(t.TempDir(), "opt", "netdata", "var", "lib", "netdata")
	withTestBuildinfoVarLibDir(t, libDir)

	want := filepath.Join(libDir, journalHostStateDirName)
	if got := netdataJournalHostStateDir(); got != want {
		t.Fatalf("netdataJournalHostStateDir() = %q, want %q", got, want)
	}
}

func TestNetdataHostFilesystemPrefixUsesNetdataHostPrefix(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "host", "..", "host")
	t.Setenv("NETDATA_HOST_PREFIX", prefix)

	want := filepath.Clean(os.Getenv("NETDATA_HOST_PREFIX"))
	if got := netdataHostFilesystemPrefix(); got != want {
		t.Fatalf("netdataHostFilesystemPrefix() = %q, want %q", got, want)
	}
}

func TestNetdataHostFilesystemPrefixIgnoresEmptyNetdataHostPrefix(t *testing.T) {
	t.Setenv("NETDATA_HOST_PREFIX", "  ")

	if got := netdataHostFilesystemPrefix(); got != "" {
		t.Fatalf("netdataHostFilesystemPrefix() = %q, want empty", got)
	}
}

func TestJournalHostLoadConfigIncludesHostFilesystemPrefix(t *testing.T) {
	if pluginconfig.VarLibDir() != "" {
		t.Skip("pluginconfig VarLibDir is already initialized")
	}

	t.Setenv("NETDATA_HOST_PREFIX", "/host")
	t.Setenv(netdataLibDirEnv, "")
	libDir := filepath.Join(t.TempDir(), "opt", "netdata", "var", "lib", "netdata")
	withTestBuildinfoVarLibDir(t, libDir)

	cfg := journalHostLoadConfig()

	if got, want := cfg.StateDir, filepath.Join(libDir, journalHostStateDirName); got != want {
		t.Fatalf("StateDir = %q, want %q", got, want)
	}
	if got, want := cfg.HostFilesystemPrefix, "/host"; got != want {
		t.Fatalf("HostFilesystemPrefix = %q, want %q", got, want)
	}
}

func TestNetdataEngineStateRootUsesNetdataLibDir(t *testing.T) {
	if pluginconfig.VarLibDir() != "" {
		t.Skip("pluginconfig VarLibDir is already initialized")
	}

	t.Setenv(netdataLibDirEnv, "")
	libDir := filepath.Join(t.TempDir(), "opt", "netdata", "var", "lib", "netdata")
	withTestBuildinfoVarLibDir(t, libDir)

	want := filepath.Join(libDir, "snmp-trap")
	if got := netdataEngineStateRoot(); got != want {
		t.Fatalf("netdataEngineStateRoot() = %q, want %q", got, want)
	}
}

func TestDecodeErrorRealtimeSurvivesCachedHostIdentityFailure(t *testing.T) {
	service := hostidentity.NewWithLoader(
		func() hostidentity.LoadConfig { return hostidentity.LoadConfig{} },
		func(hostidentity.LoadConfig) (hostidentity.Provider, error) {
			return nil, errors.New("identity unavailable")
		},
	)
	c := &Collector{hostIdentity: service}

	before := time.Now().UnixMicro()
	entry := newDecodeErrorEntry("test", decodeErrorRecord{kind: "decode_failed", err: errors.New("bad packet")}, c.monotonicUsec())
	after := time.Now().UnixMicro()

	if entry.ReceivedMonotonicUsec != 0 {
		t.Fatalf("monotonic timestamp = %d, want 0", entry.ReceivedMonotonicUsec)
	}
	if entry.ReceivedRealtimeUsec < before || entry.ReceivedRealtimeUsec > after {
		t.Fatalf("realtime timestamp = %d, want between %d and %d", entry.ReceivedRealtimeUsec, before, after)
	}
}
