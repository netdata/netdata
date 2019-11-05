// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

#if UNIT_TESTING
char *netdata_configured_user_config_dir = ".";
char *netdata_configured_stock_config_dir = ".";
char *netdata_configured_hostname = "test_host";
#endif

/**
 * Select Type
 *
 * Select the connector type based on the user input
 *
 * @param type is the string that defines the connector type
 *
 * @return It returns the connector id.
 */
BACKEND_TYPE exporting_select_type(const char *type)
{
    if (!strcmp(type, "connector_graphite") || !strcmp(type, "connector_graphite:plaintext")) {
        return BACKEND_TYPE_GRAPHITE;
    } else if (!strcmp(type, "connector_opentsdb") || !strcmp(type, "connector_opentsdb:telnet")) {
        return BACKEND_TYPE_OPENTSDB_USING_TELNET;
    } else if (!strcmp(type, "connector_opentsdb:http") || !strcmp(type, "connector_opentsdb:https")) {
        return BACKEND_TYPE_OPENTSDB_USING_HTTP;
    } else if (!strcmp(type, "connector_json") || !strcmp(type, "connector_json:plaintext")) {
        return BACKEND_TYPE_JSON;
    } else if (!strcmp(type, "connector_prometheus_remote_write") || !strcmp(type, "connector_prometheus_remote_write")) {
        return BACKEND_TYPE_PROMETEUS;
    } else if (!strcmp(type, "connector_kinesis") || !strcmp(type, "connector_kinesis:plaintext")) {
        return BACKEND_TYPE_KINESIS;
    } else if (!strcmp(type, "connector_mongodb") || !strcmp(type, "connector_mongodb:plaintext"))
        return BACKEND_TYPE_MONGODB;

    return BACKEND_TYPE_UNKNOWN;
}

/**
 * Read configuration
 *
 * Based on read configuration an engine data structure is filled with exporting connector instances.
 *
 * @return Returns a filled engine data structure or NULL if there are no connector instances configured.
 */
struct engine *read_exporting_config()
{
    int instances_to_activate = 0;
    int exporting_config_exists = 0;

    static struct engine *engine = NULL;
    struct connector_instance_list {
        struct connector_instance local_ci;
        struct connector_instance_list *next;
    };
    struct connector_instance local_ci;
    struct connector_instance_list *tmp_ci_list, *tmp_ci_list1;
    struct connector_instance_list **ci_list;

    if (unlikely(engine))
        return engine;

    char *filename = strdupz_path_subpath(netdata_configured_user_config_dir, EXPORTING_CONF);

    exporting_config_exists = appconfig_load(&exporting_config, filename, 0);
    if (!exporting_config_exists) {
        info("CONFIG: cannot load user exporting config '%s'. Will try the stock version.", filename);
        freez(filename);

        filename = strdupz_path_subpath(netdata_configured_stock_config_dir, EXPORTING_CONF);
        exporting_config_exists = appconfig_load(&exporting_config, filename, 0);
        if (!exporting_config_exists)
            info("CONFIG: cannot load stock exporting config '%s'. Running with internal defaults.", filename);
    }

    freez(filename);

    if (!exporting_config_exists)
        memcpy(&exporting_config, &netdata_config, sizeof(netdata_config));

    // Will build a list of instances per connector
    // TODO: change BACKEND to EXPORTING
    ci_list = callocz(sizeof(BACKEND_TYPE), sizeof(struct connector_instance_list *));

    while (get_connector_instance(&local_ci)) {
        BACKEND_TYPE backend_type;

        info("Processing connector instance (%s)", local_ci.instance_name);
        if (exporter_get_boolean(local_ci.instance_name, "enabled", 0)) {
            backend_type = exporting_select_type(local_ci.connector_name);

            info(
                "Instance (%s) on connector (%s) type=%d is enabled and scheduled for activation",
                local_ci.instance_name,
                local_ci.connector_name,
                backend_type);

            tmp_ci_list = (struct connector_instance_list *)callocz(1, sizeof(struct connector_instance_list));
            memcpy(&tmp_ci_list->local_ci, &local_ci, sizeof(local_ci));
            tmp_ci_list->next = ci_list[backend_type];
            ci_list[backend_type] = tmp_ci_list;
            instances_to_activate++;
        } else
            info("Instance (%s) on connector (%s) is not enabled", local_ci.instance_name, local_ci.connector_name);
    }

    if (unlikely(!instances_to_activate)) {
        info("No connector instances to activate");
        freez(ci_list);
        return NULL;
    }

    engine = (struct engine *)calloc(1, sizeof(struct engine));
    // TODO: Check and fill engine fields if actually needed

    if (exporting_config_exists) {
        engine->config.hostname =
            strdupz(exporter_get(CONFIG_SECTION_EXPORTING, "hostname", netdata_configured_hostname));
        engine->config.prefix = strdupz(exporter_get(CONFIG_SECTION_EXPORTING, "prefix", "netdata"));
        engine->config.update_every =
            exporter_get_number(CONFIG_SECTION_EXPORTING, EXPORTER_UPDATE_EVERY, EXPORTER_UPDATE_EVERY_DEFAULT);
    }

    for (size_t i = 0; i < sizeof(BACKEND_TYPE); i++) {
        // For each connector build list
        tmp_ci_list = ci_list[i];

        // If we have a list of instances for this connector then build it
        if (tmp_ci_list) {
            struct connector *tmp_connector;

            tmp_connector = (struct connector *)calloc(1, sizeof(struct connector));
            tmp_connector->next = engine->connector_root;
            engine->connector_root = tmp_connector;

            tmp_connector->config.type = i;
            tmp_connector->engine = engine;

            while (tmp_ci_list) {
                struct instance *tmp_instance;
                char *instance_name;

                info("Instance %s on %s", tmp_ci_list->local_ci.instance_name, tmp_ci_list->local_ci.connector_name);

                tmp_instance = (struct instance *)calloc(1, sizeof(struct instance));
                tmp_instance->connector = engine->connector_root;
                tmp_instance->next = engine->connector_root->instance_root;
                engine->connector_root->instance_root = tmp_instance;
                tmp_instance->connector = engine->connector_root;

                instance_name = tmp_ci_list->local_ci.instance_name;

                tmp_instance->config.name = strdupz(tmp_ci_list->local_ci.instance_name);

                tmp_instance->config.destination =
                    strdupz(exporter_get(instance_name, EXPORTER_DESTINATION, EXPORTER_DESTINATION_DEFAULT));

                tmp_instance->config.update_every =
                    exporter_get_number(instance_name, EXPORTER_UPDATE_EVERY, EXPORTER_UPDATE_EVERY_DEFAULT);

                tmp_instance->config.buffer_on_failures =
                    exporter_get_number(instance_name, EXPORTER_BUF_ONFAIL, EXPORTER_BUF_ONFAIL_DEFAULT);

                tmp_instance->config.timeoutms =
                    exporter_get_number(instance_name, EXPORTER_TIMEOUT_MS, EXPORTER_TIMEOUT_MS_DEFAULT);

                tmp_instance->config.charts_pattern = simple_pattern_create(
                    exporter_get(instance_name, EXPORTER_SEND_CHART_MATCH, EXPORTER_SEND_CHART_MATCH_DEFAULT),
                    NULL,
                    SIMPLE_PATTERN_EXACT);

                tmp_instance->config.hosts_pattern = simple_pattern_create(
                    exporter_get(instance_name, EXPORTER_SEND_HOST_MATCH, EXPORTER_SEND_HOST_MATCH_DEFAULT),
                    NULL,
                    SIMPLE_PATTERN_EXACT);

                tmp_instance->config.send_names_instead_of_ids =
                    exporter_get_boolean(instance_name, EXPORTER_SEND_NAMES, EXPORTER_SEND_NAMES_DEFAULT);

#ifdef NETDATA_INTERNAL_CHECKS
                info(
                    "     Dest=[%s], upd=[%d], buffer=[%d] timeout=[%ld] names=[%d]",
                    tmp_instance->config.destination,
                    tmp_instance->config.update_every,
                    tmp_instance->config.buffer_on_failures,
                    tmp_instance->config.timeoutms,
                    tmp_instance->config.send_names_instead_of_ids);
#endif

#ifndef UNIT_TESTING
                if (unlikely(!exporting_config_exists) && !engine->config.hostname) {
                    engine->config.hostname =
                        strdupz(config_get(instance_name, "hostname", netdata_configured_hostname));
                    engine->config.prefix = strdupz(config_get(instance_name, "prefix", "netdata"));
                    engine->config.update_every =
                        config_get_number(instance_name, EXPORTER_UPDATE_EVERY, EXPORTER_UPDATE_EVERY_DEFAULT);
                }
#endif

                tmp_ci_list1 = tmp_ci_list->next;
                freez(tmp_ci_list);
                tmp_ci_list = tmp_ci_list1;
            }
        }
    }

    freez(ci_list);

    return engine;
}
