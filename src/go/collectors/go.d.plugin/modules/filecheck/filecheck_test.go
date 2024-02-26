// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	assert.Implements(t, (*module.Module)(nil), New())
}

func TestFilecheck_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestFilecheck_Init(t *testing.T) {
	tests := map[string]struct {
		config          Config
		wantNumOfCharts int
		wantFail        bool
	}{
		"default": {
			config:   New().Config,
			wantFail: true,
		},
		"empty files->include and dirs->include": {
			config: Config{
				Files: filesConfig{},
				Dirs:  dirsConfig{},
			},
			wantFail: true,
		},
		"files->include and dirs->include": {
			config: Config{
				Files: filesConfig{
					Include: []string{
						"/path/to/file1",
						"/path/to/file2",
					},
				},
				Dirs: dirsConfig{
					Include: []string{
						"/path/to/dir1",
						"/path/to/dir2",
					},
					CollectDirSize: true,
				},
			},
			wantNumOfCharts: len(fileCharts) + len(dirCharts),
		},
		"only files->include": {
			config: Config{
				Files: filesConfig{
					Include: []string{
						"/path/to/file1",
						"/path/to/file2",
					},
				},
			},
			wantNumOfCharts: len(fileCharts),
		},
		"only dirs->include": {
			config: Config{
				Dirs: dirsConfig{
					Include: []string{
						"/path/to/dir1",
						"/path/to/dir2",
					},
					CollectDirSize: true,
				},
			},
			wantNumOfCharts: len(dirCharts),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fc := New()
			fc.Config = test.config

			if test.wantFail {
				assert.False(t, fc.Init())
			} else {
				require.True(t, fc.Init())
				assert.Equal(t, test.wantNumOfCharts, len(*fc.Charts()))
			}
		})
	}
}

func TestFilecheck_Check(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Filecheck
	}{
		"collect files":                   {prepare: prepareFilecheckFiles},
		"collect files filepath pattern":  {prepare: prepareFilecheckGlobFiles},
		"collect only non existent files": {prepare: prepareFilecheckNonExistentFiles},
		"collect dirs":                    {prepare: prepareFilecheckDirs},
		"collect dirs filepath pattern":   {prepare: prepareFilecheckGlobDirs},
		"collect only non existent dirs":  {prepare: prepareFilecheckNonExistentDirs},
		"collect files and dirs":          {prepare: prepareFilecheckFilesDirs},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fc := test.prepare()
			require.True(t, fc.Init())

			assert.True(t, fc.Check())
		})
	}
}

func TestFilecheck_Collect(t *testing.T) {
	// TODO: should use TEMP dir and create files/dirs dynamically during a test case
	tests := map[string]struct {
		prepare       func() *Filecheck
		wantCollected map[string]int64
	}{
		"collect files": {
			prepare: prepareFilecheckFiles,
			wantCollected: map[string]int64{
				"file_testdata/empty_file.log_exists":        1,
				"file_testdata/empty_file.log_mtime_ago":     5081,
				"file_testdata/empty_file.log_size_bytes":    0,
				"file_testdata/file.log_exists":              1,
				"file_testdata/file.log_mtime_ago":           4161,
				"file_testdata/file.log_size_bytes":          5707,
				"file_testdata/non_existent_file.log_exists": 0,
				"num_of_files": 3,
				"num_of_dirs":  0,
			},
		},
		"collect files filepath pattern": {
			prepare: prepareFilecheckGlobFiles,
			wantCollected: map[string]int64{
				"file_testdata/empty_file.log_exists":     1,
				"file_testdata/empty_file.log_mtime_ago":  5081,
				"file_testdata/empty_file.log_size_bytes": 0,
				"file_testdata/file.log_exists":           1,
				"file_testdata/file.log_mtime_ago":        4161,
				"file_testdata/file.log_size_bytes":       5707,
				"num_of_files":                            2,
				"num_of_dirs":                             0,
			},
		},
		"collect only non existent files": {
			prepare: prepareFilecheckNonExistentFiles,
			wantCollected: map[string]int64{
				"file_testdata/non_existent_file.log_exists": 0,
				"num_of_files": 1,
				"num_of_dirs":  0,
			},
		},
		"collect dirs": {
			prepare: prepareFilecheckDirs,
			wantCollected: map[string]int64{
				"dir_testdata/dir_exists":              1,
				"dir_testdata/dir_mtime_ago":           4087,
				"dir_testdata/dir_num_of_files":        3,
				"dir_testdata/dir_size_bytes":          8160,
				"dir_testdata/non_existent_dir_exists": 0,
				"num_of_files":                         0,
				"num_of_dirs":                          2,
			},
		},
		"collect dirs filepath pattern": {
			prepare: prepareFilecheckGlobDirs,
			wantCollected: map[string]int64{
				"dir_testdata/dir_exists":              1,
				"dir_testdata/dir_mtime_ago":           4087,
				"dir_testdata/dir_num_of_files":        3,
				"dir_testdata/dir_size_bytes":          8160,
				"dir_testdata/non_existent_dir_exists": 0,
				"num_of_files":                         0,
				"num_of_dirs":                          2,
			},
		},
		"collect dirs w/o size": {
			prepare: prepareFilecheckDirsWithoutSize,
			wantCollected: map[string]int64{
				"dir_testdata/dir_exists":              1,
				"dir_testdata/dir_mtime_ago":           4087,
				"dir_testdata/dir_num_of_files":        3,
				"dir_testdata/non_existent_dir_exists": 0,
				"num_of_files":                         0,
				"num_of_dirs":                          2,
			},
		},
		"collect only non existent dirs": {
			prepare: prepareFilecheckNonExistentDirs,
			wantCollected: map[string]int64{
				"dir_testdata/non_existent_dir_exists": 0,
				"num_of_files":                         0,
				"num_of_dirs":                          1,
			},
		},
		"collect files and dirs": {
			prepare: prepareFilecheckFilesDirs,
			wantCollected: map[string]int64{
				"dir_testdata/dir_exists":                    1,
				"dir_testdata/dir_mtime_ago":                 4120,
				"dir_testdata/dir_num_of_files":              3,
				"dir_testdata/dir_size_bytes":                8160,
				"dir_testdata/non_existent_dir_exists":       0,
				"file_testdata/empty_file.log_exists":        1,
				"file_testdata/empty_file.log_mtime_ago":     5176,
				"file_testdata/empty_file.log_size_bytes":    0,
				"file_testdata/file.log_exists":              1,
				"file_testdata/file.log_mtime_ago":           4256,
				"file_testdata/file.log_size_bytes":          5707,
				"file_testdata/non_existent_file.log_exists": 0,
				"num_of_files":                               3,
				"num_of_dirs":                                2,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fc := test.prepare()
			require.True(t, fc.Init())

			collected := fc.Collect()

			copyModTime(test.wantCollected, collected)
			assert.Equal(t, test.wantCollected, collected)
			ensureCollectedHasAllChartsDimsVarsIDs(t, fc, collected)
		})
	}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, fc *Filecheck, collected map[string]int64) {
	// TODO: check other charts
	for _, chart := range *fc.Charts() {
		if chart.Obsolete {
			continue
		}
		switch chart.ID {
		case fileExistenceChart.ID, dirExistenceChart.ID:
			for _, dim := range chart.Dims {
				_, ok := collected[dim.ID]
				assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
			}
			for _, v := range chart.Vars {
				_, ok := collected[v.ID]
				assert.Truef(t, ok, "collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
			}
		}
	}
}

func prepareFilecheckFiles() *Filecheck {
	fc := New()
	fc.Config.Files.Include = []string{
		"testdata/empty_file.log",
		"testdata/file.log",
		"testdata/non_existent_file.log",
	}
	return fc
}

func prepareFilecheckGlobFiles() *Filecheck {
	fc := New()
	fc.Config.Files.Include = []string{
		"testdata/*.log",
	}
	return fc
}

func prepareFilecheckNonExistentFiles() *Filecheck {
	fc := New()
	fc.Config.Files.Include = []string{
		"testdata/non_existent_file.log",
	}
	return fc
}

func prepareFilecheckDirs() *Filecheck {
	fc := New()
	fc.Config.Dirs.Include = []string{
		"testdata/dir",
		"testdata/non_existent_dir",
	}
	return fc
}

func prepareFilecheckGlobDirs() *Filecheck {
	fc := New()
	fc.Config.Dirs.Include = []string{
		"testdata/*ir",
		"testdata/non_existent_dir",
	}
	return fc
}

func prepareFilecheckDirsWithoutSize() *Filecheck {
	fc := New()
	fc.Config.Dirs.Include = []string{
		"testdata/dir",
		"testdata/non_existent_dir",
	}
	fc.Config.Dirs.CollectDirSize = false
	return fc
}

func prepareFilecheckNonExistentDirs() *Filecheck {
	fc := New()
	fc.Config.Dirs.Include = []string{
		"testdata/non_existent_dir",
	}
	return fc
}

func prepareFilecheckFilesDirs() *Filecheck {
	fc := New()
	fc.Config.Files.Include = []string{
		"testdata/empty_file.log",
		"testdata/file.log",
		"testdata/non_existent_file.log",
	}
	fc.Config.Dirs.Include = []string{
		"testdata/dir",
		"testdata/non_existent_dir",
	}
	return fc
}

func copyModTime(dst, src map[string]int64) {
	if src == nil || dst == nil {
		return
	}
	for key := range src {
		if strings.Contains(key, "mtime") {
			dst[key] = src[key]
		}
	}
}
