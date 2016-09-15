#include "common.h"

static inline char *softnet_column_name(uint32_t column) {
    static char buf[4] = "c00";
    char *s;

    switch(column) {
        case 0: s = "total"; break;
        case 1: s = "dropped"; break;
        case 2: s = "squeezed"; break;
        case 8: s = "collisions"; break;
        default: {
            uint32_t c = column + 1;
            buf[1] = '0' + ( c / 10);   c = c % 10;
            buf[2] = '0' + c;
            s = buf;
            break;
        }
    }

    return s;
}

int do_proc_net_softnet_stat(int update_every, unsigned long long dt) {
    (void)dt;

    static procfile *ff = NULL;
    static int do_per_core = -1;
    static uint32_t allocated_lines = 0, allocated_columns = 0, *data = NULL;

    if(do_per_core == -1) do_per_core = config_get_boolean("plugin:proc:/proc/net/softnet_stat", "softnet_stat per core", 1);

    if(!ff) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/net/softnet_stat");
        ff = procfile_open(config_get("plugin:proc:/proc/net/softnet_stat", "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
    }
    if(!ff) return 1;

    ff = procfile_readall(ff);
    if(!ff) return 0; // we return 0, so that we will retry to open it next time

    uint32_t lines = procfile_lines(ff), l;
    uint32_t words = procfile_linewords(ff, 0), w;

    if(!lines || !words) {
        error("Cannot read /proc/net/softnet_stat, %u lines and %u columns reported.", lines, words);
        return 1;
    }

    if(lines > 200) lines = 200;
    if(words > 50) words = 50;
    
    if(unlikely(!data || lines > allocated_lines || words > allocated_columns)) {
        freez(data);
        data = mallocz((lines + 1) * words * sizeof(uint32_t));
        allocated_lines = lines;
        allocated_columns = words;
    }
    
    // initialize to zero
    bzero(data, allocated_lines * allocated_columns * sizeof(uint32_t));

    // parse the values
    for(l = 0; l < lines ;l++) {
        words = procfile_linewords(ff, l);
        if(!words) continue;

        if(words > allocated_columns) words = allocated_columns;

        for(w = 0; w < words ; w++) {
            uint32_t t = strtoul(procfile_lineword(ff, l, w), NULL, 16);
            data[w] += t;
            data[((l + 1) * allocated_columns) + w] = t;
        }
    }

    if(data[(lines * allocated_columns)] == 0)
        lines--;

    RRDSET *st;

    // --------------------------------------------------------------------

    st = rrdset_find_bytype("system", "softnet_stat");
    if(!st) {
        st = rrdset_create("system", "softnet_stat", NULL, "softnet_stat", NULL, "System softnet_stat", "events/s", 955, update_every, RRDSET_TYPE_LINE);
        for(w = 0; w < allocated_columns ;w++)
            rrddim_add(st, softnet_column_name(w), NULL, 1, 1, RRDDIM_INCREMENTAL);
    }
    else rrdset_next(st);

    for(w = 0; w < allocated_columns ;w++)
        rrddim_set(st, softnet_column_name(w), data[w]);

    rrdset_done(st);

    if(do_per_core) {
        for(l = 0; l < lines ;l++) {
            char id[50+1];
            snprintfz(id, 50, "cpu%d_softnet_stat", l);

            st = rrdset_find_bytype("cpu", id);
            if(!st) {
                char title[100+1];
                snprintfz(title, 100, "CPU%d softnet_stat", l);

                st = rrdset_create("cpu", id, NULL, "softnet_stat", NULL, title, "events/s", 4101 + l, update_every, RRDSET_TYPE_LINE);
                for(w = 0; w < allocated_columns ;w++)
                    rrddim_add(st, softnet_column_name(w), NULL, 1, 1, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            for(w = 0; w < allocated_columns ;w++)
                rrddim_set(st, softnet_column_name(w), data[((l + 1) * allocated_columns) + w]);

            rrdset_done(st);
        }
    }

    return 0;
}
