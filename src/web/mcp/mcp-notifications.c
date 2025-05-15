// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP Notifications Namespace
 * 
 * The MCP Notifications namespace provides methods for managing and handling notifications.
 * In the MCP protocol, notifications enable real-time communication of events from server to client,
 * and from client to server.
 * 
 * Key features of the notifications namespace:
 * 
 * 1. Initialization:
 *    - Clients notify the server when they're initialized (notifications/initialized)
 *    - This initiates the notification subsystem
 * 
 * 2. Subscription Management:
 *    - Subscribe to specific notification types (notifications/subscribe)
 *    - Unsubscribe from notifications (notifications/unsubscribe)
 *    - Configure notification settings (notifications/getSettings)
 * 
 * 3. Notification Handling:
 *    - Acknowledge received notifications (notifications/acknowledge)
 *    - View notification history (notifications/getHistory)
 *    - Send notifications from client to server (notifications/send)
 * 
 * Notifications in MCP are bidirectional:
 *    - Server-to-client notifications inform about system events, alerts, changes
 *    - Client-to-server notifications provide user actions and status updates
 * 
 * In the Netdata context, notifications include:
 *    - Health monitoring alerts
 *    - System status changes
 *    - Configuration changes
 *    - Resource availability updates
 *    - Client status updates
 * 
 * Notifications can be transient or persistent, prioritized, and may require
 * acknowledgment depending on their type and importance.
 */

#include "mcp-notifications.h"
#include "mcp-initialize.h"

// Implementation of notifications/initialized (transport-agnostic)
static MCP_RETURN_CODE mcp_notifications_method_initialized(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, MCP_REQUEST_ID id) {
    // This is just a notification, just log it
    netdata_log_debug(D_MCP, "Client sent notifications/initialized notification");
    
    // No response needed if this is a notification (id == 0)
    if (id == 0) return MCP_RC_OK;
    
    // If it was a request (has id), send an empty success response
    mcp_init_success_result(mcpc, id);
    buffer_json_finalize(mcpc->result);
    
    return MCP_RC_OK;
}

// Stub implementations for other notifications methods (transport-agnostic)
static MCP_RETURN_CODE mcp_notifications_method_subscribe(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, MCP_REQUEST_ID id __maybe_unused) {
    buffer_sprintf(mcpc->error, "Method 'notifications/subscribe' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_notifications_method_unsubscribe(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, MCP_REQUEST_ID id __maybe_unused) {
    buffer_sprintf(mcpc->error, "Method 'notifications/unsubscribe' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_notifications_method_acknowledge(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, MCP_REQUEST_ID id __maybe_unused) {
    buffer_sprintf(mcpc->error, "Method 'notifications/acknowledge' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_notifications_method_getHistory(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, MCP_REQUEST_ID id __maybe_unused) {
    buffer_sprintf(mcpc->error, "Method 'notifications/getHistory' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_notifications_method_send(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, MCP_REQUEST_ID id __maybe_unused) {
    buffer_sprintf(mcpc->error, "Method 'notifications/send' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_notifications_method_getSettings(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, MCP_REQUEST_ID id __maybe_unused) {
    buffer_sprintf(mcpc->error, "Method 'notifications/getSettings' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

// Notifications namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_notifications_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || !method) return MCP_RC_INTERNAL_ERROR;
    
    netdata_log_debug(D_MCP, "MCP notifications method: %s", method);
    
    // Flush previous buffers
    buffer_flush(mcpc->result);
    buffer_flush(mcpc->error);
    
    MCP_RETURN_CODE rc;
    
    if (strcmp(method, "initialized") == 0) {
        rc = mcp_notifications_method_initialized(mcpc, params, id);
    }
    else if (strcmp(method, "subscribe") == 0) {
        rc = mcp_notifications_method_subscribe(mcpc, params, id);
    }
    else if (strcmp(method, "unsubscribe") == 0) {
        rc = mcp_notifications_method_unsubscribe(mcpc, params, id);
    }
    else if (strcmp(method, "acknowledge") == 0) {
        rc = mcp_notifications_method_acknowledge(mcpc, params, id);
    }
    else if (strcmp(method, "getHistory") == 0) {
        rc = mcp_notifications_method_getHistory(mcpc, params, id);
    }
    else if (strcmp(method, "send") == 0) {
        rc = mcp_notifications_method_send(mcpc, params, id);
    }
    else if (strcmp(method, "getSettings") == 0) {
        rc = mcp_notifications_method_getSettings(mcpc, params, id);
    }
    else {
        // Method not found in notifications namespace
        buffer_sprintf(mcpc->error, "Method 'notifications/%s' not implemented yet", method);
        rc = MCP_RC_NOT_IMPLEMENTED;
    }
    
    return rc;
}

