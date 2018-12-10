// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_POWER_SUPPLY_NAME "/sys/class/power_supply"

const char *ps_property_names[] = {"charge", "energy", "voltage"};
const char *ps_property_dim_names[] = {"charge_full_design", "charge_full", "charge_now", "charge_empty", "charge_empty_design",
                                       "energy_full_design", "energy_full", "energy_now", "energy_empty", "energy_empty_design",
                                       "voltage_max_design", "voltage_max", "voltage_now", "voltage_min", "voltage_min_design"};

struct ps_property_dim {
    char *name;
    char *filename;
    int fd;
    RRDDIM rd;
    unsigned long long value;
};

struct ps_property {
    char *name;
    RRDSET *st;

    int dims_num;
    struct ps_property_dim *rd;
};

struct capacity {
    char *filename;
    int fd;

    RRDSET *st;
    RRDDIM *rd;
    unsigned long long value;
};

struct power_supply {
    char *name;
    uint32_t hash;
    int found;

    struct capacity *capacity;

    int properties_num;
    struct ps_property *properties;

    struct power_supply *next;
};

static struct power_supply *power_supply_root = NULL;

void power_supply_free(struct power_supply *ps) {
    if(ps) {
        if(ps->capacity) {
            if(ps->capacity->st) rrdset_is_obsolete(ps->capacity->st);
            freez(ps->capacity->filename);
            freez(ps->capacity);
        }
        freez(ps->name);

        if(ps == power_supply_root) {
            power_supply_root = ps->next;
        }
        else {
            struct power_supply *last;
            for(last = power_supply_root; last && last->next != ps; last = last->next);
            last->next = ps->next; // TODO: Fatal if last == NULL;
        }
        freez(ps);
    }
}

int do_sys_class_power_supply(int update_every, usec_t dt) {
    (void)dt;
    static int do_capacity = -1, do_charge = -1, do_energy = -1, do_voltage = -1;
    static char *dirname = NULL;

    if(unlikely(do_capacity == -1)) {
        do_capacity = config_get_boolean("plugin:proc:/sys/class/power_supply", "battery capacity", CONFIG_BOOLEAN_YES);
        do_charge   = config_get_boolean("plugin:proc:/sys/class/power_supply", "battery charge", CONFIG_BOOLEAN_NO);
        do_energy   = config_get_boolean("plugin:proc:/sys/class/power_supply", "battery energy", CONFIG_BOOLEAN_NO);
        do_voltage  = config_get_boolean("plugin:proc:/sys/class/power_supply", "power supply voltage", CONFIG_BOOLEAN_NO);

        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/power_supply");
        dirname = config_get("plugin:proc:/sys/class/power_supply", "directory to monitor", filename);
    }

    DIR *dir = opendir(dirname);
    if(unlikely(!dir)) {
        error("Cannot read directory '%s'", dirname);
        return 1;
    }

    struct dirent *de = NULL;
    while(likely(de = readdir(dir))) {
        if(de->d_type == DT_DIR
            && (
                (de->d_name[0] == '.' && de->d_name[1] == '\0')
                || (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0')
                ))
            continue;

        if(likely(de->d_type == DT_DIR)) {
            uint32_t hash = simple_hash(de->d_name);

            struct power_supply *ps;
            for(ps = power_supply_root; ps; ps = ps->next) {
                if(unlikely(ps->hash == hash && !strcmp(ps->name, de->d_name))) {
                    ps->found = 1;
                    break;
                }
            }

            // allocate memory and initialize it if needed
            if(unlikely(!ps)) {
                ps = callocz(sizeof(struct power_supply), 1);
                ps->name = strdupz(de->d_name);
                ps->hash = simple_hash(de->d_name);
                ps->found = 1;
                ps->next = power_supply_root;
                power_supply_root = ps;

                struct stat stbuf;
                if(likely(do_capacity != CONFIG_BOOLEAN_NO)) {
                    char filename[FILENAME_MAX + 1];
                    snprintfz(filename, FILENAME_MAX, "%s/%s/%s", dirname, de->d_name, "capacity");
                    if (stat(filename, &stbuf) == 0) {
                        ps->capacity = callocz(sizeof(struct capacity), 1);
                        ps->capacity->filename = strdupz(filename);
                    }
                }
            }

            // TODO: keep files open
            if(unlikely(read_single_number_file(ps->capacity->filename, &ps->capacity->value))) {
                error("Cannot read file '%s'", ps->capacity->filename);
                power_supply_free(ps);
            }
        }
    }

    closedir(dir);

    // --------------------------------------------------------------------

    struct power_supply *ps = power_supply_root;
    while(ps) {
        if(!ps->found) {
            struct power_supply *f = ps;
            ps = ps->next;
            power_supply_free(f);
            continue;
        }

        if(ps->capacity) {
            if(unlikely(!ps->capacity->st)) {
                char id[50 + 1];
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_capacity", ps->name);

                ps->capacity->st = rrdset_create_localhost(
                        "powersupply"
                        , id
                        , NULL
                        , ps->name
                        , NULL
                        , "Capacity"
                        , "percentage"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_POWER_SUPPLY_NAME
                        , NETDATA_CHART_PRIO_POWER_SUPPLY_CAPACITY
                        , update_every
                        , RRDSET_TYPE_LINE
                );
            }
            else
                rrdset_next(ps->capacity->st);

            if(!ps->capacity->rd) ps->capacity->rd = rrddim_add(ps->capacity->st, "capacity", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrddim_set_by_pointer(ps->capacity->st, ps->capacity->rd, ps->capacity->value);

            rrdset_done(ps->capacity->st);
            ps->found = 0;
            ps = ps->next;
        }
    }

    return 0;
}
