// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP Resources Namespace
 * 
 * The MCP Resources namespace provides methods for accessing and managing resources on the server.
 * In the MCP protocol, resources are application-controlled data stores that provide context to the model.
 * Resources are passive, meaning they provide data but don't perform actions on their own.
 * 
 * Standard methods in the MCP specification:
 * 
 * 1. resources/list - Lists available resources
 *    - Returns a collection of resources the server can provide
 *    - May include resource metadata such as name, description, and URI
 *    - Can be paginated for large resource collections
 *
 * 2. resources/read - Reads a specific resource by URI
 *    - Takes a resource URI and returns its contents
 *    - Contents can be text, binary data, or structured information
 *    - URIs follow a standard format, typically with a scheme prefix
 *
 * 3. resources/templates/list - Lists available resource templates
 *    - Returns a collection of URI templates for constructing resource URIs
 *    - Templates describe how to construct valid resource URIs
 *    - May include template descriptions and parameter information
 *
 * 4. resources/subscribe - Subscribes to changes in a resource
 *    - Takes a resource URI and registers for change notifications
 *    - When the resource changes, the server sends notifications
 *    - Allows clients to maintain up-to-date views of resources
 *
 * 5. resources/unsubscribe - Unsubscribes from a resource
 *    - Takes a resource URI and removes the subscription
 *    - Stops receiving notifications for that resource
 * 
 * In the Netdata context, resources include:
 *    - metrics: Time-series data collected from various sources
 *    - logs: Log entries from system and application logs
 *    - alerts: Health monitoring alerts and notifications
 *    - contexts: Hierarchical organization of metrics and their metadata
 *    - nodes: Monitored infrastructure nodes with their metadata
 * 
 * Resources are identified by URIs (e.g., "nd://contexts") and can be hierarchical or flat,
 * supporting different access patterns like time-based querying for metrics.
 */

#include "mcp-resources.h"
#include "database/contexts/rrdcontext.h"

// Audience enum - bitmask for the intended audience of a resource
typedef enum {
    RESOURCE_AUDIENCE_USER      = 1 << 0,  // Resource useful for users
    RESOURCE_AUDIENCE_ASSISTANT = 1 << 1,  // Resource useful for assistants
    RESOURCE_AUDIENCE_BOTH      = RESOURCE_AUDIENCE_USER | RESOURCE_AUDIENCE_ASSISTANT
} RESOURCE_AUDIENCE;

// Function pointer type for resource-read callbacks
typedef MCP_RETURN_CODE (*resource_read_fn)(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id);

// Function pointer type for resource size callbacks
typedef size_t (*resource_size_fn)(void);

// Resource structure definition
typedef struct {
    const char *name;           // Resource name
    const char *uri;            // Resource URI
    const char *description;    // Human-readable description
    HTTP_CONTENT_TYPE content_type; // Content type enum
    RESOURCE_AUDIENCE audience; // Intended audience
    double priority;            // Priority (0.0-1.0)
    resource_read_fn read_fn;   // Callback function to read the resource
    resource_size_fn size_fn;   // Optional callback function to return approximate size in bytes
} MCP_RESOURCE;

// Resource template structure definition
typedef struct {
    const char *name;           // Template name
    const char *uri_template;   // URI template following RFC 6570
    const char *description;    // Human-readable description
    HTTP_CONTENT_TYPE content_type; // Content type enum
    RESOURCE_AUDIENCE audience; // Intended audience
    double priority;            // Priority (0.0-1.0)
} MCP_RESOURCE_TEMPLATE;

// Implementation of resources/list
static MCP_RETURN_CODE mcp_resources_method_list(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id __maybe_unused) {
    if (!mcpc || !params) return MCP_RC_INTERNAL_ERROR;

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Create an empty resource array object
    buffer_json_member_add_array(mcpc->result, "resources");
    buffer_json_array_close(mcpc->result); // Close resources array

    buffer_json_finalize(mcpc->result);
    return MCP_RC_OK;
}

// Implementation of resources/read
static MCP_RETURN_CODE mcp_resources_method_read(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id __maybe_unused) {
    if (!mcpc || !params) return MCP_RC_INTERNAL_ERROR;

    // Extract URI from params
    struct json_object *uri_obj = NULL;
    if (!json_object_object_get_ex(params, "uri", &uri_obj)) {
        buffer_strcat(mcpc->error, "Missing 'uri' parameter");
        return MCP_RC_INVALID_PARAMS;
    }
    
    const char *uri = json_object_get_string(uri_obj);
    if (!uri) {
        buffer_strcat(mcpc->error, "Invalid 'uri' parameter");
        return MCP_RC_INVALID_PARAMS;
    }

    netdata_log_debug(D_MCP, "MCP resources/read for URI: %s", uri);
    
    // Since we have no resources, always return not found
    buffer_sprintf(mcpc->error, "Unknown resource URI: %s", uri);
    return MCP_RC_NOT_FOUND;
}

// Implementation of resources/templates/list
static MCP_RETURN_CODE mcp_resources_method_templates_list(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id __maybe_unused) {
    if (!mcpc || !params) return MCP_RC_INTERNAL_ERROR;

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Create an empty resourceTemplates array object
    buffer_json_member_add_array(mcpc->result, "resourceTemplates");
    buffer_json_array_close(mcpc->result); // Close resourceTemplates array

    buffer_json_finalize(mcpc->result);
    return MCP_RC_OK;
}

// Implementation of resources/subscribe (transport-agnostic)
static MCP_RETURN_CODE mcp_resources_method_subscribe(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id __maybe_unused) {
    if (!mcpc || !params) return MCP_RC_INTERNAL_ERROR;
    return MCP_RC_NOT_IMPLEMENTED;
}

// Implementation of resources/unsubscribe (transport-agnostic)
static MCP_RETURN_CODE mcp_resources_method_unsubscribe(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id __maybe_unused) {
    if (!mcpc || !params) return MCP_RC_INTERNAL_ERROR;
    return MCP_RC_NOT_IMPLEMENTED;
}

// Resource namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_resources_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, MCP_REQUEST_ID id __maybe_unused) {
    if (!mcpc || !method) return MCP_RC_INTERNAL_ERROR;

    netdata_log_debug(D_MCP, "MCP resources method: %s", method);

    MCP_RETURN_CODE rc;
    
    if (strcmp(method, "list") == 0) {
        rc = mcp_resources_method_list(mcpc, params, id);
    }
    else if (strcmp(method, "read") == 0) { 
        rc = mcp_resources_method_read(mcpc, params, id);
    }
    else if (strcmp(method, "templates/list") == 0) {
        rc = mcp_resources_method_templates_list(mcpc, params, id);
    }
    else if (strcmp(method, "subscribe") == 0) {
        rc = mcp_resources_method_subscribe(mcpc, params, id);
    }
    else if (strcmp(method, "unsubscribe") == 0) {
        rc = mcp_resources_method_unsubscribe(mcpc, params, id);
    }
    else {
        // Method not found in resource namespace
        buffer_sprintf(mcpc->error, "Method 'resources/%s' not implemented yet", method);
        rc = MCP_RC_NOT_IMPLEMENTED;
    }
    
    return rc;
}
