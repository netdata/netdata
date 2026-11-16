// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_JSONRPC_H
#define NETDATA_MCP_JSONRPC_H

#include <json-c/json.h>
#include "mcp.h"

int mcp_jsonrpc_error_code(MCP_RETURN_CODE rc);
struct json_object *mcp_jsonrpc_parse_request(const char *request, size_t length, enum json_tokener_error *error);
BUFFER *mcp_jsonrpc_build_error_payload(struct json_object *id_obj, int code, const char *message,
                                        const struct mcp_response_chunk *chunks, size_t chunk_count);
BUFFER *mcp_jsonrpc_build_success_payload(struct json_object *id_obj, const struct mcp_response_chunk *chunk);
BUFFER *mcp_jsonrpc_process_single_request(MCP_CLIENT *mcpc, struct json_object *request, bool *had_error);

// Runs a JSON-RPC batch, passing each response to emit() (the batch owns and frees it afterwards).
// Returns NULL when the batch was run, or one error payload when the batch itself is invalid
// (empty, or more requests than allowed): then no request ran and emit() was never called.
typedef void (*mcp_jsonrpc_batch_emit_t)(BUFFER *response, void *data);
BUFFER *mcp_jsonrpc_process_batch(MCP_CLIENT *mcpc, struct json_object *batch,
                                  mcp_jsonrpc_batch_emit_t emit, void *data);

// emit() that appends each response to the (initially empty) BUFFER in data as a JSON array;
// the caller appends the closing "]" when the BUFFER is not empty.
void mcp_jsonrpc_batch_emit_array(BUFFER *response, void *data);

int mcp_jsonrpc_batch_unittest(void);

#endif // NETDATA_MCP_JSONRPC_H
