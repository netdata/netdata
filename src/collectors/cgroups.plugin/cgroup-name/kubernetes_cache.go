// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
)

type kubernetesCache struct {
	clusterNamePath string
	systemUIDPath   string
	containersPath  string
}

func newKubernetesCache(tmpDir string) kubernetesCache {
	return kubernetesCache{
		clusterNamePath: filepath.Join(tmpDir, "netdata-cgroups-k8s-cluster-name"),
		systemUIDPath:   filepath.Join(tmpDir, "netdata-cgroups-kubesystem-uid"),
		containersPath:  filepath.Join(tmpDir, "netdata-cgroups-containers"),
	}
}

func (c kubernetesCache) repairModes() {
	repairPrivateFileMode(c.clusterNamePath)
	repairPrivateFileMode(c.systemUIDPath)
	repairPrivateFileMode(c.containersPath)
}

func (c kubernetesCache) complete() bool {
	return isPrivateRegularFile(c.clusterNamePath) &&
		isPrivateRegularFile(c.systemUIDPath) &&
		isPrivateRegularFile(c.containersPath)
}

func (c kubernetesCache) clusterName() string {
	return firstLineFile(c.clusterNamePath)
}

func (c kubernetesCache) systemUID() string {
	return firstLineFile(c.systemUIDPath)
}

func (c kubernetesCache) lookup(pattern string) (labelSet, bool) {
	line, ok := grepFile(c.containersPath, pattern, 1)
	if !ok {
		return labelSet{}, false
	}
	return parseLabelSet(line), true
}

func (c kubernetesCache) writeClusterName(name string) {
	if name != "" {
		_ = writePrivateFile(c.clusterNamePath, []byte(name+"\n"))
	}
}

func (c kubernetesCache) writeSystemUID(uid string) {
	if uid != "" {
		_ = writePrivateFile(c.systemUIDPath, []byte(uid+"\n"))
	}
}

func (c kubernetesCache) writeContainers(containers []labelSet) {
	_ = writePrivateFile(c.containersPath, []byte(formatLabelSets(containers)+"\n"))
}

func isPrivateRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

// writePrivateFile uses a same-directory O_EXCL temporary file and rename so a
// symlink planted at a predictable cache path is replaced rather than followed.
func writePrivateFile(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".cgroup-name-cache-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
		return err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryName)
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		_ = os.Remove(temporaryName)
		return err
	}
	return nil
}

func repairPrivateFileMode(path string) {
	info, err := os.Lstat(path)
	if err == nil && info.Mode().IsRegular() {
		_ = os.Chmod(path, 0o600)
	}
}

func firstLineFile(path string) string {
	if !isPrivateRegularFile(path) {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return firstLine(string(data))
}

func firstLine(value string) string {
	if before, _, ok := strings.Cut(value, "\n"); ok {
		return before
	}
	return value
}

func grepFile(path, pattern string, max int) (string, bool) {
	if !isPrivateRegularFile(path) {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return grepString(string(data), pattern, max)
}

func grepString(value, pattern string, max int) (string, bool) {
	var matches []string
	for line := range strings.SplitSeq(strings.TrimRight(value, "\n"), "\n") {
		if strings.Contains(line, pattern) {
			matches = append(matches, line)
			if max > 0 && len(matches) >= max {
				break
			}
		}
	}
	if len(matches) == 0 {
		return "", false
	}
	return strings.Join(matches, "\n"), true
}
