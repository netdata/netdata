// SPDX-License-Identifier: GPL-3.0-or-later

package as400

import (
	"context"
	"fmt"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (a *AS400) collectIFSTopNDirectories(ctx context.Context) error {
	if a.IFSTopNDirectories <= 0 {
		return nil
	}

	query := queryIFSTopNDirectories

	chart := a.charts.Get("ifs_directory_usage")
	if chart == nil {
		return fmt.Errorf("chart not found: ifs_directory_usage")
	}

	var dirName string
	return a.doQuery(ctx, fmt.Sprintf(query, a.IFSStartPath, a.IFSTopNDirectories), func(column, value string, lineEnd bool) {
		switch column {
		case "DIR":
			dirName = value
		case "TOTAL_SIZE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				dimID := cleanName(dirName)
				if chart.GetDim(dimID) == nil {
					chart.AddDim(&module.Dim{ID: dimID, Name: dirName})
				}
				a.mx.IFSDirectoryUsage[dimID] = v
			}
		}
	})
}
