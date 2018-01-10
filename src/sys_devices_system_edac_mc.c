#include "common.h"

struct mc {
    char *name;
    char ce_updated;
    char ue_updated;

    char *ce_count_filename;
    char *ue_count_filename;

    procfile *ce_ff;
    procfile *ue_ff;

    collected_number ce_count;
    collected_number ue_count;

    RRDDIM *ce_rd;
    RRDDIM *ue_rd;

    struct mc *next;
};
static struct mc *mc_root = NULL;

static void find_all_mc() {
    char name[FILENAME_MAX + 1];
    snprintfz(name, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/system/edac/mc");
    char *dirname = config_get("plugin:proc:/sys/devices/system/edac/mc", "directory to monitor", name);

    DIR *dir = opendir(dirname);
    if(unlikely(!dir)) {
        error("Cannot read ECC memory errors directory '%s'", dirname);
        return;
    }

    struct dirent *de = NULL;
    while((de = readdir(dir))) {
        if(de->d_type == DT_DIR && de->d_name[0] == 'm' && de->d_name[1] == 'c' && isdigit(de->d_name[2])) {
            struct mc *m = callocz(1, sizeof(struct mc));
            m->name = strdupz(de->d_name);

            struct stat st;

            snprintfz(name, FILENAME_MAX, "%s/%s/ce_count", dirname, de->d_name);
            if(stat(name, &st) != -1)
                m->ce_count_filename = strdupz(name);

            snprintfz(name, FILENAME_MAX, "%s/%s/ue_count", dirname, de->d_name);
            if(stat(name, &st) != -1)
                m->ue_count_filename = strdupz(name);

            if(!m->ce_count_filename && !m->ue_count_filename) {
                freez(m->name);
                freez(m);
            }
            else {
                m->next = mc_root;
                mc_root = m;
            }
        }
    }

    closedir(dir);
}

int do_proc_sys_devices_system_edac_mc(int update_every, usec_t dt) {
    (void)dt;

    if(unlikely(mc_root == NULL)) {
        find_all_mc();
        if(unlikely(mc_root == NULL))
            return 1;
    }

    static int do_ce = -1, do_ue = -1;
    calculated_number ce_sum = 0, ue_sum = 0;
    struct mc *m;

    if(unlikely(do_ce == -1)) {
        do_ce = config_get_boolean_ondemand("plugin:proc:/sys/devices/system/edac/mc", "enable ECC memory correctable errors", CONFIG_BOOLEAN_AUTO);
        do_ue = config_get_boolean_ondemand("plugin:proc:/sys/devices/system/edac/mc", "enable ECC memory uncorrectable errors", CONFIG_BOOLEAN_AUTO);
    }

    if(do_ce != CONFIG_BOOLEAN_NO) {
        for(m = mc_root; m; m = m->next) {
            if(m->ce_count_filename) {
                m->ce_updated = 0;

                if(unlikely(!m->ce_ff)) {
                    m->ce_ff = procfile_open(m->ce_count_filename, " \t", PROCFILE_FLAG_DEFAULT);
                    if(unlikely(!m->ce_ff))
                        continue;
                }

                m->ce_ff = procfile_readall(m->ce_ff);
                if(unlikely(!m->ce_ff || procfile_lines(m->ce_ff) < 1 || procfile_linewords(m->ce_ff, 0) < 1))
                    continue;

                m->ce_count = str2ull(procfile_lineword(m->ce_ff, 0, 0));
                ce_sum += m->ce_count;
                m->ce_updated = 1;
            }
        }
    }

    if(do_ue != CONFIG_BOOLEAN_NO) {
        for(m = mc_root; m; m = m->next) {
            if(m->ue_count_filename) {
                m->ue_updated = 0;

                if(unlikely(!m->ue_ff)) {
                    m->ue_ff = procfile_open(m->ue_count_filename, " \t", PROCFILE_FLAG_DEFAULT);
                    if(unlikely(!m->ue_ff))
                        continue;
                }

                m->ue_ff = procfile_readall(m->ue_ff);
                if(unlikely(!m->ue_ff || procfile_lines(m->ue_ff) < 1 || procfile_linewords(m->ue_ff, 0) < 1))
                    continue;

                m->ue_count = str2ull(procfile_lineword(m->ue_ff, 0, 0));
                ue_sum += m->ue_count;
                m->ue_updated = 1;
            }
        }
    }

    // --------------------------------------------------------------------

    if(do_ce == CONFIG_BOOLEAN_YES || (do_ce == CONFIG_BOOLEAN_AUTO && ce_sum > 0)) {
        do_ce = CONFIG_BOOLEAN_YES;

        static RRDSET *ce_st = NULL;

        if(unlikely(!ce_st)) {
            ce_st = rrdset_create_localhost(
                    "mem"
                    , "ecc_ce"
                    , NULL
                    , "ecc"
                    , NULL
                    , "ECC Memory Correctable Errors"
                    , "errors"
                    , "proc"
                    , "/sys/devices/system/edac/mc"
                    , NETDATA_CHART_PRIO_MEM_HW + 50
                    , update_every
                    , RRDSET_TYPE_LINE
            );
        }
        else
            rrdset_next(ce_st);

        for(m = mc_root; m; m = m->next) {
            if (m->ce_count_filename && m->ce_updated) {
                if(unlikely(!m->ce_rd))
                    m->ce_rd = rrddim_add(ce_st, m->name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrddim_set_by_pointer(ce_st, m->ce_rd, m->ce_count);
            }
        }

        rrdset_done(ce_st);
    }

    // --------------------------------------------------------------------

    if(do_ue == CONFIG_BOOLEAN_YES || (do_ue == CONFIG_BOOLEAN_AUTO && ue_sum > 0)) {
        do_ue = CONFIG_BOOLEAN_YES;

        static RRDSET *ue_st = NULL;

        if(unlikely(!ue_st)) {
            ue_st = rrdset_create_localhost(
                    "mem"
                    , "ecc_ue"
                    , NULL
                    , "ecc"
                    , NULL
                    , "ECC Memory Uncorrectable Errors"
                    , "errors"
                    , "proc"
                    , "/sys/devices/system/edac/mc"
                    , NETDATA_CHART_PRIO_MEM_HW + 60
                    , update_every
                    , RRDSET_TYPE_LINE
            );
        }
        else
            rrdset_next(ue_st);

        for(m = mc_root; m; m = m->next) {
            if (m->ue_count_filename && m->ue_updated) {
                if(unlikely(!m->ue_rd))
                    m->ue_rd = rrddim_add(ue_st, m->name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrddim_set_by_pointer(ue_st, m->ue_rd, m->ue_count);
            }
        }

        rrdset_done(ue_st);
    }

    return 0;
}
