// SPDX-License-Identifier: GPL-3.0-or-later
// This is copied from Legacy ACLK, Original Author: amoss

// TODO unmess this

#include "aclk_collector_list.h"

netdata_mutex_t collector_mutex = NETDATA_MUTEX_INITIALIZER;

struct _collector *collector_list = NULL;

/*
 * Free a collector structure
 */
void _free_collector(struct _collector *collector)
{
    if (likely(collector->plugin_name))
        freez(collector->plugin_name);

    if (likely(collector->module_name))
        freez(collector->module_name);

    if (likely(collector->hostname))
        freez(collector->hostname);

    freez(collector);
}

/*
 * This will report the collector list
 *
 */
#ifdef ACLK_DEBUG
static void _dump_collector_list()
{
    struct _collector *tmp_collector;

    COLLECTOR_LOCK;

    info("DUMPING ALL COLLECTORS");

    if (unlikely(!collector_list || !collector_list->next)) {
        COLLECTOR_UNLOCK;
        info("DUMPING ALL COLLECTORS -- nothing found");
        return;
    }

    // Note that the first entry is "dummy"
    tmp_collector = collector_list->next;

    while (tmp_collector) {
        info(
            "COLLECTOR %s : [%s:%s] count = %u", tmp_collector->hostname,
            tmp_collector->plugin_name ? tmp_collector->plugin_name : "",
            tmp_collector->module_name ? tmp_collector->module_name : "", tmp_collector->count);

        tmp_collector = tmp_collector->next;
    }
    info("DUMPING ALL COLLECTORS DONE");
    COLLECTOR_UNLOCK;
}
#endif

/*
 * This will cleanup the collector list
 *
 */
void _reset_collector_list()
{
    struct _collector *tmp_collector, *next_collector;

    COLLECTOR_LOCK;

    if (unlikely(!collector_list || !collector_list->next)) {
        COLLECTOR_UNLOCK;
        return;
    }

    // Note that the first entry is "dummy"
    tmp_collector = collector_list->next;
    collector_list->count = 0;
    collector_list->next = NULL;

    // We broke the link; we can unlock
    COLLECTOR_UNLOCK;

    while (tmp_collector) {
        next_collector = tmp_collector->next;
        _free_collector(tmp_collector);
        tmp_collector = next_collector;
    }
}

/*
 * Find a collector (if it exists)
 * Must lock before calling this
 * If last_collector is not null, it will return the previous collector in the linked
 * list (used in collector delete)
 */
static struct _collector *_find_collector(
    const char *hostname, const char *plugin_name, const char *module_name, struct _collector **last_collector)
{
    struct _collector *tmp_collector, *prev_collector;
    uint32_t plugin_hash;
    uint32_t module_hash;
    uint32_t hostname_hash;

    if (unlikely(!collector_list)) {
        collector_list = callocz(1, sizeof(struct _collector));
        return NULL;
    }

    if (unlikely(!collector_list->next))
        return NULL;

    plugin_hash = plugin_name ? simple_hash(plugin_name) : 1;
    module_hash = module_name ? simple_hash(module_name) : 1;
    hostname_hash = simple_hash(hostname);

    // Note that the first entry is "dummy"
    tmp_collector = collector_list->next;
    prev_collector = collector_list;
    while (tmp_collector) {
        if (plugin_hash == tmp_collector->plugin_hash && module_hash == tmp_collector->module_hash &&
            hostname_hash == tmp_collector->hostname_hash && (!strcmp(hostname, tmp_collector->hostname)) &&
            (!plugin_name || !tmp_collector->plugin_name || !strcmp(plugin_name, tmp_collector->plugin_name)) &&
            (!module_name || !tmp_collector->module_name || !strcmp(module_name, tmp_collector->module_name))) {
            if (unlikely(last_collector))
                *last_collector = prev_collector;

            return tmp_collector;
        }

        prev_collector = tmp_collector;
        tmp_collector = tmp_collector->next;
    }

    return tmp_collector;
}

/*
 * Called to delete a collector
 * It will reduce the count (chart_count) and will remove it
 * from the linked list if the count reaches zero
 * The structure will be returned to the caller to free
 * the resources
 *
 */
struct _collector *_del_collector(const char *hostname, const char *plugin_name, const char *module_name)
{
    struct _collector *tmp_collector, *prev_collector = NULL;

    tmp_collector = _find_collector(hostname, plugin_name, module_name, &prev_collector);

    if (likely(tmp_collector)) {
        --tmp_collector->count;
        if (unlikely(!tmp_collector->count))
            prev_collector->next = tmp_collector->next;
    }
    return tmp_collector;
}

/*
 * Add a new collector (plugin / module) to the list
 * If it already exists just update the chart count
 *
 * Lock before calling
 */
struct _collector *_add_collector(const char *hostname, const char *plugin_name, const char *module_name)
{
    struct _collector *tmp_collector;

    tmp_collector = _find_collector(hostname, plugin_name, module_name, NULL);

    if (unlikely(!tmp_collector)) {
        tmp_collector = callocz(1, sizeof(struct _collector));
        tmp_collector->hostname_hash = simple_hash(hostname);
        tmp_collector->plugin_hash = plugin_name ? simple_hash(plugin_name) : 1;
        tmp_collector->module_hash = module_name ? simple_hash(module_name) : 1;

        tmp_collector->hostname = strdupz(hostname);
        tmp_collector->plugin_name = plugin_name ? strdupz(plugin_name) : NULL;
        tmp_collector->module_name = module_name ? strdupz(module_name) : NULL;

        tmp_collector->next = collector_list->next;
        collector_list->next = tmp_collector;
    }
    tmp_collector->count++;
    debug(
        D_ACLK, "ADD COLLECTOR %s [%s:%s] -- chart %u", hostname, plugin_name ? plugin_name : "*",
        module_name ? module_name : "*", tmp_collector->count);
    return tmp_collector;
}
