// SPDX-License-Identifier: GPL-3.0-or-later

#include "charts2json.h"

// generate JSON for the /api/v1/charts API call

const char* get_release_channel() {
    static int use_stable = -1;

    if (use_stable == -1) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/.environment", netdata_configured_user_config_dir);
        procfile *ff = procfile_open(filename, "=", PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
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
            use_stable = strchr(NETDATA_VERSION, '-') ? 0 : 1;
    }
    return (use_stable)?"stable":"nightly";
}

void charts2json(RRDHOST *host, BUFFER *wb) {
    static const char *custom_dashboard_info_js_filename = NULL;
    size_t c = 0, dimensions = 0, memory = 0, alarms = 0;
    RRDSET *st;

    time_t now = now_realtime_sec();

    if(unlikely(!custom_dashboard_info_js_filename))
        custom_dashboard_info_js_filename = inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "custom dashboard_info.js", "");

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(host));
    buffer_json_member_add_string(wb, "version", rrdhost_program_version(host));
    buffer_json_member_add_string(wb, "release_channel", get_release_channel());
    buffer_json_member_add_string(wb, "os", rrdhost_os(host));
    buffer_json_member_add_string(wb, "timezone", rrdhost_timezone(host));
    buffer_json_member_add_int64(wb, "update_every", host->rrd_update_every);
    buffer_json_member_add_int64(wb, "history", host->rrd_history_entries);
    buffer_json_member_add_string(wb, "memory_mode", rrd_memory_mode_name(host->rrd_memory_mode));
    buffer_json_member_add_string(wb, "custom_info", custom_dashboard_info_js_filename);

    buffer_json_member_add_object(wb, "charts");
    rrdset_foreach_read(st, host) {
        if (rrdset_is_available_for_viewers(st)) {

            buffer_json_member_add_object(wb, rrdset_id(st));
            rrdset2json(st, wb, &dimensions, &memory);
            buffer_json_object_close(wb);
            st->last_accessed_time_s = now;
            c++;
        }
    }
    rrdset_foreach_done(st);
    buffer_json_object_close(wb);

    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_read(host, rc) {
        if(rc->rrdset)
            alarms++;
    }
    foreach_rrdcalc_in_rrdhost_done(rc);

    buffer_json_member_add_int64(wb, "charts_count", (int64_t) c);
    buffer_json_member_add_int64(wb, "dimensions_count", (int64_t) dimensions);
    buffer_json_member_add_int64(wb, "alarms_count", (int64_t)alarms);
    buffer_json_member_add_int64(wb, "rrd_memory_bytes", (int64_t)memory);
    buffer_json_member_add_int64(wb, "hosts_count", (int64_t) rrdhost_hosts_available());

    buffer_json_member_add_array(wb, "hosts");
    {
        rrd_rdlock();
        RRDHOST *h;
        rrdhost_foreach_read(h) {
            if(!rrdhost_should_be_cleaned_up(h, host, now) /*&& !rrdhost_flag_check(h, RRDHOST_FLAG_ARCHIVED) */) {
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(h));
                buffer_json_object_close(wb);
            }
        }
        rrd_rdunlock();
    }
    buffer_json_array_close(wb);

    buffer_json_finalize(wb);
}
