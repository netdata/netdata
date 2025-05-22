// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext.h"
#include "rrdcontext-internal.h"
#include "rrdcontext-context-registry.h"
#include "web/mcp/mcp.h"

// The registry - using a raw JudyL array
// Key: STRING pointer
// Value: reference count (size_t)
static Pvoid_t context_registry_judyl = NULL;

// Spinlock to protect access to the registry
static SPINLOCK context_registry_spinlock = SPINLOCK_INITIALIZER;

// Clean up the context registry
void rrdcontext_context_registry_destroy(void) {
    spinlock_lock(&context_registry_spinlock);
    
    Word_t index = 0;
    Pvoid_t *PValue;
    
    // Free the strings we've held references to
    PValue = JudyLFirst(context_registry_judyl, &index, PJE0);
    while (PValue) {
        // Each string has been duplicated when added, so free it
        string_freez((STRING *)index);
        PValue = JudyLNext(context_registry_judyl, &index, PJE0);
    }
    
    // Free the entire Judy array
    JudyLFreeArray(&context_registry_judyl, PJE0);
    
    spinlock_unlock(&context_registry_spinlock);
}

// Add a context to the registry or increment its reference count
bool rrdcontext_context_registry_add(STRING *context) {
    if (unlikely(!context))
        return false;
    
    bool is_new = false;
    
    spinlock_lock(&context_registry_spinlock);
    
    // Get or insert a slot for this context
    Pvoid_t *PValue = JudyLIns(&context_registry_judyl, (Word_t)context, PJE0);
    
    if (unlikely(PValue == PJERR)) {
        // Memory allocation error
        internal_error(true, "RRDCONTEXT: JudyL memory allocation failed in rrdcontext_context_registry_add()");
        spinlock_unlock(&context_registry_spinlock);
        return false;
    }
    
    size_t count = (size_t)(Word_t)*PValue;
    
    if (count == 0) {
        // This is a new context - duplicate the string to increase its reference count
        string_dup(context);
        is_new = true;
    }
    
    // Increment the reference count
    *PValue = (void *)(Word_t)(count + 1);
    
    spinlock_unlock(&context_registry_spinlock);
    
    return is_new;
}

// Remove a context from the registry or decrement its reference count
bool rrdcontext_context_registry_remove(STRING *context) {
    if (unlikely(!context))
        return false;
    
    bool is_last = false;
    
    spinlock_lock(&context_registry_spinlock);
    
    // Try to get the value for this context
    Pvoid_t *PValue = JudyLGet(context_registry_judyl, (Word_t)context, PJE0);
    
    if (PValue) {
        size_t count = (size_t)(Word_t)*PValue;
        
        if (count > 1) {
            // More than one reference, just decrement
            *PValue = (void *)(Word_t)(count - 1);
        }
        else {
            // Last reference - remove it and free the string
            int ret;
            ret = JudyLDel(&context_registry_judyl, (Word_t)context, PJE0);
            if (ret == 1) {
                string_freez(context);
                is_last = true;
            }
        }
    }
    
    spinlock_unlock(&context_registry_spinlock);
    
    return is_last;
}

// Get the current number of unique contexts
size_t rrdcontext_context_registry_unique_count(void) {
    Word_t count = 0;
    
    spinlock_lock(&context_registry_spinlock);
    
    // Count entries manually
    Word_t index = 0;
    Pvoid_t *PValue = JudyLFirst(context_registry_judyl, &index, PJE0);
    
    while (PValue) {
        count++;
        PValue = JudyLNext(context_registry_judyl, &index, PJE0);
    }
    
    spinlock_unlock(&context_registry_spinlock);
    
    return (size_t)count;
}

void rrdcontext_context_registry_json_mcp_array(BUFFER *wb, SIMPLE_PATTERN *pattern) {
    spinlock_lock(&context_registry_spinlock);

    buffer_json_member_add_object(wb, "info");
    {
        buffer_json_member_add_string(wb, "instructions",
                                      "The following is the list of contexts.\n"
                                      "You can get additional information for any context by calling,\n"
                                      "the tool " MCP_TOOL_GET_METRICS_DETAILS " with params:\n"
                                      "`metrics=context1|context2` to get more information about context1 and context2.\n");
    }
    buffer_json_object_close(wb); // info

    buffer_json_member_add_array(wb, "header");
    buffer_json_add_array_item_string(wb, "context");
    buffer_json_add_array_item_string(wb, "number_of_nodes_having_it");
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "contexts");

    Word_t index = 0;
    bool first = true;
    Pvoid_t *PValue;
    while ((PValue = JudyLFirstThenNext(context_registry_judyl, &index, &first))) {
        if (!index || !*PValue) continue;

        const char *context_name = string2str((STRING *)index);
        
        // Skip if we have a pattern and it doesn't match
        if (pattern && !simple_pattern_matches(pattern, context_name))
            continue;

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, context_name);
        buffer_json_add_array_item_uint64(wb, *(size_t *)PValue);
        buffer_json_array_close(wb);
    }

    buffer_json_array_close(wb);

    spinlock_unlock(&context_registry_spinlock);
}

// Implementation to extract and output unique context categories
void rrdcontext_context_registry_json_mcp_categories_array(BUFFER *wb, SIMPLE_PATTERN *pattern) {
    spinlock_lock(&context_registry_spinlock);

    // JudyL array to store unique category STRINGs as keys and counts as values
    Pvoid_t categories_judyl = NULL;
    
    // First pass: count occurrences of each category
    size_t contexts_count = 0, contexts_size = 0;
    Word_t index = 0;
    bool first = true;
    Pvoid_t *PValue;
    while ((PValue = JudyLFirstThenNext(context_registry_judyl, &index, &first))) {
        if (!index || !*PValue) continue;
        
        const char *context_name = string2str((STRING *)index);
        contexts_size += string_strlen((STRING *)index) + 10;
        contexts_count++;

        // Find the last dot in the context name
        const char *first_dot = strchr(context_name, '.');
        
        // Create a STRING for the category (everything up to the last dot)
        STRING *category_str;
        if (first_dot) {
            // Create a STRING with the part before the last dot
            category_str = string_strndupz(context_name, first_dot - context_name);
        } else {
            // No dots, use the entire context as the category
            category_str = string_strdupz(context_name);
        }
        
        if (!category_str) continue;
        
        // Get or insert a slot for this category
        Pvoid_t *CategoryValue = JudyLIns(&categories_judyl, (Word_t)category_str, PJE0);
        
        if (CategoryValue) {
            // Check if this is a new entry
            size_t count = (size_t)(Word_t)*CategoryValue;
            if (count > 0) {
                // Already exists, free our reference (JudyL already has one)
                string_freez(category_str);
            }
            // Increment the count
            *CategoryValue = (void *)(Word_t)(count + 1);
        } else {
            // Failed to insert, free the STRING
            string_freez(category_str);
        }
    }
    
    // Second pass: output the unique categories and their counts

    // Header information
    buffer_json_member_add_object(wb, "info");
    {
        buffer_json_member_add_uint64(wb, "original_contexts_count", contexts_count);
        buffer_json_member_add_uint64(wb, "original_contexts_size", contexts_size);
        buffer_json_member_add_string(wb, "instructions",
                                      "The following list groups metric contexts by prefix.\n"
                                      "In case the original list of contexts is too big to be processed at once,\n"
                                      "use the `q` parameter to fetch the contexts in smaller batches.\n"
                                      "Example: call the " MCP_TOOL_LIST_METRICS " with params:\n"
                                      "`q=system.*|net.*` to get all system.* and net.* contexts\n");
    }
    buffer_json_object_close(wb); // info

    buffer_json_member_add_array(wb, "header");
    buffer_json_add_array_item_string(wb, "category");
    buffer_json_add_array_item_string(wb, "number_of_contexts");
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "categories");

    index = 0;
    first = true;
    while ((PValue = JudyLFirstThenNext(categories_judyl, &index, &first))) {
        if (!index) continue;
        
        STRING *category_str = (STRING *)index;
        const char *category = string2str(category_str);
        
        // Apply pattern filtering here, on the category itself
        if (!pattern || simple_pattern_matches(pattern, category)) {
            size_t count = (size_t)(Word_t)*PValue;
            
            buffer_json_add_array_item_array(wb);
            buffer_json_add_array_item_string(wb, category);
            buffer_json_add_array_item_uint64(wb, count);
            buffer_json_array_close(wb);
        }
        
        // Free the STRING object as we go
        string_freez(category_str);
    }
    
    buffer_json_array_close(wb);
    
    // Free the JudyL array (values were already freed in the loop above)
    JudyLFreeArray(&categories_judyl, PJE0);
    
    spinlock_unlock(&context_registry_spinlock);
}
