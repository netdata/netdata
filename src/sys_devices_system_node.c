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
    snprintfz(name, FILENAME_MAX, "%s%s", global_host_prefix, "/sys/devices/system/node");
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

    static int numa_node_count = 0;

    if(unlikely(numa_root == NULL)) {
        numa_node_count = find_all_nodes(update_every);
        if(unlikely(numa_root == NULL))
            return 1;
    }

    static int do_numastat = -1;
    struct node *m;

    if(unlikely(do_numastat == -1)) {
        do_numastat = config_get_boolean_ondemand("plugin:proc:/sys/devices/system/node", "enable per-node numa metrics", CONFIG_ONDEMAND_ONDEMAND);
    }

    if(do_numastat == CONFIG_ONDEMAND_YES || (do_numastat == CONFIG_ONDEMAND_ONDEMAND && numa_node_count >= 2)) {
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

                procfile *ff = m->numastat_ff;

                RRDSET *st = m->numastat_st;
                if(unlikely(!st)) {
                    st = rrdset_create("mem", m->name, NULL, "numa", NULL, "NUMA events", "events/s", 1000, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "local_node", "local", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "numa_foreign", "foreign", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "interleave_hit", "interleave", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "other_node", "other", 1, 1, RRDDIM_INCREMENTAL);

                    m->numastat_st = st;
                }
                else rrdset_next(st);

                uint32_t lines = procfile_lines(ff), l;
                for(l = 0; l < lines; l++) {
                    uint32_t words = procfile_linewords(ff, l);
                    if(unlikely(words < 2)) {
                        if(unlikely(words)) error("Cannot read %s numastat line %u. Expected 2 params, read %u.", m->name, l, words);
                        continue;
                    }

                    char *name = procfile_lineword(ff, l, 0);
                    char *value = procfile_lineword(ff, l, 1);
                    if (unlikely(!name || !*name || !value || !*value)) continue;

                    rrddim_set(st, name, strtoull(value, NULL, 10));
                }
                rrdset_done(st);
            }
        }
    }

    return 0;
}
