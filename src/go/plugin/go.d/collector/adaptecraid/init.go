// SPDX-License-Identifier: GPL-3.0-or-later

package adaptecraid

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

func (c *Collector) initArcconfCliExec() (arcconfCli, error) {
	if runtime.GOOS == "windows" {
		return c.initDirectArcconfCliExec()
	}
	return c.initNdsudoArcconfCliExec()
}

func (c *Collector) initNdsudoArcconfCliExec() (arcconfCli, error) {
	arcconfExec := newNdsudoArcconfCliExec(c.Timeout.Duration(), c.Logger)
	return arcconfExec, nil
}

func (c *Collector) initDirectArcconfCliExec() (arcconfCli, error) {
	path, err := ndexec.FindBinary(
		[]string{"arcconf", "ARCCONF"},
		[]string{
			filepath.Join(os.Getenv("ProgramFiles"), "Adaptec", "ARCCONF", "arcconf.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Microsemi", "ARCCONF", "arcconf.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Adaptec", "ARCCONF", "arcconf.exe"),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("arcconf: %w", err)
	}

	c.Debugf("found arcconf at: %s", path)

	return newDirectArcconfCliExec(path, c.Timeout.Duration(), c.Logger), nil
}
