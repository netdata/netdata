// SPDX-License-Identifier: GPL-3.0-or-later

#include "charts2json.h"

// generate JSON for the /api/v1/charts API call

const char* get_release_channel() {
    static int use_stable = -1;

    if (use_stable == -1) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/.environment", netdata_configured_user_config_dir);
        procfile *ff = procfile_open(filename, "=", PROCFILE_FLAG_DEFAULT);
        if (ff) {
            procfile_set_quotes(ff, "'\"");
            ff = procfile_readall(ff);
            if (ff) {
                unsigned int i;
                for (i = 0; i < procfile_lines(ff); i++) {
                    if (!procfile_linewords(ff, i))
                        continue;
                    if (!strcmp(procfile_lineword(ff, i, 0), "RELEASE_CHANNEL")) {
                        if (!strcmp(procfile_lineword(ff, i, 1), "stable"))
                            use_stable = 1;
                        else if (!strcmp(procfile_lineword(ff, i, 1), "nightly"))
                            use_stable = 0;
                        break;
                    }
                }
                procfile_close(ff);
            }
        }
        if (use_stable == -1)
            use_stable = strchr(program_version, '-') ? 0 : 1;
    }
    return (use_stable)?"stable":"nightly";
}

void charts2json(RRDHOST *host, BUFFER *wb, int skip_volatile, int show_archived) {
    static char *custom_dashboard_info_js_filename = NULL;
    size_t c, dimensions = 0, memory = 0, alarms = 0;
    RRDSET *st;

    time_t now = now_realtime_sec();

    if(unlikely(!custom_dashboard_info_js_filename))
        custom_dashboard_info_js_filename = config_get(CONFIG_SECTION_WEB, "custom dashboard_info.js", "");

    buffer_sprintf(wb, "{\n"
                       "\t\"hostname\": \"%s\""
                       ",\n\t\"version\": \"%s\""
                       ",\n\t\"release_channel\": \"%s\""
                       ",\n\t\"os\": \"%s\""
                       ",\n\t\"timezone\": \"%s\""
                       ",\n\t\"update_every\": %d"
                       ",\n\t\"history\": %ld"
                       ",\n\t\"memory_mode\": \"%s\""
                       ",\n\t\"custom_info\": \"%s\""
                       ",\n\t\"charts\": {"
                   , rrdhost_hostname(host)
                   , host->program_version
                   , get_release_channel()
                   , rrdhost_os(host)
                   , host->timezone
                   , host->rrd_update_every
                   , host->rrd_history_entries
                   , rrd_memory_mode_name(host->rrd_memory_mode)
                   , custom_dashboard_info_js_filename
    );

    c = 0;
    rrdhost_rdlock(host);
    rrdset_foreach_read(st, host) {
        if ((!show_archived && rrdset_is_available_for_viewers(st)) || (show_archived && rrdset_is_archived(st))) {
            if(c) buffer_strcat(wb, ",");
            buffer_strcat(wb, "\n\t\t\"");
            buffer_strcat(wb, rrdset_id(st));
            buffer_strcat(wb, "\": ");
            rrdset2json(st, wb, &dimensions, &memory, skip_volatile);

            c++;
            st->last_accessed_time = now;
        }
    }

    RRDCALC *rc;
    for(rc = host->alarms; rc ; rc = rc->next) {
        if(rc->rrdset)
            alarms++;
    }
    rrdhost_unlock(host);

    buffer_sprintf(wb
                   , "\n\t}"
                     ",\n\t\"charts_count\": %zu"
                     ",\n\t\"dimensions_count\": %zu"
                     ",\n\t\"alarms_count\": %zu"
                     ",\n\t\"rrd_memory_bytes\": %zu"
                     ",\n\t\"hosts_count\": %zu"
                     ",\n\t\"hosts\": ["
                   , c
                   , dimensions
                   , alarms
                   , memory
                   , rrd_hosts_available
    );

    if(unlikely(rrd_hosts_available > 1)) {
        rrd_rdlock();

        size_t found = 0;
        RRDHOST *h;
        rrdhost_foreach_read(h) {
            if(!rrdhost_should_be_removed(h, host, now) && !rrdhost_flag_check(h, RRDHOST_FLAG_ARCHIVED)) {
                buffer_sprintf(wb
                               , "%s\n\t\t{"
                                 "\n\t\t\t\"hostname\": \"%s\""
                                 "\n\t\t}"
                               , (found > 0) ? "," : ""
                               , rrdhost_hostname(h)
                );

                found++;
            }
        }

        rrd_unlock();
    }
    else {
        buffer_sprintf(wb
                       , "\n\t\t{"
                         "\n\t\t\t\"hostname\": \"%s\""
                         "\n\t\t}"
                       , rrdhost_hostname(host)
        );
    }

    buffer_sprintf(wb, "\n\t]\n}\n");
}

// generate collectors list for the api/v1/info call

struct collector {
    const char *plugin;
    const char *module;
};

struct array_printer {
    int c;
    BUFFER *wb;
};

static int print_collector_callback(const char *name, void *entry, void *data) {
    (void)name;

    struct array_printer *ap = (struct array_printer *)data;
    BUFFER *wb = ap->wb;
    struct collector *col=(struct collector *) entry;
    if(ap->c) buffer_strcat(wb, ",");
    buffer_strcat(wb, "\n\t\t{\n\t\t\t\"plugin\": \"");
    buffer_strcat(wb, col->plugin);
    buffer_strcat(wb, "\",\n\t\t\t\"module\": \"");
    buffer_strcat(wb, col->module);
    buffer_strcat(wb, "\"\n\t\t}");
    (ap->c)++;
    return 0;
}

void chartcollectors2json(RRDHOST *host, BUFFER *wb) {
    DICTIONARY *dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
    RRDSET *st;
    char name[500];

    time_t now = now_realtime_sec();
    rrdhost_rdlock(host);
    rrdset_foreach_read(st, host) {
        if (rrdset_is_available_for_viewers(st)) {
            struct collector col = {
                    .plugin = rrdset_plugin_name(st),
                    .module = rrdset_module_name(st)
            };
            sprintf(name, "%s:%s", col.plugin, col.module);
            dictionary_set(dict, name, &col, sizeof(struct collector));
            st->last_accessed_time = now;
        }
    }
    rrdhost_unlock(host);
    struct array_printer ap = {
            .c = 0,
            .wb = wb
    };
    dictionary_walkthrough_read(dict, print_collector_callback, &ap);
    dictionary_destroy(dict);
}
