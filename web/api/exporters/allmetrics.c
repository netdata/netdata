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

inline int web_client_api_request_v1_allmetrics(RRDHOST *host, struct web_client *w, char *url) {
    (void)url;
    int format = ALLMETRICS_SHELL;
    const char *prometheus_server = w->client_ip;
    uint32_t prometheus_backend_options = global_backend_options;
    PROMETHEUS_OUTPUT_OPTIONS prometheus_output_options = PROMETHEUS_OUTPUT_TIMESTAMPS | ((global_backend_options & BACKEND_OPTION_SEND_NAMES)?PROMETHEUS_OUTPUT_NAMES:0);
    const char *prometheus_prefix = global_backend_prefix;

    uint32_t end = w->total_params;
    if (end) {
        uint32_t i = 0;
        do {
            char *name = w->param_name[i].body;
            size_t lname = w->param_name[i].length;
            char *value = w->param_values[i].body;
            size_t lvalue = w->param_values[i].length;

            if(!strncmp(name, "format",lname)) {
                if(!strncmp(value, ALLMETRICS_FORMAT_SHELL,lvalue))
                    format = ALLMETRICS_SHELL;
                else if(!strncmp(value, ALLMETRICS_FORMAT_PROMETHEUS,lvalue))
                    format = ALLMETRICS_PROMETHEUS;
                else if(!strncmp(value, ALLMETRICS_FORMAT_PROMETHEUS_ALL_HOSTS,lvalue))
                    format = ALLMETRICS_PROMETHEUS_ALL_HOSTS;
                else if(!strncmp(value, ALLMETRICS_FORMAT_JSON,lvalue))
                    format = ALLMETRICS_JSON;
                else
                    format = 0;
            }
            else if(!strncmp(name, "server",lname)) {
                prometheus_server = value;
            }
            else if(!strncmp(name, "prefix",lname)) {
                prometheus_prefix = value;
            }
            else if(!strncmp(name, "data",lname) || !strncmp(name, "source",lname) || !strncmp(name, "data source",lname) || !strncmp(name, "data-source",lname) || !strncmp(name, "data_source",lname) || !strncmp(name, "datasource",lname)) {
                prometheus_backend_options = backend_parse_data_source(value, prometheus_backend_options);
            }
            else {
                int i;
                for(i = 0; prometheus_output_flags_root[i].name ; i++) {
                    if(!strncmp(name, prometheus_output_flags_root[i].name,lname)) {
                        if(!strncmp(value, "yes",lvalue) || !strncmp(value, "1",lvalue) || !strncmp(value, "true",lvalue))
                            prometheus_output_options |= prometheus_output_flags_root[i].flag;
                        else
                            prometheus_output_options &= ~prometheus_output_flags_root[i].flag;

                        break;
                    }
                }
            }
        } while( ++i < end);
    }

    buffer_flush(w->response.data);
    buffer_no_cacheable(w->response.data);

    switch(format) {
        case ALLMETRICS_JSON:
            w->response.data->contenttype = CT_APPLICATION_JSON;
            rrd_stats_api_v1_charts_allmetrics_json(host, w->response.data);
            return 200;

        case ALLMETRICS_SHELL:
            w->response.data->contenttype = CT_TEXT_PLAIN;
            rrd_stats_api_v1_charts_allmetrics_shell(host, w->response.data);
            return 200;

        case ALLMETRICS_PROMETHEUS:
            w->response.data->contenttype = CT_PROMETHEUS;
            rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(
                    host
                    , w->response.data
                    , prometheus_server
                    , prometheus_prefix
                    , prometheus_backend_options
                    , prometheus_output_options
            );
            return 200;

        case ALLMETRICS_PROMETHEUS_ALL_HOSTS:
            w->response.data->contenttype = CT_PROMETHEUS;
            rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(
                    host
                    , w->response.data
                    , prometheus_server
                    , prometheus_prefix
                    , prometheus_backend_options
                    , prometheus_output_options
            );
            return 200;

        default:
            w->response.data->contenttype = CT_TEXT_PLAIN;
            buffer_strcat(w->response.data, "Which format? '" ALLMETRICS_FORMAT_SHELL "', '" ALLMETRICS_FORMAT_PROMETHEUS "', '" ALLMETRICS_FORMAT_PROMETHEUS_ALL_HOSTS "' and '" ALLMETRICS_FORMAT_JSON "' are currently supported.");
            return 400;
    }
}
