// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDCONTEXT_CONTEXT_REGISTRY_H
#define NETDATA_RRDCONTEXT_CONTEXT_REGISTRY_H

#include "libnetdata/libnetdata.h"

/**
 * Initialize the context registry
 * Should be called during Netdata initialization
 */
void rrdcontext_context_registry_init(void);

/**
 * Clean up the context registry
 * Should be called during Netdata shutdown
 */
void rrdcontext_context_registry_destroy(void);

/**
 * Add a context to the registry
 * 
 * @param context The context STRING pointer (guaranteed unique because STRINGs are interned)
 * @return true if this is a new unique context, false if it already existed
 */
bool rrdcontext_context_registry_add(STRING *context);

/**
 * Remove a context from the registry
 * 
 * @param context The context STRING pointer
 * @return true if this was the last reference to this context, false if references remain
 */
bool rrdcontext_context_registry_remove(STRING *context);

/**
 * Get the current number of unique contexts
 * 
 * @return The count of unique contexts in the registry
 */
size_t rrdcontext_context_registry_unique_count(void);

/**
 * Callback function type for context registry traversal
 * 
 * @param context The context STRING pointer
 * @param count The reference count for this context
 * @param data User-provided data passed to rrdcontext_context_registry_foreach
 * @return 0 to continue traversal, non-zero to stop
 */
typedef int (*rrdcontext_context_registry_cb_t)(STRING *context, size_t count, void *data);

/**
 * Traverse all unique contexts in the registry
 * 
 * @param cb Callback function to call for each unique context
 * @param data User data to pass to the callback
 * @return 0 on success, or the non-zero value returned by the callback
 */
int rrdcontext_context_registry_foreach(rrdcontext_context_registry_cb_t cb, void *data);

#endif // NETDATA_RRDCONTEXT_CONTEXT_REGISTRY_H