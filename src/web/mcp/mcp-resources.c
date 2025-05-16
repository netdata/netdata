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

// Function pointer type for resource read callbacks
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

// Basic implementation of the contexts resource read function
static MCP_RETURN_CODE mcp_resource_read_contexts(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || !params || id == 0) return MCP_RC_INTERNAL_ERROR;

    // Extract URI from params to check for query parameters
    struct json_object *uri_obj = NULL;
    json_object_object_get_ex(params, "uri", &uri_obj);
    const char *uri = json_object_get_string(uri_obj);
    
    SIMPLE_PATTERN *pattern = NULL;
    
    // Check if we have a query parameter
    if (uri && strstr(uri, "?like=")) {
        const char *like_param = strstr(uri, "?like=") + 6; // Skip past "?like="
        
        // Decode the query parameter
        const char *decoded_query = mcp_uri_decode(mcpc, like_param);
        
        // Create a simple pattern
        if (decoded_query && *decoded_query)
            pattern = simple_pattern_create(decoded_query, "|", SIMPLE_PATTERN_EXACT, false);
    }

    mcp_init_success_result(mcpc, id);

    // Add the filtered contexts
    rrdcontext_context_registry_json_mcp_array(mcpc->result, pattern);
    
    // Add instructions
    buffer_json_member_add_string(mcpc->result, "instructions",
        "Additional information per context (like title, dimensions, unit, label\n"
        "keys and possible values, the list of nodes collecting it, and its retention)\n"
        "can be obtained by reading URIs in the format 'nd://contexts/{context}'\n"
        "(like nd://context/system.cpu.user).\n\n"
        "You can search contexts using glob-like patterns using the 'like' parameter:\n"
        "nd://contexts?like=*sql*|*db*|*redis*|*mongo*\n"
        "to find postgresql, mysql, mariadb and mongodb related contexts.\n\n"
        "For a high-level overview of monitoring categories, use nd://context-categories");
    
    buffer_json_finalize(mcpc->result);
    
    if (pattern)
        simple_pattern_free(pattern);

    return MCP_RC_OK;
}

// Implementation of the context categories resource read function
static MCP_RETURN_CODE mcp_resource_read_context_categories(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || !params || id == 0) return MCP_RC_INTERNAL_ERROR;

    // Extract URI from params to check for query parameters
    struct json_object *uri_obj = NULL;
    json_object_object_get_ex(params, "uri", &uri_obj);
    const char *uri = json_object_get_string(uri_obj);
    
    SIMPLE_PATTERN *pattern = NULL;
    
    // Check if we have a query parameter
    if (uri && strstr(uri, "?like=")) {
        const char *like_param = strstr(uri, "?like=") + 6; // Skip past "?like="
        
        // Decode the query parameter
        const char *decoded_query = mcp_uri_decode(mcpc, like_param);
        
        // Create a simple pattern
        if (decoded_query && *decoded_query)
            pattern = simple_pattern_create(decoded_query, "|", SIMPLE_PATTERN_EXACT, false);
    }

    mcp_init_success_result(mcpc, id);

    // Add the filtered context categories
    rrdcontext_context_registry_json_mcp_categories_array(mcpc->result, pattern);
    
    // Add instructions
    buffer_json_member_add_string(mcpc->result, "instructions",
        "Context categories provide a high-level overview of what's being monitored.\n"
        "Each category represents a group of related contexts (e.g., 'system.cpu' for CPU metrics).\n\n"
        "To explore all contexts within a specific category, use the pattern:\n"
        "nd://contexts?like={category}.*\n\n"
        "For example, if the cateogy is 'redis' to see all Redis-related contexts:\n"
        "nd://contexts?like=redis.*\n\n"
        "You can search categories using glob-like patterns with the 'like' parameter:\n"
        "nd://context-categories?like=*sql*|*db*|*mongo*\n"
        "to find postgresql, mysql, mariadb and mongodb related categories.");
    
    buffer_json_finalize(mcpc->result);
    
    if (pattern)
        simple_pattern_free(pattern);

    return MCP_RC_OK;
}

// Size estimation functions for resources
static size_t mcp_resource_contexts_size(void) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    rrdcontext_context_registry_json_mcp_array(wb, NULL);
    buffer_json_finalize(wb);
    return buffer_strlen(wb);
}

static size_t mcp_resource_context_categories_size(void) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    rrdcontext_context_registry_json_mcp_categories_array(wb, NULL);
    buffer_json_finalize(wb);
    return buffer_strlen(wb);
}

// Static array of all available resources
static const MCP_RESOURCE mcp_resources[] = {
    {
        .name = "contexts",
        .uri = "nd://contexts",
        .description =
            "Primary discovery mechanism for what's being monitored.\n"
            "Contexts are the equivalent of charts in Netdata dashboards and they are multi-node and multi-instance.\n"
            "Usually contexts have the same set of label keys and common or similar dimensions.\n"
            "Supports searches for contexts using glob-like patterns with the 'like=' parameter.\n",
        .content_type = CT_APPLICATION_JSON,
        .audience = RESOURCE_AUDIENCE_BOTH,
        .priority = 1.0,
        .read_fn = mcp_resource_read_contexts,
        .size_fn = mcp_resource_contexts_size
    },
    {
        .name = "context-categories",
        .uri = "nd://context-categories",
        .description =
            "High-level categories of contexts being monitored.\n"
            "Provides a summarized view of monitoring domains by grouping contexts by their prefix.\n"
            "Useful for getting a quick overview of what's being monitored without detailed breakdown.\n",
        .content_type = CT_APPLICATION_JSON,
        .audience = RESOURCE_AUDIENCE_BOTH,
        .priority = 0.9,
        .read_fn = mcp_resource_read_context_categories,
        .size_fn = mcp_resource_context_categories_size
    },
    // Add more resources here as they are implemented
    // Example:
    // {
    //     .name = "nodes",
    //     .uri = "nd://nodes",
    //     .description = "Infrastructure discovery...",
    //     ...
    // },
};

// Static array of all available resource templates
static const MCP_RESOURCE_TEMPLATE mcp_resource_templates[] = {
    {
        .name = "Contexts Search",
        .uri_template = "nd://contexts{?like}",
        .description = 
            "Search for monitoring contexts by matching their names against glob-like patterns.\n"
            "The 'like' parameter accepts pipe-separated patterns with wildcards\n"
            "(e.g., '?like=*sql*|*db*|*redis*|*mongo*|*{db-name}*' for common database-related contexts).",
        .content_type = CT_APPLICATION_JSON,
        .audience = RESOURCE_AUDIENCE_BOTH,
        .priority = 1.0
    },
    {
        .name = "Context Categories Search",
        .uri_template = "nd://context-categories{?like}",
        .description = 
            "Search for high-level context categories by matching their names against glob-like patterns.\n"
            "The 'like' parameter accepts pipe-separated patterns with wildcards\n"
            "(e.g., '?like=*sql*|*db*|*redis*|*mongo*|*{db-name}*' for common database-related categories).",
        .content_type = CT_APPLICATION_JSON,
        .audience = RESOURCE_AUDIENCE_BOTH,
        .priority = 0.9
    },
    // Add more templates here as they are implemented
};

// Implementation of resources/list
static MCP_RETURN_CODE mcp_resources_method_list(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || !params || !id) return MCP_RC_INTERNAL_ERROR;

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Create a resources array object
    buffer_json_member_add_array(mcpc->result, "resources");
    {
        // Iterate through our resources array and add each one
        for (size_t i = 0; i < _countof(mcp_resources); i++) {
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

            // Add size information if available
            if (resource->size_fn) {
                size_t size = resource->size_fn();
                if (size > 0) {
                    buffer_json_member_add_uint64(mcpc->result, "size", size);
                }
            }

            // Add audience annotations if specified
            if (resource->audience != 0) {
                buffer_json_member_add_object(mcpc->result, "annotations");
                {
                    buffer_json_member_add_array(mcpc->result, "audience");
                    {
                        if (resource->audience & RESOURCE_AUDIENCE_USER) {
                            buffer_json_add_array_item_string(mcpc->result, "user");
                        }

                        if (resource->audience & RESOURCE_AUDIENCE_ASSISTANT) {
                            buffer_json_add_array_item_string(mcpc->result, "assistant");
                        }
                    }
                    buffer_json_array_close(mcpc->result); // Close audience array

                    // Add priority if it's non-zero
                    if (resource->priority > 0) {
                        buffer_json_member_add_double(mcpc->result, "priority", resource->priority);
                    }
                }
                buffer_json_object_close(mcpc->result); // Close annotations object
            }

            buffer_json_object_close(mcpc->result); // Close resource object
        }
    }
    buffer_json_array_close(mcpc->result); // Close resources array

    buffer_json_finalize(mcpc->result);
    return MCP_RC_OK;
}

// Implementation of resources/read
static MCP_RETURN_CODE mcp_resources_method_read(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
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
    for (size_t i = 0; i < _countof(mcp_resources); i++) {
        const MCP_RESOURCE *resource = &mcp_resources[i];
        
        // Get the URI without query parameters for matching
        const char *query_start = strchr(uri, '?');
        size_t base_uri_length = query_start ? (size_t)(query_start - uri) : strlen(uri);
        
        // Check if the base URI matches the resource URI
        if (strlen(resource->uri) == base_uri_length && 
            strncmp(resource->uri, uri, base_uri_length) == 0) {
            
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

// Implementation of resources/templates/list
static MCP_RETURN_CODE mcp_resources_method_templates_list(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || !params || !id) return MCP_RC_INTERNAL_ERROR;

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Create a resourceTemplates array object
    buffer_json_member_add_array(mcpc->result, "resourceTemplates");
    {
        // Iterate through our templates array and add each one
        for (size_t i = 0; i < _countof(mcp_resource_templates); i++) {
            const MCP_RESOURCE_TEMPLATE *template = &mcp_resource_templates[i];

            buffer_json_add_array_item_object(mcpc->result);

            // Add required fields
            buffer_json_member_add_string(mcpc->result, "name", template->name);
            buffer_json_member_add_string(mcpc->result, "uriTemplate", template->uri_template);

            // Add optional fields
            if (template->description) {
                buffer_json_member_add_string(mcpc->result, "description", template->description);
            }

            // Convert the content_type enum to string
            const char *mime_type = content_type_id2string(template->content_type);
            if (mime_type) {
                buffer_json_member_add_string(mcpc->result, "mimeType", mime_type);
            }

            // Add audience annotations if specified
            if (template->audience != 0) {
                buffer_json_member_add_object(mcpc->result, "annotations");
                {
                    buffer_json_member_add_array(mcpc->result, "audience");
                    {
                        if (template->audience & RESOURCE_AUDIENCE_USER) {
                            buffer_json_add_array_item_string(mcpc->result, "user");
                        }

                        if (template->audience & RESOURCE_AUDIENCE_ASSISTANT) {
                            buffer_json_add_array_item_string(mcpc->result, "assistant");
                        }
                    }
                    buffer_json_array_close(mcpc->result); // Close audience array

                    // Add priority if it's non-zero
                    if (template->priority > 0) {
                        buffer_json_member_add_double(mcpc->result, "priority", template->priority);
                    }
                }
                buffer_json_object_close(mcpc->result); // Close annotations object
            }

            buffer_json_object_close(mcpc->result); // Close template object
        }
    }

    buffer_json_finalize(mcpc->result);
    return MCP_RC_OK;
}

// Implementation of resources/subscribe (transport-agnostic)
static MCP_RETURN_CODE mcp_resources_method_subscribe(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || !id || !params) return MCP_RC_INTERNAL_ERROR;
    return MCP_RC_NOT_IMPLEMENTED;
}

// Implementation of resources/unsubscribe (transport-agnostic)
static MCP_RETURN_CODE mcp_resources_method_unsubscribe(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || id == 0 || !params) return MCP_RC_INTERNAL_ERROR;
    return MCP_RC_NOT_IMPLEMENTED;
}

// Resources namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_resources_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, MCP_REQUEST_ID id) {
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
