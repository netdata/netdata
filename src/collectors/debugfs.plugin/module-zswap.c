// SPDX-License-Identifier: GPL-3.0-or-later

#include "debugfs_plugin.h"

static long system_page_size = 4096;

static collected_number pages_to_bytes(collected_number value)
{
    return value * system_page_size;
}

struct netdata_zswap_metric {
    const char *filename;

    const char *chart_id;
    const char *title;
    const char *units;
    RRDSET_TYPE charttype;
    int prio;
    const char *dimension;
    RRD_ALGORITHM algorithm;
    int divisor;

    int enabled;
    int chart_created;

    collected_number value;
    collected_number (*convertv)(collected_number v);
};

static struct netdata_zswap_metric zswap_calculated_metrics[] = {
    {.filename = "",
     .chart_id = "pool_compression_ratio",
     .dimension = "compression_ratio",
     .units = "ratio",
     .title = "Zswap compression ratio",
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_LINE,
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_MEM_ZSWAP_COMPRESS_RATIO,
     .divisor = 100,
     .convertv = NULL,
     .value = -1},
};

enum netdata_zswap_calculated {
    NETDATA_ZSWAP_COMPRESSION_RATIO_CHART,
};

enum netdata_zwap_independent {
    NETDATA_ZSWAP_POOL_TOTAL_SIZE,
    NETDATA_ZSWAP_STORED_PAGES,
    NETDATA_ZSWAP_POOL_LIMIT_HIT,
    NETDATA_ZSWAP_WRITTEN_BACK_PAGES,
    NETDATA_ZSWAP_SAME_FILLED_PAGES,
    NETDATA_ZSWAP_DUPLICATE_ENTRY,

    // Terminator
    NETDATA_ZSWAP_SITE_END
};

static struct netdata_zswap_metric zswap_independent_metrics[] = {
    // https://elixir.bootlin.com/linux/latest/source/mm/zswap.c
    {.filename = "/sys/kernel/debug/zswap/pool_total_size",
     .chart_id = "pool_compressed_size",
     .dimension = "compressed_size",
     .units = "bytes",
     .title = "Zswap compressed bytes currently stored",
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_AREA,
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_MEM_ZSWAP_POOL_TOT_SIZE,
     .divisor = 1,
     .convertv = NULL,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/stored_pages",
     .chart_id = "pool_raw_size",
     .dimension = "uncompressed_size",
     .units = "bytes",
     .title = "Zswap uncompressed bytes currently stored",
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_AREA,
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_MEM_ZSWAP_STORED_PAGE,
     .divisor = 1,
     .convertv = pages_to_bytes,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/pool_limit_hit",
     .chart_id = "pool_limit_hit",
     .dimension = "limit",
     .units = "events/s",
     .title = "Zswap pool limit was reached",
     .algorithm = RRD_ALGORITHM_INCREMENTAL,
     .charttype = RRDSET_TYPE_LINE,
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_MEM_ZSWAP_POOL_LIM_HIT,
     .divisor = 1,
     .convertv = NULL,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/written_back_pages",
     .chart_id = "written_back_raw_bytes",
     .dimension = "written_back",
     .units = "bytes/s",
     .title = "Zswap uncomressed bytes written back when pool limit was reached",
     .algorithm = RRD_ALGORITHM_INCREMENTAL,
     .charttype = RRDSET_TYPE_AREA,
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_MEM_ZSWAP_WRT_BACK_PAGES,
     .divisor = 1,
     .convertv = pages_to_bytes,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/same_filled_pages",
     .chart_id = "same_filled_raw_size",
     .dimension = "same_filled",
     .units = "bytes",
     .title = "Zswap same-value filled uncompressed bytes currently stored",
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_AREA,
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_MEM_ZSWAP_SAME_FILL_PAGE,
     .divisor = 1,
     .convertv = pages_to_bytes,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/duplicate_entry",
     .chart_id = "duplicate_entry",
     .dimension = "duplicate",
     .units = "entries/s",
     .title = "Zswap duplicate store was encountered",
     .algorithm = RRD_ALGORITHM_INCREMENTAL,
     .charttype = RRDSET_TYPE_LINE,
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_MEM_ZSWAP_DUPP_ENTRY,
     .divisor = 1,
     .convertv = NULL,
     .value = -1},

    // The terminator
    {.filename = NULL,
     .chart_id = NULL,
     .dimension = NULL,
     .units = NULL,
     .title = NULL,
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_LINE,
     .enabled = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = -1,
     .value = -1}};

enum netdata_zswap_rejected {
    NETDATA_ZSWAP_REJECTED_CHART,
    NETDATA_ZSWAP_REJECTED_COMPRESS_POOR,
    NETDATA_ZSWAP_REJECTED_KMEM_FAIL,
    NETDATA_ZSWAP_REJECTED_RALLOC_FAIL,
    NETDATA_ZSWAP_REJECTED_RRECLAIM_FAIL,

    // Terminator
    NETDATA_ZSWAP_REJECTED_END
};

static struct netdata_zswap_metric zswap_rejected_metrics[] = {
    {.filename = "/sys/kernel/debug/zswap/",
     .chart_id = "rejections",
     .dimension = NULL,
     .units = "rejections/s",
     .title = "Zswap rejections",
     .algorithm = RRD_ALGORITHM_INCREMENTAL,
     .charttype = RRDSET_TYPE_STACKED,
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_MEM_ZSWAP_REJECTS,
     .divisor = 1,
     .convertv = NULL,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/reject_compress_poor",
     .chart_id = "reject_compress_poor",
     .dimension = "compress_poor",
     .units = NULL,
     .title = NULL,
     .algorithm = RRD_ALGORITHM_INCREMENTAL,
     .charttype = RRDSET_TYPE_STACKED,
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_MEM_ZSWAP_REJECTS,
     .divisor = 1,
     .convertv = NULL,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/reject_kmemcache_fail",
     .chart_id = "reject_kmemcache_fail",
     .dimension = "kmemcache_fail",
     .units = NULL,
     .title = NULL,
     .algorithm = RRD_ALGORITHM_INCREMENTAL,
     .charttype = RRDSET_TYPE_STACKED,
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_MEM_ZSWAP_REJECTS,
     .divisor = 1,
     .convertv = NULL,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/reject_alloc_fail",
     .chart_id = "reject_alloc_fail",
     .dimension = "alloc_fail",
     .units = NULL,
     .title = NULL,
     .algorithm = RRD_ALGORITHM_INCREMENTAL,
     .charttype = RRDSET_TYPE_STACKED,
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_MEM_ZSWAP_REJECTS,
     .divisor = 1,
     .convertv = NULL,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/reject_reclaim_fail",
     .chart_id = "reject_reclaim_fail",
     .dimension = "reclaim_fail",
     .units = NULL,
     .title = NULL,
     .algorithm = RRD_ALGORITHM_INCREMENTAL,
     .charttype = RRDSET_TYPE_STACKED,
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_MEM_ZSWAP_REJECTS,
     .divisor = 1,
     .convertv = NULL,
     .value = -1},

    // The terminator
    {.filename = NULL,
     .chart_id = NULL,
     .dimension = NULL,
     .units = NULL,
     .title = NULL,
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_STACKED,
     .enabled = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = -1,
     .value = -1}};

int zswap_collect_data(struct netdata_zswap_metric *metric)
{
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, metric->filename);

    if (read_single_number_file(filename, (unsigned long long *)&metric->value)) {
        netdata_log_error("Cannot read file %s", filename);
        return 1;
    }

    if (metric->convertv)
        metric->value = metric->convertv(metric->value);

    return 0;
}

static void
zswap_send_chart_unsafe(struct netdata_zswap_metric *metric, int update_every, const char *name, const char *option)
{
    printf(
        PLUGINSD_KEYWORD_CHART " mem.zswap_%s '' '%s' '%s' 'zswap' '' '%s' %d %d '%s' 'debugfs.plugin' '%s'\n",
        metric->chart_id,
        metric->title,
        metric->units,
        debugfs_rrdset_type_name(metric->charttype),
        metric->prio,
        update_every,
        (!option) ? "" : option,
        name);
}

static void zswap_send_dimension_unsafe(struct netdata_zswap_metric *metric)
{
    int div = metric->divisor > 0 ? metric->divisor : 1;
    printf(
        PLUGINSD_KEYWORD_DIMENSION " '%s' '%s' %s 1 %d ''\n",
        metric->dimension,
        metric->dimension,
        debugfs_rrd_algorithm_name(metric->algorithm),
        div);
}

static void zswap_send_begin_unsafe(struct netdata_zswap_metric *metric)
{
    printf(PLUGINSD_KEYWORD_BEGIN " mem.zswap_%s\n", metric->chart_id);
}

static void zswap_send_set_unsafe(struct netdata_zswap_metric *metric)
{
    printf(PLUGINSD_KEYWORD_SET " %s = %lld\n", metric->dimension, metric->value);
}

static void zswap_send_end_unsafe()
{
    printf(PLUGINSD_KEYWORD_END "\n");
}

static void zswap_independent_chart(struct netdata_zswap_metric *metric, int update_every, const char *name)
{
    netdata_mutex_lock(&stdout_mutex);

    if (unlikely(!metric->chart_created)) {
        metric->chart_created = CONFIG_BOOLEAN_YES;

        zswap_send_chart_unsafe(metric, update_every, name, NULL);
        zswap_send_dimension_unsafe(metric);
    }

    zswap_send_begin_unsafe(metric);
    zswap_send_set_unsafe(metric);
    zswap_send_end_unsafe();

    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);
}

void zswap_reject_chart(int update_every, const char *name)
{
    netdata_mutex_lock(&stdout_mutex);

    struct netdata_zswap_metric *metric = &zswap_rejected_metrics[NETDATA_ZSWAP_REJECTED_CHART];

    if (unlikely(!metric->chart_created)) {
        metric->chart_created = CONFIG_BOOLEAN_YES;

        zswap_send_chart_unsafe(metric, update_every, name, NULL);
        for (int i = NETDATA_ZSWAP_REJECTED_COMPRESS_POOR; zswap_rejected_metrics[i].filename; i++) {
            metric = &zswap_rejected_metrics[i];
            if (likely(metric->enabled))
                zswap_send_dimension_unsafe(metric);
        }
    }

    metric = &zswap_rejected_metrics[NETDATA_ZSWAP_REJECTED_CHART];
    zswap_send_begin_unsafe(metric);
    for (int i = NETDATA_ZSWAP_REJECTED_COMPRESS_POOR; zswap_rejected_metrics[i].filename; i++) {
        metric = &zswap_rejected_metrics[i];
        if (likely(metric->enabled))
            zswap_send_set_unsafe(metric);
    }
    zswap_send_end_unsafe();

    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);
}

static void zswap_obsolete_charts(int update_every, const char *name)
{
    netdata_mutex_lock(&stdout_mutex);

    struct netdata_zswap_metric *metric = NULL;

    for (int i = 0; zswap_independent_metrics[i].filename; i++) {
        metric = &zswap_independent_metrics[i];
        if (likely(metric->chart_created))
            zswap_send_chart_unsafe(metric, update_every, name, "obsolete");
    }

    metric = &zswap_rejected_metrics[NETDATA_ZSWAP_REJECTED_CHART];
    if (likely(metric->chart_created))
        zswap_send_chart_unsafe(metric, update_every, name, "obsolete");

    metric = &zswap_calculated_metrics[NETDATA_ZSWAP_COMPRESSION_RATIO_CHART];
    if (likely(metric->chart_created))
        zswap_send_chart_unsafe(metric, update_every, name, "obsolete");

    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);
}

#define ZSWAP_STATE_SIZE 1 // Y or N
static int debugfs_is_zswap_enabled()
{
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "/sys/module/zswap/parameters/enabled"); // host prefix is not needed here
    char state[ZSWAP_STATE_SIZE + 1];

    int ret = read_txt_file(filename, state, sizeof(state));

    if (unlikely(!ret && !strcmp(state, "Y"))) {
        return 0;
    }
    return 1;
}

int do_module_zswap(int update_every, const char *name)
{
    static int check_if_enabled = 1;

    if (likely(check_if_enabled && debugfs_is_zswap_enabled())) {
        netdata_log_info("Zswap is disabled");
        return 1;
    }

    check_if_enabled = 0;

    system_page_size = sysconf(_SC_PAGESIZE);
    struct netdata_zswap_metric *metric = NULL;
    int enabled = 0;

    for (int i = 0; zswap_independent_metrics[i].filename; i++) {
        metric = &zswap_independent_metrics[i];
        if (unlikely(!metric->enabled))
            continue;
        if (unlikely(!(metric->enabled = !zswap_collect_data(metric))))
            continue;
        zswap_independent_chart(metric, update_every, name);
        enabled++;
    }

    struct netdata_zswap_metric *metric_size = &zswap_independent_metrics[NETDATA_ZSWAP_POOL_TOTAL_SIZE];
    struct netdata_zswap_metric *metric_raw_size = &zswap_independent_metrics[NETDATA_ZSWAP_STORED_PAGES];
    if (metric_size->enabled && metric_raw_size->enabled) {
        metric = &zswap_calculated_metrics[NETDATA_ZSWAP_COMPRESSION_RATIO_CHART];
        metric->value = 0;
        if (metric_size->value > 0)
            metric->value =
                (collected_number)((NETDATA_DOUBLE)metric_raw_size->value / (NETDATA_DOUBLE)metric_size->value * 100);
        zswap_independent_chart(metric, update_every, name);
    }

    int enabled_rejected = 0;
    for (int i = NETDATA_ZSWAP_REJECTED_COMPRESS_POOR; zswap_rejected_metrics[i].filename; i++) {
        metric = &zswap_rejected_metrics[i];
        if (unlikely(!metric->enabled))
            continue;
        if (unlikely(!(metric->enabled = !zswap_collect_data(metric))))
            continue;
        enabled++;
        enabled_rejected++;
    }

    if (likely(enabled_rejected > 0))
        zswap_reject_chart(update_every, name);

    if (unlikely(!enabled)) {
        zswap_obsolete_charts(update_every, name);
        return 1;
    }

    return 0;
}
