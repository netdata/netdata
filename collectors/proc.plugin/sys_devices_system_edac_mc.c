// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

struct edac_count {
    bool updated;
    char *filename;
    procfile *ff;
    kernel_uint_t count;
    RRDDIM *rd;
};

struct mc {
    char *name;

    struct edac_count ce;
    struct edac_count ue;
    struct edac_count ce_noinfo;
    struct edac_count ue_noinfo;

    RRDSET *st;

    struct mc *prev, *next;
};

static struct mc *mc_root = NULL;
static char *mc_dirname = NULL;

static void find_all_mc() {
    char name[FILENAME_MAX + 1];
    snprintfz(name, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/system/edac/mc");
    mc_dirname = config_get("plugin:proc:/sys/devices/system/edac/mc", "directory to monitor", name);

    DIR *dir = opendir(mc_dirname);
    if(unlikely(!dir)) {
        collector_error("Cannot read ECC memory errors directory '%s'", mc_dirname);
        return;
    }

    struct dirent *de = NULL;
    while((de = readdir(dir))) {
        if(de->d_type == DT_DIR && de->d_name[0] == 'm' && de->d_name[1] == 'c' && isdigit(de->d_name[2])) {
            struct mc *m = callocz(1, sizeof(struct mc));
            m->name = strdupz(de->d_name);

            struct stat st;

            snprintfz(name, FILENAME_MAX, "%s/%s/ce_count", mc_dirname, de->d_name);
            if(stat(name, &st) != -1)
                m->ce.filename = strdupz(name);

            snprintfz(name, FILENAME_MAX, "%s/%s/ue_count", mc_dirname, de->d_name);
            if(stat(name, &st) != -1)
                m->ue.filename = strdupz(name);

            snprintfz(name, FILENAME_MAX, "%s/%s/ce_noinfo_count", mc_dirname, de->d_name);
            if(stat(name, &st) != -1)
                m->ce_noinfo.filename = strdupz(name);

            snprintfz(name, FILENAME_MAX, "%s/%s/ue_noinfo_count", mc_dirname, de->d_name);
            if(stat(name, &st) != -1)
                m->ue_noinfo.filename = strdupz(name);

            if(!m->ce.filename && !m->ue.filename && !m->ce_noinfo.filename && !m->ue_noinfo.filename) {
                freez(m->name);
                freez(m);
            }
            else
                DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(mc_root, m, prev, next);
        }
    }

    closedir(dir);
}

static kernel_uint_t read_edac_count(struct edac_count *t) {
    t->updated = false;
    t->count = 0;

    if(t->filename) {
        if(unlikely(!t->ff)) {
            t->ff = procfile_open(t->filename, " \t", PROCFILE_FLAG_DEFAULT);
            if(unlikely(!t->ff))
                return 0;
        }

        t->ff = procfile_readall(t->ff);
        if(unlikely(!t->ff || procfile_lines(t->ff) < 1 || procfile_linewords(t->ff, 0) < 1))
            return 0;

        t->count = str2ull(procfile_lineword(t->ff, 0, 0), NULL);
        t->updated = true;
    }

    return t->count;
}

static bool read_edac_mc_file(char *mc, const char *filename, char *out, size_t out_size) {
    char f[FILENAME_MAX + 1];
    snprintfz(f, FILENAME_MAX, "%s/%s/filename", mc_dirname, mc, filename);
    return read_file(f, out, out_size) == 0;
}

int do_proc_sys_devices_system_edac_mc(int update_every, usec_t dt __maybe_unused) {
    if(unlikely(!mc_root)) {
        find_all_mc();

        if(!mc_root)
            // don't call this again
            return 1;
    }

    for(struct mc *m = mc_root; m; m = m->next) {
        read_edac_count(&m->ce);
        read_edac_count(&m->ce_noinfo);
        read_edac_count(&m->ue);
        read_edac_count(&m->ue_noinfo);
    }

    // --------------------------------------------------------------------

    for(struct mc *m = mc_root; m ; m = m->next) {
        if(unlikely(!m->st)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "mc_edac_%s", m->name);
            m->st = rrdset_create_localhost(
                    "mem"
                    , id
                    , NULL
                    , "edac"
                    , "mem.mc_edac"
                    , "Memory Controller (MC) Error Detection And Correction (EDAC) Errors"
                    , "errors"
                    , PLUGIN_PROC_NAME
                    , "/sys/devices/system/edac/mc"
                    , NETDATA_CHART_PRIO_MEM_HW_ECC_CE
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrdlabels_add(m->st->rrdlabels, "controller", m->name, RRDLABEL_SRC_AUTO);

            char buffer[1024 + 1];

            if(read_edac_mc_file(m->name, "mc_name", buffer, 1024))
                rrdlabels_add(m->st->rrdlabels, "mc_name", buffer, RRDLABEL_SRC_AUTO);

            if(read_edac_mc_file(m->name, "size_mb", buffer, 1024))
                rrdlabels_add(m->st->rrdlabels, "size_mb", buffer, RRDLABEL_SRC_AUTO);

            if(read_edac_mc_file(m->name, "max_location", buffer, 1024))
                rrdlabels_add(m->st->rrdlabels, "max_location", buffer, RRDLABEL_SRC_AUTO);

            m->ce.rd = rrddim_add(m->st, "correctable", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            m->ue.rd = rrddim_add(m->st, "uncorrectable", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            m->ce_noinfo.rd = rrddim_add(m->st, "correctable_noinfo", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            m->ue_noinfo.rd = rrddim_add(m->st, "uncorrectable_noinfo", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(m->st, m->ce.rd, (collected_number)m->ce.count);
        rrddim_set_by_pointer(m->st, m->ue.rd, (collected_number)m->ue.count);
        rrddim_set_by_pointer(m->st, m->ce_noinfo.rd, (collected_number)m->ce_noinfo.count);
        rrddim_set_by_pointer(m->st, m->ue_noinfo.rd, (collected_number)m->ue_noinfo.count);
    }

    return 0;
}
