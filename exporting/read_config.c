// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

struct config exporting_config = {.sections = NULL,
                                  .mutex = NETDATA_MUTEX_INITIALIZER,
                                  .index = {.avl_tree = {.root = NULL, .compar = appconfig_section_compare},
                                            .rwlock = AVL_LOCK_INITIALIZER}};


static _CONNECTOR_INSTANCE *find_instance(const char *section)
{
    _CONNECTOR_INSTANCE *local_ci;

    local_ci = add_connector_instance(NULL, NULL);  // Get root section
    if (unlikely(!local_ci))
        return local_ci;

    if (!section)
        return local_ci;

    while (local_ci) {
        if (!strcmp(local_ci->instance_name, section))
            break;
        local_ci = local_ci->next;
    }
    return local_ci;
}

char *expconfig_get(struct config *root, const char *section, const char *name, const char *default_value)
{
    _CONNECTOR_INSTANCE *local_ci;

    if (!strcmp(section, CONFIG_SECTION_EXPORTING))
        return appconfig_get(root, CONFIG_SECTION_EXPORTING, name, default_value);

    local_ci = find_instance(section);

    if (!local_ci)
        return NULL;    // TODO: Check if it is meaningful to return default_value

    return appconfig_get(
        root,
        local_ci->instance_name,
        name,
        appconfig_get(
            root, local_ci->connector_name, name, appconfig_get(root, CONFIG_SECTION_EXPORTING, name, default_value)));
}

int expconfig_get_boolean(struct config *root, const char *section, const char *name, int default_value)
{
    _CONNECTOR_INSTANCE *local_ci;

    if (!strcmp(section, CONFIG_SECTION_EXPORTING))
        return appconfig_get_boolean(root, CONFIG_SECTION_EXPORTING, name, default_value);

    local_ci = find_instance(section);

    if (!local_ci)
        return 0;       // TODO: Check if it is meaningful to return default_value

    return appconfig_get_boolean(
        root,
        local_ci->instance_name,
        name,
        appconfig_get_boolean(
            root,
            local_ci->connector_name,
            name,
            appconfig_get_boolean(root, CONFIG_SECTION_EXPORTING, name, default_value)));
}

long long expconfig_get_number(struct config *root, const char *section, const char *name, long long default_value)
{
    _CONNECTOR_INSTANCE *local_ci;

    if (!strcmp(section, CONFIG_SECTION_EXPORTING))
        return appconfig_get_number(root, CONFIG_SECTION_EXPORTING, name, default_value);

    local_ci = find_instance(section);

    if (!local_ci)
        return 0;   // TODO: Check if it is meaningful to return default_value

    return appconfig_get_number(
        root,
        local_ci->instance_name,
        name,
        appconfig_get_number(
            root,
            local_ci->connector_name,
            name,
            appconfig_get_number(root, CONFIG_SECTION_EXPORTING, name, default_value)));
}

/*
 * Get the next connector instance that we need to activate
 *
 * @param @target_ci will be filled with instance name and connector name
 *
 * @return  - 1 if more connectors to be fetched, 0 done
 *
 */

int get_connector_instance(struct connector_instance *target_ci)
{
    static _CONNECTOR_INSTANCE *local_ci = NULL;
    _CONNECTOR_INSTANCE *global_connector_instance;

    global_connector_instance = find_instance(NULL);       // Fetch head of instances

    if (unlikely(!global_connector_instance))
        return 0;

    if (target_ci == NULL) {
        local_ci = NULL;
        return 1;
    }
    if (local_ci == NULL)
        local_ci = global_connector_instance;
    else {
        local_ci = local_ci->next;
        if (local_ci == NULL)
            return 0;
    }

    strcpy(target_ci->instance_name, local_ci->instance_name);
    strcpy(target_ci->connector_name, local_ci->connector_name);

    return 1;
}

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
    if (!strcmp(type, "graphite") || !strcmp(type, "graphite:plaintext")) {
        return BACKEND_TYPE_GRAPHITE;
    } else if (!strcmp(type, "opentsdb") || !strcmp(type, "opentsdb:telnet")) {
        return BACKEND_TYPE_OPENTSDB_USING_TELNET;
    } else if (!strcmp(type, "opentsdb:http") || !strcmp(type, "opentsdb:https")) {
        return BACKEND_TYPE_OPENTSDB_USING_HTTP;
    } else if (!strcmp(type, "json") || !strcmp(type, "json:plaintext")) {
        return BACKEND_TYPE_JSON;
    } else if (!strcmp(type, "prometheus_remote_write")) {
        return BACKEND_TYPE_PROMETHEUS;
    } else if (!strcmp(type, "kinesis") || !strcmp(type, "kinesis:plaintext")) {
        return BACKEND_TYPE_KINESIS;
    } else if (!strcmp(type, "mongodb") || !strcmp(type, "mongodb:plaintext"))
        return BACKEND_TYPE_MONGODB;

    return BACKEND_TYPE_UNKNOWN;
}

EXPORTING_OPTIONS exporting_parse_data_source(const char *data_source, EXPORTING_OPTIONS exporting_options) {
    if(!strcmp(data_source, "raw") || !strcmp(data_source, "as collected") || !strcmp(data_source, "as-collected") || !strcmp(data_source, "as_collected") || !strcmp(data_source, "ascollected")) {
        exporting_options |= EXPORTING_SOURCE_DATA_AS_COLLECTED;
        exporting_options &= ~(EXPORTING_OPTIONS_SOURCE_BITS ^ EXPORTING_SOURCE_DATA_AS_COLLECTED);
    }
    else if(!strcmp(data_source, "average")) {
        exporting_options |= EXPORTING_SOURCE_DATA_AVERAGE;
        exporting_options &= ~(EXPORTING_OPTIONS_SOURCE_BITS ^ EXPORTING_SOURCE_DATA_AVERAGE);
    }
    else if(!strcmp(data_source, "sum") || !strcmp(data_source, "volume")) {
        exporting_options |= EXPORTING_SOURCE_DATA_SUM;
        exporting_options &= ~(EXPORTING_OPTIONS_SOURCE_BITS ^ EXPORTING_SOURCE_DATA_SUM);
    }
    else {
        error("EXPORTING: invalid data data_source method '%s'.", data_source);
    }

    return exporting_options;
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
        BACKEND_TYPE backend_type;

        struct connector_instance_list *next;
    };
    struct connector_instance local_ci;
    struct connector_instance_list *tmp_ci_list, *tmp_ci_list1, *tmp_ci_list_prev = NULL;

    if (unlikely(engine))
        return engine;

    char *filename = strdupz_path_subpath(netdata_configured_user_config_dir, EXPORTING_CONF);

    exporting_config_exists = appconfig_load(&exporting_config, filename, 0, NULL);
    if (!exporting_config_exists) {
        info("CONFIG: cannot load user exporting config '%s'. Will try the stock version.", filename);
        freez(filename);

        filename = strdupz_path_subpath(netdata_configured_stock_config_dir, EXPORTING_CONF);
        exporting_config_exists = appconfig_load(&exporting_config, filename, 0, NULL);
        if (!exporting_config_exists)
            info("CONFIG: cannot load stock exporting config '%s'. Running with internal defaults.", filename);
    }

    freez(filename);

    // TODO: change BACKEND to EXPORTING
    while (get_connector_instance(&local_ci)) {
        info("Processing connector instance (%s)", local_ci.instance_name);

        if (exporter_get_boolean(local_ci.instance_name, "enabled", 0)) {
            info(
                "Instance (%s) on connector (%s) is enabled and scheduled for activation",
                local_ci.instance_name,
                local_ci.connector_name);

            tmp_ci_list = (struct connector_instance_list *)callocz(1, sizeof(struct connector_instance_list));
            memcpy(&tmp_ci_list->local_ci, &local_ci, sizeof(local_ci));
            tmp_ci_list->backend_type = exporting_select_type(local_ci.connector_name);
            tmp_ci_list->next = tmp_ci_list_prev;
            tmp_ci_list_prev = tmp_ci_list;
            instances_to_activate++;
        } else
            info("Instance (%s) on connector (%s) is not enabled", local_ci.instance_name, local_ci.connector_name);
    }

    if (unlikely(!instances_to_activate)) {
        info("No connector instances to activate");
        return NULL;
    }

    engine = (struct engine *)callocz(1, sizeof(struct engine));
    // TODO: Check and fill engine fields if actually needed

    if (exporting_config_exists) {
        engine->config.hostname =
            strdupz(exporter_get(CONFIG_SECTION_EXPORTING, "hostname", netdata_configured_hostname));
        engine->config.prefix = strdupz(exporter_get(CONFIG_SECTION_EXPORTING, "prefix", "netdata"));
        engine->config.update_every =
            exporter_get_number(CONFIG_SECTION_EXPORTING, EXPORTER_UPDATE_EVERY, EXPORTER_UPDATE_EVERY_DEFAULT);
    }

    while (tmp_ci_list) {
        struct instance *tmp_instance;
        char *instance_name;

        info("Instance %s on %s", tmp_ci_list->local_ci.instance_name, tmp_ci_list->local_ci.connector_name);

        if (tmp_ci_list->backend_type == BACKEND_TYPE_UNKNOWN) {
            error("Unknown exporting connector type");
            goto next_connector_instance;
        }

#ifndef HAVE_KINESIS
        if (tmp_ci_list->backend_type == BACKEND_TYPE_KINESIS) {
            error("AWS Kinesis support isn't compiled");
            goto next_connector_instance;
        }
#endif

        tmp_instance = (struct instance *)callocz(1, sizeof(struct instance));
        tmp_instance->next = engine->instance_root;
        engine->instance_root = tmp_instance;

        tmp_instance->engine = engine;
        tmp_instance->config.type = tmp_ci_list->backend_type;

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

        char *data_source =
            exporter_get(instance_name, EXPORTER_DATA_SOURCE, EXPORTER_DATA_SOURCE_DEFAULT);

        tmp_instance->config.options = exporting_parse_data_source(data_source, tmp_instance->config.options);

        if (exporter_get_boolean(
                instance_name, EXPORTER_SEND_CONFIGURED_LABELS, EXPORTER_SEND_CONFIGURED_LABELS_DEFAULT))
            tmp_instance->config.options |= EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
        else
            tmp_instance->config.options &= ~EXPORTING_OPTION_SEND_CONFIGURED_LABELS;

        if (exporter_get_boolean(
                instance_name, EXPORTER_SEND_AUTOMATIC_LABELS, EXPORTER_SEND_AUTOMATIC_LABELS_DEFAULT))
            tmp_instance->config.options |= EXPORTING_OPTION_SEND_AUTOMATIC_LABELS;
        else
            tmp_instance->config.options &= ~EXPORTING_OPTION_SEND_AUTOMATIC_LABELS;

        if (exporter_get_boolean(instance_name, EXPORTER_SEND_NAMES, EXPORTER_SEND_NAMES_DEFAULT))
            tmp_instance->config.options |= EXPORTING_OPTION_SEND_NAMES;
        else
            tmp_instance->config.options &= ~EXPORTING_OPTION_SEND_NAMES;

        if (tmp_instance->config.type == BACKEND_TYPE_KINESIS) {
            struct aws_kinesis_specific_config *connector_specific_config =
                callocz(1, sizeof(struct aws_kinesis_specific_config));

            tmp_instance->config.connector_specific_config = connector_specific_config;

            connector_specific_config->stream_name = strdupz(exporter_get(
                instance_name, EXPORTER_KINESIS_STREAM_NAME, EXPORTER_KINESIS_STREAM_NAME_DEFAULT));

            connector_specific_config->auth_key_id = strdupz(exporter_get(
                instance_name, EXPORTER_AWS_ACCESS_KEY_ID, ""));

            connector_specific_config->secure_key = strdupz(exporter_get(
                instance_name, EXPORTER_AWS_SECRET_ACCESS_KEY, ""));
        }

#ifdef NETDATA_INTERNAL_CHECKS
        info(
            "     Dest=[%s], upd=[%d], buffer=[%d] timeout=[%ld] options=[%u]",
            tmp_instance->config.destination,
            tmp_instance->config.update_every,
            tmp_instance->config.buffer_on_failures,
            tmp_instance->config.timeoutms,
            tmp_instance->config.options);
#endif

        if (unlikely(!exporting_config_exists) && !engine->config.hostname) {
            engine->config.hostname =
                strdupz(config_get(instance_name, "hostname", netdata_configured_hostname));
            engine->config.prefix = strdupz(config_get(instance_name, "prefix", "netdata"));
            engine->config.update_every =
                config_get_number(instance_name, EXPORTER_UPDATE_EVERY, EXPORTER_UPDATE_EVERY_DEFAULT);
        }

next_connector_instance:
        tmp_ci_list1 = tmp_ci_list->next;
        freez(tmp_ci_list);
        tmp_ci_list = tmp_ci_list1;
    }

    return engine;
}
