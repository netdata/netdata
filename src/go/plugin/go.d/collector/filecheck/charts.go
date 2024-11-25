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

func (f *Filecheck) updateFileCharts(infos []*statInfo) {
	seen := make(map[string]bool)

	for _, info := range infos {
		seen[info.path] = true

		sf := f.seenFiles.getp(info.path)

		if !sf.hasExistenceCharts {
			sf.hasExistenceCharts = true
			f.addFileCharts(info.path,
				fileExistenceStatusChartTmpl.Copy(),
			)
		}

		if !sf.hasOtherCharts && info.fi != nil {
			sf.hasOtherCharts = true
			f.addFileCharts(info.path,
				fileModificationTimeAgoChartTmpl.Copy(),
				fileSizeChartTmpl.Copy(),
			)

		} else if sf.hasOtherCharts && info.fi == nil {
			sf.hasOtherCharts = false
			f.removeFileOtherCharts(info.path)
		}
	}

	for path := range f.seenFiles.items {
		if !seen[path] {
			delete(f.seenFiles.items, path)
			f.removeFileAllCharts(path)
		}
	}
}

func (f *Filecheck) updateDirCharts(infos []*statInfo) {
	seen := make(map[string]bool)

	for _, info := range infos {
		seen[info.path] = true

		sd := f.seenDirs.getp(info.path)

		if !sd.hasExistenceCharts {
			sd.hasExistenceCharts = true
			f.addDirCharts(info.path,
				dirExistenceStatusChartTmpl.Copy(),
			)
		}

		if !sd.hasOtherCharts && info.fi != nil {
			sd.hasOtherCharts = true
			f.addDirCharts(info.path,
				dirModificationTimeAgoChartTmpl.Copy(),
				dirFilesCountChartTmpl.Copy(),
			)
			if f.Dirs.CollectDirSize {
				f.addDirCharts(info.path,
					dirSizeChartTmpl.Copy(),
				)
			}

		} else if sd.hasOtherCharts && info.fi == nil {
			sd.hasOtherCharts = false
			f.removeDirOtherCharts(info.path)
		}
	}

	for path := range f.seenDirs.items {
		if !seen[path] {
			delete(f.seenDirs.items, path)
			f.removeDirAllCharts(path)
		}
	}
}

func (f *Filecheck) addFileCharts(filePath string, chartsTmpl ...*module.Chart) {
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

	if err := f.Charts().Add(*charts...); err != nil {
		f.Warning(err)
	}
}

func (f *Filecheck) addDirCharts(dirPath string, chartsTmpl ...*module.Chart) {
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

	if err := f.Charts().Add(*charts...); err != nil {
		f.Warning(err)
	}
}

func (f *Filecheck) removeFileAllCharts(filePath string) {
	px := fmt.Sprintf("file_%s_", cleanPath(filePath))
	f.removeCharts(func(id string) bool {
		return strings.HasPrefix(id, px)
	})
}

func (f *Filecheck) removeFileOtherCharts(filePath string) {
	px := fmt.Sprintf("file_%s_", cleanPath(filePath))
	f.removeCharts(func(id string) bool {
		return strings.HasPrefix(id, px) && !strings.HasSuffix(id, "existence_status")
	})
}

func (f *Filecheck) removeDirAllCharts(dirPath string) {
	px := fmt.Sprintf("dir_%s_", cleanPath(dirPath))
	f.removeCharts(func(id string) bool {
		return strings.HasPrefix(id, px)
	})
}

func (f *Filecheck) removeDirOtherCharts(dirPath string) {
	px := fmt.Sprintf("dir_%s_", cleanPath(dirPath))
	f.removeCharts(func(id string) bool {
		return strings.HasPrefix(id, px) && !strings.HasSuffix(id, "existence_status")
	})
}

func (f *Filecheck) removeCharts(match func(id string) bool) {
	for _, chart := range *f.Charts() {
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
