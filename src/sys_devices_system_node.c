#include "common.h"

struct node {
    char *name;
    char *numastat_filename;
    procfile *numastat_ff;
    RRDSET *numastat_st;
    struct node *next;
};
static struct node *numa_root = NULL;

static int find_all_nodes() {
    int numa_node_count = 0;
    char name[FILENAME_MAX + 1];
    snprintfz(name, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/system/node");
    char *dirname = config_get("plugin:proc:/sys/devices/system/node", "directory to monitor", name);

    DIR *dir = opendir(dirname);
    if(!dir) {
        error("Cannot read NUMA node directory '%s'", dirname);
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

        m->numastat_filename = strdupz(name);

        m->next = numa_root;
        numa_root = m;
    }

    closedir(dir);

    return numa_node_count;
}

int do_proc_sys_devices_system_node(int update_every, usec_t dt) {
    (void)dt;

    static uint32_t hash_local_node = 0, hash_numa_foreign = 0, hash_interleave_hit = 0, hash_other_node = 0, hash_numa_hit = 0, hash_numa_miss = 0;
    static int do_numastat = -1, numa_node_count = 0;
    struct node *m;

    if(unlikely(numa_root == NULL)) {
        numa_node_count = find_all_nodes();
        if(unlikely(numa_root == NULL))
            return 1;
    }

    if(unlikely(do_numastat == -1)) {
        do_numastat = config_get_boolean_ondemand("plugin:proc:/sys/devices/system/node", "enable per-node numa metrics", CONFIG_BOOLEAN_AUTO);

        hash_local_node     = simple_hash("local_node");
        hash_numa_foreign   = simple_hash("numa_foreign");
        hash_interleave_hit = simple_hash("interleave_hit");
        hash_other_node     = simple_hash("other_node");
        hash_numa_hit       = simple_hash("numa_hit");
        hash_numa_miss      = simple_hash("numa_miss");
    }

    if(do_numastat == CONFIG_BOOLEAN_YES || (do_numastat == CONFIG_BOOLEAN_AUTO && numa_node_count >= 2)) {
        for(m = numa_root; m; m = m->next) {
            if(m->numastat_filename) {

                if(unlikely(!m->numastat_ff)) {
                    m->numastat_ff = procfile_open(m->numastat_filename, " ", PROCFILE_FLAG_DEFAULT);

                    if(unlikely(!m->numastat_ff))
                        continue;
                }

                m->numastat_ff = procfile_readall(m->numastat_ff);
                if(unlikely(!m->numastat_ff || procfile_lines(m->numastat_ff) < 1 || procfile_linewords(m->numastat_ff, 0) < 1))
                    continue;

                if(unlikely(!m->numastat_st)) {
                    m->numastat_st = rrdset_create_localhost(
                            "mem"
                            , m->name
                            , NULL
                            , "numa"
                            , NULL
                            , "NUMA events"
                            , "events/s"
                            , "proc"
                            , "/sys/devices/system/node"
                            , NETDATA_CHART_PRIO_MEM_NUMA + 10
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(m->numastat_st, RRDSET_FLAG_DETAIL);

                    rrddim_add(m->numastat_st, "numa_hit",       "hit",        1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(m->numastat_st, "numa_miss",      "miss",       1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(m->numastat_st, "local_node",     "local",      1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(m->numastat_st, "numa_foreign",   "foreign",    1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(m->numastat_st, "interleave_hit", "interleave", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(m->numastat_st, "other_node",     "other",      1, 1, RRD_ALGORITHM_INCREMENTAL);

                }
                else rrdset_next(m->numastat_st);

                size_t lines = procfile_lines(m->numastat_ff), l;
                for(l = 0; l < lines; l++) {
                    size_t words = procfile_linewords(m->numastat_ff, l);

                    if(unlikely(words < 2)) {
                        if(unlikely(words))
                            error("Cannot read %s numastat line %zu. Expected 2 params, read %zu.", m->name, l, words);
                        continue;
                    }

                    char *name  = procfile_lineword(m->numastat_ff, l, 0);
                    char *value = procfile_lineword(m->numastat_ff, l, 1);

                    if (unlikely(!name || !*name || !value || !*value))
                        continue;

                    uint32_t hash = simple_hash(name);
                    if(likely(
                               (hash == hash_numa_hit       && !strcmp(name, "numa_hit"))
                            || (hash == hash_numa_miss      && !strcmp(name, "numa_miss"))
                            || (hash == hash_local_node     && !strcmp(name, "local_node"))
                            || (hash == hash_numa_foreign   && !strcmp(name, "numa_foreign"))
                            || (hash == hash_interleave_hit && !strcmp(name, "interleave_hit"))
                            || (hash == hash_other_node     && !strcmp(name, "other_node"))
                    ))
                        rrddim_set(m->numastat_st, name, (collected_number)str2kernel_uint_t(value));
                }

                rrdset_done(m->numastat_st);
            }
        }
    }

    return 0;
}
