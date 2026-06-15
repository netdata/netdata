// SPDX-License-Identifier: GPL-3.0-or-later

#include "../cgroup-path.h"
#include "../cgroup-topology-rules.h"

static bool expect_bool(bool actual, bool expected, const char *message)
{
    if(actual == expected)
        return true;

    fprintf(stderr, "%s: expected %s, got %s\n",
            message,
            expected ? "true" : "false",
            actual ? "true" : "false");
    return false;
}

static bool expect_string(const char *actual, const char *expected, const char *message)
{
    if(strcmp(actual ? actual : "", expected ? expected : "") == 0)
        return true;

    fprintf(stderr, "%s: expected '%s', got '%s'\n",
            message,
            expected ? expected : "",
            actual ? actual : "");
    return false;
}

static bool expect_path(const char *name, const char *content, const char *expected)
{
    char path[FILENAME_MAX + 1];

    if(!cgroup_path_parse_proc_pid_cgroup_content(content, path, sizeof(path))) {
        fprintf(stderr, "%s: parse failed\n", name);
        return false;
    }

    return expect_string(path, expected, name);
}

static bool test_namespace_relative_helper(void)
{
    bool ok = true;

    ok = expect_bool(cgroup_path_is_namespace_relative("/../foo"), true, "/../foo namespace-relative") && ok;
    ok = expect_bool(cgroup_path_is_namespace_relative("/../../foo"), true, "/../../foo namespace-relative") && ok;
    ok = expect_bool(cgroup_path_is_namespace_relative("../../../foo"), true, "../../../foo namespace-relative") && ok;
    ok = expect_bool(cgroup_path_is_namespace_relative("/foo/../bar"), false, "/foo/../bar not namespace-relative") && ok;
    ok = expect_bool(cgroup_path_is_namespace_relative(".../foo"), false, ".../foo not namespace-relative") && ok;
    ok = expect_bool(cgroup_path_is_namespace_relative("/..../foo"), false, "/..../foo not namespace-relative") && ok;
    ok = expect_bool(cgroup_path_is_namespace_relative("/foo/..bar"), false, "/foo/..bar not namespace-relative") && ok;
    ok = expect_bool(cgroup_path_is_namespace_relative("/foo"), false, "/foo not namespace-relative") && ok;
    ok = expect_bool(cgroup_path_is_namespace_relative("/"), false, "/ not namespace-relative") && ok;

    return ok;
}

static bool test_proc_pid_cgroup_parser(void)
{
    bool ok = true;

    ok = expect_path("v2 unified", "0::/kubepods.slice/pod-a\n", "/kubepods.slice/pod-a") && ok;
    ok = expect_path(
             "v2 namespace-relative",
             "0::/../../../kubepods-besteffort.slice/pod-a/cri-containerd-abc.scope\n",
             "/../../../kubepods-besteffort.slice/pod-a/cri-containerd-abc.scope") && ok;
    ok = expect_path(
             "v1 cpuacct precedence",
             "3:memory:/memory-path\n2:blkio:/blkio-path\n1:cpu,cpuacct:/cpuacct-path\n",
             "/cpuacct-path") && ok;
    ok = expect_path(
             "v1 namespace-relative cpuacct precedence",
             "3:memory:/memory-path\n1:cpu,cpuacct:/../../../kubepods.slice/pod-a/cri-containerd-abc.scope\n",
             "/../../../kubepods.slice/pod-a/cri-containerd-abc.scope") && ok;
    ok = expect_path(
             "v1 systemd fallback",
             "7:name=systemd:/init.scope\n9:freezer:/first-nonpreferred\n",
             "/init.scope") && ok;

    return ok;
}

static bool test_namespace_relative_classifier(void)
{
    CGROUP_TOPOLOGY_CLASSIFICATION classification;
    const char *path =
        "/../../kubepods.slice/kubepods-besteffort.slice/podabc/"
        "cri-containerd-0123456789abcdef.scope";

    cgroup_topology_classify(
        NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER,
        NIPC_ORCHESTRATOR_UNKNOWN,
        path,
        &classification);

    bool ok =
        expect_string(classification.actor_type, "container", "retry-later namespace-relative actor type") &&
        expect_string(classification.actor_kind, "pending", "retry-later namespace-relative actor kind");

    cgroup_topology_classify(
        NIPC_APPS_CGROUP_UNKNOWN_PERMANENT,
        NIPC_ORCHESTRATOR_UNKNOWN,
        path,
        &classification);

    ok = expect_bool(
             strcmp(classification.actor_type, "systemd_scope") != 0,
             true,
             "unknown-permanent namespace-relative must not become systemd scope") && ok;

    cgroup_topology_classify(
        NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER,
        NIPC_ORCHESTRATOR_UNKNOWN,
        "/user.slice/user-1001.slice/user@1001.service/app.slice/opencode.scope",
        &classification);

    ok = expect_string(classification.actor_type, "systemd_scope", "absolute systemd retry-later actor type") && ok;
    ok = expect_string(classification.actor_kind, "scope", "absolute systemd retry-later actor kind") && ok;

    cgroup_topology_classify(
        NIPC_APPS_CGROUP_KNOWN,
        NIPC_ORCHESTRATOR_UNKNOWN,
        path,
        &classification);

    ok = expect_string(classification.actor_type, "systemd_scope", "known namespace-relative systemd actor type") && ok;

    return ok;
}

int main(void)
{
    bool ok = true;

    ok = test_namespace_relative_helper() && ok;
    ok = test_proc_pid_cgroup_parser() && ok;
    ok = test_namespace_relative_classifier() && ok;

    return ok ? 0 : 1;
}
