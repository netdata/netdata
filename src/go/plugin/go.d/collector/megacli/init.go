// SPDX-License-Identifier: GPL-3.0-or-later

package megacli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

func (c *Collector) initMegaCliExec() (megaCli, error) {
	if runtime.GOOS == "windows" {
		return c.initDirectMegaCliExec()
	}
	return c.initNdsudoMegaCliExec()
}

func (c *Collector) initNdsudoMegaCliExec() (megaCli, error) {
	megaExec := newNdsudoMegaCliExec(c.Timeout.Duration(), c.Logger)
	return megaExec, nil
}

func (c *Collector) initDirectMegaCliExec() (megaCli, error) {
	path, err := ndexec.FindBinary(
		[]string{"megacli", "MegaCli", "MegaCli64", "megacli64"},
		[]string{
			filepath.Join(os.Getenv("ProgramFiles"), "LSI", "MegaCLI", "MegaCli64.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Broadcom", "MegaCLI", "MegaCli64.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "MegaCLI", "MegaCli64.exe"),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("megacli: %w", err)
	}

	c.Debugf("found megacli at: %s", path)

	return newDirectMegaCliExec(path, c.Timeout.Duration(), c.Logger), nil
}
