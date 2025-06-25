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

	query := `
		SELECT
			SUBSTRING(PATH_NAME, 1, LOCATE('/', PATH_NAME, 2) - 1) as DIR,
			SUM(OBJECT_SIZE) as TOTAL_SIZE
		FROM TABLE(QSYS2.IFS_OBJECT_STATISTICS(
			START_PATH_NAME => '/',
			SUBTREE_DIRECTORIES => 'YES'
		)) AS T
		WHERE LOCATE('/', PATH_NAME, 2) > 0
		GROUP BY SUBSTRING(PATH_NAME, 1, LOCATE('/', PATH_NAME, 2) - 1)
		ORDER BY TOTAL_SIZE DESC
		FETCH FIRST %d ROWS ONLY
	`

	chart := a.charts.Get("ifs_directory_usage")
	if chart == nil {
		return fmt.Errorf("chart not found: ifs_directory_usage")
	}

	var dirName string
	return a.doQuery(ctx, fmt.Sprintf(query, a.IFSTopNDirectories), func(column, value string, lineEnd bool) {
		switch column {
		case "DIR":
			dirName = value
		case "TOTAL_SIZE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				dimID := cleanName(dirName)
				if chart.GetDim(dimID) == nil {
					chart.AddDim(module.Dim{ID: dimID, Name: dirName})
				}
				a.mx.IFSDirectoryUsage[dimID] = v
			}
		}
	})
}
