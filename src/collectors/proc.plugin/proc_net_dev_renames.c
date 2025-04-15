// SPDX-License-Identifier: GPL-3.0-or-later

#include "proc_net_dev_renames.h"

DICTIONARY *netdev_renames = NULL;

static void netdev_rename_task_cleanup_unsafe(struct rename_task *r) {
    cgroup_netdev_release(r->cgroup_netdev_link);
    r->cgroup_netdev_link = NULL;

    rrdlabels_destroy(r->chart_labels);
    r->chart_labels = NULL;

    freez_and_set_to_null(r->container_name);
    freez_and_set_to_null(r->container_device);
    freez_and_set_to_null(r->ctx_prefix);
}

static void dictionary_netdev_rename_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct rename_task *r = value;

    spinlock_lock(&r->spinlock);
    netdev_rename_task_cleanup_unsafe(r);
    spinlock_unlock(&r->spinlock);
}

static bool dictionary_netdev_rename_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    struct rename_task *old_r = old_value;
    struct rename_task *new_r = new_value;

    spinlock_lock(&old_r->spinlock);
    SWAP(old_r->cgroup_netdev_link, new_r->cgroup_netdev_link);
    SWAP(old_r->chart_labels, new_r->chart_labels);
    SWAP(old_r->container_device, new_r->container_device);
    SWAP(old_r->container_name, new_r->container_name);
    SWAP(old_r->ctx_prefix, new_r->ctx_prefix);
    spinlock_unlock(&old_r->spinlock);

    netdev_rename_task_cleanup_unsafe(new_r);

    return true;
}

static SPINLOCK netdev_renames_spinlock = SPINLOCK_INITIALIZER;
void netdev_renames_init(void) {
    spinlock_lock(&netdev_renames_spinlock);
    if(!netdev_renames) {
        netdev_renames = dictionary_create_advanced(DICT_OPTION_FIXED_SIZE | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, sizeof(struct rename_task));
        dictionary_register_conflict_callback(netdev_renames, dictionary_netdev_rename_conflict_cb, NULL);
        dictionary_register_delete_callback(netdev_renames, dictionary_netdev_rename_delete_cb, NULL);
    }
    spinlock_unlock(&netdev_renames_spinlock);
}

void netdev_renames_destroy(void) {
    spinlock_lock(&netdev_renames_spinlock);
    dictionary_destroy(netdev_renames);
    netdev_renames = NULL;
    spinlock_unlock(&netdev_renames_spinlock);
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
