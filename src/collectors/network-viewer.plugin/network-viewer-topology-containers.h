// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETWORK_VIEWER_TOPOLOGY_CONTAINERS_H
#define NETDATA_NETWORK_VIEWER_TOPOLOGY_CONTAINERS_H

#include "collectors/common-cgroups/cgroup-topology-rules.h"
#include "network-viewer-apps-lookup-client.h"

#define NV_TOPOLOGY_CGROUP_PATH_MAX CGROUP_TOPOLOGY_CGROUP_PATH_MAX
#define NV_TOPOLOGY_CGROUP_NAME_MAX CGROUP_TOPOLOGY_CGROUP_NAME_MAX
#define NV_TOPOLOGY_ORCHESTRATOR_MAX CGROUP_TOPOLOGY_ORCHESTRATOR_MAX
#define NV_TOPOLOGY_K8S_NAME_MAX CGROUP_TOPOLOGY_K8S_NAME_MAX
#define NV_TOPOLOGY_DOCKER_IMAGE_MAX CGROUP_TOPOLOGY_DOCKER_IMAGE_MAX
#define NV_TOPOLOGY_SYSTEMD_UNIT_MAX CGROUP_TOPOLOGY_SYSTEMD_UNIT_MAX
#define NV_TOPOLOGY_SYSTEMD_KIND_MAX CGROUP_TOPOLOGY_SYSTEMD_KIND_MAX
#define NV_TOPOLOGY_ACTOR_TYPE_MAX CGROUP_TOPOLOGY_ACTOR_TYPE_MAX
#define NV_TOPOLOGY_ACTOR_KIND_MAX CGROUP_TOPOLOGY_ACTOR_KIND_MAX

SIMPLE_PATTERN *nv_label_whitelist_parse(const char *pattern);
const char *nv_cgroup_status_name(uint16_t cgroup_status);
const char *nv_orchestrator_name(uint16_t cgroup_status, uint16_t orchestrator);
const char *nv_cached_label_value(const NV_APPS_LOOKUP_FIELDS *fields, const char *key);

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
