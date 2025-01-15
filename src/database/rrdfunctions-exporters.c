// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdfunctions-internals.h"
#include "rrdfunctions-exporters.h"

void stream_sender_send_rrdset_functions(RRDSET *st, BUFFER *wb) {
    if(!st->functions_view)
        return;

    struct rrd_host_function *t;
    dfe_start_read(st->functions_view, t) {
        if(t->options & RRD_FUNCTION_DYNCFG) continue;

        buffer_sprintf(wb
                       , PLUGINSD_KEYWORD_FUNCTION " \"%s\" %d \"%s\" \"%s\" "HTTP_ACCESS_FORMAT" %d %"PRIu32"\n"
                       , t_dfe.name
                       , t->timeout
                       , string2str(t->help)
                       , string2str(t->tags)
                       , (HTTP_ACCESS_FORMAT_CAST)t->access
                       , t->priority
                       , t->version
        );
    }
    dfe_done(t);
}

void stream_sender_send_global_rrdhost_functions(RRDHOST *host, BUFFER *wb, bool dyncfg) {
    rrdhost_flag_clear(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

    size_t configs = 0;

    struct rrd_host_function *tmp;
    dfe_start_read(host->functions, tmp) {
        if(tmp->options & RRD_FUNCTION_LOCAL) continue;
        if(tmp->options & RRD_FUNCTION_DYNCFG) {
            // we should not send dyncfg to this parent
            configs++;
            continue;
        }

        buffer_sprintf(wb
                       , PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"%s\" "HTTP_ACCESS_FORMAT" %d %"PRIu32"\n"
                       , tmp_dfe.name
                       , tmp->timeout
                       , string2str(tmp->help)
                       , string2str(tmp->tags)
                       , (HTTP_ACCESS_FORMAT_CAST)tmp->access
                       , tmp->priority
                       , tmp->version
        );
    }
    dfe_done(tmp);

    if(dyncfg && configs)
        dyncfg_add_streaming(wb);
}

static void functions2json(DICTIONARY *functions, BUFFER *wb) {
    struct rrd_host_function *t;
    dfe_start_read(functions, t) {
        if (!rrd_collector_running(t->collector)) continue;
        if(t->options & (RRD_FUNCTION_DYNCFG| RRD_FUNCTION_RESTRICTED)) continue;

        buffer_json_member_add_object(wb, t_dfe.name);
        {
            buffer_json_member_add_string_or_empty(wb, "help", string2str(t->help));
            buffer_json_member_add_int64(wb, "timeout", (int64_t) t->timeout);
            buffer_json_member_add_uint64(wb, "version", (uint64_t) t->version);

            char options[65];
            snprintfz(
                options, 64
                , "%s%s"
                , (t->options & RRD_FUNCTION_LOCAL) ? "LOCAL " : ""
                , (t->options & RRD_FUNCTION_GLOBAL) ? "GLOBAL" : ""
            );

            buffer_json_member_add_string_or_empty(wb, "options", options);
            buffer_json_member_add_string_or_empty(wb, "tags", string2str(t->tags));
            http_access2buffer_json_array(wb, "access", t->access);
            buffer_json_member_add_uint64(wb, "priority", t->priority);
        }
        buffer_json_object_close(wb);
    }
    dfe_done(t);
}

void chart_functions2json(RRDSET *st, BUFFER *wb) {
    if(!st || !st->functions_view) return;

    functions2json(st->functions_view, wb);
}

void host_functions2json(RRDHOST *host, BUFFER *wb) {
    if(!host || !host->functions) return;

    buffer_json_member_add_object(wb, "functions");

    struct rrd_host_function *t;
    dfe_start_read(host->functions, t) {
        if(!rrd_collector_running(t->collector)) continue;
        if(t->options & (RRD_FUNCTION_DYNCFG| RRD_FUNCTION_RESTRICTED)) continue;

        buffer_json_member_add_object(wb, t_dfe.name);
        {
            buffer_json_member_add_string(wb, "help", string2str(t->help));
            buffer_json_member_add_int64(wb, "timeout", t->timeout);
            buffer_json_member_add_uint64(wb, "version", (uint64_t) t->version);
            buffer_json_member_add_array(wb, "options");
            {
                if (t->options & RRD_FUNCTION_GLOBAL)
                    buffer_json_add_array_item_string(wb, "GLOBAL");
                if (t->options & RRD_FUNCTION_LOCAL)
                    buffer_json_add_array_item_string(wb, "LOCAL");
            }
            buffer_json_array_close(wb);
            buffer_json_member_add_string(wb, "tags", string2str(t->tags));
            http_access2buffer_json_array(wb, "access", t->access);
            buffer_json_member_add_uint64(wb, "priority", t->priority);
        }
        buffer_json_object_close(wb);
    }
    dfe_done(t);

    buffer_json_object_close(wb);
}

void chart_functions_to_dict(DICTIONARY *rrdset_functions_view, DICTIONARY *dst, void *value, size_t value_size) {
    if(!rrdset_functions_view || !dst) return;

    struct rrd_host_function *t;
    dfe_start_read(rrdset_functions_view, t) {
        if(!rrd_collector_running(t->collector)) continue;
        if(t->options & (RRD_FUNCTION_DYNCFG| RRD_FUNCTION_RESTRICTED)) continue;

        dictionary_set(dst, t_dfe.name, value, value_size);
    }
    dfe_done(t);
}

void host_functions_to_dict(RRDHOST *host, DICTIONARY *dst, void *value, size_t value_size,
                            STRING **help, STRING **tags, HTTP_ACCESS *access, int *priority, uint32_t *version) {
    if(!host || !host->functions || !dictionary_entries(host->functions) || !dst) return;

    struct rrd_host_function *t;
    dfe_start_read(host->functions, t) {
        if(!rrd_collector_running(t->collector)) continue;
        if(t->options & (RRD_FUNCTION_DYNCFG| RRD_FUNCTION_RESTRICTED)) continue;

        if(help)
            *help = t->help;

        if(tags)
            *tags = t->tags;

        if(access)
            *access = t->access;

        if(priority)
            *priority = t->priority;

        if(version)
            *version = t->version;

        char key[UINT64_MAX_LENGTH + sizeof(RRDFUNCTIONS_VERSION_SEPARATOR) + strlen(t_dfe.name)];
        snprintfz(key, sizeof(key), "%"PRIu32 RRDFUNCTIONS_VERSION_SEPARATOR "%s",
                  t->version, t_dfe.name);

        dictionary_set(dst, key, value, value_size);
    }
    dfe_done(t);
}
