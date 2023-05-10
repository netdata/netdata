// SPDX-License-Identifier: GPL-3.0-or-later

#include "debugfs_plugin.h"

struct netdata_zswap_metric {
    const char *filename;

    const char *chart_id;
    const char *dimension;
    const char *units;
    const char *title;

    int enabled;
    int chart_created;

    int prio;

    collected_number value;
};

static struct netdata_zswap_metric zswap_independent_metrics[] = {
    // https://elixir.bootlin.com/linux/latest/source/mm/zswap.c
    {.filename = "/sys/kernel/debug/zswap/same_filled_pages",
     .chart_id = "same_filled_page",
     .dimension = "pages",
     .units = "pages",
     .title = "Total same-value filled pages currently stored",
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_SAME_FILL_PAGE,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/stored_pages",
     .chart_id = "stored_pages",
     .dimension = "page",
     .units = "pages",
     .title = "Compressed pages stored in zswap.",
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_STORED_PAGE,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/pool_total_size",
     .chart_id = "pool_total_size",
     .dimension = "pool",
     .units = "bytes",
     .title = "Total bytes used by the compressed storage",
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_POOL_TOT_SIZE,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/duplicate_entry",
     .chart_id = "duplicate_entry",
     .dimension = "duplicate",
     .units = "page",
     .title = "Duplicate store was found.",
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_DUPP_ENTRY,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/written_back_pages",
     .chart_id = "written_back_pages",
     .dimension = "pages",
     .units = "pages",
     .title = "Pages written back when pool limit was reached.",
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_STORED_PAGE,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/pool_limit_hit",
     .chart_id = "pool_limit_hit",
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .dimension = "limit",
     .units = "boolean",
     .title = "Was the pool limit reached?",
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_POOL_LIM_HIT,
     .value = -1},

    // The terminator
    {.filename = NULL, .chart_id = NULL, .enabled = CONFIG_BOOLEAN_NO}
};

static struct netdata_zswap_metric zswap_associated_metrics[] = {
    {.filename = "/sys/kernel/debug/zswap/reject_compress_poor",
     .chart_id = "reject_compress_poor",
     .dimension = "pages",
     .units = "pages",
     .title = "Compressed page was too big for the allocator to store.",
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_REJECT_COM_POOR,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/reject_kmemcache_fail",
     .chart_id = "reject_kmemcache_fail",
     .dimension = "pages",
     .units = "pages",
     .title = "Number of entry metadata that could not be allocated",
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_KMEM_FAIL,
     .value = -1},
    {.filename = "/sys/kernel/debug/zswap/reject_alloc_fail",
     .chart_id = "reject_alloc_fail",
     .dimension = "allocator",
     .units = "calls",
     .title = "Allocator could not get memory",
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_RALLOC_FAIL},
    {.filename = "/sys/kernel/debug/zswap/reject_reclaim_fail",
     .chart_id = "reject_reclaim_fail",
     .enabled = CONFIG_BOOLEAN_YES,
     .chart_created = CONFIG_BOOLEAN_NO,
     .dimension = "pages",
     .units = "pages",
     .title = "Memory cannot be reclaimed (pool limit was reached).",
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_RRECLAIM_FAIL,
     .value = -1},

    // The terminator
    {.filename = NULL, .chart_id = NULL, .enabled = CONFIG_BOOLEAN_NO}
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
    if (fd < 0 )
        return -1;

    ssize_t r = read(fd, buffer, 511);
    if (r < 0) {
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

void zswap_independent_chart(struct netdata_zswap_metric *metric, int update_every) {
    if (unlikely(!metric->chart_created)) {
        metric->chart_created = CONFIG_BOOLEAN_YES;
        fprintf(stdout,
                "CHART system.%s '' '%s' '%s' 'zswap' '' 'line' %d %d '' 'debugfs.plugin' '%s'\n",
                metric->chart_id, metric->title, metric->units, metric->prio, update_every, metric->filename);
        fprintf(stdout, "DIMENSION '%s' '%s' absolute 1 1 ''\n", metric->dimension, metric->dimension);
    }

    fprintf(stdout, "BEGIN system.%s\n"
                    "SET %s = %lld\n"
                    "END\n",
            metric->chart_id, metric->dimension, metric->value);
    fflush(stdout);
}

int debugfs_zswap(int update_every, const char *name) {
    (void)name;

    int i;
    struct netdata_zswap_metric *metric;
    for (i = 0; zswap_independent_metrics[i].filename; i++) {
        metric = &zswap_independent_metrics[i];
        metric->enabled = !zswap_collect_data(metric);
        if (metric->enabled)
            zswap_independent_chart(metric, update_every);
    }

    for (i = 0; zswap_associated_metrics[i].filename; i++) {
        metric = &zswap_associated_metrics[i];
        metric->enabled = !zswap_collect_data(metric);
    }

    return 0;
}
