// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"errors"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

func (fc Filecheck) validateConfig() error {
	if len(fc.Files.Include) == 0 && len(fc.Dirs.Include) == 0 {
		return errors.New("both 'files->include' and 'dirs->include' are empty")
	}
	return nil
}

func (fc Filecheck) initCharts() (*module.Charts, error) {
	charts := &module.Charts{}

	if len(fc.Files.Include) > 0 {
		if err := charts.Add(*fileCharts.Copy()...); err != nil {
			return nil, err
		}
	}

	if len(fc.Dirs.Include) > 0 {
		if err := charts.Add(*dirCharts.Copy()...); err != nil {
			return nil, err
		}
		if !fc.Dirs.CollectDirSize {
			if err := charts.Remove(dirSizeChart.ID); err != nil {
				return nil, err
			}
		}
	}

	if len(*charts) == 0 {
		return nil, errors.New("empty charts")
	}
	return charts, nil
}
