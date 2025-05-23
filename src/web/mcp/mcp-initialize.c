// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP Initialize Method
 * 
 * The initialize method is a core part of the Model Context Protocol (MCP),
 * serving as the initial handshake between client and server.
 * 
 * According to the MCP specification:
 * 
 * 1. Purpose:
 *    - Establishes the protocol version to use for communication
 *    - Provides information about server capabilities
 *    - Exchanges client and server metadata
 *    - Sets up the foundation for subsequent interactions
 * 
 * 2. Protocol flow:
 *    - The client sends an initialize request with its supported protocol version
 *    - The server responds with its capabilities and selected protocol version
 *    - After successful initialization, other methods become available
 * 
 * 3. Key components in the response:
 *    - protocolVersion: The protocol version the server will use
 *    - capabilities: A structured object describing supported features
 *    - serverInfo: Information about the server implementation
 * 
 * This method must be called before any other MCP method, and handles
 * protocol version negotiation and capability discovery.
 */

#include "mcp-initialize.h"
#include "database/rrd-metadata.h"
#include "daemon/common.h"

// Initialize handler - provides information about what's available (transport-agnostic)
MCP_RETURN_CODE mcp_method_initialize(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc) {
        buffer_strcat(mcpc->error, "Invalid MCP client context");
        return MCP_RC_ERROR;
    }

    // Extract client's requested protocol version
    struct json_object *protocol_version_obj = NULL;
    if (json_object_object_get_ex(params, "protocolVersion", &protocol_version_obj)) {
        const char *version_str = json_object_get_string(protocol_version_obj);
        
        // Convert to our enum
        mcpc->protocol_version = MCP_PROTOCOL_VERSION_2id(version_str);
        
        // If unknown version, default to the latest we support
        if (mcpc->protocol_version == MCP_PROTOCOL_VERSION_UNKNOWN) {
            mcpc->protocol_version = MCP_PROTOCOL_VERSION_LATEST;
        }
    } else {
        // No version specified, default to oldest version for compatibility
        mcpc->protocol_version = MCP_PROTOCOL_VERSION_2024_11_05;
    }

    netdata_log_debug(D_MCP, "MCP initialize request from client %s version %s, protocol version %s",
                      string2str(mcpc->client_name), string2str(mcpc->client_version),
                      MCP_PROTOCOL_VERSION_2str(mcpc->protocol_version));

    // Initialize result buffer with JSON structure
    mcp_init_success_result(mcpc, id);
    
    // Use rrdstats_metadata_collect to get infrastructure statistics
    RRDSTATS_METADATA metadata = rrdstats_metadata_collect();

    // Add protocol version based on what client requested
    buffer_json_member_add_string(mcpc->result, "protocolVersion", 
                                 MCP_PROTOCOL_VERSION_2str(mcpc->protocol_version));

    // Add server info object
    buffer_json_member_add_object(mcpc->result, "serverInfo");
    buffer_json_member_add_string(mcpc->result, "name", "Netdata");
    buffer_json_member_add_string(mcpc->result, "version", NETDATA_VERSION);
    buffer_json_object_close(mcpc->result); // Close serverInfo
    
    // Add capabilities object according to MCP standard
    buffer_json_member_add_object(mcpc->result, "capabilities");
    
    // Tools capabilities
    buffer_json_member_add_object(mcpc->result, "tools");
    buffer_json_member_add_boolean(mcpc->result, "listChanged", false);
    buffer_json_member_add_boolean(mcpc->result, "asyncExecution", true);
    buffer_json_member_add_boolean(mcpc->result, "batchExecution", true);
    buffer_json_object_close(mcpc->result); // Close tools

    // Resources capabilities
    buffer_json_member_add_object(mcpc->result, "resources");
    buffer_json_member_add_boolean(mcpc->result, "listChanged", true);
    buffer_json_member_add_boolean(mcpc->result, "subscribe", true);
    buffer_json_object_close(mcpc->result); // Close resources

    // Prompts capabilities
    buffer_json_member_add_object(mcpc->result, "prompts");
    buffer_json_member_add_boolean(mcpc->result, "listChanged", false);
    buffer_json_object_close(mcpc->result); // Close prompts

    // Notification capabilities
    buffer_json_member_add_object(mcpc->result, "notifications");
    buffer_json_member_add_boolean(mcpc->result, "push", true);
    buffer_json_member_add_boolean(mcpc->result, "subscription", true);
    buffer_json_object_close(mcpc->result); // Close notifications

    // Add logging capabilities
    buffer_json_member_add_object(mcpc->result, "logging");
    buffer_json_object_close(mcpc->result); // Close logging

    // Add version-specific capabilities
    if (mcpc->protocol_version >= MCP_PROTOCOL_VERSION_2025_03_26) {
        // Add completions capability - new in 2025-03-26
        buffer_json_member_add_object(mcpc->result, "completions");
        buffer_json_object_close(mcpc->result); // Close completions
    }
    
    buffer_json_object_close(mcpc->result); // Close capabilities
    
    // Add dynamic instructions based on server profile
    char instructions[1024];

    const char *common =
        "Use the resources to identify the systems, components and applications being monitored,\n"
        "and the alerts that have been configured.\n"
        "\n"
        "Use the tools to perform queries on metrics and logs, seek for outliers and anomalies,\n"
        "perform root cause analysis and get live information about processes, network connections,\n"
        "containers, VMs, systemd/windows services, sensors, kubernetes clusters, and more.\n"
        "\n"
        "Tools can also help in investigating currently raised alerts and their past transitions.";

    // Determine server role based on metadata
    if (metadata.nodes.total > 1) {
        // This is a parent node with child nodes streaming to it
        snprintfz(instructions, sizeof(instructions),
            "This is a Netdata Parent Server hosting metrics and logs for %zu node%s.\n\n%s",
            metadata.nodes.total, (metadata.nodes.total == 1) ? "" : "s", common);
    }
    else {
        // This is a standalone server
        snprintfz(instructions, sizeof(instructions),
            "This is Netdata on a Standalone Server.\n\n%s", common);
    }

    buffer_json_member_add_string(mcpc->result, "instructions", instructions);
    
    // Add _meta field (optional) - empty as requested
    buffer_json_member_add_object(mcpc->result, "_meta");
    buffer_json_object_close(mcpc->result); // Close _meta
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result);     // Finalize JSON
    
    return MCP_RC_OK;
}
