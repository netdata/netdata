// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPS_CGROUPS_ENRICHMENT_H
#define NETDATA_APPS_CGROUPS_ENRICHMENT_H 1

#include "apps_plugin.h"
#include "collectors/common-cgroups/cgroup-topology-rules.h"

#if defined(OS_LINUX)

#define APPS_ENRICHMENT_CGROUP_PATH_MAX 256
#define APPS_ENRICHMENT_CGROUP_NAME_MAX 64
#define APPS_ENRICHMENT_ORCHESTRATOR_MAX 16
#define APPS_ENRICHMENT_K8S_NAME_MAX 64
#define APPS_ENRICHMENT_DOCKER_IMAGE_MAX 256
#define APPS_ENRICHMENT_SYSTEMD_UNIT_MAX 256
#define APPS_ENRICHMENT_SYSTEMD_KIND_MAX 24
#define APPS_ENRICHMENT_ACTOR_KIND_MAX 32

typedef struct {
    char cgroup_status[32];
    char cgroup_path[APPS_ENRICHMENT_CGROUP_PATH_MAX];
    char cgroup_name[APPS_ENRICHMENT_CGROUP_NAME_MAX];
    char container_name[APPS_ENRICHMENT_CGROUP_NAME_MAX];
    char orchestrator[APPS_ENRICHMENT_ORCHESTRATOR_MAX];
    char k8s_pod_name[APPS_ENRICHMENT_K8S_NAME_MAX];
    char k8s_namespace[APPS_ENRICHMENT_K8S_NAME_MAX];
    char k8s_workload[APPS_ENRICHMENT_K8S_NAME_MAX];
    char docker_container_name[APPS_ENRICHMENT_CGROUP_NAME_MAX];
    char docker_image[APPS_ENRICHMENT_DOCKER_IMAGE_MAX];
    char systemd_unit_name[APPS_ENRICHMENT_SYSTEMD_UNIT_MAX];
    char systemd_unit_kind[APPS_ENRICHMENT_SYSTEMD_KIND_MAX];
    char actor_kind[APPS_ENRICHMENT_ACTOR_KIND_MAX];
} APPS_PROCESS_ENRICHMENT;

void apps_process_enrichment_fill(struct pid_stat *p, APPS_PROCESS_ENRICHMENT *out);
void apps_emit_process_enrichment_values(BUFFER *wb, const APPS_PROCESS_ENRICHMENT *enrichment);

#endif

#endif /* NETDATA_APPS_CGROUPS_ENRICHMENT_H */
