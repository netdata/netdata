// SPDX-License-Identifier: GPL-3.0-or-later

#include "network-viewer-topology-containers.h"

SIMPLE_PATTERN *nv_label_whitelist_parse(const char *pattern)
{
    if (!pattern || !*pattern)
        return NULL;

    return simple_pattern_create(pattern, "|", SIMPLE_PATTERN_EXACT, true);
}

const char *nv_cgroup_status_name(uint16_t cgroup_status)
{
    return cgroup_topology_cgroup_status_name(cgroup_status);
}

const char *nv_orchestrator_name(uint16_t cgroup_status, uint16_t orchestrator)
{
    return cgroup_topology_orchestrator_name(cgroup_status, orchestrator);
}

const char *nv_cached_label_value(const NV_APPS_LOOKUP_FIELDS *fields, const char *key)
{
    if (!fields || !key || !*key)
        return NULL;

    for (uint16_t i = 0; i < fields->cgroup_label_count; i++) {
        if (fields->cgroup_labels[i].key && strcmp(fields->cgroup_labels[i].key, key) == 0)
            return fields->cgroup_labels[i].value;
    }

    return NULL;
}

static bool nv_orchestrator_uses_cgroup_name_identity(uint16_t orchestrator)
{
    switch(orchestrator) {
        case NIPC_ORCHESTRATOR_DOCKER:
        case NIPC_ORCHESTRATOR_K8S:
        case NIPC_ORCHESTRATOR_KVM:
        case NIPC_ORCHESTRATOR_LXC:
        case NIPC_ORCHESTRATOR_PODMAN:
        case NIPC_ORCHESTRATOR_NSPAWN:
            return true;
        default:
            return false;
    }
}

bool nv_cgroup_fields_have_container_identity(const NV_APPS_LOOKUP_FIELDS *fields)
{
    if(!fields || fields->cgroup_status != NIPC_APPS_CGROUP_KNOWN)
        return false;

    const char *label = nv_cached_label_value(fields, "k8s_container_name");
    if(label && *label)
        return true;

    label = nv_cached_label_value(fields, "container_name");
    if(label && *label)
        return true;

    if(!nv_orchestrator_uses_cgroup_name_identity(fields->orchestrator))
        return false;

    char container_name[NV_TOPOLOGY_CONTAINER_NAME_MAX];
    nv_derive_docker_container_name(fields, fields->cgroup_name, container_name, sizeof(container_name));
    return container_name[0] != '\0';
}

void nv_container_fields_set_process_fallback(NV_TOPOLOGY_CONTAINER_FIELDS *fields, const char *process)
{
    if(!fields)
        return;

    *fields = (NV_TOPOLOGY_CONTAINER_FIELDS){ 0 };
    const char *fallback = (process && *process) ? process : "[unknown]";
    strncpyz(fields->container_name, fallback, sizeof(fields->container_name) - 1);
    strncpyz(fields->cgroup_status, nv_cgroup_status_name(UINT16_MAX), sizeof(fields->cgroup_status) - 1);
    strncpyz(fields->orchestrator, "unknown", sizeof(fields->orchestrator) - 1);
    strncpyz(fields->actor_type, "process_group", sizeof(fields->actor_type) - 1);
    strncpyz(fields->actor_kind, "process", sizeof(fields->actor_kind) - 1);
}

bool nv_cgroup_retry_later_without_path(uint16_t cgroup_status, const char *cgroup_path)
{
    return cgroup_status == NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER && (!cgroup_path || !*cgroup_path);
}

static void nv_copy_label(const NV_APPS_LOOKUP_FIELDS *fields, const char *key, char *dst, size_t dst_size)
{
    if (!dst || !dst_size)
        return;

    dst[0] = '\0';
    const char *value = nv_cached_label_value(fields, key);
    if (value && *value)
        strncpyz(dst, value, dst_size - 1);
}

void nv_derive_k8s_pod_name(const NV_APPS_LOOKUP_FIELDS *fields, char *dst, size_t dst_size)
{
    nv_copy_label(fields, "k8s_pod_name", dst, dst_size);
}

void nv_derive_k8s_namespace(const NV_APPS_LOOKUP_FIELDS *fields, char *dst, size_t dst_size)
{
    nv_copy_label(fields, "k8s_namespace", dst, dst_size);
}

static bool nv_suffix_is_alnum_hash(const char *s)
{
    if (!s || !*s)
        return false;

    size_t len = strlen(s);
    if (len < 5 || len > 16)
        return false;

    bool has_digit = false;
    for (const char *p = s; *p; p++) {
        if (!isalnum((unsigned char)*p))
            return false;
        if (isdigit((unsigned char)*p))
            has_digit = true;
    }

    return has_digit;
}

static bool nv_suffix_is_alnum_token(const char *s)
{
    if (!s || !*s)
        return false;

    size_t len = strlen(s);
    if (len < 5 || len > 16)
        return false;

    for (const char *p = s; *p; p++) {
        if (!isalnum((unsigned char)*p))
            return false;
    }

    return true;
}

static bool nv_suffix_is_uint(const char *s)
{
    if (!s || !*s)
        return false;

    for (const char *p = s; *p; p++) {
        if (!isdigit((unsigned char)*p))
            return false;
    }

    return true;
}

static bool nv_strip_one_suffix(
    const char *value,
    bool (*suffix_ok)(const char *),
    char *dst,
    size_t dst_size)
{
    if (!value || !*value || !suffix_ok || !dst || !dst_size)
        return false;

    const char *dash = strrchr(value, '-');
    if (!dash || dash == value || !suffix_ok(dash + 1))
        return false;

    size_t len = (size_t)(dash - value);
    if (len >= dst_size)
        len = dst_size - 1;
    memcpy(dst, value, len);
    dst[len] = '\0';
    return len > 0;
}

static bool nv_strip_replicaset_suffixes(
    const char *pod_name,
    bool (*suffix_ok)(const char *),
    char *dst,
    size_t dst_size)
{
    char without_pod_hash[NV_TOPOLOGY_K8S_NAME_MAX];
    if (!nv_strip_one_suffix(pod_name, suffix_ok, without_pod_hash, sizeof(without_pod_hash)))
        return false;

    return nv_strip_one_suffix(without_pod_hash, suffix_ok, dst, dst_size);
}

void nv_derive_k8s_workload(const NV_APPS_LOOKUP_FIELDS *fields, char *dst, size_t dst_size)
{
    if (!dst || !dst_size)
        return;

    dst[0] = '\0';

    const char *controller_name = nv_cached_label_value(fields, "k8s_controller_name");
    if (controller_name && *controller_name) {
        strncpyz(dst, controller_name, dst_size - 1);
        return;
    }

    const char *pod_name = nv_cached_label_value(fields, "k8s_pod_name");
    if (!pod_name || !*pod_name)
        return;

    const char *controller_kind = nv_cached_label_value(fields, "k8s_controller_kind");
    if (controller_kind && *controller_kind) {
        if (strcmp(controller_kind, "ReplicaSet") == 0) {
            (void)nv_strip_replicaset_suffixes(pod_name, nv_suffix_is_alnum_token, dst, dst_size);
            return;
        }
        if (strcmp(controller_kind, "DaemonSet") == 0) {
            (void)nv_strip_one_suffix(pod_name, nv_suffix_is_alnum_token, dst, dst_size);
            return;
        }
        if (strcmp(controller_kind, "StatefulSet") == 0) {
            (void)nv_strip_one_suffix(pod_name, nv_suffix_is_uint, dst, dst_size);
            return;
        }
    }

    if (nv_strip_replicaset_suffixes(pod_name, nv_suffix_is_alnum_hash, dst, dst_size))
        return;
    if (nv_strip_one_suffix(pod_name, nv_suffix_is_alnum_hash, dst, dst_size))
        return;
    (void)nv_strip_one_suffix(pod_name, nv_suffix_is_uint, dst, dst_size);
}

void nv_derive_docker_container_name(
    const NV_APPS_LOOKUP_FIELDS *fields,
    const char *cgroup_name,
    char *dst,
    size_t dst_size)
{
    if (!dst || !dst_size)
        return;

    dst[0] = '\0';

    const char *value = nv_cached_label_value(fields, "k8s_container_name");
    if (!value || !*value)
        value = nv_cached_label_value(fields, "container_name");
    if (!value || !*value)
        value = cgroup_name;

    if (value && *value)
        strncpyz(dst, value, dst_size - 1);
}

void nv_derive_docker_image(const NV_APPS_LOOKUP_FIELDS *fields, char *dst, size_t dst_size)
{
    nv_copy_label(fields, "image", dst, dst_size);
}

void nv_derive_systemd_unit_name(const char *cgroup_path, char *dst, size_t dst_size)
{
    if(!dst || !dst_size)
        return;

    dst[0] = '\0';
    (void)cgroup_topology_derive_systemd_unit(cgroup_path, dst, dst_size, NULL, 0);
}
