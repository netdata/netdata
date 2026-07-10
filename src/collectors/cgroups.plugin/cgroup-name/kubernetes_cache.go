// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxKubernetesCacheMetadataLine  = 64 << 10
	maxKubernetesCacheContainerLine = 16 << 20
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
	file, err := openPrivateRegularFile(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), maxKubernetesCacheMetadataLine)
	if scanner.Scan() {
		return scanner.Text()
	}
	return ""
}

func grepFile(path, pattern string, max int) (string, bool) {
	file, err := openPrivateRegularFile(path)
	if err != nil {
		return "", false
	}
	defer file.Close()

	var matches []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), maxKubernetesCacheContainerLine)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, pattern) {
			matches = append(matches, line)
			if max > 0 && len(matches) >= max {
				break
			}
		}
	}
	if scanner.Err() != nil {
		return "", false
	}
	if len(matches) == 0 {
		return "", false
	}
	return strings.Join(matches, "\n"), true
}

func openPrivateRegularFile(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, openErr := file.Stat()
	after, pathErr := os.Lstat(path)
	if openErr != nil || pathErr != nil || !opened.Mode().IsRegular() || !after.Mode().IsRegular() ||
		!os.SameFile(before, opened) || !os.SameFile(opened, after) {
		_ = file.Close()
		return nil, os.ErrInvalid
	}
	return file, nil
}
