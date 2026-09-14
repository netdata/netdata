// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETWORK_VIEWER_TOPOLOGY_CONTAINERS_H
#define NETDATA_NETWORK_VIEWER_TOPOLOGY_CONTAINERS_H

#include "collectors/common-cgroups/cgroup-topology-rules.h"
#include "network-viewer-apps-lookup-client.h"

#define NV_TOPOLOGY_CGROUP_PATH_MAX CGROUP_TOPOLOGY_CGROUP_PATH_MAX
#define NV_TOPOLOGY_CGROUP_NAME_MAX CGROUP_TOPOLOGY_CGROUP_NAME_MAX
#define NV_TOPOLOGY_CONTAINER_NAME_MAX CGROUP_TOPOLOGY_SYSTEMD_UNIT_MAX
#define NV_TOPOLOGY_ORCHESTRATOR_MAX CGROUP_TOPOLOGY_ORCHESTRATOR_MAX
#define NV_TOPOLOGY_K8S_NAME_MAX CGROUP_TOPOLOGY_K8S_NAME_MAX
#define NV_TOPOLOGY_DOCKER_IMAGE_MAX CGROUP_TOPOLOGY_DOCKER_IMAGE_MAX
#define NV_TOPOLOGY_SYSTEMD_UNIT_MAX CGROUP_TOPOLOGY_SYSTEMD_UNIT_MAX
#define NV_TOPOLOGY_SYSTEMD_KIND_MAX CGROUP_TOPOLOGY_SYSTEMD_KIND_MAX
#define NV_TOPOLOGY_ACTOR_TYPE_MAX CGROUP_TOPOLOGY_ACTOR_TYPE_MAX
#define NV_TOPOLOGY_ACTOR_KIND_MAX CGROUP_TOPOLOGY_ACTOR_KIND_MAX

typedef struct {
    char cgroup_path[NV_TOPOLOGY_CGROUP_PATH_MAX];
    char cgroup_name[NV_TOPOLOGY_CGROUP_NAME_MAX];
    char container_name[NV_TOPOLOGY_CONTAINER_NAME_MAX];
    char cgroup_status[32];
    char orchestrator[NV_TOPOLOGY_ORCHESTRATOR_MAX];
    char k8s_pod_name[NV_TOPOLOGY_K8S_NAME_MAX];
    char k8s_namespace[NV_TOPOLOGY_K8S_NAME_MAX];
    char k8s_workload[NV_TOPOLOGY_K8S_NAME_MAX];
    char docker_container_name[NV_TOPOLOGY_CGROUP_NAME_MAX];
    char docker_image[NV_TOPOLOGY_DOCKER_IMAGE_MAX];
    char systemd_unit_name[NV_TOPOLOGY_SYSTEMD_UNIT_MAX];
    char systemd_unit_kind[NV_TOPOLOGY_SYSTEMD_KIND_MAX];
    char actor_kind[NV_TOPOLOGY_ACTOR_KIND_MAX];
    char actor_type[NV_TOPOLOGY_ACTOR_TYPE_MAX];
    bool from_cache;
} NV_TOPOLOGY_CONTAINER_FIELDS;

SIMPLE_PATTERN *nv_label_whitelist_parse(const char *pattern);
const char *nv_cgroup_status_name(uint16_t cgroup_status);
const char *nv_orchestrator_name(uint16_t cgroup_status, uint16_t orchestrator);
const char *nv_cached_label_value(const NV_APPS_LOOKUP_FIELDS *fields, const char *key);
bool nv_cgroup_fields_have_container_identity(const NV_APPS_LOOKUP_FIELDS *fields);
void nv_container_fields_set_process_fallback(NV_TOPOLOGY_CONTAINER_FIELDS *fields, const char *process);
bool nv_cgroup_retry_later_without_path(uint16_t cgroup_status, const char *cgroup_path);

void nv_derive_k8s_pod_name(const NV_APPS_LOOKUP_FIELDS *fields, char *dst, size_t dst_size);
void nv_derive_k8s_namespace(const NV_APPS_LOOKUP_FIELDS *fields, char *dst, size_t dst_size);
void nv_derive_k8s_workload(const NV_APPS_LOOKUP_FIELDS *fields, char *dst, size_t dst_size);
void nv_derive_docker_container_name(
    const NV_APPS_LOOKUP_FIELDS *fields,
    const char *cgroup_name,
    char *dst,
    size_t dst_size);
void nv_derive_docker_image(const NV_APPS_LOOKUP_FIELDS *fields, char *dst, size_t dst_size);
void nv_derive_systemd_unit_name(const char *cgroup_path, char *dst, size_t dst_size);

#endif /* NETDATA_NETWORK_VIEWER_TOPOLOGY_CONTAINERS_H */
