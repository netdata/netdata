// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COLLECTOR_EBPF_LIBRARY_H
#define NETDATA_COLLECTOR_EBPF_LIBRARY_H 1

#include <stdint.h>

typedef struct netdata_publish_syscall netdata_publish_syscall_t;
typedef struct ebpf_module ebpf_module_t;
typedef struct aral ARAL;

/*****************************************************************
 *
 *  DIMENSION WRITING FUNCTIONS
 *
 *****************************************************************/

void write_chart_dimension(char *dim, long long value);
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
    char *id,
    char *suffix,
    char *title,
    char *units,
    char *family,
    char *charttype,
    char *context,
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

#endif /* NETDATA_COLLECTOR_EBPF_LIBRARY_H */
