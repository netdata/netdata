// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"errors"
	"io/fs"
	"os"
)

func (f *Filecheck) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	f.collectFiles(mx)
	f.collectDirs(mx)

	return mx, nil
}

type statInfo struct {
	path   string
	exists bool
	fi     fs.FileInfo
}

func getStatInfo(path string) *statInfo {
	fi, err := os.Stat(path)
	if err != nil {
		return &statInfo{
			path:   path,
			exists: !errors.Is(err, fs.ErrNotExist),
		}
	}

	return &statInfo{
		path:   path,
		exists: true,
		fi:     fi,
	}
}
