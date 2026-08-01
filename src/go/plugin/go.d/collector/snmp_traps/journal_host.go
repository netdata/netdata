// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/hostidentity"
)

const (
	netdataLibDirEnv        = "NETDATA_LIB_DIR"
	journalHostStateDirName = "systemd-journal-sdk"
)

type journalHostProvider = hostidentity.Provider

func newHostIdentityService() *hostidentity.Service {
	return hostidentity.New(journalHostLoadConfig)
}

func journalHostLoadConfig() hostidentity.LoadConfig {
	return hostidentity.LoadConfig{
		StateDir:             netdataJournalHostStateDir(),
		HostFilesystemPrefix: netdataHostFilesystemPrefix(),
	}
}

func netdataJournalHostStateDir() string {
	return filepath.Join(netdataLibDir(), journalHostStateDirName)
}

func netdataHostFilesystemPrefix() string {
	if dir := strings.TrimSpace(pluginconfig.HostPrefix()); dir != "" {
		return filepath.Clean(dir)
	}
	return ""
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

func netdataEngineStateRoot() string {
	return filepath.Join(netdataLibDir(), "snmp-trap")
}

func (c *Collector) monotonicUsec() int64 {
	if c != nil && c.journalHost != nil {
		return int64(c.journalHost.MonotonicUsec())
	}
	if c == nil || c.hostIdentity == nil {
		return 0
	}
	provider, err := c.hostIdentity.CachedFallback()
	if err != nil {
		return 0
	}
	return int64(provider.MonotonicUsec())
}

func (c *Collector) sourceHashSalt() string {
	if c != nil && c.hostIdentity != nil {
		if provider, err := c.hostIdentity.CachedFallback(); err == nil {
			return "machine-id:" + provider.MachineID().String()
		}
	}
	return "netdata-agent"
}
