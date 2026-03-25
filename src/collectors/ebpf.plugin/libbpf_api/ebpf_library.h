// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COLLECTOR_EBPF_LIBRARY_H
#define NETDATA_COLLECTOR_EBPF_LIBRARY_H 1

#include <stdint.h>
#include "../ebpf_socket_ipc.h"

typedef struct netdata_publish_syscall netdata_publish_syscall_t;
typedef struct netdata_syscall_stat netdata_syscall_stat_t;
typedef struct ebpf_module ebpf_module_t;
typedef struct ebpf_target ebpf_target_t;
typedef struct ebpf_tracepoint ebpf_tracepoint_t;
typedef struct aral ARAL;
typedef struct config config;

/*****************************************************************
 *
 *  DIMENSION WRITING FUNCTIONS
 *
 *****************************************************************/

void write_chart_dimension(const char *dim, long long value);
void ebpf_write_global_dimension(char *name, char *id, char *algorithm);
void ebpf_create_global_dimension(void *ptr, int end);

/*****************************************************************
 *
 *  CHART WRITING FUNCTIONS
 *
 *****************************************************************/

void write_count_chart(char *name, char *family, netdata_publish_syscall_t *move, uint32_t end);
void write_err_chart(char *name, char *family, netdata_publish_syscall_t *move, int end);
void ebpf_one_dimension_write_charts(char *family, char *chart, char *dim, long long v1);
void write_io_chart(char *chart, char *family, char *dwrite, long long vwrite, char *dread, long long vread);
void write_histogram_chart(char *family, char *name, const uint64_t *hist, char **dimensions, uint32_t end);

/*****************************************************************
 *
 *  CHART CREATION FUNCTIONS
 *
 *****************************************************************/

void ebpf_write_chart_cmd(
    char *type,
    char *id,
    char *suffix,
    char *title,
    char *units,
    char *family,
    char *charttype,
    char *context,
    int order,
    int update_every,
    char *module);

void ebpf_write_chart_obsolete(
    char *type,
    const char *id,
    char *suffix,
    char *title,
    char *units,
    char *family,
    char *charttype,
    const char *context,
    int order,
    int update_every);

void ebpf_create_chart(
    char *type,
    char *id,
    char *title,
    char *units,
    char *family,
    char *context,
    char *charttype,
    int order,
    void (*ncd)(void *, int),
    void *move,
    int end,
    int update_every,
    char *module);

/*****************************************************************
 *
 *  ARAL STATISTIC CHARTS
 *
 *****************************************************************/

int ebpf_statistic_create_aral_chart(char *name, ebpf_module_t *em);
void ebpf_statistic_obsolete_aral_chart(ebpf_module_t *em, int prio);
void ebpf_send_data_aral_chart(ARAL *memory, ebpf_module_t *em);

/*****************************************************************
 *
 *  CONFIG FILE PARSER FUNCTIONS
 *
 *****************************************************************/

void ebpf_how_to_load(const char *ptr);
void ebpf_set_apps_mode(netdata_apps_integration_flags_t value);
void ebpf_update_interval(int update_every);
void ebpf_update_table_size();
void ebpf_update_lifetime();
void ebpf_set_load_mode(netdata_ebpf_load_mode_t load, netdata_ebpf_load_mode_t origin);
void ebpf_update_load_mode(const char *str, netdata_ebpf_load_mode_t origin);
void ebpf_update_map_per_core();
void ebpf_set_ipc_value(const char *integration);
void ebpf_parse_ipc_section();
void ebpf_set_thread_mode(netdata_run_mode_t lmode);
void ebpf_enable_chart(int idx, int disable_cgroup);
void ebpf_enable_specific_chart(ebpf_module_t *em, int disable_cgroup);
void read_collector_values(int *disable_cgroups, int update_every, netdata_ebpf_load_mode_t origin);
void parse_network_viewer_section(struct config *cfg);
void ebpf_parse_service_name_section(struct config *cfg);
void ebpf_parse_ports(const char *ptr);
void ebpf_parse_ips_unsafe(const char *ptr);
void ebpf_read_local_addresses_unsafe();
int ebpf_load_collector_config(char *path, int *disable_cgroups, int update_every);
void ebpf_load_thread_config();

/*****************************************************************
 *
 *  FUNCTIONS TO CREATE CHARTS
 *
 *****************************************************************/

void ebpf_create_apps_for_module(ebpf_module_t *em, ebpf_target_t *root);
void ebpf_create_apps_charts(ebpf_target_t *root);

/*****************************************************************
 *
 *  FUNCTIONS TO READ GLOBAL HASH TABLES
 *
 *****************************************************************/

void ebpf_read_global_table_stats(
    netdata_idx_t *stats,
    netdata_idx_t *values,
    int map_fd,
    int maps_per_core,
    uint32_t begin,
    uint32_t end);

/*****************************************************************
 *
 *  FUNCTIONS TO DEFINE OPTIONS
 *
 *****************************************************************/

void ebpf_global_labels(
    netdata_syscall_stat_t *is,
    netdata_publish_syscall_t *pio,
    char **dim,
    char **name,
    int *algorithm,
    int end);

void disable_all_global_charts();
void ebpf_disable_cgroups();
void ebpf_update_disabled_plugin_stats(ebpf_module_t *em);
void ebpf_print_help();

/*****************************************************************
 *
 *  TRACEPOINT MANAGEMENT FUNCTIONS
 *
 *****************************************************************/

int ebpf_enable_tracepoint(ebpf_tracepoint_t *tp);
int ebpf_disable_tracepoint(ebpf_tracepoint_t *tp);
uint32_t ebpf_enable_tracepoints(ebpf_tracepoint_t *tps);

/*****************************************************************
 *
 *  AUXILIARY FUNCTIONS USED DURING INITIALIZATION
 *
 *****************************************************************/

void read_local_ports(char *filename, uint8_t proto);

#endif /* NETDATA_COLLECTOR_EBPF_LIBRARY_H */
