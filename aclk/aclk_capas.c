// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_capas.h"

#include "ml/ml.h"

#define HTTP_API_V2_VERSION 6

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
        { .name = "health",      .version = 1, .enabled = 0 }, // index 7, below
        { .name = "req_cancel",  .version = 1, .enabled = 1 },
        //{ .name = "dyncfg",      .version = 1, .enabled = 1 },
        { .name = NULL,          .version = 0, .enabled = 0 }
    };
    agent_capabilities[2].version = ml_capable() ? 1 : 0;
    agent_capabilities[2].enabled = ml_enabled(localhost);

    agent_capabilities[3].version = enable_metric_correlations ? metric_correlations_version : 0;
    agent_capabilities[3].enabled = enable_metric_correlations;

    agent_capabilities[7].enabled = localhost->health.health_enabled;

    return agent_capabilities;
}

struct capability *aclk_get_node_instance_capas(RRDHOST *host)
{
    bool functions = (host == localhost || (host->receiver && stream_has_capability(host->receiver, STREAM_CAP_FUNCTIONS)));

    struct capability ni_caps[] = {
        { .name = "proto",       .version = 1,                     .enabled = 1 },
        { .name = "ml",          .version = ml_capable(),          .enabled = ml_enabled(host) },
        { .name = "mc",
          .version = enable_metric_correlations ? metric_correlations_version : 0,
          .enabled = enable_metric_correlations },
        { .name = "ctx",         .version = 1,                     .enabled = 1 },
        { .name = "funcs",       .version = functions ? 1 : 0,     .enabled = functions ? 1 : 0 },
        { .name = "http_api_v2", .version = HTTP_API_V2_VERSION,   .enabled = 1 },
        { .name = "health",      .version = 1,                     .enabled = host->health.health_enabled },
        { .name = "req_cancel",  .version = 1,                     .enabled = 1 },
        //{ .name = "dyncfg",      .version = 1,                     .enabled = 1 },
        { .name = NULL,          .version = 0,                     .enabled = 0 }
    };

    struct capability *ret = mallocz(sizeof(ni_caps));
    memcpy(ret, ni_caps, sizeof(ni_caps));

    return ret;
}
