// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"
#include "graphite/graphite.h"
#include "json/json.h"
#include "opentsdb/opentsdb.h"

/**
 * Initialize connectors
 *
 * @param engine an engine data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_connectors(struct engine *engine)
{
    engine->now = now_realtime_sec();

    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        switch (connector->config.type) {
            case BACKEND_TYPE_GRAPHITE:
                if (init_graphite_connector(connector) != 0)
                    return 1;
                break;
            case BACKEND_TYPE_JSON:
                if (init_json_connector(connector) != 0)
                    return 1;
                break;
            case BACKEND_TYPE_OPENTSDB_USING_TELNET:
                if (init_opentsdb_connector(connector) != 0)
                    return 1;
                break;
            case BACKEND_TYPE_OPENTSDB_USING_HTTP:
                if (init_opentsdb_connector(connector) != 0)
                    return 1;
                break;
            default:
                error("EXPORTING: unknown exporting connector type");
                return 1;
        }
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            instance->index = engine->instance_num++;
            instance->after = engine->now;

            switch (connector->config.type) {
                case BACKEND_TYPE_GRAPHITE:
                    if (init_graphite_instance(instance) != 0)
                        return 1;
                    break;
                case BACKEND_TYPE_JSON:
                    if (init_json_instance(instance) != 0)
                        return 1;
                    break;
                case BACKEND_TYPE_OPENTSDB_USING_TELNET:
                    if (init_opentsdb_telnet_instance(instance) != 0)
                        return 1;
                    break;
                case BACKEND_TYPE_OPENTSDB_USING_HTTP:
                    if (init_opentsdb_http_instance(instance) != 0)
                        return 1;
                    break;
                default:
                    error("EXPORTING: unknown exporting connector type");
                    return 1;
            }

            // dispatch the instance worker thread
            int error = uv_thread_create(&instance->thread, connector->worker, instance);
            if (error) {
                error("EXPORTING: cannot create tread worker. uv_thread_create(): %s", uv_strerror(error));
                return 1;
            }
            char threadname[NETDATA_THREAD_NAME_MAX+1];
            snprintfz(threadname, NETDATA_THREAD_NAME_MAX, "EXPORTING-%zu", instance->index);
            uv_thread_set_name_np(instance->thread, threadname);
        }
    }

    return 0;
}
