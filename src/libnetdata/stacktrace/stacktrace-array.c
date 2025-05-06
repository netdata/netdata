// SPDX-License-Identifier: GPL-3.0-or-later

#include "stacktrace-array.h"
#include "stacktrace-common.h"

// Initialize a stacktrace array
void stacktrace_array_init(STACKTRACE_ARRAY *array) {
    if (unlikely(!array))
        return;

    spinlock_init(&array->spinlock);
    array->num_stacktraces = 0;
    memset(array->stacktraces, 0, sizeof(array->stacktraces));
}

// Add a stacktrace to an array (captures current stacktrace)
NEVER_INLINE
bool stacktrace_array_add(STACKTRACE_ARRAY *array, int skip_frames) {
    if (unlikely(!array))
        return false;

    // Get current stacktrace
    STACKTRACE current = stacktrace_get(skip_frames + 1);  // +1 to skip this function
    if (!current)
        return false;
        
    bool added = false;
    
    // Protect the stacktraces array with a spinlock
    spinlock_lock(&array->spinlock);
    
    // Check if this stacktrace already exists in the array
    bool found = false;
    for (int i = 0; i < array->num_stacktraces; i++) {
        if (array->stacktraces[i] == current) {
            found = true;
            break;
        }
    }
    
    // Add the stacktrace if it's unique and there's room
    if (!found && array->num_stacktraces < STACKTRACE_ARRAY_MAX_TRACES) {
        array->stacktraces[array->num_stacktraces++] = current;
        added = true;
    }
    
    spinlock_unlock(&array->spinlock);
    
    return added;
}

// Report stacktraces to a buffer
size_t stacktrace_array_to_buffer(STACKTRACE_ARRAY *array, BUFFER *wb, size_t *total_count, const char *prefix, bool brief_output) {
    if (unlikely(!array || !wb))
        return 0;
        
    if (!prefix)
        prefix = "STACKTRACE";
        
    size_t reported = 0;
    
    // Lock the array while we're operating on it
    spinlock_lock(&array->spinlock);
    
    // Update total count if requested
    if (total_count)
        *total_count = array->num_stacktraces;
    
    // If brief output is requested, just report the number of stacktraces
    if (brief_output) {
        buffer_sprintf(wb, "%s: %d stacktraces captured\n", prefix, array->num_stacktraces);
        spinlock_unlock(&array->spinlock);
        return array->num_stacktraces;
    }
    
    // Report each stacktrace in the array
    for (int i = 0; i < array->num_stacktraces; i++) {
        if (array->stacktraces[i]) {
            if (i > 0)
                buffer_strcat(wb, "\n");
                
            buffer_sprintf(wb, "%s #%d:\n", prefix, i+1);
            stacktrace_to_buffer(array->stacktraces[i], wb);
            reported++;
        }
    }
    
    spinlock_unlock(&array->spinlock);
    
    return reported;
}