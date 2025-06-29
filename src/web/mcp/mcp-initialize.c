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
    if (!mcpc)
        return MCP_RC_ERROR;

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
    char instructions[8192];

    const char *instructions_template = 
        "This is %s.\n"
        "\n"
        "## NETDATA'S UNIQUE CAPABILITIES\n"
        "\n"
        "### Real-Time Anomaly Detection\n"
        "Netdata performs ML-based anomaly detection (k-means clustering) on every metric during data collection. "
        "Each sample includes its anomaly status from when it was originally collected.\n"
        "\n"
        "**Critical: Anomaly Rate Interpretation**\n"
        "- Low percentages often indicate major events, not noise\n"
        "- Time window context is essential:\n"
        "  • 1%% over 1 hour = ~36 seconds of anomalies (minor)\n"
        "  • 1%% over 1 day = ~14 minutes of anomalies (moderate)\n"
        "  • 1%% over 1 week = ~2 hours of anomalies (potentially major incident)\n"
        "- Anomalies may be concentrated in time, indicating real events\n"
        "- Always query actual metrics to see anomaly distribution across data points\n"
        "- The ML model detected these anomalies in real-time without future knowledge\n"
        "\n"
        "## TOOL ARCHITECTURE AND PATTERNS\n"
        "\n"
        "### Pattern Matching Rules\n"
        "**Discovery tools** (list_metrics, list_nodes, list_running_alerts) support patterns on their PRIMARY data:\n"
        "- `list_metrics`: patterns on metric names (e.g., 'system.*', '*nginx*')\n"
        "- `list_nodes`: patterns on hostnames (e.g., '*web*', 'prod-*')\n"
        "- Secondary parameters (nodes, metrics) require EXACT names only\n"
        "\n"
        "**Query tools** (query_metrics, find_*_metrics) require EXACT names for ALL parameters:\n"
        "- No patterns allowed - you must specify exact metric names\n"
        "- Use discovery tools first to get exact names, then query\n"
        "\n"
        "### Tool Combination Strategy\n"
        "Tools are designed to work together. Use outputs from one tool as inputs to others:\n"
        "\n"
        "**Example: Find nodes running specific services**\n"
        "```\n"
        "1. list_metrics (pattern: '*redis*') → get exact context names\n"
        "2. list_nodes (metrics: ['redis.connections', 'redis.memory']) → get only nodes running redis\n"
        "```\n"
        "\n"
        "**Example: Investigate performance issues**\n"
        "```\n"
        "1. find_anomalous_metrics (timeframe) → identify problematic metrics\n"
        "2. query_metrics (exact metric names from step 1) → see detailed data\n"
        "3. find_correlated_metrics (same timeframe) → what changed significantly during this period\n"
        "```\n"
        "\n"
        "## INVESTIGATION METHODOLOGY\n"
        "\n"
        "### Discovery Workflow\n"
        "Follow the data trail using these interactive tools:\n"
        "\n"
        "**For \"What's available\" questions:**\n"
        "- `list_metrics`: Full-text search (use 'q' parameter) or pattern matching\n"
        "- `list_nodes`: Search by hostname patterns or filter by exact metric names\n"
        "- `get_metrics_details`: Get comprehensive information about specific metrics\n"
        "\n"
        "**For incident investigation:**\n"
        "- `find_anomalous_metrics`: Discover ML-detected anomalies in any timeframe\n"
        "- `find_correlated_metrics`: Find metrics that changed significantly during a time period\n"
        "  (compares against 4x previous baseline to score changes)\n"
        "- `list_alert_transitions`: See how alerts changed state during incidents\n"
        "- `query_metrics`: Get detailed time-series data with per-point anomaly information\n"
        "\n"
        "**For current system state:**\n"
        "- `execute_function`: Get live information (processes, connections, services)\n"
        "- `list_raised_alerts`: See currently active alerts requiring attention\n"
        "\n"
        "### Investigation Flow\n"
        "1. **Start with discovery**: Use broad searches to identify relevant components\n"
        "2. **Get exact names**: Convert patterns to exact metric/node names\n"
        "3. **Query for details**: Use exact names in query tools for deep analysis\n"
        "4. **Follow connections**: When data reveals related areas, investigate them\n"
        "5. **Reach conclusions**: Stop when you have sufficient information to answer comprehensively\n"
        "\n"
        "### Tool Response Patterns\n"
        "- **Categorized responses**: When results exceed limits, tools group by category\n"
        "  Use specific patterns (e.g., 'system.*') to get full details for categories\n"
        "- **Error guidance**: Tools provide specific instructions when parameters are incorrect\n"
        "- **Next steps**: Many responses include suggested follow-up actions\n"
        "- **Batch execution**: Run multiple tools in parallel for efficiency\n"
        "\n"
        "## PRACTICAL EXAMPLES\n"
        "\n"
        "**Infrastructure discovery:**\n"
        "```\n"
        "User: \"What databases are being monitored?\"\n"
        "1. list_metrics (q: \"*mysql*|*postgres*|*redis*|*mongo*\")\n"
        "2. get_metrics_details for interesting database contexts\n"
        "3. list_nodes (metrics: exact database context names) → nodes running databases\n"
        "```\n"
        "\n"
        "**Performance troubleshooting:**\n"
        "```\n"
        "User: \"System was slow yesterday 2-4 PM\"\n"
        "1. find_anomalous_metrics (yesterday 14:00-16:00)\n"
        "2. query_metrics (exact anomalous metric names) → see concentration patterns\n"
        "3. find_correlated_metrics (same timeframe) → what changed significantly during this period\n"
        "4. execute_function (if issues persist) → check current state\n"
        "```\n"
        "\n"
        "**Service-specific analysis:**\n"
        "```\n"
        "User: \"How is nginx performing?\"\n"
        "1. list_metrics (q: \"*nginx*\") → get all nginx-related contexts\n"
        "2. list_nodes (metrics: nginx contexts) → find nginx servers\n"
        "3. query_metrics (nginx metrics, specific nodes) → analyze performance\n"
        "4. list_running_alerts (metrics: nginx contexts) → check for issues\n"
        "```\n"
        "\n"
        "Remember: Netdata's per-second resolution and real-time anomaly detection provide "
        "unprecedented visibility into system behavior. Use tool combinations to build a "
        "complete picture from discovery through detailed analysis.\n"
        "\n"
        "### Infrastructure-Wide Anomaly Correlation\n"
        "For multi-node infrastructures, this single query reveals cascading anomalies across all nodes:\n"
        "\n"
        "```\n"
        "query_metrics(\n"
        "  metric: \"anomaly_detection.dimensions\",\n"
        "  dimensions: [\"anomalous\"],\n"
        "  after: <timeframe>,\n"
        "  before: <timeframe>,\n"
        "  points: <based_on_duration>,\n"
        "  time_group: \"max\",\n"
        "  group_by: [\"node\"],\n"
        "  aggregation: \"max\"\n"
        ")\n"
        "```\n"
        "\n"
        "This returns the COUNT of dimensions (time-series) that were anomalous SIMULTANEOUSLY on each node.\n"
        "\n"
        "The resulting time-series shows anomaly propagation patterns:\n"
        "- **Simultaneous spikes across nodes** = External event (network outage, DNS, etc.)\n"
        "- **Sequential spikes with delays** = Cascading failure showing dependencies\n"
        "- **Isolated node spikes** = Node-specific issues\n"
        "\n"
        "The time-series visualization immediately reveals which nodes were affected and in what order - "
        "critical for root cause analysis in distributed systems.\n"
        "\n"
        "After identifying the cascade pattern, use find_anomalous_metrics on specific nodes/times for details.";

    // Determine server role and create complete instructions
    if (metadata.nodes.total > 1) {
        snprintfz(instructions, sizeof(instructions), instructions_template,
            "a Netdata Parent Server hosting metrics and logs for multiple nodes");
    } else {
        snprintfz(instructions, sizeof(instructions), instructions_template,
            "Netdata on a standalone server");
    }

    buffer_json_member_add_string(mcpc->result, "instructions", instructions);
    
    // Add _meta field (optional) - empty as requested
    buffer_json_member_add_object(mcpc->result, "_meta");
    buffer_json_object_close(mcpc->result); // Close _meta
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result);     // Finalize JSON
    
    return MCP_RC_OK;
}
