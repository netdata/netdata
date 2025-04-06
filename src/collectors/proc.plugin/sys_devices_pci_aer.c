// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

static const char *pci_aer_dirname = NULL;

typedef enum __attribute__((packed)) {
    AER_DEV_NONFATAL                    = (1 << 0),
    AER_DEV_CORRECTABLE                 = (1 << 1),
    AER_DEV_FATAL                       = (1 << 2),
    AER_ROOTPORT_TOTAL_ERR_COR          = (1 << 3),
    AER_ROOTPORT_TOTAL_ERR_FATAL        = (1 << 4),
} AER_TYPE;

struct aer_value {
    kernel_uint_t count;
    RRDDIM *rd;
};

struct aer_entry {
    bool updated;

    STRING *name;
    AER_TYPE type;

    procfile *ff;
    DICTIONARY *values;

    RRDSET *st;
};

DICTIONARY *aer_root = NULL;

static bool aer_value_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
	struct aer_value *v = old_value;
    struct aer_value *nv = new_value;

    v->count = nv->count;

    return false;
}

static void aer_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct aer_entry *a = value;
    a->values = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE|DICT_OPTION_FIXED_SIZE, &dictionary_stats_category_collectors, sizeof(struct aer_value));
    dictionary_register_conflict_callback(a->values, aer_value_conflict_callback, NULL);
}

static void add_pci_aer(const char *base_dir, const char *d_name, AER_TYPE type) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/%s", base_dir, d_name);
    struct aer_entry *a = dictionary_set(aer_root, buffer, NULL, sizeof(struct aer_entry));

    if(!a->name)
        a->name = string_strdupz(d_name);

    a->type = type;
}

static bool recursively_find_pci_aer(AER_TYPE types, const char *base_dir, const char *d_name, int depth) {
    if(depth > 100)
        return false;

    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/%s", base_dir, d_name);
    DIR *dir = opendir(buffer);
    if(unlikely(!dir)) {
        collector_error("Cannot read PCI_AER directory '%s'", buffer);
        return true;
    }

    struct dirent *de = NULL;
    while((de = readdir(dir))) {
        if(de->d_type == DT_DIR) {
            if(de->d_name[0] == '.')
                continue;

            recursively_find_pci_aer(types, buffer, de->d_name, depth + 1);
        }
        else if(de->d_type == DT_REG) {
            if((types & AER_DEV_NONFATAL) && strcmp(de->d_name, "aer_dev_nonfatal") == 0) {
                add_pci_aer(buffer, de->d_name, AER_DEV_NONFATAL);
            }
            else if((types & AER_DEV_CORRECTABLE) && strcmp(de->d_name, "aer_dev_correctable") == 0) {
                add_pci_aer(buffer, de->d_name, AER_DEV_CORRECTABLE);
            }
            else if((types & AER_DEV_FATAL) && strcmp(de->d_name, "aer_dev_fatal") == 0) {
                add_pci_aer(buffer, de->d_name, AER_DEV_FATAL);
            }
            else if((types & AER_ROOTPORT_TOTAL_ERR_COR) && strcmp(de->d_name, "aer_rootport_total_err_cor") == 0) {
                add_pci_aer(buffer, de->d_name, AER_ROOTPORT_TOTAL_ERR_COR);
            }
            else if((types & AER_ROOTPORT_TOTAL_ERR_FATAL) && strcmp(de->d_name, "aer_rootport_total_err_fatal") == 0) {
                add_pci_aer(buffer, de->d_name, AER_ROOTPORT_TOTAL_ERR_FATAL);
            }
        }
    }
    closedir(dir);
    return true;
}

static void find_all_pci_aer(AER_TYPE types) {
    char name[FILENAME_MAX + 1];
    snprintfz(name, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices");
    pci_aer_dirname = inicfg_get(&netdata_config, "plugin:proc:/sys/devices/pci/aer", "directory to monitor", name);

    DIR *dir = opendir(pci_aer_dirname);
    if(unlikely(!dir)) {
        collector_error("Cannot read PCI_AER directory '%s'", pci_aer_dirname);
        return;
    }

    struct dirent *de = NULL;
    while((de = readdir(dir))) {
        if(de->d_type == DT_DIR && de->d_name[0] == 'p' && de->d_name[1] == 'c' && de->d_name[2] == 'i' && isdigit(de->d_name[3]))
            recursively_find_pci_aer(types, pci_aer_dirname, de->d_name, 1);
    }
    closedir(dir);
}

static void read_pci_aer_values(const char *filename, struct aer_entry *t) {
    t->updated = false;

    if(unlikely(!t->ff)) {
        t->ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!t->ff))
            return;
    }

    t->ff = procfile_readall(t->ff);
    if(unlikely(!t->ff || procfile_lines(t->ff) < 1 || procfile_linewords(t->ff, 0) < 1))
        return;

    size_t lines = procfile_lines(t->ff);
    for(size_t l = 0; l < lines ; l++) {
        if(procfile_linewords(t->ff, l) != 2)
            continue;

        struct aer_value v = {
                .count = str2ull(procfile_lineword(t->ff, l, 1), NULL)
        };

        char *key = procfile_lineword(t->ff, l, 0);
        if(!key || !*key || (key[0] == 'T' && key[1] == 'O' && key[2] == 'T' && key[3] == 'A' && key[4] == 'L' && key[5] == '_'))
            continue;

        dictionary_set(t->values, key, &v, sizeof(v));
    }

    t->updated = true;
}

static void read_pci_aer_count(const char *filename, struct aer_entry *t) {
    t->updated = false;

    if(unlikely(!t->ff)) {
        t->ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!t->ff))
            return;
    }

    t->ff = procfile_readall(t->ff);
    if(unlikely(!t->ff || procfile_lines(t->ff) < 1 || procfile_linewords(t->ff, 0) < 1))
        return;

    struct aer_value v = {
            .count = str2ull(procfile_lineword(t->ff, 0, 0), NULL)
    };
    dictionary_set(t->values, "count", &v, sizeof(v));
    t->updated = true;
}

static void add_label_from_link(struct aer_entry *a, const char *path, const char *link) {
    char name[FILENAME_MAX + 1];
    strncpyz(name, path, FILENAME_MAX);
    char *slash = strrchr(name, '/');
    if(slash)
        *slash = '\0';

    char name2[FILENAME_MAX + 1];
    snprintfz(name2, FILENAME_MAX, "%s/%s", name, link);

    ssize_t len = readlink(name2, name, FILENAME_MAX);
    if(len != -1) {
        name[len] = '\0';  // Null-terminate the string
        slash = strrchr(name, '/');
        if(slash) slash++;
        else slash = name;
        rrdlabels_add(a->st->rrdlabels, link, slash, RRDLABEL_SRC_AUTO);
    }
}

int do_proc_sys_devices_pci_aer(int update_every, usec_t dt __maybe_unused) {
    if(unlikely(!aer_root)) {
        int do_root_ports = CONFIG_BOOLEAN_AUTO;
        int do_pci_slots = CONFIG_BOOLEAN_NO;

        char buffer[128];
        rrdlabels_get_value_strcpyz(localhost->rrdlabels, buffer, sizeof(buffer), "_virtualization");
        if(strcmp(buffer, "none") != 0) {
            // no need to run on virtualized environments
            do_root_ports = CONFIG_BOOLEAN_NO;
            do_pci_slots = CONFIG_BOOLEAN_NO;
        }

        do_root_ports = inicfg_get_boolean(&netdata_config, "plugin:proc:/sys/class/pci/aer", "enable root ports", do_root_ports);
        do_pci_slots = inicfg_get_boolean(&netdata_config, "plugin:proc:/sys/class/pci/aer", "enable pci slots", do_pci_slots);

        if(!do_root_ports && !do_pci_slots)
            return 1;

        aer_root = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, &dictionary_stats_category_collectors, sizeof(struct aer_entry));
        dictionary_register_insert_callback(aer_root, aer_insert_callback, NULL);

        AER_TYPE types = ((do_root_ports) ? (AER_ROOTPORT_TOTAL_ERR_COR|AER_ROOTPORT_TOTAL_ERR_FATAL) : 0) |
                ((do_pci_slots) ? (AER_DEV_FATAL|AER_DEV_NONFATAL|AER_DEV_CORRECTABLE) : 0);

        find_all_pci_aer(types);

        if(!dictionary_entries(aer_root))
            return 1;
    }

	struct aer_entry *a;
    dfe_start_read(aer_root, a) {
        switch(a->type) {
            case AER_DEV_NONFATAL:
            case AER_DEV_FATAL:
            case AER_DEV_CORRECTABLE:
                read_pci_aer_values(a_dfe.name, a);
                break;

            case AER_ROOTPORT_TOTAL_ERR_COR:
            case AER_ROOTPORT_TOTAL_ERR_FATAL:
                read_pci_aer_count(a_dfe.name, a);
                break;
        }

        if(!a->updated)
            continue;

        if(!a->st) {
            const char *title = "";
            const char *context = "";

            switch(a->type) {
                case AER_DEV_NONFATAL:
                    title = "PCI Advanced Error Reporting (AER) Non-Fatal Errors";
                    context = "pci.aer_nonfatal";
                    break;

                case AER_DEV_FATAL:
                    title = "PCI Advanced Error Reporting (AER) Fatal Errors";
                    context = "pci.aer_fatal";
                    break;

                case AER_DEV_CORRECTABLE:
                    title = "PCI Advanced Error Reporting (AER) Correctable Errors";
                    context = "pci.aer_correctable";
                    break;

                case AER_ROOTPORT_TOTAL_ERR_COR:
                    title = "PCI Root-Port Advanced Error Reporting (AER) Correctable Errors";
                    context = "pci.rootport_aer_correctable";
                    break;

                case AER_ROOTPORT_TOTAL_ERR_FATAL:
                    title = "PCI Root-Port Advanced Error Reporting (AER) Fatal Errors";
                    context = "pci.rootport_aer_fatal";
                    break;

                default:
                    title = "Unknown PCI Advanced Error Reporting";
                    context = "pci.unknown_aer";
                    break;
            }

            char id[RRD_ID_LENGTH_MAX + 1];
            char nm[RRD_ID_LENGTH_MAX + 1];
            size_t len = strlen(pci_aer_dirname);

            const char *fname = a_dfe.name;
            if(strncmp(a_dfe.name, pci_aer_dirname, len) == 0)
                fname = &a_dfe.name[len];

            if(*fname == '/')
                fname++;

            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_%s", &context[4], fname);
            char *slash = strrchr(id, '/');
            if(slash)
                *slash = '\0';

            netdata_fix_chart_id(id);

            snprintfz(nm, RRD_ID_LENGTH_MAX, "%s", fname);
            slash = strrchr(nm, '/');
            if(slash)
                *slash = '\0';

            a->st = rrdset_create_localhost(
                    "pci"
            		, id
            		, NULL
            		, "aer"
            		, context
            		, title
            		, "errors/s"
            		, PLUGIN_PROC_NAME
            		, "/sys/devices/pci/aer"
            		, NETDATA_CHART_PRIO_PCI_AER
            		, update_every
            		, RRDSET_TYPE_LINE
            );

            rrdlabels_add(a->st->rrdlabels, "device", nm, RRDLABEL_SRC_AUTO);
            add_label_from_link(a, a_dfe.name, "driver");

            struct aer_value *v;
            dfe_start_read(a->values, v) {
                v->rd = rrddim_add(a->st, v_dfe.name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            dfe_done(v);
        }

        struct aer_value *v;
        dfe_start_read(a->values, v) {
            if(unlikely(!v->rd))
                v->rd = rrddim_add(a->st, v_dfe.name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(a->st, v->rd, (collected_number)v->count);
        }
        dfe_done(v);

        rrdset_done(a->st);
    }
    dfe_done(a);

    return 0;
}
