// SPDX-License-Identifier: GPL-3.0-or-later

#include "proc_net_dev_renames.h"

DICTIONARY *netdev_renames = NULL;

static XXH64_hash_t rename_task_hash(struct rename_task *r) {
    struct rename_task local_copy = *r;
    local_copy.checksum = 0;
    return XXH3_64bits(&local_copy, sizeof(local_copy));
}

// Set the checksum for a rename_task
static void rename_task_set_checksum(struct rename_task *r) {
    r->checksum = rename_task_hash(r);
}

// Verify the checksum for a rename_task
void rename_task_verify_checksum(struct rename_task *r) {
    if(r->checksum != rename_task_hash(r))
        fatal("MEMORY CORRUPTION DETECTED in rename_task structure. "
              "Expected checksum: 0x%016" PRIx64 ", "
              "Calculated checksum: 0x%016" PRIx64,
              r->checksum, rename_task_hash(r));
}

static void dictionary_netdev_rename_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct rename_task *r = value;

    rename_task_verify_checksum(r);

    cgroup_netdev_release(r->cgroup_netdev_link);
    r->cgroup_netdev_link = NULL;

    rrdlabels_destroy(r->chart_labels);
    r->chart_labels = NULL;

    freez_and_set_to_null(r->container_name);
    freez_and_set_to_null(r->container_device);
    freez_and_set_to_null(r->ctx_prefix);
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
        .checksum = 0, // Will be set below
    };
    rrdlabels_migrate_to_these(tmp.chart_labels, labels);
    
    // Set the checksum after all fields are initialized
    rename_task_set_checksum(&tmp);

    dictionary_set(netdev_renames, host_device, &tmp, sizeof(tmp));
}

// other threads can call this function to delete a rename to a netdev
void cgroup_rename_task_device_del(const char *host_device) {
    dictionary_del(netdev_renames, host_device);
}
