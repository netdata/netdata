// SPDX-License-Identifier: GPL-3.0-or-later

#include "debugfs_plugin.h"

struct netdata_zswap_metric {
    const char *filename;

    const char *chart_id;
    const char *dimension;
    const char *units;
    const char *title;
    const char *algorithm; // TODO: use enum RRD_ALGORITHM

    int enabled;
    int obsolete;
    int chart_created;

    int prio;

    collected_number value;
};

static struct netdata_zswap_metric zswap_independent_metrics[] = {
    // https://elixir.bootlin.com/linux/latest/source/mm/zswap.c
    {.filename = "/sys/kernel/debug/zswap/same_filled_pages",
     .chart_id = "same_filled_page",
     .dimension = "same_filled",
     .units = "pages",
     .title = "Zswap same-value filled pages currently stored",
     .algorithm = "absolulte",
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
     .algorithm = "absolulte",
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
     .algorithm = "absolulte",
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
     .algorithm = "incremental",
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
     .algorithm = "incremental",
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
     .algorithm = "incremental",
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_POOL_LIM_HIT,
     .value = -1},

    // The terminator
    {.filename = NULL, .chart_id = NULL, .enabled = CONFIG_BOOLEAN_NO}
};

enum netdata_zswap_rejected {
    NETDATA_ZSWAP_REJECTED_COMPRESS_POOR,
    NETDATA_ZSWAP_REJECTED_KMEM_FAIL,
    NETDATA_ZSWAP_REJECTED_RALLOC_FAIL,
    NETDATA_ZSWAP_REJECTED_RRECLAIM_FAIL
};


static struct netdata_zswap_metric zswap_rejected_metrics[] = {
    {.filename = "/sys/kernel/debug/zswap/reject_compress_poor",
     .chart_id = "reject_compress_poor",
     .dimension = "compress_poor",
     .units = NULL,
     .title = NULL,
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
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_REJECTS},
    {.filename = "/sys/kernel/debug/zswap/reject_reclaim_fail",
     .chart_id = "reject_reclaim_fail",
     .dimension = "reclaim_fail",
     .units = NULL,
     .title = NULL,
     .enabled = CONFIG_BOOLEAN_YES,
     .obsolete = CONFIG_BOOLEAN_NO,
     .chart_created = CONFIG_BOOLEAN_NO,
     .prio = NETDATA_CHART_PRIO_SYSTEM_ZSWAP_REJECTS,
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
    if (fd < 0 ) {
        metric->obsolete = CONFIG_BOOLEAN_YES;
        error("Cannot open file %s", filename);
        return -1;
    }

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

static void zswap_obsolete_independent_chart(struct netdata_zswap_metric *metric, int update_every, const char *name)
{
    fprintf(
        stdout,
        "CHART system.zswap_%s '' '%s' '%s' 'zswap' '' 'line' %d %d 'obsolete' 'debugfs.plugin' '%s'\n",
        metric->chart_id,
        metric->title,
        metric->units,
        metric->prio,
        update_every,
        name);
}

static void zswap_independent_chart(struct netdata_zswap_metric *metric, int update_every, const char *name)
{
    if (unlikely(!metric->chart_created)) {
        metric->chart_created = CONFIG_BOOLEAN_YES;
        fprintf(
            stdout,
            "CHART system.zswap_%s '' '%s' '%s' 'zswap' '' 'line' %d %d '' 'debugfs.plugin' '%s'\n",
            metric->chart_id,
            metric->title,
            metric->units,
            metric->prio,
            update_every,
            name);
        fprintf(
            stdout,
            "DIMENSION '%s' '%s' %s 1 1 ''\n",
            metric->dimension,
            metric->dimension,
            metric->algorithm);
    }

    fprintf(stdout, "BEGIN system.zswap_%s\n"
                    "SET %s = %lld\n"
                    "END\n",
            metric->chart_id, metric->dimension, metric->value);
    fflush(stdout);
}

void zswap_reject_chart(int update_every, const char *name) {
    int i;
    struct netdata_zswap_metric *metric = &zswap_rejected_metrics[NETDATA_ZSWAP_REJECTED_COMPRESS_POOR];
    if (unlikely(!metric->chart_created)) {
        metric->chart_created = CONFIG_BOOLEAN_YES;
        fprintf(stdout,
                "CHART system.zswap_rejections '' 'Zswap rejections' 'rejections/s' 'zswap' 'system.zswap_rejections' 'stacked' %d %d '' 'debugfs.plugin' '%s'\n",
                metric->prio, update_every, name);
        for (i = 0; zswap_rejected_metrics[i].filename; i++) {
            metric = &zswap_rejected_metrics[i];
            fprintf(stdout,
                    "DIMENSION '%s' '%s' incremental 1 1 ''\n",
                    metric->dimension,
                    metric->dimension);
        }
    }

    fprintf(stdout, "BEGIN system.zswap_rejections\n");
    for (i = 0; zswap_rejected_metrics[i].filename; i++) {
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
    // TODO: stop collector if zswap is not enabled

    int i;
    struct netdata_zswap_metric *metric;
    for (i = 0; zswap_independent_metrics[i].filename; i++) {
        metric = &zswap_independent_metrics[i];
        if (likely(metric->enabled)) {
            metric->enabled = !zswap_collect_data(metric);
            if (unlikely(metric->obsolete == CONFIG_BOOLEAN_NO))
                zswap_independent_chart(metric, update_every, name);
            else if (likely(metric->chart_id && metric->obsolete))
                zswap_obsolete_independent_chart(metric, update_every, name);
        }
    }

    for (i = 0; zswap_rejected_metrics[i].filename; i++) {
        metric = &zswap_rejected_metrics[i];
        metric->enabled = !zswap_collect_data(metric);
    }

    zswap_reject_chart(update_every, name);

    return 0;
}
