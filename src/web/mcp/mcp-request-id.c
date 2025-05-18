// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-request-id.h"
#include "mcp.h"

// Request ID structure - stored in JudyL array
typedef struct mcp_request_id_entry {
    enum {
        MCP_REQUEST_ID_TYPE_INT,
        MCP_REQUEST_ID_TYPE_STRING
    } type;
    
    union {
        int64_t int_value;
        STRING *str_value;
    };
} MCP_REQUEST_ID_ENTRY;

/**
 * Extract and register a request ID from a JSON object
 * 
 * @param mcpc The MCP client context
 * @param request The JSON request object that may contain an ID
 * @return MCP_REQUEST_ID - the assigned ID (0 if no ID was present)
 */
MCP_REQUEST_ID mcp_request_id_add(MCP_CLIENT *mcpc, struct json_object *request) {
    if (!mcpc || !request)
        return 0;
    
    // Extract ID (optional, for notifications)
    struct json_object *id_obj = NULL;
    bool has_id = json_object_object_get_ex(request, "id", &id_obj);
    
    if (!has_id)
        return 0;
    
    // Allocate a new entry
    MCP_REQUEST_ID_ENTRY *entry = callocz(1, sizeof(MCP_REQUEST_ID_ENTRY));
    
    // Generate a new sequential ID
    MCP_REQUEST_ID id = ++mcpc->request_id_counter;
    
    // Store the entry in the JudyL array
    Word_t Index = (Word_t)id;
    Pvoid_t *PValue = JudyLIns(&mcpc->request_ids, Index, NULL);
    if (unlikely(PValue == PJERR)) {
        netdata_log_error("MCP: JudyLIns failed for request ID %zu", id);
        freez(entry);
        return 0;
    }
    
    // Parse the ID value
    if (json_object_get_type(id_obj) == json_type_int) {
        entry->type = MCP_REQUEST_ID_TYPE_INT;
        entry->int_value = json_object_get_int64(id_obj);
    }
    else if (json_object_get_type(id_obj) == json_type_string) {
        entry->type = MCP_REQUEST_ID_TYPE_STRING;
        entry->str_value = string_strdupz(json_object_get_string(id_obj));
    }
    else {
        // Unsupported ID type, treat as no ID
        freez(entry);
        return 0;
    }
    
    // Store the entry in the JudyL
    *PValue = entry;
    
    return id;
}

/**
 * Delete a request ID from the registry
 * 
 * @param mcpc The MCP client context
 * @param id The request ID to delete
 */
void mcp_request_id_del(MCP_CLIENT *mcpc, MCP_REQUEST_ID id) {
    if (!mcpc || id == 0)
        return;
    
    // Get the entry from JudyL
    Word_t Index = (Word_t)id;
    Pvoid_t *PValue = JudyLGet(mcpc->request_ids, Index, NULL);
    if (!PValue)
        return;
    
    MCP_REQUEST_ID_ENTRY *entry = *PValue;
    
    // Free string value if present
    if (entry->type == MCP_REQUEST_ID_TYPE_STRING)
        string_freez(entry->str_value);
    
    // Free the entry
    freez(entry);
    
    // Remove the entry from JudyL
    int rc = JudyLDel(&mcpc->request_ids, Index, NULL);
    if (unlikely(!rc)) {
        netdata_log_error("MCP: JudyLDel failed for request ID %zu", id);
    }
}

/**
 * Clean up all request IDs for a client
 * 
 * @param mcpc The MCP client context
 */
void mcp_request_id_cleanup_all(MCP_CLIENT *mcpc) {
    if (!mcpc || !mcpc->request_ids)
        return;
    
    Word_t Index = 0;
    Pvoid_t *PValue;
    
    // Get the first index
    PValue = JudyLFirst(mcpc->request_ids, &Index, NULL);
    
    // Iterate through all entries
    while (PValue != NULL) {
        // Free the request ID entry
        if (PValue) {
            MCP_REQUEST_ID_ENTRY *entry = *PValue;
            if (entry->type == MCP_REQUEST_ID_TYPE_STRING)
                string_freez(entry->str_value);
            freez(entry);
        }
        
        // Move to next entry
        PValue = JudyLNext(mcpc->request_ids, &Index, NULL);
    }
    
    // Free the JudyL array
    JudyLFreeArray(&mcpc->request_ids, NULL);
    mcpc->request_ids = NULL;
}

/**
 * Add a request ID to a buffer as a JSON member
 * 
 * @param mcpc The MCP client context
 * @param wb The buffer to add the ID to
 * @param key The JSON key name to use
 * @param id The request ID to add
 */
void mcp_request_id_to_buffer(MCP_CLIENT *mcpc, BUFFER *wb, const char *key, MCP_REQUEST_ID id) {
    if (!wb || !key) {
        return;
    }
    
    if (!mcpc || id == 0) {
        // For ID 0 or no client context, add it as a numeric 0
        buffer_json_member_add_uint64(wb, key, 0);
        return;
    }
    
    // Get the entry from JudyL
    Word_t Index = (Word_t)id;
    Pvoid_t *PValue = JudyLGet(mcpc->request_ids, Index, NULL);
    if (!PValue) {
        // If entry not found, add 0 as the ID
        buffer_json_member_add_uint64(wb, key, 0);
        return;
    }
    
    MCP_REQUEST_ID_ENTRY *entry = *PValue;
    
    // Add the ID based on its type
    if (entry->type == MCP_REQUEST_ID_TYPE_INT) {
        buffer_json_member_add_uint64(wb, key, entry->int_value);
    }
    else if (entry->type == MCP_REQUEST_ID_TYPE_STRING) {
        buffer_json_member_add_string(wb, key, string2str(entry->str_value));
    }
}