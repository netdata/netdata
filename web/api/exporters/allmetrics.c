// SPDX-License-Identifier: GPL-3.0-or-later

#include "allmetrics.h"

struct prometheus_output_options {
    char *name;
    PROMETHEUS_OUTPUT_OPTIONS flag;
} prometheus_output_flags_root[] = {
    { "help",       PROMETHEUS_OUTPUT_HELP       },
    { "types",      PROMETHEUS_OUTPUT_TYPES      },
    { "names",      PROMETHEUS_OUTPUT_NAMES      },
    { "timestamps", PROMETHEUS_OUTPUT_TIMESTAMPS },
    { "variables",  PROMETHEUS_OUTPUT_VARIABLES  },
    { "oldunits",   PROMETHEUS_OUTPUT_OLDUNITS   },
    { "hideunits",  PROMETHEUS_OUTPUT_HIDEUNITS  },
    // terminator
    { NULL, PROMETHEUS_OUTPUT_NONE },
};

/**
 * @brief Update simple pattern for chart filtering if there is a new filter
 * 
 * @param filter_p filter structure to update
 * @param filter_string a new filter to create
 * @return Returns 1 if the filter has changed, 0 otherwise 
 */
int lock_and_update_allmetrics_filter(struct allmetrics_filter **filter_p, const char *filter_string)
{
    struct allmetrics_filter *filter = *filter_p;
    int filter_changed = 0;

    if (!filter) {
        filter = callocz(1, sizeof(struct allmetrics_filter));
        *filter_p = filter;

        if (uv_mutex_init(&filter->filter_mutex)) {
            freez(filter);
            fatal("Cannot initialize mutex for allmetrics filter"); // TODO: can we do without fatal()?
        }

        if (uv_cond_init(&filter->filter_cond)) {
            freez(filter);
            fatal("Cannot initialize conditional variable for allmetrics filter"); // TODO: can we do without fatal()?
        }
    }
    
    uv_mutex_lock(&filter->filter_mutex);
    filter->request_number++;

    if (filter->filter_string && filter_string) {
        if (strcmp(filter->filter_string, filter_string))
            filter_changed = 1;
    } else if (filter->filter_string || filter_string) {
        filter_changed = 1;
    }

    if (filter_changed) {
        while (filter->request_number > 1)
            uv_cond_wait(&filter->filter_cond, &filter->filter_mutex);

        freez(filter->filter_string);
        simple_pattern_free(filter->filter_sp);

        if (filter_string) {
            filter->filter_string = strdupz(filter_string);
            filter->filter_sp = simple_pattern_create(filter_string, NULL, SIMPLE_PATTERN_EXACT);
        } else {
            filter->filter_string = NULL;
            filter->filter_sp = NULL;
        }
    } else {
        uv_mutex_unlock(&filter->filter_mutex);
    }

    return filter_changed;
}

void unlock_allmetrics_filter(struct allmetrics_filter *filter, int filter_changed)
{
    if (!filter_changed) {
        uv_mutex_lock(&filter->filter_mutex);
    }

    filter->request_number--;

    uv_mutex_unlock(&filter->filter_mutex);
    uv_cond_signal(&filter->filter_cond);
}

int chart_is_filtered_out(RRDSET *st, struct allmetrics_filter *filter, int filter_changed, int filter_type)
{
    if (filter_changed) {
        if (!filter->filter_sp || simple_pattern_matches(filter->filter_sp, st->id) || simple_pattern_matches(filter->filter_sp, st->name))
            st->allmetrics_filter &= !filter_type; // chart should be sent
        else
            st->allmetrics_filter |= filter_type; // chart should be filtered out
    }

    if (unlikely(st->allmetrics_filter & filter_type))
        return 1;
    else
        return 0;
}

inline int web_client_api_request_v1_allmetrics(RRDHOST *host, struct web_client *w, char *url) {
    int format = ALLMETRICS_SHELL;
    const char *filter = NULL;
    const char *prometheus_server = w->client_ip;

    uint32_t prometheus_exporting_options;
    if (prometheus_exporter_instance)
        prometheus_exporting_options = prometheus_exporter_instance->config.options;
    else
        prometheus_exporting_options = global_exporting_options;

    PROMETHEUS_OUTPUT_OPTIONS prometheus_output_options =
        PROMETHEUS_OUTPUT_TIMESTAMPS |
        ((prometheus_exporting_options & EXPORTING_OPTION_SEND_NAMES) ? PROMETHEUS_OUTPUT_NAMES : 0);

    const char *prometheus_prefix;
    if (prometheus_exporter_instance)
        prometheus_prefix = prometheus_exporter_instance->config.prefix;
    else
        prometheus_prefix = global_exporting_prefix;

    while(url) {
        char *value = mystrsep(&url, "&");
        if (!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        if(!strcmp(name, "format")) {
            if(!strcmp(value, ALLMETRICS_FORMAT_SHELL))
                format = ALLMETRICS_SHELL;
            else if(!strcmp(value, ALLMETRICS_FORMAT_PROMETHEUS))
                format = ALLMETRICS_PROMETHEUS;
            else if(!strcmp(value, ALLMETRICS_FORMAT_PROMETHEUS_ALL_HOSTS))
                format = ALLMETRICS_PROMETHEUS_ALL_HOSTS;
            else if(!strcmp(value, ALLMETRICS_FORMAT_JSON))
                format = ALLMETRICS_JSON;
            else
                format = 0;
        }
        else if(!strcmp(name, "filter")) {
            filter = value;
        }
        else if(!strcmp(name, "server")) {
            prometheus_server = value;
        }
        else if(!strcmp(name, "prefix")) {
            prometheus_prefix = value;
        }
        else if(!strcmp(name, "data") || !strcmp(name, "source") || !strcmp(name, "data source") || !strcmp(name, "data-source") || !strcmp(name, "data_source") || !strcmp(name, "datasource")) {
            prometheus_exporting_options = exporting_parse_data_source(value, prometheus_exporting_options);
        }
        else {
            int i;
            for(i = 0; prometheus_output_flags_root[i].name ; i++) {
                if(!strcmp(name, prometheus_output_flags_root[i].name)) {
                    if(!strcmp(value, "yes") || !strcmp(value, "1") || !strcmp(value, "true"))
                        prometheus_output_options |= prometheus_output_flags_root[i].flag;
                    else
                        prometheus_output_options &= ~prometheus_output_flags_root[i].flag;

                    break;
                }
            }
        }
    }

    buffer_flush(w->response.data);
    buffer_no_cacheable(w->response.data);

    switch(format) {
        case ALLMETRICS_JSON:
            w->response.data->contenttype = CT_APPLICATION_JSON;
            rrd_stats_api_v1_charts_allmetrics_json(host, filter, w->response.data);
            return HTTP_RESP_OK;

        case ALLMETRICS_SHELL:
            w->response.data->contenttype = CT_TEXT_PLAIN;
            rrd_stats_api_v1_charts_allmetrics_shell(host, filter, w->response.data);
            return HTTP_RESP_OK;

        case ALLMETRICS_PROMETHEUS:
            w->response.data->contenttype = CT_PROMETHEUS;
            rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(
                    host
                    , filter
                    , w->response.data
                    , prometheus_server
                    , prometheus_prefix
                    , prometheus_exporting_options
                    , prometheus_output_options
            );
            return HTTP_RESP_OK;

        case ALLMETRICS_PROMETHEUS_ALL_HOSTS:
            w->response.data->contenttype = CT_PROMETHEUS;
            rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(
                    host
                    , filter
                    , w->response.data
                    , prometheus_server
                    , prometheus_prefix
                    , prometheus_exporting_options
                    , prometheus_output_options
            );
            return HTTP_RESP_OK;

        default:
            w->response.data->contenttype = CT_TEXT_PLAIN;
            buffer_strcat(w->response.data, "Which format? '" ALLMETRICS_FORMAT_SHELL "', '" ALLMETRICS_FORMAT_PROMETHEUS "', '" ALLMETRICS_FORMAT_PROMETHEUS_ALL_HOSTS "' and '" ALLMETRICS_FORMAT_JSON "' are currently supported.");
            return HTTP_RESP_BAD_REQUEST;
    }
}
