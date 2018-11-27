// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_MDSTAT_NAME "/proc/mdstat"

struct raid {
    int used;
    char *name;
    RRDDIM *rd_health;
    unsigned long long failed_disks;

    RRDSET *st_disks;
    RRDDIM *rd_total;
    RRDSET *rd_inuse;
    unsigned long long total_disks;
    unsigned long long inuse_disks;

    RRDSET *st_operation;
    RRDDIM *rd_resync;
    RRDDIM *rd_recovery;
    RRDDIM *rd_reshape;
    RRDDIM *rd_check;
    unsigned long long resync;
    unsigned long long recovery;
    unsigned long long reshape;
    unsigned long long check;

    RRDSET *st_finish;
    RRDDIM *rd_finish_in;
    unsigned long long finish_in;

    RRDSET *st_speed;
    RRDDIM *rd_speed;
    unsigned long long speed;

    // char *filename_mismatch_cnt;
    // int *fd_mismatch_cnt;
    RRDSET *st_mismatch_cnt;
    RRDDIM *rd_mismatch_cnt;
    unsigned long long mismatch_cnt;
};

int do_proc_mdstat(int update_every, usec_t dt) {
    (void)dt;
    static procfile *ff = NULL;
    static struct raid *raids = NULL;
    static size_t raids_num = 0, raids_allocated = 0;
    // static int do_mismatch_cnt = -1;

    // if(unlikely(do_mismatch_cnt == -1))
    //     do_mismatch_cnt = config_get_boolean_ondemand("plugin:proc:/proc/mdstat", "mismatch sectors", CONFIG_BOOLEAN_AUTO);

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/mdstat");
        ff = procfile_open(config_get("plugin:proc:/proc/mdstat", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff);
    size_t words = procfile_linewords(ff, 0);

    if(unlikely(!lines)) {
        error("Cannot read /proc/mdstat, zero lines reported.");
        return 1;
    }

    // find how many raids are there
    size_t l;
    raids_num = 0;
    for(l = 1; l < lines - 1 ; l++) {
        if(likely(strncmp(procfile_lineword(ff, l, 0), "md", 2) == 0)) // TODO: We can't rely on md* names so it has to be changed
            raids_num++;
    }

    if(unlikely(!raids_num)) {
        error("PLUGIN: PROC_MDSTAT: Cannot find the number of raids in /proc/mdstat");
        return 1;
    }

    // allocate the size we need;
    if(unlikely(raids_num != raids_allocated)) {
        size_t raid_idx;

        raids = (struct raid *)reallocz(raids, raids_num * sizeof(struct raid));

        // reset all interrupt RRDDIM pointers as any line could have shifted
        for(raid_idx = 0; raid_idx < raids_num; raid_idx++) {
            struct raid *raid = &raids[raid_idx];
            raid->rd_health = NULL;
            raid->name = NULL;
        }

        raids_allocated = raids_num;
    }
    raids[0].used = 0;

    // loop through all lines except the first and the last ones
    size_t raid_idx = 0;
    for(l = 1; l < (lines - 1) && raid_idx < raids_num; l++) {
        struct raid *raid = &raids[raid_idx];
        raid->used = 0;

        words = procfile_linewords(ff, l);
        if(unlikely(!words)) continue;

        raid->name = procfile_lineword(ff, l, 0);
        if(unlikely(!raid->name || !raid->name[0])) continue;

        // check if raid has disk status
        words = procfile_linewords(ff, l + 1);
        if(!words) continue;
        if(procfile_lineword(ff, l + 1, words - 1)[0] != '[') continue;

        // split inuse and total number of disks
        char *s = NULL, *str_inuse = NULL, *str_total = NULL;

        s = procfile_lineword(ff, l + 1, words - 2);
        str_inuse = s + 1;
        if(unlikely(s[0] != '[')) {
            error("Cannot read /proc/mdstat raid health status. Unexpected format: missing opening bracket.");
            continue;
        }
        while(*s != '\0') {
            if(*s == '/') {
                *s = '\0';
                str_total = s + 1;
            }
            if(*s == ']') {
                *s = '\0';
                break;
            }
            s++;
        }
        if(unlikely(str_inuse[0] == '\0' || str_total[0] == '\0')) {
            error("Cannot read /proc/mdstat raid health status. Unexpected format.");
            continue;
        }

        raid->inuse_disks = str2ull(str_inuse);
        raid->total_disks = str2ull(str_total);
        raid->failed_disks = raid->total_disks - raid->inuse_disks;

        raid_idx++;
        raid->used = 1;
    }

    // --------------------------------------------------------------------

    static RRDSET *st_mdstat_health = NULL;
    if(unlikely(!st_mdstat_health))
        st_mdstat_health = rrdset_create_localhost(
                "mdstat"
                , "new_mdstat_health" // TODO: rename chart
                , NULL
                , "health"
                , NULL
                , "Faulty Devices In MD"
                , "failed disks"
                , PLUGIN_PROC_NAME
                , PLUGIN_PROC_MODULE_MDSTAT_NAME
                , 6500 // TODO: define NETDATA_CHART_PRIO_MDSTAT_HEALTH
                , update_every
                , RRDSET_TYPE_LINE
        );
    else
        rrdset_next(st_mdstat_health);

    for(raid_idx = 0; raid_idx < raids_num; raid_idx++) {
        struct raid *raid = &raids[raid_idx];

        if(raid->used) {
            // if(unlikely(!raid->rd_health || strncmp(raid->name, raid->old_name, MAX_RAID_NAME) != 0)) {
                raid->rd_health = rrddim_add(st_mdstat_health, raid->name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                // rrddim_set_name(st_mdstat_health, raid->rd, raid->name);
            // }

            rrddim_set_by_pointer(st_mdstat_health, raid->rd_health, raid->failed_disks);
        }
    }

    rrdset_done(st_mdstat_health);

    return 0;
}
