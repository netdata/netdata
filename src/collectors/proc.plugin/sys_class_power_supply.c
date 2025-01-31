// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_POWER_SUPPLY_NAME "/sys/class/power_supply"
#define PROP_VALUE_LENGTH_MAX 30

const char *ps_property_names[]  = {"capacity", "power", "charge", "energy", "voltage"};
const char *ps_property_titles[] = {"Battery capacity", "Battery power", "Battery charge", "Battery energy", "Power supply voltage"};
const char *ps_property_units[]  = {"percentage", "W", "Ah", "Wh", "V"};

const long ps_property_priorities[] = {
    NETDATA_CHART_PRIO_POWER_SUPPLY_CAPACITY,
    NETDATA_CHART_PRIO_POWER_SUPPLY_POWER,
    NETDATA_CHART_PRIO_POWER_SUPPLY_CHARGE,
    NETDATA_CHART_PRIO_POWER_SUPPLY_ENERGY,
    NETDATA_CHART_PRIO_POWER_SUPPLY_VOLTAGE
};

enum {
    PS_PROP_CAPACITY,
    PS_PROP_POWER,
    PS_PROP_CHARGE,
    PS_PROP_ENERGY,
    PS_PROP_VOLTAGE,
    PS_PROP_END
};

const unsigned long ps_property_divisors[] = {1, 1000000, 1000000, 1000000, 1000000 };

const char *ps_property_dim_names[] = {
    "", NULL, // property name will be used instead for capacity
    "now", NULL,
    "empty_design", "empty", "now", "full", "full_design", NULL,
    "empty_design", "empty", "now", "full", "full_design", NULL,
    "min_design", "min", "now", "max", "max_design", NULL};

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

    long priority;
    unsigned long divisor;

    RRDSET *st;

    struct ps_property_dim *property_dim_root;

    struct ps_property *next;
};

struct power_supply {
    char *name;
    uint32_t hash;
    int found;

    struct simple_property *capacity, *power;

    struct ps_property *property_root;

    struct power_supply *next;
};

static struct power_supply *power_supply_root = NULL;
static int files_num = 0;

void power_supply_free(struct power_supply *ps) {
    if(likely(ps)) {
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
            if(likely(pr->st)) rrdset_is_obsolete___safe_from_collector_thread(pr->st);
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
    static int do_property[PS_PROP_END] = {-1};
    static int keep_fds_open = CONFIG_BOOLEAN_NO, keep_fds_open_config = -1;
    static const char *dirname = NULL;

    if(unlikely(do_property[PS_PROP_CAPACITY] == -1)) {
        do_property[PS_PROP_CAPACITY] = config_get_boolean("plugin:proc:/sys/class/power_supply", "battery capacity", CONFIG_BOOLEAN_YES);
        do_property[PS_PROP_POWER] = config_get_boolean("plugin:proc:/sys/class/power_supply", "battery power", CONFIG_BOOLEAN_YES);
        do_property[PS_PROP_CHARGE] = config_get_boolean("plugin:proc:/sys/class/power_supply", "battery charge", CONFIG_BOOLEAN_NO);
        do_property[PS_PROP_ENERGY] = config_get_boolean("plugin:proc:/sys/class/power_supply", "battery energy", CONFIG_BOOLEAN_NO);
        do_property[PS_PROP_VOLTAGE] = config_get_boolean("plugin:proc:/sys/class/power_supply", "power supply voltage", CONFIG_BOOLEAN_NO);

        keep_fds_open_config = config_get_boolean_ondemand("plugin:proc:/sys/class/power_supply", "keep files open", CONFIG_BOOLEAN_AUTO);

        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/power_supply");
        dirname = config_get("plugin:proc:/sys/class/power_supply", "directory to monitor", filename);
    }

    DIR *dir = opendir(dirname);
    if(unlikely(!dir)) {
        collector_error("Cannot read directory '%s'", dirname);
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

                // allocate memory and initialize structures for every property and file found
                size_t pr_idx;
                size_t pd_idx = 0;
                size_t prev_idx = PS_PROP_END; // there is no property with this index

                for(pr_idx = 0; pr_idx < PS_PROP_END; pr_idx++) {
                    if(unlikely(do_property[pr_idx] != CONFIG_BOOLEAN_NO)) {
                        int pd_cur_idx = 0;
                        struct ps_property *pr = NULL;
                        int min_value_found = 0, max_value_found = 0;
                        while (ps_property_dim_names[pd_idx] != NULL) {
                            char filename[FILENAME_MAX + 1];
                            if (likely(strnlen(ps_property_dim_names[pd_idx], 1) > 0)) {
                                snprintfz(filename, FILENAME_MAX, "%s/%s/%s_%s", dirname, de->d_name,
                                          ps_property_names[pr_idx], ps_property_dim_names[pd_idx]);
                            }
                            else {
                                snprintfz(filename, FILENAME_MAX, "%s/%s/%s", dirname, de->d_name,
                                          ps_property_names[pr_idx]);
                            }
                            if (stat(filename, &stbuf) == 0) {
                                 if(unlikely(pd_cur_idx == 1))
                                     min_value_found = 1;
                                 if(unlikely(pd_cur_idx == 3))
                                     max_value_found = 1;

                                // add chart
                                if(unlikely(prev_idx != pr_idx)) {
                                    pr = callocz(sizeof(struct ps_property), 1);
                                    pr->name = strdupz(ps_property_names[pr_idx]);
                                    pr->title = strdupz(ps_property_titles[pr_idx]);
                                    pr->units = strdupz(ps_property_units[pr_idx]);
                                    pr->priority = ps_property_priorities[pr_idx];
                                    pr->divisor = ps_property_divisors[pr_idx];
                                    prev_idx = pr_idx;
                                    pr->next = ps->property_root;
                                    ps->property_root = pr;
                                }

                                // add dimension
                                struct ps_property_dim *pd;
                                pd= callocz(sizeof(struct ps_property_dim), 1);
                                if (likely(strnlen(ps_property_dim_names[pd_idx], 1) > 0)) {
                                    pd->name = strdupz(ps_property_dim_names[pd_idx]);
                                }
                                else {
                                    pd->name = strdupz(ps_property_names[pr_idx]);
                                }
                                pd->filename = strdupz(filename);
                                pd->fd = -1;
                                files_num++;
                                pd->next = pr->property_dim_root;
                                pr->property_dim_root = pd;
                            }
                            pd_idx += 1;
                            pd_cur_idx += 1;
                        }
                        pd_idx += 1;

                        // create a zero empty/min dimension
                        if(unlikely(max_value_found && !min_value_found)) {
                            struct ps_property_dim *pd;
                            pd= callocz(sizeof(struct ps_property_dim), 1);
                            pd->name = strdupz(ps_property_dim_names[pd_idx - pd_cur_idx]);
                            pd->always_zero = 1;
                            pd->next = pr->property_dim_root;
                            pr->property_dim_root = pd;
                        }
                    }
                }
            }

            // read property files
            int read_error = 0;
            struct ps_property *pr;
            if (likely(ps))
            {
                for(pr = ps->property_root; pr && !read_error; pr = pr->next) {
                    struct ps_property_dim *pd;
                    for(pd = pr->property_dim_root; pd; pd = pd->next) {
                        if(likely(!pd->always_zero)) {
                            char buffer[PROP_VALUE_LENGTH_MAX + 1];

                            if(unlikely(pd->fd == -1)) {
                                pd->fd = open(pd->filename, O_RDONLY | O_CLOEXEC, 0666);
                                if(unlikely(pd->fd == -1)) {
                                    collector_error("Cannot open file '%s'", pd->filename);
                                    read_error = 1;
                                    power_supply_free(ps);
                                    break;
                                }
                            }

                            ssize_t r = read(pd->fd, buffer, PROP_VALUE_LENGTH_MAX);
                            if(unlikely(r < 1)) {
                                collector_error("Cannot read file '%s'", pd->filename);
                                read_error = 1;
                                power_supply_free(ps);
                                break;
                            }
                            buffer[r] = '\0';
                            pd->value = str2ull(buffer, NULL);

                            if(unlikely(!keep_fds_open)) {
                                close(pd->fd);
                                pd->fd = -1;
                            }
                            else if(unlikely(lseek(pd->fd, 0, SEEK_SET) == -1)) {
                                collector_error("Cannot seek in file '%s'", pd->filename);
                                close(pd->fd);
                                pd->fd = -1;
                            }
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
                        , pr->name
                        , context
                        , pr->title
                        , pr->units
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_POWER_SUPPLY_NAME
                        , pr->priority
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdlabels_add(pr->st->rrdlabels, "device", ps->name, RRDLABEL_SRC_AUTO);
            }

            struct ps_property_dim *pd;
            for(pd = pr->property_dim_root; pd; pd = pd->next) {
                if(unlikely(!pd->rd)) pd->rd = rrddim_add(pr->st, pd->name, NULL, 1, pr->divisor, RRD_ALGORITHM_ABSOLUTE);
                rrddim_set_by_pointer(pr->st, pd->rd, pd->value);
            }

            rrdset_done(pr->st);
        }

        ps->found = 0;
        ps = ps->next;
    }

    return 0;
}
