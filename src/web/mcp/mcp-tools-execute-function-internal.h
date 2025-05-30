// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_TOOLS_EXECUTE_FUNCTION_INTERNAL_H
#define NETDATA_MCP_TOOLS_EXECUTE_FUNCTION_INTERNAL_H

#include "mcp.h"
#include "database/contexts/rrdcontext.h"

// Pagination units enumeration - only supported types enable cursor pagination
typedef enum mcp_pagination_units {
    MCP_PAGINATION_UNITS_UNKNOWN = 0,      // Unknown units - pagination disabled
    MCP_PAGINATION_UNITS_TIMESTAMP_USEC,   // Microsecond timestamps
} MCP_PAGINATION_UNITS;

// Define operator types for faster comparison
typedef enum {
    OP_EQUALS,           // ==
    OP_NOT_EQUALS,       // != or <>
    OP_LESS,             // <
    OP_LESS_EQUALS,      // <=
    OP_GREATER,          // >
    OP_GREATER_EQUALS,   // >=
    OP_MATCH,            // simple pattern
    OP_NOT_MATCH,        // a negative match of a simple pattern
    OP_UNKNOWN           // unknown operator
} OPERATOR_TYPE;

// Maximum number of conditions we expect to handle
#define MAX_CONDITIONS 20
#define MAX_COLUMNS 300  // Maximum number of columns we can handle
#define MAX_SELECTED_COLUMNS 100  // Maximum number of columns that can be selected

// Result status for table processing
typedef enum {
    MCP_TABLE_OK,                                    // Success
    MCP_TABLE_ERROR_INVALID_CONDITIONS,              // Condition format/parsing error
    MCP_TABLE_ERROR_NO_MATCHES_WITH_MISSING_COLUMNS, // No matches, some columns not found
    MCP_TABLE_ERROR_NO_MATCHES,                      // No matches with valid columns
    MCP_TABLE_ERROR_INVALID_SORT_ORDER,              // Invalid sort order parameter
    MCP_TABLE_ERROR_COLUMNS_NOT_FOUND,               // Requested columns not found
    MCP_TABLE_ERROR_SORT_COLUMN_NOT_FOUND,           // Sort column not found
    MCP_TABLE_ERROR_TOO_MANY_COLUMNS,                // Exceeds MAX_COLUMNS
    MCP_TABLE_NOT_JSON,                              // Response is not valid JSON
    MCP_TABLE_NOT_PROCESSABLE,                       // JSON but not a processable table format
    MCP_TABLE_EMPTY_RESULT,                          // Function returned no rows
    MCP_TABLE_INFO_MISSING_COLUMNS_FOUND_RESULTS,    // Missing columns but found via wildcard
    MCP_TABLE_RESPONSE_TOO_BIG                        // Result too big, guidance added
} MCP_TABLE_RESULT_STATUS;

// Function response types
typedef enum {
    FN_TYPE_UNKNOWN = 0,
    FN_TYPE_TABLE,                     // Regular table (has_history=false)
    FN_TYPE_TABLE_WITH_HISTORY,        // Logs table (has_history=true)
    FN_TYPE_NOT_TABLE                  // Not a table format
} MCP_FUNCTION_TYPE;

// Value types for conditions
typedef enum {
    COND_VALUE_STRING,
    COND_VALUE_NUMBER,
    COND_VALUE_BOOLEAN,
    COND_VALUE_NULL
} CONDITION_VALUE_TYPE;

// Structure to hold preprocessed condition information
typedef struct condition_s {
    int column_index;                // Index of the column in the row (-1 for wildcard search)
    const char *column_name;         // Name of the column (referenced from json-c, not owned)
    OPERATOR_TYPE op;                // Operator type
    CONDITION_VALUE_TYPE v_type;     // Type of the value
    union {
        const char *v_str;           // String value (referenced from json-c, not owned)
        bool v_bool;                 // Boolean value
        double v_num;                // Numeric value (using double to handle both int and float)
    };
    SIMPLE_PATTERN *pattern;         // Pre-compiled pattern for MATCH operations (owned - must be freed)
} CONDITION;

// Structure to hold an array of conditions
typedef struct {
    CONDITION items[MAX_CONDITIONS]; // Fixed array of conditions
    size_t count;                    // Number of conditions currently in use
    bool has_missing_columns;        // True if any column was not found
} CONDITION_ARRAY;

// Structure to hold function data throughout processing
typedef struct {
    // Request context - all parsed parameters
    struct {
        // Core request data
        MCP_CLIENT *mcpc;              // The MCP client
        struct json_object *params;    // The raw parameters as given by the client
        
        // Parsed required parameters
        const char *function;          // Function name to execute
        const char *node;              // Node name/id/guid
        RRDHOST *host;                 // The resolved host
        time_t timeout;                // Timeout in seconds
        
        // Transaction tracking
        nd_uuid_t transaction_uuid;    // Transaction UUID
        char transaction[UUID_STR_LEN]; // Transaction UUID string
        
        // Authentication
        USER_AUTH *auth;               // User authentication info
        
        // Parsed optional parameters for table filtering
        struct {
            const char *column;        // Column to sort by (referenced from json-c, not owned)
            bool descending;           // true for DESC, false for ASC
        } sort;                        // Sort configuration

        size_t limit;                  // Row limit (0 = no limit)
        struct {
            const char *array[MAX_SELECTED_COLUMNS];  // Column names to include (referenced from json-c, not owned)
            size_t count;              // Number of columns selected
        } columns;                     // Selected columns

        CONDITION_ARRAY conditions;    // Preprocessed conditions
        
        // Time-based and history parameters
        time_t after;                   // Start time for the query (0 = not specified)
        time_t before;                  // End time for the query (0 = not specified)
        const char *cursor;             // Pagination cursor (MCP standard) (referenced from json-c, not owned)
        usec_t anchor;                  // Internal anchor timestamp converted from cursor (0 = not specified)
        size_t last;                    // Number of last rows (0 = not specified)
        const char *direction;          // Query direction: "forward" or "backward" (referenced from json-c, not owned)
        const char *query;              // Full-text search query (referenced from json-c, not owned)
    } request;
    
    // Pagination settings (copied from registry_entry to avoid keeping it locked)
    struct {
        bool enabled;                   // whether pagination is supported
        MCP_PAGINATION_UNITS units;     // units of the pagination column
        STRING *column;                 // column name in data (owned copy)
    } pagination;
    
    // Input data from the function
    struct {
        struct json_object *jobj;      // The parsed JSON object
        BUFFER *json;                  // The original JSON response
        MCP_FUNCTION_TYPE type;        // Type of response
        size_t rows;                   // Number of rows in original data
        size_t columns;                // Number of columns available
    } input;
    
    // Output data after processing
    struct {
        MCP_TABLE_RESULT_STATUS status; // Result of processing
        BUFFER *result;                // Response to send to the client
        size_t rows;                   // Number of rows after filtering
        size_t columns;                // Number of columns selected
    } output;
} MCP_FUNCTION_DATA;

// Function prototypes shared between execute-function and execute-function-logs

// Initialize MCP_FUNCTION_DATA structure
void mcp_functions_data_init(MCP_FUNCTION_DATA *data);

// Clean up MCP_FUNCTION_DATA structure
void mcp_functions_data_cleanup(MCP_FUNCTION_DATA *data);

// Analyze the JSON response and determine its type
MCP_FUNCTION_TYPE mcp_functions_analyze_response(struct json_object *json_obj, int *out_status);

// Convert string operator to enum type
OPERATOR_TYPE mcp_functions_string_to_operator(const char *op_str);

// Free patterns in the condition array
void mcp_functions_free_condition_patterns(CONDITION_ARRAY *condition_array);

#endif // NETDATA_MCP_TOOLS_EXECUTE_FUNCTION_INTERNAL_H