// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_REQUEST_ID_H
#define NETDATA_MCP_REQUEST_ID_H

#include "libnetdata/libnetdata.h"

// Request ID type - 0 is reserved for "no ID given"
typedef size_t MCP_REQUEST_ID;

// Forward declaration
struct mcp_client;

/**
 * Extract and register a request ID from a JSON object
 * 
 * @param mcpc The MCP client context
 * @param request The JSON request object that may contain an ID
 * @return MCP_REQUEST_ID - the assigned ID (0 if no ID was present)
 */
MCP_REQUEST_ID mcp_request_id_add(struct mcp_client *mcpc, struct json_object *request);

/**
 * Delete a request ID from the registry
 * 
 * @param mcpc The MCP client context
 * @param id The request ID to delete
 */
void mcp_request_id_del(struct mcp_client *mcpc, MCP_REQUEST_ID id);

/**
 * Clean up all request IDs for a client
 * 
 * @param mcpc The MCP client context
 */
void mcp_request_id_cleanup_all(struct mcp_client *mcpc);

/**
 * Add a request ID to a buffer as a JSON member
 * 
 * @param mcpc The MCP client context
 * @param wb The buffer to add the ID to
 * @param key The JSON key name to use
 * @param id The request ID to add
 */
void mcp_request_id_to_buffer(struct mcp_client *mcpc, BUFFER *wb, const char *key, MCP_REQUEST_ID id);

#endif // NETDATA_MCP_REQUEST_ID_H
