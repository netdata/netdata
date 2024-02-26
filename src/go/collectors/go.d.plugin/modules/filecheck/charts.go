// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import "github.com/netdata/netdata/go/go.d.plugin/agent/module"

var (
	fileCharts = module.Charts{
		fileExistenceChart.Copy(),
		fileModTimeAgoChart.Copy(),
		fileSizeChart.Copy(),
	}

	fileExistenceChart = module.Chart{
		ID:    "file_existence",
		Title: "File Existence (0: not exists, 1: exists)",
		Units: "boolean",
		Fam:   "files",
		Ctx:   "filecheck.file_existence",
		Vars: module.Vars{
			{ID: "num_of_files"},
		},
	}
	fileModTimeAgoChart = module.Chart{
		ID:    "file_mtime_ago",
		Title: "File Time Since the Last Modification",
		Units: "seconds",
		Fam:   "files",
		Ctx:   "filecheck.file_mtime_ago",
	}
	fileSizeChart = module.Chart{
		ID:    "file_size",
		Title: "File Size",
		Units: "bytes",
		Fam:   "files",
		Ctx:   "filecheck.file_size",
	}
)

var (
	dirCharts = module.Charts{
		dirExistenceChart.Copy(),
		dirModTimeChart.Copy(),
		dirNumOfFilesChart.Copy(),
		dirSizeChart.Copy(),
	}

	dirExistenceChart = module.Chart{
		ID:    "dir_existence",
		Title: "Dir Existence (0: not exists, 1: exists)",
		Units: "boolean",
		Fam:   "dirs",
		Ctx:   "filecheck.dir_existence",
		Vars: module.Vars{
			{ID: "num_of_dirs"},
		},
	}
	dirModTimeChart = module.Chart{
		ID:    "dir_mtime_ago",
		Title: "Dir Time Since the Last Modification",
		Units: "seconds",
		Fam:   "dirs",
		Ctx:   "filecheck.dir_mtime_ago",
	}
	dirNumOfFilesChart = module.Chart{
		ID:    "dir_num_of_files",
		Title: "Dir Number of Files",
		Units: "files",
		Fam:   "dirs",
		Ctx:   "filecheck.dir_num_of_files",
	}
	dirSizeChart = module.Chart{
		ID:    "dir_size",
		Title: "Dir Size",
		Units: "bytes",
		Fam:   "dirs",
		Ctx:   "filecheck.dir_size",
	}
)
