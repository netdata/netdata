// SPDX-License-Identifier: GPL-3.0-or-later
// This is copied from Legacy ACLK, Original Author: amoss

// TODO unmess this

#ifndef ACLK_COLLECTOR_LIST_H
#define ACLK_COLLECTOR_LIST_H

#include "libnetdata/libnetdata.h"

extern netdata_mutex_t collector_mutex;

#define COLLECTOR_LOCK netdata_mutex_lock(&collector_mutex)
#define COLLECTOR_UNLOCK netdata_mutex_unlock(&collector_mutex)

/*
 * Maintain a list of collectors and chart count
 * If all the charts of a collector are deleted
 * then a new metadata dataset must be send to the cloud
 *
 */
struct _collector {
    time_t created;
    uint32_t count; //chart count
    uint32_t hostname_hash;
    uint32_t plugin_hash;
    uint32_t module_hash;
    char *hostname;
    char *plugin_name;
    char *module_name;
    struct _collector *next;
};

extern struct _collector *collector_list;

struct _collector *_add_collector(const char *hostname, const char *plugin_name, const char *module_name);
struct _collector *_del_collector(const char *hostname, const char *plugin_name, const char *module_name);
void _reset_collector_list();
void _free_collector(struct _collector *collector);

#endif /* ACLK_COLLECTOR_LIST_H */
