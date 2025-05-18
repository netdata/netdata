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
#include "database/rrd-retention.h"
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
    
    // Use rrdstats_retention_collect to get retention information
    RRDSTATS_RETENTION retention = rrdstats_retention_collect();

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
    
    // Add _meta field (optional)
    buffer_json_member_add_object(mcpc->result, "_meta");
    buffer_json_member_add_string(mcpc->result, "generator", "netdata");

    // Get current time and calculate uptimes
    time_t now = now_realtime_sec();
    time_t system_uptime_seconds = now_boottime_sec();
    time_t netdata_uptime_seconds = now - netdata_start_time;

    buffer_json_member_add_int64(mcpc->result, "timestamp", (int64_t)now);

    // Add system uptime info - both raw seconds and human-readable format
    char human_readable[128];
    duration_snprintf_time_t(human_readable, sizeof(human_readable), system_uptime_seconds);

    buffer_json_member_add_object(mcpc->result, "system_uptime");
    buffer_json_member_add_int64(mcpc->result, "seconds", (int64_t)system_uptime_seconds);
    buffer_json_member_add_string(mcpc->result, "human", human_readable);
    buffer_json_object_close(mcpc->result); // Close system_uptime

    // Add netdata uptime info - both raw seconds and human-readable format
    duration_snprintf_time_t(human_readable, sizeof(human_readable), netdata_uptime_seconds);

    buffer_json_member_add_object(mcpc->result, "netdata_uptime");
    buffer_json_member_add_int64(mcpc->result, "seconds", (int64_t)netdata_uptime_seconds);
    buffer_json_member_add_string(mcpc->result, "human", human_readable);
    buffer_json_object_close(mcpc->result); // Close netdata_uptime

    // Add infrastructure statistics to metadata
    buffer_json_member_add_object(mcpc->result, "infrastructure");

    // Add nodes statistics
    buffer_json_member_add_object(mcpc->result, "nodes");
    buffer_json_member_add_int64(mcpc->result, "total", metadata.nodes.total);
    buffer_json_member_add_int64(mcpc->result, "receiving_from_children", metadata.nodes.receiving);
    buffer_json_member_add_int64(mcpc->result, "sending_to_next_parent", metadata.nodes.sending);
    buffer_json_member_add_int64(mcpc->result, "archived_but_available_for_queries", metadata.nodes.archived);
    buffer_json_member_add_string(mcpc->result, "info", "Nodes (or hosts, or servers, or devices) are Netdata Agent installations or virtual Netdata nodes or SNMP devices.");
    buffer_json_object_close(mcpc->result); // Close nodes

    // Add metrics statistics
    buffer_json_member_add_object(mcpc->result, "metrics");
    buffer_json_member_add_int64(mcpc->result, "currently_being_collected", metadata.metrics.collected);
    buffer_json_member_add_int64(mcpc->result, "total_available_for_queries", metadata.metrics.available);
    buffer_json_member_add_string(mcpc->result, "info", "Metrics are unique time-series in the Netdata time-series database.");
    buffer_json_object_close(mcpc->result); // Close metrics

    // Add instances statistics
    buffer_json_member_add_object(mcpc->result, "instances");
    buffer_json_member_add_int64(mcpc->result, "currently_being_collected", metadata.instances.collected);
    buffer_json_member_add_int64(mcpc->result, "total_available_for_queries", metadata.instances.available);
    buffer_json_member_add_string(mcpc->result, "info", "Instances are collections of metrics referring to a component (system, disk, network interface, application, process, container, etc).");
    buffer_json_object_close(mcpc->result); // Close instances

    // Add contexts statistics
    buffer_json_member_add_object(mcpc->result, "contexts");
    buffer_json_member_add_int64(mcpc->result, "unique_across_all_nodes", metadata.contexts.unique);
    buffer_json_member_add_string(mcpc->result, "info", "Contexts are distinct charts shown on the Netdata dashboards, like system.cpu (system CPU utilization), or net.net (network interfaces bandwidth). When monitoring applications, the context usually includes the application name.");
    buffer_json_object_close(mcpc->result); // Close contexts

    // Add retention information
    if (retention.storage_tiers > 0) {
        buffer_json_member_add_object(mcpc->result, "retention");
        buffer_json_member_add_array(mcpc->result, "tiers");

        for (size_t i = 0; i < retention.storage_tiers; i++) {
            RRD_STORAGE_TIER *tier_info = &retention.tiers[i];

            // Skip empty tiers
            if (tier_info->metrics == 0 && tier_info->samples == 0)
                continue;

            buffer_json_add_array_item_object(mcpc->result);

            // Add basic tier info
            buffer_json_member_add_int64(mcpc->result, "tier", tier_info->tier);
            buffer_json_member_add_string(mcpc->result, "backend",
                tier_info->backend == STORAGE_ENGINE_BACKEND_DBENGINE ? "dbengine" :
                tier_info->backend == STORAGE_ENGINE_BACKEND_RRDDIM ? "ram" : "unknown");
            buffer_json_member_add_int64(mcpc->result, "granularity", tier_info->group_seconds);
            buffer_json_member_add_string(mcpc->result, "granularity_human", tier_info->granularity_human);

            // Add metrics info
            buffer_json_member_add_int64(mcpc->result, "metrics", tier_info->metrics);
            buffer_json_member_add_int64(mcpc->result, "samples", tier_info->samples);

            // Add storage info when available
            if (tier_info->disk_max > 0) {
                buffer_json_member_add_int64(mcpc->result, "disk_used", tier_info->disk_used);
                buffer_json_member_add_int64(mcpc->result, "disk_max", tier_info->disk_max);
                // Format disk_percent to have only 2 decimal places
                double rounded_percent = floor(tier_info->disk_percent * 100.0 + 0.5) / 100.0;
                buffer_json_member_add_double(mcpc->result, "disk_percent", rounded_percent);
            }

            // Add retention info
            if (tier_info->retention > 0) {
                buffer_json_member_add_int64(mcpc->result, "first_time_s", tier_info->first_time_s);
                buffer_json_member_add_int64(mcpc->result, "last_time_s", tier_info->last_time_s);
                buffer_json_member_add_int64(mcpc->result, "retention", tier_info->retention);
                buffer_json_member_add_string(mcpc->result, "retention_human", tier_info->retention_human);

                if (tier_info->requested_retention > 0) {
                    buffer_json_member_add_int64(mcpc->result, "requested_retention", tier_info->requested_retention);
                    buffer_json_member_add_string(mcpc->result, "requested_retention_human", tier_info->requested_retention_human);
                }

                if (tier_info->expected_retention > 0) {
                    buffer_json_member_add_int64(mcpc->result, "expected_retention", tier_info->expected_retention);
                    buffer_json_member_add_string(mcpc->result, "expected_retention_human", tier_info->expected_retention_human);
                }
            }

            buffer_json_object_close(mcpc->result); // Close tier object
        }

        buffer_json_array_close(mcpc->result); // Close tiers array
        buffer_json_member_add_string(mcpc->result, "info", "Metrics retention information for each storage tier in the Netdata database.\nHigher tiers can provide min, max, average, sum and anomaly rate with the same accuracy as tier 0.\nTiers are automatically selected during query.");
        buffer_json_object_close(mcpc->result); // Close retention
    }

    buffer_json_object_close(mcpc->result); // Close infrastructure
    buffer_json_object_close(mcpc->result); // Close _meta
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result);     // Finalize JSON
    
    return MCP_RC_OK;
}
