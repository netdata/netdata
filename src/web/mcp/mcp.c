// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp.h"
#include "mcp-initialize.h"
#include "mcp-ping.h"
#include "mcp-tools.h"
#include "mcp-resources.h"
#include "mcp-prompts.h"
#include "mcp-logging.h"
#include "mcp-completion.h"
#include "mcp-tools-execute-function-registry.h"
#include "web/api/mcp_auth.h"

static bool mcp_initialized = false;

// Define the enum to string mapping for protocol versions
ENUM_STR_MAP_DEFINE(MCP_PROTOCOL_VERSION) = {
    { .id = MCP_PROTOCOL_VERSION_2024_11_05, .name = "2024-11-05" },
    { .id = MCP_PROTOCOL_VERSION_2025_03_26, .name = "2025-03-26" },
    { .id = MCP_PROTOCOL_VERSION_UNKNOWN, .name = "unknown" },
    
    // terminator
    { .name = NULL, .id = 0 }
};
ENUM_STR_DEFINE_FUNCTIONS(MCP_PROTOCOL_VERSION, MCP_PROTOCOL_VERSION_UNKNOWN, "unknown");

// Define the enum to string mapping for return codes
ENUM_STR_MAP_DEFINE(MCP_RETURN_CODE) = {
    { .id = MCP_RC_OK, .name = "OK" },
    { .id = MCP_RC_ERROR, .name = "ERROR" },
    { .id = MCP_RC_INVALID_PARAMS, .name = "INVALID_PARAMS" },
    { .id = MCP_RC_NOT_FOUND, .name = "NOT_FOUND" },
    { .id = MCP_RC_INTERNAL_ERROR, .name = "INTERNAL_ERROR" },
    { .id = MCP_RC_NOT_IMPLEMENTED, .name = "NOT_IMPLEMENTED" },
    { .id = MCP_RC_BAD_REQUEST, .name = "BAD_REQUEST" },
    
    // terminator
    { .name = NULL, .id = 0 }
};
ENUM_STR_DEFINE_FUNCTIONS(MCP_RETURN_CODE, MCP_RC_ERROR, "ERROR");

// Define the enum to string mapping for logging levels
ENUM_STR_MAP_DEFINE(MCP_LOGGING_LEVEL) = {
    { .id = MCP_LOGGING_LEVEL_DEBUG, .name = "debug" },
    { .id = MCP_LOGGING_LEVEL_INFO, .name = "info" },
    { .id = MCP_LOGGING_LEVEL_NOTICE, .name = "notice" },
    { .id = MCP_LOGGING_LEVEL_WARNING, .name = "warning" },
    { .id = MCP_LOGGING_LEVEL_ERROR, .name = "error" },
    { .id = MCP_LOGGING_LEVEL_CRITICAL, .name = "critical" },
    { .id = MCP_LOGGING_LEVEL_ALERT, .name = "alert" },
    { .id = MCP_LOGGING_LEVEL_EMERGENCY, .name = "emergency" },
    { .id = MCP_LOGGING_LEVEL_UNKNOWN, .name = "unknown" },
    
    // terminator
    { .name = NULL, .id = 0 }
};
ENUM_STR_DEFINE_FUNCTIONS(MCP_LOGGING_LEVEL, MCP_LOGGING_LEVEL_UNKNOWN, "unknown");

// Create a response context for a transport session
MCP_CLIENT *mcp_create_client(MCP_TRANSPORT transport, void *transport_ctx) {
    MCP_CLIENT *mcpc = callocz(1, sizeof(MCP_CLIENT));

    mcpc->transport = transport;
    mcpc->protocol_version = MCP_PROTOCOL_VERSION_UNKNOWN; // Will be set during initialization
    mcpc->ready = false; // Client is not ready until initialized notification is received

    // Set capabilities based on transport type
    switch (transport) {
        case MCP_TRANSPORT_WEBSOCKET:
            mcpc->websocket = (struct websocket_server_client *)transport_ctx;
            mcpc->capabilities = MCP_CAPABILITY_ASYNC_COMMUNICATION |
                               MCP_CAPABILITY_SUBSCRIPTIONS |
                               MCP_CAPABILITY_NOTIFICATIONS;
            break;

        case MCP_TRANSPORT_HTTP:
            mcpc->http = (struct web_client *)transport_ctx;
            mcpc->capabilities = MCP_CAPABILITY_NONE; // HTTP has no special capabilities
            break;

        case MCP_TRANSPORT_SSE:
            mcpc->http = (struct web_client *)transport_ctx;
            mcpc->capabilities = MCP_CAPABILITY_ASYNC_COMMUNICATION |
                               MCP_CAPABILITY_SUBSCRIPTIONS |
                               MCP_CAPABILITY_NOTIFICATIONS;
            break;

        default:
            mcpc->generic = transport_ctx;
            mcpc->capabilities = MCP_CAPABILITY_NONE;
            break;
    }

    // Default client info (will be updated later from actual client)
    mcpc->client_name = string_strdupz("unknown");
    mcpc->client_version = string_strdupz("0.0.0");

    // Set default logging level to info
    mcpc->logging_level = MCP_LOGGING_LEVEL_INFO;

    // Persistent buffers
    mcpc->error = buffer_create(1024, NULL);
    mcpc->result = NULL;

    mcpc->last_return_code = MCP_RC_OK;
    mcpc->last_response_error = false;

    return mcpc;
}

// Free a response context
void mcp_free_client(MCP_CLIENT *mcpc) {
    if (!mcpc)
        return;

    string_freez(mcpc->client_name);
    string_freez(mcpc->client_version);

    if (mcpc->error)
        buffer_free(mcpc->error);

    mcp_client_release_response(mcpc);

    freez(mcpc);
}

void mcp_client_clear_error(MCP_CLIENT *mcpc) {
    if (mcpc && mcpc->error)
        buffer_reset(mcpc->error);
}

static void mcp_client_free_chunks(MCP_CLIENT *mcpc) {
    if (!mcpc || !mcpc->response_chunks)
        return;

    for (size_t i = 0; i < mcpc->response_chunks_used; i++) {
        if (mcpc->response_chunks[i].buffer)
            buffer_free(mcpc->response_chunks[i].buffer);
    }

    freez(mcpc->response_chunks);
    mcpc->response_chunks = NULL;
    mcpc->response_chunks_used = 0;
    mcpc->response_chunks_size = 0;
    mcpc->result = NULL;
}

void mcp_client_prepare_response(MCP_CLIENT *mcpc) {
    if (!mcpc)
        return;

    mcp_client_free_chunks(mcpc);
    mcpc->last_return_code = MCP_RC_OK;
    mcpc->last_response_error = false;
}

void mcp_client_release_response(MCP_CLIENT *mcpc) {
    mcp_client_free_chunks(mcpc);
}

static struct mcp_response_chunk *mcp_response_append_chunk(MCP_CLIENT *mcpc, enum mcp_response_chunk_type type) {
    if (!mcpc)
        return NULL;

    const size_t MAX_RESPONSE_BYTES = 16 * 1024 * 1024; // 16 MiB per request safeguard
    if (mcp_client_response_size(mcpc) >= MAX_RESPONSE_BYTES) {
        netdata_log_error("MCP: response size limit reached");
        return NULL;
    }

    if (mcpc->response_chunks_used == mcpc->response_chunks_size) {
        size_t new_size = mcpc->response_chunks_size ? mcpc->response_chunks_size * 2 : 4;
        struct mcp_response_chunk *tmp = reallocz(mcpc->response_chunks, new_size * sizeof(*tmp));
        if (unlikely(!tmp))
            return NULL;
        mcpc->response_chunks = tmp;
        mcpc->response_chunks_size = new_size;
    }

    struct mcp_response_chunk *chunk = &mcpc->response_chunks[mcpc->response_chunks_used++];
    chunk->buffer = NULL;
    chunk->type = type;
    return chunk;
}

BUFFER *mcp_response_add_json_chunk(MCP_CLIENT *mcpc, size_t initial_capacity) {
    struct mcp_response_chunk *chunk = mcp_response_append_chunk(mcpc, MCP_RESPONSE_CHUNK_JSON);
    if (!chunk)
        return NULL;

    size_t capacity = initial_capacity ? initial_capacity : 4096;
    chunk->buffer = buffer_create(capacity, NULL);
    mcpc->result = chunk->buffer;
    buffer_json_initialize(chunk->buffer, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    return chunk->buffer;
}

BUFFER *mcp_response_add_text_chunk(MCP_CLIENT *mcpc, size_t initial_capacity) {
    struct mcp_response_chunk *chunk = mcp_response_append_chunk(mcpc, MCP_RESPONSE_CHUNK_TEXT);
    if (!chunk)
        return NULL;

    size_t capacity = initial_capacity ? initial_capacity : 1024;
    chunk->buffer = buffer_create(capacity, NULL);
    chunk->buffer->content_type = CT_TEXT_PLAIN;
    buffer_no_cacheable(chunk->buffer);
    mcpc->result = chunk->buffer;
    return chunk->buffer;
}

size_t mcp_client_response_chunk_count(const MCP_CLIENT *mcpc) {
    return mcpc ? mcpc->response_chunks_used : 0;
}

const struct mcp_response_chunk *mcp_client_response_chunks(const MCP_CLIENT *mcpc) {
    return mcpc ? mcpc->response_chunks : NULL;
}

size_t mcp_client_response_size(const MCP_CLIENT *mcpc) {
    if (!mcpc || !mcpc->response_chunks)
        return 0;

    size_t total = 0;
    for (size_t i = 0; i < mcpc->response_chunks_used; i++) {
        if (mcpc->response_chunks[i].buffer)
            total += buffer_strlen(mcpc->response_chunks[i].buffer);
    }
    return total;
}

const char *mcp_client_error_message(MCP_CLIENT *mcpc) {
    if (!mcpc || !mcpc->error)
        return NULL;
    return buffer_strlen(mcpc->error) ? buffer_tostring(mcpc->error) : NULL;
}

void mcp_init_success_result(MCP_CLIENT *mcpc, MCP_REQUEST_ID id __maybe_unused) {
    if (!mcpc)
        return;

    BUFFER *chunk = mcp_response_add_json_chunk(mcpc, 4096);
    if (!chunk)
        return;

    mcpc->last_return_code = MCP_RC_OK;
    mcpc->last_response_error = false;
    mcp_client_clear_error(mcpc);
}

MCP_RETURN_CODE mcp_error_result(MCP_CLIENT *mcpc, MCP_REQUEST_ID id __maybe_unused, MCP_RETURN_CODE rc) {
    if (!mcpc)
        return rc;

    mcpc->last_return_code = rc;
    mcpc->last_response_error = true;

    BUFFER *chunk = mcp_response_add_json_chunk(mcpc, 512);
    if (!chunk)
        return rc;

    const char *error_message = buffer_strlen(mcpc->error)
                              ? buffer_tostring(mcpc->error)
                              : MCP_RETURN_CODE_2str(rc);

    buffer_json_member_add_string(chunk, "status", "error");
    buffer_json_member_add_string(chunk, "code", MCP_RETURN_CODE_2str(rc));
    buffer_json_member_add_int64(chunk, "codeNumeric", rc);
    if (error_message)
        buffer_json_member_add_string(chunk, "message", error_message);
    buffer_json_finalize(chunk);

    return rc;
}

// Parse and extract client info from initialize request params
static void mcp_extract_client_info(MCP_CLIENT *mcpc, struct json_object *params) {
    if (!mcpc || !params) return;
    
    struct json_object *client_info_obj = NULL;
    struct json_object *client_name_obj = NULL;
    struct json_object *client_version_obj = NULL;
    
    if (json_object_object_get_ex(params, "clientInfo", &client_info_obj)) {
        if (json_object_object_get_ex(client_info_obj, "name", &client_name_obj)) {
            string_freez(mcpc->client_name);
            mcpc->client_name = string_strdupz(json_object_get_string(client_name_obj));
        }
        if (json_object_object_get_ex(client_info_obj, "version", &client_version_obj)) {
            string_freez(mcpc->client_version);
            mcpc->client_version = string_strdupz(json_object_get_string(client_version_obj));
        }
    }
}

MCP_RETURN_CODE mcp_dispatch_method(MCP_CLIENT *mcpc, const char *method, struct json_object *params, MCP_REQUEST_ID id __maybe_unused) {
    if (!mcpc)
        return MCP_RC_INTERNAL_ERROR;

    if (!method || !*method) {
        buffer_strcat(mcpc->error, "Empty method name");
        mcp_error_result(mcpc, 0, MCP_RC_INVALID_PARAMS);
        return MCP_RC_INVALID_PARAMS;
    }

    if (!params || json_object_get_type(params) != json_type_object) {
        buffer_strcat(mcpc->error, "Parameters must be an object");
        mcp_error_result(mcpc, 0, MCP_RC_INVALID_PARAMS);
        return MCP_RC_INVALID_PARAMS;
    }

    MCP_RETURN_CODE rc = MCP_RC_OK;

    if (strcmp(method, "notifications/initialized") == 0) {
        mcpc->ready = true;
        netdata_log_debug(D_WEB_CLIENT, "MCP client %s v%s is now ready",
                          string2str(mcpc->client_name), string2str(mcpc->client_version));
        mcp_client_prepare_response(mcpc);
        mcp_init_success_result(mcpc, 0);
        buffer_json_finalize(mcpc->result);
        return MCP_RC_OK;
    }

    if (!mcpc->ready && strcmp(method, "initialize") != 0) {
        netdata_log_debug(D_WEB_CLIENT, "MCP method %s called before initialize", method);
    }

    mcp_client_prepare_response(mcpc);
    mcp_client_clear_error(mcpc);

    if (strncmp(method, "tools/", 6) == 0) {
        rc = mcp_tools_route(mcpc, method + 6, params, 0);
        if (!mcpc->ready)
            mcpc->ready = true;
    }
    else if (strncmp(method, "resources/", 10) == 0) {
        rc = mcp_resources_route(mcpc, method + 10, params, 0);
        if (!mcpc->ready)
            mcpc->ready = true;
    }
    else if (strncmp(method, "prompts/", 8) == 0) {
        rc = mcp_prompts_route(mcpc, method + 8, params, 0);
        if (!mcpc->ready)
            mcpc->ready = true;
    }
    else if (strncmp(method, "logging/", 8) == 0) {
        rc = mcp_logging_route(mcpc, method + 8, params, 0);
    }
    else if (strncmp(method, "completion/", 11) == 0) {
        rc = mcp_completion_route(mcpc, method + 11, params, 0);
        if (!mcpc->ready)
            mcpc->ready = true;
    }
    else if (strcmp(method, "initialize") == 0) {
        mcp_extract_client_info(mcpc, params);
        netdata_log_debug(D_WEB_CLIENT, "MCP initialize request from client %s v%s",
                          string2str(mcpc->client_name), string2str(mcpc->client_version));
        rc = mcp_method_initialize(mcpc, params, 0);
    }
    else if (strcmp(method, "ping") == 0) {
        rc = mcp_method_ping(mcpc, params, 0);
    }
    else {
        buffer_sprintf(mcpc->error, "Method '%s' not found", method);
        rc = MCP_RC_NOT_FOUND;
    }

    if (rc != MCP_RC_OK)
        mcp_error_result(mcpc, 0, rc);

    // Ensure at least one chunk exists on success
    if (rc == MCP_RC_OK && mcp_client_response_chunk_count(mcpc) == 0) {
        buffer_strcat(mcpc->error, "method generated empty result");
        rc = mcp_error_result(mcpc, 0, MCP_RC_INTERNAL_ERROR);
    }

    return rc;
}

// Initialize the MCP subsystem
void mcp_initialize_subsystem(void) {
    if (unlikely(mcp_initialized))
        return;

    mcp_functions_registry_init();

#ifdef NETDATA_MCP_DEV_PREVIEW_API_KEY
    mcp_api_key_initialize();
#endif

    // debug_flags |= D_MCP;

    netdata_log_info("MCP subsystem initialized");
    mcp_initialized = true;
}
