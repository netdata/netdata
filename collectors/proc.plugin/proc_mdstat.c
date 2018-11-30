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
    RRDDIM *rd_inuse;
    unsigned long long total_disks;
    unsigned long long inuse_disks;

    RRDSET *st_operation;
    RRDDIM *rd_check;
    RRDDIM *rd_resync;
    RRDDIM *rd_recovery;
    RRDDIM *rd_reshape;
    calculated_number check;
    calculated_number resync;
    calculated_number recovery;
    calculated_number reshape;

    RRDSET *st_finish;
    RRDDIM *rd_finish_in;
    calculated_number finish_in;

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
    static char *mismatch_cnt_filename = NULL;
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

    // static int old_raids_num = 0;

    // if(old_raids_num < raid_num) {
    //     st_mdstat_health = reallocz(st_mdstat_health, sizeof(RRDSET *) * raid_num);
    //     memset(&st_mdstat_health[old_raids_num], 0, sizeof(RRDSET *) * (raid_num - old_raids_num));
    //     old_raids_num = raid_num;
    // }

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
        if(unlikely(words < 2)) continue;

        raid->name = procfile_lineword(ff, l, 0);
        if(unlikely(!raid->name || !raid->name[0])) continue;

        // check if raid has disk status
        words = procfile_linewords(ff, l + 1);
        if(words < 2) {
            l++;
            continue;
        }
        if(procfile_lineword(ff, l + 1, words - 1)[0] != '[') {
            l++;
            continue;
        }

        // split inuse and total number of disks
        char *s = NULL, *str_inuse = NULL, *str_total = NULL;

        s = procfile_lineword(ff, l + 1, words - 2);
        if(unlikely(s[0] != '[')) {
            error("Cannot read /proc/mdstat raid health status. Unexpected format: missing opening bracket.");
            continue;
        }
        str_inuse = ++s;
        while(*s) {
            if(*s == '/') {
                *s = '\0';
                str_total = s + 1;
            }
            else if(*s == ']') {
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

        // check if any operation is performed on the raid
        words = procfile_linewords(ff, l + 2);
        if(words < 2) continue;
        if (unlikely(words < 7)) {
            error("Cannot read mdstat line. Expected 7 params, read %zu.", words);
            continue;
        }
        if(procfile_lineword(ff, l + 2, 0)[0] != '[') continue;

        char *str_percentage = procfile_lineword(ff, l + 2, 3);

        // remove trailing '%'
        s = str_percentage;
        while(*s) {
            if(*s == '%') {
                *s = '\0';
            }
            s++;
        }

        raid->check = 0;
        raid->resync = 0;
        raid->recovery = 0;
        raid->reshape = 0;
        calculated_number percentage = str2ld(str_percentage, NULL) * 100;
        // possible operations: check, resync, recovery, reshape
        // only 4-th character is checked
        switch(procfile_lineword(ff, l + 2, 1)[3]) {
            case 'c': // check
                raid->check = percentage;
                break;
            case 'y': // resync
                raid->resync = percentage;
                break;
            case 'o': // recovery
                raid->recovery = percentage;
                break;
            case 'h': // reshape
                raid->reshape = percentage;
                break;
        }

        char *str_finish_in = procfile_lineword(ff, l + 2, 5);

        // remove trailing "min"
        s = str_finish_in;
        while(*s) {
            if(*s == 'm') {
                *s = '\0';
            }
            s++;
        }

        str_finish_in += 7; // skip "finish=" in the beginning

        if(s > str_finish_in)
            raid->finish_in = str2ld(str_finish_in, NULL) * 60;

        char *str_speed = procfile_lineword(ff, l + 2, 6);

        // remove trailing "K/sec"
        s = str_speed;
        while(*s) {
            if(*s == 'K') {
                *s = '\0';
            }
            s++;
        }

        str_speed += 6; // skip "speed=" in the beginning

        if(s > str_speed)
            raid->speed = str2ull(str_speed);
    }

    // read mismatch_cnt files

    if(!mismatch_cnt_filename) {
        char filename[FILENAME_MAX + 1];

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/block/%s/md/mismatch_cnt");
        mismatch_cnt_filename = config_get("plugin:proc:/proc/mdstat", "mismatch_cnt filename to monitor", filename);
    }

    for(raid_idx = 0; raid_idx < raids_num ; raid_idx++) {
        char filename[FILENAME_MAX + 1];
        struct raid *raid = &raids[raid_idx];

        snprintfz(filename, FILENAME_MAX, mismatch_cnt_filename, raid->name);
        if(read_single_number_file(filename, &raid->mismatch_cnt) == -1) {
            error("Cannot read file '%s'", filename);
            return 1;
        }
    }

    // --------------------------------------------------------------------

    static RRDSET *st_mdstat_health = NULL;
    if(unlikely(!st_mdstat_health))
        st_mdstat_health = rrdset_create_localhost(
                "mdstat"
                , "new_mdstat_health" // TODO: rename chart
                , NULL
                , "health"
                , "md.health"
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

    // --------------------------------------------------------------------

    for(raid_idx = 0; raid_idx < raids_num ; raid_idx++) {
        struct raid *raid = &raids[raid_idx];
        char id[50 + 1];
        char family[50 + 1];

        if(unlikely(!raid->st_disks)) {
            snprintfz(id, 50, "new_%s_disks", raid->name); // TODO: rename chart
            snprintfz(family, 50, "%s", raid->name);

            raid->st_disks = rrdset_create_localhost(
                    "mdstat"
                    , id
                    , NULL
                    , family
                    , "md.disks"
                    , "Disk Stats"
                    , "disks"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MDSTAT_NAME
                    , 6500 + raid_idx // TODO: define NETDATA_CHART_PRIO_MDSTAT_RAID
                    , update_every
                    , RRDSET_TYPE_STACKED
            );
        }
        else
            rrdset_next(raid->st_disks);

        if(raid->used) {
            raid->rd_inuse = rrddim_add(raid->st_disks, "inuse", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            raid->rd_total = rrddim_add(raid->st_disks, "total", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(raid->st_disks, raid->rd_inuse, raid->inuse_disks);
            rrddim_set_by_pointer(raid->st_disks, raid->rd_total, raid->total_disks);
        }

        rrdset_done(raid->st_disks);

        // --------------------------------------------------------------------

        if(unlikely(!raid->st_mismatch_cnt)) {
            snprintfz(id, 50, "new_%s_mismatch", raid->name); // TODO: rename chart
            snprintfz(family, 50, "%s", raid->name);

            raid->st_mismatch_cnt = rrdset_create_localhost(
                    "mdstat"
                    , id
                    , NULL
                    , family
                    , "md.mismatch_cnt"
                    , "Mismatch Count"
                    , "unsynchronized blocks"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MDSTAT_NAME
                    , 6500 + raid_idx + 1 // TODO: define NETDATA_CHART_PRIO_MDSTAT_RAID
                    , update_every
                    , RRDSET_TYPE_LINE
            );
        }
        else
            rrdset_next(raid->st_mismatch_cnt);

        if(raid->used) {
            raid->rd_mismatch_cnt = rrddim_add(raid->st_mismatch_cnt, "count", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(raid->st_mismatch_cnt, raid->rd_mismatch_cnt, raid->mismatch_cnt);
        }

        rrdset_done(raid->st_mismatch_cnt);

        // --------------------------------------------------------------------

        if(unlikely(!raid->st_operation)) {
            snprintfz(id, 50, "new_%s_operation", raid->name); // TODO: rename chart
            snprintfz(family, 50, "%s", raid->name);

            raid->st_operation = rrdset_create_localhost(
                    "mdstat"
                    , id
                    , NULL
                    , family
                    , "ms.status"
                    , "Current Status"
                    , "percent"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MDSTAT_NAME
                    , 6500 + raid_idx + 2 // TODO: define NETDATA_CHART_PRIO_MDSTAT_RAID
                    , update_every
                    , RRDSET_TYPE_LINE
            );
        }
        else
            rrdset_next(raid->st_operation);

        if(raid->used) {
            raid->rd_check    = rrddim_add(raid->st_operation, "check",    NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
            raid->rd_resync   = rrddim_add(raid->st_operation, "resync",   NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
            raid->rd_recovery = rrddim_add(raid->st_operation, "recovery", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
            raid->rd_reshape  = rrddim_add(raid->st_operation, "reshape",  NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(raid->st_operation, raid->rd_check, raid->check);
            rrddim_set_by_pointer(raid->st_operation, raid->rd_resync, raid->resync);
            rrddim_set_by_pointer(raid->st_operation, raid->rd_recovery, raid->recovery);
            rrddim_set_by_pointer(raid->st_operation, raid->rd_reshape, raid->reshape);
        }

        rrdset_done(raid->st_operation);

        // --------------------------------------------------------------------

        if(unlikely(!raid->st_finish)) {
            snprintfz(id, 50, "new_%s_finish", raid->name); // TODO: rename chart
            snprintfz(family, 50, "%s", raid->name);

            raid->st_finish = rrdset_create_localhost(
                    "mdstat"
                    , id
                    , NULL
                    , family
                    , "md.rate"
                    , "Approximate Time Unit Finish"
                    , "seconds"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MDSTAT_NAME
                    , 6500 + raid_idx + 3 // TODO: define NETDATA_CHART_PRIO_MDSTAT_RAID
                    , update_every
                    , RRDSET_TYPE_LINE
            );
        }
        else
            rrdset_next(raid->st_finish);

        if(raid->used) {
            raid->rd_finish_in = rrddim_add(raid->st_finish, "finish_in", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(raid->st_finish, raid->rd_finish_in, raid->finish_in);
        }

        rrdset_done(raid->st_finish);

        // --------------------------------------------------------------------

        if(unlikely(!raid->st_speed)) {
            snprintfz(id, 50, "new_%s_speed", raid->name); // TODO: rename chart
            snprintfz(family, 50, "%s", raid->name);

            raid->st_speed = rrdset_create_localhost(
                    "mdstat"
                    , id
                    , NULL
                    , family
                    , "md.rate"
                    , "Operation Speed"
                    , "KB/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MDSTAT_NAME
                    , 6500 + raid_idx + 4 // TODO: define NETDATA_CHART_PRIO_MDSTAT_RAID
                    , update_every
                    , RRDSET_TYPE_LINE
            );
        }
        else
            rrdset_next(raid->st_speed);

        if(raid->used) {
            raid->rd_speed = rrddim_add(raid->st_speed, "speed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(raid->st_speed, raid->rd_speed, raid->speed);
        }

        rrdset_done(raid->st_speed);
    }

    return 0;
}
