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

static bool expect_uint(uint32_t actual, uint32_t expected, const char *message)
{
    if(actual == expected)
        return true;

    fprintf(stderr, "%s: got '%u', expected '%u'\n", message, actual, expected);
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

static bool test_container_identity_gating(void)
{
    bool ok = true;

    NV_APPS_LOOKUP_FIELDS rootless_docker = {
        .cgroup_status = NIPC_APPS_CGROUP_KNOWN,
        .orchestrator = NIPC_ORCHESTRATOR_DOCKER,
        .cgroup_path = "/sys/fs/cgroup/user.slice/user-1000.slice/docker-abcdef.scope",
        .cgroup_name = "rootless-docker",
    };
    ok = expect_true(
             nv_cgroup_fields_have_container_identity(&rootless_docker),
             "known rootless Docker cgroup should keep cgroups-provided identity") && ok;

    NV_APPS_LOOKUP_LABEL labels[] = {
        label("container_name", "labeled-container"),
    };
    NV_APPS_LOOKUP_FIELDS labeled_container = fields_from_labels(labels, _countof(labels));
    labeled_container.cgroup_path = "/sys/fs/cgroup/user.slice/user-1001.slice/docker-fedcba.scope";
    ok = expect_true(
             nv_cgroup_fields_have_container_identity(&labeled_container),
             "known cgroup with container label should keep cgroups-provided identity") && ok;

    NV_APPS_LOOKUP_FIELDS user_slice_retry = {
        .cgroup_status = NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER,
        .orchestrator = NIPC_ORCHESTRATOR_UNKNOWN,
        .cgroup_path = "/sys/fs/cgroup/user.slice/user-1002.slice/app.slice/app-code.scope",
        .cgroup_name = "ignored-user-session",
    };
    ok = expect_true(
             !nv_cgroup_fields_have_container_identity(&user_slice_retry),
             "unknown user.slice cgroup should use network-viewer user fallback") && ok;

    NV_APPS_LOOKUP_FIELDS known_without_name = {
        .cgroup_status = NIPC_APPS_CGROUP_KNOWN,
        .orchestrator = NIPC_ORCHESTRATOR_UNKNOWN,
        .cgroup_path = "/sys/fs/cgroup/user.slice/user-1003.slice/session-2.scope",
    };
    ok = expect_true(
             !nv_cgroup_fields_have_container_identity(&known_without_name),
             "known cgroup without name or labels should still allow user fallback") && ok;

    NV_APPS_LOOKUP_FIELDS known_user_slice_with_name = {
        .cgroup_status = NIPC_APPS_CGROUP_KNOWN,
        .orchestrator = NIPC_ORCHESTRATOR_UNKNOWN,
        .cgroup_path = "/sys/fs/cgroup/user.slice/user-1003.slice/user@1003.service/app.slice/app-code.scope",
        .cgroup_name = "app-code",
    };
    ok = expect_true(
             !nv_cgroup_fields_have_container_identity(&known_user_slice_with_name),
             "known user.slice cgroup name should still allow user fallback") && ok;

    NV_APPS_LOOKUP_FIELDS systemd_service_with_name = {
        .cgroup_status = NIPC_APPS_CGROUP_KNOWN,
        .orchestrator = NIPC_ORCHESTRATOR_SYSTEMD,
        .cgroup_path = "/sys/fs/cgroup/system.slice/sshd.service",
        .cgroup_name = "sshd.service",
    };
    ok = expect_true(
             !nv_cgroup_fields_have_container_identity(&systemd_service_with_name),
             "systemd service cgroup name should not claim container identity") && ok;

    return ok;
}

static bool test_process_fallback_container_fields(void)
{
    NV_TOPOLOGY_CONTAINER_FIELDS fields;
    nv_container_fields_set_process_fallback(&fields, NULL);

    bool ok = expect_string(fields.container_name, "[unknown]", "missing process fallback container name");
    ok = expect_string(fields.cgroup_status, "unknown", "missing process fallback cgroup status") && ok;
    ok = expect_string(fields.orchestrator, "unknown", "missing process fallback orchestrator") && ok;
    ok = expect_string(fields.actor_type, "process_group", "missing process fallback actor type") && ok;
    ok = expect_string(fields.actor_kind, "process", "missing process fallback actor kind") && ok;
    ok = expect_true(!fields.from_cache, "missing process fallback must not look cache-derived") && ok;

    nv_container_fields_set_process_fallback(&fields, "postgres");
    ok = expect_string(fields.container_name, "postgres", "process fallback container name") && ok;
    ok = expect_string(fields.cgroup_status, "unknown", "process fallback cgroup status") && ok;
    ok = expect_string(fields.actor_type, "process_group", "process fallback actor type") && ok;
    ok = expect_string(fields.actor_kind, "process", "process fallback actor kind") && ok;

    return ok;
}

static bool test_retry_later_empty_path_fallback_gate(void)
{
    bool ok = true;

    ok = expect_true(
             nv_cgroup_retry_later_without_path(NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER, NULL),
             "retry-later NULL cgroup path should keep process fallback") && ok;
    ok = expect_true(
             nv_cgroup_retry_later_without_path(NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER, ""),
             "retry-later empty cgroup path should keep process fallback") && ok;
    ok = expect_true(
             !nv_cgroup_retry_later_without_path(
                 NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER,
                 "/sys/fs/cgroup/docker/0123456789abcdef"),
             "retry-later non-empty cgroup path should remain classifiable") && ok;
    ok = expect_true(
             !nv_cgroup_retry_later_without_path(NIPC_APPS_CGROUP_HOST_ROOT, NULL),
             "host-root empty cgroup path should remain classifiable") && ok;
    ok = expect_true(
             !nv_cgroup_retry_later_without_path(NIPC_APPS_CGROUP_KNOWN, ""),
             "known empty cgroup path should not use retry-later fallback gate") && ok;

    return ok;
}

static bool test_namespace_relative_non_known_container_scope(void)
{
    const char *path =
        "/../../kubepods.slice/kubepods-besteffort.slice/podabc/"
        "cri-containerd-0123456789abcdef.scope";
    const char *fallback = "worker";
    NV_TOPOLOGY_CONTAINER_FIELDS fields;

    nv_container_fields_set_process_fallback(&fields, fallback);

    CGROUP_TOPOLOGY_CLASSIFICATION classification;
    cgroup_topology_classify(
        NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER,
        NIPC_ORCHESTRATOR_UNKNOWN,
        path,
        &classification);

    strncpyz(fields.actor_type, classification.actor_type, sizeof(fields.actor_type) - 1);
    strncpyz(fields.actor_kind, classification.actor_kind, sizeof(fields.actor_kind) - 1);
    strncpyz(fields.systemd_unit_name, classification.systemd_unit_name, sizeof(fields.systemd_unit_name) - 1);

    bool use_systemd_unit_name = strncmp(fields.actor_type, "systemd_", strlen("systemd_")) == 0;
    bool ok =
        expect_string(fields.actor_type, "container", "namespace-relative retry-later actor type") &&
        expect_string(fields.actor_kind, "pending", "namespace-relative retry-later actor kind") &&
        expect_true(!use_systemd_unit_name, "namespace-relative retry-later must not use systemd unit name") &&
        expect_string(fields.container_name, fallback, "namespace-relative retry-later should keep process fallback");

    nv_container_fields_set_process_fallback(&fields, fallback);
    cgroup_topology_classify(
        NIPC_APPS_CGROUP_UNKNOWN_PERMANENT,
        NIPC_ORCHESTRATOR_UNKNOWN,
        path,
        &classification);
    strncpyz(fields.actor_type, classification.actor_type, sizeof(fields.actor_type) - 1);
    strncpyz(fields.actor_kind, classification.actor_kind, sizeof(fields.actor_kind) - 1);
    strncpyz(fields.systemd_unit_name, classification.systemd_unit_name, sizeof(fields.systemd_unit_name) - 1);

    use_systemd_unit_name = strncmp(fields.actor_type, "systemd_", strlen("systemd_")) == 0;
    if(!use_systemd_unit_name)
        strncpyz(fields.container_name, fallback, sizeof(fields.container_name) - 1);

    ok = expect_true(!use_systemd_unit_name, "namespace-relative unknown-permanent must not use systemd unit name") && ok;
    ok = expect_string(fields.container_name, fallback, "namespace-relative unknown-permanent should keep process fallback") && ok;

    return ok;
}

static bool test_systemd_and_orchestrator(void)
{
    char value[NV_TOPOLOGY_SYSTEMD_UNIT_MAX];
    nv_derive_systemd_unit_name("/sys/fs/cgroup/system.slice/nginx.service", value, sizeof(value));
    bool ok = expect_string(value, "nginx.service", "systemd service path");

    nv_derive_systemd_unit_name("/sys/fs/cgroup/user.slice/user-1000.slice/session-2.scope", value, sizeof(value));
    ok = expect_string(value, "session-2.scope", "systemd scope path") && ok;

    char unit_kind[NV_TOPOLOGY_SYSTEMD_KIND_MAX];
    ok = expect_true(
             cgroup_topology_derive_systemd_unit(
                 "/sys/fs/cgroup/system.slice/system-cups.slice/cups.service",
                 value, sizeof(value), unit_kind, sizeof(unit_kind)),
             "nested systemd service should be classified") && ok;
    ok = expect_string(value, "cups.service", "nested systemd service name") && ok;
    ok = expect_string(unit_kind, "service", "nested systemd service kind") && ok;

    CGROUP_TOPOLOGY_CLASSIFICATION classification;
    cgroup_topology_classify(
        NIPC_APPS_CGROUP_KNOWN, NIPC_ORCHESTRATOR_UNKNOWN,
        "/sys/fs/cgroup/user.slice/user-1000.slice/app.slice/app-code.scope",
        &classification);
    ok = expect_string(classification.effective_orchestrator, "systemd", "systemd scope effective orchestrator") && ok;
    ok = expect_string(classification.actor_type, "systemd_scope", "systemd scope actor type") && ok;
    ok = expect_string(classification.actor_kind, "scope", "systemd scope actor kind") && ok;
    ok = expect_string(classification.actor_icon, "systemd", "systemd scope actor icon") && ok;

    uint32_t user_slice_uid = NIPC_UID_UNSET;
    ok = expect_true(
             cgroup_topology_parse_user_slice_uid(
                 "/sys/fs/cgroup/user.slice/user-1001.slice/app.slice/app-code.scope",
                 &user_slice_uid),
             "user.slice path should be detected") && ok;
    ok = expect_uint(user_slice_uid, 1001, "user.slice uid should be parsed") && ok;

    user_slice_uid = NIPC_UID_UNSET;
    ok = expect_true(
             cgroup_topology_parse_user_slice_uid(
                 "user.slice_user-1002.slice_user@1002.service_app.slice_app-code.scope",
                 &user_slice_uid),
             "normalized user.slice path should be detected") && ok;
    ok = expect_uint(user_slice_uid, 1002, "normalized user.slice uid should be parsed") && ok;

    user_slice_uid = NIPC_UID_UNSET;
    ok = expect_true(
             !cgroup_topology_parse_user_slice_uid(
                 "/sys/fs/cgroup/system.slice/nginx.service",
                 &user_slice_uid),
             "system.slice service should not be treated as user.slice") && ok;
    ok = expect_uint(user_slice_uid, NIPC_UID_UNSET, "non-user.slice uid should remain unset") && ok;

    cgroup_topology_classify(
        NIPC_APPS_CGROUP_KNOWN, NIPC_ORCHESTRATOR_DOCKER,
        "/sys/fs/cgroup/docker/container.scope",
        &classification);
    ok = expect_string(classification.actor_type, "docker_container", "known container orchestrator should win over systemd-shaped path") && ok;
    ok = expect_string(classification.actor_icon, "docker", "known container orchestrator icon should win over systemd-shaped path") && ok;

    cgroup_topology_classify(
        NIPC_APPS_CGROUP_KNOWN, NIPC_ORCHESTRATOR_DOCKER,
        "/sys/fs/cgroup/docker/0123456789abcdef",
        &classification);
    ok = expect_string(classification.actor_type, "docker_container", "docker actor type") && ok;
    ok = expect_string(classification.actor_icon, "docker", "docker actor icon") && ok;

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
    ok = expect_string(
             nv_cgroup_status_name(NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER),
             "retry_later",
             "retry-later cgroup status rendering") && ok;
    ok = expect_string(
             nv_cgroup_status_name(NIPC_APPS_CGROUP_KNOWN),
             "known",
             "known cgroup status rendering") && ok;

    return ok;
}

int main(void)
{
    bool ok = true;

    ok = test_whitelist() && ok;
    ok = test_k8s_direct_labels() && ok;
    ok = test_k8s_workload_fallbacks() && ok;
    ok = test_missing_and_fallback_labels() && ok;
    ok = test_container_identity_gating() && ok;
    ok = test_process_fallback_container_fields() && ok;
    ok = test_retry_later_empty_path_fallback_gate() && ok;
    ok = test_namespace_relative_non_known_container_scope() && ok;
    ok = test_systemd_and_orchestrator() && ok;

    return ok ? 0 : 1;
}
