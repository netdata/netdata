// SPDX-License-Identifier: GPL-3.0+
#include "common.h"

static inline char *softnet_column_name(size_t column) {
    switch(column) {
        // https://github.com/torvalds/linux/blob/a7fd20d1c476af4563e66865213474a2f9f473a4/net/core/net-procfs.c#L161-L166
        case 0: return "processed";
        case 1: return "dropped";
        case 2: return "squeezed";
        case 9: return "received_rps";
        case 10: return "flow_limit_count";
        default: return NULL;
    }
}

int do_proc_net_softnet_stat(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;
    static int do_per_core = -1;
    static size_t allocated_lines = 0, allocated_columns = 0;
    static uint32_t *data = NULL;

    if(unlikely(do_per_core == -1)) do_per_core = config_get_boolean("plugin:proc:/proc/net/softnet_stat", "softnet_stat per core", 1);

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/softnet_stat");
        ff = procfile_open(config_get("plugin:proc:/proc/net/softnet_stat", "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;
    size_t words = procfile_linewords(ff, 0), w;

    if(unlikely(!lines || !words)) {
        error("Cannot read /proc/net/softnet_stat, %zu lines and %zu columns reported.", lines, words);
        return 1;
    }

    if(unlikely(lines > 200)) lines = 200;
    if(unlikely(words > 50)) words = 50;

    if(unlikely(!data || lines > allocated_lines || words > allocated_columns)) {
        freez(data);
        allocated_lines = lines;
        allocated_columns = words;
        data = mallocz((allocated_lines + 1) * allocated_columns * sizeof(uint32_t));
    }

    // initialize to zero
    memset(data, 0, (allocated_lines + 1) * allocated_columns * sizeof(uint32_t));

    // parse the values
    for(l = 0; l < lines ;l++) {
        words = procfile_linewords(ff, l);
        if(unlikely(!words)) continue;

        if(unlikely(words > allocated_columns))
            words = allocated_columns;

        for(w = 0; w < words ; w++) {
            if(unlikely(softnet_column_name(w))) {
                uint32_t t = (uint32_t)strtoul(procfile_lineword(ff, l, w), NULL, 16);
                data[w] += t;
                data[((l + 1) * allocated_columns) + w] = t;
            }
        }
    }

    if(unlikely(data[(lines * allocated_columns)] == 0))
        lines--;

    RRDSET *st;

    // --------------------------------------------------------------------

    st = rrdset_find_bytype_localhost("system", "softnet_stat");
    if(unlikely(!st)) {
        st = rrdset_create_localhost(
                "system"
                , "softnet_stat"
                , NULL
                , "softnet_stat"
                , "system.softnet_stat"
                , "System softnet_stat"
                , "events/s"
                , "proc"
                , "net/softnet_stat"
                , 955
                , update_every
                , RRDSET_TYPE_LINE
        );
        for(w = 0; w < allocated_columns ;w++)
            if(unlikely(softnet_column_name(w)))
                rrddim_add(st, softnet_column_name(w), NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }
    else rrdset_next(st);

    for(w = 0; w < allocated_columns ;w++)
        if(unlikely(softnet_column_name(w)))
            rrddim_set(st, softnet_column_name(w), data[w]);

    rrdset_done(st);

    if(do_per_core) {
        for(l = 0; l < lines ;l++) {
            char id[50+1];
            snprintfz(id, 50, "cpu%zu_softnet_stat", l);

            st = rrdset_find_bytype_localhost("cpu", id);
            if(unlikely(!st)) {
                char title[100+1];
                snprintfz(title, 100, "CPU%zu softnet_stat", l);

                st = rrdset_create_localhost(
                        "cpu"
                        , id
                        , NULL
                        , "softnet_stat"
                        , "cpu.softnet_stat"
                        , title
                        , "events/s"
                        , "proc"
                        , "net/softnet_stat"
                        , 4101 + l
                        , update_every
                        , RRDSET_TYPE_LINE
                );
                for(w = 0; w < allocated_columns ;w++)
                    if(unlikely(softnet_column_name(w)))
                        rrddim_add(st, softnet_column_name(w), NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(st);

            for(w = 0; w < allocated_columns ;w++)
                if(unlikely(softnet_column_name(w)))
                    rrddim_set(st, softnet_column_name(w), data[((l + 1) * allocated_columns) + w]);

            rrdset_done(st);
        }
    }

    return 0;
}
