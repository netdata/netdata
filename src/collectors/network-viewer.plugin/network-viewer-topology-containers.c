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

static const char *nv_common_label_lookup(void *data, const char *key)
{
    return nv_cached_label_value(data, key);
}

bool nv_cgroup_fields_have_container_identity(const NV_APPS_LOOKUP_FIELDS *fields)
{
    return fields &&
        cgroup_topology_has_container_identity(
            fields->cgroup_status,
            fields->orchestrator,
            nv_common_label_lookup,
            (void *)fields,
            fields->cgroup_name);
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

static void nv_derive_common_labels(
    const NV_APPS_LOOKUP_FIELDS *fields,
    const char *cgroup_name,
    CGROUP_TOPOLOGY_DERIVED_LABELS *derived)
{
    cgroup_topology_derive_label_fields(
        nv_common_label_lookup,
        (void *)fields,
        cgroup_name,
        derived);
}

void nv_derive_k8s_pod_name(const NV_APPS_LOOKUP_FIELDS *fields, char *dst, size_t dst_size)
{
    if(!dst || !dst_size)
        return;

    dst[0] = '\0';
    CGROUP_TOPOLOGY_DERIVED_LABELS derived;
    nv_derive_common_labels(fields, NULL, &derived);
    strncpyz(dst, derived.k8s_pod_name, dst_size - 1);
}

void nv_derive_k8s_namespace(const NV_APPS_LOOKUP_FIELDS *fields, char *dst, size_t dst_size)
{
    if(!dst || !dst_size)
        return;

    dst[0] = '\0';
    CGROUP_TOPOLOGY_DERIVED_LABELS derived;
    nv_derive_common_labels(fields, NULL, &derived);
    strncpyz(dst, derived.k8s_namespace, dst_size - 1);
}

void nv_derive_k8s_workload(const NV_APPS_LOOKUP_FIELDS *fields, char *dst, size_t dst_size)
{
    if(!dst || !dst_size)
        return;

    dst[0] = '\0';
    CGROUP_TOPOLOGY_DERIVED_LABELS derived;
    nv_derive_common_labels(fields, NULL, &derived);
    strncpyz(dst, derived.k8s_workload, dst_size - 1);
}

void nv_derive_docker_container_name(
    const NV_APPS_LOOKUP_FIELDS *fields,
    const char *cgroup_name,
    char *dst,
    size_t dst_size)
{
    if(!dst || !dst_size)
        return;

    dst[0] = '\0';
    CGROUP_TOPOLOGY_DERIVED_LABELS derived;
    nv_derive_common_labels(fields, cgroup_name, &derived);
    strncpyz(dst, derived.container_name, dst_size - 1);
}

void nv_derive_docker_image(const NV_APPS_LOOKUP_FIELDS *fields, char *dst, size_t dst_size)
{
    if(!dst || !dst_size)
        return;

    dst[0] = '\0';
    CGROUP_TOPOLOGY_DERIVED_LABELS derived;
    nv_derive_common_labels(fields, NULL, &derived);
    strncpyz(dst, derived.docker_image, dst_size - 1);
}

void nv_derive_systemd_unit_name(const char *cgroup_path, char *dst, size_t dst_size)
{
    if(!dst || !dst_size)
        return;

    dst[0] = '\0';
    (void)cgroup_topology_derive_systemd_unit(cgroup_path, dst, dst_size, NULL, 0);
}
