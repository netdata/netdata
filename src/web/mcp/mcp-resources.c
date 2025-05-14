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

// Audience enum - bitmask for the intended audience of a resource
typedef enum {
    RESOURCE_AUDIENCE_USER      = 1 << 0,  // Resource useful for users
    RESOURCE_AUDIENCE_ASSISTANT = 1 << 1,  // Resource useful for assistants
    RESOURCE_AUDIENCE_BOTH      = RESOURCE_AUDIENCE_USER | RESOURCE_AUDIENCE_ASSISTANT
} RESOURCE_AUDIENCE;

// Function pointer type for resource read callbacks
typedef MCP_RETURN_CODE (*resource_read_fn)(MCP_CLIENT *mcpc, struct json_object *params, uint64_t id);

// Resource structure definition
typedef struct {
    const char *name;           // Resource name
    const char *uri;            // Resource URI
    const char *description;    // Human-readable description
    HTTP_CONTENT_TYPE content_type; // Content type enum
    RESOURCE_AUDIENCE audience; // Intended audience
    double priority;            // Priority (0.0-1.0)
    resource_read_fn read_fn;   // Callback function to read the resource
} MCP_RESOURCE;

// Basic implementation of the contexts resource read function
static MCP_RETURN_CODE mcp_resource_read_contexts(MCP_CLIENT *mcpc, struct json_object *params, uint64_t id) {
    if (!mcpc || !params || id == 0) return MCP_RC_INTERNAL_ERROR;
    return MCP_RC_NOT_IMPLEMENTED;
}

// Static array of all available resources
static const MCP_RESOURCE mcp_resources[] = {
    {
        .name = "contexts",
        .uri = "nd://contexts",
        .description =
            "Primary discovery mechanism for what's being monitored.\n"
            "Provides the most concise overview of monitoring categories.",
        .content_type = CT_APPLICATION_JSON,
        .audience = RESOURCE_AUDIENCE_BOTH,
        .priority = 1.0,
        .read_fn = mcp_resource_read_contexts
    },
    // Add more resources here as the are implemented
    // Example:
    // {
    //     .name = "nodes",
    //     .uri = "nd://nodes",
    //     .description = "Infrastructure discovery...",
    //     ...
    // },
};

// Number of resources in the array
#define MCP_RESOURCES_COUNT (sizeof(mcp_resources) / sizeof(MCP_RESOURCE))

// Implementation of resources/list (transport-agnostic)
static MCP_RETURN_CODE mcp_resources_method_list(MCP_CLIENT *mcpc, struct json_object *params, uint64_t id) {
    if (!mcpc || !params || !id) return MCP_RC_INTERNAL_ERROR;

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Create a resources array object
    buffer_json_member_add_array(mcpc->result, "resources");
    
    // Iterate through our resources array and add each one
    for (size_t i = 0; i < MCP_RESOURCES_COUNT; i++) {
        const MCP_RESOURCE *resource = &mcp_resources[i];
        
        buffer_json_add_array_item_object(mcpc->result);
        
        // Add required fields
        buffer_json_member_add_string(mcpc->result, "name", resource->name);
        buffer_json_member_add_string(mcpc->result, "uri", resource->uri);
        
        // Add optional fields
        if (resource->description) {
            buffer_json_member_add_string(mcpc->result, "description", resource->description);
        }
        
        // Convert the content_type enum to string
        const char *mime_type = content_type_id2string(resource->content_type);
        if (mime_type) {
            buffer_json_member_add_string(mcpc->result, "mimeType", mime_type);
        }
        
        // Add audience annotations if specified
        if (resource->audience != 0) {
            buffer_json_member_add_object(mcpc->result, "annotations");
            
            buffer_json_member_add_array(mcpc->result, "audience");
            
            if (resource->audience & RESOURCE_AUDIENCE_USER) {
                buffer_json_add_array_item_string(mcpc->result, "user");
            }
            
            if (resource->audience & RESOURCE_AUDIENCE_ASSISTANT) {
                buffer_json_add_array_item_string(mcpc->result, "assistant");
            }
            
            buffer_json_array_close(mcpc->result); // Close audience array
            
            // Add priority if it's non-zero
            if (resource->priority > 0) {
                buffer_json_member_add_double(mcpc->result, "priority", resource->priority);
            }
            
            buffer_json_object_close(mcpc->result); // Close annotations object
        }
        
        buffer_json_object_close(mcpc->result); // Close resource object
    }
    
    buffer_json_array_close(mcpc->result); // Close resources array
    buffer_json_object_close(mcpc->result); // Close result object
    
    // For now, no need for pagination since we have a small number of resources
    // If we add many resources later, implement cursor-based pagination here
    
    return MCP_RC_OK;
}

// Implementation of resources/read (transport-agnostic)
static MCP_RETURN_CODE mcp_resources_method_read(MCP_CLIENT *mcpc, struct json_object *params, uint64_t id) {
    if (!mcpc || id == 0 || !params) return MCP_RC_INTERNAL_ERROR;

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
    
    // Find the matching resource in our array
    for (size_t i = 0; i < MCP_RESOURCES_COUNT; i++) {
        const MCP_RESOURCE *resource = &mcp_resources[i];
        
        // Check if the URI matches
        if (strcmp(resource->uri, uri) == 0) {
            // Found matching resource, check if read function exists
            if (resource->read_fn) {
                // Call the resource-specific read function
                return resource->read_fn(mcpc, params, id);
            }
            else {
                // No read function implemented
                buffer_strcat(mcpc->error, "Resource reading not implemented");
                return MCP_RC_NOT_IMPLEMENTED;
            }
        }
    }
    
    // No matching resource found
    buffer_sprintf(mcpc->error, "Unknown resource URI: %s", uri);
    return MCP_RC_NOT_FOUND;
}

// Implementation of resources/templates/list (transport-agnostic)
static MCP_RETURN_CODE mcp_resources_method_templates_list(MCP_CLIENT *mcpc, struct json_object *params, uint64_t id) {
    if (!mcpc || !params || !id) return MCP_RC_INTERNAL_ERROR;
    return MCP_RC_NOT_IMPLEMENTED;
}

// Implementation of resources/subscribe (transport-agnostic)
static MCP_RETURN_CODE mcp_resources_method_subscribe(MCP_CLIENT *mcpc, struct json_object *params, uint64_t id) {
    if (!mcpc || !id || !params) return MCP_RC_INTERNAL_ERROR;
    return MCP_RC_NOT_IMPLEMENTED;
}

// Implementation of resources/unsubscribe (transport-agnostic)
static MCP_RETURN_CODE mcp_resources_method_unsubscribe(MCP_CLIENT *mcpc, struct json_object *params, uint64_t id) {
    if (!mcpc || id == 0 || !params) return MCP_RC_INTERNAL_ERROR;
    return MCP_RC_NOT_IMPLEMENTED;
}

// Resources namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_resources_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, uint64_t id) {
    if (!mcpc || !method) return MCP_RC_INTERNAL_ERROR;

    netdata_log_debug(D_MCP, "MCP resources method: %s", method);

    // Clear previous buffers
    buffer_flush(mcpc->result);
    buffer_flush(mcpc->error);
    
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
        // Method not found in resources namespace
        buffer_sprintf(mcpc->error, "Method 'resources/%s' not implemented yet", method);
        rc = MCP_RC_NOT_IMPLEMENTED;
    }
    
    return rc;
}
