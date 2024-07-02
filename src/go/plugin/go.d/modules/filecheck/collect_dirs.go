// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (f *Filecheck) collectDirs(mx map[string]int64) {
	now := time.Now()

	if f.isTimeToDiscoverDirs(now) {
		f.lastDiscDirsTime = now
		f.curDirs = f.discoveryDirs()
	}

	var infos []*statInfo

	for _, dir := range f.curDirs {
		si := getStatInfo(dir)
		infos = append(infos, si)

		f.collectDir(mx, si, now)
	}

	f.updateDirCharts(infos)
}

func (f *Filecheck) collectDir(mx map[string]int64, si *statInfo, now time.Time) {
	px := fmt.Sprintf("dir_%s_", si.path)

	mx[px+"existence_status_exist"] = 0
	mx[px+"existence_status_not_exist"] = 0
	if !si.exists {
		mx[px+"existence_status_not_exist"] = 1
	} else {
		mx[px+"existence_status_exist"] = 1
	}

	if si.fi == nil || !si.fi.IsDir() {
		return
	}

	mx[px+"mtime_ago"] = int64(now.Sub(si.fi.ModTime()).Seconds())

	if v, err := calcFilesInDir(si.path); err == nil {
		mx[px+"files_count"] = v
	}
	if f.Dirs.CollectDirSize {
		if v, err := calcDirSize(si.path); err == nil {
			mx[px+"size_bytes"] = v
		}
	}
}

func (f *Filecheck) discoveryDirs() (dirs []string) {
	return discoverFilesOrDirs(f.Dirs.Include, func(v string, fi os.FileInfo) bool {
		return fi.IsDir() && !f.dirsFilter.MatchString(v)
	})
}

func (f *Filecheck) isTimeToDiscoverDirs(now time.Time) bool {
	return now.After(f.lastDiscDirsTime.Add(f.DiscoveryEvery.Duration()))
}

func calcFilesInDir(dirPath string) (int64, error) {
	f, err := os.Open(dirPath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	names, err := f.Readdirnames(-1)
	return int64(len(names)), err
}

func calcDirSize(dirPath string) (int64, error) {
	var size int64
	err := filepath.Walk(dirPath, func(_ string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			size += fi.Size()
		}
		return nil
	})
	return size, err
}
