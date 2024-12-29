// SPDX-License-Identifier: GPL-3.0-or-later

#include "debugfs_plugin.h"

#define NETDATA_ORDER_FRAGMENTATION 11

static char *orders[NETDATA_ORDER_FRAGMENTATION] = { "order0", "order1", "order2", "order3", "order4",
                                                    "order5", "order6", "order7", "order8", "order9",
                                                    "order10"
};

static struct netdata_extrafrag {
    char *node_zone;
    uint32_t hash;

    char *id;

    collected_number orders[NETDATA_ORDER_FRAGMENTATION];

    struct netdata_extrafrag *next;
} *netdata_extrafrags_root = NULL;

static struct netdata_extrafrag *find_or_create_extrafrag(const char *name)
{
    struct netdata_extrafrag *extrafrag;
    uint32_t hash = simple_hash(name);

    // search it, from beginning to the end
    for (extrafrag = netdata_extrafrags_root ; extrafrag ; extrafrag = extrafrag->next) {
        if (unlikely(hash == extrafrag->hash && !strcmp(name, extrafrag->node_zone))) {
            return extrafrag;
        }
    }

    extrafrag = callocz(1, sizeof(struct netdata_extrafrag));
    extrafrag->node_zone = strdupz(name);
    extrafrag->hash = hash;

    if (netdata_extrafrags_root) {
        struct netdata_extrafrag *last_node;
        for (last_node = netdata_extrafrags_root; last_node->next ; last_node = last_node->next);

        last_node->next = extrafrag;
    } else
        netdata_extrafrags_root = extrafrag;


    return extrafrag;
}

static void extfrag_send_chart(char *chart_id, collected_number *values)
{
    int i;
    printf(PLUGINSD_KEYWORD_BEGIN " mem.fragmentation_index_%s\n", chart_id);

    for (i = 0; i < NETDATA_ORDER_FRAGMENTATION; i++)
        printf(PLUGINSD_KEYWORD_SET " %s = %lld\n", orders[i], values[i]);

    printf(PLUGINSD_KEYWORD_END "\n");
}

int do_module_numa_extfrag(int update_every, const char *name) {
    static procfile *ff = NULL;;

    if (unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/kernel/debug/extfrag/extfrag_index");

        ff = procfile_open(filename, " \t,", PROCFILE_FLAG_DEFAULT);
        if (unlikely(!ff))
            return 1;
    }

    ff = procfile_readall(ff);
    if (unlikely(!ff))
        return 1;

    size_t l, i, j, lines = procfile_lines(ff);
    for (l = 0; l < lines; l++) {
        char chart_id[64];
        char zone_lowercase[32];
        if (unlikely(procfile_linewords(ff, l) < 15)) continue;
        char *zone = procfile_lineword(ff, l, 3);
        strncpyz(zone_lowercase, zone, 31);
        debugfs2lower(zone_lowercase);

        char *id = procfile_lineword(ff, l, 1);
        snprintfz(chart_id, 63, "node_%s_%s", id, zone_lowercase);
        debugfs2lower(chart_id);

        struct netdata_extrafrag *extrafrag = find_or_create_extrafrag(chart_id);
        collected_number *line_orders = extrafrag->orders;
        for (i = 4, j = 0 ; i < 15; i++, j++) {
            NETDATA_DOUBLE value = str2ndd(procfile_lineword(ff, l, i), NULL);
            line_orders[j] = (collected_number) (value * 1000.0);
        }

        netdata_mutex_lock(&stdout_mutex);

        if (unlikely(!extrafrag->id)) {
            extrafrag->id = extrafrag->node_zone;
                printf(
                PLUGINSD_KEYWORD_CHART " mem.fragmentation_index_%s '' 'Memory fragmentation index for each order' 'index' 'fragmentation' 'mem.numa_node_zone_fragmentation_index' 'line' %d %d '' 'debugfs.plugin' '%s'\n",
                extrafrag->node_zone,
                NETDATA_CHART_PRIO_MEM_FRAGMENTATION,
                update_every,
                name);

            for (i = 0; i < NETDATA_ORDER_FRAGMENTATION; i++)
                printf(PLUGINSD_KEYWORD_DIMENSION " '%s' '%s' absolute 1 1000 ''\n", orders[i], orders[i]);

            printf(
                PLUGINSD_KEYWORD_CLABEL " 'numa_node' 'node%s' 1\n"
                PLUGINSD_KEYWORD_CLABEL " 'zone' '%s' 1\n"
                PLUGINSD_KEYWORD_CLABEL_COMMIT "\n",
                id,
                zone);
        }
        extfrag_send_chart(chart_id, line_orders);

        fflush(stdout);
        netdata_mutex_unlock(&stdout_mutex);
    }

    return 0;
}
