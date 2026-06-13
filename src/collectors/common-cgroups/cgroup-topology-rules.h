// SPDX-License-Identifier: GPL-3.0-or-later
// cppcheck-suppress-file unusedStructMember

#ifndef NETDATA_CGROUP_TOPOLOGY_RULES_H
#define NETDATA_CGROUP_TOPOLOGY_RULES_H 1

#include "libnetdata/libnetdata.h"
#include "libnetdata/netipc/netipc_netdata.h"

#define CGROUP_TOPOLOGY_CGROUP_PATH_MAX 256
#define CGROUP_TOPOLOGY_CGROUP_NAME_MAX 64
#define CGROUP_TOPOLOGY_ORCHESTRATOR_MAX 16
#define CGROUP_TOPOLOGY_K8S_NAME_MAX 64
#define CGROUP_TOPOLOGY_DOCKER_IMAGE_MAX 256
#define CGROUP_TOPOLOGY_SYSTEMD_UNIT_MAX 256
#define CGROUP_TOPOLOGY_SYSTEMD_KIND_MAX 24
#define CGROUP_TOPOLOGY_ACTOR_TYPE_MAX 32
#define CGROUP_TOPOLOGY_ACTOR_KIND_MAX 32

typedef struct {
    char systemd_unit_name[CGROUP_TOPOLOGY_SYSTEMD_UNIT_MAX];
    char systemd_unit_kind[CGROUP_TOPOLOGY_SYSTEMD_KIND_MAX];
    char effective_orchestrator[CGROUP_TOPOLOGY_ORCHESTRATOR_MAX];
    char actor_type[CGROUP_TOPOLOGY_ACTOR_TYPE_MAX];
    char actor_kind[CGROUP_TOPOLOGY_ACTOR_KIND_MAX];
    const char *actor_label;
    const char *actor_icon;
} CGROUP_TOPOLOGY_CLASSIFICATION;

typedef const char *(*CGROUP_TOPOLOGY_LABEL_LOOKUP_CB)(void *data, const char *key);

typedef struct {
    char k8s_pod_name[CGROUP_TOPOLOGY_K8S_NAME_MAX];
    char k8s_namespace[CGROUP_TOPOLOGY_K8S_NAME_MAX];
    char k8s_workload[CGROUP_TOPOLOGY_K8S_NAME_MAX];
    char container_name[CGROUP_TOPOLOGY_CGROUP_NAME_MAX];
    char docker_image[CGROUP_TOPOLOGY_DOCKER_IMAGE_MAX];
} CGROUP_TOPOLOGY_DERIVED_LABELS;

const char *cgroup_topology_cgroup_status_name(uint16_t cgroup_status);
const char *cgroup_topology_orchestrator_name(uint16_t cgroup_status, uint16_t orchestrator);

bool cgroup_topology_derive_systemd_unit(
    const char *cgroup_path,
    char *unit_name,
    size_t unit_name_size,
    char *unit_kind,
    size_t unit_kind_size);

bool cgroup_topology_parse_user_slice_uid(const char *cgroup_path, uint32_t *uid);

void cgroup_topology_derive_label_fields(
    CGROUP_TOPOLOGY_LABEL_LOOKUP_CB lookup_cb,
    void *lookup_data,
    const char *cgroup_name,
    CGROUP_TOPOLOGY_DERIVED_LABELS *out);

bool cgroup_topology_has_container_identity(
    uint16_t cgroup_status,
    uint16_t orchestrator,
    CGROUP_TOPOLOGY_LABEL_LOOKUP_CB lookup_cb,
    void *lookup_data,
    const char *cgroup_name);

void cgroup_topology_classify(
    uint16_t cgroup_status,
    uint16_t orchestrator,
    const char *cgroup_path,
    CGROUP_TOPOLOGY_CLASSIFICATION *out);

#endif /* NETDATA_CGROUP_TOPOLOGY_RULES_H */
