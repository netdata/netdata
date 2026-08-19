// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps-cgroups-enrichment.h"

#if defined(OS_LINUX)

#include "apps-cgroups-lookup-client.h"

#define APPS_ENRICHMENT_PENDING_CONTAINER_NAME "[pending]"

static const char *apps_cgroup_label_value(const struct cgroup_lookup_entry *entry, const char *key)
{
    if(!entry || !key || !*key)
        return NULL;

    for(uint16_t i = 0; i < entry->cgroup_label_count; i++) {
        const char *candidate = entry->cgroup_labels[i].key ? string2str(entry->cgroup_labels[i].key) : NULL;
        if(candidate && strcmp(candidate, key) == 0)
            return entry->cgroup_labels[i].value ? string2str(entry->cgroup_labels[i].value) : NULL;
    }

    return NULL;
}

static const char *apps_common_label_lookup(void *data, const char *key)
{
    return apps_cgroup_label_value(data, key);
}

static void apps_process_enrichment_set_process_fallback(APPS_PROCESS_ENRICHMENT *out, const char *process)
{
    if(!out)
        return;

    *out = (APPS_PROCESS_ENRICHMENT){ 0 };
    const char *fallback = (process && *process) ? process : "[unknown]";
    strncpyz(out->container_name, fallback, sizeof(out->container_name) - 1);
    strncpyz(out->cgroup_status, cgroup_topology_cgroup_status_name(UINT16_MAX), sizeof(out->cgroup_status) - 1);
    strncpyz(out->orchestrator, "unknown", sizeof(out->orchestrator) - 1);
    strncpyz(out->actor_kind, "process", sizeof(out->actor_kind) - 1);
    strncpyz(out->actor_type, "process_group", sizeof(out->actor_type) - 1);
}

static void apps_process_enrichment_apply_classification(
    APPS_PROCESS_ENRICHMENT *out,
    uint16_t cgroup_status,
    uint16_t orchestrator,
    const char *path)
{
    if(!out)
        return;

    CGROUP_TOPOLOGY_CLASSIFICATION classification;
    cgroup_topology_classify(cgroup_status, orchestrator, path, &classification);

    strncpyz(out->cgroup_status, cgroup_topology_cgroup_status_name(cgroup_status), sizeof(out->cgroup_status) - 1);
    strncpyz(out->orchestrator, classification.effective_orchestrator, sizeof(out->orchestrator) - 1);
    strncpyz(out->systemd_unit_name, classification.systemd_unit_name, sizeof(out->systemd_unit_name) - 1);
    strncpyz(out->systemd_unit_kind, classification.systemd_unit_kind, sizeof(out->systemd_unit_kind) - 1);
    strncpyz(out->actor_kind, classification.actor_kind, sizeof(out->actor_kind) - 1);
    strncpyz(out->actor_type, classification.actor_type, sizeof(out->actor_type) - 1);
}

void apps_process_enrichment_fill(struct pid_stat *p, APPS_PROCESS_ENRICHMENT *out)
{
    if(!out)
        return;

    const char *process = p && p->comm ? string2str(p->comm) : "[unknown]";
    apps_process_enrichment_set_process_fallback(out, process);
    if(!p || !p->cgroup_path)
        return;

    const char *path = string2str(p->cgroup_path);
    strncpyz(out->cgroup_path, path, sizeof(out->cgroup_path) - 1);

    if(apps_cgroups_lookup_is_host_root_path(path)) {
        apps_process_enrichment_apply_classification(
            out, NIPC_APPS_CGROUP_HOST_ROOT, NIPC_ORCHESTRATOR_UNKNOWN, path);
        strncpyz(out->container_name, process && *process ? process : "[unknown]", sizeof(out->container_name) - 1);
        return;
    }

    if(!p->cgroup_cache) {
        apps_process_enrichment_apply_classification(
            out, NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER, NIPC_ORCHESTRATOR_UNKNOWN, path);
        if(strncmp(out->actor_type, "systemd_", strlen("systemd_")) == 0 && out->systemd_unit_name[0])
            strncpyz(out->container_name, out->systemd_unit_name, sizeof(out->container_name) - 1);
        else if(strcmp(out->actor_kind, "pending") == 0)
            strncpyz(out->container_name, APPS_ENRICHMENT_PENDING_CONTAINER_NAME, sizeof(out->container_name) - 1);
        return;
    }

    struct cgroup_lookup_entry *entry = p->cgroup_cache;
    uint16_t cgroup_status = NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER;
    uint16_t orchestrator = NIPC_ORCHESTRATOR_UNKNOWN;

    if(entry->cgroup_status == NIPC_CGROUP_LOOKUP_KNOWN) {
        cgroup_status = NIPC_APPS_CGROUP_KNOWN;
        orchestrator = entry->orchestrator;
    }
    else if(entry->cgroup_status == NIPC_CGROUP_LOOKUP_UNKNOWN_PERMANENT) {
        cgroup_status = NIPC_APPS_CGROUP_UNKNOWN_PERMANENT;
    }

    apps_process_enrichment_apply_classification(out, cgroup_status, orchestrator, path);
    bool use_systemd_unit_name = strncmp(out->actor_type, "systemd_", strlen("systemd_")) == 0;

    if(cgroup_status != NIPC_APPS_CGROUP_KNOWN) {
        if(use_systemd_unit_name && out->systemd_unit_name[0])
            strncpyz(out->container_name, out->systemd_unit_name, sizeof(out->container_name) - 1);
        else if(cgroup_status == NIPC_APPS_CGROUP_UNKNOWN_PERMANENT)
            strncpyz(out->container_name, process && *process ? process : "[unknown]", sizeof(out->container_name) - 1);
        else if(strcmp(out->actor_kind, "pending") == 0)
            strncpyz(out->container_name, APPS_ENRICHMENT_PENDING_CONTAINER_NAME, sizeof(out->container_name) - 1);
        return;
    }

    const char *cgroup_name = entry->cgroup_name ? string2str(entry->cgroup_name) : "";
    strncpyz(out->cgroup_name, cgroup_name, sizeof(out->cgroup_name) - 1);
    CGROUP_TOPOLOGY_DERIVED_LABELS derived;
    cgroup_topology_derive_label_fields(apps_common_label_lookup, entry, cgroup_name, &derived);
    strncpyz(out->k8s_pod_name, derived.k8s_pod_name, sizeof(out->k8s_pod_name) - 1);
    strncpyz(out->k8s_namespace, derived.k8s_namespace, sizeof(out->k8s_namespace) - 1);
    strncpyz(out->k8s_workload, derived.k8s_workload, sizeof(out->k8s_workload) - 1);
    strncpyz(out->docker_container_name, derived.container_name, sizeof(out->docker_container_name) - 1);
    strncpyz(out->docker_image, derived.docker_image, sizeof(out->docker_image) - 1);

    if(use_systemd_unit_name && out->systemd_unit_name[0])
        strncpyz(out->container_name, out->systemd_unit_name, sizeof(out->container_name) - 1);
    else if(out->docker_container_name[0])
        strncpyz(out->container_name, out->docker_container_name, sizeof(out->container_name) - 1);
    else if(out->cgroup_name[0])
        strncpyz(out->container_name, out->cgroup_name, sizeof(out->container_name) - 1);
    else
        strncpyz(out->container_name, process && *process ? process : "[unknown]", sizeof(out->container_name) - 1);
}

static void apps_add_enrichment_value(BUFFER *wb, const char *value)
{
    buffer_json_add_array_item_string(wb, value && *value ? value : NULL);
}

void apps_emit_process_enrichment_values(BUFFER *wb, const APPS_PROCESS_ENRICHMENT *enrichment)
{
    apps_add_enrichment_value(wb, enrichment->cgroup_status);
    apps_add_enrichment_value(wb, enrichment->cgroup_path);
    apps_add_enrichment_value(wb, enrichment->cgroup_name);
    apps_add_enrichment_value(wb, enrichment->container_name);
    apps_add_enrichment_value(wb, enrichment->orchestrator);
    apps_add_enrichment_value(wb, enrichment->k8s_pod_name);
    apps_add_enrichment_value(wb, enrichment->k8s_namespace);
    apps_add_enrichment_value(wb, enrichment->k8s_workload);
    apps_add_enrichment_value(wb, enrichment->docker_container_name);
    apps_add_enrichment_value(wb, enrichment->docker_image);
    apps_add_enrichment_value(wb, enrichment->systemd_unit_name);
    apps_add_enrichment_value(wb, enrichment->systemd_unit_kind);
    apps_add_enrichment_value(wb, enrichment->actor_kind);
    apps_add_enrichment_value(wb, enrichment->actor_type);
}

#endif
