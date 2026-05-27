// SPDX-License-Identifier: GPL-3.0-or-later

#include "../network-viewer-topology-containers.h"

static bool expect_string(const char *actual, const char *expected, const char *message)
{
    if (strcmp(actual ? actual : "", expected ? expected : "") == 0)
        return true;

    fprintf(stderr, "%s: got '%s', expected '%s'\n", message, actual ? actual : "", expected ? expected : "");
    return false;
}

static bool expect_true(bool condition, const char *message)
{
    if (condition)
        return true;

    fprintf(stderr, "%s\n", message);
    return false;
}

static NV_APPS_LOOKUP_LABEL label(const char *key, const char *value)
{
    return (NV_APPS_LOOKUP_LABEL){ .key = (char *)key, .value = (char *)value };
}

static NV_APPS_LOOKUP_FIELDS fields_from_labels(NV_APPS_LOOKUP_LABEL *labels, uint16_t label_count)
{
    return (NV_APPS_LOOKUP_FIELDS){
        .cgroup_status = NIPC_APPS_CGROUP_KNOWN,
        .orchestrator = NIPC_ORCHESTRATOR_K8S,
        .cgroup_labels = labels,
        .cgroup_label_count = label_count,
    };
}

static bool test_whitelist(void)
{
    SIMPLE_PATTERN *pattern = nv_label_whitelist_parse("team|app|version-*");
    bool ok =
        expect_true(pattern != NULL, "whitelist parser returned NULL") &&
        expect_true(simple_pattern_matches(pattern, "team"), "pipe token 'team' did not match independently") &&
        expect_true(simple_pattern_matches(pattern, "app"), "pipe token 'app' did not match independently") &&
        expect_true(simple_pattern_matches(pattern, "version-prod"), "wildcard token did not match") &&
        expect_true(!simple_pattern_matches(pattern, "owner"), "unexpected whitelist match");
    simple_pattern_free(pattern);

    pattern = nv_label_whitelist_parse("team,app");
    ok = expect_true(pattern != NULL, "comma-literal whitelist parser returned NULL") && ok;
    ok = expect_true(simple_pattern_matches(pattern, "team,app"), "comma should be literal for labels:<pattern>") && ok;
    ok = expect_true(!simple_pattern_matches(pattern, "team"), "comma-separated labels must not split") && ok;
    simple_pattern_free(pattern);
    return ok;
}

static bool test_k8s_direct_labels(void)
{
    NV_APPS_LOOKUP_LABEL labels[] = {
        label("k8s_pod_name", "api-6d4cf56db6-x7abc"),
        label("k8s_namespace", "prod"),
        label("k8s_controller_name", "api"),
        label("k8s_container_name", "app"),
        label("container_name", "docker-name"),
        label("image", "registry.example/app:v1"),
    };
    NV_APPS_LOOKUP_FIELDS fields = fields_from_labels(labels, _countof(labels));

    char value[NV_TOPOLOGY_DOCKER_IMAGE_MAX];
    nv_derive_k8s_pod_name(&fields, value, sizeof(value));
    bool ok = expect_string(value, "api-6d4cf56db6-x7abc", "pod name direct label");
    nv_derive_k8s_namespace(&fields, value, sizeof(value));
    ok = expect_string(value, "prod", "namespace direct label") && ok;
    nv_derive_k8s_workload(&fields, value, sizeof(value));
    ok = expect_string(value, "api", "controller name should win for workload") && ok;
    nv_derive_docker_container_name(&fields, "fallback-cgroup", value, sizeof(value));
    ok = expect_string(value, "app", "k8s container name should win over container_name") && ok;
    nv_derive_docker_image(&fields, value, sizeof(value));
    ok = expect_string(value, "registry.example/app:v1", "image direct label") && ok;

    return ok;
}

static bool test_k8s_workload_fallbacks(void)
{
    char value[NV_TOPOLOGY_K8S_NAME_MAX];
    bool ok = true;

    NV_APPS_LOOKUP_LABEL replicaset_labels[] = {
        label("k8s_pod_name", "frontend-6d4cf56db6-x7abc"),
        label("k8s_controller_kind", "ReplicaSet"),
    };
    NV_APPS_LOOKUP_FIELDS fields = fields_from_labels(replicaset_labels, _countof(replicaset_labels));
    nv_derive_k8s_workload(&fields, value, sizeof(value));
    ok = expect_string(value, "frontend", "ReplicaSet workload fallback") && ok;

    NV_APPS_LOOKUP_LABEL daemonset_labels[] = {
        label("k8s_pod_name", "node-exporter-abcde"),
        label("k8s_controller_kind", "DaemonSet"),
    };
    fields = fields_from_labels(daemonset_labels, _countof(daemonset_labels));
    nv_derive_k8s_workload(&fields, value, sizeof(value));
    ok = expect_string(value, "node-exporter", "DaemonSet workload fallback") && ok;

    NV_APPS_LOOKUP_LABEL statefulset_labels[] = {
        label("k8s_pod_name", "postgres-0"),
        label("k8s_controller_kind", "StatefulSet"),
    };
    fields = fields_from_labels(statefulset_labels, _countof(statefulset_labels));
    nv_derive_k8s_workload(&fields, value, sizeof(value));
    ok = expect_string(value, "postgres", "StatefulSet workload fallback") && ok;

    NV_APPS_LOOKUP_LABEL unknown_kind_labels[] = {
        label("k8s_pod_name", "worker-7f8d9c6b5a-q1w2e"),
    };
    fields = fields_from_labels(unknown_kind_labels, _countof(unknown_kind_labels));
    nv_derive_k8s_workload(&fields, value, sizeof(value));
    ok = expect_string(value, "worker", "kindless ReplicaSet-shaped workload fallback") && ok;

    return ok;
}

static bool test_missing_and_fallback_labels(void)
{
    char value[NV_TOPOLOGY_DOCKER_IMAGE_MAX];
    NV_APPS_LOOKUP_FIELDS fields = fields_from_labels(NULL, 0);

    nv_derive_k8s_pod_name(&fields, value, sizeof(value));
    bool ok = expect_string(value, "", "missing pod label should return empty");
    nv_derive_k8s_workload(&fields, value, sizeof(value));
    ok = expect_string(value, "", "missing workload labels should return empty") && ok;
    nv_derive_docker_container_name(&fields, "fallback-cgroup", value, sizeof(value));
    ok = expect_string(value, "fallback-cgroup", "container name should fall back to cgroup name") && ok;
    nv_derive_docker_image(&fields, value, sizeof(value));
    ok = expect_string(value, "", "missing image label should return empty") && ok;

    NV_APPS_LOOKUP_LABEL labels[] = {
        label("container_name", "docker-name"),
    };
    fields = fields_from_labels(labels, _countof(labels));
    nv_derive_docker_container_name(&fields, "fallback-cgroup", value, sizeof(value));
    ok = expect_string(value, "docker-name", "container_name label fallback") && ok;

    return ok;
}

static bool test_systemd_and_orchestrator(void)
{
    char value[NV_TOPOLOGY_SYSTEMD_UNIT_MAX];
    nv_derive_systemd_unit_name("/sys/fs/cgroup/system.slice/nginx.service", value, sizeof(value));
    bool ok = expect_string(value, "nginx.service", "systemd service path");

    nv_derive_systemd_unit_name("/sys/fs/cgroup/user.slice/user-1000.slice/session-2.scope", value, sizeof(value));
    ok = expect_string(value, "session-2.scope", "systemd scope path") && ok;

    ok = expect_string(
             nv_orchestrator_name(NIPC_APPS_CGROUP_KNOWN, NIPC_ORCHESTRATOR_DOCKER),
             "docker",
             "docker orchestrator rendering") && ok;
    ok = expect_string(
             nv_orchestrator_name(NIPC_APPS_CGROUP_HOST_ROOT, NIPC_ORCHESTRATOR_UNKNOWN),
             "host_root",
             "host-root orchestrator rendering") && ok;
    ok = expect_string(
             nv_orchestrator_name(NIPC_APPS_CGROUP_UNKNOWN_PERMANENT, NIPC_ORCHESTRATOR_UNKNOWN),
             "unknown",
             "unknown-permanent orchestrator rendering") && ok;

    return ok;
}

int main(void)
{
    bool ok = true;

    ok = test_whitelist() && ok;
    ok = test_k8s_direct_labels() && ok;
    ok = test_k8s_workload_fallbacks() && ok;
    ok = test_missing_and_fallback_labels() && ok;
    ok = test_systemd_and_orchestrator() && ok;

    return ok ? 0 : 1;
}
