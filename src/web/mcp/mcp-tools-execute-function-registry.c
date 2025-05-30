// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-execute-function-registry.h"
#include "database/rrdfunctions.h"

// Parameter type string mappings
ENUM_STR_MAP_DEFINE(MCP_REQUIRED_PARAMS_TYPE) = {
    { MCP_REQUIRED_PARAMS_TYPE_SELECT, "select" },
    { MCP_REQUIRED_PARAMS_TYPE_MULTISELECT, "multiselect" },
    { 0, NULL }
};

ENUM_STR_DEFINE_FUNCTIONS(MCP_REQUIRED_PARAMS_TYPE, MCP_REQUIRED_PARAMS_TYPE_SELECT, "select")

// Static dictionary to store function registry entries
static DICTIONARY *functions_registry = NULL;

// Cleanup function for registry entries
static void registry_entry_cleanup(MCP_FUNCTION_REGISTRY_ENTRY *entry) {
    if (!entry)
        return;
    
    // Free STRING pointers
    string_freez(entry->help);
    
    // Free parameters
    for (size_t i = 0; i < entry->required_params_count; i++) {
        MCP_FUNCTION_PARAM *param = &entry->required_params[i];
        string_freez(param->id);
        string_freez(param->name);
        string_freez(param->help);
        
        // Free options
        for (size_t j = 0; j < param->options_count; j++) {
            string_freez(param->options[j].id);
            string_freez(param->options[j].name);
        }
        freez(param->options);
    }
    freez(entry->required_params);
}

// Dictionary callbacks
static void registry_entry_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    MCP_FUNCTION_REGISTRY_ENTRY *entry = (MCP_FUNCTION_REGISTRY_ENTRY *)value;

    rw_spinlock_init(&entry->spinlock);
    spinlock_init(&entry->update_spinlock);
}

static bool registry_entry_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    MCP_FUNCTION_REGISTRY_ENTRY *old_entry = (MCP_FUNCTION_REGISTRY_ENTRY *)old_value;
    MCP_FUNCTION_REGISTRY_ENTRY *new_entry = (MCP_FUNCTION_REGISTRY_ENTRY *)new_value;
    
    // Get write lock on the old entry
    rw_spinlock_write_lock(&old_entry->spinlock);
    
    // Swap all members between old and new
    // This moves the old data to new_entry and new data to old_entry
    SWAP(old_entry->type, new_entry->type);
    SWAP(old_entry->has_history, new_entry->has_history);
    SWAP(old_entry->update_every, new_entry->update_every);
    SWAP(old_entry->version, new_entry->version);
    SWAP(old_entry->supports_post, new_entry->supports_post);
    SWAP(old_entry->help, new_entry->help);
    SWAP(old_entry->required_params_count, new_entry->required_params_count);
    SWAP(old_entry->required_params, new_entry->required_params);
    SWAP(old_entry->last_update, new_entry->last_update);
    SWAP(old_entry->expires, new_entry->expires);
    
    // Release the lock
    rw_spinlock_write_unlock(&old_entry->spinlock);
    
    // Cleanup the new entry (which now contains the old data)
    registry_entry_cleanup(new_entry);
    
    // Return false to reject the new value (we've already updated old_value)
    return false;
}

static void registry_entry_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    MCP_FUNCTION_REGISTRY_ENTRY *entry = (MCP_FUNCTION_REGISTRY_ENTRY *)value;
    
    // Cleanup the entry data
    registry_entry_cleanup(entry);
}

// Initialize the functions registry
void mcp_functions_registry_init(void) {
    if (functions_registry)
        return;
    
    functions_registry = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
        NULL,
        sizeof(MCP_FUNCTION_REGISTRY_ENTRY));
    
    dictionary_register_insert_callback(functions_registry, registry_entry_insert_callback, NULL);
    dictionary_register_delete_callback(functions_registry, registry_entry_delete_callback, NULL);
    dictionary_register_conflict_callback(functions_registry, registry_entry_conflict_callback, NULL);
}

// Cleanup the functions registry
void mcp_functions_registry_cleanup(void) {
    if (!functions_registry)
        return;
    
    dictionary_destroy(functions_registry);
    functions_registry = NULL;
}

// Parse JSON info response and populate registry entry
static int parse_function_info(struct json_object *json_obj, MCP_FUNCTION_REGISTRY_ENTRY *entry) {
    struct json_object *jobj;
    
    // Parse version (v3+ supports POST)
    entry->version = 1; // default to v1
    if (json_object_object_get_ex(json_obj, "v", &jobj)) {
        entry->version = json_object_get_int(jobj);
    }
    entry->supports_post = (entry->version >= 3);
    
    // Parse type
    if (json_object_object_get_ex(json_obj, "type", &jobj)) {
        const char *type_str = json_object_get_string(jobj);
        if (strcmp(type_str, "table") == 0) {
            entry->type = FN_TYPE_TABLE;
        } else {
            entry->type = FN_TYPE_UNKNOWN;
        }
    }
    
    // Parse has_history
    if (json_object_object_get_ex(json_obj, "has_history", &jobj)) {
        entry->has_history = json_object_get_boolean(jobj);
        if (entry->has_history && entry->type == FN_TYPE_TABLE) {
            entry->type = FN_TYPE_TABLE_WITH_HISTORY;
        }
    }
    
    // Parse update_every
    if (json_object_object_get_ex(json_obj, "update_every", &jobj)) {
        entry->update_every = json_object_get_int(jobj);
    }
    
    // Parse help
    if (json_object_object_get_ex(json_obj, "help", &jobj)) {
        entry->help = string_strdupz(json_object_get_string(jobj));
    }
    
    // Parse accepted_params to detect supported optional parameters
    if (json_object_object_get_ex(json_obj, "accepted_params", &jobj)) {
        if (json_object_is_type(jobj, json_type_array)) {
            size_t accepted_count = json_object_array_length(jobj);
            for (size_t i = 0; i < accepted_count; i++) {
                struct json_object *param_obj = json_object_array_get_idx(jobj, i);
                if (param_obj && json_object_is_type(param_obj, json_type_string)) {
                    const char *param_name = json_object_get_string(param_obj);
                    
                    // Check for timeframe parameters
                    if (strcmp(param_name, "after") == 0 || strcmp(param_name, "before") == 0) {
                        entry->has_timeframe = true;
                    }
                    // Check for other specific parameters
                    else if (strcmp(param_name, "anchor") == 0) {
                        entry->has_anchor = true;
                    }
                    else if (strcmp(param_name, "last") == 0) {
                        entry->has_last = true;
                    }
                    else if (strcmp(param_name, "data_only") == 0) {
                        entry->has_data_only = true;
                    }
                    else if (strcmp(param_name, "direction") == 0) {
                        entry->has_direction = true;
                    }
                    else if (strcmp(param_name, "query") == 0) {
                        entry->has_query = true;
                    }
                    else if (strcmp(param_name, "all_fields_selected") == 0) {
                        entry->has_all_fields_selected = true;
                    }
                }
            }
        }
    }
    
    // Parse required_params
    if (json_object_object_get_ex(json_obj, "required_params", &jobj)) {
        if (json_object_is_type(jobj, json_type_array)) {
            entry->required_params_count = json_object_array_length(jobj);
            if (entry->required_params_count > 0) {
                entry->required_params = callocz(entry->required_params_count, sizeof(MCP_FUNCTION_PARAM));
                
                for (size_t i = 0; i < entry->required_params_count; i++) {
                    struct json_object *param_obj = json_object_array_get_idx(jobj, i);
                    MCP_FUNCTION_PARAM *param = &entry->required_params[i];
                    
                    struct json_object *field;
                    
                    // Parse param fields
                    if (json_object_object_get_ex(param_obj, "id", &field))
                        param->id = string_strdupz(json_object_get_string(field));
                    
                    if (json_object_object_get_ex(param_obj, "name", &field))
                        param->name = string_strdupz(json_object_get_string(field));
                    
                    if (json_object_object_get_ex(param_obj, "help", &field))
                        param->help = string_strdupz(json_object_get_string(field));
                    
                    if (json_object_object_get_ex(param_obj, "type", &field)) {
                        const char *type_str = json_object_get_string(field);
                        param->type = MCP_REQUIRED_PARAMS_TYPE_2id(type_str);
                    }
                    
                    if (json_object_object_get_ex(param_obj, "unique_view", &field))
                        param->unique_view = json_object_get_boolean(field);
                    
                    // Parse options
                    if (json_object_object_get_ex(param_obj, "options", &field)) {
                        if (json_object_is_type(field, json_type_array)) {
                            param->options_count = json_object_array_length(field);
                            if (param->options_count > 0) {
                                param->options = callocz(param->options_count, sizeof(MCP_FUNCTION_PARAM_OPTION));
                                
                                for (size_t j = 0; j < param->options_count; j++) {
                                    struct json_object *opt_obj = json_object_array_get_idx(field, j);
                                    MCP_FUNCTION_PARAM_OPTION *opt = &param->options[j];
                                    
                                    struct json_object *opt_field;
                                    if (json_object_object_get_ex(opt_obj, "id", &opt_field))
                                        opt->id = string_strdupz(json_object_get_string(opt_field));
                                    
                                    if (json_object_object_get_ex(opt_obj, "name", &opt_field))
                                        opt->name = string_strdupz(json_object_get_string(opt_field));
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    time_t now = now_realtime_sec();
    entry->last_update = now;
    entry->expires = now + MCP_FUNCTIONS_REGISTRY_TTL;

    return 0;
}

// Fetch function info from the node (private function)
static MCP_FUNCTION_REGISTRY_ENTRY *mcp_function_get_info(RRDHOST *host, const char *function_name, BUFFER *error) {
    if (!host || !function_name) {
        buffer_strcat(error, "Invalid host or function name");
        return NULL;
    }
    
    // Prepare the info request
    char info_function[256];
    snprintfz(info_function, sizeof(info_function), "%s info", function_name);

    USER_AUTH auth = {
        .user_role = HTTP_USER_ROLE_ADMIN,
        .access = HTTP_ACCESS_ALL,
        .method = USER_AUTH_METHOD_GOD,
        .client_ip = "mcp-info",
        .client_name = "mcp-tools-execute-function-registry",
    };

    // Create source buffer from user_auth
    CLEAN_BUFFER *source = buffer_create(0, NULL);
    user_auth_to_source_buffer(&auth, source);
    buffer_strcat(source, ",modelcontextprotocol");

    // Call the function with info parameter
    BUFFER *response = buffer_create(0, NULL);
    int code = rrd_function_run(
        host,
        response,
        10,
        auth.access,
        info_function,
        true,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        buffer_tostring(source),
        false);
    
    if (code != HTTP_RESP_OK) {
        buffer_sprintf(error, "Failed to get function info: HTTP %d", code);
        buffer_free(response);
        return NULL;
    }
    
    // Parse JSON response
    struct json_tokener *tokener = json_tokener_new();
    struct json_object *json_obj = json_tokener_parse_ex(tokener, buffer_tostring(response), buffer_strlen(response));
    json_tokener_free(tokener);
    buffer_free(response);
    
    if (!json_obj) {
        buffer_strcat(error, "Failed to parse JSON response");
        return NULL;
    }
    
    // Check if it's a special info response with required_params
    struct json_object *required_params_obj;
    if (!json_object_object_get_ex(json_obj, "required_params", &required_params_obj)) {
        // This function doesn't support parameters
        MCP_FUNCTION_REGISTRY_ENTRY *entry = callocz(1, sizeof(MCP_FUNCTION_REGISTRY_ENTRY));
        parse_function_info(json_obj, entry);
        json_object_put(json_obj);
        return entry;
    }
    
    // Create and populate registry entry
    MCP_FUNCTION_REGISTRY_ENTRY *entry = callocz(1, sizeof(MCP_FUNCTION_REGISTRY_ENTRY));
    if (parse_function_info(json_obj, entry) != 0) {
        json_object_put(json_obj);
        freez(entry);
        buffer_strcat(error, "Failed to parse function info");
        return NULL;
    }
    
    json_object_put(json_obj);
    return entry;
}

// Create dictionary key from host and function name
static void create_registry_key(BUFFER *key_buffer, RRDHOST *host, const char *function_name) {
    buffer_flush(key_buffer);
    buffer_sprintf(key_buffer, "%s|%s", rrdhost_hostname(host), function_name);
}

// Get a registry entry for a function
MCP_FUNCTION_REGISTRY_ENTRY *mcp_functions_registry_get(RRDHOST *host, const char *function_name, BUFFER *error) {
    if (!functions_registry) {
        buffer_strcat(error, "Functions registry not initialized");
        return NULL;
    }
    
    // Create dictionary key
    CLEAN_BUFFER *key = buffer_create(0, NULL);
    create_registry_key(key, host, function_name);
    const char *key_str = buffer_tostring(key);
    
    time_t now = now_realtime_sec();
    
    // Try to get existing entry
    MCP_FUNCTION_REGISTRY_ENTRY *entry = dictionary_get(functions_registry, key_str);
    MCP_FUNCTION_REGISTRY_ENTRY *old_entry = NULL;

    if(entry && entry->last_update + MCP_FUNCTIONS_REGISTRY_TTL < now && spinlock_trylock(&entry->update_spinlock)) {
        old_entry = entry;
        entry = NULL;
    }

    if(!entry) {
        MCP_FUNCTION_REGISTRY_ENTRY *new_info = mcp_function_get_info(host, function_name, error);
        if(new_info) {
            entry = dictionary_set(functions_registry, key_str, new_info, sizeof(MCP_FUNCTION_REGISTRY_ENTRY));
            freez(new_info);
        }
        else if(old_entry)
            entry = old_entry;
        else
            return NULL;
    }

    if(old_entry)
        spinlock_unlock(&old_entry->update_spinlock);

    if(entry)
        rw_spinlock_read_lock(&entry->spinlock);

    return entry;
}

// Release a registry entry
void mcp_functions_registry_release(MCP_FUNCTION_REGISTRY_ENTRY *entry) {
    if (!entry)
        return;
    
    rw_spinlock_read_unlock(&entry->spinlock);
}