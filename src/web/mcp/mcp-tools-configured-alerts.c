// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-configured-alerts.h"
#include "mcp-params.h"
#include "health/health_internals.h"

// Schema for list_configured_alerts - no parameters
void mcp_tool_list_configured_alerts_schema(BUFFER *buffer) {
    // Tool metadata
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "List configured alerts");
    
    buffer_json_member_add_object(buffer, "properties");
    // No properties - this tool accepts no parameters
    buffer_json_object_close(buffer); // properties
    
    buffer_json_object_close(buffer); // inputSchema
}

// Execute list_configured_alerts - no filtering, returns all prototypes
MCP_RETURN_CODE mcp_tool_list_configured_alerts_execute(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, MCP_REQUEST_ID id __maybe_unused) {
    if (!mcpc)
        return MCP_RC_ERROR;
    
    // Create a temporary buffer for the result
    CLEAN_BUFFER *t = buffer_create(0, NULL);
    size_t count = 0;

    // Build the JSON response in tabular format
    buffer_json_initialize(t, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_uint64(t, "format_version", 1);
    
    // Add header for tabular data
    buffer_json_member_add_array(t, "configured_alerts_header");
    {
        buffer_json_add_array_item_string(t, "name");
        buffer_json_add_array_item_string(t, "applies_to");
        buffer_json_add_array_item_string(t, "on");
        buffer_json_add_array_item_string(t, "summary");
        buffer_json_add_array_item_string(t, "component");
        buffer_json_add_array_item_string(t, "classification");
        buffer_json_add_array_item_string(t, "type");
        buffer_json_add_array_item_string(t, "recipient");
    }
    buffer_json_array_close(t); // configured_alerts_header
    
    // Add tabular data
    buffer_json_member_add_array(t, "configured_alerts");
    {
        // Iterate through all alert prototypes
        RRD_ALERT_PROTOTYPE *ap;
        dfe_start_read(health_globals.prototypes.dict, ap) {
            // Only use the first rule for each prototype
            if (ap) {
                buffer_json_add_array_item_array(t); // Start row
                {
                    // name
                    buffer_json_add_array_item_string(t, string2str(ap->config.name));
                    
                    // alert_type (template or alarm)
                    buffer_json_add_array_item_string(t, ap->match.is_template ? "context" : "instance");
                    
                    // on (context or instance)
                    if(ap->match.is_template)
                        buffer_json_add_array_item_string(t, string2str(ap->match.on.context));
                    else
                        buffer_json_add_array_item_string(t, string2str(ap->match.on.chart));
                    
                    // summary
                    buffer_json_add_array_item_string(t, string2str(ap->config.summary));
                    
                    // component
                    buffer_json_add_array_item_string(t, string2str(ap->config.component));
                    
                    // classification
                    buffer_json_add_array_item_string(t, string2str(ap->config.classification));
                    
                    // type
                    buffer_json_add_array_item_string(t, string2str(ap->config.type));
                    
                    // recipient
                    buffer_json_add_array_item_string(t, string2str(ap->config.recipient));
                }
                buffer_json_array_close(t); // End row
                
                count++;
            }
        }
        dfe_done(ap);
    }
    buffer_json_array_close(t); // configured_alerts
    
    // Add summary
    buffer_json_member_add_object(t, "summary");
    {
        buffer_json_member_add_uint64(t, "total_prototypes", count);
    }
    buffer_json_object_close(t); // summary
    
    buffer_json_finalize(t);
    
    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Start building a content array for the result
    buffer_json_member_add_array(mcpc->result, "content");
    {
        // Return text content for LLM compatibility
        buffer_json_add_array_item_object(mcpc->result);
        {
            buffer_json_member_add_string(mcpc->result, "type", "text");
            buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(t));
        }
        buffer_json_object_close(mcpc->result); // Close text content
    }
    buffer_json_array_close(mcpc->result);  // Close content array
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result); // Finalize the JSON
    
    return MCP_RC_OK;
}
