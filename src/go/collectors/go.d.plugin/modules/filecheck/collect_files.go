// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"fmt"
	"os"
	"time"
)

func (f *Filecheck) collectFiles(mx map[string]int64) {
	now := time.Now()

	if f.isTimeToDiscoverFiles(now) {
		f.lastDiscFilesTime = now
		f.curFiles = f.discoverFiles()
	}

	var infos []*statInfo

	for _, file := range f.curFiles {
		si := getStatInfo(file)

		infos = append(infos, si)

		f.collectFile(mx, si, now)
	}

	f.updateFileCharts(infos)
}

func (f *Filecheck) collectFile(mx map[string]int64, si *statInfo, now time.Time) {
	px := fmt.Sprintf("file_%s_", si.path)

	mx[px+"existence_status_exist"] = 0
	mx[px+"existence_status_not_exist"] = 0
	if !si.exists {
		mx[px+"existence_status_not_exist"] = 1
	} else {
		mx[px+"existence_status_exist"] = 1
	}

	if si.fi == nil || !si.fi.Mode().IsRegular() {
		return
	}

	mx[px+"mtime_ago"] = int64(now.Sub(si.fi.ModTime()).Seconds())
	mx[px+"size_bytes"] = si.fi.Size()
}

func (f *Filecheck) discoverFiles() (files []string) {
	return discoverFilesOrDirs(f.Files.Include, func(absPath string, fi os.FileInfo) bool {
		return fi.Mode().IsRegular() && !f.filesFilter.MatchString(absPath)
	})
}

func (f *Filecheck) isTimeToDiscoverFiles(now time.Time) bool {
	return now.After(f.lastDiscFilesTime.Add(f.DiscoveryEvery.Duration()))
}
