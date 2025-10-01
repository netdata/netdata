// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_AUTH_H
#define NETDATA_MCP_AUTH_H

#include "daemon/common.h"

#define NETDATA_MCP_DEV_PREVIEW_API_KEY 1

#ifdef NETDATA_MCP_DEV_PREVIEW_API_KEY

#define MCP_DEV_PREVIEW_API_KEY_FILENAME "mcp_dev_preview_api_key"
#define MCP_DEV_PREVIEW_API_KEY_LENGTH 36  // UUID format: 8-4-4-4-12 = 32 hex chars + 4 hyphens

// Initialize the MCP API key subsystem - creates key file if it doesn't exist
void mcp_api_key_initialize(void);

// Verify if the provided API key matches the stored one
// Returns true if valid and agent is claimed, false otherwise
bool mcp_api_key_verify(const char *api_key);

// Get the current API key (for display purposes)
// Returns a static buffer that should not be freed
const char *mcp_api_key_get(void);

#endif // NETDATA_MCP_DEV_PREVIEW_API_KEY

#endif // NETDATA_MCP_AUTH_H
