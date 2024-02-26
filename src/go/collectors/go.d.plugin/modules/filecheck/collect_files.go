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

func (fc *Filecheck) collectFiles(ms map[string]int64) {
	curTime := time.Now()
	if time.Since(fc.lastDiscoveryFiles) >= fc.DiscoveryEvery.Duration {
		fc.lastDiscoveryFiles = curTime
		fc.curFiles = fc.discoveryFiles()
		fc.updateFilesCharts(fc.curFiles)
	}

	for _, path := range fc.curFiles {
		fc.collectFile(ms, path, curTime)
	}
	ms["num_of_files"] = int64(len(fc.curFiles))
}

func (fc *Filecheck) collectFile(ms map[string]int64, path string, curTime time.Time) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			ms[fileDimID(path, "exists")] = 0
		} else {
			ms[fileDimID(path, "exists")] = 1
		}
		fc.Debug(err)
		return
	}

	if info.IsDir() {
		return
	}

	ms[fileDimID(path, "exists")] = 1
	ms[fileDimID(path, "size_bytes")] = info.Size()
	ms[fileDimID(path, "mtime_ago")] = int64(curTime.Sub(info.ModTime()).Seconds())
}

func (fc Filecheck) discoveryFiles() (files []string) {
	for _, path := range fc.Files.Include {
		if hasMeta(path) {
			continue
		}
		files = append(files, path)
	}

	for _, path := range fc.Files.Include {
		if !hasMeta(path) {
			continue
		}
		matches, _ := filepath.Glob(path)
		for _, v := range matches {
			fi, err := os.Lstat(v)
			if err == nil && fi.Mode().IsRegular() {
				files = append(files, v)
			}
		}
	}
	return removeDuplicates(files)
}

func (fc *Filecheck) updateFilesCharts(files []string) {
	set := make(map[string]bool, len(files))
	for _, path := range files {
		set[path] = true
		if !fc.collectedFiles[path] {
			fc.collectedFiles[path] = true
			fc.addFileToCharts(path)
		}
	}
	for path := range fc.collectedFiles {
		if !set[path] {
			delete(fc.collectedFiles, path)
			fc.removeFileFromCharts(path)
		}
	}
}

func (fc *Filecheck) addFileToCharts(path string) {
	for _, chart := range *fc.Charts() {
		if !strings.HasPrefix(chart.ID, "file_") {
			continue
		}

		var id string
		switch chart.ID {
		case fileExistenceChart.ID:
			id = fileDimID(path, "exists")
		case fileModTimeAgoChart.ID:
			id = fileDimID(path, "mtime_ago")
		case fileSizeChart.ID:
			id = fileDimID(path, "size_bytes")
		default:
			fc.Warningf("add dimension: couldn't dim id for '%s' chart (file '%s')", chart.ID, path)
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

func (fc *Filecheck) removeFileFromCharts(path string) {
	for _, chart := range *fc.Charts() {
		if !strings.HasPrefix(chart.ID, "file_") {
			continue
		}

		var id string
		switch chart.ID {
		case fileExistenceChart.ID:
			id = fileDimID(path, "exists")
		case fileModTimeAgoChart.ID:
			id = fileDimID(path, "mtime_ago")
		case fileSizeChart.ID:
			id = fileDimID(path, "size_bytes")
		default:
			fc.Warningf("remove dimension: couldn't dim id for '%s' chart (file '%s')", chart.ID, path)
			continue
		}

		if err := chart.MarkDimRemove(id, true); err != nil {
			fc.Warning(err)
			continue
		}
		chart.MarkNotCreated()
	}
}

func fileDimID(path, metric string) string {
	return fmt.Sprintf("file_%s_%s", reSpace.ReplaceAllString(path, "_"), metric)
}
