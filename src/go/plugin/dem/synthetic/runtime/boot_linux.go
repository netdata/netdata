// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func privateDirectory(path string) error {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mode&0777 != 0700 || st.Uid != uint32(os.Geteuid()) {
		return errors.New("runtime store must be a private directory owned by the service account")
	}
	return nil
}

func hostBoot() bootIdentity {
	const source = "/proc/sys/kernel/random/boot_id"
	fd, err := unix.Open(source, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return bootIdentity{}
	}
	file := os.NewFile(uintptr(fd), source)
	defer file.Close()
	var fs unix.Statfs_t
	var st unix.Statx_t
	if unix.Fstatfs(fd, &fs) != nil || fs.Type != unix.PROC_SUPER_MAGIC ||
		unix.Statx(fd, "", unix.AT_EMPTY_PATH, unix.STATX_TYPE|unix.STATX_MNT_ID, &st) != nil ||
		st.Mask&unix.STATX_MNT_ID == 0 || st.Mode&unix.S_IFMT != unix.S_IFREG {
		return bootIdentity{}
	}
	mounts, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return bootIdentity{}
	}
	defer mounts.Close()
	if !kernelBootMount(mounts, st.Mnt_id, source) {
		return bootIdentity{}
	}
	data, err := io.ReadAll(io.LimitReader(file, 38))
	id := strings.TrimSuffix(string(data), "\n")
	if err != nil || len(data) != 37 || !validBootID(id) {
		return bootIdentity{}
	}
	return bootIdentity{ID: id, Verified: true}
}

// Match the opened FD, not merely the apparent pathname. Container managers can
// overmount boot_id with a container UUID; procfs uuid binds are also rejected.
func kernelBootMount(reader io.Reader, mountID uint64, source string) bool {
	scanner := bufio.NewScanner(reader)
	matches := 0
	valid := false
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			return false
		}
		id, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return false
		}
		if id != mountID {
			continue
		}
		matches++
		separator := 6
		for separator < len(fields) && fields[separator] != "-" {
			separator++
		}
		if separator+3 >= len(fields) || fields[separator+1] != "proc" {
			continue
		}
		root, mount := mountPath(fields[3]), mountPath(fields[4])
		relative, err := filepath.Rel(mount, source)
		if err != nil || relative == ".." || strings.HasPrefix(relative, "../") {
			continue
		}
		valid = filepath.Join(root, relative) == "/sys/kernel/random/boot_id"
	}
	return scanner.Err() == nil && matches == 1 && valid
}

func mountPath(value string) string {
	return strings.NewReplacer("\\040", " ", "\\011", "\t", "\\012", "\n", "\\134", "\\").Replace(value)
}
