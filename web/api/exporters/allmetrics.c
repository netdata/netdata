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
    int format = ALLMETRICS_SHELL;
    const char *prometheus_server = w->client_ip;
    uint32_t prometheus_backend_options = global_backend_options;
    PROMETHEUS_OUTPUT_OPTIONS prometheus_output_options = PROMETHEUS_OUTPUT_TIMESTAMPS | ((global_backend_options & BACKEND_OPTION_SEND_NAMES)?PROMETHEUS_OUTPUT_NAMES:0);
    const char *prometheus_prefix = global_backend_prefix;

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
        else if(!strcmp(name, "server")) {
            prometheus_server = value;
        }
        else if(!strcmp(name, "prefix")) {
            prometheus_prefix = value;
        }
        else if(!strcmp(name, "data") || !strcmp(name, "source") || !strcmp(name, "data source") || !strcmp(name, "data-source") || !strcmp(name, "data_source") || !strcmp(name, "datasource")) {
            prometheus_backend_options = backend_parse_data_source(value, prometheus_backend_options);
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
