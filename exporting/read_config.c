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
    static struct engine *engine = NULL;
    struct connector_instance_list {
        struct connector_instance local_ci;
        struct connector_instance_list  *next;
    };
    struct connector_instance       local_ci;
    struct connector_instance_list *tmp_ci_list, *tmp_ci_list1;
    struct connector_instance_list **ci_list;

    if (unlikely(engine))
        return engine;

    // TODO: compose the configuration filename ()
#if UNIT_TESTING
    // TODO: filename = "./exporting.conf"
#endif

    // TODO: read and parse the configuration file ()

    // File is already read and parsed
    ci_list = callocz(sizeof(BACKEND_TYPE), sizeof(struct connector_instance_list *));

    while (get_connector_instance(&local_ci)) {
        BACKEND_TYPE    backend_type;
        if (exporter_get_boolean(local_ci.instance_name, "enabled",0)) {
            backend_type =  backend_select_type(local_ci.connector_name);

            info("Instance %s on %s (connector_type = %d) -- Enabled", local_ci.instance_name,
                    local_ci.connector_name, backend_type);

            tmp_ci_list  = (struct connector_instance_list *) callocz(1, sizeof(struct connector_instance_list));
            memcpy(&tmp_ci_list->local_ci, &local_ci, sizeof(local_ci));
            tmp_ci_list->next = ci_list[backend_type];
            ci_list[backend_type] = tmp_ci_list;
        }
        else
            info("Instance %s on %s (connector_type = %d) -- Disabled", local_ci.instance_name,
                    local_ci.connector_name, backend_type);
    }

    engine = (struct engine *)calloc(1, sizeof(struct engine));
    // TODO: Check and fill engine fields if actually needed

    for(int i=0; i < sizeof(BACKEND_TYPE); i++) {
        // For each connector build list
        tmp_ci_list = ci_list[i];

        // If we have a list of instances for this connector then build it
        if (tmp_ci_list) {
            struct connector *tmp_connector;

            tmp_connector = (struct connector *) calloc(1, sizeof(struct connector));
            tmp_connector->next = engine->connector_root;
            engine->connector_root = tmp_connector;

            tmp_connector->config.type = i;
            tmp_connector->engine = engine;

            while (tmp_ci_list) {
                struct instance *tmp_instance;

                info("  Instance %s on %s", tmp_ci_list->local_ci.instance_name,
                        tmp_ci_list->local_ci.connector_name);

                tmp_instance = (struct instance *)calloc(1, sizeof(struct instance));
                tmp_instance->connector = engine->connector_root;
                tmp_instance->next = engine->connector_root->instance_root;
                engine->connector_root->instance_root = tmp_instance;
                tmp_instance->connector = engine->connector_root;

                tmp_instance->config.name = strdupz(tmp_ci_list->local_ci.instance_name);
                tmp_instance->config.destination = strdupz(
                        exporter_get(tmp_ci_list->local_ci.instance_name, "destination", "localhost"));
                tmp_instance->config.update_every = exporter_get_number(tmp_ci_list->local_ci.instance_name,
                                                                            "update every", 10);
                tmp_instance->config.buffer_on_failures = exporter_get_number(
                        tmp_ci_list->local_ci.instance_name, "buffer on failures", 10);
                tmp_instance->config.timeoutms = exporter_get_number(tmp_ci_list->local_ci.instance_name,
                                                                         "timeout ms", 10000);
                tmp_instance->config.charts_pattern = simple_pattern_create(
                        exporter_get(tmp_ci_list->local_ci.instance_name, "send charts matching", "*"), NULL,
                        SIMPLE_PATTERN_EXACT);
                tmp_instance->config.hosts_pattern = simple_pattern_create(
                        exporter_get(tmp_ci_list->local_ci.instance_name, "send hosts matching", "localhost *"),
                        NULL, SIMPLE_PATTERN_EXACT);
                tmp_instance->config.send_names_instead_of_ids = exporter_get_boolean(
                        tmp_ci_list->local_ci.instance_name, "send names instead of ids", 1);

                info("     Dest=[%s], upd=[%d], buffer=[%d] timeout=[%ld] names=[%d]",
                        tmp_instance->config.destination, tmp_instance->config.update_every,
                        tmp_instance->config.buffer_on_failures, tmp_instance->config.timeoutms,
                        tmp_instance->config.send_names_instead_of_ids);

                tmp_ci_list1 = tmp_ci_list->next;
                freez(tmp_ci_list);
                tmp_ci_list = tmp_ci_list1;
            }
        }
    }

    freez(ci_list);

    // temporary configuration stub
//    struct engine *engine = (struct engine *)calloc(1, sizeof(struct engine));
//    engine->config.prefix = strdupz("netdata");
//    engine->config.hostname = strdupz("test-host");
//    engine->config.update_every = 3;
//    engine->config.options = BACKEND_SOURCE_DATA_AVERAGE | BACKEND_OPTION_SEND_NAMES;

    //engine->connector_root = (struct connector *)calloc(1, sizeof(struct connector));
    //engine->connector_root->config.type = BACKEND_TYPE_GRAPHITE;
    //engine->connector_root->engine = engine;

//    engine->connector_root->instance_root = (struct instance *)calloc(1, sizeof(struct instance));
//    struct instance *instance = engine->connector_root->instance_root;
//    instance->connector = engine->connector_root;
//    instance->config.destination = strdupz("localhost");
//    instance->config.update_every = 1;
//    instance->config.buffer_on_failures = 10;
//    instance->config.timeoutms = 10000;
//    instance->config.charts_pattern = strdupz("*");
//    instance->config.hosts_pattern = strdupz("localhost *");
//    instance->config.send_names_instead_of_ids = 1;

    return engine;
}
