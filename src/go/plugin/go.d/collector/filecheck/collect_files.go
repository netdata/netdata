// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"fmt"
	"os"
	"time"
)

func (c *Collector) collectFiles(mx map[string]int64) {
	now := time.Now()

	if c.isTimeToDiscoverFiles(now) {
		c.lastDiscFilesTime = now
		c.curFiles = c.discoverFiles()
	}

	var infos []*statInfo

	for _, file := range c.curFiles {
		si := getStatInfo(file)

		infos = append(infos, si)

		c.collectFile(mx, si, now)
	}

	c.updateFileCharts(infos)
}

func (c *Collector) collectFile(mx map[string]int64, si *statInfo, now time.Time) {
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

func (c *Collector) discoverFiles() (files []string) {
	return discoverFilesOrDirs(c.Files.Include, func(absPath string, fi os.FileInfo) bool {
		return fi.Mode().IsRegular() && !c.filesFilter.MatchString(absPath)
	})
}

func (c *Collector) isTimeToDiscoverFiles(now time.Time) bool {
	return now.After(c.lastDiscFilesTime.Add(c.DiscoveryEvery.Duration()))
}
