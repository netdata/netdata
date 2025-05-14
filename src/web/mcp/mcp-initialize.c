// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-initialize.h"
#include "database/rrd-metadata.h"
#include "database/rrd-retention.h"
#include "daemon/common.h"

// Initialize handler - provides information about what's available (transport-agnostic)
int mcp_method_initialize(MCP_CLIENT *mcpc, struct json_object *params, uint64_t id) {
    if (!mcpc || id == 0) return -1;

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

    struct json_object *result = json_object_new_object();
    
    // Use rrdstats_metadata_collect to get infrastructure statistics
    RRDSTATS_METADATA metadata = rrdstats_metadata_collect();
    
    // Use rrdstats_retention_collect to get retention information
    RRDSTATS_RETENTION retention = rrdstats_retention_collect();
    
    // Add protocol version based on what client requested
    json_object_object_add(result, "protocolVersion",
                          json_object_new_string(MCP_PROTOCOL_VERSION_2str(mcpc->protocol_version)));

    // Add server info object
    struct json_object *server_info = json_object_new_object();
    json_object_object_add(server_info, "name", json_object_new_string("Netdata"));
    json_object_object_add(server_info, "version", json_object_new_string(NETDATA_VERSION));
    json_object_object_add(result, "serverInfo", server_info);
    
    // Add capabilities object according to MCP standard
    struct json_object *capabilities = json_object_new_object();
    
    // Tools capabilities
    struct json_object *tools_caps = json_object_new_object();
    json_object_object_add(tools_caps, "listChanged", json_object_new_boolean(false));
    json_object_object_add(tools_caps, "asyncExecution", json_object_new_boolean(true));
    json_object_object_add(tools_caps, "batchExecution", json_object_new_boolean(true));
    json_object_object_add(capabilities, "tools", tools_caps);
    
    // Resources capabilities
    struct json_object *resources_caps = json_object_new_object();
    json_object_object_add(resources_caps, "listChanged", json_object_new_boolean(true));
    json_object_object_add(resources_caps, "subscribe", json_object_new_boolean(true));
    json_object_object_add(capabilities, "resources", resources_caps);
    
    // Prompts capabilities
    struct json_object *prompts_caps = json_object_new_object();
    json_object_object_add(prompts_caps, "listChanged", json_object_new_boolean(false));
    json_object_object_add(capabilities, "prompts", prompts_caps);
    
    // Notification capabilities (renamed to notifications per standard)
    struct json_object *notifications_caps = json_object_new_object();
    json_object_object_add(notifications_caps, "push", json_object_new_boolean(true));
    json_object_object_add(notifications_caps, "subscription", json_object_new_boolean(true));
    json_object_object_add(capabilities, "notifications", notifications_caps);
    
    // Add logging capabilities
    struct json_object *logging_caps = json_object_new_object();
    json_object_object_add(capabilities, "logging", logging_caps);
    
    // Add version-specific capabilities
    if (mcpc->protocol_version >= MCP_PROTOCOL_VERSION_2025_03_26) {
        // Add completions capability - new in 2025-03-26
        struct json_object *completions_caps = json_object_new_object();
        json_object_object_add(capabilities, "completions", completions_caps);
    }
    
    json_object_object_add(result, "capabilities", capabilities);
    
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
    
    json_object_object_add(result, "instructions", json_object_new_string(instructions));
    
    // Add _meta field (optional)
    struct json_object *meta = json_object_new_object();
    json_object_object_add(meta, "generator", json_object_new_string("netdata"));
    
    // Get current time and calculate uptimes
    time_t now = now_realtime_sec();
    time_t system_uptime_seconds = now_boottime_sec();
    time_t netdata_uptime_seconds = now - netdata_start_time;
    
    json_object_object_add(meta, "timestamp", json_object_new_int64((int64_t)now));
    
    // Add system uptime info - both raw seconds and human-readable format
    char human_readable[128];
    duration_snprintf_time_t(human_readable, sizeof(human_readable), system_uptime_seconds);
    
    struct json_object *system_uptime = json_object_new_object();
    json_object_object_add(system_uptime, "seconds", json_object_new_int64((int64_t)system_uptime_seconds));
    json_object_object_add(system_uptime, "human", json_object_new_string(human_readable));
    json_object_object_add(meta, "system_uptime", system_uptime);
    
    // Add netdata uptime info - both raw seconds and human-readable format
    duration_snprintf_time_t(human_readable, sizeof(human_readable), netdata_uptime_seconds);
    
    struct json_object *netdata_uptime = json_object_new_object();
    json_object_object_add(netdata_uptime, "seconds", json_object_new_int64((int64_t)netdata_uptime_seconds));
    json_object_object_add(netdata_uptime, "human", json_object_new_string(human_readable));
    json_object_object_add(meta, "netdata_uptime", netdata_uptime);

    // Add infrastructure statistics to metadata
    struct json_object *infrastructure = json_object_new_object();

    // Add nodes statistics
    struct json_object *nodes = json_object_new_object();
    json_object_object_add(nodes, "total", json_object_new_int64(metadata.nodes.total));
    json_object_object_add(nodes, "receiving_from_children", json_object_new_int64(metadata.nodes.receiving));
    json_object_object_add(nodes, "sending_to_next_parent", json_object_new_int64(metadata.nodes.sending));
    json_object_object_add(nodes, "archived_but_available_for_queries", json_object_new_int64(metadata.nodes.archived));
    json_object_object_add(nodes, "info", json_object_new_string("Nodes (or hosts, or servers, or devices) are Netdata Agent installations or virtual Netdata nodes or SNMP devices."));
    json_object_object_add(infrastructure, "nodes", nodes);

    // Add metrics statistics
    struct json_object *metrics = json_object_new_object();
    json_object_object_add(metrics, "currently_being_collected", json_object_new_int64(metadata.metrics.collected));
    json_object_object_add(metrics, "total_available_for_queries", json_object_new_int64(metadata.metrics.available));
    json_object_object_add(metrics, "info", json_object_new_string("Metrics are unique time-series in the Netdata time-series database."));
    json_object_object_add(infrastructure, "metrics", metrics);

    // Add instances statistics
    struct json_object *instances = json_object_new_object();
    json_object_object_add(instances, "currently_being_collected", json_object_new_int64(metadata.instances.collected));
    json_object_object_add(instances, "total_available_for_queries", json_object_new_int64(metadata.instances.available));
    json_object_object_add(instances, "info", json_object_new_string("Instances are collections of metrics referring to a component (system, disk, network interface, application, process, container, etc)."));
    json_object_object_add(infrastructure, "instances", instances);

    // Add contexts statistics
    struct json_object *contexts = json_object_new_object();
    json_object_object_add(contexts, "unique_across_all_nodes", json_object_new_int64(metadata.contexts.unique));
    json_object_object_add(contexts, "info", json_object_new_string("Contexts are distinct charts shown on the Netdata dashboards, like system.cpu (system CPU utilization), or net.net (network interfaces bandwidth). When monitoring applications, the context usually includes the application name."));
    json_object_object_add(infrastructure, "contexts", contexts);

    // Add retention information
    if (retention.storage_tiers > 0) {
        struct json_object *retention_obj = json_object_new_object();
        struct json_object *tiers_array = json_object_new_array();

        for (size_t i = 0; i < retention.storage_tiers; i++) {
            RRD_STORAGE_TIER *tier_info = &retention.tiers[i];
            
            // Skip empty tiers
            if (tier_info->metrics == 0 && tier_info->samples == 0)
                continue;
                
            struct json_object *tier = json_object_new_object();
            
            // Add basic tier info
            json_object_object_add(tier, "tier", json_object_new_int64(tier_info->tier));
            json_object_object_add(tier, "backend", json_object_new_string(
                tier_info->backend == STORAGE_ENGINE_BACKEND_DBENGINE ? "dbengine" : 
                tier_info->backend == STORAGE_ENGINE_BACKEND_RRDDIM ? "ram" : "unknown"));
            json_object_object_add(tier, "granularity", json_object_new_int64(tier_info->group_seconds));
            json_object_object_add(tier, "granularity_human", json_object_new_string(tier_info->granularity_human));
            
            // Add metrics info
            json_object_object_add(tier, "metrics", json_object_new_int64(tier_info->metrics));
            json_object_object_add(tier, "samples", json_object_new_int64(tier_info->samples));
            
            // Add storage info when available
            if (tier_info->disk_max > 0) {
                json_object_object_add(tier, "disk_used", json_object_new_int64(tier_info->disk_used));
                json_object_object_add(tier, "disk_max", json_object_new_int64(tier_info->disk_max));
                // Format disk_percent to have only 2 decimal places
                double rounded_percent = floor(tier_info->disk_percent * 100.0 + 0.5) / 100.0;
                json_object_object_add(tier, "disk_percent", json_object_new_double(rounded_percent));
            }
            
            // Add retention info
            if (tier_info->retention > 0) {
                json_object_object_add(tier, "first_time_s", json_object_new_int64(tier_info->first_time_s));
                json_object_object_add(tier, "last_time_s", json_object_new_int64(tier_info->last_time_s));
                json_object_object_add(tier, "retention", json_object_new_int64(tier_info->retention));
                json_object_object_add(tier, "retention_human", json_object_new_string(tier_info->retention_human));
                
                if (tier_info->requested_retention > 0) {
                    json_object_object_add(tier, "requested_retention", json_object_new_int64(tier_info->requested_retention));
                    json_object_object_add(tier, "requested_retention_human", json_object_new_string(tier_info->requested_retention_human));
                }
                
                if (tier_info->expected_retention > 0) {
                    json_object_object_add(tier, "expected_retention", json_object_new_int64(tier_info->expected_retention));
                    json_object_object_add(tier, "expected_retention_human", json_object_new_string(tier_info->expected_retention_human));
                }
            }
            
            json_object_array_add(tiers_array, tier);
        }
        
        json_object_object_add(retention_obj, "tiers", tiers_array);
        json_object_object_add(retention_obj, "info", json_object_new_string("Metrics retention information for each storage tier in the Netdata database.\nHigher tiers can provide min, max, average, sum and anomaly rate with the same accuracy as tier 0.\nTiers are automatically selected during query."));
        json_object_object_add(infrastructure, "retention", retention_obj);
    }

    json_object_object_add(meta, "infrastructure", infrastructure);
    json_object_object_add(result, "_meta", meta);

    // Send success response
    int ret = mcp_send_success_response(mcpc, result, id);

    // Free the result object (it's been copied in mcp_send_success_response)
    json_object_put(result);
    
    return ret;
}
