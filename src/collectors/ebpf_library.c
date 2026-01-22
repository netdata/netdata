// SPDX-License-Identifier: GPL-3.0-or-later

#include <stdio.h>
#include <stdlib.h>

#include "libnetdata/libnetdata.h"
#include "ebpf_library.h"
#include "ebpf.plugin/ebpf.h"
#include "ebpf.plugin/libbpf_api/ebpf.h"
#include "ebpf.plugin/ebpf_process.h"

extern char *ebpf_algorithms[];

/*****************************************************************
 *
 *  DIMENSION WRITING FUNCTIONS
 *
 *****************************************************************/

void write_chart_dimension(char *dim, long long value)
{
    printf("SET %s = %lld\n", dim, value);
}

void ebpf_write_global_dimension(char *name, char *id, char *algorithm)
{
    printf("DIMENSION %s %s %s 1 1\n", name, id, algorithm);
}

void ebpf_create_global_dimension(void *ptr, int end)
{
    netdata_publish_syscall_t *move = ptr;

    int i = 0;
    while (move && i < end) {
        ebpf_write_global_dimension(move->name, move->dimension, move->algorithm);

        move = move->next;
        i++;
    }
}

/*****************************************************************
 *
 *  CHART WRITING FUNCTIONS
 *
 *****************************************************************/

void write_count_chart(char *name, char *family, netdata_publish_syscall_t *move, uint32_t end)
{
    ebpf_write_begin_chart(family, name, "");

    uint32_t i = 0;
    while (move && i < end) {
        write_chart_dimension(move->name, move->ncall);

        move = move->next;
        i++;
    }

    ebpf_write_end_chart();
}

void write_err_chart(char *name, char *family, netdata_publish_syscall_t *move, int end)
{
    ebpf_write_begin_chart(family, name, "");

    int i = 0;
    while (move && i < end) {
        write_chart_dimension(move->name, move->nerr);

        move = move->next;
        i++;
    }

    ebpf_write_end_chart();
}

void ebpf_one_dimension_write_charts(char *family, char *chart, char *dim, long long v1)
{
    ebpf_write_begin_chart(family, chart, "");

    write_chart_dimension(dim, v1);

    ebpf_write_end_chart();
}

void write_io_chart(char *chart, char *family, char *dwrite, long long vwrite, char *dread, long long vread)
{
    ebpf_write_begin_chart(family, chart, "");

    write_chart_dimension(dwrite, vwrite);
    write_chart_dimension(dread, vread);

    ebpf_write_end_chart();
}

void write_histogram_chart(char *family, char *name, const uint64_t *hist, char **dimensions, uint32_t end)
{
    ebpf_write_begin_chart(family, name, "");

    uint32_t i;
    for (i = 0; i < end; i++) {
        write_chart_dimension(dimensions[i], (long long)hist[i]);
    }

    ebpf_write_end_chart();

    fflush(stdout);
}

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
    char *module)
{
    printf(
        "CHART %s.%s%s '' '%s' '%s' '%s' '%s' '%s' %d %d '' 'ebpf.plugin' '%s'\n",
        type,
        id,
        suffix,
        title,
        units,
        (family) ? family : "",
        (context) ? context : "",
        (charttype) ? charttype : "",
        order,
        update_every,
        module);
}

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
    int update_every)
{
    printf(
        "CHART %s.%s%s '' '%s' '%s' '%s' '%s' '%s' %d %d 'obsolete'\n",
        type,
        id,
        suffix,
        title,
        units,
        (family) ? family : "",
        (context) ? context : "",
        (charttype) ? charttype : "",
        order,
        update_every);
}

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
    char *module)
{
    ebpf_write_chart_cmd(type, id, "", title, units, family, charttype, context, order, update_every, module);

    if (ncd) {
        ncd(move, end);
    }
}

/*****************************************************************
 *
 *  ARAL STATISTIC CHARTS
 *
 *****************************************************************/

int ebpf_statistic_create_aral_chart(char *name, ebpf_module_t *em)
{
    static int priority = NETATA_EBPF_ORDER_STAT_ARAL_BEGIN;
    char *mem = {NETDATA_EBPF_STAT_DIMENSION_MEMORY};
    char *aral = {NETDATA_EBPF_STAT_DIMENSION_ARAL};

    snprintfz(em->memory_usage, NETDATA_EBPF_CHART_MEM_LENGTH - 1, "aral_%s_size", name);
    snprintfz(em->memory_allocations, NETDATA_EBPF_CHART_MEM_LENGTH - 1, "aral_%s_alloc", name);

    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        em->memory_usage,
        "",
        "Bytes allocated for ARAL.",
        "bytes",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        "netdata.ebpf_aral_stat_size",
        priority++,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(mem, mem, ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        em->memory_allocations,
        "",
        "Calls to allocate memory.",
        "calls",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        "netdata.ebpf_aral_stat_alloc",
        priority++,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(aral, aral, ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    return priority - 2;
}

void ebpf_statistic_obsolete_aral_chart(ebpf_module_t *em, int prio)
{
    ebpf_write_chart_obsolete(
        NETDATA_MONITORING_FAMILY,
        em->memory_allocations,
        "",
        "Calls to allocate memory.",
        "calls",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        "netdata.ebpf_aral_stat_alloc",
        prio++,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_MONITORING_FAMILY,
        em->memory_allocations,
        "",
        "Calls to allocate memory.",
        "calls",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        "netdata.ebpf_aral_stat_alloc",
        prio++,
        em->update_every);
}

void ebpf_send_data_aral_chart(ARAL *memory, ebpf_module_t *em)
{
    char *mem = {NETDATA_EBPF_STAT_DIMENSION_MEMORY};
    char *aral = {NETDATA_EBPF_STAT_DIMENSION_ARAL};

    struct aral_statistics *stats = aral_get_statistics(memory);

    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, em->memory_usage, "");
    write_chart_dimension(mem, (long long)stats->structures.allocated_bytes);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, em->memory_allocations, "");
    write_chart_dimension(aral, (long long)stats->structures.allocations);
    ebpf_write_end_chart();
}
