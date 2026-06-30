// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	sdkjournal "github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/netdata/systemd-journal-sdk/go/journalhost"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
)

const (
	netdataLibDirEnv        = "NETDATA_LIB_DIR"
	journalHostStateDirName = "systemd-journal-sdk"
)

type journalHostProvider interface {
	MachineID() sdkjournal.UUID
	BootID() sdkjournal.UUID
	MonotonicUsec() uint64
}

var defaultJournalHost struct {
	once     sync.Once
	provider journalHostProvider
	err      error
}

func loadJournalHostProvider() (journalHostProvider, error) {
	stateDir := netdataJournalHostStateDir()
	provider, err := journalhost.Load(journalhost.LoadOptions{StateDir: stateDir})
	if err != nil {
		return nil, fmt.Errorf("load local journal host identity using state directory %s: %w", stateDir, err)
	}
	return provider, nil
}

func defaultJournalHostProvider() (journalHostProvider, error) {
	defaultJournalHost.once.Do(func() {
		defaultJournalHost.provider, defaultJournalHost.err = journalhost.Load(journalhost.LoadOptions{
			StateDir: netdataJournalHostStateDir(),
		})
	})
	return defaultJournalHost.provider, defaultJournalHost.err
}

func netdataJournalHostStateDir() string {
	return filepath.Join(netdataLibDir(), journalHostStateDirName)
}

func netdataLibDir() string {
	if dir := strings.TrimSpace(pluginconfig.VarLibDir()); dir != "" {
		return filepath.Clean(dir)
	}
	if dir := strings.TrimSpace(os.Getenv(netdataLibDirEnv)); dir != "" {
		return filepath.Clean(dir)
	}
	if dir := strings.TrimSpace(buildinfo.VarLibDir); dir != "" {
		return filepath.Clean(dir)
	}
	return filepath.Clean(buildinfo.DefaultVarLibDir)
}

func defaultMonotonicUsec() int64 {
	provider, err := defaultJournalHostProvider()
	if err != nil {
		return 0
	}
	return int64(provider.MonotonicUsec())
}

func (c *Collector) monotonicUsec() int64 {
	if c != nil && c.journalHost != nil {
		return int64(c.journalHost.MonotonicUsec())
	}
	return monotonicUsec()
}
