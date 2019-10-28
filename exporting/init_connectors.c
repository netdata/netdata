// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"
#include "graphite/graphite.h"

/**
 * Initialize connectors
 *
 * @param engine an engine data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_connectors(struct engine *engine)
{
    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        switch (connector->config.type) {
                case BACKEND_TYPE_GRAPHITE:
                    if (init_graphite_connector(connector) != 0)
                        return 1;
                    break;
                default:
                    error("EXPORTING: unknown exporting connector type");
                    return 1;
            }
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            switch (connector->config.type) {
                case BACKEND_TYPE_GRAPHITE:
                    if (init_graphite_instance(instance) != 0)
                        return 1;
                    break;
                default:
                    error("EXPORTING: unknown exporting connector type");
                    return 1;
            }
        }
    }

    return 0;
}
