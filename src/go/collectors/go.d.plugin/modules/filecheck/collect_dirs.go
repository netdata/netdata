// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

func (fc *Filecheck) collectDirs(ms map[string]int64) {
	curTime := time.Now()
	if time.Since(fc.lastDiscoveryDirs) >= fc.DiscoveryEvery.Duration() {
		fc.lastDiscoveryDirs = curTime
		fc.curDirs = fc.discoveryDirs()
		fc.updateDirsCharts(fc.curDirs)
	}

	for _, path := range fc.curDirs {
		fc.collectDir(ms, path, curTime)
	}
	ms["num_of_dirs"] = int64(len(fc.curDirs))
}

func (fc *Filecheck) collectDir(ms map[string]int64, path string, curTime time.Time) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			ms[dirDimID(path, "exists")] = 0
		} else {
			ms[dirDimID(path, "exists")] = 1
		}
		fc.Debug(err)
		return
	}

	if !info.IsDir() {
		return
	}

	ms[dirDimID(path, "exists")] = 1
	ms[dirDimID(path, "mtime_ago")] = int64(curTime.Sub(info.ModTime()).Seconds())
	if num, err := calcDirNumOfFiles(path); err == nil {
		ms[dirDimID(path, "num_of_files")] = int64(num)
	}
	if fc.Dirs.CollectDirSize {
		if size, err := calcDirSize(path); err == nil {
			ms[dirDimID(path, "size_bytes")] = size
		}
	}
}

func (fc *Filecheck) discoveryDirs() (dirs []string) {
	for _, path := range fc.Dirs.Include {
		if hasMeta(path) {
			continue
		}
		dirs = append(dirs, path)
	}

	for _, path := range fc.Dirs.Include {
		if !hasMeta(path) {
			continue
		}
		matches, _ := filepath.Glob(path)
		for _, v := range matches {
			fi, err := os.Lstat(v)
			if err == nil && fi.IsDir() {
				dirs = append(dirs, v)
			}
		}
	}
	return removeDuplicates(dirs)
}

func (fc *Filecheck) updateDirsCharts(dirs []string) {
	set := make(map[string]bool, len(dirs))
	for _, path := range dirs {
		set[path] = true
		if !fc.collectedDirs[path] {
			fc.collectedDirs[path] = true
			fc.addDirToCharts(path)
		}
	}
	for path := range fc.collectedDirs {
		if !set[path] {
			delete(fc.collectedDirs, path)
			fc.removeDirFromCharts(path)
		}
	}
}

func (fc *Filecheck) addDirToCharts(path string) {
	for _, chart := range *fc.Charts() {
		if !strings.HasPrefix(chart.ID, "dir_") {
			continue
		}

		var id string
		switch chart.ID {
		case dirExistenceChart.ID:
			id = dirDimID(path, "exists")
		case dirModTimeChart.ID:
			id = dirDimID(path, "mtime_ago")
		case dirNumOfFilesChart.ID:
			id = dirDimID(path, "num_of_files")
		case dirSizeChart.ID:
			id = dirDimID(path, "size_bytes")
		default:
			fc.Warningf("add dimension: couldn't dim id for '%s' chart (dir '%s')", chart.ID, path)
			continue
		}

		dim := &module.Dim{ID: id, Name: reSpace.ReplaceAllString(path, "_")}

		if err := chart.AddDim(dim); err != nil {
			fc.Warning(err)
			continue
		}
		chart.MarkNotCreated()
	}
}

func (fc *Filecheck) removeDirFromCharts(path string) {
	for _, chart := range *fc.Charts() {
		if !strings.HasPrefix(chart.ID, "dir_") {
			continue
		}

		var id string
		switch chart.ID {
		case dirExistenceChart.ID:
			id = dirDimID(path, "exists")
		case dirModTimeChart.ID:
			id = dirDimID(path, "mtime_ago")
		case dirNumOfFilesChart.ID:
			id = dirDimID(path, "num_of_files")
		case dirSizeChart.ID:
			id = dirDimID(path, "size_bytes")
		default:
			fc.Warningf("remove dimension: couldn't dim id for '%s' chart (dir '%s')", chart.ID, path)
			continue
		}

		if err := chart.MarkDimRemove(id, true); err != nil {
			fc.Warning(err)
			continue
		}
		chart.MarkNotCreated()
	}
}

func dirDimID(path, metric string) string {
	return fmt.Sprintf("dir_%s_%s", reSpace.ReplaceAllString(path, "_"), metric)
}

func calcDirNumOfFiles(dirpath string) (int, error) {
	f, err := os.Open(dirpath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	// TODO: include dirs?
	names, err := f.Readdirnames(-1)
	return len(names), err
}

func calcDirSize(dirpath string) (int64, error) {
	var size int64
	err := filepath.Walk(dirpath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
