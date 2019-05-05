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
    int fd;

    RRDDIM *rd;
    unsigned long long value;
    int always_zero;

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

    struct ps_property *property_root;

    struct power_supply *next;
};

static struct power_supply *power_supply_root = NULL;
static int files_num = 0;

void power_supply_free(struct power_supply *ps) {
    if(likely(ps)) {

        // free capacity structure
        if(likely(ps->capacity)) {
            if(likely(ps->capacity->st)) rrdset_is_obsolete(ps->capacity->st);
            freez(ps->capacity->filename);
            if(likely(ps->capacity->fd != -1)) close(ps->capacity->fd);
            files_num--;
            freez(ps->capacity);
        }
        freez(ps->name);

        struct ps_property *pr = ps->property_root;
        while(likely(pr)) {

            // free dimensions
            struct ps_property_dim *pd = pr->property_dim_root;
            while(likely(pd)) {
                freez(pd->name);
                freez(pd->filename);
                if(likely(pd->fd != -1)) close(pd->fd);
                files_num--;
                struct ps_property_dim *d = pd;
                pd = pd->next;
                freez(d);
            }

            // free properties
            if(likely(pr->st)) rrdset_is_obsolete(pr->st);
            freez(pr->name);
            freez(pr->title);
            freez(pr->units);
            struct ps_property *p = pr;
            pr = pr->next;
            freez(p);
        }

        // remove power supply from linked list
        if(likely(ps == power_supply_root)) {
            power_supply_root = ps->next;
        }
        else {
            struct power_supply *last;
            for(last = power_supply_root; last && last->next != ps; last = last->next);
            if(likely(last)) last->next = ps->next;
        }

        freez(ps);
    }
}

int do_sys_class_power_supply(int update_every, usec_t dt) {
    (void)dt;
    static int do_capacity = -1, do_property[3] = {-1};
    static int keep_fds_open = CONFIG_BOOLEAN_NO, keep_fds_open_config = -1;
    static char *dirname = NULL;

    if(unlikely(do_capacity == -1)) {
        do_capacity    = config_get_boolean("plugin:proc:/sys/class/power_supply", "battery capacity", CONFIG_BOOLEAN_YES);
        do_property[0] = config_get_boolean("plugin:proc:/sys/class/power_supply", "battery charge", CONFIG_BOOLEAN_NO);
        do_property[1] = config_get_boolean("plugin:proc:/sys/class/power_supply", "battery energy", CONFIG_BOOLEAN_NO);
        do_property[2] = config_get_boolean("plugin:proc:/sys/class/power_supply", "power supply voltage", CONFIG_BOOLEAN_NO);

        keep_fds_open_config = config_get_boolean_ondemand("plugin:proc:/sys/class/power_supply", "keep files open", CONFIG_BOOLEAN_AUTO);

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
        if(likely(de->d_type == DT_DIR
            && (
                (de->d_name[0] == '.' && de->d_name[1] == '\0')
                || (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0')
                )))
            continue;

        if(likely(de->d_type == DT_LNK || de->d_type == DT_DIR)) {
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
                        ps->capacity->fd = -1;
                        files_num++;
                    }
                }

                // allocate memory and initialize structures for every property and file found
                size_t pr_idx, pd_idx;
                size_t prev_idx = 3; // there is no property with this index

                for(pr_idx = 0; pr_idx < 3; pr_idx++) {
                    if(unlikely(do_property[pr_idx] != CONFIG_BOOLEAN_NO)) {
                        struct ps_property *pr = NULL;
                        int min_value_found = 0, max_value_found = 0;

                        for(pd_idx = pr_idx * 5; pd_idx < pr_idx * 5 + 5; pd_idx++) {

                            // check if file exists
                            char filename[FILENAME_MAX + 1];
                            snprintfz(filename, FILENAME_MAX, "%s/%s/%s_%s", dirname, de->d_name,
                                      ps_property_names[pr_idx], ps_property_dim_names[pd_idx]);
                            if (stat(filename, &stbuf) == 0) {

                                if(unlikely(pd_idx == pr_idx * 5 + 1))
                                    min_value_found = 1;
                                if(unlikely(pd_idx == pr_idx * 5 + 3))
                                    max_value_found = 1;

                                // add chart
                                if(unlikely(prev_idx != pr_idx)) {
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
                                pd->fd = -1;
                                files_num++;
                                pd->next = pr->property_dim_root;
                                pr->property_dim_root = pd;
                            }
                        }

                        // create a zero empty/min dimension
                        if(unlikely(max_value_found && !min_value_found)) {
                            struct ps_property_dim *pd;
                            pd= callocz(sizeof(struct ps_property_dim), 1);
                            pd->name = strdupz(ps_property_dim_names[pr_idx * 5 + 1]);
                            pd->always_zero = 1;
                            pd->next = pr->property_dim_root;
                            pr->property_dim_root = pd;
                        }
                    }
                }
            }

            // read capacity file
            if(likely(ps->capacity)) {
                char buffer[30 + 1];

                if(unlikely(ps->capacity->fd == -1)) {
                    ps->capacity->fd = open(ps->capacity->filename, O_RDONLY, 0666);
                    if(unlikely(ps->capacity->fd == -1)) {
                        error("Cannot open file '%s'", ps->capacity->filename);
                        power_supply_free(ps);
                    }
                }

                ssize_t r = read(ps->capacity->fd, buffer, 30);
                if(unlikely(r < 1)) {
                    error("Cannot read file '%s'", ps->capacity->filename);
                    power_supply_free(ps);
                }
                else {
                    buffer[r] = '\0';
                    ps->capacity->value = str2ull(buffer);
                }

                if(unlikely(!keep_fds_open)) {
                    close(ps->capacity->fd);
                    ps->capacity->fd = -1;
                }
                else if(unlikely(lseek(ps->capacity->fd, 0, SEEK_SET) == -1)) {
                    error("Cannot seek in file '%s'", ps->capacity->filename);
                    close(ps->capacity->fd);
                    ps->capacity->fd = -1;
                }
            }

            // read property files
            int read_error = 0;
            struct ps_property *pr;
            for(pr = ps->property_root; pr && !read_error; pr = pr->next) {
                struct ps_property_dim *pd;
                for(pd = pr->property_dim_root; pd; pd = pd->next) {
                    if(likely(!pd->always_zero)) {
                        char buffer[30 + 1];

                        if(unlikely(pd->fd == -1)) {
                            pd->fd = open(pd->filename, O_RDONLY, 0666);
                            if(unlikely(pd->fd == -1)) {
                                error("Cannot open file '%s'", pd->filename);
                                read_error = 1;
                                power_supply_free(ps);
                                break;
                            }
                        }

                        ssize_t r = read(pd->fd, buffer, 30);
                        if(unlikely(r < 1)) {
                            error("Cannot read file '%s'", pd->filename);
                            read_error = 1;
                            power_supply_free(ps);
                            break;
                        }
                        buffer[r] = '\0';
                        pd->value = str2ull(buffer);

                        if(unlikely(!keep_fds_open)) {
                            close(pd->fd);
                            pd->fd = -1;
                        }
                        else if(unlikely(lseek(pd->fd, 0, SEEK_SET) == -1)) {
                            error("Cannot seek in file '%s'", pd->filename);
                            close(pd->fd);
                            pd->fd = -1;
                        }
                    }
                }
            }
        }
    }

    closedir(dir);

    keep_fds_open = keep_fds_open_config;
    if(likely(keep_fds_open_config == CONFIG_BOOLEAN_AUTO)) {
        if(unlikely(files_num > 32))
            keep_fds_open = CONFIG_BOOLEAN_NO;
        else
            keep_fds_open = CONFIG_BOOLEAN_YES;
    }

    // --------------------------------------------------------------------

    struct power_supply *ps = power_supply_root;
    while(unlikely(ps)) {
        if(unlikely(!ps->found)) {
            struct power_supply *f = ps;
            ps = ps->next;
            power_supply_free(f);
            continue;
        }

        if(likely(ps->capacity)) {
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

            if(unlikely(!ps->capacity->rd)) ps->capacity->rd = rrddim_add(ps->capacity->st, "capacity", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrddim_set_by_pointer(ps->capacity->st, ps->capacity->rd, ps->capacity->value);

            rrdset_done(ps->capacity->st);
        }

        struct ps_property *pr;
        for(pr = ps->property_root; pr; pr = pr->next) {
            if(unlikely(!pr->st)) {
                char id[RRD_ID_LENGTH_MAX + 1], context[RRD_ID_LENGTH_MAX + 1];
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
                if(unlikely(!pd->rd)) pd->rd = rrddim_add(pr->st, pd->name, NULL, 1, 1000000, RRD_ALGORITHM_ABSOLUTE);
                rrddim_set_by_pointer(pr->st, pd->rd, pd->value);
            }

            rrdset_done(pr->st);
        }

        ps->found = 0;
        ps = ps->next;
    }

    return 0;
}
