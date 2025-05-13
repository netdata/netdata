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
static int mcp_notifications_method_initialized(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    // This is just a notification, just log it
    netdata_log_debug(D_MCP, "Client sent notifications/initialized notification");
    
    // No response needed if this is a notification (id == 0)
    if (id == 0) return 0;
    
    // If it was a request (has id), send an empty success response
    struct json_object *result = json_object_new_object();
    int ret = mcp_send_success_response(mcpc, result, id);
    json_object_put(result);
    
    return ret;
}

// Stub implementations for other notifications methods (transport-agnostic)
static int mcp_notifications_method_subscribe(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "notifications/subscribe", id);
}

static int mcp_notifications_method_unsubscribe(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "notifications/unsubscribe", id);
}

static int mcp_notifications_method_acknowledge(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "notifications/acknowledge", id);
}

static int mcp_notifications_method_getHistory(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "notifications/getHistory", id);
}

static int mcp_notifications_method_send(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "notifications/send", id);
}

static int mcp_notifications_method_getSettings(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "notifications/getSettings", id);
}

// Notifications namespace method dispatcher (transport-agnostic)
int mcp_notifications_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, uint64_t id) {
    if (!mcpc || !method) return -1;
    
    netdata_log_debug(D_MCP, "MCP notifications method: %s", method);
    
    if (strcmp(method, "initialized") == 0) {
        return mcp_notifications_method_initialized(mcpc, params, id);
    }
    else if (strcmp(method, "subscribe") == 0) {
        return mcp_notifications_method_subscribe(mcpc, params, id);
    }
    else if (strcmp(method, "unsubscribe") == 0) {
        return mcp_notifications_method_unsubscribe(mcpc, params, id);
    }
    else if (strcmp(method, "acknowledge") == 0) {
        return mcp_notifications_method_acknowledge(mcpc, params, id);
    }
    else if (strcmp(method, "getHistory") == 0) {
        return mcp_notifications_method_getHistory(mcpc, params, id);
    }
    else if (strcmp(method, "send") == 0) {
        return mcp_notifications_method_send(mcpc, params, id);
    }
    else if (strcmp(method, "getSettings") == 0) {
        return mcp_notifications_method_getSettings(mcpc, params, id);
    }
    else {
        // Method not found in notifications namespace
        char full_method[256];
        snprintf(full_method, sizeof(full_method), "notifications/%s", method);
        return mcp_method_not_implemented_generic(mcpc, full_method, id);
    }
}

