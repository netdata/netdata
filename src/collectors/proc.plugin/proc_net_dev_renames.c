// SPDX-License-Identifier: GPL-3.0-or-later

#include "proc_net_dev_renames.h"

DICTIONARY *netdev_renames = NULL;

static void dictionary_netdev_rename_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct rename_task *r = value;

    cgroup_netdev_release(r->cgroup_netdev_link);
    rrdlabels_destroy(r->chart_labels);
    freez((void *) r->container_name);
    freez((void *) r->container_device);
    freez((void *) r->ctx_prefix);
}

void netdev_renames_init(void) {
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

    spinlock_lock(&spinlock);
    if(!netdev_renames) {
        netdev_renames = dictionary_create_advanced(DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct rename_task));
        dictionary_register_delete_callback(netdev_renames, dictionary_netdev_rename_delete_cb, NULL);
    }
    spinlock_unlock(&spinlock);
}

void cgroup_rename_task_add(
    const char *host_device,
    const char *container_device,
    const char *container_name,
    RRDLABELS *labels,
    const char *ctx_prefix,
    const DICTIONARY_ITEM *cgroup_netdev_link)
{
    netdev_renames_init();

    struct rename_task tmp = {
        .container_device = strdupz(container_device),
        .container_name   = strdupz(container_name),
        .ctx_prefix       = strdupz(ctx_prefix),
        .chart_labels     = rrdlabels_create(),
        .cgroup_netdev_link = cgroup_netdev_link,
    };
    rrdlabels_migrate_to_these(tmp.chart_labels, labels);

    dictionary_set(netdev_renames, host_device, &tmp, sizeof(tmp));
}

// other threads can call this function to delete a rename to a netdev
void cgroup_rename_task_device_del(const char *host_device) {
    dictionary_del(netdev_renames, host_device);
}
