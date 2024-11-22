// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_capas.h"

#include "ml/ml_public.h"

#define HTTP_API_V2_VERSION 7

size_t aclk_get_http_api_version(void) {
    return HTTP_API_V2_VERSION;
}

const struct capability *aclk_get_agent_capas()
{
    static struct capability agent_capabilities[] = {
        { .name = "json",        .version = 2, .enabled = 0 },
        { .name = "proto",       .version = 1, .enabled = 1 },
        { .name = "ml",          .version = 0, .enabled = 0 }, // index 2, below
        { .name = "mc",          .version = 0, .enabled = 0 }, // index 3, below
        { .name = "ctx",         .version = 1, .enabled = 1 },
        { .name = "funcs",       .version = 1, .enabled = 1 },
        { .name = "http_api_v2", .version = HTTP_API_V2_VERSION, .enabled = 1 },
        { .name = "health",      .version = 2, .enabled = 0 }, // index 7, below
        { .name = "req_cancel",  .version = 1, .enabled = 1 },
        { .name = "dyncfg",      .version = 2, .enabled = 1 },
        { .name = NULL,          .version = 0, .enabled = 0 }
    };
    agent_capabilities[2].version = ml_capable() ? 1 : 0;
    agent_capabilities[2].enabled = ml_enabled(localhost);

    agent_capabilities[3].version = metric_correlations_version;
    agent_capabilities[3].enabled = 1;

    agent_capabilities[7].enabled = localhost->health.enabled;

    return agent_capabilities;
}

struct capability *aclk_get_node_instance_capas(RRDHOST *host)
{
    bool functions = (host == localhost || receiver_has_capability(host, STREAM_CAP_FUNCTIONS));
    bool dyncfg = (host == localhost || dyncfg_available_for_rrdhost(host));

    struct capability ni_caps[] = {
        { .name = "proto",       .version = 1,                     .enabled = 1 },
        { .name = "ml",          .version = ml_capable(),          .enabled = ml_enabled(host) },
        { .name = "mc",          .version = metric_correlations_version, .enabled = 1 },
        { .name = "ctx",         .version = 1,                     .enabled = 1 },
        { .name = "funcs",       .version = functions ? 1 : 0,     .enabled = functions ? 1 : 0 },
        { .name = "http_api_v2", .version = HTTP_API_V2_VERSION,   .enabled = 1 },
        { .name = "health",      .version = 2,                     .enabled = host->health.enabled},
        { .name = "req_cancel",  .version = 1,                     .enabled = 1 },
        { .name = "dyncfg",      .version = 2,                     .enabled = dyncfg },
        { .name = NULL,          .version = 0,                     .enabled = 0 }
    };

    struct capability *ret = mallocz(sizeof(ni_caps));
    memcpy(ret, ni_caps, sizeof(ni_caps));

    return ret;
}
