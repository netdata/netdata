// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioFileExistenceStatus = module.Priority + iota
	prioFileModificationTimeAgo
	prioFileSize

	prioDirExistenceStatus
	prioDirModificationTimeAgo
	prioDirSize
	prioDirFilesCount
)

var (
	fileExistenceStatusChartTmpl = module.Chart{
		ID:       "file_%s_existence_status",
		Title:    "File existence",
		Units:    "status",
		Fam:      "file existence",
		Ctx:      "filecheck.file_existence_status",
		Priority: prioFileExistenceStatus,
		Dims: module.Dims{
			{ID: "file_%s_existence_status_exist", Name: "exist"},
			{ID: "file_%s_existence_status_not_exist", Name: "not_exist"},
		},
	}

	fileModificationTimeAgoChartTmpl = module.Chart{
		ID:       "file_%s_modification_time_ago",
		Title:    "File time since the last modification",
		Units:    "seconds",
		Fam:      "file mtime",
		Ctx:      "filecheck.file_modification_time_ago",
		Priority: prioFileModificationTimeAgo,
		Dims: module.Dims{
			{ID: "file_%s_mtime_ago", Name: "mtime_ago"},
		},
	}
	fileSizeChartTmpl = module.Chart{
		ID:       "file_%s_size",
		Title:    "File size",
		Units:    "bytes",
		Fam:      "file size",
		Ctx:      "filecheck.file_size_bytes",
		Priority: prioFileSize,
		Dims: module.Dims{
			{ID: "file_%s_size_bytes", Name: "size"},
		},
	}
)

var (
	dirExistenceStatusChartTmpl = module.Chart{
		ID:       "dir_%s_existence_status",
		Title:    "Directory existence",
		Units:    "status",
		Fam:      "dir existence",
		Ctx:      "filecheck.dir_existence_status",
		Priority: prioDirExistenceStatus,
		Dims: module.Dims{
			{ID: "dir_%s_existence_status_exist", Name: "exist"},
			{ID: "dir_%s_existence_status_not_exist", Name: "not_exist"},
		},
	}

	dirModificationTimeAgoChartTmpl = module.Chart{
		ID:       "dir_%s_modification_time_ago",
		Title:    "Directory time since the last modification",
		Units:    "seconds",
		Fam:      "dir mtime",
		Ctx:      "filecheck.dir_modification_time_ago",
		Priority: prioDirModificationTimeAgo,
		Dims: module.Dims{
			{ID: "dir_%s_mtime_ago", Name: "mtime_ago"},
		},
	}
	dirSizeChartTmpl = module.Chart{
		ID:       "dir_%s_size",
		Title:    "Directory size",
		Units:    "bytes",
		Fam:      "dir size",
		Ctx:      "filecheck.dir_size_bytes",
		Priority: prioDirSize,
		Dims: module.Dims{
			{ID: "dir_%s_size_bytes", Name: "size"},
		},
	}
	dirFilesCountChartTmpl = module.Chart{
		ID:       "dir_%s_files_count",
		Title:    "Directory files count",
		Units:    "files",
		Fam:      "dir files",
		Ctx:      "filecheck.dir_files_count",
		Priority: prioDirFilesCount,
		Dims: module.Dims{
			{ID: "dir_%s_files_count", Name: "files"},
		},
	}
)

func (c *Collector) updateFileCharts(infos []*statInfo) {
	seen := make(map[string]bool)

	for _, info := range infos {
		seen[info.path] = true

		sf := c.seenFiles.getp(info.path)

		if !sf.hasExistenceCharts {
			sf.hasExistenceCharts = true
			c.addFileCharts(info.path,
				fileExistenceStatusChartTmpl.Copy(),
			)
		}

		if !sf.hasOtherCharts && info.fi != nil {
			sf.hasOtherCharts = true
			c.addFileCharts(info.path,
				fileModificationTimeAgoChartTmpl.Copy(),
				fileSizeChartTmpl.Copy(),
			)

		} else if sf.hasOtherCharts && info.fi == nil {
			sf.hasOtherCharts = false
			c.removeFileOtherCharts(info.path)
		}
	}

	for path := range c.seenFiles.items {
		if !seen[path] {
			delete(c.seenFiles.items, path)
			c.removeFileAllCharts(path)
		}
	}
}

func (c *Collector) updateDirCharts(infos []*statInfo) {
	seen := make(map[string]bool)

	for _, info := range infos {
		seen[info.path] = true

		sd := c.seenDirs.getp(info.path)

		if !sd.hasExistenceCharts {
			sd.hasExistenceCharts = true
			c.addDirCharts(info.path,
				dirExistenceStatusChartTmpl.Copy(),
			)
		}

		if !sd.hasOtherCharts && info.fi != nil {
			sd.hasOtherCharts = true
			c.addDirCharts(info.path,
				dirModificationTimeAgoChartTmpl.Copy(),
				dirFilesCountChartTmpl.Copy(),
			)
			if c.Dirs.CollectDirSize {
				c.addDirCharts(info.path,
					dirSizeChartTmpl.Copy(),
				)
			}

		} else if sd.hasOtherCharts && info.fi == nil {
			sd.hasOtherCharts = false
			c.removeDirOtherCharts(info.path)
		}
	}

	for path := range c.seenDirs.items {
		if !seen[path] {
			delete(c.seenDirs.items, path)
			c.removeDirAllCharts(path)
		}
	}
}

func (c *Collector) addFileCharts(filePath string, chartsTmpl ...*module.Chart) {
	cs := append(module.Charts{}, chartsTmpl...)
	charts := cs.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanPath(filePath))
		chart.Labels = []module.Label{
			{Key: "file_path", Value: filePath},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, filePath)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addDirCharts(dirPath string, chartsTmpl ...*module.Chart) {
	cs := append(module.Charts{}, chartsTmpl...)
	charts := cs.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanPath(dirPath))
		chart.Labels = []module.Label{
			{Key: "dir_path", Value: dirPath},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, dirPath)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeFileAllCharts(filePath string) {
	px := fmt.Sprintf("file_%s_", cleanPath(filePath))
	c.removeCharts(func(id string) bool {
		return strings.HasPrefix(id, px)
	})
}

func (c *Collector) removeFileOtherCharts(filePath string) {
	px := fmt.Sprintf("file_%s_", cleanPath(filePath))
	c.removeCharts(func(id string) bool {
		return strings.HasPrefix(id, px) && !strings.HasSuffix(id, "existence_status")
	})
}

func (c *Collector) removeDirAllCharts(dirPath string) {
	px := fmt.Sprintf("dir_%s_", cleanPath(dirPath))
	c.removeCharts(func(id string) bool {
		return strings.HasPrefix(id, px)
	})
}

func (c *Collector) removeDirOtherCharts(dirPath string) {
	px := fmt.Sprintf("dir_%s_", cleanPath(dirPath))
	c.removeCharts(func(id string) bool {
		return strings.HasPrefix(id, px) && !strings.HasSuffix(id, "existence_status")
	})
}

func (c *Collector) removeCharts(match func(id string) bool) {
	for _, chart := range *c.Charts() {
		if match(chart.ID) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func cleanPath(path string) string {
	path = strings.ReplaceAll(path, " ", "_")
	path = strings.ReplaceAll(path, ".", "_")
	return path
}
