// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_POWER_SUPPLY_NAME "/sys/class/power_supply"

const char *ps_property_names[]  = {        "charge",         "energy",              "voltage"};
const char *ps_property_titles[] = {"Battery charge", "Battery energy", "Power supply voltage"};
const char *ps_property_units[]  = {            "Ah",             "Wh",                    "V"};

const char *ps_property_dim_names[] = {"empty_design", "empty", "now", "full", "full_design",
                                       "empty_design", "empty", "now", "full", "full_design",
                                         "min_design",   "min", "now",  "max",  "max_design"};

struct ps_property_dim {
    char *name;
    char *filename;

    RRDDIM *rd;
    unsigned long long value;

    struct ps_property_dim *next;
};

struct ps_property {
    char *name;
    char *title;
    char *units;

    RRDSET *st;

    struct ps_property_dim *property_dim_root;

    struct ps_property *next;
};

struct capacity {
    char *filename;

    RRDSET *st;
    RRDDIM *rd;
    unsigned long long value;
};

struct power_supply {
    char *name;
    uint32_t hash;
    int found;

    struct capacity *capacity;

    struct ps_property *property_root;

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

        struct ps_property *pr = ps->property_root;
        while(pr) {
            struct ps_property_dim *pd = pr->property_dim_root;
            while(pd) {
                freez(pd->name);
                freez(pd->filename);
                struct ps_property_dim *d = pd;
                pd = pd->next;
                freez(d);
            }
            if(pr->st) rrdset_is_obsolete(pr->st);
            freez(pr->name);
            freez(pr->title);
            freez(pr->units);
            struct ps_property *p = pr;
            pr = pr->next;
            freez(p);
        }

        if(ps == power_supply_root) {
            power_supply_root = ps->next;
        }
        else {
            struct power_supply *last;
            for(last = power_supply_root; last && last->next != ps; last = last->next);
            last->next = ps->next;
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

            // allocate memory for power supply and initialize it
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

                // allocate memory and initialize structures for every property and every file found
                size_t pr_idx, pd_idx;
                size_t prev_idx = 3; // there is no property with this index

                for(pr_idx = 0; pr_idx < 3; pr_idx++) {
                    struct ps_property *pr;

                    for(pd_idx = pr_idx * 5; pd_idx < pr_idx * 5 + 5; pd_idx++) {

                        // check if file exists
                        char filename[FILENAME_MAX + 1];
                        snprintfz(filename, FILENAME_MAX, "%s/%s/%s_%s", dirname, de->d_name,
                                  ps_property_names[pr_idx], ps_property_dim_names[pd_idx]);
                        if (stat(filename, &stbuf) == 0) {

                            // add chart
                            if(prev_idx != pr_idx) {
                                pr = callocz(sizeof(struct ps_property), 1);
                                pr->name = strdupz(ps_property_names[pr_idx]);
                                pr->title = strdupz(ps_property_titles[pr_idx]);
                                pr->units = strdupz(ps_property_units[pr_idx]);
                                prev_idx = pr_idx;
                                pr->next = ps->property_root;
                                ps->property_root = pr;
                            }

                            // add dimension
                            struct ps_property_dim *pd;
                            pd= callocz(sizeof(struct ps_property_dim), 1);
                            pd->name = strdupz(ps_property_dim_names[pd_idx]);
                            pd->filename = strdupz(filename);
                            pd->next = pr->property_dim_root;
                            pr->property_dim_root = pd;
                        }
                    }
                }
            }

            // TODO: keep files open
            if(ps->capacity) {
                if(unlikely(read_single_number_file(ps->capacity->filename, &ps->capacity->value))) {
                    error("Cannot read file '%s'", ps->capacity->filename);
                    power_supply_free(ps);
                }
            }

            int read_error = 0;
            struct ps_property *pr;
            for(pr = ps->property_root; pr && !read_error; pr = pr->next) {
                struct ps_property_dim *pd;
                for(pd = pr->property_dim_root; pd; pd = pd->next) {
                    if(unlikely(read_single_number_file(pd->filename, &pd->value))) {
                        error("Cannot read file '%s'", pd->filename);
                        read_error = 1;
                        power_supply_free(ps);
                        break;
                    }
                }
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
                ps->capacity->st = rrdset_create_localhost(
                        "powersupply_capacity"
                        , ps->name
                        , NULL
                        , ps->name
                        , "powersupply.capacity"
                        , "Battery capacity"
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
        }

        struct ps_property *pr;
        for(pr = ps->property_root; pr; pr = pr->next) {
            if(unlikely(!pr->st)) {
                char id[50 + 1], context[50 + 1];
                snprintfz(id, RRD_ID_LENGTH_MAX, "powersupply_%s", pr->name);
                snprintfz(context, RRD_ID_LENGTH_MAX, "powersupply.%s", pr->name);

                pr->st = rrdset_create_localhost(
                        id
                        , ps->name
                        , NULL
                        , ps->name
                        , context
                        , pr->title
                        , pr->units
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_POWER_SUPPLY_NAME
                        , NETDATA_CHART_PRIO_POWER_SUPPLY_CAPACITY
                        , update_every
                        , RRDSET_TYPE_LINE
                );
            }
            else
                rrdset_next(pr->st);

            struct ps_property_dim *pd;
            for(pd = pr->property_dim_root; pd; pd = pd->next) {
                if(!pd->rd) pd->rd = rrddim_add(pr->st, pd->name, NULL, 1, 1000000, RRD_ALGORITHM_ABSOLUTE);
                rrddim_set_by_pointer(pr->st, pd->rd, pd->value);
            }

            rrdset_done(pr->st);
        }

        ps->found = 0;
        ps = ps->next;
    }

    return 0;
}
