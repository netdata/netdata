# MCP Implementation Plan

## Overview

This document outlines the complete plan for implementing the Model Context Protocol (MCP) system with clean separation between transport and business logic, supporting both HTTP and WebSocket transports.

## Architecture Overview

### Core Design Principles
1. **Transport-agnostic MCP core** - Business logic separated from transport protocols
2. **Registry-based tool system** - Single source of truth for all MCP tools
3. **Netdata-compatible authorization** - Reuse existing HTTP_ACL and HTTP_ACCESS system
4. **Multi-buffer responses** - Support ordered responses using libnetdata double-linked lists
5. **Clean job-based execution** - Each request becomes a structured job

## Phase 1 – Transport Decoupling (Current Focus)

### Goals
- Keep request parsing inside each adapter while handing a parsed `json_object *` to the core. [done]
- Transform `MCP_CLIENT` into a session container with a per-request array of `BUFFER *` chunks instead of a single result buffer and JSON-RPC metadata. [done]
- Provide helper APIs (e.g. `mcp_response_reset`, `mcp_response_add_json`, `mcp_response_add_text`, `mcp_response_finalize`) so namespace handlers build transport-neutral responses without touching envelopes. [done]
- Ensure adapters own correlation data: WebSocket keeps JSON-RPC ids, future transports can pick their own tokens. [done]
- Preserve existing namespace function signatures by passing the same `MCP_CLIENT *`, params object, and `MCP_REQUEST_ID` while changing only the response building helpers they call. [done]

### Deliverables
- Response buffer management implementation with request-level limits and ownership handled by `MCP_CLIENT`. [done]
- Updated namespace implementations (initialize, ping, tools, resources, prompts, logging, completion, etc.) to use the new helper APIs. [done]
- WebSocket adapter refactor that wraps/unwraps JSON-RPC entirely in adapter code, including batching and notifications. [done]
- Documentation updates describing the new lifecycle and expectations for adapters. [done]

### Open Questions / Checks
- Confirm memory caps for accumulated response buffers and expose configuration knobs if required. [done]
- Validate streaming semantics: adapters must never split a single `BUFFER`, but may send multiple buffers sequentially. [done]
- Identify any shared utilities (UUID helpers, auth context) that should remain in core versus adapter. [done]

Status:
- [x] Response buffer helpers implemented in mcp.c (prepare, add_json/text, finalize via buffer_json_finalize in handlers)
- [x] Namespaces updated to use helpers (initialize, ping, tools, resources, prompts, logging, completion)
- [x] WebSocket adapter wraps JSON-RPC (batching, notifications) and converts MCP response chunks to JSON-RPC payloads
- [x] Error handling unified via mcp_error_result and mcpc->error buffer

## 1. Core MCP Architecture Refactoring

### A. Job-Based Request Processing

#### MCP_REQ_JOB Structure (Transport-Agnostic)
```c
typedef struct mcp_req_job {
    // Job identification
    nd_uuid_t job_id;
    char job_id_str[UUID_STR_LEN];
    
    // Request data (pure, no transport context)
    struct json_object *params;              // Parsed JSON parameters
    const char *tool_name;                   // Tool to execute
    USER_AUTH *auth;                         // Authentication context
    
    // Response data (ordered list using libnetdata double-linked lists)
    MCP_RESPONSE_BUFFER *response_buffers;   // Head of double-linked list
    
    // Status and metadata
    int status_code;                         // Overall job status
    const char *error_message;               // Error description if failed
    bool completed;                          // Job completion status
    usec_t created_usec;                     // Creation timestamp
    usec_t completed_usec;                   // Completion timestamp
    
    // Pagination support
    const char *next_cursor;                 // For paginated responses
} MCP_REQ_JOB;
```

#### MCP_RESPONSE_BUFFER Structure (Using BUFFER's Built-in HTTP Metadata)
```c
typedef struct mcp_response_buffer {
    BUFFER *buffer;                          // Uses existing BUFFER with HTTP metadata built-in
    const char *response_type;               // "text", "error", "data", etc.
    
    // Double-linked list support using libnetdata macros
    struct mcp_response_buffer *prev;
    struct mcp_response_buffer *next;
} MCP_RESPONSE_BUFFER;
```

### B. Adapter-Specific Job Structures

#### HTTP Adapter Job
```c
typedef struct mcp_http_adapter_job {
    MCP_REQ_JOB req;                         // Core MCP job (no MCP_CLIENT field!)
    
    // HTTP-specific data
    struct web_client *web_client;           // HTTP client context
    const char *url_path;                    // Original URL path
    const char *query_string;                // URL query parameters
} MCP_HTTP_ADAPTER_JOB;
```

#### WebSocket/JSON-RPC Adapter Job  
```c
typedef struct mcp_jsonrpc_adapter_job {
    MCP_REQ_JOB req;                         // Core MCP job
    
    // JSON-RPC/WebSocket specific data
    MCP_CLIENT *mcpc;                        // WebSocket client context (adapter manages this)
    uint64_t jsonrpc_id;                     // JSON-RPC request ID
    const char *jsonrpc_method;              // Original method name
} MCP_JSONRPC_ADAPTER_JOB;
```

**Status**: 
- [ ] Implement MCP_REQ_JOB structure
- [ ] Implement MCP_RESPONSE_BUFFER with libnetdata double-linked list support
- [ ] Create adapter job structures
- [ ] Implement job lifecycle management functions

## 2. Registry-Based Tool System

### A. Tool Registry Structure (Following web_api_command Pattern)

```c
typedef struct mcp_tool_registry_entry {
    // Tool identification (similar to web_api_command)
    const char *name;                    // Tool name (e.g., "execute_function")
    uint32_t hash;                       // Hash for fast lookup (like api_commands_v3)
    
    // Authorization (following Netdata pattern exactly)
    HTTP_ACL acl;                        // ACL requirements (e.g., HTTP_ACL_FUNCTIONS)
    HTTP_ACCESS access;                  // Access level requirements
    
    // Execution
    int (*execute)(MCP_REQ_JOB *job);    // Function pointer (similar to callback)
    
    // MCP-specific metadata
    MCP_NAMESPACE namespace;             // Which MCP namespace
    const char *title;                   // Human-readable title
    const char *description;             // Tool description
    const char *input_schema_json;       // JSON schema for parameters (static string)
    
    // Feature flags
    bool supports_pagination;           // Whether tool supports cursor pagination
    bool supports_streaming;            // Whether tool supports streaming responses (future)
    
    // Caching hints for adapters
    bool cacheable_schema;              // Whether schema responses can be cached
    time_t schema_cache_duration;       // How long to cache schema responses
} MCP_TOOL_REGISTRY_ENTRY;
```

### B. Registry Implementation

#### Global Static Registry (Following api_commands_v3 Pattern)
```c
MCP_TOOL_REGISTRY_ENTRY mcp_tools_registry[] = {
    // Function execution tools
    {
        .name = "execute_function",
        .hash = 0,  // Will be calculated on init like api_commands_v3
        .acl = HTTP_ACL_FUNCTIONS,                    // Same as existing function APIs
        .access = HTTP_ACCESS_ANONYMOUS_DATA,         // Same as api_v1_function
        .execute = mcp_tool_execute_function,
        .namespace = MCP_NAMESPACE_TOOLS,
        .title = "Execute Netdata Function",
        .description = "Execute live data collection functions on nodes",
        .input_schema_json = MCP_EXECUTE_FUNCTION_SCHEMA_JSON,
        .supports_pagination = true,
        .supports_streaming = false,
        .cacheable_schema = true,
        .schema_cache_duration = 3600,
    },
    // ... more tools
    
    // Terminator (like api_commands_v3)
    { .name = NULL }
};
```

#### Registry Access Functions
```c
// Initialize registry (calculate hashes like web_client_api_request_v3)
void mcp_tools_registry_init(void);

// Tool lookup (similar to web_client_api_request_vX)
const MCP_TOOL_REGISTRY_ENTRY *mcp_find_tool(const char *tool_name);

// Get tools by namespace
const MCP_TOOL_REGISTRY_ENTRY **mcp_get_tools_by_namespace(MCP_NAMESPACE namespace, size_t *count);
```

**Status**: 
- [ ] Define MCP_TOOL_REGISTRY_ENTRY structure
- [ ] Implement static registry array with all current tools
- [ ] Implement registry initialization and lookup functions
- [ ] Add authorization checking using HTTP_ACL/HTTP_ACCESS

## 3. Transport Adapters

### A. HTTP Adapter (Integrated with Netdata Web Server)

#### HTTP Route Registration
```c
// HTTP adapter decides its own URL structure
int mcp_http_adapter_init_routes(void) {
    // Direct tool execution endpoints
    web_client_api_request_v3_register("/api/v3/mcp/execute_function", mcp_http_handle_execute_function);
    web_client_api_request_v3_register("/api/v3/mcp/query_metrics", mcp_http_handle_query_metrics);
    
    // Generic endpoints using registry
    web_client_api_request_v3_register("/api/v3/mcp/tools", mcp_http_handle_tools_list);
    web_client_api_request_v3_register("/api/v3/mcp/tools/*/call", mcp_http_handle_tool_call);
    web_client_api_request_v3_register("/api/v3/mcp/tools/*/schema", mcp_http_handle_tool_schema);
    
    return 0;
}
```

#### Authorization Integration (Following Netdata Pattern Exactly)
```c
// Generic tool execution using registry (like web_client_api_request_vX)
int mcp_http_handle_tool_call(RRDHOST *host, struct web_client *w, char *url) {
    const char *tool_name = extract_tool_name_from_url(url);
    
    // Look up in registry
    const MCP_TOOL_REGISTRY_ENTRY *tool = mcp_find_tool(tool_name);
    if (!tool) {
        return web_client_api_request_v1_info_fill_buffer(host, w, "Tool not found");
    }
    
    // Check ACL and access (following Netdata pattern exactly)
    if(tool->acl != HTTP_ACL_NOCHECK) {
        if(!(w->acl & tool->acl)) {
            web_client_permission_denied_acl(w);
            return HTTP_RESP_FORBIDDEN;
        }
        
        if(tool->access != HTTP_ACCESS_NONE) {
            if(!web_client_can_access_with_auth(w, tool->access)) {
                web_client_permission_denied_access(w, tool->access);
                return HTTP_ACCESS_PERMISSION_DENIED_HTTP_CODE(tool->access);
            }
        }
    }
    
    // Execute tool
    // ... implementation
}
```

**Status**: 
- [ ] Implement HTTP route registration
- [ ] Implement HTTP request parsing (JSON body to params)
- [ ] Implement HTTP response conversion (BUFFER list to HTTP JSON)
- [ ] Integrate with existing Netdata authorization system
- [ ] Add HTTP-specific error handling

### B. WebSocket/JSON-RPC Adapter (Manages MCP_CLIENT)

#### Adapter Responsibilities
- **MCP_CLIENT management** (WebSocket connections, stdio pipes)
- **JSON-RPC protocol** wrapping/unwrapping
- **Ping/pong handling** - MCP core not involved
- **Client notifications** 
- **Connection lifecycle**

#### JSON-RPC Implementation
```c
void mcp_jsonrpc_handle_tools_call(MCP_CLIENT *mcpc, struct json_object *request) {
    struct json_object *params = json_object_object_get(request, "params");
    const char *tool_name = json_object_get_string(json_object_object_get(params, "name"));
    
    // Look up in registry (same as HTTP)
    const MCP_TOOL_REGISTRY_ENTRY *tool = mcp_find_tool(tool_name);
    if (!tool) {
        mcp_jsonrpc_send_error(mcpc, extract_id_from_request(request), -32601, "Tool not found");
        return;
    }
    
    // Check ACL and access (same authorization as HTTP!)
    // ... authorization code
    
    // Execute (same execution path as HTTP)
    // ... execution code
}

// Adapter handles ping separately
void mcp_jsonrpc_handle_ping(MCP_CLIENT *mcpc, struct json_object *request) {
    // Adapter handles ping/pong - MCP core not involved
    uint64_t id = extract_id_from_request(request);
    mcp_jsonrpc_send_pong_response(mcpc, id);
}
```

**Status**: 
- [ ] Extract JSON-RPC code from current MCP implementation
- [ ] Move MCP_CLIENT management to WebSocket adapter
- [ ] Implement JSON-RPC request/response conversion
- [ ] Handle ping/pong and notifications in adapter
- [ ] Ensure same authorization as HTTP adapter

## 4. Core MCP Interface (Transport-Agnostic)

### A. Core Execution Function
```c
// Main MCP execution function - completely transport agnostic
int mcp_execute_tool(MCP_REQ_JOB *job);

// Response buffer management using libnetdata double-linked list macros
MCP_RESPONSE_BUFFER *mcp_job_add_response_buffer(MCP_REQ_JOB *job, BUFFER *buffer, const char *response_type);
void mcp_job_prepend_response_buffer(MCP_REQ_JOB *job, MCP_RESPONSE_BUFFER *item);
void mcp_job_append_response_buffer(MCP_REQ_JOB *job, MCP_RESPONSE_BUFFER *item);

// Helper functions for common response types
MCP_RESPONSE_BUFFER *mcp_job_add_text_response(MCP_REQ_JOB *job, const char *text, int http_status, HTTP_CONTENT_TYPE content_type);
MCP_RESPONSE_BUFFER *mcp_job_add_json_response(MCP_REQ_JOB *job, BUFFER *json_buffer);
MCP_RESPONSE_BUFFER *mcp_job_add_error_response(MCP_REQ_JOB *job, const char *error_msg, int http_status);
```

### B. Tool Implementation Interface
```c
// Tool function signature - no client context needed
typedef int (*mcp_tool_func_t)(MCP_REQ_JOB *job);

// Example simplified tool implementation
int mcp_tool_execute_function(MCP_REQ_JOB *job) {
    // Pure business logic - no transport awareness
    const char *node = json_object_get_string(json_object_object_get(job->params, "node"));
    
    if (!node) {
        mcp_job_add_error_response(job, "Missing required parameter: node", 400);
        return -1;
    }
    
    // Execute function using existing BUFFER with built-in HTTP metadata
    BUFFER *result = buffer_create(0, NULL);
    buffer_set_content_type(result, CT_APPLICATION_JSON);
    buffer_cacheable(result);
    result->expires = now_monotonic_usec() + 300 * USEC_PER_SEC;
    
    int status = execute_netdata_function(node, function, result);
    
    if (status == 0) {
        mcp_job_add_response_buffer(job, result, "text");
    } else {
        mcp_job_add_error_response(job, "Function execution failed", 500);
        buffer_free(result);
    }
    
    return status;
}
```

**Status**: 
- [ ] Implement core execution function
- [ ] Implement response buffer management with double-linked lists
- [ ] Update all existing tools to use new job interface
- [ ] Remove transport dependencies from tool implementations

## 5. Directory Structure

```
src/web/mcp/
├── core/
│   ├── mcp-registry.c/h         # Global tool registry
│   ├── mcp-job.c/h             # Job management
│   ├── mcp-response.c/h        # Response management
│   ├── mcp-tools-*.c/h        # Individual tool implementations
│   └── mcp-core.c/h           # Core execution (tool lookup + execute)
├── adapters/
│   ├── websocket/
│   │   ├── mcp-jsonrpc-adapter.c/h  # tools/list, tools/call implementation
│   │   └── mcp-client.c/h          # MCP_CLIENT management
│   └── http/
│       └── mcp-http-adapter.c/h    # HTTP routes using registry
├── schemas/
│   ├── execute_function.json       # Static schema definitions
│   ├── query_metrics.json
│   └── list_metrics.json
└── mcp.h                       # Public interfaces
```

**Status**: 
- [ ] Create directory structure
- [ ] Move existing code to appropriate locations
- [ ] Update build system for new structure

## 6. Logs Tools Implementation (Advanced Features)

### Progressive Logs Discovery Workflow

#### Specialized Logs Tools
1. **`list_logs_sources`** - Discovery: "What logs are available?"
2. **`list_logs_fields`** - Schema discovery: "What fields exist in this source?"
3. **`list_logs_fields_values`** - Facets analysis: "What values can these fields have and how common are they?"
4. **`query_logs`** - Actual log retrieval with all the power of the current system

#### Implementation Strategy
- **Tools 1-2**: Use the `info=true` calls to logs functions to get metadata
- **Tool 3**: Call logs functions with specific facets parameters, return just the facets counts
- **Tool 4**: Current full query capability

**Status**: 
- [x] Design the 4-tool architecture
- [ ] Implement `list_logs_sources`
- [ ] Implement `list_logs_fields`
- [ ] Implement `list_logs_fields_values` (the key facets tool)
- [ ] Implement `query_logs`
- [ ] Update `execute_function` to exclude logs functions
- [ ] Test the complete workflow

## 7. Implementation Phases

### Phase 1: Transport Decoupling (Priority: High)
1. Implement response buffer helpers on `MCP_CLIENT` (`mcp_response_reset/add/finalize`) with per-request limits.
2. Migrate all namespace handlers (initialize, ping, tools, resources, prompts, logging, completion, etc.) to the helper API.
3. Refactor WebSocket adapter to own JSON-RPC parsing, batching, and id correlation; keep adapters blocked while the core runs.
4. Remove JSON-RPC-specific data (Judy request-id map, envelope builders) from the core and document the new lifecycle.
5. Update docs/tests to reflect the transport-neutral contract.

### Phase 2: Additional Transports (Priority: High)
1. Implement streamable HTTP adapter using the new buffer array (one `MCP_CLIENT` per HTTP connection).
2. Implement SSE adapter reusing the same response chunks without splitting buffers.
3. Share adapter scaffolding (factory helpers, auth wiring) and add coverage for reconnection scenarios.
4. Evaluate whether a shared parser wrapper is needed or adapters should keep bespoke parsing.

### Phase 3: Advanced Features (Priority: Medium)
1. Specialized logs tools workflow.
2. Enhanced error handling, status reporting, and potential job queue abstractions once multiple transports are stable.
3. **Performance optimizations**
4. **Comprehensive testing**

### Phase 4: Future Enhancements (Priority: Low)
1. **Streaming support for long-running operations**
2. **Additional MCP namespaces (resources, prompts)**
3. **Advanced caching strategies**
4. **Monitoring and metrics**

## Benefits of This Architecture

1. ✅ **Code Reuse**: All existing MCP tools work unchanged after refactoring
2. ✅ **Consistency**: Same functionality via HTTP and WebSocket  
3. ✅ **Integration**: Native part of Netdata web server
4. ✅ **Authorization**: Reuses existing HTTP_ACL/HTTP_ACCESS system
5. ✅ **Maintenance**: Single codebase for all MCP logic
6. ✅ **Performance**: No extra proxy/adapter process
7. ✅ **Scalability**: Clean separation enables easy addition of new tools and transports
