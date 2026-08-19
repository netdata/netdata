// SPDX-License-Identifier: GPL-3.0-or-later

#include "../apps-cgroups-path.h"

static bool expect_path(const char *name, const char *content, const char *expected)
{
    char path[FILENAME_MAX + 1];

    if (!apps_cgroup_parse_proc_pid_cgroup_content(content, path, sizeof(path))) {
        fprintf(stderr, "%s: parse failed\n", name);
        return false;
    }

    if (strcmp(path, expected) != 0) {
        fprintf(stderr, "%s: expected '%s', got '%s'\n", name, expected, path);
        return false;
    }

    return true;
}

static bool expect_failure(const char *name, const char *content)
{
    char path[FILENAME_MAX + 1];

    if (apps_cgroup_parse_proc_pid_cgroup_content(content, path, sizeof(path))) {
        fprintf(stderr, "%s: expected failure, got '%s'\n", name, path);
        return false;
    }

    return true;
}

int main(void)
{
    if (!expect_path("v2 unified", "0::/kubepods.slice/pod-a\n", "/kubepods.slice/pod-a"))
        return 1;

    if (!expect_path(
            "v2 beats v1",
            "12:memory:/legacy\n0::/unified\n2:cpu,cpuacct:/cpu\n",
            "/unified"))
        return 1;

    if (!expect_path(
            "v2 namespace-relative",
            "0::/../../../kubepods-besteffort.slice/pod-a/cri-containerd-abc.scope\n",
            "/../../../kubepods-besteffort.slice/pod-a/cri-containerd-abc.scope"))
        return 1;

    if (!expect_path(
            "crlf malformed line advances",
            "malformed\r\n0::/unified\r\n",
            "/unified"))
        return 1;

    if (!expect_path(
            "v1 cpuacct precedence",
            "3:memory:/memory-path\n2:blkio:/blkio-path\n1:cpu,cpuacct:/cpuacct-path\n",
            "/cpuacct-path"))
        return 1;

    if (!expect_path(
            "v1 namespace-relative cpuacct precedence",
            "3:memory:/memory-path\n1:cpu,cpuacct:/../../../kubepods.slice/pod-a/cri-containerd-abc.scope\n",
            "/../../../kubepods.slice/pod-a/cri-containerd-abc.scope"))
        return 1;

    if (!expect_path(
            "v1 blkio precedence",
            "3:memory:/memory-path\n2:blkio:/blkio-path\n4:name=systemd:/init.scope\n",
            "/blkio-path"))
        return 1;

    if (!expect_path(
            "v1 systemd fallback",
            "7:name=systemd:/init.scope\n9:freezer:/first-nonpreferred\n",
            "/init.scope"))
        return 1;

    if (!expect_path(
            "v1 first fallback",
            "9:freezer:/first\n10:devices:/second\n",
            "/first"))
        return 1;

    if (!expect_failure("empty path fails closed", "1:cpu,cpuacct:\n"))
        return 1;

    if (!expect_failure("relative path fails closed", "1:cpu,cpuacct:relative\n"))
        return 1;

    return 0;
}
