// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api_v3.h"
#include "v3/api_v3_calls.h"

static struct web_api_command api_commands_v3[] = {
    // time-series multi-node multi-instance data APIs
    {
        .api = "data",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_data,
        .allow_subpaths = 0
    },
    {
        .api = "weights",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_weights,
        .allow_subpaths = 0
    },

    // badges can be fetched with both dashboard and badge ACL
    {
     .api = "badge.svg",
     .hash = 0,
     .acl = HTTP_ACL_BADGES,
     .access = HTTP_ACCESS_ANONYMOUS_DATA,
     .callback = web_client_api_request_v3_badge,
     .allow_subpaths = 0
    },

    // time-series multi-node multi-instance metadata APIs
    {
        .api = "contexts",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_contexts,
        .allow_subpaths = 0
    },

    // full text search
    {
     .api = "q",
     .hash = 0,
     .acl = HTTP_ACL_DASHBOARD,
     .access = HTTP_ACCESS_ANONYMOUS_DATA,
     .callback = web_client_api_request_v3_q,
     .allow_subpaths = 0
    },

    // exporting API
    {
        .api = "allmetrics",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_allmetrics,
        .allow_subpaths = 0
    },

    // multi-node multi-instance alerts APIs
    {
        .api = "alerts",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_alerts,
        .allow_subpaths = 0
    },
    {
        .api = "alert_transitions",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_alert_transitions,
        .allow_subpaths = 0
    },
    {
        .api = "alert_config",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_alert_config,
        .allow_subpaths = 0
    },

    // agent information APIs
    {
        .api = "info",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_info,
        .allow_subpaths = 0
    },
    {
        .api = "nodes",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_nodes,
        .allow_subpaths = 0
    },
    {
        .api = "node_instances",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_node_instances,
        .allow_subpaths = 0
    },
    {
        .api = "versions",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_versions,
        .allow_subpaths = 0
    },
    {
        .api = "progress",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_progress,
        .allow_subpaths = 0
    },

    // functions APIs
    {
        .api = "function",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_function,
        .allow_subpaths = 0
    },
    {
        .api = "functions",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_functions,
        .allow_subpaths = 0
    },

    // WebRTC APIs
    {
        .api = "rtc_offer",
        .hash = 0,
        .acl = HTTP_ACL_ACLK | ACL_DEV_OPEN_ACCESS,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE,
        .callback = web_client_api_request_v3_webrtc,
        .allow_subpaths = 0
    },

    // dyncfg APIs
    {
        .api = "config",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_config,
        .allow_subpaths = 0
    },

    // management APIs
    {
        .api = "claim",
        .hash = 0,
        .acl = HTTP_ACL_NOCHECK,
        .access = HTTP_ACCESS_NONE,
        .callback = web_client_api_request_v3_claim,
        .allow_subpaths = 0
    },
    {
        .api = "bearer_protection",
        .hash = 0,
        .acl = HTTP_ACL_ACLK | ACL_DEV_OPEN_ACCESS,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_VIEW_AGENT_CONFIG | HTTP_ACCESS_EDIT_AGENT_CONFIG,
        .callback = api_v3_bearer_protection,
        .allow_subpaths = 0
    },
    {
        .api = "bearer_get_token",
        .hash = 0,
        .acl = HTTP_ACL_ACLK | ACL_DEV_OPEN_ACCESS,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE,
        .callback = api_v3_bearer_token,
        .allow_subpaths = 0
    },

    // Netdata branding APIs
    {
        .api = "ilove.svg",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_ilove,
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
