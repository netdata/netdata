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

static void apps_copy_label(const struct cgroup_lookup_entry *entry, const char *key, char *dst, size_t dst_size)
{
    if(!dst || !dst_size)
        return;

    dst[0] = '\0';
    const char *value = apps_cgroup_label_value(entry, key);
    if(value && *value)
        strncpyz(dst, value, dst_size - 1);
}

static bool apps_suffix_is_alnum_hash(const char *s)
{
    if(!s || !*s)
        return false;

    size_t len = strlen(s);
    if(len < 5 || len > 16)
        return false;

    bool has_digit = false;
    for(const char *p = s; *p; p++) {
        if(!isalnum((unsigned char)*p))
            return false;
        if(isdigit((unsigned char)*p))
            has_digit = true;
    }

    return has_digit;
}

static bool apps_suffix_is_alnum_token(const char *s)
{
    if(!s || !*s)
        return false;

    size_t len = strlen(s);
    if(len < 5 || len > 16)
        return false;

    for(const char *p = s; *p; p++) {
        if(!isalnum((unsigned char)*p))
            return false;
    }

    return true;
}

static bool apps_suffix_is_uint(const char *s)
{
    if(!s || !*s)
        return false;

    for(const char *p = s; *p; p++) {
        if(!isdigit((unsigned char)*p))
            return false;
    }

    return true;
}

static bool apps_strip_one_suffix(
    const char *value,
    bool (*suffix_ok)(const char *),
    char *dst,
    size_t dst_size)
{
    if(!value || !*value || !suffix_ok || !dst || !dst_size)
        return false;

    const char *dash = strrchr(value, '-');
    if(!dash || dash == value || !suffix_ok(dash + 1))
        return false;

    size_t len = (size_t)(dash - value);
    if(len >= dst_size)
        len = dst_size - 1;
    memcpy(dst, value, len);
    dst[len] = '\0';
    return len > 0;
}

static bool apps_strip_replicaset_suffixes(
    const char *pod_name,
    bool (*suffix_ok)(const char *),
    char *dst,
    size_t dst_size)
{
    char without_pod_hash[APPS_ENRICHMENT_K8S_NAME_MAX];
    if(!apps_strip_one_suffix(pod_name, suffix_ok, without_pod_hash, sizeof(without_pod_hash)))
        return false;

    return apps_strip_one_suffix(without_pod_hash, suffix_ok, dst, dst_size);
}

static void apps_derive_k8s_workload(const struct cgroup_lookup_entry *entry, char *dst, size_t dst_size)
{
    if(!dst || !dst_size)
        return;

    dst[0] = '\0';

    const char *controller_name = apps_cgroup_label_value(entry, "k8s_controller_name");
    if(controller_name && *controller_name) {
        strncpyz(dst, controller_name, dst_size - 1);
        return;
    }

    const char *pod_name = apps_cgroup_label_value(entry, "k8s_pod_name");
    if(!pod_name || !*pod_name)
        return;

    const char *controller_kind = apps_cgroup_label_value(entry, "k8s_controller_kind");
    if(controller_kind && *controller_kind) {
        if(strcmp(controller_kind, "ReplicaSet") == 0) {
            (void)apps_strip_replicaset_suffixes(pod_name, apps_suffix_is_alnum_token, dst, dst_size);
            return;
        }
        if(strcmp(controller_kind, "DaemonSet") == 0) {
            (void)apps_strip_one_suffix(pod_name, apps_suffix_is_alnum_token, dst, dst_size);
            return;
        }
        if(strcmp(controller_kind, "StatefulSet") == 0) {
            (void)apps_strip_one_suffix(pod_name, apps_suffix_is_uint, dst, dst_size);
            return;
        }
    }

    if(apps_strip_replicaset_suffixes(pod_name, apps_suffix_is_alnum_hash, dst, dst_size))
        return;
    if(apps_strip_one_suffix(pod_name, apps_suffix_is_alnum_hash, dst, dst_size))
        return;
    (void)apps_strip_one_suffix(pod_name, apps_suffix_is_uint, dst, dst_size);
}

void apps_process_enrichment_fill(struct pid_stat *p, APPS_PROCESS_ENRICHMENT *out)
{
    if(!out)
        return;

    *out = (APPS_PROCESS_ENRICHMENT){ 0 };
    strncpyz(out->container_name, APPS_ENRICHMENT_PENDING_CONTAINER_NAME, sizeof(out->container_name) - 1);
    strncpyz(out->cgroup_status, cgroup_topology_cgroup_status_name(NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER), sizeof(out->cgroup_status) - 1);
    strncpyz(out->actor_type, "container", sizeof(out->actor_type) - 1);

    const char *process = p && p->comm ? string2str(p->comm) : "[unknown]";
    if(!p || !p->cgroup_path)
        return;

    const char *path = string2str(p->cgroup_path);
    strncpyz(out->cgroup_path, path, sizeof(out->cgroup_path) - 1);

    if(apps_cgroups_lookup_is_host_root_path(path)) {
        CGROUP_TOPOLOGY_CLASSIFICATION classification;
        cgroup_topology_classify(NIPC_APPS_CGROUP_HOST_ROOT, NIPC_ORCHESTRATOR_UNKNOWN, path, &classification);
        strncpyz(out->cgroup_status, cgroup_topology_cgroup_status_name(NIPC_APPS_CGROUP_HOST_ROOT), sizeof(out->cgroup_status) - 1);
        strncpyz(out->orchestrator, classification.effective_orchestrator, sizeof(out->orchestrator) - 1);
        strncpyz(out->actor_kind, classification.actor_kind, sizeof(out->actor_kind) - 1);
        strncpyz(out->actor_type, classification.actor_type, sizeof(out->actor_type) - 1);
        strncpyz(out->container_name, process && *process ? process : "[unknown]", sizeof(out->container_name) - 1);
        return;
    }

    if(!p->cgroup_cache)
        return;

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

    CGROUP_TOPOLOGY_CLASSIFICATION classification;
    cgroup_topology_classify(cgroup_status, orchestrator, path, &classification);

    strncpyz(out->cgroup_status, cgroup_topology_cgroup_status_name(cgroup_status), sizeof(out->cgroup_status) - 1);
    strncpyz(out->orchestrator, classification.effective_orchestrator, sizeof(out->orchestrator) - 1);
    strncpyz(out->systemd_unit_name, classification.systemd_unit_name, sizeof(out->systemd_unit_name) - 1);
    strncpyz(out->systemd_unit_kind, classification.systemd_unit_kind, sizeof(out->systemd_unit_kind) - 1);
    strncpyz(out->actor_kind, classification.actor_kind, sizeof(out->actor_kind) - 1);
    strncpyz(out->actor_type, classification.actor_type, sizeof(out->actor_type) - 1);
    bool use_systemd_unit_name = strncmp(classification.actor_type, "systemd_", strlen("systemd_")) == 0;

    if(cgroup_status != NIPC_APPS_CGROUP_KNOWN) {
        if(use_systemd_unit_name && out->systemd_unit_name[0])
            strncpyz(out->container_name, out->systemd_unit_name, sizeof(out->container_name) - 1);
        else if(cgroup_status == NIPC_APPS_CGROUP_UNKNOWN_PERMANENT)
            strncpyz(out->container_name, process && *process ? process : "[unknown]", sizeof(out->container_name) - 1);
        return;
    }

    const char *cgroup_name = entry->cgroup_name ? string2str(entry->cgroup_name) : "";
    strncpyz(out->cgroup_name, cgroup_name, sizeof(out->cgroup_name) - 1);
    apps_copy_label(entry, "k8s_pod_name", out->k8s_pod_name, sizeof(out->k8s_pod_name));
    apps_copy_label(entry, "k8s_namespace", out->k8s_namespace, sizeof(out->k8s_namespace));
    apps_derive_k8s_workload(entry, out->k8s_workload, sizeof(out->k8s_workload));

    const char *container_name = apps_cgroup_label_value(entry, "k8s_container_name");
    if(!container_name || !*container_name)
        container_name = apps_cgroup_label_value(entry, "container_name");
    if(!container_name || !*container_name)
        container_name = cgroup_name;
    if(container_name && *container_name)
        strncpyz(out->docker_container_name, container_name, sizeof(out->docker_container_name) - 1);

    apps_copy_label(entry, "image", out->docker_image, sizeof(out->docker_image));
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
