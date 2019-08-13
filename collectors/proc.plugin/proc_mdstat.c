// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_MDSTAT_NAME "/proc/mdstat"

struct raid {
    int redundant;
    char *name;
    uint32_t hash;

    RRDDIM *rd_health;
    unsigned long long failed_disks;

    RRDSET *st_disks;
    RRDDIM *rd_down;
    RRDDIM *rd_inuse;
    unsigned long long total_disks;
    unsigned long long inuse_disks;

    RRDSET *st_operation;
    RRDDIM *rd_check;
    RRDDIM *rd_resync;
    RRDDIM *rd_recovery;
    RRDDIM *rd_reshape;
    unsigned long long check;
    unsigned long long resync;
    unsigned long long recovery;
    unsigned long long reshape;

    RRDSET *st_finish;
    RRDDIM *rd_finish_in;
    unsigned long long finish_in;

    RRDSET *st_speed;
    RRDDIM *rd_speed;
    unsigned long long speed;

    char *mismatch_cnt_filename;
    RRDSET *st_mismatch_cnt;
    RRDDIM *rd_mismatch_cnt;
    unsigned long long mismatch_cnt;

    RRDSET *st_nonredundant;
    RRDDIM *rd_nonredundant;
};

struct old_raid {
    int redundant;
    char *name;
    uint32_t hash;
    int found;
};

static inline char *remove_trailing_chars(char *s, char c) {
    while(*s) {
        if(unlikely(*s == c)) {
            *s = '\0';
        }
        s++;
    }
    return s;
}

static inline void make_chart_obsolete(char *name, const char *id_modifier) {
    char id[50 + 1];
    RRDSET *st = NULL;

    if(likely(name && id_modifier)) {
        snprintfz(id, 50, "mdstat.%s_%s", name, id_modifier);
        st = rrdset_find_byname_localhost(id);
        if(likely(st)) rrdset_is_obsolete(st);
    }
}

int do_proc_mdstat(int update_every, usec_t dt) {
    (void)dt;
    static procfile *ff = NULL;
    static int do_health = -1, do_nonredundant = -1, do_disks = -1, do_operations = -1, do_mismatch = -1, do_mismatch_config = -1;
    static int make_charts_obsolete = -1;
    static char *mdstat_filename = NULL, *mismatch_cnt_filename = NULL;
    static struct raid *raids = NULL;
    static size_t raids_allocated = 0;
    size_t raids_num = 0, raid_idx = 0, redundant_num = 0;
    static struct old_raid *old_raids = NULL;
    static size_t old_raids_allocated = 0;
    size_t old_raid_idx = 0;

    if(unlikely(do_health == -1)){
        do_health           = config_get_boolean("plugin:proc:/proc/mdstat", "faulty devices", CONFIG_BOOLEAN_YES);
        do_nonredundant     = config_get_boolean("plugin:proc:/proc/mdstat", "nonredundant arrays availability", CONFIG_BOOLEAN_YES);
        do_mismatch_config  = config_get_boolean_ondemand("plugin:proc:/proc/mdstat", "mismatch count", CONFIG_BOOLEAN_AUTO);
        do_disks            = config_get_boolean("plugin:proc:/proc/mdstat", "disk stats", CONFIG_BOOLEAN_YES);
        do_operations       = config_get_boolean("plugin:proc:/proc/mdstat", "operation status", CONFIG_BOOLEAN_YES);

        make_charts_obsolete = config_get_boolean("plugin:proc:/proc/mdstat", "make charts obsolete", CONFIG_BOOLEAN_YES);

        char filename[FILENAME_MAX + 1];

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/mdstat");
        mdstat_filename = config_get("plugin:proc:/proc/mdstat", "filename to monitor", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/block/%s/md/mismatch_cnt");
        mismatch_cnt_filename = config_get("plugin:proc:/proc/mdstat", "mismatch_cnt filename to monitor", filename);
    }

    if(unlikely(!ff)) {
        ff = procfile_open(mdstat_filename, " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry opening it next time

    size_t lines = procfile_lines(ff);
    size_t words = 0;

    if(unlikely(lines < 2)) {
        error("Cannot read /proc/mdstat. Expected 2 or more lines, read %zu.", lines);
        return 1;
    }

    // find how many raids are there
    size_t l;
    raids_num = 0;
    for(l = 1; l < lines - 2 ; l++) {
       if(unlikely(procfile_lineword(ff, l, 1)[0] == 'a')) // check if the raid is active
            raids_num++;
    }

    if(unlikely(!raids_num && !old_raids_allocated)) return 0; // we return 0, so that we will retry searching for raids next time

    // allocate the memory we need;
    if(unlikely(raids_num != raids_allocated)) {
        for(raid_idx = 0; raid_idx < raids_allocated; raid_idx++) {
            struct raid *raid = &raids[raid_idx];
            freez(raid->name);
            freez(raid->mismatch_cnt_filename);
        }
        if(raids_num) {
            raids = (struct raid *)reallocz(raids, raids_num * sizeof(struct raid));
            memset(raids, 0, raids_num * sizeof(struct raid));
        }
        else {
            freez(raids);
            raids = NULL;
        }
        raids_allocated = raids_num;
    }

    // loop through all lines except the first and the last ones
    for(l = 1, raid_idx = 0; l < (lines - 2) && raid_idx < raids_num; l++) {
        struct raid *raid = &raids[raid_idx];
        raid->redundant = 0;

        words = procfile_linewords(ff, l);
        if(unlikely(words < 2)) continue;

        if(unlikely(procfile_lineword(ff, l, 1)[0] != 'a')) continue;
        if(unlikely(!raid->name)) {
            raid->name = strdupz(procfile_lineword(ff, l, 0));
            raid->hash = simple_hash(raid->name);
        }
        else if(unlikely(strcmp(raid->name, procfile_lineword(ff, l, 0)))) {
            freez(raid->name);
            freez(raid->mismatch_cnt_filename);
            memset(raid, 0, sizeof(struct raid));
            raid->name = strdupz(procfile_lineword(ff, l, 0));
            raid->hash = simple_hash(raid->name);
        }
        if(unlikely(!raid->name || !raid->name[0])) continue;
        raid_idx++;

        // check if raid has disk status
        l++;
        words = procfile_linewords(ff, l);
        if(words < 2 || procfile_lineword(ff, l, words - 1)[0] != '[') continue;

        // split inuse and total number of disks
        if(likely(do_health || do_disks)) {
            char *s = NULL, *str_total = NULL, *str_inuse = NULL;

            s = procfile_lineword(ff, l, words - 2);
            if(unlikely(s[0] != '[')) {
                error("Cannot read /proc/mdstat raid health status. Unexpected format: missing opening bracket.");
                continue;
            }
            str_total = ++s;
            while(*s) {
                if(unlikely(*s == '/')) {
                    *s = '\0';
                    str_inuse = s + 1;
                }
                else if(unlikely(*s == ']')) {
                    *s = '\0';
                    break;
                }
                s++;
            }
            if(unlikely(str_total[0] == '\0' || str_inuse[0] == '\0')) {
                error("Cannot read /proc/mdstat raid health status. Unexpected format.");
                continue;
            }

            raid->inuse_disks = str2ull(str_inuse);
            raid->total_disks = str2ull(str_total);
            raid->failed_disks = raid->total_disks - raid->inuse_disks;
        }

        raid->redundant = 1;
        redundant_num++;
        l++;

        // check if any operation is performed on the raid
        if(likely(do_operations)) {
            char *s = NULL;

            raid->check = 0;
            raid->resync = 0;
            raid->recovery = 0;
            raid->reshape = 0;
            raid->finish_in = 0;
            raid->speed = 0;

            words = procfile_linewords(ff, l);
            if(likely(words < 2)) continue;
            if(unlikely(procfile_lineword(ff, l, 0)[0] != '[')) continue;
            if(unlikely(words < 7)) {
                error("Cannot read /proc/mdstat line. Expected 7 params, read %zu.", words);
                continue;
            }

            char *word;
            word = procfile_lineword(ff, l, 3);
            remove_trailing_chars(word, '%');

            unsigned long long percentage = (unsigned long long)(str2ld(word, NULL) * 100);
            // possible operations: check, resync, recovery, reshape
            // 4-th character is unique for each operation so it is checked
            switch(procfile_lineword(ff, l, 1)[3]) {
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

            word = procfile_lineword(ff, l, 5);
            s = remove_trailing_chars(word, 'm'); // remove trailing "min"

            word += 7; // skip leading "finish="

            if(likely(s > word))
                raid->finish_in = (unsigned long long)(str2ld(word, NULL) * 60);

            word = procfile_lineword(ff, l, 6);
            s = remove_trailing_chars(word, 'K'); // remove trailing "K/sec"

            word += 6; // skip leading "speed="

            if(likely(s > word))
                raid->speed = str2ull(word);
        }
    }

    // read mismatch_cnt files
    if(do_mismatch == -1) {
        if(do_mismatch_config == CONFIG_BOOLEAN_AUTO) {
            if(raids_num > 50)
                do_mismatch = CONFIG_BOOLEAN_NO;
            else
                do_mismatch = CONFIG_BOOLEAN_YES;
        }
        else
            do_mismatch = do_mismatch_config;
    }

    if(likely(do_mismatch)) {
        for(raid_idx = 0; raid_idx < raids_num ; raid_idx++) {
            char filename[FILENAME_MAX + 1];
            struct raid *raid = &raids[raid_idx];

            if(likely(raid->redundant)) {
                if(unlikely(!raid->mismatch_cnt_filename)) {
                    snprintfz(filename, FILENAME_MAX, mismatch_cnt_filename, raid->name);
                    raid->mismatch_cnt_filename = strdupz(filename);
                }
                if(unlikely(read_single_number_file(raid->mismatch_cnt_filename, &raid->mismatch_cnt))) {
                    error("Cannot read file '%s'", raid->mismatch_cnt_filename);
                    do_mismatch = CONFIG_BOOLEAN_NO;
                    error("Monitoring for mismatch count has been disabled");
                    break;
                }
            }
        }
    }

    // check for disappeared raids
    for(old_raid_idx = 0; old_raid_idx < old_raids_allocated; old_raid_idx++) {
        struct old_raid *old_raid = &old_raids[old_raid_idx];
        int found = 0;

        for(raid_idx = 0; raid_idx < raids_num ; raid_idx++) {
            struct raid *raid = &raids[raid_idx];

            if(unlikely(raid->hash == old_raid->hash
                        && !strcmp(raid->name, old_raid->name)
                        && raid->redundant == old_raid->redundant)) found = 1;
        }

        old_raid->found = found;
    }

    int raid_disappeared = 0;
    for(old_raid_idx = 0; old_raid_idx < old_raids_allocated; old_raid_idx++) {
        struct old_raid *old_raid = &old_raids[old_raid_idx];

        if(unlikely(!old_raid->found)) {
            if(likely(make_charts_obsolete)) {
                make_chart_obsolete(old_raid->name, "disks");
                make_chart_obsolete(old_raid->name, "mismatch");
                make_chart_obsolete(old_raid->name, "operation");
                make_chart_obsolete(old_raid->name, "finish");
                make_chart_obsolete(old_raid->name, "speed");
                make_chart_obsolete(old_raid->name, "availability");
            }
            raid_disappeared = 1;
        }
    }

    // allocate memory for nonredundant arrays
    if(unlikely(raid_disappeared || old_raids_allocated != raids_num)) {
        for(old_raid_idx = 0; old_raid_idx < old_raids_allocated; old_raid_idx++) {
            freez(old_raids[old_raid_idx].name);
        }
        if(likely(raids_num)) {
            old_raids = reallocz(old_raids, sizeof(struct old_raid) * raids_num);
            memset(old_raids, 0, sizeof(struct old_raid) * raids_num);
        }
        else {
            freez(old_raids);
            old_raids = NULL;
        }
        old_raids_allocated = raids_num;
        for(old_raid_idx = 0; old_raid_idx < old_raids_allocated; old_raid_idx++) {
            struct old_raid *old_raid = &old_raids[old_raid_idx];
            struct raid *raid = &raids[old_raid_idx];

            old_raid->name = strdupz(raid->name);
            old_raid->hash = raid->hash;
            old_raid->redundant = raid->redundant;
        }
    }

    // --------------------------------------------------------------------

    if(likely(do_health && redundant_num)) {
        static RRDSET *st_mdstat_health = NULL;
        if(unlikely(!st_mdstat_health)) {
            st_mdstat_health = rrdset_create_localhost(
                    "mdstat"
                    , "mdstat_health"
                    , NULL
                    , "health"
                    , "md.health"
                    , "Faulty Devices In MD"
                    , "failed disks"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MDSTAT_NAME
                    , NETDATA_CHART_PRIO_MDSTAT_HEALTH
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrdset_isnot_obsolete(st_mdstat_health);
        }
        else
            rrdset_next(st_mdstat_health);

        if(!redundant_num) {
            if(likely(make_charts_obsolete)) make_chart_obsolete("mdstat", "health");
        }
        else {
            for(raid_idx = 0; raid_idx < raids_num; raid_idx++) {
                struct raid *raid = &raids[raid_idx];

                if(likely(raid->redundant)) {
                    if(unlikely(!raid->rd_health && !(raid->rd_health = rrddim_find(st_mdstat_health, raid->name))))
                        raid->rd_health = rrddim_add(st_mdstat_health, raid->name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                    rrddim_set_by_pointer(st_mdstat_health, raid->rd_health, raid->failed_disks);
                }
            }

            rrdset_done(st_mdstat_health);
        }
    }

    // --------------------------------------------------------------------

    for(raid_idx = 0; raid_idx < raids_num ; raid_idx++) {
        struct raid *raid = &raids[raid_idx];
        char id[50 + 1];
        char family[50 + 1];

        if(likely(raid->redundant)) {
            if(likely(do_disks)) {
                snprintfz(id, 50, "%s_disks", raid->name);

                if(unlikely(!raid->st_disks && !(raid->st_disks = rrdset_find_byname_localhost(id)))) {
                    snprintfz(family, 50, "%s", raid->name);

                    raid->st_disks = rrdset_create_localhost(
                            "mdstat"
                            , id
                            , NULL
                            , family
                            , "md.disks"
                            , "Disks Stats"
                            , "disks"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_MDSTAT_NAME
                            , NETDATA_CHART_PRIO_MDSTAT_DISKS + raid_idx * 10
                            , update_every
                            , RRDSET_TYPE_STACKED
                    );

                    rrdset_isnot_obsolete(raid->st_disks);
                }
                else
                    rrdset_next(raid->st_disks);

                if(unlikely(!raid->rd_inuse && !(raid->rd_inuse = rrddim_find(raid->st_disks, "inuse"))))
                    raid->rd_inuse = rrddim_add(raid->st_disks, "inuse", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                if(unlikely(!raid->rd_down && !(raid->rd_down = rrddim_find(raid->st_disks, "down"))))
                    raid->rd_down = rrddim_add(raid->st_disks, "down", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrddim_set_by_pointer(raid->st_disks, raid->rd_inuse, raid->inuse_disks);
                rrddim_set_by_pointer(raid->st_disks, raid->rd_down, raid->failed_disks);

                rrdset_done(raid->st_disks);
            }

            // --------------------------------------------------------------------

            if(likely(do_mismatch)) {
                snprintfz(id, 50, "%s_mismatch", raid->name);

                if(unlikely(!raid->st_mismatch_cnt && !(raid->st_mismatch_cnt = rrdset_find_byname_localhost(id)))) {
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
                            , NETDATA_CHART_PRIO_MDSTAT_MISMATCH + raid_idx * 10
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_isnot_obsolete(raid->st_mismatch_cnt);
                }
                else
                    rrdset_next(raid->st_mismatch_cnt);

                if(unlikely(!raid->rd_mismatch_cnt && !(raid->rd_mismatch_cnt = rrddim_find(raid->st_mismatch_cnt, "count"))))
                    raid->rd_mismatch_cnt = rrddim_add(raid->st_mismatch_cnt, "count", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrddim_set_by_pointer(raid->st_mismatch_cnt, raid->rd_mismatch_cnt, raid->mismatch_cnt);

                rrdset_done(raid->st_mismatch_cnt);
            }

            // --------------------------------------------------------------------

            if(likely(do_operations)) {
                snprintfz(id, 50, "%s_operation", raid->name);

                if(unlikely(!raid->st_operation && !(raid->st_operation = rrdset_find_byname_localhost(id)))) {
                    snprintfz(family, 50, "%s", raid->name);

                    raid->st_operation = rrdset_create_localhost(
                            "mdstat"
                            , id
                            , NULL
                            , family
                            , "md.status"
                            , "Current Status"
                            , "percent"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_MDSTAT_NAME
                            , NETDATA_CHART_PRIO_MDSTAT_OPERATION + raid_idx * 10
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_isnot_obsolete(raid->st_operation);
                }
                else
                    rrdset_next(raid->st_operation);

                if(unlikely(!raid->rd_check && !(raid->rd_check = rrddim_find(raid->st_operation, "check"))))
                    raid->rd_check = rrddim_add(raid->st_operation, "check", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                if(unlikely(!raid->rd_resync && !(raid->rd_resync = rrddim_find(raid->st_operation, "resync"))))
                    raid->rd_resync = rrddim_add(raid->st_operation, "resync", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                if(unlikely(!raid->rd_recovery && !(raid->rd_recovery = rrddim_find(raid->st_operation, "recovery"))))
                    raid->rd_recovery = rrddim_add(raid->st_operation, "recovery", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                if(unlikely(!raid->rd_reshape && !(raid->rd_reshape = rrddim_find(raid->st_operation, "reshape"))))
                    raid->rd_reshape = rrddim_add(raid->st_operation, "reshape", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);

                rrddim_set_by_pointer(raid->st_operation, raid->rd_check, raid->check);
                rrddim_set_by_pointer(raid->st_operation, raid->rd_resync, raid->resync);
                rrddim_set_by_pointer(raid->st_operation, raid->rd_recovery, raid->recovery);
                rrddim_set_by_pointer(raid->st_operation, raid->rd_reshape, raid->reshape);

                rrdset_done(raid->st_operation);

                // --------------------------------------------------------------------

                snprintfz(id, 50, "%s_finish", raid->name);

                if(unlikely(!raid->st_finish && !(raid->st_finish = rrdset_find_byname_localhost(id)))) {
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
                            , NETDATA_CHART_PRIO_MDSTAT_FINISH + raid_idx * 10
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_isnot_obsolete(raid->st_finish);
                }
                else
                    rrdset_next(raid->st_finish);

                if(unlikely(!raid->rd_finish_in && !(raid->rd_finish_in = rrddim_find(raid->st_finish, "finish_in"))))
                    raid->rd_finish_in = rrddim_add(raid->st_finish, "finish_in", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrddim_set_by_pointer(raid->st_finish, raid->rd_finish_in, raid->finish_in);

                rrdset_done(raid->st_finish);

                // --------------------------------------------------------------------

                snprintfz(id, 50, "%s_speed", raid->name);

                if(unlikely(!raid->st_speed && !(raid->st_speed = rrdset_find_byname_localhost(id)))) {
                    snprintfz(family, 50, "%s", raid->name);

                    raid->st_speed = rrdset_create_localhost(
                            "mdstat"
                            , id
                            , NULL
				    , family
                            , "md.rate"
                            , "Operation Speed"
                            , "KiB/s"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_MDSTAT_NAME
                            , NETDATA_CHART_PRIO_MDSTAT_SPEED + raid_idx * 10
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_isnot_obsolete(raid->st_speed);
                }
                else
                    rrdset_next(raid->st_speed);

                if(unlikely(!raid->rd_speed && !(raid->rd_speed = rrddim_find(raid->st_speed, "speed"))))
                    raid->rd_speed = rrddim_add(raid->st_speed, "speed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrddim_set_by_pointer(raid->st_speed, raid->rd_speed, raid->speed);

                rrdset_done(raid->st_speed);
            }
        }
        else {

            // --------------------------------------------------------------------

            if(likely(do_nonredundant)) {
                snprintfz(id, 50, "%s_availability", raid->name);

                if(unlikely(!raid->st_nonredundant && !(raid->st_nonredundant = rrdset_find_localhost(id)))) {
                    snprintfz(family, 50, "%s", raid->name);

                    raid->st_nonredundant = rrdset_create_localhost(
                            "mdstat"
                            , id
                            , NULL
                            , family
                            , "md.nonredundant"
                            , "Nonredundant Array Availability"
                            , "boolean"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_MDSTAT_NAME
                            , NETDATA_CHART_PRIO_MDSTAT_NONREDUNDANT + raid_idx * 10
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_isnot_obsolete(raid->st_nonredundant);
                }
                else
                    rrdset_next(raid->st_nonredundant);

                if(unlikely(!raid->rd_nonredundant && !(raid->rd_nonredundant = rrddim_find(raid->st_nonredundant, "available"))))
                    raid->rd_nonredundant = rrddim_add(raid->st_nonredundant, "available", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrddim_set_by_pointer(raid->st_nonredundant, raid->rd_nonredundant, 1);

                rrdset_done(raid->st_nonredundant);
            }
        }
    }

    return 0;
}
