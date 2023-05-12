// SPDX-License-Identifier: GPL-3.0-or-later

#include "debugfs_plugin.h"

struct netdata_zswap_metric {
    const char *filename;

    const char *chart_id;
    const char *dimension;
    const char *units;
    const char *title;
    RRD_ALGORITHM algorithm;
    RRDSET_TYPE charttype;

    int enabled;
    int obsolete;
    int chart_created;

    int prio;

    collected_number value;
};

enum netdata_zwap_independent {
    NETDATA_ZSWAP_SAME_FILLED_PAGE,
    NETDATA_ZSWAP_STORED_PAGE,
    NETDATA_ZSWAP_POOL_TOTAL_SIZE,
    NETDATA_ZSWAP_DUPLICATE_ENTRY,
    NETDATA_ZSWAP_WRITTEN_BACK_PAGE,
    NETDATA_ZSWAP_POOL_LIMIT_HIT,

    // Terminator
    NETDATA_ZSWAP_SITE_END
};

static struct netdata_zswap_metric zswap_independent_metrics[] = {
    // https://elixir.bootlin.com/linux/latest/source/mm/zswap.c
    {.filename = "/sys/kernel/debug/zswap/same_filled_pages",
     .chart_id = "same_filled_page",
     .dimension = "same_filled",
     .units = "pages",
     .title = "Zswap same-value filled pages currently stored",
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_LINE,
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_SAME_FILL_PAGE,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/stored_pages",
     .chart_id = "stored_pages",
     .dimension = "compressed",
     .units = "pages",
     .title = "Zswap compressed pages currently stored",
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_LINE,
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_STORED_PAGE,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/pool_total_size",
     .chart_id = "pool_total_size",
     .dimension = "pool",
     .units = "bytes",
     .title = "Zswap bytes used by the compressed storage",
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_LINE,
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_POOL_TOT_SIZE,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/duplicate_entry",
     .chart_id = "duplicate_entry",
     .dimension = "duplicate",
     .units = "entries/s",
     .title = "Zswap duplicate store was encountered",
     .algorithm = RRD_ALGORITHM_INCREMENTAL,
     .charttype = RRDSET_TYPE_LINE,
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_DUPP_ENTRY,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/written_back_pages",
     .chart_id = "written_back_pages",
     .dimension = "written_back",
     .units = "pages/s",
     .title = "Zswap pages written back when pool limit was reached",
     .algorithm = RRD_ALGORITHM_INCREMENTAL,
     .charttype = RRDSET_TYPE_LINE,
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_STORED_PAGE,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/pool_limit_hit",
     .chart_id = "pool_limit_hit",
     .dimension = "limit",
     .units = "events/s",
     .title = "Zswap pool limit was reached",
     .algorithm = RRD_ALGORITHM_INCREMENTAL,
     .charttype = RRDSET_TYPE_LINE,
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_POOL_LIM_HIT,
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
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = -1,
     .value = -1}
};

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
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_LINE,
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_REJECTS,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/reject_compress_poor",
     .chart_id = "reject_compress_poor",
     .dimension = "compress_poor",
     .units = NULL,
     .title = NULL,
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_STACKED,
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_REJECTS,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/reject_kmemcache_fail",
     .chart_id = "reject_kmemcache_fail",
     .dimension = "kmemcache_fail",
     .units = NULL,
     .title = NULL,
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_STACKED,
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_REJECTS,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/reject_alloc_fail",
     .chart_id = "reject_alloc_fail",
     .dimension = "alloc_fail",
     .units = NULL,
     .title = NULL,
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_STACKED,
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_REJECTS},
    {.filename = "/sys/kernel/debug/zswap/reject_reclaim_fail",
     .chart_id = "reject_reclaim_fail",
     .dimension = "reclaim_fail",
     .units = NULL,
     .title = NULL,
     .algorithm = RRD_ALGORITHM_ABSOLUTE,
     .charttype = RRDSET_TYPE_STACKED,
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_REJECTS,
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
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = -1,
     .value = -1}
};

int zswap_collect_data(struct netdata_zswap_metric *metric) {
    int fd;
    int ret = 0;
    char buffer[512];

    char filename[FILENAME_MAX + 1];
    snprintfz(filename,
              FILENAME_MAX,
              "%s%s",
              netdata_configured_host_prefix,
              metric->filename);
    // we are not using profile_open/procfile_read, because they will generate error during runtime.
    fd = open(filename, O_RDONLY, 0444);
    if (fd < 0 ) {
        metric->obsolete = CONFIG_BOOLEAN_YES;
        error("Cannot open file %s", filename);
        return -1;
    }

    ssize_t r = read(fd, buffer, 511);
    // We expect at list 1 character
    if (r < 2) {
        ret = -1;
        goto zswap_charts_end;
    }
    // We discard breakline
    buffer[r - 1] = '\0';
    metric->value = str2ll(buffer, NULL);

zswap_charts_end:
    close(fd);
    return ret;
}

static void zswap_send_chart(struct netdata_zswap_metric *metric,
                                         int update_every,
                                         const char *name,
                                         const char *option)
{
    fprintf(
        stdout,
        "CHART system.zswap_%s '' '%s' '%s' 'zswap' '' '%s' %d %d '%s' 'debugfs.plugin' '%s'\n",
        metric->chart_id,
        metric->title,
        metric->units,
        debugfs_rrdset_type_name(metric->charttype),
        metric->prio,
        update_every,
        (!option)? "" : option,
        name);
}

static void zswap_independent_chart(struct netdata_zswap_metric *metric, int update_every, const char *name)
{
    if (unlikely(!metric->chart_created)) {
        metric->chart_created = CONFIG_BOOLEAN_YES;
        zswap_send_chart(metric, update_every, name, NULL);
        fprintf(
            stdout,
            "DIMENSION '%s' '%s' %s 1 1 ''\n",
            metric->dimension,
            metric->dimension,
            debugfs_rrd_algorithm_name(metric->algorithm));
    }

    fprintf(stdout, "BEGIN system.zswap_%s\n"
                    "SET %s = %lld\n"
                    "END\n",
            metric->chart_id, metric->dimension, metric->value);
    fflush(stdout);
}

void zswap_reject_chart(int update_every, const char *name) {
    int i;
    struct netdata_zswap_metric *metric = &zswap_rejected_metrics[NETDATA_ZSWAP_REJECTED_CHART];
    if (unlikely(!metric->chart_created)) {
        metric->chart_created = CONFIG_BOOLEAN_YES;
        zswap_send_chart(metric, update_every, name, NULL);
        for (i = NETDATA_ZSWAP_REJECTED_COMPRESS_POOR; zswap_rejected_metrics[i].filename; i++) {
            metric = &zswap_rejected_metrics[i];
            if (likely(metric->enabled))
                fprintf(stdout,
                    "DIMENSION '%s' '%s' %s 1 1 ''\n",
                    metric->dimension,
                    metric->dimension,
                    debugfs_rrd_algorithm_name(metric->algorithm));
        }
    }

    fprintf(stdout, "BEGIN system.zswap_%s\n", zswap_rejected_metrics[NETDATA_ZSWAP_REJECTED_CHART].chart_id);
    for (i = NETDATA_ZSWAP_REJECTED_COMPRESS_POOR; zswap_rejected_metrics[i].filename; i++) {
        metric = &zswap_rejected_metrics[i];
        fprintf(stdout,
                "SET %s = %lld\n",
                metric->dimension,
                metric->value);
    }
    fprintf(stdout, "END\n");
    fflush(stdout);
}

int debugfs_zswap(int update_every, const char *name) {
    int i;
    struct netdata_zswap_metric *metric;
    int independent_disabled = 0;
    for (i = 0; zswap_independent_metrics[i].filename; i++) {
        metric = &zswap_independent_metrics[i];
        if (likely(metric->enabled)) {
            metric->enabled = !zswap_collect_data(metric);
            if (unlikely(metric->obsolete == CONFIG_BOOLEAN_NO))
                zswap_independent_chart(metric, update_every, name);
            else if (likely(metric->chart_created && metric->obsolete)) {
                independent_disabled++;
                zswap_send_chart(metric, update_every, name, "obsolete");
            }
        }
    }

    // We set as 1, because the first structure with dimensions is not zero.
    int rejected_disabled = 1;
    for (i = NETDATA_ZSWAP_REJECTED_COMPRESS_POOR; zswap_rejected_metrics[i].filename; i++) {
        metric = &zswap_rejected_metrics[i];
        metric->enabled = !zswap_collect_data(metric);
        if (unlikely(!metric->enabled)) {
            rejected_disabled++;
        }
    }

    // When more than half of metrics are present we still plot chart.
    if (unlikely(rejected_disabled < NETDATA_ZSWAP_REJECTED_END))
        zswap_reject_chart(update_every, name);
    else {
        zswap_send_chart(&zswap_rejected_metrics[NETDATA_ZSWAP_REJECTED_CHART], update_every, name, "obsolete");
        // We do not have metrics, so we are disabling it
        if (independent_disabled == NETDATA_ZSWAP_SITE_END)
            return -1;
    }

    return 0;
}
