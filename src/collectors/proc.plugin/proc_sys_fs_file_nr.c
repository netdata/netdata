// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

int do_proc_sys_fs_file_nr(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/sys/fs/file-nr");
        ff = procfile_open(inicfg_get(&netdata_config, "plugin:proc:/proc/sys/fs/file-nr", "filename to monitor", filename), "", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    uint64_t allocated = str2ull(procfile_lineword(ff, 0, 0), NULL);
    uint64_t unused = str2ull(procfile_lineword(ff, 0, 1), NULL);
    uint64_t max = str2ull(procfile_lineword(ff, 0, 2), NULL);

    uint64_t used = allocated - unused;

    static RRDSET *st_files = NULL;
    static RRDDIM *rd_used = NULL;

    if(unlikely(!st_files)) {
        st_files = rrdset_create_localhost(
                "system"
                , "file_nr_used"
                , NULL
                , "files"
                , NULL
                , "File Descriptors"
                , "files"
                , PLUGIN_PROC_NAME
                , "/proc/sys/fs/file-nr"
                , NETDATA_CHART_PRIO_SYSTEM_FILES_NR
                , update_every
                , RRDSET_TYPE_LINE
        );

        rd_used = rrddim_add(st_files, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_files, rd_used, (collected_number )used);
    rrdset_done(st_files);

    static RRDSET *st_files_utilization = NULL;
    static RRDDIM *rd_utilization = NULL;

    if(unlikely(!st_files_utilization)) {
        st_files_utilization = rrdset_create_localhost(
                "system"
                , "file_nr_utilization"
                , NULL
                , "files"
                , NULL
                , "File Descriptors Utilization"
                , "percentage"
                , PLUGIN_PROC_NAME
                , "/proc/sys/fs/file-nr"
                , NETDATA_CHART_PRIO_SYSTEM_FILES_NR + 1
                , update_every
                , RRDSET_TYPE_LINE
        );

        rd_utilization = rrddim_add(st_files_utilization, "utilization", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
    }

    NETDATA_DOUBLE d_used = (NETDATA_DOUBLE)used;
    NETDATA_DOUBLE d_max = (NETDATA_DOUBLE)max;
    NETDATA_DOUBLE percent = d_used * 100.0 / d_max;

    rrddim_set_by_pointer(st_files_utilization, rd_utilization, (collected_number)(percent * 10000));
    rrdset_done(st_files_utilization);

    return 0;
}
