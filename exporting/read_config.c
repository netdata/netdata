// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

/**
 * Read configuration
 *
 * Based on read configuration an engine data structure is filled with exporting connector instances.
 *
 * @return Returns a filled engine data structure or NULL if there are no connector instances configured.
 */
struct engine *read_exporting_config()
{
    // TODO: compose the configuration filename ()
#if UNIT_TESTING
    // TODO: filename = "./exporting.conf"
#endif

    // TODO: read and parse the configuration file ()

    // temporary configuration stub
    struct engine *engine = (struct engine *)calloc(1, sizeof(struct engine));
    engine->config.prefix = strdupz("netdata");
    engine->config.hostname = strdupz("test-host");
    engine->config.update_every = 3;
    engine->config.options = BACKEND_SOURCE_DATA_AVERAGE | BACKEND_OPTION_SEND_NAMES;

    engine->connector_root = (struct connector *)calloc(1, sizeof(struct connector));
    engine->connector_root->config.type = BACKEND_TYPE_GRAPHITE;
    engine->connector_root->engine = engine;

    engine->connector_root->instance_root = (struct instance *)calloc(1, sizeof(struct instance));
    struct instance *instance = engine->connector_root->instance_root;
    instance->connector = engine->connector_root;
    instance->config.destination = strdupz("localhost");
    instance->config.update_every = 1;
    instance->config.buffer_on_failures = 10;
    instance->config.timeoutms = 10000;
    instance->config.charts_pattern = strdupz("*");
    instance->config.hosts_pattern = strdupz("localhost *");
    instance->config.send_names_instead_of_ids = 1;

    return engine;
}
