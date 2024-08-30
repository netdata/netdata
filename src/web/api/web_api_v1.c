// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api_v1.h"
#include "v1/api_v1_calls.h"
#include "v2/api_v2_calls.h"
#include "v3/api_v3_calls.h"

static struct web_api_command api_commands_v1[] = {
    // time-series data APIs
    {
        .api = "data",
        .hash = 0,
        .acl = HTTP_ACL_METRICS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_data,
        .allow_subpaths = 0
    },
#if defined(ENABLE_API_V1)
    {
        .api = "weights",
        .hash = 0,
        .acl = HTTP_ACL_METRICS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_weights,
        .allow_subpaths = 0
    },
    {
        // deprecated - do not use anymore - use "weights"
        .api = "metric_correlations",
        .hash = 0,
        .acl = HTTP_ACL_METRICS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_metric_correlations,
        .allow_subpaths = 0
    },
#endif
    {
        .api = "badge.svg",
        .hash = 0,
        .acl = HTTP_ACL_BADGES,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_badge,
        .allow_subpaths = 0
    },
    {
        // exporting API
        .api = "allmetrics",
        .hash = 0,
        .acl = HTTP_ACL_METRICS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_allmetrics,
        .allow_subpaths = 0
    },

    // alerts APIs
#if defined(ENABLE_API_V1)
    {
        .api = "alarms",
        .hash = 0,
        .acl = HTTP_ACL_ALERTS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_alarms,
        .allow_subpaths = 0
    },
    {
        .api = "alarms_values",
        .hash = 0,
        .acl = HTTP_ACL_ALERTS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_alarms_values,
        .allow_subpaths = 0
    },
    {
        .api = "alarm_log",
        .hash = 0,
        .acl = HTTP_ACL_ALERTS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_alarm_log,
        .allow_subpaths = 0
    },
    {
        .api = "alarm_variables",
        .hash = 0,
        .acl = HTTP_ACL_ALERTS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_alarm_variables,
        .allow_subpaths = 0
    },
    {
        .api = "variable",
        .hash = 0,
        .acl = HTTP_ACL_ALERTS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_variable,
        .allow_subpaths = 0
    },
    {
        .api = "alarm_count",
        .hash = 0,
        .acl = HTTP_ACL_ALERTS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_alarm_count,
        .allow_subpaths = 0
    },
#endif

    // functions APIs - they check permissions per function call
    {
        .api = "function",
        .hash = 0,
        .acl = HTTP_ACL_FUNCTIONS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_function,
        .allow_subpaths = 0
    },

#if defined(ENABLE_API_V1)
    {
        .api = "functions",
        .hash = 0,
        .acl = HTTP_ACL_FUNCTIONS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_functions,
        .allow_subpaths = 0
    },
#endif

    // time-series metadata APIs
#if defined(ENABLE_API_V1)
    {
        .api = "chart",
        .hash = 0,
        .acl = HTTP_ACL_METRICS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_chart,
        .allow_subpaths = 0
    },
#endif
    {
        .api = "charts",
        .hash = 0,
        .acl = HTTP_ACL_METRICS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_charts,
        .allow_subpaths = 0
    },
    {
        .api = "context",
        .hash = 0,
        .acl = HTTP_ACL_METRICS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_context,
        .allow_subpaths = 0
    },
    {
        .api = "contexts",
        .hash = 0,
        .acl = HTTP_ACL_METRICS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_contexts,
        .allow_subpaths = 0
    },

    // registry APIs
#if defined(ENABLE_API_V1)
    {
        // registry checks the ACL by itself, so we allow everything
        .api = "registry",
        .hash = 0,
        .acl = HTTP_ACL_NONE, // it manages acl by itself
        .access = HTTP_ACCESS_NONE, // it manages access by itself
        .callback = api_v1_registry,
        .allow_subpaths = 0
    },
#endif

    // agent information APIs
#if defined(ENABLE_API_V1)
    {
        .api = "info",
        .hash = 0,
        .acl = HTTP_ACL_NODES,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_info,
        .allow_subpaths = 0
    },
    {
        .api = "aclk",
        .hash = 0,
        .acl = HTTP_ACL_NODES,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_aclk,
        .allow_subpaths = 0
    },
    {
        // deprecated - use /api/v2/info
        .api = "dbengine_stats",
        .hash = 0,
        .acl = HTTP_ACL_NODES,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_dbengine_stats,
        .allow_subpaths = 0
    },
    {
        .api = "ml_info",
        .hash = 0,
        .acl = HTTP_ACL_NODES,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_ml_info,
        .allow_subpaths = 0
    },
    {
     .api = "manage",
     .hash = 0,
     .acl = HTTP_ACL_MANAGEMENT,
     .access = HTTP_ACCESS_NONE, // it manages access by itself
     .callback = api_v1_manage,
     .allow_subpaths = 1
    },
#endif

    // dyncfg APIs
    {
        .api = "config",
        .hash = 0,
        .acl = HTTP_ACL_DYNCFG,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_config,
        .allow_subpaths = 0
    },

    {
        // terminator - keep this last on this list
        .api = NULL,
        .hash = 0,
        .acl = HTTP_ACL_NONE,
        .access = HTTP_ACCESS_NONE,
        .callback = NULL,
        .allow_subpaths = 0
    },
};

inline int web_client_api_request_v1(RRDHOST *host, struct web_client *w, char *url_path_endpoint) {
    static int initialized = 0;

    if(unlikely(initialized == 0)) {
        initialized = 1;

        for(int i = 0; api_commands_v1[i].api ; i++)
            api_commands_v1[i].hash = simple_hash(api_commands_v1[i].api);
    }

    return web_client_api_request_vX(host, w, url_path_endpoint, api_commands_v1);
}
