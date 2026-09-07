// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
)

const LifecycleFilename = "lifecycle.zst"
const TopologyDirectory = "topology"

func DirectoryPath(varLibDir string) string {
	dir := strings.TrimSpace(varLibDir)
	if dir == "" {
		dir = strings.TrimSpace(buildinfo.VarLibDir)
	}
	if dir == "" {
		dir = buildinfo.DefaultVarLibDir
	}
	return filepath.Join(dir, "snmp", "diagnostics")
}

type CheckpointFile struct {
	Sequence uint64 `json:"sequence"`
	Filename string `json:"filename"`
}

func checkpointFilename(sequence uint64) string { return fmt.Sprintf("checkpoint-%020d.zst", sequence) }

// ListCheckpoints reads filenames only. Evidence is decoded when selected, not
// during rotation; temporary files and unrelated entries never enter retention.
func ListCheckpoints(directory string) ([]CheckpointFile, error) {
	files, err := os.ReadDir(filepath.Join(directory, TopologyDirectory))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []CheckpointFile
	for _, file := range files {
		if !file.Type().IsRegular() {
			continue
		}
		name := file.Name()
		number := strings.TrimSuffix(strings.TrimPrefix(name, "checkpoint-"), ".zst")
		sequence, err := strconv.ParseUint(number, 10, 64)
		if err != nil || sequence == 0 || name != checkpointFilename(sequence) {
			continue
		}
		result = append(result, CheckpointFile{Sequence: sequence, Filename: name})
	}
	return result, nil
}
