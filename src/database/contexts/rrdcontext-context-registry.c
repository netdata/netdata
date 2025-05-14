// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext.h"
#include "rrdcontext-internal.h"
#include "rrdcontext-context-registry.h"

// The registry - using a raw JudyL array
// Key: STRING pointer
// Value: reference count (size_t)
static Pvoid_t context_registry_judyl = NULL;

// Spinlock to protect access to the registry
static SPINLOCK context_registry_spinlock = SPINLOCK_INITIALIZER;

// Initialize the context registry
void rrdcontext_context_registry_init(void) {
    // Nothing to do here - context_registry_judyl is already NULL-initialized
}

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

// Traverse all unique contexts in the registry
int rrdcontext_context_registry_foreach(rrdcontext_context_registry_cb_t cb, void *data) {
    if (unlikely(!cb))
        return 0;
    
    int ret = 0;
    
    spinlock_lock(&context_registry_spinlock);
    
    // Traverse the registry using JudyLFirst/JudyLNext
    Word_t index = 0;
    Pvoid_t *PValue = JudyLFirst(context_registry_judyl, &index, PJE0);
    
    while (PValue && ret == 0) {
        size_t count = (size_t)(Word_t)*PValue;
        ret = cb((STRING *)index, count, data);
        PValue = JudyLNext(context_registry_judyl, &index, PJE0);
    }
    
    spinlock_unlock(&context_registry_spinlock);
    
    return ret;
}