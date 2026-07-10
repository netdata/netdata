// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	reDockerDispatch     = regexp.MustCompile(`^.*docker[-_/\.][a-fA-F0-9]+[-_\.]?.*$`)
	reDockerExtract      = regexp.MustCompile(`^.*docker[-_/]([a-fA-F0-9]+)[-_\.]?.*$`)
	reECSDispatch        = regexp.MustCompile(`^.*ecs[-_/\.][a-fA-F0-9]+[-_\.]?.*$`)
	reECSExtract         = regexp.MustCompile(`^.*ecs[-_/].*[-_/]([a-fA-F0-9]+)[-_\.]?.*$`)
	reContainerdDispatch = regexp.MustCompile(`system.slice_containerd.service_cpuset_[a-fA-F0-9]+[-_\.]?.*$`)
	reContainerdExtract  = regexp.MustCompile(`^.*ystem.slice_containerd.service_cpuset_([a-fA-F0-9]+)[-_\.]?.*$`)
	rePodmanDispatch     = regexp.MustCompile(`^.*libpod-[a-fA-F0-9]+.*$`)
	rePodmanExtract      = regexp.MustCompile(`^.*libpod-(conmon-)?([a-fA-F0-9]+).*$`)
	reNspawn             = regexp.MustCompile(`.*machine\.slice[_/](.*)\.service`)
	reProxmoxQemu        = regexp.MustCompile(`qemu.slice_([0-9]+).scope`)
	reProxmoxLXC         = regexp.MustCompile(`lxc_([0-9]+)`)
	reLibvirtQemu        = regexp.MustCompile(`machine_.*\.libvirt-qemu`)
	reLxcPayload         = regexp.MustCompile(`lxc\.payload\.(.*)`)
	reProxmoxConfName    = regexp.MustCompile(`\s*name\s*:\s*(.*)?$`)
	reProxmoxConfHost    = regexp.MustCompile(`\s*hostname\s*:\s*(.*)?$`)
	reMachineIDSegment   = regexp.MustCompile(`[\/_]x2d[[:digit:]]*`)
)

func (r *resolver) resolveNonKubernetes(ctx context.Context, cgroup string) (resolution, bool) {
	switch {
	case reDockerDispatch.MatchString(cgroup):
		return r.resolveDockerID(ctx, extractOrOriginal(reDockerExtract, cgroup, 1), cgroup)
	case reECSDispatch.MatchString(cgroup):
		return r.resolveDockerID(ctx, extractOrOriginal(reECSExtract, cgroup, 1), cgroup)
	case reContainerdDispatch.MatchString(cgroup):
		return r.resolveDockerID(ctx, extractOrOriginal(reContainerdExtract, cgroup, 1), cgroup)
	case rePodmanDispatch.MatchString(cgroup):
		match := rePodmanExtract.FindStringSubmatch(cgroup)
		id := cgroup
		if len(match) > 2 {
			id = match[2]
		}
		return r.resolvePodmanID(ctx, id, cgroup)
	case reNspawn.MatchString(cgroup):
		name := reNspawn.ReplaceAllString(cgroup, "$1")
		return resolution{name: name}, name != ""
	case strings.Contains(cgroup, "machine.slice_machine") && strings.Contains(cgroup, "-lxc"):
		return resolution{name: "lxc/" + lxcMachineName(cgroup)}, true
	case strings.Contains(cgroup, "machine.slice_machine") && strings.Contains(cgroup, "-qemu"):
		return resolution{name: "qemu_" + machineName(cgroup, "-qemu")}, true
	case reLibvirtQemu.MatchString(cgroup):
		name := strings.TrimPrefix(cgroup, "machine_")
		name = strings.TrimSuffix(name, ".libvirt-qemu")
		name = strings.Replace(name, "-", "_", 1)
		return resolution{name: "qemu_" + name}, true
	case reProxmoxQemu.MatchString(cgroup) && isDir(hostPath(r.config.hostPrefix, "/etc/pve")):
		match := reProxmoxQemu.FindStringSubmatch(cgroup)
		filename := hostPath(r.config.hostPrefix, "/etc/pve/qemu-server/"+match[1]+".conf")
		if fileReadable(filename) {
			name := "qemu_" + firstConfigValue(filename, reProxmoxConfName, "^name: ")
			return resolution{name: name}, true
		}
		r.error(fmt.Sprintf("proxmox config file missing %s or netdata does not have read access.  Please ensure netdata is a member of www-data group.", filename))
	case reProxmoxLXC.MatchString(cgroup) && isDir(hostPath(r.config.hostPrefix, "/etc/pve")):
		match := reProxmoxLXC.FindStringSubmatch(cgroup)
		filename := hostPath(r.config.hostPrefix, "/etc/pve/lxc/"+match[1]+".conf")
		if fileReadable(filename) {
			name := firstConfigValue(filename, reProxmoxConfHost, "^hostname: ")
			return resolution{name: name}, name != ""
		}
		r.error(fmt.Sprintf("proxmox config file missing %s or netdata does not have read access.  Please ensure netdata is a member of www-data group.", filename))
	case strings.Contains(cgroup, "lxc.payload"):
		name := reLxcPayload.ReplaceAllString(cgroup, "$1")
		return resolution{name: name}, name != ""
	}
	return resolution{}, false
}

func extractOrOriginal(re *regexp.Regexp, value string, index int) string {
	match := re.FindStringSubmatch(value)
	if len(match) > index {
		return match[index]
	}
	return value
}

func lxcMachineName(value string) string {
	name := machineName(value, "-lxc")
	name = reMachineIDSegment.ReplaceAllString(name, "")
	name = strings.ReplaceAll(name, "/x2d", "")
	name = strings.ReplaceAll(name, "_x2d", "")
	name = strings.ReplaceAll(name, ".scope", "")
	return name
}

// machineName removes only the first numeric x2d segment. Later digits belong
// to the machine name and must remain distinct (for example web01 and web02).
func machineName(value, marker string) string {
	if index := strings.LastIndex(value, marker); index >= 0 {
		value = value[index+len(marker):]
	}
	if location := reMachineIDSegment.FindStringIndex(value); location != nil {
		value = value[:location[0]] + value[location[1]:]
	}
	value = strings.ReplaceAll(value, "/x2d", "")
	value = strings.ReplaceAll(value, "_x2d", "")
	return strings.ReplaceAll(value, ".scope", "")
}

func hostPath(prefix, path string) string {
	return prefix + path
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileReadable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}

// firstConfigValue keeps the legacy grep-before-sed gate: "name:vm" does not
// match the required "name: " prefix even though the expression accepts it.
func firstConfigValue(path string, expression *regexp.Regexp, grepPrefix string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if !strings.HasPrefix(line, strings.TrimPrefix(grepPrefix, "^")) {
			continue
		}
		if match := expression.FindStringSubmatch(line); len(match) > 1 {
			return match[1]
		}
		return ""
	}
	return ""
}
