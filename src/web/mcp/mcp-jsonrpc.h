// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_JSONRPC_H
#define NETDATA_MCP_JSONRPC_H

#include <json-c/json.h>
#include "mcp.h"

int mcp_jsonrpc_error_code(MCP_RETURN_CODE rc);
BUFFER *mcp_jsonrpc_build_error_payload(struct json_object *id_obj, int code, const char *message,
                                        const struct mcp_response_chunk *chunks, size_t chunk_count);
BUFFER *mcp_jsonrpc_build_success_payload(struct json_object *id_obj, const struct mcp_response_chunk *chunk);
BUFFER *mcp_jsonrpc_process_single_request(MCP_CLIENT *mcpc, struct json_object *request, bool *had_error);
BUFFER *mcp_jsonrpc_build_batch_response(BUFFER **responses, size_t count);

#endif // NETDATA_MCP_JSONRPC_H
