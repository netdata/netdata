// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

const (
	// dmiTablesDir exposes the raw SMBIOS entry point and structure table on Linux.
	dmiTablesDir = "/sys/firmware/dmi/tables"
	// stateFileName is the collector's private baseline file under the Agent's varlib directory.
	stateFileName = "smbios-memory.json"
)

func (c *Collector) initTableReader() {
	if c.readTable != nil {
		return
	}
	dir := filepath.Join(pluginconfig.HostPrefix(), dmiTablesDir)
	c.readTable = func() (*inventory.Table, error) { return readHostTable(dir) }
}

func readHostTable(dir string) (*inventory.Table, error) {
	entry, err := os.ReadFile(filepath.Join(dir, "smbios_entry_point"))
	if err != nil {
		return nil, fmt.Errorf("read SMBIOS entry point: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "DMI"))
	if err != nil {
		return nil, fmt.Errorf("read DMI table: %w", err)
	}
	return parseTable(entry, data)
}

func (c *Collector) initBaselineStore() {
	if c.baseline.path == "" {
		dir := cmp.Or(pluginconfig.VarLibDir(), buildinfo.VarLibDir, buildinfo.DefaultVarLibDir)
		c.baseline.path = filepath.Join(dir, stateFileName)
	}
	if c.baseline.owner == "" {
		c.baseline.owner = pluginconfig.RegistryUniqueID()
	}
	if c.isTerminal() {
		c.baseline.readOnly = true
		c.Infof("running from a terminal: the memory baseline %s is read-only, changes are not saved", c.baseline.path)
	}
}
