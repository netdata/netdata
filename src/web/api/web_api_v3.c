// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api_v3.h"
#include "v1/api_v1_calls.h"
#include "v2/api_v2_calls.h"
#include "v3/api_v3_calls.h"

static struct web_api_command api_commands_v3[] = {
    // time-series multi-node multi-instance data APIs
    {
        .api = "data",
        .hash = 0,
        .acl = HTTP_ACL_METRICS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v2_data,
        .allow_subpaths = 0
    },
    // badges
    {
     .api = "badge.svg",
     .hash = 0,
     .acl = HTTP_ACL_BADGES,
     .access = HTTP_ACCESS_ANONYMOUS_DATA,
     .callback = api_v1_badge,
     .allow_subpaths = 0
    },
    // scoring engine
    {
        .api = "weights",
        .hash = 0,
        .acl = HTTP_ACL_METRICS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v2_weights,
        .allow_subpaths = 0
    },
    // exporting API
    {
     .api = "allmetrics",
     .hash = 0,
     .acl = HTTP_ACL_METRICS,
     .access = HTTP_ACCESS_ANONYMOUS_DATA,
     .callback = api_v1_allmetrics,
     .allow_subpaths = 0
    },

    // time-series multi-node multi-instance metadata APIs
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
        .callback = api_v2_contexts,
        .allow_subpaths = 0
    },

    // fulltext search
    {
     .api = "q",
     .hash = 0,
     .acl = HTTP_ACL_METRICS,
     .access = HTTP_ACCESS_ANONYMOUS_DATA,
     .callback = api_v2_q,
     .allow_subpaths = 0
    },

    // multi-node multi-instance alerts APIs
    {
        .api = "alerts",
        .hash = 0,
        .acl = HTTP_ACL_ALERTS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v2_alerts,
        .allow_subpaths = 0
    },
    {
        .api = "alert_transitions",
        .hash = 0,
        .acl = HTTP_ACL_ALERTS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v2_alert_transitions,
        .allow_subpaths = 0
    },
    {
        .api = "alert_config",
        .hash = 0,
        .acl = HTTP_ACL_ALERTS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v2_alert_config,
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

    // agent information APIs
    {
        .api = "info",
        .hash = 0,
        .acl = HTTP_ACL_NOCHECK,
        .access = HTTP_ACCESS_NONE,
        .callback = api_v2_info,
        .allow_subpaths = 0
    },
    {
        .api = "nodes",
        .hash = 0,
        .acl = HTTP_ACL_NODES,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v2_nodes,
        .allow_subpaths = 0
    },
    {
        .api = "node_instances",
        .hash = 0,
        .acl = HTTP_ACL_NODES,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v2_node_instances,
        .allow_subpaths = 0
    },
    {
        .api = "stream_path",
        .hash = 0,
        .acl = HTTP_ACL_NODES,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v3_stream_path,
        .allow_subpaths = 0
    },
    {
        .api = "versions",
        .hash = 0,
        .acl = HTTP_ACL_NOCHECK,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v2_versions,
        .allow_subpaths = 0
    },
    {
        .api = "progress",
        .hash = 0,
        .acl = HTTP_ACL_NOCHECK,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v2_progress,
        .allow_subpaths = 0
    },

    // functions APIs
    {
        .api = "function",
        .hash = 0,
        .acl = HTTP_ACL_FUNCTIONS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_function,
        .allow_subpaths = 0
    },
    {
        .api = "functions",
        .hash = 0,
        .acl = HTTP_ACL_FUNCTIONS,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v2_functions,
        .allow_subpaths = 0
    },

    // dyncfg APIs
    {
        .api = "config",
        .hash = 0,
        .acl = HTTP_ACL_DYNCFG,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v1_config,
        .allow_subpaths = 0
    },

    // settings APIs
    {
        .api = "settings",
        .hash = 0,
        .acl = HTTP_ACL_NOCHECK,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = api_v3_settings,
        .allow_subpaths = 0
    },

    {
        .api = "stream_info",
        .hash = 0,
        .acl = HTTP_ACL_NOCHECK,
        .access = HTTP_ACCESS_NONE,
        .callback = api_v3_stream_info,
        .allow_subpaths = 0
    },

    // WebRTC APIs
    {
        .api = "rtc_offer",
        .hash = 0,
        .acl = HTTP_ACL_ACLK | ACL_DEV_OPEN_ACCESS,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE,
        .callback = api_v2_webrtc,
        .allow_subpaths = 0
    },

    // management APIs
    {
        .api = "claim",
        .hash = 0,
        .acl = HTTP_ACL_NOCHECK,
        .access = HTTP_ACCESS_NONE,
        .callback = api_v3_claim,
        .allow_subpaths = 0
    },
    {
        .api = "bearer_protection",
        .hash = 0,
        .acl = HTTP_ACL_ACLK | ACL_DEV_OPEN_ACCESS,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_VIEW_AGENT_CONFIG | HTTP_ACCESS_EDIT_AGENT_CONFIG,
        .callback = api_v2_bearer_protection,
        .allow_subpaths = 0
    },
    {
        .api = "bearer_get_token",
        .hash = 0,
        .acl = HTTP_ACL_ACLK | ACL_DEV_OPEN_ACCESS,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE,
        .callback = api_v2_bearer_get_token,
        .allow_subpaths = 0
    },
    {
        .api = "me",
        .hash = 0,
        .acl = HTTP_ACL_NOCHECK,
        .access = HTTP_ACCESS_NONE,
        .callback = api_v3_me,
        .allow_subpaths = 0
    },

    {// terminator
     .api = NULL,
     .hash = 0,
     .acl = HTTP_ACL_NONE,
     .access = HTTP_ACCESS_NONE,
     .callback = NULL,
     .allow_subpaths = 0
    },
};

inline int web_client_api_request_v3(RRDHOST *host, struct web_client *w, char *url_path_endpoint) {
    static int initialized = 0;

    if(unlikely(initialized == 0)) {
        initialized = 1;

        for(int i = 0; api_commands_v3[i].api ; i++)
            api_commands_v3[i].hash = simple_hash(api_commands_v3[i].api);
    }

    return web_client_api_request_vX(host, w, url_path_endpoint, api_commands_v3);
}
