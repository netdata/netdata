// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP Resources Namespace
 * 
 * The MCP Resources namespace provides methods for accessing and managing resources on the server.
 * In the MCP protocol, resources are application-controlled data stores that provide context to the model.
 * Resources are passive, meaning they provide data but don't perform actions on their own.
 * 
 * Key features of the resources namespace:
 * 
 * 1. Resource Discovery:
 *    - Clients can list available resources (resources/list)
 *    - Get detailed descriptions and schemas (resources/describe, resources/getSchema)
 *    - Search for resources matching specific criteria (resources/search)
 * 
 * 2. Resource Access:
 *    - Retrieve specific resources or portions of resources (resources/get)
 *    - Access resources by ID or path
 *    - Resources can be structured or unstructured
 * 
 * 3. Resource Subscriptions:
 *    - Subscribe to updates for specific resources (resources/subscribe)
 *    - Unsubscribe from resources (resources/unsubscribe)
 *    - Get real-time updates when subscribed resources change
 * 
 * In the Netdata context, resources include:
 *    - metrics: Time-series data collected from various sources
 *    - logs: Log entries from system and application logs
 *    - alerts: Health monitoring alerts and notifications
 *    - functions: Live infrastructure snapshots providing real-time views
 *    - nodes: Monitored infrastructure nodes with their metadata
 * 
 * Resources can be hierarchical or flat, and may support different access patterns
 * (e.g., time-based querying for metrics, full-text search for logs).
 */

#include "mcp-resources.h"
#include "mcp-initialize.h"

// Implementation of resources/list (transport-agnostic)
static int mcp_resources_method_list(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    if (!mcpc || id == 0) return -1;

    struct json_object *result = json_object_new_object();
    struct json_object *resources_array = json_object_new_array();
    
    // Resource: metrics
    struct json_object *metrics_resource = json_object_new_object();
    json_object_object_add(metrics_resource, "name", json_object_new_string("metrics"));
    json_object_object_add(metrics_resource, "description", json_object_new_string(
        "Time-series metrics collected by Netdata from various sources"
    ));
    json_object_array_add(resources_array, metrics_resource);
    
    // Resource: logs
    struct json_object *logs_resource = json_object_new_object();
    json_object_object_add(logs_resource, "name", json_object_new_string("logs"));
    json_object_object_add(logs_resource, "description", json_object_new_string(
        "Log entries collected from system and application logs"
    ));
    json_object_array_add(resources_array, logs_resource);
    
    // Resource: alerts
    struct json_object *alerts_resource = json_object_new_object();
    json_object_object_add(alerts_resource, "name", json_object_new_string("alerts"));
    json_object_object_add(alerts_resource, "description", json_object_new_string(
        "Health monitoring alerts and notifications"
    ));
    json_object_array_add(resources_array, alerts_resource);
    
    // Resource: functions
    struct json_object *functions_resource = json_object_new_object();
    json_object_object_add(functions_resource, "name", json_object_new_string("functions"));
    json_object_object_add(functions_resource, "description", json_object_new_string(
        "Live infrastructure snapshots providing real-time views"
    ));
    json_object_array_add(resources_array, functions_resource);
    
    // Resource: nodes
    struct json_object *nodes_resource = json_object_new_object();
    json_object_object_add(nodes_resource, "name", json_object_new_string("nodes"));
    json_object_object_add(nodes_resource, "description", json_object_new_string(
        "Monitored infrastructure nodes with their metadata"
    ));
    json_object_array_add(resources_array, nodes_resource);
    
    // Add resources array to result
    json_object_object_add(result, "resources", resources_array);
    
    // Send success response and free the result object
    int ret = mcp_send_success_response(mcpc, result, id);
    json_object_put(result);
    
    return ret;
}

// Stub implementations for other methods (transport-agnostic)
static int mcp_resources_method_get(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "resources/get", id);
}

static int mcp_resources_method_search(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "resources/search", id);
}

static int mcp_resources_method_subscribe(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "resources/subscribe", id);
}

static int mcp_resources_method_unsubscribe(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "resources/unsubscribe", id);
}

static int mcp_resources_method_describe(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "resources/describe", id);
}

static int mcp_resources_method_getSchema(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "resources/getSchema", id);
}

// Resources namespace method dispatcher (transport-agnostic)
int mcp_resources_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, uint64_t id) {
    if (!mcpc || !method) return -1;

    netdata_log_debug(D_MCP, "MCP resources method: %s", method);

    if (strcmp(method, "list") == 0) {
        return mcp_resources_method_list(mcpc, params, id);
    }
    else if (strcmp(method, "get") == 0) {
        return mcp_resources_method_get(mcpc, params, id);
    }
    else if (strcmp(method, "search") == 0) {
        return mcp_resources_method_search(mcpc, params, id);
    }
    else if (strcmp(method, "subscribe") == 0) {
        return mcp_resources_method_subscribe(mcpc, params, id);
    }
    else if (strcmp(method, "unsubscribe") == 0) {
        return mcp_resources_method_unsubscribe(mcpc, params, id);
    }
    else if (strcmp(method, "describe") == 0) {
        return mcp_resources_method_describe(mcpc, params, id);
    }
    else if (strcmp(method, "getSchema") == 0) {
        return mcp_resources_method_getSchema(mcpc, params, id);
    }
    else {
        // Method not found in resources namespace
        char full_method[256];
        snprintf(full_method, sizeof(full_method), "resources/%s", method);
        return mcp_method_not_implemented_generic(mcpc, full_method, id);
    }
}
