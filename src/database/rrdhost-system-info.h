// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDHOST_SYSTEM_INFO_H
#define NETDATA_RRDHOST_SYSTEM_INFO_H

#include "libnetdata/libnetdata.h"
#include "rrdlabels.h"
#include "daemon/daemon-status-file.h"

#ifdef RRDHOST_SYSTEM_INFO_INTERNALS
struct rrdhost_system_info {
    char *cloud_provider_type;
    char *cloud_instance_type;
    char *cloud_instance_region;

    char *host_os_name;
    char *host_os_id;
    char *host_os_id_like;
    char *host_os_version;
    char *host_os_version_id;
    char *host_os_detection;
    char *host_cores;
    char *host_cpu_freq;
    char *host_cpu_model;
    char *host_ram_total;
    char *host_disk_space;
    char *container_os_name;
    char *container_os_id;
    char *container_os_id_like;
    char *container_os_version;
    char *container_os_version_id;
    char *container_os_detection;
    char *kernel_name;
    char *kernel_version;
    char *architecture;
    char *virtualization;
    char *virt_detection;
    char *container;
    char *container_detection;
    char *is_k8s_node;
    int16_t hops;
    bool ml_capable;
    bool ml_enabled;
    char *install_type;
    char *prebuilt_arch;
    char *prebuilt_dist;
    int mc_version;
};
#else
struct rrdhost_system_info;
#endif

// --------------------------------------------------------------------------------------------------------------------
// allocation and free

struct rrdhost_system_info *rrdhost_system_info_create(void);
void rrdhost_system_info_free(struct rrdhost_system_info *system_info);

// --------------------------------------------------------------------------------------------------------------------
// importing fields

// detect system info on current system
int rrdhost_system_info_detect(struct rrdhost_system_info *system_info);

// import from host rrdlabels
struct rrdhost_system_info *rrdhost_system_info_from_host_labels(RRDLABELS *labels);

// --------------------------------------------------------------------------------------------------------------------
// setting individual fields

void rrdhost_system_info_hops_set(struct rrdhost_system_info *si, int16_t hops);
void rrdhost_system_info_ml_capable_set(struct rrdhost_system_info *system_info, bool capable);
void rrdhost_system_info_ml_enabled_set(struct rrdhost_system_info *system_info, bool enabled);
void rrdhost_system_info_mc_version_set(struct rrdhost_system_info *system_info, int version);

int rrdhost_system_info_set_by_name(struct rrdhost_system_info *system_info, char *name, char *value);

// --------------------------------------------------------------------------------------------------------------------
// reading individual fields

const char *rrdhost_system_info_install_type(struct rrdhost_system_info *si);
const char *rrdhost_system_info_prebuilt_dist(struct rrdhost_system_info *si);
int16_t rrdhost_system_info_hops(struct rrdhost_system_info *si);

// --------------------------------------------------------------------------------------------------------------------
// exporting in various forms

void rrdhost_system_info_to_rrdlabels(struct rrdhost_system_info *system_info, RRDLABELS *labels);

void rrdhost_system_info_to_json_v1(BUFFER *wb, struct rrdhost_system_info *system_info);
void rrdhost_system_info_to_json_v2(BUFFER *wb, struct rrdhost_system_info *system_info);
void rrdhost_system_info_to_url_encode_stream(BUFFER *wb, struct rrdhost_system_info *system_info);

typedef int (*add_host_sysinfo_key_value_t)(const char *name, const char *value, nd_uuid_t *uuid);
int rrdhost_system_info_foreach(struct rrdhost_system_info *system_info, add_host_sysinfo_key_value_t cb, nd_uuid_t *uuid);

struct update_node_info;
void rrdhost_system_info_to_node_info(struct rrdhost_system_info *system_info, struct update_node_info *node_info);

void rrdhost_system_info_to_streaming_function_array(BUFFER *wb, struct rrdhost_system_info *system_info);

void get_daemon_status_fields_from_system_info(DAEMON_STATUS_FILE *ds);
void rrdhost_system_info_swap(struct rrdhost_system_info *a, struct rrdhost_system_info *b);

#endif //NETDATA_RRDHOST_SYSTEM_INFO_H
