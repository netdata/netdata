// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_capas.h"

#include "ml/ml.h"

const struct capability *aclk_get_agent_capas()
{
    static struct capability agent_capabilities[] = {
        { .name = "json",  .version = 2, .enabled = 0 },
        { .name = "proto", .version = 1, .enabled = 1 },
        { .name = "ml",    .version = 0, .enabled = 0 },
        { .name = "mc",    .version = 0, .enabled = 0 },
        { .name = "ctx",   .version = 1, .enabled = 1 },
        { .name = "funcs", .version = 1, .enabled = 1 },
        { .name = NULL,    .version = 0, .enabled = 0 }
    };
    agent_capabilities[2].version = ml_capable() ? 1 : 0;
    agent_capabilities[2].enabled = ml_enabled(localhost);

    agent_capabilities[3].version = enable_metric_correlations ? metric_correlations_version : 0;
    agent_capabilities[3].enabled = enable_metric_correlations;

    return agent_capabilities;
}

struct capability *aclk_get_node_instance_capas(RRDHOST *host)
{
    struct capability ni_caps[] = {
        { .name = "proto", .version = 1,                     .enabled = 1 },
        { .name = "ml",    .version = ml_capable(),          .enabled = ml_enabled(host) },
        { .name = "mc",
          .version = enable_metric_correlations ? metric_correlations_version : 0,
          .enabled = enable_metric_correlations },
        { .name = "ctx",   .version = 1,                     .enabled = 1 },
        { .name = "funcs", .version = 0,                     .enabled = 0 },
        { .name = NULL,    .version = 0,                     .enabled = 0 }
    };
    if (host->receiver && stream_has_capability(host->receiver, STREAM_CAP_FUNCTIONS)) {
        ni_caps[4].version = 1;
        ni_caps[4].enabled = 1;
    }

    struct capability *ret = mallocz(sizeof(ni_caps));
    memcpy(ret, ni_caps, sizeof(ni_caps));
    return ret;
}
