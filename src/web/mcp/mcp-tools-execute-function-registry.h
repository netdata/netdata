// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_TOOLS_EXECUTE_FUNCTION_REGISTRY_H
#define NETDATA_MCP_TOOLS_EXECUTE_FUNCTION_REGISTRY_H

#include "mcp.h"
#include "mcp-tools-execute-function-internal.h"
#include "libnetdata/template-enum.h"

// Registry entry TTL in seconds (10 minutes)
#define MCP_FUNCTIONS_REGISTRY_TTL 600

// Parameter type enumeration
typedef enum mcp_required_params_type {
    MCP_REQUIRED_PARAMS_TYPE_SELECT = 0,
    MCP_REQUIRED_PARAMS_TYPE_MULTISELECT
} MCP_REQUIRED_PARAMS_TYPE;

ENUM_STR_DEFINE_FUNCTIONS_EXTERN(MCP_REQUIRED_PARAMS_TYPE)

// Parameter option structure
typedef struct mcp_function_param_option {
    STRING *id;
    STRING *name;
    STRING *info;  // Additional information about the option (e.g., file count, size, coverage)
} MCP_FUNCTION_PARAM_OPTION;

// Parameter structure
typedef struct mcp_function_param {
    STRING *id;
    STRING *name;
    STRING *help;
    MCP_REQUIRED_PARAMS_TYPE type;
    bool unique_view;
    size_t options_count;
    MCP_FUNCTION_PARAM_OPTION *options;
} MCP_FUNCTION_PARAM;

// Registry entry structure
typedef struct mcp_function_registry_entry {
    RW_SPINLOCK spinlock;               // Read/write lock for thread-safe access
    SPINLOCK update_spinlock;           // Spinlock to coordinate updates without blocking readers
    MCP_FUNCTION_TYPE type;             // Function type (table, table with history, etc.)
    bool has_history;                   // whether the function supports history
    int update_every;                   // update interval in seconds
    STRING *help;                       // help text
    int version;                        // Function version (v3+ supports POST)
    bool supports_post;                 // true if version >= 3
    size_t required_params_count;       // number of required parameters
    MCP_FUNCTION_PARAM *required_params; // array of required parameters
    // Supported optional parameters (detected from accepted_params)
    bool has_timeframe;                 // supports after, before parameters
    bool has_anchor;                    // supports anchor parameter for pagination
    bool has_last;                      // supports last parameter (row limit)
    bool has_data_only;                 // supports data_only parameter
    bool has_direction;                 // supports direction parameter
    bool has_query;                     // supports query parameter for full-text search
    bool has_slice;                     // supports slice parameter for database-level filtering
    time_t last_update;                 // timestamp of last info update
    time_t expires;                     // expiration timestamp
} MCP_FUNCTION_REGISTRY_ENTRY;

// Initialize the functions registry
void mcp_functions_registry_init(void);

// Cleanup the functions registry
void mcp_functions_registry_cleanup(void);

// Get a registry entry for a function (read-locked)
// This will fetch info if the entry doesn't exist or has expired
// The returned entry is read-locked and MUST be released with mcp_functions_registry_release()
// Returns NULL on error (with error details in the error buffer)
MCP_FUNCTION_REGISTRY_ENTRY *mcp_functions_registry_get(RRDHOST *host, const char *function_name, BUFFER *error);

// Release a registry entry (unlocks the read lock)
// MUST be called after mcp_functions_registry_get() as soon as possible
void mcp_functions_registry_release(MCP_FUNCTION_REGISTRY_ENTRY *entry);

#endif // NETDATA_MCP_TOOLS_EXECUTE_FUNCTION_REGISTRY_H