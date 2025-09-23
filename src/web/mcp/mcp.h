// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_H
#define NETDATA_MCP_H

#include "libnetdata/libnetdata.h"
#include <json-c/json.h>
#include "libnetdata/buffer/buffer.h"

// Request ID type - adapters may use 0 when no correlation is required
typedef size_t MCP_REQUEST_ID;

// MCP tool names - use these constants when referring to tools
#define MCP_TOOL_LIST_METRICS "list_metrics"
#define MCP_TOOL_GET_METRICS_DETAILS "get_metrics_details"
#define MCP_TOOL_LIST_NODES "list_nodes"
#define MCP_TOOL_GET_NODES_DETAILS "get_nodes_details"
#define MCP_TOOL_LIST_FUNCTIONS "list_functions"
#define MCP_TOOL_EXECUTE_FUNCTION "execute_function"
#define MCP_TOOL_QUERY_METRICS "query_metrics"
#define MCP_TOOL_FIND_CORRELATED_METRICS "find_correlated_metrics"
#define MCP_TOOL_FIND_ANOMALOUS_METRICS "find_anomalous_metrics"
#define MCP_TOOL_FIND_UNSTABLE_METRICS "find_unstable_metrics"
#define MCP_TOOL_LIST_RAISED_ALERTS "list_raised_alerts"
#define MCP_TOOL_LIST_ALL_ALERTS "list_running_alerts"
#define MCP_TOOL_LIST_ALERT_TRANSITIONS "list_alert_transitions"

#define MCP_INFO_TOO_MANY_CONTEXTS_GROUPED_IN_CATEGORIES                                                                \
    "The response has been grouped into categories to minimize size.\n"                                                 \
    "Next Steps: repeat the '"MCP_TOOL_LIST_METRICS"' call with a pattern to match what is interesting, "               \
    "or run '" MCP_TOOL_GET_METRICS_DETAILS "' to get more information for the contexts of interest."

#define MCP_INFO_CONTEXT_ARRAY_RESPONSE \
    "Next Steps: run the '"MCP_TOOL_GET_METRICS_DETAILS"' tool to get more information for the contexts of interest."

#define MCP_INFO_CONTEXT_NEXT_STEPS \
    "Next Steps: Query time-series data with the '"MCP_TOOL_QUERY_METRICS"' tool, using different aggregations to inspect different views:\n" \
    "   - 'group_by: dimension' will aggregate all time-series by the listed dimensions\n" \
    "   - 'group_by: instance' will aggregate all time-series by the listed instances\n" \
    "   - 'group_by: label, group_by_label: {label_key}' will aggregate by the listed label values\n" \
    "\n" \
    "Dimensions, instances and labels can also be used for filtering in '"MCP_TOOL_QUERY_METRICS"':\n" \
    "   - 'dimensions: dimension1|dimension2|*dimension*' will select only the time-series with the given dimension\n" \
    "   - 'instances: instance1|instance2|*instance*' will select only the time-series with the given instance\n" \
    "   - 'labels' can be specified in two formats:\n" \
    "      • String format: 'labels: key1:value1|key1:value2|key2:value3' (values with same key are ORed, different keys are ANDed)\n" \
    "      • Structured format: 'labels: {\"key1\": [\"value1\", \"value2\"], \"key2\": \"value3\"}' (array values are ORed, different keys are ANDed)"

// MCP default values for all tools
#define MCP_DEFAULT_AFTER_TIME              (-3600) // 1 hour ago
#define MCP_DEFAULT_BEFORE_TIME             0    // now
#define MCP_DEFAULT_TIMEOUT_WEIGHTS         300  // 5 minutes
#define MCP_METADATA_CARDINALITY_LIMIT      50   // For metadata queries
#define MCP_DATA_CARDINALITY_LIMIT          10   // For data queries
#define MCP_WEIGHTS_CARDINALITY_LIMIT       50   // For weights queries (minimum is 30)
#define MCP_METADATA_CARDINALITY_LIMIT_MAX  500  // For metadata queries
#define MCP_DATA_CARDINALITY_LIMIT_MAX      500  // For data queries
#define MCP_WEIGHTS_CARDINALITY_LIMIT_MAX   500  // For weights queries
#define MCP_ALERTS_CARDINALITY_LIMIT        100  // For alert queries
#define MCP_ALERTS_CARDINALITY_LIMIT_MAX    500  // For alert queries

// MCP query info messages
#define MCP_QUERY_INFO_SUMMARY_SECTION \
    "The summary section breaks down the different sources that contribute " \
    "data to the query. Use this to detect spikes, dives, anomalies (the % of anomalous samples vs the total samples) " \
    "and evaluate the different groupings that may be beneficial for the task at hand."

#define MCP_QUERY_INFO_DATABASE_SECTION \
    "The database section provides metadata about the underlying data storage, " \
    "including retention periods and update frequencies, and data availability " \
    "across different storage tiers."

#define MCP_QUERY_INFO_VIEW_SECTION \
    "The view section provides summarized data for the visible time window. " \
    "For each dimension returned, it contains the minimum, maximum, and average values, " \
    "the anomaly rate (% of anomalous samples vs total samples) and contribution percentages, " \
    "across all points."

#define MCP_QUERY_INFO_RESULT_SECTION \
    "The 'result' section contains the actual time-series data points.\n" \
    "Each point of each dimension is represented as an array of 3 values:\n" \
    "  a) the value itself, aggregated as requested\n" \
    "  b) the point anomaly rate percentage (% of anomalous samples vs total samples)\n" \
    "  c) the point annotations, a combined bitmap of 1+2+4, where:\n" \
    "     1 = empty data, value should be ignored\n" \
    "     2 = counter has been reset or overflown, value may not be accurate\n" \
    "     4 = partial data, at least one of the sources aggregated had gaps at that time\n" \
    "Summarized data across the entire time-frame is provided at the 'view' section."

// MCP protocol versions
typedef enum {
    MCP_PROTOCOL_VERSION_UNKNOWN = 0,
    MCP_PROTOCOL_VERSION_2024_11_05 = 20241105, // Using numeric date format for natural ordering
    MCP_PROTOCOL_VERSION_2025_03_26 = 20250326,
    // Add future versions here
    
    // Always keep this pointing to the latest version
    MCP_PROTOCOL_VERSION_LATEST = MCP_PROTOCOL_VERSION_2025_03_26
} MCP_PROTOCOL_VERSION;
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(MCP_PROTOCOL_VERSION);

// Content types (for messages and tool responses)
typedef enum {
    MCP_CONTENT_TYPE_TEXT = 0,
    MCP_CONTENT_TYPE_IMAGE = 1,
    MCP_CONTENT_TYPE_AUDIO = 2, // New in 2025-03-26
} MCP_CONTENT_TYPE;

// Logging levels (as defined in MCP schema)
typedef enum {
    MCP_LOGGING_LEVEL_UNKNOWN = 0,
    MCP_LOGGING_LEVEL_DEBUG,
    MCP_LOGGING_LEVEL_INFO,
    MCP_LOGGING_LEVEL_NOTICE,
    MCP_LOGGING_LEVEL_WARNING,
    MCP_LOGGING_LEVEL_ERROR,
    MCP_LOGGING_LEVEL_CRITICAL,
    MCP_LOGGING_LEVEL_ALERT,
    MCP_LOGGING_LEVEL_EMERGENCY
} MCP_LOGGING_LEVEL;
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(MCP_LOGGING_LEVEL);

// Forward declarations for transport-specific types
struct websocket_server_client;
struct web_client;

// Transport types for MCP
typedef enum {
    MCP_TRANSPORT_UNKNOWN = 0,
    MCP_TRANSPORT_WEBSOCKET,
    MCP_TRANSPORT_HTTP,
    MCP_TRANSPORT_SSE,
    // Add more as needed
} MCP_TRANSPORT;

// Transport capabilities
typedef enum {
    MCP_CAPABILITY_NONE = 0,
    MCP_CAPABILITY_ASYNC_COMMUNICATION = (1 << 0),  // Can send messages at any time
    MCP_CAPABILITY_SUBSCRIPTIONS = (1 << 1),        // Supports subscriptions
    MCP_CAPABILITY_NOTIFICATIONS = (1 << 2),        // Supports notifications
    // Add more as needed
} MCP_CAPABILITY;

// Return codes for MCP functions
typedef enum {
    MCP_RC_OK = 0,             // Success, result buffer contains valid response
    MCP_RC_ERROR = 1,          // Generic error, error buffer contains message
    MCP_RC_INVALID_PARAMS = 2, // Invalid parameters in request
    MCP_RC_NOT_FOUND = 3,      // Resource or method not found
    MCP_RC_INTERNAL_ERROR = 4, // Internal server error
    MCP_RC_NOT_IMPLEMENTED = 5, // Method not implemented
    MCP_RC_BAD_REQUEST = 6      // Bad or malformed request
    // Can add more specific errors as needed
} MCP_RETURN_CODE;
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(MCP_RETURN_CODE);

// Response handling context
typedef struct mcp_client {
    // Transport type and capabilities
    MCP_TRANSPORT transport;
    MCP_CAPABILITY capabilities;
    
    // Protocol version (detected during initialization)
    MCP_PROTOCOL_VERSION protocol_version;
    
    // Client state
    bool ready;                                    // Set to true when client is ready for normal operations
    
    // Transport-specific context
    union {
        struct websocket_server_client *websocket;  // WebSocket client
        struct web_client *http;                    // HTTP client
        void *generic;                              // Generic context
    };
    
    // Authentication and authorization
    USER_AUTH *user_auth;                          // Pointer to user auth from the underlying transport
    
    // Client information
    STRING *client_name;                           // Client name (for logging, interned)
    STRING *client_version;                        // Client version (for logging, interned)
    
    // Logging configuration
    MCP_LOGGING_LEVEL logging_level;              // Current logging level set by client

    // Per-request response data
    BUFFER *error;                                 // Persistent buffer accumulating error messages
    BUFFER *result;                                // Convenience pointer to currently active response chunk
    struct mcp_response_chunk {
        BUFFER *buffer;                            // Response payload
        enum mcp_response_chunk_type {
            MCP_RESPONSE_CHUNK_JSON = 0,
            MCP_RESPONSE_CHUNK_TEXT,
            MCP_RESPONSE_CHUNK_BINARY,
        } type;                                    // Encoding hint for adapters
    } *response_chunks;
    size_t response_chunks_used;
    size_t response_chunks_size;

    // Last response status
    MCP_RETURN_CODE last_return_code;
    bool last_response_error;
} MCP_CLIENT;

// Helper function to convert string version to numeric version
MCP_PROTOCOL_VERSION mcp_protocol_version_from_string(const char *version_str);

// Helper function to convert numeric version to string version
const char *mcp_protocol_version_to_string(MCP_PROTOCOL_VERSION version);

// Create a response context for a transport session
MCP_CLIENT *mcp_create_client(MCP_TRANSPORT transport, void *transport_ctx);

// Free a response context
void mcp_free_client(MCP_CLIENT *mcpc);

// Helper functions for creating and sending JSON-RPC responses

// Response lifecycle helpers
void mcp_client_prepare_response(MCP_CLIENT *mcpc);
void mcp_client_release_response(MCP_CLIENT *mcpc);
BUFFER *mcp_response_add_json_chunk(MCP_CLIENT *mcpc, size_t initial_capacity);
BUFFER *mcp_response_add_text_chunk(MCP_CLIENT *mcpc, size_t initial_capacity);
size_t mcp_client_response_chunk_count(const MCP_CLIENT *mcpc);
const struct mcp_response_chunk *mcp_client_response_chunks(const MCP_CLIENT *mcpc);
size_t mcp_client_response_size(const MCP_CLIENT *mcpc);
void mcp_init_success_result(MCP_CLIENT *mcpc, MCP_REQUEST_ID id);
MCP_RETURN_CODE mcp_error_result(MCP_CLIENT *mcpc, MCP_REQUEST_ID id, MCP_RETURN_CODE rc);
const char *mcp_client_error_message(MCP_CLIENT *mcpc);
void mcp_client_clear_error(MCP_CLIENT *mcpc);

// Check if a capability is supported by the transport
static inline bool mcp_has_capability(MCP_CLIENT *mcpc, MCP_CAPABILITY capability) {
    return mcpc && (mcpc->capabilities & capability);
}

// Initialize the MCP subsystem
void mcp_initialize_subsystem(void);

// Transport-agnostic dispatcher (method string follows MCP namespace semantics)
MCP_RETURN_CODE mcp_dispatch_method(MCP_CLIENT *mcpc, const char *method, struct json_object *params, MCP_REQUEST_ID id);

#endif // NETDATA_MCP_H
