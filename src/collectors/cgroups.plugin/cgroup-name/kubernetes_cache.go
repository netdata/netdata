// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxKubernetesCacheMetadataLine  = 64 << 10
	maxKubernetesCacheContainerLine = 16 << 20
	gcpClusterNameNegativeTTL       = 5 * time.Minute
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

func (c kubernetesCache) clusterNameNeedsRefresh(name string, now time.Time) bool {
	if name == "" {
		return true
	}
	if name != unknownKubernetesClusterName {
		return false
	}
	info, err := os.Lstat(c.clusterNamePath)
	if err != nil || !info.Mode().IsRegular() {
		return true
	}
	return !info.ModTime().Add(gcpClusterNameNegativeTTL).After(now)
}

func (c kubernetesCache) lookupContainer(id string) (labelSet, bool) {
	return findLabelSetInFile(c.containersPath, "container_id", id)
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

func findLabelSetInFile(path, name, value string) (labelSet, bool) {
	if name == "" || value == "" {
		return labelSet{}, false
	}
	file, err := openPrivateRegularFile(path)
	if err != nil {
		return labelSet{}, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), maxKubernetesCacheContainerLine)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, name+"=") {
			continue
		}
		labels := parseLabelSet(line)
		if candidate, ok := labels.value(name); ok && candidate == value {
			return labels, true
		}
	}
	if scanner.Err() != nil {
		return labelSet{}, false
	}
	return labelSet{}, false
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
