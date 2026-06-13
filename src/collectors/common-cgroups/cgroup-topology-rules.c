// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-topology-rules.h"

#include <ctype.h>

typedef struct {
    const char *suffix;
    const char *kind;
    const char *actor_type;
    const char *actor_label;
    const char *actor_icon;
} CGROUP_TOPOLOGY_SYSTEMD_RULE;

typedef struct {
    uint16_t orchestrator;
    const char *orchestrator_name;
    const char *actor_type;
    const char *actor_kind;
    const char *actor_label;
    const char *actor_icon;
} CGROUP_TOPOLOGY_ORCHESTRATOR_RULE;

static const CGROUP_TOPOLOGY_SYSTEMD_RULE systemd_rules[] = {
    { ".service", "service", "systemd_service", "Systemd service", "systemd" },
    { ".scope", "scope", "systemd_scope", "Systemd scope", "systemd" },
    { ".slice", "slice", "systemd_slice", "Systemd slice", "systemd" },
    { ".socket", "socket", "systemd_socket", "Systemd socket", "systemd" },
    { ".target", "target", "systemd_target", "Systemd target", "systemd" },
    { ".timer", "timer", "systemd_timer", "Systemd timer", "systemd" },
    { ".mount", "mount", "systemd_mount", "Systemd mount", "storage" },
    { ".path", "path", "systemd_path", "Systemd path", "systemd" },
    { ".swap", "swap", "systemd_swap", "Systemd swap", "storage" },
    { ".device", "device", "systemd_device", "Systemd device", "device" },
};

static const CGROUP_TOPOLOGY_ORCHESTRATOR_RULE orchestrator_rules[] = {
    { NIPC_ORCHESTRATOR_DOCKER, "docker", "docker_container", "docker_container", "Docker container", "docker" },
    { NIPC_ORCHESTRATOR_K8S, "k8s", "k8s_container", "k8s_container", "Kubernetes container", "kubernetes" },
    { NIPC_ORCHESTRATOR_KVM, "kvm", "vm", "vm", "Virtual machine", "vm" },
    { NIPC_ORCHESTRATOR_LXC, "lxc", "lxc_container", "lxc_container", "LXC container", "lxc" },
    { NIPC_ORCHESTRATOR_PODMAN, "podman", "podman_container", "podman_container", "Podman container", "podman" },
    { NIPC_ORCHESTRATOR_NSPAWN, "nspawn", "nspawn_container", "nspawn_container", "systemd-nspawn container", "nspawn" },
    { NIPC_ORCHESTRATOR_SYSTEMD, "systemd", "systemd_unit", "systemd_unit", "Systemd unit", "systemd" },
};

const char *cgroup_topology_cgroup_status_name(uint16_t cgroup_status)
{
    switch(cgroup_status) {
        case NIPC_APPS_CGROUP_KNOWN:
            return "known";
        case NIPC_APPS_CGROUP_HOST_ROOT:
            return "host_root";
        case NIPC_APPS_CGROUP_UNKNOWN_PERMANENT:
            return "unknown_permanent";
        case NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER:
            return "retry_later";
        default:
            return "unknown";
    }
}

const char *cgroup_topology_orchestrator_name(uint16_t cgroup_status, uint16_t orchestrator)
{
    if(cgroup_status == NIPC_APPS_CGROUP_HOST_ROOT)
        return "host_root";

    if(cgroup_status == NIPC_APPS_CGROUP_UNKNOWN_PERMANENT)
        return "unknown";

    for(size_t i = 0; i < _countof(orchestrator_rules); i++) {
        if(orchestrator_rules[i].orchestrator == orchestrator)
            return orchestrator_rules[i].orchestrator_name;
    }

    return "unknown";
}

typedef struct {
    const char *id;
    const char *name;
    RRDF_FIELD_OPTIONS options;
} CGROUP_TOPOLOGY_RRDF_FIELD;

static const CGROUP_TOPOLOGY_RRDF_FIELD cgroup_topology_rrdf_fields[] = {
    { "CgroupStatus", "Cgroup Status", RRDF_FIELD_OPTS_NONE },
    { "CgroupPath", "Cgroup Path", RRDF_FIELD_OPTS_NONE | RRDF_FIELD_OPTS_FULL_WIDTH },
    { "CgroupName", "Cgroup Name", RRDF_FIELD_OPTS_NONE },
    { "ContainerName", "Container / Service Name", RRDF_FIELD_OPTS_VISIBLE },
    { "Orchestrator", "Orchestrator", RRDF_FIELD_OPTS_VISIBLE },
    { "K8sPodName", "Kubernetes Pod", RRDF_FIELD_OPTS_NONE },
    { "K8sNamespace", "Kubernetes Namespace", RRDF_FIELD_OPTS_NONE },
    { "K8sWorkload", "Kubernetes Workload", RRDF_FIELD_OPTS_NONE },
    { "DockerContainerName", "Container Name", RRDF_FIELD_OPTS_NONE },
    { "DockerImage", "Container Image", RRDF_FIELD_OPTS_NONE | RRDF_FIELD_OPTS_FULL_WIDTH },
    { "SystemdUnitName", "Systemd Unit", RRDF_FIELD_OPTS_NONE },
    { "SystemdUnitKind", "Systemd Unit Kind", RRDF_FIELD_OPTS_NONE },
    { "ActorKind", "Actor Kind", RRDF_FIELD_OPTS_NONE },
};

void cgroup_topology_emit_rrdf_table_fields(BUFFER *wb, size_t *field_id, bool include_actor_type)
{
    if(!wb || !field_id)
        return;

    for(size_t i = 0; i < _countof(cgroup_topology_rrdf_fields); i++) {
        const CGROUP_TOPOLOGY_RRDF_FIELD *field = &cgroup_topology_rrdf_fields[i];
        buffer_rrdf_table_add_field(
            wb, (*field_id)++, field->id, field->name, RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
            RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT, field->options, NULL);
    }

    if(include_actor_type)
        buffer_rrdf_table_add_field(
            wb, (*field_id)++, "ActorType", "Actor Type", RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
            RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT, RRDF_FIELD_OPTS_NONE, NULL);
}

static const CGROUP_TOPOLOGY_SYSTEMD_RULE *cgroup_topology_systemd_rule_for_component(const char *name)
{
    if(!name || !*name)
        return NULL;

    size_t name_len = strlen(name);
    for(size_t i = 0; i < _countof(systemd_rules); i++) {
        size_t suffix_len = strlen(systemd_rules[i].suffix);
        if(name_len >= suffix_len && strcmp(&name[name_len - suffix_len], systemd_rules[i].suffix) == 0)
            return &systemd_rules[i];
    }

    return NULL;
}

static bool cgroup_topology_path_has_user_slice(const char *cgroup_path)
{
    if(!cgroup_path || !*cgroup_path)
        return false;

    if(strcmp(cgroup_path, "user.slice") == 0 ||
       strncmp(cgroup_path, "user.slice/", sizeof("user.slice/") - 1) == 0 ||
       strncmp(cgroup_path, "/user.slice/", sizeof("/user.slice/") - 1) == 0 ||
       strstr(cgroup_path, "/user.slice/") ||
       strstr(cgroup_path, "user.slice_user-"))
        return true;

    return false;
}

static bool cgroup_topology_parse_uid_after_token(
    const char *cgroup_path,
    const char *token,
    uint32_t *uid)
{
    const char *p = cgroup_path;
    size_t token_len = strlen(token);

    while((p = strstr(p, token))) {
        char *end = NULL;
        uint64_t parsed = str2uint64_t(p + token_len, &end);
        if(end != p + token_len && parsed <= UINT32_MAX) {
            if(uid)
                *uid = (uint32_t)parsed;
            return true;
        }
        p += token_len;
    }

    return false;
}

bool cgroup_topology_parse_user_slice_uid(const char *cgroup_path, uint32_t *uid)
{
    if(uid)
        *uid = NIPC_UID_UNSET;

    if(!cgroup_topology_path_has_user_slice(cgroup_path))
        return false;

    if(cgroup_topology_parse_uid_after_token(cgroup_path, "user-", uid))
        return true;

    return cgroup_topology_parse_uid_after_token(cgroup_path, "user@", uid);
}

static void cgroup_topology_copy_label(
    CGROUP_TOPOLOGY_LABEL_LOOKUP_CB lookup_cb,
    void *lookup_data,
    const char *key,
    char *dst,
    size_t dst_size)
{
    if(!dst || !dst_size)
        return;

    dst[0] = '\0';

    if(!lookup_cb || !key || !*key)
        return;

    const char *value = lookup_cb(lookup_data, key);
    if(value && *value)
        strncpyz(dst, value, dst_size - 1);
}

static bool cgroup_topology_suffix_is_alnum_hash(const char *s)
{
    if(!s)
        return false;

    size_t len = 0;
    bool has_digit = false;
    for(const char *p = s; *p; p++) {
        if(!isalnum((unsigned char)*p))
            return false;

        if(++len > 16)
            return false;

        if(isdigit((unsigned char)*p))
            has_digit = true;
    }

    return len >= 5 && has_digit;
}

static bool cgroup_topology_suffix_is_alnum_token(const char *s)
{
    if(!s)
        return false;

    size_t len = 0;
    for(const char *p = s; *p; p++) {
        if(!isalnum((unsigned char)*p))
            return false;

        if(++len > 16)
            return false;
    }

    return len >= 5;
}

static bool cgroup_topology_suffix_is_uint(const char *s)
{
    if(!s || !*s)
        return false;

    for(const char *p = s; *p; p++) {
        if(!isdigit((unsigned char)*p))
            return false;
    }

    return true;
}

static bool cgroup_topology_strip_one_suffix(
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

static bool cgroup_topology_strip_replicaset_suffixes(
    const char *pod_name,
    bool (*suffix_ok)(const char *),
    char *dst,
    size_t dst_size)
{
    char without_pod_hash[CGROUP_TOPOLOGY_K8S_NAME_MAX];
    if(!cgroup_topology_strip_one_suffix(pod_name, suffix_ok, without_pod_hash, sizeof(without_pod_hash)))
        return false;

    return cgroup_topology_strip_one_suffix(without_pod_hash, suffix_ok, dst, dst_size);
}

static void cgroup_topology_derive_k8s_workload(
    CGROUP_TOPOLOGY_LABEL_LOOKUP_CB lookup_cb,
    void *lookup_data,
    char *dst,
    size_t dst_size)
{
    if(!dst || !dst_size)
        return;

    dst[0] = '\0';

    if(!lookup_cb)
        return;

    const char *controller_name = lookup_cb(lookup_data, "k8s_controller_name");
    if(controller_name && *controller_name) {
        strncpyz(dst, controller_name, dst_size - 1);
        return;
    }

    const char *pod_name = lookup_cb(lookup_data, "k8s_pod_name");
    if(!pod_name || !*pod_name)
        return;

    const char *controller_kind = lookup_cb(lookup_data, "k8s_controller_kind");
    if(controller_kind && *controller_kind) {
        if(strcmp(controller_kind, "ReplicaSet") == 0) {
            (void)cgroup_topology_strip_replicaset_suffixes(
                pod_name, cgroup_topology_suffix_is_alnum_token, dst, dst_size);
            return;
        }
        if(strcmp(controller_kind, "DaemonSet") == 0) {
            (void)cgroup_topology_strip_one_suffix(
                pod_name, cgroup_topology_suffix_is_alnum_token, dst, dst_size);
            return;
        }
        if(strcmp(controller_kind, "StatefulSet") == 0) {
            (void)cgroup_topology_strip_one_suffix(
                pod_name, cgroup_topology_suffix_is_uint, dst, dst_size);
            return;
        }
    }

    if(cgroup_topology_strip_replicaset_suffixes(
           pod_name, cgroup_topology_suffix_is_alnum_hash, dst, dst_size))
        return;
    if(cgroup_topology_strip_one_suffix(
           pod_name, cgroup_topology_suffix_is_alnum_hash, dst, dst_size))
        return;
    (void)cgroup_topology_strip_one_suffix(pod_name, cgroup_topology_suffix_is_uint, dst, dst_size);
}

void cgroup_topology_derive_label_fields(
    CGROUP_TOPOLOGY_LABEL_LOOKUP_CB lookup_cb,
    void *lookup_data,
    const char *cgroup_name,
    CGROUP_TOPOLOGY_DERIVED_LABELS *out)
{
    if(!out)
        return;

    *out = (CGROUP_TOPOLOGY_DERIVED_LABELS){ 0 };

    cgroup_topology_copy_label(
        lookup_cb, lookup_data, "k8s_pod_name", out->k8s_pod_name, sizeof(out->k8s_pod_name));
    cgroup_topology_copy_label(
        lookup_cb, lookup_data, "k8s_namespace", out->k8s_namespace, sizeof(out->k8s_namespace));
    cgroup_topology_derive_k8s_workload(
        lookup_cb, lookup_data, out->k8s_workload, sizeof(out->k8s_workload));
    cgroup_topology_copy_label(
        lookup_cb, lookup_data, "image", out->docker_image, sizeof(out->docker_image));

    const char *container_name = lookup_cb ? lookup_cb(lookup_data, "k8s_container_name") : NULL;
    if(!container_name || !*container_name)
        container_name = lookup_cb ? lookup_cb(lookup_data, "container_name") : NULL;
    if(!container_name || !*container_name)
        container_name = cgroup_name;
    if(container_name && *container_name)
        strncpyz(out->container_name, container_name, sizeof(out->container_name) - 1);
}

static bool cgroup_topology_orchestrator_uses_cgroup_name_identity(uint16_t orchestrator)
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

bool cgroup_topology_has_container_identity(
    uint16_t cgroup_status,
    uint16_t orchestrator,
    CGROUP_TOPOLOGY_LABEL_LOOKUP_CB lookup_cb,
    void *lookup_data,
    const char *cgroup_name)
{
    if(cgroup_status != NIPC_APPS_CGROUP_KNOWN)
        return false;

    const char *label = lookup_cb ? lookup_cb(lookup_data, "k8s_container_name") : NULL;
    if(label && *label)
        return true;

    label = lookup_cb ? lookup_cb(lookup_data, "container_name") : NULL;
    if(label && *label)
        return true;

    if(!cgroup_topology_orchestrator_uses_cgroup_name_identity(orchestrator))
        return false;

    return cgroup_name && *cgroup_name;
}

bool cgroup_topology_derive_systemd_unit(
    const char *cgroup_path,
    char *unit_name,
    size_t unit_name_size,
    char *unit_kind,
    size_t unit_kind_size)
{
    if(unit_name && unit_name_size)
        unit_name[0] = '\0';
    if(unit_kind && unit_kind_size)
        unit_kind[0] = '\0';

    if(!cgroup_path || !*cgroup_path || !unit_name || !unit_name_size)
        return false;

    char path[CGROUP_TOPOLOGY_SYSTEMD_UNIT_MAX];
    strncpyz(path, cgroup_path, sizeof(path) - 1);

    char *candidate = NULL;
    const CGROUP_TOPOLOGY_SYSTEMD_RULE *rule = NULL;
    char *remaining = path;
    char *component;
    while(remaining && (component = strsep_skip_consecutive_separators(&remaining, "/"))) {
        component = trim(component);
        const CGROUP_TOPOLOGY_SYSTEMD_RULE *candidate_rule = cgroup_topology_systemd_rule_for_component(component);
        if(candidate_rule) {
            candidate = component;
            rule = candidate_rule;
        }
    }

    if(!candidate || !*candidate || !rule)
        return false;

    strncpyz(unit_name, candidate, unit_name_size - 1);
    if(unit_kind && unit_kind_size)
        strncpyz(unit_kind, rule->kind, unit_kind_size - 1);

    return true;
}

static void cgroup_topology_set_classification(
    CGROUP_TOPOLOGY_CLASSIFICATION *out,
    const char *effective_orchestrator,
    const char *actor_type,
    const char *actor_kind,
    const char *actor_label,
    const char *actor_icon)
{
    strncpyz(out->effective_orchestrator, effective_orchestrator ? effective_orchestrator : "unknown", sizeof(out->effective_orchestrator) - 1);
    strncpyz(out->actor_type, actor_type ? actor_type : "container", sizeof(out->actor_type) - 1);
    strncpyz(out->actor_kind, actor_kind ? actor_kind : "unknown", sizeof(out->actor_kind) - 1);
    out->actor_label = actor_label ? actor_label : "Container";
    out->actor_icon = actor_icon ? actor_icon : "container";
}

void cgroup_topology_classify(
    uint16_t cgroup_status,
    uint16_t orchestrator,
    const char *cgroup_path,
    CGROUP_TOPOLOGY_CLASSIFICATION *out)
{
    if(!out)
        return;

    *out = (CGROUP_TOPOLOGY_CLASSIFICATION){ 0 };

    bool has_systemd_unit = cgroup_topology_derive_systemd_unit(
        cgroup_path,
        out->systemd_unit_name,
        sizeof(out->systemd_unit_name),
        out->systemd_unit_kind,
        sizeof(out->systemd_unit_kind));

    if(cgroup_status == NIPC_APPS_CGROUP_KNOWN && orchestrator != NIPC_ORCHESTRATOR_UNKNOWN &&
       orchestrator != NIPC_ORCHESTRATOR_SYSTEMD) {
        for(size_t i = 0; i < _countof(orchestrator_rules); i++) {
            if(orchestrator_rules[i].orchestrator == orchestrator) {
                cgroup_topology_set_classification(
                    out,
                    orchestrator_rules[i].orchestrator_name,
                    orchestrator_rules[i].actor_type,
                    orchestrator_rules[i].actor_kind,
                    orchestrator_rules[i].actor_label,
                    orchestrator_rules[i].actor_icon);
                return;
            }
        }
    }

    if(has_systemd_unit) {
        const CGROUP_TOPOLOGY_SYSTEMD_RULE *rule =
            cgroup_topology_systemd_rule_for_component(out->systemd_unit_name);
        cgroup_topology_set_classification(
            out, "systemd",
            rule ? rule->actor_type : "systemd_unit",
            rule ? rule->kind : "systemd_unit",
            rule ? rule->actor_label : "Systemd unit",
            rule ? rule->actor_icon : "service");
        return;
    }

    if(cgroup_status == NIPC_APPS_CGROUP_HOST_ROOT || cgroup_status == NIPC_APPS_CGROUP_UNKNOWN_PERMANENT) {
        cgroup_topology_set_classification(out, cgroup_topology_orchestrator_name(cgroup_status, orchestrator),
                                           "process_group", "process", "Process", "process");
        return;
    }

    if(cgroup_status != NIPC_APPS_CGROUP_KNOWN) {
        cgroup_topology_set_classification(out, "unknown", "container", "pending", "Pending actor", "unknown");
        return;
    }

    for(size_t i = 0; i < _countof(orchestrator_rules); i++) {
        if(orchestrator_rules[i].orchestrator == orchestrator) {
            cgroup_topology_set_classification(
                out,
                orchestrator_rules[i].orchestrator_name,
                orchestrator_rules[i].actor_type,
                orchestrator_rules[i].actor_kind,
                orchestrator_rules[i].actor_label,
                orchestrator_rules[i].actor_icon);
            return;
        }
    }

    cgroup_topology_set_classification(out, "unknown", "container", "unknown_container", "Container", "container");
}
