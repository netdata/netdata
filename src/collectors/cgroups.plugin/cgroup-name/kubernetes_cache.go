// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	maxKubernetesCacheMetadataLine  = 64 << 10
	maxKubernetesCacheContainerLine = 16 << 20
	maxKubernetesCacheContainers    = 128 << 20
	gcpClusterNameNegativeTTL       = 5 * time.Minute
)

type kubernetesCache struct {
	clusterNamePath string
	systemUIDPath   string
	containersPath  string
}

func newKubernetesCache(dir string) (kubernetesCache, error) {
	if dir == "" {
		return kubernetesCache{}, errors.New("cache directory is empty")
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return kubernetesCache{}, fmt.Errorf("create private cache directory: %w", err)
	}
	if err := validatePrivateDirectory(dir); err != nil {
		return kubernetesCache{}, err
	}
	return kubernetesCache{
		clusterNamePath: filepath.Join(dir, "netdata-cgroups-k8s-cluster-name"),
		systemUIDPath:   filepath.Join(dir, "netdata-cgroups-kubesystem-uid"),
		containersPath:  filepath.Join(dir, "netdata-cgroups-containers"),
	}, nil
}

func (c kubernetesCache) complete() bool {
	return isPrivateRegularFile(c.clusterNamePath, maxKubernetesCacheMetadataLine) &&
		isPrivateRegularFile(c.systemUIDPath, maxKubernetesCacheMetadataLine) &&
		isPrivateRegularFile(c.containersPath, maxKubernetesCacheContainers)
}

func (c kubernetesCache) clusterName() string {
	return firstLineFile(c.clusterNamePath, maxKubernetesCacheMetadataLine)
}

func (c kubernetesCache) systemUID() string {
	return firstLineFile(c.systemUIDPath, maxKubernetesCacheMetadataLine)
}

func (c kubernetesCache) clusterNameNeedsRefresh(name string, now time.Time) bool {
	if name == "" {
		return true
	}
	if name != unknownKubernetesClusterName {
		return false
	}
	file, err := openPrivateRegularFile(c.clusterNamePath, maxKubernetesCacheMetadataLine)
	if err != nil {
		return true
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return true
	}
	return !info.ModTime().Add(gcpClusterNameNegativeTTL).After(now)
}

func (c kubernetesCache) lookupContainer(ctx context.Context, id string) (labelSet, bool) {
	return findContainerLabelSetInFile(ctx, c.containersPath, id)
}

func (c kubernetesCache) writeClusterName(name string) error {
	if name == "" || c.clusterNamePath == "" {
		return nil
	}
	if len(name)+1 > maxKubernetesCacheMetadataLine {
		return fmt.Errorf("cluster-name cache exceeds %d bytes", maxKubernetesCacheMetadataLine)
	}
	return writePrivateFile(c.clusterNamePath, []byte(name+"\n"))
}

func (c kubernetesCache) writeSystemUID(uid string) error {
	if uid == "" || c.systemUIDPath == "" {
		return nil
	}
	if len(uid)+1 > maxKubernetesCacheMetadataLine {
		return fmt.Errorf("system-UID cache exceeds %d bytes", maxKubernetesCacheMetadataLine)
	}
	return writePrivateFile(c.systemUIDPath, []byte(uid+"\n"))
}

func (c kubernetesCache) writeContainers(containers []labelSet) error {
	if c.containersPath == "" {
		return nil
	}
	data := formatLabelSets(containers) + "\n"
	if len(data) > maxKubernetesCacheContainers {
		return fmt.Errorf("container cache exceeds %d bytes", maxKubernetesCacheContainers)
	}
	return writePrivateFile(c.containersPath, []byte(data))
}

func isPrivateRegularFile(path string, maxSize int64) bool {
	info, err := os.Lstat(path)
	return err == nil && privateRegularFileInfo(info, maxSize)
}

// writePrivateFile uses a same-directory O_EXCL temporary file and rename so a
// symlink planted at a predictable cache path is replaced rather than followed.
func writePrivateFile(path string, data []byte) error {
	if path == "" {
		return errors.New("cache path is empty")
	}
	if err := validatePrivateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
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

func validatePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect cache directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("cache directory is not a real directory")
	}
	if info.Mode().Perm() != 0o700 {
		return fmt.Errorf("cache directory mode is %o, want 0700", info.Mode().Perm())
	}
	if !ownedByCurrentUser(info) {
		return errors.New("cache directory is not owned by the current user")
	}
	return nil
}

func firstLineFile(path string, maxSize int64) string {
	file, err := openPrivateRegularFile(path, maxSize)
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

func findContainerLabelSetInFile(ctx context.Context, path, id string) (labelSet, bool) {
	if id == "" {
		return labelSet{}, false
	}
	if err := ctx.Err(); err != nil {
		return labelSet{}, false
	}
	file, err := openPrivateRegularFile(path, maxKubernetesCacheContainers)
	if err != nil {
		return labelSet{}, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), maxKubernetesCacheContainerLine)
	target := []byte(formatLabel(label{name: "container_id", value: id}))
	suffix := append([]byte{','}, target...)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return labelSet{}, false
		}
		line := scanner.Bytes()
		if !bytes.Equal(line, target) && !bytes.HasSuffix(line, suffix) {
			continue
		}
		labels := parseLabelSet(string(line))
		if candidate, ok := labels.value("container_id"); ok && candidate == id {
			return labels, true
		}
	}
	if scanner.Err() != nil {
		return labelSet{}, false
	}
	return labelSet{}, false
}

func openPrivateRegularFile(path string, maxSize int64) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil || !privateRegularFileInfo(before, maxSize) {
		return nil, os.ErrInvalid
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, openErr := file.Stat()
	after, pathErr := os.Lstat(path)
	if openErr != nil || pathErr != nil || !privateRegularFileInfo(opened, maxSize) || !privateRegularFileInfo(after, maxSize) ||
		!os.SameFile(before, opened) || !os.SameFile(opened, after) {
		_ = file.Close()
		return nil, os.ErrInvalid
	}
	return file, nil
}

func privateRegularFileInfo(info os.FileInfo, maxSize int64) bool {
	if info == nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() < 0 || info.Size() > maxSize ||
		!ownedByCurrentUser(info) {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}

func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}
