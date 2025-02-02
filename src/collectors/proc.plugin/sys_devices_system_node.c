// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

struct node {
    char *name;

    struct {
        char *filename;
        procfile *ff;
        RRDSET *st;
    } numastat;

    struct {
        char *filename;
        procfile *ff;
        RRDSET *st_mem_usage;
        RRDSET *st_mem_activity;
    } meminfo;

    struct node *next;
};
static struct node *numa_root = NULL;
static int numa_node_count = 0;

static int find_all_nodes() {
    int numa_node_count = 0;
    char name[FILENAME_MAX + 1];
    snprintfz(name, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/system/node");
    const char *dirname = inicfg_get(&netdata_config, "plugin:proc:/sys/devices/system/node", "directory to monitor", name);

    DIR *dir = opendir(dirname);
    if(!dir) {
        nd_log(
            NDLS_COLLECTORS, errno == ENOENT ? NDLP_INFO : NDLP_ERR, "Cannot read NUMA node directory '%s'", dirname);
        return 0;
    }

    struct dirent *de = NULL;
    while((de = readdir(dir))) {
        if(de->d_type != DT_DIR)
            continue;

        if(strncmp(de->d_name, "node", 4) != 0)
            continue;

        if(!isdigit(de->d_name[4]))
            continue;

        numa_node_count++;

        struct node *m = callocz(1, sizeof(struct node));
        m->name = strdupz(de->d_name);

        struct stat st;

        snprintfz(name, FILENAME_MAX, "%s/%s/numastat", dirname, de->d_name);
        if(stat(name, &st) == -1) {
            freez(m->name);
            freez(m);
            continue;
        }
        m->numastat.filename = strdupz(name);

        snprintfz(name, FILENAME_MAX, "%s/%s/meminfo", dirname, de->d_name);
        if(stat(name, &st) == -1) {
            freez(m->numastat.filename);
            freez(m->name);
            freez(m);
            continue;
        }
        m->meminfo.filename = strdupz(name);

        m->next = numa_root;
        numa_root = m;
    }

    closedir(dir);

    return numa_node_count;
}

static void do_muma_numastat(struct node *m, int update_every) {
    static uint32_t hash_local_node = 0, hash_numa_foreign = 0, hash_interleave_hit = 0, hash_other_node = 0, hash_numa_hit = 0, hash_numa_miss = 0;
    static bool initialized = false;

    if(unlikely(!initialized)) {
        initialized = true;
        hash_local_node     = simple_hash("local_node");
        hash_numa_foreign   = simple_hash("numa_foreign");
        hash_interleave_hit = simple_hash("interleave_hit");
        hash_other_node     = simple_hash("other_node");
        hash_numa_hit       = simple_hash("numa_hit");
        hash_numa_miss      = simple_hash("numa_miss");
    }

    if (m->numastat.filename) {
        if(unlikely(!m->numastat.ff)) {
            m->numastat.ff = procfile_open(m->numastat.filename, " ", PROCFILE_FLAG_DEFAULT);

            if(unlikely(!m->numastat.ff))
                return;
        }

        m->numastat.ff = procfile_readall(m->numastat.ff);
        if(unlikely(!m->numastat.ff || procfile_lines(m->numastat.ff) < 1 || procfile_linewords(m->numastat.ff, 0) < 1))
            return;

        if(unlikely(!m->numastat.st)) {
            m->numastat.st = rrdset_create_localhost(
                "numa_node_stat"
                , m->name
                , NULL
                , "numa"
                , "mem.numa_node_stat"
                , "NUMA Node Memory Allocation Events"
                , "events/s"
                , PLUGIN_PROC_NAME
                , "/sys/devices/system/node"
                , NETDATA_CHART_PRIO_MEM_NUMA_NODES_NUMASTAT
                , update_every
                , RRDSET_TYPE_LINE
            );

            rrdlabels_add(m->numastat.st->rrdlabels, "numa_node", m->name, RRDLABEL_SRC_AUTO);

            rrddim_add(m->numastat.st, "numa_hit",       "hit",        1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(m->numastat.st, "numa_miss",      "miss",       1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(m->numastat.st, "local_node",     "local",      1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(m->numastat.st, "numa_foreign",   "foreign",    1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(m->numastat.st, "interleave_hit", "interleave", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(m->numastat.st, "other_node",     "other",      1, 1, RRD_ALGORITHM_INCREMENTAL);

        }

        size_t lines = procfile_lines(m->numastat.ff), l;
        for(l = 0; l < lines; l++) {
            size_t words = procfile_linewords(m->numastat.ff, l);

            if(unlikely(words < 2)) {
                if(unlikely(words))
                    collector_error("Cannot read %s line %zu. Expected 2 params, read %zu.", m->numastat.filename, l, words);
                continue;
            }

            char *name = procfile_lineword(m->numastat.ff, l, 0);
            char *value = procfile_lineword(m->numastat.ff, l, 1);

            if (unlikely(!name || !*name || !value || !*value))
                continue;

            uint32_t hash = simple_hash(name);

            if ((hash == hash_numa_hit && !strcmp(name, "numa_hit")) ||
                (hash == hash_numa_miss && !strcmp(name, "numa_miss")) ||
                (hash == hash_local_node && !strcmp(name, "local_node")) ||
                (hash == hash_numa_foreign && !strcmp(name, "numa_foreign")) ||
                (hash == hash_interleave_hit && !strcmp(name, "interleave_hit")) ||
                (hash == hash_other_node && !strcmp(name, "other_node"))) {
                rrddim_set(m->numastat.st, name, (collected_number)str2kernel_uint_t(value));
            }
        }

        rrdset_done(m->numastat.st);
    }
}

static void do_numa_meminfo(struct node *m, int update_every) {
    static uint32_t hash_MemFree = 0, hash_MemUsed = 0, hash_ActiveAnon = 0, hash_InactiveAnon = 0, hash_ActiveFile = 0,
                    hash_InactiveFile = 0;
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialized = true;
        hash_MemFree = simple_hash("MemFree");
        hash_MemUsed = simple_hash("MemUsed");
        hash_ActiveAnon = simple_hash("Active(anon)");
        hash_InactiveAnon = simple_hash("Inactive(anon)");
        hash_ActiveFile = simple_hash("Active(file)");
        hash_InactiveFile = simple_hash("Inactive(file)");
    }

    if (m->meminfo.filename) {
        if (unlikely(!m->meminfo.ff)) {
            m->meminfo.ff = procfile_open(m->meminfo.filename, " :", PROCFILE_FLAG_DEFAULT);
            if (unlikely(!m->meminfo.ff))
                return;
        }

        m->meminfo.ff = procfile_readall(m->meminfo.ff);
        if (unlikely(!m->meminfo.ff || procfile_lines(m->meminfo.ff) < 1 || procfile_linewords(m->meminfo.ff, 0) < 1))
            return;

        if (unlikely(!m->meminfo.st_mem_usage)) {
            m->meminfo.st_mem_usage = rrdset_create_localhost(
                "numa_node_mem_usage",
                m->name,
                NULL,
                "numa",
                "mem.numa_node_mem_usage",
                "NUMA Node Memory Usage",
                "bytes",
                PLUGIN_PROC_NAME,
                "/sys/devices/system/node",
                NETDATA_CHART_PRIO_MEM_NUMA_NODES_MEMINFO,
                update_every,
                RRDSET_TYPE_STACKED);

            rrdlabels_add(m->meminfo.st_mem_usage->rrdlabels, "numa_node", m->name, RRDLABEL_SRC_AUTO);

            rrddim_add(m->meminfo.st_mem_usage, "MemFree", "free", 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(m->meminfo.st_mem_usage, "MemUsed", "used", 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        if (unlikely(!m->meminfo.st_mem_activity)) {
            m->meminfo.st_mem_activity = rrdset_create_localhost(
                "numa_node_mem_activity",
                m->name,
                NULL,
                "numa",
                "mem.numa_node_mem_activity",
                "NUMA Node Memory Activity",
                "bytes",
                PLUGIN_PROC_NAME,
                "/sys/devices/system/node",
                NETDATA_CHART_PRIO_MEM_NUMA_NODES_ACTIVITY,
                update_every,
                RRDSET_TYPE_STACKED);

            rrdlabels_add(m->meminfo.st_mem_activity->rrdlabels, "numa_node", m->name, RRDLABEL_SRC_AUTO);

            rrddim_add(m->meminfo.st_mem_activity, "Active(anon)", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(m->meminfo.st_mem_activity, "Inactive(anon)", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(m->meminfo.st_mem_activity, "Active(file)", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(m->meminfo.st_mem_activity, "Inactive(file)", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        size_t lines = procfile_lines(m->meminfo.ff), l;
        for (l = 0; l < lines; l++) {
            size_t words = procfile_linewords(m->meminfo.ff, l);

            if (unlikely(words < 4)) {
                if (words)
                    collector_error(
                        "Cannot read %s line %zu. Expected 4 params, read %zu.", m->meminfo.filename, l, words);
                continue;
            }

            char *name = procfile_lineword(m->meminfo.ff, l, 2);
            char *value = procfile_lineword(m->meminfo.ff, l, 3);

            if (unlikely(!name || !*name || !value || !*value))
                continue;

            uint32_t hash = simple_hash(name);

            if ((hash == hash_MemFree && !strcmp(name, "MemFree")) ||
                (hash == hash_MemUsed && !strcmp(name, "MemUsed"))) {
                rrddim_set(m->meminfo.st_mem_usage, name, (collected_number)str2kernel_uint_t(value) * 1024);
            } else if (
                (hash == hash_ActiveAnon && !strcmp(name, "Active(anon)")) ||
                (hash == hash_InactiveAnon && !strcmp(name, "Inactive(anon)")) ||
                (hash == hash_ActiveFile && !strcmp(name, "Active(file)")) ||
                (hash == hash_InactiveFile && !strcmp(name, "Inactive(file)"))) {
                rrddim_set(m->meminfo.st_mem_activity, name, (collected_number)str2kernel_uint_t(value) * 1024);
            }
        }
        rrdset_done(m->meminfo.st_mem_usage);
        rrdset_done(m->meminfo.st_mem_activity);
    }
}

int do_proc_sys_devices_system_node(int update_every, usec_t dt) {
    (void)dt;
    struct node *m;

    static int do_numastat = -1;

    if(unlikely(do_numastat == -1)) {
        do_numastat = inicfg_get_boolean_ondemand(
            &netdata_config, "plugin:proc:/sys/devices/system/node", "enable per-node numa metrics", CONFIG_BOOLEAN_AUTO);
    }

    if(unlikely(numa_root == NULL)) {
        numa_node_count = find_all_nodes();
        if(unlikely(numa_root == NULL))
            return 1;
    }

    if (do_numastat == CONFIG_BOOLEAN_YES || (do_numastat == CONFIG_BOOLEAN_AUTO && numa_node_count >= 2)) {
        for (m = numa_root; m; m = m->next) {
            do_muma_numastat(m, update_every);
            do_numa_meminfo(m, update_every);
        }
        return 0;
    }

    return 1;
}
