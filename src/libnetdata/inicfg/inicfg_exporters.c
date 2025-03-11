// SPDX-License-Identifier: GPL-3.0-or-later

#include "inicfg_internals.h"

/*
 * @Input:
 *      Connector / instance to add to an internal structure
 * @Return
 *      The current head of the linked list of connector_instance
 *
 */

_CONNECTOR_INSTANCE *add_connector_instance(struct config_section *connector, struct config_section *instance)
{
    static struct _connector_instance *global_connector_instance = NULL;
    struct _connector_instance *local_ci, *local_ci_tmp;

    if (unlikely(!connector)) {
        if (unlikely(!instance))
            return global_connector_instance;

        local_ci = global_connector_instance;
        while (local_ci) {
            local_ci_tmp = local_ci->next;
            freez(local_ci);
            local_ci = local_ci_tmp;
        }
        global_connector_instance = NULL;
        return NULL;
    }

    local_ci = callocz(1, sizeof(struct _connector_instance));
    local_ci->instance = instance;
    local_ci->connector = connector;
    strncpyz(local_ci->instance_name, string2str(instance->name), CONFIG_MAX_NAME);
    strncpyz(local_ci->connector_name, string2str(connector->name), CONFIG_MAX_NAME);
    local_ci->next = global_connector_instance;
    global_connector_instance = local_ci;

    return global_connector_instance;
}

int is_valid_connector(char *type, int check_reserved) {
    int rc = 1;

    if (unlikely(!type))
        return 0;

    if (!check_reserved) {
        if (unlikely(is_valid_connector(type,1))) {
            return 0;
        }
        //if (unlikely(*type == ':')
        //    return 0;
        char *separator = strrchr(type, ':');
        if (likely(separator)) {
            *separator = '\0';
            rc = (int)(separator - type);
        } else
            return 0;
    }
    //    else {
    //        if (unlikely(is_valid_connector(type,1))) {
    //            netdata_log_error("Section %s invalid -- reserved name", type);
    //            return 0;
    //        }
    //    }

    if (!strcmp(type, "graphite") || !strcmp(type, "graphite:plaintext")) {
        return rc;
    } else if (!strcmp(type, "graphite:http") || !strcmp(type, "graphite:https")) {
        return rc;
    } else if (!strcmp(type, "json") || !strcmp(type, "json:plaintext")) {
        return rc;
    } else if (!strcmp(type, "json:http") || !strcmp(type, "json:https")) {
        return rc;
    } else if (!strcmp(type, "opentsdb") || !strcmp(type, "opentsdb:telnet")) {
        return rc;
    } else if (!strcmp(type, "opentsdb:http") || !strcmp(type, "opentsdb:https")) {
        return rc;
    } else if (!strcmp(type, "prometheus_remote_write")) {
        return rc;
    } else if (!strcmp(type, "prometheus_remote_write:http") || !strcmp(type, "prometheus_remote_write:https")) {
        return rc;
    } else if (!strcmp(type, "kinesis") || !strcmp(type, "kinesis:plaintext")) {
        return rc;
    } else if (!strcmp(type, "pubsub") || !strcmp(type, "pubsub:plaintext")) {
        return rc;
    } else if (!strcmp(type, "mongodb") || !strcmp(type, "mongodb:plaintext")) {
        return rc;
    }

    return 0;
}

